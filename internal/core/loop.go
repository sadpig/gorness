package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/llm"
	"gorness.dev/gorness/internal/store"
	"gorness.dev/gorness/internal/tools"
)

const (
	DoomLoopThreshold          = 3
	DefaultMaxToolConcurrency  = 8
	DefaultMaxSteps            = 64
	AutoCompactionContextRatio = 0.85
)

var (
	ErrDoomLoop                = errors.New("doom loop detected: 3 consecutive identical failing tool calls")
	ErrMaxStepsExceeded        = errors.New("agent loop exceeded maximum step limit")
	reClosingSystemReminderTag = regexp.MustCompile(`(?i)</\s*system-reminder\b[^>]*>`)
)

func sanitizeSystemReminderContent(raw string) string {
	clean := config.EscapeBidi(raw)
	return reClosingSystemReminderTag.ReplaceAllString(clean, "&lt;/system-reminder&gt;")
}

// errGroup 提供零外部依赖的受限并发协程组（语义等价于 x/sync/errgroup + SetLimit），严格遵守五依赖上限铁律。
type errGroup struct {
	ctx    context.Context
	cancel context.CancelFunc
	sem    chan struct{}
	wg     sync.WaitGroup

	errOnce sync.Once
	err     error
}

func newErrGroup(parent context.Context, limit int) *errGroup {
	if limit <= 0 {
		limit = DefaultMaxToolConcurrency
	}
	ctx, cancel := context.WithCancel(parent)
	return &errGroup{
		ctx:    ctx,
		cancel: cancel,
		sem:    make(chan struct{}, limit),
	}
}

func (g *errGroup) Go(fn func(ctx context.Context) error) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		select {
		case g.sem <- struct{}{}:
		case <-g.ctx.Done():
			g.recordErr(g.ctx.Err())
			return
		}
		defer func() { <-g.sem }()

		if err := fn(g.ctx); err != nil {
			g.recordErr(err)
		}
	}()
}

func (g *errGroup) recordErr(err error) {
	if err == nil {
		return
	}
	g.errOnce.Do(func() {
		g.err = err
		g.cancel()
	})
}

func (g *errGroup) Wait() error {
	g.wg.Wait()
	g.cancel()
	return g.err
}

type doomTracker struct {
	lastKey string
	count   int
}

func (d *doomTracker) observe(toolName string, rawArgs json.RawMessage, failed bool) (count int, triggered bool) {
	if !failed {
		d.lastKey = ""
		d.count = 0
		return 0, false
	}
	key := canonicalToolKey(toolName, rawArgs)
	if key == d.lastKey {
		d.count++
	} else {
		d.lastKey = key
		d.count = 1
	}
	return d.count, d.count >= DoomLoopThreshold
}

func canonicalToolKey(toolName string, rawArgs json.RawMessage) string {
	trimmed := bytes.TrimSpace(rawArgs)
	var norm string
	if len(trimmed) == 0 {
		norm = "{}"
	} else {
		var buf bytes.Buffer
		if err := json.Compact(&buf, trimmed); err == nil {
			norm = buf.String()
		} else {
			norm = string(trimmed)
		}
	}
	return strings.TrimSpace(toolName) + "\x00" + norm
}

type RunOutcome struct {
	Status          string               `json:"status"` // "completed" | "deferred" | "doom_loop" | "cancelled" | "failed"
	StopReason      string               `json:"stop_reason,omitempty"`
	Steps           int                  `json:"steps"`
	FinalText       string               `json:"final_text,omitempty"`
	InputTokens     int                  `json:"input_tokens"`
	OutputTokens    int                  `json:"output_tokens"`
	CacheReadTokens int                  `json:"cache_read_tokens"`
	CostUSD         float64              `json:"cost_usd"`
	Deferred        *DeferredToolPayload `json:"deferred,omitempty"`
	CompletedTurn1  bool                 `json:"-"`
}

type agentLoopConfig struct {
	runID              string
	ws                 *store.Workspace
	sessionID          string
	effectiveRoot      string
	userHomeDir        string
	agentRole          string
	prompt             string
	route              llm.ResolvedRoute
	snap               *config.Snapshot
	profile            *config.AgentProfile
	permMode           string
	sandboxMode        tools.SandboxMode
	unattended         bool
	maxSteps           int
	maxToolConcurrency int
	allowedTools       []string
	extraWriteDirs     []string
	staticReminders    []string
	initialDynamic     []string
	resumeApproval     *DeferredResume
	configTampered     bool

	reg       *tools.Registry
	pathGuard *tools.PathGuard
	skills    *tools.SkillCatalog
	sessions  *store.SessionStore
	snapshots *store.SnapshotStore
	compactor *Compactor
	router    *llm.Router
	hooks     *HookBus
	gate      *PermissionGate
	broker    *ApprovalBroker
	grants    *GrantStore
	secrets   []string
	now       func() time.Time

	emit              func(eventType string, payload any)
	drainSteer        func() []string
	onStepChange      func(step int, status string)
	onWaitingApproval func(waiting bool)
}

// AgentLoop 驱动单会话多步推理与工具执行循环：
// 1. 静态系统前缀（frozenSystem）在整次运行内字节级锁定，中途 Steer 指令与路径技能仅以 <system-reminder> 注入步边界末尾消息；
// 2. 连续并发安全只读工具通过受限 errGroup（上限 8）并行扇出，写/Shell/MCP 工具严格串行；
// 3. 连续 3 次同参失败调用立即触发 DoomLoop 熔断。
type AgentLoop struct {
	cfg             agentLoopConfig
	activatedTools  map[string]bool
	activatedSkills map[string]bool
	doom            doomTracker
}

func newAgentLoop(cfg agentLoopConfig) *AgentLoop {
	if cfg.maxSteps <= 0 {
		cfg.maxSteps = DefaultMaxSteps
	}
	if cfg.maxToolConcurrency <= 0 {
		cfg.maxToolConcurrency = DefaultMaxToolConcurrency
	}
	if cfg.maxToolConcurrency > DefaultMaxToolConcurrency {
		cfg.maxToolConcurrency = DefaultMaxToolConcurrency
	}
	if cfg.now == nil {
		cfg.now = time.Now
	}
	return &AgentLoop{
		cfg:             cfg,
		activatedTools:  make(map[string]bool),
		activatedSkills: make(map[string]bool),
	}
}

