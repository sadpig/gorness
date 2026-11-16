package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"

	"gorness.dev/gorness/internal/config"
)

const (
	maxSearchableFileBytes = 2 * 1024 * 1024
	defaultSearchLimit     = 100
	defaultRepoMapTokens   = 2048
)

type searchCodeInput struct {
	Pattern       string `json:"pattern,omitempty"`
	Path          string `json:"path,omitempty"`
	Include       string `json:"include,omitempty"`
	Literal       bool   `json:"literal,omitempty"`
	CaseSensitive *bool  `json:"case_sensitive,omitempty"`
	Mode          string `json:"mode,omitempty"` // "grep" | "glob" | "repomap"
	Limit         int    `json:"limit,omitempty"`
	MaxTokens     int    `json:"max_tokens,omitempty"`
}

type searchMatch struct {
	RelPath string
	Line    int
	Preview string
}

func (r *Registry) makeSearchCodeTool() Tool {
	return Tool{
		Name:            "search_code",
		Description:     "Search workspace files concurrently by regex/literal pattern, file glob, or structural RepoMap symbol summary.",
		InputSchema:     json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string","description":"Regular expression, literal text, or glob pattern"},"path":{"type":"string","description":"Optional subdirectory or file within workspace"},"include":{"type":"string","description":"Optional file glob filter such as *.go or *.{ts,tsx}"},"literal":{"type":"boolean","description":"Treat pattern as literal substring"},"case_sensitive":{"type":"boolean","description":"Case-sensitive matching (default true)"},"mode":{"type":"string","enum":["grep","glob","repomap"],"description":"Search mode (default grep)"},"limit":{"type":"integer","description":"Max results (default 100)"},"max_tokens":{"type":"integer","description":"Token budget when mode=repomap"}}} `),
		ReadOnly:        true,
		Mutating:        false,
		ConcurrencySafe: true,
		Handler:         r.handleSearchCode,
	}
}

func (r *Registry) handleSearchCode(ctx context.Context, call CallContext, rawInput json.RawMessage) (ToolResult, error) {
	var in searchCodeInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return ToolResult{}, fmt.Errorf("invalid search_code input: %w", err)
	}
	pg, err := r.resolvePathGuard(call)
	if err != nil {
		return ToolResult{}, err
	}
	logsDir := r.resolveLogsDir(call)

	mode := strings.ToLower(strings.TrimSpace(in.Mode))
	if mode == "repomap" {
		budget := in.MaxTokens
		if budget <= 0 {
			budget = defaultRepoMapTokens
		}
		rm, err := RepoMapWithVault(pg.WorkspaceRoot(), pg.VaultRoot(), budget)
		if err != nil {
			return ToolResult{}, err
		}
		return ToolResult{
			Output: wrapUntrustedWithDir("search_code:repomap", rm, logsDir),
		}, nil
	}

	scopeRoot := pg.WorkspaceRoot()
	if strings.TrimSpace(in.Path) != "" {
		canonScope, _, inWS, rErr := pg.ResolveInWorkspace(in.Path)
		if rErr != nil {
			return ToolResult{}, rErr
		}
		if !inWS && !call.AllowOutsideWorkspace {
			return ToolResult{}, fmt.Errorf("%w: %s", ErrPathEscapesWorkspace, in.Path)
		}
		scopeRoot = canonScope
	}

	limit := in.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > 1000 {
		limit = 1000
	}

	files, err := collectSafeWorkspaceFiles(pg.WorkspaceRoot(), pg.VaultRoot(), scopeRoot, in.Include)
	if err != nil {
		return ToolResult{}, err
	}

	if mode == "glob" || (strings.TrimSpace(in.Pattern) == "" && strings.TrimSpace(in.Include) != "") {
		var matchedFiles []string
		pat := strings.TrimSpace(in.Pattern)
		for _, f := range files {
			if pat == "" || matchIncludePatterns(pat, f.relSlash) {
				matchedFiles = append(matchedFiles, f.relSlash)
				if len(matchedFiles) >= limit {
					break
				}
			}
		}
		out := strings.Join(matchedFiles, "\n")
		if out == "" {
			out = "(no matching files)"
		}
		return ToolResult{Output: wrapUntrustedWithDir("search_code:glob", out, logsDir)}, nil
	}

	if strings.TrimSpace(in.Pattern) == "" {
		return ToolResult{}, errors.New("search_code requires a non-empty pattern (or mode=\"repomap\")")
	}

	caseSensitive := true
	if in.CaseSensitive != nil {
		caseSensitive = *in.CaseSensitive
	}

	matcher, err := buildLineMatcher(in.Pattern, in.Literal, caseSensitive)
	if err != nil {
		return ToolResult{}, err
	}

	hits := runConcurrentFileGrep(ctx, files, matcher, limit)
	if len(hits) == 0 {
		return ToolResult{Output: wrapUntrustedWithDir("search_code", "(no matches)", logsDir)}, nil
	}

	var b strings.Builder
	for _, h := range hits {
		fmt.Fprintf(&b, "%s:%d: %s\n", h.RelPath, h.Line, h.Preview)
	}
	return ToolResult{
		Output: wrapUntrustedWithDir("search_code", strings.TrimRight(b.String(), "\n"), logsDir),
		Metadata: map[string]any{
			"matches": len(hits),
		},
	}, nil
}

