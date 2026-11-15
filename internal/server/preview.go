package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorness.dev/gorness/internal/config"
)

// PreviewShellCSP 严格锁定外层预览壳与内层 srcdoc 的内容安全策略，阻断一切网络外联、表单提交与嵌套外框。
const PreviewShellCSP = "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; img-src data:; connect-src 'none'; frame-src 'none'; form-action 'none'"

const maxPreviewEntries = 256

var previewFallbackSeq atomic.Uint64

type PreviewItem struct {
	Token       string    `json:"token"`
	WorkspaceID string    `json:"workspace_id,omitempty"`
	Title       string    `json:"title"`
	HTML        string    `json:"-"`
	FilePath    string    `json:"file_path,omitempty"`
	URL         string    `json:"url"`
	CreatedAt   time.Time `json:"created_at"`
}

// PreviewStore 管理内存态高熵 HTML 产物预览令牌，并在 /preview/:token 中渲染无 allow-same-origin 的双层隔离沙箱壳。
type PreviewStore struct {
	mu     sync.RWMutex
	now    func() time.Time
	byTok  map[string]PreviewItem
	byPath map[string]string
	order  []string
}

func NewPreviewStore(nowFn func() time.Time) *PreviewStore {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &PreviewStore{
		now:    nowFn,
		byTok:  make(map[string]PreviewItem),
		byPath: make(map[string]string),
	}
}

func (s *PreviewStore) Register(wsID, title, rawHTML, filePath string) PreviewItem {
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		cleanTitle = "Untitled HTML Artifact"
	}
	cleanPath := strings.TrimSpace(filePath)
	now := s.now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	if cleanPath != "" {
		if existingTok, ok := s.byPath[cleanPath]; ok {
			if existing, has := s.byTok[existingTok]; has {
				existing.Title = cleanTitle
				existing.HTML = rawHTML
				existing.WorkspaceID = strings.TrimSpace(wsID)
				existing.CreatedAt = now
				s.byTok[existingTok] = existing
				return existing
			}
		}
	}

	tok := generatePreviewToken()
	item := PreviewItem{
		Token:       tok,
		WorkspaceID: strings.TrimSpace(wsID),
		Title:       cleanTitle,
		HTML:        rawHTML,
		FilePath:    cleanPath,
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
	return item
}

func (s *PreviewStore) Get(token string) (PreviewItem, bool) {
	if !isSafeHexToken(token) {
		return PreviewItem{}, false
	}
	s.mu.RLock()
	item, ok := s.byTok[token]
	s.mu.RUnlock()
	return item, ok
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
	lower := strings.ToLower(rawHTML)
	if idx := strings.Index(lower, "<head>"); idx >= 0 {
		insertAt := idx + len("<head>")
		return rawHTML[:insertAt] + "\n" + metaTag + "\n" + rawHTML[insertAt:]
	}
	return "<!DOCTYPE html><html><head><meta charset=\"utf-8\">" + metaTag + "</head><body>\n" + rawHTML + "\n</body></html>"
}

func generatePreviewToken() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%032x", previewFallbackSeq.Add(1))
	}
	return hex.EncodeToString(buf)
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
