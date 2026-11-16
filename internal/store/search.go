package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"gorness.dev/gorness/internal/config"
)

type SearchHit struct {
	Kind        string    `json:"kind,omitempty"`
	SessionID   string    `json:"session_id"`
	WorkspaceID string    `json:"workspace_id"`
	Title       string    `json:"title"`
	Path        string    `json:"path,omitempty"`
	EntryID     string    `json:"entry_id,omitempty"`
	EntrySeq    uint64    `json:"entry_seq,omitempty"`
	Snippet     string    `json:"snippet"`
	Score       float64   `json:"score"`
	UpdatedAt   time.Time `json:"updated_at"`
}

var bm25Stopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "from": true,
	"into": true, "that": true, "this": true, "tool": true, "please": true,
	"use": true, "run": true, "get": true, "how": true, "what": true,
}

var bm25Synonyms = map[string][]string{
	"kubernetes": {"k8s"},
	"k8s":        {"kubernetes"},
	"postgres":   {"postgresql", "sql", "db"},
	"postgresql": {"postgres", "sql", "db"},
	"database":   {"db", "sql", "postgres"},
	"db":         {"database", "sql", "postgres"},
}

// TokenizeQuery splits English/code identifiers (camelCase, snake_case, digits), extracts CJK bigrams/unigrams,
// filters common stopwords, and expands domain synonyms for unified BM25 and full-text search.
func TokenizeQuery(query string) []string {
	if strings.TrimSpace(query) == "" {
		return nil
	}
	rawTokens := extractQuerySubTokens(query)
	seen := make(map[string]struct{}, len(rawTokens)*2)
	out := make([]string, 0, len(rawTokens))
	add := func(tok string) {
		if tok == "" || bm25Stopwords[tok] {
			return
		}
		if _, ok := seen[tok]; ok {
			return
		}
		seen[tok] = struct{}{}
		out = append(out, tok)
	}
	for _, tok := range rawTokens {
		add(tok)
		for _, syn := range bm25Synonyms[tok] {
			add(syn)
		}
	}
	return out
}

type markdownSearchDoc struct {
	kind        string
	id          string
	workspaceID string
	path        string
	defaultName string
}

