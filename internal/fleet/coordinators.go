package fleet

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"gorness.dev/gorness/internal/config"
)

var errNodeStallReleased = errors.New("stalled node cancelled and blackboard tasks released")

type assignedTaskBrief struct {
	Task BlackboardTask
	Body string
}

type coderAssignment struct {
	Items []assignedTaskBrief
}

func (a coderAssignment) TaskIDs() []string {
	ids := make([]string, 0, len(a.Items))
	for _, it := range a.Items {
		if it.Task.ID != "" {
			ids = append(ids, it.Task.ID)
		}
	}
	return ids
}

func (a coderAssignment) FormatBrief() string {
	if len(a.Items) == 0 {
		return ""
	}
	if len(a.Items) == 1 {
		it := a.Items[0]
		desc := strings.TrimSpace(it.Body)
		if desc == "" || desc == strings.TrimSpace(it.Task.Title) {
			return fmt.Sprintf("Assigned Disjoint Task [%s]: %s", it.Task.ID, it.Task.Title)
		}
		return fmt.Sprintf("Assigned Disjoint Task [%s]: %s\nTask Brief:\n%s", it.Task.ID, it.Task.Title, desc)
	}
	var lines []string
	lines = append(lines, "Assigned Disjoint Tasks:")
	for idx, it := range a.Items {
		desc := strings.TrimSpace(it.Body)
		if desc != "" && desc != strings.TrimSpace(it.Task.Title) {
			lines = append(lines, fmt.Sprintf("%d. [%s] %s — %s", idx+1, it.Task.ID, it.Task.Title, desc))
		} else {
			lines = append(lines, fmt.Sprintf("%d. [%s] %s", idx+1, it.Task.ID, it.Task.Title))
		}
	}
	return strings.Join(lines, "\n")
}

var (
	reLinePrefix         = regexp.MustCompile(`^(?:#{1,6}\s+|>\s+|[-*+]\s+)+`)
	reVerdictReject      = regexp.MustCompile(`(?i)^(?:verdict|status|decision|result)\s*:\s*(?:reject|rejected|fail|failed|check_failed)(?:\s*$|[\s.!,:;\)\]]|\s+[-—])`)
	reVerdictAccept      = regexp.MustCompile(`(?i)^(?:verdict|status|decision|result)\s*:\s*(?:accept|accepted|pass|passed|approve|approved|lgtm)(?:\s*$|[\s.!,:;\)\]]|\s+[-—])`)
	reWordReject         = regexp.MustCompile(`(?i)\b(?:reject|rejected|fail|failed|check_failed)\b`)
	reStandaloneAccept   = regexp.MustCompile(`(?i)^\[?(?:accept|accepted|pass|passed|approve|approved|lgtm)\]?[.!]*$`)
	reInlineCodeOrBold   = strings.NewReplacer("*", "", "`", "", "~", "")
	reWordSplitDelims    = strings.NewReplacer("*", " ", "_", " ", "`", " ", "~", " ")
	reLeadingUnderscores = regexp.MustCompile(`(^|[^a-zA-Z0-9])_+`)
	reTrailingUnderscore = regexp.MustCompile(`_+([^a-zA-Z0-9]|$)`)
)

func stripInlineEmphasisPreserveSnake(s string) string {
	s = reInlineCodeOrBold.Replace(s)
	s = reLeadingUnderscores.ReplaceAllString(s, "$1")
	s = reTrailingUnderscore.ReplaceAllString(s, "$1")
	return s
}

func stripMarkdownVerdictLine(line string) string {
	s := strings.TrimSpace(line)
	s = reLinePrefix.ReplaceAllString(s, "")
	s = stripInlineEmphasisPreserveSnake(s)
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "[] ")
	return strings.TrimSpace(s)
}

