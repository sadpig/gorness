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

	defaultTickInterval = 250 * time.Millisecond
	defaultRetryBackoff = 25 * time.Millisecond
)

var (
	ErrWorkerLeaseHeld  = errors.New("alwayson worker lease is already held by another active consumer")
	ErrDeferredNotFound = errors.New("deferred card not found")
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
	CreatedAt   time.Time        `json:"created_at" toml:"created_at"`
	ResolvedAt  *time.Time       `json:"resolved_at,omitempty" toml:"resolved_at,omitempty"`
}

type deferredHunkRecord struct {
	ID          string `toml:"id"`
	Fingerprint string `toml:"fingerprint"`
	Header      string `toml:"header"`
	OldStart    int    `toml:"old_start"`
	OldLines    int    `toml:"old_lines"`
	NewStart    int    `toml:"new_start"`
	NewLines    int    `toml:"new_lines"`
	Preview     string `toml:"preview"`
}

type deferredFrontmatter struct {
	ID              string               `toml:"id"`
	RunID           string               `toml:"run_id"`
	WorkspaceID     string               `toml:"workspace_id"`
	SessionID       string               `toml:"session_id"`
	AgentRole       string               `toml:"agent_role,omitempty"`
	ToolCallID      string               `toml:"tool_call_id,omitempty"`
	ToolName        string               `toml:"tool_name"`
	ReasonCode      string               `toml:"reason_code"`
	Reason          string               `toml:"reason"`
	Fingerprint     string               `toml:"fingerprint,omitempty"`
	ArgsJSON        string               `toml:"args_json,omitempty"`
	Hunks           []deferredHunkRecord `toml:"hunks,omitempty"`
	Status          string               `toml:"status"`
	SelectedHunkIDs []string             `toml:"selected_hunk_ids,omitempty"`
	CreatedAt       time.Time            `toml:"created_at"`
	ResolvedAt      *time.Time           `toml:"resolved_at,omitempty"`
}

type AlwaysOnState struct {
	Running       bool           `json:"running"`
	QueuedCount   int            `json:"queued_count"`
	RunningCount  int            `json:"running_count"`
	FailedCount   int            `json:"failed_count"`
	DeferredCount int            `json:"deferred_count"`
	Inbox         []InboxEvent   `json:"inbox"`
	Timers        []TimerEntry   `json:"timers"`
	Deferred      []DeferredCard `json:"deferred"`
	Outbox        []OutboxItem   `json:"outbox"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type ServiceOptions struct {
	Config       *config.Manager
	Engine       *core.Engine
	Notifier     Notifier
	Now          func() time.Time
	TickInterval time.Duration
	RetryBackoff time.Duration
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

	inbox  *inboxStore
	timers *timerStore
	memory *memoryStore
	outbox *outboxStore

	wakeCh chan struct{}

	captureMu sync.Mutex
	captures  map[string]*runNotificationCapture // sessionID -> active Always-On run capture

	lifeMu      sync.Mutex
	running     bool
	stopCancel  context.CancelFunc
	leaseUnlock func()
	wg          sync.WaitGroup
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

	paths := opts.Config.Paths()
	for _, dir := range []string{
		paths.AlwaysOnDir(),
		filepath.Join(paths.AlwaysOnDir(), "deferred"),
		filepath.Join(paths.AlwaysOnDir(), "deferred", "resolved"),
		paths.MemoryDir(),
		filepath.Join(paths.MemoryDir(), "notes"),
		filepath.Join(paths.RuntimeDir(), "locks"),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create alwayson dir %s: %w", dir, err)
		}
	}

	svc := &Service{
		cfg:          opts.Config,
		engine:       opts.Engine,
		paths:        paths,
		notifier:     notifier,
		now:          nowFn,
		tickInterval: tickInt,
		retryBackoff: retryBackoff,
		inbox:        newInboxStore(paths),
		timers:       newTimerStore(paths),
		memory:       newMemoryStore(paths),
		outbox:       newOutboxStore(paths),
		wakeCh:       make(chan struct{}, 1),
		captures:     make(map[string]*runNotificationCapture),
	}

	if !svc.isPluginMounted() {
		if err := svc.mountPluginTools(); err != nil {
			return nil, err
		}
	}

	return svc, nil
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
	if hooks == nil {
		return nil
	}
	for _, id := range hooks.MountedPlugins() {
		if id == PluginID {
			_ = hooks.UnmountPlugin(PluginID)
			break
		}
	}

	return hooks.MountPlugin(core.Plugin{
		ID: PluginID,
		Setup: func(sc *core.Scope) error {
			sc.RegisterTool(s.makeSetFollowUpTool())
			sc.RegisterTool(s.makeNotifyUserTool())
			return nil
		},
	})
}

func (s *Service) makeSetFollowUpTool() tools.Tool {
	return tools.Tool{
		Name:        "set_followup",
		Description: "Schedule a durable Always-On follow-up alarm in timers.toml to re-awaken this session or workspace after a delay.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"after_seconds":{"type":"integer","description":"Delay in seconds before follow-up fires"},"after":{"type":"string","description":"Optional Go duration string such as '5m' or '1h'"},"note":{"type":"string","description":"Reminder note for the follow-up wake"}},"required":["note"]}`),
		ReadOnly:    false,
		Mutating:    false, // Local harness scheduling metadata (safe in auto/accept_edits unattended runs)
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
		Name:        "notify_user",
		Description: "Deliver an actionable alert to the Gorness Outbox and macOS Notification Center, automatically deduplicating findings already recorded in REPORTED.md.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"title":{"type":"string","description":"Concise alert title"},"body":{"type":"string","description":"Actionable details for the user"}},"required":["title","body"]}`),
		ReadOnly:    false,
		Mutating:    false, // Local harness notification metadata (allowed in unattended auto runs)
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

			if _, err := s.outbox.append(OutboxItem{
				WorkspaceID: wsID,
				SessionID:   call.SessionID,
				Outcome:     OutcomeSend,
				Title:       title,
				Body:        body,
				Delivered:   delivered,
			}, s.now()); err != nil {
				return tools.ToolResult{}, err
			}

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
	_, deduped, err := s.inbox.enqueue(ev, s.now())
	if err != nil {
		return false, err
	}
	if !deduped {
		s.signalWake()
	}
	return deduped, nil
}

