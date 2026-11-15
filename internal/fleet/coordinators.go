package fleet

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"gorness.dev/gorness/internal/config"
)

var (
	reVerdictReject    = regexp.MustCompile(`(?i)\b(?:verdict|status|decision|result)\s*:\s*(reject|rejected|fail|failed)\b`)
	reVerdictAccept    = regexp.MustCompile(`(?i)\b(?:verdict|status|decision|result)\s*:\s*(accept|accepted|pass|passed|approve|approved|lgtm)\b`)
	reWordReject       = regexp.MustCompile(`(?i)\b(reject|rejected|fail|failed|check_failed)\b`)
	reStandaloneAccept = regexp.MustCompile(`(?i)^(?:accept|accepted|pass|passed|approve|approved|lgtm)(?:\b|[.!:,;\s—-]|$)`)
	reMarkdownEmphasis = strings.NewReplacer("*", "", "_", "", "`", "", "~", "")
)

func stripMarkdownVerdictLine(line string) string {
	s := reMarkdownEmphasis.Replace(line)
	s = strings.TrimSpace(s)
	s = strings.TrimLeft(s, "#>-+[] ")
	s = strings.TrimRight(s, "[] ")
	return strings.TrimSpace(s)
}

func parseVerifierVerdict(raw string) (accepted bool, feedback string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false, "empty verifier output"
	}
	lines := strings.Split(trimmed, "\n")
	cleanedLines := make([]string, 0, len(lines))
	for i := len(lines) - 1; i >= 0; i-- {
		cleanLine := stripMarkdownVerdictLine(lines[i])
		if cleanLine == "" {
			continue
		}
		cleanedLines = append(cleanedLines, cleanLine)
		if reVerdictReject.MatchString(cleanLine) {
			return false, trimmed
		}
		if reVerdictAccept.MatchString(cleanLine) {
			return true, trimmed
		}
	}

	cleanedAll := reMarkdownEmphasis.Replace(trimmed)
	if reWordReject.MatchString(cleanedAll) {
		return false, trimmed
	}
	for _, cleanLine := range cleanedLines {
		if reStandaloneAccept.MatchString(cleanLine) {
			return true, trimmed
		}
	}
	// 对抗验证官默认拒绝（fail-closed）：未给出明确通过裁决一律视为未通过
	return false, trimmed
}

func parseStructuredDirectiveLines(text, prefix string) []string {
	upperPrefix := strings.ToUpper(strings.TrimSpace(prefix))
	var items []string
	for _, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(rawLine), "-*0123456789. "))
		if len(line) > len(upperPrefix) && strings.EqualFold(line[:len(upperPrefix)], upperPrefix) {
			rest := strings.TrimSpace(line[len(upperPrefix):])
			if rest != "" {
				items = append(items, rest)
			}
		}
	}
	return items
}

