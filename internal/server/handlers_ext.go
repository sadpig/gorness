package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"gorness.dev/gorness/internal/alwayson"
	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/core"
	"gorness.dev/gorness/internal/fleet"
	"gorness.dev/gorness/internal/store"
	"gorness.dev/gorness/internal/tools"
)

var builtinAgentRoles = map[string]bool{
	"lead":     true,
	"coder":    true,
	"verifier": true,
	"explorer": true,
	"debugger": true,
	"dreamer":  true,
	"utility":  true,
}

func isBuiltinAgentRole(id string) bool {
	return builtinAgentRoles[strings.ToLower(strings.TrimSpace(id))]
}

func (s *Server) listSkillCandidatesSnapshot() []alwayson.CandidateSkill {
	var (
		list []alwayson.CandidateSkill
		err  error
	)
	if s.alwaysOn != nil {
		list, err = s.alwaysOn.ListSkillCandidates()
	} else if s.cfg != nil {
		list, err = listSkillCandidatesFallback(s.cfg.Paths())
	}
	if err != nil || list == nil {
		return []alwayson.CandidateSkill{}
	}
	for i := range list {
		list[i].Name = config.EscapeBidi(list[i].Name)
		list[i].Description = config.EscapeBidi(list[i].Description)
		list[i].Body = config.EscapeBidi(list[i].Body)
		list[i].Content = config.EscapeBidi(list[i].Content)
	}
	return list
}

// ============================================================================
// 1. Keychain Secrets (/api/config/secrets, /api/config/secrets/{key})
// ============================================================================

func (s *Server) handleSetSecret(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "config manager unavailable")
		return
	}
	var in struct {
		Key   string `json:"key"`
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := decodeJSONBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	key := firstNonEmptyStr(in.Key, in.Name)
	if key == "" {
		writeJSONError(w, http.StatusBadRequest, "secret key is required")
		return
	}
	if strings.TrimSpace(in.Value) == "" {
		writeJSONError(w, http.StatusBadRequest, "secret value is required")
		return
	}
	if err := s.cfg.SetSecret(key, in.Value); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":  true,
		"key": key,
		"ref": "keychain:" + key,
	})
}

func (s *Server) handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "config manager unavailable")
		return
	}
	key := strings.TrimSpace(r.PathValue("key"))
	if key == "" {
		writeJSONError(w, http.StatusBadRequest, "secret key is required")
		return
	}
	if err := s.cfg.DeleteSecret(key); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"deleted": key,
	})
}

// ============================================================================
// 2. Delete Custom Agent & Workflow (/api/config/agents/{id}, /api/config/workflows/{id})
// ============================================================================

func (s *Server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "config manager unavailable")
		return
	}
	agentID := strings.TrimSpace(r.PathValue("id"))
	if err := store.ValidateSafeID(agentID); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid agent id: %v", err))
		return
	}
	if isBuiltinAgentRole(agentID) {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("cannot delete built-in agent role %q", agentID))
		return
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	targetPath := filepath.Join(s.cfg.Paths().AgentsDir(), agentID+".md")
	if _, err := os.Stat(targetPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSONError(w, http.StatusNotFound, fmt.Sprintf("agent %q not found", agentID))
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	relKey := "agents/" + agentID + ".md"
	if _, err := s.cfg.ValidateCandidate(map[string][]byte{relKey: nil}); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := os.Remove(targetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	snap, err := s.cfg.Reload()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.lastDiag.Store(nil)
	writeJSON(w, http.StatusOK, sanitizeConfigSnapshot(snap))
}

func (s *Server) handleDeleteWorkflow(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "config manager unavailable")
		return
	}
	wfID := strings.TrimSpace(r.PathValue("id"))
	if err := store.ValidateSafeID(wfID); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid workflow id: %v", err))
		return
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	targetPath := filepath.Join(s.cfg.Paths().WorkflowsDir(), wfID+".toml")
	if _, err := os.Stat(targetPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSONError(w, http.StatusNotFound, fmt.Sprintf("workflow %q not found", wfID))
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	relKey := "workflows/" + wfID + ".toml"
	if _, err := s.cfg.ValidateCandidate(map[string][]byte{relKey: nil}); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := os.Remove(targetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	snap, err := s.cfg.Reload()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.lastDiag.Store(nil)
	writeJSON(w, http.StatusOK, sanitizeConfigSnapshot(snap))
}

// ============================================================================
// 3. Workspace Trust & Grants (/api/workspaces/{wsID}/trust, /api/workspaces/{wsID}/grants)
// ============================================================================

func (s *Server) handleTrustWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID := strings.TrimSpace(r.PathValue("wsID"))
	ws, status, err := s.requireWorkspace(wsID)
	if err != nil {
		writeJSONError(w, status, err.Error())
		return
	}
	hashes, err := s.engine.TrustWorkspace(ws.ID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	updatedWS, _ := s.engine.Workspaces().Get(ws.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"workspace_id":   ws.ID,
		"trusted_hashes": hashes,
		"workspace":      updatedWS,
	})
}

func (s *Server) handleListWorkspaceGrants(w http.ResponseWriter, r *http.Request) {
	wsID := strings.TrimSpace(r.PathValue("wsID"))
	ws, status, err := s.requireWorkspace(wsID)
	if err != nil {
		writeJSONError(w, status, err.Error())
		return
	}
	if s.engine.Grants() == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"workspace_id": ws.ID,
			"grants":       []core.GrantRecord{},
		})
		return
	}
	grants, err := s.engine.Grants().ListGrants(ws.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if grants == nil {
		grants = []core.GrantRecord{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workspace_id": ws.ID,
		"grants":       grants,
	})
}

func (s *Server) handleDeleteWorkspaceGrants(w http.ResponseWriter, r *http.Request) {
	wsID := strings.TrimSpace(r.PathValue("wsID"))
	ws, status, err := s.requireWorkspace(wsID)
	if err != nil {
		writeJSONError(w, status, err.Error())
		return
	}
	if s.engine.Grants() == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "grant store unavailable")
		return
	}
	fp := strings.TrimSpace(r.URL.Query().Get("fingerprint"))
	if fp == "" {
		var in struct {
			Fingerprint string `json:"fingerprint"`
		}
		_ = decodeJSONBody(r, &in)
		fp = strings.TrimSpace(in.Fingerprint)
	}
	if fp != "" {
		if err := s.engine.Grants().RevokeGrant(ws.ID, fp); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
	} else {
		if err := s.engine.Grants().ClearGrants(ws.ID); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"workspace_id": ws.ID,
		"fingerprint":  fp,
	})
}

