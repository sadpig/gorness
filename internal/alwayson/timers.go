package alwayson

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/robfig/cron/v3"
	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/store"
)

const (
	TimerKindFollowUp = "followup"
	TimerKindCron     = "cron"
)

var ErrTimerNotFound = errors.New("timer not found")

var standardCronParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

type TimerEntry struct {
	ID          string     `toml:"id" json:"id"`
	WorkspaceID string     `toml:"workspace_id" json:"workspace_id"`
	SessionID   string     `toml:"session_id,omitempty" json:"session_id,omitempty"`
	AgentRole   string     `toml:"agent_role,omitempty" json:"agent_role,omitempty"`
	Kind        string     `toml:"kind" json:"kind"`
	CronExpr    string     `toml:"cron_expr,omitempty" json:"cron_expr,omitempty"`
	Note        string     `toml:"note,omitempty" json:"note,omitempty"`
	DueAt       time.Time  `toml:"due_at" json:"due_at"`
	LastFiredAt *time.Time `toml:"last_fired_at,omitempty" json:"last_fired_at,omitempty"`
	Enabled     bool       `toml:"enabled" json:"enabled"`
}

// TimerPatch 支持对现有定时器的局部更新（启用/禁用、Cron 表达式、备注、角色、延迟重设等）。
type TimerPatch struct {
	CronExpr     *string    `json:"cron_expr,omitempty" toml:"cron_expr,omitempty"`
	Note         *string    `json:"note,omitempty" toml:"note,omitempty"`
	AgentRole    *string    `json:"agent_role,omitempty" toml:"agent_role,omitempty"`
	SessionID    *string    `json:"session_id,omitempty" toml:"session_id,omitempty"`
	Enabled      *bool      `json:"enabled,omitempty" toml:"enabled,omitempty"`
	AfterSeconds *int       `json:"after_seconds,omitempty" toml:"after_seconds,omitempty"`
	DueAt        *time.Time `json:"due_at,omitempty" toml:"due_at,omitempty"`
}

type timersDoc struct {
	Timers []TimerEntry `toml:"timers"`
}

type timerStore struct {
	paths config.VaultPaths
}

func newTimerStore(paths config.VaultPaths) *timerStore {
	return &timerStore{paths: paths}
}

func (s *timerStore) lockPath() string {
	return filepath.Join(s.paths.RuntimeDir(), "locks", "alwayson-timers.lock")
}

func (s *timerStore) filePath() string {
	return filepath.Join(s.paths.AlwaysOnDir(), "timers.toml")
}

func ParseCronSchedule(expr string) (cron.Schedule, error) {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "" {
		return nil, errors.New("cron_expr is required")
	}
	sched, err := standardCronParser.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression %q: %w", trimmed, err)
	}
	return sched, nil
}

func (s *timerStore) scheduleFollowUp(
	wsID, sessionID string,
	after time.Duration,
	note string,
	now time.Time,
) (*TimerEntry, error) {
	cleanWS := strings.TrimSpace(wsID)
	if cleanWS == "" {
		return nil, errors.New("workspace_id is required for follow-up timer")
	}

	entry := TimerEntry{
		ID:          fmt.Sprintf("fup_%d_%s", now.UTC().UnixMilli(), store.RandomHex(4)),
		WorkspaceID: cleanWS,
		SessionID:   strings.TrimSpace(sessionID),
		Kind:        TimerKindFollowUp,
		Note:        strings.TrimSpace(note),
		DueAt:       now.UTC().Add(after),
		Enabled:     true,
	}

	err := store.WithFlock(s.lockPath(), true, func() error {
		entries, err := s.readUnlocked()
		if err != nil {
			return err
		}
		entries = append(entries, entry)
		return s.writeUnlocked(entries)
	})
	if err != nil {
		return nil, err
	}
	cp := entry
	return &cp, nil
}

