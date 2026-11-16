package server_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gorness.dev/gorness/internal/alwayson"
	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/core"
	"gorness.dev/gorness/internal/fleet"
	"gorness.dev/gorness/internal/llm"
	"gorness.dev/gorness/internal/server"
	"gorness.dev/gorness/internal/store"
	"gorness.dev/gorness/internal/tools"
)

type fakeNotifier struct {
	mu    sync.Mutex
	calls []string
}

func (n *fakeNotifier) NotifyMac(title, subtitle, body string) error {
	n.mu.Lock()
	n.calls = append(n.calls, title+"|"+subtitle+"|"+body)
	n.mu.Unlock()
	return nil
}

func initTempGitRepo(t *testing.T, dir string) {
	t.Helper()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Temp Repo\nInitial line.\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte("package app\n\nfunc Value() int { return 1 }\n"), 0o644); err != nil {
		t.Fatalf("write app.go: %v", err)
	}
	runGit("init", "-b", "main")
	runGit("config", "user.name", "Gorness Test")
	runGit("config", "user.email", "test@gorness.local")
	runGit("config", "commit.gpgsign", "false")
	runGit("add", "-A")
	runGit("commit", "-m", "chore: initial commit")
}

func newTestStack(t *testing.T, streamer llm.Streamer, opener server.OpenerFunc) (*server.Server, string, func()) {
	t.Helper()
	baseDir := t.TempDir()
	fakeHome := filepath.Join(baseDir, "home")
	vaultDir := filepath.Join(fakeHome, ".gorness")
	if err := os.MkdirAll(fakeHome, 0o700); err != nil {
		t.Fatalf("mkdir fakeHome: %v", err)
	}

	cfgMgr, err := config.NewManager(config.Options{
		VaultRoot:   vaultDir,
		UserHomeDir: fakeHome,
		EnvLookup: func(key string) (string, bool) {
			return "test-secret-" + strings.ToLower(key), true
		},
	})
	if err != nil {
		t.Fatalf("config.NewManager: %v", err)
	}

	router := llm.NewRouter(cfgMgr, cfgMgr.Secrets(), streamer)
	eng, err := core.NewEngine(core.EngineOptions{
		Config:   cfgMgr,
		Router:   router,
		UserHome: fakeHome,
	})
	if err != nil {
		t.Fatalf("core.NewEngine: %v", err)
	}

	aoSvc, err := alwayson.NewService(alwayson.ServiceOptions{
		Config:       cfgMgr,
		Engine:       eng,
		Notifier:     &fakeNotifier{},
		TickInterval: 25 * time.Millisecond,
		RetryBackoff: 15 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("alwayson.NewService: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := aoSvc.Start(ctx); err != nil {
		cancel()
		t.Fatalf("alwayson.Start: %v", err)
	}

	fleetMgr, err := fleet.NewManager(fleet.ManagerOptions{
		Config: cfgMgr,
		Engine: eng,
		VerifyRunner: func(_ context.Context, _, _ string) (tools.CommandResult, error) {
			return tools.CommandResult{Stdout: "ok", Combined: "ok", ExitCode: 0}, nil
		},
	})
	if err != nil {
		cancel()
		_ = aoSvc.Close()
		t.Fatalf("fleet.NewManager: %v", err)
	}

	if opener == nil {
		opener = func(_ context.Context, _ []string) error { return nil }
	}
	srv := server.NewServer(server.Deps{
		Config:            cfgMgr,
		Engine:            eng,
		AlwaysOn:          aoSvc,
		Fleet:             fleetMgr,
		Opener:            opener,
		HeartbeatInterval: 200 * time.Millisecond,
	})

	cleanup := func() {
		cancel()
		_ = srv.Close()
		_ = fleetMgr.Close()
		_ = aoSvc.Close()
		_ = eng.Close()
	}
	return srv, vaultDir, cleanup
}

func TestServerOriginGuardAndIsolatedPreviewCSP(t *testing.T) {
	var (
		openMu    sync.Mutex
		openCalls [][]string
	)
	opener := func(_ context.Context, args []string) error {
		openMu.Lock()
		cp := append([]string(nil), args...)
		openCalls = append(openCalls, cp)
		openMu.Unlock()
		return nil
	}

	noopStreamer := llm.StreamerFunc(func(_ context.Context, _ llm.Request) (<-chan llm.StreamEvent, error) {
		ch := make(chan llm.StreamEvent, 2)
		ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "ok"}
		ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn"}
		close(ch)
		return ch, nil
	})

	srv, vaultDir, cleanup := newTestStack(t, noopStreamer, opener)
	defer cleanup()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	client := ts.Client()

	// 1. Cross-origin Origin, null Origin, cross-port Origin, cross-origin Referer, and DNS-rebinding Host must get 403 and zero CORS headers.
	blockedCases := []struct {
		name    string
		method  string
		headers map[string]string
		host    string
	}{
		{
			name:   "cross_origin_post",
			method: http.MethodPost,
			headers: map[string]string{
				"Origin":       "https://evil.example",
				"Content-Type": "application/json",
			},
		},
		{
			name:   "cross_origin_options_preflight",
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        "https://evil.example",
				"Access-Control-Request-Method": "POST",
			},
		},
		{
			name:   "sandbox_null_origin",
			method: http.MethodPost,
			headers: map[string]string{
				"Origin": "null",
			},
		},
		{
			name:   "cross_port_loopback_origin",
			method: http.MethodPost,
			headers: map[string]string{
				"Origin": "http://127.0.0.1:65500",
			},
		},
		{
			name:   "cross_origin_referer",
			method: http.MethodGet,
			headers: map[string]string{
				"Referer": "https://evil.example/attack.html",
			},
		},
		{
			name:   "dns_rebinding_host",
			method: http.MethodGet,
			host:   "rebind.evil.example:3456",
		},
	}

	for _, tc := range blockedCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, ts.URL+"/api/bootstrap", strings.NewReader(`{}`))
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			if tc.host != "" {
				req.Host = tc.host
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("do request: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("expected 403 Forbidden for %s, got %d", tc.name, resp.StatusCode)
			}
			for k := range resp.Header {
				if strings.HasPrefix(strings.ToLower(k), "access-control-allow-") {
					t.Fatalf("unexpected CORS header %q emitted on %s", k, tc.name)
				}
			}
		})
	}

	// Valid same-origin loopback request must succeed with zero CORS headers.
	{
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/health", nil)
		req.Header.Set("Origin", ts.URL)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("same-origin health: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 for same-origin health, got %d", resp.StatusCode)
		}
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("expected empty Access-Control-Allow-Origin, got %q", got)
		}
	}

	// 2. Verify /preview/:token strict CSP shell and sandboxed iframe WITHOUT allow-same-origin, plus Bidi & attribute escape.
	untrustedTitle := "Report \u202e<script>alert('title')</script>"
	untrustedHTML := `<html><body><h1>Chart</h1>"><iframe src="https://evil.example"></iframe><script>fetch("http://127.0.0.1/api/bootstrap")</script></body></html>`
	previewReqBody, _ := json.Marshal(map[string]any{
		"title": untrustedTitle,
		"html":  untrustedHTML,
	})
	pResp, err := client.Post(ts.URL+"/api/previews", "application/json", bytes.NewReader(previewReqBody))
	if err != nil {
		t.Fatalf("POST /api/previews: %v", err)
	}
	defer pResp.Body.Close()
	if pResp.StatusCode != http.StatusOK && pResp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(pResp.Body)
		t.Fatalf("POST /api/previews status=%d body=%s", pResp.StatusCode, string(raw))
	}
	var previewCreated struct {
		Token string `json:"token"`
		URL   string `json:"url"`
	}
	if err := json.NewDecoder(pResp.Body).Decode(&previewCreated); err != nil {
		t.Fatalf("decode preview response: %v", err)
	}
	if previewCreated.Token == "" {
		t.Fatalf("expected non-empty preview token")
	}

	shellResp, err := client.Get(ts.URL + "/preview/" + previewCreated.Token)
	if err != nil {
		t.Fatalf("GET /preview/:token: %v", err)
	}
	shellBytes, _ := io.ReadAll(shellResp.Body)
	shellResp.Body.Close()
	if shellResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /preview/:token status=%d", shellResp.StatusCode)
	}

	csp := shellResp.Header.Get("Content-Security-Policy")
	for _, wantDirective := range []string{
		"default-src 'none'",
		"style-src 'unsafe-inline'",
		"script-src 'unsafe-inline'",
		"img-src data:",
		"connect-src 'none'",
		"frame-src 'none'",
		"form-action 'none'",
	} {
		if !strings.Contains(csp, wantDirective) {
			t.Fatalf("CSP header %q missing required directive %q", csp, wantDirective)
		}
	}

	shellHTML := string(shellBytes)
	if !strings.Contains(shellHTML, `sandbox="allow-scripts"`) {
		t.Fatalf("preview shell missing sandbox=\"allow-scripts\":\n%s", shellHTML)
	}
	if strings.Contains(strings.ToLower(shellHTML), "allow-same-origin") {
		t.Fatalf("preview shell MUST NEVER include allow-same-origin:\n%s", shellHTML)
	}
	if !strings.Contains(shellHTML, "&lt;U+202E&gt;") && !strings.Contains(shellHTML, "<U+202E>") {
		t.Fatalf("expected Bidi control char neutralized in preview shell, got:\n%s", shellHTML)
	}
	if strings.Contains(shellHTML, `<script>alert('title')</script>`) {
		t.Fatalf("untrusted title script tag was not escaped in preview shell!")
	}

	// 3. Verify external_open (/api/open-safe): allowlisted docs copied 0400 into runtime/open-copies/, non-allowlisted revealed with -R, symlinks outside workspace blocked.
	wsDir := filepath.Join(t.TempDir(), "proj-open")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("mkdir wsDir: %v", err)
	}
	docPath := filepath.Join(wsDir, "SPEC.md")
	if err := os.WriteFile(docPath, []byte("# Architecture Spec\nSafe markdown copy.\n"), 0o644); err != nil {
		t.Fatalf("write SPEC.md: %v", err)
	}
	scriptPath := filepath.Join(wsDir, "install.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho pwn\n"), 0o755); err != nil {
		t.Fatalf("write install.sh: %v", err)
	}
	escapeLink := filepath.Join(wsDir, "escape.toml")
	if err := os.Symlink(filepath.Join(vaultDir, "providers.toml"), escapeLink); err != nil {
		t.Fatalf("symlink escape.toml: %v", err)
	}

	wsRegBody, _ := json.Marshal(map[string]string{"path": wsDir, "name": "OpenProj"})
	wsResp, err := client.Post(ts.URL+"/api/workspaces", "application/json", bytes.NewReader(wsRegBody))
	if err != nil {
		t.Fatalf("register workspace: %v", err)
	}
	var wsObj struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(wsResp.Body).Decode(&wsObj)
	wsResp.Body.Close()

	// 3a. Allowlisted .md -> copied 0400 into <vault>/runtime/open-copies/
	openDocBody, _ := json.Marshal(map[string]string{
		"workspace_id": wsObj.ID,
		"path":         "SPEC.md",
	})
	oResp1, err := client.Post(ts.URL+"/api/open-safe", "application/json", bytes.NewReader(openDocBody))
	if err != nil {
		t.Fatalf("POST /api/open-safe SPEC.md: %v", err)
	}
	var openRes1 struct {
		Action     string `json:"action"`
		OpenedPath string `json:"opened_path"`
	}
	_ = json.NewDecoder(oResp1.Body).Decode(&openRes1)
	oResp1.Body.Close()
	if oResp1.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for SPEC.md open-safe, got %d", oResp1.StatusCode)
	}
	wantCopiesPrefix := filepath.Join(vaultDir, "runtime", "open-copies")
	if canonPrefix, eErr := filepath.EvalSymlinks(wantCopiesPrefix); eErr == nil {
		wantCopiesPrefix = canonPrefix
	}
	canonOpened, err := filepath.EvalSymlinks(openRes1.OpenedPath)
	if err != nil {
		t.Fatalf("EvalSymlinks opened copy %s: %v", openRes1.OpenedPath, err)
	}
	if !strings.HasPrefix(canonOpened, wantCopiesPrefix+string(filepath.Separator)) {
		t.Fatalf("opened copy %q is not inside %q", canonOpened, wantCopiesPrefix)
	}
	stCopy, err := os.Stat(canonOpened)
	if err != nil {
		t.Fatalf("stat opened copy: %v", err)
	}
	if stCopy.Mode().Perm() != 0o400 {
		t.Fatalf("expected 0400 read-only mode on copy, got %04o", stCopy.Mode().Perm())
	}

	// 3b. Non-allowlisted .sh -> revealed in Finder via open -R
	openScriptBody, _ := json.Marshal(map[string]string{
		"workspace_id": wsObj.ID,
		"path":         "install.sh",
	})
	oResp2, err := client.Post(ts.URL+"/api/open-safe", "application/json", bytes.NewReader(openScriptBody))
	if err != nil {
		t.Fatalf("POST /api/open-safe install.sh: %v", err)
	}
	oResp2.Body.Close()
	if oResp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for install.sh reveal, got %d", oResp2.StatusCode)
	}

	// 3c. Symlink escaping into vault -> rejected with 403
	openEscapeBody, _ := json.Marshal(map[string]string{
		"workspace_id": wsObj.ID,
		"path":         "escape.toml",
	})
	oResp3, err := client.Post(ts.URL+"/api/open-safe", "application/json", bytes.NewReader(openEscapeBody))
	if err != nil {
		t.Fatalf("POST /api/open-safe escape.toml: %v", err)
	}
	oResp3.Body.Close()
	if oResp3.StatusCode != http.StatusForbidden && oResp3.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 403/400 for escaping symlink, got %d", oResp3.StatusCode)
	}

	openMu.Lock()
	defer openMu.Unlock()
	if len(openCalls) != 2 {
		t.Fatalf("expected exactly 2 opener invocations, got %d: %v", len(openCalls), openCalls)
	}
	if len(openCalls[0]) != 1 || openCalls[0][0] != openRes1.OpenedPath {
		t.Fatalf("unexpected 1st opener args: %v", openCalls[0])
	}
	if len(openCalls[1]) != 2 || openCalls[1][0] != "-R" {
		t.Fatalf("expected Finder reveal ['-R', path] for non-allowlisted file, got %v", openCalls[1])
	}

	// 4. Verify HTTP /api/approvals/:id/resolve enforces 500ms settle guard (409 before 500ms, 200 after 500ms).
	askStreamer := llm.StreamerFunc(func(_ context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
		ch := make(chan llm.StreamEvent, 4)
		hasToolRes := len(req.History) > 0 && req.History[len(req.History)-1].Role == "tool"
		if !hasToolRes {
			args, _ := json.Marshal(map[string]any{"command": "echo settled"})
			ch <- llm.StreamEvent{
				Type:       llm.StreamToolCall,
				ToolCallID: "call_ask_1",
				ToolName:   "run_command",
				ToolInput:  args,
			}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use"}
		} else {
			ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "approved command finished"}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn"}
		}
		close(ch)
		return ch, nil
	})
	askSrv, _, askCleanup := newTestStack(t, askStreamer, opener)
	defer askCleanup()
	askTS := httptest.NewServer(askSrv.Handler())
	defer askTS.Close()

	wsAskResp, err := askTS.Client().Post(askTS.URL+"/api/workspaces", "application/json", bytes.NewReader(wsRegBody))
	if err != nil {
		t.Fatalf("register workspace on askTS: %v", err)
	}
	var wsAsk struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(wsAskResp.Body).Decode(&wsAsk)
	wsAskResp.Body.Close()

	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		runReq, _ := json.Marshal(map[string]any{
			"workspace_id": wsAsk.ID,
			"perm_mode":    "ask",
			"sandbox_mode": "off",
			"prompt":       "run echo",
		})
		rResp, rErr := askTS.Client().Post(askTS.URL+"/api/runs/stream", "application/json", bytes.NewReader(runReq))
		if rErr == nil {
			_, _ = io.Copy(io.Discard, rResp.Body)
			rResp.Body.Close()
		}
	}()

	var apprID string
	waitDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(waitDeadline) {
		aResp, aErr := askTS.Client().Get(askTS.URL + "/api/approvals")
		if aErr == nil {
			var list []core.PendingApproval
			_ = json.NewDecoder(aResp.Body).Decode(&list)
			aResp.Body.Close()
			if len(list) > 0 {
				apprID = list[0].ID
				break
			}
		}
		time.Sleep(15 * time.Millisecond)
	}
	if apprID == "" {
		t.Fatalf("expected pending approval to surface in ask mode")
	}

	resolvePayload, _ := json.Marshal(map[string]any{"action": "approve_all"})
	earlyResp, err := askTS.Client().Post(askTS.URL+"/api/approvals/"+apprID+"/resolve", "application/json", bytes.NewReader(resolvePayload))
	if err != nil {
		t.Fatalf("early resolve: %v", err)
	}
	earlyResp.Body.Close()
	if earlyResp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 Conflict for <500ms rapid approval, got %d", earlyResp.StatusCode)
	}

	time.Sleep(530 * time.Millisecond)
	settledResp, err := askTS.Client().Post(askTS.URL+"/api/approvals/"+apprID+"/resolve", "application/json", bytes.NewReader(resolvePayload))
	if err != nil {
		t.Fatalf("settled resolve: %v", err)
	}
	settledResp.Body.Close()
	if settledResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK after 500ms settle window, got %d", settledResp.StatusCode)
	}
	<-runDone
}

