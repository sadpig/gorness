package alwayson

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"
	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/core"
	"gorness.dev/gorness/internal/store"
	"gorness.dev/gorness/internal/tools"
)

const (
	PluginID = "alwayson"

	defaultTickInterval          = 250 * time.Millisecond
	defaultRetryBackoff          = 25 * time.Millisecond
	defaultDreamSessionThreshold = 5
	defaultDreamCooldown         = 10 * time.Minute
)

var (
	ErrWorkerLeaseHeld  = errors.New("alwayson worker lease is already held by another active consumer")
	ErrDeferredNotFound = errors.New("deferred card not found")

	memoryPluginToolNames = []string{
		"memory_read",
		"memory_write",
		"progress_read",
		"progress_write",
	}
)

type DeferredCard struct {
	ID          string           `json:"id" toml:"id"`
	RunID       string           `json:"run_id" toml:"run_id"`
	WorkspaceID string           `json:"workspace_id" toml:"workspace_id"`
	SessionID   string           `json:"session_id" toml:"session_id"`
	AgentRole   string           `json:"agent_role,omitempty" toml:"agent_role,omitempty"`
	ToolCallID  string           `json:"tool_call_id,omitempty" toml:"tool_call_id,omitempty"`
	ToolName    string           `json:"tool_name" toml:"tool_name"`
	ReasonCode  string           `json:"reason_code" toml:"reason_code"`
	Reason      string           `json:"reason" toml:"reason"`
	Fingerprint string           `json:"fingerprint,omitempty" toml:"fingerprint,omitempty"`
	Args        json.RawMessage  `json:"args,omitempty" toml:"-"`
	Hunks       []tools.DiffHunk `json:"hunks,omitempty" toml:"hunks,omitempty"`
	Status      string           `json:"status,omitempty" toml:"status,omitempty"`
	Remember    bool             `json:"remember,omitempty" toml:"remember,omitempty"`
	Feedback    string           `json:"feedback,omitempty" toml:"feedback,omitempty"`
	CreatedAt   time.Time        `json:"created_at" toml:"created_at"`
	ResolvedAt  *time.Time       `json:"resolved_at,omitempty" toml:"resolved_at,omitempty"`
}

// DeferredResolution 表示用户对挂起工具卡片的审批决策（支持部分 hunk 选择、持久记住授权与转向反馈注入）。
type DeferredResolution struct {
	Decision        string   `json:"decision,omitempty"`
	Approved        bool     `json:"approved"`
	SelectedHunkIDs []string `json:"selected_hunk_ids,omitempty"`
	Remember        bool     `json:"remember,omitempty"`
	Feedback        string   `json:"feedback,omitempty"`
}

// Resolution 为 DeferredResolution 的别名，便于网关层直接引用。
type Resolution = DeferredResolution

type deferredFrontmatter struct {
	ID              string           `toml:"id" json:"id"`
	RunID           string           `toml:"run_id" json:"run_id"`
	WorkspaceID     string           `toml:"workspace_id" json:"workspace_id"`
	SessionID       string           `toml:"session_id" json:"session_id"`
	AgentRole       string           `toml:"agent_role,omitempty" json:"agent_role,omitempty"`
	ToolCallID      string           `toml:"tool_call_id,omitempty" json:"tool_call_id,omitempty"`
	ToolName        string           `toml:"tool_name" json:"tool_name"`
	ReasonCode      string           `toml:"reason_code" json:"reason_code"`
	Reason          string           `toml:"reason" json:"reason"`
	Fingerprint     string           `toml:"fingerprint,omitempty" json:"fingerprint,omitempty"`
	ArgsJSON        string           `toml:"args_json,omitempty" json:"args_json,omitempty"`
	Hunks           []tools.DiffHunk `toml:"hunks,omitempty" json:"hunks,omitempty"`
	Status          string           `toml:"status" json:"status"`
	SelectedHunkIDs []string         `toml:"selected_hunk_ids,omitempty" json:"selected_hunk_ids,omitempty"`
	Remember        bool             `toml:"remember,omitempty" json:"remember,omitempty"`
	Feedback        string           `toml:"feedback,omitempty" json:"feedback,omitempty"`
	CreatedAt       time.Time        `toml:"created_at" json:"created_at"`
	ResolvedAt      *time.Time       `toml:"resolved_at,omitempty" json:"resolved_at,omitempty"`
}

