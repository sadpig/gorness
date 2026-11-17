package server_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/fleet"
	"gorness.dev/gorness/internal/llm"
)

func doJSONReq(t *testing.T, client *http.Client, baseURL, method, path string, body any) (int, []byte, http.Header) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body for %s %s: %v", method, path, err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, baseURL+path, reader)
	if err != nil {
		t.Fatalf("new request %s %s: %v", method, path, err)
	}
	req.Header.Set("Origin", baseURL)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out, resp.Header
}

func TestRemovedAliasRoutesAndPreWriteValidationAndRedaction(t *testing.T) {
	noopStreamer := llm.StreamerFunc(func(_ context.Context, _ llm.Request) (<-chan llm.StreamEvent, error) {
		ch := make(chan llm.StreamEvent, 2)
		ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "ok"}
		ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn"}
		close(ch)
		return ch, nil
	})

	srv, vaultDir, cleanup := newTestStack(t, noopStreamer, nil)
	defer cleanup()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	client := ts.Client()

	// 1. Verify all 9 removed duplicate route aliases return 404 or 405.
	aliasChecks := []struct {
		method string
		path   string
		want   []int
	}{
		{http.MethodPost, "/api/config/global", []int{http.StatusMethodNotAllowed, http.StatusNotFound}},
		{http.MethodPost, "/api/config/rules", []int{http.StatusMethodNotAllowed, http.StatusNotFound}},
		{http.MethodPut, "/api/config/mcp", []int{http.StatusMethodNotAllowed, http.StatusNotFound}},
		{http.MethodPut, "/api/workspaces/ws1", []int{http.StatusMethodNotAllowed, http.StatusNotFound}},
		{http.MethodPut, "/api/sessions/sess1", []int{http.StatusMethodNotAllowed, http.StatusNotFound}},
		{http.MethodGet, "/api/alwayson/state", []int{http.StatusNotFound, http.StatusMethodNotAllowed}},
		{http.MethodGet, "/api/fleet/missions/m1/diff", []int{http.StatusNotFound, http.StatusMethodNotAllowed}},
		{http.MethodPost, "/api/fleet/missions/m1/steer", []int{http.StatusNotFound, http.StatusMethodNotAllowed}},
	}
	for _, tc := range aliasChecks {
		st, raw, _ := doJSONReq(t, client, ts.URL, tc.method, tc.path, map[string]any{})
		matched := false
		for _, code := range tc.want {
			if st == code {
				matched = true
				break
			}
		}
		if !matched {
			t.Fatalf("expected %s %s to be rejected with %v, got %d: %s", tc.method, tc.path, tc.want, st, string(raw))
		}
	}

	// 2. Verify PUT /api/config/global saves ui_theme & ui_lang and pre-write ValidateCandidate rejects invalid config.
	stGlob, rawGlob, _ := doJSONReq(t, client, ts.URL, http.MethodPut, "/api/config/global", map[string]any{
		"ui_theme": "dark",
		"ui_lang":  "zh",
	})
	if stGlob != http.StatusOK {
		t.Fatalf("PUT /api/config/global status=%d: %s", stGlob, string(rawGlob))
	}
	var snapGlob config.Snapshot
	if err := json.Unmarshal(rawGlob, &snapGlob); err != nil {
		t.Fatalf("decode global snapshot: %v", err)
	}
	if snapGlob.Global.UITheme != "dark" || snapGlob.Global.UILang != "zh" {
		t.Fatalf("expected ui_theme=dark ui_lang=zh, got %+v", snapGlob.Global)
	}

	stBadGlob, _, _ := doJSONReq(t, client, ts.URL, http.MethodPut, "/api/config/global", map[string]any{
		"default_provider": "nonexistent_provider_xyz",
	})
	if stBadGlob != http.StatusBadRequest {
		t.Fatalf("expected 400 when setting nonexistent default_provider, got %d", stBadGlob)
	}

	// 3. Keychain secrets endpoints: POST /api/config/secrets & DELETE /api/config/secrets/{key}
	stSecBad, _, _ := doJSONReq(t, client, ts.URL, http.MethodPost, "/api/config/secrets", map[string]any{
		"key": "",
	})
	if stSecBad != http.StatusBadRequest {
		t.Fatalf("expected 400 on empty secret key, got %d", stSecBad)
	}

	// 4. Custom Agent & Workflow upsert + delete (and built-in agent guard).
	stBuiltInDel, _, _ := doJSONReq(t, client, ts.URL, http.MethodDelete, "/api/config/agents/coder", nil)
	if stBuiltInDel != http.StatusBadRequest {
		t.Fatalf("expected 400 deleting built-in agent role coder, got %d", stBuiltInDel)
	}

	coderAgent := snapGlob.Agents["coder"]
	stUpAgent, rawUpAgent, _ := doJSONReq(t, client, ts.URL, http.MethodPost, "/api/config/agents", map[string]any{
		"id":            "auditor",
		"name":          "Security Auditor",
		"provider":      coderAgent.Provider,
		"model":         coderAgent.Model,
		"system_prompt": "Audit code carefully.",
	})
	if stUpAgent != http.StatusOK {
		t.Fatalf("POST /api/config/agents status=%d: %s", stUpAgent, string(rawUpAgent))
	}
	stDelAgent, rawDelAgent, _ := doJSONReq(t, client, ts.URL, http.MethodDelete, "/api/config/agents/auditor", nil)
	if stDelAgent != http.StatusOK {
		t.Fatalf("DELETE /api/config/agents/auditor status=%d: %s", stDelAgent, string(rawDelAgent))
	}

	baseWF := snapGlob.Workflows["hybrid-fleet"]
	stUpWF, rawUpWF, _ := doJSONReq(t, client, ts.URL, http.MethodPost, "/api/config/workflows", map[string]any{
		"id":          "sec_review",
		"name":        "Security Review",
		"description": "Run security review",
		"topology":    baseWF.Topology,
		"nodes":       baseWF.Nodes,
	})
	if stUpWF != http.StatusOK {
		t.Fatalf("POST /api/config/workflows status=%d: %s", stUpWF, string(rawUpWF))
	}
	stDelWF, rawDelWF, _ := doJSONReq(t, client, ts.URL, http.MethodDelete, "/api/config/workflows/sec_review", nil)
	if stDelWF != http.StatusOK {
		t.Fatalf("DELETE /api/config/workflows/sec_review status=%d: %s", stDelWF, string(rawDelWF))
	}

	// 5. MCP Env & Headers sanitization + Config files redaction and [REDACTED] preservation.
	stMCP, rawMCP, _ := doJSONReq(t, client, ts.URL, http.MethodPost, "/api/config/mcp", map[string]any{
		"id":        "sec_mcp",
		"transport": "stdio",
		"command":   "npx",
		"enabled":   true,
		"env": map[string]string{
			"GITHUB_API_TOKEN": "ghp_super_secret_value_123",
			"NORMAL_FLAG":      "1",
		},
		"headers": map[string]string{
			"Authorization": "Bearer topsecret_jwt_999",
		},
	})
	if stMCP != http.StatusOK {
		t.Fatalf("POST /api/config/mcp status=%d: %s", stMCP, string(rawMCP))
	}
	var snapMCP config.Snapshot
	if err := json.Unmarshal(rawMCP, &snapMCP); err != nil {
		t.Fatalf("decode mcp snapshot: %v", err)
	}
	srvMCP := snapMCP.MCP.Servers["sec_mcp"]
	if srvMCP.Env["GITHUB_API_TOKEN"] != "[REDACTED]" {
		t.Fatalf("expected GITHUB_API_TOKEN to be [REDACTED] in snapshot, got %q", srvMCP.Env["GITHUB_API_TOKEN"])
	}
	if srvMCP.Headers["Authorization"] != "[REDACTED]" {
		t.Fatalf("expected Authorization header to be [REDACTED] in snapshot, got %q", srvMCP.Headers["Authorization"])
	}

	// Verify GET /api/config/files?path=mcp.toml redacts raw secret values in mcp.toml.
	stFiles, rawFiles, _ := doJSONReq(t, client, ts.URL, http.MethodGet, "/api/config/files?path=mcp.toml", nil)
	if stFiles != http.StatusOK {
		t.Fatalf("GET /api/config/files?path=mcp.toml status=%d: %s", stFiles, string(rawFiles))
	}
	var fileResp struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(rawFiles, &fileResp); err != nil {
		t.Fatalf("decode file response: %v", err)
	}
	mcpRedactedContent := fileResp.Content
	if strings.Contains(mcpRedactedContent, "ghp_super_secret_value_123") || !strings.Contains(mcpRedactedContent, "[REDACTED]") {
		t.Fatalf("expected mcp.toml in GET /api/config/files to redact secret, got:\n%s", mcpRedactedContent)
	}

	// Save back mcp.toml with [REDACTED] intact and verify original secret is preserved on disk.
	updatedMCPContent := strings.Replace(mcpRedactedContent, "NORMAL_FLAG = '1'", "NORMAL_FLAG = '2'", 1)
	updatedMCPContent = strings.Replace(updatedMCPContent, `NORMAL_FLAG = "1"`, `NORMAL_FLAG = "2"`, 1)
	stSaveFile, rawSaveFile, _ := doJSONReq(t, client, ts.URL, http.MethodPut, "/api/config/files", map[string]any{
		"path":    "mcp.toml",
		"content": updatedMCPContent,
	})
	if stSaveFile != http.StatusOK {
		t.Fatalf("PUT /api/config/files status=%d: %s", stSaveFile, string(rawSaveFile))
	}
	diskMCP, err := os.ReadFile(filepath.Join(vaultDir, "mcp.toml"))
	if err != nil {
		t.Fatalf("read disk mcp.toml: %v", err)
	}
	if !strings.Contains(string(diskMCP), "ghp_super_secret_value_123") {
		t.Fatalf("expected original secret to be preserved on disk when saving [REDACTED], got:\n%s", string(diskMCP))
	}
}

