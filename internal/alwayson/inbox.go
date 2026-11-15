package alwayson

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pelletier/go-toml/v2"
	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/store"
)

const (
	MaxAttempts = 3

	StatusQueued   = "queued"
	StatusRunning  = "running"
	StatusDone     = "done"
	StatusFailed   = "failed"
	StatusDeferred = "deferred"

	SourceDeferredResume = "deferred_resume"
)

var idFallbackSeq atomic.Uint64

type InboxEvent struct {
	ID          string         `json:"id" toml:"id"`
	InboxKey    string         `json:"inbox_key" toml:"inbox_key"`
	Source      string         `json:"source" toml:"source"`
	SourceID    string         `json:"source_id,omitempty" toml:"source_id,omitempty"`
	WorkspaceID string         `json:"workspace_id" toml:"workspace_id"`
	SessionID   string         `json:"session_id,omitempty" toml:"session_id,omitempty"`
	AgentRole   string         `json:"agent_role,omitempty" toml:"agent_role,omitempty"`
	Payload     map[string]any `json:"payload,omitempty" toml:"payload,omitempty"`
	Status      string         `json:"status" toml:"status"`
	Attempts    int            `json:"attempts" toml:"attempts"`
	CreatedAt   time.Time      `json:"created_at" toml:"created_at"`
}

type inboxItemState struct {
	ID          string    `toml:"id" json:"id"`
	InboxKey    string    `toml:"inbox_key" json:"inbox_key"`
	Source      string    `toml:"source" json:"source"`
	SourceID    string    `toml:"source_id,omitempty" json:"source_id,omitempty"`
	WorkspaceID string    `toml:"workspace_id" json:"workspace_id"`
	SessionID   string    `toml:"session_id,omitempty" json:"session_id,omitempty"`
	AgentRole   string    `toml:"agent_role,omitempty" json:"agent_role,omitempty"`
	Status      string    `toml:"status" json:"status"`
	Attempts    int       `toml:"attempts" json:"attempts"`
	LastError   string    `toml:"last_error,omitempty" json:"last_error,omitempty"`
	CreatedAt   time.Time `toml:"created_at" json:"created_at"`
	UpdatedAt   time.Time `toml:"updated_at" json:"updated_at"`
}

type inboxStateDoc struct {
	UpdatedAt time.Time        `toml:"updated_at"`
	Items     []inboxItemState `toml:"items"`
}

type syncedInboxRecord struct {
	event     InboxEvent
	updatedAt time.Time
	lastError string
}

type inboxStore struct {
	paths config.VaultPaths
}

func newInboxStore(paths config.VaultPaths) *inboxStore {
	return &inboxStore{paths: paths}
}

func (s *inboxStore) lockPath() string {
	return filepath.Join(s.paths.RuntimeDir(), "locks", "alwayson-inbox.lock")
}

func (s *inboxStore) jsonlPath() string {
	return filepath.Join(s.paths.AlwaysOnDir(), "inbox.jsonl")
}

func (s *inboxStore) statePath() string {
	return filepath.Join(s.paths.AlwaysOnDir(), "inbox.state.toml")
}

// recoverOnStart 扫描 inbox.jsonl 与 inbox.state.toml：
// 1. 过滤掉重复 (source, source_id) 的冗余行；
// 2. 将上次进程遗留的 stale "running" 任务重置为 "queued" 重试，若已达 MaxAttempts 则标记为 "failed"；
// 3. 原子落盘最新 inbox.state.toml 状态投影。
func (s *inboxStore) recoverOnStart(now time.Time) ([]InboxEvent, error) {
	var out []InboxEvent
	err := store.WithFlock(s.lockPath(), true, func() error {
		records, _, err := s.loadUnlocked(now)
		if err != nil {
			return err
		}
		for i := range records {
			if records[i].event.Status == StatusRunning {
				if records[i].event.Attempts >= MaxAttempts {
					records[i].event.Status = StatusFailed
				} else {
					records[i].event.Status = StatusQueued
				}
				records[i].updatedAt = now.UTC()
			}
		}
		if err := s.writeStateUnlocked(records, now); err != nil {
			return err
		}
		out = extractEvents(records)
		return nil
	})
	return out, err
}