func (l *AgentLoop) Run(ctx context.Context) (RunOutcome, error) {
	var outcome RunOutcome

	priorBranch, err := l.cfg.sessions.LoadActiveBranch(l.cfg.sessionID)
	if err != nil {
		return outcome, err
	}
	wasEmptySession := len(priorBranch) == 0
	l.rehydrateActivatedState(priorBranch)

	// 1. 修复可能残留的未闭合 tool_use，或执行挂起恢复的 Deferred 工具调用
	if err := l.prepareInitialSessionState(ctx, priorBranch); err != nil {
		return outcome, err
	}

	// 2. 追加本轮用户输入（若非纯 ResumeApproval 唤醒）
	if trimmedPrompt := strings.TrimSpace(l.cfg.prompt); trimmedPrompt != "" {
		if _, err := l.cfg.sessions.Append(l.cfg.sessionID, store.SessionEntry{
			Role:      "user",
			AgentRole: l.cfg.agentRole,
			Blocks: []store.ContentBlock{
				{Type: "text", Text: trimmedPrompt},
			},
			CreatedAt: l.cfg.now().UTC(),
		}); err != nil {
			return outcome, err
		}
	}

	// 3. 预构建整次运行恒定不变的静态系统提示词前缀（保障 Prompt Cache 最高命中率）
	frozenSystem := l.buildStaticSystemPrompt()
	var pendingSkillReminders []string

	for step := 1; step <= l.cfg.maxSteps; step++ {
		if ctx.Err() != nil {
			outcome.Status = "cancelled"
			return outcome, ctx.Err()
		}

		outcome.Steps = step
		if l.cfg.onStepChange != nil {
			l.cfg.onStepChange(step, "running")
		}
		l.emit(EventStepStarted, map[string]any{"step": step})

		// 步边界 1：排空并注入所有运行中到达的 Steer 指令（挂载为末尾 <system-reminder>，绝不改动 frozenSystem）
		if err := l.flushQueuedSteers(); err != nil {
			return outcome, err
		}

		// 执行 PreStep 钩子
		preStepEv := &PreStepEvent{
			RunID:       l.cfg.runID,
			WorkspaceID: l.cfg.ws.ID,
			SessionID:   l.cfg.sessionID,
			AgentRole:   l.cfg.agentRole,
			Step:        step,
		}
		if l.cfg.hooks != nil {
			if err := l.cfg.hooks.RunPreStep(ctx, preStepEv); err != nil {
				outcome.Status = "failed"
				return outcome, err
			}
		}

		history, err := l.cfg.sessions.LoadActiveBranch(l.cfg.sessionID)
		if err != nil {
			outcome.Status = "failed"
			return outcome, err
		}

		visibleTools := filterSchemasForRun(l.cfg.reg.VisibleSchemas(l.activatedTools), l.cfg.profile, l.cfg.allowedTools)

		var stepReminders []string
		if step == 1 && len(l.cfg.initialDynamic) > 0 {
			stepReminders = append(stepReminders, l.cfg.initialDynamic...)
		}
		if len(pendingSkillReminders) > 0 {
			stepReminders = append(stepReminders, pendingSkillReminders...)
			pendingSkillReminders = nil
		}
		if len(preStepEv.DynamicReminders) > 0 {
			stepReminders = append(stepReminders, preStepEv.DynamicReminders...)
		}

		sortedTools, _, augHistory := llm.AssemblePrompt(nil, visibleTools, history, stepReminders)

		streamCh, err := l.cfg.router.StreamWithRetry(ctx, l.cfg.route, llm.Request{
			Model:        l.cfg.route.ModelID,
			Effort:       l.cfg.route.Effort,
			StaticSystem: frozenSystem,
			History:      augHistory,
			Tools:        sortedTools,
		})
		if err != nil {
			if ctx.Err() != nil {
				outcome.Status = "cancelled"
				return outcome, ctx.Err()
			}
			outcome.Status = "failed"
			return outcome, err
		}

		stepRes, err := l.consumeStepStream(ctx, step, streamCh)
		if err != nil {
			if ctx.Err() != nil {
				outcome.Status = "cancelled"
				return outcome, ctx.Err()
			}
			outcome.Status = "failed"
			return outcome, err
		}

		outcome.InputTokens += stepRes.inputTokens
		outcome.OutputTokens += stepRes.outputTokens
		outcome.CacheReadTokens += stepRes.cacheReadTokens
		answeringModel := l.cfg.route.ModelID
		answeringSpec := l.cfg.route.Spec
		if strings.TrimSpace(stepRes.modelID) != "" {
			answeringModel = strings.TrimSpace(stepRes.modelID)
			answeringSpec = resolveRouteSpec(l.cfg.route, l.cfg.snap, stepRes.providerID, answeringModel)
		}
		stepCost := llm.EstimateCostUSD(answeringSpec, stepRes.inputTokens, stepRes.outputTokens, stepRes.cacheReadTokens)
		outcome.CostUSD += stepCost

		assistantBlocks := buildAssistantContentBlocks(stepRes)
		var assistantSeq uint64
		if len(assistantBlocks) > 0 {
			storedAssistant, aErr := l.cfg.sessions.Append(l.cfg.sessionID, store.SessionEntry{
				Role:            "assistant",
				ModelID:         answeringModel,
				AgentRole:       l.cfg.agentRole,
				Blocks:          assistantBlocks,
				InputTokens:     stepRes.inputTokens,
				OutputTokens:    stepRes.outputTokens,
				CacheReadTokens: stepRes.cacheReadTokens,
				CostUSD:         stepCost,
				CreatedAt:       l.cfg.now().UTC(),
			})
			if aErr != nil {
				outcome.Status = "failed"
				return outcome, aErr
			}
			assistantSeq = storedAssistant.Seq
		}

		// 若本步无工具调用：检查流式期间是否有新 Steer 抵达，以及 TurnStopping 钩子是否拦截结束
		if len(stepRes.toolCalls) == 0 {
			injected, sErr := l.flushQueuedSteersCount()
			if sErr != nil {
				outcome.Status = "failed"
				return outcome, sErr
			}
			if injected > 0 {
				l.maybeAutoCompact(ctx, answeringSpec, stepRes.inputTokens)
				continue
			}

			stopEv := &TurnStoppingEvent{
				RunID:       l.cfg.runID,
				WorkspaceID: l.cfg.ws.ID,
				SessionID:   l.cfg.sessionID,
				AgentRole:   l.cfg.agentRole,
				Step:        step,
				StopReason:  stepRes.stopReason,
			}
			if l.cfg.hooks != nil {
				if hErr := l.cfg.hooks.RunTurnStopping(ctx, stopEv); hErr != nil {
					outcome.Status = "failed"
					return outcome, hErr
				}
			}
			if !stopEv.VetoStop && l.cfg.snap != nil && len(l.cfg.snap.Hooks.TurnStopping) > 0 {
				if hErr := ExecuteTurnStoppingCommandHooks(ctx, l.cfg.snap.Hooks.TurnStopping, l.cfg.effectiveRoot, l.cfg.secrets, stopEv); hErr != nil {
					outcome.Status = "failed"
					return outcome, hErr
				}
			}
			if stopEv.VetoStop {
				fp := strings.TrimSpace(stopEv.FollowUpPrompt)
				if fp == "" {
					fp = "TurnStopping hook vetoed stop; verify requirements before completing."
				}
				pendingSkillReminders = append(pendingSkillReminders, sanitizeSystemReminderContent(fp))
				l.maybeAutoCompact(ctx, answeringSpec, stepRes.inputTokens)
				continue
			}

			l.maybeAutoCompact(ctx, answeringSpec, stepRes.inputTokens)

			outcome.Status = "completed"
			outcome.StopReason = stepRes.stopReason
			outcome.FinalText = stepRes.text
			outcome.CompletedTurn1 = wasEmptySession
			return outcome, nil
		}

		// 执行本步全部工具调用（只读并发扇出 + 写/Shell/MCP 串行 + PermissionGate + ApprovalBroker + DoomLoop 检测）
		execBatch, err := l.executeStepTools(ctx, step, assistantSeq, stepRes.toolCalls)
		if err != nil {
			if ctx.Err() != nil {
				outcome.Status = "cancelled"
				return outcome, ctx.Err()
			}
			outcome.Status = "failed"
			return outcome, err
		}

		if len(execBatch.resultBlocks) > 0 {
			if _, err := l.cfg.sessions.Append(l.cfg.sessionID, store.SessionEntry{
				Role:      "tool",
				AgentRole: l.cfg.agentRole,
				Blocks:    execBatch.resultBlocks,
				CreatedAt: l.cfg.now().UTC(),
			}); err != nil {
				outcome.Status = "failed"
				return outcome, err
			}
		}

		if len(execBatch.skillReminders) > 0 {
			pendingSkillReminders = append(pendingSkillReminders, execBatch.skillReminders...)
		}

		// 无人值守遇到 GateDefer：发出清晰挂起事件并立即释放运行协程，绝不阻塞后台队列
		if execBatch.deferred != nil {
			outcome.Status = "deferred"
			outcome.StopReason = "deferred"
			outcome.Deferred = execBatch.deferred
			l.emit(EventToolDeferred, *execBatch.deferred)
			return outcome, nil
		}

		// 连续 3 次完全相同且失败的工具调用：触发 DoomLoop 熔断
		if execBatch.doom != nil {
			outcome.Status = "doom_loop"
			outcome.StopReason = "doom_loop"
			l.emit(EventDoomLoop, *execBatch.doom)
			return outcome, ErrDoomLoop
		}

		// 当输入上下文达到模型 ContextWindow 的 85% 及以上时，在步边界自动执行锚定增量压缩
		l.maybeAutoCompact(ctx, answeringSpec, stepRes.inputTokens)
	}

	outcome.Status = "failed"
	outcome.StopReason = "max_steps"
	return outcome, ErrMaxStepsExceeded
}