type wsFileEntry struct {
	absPath  string
	relSlash string
}

func collectSafeWorkspaceFiles(canonWs, canonVault, scopePath, include string) ([]wsFileEntry, error) {
	st, err := os.Stat(scopePath)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		rel, rErr := filepath.Rel(canonWs, scopePath)
		if rErr != nil {
			rel = filepath.Base(scopePath)
		}
		relSlash := filepath.ToSlash(rel)
		if include != "" && !matchIncludePatterns(include, relSlash) {
			return nil, nil
		}
		return []wsFileEntry{{absPath: scopePath, relSlash: relSlash}}, nil
	}

	wsInWorktree := canonVault != "" && config.IsStrictlyWithinRoot(canonWs, filepath.Join(canonVault, "runtime", "worktrees"))

	var out []wsFileEntry
	var walk func(dir string)
	walk = func(dir string) {
		entries, rErr := os.ReadDir(dir)
		if rErr != nil {
			return
		}
		for _, ent := range entries {
			name := ent.Name()
			if shouldSkipSearchDirName(name) {
				continue
			}
			full := filepath.Join(dir, name)
			lst, lErr := os.Lstat(full)
			if lErr != nil {
				continue
			}
			if lst.Mode()&os.ModeSymlink != 0 {
				resolved, eErr := config.CanonicalPath(full)
				if eErr != nil {
					continue
				}
				if !config.IsStrictlyWithinRoot(resolved, canonWs) {
					continue
				}
				if canonVault != "" && config.IsWithinRoot(resolved, canonVault) && !wsInWorktree {
					continue
				}
				rst, sErr := os.Stat(resolved)
				if sErr != nil || rst.IsDir() || !rst.Mode().IsRegular() || rst.Size() > maxSearchableFileBytes {
					continue
				}
				rel, _ := filepath.Rel(canonWs, full)
				relSlash := filepath.ToSlash(rel)
				if include == "" || matchIncludePatterns(include, relSlash) {
					out = append(out, wsFileEntry{absPath: resolved, relSlash: relSlash})
				}
				continue
			}
			if lst.IsDir() {
				walk(full)
				continue
			}
			if !lst.Mode().IsRegular() || lst.Size() > maxSearchableFileBytes {
				continue
			}
			rel, _ := filepath.Rel(canonWs, full)
			relSlash := filepath.ToSlash(rel)
			if include == "" || matchIncludePatterns(include, relSlash) {
				out = append(out, wsFileEntry{absPath: full, relSlash: relSlash})
			}
		}
	}
	walk(scopePath)

	sort.Slice(out, func(i, j int) bool {
		return out[i].relSlash < out[j].relSlash
	})
	return out, nil
}

func shouldSkipSearchDirName(name string) bool {
	return config.IsIgnoredWalkDir(name)
}

