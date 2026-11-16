package alwayson

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/store"
)

const (
	MemoryLayerInstructions = "instructions"
	MemoryLayerIndex        = "index"
)

var (
	ErrMemoryNotFound     = errors.New("memory entry not found")
	ErrInvalidMemoryLayer = errors.New("invalid global memory layer")

	reSlugNonAlnum  = regexp.MustCompile(`[^a-z0-9]+`)
	reReminderClose = regexp.MustCompile(`(?i)</\s*system-reminder\s*>`)
	reReminderOpen  = regexp.MustCompile(`(?i)<\s*system-reminder\b`)
)

// MemoryNote 表示第 3 层全局耐久记忆卡片（`~/.gorness/memory/notes/<slug>.md`）。
type MemoryNote struct {
	Slug      string    `json:"slug" toml:"slug"`
	Title     string    `json:"title" toml:"title"`
	Summary   string    `json:"summary,omitempty" toml:"summary,omitempty"`
	Content   string    `json:"content" toml:"content"`
	Tags      []string  `json:"tags,omitempty" toml:"tags,omitempty"`
	UpdatedAt time.Time `json:"updated_at" toml:"updated_at"`
}

type memoryNoteFrontmatter struct {
	Slug      string    `toml:"slug" json:"slug"`
	Title     string    `toml:"title" json:"title"`
	Summary   string    `toml:"summary,omitempty" json:"summary,omitempty"`
	Tags      []string  `toml:"tags,omitempty" json:"tags,omitempty"`
	UpdatedAt time.Time `toml:"updated_at" json:"updated_at"`
}

// ProgressEntry 表示第 4 层工作区专属进度记忆（`~/.gorness/workspaces/<wsID>/memory/progress/<slug>.md`）。
type ProgressEntry struct {
	WorkspaceID string    `json:"workspace_id" toml:"workspace_id"`
	Slug        string    `json:"slug" toml:"slug"`
	Title       string    `json:"title,omitempty" toml:"title,omitempty"`
	Content     string    `json:"content" toml:"content"`
	UpdatedAt   time.Time `json:"updated_at" toml:"updated_at"`
}

// MemoryStore 管理四层指针记忆：
//   - Layer 1: `memory/INSTRUCTIONS.md`
//   - Layer 2: `memory/INDEX.md`（外加 Always-On 去重账本 `memory/REPORTED.md`）
//   - Layer 3: `memory/notes/<slug>.md`
//   - Layer 4: `workspaces/<wsID>/memory/progress/<slug>.md`
type MemoryStore struct {
	paths    config.VaultPaths
	nowFn    func() time.Time
	onMutate func()
}

func NewMemoryStore(paths config.VaultPaths) *MemoryStore {
	return &MemoryStore{
		paths: paths,
		nowFn: time.Now,
	}
}

func newMemoryStore(paths config.VaultPaths) *MemoryStore {
	return NewMemoryStore(paths)
}

func (m *MemoryStore) now() time.Time {
	if m != nil && m.nowFn != nil {
		return m.nowFn().UTC()
	}
	return time.Now().UTC()
}

func (m *MemoryStore) notifyChanged() {
	if m != nil && m.onMutate != nil {
		m.onMutate()
	}
}

func (m *MemoryStore) lockPath() string {
	return filepath.Join(m.paths.RuntimeDir(), "locks", "alwayson-memory.lock")
}

func (m *MemoryStore) instructionsPath() string {
	return filepath.Join(m.paths.MemoryDir(), "INSTRUCTIONS.md")
}

func (m *MemoryStore) indexPath() string {
	return filepath.Join(m.paths.MemoryDir(), "INDEX.md")
}

func (m *MemoryStore) reportedPath() string {
	return filepath.Join(m.paths.MemoryDir(), "REPORTED.md")
}

func (m *MemoryStore) notesDir() string {
	return filepath.Join(m.paths.MemoryDir(), "notes")
}

func (m *MemoryStore) resolveGlobalLayerPath(layer string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(layer)) {
	case "instructions", "instructions.md", "layer1":
		return m.instructionsPath(), nil
	case "index", "index.md", "layer2":
		return m.indexPath(), nil
	default:
		return "", fmt.Errorf("%w: %q (expected 'instructions' or 'index')", ErrInvalidMemoryLayer, layer)
	}
}