func (s *timerStore) upsertCron(entry TimerEntry, now time.Time) error {
	cleanWS := strings.TrimSpace(entry.WorkspaceID)
	if cleanWS == "" {
		return errors.New("workspace_id is required for cron timer")
	}
	sched, err := ParseCronSchedule(entry.CronExpr)
	if err != nil {
		return err
	}

	entry.ID = strings.TrimSpace(entry.ID)
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("cron_%d_%s", now.UTC().UnixMilli(), store.RandomHex(4))
	} else if err := store.ValidateSafeID(entry.ID); err != nil {
		return err
	}
	entry.WorkspaceID = cleanWS
	entry.SessionID = strings.TrimSpace(entry.SessionID)
	entry.AgentRole = strings.TrimSpace(entry.AgentRole)
	entry.Kind = TimerKindCron
	entry.CronExpr = strings.TrimSpace(entry.CronExpr)
	entry.Note = strings.TrimSpace(entry.Note)
	if entry.DueAt.IsZero() {
		entry.DueAt = sched.Next(now.UTC())
	} else {
		entry.DueAt = entry.DueAt.UTC()
	}

	return store.WithFlock(s.lockPath(), true, func() error {
		entries, err := s.readUnlocked()
		if err != nil {
			return err
		}
		replaced := false
		for i := range entries {
			if entries[i].ID == entry.ID {
				entries[i] = entry
				replaced = true
				break
			}
		}
		if !replaced {
			entries = append(entries, entry)
		}
		return s.writeUnlocked(entries)
	})
}

func (s *timerStore) update(id string, patch TimerPatch, now time.Time) (TimerEntry, error) {
	if err := store.ValidateSafeID(id); err != nil {
		return TimerEntry{}, err
	}
	nowUTC := now.UTC()
	var updated TimerEntry

	err := store.WithFlock(s.lockPath(), true, func() error {
		entries, err := s.readUnlocked()
		if err != nil {
			return err
		}
		idx := -1
		for i := range entries {
			if entries[i].ID == id {
				idx = i
				break
			}
		}
		if idx < 0 {
			return fmt.Errorf("%w: %s", ErrTimerNotFound, id)
		}

		cur := entries[idx]
		if patch.Note != nil {
			cur.Note = strings.TrimSpace(*patch.Note)
		}
		if patch.AgentRole != nil {
			cur.AgentRole = strings.TrimSpace(*patch.AgentRole)
		}
		if patch.SessionID != nil {
			cur.SessionID = strings.TrimSpace(*patch.SessionID)
		}
		if patch.CronExpr != nil {
			expr := strings.TrimSpace(*patch.CronExpr)
			sched, pErr := ParseCronSchedule(expr)
			if pErr != nil {
				return pErr
			}
			cur.Kind = TimerKindCron
			cur.CronExpr = expr
			if patch.DueAt == nil && patch.AfterSeconds == nil {
				cur.DueAt = sched.Next(nowUTC)
			}
		}
		if patch.AfterSeconds != nil {
			cur.DueAt = nowUTC.Add(time.Duration(*patch.AfterSeconds) * time.Second)
		}
		if patch.DueAt != nil && !patch.DueAt.IsZero() {
			cur.DueAt = patch.DueAt.UTC()
		}
		if patch.Enabled != nil {
			wasDisabled := !cur.Enabled
			cur.Enabled = *patch.Enabled
			if cur.Enabled && wasDisabled && patch.DueAt == nil && patch.AfterSeconds == nil &&
				(cur.Kind == TimerKindCron || strings.TrimSpace(cur.CronExpr) != "") &&
				(cur.DueAt.IsZero() || !cur.DueAt.After(nowUTC)) {
				if sched, pErr := ParseCronSchedule(cur.CronExpr); pErr == nil {
					cur.DueAt = sched.Next(nowUTC)
				}
			}
		}

		entries[idx] = cur
		updated = cur
		return s.writeUnlocked(entries)
	})
	return updated, err
}

func (s *timerStore) delete(id string) error {
	if err := store.ValidateSafeID(id); err != nil {
		return err
	}
	return store.WithFlock(s.lockPath(), true, func() error {
		entries, err := s.readUnlocked()
		if err != nil {
			return err
		}
		kept := make([]TimerEntry, 0, len(entries))
		found := false
		for _, e := range entries {
			if e.ID == id {
				found = true
				continue
			}
			kept = append(kept, e)
		}
		if !found {
			return fmt.Errorf("%w: %s", ErrTimerNotFound, id)
		}
		return s.writeUnlocked(kept)
	})
}