func matchIncludePatterns(include, relSlash string) bool {
	for _, expanded := range expandBraces(strings.TrimSpace(include)) {
		if matchGlobPattern(expanded, relSlash) {
			return true
		}
	}
	return false
}

func expandBraces(pat string) []string {
	openIdx := strings.IndexByte(pat, '{')
	closeIdx := strings.IndexByte(pat, '}')
	if openIdx < 0 || closeIdx <= openIdx {
		return []string{pat}
	}
	prefix := pat[:openIdx]
	suffix := pat[closeIdx+1:]
	alts := strings.Split(pat[openIdx+1:closeIdx], ",")
	out := make([]string, 0, len(alts))
	for _, alt := range alts {
		out = append(out, prefix+strings.TrimSpace(alt)+suffix)
	}
	return out
}

func buildLineMatcher(pattern string, literal, caseSensitive bool) (func(string) bool, error) {
	if literal {
		if caseSensitive {
			return func(line string) bool { return strings.Contains(line, pattern) }, nil
		}
		lowerPat := strings.ToLower(pattern)
		return func(line string) bool { return strings.Contains(strings.ToLower(line), lowerPat) }, nil
	}
	expr := pattern
	if !caseSensitive {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid search regex %q: %w", pattern, err)
	}
	return re.MatchString, nil
}

func runConcurrentFileGrep(ctx context.Context, files []wsFileEntry, matchFn func(string) bool, limit int) []searchMatch {
	if len(files) == 0 {
		return nil
	}
	workers := runtime.NumCPU()
	if workers < 2 {
		workers = 2
	}
	if workers > 8 {
		workers = 8
	}
	if workers > len(files) {
		workers = len(files)
	}

	jobs := make(chan wsFileEntry, len(files))
	for _, f := range files {
		jobs <- f
	}
	close(jobs)

	var mu sync.Mutex
	var hits []searchMatch

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for entry := range jobs {
				if ctx != nil && ctx.Err() != nil {
					return
				}
				mu.Lock()
				alreadyFull := len(hits) >= limit*2
				mu.Unlock()
				if alreadyFull {
					return
				}

				fileHits := grepSingleFile(entry, matchFn, limit)
				if len(fileHits) > 0 {
					mu.Lock()
					hits = append(hits, fileHits...)
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].RelPath != hits[j].RelPath {
			return hits[i].RelPath < hits[j].RelPath
		}
		return hits[i].Line < hits[j].Line
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

func grepSingleFile(entry wsFileEntry, matchFn func(string) bool, maxPerFile int) []searchMatch {
	raw, err := os.ReadFile(entry.absPath)
	if err != nil || isLikelyBinary(raw) {
		return nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), maxSearchableFileBytes)
	lineNo := 0
	var out []searchMatch
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if matchFn(line) {
			preview := strings.TrimSpace(line)
			if len(preview) > 240 {
				preview = preview[:240] + "..."
			}
			out = append(out, searchMatch{
				RelPath: entry.relSlash,
				Line:    lineNo,
				Preview: preview,
			})
			if len(out) >= maxPerFile {
				break
			}
		}
	}
	return out
}

// ============================================================================
// RepoMap: Concurrent Go / TS / Python / Rust Top-Level Symbol Extractor
// ============================================================================

type repoSymbol struct {
	Name      string
	Signature string
	Line      int
}

type parsedFileSymbols struct {
	RelPath string
	Symbols []repoSymbol
	Idents  map[string]struct{}
	Score   int
}

var (
	reTSSymbol   = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:abstract\s+)?(?:async\s+)?(function|class|interface|type|enum|const)\s+([A-Za-z_$][A-Za-z0-9_$]*)`)
	rePySymbol   = regexp.MustCompile(`^(?:async\s+)?(def|class)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	reRsSymbol   = regexp.MustCompile(`^(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?(?:unsafe\s+)?(fn|struct|enum|trait|type|mod|impl)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	reGoFallback = regexp.MustCompile(`^(func|type|var|const)\s+(?:\([^)]+\)\s*)?([A-Za-z_][A-Za-z0-9_]*)`)
	reIdentToken = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{2,}`)
)