func defaultWorkflowNodesAndEdges(topo TopologyKind) ([]config.WorkflowNodeSpec, []config.WorkflowEdgeSpec) {
	switch topo {
	case TopoReviewLoop:
		return []config.WorkflowNodeSpec{
			{ID: "coder", Label: "Generator Coder", Role: "coder", Worktree: true},
			{ID: "verifier", Label: "Adversarial Verifier", Role: "verifier"},
		}, []config.WorkflowEdgeSpec{
			{From: "coder", To: "verifier", Label: "submit diff", Kind: "review"},
			{From: "verifier", To: "coder", Label: "feedback", Kind: "feedback"},
		}

	case TopoParallelExplore:
		return []config.WorkflowNodeSpec{
			{ID: "lead", Label: "Synthesis Lead", Role: "lead"},
			{ID: "explorer_1", Label: "Explorer A", Role: "explorer"},
			{ID: "explorer_2", Label: "Explorer B", Role: "explorer"},
		}, []config.WorkflowEdgeSpec{
			{From: "lead", To: "explorer_1", Label: "partition A", Kind: "fanout"},
			{From: "lead", To: "explorer_2", Label: "partition B", Kind: "fanout"},
		}

	case TopoWorktreeTeam:
		return []config.WorkflowNodeSpec{
			{ID: "coder_1", Label: "Teammate 1", Role: "coder", Worktree: true},
			{ID: "coder_2", Label: "Teammate 2", Role: "coder", Worktree: true},
			{ID: "verifier", Label: "Refinery Verifier", Role: "verifier"},
		}, []config.WorkflowEdgeSpec{
			{From: "coder_1", To: "verifier", Label: "rebase & test", Kind: "merge_queue"},
			{From: "coder_2", To: "verifier", Label: "rebase & test", Kind: "merge_queue"},
		}

	case TopoEventPipeline:
		return []config.WorkflowNodeSpec{
			{ID: "explorer", Label: "Recon Stage", Role: "explorer"},
			{ID: "coder", Label: "Build Stage", Role: "coder", Worktree: true},
			{ID: "debugger", Label: "Debug Stage", Role: "debugger", Worktree: true},
			{ID: "verifier", Label: "Gate Stage", Role: "verifier"},
		}, []config.WorkflowEdgeSpec{
			{From: "explorer", To: "coder", Label: "context_ready", Kind: "event"},
			{From: "coder", To: "verifier", Label: "patch_ready", Kind: "event"},
			{From: "verifier", To: "debugger", Label: "check_failed", Kind: "event"},
		}

	case TopoSharedBoard:
		return []config.WorkflowNodeSpec{
			{ID: "lead", Label: "Board Seed", Role: "lead"},
			{ID: "worker_1", Label: "Board Worker 1", Role: "coder", Worktree: true},
			{ID: "worker_2", Label: "Board Worker 2", Role: "coder", Worktree: true},
			{ID: "verifier", Label: "Board Verifier", Role: "verifier"},
		}, []config.WorkflowEdgeSpec{
			{From: "lead", To: "worker_1", Label: "tasks/*.md", Kind: "blackboard"},
			{From: "lead", To: "worker_2", Label: "tasks/*.md", Kind: "blackboard"},
			{From: "worker_1", To: "verifier", Label: "land", Kind: "merge_queue"},
			{From: "worker_2", To: "verifier", Label: "land", Kind: "merge_queue"},
		}

	default: // TopoHybridFleet
		return []config.WorkflowNodeSpec{
			{ID: "lead", Label: "Lead Architect", Role: "lead"},
			{ID: "explorer", Label: "Code Explorer", Role: "explorer"},
			{ID: "coder", Label: "Worktree Coder", Role: "coder", Worktree: true},
			{ID: "verifier", Label: "Blind Verifier", Role: "verifier"},
		}, []config.WorkflowEdgeSpec{
			{From: "lead", To: "explorer", Label: "recon", Kind: "fanout"},
			{From: "lead", To: "coder", Label: "implement", Kind: "dispatch"},
			{From: "coder", To: "verifier", Label: "audit", Kind: "gate"},
		}
	}
}

func (rt *missionRuntime) executeTopology(ctx context.Context) error {
	switch rt.snap.Topology {
	case TopoParallelExplore:
		return rt.runParallelExplore(ctx)
	case TopoReviewLoop:
		return rt.runReviewLoop(ctx)
	case TopoWorktreeTeam:
		return rt.runWorktreeTeam(ctx)
	case TopoEventPipeline:
		return rt.runEventPipeline(ctx)
	case TopoSharedBoard:
		return rt.runSharedBoard(ctx)
	case TopoHybridFleet:
		return rt.runHybridFleet(ctx)
	default:
		return fmt.Errorf("unsupported topology %q", rt.snap.Topology)
	}
}

