package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
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

type EventType = string
type PluginTool = tools.Tool

const (
	EventRunStarted        = "run_started"
	EventStepStarted       = "step_started"
	EventStepReset         = "step_reset"
	EventRetry             = "retry"
	EventThinkingDelta     = "thinking_delta"
	EventTextDelta         = "text_delta"
	EventSteerInjected     = "steer_injected"
	EventToolCall          = "tool_call"
	EventToolResult        = "tool_result"
	EventApprovalRequired  = "approval_required"
	EventApprovalResolved  = "approval_resolved"
	EventToolDeferred      = "tool_deferred"
	EventDoomLoop          = "doom_loop"
	EventTitleUpdated      = "title_updated"
	EventCompacted         = "compacted"
	EventSubagentStarted   = "subagent_started"
	EventSubagentCompleted = "subagent_completed"
	EventRunCompleted      = "run_completed"
	EventRunFailed         = "run_failed"
	EventRunCancelled      = "run_cancelled"

	EventConfigReloaded   = "config_reloaded"
	EventConfigDiagnostic = "config_diagnostic"
	EventSessionUpdated   = "session_updated"
	EventAlwaysOnUpdated  = "alwayson_updated"
	EventFleetUpdated     = "fleet_updated"

	defaultSessionTitle = "New Session"
	maxRunReplayEvents  = 4096
	maxRecentRunReplays = 64
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
	ExtraTools      []PluginTool    `json:"-"`
	Unattended      bool            `json:"unattended,omitempty"`
	MaxSteps        int             `json:"max_steps,omitempty"`
	StaticReminders []string        `json:"static_reminders,omitempty"`
	DynamicContext  []string        `json:"dynamic_context,omitempty"`
	ResumeApproval  *DeferredResume `json:"resume_approval,omitempty"`
}

type RunLogRecord struct {
	RunID           string    `json:"run_id"`
	WorkspaceID     string    `json:"workspace_id"`
	SessionID       string    `json:"session_id"`
	AgentRole       string    `json:"agent_role"`
	ProviderID      string    `json:"provider_id"`
	ModelID         string    `json:"model_id"`
	Status          string    `json:"status"`
	Steps           int       `json:"steps"`
	InputTokens     int       `json:"input_tokens"`
	OutputTokens    int       `json:"output_tokens"`
	CacheReadTokens int       `json:"cache_read_tokens,omitempty"`
	CostUSD         float64   `json:"cost_usd"`
	StartedAt       time.Time `json:"started_at"`
	CompletedAt     time.Time `json:"completed_at"`
	DurationMs      int64     `json:"duration_ms"`
	PromptPreview   string    `json:"prompt_preview,omitempty"`
	Error           string    `json:"error,omitempty"`
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
	Artifacts  *store.ArtifactStore
	Router     *llm.Router
	Hooks      *HookBus
	Seatbelt   *tools.SeatbeltRunner
	UserHome   string
	Now        func() time.Time
}

type activeRunState struct {
	summary ActiveRunSummary
	cancel  context.CancelFunc

	mu       sync.Mutex
	steers   []string
	history  []RunEvent
	subs     map[uint64]chan RunEvent
	subSeq   uint64
	finished bool
}

func (r *activeRunState) recordAndFanout(ctx context.Context, ev RunEvent) {
	r.mu.Lock()
	if len(r.history) < maxRunReplayEvents {
		r.history = append(r.history, ev)
	} else {
		// Evict oldest delta event if present so lifecycle milestones remain intact.
		evicted := false
		for i, prev := range r.history {
			if prev.Type == EventTextDelta || prev.Type == EventThinkingDelta {
				copy(r.history[i:], r.history[i+1:])
				r.history[len(r.history)-1] = ev
				evicted = true
				break
			}
		}
		if !evicted {
			copy(r.history, r.history[1:])
			r.history[len(r.history)-1] = ev
		}
	}
	if !r.finished && len(r.subs) > 0 {
		for _, subCh := range r.subs {
			select {
			case subCh <- ev:
			default:
				if ev.Type != EventTextDelta && ev.Type != EventThinkingDelta && ctx.Err() == nil {
					select {
					case subCh <- ev:
					default:
					}
				}
			}
		}
	}
	r.mu.Unlock()
}

