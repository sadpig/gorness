package server

import (
	"bytes"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pelletier/go-toml/v2"
	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/store"
)

// PreviewShellCSP 严格锁定外层预览壳与内层 srcdoc 的内容安全策略，阻断一切网络外联、表单提交与嵌套外框。
const PreviewShellCSP = "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; img-src data:; connect-src 'none'; frame-src 'none'; form-action 'none'"

const (
	maxPreviewEntries           = 256
	maxPersistedInlineHTMLBytes = 2 * 1024 * 1024 // 2 MiB inline HTML cap in previews.toml
)

var previewFallbackSeq atomic.Uint64

type PreviewItem struct {
	Token       string    `json:"token" toml:"token"`
	WorkspaceID string    `json:"workspace_id,omitempty" toml:"workspace_id,omitempty"`
	Title       string    `json:"title" toml:"title"`
	HTML        string    `json:"-" toml:"html,omitempty"`
	FilePath    string    `json:"file_path,omitempty" toml:"file_path,omitempty"`
	SavedPath   string    `json:"saved_path,omitempty" toml:"-"`
	URL         string    `json:"url" toml:"-"`
	CreatedAt   time.Time `json:"created_at" toml:"created_at"`
}

type previewRegistryDoc struct {
	Items []PreviewItem `toml:"items"`
}

// PreviewStore 管理高熵 HTML 产物预览令牌（持久化至 <vault>/runtime/previews.toml，零 .json），
// 并在 /preview/:token 中渲染无 allow-same-origin 的双层隔离沙箱壳。
type PreviewStore struct {
	mu          sync.RWMutex
	now         func() time.Time
	persistPath string
	lockPath    string
	byTok       map[string]PreviewItem
	byPath      map[string]string
	order       []string
}

func NewPreviewStore(nowFn func() time.Time) *PreviewStore {
	return NewPersistentPreviewStore(config.VaultPaths{}, nowFn)
}

func NewPersistentPreviewStore(paths config.VaultPaths, nowFn func() time.Time) *PreviewStore {
	if nowFn == nil {
		nowFn = time.Now
	}
	s := &PreviewStore{
		now:    nowFn,
		byTok:  make(map[string]PreviewItem),
		byPath: make(map[string]string),
	}
	if strings.TrimSpace(paths.Root) != "" {
		s.persistPath = filepath.Join(paths.RuntimeDir(), "previews.toml")
		s.lockPath = filepath.Join(paths.RuntimeDir(), "locks", "previews.lock")
		s.loadPersisted()
	}
	return s
}

func (s *PreviewStore) loadPersisted() {
	if s.persistPath == "" {
		return
	}
	var raw []byte
	err := store.WithFlock(s.lockPath, false, func() error {
		data, rErr := os.ReadFile(s.persistPath)
		if rErr != nil {
			if os.IsNotExist(rErr) {
				return nil
			}
			return rErr
		}
		raw = data
		return nil
	})
	if err != nil || len(bytes.TrimSpace(raw)) == 0 {
		return
	}

	var doc previewRegistryDoc
	if err := toml.Unmarshal(raw, &doc); err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range doc.Items {
		tok := strings.TrimSpace(item.Token)
		if !isSafeHexToken(tok) {
			continue
		}
		if _, dup := s.byTok[tok]; dup {
			continue
		}
		item.Token = tok
		item.WorkspaceID = strings.TrimSpace(item.WorkspaceID)
		item.Title = strings.TrimSpace(item.Title)
		if item.Title == "" {
			item.Title = "Untitled HTML Artifact"
		}
		item.FilePath = strings.TrimSpace(item.FilePath)
		item.SavedPath = item.FilePath
		item.URL = "/preview/" + tok
		if item.FilePath != "" {
			if diskHTML, rErr := os.ReadFile(item.FilePath); rErr == nil {
				item.HTML = string(diskHTML)
			}
		}
		if item.HTML == "" {
			continue
		}
		if len(s.order) >= maxPreviewEntries {
			oldest := s.order[0]
			s.order = s.order[1:]
			if evicted, ok := s.byTok[oldest]; ok && evicted.FilePath != "" {
				delete(s.byPath, evicted.FilePath)
			}
			delete(s.byTok, oldest)
		}
		s.byTok[tok] = item
		s.order = append(s.order, tok)
		if item.FilePath != "" {
			s.byPath[item.FilePath] = tok
		}
	}
}

func (s *PreviewStore) persistLocked() {
	if s.persistPath == "" {
		return
	}
	items := make([]PreviewItem, 0, len(s.order))
	for _, tok := range s.order {
		item, ok := s.byTok[tok]
		if !ok {
			continue
		}
		persisted := PreviewItem{
			Token:       item.Token,
			WorkspaceID: item.WorkspaceID,
			Title:       item.Title,
			FilePath:    item.FilePath,
			CreatedAt:   item.CreatedAt,
		}
		if persisted.FilePath == "" || len(item.HTML) <= maxPersistedInlineHTMLBytes {
			if len(item.HTML) <= maxPersistedInlineHTMLBytes {
				persisted.HTML = item.HTML
			}
		}
		items = append(items, persisted)
	}
	encoded, err := toml.Marshal(previewRegistryDoc{Items: items})
	if err != nil {
		return
	}
	_ = store.WithFlock(s.lockPath, true, func() error {
		return store.AtomicWriteFile(s.persistPath, encoded, 0o600)
	})
}