// 1. TopoParallelExplore: 主从静态前缀缓存共享并发只读探索 + Lead 汇总合成
func (rt *missionRuntime) runParallelExplore(ctx context.Context) error {
	lead := rt.firstNodeByRole("lead")
	explorers := rt.nodesByRole("explorer")
	if len(explorers) == 0 {
		for _, n := range rt.nodesSnapshot() {
			if lead == nil || n.ID != lead.ID {
				explorers = append(explorers, n)
			}
		}
	}

	findings := make([]string, len(explorers))
	sem := make(chan struct{}, rt.maxConcurrency)
	errCh := make(chan error, len(explorers))
	var wg sync.WaitGroup

	for i, exp := range explorers {
		i, exp := i, exp
		if lead != nil {
			rt.pulseEdge(lead.ID, exp.ID, "fanout partition")
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
			defer func() { <-sem }()

			prompt := fmt.Sprintf(
				"Parallel Exploration Mission (%s — partition %d/%d, node %s [%s]):\nGoal: %s\nInspect relevant files and report concise findings.",
				rt.snap.ID, i+1, len(explorers), exp.ID, exp.Label, rt.snap.Goal,
			)
			out, err := rt.runNode(ctx, exp.ID, prompt, "", nil)
			if err != nil {
				errCh <- err
				return
			}
			findings[i] = fmt.Sprintf("### %s (%s)\n%s", exp.Label, exp.ID, strings.TrimSpace(out.FinalText))
			_, _ = rt.recordNote(exp.ID, exp.Label+" Findings", out.FinalText)
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}

	combined := strings.Join(findings, "\n\n")
	if lead != nil {
		synthPrompt := fmt.Sprintf(
			"Synthesize the parallel explorer findings for mission goal:\n%s\n\nExplorer Reports:\n%s",
			rt.snap.Goal, combined,
		)
		leadOut, err := rt.runNode(ctx, lead.ID, synthPrompt, "", nil)
		if err != nil {
			return err
		}
		rt.setSummary(leadOut.FinalText)
		_, _ = rt.recordNote(lead.ID, "Lead Synthesis", leadOut.FinalText)
	} else {
		rt.setSummary(combined)
	}
	return nil
}

// 2. TopoReviewLoop: Generator-Verifier 异构模型对抗闭环（支持多轮驳回返工与实时 Steer）
func (rt *missionRuntime) runReviewLoop(ctx context.Context) error {
	coder := rt.firstNodeByRole("coder")
	if coder == nil {
		return fmt.Errorf("review-loop requires a coder node")
	}
	verifier := rt.firstNodeByRole("verifier")

	task, err := rt.bb.CreateTask("task-review-1", "", rt.snap.Goal, rt.snap.Goal, 0)
	if err == nil {
		_, _, _ = rt.bb.ClaimTask(task.ID, coder.ID, "")
		rt.syncBlackboardState(EventTaskUpdated, coder.ID, task.ID, "claimed review-loop task")
	}

	var lease *WorktreeLease
	if rt.shouldUseWorktree(coder.ID) {
		lease, err = rt.acquireNodeWorktree(ctx, coder.ID, coder.ID)
		if err != nil {
			return err
		}
		defer rt.releaseWorktree(context.Background(), coder.ID, lease, true)
	}

	wtPath := ""
	if lease != nil {
		wtPath = lease.Path
	}

	var lastCritique string
	maxRounds := rt.maxRounds
	if maxRounds <= 0 {
		maxRounds = DefaultMaxRounds
	}

	for round := 1; round <= maxRounds; round++ {
		var prompt string
		if round == 1 {
			prompt = fmt.Sprintf(
				"Implement the following mission goal (node=%s, round=%d/%d):\n%s",
				coder.ID, round, maxRounds, rt.snap.Goal,
			)
		} else {
			prompt = fmt.Sprintf(
				"Mission goal (node=%s, round=%d/%d):\n%s\n\nAddress the previous verification / blind review critique:\n%s",
				coder.ID, round, maxRounds, rt.snap.Goal, lastCritique,
			)
		}

		if _, err := rt.runNode(ctx, coder.ID, prompt, wtPath, nil); err != nil {
			return err
		}

		if verifier != nil {
			rt.pulseEdge(coder.ID, verifier.ID, fmt.Sprintf("submit diff (round %d)", round))
		}

		landRes, err := rt.verifyAndMaybeLand(ctx, coder.ID, verifier, lease, fmt.Sprintf("feat(fleet): %s round %d", coder.ID, round))
		if err != nil {
			return err
		}

		if landRes.Accepted {
			if task.ID != "" {
				_, _ = rt.bb.CompleteTask(task.ID, coder.ID, landRes.LandedCommit)
				rt.syncBlackboardState(EventTaskUpdated, coder.ID, task.ID, "review-loop task merged")
			}
			rt.setSummary(landRes.ReviewNotes)
			return nil
		}

		lastCritique = landRes.Reason
		verID := coder.ID
		if verifier != nil {
			verID = verifier.ID
		}
		_, _ = rt.recordNote(verID, fmt.Sprintf("Round %d Rejection", round), lastCritique)

		if round < maxRounds && verifier != nil {
			rt.pulseEdge(verifier.ID, coder.ID, fmt.Sprintf("feedback (round %d)", round))
		}
	}

	return fmt.Errorf("review-loop exhausted %d rounds without passing verification: %s", maxRounds, lastCritique)
}

// 3. TopoWorktreeTeam: 独立 Git Worktree 并发专家战队 + Refinery 串行变基测试盲审快进合入
func (rt *missionRuntime) runWorktreeTeam(ctx context.Context) error {
	coders := rt.nodesByRole("coder")
	if len(coders) == 0 {
		return fmt.Errorf("worktree-team requires at least one coder node")
	}
	verifier := rt.firstNodeByRole("verifier")

	leases := make(map[string]*WorktreeLease, len(coders))
	tasks := make(map[string]BlackboardTask, len(coders))
	for i, c := range coders {
		tid := fmt.Sprintf("task-%d", i+1)
		t, err := rt.bb.CreateTask(tid, "", fmt.Sprintf("%s: %s", c.Label, rt.snap.Goal), rt.snap.Goal, 0)
		if err == nil {
			tasks[c.ID] = t
		}
		if rt.shouldUseWorktree(c.ID) {
			lease, err := rt.acquireNodeWorktree(ctx, c.ID, c.ID)
			if err != nil {
				for nid, l := range leases {
					rt.releaseWorktree(context.Background(), nid, l, true)
				}
				return err
			}
			leases[c.ID] = lease
		}
	}
	rt.syncBlackboardState(EventTaskUpdated, "", "", "seeded worktree-team tasks")

	defer func() {
		for nid, l := range leases {
			rt.releaseWorktree(context.Background(), nid, l, true)
		}
	}()

	runCtx, cancelTeam := context.WithCancel(ctx)
	defer cancelTeam()

	sem := make(chan struct{}, rt.maxConcurrency)
	errCh := make(chan error, len(coders))
	var wg sync.WaitGroup
	var summariesMu sync.Mutex
	var summaries []string

	for i, coder := range coders {
		i, coder := i, coder
		lease := leases[coder.ID]
		task := tasks[coder.ID]

		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-runCtx.Done():
				errCh <- runCtx.Err()
				return
			}
			releasedSem := false
			releaseSem := func() {
				if !releasedSem {
					<-sem
					releasedSem = true
				}
			}
			defer releaseSem()

			branchName := ""
			wtPath := ""
			if lease != nil {
				branchName = lease.Branch
				wtPath = lease.Path
			}
			if task.ID != "" {
				_, _, _ = rt.bb.ClaimTask(task.ID, coder.ID, branchName)
				rt.syncBlackboardState(EventTaskUpdated, coder.ID, task.ID, "claimed by "+coder.ID)
			}

			prompt := fmt.Sprintf(
				"Worktree Specialist Assignment (node=%s, label=%s, specialist=%d/%d):\nMission Goal: %s\nWork strictly within your isolated Git worktree.",
				coder.ID, coder.Label, i+1, len(coders), rt.snap.Goal,
			)
			coderOut, err := rt.runNode(runCtx, coder.ID, prompt, wtPath, nil)
			if err != nil {
				cancelTeam()
				errCh <- err
				return
			}
			// 编码运行已完成，立即释放并发槽位供其他队友或 Refinery Verifier 使用
			releaseSem()

			_, _ = rt.recordNote(coder.ID, coder.Label+" Implementation", coderOut.FinalText)

			if verifier != nil {
				rt.pulseEdge(coder.ID, verifier.ID, "enqueue rebase & test")
			}

			landRes, lErr := rt.verifyAndMaybeLand(
				runCtx,
				coder.ID,
				verifier,
				lease,
				fmt.Sprintf("feat(fleet): land %s (%s)", coder.ID, coder.Label),
			)
			if lErr != nil {
				cancelTeam()
				errCh <- lErr
				return
			}
			if !landRes.Accepted {
				cancelTeam()
				errCh <- fmt.Errorf("refinery rejected %s at stage %s: %s", coder.ID, landRes.Stage, landRes.Reason)
				return
			}

			if task.ID != "" {
				_, _ = rt.bb.CompleteTask(task.ID, coder.ID, landRes.LandedCommit)
				rt.syncBlackboardState(EventTaskUpdated, coder.ID, task.ID, "landed "+landRes.LandedCommit)
			}

			// 合入成功后立即清理该队友的临时工作树
			if lease != nil {
				rt.releaseWorktree(runCtx, coder.ID, lease, true)
			}

			summariesMu.Lock()
			summaries = append(summaries, fmt.Sprintf("- %s (%s): landed %s", coder.Label, coder.ID, shortSHA(landRes.LandedCommit)))
			summariesMu.Unlock()
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}

	summariesMu.Lock()
	rt.setSummary(strings.Join(summaries, "\n"))
	summariesMu.Unlock()
	return nil
}

