package server

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorness.dev/gorness/internal/alwayson"
	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/core"
	"gorness.dev/gorness/internal/store"
)

func (s *Server) memoryStore() (*alwayson.MemoryStore, bool) {
	if s.alwaysOn != nil && s.alwaysOn.Memory() != nil {
		return s.alwaysOn.Memory(), true
	}
	if s.fallbackMemory != nil {
		return s.fallbackMemory, true
	}
	return nil, false
}

func (s *Server) notifyFallbackMemoryChanged() {
	if s.alwaysOn == nil && s.engine != nil {
		s.engine.PublishSystemEvent(core.EventAlwaysOnUpdated, map[string]any{
			"action": "memory_updated",
		})
	}
}

func (s *Server) requireWorkspace(wsID string) (*store.Workspace, int, error) {
	cleanID := strings.TrimSpace(wsID)
	if err := store.ValidateSafeID(cleanID); err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("invalid workspace id: %w", err)
	}
	if s.engine == nil || s.engine.Workspaces() == nil {
		return nil, http.StatusServiceUnavailable, errors.New("workspace store unavailable")
	}
	ws, err := s.engine.Workspaces().Get(cleanID)
	if err != nil {
		return nil, http.StatusNotFound, err
	}
	return ws, http.StatusOK, nil
}

// ============================================================================
// 4-Layer Pointer Memory Endpoints (/api/memory/*, /api/workspaces/{wsID}/memory/*)
// ============================================================================

func (s *Server) handleGetMemory(w http.ResponseWriter, r *http.Request) {
	mem, ok := s.memoryStore()
	if !ok {
		writeJSONError(w, http.StatusServiceUnavailable, "memory store unavailable")
		return
	}

	instructions, err := mem.GetGlobalLayer(alwayson.MemoryLayerInstructions)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	index, err := mem.GetGlobalLayer(alwayson.MemoryLayerIndex)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	noteItems, err := mem.ListNotes()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if noteItems == nil {
		noteItems = []alwayson.MemoryNote{}
	}
	notesMap := make(map[string]string, len(noteItems))
	for i := range noteItems {
		noteItems[i].Title = config.EscapeBidi(noteItems[i].Title)
		noteItems[i].Summary = config.EscapeBidi(noteItems[i].Summary)
		noteItems[i].Content = config.EscapeBidi(noteItems[i].Content)
		notesMap[noteItems[i].Slug] = noteItems[i].Content
	}

	progressMap := map[string]string{}
	progressItems := []alwayson.ProgressEntry{}
	if wsID := strings.TrimSpace(r.URL.Query().Get("workspace_id")); wsID != "" {
		if _, status, wsErr := s.requireWorkspace(wsID); wsErr != nil {
			writeJSONError(w, status, wsErr.Error())
			return
		}
		if list, pErr := mem.ListProgress(wsID); pErr == nil {
			progressItems = list
			for i := range progressItems {
				progressItems[i].Title = config.EscapeBidi(progressItems[i].Title)
				progressItems[i].Content = config.EscapeBidi(progressItems[i].Content)
				progressMap[progressItems[i].Slug] = progressItems[i].Content
			}
		}
	}
	if progressItems == nil {
		progressItems = []alwayson.ProgressEntry{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"instructions":   config.EscapeBidi(instructions),
		"index":          config.EscapeBidi(index),
		"notes":          notesMap,
		"note_items":     noteItems,
		"progress":       progressMap,
		"progress_items": progressItems,
	})
}

func (s *Server) handleUpdateMemoryLayer(w http.ResponseWriter, r *http.Request) {
	mem, ok := s.memoryStore()
	if !ok {
		writeJSONError(w, http.StatusServiceUnavailable, "memory store unavailable")
		return
	}
	var in struct {
		Layer   string `json:"layer"`
		Content string `json:"content"`
	}
	if err := decodeJSONBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := mem.SaveGlobalLayer(in.Layer, in.Content); err != nil {
		if errors.Is(err, alwayson.ErrInvalidMemoryLayer) {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.notifyFallbackMemoryChanged()
	saved, _ := mem.GetGlobalLayer(in.Layer)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"layer":   strings.ToLower(strings.TrimSpace(in.Layer)),
		"content": config.EscapeBidi(saved),
	})
}