func (r *activeRunState) finishAndSnapshot() []RunEvent {
	r.mu.Lock()
	r.finished = true
	hist := append([]RunEvent(nil), r.history...)
	for id, ch := range r.subs {
		delete(r.subs, id)
		close(ch)
	}
	r.mu.Unlock()
	return hist
}

func (r *activeRunState) subscribe() ([]RunEvent, <-chan RunEvent, func()) {
	r.mu.Lock()
	replay := append([]RunEvent(nil), r.history...)
	if r.finished {
		r.mu.Unlock()
		closedCh := make(chan RunEvent)
		close(closedCh)
		return replay, closedCh, func() {}
	}
	if r.subs == nil {
		r.subs = make(map[uint64]chan RunEvent)
	}
	ch := make(chan RunEvent, 256)
	r.subSeq++
	subID := r.subSeq
	r.subs[subID] = ch
	r.mu.Unlock()

	var once sync.Once
	unsub := func() {
		once.Do(func() {
			r.mu.Lock()
			if existing, ok := r.subs[subID]; ok {
				delete(r.subs, subID)
				close(existing)
			}
			r.mu.Unlock()
		})
	}
	return replay, ch, unsub
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

type pooledMCP struct {
	key         string
	fingerprint string
	mgr         *tools.MCPManager
	refs        int
	evicted     bool
}

// Engine 统一调度跨项目并发运行、单会话非阻塞文件锁互斥（TryFlock）、500ms 审批经纪人、步边界 Steer、锚定压缩与首轮异步自动命名。
type Engine struct {
	cfg        *config.Manager
	workspaces *store.WorkspaceStore
	sessions   *store.SessionStore
	snapshots  *store.SnapshotStore
	artifacts  *store.ArtifactStore
	router     *llm.Router
	hooks      *HookBus
	grants     *GrantStore
	gate       *PermissionGate
	broker     *ApprovalBroker
	compactor  *Compactor
	seatbelt   *tools.SeatbeltRunner
	readHashes *tools.ReadHashTracker
	userHome   string
	now        func() time.Time

	workerSem chan struct{}
	eventSeq  atomic.Uint64
	runNonce  atomic.Uint64

	runsMu      sync.RWMutex
	runs        map[string]*activeRunState
	recentRuns  map[string][]RunEvent
	recentOrder []string
	closed      bool

	subsMu sync.RWMutex
	subs   map[uint64]chan RunEvent
	subSeq uint64

	pluginMu    sync.RWMutex
	pluginTools []PluginTool

	sessionHeadsMu sync.Mutex
	sessionHeads   map[string]string

	mcpMu       sync.Mutex
	mcpPool     map[string]*pooledMCP
	reloadUnsub func()

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

	artStore := opts.Artifacts
	if artStore == nil {
		var err error
		artStore, err = store.NewArtifactStore(paths)
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

	eng := &Engine{
		cfg:          opts.Config,
		workspaces:   wsStore,
		sessions:     sessStore,
		snapshots:    snapStore,
		artifacts:    artStore,
		router:       router,
		hooks:        hooks,
		grants:       grants,
		gate:         gate,
		broker:       broker,
		compactor:    compactor,
		seatbelt:     sb,
		readHashes:   tools.NewReadHashTracker(),
		userHome:     opts.UserHome,
		now:          nowFn,
		workerSem:    make(chan struct{}, maxConc),
		runs:         make(map[string]*activeRunState),
		recentRuns:   make(map[string][]RunEvent),
		subs:         make(map[uint64]chan RunEvent),
		sessionHeads: make(map[string]string),
		mcpPool:      make(map[string]*pooledMCP),
		bgCtx:        bgCtx,
		bgCancel:     bgCancel,
	}

	eng.reloadUnsub = opts.Config.OnReload(func(snap *config.Snapshot) {
		eng.evictAllMCP()
		eng.PublishSystemEvent(EventConfigReloaded, map[string]any{
			"timestamp": eng.now().UTC(),
		})
	})

	return eng, nil
}

func (e *Engine) Config() *config.Manager            { return e.cfg }
func (e *Engine) Workspaces() *store.WorkspaceStore  { return e.workspaces }
func (e *Engine) Sessions() *store.SessionStore      { return e.sessions }
func (e *Engine) Snapshots() *store.SnapshotStore    { return e.snapshots }
func (e *Engine) Artifacts() *store.ArtifactStore    { return e.artifacts }
func (e *Engine) Router() *llm.Router                { return e.router }
func (e *Engine) Hooks() *HookBus                    { return e.hooks }
func (e *Engine) Gate() *PermissionGate              { return e.gate }
func (e *Engine) Grants() *GrantStore                { return e.grants }
func (e *Engine) Broker() *ApprovalBroker            { return e.broker }
func (e *Engine) ReadHashes() *tools.ReadHashTracker { return e.readHashes }

// RegisterPluginTools 动态注册全局常驻插件工具（供 Always-On MemoryStore / Fleet 等子系统在 Engine 初始化后挂载）。
func (e *Engine) RegisterPluginTools(defs ...PluginTool) {
	if e == nil || len(defs) == 0 {
		return
	}
	e.pluginMu.Lock()
	defer e.pluginMu.Unlock()
	for _, d := range defs {
		name := strings.TrimSpace(d.Name)
		if name == "" {
			continue
		}
		d.Name = name
		replaced := false
		for i := range e.pluginTools {
			if e.pluginTools[i].Name == name {
				e.pluginTools[i] = d
				replaced = true
				break
			}
		}
		if !replaced {
			e.pluginTools = append(e.pluginTools, d)
		}
	}
}

func (e *Engine) UnregisterPluginTools(names ...string) {
	if e == nil || len(names) == 0 {
		return
	}
	drop := make(map[string]bool, len(names))
	for _, n := range names {
		if trimmed := strings.TrimSpace(n); trimmed != "" {
			drop[trimmed] = true
		}
	}
	if len(drop) == 0 {
		return
	}
	e.pluginMu.Lock()
	filtered := e.pluginTools[:0]
	for _, pt := range e.pluginTools {
		if !drop[pt.Name] {
			filtered = append(filtered, pt)
		}
	}
	e.pluginTools = filtered
	e.pluginMu.Unlock()
}

func (e *Engine) snapshotPluginTools() []PluginTool {
	e.pluginMu.RLock()
	out := make([]PluginTool, len(e.pluginTools))
	copy(out, e.pluginTools)
	e.pluginMu.RUnlock()
	return out
}

// SubscribeRun 重放指定运行的历史事件缓冲并返回实时事件订阅流（支持页面刷新重连与多终端并发旁听）。
func (e *Engine) SubscribeRun(runID string) ([]RunEvent, <-chan RunEvent, func(), error) {
	target := strings.TrimSpace(runID)
	if target == "" {
		return nil, nil, nil, fmt.Errorf("%w: empty run_id", ErrRunNotFound)
	}

	e.runsMu.RLock()
	st, active := e.runs[target]
	if !active {
		if recent, ok := e.recentRuns[target]; ok {
			replay := append([]RunEvent(nil), recent...)
			e.runsMu.RUnlock()
			closedCh := make(chan RunEvent)
			close(closedCh)
			return replay, closedCh, func() {}, nil
		}
		e.runsMu.RUnlock()
		return nil, nil, nil, fmt.Errorf("%w: %s", ErrRunNotFound, target)
	}
	e.runsMu.RUnlock()

	replay, ch, unsub := st.subscribe()
	return replay, ch, unsub, nil
}

// PublishSystemEvent 向全局事件总线广播系统状态事件（如 config_reloaded, config_diagnostic, session_updated, alwayson_updated, fleet_updated）。
func (e *Engine) PublishSystemEvent(evType EventType, payload any) {
	if e == nil {
		return
	}
	cleanType := strings.TrimSpace(evType)
	if cleanType == "" {
		return
	}
	ev := RunEvent{
		Seq:       e.eventSeq.Add(1),
		Type:      cleanType,
		Payload:   payload,
		Timestamp: e.now().UTC(),
	}
	e.broadcastGlobal(ev)
}

// RunUtilityPrompt 使用指定角色（默认 utility）执行无工具单轮轻量提示词并返回精炼文本结果（供 Compactor、AutoTitler、Dreamer 等共享）。
func (e *Engine) RunUtilityPrompt(ctx context.Context, role, system, user string) (string, error) {
	if e == nil {
		return "", ErrEngineClosed
	}
	res, err := executeUtilityPrompt(ctx, e.cfg, e.workspaces, e.router, "", role, system, user)
	if err != nil {
		return "", err
	}
	return res.Text, nil
}

// TrustWorkspace 计算工作区 .gorness/ 与 .mcp.json 的当前 SHA-256 指纹集合并写入 config.toml 授信记录。
func (e *Engine) TrustWorkspace(wsID string) (map[string]string, error) {
	if e == nil {
		return nil, ErrEngineClosed
	}
	ws, err := e.workspaces.Get(strings.TrimSpace(wsID))
	if err != nil {
		return nil, err
	}
	hashes, err := config.ComputeWorkspaceTrustHashes(ws.RootPath)
	if err != nil {
		return nil, err
	}
	copied := make(map[string]string, len(hashes))
	for k, v := range hashes {
		copied[k] = v
	}
	if _, err := e.workspaces.Update(ws.ID, func(w *store.Workspace) {
		w.TrustedHashes = copied
	}); err != nil {
		return nil, err
	}
	e.evictWorkspaceMCP(ws.ID)
	out := make(map[string]string, len(copied))
	for k, v := range copied {
		out[k] = v
	}
	return out, nil
}

// InvalidateSessionReadHashes 显式清空指定会话跨轮次累积的已读文件哈希缓存（例如在 /rewind 回滚后调用）。
func (e *Engine) InvalidateSessionReadHashes(sessionID string) {
	if e == nil || e.readHashes == nil {
		return
	}
	trimmed := strings.TrimSpace(sessionID)
	e.readHashes.InvalidateSession(trimmed)
	e.sessionHeadsMu.Lock()
	delete(e.sessionHeads, trimmed)
	e.sessionHeadsMu.Unlock()
}

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

	// 若会话头指针自上次运行结束后发生了外部回滚（如 /rewind 或分支切换），立即失效跨轮次 readHashes 防止陈旧哈希绕过预读检查
	e.syncSessionReadHashesBeforeRun(meta)

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
		WorkspaceID:              ws.ID,
		WorkspaceRoot:            effectiveRoot,
		VaultPaths:               wsSnap.Paths,
		Snapshots:                e.snapshots,
		Artifacts:                e.artifacts,
		PathGuard:                pg,
		Seatbelt:                 e.seatbelt,
		Skills:                   skillCat,
		ReadHashes:               e.readHashes,
		DefaultCommandTimeoutSec: wsSnap.Global.CommandTimeoutSeconds,
	})
	if err != nil {
		unlockSession()
		return "", nil, err
	}

	// 将微内核 HookBus、Engine 常驻插件工具与本次运行专属附加工具同步注入工具表
	if e.hooks != nil {
		for _, pt := range e.hooks.PluginTools() {
			reg.Register(pt)
		}
	}
	for _, pt := range e.snapshotPluginTools() {
		reg.Register(pt)
	}
	for _, pt := range req.ExtraTools {
		if strings.TrimSpace(pt.Name) != "" {
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
		subs:   make(map[uint64]chan RunEvent),
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
	extraWriteDirsCopy := append([]string(nil), req.ExtraWriteDirs...)

	e.bgWG.Add(1)
	go func() {
		defer e.bgWG.Done()
		defer close(stream)
		defer func() {
			e.recordSessionHeadAfterRun(meta.ID)
			finalHistory := state.finishAndSnapshot()
			e.broker.CancelRun(runID)
			e.runsMu.Lock()
			delete(e.runs, runID)
			e.storeRecentRunReplayLocked(runID, finalHistory)
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
			state.recordAndFanout(runCtx, ev)
			sendRunEvent(runCtx, stream, ev)
			e.broadcastGlobal(ev)
		}

		// 为非子代理角色挂载有界 spawn_subagent 工具（explorer/verifier/debugger/utility 禁止递归派生子代理）
		if !isSubagentOrUtilityRole(agentRole) {
			if _, exists := reg.Get("spawn_subagent"); !exists {
				reg.Register(e.makeSpawnSubagentTool(subagentParentContext{
					parentRunID:          runID,
					ws:                   ws,
					parentSessionID:      meta.ID,
					effectiveRoot:        effectiveRoot,
					wsSnap:               wsSnap,
					configTampered:       configTampered,
					parentReadOnly:       profilePtr != nil && profilePtr.ReadOnly,
					parentPermMode:       permMode,
					parentSandbox:        sandboxMode,
					parentUnattended:     req.Unattended,
					parentAllowedTools:   allowedToolsCopy,
					parentExtraWriteDirs: extraWriteDirsCopy,
					pathGuard:            pg,
					skills:               skillCat,
					secrets:              knownSecrets,
					emit:                 emit,
				}))
			}
		}

		writeSummaryLog := func(status string, outcome RunOutcome, errMsg string) {
			completedAt := e.now().UTC()
			dur := completedAt.Sub(now).Milliseconds()
			if dur < 0 {
				dur = 0
			}
			e.appendRunLog(RunLogRecord{
				RunID:           runID,
				WorkspaceID:     ws.ID,
				SessionID:       meta.ID,
				AgentRole:       agentRole,
				ProviderID:      route.ProviderID,
				ModelID:         route.ModelID,
				Status:          status,
				Steps:           outcome.Steps,
				InputTokens:     outcome.InputTokens,
				OutputTokens:    outcome.OutputTokens,
				CacheReadTokens: outcome.CacheReadTokens,
				CostUSD:         outcome.CostUSD,
				StartedAt:       now,
				CompletedAt:     completedAt,
				DurationMs:      dur,
				PromptPreview:   req.Prompt,
				Error:           errMsg,
			}, knownSecrets)
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
			writeSummaryLog("cancelled", RunOutcome{Status: "cancelled"}, "cancelled while awaiting worker slot")
			emit(EventRunCancelled, map[string]any{"reason": "cancelled while awaiting worker slot"})
			return
		}

		// 复用或按需连接已授信的温热 MCPManager 池（配置变更或引擎关闭时统一回收，单个服务器失败不阻断其余健康服务器的工具挂载）
		if len(wsSnap.MCP.Servers) > 0 {
			mcpMgr, releaseMCP := e.acquireMCPManager(runCtx, ws.ID, effectiveRoot, wsSnap.Paths.MCPFile(), wsSnap.MCP)
			if releaseMCP != nil {
				defer releaseMCP()
			}
			if mcpMgr != nil {
				mcpMgr.RegisterInto(reg)
			}
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
			extraWriteDirs:     extraWriteDirsCopy,
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
			compactor:          e.compactor,
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
			writeSummaryLog("deferred", outcome, "")
			emit(EventRunCompleted, outcome)
			return

		case errors.Is(runErr, ErrDoomLoop):
			writeSummaryLog("doom_loop", outcome, ErrDoomLoop.Error())
			emit(EventRunFailed, RunErrorPayload{
				Code:    "doom_loop",
				Message: ErrDoomLoop.Error(),
			})
			return

		case errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) || runCtx.Err() != nil:
			writeSummaryLog("cancelled", outcome, "run cancelled")
			emit(EventRunCancelled, map[string]any{
				"steps": outcome.Steps,
			})
			return

		case runErr != nil:
			redactedErr := config.Redact(runErr.Error(), knownSecrets)
			writeSummaryLog("failed", outcome, redactedErr)
			emit(EventRunFailed, RunErrorPayload{
				Code:    "run_error",
				Message: redactedErr,
			})
			return
		}

		writeSummaryLog("completed", outcome, "")
		emit(EventRunCompleted, outcome)

		// 首轮对话顺利完成后，异步调度轻量 utility 角色为仍处于默认标题的会话生成精炼标题
		autoTitle := true
		if wsSnap != nil && (wsSnap.Global.MaxSteps > 0 || wsSnap.Global.MaxConcurrency > 0 || wsSnap.Global.DefaultProvider != "") {
			autoTitle = wsSnap.Global.AutoGenerateTitle
		}
		if autoTitle && outcome.CompletedTurn1 && strings.TrimSpace(req.Prompt) != "" {
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
	e.recordSessionHeadAfterRun(meta.ID)

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

	if e.reloadUnsub != nil {
		e.reloadUnsub()
	}

	for _, c := range cancels {
		c()
	}
	e.bgCancel()
	e.bgWG.Wait()

	e.evictAllMCP()

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

		wsID := ""
		if ws != nil {
			wsID = ws.ID
		}

		replyPreview := assistantReply
		if len(replyPreview) > 400 {
			replyPreview = replyPreview[:400]
		}
		prompt := fmt.Sprintf("Generate a concise 3-6 word session title (no quotes, no trailing punctuation) for:\nUser: %s\nAssistant: %s", userPrompt, replyPreview)

		res, err := executeUtilityPrompt(ctx, e.cfg, e.workspaces, e.router, wsID, "utility", "", prompt)
		if err != nil {
			return
		}

		title := sanitizeGeneratedTitle(res.Text)
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
			e.broadcastGlobal(RunEvent{
				Seq:         e.eventSeq.Add(1),
				Type:        EventTitleUpdated,
				WorkspaceID: wsID,
				SessionID:   sessionID,
				AgentRole:   "utility",
				ModelID:     res.ModelID,
				Payload: TitleUpdatedPayload{
					SessionID: sessionID,
					Title:     title,
				},
				Timestamp: e.now().UTC(),
			})
		}
	}()
}