// GetGlobalLayer 读取第 1 层 (`instructions`) 或第 2 层 (`index`) 全局记忆 Markdown 内容。
func (m *MemoryStore) GetGlobalLayer(layer string) (string, error) {
	target, err := m.resolveGlobalLayerPath(layer)
	if err != nil {
		return "", err
	}
	var content string
	rErr := store.WithFlock(m.lockPath(), false, func() error {
		raw, err := os.ReadFile(target)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				content = ""
				return nil
			}
			return fmt.Errorf("read %s: %w", filepath.Base(target), err)
		}
		content = string(raw)
		return nil
	})
	return content, rErr
}

// SaveGlobalLayer 原子覆写第 1 层 (`instructions`) 或第 2 层 (`index`) 全局记忆 Markdown 内容（自动剥离/转义双向控制符）。
func (m *MemoryStore) SaveGlobalLayer(layer, content string) error {
	target, err := m.resolveGlobalLayerPath(layer)
	if err != nil {
		return err
	}
	trimmed := strings.TrimRight(config.EscapeBidi(content), "\r\n")
	if trimmed != "" {
		trimmed += "\n"
	}
	wErr := store.WithFlock(m.lockPath(), true, func() error {
		return store.AtomicWriteFile(target, []byte(trimmed), 0o600)
	})
	if wErr == nil {
		m.notifyChanged()
	}
	return wErr
}

func validateMemorySlug(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "." || trimmed == ".." ||
		strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") || strings.Contains(trimmed, "..") {
		return "", fmt.Errorf("%w: %q", store.ErrInvalidID, raw)
	}
	slug := sanitizeSlug(trimmed)
	if slug == "" {
		return "", fmt.Errorf("%w: %q", store.ErrInvalidID, raw)
	}
	return slug, nil
}

// ListNotes 返回第 3 层 `memory/notes/*.md` 下全部记忆卡片（按更新时间倒序）。
func (m *MemoryStore) ListNotes() ([]MemoryNote, error) {
	var out []MemoryNote
	err := store.WithFlock(m.lockPath(), false, func() error {
		notes, lErr := m.listNotesUnlocked()
		if lErr != nil {
			return lErr
		}
		out = notes
		return nil
	})
	return out, err
}

func (m *MemoryStore) listNotesUnlocked() ([]MemoryNote, error) {
	entries, err := os.ReadDir(m.notesDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []MemoryNote{}, nil
		}
		return nil, fmt.Errorf("readdir notes: %w", err)
	}
	notes := make([]MemoryNote, 0, len(entries))
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".md") {
			continue
		}
		slug := strings.TrimSuffix(ent.Name(), ".md")
		if store.ValidateSafeID(slug) != nil {
			continue
		}
		p := filepath.Join(m.notesDir(), ent.Name())
		raw, rErr := os.ReadFile(p)
		if rErr != nil {
			continue
		}
		var modTime time.Time
		if fi, sErr := ent.Info(); sErr == nil {
			modTime = fi.ModTime().UTC()
		}
		notes = append(notes, parseMemoryNoteMD(raw, slug, modTime))
	}
	sort.Slice(notes, func(i, j int) bool {
		if !notes[i].UpdatedAt.Equal(notes[j].UpdatedAt) {
			return notes[i].UpdatedAt.After(notes[j].UpdatedAt)
		}
		return notes[i].Slug < notes[j].Slug
	})
	return notes, nil
}

// GetNote 读取单张第 3 层全局记忆卡片（`memory/notes/<slug>.md`）。
func (m *MemoryStore) GetNote(slug string) (MemoryNote, error) {
	safeSlug, err := validateMemorySlug(slug)
	if err != nil {
		return MemoryNote{}, err
	}
	notePath := filepath.Join(m.notesDir(), safeSlug+".md")

	var note MemoryNote
	rErr := store.WithFlock(m.lockPath(), false, func() error {
		fi, sErr := os.Stat(notePath)
		if sErr != nil {
			if errors.Is(sErr, os.ErrNotExist) {
				return fmt.Errorf("%w: %s", ErrMemoryNotFound, safeSlug)
			}
			return sErr
		}
		raw, err := os.ReadFile(notePath)
		if err != nil {
			return err
		}
		note = parseMemoryNoteMD(raw, safeSlug, fi.ModTime().UTC())
		return nil
	})
	return note, rErr
}