type AlwaysOnState struct {
	Running         bool             `json:"running"`
	QueuedCount     int              `json:"queued_count"`
	RunningCount    int              `json:"running_count"`
	FailedCount     int              `json:"failed_count"`
	DeferredCount   int              `json:"deferred_count"`
	CandidatesCount int              `json:"candidates_count"`
	Inbox           []InboxEvent     `json:"inbox"`
	Timers          []TimerEntry     `json:"timers"`
	Deferred        []DeferredCard   `json:"deferred"`
	Outbox          []OutboxItem     `json:"outbox"`
	Candidates      []CandidateSkill `json:"candidates,omitempty"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

type ServiceOptions struct {
	Config                *config.Manager
	Engine                *core.Engine
	Notifier              Notifier
	Now                   func() time.Time
	TickInterval          time.Duration
	RetryBackoff          time.Duration
	MaxInboxRetained      int
	MaxOutboxRetained     int
	DreamSessionThreshold int
	DreamCooldown         time.Duration
	DisableAutoDream      bool
}

type runNotificationCapture struct {
	wsID                   string
	sessionID              string
	notifiedCount          int
	suppressedCount        int
	scheduledFollowUpCount int
	lastSuppressed         string
}

type Service struct {
	cfg          *config.Manager
	engine       *core.Engine
	paths        config.VaultPaths
	notifier     Notifier
	now          func() time.Time
	tickInterval time.Duration
	retryBackoff time.Duration

	inbox   *inboxStore
	timers  *timerStore
	memory  *MemoryStore
	outbox  *outboxStore
	dreamer *Dreamer

	wakeCh chan struct{}

	captureMu sync.Mutex
	captures  map[string]*runNotificationCapture // sessionID -> active Always-On run capture

	dreamMu                sync.Mutex
	disableAutoDream       bool
	customDreamCooldown    bool
	dreamSessionThreshold  int
	dreamCooldown          time.Duration
	completedSinceDream    int
	lastCompletedWorkspace string
	lastDreamAt            time.Time
	dreamRunning           bool

	lifeMu          sync.Mutex
	running         bool
	ownsPluginMount bool
	stopCancel      context.CancelFunc
	leaseUnlock     func()
	globalUnsub     func()
	wg              sync.WaitGroup
}

func NewService(opts ServiceOptions) (*Service, error) {
	if opts.Config == nil {
		return nil, errors.New("alwayson.NewService: Config manager is required")
	}
	if opts.Engine == nil {
		return nil, errors.New("alwayson.NewService: Engine is required")
	}

	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	notifier := opts.Notifier
	if notifier == nil {
		notifier = NewOSAScriptNotifier()
	}
	tickInt := opts.TickInterval
	if tickInt <= 0 {
		tickInt = defaultTickInterval
	}
	retryBackoff := opts.RetryBackoff
	if retryBackoff <= 0 {
		retryBackoff = defaultRetryBackoff
	}
	dreamThresh := opts.DreamSessionThreshold
	if dreamThresh <= 0 {
		dreamThresh = defaultDreamSessionThreshold
	}
	dreamCooldown := opts.DreamCooldown
	if dreamCooldown <= 0 {
		dreamCooldown = defaultDreamCooldown
	}

	paths := opts.Config.Paths()
	for _, dir := range []string{
		paths.AlwaysOnDir(),
		filepath.Join(paths.AlwaysOnDir(), "deferred"),
		filepath.Join(paths.AlwaysOnDir(), "deferred", "resolved"),
		paths.MemoryDir(),
		filepath.Join(paths.MemoryDir(), "notes"),
		filepath.Join(paths.SkillsDir(), "_candidates"),
		filepath.Join(paths.RuntimeDir(), "locks"),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create alwayson dir %s: %w", dir, err)
		}
	}

	mem := NewMemoryStore(paths)
	mem.nowFn = nowFn

	svc := &Service{
		cfg:                   opts.Config,
		engine:                opts.Engine,
		paths:                 paths,
		notifier:              notifier,
		now:                   nowFn,
		tickInterval:          tickInt,
		retryBackoff:          retryBackoff,
		inbox:                 newInboxStore(paths, opts.MaxInboxRetained),
		timers:                newTimerStore(paths),
		memory:                mem,
		outbox:                newOutboxStore(paths, opts.MaxOutboxRetained),
		wakeCh:                make(chan struct{}, 1),
		captures:              make(map[string]*runNotificationCapture),
		disableAutoDream:      opts.DisableAutoDream,
		customDreamCooldown:   opts.DreamCooldown > 0,
		dreamSessionThreshold: dreamThresh,
		dreamCooldown:         dreamCooldown,
	}
	svc.dreamer = &Dreamer{svc: svc}
	mem.onMutate = func() {
		svc.publishAlwaysOnEvent("memory_updated", nil)
	}

	if !svc.isPluginMounted() {
		if err := svc.mountPluginTools(); err != nil {
			return nil, err
		}
	}

	globalCh, unsub := svc.engine.SubscribeGlobal()
	svc.globalUnsub = unsub
	svc.wg.Add(1)
	go func() {
		defer svc.wg.Done()
		svc.listenGlobalEvents(globalCh)
	}()

	return svc, nil
}

// Memory 暴露四层记忆存储实例（供 HTTP API、CLI、TUI 与插件工具统一读写）。
func (s *Service) Memory() *MemoryStore {
	return s.memory
}

// Dreamer 暴露闲时记忆蒸馏器实例。
func (s *Service) Dreamer() *Dreamer {
	return s.dreamer
}

func (s *Service) publishAlwaysOnEvent(action string, extra map[string]any) {
	if s == nil || s.engine == nil {
		return
	}
	payload := map[string]any{
		"action": action,
	}
	for k, v := range extra {
		payload[k] = v
	}
	s.engine.PublishSystemEvent(core.EventAlwaysOnUpdated, payload)
}

// listenGlobalEvents 订阅全局引擎事件总线：
// 1. 捕获任意无人值守运行 (`POST /api/runs/stream`、CLI、ACP 或后台 worker) 触发的 `EventToolDeferred`，幂等写入 `alwayson/deferred/<id>.md`；
// 2. 计数新完成的业务会话，以便后台空闲时自动触发 `Dreamer.RunDream`。
func (s *Service) listenGlobalEvents(ch <-chan core.RunEvent) {
	for ev := range ch {
		switch ev.Type {
		case core.EventToolDeferred:
			var def core.DeferredToolPayload
			switch p := ev.Payload.(type) {
			case core.DeferredToolPayload:
				def = p
			case *core.DeferredToolPayload:
				if p == nil {
					continue
				}
				def = *p
			default:
				continue
			}
			if strings.TrimSpace(def.WorkspaceID) == "" {
				def.WorkspaceID = ev.WorkspaceID
			}
			if strings.TrimSpace(def.SessionID) == "" {
				def.SessionID = ev.SessionID
			}
			if strings.TrimSpace(def.RunID) == "" {
				def.RunID = ev.RunID
			}
			role := strings.TrimSpace(ev.AgentRole)
			if role == "" {
				role = "coder"
			}
			_, _, _ = s.recordDeferredAndNotifyIfNew(def, role)

		case core.EventRunCompleted:
			role := strings.TrimSpace(ev.AgentRole)
			if role == "dreamer" || role == "utility" {
				continue
			}
			if out, ok := ev.Payload.(core.RunOutcome); ok && out.Status == "deferred" {
				continue
			}
			s.dreamMu.Lock()
			s.completedSinceDream++
			if strings.TrimSpace(ev.WorkspaceID) != "" {
				s.lastCompletedWorkspace = strings.TrimSpace(ev.WorkspaceID)
			}
			shouldWake := !s.disableAutoDream && s.completedSinceDream >= s.dreamSessionThreshold && !s.dreamRunning
			s.dreamMu.Unlock()
			if shouldWake {
				s.signalWake()
			}
		}
	}
}

func (s *Service) recordDreamCompleted(now time.Time) {
	s.dreamMu.Lock()
	s.completedSinceDream = 0
	s.lastDreamAt = now.UTC()
	s.dreamMu.Unlock()
}

func (s *Service) maybeAutoDream(ctx context.Context) {
	if s.disableAutoDream {
		return
	}
	cooldown := s.dreamCooldown
	if s.cfg != nil && s.cfg.Current() != nil {
		g := s.cfg.Current().Global
		if g.MaxSteps > 0 || g.MaxConcurrency > 0 || g.DefaultProvider != "" {
			if !g.AutoDream || g.DreamerIdleMinutes <= 0 {
				return
			}
			if !s.customDreamCooldown && g.DreamerIdleMinutes > 0 {
				cooldown = time.Duration(g.DreamerIdleMinutes) * time.Minute
			}
		}
	}
	now := s.now().UTC()
	s.dreamMu.Lock()
	if s.dreamRunning || s.completedSinceDream < s.dreamSessionThreshold {
		s.dreamMu.Unlock()
		return
	}
	if !s.lastDreamAt.IsZero() && now.Sub(s.lastDreamAt) < cooldown {
		s.dreamMu.Unlock()
		return
	}
	wsID := s.lastCompletedWorkspace
	s.dreamMu.Unlock()

	// Only run idle dream pass when Engine has zero active runs and Inbox has no queued/running items.
	if len(s.engine.ActiveRuns()) > 0 || s.inbox.hasActiveOrQueuedWork(now) {
		return
	}

	s.dreamMu.Lock()
	if s.dreamRunning || s.completedSinceDream < s.dreamSessionThreshold {
		s.dreamMu.Unlock()
		return
	}
	if !s.lastDreamAt.IsZero() && now.Sub(s.lastDreamAt) < cooldown {
		s.dreamMu.Unlock()
		return
	}
	s.dreamRunning = true
	s.dreamMu.Unlock()

	defer func() {
		s.dreamMu.Lock()
		s.dreamRunning = false
		s.dreamMu.Unlock()
	}()

	_, _ = s.dreamer.RunDream(ctx, wsID)
}

func (s *Service) isPluginMounted() bool {
	hooks := s.engine.Hooks()
	if hooks == nil {
		return false
	}
	for _, id := range hooks.MountedPlugins() {
		if id == PluginID {
			return true
		}
	}
	return false
}

func (s *Service) mountPluginTools() error {
	hooks := s.engine.Hooks()
	if hooks != nil {
		for _, id := range hooks.MountedPlugins() {
			if id == PluginID {
				_ = hooks.UnmountPlugin(PluginID)
				break
			}
		}
		if err := hooks.MountPlugin(core.Plugin{
			ID: PluginID,
			Setup: func(sc *core.Scope) error {
				sc.OnPreStep(s.onPreStepMemoryHook)
				sc.RegisterTool(s.makeSetFollowUpTool())
				sc.RegisterTool(s.makeNotifyUserTool())
				return nil
			},
		}); err != nil {
			return err
		}
	}

	s.engine.RegisterPluginTools(
		s.makeMemoryReadTool(),
		s.makeMemoryWriteTool(),
		s.makeProgressReadTool(),
		s.makeProgressWriteTool(),
	)

	s.lifeMu.Lock()
	s.ownsPluginMount = true
	s.lifeMu.Unlock()
	return nil
}

func (s *Service) onPreStepMemoryHook(_ context.Context, ev *core.PreStepEvent) error {
	if ev == nil || ev.Step != 1 || strings.EqualFold(strings.TrimSpace(ev.AgentRole), "utility") {
		return nil
	}
	reminders := s.memory.buildStepOneMemoryReminders(ev.WorkspaceID)
	if len(reminders) > 0 {
		ev.DynamicReminders = append(ev.DynamicReminders, reminders...)
	}
	return nil
}

func (s *Service) makeMemoryReadTool() tools.Tool {
	return tools.Tool{
		Name:            "memory_read",
		Description:     "Read Gorness 4-layer global memory: Layer 1 ('instructions' -> INSTRUCTIONS.md), Layer 2 ('index' -> INDEX.md), or Layer 3 durable notes ('notes' list or specific note by 'slug').",
		InputSchema:     json.RawMessage(`{"type":"object","properties":{"layer":{"type":"string","description":"Global memory layer: 'instructions', 'index', or 'notes'"},"slug":{"type":"string","description":"Optional note slug under memory/notes/<slug>.md"}}}`),
		ReadOnly:        true,
		Mutating:        false,
		ConcurrencySafe: true,
		Handler: func(_ context.Context, _ tools.CallContext, rawInput json.RawMessage) (tools.ToolResult, error) {
			var in struct {
				Layer string `json:"layer"`
				Slug  string `json:"slug"`
			}
			if len(bytes.TrimSpace(rawInput)) > 0 {
				if err := json.Unmarshal(rawInput, &in); err != nil {
					return tools.ToolResult{}, fmt.Errorf("invalid memory_read input: %w", err)
				}
			}

			if strings.TrimSpace(in.Slug) != "" {
				note, err := s.memory.GetNote(in.Slug)
				if err != nil {
					return tools.ToolResult{}, err
				}
				encoded, _ := json.Marshal(note)
				return tools.ToolResult{Output: string(encoded)}, nil
			}

			layer := strings.ToLower(strings.TrimSpace(in.Layer))
			switch layer {
			case "instructions", "instructions.md", "layer1", "index", "index.md", "layer2":
				content, err := s.memory.GetGlobalLayer(layer)
				if err != nil {
					return tools.ToolResult{}, err
				}
				out, _ := json.Marshal(map[string]any{
					"layer":   layer,
					"content": content,
				})
				return tools.ToolResult{Output: string(out)}, nil

			case "notes", "note", "layer3":
				notes, err := s.memory.ListNotes()
				if err != nil {
					return tools.ToolResult{}, err
				}
				type noteMeta struct {
					Slug      string    `json:"slug"`
					Title     string    `json:"title"`
					Summary   string    `json:"summary"`
					Tags      []string  `json:"tags,omitempty"`
					UpdatedAt time.Time `json:"updated_at"`
				}
				summaries := make([]noteMeta, len(notes))
				for i, n := range notes {
					summaries[i] = noteMeta{
						Slug:      n.Slug,
						Title:     n.Title,
						Summary:   n.Summary,
						Tags:      n.Tags,
						UpdatedAt: n.UpdatedAt,
					}
				}
				out, _ := json.Marshal(map[string]any{"notes": summaries})
				return tools.ToolResult{Output: string(out)}, nil

			case "":
				inst, _ := s.memory.GetGlobalLayer(MemoryLayerInstructions)
				idx, _ := s.memory.GetGlobalLayer(MemoryLayerIndex)
				notes, _ := s.memory.ListNotes()
				slugs := make([]string, len(notes))
				for i, n := range notes {
					slugs[i] = n.Slug
				}
				out, _ := json.Marshal(map[string]any{
					"instructions": inst,
					"index":        idx,
					"note_slugs":   slugs,
				})
				return tools.ToolResult{Output: string(out)}, nil

			default:
				return tools.ToolResult{}, fmt.Errorf("%w: %q", ErrInvalidMemoryLayer, in.Layer)
			}
		},
	}
}

func (s *Service) makeMemoryWriteTool() tools.Tool {
	return tools.Tool{
		Name:            "memory_write",
		Description:     "Write or delete Gorness global memory: update Layer 1 ('instructions') or Layer 2 ('index'), or upsert/delete a Layer 3 note ('note') under memory/notes/<slug>.md with automatic INDEX.md pointer synchronization.",
		InputSchema:     json.RawMessage(`{"type":"object","properties":{"layer":{"type":"string","description":"Target layer: 'instructions', 'index', or 'note' (default 'note')"},"slug":{"type":"string","description":"Note slug when writing or deleting a Layer 3 note"},"title":{"type":"string","description":"Human-readable note title"},"content":{"type":"string","description":"Markdown content to persist"},"tags":{"type":"array","items":{"type":"string"},"description":"Optional classification tags for Layer 3 notes"},"delete":{"type":"boolean","description":"Delete the specified Layer 3 note slug when true"}}}`),
		ReadOnly:        false,
		Mutating:        false,
		ConcurrencySafe: true,
		Handler: func(_ context.Context, _ tools.CallContext, rawInput json.RawMessage) (tools.ToolResult, error) {
			var in struct {
				Layer   string   `json:"layer"`
				Slug    string   `json:"slug"`
				Title   string   `json:"title"`
				Content string   `json:"content"`
				Tags    []string `json:"tags"`
				Delete  bool     `json:"delete"`
			}
			if err := json.Unmarshal(rawInput, &in); err != nil {
				return tools.ToolResult{}, fmt.Errorf("invalid memory_write input: %w", err)
			}

			layer := strings.ToLower(strings.TrimSpace(in.Layer))
			if layer == "" {
				layer = "note"
			}

			switch layer {
			case "instructions", "instructions.md", "layer1", "index", "index.md", "layer2":
				if err := s.memory.SaveGlobalLayer(layer, in.Content); err != nil {
					return tools.ToolResult{}, err
				}
				out, _ := json.Marshal(map[string]any{"saved": true, "layer": layer})
				return tools.ToolResult{Output: string(out)}, nil

			case "note", "notes", "layer3":
				slug := strings.TrimSpace(in.Slug)
				if slug == "" && strings.TrimSpace(in.Title) != "" {
					slug = sanitizeSlug(in.Title)
				}
				if slug == "" {
					return tools.ToolResult{}, errors.New("memory_write for 'note' requires slug or title")
				}
				if in.Delete {
					if err := s.memory.DeleteNote(slug); err != nil {
						return tools.ToolResult{}, err
					}
					out, _ := json.Marshal(map[string]any{"deleted": true, "slug": slug})
					return tools.ToolResult{Output: string(out)}, nil
				}
				note, err := s.memory.SaveNote(slug, in.Title, in.Content, in.Tags)
				if err != nil {
					return tools.ToolResult{}, err
				}
				out, _ := json.Marshal(note)
				return tools.ToolResult{Output: string(out)}, nil

			default:
				return tools.ToolResult{}, fmt.Errorf("%w: %q", ErrInvalidMemoryLayer, in.Layer)
			}
		},
	}
}

func (s *Service) makeProgressReadTool() tools.Tool {
	return tools.Tool{
		Name:            "progress_read",
		Description:     "Read Layer 4 workspace-scoped progress memory under workspaces/<wsID>/memory/progress/<slug>.md (or list all progress entries when slug is omitted).",
		InputSchema:     json.RawMessage(`{"type":"object","properties":{"workspace_id":{"type":"string","description":"Optional workspace ID (defaults to active workspace)"},"slug":{"type":"string","description":"Optional progress entry slug"}}}`),
		ReadOnly:        true,
		Mutating:        false,
		ConcurrencySafe: true,
		Handler: func(_ context.Context, call tools.CallContext, rawInput json.RawMessage) (tools.ToolResult, error) {
			var in struct {
				WorkspaceID string `json:"workspace_id"`
				Slug        string `json:"slug"`
			}
			if len(bytes.TrimSpace(rawInput)) > 0 {
				if err := json.Unmarshal(rawInput, &in); err != nil {
					return tools.ToolResult{}, fmt.Errorf("invalid progress_read input: %w", err)
				}
			}
			wsID := strings.TrimSpace(in.WorkspaceID)
			if wsID == "" {
				wsID = s.resolveWorkspaceIDFromCall(call)
			}
			if wsID == "" {
				return tools.ToolResult{}, errors.New("unable to resolve workspace_id for progress_read")
			}

			if strings.TrimSpace(in.Slug) != "" {
				entry, err := s.memory.GetProgress(wsID, in.Slug)
				if err != nil {
					return tools.ToolResult{}, err
				}
				out, _ := json.Marshal(entry)
				return tools.ToolResult{Output: string(out)}, nil
			}

			entries, err := s.memory.ListProgress(wsID)
			if err != nil {
				return tools.ToolResult{}, err
			}
			out, _ := json.Marshal(map[string]any{"workspace_id": wsID, "entries": entries})
			return tools.ToolResult{Output: string(out)}, nil
		},
	}
}

func (s *Service) makeProgressWriteTool() tools.Tool {
	return tools.Tool{
		Name:            "progress_write",
		Description:     "Save or delete a Layer 4 workspace-scoped progress checkpoint under workspaces/<wsID>/memory/progress/<slug>.md.",
		InputSchema:     json.RawMessage(`{"type":"object","properties":{"workspace_id":{"type":"string","description":"Optional workspace ID (defaults to active workspace)"},"slug":{"type":"string","description":"Progress entry slug (e.g. 'current', 'migration-plan')"},"content":{"type":"string","description":"Markdown progress checkpoint content"},"delete":{"type":"boolean","description":"Delete the progress entry when true"}},"required":["slug"]}`),
		ReadOnly:        false,
		Mutating:        false,
		ConcurrencySafe: true,
		Handler: func(_ context.Context, call tools.CallContext, rawInput json.RawMessage) (tools.ToolResult, error) {
			var in struct {
				WorkspaceID string `json:"workspace_id"`
				Slug        string `json:"slug"`
				Content     string `json:"content"`
				Delete      bool   `json:"delete"`
			}
			if err := json.Unmarshal(rawInput, &in); err != nil {
				return tools.ToolResult{}, fmt.Errorf("invalid progress_write input: %w", err)
			}
			wsID := strings.TrimSpace(in.WorkspaceID)
			if wsID == "" {
				wsID = s.resolveWorkspaceIDFromCall(call)
			}
			if wsID == "" {
				return tools.ToolResult{}, errors.New("unable to resolve workspace_id for progress_write")
			}

			if in.Delete {
				if err := s.memory.DeleteProgress(wsID, in.Slug); err != nil {
					return tools.ToolResult{}, err
				}
				out, _ := json.Marshal(map[string]any{"deleted": true, "workspace_id": wsID, "slug": in.Slug})
				return tools.ToolResult{Output: string(out)}, nil
			}

			entry, err := s.memory.SaveProgress(wsID, in.Slug, in.Content)
			if err != nil {
				return tools.ToolResult{}, err
			}
			out, _ := json.Marshal(entry)
			return tools.ToolResult{Output: string(out)}, nil
		},
	}
}

func (s *Service) makeSetFollowUpTool() tools.Tool {
	return tools.Tool{
		Name:            "set_followup",
		Description:     "Schedule a durable Always-On follow-up alarm in timers.toml to re-awaken this session or workspace after a delay.",
		InputSchema:     json.RawMessage(`{"type":"object","properties":{"after_seconds":{"type":"integer","description":"Delay in seconds before follow-up fires"},"after":{"type":"string","description":"Optional Go duration string such as '5m' or '1h'"},"note":{"type":"string","description":"Reminder note for the follow-up wake"}},"required":["note"]}`),
		ReadOnly:        false,
		Mutating:        false,
		ConcurrencySafe: true,
		Handler: func(_ context.Context, call tools.CallContext, rawInput json.RawMessage) (tools.ToolResult, error) {
			var in struct {
				AfterSeconds int    `json:"after_seconds"`
				After        string `json:"after"`
				Note         string `json:"note"`
			}
			if err := json.Unmarshal(rawInput, &in); err != nil {
				return tools.ToolResult{}, fmt.Errorf("invalid set_followup input: %w", err)
			}
			var delay time.Duration
			switch {
			case strings.TrimSpace(in.After) != "":
				parsed, pErr := time.ParseDuration(strings.TrimSpace(in.After))
				if pErr != nil {
					return tools.ToolResult{}, fmt.Errorf("invalid duration %q: %w", in.After, pErr)
				}
				delay = parsed
			case in.AfterSeconds > 0:
				delay = time.Duration(in.AfterSeconds) * time.Second
			default:
				delay = 60 * time.Second
			}

			wsID := s.resolveWorkspaceIDFromCall(call)
			if wsID == "" {
				return tools.ToolResult{}, errors.New("unable to resolve workspace_id for set_followup")
			}
			entry, err := s.ScheduleFollowUp(wsID, call.SessionID, delay, in.Note)
			if err != nil {
				return tools.ToolResult{}, err
			}
			if strings.TrimSpace(call.SessionID) != "" {
				s.captureMu.Lock()
				if capState := s.captures[call.SessionID]; capState != nil {
					capState.scheduledFollowUpCount++
				}
				s.captureMu.Unlock()
			}
			out, _ := json.Marshal(entry)
			return tools.ToolResult{Output: string(out)}, nil
		},
	}
}

func (s *Service) makeNotifyUserTool() tools.Tool {
	return tools.Tool{
		Name:            "notify_user",
		Description:     "Deliver an actionable alert to the Gorness Outbox and macOS Notification Center, automatically deduplicating findings already recorded in REPORTED.md.",
		InputSchema:     json.RawMessage(`{"type":"object","properties":{"title":{"type":"string","description":"Concise alert title"},"body":{"type":"string","description":"Actionable details for the user"}},"required":["title","body"]}`),
		ReadOnly:        false,
		Mutating:        false,
		ConcurrencySafe: true,
		Handler: func(_ context.Context, call tools.CallContext, rawInput json.RawMessage) (tools.ToolResult, error) {
			var in struct {
				Title string `json:"title"`
				Body  string `json:"body"`
			}
			if err := json.Unmarshal(rawInput, &in); err != nil {
				return tools.ToolResult{}, fmt.Errorf("invalid notify_user input: %w", err)
			}
			title := strings.TrimSpace(in.Title)
			body := strings.TrimSpace(in.Body)
			if title == "" && body == "" {
				return tools.ToolResult{}, errors.New("notify_user requires title or body")
			}
			if title == "" {
				title = "Gorness Alert"
			}

			wsID := s.resolveWorkspaceIDFromCall(call)
			alreadyReported, err := s.memory.checkAndRecordReported(wsID, title, body, s.now())
			if err != nil {
				return tools.ToolResult{}, err
			}

			s.captureMu.Lock()
			capState := s.captures[call.SessionID]
			if alreadyReported {
				if capState != nil {
					capState.suppressedCount++
					capState.lastSuppressed = title
				}
				s.captureMu.Unlock()
				return tools.ToolResult{
					Output: fmt.Sprintf("Suppressed repeat notification %q (already present in REPORTED.md).", title),
				}, nil
			}
			if capState != nil {
				capState.notifiedCount++
			}
			s.captureMu.Unlock()

			subtitle := s.workspaceLabel(wsID)
			delivered := false
			if s.notifier != nil {
				if nErr := s.notifier.NotifyMac(title, subtitle, body); nErr == nil {
					delivered = true
				}
			}

			outItem, err := s.outbox.append(OutboxItem{
				WorkspaceID: wsID,
				SessionID:   call.SessionID,
				Outcome:     OutcomeSend,
				Title:       title,
				Body:        body,
				Delivered:   delivered,
			}, s.now())
			if err != nil {
				return tools.ToolResult{}, err
			}
			s.publishAlwaysOnEvent("outbox_sent", map[string]any{"outbox_id": outItem.ID})

			return tools.ToolResult{
				Output: fmt.Sprintf("Delivered alert %q to outbox (macOS notified=%v).", title, delivered),
			}, nil
		},
	}
}

func (s *Service) resolveWorkspaceIDFromCall(call tools.CallContext) string {
	if strings.TrimSpace(call.SessionID) != "" {
		s.captureMu.Lock()
		if c, ok := s.captures[call.SessionID]; ok && c.wsID != "" {
			wsID := c.wsID
			s.captureMu.Unlock()
			return wsID
		}
		s.captureMu.Unlock()

		if meta, err := s.engine.Sessions().GetMeta(call.SessionID); err == nil && meta != nil {
			return meta.WorkspaceID
		}
	}
	if strings.TrimSpace(call.WorkspaceID) != "" {
		return strings.TrimSpace(call.WorkspaceID)
	}
	if strings.TrimSpace(call.WorkspaceRoot) != "" {
		if ws, err := s.engine.Workspaces().GetByPath(call.WorkspaceRoot); err == nil && ws != nil {
			return ws.ID
		}
	}
	return ""
}

func (s *Service) workspaceLabel(wsID string) string {
	if wsID == "" {
		return "Gorness"
	}
	if ws, err := s.engine.Workspaces().Get(wsID); err == nil && ws != nil && strings.TrimSpace(ws.Name) != "" {
		return ws.Name
	}
	return wsID
}

func (s *Service) workerLeaseLockPath() string {
	return filepath.Join(s.paths.RuntimeDir(), "locks", "alwayson-worker.lock")
}

func (s *Service) deferredLockPath() string {
	return filepath.Join(s.paths.RuntimeDir(), "locks", "alwayson-deferred.lock")
}

func (s *Service) deferredDir() string {
	return filepath.Join(s.paths.AlwaysOnDir(), "deferred")
}

func (s *Service) deferredResolvedDir() string {
	return filepath.Join(s.deferredDir(), "resolved")
}

func (s *Service) signalWake() {
	select {
	case s.wakeCh <- struct{}{}:
	default:
	}
}

func (s *Service) Enqueue(ev InboxEvent) (bool, error) {
	stored, deduped, err := s.inbox.enqueue(ev, s.now())
	if err != nil {
		return false, err
	}
	if !deduped {
		s.signalWake()
		s.publishAlwaysOnEvent("inbox_enqueued", map[string]any{"event_id": stored.ID})
	}
	return deduped, nil
}

func (s *Service) ScheduleFollowUp(wsID, sessionID string, after time.Duration, note string) (*TimerEntry, error) {
	entry, err := s.timers.scheduleFollowUp(wsID, sessionID, after, note, s.now())
	if err != nil {
		return nil, err
	}
	s.signalWake()
	s.publishAlwaysOnEvent("timer_scheduled", map[string]any{"timer_id": entry.ID})
	return entry, nil
}

func (s *Service) UpsertCron(entry TimerEntry) error {
	if err := s.timers.upsertCron(entry, s.now()); err != nil {
		return err
	}
	s.signalWake()
	s.publishAlwaysOnEvent("timer_upserted", map[string]any{"timer_id": entry.ID})
	return nil
}

func (s *Service) UpdateTimer(id string, patch TimerPatch) (TimerEntry, error) {
	updated, err := s.timers.update(id, patch, s.now())
	if err != nil {
		return TimerEntry{}, err
	}
	s.signalWake()
	s.publishAlwaysOnEvent("timer_updated", map[string]any{"timer_id": updated.ID})
	return updated, nil
}

func (s *Service) DeleteTimer(id string) error {
	if err := s.timers.delete(id); err != nil {
		return err
	}
	s.publishAlwaysOnEvent("timer_deleted", map[string]any{"timer_id": id})
	return nil
}

func (s *Service) TriggerTimer(ctx context.Context, id string) error {
	_ = ctx
	now := s.now()
	stored, err := s.timers.triggerNow(id, now, func(ev InboxEvent) (InboxEvent, bool, error) {
		return s.inbox.enqueue(ev, now)
	})
	if err != nil {
		return err
	}
	s.signalWake()
	s.publishAlwaysOnEvent("timer_triggered", map[string]any{
		"timer_id": id,
		"event_id": stored.ID,
	})
	return nil
}

func (s *Service) ListTimers() ([]TimerEntry, error) {
	return s.timers.list()
}

func (s *Service) TickTimers(now time.Time) ([]InboxEvent, error) {
	if now.IsZero() {
		now = s.now()
	}
	enqueued, err := s.timers.tickDue(now, func(ev InboxEvent) (InboxEvent, bool, error) {
		return s.inbox.enqueue(ev, now)
	})
	if err != nil {
		return nil, err
	}
	if len(enqueued) > 0 {
		s.signalWake()
		s.publishAlwaysOnEvent("timers_fired", map[string]any{"count": len(enqueued)})
	}
	return enqueued, nil
}

func (s *Service) ListOutbox(limit int) ([]OutboxItem, error) {
	return s.outbox.list(limit)
}

func (s *Service) AckOutbox(ids []string) (int, error) {
	acked, err := s.outbox.ack(ids, s.now())
	if err != nil {
		return 0, err
	}
	if acked > 0 {
		s.publishAlwaysOnEvent("outbox_acked", map[string]any{"count": acked})
	}
	return acked, nil
}

// Compact 手动或定时压缩已确认的 Outbox 记录及已终态超限的 Inbox JSONL 历史。
func (s *Service) Compact(now time.Time) error {
	if now.IsZero() {
		now = s.now()
	}
	if _, err := s.outbox.compact(); err != nil {
		return err
	}
	if _, err := s.inbox.compact(now); err != nil {
		return err
	}
	return nil
}

func (s *Service) ListDeferred() ([]DeferredCard, error) {
	var cards []DeferredCard
	err := store.WithFlock(s.deferredLockPath(), false, func() error {
		entries, err := os.ReadDir(s.deferredDir())
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("readdir deferred: %w", err)
		}
		for _, ent := range entries {
			if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".md") {
				continue
			}
			p := filepath.Join(s.deferredDir(), ent.Name())
			raw, rErr := os.ReadFile(p)
			if rErr != nil {
				continue
			}
			card, pErr := parseDeferredCardMD(raw, strings.TrimSuffix(ent.Name(), ".md"))
			if pErr != nil {
				continue
			}
			if card.Status != "" && card.Status != "pending" {
				continue
			}
			cards = append(cards, card)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(cards, func(i, j int) bool {
		if !cards[i].CreatedAt.Equal(cards[j].CreatedAt) {
			return cards[i].CreatedAt.Before(cards[j].CreatedAt)
		}
		return cards[i].ID < cards[j].ID
	})
	return cards, nil
}

// ResolveDeferred 兼容三参调用 `(id, approved, selectedHunks)` 与扩展 `DeferredResolution` 选项（支持 `Remember` 与 `Feedback`）。
func (s *Service) ResolveDeferred(id string, approved bool, selectedHunks []string, opts ...DeferredResolution) error {
	res := DeferredResolution{
		Approved:        approved,
		SelectedHunkIDs: selectedHunks,
	}
	if len(opts) > 0 {
		res = opts[0]
		res.Approved = approved
		if len(selectedHunks) > 0 && len(res.SelectedHunkIDs) == 0 {
			res.SelectedHunkIDs = selectedHunks
		}
	}
	return s.ResolveDeferredCard(id, res)
}

// ResolveDeferredCard 处理挂起工具调用卡片的完整审批决议，并向 Inbox 投递高优先级 `deferred_resume` 恢复事件。
func (s *Service) ResolveDeferredCard(id string, res DeferredResolution) error {
	switch strings.ToLower(strings.TrimSpace(res.Decision)) {
	case "approve", "approved", "allow", "allowed", "accept", "accepted":
		res.Approved = true
	case "deny", "denied", "reject", "rejected":
		res.Approved = false
	}
	if err := store.ValidateSafeID(id); err != nil || strings.Contains(id, "..") {
		return fmt.Errorf("%w: invalid id %q", ErrDeferredNotFound, id)
	}
	safeID := id

	var card DeferredCard
	now := s.now().UTC()
	cardPath := filepath.Join(s.deferredDir(), safeID+".md")

	// Step 1: Read and validate pending deferred card WITHOUT removing it yet.
	err := store.WithFlock(s.deferredLockPath(), false, func() error {
		raw, rErr := os.ReadFile(cardPath)
		if rErr != nil {
			if errors.Is(rErr, os.ErrNotExist) {
				return fmt.Errorf("%w: %s", ErrDeferredNotFound, safeID)
			}
			return fmt.Errorf("read deferred card %s: %w", safeID, rErr)
		}
		parsed, pErr := parseDeferredCardMD(raw, safeID)
		if pErr != nil {
			return fmt.Errorf("parse deferred card %s: %w", safeID, pErr)
		}
		card = parsed
		return nil
	})
	if err != nil {
		return err
	}

	feedback := strings.TrimSpace(res.Feedback)

	// Step 2: Enqueue idempotent deferred_resume event FIRST (no nested locks) so a failed enqueue never loses the pending card.
	resume := core.DeferredResume{
		DeferredID:      card.ID,
		ToolCallID:      card.ToolCallID,
		ToolName:        card.ToolName,
		Args:            card.Args,
		Fingerprint:     card.Fingerprint,
		Approved:        res.Approved,
		Remember:        res.Remember,
		Feedback:        feedback,
		SelectedHunkIDs: res.SelectedHunkIDs,
	}
	resumeJSON, err := json.Marshal(resume)
	if err != nil {
		return fmt.Errorf("marshal deferred resume: %w", err)
	}
	var resumeMap map[string]any
	_ = json.Unmarshal(resumeJSON, &resumeMap)

	_, _, err = s.inbox.enqueue(InboxEvent{
		InboxKey:    fmt.Sprintf("resume:%s:%s", card.WorkspaceID, card.ID),
		Source:      SourceDeferredResume,
		SourceID:    card.ID,
		WorkspaceID: card.WorkspaceID,
		SessionID:   card.SessionID,
		AgentRole:   card.AgentRole,
		Payload: map[string]any{
			"deferred_id":          card.ID,
			"approved":             res.Approved,
			"remember":             res.Remember,
			"feedback":             feedback,
			"resume_approval":      resumeMap,
			"resume_approval_json": string(resumeJSON),
		},
		CreatedAt: now,
	}, now)
	if err != nil {
		return err
	}

	// If Remember is requested on an approved card with a known fingerprint, also persist immediately in GrantStore.
	if res.Approved && res.Remember && strings.TrimSpace(card.Fingerprint) != "" && s.engine.Grants() != nil {
		_ = s.engine.Grants().SaveGrant(card.WorkspaceID, card.Fingerprint, card.ToolName, "approved via deferred card")
	}

	// Step 3: Atomically archive resolved card to deferred/resolved/<id>.md, remove pending file, and wake worker.
	err = store.WithFlock(s.deferredLockPath(), true, func() error {
		status := "denied"
		if res.Approved {
			status = "approved"
		}
		card.Status = status
		card.Remember = res.Remember
		card.Feedback = feedback
		card.ResolvedAt = &now

		resolvedMD, fErr := formatDeferredCardMD(card, res.SelectedHunkIDs)
		if fErr != nil {
			return fErr
		}
		resolvedPath := filepath.Join(s.deferredResolvedDir(), safeID+".md")
		if wErr := store.AtomicWriteFile(resolvedPath, resolvedMD, 0o600); wErr != nil {
			return wErr
		}
		if rmErr := os.Remove(cardPath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			return rmErr
		}
		return nil
	})
	if err != nil {
		return err
	}

	s.signalWake()
	s.publishAlwaysOnEvent("deferred_resolved", map[string]any{
		"deferred_id": safeID,
		"approved":    res.Approved,
		"remember":    res.Remember,
	})
	return nil
}

func (s *Service) persistDeferredCard(def core.DeferredToolPayload, agentRole string) (DeferredCard, error) {
	card, _, err := s.persistDeferredCardIfNew(def, agentRole, true)
	return card, err
}

func (s *Service) persistDeferredCardIfNew(def core.DeferredToolPayload, agentRole string, overwrite bool) (DeferredCard, bool, error) {
	now := s.now().UTC()
	safeID := strings.TrimSpace(def.ID)
	if store.ValidateSafeID(safeID) != nil || strings.Contains(safeID, "..") {
		safeID = fmt.Sprintf("def_%d_%s", now.UnixMilli(), store.RandomHex(4))
	}

	card := DeferredCard{
		ID:          safeID,
		RunID:       def.RunID,
		WorkspaceID: def.WorkspaceID,
		SessionID:   def.SessionID,
		AgentRole:   agentRole,
		ToolCallID:  def.ToolCallID,
		ToolName:    def.ToolName,
		ReasonCode:  def.ReasonCode,
		Reason:      def.Reason,
		Fingerprint: def.Fingerprint,
		Args:        append(json.RawMessage(nil), def.Args...),
		Hunks:       append([]tools.DiffHunk(nil), def.Hunks...),
		Status:      "pending",
		CreatedAt:   now,
	}

	mdBytes, err := formatDeferredCardMD(card, nil)
	if err != nil {
		return DeferredCard{}, false, err
	}

	cardPath := filepath.Join(s.deferredDir(), safeID+".md")
	resolvedPath := filepath.Join(s.deferredResolvedDir(), safeID+".md")

	var created bool
	err = store.WithFlock(s.deferredLockPath(), true, func() error {
		if !overwrite {
			if raw, rErr := os.ReadFile(cardPath); rErr == nil {
				if existing, pErr := parseDeferredCardMD(raw, safeID); pErr == nil {
					card = existing
				}
				return nil
			}
			if _, rErr := os.Stat(resolvedPath); rErr == nil {
				return nil
			}
		} else {
			if _, sErr := os.Stat(cardPath); errors.Is(sErr, os.ErrNotExist) {
				if _, rsErr := os.Stat(resolvedPath); errors.Is(rsErr, os.ErrNotExist) {
					created = true
				}
			}
		}
		if wErr := store.AtomicWriteFile(cardPath, mdBytes, 0o600); wErr != nil {
			return wErr
		}
		if !overwrite {
			created = true
		}
		return nil
	})
	if err != nil {
		return DeferredCard{}, false, err
	}
	return card, created, nil
}

func (s *Service) recordDeferredAndNotifyIfNew(def core.DeferredToolPayload, agentRole string) (DeferredCard, bool, error) {
	card, created, err := s.persistDeferredCardIfNew(def, agentRole, false)
	if err != nil {
		return DeferredCard{}, false, err
	}
	if !created {
		return card, false, nil
	}

	title := fmt.Sprintf("Approval Needed: %s", card.ToolName)
	body := fmt.Sprintf("[%s] %s (card %s)", card.ReasonCode, card.Reason, card.ID)
	delivered := false
	if s.notifier != nil {
		if nErr := s.notifier.NotifyMac(title, s.workspaceLabel(card.WorkspaceID), body); nErr == nil {
			delivered = true
		}
	}
	_, _ = s.outbox.append(OutboxItem{
		WorkspaceID: card.WorkspaceID,
		SessionID:   card.SessionID,
		Outcome:     OutcomeDefer,
		Title:       title,
		Body:        body,
		Delivered:   delivered,
	}, s.now())
	s.publishAlwaysOnEvent("deferred_created", map[string]any{
		"deferred_id":  card.ID,
		"workspace_id": card.WorkspaceID,
		"session_id":   card.SessionID,
		"tool_name":    card.ToolName,
	})
	return card, true, nil
}

func (s *Service) StateSnapshot() AlwaysOnState {
	now := s.now().UTC()

	s.lifeMu.Lock()
	running := s.running
	s.lifeMu.Unlock()

	inboxItems, _ := s.inbox.list(now)
	timers, _ := s.timers.list()
	deferredCards, _ := s.ListDeferred()
	outboxItems, _ := s.outbox.list(100)
	candidates, _ := s.ListSkillCandidates()

	var queuedCnt, runningCnt, failedCnt int
	for _, ev := range inboxItems {
		switch ev.Status {
		case StatusQueued:
			queuedCnt++
		case StatusRunning:
			runningCnt++
		case StatusFailed:
			failedCnt++
		}
	}

	return AlwaysOnState{
		Running:         running,
		QueuedCount:     queuedCnt,
		RunningCount:    runningCnt,
		FailedCount:     failedCnt,
		DeferredCount:   len(deferredCards),
		CandidatesCount: len(candidates),
		Inbox:           inboxItems,
		Timers:          timers,
		Deferred:        deferredCards,
		Outbox:          outboxItems,
		Candidates:      candidates,
		UpdatedAt:       now,
	}
}

func (s *Service) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	s.lifeMu.Lock()
	if s.running {
		s.lifeMu.Unlock()
		return nil
	}

	unlock, ok, err := store.TryFlock(s.workerLeaseLockPath())
	if err != nil {
		s.lifeMu.Unlock()
		return err
	}
	if !ok {
		s.lifeMu.Unlock()
		return ErrWorkerLeaseHeld
	}

	s.lifeMu.Unlock()
	if err := s.mountPluginTools(); err != nil {
		unlock()
		return err
	}
	if _, rErr := s.inbox.recoverOnStart(s.now()); rErr != nil {
		unlock()
		return rErr
	}

	s.lifeMu.Lock()
	loopCtx, loopCancel := context.WithCancel(ctx)
	s.running = true
	s.stopCancel = loopCancel
	s.leaseUnlock = unlock
	s.wg.Add(1)
	s.lifeMu.Unlock()

	go func() {
		defer s.wg.Done()
		defer func() {
			s.lifeMu.Lock()
			s.running = false
			if s.leaseUnlock != nil {
				s.leaseUnlock()
				s.leaseUnlock = nil
			}
			s.stopCancel = nil
			s.lifeMu.Unlock()
		}()

		s.runWorkerLoop(loopCtx)
	}()

	s.signalWake()
	s.publishAlwaysOnEvent("worker_started", nil)
	return nil
}

func (s *Service) Close() error {
	s.lifeMu.Lock()
	cancel := s.stopCancel
	globalUnsub := s.globalUnsub
	s.globalUnsub = nil
	ownsMount := s.ownsPluginMount
	s.ownsPluginMount = false
	s.lifeMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if globalUnsub != nil {
		globalUnsub()
	}
	s.wg.Wait()

	if ownsMount && s.engine != nil {
		if hooks := s.engine.Hooks(); hooks != nil {
			_ = hooks.UnmountPlugin(PluginID)
		}
		s.engine.UnregisterPluginTools(memoryPluginToolNames...)
	}
	return nil
}

func (s *Service) currentDebounce() time.Duration {
	if snap := s.cfg.Current(); snap != nil {
		if snap.Global.InboxDebounceMS > 0 {
			return time.Duration(snap.Global.InboxDebounceMS) * time.Millisecond
		}
		if snap.Global.InboxDebounceMS == 0 {
			return 0
		}
	}
	return 400 * time.Millisecond
}

func (s *Service) runWorkerLoop(ctx context.Context) {
	timer := time.NewTimer(s.tickInterval)
	defer timer.Stop()

	var tickSeq uint64
	for {
		if ctx.Err() != nil {
			return
		}
		tickSeq++

		// 1. Move any due Follow-up / Cron timers into Inbox without invoking LLM directly.
		_, _ = s.TickTimers(s.now())

		// 2. Drain all currently ready micro-batches in Inbox.
		for {
			if ctx.Err() != nil {
				return
			}
			batch, nextWake, err := s.inbox.claimNextBatch(s.now(), s.currentDebounce(), s.retryBackoff)
			if err != nil {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(s.tickInterval)
				break
			}
			if len(batch) == 0 {
				// 3. When queue is idle, check periodic JSONL compaction & idle Auto-Dreamer threshold/cooldown.
				if tickSeq%8 == 0 {
					_, _ = s.outbox.maybeCompact()
					_, _ = s.inbox.maybeCompact(s.now())
				}
				s.maybeAutoDream(ctx)

				waitDur := s.tickInterval
				if nextWake > 0 && nextWake < waitDur {
					waitDur = nextWake
				}
				if waitDur < 2*time.Millisecond {
					waitDur = 2 * time.Millisecond
				}
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(waitDur)
				break
			}

			s.executeClaimedBatch(ctx, batch)
		}

		select {
		case <-ctx.Done():
			return
		case <-s.wakeCh:
		case <-timer.C:
		}
	}
}

func (s *Service) executeClaimedBatch(ctx context.Context, batch []InboxEvent) {
	if len(batch) == 0 {
		return
	}

	ids := make([]string, len(batch))
	for i, ev := range batch {
		ids[i] = ev.ID
	}

	head := batch[0]
	wsID := strings.TrimSpace(head.WorkspaceID)
	sessionID := strings.TrimSpace(head.SessionID)
	agentRole := strings.TrimSpace(head.AgentRole)
	if agentRole == "" {
		agentRole = "coder"
	}

	resumeReq, isResume := extractDeferredResume(head)
	if !isResume && sessionID != "" && s.hasPendingDeferredForSession(sessionID) {
		_ = s.inbox.requeueBusyBatch(ids, s.retryBackoff, s.now())
		return
	}

	autoCreatedSession := false
	if sessionID == "" {
		meta, cErr := s.engine.Sessions().Create(wsID, "Always-On Session", agentRole)
		if cErr != nil {
			_ = s.inbox.completeBatch(ids, "", StatusFailed, cErr.Error(), s.retryBackoff, s.now())
			return
		}
		sessionID = meta.ID
		autoCreatedSession = true
	}

	prompt := ""
	if !isResume {
		prompt = formatBatchPrompt(batch)
	}

	dynamicReminders := s.memory.buildUnattendedReminders(wsID)
	if isResume && resumeReq != nil && resumeReq.Approved && strings.TrimSpace(resumeReq.Feedback) != "" {
		dynamicReminders = append(dynamicReminders, "User approval steering feedback: "+sanitizeMemoryReminder(strings.TrimSpace(resumeReq.Feedback)))
	}

	capState := &runNotificationCapture{
		wsID:      wsID,
		sessionID: sessionID,
	}
	s.captureMu.Lock()
	s.captures[sessionID] = capState
	s.captureMu.Unlock()

	defer func() {
		s.captureMu.Lock()
		delete(s.captures, sessionID)
		s.captureMu.Unlock()
	}()

	_, evCh, startErr := s.engine.StartRun(ctx, core.RunRequest{
		WorkspaceID:    wsID,
		SessionID:      sessionID,
		Prompt:         prompt,
		AgentRole:      agentRole,
		Unattended:     true,
		DynamicContext: dynamicReminders,
		ResumeApproval: resumeReq,
	})
	if startErr != nil {
		if errors.Is(startErr, core.ErrSessionBusy) {
			_ = s.inbox.requeueBusyBatch(ids, s.retryBackoff, s.now())
			return
		}
		if autoCreatedSession && s.isTrivialFailedAutoSession(sessionID) {
			_ = s.engine.Sessions().Delete(sessionID)
		}
		_ = s.inbox.completeBatch(ids, "", StatusFailed, startErr.Error(), s.retryBackoff, s.now())
		return
	}

	var (
		deferredPayload *core.DeferredToolPayload
		runFailedErr    string
		finalText       string
	)

	for ev := range evCh {
		switch ev.Type {
		case core.EventToolDeferred:
			if p, ok := ev.Payload.(core.DeferredToolPayload); ok {
				cp := p
				deferredPayload = &cp
			} else if pPtr, ok := ev.Payload.(*core.DeferredToolPayload); ok && pPtr != nil {
				cp := *pPtr
				deferredPayload = &cp
			}
		case core.EventRunFailed:
			if p, ok := ev.Payload.(core.RunErrorPayload); ok {
				runFailedErr = p.Message
			} else {
				runFailedErr = "alwayson run failed"
			}
		case core.EventRunCancelled:
			if ctx.Err() != nil {
				_ = s.inbox.requeueBusyBatch(ids, s.retryBackoff, s.now())
				return
			}
			runFailedErr = "run cancelled"
		case core.EventRunCompleted:
			if out, ok := ev.Payload.(core.RunOutcome); ok {
				finalText = out.FinalText
				if out.Status == "deferred" && deferredPayload == nil && out.Deferred != nil {
					cp := *out.Deferred
					deferredPayload = &cp
				}
			}
		}
	}

	if deferredPayload != nil {
		if _, _, dErr := s.recordDeferredAndNotifyIfNew(*deferredPayload, agentRole); dErr != nil {
			_ = s.inbox.completeBatch(ids, sessionID, StatusFailed, dErr.Error(), s.retryBackoff, s.now())
			return
		}
		_ = s.inbox.completeBatch(ids, sessionID, StatusDeferred, "", s.retryBackoff, s.now())
		s.publishAlwaysOnEvent("batch_deferred", map[string]any{"session_id": sessionID})
		return
	}

	if runFailedErr != "" {
		s.captureMu.Lock()
		hasSideEffects := capState.notifiedCount > 0 || capState.suppressedCount > 0 || capState.scheduledFollowUpCount > 0
		s.captureMu.Unlock()

		willRetry := false
		for _, ev := range batch {
			if ev.Attempts < MaxAttempts {
				willRetry = true
				break
			}
		}

		recordedSession := sessionID
		if autoCreatedSession && willRetry {
			recordedSession = ""
			if !hasSideEffects && s.isTrivialFailedAutoSession(sessionID) {
				_ = s.engine.Sessions().Delete(sessionID)
			}
		}
		_ = s.inbox.completeBatch(ids, recordedSession, StatusFailed, runFailedErr, s.retryBackoff, s.now())
		s.publishAlwaysOnEvent("batch_failed", map[string]any{"session_id": recordedSession})
		return
	}

	s.captureMu.Lock()
	sentCount := capState.notifiedCount
	suppressedTitle := capState.lastSuppressed
	s.captureMu.Unlock()

	if sentCount == 0 {
		silentTitle := suppressedTitle
		silentBody := truncateString(collapseWhitespace(finalText), 240)
		_, _ = s.outbox.append(OutboxItem{
			WorkspaceID: wsID,
			SessionID:   sessionID,
			Outcome:     OutcomeSilent,
			Title:       silentTitle,
			Body:        silentBody,
			Delivered:   false,
		}, s.now())
	}

	_ = s.inbox.completeBatch(ids, sessionID, StatusDone, "", s.retryBackoff, s.now())
	s.publishAlwaysOnEvent("batch_completed", map[string]any{"session_id": sessionID})
}

func (s *Service) hasPendingDeferredForSession(sessionID string) bool {
	target := strings.TrimSpace(sessionID)
	if target == "" {
		return false
	}
	cards, err := s.ListDeferred()
	if err != nil {
		return false
	}
	for _, card := range cards {
		if strings.TrimSpace(card.SessionID) == target {
			return true
		}
	}
	return false
}

func (s *Service) isTrivialFailedAutoSession(sessionID string) bool {
	branch, err := s.engine.Sessions().LoadActiveBranch(sessionID)
	if err != nil {
		return false
	}
	if len(branch) == 0 {
		return true
	}
	if len(branch) == 1 && branch[0].Role == "user" {
		return true
	}
	return false
}

func extractDeferredResume(ev InboxEvent) (*core.DeferredResume, bool) {
	if ev.Payload == nil {
		return nil, false
	}
	if rawStr, ok := ev.Payload["resume_approval_json"].(string); ok && strings.TrimSpace(rawStr) != "" {
		var res core.DeferredResume
		if err := json.Unmarshal([]byte(rawStr), &res); err == nil && res.ToolName != "" {
			return &res, true
		}
	}
	if sub, ok := ev.Payload["resume_approval"]; ok && sub != nil {
		encoded, err := json.Marshal(sub)
		if err == nil {
			var res core.DeferredResume
			if uErr := json.Unmarshal(encoded, &res); uErr == nil && res.ToolName != "" {
				return &res, true
			}
		}
	}
	return nil, false
}

func formatBatchPrompt(batch []InboxEvent) string {
	if len(batch) == 1 {
		ev := batch[0]
		return fmt.Sprintf("[Always-On Event | source=%s | id=%s]\n%s", ev.Source, ev.ID, renderEventBody(ev))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[Always-On Coalesced Micro-Batch: %d events]\n", len(batch))
	for i, ev := range batch {
		fmt.Fprintf(&b, "%d. [source=%s | id=%s] %s\n", i+1, ev.Source, ev.ID, renderEventBody(ev))
	}
	return strings.TrimSpace(b.String())
}

func renderEventBody(ev InboxEvent) string {
	if len(ev.Payload) == 0 {
		return fmt.Sprintf("Event %s from %s", ev.ID, ev.Source)
	}
	for _, k := range []string{"text", "prompt", "message", "note"} {
		if s, ok := ev.Payload[k].(string); ok && strings.TrimSpace(s) != "" {
			if len(ev.Payload) == 1 {
				return strings.TrimSpace(s)
			}
			raw, _ := json.Marshal(ev.Payload)
			return strings.TrimSpace(s) + "\nPayload: " + string(raw)
		}
	}
	raw, _ := json.Marshal(ev.Payload)
	return string(raw)
}

func formatDeferredCardMD(card DeferredCard, selectedHunks []string) ([]byte, error) {
	argsStr := strings.TrimSpace(string(card.Args))
	if argsStr == "" {
		argsStr = "{}"
	}
	status := card.Status
	if status == "" {
		status = "pending"
	}

	fm := deferredFrontmatter{
		ID:              card.ID,
		RunID:           card.RunID,
		WorkspaceID:     card.WorkspaceID,
		SessionID:       card.SessionID,
		AgentRole:       card.AgentRole,
		ToolCallID:      card.ToolCallID,
		ToolName:        card.ToolName,
		ReasonCode:      card.ReasonCode,
		Reason:          card.Reason,
		Fingerprint:     card.Fingerprint,
		ArgsJSON:        argsStr,
		Hunks:           append([]tools.DiffHunk(nil), card.Hunks...),
		Status:          status,
		SelectedHunkIDs: selectedHunks,
		Remember:        card.Remember,
		Feedback:        card.Feedback,
		CreatedAt:       card.CreatedAt.UTC(),
		ResolvedAt:      card.ResolvedAt,
	}

	fmBytes, err := toml.Marshal(fm)
	if err != nil {
		return nil, fmt.Errorf("marshal deferred frontmatter: %w", err)
	}

	var doc bytes.Buffer
	doc.WriteString("+++\n")
	doc.Write(bytes.TrimRight(fmBytes, "\n"))
	doc.WriteString("\n+++\n\n")
	fmt.Fprintf(&doc, "# Deferred Tool Call: `%s`\n\n", card.ToolName)
	fmt.Fprintf(&doc, "- **Card ID**: `%s`\n", card.ID)
	fmt.Fprintf(&doc, "- **Workspace**: `%s`\n", card.WorkspaceID)
	fmt.Fprintf(&doc, "- **Session**: `%s`\n", card.SessionID)
	fmt.Fprintf(&doc, "- **Reason**: `[%s]` %s\n\n", card.ReasonCode, card.Reason)
	doc.WriteString("## Arguments\n\n```json\n")
	doc.WriteString(argsStr)
	doc.WriteString("\n```\n")

	if len(card.Hunks) > 0 {
		doc.WriteString("\n## Proposed Diff Hunks\n\n")
		for _, h := range card.Hunks {
			fmt.Fprintf(&doc, "### `%s` (%s)\n\n```diff\n%s\n```\n\n", h.ID, h.Header, h.Preview)
		}
	}

	return doc.Bytes(), nil
}

