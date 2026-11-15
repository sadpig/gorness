package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	"gorness.dev/gorness/internal/tools"
)

func initTestGitRepo(t *testing.T) string {
	t.Helper()
	repoDir := t.TempDir()
	canonDir, err := filepath.EvalSymlinks(repoDir)
	if err == nil {
		repoDir = canonDir
	}

	runGit(t, repoDir, "init")
	runGit(t, repoDir, "checkout", "-B", "main")

	files := map[string]string{
		"go.mod": "module example.com/fleetdemo\n\ngo 1.22\n",
		"base.go": `package fleetdemo

func BaseValue() int {
	return 10
}
`,
		"base_test.go": `package fleetdemo

import "testing"

func TestBaseValue(t *testing.T) {
	if BaseValue() != 10 {
		t.Fatalf("expected 10, got %d", BaseValue())
	}
}
`,
	}
	for name, body := range files {
		p := filepath.Join(repoDir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	runGit(t, repoDir, "add", "-A")
	runGit(t, repoDir, "-c", "user.name=Test Init", "-c", "user.email=test@gorness.local", "-c", "commit.gpgsign=false", "commit", "-m", "initial commit")
	return repoDir
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s failed: %v\noutput: %s", args, dir, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func setupTestFleet(t *testing.T, streamer llm.Streamer, verifyRunner VerifyRunner) (*Manager, *core.Engine, *config.Manager, *store.Workspace, string) {
	t.Helper()
	baseDir := t.TempDir()
	canonBase, err := filepath.EvalSymlinks(baseDir)
	if err == nil {
		baseDir = canonBase
	}
	vaultRoot := filepath.Join(baseDir, "vault")
	homeDir := filepath.Join(baseDir, "home")
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		t.Fatalf("mkdir homeDir: %v", err)
	}

	cfgMgr, err := config.NewManager(config.Options{
		VaultRoot:   vaultRoot,
		UserHomeDir: homeDir,
		EnvLookup: func(key string) (string, bool) {
			switch key {
			case "ANTHROPIC_API_KEY":
				return "sk-ant-fleet-test", true
			case "OPENAI_API_KEY":
				return "sk-openai-fleet-test", true
			case "GEMINI_API_KEY":
				return "gem-fleet-test", true
			case "MLX_API_KEY":
				return "mlx-fleet-test", true
			default:
				return "", false
			}
		},
	})
	if err != nil {
		t.Fatalf("config.NewManager: %v", err)
	}

	wsStore, err := store.NewWorkspaceStore(cfgMgr.Paths())
	if err != nil {
		t.Fatalf("NewWorkspaceStore: %v", err)
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
	sb := tools.NewSeatbeltRunner(tools.WithRunnerVaultRoot(cfgMgr.Paths().Root))

	eng, err := core.NewEngine(core.EngineOptions{
		Config:     cfgMgr,
		Workspaces: wsStore,
		Sessions:   sessStore,
		Snapshots:  snapStore,
		Router:     router,
		Seatbelt:   sb,
		UserHome:   homeDir,
	})
	if err != nil {
		t.Fatalf("core.NewEngine: %v", err)
	}

	repoDir := initTestGitRepo(t)
	ws, err := wsStore.Register(repoDir, "fleet-repo")
	if err != nil {
		t.Fatalf("Register workspace: %v", err)
	}

	mgr, err := NewManager(ManagerOptions{
		Config:       cfgMgr,
		Engine:       eng,
		Seatbelt:     sb,
		VerifyRunner: verifyRunner,
	})
	if err != nil {
		t.Fatalf("fleet.NewManager: %v", err)
	}

	t.Cleanup(func() {
		_ = mgr.Close()
		_ = eng.Close()
	})
	return mgr, eng, cfgMgr, ws, repoDir
}

func emitTextTurn(text string, inTok, outTok int) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, 4)
	ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: text}
	ch <- llm.StreamEvent{
		Type:         llm.StreamDone,
		StopReason:   "end_turn",
		InputTokens:  inTok,
		OutputTokens: outTok,
	}
	close(ch)
	return ch, nil
}

func emitWriteFileTurn(callID, relPath, content string) (<-chan llm.StreamEvent, error) {
	rawArgs, _ := json.Marshal(map[string]any{
		"path":    relPath,
		"content": content,
	})
	ch := make(chan llm.StreamEvent, 4)
	ch <- llm.StreamEvent{
		Type:       llm.StreamToolCall,
		ToolCallID: callID,
		ToolName:   "write_file",
		ToolInput:  rawArgs,
	}
	ch <- llm.StreamEvent{
		Type:         llm.StreamDone,
		StopReason:   "tool_use",
		InputTokens:  120,
		OutputTokens: 60,
	}
	close(ch)
	return ch, nil
}

func historyHasToolResult(history []store.SessionEntry) bool {
	for _, ent := range history {
		if ent.Role == "tool" {
			return true
		}
		for _, b := range ent.Blocks {
			if b.Type == "tool_result" {
				return true
			}
		}
	}
	return false
}

func lastEntryIsToolResult(history []store.SessionEntry) bool {
	if len(history) == 0 {
		return false
	}
	last := history[len(history)-1]
	if last.Role == "tool" {
		return true
	}
	for _, b := range last.Blocks {
		if b.Type == "tool_result" {
			return true
		}
	}
	return false
}

func flattenHistoryText(history []store.SessionEntry) string {
	var sb strings.Builder
	for _, ent := range history {
		for _, b := range ent.Blocks {
			if b.Text != "" {
				sb.WriteString(b.Text)
				sb.WriteByte('\n')
			}
		}
	}
	return sb.String()
}

func collectMissionEvents(ch <-chan MissionEvent) []MissionEvent {
	var out []MissionEvent
	for ev := range ch {
		out = append(out, ev)
	}
	return out
}