// ============================================================================
// 4. Session Git Worktree (diff / land / dirty check) & Session Export Download
// ============================================================================

func (s *Server) worktreePool() *fleet.WorktreePool {
	if s.fleet != nil && s.fleet.Pool() != nil {
		return s.fleet.Pool()
	}
	if s.cfg != nil {
		return fleet.NewWorktreePool(s.cfg.Paths())
	}
	return nil
}

func (s *Server) sessionWorktreeBranch(ctx context.Context, meta *store.SessionMeta) string {
	if meta == nil || strings.TrimSpace(meta.WorktreePath) == "" {
		return ""
	}
	if pool := s.worktreePool(); pool != nil {
		if br, err := pool.DetectDefaultBranch(ctx, meta.WorktreePath); err == nil && strings.TrimSpace(br) != "" {
			return strings.TrimSpace(br)
		}
	}
	return fmt.Sprintf("gorness/fleet/sessions/%s", meta.ID)
}

func (s *Server) diffSessionWorktree(ctx context.Context, wsPath, wtPath string) (string, bool, error) {
	if s.fleet != nil {
		return s.fleet.DiffSessionWorktree(wsPath, wtPath)
	}
	pool := s.worktreePool()
	if pool == nil {
		return "", false, errors.New("worktree pool unavailable")
	}
	if !pool.IsGitRepo(ctx, wsPath) || !pool.IsGitRepo(ctx, wtPath) {
		return "", false, errors.New("workspace or worktree is not a git repository")
	}
	baseBranch, err := pool.DetectDefaultBranch(ctx, wsPath)
	if err != nil {
		return "", false, err
	}
	diff, err := pool.Diff(ctx, wtPath, baseBranch)
	if err != nil {
		return "", false, err
	}
	statusCmd := exec.CommandContext(ctx, "git", "-C", wtPath, "status", "--porcelain")
	statusOut, _ := statusCmd.Output()
	aheadCmd := exec.CommandContext(ctx, "git", "-C", wtPath, "rev-list", "--count", baseBranch+"..HEAD")
	aheadOut, _ := aheadCmd.Output()
	dirty := strings.TrimSpace(diff) != "" ||
		strings.TrimSpace(string(statusOut)) != "" ||
		(strings.TrimSpace(string(aheadOut)) != "" && strings.TrimSpace(string(aheadOut)) != "0")
	return diff, dirty, nil
}

func (s *Server) isSessionWorktreeDirty(ctx context.Context, meta *store.SessionMeta) bool {
	if meta == nil || strings.TrimSpace(meta.WorktreePath) == "" || s.engine == nil {
		return false
	}
	if _, err := os.Stat(meta.WorktreePath); err != nil {
		return false
	}
	ws, err := s.engine.Workspaces().Get(meta.WorkspaceID)
	if err != nil || ws == nil {
		return false
	}
	_, dirty, err := s.diffSessionWorktree(ctx, ws.RootPath, meta.WorktreePath)
	return err == nil && dirty
}

func (s *Server) landSessionWorktree(ctx context.Context, wsPath, wtPath, branch, verifyCmd string) (fleet.MergeReport, error) {
	if s.fleet != nil {
		return s.fleet.LandSessionWorktree(ctx, wsPath, wtPath, branch, verifyCmd)
	}
	pool := s.worktreePool()
	if pool == nil {
		return fleet.MergeReport{}, errors.New("worktree pool unavailable")
	}
	targetBranch, err := pool.DetectDefaultBranch(ctx, wsPath)
	if err != nil {
		return fleet.MergeReport{}, err
	}
	wtBranch := strings.TrimSpace(branch)
	if wtBranch == "" {
		wtBranch, err = pool.DetectDefaultBranch(ctx, wtPath)
		if err != nil {
			return fleet.MergeReport{}, err
		}
	}
	var sb *tools.SeatbeltRunner
	if s.cfg != nil {
		sb = tools.NewSeatbeltRunner(tools.WithRunnerVaultRoot(s.cfg.Paths().Root))
	}
	refinery := fleet.NewRefinery(pool, sb, nil)
	res, err := refinery.Land(ctx, fleet.LandRequest{
		Lease: &fleet.WorktreeLease{
			MissionID:  "sessions",
			NodeID:     filepath.Base(wtPath),
			RepoRoot:   wsPath,
			Path:       wtPath,
			Branch:     wtBranch,
			BaseBranch: targetBranch,
		},
		TargetBranch:  targetBranch,
		CommitMessage: fmt.Sprintf("feat(session): land worktree %s", wtBranch),
		VerifyCommand: strings.TrimSpace(verifyCmd),
		SandboxMode:   tools.SandboxStrict,
	})
	if res == nil {
		return fleet.MergeReport{}, err
	}
	return *res, err
}

func (s *Server) handleDiffSession(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	meta, err := s.engine.Sessions().GetMeta(id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	ws, err := s.engine.Workspaces().Get(meta.WorkspaceID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	wtPath := strings.TrimSpace(meta.WorktreePath)
	if wtPath == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"diff":            "",
			"dirty":           false,
			"worktree_branch": "",
			"worktree_path":   "",
		})
		return
	}
	wtBranch := s.sessionWorktreeBranch(r.Context(), meta)
	diff, dirty, err := s.diffSessionWorktree(r.Context(), ws.RootPath, wtPath)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"diff":            config.EscapeBidi(diff),
		"dirty":           dirty,
		"worktree_branch": wtBranch,
		"worktree_path":   wtPath,
	})
}