// Search 并发流式扫描全项目活跃会话分支以及全局/工作区指针记忆（memory/notes/*.md、progress/*.md、artifacts/*.md），
// 融合中英文分词、代码标识符精确匹配与标题加权排序。
func (s *SessionStore) Search(query string, limit int) ([]SearchHit, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return []SearchHit{}, nil
	}
	if limit <= 0 {
		limit = 20
	}

	s.mu.RLock()
	metas := make([]*SessionMeta, 0, len(s.metas))
	for _, m := range s.metas {
		metas = append(metas, cloneSessionMeta(m))
	}
	s.mu.RUnlock()

	mdDocs := s.discoverSearchableMarkdownDocs()
	if len(metas) == 0 && len(mdDocs) == 0 {
		return []SearchHit{}, nil
	}

	lowerQ := strings.ToLower(trimmed)
	clauses := strings.Fields(lowerQ)
	subTokens := TokenizeQuery(trimmed)

	var hitsMu sync.Mutex
	hits := make([]SearchHit, 0)

	if len(metas) > 0 {
		workerCount := runtime.NumCPU()
		if workerCount < 2 {
			workerCount = 2
		}
		if workerCount > 8 {
			workerCount = 8
		}
		if workerCount > len(metas) {
			workerCount = len(metas)
		}

		jobs := make(chan *SessionMeta, len(metas))
		for _, m := range metas {
			jobs <- m
		}
		close(jobs)

		var wg sync.WaitGroup
		for i := 0; i < workerCount; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for meta := range jobs {
					hit, ok := s.scoreSessionForQuery(meta, trimmed, lowerQ, clauses, subTokens)
					if ok {
						hitsMu.Lock()
						hits = append(hits, hit)
						hitsMu.Unlock()
					}
				}
			}()
		}
		wg.Wait()
	}

	for _, doc := range mdDocs {
		if hit, ok := scoreMarkdownDocForQuery(doc, lowerQ, clauses, subTokens); ok {
			hits = append(hits, hit)
		}
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if !hits[i].UpdatedAt.Equal(hits[j].UpdatedAt) {
			return hits[i].UpdatedAt.After(hits[j].UpdatedAt)
		}
		if hits[i].SessionID != hits[j].SessionID {
			return hits[i].SessionID < hits[j].SessionID
		}
		return hits[i].Path < hits[j].Path
	})

	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func (s *SessionStore) discoverSearchableMarkdownDocs() []markdownSearchDoc {
	var docs []markdownSearchDoc
	collectDir := func(dir, kind, wsID string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, ent := range entries {
			if ent.IsDir() || !strings.HasSuffix(strings.ToLower(ent.Name()), ".md") {
				continue
			}
			stem := strings.TrimSuffix(ent.Name(), filepath.Ext(ent.Name()))
			docs = append(docs, markdownSearchDoc{
				kind:        kind,
				id:          kind + ":" + stem,
				workspaceID: wsID,
				path:        filepath.Join(dir, ent.Name()),
				defaultName: stem,
			})
		}
	}

	collectDir(filepath.Join(s.paths.MemoryDir(), "notes"), "memory_note", "")

	if wsEntries, err := os.ReadDir(s.paths.WorkspacesDir()); err == nil {
		for _, ent := range wsEntries {
			if !ent.IsDir() || ValidateSafeID(ent.Name()) != nil {
				continue
			}
			wsID := ent.Name()
			wsBase := filepath.Join(s.paths.WorkspacesDir(), wsID)
			collectDir(filepath.Join(wsBase, "memory", "progress"), "progress", wsID)
			collectDir(filepath.Join(wsBase, "artifacts"), "artifact", wsID)
		}
	}
	return docs
}

func scoreMarkdownDocForQuery(
	doc markdownSearchDoc,
	lowerQ string,
	clauses []string,
	subTokens []string,
) (SearchHit, bool) {
	info, err := os.Lstat(doc.path)
	if err != nil || !info.Mode().IsRegular() {
		return SearchHit{}, false
	}
	raw, err := os.ReadFile(doc.path)
	if err != nil {
		return SearchHit{}, false
	}
	text := string(raw)
	title := extractMarkdownTitle(text, doc.defaultName)
	titleLower := strings.ToLower(title)
	bodyLower := strings.ToLower(text)

	clauseMatched := make([]bool, len(clauses))
	var score float64
	if titleLower == lowerQ {
		score += 200
	} else if strings.Contains(titleLower, lowerQ) {
		score += 130
	}
	for idx, c := range clauses {
		if strings.Contains(titleLower, c) {
			clauseMatched[idx] = true
			score += 40
		}
		if containsIdentifierBoundary(title, c) {
			score += 12
		}
		if strings.Contains(bodyLower, c) {
			clauseMatched[idx] = true
			score += 18
		}
		if containsIdentifierBoundary(text, c) {
			score += 12
		}
	}
	if strings.Contains(bodyLower, lowerQ) {
		score += 36
	}
	for _, tok := range subTokens {
		if strings.Contains(titleLower, tok) {
			score += 5
		}
		if strings.Contains(bodyLower, tok) {
			score += 2
		}
	}

	for _, ok := range clauseMatched {
		if !ok {
			return SearchHit{}, false
		}
	}
	if score <= 0 {
		return SearchHit{}, false
	}

	needle := lowerQ
	if !strings.Contains(bodyLower, needle) && len(clauses) > 0 {
		needle = clauses[0]
	}
	snippet := config.EscapeBidi(extractRuneSafeSnippet(text, needle, 160))

	return SearchHit{
		Kind:        doc.kind,
		SessionID:   doc.id,
		WorkspaceID: doc.workspaceID,
		Title:       config.EscapeBidi(title),
		Path:        doc.path,
		Snippet:     snippet,
		Score:       score,
		UpdatedAt:   info.ModTime().UTC(),
	}, true
}