// SaveNote 原子保存第 3 层记忆卡片并同步更新第 2 层 `memory/INDEX.md` 指针索引。
func (m *MemoryStore) SaveNote(slug, title, content string, tags []string) (MemoryNote, error) {
	return m.saveNoteInternal(slug, title, "", content, tags, m.now())
}

func (m *MemoryStore) saveNoteInternal(slug, title, summary, content string, tags []string, now time.Time) (MemoryNote, error) {
	safeSlug, err := validateMemorySlug(slug)
	if err != nil {
		return MemoryNote{}, err
	}
	cleanTitle := strings.TrimSpace(config.EscapeBidi(title))
	if cleanTitle == "" {
		cleanTitle = safeSlug
	}
	cleanBody := strings.TrimSpace(config.EscapeBidi(content))
	cleanSummary := collapseWhitespace(config.EscapeBidi(summary))
	if cleanSummary == "" {
		cleanSummary = collapseWhitespace(truncateString(cleanBody, 120))
	}
	cleanTags := normalizeTags(tags)
	nowUTC := now.UTC()

	note := MemoryNote{
		Slug:      safeSlug,
		Title:     cleanTitle,
		Summary:   cleanSummary,
		Content:   cleanBody,
		Tags:      cleanTags,
		UpdatedAt: nowUTC,
	}
	docBytes, err := formatMemoryNoteMD(note)
	if err != nil {
		return MemoryNote{}, err
	}

	notePath := filepath.Join(m.notesDir(), safeSlug+".md")
	wErr := store.WithFlock(m.lockPath(), true, func() error {
		if err := store.AtomicWriteFile(notePath, docBytes, 0o600); err != nil {
			return err
		}
		return m.upsertIndexPointerUnlocked(note)
	})
	if wErr != nil {
		return MemoryNote{}, wErr
	}
	m.notifyChanged()
	return note, nil
}