type streamedStep struct {
	providerID      string
	modelID         string
	thinking        string
	signature       string
	text            string
	compactions     []json.RawMessage
	toolCalls       []ToolInvocation
	stopReason      string
	inputTokens     int
	outputTokens    int
	cacheReadTokens int
}

func (l *AgentLoop) consumeStepStream(ctx context.Context, step int, ch <-chan llm.StreamEvent) (streamedStep, error) {
	var (
		thinkBuf    strings.Builder
		sigBuf      strings.Builder
		textBuf     strings.Builder
		compactions []json.RawMessage
		toolCalls   []ToolInvocation
		res         streamedStep
	)

	for ev := range ch {
		switch ev.Type {
		case llm.StreamStepReset:
			thinkBuf.Reset()
			sigBuf.Reset()
			textBuf.Reset()
			compactions = nil
			toolCalls = nil
			l.emit(EventStepReset, map[string]any{"step": step, "attempt": ev.Attempt})

		case llm.StreamRetry:
			var errMsg string
			if ev.Err != nil {
				errMsg = config.Redact(ev.Err.Error(), l.cfg.secrets)
			}
			l.emit(EventRetry, map[string]any{"step": step, "attempt": ev.Attempt, "error": errMsg})

		case llm.StreamThinkingDelta:
			thinkBuf.WriteString(ev.Thinking)
			sigBuf.WriteString(ev.Signature)
			if ev.Thinking != "" {
				l.emit(EventThinkingDelta, map[string]any{"step": step, "thinking": ev.Thinking})
			}

		case llm.StreamTextDelta:
			textBuf.WriteString(ev.Text)
			if ev.Text != "" {
				l.emit(EventTextDelta, map[string]any{"step": step, "text": ev.Text})
			}

		case llm.StreamCompaction:
			if len(ev.Compaction) > 0 {
				compactions = append(compactions, append(json.RawMessage(nil), ev.Compaction...))
			}

		case llm.StreamToolCall:
			callID := strings.TrimSpace(ev.ToolCallID)
			if callID == "" {
				callID = fmt.Sprintf("call_%d_%d", step, len(toolCalls)+1)
			}
			args := ev.ToolInput
			if len(bytes.TrimSpace(args)) == 0 {
				args = json.RawMessage(`{}`)
			}
			inv := ToolInvocation{
				ToolCallID: callID,
				ToolName:   strings.TrimSpace(ev.ToolName),
				Args:       append(json.RawMessage(nil), args...),
				Signature:  ev.Signature,
			}
			toolCalls = append(toolCalls, inv)
			l.emit(EventToolCall, ToolCallPayload{
				Step:       step,
				ToolCallID: inv.ToolCallID,
				ToolName:   inv.ToolName,
				Args:       inv.Args,
			})

		case llm.StreamDone:
			res.providerID = ev.ProviderID
			res.modelID = ev.ModelID
			res.stopReason = ev.StopReason
			res.inputTokens = ev.InputTokens
			res.outputTokens = ev.OutputTokens
			res.cacheReadTokens = ev.CacheReadTokens

		case llm.StreamError:
			if ev.Err != nil {
				return res, ev.Err
			}
			return res, errors.New("llm stream terminated with error")
		}
	}

	if ctx.Err() != nil {
		return res, ctx.Err()
	}

	res.thinking = thinkBuf.String()
	res.signature = sigBuf.String()
	res.text = textBuf.String()
	res.compactions = compactions
	res.toolCalls = toolCalls
	return res, nil
}

func buildAssistantContentBlocks(s streamedStep) []store.ContentBlock {
	var blocks []store.ContentBlock
	if s.thinking != "" || s.signature != "" {
		blocks = append(blocks, store.ContentBlock{
			Type:      "thinking",
			Thinking:  s.thinking,
			Signature: s.signature,
		})
	}
	for _, comp := range s.compactions {
		blocks = append(blocks, store.ContentBlock{
			Type:           "compaction",
			CompactionData: comp,
		})
	}
	if s.text != "" {
		blocks = append(blocks, store.ContentBlock{
			Type: "text",
			Text: s.text,
		})
	}
	for _, tc := range s.toolCalls {
		blocks = append(blocks, store.ContentBlock{
			Type:       "tool_use",
			ToolCallID: tc.ToolCallID,
			ToolName:   tc.ToolName,
			ToolInput:  tc.Args,
			Signature:  tc.Signature,
		})
	}
	return blocks
}

type stepToolsExecution struct {
	resultBlocks   []store.ContentBlock
	skillReminders []string
	deferred       *DeferredToolPayload
	doom           *DoomLoopPayload
}