func (s *timerStore) triggerNow(
	id string,
	now time.Time,
	enqueueFn func(InboxEvent) (InboxEvent, bool, error),
) (InboxEvent, error) {
	if err := store.ValidateSafeID(id); err != nil {
		return InboxEvent{}, err
	}
	nowUTC := now.UTC()
	var target TimerEntry
	err := store.WithFlock(s.lockPath(), false, func() error {
		entries, rErr := s.readUnlocked()
		if rErr != nil {
			return rErr
		}
		for _, e := range entries {
			if e.ID == id {
				target = e
				return nil
			}
		}
		return fmt.Errorf("%w: %s", ErrTimerNotFound, id)
	})
	if err != nil {
		return InboxEvent{}, err
	}

	ev := makeTimerInboxEvent(target, nowUTC, true)
	stored, _, enqErr := enqueueFn(ev)
	if enqErr != nil {
		return InboxEvent{}, enqErr
	}

	_ = store.WithFlock(s.lockPath(), true, func() error {
		entries, rErr := s.readUnlocked()
		if rErr != nil {
			return rErr
		}
		kept := make([]TimerEntry, 0, len(entries))
		changed := false
		for _, e := range entries {
			if e.ID != id {
				kept = append(kept, e)
				continue
			}
			changed = true
			if e.Kind == TimerKindCron || strings.TrimSpace(e.CronExpr) != "" {
				firedAt := nowUTC
				e.LastFiredAt = &firedAt
				kept = append(kept, e)
			}
			// One-shot follow-up timer triggered manually is auto-removed cleanly.
		}
		if !changed {
			return nil
		}
		return s.writeUnlocked(kept)
	})

	return stored, nil
}

func (s *timerStore) list() ([]TimerEntry, error) {
	var out []TimerEntry
	err := store.WithFlock(s.lockPath(), false, func() error {
		entries, err := s.readUnlocked()
		if err != nil {
			return err
		}
		out = entries
		return nil
	})
	return out, err
}

// tickDue 扫描到期定时器，先通过 enqueueFn 幂等投递至 Inbox（绝不直接调用 LLM），再原子推进 Cron 并清理已触发的一次性 Follow-up。
func (s *timerStore) tickDue(
	now time.Time,
	enqueueFn func(InboxEvent) (InboxEvent, bool, error),
) ([]InboxEvent, error) {
	nowUTC := now.UTC()
	var dueSnapshot []TimerEntry

	err := store.WithFlock(s.lockPath(), true, func() error {
		entries, err := s.readUnlocked()
		if err != nil {
			return err
		}
		initializedMissingDue := false
		for i := range entries {
			e := &entries[i]
			if !e.Enabled || strings.TrimSpace(e.WorkspaceID) == "" {
				continue
			}
			if e.DueAt.IsZero() && (e.Kind == TimerKindCron || strings.TrimSpace(e.CronExpr) != "") {
				if sched, pErr := ParseCronSchedule(e.CronExpr); pErr == nil {
					e.DueAt = sched.Next(nowUTC)
					initializedMissingDue = true
				}
			}
			if !e.DueAt.IsZero() && !e.DueAt.After(nowUTC) {
				cp := *e
				cp.DueAt = cp.DueAt.UTC()
				dueSnapshot = append(dueSnapshot, cp)
			}
		}
		if initializedMissingDue {
			if wErr := s.writeUnlocked(entries); wErr != nil {
				return wErr
			}
		}
		return nil
	})
	if err != nil || len(dueSnapshot) == 0 {
		return nil, err
	}

	type firedResult struct {
		timerID string
		dueAt   time.Time
		stored  InboxEvent
		added   bool
	}
	var fired []firedResult

	for _, e := range dueSnapshot {
		ev := makeTimerInboxEvent(e, nowUTC, false)
		stored, deduped, enqErr := enqueueFn(ev)
		if enqErr != nil {
			return nil, enqErr
		}
		fired = append(fired, firedResult{
			timerID: e.ID,
			dueAt:   e.DueAt.UTC(),
			stored:  stored,
			added:   !deduped,
		})
	}

	firedMap := make(map[string]time.Time, len(fired))
	var enqueuedEvents []InboxEvent
	for _, f := range fired {
		firedMap[f.timerID] = f.dueAt
		if f.added {
			enqueuedEvents = append(enqueuedEvents, f.stored)
		}
	}

	err = store.WithFlock(s.lockPath(), true, func() error {
		entries, err := s.readUnlocked()
		if err != nil {
			return err
		}
		changed := false
		kept := make([]TimerEntry, 0, len(entries))
		for _, cur := range entries {
			_, hit := firedMap[cur.ID]
			if !hit || !cur.Enabled {
				// Also prune any legacy disabled one-shot follow-up entries left on disk.
				if !cur.Enabled && cur.Kind != TimerKindCron && strings.TrimSpace(cur.CronExpr) == "" {
					changed = true
					continue
				}
				kept = append(kept, cur)
				continue
			}
			// Guard against racing external edit that already pushed DueAt into the future.
			if !cur.DueAt.IsZero() && cur.DueAt.After(nowUTC) {
				kept = append(kept, cur)
				continue
			}
			changed = true
			if cur.Kind == TimerKindCron || strings.TrimSpace(cur.CronExpr) != "" {
				firedAt := nowUTC
				cur.LastFiredAt = &firedAt
				sched, pErr := ParseCronSchedule(cur.CronExpr)
				if pErr != nil {
					cur.Enabled = false
				} else {
					cur.DueAt = sched.Next(nowUTC)
					cur.Enabled = true
				}
				kept = append(kept, cur)
			}
			// Fired one-shot follow-up timers are auto-removed cleanly instead of accumulating.
		}
		if !changed {
			return nil
		}
		return s.writeUnlocked(kept)
	})
	if err != nil {
		return nil, err
	}

	return enqueuedEvents, nil
}