func (s *Server) handleGetMemoryNote(w http.ResponseWriter, r *http.Request) {
	mem, ok := s.memoryStore()
	if !ok {
		writeJSONError(w, http.StatusServiceUnavailable, "memory store unavailable")
		return
	}
	slug := strings.TrimSpace(r.PathValue("slug"))
	note, err := mem.GetNote(slug)
	if err != nil {
		switch {
		case errors.Is(err, alwayson.ErrMemoryNotFound):
			writeJSONError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, store.ErrInvalidID):
			writeJSONError(w, http.StatusBadRequest, err.Error())
		default:
			writeJSONError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	note.Title = config.EscapeBidi(note.Title)
	note.Summary = config.EscapeBidi(note.Summary)
	note.Content = config.EscapeBidi(note.Content)
	writeJSON(w, http.StatusOK, note)
}

func (s *Server) handleSaveMemoryNote(w http.ResponseWriter, r *http.Request) {
	mem, ok := s.memoryStore()
	if !ok {
		writeJSONError(w, http.StatusServiceUnavailable, "memory store unavailable")
		return
	}
	slug := strings.TrimSpace(r.PathValue("slug"))
	var in struct {
		Title   *string   `json:"title"`
		Content string    `json:"content"`
		Tags    *[]string `json:"tags"`
	}
	if err := decodeJSONBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	var title string
	var tags []string
	if existing, gErr := mem.GetNote(slug); gErr == nil {
		title = existing.Title
		tags = existing.Tags
	}
	if in.Title != nil && strings.TrimSpace(*in.Title) != "" {
		title = strings.TrimSpace(*in.Title)
	}
	if in.Tags != nil {
		tags = *in.Tags
	}

	saved, err := mem.SaveNote(slug, title, in.Content, tags)
	if err != nil {
		if errors.Is(err, store.ErrInvalidID) {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.notifyFallbackMemoryChanged()
	saved.Title = config.EscapeBidi(saved.Title)
	saved.Summary = config.EscapeBidi(saved.Summary)
	saved.Content = config.EscapeBidi(saved.Content)
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleDeleteMemoryNote(w http.ResponseWriter, r *http.Request) {
	mem, ok := s.memoryStore()
	if !ok {
		writeJSONError(w, http.StatusServiceUnavailable, "memory store unavailable")
		return
	}
	slug := strings.TrimSpace(r.PathValue("slug"))
	if err := mem.DeleteNote(slug); err != nil {
		switch {
		case errors.Is(err, alwayson.ErrMemoryNotFound):
			writeJSONError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, store.ErrInvalidID):
			writeJSONError(w, http.StatusBadRequest, err.Error())
		default:
			writeJSONError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.notifyFallbackMemoryChanged()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"deleted": slug,
	})
}

func (s *Server) handleListWorkspaceProgress(w http.ResponseWriter, r *http.Request) {
	mem, ok := s.memoryStore()
	if !ok {
		writeJSONError(w, http.StatusServiceUnavailable, "memory store unavailable")
		return
	}
	wsID := strings.TrimSpace(r.PathValue("wsID"))
	if _, status, err := s.requireWorkspace(wsID); err != nil {
		writeJSONError(w, status, err.Error())
		return
	}
	entries, err := mem.ListProgress(wsID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if entries == nil {
		entries = []alwayson.ProgressEntry{}
	}
	notesMap := make(map[string]string, len(entries))
	for i := range entries {
		entries[i].Title = config.EscapeBidi(entries[i].Title)
		entries[i].Content = config.EscapeBidi(entries[i].Content)
		notesMap[entries[i].Slug] = entries[i].Content
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workspace_id": wsID,
		"entries":      entries,
		"notes":        notesMap,
	})
}

func (s *Server) handleGetWorkspaceProgress(w http.ResponseWriter, r *http.Request) {
	mem, ok := s.memoryStore()
	if !ok {
		writeJSONError(w, http.StatusServiceUnavailable, "memory store unavailable")
		return
	}
	wsID := strings.TrimSpace(r.PathValue("wsID"))
	if _, status, err := s.requireWorkspace(wsID); err != nil {
		writeJSONError(w, status, err.Error())
		return
	}
	slug := strings.TrimSpace(r.PathValue("slug"))

	allEntries, _ := mem.ListProgress(wsID)
	notesMap := make(map[string]string, len(allEntries))
	for _, p := range allEntries {
		notesMap[p.Slug] = config.EscapeBidi(p.Content)
	}

	entry, err := mem.GetProgress(wsID, slug)
	if err != nil {
		switch {
		case errors.Is(err, alwayson.ErrMemoryNotFound):
			if strings.EqualFold(slug, "current") {
				writeJSON(w, http.StatusOK, map[string]any{
					"workspace_id": wsID,
					"slug":         "current",
					"title":        "current",
					"content":      "",
					"notes":        notesMap,
				})
				return
			}
			writeJSONError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, store.ErrInvalidID):
			writeJSONError(w, http.StatusBadRequest, err.Error())
		default:
			writeJSONError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"workspace_id": entry.WorkspaceID,
		"slug":         entry.Slug,
		"title":        config.EscapeBidi(entry.Title),
		"content":      config.EscapeBidi(entry.Content),
		"updated_at":   entry.UpdatedAt,
		"notes":        notesMap,
	})
}

func (s *Server) handleSaveWorkspaceProgress(w http.ResponseWriter, r *http.Request) {
	mem, ok := s.memoryStore()
	if !ok {
		writeJSONError(w, http.StatusServiceUnavailable, "memory store unavailable")
		return
	}
	wsID := strings.TrimSpace(r.PathValue("wsID"))
	if _, status, err := s.requireWorkspace(wsID); err != nil {
		writeJSONError(w, status, err.Error())
		return
	}
	slug := strings.TrimSpace(r.PathValue("slug"))
	var in struct {
		Content string `json:"content"`
	}
	if err := decodeJSONBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	saved, err := mem.SaveProgress(wsID, slug, in.Content)
	if err != nil {
		if errors.Is(err, store.ErrInvalidID) {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.notifyFallbackMemoryChanged()
	saved.Title = config.EscapeBidi(saved.Title)
	saved.Content = config.EscapeBidi(saved.Content)
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleDeleteWorkspaceProgress(w http.ResponseWriter, r *http.Request) {
	mem, ok := s.memoryStore()
	if !ok {
		writeJSONError(w, http.StatusServiceUnavailable, "memory store unavailable")
		return
	}
	wsID := strings.TrimSpace(r.PathValue("wsID"))
	if _, status, err := s.requireWorkspace(wsID); err != nil {
		writeJSONError(w, status, err.Error())
		return
	}
	slug := strings.TrimSpace(r.PathValue("slug"))
	if err := mem.DeleteProgress(wsID, slug); err != nil {
		switch {
		case errors.Is(err, alwayson.ErrMemoryNotFound):
			writeJSONError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, store.ErrInvalidID):
			writeJSONError(w, http.StatusBadRequest, err.Error())
		default:
			writeJSONError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.notifyFallbackMemoryChanged()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"workspace_id": wsID,
		"deleted":      slug,
	})
}

// ============================================================================
// Candidate Skills Review & Promotion (/api/skills/candidates/*)
// ============================================================================

func (s *Server) handleListSkillCandidates(w http.ResponseWriter, _ *http.Request) {
	var (
		list []alwayson.CandidateSkill
		err  error
	)
	if s.alwaysOn != nil {
		list, err = s.alwaysOn.ListSkillCandidates()
	} else if s.cfg != nil {
		list, err = listSkillCandidatesFallback(s.cfg.Paths())
	} else {
		list = []alwayson.CandidateSkill{}
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []alwayson.CandidateSkill{}
	}
	for i := range list {
		list[i].Name = config.EscapeBidi(list[i].Name)
		list[i].Description = config.EscapeBidi(list[i].Description)
		list[i].Body = config.EscapeBidi(list[i].Body)
		list[i].Content = config.EscapeBidi(list[i].Content)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"candidates": list,
	})
}

func (s *Server) handlePromoteSkillCandidate(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.PathValue("slug"))
	var err error
	if s.alwaysOn != nil {
		err = s.alwaysOn.PromoteSkillCandidate(slug)
	} else if s.cfg != nil {
		err = promoteSkillCandidateFallback(s.cfg.Paths(), slug)
	} else {
		writeJSONError(w, http.StatusServiceUnavailable, "skill candidate store unavailable")
		return
	}
	if err != nil {
		switch {
		case errors.Is(err, alwayson.ErrCandidateNotFound):
			writeJSONError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, store.ErrInvalidID):
			writeJSONError(w, http.StatusBadRequest, err.Error())
		default:
			writeJSONError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	if s.cfg != nil {
		_, _ = s.cfg.Reload()
	}
	s.notifyFallbackMemoryChanged()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"promoted": slug,
	})
}

func (s *Server) handleDeleteSkillCandidate(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.PathValue("slug"))
	var err error
	if s.alwaysOn != nil {
		err = s.alwaysOn.DeleteSkillCandidate(slug)
	} else if s.cfg != nil {
		err = deleteSkillCandidateFallback(s.cfg.Paths(), slug)
	} else {
		writeJSONError(w, http.StatusServiceUnavailable, "skill candidate store unavailable")
		return
	}
	if err != nil {
		switch {
		case errors.Is(err, alwayson.ErrCandidateNotFound):
			writeJSONError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, store.ErrInvalidID):
			writeJSONError(w, http.StatusBadRequest, err.Error())
		default:
			writeJSONError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.notifyFallbackMemoryChanged()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"deleted": slug,
	})
}

