package alwayson

import (
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

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/store"
)

var reSlugNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

type memoryStore struct {
	paths config.VaultPaths
}

func newMemoryStore(paths config.VaultPaths) *memoryStore {
	return &memoryStore{paths: paths}
}

func (m *memoryStore) lockPath() string {
	return filepath.Join(m.paths.RuntimeDir(), "locks", "alwayson-memory.lock")
}

func (m *memoryStore) indexPath() string {
	return filepath.Join(m.paths.MemoryDir(), "INDEX.md")
}

func (m *memoryStore) reportedPath() string {
	return filepath.Join(m.paths.MemoryDir(), "REPORTED.md")
}

func (m *memoryStore) notesDir() string {
	return filepath.Join(m.paths.MemoryDir(), "notes")
}

func validateSafeWorkspaceID(wsID string) (string, error) {
	trimmed := strings.TrimSpace(wsID)
	if trimmed == "" || trimmed != wsID || wsID == "." || wsID == ".." {
		return "", fmt.Errorf("%w: %q", store.ErrInvalidID, wsID)
	}
	for _, r := range wsID {
		if r == '/' || r == '\\' || r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("%w: %q", store.ErrInvalidID, wsID)
		}
	}
	return trimmed, nil
}

func (m *memoryStore) workspaceMemoryDir(wsID string) (string, error) {
	safeWS, err := validateSafeWorkspaceID(wsID)
	if err != nil {
		return "", err
	}
	wsRoot := filepath.Clean(m.paths.WorkspacesDir())
	dir := filepath.Join(wsRoot, safeWS, "memory")
	if !strings.HasPrefix(dir, wsRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: workspace memory path escapes %s", store.ErrInvalidPath, wsRoot)
	}
	return dir, nil
}

func (m *memoryStore) workspaceProgressDir(wsID string) (string, error) {
	memDir, err := m.workspaceMemoryDir(wsID)
	if err != nil {
		return "", err
	}
	return filepath.Join(memDir, "progress"), nil
}

// buildDynamicReminders 组装 Always-On 唤醒时的四层指针记忆摘要（INDEX.md、REPORTED.md 与项目专属 progress 指针）：
// 注意：第 1 层 INSTRUCTIONS.md 已由 core.AgentLoop 锁定在静态缓存前缀中，此处仅注入动态索引指针以最大化 Prompt Cache 命中率。
func (m *memoryStore) buildDynamicReminders(wsID string) []string {
	var sections []string

	sections = append(sections, strings.Join([]string{
		"Always-On Unattended Execution Contract:",
		"- If you discover a new actionable issue or milestone that requires human attention and is NOT already listed in REPORTED.md, call `notify_user` with a concise `title` and `body`.",
		"- If nothing requires human attention or the finding was already reported in REPORTED.md, do NOT call `notify_user` so the run concludes silently.",
		"- If you need to re-check status later, call `set_followup` with `after_seconds` and `note`.",
	}, "\n"))

	if rawIdx, err := os.ReadFile(m.indexPath()); err == nil {
		if trimmed := strings.TrimSpace(string(rawIdx)); trimmed != "" {
			sections = append(sections, "<memory_index>\n"+truncateString(trimmed, 4096)+"\n</memory_index>")
		}
	}

	if rawRep, err := os.ReadFile(m.reportedPath()); err == nil {
		if trimmed := strings.TrimSpace(string(rawRep)); trimmed != "" {
			sections = append(sections, "<reported_ledger>\n"+truncateString(trimmed, 4096)+"\n</reported_ledger>")
		}
	}

	if cleanWS := strings.TrimSpace(wsID); cleanWS != "" {
		if progDir, pErr := m.workspaceProgressDir(cleanWS); pErr == nil {
			if entries, err := os.ReadDir(progDir); err == nil && len(entries) > 0 {
				var names []string
				for _, ent := range entries {
					if !ent.IsDir() && strings.HasSuffix(ent.Name(), ".md") {
						names = append(names, ent.Name())
					}
				}
				sort.Strings(names)
				if len(names) > 0 {
					sections = append(sections, fmt.Sprintf("<workspace_progress_pointers workspace=%q>\n%s\n</workspace_progress_pointers>",
						cleanWS, strings.Join(names, "\n")))
				}
			}
		}
	}

	return sections
}

// checkAndRecordReported 在排他文件锁下检查 REPORTED.md 是否已播报过相同事项；
// 若已播报则返回 alreadyReported=true 予以静默抑制；若为新发现则原子追加至 REPORTED.md。
func (m *memoryStore) checkAndRecordReported(wsID, title, body string, now time.Time) (bool, error) {
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
		// Support hand-edited lines without [fp:...]: enforce [ws:<wsTag>] scope (when [ws:...] is present)
		// and exact full-line suffix boundary so prefixes never collide.
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

func (m *memoryStore) upsertNote(slug, title, summary, body string, now time.Time) (string, error) {
	safeSlug := sanitizeSlug(slug)
	if safeSlug == "" {
		safeSlug = fmt.Sprintf("note-%d", now.UTC().Unix())
	}
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		cleanTitle = safeSlug
	}
	cleanSummary := collapseWhitespace(summary)
	if cleanSummary == "" {
		cleanSummary = collapseWhitespace(truncateString(body, 120))
	}

	notePath := filepath.Join(m.notesDir(), safeSlug+".md")
	noteDoc := fmt.Sprintf("# %s\n\n_Updated: %s_\n\n%s\n",
		cleanTitle,
		now.UTC().Format(time.RFC3339),
		strings.TrimSpace(body),
	)

	err := store.WithFlock(m.lockPath(), true, func() error {
		if err := store.AtomicWriteFile(notePath, []byte(noteDoc), 0o600); err != nil {
			return err
		}

		idxContent := "# Memory Index (全局记忆索引)\n\nPointers to durable engineering notes stored under `notes/*.md`.\n"
		if raw, rErr := os.ReadFile(m.indexPath()); rErr == nil && strings.TrimSpace(string(raw)) != "" {
			idxContent = string(raw)
		}

		relLink := fmt.Sprintf("(notes/%s.md)", safeSlug)
		pointerLine := fmt.Sprintf("- [%s](notes/%s.md) — %s", cleanTitle, safeSlug, cleanSummary)

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
	})
	if err != nil {
		return "", err
	}
	return notePath, nil
}

func (m *memoryStore) writeWorkspaceProgress(wsID, slug, content string) (string, error) {
	progDir, err := m.workspaceProgressDir(wsID)
	if err != nil {
		return "", err
	}
	safeSlug := sanitizeSlug(slug)
	if safeSlug == "" {
		safeSlug = "latest"
	}
	target := filepath.Join(progDir, safeSlug+".md")
	if err := store.AtomicWriteFile(target, []byte(strings.TrimSpace(content)+"\n"), 0o600); err != nil {
		return "", err
	}
	return target, nil
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
