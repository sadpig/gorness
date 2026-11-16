package tui

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"gorness.dev/gorness/internal/alwayson"
	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/core"
	"gorness.dev/gorness/internal/fleet"
	"gorness.dev/gorness/internal/llm"
	"gorness.dev/gorness/internal/server"
	"gorness.dev/gorness/internal/store"
	"gorness.dev/gorness/internal/tools"
)

func TestTUIFirstSessionAdoptionInlineCommandApprovalAndSSEReconnect(t *testing.T) {
	baseDir := t.TempDir()
	homeDir := filepath.Join(baseDir, "home")
	vaultDir := filepath.Join(homeDir, ".gorness")
	wsDir := filepath.Join(baseDir, "brand-new-ws")
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		t.Fatalf("mkdir homeDir: %v", err)
	}
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("mkdir wsDir: %v", err)
	}

	cfgMgr, err := config.NewManager(config.Options{
		VaultRoot:   vaultDir,
		UserHomeDir: homeDir,
		EnvLookup:   func(k string) (string, bool) { return "test-secret-" + k, true },
	})
	if err != nil {
		t.Fatalf("config.NewManager: %v", err)
	}

	var step atomic.Int32
	streamer := llm.StreamerFunc(func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
		if strings.Contains(req.StaticSystem, "Housekeeping Utility") {
			ch := make(chan llm.StreamEvent, 2)
			ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "First Session"}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn"}
			close(ch)
			return ch, nil
		}
		ch := make(chan llm.StreamEvent, 4)
		if step.Add(1) == 1 {
			ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Preparing deployment push..."}
			ch <- llm.StreamEvent{
				Type:       llm.StreamToolCall,
				ToolCallID: "call_tui_cmd_1",
				ToolName:   "run_command",
				ToolInput:  json.RawMessage(`{"command":"echo tui-approved-ok"}`),
			}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use", InputTokens: 20, OutputTokens: 10}
			close(ch)
			return ch, nil
		}
		ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Command completed after TUI approval."}
		ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", InputTokens: 30, OutputTokens: 12}
		close(ch)
		return ch, nil
	})

	router := llm.NewRouter(cfgMgr, cfgMgr.Secrets(), streamer)
	eng, err := core.NewEngine(core.EngineOptions{
		Config:   cfgMgr,
		Router:   router,
		UserHome: homeDir,
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer func() { _ = eng.Close() }()

	ws, err := eng.Workspaces().Register(wsDir, "BrandNewWS")
	if err != nil {
		t.Fatalf("Register ws: %v", err)
	}
	// Set permission_mode = "ask" so run_command triggers interactive GateAsk on the very first prompt!
	rawCfg, _ := os.ReadFile(cfgMgr.Paths().ConfigFile())
	askCfg := strings.Replace(string(rawCfg), `permission_mode = "auto"`, `permission_mode = "ask"`, 1)
	if err := os.WriteFile(cfgMgr.Paths().ConfigFile(), []byte(askCfg), 0o600); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}
	if _, err := cfgMgr.Reload(); err != nil {
		t.Fatalf("Reload config: %v", err)
	}

	srv := server.NewServer(server.Deps{
		Config:            cfgMgr,
		Engine:            eng,
		HeartbeatInterval: 100 * time.Millisecond,
	})
	defer func() { _ = srv.Close() }()

	// Wrap handler to simulate a transient first SSE 502 failure so runGlobalSSELoop exercises backoff reconnect!
	var sseAttempts atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/events/stream" && sseAttempts.Add(1) == 1 {
			http.Error(w, "simulated transient SSE drop", http.StatusBadGateway)
			return
		}
		srv.Handler().ServeHTTP(w, r)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewModel(ctx, ts.URL, wsDir)
	m.width = 120
	m.height = 36

	// 1. Bootstrap on brand-new workspace with ZERO existing sessions -> activeSessionID is "".
	bootMsg := m.initBootstrapCmd()()
	_, _ = m.Update(bootMsg)
	if m.activeWsID != ws.ID || m.activeSessionID != "" {
		t.Fatalf("expected activeWsID=%q and empty activeSessionID on new workspace, got ws=%q sess=%q", ws.ID, m.activeWsID, m.activeSessionID)
	}

	// Start SSE worker (attempt #1 gets 502, attempt #2 reconnects after 100ms backoff!).
	go m.runGlobalSSELoop()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && sseAttempts.Load() < 2 {
		time.Sleep(20 * time.Millisecond)
	}
	if sseAttempts.Load() < 2 {
		t.Fatalf("expected startGlobalSSEWorkerCmd backoff loop to reconnect after transient 502, attempts=%d", sseAttempts.Load())
	}

	// 2. Dispatch first prompt on empty workspace (`m.activeSessionID == ""`).
	m.input = "Deploy first turn"
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	runResultCh := make(chan tea.Msg, 1)
	go func() {
		runResultCh <- cmd()
	}()

	// Pump SSE events into m.Update until EventApprovalRequired surfaces and adopts the auto-created SessionID!
	waitApprovalDeadline := time.After(5 * time.Second)
	for m.currentApproval() == nil {
		select {
		case ev := <-m.sseCh:
			_, tickCmd := m.Update(globalSSEEventMsg{ev: ev})
			if tickCmd != nil {
				// Ensure settle tick command is non-nil when approval surfaces.
			}
		case <-waitApprovalDeadline:
			t.Fatalf("timed out waiting for first-session approval in TUI (activeSessionID=%q)", m.activeSessionID)
		}
	}

	if m.activeSessionID == "" {
		t.Fatal("expected TUI to adopt auto-created SessionID on brand-new workspace")
	}
	if m.focus != focusApproval {
		t.Fatalf("expected focus to switch to focusApproval, got %v", m.focus)
	}

	// 3. Verify renderInlineApproval displays Reason AND compact Args (not a blind command box!).
	viewBeforeSettle := m.View()
	if !strings.Contains(viewBeforeSettle, "Reason:") || !strings.Contains(viewBeforeSettle, `{"command":"echo tui-approved-ok"}`) {
		t.Fatalf("expected inline approval box to show Reason and compact command Args, got:\n%s", viewBeforeSettle)
	}
	if !strings.Contains(viewBeforeSettle, "Preparing deployment push...") {
		t.Fatalf("expected live streaming text visible on adopted first session, got:\n%s", viewBeforeSettle)
	}

	// 4. Wait past 500ms SettleWindow, verify tick refresh updates View(), then press 'y' to approve and unblock the run!
	time.Sleep(core.SettleWindow + 40*time.Millisecond)
	_, _ = m.Update(approvalSettledTickMsg{approvalID: m.currentApproval().ID})
	viewAfterSettle := m.View()
	if !strings.Contains(viewAfterSettle, "y:approve selected") {
		t.Fatalf("expected settled approval controls after SettleWindow tick, got:\n%s", viewAfterSettle)
	}

	_, resolveCmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if resolveCmd == nil {
		t.Fatal("expected non-nil resolveApprovalCmd on pressing 'y'")
	}
	resMsg := resolveCmd()
	if sn, ok := resMsg.(statusNoticeMsg); !ok || sn.err != nil {
		t.Fatalf("expected clean approval resolution, got %#v", resMsg)
	}

	select {
	case doneMsg := <-runResultCh:
		if sn, ok := doneMsg.(statusNoticeMsg); !ok || sn.err != nil {
			t.Fatalf("expected first-session run to complete cleanly after TUI approval, got %#v", doneMsg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first-session run remained deadlocked after resolving approval")
	}
}

type capturedHTTPReq struct {
	Method string
	Path   string
	Query  string
	Body   map[string]any
}

func TestTUIWorkspaceSwitcherDynamicRolesCanonicalVerbsAndUDS(t *testing.T) {
	shortDir, err := os.MkdirTemp("/tmp", "gtui-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(shortDir) })
	sockPath := filepath.Join(shortDir, "d.sock")

	var mu sync.Mutex
	var captured []capturedHTTPReq
	record := func(r *http.Request) map[string]any {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &body)
		}
		mu.Lock()
		captured = append(captured, capturedHTTPReq{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Body:   body,
		})
		mu.Unlock()
		return body
	}

	workspaces := []*store.Workspace{
		{ID: "ws_alpha", Name: "AlphaService", RootPath: "/tmp/alpha"},
		{ID: "ws_beta", Name: "BetaFrontend", RootPath: "/tmp/beta"},
	}
	sessions := []*store.SessionMeta{
		{ID: "sess_alpha_1", WorkspaceID: "ws_alpha", Title: "Alpha API work", AgentRole: "coder", HeadSeq: 2},
		{ID: "sess_beta_1", WorkspaceID: "ws_beta", Title: "Beta UI polish", AgentRole: "coder", HeadSeq: 1},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		_ = json.NewEncoder(w).Encode(bootstrapPayload{
			Version:   config.Version,
			VaultRoot: shortDir,
			Config: &config.Snapshot{
				Agents: map[string]config.AgentProfile{
					"coder":            {Provider: "anthropic", Model: "claude-sonnet-4"},
					"security_auditor": {Provider: "anthropic", Model: "claude-opus-4"},
				},
			},
			Workspaces: workspaces,
			Sessions:   sessions,
		})
	})
	mux.HandleFunc("/api/workspaces", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		_ = json.NewEncoder(w).Encode(workspaces)
	})
	mux.HandleFunc("/api/workspaces/ws_beta", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		if r.Method != http.MethodPatch {
			http.Error(w, "expected PATCH", http.StatusMethodNotAllowed)
			return
		}
		_ = json.NewEncoder(w).Encode(workspaces[1])
	})
	mux.HandleFunc("/api/sessions/sess_alpha_1", func(w http.ResponseWriter, r *http.Request) {
		body := record(r)
		if r.Method == http.MethodPatch {
			if role, ok := body["agent_role"].(string); ok {
				sessions[0].AgentRole = role
			}
			_ = json.NewEncoder(w).Encode(sessions[0])
			return
		}
		_ = json.NewEncoder(w).Encode(sessionDetailPayload{Meta: sessions[0]})
	})
	mux.HandleFunc("/api/sessions/sess_beta_1", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		_ = json.NewEncoder(w).Encode(sessionDetailPayload{Meta: sessions[1]})
	})
	mux.HandleFunc("/api/alwayson", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		_ = json.NewEncoder(w).Encode(alwayson.AlwaysOnState{
			QueuedCount:   3,
			DeferredCount: 2,
		})
	})
	mux.HandleFunc("/api/runs/run_live_99/steer", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("Listen unix: %v", err)
	}
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect over unix:// socket via daemonctl!
	m := NewModel(ctx, "unix://"+sockPath, "")
	m.width = 120
	m.height = 34

	bootMsg := m.initBootstrapCmd()()
	_, loadCmd := m.Update(bootMsg)
	if loadCmd != nil {
		_, _ = m.Update(loadCmd())
	}
	if m.activeWsID != "ws_alpha" || m.activeSessionID != "sess_alpha_1" {
		t.Fatalf("unexpected initial selection ws=%q sess=%q", m.activeWsID, m.activeSessionID)
	}

	// 1. Dynamic role cycling from boot.Config.Agents (coder -> security_auditor -> coder)
	_, roleCmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlR})
	if m.agentRole != "security_auditor" {
		t.Fatalf("expected dynamic role cycle to security_auditor, got %q", m.agentRole)
	}
	if roleCmd == nil {
		t.Fatal("expected PATCH /api/sessions/sess_alpha_1 command on Ctrl+R")
	}
	_, _ = m.Update(roleCmd())

	// 2. Ctrl+W interactive workspace switcher -> GET /api/workspaces -> navigate to BetaFrontend -> Enter
	_, wsFetchCmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlW})
	if !m.wsSwitcherOpen || wsFetchCmd == nil {
		t.Fatal("expected Ctrl+W to open workspace switcher and fetch GET /api/workspaces")
	}
	_, _ = m.Update(wsFetchCmd())
	if !strings.Contains(m.View(), "Switch Workspace") || !strings.Contains(m.View(), "BetaFrontend") {
		t.Fatalf("expected workspace switcher overlay in View(), got:\n%s", m.View())
	}
	_, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	_, switchCmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.wsSwitcherOpen {
		t.Fatal("expected Enter to close workspace switcher")
	}
	if m.activeWsID != "ws_beta" || m.activeSessionID != "sess_beta_1" {
		t.Fatalf("expected switched workspace ws_beta and session sess_beta_1, got ws=%q sess=%q", m.activeWsID, m.activeSessionID)
	}
	if switchCmd != nil {
		_ = switchCmd
	}

	// 3. /perm uses canonical PATCH /api/workspaces/{wsID}
	_, permCmd := m.dispatchComposerInput("/perm accept_edits")
	if permCmd == nil {
		t.Fatal("expected /perm command")
	}
	if sn, ok := permCmd().(statusNoticeMsg); !ok || sn.err != nil {
		t.Fatalf("/perm failed: %#v", sn)
	}

	// 4. /alwayson queries GET /api/alwayson and updates footer summary
	_, aoCmd := m.dispatchComposerInput("/alwayson")
	_, _ = m.Update(aoCmd())
	if !strings.Contains(m.View(), "Always-On: 3 queued · 2 deferred") {
		t.Fatalf("expected updated Always-On counts in View(), got:\n%s", m.View())
	}

	// 5. Mid-run steer posts {"instruction": ...} to POST /api/runs/{id}/steer
	m.liveRunID = "run_live_99"
	_, steerCmd := m.dispatchComposerInput("Focus on unit tests first")
	if steerCmd == nil {
		t.Fatal("expected steer command when run is active")
	}
	if sn, ok := steerCmd().(statusNoticeMsg); !ok || sn.err != nil {
		t.Fatalf("steer failed: %#v", sn)
	}

	mu.Lock()
	defer mu.Unlock()
	var sawPatchSess, sawGetWS, sawPatchWS, sawGetAO, sawSteer bool
	for _, req := range captured {
		switch {
		case req.Method == http.MethodPatch && req.Path == "/api/sessions/sess_alpha_1" && req.Body["agent_role"] == "security_auditor":
			sawPatchSess = true
		case req.Method == http.MethodGet && req.Path == "/api/workspaces":
			sawGetWS = true
		case req.Method == http.MethodPatch && req.Path == "/api/workspaces/ws_beta" && req.Body["perm_mode"] == "accept_edits":
			sawPatchWS = true
		case req.Method == http.MethodGet && req.Path == "/api/alwayson":
			sawGetAO = true
		case req.Method == http.MethodPost && req.Path == "/api/runs/run_live_99/steer" && req.Body["instruction"] == "Focus on unit tests first":
			sawSteer = true
		}
	}
	if !sawPatchSess || !sawGetWS || !sawPatchWS || !sawGetAO || !sawSteer {
		t.Fatalf("missing expected canonical API requests: patchSess=%v getWS=%v patchWS=%v getAO=%v steer=%v\nall=%#v",
			sawPatchSess, sawGetWS, sawPatchWS, sawGetAO, sawSteer, captured)
	}
}