func extractMarkdownTitle(raw, fallback string) string {
	meta, body, err := config.ParseFrontmatterMD([]byte(raw))
	if err == nil {
		if t, ok := meta["title"].(string); ok && strings.TrimSpace(t) != "" {
			return strings.TrimSpace(t)
		}
	} else {
		body = raw
	}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			h := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			if h != "" {
				return h
			}
		}
	}
	return fallback
}

func (s *SessionStore) scoreSessionForQuery(
	meta *SessionMeta,
	rawQuery string,
	lowerQ string,
	clauses []string,
	subTokens []string,
) (SearchHit, bool) {
	entries, err := s.LoadActiveBranch(meta.ID)
	if err != nil {
		return SearchHit{}, false
	}

	titleLower := strings.ToLower(meta.Title)
	clauseMatched := make([]bool, len(clauses))
	for idx, c := range clauses {
		if strings.Contains(titleLower, c) {
			clauseMatched[idx] = true
		}
	}

	var score float64
	if titleLower == lowerQ {
		score += 220
	} else if strings.Contains(titleLower, lowerQ) {
		score += 140
	}
	for _, c := range clauses {
		if strings.Contains(titleLower, c) {
			score += 45
		}
		if containsIdentifierBoundary(meta.Title, c) {
			score += 15
		}
	}
	for _, tok := range subTokens {
		if strings.Contains(titleLower, tok) {
			score += 6
		}
	}

	var bestEntryScore float64
	var bestEntryID string
	var bestEntrySeq uint64
	var bestEntryText string
	matchedEntries := 0

	for _, e := range entries {
		text := collectEntrySearchableText(e)
		if text == "" {
			continue
		}
		textLower := strings.ToLower(text)

		eScore := 0.0
		if strings.Contains(textLower, lowerQ) {
			eScore += 36
		}
		for idx, c := range clauses {
			if strings.Contains(textLower, c) {
				clauseMatched[idx] = true
				eScore += 18
			}
			if containsIdentifierBoundary(text, c) {
				eScore += 12
			}
		}
		for _, tok := range subTokens {
			if strings.Contains(textLower, tok) {
				eScore += 2
			}
		}

		if eScore > 0 {
			matchedEntries++
			if eScore >= bestEntryScore {
				bestEntryScore = eScore
				bestEntryID = e.ID
				bestEntrySeq = e.Seq
				bestEntryText = text
			}
		}
	}

	for _, ok := range clauseMatched {
		if !ok {
			return SearchHit{}, false
		}
	}
	if matchedEntries > 0 {
		extra := matchedEntries - 1
		if extra > 4 {
			extra = 4
		}
		score += bestEntryScore + float64(extra)*5.0
	}
	if score <= 0 {
		return SearchHit{}, false
	}

	snippetSource := bestEntryText
	if snippetSource == "" {
		snippetSource = meta.Title
	}
	needle := lowerQ
	if !strings.Contains(strings.ToLower(snippetSource), needle) && len(clauses) > 0 {
		needle = clauses[0]
	}
	snippet := config.EscapeBidi(extractRuneSafeSnippet(snippetSource, needle, 160))
	_ = rawQuery

	return SearchHit{
		SessionID:   meta.ID,
		WorkspaceID: meta.WorkspaceID,
		Title:       config.EscapeBidi(meta.Title),
		EntryID:     bestEntryID,
		EntrySeq:    bestEntrySeq,
		Snippet:     snippet,
		Score:       score,
		UpdatedAt:   meta.UpdatedAt,
	}, true
}