func (e *Engine) syncSessionReadHashesBeforeRun(meta *store.SessionMeta) {
	if e == nil || e.readHashes == nil || meta == nil {
		return
	}
	curHead := fmt.Sprintf("%s:%d", meta.HeadEntryID, meta.HeadSeq)
	e.sessionHeadsMu.Lock()
	prevHead, recorded := e.sessionHeads[meta.ID]
	e.sessionHeadsMu.Unlock()
	if recorded && prevHead != curHead {
		e.readHashes.InvalidateSession(meta.ID)
	}
}

func (e *Engine) recordSessionHeadAfterRun(sessionID string) {
	if e == nil || e.sessions == nil {
		return
	}
	curMeta, err := e.sessions.GetMeta(sessionID)
	if err != nil || curMeta == nil {
		return
	}
	curHead := fmt.Sprintf("%s:%d", curMeta.HeadEntryID, curMeta.HeadSeq)
	e.sessionHeadsMu.Lock()
	e.sessionHeads[sessionID] = curHead
	e.sessionHeadsMu.Unlock()
}

func (e *Engine) storeRecentRunReplayLocked(runID string, history []RunEvent) {
	if len(history) == 0 {
		return
	}
	if _, exists := e.recentRuns[runID]; !exists {
		e.recentOrder = append(e.recentOrder, runID)
	}
	e.recentRuns[runID] = history
	for len(e.recentOrder) > maxRecentRunReplays {
		oldest := e.recentOrder[0]
		e.recentOrder = e.recentOrder[1:]
		delete(e.recentRuns, oldest)
	}
}

