package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/store"
)

type publishArtifactInput struct {
	ID      string `json:"id,omitempty"`
	Title   string `json:"title"`
	Format  string `json:"format,omitempty"`
	Content string `json:"content"`
}

func (r *Registry) makePublishArtifactTool() Tool {
	return Tool{
		Name:            "publish_artifact",
		Description:     "Publish a rendered HTML or Markdown artifact to the workspace artifact store so it appears in the Artifacts canvas.",
		InputSchema:     json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Optional safe artifact ID (letters, digits, hyphens, underscores)"},"title":{"type":"string","description":"Human-readable artifact title"},"format":{"type":"string","enum":["html","md"],"description":"Artifact format: html or md (default md)"},"content":{"type":"string","description":"Complete HTML or Markdown content"}},"required":["title","content"]}`),
		ReadOnly:        false,
		Mutating:        true,
		ConcurrencySafe: true,
		Handler:         r.handlePublishArtifact,
	}
}

func (r *Registry) handlePublishArtifact(_ context.Context, call CallContext, rawInput json.RawMessage) (ToolResult, error) {
	var in publishArtifactInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return ToolResult{}, fmt.Errorf("invalid publish_artifact input: %w", err)
	}
	if strings.TrimSpace(in.Title) == "" {
		return ToolResult{}, errors.New("publish_artifact requires a non-empty title")
	}
	if strings.TrimSpace(in.Content) == "" {
		return ToolResult{}, errors.New("publish_artifact requires non-empty content")
	}

	artStore := r.resolveArtifactStore(call)
	if artStore == nil {
		return ToolResult{}, errors.New("artifact store is not configured (missing vault root)")
	}

	wsID := strings.TrimSpace(call.WorkspaceID)
	if wsID == "" && r != nil {
		wsID = strings.TrimSpace(r.wsID)
	}
	if wsID == "" {
		wsID = "default"
	}

	format := strings.ToLower(strings.TrimSpace(in.Format))
	if format == "" {
		format = "md"
	}

	saved, err := artStore.Save(wsID, store.Artifact{
		ID:      strings.TrimSpace(in.ID),
		Title:   strings.TrimSpace(in.Title),
		Format:  format,
		Content: in.Content,
	})
	if err != nil {
		return ToolResult{}, err
	}

	summary, _ := json.Marshal(map[string]any{
		"id":           saved.ID,
		"workspace_id": saved.WorkspaceID,
		"title":        saved.Title,
		"format":       saved.Format,
		"size_bytes":   saved.SizeBytes,
	})
	return ToolResult{
		Output: string(summary),
		Metadata: map[string]any{
			"id":           saved.ID,
			"workspace_id": saved.WorkspaceID,
			"title":        saved.Title,
			"format":       saved.Format,
			"size_bytes":   saved.SizeBytes,
		},
	}, nil
}

func (r *Registry) resolveArtifactStore(call CallContext) *store.ArtifactStore {
	if call.Artifacts != nil {
		return call.Artifacts
	}
	if r != nil && r.artifacts != nil {
		return r.artifacts
	}
	vault := strings.TrimSpace(call.VaultRoot)
	if vault == "" && r != nil {
		vault = strings.TrimSpace(r.vaultPaths.Root)
	}
	if vault == "" {
		return nil
	}
	as, err := store.NewArtifactStore(config.VaultPaths{Root: vault})
	if err != nil {
		return nil
	}
	return as
}
