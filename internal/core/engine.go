package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/llm"
	"gorness.dev/gorness/internal/store"
	"gorness.dev/gorness/internal/tools"
)

const (
	EventRunStarted       = "run_started"
	EventStepStarted      = "step_started"
	EventStepReset        = "step_reset"
	EventRetry            = "retry"
	EventThinkingDelta    = "thinking_delta"
	EventTextDelta        = "text_delta"
	EventSteerInjected    = "steer_injected"
	EventToolCall         = "tool_call"
	EventToolResult       = "tool_result"
	EventApprovalRequired = "approval_required"
	EventApprovalResolved = "approval_resolved"
	EventToolDeferred     = "tool_deferred"
	EventDoomLoop         = "doom_loop"
	EventTitleUpdated     = "title_updated"
	EventCompacted        = "compacted"
	EventRunCompleted     = "run_completed"
	EventRunFailed        = "run_failed"
	EventRunCancelled     = "run_cancelled"

	defaultSessionTitle = "New Session"
)

var (
	ErrSessionBusy  = errors.New("session already has an active run")
	ErrRunNotFound  = errors.New("active run not found")
	ErrEngineClosed = errors.New("engine is closed")
)

type RunEvent struct {
	Seq         uint64    `json:"seq"`
	Type        string    `json:"type"`
	RunID       string    `json:"run_id"`
	WorkspaceID string    `json:"workspace_id"`
	SessionID   string    `json:"session_id"`
	AgentRole   string    `json:"agent_role"`
	ModelID     string    `json:"model_id"`
	Payload     any       `json:"payload,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

type ToolCallPayload struct {
	Step       int             `json:"step"`
	ToolCallID string          `json:"tool_call_id"`
	ToolName   string          `json:"tool_name"`
	Args       json.RawMessage `json:"args"`
}

type ToolResultPayload struct {
	Step       int    `json:"step"`
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	Output     string `json:"output"`
	IsError    bool   `json:"is_error,omitempty"`
}

type SteerInjectedPayload struct {
	Message string `json:"message"`
}

type ApprovalResolvedPayload struct {
	ApprovalID      string         `json:"approval_id"`
	ToolCallID      string         `json:"tool_call_id"`
	Action          ApprovalAction `json:"action"`
	SelectedHunkIDs []string       `json:"selected_hunk_ids"`
	Remember        bool           `json:"remember,omitempty"`
}

type DeferredToolPayload struct {
	ID          string           `json:"id"`
	RunID       string           `json:"run_id"`
	WorkspaceID string           `json:"workspace_id"`
	SessionID   string           `json:"session_id"`
	ToolCallID  string           `json:"tool_call_id"`
	ToolName    string           `json:"tool_name"`
	ReasonCode  string           `json:"reason_code"`
	Reason      string           `json:"reason"`
	Fingerprint string           `json:"fingerprint"`
	Args        json.RawMessage  `json:"args"`
	Hunks       []tools.DiffHunk `json:"hunks,omitempty"`
}

type DeferredResume struct {
	DeferredID      string          `json:"deferred_id,omitempty"`
	ToolCallID      string          `json:"tool_call_id"`
	ToolName        string          `json:"tool_name"`
	Args            json.RawMessage `json:"args"`
	Fingerprint     string          `json:"fingerprint,omitempty"`
	Approved        bool            `json:"approved"`
	SelectedHunkIDs []string        `json:"selected_hunk_ids"`
	Remember        bool            `json:"remember,omitempty"`
	Feedback        string          `json:"feedback,omitempty"`
}

type DoomLoopPayload struct {
	ToolName            string          `json:"tool_name"`
	Args                json.RawMessage `json:"args"`
	ConsecutiveFailures int             `json:"consecutive_failures"`
	LastError           string          `json:"last_error,omitempty"`
}

type TitleUpdatedPayload struct {
	SessionID string `json:"session_id"`
	Title     string `json:"title"`
}

type RunErrorPayload struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

type RunRequest struct {
	WorkspaceID     string          `json:"workspace_id"`
	SessionID       string          `json:"session_id,omitempty"`
	Prompt          string          `json:"prompt,omitempty"`
	AgentRole       string          `json:"agent_role,omitempty"`
	ModelOverride   string          `json:"model_override,omitempty"`
	PermMode        string          `json:"perm_mode,omitempty"`
	SandboxMode     string          `json:"sandbox_mode,omitempty"`
	WorktreePath    string          `json:"worktree_path,omitempty"`
	ExtraWriteDirs  []string        `json:"extra_write_dirs,omitempty"`
	AllowedTools    []string        `json:"allowed_tools,omitempty"`
	Unattended      bool            `json:"unattended,omitempty"`
	MaxSteps        int             `json:"max_steps,omitempty"`
	StaticReminders []string        `json:"static_reminders,omitempty"`
	DynamicContext  []string        `json:"dynamic_context,omitempty"`
	ResumeApproval  *DeferredResume `json:"resume_approval,omitempty"`
}

type ActiveRunSummary struct {
	RunID        string    `json:"run_id"`
	WorkspaceID  string    `json:"workspace_id"`
	SessionID    string    `json:"session_id"`
	AgentRole    string    `json:"agent_role"`
	ProviderID   string    `json:"provider_id"`
	ModelID      string    `json:"model_id"`
	WorktreePath string    `json:"worktree_path,omitempty"`
	Unattended   bool      `json:"unattended,omitempty"`
	Step         int       `json:"step"`
	Status       string    `json:"status"`
	StartedAt    time.Time `json:"started_at"`
}

type EngineOptions struct {
	Config     *config.Manager
	Workspaces *store.WorkspaceStore
	Sessions   *store.SessionStore
	Snapshots  *store.SnapshotStore
	Router     *llm.Router
	Hooks      *HookBus
	Seatbelt   *tools.SeatbeltRunner
	UserHome   string
	Now        func() time.Time
}

type activeRunState struct {
	summary ActiveRunSummary
	cancel  context.CancelFunc

	mu     sync.Mutex
	steers []string
}

func (r *activeRunState) enqueueSteer(msg string) {
	trimmed := strings.TrimSpace(msg)
	if trimmed == "" {
		return
	}
	r.mu.Lock()
	r.steers = append(r.steers, trimmed)
	r.mu.Unlock()
}

func (r *activeRunState) drainSteers() []string {
	r.mu.Lock()
	if len(r.steers) == 0 {
		r.mu.Unlock()
		return nil
	}
	out := append([]string(nil), r.steers...)
	r.steers = nil
	r.mu.Unlock()
	return out
}

func (r *activeRunState) updateStepStatus(step int, status string) {
	r.mu.Lock()
	r.summary.Step = step
	if status != "" {
		r.summary.Status = status
	}
	r.mu.Unlock()
}

func (r *activeRunState) snapshot() ActiveRunSummary {
	r.mu.Lock()
	cp := r.summary
	r.mu.Unlock()
	return cp
}

// Engine 统一调度跨项目并发运行、单会话非阻塞文件锁互斥（TryFlock）、500ms 审批经纪人、步边界 Steer、锚定压缩与首轮异步自动命名。
type Engine struct {
	cfg        *config.Manager
	workspaces *store.WorkspaceStore
	sessions   *store.SessionStore
	snapshots  *store.SnapshotStore
	router     *llm.Router
	hooks      *HookBus
	grants     *GrantStore
	gate       *PermissionGate
	broker     *ApprovalBroker
	compactor  *Compactor
	seatbelt   *tools.SeatbeltRunner
	userHome   string
	now        func() time.Time

	workerSem chan struct{}
	eventSeq  atomic.Uint64
	runNonce  atomic.Uint64

	runsMu sync.RWMutex
	runs   map[string]*activeRunState
	closed bool

	subsMu sync.RWMutex
	subs   map[uint64]chan RunEvent
	subSeq uint64

	bgCtx    context.Context
	bgCancel context.CancelFunc
	bgWG     sync.WaitGroup
}

func NewEngine(opts EngineOptions) (*Engine, error) {
	if opts.Config == nil {
		return nil, errors.New("core.NewEngine: Config manager is required")
	}

	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	paths := opts.Config.Paths()
	wsStore := opts.Workspaces
	if wsStore == nil {
		var err error
		wsStore, err = store.NewWorkspaceStore(paths)
		if err != nil {
			return nil, err
		}
	}

	sessStore := opts.Sessions
	if sessStore == nil {
		var err error
		sessStore, err = store.NewSessionStore(paths)
		if err != nil {
			return nil, err
		}
	}

	snapStore := opts.Snapshots
	if snapStore == nil {
		var err error
		snapStore, err = store.NewSnapshotStore(paths)
		if err != nil {
			return nil, err
		}
	}

	router := opts.Router
	if router == nil {
		router = llm.NewRouter(opts.Config, opts.Config.Secrets(), nil)
	}

	hooks := opts.Hooks
	if hooks == nil {
		hooks = NewHookBus(nil)
		hooks.Freeze()
	}

	sb := opts.Seatbelt
	if sb == nil {
		sb = tools.NewSeatbeltRunner(tools.WithRunnerVaultRoot(paths.Root))
	}

	grants := NewGrantStore(paths)
	gate := NewPermissionGate(grants)
	broker := NewApprovalBroker(grants, nowFn)
	compactor := NewCompactor(opts.Config, wsStore, sessStore, router, nowFn)

	maxConc := 4
	if snap := opts.Config.Current(); snap != nil && snap.Global.MaxConcurrency > 0 {
		maxConc = snap.Global.MaxConcurrency
	}
	if maxConc < 2 {
		maxConc = 2
	}

	bgCtx, bgCancel := context.WithCancel(context.Background())

	return &Engine{
		cfg:        opts.Config,
		workspaces: wsStore,
		sessions:   sessStore,
		snapshots:  snapStore,
		router:     router,
		hooks:      hooks,
		grants:     grants,
		gate:       gate,
		broker:     broker,
		compactor:  compactor,
		seatbelt:   sb,
		userHome:   opts.UserHome,
		now:        nowFn,
		workerSem:  make(chan struct{}, maxConc),
		runs:       make(map[string]*activeRunState),
		subs:       make(map[uint64]chan RunEvent),
		bgCtx:      bgCtx,
		bgCancel:   bgCancel,
	}, nil
}

func (e *Engine) Config() *config.Manager           { return e.cfg }
func (e *Engine) Workspaces() *store.WorkspaceStore { return e.workspaces }
func (e *Engine) Sessions() *store.SessionStore     { return e.sessions }
func (e *Engine) Snapshots() *store.SnapshotStore   { return e.snapshots }
func (e *Engine) Router() *llm.Router               { return e.router }
func (e *Engine) Hooks() *HookBus                   { return e.hooks }
func (e *Engine) Gate() *PermissionGate             { return e.gate }
func (e *Engine) Grants() *GrantStore               { return e.grants }
func (e *Engine) Broker() *ApprovalBroker           { return e.broker }

// SubscribeGlobal 订阅全项目实时运行事件广播流（供 Workbench 侧栏与全局监控台消费）。
func (e *Engine) SubscribeGlobal() (<-chan RunEvent, func()) {
	ch := make(chan RunEvent, 256)
	e.subsMu.Lock()
	e.subSeq++
	id := e.subSeq
	e.subs[id] = ch
	e.subsMu.Unlock()

	var once sync.Once
	unsub := func() {
		once.Do(func() {
			e.subsMu.Lock()
			if existing, ok := e.subs[id]; ok {
				delete(e.subs, id)
				close(existing)
			}
			e.subsMu.Unlock()
		})
	}
	return ch, unsub
}

func (e *Engine) StartRun(ctx context.Context, req RunRequest) (string, <-chan RunEvent, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	e.runsMu.RLock()
	if e.closed {
		e.runsMu.RUnlock()
		return "", nil, ErrEngineClosed
	}
	e.runsMu.RUnlock()

	if strings.TrimSpace(req.WorkspaceID) == "" {
		return "", nil, fmt.Errorf("%w: workspace_id is required", store.ErrWorkspaceNotFound)
	}
	if strings.TrimSpace(req.Prompt) == "" && req.ResumeApproval == nil {
		return "", nil, errors.New("prompt or resume_approval is required")
	}

	ws, err := e.workspaces.Get(req.WorkspaceID)
	if err != nil {
		return "", nil, err
	}

	var meta *store.SessionMeta
	if strings.TrimSpace(req.SessionID) == "" {
		role := strings.TrimSpace(req.AgentRole)
		if role == "" {
			role = "coder"
		}
		meta, err = e.sessions.Create(ws.ID, defaultSessionTitle, role)
		if err != nil {
			return "", nil, err
		}
	} else {
		meta, err = e.sessions.GetMeta(req.SessionID)
		if err != nil {
			return "", nil, err
		}
		if meta.WorkspaceID != ws.ID {
			return "", nil, fmt.Errorf("session %s belongs to workspace %s, not %s", meta.ID, meta.WorkspaceID, ws.ID)
		}
	}

	// 单会话单写者非阻塞文件锁：同一会话并发重入立即返回 ErrSessionBusy，不同会话与不同工作区完全并行！
	unlockSession, locked, err := store.TryFlock(e.sessions.RunLockPath(meta.ID))
	if err != nil {
		return "", nil, err
	}
	if !locked {
		return "", nil, fmt.Errorf("%w: %s", ErrSessionBusy, meta.ID)
	}

	// 计算有效工作树根目录（支持 Fleet 隔离 Worktree 覆盖，会话记录仍留在原 Workspace 下）
	effectiveRoot := ws.RootPath
	if strings.TrimSpace(meta.WorktreePath) != "" {
		effectiveRoot = strings.TrimSpace(meta.WorktreePath)
	}
	if strings.TrimSpace(req.WorktreePath) != "" {
		effectiveRoot = strings.TrimSpace(req.WorktreePath)
	}

	wsSnap, warnings, err := e.cfg.ForWorkspace(effectiveRoot, ws.TrustedHashes)
	if err != nil {
		unlockSession()
		return "", nil, err
	}
	configTampered := DetectTamperedProjectConfig(effectiveRoot, ws.TrustedHashes, warnings)
	if !configTampered && effectiveRoot != ws.RootPath {
		if _, baseWarnings, bErr := e.cfg.ForWorkspace(ws.RootPath, ws.TrustedHashes); bErr == nil {
			configTampered = DetectTamperedProjectConfig(ws.RootPath, ws.TrustedHashes, baseWarnings)
		}
	}

	agentRole := strings.TrimSpace(req.AgentRole)
	if agentRole == "" {
		agentRole = strings.TrimSpace(meta.AgentRole)
	}
	if agentRole == "" {
		agentRole = "coder"
	}

	runModelOverride := strings.TrimSpace(req.ModelOverride)
	if runModelOverride == "" {
		runModelOverride = strings.TrimSpace(meta.ModelOverride)
	}
	wsRoleModel := ws.RoleModels[agentRole]

	route, err := e.router.Resolve(agentRole, wsRoleModel, runModelOverride, wsSnap)
	if err != nil {
		unlockSession()
		return "", nil, err
	}

	var profilePtr *config.AgentProfile
	if prof, ok := wsSnap.Agents[agentRole]; ok {
		pCopy := prof
		profilePtr = &pCopy
	}

	permMode := wsSnap.Global.PermissionMode
	if strings.TrimSpace(ws.PermMode) != "" {
		permMode = ws.PermMode
	}
	if strings.TrimSpace(req.PermMode) != "" {
		permMode = req.PermMode
	}
	permMode = NormalizePermMode(permMode)

	sbModeRaw := wsSnap.Global.SandboxMode
	if strings.TrimSpace(ws.SandboxMode) != "" {
		sbModeRaw = ws.SandboxMode
	}
	if strings.TrimSpace(req.SandboxMode) != "" {
		sbModeRaw = req.SandboxMode
	}
	sandboxMode := tools.NormalizeSandboxMode(sbModeRaw)

	maxSteps := wsSnap.Global.MaxSteps
	if req.MaxSteps > 0 {
		maxSteps = req.MaxSteps
	}

	pg, err := tools.NewPathGuard(effectiveRoot, wsSnap.Paths.Root)
	if err != nil {
		unlockSession()
		return "", nil, err
	}

	skillCat := tools.DiscoverSkills(wsSnap, effectiveRoot)
	reg, err := tools.NewRegistry(tools.RegistryOptions{
		WorkspaceRoot: effectiveRoot,
		VaultPaths:    wsSnap.Paths,
		Snapshots:     e.snapshots,
		PathGuard:     pg,
		Seatbelt:      e.seatbelt,
		Skills:        skillCat,
	})
	if err != nil {
		unlockSession()
		return "", nil, err
	}

	// 将微内核 HookBus 上动态注册的插件工具同步注入本次运行的工具表
	if e.hooks != nil {
		for _, pt := range e.hooks.PluginTools() {
			reg.Register(pt)
		}
	}

	now := e.now().UTC()
	runID := fmt.Sprintf("run_%d_%d_%s", now.UnixMilli(), e.runNonce.Add(1), randomShortHex(4))

	runCtx, runCancel := context.WithCancel(ctx)
	state := &activeRunState{
		summary: ActiveRunSummary{
			RunID:        runID,
			WorkspaceID:  ws.ID,
			SessionID:    meta.ID,
			AgentRole:    agentRole,
			ProviderID:   route.ProviderID,
			ModelID:      route.ModelID,
			WorktreePath: req.WorktreePath,
			Unattended:   req.Unattended,
			Step:         0,
			Status:       "running",
			StartedAt:    now,
		},
		cancel: runCancel,
	}

	e.runsMu.Lock()
	if e.closed {
		e.runsMu.Unlock()
		runCancel()
		unlockSession()
		return "", nil, ErrEngineClosed
	}
	e.runs[runID] = state
	e.runsMu.Unlock()

	stream := make(chan RunEvent, 256)
	knownSecrets := e.collectKnownSecrets()
	allowedToolsCopy := cloneOptionalStrings(req.AllowedTools)

	e.bgWG.Add(1)
	go func() {
		defer e.bgWG.Done()
		defer close(stream)
		defer func() {
			e.broker.CancelRun(runID)
			e.runsMu.Lock()
			delete(e.runs, runID)
			e.runsMu.Unlock()
			runCancel()
			unlockSession()
		}()

		emit := func(evType string, payload any) {
			ev := RunEvent{
				Seq:         e.eventSeq.Add(1),
				Type:        evType,
				RunID:       runID,
				WorkspaceID: ws.ID,
				SessionID:   meta.ID,
				AgentRole:   agentRole,
				ModelID:     route.ModelID,
				Payload:     payload,
				Timestamp:   e.now().UTC(),
			}
			sendRunEvent(runCtx, stream, ev)
			e.broadcastGlobal(ev)
		}

		// 获取全局并发工作槽位（若在排队等待时被取消则立即退出）
		hasSlot := false
		acquireSlot := func() bool {
			if hasSlot {
				return true
			}
			select {
			case e.workerSem <- struct{}{}:
				hasSlot = true
				return true
			case <-runCtx.Done():
				return false
			}
		}
		releaseSlot := func() {
			if hasSlot {
				<-e.workerSem
				hasSlot = false
			}
		}
		defer releaseSlot()

		if !acquireSlot() {
			emit(EventRunCancelled, map[string]any{"reason": "cancelled while awaiting worker slot"})
			return
		}

	// 按需连接已授信的 MCP 服务器并在运行结束时回收子进程组（单个服务器失败不阻断其余健康服务器的工具挂载）
		var mcpMgr *tools.MCPManager
		if len(wsSnap.MCP.Servers) > 0 {
			mcpMgr = tools.NewMCPManager(e.cfg.Secrets(), wsSnap.Paths.MCPFile(), effectiveRoot)
			defer func() { _ = mcpMgr.Close() }()
			_ = mcpMgr.ConnectAll(runCtx, wsSnap.MCP)
			mcpMgr.RegisterInto(reg)
		}

		emit(EventRunStarted, map[string]any{
			"session_id":    meta.ID,
			"workspace_id":  ws.ID,
			"agent_role":    agentRole,
			"provider_id":   route.ProviderID,
			"model_id":      route.ModelID,
			"perm_mode":     permMode,
			"sandbox_mode":  sandboxMode,
			"worktree_path": req.WorktreePath,
			"unattended":    req.Unattended,
		})

		loop := newAgentLoop(agentLoopConfig{
			runID:              runID,
			ws:                 ws,
			sessionID:          meta.ID,
			effectiveRoot:      effectiveRoot,
			userHomeDir:        e.userHome,
			agentRole:          agentRole,
			prompt:             req.Prompt,
			route:              route,
			snap:               wsSnap,
			profile:            profilePtr,
			permMode:           permMode,
			sandboxMode:        sandboxMode,
			unattended:         req.Unattended,
			maxSteps:           maxSteps,
			maxToolConcurrency: wsSnap.Global.MaxToolConcurrency,
			allowedTools:       allowedToolsCopy,
			extraWriteDirs:     append([]string(nil), req.ExtraWriteDirs...),
			staticReminders:    append([]string(nil), req.StaticReminders...),
			initialDynamic:     append([]string(nil), req.DynamicContext...),
			resumeApproval:     cloneDeferredResume(req.ResumeApproval),
			configTampered:     configTampered,
			reg:                reg,
			pathGuard:          pg,
			skills:             skillCat,
			sessions:           e.sessions,
			snapshots:          e.snapshots,
			router:             e.router,
			hooks:              e.hooks,
			gate:               e.gate,
			broker:             e.broker,
			grants:             e.grants,
			secrets:            knownSecrets,
			now:                e.now,
			emit:               emit,
			drainSteer:         state.drainSteers,
			onStepChange:       state.updateStepStatus,
			onWaitingApproval: func(waiting bool) {
				if waiting {
					releaseSlot()
				} else {
					_ = acquireSlot()
				}
			},
		})

		outcome, runErr := loop.Run(runCtx)

		switch {
		case outcome.Status == "deferred":
			emit(EventRunCompleted, outcome)
			return

		case errors.Is(runErr, ErrDoomLoop):
			emit(EventRunFailed, RunErrorPayload{
				Code:    "doom_loop",
				Message: ErrDoomLoop.Error(),
			})
			return

		case errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) || runCtx.Err() != nil:
			emit(EventRunCancelled, map[string]any{
				"steps": outcome.Steps,
			})
			return

		case runErr != nil:
			emit(EventRunFailed, RunErrorPayload{
				Code:    "run_error",
				Message: config.Redact(runErr.Error(), knownSecrets),
			})
			return
		}

		emit(EventRunCompleted, outcome)

		// 首轮对话顺利完成后，异步调度轻量 utility 角色为仍处于默认标题的会话生成精炼标题
		if outcome.CompletedTurn1 && strings.TrimSpace(req.Prompt) != "" {
			if curMeta, mErr := e.sessions.GetMeta(meta.ID); mErr == nil && curMeta.Title == defaultSessionTitle {
				e.spawnAutoTitler(ws, curMeta.ID, req.Prompt, outcome.FinalText)
			}
		}
	}()

	return runID, stream, nil
}

func (e *Engine) Steer(runID, message string) error {
	e.runsMu.RLock()
	st, ok := e.runs[strings.TrimSpace(runID)]
	e.runsMu.RUnlock()
	if !ok {
		return fmt.Errorf("%w: %s", ErrRunNotFound, runID)
	}
	st.enqueueSteer(message)
	return nil
}

func (e *Engine) StopRun(runID string) error {
	target := strings.TrimSpace(runID)
	e.runsMu.RLock()
	st, ok := e.runs[target]
	e.runsMu.RUnlock()
	if !ok {
		return fmt.Errorf("%w: %s", ErrRunNotFound, target)
	}
	e.broker.CancelRun(target)
	st.cancel()
	return nil
}

func (e *Engine) ActiveRuns() []ActiveRunSummary {
	e.runsMu.RLock()
	out := make([]ActiveRunSummary, 0, len(e.runs))
	for _, st := range e.runs {
		out = append(out, st.snapshot())
	}
	e.runsMu.RUnlock()

	sort.Slice(out, func(i, j int) bool {
		if !out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].StartedAt.Before(out[j].StartedAt)
		}
		return out[i].RunID < out[j].RunID
	})
	return out
}

func (e *Engine) CompactSession(ctx context.Context, sessionID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	meta, err := e.sessions.GetMeta(sessionID)
	if err != nil {
		return err
	}

	unlock, ok, err := store.TryFlock(e.sessions.RunLockPath(meta.ID))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: %s", ErrSessionBusy, meta.ID)
	}
	defer unlock()

	res, err := e.compactor.Compact(ctx, meta.ID)
	if err != nil {
		return err
	}

	e.broadcastGlobal(RunEvent{
		Seq:         e.eventSeq.Add(1),
		Type:        EventCompacted,
		WorkspaceID: meta.WorkspaceID,
		SessionID:   meta.ID,
		AgentRole:   "utility",
		Payload:     res,
		Timestamp:   e.now().UTC(),
	})
	return nil
}

// WaitBackground 等待全部后台协程（含运行协程与异步 utility 自动命名协程）排空完成。
func (e *Engine) WaitBackground() {
	if e == nil {
		return
	}
	e.bgWG.Wait()
}

func (e *Engine) Close() error {
	if e == nil {
		return nil
	}
	e.runsMu.Lock()
	if e.closed {
		e.runsMu.Unlock()
		return nil
	}
	e.closed = true
	cancels := make([]context.CancelFunc, 0, len(e.runs))
	for _, st := range e.runs {
		cancels = append(cancels, st.cancel)
	}
	e.runsMu.Unlock()

	for _, c := range cancels {
		c()
	}
	e.bgCancel()
	e.bgWG.Wait()

	e.subsMu.Lock()
	for id, ch := range e.subs {
		delete(e.subs, id)
		close(ch)
	}
	e.subsMu.Unlock()

	if e.hooks != nil {
		return e.hooks.Close()
	}
	return nil
}

func (e *Engine) spawnAutoTitler(ws *store.Workspace, sessionID, userPrompt, assistantReply string) {
	e.bgWG.Add(1)
	go func() {
		defer e.bgWG.Done()

		ctx, cancel := context.WithTimeout(e.bgCtx, 8*time.Second)
		defer cancel()

		snap := e.cfg.Current()
		var wsRoleModel string
		if ws != nil {
			wsRoleModel = ws.RoleModels["utility"]
			if wsSnap, _, err := e.cfg.ForWorkspace(ws.RootPath, ws.TrustedHashes); err == nil && wsSnap != nil {
				snap = wsSnap
			}
		}

		route, err := e.router.Resolve("utility", wsRoleModel, "", snap)
		if err != nil {
			return
		}

		staticSys := "You are the Housekeeping Utility agent in Gorness. Produce crisp session titles and faithful anchored summaries preserving exact file paths, decisions, and unresolved tasks."
		if snap != nil {
			if prof, ok := snap.Agents["utility"]; ok && strings.TrimSpace(prof.SystemPrompt) != "" {
				staticSys = prof.SystemPrompt
			}
		}

		replyPreview := assistantReply
		if len(replyPreview) > 400 {
			replyPreview = replyPreview[:400]
		}
		prompt := fmt.Sprintf("Generate a concise 3-6 word session title (no quotes, no trailing punctuation) for:\nUser: %s\nAssistant: %s", userPrompt, replyPreview)

		ch, err := e.router.StreamWithRetry(ctx, route, llm.Request{
			StaticSystem: staticSys,
			History: []store.SessionEntry{
				{
					Seq:    1,
					ID:     "title_req",
					Role:   "user",
					Blocks: []store.ContentBlock{{Type: "text", Text: prompt}},
				},
			},
		})
		if err != nil {
			return
		}

		var buf strings.Builder
		for ev := range ch {
			switch ev.Type {
			case llm.StreamStepReset:
				buf.Reset()
			case llm.StreamTextDelta:
				buf.WriteString(ev.Text)
			case llm.StreamError:
				return
			}
		}

		title := sanitizeGeneratedTitle(buf.String())
		if title == "" {
			return
		}

		var applied bool
		_, err = e.sessions.UpdateMeta(sessionID, func(m *store.SessionMeta) {
			if m.Title == defaultSessionTitle || strings.TrimSpace(m.Title) == "" {
				m.Title = title
				applied = true
			}
		})
		if err == nil && applied {
			wsID := ""
			if ws != nil {
				wsID = ws.ID
			}
			e.broadcastGlobal(RunEvent{
				Seq:         e.eventSeq.Add(1),
				Type:        EventTitleUpdated,
				WorkspaceID: wsID,
				SessionID:   sessionID,
				AgentRole:   "utility",
				ModelID:     route.ModelID,
				Payload: TitleUpdatedPayload{
					SessionID: sessionID,
					Title:     title,
				},
				Timestamp: e.now().UTC(),
			})
		}
	}()
}

func sanitizeGeneratedTitle(raw string) string {
	cleaned := config.EscapeBidi(strings.TrimSpace(raw))
	if idx := strings.IndexAny(cleaned, "\r\n"); idx >= 0 {
		cleaned = cleaned[:idx]
	}
	cleaned = strings.TrimSpace(strings.TrimLeft(cleaned, "#*- "))
	cleaned = strings.Trim(cleaned, "\"'`“”‘’ ")
	runes := []rune(cleaned)
	if len(runes) > 64 {
		cleaned = strings.TrimSpace(string(runes[:64]))
	}
	return cleaned
}

