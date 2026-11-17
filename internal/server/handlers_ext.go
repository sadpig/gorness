package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