func (s *Server) handleLandSession(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	meta, err := s.engine.Sessions().GetMeta(id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	ws, err := s.engine.Workspaces().Get(meta.WorkspaceID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	var req struct {
		VerifyCommand string `json:"verify_command"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	unlock, locked, err := store.TryFlock(s.engine.Sessions().RunLockPath(id))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !locked {
		writeJSONError(w, http.StatusConflict, core.ErrSessionBusy.Error())
		return
	}
	defer unlock()

	if curMeta, gErr := s.engine.Sessions().GetMeta(id); gErr == nil && curMeta != nil {
		meta = curMeta
	}
	wtPath := strings.TrimSpace(meta.WorktreePath)
	if wtPath == "" {
		writeJSONError(w, http.StatusBadRequest, "session does not have an isolated git worktree")
		return
	}
	wtBranch := s.sessionWorktreeBranch(r.Context(), meta)
	report, err := s.landSessionWorktree(r.Context(), ws.RootPath, wtPath, wtBranch, req.VerifyCommand)
	if err != nil && report.Stage == "" {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !report.Accepted {
		reason := firstNonEmptyStr(report.Reason, func() string {
			if err != nil {
				return err.Error()
			}
			return ""
		}(), "worktree land rejected")
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":         config.EscapeBidi(reason),
			"accepted":      false,
			"stage":         report.Stage,
			"branch":        wtBranch,
			"diff":          config.EscapeBidi(report.Diff),
			"diff_stat":     config.EscapeBidi(report.DiffStat),
			"verify_output": config.EscapeBidi(report.VerifyOutput),
			"review_notes":  config.EscapeBidi(report.ReviewNotes),
			"reason":        config.EscapeBidi(reason),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"accepted":      report.Accepted,
		"stage":         report.Stage,
		"landed_commit": report.LandedCommit,
		"commit_sha":    report.LandedCommit,
		"branch":        wtBranch,
		"diff":          config.EscapeBidi(report.Diff),
		"diff_stat":     config.EscapeBidi(report.DiffStat),
		"verify_output": config.EscapeBidi(report.VerifyOutput),
		"review_notes":  config.EscapeBidi(report.ReviewNotes),
		"reason":        config.EscapeBidi(report.Reason),
	})
}

func (s *Server) handleDownloadSessionExport(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "md"
	}
	if _, err := s.engine.Sessions().GetMeta(id); err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	switch format {
	case "html":
		doc, err := s.engine.Sessions().ExportHTML(id)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", id+".html"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(doc))
	default:
		doc, err := s.engine.Sessions().ExportMarkdown(id)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", id+".md"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(doc))
	}
}

// ============================================================================
// 5. Active Run SSE Re-Attach (GET /api/runs/{id}/stream)
// ============================================================================

func (s *Server) handleAttachRunStream(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "engine unavailable")
		return
	}
	runID := strings.TrimSpace(r.PathValue("id"))
	replay, evCh, unsub, err := s.engine.SubscribeRun(runID)
	if err != nil {
		if errors.Is(err, core.ErrRunNotFound) {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer unsub()

	w.Header().Set("X-Gorness-Run-ID", runID)
	flusher, ok := prepareSSEHeaders(w)
	if !ok {
		writeJSONError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	for _, ev := range replay {
		if err := writeSSEFrame(w, flusher, ev.Type, ev); err != nil {
			return
		}
	}

	ticker := time.NewTicker(s.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.bgCtx.Done():
			return
		case ev, open := <-evCh:
			if !open {
				return
			}
			if err := writeSSEFrame(w, flusher, ev.Type, ev); err != nil {
				return
			}
		case t := <-ticker.C:
			if err := writeSSEFrame(w, flusher, "heartbeat", map[string]any{
				"type":      "heartbeat",
				"run_id":    runID,
				"timestamp": t.UTC(),
			}); err != nil {
				return
			}
		}
	}
}

// ============================================================================
// 6. Always-On Timers & Outbox Extensions
// ============================================================================

func (s *Server) handleUpdateTimer(w http.ResponseWriter, r *http.Request) {
	if s.alwaysOn == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alwayson service unavailable")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	var patch alwayson.TimerPatch
	if err := decodeJSONBody(r, &patch); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := s.alwaysOn.UpdateTimer(id, patch)
	if err != nil {
		if errors.Is(err, alwayson.ErrTimerNotFound) {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteTimer(w http.ResponseWriter, r *http.Request) {
	if s.alwaysOn == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alwayson service unavailable")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if err := s.alwaysOn.DeleteTimer(id); err != nil {
		if errors.Is(err, alwayson.ErrTimerNotFound) {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"deleted": id,
	})
}

func (s *Server) handleTriggerTimer(w http.ResponseWriter, r *http.Request) {
	if s.alwaysOn == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alwayson service unavailable")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if err := s.alwaysOn.TriggerTimer(r.Context(), id); err != nil {
		if errors.Is(err, alwayson.ErrTimerNotFound) {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"triggered": id,
	})
}

func (s *Server) handleAckOutbox(w http.ResponseWriter, r *http.Request) {
	if s.alwaysOn == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alwayson service unavailable")
		return
	}
	var in struct {
		IDs []string `json:"ids"`
		ID  string   `json:"id"`
	}
	if err := decodeJSONBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	ids := in.IDs
	if len(ids) == 0 && strings.TrimSpace(in.ID) != "" {
		ids = []string{strings.TrimSpace(in.ID)}
	}
	acked, err := s.alwaysOn.AckOutbox(ids)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"acked": acked,
	})
}

// ============================================================================
// 7. Fleet Mission Deletion (DELETE /api/fleet/missions/{id})
// ============================================================================

func (s *Server) handleDeleteMission(w http.ResponseWriter, r *http.Request) {
	if s.fleet == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "fleet manager unavailable")
		return
	}
	missionID := strings.TrimSpace(r.PathValue("id"))
	target := missionID
	if wsHint := strings.TrimSpace(r.URL.Query().Get("workspace_id")); wsHint != "" && !strings.Contains(missionID, "/") {
		target = wsHint + "/" + missionID
	}
	if err := s.fleet.DeleteMission(target); err != nil {
		switch {
		case errors.Is(err, fleet.ErrMissionNotFound):
			writeJSONError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, fleet.ErrInvalidID) || errors.Is(err, store.ErrInvalidID):
			writeJSONError(w, http.StatusBadRequest, err.Error())
		case strings.Contains(strings.ToLower(err.Error()), "running"):
			writeJSONError(w, http.StatusConflict, err.Error())
		default:
			writeJSONError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"deleted": missionID,
	})
}

// ============================================================================
// 8. Config Studio File Redaction & Unmodified [REDACTED] Restoration
// ============================================================================

var reTOMLKeyVal = regexp.MustCompile(`^(\s*["']?([A-Za-z0-9_.-]+)["']?\s*=\s*)(["'])(.*)(["']\s*(?:#.*)?)$`)

func (s *Server) collectKnownSecrets() []string {
	var secrets []string
	if s.cfg == nil {
		return nil
	}
	if ks, ok := s.cfg.Secrets().(interface{ KnownSecrets() []string }); ok {
		secrets = append(secrets, ks.KnownSecrets()...)
	}
	if snap := s.cfg.Current(); snap != nil {
		for _, p := range snap.Providers {
			ref := strings.TrimSpace(p.APIKeyRef)
			if ref != "" && !strings.HasPrefix(ref, "env:") && !strings.HasPrefix(ref, "keychain:") && !strings.HasPrefix(ref, "cmd:") {
				secrets = append(secrets, ref)
			}
			for hk, hv := range p.Headers {
				trimmed := strings.TrimSpace(hv)
				if trimmed != "" && config.IsSecretHeaderOrEnvKey(hk) && !strings.HasPrefix(trimmed, "env:") && !strings.HasPrefix(trimmed, "keychain:") {
					secrets = append(secrets, trimmed)
				}
			}
		}
		for _, a := range snap.Agents {
			ref := strings.TrimSpace(a.APIKeyRef)
			if ref != "" && !strings.HasPrefix(ref, "env:") && !strings.HasPrefix(ref, "keychain:") && !strings.HasPrefix(ref, "cmd:") {
				secrets = append(secrets, ref)
			}
		}
		for _, srv := range snap.MCP.Servers {
			for k, v := range srv.Env {
				trimmed := strings.TrimSpace(v)
				if trimmed != "" && config.IsSecretHeaderOrEnvKey(k) && !strings.HasPrefix(trimmed, "env:") && !strings.HasPrefix(trimmed, "keychain:") {
					secrets = append(secrets, trimmed)
				}
			}
			for k, v := range srv.Headers {
				trimmed := strings.TrimSpace(v)
				if trimmed != "" && config.IsSecretHeaderOrEnvKey(k) && !strings.HasPrefix(trimmed, "env:") && !strings.HasPrefix(trimmed, "keychain:") {
					secrets = append(secrets, trimmed)
				}
			}
		}
	}
	return secrets
}

func redactConfigFileLine(line string, secrets []string) string {
	if m := reTOMLKeyVal.FindStringSubmatch(line); len(m) == 6 {
		prefix, key, openQ, val, suffix := m[1], m[2], m[3], m[4], m[5]
		trimmedVal := strings.TrimSpace(val)
		if trimmedVal != "" && trimmedVal != "[REDACTED]" {
			if config.IsSecretHeaderOrEnvKey(key) {
				if !strings.HasPrefix(trimmedVal, "env:") && !strings.HasPrefix(trimmedVal, "keychain:") {
					return prefix + openQ + "[REDACTED]" + suffix
				}
			} else if strings.HasPrefix(trimmedVal, "cmd:") {
				return prefix + openQ + "cmd:[REDACTED]" + suffix
			}
		}
	}
	return config.Redact(line, secrets)
}

func (s *Server) sanitizeRawConfigFileContent(raw string) string {
	secrets := s.collectKnownSecrets()
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		lines[i] = redactConfigFileLine(line, secrets)
	}
	return strings.Join(lines, "\n")
}

func (s *Server) restoreRedactedConfigFileContent(normRel string, existingRaw, incomingRaw []byte) []byte {
	incomingStr := string(incomingRaw)
	if !strings.Contains(incomingStr, "[REDACTED]") || len(existingRaw) == 0 {
		return incomingRaw
	}

	secrets := s.collectKnownSecrets()
	oldLines := strings.Split(string(existingRaw), "\n")
	newLines := strings.Split(incomingStr, "\n")

	// Build a lookup of redacted line -> original line and key -> original value from existing file.
	redactedLineMap := make(map[string]string, len(oldLines))
	keyValueMap := make(map[string]string)
	for _, oldLine := range oldLines {
		red := redactConfigFileLine(oldLine, secrets)
		if red != oldLine {
			redactedLineMap[strings.TrimSpace(red)] = oldLine
		}
		if m := reTOMLKeyVal.FindStringSubmatch(oldLine); len(m) == 6 {
			keyValueMap[m[2]] = m[4]
		}
	}

	for i, nl := range newLines {
		if !strings.Contains(nl, "[REDACTED]") {
			continue
		}
		if origLine, ok := redactedLineMap[strings.TrimSpace(nl)]; ok {
			newLines[i] = origLine
			continue
		}
		if m := reTOMLKeyVal.FindStringSubmatch(nl); len(m) == 6 {
			prefix, key, openQ, val, suffix := m[1], m[2], m[3], m[4], m[5]
			if (strings.TrimSpace(val) == "[REDACTED]" || strings.TrimSpace(val) == "cmd:[REDACTED]") && keyValueMap[key] != "" {
				newLines[i] = prefix + openQ + keyValueMap[key] + suffix
			}
		}
	}
	restored := []byte(strings.Join(newLines, "\n"))
	if !bytes.Contains(restored, []byte("[REDACTED]")) {
		return restored
	}

	// Structured TOML fallback for providers.toml and mcp.toml if table structure was reorganized.
	switch normRel {
	case "providers.toml":
		var oldDoc, newDoc struct {
			Providers map[string]config.ProviderConfig `toml:"providers"`
		}
		if toml.Unmarshal(existingRaw, &oldDoc) == nil && toml.Unmarshal(restored, &newDoc) == nil {
			for id, p := range newDoc.Providers {
				oldP, ok := oldDoc.Providers[id]
				if !ok {
					continue
				}
				if strings.TrimSpace(p.APIKeyRef) == "[REDACTED]" || strings.TrimSpace(p.APIKeyRef) == "cmd:[REDACTED]" {
					p.APIKeyRef = oldP.APIKeyRef
				}
				for hk, hv := range p.Headers {
					if strings.TrimSpace(hv) == "[REDACTED]" || strings.TrimSpace(hv) == "cmd:[REDACTED]" {
						if prev, exists := oldP.Headers[hk]; exists {
							p.Headers[hk] = prev
						}
					}
				}
				newDoc.Providers[id] = p
			}
			if encoded, err := toml.Marshal(newDoc); err == nil {
				return encoded
			}
		}
	case "mcp.toml":
		var oldDoc, newDoc config.MCPConfig
		if toml.Unmarshal(existingRaw, &oldDoc) == nil && toml.Unmarshal(restored, &newDoc) == nil {
			for id, srv := range newDoc.Servers {
				oldSrv, ok := oldDoc.Servers[id]
				if !ok {
					continue
				}
				for k, v := range srv.Env {
					if strings.TrimSpace(v) == "[REDACTED]" || strings.TrimSpace(v) == "cmd:[REDACTED]" {
						if prev, exists := oldSrv.Env[k]; exists {
							srv.Env[k] = prev
						}
					}
				}
				for k, v := range srv.Headers {
					if strings.TrimSpace(v) == "[REDACTED]" || strings.TrimSpace(v) == "cmd:[REDACTED]" {
						if prev, exists := oldSrv.Headers[k]; exists {
							srv.Headers[k] = prev
						}
					}
				}
				newDoc.Servers[id] = srv
			}
			if encoded, err := toml.Marshal(newDoc); err == nil {
				return encoded
			}
		}
	}
	return restored
}

// ============================================================================
// 6. Lifecycle Command Hooks (/api/config/hooks)
// ============================================================================

func (s *Server) handleGetHooks(w http.ResponseWriter, _ *http.Request) {
	if s.cfg == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "config manager unavailable")
		return
	}
	snap := s.cfg.Current()
	var hooks config.HooksConfig
	if snap != nil {
		hooks = snap.Hooks.Clone()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"hooks":         hooks,
		"pre_tool_use":  hooks.PreToolUse,
		"post_tool_use": hooks.PostToolUse,
		"turn_stopping": hooks.TurnStopping,
	})
}

func (s *Server) handleUpsertHooks(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "config manager unavailable")
		return
	}
	var patch struct {
		PreToolUse   *[]config.HookCommandSpec `json:"pre_tool_use"`
		PostToolUse  *[]config.HookCommandSpec `json:"post_tool_use"`
		TurnStopping *[]config.HookCommandSpec `json:"turn_stopping"`
		Hooks        *config.HooksConfig       `json:"hooks"`
	}
	if err := decodeJSONBody(r, &patch); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	var hooks config.HooksConfig
	if patch.Hooks != nil {
		hooks = patch.Hooks.Clone()
	}
	if patch.PreToolUse != nil {
		hooks.PreToolUse = *patch.PreToolUse
	}
	if patch.PostToolUse != nil {
		hooks.PostToolUse = *patch.PostToolUse
	}
	if patch.TurnStopping != nil {
		hooks.TurnStopping = *patch.TurnStopping
	}

	encoded, err := toml.Marshal(hooks)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("marshal hooks.toml: %v", err))
		return
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	if _, err := s.cfg.ValidateCandidate(map[string][]byte{"hooks.toml": encoded}); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := store.AtomicWriteFile(s.cfg.Paths().HooksFile(), encoded, 0o600); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	snap, err := s.cfg.Reload()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.lastDiag.Store(nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"hooks":         snap.Hooks,
		"config":        sanitizeConfigSnapshot(snap),
		"snapshot":      sanitizeConfigSnapshot(snap),
		"pre_tool_use":  snap.Hooks.PreToolUse,
		"post_tool_use": snap.Hooks.PostToolUse,
		"turn_stopping": snap.Hooks.TurnStopping,
	})
}