func (s *inboxStore) enqueue(ev InboxEvent, now time.Time) (InboxEvent, bool, error) {
	if strings.TrimSpace(ev.WorkspaceID) == "" {
		return InboxEvent{}, false, errors.New("inbox event requires workspace_id")
	}

	var stored InboxEvent
	var deduped bool

	err := store.WithFlock(s.lockPath(), true, func() error {
		records, seenSources, err := s.loadUnlocked(now)
		if err != nil {
			return err
		}

		srcKey := makeSourceDedupKey(ev.Source, ev.SourceID)
		if srcKey != "" {
			if _, exists := seenSources[srcKey]; exists {
				deduped = true
				return nil
			}
		}

		norm := normalizeNewInboxEvent(ev, now)
		// Ensure ID uniqueness too.
		for _, r := range records {
			if r.event.ID == norm.ID {
				deduped = true
				stored = cloneInboxEvent(r.event)
				return nil
			}
		}

		if err := appendJSONLineNoEscape(s.jsonlPath(), norm); err != nil {
			return err
		}

		records = append(records, syncedInboxRecord{
			event:     norm,
			updatedAt: norm.CreatedAt,
		})
		if err := s.writeStateUnlocked(records, now); err != nil {
			return err
		}
		stored = cloneInboxEvent(norm)
		return nil
	})
	if err != nil {
		return InboxEvent{}, false, err
	}
	return stored, deduped, nil
}

// claimNextBatch 在排他文件锁下挑选下一组已满足防抖窗口的可执行微批事件：
// - deferred_resume 高优先级审批恢复事件零延迟优先出队；
// - 同一 InboxKey 下的连发排队事件在 InboxDebounceMS 防抖窗口收敛后一次性打包为单批返回。
func (s *inboxStore) claimNextBatch(
	now time.Time,
	debounce time.Duration,
	retryBackoff time.Duration,
) ([]InboxEvent, time.Duration, error) {
	if debounce < 0 {
		debounce = 0
	}
	if retryBackoff <= 0 {
		retryBackoff = 15 * time.Millisecond
	}

	var claimed []InboxEvent
	var nextWake time.Duration

	err := store.WithFlock(s.lockPath(), true, func() error {
		records, _, err := s.loadUnlocked(now)
		if err != nil {
			return err
		}

		runningKeys := make(map[string]bool)
		for _, r := range records {
			if r.event.Status == StatusRunning {
				runningKeys[r.event.InboxKey] = true
			}
		}

		// 1. High-priority pass: deferred_resume events bypass debounce.
		for i := range records {
			if records[i].event.Status != StatusQueued || records[i].event.Source != SourceDeferredResume {
				continue
			}
			if runningKeys[records[i].event.InboxKey] {
				continue
			}
			records[i].event.Status = StatusRunning
			records[i].event.Attempts++
			records[i].updatedAt = now.UTC()
			if err := s.writeStateUnlocked(records, now); err != nil {
				return err
			}
			claimed = []InboxEvent{cloneInboxEvent(records[i].event)}
			return nil
		}

		// 2. Group queued items by InboxKey preserving FIFO key discovery order.
		type keyGroup struct {
			key        string
			indices    []int
			firstAt    time.Time
			latestAt   time.Time
			lastUpdAt  time.Time
			allRetries bool
		}
		var orderedKeys []string
		byKey := make(map[string]*keyGroup)

		for i, r := range records {
			if r.event.Status != StatusQueued {
				continue
			}
			k := r.event.InboxKey
			if runningKeys[k] {
				continue
			}
			g, ok := byKey[k]
			if !ok {
				g = &keyGroup{
					key:        k,
					firstAt:    r.event.CreatedAt,
					latestAt:   r.event.CreatedAt,
					lastUpdAt:  r.updatedAt,
					allRetries: true,
				}
				byKey[k] = g
				orderedKeys = append(orderedKeys, k)
			}
			g.indices = append(g.indices, i)
			if r.event.CreatedAt.Before(g.firstAt) {
				g.firstAt = r.event.CreatedAt
			}
			if r.event.CreatedAt.After(g.latestAt) {
				g.latestAt = r.event.CreatedAt
			}
			if r.updatedAt.After(g.lastUpdAt) {
				g.lastUpdAt = r.updatedAt
			}
			if r.event.Attempts == 0 {
				g.allRetries = false
			}
		}

		var minWait time.Duration
		haveWait := false

		for _, k := range orderedKeys {
			g := byKey[k]
			var readyAt time.Time
			switch {
			case g.allRetries:
				readyAt = g.lastUpdAt.Add(retryBackoff)
			case debounce == 0:
				readyAt = now
			default:
				sliding := g.latestAt.Add(debounce)
				maxWindow := g.firstAt.Add(3 * debounce)
				if sliding.Before(maxWindow) {
					readyAt = sliding
				} else {
					readyAt = maxWindow
				}
			}

			if !now.Before(readyAt) {
				claimed = make([]InboxEvent, 0, len(g.indices))
				for _, idx := range g.indices {
					records[idx].event.Status = StatusRunning
					records[idx].event.Attempts++
					records[idx].updatedAt = now.UTC()
					claimed = append(claimed, cloneInboxEvent(records[idx].event))
				}
				return s.writeStateUnlocked(records, now)
			}

			wait := readyAt.Sub(now)
			if !haveWait || wait < minWait {
				minWait = wait
				haveWait = true
			}
		}

		if haveWait {
			nextWake = minWait
		}
		return nil
	})

	return claimed, nextWake, err
}