func (e *Engine) broadcastGlobal(ev RunEvent) {
	e.subsMu.RLock()
	for _, ch := range e.subs {
		select {
		case ch <- ev:
		default:
		}
	}
	e.subsMu.RUnlock()
}

func (e *Engine) collectKnownSecrets() []string {
	if e.cfg == nil || e.cfg.Secrets() == nil {
		return nil
	}
	if ks, ok := e.cfg.Secrets().(interface{ KnownSecrets() []string }); ok {
		return ks.KnownSecrets()
	}
	return nil
}

func sendRunEvent(ctx context.Context, ch chan<- RunEvent, ev RunEvent) {
	select {
	case ch <- ev:
		return
	default:
	}
	// 对高频流式增量块，若运行已取消且缓冲区已满则立即丢弃；对状态终态事件优先投递但不无限死锁
	isDelta := ev.Type == EventTextDelta || ev.Type == EventThinkingDelta
	if isDelta && ctx.Err() != nil {
		return
	}
	select {
	case ch <- ev:
	case <-ctx.Done():
		select {
		case ch <- ev:
		default:
		}
	}
}

func cloneOptionalStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneDeferredResume(in *DeferredResume) *DeferredResume {
	if in == nil {
		return nil
	}
	cp := *in
	if in.Args != nil {
		cp.Args = append(json.RawMessage(nil), in.Args...)
	}
	if in.SelectedHunkIDs != nil {
		cp.SelectedHunkIDs = make([]string, len(in.SelectedHunkIDs))
		copy(cp.SelectedHunkIDs, in.SelectedHunkIDs)
	}
	return &cp
}