// 4. TopoEventPipeline: 强类型事件驱动流水线（Explorer -> Coder -> Verifier <-> Debugger + MaxHops 熔断）
func (rt *missionRuntime) runEventPipeline(ctx context.Context) error {
	explorer := rt.firstNodeByRole("explorer")
	coder := rt.firstNodeByRole("coder")
	debugger := rt.firstNodeByRole("debugger")
	verifier := rt.firstNodeByRole("verifier")

	if coder == nil {
		return fmt.Errorf("event-pipeline requires a coder stage")
	}

	maxHops := rt.maxHops
	if maxHops <= 0 {
		maxHops = DefaultMaxHops
	}
	hops := 0
	advanceHop := func(fromID, toID, eventKind string) error {
		hops++
		if hops > maxHops {
			return fmt.Errorf("%w: pipeline reached hop %d > max_hops %d (%s: %s -> %s)", ErrMaxHopsExceeded, hops, maxHops, eventKind, fromID, toID)
		}
		if fromID != "" && toID != "" {
			rt.pulseEdge(fromID, toID, eventKind)
		}
		return nil
	}

	var lease *WorktreeLease
	if rt.shouldUseWorktree(coder.ID) {
		var err error
		lease, err = rt.acquireNodeWorktree(ctx, coder.ID, coder.ID)
		if err != nil {
			return err
		}
		defer rt.releaseWorktree(context.Background(), coder.ID, lease, true)
	}
	wtPath := ""
	if lease != nil {
		wtPath = lease.Path
	}

	var reconText string
	prevStage := ""
	if explorer != nil {
		if err := advanceHop("", explorer.ID, "pipeline_start"); err != nil {
			return err
		}
		expOut, err := rt.runNode(ctx, explorer.ID, fmt.Sprintf("Recon Stage for pipeline goal:\n%s", rt.snap.Goal), "", nil)
		if err != nil {
			return err
		}
		reconText = expOut.FinalText
		_, _ = rt.recordNote(explorer.ID, "Pipeline Recon", reconText)
		prevStage = explorer.ID
	}

	if err := advanceHop(prevStage, coder.ID, "context_ready"); err != nil {
		return err
	}
	coderPrompt := fmt.Sprintf("Build Stage for pipeline goal:\n%s\n\nRecon Context:\n%s", rt.snap.Goal, reconText)
	if _, err := rt.runNode(ctx, coder.ID, coderPrompt, wtPath, nil); err != nil {
		return err
	}

	lastAuthor := coder.ID
	for {
		gateTarget := coder.ID
		if verifier != nil {
			gateTarget = verifier.ID
		}
		if err := advanceHop(lastAuthor, gateTarget, "patch_ready"); err != nil {
			return err
		}

		landRes, err := rt.verifyAndMaybeLand(ctx, lastAuthor, verifier, lease, fmt.Sprintf("feat(pipeline): %s stage patch", lastAuthor))
		if err != nil {
			return err
		}
		if landRes.Accepted {
			rt.setSummary(landRes.ReviewNotes)
			return nil
		}

		if debugger == nil {
			return fmt.Errorf("event-pipeline gate rejected patch: %s", landRes.Reason)
		}
		if err := advanceHop(gateTarget, debugger.ID, "check_failed"); err != nil {
			return err
		}

		dbgPrompt := fmt.Sprintf(
			"Debug Stage for pipeline goal:\n%s\n\nGate Failure (%s):\n%s",
			rt.snap.Goal, landRes.Stage, landRes.Reason,
		)
		dbgOut, err := rt.runNode(ctx, debugger.ID, dbgPrompt, wtPath, nil)
		if err != nil {
			return err
		}
		_, _ = rt.recordNote(debugger.ID, "Debugger Root-Cause Fix", dbgOut.FinalText)
		lastAuthor = debugger.ID
	}
}

