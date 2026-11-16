package tui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/core"
	"gorness.dev/gorness/internal/llm"
	"gorness.dev/gorness/internal/server"
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