func (s *inboxStore) completeBatch(ids []string, sessionID, outcomeStatus, errMsg string, now time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	return store.WithFlock(s.lockPath(), true, func() error {
		records, _, err := s.loadUnlocked(now)
		if err != nil {
			return err
		}
		for i := range records {
			if !idSet[records[i].event.ID] {
				continue
			}
			if strings.TrimSpace(sessionID) != "" && records[i].event.SessionID == "" {
				records[i].event.SessionID = strings.TrimSpace(sessionID)
			}
			records[i].updatedAt = now.UTC()
			records[i].lastError = strings.TrimSpace(errMsg)

			if outcomeStatus == StatusFailed {
				if records[i].event.Attempts >= MaxAttempts {
					records[i].event.Status = StatusFailed
				} else {
					records[i].event.Status = StatusQueued
				}
			} else {
				records[i].event.Status = outcomeStatus
			}
		}
		return s.writeStateUnlocked(records, now)
	})
}

func (s *inboxStore) requeueBusyBatch(ids []string, now time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	return store.WithFlock(s.lockPath(), true, func() error {
		records, _, err := s.loadUnlocked(now)
		if err != nil {
			return err
		}
		for i := range records {
			if !idSet[records[i].event.ID] {
				continue
			}
			if records[i].event.Attempts > 0 {
				records[i].event.Attempts--
			}
			records[i].event.Status = StatusQueued
			records[i].updatedAt = now.UTC()
		}
		return s.writeStateUnlocked(records, now)
	})
}

func (s *inboxStore) list(now time.Time) ([]InboxEvent, error) {
	var out []InboxEvent
	err := store.WithFlock(s.lockPath(), false, func() error {
		records, _, err := s.loadUnlocked(now)
		if err != nil {
			return err
		}
		out = extractEvents(records)
		return nil
	})
	return out, err
}