func (s *PreviewStore) Register(wsID, title, rawHTML, filePath string) PreviewItem {
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		cleanTitle = "Untitled HTML Artifact"
	}
	cleanWS := strings.TrimSpace(wsID)
	cleanPath := strings.TrimSpace(filePath)
	now := s.now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	if cleanPath != "" {
		if existingTok, ok := s.byPath[cleanPath]; ok {
			if existing, has := s.byTok[existingTok]; has {
				changed := existing.Title != cleanTitle || existing.HTML != rawHTML || existing.WorkspaceID != cleanWS
				existing.Title = cleanTitle
				existing.HTML = rawHTML
				existing.WorkspaceID = cleanWS
				existing.FilePath = cleanPath
				existing.SavedPath = cleanPath
				existing.URL = "/preview/" + existingTok
				if changed {
					existing.CreatedAt = now
					s.byTok[existingTok] = existing
					s.persistLocked()
				}
				return existing
			}
		}
	}

	tok := generatePreviewToken()
	item := PreviewItem{
		Token:       tok,
		WorkspaceID: cleanWS,
		Title:       cleanTitle,
		HTML:        rawHTML,
		FilePath:    cleanPath,
		SavedPath:   cleanPath,
		URL:         "/preview/" + tok,
		CreatedAt:   now,
	}

	if len(s.order) >= maxPreviewEntries {
		oldest := s.order[0]
		s.order = s.order[1:]
		if evicted, ok := s.byTok[oldest]; ok && evicted.FilePath != "" {
			delete(s.byPath, evicted.FilePath)
		}
		delete(s.byTok, oldest)
	}

	s.byTok[tok] = item
	s.order = append(s.order, tok)
	if cleanPath != "" {
		s.byPath[cleanPath] = tok
	}
	s.persistLocked()
	return item
}

func (s *PreviewStore) FindByFilePath(filePath string) (PreviewItem, bool) {
	cleanPath := strings.TrimSpace(filePath)
	if cleanPath == "" {
		return PreviewItem{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	tok, ok := s.byPath[cleanPath]
	if !ok {
		return PreviewItem{}, false
	}
	item, has := s.byTok[tok]
	return item, has
}

func (s *PreviewStore) Get(token string) (PreviewItem, bool) {
	if !isSafeHexToken(token) {
		return PreviewItem{}, false
	}
	s.mu.RLock()
	item, ok := s.byTok[token]
	s.mu.RUnlock()
	if !ok {
		return PreviewItem{}, false
	}
	if item.FilePath != "" {
		if diskRaw, err := os.ReadFile(item.FilePath); err == nil && len(diskRaw) > 0 {
			item.HTML = string(diskRaw)
		}
	}
	return item, true
}

func (s *PreviewStore) List(wsID string) []PreviewItem {
	filterWS := strings.TrimSpace(wsID)
	s.mu.RLock()
	out := make([]PreviewItem, 0, len(s.byTok))
	for _, item := range s.byTok {
		if filterWS != "" && item.WorkspaceID != filterWS {
			continue
		}
		cp := item
		cp.HTML = ""
		cp.SavedPath = cp.FilePath
		cp.URL = "/preview/" + cp.Token
		out = append(out, cp)
	}
	s.mu.RUnlock()

	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].Token < out[j].Token
	})
	return out
}