func TestSessionWorktreeDiffLandDirtyGuardAndExportSplit(t *testing.T) {
	noopStreamer := llm.StreamerFunc(func(_ context.Context, _ llm.Request) (<-chan llm.StreamEvent, error) {
		ch := make(chan llm.StreamEvent, 2)
		ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "assistant reply"}
		ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn"}
		close(ch)
		return ch, nil
	})

	srv, vaultDir, cleanup := newTestStack(t, noopStreamer, nil)
	defer cleanup()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	client := ts.Client()

	// Create two git repo workspaces.
	repoA := filepath.Join(t.TempDir(), "repo-a")
	repoB := filepath.Join(t.TempDir(), "repo-b")
	initTempGitRepo(t, repoA)
	initTempGitRepo(t, repoB)

	stWSA, rawWSA, _ := doJSONReq(t, client, ts.URL, http.MethodPost, "/api/workspaces", map[string]any{
		"name": "Repo A",
		"path": repoA,
	})
	if stWSA != http.StatusCreated && stWSA != http.StatusOK {
		t.Fatalf("create wsA status=%d: %s", stWSA, string(rawWSA))
	}
	var wsA struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rawWSA, &wsA)

	stWSB, rawWSB, _ := doJSONReq(t, client, ts.URL, http.MethodPost, "/api/workspaces", map[string]any{
		"name": "Repo B",
		"path": repoB,
	})
	if stWSB != http.StatusCreated && stWSB != http.StatusOK {
		t.Fatalf("create wsB status=%d: %s", stWSB, string(rawWSB))
	}
	var wsB struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rawWSB, &wsB)

	// Workspace trust & grants endpoints.
	stTrust, rawTrust, _ := doJSONReq(t, client, ts.URL, http.MethodPost, "/api/workspaces/"+wsA.ID+"/trust", nil)
	if stTrust != http.StatusOK {
		t.Fatalf("POST /api/workspaces/%s/trust status=%d: %s", wsA.ID, stTrust, string(rawTrust))
	}
	stGrants, rawGrants, _ := doJSONReq(t, client, ts.URL, http.MethodGet, "/api/workspaces/"+wsA.ID+"/grants", nil)
	if stGrants != http.StatusOK {
		t.Fatalf("GET /api/workspaces/%s/grants status=%d: %s", wsA.ID, stGrants, string(rawGrants))
	}
	stDelGrants, rawDelGrants, _ := doJSONReq(t, client, ts.URL, http.MethodDelete, "/api/workspaces/"+wsA.ID+"/grants", nil)
	if stDelGrants != http.StatusOK {
		t.Fatalf("DELETE /api/workspaces/%s/grants status=%d: %s", wsA.ID, stDelGrants, string(rawDelGrants))
	}

	// Create session with isolate_git=true.
	stSess, rawSess, _ := doJSONReq(t, client, ts.URL, http.MethodPost, "/api/sessions", map[string]any{
		"workspace_id": wsA.ID,
		"title":        "Worktree Feature Session",
		"isolate_git":  true,
	})
	if stSess != http.StatusCreated {
		t.Fatalf("POST /api/sessions status=%d: %s", stSess, string(rawSess))
	}
	var sess struct {
		ID           string `json:"id"`
		WorktreePath string `json:"worktree_path"`
	}
	_ = json.Unmarshal(rawSess, &sess)
	if sess.WorktreePath == "" {
		t.Fatalf("expected isolated worktree path for session, got empty")
	}

	// Fork session and verify child inherits isolated worktree.
	stFork, rawFork, _ := doJSONReq(t, client, ts.URL, http.MethodPost, "/api/sessions/"+sess.ID+"/fork", map[string]any{
		"title": "Forked Worktree Session",
	})
	if stFork != http.StatusCreated {
		t.Fatalf("POST /api/sessions/%s/fork status=%d: %s", sess.ID, stFork, string(rawFork))
	}
	var forked struct {
		ID           string `json:"id"`
		WorktreePath string `json:"worktree_path"`
	}
	_ = json.Unmarshal(rawFork, &forked)
	if forked.WorktreePath == "" || forked.WorktreePath == sess.WorktreePath {
		t.Fatalf("expected forked session to have its own worktree path, got %q (parent=%q)", forked.WorktreePath, sess.WorktreePath)
	}

	// Modify a file in parent session worktree so it becomes dirty.
	if err := os.WriteFile(filepath.Join(sess.WorktreePath, "app.go"), []byte("package app\n\nfunc Value() int { return 42 }\n"), 0o644); err != nil {
		t.Fatalf("write dirty file in worktree: %v", err)
	}

	// GET /api/sessions/{id}/diff should report dirty=true and non-empty diff.
	stDiff, rawDiff, _ := doJSONReq(t, client, ts.URL, http.MethodGet, "/api/sessions/"+sess.ID+"/diff", nil)
	if stDiff != http.StatusOK {
		t.Fatalf("GET /api/sessions/%s/diff status=%d: %s", sess.ID, stDiff, string(rawDiff))
	}
	var diffOut struct {
		Diff           string `json:"diff"`
		Dirty          bool   `json:"dirty"`
		WorktreeBranch string `json:"worktree_branch"`
		WorktreePath   string `json:"worktree_path"`
	}
	if err := json.Unmarshal(rawDiff, &diffOut); err != nil {
		t.Fatalf("decode diff response: %v", err)
	}
	if !diffOut.Dirty || !strings.Contains(diffOut.Diff, "return 42") {
		t.Fatalf("expected dirty=true and diff containing 'return 42', got %+v", diffOut)
	}

	// DELETE /api/sessions/{id} and POST /api/sessions/{id}/move without force=true must return 409 Conflict.
	stDelDirty, _, _ := doJSONReq(t, client, ts.URL, http.MethodDelete, "/api/sessions/"+sess.ID, nil)
	if stDelDirty != http.StatusConflict {
		t.Fatalf("expected 409 Conflict when deleting dirty worktree session without force, got %d", stDelDirty)
	}
	stMoveDirty, _, _ := doJSONReq(t, client, ts.URL, http.MethodPost, "/api/sessions/"+sess.ID+"/move", map[string]any{
		"target_workspace_id": wsB.ID,
	})
	if stMoveDirty != http.StatusConflict {
		t.Fatalf("expected 409 Conflict when moving dirty worktree session without force, got %d", stMoveDirty)
	}

	// Commit changes in session worktree and land onto workspace main branch via POST /api/sessions/{id}/land.
	for _, args := range [][]string{
		{"-C", sess.WorktreePath, "add", "-A"},
		{"-C", sess.WorktreePath, "commit", "-m", "feat: return 42"},
	} {
		cmd := exec.Command("git", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, string(out))
		}
	}
	stLand, rawLand, _ := doJSONReq(t, client, ts.URL, http.MethodPost, "/api/sessions/"+sess.ID+"/land", map[string]any{})
	if stLand != http.StatusOK {
		t.Fatalf("POST /api/sessions/%s/land status=%d: %s", sess.ID, stLand, string(rawLand))
	}
	mainAppGo, err := os.ReadFile(filepath.Join(repoA, "app.go"))
	if err != nil {
		t.Fatalf("read repoA app.go after land: %v", err)
	}
	if !strings.Contains(string(mainAppGo), "return 42") {
		t.Fatalf("expected landed changes in repoA/app.go, got:\n%s", string(mainAppGo))
	}

	// Verify split export: GET /api/sessions/{id}/export returns attachment download; POST saves to Artifacts & PreviewStore.
	stGetExp, rawGetExp, hdrGetExp := doJSONReq(t, client, ts.URL, http.MethodGet, "/api/sessions/"+sess.ID+"/export?format=md", nil)
	if stGetExp != http.StatusOK {
		t.Fatalf("GET /api/sessions/%s/export status=%d: %s", sess.ID, stGetExp, string(rawGetExp))
	}
	if !strings.Contains(hdrGetExp.Get("Content-Disposition"), "attachment") {
		t.Fatalf("expected Content-Disposition: attachment on GET export, got %q", hdrGetExp.Get("Content-Disposition"))
	}

	stPostExp, rawPostExp, _ := doJSONReq(t, client, ts.URL, http.MethodPost, "/api/sessions/"+sess.ID+"/export?format=html", map[string]any{
		"format": "html",
		"title":  "Exported HTML Deliverable",
	})
	if stPostExp != http.StatusOK {
		t.Fatalf("POST /api/sessions/%s/export status=%d: %s", sess.ID, stPostExp, string(rawPostExp))
	}
	var postExpOut struct {
		ArtifactPath string `json:"artifact_path"`
		PreviewToken string `json:"preview_token"`
	}
	_ = json.Unmarshal(rawPostExp, &postExpOut)
	if postExpOut.ArtifactPath == "" || postExpOut.PreviewToken == "" {
		t.Fatalf("expected artifact_path and preview_token from POST export, got %+v", postExpOut)
	}

	// Verify GET /api/workspaces/{wsID}/artifacts merges Artifacts().List(wsID).
	stArts, rawArts, _ := doJSONReq(t, client, ts.URL, http.MethodGet, "/api/workspaces/"+wsA.ID+"/artifacts", nil)
	if stArts != http.StatusOK {
		t.Fatalf("GET /api/workspaces/%s/artifacts status=%d: %s", wsA.ID, stArts, string(rawArts))
	}
	if !strings.Contains(string(rawArts), "Exported HTML Deliverable") {
		t.Fatalf("expected exported artifact in workspace artifacts list, got: %s", string(rawArts))
	}

	// Verify force delete on forked session cleans up cleanly.
	stDelFork, _, _ := doJSONReq(t, client, ts.URL, http.MethodDelete, "/api/sessions/"+forked.ID+"?force=true", nil)
	if stDelFork != http.StatusOK {
		t.Fatalf("DELETE forked session with force=true status=%d", stDelFork)
	}

	// Zero-DB invariant check.
	_ = filepath.WalkDir(vaultDir, func(path string, d os.DirEntry, wErr error) error {
		if wErr == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(d.Name()), ".json") {
			t.Fatalf("Zero-DB violation: .json file in vault: %s", path)
		}
		return nil
	})
}