func (l *AgentLoop) executeStepTools(
	ctx context.Context,
	step int,
	stepSeq uint64,
	calls []ToolInvocation,
) (stepToolsExecution, error) {
	var out stepToolsExecution

	// 解析工具元数据
	resolved := make([]ToolInvocation, len(calls))
	for i, c := range calls {
		inv := c
		if t, ok := l.cfg.reg.Get(c.ToolName); ok {
			tCopy := t
			inv.Tool = &tCopy
		}
		resolved[i] = inv
	}

	idx := 0
	for idx < len(resolved) {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}

		// 检查是否可以组建连续并发安全只读批次
		var (
			cachedPreHook    GateResult
			cachedPreHookErr error
			hasCachedPreHook bool
		)
		if l.isConcurrentReadCandidate(resolved[idx]) {
			var parallelBatch []ToolInvocation
			for idx < len(resolved) && l.isConcurrentReadCandidate(resolved[idx]) {
				inv := resolved[idx]
				hookDec, hErr := l.runPreToolHooks(ctx, step, inv)
				if hErr != nil {
					cachedPreHook = hookDec
					cachedPreHookErr = hErr
					hasCachedPreHook = true
					break
				}
				gateRes := l.evaluateGate(inv, hookDec)
				if gateRes.Action != GateAllow {
					cachedPreHook = hookDec
					hasCachedPreHook = true
					break
				}
				parallelBatch = append(parallelBatch, inv)
				idx++
			}

			if len(parallelBatch) > 0 {
				batchBlocks, touchedPaths, err := l.runParallelReadBatch(ctx, step, stepSeq, parallelBatch)
				if err != nil {
					return out, err
				}
				for j, blk := range batchBlocks {
					out.resultBlocks = append(out.resultBlocks, blk)
					l.emit(EventToolResult, ToolResultPayload{
						Step:       step,
						ToolCallID: blk.ToolCallID,
						ToolName:   blk.ToolName,
						Output:     blk.Text,
						IsError:    blk.IsError,
					})
					if cnt, hit := l.doom.observe(parallelBatch[j].ToolName, parallelBatch[j].Args, blk.IsError); hit {
						out.doom = &DoomLoopPayload{
							ToolName:            parallelBatch[j].ToolName,
							Args:                parallelBatch[j].Args,
							ConsecutiveFailures: cnt,
							LastError:           blk.Text,
						}
						// 为本步尚未执行的剩余 tool_use 补齐中断结果，避免会话历史残留悬空调用
						for rem := idx; rem < len(resolved); rem++ {
							out.resultBlocks = append(out.resultBlocks, makeSkippedToolResult(resolved[rem], "Skipped after doom loop halt."))
						}
						return out, nil
					}
				}
				out.skillReminders = append(out.skillReminders, l.collectPathSkillReminders(touchedPaths)...)
				continue
			}
		}

		// 串行执行单个工具调用（支持按需即时 StageEdit、500ms 审批卡部分块合并、GateDefer 非阻塞挂起）
		inv := resolved[idx]
		idx++

		blk, touched, deferred, fatalErr := l.runSerialTool(ctx, step, stepSeq, inv, cachedPreHook, cachedPreHookErr, hasCachedPreHook)
		if fatalErr != nil {
			return out, fatalErr
		}
		if deferred != nil {
			out.deferred = deferred
			return out, nil
		}

		out.resultBlocks = append(out.resultBlocks, blk)
		l.emit(EventToolResult, ToolResultPayload{
			Step:       step,
			ToolCallID: blk.ToolCallID,
			ToolName:   blk.ToolName,
			Output:     blk.Text,
			IsError:    blk.IsError,
		})
		if touched != "" {
			out.skillReminders = append(out.skillReminders, l.collectPathSkillReminders([]string{touched})...)
		}

		if cnt, hit := l.doom.observe(inv.ToolName, inv.Args, blk.IsError); hit {
			out.doom = &DoomLoopPayload{
				ToolName:            inv.ToolName,
				Args:                inv.Args,
				ConsecutiveFailures: cnt,
				LastError:           blk.Text,
			}
			for rem := idx; rem < len(resolved); rem++ {
				out.resultBlocks = append(out.resultBlocks, makeSkippedToolResult(resolved[rem], "Skipped after doom loop halt."))
			}
			return out, nil
		}
	}

	return out, nil
}

func (l *AgentLoop) isConcurrentReadCandidate(inv ToolInvocation) bool {
	if inv.AlwaysAsk || inv.Tool == nil {
		return false
	}
	return inv.Tool.ConcurrencySafe && inv.Tool.ReadOnly && !inv.Tool.Mutating && !inv.Tool.IsMCP
}

func (l *AgentLoop) runParallelReadBatch(
	ctx context.Context,
	step int,
	stepSeq uint64,
	batch []ToolInvocation,
) ([]store.ContentBlock, []string, error) {
	type slotResult struct {
		res      tools.ToolResult
		execErr  error
		duration time.Duration
	}
	slots := make([]slotResult, len(batch))

	// 为并发只读调用预快照已激活表，执行完毕后在主协程统一合并，彻底杜绝并发 map 读写竞态
	grp := newErrGroup(ctx, l.cfg.maxToolConcurrency)
	for i := range batch {
		i := i
		inv := batch[i]
		localActTools := make(map[string]bool)
		localActSkills := make(map[string]bool)

		grp.Go(func(gCtx context.Context) error {
			started := l.cfg.now()
			// 批次内所有工具均已通过 PermissionGate 第 1..9 步返回 GateAllow（含 saved-grant / AllowPaths / yolo 授权的工作区外读取）
			callCtx := l.makeToolCallContext(stepSeq, true, nil)
			callCtx.ActivatedTools = localActTools
			callCtx.ActivatedSkills = localActSkills

			res, err := l.cfg.reg.Execute(gCtx, inv.ToolName, callCtx, inv.Args)
			if err != nil && gCtx.Err() != nil {
				return gCtx.Err()
			}
			slots[i] = slotResult{
				res:      res,
				execErr:  err,
				duration: l.cfg.now().Sub(started),
			}
			return nil
		})
	}

	if err := grp.Wait(); err != nil {
		return nil, nil, err
	}

	blocks := make([]store.ContentBlock, len(batch))
	var touched []string

	for i, inv := range batch {
		sr := slots[i]
		res := sr.res
		if sr.execErr != nil {
			res = tools.ToolResult{
				Output:  config.Redact(sr.execErr.Error(), l.cfg.secrets),
				IsError: true,
			}
		}
		for _, actName := range res.Activated {
			if inv.ToolName == "search_tools" {
				l.activatedTools[actName] = true
			} else if inv.ToolName == "activate_skill" {
				l.activatedSkills[actName] = true
			}
		}
		if res.TouchedPath != "" {
			touched = append(touched, res.TouchedPath)
		}
		l.runPostToolHooks(ctx, step, inv, &res, sr.duration, sr.execErr)
		blocks[i] = store.ContentBlock{
			Type:       "tool_result",
			ToolCallID: inv.ToolCallID,
			ToolName:   inv.ToolName,
			Text:       res.Output,
			IsError:    res.IsError,
		}
	}

	return blocks, touched, nil
}