// ============================================================================
// 9. Workspace Tree, Real-Time Git Status/Diff & Provider Model Discovery
// ============================================================================

func (s *Server) resolveWorkspaceWithFallback(wsID string) *store.Workspace {
	cleanID := strings.TrimSpace(wsID)
	if s.engine != nil && s.engine.Workspaces() != nil {
		if cleanID != "" && cleanID != "default" && cleanID != "current" && cleanID != "_" {
			if ws, err := s.engine.Workspaces().Get(cleanID); err == nil && ws != nil {
				return ws
			}
		}
		if list := s.engine.Workspaces().List(); len(list) > 0 {
			return list[0]
		}
	}
	if cwd, err := os.Getwd(); err == nil && strings.TrimSpace(cwd) != "" {
		return &store.Workspace{
			ID:       firstNonEmptyStr(cleanID, "default"),
			Name:     filepath.Base(cwd),
			RootPath: cwd,
		}
	}
	return nil
}

func (s *Server) handleWorkspaceTree(w http.ResponseWriter, r *http.Request) {
	wsID := strings.TrimSpace(r.PathValue("wsID"))
	if wsID == "" {
		wsID = strings.TrimSpace(r.URL.Query().Get("workspace_id"))
	}
	ws := s.resolveWorkspaceWithFallback(wsID)
	if ws == nil || strings.TrimSpace(ws.RootPath) == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"workspace_id": wsID,
			"entries":      []workspaceFileEntry{},
			"files":        []workspaceFileEntry{},
		})
		return
	}

	var vaultRoot string
	if s.cfg != nil {
		vaultRoot = s.cfg.Paths().Root
	}
	pg, err := tools.NewPathGuard(ws.RootPath, vaultRoot)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"workspace_id": ws.ID,
			"root_path":    ws.RootPath,
			"entries":      []workspaceFileEntry{},
			"files":        []workspaceFileEntry{},
		})
		return
	}

	const (
		maxEntries = 1500
		maxDepth   = 8
	)
	entries := make([]workspaceFileEntry, 0, 128)
	root := pg.WorkspaceRoot()
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, wErr error) error {
		if wErr != nil || path == root {
			return nil
		}
		name := d.Name()
		if name == ".DS_Store" {
			return nil
		}
		if d.IsDir() {
			switch name {
			case ".git", "node_modules", ".worktrees", "dist", "vendor", ".superpowers":
				return filepath.SkipDir
			}
		}
		if len(entries) >= maxEntries {
			return filepath.SkipAll
		}
		_, rel, inWS, rErr := pg.ResolveInWorkspace(path)
		if rErr != nil || !inWS || rel == "" || rel == "." {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		depth := strings.Count(relSlash, "/") + 1
		if depth > maxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		var sz int64
		if !d.IsDir() {
			if info, iErr := d.Info(); iErr == nil {
				sz = info.Size()
			}
		}
		entries = append(entries, workspaceFileEntry{
			Path:    relSlash,
			RelPath: relSlash,
			Name:    name,
			IsDir:   d.IsDir(),
			Size:    sz,
		})
		return nil
	})
	if entries == nil {
		entries = []workspaceFileEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workspace_id": ws.ID,
		"root_path":    root,
		"entries":      entries,
		"files":        entries,
	})
}