func TestWorktreePoolAndRefinerySerialMerge(t *testing.T) {
	// Barrier proves coder_1 and coder_2 run concurrently in isolated Git worktrees.
	var arrived atomic.Int32
	bothRunning := make(chan struct{})
	var closeOnce sync.Once

	var mu sync.Mutex
	verifierProviders := make([]string, 0, 2)
	coderModels := make([]string, 0, 2)
	var sawActiveWorktrees atomic.Bool

	var vaultWorktreesRoot atomic.Value // string

	streamer := llm.StreamerFunc(func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
		promptText := flattenHistoryText(req.History)

		// Verifier must be routed heterogeneously to OpenAI gpt-5.5 per agents/verifier.md!
		if strings.Contains(req.StaticSystem, "Adversarial Verifier") || req.Provider.ID == "openai" {
			mu.Lock()
			verifierProviders = append(verifierProviders, req.Provider.ID+"/"+req.Model)
			mu.Unlock()
			return emitTextTurn("VERDICT: ACCEPT\nDiff and go test ./... audit clean.", 200, 40)
		}

		// Coder 1 vs Coder 2 (identified via node assignment in prompt/context).
		isCoder1 := strings.Contains(promptText, "coder_1") || strings.Contains(promptText, "Teammate 1")
		if !historyHasToolResult(req.History) {
			mu.Lock()
			coderModels = append(coderModels, req.Provider.ID+"/"+req.Model)
			mu.Unlock()

			if arrived.Add(1) == 2 {
				closeOnce.Do(func() { close(bothRunning) })
			}
			select {
			case <-bothRunning:
			case <-time.After(5 * time.Second):
				return nil, errors.New("timed out waiting for concurrent worktree teammate")
			case <-ctx.Done():
				return nil, ctx.Err()
			}

			if wtRoot, ok := vaultWorktreesRoot.Load().(string); ok && wtRoot != "" {
				if entries, err := os.ReadDir(wtRoot); err == nil && len(entries) > 0 {
					sawActiveWorktrees.Store(true)
				}
			}

			if isCoder1 {
				return emitWriteFileTurn("call_pkg_a", "pkg_a.go", `package fleetdemo

func FeatureA() int {
	return BaseValue() + 1
}
`)
			}
			return emitWriteFileTurn("call_pkg_b", "pkg_b.go", `package fleetdemo

func FeatureB() int {
	return BaseValue() + 2
}
`)
		}

		// Second tool call adds unit tests in each worktree so `go test ./...` exercises both files!
		toolResultsCount := 0
		for _, ent := range req.History {
			if ent.Role == "tool" {
				toolResultsCount++
			}
		}
		if toolResultsCount == 1 {
			if isCoder1 {
				return emitWriteFileTurn("call_pkg_a_test", "pkg_a_test.go", `package fleetdemo

import "testing"

func TestFeatureA(t *testing.T) {
	if FeatureA() != 11 {
		t.Fatalf("expected 11, got %d", FeatureA())
	}
}
`)
			}
			return emitWriteFileTurn("call_pkg_b_test", "pkg_b_test.go", `package fleetdemo

import "testing"

func TestFeatureB(t *testing.T) {
	if FeatureB() != 12 {
		t.Fatalf("expected 12, got %d", FeatureB())
	}
}
`)
		}

		if isCoder1 {
			return emitTextTurn("Implemented pkg_a.go and pkg_a_test.go in isolated worktree.", 150, 30)
		}
		return emitTextTurn("Implemented pkg_b.go and pkg_b_test.go in isolated worktree.", 150, 30)
	})

	// Use real `go test ./...` execution via SeatbeltRunner (VerifyRunner == nil)!
	mgr, _, cfgMgr, ws, repoDir := setupTestFleet(t, streamer, nil)
	vaultWorktreesRoot.Store(cfgMgr.Paths().WorktreesDir())

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	initialSnap, stream, err := mgr.StartMission(ctx, MissionRequest{
		WorkspaceID:   ws.ID,
		WorkflowID:    "worktree-team",
		Topology:      TopoWorktreeTeam,
		Goal:          "Implement FeatureA in pkg_a.go and FeatureB in pkg_b.go concurrently and land onto main.",
		VerifyCommand: "go test ./...",
	})
	if err != nil {
		t.Fatalf("StartMission(worktree-team): %v", err)
	}
	if initialSnap.Topology != TopoWorktreeTeam {
		t.Fatalf("expected topology %q, got %q", TopoWorktreeTeam, initialSnap.Topology)
	}

	events := collectMissionEvents(stream)
	if len(events) == 0 {
		t.Fatal("expected live MissionEvents from worktree-team run")
	}

	finalSnap, err := mgr.GetMission(ws.ID, initialSnap.ID)
	if err != nil {
		t.Fatalf("GetMission: %v", err)
	}
	if finalSnap.Status != "completed" {
		t.Fatalf("expected mission status completed, got %q (error=%q)", finalSnap.Status, finalSnap.Error)
	}
	if !sawActiveWorktrees.Load() {
		t.Fatal("expected active isolated worktrees under runtime/worktrees/ while coders ran concurrently")
	}

	// Assert heterogeneous model routing: coders on anthropic/claude-sonnet-5-5, verifier on openai/gpt-5.5.
	mu.Lock()
	gotVerifiers := append([]string(nil), verifierProviders...)
	gotCoders := append([]string(nil), coderModels...)
	mu.Unlock()
	if len(gotVerifiers) < 2 {
		t.Fatalf("expected verifier blind review for each coder branch, got %v", gotVerifiers)
	}
	for _, vp := range gotVerifiers {
		if vp != "openai/gpt-5.5" {
			t.Fatalf("expected heterogeneous verifier openai/gpt-5.5, got %q", vp)
		}
	}
	for _, cm := range gotCoders {
		if cm != "anthropic/claude-sonnet-5-5" {
			t.Fatalf("expected coder anthropic/claude-sonnet-5-5, got %q", cm)
		}
	}

	// Assert both pkg_a.go and pkg_b.go landed cleanly onto `main` via serial rebase + fast-forward merge!
	for _, wantFile := range []string{"pkg_a.go", "pkg_a_test.go", "pkg_b.go", "pkg_b_test.go"} {
		raw, rErr := os.ReadFile(filepath.Join(repoDir, wantFile))
		if rErr != nil || len(raw) == 0 {
			t.Fatalf("expected %s landed on main in %s, err=%v", wantFile, repoDir, rErr)
		}
	}

	// Verify linear commit history on `main` (zero merge commits; initial + 2 rebased fast-forward commits = 3 commits).
	mergesOut := runGit(t, repoDir, "rev-list", "--merges", "main")
	if mergesOut != "" {
		t.Fatalf("expected pure fast-forward linear history with 0 merge commits, got merges: %s", mergesOut)
	}
	commitCountOut := runGit(t, repoDir, "rev-list", "--count", "main")
	if commitCountOut != "3" {
		logOut := runGit(t, repoDir, "log", "--oneline", "main")
		t.Fatalf("expected 3 linear commits on main, got %s:\n%s", commitCountOut, logOut)
	}

	// InspectNodeDiff must still report each coder's diff even after temporary worktrees cleaned up.
	diff1, err := mgr.InspectNodeDiff(ws.ID, initialSnap.ID, "coder_1")
	if err != nil || !strings.Contains(diff1, "pkg_a.go") {
		t.Fatalf("expected coder_1 diff to mention pkg_a.go, got %q (err=%v)", diff1, err)
	}
	diff2, err := mgr.InspectNodeDiff(ws.ID, initialSnap.ID, "coder_2")
	if err != nil || !strings.Contains(diff2, "pkg_b.go") {
		t.Fatalf("expected coder_2 diff to mention pkg_b.go, got %q (err=%v)", diff2, err)
	}

	// Temporary worktrees under runtime/worktrees/<missionID> and git worktree list must be cleaned up!
	missionWtDir := filepath.Join(cfgMgr.Paths().WorktreesDir(), initialSnap.ID)
	if entries, err := os.ReadDir(missionWtDir); err == nil && len(entries) > 0 {
		t.Fatalf("expected temporary worktrees cleaned up under %s, still found %d entries", missionWtDir, len(entries))
	}
	wtList := runGit(t, repoDir, "worktree", "list")
	if strings.Contains(wtList, initialSnap.ID) {
		t.Fatalf("expected git worktree list to have pruned mission worktrees, got:\n%s", wtList)
	}
}