func (l *AgentLoop) runSerialTool(
	ctx context.Context,
	step int,
	stepSeq uint64,
	inv ToolInvocation,
	cachedHookDec GateResult,
	cachedHookErr error,
	hasCachedHook bool,
) (store.ContentBlock, string, *DeferredToolPayload, error) {
	// 1. 先执行 PreToolUse 钩子与首轮 PermissionGate 拒绝检查（Step 1 Hard-Floor / ACL / Step 2 Plan / Step 3 Unattended Tampered / Step 4 Hook Deny）：
	// 任何应被拒绝的调用立即返回 Permission denied，严禁提前读取磁盘文件或泄露 old_string 是否匹配！
	var (
		hookDec GateResult
		hErr    error
	)
	if hasCachedHook {
		hookDec, hErr = cachedHookDec, cachedHookErr
	} else {
		hookDec, hErr = l.runPreToolHooks(ctx, step, inv)
	}
	if hErr != nil && !hookDec.RequiresAction() {
		hookDec = GateResult{
			Action:     GateDeny,
			ReasonCode: "hook-error",
			Reason:     hErr.Error(),
		}
	}

	gateRes := l.evaluateGate(inv, hookDec)
	if gateRes.Action == GateDeny {
		msg := fmt.Sprintf("Permission denied [%s]: %s", gateRes.ReasonCode, gateRes.Reason)
		return store.ContentBlock{
			Type:       "tool_result",
			ToolCallID: inv.ToolCallID,
			ToolName:   inv.ToolName,
			Text:       msg,
			IsError:    true,
		}, "", nil, nil
	}

	// 2. 通过拒绝预检后，为 edit_file / write_file 计算暂存 DiffHunk，并重新评估 GateResult 以将 Hunk 指纹纳入内容指纹
	if inv.ToolName == "edit_file" || inv.ToolName == "write_file" {
		staged, stageErr := l.tryStageForGate(inv)
		if stageErr != nil {
			errText := config.Redact(stageErr.Error(), l.cfg.secrets)
			return store.ContentBlock{
				Type:       "tool_result",
				ToolCallID: inv.ToolCallID,
				ToolName:   inv.ToolName,
				Text:       errText,
				IsError:    true,
			}, "", nil, nil
		}
		if staged != nil {
			inv.Staged = staged
			if staged.PreconditionHash != "" {
				l.cfg.reg.RecordReadHash(l.cfg.sessionID, staged.AbsPath, staged.PreconditionHash)
			}
			gateRes = l.evaluateGate(inv, hookDec)
		}
	}

	var (
		allowOutside    bool
		selectedHunkIDs []string
	)

	switch gateRes.Action {
	case GateDeny:
		msg := fmt.Sprintf("Permission denied [%s]: %s", gateRes.ReasonCode, gateRes.Reason)
		return store.ContentBlock{
			Type:       "tool_result",
			ToolCallID: inv.ToolCallID,
			ToolName:   inv.ToolName,
			Text:       msg,
			IsError:    true,
		}, "", nil, nil

	case GateDefer:
		var hunks []tools.DiffHunk
		if inv.Staged != nil && len(inv.Staged.Hunks) > 0 {
			hunks = append([]tools.DiffHunk(nil), inv.Staged.Hunks...)
		}
		def := &DeferredToolPayload{
			ID:          fmt.Sprintf("def_%s_%s", l.cfg.runID, inv.ToolCallID),
			RunID:       l.cfg.runID,
			WorkspaceID: l.cfg.ws.ID,
			SessionID:   l.cfg.sessionID,
			ToolCallID:  inv.ToolCallID,
			ToolName:    inv.ToolName,
			ReasonCode:  gateRes.ReasonCode,
			Reason:      gateRes.Reason,
			Fingerprint: gateRes.Fingerprint,
			Args:        inv.Args,
			Hunks:       hunks,
		}
		return store.ContentBlock{}, "", def, nil

	case GateAsk:
		var hunks []tools.DiffHunk
		if inv.Staged != nil && len(inv.Staged.Hunks) > 0 {
			hunks = append([]tools.DiffHunk(nil), inv.Staged.Hunks...)
		}
		surfaced, decCh := l.cfg.broker.Submit(PendingApproval{
			RunID:       l.cfg.runID,
			WorkspaceID: l.cfg.ws.ID,
			SessionID:   l.cfg.sessionID,
			ToolCallID:  inv.ToolCallID,
			ToolName:    inv.ToolName,
			ReasonCode:  gateRes.ReasonCode,
			Reason:      gateRes.Reason,
			Fingerprint: gateRes.Fingerprint,
			Args:        inv.Args,
			Hunks:       hunks,
		})

		if l.cfg.onStepChange != nil {
			l.cfg.onStepChange(step, "waiting_approval")
		}
		if l.cfg.onWaitingApproval != nil {
			l.cfg.onWaitingApproval(true)
		}
		l.emit(EventApprovalRequired, surfaced)

		var dec ApprovalDecision
		var ok bool
		select {
		case dec, ok = <-decCh:
		case <-ctx.Done():
			l.cfg.broker.Cancel(surfaced.ID)
			if l.cfg.onWaitingApproval != nil {
				l.cfg.onWaitingApproval(false)
			}
			return store.ContentBlock{}, "", nil, ctx.Err()
		}

		if l.cfg.onWaitingApproval != nil {
			l.cfg.onWaitingApproval(false)
		}
		if l.cfg.onStepChange != nil {
			l.cfg.onStepChange(step, "running")
		}
		if !ok {
			if ctx.Err() != nil {
				return store.ContentBlock{}, "", nil, ctx.Err()
			}
			return store.ContentBlock{
				Type:       "tool_result",
				ToolCallID: inv.ToolCallID,
				ToolName:   inv.ToolName,
				Text:       "Approval cancelled before resolution.",
				IsError:    true,
			}, "", nil, nil
		}

		l.emit(EventApprovalResolved, ApprovalResolvedPayload{
			ApprovalID:      surfaced.ID,
			ToolCallID:      inv.ToolCallID,
			Action:          dec.Action,
			SelectedHunkIDs: dec.SelectedHunkIDs,
			Remember:        dec.Remember,
		})

		if dec.Action == ApprovalActionDeny {
			reason := "Tool call denied by user."
			if strings.TrimSpace(dec.Feedback) != "" {
				reason += " Feedback: " + strings.TrimSpace(dec.Feedback)
			}
			return store.ContentBlock{
				Type:       "tool_result",
				ToolCallID: inv.ToolCallID,
				ToolName:   inv.ToolName,
				Text:       reason,
				IsError:    true,
			}, "", nil, nil
		}

		allowOutside = true
		if dec.Action == ApprovalActionApproveHunks {
			selectedHunkIDs = dec.SelectedHunkIDs
		}

	case GateAllow:
		allowOutside = true
	}

	started := l.cfg.now()
	callCtx := l.makeToolCallContext(stepSeq, allowOutside, selectedHunkIDs)
	res, execErr := l.cfg.reg.Execute(ctx, inv.ToolName, callCtx, inv.Args)
	if execErr != nil {
		if ctx.Err() != nil {
			return store.ContentBlock{}, "", nil, ctx.Err()
		}
		res = tools.ToolResult{
			Output:  config.Redact(execErr.Error(), l.cfg.secrets),
			IsError: true,
		}
	}

	l.runPostToolHooks(ctx, step, inv, &res, l.cfg.now().Sub(started), execErr)

	return store.ContentBlock{
		Type:       "tool_result",
		ToolCallID: inv.ToolCallID,
		ToolName:   inv.ToolName,
		Text:       res.Output,
		IsError:    res.IsError,
	}, res.TouchedPath, nil, nil
}

func (l *AgentLoop) tryStageForGate(inv ToolInvocation) (*tools.StagedEdit, error) {
	// 先检查路径是否命中 Vault 机密区或工作区外：若命中 Vault 机密，交由 PermissionGate 第 1 步统一返回 hard-floor 拒绝！
	callCtx := l.makeToolCallContext(0, false, nil)
	staged, err := l.cfg.reg.StageEdit(callCtx, inv.ToolName, inv.Args)
	if err == nil {
		return staged, nil
	}
	if errors.Is(err, tools.ErrVaultSecretBlocked) {
		return nil, nil
	}
	if errors.Is(err, tools.ErrPathEscapesWorkspace) {
		// 工作区外正常文件：若是只读 Plan 模式直接交由 Gate 拒绝，否则允许生成预览 DiffHunk 供 500ms 审批卡展示
		if NormalizePermMode(l.cfg.permMode) == PermModePlan {
			return nil, nil
		}
		outsideCtx := l.makeToolCallContext(0, true, nil)
		outStaged, outErr := l.cfg.reg.StageEdit(outsideCtx, inv.ToolName, inv.Args)
		if outErr == nil {
			return outStaged, nil
		}
		if errors.Is(outErr, tools.ErrVaultSecretBlocked) {
			return nil, nil
		}
		return nil, outErr
	}
	return nil, err
}