// 5. TopoSharedBoard: Markdown 共享黑板 Flock 抢单 + 动态拆树（深度 <= 3） + ConvergenceGuard 收敛守卫
func (rt *missionRuntime) runSharedBoard(ctx context.Context) error {
	lead := rt.firstNodeByRole("lead")
	verifier := rt.firstNodeByRole("verifier")
	var workers []FleetNode
	for _, n := range rt.nodesSnapshot() {
		if (lead != nil && n.ID == lead.ID) || (verifier != nil && n.ID == verifier.ID) {
			continue
		}
		workers = append(workers, n)
	}
	if len(workers) == 0 && lead != nil {
		workers = append(workers, *lead)
	}

	existingTasks, _ := rt.bb.ListTasks()
	if len(existingTasks) == 0 {
		var seededTitles []string
		if lead != nil {
			seedPrompt := fmt.Sprintf(
				"Decompose the mission goal into concrete tasks on the shared blackboard.\nEmit one `TASK: <concise title>` per line.\nGoal: %s",
				rt.snap.Goal,
			)
			leadOut, err := rt.runNode(ctx, lead.ID, seedPrompt, "", nil)
			if err != nil {
				return err
			}
			seededTitles = parseStructuredDirectiveLines(leadOut.FinalText, "TASK:")
			_, _ = rt.recordNote(lead.ID, "Board Task Seed", leadOut.FinalText)
		}
		if len(seededTitles) == 0 {
			seededTitles = []string{rt.snap.Goal}
		}
		for i, title := range seededTitles {
			tid := fmt.Sprintf("task-%d", i+1)
			_, _ = rt.bb.CreateTask(tid, "", title, rt.snap.Goal, 0)
		}
		rt.syncBlackboardState(EventTaskUpdated, "", "", "seeded blackboard tasks")
		if lead != nil {
			for _, w := range workers {
				rt.pulseEdge(lead.ID, w.ID, "tasks/*.md")
			}
		}
	}

	guard := NewConvergenceGuard(rt.maxIdleRounds, rt.maxRounds)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		var roundProgressed atomic.Bool
		sem := make(chan struct{}, rt.maxConcurrency)
		errCh := make(chan error, len(workers))
		var wg sync.WaitGroup

		for _, worker := range workers {
			worker := worker
			wg.Add(1)
			go func() {
				defer wg.Done()
				if ctx.Err() != nil {
					return
				}
				claimed, won, cErr := rt.bb.ClaimNextOpenTask(worker.ID, "")
				if cErr != nil {
					errCh <- cErr
					return
				}
				if !won || claimed == nil {
					return
				}
				roundProgressed.Store(true)
				rt.syncBlackboardState(EventTaskUpdated, worker.ID, claimed.ID, fmt.Sprintf("%s claimed %s", worker.ID, claimed.ID))

				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				}

				_, taskBody, _ := rt.bb.GetTaskWithBody(claimed.ID)
				var lease *WorktreeLease
				wtPath := ""
				if rt.shouldUseWorktree(worker.ID) {
					l, lErr := rt.acquireNodeWorktree(ctx, worker.ID, worker.ID)
					if lErr != nil {
						<-sem
						errCh <- lErr
						return
					}
					lease = l
					wtPath = l.Path
				}

				workerPrompt := fmt.Sprintf(
					"Shared Blackboard Task [id=%s, depth=%d, max_depth=%d, node=%s]:\nTitle: %s\nDetails: %s\nMission Goal: %s\nIf this task should be decomposed further (and depth < %d), reply with `SPLIT_TASK: <subtask>` lines; otherwise implement/complete it.",
					claimed.ID, claimed.Depth, MaxTaskSplitDepth, worker.ID, claimed.Title, taskBody, rt.snap.Goal, MaxTaskSplitDepth,
				)
				wOut, wErr := rt.runNode(ctx, worker.ID, workerPrompt, wtPath, nil)
				<-sem
				if wErr != nil {
					if lease != nil {
						rt.releaseWorktree(context.Background(), worker.ID, lease, true)
					}
					errCh <- wErr
					return
				}

				splitDirectives := parseStructuredDirectiveLines(wOut.FinalText, "SPLIT_TASK:")
				if len(splitDirectives) > 0 && claimed.Depth < MaxTaskSplitDepth {
					specs := make([]SubtaskSpec, len(splitDirectives))
					for idx, st := range splitDirectives {
						specs[idx] = SubtaskSpec{
							ID:          fmt.Sprintf("%s-%d", claimed.ID, idx+1),
							Title:       st,
							Description: rt.snap.Goal,
						}
					}
					if _, sErr := rt.bb.SplitTask(claimed.ID, worker.ID, specs); sErr == nil {
						if lease != nil {
							rt.releaseWorktree(ctx, worker.ID, lease, true)
						}
						_, _ = rt.recordNote(worker.ID, "Split "+claimed.ID, wOut.FinalText)
						rt.syncBlackboardState(EventTaskUpdated, worker.ID, claimed.ID, fmt.Sprintf("split %s into %d subtasks", claimed.ID, len(specs)))
						return
					}
				}

				_, _ = rt.recordNote(worker.ID, "Completed "+claimed.ID, wOut.FinalText)
				landedSHA := ""
				if lease != nil || verifier != nil || strings.TrimSpace(rt.verifyCommand) != "" {
					if verifier != nil {
						rt.pulseEdge(worker.ID, verifier.ID, "land "+claimed.ID)
					}
					landRes, lErr := rt.verifyAndMaybeLand(ctx, worker.ID, verifier, lease, fmt.Sprintf("feat(board): %s complete %s", worker.ID, claimed.ID))
					if lease != nil {
						rt.releaseWorktree(ctx, worker.ID, lease, true)
					}
					if lErr != nil {
						errCh <- lErr
						return
					}
					if !landRes.Accepted {
						errCh <- fmt.Errorf("shared-board refinery rejected %s (%s): %s", claimed.ID, worker.ID, landRes.Reason)
						return
					}
					landedSHA = landRes.LandedCommit
				}
				_, _ = rt.bb.CompleteTask(claimed.ID, worker.ID, landedSHA)
				rt.syncBlackboardState(EventTaskUpdated, worker.ID, claimed.ID, "completed "+claimed.ID)
			}()
		}

		wg.Wait()
		close(errCh)
		for err := range errCh {
			if err != nil {
				return err
			}
		}

		halt, reason := guard.ObserveRound(roundProgressed.Load())
		if halt {
			tasks, _ := rt.bb.ListTasks()
			unfinished := countUnfinishedBoardTasks(tasks)
			if unfinished > 0 {
				return fmt.Errorf("shared-board halted (%s) before convergence: %d task(s) remain unfinished", reason, unfinished)
			}
			rt.setSummary(fmt.Sprintf("Shared blackboard converged (%s) with %d total tasks.", reason, len(tasks)))
			return nil
		}
	}
}