// DeleteNote 删除第 3 层记忆卡片并同步清理 `memory/INDEX.md` 对应指针行。
func (m *MemoryStore) DeleteNote(slug string) error {
	safeSlug, err := validateMemorySlug(slug)
	if err != nil {
		return err
	}
	notePath := filepath.Join(m.notesDir(), safeSlug+".md")
	dErr := store.WithFlock(m.lockPath(), true, func() error {
		if _, sErr := os.Stat(notePath); sErr != nil {
			if errors.Is(sErr, os.ErrNotExist) {
				return fmt.Errorf("%w: %s", ErrMemoryNotFound, safeSlug)
			}
			return sErr
		}
		if err := os.Remove(notePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return m.removeIndexPointerUnlocked(safeSlug)
	})
	if dErr == nil {
		m.notifyChanged()
	}
	return dErr
}

func (m *MemoryStore) upsertIndexPointerUnlocked(note MemoryNote) error {
	idxContent := "# Memory Index (全局记忆索引)\n\nPointers to durable engineering notes stored under `notes/*.md`.\n"
	if raw, rErr := os.ReadFile(m.indexPath()); rErr == nil && strings.TrimSpace(string(raw)) != "" {
		idxContent = string(raw)
	}

	relLink := fmt.Sprintf("(notes/%s.md)", note.Slug)
	pointerLine := fmt.Sprintf("- [%s](notes/%s.md) — %s", note.Title, note.Slug, note.Summary)
	if len(note.Tags) > 0 {
		pointerLine += fmt.Sprintf(" [tags: %s]", strings.Join(note.Tags, ", "))
	}

	lines := strings.Split(strings.TrimRight(idxContent, "\n"), "\n")
	replaced := false
	for i, line := range lines {
		if strings.Contains(line, relLink) {
			lines[i] = pointerLine
			replaced = true
			break
		}
	}
	if !replaced {
		lines = append(lines, pointerLine)
	}
	updatedIdx := strings.Join(lines, "\n") + "\n"
	return store.AtomicWriteFile(m.indexPath(), []byte(updatedIdx), 0o600)
}

func (m *MemoryStore) removeIndexPointerUnlocked(safeSlug string) error {
	raw, err := os.ReadFile(m.indexPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	relLink := fmt.Sprintf("(notes/%s.md)", safeSlug)
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	kept := make([]string, 0, len(lines))
	changed := false
	for _, line := range lines {
		if strings.Contains(line, relLink) {
			changed = true
			continue
		}
		kept = append(kept, line)
	}
	if !changed {
		return nil
	}
	updatedIdx := strings.Join(kept, "\n") + "\n"
	return store.AtomicWriteFile(m.indexPath(), []byte(updatedIdx), 0o600)
}

func (m *MemoryStore) workspaceMemoryDir(wsID string) (string, error) {
	if err := store.ValidateSafeID(wsID); err != nil {
		return "", err
	}
	if strings.Contains(wsID, "..") {
		return "", fmt.Errorf("%w: %q", store.ErrInvalidID, wsID)
	}
	wsRoot := filepath.Clean(m.paths.WorkspacesDir())
	dir := filepath.Join(wsRoot, wsID, "memory")
	if !strings.HasPrefix(dir, wsRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: workspace memory path escapes %s", store.ErrInvalidPath, wsRoot)
	}
	return dir, nil
}

func (m *MemoryStore) workspaceProgressDir(wsID string) (string, error) {
	memDir, err := m.workspaceMemoryDir(wsID)
	if err != nil {
		return "", err
	}
	return filepath.Join(memDir, "progress"), nil
}

// ListProgress 列出指定工作区下的第 4 层进度记忆（`workspaces/<wsID>/memory/progress/*.md`）。
func (m *MemoryStore) ListProgress(wsID string) ([]ProgressEntry, error) {
	progDir, err := m.workspaceProgressDir(wsID)
	if err != nil {
		return nil, err
	}
	var out []ProgressEntry
	rErr := store.WithFlock(m.lockPath(), false, func() error {
		entries, err := os.ReadDir(progDir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				out = []ProgressEntry{}
				return nil
			}
			return fmt.Errorf("readdir progress: %w", err)
		}
		items := make([]ProgressEntry, 0, len(entries))
		for _, ent := range entries {
			if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".md") {
				continue
			}
			slug := strings.TrimSuffix(ent.Name(), ".md")
			if store.ValidateSafeID(slug) != nil {
				continue
			}
			p := filepath.Join(progDir, ent.Name())
			raw, rErr := os.ReadFile(p)
			if rErr != nil {
				continue
			}
			var modTime time.Time
			if fi, sErr := ent.Info(); sErr == nil {
				modTime = fi.ModTime().UTC()
			}
			content := strings.TrimSpace(string(raw))
			items = append(items, ProgressEntry{
				WorkspaceID: wsID,
				Slug:        slug,
				Title:       extractMarkdownTitle(content, slug),
				Content:     content,
				UpdatedAt:   modTime,
			})
		}
		sort.Slice(items, func(i, j int) bool {
			if !items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
				return items[i].UpdatedAt.After(items[j].UpdatedAt)
			}
			return items[i].Slug < items[j].Slug
		})
		out = items
		return nil
	})
	return out, rErr
}

// GetProgress 读取指定工作区下单条第 4 层进度记忆。
func (m *MemoryStore) GetProgress(wsID, slug string) (ProgressEntry, error) {
	progDir, err := m.workspaceProgressDir(wsID)
	if err != nil {
		return ProgressEntry{}, err
	}
	safeSlug, err := validateMemorySlug(slug)
	if err != nil {
		return ProgressEntry{}, err
	}
	target := filepath.Join(progDir, safeSlug+".md")

	var item ProgressEntry
	rErr := store.WithFlock(m.lockPath(), false, func() error {
		fi, sErr := os.Stat(target)
		if sErr != nil {
			if errors.Is(sErr, os.ErrNotExist) {
				return fmt.Errorf("%w: %s/%s", ErrMemoryNotFound, wsID, safeSlug)
			}
			return sErr
		}
		raw, err := os.ReadFile(target)
		if err != nil {
			return err
		}
		content := strings.TrimSpace(string(raw))
		item = ProgressEntry{
			WorkspaceID: wsID,
			Slug:        safeSlug,
			Title:       extractMarkdownTitle(content, safeSlug),
			Content:     content,
			UpdatedAt:   fi.ModTime().UTC(),
		}
		return nil
	})
	return item, rErr
}