func (l *AgentLoop) runPreToolHooks(ctx context.Context, step int, inv ToolInvocation) (GateResult, error) {
	hasBus := l.cfg.hooks != nil
	hasCmdHooks := l.cfg.snap != nil && len(l.cfg.snap.Hooks.PreToolUse) > 0
	if !hasBus && !hasCmdHooks {
		return GateResult{}, nil
	}
	ev := &PreToolUseEvent{
		RunID:       l.cfg.runID,
		WorkspaceID: l.cfg.ws.ID,
		SessionID:   l.cfg.sessionID,
		AgentRole:   l.cfg.agentRole,
		Step:        step,
		Call:        inv,
	}
	var chosen GateResult
	if hasBus {
		busDec, err := l.cfg.hooks.RunPreToolUse(ctx, ev)
		if err != nil {
			return busDec, err
		}
		chosen = mergePreToolGateDecision(chosen, busDec)
		if chosen.Action == GateDeny {
			return chosen, nil
		}
	}
	if hasCmdHooks {
		cmdDec, err := ExecutePreToolCommandHooks(ctx, l.cfg.snap.Hooks.PreToolUse, l.cfg.effectiveRoot, l.cfg.secrets, ev)
		if err != nil {
			return cmdDec, err
		}
		chosen = mergePreToolGateDecision(chosen, cmdDec)
	}
	return chosen, nil
}

func (l *AgentLoop) runPostToolHooks(ctx context.Context, step int, inv ToolInvocation, res *tools.ToolResult, dur time.Duration, execErr error) {
	hasBus := l.cfg.hooks != nil
	hasCmdHooks := l.cfg.snap != nil && len(l.cfg.snap.Hooks.PostToolUse) > 0
	if !hasBus && !hasCmdHooks {
		return
	}
	postEv := &PostToolUseEvent{
		RunID:       l.cfg.runID,
		WorkspaceID: l.cfg.ws.ID,
		SessionID:   l.cfg.sessionID,
		AgentRole:   l.cfg.agentRole,
		Step:        step,
		Call:        inv,
		Result:      res,
		Duration:    dur,
		Err:         execErr,
	}
	if hasBus {
		_ = l.cfg.hooks.RunPostToolUse(ctx, postEv)
	}
	if hasCmdHooks {
		_ = ExecutePostToolCommandHooks(ctx, l.cfg.snap.Hooks.PostToolUse, l.cfg.effectiveRoot, l.cfg.secrets, postEv)
	}
}

func (l *AgentLoop) maybeAutoCompact(ctx context.Context, spec config.ModelSpec, inputTokens int) {
	if l.cfg.compactor == nil || spec.ContextWindow <= 0 || inputTokens <= 0 {
		return
	}
	threshold := int(float64(spec.ContextWindow) * AutoCompactionContextRatio)
	if threshold < 1 {
		threshold = 1
	}
	if inputTokens < threshold {
		return
	}
	res, err := l.cfg.compactor.Compact(ctx, l.cfg.sessionID)
	if err == nil && res != nil && !res.Skipped {
		l.emit(EventCompacted, res)
	}
}

func (l *AgentLoop) evaluateGate(inv ToolInvocation, hookDec GateResult) GateResult {
	evalCtx := EvalContext{
		WorkspaceID:       l.cfg.ws.ID,
		WorkspaceRoot:     l.cfg.effectiveRoot,
		VaultRoot:         l.cfg.snap.Paths.Root,
		UserHomeDir:       l.cfg.userHomeDir,
		EffectiveMode:     l.cfg.permMode,
		Unattended:        l.cfg.unattended,
		ConfigTampered:    l.cfg.configTampered,
		TrustedHashes:     l.cfg.ws.TrustedHashes,
		UntrustedWarnings: l.cfg.snap.UntrustedWarnings,
		HookDecision:      hookDec,
		AgentProfile:      l.cfg.profile,
		AllowedTools:      l.cfg.allowedTools,
		Rules:             l.cfg.snap.Rules,
		PathGuard:         l.cfg.pathGuard,
	}
	return l.cfg.gate.Evaluate(evalCtx, inv)
}