func TestMarkdownBlackboardFlockClaimSplitAndConvergenceGuard(t *testing.T) {
	baseDir := t.TempDir()
	paths := config.VaultPaths{Root: filepath.Join(baseDir, "vault")}
	const (
		wsID      = "ws-board-test"
		missionID = "msn-board-flock"
	)

	// Path traversal IDs must be rejected immediately.
	if _, err := NewMarkdownBlackboard(paths, "../escape", missionID); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("expected ErrInvalidID for traversal wsID, got %v", err)
	}
	if _, err := NewMarkdownBlackboard(paths, wsID, "bad/mission"); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("expected ErrInvalidID for slash missionID, got %v", err)
	}

	bb, err := NewMarkdownBlackboard(paths, wsID, missionID)
	if err != nil {
		t.Fatalf("NewMarkdownBlackboard: %v", err)
	}
	if err := bb.InitMission(&MissionSnapshot{
		ID:          missionID,
		WorkspaceID: wsID,
		WorkflowID:  "shared-board",
		Goal:        "Decompose and execute tasks with zero duplicate claims",
		Topology:    TopoSharedBoard,
		Status:      "running",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatalf("InitMission: %v", err)
	}

	// 1. Seed 4 open tasks and contend from 8 concurrent peer workers via Flock.
	taskIDs := []string{"task-1", "task-2", "task-3", "task-4"}
	for _, tid := range taskIDs {
		if _, err := bb.CreateTask(tid, "", "Title for "+tid, "Detailed body for "+tid, 0); err != nil {
			t.Fatalf("CreateTask(%s): %v", tid, err)
		}
	}

	const numWorkers = 8
	startBarrier := make(chan struct{})
	var wg sync.WaitGroup
	var winnersMu sync.Mutex
	winnersByTask := make(map[string][]string)

	for w := 0; w < numWorkers; w++ {
		workerID := fmt.Sprintf("worker_%d", w+1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startBarrier
			// Contend directly on every task card AND via ClaimNextOpenTask, plus share a note.
			for {
				claimed, ok, cErr := bb.ClaimNextOpenTask(workerID, "gorness/fleet/"+missionID+"/"+workerID)
				if cErr != nil {
					t.Errorf("ClaimNextOpenTask(%s): %v", workerID, cErr)
					return
				}
				if !ok {
					break
				}
				winnersMu.Lock()
				winnersByTask[claimed.ID] = append(winnersByTask[claimed.ID], workerID)
				winnersMu.Unlock()

				if _, nErr := bb.AddNote(workerID, "Progress on "+claimed.ID, "Investigated and completed "+claimed.ID); nErr != nil {
					t.Errorf("AddNote(%s): %v", workerID, nErr)
				}
				if _, compErr := bb.CompleteTask(claimed.ID, workerID, "deadbeef"); compErr != nil {
					t.Errorf("CompleteTask(%s): %v", claimed.ID, compErr)
				}
			}
		}()
	}
	close(startBarrier)
	wg.Wait()

	// Assert exact 1 winner per task and zero duplicate claims across all 8 peer workers!
	for _, tid := range taskIDs {
		winners := winnersByTask[tid]
		if len(winners) != 1 {
			t.Fatalf("task %s expected exact 1 Flock claim winner, got %d (%v)", tid, len(winners), winners)
		}
	}

	// Also contend 8 workers simultaneously on a single task via ClaimTask(taskID, ...).
	if _, err := bb.CreateTask("task-contended", "", "Single Hotspot Task", "Hotspot body", 0); err != nil {
		t.Fatalf("CreateTask(task-contended): %v", err)
	}
	hotspotStart := make(chan struct{})
	var hotspotWins atomic.Int32
	for w := 0; w < numWorkers; w++ {
		workerID := fmt.Sprintf("peer_%d", w+1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-hotspotStart
			_, won, cErr := bb.ClaimTask("task-contended", workerID, "branch-"+workerID)
			if cErr != nil {
				t.Errorf("ClaimTask hotspot error: %v", cErr)
				return
			}
			if won {
				hotspotWins.Add(1)
			}
		}()
	}
	close(hotspotStart)
	wg.Wait()
	if got := hotspotWins.Load(); got != 1 {
		t.Fatalf("expected exact 1 winner on single hotspot task, got %d", got)
	}

	// 2. Dynamically split a complex task to depth 3 (0 -> 1 -> 2 -> 3), and reject depth 4!
	rootTask, err := bb.CreateTask("task-root", "", "Complex Subsystem Refactor", "Split into smaller steps", 0)
	if err != nil {
		t.Fatalf("CreateTask(task-root): %v", err)
	}
	if _, won, err := bb.ClaimTask(rootTask.ID, "worker_1", "br-1"); err != nil || !won {
		t.Fatalf("ClaimTask(task-root): won=%v err=%v", won, err)
	}

	d1Tasks, err := bb.SplitTask(rootTask.ID, "worker_1", []SubtaskSpec{
		{ID: "task-d1-a", Title: "Depth 1 Part A", Description: "Subtask A"},
		{ID: "task-d1-b", Title: "Depth 1 Part B", Description: "Subtask B"},
	})
	if err != nil || len(d1Tasks) != 2 || d1Tasks[0].Depth != 1 {
		t.Fatalf("SplitTask depth 1 failed: tasks=%+v err=%v", d1Tasks, err)
	}

	if _, won, err := bb.ClaimTask("task-d1-a", "worker_2", "br-2"); err != nil || !won {
		t.Fatalf("ClaimTask(task-d1-a): won=%v err=%v", won, err)
	}
	d2Tasks, err := bb.SplitTask("task-d1-a", "worker_2", []SubtaskSpec{
		{ID: "task-d2-a", Title: "Depth 2 Leaf Prep", Description: "Deeper split"},
	})
	if err != nil || len(d2Tasks) != 1 || d2Tasks[0].Depth != 2 {
		t.Fatalf("SplitTask depth 2 failed: tasks=%+v err=%v", d2Tasks, err)
	}

	if _, won, err := bb.ClaimTask("task-d2-a", "worker_3", "br-3"); err != nil || !won {
		t.Fatalf("ClaimTask(task-d2-a): won=%v err=%v", won, err)
	}
	d3Tasks, err := bb.SplitTask("task-d2-a", "worker_3", []SubtaskSpec{
		{ID: "task-d3-a", Title: "Depth 3 Atomic Unit", Description: "Max depth leaf"},
	})
	if err != nil || len(d3Tasks) != 1 || d3Tasks[0].Depth != 3 {
		t.Fatalf("SplitTask depth 3 failed: tasks=%+v err=%v", d3Tasks, err)
	}

	// Splitting depth 3 -> depth 4 must be rejected with ErrMaxDepthReached!
	if _, won, err := bb.ClaimTask("task-d3-a", "worker_4", "br-4"); err != nil || !won {
		t.Fatalf("ClaimTask(task-d3-a): won=%v err=%v", won, err)
	}
	if _, err := bb.SplitTask("task-d3-a", "worker_4", []SubtaskSpec{
		{ID: "task-d4-illegal", Title: "Too Deep", Description: "Must fail"},
	}); !errors.Is(err, ErrMaxDepthReached) {
		t.Fatalf("expected ErrMaxDepthReached when splitting depth 3 task, got %v", err)
	}

	// Verify MISSION.md, tasks/*.md, notes/*.md, and events.jsonl exist on disk and round-trip cleanly.
	allTasks, err := bb.ListTasks()
	if err != nil || len(allTasks) < 9 {
		t.Fatalf("ListTasks expected >= 9 tasks, got %d (err=%v)", len(allTasks), err)
	}
	notes, err := bb.ListNotes()
	if err != nil || len(notes) != 4 {
		t.Fatalf("ListNotes expected 4 notes, got %d (err=%v)", len(notes), err)
	}
	missionDir := filepath.Join(paths.WorkspacesDir(), wsID, "missions", missionID)
	for _, rel := range []string{"MISSION.md", "events.jsonl"} {
		if _, stErr := os.Stat(filepath.Join(missionDir, rel)); stErr != nil {
			t.Fatalf("expected %s on disk: %v", rel, stErr)
		}
	}

	// 3. Verify ConvergenceGuard halts cleanly when max_idle_rounds is reached (both unit & shared-board mission).
	guard := NewConvergenceGuard(3, 20)
	if halt, _ := guard.ObserveRound(true); halt {
		t.Fatal("should not halt on active progress round")
	}
	if halt, _ := guard.ObserveRound(false); halt || guard.IdleRounds() != 1 {
		t.Fatalf("idle round 1 unexpected state: halt=%v idle=%d", halt, guard.IdleRounds())
	}
	if halt, _ := guard.ObserveRound(true); halt || guard.IdleRounds() != 0 {
		t.Fatalf("progress round must reset idle counter to 0, got halt=%v idle=%d", halt, guard.IdleRounds())
	}
	if halt, _ := guard.ObserveRound(false); halt {
		t.Fatal("should not halt on idle round 1")
	}
	if halt, _ := guard.ObserveRound(false); halt {
		t.Fatal("should not halt on idle round 2")
	}
	halt, reason := guard.ObserveRound(false)
	if !halt || !strings.Contains(reason, "max_idle_rounds") {
		t.Fatalf("expected ConvergenceGuard to halt on idle round 3, got halt=%v reason=%q", halt, reason)
	}

	// Also exercise full TopoSharedBoard coordinator through Manager with MaxIdleRounds = 2!
	boardStreamer := llm.StreamerFunc(func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
		promptText := flattenHistoryText(req.History)
		if strings.Contains(req.StaticSystem, "Lead Architect") {
			return emitTextTurn("TASK: Audit module boundaries\nTASK: Document shared contracts", 100, 40)
		}
		if strings.Contains(req.StaticSystem, "Adversarial Verifier") || req.Provider.ID == "openai" {
			return emitTextTurn("VERDICT: ACCEPT\nBoard task verified.", 80, 20)
		}
		// Split first task once if depth 0 and not yet split, otherwise complete cleanly!
		if strings.Contains(promptText, "Audit module boundaries") && strings.Contains(promptText, "depth=0") {
			return emitTextTurn("SPLIT_TASK: Check config boundary\nSPLIT_TASK: Check store boundary", 90, 30)
		}
		return emitTextTurn("Completed assigned blackboard task and recorded findings.", 90, 30)
	})
	fastVerify := func(ctx context.Context, cmdStr, workDir string) (tools.CommandResult, error) {
		return tools.CommandResult{Stdout: "ok", Combined: "ok", ExitCode: 0}, nil
	}
	mgr, _, _, ws, _ := setupTestFleet(t, boardStreamer, fastVerify)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	sbSnap, sbStream, err := mgr.StartMission(ctx, MissionRequest{
		WorkspaceID:   ws.ID,
		WorkflowID:    "shared-board",
		Topology:      TopoSharedBoard,
		Goal:          "Audit and document module contracts on shared blackboard",
		MaxIdleRounds: 2,
	})
	if err != nil {
		t.Fatalf("StartMission(shared-board): %v", err)
	}
	_ = collectMissionEvents(sbStream)

	sbFinal, err := mgr.GetMission(ws.ID, sbSnap.ID)
	if err != nil {
		t.Fatalf("GetMission(shared-board): %v", err)
	}
	if sbFinal.Status != "completed" {
		t.Fatalf("expected shared-board mission completed cleanly via ConvergenceGuard, got %q (err=%q)", sbFinal.Status, sbFinal.Error)
	}
	if len(sbFinal.Tasks) < 4 {
		t.Fatalf("expected seeded + dynamically split tasks on shared-board snapshot, got %d", len(sbFinal.Tasks))
	}
	if len(sbFinal.Notes) == 0 {
		t.Fatal("expected shared-board workers to record notes on blackboard")
	}
}