func (s *inboxStore) loadUnlocked(now time.Time) ([]syncedInboxRecord, map[string]string, error) {
	stateByID := make(map[string]inboxItemState)
	if rawState, err := os.ReadFile(s.statePath()); err == nil && len(bytes.TrimSpace(rawState)) > 0 {
		var doc inboxStateDoc
		if uErr := toml.Unmarshal(rawState, &doc); uErr != nil {
			return nil, nil, fmt.Errorf("parse inbox.state.toml: %w", uErr)
		}
		for _, item := range doc.Items {
			if strings.TrimSpace(item.ID) != "" {
				stateByID[item.ID] = item
			}
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("read inbox.state.toml: %w", err)
	}

	seenSources := make(map[string]string)
	idxByID := make(map[string]int)
	var records []syncedInboxRecord

	f, err := os.Open(s.jsonlPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, seenSources, nil
		}
		return nil, nil, fmt.Errorf("open inbox.jsonl: %w", err)
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	lineNo := 0
	for {
		line, rErr := reader.ReadBytes('\n')
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) > 0 {
			lineNo++
			var ev InboxEvent
			if uErr := json.Unmarshal(trimmed, &ev); uErr != nil {
				return nil, nil, fmt.Errorf("parse inbox.jsonl line %d: %w", lineNo, uErr)
			}
			if strings.TrimSpace(ev.ID) == "" {
				ev.ID = fmt.Sprintf("ev_line_%d", lineNo)
			}
			ev = normalizeExistingInboxEvent(ev, now)

			srcKey := makeSourceDedupKey(ev.Source, ev.SourceID)
			if existingIdx, sameID := idxByID[ev.ID]; sameID {
				records[existingIdx].event = ev
				records[existingIdx].updatedAt = ev.CreatedAt
			} else {
				if srcKey != "" {
					if ownerID, dup := seenSources[srcKey]; dup && ownerID != ev.ID {
						// Ignore duplicate (source, source_id) entries in inbox.jsonl.
						if errors.Is(rErr, io.EOF) {
							break
						}
						continue
					}
					seenSources[srcKey] = ev.ID
				}
				idxByID[ev.ID] = len(records)
				records = append(records, syncedInboxRecord{
					event:     ev,
					updatedAt: ev.CreatedAt,
				})
			}
		}
		if errors.Is(rErr, io.EOF) {
			break
		}
		if rErr != nil {
			return nil, nil, fmt.Errorf("read inbox.jsonl: %w", rErr)
		}
	}

	for i := range records {
		if st, ok := stateByID[records[i].event.ID]; ok {
			if st.Status != "" {
				records[i].event.Status = st.Status
			}
			if st.Attempts > records[i].event.Attempts || st.Status != StatusRunning {
				records[i].event.Attempts = st.Attempts
			}
			if st.SessionID != "" {
				records[i].event.SessionID = st.SessionID
			}
			if st.InboxKey != "" {
				records[i].event.InboxKey = st.InboxKey
			}
			if !st.UpdatedAt.IsZero() {
				records[i].updatedAt = st.UpdatedAt
			}
			records[i].lastError = st.LastError
		}
	}

	return records, seenSources, nil
}

func (s *inboxStore) writeStateUnlocked(records []syncedInboxRecord, now time.Time) error {
	doc := inboxStateDoc{
		UpdatedAt: now.UTC(),
		Items:     make([]inboxItemState, len(records)),
	}
	for i, r := range records {
		upd := r.updatedAt
		if upd.IsZero() {
			upd = now.UTC()
		}
		doc.Items[i] = inboxItemState{
			ID:          r.event.ID,
			InboxKey:    r.event.InboxKey,
			Source:      r.event.Source,
			SourceID:    r.event.SourceID,
			WorkspaceID: r.event.WorkspaceID,
			SessionID:   r.event.SessionID,
			AgentRole:   r.event.AgentRole,
			Status:      r.event.Status,
			Attempts:    r.event.Attempts,
			LastError:   r.lastError,
			CreatedAt:   r.event.CreatedAt,
			UpdatedAt:   upd,
		}
	}
	encoded, err := toml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal inbox.state.toml: %w", err)
	}
	return store.AtomicWriteFile(s.statePath(), encoded, 0o600)
}