func validateSafeSkillCandidateSlug(slug string) (string, error) {
	trimmed := strings.TrimSpace(slug)
	if err := store.ValidateSafeID(trimmed); err != nil {
		return "", err
	}
	if strings.Contains(trimmed, "..") || trimmed == "_candidates" {
		return "", fmt.Errorf("%w: %q", store.ErrInvalidID, slug)
	}
	return trimmed, nil
}

func listSkillCandidatesFallback(paths config.VaultPaths) ([]alwayson.CandidateSkill, error) {
	candRoot := filepath.Join(paths.SkillsDir(), "_candidates")
	lockPath := filepath.Join(paths.RuntimeDir(), "locks", "alwayson-candidates.lock")
	var out []alwayson.CandidateSkill
	err := store.WithFlock(lockPath, false, func() error {
		entries, rErr := os.ReadDir(candRoot)
		if rErr != nil {
			if errors.Is(rErr, os.ErrNotExist) {
				out = []alwayson.CandidateSkill{}
				return nil
			}
			return rErr
		}
		items := make([]alwayson.CandidateSkill, 0, len(entries))
		for _, ent := range entries {
			if !ent.IsDir() {
				continue
			}
			slug, vErr := validateSafeSkillCandidateSlug(ent.Name())
			if vErr != nil {
				continue
			}
			skillFile := filepath.Join(candRoot, slug, "SKILL.md")
			fi, sErr := os.Stat(skillFile)
			if sErr != nil || fi.IsDir() {
				continue
			}
			raw, fErr := os.ReadFile(skillFile)
			if fErr != nil {
				continue
			}
			items = append(items, parseFallbackCandidateSkill(raw, slug, skillFile, fi.ModTime().UTC()))
		}
		sort.Slice(items, func(i, j int) bool {
			if !items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
				return items[i].UpdatedAt.After(items[j].UpdatedAt)
			}
			return items[i].Slug < items[j].Slug
		})
		out = items
		return nil
	})
	return out, err
}

