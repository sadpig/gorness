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
	"time"

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/store"
)

const (
	OutcomeSend   = "send"
	OutcomeSilent = "silent"
	OutcomeDefer  = "defer"

	osaScriptBinPath = "/usr/bin/osascript"
	osaTimeout       = 5 * time.Second
)

type OutboxItem struct {
	ID          string    `json:"id" toml:"id"`
	WorkspaceID string    `json:"workspace_id" toml:"workspace_id"`
	SessionID   string    `json:"session_id,omitempty" toml:"session_id,omitempty"`
	Outcome     string    `json:"outcome" toml:"outcome"` // "send" | "silent" | "defer"
	Title       string    `json:"title,omitempty" toml:"title,omitempty"`
	Body        string    `json:"body,omitempty" toml:"body,omitempty"`
	Delivered   bool      `json:"delivered" toml:"delivered"`
	CreatedAt   time.Time `json:"created_at" toml:"created_at"`
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
	paths config.VaultPaths
}

func newOutboxStore(paths config.VaultPaths) *outboxStore {
	return &outboxStore{paths: paths}
}

func (s *outboxStore) lockPath() string {
	return filepath.Join(s.paths.RuntimeDir(), "locks", "alwayson-outbox.lock")
}

func (s *outboxStore) filePath() string {
	return filepath.Join(s.paths.AlwaysOnDir(), "outbox.jsonl")
}

func (s *outboxStore) append(item OutboxItem, now time.Time) (OutboxItem, error) {
	if strings.TrimSpace(item.ID) == "" {
		item.ID = fmt.Sprintf("out_%d_%s", now.UTC().UnixMilli(), randomHex(5))
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
		return appendJSONLineNoEscape(s.filePath(), item)
	})
	if err != nil {
		return OutboxItem{}, err
	}
	return item, nil
}

func (s *outboxStore) list(limit int) ([]OutboxItem, error) {
	var out []OutboxItem
	err := store.WithFlock(s.lockPath(), false, func() error {
		f, err := os.Open(s.filePath())
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("open outbox.jsonl: %w", err)
		}
		defer f.Close()

		reader := bufio.NewReader(f)
		for {
			line, rErr := reader.ReadBytes('\n')
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) > 0 {
				var item OutboxItem
				if uErr := json.Unmarshal(trimmed, &item); uErr != nil {
					return fmt.Errorf("parse outbox.jsonl: %w", uErr)
				}
				out = append(out, item)
			}
			if errors.Is(rErr, io.EOF) {
				break
			}
			if rErr != nil {
				return fmt.Errorf("read outbox.jsonl: %w", rErr)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}
