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

var standardCronParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

type TimerEntry struct {
	ID          string    `toml:"id" json:"id"`
	WorkspaceID string    `toml:"workspace_id" json:"workspace_id"`
	SessionID   string    `toml:"session_id,omitempty" json:"session_id,omitempty"`
	Kind        string    `toml:"kind" json:"kind"`
	CronExpr    string    `toml:"cron_expr,omitempty" json:"cron_expr,omitempty"`
	Note        string    `toml:"note,omitempty" json:"note,omitempty"`
	DueAt       time.Time `toml:"due_at" json:"due_at"`
	Enabled     bool      `toml:"enabled" json:"enabled"`
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
		ID:          fmt.Sprintf("fup_%d_%s", now.UTC().UnixMilli(), randomHex(4)),
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
		entry.ID = fmt.Sprintf("cron_%d_%s", now.UTC().UnixMilli(), randomHex(4))
	}
	entry.WorkspaceID = cleanWS
	entry.SessionID = strings.TrimSpace(entry.SessionID)
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

// tickDue 扫描到期定时器，先通过 enqueueFn 幂等投递至 Inbox（绝不直接调用 LLM），再原子推进/禁用 timers.toml。
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

		ev := InboxEvent{
			InboxKey:    key,
			Source:      "timer:" + kind,
			SourceID:    fmt.Sprintf("%s@%d", e.ID, e.DueAt.UTC().UnixNano()),
			WorkspaceID: e.WorkspaceID,
			SessionID:   e.SessionID,
			AgentRole:   "coder",
			Payload: map[string]any{
				"timer_id":  e.ID,
				"kind":      kind,
				"cron_expr": e.CronExpr,
				"note":      e.Note,
				"text":      formatTimerNoteText(kind, e.ID, e.Note),
				"due_at":    e.DueAt.UTC().Format(time.RFC3339Nano),
			},
			CreatedAt: nowUTC,
		}

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
		for i := range entries {
			firedDue, hit := firedMap[entries[i].ID]
			if !hit || !entries[i].Enabled {
				continue
			}
			// Guard against racing external edit that already pushed DueAt into the future.
			if !entries[i].DueAt.IsZero() && entries[i].DueAt.After(nowUTC) {
				continue
			}
			_ = firedDue
			changed = true
			if entries[i].Kind == TimerKindCron || strings.TrimSpace(entries[i].CronExpr) != "" {
				sched, pErr := ParseCronSchedule(entries[i].CronExpr)
				if pErr != nil {
					entries[i].Enabled = false
				} else {
					entries[i].DueAt = sched.Next(nowUTC)
					entries[i].Enabled = true
				}
			} else {
				entries[i].Enabled = false
			}
		}
		if !changed {
			return nil
		}
		return s.writeUnlocked(entries)
	})
	if err != nil {
		return nil, err
	}

	return enqueuedEvents, nil
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