func (l *AgentLoop) prepareInitialSessionState(ctx context.Context, branch []store.SessionEntry) error {
	danglingUses := findUnansweredToolUses(branch)

	if l.cfg.resumeApproval != nil {
		resReq := *l.cfg.resumeApproval
		var resultBlocks []store.ContentBlock
		matchedDangling := false

		for _, du := range danglingUses {
			if du.ToolCallID == resReq.ToolCallID {
				matchedDangling = true
				blk, err := l.executeResumedCall(ctx, resReq)
				if err != nil {
					return err
				}
				resultBlocks = append(resultBlocks, blk)
			} else {
				resultBlocks = append(resultBlocks, store.ContentBlock{
					Type:       "tool_result",
					ToolCallID: du.ToolCallID,
					ToolName:   du.ToolName,
					Text:       "Tool call interrupted when sibling call deferred.",
					IsError:    true,
				})
			}
		}

		if len(resultBlocks) > 0 {
			if _, err := l.cfg.sessions.Append(l.cfg.sessionID, store.SessionEntry{
				Role:      "tool",
				AgentRole: l.cfg.agentRole,
				Blocks:    resultBlocks,
				CreatedAt: l.cfg.now().UTC(),
			}); err != nil {
				return err
			}
			for _, rb := range resultBlocks {
				l.emit(EventToolResult, ToolResultPayload{
					Step:       0,
					ToolCallID: rb.ToolCallID,
					ToolName:   rb.ToolName,
					Output:     rb.Text,
					IsError:    rb.IsError,
				})
			}
		}

		if !matchedDangling {
			// 会话在此期间已向前演进（原未配对 tool_use 已被先前回合修复或回退覆盖）：
			// 绝不执行过期的延迟调用，更绝不向历史写入无主孤立 tool_result 破坏协议合法性！
			note := fmt.Sprintf(
				"<system-reminder>\nDeferred tool call %q (id=%s, approved=%v) was superseded by newer session activity and was not executed.\n</system-reminder>",
				resReq.ToolName,
				resReq.ToolCallID,
				resReq.Approved,
			)
			if _, err := l.cfg.sessions.Append(l.cfg.sessionID, store.SessionEntry{
				Role:      "user",
				AgentRole: l.cfg.agentRole,
				Blocks: []store.ContentBlock{
					{Type: "text", Text: note},
				},
				CreatedAt: l.cfg.now().UTC(),
			}); err != nil {
				return err
			}
		}
		return nil
	}

	// 普通运行启动时若上次会话异常中断留下了未配对的 tool_use，自动补齐中断结果保持协议合法
	if len(danglingUses) > 0 {
		repairBlocks := make([]store.ContentBlock, len(danglingUses))
		for i, du := range danglingUses {
			repairBlocks[i] = store.ContentBlock{
				Type:       "tool_result",
				ToolCallID: du.ToolCallID,
				ToolName:   du.ToolName,
				Text:       "Tool execution was interrupted or deferred before completion.",
				IsError:    true,
			}
		}
		if _, err := l.cfg.sessions.Append(l.cfg.sessionID, store.SessionEntry{
			Role:      "tool",
			AgentRole: l.cfg.agentRole,
			Blocks:    repairBlocks,
			CreatedAt: l.cfg.now().UTC(),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (l *AgentLoop) executeResumedCall(ctx context.Context, resume DeferredResume) (store.ContentBlock, error) {
	inv := ToolInvocation{
		ToolCallID: resume.ToolCallID,
		ToolName:   strings.TrimSpace(resume.ToolName),
		Args:       resume.Args,
	}
	if t, ok := l.cfg.reg.Get(inv.ToolName); ok {
		tCopy := t
		inv.Tool = &tCopy
	}

	// 即使从挂起卡片恢复执行，仍必须通过第 1 步 Hard-Floor 与工具 ACL 复核，杜绝伪造卡片越权
	evalCtx := EvalContext{
		WorkspaceID:   l.cfg.ws.ID,
		WorkspaceRoot: l.cfg.effectiveRoot,
		VaultRoot:     l.cfg.snap.Paths.Root,
		UserHomeDir:   l.cfg.userHomeDir,
		EffectiveMode: PermModeYolo,
		AgentProfile:  l.cfg.profile,
		AllowedTools:  l.cfg.allowedTools,
		Rules:         l.cfg.snap.Rules,
		PathGuard:     l.cfg.pathGuard,
	}
	target := extractTargetInfo(evalCtx, l.cfg.pathGuard, inv)
	if reason, blocked := checkHardFloor(evalCtx, inv, target); blocked {
		return store.ContentBlock{
			Type:       "tool_result",
			ToolCallID: inv.ToolCallID,
			ToolName:   inv.ToolName,
			Text:       "Permission denied [hard-floor]: " + reason,
			IsError:    true,
		}, nil
	}
	if reason, code, blocked := checkToolACL(evalCtx, inv); blocked {
		return store.ContentBlock{
			Type:       "tool_result",
			ToolCallID: inv.ToolCallID,
			ToolName:   inv.ToolName,
			Text:       fmt.Sprintf("Permission denied [%s]: %s", code, reason),
			IsError:    true,
		}, nil
	}

	if !resume.Approved {
		msg := "Deferred tool call was denied by user."
		if strings.TrimSpace(resume.Feedback) != "" {
			msg += " Feedback: " + strings.TrimSpace(resume.Feedback)
		}
		return store.ContentBlock{
			Type:       "tool_result",
			ToolCallID: inv.ToolCallID,
			ToolName:   inv.ToolName,
			Text:       msg,
			IsError:    true,
		}, nil
	}

	if inv.ToolName == "edit_file" || inv.ToolName == "write_file" {
		staged, stageErr := l.tryStageForGate(inv)
		if stageErr != nil {
			return store.ContentBlock{
				Type:       "tool_result",
				ToolCallID: inv.ToolCallID,
				ToolName:   inv.ToolName,
				Text:       config.Redact(stageErr.Error(), l.cfg.secrets),
				IsError:    true,
			}, nil
		}
		inv.Staged = staged
	}

	if strings.TrimSpace(resume.Fingerprint) != "" {
		actualFP := ComputeFingerprint(l.cfg.ws.ID, l.cfg.effectiveRoot, inv)
		if actualFP != strings.TrimSpace(resume.Fingerprint) {
			return store.ContentBlock{
				Type:       "tool_result",
				ToolCallID: inv.ToolCallID,
				ToolName:   inv.ToolName,
				Text:       fmt.Sprintf("%v: deferred fingerprint changed before resume (expected %s, got %s)", tools.ErrStalePrecondition, resume.Fingerprint, actualFP),
				IsError:    true,
			}, nil
		}
	}

	if resume.Remember && l.cfg.grants != nil {
		fp := resume.Fingerprint
		if fp == "" {
			fp = ComputeFingerprint(l.cfg.ws.ID, l.cfg.effectiveRoot, inv)
		}
		_ = l.cfg.grants.SaveGrant(l.cfg.ws.ID, fp, inv.ToolName, "approved via deferred card")
	}

	seq, _ := l.cfg.sessions.NextSeq()
	callCtx := l.makeToolCallContext(seq, true, resume.SelectedHunkIDs)
	res, err := l.cfg.reg.Execute(ctx, inv.ToolName, callCtx, inv.Args)
	if err != nil {
		if ctx.Err() != nil {
			return store.ContentBlock{}, ctx.Err()
		}
		res = tools.ToolResult{
			Output:  config.Redact(err.Error(), l.cfg.secrets),
			IsError: true,
		}
	}

	return store.ContentBlock{
		Type:       "tool_result",
		ToolCallID: inv.ToolCallID,
		ToolName:   inv.ToolName,
		Text:       res.Output,
		IsError:    res.IsError,
	}, nil
}

func (l *AgentLoop) makeToolCallContext(seq uint64, allowOutside bool, selectedHunkIDs []string) tools.CallContext {
	sbMode := l.cfg.sandboxMode
	if l.cfg.profile != nil && l.cfg.profile.ReadOnly && sbMode != tools.SandboxOff {
		sbMode = tools.SandboxReadOnly
	}
	return tools.CallContext{
		WorkspaceID:           l.cfg.ws.ID,
		WorkspaceRoot:         l.cfg.effectiveRoot,
		VaultRoot:             l.cfg.snap.Paths.Root,
		SessionID:             l.cfg.sessionID,
		Seq:                   seq,
		SandboxMode:           sbMode,
		AllowedDomains:        l.cfg.snap.Rules.AllowedDomains,
		ExtraWriteDirs:        l.cfg.extraWriteDirs,
		AllowOutsideWorkspace: allowOutside,
		SelectedHunkIDs:       selectedHunkIDs,
		ActivatedTools:        l.activatedTools,
		ActivatedSkills:       l.activatedSkills,
		Snapshots:             l.cfg.snapshots,
		PathGuard:             l.cfg.pathGuard,
		Secrets:               l.cfg.secrets,
	}
}

func (l *AgentLoop) buildStaticSystemPrompt() string {
	var parts []string
	if l.cfg.profile != nil && strings.TrimSpace(l.cfg.profile.SystemPrompt) != "" {
		parts = append(parts, strings.TrimSpace(l.cfg.profile.SystemPrompt))
	}

	if strings.TrimSpace(l.cfg.snap.Paths.Root) != "" {
		instrPath := filepath.Join(l.cfg.snap.Paths.MemoryDir(), "INSTRUCTIONS.md")
		if raw, err := os.ReadFile(instrPath); err == nil {
			if trimmed := strings.TrimSpace(string(raw)); trimmed != "" {
				parts = append(parts, trimmed)
			}
		}
	}

	// 若当前角色未显式将工具收缩为空切片（如 utility allowed_tools = []），将可用技能摘要纳入静态前缀
	hasZeroTools := (l.cfg.profile != nil && l.cfg.profile.AllowedTools != nil && len(l.cfg.profile.AllowedTools) == 0) ||
		(l.cfg.allowedTools != nil && len(l.cfg.allowedTools) == 0)
	if !hasZeroTools && l.cfg.skills != nil {
		if xmlSum := strings.TrimSpace(l.cfg.skills.XMLSummary()); xmlSum != "" {
			parts = append(parts, xmlSum)
		}
	}

	for _, s := range l.cfg.staticReminders {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}

	_, frozen, _ := llm.AssemblePrompt(parts, nil, nil, nil)
	return frozen
}

func (l *AgentLoop) flushQueuedSteers() error {
	_, err := l.flushQueuedSteersCount()
	return err
}

func (l *AgentLoop) flushQueuedSteersCount() (int, error) {
	if l.cfg.drainSteer == nil {
		return 0, nil
	}
	msgs := l.cfg.drainSteer()
	if len(msgs) == 0 {
		return 0, nil
	}

	blocks := make([]store.ContentBlock, 0, len(msgs))
	injected := 0
	for _, m := range msgs {
		trimmed := strings.TrimSpace(m)
		if trimmed == "" {
			continue
		}
		safeSteer := sanitizeSystemReminderContent(trimmed)
		wrapped := "<system-reminder>\nSteer: " + safeSteer + "\n</system-reminder>"
		blocks = append(blocks, store.ContentBlock{
			Type: "text",
			Text: wrapped,
		})
		injected++
		l.emit(EventSteerInjected, SteerInjectedPayload{Message: safeSteer})
	}
	if len(blocks) == 0 {
		return 0, nil
	}

	_, err := l.cfg.sessions.Append(l.cfg.sessionID, store.SessionEntry{
		Role:      "user",
		AgentRole: l.cfg.agentRole,
		Blocks:    blocks,
		CreatedAt: l.cfg.now().UTC(),
	})
	return injected, err
}

func (l *AgentLoop) collectPathSkillReminders(touchedPaths []string) []string {
	if l.cfg.skills == nil || len(touchedPaths) == 0 {
		return nil
	}
	matched := l.cfg.skills.ActiveForPaths(touchedPaths)
	var reminders []string
	for _, sk := range matched {
		if l.activatedSkills[sk.Name] {
			continue
		}
		l.activatedSkills[sk.Name] = true
		safeName := sanitizeSystemReminderContent(sk.Name)
		safeBody := sanitizeSystemReminderContent(sk.Body)
		reminders = append(reminders, fmt.Sprintf("Auto-activated skill %q for path match:\n%s", safeName, safeBody))
	}
	return reminders
}

func (l *AgentLoop) emit(eventType string, payload any) {
	if l.cfg.emit != nil {
		l.cfg.emit(eventType, payload)
	}
}

func filterSchemasForRun(
	schemas []llm.ToolSchema,
	prof *config.AgentProfile,
	runAllowed []string,
) []llm.ToolSchema {
	if prof == nil && runAllowed == nil {
		return schemas
	}
	if (prof != nil && prof.AllowedTools != nil && len(prof.AllowedTools) == 0) ||
		(runAllowed != nil && len(runAllowed) == 0) {
		return nil
	}

	out := make([]llm.ToolSchema, 0, len(schemas))
	for _, sc := range schemas {
		if prof != nil {
			if containsString(prof.DisallowedTools, sc.Name) {
				continue
			}
			if prof.AllowedTools != nil && !containsString(prof.AllowedTools, sc.Name) && sc.Name != "search_tools" {
				continue
			}
			if prof.ReadOnly && (sc.Name == "edit_file" || sc.Name == "write_file" || strings.HasPrefix(sc.Name, "mcp__")) {
				continue
			}
		}
		if runAllowed != nil && !containsString(runAllowed, sc.Name) && sc.Name != "search_tools" {
			continue
		}
		out = append(out, sc)
	}
	return out
}

func findUnansweredToolUses(branch []store.SessionEntry) []store.ContentBlock {
	if len(branch) == 0 {
		return nil
	}
	answered := make(map[string]bool)
	idx := len(branch) - 1
	for idx >= 0 && (branch[idx].Role == "tool" || isToolResultOnlyEntry(branch[idx])) {
		for _, b := range branch[idx].Blocks {
			if b.Type == "tool_result" && b.ToolCallID != "" {
				answered[b.ToolCallID] = true
			}
		}
		idx--
	}
	if idx < 0 || branch[idx].Role != "assistant" {
		return nil
	}
	var out []store.ContentBlock
	for _, b := range branch[idx].Blocks {
		if b.Type == "tool_use" && b.ToolCallID != "" && !answered[b.ToolCallID] {
			out = append(out, b)
		}
	}
	return out
}

func makeSkippedToolResult(inv ToolInvocation, msg string) store.ContentBlock {
	return store.ContentBlock{
		Type:       "tool_result",
		ToolCallID: inv.ToolCallID,
		ToolName:   inv.ToolName,
		Text:       msg,
		IsError:    true,
	}
}

func (l *AgentLoop) rehydrateActivatedState(branch []store.SessionEntry) {
	if len(branch) == 0 {
		return
	}
	for _, ent := range branch {
		for _, blk := range ent.Blocks {
			switch blk.Type {
			case "tool_use":
				name := strings.TrimSpace(blk.ToolName)
				switch name {
				case "":
				case "activate_skill":
					var in struct {
						Name string `json:"name"`
					}
					if json.Unmarshal(blk.ToolInput, &in) == nil && strings.TrimSpace(in.Name) != "" {
						l.activatedSkills[strings.TrimSpace(in.Name)] = true
					}
				case "search_tools":
					if l.cfg.reg != nil {
						var in struct {
							Query string `json:"query"`
							Limit int    `json:"limit"`
						}
						if json.Unmarshal(blk.ToolInput, &in) == nil && strings.TrimSpace(in.Query) != "" {
							for _, m := range l.cfg.reg.SearchBM25(in.Query, in.Limit) {
								l.activatedTools[m.Name] = true
							}
						}
					}
				default:
					l.activatedTools[name] = true
				}
			case "tool_result":
				if blk.ToolName == "search_tools" && l.cfg.reg != nil {
					for _, line := range strings.Split(blk.Text, "\n") {
						trimmed := strings.TrimSpace(line)
						if after, ok := strings.CutPrefix(trimmed, "- "); ok {
							if cand, _, hasColon := strings.Cut(after, ":"); hasColon {
								cand = strings.TrimSpace(cand)
								if _, exists := l.cfg.reg.Get(cand); exists {
									l.activatedTools[cand] = true
								}
							}
						}
					}
				}
			}
		}
	}
}

func resolveRouteSpec(route llm.ResolvedRoute, snap *config.Snapshot, providerID, modelID string) config.ModelSpec {
	cleanProv := strings.TrimSpace(providerID)
	cleanModel := strings.TrimSpace(modelID)
	for cur := &route; cur != nil; cur = cur.Fallback {
		if cur.ModelID == cleanModel && (cleanProv == "" || cur.ProviderID == cleanProv) {
			return cur.Spec
		}
	}
	if snap != nil && cleanProv != "" {
		if prov, ok := snap.Providers[cleanProv]; ok {
			if spec, has := prov.Models[cleanModel]; has {
				return spec
			}
		}
	}
	return route.Spec
}