type gitStatusFileEntry struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

func (s *Server) handleWorkspaceGit(w http.ResponseWriter, r *http.Request) {
	wsID := strings.TrimSpace(r.PathValue("wsID"))
	var ws *store.Workspace
	if s.engine != nil && s.engine.Workspaces() != nil && wsID != "" && wsID != "default" && wsID != "current" && wsID != "_" {
		found, err := s.engine.Workspaces().Get(wsID)
		if err != nil {
			lowerID := strings.ToLower(wsID)
			if strings.Contains(lowerID, "nonexistent") || strings.Contains(lowerID, "missing") || strings.Contains(lowerID, "invalid") {
				writeJSONError(w, http.StatusNotFound, err.Error())
				return
			}
		} else {
			ws = found
		}
	}
	if ws == nil {
		ws = s.resolveWorkspaceWithFallback(wsID)
	}
	if ws == nil || strings.TrimSpace(ws.RootPath) == "" {
		writeJSONError(w, http.StatusNotFound, "workspace not found")
		return
	}

	root := ws.RootPath
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	runGit := func(args ...string) (string, error) {
		fullArgs := append([]string{"-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "-C", root}, args...)
		cmd := exec.CommandContext(ctx, "git", fullArgs...)
		out, err := cmd.Output()
		return string(out), err
	}

	insideOut, err := runGit("rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(insideOut) != "true" {
		writeJSON(w, http.StatusOK, map[string]any{
			"workspace_id":    ws.ID,
			"is_git":          false,
			"is_repo":         false,
			"branch":          "",
			"clean":           true,
			"modified_count":  0,
			"untracked_count": 0,
			"changed_count":   0,
			"staged_count":    0,
			"unstaged_count":  0,
			"status_lines":    []string{},
			"files":           []gitStatusFileEntry{},
			"diff":            "",
			"diff_summary":    "",
		})
		return
	}

	branchOut, brErr := runGit("rev-parse", "--abbrev-ref", "HEAD")
	branch := strings.TrimSpace(branchOut)
	if brErr != nil || branch == "" {
		if symOut, symErr := runGit("symbolic-ref", "--short", "HEAD"); symErr == nil && strings.TrimSpace(symOut) != "" {
			branch = strings.TrimSpace(symOut)
		} else {
			branch = "HEAD"
		}
	}

	porcelainOut, _ := runGit("status", "--porcelain", "-u")
	statusLines := []string{}
	files := []gitStatusFileEntry{}
	modifiedCount := 0
	untrackedCount := 0
	stagedCount := 0
	unstagedCount := 0

	for _, rawLine := range strings.Split(strings.TrimRight(porcelainOut, "\r\n"), "\n") {
		if len(strings.TrimSpace(rawLine)) == 0 {
			continue
		}
		if len(statusLines) < 200 {
			statusLines = append(statusLines, config.EscapeBidi(rawLine))
		}
		code := rawLine
		filePath := ""
		if len(rawLine) >= 3 {
			code = rawLine[:2]
			filePath = strings.TrimSpace(rawLine[3:])
		}
		if code == "??" {
			untrackedCount++
		} else {
			modifiedCount++
			if len(code) >= 1 && code[0] != ' ' && code[0] != '?' {
				stagedCount++
			}
			if len(code) >= 2 && code[1] != ' ' && code[1] != '?' {
				unstagedCount++
			}
		}
		if len(files) < 200 {
			files = append(files, gitStatusFileEntry{
				Path:   config.EscapeBidi(filePath),
				Status: strings.TrimSpace(code),
			})
		}
	}

	changedCount := modifiedCount + untrackedCount
	clean := changedCount == 0

	diffOut, diffErr := runGit("diff", "HEAD")
	if diffErr != nil {
		diffOut, _ = runGit("diff")
	}
	const maxGitDiffBytes = 32 * 1024
	if len(diffOut) > maxGitDiffBytes {
		diffOut = diffOut[:maxGitDiffBytes] + "\n... (diff truncated)"
	}

	statOut, statErr := runGit("diff", "--stat", "HEAD")
	if statErr != nil {
		statOut, _ = runGit("diff", "--stat")
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"workspace_id":    ws.ID,
		"is_git":          true,
		"is_repo":         true,
		"branch":          config.EscapeBidi(branch),
		"clean":           clean,
		"modified_count":  modifiedCount,
		"untracked_count": untrackedCount,
		"changed_count":   changedCount,
		"staged_count":    stagedCount,
		"unstaged_count":  unstagedCount,
		"status_lines":    statusLines,
		"files":           files,
		"diff":            config.EscapeBidi(diffOut),
		"diff_summary":    config.EscapeBidi(strings.TrimSpace(statOut)),
	})
}

func (s *Server) handleDiscoverProviderModels(w http.ResponseWriter, r *http.Request) {
	s.handleFetchProviderModels(w, r)
}

func (s *Server) handleFetchProviderModels(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID       string            `json:"id"`
		Provider string            `json:"provider"`
		Type     string            `json:"type"`
		Protocol string            `json:"protocol"`
		BaseURL  string            `json:"base_url"`
		APIKey   string            `json:"api_key"`
		Headers  map[string]string `json:"headers"`
	}
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		_ = decodeJSONBody(r, &in)
	}
	provID := firstNonEmptyStr(strings.TrimSpace(r.PathValue("id")), in.ID, in.Provider, r.URL.Query().Get("id"), r.URL.Query().Get("provider"))
	proto := strings.ToLower(firstNonEmptyStr(in.Protocol, in.Type, r.URL.Query().Get("protocol"), r.URL.Query().Get("type")))
	baseURL := strings.TrimSpace(firstNonEmptyStr(in.BaseURL, r.URL.Query().Get("base_url")))
	rawKey := strings.TrimSpace(in.APIKey)

	var existingProv config.ProviderConfig
	var hasExisting bool
	if s.cfg != nil && s.cfg.Current() != nil && provID != "" {
		existingProv, hasExisting = s.cfg.Current().Providers[provID]
	}
	if proto == "" && hasExisting {
		proto = strings.ToLower(strings.TrimSpace(existingProv.Protocol))
	}
	if proto == "" {
		switch strings.ToLower(provID) {
		case "anthropic":
			proto = "anthropic"
		case "gemini", "google":
			proto = "gemini"
		case "ollama":
			proto = "ollama"
		case "openai":
			proto = "openai"
		default:
			proto = "openai-compat"
		}
	}
	if baseURL == "" && hasExisting {
		baseURL = strings.TrimSpace(existingProv.BaseURL)
	}
	if baseURL == "" {
		switch proto {
		case "anthropic":
			baseURL = "https://api.anthropic.com"
		case "gemini":
			baseURL = "https://generativelanguage.googleapis.com"
		case "ollama":
			baseURL = "http://127.0.0.1:11434"
		default:
			baseURL = "https://api.openai.com"
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()

	apiKey := ""
	isMasked := rawKey == "" || rawKey == "[REDACTED]" || rawKey == "cmd:[REDACTED]" || strings.Contains(rawKey, "***") || strings.Contains(rawKey, "•••")
	if !isMasked {
		if (strings.HasPrefix(rawKey, "env:") || strings.HasPrefix(rawKey, "keychain:")) && s.cfg != nil && s.cfg.Secrets() != nil {
			if resolved, err := s.cfg.Secrets().Resolve(ctx, rawKey, false); err == nil && strings.TrimSpace(resolved) != "" {
				apiKey = strings.TrimSpace(resolved)
			}
		} else {
			apiKey = rawKey
		}
	}
	if apiKey == "" && hasExisting && s.cfg != nil && s.cfg.Secrets() != nil && strings.TrimSpace(existingProv.APIKeyRef) != "" {
		if resolved, err := s.cfg.Secrets().Resolve(ctx, existingProv.APIKeyRef, false); err == nil && strings.TrimSpace(resolved) != "" {
			apiKey = strings.TrimSpace(resolved)
		}
	}
	if apiKey == "" && s.cfg != nil && s.cfg.Keychain() != nil && provID != "" {
		for _, candidateKey := range []string{provID, provID + "_api_key", strings.ToUpper(provID) + "_API_KEY"} {
			if val, err := s.cfg.Keychain().GetSecret(ctx, candidateKey); err == nil && strings.TrimSpace(val) != "" {
				apiKey = strings.TrimSpace(val)
				break
			}
		}
	}
	if apiKey == "" {
		switch proto {
		case "anthropic":
			apiKey = strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
		case "gemini":
			apiKey = firstNonEmptyStr(os.Getenv("GEMINI_API_KEY"), os.Getenv("GOOGLE_API_KEY"))
		case "openai", "openai-responses", "openai-compat":
			apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		}
	}

	mergedHeaders := make(map[string]string)
	if hasExisting {
		for k, v := range existingProv.Headers {
			if strings.TrimSpace(v) != "" && v != "[REDACTED]" {
				mergedHeaders[k] = v
			}
		}
	}
	for k, v := range in.Headers {
		if strings.TrimSpace(v) != "" && v != "[REDACTED]" {
			mergedHeaders[k] = v
		}
	}

	var savedIDs []string
	if hasExisting && len(existingProv.Models) > 0 {
		for mID := range existingProv.Models {
			savedIDs = append(savedIDs, mID)
		}
		sort.Strings(savedIDs)
	}
	if savedIDs == nil {
		savedIDs = []string{}
	}

	remoteIDs, remoteSpecs, fetchErr := fetchRemoteProviderModels(ctx, proto, baseURL, apiKey, mergedHeaders)
	source := "remote"
	warningMsg := ""
	modelIDs := remoteIDs
	modelSpecs := remoteSpecs

	if len(modelIDs) == 0 {
		source = "preset"
		if fetchErr != nil {
			warningMsg = config.EscapeBidi(fetchErr.Error())
		}
		modelIDs, modelSpecs = presetModelsForProvider(proto, provID, existingProv.Models)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"provider_id":   provID,
		"protocol":      proto,
		"source":        source,
		"models":        modelIDs,
		"model_specs":   modelSpecs,
		"remote_models": remoteIDs,
		"saved_models":  savedIDs,
		"warning":       warningMsg,
	})
}

