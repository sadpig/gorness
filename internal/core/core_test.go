package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/llm"
	"gorness.dev/gorness/internal/store"
	"gorness.dev/gorness/internal/tools"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(initial time.Time) *fakeClock {
	return &fakeClock{now: initial}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	return c.now
}

func setupTestEngine(t *testing.T, clock *fakeClock, streamer llm.Streamer) (*Engine, *config.Manager, string, string) {
	t.Helper()
	baseDir := t.TempDir()
	vaultRoot := filepath.Join(baseDir, "vault")
	homeDir := filepath.Join(baseDir, "home")
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		t.Fatalf("mkdir homeDir: %v", err)
	}

	nowFn := time.Now
	if clock != nil {
		nowFn = clock.Now
	}

	cfgMgr, err := config.NewManager(config.Options{
		VaultRoot:   vaultRoot,
		UserHomeDir: homeDir,
		EnvLookup: func(key string) (string, bool) {
			switch key {
			case "ANTHROPIC_API_KEY":
				return "sk-ant-test-key", true
			case "OPENAI_API_KEY":
				return "sk-openai-test-key", true
			case "GEMINI_API_KEY":
				return "gem-test-key", true
			case "MLX_API_KEY":
				return "mlx-test-key", true
			default:
				return "", false
			}
		},
		Now: nowFn,
	})
	if err != nil {
		t.Fatalf("config.NewManager: %v", err)
	}

	wsStore, err := store.NewWorkspaceStore(cfgMgr.Paths())
	if err != nil {
		t.Fatalf("store.NewWorkspaceStore: %v", err)
	}
	sessStore, err := store.NewSessionStore(cfgMgr.Paths())
	if err != nil {
		t.Fatalf("store.NewSessionStore: %v", err)
	}
	snapStore, err := store.NewSnapshotStore(cfgMgr.Paths())
	if err != nil {
		t.Fatalf("store.NewSnapshotStore: %v", err)
	}

	router := llm.NewRouter(cfgMgr, cfgMgr.Secrets(), streamer)

	eng, err := NewEngine(EngineOptions{
		Config:     cfgMgr,
		Workspaces: wsStore,
		Sessions:   sessStore,
		Snapshots:  snapStore,
		Router:     router,
		UserHome:   homeDir,
		Now:        nowFn,
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	t.Cleanup(func() {
		_ = eng.Close()
	})
	return eng, cfgMgr, vaultRoot, homeDir
}

func drainRunEvents(ch <-chan RunEvent) []RunEvent {
	var out []RunEvent
	for ev := range ch {
		out = append(out, ev)
	}
	return out
}

func TestMicrokernelScopeLIFOUnwindAndZeroOverheadFrozenHooks(t *testing.T) {
	wsRoot := t.TempDir()
	vaultRoot := filepath.Join(t.TempDir(), "vault")
	paths := config.VaultPaths{Root: vaultRoot}

	reg, err := tools.NewRegistry(tools.RegistryOptions{
		WorkspaceRoot: wsRoot,
		VaultPaths:    paths,
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	bus := NewHookBus(reg)

	// 1. Mounting a plugin whose dependency is not yet mounted must fail cleanly.
	err = bus.MountPlugin(Plugin{
		ID:        "orphan",
		DependsOn: []string{"missing_base"},
		Setup:     func(sc *Scope) error { return nil },
	})
	if err == nil {
		t.Fatal("expected error when mounting plugin with missing dependency")
	}

	// 2. Partial setup failure must unwind already-registered tools and cleanups in LIFO order.
	var failedOrder []string
	err = bus.MountPlugin(Plugin{
		ID: "broken_plugin",
		Setup: func(sc *Scope) error {
			sc.OnCleanup(func() { failedOrder = append(failedOrder, "broken:cleanup1") })
			sc.RegisterTool(tools.Tool{
				Name:        "temp_broken_tool",
				Description: "Should rollback on setup error",
				ReadOnly:    true,
				Handler: func(ctx context.Context, call tools.CallContext, rawInput json.RawMessage) (tools.ToolResult, error) {
					return tools.ToolResult{Output: "ok"}, nil
				},
			})
			sc.OnCleanup(func() { failedOrder = append(failedOrder, "broken:cleanup2") })
			return errors.New("boom during setup")
		},
	})
	if err == nil {
		t.Fatal("expected broken_plugin setup error")
	}
	if _, exists := reg.Get("temp_broken_tool"); exists {
		t.Fatal("expected temp_broken_tool to be rolled back when Setup failed")
	}
	if strings.Join(failedOrder, ",") != "broken:cleanup2,broken:cleanup1" {
		t.Fatalf("expected LIFO rollback on failed setup, got %v", failedOrder)
	}

	// 3. Mount base plugin and dependent plugin registering tools, hooks across all 4 phases, and cleanups.
	var mu sync.Mutex
	var cleanupLog []string
	var hookTrace []string
	recordCleanup := func(tag string) {
		mu.Lock()
		cleanupLog = append(cleanupLog, tag)
		mu.Unlock()
	}
	recordHook := func(tag string) {
		mu.Lock()
		hookTrace = append(hookTrace, tag)
		mu.Unlock()
	}

	if err := bus.MountPlugin(Plugin{
		ID: "base_plugin",
		Setup: func(sc *Scope) error {
			sc.OnCleanup(func() { recordCleanup("base:1") })
			sc.RegisterTool(tools.Tool{
				Name:        "base_inspect",
				Description: "Base plugin inspection tool",
				ReadOnly:    true,
				Handler: func(ctx context.Context, call tools.CallContext, rawInput json.RawMessage) (tools.ToolResult, error) {
					return tools.ToolResult{Output: "base-ok"}, nil
				},
			})
			sc.OnPreStep(func(ctx context.Context, ev *PreStepEvent) error {
				recordHook("base:pre_step")
				return nil
			})
			sc.OnPreToolUse(func(ctx context.Context, ev *PreToolUseEvent) (GateResult, error) {
				recordHook("base:pre_tool")
				return GateResult{}, nil
			})
			sc.OnPostToolUse(func(ctx context.Context, ev *PostToolUseEvent) error {
				recordHook("base:post_tool")
				return nil
			})
			sc.OnTurnStopping(func(ctx context.Context, ev *TurnStoppingEvent) error {
				recordHook("base:turn_stop")
				return nil
			})
			sc.OnCleanup(func() { recordCleanup("base:2") })
			return nil
		},
	}); err != nil {
		t.Fatalf("MountPlugin(base_plugin): %v", err)
	}

	if err := bus.MountPlugin(Plugin{
		ID:        "dependent_plugin",
		DependsOn: []string{"base_plugin"},
		Setup: func(sc *Scope) error {
			sc.OnCleanup(func() { recordCleanup("dep:1") })
			sc.RegisterTool(tools.Tool{
				Name:        "dep_lint",
				Description: "Dependent plugin lint tool",
				ReadOnly:    true,
				Handler: func(ctx context.Context, call tools.CallContext, rawInput json.RawMessage) (tools.ToolResult, error) {
					return tools.ToolResult{Output: "dep-ok"}, nil
				},
			})
			sc.OnPreStep(func(ctx context.Context, ev *PreStepEvent) error {
				recordHook("dep:pre_step")
				return nil
			})
			sc.OnPreToolUse(func(ctx context.Context, ev *PreToolUseEvent) (GateResult, error) {
				recordHook("dep:pre_tool")
				if ev.Call.ToolName == "flag_me" {
					return GateResult{
						Action:     GateAsk,
						ReasonCode: "dep-audit",
						Reason:     "dependent plugin requested confirmation",
					}, nil
				}
				return GateResult{}, nil
			})
			sc.OnPostToolUse(func(ctx context.Context, ev *PostToolUseEvent) error {
				recordHook("dep:post_tool")
				return nil
			})
			sc.OnTurnStopping(func(ctx context.Context, ev *TurnStoppingEvent) error {
				recordHook("dep:turn_stop")
				return nil
			})
			sc.OnCleanup(func() { recordCleanup("dep:2") })
			return nil
		},
	}); err != nil {
		t.Fatalf("MountPlugin(dependent_plugin): %v", err)
	}

	if _, ok := reg.Get("base_inspect"); !ok {
		t.Fatal("expected base_inspect registered in tool Registry")
	}
	if _, ok := reg.Get("dep_lint"); !ok {
		t.Fatal("expected dep_lint registered in tool Registry")
	}

	// 4. Freeze HookBus and verify immutable zero-lock dispatch across all 4 hook slices.
	bus.Freeze()
	if !bus.IsFrozen() {
		t.Fatal("expected HookBus.IsFrozen() == true after Freeze()")
	}

	ctx := context.Background()
	preStepEv := PreStepEvent{RunID: "r1", Step: 1}
	if err := bus.RunPreStep(ctx, &preStepEv); err != nil {
		t.Fatalf("RunPreStep: %v", err)
	}
	preToolEv := PreToolUseEvent{RunID: "r1", Step: 1, Call: ToolInvocation{ToolName: "flag_me"}}
	hookDec, err := bus.RunPreToolUse(ctx, &preToolEv)
	if err != nil {
		t.Fatalf("RunPreToolUse: %v", err)
	}
	if hookDec.Action != GateAsk || hookDec.ReasonCode != "dep-audit" {
		t.Fatalf("expected GateAsk (dep-audit) from PreToolUse hook, got %+v", hookDec)
	}
	toolRes := tools.ToolResult{Output: "done"}
	postToolEv := PostToolUseEvent{RunID: "r1", Step: 1, Call: preToolEv.Call, Result: &toolRes}
	if err := bus.RunPostToolUse(ctx, &postToolEv); err != nil {
		t.Fatalf("RunPostToolUse: %v", err)
	}
	stopEv := TurnStoppingEvent{RunID: "r1", Step: 1, StopReason: "end_turn"}
	if err := bus.RunTurnStopping(ctx, &stopEv); err != nil {
		t.Fatalf("RunTurnStopping: %v", err)
	}

	wantTrace := "base:pre_step,dep:pre_step,base:pre_tool,dep:pre_tool,base:post_tool,dep:post_tool,base:turn_stop,dep:turn_stop"
	if got := strings.Join(hookTrace, ","); got != wantTrace {
		t.Fatalf("hook execution trace mismatch:\ngot:  %s\nwant: %s", got, wantTrace)
	}

	// Verify zero-overhead frozen empty hook dispatch performs 0 heap allocations.
	emptyBus := NewHookBus(nil)
	emptyBus.Freeze()
	allocs := testing.AllocsPerRun(50, func() {
		_ = emptyBus.RunPreStep(ctx, &preStepEv)
		_, _ = emptyBus.RunPreToolUse(ctx, &preToolEv)
		_ = emptyBus.RunPostToolUse(ctx, &postToolEv)
		_ = emptyBus.RunTurnStopping(ctx, &stopEv)
	})
	if allocs != 0 {
		t.Fatalf("expected 0 allocations on frozen empty HookBus hot path, got %v", allocs)
	}

	// 5. Unmount base_plugin -> dependent_plugin MUST unwind first (LIFO), then base_plugin (LIFO),
	// and all registered tools and hooks must vanish cleanly!
	if err := bus.UnmountPlugin("base_plugin"); err != nil {
		t.Fatalf("UnmountPlugin(base_plugin): %v", err)
	}

	wantCleanup := "dep:2,dep:1,base:2,base:1"
	if got := strings.Join(cleanupLog, ","); got != wantCleanup {
		t.Fatalf("expected dependent plugin LIFO cleanup before base plugin:\ngot:  %s\nwant: %s", got, wantCleanup)
	}

	if _, ok := reg.Get("dep_lint"); ok {
		t.Fatal("expected dep_lint tool unregistered after unmount")
	}
	if _, ok := reg.Get("base_inspect"); ok {
		t.Fatal("expected base_inspect tool unregistered after unmount")
	}

	mu.Lock()
	hookTrace = nil
	mu.Unlock()

	if err := bus.RunPreStep(ctx, &preStepEv); err != nil {
		t.Fatalf("RunPreStep after unmount: %v", err)
	}
	decAfter, err := bus.RunPreToolUse(ctx, &preToolEv)
	if err != nil {
		t.Fatalf("RunPreToolUse after unmount: %v", err)
	}
	if decAfter.RequiresAction() {
		t.Fatalf("expected zero hook action after unmount, got %+v", decAfter)
	}
	if err := bus.RunPostToolUse(ctx, &postToolEv); err != nil {
		t.Fatalf("RunPostToolUse after unmount: %v", err)
	}
	if err := bus.RunTurnStopping(ctx, &stopEv); err != nil {
		t.Fatalf("RunTurnStopping after unmount: %v", err)
	}
	if len(hookTrace) != 0 {
		t.Fatalf("expected 0 hook invocations after unmounting plugins, got %v", hookTrace)
	}
}

func TestPermissionGateNineStepOrder(t *testing.T) {
	baseDir := t.TempDir()
	wsRoot := filepath.Join(baseDir, "repo")
	vaultRoot := filepath.Join(baseDir, "vault")
	homeDir := filepath.Join(baseDir, "home")
	for _, d := range []string{wsRoot, vaultRoot, homeDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	cfgMgr, err := config.NewManager(config.Options{
		VaultRoot:   vaultRoot,
		UserHomeDir: homeDir,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	pg, err := tools.NewPathGuard(wsRoot, vaultRoot)
	if err != nil {
		t.Fatalf("NewPathGuard: %v", err)
	}
	reg, err := tools.NewRegistry(tools.RegistryOptions{
		WorkspaceRoot: wsRoot,
		VaultPaths:    cfgMgr.Paths(),
		PathGuard:     pg,
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	// Register a fake MCP tool that self-reports readOnlyHint: true.
	mcpReadHintTool := tools.Tool{
		Name:            "mcp__github__list_issues",
		Description:     "List GitHub issues",
		ReadOnly:        false,
		Mutating:        true,
		ConcurrencySafe: false,
		IsMCP:           true,
		MCPServerID:     "github",
		MCPReadOnlyHint: true,
	}
	reg.Register(mcpReadHintTool)

	readTool, _ := reg.Get("read_file")
	writeTool, _ := reg.Get("write_file")
	runCmdTool, _ := reg.Get("run_command")

	grants := NewGrantStore(cfgMgr.Paths())
	gate := NewPermissionGate(grants)
	snap := cfgMgr.Current()

	baseEval := EvalContext{
		WorkspaceID:   "ws-test",
		WorkspaceRoot: wsRoot,
		VaultRoot:     vaultRoot,
		UserHomeDir:   homeDir,
		EffectiveMode: PermModeAuto,
		Rules:         snap.Rules,
		PathGuard:     pg,
	}

	// Step (1): Hard-floor blocks `rm -rf /` and `~/.gorness/providers.toml` EVEN in `yolo` mode!
	yoloCtx := baseEval
	yoloCtx.EffectiveMode = PermModeYolo

	resRmRoot := gate.Evaluate(yoloCtx, ToolInvocation{
		ToolName: "run_command",
		Args:     json.RawMessage(`{"command":"rm -rf /"}`),
		Tool:     &runCmdTool,
	})
	if resRmRoot.Action != GateDeny || resRmRoot.ReasonCode != "hard-floor" {
		t.Fatalf("Step 1 failed: expected hard-floor GateDeny on 'rm -rf /' in yolo mode, got %+v", resRmRoot)
	}

	for _, secretPath := range []string{
		"~/.gorness/providers.toml",
		filepath.Join(vaultRoot, "providers.toml"),
	} {
		rawArgs, _ := json.Marshal(map[string]string{"path": secretPath})
		resVaultRead := gate.Evaluate(yoloCtx, ToolInvocation{
			ToolName: "read_file",
			Args:     rawArgs,
			Tool:     &readTool,
		})
		if resVaultRead.Action != GateDeny || resVaultRead.ReasonCode != "hard-floor" {
			t.Fatalf("Step 1 failed: expected hard-floor GateDeny on %q in yolo mode, got %+v", secretPath, resVaultRead)
		}
	}

	// Step (2): `plan` mode denies writes and MCP tools (even MCP with readOnlyHint: true), while allowing built-in read_file.
	planCtx := baseEval
	planCtx.EffectiveMode = PermModePlan

	resPlanWrite := gate.Evaluate(planCtx, ToolInvocation{
		ToolName: "write_file",
		Args:     json.RawMessage(`{"path":"main.go","content":"package main\n"}`),
		Tool:     &writeTool,
	})
	if resPlanWrite.Action != GateDeny || resPlanWrite.ReasonCode != "plan-readonly" {
		t.Fatalf("Step 2 failed: expected plan-readonly GateDeny on write_file, got %+v", resPlanWrite)
	}

	resPlanMCP := gate.Evaluate(planCtx, ToolInvocation{
		ToolName: mcpReadHintTool.Name,
		Args:     json.RawMessage(`{}`),
		Tool:     &mcpReadHintTool,
	})
	if resPlanMCP.Action != GateDeny || resPlanMCP.ReasonCode != "plan-readonly" {
		t.Fatalf("Step 2 failed: expected plan-readonly GateDeny on MCP tool, got %+v", resPlanMCP)
	}

	resPlanRead := gate.Evaluate(planCtx, ToolInvocation{
		ToolName: "read_file",
		Args:     json.RawMessage(`{"path":"README.md"}`),
		Tool:     &readTool,
	})
	if resPlanRead.Action != GateAllow {
		t.Fatalf("Step 2 failed: expected built-in in-workspace read_file allowed in plan mode, got %+v", resPlanRead)
	}

	// Step (3): Untrusted modified `.gorness/mcp.toml` downgrades calls to `ask` (and `deny` when unattended).
	wsGornessDir := filepath.Join(wsRoot, ".gorness")
	if err := os.MkdirAll(wsGornessDir, 0o755); err != nil {
		t.Fatalf("mkdir .gorness: %v", err)
	}
	untrustedMCPPath := filepath.Join(wsGornessDir, "mcp.toml")
	if err := os.WriteFile(untrustedMCPPath, []byte("[servers.evil]\ncommand = \"sh\"\n"), 0o644); err != nil {
		t.Fatalf("write untrusted .gorness/mcp.toml: %v", err)
	}
	wsSnap, warnings, err := cfgMgr.ForWorkspace(wsRoot, nil)
	if err != nil {
		t.Fatalf("ForWorkspace: %v", err)
	}
	if !DetectTamperedProjectConfig(wsRoot, nil, warnings) {
		t.Fatalf("expected DetectTamperedProjectConfig == true for untrusted .gorness/mcp.toml (warnings=%v)", warnings)
	}

	tamperedInteractive := baseEval
	tamperedInteractive.Rules = wsSnap.Rules
	tamperedInteractive.ConfigTampered = DetectTamperedProjectConfig(wsRoot, nil, warnings)

	resTamperedAsk := gate.Evaluate(tamperedInteractive, ToolInvocation{
		ToolName: "write_file",
		Args:     json.RawMessage(`{"path":"app.go","content":"package app\n"}`),
		Tool:     &writeTool,
	})
	if resTamperedAsk.Action != GateAsk || resTamperedAsk.ReasonCode != "tampered-project-config" {
		t.Fatalf("Step 3 failed: expected tampered-project-config GateAsk, got %+v", resTamperedAsk)
	}

	tamperedUnattended := tamperedInteractive
	tamperedUnattended.Unattended = true
	resTamperedDeny := gate.Evaluate(tamperedUnattended, ToolInvocation{
		ToolName: "write_file",
		Args:     json.RawMessage(`{"path":"app.go","content":"package app\n"}`),
		Tool:     &writeTool,
	})
	if resTamperedDeny.Action != GateDeny || resTamperedDeny.ReasonCode != "tampered-project-config" {
		t.Fatalf("Step 3 failed: expected tampered-project-config GateDeny when unattended, got %+v", resTamperedDeny)
	}

	// Clean up untrusted .gorness/mcp.toml for subsequent step checks.
	_ = os.Remove(untrustedMCPPath)

	// Step (4): PreToolUse Hook decision short-circuits ask/deny, and Step (5) MCP `readOnlyHint: true` is NOT exempted as built-in readonly!
	hookAskCtx := baseEval
	hookAskCtx.HookDecision = GateResult{Action: GateAsk, ReasonCode: "custom-hook-check", Reason: "verify before write"}
	resHookAsk := gate.Evaluate(hookAskCtx, ToolInvocation{
		ToolName: "read_file",
		Args:     json.RawMessage(`{"path":"README.md"}`),
		Tool:     &readTool,
	})
	if resHookAsk.Action != GateAsk || resHookAsk.ReasonCode != "custom-hook-check" {
		t.Fatalf("Step 4 failed: expected PreToolUse hook ask to precede Step 5 readonly exemption, got %+v", resHookAsk)
	}

	askModeCtx := baseEval
	askModeCtx.EffectiveMode = PermModeAsk
	resBuiltinReadInAsk := gate.Evaluate(askModeCtx, ToolInvocation{
		ToolName: "read_file",
		Args:     json.RawMessage(`{"path":"README.md"}`),
		Tool:     &readTool,
	})
	if resBuiltinReadInAsk.Action != GateAllow {
		t.Fatalf("Step 5 failed: expected built-in read_file to be exempted as GateAllow in ask mode, got %+v", resBuiltinReadInAsk)
	}

	for _, mode := range []string{PermModeAsk, PermModeAcceptEdits, PermModeAuto} {
		mCtx := baseEval
		mCtx.EffectiveMode = mode
		resMCP := gate.Evaluate(mCtx, ToolInvocation{
			ToolName: mcpReadHintTool.Name,
			Args:     json.RawMessage(`{"repo":"gorness"}`),
			Tool:     &mcpReadHintTool,
		})
		if resMCP.Action != GateAsk {
			t.Fatalf("Step 5 failed in mode %q: MCP tool with readOnlyHint=true MUST NOT be exempted as built-in readonly, got %+v", mode, resMCP)
		}
	}

	// Step (5 in brief / Step 6 in 9-step): Touching `.gorness/` or `.zshrc` triggers `auto-run-config` ask even in `auto` and `yolo` modes!
	for _, mode := range []string{PermModeAuto, PermModeAcceptEdits, PermModeYolo} {
		mCtx := baseEval
		mCtx.EffectiveMode = mode

		resGornessEdit := gate.Evaluate(mCtx, ToolInvocation{
			ToolName: "write_file",
			Args:     json.RawMessage(`{"path":".gorness/agents/coder.md","content":"+++\nid=\"coder\"\n+++\npwned"}`),
			Tool:     &writeTool,
		})
		if resGornessEdit.Action != GateAsk || resGornessEdit.ReasonCode != "auto-run-config" {
			t.Fatalf("Step 6 failed in mode %q: touching .gorness/ must return auto-run-config GateAsk, got %+v", mode, resGornessEdit)
		}

		zshrcPath := filepath.Join(homeDir, ".zshrc")
		zshrcArgs, _ := json.Marshal(map[string]string{"path": zshrcPath, "content": "export EVIL=1\n"})
		resZshrc := gate.Evaluate(mCtx, ToolInvocation{
			ToolName: "write_file",
			Args:     zshrcArgs,
			Tool:     &writeTool,
		})
		if resZshrc.Action != GateAsk || resZshrc.ReasonCode != "auto-run-config" {
			t.Fatalf("Step 6 failed in mode %q: touching .zshrc must return auto-run-config GateAsk, got %+v", mode, resZshrc)
		}
	}

	// Step 7 & Step 8: Saved fingerprint grant allows matching call in same workspace,
	// fails on modified args or different workspace, and NEVER bypasses AlwaysAsk!
	pushCall := ToolInvocation{
		ToolName: "run_command",
		Args:     json.RawMessage(`{"command":"git push origin feat/gorness-v1"}`),
		Tool:     &runCmdTool,
	}
	resPushBefore := gate.Evaluate(baseEval, pushCall)
	if resPushBefore.Action != GateAsk || resPushBefore.Fingerprint == "" {
		t.Fatalf("expected git push to require GateAsk before grant, got %+v", resPushBefore)
	}
	if err := grants.SaveGrant(baseEval.WorkspaceID, resPushBefore.Fingerprint, pushCall.ToolName, "git push origin feat/gorness-v1"); err != nil {
		t.Fatalf("SaveGrant: %v", err)
	}

	// Reload GrantStore from disk to prove zero-DB TOML persistence at workspaces/<wsID>/grants.toml!
	reloadedGrants := NewGrantStore(cfgMgr.Paths())
	reloadedGate := NewPermissionGate(reloadedGrants)
	resPushAfter := reloadedGate.Evaluate(baseEval, pushCall)
	if resPushAfter.Action != GateAllow || resPushAfter.ReasonCode != "saved-grant" {
		t.Fatalf("Step 8 failed: expected saved-grant GateAllow after reload, got %+v", resPushAfter)
	}

	otherWsEval := baseEval
	otherWsEval.WorkspaceID = "ws-other"
	if resOtherWS := reloadedGate.Evaluate(otherWsEval, pushCall); resOtherWS.Action != GateAsk {
		t.Fatalf("Step 8 failed: grant must stay scoped to WorkspaceID, got %+v", resOtherWS)
	}

	alwaysAskCall := pushCall
	alwaysAskCall.AlwaysAsk = true
	if resAlways := reloadedGate.Evaluate(baseEval, alwaysAskCall); resAlways.Action != GateAsk || resAlways.ReasonCode != "always-ask" {
		t.Fatalf("Step 7 failed: AlwaysAsk must precede saved fingerprint grant, got %+v", resAlways)
	}

	// Step (6 in brief / Step 9 unattended): Unattended background run needing confirmation returns `GateDefer`!
	unattendedCtx := baseEval
	unattendedCtx.Unattended = true
	resDeferredCmd := reloadedGate.Evaluate(unattendedCtx, ToolInvocation{
		ToolName: "run_command",
		Args:     json.RawMessage(`{"command":"git push --force origin main"}`),
		Tool:     &runCmdTool,
	})
	if resDeferredCmd.Action != GateDefer {
		t.Fatalf("expected GateDefer for risky command in unattended background run, got %+v", resDeferredCmd)
	}

	resDeferredAutoCfg := reloadedGate.Evaluate(unattendedCtx, ToolInvocation{
		ToolName: "write_file",
		Args:     json.RawMessage(`{"path":".gorness/rules.toml","content":""}`),
		Tool:     &writeTool,
	})
	if resDeferredAutoCfg.Action != GateDefer || resDeferredAutoCfg.ReasonCode != "auto-run-config" {
		t.Fatalf("expected GateDefer (auto-run-config) in unattended run, got %+v", resDeferredAutoCfg)
	}
}

func TestApprovalBrokerSettleGuard(t *testing.T) {
	t0 := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	clock := newFakeClock(t0)

	var stepCount atomic.Int32
	var step2ToolOutput string
	var mu sync.Mutex

	streamer := llm.StreamerFunc(func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
		ch := make(chan llm.StreamEvent, 8)
		// Utility auto-titler call
		if len(req.Tools) == 0 && strings.Contains(strings.ToLower(req.StaticSystem), "housekeeping utility") {
			ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Partial Hunk Approval Test"}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", InputTokens: 20, OutputTokens: 5}
			close(ch)
			return ch, nil
		}

		n := stepCount.Add(1)
		if n == 1 {
			editArgs, _ := json.Marshal(map[string]any{
				"path":        "service.go",
				"old_string":  "\"v1\"",
				"new_string":  "\"v2\"",
				"replace_all": true,
			})
			ch <- llm.StreamEvent{
				Type:       llm.StreamToolCall,
				ToolCallID: "call_edit_2hunks",
				ToolName:   "edit_file",
				ToolInput:  editArgs,
			}
			ch <- llm.StreamEvent{
				Type:         llm.StreamDone,
				StopReason:   "tool_use",
				InputTokens:  100,
				OutputTokens: 30,
			}
			close(ch)
			return ch, nil
		}

		for _, entry := range req.History {
			for _, b := range entry.Blocks {
				if b.Type == "tool_result" && b.ToolCallID == "call_edit_2hunks" {
					mu.Lock()
					step2ToolOutput = b.Text
					mu.Unlock()
				}
			}
		}
		ch <- llm.StreamEvent{
			Type: llm.StreamTextDelta,
			Text: "Applied selected hunk cleanly.",
		}
		ch <- llm.StreamEvent{
			Type:         llm.StreamDone,
			StopReason:   "end_turn",
			InputTokens:  140,
			OutputTokens: 12,
		}
		close(ch)
		return ch, nil
	})

	eng, _, _, _ := setupTestEngine(t, clock, streamer)

	wsDir := t.TempDir()
	originalSrc := strings.Join([]string{
		"package service",
		"",
		"var HeaderVersion = \"v1\"",
		"",
		"// spacer 1",
		"// spacer 2",
		"// spacer 3",
		"// spacer 4",
		"// spacer 5",
		"",
		"func FooterVersion() string {",
		"\treturn \"v1\"",
		"}",
		"",
	}, "\n")
	servicePath := filepath.Join(wsDir, "service.go")
	if err := os.WriteFile(servicePath, []byte(originalSrc), 0o644); err != nil {
		t.Fatalf("write service.go: %v", err)
	}

	ws, err := eng.Workspaces().Register(wsDir, "ApprovalTestRepo")
	if err != nil {
		t.Fatalf("Register workspace: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	runID, stream, err := eng.StartRun(ctx, RunRequest{
		WorkspaceID: ws.ID,
		Prompt:      "Update versions in service.go",
		AgentRole:   "coder",
		PermMode:    PermModeAsk,
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if runID == "" {
		t.Fatal("expected non-empty runID")
	}

	// Wait until EventApprovalRequired is surfaced on the run stream.
	var surfaced *PendingApproval
	var events []RunEvent
	for ev := range stream {
		events = append(events, ev)
		if ev.Type == EventApprovalRequired {
			pa, ok := ev.Payload.(PendingApproval)
			if !ok {
				if paPtr, okPtr := ev.Payload.(*PendingApproval); okPtr && paPtr != nil {
					pa = *paPtr
					ok = true
				}
			}
			if !ok {
				t.Fatalf("expected PendingApproval payload on %s, got %T", EventApprovalRequired, ev.Payload)
			}
			surfaced = &pa
			break
		}
	}

	if surfaced == nil {
		t.Fatalf("run ended without emitting %s (events=%+v)", EventApprovalRequired, events)
	}
	if len(surfaced.Hunks) != 2 {
		t.Fatalf("expected 2 staged DiffHunks on PendingApproval, got %d: %+v", len(surfaced.Hunks), surfaced.Hunks)
	}

	// 1. Attempt Resolve at +100ms (< 500ms SettleWindow) -> MUST fail with ErrApprovalNotSettled and stay pending!
	clock.Advance(100 * time.Millisecond)
	err = eng.Broker().Resolve(surfaced.ID, ApproveHunks([]string{surfaced.Hunks[0].ID}))
	if !errors.Is(err, ErrApprovalNotSettled) {
		t.Fatalf("expected ErrApprovalNotSettled at +100ms, got %v", err)
	}
	pendingStill := eng.Broker().ListPending(ws.ID)
	if len(pendingStill) != 1 || pendingStill[0].ID != surfaced.ID {
		t.Fatalf("expected approval %q to remain pending after premature click, got %+v", surfaced.ID, pendingStill)
	}

	// Disk must still be untouched while approval is pending.
	diskMid, _ := os.ReadFile(servicePath)
	if string(diskMid) != originalSrc {
		t.Fatalf("service.go was mutated before approval settled:\n%s", string(diskMid))
	}

	// 2. Advance clock past 500ms (+450ms -> total +550ms) and approve ONLY hunk #1!
	clock.Advance(450 * time.Millisecond)
	if err := eng.Broker().Resolve(surfaced.ID, ApproveHunks([]string{surfaced.Hunks[0].ID})); err != nil {
		t.Fatalf("Resolve after 550ms failed: %v", err)
	}
	if gotPending := eng.Broker().ListPending(ws.ID); len(gotPending) != 0 {
		t.Fatalf("expected 0 pending approvals after valid resolution, got %+v", gotPending)
	}

	// Drain remaining events until run finishes cleanly.
	restEvents := drainRunEvents(stream)
	sawCompleted := false
	for _, ev := range restEvents {
		if ev.Type == EventRunCompleted {
			sawCompleted = true
		}
		if ev.Type == EventRunFailed {
			t.Fatalf("unexpected EventRunFailed after hunk approval: %+v", ev.Payload)
		}
	}
	if !sawCompleted {
		t.Fatalf("expected EventRunCompleted after approval resumed loop, got %+v", restEvents)
	}

	// Verify ONLY hunk #1 was applied on disk, while hunk #2 remained "v1"!
	finalDisk, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatalf("read final service.go: %v", err)
	}
	if !strings.Contains(string(finalDisk), `var HeaderVersion = "v2"`) {
		t.Fatalf("expected hunk #1 applied (HeaderVersion = v2), got:\n%s", string(finalDisk))
	}
	if !strings.Contains(string(finalDisk), "\treturn \"v1\"") {
		t.Fatalf("expected hunk #2 untouched (return v1), got:\n%s", string(finalDisk))
	}

	mu.Lock()
	gotToolOut := step2ToolOutput
	mu.Unlock()
	if !strings.Contains(gotToolOut, `"applied_hunks":1`) || !strings.Contains(gotToolOut, `"total_hunks":2`) {
		t.Fatalf("expected honest partial hunk summary reported to model, got %q", gotToolOut)
	}

	// 3. Verify zero-hunk approval (`ApproveHunks([]string{})`) leaves disk untouched (`written: false`),
	// and `ApproveAndRemember` persists to `~/.gorness/workspaces/<wsID>/grants.toml`!
	stepCount.Store(0)
	_, streamZero, err := eng.StartRun(ctx, RunRequest{
		WorkspaceID: ws.ID,
		Prompt:      "Second edit attempt with zero hunks selected",
		AgentRole:   "coder",
		PermMode:    PermModeAsk,
	})
	if err != nil {
		t.Fatalf("StartRun zero-hunk: %v", err)
	}
	var zeroAppr *PendingApproval
	for ev := range streamZero {
		if ev.Type == EventApprovalRequired {
			pa := ev.Payload.(PendingApproval)
			zeroAppr = &pa
			break
		}
	}
	if zeroAppr == nil {
		t.Fatal("expected PendingApproval for zero-hunk test")
	}
	diskBeforeZero, _ := os.ReadFile(servicePath)
	clock.Advance(600 * time.Millisecond)
	if err := eng.Broker().ApproveHunks(zeroAppr.ID, []string{}); err != nil {
		t.Fatalf("Broker.ApproveHunks([]): %v", err)
	}
	_ = drainRunEvents(streamZero)
	diskAfterZero, _ := os.ReadFile(servicePath)
	if string(diskAfterZero) != string(diskBeforeZero) {
		t.Fatalf("zero-hunk approval must not touch disk:\ngot:  %s\nwant: %s", diskAfterZero, diskBeforeZero)
	}
	mu.Lock()
	gotZeroOut := step2ToolOutput
	mu.Unlock()
	if !strings.Contains(gotZeroOut, `"written":false`) {
		t.Fatalf("expected written:false reported to model on zero-hunk approval, got %q", gotZeroOut)
	}
}

func TestMultiWorkspaceParallelRunsSteerDoomLoopAndCompaction(t *testing.T) {
	// Shared coordination channels to prove true simultaneous execution across Workspace A & Workspace B,
	// parallel read_file fan-out inside Run A, and mid-run Steer injection at step boundary.
	wsAEntered := make(chan struct{})
	wsBEntered := make(chan struct{})
	steerInjectedDone := make(chan struct{})

	var mu sync.Mutex
	var runAStaticSystems []string
	var runAStep2History []store.SessionEntry
	var doomLLMCalls atomic.Int32
	var compactionPrompts []string

	streamer := llm.StreamerFunc(func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
		ch := make(chan llm.StreamEvent, 16)

		// Handle `utility` housekeeping calls (auto-titler and anchored compaction).
		if strings.Contains(strings.ToLower(req.StaticSystem), "housekeeping utility") {
			var promptText strings.Builder
			for _, e := range req.History {
				for _, b := range e.Blocks {
					promptText.WriteString(b.Text)
					promptText.WriteByte('\n')
				}
			}
			fullPrompt := promptText.String()
			if strings.Contains(fullPrompt, "<turns_to_compact>") {
				mu.Lock()
				compactionPrompts = append(compactionPrompts, fullPrompt)
				passNum := len(compactionPrompts)
				mu.Unlock()

				var summary string
				if passNum == 1 {
					summary = "## Goal & Intent\nBuild multi-workspace scheduler.\n## Key Decisions & Context\nANCHOR_FACT_PASS_1: adopted Flock per-session lock.\n## Files & Artifacts Touched\ninternal/core/engine.go"
				} else {
					hasPrior := strings.Contains(fullPrompt, "ANCHOR_FACT_PASS_1")
					summary = fmt.Sprintf("## Goal & Intent\nBuild multi-workspace scheduler.\n## Key Decisions & Context\nMERGED_PRIOR=%v; ANCHOR_FACT_PASS_1 + ANCHOR_FACT_PASS_2: added 500ms settle guard.\n## Files & Artifacts Touched\ninternal/core/engine.go, internal/core/approvals.go", hasPrior)
				}
				ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: summary}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", InputTokens: 80, OutputTokens: 40}
				close(ch)
				return ch, nil
			}

			ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Concurrent Engine Architecture"}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", InputTokens: 25, OutputTokens: 4}
			close(ch)
			return ch, nil
		}

		// Inspect user prompt to dispatch scenario: Workspace A, Workspace B, or DoomLoop.
		firstUserText := ""
		for _, e := range req.History {
			if e.Role == "user" {
				for _, b := range e.Blocks {
					if b.Type == "text" {
						firstUserText = b.Text
						break
					}
				}
				if firstUserText != "" {
					break
				}
			}
		}

		switch {
		case strings.Contains(firstUserText, "RUN_WS_A"):
			mu.Lock()
			runAStaticSystems = append(runAStaticSystems, req.StaticSystem)
			stepNum := len(runAStaticSystems)
			if stepNum == 2 {
				runAStep2History = append([]store.SessionEntry(nil), req.History...)
			}
			mu.Unlock()

			if stepNum == 1 {
				close(wsAEntered)
				// Wait for Workspace B to ALSO be inside Step 1 concurrently AND for test to inject Steer!
				select {
				case <-wsBEntered:
				case <-ctx.Done():
					close(ch)
					return ch, ctx.Err()
				}
				select {
				case <-steerInjectedDone:
				case <-ctx.Done():
					close(ch)
					return ch, ctx.Err()
				}

				// Emit two consecutive concurrency-safe read_file calls!
				ch <- llm.StreamEvent{
					Type:       llm.StreamToolCall,
					ToolCallID: "wsA_read_1",
					ToolName:   "read_file",
					ToolInput:  json.RawMessage(`{"path":"alpha1.txt"}`),
				}
				ch <- llm.StreamEvent{
					Type:       llm.StreamToolCall,
					ToolCallID: "wsA_read_2",
					ToolName:   "read_file",
					ToolInput:  json.RawMessage(`{"path":"alpha2.txt"}`),
				}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use", InputTokens: 90, OutputTokens: 20}
				close(ch)
				return ch, nil
			}

			ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Workspace A finished with [STEER-APPLIED]."}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", InputTokens: 120, OutputTokens: 15}
			close(ch)
			return ch, nil

		case strings.Contains(firstUserText, "RUN_WS_B"):
			// Check if this is step 1 of Workspace B
			hasToolResult := false
			for _, e := range req.History {
				if e.Role == "tool" {
					hasToolResult = true
				}
			}
			if !hasToolResult {
				close(wsBEntered)
				select {
				case <-wsAEntered:
				case <-ctx.Done():
					close(ch)
					return ch, ctx.Err()
				}
				ch <- llm.StreamEvent{
					Type:       llm.StreamToolCall,
					ToolCallID: "wsB_read_1",
					ToolName:   "read_file",
					ToolInput:  json.RawMessage(`{"path":"beta.txt"}`),
				}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use", InputTokens: 70, OutputTokens: 15}
				close(ch)
				return ch, nil
			}
			ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Workspace B finished independently."}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", InputTokens: 95, OutputTokens: 10}
			close(ch)
			return ch, nil

		case strings.Contains(firstUserText, "TRIGGER_DOOM_LOOP"):
			callIdx := doomLLMCalls.Add(1)
			ch <- llm.StreamEvent{
				Type:       llm.StreamToolCall,
				ToolCallID: fmt.Sprintf("doom_call_%d", callIdx),
				ToolName:   "edit_file",
				ToolInput:  json.RawMessage(`{"path":"alpha1.txt","old_string":"NON_EXISTENT_PATTERN_XYZ","new_string":"replacement"}`),
			}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use", InputTokens: 60, OutputTokens: 20}
			close(ch)
			return ch, nil
		}

		ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "ok"}
		ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn"}
		close(ch)
		return ch, nil
	})

	eng, _, _, _ := setupTestEngine(t, nil, streamer)

	wsADir := t.TempDir()
	wsBDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(wsADir, "alpha1.txt"), []byte("alpha-one\n"), 0o644); err != nil {
		t.Fatalf("write alpha1.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsADir, "alpha2.txt"), []byte("alpha-two\n"), 0o644); err != nil {
		t.Fatalf("write alpha2.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsBDir, "beta.txt"), []byte("beta-only\n"), 0o644); err != nil {
		t.Fatalf("write beta.txt: %v", err)
	}

	wsA, err := eng.Workspaces().Register(wsADir, "ProjectAlpha")
	if err != nil {
		t.Fatalf("Register wsA: %v", err)
	}
	wsB, err := eng.Workspaces().Register(wsBDir, "ProjectBeta")
	if err != nil {
		t.Fatalf("Register wsB: %v", err)
	}

	sessA, err := eng.Sessions().Create(wsA.ID, "New Session", "coder")
	if err != nil {
		t.Fatalf("Create sessA: %v", err)
	}
	sessB, err := eng.Sessions().Create(wsB.ID, "New Session", "coder")
	if err != nil {
		t.Fatalf("Create sessB: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 1. Start Run A and Run B concurrently across Workspace A and Workspace B!
	runAID, streamA, err := eng.StartRun(ctx, RunRequest{
		WorkspaceID: wsA.ID,
		SessionID:   sessA.ID,
		Prompt:      "RUN_WS_A: analyze alpha files",
		AgentRole:   "coder",
	})
	if err != nil {
		t.Fatalf("StartRun A: %v", err)
	}

	runBID, streamB, err := eng.StartRun(ctx, RunRequest{
		WorkspaceID: wsB.ID,
		SessionID:   sessB.ID,
		Prompt:      "RUN_WS_B: analyze beta files",
		AgentRole:   "coder",
	})
	if err != nil {
		t.Fatalf("StartRun B: %v", err)
	}

	// Wait until BOTH runs are simultaneously active inside Step 1.
	<-wsAEntered
	<-wsBEntered

	// Verify ActiveRuns() lists both concurrent runs and TryFlock blocks a conflicting run on sessA!
	if active := eng.ActiveRuns(); len(active) != 2 {
		t.Fatalf("expected 2 concurrent ActiveRuns, got %d: %+v", len(active), active)
	}
	if _, _, busyErr := eng.StartRun(ctx, RunRequest{
		WorkspaceID: wsA.ID,
		SessionID:   sessA.ID,
		Prompt:      "conflicting concurrent write to same session",
	}); !errors.Is(busyErr, ErrSessionBusy) {
		t.Fatalf("expected ErrSessionBusy on concurrent run against active session, got %v", busyErr)
	}

	// Inject mid-run Steer into Run A while Step 1 is in flight!
	steerMsg := "Include architecture tag [STEER-APPLIED]"
	if err := eng.Steer(runAID, steerMsg); err != nil {
		t.Fatalf("Steer(runA): %v", err)
	}
	close(steerInjectedDone)
	_ = runBID

	var wg sync.WaitGroup
	var evsA, evsB []RunEvent
	wg.Add(2)
	go func() {
		defer wg.Done()
		evsA = drainRunEvents(streamA)
	}()
	go func() {
		defer wg.Done()
		evsB = drainRunEvents(streamB)
	}()
	wg.Wait()

	// Wait for async utility auto-titler goroutines to settle cleanly.
	eng.WaitBackground()

	// Verify both runs completed cleanly.
	hasEvent := func(evs []RunEvent, typ string) bool {
		for _, e := range evs {
			if e.Type == typ {
				return true
			}
		}
		return false
	}
	if !hasEvent(evsA, EventRunCompleted) || !hasEvent(evsA, EventSteerInjected) {
		t.Fatalf("expected Run A to emit EventSteerInjected and EventRunCompleted, got %+v", evsA)
	}
	if !hasEvent(evsB, EventRunCompleted) {
		t.Fatalf("expected Run B to emit EventRunCompleted, got %+v", evsB)
	}

	// Verify Steer was injected at step boundary WITHOUT mutating StaticSystem!
	mu.Lock()
	sysSnaps := append([]string(nil), runAStaticSystems...)
	histStep2 := append([]store.SessionEntry(nil), runAStep2History...)
	mu.Unlock()

	if len(sysSnaps) != 2 {
		t.Fatalf("expected 2 LLM steps in Run A, got %d", len(sysSnaps))
	}
	if sysSnaps[0] != sysSnaps[1] {
		t.Fatalf("StaticSystem mutated after mid-run Steer!\nstep1: %q\nstep2: %q", sysSnaps[0], sysSnaps[1])
	}
	if strings.Contains(sysSnaps[1], steerMsg) {
		t.Fatalf("Steer message leaked into StaticSystem: %q", sysSnaps[1])
	}

	foundSteerReminder := false
	for _, e := range histStep2 {
		for _, b := range e.Blocks {
			if strings.Contains(b.Text, "<system-reminder>") && strings.Contains(b.Text, steerMsg) {
				foundSteerReminder = true
			}
		}
	}
	if !foundSteerReminder {
		t.Fatalf("expected mid-run Steer wrapped in <system-reminder> in Step 2 history, got %+v", histStep2)
	}

	// Verify async `utility` role auto-titler updated sessA's title from "New Session"!
	updatedMetaA, err := eng.Sessions().GetMeta(sessA.ID)
	if err != nil {
		t.Fatalf("GetMeta(sessA): %v", err)
	}
	if updatedMetaA.Title == "New Session" || updatedMetaA.Title == "" {
		t.Fatalf("expected async utility auto-titler to update title after Turn 1, still got %q", updatedMetaA.Title)
	}

	// 2. Trigger 3 identical failing tool calls -> verify DoomLoop halts runaway repetition at 3!
	sessDoom, err := eng.Sessions().Create(wsA.ID, "DoomLoop Session", "coder")
	if err != nil {
		t.Fatalf("Create sessDoom: %v", err)
	}
	_, doomStream, err := eng.StartRun(ctx, RunRequest{
		WorkspaceID: wsA.ID,
		SessionID:   sessDoom.ID,
		Prompt:      "TRIGGER_DOOM_LOOP",
		AgentRole:   "coder",
	})
	if err != nil {
		t.Fatalf("StartRun doom: %v", err)
	}
	doomEvents := drainRunEvents(doomStream)
	if !hasEvent(doomEvents, EventDoomLoop) {
		t.Fatalf("expected %s event after 3 identical failing tool calls, got %+v", EventDoomLoop, doomEvents)
	}
	if gotCalls := doomLLMCalls.Load(); gotCalls != 3 {
		t.Fatalf("expected DoomLoop to halt immediately after 3 identical failing tool calls (no 4th LLM call), got %d calls", gotCalls)
	}

	// 3. Verify CompactSession merges anchored summary while keeping recent 30% turns verbatim!
	sessCompact, err := eng.Sessions().Create(wsA.ID, "Long Architecture Session", "coder")
	if err != nil {
		t.Fatalf("Create sessCompact: %v", err)
	}

	appendTurn := func(turnNum int) {
		_, err := eng.Sessions().Append(sessCompact.ID, store.SessionEntry{
			Role:      "user",
			AgentRole: "coder",
			Blocks: []store.ContentBlock{
				{Type: "text", Text: fmt.Sprintf("TURN_%02d_USER_PROMPT: implement module part %d", turnNum, turnNum)},
			},
		})
		if err != nil {
			t.Fatalf("append user turn %d: %v", turnNum, err)
		}
		_, err = eng.Sessions().Append(sessCompact.ID, store.SessionEntry{
			Role:      "assistant",
			ModelID:   "claude-sonnet-5-5",
			AgentRole: "coder",
			Blocks: []store.ContentBlock{
				{
					Type:      "thinking",
					Thinking:  fmt.Sprintf("TURN_%02d_THINKING_EXACT", turnNum),
					Signature: fmt.Sprintf("sig_turn_%02d_exact==", turnNum),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("TURN_%02d_ASSISTANT_REPLY: completed module part %d", turnNum, turnNum),
				},
			},
		})
		if err != nil {
			t.Fatalf("append assistant turn %d: %v", turnNum, err)
		}
	}

	// Populate 10 turns (T1..T10) -> Pass 1 compaction compacts T1..T7 and keeps T8, T9, T10 (recent 30%) verbatim.
	for i := 1; i <= 10; i++ {
		appendTurn(i)
	}
	if err := eng.CompactSession(ctx, sessCompact.ID); err != nil {
		t.Fatalf("CompactSession pass 1: %v", err)
	}

	branchPass1, err := eng.Sessions().LoadActiveBranch(sessCompact.ID)
	if err != nil {
		t.Fatalf("LoadActiveBranch pass 1: %v", err)
	}
	// 1 anchor entry + 3 recent turns * 2 entries/turn = 7 entries
	if len(branchPass1) != 7 {
		t.Fatalf("expected 1 anchor + 6 verbatim entries (recent 30%% = 3 of 10 turns), got %d entries: %+v", len(branchPass1), branchPass1)
	}
	if !strings.Contains(branchPass1[0].Blocks[0].Text, "ANCHOR_FACT_PASS_1") {
		t.Fatalf("expected ANCHOR_FACT_PASS_1 in anchor entry, got %+v", branchPass1[0])
	}
	if !strings.Contains(branchPass1[1].Blocks[0].Text, "TURN_08_USER_PROMPT") ||
		!strings.Contains(branchPass1[5].Blocks[0].Text, "TURN_10_USER_PROMPT") {
		t.Fatalf("expected turns 8..10 kept verbatim after pass 1, got %+v", branchPass1[1:])
	}

	// Append 7 more turns (T11..T17) -> now active branch has Anchor1 + 10 turns (T8..T17).
	// Pass 2 compaction MUST merge Anchor1 + aged-out turns (T8..T14) into Anchor2, while keeping recent 30% (T15, T16, T17) verbatim!
	for i := 11; i <= 17; i++ {
		appendTurn(i)
	}
	if err := eng.CompactSession(ctx, sessCompact.ID); err != nil {
		t.Fatalf("CompactSession pass 2: %v", err)
	}

	branchPass2, err := eng.Sessions().LoadActiveBranch(sessCompact.ID)
	if err != nil {
		t.Fatalf("LoadActiveBranch pass 2: %v", err)
	}
	if len(branchPass2) != 7 {
		t.Fatalf("expected 1 merged anchor + 6 verbatim entries (recent 30%% = 3 of 10 turns), got %d entries", len(branchPass2))
	}
	mergedText := branchPass2[0].Blocks[0].Text
	if !strings.Contains(mergedText, "MERGED_PRIOR=true") || !strings.Contains(mergedText, "ANCHOR_FACT_PASS_2") {
		t.Fatalf("expected Pass 2 anchor to merge prior anchor summary + delta turns, got:\n%s", mergedText)
	}
	// Verify kept turns T15..T17 are 100% verbatim including signed Thinking blocks, and T14 was compacted out!
	for _, e := range branchPass2[1:] {
		for _, b := range e.Blocks {
			if strings.Contains(b.Text, "TURN_14_") {
				t.Fatalf("expected TURN_14 to be compacted into anchor, but found verbatim: %+v", e)
			}
		}
	}
	if !strings.Contains(branchPass2[1].Blocks[0].Text, "TURN_15_USER_PROMPT") {
		t.Fatalf("expected first verbatim kept turn to be TURN_15, got %+v", branchPass2[1])
	}
	lastKeptAssistant := branchPass2[len(branchPass2)-1]
	if lastKeptAssistant.Blocks[0].Thinking != "TURN_17_THINKING_EXACT" ||
		lastKeptAssistant.Blocks[0].Signature != "sig_turn_17_exact==" ||
		!strings.Contains(lastKeptAssistant.Blocks[1].Text, "TURN_17_ASSISTANT_REPLY") {
		t.Fatalf("expected TURN_17 signed thinking & reply preserved verbatim, got %+v", lastKeptAssistant)
	}
}

func TestUnattendedDeferResumeWorktreeAndAllowedToolsIntersection(t *testing.T) {
	var resumeSawOutput string
	var mu sync.Mutex

	streamer := llm.StreamerFunc(func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
		ch := make(chan llm.StreamEvent, 8)

		if strings.Contains(strings.ToLower(req.StaticSystem), "housekeeping utility") {
			ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Deferred Resume Check"}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn"}
			close(ch)
			return ch, nil
		}

		// Inspect whether tool_result already exists in history.
		for _, e := range req.History {
			for _, b := range e.Blocks {
				if b.Type == "tool_result" && b.ToolCallID == "defer_call_1" {
					mu.Lock()
					resumeSawOutput = b.Text
					mu.Unlock()
					ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Resumed deferred deployment cleanly."}
					ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", InputTokens: 50, OutputTokens: 10}
					close(ch)
					return ch, nil
				}
			}
		}

		// Check if this is the subagent AllowedTools restriction check.
		for _, e := range req.History {
			for _, b := range e.Blocks {
				if strings.Contains(b.Text, "SUBAGENT_RESTRICTED_CALL") {
					if len(e.Blocks) > 0 && b.Type == "text" {
						// Check if write_file already denied
						alreadyDenied := false
						for _, he := range req.History {
							for _, hb := range he.Blocks {
								if hb.Type == "tool_result" && hb.IsError {
									alreadyDenied = true
								}
							}
						}
						if alreadyDenied {
							ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Stopped after subagent restriction."}
							ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn"}
							close(ch)
							return ch, nil
						}
						ch <- llm.StreamEvent{
							Type:       llm.StreamToolCall,
							ToolCallID: "sub_write_1",
							ToolName:   "write_file",
							ToolInput:  json.RawMessage(`{"path":"forbidden.txt","content":"nope"}`),
						}
						ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use"}
						close(ch)
						return ch, nil
					}
				}
			}
		}

		// Unattended initial run: write to .gorness/mcp.toml -> triggers auto-run-config -> GateDefer!
		ch <- llm.StreamEvent{
			Type:       llm.StreamToolCall,
			ToolCallID: "defer_call_1",
			ToolName:   "write_file",
			ToolInput:  json.RawMessage(`{"path":".gorness/notes.toml","content":"key = \"val\"\n"}`),
		}
		ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use", InputTokens: 40, OutputTokens: 15}
		close(ch)
		return ch, nil
	})

	eng, cfgMgr, _, _ := setupTestEngine(t, nil, streamer)

	wsDir := t.TempDir()
	wtDir := filepath.Join(cfgMgr.Paths().WorktreesDir(), "wt-worker-1")
	if err := os.MkdirAll(wtDir, 0o755); err != nil {
		t.Fatalf("mkdir wtDir: %v", err)
	}

	ws, err := eng.Workspaces().Register(wsDir, "AlwaysOnFleetRepo")
	if err != nil {
		t.Fatalf("Register ws: %v", err)
	}
	sess, err := eng.Sessions().Create(ws.ID, "Nightly Check", "coder")
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Unattended run in isolated WorktreePath hits GateDefer and exits cleanly WITHOUT blocking!
	_, deferStream, err := eng.StartRun(ctx, RunRequest{
		WorkspaceID:  ws.ID,
		SessionID:    sess.ID,
		Prompt:       "Update .gorness/notes.toml in worktree",
		AgentRole:    "coder",
		WorktreePath: wtDir,
		Unattended:   true,
	})
	if err != nil {
		t.Fatalf("StartRun unattended: %v", err)
	}

	var deferredPayload *DeferredToolPayload
	for ev := range deferStream {
		if ev.Type == EventToolDeferred {
			switch p := ev.Payload.(type) {
			case DeferredToolPayload:
				cp := p
				deferredPayload = &cp
			case *DeferredToolPayload:
				if p != nil {
					cp := *p
					deferredPayload = &cp
				}
			}
		}
	}
	if deferredPayload == nil {
		t.Fatal("expected EventToolDeferred payload on unattended run")
	}
	if deferredPayload.ToolCallID != "defer_call_1" || deferredPayload.ReasonCode != "auto-run-config" || len(deferredPayload.Hunks) == 0 {
		t.Fatalf("unexpected DeferredToolPayload: %+v", deferredPayload)
	}
	// Session lock must already be released!
	if active := eng.ActiveRuns(); len(active) != 0 {
		t.Fatalf("expected worker released immediately on GateDefer, got active=%+v", active)
	}
	// File must NOT exist yet before ResumeApproval.
	if _, stErr := os.Stat(filepath.Join(wtDir, ".gorness", "notes.toml")); !errors.Is(stErr, os.ErrNotExist) {
		t.Fatalf("deferred file must not be written before approval: %v", stErr)
	}

	// 2. Resume deferred tool call -> writes inside wtDir (leaving main wsDir untouched) and finishes session!
	_, resumeStream, err := eng.StartRun(ctx, RunRequest{
		WorkspaceID:  ws.ID,
		SessionID:    sess.ID,
		AgentRole:    "coder",
		WorktreePath: wtDir,
		Unattended:   true,
		ResumeApproval: &DeferredResume{
			DeferredID: deferredPayload.ID,
			ToolCallID: deferredPayload.ToolCallID,
			ToolName:   deferredPayload.ToolName,
			Args:       deferredPayload.Args,
			Approved:   true,
		},
	})
	if err != nil {
		t.Fatalf("StartRun ResumeApproval: %v", err)
	}
	resumeEvs := drainRunEvents(resumeStream)
	sawResumeComplete := false
	for _, ev := range resumeEvs {
		if ev.Type == EventRunCompleted {
			sawResumeComplete = true
		}
	}
	if !sawResumeComplete {
		t.Fatalf("expected EventRunCompleted after ResumeApproval, got %+v", resumeEvs)
	}

	// Verify file landed in wtDir and NOT in main wsDir!
	wtNotes, err := os.ReadFile(filepath.Join(wtDir, ".gorness", "notes.toml"))
	if err != nil || !strings.Contains(string(wtNotes), `key = "val"`) {
		t.Fatalf("expected notes.toml written inside isolated worktree wtDir, got %q (err=%v)", wtNotes, err)
	}
	if _, err := os.Stat(filepath.Join(wsDir, ".gorness", "notes.toml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("main workspace wsDir must remain untouched when WorktreePath is specified")
	}
	mu.Lock()
	sawOut := resumeSawOutput
	mu.Unlock()
	if !strings.Contains(sawOut, `"written":true`) {
		t.Fatalf("expected resumed LLM turn to observe written:true tool_result, got %q", sawOut)
	}

	// 3. Verify RunRequest.AllowedTools subagent intersection blocks write_file even for `coder` role!
	subSess, err := eng.Sessions().Create(ws.ID, "Restricted Subagent", "coder")
	if err != nil {
		t.Fatalf("Create subSess: %v", err)
	}
	_, subStream, err := eng.StartRun(ctx, RunRequest{
		WorkspaceID:  ws.ID,
		SessionID:    subSess.ID,
		Prompt:       "SUBAGENT_RESTRICTED_CALL",
		AgentRole:    "coder",
		AllowedTools: []string{"read_file"},
	})
	if err != nil {
		t.Fatalf("StartRun restricted subagent: %v", err)
	}
	_ = drainRunEvents(subStream)
	if _, err := os.Stat(filepath.Join(wsDir, "forbidden.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("expected AllowedTools intersection to block write_file")
	}

	// 4. Verify parallel read tool batch caps concurrency at 8 via errgroup and StopRun cancels cleanly!
	var inFlight atomic.Int32
	var maxObserved atomic.Int32
	if err := eng.Hooks().MountPlugin(Plugin{
		ID: "parallel_probe",
		Setup: func(sc *Scope) error {
			sc.RegisterTool(tools.Tool{
				Name:            "probe_concurrent_read",
				Description:     "Concurrency-safe read probe",
				ReadOnly:        true,
				Mutating:        false,
				ConcurrencySafe: true,
				Handler: func(ctx context.Context, call tools.CallContext, rawInput json.RawMessage) (tools.ToolResult, error) {
					cur := inFlight.Add(1)
					for {
						prev := maxObserved.Load()
						if cur <= prev || maxObserved.CompareAndSwap(prev, cur) {
							break
						}
					}
					select {
					case <-time.After(25 * time.Millisecond):
					case <-ctx.Done():
						inFlight.Add(-1)
						return tools.ToolResult{}, ctx.Err()
					}
					inFlight.Add(-1)
					return tools.ToolResult{Output: string(rawInput)}, nil
				},
			})
			return nil
		},
	}); err != nil {
		t.Fatalf("MountPlugin(parallel_probe): %v", err)
	}

	parStreamer := llm.StreamerFunc(func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
		ch := make(chan llm.StreamEvent, 20)
		hasResults := false
		for _, e := range req.History {
			if e.Role == "tool" {
				hasResults = true
			}
			for _, b := range e.Blocks {
				if strings.Contains(b.Text, "BLOCK_FOR_STOP_RUN") {
					<-ctx.Done()
					close(ch)
					return ch, ctx.Err()
				}
			}
		}
		if !hasResults {
			for i := 0; i < 12; i++ {
				ch <- llm.StreamEvent{
					Type:       llm.StreamToolCall,
					ToolCallID: fmt.Sprintf("par_%d", i),
					ToolName:   "probe_concurrent_read",
					ToolInput:  json.RawMessage(fmt.Sprintf(`{"idx":%d}`, i)),
				}
			}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use"}
			close(ch)
			return ch, nil
		}
		ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "All 12 concurrent reads complete."}
		ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn"}
		close(ch)
		return ch, nil
	})
	parEng, _, _, _ := setupTestEngine(t, nil, parStreamer)
	_ = parEng.Hooks().MountPlugin(Plugin{
		ID: "parallel_probe",
		Setup: func(sc *Scope) error {
			t, _ := eng.Hooks().PluginTools()[0], true
			sc.RegisterTool(t)
			return nil
		},
	})
	parWS, err := parEng.Workspaces().Register(wsDir, "ParallelRepo")
	if err != nil {
		t.Fatalf("Register parWS: %v", err)
	}
	_, parStream, err := parEng.StartRun(ctx, RunRequest{
		WorkspaceID: parWS.ID,
		Prompt:      "Fan out 12 reads",
		AgentRole:   "coder",
	})
	if err != nil {
		t.Fatalf("StartRun parallel: %v", err)
	}
	_ = drainRunEvents(parStream)
	peak := maxObserved.Load()
	if peak < 2 || peak > 8 {
		t.Fatalf("expected concurrent read peak in [2, 8], got %d", peak)
	}

	// StopRun test: start blocking run, call StopRun, verify EventRunCancelled and session lock released!
	stopSess, err := parEng.Sessions().Create(parWS.ID, "Stoppable", "coder")
	if err != nil {
		t.Fatalf("Create stopSess: %v", err)
	}
	stopRunID, stopStream, err := parEng.StartRun(ctx, RunRequest{
		WorkspaceID: parWS.ID,
		SessionID:   stopSess.ID,
		Prompt:      "BLOCK_FOR_STOP_RUN",
		AgentRole:   "coder",
	})
	if err != nil {
		t.Fatalf("StartRun stoppable: %v", err)
	}
	// Wait for EventRunStarted before calling StopRun.
	for ev := range stopStream {
		if ev.Type == EventRunStarted {
			break
		}
	}
	if err := parEng.StopRun(stopRunID); err != nil {
		t.Fatalf("StopRun: %v", err)
	}
	stopRest := drainRunEvents(stopStream)
	sawCancelled := false
	for _, ev := range stopRest {
		if ev.Type == EventRunCancelled {
			sawCancelled = true
		}
	}
	if !sawCancelled {
		t.Fatalf("expected EventRunCancelled after StopRun, got %+v", stopRest)
	}
	// Session flock must be released immediately so another run can acquire it!
	unlockAfterStop, okLock, lErr := store.TryFlock(parEng.Sessions().RunLockPath(stopSess.ID))
	if lErr != nil || !okLock {
		t.Fatalf("expected session run lock released after StopRun, ok=%v err=%v", okLock, lErr)
	}
	unlockAfterStop()
}

func TestFixRound1ReviewFindings(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// =========================================================================
	// Critical #1: Client-side compaction anchor emits valid Type="text" user block
	// on Anthropic /v1/messages wire (never raw {"type":"compaction", ...} in user message).
	// =========================================================================
	t.Run("Critical1_CompactionAnchorValidAnthropicUserTextBlock", func(t *testing.T) {
		var capturedAnthropicBody []byte
		var wireMu sync.Mutex

		mockAnthropic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			wireMu.Lock()
			capturedAnthropicBody = append([]byte(nil), raw...)
			wireMu.Unlock()

			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":50,\"output_tokens\":1}}}\n\n")
			fmt.Fprint(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"ok\"}}\n\n")
			fmt.Fprint(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
			fmt.Fprint(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n")
			fmt.Fprint(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		}))
		defer mockAnthropic.Close()

		utilStreamer := llm.StreamerFunc(func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
			ch := make(chan llm.StreamEvent, 4)
			ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "## Goal & Intent\nWire format safety check."}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", InputTokens: 30, OutputTokens: 10}
			close(ch)
			return ch, nil
		})
		eng, _, _, _ := setupTestEngine(t, nil, utilStreamer)
		ws, err := eng.Workspaces().Register(t.TempDir(), "CompactionWireRepo")
		if err != nil {
			t.Fatalf("Register ws: %v", err)
		}
		sess, err := eng.Sessions().Create(ws.ID, "Wire Check", "coder")
		if err != nil {
			t.Fatalf("Create session: %v", err)
		}
		for i := 1; i <= 4; i++ {
			_, _ = eng.Sessions().Append(sess.ID, store.SessionEntry{
				Role:   "user",
				Blocks: []store.ContentBlock{{Type: "text", Text: fmt.Sprintf("Turn %d user", i)}},
			})
			_, _ = eng.Sessions().Append(sess.ID, store.SessionEntry{
				Role:   "assistant",
				Blocks: []store.ContentBlock{{Type: "text", Text: fmt.Sprintf("Turn %d assistant", i)}},
			})
		}

		if err := eng.CompactSession(ctx, sess.ID); err != nil {
			t.Fatalf("CompactSession: %v", err)
		}
		branch, err := eng.Sessions().LoadActiveBranch(sess.ID)
		if err != nil || len(branch) == 0 {
			t.Fatalf("LoadActiveBranch: len=%d err=%v", len(branch), err)
		}
		anchorBlk := branch[0].Blocks[0]
		if anchorBlk.Type != "text" {
			t.Fatalf("expected anchor block Type='text', got %q", anchorBlk.Type)
		}
		if bytes.Contains(anchorBlk.CompactionData, []byte(`"type"`)) {
			t.Fatalf("expected CompactionData metadata without 'type' key, got %s", anchorBlk.CompactionData)
		}
		if len(anchorBlk.CompactionData) == 0 {
			t.Fatal("expected CompactionData metadata preserved on anchor block")
		}

		// Stream compacted branch through real Anthropic wire serializer and verify messages[0].content[0].type == "text".
		native := llm.NewNativeStreamer(mockAnthropic.Client())
		ch, err := native.Stream(ctx, llm.Request{
			Provider: config.ProviderConfig{
				ID:       "anthropic",
				Protocol: "anthropic",
				BaseURL:  mockAnthropic.URL,
			},
			APIKey:  "sk-ant-test",
			Model:   "claude-sonnet-5-5",
			History: branch,
		})
		if err != nil {
			t.Fatalf("NativeStreamer.Stream: %v", err)
		}
		for ev := range ch {
			if ev.Type == llm.StreamError {
				t.Fatalf("Anthropic stream error: %v", ev.Err)
			}
		}

		wireMu.Lock()
		wireBytes := append([]byte(nil), capturedAnthropicBody...)
		wireMu.Unlock()

		var wirePayload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(wireBytes, &wirePayload); err != nil {
			t.Fatalf("unmarshal Anthropic wire payload: %v", err)
		}
		if len(wirePayload.Messages) == 0 || wirePayload.Messages[0].Role != "user" || len(wirePayload.Messages[0].Content) == 0 {
			t.Fatalf("unexpected wire messages[0]: %s", wireBytes)
		}
		if gotType := wirePayload.Messages[0].Content[0].Type; gotType != "text" {
			t.Fatalf("expected Anthropic messages[0].content[0].type == 'text', got %q (payload=%s)", gotType, wireBytes)
		}
		if !strings.Contains(wirePayload.Messages[0].Content[0].Text, "<anchored_summary>") {
			t.Fatalf("expected <anchored_summary> in messages[0].content[0].text, got %q", wirePayload.Messages[0].Content[0].Text)
		}
	})

	// =========================================================================
	// Critical #2: Hard-Floor vault check blocks mixed `<vaultRoot>/providers.toml && <worktreePath>` commands.
	// =========================================================================
	t.Run("Critical2_HardFloorVaultCheckWithManagedWorktreePath", func(t *testing.T) {
		base := t.TempDir()
		vaultRoot := filepath.Join(base, "vault")
		wtDir := filepath.Join(vaultRoot, "runtime", "worktrees", "wt-alpha")
		if err := os.MkdirAll(wtDir, 0o700); err != nil {
			t.Fatalf("mkdir wtDir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(vaultRoot, "providers.toml"), []byte("secret=1\n"), 0o600); err != nil {
			t.Fatalf("write providers.toml: %v", err)
		}
		pg, err := tools.NewPathGuard(wtDir, vaultRoot)
		if err != nil {
			t.Fatalf("NewPathGuard(wtDir): %v", err)
		}

		gate := NewPermissionGate(nil)
		evalCtx := EvalContext{
			WorkspaceID:   "ws-wt",
			WorkspaceRoot: wtDir,
			VaultRoot:     vaultRoot,
			EffectiveMode: PermModeYolo,
			PathGuard:     pg,
		}
		runCmdMeta := tools.Tool{Name: "run_command", Mutating: true}

		// Legitimate command referencing only worktree path -> allowed in yolo mode
		legitCmd, _ := json.Marshal(map[string]string{"command": fmt.Sprintf("ls -la %s", wtDir)})
		if res := gate.Evaluate(evalCtx, ToolInvocation{ToolName: "run_command", Args: legitCmd, Tool: &runCmdMeta}); res.Action != GateAllow {
			t.Fatalf("expected legitimate worktree command allowed, got %+v", res)
		}

		// Mixed command referencing BOTH vault secret file AND worktree path -> MUST be hard-floor denied!
		mixedCmd, _ := json.Marshal(map[string]string{
			"command": fmt.Sprintf("cat %s && ls %s", filepath.Join(vaultRoot, "providers.toml"), wtDir),
		})
		resMixed := gate.Evaluate(evalCtx, ToolInvocation{ToolName: "run_command", Args: mixedCmd, Tool: &runCmdMeta})
		if resMixed.Action != GateDeny || resMixed.ReasonCode != "hard-floor" {
			t.Fatalf("expected hard-floor GateDeny on mixed vault+worktree command, got %+v", resMixed)
		}

		// Traversal command out of worktree into vault -> MUST be hard-floor denied!
		travCmd, _ := json.Marshal(map[string]string{
			"command": fmt.Sprintf("cat %s/../../../providers.toml", wtDir),
		})
		resTrav := gate.Evaluate(evalCtx, ToolInvocation{ToolName: "run_command", Args: travCmd, Tool: &runCmdMeta})
		if resTrav.Action != GateDeny || resTrav.ReasonCode != "hard-floor" {
			t.Fatalf("expected hard-floor GateDeny on worktree ../ traversal command, got %+v", resTrav)
		}
	})

	// =========================================================================
	// Important #1: GateDeny / PreToolUse deny precedes tryStageForGate (no disk read / no old_string oracle leak).
	// =========================================================================
	t.Run("Important1_GateDenyPrecedesStageEditNoOracleLeak", func(t *testing.T) {
		var observedResults []string
		var resMu sync.Mutex

		streamer := llm.StreamerFunc(func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
			ch := make(chan llm.StreamEvent, 8)
			if strings.Contains(strings.ToLower(req.StaticSystem), "housekeeping utility") {
				ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Deny Precedence"}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn"}
				close(ch)
				return ch, nil
			}
			for _, e := range req.History {
				for _, b := range e.Blocks {
					if b.Type == "tool_result" {
						resMu.Lock()
						observedResults = append(observedResults, b.Text)
						resMu.Unlock()
						ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "done"}
						ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn"}
						close(ch)
						return ch, nil
					}
				}
			}
			// Deliberately pass a mismatched old_string on secret/cred.env!
			ch <- llm.StreamEvent{
				Type:       llm.StreamToolCall,
				ToolCallID: "probe_edit",
				ToolName:   "edit_file",
				ToolInput:  json.RawMessage(`{"path":"secret/cred.env","old_string":"WRONG_OLD_STRING_ORACLE","new_string":"x"}`),
			}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use"}
			close(ch)
			return ch, nil
		})

		eng, _, _, _ := setupTestEngine(t, nil, streamer)
		wsDir := t.TempDir()
		secretEnvPath := filepath.Join(wsDir, "secret", "cred.env")
		if err := os.MkdirAll(filepath.Dir(secretEnvPath), 0o755); err != nil {
			t.Fatalf("mkdir secret: %v", err)
		}
		if err := os.WriteFile(secretEnvPath, []byte("REAL_SECRET=topsecret\n"), 0o600); err != nil {
			t.Fatalf("write cred.env: %v", err)
		}
		// Add workspace rules.toml denying secret/cred.env (tightening deny_paths is always applied).
		if err := os.MkdirAll(filepath.Join(wsDir, ".gorness"), 0o755); err != nil {
			t.Fatalf("mkdir .gorness: %v", err)
		}
		rulesTOML := fmt.Sprintf("deny_paths = [%q]\n", filepath.ToSlash(secretEnvPath))
		if err := os.WriteFile(filepath.Join(wsDir, ".gorness", "rules.toml"), []byte(rulesTOML), 0o644); err != nil {
			t.Fatalf("write rules.toml: %v", err)
		}

		ws, err := eng.Workspaces().Register(wsDir, "OracleLeakRepo")
		if err != nil {
			t.Fatalf("Register ws: %v", err)
		}

		// Case A: Rules.DenyPaths (Step 1 hard-floor)
		_, s1, err := eng.StartRun(ctx, RunRequest{WorkspaceID: ws.ID, Prompt: "case deny_paths", PermMode: PermModeAuto})
		if err != nil {
			t.Fatalf("StartRun case A: %v", err)
		}
		_ = drainRunEvents(s1)

		// Case B: Plan mode (Step 2 plan-readonly) on non-denied normal file
		_ = os.Remove(filepath.Join(wsDir, ".gorness", "rules.toml"))
		_, s2, err := eng.StartRun(ctx, RunRequest{WorkspaceID: ws.ID, Prompt: "case plan", PermMode: PermModePlan})
		if err != nil {
			t.Fatalf("StartRun case B: %v", err)
		}
		_ = drainRunEvents(s2)

		// Case C: Subagent AllowedTools restriction
		_, s3, err := eng.StartRun(ctx, RunRequest{WorkspaceID: ws.ID, Prompt: "case acl", AllowedTools: []string{"read_file"}})
		if err != nil {
			t.Fatalf("StartRun case C: %v", err)
		}
		_ = drainRunEvents(s3)

		// Case D: PreToolUse Hook returning GateDeny
		unsub := eng.Hooks().OnPreToolUse(func(ctx context.Context, ev *PreToolUseEvent) (GateResult, error) {
			return GateResult{Action: GateDeny, ReasonCode: "policy-hook", Reason: "blocked by pre-tool hook"}, nil
		})
		eng.Hooks().Freeze()
		_, s4, err := eng.StartRun(ctx, RunRequest{WorkspaceID: ws.ID, Prompt: "case hook", PermMode: PermModeAuto})
		if err != nil {
			t.Fatalf("StartRun case D: %v", err)
		}
		_ = drainRunEvents(s4)
		unsub()
		eng.Hooks().Freeze()

		resMu.Lock()
		got := append([]string(nil), observedResults...)
		resMu.Unlock()
		if len(got) != 4 {
			t.Fatalf("expected 4 tool_result observations, got %d: %v", len(got), got)
		}
		wantCodes := []string{"[hard-floor]", "[plan-readonly]", "[subagent-tool-not-allowed]", "[policy-hook]"}
		for i, wc := range wantCodes {
			if strings.Contains(got[i], "old_string") {
				t.Fatalf("case %d leaked old_string match error before GateDeny: %q", i, got[i])
			}
			if !strings.Contains(got[i], "Permission denied "+wc) {
				t.Fatalf("case %d expected 'Permission denied %s', got %q", i, wc, got[i])
			}
		}
	})

	// =========================================================================
	// Important #2: Outside-workspace GateAllow passes AllowOutsideWorkspace=true
	// in both parallel read batches and Rules.AllowPaths serial edits!
	// =========================================================================
	t.Run("Important2_OutsideWorkspaceAllowPathsParallelReadAndSerialEdit", func(t *testing.T) {
		sharedDir := t.TempDir()
		docPath := filepath.Join(sharedDir, "shared_spec.md")
		editTarget := filepath.Join(sharedDir, "notes.txt")
		if err := os.WriteFile(docPath, []byte("SHARED_SPEC_TOKEN_777\n"), 0o644); err != nil {
			t.Fatalf("write shared_spec.md: %v", err)
		}
		if err := os.WriteFile(editTarget, []byte("status = draft\n"), 0o644); err != nil {
			t.Fatalf("write notes.txt: %v", err)
		}

		var toolErrors []string
		var errMu sync.Mutex

		streamer := llm.StreamerFunc(func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
			ch := make(chan llm.StreamEvent, 8)
			if strings.Contains(strings.ToLower(req.StaticSystem), "housekeeping utility") {
				ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Shared AllowPaths"}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn"}
				close(ch)
				return ch, nil
			}
			sawToolTurn := false
			for _, e := range req.History {
				if e.Role == "tool" {
					sawToolTurn = true
					for _, b := range e.Blocks {
						if b.IsError {
							errMu.Lock()
							toolErrors = append(toolErrors, fmt.Sprintf("%s: %s", b.ToolCallID, b.Text))
							errMu.Unlock()
						}
					}
				}
			}
			if sawToolTurn {
				ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Completed outside-workspace operations."}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn"}
				close(ch)
				return ch, nil
			}

			rArgs, _ := json.Marshal(map[string]string{"path": docPath})
			sArgs, _ := json.Marshal(map[string]string{"pattern": "SHARED_SPEC_TOKEN_777", "path": sharedDir})
			eArgs, _ := json.Marshal(map[string]string{"path": editTarget, "old_string": "status = draft", "new_string": "status = approved"})

			// Consecutive read_file + search_code outside wsRoot (runs in runParallelReadBatch!) + edit_file outside wsRoot!
			ch <- llm.StreamEvent{Type: llm.StreamToolCall, ToolCallID: "out_read", ToolName: "read_file", ToolInput: rArgs}
			ch <- llm.StreamEvent{Type: llm.StreamToolCall, ToolCallID: "out_search", ToolName: "search_code", ToolInput: sArgs}
			ch <- llm.StreamEvent{Type: llm.StreamToolCall, ToolCallID: "out_edit", ToolName: "edit_file", ToolInput: eArgs}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use"}
			close(ch)
			return ch, nil
		})

		eng, cfgMgr, _, _ := setupTestEngine(t, nil, streamer)
		// Notice we pass ONLY the un-canonicalized sharedDir + "/*" to also verify /var -> /private/var pattern canonicalization!
		rulesBody := fmt.Sprintf("allow_paths = [%q]\n", filepath.ToSlash(sharedDir)+"/*")
		if err := os.WriteFile(cfgMgr.Paths().RulesFile(), []byte(rulesBody), 0o600); err != nil {
			t.Fatalf("write global rules.toml: %v", err)
		}
		reloadedCfg, err := config.NewManager(config.Options{
			VaultRoot: cfgMgr.Paths().Root,
			EnvLookup: func(key string) (string, bool) {
				if key == "ANTHROPIC_API_KEY" {
					return "sk-ant-test-key", true
				}
				return "", false
			},
		})
		if err != nil {
			t.Fatalf("reload config manager: %v", err)
		}
		eng2, err := NewEngine(EngineOptions{
			Config:     reloadedCfg,
			Workspaces: eng.Workspaces(),
			Sessions:   eng.Sessions(),
			Snapshots:  eng.Snapshots(),
			Router:     llm.NewRouter(reloadedCfg, reloadedCfg.Secrets(), streamer),
		})
		if err != nil {
			t.Fatalf("NewEngine eng2: %v", err)
		}
		defer eng2.Close()

		ws, err := eng2.Workspaces().Register(t.TempDir(), "OutsideAllowRepo")
		if err != nil {
			t.Fatalf("Register ws: %v", err)
		}
		_, runStream, err := eng2.StartRun(ctx, RunRequest{
			WorkspaceID: ws.ID,
			Prompt:      "Read, search, and edit shared directory",
			PermMode:    PermModeAcceptEdits,
		})
		if err != nil {
			t.Fatalf("StartRun: %v", err)
		}
		evs := drainRunEvents(runStream)
		sawCompleted := false
		for _, ev := range evs {
			if ev.Type == EventRunFailed {
				t.Fatalf("unexpected EventRunFailed: %+v", ev.Payload)
			}
			if ev.Type == EventRunCompleted {
				sawCompleted = true
			}
		}
		if !sawCompleted {
			t.Fatalf("expected EventRunCompleted, got %+v", evs)
		}

		errMu.Lock()
		errs := append([]string(nil), toolErrors...)
		errMu.Unlock()
		if len(errs) != 0 {
			t.Fatalf("expected 0 tool errors for AllowPaths parallel reads & serial edit, got %v", errs)
		}
		updatedNotes, _ := os.ReadFile(editTarget)
		if !strings.Contains(string(updatedNotes), "status = approved") {
			t.Fatalf("expected outside-workspace notes.txt edited cleanly, got %q", updatedNotes)
		}
	})

	// =========================================================================
	// Important #3: Multi-tool step [call_1 (ok), call_2 (GateDefer), call_3 (pending)]
	// pairs all 3 tool_use IDs on both ResumeApproval and normal follow-up run!
	// =========================================================================
	t.Run("Important3_SiblingCompletedBeforeGateDeferPairsAllToolUses", func(t *testing.T) {
		var resumedHistory []store.SessionEntry
		var histMu sync.Mutex

		streamer := llm.StreamerFunc(func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
			ch := make(chan llm.StreamEvent, 8)
			if strings.Contains(strings.ToLower(req.StaticSystem), "housekeeping utility") {
				ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Sibling Defer"}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn"}
				close(ch)
				return ch, nil
			}
			// Check if call_2 already has a tool_result
			for _, e := range req.History {
				for _, b := range e.Blocks {
					if b.Type == "tool_result" && b.ToolCallID == "sib_call_2" {
						histMu.Lock()
						resumedHistory = append([]store.SessionEntry(nil), req.History...)
						histMu.Unlock()
						ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "All 3 sibling tool_uses paired!"}
						ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn"}
						close(ch)
						return ch, nil
					}
				}
			}

			ch <- llm.StreamEvent{
				Type:       llm.StreamToolCall,
				ToolCallID: "sib_call_1",
				ToolName:   "read_file",
				ToolInput:  json.RawMessage(`{"path":"readme.txt"}`),
			}
			ch <- llm.StreamEvent{
				Type:       llm.StreamToolCall,
				ToolCallID: "sib_call_2",
				ToolName:   "write_file",
				ToolInput:  json.RawMessage(`{"path":".gorness/hook.toml","content":"enabled = true\n"}`),
			}
			ch <- llm.StreamEvent{
				Type:       llm.StreamToolCall,
				ToolCallID: "sib_call_3",
				ToolName:   "write_file",
				ToolInput:  json.RawMessage(`{"path":"after.txt","content":"later\n"}`),
			}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use"}
			close(ch)
			return ch, nil
		})

		eng, _, _, _ := setupTestEngine(t, nil, streamer)
		wsDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(wsDir, "readme.txt"), []byte("hello\n"), 0o644); err != nil {
			t.Fatalf("write readme.txt: %v", err)
		}
		ws, err := eng.Workspaces().Register(wsDir, "SiblingDeferRepo")
		if err != nil {
			t.Fatalf("Register ws: %v", err)
		}
		sess, err := eng.Sessions().Create(ws.ID, "Multi-Tool Defer", "coder")
		if err != nil {
			t.Fatalf("Create session: %v", err)
		}

		// Run 1 (unattended): sib_call_1 succeeds (appending Role="tool"), sib_call_2 hits GateDefer, sib_call_3 unexecuted!
		_, st1, err := eng.StartRun(ctx, RunRequest{
			WorkspaceID: ws.ID,
			SessionID:   sess.ID,
			Prompt:      "Run 3 tools",
			Unattended:  true,
		})
		if err != nil {
			t.Fatalf("StartRun 1: %v", err)
		}
		var def *DeferredToolPayload
		for ev := range st1 {
			if ev.Type == EventToolDeferred {
				p := ev.Payload.(DeferredToolPayload)
				def = &p
			}
		}
		if def == nil || def.ToolCallID != "sib_call_2" {
			t.Fatalf("expected sib_call_2 deferred, got %+v", def)
		}

		// Run 2: Resume sib_call_2 -> must execute sib_call_2 AND synthesize interrupted tool_result for sib_call_3!
		_, st2, err := eng.StartRun(ctx, RunRequest{
			WorkspaceID: ws.ID,
			SessionID:   sess.ID,
			Unattended:  true,
			ResumeApproval: &DeferredResume{
				DeferredID: def.ID,
				ToolCallID: def.ToolCallID,
				ToolName:   def.ToolName,
				Args:       def.Args,
				Approved:   true,
			},
		})
		if err != nil {
			t.Fatalf("StartRun 2 resume: %v", err)
		}
		_ = drainRunEvents(st2)

		histMu.Lock()
		hist := append([]store.SessionEntry(nil), resumedHistory...)
		histMu.Unlock()

		paired := make(map[string]store.ContentBlock)
		for _, e := range hist {
			for _, b := range e.Blocks {
				if b.Type == "tool_result" {
					paired[b.ToolCallID] = b
				}
			}
		}
		for _, wantID := range []string{"sib_call_1", "sib_call_2", "sib_call_3"} {
			if _, ok := paired[wantID]; !ok {
				t.Fatalf("missing paired tool_result for %q in resumed history: %+v", wantID, hist)
			}
		}
		if paired["sib_call_1"].IsError || paired["sib_call_2"].IsError {
			t.Fatalf("expected sib_call_1 and sib_call_2 to succeed, got %+v / %+v", paired["sib_call_1"], paired["sib_call_2"])
		}
		if !paired["sib_call_3"].IsError {
			t.Fatalf("expected unexecuted sibling sib_call_3 marked IsError=true, got %+v", paired["sib_call_3"])
		}
	})

	// =========================================================================
	// Important #4: `search_tools` allowed when len(AllowedTools) > 0 (unless in DisallowedTools),
	// and blocked when AllowedTools == []string{}!
	// =========================================================================
	t.Run("Important4_SearchToolsACLAlignedWithVisibleSchemas", func(t *testing.T) {
		var step atomic.Int32
		var searchToolErr bool
		var step2VisibleNames []string
		var forbiddenCallText string
		var mu sync.Mutex

		streamer := llm.StreamerFunc(func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
			ch := make(chan llm.StreamEvent, 8)
			if strings.Contains(strings.ToLower(req.StaticSystem), "housekeeping utility") {
				ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "BM25 AllowedTools"}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn"}
				close(ch)
				return ch, nil
			}
			n := step.Add(1)
			switch n {
			case 1:
				ch <- llm.StreamEvent{
					Type:       llm.StreamToolCall,
					ToolCallID: "st_1",
					ToolName:   "search_tools",
					ToolInput:  json.RawMessage(`{"query":"ext_allowed_alpha ext_forbidden_beta","limit":5}`),
				}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use"}
			case 2:
				mu.Lock()
				for _, ts := range req.Tools {
					step2VisibleNames = append(step2VisibleNames, ts.Name)
				}
				for _, e := range req.History {
					for _, b := range e.Blocks {
						if b.Type == "tool_result" && b.ToolCallID == "st_1" && b.IsError {
							searchToolErr = true
						}
					}
				}
				mu.Unlock()
				// Call both allowed extension tool and forbidden extension tool
				ch <- llm.StreamEvent{
					Type:       llm.StreamToolCall,
					ToolCallID: "call_alpha",
					ToolName:   "ext_allowed_alpha",
					ToolInput:  json.RawMessage(`{}`),
				}
				ch <- llm.StreamEvent{
					Type:       llm.StreamToolCall,
					ToolCallID: "call_beta",
					ToolName:   "ext_forbidden_beta",
					ToolInput:  json.RawMessage(`{}`),
				}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use"}
			default:
				mu.Lock()
				for _, e := range req.History {
					for _, b := range e.Blocks {
						if b.Type == "tool_result" && b.ToolCallID == "call_beta" {
							forbiddenCallText = b.Text
						}
					}
				}
				mu.Unlock()
				ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "done"}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn"}
			}
			close(ch)
			return ch, nil
		})

		eng, _, _, _ := setupTestEngine(t, nil, streamer)
		// Mount 10 deferred plugin tools so total tools > 12 (BM25DeferThreshold)
		if err := eng.Hooks().MountPlugin(Plugin{
			ID: "deferred_suite",
			Setup: func(sc *Scope) error {
				names := []string{
					"ext_allowed_alpha",
					"ext_forbidden_beta",
					"ext_gamma_3", "ext_gamma_4", "ext_gamma_5",
					"ext_gamma_6", "ext_gamma_7", "ext_gamma_8",
					"ext_gamma_9", "ext_gamma_10",
				}
				for _, nm := range names {
					nm := nm
					sc.RegisterTool(tools.Tool{
						Name:            nm,
						Description:     "Deferred extension tool " + nm,
						ReadOnly:        true,
						ConcurrencySafe: true,
						DeferLoad:       true,
						Handler: func(ctx context.Context, call tools.CallContext, rawInput json.RawMessage) (tools.ToolResult, error) {
							return tools.ToolResult{Output: "ran:" + nm}, nil
						},
					})
				}
				return nil
			},
		}); err != nil {
			t.Fatalf("MountPlugin: %v", err)
		}

		ws, err := eng.Workspaces().Register(t.TempDir(), "SearchToolsACLRepo")
		if err != nil {
			t.Fatalf("Register ws: %v", err)
		}
		_, st, err := eng.StartRun(ctx, RunRequest{
			WorkspaceID:  ws.ID,
			Prompt:       "Activate and invoke ext_allowed_alpha",
			AllowedTools: []string{"read_file", "ext_allowed_alpha"},
		})
		if err != nil {
			t.Fatalf("StartRun: %v", err)
		}
		_ = drainRunEvents(st)

		mu.Lock()
		hadSearchErr := searchToolErr
		vis2 := append([]string(nil), step2VisibleNames...)
		betaOut := forbiddenCallText
		mu.Unlock()

		if hadSearchErr {
			t.Fatal("expected search_tools to succeed under non-empty AllowedTools")
		}
		if !containsString(vis2, "ext_allowed_alpha") {
			t.Fatalf("expected ext_allowed_alpha visible after search_tools, got %v", vis2)
		}
		if containsString(vis2, "ext_forbidden_beta") {
			t.Fatalf("expected ext_forbidden_beta hidden by AllowedTools filter, got %v", vis2)
		}
		if !strings.Contains(betaOut, "Permission denied [subagent-tool-not-allowed]") {
			t.Fatalf("expected ext_forbidden_beta blocked by ACL, got %q", betaOut)
		}

		// Also verify direct Gate ACL checks: empty AllowedTools=[] blocks search_tools, and DisallowedTools=["search_tools"] blocks search_tools!
		gate := NewPermissionGate(nil)
		searchMeta := tools.Tool{Name: "search_tools", ReadOnly: true, ConcurrencySafe: true}
		if resEmpty := gate.Evaluate(EvalContext{
			EffectiveMode: PermModeAuto,
			AllowedTools:  []string{},
		}, ToolInvocation{ToolName: "search_tools", Tool: &searchMeta}); resEmpty.Action != GateDeny {
			t.Fatalf("expected empty AllowedTools=[] to deny search_tools, got %+v", resEmpty)
		}
		if resDisallowed := gate.Evaluate(EvalContext{
			EffectiveMode: PermModeAuto,
			AgentProfile: &config.AgentProfile{
				ID:              "strict",
				AllowedTools:    []string{"read_file"},
				DisallowedTools: []string{"search_tools"},
			},
		}, ToolInvocation{ToolName: "search_tools", Tool: &searchMeta}); resDisallowed.Action != GateDeny || resDisallowed.ReasonCode != "agent-disallowed-tool" {
			t.Fatalf("expected DisallowedTools=['search_tools'] to deny search_tools, got %+v", resDisallowed)
		}
	})
}

