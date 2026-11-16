package alwayson

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/store"
)

const (
	OutcomeSend   = "send"
	OutcomeSilent = "silent"
	OutcomeDefer  = "defer"

	defaultMaxOutboxRetained = 200

	osaScriptBinPath = "/usr/bin/osascript"
	osaTimeout       = 5 * time.Second
)

type OutboxItem struct {
	ID          string     `json:"id" toml:"id"`
	WorkspaceID string     `json:"workspace_id" toml:"workspace_id"`
	SessionID   string     `json:"session_id,omitempty" toml:"session_id,omitempty"`
	Outcome     string     `json:"outcome" toml:"outcome"` // "send" | "silent" | "defer"
	Title       string     `json:"title,omitempty" toml:"title,omitempty"`
	Body        string     `json:"body,omitempty" toml:"body,omitempty"`
	Delivered   bool       `json:"delivered" toml:"delivered"`
	Acked       bool       `json:"acked,omitempty" toml:"acked,omitempty"`
	AckedAt     *time.Time `json:"acked_at,omitempty" toml:"acked_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at" toml:"created_at"`
}

type Notifier interface {
	NotifyMac(title, subtitle, body string) error
}

// BuildOSAScriptArgs 使用 `on run argv` 位置参数传递通知标题、副标题与正文，彻底杜绝任意引号、反斜杠或命令替换引发的 AppleScript 注入。
func BuildOSAScriptArgs(title, subtitle, body string) []string {
	return []string{
		"-e", "on run argv",
		"-e", "display notification (item 3 of argv) with title (item 1 of argv) subtitle (item 2 of argv)",
		"-e", "end run",
		"--",
		config.EscapeBidi(title),
		config.EscapeBidi(subtitle),
		config.EscapeBidi(body),
	}
}

type OSAScriptNotifier struct {
	BinPath string
	Timeout time.Duration
	ExecFn  func(ctx context.Context, bin string, args []string) error
}

func NewOSAScriptNotifier() *OSAScriptNotifier {
	return &OSAScriptNotifier{
		BinPath: osaScriptBinPath,
		Timeout: osaTimeout,
	}
}