func TestEndToEndMultiProjectSessionFleetAndAlwaysOnAPI(t *testing.T) {
	wsADir := filepath.Join(t.TempDir(), "alpha-service")
	wsBDir := filepath.Join(t.TempDir(), "beta-worker")
	initTempGitRepo(t, wsADir)
	initTempGitRepo(t, wsBDir)

	var (
		mu             sync.Mutex
		coderSawWrite  = make(map[string]bool)
		fleetWriteDone bool
	)

	mockStreamer := llm.StreamerFunc(func(_ context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
		ch := make(chan llm.StreamEvent, 8)
		var lastUser string
		hasToolResult := false
		for i := len(req.History) - 1; i >= 0; i-- {
			if req.History[i].Role == "tool" {
				if i == len(req.History)-1 {
					hasToolResult = true
				}
			}
			if req.History[i].Role == "user" && lastUser == "" {
				for _, b := range req.History[i].Blocks {
					if b.Text != "" {
						lastUser = b.Text
						break
					}
				}
			}
		}

		sysLower := strings.ToLower(req.StaticSystem)
		switch {
		case strings.Contains(sysLower, "housekeeping utility"):
			if strings.Contains(lastUser, "turns_to_compact") {
				ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "## Goal & Intent\nCompacted multi-project progress."}
			} else {
				ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Alpha Service Refactor"}
			}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", InputTokens: 20, OutputTokens: 6}

		case strings.Contains(sysLower, "adversarial verifier"):
			ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Diff and tests verified cleanly.\nVERDICT: ACCEPT"}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", InputTokens: 40, OutputTokens: 12}

		case strings.Contains(sysLower, "fast explorer"):
			ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Recon complete: app.go exports Value()."}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", InputTokens: 30, OutputTokens: 10}

		case strings.Contains(sysLower, "lead architect"):
			ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Plan: update app.go and add fleet_feature.go."}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", InputTokens: 50, OutputTokens: 15}

		default:
			// Coder / custom specialist agent
			mu.Lock()
			isFleet := strings.Contains(lastUser, "Add FleetStamp helper")
			alreadyFleetWrote := fleetWriteDone
			if isFleet && !alreadyFleetWrote && !hasToolResult {
				fleetWriteDone = true
				mu.Unlock()
				args, _ := json.Marshal(map[string]any{
					"path":    "fleet_feature.go",
					"content": "package app\n\nfunc FleetStamp() string { return \"landed\" }\n",
				})
				ch <- llm.StreamEvent{
					Type:       llm.StreamToolCall,
					ToolCallID: "call_fleet_write_1",
					ToolName:   "write_file",
					ToolInput:  args,
				}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use", InputTokens: 45, OutputTokens: 20}
				close(ch)
				return ch, nil
			}
			needEdit := strings.Contains(lastUser, "Update app.go Value to 42") && !coderSawWrite["wsA"] && !hasToolResult
			if needEdit {
				coderSawWrite["wsA"] = true
			}
			mu.Unlock()

			if needEdit {
				args, _ := json.Marshal(map[string]any{
					"path":    "app.go",
					"content": "package app\n\nfunc Value() int { return 42 }\n",
				})
				ch <- llm.StreamEvent{
					Type:       llm.StreamToolCall,
					ToolCallID: "call_edit_app_1",
					ToolName:   "write_file",
					ToolInput:  args,
				}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use", InputTokens: 35, OutputTokens: 18}
				close(ch)
				return ch, nil
			}

			ch <- llm.StreamEvent{Type: llm.StreamThinkingDelta, Thinking: "Checking workspace state..."}
			ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Completed task for: " + lastUser}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", InputTokens: 32, OutputTokens: 14}
		}

		close(ch)
		return ch, nil
	})

	srv, vaultDir, cleanup := newTestStack(t, mockStreamer, nil)
	defer cleanup()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	client := ts.Client()

	// 1. Verify embedded Apple HIG Workbench (/, /styles.css, /app.js) serves cleanly.
	for _, assetPath := range []string{"/", "/styles.css", "/app.js"} {
		resp, err := client.Get(ts.URL + assetPath)
		if err != nil {
			t.Fatalf("GET %s: %v", assetPath, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status=%d", assetPath, resp.StatusCode)
		}
		if len(body) < 128 {
			t.Fatalf("GET %s unexpectedly short (%d bytes)", assetPath, len(body))
		}
	}

	// 2. Register 2 concurrent workspaces (wsA & wsB).
	registerWS := func(path, name string) string {
		t.Helper()
		b, _ := json.Marshal(map[string]string{"path": path, "name": name})
		resp, err := client.Post(ts.URL+"/api/workspaces", "application/json", bytes.NewReader(b))
		if err != nil {
			t.Fatalf("POST /api/workspaces: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			raw, _ := io.ReadAll(resp.Body)
			t.Fatalf("register workspace %s status=%d: %s", name, resp.StatusCode, string(raw))
		}
		var out struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.ID == "" {
			t.Fatalf("decode workspace %s: %v", name, err)
		}
		return out.ID
	}
	wsAID := registerWS(wsADir, "Alpha Service")
	wsBID := registerWS(wsBDir, "Beta Worker")

	// 3. Create provider & agent profile via /api/config/* and verify immediate hot-reload.
	provPayload, _ := json.Marshal(map[string]any{
		"id":       "apple_gateway",
		"name":     "Apple Internal Gateway",
		"protocol": "openai-compat",
		"base_url": "http://127.0.0.1:18080/v1",
		"api_key":  "env:APPLE_GW_KEY",
		"models": map[string]any{
			"carbon-coder": map[string]any{
				"name":           "Carbon Coder",
				"context_window": 200000,
			},
		},
	})
	if resp, err := client.Post(ts.URL+"/api/config/providers", "application/json", bytes.NewReader(provPayload)); err != nil {
		t.Fatalf("POST /api/config/providers: %v", err)
	} else {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST /api/config/providers status=%d: %s", resp.StatusCode, string(raw))
		}
	}

	agentPayload, _ := json.Marshal(map[string]any{
		"id":            "specialist",
		"name":          "Carbon Specialist",
		"provider":      "apple_gateway",
		"model":         "carbon-coder",
		"effort":        "high",
		"system_prompt": "You are the Carbon Specialist engineer.",
	})
	if resp, err := client.Post(ts.URL+"/api/config/agents", "application/json", bytes.NewReader(agentPayload)); err != nil {
		t.Fatalf("POST /api/config/agents: %v", err)
	} else {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST /api/config/agents status=%d: %s", resp.StatusCode, string(raw))
		}
	}

	// 4. Stream sessions concurrently across wsA (using newly created specialist agent!) and wsB over /api/runs/stream.
	runStreamSession := func(wsID, role, prompt string) (sessionID string, events []core.RunEvent) {
		t.Helper()
		b, _ := json.Marshal(map[string]any{
			"workspace_id": wsID,
			"agent_role":   role,
			"prompt":       prompt,
		})
		resp, err := client.Post(ts.URL+"/api/runs/stream", "application/json", bytes.NewReader(b))
		if err != nil {
			t.Fatalf("POST /api/runs/stream: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			raw, _ := io.ReadAll(resp.Body)
			t.Fatalf("POST /api/runs/stream status=%d: %s", resp.StatusCode, string(raw))
		}
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "" {
				continue
			}
			var probe struct {
				Type string `json:"type"`
			}
			if json.Unmarshal([]byte(payload), &probe) == nil && probe.Type == "heartbeat" {
				continue
			}
			var ev core.RunEvent
			if err := json.Unmarshal([]byte(payload), &ev); err != nil {
				t.Fatalf("parse SSE RunEvent %q: %v", payload, err)
			}
			if ev.SessionID != "" {
				sessionID = ev.SessionID
			}
			events = append(events, ev)
		}
		return sessionID, events
	}

	var (
		sessAID string
		sessBID string
		evA     []core.RunEvent
		evB     []core.RunEvent
		wg      sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		sessAID, evA = runStreamSession(wsAID, "specialist", "Update app.go Value to 42")
	}()
	go func() {
		defer wg.Done()
		sessBID, evB = runStreamSession(wsBID, "coder", "Inspect beta worker baseline")
	}()
	wg.Wait()

	if sessAID == "" || sessBID == "" {
		t.Fatalf("expected session IDs for both concurrent workspaces, got A=%q B=%q", sessAID, sessBID)
	}
	if len(evA) == 0 || len(evB) == 0 {
		t.Fatalf("expected non-empty streamed events: len(evA)=%d len(evB)=%d", len(evA), len(evB))
	}

	// Verify app.go in wsA was updated to 42 before rewind.
	appAfterEdit, err := os.ReadFile(filepath.Join(wsADir, "app.go"))
	if err != nil || !strings.Contains(string(appAfterEdit), "return 42") {
		t.Fatalf("expected app.go in wsA to contain 'return 42', got: %s (err=%v)", string(appAfterEdit), err)
	}

	// 5. Fork & Rewind session (restoring source file via SHA-256 snapshot!).
	forkBody, _ := json.Marshal(map[string]any{"at_seq": 999999, "title": "Alpha Forked"})
	fResp, err := client.Post(ts.URL+"/api/sessions/"+sessAID+"/fork", "application/json", bytes.NewReader(forkBody))
	if err != nil {
		t.Fatalf("POST fork: %v", err)
	}
	var forkedMeta struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(fResp.Body).Decode(&forkedMeta)
	fResp.Body.Close()
	if fResp.StatusCode != http.StatusOK && fResp.StatusCode != http.StatusCreated {
		t.Fatalf("fork status=%d", fResp.StatusCode)
	}
	if forkedMeta.ID == "" || forkedMeta.ID == sessAID {
		t.Fatalf("unexpected forked session id %q", forkedMeta.ID)
	}

	rewindBody, _ := json.Marshal(map[string]any{"to_seq": 0, "restore_files": true})
	rResp, err := client.Post(ts.URL+"/api/sessions/"+sessAID+"/rewind", "application/json", bytes.NewReader(rewindBody))
	if err != nil {
		t.Fatalf("POST rewind: %v", err)
	}
	rRaw, _ := io.ReadAll(rResp.Body)
	rResp.Body.Close()
	if rResp.StatusCode != http.StatusOK {
		t.Fatalf("rewind status=%d body=%s", rResp.StatusCode, string(rRaw))
	}
	appAfterRewind, err := os.ReadFile(filepath.Join(wsADir, "app.go"))
	if err != nil || !strings.Contains(string(appAfterRewind), "return 1") {
		t.Fatalf("expected rewind with restore_files=true to restore 'return 1', got: %s", string(appAfterRewind))
	}

	// Commit clean baseline or verify wsA is clean for hybrid-fleet mission.
	// 6. Launch a hybrid-fleet mission in wsA and stream topology events over /api/fleet/missions/:id/stream.
	missionReq, _ := json.Marshal(map[string]any{
		"workspace_id": wsAID,
		"workflow_id":  "hybrid-fleet",
		"goal":         "Add FleetStamp helper",
	})
	mResp, err := client.Post(ts.URL+"/api/fleet/missions", "application/json", bytes.NewReader(missionReq))
	if err != nil {
		t.Fatalf("POST /api/fleet/missions: %v", err)
	}
	var startedSnap fleet.MissionSnapshot
	if err := json.NewDecoder(mResp.Body).Decode(&startedSnap); err != nil {
		t.Fatalf("decode started mission: %v", err)
	}
	mResp.Body.Close()
	if startedSnap.ID == "" {
		t.Fatalf("expected non-empty mission ID")
	}

	streamResp, err := client.Get(ts.URL + "/api/fleet/missions/" + startedSnap.ID + "/stream")
	if err != nil {
		t.Fatalf("GET /api/fleet/missions/:id/stream: %v", err)
	}
	var fleetEvents []fleet.MissionEvent
	fScanner := bufio.NewScanner(streamResp.Body)
	fScanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for fScanner.Scan() {
		line := fScanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		var probe struct {
			Type string `json:"type"`
		}
		if json.Unmarshal([]byte(payload), &probe) == nil && probe.Type == "heartbeat" {
			continue
		}
		var mev fleet.MissionEvent
		if err := json.Unmarshal([]byte(payload), &mev); err != nil {
			t.Fatalf("parse fleet MissionEvent: %v", err)
		}
		fleetEvents = append(fleetEvents, mev)
		if mev.Type == fleet.EventMissionCompleted || mev.Type == fleet.EventMissionFailed || mev.Type == fleet.EventMissionCancelled {
			break
		}
	}
	streamResp.Body.Close()
	if len(fleetEvents) == 0 {
		t.Fatalf("expected topology events from /api/fleet/missions/:id/stream")
	}
	lastFleetSnap := fleetEvents[len(fleetEvents)-1].Snapshot
	if lastFleetSnap == nil || lastFleetSnap.Status != fleet.MissionCompleted {
		t.Fatalf("expected hybrid-fleet mission completed, got snapshot=%+v", lastFleetSnap)
	}

	// Inspect coder node diff via /api/fleet/missions/:id/nodes/coder/diff
	diffResp, err := client.Get(fmt.Sprintf("%s/api/fleet/missions/%s/nodes/coder/diff", ts.URL, startedSnap.ID))
	if err != nil {
		t.Fatalf("GET coder node diff: %v", err)
	}
	var diffObj struct {
		Diff string `json:"diff"`
	}
	_ = json.NewDecoder(diffResp.Body).Decode(&diffObj)
	diffResp.Body.Close()
	if !strings.Contains(diffObj.Diff, "FleetStamp") {
		t.Fatalf("expected coder node diff to include FleetStamp, got: %q", diffObj.Diff)
	}

	// 7. Trigger Always-On webhook into /api/alwayson/webhook/local and verify processing.
	whBody, _ := json.Marshal(map[string]any{
		"workspace_id": wsBID,
		"source_id":    "wh_evt_101",
		"text":         "Run nightly beta health check",
	})
	whResp, err := client.Post(ts.URL+"/api/alwayson/webhook/local", "application/json", bytes.NewReader(whBody))
	if err != nil {
		t.Fatalf("POST /api/alwayson/webhook/local: %v", err)
	}
	whResp.Body.Close()
	if whResp.StatusCode != http.StatusOK && whResp.StatusCode != http.StatusAccepted {
		t.Fatalf("webhook status=%d", whResp.StatusCode)
	}

	deadline := time.Now().Add(5 * time.Second)
	processed := false
	for time.Now().Before(deadline) {
		stResp, gErr := client.Get(ts.URL + "/api/alwayson/state")
		if gErr == nil {
			var st alwayson.AlwaysOnState
			_ = json.NewDecoder(stResp.Body).Decode(&st)
			stResp.Body.Close()
			for _, item := range st.Inbox {
				if item.SourceID == "wh_evt_101" && item.Status == alwayson.StatusDone {
					processed = true
					break
				}
			}
			if processed {
				break
			}
		}
		time.Sleep(40 * time.Millisecond)
	}
	if !processed {
		t.Fatalf("timed out waiting for Always-On webhook event wh_evt_101 to finish")
	}

	// 8. Verify strict Zero-DB file format invariant: zero .json files anywhere inside vaultDir.
	if err := filepath.WalkDir(vaultDir, func(path string, d os.DirEntry, wErr error) error {
		if wErr != nil {
			return wErr
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(d.Name()), ".json") {
			t.Fatalf("Zero-DB violation: forbidden .json file written inside vault: %s", path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk vaultDir: %v", err)
	}
}

func TestWorkbenchDraftRestoreNestedSubagentsAndRunErrors(t *testing.T) {
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node binary not available")
	}

	wsDir := filepath.Join(t.TempDir(), "studio-ui-proj")
	initTempGitRepo(t, wsDir)

	baseDir := t.TempDir()
	fakeHome := filepath.Join(baseDir, "home")
	vaultDir := filepath.Join(fakeHome, ".gorness")
	if err := os.MkdirAll(fakeHome, 0o700); err != nil {
		t.Fatalf("mkdir fakeHome: %v", err)
	}

	cfgMgr, err := config.NewManager(config.Options{
		VaultRoot:   vaultDir,
		UserHomeDir: fakeHome,
		EnvLookup:   func(k string) (string, bool) { return "secret-" + k, true },
	})
	if err != nil {
		t.Fatalf("config.NewManager: %v", err)
	}

	uiStreamer := llm.StreamerFunc(func(_ context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
		ch := make(chan llm.StreamEvent, 4)
		var lastText string
		if len(req.History) > 0 {
			for _, b := range req.History[len(req.History)-1].Blocks {
				if b.Text != "" {
					lastText = b.Text
				}
			}
		}
		if strings.Contains(lastText, "TRIGGER_STREAM_FAIL") {
			ch <- llm.StreamEvent{Type: llm.StreamError, Err: fmt.Errorf("upstream gateway 502 authentication failure")}
			close(ch)
			return ch, nil
		}
		ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "ok"}
		ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn"}
		close(ch)
		return ch, nil
	})

	router := llm.NewRouter(cfgMgr, cfgMgr.Secrets(), uiStreamer)
	eng, err := core.NewEngine(core.EngineOptions{
		Config:   cfgMgr,
		Router:   router,
		UserHome: fakeHome,
	})
	if err != nil {
		t.Fatalf("core.NewEngine: %v", err)
	}
	defer func() { _ = eng.Close() }()

	srv := server.NewServer(server.Deps{
		Config: cfgMgr,
		Engine: eng,
		Opener: func(_ context.Context, _ []string) error { return nil },
	})
	defer func() { _ = srv.Close() }()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ws, err := eng.Workspaces().Register(wsDir, "StudioUIProj")
	if err != nil {
		t.Fatalf("register ws: %v", err)
	}
	sess, err := eng.Sessions().Create(ws.ID, "Parent Architecture Session", "lead")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Seed a user turn + subagent delegation tool_use & tool_result + forked child session + persisted draft!
	_, _ = eng.Sessions().Append(sess.ID, store.SessionEntry{
		Role:      "user",
		AgentRole: "lead",
		Blocks:    []store.ContentBlock{{Type: "text", Text: "Delegate audit to explorer"}},
	})
	subEntry, err := eng.Sessions().Append(sess.ID, store.SessionEntry{
		Role:      "assistant",
		AgentRole: "lead",
		ModelID:   "claude-opus-5-5",
		Blocks: []store.ContentBlock{
			{
				Type:       "tool_use",
				ToolCallID: "call_sub_1",
				ToolName:   "spawn_subagent",
				ToolInput:  json.RawMessage(`{"agent_role":"explorer","prompt":"Scan app.go symbols"}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("append subagent tool_use: %v", err)
	}
	_, _ = eng.Sessions().Append(sess.ID, store.SessionEntry{
		Role:      "tool",
		AgentRole: "lead",
		Blocks: []store.ContentBlock{
			{
				Type:       "tool_result",
				ToolCallID: "call_sub_1",
				ToolName:   "spawn_subagent",
				Text:       "Found Value() in app.go:3",
			},
		},
	})
	if _, err := eng.Sessions().Fork(sess.ID, subEntry.Seq, "Explorer Branch Investigation"); err != nil {
		t.Fatalf("fork child session: %v", err)
	}
	const wantDraft = "Persisted unsent multi-line refactor draft"
	if _, err := eng.Sessions().UpdateMeta(sess.ID, func(m *store.SessionMeta) {
		m.Draft = wantDraft
	}); err != nil {
		t.Fatalf("persist draft: %v", err)
	}

	appJSPath, err := filepath.Abs(filepath.Join("..", "..", "web", "app.js"))
	if err != nil {
		t.Fatalf("resolve web/app.js path: %v", err)
	}

	nodeScript := fmt.Sprintf(`
import assert from "node:assert/strict";
import { pathToFileURL } from "node:url";

const baseURL = %q;
const elements = new Map();
function makeEl(id) {
  return {
    id,
    value: "",
    textContent: "",
    innerHTML: "",
    checked: true,
    disabled: false,
    href: "",
    src: "",
    style: { setProperty() {}, width: "" },
    dataset: {},
    scrollHeight: 200,
    scrollTop: 200,
    clientHeight: 200,
    classList: {
      _set: new Set(["hidden"]),
      add(...c) { c.forEach((x) => this._set.add(x)); },
      remove(...c) { c.forEach((x) => this._set.delete(x)); },
      toggle(c, force) {
        if (force === undefined) {
          if (this._set.has(c)) { this._set.delete(c); return false; }
          this._set.add(c); return true;
        }
        if (force) this._set.add(c); else this._set.delete(c);
        return force;
      },
      contains(c) { return this._set.has(c); }
    },
    setAttribute(k, v) { this[k] = v; },
    getAttribute(k) { return this[k] || ""; },
    addEventListener() {}
  };
}
function getEl(id) {
  if (!elements.has(id)) elements.set(id, makeEl(id));
  return elements.get(id);
}

const storage = new Map();
globalThis.__GORNESS_TEST_NO_AUTOBOOT__ = true;
globalThis.localStorage = {
  getItem: (k) => storage.get(k) ?? null,
  setItem: (k, v) => storage.set(k, String(v))
};
globalThis.CSS = { escape: (s) => String(s) };
globalThis.document = {
  activeElement: null,
  documentElement: { lang: "zh-CN", style: { setProperty() {} }, setAttribute() {} },
  getElementById: (id) => getEl(id),
  querySelectorAll: () => [],
  querySelector: () => null,
  addEventListener() {}
};
globalThis.window = { innerWidth: 1280, addEventListener() {} };

const origFetch = globalThis.fetch;
globalThis.fetch = (input, init = {}) => {
  const url = typeof input === "string" && input.startsWith("/") ? baseURL + input : input;
  const headers = Object.assign({ Origin: baseURL }, init.headers || {});
  return origFetch(url, Object.assign({}, init, { headers }));
};

const mod = await import(pathToFileURL(%q).href);

// 1. Verify initial Studio loadBootstrap() restores persisted session draft and does NOT wipe it after 450ms timer!
await mod.loadBootstrap();
assert.equal(getEl("composer-input").value, %q, "composer must restore persisted draft on Studio load");
await new Promise((r) => setTimeout(r, 550));
const checkAfterBoot = await (await globalThis.fetch("/api/sessions/" + encodeURIComponent(%q))).json();
assert.equal(checkAfterBoot.meta.draft, %q, "server-side persisted draft must NOT be overwritten with empty string after 450ms debounce");

// 2. Verify Mode A renderChatHistory() renders collapsible nested .subagent-card blocks for both subagent tool delegation and forked child session!
const historyHTML = getEl("chat-history-entries").innerHTML;
assert.match(historyHTML, /class="subagent-card/, "expected nested .subagent-card in Mode A conversation");
assert.match(historyHTML, /Explorer Branch Investigation/, "expected forked child session card in Mode A");
assert.match(historyHTML, /Scan app\.go symbols/, "expected subagent delegation task summary in Mode A");
assert.match(historyHTML, /Found Value\(\) in app\.go:3/, "expected paired subagent result inside nested card");

// 3. Verify failed /api/runs/stream (HTTP 400 invalid provider/model) preserves composer prompt and surfaces error banner/card!
getEl("composer-input").value = "Unsent prompt preserved on HTTP 400";
getEl("model-override-input").value = "unknown_provider/nonexistent_model";
await mod.sendPromptOrSteer();
assert.equal(getEl("composer-input").value, "Unsent prompt preserved on HTTP 400", "prompt must not be lost when /api/runs/stream fails");
assert.equal(getEl("reload-diag-banner").classList.contains("hidden"), false, "diag banner should be visible on HTTP failure");
assert.match(getEl("chat-history-entries").innerHTML, /chat-error-card/, "timeline should render inline chat-error-card");

// 4. Verify mid-stream run_failed SSE event surfaces error persistently after stream closes!
getEl("model-override-input").value = "";
getEl("composer-input").value = "TRIGGER_STREAM_FAIL";
await mod.sendPromptOrSteer();
assert.match(mod.state.lastRunError, /502 authentication failure/, "state.lastRunError must capture run_failed message");
assert.match(getEl("reload-diag-banner").textContent, /502 authentication failure/, "diag banner must retain run_failed message");
assert.match(getEl("chat-history-entries").innerHTML, /502 authentication failure/, "timeline error card must retain run_failed message");

// 5. Verify first auto-created session on a fresh workspace sets state.sessionLoaded = true after stream ends!
mod.state.activeSessionId = "";
mod.state.sessionLoaded = false;
getEl("composer-input").value = "First prompt on empty workspace";
await mod.sendPromptOrSteer();
assert.ok(mod.state.activeSessionId, "auto-created session ID must be adopted");
assert.equal(mod.state.sessionLoaded, true, "state.sessionLoaded must be set to true after first auto-created session stream completes");

// 6. Verify Fleet Studio SSE auto-attach in loadBootstrap(), setCenterMode("fleet"), and [data-open-fleet-mission] when mission is running!
const sseUrls = [];
globalThis.EventSource = class MockEventSource {
  constructor(url) {
    this.url = url;
    this.readyState = 1;
    sseUrls.push(url);
  }
  addEventListener() {}
  close() {
    this.readyState = 2;
  }
};
mod.state.missions = [{ id: "msn_live_99", workspace_id: mod.state.activeWorkspaceId, topology: "hybrid-fleet", goal: "Live mission", status: "running", nodes: [], edges: [] }];
mod.state.activeMissionId = "msn_live_99";
mod.setCenterMode("fleet");
assert.equal(sseUrls.includes("/api/fleet/missions/msn_live_99/stream"), true, "setCenterMode('fleet') must auto-subscribe to running mission SSE");

// Also verify loadBootstrap() auto-subscribes when server bootstrap returns a running mission!
const origFetch2 = globalThis.fetch;
globalThis.fetch = async (input, init = {}) => {
  const resp = await origFetch2(input, init);
  if (typeof input === "string" && input === "/api/bootstrap") {
    const data = await resp.json();
    data.missions = [{ id: "msn_boot_live_1", workspace_id: mod.state.activeWorkspaceId, topology: "hybrid-fleet", goal: "Boot live mission", status: "running", nodes: [], edges: [] }];
    return new Response(JSON.stringify(data), { status: 200, headers: { "Content-Type": "application/json" } });
  }
  return resp;
};
mod.state.activeMissionId = "";
if (mod.state.missionSSE) { mod.state.missionSSE.close(); mod.state.missionSSE = null; mod.state.missionSSEId = ""; }
await mod.loadBootstrap();
assert.equal(sseUrls.includes("/api/fleet/missions/msn_boot_live_1/stream"), true, "loadBootstrap() must auto-subscribe when active mission is running");
`, ts.URL, appJSPath, wantDraft, sess.ID, wantDraft)

	cmd := exec.Command(nodeBin, "--input-type=module", "-e", nodeScript)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("node workbench UI verification failed: %v\n%s", err, string(out))
	}
}

func TestConfigMatrixPatchPreservationSkillFilesAndSessionRunLockGuards(t *testing.T) {
	blockCh := make(chan struct{})
	startedRunCh := make(chan struct{})
	var startOnce sync.Once

	blockingStreamer := llm.StreamerFunc(func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
		var lastText string
		if len(req.History) > 0 {
			for _, b := range req.History[len(req.History)-1].Blocks {
				if b.Text != "" {
					lastText = b.Text
				}
			}
		}
		if strings.Contains(lastText, "HOLD_SESSION_RUN_LOCK") {
			startOnce.Do(func() { close(startedRunCh) })
			select {
			case <-blockCh:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		ch := make(chan llm.StreamEvent, 2)
		ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "done"}
		ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn"}
		close(ch)
		return ch, nil
	})

	srv, vaultDir, cleanup := newTestStack(t, blockingStreamer, nil)
	defer cleanup()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	client := ts.Client()

	// 1. Studio Agent-Model Matrix save MUST preserve read_only, allowed_tools (including empty [] on utility!), disallowed_tools, fallbacks, description, name!
	postJSON := func(method, path string, body any) (int, []byte) {
		t.Helper()
		var rdr io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			rdr = bytes.NewReader(raw)
		}
		req, err := http.NewRequest(method, ts.URL+path, rdr)
		if err != nil {
			t.Fatalf("NewRequest %s %s: %v", method, path, err)
		}
		req.Header.Set("Origin", ts.URL)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Do %s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		out, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, out
	}

	// Matrix save on verifier (omits read_only, allowed_tools, fallbacks)
	if st, raw := postJSON(http.MethodPost, "/api/config/agents", map[string]any{
		"id":       "verifier",
		"provider": "anthropic",
		"model":    "claude-opus-5-5",
		"effort":   "max",
	}); st != http.StatusOK {
		t.Fatalf("upsert verifier status=%d: %s", st, string(raw))
	}

	// Matrix save on utility (omits read_only and empty allowed_tools = []!)
	if st, raw := postJSON(http.MethodPost, "/api/config/agents", map[string]any{
		"id":       "utility",
		"provider": "anthropic",
		"model":    "claude-sonnet-5-5",
		"effort":   "low",
	}); st != http.StatusOK {
		t.Fatalf("upsert utility status=%d: %s", st, string(raw))
	}

	// Reload from disk via fresh config.Manager to verify exact persisted TOML frontmatter!
	reloadedMgr, err := config.NewManager(config.Options{VaultRoot: vaultDir})
	if err != nil {
		t.Fatalf("reload config.Manager: %v", err)
	}
	verProf := reloadedMgr.Current().Agents["verifier"]
	if !verProf.ReadOnly || len(verProf.AllowedTools) != 3 || len(verProf.Fallbacks) != 2 || verProf.Model != "claude-opus-5-5" || verProf.Effort != "max" {
		t.Fatalf("verifier fields wiped by matrix save: %+v", verProf)
	}
	utilProf := reloadedMgr.Current().Agents["utility"]
	if !utilProf.ReadOnly || utilProf.AllowedTools == nil || len(utilProf.AllowedTools) != 0 || utilProf.Model != "claude-sonnet-5-5" {
		t.Fatalf("utility read_only or empty allowed_tools=[] wiped by matrix save: %#v", utilProf)
	}

	// Provider partial patch preservation + concurrent configMu race check
	if st, raw := postJSON(http.MethodPost, "/api/config/providers", map[string]any{
		"id":          "matrix_gw",
		"name":        "Matrix Gateway",
		"protocol":    "openai-compat",
		"base_url":    "http://127.0.0.1:18100/v1",
		"api_key":     "sk-real-matrix-secret",
		"timeout_sec": 95,
		"headers":     map[string]string{"X-Team": "core", "Authorization": "Bearer secret-tok"},
		"models":      map[string]any{"m-alpha": map[string]any{"name": "Alpha"}},
	}); st != http.StatusOK {
		t.Fatalf("seed matrix_gw status=%d: %s", st, string(raw))
	}
	if st, raw := postJSON(http.MethodPost, "/api/config/providers", map[string]any{
		"id":       "matrix_gw",
		"base_url": "http://127.0.0.1:18200/v1",
		"api_key":  "[REDACTED]",
	}); st != http.StatusOK {
		t.Fatalf("patch matrix_gw status=%d: %s", st, string(raw))
	}
	if _, err := reloadedMgr.Reload(); err != nil {
		t.Fatalf("Reload after provider patch: %v", err)
	}
	gw := reloadedMgr.Current().Providers["matrix_gw"]
	if gw.BaseURL != "http://127.0.0.1:18200/v1" || gw.APIKeyRef != "sk-real-matrix-secret" || gw.TimeoutSec != 95 || gw.Headers["X-Team"] != "core" || len(gw.Models) != 1 {
		t.Fatalf("provider patch wiped existing fields: %+v", gw)
	}

	var cfgWG sync.WaitGroup
	for i := 0; i < 6; i++ {
		cfgWG.Add(1)
		go func(idx int) {
			defer cfgWG.Done()
			ef := "high"
			if idx%2 == 0 {
				ef = "medium"
			}
			st, _ := postJSON(http.MethodPost, "/api/config/agents", map[string]any{
				"id":     "coder",
				"effort": ef,
			})
			if st != http.StatusOK {
				t.Errorf("concurrent agent patch failed with status %d", st)
			}
		}(i)
	}
	cfgWG.Wait()

	// 2. skills/*/SKILL.md and skills/_candidates/*/SKILL.md in Vault Editor list & GET/PUT /api/config/files!
	userSkillPath := filepath.Join(vaultDir, "skills", "go-audit", "SKILL.md")
	candSkillPath := filepath.Join(vaultDir, "skills", "_candidates", "dream-audit", "SKILL.md")
	for _, sp := range []string{userSkillPath, candSkillPath} {
		if err := store.AtomicWriteFile(sp, []byte("---\nname: audit\ndescription: Audit skill\n---\n# Body\n"), 0o600); err != nil {
			t.Fatalf("write %s: %v", sp, err)
		}
	}
	stCfg, rawCfg := postJSON(http.MethodGet, "/api/config/files", nil)
	if stCfg != http.StatusOK || !strings.Contains(string(rawCfg), "skills/go-audit/SKILL.md") || !strings.Contains(string(rawCfg), "skills/_candidates/dream-audit/SKILL.md") {
		t.Fatalf("expected skills/*/SKILL.md and skills/_candidates/*/SKILL.md in /api/config/files, got (%d): %s", stCfg, string(rawCfg))
	}
	if stPut, rawPut := postJSON(http.MethodPut, "/api/config/files", map[string]string{
		"rel_path": "skills/_candidates/dream-audit/SKILL.md",
		"content":  "---\nname: dream-audit\ndescription: Updated candidate skill\n---\n# Updated\n",
	}); stPut != http.StatusOK {
		t.Fatalf("PUT candidate SKILL.md failed (%d): %s", stPut, string(rawPut))
	}

	// 3. Rewind & Delete session MUST enforce RunLockPath (409 Conflict during active run) & clean up isolated WorktreePath on delete!
	repoDir := filepath.Join(t.TempDir(), "lock-repo")
	initTempGitRepo(t, repoDir)
	_, wsRaw := postJSON(http.MethodPost, "/api/workspaces", map[string]string{"path": repoDir, "name": "LockRepo"})
	var wsCreated struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(wsRaw, &wsCreated)

	stSess, sessRaw := postJSON(http.MethodPost, "/api/sessions", map[string]any{
		"workspace_id": wsCreated.ID,
		"title":        "Isolated Busy Session",
		"agent_role":   "coder",
		"isolate_git":  true,
	})
	if stSess != http.StatusCreated {
		t.Fatalf("create isolated session status=%d: %s", stSess, string(sessRaw))
	}
	var createdMeta store.SessionMeta
	_ = json.Unmarshal(sessRaw, &createdMeta)
	if createdMeta.WorktreePath == "" {
		t.Fatalf("expected non-empty WorktreePath on isolate_git session: %+v", createdMeta)
	}
	if _, err := os.Stat(createdMeta.WorktreePath); err != nil {
		t.Fatalf("expected isolated worktree directory on disk: %v", err)
	}

	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		_, _ = postJSON(http.MethodPost, "/api/runs/stream", map[string]any{
			"workspace_id": wsCreated.ID,
			"session_id":   createdMeta.ID,
			"prompt":       "HOLD_SESSION_RUN_LOCK",
		})
	}()

	select {
	case <-startedRunCh:
	case <-time.After(4 * time.Second):
		t.Fatal("timed out waiting for active run to acquire session lock")
	}

	if stRew, _ := postJSON(http.MethodPost, "/api/sessions/"+createdMeta.ID+"/rewind", map[string]any{"to_seq": 0}); stRew != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on rewind during active run, got %d", stRew)
	}
	if stDel, _ := postJSON(http.MethodDelete, "/api/sessions/"+createdMeta.ID, nil); stDel != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on delete during active run, got %d", stDel)
	}

	close(blockCh)
	<-runDone

	if stDelOK, rawDelOK := postJSON(http.MethodDelete, "/api/sessions/"+createdMeta.ID, nil); stDelOK != http.StatusOK {
		t.Fatalf("expected 200 OK deleting idle session, got %d: %s", stDelOK, string(rawDelOK))
	}
	if _, err := os.Stat(createdMeta.WorktreePath); !os.IsNotExist(err) {
		t.Fatalf("expected isolated session WorktreePath %s removed on delete, stat err=%v", createdMeta.WorktreePath, err)
	}
}

func TestZeroTypingConfigMatrixRulesMCPAndWorkspaceSuggestions(t *testing.T) {
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node binary not available")
	}

	srv, vaultDir, cleanup := newTestStack(t, nil, nil)
	defer cleanup()

	// Create a discoverable project folder inside the test user home directory
	fakeHome := filepath.Dir(vaultDir)
	sampleProj := filepath.Join(fakeHome, "Projects", "demo-service")
	if err := os.MkdirAll(filepath.Join(sampleProj, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir sampleProj: %v", err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	client := ts.Client()

	doJSON := func(method, path string, body any) (int, []byte) {
		t.Helper()
		var rdr io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			rdr = bytes.NewReader(raw)
		}
		req, err := http.NewRequest(method, ts.URL+path, rdr)
		if err != nil {
			t.Fatalf("NewRequest %s %s: %v", method, path, err)
		}
		req.Header.Set("Origin", ts.URL)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Do %s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		out, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, out
	}

	// 1. Verify /api/config/rules updates rules.toml via structured click-toggle arrays.
	stRules, rawRules := doJSON(http.MethodPost, "/api/config/rules", map[string]any{
		"allowed_domains": []string{"api.anthropic.com", "api.deepseek.com", "github.com"},
		"allow_commands":  []string{"go test*", "npm test*"},
	})
	if stRules != http.StatusOK {
		t.Fatalf("POST /api/config/rules status=%d: %s", stRules, string(rawRules))
	}
	var snapAfterRules config.Snapshot
	if err := json.Unmarshal(rawRules, &snapAfterRules); err != nil {
		t.Fatalf("decode rules snapshot: %v", err)
	}
	if len(snapAfterRules.Rules.AllowedDomains) != 3 || snapAfterRules.Rules.AllowedDomains[1] != "api.deepseek.com" {
		t.Fatalf("unexpected AllowedDomains after /api/config/rules: %+v", snapAfterRules.Rules.AllowedDomains)
	}

	// 2. Verify /api/config/mcp upserts and toggles MCP servers in mcp.toml.
	stMCP, rawMCP := doJSON(http.MethodPost, "/api/config/mcp", map[string]any{
		"id":          "filesystem",
		"transport":   "stdio",
		"command":     "npx",
		"args":        []string{"-y", "@modelcontextprotocol/server-filesystem", "."},
		"enabled":     true,
		"timeout_sec": 60,
	})
	if stMCP != http.StatusOK {
		t.Fatalf("POST /api/config/mcp status=%d: %s", stMCP, string(rawMCP))
	}
	var snapAfterMCP config.Snapshot
	if err := json.Unmarshal(rawMCP, &snapAfterMCP); err != nil {
		t.Fatalf("decode mcp snapshot: %v", err)
	}
	if _, ok := snapAfterMCP.MCP.Servers["filesystem"]; !ok {
		t.Fatalf("expected filesystem MCP server in snapshot, got: %+v", snapAfterMCP.MCP.Servers)
	}

	// 3. Verify GET /api/workspaces/suggestions discovers ~/Projects/demo-service for 1-click mounting.
	stSug, rawSug := doJSON(http.MethodGet, "/api/workspaces/suggestions", nil)
	if stSug != http.StatusOK || !strings.Contains(string(rawSug), "demo-service") {
		t.Fatalf("expected demo-service in /api/workspaces/suggestions (%d): %s", stSug, string(rawSug))
	}

	// 4. Verify Web UI exports zero-typing helpers: applyProviderPreset, applyMatrixPreset, patchAgentField, toggleRuleChip, toggleMCPPreset, and renders cascading dropdowns instead of text inputs!
	appJSPath, err := filepath.Abs(filepath.Join("..", "..", "web", "app.js"))
	if err != nil {
		t.Fatalf("resolve web/app.js: %v", err)
	}
	nodeScript := fmt.Sprintf(`
import assert from "node:assert/strict";
import { pathToFileURL } from "node:url";

const baseURL = %q;
const elements = new Map();
function makeEl(id) {
  return {
    id,
    value: "",
    textContent: "",
    innerHTML: "",
    checked: true,
    disabled: false,
    style: { setProperty() {}, width: "" },
    dataset: {},
    classList: {
      _set: new Set(["hidden"]),
      add(...c) { c.forEach((x) => this._set.add(x)); },
      remove(...c) { c.forEach((x) => this._set.delete(x)); },
      toggle(c, force) {
        if (force === undefined) {
          if (this._set.has(c)) { this._set.delete(c); return false; }
          this._set.add(c); return true;
        }
        if (force) this._set.add(c); else this._set.delete(c);
        return force;
      },
      contains(c) { return this._set.has(c); }
    },
    setAttribute(k, v) { this[k] = v; },
    getAttribute(k) { return this[k] || ""; },
    addEventListener() {}
  };
}
function getEl(id) {
  if (!elements.has(id)) elements.set(id, makeEl(id));
  return elements.get(id);
}

globalThis.__GORNESS_TEST_NO_AUTOBOOT__ = true;
globalThis.localStorage = { getItem: () => null, setItem: () => {} };
globalThis.CSS = { escape: (s) => String(s) };
globalThis.document = {
  activeElement: null,
  documentElement: { lang: "zh-CN", style: { setProperty() {} }, setAttribute() {} },
  getElementById: (id) => getEl(id),
  querySelectorAll: () => [],
  querySelector: () => null,
  addEventListener() {}
};
globalThis.window = { innerWidth: 1280, addEventListener() {} };

const origFetch = globalThis.fetch;
globalThis.fetch = (input, init = {}) => {
  const url = typeof input === "string" && input.startsWith("/") ? baseURL + input : input;
  const headers = Object.assign({ Origin: baseURL }, init.headers || {});
  return origFetch(url, Object.assign({}, init, { headers }));
};

const mod = await import(pathToFileURL(%q).href);
await mod.loadBootstrap();

// Verify model-override-input is populated with <option> items for 1-click selection!
assert.match(getEl("model-override-input").innerHTML, /<option value="anthropic\/claude-sonnet-5-5"/, "model-override-input must be populated with selectable provider/model options");

// Verify 1-click Provider Preset adds DeepSeek with full model catalog without typing!
await mod.applyProviderPreset("deepseek");
assert.ok(mod.state.config.providers.deepseek, "applyProviderPreset('deepseek') must register deepseek provider");
assert.ok(mod.state.config.providers.deepseek.models["deepseek-chat"], "deepseek preset must populate model catalog");

// Verify zero-typing Agent-Model Matrix renders <select> for model (no <input data-agent-model type="text">) and auto-saves via patchAgentRole!
mod.setCenterMode("config");
const studioHTML = getEl("config-studio-content").innerHTML;
assert.match(studioHTML, /data-agent-model-select="coder"/, "Config Studio must render <select> dropdown for agent model");
assert.doesNotMatch(studioHTML, /<input[^>]*data-agent-model/, "Agent Matrix must NOT use raw text input for model selection");

await mod.patchAgentRole("coder", { provider: "deepseek", model: "deepseek-chat", effort: "xhigh" });
assert.equal(mod.state.config.agents.coder.provider, "deepseek", "patchAgentRole must auto-save provider");
assert.equal(mod.state.config.agents.coder.model, "deepseek-chat", "patchAgentRole must auto-save model");
assert.equal(mod.state.config.agents.coder.effort, "xhigh", "patchAgentRole must auto-save effort");
`, ts.URL, appJSPath)

	cmd := exec.Command(nodeBin, "--input-type=module", "-e", nodeScript)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("node zero-typing config matrix test failed: %v\n%s", err, string(out))
	}
}