func makeTimerInboxEvent(e TimerEntry, nowUTC time.Time, manual bool) InboxEvent {
	kind := strings.TrimSpace(e.Kind)
	if kind == "" {
		if strings.TrimSpace(e.CronExpr) != "" {
			kind = TimerKindCron
		} else {
			kind = TimerKindFollowUp
		}
	}
	key := fmt.Sprintf("%s:timer:%s", e.WorkspaceID, e.ID)
	if e.SessionID != "" {
		key = fmt.Sprintf("%s:%s", e.WorkspaceID, e.SessionID)
	}
	role := strings.TrimSpace(e.AgentRole)
	if role == "" {
		role = "coder"
	}
	source := "timer:" + kind
	sourceID := fmt.Sprintf("%s@%d", e.ID, e.DueAt.UTC().UnixNano())
	if manual {
		source = "timer:manual"
		sourceID = fmt.Sprintf("%s@manual@%d_%s", e.ID, nowUTC.UnixNano(), store.RandomHex(3))
	}
	return InboxEvent{
		InboxKey:    key,
		Source:      source,
		SourceID:    sourceID,
		WorkspaceID: e.WorkspaceID,
		SessionID:   e.SessionID,
		AgentRole:   role,
		Payload: map[string]any{
			"timer_id":  e.ID,
			"kind":      kind,
			"cron_expr": e.CronExpr,
			"note":      e.Note,
			"manual":    manual,
			"text":      formatTimerNoteText(kind, e.ID, e.Note),
			"due_at":    e.DueAt.UTC().Format(time.RFC3339Nano),
		},
		CreatedAt: nowUTC,
	}
}

func (s *timerStore) readUnlocked() ([]TimerEntry, error) {
	raw, err := os.ReadFile(s.filePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read timers.toml: %w", err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	var doc timersDoc
	if err := toml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse timers.toml: %w", err)
	}
	return doc.Timers, nil
}

func (s *timerStore) writeUnlocked(entries []TimerEntry) error {
	encoded, err := toml.Marshal(timersDoc{Timers: entries})
	if err != nil {
		return fmt.Errorf("marshal timers.toml: %w", err)
	}
	return store.AtomicWriteFile(s.filePath(), encoded, 0o600)
}

func formatTimerNoteText(kind, id, note string) string {
	trimmed := strings.TrimSpace(note)
	if trimmed == "" {
		return fmt.Sprintf("Scheduled %s timer (%s) fired.", kind, id)
	}
	return fmt.Sprintf("Scheduled %s timer (%s): %s", kind, id, trimmed)
}