func TestTUIScrollableDiffHunkApprovalRememberAndFeedback(t *testing.T) {
	var resolvedBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/approvals/appr_diff_6/resolve" {
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &resolvedBody)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
			return
		}
		_ = json.NewEncoder(w).Encode(bootstrapPayload{})
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewModel(ctx, ts.URL, "")
	m.width = 120
	m.height = 28
	m.activeWsID = "ws_1"
	m.activeSessionID = "sess_1"

	appr := core.PendingApproval{
		ID:          "appr_diff_6",
		WorkspaceID: "ws_1",
		SessionID:   "sess_1",
		ToolName:    "edit_file",
		ReasonCode:  "write_outside_auto",
		Reason:      "Multi-hunk file edit requires review",
		Hunks: []tools.DiffHunk{
			{ID: "h1", Header: "@@ -1,3 +1,4 @@", Preview: "-old1\n+new1_alpha"},
			{ID: "h2", Header: "@@ -10,3 +11,4 @@", Preview: "-old2\n+new2_beta"},
			{ID: "h3", Header: "@@ -20,3 +22,4 @@", Preview: "-old3\n+new3_gamma"},
			{ID: "h4", Header: "@@ -30,3 +33,4 @@", Preview: "-old4\n+new4_delta"},
			{ID: "h5", Header: "@@ -40,3 +44,4 @@", Preview: "-old5\n+new5_epsilon"},
			{ID: "h6", Header: "@@ -50,3 +55,4 @@", Preview: "-old6\n+new6_zeta_tail"},
		},
	}
	m.ensureApprovalTracked(appr)
	m.surfacedLocal[appr.ID] = time.Now().Add(-time.Second)
	m.upsertApproval(appr)
	m.focus = focusApproval

	initialView := m.View()
	if strings.Contains(initialView, "more hunks") {
		t.Fatalf("expected full scrollable diff hunks without truncation placeholder, got:\n%s", initialView)
	}

	// Move cursor down to h2 and press Space to deselect h2
	_, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	_, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if m.selectedHunks[appr.ID]["h2"] {
		t.Fatal("expected h2 to be deselected after Space")
	}

	// Navigate down to h6 and verify scrolling brings h6 preview into viewport
	for i := 0; i < 4; i++ {
		_, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	}
	_, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	scrolledView := m.View()
	if !strings.Contains(scrolledView, "+new6_zeta_tail") {
		t.Fatalf("expected scrolling to reveal 6th hunk '+new6_zeta_tail', got:\n%s", scrolledView)
	}

	// Toggle Remember ('r') and enter text feedback ('f' -> runes -> Enter)
	_, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	_, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	_, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("skip hunk 2")})
	_, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})

	if !strings.Contains(m.View(), "Remember: [x]") || !strings.Contains(m.View(), "skip hunk 2") {
		t.Fatalf("expected Remember [x] and feedback rendered in approval card, got:\n%s", m.View())
	}

	// Approve selected ('y')
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("expected resolveApprovalCmd")
	}
	msg := cmd()
	_, _ = m.Update(msg)

	if got := resolvedBody["action"]; got != string(core.ApprovalActionApproveHunks) {
		t.Fatalf("action = %v, want %q", got, core.ApprovalActionApproveHunks)
	}
	if got := resolvedBody["remember"]; got != true {
		t.Fatalf("remember = %v, want true", got)
	}
	if got := resolvedBody["feedback"]; got != "skip hunk 2" {
		t.Fatalf("feedback = %v, want 'skip hunk 2'", got)
	}
	rawIDs, _ := json.Marshal(resolvedBody["selected_hunk_ids"])
	var gotIDs []string
	_ = json.Unmarshal(rawIDs, &gotIDs)
	wantIDs := []string{"h1", "h3", "h4", "h5", "h6"}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("selected_hunk_ids = %v, want %v", gotIDs, wantIDs)
	}
}