func TestAllSixTopologiesAndLiveNodeSteer(t *testing.T) {
	var (
		reviewRound      atomic.Int32
		coderSteerReady  = make(chan struct{})
		steerSent        = make(chan struct{})
		readyOnce        sync.Once
		sawSteerInPrompt atomic.Bool
		pipelineReject   atomic.Bool
	)

	streamer := llm.StreamerFunc(func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
		promptText := flattenHistoryText(req.History)

		if strings.Contains(promptText, "SteerSentinel42") {
			sawSteerInPrompt.Store(true)
		}

		// 1. Verifier role (openai / gpt-5.5)
		if strings.Contains(req.StaticSystem, "Adversarial Verifier") || req.Provider.ID == "openai" {
			if pipelineReject.Load() && strings.Contains(promptText, "HOP_BREAKER_TEST") {
				return emitTextTurn("VERDICT: REJECT\nPersistent check_failed to trigger MaxHops loop breaker.", 80, 25)
			}
			if strings.Contains(promptText, "REVIEW_LOOP_GOAL") {
				r := reviewRound.Add(1)
				if r == 1 {
					return emitTextTurn("VERDICT: REJECT\nRound 1 missing edge-case guard; please fix.", 90, 30)
				}
				return emitTextTurn("VERDICT: ACCEPT\nRound 2 fix looks solid and steered sentinel is present.", 90, 30)
			}
			return emitTextTurn("VERDICT: ACCEPT\nVerified cleanly.", 80, 20)
		}

		// 2. Explorer role (anthropic / claude-haiku-5-5)
		if strings.Contains(req.StaticSystem, "Fast Explorer") {
			return emitTextTurn("Recon findings: base.go defines BaseValue() == 10.", 70, 25)
		}

		// 3. Lead Architect role (anthropic / claude-opus-5-5)
		if strings.Contains(req.StaticSystem, "Lead Architect") {
			return emitTextTurn("Synthesized plan & architecture summary across subsystems.", 110, 40)
		}

		// 4. Debugger role (anthropic / claude-opus-5-5)
		if strings.Contains(req.StaticSystem, "Root-Cause Debugger") {
			return emitTextTurn("Applied debugger root-cause investigation step.", 100, 30)
		}

		// 5. Coder role (anthropic / claude-sonnet-5-5)
		if strings.Contains(promptText, "REVIEW_LOOP_GOAL") {
			// On very first coder call, pause until test injects live SteerNode instruction!
			if reviewRound.Load() == 0 && !strings.Contains(promptText, "SteerSentinel42") && !lastEntryIsToolResult(req.History) {
				readyOnce.Do(func() { close(coderSteerReady) })
				select {
				case <-steerSent:
				case <-time.After(5 * time.Second):
					return nil, errors.New("timed out waiting for SteerNode injection")
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				return emitTextTurn("Initial draft step before processing steer.", 80, 20)
			}
			if !lastEntryIsToolResult(req.History) {
				fileBody := "package fleetdemo\n\n// SteerSentinel42\nfunc ReviewedFeature() int {\n\treturn BaseValue() + 42\n}\n"
				if reviewRound.Load() >= 1 {
					fileBody = "package fleetdemo\n\n// SteerSentinel42 + ReviewerGuard\nfunc ReviewedFeature() int {\n\tif BaseValue() < 0 {\n\t\treturn 0\n\t}\n\treturn BaseValue() + 42\n}\n"
				}
				return emitWriteFileTurn(fmt.Sprintf("call_rev_write_%d", reviewRound.Load()+1), "reviewed.go", fileBody)
			}
			return emitTextTurn("Updated reviewed.go.", 80, 20)
		}

		if strings.Contains(promptText, "HYBRID_FLEET_GOAL") && !lastEntryIsToolResult(req.History) {
			return emitWriteFileTurn("call_hybrid_write", "hybrid_feature.go", "package fleetdemo\n\nfunc HybridFeature() int { return BaseValue() + 7 }\n")
		}

		return emitTextTurn("Completed stage work.", 80, 20)
	})

	fastVerify := func(ctx context.Context, cmdStr, workDir string) (tools.CommandResult, error) {
		return tools.CommandResult{Stdout: "PASS", Combined: "PASS", ExitCode: 0}, nil
	}

	mgr, _, _, ws, repoDir := setupTestFleet(t, streamer, fastVerify)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// --- Case A: `review-loop` (reject -> fix -> accept) + live `SteerNode` + live `Subscribe` ---
	revSnap, revStream, err := mgr.StartMission(ctx, MissionRequest{
		WorkspaceID: ws.ID,
		WorkflowID:  "review-loop",
		Topology:    TopoReviewLoop,
		Goal:        "REVIEW_LOOP_GOAL: Implement ReviewedFeature in reviewed.go",
		MaxRounds:   3,
	})
	if err != nil {
		t.Fatalf("StartMission(review-loop): %v", err)
	}

	// Verify heterogeneous node metadata on initial MissionSnapshot!
	nodeRoles := make(map[string]FleetNode)
	for _, n := range revSnap.Nodes {
		nodeRoles[n.ID] = n
	}
	if nodeRoles["coder"].ProviderID != "anthropic" || nodeRoles["coder"].ModelID != "claude-sonnet-5-5" {
		t.Fatalf("unexpected coder node route metadata: %+v", nodeRoles["coder"])
	}
	if nodeRoles["verifier"].ProviderID != "openai" || nodeRoles["verifier"].ModelID != "gpt-5.5" {
		t.Fatalf("unexpected verifier node route metadata: %+v", nodeRoles["verifier"])
	}

	// Attach secondary live viewer via Subscribe while mission is running!
	subSnap, subCh, unsub, err := mgr.Subscribe(ws.ID, revSnap.ID)
	if err != nil || subSnap == nil {
		t.Fatalf("Subscribe(live): snap=%v err=%v", subSnap, err)
	}
	defer unsub()

	// Wait until coder is actively running, then inject mid-run SteerNode instruction!
	select {
	case <-coderSteerReady:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for coder node to start running")
	}
	if err := mgr.SteerNode(ws.ID, revSnap.ID, "coder", "Include SteerSentinel42 in comment"); err != nil {
		t.Fatalf("SteerNode: %v", err)
	}
	close(steerSent)

	revEvents := collectMissionEvents(revStream)
	subEvents := collectMissionEvents(subCh)
	if len(revEvents) == 0 || len(subEvents) == 0 {
		t.Fatalf("expected events on both primary stream (%d) and Subscribe viewer (%d)", len(revEvents), len(subEvents))
	}
	if !sawSteerInPrompt.Load() {
		t.Fatal("expected mid-run SteerNode instruction to be injected into coder session")
	}
	if gotRounds := reviewRound.Load(); gotRounds != 2 {
		t.Fatalf("expected review-loop reject->fix->accept to take 2 verifier rounds, got %d", gotRounds)
	}

	revFinal, err := mgr.GetMission(ws.ID, revSnap.ID)
	if err != nil || revFinal.Status != "completed" {
		t.Fatalf("expected review-loop completed, got snap=%+v err=%v", revFinal, err)
	}
	landedReviewed, err := os.ReadFile(filepath.Join(repoDir, "reviewed.go"))
	if err != nil || !strings.Contains(string(landedReviewed), "ReviewerGuard") {
		t.Fatalf("expected reviewed.go v2 landed on main, got %q (err=%v)", string(landedReviewed), err)
	}
	// Verify edge pulses occurred on both coder->verifier and verifier->coder!
	pulsedEdges := 0
	for _, edge := range revFinal.Edges {
		if edge.PulseSeq > 0 {
			pulsedEdges++
		}
	}
	if pulsedEdges < 2 {
		t.Fatalf("expected both submit and feedback edges pulsed in review-loop, got %+v", revFinal.Edges)
	}

	// --- Case B: `parallel-explore` (read-only concurrent explorers + lead synthesis) ---
	peSnap, peStream, err := mgr.StartMission(ctx, MissionRequest{
		WorkspaceID: ws.ID,
		WorkflowID:  "parallel-explore",
		Topology:    TopoParallelExplore,
		Goal:        "Explore subsystem boundaries in parallel and synthesize architecture",
	})
	if err != nil {
		t.Fatalf("StartMission(parallel-explore): %v", err)
	}
	_ = collectMissionEvents(peStream)
	peFinal, err := mgr.GetMission(ws.ID, peSnap.ID)
	if err != nil || peFinal.Status != "completed" {
		t.Fatalf("expected parallel-explore completed, got %+v (err=%v)", peFinal, err)
	}
	if !strings.Contains(peFinal.Summary, "Synthesized plan") {
		t.Fatalf("expected lead synthesis summary on parallel-explore snapshot, got %q", peFinal.Summary)
	}

	// --- Case C: `event-pipeline` with `MaxHops = 8` loop breaker ---
	pipelineReject.Store(true)
	epSnap, epStream, err := mgr.StartMission(ctx, MissionRequest{
		WorkspaceID: ws.ID,
		WorkflowID:  "event-pipeline",
		Topology:    TopoEventPipeline,
		Goal:        "HOP_BREAKER_TEST: Trigger deterministic MaxHops loop breaker",
		MaxHops:     8,
	})
	if err != nil {
		t.Fatalf("StartMission(event-pipeline): %v", err)
	}
	_ = collectMissionEvents(epStream)
	epFinal, err := mgr.GetMission(ws.ID, epSnap.ID)
	if err != nil {
		t.Fatalf("GetMission(event-pipeline): %v", err)
	}
	if epFinal.Status != "failed" || !strings.Contains(strings.ToLower(epFinal.Error), "hop") {
		t.Fatalf("expected event-pipeline to halt with failed status on MaxHops=8 loop breaker, got status=%q error=%q", epFinal.Status, epFinal.Error)
	}
	pipelineReject.Store(false)

	// --- Case D: `hybrid-fleet` (lead -> explorer -> worktree coder -> blind verifier -> main) ---
	hfSnap, hfStream, err := mgr.StartMission(ctx, MissionRequest{
		WorkspaceID: ws.ID,
		WorkflowID:  "hybrid-fleet",
		Topology:    TopoHybridFleet,
		Goal:        "HYBRID_FLEET_GOAL: Add HybridFeature in hybrid_feature.go",
	})
	if err != nil {
		t.Fatalf("StartMission(hybrid-fleet): %v", err)
	}
	// Verify all 4 heterogeneous roles carry distinct model metadata on hybrid-fleet!
	hfRoleModels := make(map[string]string)
	for _, n := range hfSnap.Nodes {
		hfRoleModels[n.RoleID] = n.ProviderID + "/" + n.ModelID
	}
	if hfRoleModels["lead"] != "anthropic/claude-opus-5-5" ||
		hfRoleModels["explorer"] != "anthropic/claude-haiku-5-5" ||
		hfRoleModels["coder"] != "anthropic/claude-sonnet-5-5" ||
		hfRoleModels["verifier"] != "openai/gpt-5.5" {
		t.Fatalf("unexpected hybrid-fleet heterogeneous role models: %+v", hfRoleModels)
	}

	_ = collectMissionEvents(hfStream)
	hfFinal, err := mgr.GetMission(ws.ID, hfSnap.ID)
	if err != nil || hfFinal.Status != "completed" {
		t.Fatalf("expected hybrid-fleet completed, got %+v (err=%v)", hfFinal, err)
	}
	rawHybrid, err := os.ReadFile(filepath.Join(repoDir, "hybrid_feature.go"))
	if err != nil || !strings.Contains(string(rawHybrid), "HybridFeature") {
		t.Fatalf("expected hybrid_feature.go landed on main, got %q (err=%v)", string(rawHybrid), err)
	}
	if hfFinal.TotalCostUSD <= 0 {
		t.Fatalf("expected positive accumulated TotalCostUSD across fleet nodes, got %f", hfFinal.TotalCostUSD)
	}

	// Verify ListMissions returns all 4 missions launched in this workspace, and post-completion Subscribe works cleanly!
	listed, err := mgr.ListMissions(ws.ID)
	if err != nil || len(listed) != 4 {
		t.Fatalf("expected 4 missions in ListMissions, got %d (err=%v)", len(listed), err)
	}
	doneSnap, doneCh, doneUnsub, err := mgr.Subscribe(ws.ID, hfSnap.ID)
	if err != nil || doneSnap.Status != "completed" {
		t.Fatalf("Subscribe(completed mission): snap=%+v err=%v", doneSnap, err)
	}
	doneUnsub()
	for range doneCh {
	}

	// Verify path traversal validation on Manager query/mutation endpoints.
	if _, err := mgr.GetMission("../bad", hfSnap.ID); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("expected ErrInvalidID on GetMission traversal, got %v", err)
	}
	if _, err := mgr.InspectNodeDiff(ws.ID, hfSnap.ID, "../secret"); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("expected ErrInvalidID on InspectNodeDiff traversal, got %v", err)
	}
}