// SaveProgress 原子保存指定工作区下的第 4 层进度记忆。
func (m *MemoryStore) SaveProgress(wsID, slug, content string) (ProgressEntry, error) {
	progDir, err := m.workspaceProgressDir(wsID)
	if err != nil {
		return ProgressEntry{}, err
	}
	safeSlug, err := validateMemorySlug(slug)
	if err != nil {
		return ProgressEntry{}, err
	}
	trimmed := strings.TrimSpace(config.EscapeBidi(content))
	nowUTC := m.now()
	target := filepath.Join(progDir, safeSlug+".md")

	wErr := store.WithFlock(m.lockPath(), true, func() error {
		return store.AtomicWriteFile(target, []byte(trimmed+"\n"), 0o600)
	})
	if wErr != nil {
		return ProgressEntry{}, wErr
	}
	m.notifyChanged()
	return ProgressEntry{
		WorkspaceID: wsID,
		Slug:        safeSlug,
		Title:       extractMarkdownTitle(trimmed, safeSlug),
		Content:     trimmed,
		UpdatedAt:   nowUTC,
	}, nil
}

// DeleteProgress 删除指定工作区下的第 4 层进度记忆。
func (m *MemoryStore) DeleteProgress(wsID, slug string) error {
	progDir, err := m.workspaceProgressDir(wsID)
	if err != nil {
		return err
	}
	safeSlug, err := validateMemorySlug(slug)
	if err != nil {
		return err
	}
	target := filepath.Join(progDir, safeSlug+".md")
	dErr := store.WithFlock(m.lockPath(), true, func() error {
		if _, sErr := os.Stat(target); sErr != nil {
			if errors.Is(sErr, os.ErrNotExist) {
				return fmt.Errorf("%w: %s/%s", ErrMemoryNotFound, wsID, safeSlug)
			}
			return sErr
		}
		return os.Remove(target)
	})
	if dErr == nil {
		m.notifyChanged()
	}
	return dErr
}

const (
	defaultBootstrapInstructionsMD = "# Persistent Instructions (长期行为指令 / Always-On Static Prefix)\n\n- Keep changes minimal, Idiomatic Go, and race-free.\n- Verify behavior with automated tests before reporting completion."
	defaultBootstrapIndexMD        = "# Memory Index (全局记忆索引)\n\nPointers to durable engineering notes stored under `notes/*.md`."
)

// buildStepOneMemoryReminders 为 `PreStepHook (Step == 1)` 组装精炼且经过双向控制符与 `</system-reminder>` 转义净化后的四层记忆动态提示。
// 当新保险库仅包含未修改的初始占位模板且尚无笔记/进度记忆时，跳过空壳注入以保持提示词紧凑纯净。
func (m *MemoryStore) buildStepOneMemoryReminders(wsID string) []string {
	var reminders []string

	if rawInst, err := os.ReadFile(m.instructionsPath()); err == nil {
		if trimmed := strings.TrimSpace(string(rawInst)); trimmed != "" && trimmed != defaultBootstrapInstructionsMD {
			reminders = append(reminders, "<memory_instructions>\n"+sanitizeMemoryReminder(truncateString(trimmed, 2048))+"\n</memory_instructions>")
		}
	}

	if rawIdx, err := os.ReadFile(m.indexPath()); err == nil {
		if trimmed := strings.TrimSpace(string(rawIdx)); trimmed != "" && trimmed != defaultBootstrapIndexMD {
			reminders = append(reminders, "<memory_index>\n"+sanitizeMemoryReminder(truncateString(trimmed, 4096))+"\n</memory_index>")
		}
	}

	if cleanWS := strings.TrimSpace(wsID); cleanWS != "" {
		if items, pErr := m.ListProgress(cleanWS); pErr == nil && len(items) > 0 {
			var lines []string
			limit := len(items)
			if limit > 12 {
				limit = 12
			}
			for _, it := range items[:limit] {
				snippet := collapseWhitespace(truncateString(it.Content, 160))
				lines = append(lines, fmt.Sprintf("- [%s] (%s): %s", it.Slug, it.Title, snippet))
			}
			body := sanitizeMemoryReminder(strings.Join(lines, "\n"))
			reminders = append(reminders, fmt.Sprintf("<workspace_progress workspace=%q>\n%s\n</workspace_progress>", cleanWS, body))
		}
	}

	return reminders
}