func TestMemoryFourLayersSkillCandidatesTimersOutboxFleetDeleteAndRunStream(t *testing.T) {
	runHold := make(chan struct{})
	slowStreamer := llm.StreamerFunc(func(ctx context.Context, _ llm.Request) (<-chan llm.StreamEvent, error) {
		ch := make(chan llm.StreamEvent, 4)
		go func() {
			defer close(ch)
			ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "chunk-1 "}
			select {
			case <-runHold:
			case <-ctx.Done():
				return
			}
			ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "chunk-2"}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn"}
		}()
		return ch, nil
	})

	srv, vaultDir, cleanup := newTestStack(t, slowStreamer, nil)
	defer cleanup()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	client := ts.Client()

	repoDir := filepath.Join(t.TempDir(), "mem-repo")
	initTempGitRepo(t, repoDir)
	stWS, rawWS, _ := doJSONReq(t, client, ts.URL, http.MethodPost, "/api/workspaces", map[string]any{
		"name": "Memory Repo",
		"path": repoDir,
	})
	if stWS != http.StatusCreated && stWS != http.StatusOK {
		t.Fatalf("create workspace status=%d: %s", stWS, string(rawWS))
	}
	var ws struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rawWS, &ws)

	// 1. 4-layer memory read & write (`GET /api/memory`, `PUT /api/memory/layer`).
	stPutMem, rawPutMem, _ := doJSONReq(t, client, ts.URL, http.MethodPut, "/api/memory/layer", map[string]any{
		"layer":   "instructions",
		"content": "# User Memory\nPrefer concise Go tests.\n",
	})
	if stPutMem != http.StatusOK {
		t.Fatalf("PUT /api/memory/layer status=%d: %s", stPutMem, string(rawPutMem))
	}
	stGetMem, rawGetMem, _ := doJSONReq(t, client, ts.URL, http.MethodGet, "/api/memory?workspace_id="+ws.ID+"&agent_role=coder", nil)
	if stGetMem != http.StatusOK || !strings.Contains(string(rawGetMem), "Prefer concise Go tests.") {
		t.Fatalf("GET /api/memory status=%d: %s", stGetMem, string(rawGetMem))
	}

	// 2. Candidate skills list, promote, delete + bootstrap/alwayson skill_candidates.
	candDir := filepath.Join(vaultDir, "skills", "_candidates", "go-table-tests")
	if err := os.MkdirAll(candDir, 0o700); err != nil {
		t.Fatalf("mkdir candidate skill: %v", err)
	}
	candContent := "+++\nname = \"go-table-tests\"\ndescription = \"Write table-driven tests\"\n+++\n# Instructions\nUse table-driven tests.\n"
	if err := os.WriteFile(filepath.Join(candDir, "SKILL.md"), []byte(candContent), 0o600); err != nil {
		t.Fatalf("write candidate SKILL.md: %v", err)
	}

	stBoot, rawBoot, _ := doJSONReq(t, client, ts.URL, http.MethodGet, "/api/bootstrap", nil)
	if stBoot != http.StatusOK || !strings.Contains(string(rawBoot), "go-table-tests") {
		t.Fatalf("expected go-table-tests in /api/bootstrap skill_candidates, got (%d): %s", stBoot, string(rawBoot))
	}

	stAO, rawAO, _ := doJSONReq(t, client, ts.URL, http.MethodGet, "/api/alwayson", nil)
	if stAO != http.StatusOK || !strings.Contains(string(rawAO), "go-table-tests") {
		t.Fatalf("expected go-table-tests in /api/alwayson skill_candidates, got (%d): %s", stAO, string(rawAO))
	}

	stGetCand, rawGetCand, _ := doJSONReq(t, client, ts.URL, http.MethodGet, "/api/skills/candidates", nil)
	if stGetCand != http.StatusOK || !strings.Contains(string(rawGetCand), "Write table-driven tests") {
		t.Fatalf("GET /api/skills/candidates status=%d: %s", stGetCand, string(rawGetCand))
	}

	stPromCand, rawPromCand, _ := doJSONReq(t, client, ts.URL, http.MethodPost, "/api/skills/candidates/go-table-tests/promote", map[string]any{})
	if stPromCand != http.StatusOK {
		t.Fatalf("POST /api/skills/candidates/go-table-tests/promote status=%d: %s", stPromCand, string(rawPromCand))
	}

	// 3. Always-On Timers: create, PATCH, trigger, DELETE, and outbox/ack.
	stCreateTimer, rawCreateTimer, _ := doJSONReq(t, client, ts.URL, http.MethodPost, "/api/alwayson/timers", map[string]any{
		"id":           "daily_audit",
		"workspace_id": ws.ID,
		"kind":         "cron",
		"cron_expr":    "0 9 * * *",
		"note":         "Morning check",
		"enabled":      true,
	})
	if stCreateTimer != http.StatusCreated {
		t.Fatalf("POST /api/alwayson/timers status=%d: %s", stCreateTimer, string(rawCreateTimer))
	}

	stPatchTimer, rawPatchTimer, _ := doJSONReq(t, client, ts.URL, http.MethodPatch, "/api/alwayson/timers/daily_audit", map[string]any{
		"note":    "Updated morning check",
		"enabled": false,
	})
	if stPatchTimer != http.StatusOK || !strings.Contains(string(rawPatchTimer), "Updated morning check") {
		t.Fatalf("PATCH /api/alwayson/timers/daily_audit status=%d: %s", stPatchTimer, string(rawPatchTimer))
	}

	stTrigTimer, rawTrigTimer, _ := doJSONReq(t, client, ts.URL, http.MethodPost, "/api/alwayson/timers/daily_audit/trigger", map[string]any{})
	if stTrigTimer != http.StatusOK {
		t.Fatalf("POST /api/alwayson/timers/daily_audit/trigger status=%d: %s", stTrigTimer, string(rawTrigTimer))
	}

	stDelTimer, rawDelTimer, _ := doJSONReq(t, client, ts.URL, http.MethodDelete, "/api/alwayson/timers/daily_audit", nil)
	if stDelTimer != http.StatusOK {
		t.Fatalf("DELETE /api/alwayson/timers/daily_audit status=%d: %s", stDelTimer, string(rawDelTimer))
	}

	stAckOutbox, rawAckOutbox, _ := doJSONReq(t, client, ts.URL, http.MethodPost, "/api/alwayson/outbox/ack", map[string]any{
		"all": true,
	})
	if stAckOutbox != http.StatusOK {
		t.Fatalf("POST /api/alwayson/outbox/ack status=%d: %s", stAckOutbox, string(rawAckOutbox))
	}

	// 4. Active Run SSE Re-Attach (GET /api/runs/{id}/stream).
	stSess, rawSess, _ := doJSONReq(t, client, ts.URL, http.MethodPost, "/api/sessions", map[string]any{
		"workspace_id": ws.ID,
		"title":        "Stream Attach Session",
	})
	if stSess != http.StatusCreated {
		t.Fatalf("create session status=%d: %s", stSess, string(rawSess))
	}
	var sess struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rawSess, &sess)

	// Start run in background HTTP call while runHold keeps it active.
	runReqBody, _ := json.Marshal(map[string]any{
		"workspace_id": ws.ID,
		"session_id":   sess.ID,
		"prompt":       "Hello stream",
	})
	runHTTPReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/runs/stream", bytes.NewReader(runReqBody))
	runHTTPReq.Header.Set("Origin", ts.URL)
	runHTTPReq.Header.Set("Content-Type", "application/json")

	runRespCh := make(chan *http.Response, 1)
	go func() {
		resp, err := client.Do(runHTTPReq)
		if err == nil {
			runRespCh <- resp
		}
	}()

	var runResp *http.Response
	select {
	case runResp = <-runRespCh:
	case <-time.After(3 * time.Second):
		close(runHold)
		t.Fatalf("timed out waiting for POST /api/runs/stream headers")
	}
	defer runResp.Body.Close()
	runID := strings.TrimSpace(runResp.Header.Get("X-Gorness-Run-ID"))
	if runID == "" {
		close(runHold)
		t.Fatalf("expected X-Gorness-Run-ID header")
	}

	// Re-attach to active run via GET /api/runs/{id}/stream, then release runHold.
	attachReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/runs/"+runID+"/stream", nil)
	attachReq.Header.Set("Origin", ts.URL)
	attachResp, err := client.Do(attachReq)
	if err != nil {
		close(runHold)
		t.Fatalf("GET /api/runs/%s/stream: %v", runID, err)
	}
	defer attachResp.Body.Close()
	if attachResp.StatusCode != http.StatusOK {
		close(runHold)
		t.Fatalf("GET /api/runs/%s/stream status=%d", runID, attachResp.StatusCode)
	}

	// Test POST /api/runs/{id}/steer with {"instruction": "..."} while run is active.
	stSteer, rawSteer, _ := doJSONReq(t, client, ts.URL, http.MethodPost, "/api/runs/"+runID+"/steer", map[string]any{
		"instruction": "Focus on brevity",
	})
	if stSteer != http.StatusOK {
		close(runHold)
		t.Fatalf("POST /api/runs/%s/steer status=%d: %s", runID, stSteer, string(rawSteer))
	}

	close(runHold)

	scanner := bufio.NewScanner(attachResp.Body)
	var sawTextDelta, sawRunFinished bool
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, `"text_delta"`) {
			sawTextDelta = true
		}
		if strings.Contains(line, `"run_completed"`) || strings.Contains(line, `"run_finished"`) {
			sawRunFinished = true
			break
		}
	}
	if !sawTextDelta || !sawRunFinished {
		t.Fatalf("expected re-attached stream to receive text_delta (%v) and run_finished (%v)", sawTextDelta, sawRunFinished)
	}

	// 5. Fleet Mission DELETE /api/fleet/missions/{id} (404 on missing, 200 on completed mission).
	stDelMissingMission, _, _ := doJSONReq(t, client, ts.URL, http.MethodDelete, "/api/fleet/missions/nonexistent_mission", nil)
	if stDelMissingMission != http.StatusNotFound {
		t.Fatalf("expected 404 deleting nonexistent mission, got %d", stDelMissingMission)
	}

	stStartMission, rawStartMission, _ := doJSONReq(t, client, ts.URL, http.MethodPost, "/api/fleet/missions", map[string]any{
		"workspace_id": ws.ID,
		"workflow_id":  "hybrid-fleet",
		"goal":         "Quick check",
	})
	if stStartMission != http.StatusCreated {
		t.Fatalf("POST /api/fleet/missions status=%d: %s", stStartMission, string(rawStartMission))
	}
	var startedMission fleet.MissionSnapshot
	_ = json.Unmarshal(rawStartMission, &startedMission)

	// Wait for mission to complete (or stop it if still running), then delete it.
	_, _, _ = doJSONReq(t, client, ts.URL, http.MethodPost, "/api/fleet/missions/"+startedMission.ID+"/stop?workspace_id="+ws.ID, map[string]any{})
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		stDelTry, _, _ := doJSONReq(t, client, ts.URL, http.MethodDelete, "/api/fleet/missions/"+startedMission.ID+"?workspace_id="+ws.ID, nil)
		if stDelTry == http.StatusOK {
			break
		}
		if stDelTry != http.StatusConflict {
			t.Fatalf("unexpected status deleting mission %s: %d", startedMission.ID, stDelTry)
		}
		time.Sleep(30 * time.Millisecond)
	}
	stGetDeleted, _, _ := doJSONReq(t, client, ts.URL, http.MethodGet, "/api/fleet/missions/"+startedMission.ID+"?workspace_id="+ws.ID, nil)
	if stGetDeleted != http.StatusNotFound {
		t.Fatalf("expected 404 after deleting mission %s, got %d", startedMission.ID, stGetDeleted)
	}
}