func parseVerifierVerdict(raw string) (accepted bool, feedback string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false, "empty verifier output"
	}
	lines := strings.Split(trimmed, "\n")
	lastNonEmptyRaw := ""
	for i := len(lines) - 1; i >= 0; i-- {
		rawLine := strings.TrimSpace(lines[i])
		if rawLine == "" {
			continue
		}
		if lastNonEmptyRaw == "" {
			lastNonEmptyRaw = rawLine
		}
		cleanLine := stripMarkdownVerdictLine(rawLine)
		if cleanLine == "" {
			continue
		}
		if reVerdictReject.MatchString(cleanLine) {
			return false, trimmed
		}
		if reVerdictAccept.MatchString(cleanLine) {
			return true, trimmed
		}
	}

	if reWordReject.MatchString(trimmed) || reWordReject.MatchString(reWordSplitDelims.Replace(trimmed)) {
		return false, trimmed
	}
	if lastNonEmptyRaw != "" {
		cleanFinal := strings.TrimSpace(stripInlineEmphasisPreserveSnake(lastNonEmptyRaw))
		if reStandaloneAccept.MatchString(cleanFinal) {
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
		line := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(rawLine), "-*0123456789.) "))
		line = strings.TrimSpace(reInlineCodeOrBold.Replace(line))
		if len(line) > len(upperPrefix) && strings.EqualFold(line[:len(upperPrefix)], upperPrefix) {
			rest := strings.TrimSpace(line[len(upperPrefix):])
			if rest != "" {
				items = append(items, rest)
			}
		}
	}
	return items
}

var reNumberedOrBulletItem = regexp.MustCompile(`^(?:[-*+]\s+|\d+[.)]\s+)(.+)$`)

func splitTitleAndDetail(raw, fallbackDetail string) (string, string) {
	trimmed := strings.TrimSpace(raw)
	for _, sep := range []string{" :: ", " — ", " -- ", " | "} {
		if head, tail, ok := strings.Cut(trimmed, sep); ok && strings.TrimSpace(head) != "" && strings.TrimSpace(tail) != "" {
			return strings.TrimSpace(head), strings.TrimSpace(tail)
		}
	}
	if strings.TrimSpace(fallbackDetail) != "" {
		return trimmed, strings.TrimSpace(fallbackDetail)
	}
	return trimmed, trimmed
}

func parseTaskBreakdownSpecs(leadText, goal string, numCoders int) []SubtaskSpec {
	var specs []SubtaskSpec

	// 1. Explicit `TASK: <title> [:: <detail>]` directives from Lead or Goal.
	for _, src := range []string{leadText, goal} {
		if len(specs) > 0 {
			break
		}
		for _, item := range parseStructuredDirectiveLines(src, "TASK:") {
			title, detail := splitTitleAndDetail(item, "")
			if title != "" {
				specs = append(specs, SubtaskSpec{Title: title, Description: detail})
			}
		}
	}

	// 2. Markdown numbered or bulleted items from Lead output when multiple coders need distinct briefs.
	if len(specs) == 0 && strings.TrimSpace(leadText) != "" && numCoders > 1 {
		for _, rawLine := range strings.Split(leadText, "\n") {
			m := reNumberedOrBulletItem.FindStringSubmatch(strings.TrimSpace(rawLine))
			if len(m) == 2 {
				cand := strings.TrimSpace(stripInlineEmphasisPreserveSnake(m[1]))
				if cand != "" {
					title, detail := splitTitleAndDetail(cand, "")
					specs = append(specs, SubtaskSpec{Title: title, Description: detail})
				}
			}
		}
	}

	// 3. Multi-clause goal decomposition (numbered items or `;` separated clauses) when multiple coders run without a Lead breakdown.
	if len(specs) == 0 && numCoders > 1 {
		for _, rawLine := range strings.Split(goal, "\n") {
			m := reNumberedOrBulletItem.FindStringSubmatch(strings.TrimSpace(rawLine))
			if len(m) == 2 {
				cand := strings.TrimSpace(m[1])
				if cand != "" {
					title, detail := splitTitleAndDetail(cand, "")
					specs = append(specs, SubtaskSpec{Title: title, Description: detail})
				}
			}
		}
		if len(specs) == 0 && strings.Contains(goal, ";") {
			parts := strings.Split(goal, ";")
			if len(parts) >= 2 {
				for _, p := range parts {
					if cand := strings.TrimSpace(p); cand != "" {
						title, detail := splitTitleAndDetail(cand, "")
						specs = append(specs, SubtaskSpec{Title: title, Description: detail})
					}
				}
			}
		}
	}

	if numCoders <= 1 {
		if len(specs) == 0 {
			fallbackDetail := strings.TrimSpace(leadText)
			if fallbackDetail == "" {
				fallbackDetail = goal
			}
			specs = append(specs, SubtaskSpec{Title: goal, Description: fallbackDetail})
		}
		return specs
	}

	// Ensure at least numCoders distinct task cards exist so parallel worktrees never receive identical briefs.
	shortGoal := goal
	if len(shortGoal) > 96 {
		shortGoal = shortGoal[:96] + "..."
	}
	for len(specs) < numCoders {
		idx := len(specs) + 1
		specs = append(specs, SubtaskSpec{
			Title: fmt.Sprintf("Partition %d/%d: %s", idx, numCoders, shortGoal),
			Description: fmt.Sprintf(
				"Exclusive specialist partition %d of %d for mission goal: %s\nImplement only partition %d/%d scope and avoid duplicating sibling partitions.",
				idx, numCoders, goal, idx, numCoders,
			),
		})
	}
	return specs
}

