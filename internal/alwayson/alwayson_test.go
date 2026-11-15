package alwayson

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/core"
	"gorness.dev/gorness/internal/llm"
	"gorness.dev/gorness/internal/store"
)

type recordedNotification struct {
	Title    string
	Subtitle string
	Body     string
}

type fakeNotifier struct {
	mu    sync.Mutex
	items []recordedNotification
}

func (f *fakeNotifier) NotifyMac(title, subtitle, body string) error {
	f.mu.Lock()
	f.items = append(f.items, recordedNotification{
		Title:    title,
		Subtitle: subtitle,
		Body:     body,
	})
	f.mu.Unlock()
	return nil
}

func (f *fakeNotifier) List() []recordedNotification {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]recordedNotification, len(f.items))
	copy(out, f.items)
	return out
}

type scriptedStreamer struct {
	fn func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error)
}

func (s *scriptedStreamer) Stream(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
	return s.fn(ctx, req)
}

func makeTextStream(text string) <-chan llm.StreamEvent {
	ch := make(chan llm.StreamEvent, 4)
	ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: text}
	ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", InputTokens: 20, OutputTokens: 10}
	close(ch)
	return ch
}

func makeToolCallStream(callID, toolName string, input any) <-chan llm.StreamEvent {
	raw, _ := json.Marshal(input)
	ch := make(chan llm.StreamEvent, 4)
	ch <- llm.StreamEvent{
		Type:       llm.StreamToolCall,
		ToolCallID: callID,
		ToolName:   toolName,
		ToolInput:  raw,
	}
	ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use", InputTokens: 25, OutputTokens: 15}
	close(ch)
	return ch
}

func makeErrorStream(err error) <-chan llm.StreamEvent {
	ch := make(chan llm.StreamEvent, 2)
	ch <- llm.StreamEvent{Type: llm.StreamError, Err: err}
	close(ch)
	return ch
}