func (s *PreviewStore) ServePreview(w http.ResponseWriter, token string) {
	item, ok := s.Get(strings.TrimSpace(token))
	if !ok {
		w.Header().Set("Content-Security-Policy", PreviewShellCSP)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("preview token not found or expired"))
		return
	}

	w.Header().Set("Content-Security-Policy", PreviewShellCSP)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	safeTitle := html.EscapeString(config.EscapeBidi(item.Title))
	innerDoc := wrapSrcDocWithCSP(item.HTML)
	escapedSrcDoc := html.EscapeString(innerDoc)

	var b strings.Builder
	b.Grow(len(escapedSrcDoc) + 1600)
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	b.WriteString("  <meta charset=\"utf-8\">\n")
	b.WriteString("  <meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	fmt.Fprintf(&b, "  <meta http-equiv=\"Content-Security-Policy\" content=\"%s\">\n", html.EscapeString(PreviewShellCSP))
	fmt.Fprintf(&b, "  <title>%s — Gorness Isolated Preview</title>\n", safeTitle)
	b.WriteString(`  <style>
    :root {
      color-scheme: light dark;
      --bg: #f5f5f7;
      --bar: #ffffff;
      --fg: #1d1d1f;
      --muted: #6e6e73;
      --line: rgba(0, 0, 0, 0.09);
      --pill-bg: rgba(36, 138, 61, 0.12);
      --pill-fg: #248a3d;
    }
    @media (prefers-color-scheme: dark) {
      :root {
        --bg: #0a0a0e;
        --bar: #15151b;
        --fg: #f5f5f7;
        --muted: #98989d;
        --line: rgba(255, 255, 255, 0.10);
        --pill-bg: rgba(48, 209, 88, 0.16);
        --pill-fg: #30d158;
      }
    }
    * { box-sizing: border-box; }
    html, body {
      margin: 0;
      height: 100%;
      background: var(--bg);
      color: var(--fg);
      font-family: -apple-system, BlinkMacSystemFont, "SF Pro Text", "Helvetica Neue", sans-serif;
      display: flex;
      flex-direction: column;
      overflow: hidden;
    }
    header {
      height: 38px;
      padding: 0 14px;
      background: var(--bar);
      border-bottom: 1px solid var(--line);
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      font-size: 12.5px;
      flex-shrink: 0;
    }
    .title {
      font-weight: 600;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    .badge {
      padding: 2px 8px;
      border-radius: 999px;
      background: var(--pill-bg);
      color: var(--pill-fg);
      font-family: "SF Mono", ui-monospace, Menlo, monospace;
      font-size: 11px;
      white-space: nowrap;
    }
    iframe.preview-frame {
      flex: 1;
      width: 100%;
      border: 0;
      background: #ffffff;
    }
  </style>
</head>
<body>
  <header>
`)
	fmt.Fprintf(&b, "    <span class=\"title\">%s</span>\n", safeTitle)
	b.WriteString("    <span class=\"badge\">sandbox=\"allow-scripts\" · opaque-origin · connect-src 'none'</span>\n")
	b.WriteString("  </header>\n")
	// 安全铁律：仅允许 allow-scripts，严禁附加 allow-same-origin！
	fmt.Fprintf(&b, "  <iframe class=\"preview-frame\" sandbox=\"allow-scripts\" referrerpolicy=\"no-referrer\" srcdoc=\"%s\"></iframe>\n", escapedSrcDoc)
	b.WriteString("</body>\n</html>\n")

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}

func wrapSrcDocWithCSP(rawHTML string) string {
	metaTag := fmt.Sprintf(`<meta http-equiv="Content-Security-Policy" content="%s">`, html.EscapeString(PreviewShellCSP))
	if endIdx, ok := findOpeningHTMLTagEnd(rawHTML, "head"); ok {
		return rawHTML[:endIdx] + "\n" + metaTag + "\n" + rawHTML[endIdx:]
	}
	if endIdx, ok := findOpeningHTMLTagEnd(rawHTML, "html"); ok {
		return rawHTML[:endIdx] + "\n<head><meta charset=\"utf-8\">" + metaTag + "</head>\n" + rawHTML[endIdx:]
	}
	return "<!DOCTYPE html><html><head><meta charset=\"utf-8\">" + metaTag + "</head><body>\n" + rawHTML + "\n</body></html>"
}

// findOpeningHTMLTagEnd 定位形如 <tag> 或 <tag attr="val"> 的开标签闭合 '>' 下标后一位，
// 严格校验标签名边界（例如匹配 <head ...> 但绝不误匹配 <header>）并感知单双引号属性值中的 '>'。
func findOpeningHTMLTagEnd(doc, tagName string) (int, bool) {
	lower := strings.ToLower(doc)
	needle := "<" + strings.ToLower(tagName)
	searchFrom := 0
	for searchFrom < len(lower) {
		rel := strings.Index(lower[searchFrom:], needle)
		if rel < 0 {
			return 0, false
		}
		tagStart := searchFrom + rel
		afterName := tagStart + len(needle)
		if afterName >= len(lower) {
			return 0, false
		}
		boundary := lower[afterName]
		if boundary != '>' && boundary != ' ' && boundary != '\t' && boundary != '\n' && boundary != '\r' && boundary != '/' {
			searchFrom = afterName
			continue
		}
		var quote byte
		for i := afterName; i < len(doc); i++ {
			ch := doc[i]
			if quote != 0 {
				if ch == quote {
					quote = 0
				}
				continue
			}
			if ch == '"' || ch == '\'' {
				quote = ch
				continue
			}
			if ch == '>' {
				return i + 1, true
			}
		}
		return 0, false
	}
	return 0, false
}

func generatePreviewToken() string {
	tok := store.RandomHex(16)
	if isSafeHexToken(tok) {
		return tok
	}
	return fmt.Sprintf("%032x", previewFallbackSeq.Add(1))
}

func isSafeHexToken(tok string) bool {
	if len(tok) < 16 || len(tok) > 128 {
		return false
	}
	for i := 0; i < len(tok); i++ {
		c := tok[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