// resolveOrSeedDisjointTasks 从黑板 `tasks/*.md` 加载现有待办任务（若为空则根据 Lead 拆解写入独立任务卡），
// 并按无交集分桶分配给 numCoders 个并行编码节点，彻底消除多 Worktree 重复收到相同目标的问题。
func (rt *missionRuntime) resolveOrSeedDisjointTasks(idPrefix, leadText, goal string, numCoders int) []coderAssignment {
	if numCoders <= 0 {
		return nil
	}

	loadOpen := func() []assignedTaskBrief {
		all, err := rt.bb.ListTasks()
		if err != nil {
			return nil
		}
		var open []assignedTaskBrief
		for _, t := range all {
			if t.Status != TaskOpen {
				continue
			}
			fullTask, body, rErr := rt.bb.GetTaskWithBody(t.ID)
			if rErr == nil {
				open = append(open, assignedTaskBrief{Task: fullTask, Body: body})
			} else {
				open = append(open, assignedTaskBrief{Task: t, Body: t.Title})
			}
		}
		return open
	}

	openTasks := loadOpen()
	if len(openTasks) == 0 {
		specs := parseTaskBreakdownSpecs(leadText, goal, numCoders)
		for i, sp := range specs {
			tid := strings.TrimSpace(sp.ID)
			if tid == "" {
				tid = fmt.Sprintf("%s-%d", idPrefix, i+1)
			}
			t, err := rt.bb.CreateTask(tid, "", sp.Title, sp.Description, 0)
			if err == nil {
				openTasks = append(openTasks, assignedTaskBrief{Task: t, Body: sp.Description})
			}
		}
		rt.syncBlackboardState(EventTaskUpdated, "", "", fmt.Sprintf("seeded %d disjoint task(s)", len(openTasks)))
	} else if len(openTasks) < numCoders {
		shortGoal := goal
		if len(shortGoal) > 96 {
			shortGoal = shortGoal[:96] + "..."
		}
		for len(openTasks) < numCoders {
			idx := len(openTasks) + 1
			tid := fmt.Sprintf("%s-part-%d", idPrefix, idx)
			desc := fmt.Sprintf("Supplemental specialist partition %d of %d for goal: %s", idx, numCoders, goal)
			t, err := rt.bb.CreateTask(tid, "", fmt.Sprintf("Partition %d/%d: %s", idx, numCoders, shortGoal), desc, 0)
			if err == nil {
				openTasks = append(openTasks, assignedTaskBrief{Task: t, Body: desc})
			} else {
				break
			}
		}
		rt.syncBlackboardState(EventTaskUpdated, "", "", "expanded tasks for parallel specialists")
	}

	assignments := make([]coderAssignment, numCoders)
	for idx, item := range openTasks {
		slot := idx % numCoders
		assignments[slot].Items = append(assignments[slot].Items, item)
	}
	return assignments
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
		synthPrompt = appendPeerNotesSection(synthPrompt, rt.peerNotesPromptBlock(lead.ID))
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
		prompt = appendPeerNotesSection(prompt, rt.peerNotesPromptBlock(coder.ID))

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

// 3. TopoWorktreeTeam: 独立 Git Worktree 并发专家战队（无交集独立任务分发） + Refinery 串行变基测试盲审快进合入
func (rt *missionRuntime) runWorktreeTeam(ctx context.Context) error {
	lead := rt.firstNodeByRole("lead")
	coders := rt.nodesByRole("coder")
	if len(coders) == 0 {
		return fmt.Errorf("worktree-team requires at least one coder node")
	}
	verifier := rt.firstNodeByRole("verifier")

	var leadPlan string
	if lead != nil {
		planPrompt := fmt.Sprintf(
			"Decompose the mission goal into %d disjoint specialist tasks for isolated Git worktrees.\nEmit one `TASK: <concise title> :: <distinct file scope & brief>` per line.\nGoal: %s",
			len(coders), rt.snap.Goal,
		)
		leadOut, err := rt.runNode(ctx, lead.ID, planPrompt, "", nil)
		if err != nil {
			return err
		}
		leadPlan = leadOut.FinalText
		_, _ = rt.recordNote(lead.ID, "Worktree Task Breakdown", leadPlan)
	}

	assignments := rt.resolveOrSeedDisjointTasks("task", leadPlan, rt.snap.Goal, len(coders))

	leases := make(map[string]*WorktreeLease, len(coders))
	for _, c := range coders {
		if lead != nil {
			rt.pulseEdge(lead.ID, c.ID, "dispatch task")
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
		assignment := assignments[i]

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
			for _, tid := range assignment.TaskIDs() {
				_, _, _ = rt.bb.ClaimTask(tid, coder.ID, branchName)
				rt.syncBlackboardState(EventTaskUpdated, coder.ID, tid, "claimed by "+coder.ID)
			}

			taskBrief := assignment.FormatBrief()
			prompt := fmt.Sprintf(
				"Worktree Specialist Assignment (node=%s, label=%s, specialist=%d/%d):\n%s\nMission Context: %s\nWork strictly within your isolated Git worktree and assigned task scope.",
				coder.ID, coder.Label, i+1, len(coders), taskBrief, rt.snap.Goal,
			)
			prompt = appendPeerNotesSection(prompt, rt.peerNotesPromptBlock(coder.ID))

			coderOut, err := rt.runNode(runCtx, coder.ID, prompt, wtPath, nil)
			if err != nil {
				cancelTeam()
				errCh <- err
				return
			}
			// 编码运行已完成，立即释放并发槽位供其他队友或 Refinery Verifier 使用
			releaseSem()

			_, _ = rt.recordNote(coder.ID, "Specialist Patch Ready", coderOut.FinalText)

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

			for _, tid := range assignment.TaskIDs() {
				_, _ = rt.bb.CompleteTask(tid, coder.ID, landRes.LandedCommit)
				rt.syncBlackboardState(EventTaskUpdated, coder.ID, tid, "landed "+landRes.LandedCommit)
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
	coderPrompt = appendPeerNotesSection(coderPrompt, rt.peerNotesPromptBlock(coder.ID))
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
		dbgPrompt = appendPeerNotesSection(dbgPrompt, rt.peerNotesPromptBlock(debugger.ID))
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
					"Shared Blackboard Task [id=%s, depth=%d, max_depth=%d, node=%s]:\nTitle: %s\nDetails: %s\nMission Goal: %s\nIf this task should be decomposed further (and depth < %d), call `blackboard_split_task` or reply with `SPLIT_TASK: <subtask>` lines; otherwise implement/complete it.",
					claimed.ID, claimed.Depth, MaxTaskSplitDepth, worker.ID, claimed.Title, taskBody, rt.snap.Goal, MaxTaskSplitDepth,
				)
				workerPrompt = appendPeerNotesSection(workerPrompt, rt.peerNotesPromptBlock(worker.ID))
				wOut, wErr := rt.runNode(ctx, worker.ID, workerPrompt, wtPath, nil)
				<-sem
				if wErr != nil {
					if lease != nil {
						rt.releaseWorktree(context.Background(), worker.ID, lease, true)
					}
					if errors.Is(wErr, errNodeStallReleased) {
						// Witness 已将该僵死工人的任务卡释放回 open，交由后续轮次或其他对等工人接手，不中断全局黑板
						return
					}
					errCh <- wErr
					return
				}

				// 若节点运行中已通过 blackboard_split_task 工具将该卡动态拆解，则直接释放临时工作树进入下一轮收敛
				if curTask, _, gErr := rt.bb.GetTaskWithBody(claimed.ID); gErr == nil && curTask.Status == TaskSplit {
					if lease != nil {
						rt.releaseWorktree(ctx, worker.ID, lease, true)
					}
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

// 6. TopoHybridFleet: 默认旗舰混合舰队（Lead 架构拆解独立任务 -> Explorer 并发侦察 -> Worktree Coder 无交集隔离实现 -> Verifier 对抗门控合入）
func (rt *missionRuntime) runHybridFleet(ctx context.Context) error {
	lead := rt.firstNodeByRole("lead")
	explorers := rt.nodesByRole("explorer")
	coders := rt.nodesByRole("coder")
	verifier := rt.firstNodeByRole("verifier")

	var planSummary string
	if lead != nil {
		leadPrompt := fmt.Sprintf(
			"Plan architecture and decompose concrete disjoint implementation tasks for mission goal:\n%s\nEmit one `TASK: <concise title> :: <distinct scope and files>` line per implementation task (%d coder specialist(s)).",
			rt.snap.Goal, max(1, len(coders)),
		)
		leadOut, err := rt.runNode(ctx, lead.ID, leadPrompt, "", nil)
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
				expPrompt = appendPeerNotesSection(expPrompt, rt.peerNotesPromptBlock(exp.ID))
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

	assignments := rt.resolveOrSeedDisjointTasks("task-hybrid", planSummary, rt.snap.Goal, len(coders))

	sem := make(chan struct{}, rt.maxConcurrency)
	errCh := make(chan error, len(coders))
	var wg sync.WaitGroup

	maxRounds := rt.maxRounds
	if maxRounds <= 0 {
		maxRounds = DefaultMaxRounds
	}

	for i, coder := range coders {
		i, coder := i, coder
		assignment := assignments[i]
		if lead != nil {
			rt.pulseEdge(lead.ID, coder.ID, "implement")
		}

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
			for _, tid := range assignment.TaskIDs() {
				_, _, _ = rt.bb.ClaimTask(tid, coder.ID, branch)
				rt.syncBlackboardState(EventTaskUpdated, coder.ID, tid, "claimed by "+coder.ID)
			}

			taskBrief := assignment.FormatBrief()
			var critique string
			for round := 1; round <= maxRounds; round++ {
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				}

				cPrompt := fmt.Sprintf(
					"Hybrid Fleet Implementation (node=%s, specialist=%d/%d, round=%d/%d):\n%s\n\nMission Goal: %s\n\nLead Plan:\n%s\n\nExplorer Recon:\n%s",
					coder.ID, i+1, len(coders), round, maxRounds, taskBrief, rt.snap.Goal, planSummary, reconCombined,
				)
				if critique != "" {
					cPrompt += "\n\nPrevious Verifier Critique to Fix:\n" + critique
				}
				cPrompt = appendPeerNotesSection(cPrompt, rt.peerNotesPromptBlock(coder.ID))

				cOut, cErr := rt.runNode(ctx, coder.ID, cPrompt, wtPath, nil)
				<-sem
				if cErr != nil {
					errCh <- cErr
					return
				}
				_, _ = rt.recordNote(coder.ID, "Hybrid Specialist Implementation", cOut.FinalText)

				if verifier != nil {
					rt.pulseEdge(coder.ID, verifier.ID, fmt.Sprintf("audit (round %d)", round))
				}

				landRes, lErr := rt.verifyAndMaybeLand(ctx, coder.ID, verifier, lease, fmt.Sprintf("feat(hybrid): %s round %d", coder.ID, round))
				if lErr != nil {
					errCh <- lErr
					return
				}
				if landRes.Accepted {
					for _, tid := range assignment.TaskIDs() {
						_, _ = rt.bb.CompleteTask(tid, coder.ID, landRes.LandedCommit)
						rt.syncBlackboardState(EventTaskUpdated, coder.ID, tid, "landed "+landRes.LandedCommit)
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