func (s *Service) ScheduleFollowUp(wsID, sessionID string, after time.Duration, note string) (*TimerEntry, error) {
	entry, err := s.timers.scheduleFollowUp(wsID, sessionID, after, note, s.now())
	if err != nil {
		return nil, err
	}
	s.signalWake()
	return entry, nil
}

func (s *Service) UpsertCron(entry TimerEntry) error {
	if err := s.timers.upsertCron(entry, s.now()); err != nil {
		return err
	}
	s.signalWake()
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
	}
	return enqueued, nil
}

func (s *Service) ListOutbox(limit int) ([]OutboxItem, error) {
	return s.outbox.list(limit)
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

func (s *Service) ResolveDeferred(id string, approved bool, selectedHunks []string) error {
	safeID := sanitizeDeferredID(id)
	if safeID == "" {
		return fmt.Errorf("%w: invalid id %q", ErrDeferredNotFound, id)
	}

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

	// Step 2: Enqueue idempotent deferred_resume event FIRST (no nested locks) so a failed enqueue never loses the pending card.
	resume := core.DeferredResume{
		DeferredID:      card.ID,
		ToolCallID:      card.ToolCallID,
		ToolName:        card.ToolName,
		Args:            card.Args,
		Fingerprint:     card.Fingerprint,
		Approved:        approved,
		SelectedHunkIDs: selectedHunks,
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
			"approved":             approved,
			"resume_approval":      resumeMap,
			"resume_approval_json": string(resumeJSON),
		},
		CreatedAt: now,
	}, now)
	if err != nil {
		return err
	}

	// Step 3: Atomically archive resolved card to deferred/resolved/<id>.md, remove pending file, and wake worker.
	err = store.WithFlock(s.deferredLockPath(), true, func() error {
		status := "denied"
		if approved {
			status = "approved"
		}
		card.Status = status
		card.ResolvedAt = &now

		resolvedMD, fErr := formatDeferredCardMD(card, selectedHunks)
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
	return nil
}

func (s *Service) persistDeferredCard(def core.DeferredToolPayload, agentRole string) (DeferredCard, error) {
	now := s.now().UTC()
	safeID := sanitizeDeferredID(def.ID)
	if safeID == "" {
		safeID = fmt.Sprintf("def_%d_%s", now.UnixMilli(), randomHex(4))
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
		return DeferredCard{}, err
	}

	cardPath := filepath.Join(s.deferredDir(), safeID+".md")
	err = store.WithFlock(s.deferredLockPath(), true, func() error {
		return store.AtomicWriteFile(cardPath, mdBytes, 0o600)
	})
	if err != nil {
		return DeferredCard{}, err
	}
	return card, nil
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
		Running:       running,
		QueuedCount:   queuedCnt,
		RunningCount:  runningCnt,
		FailedCount:   failedCnt,
		DeferredCount: len(deferredCards),
		Inbox:         inboxItems,
		Timers:        timers,
		Deferred:      deferredCards,
		Outbox:        outboxItems,
		UpdatedAt:     now,
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

	if err := s.mountPluginTools(); err != nil {
		unlock()
		s.lifeMu.Unlock()
		return err
	}

	if _, rErr := s.inbox.recoverOnStart(s.now()); rErr != nil {
		unlock()
		s.lifeMu.Unlock()
		return rErr
	}

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
	return nil
}

func (s *Service) Close() error {
	s.lifeMu.Lock()
	cancel := s.stopCancel
	s.lifeMu.Unlock()

	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
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

	for {
		if ctx.Err() != nil {
			return
		}

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

	// Pre-create session when unset so notify_user / set_followup capture state is race-free before StartRun begins.
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

	dynamicReminders := s.memory.buildDynamicReminders(wsID)

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
				// Daemon shutting down: restore queued state without burning attempts.
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
		card, dErr := s.persistDeferredCard(*deferredPayload, agentRole)
		if dErr != nil {
			_ = s.inbox.completeBatch(ids, sessionID, StatusFailed, dErr.Error(), s.retryBackoff, s.now())
			return
		}

		title := fmt.Sprintf("Approval Needed: %s", card.ToolName)
		body := fmt.Sprintf("[%s] %s (card %s)", card.ReasonCode, card.Reason, card.ID)
		delivered := false
		if s.notifier != nil {
			if nErr := s.notifier.NotifyMac(title, s.workspaceLabel(wsID), body); nErr == nil {
				delivered = true
			}
		}
		_, _ = s.outbox.append(OutboxItem{
			WorkspaceID: wsID,
			SessionID:   sessionID,
			Outcome:     OutcomeDefer,
			Title:       title,
			Body:        body,
			Delivered:   delivered,
		}, s.now())

		_ = s.inbox.completeBatch(ids, sessionID, StatusDeferred, "", s.retryBackoff, s.now())
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
			// Clear recordedSession so retry creates a fresh session rather than re-appending the prompt.
			recordedSession = ""
			// Only garbage-collect the failed session if no assistant/tool entries or side effects occurred.
			if !hasSideEffects && s.isTrivialFailedAutoSession(sessionID) {
				_ = s.engine.Sessions().Delete(sessionID)
			}
		}
		_ = s.inbox.completeBatch(ids, recordedSession, StatusFailed, runFailedErr, s.retryBackoff, s.now())
		return
	}

	s.captureMu.Lock()
	sentCount := capState.notifiedCount
	suppressedTitle := capState.lastSuppressed
	s.captureMu.Unlock()

	// Silent-by-default Outbox contract: when nothing new required user notification, record Outcome="silent".
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
	hunkRecs := make([]deferredHunkRecord, len(card.Hunks))
	for i, h := range card.Hunks {
		hunkRecs[i] = deferredHunkRecord{
			ID:          h.ID,
			Fingerprint: h.Fingerprint,
			Header:      h.Header,
			OldStart:    h.OldStart,
			OldLines:    h.OldLines,
			NewStart:    h.NewStart,
			NewLines:    h.NewLines,
			Preview:     h.Preview,
		}
	}
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
		Hunks:           hunkRecs,
		Status:          status,
		SelectedHunkIDs: selectedHunks,
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
	cleaned := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if strings.HasPrefix(cleaned, "+++\n") {
		rest := strings.TrimPrefix(cleaned, "+++\n")
		if closeIdx := strings.Index(rest, "\n+++"); closeIdx >= 0 {
			header := rest[:closeIdx]
			var fm deferredFrontmatter
			if err := toml.Unmarshal([]byte(header), &fm); err == nil {
				return frontmatterToDeferredCard(fm, fallbackID), nil
			}
		}
	}

	meta, _, err := config.ParseFrontmatterMD(raw)
	if err != nil {
		return DeferredCard{}, err
	}
	encoded, err := toml.Marshal(meta)
	if err != nil {
		return DeferredCard{}, err
	}
	var fm deferredFrontmatter
	if err := toml.Unmarshal(encoded, &fm); err != nil {
		return DeferredCard{}, err
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
	hunks := make([]tools.DiffHunk, len(fm.Hunks))
	for i, hr := range fm.Hunks {
		hunks[i] = tools.DiffHunk{
			ID:          hr.ID,
			Fingerprint: hr.Fingerprint,
			Header:      hr.Header,
			OldStart:    hr.OldStart,
			OldLines:    hr.OldLines,
			NewStart:    hr.NewStart,
			NewLines:    hr.NewLines,
			Preview:     hr.Preview,
		}
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
		Hunks:       hunks,
		Status:      fm.Status,
		CreatedAt:   fm.CreatedAt,
		ResolvedAt:  fm.ResolvedAt,
	}
}

func sanitizeDeferredID(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "." || trimmed == ".." {
		return ""
	}
	var b strings.Builder
	for _, r := range trimmed {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}