func TestWitnessStallMonitorAndStopMission(t *testing.T) {
	startedCh := make(chan struct{})
	var startOnce sync.Once

	slowStreamer := llm.StreamerFunc(func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
		startOnce.Do(func() { close(startedCh) })
		<-ctx.Done()
		return nil, ctx.Err()
	})

	_, eng, cfgMgr, ws, _ := setupTestFleet(t, slowStreamer, nil)

	mgr, err := NewManager(ManagerOptions{
		Config:        cfgMgr,
		Engine:        eng,
		StallTimeout:  35 * time.Millisecond,
		CheckInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewManager with short stall timeout: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	snap, stream, err := mgr.StartMission(ctx, MissionRequest{
		WorkspaceID: ws.ID,
		WorkflowID:  "parallel-explore",
		Topology:    TopoParallelExplore,
		Goal:        "Verify Witness stall detection and StopMission cancellation",
	})
	if err != nil {
		t.Fatalf("StartMission: %v", err)
	}

	select {
	case <-startedCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for slow node to start")
	}

	var sawStalled bool
	deadline := time.After(3 * time.Second)
	for !sawStalled {
		select {
		case ev, ok := <-stream:
			if !ok {
				t.Fatal("stream closed before EventNodeStalled")
			}
			if ev.Type == EventNodeStalled {
				sawStalled = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for Witness EventNodeStalled")
		}
	}

	if err := mgr.StopMission(ws.ID, snap.ID); err != nil {
		t.Fatalf("StopMission: %v", err)
	}
	_ = collectMissionEvents(stream)

	finalSnap, err := mgr.GetMission(ws.ID, snap.ID)
	if err != nil {
		t.Fatalf("GetMission after StopMission: %v", err)
	}
	if finalSnap.Status != MissionCancelled {
		t.Fatalf("expected cancelled status after StopMission, got %q", finalSnap.Status)
	}
}

func TestRefineryVerifyGateRejectionAndBudgetGuard(t *testing.T) {
	streamer := llm.StreamerFunc(func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
		if !lastEntryIsToolResult(req.History) && !strings.Contains(req.StaticSystem, "Adversarial Verifier") && !strings.Contains(req.StaticSystem, "Lead Architect") && !strings.Contains(req.StaticSystem, "Fast Explorer") {
			return emitWriteFileTurn("call_broken", "broken.go", "package fleetdemo\n\nfunc Broken() int { return }\n")
		}
		return emitTextTurn("Done", 50000, 10000)
	})

	failingVerify := func(ctx context.Context, cmdStr, workDir string) (tools.CommandResult, error) {
		return tools.CommandResult{
			Stderr:   "broken.go:3:21: not enough return values",
			Combined: "broken.go:3:21: not enough return values",
			ExitCode: 2,
		}, nil
	}

	mgr, _, cfgMgr, ws, repoDir := setupTestFleet(t, streamer, failingVerify)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 1. Failing VerifyCommand must block broken.go from landing on `main` and still clean up worktrees!
	snap, stream, err := mgr.StartMission(ctx, MissionRequest{
		WorkspaceID:   ws.ID,
		WorkflowID:    "worktree-team",
		Topology:      TopoWorktreeTeam,
		Goal:          "Attempt broken patch that fails VerifyCommand",
		VerifyCommand: "go test ./...",
	})
	if err != nil {
		t.Fatalf("StartMission: %v", err)
	}
	_ = collectMissionEvents(stream)

	got, err := mgr.GetMission(ws.ID, snap.ID)
	if err != nil || got.Status != MissionFailed {
		t.Fatalf("expected failed mission on non-zero VerifyCommand, got %+v (err=%v)", got, err)
	}
	if _, stErr := os.Stat(filepath.Join(repoDir, "broken.go")); !errors.Is(stErr, os.ErrNotExist) {
		t.Fatal("broken.go must NEVER land on main when Refinery VerifyCommand fails")
	}
	missionWtDir := filepath.Join(cfgMgr.Paths().WorktreesDir(), snap.ID)
	if entries, rErr := os.ReadDir(missionWtDir); rErr == nil && len(entries) > 0 {
		t.Fatalf("expected failed mission worktrees cleaned up, found %d entries", len(entries))
	}

	// 2. BudgetUSD guard halts multi-step mission once spent USD exceeds BudgetUSD.
	bSnap, bStream, err := mgr.StartMission(ctx, MissionRequest{
		WorkspaceID: ws.ID,
		WorkflowID:  "parallel-explore",
		Topology:    TopoParallelExplore,
		Goal:        "Exceed tiny budget",
		BudgetUSD:   0.0001,
	})
	if err != nil {
		t.Fatalf("StartMission(budget): %v", err)
	}
	_ = collectMissionEvents(bStream)
	bFinal, err := mgr.GetMission(ws.ID, bSnap.ID)
	if err != nil || bFinal.Status != MissionFailed || !strings.Contains(bFinal.Error, "budget") {
		t.Fatalf("expected budget failure, got %+v (err=%v)", bFinal, err)
	}
}

func TestVaultFileFormatsAndLosslessMissionMDFallback(t *testing.T) {
	baseDir := t.TempDir()
	paths := config.VaultPaths{Root: filepath.Join(baseDir, "vault")}
	bb, err := NewMarkdownBlackboard(paths, "ws-fmt", "msn-fmt")
	if err != nil {
		t.Fatalf("NewMarkdownBlackboard: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	orig := &MissionSnapshot{
		ID:          "msn-fmt",
		WorkspaceID: "ws-fmt",
		WorkflowID:  "hybrid-fleet",
		Goal:        "Preserve exact multi-line goal\nwithout heading corruption",
		Topology:    TopoHybridFleet,
		Status:      MissionFailed,
		Nodes: []FleetNode{
			{
				ID:         "coder",
				Label:      "Worktree Coder",
				RoleID:     "coder",
				ProviderID: "anthropic",
				ModelID:    "claude-sonnet-5-5",
				Effort:     "high",
				Status:     NodeCompleted,
				Tokens:     320,
				CostUSD:    0.0125,
				DiffStat:   "1 file changed",
				UpdatedAt:  now,
			},
		},
		Edges: []FleetEdge{
			{From: "coder", To: "verifier", Label: "audit", Kind: "gate", PulseSeq: 2},
		},
		MergeQueue:   []string{"coder"},
		TotalCostUSD: 0.0125,
		BudgetUSD:    1.5,
		Summary:      "Partial summary text",
		Error:        "verifier rejected patch",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := bb.InitMission(orig); err != nil {
		t.Fatalf("InitMission: %v", err)
	}
	if _, err := bb.CreateTask("task-1", "", "Task One", "Body", 0); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if _, err := bb.AddNote("coder", "Note One", "Detailed note"); err != nil {
		t.Fatalf("AddNote: %v", err)
	}

	rawDiff := "diff --git a/pkg.go b/pkg.go\n--- a/pkg.go\n+++ b/pkg.go\n@@ -1,2 +1,2 @@\n context line with leading space\n-old\n+new\n"
	if err := bb.SaveNodeDiff("coder", rawDiff); err != nil {
		t.Fatalf("SaveNodeDiff: %v", err)
	}
	gotDiff, err := bb.ReadNodeDiff("coder")
	if err != nil || gotDiff != rawDiff {
		t.Fatalf("ReadNodeDiff mismatch:\ngot:  %q\nwant: %q\nerr:  %v", gotDiff, rawDiff, err)
	}

	// Walk entire vault and assert zero .json or .diff files exist!
	err = filepath.WalkDir(paths.Root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		switch ext {
		case ".toml", ".md", ".jsonl", ".lock":
			return nil
		default:
			t.Fatalf("disallowed file extension %q found under vault: %s", ext, path)
			return nil
		}
	})
	if err != nil {
		t.Fatalf("WalkDir vault: %v", err)
	}

	// Delete snapshot.toml to exercise MISSION.md fallback and verify 100% lossless round-trip!
	if err := os.Remove(bb.SnapshotFile()); err != nil {
		t.Fatalf("Remove snapshot.toml: %v", err)
	}
	fallbackSnap, err := bb.LoadSnapshot()
	if err != nil {
		t.Fatalf("LoadSnapshot via MISSION.md fallback: %v", err)
	}
	if fallbackSnap.Goal != orig.Goal {
		t.Fatalf("fallback Goal corrupted:\ngot:  %q\nwant: %q", fallbackSnap.Goal, orig.Goal)
	}
	if fallbackSnap.Summary != orig.Summary || fallbackSnap.Error != orig.Error {
		t.Fatalf("fallback Summary/Error mismatch: summary=%q error=%q", fallbackSnap.Summary, fallbackSnap.Error)
	}
	if len(fallbackSnap.Nodes) != 1 || fallbackSnap.Nodes[0].ID != "coder" || fallbackSnap.Nodes[0].ModelID != "claude-sonnet-5-5" {
		t.Fatalf("fallback Nodes lost: %+v", fallbackSnap.Nodes)
	}
	if len(fallbackSnap.Edges) != 1 || fallbackSnap.Edges[0].PulseSeq != 2 {
		t.Fatalf("fallback Edges lost: %+v", fallbackSnap.Edges)
	}
	if len(fallbackSnap.MergeQueue) != 1 || fallbackSnap.MergeQueue[0] != "coder" {
		t.Fatalf("fallback MergeQueue lost: %+v", fallbackSnap.MergeQueue)
	}
	if len(fallbackSnap.Tasks) != 1 || len(fallbackSnap.Notes) != 1 {
		t.Fatalf("fallback Tasks/Notes mismatch: tasks=%d notes=%d", len(fallbackSnap.Tasks), len(fallbackSnap.Notes))
	}
}

func TestConcurrentVerifierSerializedWithoutWorktrees(t *testing.T) {
	var arrived atomic.Int32
	bothDone := make(chan struct{})
	var once sync.Once
	var verifierCalls atomic.Int32

	streamer := llm.StreamerFunc(func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
		if strings.Contains(req.StaticSystem, "Adversarial Verifier") || req.Provider.ID == "openai" {
			verifierCalls.Add(1)
			time.Sleep(25 * time.Millisecond)
			return emitTextTurn("**VERDICT**: **ACCEPT**\nSerialized in-place audit passed.", 80, 20)
		}
		if arrived.Add(1) == 2 {
			once.Do(func() { close(bothDone) })
		}
		select {
		case <-bothDone:
		case <-time.After(5 * time.Second):
			return nil, errors.New("timed out waiting for sibling coder")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return emitTextTurn("Completed in-place specialist check.", 60, 20)
	})

	fastVerify := func(ctx context.Context, cmdStr, workDir string) (tools.CommandResult, error) {
		return tools.CommandResult{Stdout: "ok", Combined: "ok", ExitCode: 0}, nil
	}

	mgr, _, _, ws, _ := setupTestFleet(t, streamer, fastVerify)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	disableWT := false
	snap, stream, err := mgr.StartMission(ctx, MissionRequest{
		WorkspaceID:  ws.ID,
		WorkflowID:   "worktree-team",
		Topology:     TopoWorktreeTeam,
		Goal:         "Concurrent specialists with UseWorktrees=false must serialize verifier session",
		UseWorktrees: &disableWT,
	})
	if err != nil {
		t.Fatalf("StartMission: %v", err)
	}
	_ = collectMissionEvents(stream)

	finalSnap, err := mgr.GetMission(ws.ID, snap.ID)
	if err != nil {
		t.Fatalf("GetMission: %v", err)
	}
	if finalSnap.Status != MissionCompleted {
		t.Fatalf("expected MissionCompleted without ErrSessionBusy when UseWorktrees=false, got status=%q err=%q", finalSnap.Status, finalSnap.Error)
	}
	if got := verifierCalls.Load(); got != 2 {
		t.Fatalf("expected 2 serialized verifier calls, got %d", got)
	}
}

func TestSharedBoardMaxIdleRoundsDefaultAndIncompleteMaxRoundsFailure(t *testing.T) {
	// 1. Default shared-board (MaxRounds unset, max_idle_rounds = 3 from shared-board.toml)
	// runs 3+ productive rounds (via multi-level splits) and still halts via max_idle_rounds (3), NOT DefaultMaxRounds!
	streamer := llm.StreamerFunc(func(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
		promptText := flattenHistoryText(req.History)
		if strings.Contains(req.StaticSystem, "Lead Architect") {
			return emitTextTurn("TASK: Root task A\nTASK: Root task B\nTASK: Root task C\nTASK: Root task D", 80, 30)
		}
		if strings.Contains(req.StaticSystem, "Adversarial Verifier") || req.Provider.ID == "openai" {
			return emitTextTurn("VERDICT: ACCEPT\nVerified.", 60, 15)
		}
		if strings.Contains(promptText, "Root task A") && strings.Contains(promptText, "depth=0") {
			return emitTextTurn("SPLIT_TASK: Subtask A1\nSPLIT_TASK: Subtask A2", 70, 20)
		}
		if strings.Contains(promptText, "Subtask A1") && strings.Contains(promptText, "depth=1") {
			return emitTextTurn("SPLIT_TASK: Subtask A1a", 70, 20)
		}
		return emitTextTurn("Finished board leaf task.", 70, 20)
	})
	fastVerify := func(ctx context.Context, cmdStr, workDir string) (tools.CommandResult, error) {
		return tools.CommandResult{Stdout: "ok", Combined: "ok", ExitCode: 0}, nil
	}

	mgr, _, _, ws, _ := setupTestFleet(t, streamer, fastVerify)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	disableWT := false
	snap, stream, err := mgr.StartMission(ctx, MissionRequest{
		WorkspaceID:  ws.ID,
		WorkflowID:   "shared-board",
		Topology:     TopoSharedBoard,
		Goal:         "Exercise >3 productive rounds then converge on default max_idle_rounds=3",
		UseWorktrees: &disableWT,
	})
	if err != nil {
		t.Fatalf("StartMission(shared-board default): %v", err)
	}
	_ = collectMissionEvents(stream)

	finalSnap, err := mgr.GetMission(ws.ID, snap.ID)
	if err != nil {
		t.Fatalf("GetMission: %v", err)
	}
	if finalSnap.Status != MissionCompleted {
		t.Fatalf("expected shared-board completed after >3 productive rounds, got status=%q err=%q", finalSnap.Status, finalSnap.Error)
	}
	if !strings.Contains(finalSnap.Summary, "max_idle_rounds reached (3)") {
		t.Fatalf("expected convergence via max_idle_rounds reached (3), got summary=%q", finalSnap.Summary)
	}

	// 2. Explicit MaxRounds = 1 while 4 tasks exist for 2 workers leaves 2 open tasks -> must fail (incomplete convergence)!
	incSnap, incStream, err := mgr.StartMission(ctx, MissionRequest{
		WorkspaceID:  ws.ID,
		WorkflowID:   "shared-board",
		Topology:     TopoSharedBoard,
		Goal:         "Cap at MaxRounds=1 with remaining open tasks",
		MaxRounds:    1,
		UseWorktrees: &disableWT,
	})
	if err != nil {
		t.Fatalf("StartMission(shared-board MaxRounds=1): %v", err)
	}
	_ = collectMissionEvents(incStream)

	incFinal, err := mgr.GetMission(ws.ID, incSnap.ID)
	if err != nil {
		t.Fatalf("GetMission(incomplete): %v", err)
	}
	if incFinal.Status != MissionFailed || !strings.Contains(incFinal.Error, "unfinished") {
		t.Fatalf("expected MissionFailed for incomplete convergence at MaxRounds=1, got status=%q err=%q", incFinal.Status, incFinal.Error)
	}
}

func TestParseVerifierVerdictMarkdownEmphasisAndFailClosed(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{
			name: "markdown bold fail after prose mentioning pass",
			raw:  "Unit tests pass, but error handling is unsafe.\n**VERDICT**: **FAIL**",
			want: false,
		},
		{
			name: "markdown list colon inside bold reject",
			raw:  "- **VERDICT:** `REJECT` — race condition on map",
			want: false,
		},
		{
			name: "status italic failed",
			raw:  "### Status: _failed_",
			want: false,
		},
		{
			name: "ambiguous commentary without explicit verdict fails closed",
			raw:  "I reviewed the diff and left some observations on error wrapping.",
			want: false,
		},
		{
			name: "prose mentioning pass mid-sentence without verdict fails closed",
			raw:  "Unit tests pass, please double check edge cases in parser.",
			want: false,
		},
		{
			name: "subcase 1a: imperative bullet starting with Pass fails closed",
			raw:  "Please address before landing:\n- Pass ctx to exec.CommandContext instead of context.Background().",
			want: false,
		},
		{
			name: "subcase 1b: sentence starting with Passed without explicit verdict fails closed",
			raw:  "Passed compilation, however input validation is missing.",
			want: false,
		},
		{
			name: "subcase 1c: bullet item '- Pass' is not a standalone verdict",
			raw:  "Checklist:\n- Pass",
			want: false,
		},
		{
			name: "subcase 2a: LGTM with snake_case check_failed rejects",
			raw:  "LGTM on formatting, but check_failed on race test",
			want: false,
		},
		{
			name: "subcase 2b: trailing standalone LGTM overridden by earlier check_failed",
			raw:  "check_failed on unit suite\nLGTM",
			want: false,
		},
		{
			name: "subcase 2c: underscore-wrapped __CHECK_FAILED__ verdict rejects",
			raw:  "__VERDICT__: __CHECK_FAILED__",
			want: false,
		},
		{
			name: "subcase 3a: mid-sentence status: pass-through fails closed",
			raw:  "When checking status: pass-through headers are dropped",
			want: false,
		},
		{
			name: "subcase 3b: line-start Status: pass-through hyphenated word fails closed",
			raw:  "Status: pass-through headers are dropped",
			want: false,
		},
		{
			name: "markdown bold accept",
			raw:  "All invariants verified.\n**VERDICT**: **ACCEPT**",
			want: true,
		},
		{
			name: "heading backtick pass",
			raw:  "### Verdict: `PASS`",
			want: true,
		},
		{
			name: "clean standalone final LGTM line accepts when zero reject cues exist",
			raw:  "All invariants and race checks verified.\n**LGTM!**",
			want: true,
		},
	}

	for _, tc := range cases {
		got, _ := parseVerifierVerdict(tc.raw)
		if got != tc.want {
			t.Fatalf("%s: parseVerifierVerdict(%q) = %v, want %v", tc.name, tc.raw, got, tc.want)
		}
	}
}

func TestWorktreeDiffUntrackedSymlinkZeroByteAndBinarySafety(t *testing.T) {
	repoDir := initTestGitRepo(t)
	outsideDir := t.TempDir()
	secretFile := filepath.Join(outsideDir, "id_rsa")
	if err := os.WriteFile(secretFile, []byte("TOP_SECRET_PRIVATE_KEY_MATERIAL\n"), 0o600); err != nil {
		t.Fatalf("write secretFile: %v", err)
	}

	vaultDir := filepath.Join(t.TempDir(), "vault")
	pool := NewWorktreePool(config.VaultPaths{Root: vaultDir})
	ctx := context.Background()

	lease, err := pool.Acquire(ctx, repoDir, "msn-diff-sec", "coder_1", "main")
	if err != nil {
		t.Fatalf("Acquire worktree: %v", err)
	}
	defer func() { _ = pool.Release(ctx, lease, true) }()

	// 1. Untracked symlink pointing to outside secret file
	if err := os.Symlink(secretFile, filepath.Join(lease.Path, "stolen_key.pem")); err != nil {
		t.Fatalf("symlink file: %v", err)
	}
	// 2. Untracked symlinked directory pointing outside
	if err := os.Symlink(outsideDir, filepath.Join(lease.Path, "linked_outside")); err != nil {
		t.Fatalf("symlink dir: %v", err)
	}
	// 3. 0-byte empty untracked file
	if err := os.WriteFile(filepath.Join(lease.Path, "empty.txt"), nil, 0o644); err != nil {
		t.Fatalf("write empty.txt: %v", err)
	}
	// 4. Binary untracked file containing NUL bytes
	if err := os.WriteFile(filepath.Join(lease.Path, "icon.bin"), []byte{0x89, 0x50, 0x00, 0x47}, 0o644); err != nil {
		t.Fatalf("write icon.bin: %v", err)
	}

	diffOut, err := pool.Diff(ctx, lease.Path, "main")
	if err != nil {
		t.Fatalf("pool.Diff: %v", err)
	}

	if strings.Contains(diffOut, "TOP_SECRET_PRIVATE_KEY_MATERIAL") {
		t.Fatalf("SECURITY VIOLATION: untracked symlink leaked target file contents into diff:\n%s", diffOut)
	}
	if !strings.Contains(diffOut, "new file mode 120000") {
		t.Fatalf("expected symlink mode 120000 header without target file body, got:\n%s", diffOut)
	}
	// Empty file must NOT emit a spurious @@ -0,0 +1,1 @@ hunk!
	emptySection := "diff --git a/empty.txt b/empty.txt\nnew file mode 100644\n"
	if !strings.Contains(diffOut, emptySection) || strings.Contains(diffOut, "+++ b/empty.txt") {
		t.Fatalf("expected clean 0-byte header for empty.txt without spurious hunk, got:\n%s", diffOut)
	}
	if !strings.Contains(diffOut, "Binary files /dev/null and b/icon.bin differ") {
		t.Fatalf("expected binary notice for NUL-containing file icon.bin, got:\n%s", diffOut)
	}
}