// buildUnattendedReminders 组装无人值守唤醒契约与已播报去重账本（Layer 1/2/4 由 PreStepHook Step==1 统一注入，避免重复）。
func (m *MemoryStore) buildUnattendedReminders(wsID string) []string {
	_ = wsID
	sections := []string{
		strings.Join([]string{
			"Always-On Unattended Execution Contract:",
			"- If you discover a new actionable issue or milestone that requires human attention and is NOT already listed in REPORTED.md, call `notify_user` with a concise `title` and `body`.",
			"- If nothing requires human attention or the finding was already reported in REPORTED.md, do NOT call `notify_user` so the run concludes silently.",
			"- If you need to re-check status later, call `set_followup` with `after_seconds` and `note`.",
		}, "\n"),
	}

	if rawRep, err := os.ReadFile(m.reportedPath()); err == nil {
		if trimmed := strings.TrimSpace(string(rawRep)); trimmed != "" {
			sections = append(sections, "<reported_ledger>\n"+sanitizeMemoryReminder(truncateString(trimmed, 4096))+"\n</reported_ledger>")
		}
	}
	return sections
}

// checkAndRecordReported 在排他文件锁下检查 REPORTED.md 是否已播报过相同事项；
// 若已播报则返回 alreadyReported=true 予以静默抑制；若为新发现则原子追加至 REPORTED.md。
func (m *MemoryStore) checkAndRecordReported(wsID, title, body string, now time.Time) (bool, error) {
	wsTag := strings.TrimSpace(wsID)
	if wsTag == "" {
		wsTag = "global"
	}
	fp := reportedFingerprint(wsTag, title, body)
	fpMarker := "[fp:" + fp + "]"
	oneLineTitle := collapseWhitespace(title)
	oneLineBody := collapseWhitespace(body)
	exactSignature := "**" + oneLineTitle + "**: " + oneLineBody

	var already bool
	err := store.WithFlock(m.lockPath(), true, func() error {
		existing := "# Reported Findings Ledger (已播报事项账本)\n\nTracks delivered Always-On alerts to suppress duplicate notifications.\n"
		if raw, err := os.ReadFile(m.reportedPath()); err == nil {
			existing = string(raw)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read REPORTED.md: %w", err)
		}

		if matchesReportedLedger(existing, wsTag, fpMarker, exactSignature) {
			already = true
			return nil
		}

		var b strings.Builder
		b.WriteString(strings.TrimRight(existing, "\n"))
		b.WriteString("\n")
		fmt.Fprintf(&b, "- [%s] [ws:%s] %s %s\n",
			now.UTC().Format(time.RFC3339),
			wsTag,
			fpMarker,
			exactSignature,
		)
		return store.AtomicWriteFile(m.reportedPath(), []byte(b.String()), 0o600)
	})
	return already, err
}

func matchesReportedLedger(existing, wsTag, fpMarker, exactSignature string) bool {
	wantWSTag := "[ws:" + wsTag + "]"
	for _, rawLine := range strings.Split(existing, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, fpMarker) {
			return true
		}
		if !strings.Contains(line, "[fp:") && exactSignature != "****: " {
			if strings.Contains(line, "[ws:") && !strings.Contains(line, wantWSTag) {
				continue
			}
			normLine := collapseWhitespace(line)
			if normLine == exactSignature || normLine == "- "+exactSignature || strings.HasSuffix(normLine, " "+exactSignature) {
				return true
			}
		}
	}
	return false
}

func (m *MemoryStore) upsertNote(slug, title, summary, body string, now time.Time) (string, error) {
	safeSlug := sanitizeSlug(slug)
	if safeSlug == "" {
		safeSlug = fmt.Sprintf("note-%d", now.UTC().Unix())
	}
	note, err := m.saveNoteInternal(safeSlug, title, summary, body, nil, now)
	if err != nil {
		return "", err
	}
	return filepath.Join(m.notesDir(), note.Slug+".md"), nil
}

func (m *MemoryStore) writeWorkspaceProgress(wsID, slug, content string) (string, error) {
	progDir, err := m.workspaceProgressDir(wsID)
	if err != nil {
		return "", err
	}
	safeSlug := sanitizeSlug(slug)
	if safeSlug == "" {
		safeSlug = "latest"
	}
	if _, err := m.SaveProgress(wsID, safeSlug, content); err != nil {
		return "", err
	}
	return filepath.Join(progDir, safeSlug+".md"), nil
}