func TestFinalFixWaveGitConfigGateGeminiToolSigFallbackModelPartialMCPAndRehydration(t *testing.T) {
	// 1. PermissionGate: .git/config must trigger auto-run-config GateAsk in both auto and yolo modes (file tools & shell commands).
	t.Run("git_config_auto_run_gate", func(t *testing.T) {
		wsRoot := t.TempDir()
		vaultRoot := filepath.Join(t.TempDir(), "vault")
		pg, err := tools.NewPathGuard(wsRoot, vaultRoot)
		if err != nil {
			t.Fatalf("NewPathGuard: %v", err)
		}
		gate := NewPermissionGate(nil)
		for _, mode := range []string{PermModeAuto, PermModeYolo} {
			evalCtx := EvalContext{
				WorkspaceID:   "ws-gitcfg",
				WorkspaceRoot: wsRoot,
				VaultRoot:     vaultRoot,
				EffectiveMode: mode,
				PathGuard:     pg,
			}
			resWrite := gate.Evaluate(evalCtx, ToolInvocation{
				ToolName: "write_file",
				Args:     json.RawMessage(`{"path":".git/config","content":"[core]\n\tfsmonitor = evil\n"}`),
			})
			if resWrite.Action != GateAsk || resWrite.ReasonCode != "auto-run-config" {
				t.Fatalf("mode=%s write_file .git/config expected GateAsk/auto-run-config, got %+v", mode, resWrite)
			}
			resCmd := gate.Evaluate(evalCtx, ToolInvocation{
				ToolName: "run_command",
				Args:     json.RawMessage(`{"command":"printf '[core]\nfsmonitor=evil\n' >> .git/config"}`),
			})
			if resCmd.Action != GateAsk || resCmd.ReasonCode != "auto-run-config" {
				t.Fatalf("mode=%s run_command .git/config expected GateAsk/auto-run-config, got %+v", mode, resCmd)
			}
		}
	})

	// 2. Gemini tool_use thoughtSignature persistence + Fallback answering ModelID & pricing + Partial MCP resilience + Cross-turn rehydration!
	t.Run("gemini_sig_fallback_cost_partial_mcp_and_rehydration", func(t *testing.T) {
		var mu sync.Mutex
		var turn2Step1Tools []string
		stepCounter := 0

		streamer := llm.StreamerFunc(func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
			if strings.Contains(req.StaticSystem, "Housekeeping Utility") {
				ch := make(chan llm.StreamEvent, 2)
				ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Rehydration Session"}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn"}
				close(ch)
				return ch, nil
			}

			// Simulate primary anthropic failure when prompt requests FALLBACK_TEST so fallback openai/gpt-5.5 answers!
			var lastUser string
			for _, ent := range req.History {
				if ent.Role == "user" {
					for _, b := range ent.Blocks {
						if b.Text != "" {
							lastUser = b.Text
						}
					}
				}
			}
			if strings.Contains(lastUser, "FALLBACK_TEST") {
				if req.Provider.ID == "anthropic" {
					return nil, &llm.APIError{ProviderID: "anthropic", StatusCode: 400, Message: "primary unavailable", Retryable: false}
				}
				ch := make(chan llm.StreamEvent, 2)
				ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Fallback gpt-5.5 reply"}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "completed", InputTokens: 1_000_000, OutputTokens: 1_000_000}
				close(ch)
				return ch, nil
			}

			mu.Lock()
			stepCounter++
			curStep := stepCounter
			if strings.Contains(lastUser, "TURN2_REHYDRATE_CHECK") {
				turn2Step1Tools = make([]string, 0, len(req.Tools))
				for _, ts := range req.Tools {
					turn2Step1Tools = append(turn2Step1Tools, ts.Name)
				}
			}
			mu.Unlock()

			ch := make(chan llm.StreamEvent, 4)
			if curStep == 1 {
				// Emit search_tools + activate_skill with Gemini thoughtSignature!
				ch <- llm.StreamEvent{
					Type:       llm.StreamToolCall,
					ToolCallID: "gem_tc_search",
					ToolName:   "search_tools",
					ToolInput:  json.RawMessage(`{"query":"healthy cluster ping","limit":5}`),
					Signature:  "gem_tool_sig_alpha==",
				}
				ch <- llm.StreamEvent{
					Type:       llm.StreamToolCall,
					ToolCallID: "gem_tc_skill",
					ToolName:   "activate_skill",
					ToolInput:  json.RawMessage(`{"name":"go-hygiene"}`),
					Signature:  "gem_tool_sig_beta==",
				}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use", InputTokens: 50, OutputTokens: 20}
				close(ch)
				return ch, nil
			}
			ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Done turn"}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "STOP", InputTokens: 40, OutputTokens: 10}
			close(ch)
			return ch, nil
		})

		eng, cfgMgr, _, _ := setupTestEngine(t, nil, streamer)
		wsDir := t.TempDir()

		// Create a workspace skill so activate_skill exists.
		skillDir := filepath.Join(wsDir, ".gorness", "skills", "go-hygiene")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatalf("mkdir skill: %v", err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: go-hygiene\ndescription: Go hygiene rules\npaths: [\"**/*.go\"]\n---\nHYGIENE_BODY\n"), 0o644); err != nil {
			t.Fatalf("write SKILL.md: %v", err)
		}
		if err := os.WriteFile(filepath.Join(wsDir, "sample.go"), []byte("package sample\n"), 0o644); err != nil {
			t.Fatalf("write sample.go: %v", err)
		}

		// Start a healthy HTTP MCP server exposing 10 tools (so total tools > 12 => BM25 folding active!),
		// alongside a broken MCP server in global mcp.toml!
		healthyMCP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var rpc struct {
				ID     any    `json:"id"`
				Method string `json:"method"`
			}
			_ = json.NewDecoder(r.Body).Decode(&rpc)
			w.Header().Set("Content-Type", "application/json")
			switch rpc.Method {
			case "initialize":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      rpc.ID,
					"result":  map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}},
				})
			case "notifications/initialized":
				w.WriteHeader(http.StatusAccepted)
			case "tools/list":
				var list []map[string]any
				list = append(list, map[string]any{
					"name":        "cluster_ping",
					"description": "Healthy cluster ping diagnostic tool",
				})
				for i := 1; i <= 9; i++ {
					list = append(list, map[string]any{
						"name":        fmt.Sprintf("extra_tool_%d", i),
						"description": fmt.Sprintf("Extra filler tool %d", i),
					})
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      rpc.ID,
					"result":  map[string]any{"tools": list},
				})
			}
		}))
		defer healthyMCP.Close()

		mcpTOML := fmt.Sprintf(`[servers.broken_srv]
transport = "stdio"
command = "/nonexistent/gorness-broken-mcp"

[servers.healthy_srv]
transport = "http"
url = %q
`, healthyMCP.URL)
		if err := store.AtomicWriteFile(cfgMgr.Paths().MCPFile(), []byte(mcpTOML), 0o600); err != nil {
			t.Fatalf("write mcp.toml: %v", err)
		}
		if _, err := cfgMgr.Reload(); err != nil {
			t.Fatalf("Reload mcp.toml: %v", err)
		}

		ws, err := eng.Workspaces().Register(wsDir, "FixWaveRepo")
		if err != nil {
			t.Fatalf("Register ws: %v", err)
		}
		sess, err := eng.Sessions().Create(ws.ID, "Gemini & Rehydrate Session", "coder")
		if err != nil {
			t.Fatalf("Create session: %v", err)
		}

		ctx := context.Background()
		// Turn 1: Gemini model override -> emits tool_use with Signature and activates mcp__healthy_srv__cluster_ping via search_tools!
		_, evs1, err := eng.StartRun(ctx, RunRequest{
			WorkspaceID:   ws.ID,
			SessionID:     sess.ID,
			Prompt:        "Activate cluster ping tool and hygiene skill",
			ModelOverride: "gemini/gemini-2.5-pro",
		})
		if err != nil {
			t.Fatalf("Turn 1 StartRun: %v", err)
		}
		_ = drainRunEvents(evs1)

		branch1, err := eng.Sessions().LoadActiveBranch(sess.ID)
		if err != nil {
			t.Fatalf("LoadActiveBranch turn 1: %v", err)
		}
		foundSig := false
		for _, ent := range branch1 {
			for _, blk := range ent.Blocks {
				if blk.Type == "tool_use" && blk.ToolCallID == "gem_tc_search" && blk.Signature == "gem_tool_sig_alpha==" {
					foundSig = true
				}
			}
		}
		if !foundSig {
			t.Fatalf("expected Gemini tool_use Signature persisted in session jsonl, got branch=%+v", branch1)
		}

		// Turn 2: Fresh StartRun on same session -> Step 1 MUST rehydrate mcp__healthy_srv__cluster_ping immediately!
		_, evs2, err := eng.StartRun(ctx, RunRequest{
			WorkspaceID:   ws.ID,
			SessionID:     sess.ID,
			Prompt:        "TURN2_REHYDRATE_CHECK",
			ModelOverride: "gemini/gemini-2.5-pro",
		})
		if err != nil {
			t.Fatalf("Turn 2 StartRun: %v", err)
		}
		_ = drainRunEvents(evs2)

		mu.Lock()
		gotTurn2Tools := append([]string(nil), turn2Step1Tools...)
		mu.Unlock()
		if !containsString(gotTurn2Tools, "mcp__healthy_srv__cluster_ping") {
			t.Fatalf("expected mcp__healthy_srv__cluster_ping rehydrated on Turn 2 Step 1 despite broken sibling MCP server, got %v", gotTurn2Tools)
		}

		// Turn 3 (separate session): Fallback from coder primary (claude-sonnet-5-5: $3+$15=$18) to fallback (gpt-5.5: $2.5+$10=$12.5)!
		fbSess, err := eng.Sessions().Create(ws.ID, "Fallback Accounting", "coder")
		if err != nil {
			t.Fatalf("Create fbSess: %v", err)
		}
		_, fbStream, err := eng.StartRun(ctx, RunRequest{
			WorkspaceID: ws.ID,
			SessionID:   fbSess.ID,
			Prompt:      "FALLBACK_TEST",
			AgentRole:   "coder",
		})
		if err != nil {
			t.Fatalf("Fallback StartRun: %v", err)
		}
		fbEvents := drainRunEvents(fbStream)
		var fbOutcome *RunOutcome
		for _, ev := range fbEvents {
			if ev.Type == EventRunCompleted {
				if out, ok := ev.Payload.(RunOutcome); ok {
					cp := out
					fbOutcome = &cp
				}
			}
		}
		if fbOutcome == nil {
			t.Fatal("expected EventRunCompleted on fallback run")
		}
		if fbOutcome.CostUSD < 12.49 || fbOutcome.CostUSD > 12.51 {
			t.Fatalf("expected fallback gpt-5.5 cost $12.50 (not primary $18.00), got $%.4f", fbOutcome.CostUSD)
		}
		fbBranch, err := eng.Sessions().LoadActiveBranch(fbSess.ID)
		if err != nil {
			t.Fatalf("LoadActiveBranch fbSess: %v", err)
		}
		var recordedAssistantModel string
		for _, ent := range fbBranch {
			if ent.Role == "assistant" {
				recordedAssistantModel = ent.ModelID
			}
		}
		if recordedAssistantModel != "gpt-5.5" {
			t.Fatalf("expected assistant SessionEntry.ModelID='gpt-5.5' after fallback, got %q", recordedAssistantModel)
		}
	})
}

