package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	defaultReadLimitLines = 2000
	maxReadLimitLines     = 5000
)

type readFileInput struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type editFileInput struct {
	Path           string `json:"path"`
	OldString      string `json:"old_string"`
	NewString      string `json:"new_string"`
	ReplaceAll     bool   `json:"replace_all,omitempty"`
	ExpectedSHA256 string `json:"expected_sha256,omitempty"`
}

type writeFileInput struct {
	Path           string `json:"path"`
	Content        string `json:"content"`
	ExpectedSHA256 string `json:"expected_sha256,omitempty"`
}

func (r *Registry) makeReadFileTool() Tool {
	return Tool{
		Name:            "read_file",
		Description:     "Read a file or directory listing within the workspace with 1-based line numbers and SHA-256 precondition tracking.",
		InputSchema:     json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Workspace-relative or absolute path to read"},"offset":{"type":"integer","description":"1-based starting line number (default 1)"},"limit":{"type":"integer","description":"Maximum number of lines to read (default 2000)"}},"required":["path"]}`),
		ReadOnly:        true,
		Mutating:        false,
		ConcurrencySafe: true,
		Handler:         r.handleReadFile,
	}
}

func (r *Registry) makeEditFileTool() Tool {
	return Tool{
		Name:            "edit_file",
		Description:     "Apply a targeted edit to an existing file using 4-stage fuzzy matching, SHA-256 precondition verification, and snapshot backup.",
		InputSchema:     json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Target file path"},"old_string":{"type":"string","description":"Existing text to locate and replace"},"new_string":{"type":"string","description":"Replacement text"},"replace_all":{"type":"boolean","description":"Replace every occurrence instead of requiring a unique match"},"expected_sha256":{"type":"string","description":"Optional SHA-256 hex precondition"}},"required":["path","old_string","new_string"]}`),
		ReadOnly:        false,
		Mutating:        true,
		ConcurrencySafe: false,
		Handler:         r.handleEditFile,
	}
}

func (r *Registry) makeWriteFileTool() Tool {
	return Tool{
		Name:            "write_file",
		Description:     "Create a new file or atomically replace an existing file's contents with snapshot backup.",
		InputSchema:     json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Target file path"},"content":{"type":"string","description":"Complete content to write"},"expected_sha256":{"type":"string","description":"Optional SHA-256 hex precondition when overwriting"}},"required":["path","content"]}`),
		ReadOnly:        false,
		Mutating:        true,
		ConcurrencySafe: false,
		Handler:         r.handleWriteFile,
	}
}

func (r *Registry) handleReadFile(_ context.Context, call CallContext, rawInput json.RawMessage) (ToolResult, error) {
	var in readFileInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return ToolResult{}, fmt.Errorf("invalid read_file input: %w", err)
	}
	pg, err := r.resolvePathGuard(call)
	if err != nil {
		return ToolResult{}, err
	}
	canonAbs, rel, inWS, err := pg.ResolveInWorkspace(in.Path)
	if err != nil {
		return ToolResult{}, err
	}
	isOverflowLog := pg.IsOverflowLogFile(canonAbs)
	if !inWS && !call.AllowOutsideWorkspace && !isOverflowLog {
		return ToolResult{}, fmt.Errorf("%w: %s", ErrPathEscapesWorkspace, in.Path)
	}

	displayPath := rel
	if displayPath == "" {
		displayPath = canonAbs
	}

	st, err := os.Stat(canonAbs)
	if err != nil {
		return ToolResult{}, err
	}

	logsDir := r.resolveLogsDir(call)
	if st.IsDir() {
		entries, err := os.ReadDir(canonAbs)
		if err != nil {
			return ToolResult{}, err
		}
		names := make([]string, 0, len(entries))
		for _, ent := range entries {
			n := ent.Name()
			if ent.IsDir() {
				n += "/"
			}
			names = append(names, n)
		}
		sort.Strings(names)
		body := strings.Join(names, "\n")
		return ToolResult{
			Output:      wrapUntrustedWithDir("read_file:"+displayPath, body, logsDir),
			Metadata:    map[string]any{"path": displayPath, "is_dir": true, "in_workspace": inWS},
			TouchedPath: rel,
		}, nil
	}

	raw, err := os.ReadFile(canonAbs)
	if err != nil {
		return ToolResult{}, err
	}
	fileHash := sha256HexBytes(raw)
	r.RecordReadHash(call.SessionID, canonAbs, fileHash)

	if isLikelyBinary(raw) {
		msg := fmt.Sprintf("[binary file: %s | bytes: %d | sha256: %s]", displayPath, len(raw), fileHash)
		return ToolResult{
			Output:      wrapUntrustedWithDir("read_file:"+displayPath, msg, logsDir),
			Metadata:    map[string]any{"path": displayPath, "sha256": fileHash, "binary": true, "in_workspace": inWS},
			TouchedPath: rel,
		}, nil
	}

	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	var lines []string
	if len(raw) > 0 {
		lines = strings.Split(text, "\n")
	}

	offset := in.Offset
	if offset <= 0 {
		offset = 1
	}
	limit := in.Limit
	if limit <= 0 {
		limit = defaultReadLimitLines
	}
	if limit > maxReadLimitLines {
		limit = maxReadLimitLines
	}

	startIdx := offset - 1
	if startIdx > len(lines) {
		startIdx = len(lines)
	}
	endIdx := startIdx + limit
	if endIdx > len(lines) {
		endIdx = len(lines)
	}

	var formatted strings.Builder
	fmt.Fprintf(&formatted, "[file: %s | lines: %d-%d of %d | sha256: %s]\n",
		displayPath, offset, endIdx, len(lines), fileHash)
	for i := startIdx; i < endIdx; i++ {
		fmt.Fprintf(&formatted, "%d: %s\n", i+1, lines[i])
	}

	return ToolResult{
		Output:      wrapUntrustedWithDir("read_file:"+displayPath, strings.TrimRight(formatted.String(), "\n"), logsDir),
		Metadata:    map[string]any{"path": displayPath, "sha256": fileHash, "total_lines": len(lines), "in_workspace": inWS},
		TouchedPath: rel,
	}, nil
}