func lastPromptText(req llm.Request) string {
	var parts []string
	for _, ent := range req.History {
		if ent.Role != "user" {
			continue
		}
		for _, blk := range ent.Blocks {
			if blk.Type == "text" && strings.TrimSpace(blk.Text) != "" {
				parts = append(parts, blk.Text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func hasToolResultInHistory(req llm.Request, toolName string) (string, bool) {
	for i := len(req.History) - 1; i >= 0; i-- {
		ent := req.History[i]
		if ent.Role != "tool" {
			continue
		}
		for _, blk := range ent.Blocks {
			if blk.Type == "tool_result" && blk.ToolName == toolName {
				return blk.Text, true
			}
		}
	}
	return "", false
}

type alwaysOnTestEnv struct {
	vaultRoot string
	homeDir   string
	wsRoot    string
	ws        *store.Workspace
	cfgMgr    *config.Manager
	engine    *core.Engine
	notifier  *fakeNotifier
}

func setupAlwaysOnEnv(t *testing.T, debounceMS int, streamer llm.Streamer) *alwaysOnTestEnv {
	t.Helper()
	base := t.TempDir()
	vaultRoot := filepath.Join(base, "home", ".gorness")
	homeDir := filepath.Join(base, "home")
	wsRoot := filepath.Join(base, "project-alpha")
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.MkdirAll(wsRoot, 0o755); err != nil {
		t.Fatalf("mkdir wsRoot: %v", err)
	}

	cfgOpts := config.Options{
		VaultRoot:   vaultRoot,
		UserHomeDir: homeDir,
		EnvLookup: func(key string) (string, bool) {
			switch key {
			case "ANTHROPIC_API_KEY":
				return "sk-ant-test", true
			case "OPENAI_API_KEY":
				return "sk-oai-test", true
			case "GEMINI_API_KEY":
				return "gem-test", true
			case "MLX_API_KEY":
				return "mlx-test", true
			default:
				return "", false
			}
		},
	}

	cfgMgr, err := config.NewManager(cfgOpts)
	if err != nil {
		t.Fatalf("config.NewManager: %v", err)
	}

	if debounceMS > 0 {
		cfgPath := cfgMgr.Paths().ConfigFile()
		raw, err := os.ReadFile(cfgPath)
		if err != nil {
			t.Fatalf("read config.toml: %v", err)
		}
		updated := strings.Replace(
			string(raw),
			"inbox_debounce_ms = 400",
			fmt.Sprintf("inbox_debounce_ms = %d", debounceMS),
			1,
		)
		if err := store.AtomicWriteFile(cfgPath, []byte(updated), 0o600); err != nil {
			t.Fatalf("write config.toml: %v", err)
		}
		cfgMgr, err = config.NewManager(cfgOpts)
		if err != nil {
			t.Fatalf("reload config.NewManager: %v", err)
		}
	}

	wsStore, err := store.NewWorkspaceStore(cfgMgr.Paths())
	if err != nil {
		t.Fatalf("NewWorkspaceStore: %v", err)
	}
	ws, err := wsStore.Register(wsRoot, "project-alpha")
	if err != nil {
		t.Fatalf("Register workspace: %v", err)
	}

	sessStore, err := store.NewSessionStore(cfgMgr.Paths())
	if err != nil {
		t.Fatalf("NewSessionStore: %v", err)
	}
	snapStore, err := store.NewSnapshotStore(cfgMgr.Paths())
	if err != nil {
		t.Fatalf("NewSnapshotStore: %v", err)
	}
	router := llm.NewRouter(cfgMgr, cfgMgr.Secrets(), streamer)
	hooks := core.NewHookBus(nil)
	hooks.Freeze()

	eng, err := core.NewEngine(core.EngineOptions{
		Config:     cfgMgr,
		Workspaces: wsStore,
		Sessions:   sessStore,
		Snapshots:  snapStore,
		Router:     router,
		Hooks:      hooks,
		UserHome:   homeDir,
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	t.Cleanup(func() {
		_ = eng.Close()
	})

	return &alwaysOnTestEnv{
		vaultRoot: vaultRoot,
		homeDir:   homeDir,
		wsRoot:    wsRoot,
		ws:        ws,
		cfgMgr:    cfgMgr,
		engine:    eng,
		notifier:  &fakeNotifier{},
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, desc string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for: %s", timeout, desc)
}

func TestInboxRecoverAndIdempotency(t *testing.T) {
	env := setupAlwaysOnEnv(t, 20, &scriptedStreamer{
		fn: func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
			// Utility auto-titler calls can return a crisp title immediately.
			if strings.Contains(req.StaticSystem, "Housekeeping Utility") {
				return makeTextStream("Background Check"), nil
			}
			prompt := lastPromptText(req)
			if strings.Contains(prompt, "stale-crash-item") {
				return makeErrorStream(&llm.APIError{
					ProviderID: req.Provider.ID,
					StatusCode: 400,
					Message:    "simulated unrecoverable payload error",
					Retryable:  false,
				}), nil
			}
			return makeTextStream("Processed webhook cleanly with nothing urgent."), nil
		},
	})

	var staleRunFailures atomic.Int32
	var normalRunCompletions atomic.Int32
	evStream, unsub := env.engine.SubscribeGlobal()
	defer unsub()
	go func() {
		for ev := range evStream {
			switch ev.Type {
			case core.EventRunFailed:
				staleRunFailures.Add(1)
			case core.EventRunCompleted:
				normalRunCompletions.Add(1)
			}
		}
	}()

	// Pre-populate inbox.jsonl with:
	// 1. A stale "running" item (already on attempt 1 when previous process crashed)
	// 2. Two duplicate (source, source_id) lines in inbox.jsonl
	now := time.Now().UTC().Add(-2 * time.Minute)
	seedEvents := []InboxEvent{
		{
			ID:          "ev_stale_1",
			InboxKey:    env.ws.ID + ":stale",
			Source:      "webhook",
			SourceID:    "wh-stale-1",
			WorkspaceID: env.ws.ID,
			AgentRole:   "coder",
			Payload:     map[string]any{"text": "stale-crash-item"},
			Status:      "running",
			Attempts:    1,
			CreatedAt:   now,
		},
		{
			ID:          "ev_dup_1",
			InboxKey:    env.ws.ID + ":dup",
			Source:      "github",
			SourceID:    "delivery-99",
			WorkspaceID: env.ws.ID,
			AgentRole:   "coder",
			Payload:     map[string]any{"text": "first-delivery-99"},
			Status:      "queued",
			Attempts:    0,
			CreatedAt:   now.Add(time.Second),
		},
		{
			ID:          "ev_dup_2",
			InboxKey:    env.ws.ID + ":dup",
			Source:      "github",
			SourceID:    "delivery-99",
			WorkspaceID: env.ws.ID,
			AgentRole:   "coder",
			Payload:     map[string]any{"text": "duplicate-delivery-99"},
			Status:      "queued",
			Attempts:    0,
			CreatedAt:   now.Add(2 * time.Second),
		},
	}

	inboxPath := filepath.Join(env.cfgMgr.Paths().AlwaysOnDir(), "inbox.jsonl")
	var lines []byte
	for _, ev := range seedEvents {
		b, err := json.Marshal(ev)
		if err != nil {
			t.Fatalf("marshal seed event: %v", err)
		}
		lines = append(lines, b...)
		lines = append(lines, '\n')
	}
	if err := store.AtomicWriteFile(inboxPath, lines, 0o600); err != nil {
		t.Fatalf("seed inbox.jsonl: %v", err)
	}

	svc, err := NewService(ServiceOptions{
		Config:       env.cfgMgr,
		Engine:       env.engine,
		Notifier:     env.notifier,
		TickInterval: 20 * time.Millisecond,
		RetryBackoff: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		_ = svc.Close()
	}()

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("svc.Start: %v", err)
	}

	// Second competing Service on the same vault must be rejected by single-active TryFlock consumer lease.
	competingSvc, err := NewService(ServiceOptions{
		Config:   env.cfgMgr,
		Engine:   env.engine,
		Notifier: env.notifier,
	})
	if err != nil {
		t.Fatalf("NewService competing: %v", err)
	}
	if err := competingSvc.Start(ctx); !errors.Is(err, ErrWorkerLeaseHeld) {
		t.Fatalf("expected ErrWorkerLeaseHeld from competing consumer, got %v", err)
	}

	// Enqueueing the same (source, source_id) again via API must return deduped == true.
	deduped, err := svc.Enqueue(InboxEvent{
		Source:      "github",
		SourceID:    "delivery-99",
		WorkspaceID: env.ws.ID,
		Payload:     map[string]any{"text": "api-duplicate-delivery-99"},
	})
	if err != nil {
		t.Fatalf("Enqueue duplicate: %v", err)
	}
	if !deduped {
		t.Fatalf("expected deduped=true for duplicate (source, source_id)")
	}

	waitForCondition(t, 4*time.Second, "stale item to exhaust MaxAttempts=3 and normal item to finish", func() bool {
		snap := svc.StateSnapshot()
		var staleFailed, dupDone bool
		for _, item := range snap.Inbox {
			if item.ID == "ev_stale_1" && item.Status == "failed" && item.Attempts == MaxAttempts {
				staleFailed = true
			}
			if item.ID == "ev_dup_1" && item.Status == "done" && item.Attempts == 1 {
				dupDone = true
			}
		}
		return staleFailed && dupDone
	})

	// Stale item started at Attempts=1, so worker should have executed attempts #2 and #3 (2 StartRun failures) then stopped at MaxAttempts=3.
	if got := staleRunFailures.Load(); got != 2 {
		t.Fatalf("expected 2 retry runs to reach MaxAttempts=3 from Attempts=1, got %d", got)
	}
	if got := normalRunCompletions.Load(); got != 1 {
		t.Fatalf("expected exactly 1 run for deduplicated (github, delivery-99), got %d", got)
	}

	statePath := filepath.Join(env.cfgMgr.Paths().AlwaysOnDir(), "inbox.state.toml")
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("expected atomic state projection %s to exist: %v", statePath, err)
	}
}

func TestMicroBatchingAndSilentVsSendOutbox(t *testing.T) {
	var coderInvocations atomic.Int32
	var mu sync.Mutex
	var capturedPrompts []string

	env := setupAlwaysOnEnv(t, 50, &scriptedStreamer{
		fn: func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
			if strings.Contains(req.StaticSystem, "Housekeeping Utility") {
				return makeTextStream("CI Watcher"), nil
			}
			prompt := lastPromptText(req)

			// Second step after notify_user tool execution: conclude turn cleanly.
			if _, hasNotifyResult := hasToolResultInHistory(req, "notify_user"); hasNotifyResult {
				return makeTextStream("Completed evaluation."), nil
			}

			coderInvocations.Add(1)
			mu.Lock()
			capturedPrompts = append(capturedPrompts, prompt)
			mu.Unlock()

			switch {
			case strings.Contains(prompt, "batch-msg-1"):
				return makeToolCallStream("call_notify_1", "notify_user", map[string]any{
					"title": "Flaky Test Alert",
					"body":  "TestPoolRace failed on darwin/arm64",
				}), nil
			case strings.Contains(prompt, "repeat-alert-trigger"):
				// Model attempts to notify the exact same finding again -> REPORTED.md must suppress it!
				return makeToolCallStream("call_notify_2", "notify_user", map[string]any{
					"title": "Flaky Test Alert",
					"body":  "TestPoolRace failed on darwin/arm64",
				}), nil
			default:
				// Nothing requires user attention -> silent outcome without calling notify_user.
				return makeTextStream("All green, nothing to report."), nil
			}
		},
	})

	// Verify osascript argv builder prevents AppleScript injection by passing untrusted strings after "--".
	maliciousTitle := `Alert "quoted" \ backslash $(touch /tmp/pwn)`
	osaArgs := BuildOSAScriptArgs(maliciousTitle, "project-alpha", `Body with 'single' & "double" quotes`)
	sepIdx := -1
	for i, a := range osaArgs {
		if a == "--" {
			sepIdx = i
			break
		}
		if strings.Contains(a, "touch /tmp/pwn") {
			t.Fatalf("untrusted text leaked into AppleScript -e code argument %q", a)
		}
	}
	if sepIdx < 0 || len(osaArgs) != sepIdx+4 || osaArgs[sepIdx+1] != maliciousTitle {
		t.Fatalf("unexpected osascript argv layout: %#v", osaArgs)
	}

	svc, err := NewService(ServiceOptions{
		Config:       env.cfgMgr,
		Engine:       env.engine,
		Notifier:     env.notifier,
		TickInterval: 15 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		_ = svc.Close()
	}()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("svc.Start: %v", err)
	}

	// 1. Enqueue 3 rapid events within the 50ms debounce window under the same InboxKey.
	for i := 1; i <= 3; i++ {
		deduped, err := svc.Enqueue(InboxEvent{
			InboxKey:    env.ws.ID + ":ci",
			Source:      "local_hook",
			SourceID:    fmt.Sprintf("ev-rapid-%d", i),
			WorkspaceID: env.ws.ID,
			Payload:     map[string]any{"text": fmt.Sprintf("batch-msg-%d", i)},
		})
		if err != nil || deduped {
			t.Fatalf("Enqueue %d: deduped=%v err=%v", i, deduped, err)
		}
	}

	waitForCondition(t, 4*time.Second, "batched run to deliver new finding to Outbox", func() bool {
		snap := svc.StateSnapshot()
		return len(snap.Outbox) >= 1
	})

	if got := coderInvocations.Load(); got != 1 {
		t.Fatalf("expected 3 rapid events within 50ms debounce window to coalesce into 1 StartRun invocation, got %d", got)
	}
	mu.Lock()
	firstPrompt := capturedPrompts[0]
	mu.Unlock()
	for _, want := range []string{"batch-msg-1", "batch-msg-2", "batch-msg-3"} {
		if !strings.Contains(firstPrompt, want) {
			t.Fatalf("expected batched prompt to include %q, got:\n%s", want, firstPrompt)
		}
	}

	snap1 := svc.StateSnapshot()
	if snap1.Outbox[0].Outcome != "send" || !snap1.Outbox[0].Delivered {
		t.Fatalf("expected first finding Outbox Outcome='send' Delivered=true, got %+v", snap1.Outbox[0])
	}
	if notes := env.notifier.List(); len(notes) != 1 || notes[0].Title != "Flaky Test Alert" {
		t.Fatalf("expected 1 macOS notification for new finding, got %+v", notes)
	}

	reportedBytes, err := os.ReadFile(filepath.Join(env.cfgMgr.Paths().MemoryDir(), "REPORTED.md"))
	if err != nil {
		t.Fatalf("read REPORTED.md: %v", err)
	}
	if !strings.Contains(string(reportedBytes), "Flaky Test Alert") {
		t.Fatalf("expected REPORTED.md to record delivered alert, got:\n%s", string(reportedBytes))
	}

	// 2. Enqueue a repeat alert event -> REPORTED.md deduplication must suppress notification and record Outcome=="silent".
	if _, err := svc.Enqueue(InboxEvent{
		InboxKey:    env.ws.ID + ":ci",
		Source:      "local_hook",
		SourceID:    "ev-repeat-4",
		WorkspaceID: env.ws.ID,
		Payload:     map[string]any{"text": "repeat-alert-trigger"},
	}); err != nil {
		t.Fatalf("Enqueue repeat event: %v", err)
	}

	waitForCondition(t, 4*time.Second, "repeat alert run to record silent Outbox item", func() bool {
		return len(svc.StateSnapshot().Outbox) >= 2
	})

	snap2 := svc.StateSnapshot()
	if snap2.Outbox[1].Outcome != "silent" || snap2.Outbox[1].Delivered {
		t.Fatalf("expected repeat finding to be suppressed as Outcome='silent' Delivered=false, got %+v", snap2.Outbox[1])
	}
	if len(env.notifier.List()) != 1 {
		t.Fatalf("expected repeat finding NOT to invoke Notifier again, got %d notifications", len(env.notifier.List()))
	}

	// 3. Enqueue a quiet event where nothing needs attention -> Outcome=="silent".
	if _, err := svc.Enqueue(InboxEvent{
		InboxKey:    env.ws.ID + ":ci",
		Source:      "local_hook",
		SourceID:    "ev-quiet-5",
		WorkspaceID: env.ws.ID,
		Payload:     map[string]any{"text": "routine-green-check"},
	}); err != nil {
		t.Fatalf("Enqueue quiet event: %v", err)
	}

	waitForCondition(t, 4*time.Second, "quiet run to record silent Outbox item", func() bool {
		return len(svc.StateSnapshot().Outbox) >= 3
	})
	snap3 := svc.StateSnapshot()
	if snap3.Outbox[2].Outcome != "silent" {
		t.Fatalf("expected quiet run Outbox Outcome='silent', got %+v", snap3.Outbox[2])
	}
}

func TestNonBlockingDeferredCallAndEventResume(t *testing.T) {
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "release-notes.md")

	var secondEventDone atomic.Bool
	var resumedCompleted atomic.Bool

	env := setupAlwaysOnEnv(t, 20, &scriptedStreamer{
		fn: func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
			if strings.Contains(req.StaticSystem, "Housekeeping Utility") {
				return makeTextStream("Release Task"), nil
			}
			// When resumed after human approval, history ends with the executed write_file tool_result!
			if out, ok := hasToolResultInHistory(req, "write_file"); ok && strings.Contains(out, `"written":true`) {
				resumedCompleted.Store(true)
				return makeTextStream("Release notes written after approval."), nil
			}

			prompt := lastPromptText(req)
			if strings.Contains(prompt, "write-outside-release-notes") {
				// Writing outside workspace in unattended auto mode triggers GateDefer!
				return makeToolCallStream("call_defer_write_1", "write_file", map[string]any{
					"path":    outsideFile,
					"content": "# Release v1.0\nShipped cleanly.\n",
				}), nil
			}
			if strings.Contains(prompt, "unrelated-second-task") {
				secondEventDone.Store(true)
				return makeTextStream("Second unrelated task completed without blocking."), nil
			}
			return makeTextStream("Done."), nil
		},
	})

	svc, err := NewService(ServiceOptions{
		Config:       env.cfgMgr,
		Engine:       env.engine,
		Notifier:     env.notifier,
		TickInterval: 15 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		_ = svc.Close()
	}()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("svc.Start: %v", err)
	}

	// Enqueue Event 1 (will hit GateDefer) and Event 2 (unrelated task on separate InboxKey).
	if _, err := svc.Enqueue(InboxEvent{
		ID:          "ev_defer_1",
		InboxKey:    env.ws.ID + ":release",
		Source:      "webhook",
		SourceID:    "rel-1",
		WorkspaceID: env.ws.ID,
		Payload:     map[string]any{"text": "write-outside-release-notes"},
	}); err != nil {
		t.Fatalf("Enqueue ev_defer_1: %v", err)
	}

	if _, err := svc.Enqueue(InboxEvent{
		ID:          "ev_unrelated_2",
		InboxKey:    env.ws.ID + ":docs",
		Source:      "webhook",
		SourceID:    "docs-2",
		WorkspaceID: env.ws.ID,
		Payload:     map[string]any{"text": "unrelated-second-task"},
	}); err != nil {
		t.Fatalf("Enqueue ev_unrelated_2: %v", err)
	}

	// Verify deferred card is written to alwayson/deferred/<id>.md AND Event 2 finishes immediately without blocking!
	var cards []DeferredCard
	waitForCondition(t, 4*time.Second, "deferred card persisted and second unrelated event processed", func() bool {
		var lErr error
		cards, lErr = svc.ListDeferred()
		if lErr != nil {
			return false
		}
		return len(cards) == 1 && secondEventDone.Load()
	})

	card := cards[0]
	if card.ToolName != "write_file" || card.SessionID == "" || card.WorkspaceID != env.ws.ID {
		t.Fatalf("unexpected DeferredCard: %+v", card)
	}
	if len(card.Hunks) == 0 {
		t.Fatalf("expected staged DiffHunks on DeferredCard for write_file")
	}

	cardPath := filepath.Join(env.cfgMgr.Paths().AlwaysOnDir(), "deferred", card.ID+".md")
	if _, err := os.Stat(cardPath); err != nil {
		t.Fatalf("expected deferred markdown card at %s: %v", cardPath, err)
	}
	if _, err := os.Stat(outsideFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected outsideFile NOT to be written before ResolveDeferred approval")
	}

	// Verify Outbox recorded a "defer" item.
	foundDeferOutbox := false
	for _, ob := range svc.StateSnapshot().Outbox {
		if ob.Outcome == "defer" && ob.SessionID == card.SessionID {
			foundDeferOutbox = true
			break
		}
	}
	if !foundDeferOutbox {
		t.Fatalf("expected Outbox to contain Outcome='defer' for session %s, got %+v", card.SessionID, svc.StateSnapshot().Outbox)
	}

	// Now resolve the deferred card with approval -> resume event enters Inbox, executes write_file, and finishes original session!
	origSessionID := card.SessionID
	if err := svc.ResolveDeferred(card.ID, true, nil); err != nil {
		t.Fatalf("ResolveDeferred: %v", err)
	}

	afterCards, err := svc.ListDeferred()
	if err != nil {
		t.Fatalf("ListDeferred after resolve: %v", err)
	}
	if len(afterCards) != 0 {
		t.Fatalf("expected 0 pending deferred cards after ResolveDeferred, got %d", len(afterCards))
	}

	waitForCondition(t, 4*time.Second, "resumed session to execute deferred tool and complete", func() bool {
		return resumedCompleted.Load()
	})

	writtenBytes, err := os.ReadFile(outsideFile)
	if err != nil {
		t.Fatalf("expected deferred write_file to execute on resume: %v", err)
	}
	if !strings.Contains(string(writtenBytes), "Shipped cleanly.") {
		t.Fatalf("unexpected outsideFile content: %q", string(writtenBytes))
	}

	// Confirm the exact original SessionID has the completed final assistant turn!
	waitForCondition(t, 2*time.Second, "original session final assistant entry", func() bool {
		branch, bErr := env.engine.Sessions().LoadActiveBranch(origSessionID)
		if bErr != nil || len(branch) == 0 {
			return false
		}
		last := branch[len(branch)-1]
		return last.Role == "assistant" && len(last.Blocks) > 0 && strings.Contains(last.Blocks[0].Text, "Release notes written after approval.")
	})
}

func TestTimersAndDreamerCandidateSkill(t *testing.T) {
	var llmCalls atomic.Int32

	env := setupAlwaysOnEnv(t, 20, &scriptedStreamer{
		fn: func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
			if strings.Contains(req.StaticSystem, "Housekeeping Utility") {
				return makeTextStream("Follow-Up Check"), nil
			}
			if _, ok := hasToolResultInHistory(req, "set_followup"); ok {
				return makeTextStream("Scheduled self follow-up."), nil
			}
			if strings.Contains(lastPromptText(req), "schedule-self-followup") {
				return makeToolCallStream("call_fup_1", "set_followup", map[string]any{
					"after_seconds": 300,
					"note":          "re-check deployment canary",
				}), nil
			}
			llmCalls.Add(1)
			// Dreamer response distilling a reusable candidate SKILL.md and pointer notes.
			dreamMD := "---\nname: flock-crash-recovery\ndescription: Safely recover interrupted JSONL queues using syscall.Flock and atomic TOML state projections.\n---\n\n# Flock Crash Recovery\n\n1. Acquire exclusive `syscall.Flock` before mutating state.\n2. Reconcile stale `running` entries up to `MaxAttempts = 3`.\n"
			return makeTextStream(dreamMD), nil
		},
	})

	svc, err := NewService(ServiceOptions{
		Config:   env.cfgMgr,
		Engine:   env.engine,
		Notifier: env.notifier,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// 1. Schedule a due followup and a due cron entry in timers.toml WITHOUT starting the Inbox worker.
	fup, err := svc.ScheduleFollowUp(env.ws.ID, "", -1*time.Second, "verify post-deploy metrics")
	if err != nil {
		t.Fatalf("ScheduleFollowUp: %v", err)
	}
	pastDue := time.Now().UTC().Add(-30 * time.Second)
	if err := svc.UpsertCron(TimerEntry{
		ID:          "cron_nightly_audit",
		WorkspaceID: env.ws.ID,
		Kind:        "cron",
		CronExpr:    "*/5 * * * *",
		Note:        "run dependency vulnerability scan",
		DueAt:       pastDue,
		Enabled:     true,
	}); err != nil {
		t.Fatalf("UpsertCron: %v", err)
	}

	// Tick timers directly -> must move both due entries into Inbox WITHOUT calling LLM directly!
	now := time.Now().UTC()
	enqueued, err := svc.TickTimers(now)
	if err != nil {
		t.Fatalf("TickTimers: %v", err)
	}
	if len(enqueued) != 2 {
		t.Fatalf("expected 2 due timers moved into Inbox, got %d (%+v)", len(enqueued), enqueued)
	}
	if got := llmCalls.Load(); got != 0 {
		t.Fatalf("expected TickTimers NOT to invoke LLM directly, got %d LLM calls", got)
	}

	snap := svc.StateSnapshot()
	if snap.QueuedCount != 2 {
		t.Fatalf("expected 2 queued Inbox events after TickTimers, got %d", snap.QueuedCount)
	}
	for _, tm := range snap.Timers {
		switch tm.ID {
		case fup.ID:
			if tm.Enabled {
				t.Fatalf("expected one-shot followup %s to be disabled after firing", tm.ID)
			}
		case "cron_nightly_audit":
			if !tm.Enabled || !tm.DueAt.After(now) {
				t.Fatalf("expected cron entry to remain enabled and advance DueAt after %s, got %+v", now, tm)
			}
		}
	}

	// Also verify launchd plist generation produces valid XML and user LaunchAgents target path.
	plistBytes, plistPath := svc.GenerateLaunchdPlist("/usr/local/bin/gorness", env.vaultRoot)
	if !strings.Contains(string(plistBytes), "<string>dev.gorness.daemon</string>") ||
		!strings.Contains(string(plistBytes), "/usr/local/bin/gorness") {
		t.Fatalf("unexpected launchd plist XML:\n%s", string(plistBytes))
	}
	if !strings.HasSuffix(plistPath, filepath.Join("Library", "LaunchAgents", "dev.gorness.daemon.plist")) {
		t.Fatalf("unexpected launchd plist target path: %s", plistPath)
	}

	// 2. Seed a completed session with lessons and run RunDreamPass -> verify candidate SKILL.md in skills/_candidates/.
	sess, err := env.engine.Sessions().Create(env.ws.ID, "Fix Flock Queue Recovery", "coder")
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	if _, err := env.engine.Sessions().Append(sess.ID, store.SessionEntry{
		Role:      "user",
		AgentRole: "coder",
		Blocks:    []store.ContentBlock{{Type: "text", Text: "Ensure inbox recovery handles stale running tasks safely with Flock."}},
	}); err != nil {
		t.Fatalf("Append user entry: %v", err)
	}
	if _, err := env.engine.Sessions().Append(sess.ID, store.SessionEntry{
		Role:      "assistant",
		AgentRole: "coder",
		Blocks:    []store.ContentBlock{{Type: "text", Text: "Implemented atomic inbox.state.toml projection under syscall.Flock with MaxAttempts=3."}},
	}); err != nil {
		t.Fatalf("Append assistant entry: %v", err)
	}

	candidates, err := svc.RunDreamPass(context.Background(), env.ws.ID)
	if err != nil {
		t.Fatalf("RunDreamPass: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatalf("expected RunDreamPass to produce at least 1 candidate SKILL.md")
	}

	candSkillPath := candidates[0]
	wantCandRoot := filepath.Join(env.cfgMgr.Paths().SkillsDir(), "_candidates")
	if !strings.HasPrefix(candSkillPath, wantCandRoot+string(filepath.Separator)) || filepath.Base(candSkillPath) != "SKILL.md" {
		t.Fatalf("expected candidate skill under %s/*/SKILL.md, got %s", wantCandRoot, candSkillPath)
	}
	skillRaw, err := os.ReadFile(candSkillPath)
	if err != nil {
		t.Fatalf("read candidate SKILL.md: %v", err)
	}
	meta, body, err := config.ParseFrontmatterMD(skillRaw)
	if err != nil {
		t.Fatalf("ParseFrontmatterMD on candidate SKILL.md: %v", err)
	}
	if fmt.Sprint(meta["name"]) == "" || strings.TrimSpace(body) == "" {
		t.Fatalf("expected candidate SKILL.md to have frontmatter name and body, got meta=%v body=%q", meta, body)
	}

	// 3. Verify model-facing set_followup plugin tool executes cleanly inside unattended Engine.StartRun.
	_, runCh, err := env.engine.StartRun(context.Background(), core.RunRequest{
		WorkspaceID: env.ws.ID,
		Prompt:      "schedule-self-followup",
		Unattended:  true,
	})
	if err != nil {
		t.Fatalf("StartRun(set_followup): %v", err)
	}
	for range runCh {
	}
	foundAgentFollowUp := false
	for _, tm := range svc.StateSnapshot().Timers {
		if tm.Kind == "followup" && tm.Enabled && strings.Contains(tm.Note, "re-check deployment canary") {
			foundAgentFollowUp = true
			break
		}
	}
	if !foundAgentFollowUp {
		t.Fatalf("expected model set_followup tool to persist enabled followup timer, got %+v", svc.StateSnapshot().Timers)
	}
}