func computeMCPFingerprint(mcpCfg config.MCPConfig) string {
	raw, _ := json.Marshal(mcpCfg)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (e *Engine) acquireMCPManager(
	ctx context.Context,
	wsID, effectiveRoot, globalMCPFile string,
	mcpCfg config.MCPConfig,
) (*tools.MCPManager, func()) {
	canonRoot, cErr := config.CanonicalPath(effectiveRoot)
	if cErr != nil || canonRoot == "" {
		canonRoot = filepath.Clean(effectiveRoot)
	}
	poolKey := wsID + "\x00" + canonRoot
	fp := computeMCPFingerprint(mcpCfg)

	var staleMgr *tools.MCPManager
	e.mcpMu.Lock()
	if existing, ok := e.mcpPool[poolKey]; ok && existing != nil {
		if !existing.evicted && existing.fingerprint == fp && existing.mgr != nil && existing.mgr.IsHealthy() {
			existing.refs++
			e.mcpMu.Unlock()
			return existing.mgr, func() { e.releasePooledMCP(existing) }
		}
		existing.evicted = true
		delete(e.mcpPool, poolKey)
		if existing.refs <= 0 {
			staleMgr = existing.mgr
		}
	}
	e.mcpMu.Unlock()

	if staleMgr != nil {
		_ = staleMgr.Close()
	}

	mgr := tools.NewMCPManager(e.cfg.Secrets(), globalMCPFile, effectiveRoot)
	connErr := mgr.ConnectAll(ctx, mcpCfg)
	if ctx.Err() != nil {
		_ = mgr.Close()
		return nil, nil
	}
	// 若存在部分或全部服务器握手失败，仅在当次运行挂载已成功的工具并在运行结束时立即释放，避免缓存半残连接。
	if connErr != nil {
		return mgr, func() { _ = mgr.Close() }
	}

	e.runsMu.RLock()
	isClosed := e.closed
	e.runsMu.RUnlock()
	if isClosed {
		return mgr, func() { _ = mgr.Close() }
	}

	entry := &pooledMCP{
		key:         poolKey,
		fingerprint: fp,
		mgr:         mgr,
		refs:        1,
	}
	var racePrev *tools.MCPManager
	e.mcpMu.Lock()
	if prev, ok := e.mcpPool[poolKey]; ok && prev != nil {
		prev.evicted = true
		if prev.refs <= 0 {
			racePrev = prev.mgr
		}
	}
	e.mcpPool[poolKey] = entry
	e.mcpMu.Unlock()
	if racePrev != nil {
		_ = racePrev.Close()
	}

	return mgr, func() { e.releasePooledMCP(entry) }
}

func (e *Engine) releasePooledMCP(entry *pooledMCP) {
	if entry == nil {
		return
	}
	var toClose *tools.MCPManager
	e.mcpMu.Lock()
	entry.refs--
	if entry.refs <= 0 {
		unhealthy := entry.mgr == nil || !entry.mgr.IsHealthy()
		if entry.evicted || unhealthy {
			if !entry.evicted {
				entry.evicted = true
				if cur, ok := e.mcpPool[entry.key]; ok && cur == entry {
					delete(e.mcpPool, entry.key)
				}
			}
			toClose = entry.mgr
		}
	}
	e.mcpMu.Unlock()
	if toClose != nil {
		_ = toClose.Close()
	}
}

func (e *Engine) evictAllMCP() {
	if e == nil {
		return
	}
	var toClose []*tools.MCPManager
	e.mcpMu.Lock()
	for k, entry := range e.mcpPool {
		delete(e.mcpPool, k)
		if entry != nil {
			entry.evicted = true
			if entry.refs <= 0 && entry.mgr != nil {
				toClose = append(toClose, entry.mgr)
			}
		}
	}
	e.mcpMu.Unlock()
	for _, m := range toClose {
		_ = m.Close()
	}
}

func (e *Engine) evictWorkspaceMCP(wsID string) {
	if e == nil || strings.TrimSpace(wsID) == "" {
		return
	}
	prefix := strings.TrimSpace(wsID) + "\x00"
	var toClose []*tools.MCPManager
	e.mcpMu.Lock()
	for k, entry := range e.mcpPool {
		if strings.HasPrefix(k, prefix) {
			delete(e.mcpPool, k)
			if entry != nil {
				entry.evicted = true
				if entry.refs <= 0 && entry.mgr != nil {
					toClose = append(toClose, entry.mgr)
				}
			}
		}
	}
	e.mcpMu.Unlock()
	for _, m := range toClose {
		_ = m.Close()
	}
}

func (e *Engine) appendRunLog(rec RunLogRecord, knownSecrets []string) {
	if e == nil || e.cfg == nil {
		return
	}
	paths := e.cfg.Paths()
	if strings.TrimSpace(paths.Root) == "" {
		return
	}
	if rec.PromptPreview != "" {
		cleaned := config.EscapeBidi(config.Redact(strings.TrimSpace(rec.PromptPreview), knownSecrets))
		cleaned = strings.ReplaceAll(cleaned, "\r\n", " ")
		cleaned = strings.ReplaceAll(cleaned, "\n", " ")
		runes := []rune(cleaned)
		if len(runes) > 240 {
			cleaned = string(runes[:240]) + "..."
		}
		rec.PromptPreview = cleaned
	}
	if rec.Error != "" {
		cleaned := config.EscapeBidi(config.Redact(strings.TrimSpace(rec.Error), knownSecrets))
		runes := []rune(cleaned)
		if len(runes) > 512 {
			cleaned = string(runes[:512]) + "..."
		}
		rec.Error = cleaned
	}

	logPath := filepath.Join(paths.LogsDir(), "runs.jsonl")
	lockPath := logPath + ".lock"
	_ = store.WithFlock(lockPath, true, func() error {
		return store.AppendJSONLine(logPath, rec)
	})
}

func isSubagentOrUtilityRole(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "explorer", "verifier", "debugger", "utility":
		return true
	default:
		return false
	}
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

func sendRunEvent(ctx context.Context, ch chan RunEvent, ev RunEvent) {
	select {
	case ch <- ev:
		return
	default:
	}
	// 对高频流式增量块，若缓冲区已满（例如客户端已断开或无人消费）直接丢弃，严禁阻塞后台 AgentLoop
	isDelta := ev.Type == EventTextDelta || ev.Type == EventThinkingDelta
	if isDelta {
		return
	}
	// 对结构化事件与终态事件，至多短暂等待 5ms；若仍满则淘汰最旧事件以确保终态可达且绝不死锁会话锁
	timer := time.NewTimer(5 * time.Millisecond)
	defer timer.Stop()
	select {
	case ch <- ev:
		return
	case <-ctx.Done():
	case <-timer.C:
	}
	select {
	case ch <- ev:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- ev:
	default:
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