func (r *Registry) stageEditFromInput(call CallContext, toolName string, rawInput json.RawMessage) (*StagedEdit, error) {
	pg, err := r.resolvePathGuard(call)
	if err != nil {
		return nil, err
	}

	switch toolName {
	case "edit_file":
		var in editFileInput
		if err := json.Unmarshal(rawInput, &in); err != nil {
			return nil, fmt.Errorf("invalid edit_file input: %w", err)
		}
		canonAbs, rel, inWS, err := pg.ResolveInWorkspace(in.Path)
		if err != nil {
			return nil, err
		}
		if pg.IsOverflowLogFile(canonAbs) {
			return nil, fmt.Errorf("%w: %s", ErrVaultSecretBlocked, in.Path)
		}
		if !inWS && !call.AllowOutsideWorkspace {
			return nil, fmt.Errorf("%w: %s", ErrPathEscapesWorkspace, in.Path)
		}
		readHash := strings.TrimSpace(in.ExpectedSHA256)
		if readHash == "" {
			readHash = r.LastReadHash(call.SessionID, canonAbs)
		}
		return ComputeStagedEdit(canonAbs, rel, in.OldString, in.NewString, in.ReplaceAll, readHash)

	case "write_file":
		var in writeFileInput
		if err := json.Unmarshal(rawInput, &in); err != nil {
			return nil, fmt.Errorf("invalid write_file input: %w", err)
		}
		canonAbs, rel, inWS, err := pg.ResolveInWorkspace(in.Path)
		if err != nil {
			return nil, err
		}
		if pg.IsOverflowLogFile(canonAbs) {
			return nil, fmt.Errorf("%w: %s", ErrVaultSecretBlocked, in.Path)
		}
		if !inWS && !call.AllowOutsideWorkspace {
			return nil, fmt.Errorf("%w: %s", ErrPathEscapesWorkspace, in.Path)
		}
		readHash := strings.TrimSpace(in.ExpectedSHA256)
		if readHash == "" {
			if _, stErr := os.Lstat(canonAbs); stErr == nil {
				readHash = r.LastReadHash(call.SessionID, canonAbs)
			}
		}
		return ComputeStagedWrite(canonAbs, rel, []byte(in.Content), readHash)

	default:
		return nil, fmt.Errorf("tool %q does not support staged hunk diffs", toolName)
	}
}

func (r *Registry) handleEditFile(_ context.Context, call CallContext, rawInput json.RawMessage) (ToolResult, error) {
	staged, err := r.stageEditFromInput(call, "edit_file", rawInput)
	if err != nil {
		return ToolResult{}, err
	}
	return r.applyStagedAndFormat(call, staged)
}

func (r *Registry) handleWriteFile(_ context.Context, call CallContext, rawInput json.RawMessage) (ToolResult, error) {
	staged, err := r.stageEditFromInput(call, "write_file", rawInput)
	if err != nil {
		return ToolResult{}, err
	}
	if len(staged.Hunks) == 0 && bytes.Equal(staged.BeforeBytes, staged.ProposedBytes) && staged.PreconditionHash != "" {
		return ToolResult{
			Output:      fmt.Sprintf(`{"path":%q,"written":false,"applied_hunks":0,"total_hunks":0,"unchanged":true}`, displayStagedPath(staged)),
			Staged:      staged,
			TouchedPath: staged.RelPath,
		}, nil
	}
	return r.applyStagedAndFormat(call, staged)
}

func (r *Registry) applyStagedAndFormat(call CallContext, staged *StagedEdit) (ToolResult, error) {
	if staged == nil {
		return ToolResult{}, errors.New("nil staged edit")
	}
	var selected []string
	if call.SelectedHunkIDs != nil {
		selected = call.SelectedHunkIDs
	} else {
		selected = make([]string, 0, len(staged.Hunks))
		for _, h := range staged.Hunks {
			selected = append(selected, h.ID)
		}
	}

	snaps := call.Snapshots
	if snaps == nil {
		snaps = r.snapshots
	}

	written, applied, err := ApplySelectedHunks(staged, selected, snaps, call.SessionID, call.Seq)
	if err != nil {
		return ToolResult{}, err
	}
	if written {
		if diskBytes, rErr := os.ReadFile(staged.AbsPath); rErr == nil {
			r.RecordReadHash(call.SessionID, staged.AbsPath, sha256HexBytes(diskBytes))
		}
	}

	disp := displayStagedPath(staged)
	summary, _ := json.Marshal(map[string]any{
		"path":          disp,
		"written":       written,
		"applied_hunks": applied,
		"total_hunks":   len(staged.Hunks),
	})

	return ToolResult{
		Output: string(summary),
		Metadata: map[string]any{
			"path":          disp,
			"written":       written,
			"applied_hunks": applied,
			"total_hunks":   len(staged.Hunks),
		},
		Staged:      staged,
		TouchedPath: staged.RelPath,
	}, nil
}

func displayStagedPath(s *StagedEdit) string {
	if s.RelPath != "" && s.RelPath != "." {
		return s.RelPath
	}
	return s.AbsPath
}

func isLikelyBinary(data []byte) bool {
	probe := data
	if len(probe) > 8192 {
		probe = probe[:8192]
	}
	return bytes.IndexByte(probe, 0) >= 0
}