func normalizeNewInboxEvent(ev InboxEvent, now time.Time) InboxEvent {
	cp := cloneInboxEvent(ev)
	cp.ID = strings.TrimSpace(cp.ID)
	if cp.ID == "" {
		cp.ID = fmt.Sprintf("ev_%d_%s", now.UTC().UnixMilli(), randomHex(6))
	}
	cp.Source = strings.TrimSpace(cp.Source)
	if cp.Source == "" {
		cp.Source = "local"
	}
	cp.SourceID = strings.TrimSpace(cp.SourceID)
	cp.WorkspaceID = strings.TrimSpace(cp.WorkspaceID)
	cp.SessionID = strings.TrimSpace(cp.SessionID)
	cp.AgentRole = strings.TrimSpace(cp.AgentRole)
	if cp.AgentRole == "" {
		cp.AgentRole = "coder"
	}
	cp.InboxKey = strings.TrimSpace(cp.InboxKey)
	if cp.InboxKey == "" {
		cp.InboxKey = defaultInboxKey(cp)
	}
	cp.Status = strings.TrimSpace(cp.Status)
	if cp.Status == "" {
		cp.Status = StatusQueued
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now.UTC()
	} else {
		cp.CreatedAt = cp.CreatedAt.UTC()
	}
	return cp
}

func normalizeExistingInboxEvent(ev InboxEvent, now time.Time) InboxEvent {
	cp := cloneInboxEvent(ev)
	cp.Source = strings.TrimSpace(cp.Source)
	if cp.Source == "" {
		cp.Source = "local"
	}
	cp.SourceID = strings.TrimSpace(cp.SourceID)
	cp.WorkspaceID = strings.TrimSpace(cp.WorkspaceID)
	cp.SessionID = strings.TrimSpace(cp.SessionID)
	cp.AgentRole = strings.TrimSpace(cp.AgentRole)
	if cp.AgentRole == "" {
		cp.AgentRole = "coder"
	}
	cp.InboxKey = strings.TrimSpace(cp.InboxKey)
	if cp.InboxKey == "" {
		cp.InboxKey = defaultInboxKey(cp)
	}
	cp.Status = strings.TrimSpace(cp.Status)
	if cp.Status == "" {
		cp.Status = StatusQueued
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now.UTC()
	} else {
		cp.CreatedAt = cp.CreatedAt.UTC()
	}
	return cp
}

func defaultInboxKey(ev InboxEvent) string {
	if ev.Source == SourceDeferredResume {
		return fmt.Sprintf("resume:%s:%s", ev.WorkspaceID, ev.ID)
	}
	if ev.SessionID != "" {
		return fmt.Sprintf("%s:%s", ev.WorkspaceID, ev.SessionID)
	}
	return fmt.Sprintf("%s:%s", ev.WorkspaceID, ev.AgentRole)
}

func makeSourceDedupKey(source, sourceID string) string {
	s := strings.TrimSpace(source)
	sid := strings.TrimSpace(sourceID)
	if s == "" || sid == "" {
		return ""
	}
	return s + "\x00" + sid
}

func extractEvents(records []syncedInboxRecord) []InboxEvent {
	out := make([]InboxEvent, len(records))
	for i, r := range records {
		out[i] = cloneInboxEvent(r.event)
	}
	return out
}

func cloneInboxEvent(ev InboxEvent) InboxEvent {
	cp := ev
	if ev.Payload != nil {
		raw, err := json.Marshal(ev.Payload)
		if err == nil {
			var cloned map[string]any
			if json.Unmarshal(raw, &cloned) == nil {
				cp.Payload = cloned
			}
		}
	}
	return cp
}

func appendJSONLineNoEscape(path string, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create parent dir for %s: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open append %s: %w", path, err)
	}
	data := buf.Bytes()
	for len(data) > 0 {
		n, wErr := f.Write(data)
		if wErr != nil {
			_ = f.Close()
			return fmt.Errorf("write %s: %w", path, wErr)
		}
		data = data[n:]
	}
	if sErr := f.Sync(); sErr != nil {
		_ = f.Close()
		return fmt.Errorf("sync %s: %w", path, sErr)
	}
	return f.Close()
}

func randomHex(numBytes int) string {
	b := make([]byte, numBytes)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%012x", idFallbackSeq.Add(1))
	}
	return hex.EncodeToString(b)
}