func formatMemoryNoteMD(note MemoryNote) ([]byte, error) {
	fm := memoryNoteFrontmatter{
		Slug:      note.Slug,
		Title:     note.Title,
		Summary:   note.Summary,
		Tags:      note.Tags,
		UpdatedAt: note.UpdatedAt.UTC(),
	}
	fmBytes, err := toml.Marshal(fm)
	if err != nil {
		return nil, fmt.Errorf("marshal note frontmatter: %w", err)
	}
	var buf bytes.Buffer
	buf.WriteString("+++\n")
	buf.Write(bytes.TrimRight(fmBytes, "\n"))
	buf.WriteString("\n+++\n\n")
	if strings.TrimSpace(note.Content) != "" {
		buf.WriteString(strings.TrimSpace(note.Content))
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

func parseMemoryNoteMD(raw []byte, fallbackSlug string, fallbackUpdated time.Time) MemoryNote {
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "+++") || strings.HasPrefix(trimmed, "---") {
		if fm, body, err := config.ParseFrontmatterAs[memoryNoteFrontmatter](string(raw)); err == nil {
			slug := strings.TrimSpace(fm.Slug)
			if slug == "" {
				slug = fallbackSlug
			}
			title := strings.TrimSpace(fm.Title)
			if title == "" {
				title = extractMarkdownTitle(body, slug)
			}
			cleanBody := strings.TrimSpace(body)
			summary := strings.TrimSpace(fm.Summary)
			if summary == "" {
				summary = collapseWhitespace(truncateString(cleanBody, 120))
			}
			upd := fm.UpdatedAt.UTC()
			if upd.IsZero() {
				upd = fallbackUpdated
			}
			return MemoryNote{
				Slug:      slug,
				Title:     title,
				Summary:   summary,
				Content:   cleanBody,
				Tags:      normalizeTags(fm.Tags),
				UpdatedAt: upd,
			}
		}
	}

	// Legacy plain markdown note compatibility (`# Title` + `_Updated: ..._` + body).
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	title := fallbackSlug
	updated := fallbackUpdated
	bodyStart := 0
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "# ") {
			title = strings.TrimSpace(strings.TrimPrefix(t, "# "))
			bodyStart = i + 1
			for bodyStart < len(lines) {
				next := strings.TrimSpace(lines[bodyStart])
				if next == "" {
					bodyStart++
					continue
				}
				if strings.HasPrefix(next, "_Updated:") && strings.HasSuffix(next, "_") {
					tsRaw := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(next, "_Updated:"), "_"))
					if parsed, pErr := time.Parse(time.RFC3339, tsRaw); pErr == nil {
						updated = parsed.UTC()
					}
					bodyStart++
				}
				break
			}
		}
		break
	}
	content := strings.TrimSpace(strings.Join(lines[bodyStart:], "\n"))
	return MemoryNote{
		Slug:      fallbackSlug,
		Title:     title,
		Summary:   collapseWhitespace(truncateString(content, 120)),
		Content:   content,
		UpdatedAt: updated,
	}
}

func extractMarkdownTitle(content, fallback string) string {
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "#") {
			cleaned := strings.TrimSpace(strings.TrimLeft(t, "#"))
			if cleaned != "" {
				return cleaned
			}
		}
		break
	}
	return fallback
}

func normalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(tags))
	out := make([]string, 0, len(tags))
	for _, raw := range tags {
		t := strings.ToLower(strings.TrimSpace(raw))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sanitizeMemoryReminder(text string) string {
	clean := config.EscapeBidi(text)
	clean = reReminderClose.ReplaceAllString(clean, "[/system-reminder]")
	clean = reReminderOpen.ReplaceAllString(clean, "[system-reminder")
	return clean
}

func reportedFingerprint(wsID, title, body string) string {
	norm := strings.ToLower(strings.TrimSpace(wsID)) + "\x00" +
		strings.ToLower(collapseWhitespace(title)) + "\x00" +
		strings.ToLower(collapseWhitespace(body))
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:8])
}

func sanitizeSlug(raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	slug := strings.Trim(reSlugNonAlnum.ReplaceAllString(lower, "-"), "-")
	if len(slug) > 64 {
		slug = strings.Trim(slug[:64], "-")
	}
	return slug
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncateString(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	return s[:maxBytes]
}