func collectEntrySearchableText(e SessionEntry) string {
	var parts []string
	for _, b := range e.Blocks {
		if b.Text != "" {
			parts = append(parts, b.Text)
		}
		if b.Thinking != "" {
			parts = append(parts, b.Thinking)
		}
		if b.ToolName != "" {
			parts = append(parts, b.ToolName)
		}
		if len(b.ToolInput) > 0 {
			parts = append(parts, string(b.ToolInput))
		}
		if len(b.CompactionData) > 0 {
			parts = append(parts, string(b.CompactionData))
		}
	}
	return strings.Join(parts, "\n")
}

func extractQuerySubTokens(q string) []string {
	seen := make(map[string]struct{})
	add := func(s string) {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			return
		}
		if _, exists := seen[s]; !exists {
			seen[s] = struct{}{}
		}
	}

	// Split identifier boundaries (camelCase / snake_case) and CJK bigrams.
	runes := []rune(q)
	var identBuf []rune
	var cjkBuf []rune

	flushIdent := func() {
		if len(identBuf) >= 2 {
			add(string(identBuf))
		}
		identBuf = identBuf[:0]
	}
	flushCJK := func() {
		if len(cjkBuf) == 1 {
			add(string(cjkBuf))
		} else if len(cjkBuf) >= 2 {
			for i := 0; i+1 < len(cjkBuf); i++ {
				add(string(cjkBuf[i : i+2]))
			}
		}
		cjkBuf = cjkBuf[:0]
	}

	for i, r := range runes {
		switch {
		case isCJKRune(r):
			flushIdent()
			cjkBuf = append(cjkBuf, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushCJK()
			if len(identBuf) > 0 && unicode.IsUpper(r) && i > 0 && unicode.IsLower(runes[i-1]) {
				flushIdent()
			}
			identBuf = append(identBuf, r)
		default:
			flushIdent()
			flushCJK()
		}
	}
	flushIdent()
	flushCJK()

	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func isCJKRune(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		(r >= 0x3040 && r <= 0x30ff) ||
		(r >= 0xac00 && r <= 0xd7af)
}

func isIdentRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

func containsIdentifierBoundary(text, lowerNeedle string) bool {
	if lowerNeedle == "" {
		return false
	}
	for _, r := range lowerNeedle {
		if !isIdentRune(r) {
			return false
		}
	}
	lowerText := strings.ToLower(text)
	searchFrom := 0
	for {
		idx := strings.Index(lowerText[searchFrom:], lowerNeedle)
		if idx < 0 {
			return false
		}
		pos := searchFrom + idx
		end := pos + len(lowerNeedle)
		leftOK := pos == 0 || !isIdentRune(rune(lowerText[pos-1]))
		rightOK := end == len(lowerText) || !isIdentRune(rune(lowerText[end]))
		if leftOK && rightOK {
			return true
		}
		searchFrom = pos + 1
		if searchFrom >= len(lowerText) {
			return false
		}
	}
}

func extractRuneSafeSnippet(source, lowerNeedle string, maxRunes int) string {
	clean := strings.Join(strings.Fields(source), " ")
	runes := []rune(clean)
	if len(runes) <= maxRunes {
		return clean
	}

	lowerRunes := []rune(strings.ToLower(clean))
	needleRunes := []rune(lowerNeedle)
	matchIdx := indexRuneSlice(lowerRunes, needleRunes)
	if matchIdx < 0 {
		return string(runes[:maxRunes]) + "..."
	}

	half := maxRunes / 2
	start := matchIdx - half
	if start < 0 {
		start = 0
	}
	end := start + maxRunes
	if end > len(runes) {
		end = len(runes)
		if end-maxRunes > 0 {
			start = end - maxRunes
		} else {
			start = 0
		}
	}

	var b strings.Builder
	if start > 0 {
		b.WriteString("...")
	}
	b.WriteString(string(runes[start:end]))
	if end < len(runes) {
		b.WriteString("...")
	}
	return b.String()
}

func indexRuneSlice(haystack, needle []rune) int {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return -1
	}
	limit := len(haystack) - len(needle)
	for i := 0; i <= limit; i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// ExportMarkdown 将会话活跃分支导出为自包含 Markdown 归档，并对全部文本执行 Bidi 双向控制符净化。
func (s *SessionStore) ExportMarkdown(sessionID string) (string, error) {
	meta, err := s.GetMeta(sessionID)
	if err != nil {
		return "", err
	}
	entries, err := s.LoadActiveBranch(sessionID)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", config.EscapeBidi(meta.Title))
	fmt.Fprintf(&b, "- **Session ID:** `%s`\n", config.EscapeBidi(meta.ID))
	fmt.Fprintf(&b, "- **Workspace ID:** `%s`\n", config.EscapeBidi(meta.WorkspaceID))
	if meta.AgentRole != "" {
		fmt.Fprintf(&b, "- **Agent Role:** `%s`\n", config.EscapeBidi(meta.AgentRole))
	}
	if meta.ParentSessionID != "" {
		fmt.Fprintf(&b, "- **Forked From:** `%s` (at seq `%d`)\n", config.EscapeBidi(meta.ParentSessionID), meta.ForkedAtSeq)
	}
	fmt.Fprintf(&b, "- **Head Seq:** `%d`\n", meta.HeadSeq)
	fmt.Fprintf(&b, "- **Total Tokens:** `%d`\n", meta.TotalTokens)
	fmt.Fprintf(&b, "- **Total Cost (USD):** `$%.4f`\n", meta.TotalCostUSD)
	fmt.Fprintf(&b, "- **Updated At:** `%s`\n\n", meta.UpdatedAt.UTC().Format(time.RFC3339))
	b.WriteString("---\n\n")

	for _, e := range entries {
		roleLabel := config.EscapeBidi(e.Role)
		headerExtra := ""
		if e.AgentRole != "" || e.ModelID != "" {
			headerExtra = fmt.Sprintf(" (`%s` %s)", config.EscapeBidi(e.AgentRole), config.EscapeBidi(e.ModelID))
		}
		fmt.Fprintf(&b, "## [Seq #%d] %s%s\n\n", e.Seq, roleLabel, headerExtra)

		for _, blk := range e.Blocks {
			switch blk.Type {
			case "thinking":
				safeThink := config.EscapeBidi(blk.Thinking)
				fence := markdownFenceFor(safeThink)
				b.WriteString("<details>\n<summary>Thinking</summary>\n\n")
				fmt.Fprintf(&b, "%stext\n%s\n%s\n\n", fence, safeThink, fence)
				b.WriteString("</details>\n\n")
			case "tool_use":
				pretty := prettyJSONOrRaw(blk.ToolInput)
				safeInput := config.EscapeBidi(pretty)
				fence := markdownFenceFor(safeInput)
				fmt.Fprintf(&b, "**Tool Call:** `%s` (`%s`)\n\n%sjson\n%s\n%s\n\n",
					config.EscapeBidi(blk.ToolName),
					config.EscapeBidi(blk.ToolCallID),
					fence, safeInput, fence,
				)
			case "tool_result":
				status := "OK"
				if blk.IsError {
					status = "ERROR"
				}
				safeText := config.EscapeBidi(blk.Text)
				fence := markdownFenceFor(safeText)
				fmt.Fprintf(&b, "**Tool Result [%s]:** (`%s`)\n\n%stext\n%s\n%s\n\n",
					status,
					config.EscapeBidi(blk.ToolCallID),
					fence, safeText, fence,
				)
			case "compaction":
				pretty := prettyJSONOrRaw(blk.CompactionData)
				safeComp := config.EscapeBidi(pretty)
				fence := markdownFenceFor(safeComp)
				fmt.Fprintf(&b, "**Compaction Anchor:**\n\n%sjson\n%s\n%s\n\n", fence, safeComp, fence)
			default:
				if blk.Text != "" {
					fmt.Fprintf(&b, "%s\n\n", config.EscapeBidi(blk.Text))
				}
			}
		}
	}

	return b.String(), nil
}

// ExportHTML 将会话活跃分支导出为单文件自包含 Apple HIG 风格 HTML，并对所有文本执行 EscapeBidi + html.EscapeString 彻底免疫 XSS。
func (s *SessionStore) ExportHTML(sessionID string) (string, error) {
	meta, err := s.GetMeta(sessionID)
	if err != nil {
		return "", err
	}
	entries, err := s.LoadActiveBranch(sessionID)
	if err != nil {
		return "", err
	}

	safe := func(v string) string {
		return html.EscapeString(config.EscapeBidi(v))
	}

	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	b.WriteString("  <meta charset=\"utf-8\">\n")
	b.WriteString("  <meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	b.WriteString("  <meta http-equiv=\"Content-Security-Policy\" content=\"default-src 'none'; style-src 'unsafe-inline'; img-src data:;\">\n")
	fmt.Fprintf(&b, "  <title>%s — Gorness Session Archive</title>\n", safe(meta.Title))
	b.WriteString(`  <style>
    :root {
      color-scheme: light dark;
      --bg: #f5f5f7;
      --card: #ffffff;
      --fg: #1d1d1f;
      --muted: #6e6e73;
      --border: #d2d2d7;
      --accent: #0071e3;
      --code-bg: #f0f0f3;
      --err-bg: #fff2f2;
      --err-fg: #c93400;
    }
    @media (prefers-color-scheme: dark) {
      :root {
        --bg: #121214;
        --card: #1c1c1e;
        --fg: #f5f5f7;
        --muted: #98989d;
        --border: #38383a;
        --accent: #2997ff;
        --code-bg: #2c2c2e;
        --err-bg: #3a1818;
        --err-fg: #ff6961;
      }
    }
    body {
      margin: 0;
      padding: 32px 20px;
      font-family: -apple-system, BlinkMacSystemFont, "SF Pro Text", "Helvetica Neue", sans-serif;
      background: var(--bg);
      color: var(--fg);
      line-height: 1.55;
    }
    main { max-width: 880px; margin: 0 auto; }
    header {
      background: var(--card);
      border: 1px solid var(--border);
      border-radius: 14px;
      padding: 20px 24px;
      margin-bottom: 24px;
    }
    h1 { margin: 0 0 10px 0; font-size: 24px; font-weight: 600; }
    .meta { display: flex; flex-wrap: wrap; gap: 14px; font-size: 13px; color: var(--muted); }
    .entry {
      background: var(--card);
      border: 1px solid var(--border);
      border-radius: 14px;
      padding: 18px 22px;
      margin-bottom: 16px;
    }
    .entry-head {
      display: flex;
      justify-content: space-between;
      align-items: baseline;
      border-bottom: 1px solid var(--border);
      padding-bottom: 8px;
      margin-bottom: 12px;
      font-size: 13px;
      color: var(--muted);
    }
    .role { font-weight: 600; color: var(--accent); text-transform: uppercase; letter-spacing: 0.04em; }
    .text-block { white-space: pre-wrap; word-break: break-word; margin: 10px 0; }
    details.thinking {
      background: var(--code-bg);
      border-radius: 10px;
      padding: 10px 14px;
      margin: 10px 0;
    }
    details.thinking summary { cursor: pointer; font-size: 13px; font-weight: 500; color: var(--muted); }
    pre {
      background: var(--code-bg);
      border-radius: 10px;
      padding: 12px 14px;
      overflow-x: auto;
      font-family: "SF Mono", Menlo, Monaco, monospace;
      font-size: 12.5px;
      white-space: pre-wrap;
      word-break: break-word;
    }
    .tool-err pre { background: var(--err-bg); color: var(--err-fg); }
  </style>
</head>
<body>
  <main>
`)

	b.WriteString("    <header>\n")
	fmt.Fprintf(&b, "      <h1>%s</h1>\n", safe(meta.Title))
	b.WriteString("      <div class=\"meta\">\n")
	fmt.Fprintf(&b, "        <span>Session: <code>%s</code></span>\n", safe(meta.ID))
	fmt.Fprintf(&b, "        <span>Workspace: <code>%s</code></span>\n", safe(meta.WorkspaceID))
	if meta.AgentRole != "" {
		fmt.Fprintf(&b, "        <span>Role: <code>%s</code></span>\n", safe(meta.AgentRole))
	}
	if meta.ParentSessionID != "" {
		fmt.Fprintf(&b, "        <span>Forked From: <code>%s</code> (seq <code>%d</code>)</span>\n", safe(meta.ParentSessionID), meta.ForkedAtSeq)
	}
	fmt.Fprintf(&b, "        <span>Head Seq: <code>%d</code></span>\n", meta.HeadSeq)
	fmt.Fprintf(&b, "        <span>Tokens: <code>%d</code></span>\n", meta.TotalTokens)
	fmt.Fprintf(&b, "        <span>Cost: <code>$%.4f</code></span>\n", meta.TotalCostUSD)
	fmt.Fprintf(&b, "        <span>Updated: <code>%s</code></span>\n", safe(meta.UpdatedAt.UTC().Format(time.RFC3339)))
	b.WriteString("      </div>\n    </header>\n")

	for _, e := range entries {
		b.WriteString("    <section class=\"entry\">\n")
		b.WriteString("      <div class=\"entry-head\">\n")
		fmt.Fprintf(&b, "        <span><span class=\"role\">%s</span> #%d", safe(e.Role), e.Seq)
		if e.AgentRole != "" || e.ModelID != "" {
			fmt.Fprintf(&b, " &middot; %s %s", safe(e.AgentRole), safe(e.ModelID))
		}
		b.WriteString("</span>\n")
		fmt.Fprintf(&b, "        <span>%s</span>\n", safe(e.CreatedAt.UTC().Format(time.RFC3339)))
		b.WriteString("      </div>\n")

		for _, blk := range e.Blocks {
			switch blk.Type {
			case "thinking":
				b.WriteString("      <details class=\"thinking\">\n        <summary>Adaptive Thinking</summary>\n")
				fmt.Fprintf(&b, "        <pre>%s</pre>\n      </details>\n", safe(blk.Thinking))
			case "tool_use":
				b.WriteString("      <div class=\"tool-call\">\n")
				fmt.Fprintf(&b, "        <div><strong>Tool Call:</strong> <code>%s</code> (<code>%s</code>)</div>\n",
					safe(blk.ToolName), safe(blk.ToolCallID))
				fmt.Fprintf(&b, "        <pre>%s</pre>\n      </div>\n", safe(prettyJSONOrRaw(blk.ToolInput)))
			case "tool_result":
				cls := "tool-res"
				label := "Tool Result"
				if blk.IsError {
					cls = "tool-res tool-err"
					label = "Tool Error"
				}
				fmt.Fprintf(&b, "      <div class=\"%s\">\n        <div><strong>%s:</strong> (<code>%s</code>)</div>\n        <pre>%s</pre>\n      </div>\n",
					cls, label, safe(blk.ToolCallID), safe(blk.Text))
			case "compaction":
				fmt.Fprintf(&b, "      <div class=\"compaction\">\n        <div><strong>Compaction Anchor</strong></div>\n        <pre>%s</pre>\n      </div>\n",
					safe(prettyJSONOrRaw(blk.CompactionData)))
			default:
				if blk.Text != "" {
					fmt.Fprintf(&b, "      <div class=\"text-block\">%s</div>\n", safe(blk.Text))
				}
			}
		}
		b.WriteString("    </section>\n")
	}

	b.WriteString("  </main>\n</body>\n</html>\n")
	return b.String(), nil
}

func prettyJSONOrRaw(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err == nil {
		return buf.String()
	}
	return string(raw)
}

func markdownFenceFor(text string) string {
	fence := "```"
	for strings.Contains(text, fence) {
		fence += "`"
	}
	return fence
}