func TestTUISlashCommandsRewindWorktreeLandFleetArchiveDelete(t *testing.T) {
	var mu sync.Mutex
	var captured []capturedHTTPReq
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &body)
		}
		mu.Lock()
		captured = append(captured, capturedHTTPReq{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Body:   body,
		})
		mu.Unlock()

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/sessions":
			_ = json.NewEncoder(w).Encode(store.SessionMeta{
				ID:           "sess_wt_2",
				WorkspaceID:  "ws_main",
				Title:        "Worktree Fix",
				WorktreePath: "/tmp/worktree/sess_wt_2",
				HeadSeq:      3,
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/land"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"commit_sha": "abcdef1234567890",
				"branch":     "main",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/fleet/missions":
			_ = json.NewEncoder(w).Encode(fleet.MissionSnapshot{
				ID:           "mission_42",
				WorkspaceID:  "ws_main",
				WorkflowID:   body["workflow_id"].(string),
				Topology:     fleet.TopoReviewLoop,
				Goal:         body["goal"].(string),
				Status:       fleet.MissionRunning,
				TotalCostUSD: 0.125,
				Nodes: []fleet.FleetNode{
					{ID: "coder", Status: fleet.NodeCompleted},
					{ID: "reviewer", Status: fleet.NodeRunning},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		}
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewModel(ctx, ts.URL, "")
	m.activeWsID = "ws_main"
	m.activeSessionID = "sess_1"
	m.boot.Config = &config.Snapshot{
		Workflows: map[string]config.WorkflowTemplate{
			"review-loop": {Topology: "review-loop"},
		},
	}
	m.boot.Sessions = []*store.SessionMeta{
		{ID: "sess_1", WorkspaceID: "ws_main", Title: "Primary", HeadSeq: 1},
		{ID: "sess_keep", WorkspaceID: "ws_main", Title: "Backup", HeadSeq: 2},
	}
	m.entries = []store.SessionEntry{{Seq: 1, Role: "user"}}

	// 1. Bare /rewind at seq #1 must NOT implicitly send to_seq=0
	_, rwAtOneCmd := m.dispatchComposerInput("/rewind")
	rwAtOneMsg := rwAtOneCmd()
	if sn, ok := rwAtOneMsg.(statusNoticeMsg); !ok || sn.err == nil {
		t.Fatalf("expected bare /rewind at #1 to guard against implicit clear, got %#v", rwAtOneMsg)
	}

	// Advance session to seq #3: bare /rewind targets maxSeq - 1 == 2 with restore_files: true
	m.entries = []store.SessionEntry{{Seq: 1}, {Seq: 2}, {Seq: 3}}
	_, rwPrevCmd := m.dispatchComposerInput("/rewind")
	if sn, ok := rwPrevCmd().(statusNoticeMsg); !ok || sn.err != nil {
		t.Fatalf("expected bare /rewind at #3 to succeed, got %#v", sn)
	}

	// Explicit /rewind 0 clears session (to_seq: 0, restore_files: true)
	_, rwZeroCmd := m.dispatchComposerInput("/rewind 0")
	if sn, ok := rwZeroCmd().(statusNoticeMsg); !ok || sn.err != nil {
		t.Fatalf("expected explicit /rewind 0 to succeed, got %#v", sn)
	}

	// /rewind 1 --no-files sends restore_files: false
	_, rwNoFilesCmd := m.dispatchComposerInput("/rewind 1 --no-files")
	if sn, ok := rwNoFilesCmd().(statusNoticeMsg); !ok || sn.err != nil {
		t.Fatalf("expected /rewind 1 --no-files to succeed, got %#v", sn)
	}

	// 2. /new --worktree Worktree Fix -> POST /api/sessions with isolate_git: true
	_, newWtCmd := m.dispatchComposerInput("/new --worktree Worktree Fix")
	_, _ = m.Update(newWtCmd())
	if m.activeSessionID != "sess_wt_2" {
		t.Fatalf("expected activeSessionID=sess_wt_2, got %q", m.activeSessionID)
	}

	// 3. /land go test ./... -> POST /api/sessions/sess_wt_2/land
	_, landCmd := m.dispatchComposerInput("/land go test ./...")
	landMsg := landCmd()
	_, _ = m.Update(landMsg)
	if !strings.Contains(m.statusNotice, "abcdef12") {
		t.Fatalf("expected landed commit SHA in statusNotice, got %q", m.statusNotice)
	}

	// 4. /fleet review-loop Audit auth handlers -> POST /api/fleet/missions + live status summary
	_, fleetCmd := m.dispatchComposerInput("/fleet review-loop Audit auth handlers")
	_, _ = m.Update(fleetCmd())
	if !strings.Contains(m.statusNotice, "Mission mission_42 [review-loop · running] 1/2 nodes") {
		t.Fatalf("expected live mission summary in statusNotice, got %q", m.statusNotice)
	}
	if !strings.Contains(m.View(), "Fleet: 1 live (review-loop 1/2 nodes") {
		t.Fatalf("expected live mission progress in footer bar, got:\n%s", m.View())
	}

	// 5. /archive -> PATCH /api/sessions/sess_wt_2 {"archived": true}
	_, archCmd := m.dispatchComposerInput("/archive")
	_, _ = m.Update(archCmd())
	if m.activeSessionID == "sess_wt_2" {
		t.Fatal("expected archived session to be deselected")
	}

	// 6. /delete -> DELETE /api/sessions/{active}
	targetDeleteID := m.activeSessionID
	_, delCmd := m.dispatchComposerInput("/delete")
	_, _ = m.Update(delCmd())

	mu.Lock()
	defer mu.Unlock()

	var rewindCalls []map[string]any
	var sawIsolateGit, sawLand, sawFleet, sawArchive, sawDelete bool
	for _, req := range captured {
		switch {
		case req.Method == http.MethodPost && req.Path == "/api/sessions/sess_1/rewind":
			rewindCalls = append(rewindCalls, req.Body)
		case req.Method == http.MethodPost && req.Path == "/api/sessions" && req.Body["isolate_git"] == true:
			sawIsolateGit = true
		case req.Method == http.MethodPost && req.Path == "/api/sessions/sess_wt_2/land" && req.Body["verify_command"] == "go test ./...":
			sawLand = true
		case req.Method == http.MethodPost && req.Path == "/api/fleet/missions" &&
			req.Body["workflow_id"] == "review-loop" && req.Body["goal"] == "Audit auth handlers":
			sawFleet = true
		case req.Method == http.MethodPatch && req.Path == "/api/sessions/sess_wt_2" && req.Body["archived"] == true:
			sawArchive = true
		case req.Method == http.MethodDelete && req.Path == "/api/sessions/"+targetDeleteID:
			sawDelete = true
		}
	}

	if len(rewindCalls) != 3 {
		t.Fatalf("expected exactly 3 rewind HTTP calls (at #1 blocked locally), got %d: %#v", len(rewindCalls), rewindCalls)
	}
	if rewindCalls[0]["to_seq"] != float64(2) || rewindCalls[0]["restore_files"] != true {
		t.Fatalf("bare /rewind body = %#v, want to_seq=2 restore_files=true", rewindCalls[0])
	}
	if rewindCalls[1]["to_seq"] != float64(0) || rewindCalls[1]["restore_files"] != true {
		t.Fatalf("/rewind 0 body = %#v, want to_seq=0 restore_files=true", rewindCalls[1])
	}
	if rewindCalls[2]["to_seq"] != float64(1) || rewindCalls[2]["restore_files"] != false {
		t.Fatalf("/rewind 1 --no-files body = %#v, want to_seq=1 restore_files=false", rewindCalls[2])
	}
	if !sawIsolateGit || !sawLand || !sawFleet || !sawArchive || !sawDelete {
		t.Fatalf("missing expected slash command API calls: isolateGit=%v land=%v fleet=%v archive=%v delete=%v\nall=%#v",
			sawIsolateGit, sawLand, sawFleet, sawArchive, sawDelete, captured)
	}
}