func TestConfigHooksGetAndUpsertWithValidation(t *testing.T) {
	srv, vaultDir, cleanup := newTestStack(t, nil, nil)
	defer cleanup()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	client := ts.Client()

	// 1. Initial GET /api/config/hooks should succeed and return default/empty hooks.
	stGetInit, rawGetInit, _ := doJSONReq(t, client, ts.URL, http.MethodGet, "/api/config/hooks", nil)
	if stGetInit != http.StatusOK {
		t.Fatalf("GET /api/config/hooks status=%d: %s", stGetInit, string(rawGetInit))
	}
	var getInitResp struct {
		Hooks config.HooksConfig `json:"hooks"`
	}
	if err := json.Unmarshal(rawGetInit, &getInitResp); err != nil {
		t.Fatalf("decode initial hooks response: %v", err)
	}

	// 2. PUT /api/config/hooks with invalid payload:
	// 2a. Malformed / type-mismatched JSON body
	stBadJSON, rawBadJSON, _ := doJSONReq(t, client, ts.URL, http.MethodPut, "/api/config/hooks", map[string]any{
		"pre_tool_use": "not-an-array",
	})
	if stBadJSON != http.StatusBadRequest {
		t.Fatalf("expected 400 on malformed json, got %d: %s", stBadJSON, string(rawBadJSON))
	}

	// 2b. Hook with empty command (pre-write ValidateCandidate must reject it)
	stEmptyCmd, rawEmptyCmd, _ := doJSONReq(t, client, ts.URL, http.MethodPut, "/api/config/hooks", map[string]any{
		"pre_tool_use": []map[string]any{
			{
				"id":      "invalid-hook",
				"command": "   ",
			},
		},
	})
	if stEmptyCmd != http.StatusBadRequest {
		t.Fatalf("expected 400 on empty command hook, got %d: %s", stEmptyCmd, string(rawEmptyCmd))
	}

	// Verify disk hooks.toml was NOT written with invalid data
	diskHooksPre, err := os.ReadFile(filepath.Join(vaultDir, "hooks.toml"))
	if err == nil && strings.Contains(string(diskHooksPre), "invalid-hook") {
		t.Fatalf("disk hooks.toml was modified on failed validation: %s", string(diskHooksPre))
	}

	// 3. PUT /api/config/hooks with valid lifecycle hooks
	validPayload := map[string]any{
		"pre_tool_use": []map[string]any{
			{
				"id":          "audit-shell",
				"matcher":     "run_command",
				"tools":       []string{"run_command"},
				"command":     "cat >/dev/null",
				"timeout_sec": 15,
			},
		},
		"post_tool_use": []map[string]any{
			{
				"id":          "log-output",
				"matcher":     "*",
				"command":     "echo post-tool",
				"timeout_sec": 5,
			},
		},
		"turn_stopping": []map[string]any{
			{
				"id":          "notify-stop",
				"command":     "echo stopping",
				"timeout_sec": 10,
			},
		},
	}
	stPut, rawPut, _ := doJSONReq(t, client, ts.URL, http.MethodPut, "/api/config/hooks", validPayload)
	if stPut != http.StatusOK {
		t.Fatalf("PUT /api/config/hooks status=%d: %s", stPut, string(rawPut))
	}

	var putResp struct {
		Hooks    config.HooksConfig `json:"hooks"`
		Config   *config.Snapshot   `json:"config"`
		Snapshot *config.Snapshot   `json:"snapshot"`
	}
	if err := json.Unmarshal(rawPut, &putResp); err != nil {
		t.Fatalf("decode put hooks response: %v", err)
	}

	if len(putResp.Hooks.PreToolUse) != 1 || putResp.Hooks.PreToolUse[0].ID != "audit-shell" {
		t.Fatalf("expected pre_tool_use with id audit-shell, got %+v", putResp.Hooks.PreToolUse)
	}
	if putResp.Hooks.PreToolUse[0].Command != "cat >/dev/null" || putResp.Hooks.PreToolUse[0].TimeoutSec != 15 {
		t.Fatalf("unexpected pre_tool_use spec: %+v", putResp.Hooks.PreToolUse[0])
	}
	if len(putResp.Hooks.PostToolUse) != 1 || putResp.Hooks.PostToolUse[0].ID != "log-output" {
		t.Fatalf("expected post_tool_use with id log-output, got %+v", putResp.Hooks.PostToolUse)
	}
	if len(putResp.Hooks.TurnStopping) != 1 || putResp.Hooks.TurnStopping[0].ID != "notify-stop" {
		t.Fatalf("expected turn_stopping with id notify-stop, got %+v", putResp.Hooks.TurnStopping)
	}
	if putResp.Config == nil || len(putResp.Config.Hooks.PreToolUse) != 1 {
		t.Fatalf("expected config snapshot with updated hooks, got %+v", putResp.Config)
	}

	// 4. Verify disk hooks.toml file
	diskHooks, err := os.ReadFile(filepath.Join(vaultDir, "hooks.toml"))
	if err != nil {
		t.Fatalf("read disk hooks.toml: %v", err)
	}
	if !strings.Contains(string(diskHooks), "audit-shell") || !strings.Contains(string(diskHooks), "cat >/dev/null") {
		t.Fatalf("disk hooks.toml does not contain saved hooks:\n%s", string(diskHooks))
	}

	// 5. Subsequent GET /api/config/hooks should reflect the newly saved hooks
	stGet, rawGet, _ := doJSONReq(t, client, ts.URL, http.MethodGet, "/api/config/hooks", nil)
	if stGet != http.StatusOK {
		t.Fatalf("GET /api/config/hooks status=%d: %s", stGet, string(rawGet))
	}
	var getResp struct {
		Hooks config.HooksConfig `json:"hooks"`
	}
	if err := json.Unmarshal(rawGet, &getResp); err != nil {
		t.Fatalf("decode get hooks response: %v", err)
	}
	if len(getResp.Hooks.PreToolUse) != 1 || getResp.Hooks.PreToolUse[0].ID != "audit-shell" {
		t.Fatalf("expected 1 pre_tool_use hook after put, got %+v", getResp.Hooks.PreToolUse)
	}
	if len(getResp.Hooks.PostToolUse) != 1 || getResp.Hooks.PostToolUse[0].ID != "log-output" {
		t.Fatalf("expected 1 post_tool_use hook after put, got %+v", getResp.Hooks.PostToolUse)
	}
	if len(getResp.Hooks.TurnStopping) != 1 || getResp.Hooks.TurnStopping[0].ID != "notify-stop" {
		t.Fatalf("expected 1 turn_stopping hook after put, got %+v", getResp.Hooks.TurnStopping)
	}

	// Also verify direct unmarshal as config.HooksConfig
	var directHooks config.HooksConfig
	if err := json.Unmarshal(rawGet, &directHooks); err != nil {
		t.Fatalf("decode direct hooks config: %v", err)
	}
	if len(directHooks.PreToolUse) != 1 || directHooks.PreToolUse[0].ID != "audit-shell" {
		t.Fatalf("expected direct unmarshal pre_tool_use to have audit-shell, got %+v", directHooks.PreToolUse)
	}

	// 6. PUT /api/config/hooks clearing hooks
	emptyPayload := map[string]any{
		"pre_tool_use":  []any{},
		"post_tool_use": []any{},
		"turn_stopping": []any{},
	}
	stClear, rawClear, _ := doJSONReq(t, client, ts.URL, http.MethodPut, "/api/config/hooks", emptyPayload)
	if stClear != http.StatusOK {
		t.Fatalf("PUT /api/config/hooks clear status=%d: %s", stClear, string(rawClear))
	}
	var clearResp struct {
		Hooks config.HooksConfig `json:"hooks"`
	}
	if err := json.Unmarshal(rawClear, &clearResp); err != nil {
		t.Fatalf("decode clear hooks response: %v", err)
	}
	if !clearResp.Hooks.IsEmpty() {
		t.Fatalf("expected hooks to be empty after clearing, got %+v", clearResp.Hooks)
	}
}