func parseFallbackCandidateSkill(raw []byte, slug, sourceFile string, fallbackTime time.Time) alwayson.CandidateSkill {
	content := string(raw)
	item := alwayson.CandidateSkill{
		Slug:       slug,
		Name:       slug,
		Body:       strings.TrimSpace(content),
		Content:    content,
		SourceFile: sourceFile,
		UpdatedAt:  fallbackTime,
	}
	type skillFM struct {
		Name        string   `toml:"name"`
		Description string   `toml:"description"`
		Paths       []string `toml:"paths"`
	}
	if fm, body, err := config.ParseFrontmatterAs[skillFM](content); err == nil {
		item.Body = strings.TrimSpace(body)
		if strings.TrimSpace(fm.Name) != "" {
			item.Name = strings.TrimSpace(fm.Name)
		}
		item.Description = strings.TrimSpace(fm.Description)
		item.Paths = fm.Paths
	}
	return item
}

func promoteSkillCandidateFallback(paths config.VaultPaths, slug string) error {
	safeSlug, err := validateSafeSkillCandidateSlug(slug)
	if err != nil {
		return err
	}
	candDir := filepath.Join(paths.SkillsDir(), "_candidates", safeSlug)
	candFile := filepath.Join(candDir, "SKILL.md")
	destFile := filepath.Join(paths.SkillsDir(), safeSlug, "SKILL.md")
	lockPath := filepath.Join(paths.RuntimeDir(), "locks", "alwayson-candidates.lock")

	return store.WithFlock(lockPath, true, func() error {
		raw, rErr := os.ReadFile(candFile)
		if rErr != nil {
			if errors.Is(rErr, os.ErrNotExist) {
				return fmt.Errorf("%w: %s", alwayson.ErrCandidateNotFound, safeSlug)
			}
			return rErr
		}
		if wErr := store.AtomicWriteFile(destFile, raw, 0o600); wErr != nil {
			return wErr
		}
		return os.RemoveAll(candDir)
	})
}

func deleteSkillCandidateFallback(paths config.VaultPaths, slug string) error {
	safeSlug, err := validateSafeSkillCandidateSlug(slug)
	if err != nil {
		return err
	}
	candDir := filepath.Join(paths.SkillsDir(), "_candidates", safeSlug)
	lockPath := filepath.Join(paths.RuntimeDir(), "locks", "alwayson-candidates.lock")

	return store.WithFlock(lockPath, true, func() error {
		if _, sErr := os.Stat(candDir); sErr != nil {
			if errors.Is(sErr, os.ErrNotExist) {
				return fmt.Errorf("%w: %s", alwayson.ErrCandidateNotFound, safeSlug)
			}
			return sErr
		}
		return os.RemoveAll(candDir)
	})
}