func parseDeferredCardMD(raw []byte, fallbackID string) (DeferredCard, error) {
	fm, _, err := config.ParseFrontmatterAs[deferredFrontmatter](string(raw))
	if err != nil {
		return DeferredCard{}, err
	}
	// Support legacy snake_case diff hunk frontmatter keys (`old_start`, etc.) if present.
	if len(fm.Hunks) > 0 && fm.Hunks[0].OldStart == 0 && fm.Hunks[0].NewStart == 0 && strings.Contains(string(raw), "old_start") {
		if meta, _, mErr := config.ParseFrontmatterMD(raw); mErr == nil {
			if jb, jErr := json.Marshal(meta); jErr == nil {
				var jsonFM deferredFrontmatter
				if json.Unmarshal(jb, &jsonFM) == nil && len(jsonFM.Hunks) == len(fm.Hunks) {
					fm.Hunks = jsonFM.Hunks
				}
			}
		}
	}
	return frontmatterToDeferredCard(fm, fallbackID), nil
}

func frontmatterToDeferredCard(fm deferredFrontmatter, fallbackID string) DeferredCard {
	id := strings.TrimSpace(fm.ID)
	if id == "" {
		id = fallbackID
	}
	var args json.RawMessage
	if strings.TrimSpace(fm.ArgsJSON) != "" {
		args = json.RawMessage(fm.ArgsJSON)
	} else {
		args = json.RawMessage(`{}`)
	}
	return DeferredCard{
		ID:          id,
		RunID:       fm.RunID,
		WorkspaceID: fm.WorkspaceID,
		SessionID:   fm.SessionID,
		AgentRole:   fm.AgentRole,
		ToolCallID:  fm.ToolCallID,
		ToolName:    fm.ToolName,
		ReasonCode:  fm.ReasonCode,
		Reason:      fm.Reason,
		Fingerprint: fm.Fingerprint,
		Args:        args,
		Hunks:       append([]tools.DiffHunk(nil), fm.Hunks...),
		Status:      fm.Status,
		Remember:    fm.Remember,
		Feedback:    fm.Feedback,
		CreatedAt:   fm.CreatedAt,
		ResolvedAt:  fm.ResolvedAt,
	}
}