func countUnfinishedBoardTasks(tasks []BlackboardTask) int {
	n := 0
	for _, t := range tasks {
		switch t.Status {
		case TaskCompleted, TaskMerged, TaskSplit:
		default:
			n++
		}
	}
	return n
}

// 6. TopoHybridFleet: 默认旗舰混合舰队（Lead 架构拆解 -> Explorer 并发侦察 -> Worktree Coder 隔离实现 -> Verifier 对抗门控合入）
func (rt *missionRuntime) runHybridFleet(ctx context.Context) error {
	lead := rt.firstNodeByRole("lead")
	explorers := rt.nodesByRole("explorer")
	coders := rt.nodesByRole("coder")
	verifier := rt.firstNodeByRole("verifier")

	var planSummary string
	if lead != nil {
		leadOut, err := rt.runNode(ctx, lead.ID, fmt.Sprintf("Plan architecture and implementation tasks for mission goal:\n%s", rt.snap.Goal), "", nil)
		if err != nil {
			return err
		}
		planSummary = leadOut.FinalText
		_, _ = rt.recordNote(lead.ID, "Architecture Plan", planSummary)
	}

	var reconParts []string
	if len(explorers) > 0 {
		reconResults := make([]string, len(explorers))
		sem := make(chan struct{}, rt.maxConcurrency)
		errCh := make(chan error, len(explorers))
		var wg sync.WaitGroup
		for i, exp := range explorers {
			i, exp := i, exp
			if lead != nil {
				rt.pulseEdge(lead.ID, exp.ID, "recon")
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				}
				defer func() { <-sem }()

				expPrompt := fmt.Sprintf("Reconnaissance for hybrid-fleet goal:\n%s\n\nLead Plan:\n%s", rt.snap.Goal, planSummary)
				out, err := rt.runNode(ctx, exp.ID, expPrompt, "", nil)
				if err != nil {
					errCh <- err
					return
				}
				reconResults[i] = out.FinalText
				_, _ = rt.recordNote(exp.ID, exp.Label+" Recon", out.FinalText)
			}()
		}
		wg.Wait()
		close(errCh)
		for err := range errCh {
			if err != nil {
				return err
			}
		}
		reconParts = reconResults
	}
	reconCombined := strings.Join(reconParts, "\n\n")

	if len(coders) == 0 {
		rt.setSummary(planSummary)
		return nil
	}

	sem := make(chan struct{}, rt.maxConcurrency)
	errCh := make(chan error, len(coders))
	var wg sync.WaitGroup

	maxRounds := rt.maxRounds
	if maxRounds <= 0 {
		maxRounds = DefaultMaxRounds
	}

	for i, coder := range coders {
		i, coder := i, coder
		if lead != nil {
			rt.pulseEdge(lead.ID, coder.ID, "implement")
		}
		tid := fmt.Sprintf("task-hybrid-%d", i+1)
		task, _ := rt.bb.CreateTask(tid, "", fmt.Sprintf("%s: %s", coder.Label, rt.snap.Goal), planSummary, 0)

		wg.Add(1)
		go func() {
			defer wg.Done()
			var lease *WorktreeLease
			if rt.shouldUseWorktree(coder.ID) {
				var lErr error
				lease, lErr = rt.acquireNodeWorktree(ctx, coder.ID, coder.ID)
				if lErr != nil {
					errCh <- lErr
					return
				}
				defer rt.releaseWorktree(context.Background(), coder.ID, lease, true)
			}
			wtPath := ""
			branch := ""
			if lease != nil {
				wtPath = lease.Path
				branch = lease.Branch
			}
			if task.ID != "" {
				_, _, _ = rt.bb.ClaimTask(task.ID, coder.ID, branch)
				rt.syncBlackboardState(EventTaskUpdated, coder.ID, task.ID, "claimed by "+coder.ID)
			}

			var critique string
			for round := 1; round <= maxRounds; round++ {
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				}

				cPrompt := fmt.Sprintf(
					"Hybrid Fleet Implementation (node=%s, round=%d/%d):\nGoal: %s\n\nLead Plan:\n%s\n\nExplorer Recon:\n%s",
					coder.ID, round, maxRounds, rt.snap.Goal, planSummary, reconCombined,
				)
				if critique != "" {
					cPrompt += "\n\nPrevious Verifier Critique to Fix:\n" + critique
				}

				_, cErr := rt.runNode(ctx, coder.ID, cPrompt, wtPath, nil)
				<-sem
				if cErr != nil {
					errCh <- cErr
					return
				}

				if verifier != nil {
					rt.pulseEdge(coder.ID, verifier.ID, fmt.Sprintf("audit (round %d)", round))
				}

				landRes, lErr := rt.verifyAndMaybeLand(ctx, coder.ID, verifier, lease, fmt.Sprintf("feat(hybrid): %s round %d", coder.ID, round))
				if lErr != nil {
					errCh <- lErr
					return
				}
				if landRes.Accepted {
					if task.ID != "" {
						_, _ = rt.bb.CompleteTask(task.ID, coder.ID, landRes.LandedCommit)
						rt.syncBlackboardState(EventTaskUpdated, coder.ID, task.ID, "landed "+landRes.LandedCommit)
					}
					if lease != nil {
						rt.releaseWorktree(ctx, coder.ID, lease, true)
					}
					return
				}
				critique = landRes.Reason
				if verifier != nil && round < maxRounds {
					rt.pulseEdge(verifier.ID, coder.ID, "audit feedback")
				}
			}
			errCh <- fmt.Errorf("hybrid-fleet coder %s exhausted %d audit rounds: %s", coder.ID, maxRounds, critique)
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}

	if strings.TrimSpace(planSummary) != "" {
		rt.setSummary(planSummary)
	} else {
		rt.setSummary("Hybrid fleet mission completed and verified.")
	}
	return nil
}

func shortSHA(sha string) string {
	trimmed := strings.TrimSpace(sha)
	if len(trimmed) > 8 {
		return trimmed[:8]
	}
	return trimmed
}