// RepoMap 并发扫描工作区 Go / TS / Python / Rust 顶层符号并按跨文件引用中心度在 maxTokens 预算内生成高信噪比仓库骨架图。
func RepoMap(wsRoot string, maxTokens int) (string, error) {
	return RepoMapWithVault(wsRoot, "", maxTokens)
}

// RepoMapWithVault 携带真实 vaultRoot 执行安全边界校验并生成仓库符号图。
func RepoMapWithVault(wsRoot, vaultRoot string, maxTokens int) (string, error) {
	if strings.TrimSpace(wsRoot) == "" {
		return "", fmt.Errorf("%w: empty workspace root", ErrInvalidTargetPath)
	}
	canonWs, err := config.CanonicalPath(wsRoot)
	if err != nil {
		return "", fmt.Errorf("canonicalize workspace %s: %w", wsRoot, wrapOnlyErr(err))
	}
	var canonVault string
	if strings.TrimSpace(vaultRoot) != "" {
		if cv, vErr := config.CanonicalPath(vaultRoot); vErr == nil {
			canonVault = cv
		} else {
			canonVault = filepath.Clean(vaultRoot)
		}
	}
	if maxTokens <= 0 {
		maxTokens = defaultRepoMapTokens
	}
	maxChars := maxTokens * 4

	files, err := collectSafeWorkspaceFiles(canonWs, canonVault, canonWs, "")
	if err != nil {
		return "", err
	}

	var codeFiles []wsFileEntry
	for _, f := range files {
		if isSupportedRepoMapExt(filepath.Ext(f.relSlash)) {
			codeFiles = append(codeFiles, f)
		}
	}
	if len(codeFiles) == 0 {
		return "", nil
	}

	workers := runtime.NumCPU()
	if workers < 2 {
		workers = 2
	}
	if workers > 8 {
		workers = 8
	}
	if workers > len(codeFiles) {
		workers = len(codeFiles)
	}

	jobs := make(chan wsFileEntry, len(codeFiles))
	for _, cf := range codeFiles {
		jobs <- cf
	}
	close(jobs)

	var mu sync.Mutex
	parsed := make([]parsedFileSymbols, 0, len(codeFiles))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for entry := range jobs {
				pf, ok := extractFileSymbols(entry)
				if ok && len(pf.Symbols) > 0 {
					mu.Lock()
					parsed = append(parsed, pf)
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()

	if len(parsed) == 0 {
		return "", nil
	}

	// 计算跨文件引用频次（轻量级引用中心度排序，高入度核心模块优先展示）。
	identFileFreq := make(map[string]int)
	for _, pf := range parsed {
		for id := range pf.Idents {
			identFileFreq[id]++
		}
	}
	for i := range parsed {
		score := len(parsed[i].Symbols)
		for _, sym := range parsed[i].Symbols {
			refs := identFileFreq[sym.Name] - 1
			if refs > 0 {
				score += refs * 4
			}
		}
		parsed[i].Score = score
	}

	sort.Slice(parsed, func(i, j int) bool {
		if parsed[i].Score != parsed[j].Score {
			return parsed[i].Score > parsed[j].Score
		}
		return parsed[i].RelPath < parsed[j].RelPath
	})

	var b strings.Builder
	for _, pf := range parsed {
		var section strings.Builder
		fmt.Fprintf(&section, "%s:\n", pf.RelPath)
		for _, sym := range pf.Symbols {
			fmt.Fprintf(&section, "  %s (L%d)\n", sym.Signature, sym.Line)
		}
		chunk := section.String()
		if b.Len()+len(chunk) > maxChars {
			// 若整段超出预算，尝试逐符号装填剩余空间。
			header := pf.RelPath + ":\n"
			if b.Len()+len(header) >= maxChars {
				break
			}
			addedAny := false
			for _, sym := range pf.Symbols {
				lineStr := fmt.Sprintf("  %s (L%d)\n", sym.Signature, sym.Line)
				needed := len(lineStr)
				if !addedAny {
					needed += len(header)
				}
				if b.Len()+needed > maxChars {
					break
				}
				if !addedAny {
					b.WriteString(header)
					addedAny = true
				}
				b.WriteString(lineStr)
			}
			break
		}
		b.WriteString(chunk)
	}

	return strings.TrimRight(b.String(), "\n"), nil
}

func isSupportedRepoMapExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".py", ".rs":
		return true
	default:
		return false
	}
}