func TestSlice1CoreHooksTrustReadHashesMCPPoolCompactionSubagentAndSubscriptions(t *testing.T) {
	var (
		turnStoppingCalls atomic.Int32
		subagentHit       atomic.Int32
		liveStreamGate    = make(chan struct{})
	)

	streamer := llm.StreamerFunc(func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
		ch := make(chan llm.StreamEvent, 16)
		var allText, lastText string
		for i, ent := range req.History {
			for _, b := range ent.Blocks {
				if b.Text != "" {
					allText += b.Text + "\n"
					if i == len(req.History)-1 {
						lastText += b.Text + "\n"
					}
				}
			}
		}

		go func() {
			defer close(ch)

			// Utility prompts (RunUtilityPrompt, Compactor, AutoTitler)
			if strings.Contains(req.StaticSystem, "Housekeeping Utility") || strings.Contains(lastText, "<turns_to_compact>") {
				ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "## Goal & Intent\nAnchored auto-compaction summary.\n## Key Decisions & Context\nKept 30% recent turns."}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", ModelID: req.Model, InputTokens: 20, OutputTokens: 15}
				return
			}

			switch {
			case strings.Contains(allText, "HOOK_PRE_BLOCK"):
				if strings.Contains(lastText, "blocked by policy stderr") {
					ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Observed pre-hook block feedback."}
					ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", ModelID: req.Model, InputTokens: 50, OutputTokens: 10}
					return
				}
				ch <- llm.StreamEvent{
					Type:       llm.StreamToolCall,
					ToolCallID: "tc_hook_block",
					ToolName:   "write_file",
					ToolInput:  json.RawMessage(`{"path":"blocked.txt","content":"nope"}`),
				}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use", ModelID: req.Model, InputTokens: 40, OutputTokens: 12}

			case strings.Contains(allText, "HOOK_POST_AND_STOP"):
				if strings.Contains(allText, "Verify hook marker before stopping") {
					ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Verified after turn_stopping veto."}
					ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", ModelID: req.Model, InputTokens: 60, OutputTokens: 12}
					return
				}
				if strings.Contains(lastText, "<untrusted_content") || strings.Contains(lastText, "allowed.txt") {
					turnStoppingCalls.Add(1)
					ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Initial completion attempt."}
					ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", ModelID: req.Model, InputTokens: 55, OutputTokens: 10}
					return
				}
				ch <- llm.StreamEvent{
					Type:       llm.StreamToolCall,
					ToolCallID: "tc_hook_post",
					ToolName:   "write_file",
					ToolInput:  json.RawMessage(`{"path":"allowed.txt","content":"ok\n"}`),
				}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use", ModelID: req.Model, InputTokens: 45, OutputTokens: 12}

			case strings.Contains(lastText, "READHASH_EDIT_ONLY"):
				ch <- llm.StreamEvent{
					Type:       llm.StreamToolCall,
					ToolCallID: "tc_rh_edit",
					ToolName:   "edit_file",
					ToolInput:  json.RawMessage(`{"path":"target.txt","old_string":"alpha-v1","new_string":"alpha-v2"}`),
				}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use", ModelID: req.Model, InputTokens: 30, OutputTokens: 12}

			case strings.Contains(allText, "READHASH_EDIT_ONLY"):
				ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Edit attempt finished."}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", ModelID: req.Model, InputTokens: 35, OutputTokens: 8}

			case strings.Contains(allText, "READHASH_TURN1_READ"):
				if strings.Contains(lastText, "alpha-v1") {
					ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Read target.txt."}
					ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", ModelID: req.Model, InputTokens: 30, OutputTokens: 8}
					return
				}
				ch <- llm.StreamEvent{
					Type:       llm.StreamToolCall,
					ToolCallID: "tc_rh_read",
					ToolName:   "read_file",
					ToolInput:  json.RawMessage(`{"path":"target.txt"}`),
				}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use", ModelID: req.Model, InputTokens: 25, OutputTokens: 10}

			case strings.Contains(allText, "AUTO_COMPACT_TRIGGER"):
				if strings.Contains(lastText, "compaction-probe-ok") {
					ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Completed after auto-compaction."}
					ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", ModelID: req.Model, InputTokens: 4000, OutputTokens: 20}
					return
				}
				// Return InputTokens = 180,000 (>= 85% of 200,000 ContextWindow) so maybeAutoCompact fires right before next step!
				ch <- llm.StreamEvent{
					Type:       llm.StreamToolCall,
					ToolCallID: "tc_ac_1",
					ToolName:   "dyn_probe",
					ToolInput:  json.RawMessage(`{}`),
				}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use", ModelID: req.Model, InputTokens: 180000, OutputTokens: 25}

			case strings.Contains(allText, "SUBAGENT_SCAN_TASK") && !strings.Contains(allText, "PARENT_DELEGATE_SUBAGENT"):
				subagentHit.Add(1)
				for _, sc := range req.Tools {
					if sc.Name == "spawn_subagent" || sc.Name == "write_file" || sc.Name == "edit_file" {
						ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "VIOLATION: forbidden tool exposed to explorer: " + sc.Name}
						ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", ModelID: req.Model, InputTokens: 10, OutputTokens: 10}
						return
					}
				}
				if strings.Contains(lastText, "alpha") {
					ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Explorer distilled report: target.txt line 1 confirmed."}
					ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", ModelID: req.Model, InputTokens: 28, OutputTokens: 14}
					return
				}
				ch <- llm.StreamEvent{
					Type:       llm.StreamToolCall,
					ToolCallID: "sub_tc_1",
					ToolName:   "read_file",
					ToolInput:  json.RawMessage(`{"path":"target.txt"}`),
				}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use", ModelID: req.Model, InputTokens: 20, OutputTokens: 10}

			case strings.Contains(allText, "PARENT_DELEGATE_SUBAGENT"):
				if strings.Contains(lastText, "[subagent:explorer") {
					ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Parent integrated explorer report."}
					ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", ModelID: req.Model, InputTokens: 45, OutputTokens: 12}
					return
				}
				ch <- llm.StreamEvent{
					Type:       llm.StreamToolCall,
					ToolCallID: "parent_sub_1",
					ToolName:   "spawn_subagent",
					ToolInput:  json.RawMessage(`{"agent_role":"explorer","prompt":"SUBAGENT_SCAN_TASK","paths":["target.txt"],"max_steps":99}`),
				}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use", ModelID: req.Model, InputTokens: 35, OutputTokens: 15}

			case strings.Contains(allText, "SUBSCRIBE_LIVE_RUN"):
				ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "chunk-before-attach "}
				<-liveStreamGate
				ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "chunk-after-attach"}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", ModelID: req.Model, InputTokens: 20, OutputTokens: 10}

			default:
				ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "ok"}
				ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "end_turn", ModelID: req.Model, InputTokens: 15, OutputTokens: 5}
			}
		}()
		return ch, nil
	})

	clock := newFakeClock(time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC))
	eng, cfgMgr, vaultRoot, _ := setupTestEngine(t, clock, streamer)
	ctx := context.Background()

	wsDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wsDir, ".gorness"), 0o755); err != nil {
		t.Fatalf("mkdir ws .gorness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, "target.txt"), []byte("alpha-v1\n"), 0o644); err != nil {
		t.Fatalf("write target.txt: %v", err)
	}

	// 1. Untrusted workspace hooks.toml skipped until TrustWorkspace(ws.ID) is called; GrantStore CRUD
	postMarker := filepath.Join(t.TempDir(), "post_hook_ran.marker")
	stopMarker := filepath.Join(t.TempDir(), "stop_hook_vetoed_once.marker")
	wsHooks := fmt.Sprintf(`
[[pre_tool_use]]
tool = "write_file"
command = "cat | grep -q 'blocked.txt' && { echo 'blocked by policy stderr sk-ant-test-key' >&2; exit 2; } || exit 0"

[[post_tool_use]]
tool = "write_file"
command = "printf 'ran' > '%s'"

[[turn_stopping]]
command = "if [ -f '%s' ] && [ ! -f '%s' ]; then touch '%s'; echo 'Verify hook marker before stopping' >&2; exit 2; fi; exit 0"
`, postMarker, postMarker, stopMarker, stopMarker)
	if err := os.WriteFile(filepath.Join(wsDir, ".gorness", "hooks.toml"), []byte(wsHooks), 0o600); err != nil {
		t.Fatalf("write ws hooks.toml: %v", err)
	}

	ws, err := eng.Workspaces().Register(wsDir, "Slice1CoreRepo")
	if err != nil {
		t.Fatalf("Register ws: %v", err)
	}

	// GrantStore CRUD check
	if err := eng.Grants().SaveGrant(ws.ID, "fp_1", "run_command", "go test"); err != nil {
		t.Fatalf("SaveGrant fp_1: %v", err)
	}
	if err := eng.Grants().SaveGrant(ws.ID, "fp_2", "write_file", "docs"); err != nil {
		t.Fatalf("SaveGrant fp_2: %v", err)
	}
	grantsList, err := eng.Grants().ListGrants(ws.ID)
	if err != nil || len(grantsList) != 2 {
		t.Fatalf("ListGrants expected 2, got %+v err=%v", grantsList, err)
	}
	if err := eng.Grants().RevokeGrant(ws.ID, "fp_1"); err != nil || eng.Grants().HasGrant(ws.ID, "fp_1") {
		t.Fatalf("RevokeGrant fp_1 failed: err=%v", err)
	}
	if err := eng.Grants().ClearGrants(ws.ID); err != nil || eng.Grants().HasGrant(ws.ID, "fp_2") {
		t.Fatalf("ClearGrants failed: err=%v", err)
	}

	// TrustWorkspace -> enables workspace hooks.toml!
	trustedHashes, err := eng.TrustWorkspace(ws.ID)
	if err != nil || len(trustedHashes) == 0 {
		t.Fatalf("TrustWorkspace failed: hashes=%v err=%v", trustedHashes, err)
	}

	// PreToolUse exit 2 blocks write_file("blocked.txt") with redacted stderr feedback!
	_, preEvs, err := eng.StartRun(ctx, RunRequest{
		WorkspaceID: ws.ID,
		Prompt:      "HOOK_PRE_BLOCK with secret sk-ant-test-key",
		PermMode:    PermModeAuto,
	})
	if err != nil {
		t.Fatalf("StartRun HOOK_PRE_BLOCK: %v", err)
	}
	var sawHookBlocked bool
	for _, ev := range drainRunEvents(preEvs) {
		if ev.Type == EventToolResult {
			if tr, ok := ev.Payload.(ToolResultPayload); ok && tr.IsError && strings.Contains(tr.Output, "blocked by policy stderr [REDACTED]") {
				sawHookBlocked = true
			}
		}
	}
	if !sawHookBlocked {
		t.Fatal("expected PreToolUse exit 2 to block write_file with redacted stderr feedback")
	}
	if _, err := os.Stat(filepath.Join(wsDir, "blocked.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("blocked.txt must NOT exist on disk when PreToolUse exited 2")
	}

	// PostToolUse + TurnStopping veto once then complete cleanly!
	_, postEvs, err := eng.StartRun(ctx, RunRequest{
		WorkspaceID: ws.ID,
		Prompt:      "HOOK_POST_AND_STOP",
		PermMode:    PermModeAuto,
	})
	if err != nil {
		t.Fatalf("StartRun HOOK_POST_AND_STOP: %v", err)
	}
	postDone := drainRunEvents(postEvs)
	if _, err := os.Stat(postMarker); err != nil {
		t.Fatalf("expected PostToolUse marker created, stat err=%v", err)
	}
	if _, err := os.Stat(stopMarker); err != nil {
		t.Fatalf("expected TurnStopping marker created, stat err=%v", err)
	}
	var postOutcome RunOutcome
	for _, ev := range postDone {
		if ev.Type == EventRunCompleted {
			if out, ok := ev.Payload.(RunOutcome); ok {
				postOutcome = out
			}
		}
	}
	if !strings.Contains(postOutcome.FinalText, "Verified after turn_stopping veto.") {
		t.Fatalf("expected TurnStopping veto follow-up loop, got outcome=%+v", postOutcome)
	}

	// 2. Cross-turn readHashes stale-modification protection & rewind invalidation
	rhSess, err := eng.Sessions().Create(ws.ID, "ReadHash Session", "coder")
	if err != nil {
		t.Fatalf("Create rhSess: %v", err)
	}
	_, rhEv1, err := eng.StartRun(ctx, RunRequest{
		WorkspaceID: ws.ID,
		SessionID:   rhSess.ID,
		Prompt:      "READHASH_TURN1_READ",
		PermMode:    PermModeAuto,
	})
	if err != nil {
		t.Fatalf("Turn 1 read: %v", err)
	}
	_ = drainRunEvents(rhEv1)

	// Half A: External edit on disk between Turn 1 and Turn 2 MUST trigger ErrStalePrecondition in Turn 2
	// because Engine persisted Turn 1's readHash across turns!
	if err := os.WriteFile(filepath.Join(wsDir, "target.txt"), []byte("alpha-v1\nexternal-human-edit\n"), 0o644); err != nil {
		t.Fatalf("external edit target.txt: %v", err)
	}
	_, rhEvStale, err := eng.StartRun(ctx, RunRequest{
		WorkspaceID: ws.ID,
		SessionID:   rhSess.ID,
		Prompt:      "READHASH_EDIT_ONLY",
		PermMode:    PermModeAuto,
	})
	if err != nil {
		t.Fatalf("Turn 2 stale edit check: %v", err)
	}
	var sawStalePrecondition bool
	for _, ev := range drainRunEvents(rhEvStale) {
		if ev.Type == EventToolResult {
			if tr, ok := ev.Payload.(ToolResultPayload); ok && tr.IsError && strings.Contains(strings.ToLower(tr.Output), "stale") {
				sawStalePrecondition = true
			}
		}
	}
	if !sawStalePrecondition {
		t.Fatal("expected cross-turn external file modification to be blocked by ErrStalePrecondition")
	}

	// Restore target.txt to alpha-v1 -> Turn 2b edit succeeds and updates readHashes to H(alpha-v2)!
	if err := os.WriteFile(filepath.Join(wsDir, "target.txt"), []byte("alpha-v1\n"), 0o644); err != nil {
		t.Fatalf("reset target.txt: %v", err)
	}
	_, rhEv2, err := eng.StartRun(ctx, RunRequest{
		WorkspaceID: ws.ID,
		SessionID:   rhSess.ID,
		Prompt:      "READHASH_EDIT_ONLY",
		PermMode:    PermModeAuto,
	})
	if err != nil {
		t.Fatalf("Turn 2b edit: %v", err)
	}
	_ = drainRunEvents(rhEv2)
	if raw, err := os.ReadFile(filepath.Join(wsDir, "target.txt")); err != nil || !strings.Contains(string(raw), "alpha-v2") {
		t.Fatalf("expected cross-turn edit_file to succeed and write alpha-v2, got %q err=%v", string(raw), err)
	}

	// Half B: Rewind rhSess back to seq 0 (restoring target.txt to alpha-v1) -> invalidates stale H(alpha-v2) in readHashes
	// so post-rewind Turn 3 edit against restored alpha-v1 succeeds instead of failing stale precondition!
	if _, _, err := eng.Sessions().Rewind(rhSess.ID, 0, true, eng.Snapshots(), wsDir); err != nil {
		t.Fatalf("Sessions().Rewind: %v", err)
	}
	_, rhEv3, err := eng.StartRun(ctx, RunRequest{
		WorkspaceID: ws.ID,
		SessionID:   rhSess.ID,
		Prompt:      "READHASH_EDIT_ONLY",
		PermMode:    PermModeAuto,
	})
	if err != nil {
		t.Fatalf("Turn 3 post-rewind edit: %v", err)
	}
	_ = drainRunEvents(rhEv3)
	if raw, err := os.ReadFile(filepath.Join(wsDir, "target.txt")); err != nil || !strings.Contains(string(raw), "alpha-v2") {
		t.Fatalf("expected post-rewind edit_file to succeed after readHashes invalidation, got %q err=%v", string(raw), err)
	}

	// 3. Warm MCPManager pool across runs + eviction & EventConfigReloaded on cfgMgr.Reload()
	var mcpInitHits atomic.Int32
	mcpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var rpc struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&rpc)
		if rpc.Method == "notifications/initialized" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch rpc.Method {
		case "initialize":
			mcpInitHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      rpc.ID,
				"result":  map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}},
			})
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      rpc.ID,
				"result":  map[string]any{"tools": []map[string]any{{"name": "warm_ping", "description": "Warm pooled ping"}}},
			})
		}
	}))
	defer mcpSrv.Close()

	globalSub, unsubGlobal := eng.SubscribeGlobal()
	defer unsubGlobal()

	mcpTOML := fmt.Sprintf("[servers.warm]\ntransport = \"http\"\nurl = %q\n", mcpSrv.URL)
	if err := os.WriteFile(cfgMgr.Paths().MCPFile(), []byte(mcpTOML), 0o600); err != nil {
		t.Fatalf("write mcp.toml: %v", err)
	}
	if _, err := cfgMgr.Reload(); err != nil {
		t.Fatalf("Reload mcp.toml: %v", err)
	}

	for i := 0; i < 2; i++ {
		_, sCh, err := eng.StartRun(ctx, RunRequest{WorkspaceID: ws.ID, Prompt: "warm mcp check"})
		if err != nil {
			t.Fatalf("warm mcp run %d: %v", i, err)
		}
		_ = drainRunEvents(sCh)
	}
	if got := mcpInitHits.Load(); got != 1 {
		t.Fatalf("expected warm MCPManager pool to handshake once across 2 runs, got %d handshakes", got)
	}

	// Reload evicts warm MCP pool -> next run reconnects (handshake #2)
	if _, err := cfgMgr.Reload(); err != nil {
		t.Fatalf("second Reload: %v", err)
	}
	_, sChAfterReload, err := eng.StartRun(ctx, RunRequest{WorkspaceID: ws.ID, Prompt: "warm mcp after reload"})
	if err != nil {
		t.Fatalf("post-reload mcp run: %v", err)
	}
	_ = drainRunEvents(sChAfterReload)
	if got := mcpInitHits.Load(); got != 2 {
		t.Fatalf("expected MCP handshake #2 after config reload eviction, got %d", got)
	}

	// 4. RegisterPluginTools, RunUtilityPrompt, Auto-Compaction (>=85% ContextWindow), and spawn_subagent
	eng.RegisterPluginTools(PluginTool{
		Name:            "dyn_probe",
		Description:     "Dynamic plugin tool for compaction probe",
		InputSchema:     json.RawMessage(`{"type":"object"}`),
		ReadOnly:        true,
		ConcurrencySafe: true,
		Handler: func(_ context.Context, _ tools.CallContext, _ json.RawMessage) (tools.ToolResult, error) {
			return tools.ToolResult{Output: "compaction-probe-ok"}, nil
		},
	})
	defer eng.UnregisterPluginTools("dyn_probe")

	utilReply, err := eng.RunUtilityPrompt(ctx, "utility", "", "Produce summary")
	if err != nil || !strings.Contains(utilReply, "Anchored auto-compaction summary") {
		t.Fatalf("RunUtilityPrompt failed: reply=%q err=%v", utilReply, err)
	}

	compSess, err := eng.Sessions().Create(ws.ID, "Auto Compaction Session", "coder")
	if err != nil {
		t.Fatalf("Create compSess: %v", err)
	}
	for i := 1; i <= 4; i++ {
		_, _ = eng.Sessions().Append(compSess.ID, store.SessionEntry{
			Role:   "user",
			Blocks: []store.ContentBlock{{Type: "text", Text: fmt.Sprintf("Historical turn %d", i)}},
		})
		_, _ = eng.Sessions().Append(compSess.ID, store.SessionEntry{
			Role:   "assistant",
			Blocks: []store.ContentBlock{{Type: "text", Text: fmt.Sprintf("Historical reply %d", i)}},
		})
	}
	_, acStream, err := eng.StartRun(ctx, RunRequest{
		WorkspaceID: ws.ID,
		SessionID:   compSess.ID,
		Prompt:      "AUTO_COMPACT_TRIGGER",
		PermMode:    PermModeAuto,
	})
	if err != nil {
		t.Fatalf("StartRun AUTO_COMPACT_TRIGGER: %v", err)
	}
	var sawAutoCompacted bool
	for _, ev := range drainRunEvents(acStream) {
		if ev.Type == EventCompacted {
			sawAutoCompacted = true
		}
	}
	if !sawAutoCompacted {
		t.Fatal("expected EventCompacted emitted automatically when InputTokens >= 85% of ContextWindow")
	}

	// spawn_subagent (explorer: read-only, non-recursive, max 8 steps, emits SubagentStarted & SubagentCompleted)
	_, subStream, err := eng.StartRun(ctx, RunRequest{
		WorkspaceID: ws.ID,
		Prompt:      "PARENT_DELEGATE_SUBAGENT",
		PermMode:    PermModeAuto,
	})
	if err != nil {
		t.Fatalf("StartRun PARENT_DELEGATE_SUBAGENT: %v", err)
	}
	var sawSubStart, sawSubDone bool
	for _, ev := range drainRunEvents(subStream) {
		switch ev.Type {
		case EventSubagentStarted:
			if p, ok := ev.Payload.(SubagentStartedPayload); ok && p.AgentRole == "explorer" && p.ReadOnly && p.MaxSteps == MaxSubagentSteps {
				sawSubStart = true
			}
		case EventSubagentCompleted:
			if p, ok := ev.Payload.(SubagentCompletedPayload); ok && p.Status == "completed" && strings.Contains(p.Summary, "Explorer distilled report") {
				sawSubDone = true
			}
		}
	}
	if !sawSubStart || !sawSubDone || subagentHit.Load() == 0 {
		t.Fatalf("expected spawn_subagent lifecycle events & execution: start=%v done=%v hits=%d", sawSubStart, sawSubDone, subagentHit.Load())
	}

	// 5. SubscribeRun (live attach + post-completion replay), PublishSystemEvent, and redacted runs.jsonl
	liveRunID, livePrimary, err := eng.StartRun(ctx, RunRequest{
		WorkspaceID: ws.ID,
		Prompt:      "SUBSCRIBE_LIVE_RUN",
	})
	if err != nil {
		t.Fatalf("StartRun SUBSCRIBE_LIVE_RUN: %v", err)
	}
	// Wait until chunk-before-attach has been emitted into active history before calling SubscribeRun
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		replay, _, unsub, sErr := eng.SubscribeRun(liveRunID)
		if sErr == nil {
			hasFirstChunk := false
			for _, ev := range replay {
				if ev.Type == EventTextDelta {
					if m, ok := ev.Payload.(map[string]any); ok && m["text"] == "chunk-before-attach " {
						hasFirstChunk = true
					}
				}
			}
			unsub()
			if hasFirstChunk {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
	}

	replayMid, liveSubCh, unsubLive, err := eng.SubscribeRun(liveRunID)
	if err != nil {
		t.Fatalf("SubscribeRun mid-run: %v", err)
	}
	defer unsubLive()
	close(liveStreamGate)

	liveTail := drainRunEvents(liveSubCh)
	_ = drainRunEvents(livePrimary)
	eng.WaitBackground()

	var combinedText strings.Builder
	for _, ev := range append(replayMid, liveTail...) {
		if ev.Type == EventTextDelta {
			if m, ok := ev.Payload.(map[string]any); ok {
				if s, ok := m["text"].(string); ok {
					combinedText.WriteString(s)
				}
			}
		}
	}
	if combinedText.String() != "chunk-before-attach chunk-after-attach" {
		t.Fatalf("SubscribeRun replay + live delta mismatch: got %q", combinedText.String())
	}

	// Post-completion SubscribeRun replays from bounded recentRuns cache and returns a closed channel
	postReplay, postCh, postUnsub, err := eng.SubscribeRun(liveRunID)
	if err != nil || len(postReplay) == 0 {
		t.Fatalf("SubscribeRun post-completion expected replay, got len=%d err=%v", len(postReplay), err)
	}
	postUnsub()
	if _, open := <-postCh; open {
		t.Fatal("expected post-completion SubscribeRun live channel to be closed")
	}

	eng.PublishSystemEvent(EventAlwaysOnUpdated, map[string]any{"inbox_pending": 0})
	var sawReloadEvent, sawAlwaysOnEvent bool
drainLoop:
	for {
		select {
		case ev := <-globalSub:
			if ev.Type == EventConfigReloaded {
				sawReloadEvent = true
			}
			if ev.Type == EventAlwaysOnUpdated {
				sawAlwaysOnEvent = true
			}
		default:
			break drainLoop
		}
	}
	if !sawReloadEvent || !sawAlwaysOnEvent {
		t.Fatalf("expected global EventConfigReloaded (%v) and EventAlwaysOnUpdated (%v)", sawReloadEvent, sawAlwaysOnEvent)
	}

	// Verify runtime/logs/runs.jsonl exists, contains redacted RunLogRecords, and ZERO .json files exist in vaultRoot!
	runsLogBytes, err := os.ReadFile(filepath.Join(cfgMgr.Paths().LogsDir(), "runs.jsonl"))
	if err != nil || len(runsLogBytes) == 0 {
		t.Fatalf("expected non-empty runtime/logs/runs.jsonl, err=%v", err)
	}
	if strings.Contains(string(runsLogBytes), "sk-ant-test-key") {
		t.Fatalf("runs.jsonl leaked raw API key:\n%s", string(runsLogBytes))
	}
	if !strings.Contains(string(runsLogBytes), "[REDACTED]") {
		t.Fatalf("expected [REDACTED] prompt preview in runs.jsonl, got:\n%s", string(runsLogBytes))
	}
	if err := filepath.WalkDir(vaultRoot, func(p string, d os.DirEntry, wErr error) error {
		if wErr != nil {
			return wErr
		}
		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".json") {
			return fmt.Errorf("forbidden .json file inside vault: %s", p)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestUnconsumedRunStreamDoesNotDeadlockAndReleasesSessionLock(t *testing.T) {
	ctx := context.Background()
	wsRoot := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(wsRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	streamer := llm.StreamerFunc(func(ctx context.Context, _ llm.Request) (<-chan llm.StreamEvent, error) {
		ch := make(chan llm.StreamEvent, 512)
		go func() {
			defer close(ch)
			// Emit 400 deltas (> 256 stream buffer capacity) to verify sendRunEvent never blocks when caller abandons stream.
			for i := 0; i < 400; i++ {
				ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "x"}
			}
			ch <- llm.StreamEvent{
				Type:         llm.StreamDone,
				StopReason:   "end_turn",
				InputTokens:  100,
				OutputTokens: 400,
			}
		}()
		return ch, nil
	})

	eng, cfgMgr, _, _ := setupTestEngine(t, nil, streamer)
	ws, err := eng.Workspaces().Register(wsRoot, "Deadlock Test WS")
	if err != nil {
		t.Fatal(err)
	}
	// Disable auto-generate title via config to also verify auto_generate_title=false is respected.
	rawCfg := `host = "127.0.0.1"
port = 3456
default_provider = "anthropic"
default_model = "claude-sonnet-5-5"
auto_generate_title = false
`
	if err := os.WriteFile(cfgMgr.Paths().ConfigFile(), []byte(rawCfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cfgMgr.Reload(); err != nil {
		t.Fatal(err)
	}

	sess, err := eng.Sessions().Create(ws.ID, "", "coder")
	if err != nil {
		t.Fatal(err)
	}

	// Start run and intentionally NEVER read from the returned stream channel.
	_, _, err = eng.StartRun(ctx, RunRequest{
		WorkspaceID: ws.ID,
		SessionID:   sess.ID,
		Prompt:      "Emit 400 chunks without consumer",
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	doneCh := make(chan struct{})
	go func() {
		eng.WaitBackground()
		close(doneCh)
	}()

	select {
	case <-doneCh:
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock detected: AgentLoop blocked on unconsumed run stream")
	}

	// Verify session flock was released and a second run on the same session succeeds immediately.
	_, stream2, err := eng.StartRun(ctx, RunRequest{
		WorkspaceID: ws.ID,
		SessionID:   sess.ID,
		Prompt:      "Second run after unconsumed stream",
	})
	if err != nil {
		t.Fatalf("expected session lock to be released for second run, got: %v", err)
	}
	_ = drainRunEvents(stream2)
	eng.WaitBackground()

	meta, err := eng.Sessions().GetMeta(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title != defaultSessionTitle {
		t.Fatalf("expected default session title %q when auto_generate_title=false, got %q", defaultSessionTitle, meta.Title)
	}
}