func (n *OSAScriptNotifier) NotifyMac(title, subtitle, body string) error {
	bin := osaScriptBinPath
	timeout := osaTimeout
	if n != nil {
		if strings.TrimSpace(n.BinPath) != "" {
			bin = strings.TrimSpace(n.BinPath)
		}
		if n.Timeout > 0 {
			timeout = n.Timeout
		}
	}

	args := BuildOSAScriptArgs(title, subtitle, body)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if n != nil && n.ExecFn != nil {
		return n.ExecFn(ctx, bin, args)
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("osascript notify failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

type outboxStore struct {
	paths       config.VaultPaths
	maxRetained int

	mu          sync.Mutex
	loaded      bool
	lastModNano int64
	lastSize    int64
	items       []OutboxItem
}

func newOutboxStore(paths config.VaultPaths, maxRetained ...int) *outboxStore {
	bound := defaultMaxOutboxRetained
	if len(maxRetained) > 0 && maxRetained[0] > 0 {
		bound = maxRetained[0]
	}
	return &outboxStore{
		paths:       paths,
		maxRetained: bound,
	}
}

func (s *outboxStore) lockPath() string {
	return filepath.Join(s.paths.RuntimeDir(), "locks", "alwayson-outbox.lock")
}

func (s *outboxStore) filePath() string {
	return filepath.Join(s.paths.AlwaysOnDir(), "outbox.jsonl")
}

func (s *outboxStore) append(item OutboxItem, now time.Time) (OutboxItem, error) {
	if strings.TrimSpace(item.ID) == "" {
		item.ID = fmt.Sprintf("out_%d_%s", now.UTC().UnixMilli(), store.RandomHex(5))
	}
	if strings.TrimSpace(item.Outcome) == "" {
		item.Outcome = OutcomeSilent
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now.UTC()
	} else {
		item.CreatedAt = item.CreatedAt.UTC()
	}

	err := store.WithFlock(s.lockPath(), true, func() error {
		s.mu.Lock()
		defer s.mu.Unlock()

		if err := s.ensureLoadedLocked(); err != nil {
			return err
		}
		if err := store.AppendJSONLine(s.filePath(), item); err != nil {
			return err
		}
		s.items = append(s.items, cloneOutboxItem(item))

		if len(s.items) > s.maxRetained*2 {
			_, err := s.compactLocked()
			return err
		}
		s.refreshFileStatLocked()
		return nil
	})
	if err != nil {
		return OutboxItem{}, err
	}
	return item, nil
}

func (s *outboxStore) list(limit int) ([]OutboxItem, error) {
	var out []OutboxItem
	err := store.WithFlock(s.lockPath(), false, func() error {
		s.mu.Lock()
		defer s.mu.Unlock()

		if err := s.ensureLoadedLocked(); err != nil {
			return err
		}
		start := 0
		if limit > 0 && len(s.items) > limit {
			start = len(s.items) - limit
		}
		out = make([]OutboxItem, len(s.items)-start)
		for i, it := range s.items[start:] {
			out[i] = cloneOutboxItem(it)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *outboxStore) ack(ids []string, now time.Time) (int, error) {
	idSet := make(map[string]bool, len(ids))
	for _, raw := range ids {
		if trimmed := strings.TrimSpace(raw); trimmed != "" {
			idSet[trimmed] = true
		}
	}
	if len(idSet) == 0 {
		return 0, nil
	}

	nowUTC := now.UTC()
	var ackedCount int
	err := store.WithFlock(s.lockPath(), true, func() error {
		s.mu.Lock()
		defer s.mu.Unlock()

		if err := s.ensureLoadedLocked(); err != nil {
			return err
		}
		for i := range s.items {
			if idSet[s.items[i].ID] && !s.items[i].Acked {
				s.items[i].Acked = true
				ts := nowUTC
				s.items[i].AckedAt = &ts
				ackedCount++
			}
		}
		if ackedCount == 0 {
			return nil
		}
		return s.rewriteLocked(s.items)
	})
	return ackedCount, err
}

// compact 清理已确认 (`acked == true`) 及超出容量上限的历史 Outbox 行，原子重写 `outbox.jsonl`。
func (s *outboxStore) compact() (int, error) {
	var removed int
	err := store.WithFlock(s.lockPath(), true, func() error {
		s.mu.Lock()
		defer s.mu.Unlock()

		if err := s.ensureLoadedLocked(); err != nil {
			return err
		}
		n, err := s.compactLocked()
		removed = n
		return err
	})
	return removed, err
}

func (s *outboxStore) maybeCompact() (int, error) {
	s.mu.Lock()
	needs := s.loaded && len(s.items) > s.maxRetained
	if !needs && s.loaded {
		for _, it := range s.items {
			if it.Acked && len(s.items) > s.maxRetained/2 {
				needs = true
				break
			}
		}
	}
	s.mu.Unlock()
	if !needs {
		return 0, nil
	}
	return s.compact()
}

func (s *outboxStore) compactLocked() (int, error) {
	before := len(s.items)
	kept := make([]OutboxItem, 0, len(s.items))
	for _, it := range s.items {
		if it.Acked {
			continue
		}
		kept = append(kept, it)
	}
	if s.maxRetained > 0 && len(kept) > s.maxRetained {
		needDrop := len(kept) - s.maxRetained
		filtered := make([]OutboxItem, 0, len(kept))
		for _, it := range kept {
			if needDrop > 0 && it.Outcome == OutcomeSilent {
				needDrop--
				continue
			}
			filtered = append(filtered, it)
		}
		kept = filtered
		if len(kept) > s.maxRetained {
			kept = kept[len(kept)-s.maxRetained:]
		}
	}
	removed := before - len(kept)
	if removed == 0 {
		return 0, nil
	}
	if err := s.rewriteLocked(kept); err != nil {
		return 0, err
	}
	return removed, nil
}

func (s *outboxStore) ensureLoadedLocked() error {
	modNano, size := statSignature(s.filePath())
	if s.loaded && s.lastModNano == modNano && s.lastSize == size {
		return nil
	}
	if modNano == -1 {
		s.items = nil
		s.loaded = true
		s.lastModNano = -1
		s.lastSize = -1
		return nil
	}

	f, err := os.Open(s.filePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.items = nil
			s.loaded = true
			s.lastModNano = -1
			s.lastSize = -1
			return nil
		}
		return fmt.Errorf("open outbox.jsonl: %w", err)
	}
	defer f.Close()

	var parsed []OutboxItem
	idxByID := make(map[string]int)
	reader := bufio.NewReader(f)
	for {
		line, rErr := reader.ReadBytes('\n')
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) > 0 {
			var item OutboxItem
			if uErr := json.Unmarshal(trimmed, &item); uErr != nil {
				return fmt.Errorf("parse outbox.jsonl: %w", uErr)
			}
			if item.ID != "" {
				if idx, ok := idxByID[item.ID]; ok {
					parsed[idx] = item
				} else {
					idxByID[item.ID] = len(parsed)
					parsed = append(parsed, item)
				}
			} else {
				parsed = append(parsed, item)
			}
		}
		if errors.Is(rErr, io.EOF) {
			break
		}
		if rErr != nil {
			return fmt.Errorf("read outbox.jsonl: %w", rErr)
		}
	}

	s.items = parsed
	s.loaded = true
	s.lastModNano = modNano
	s.lastSize = size
	return nil
}

func (s *outboxStore) rewriteLocked(items []OutboxItem) error {
	var buf bytes.Buffer
	for _, it := range items {
		line, err := store.MarshalJSONLine(it)
		if err != nil {
			return fmt.Errorf("marshal outbox item: %w", err)
		}
		buf.Write(line)
	}
	if err := store.AtomicWriteFile(s.filePath(), buf.Bytes(), 0o600); err != nil {
		return err
	}
	cloned := make([]OutboxItem, len(items))
	for i, it := range items {
		cloned[i] = cloneOutboxItem(it)
	}
	s.items = cloned
	s.loaded = true
	s.refreshFileStatLocked()
	return nil
}

func (s *outboxStore) refreshFileStatLocked() {
	s.lastModNano, s.lastSize = statSignature(s.filePath())
}

func cloneOutboxItem(it OutboxItem) OutboxItem {
	cp := it
	if it.AckedAt != nil {
		t := *it.AckedAt
		cp.AckedAt = &t
	}
	return cp
}

func statSignature(path string) (int64, int64) {
	fi, err := os.Stat(path)
	if err != nil {
		return -1, -1
	}
	return fi.ModTime().UnixNano(), fi.Size()
}