func extractFileSymbols(entry wsFileEntry) (parsedFileSymbols, bool) {
	raw, err := os.ReadFile(entry.absPath)
	if err != nil || isLikelyBinary(raw) {
		return parsedFileSymbols{}, false
	}

	ext := strings.ToLower(filepath.Ext(entry.relSlash))
	var syms []repoSymbol
	switch ext {
	case ".go":
		syms = extractGoSymbols(entry.absPath, raw)
	case ".ts", ".tsx", ".js", ".jsx", ".mjs":
		syms = extractRegexTopSymbols(raw, reTSSymbol)
	case ".py":
		syms = extractRegexTopSymbols(raw, rePySymbol)
	case ".rs":
		syms = extractRegexTopSymbols(raw, reRsSymbol)
	}

	idents := make(map[string]struct{})
	for _, m := range reIdentToken.FindAll(raw, 4096) {
		idents[string(m)] = struct{}{}
	}

	return parsedFileSymbols{
		RelPath: entry.relSlash,
		Symbols: syms,
		Idents:  idents,
	}, true
}

func extractGoSymbols(filename string, raw []byte) []repoSymbol {
	fset := token.NewFileSet()
	fileAST, err := parser.ParseFile(fset, filename, raw, 0)
	if err != nil || fileAST == nil {
		return extractRegexTopSymbols(raw, reGoFallback)
	}

	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	lineAt := func(pos token.Pos) (int, string) {
		ln := fset.Position(pos).Line
		if ln >= 1 && ln <= len(lines) {
			sig := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(lines[ln-1]), "{"))
			sig = strings.TrimSpace(sig)
			return ln, sig
		}
		return ln, ""
	}

	var syms []repoSymbol
	for _, decl := range fileAST.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name == nil {
				continue
			}
			ln, sig := lineAt(d.Pos())
			if sig == "" {
				sig = "func " + d.Name.Name
			}
			syms = append(syms, repoSymbol{
				Name:      d.Name.Name,
				Signature: sig,
				Line:      ln,
			})
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch sp := spec.(type) {
				case *ast.TypeSpec:
					ln, sig := lineAt(sp.Pos())
					if sig == "" || !strings.HasPrefix(sig, "type ") {
						sig = "type " + sp.Name.Name
					}
					syms = append(syms, repoSymbol{
						Name:      sp.Name.Name,
						Signature: sig,
						Line:      ln,
					})
				case *ast.ValueSpec:
					if d.Tok == token.CONST || d.Tok == token.VAR {
						for _, ident := range sp.Names {
							if ident.IsExported() {
								ln, _ := lineAt(ident.Pos())
								syms = append(syms, repoSymbol{
									Name:      ident.Name,
									Signature: strings.ToLower(d.Tok.String()) + " " + ident.Name,
									Line:      ln,
								})
							}
						}
					}
				}
			}
		}
	}
	return syms
}

func extractRegexTopSymbols(raw []byte, re *regexp.Regexp) []repoSymbol {
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	var syms []repoSymbol
	for idx, rawLine := range lines {
		// 仅提取顶层声明（无前导缩进）。
		if len(rawLine) == 0 || rawLine[0] == ' ' || rawLine[0] == '\t' || rawLine[0] == '#' || strings.HasPrefix(rawLine, "//") {
			continue
		}
		trimmed := strings.TrimSpace(rawLine)
		m := re.FindStringSubmatch(trimmed)
		if len(m) < 3 {
			continue
		}
		sig := trimmed
		if braceIdx := strings.IndexByte(sig, '{'); braceIdx > 0 {
			sig = strings.TrimSpace(sig[:braceIdx])
		}
		sig = strings.TrimSuffix(sig, ":")
		if len(sig) > 120 {
			sig = sig[:120] + "..."
		}
		syms = append(syms, repoSymbol{
			Name:      m[2],
			Signature: sig,
			Line:      idx + 1,
		})
	}
	return syms
}