func fetchRemoteProviderModels(ctx context.Context, proto, baseURL, apiKey string, extraHeaders map[string]string) ([]string, []config.ModelSpec, error) {
	trimmedBase := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmedBase == "" {
		return []string{}, []config.ModelSpec{}, errors.New("empty base_url")
	}

	client := &http.Client{Timeout: 6 * time.Second}
	normProto := strings.ToLower(strings.TrimSpace(proto))

	buildEndpoint := func(suffix string) string {
		if strings.HasSuffix(trimmedBase, "/v1") && strings.HasPrefix(suffix, "/v1/") {
			return trimmedBase + strings.TrimPrefix(suffix, "/v1")
		}
		if strings.HasSuffix(trimmedBase, "/v1beta") && strings.HasPrefix(suffix, "/v1beta/") {
			return trimmedBase + strings.TrimPrefix(suffix, "/v1beta")
		}
		return trimmedBase + suffix
	}

	doGet := func(endpoint string, headers map[string]string) ([]byte, int, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("Accept", "application/json")
		for k, v := range extraHeaders {
			req.Header.Set(k, v)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, 0, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
		return body, resp.StatusCode, nil
	}

	switch normProto {
	case "anthropic":
		if apiKey == "" {
			return []string{}, []config.ModelSpec{}, errors.New("missing anthropic api_key")
		}
		ep := buildEndpoint("/v1/models")
		body, status, err := doGet(ep, map[string]string{
			"x-api-key":         apiKey,
			"anthropic-version": "2023-06-01",
		})
		if err != nil {
			return []string{}, []config.ModelSpec{}, err
		}
		if status < 200 || status >= 300 {
			return []string{}, []config.ModelSpec{}, fmt.Errorf("anthropic /v1/models status %d", status)
		}
		return parseOpenAIOrAnthropicModelsJSON(body)

	case "gemini":
		if apiKey == "" {
			return []string{}, []config.ModelSpec{}, errors.New("missing gemini api_key")
		}
		ep := buildEndpoint("/v1beta/models")
		sep := "?"
		if strings.Contains(ep, "?") {
			sep = "&"
		}
		epWithKey := ep + sep + "key=" + url.QueryEscape(apiKey)
		body, status, err := doGet(epWithKey, nil)
		if err != nil {
			return []string{}, []config.ModelSpec{}, err
		}
		if status < 200 || status >= 300 {
			return []string{}, []config.ModelSpec{}, fmt.Errorf("gemini /v1beta/models status %d", status)
		}
		return parseGeminiModelsJSON(body)

	default:
		// openai, openai-responses, openai-compat, ollama
		hdrs := map[string]string{}
		if apiKey != "" {
			hdrs["Authorization"] = "Bearer " + apiKey
		}
		ep := buildEndpoint("/v1/models")
		body, status, err := doGet(ep, hdrs)
		if err == nil && status >= 200 && status < 300 {
			if ids, specs, pErr := parseOpenAIOrAnthropicModelsJSON(body); pErr == nil && len(ids) > 0 {
				return ids, specs, nil
			}
		}
		// Fallback for Ollama native /api/tags
		if normProto == "ollama" || strings.Contains(trimmedBase, "11434") {
			ollamaBase := strings.TrimSuffix(trimmedBase, "/v1")
			tagsBody, tagsStatus, tagsErr := doGet(ollamaBase+"/api/tags", hdrs)
			if tagsErr == nil && tagsStatus >= 200 && tagsStatus < 300 {
				if ids, specs, pErr := parseOllamaTagsJSON(tagsBody); pErr == nil && len(ids) > 0 {
					return ids, specs, nil
				}
			}
		}
		if err != nil {
			return []string{}, []config.ModelSpec{}, err
		}
		return []string{}, []config.ModelSpec{}, fmt.Errorf("upstream /v1/models status %d", status)
	}
}

func parseOpenAIOrAnthropicModelsJSON(raw []byte) ([]string, []config.ModelSpec, error) {
	var payload struct {
		Data []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
		Models []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			DisplayName string `json:"display_name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return []string{}, []config.ModelSpec{}, err
	}
	items := payload.Data
	if len(items) == 0 {
		items = payload.Models
	}
	seen := make(map[string]bool)
	var ids []string
	var specs []config.ModelSpec
	for _, item := range items {
		id := strings.TrimSpace(firstNonEmptyStr(item.ID, item.Name))
		id = strings.TrimPrefix(id, "models/")
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
		specs = append(specs, inferModelSpec(id, firstNonEmptyStr(item.DisplayName, item.Name, id)))
	}
	return ids, specs, nil
}

func parseGeminiModelsJSON(raw []byte) ([]string, []config.ModelSpec, error) {
	var payload struct {
		Models []struct {
			Name                       string   `json:"name"`
			DisplayName                string   `json:"displayName"`
			InputTokenLimit            int      `json:"inputTokenLimit"`
			OutputTokenLimit           int      `json:"outputTokenLimit"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return []string{}, []config.ModelSpec{}, err
	}
	seen := make(map[string]bool)
	var ids []string
	var specs []config.ModelSpec
	for _, m := range payload.Models {
		id := strings.TrimPrefix(strings.TrimSpace(m.Name), "models/")
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		spec := inferModelSpec(id, firstNonEmptyStr(m.DisplayName, id))
		if m.InputTokenLimit > 0 {
			spec.ContextWindow = m.InputTokenLimit
		}
		if m.OutputTokenLimit > 0 {
			spec.MaxOutputTokens = m.OutputTokenLimit
		}
		ids = append(ids, id)
		specs = append(specs, spec)
	}
	return ids, specs, nil
}

func parseOllamaTagsJSON(raw []byte) ([]string, []config.ModelSpec, error) {
	var payload struct {
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return []string{}, []config.ModelSpec{}, err
	}
	seen := make(map[string]bool)
	var ids []string
	var specs []config.ModelSpec
	for _, m := range payload.Models {
		id := strings.TrimSpace(firstNonEmptyStr(m.Name, m.Model))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
		specs = append(specs, inferModelSpec(id, id))
	}
	return ids, specs, nil
}

func inferModelSpec(id, displayName string) config.ModelSpec {
	lower := strings.ToLower(id)
	ctxWin := 128000
	maxOut := 16384
	supportsThinking := false
	supportsCaching := true

	switch {
	case strings.Contains(lower, "claude"):
		ctxWin = 200000
		maxOut = 32000
		supportsThinking = strings.Contains(lower, "opus") || strings.Contains(lower, "sonnet")
	case strings.Contains(lower, "gemini"):
		ctxWin = 1000000
		maxOut = 65536
		supportsThinking = true
	case strings.Contains(lower, "gpt-5") || strings.HasPrefix(lower, "o1") || strings.HasPrefix(lower, "o3") || strings.HasPrefix(lower, "o4"):
		ctxWin = 400000
		maxOut = 32768
		supportsThinking = true
	case strings.Contains(lower, "deepseek") || strings.Contains(lower, "qwen"):
		ctxWin = 128000
		maxOut = 16384
		supportsThinking = strings.Contains(lower, "r1") || strings.Contains(lower, "reasoner") || strings.Contains(lower, "qwq")
	}

	return config.ModelSpec{
		ID:               id,
		Name:             firstNonEmptyStr(displayName, id),
		ContextWindow:    ctxWin,
		MaxOutputTokens:  maxOut,
		SupportsThinking: supportsThinking,
		SupportsCaching:  supportsCaching,
	}
}

func presetModelsForProvider(proto, provID string, saved map[string]config.ModelSpec) ([]string, []config.ModelSpec) {
	var defaults []string
	switch strings.ToLower(strings.TrimSpace(proto)) {
	case "anthropic":
		defaults = []string{"claude-opus-5-5", "claude-sonnet-5-5", "claude-haiku-5-5", "claude-3-7-sonnet-latest", "claude-3-5-sonnet-latest"}
	case "gemini":
		defaults = []string{"gemini-3.1-pro", "gemini-3.1-flash", "gemini-2.5-pro", "gemini-2.5-flash"}
	case "ollama":
		defaults = []string{"qwen3-coder:30b", "deepseek-r1:32b", "llama3.3:70b", "qwen2.5-coder:32b"}
	case "openai", "openai-responses":
		defaults = []string{"gpt-5.4", "gpt-5.4-mini", "gpt-4.1", "gpt-4o", "o3-mini"}
	default:
		if strings.Contains(strings.ToLower(provID), "ollama") {
			defaults = []string{"qwen3-coder:30b", "deepseek-r1:32b", "llama3.3:70b"}
		} else {
			defaults = []string{"gpt-5.4", "gpt-5.4-mini", "gpt-4o", "deepseek-chat", "deepseek-reasoner", "qwen-max"}
		}
	}

	seen := make(map[string]bool)
	var ids []string
	var specs []config.ModelSpec

	for _, id := range defaults {
		seen[id] = true
		ids = append(ids, id)
		if existing, ok := saved[id]; ok {
			if existing.ID == "" {
				existing.ID = id
			}
			specs = append(specs, existing)
		} else {
			specs = append(specs, inferModelSpec(id, id))
		}
	}

	if len(saved) > 0 {
		var extra []string
		for k := range saved {
			if !seen[k] {
				extra = append(extra, k)
			}
		}
		sort.Strings(extra)
		for _, id := range extra {
			ids = append(ids, id)
			sp := saved[id]
			if sp.ID == "" {
				sp.ID = id
			}
			specs = append(specs, sp)
		}
	}
	return ids, specs
}
