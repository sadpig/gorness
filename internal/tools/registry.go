package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/llm"
	"gorness.dev/gorness/internal/store"
)

const (
	BM25DeferThreshold       = 12
	MaxInlineToolOutputBytes = 32 * 1024
	defaultCommandTimeoutSec = 120
)

var (
	ErrToolNotFound = errors.New("tool not found")

	reClosingUntrustedTag = regexp.MustCompile(`(?i)</\s*untrusted_content\b[^>]*>`)
	globalOverflowLogsDir atomic.Pointer[string]
)

// SetOverflowLogsDir 配置 WrapUntrustedContent 超长截断溢出的默认落盘目录（例如 ~/.gorness/runtime/logs）。
func SetOverflowLogsDir(dir string) {
	trimmed := strings.TrimSpace(dir)
	globalOverflowLogsDir.Store(&trimmed)
}

// WrapUntrustedContent 将外部文件、命令输出与 MCP 返回包裹在不可信安全信封内：
// 1. 净化 Bidi 双向控制符；
// 2. 拆解任何内嵌闭合标签 </untrusted_content> 阻断提示词注入逃逸；
// 3. 超长输出按 UTF-8 字符边界截断并将完整原文保存至 runtime/logs/ 可寻址溢出文件。
func WrapUntrustedContent(source, body string) string {
	var dir string
	if p := globalOverflowLogsDir.Load(); p != nil {
		dir = *p
	}
	return wrapUntrustedWithDir(source, body, dir)
}

func wrapUntrustedWithDir(source, body, logsDir string) string {
	safeSrc := escapeXMLText(config.EscapeBidi(strings.TrimSpace(source)))
	if safeSrc == "" {
		safeSrc = "tool"
	}

	defanged := config.EscapeBidi(body)
	defanged = reClosingUntrustedTag.ReplaceAllString(defanged, "&lt;/untrusted_content&gt;")

	payload := defanged
	if len(defanged) > MaxInlineToolOutputBytes {
		targetDir := strings.TrimSpace(logsDir)
		if targetDir == "" {
			if p := globalOverflowLogsDir.Load(); p != nil && *p != "" {
				targetDir = *p
			} else {
				targetDir = filepath.Join(os.TempDir(), "gorness-runtime-logs")
			}
		}
		digest := sha256HexBytes([]byte(defanged))[:12]
		overflowPath := filepath.Join(targetDir, fmt.Sprintf("overflow-%s.log", digest))
		savedNotice := ""
		if err := store.AtomicWriteFile(overflowPath, []byte(defanged), 0o600); err == nil {
			savedNotice = fmt.Sprintf("full output saved to %s", overflowPath)
		} else {
			savedNotice = "overflow write skipped"
		}

		headBudget := 20 * 1024
		tailBudget := 8 * 1024
		head := truncateUTF8Prefix(defanged, headBudget)
		tail := truncateUTF8Suffix(defanged, tailBudget)
		omitted := len(defanged) - len(head) - len(tail)
		if omitted < 0 {
			omitted = 0
		}
		payload = fmt.Sprintf("%s\n\n... [truncated %d bytes; %s] ...\n\n%s", head, omitted, savedNotice, tail)
	}

	return fmt.Sprintf("<untrusted_content source=\"%s\">\n%s\n</untrusted_content>", safeSrc, payload)
}

func truncateUTF8Prefix(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

func truncateUTF8Suffix(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	start := len(s) - maxBytes
	for start < len(s) && !utf8.RuneStart(s[start]) {
		start++
	}
	return s[start:]
}

func logsDirFromVault(vaultRoot string) string {
	if strings.TrimSpace(vaultRoot) == "" {
		return ""
	}
	return config.VaultPaths{Root: vaultRoot}.LogsDir()
}

type CallContext struct {
	WorkspaceRoot         string
	VaultRoot             string
	SessionID             string
	Seq                   uint64
	SandboxMode           SandboxMode
	AllowedDomains        []string
	ExtraWriteDirs        []string
	AllowOutsideWorkspace bool
	SelectedHunkIDs       []string
	ActivatedTools        map[string]bool
	ActivatedSkills       map[string]bool
	Snapshots             *store.SnapshotStore
	PathGuard             *PathGuard
	Secrets               []string
}

type ToolResult struct {
	Output      string         `json:"output"`
	IsError     bool           `json:"is_error,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Staged      *StagedEdit    `json:"-"`
	Activated   []string       `json:"activated,omitempty"`
	TouchedPath string         `json:"touched_path,omitempty"`
}

type ToolHandler func(ctx context.Context, call CallContext, rawInput json.RawMessage) (ToolResult, error)

type Tool struct {
	Name            string
	Description     string
	InputSchema     json.RawMessage
	ReadOnly        bool
	Mutating        bool
	ConcurrencySafe bool
	IsMCP           bool
	MCPServerID     string
	MCPSourceFile   string
	MCPReadOnlyHint bool
	DeferLoad       bool
	Handler         ToolHandler
}

type RegistryOptions struct {
	WorkspaceRoot string
	VaultPaths    config.VaultPaths
	Snapshots     *store.SnapshotStore
	PathGuard     *PathGuard
	Seatbelt      *SeatbeltRunner
	Skills        *SkillCatalog
}

// Registry 聚合 5 大核心内置工具、动态 activate_skill、MCP 外部工具与超过 12 个工具时的自动 BM25 search_tools 延迟加载。
type Registry struct {
	wsRoot     string
	vaultPaths config.VaultPaths
	snapshots  *store.SnapshotStore
	pathGuard  *PathGuard
	seatbelt   *SeatbeltRunner

	mu         sync.RWMutex
	tools      map[string]Tool
	skills     *SkillCatalog
	readHashes map[string]string // key: sessionID + "\x00" + canonAbsPath
}

func NewRegistry(opts RegistryOptions) (*Registry, error) {
	pg := opts.PathGuard
	if pg == nil && strings.TrimSpace(opts.WorkspaceRoot) != "" {
		var err error
		pg, err = NewPathGuard(opts.WorkspaceRoot, opts.VaultPaths.Root)
		if err != nil {
			return nil, err
		}
	}
	sb := opts.Seatbelt
	if sb == nil {
		sb = NewSeatbeltRunner(WithRunnerVaultRoot(opts.VaultPaths.Root))
	}
	if strings.TrimSpace(opts.VaultPaths.Root) != "" {
		SetOverflowLogsDir(opts.VaultPaths.LogsDir())
	}

	r := &Registry{
		wsRoot:     opts.WorkspaceRoot,
		vaultPaths: opts.VaultPaths,
		snapshots:  opts.Snapshots,
		pathGuard:  pg,
		seatbelt:   sb,
		tools:      make(map[string]Tool),
		skills:     opts.Skills,
		readHashes: make(map[string]string),
	}

	for _, coreTool := range []Tool{
		r.makeReadFileTool(),
		r.makeEditFileTool(),
		r.makeWriteFileTool(),
		r.makeRunCommandTool(),
		r.makeSearchCodeTool(),
	} {
		r.tools[coreTool.Name] = coreTool
	}
	return r, nil
}

func (r *Registry) Register(t Tool) {
	if r == nil || strings.TrimSpace(t.Name) == "" {
		return
	}
	if len(bytes.TrimSpace(t.InputSchema)) == 0 {
		t.InputSchema = json.RawMessage(`{"type":"object","properties":{}}`)
	}
	r.mu.Lock()
	r.tools[t.Name] = t
	r.mu.Unlock()
}

func (r *Registry) Unregister(name string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	delete(r.tools, strings.TrimSpace(name))
	r.mu.Unlock()
}

func (r *Registry) SetSkillCatalog(cat *SkillCatalog) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.skills = cat
	r.mu.Unlock()
}

func (r *Registry) SkillCatalog() *SkillCatalog {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.skills
}

func (r *Registry) Get(name string) (Tool, bool) {
	if r == nil {
		return Tool{}, false
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "search_tools" {
		return r.makeSearchToolsTool(), true
	}
	if trimmed == "activate_skill" {
		r.mu.RLock()
		hasSkills := r.skills != nil && len(r.skills.All()) > 0
		if custom, ok := r.tools["activate_skill"]; ok {
			r.mu.RUnlock()
			return custom, true
		}
		r.mu.RUnlock()
		if hasSkills {
			return r.makeActivateSkillTool(), true
		}
		return Tool{}, false
	}

	r.mu.RLock()
	t, ok := r.tools[trimmed]
	r.mu.RUnlock()
	return t, ok
}

func (r *Registry) All() []Tool {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	list := make([]Tool, 0, len(r.tools)+1)
	hasCustomSkillTool := false
	for _, t := range r.tools {
		if t.Name == "activate_skill" {
			hasCustomSkillTool = true
		}
		list = append(list, t)
	}
	skills := r.skills
	r.mu.RUnlock()

	if !hasCustomSkillTool && skills != nil && len(skills.All()) > 0 {
		list = append(list, r.makeActivateSkillTool())
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})
	return list
}

// VisibleSchemas 计算当前轮次向模型暴露的缓存稳定工具 Schema 列表：
// 当总工具数超过 12 个时自动启用 BM25 延迟折叠，仅暴露核心常驻工具 + search_tools + 已激活工具。
func (r *Registry) VisibleSchemas(activated map[string]bool) []llm.ToolSchema {
	all := r.All()
	fold := len(all) > BM25DeferThreshold

	var out []llm.ToolSchema
	hasFolded := false
	for _, t := range all {
		if t.Name == "search_tools" {
			continue
		}
		isAlwaysVisible := isCoreBuiltinTool(t.Name) && !t.IsMCP && !t.DeferLoad
		if !fold {
			// 总数 <= 12 时直接暴露全部已注册工具，无需额外 search_tools 往返。
			out = append(out, toLLMToolSchema(t, false))
			continue
		}
		if isAlwaysVisible || (activated != nil && activated[t.Name]) {
			out = append(out, toLLMToolSchema(t, t.DeferLoad || t.IsMCP))
		} else {
			hasFolded = true
		}
	}

	if fold && hasFolded {
		st := r.makeSearchToolsTool()
		out = append(out, toLLMToolSchema(st, false))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func (r *Registry) StageEdit(call CallContext, toolName string, rawInput json.RawMessage) (*StagedEdit, error) {
	return r.stageEditFromInput(call, toolName, rawInput)
}

func (r *Registry) Execute(ctx context.Context, name string, call CallContext, rawInput json.RawMessage) (ToolResult, error) {
	t, ok := r.Get(name)
	if !ok || t.Handler == nil {
		return ToolResult{}, fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}
	if len(bytes.TrimSpace(rawInput)) == 0 {
		rawInput = json.RawMessage(`{}`)
	}
	return t.Handler(ctx, call, rawInput)
}

func (r *Registry) RecordReadHash(sessionID, absPath, sha256Hex string) {
	if r == nil || strings.TrimSpace(absPath) == "" || strings.TrimSpace(sha256Hex) == "" {
		return
	}
	canon := canonicalizeExistingOrClean(absPath)
	key := strings.TrimSpace(sessionID) + "\x00" + canon
	r.mu.Lock()
	r.readHashes[key] = strings.ToLower(strings.TrimSpace(sha256Hex))
	r.mu.Unlock()
}

func (r *Registry) LastReadHash(sessionID, absPath string) string {
	if r == nil || strings.TrimSpace(absPath) == "" {
		return ""
	}
	canon := canonicalizeExistingOrClean(absPath)
	key := strings.TrimSpace(sessionID) + "\x00" + canon
	r.mu.RLock()
	h := r.readHashes[key]
	if h == "" && sessionID != "" {
		h = r.readHashes["\x00"+canon]
	}
	r.mu.RUnlock()
	return h
}

func (r *Registry) resolvePathGuard(call CallContext) (*PathGuard, error) {
	if call.PathGuard != nil {
		return call.PathGuard, nil
	}
	if r.pathGuard != nil && (strings.TrimSpace(call.WorkspaceRoot) == "" || canonicalizeExistingOrClean(call.WorkspaceRoot) == r.pathGuard.WorkspaceRoot()) {
		return r.pathGuard, nil
	}
	ws := call.WorkspaceRoot
	if strings.TrimSpace(ws) == "" {
		ws = r.wsRoot
	}
	vault := call.VaultRoot
	if strings.TrimSpace(vault) == "" {
		vault = r.vaultPaths.Root
	}
	return NewPathGuard(ws, vault)
}

func (r *Registry) resolveLogsDir(call CallContext) string {
	if strings.TrimSpace(call.VaultRoot) != "" {
		return logsDirFromVault(call.VaultRoot)
	}
	if strings.TrimSpace(r.vaultPaths.Root) != "" {
		return r.vaultPaths.LogsDir()
	}
	return ""
}

// ============================================================================
// Core Tool #4: run_command (macOS Seatbelt + Process Group + Git Status Hint)
// ============================================================================

type runCommandInput struct {
	Command    string `json:"command"`
	WorkDir    string `json:"workdir,omitempty"`
	TimeoutSec int    `json:"timeout_sec,omitempty"`
}

func (r *Registry) makeRunCommandTool() Tool {
	return Tool{
		Name:            "run_command",
		Description:     "Execute a zsh command inside the macOS Seatbelt sandbox with process-group management and optional domain allowlist proxy.",
		InputSchema:     json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"Shell command to execute"},"workdir":{"type":"string","description":"Optional working directory inside workspace"},"timeout_sec":{"type":"integer","description":"Command timeout in seconds (default 120)"}},"required":["command"]}`),
		ReadOnly:        false,
		Mutating:        true,
		ConcurrencySafe: false,
		Handler:         r.handleRunCommand,
	}
}

func (r *Registry) handleRunCommand(ctx context.Context, call CallContext, rawInput json.RawMessage) (ToolResult, error) {
	var in runCommandInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return ToolResult{}, fmt.Errorf("invalid run_command input: %w", err)
	}
	if strings.TrimSpace(in.Command) == "" {
		return ToolResult{}, errors.New("run_command requires a non-empty command")
	}

	pg, err := r.resolvePathGuard(call)
	if err != nil {
		return ToolResult{}, err
	}
	wsRoot := pg.WorkspaceRoot()
	workDir := wsRoot
	inWS := true
	if strings.TrimSpace(in.WorkDir) != "" {
		canonWD, _, resolvedInWS, rErr := pg.ResolveInWorkspace(in.WorkDir)
		if rErr != nil {
			return ToolResult{}, rErr
		}
		if !resolvedInWS && !call.AllowOutsideWorkspace {
			return ToolResult{}, fmt.Errorf("%w: %s", ErrPathEscapesWorkspace, in.WorkDir)
		}
		st, stErr := os.Stat(canonWD)
		if stErr != nil {
			return ToolResult{}, stErr
		}
		if !st.IsDir() {
			return ToolResult{}, fmt.Errorf("%w: workdir %s is not a directory", ErrInvalidTargetPath, in.WorkDir)
		}
		workDir = canonWD
		inWS = resolvedInWS
	}

	timeout := time.Duration(defaultCommandTimeoutSec) * time.Second
	if in.TimeoutSec > 0 {
		timeout = time.Duration(in.TimeoutSec) * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	mode := call.SandboxMode
	if mode == "" {
		mode = SandboxStrict
	}

	sandboxRoot := workDir
	gitStatusDir := workDir
	if inWS && wsRoot != "" {
		sandboxRoot = wsRoot
		gitStatusDir = wsRoot
	}

	gitBefore := quickGitStatus(gitStatusDir)
	res, err := r.seatbelt.runWithParams(runCtx, seatbeltRunParams{
		CmdStr:         in.Command,
		SandboxRoot:    sandboxRoot,
		WorkDir:        workDir,
		ExtraWriteDirs: call.ExtraWriteDirs,
		Mode:           mode,
		AllowedDomains: call.AllowedDomains,
	})
	gitAfter := quickGitStatus(gitStatusDir)
	if gitBefore != gitAfter && gitAfter != "" {
		res.GitChanges = gitAfter
	}

	if err != nil && !res.TimedOut {
		return ToolResult{}, err
	}

	var out strings.Builder
	fmt.Fprintf(&out, "[exit_code: %d | duration_ms: %d", res.ExitCode, res.Duration.Milliseconds())
	if res.TimedOut {
		out.WriteString(" | timed_out: true")
	}
	out.WriteString("]\n")
	if res.Combined != "" {
		out.WriteString(config.Redact(res.Combined, call.Secrets))
	}
	if res.GitChanges != "" {
		if !strings.HasSuffix(out.String(), "\n") {
			out.WriteByte('\n')
		}
		out.WriteString("[git_status]\n")
		out.WriteString(res.GitChanges)
	}

	return ToolResult{
		Output:  wrapUntrustedWithDir("run_command", strings.TrimRight(out.String(), "\n"), r.resolveLogsDir(call)),
		IsError: res.ExitCode != 0 || res.TimedOut,
		Metadata: map[string]any{
			"exit_code":   res.ExitCode,
			"duration_ms": res.Duration.Milliseconds(),
			"timed_out":   res.TimedOut,
		},
	}, nil
}

func quickGitStatus(dir string) string {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain", "-u")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ============================================================================
// Dynamic Built-in: activate_skill
// ============================================================================

type activateSkillInput struct {
	Name string `json:"name"`
}

func (r *Registry) makeActivateSkillTool() Tool {
	return Tool{
		Name:            "activate_skill",
		Description:     "Load the full instructions of a discovered SKILL.md skill by name.",
		InputSchema:     json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"Skill name from <available_skills>"}},"required":["name"]}`),
		ReadOnly:        true,
		Mutating:        false,
		ConcurrencySafe: true,
		Handler: func(_ context.Context, call CallContext, rawInput json.RawMessage) (ToolResult, error) {
			var in activateSkillInput
			if err := json.Unmarshal(rawInput, &in); err != nil {
				return ToolResult{}, fmt.Errorf("invalid activate_skill input: %w", err)
			}
			cat := r.SkillCatalog()
			if cat == nil {
				return ToolResult{}, errors.New("no skill catalog configured")
			}
			sk, ok := cat.Get(in.Name)
			if !ok {
				return ToolResult{
					Output:  fmt.Sprintf("skill %q not found", in.Name),
					IsError: true,
				}, nil
			}
			if call.ActivatedSkills != nil {
				call.ActivatedSkills[sk.Name] = true
			}
			formatted := fmt.Sprintf("<skill_content name=%q scope=%q>\n%s\n</skill_content>",
				config.EscapeBidi(sk.Name), sk.Scope, config.EscapeBidi(sk.Body))
			return ToolResult{
				Output:    formatted,
				Activated: []string{sk.Name},
			}, nil
		},
	}
}

// ============================================================================
// BM25 Deferred Tool Search: search_tools
// ============================================================================

type searchToolsInput struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

func (r *Registry) makeSearchToolsTool() Tool {
	return Tool{
		Name:            "search_tools",
		Description:     "Search deferred MCP and extension tools by BM25 keyword relevance and activate matching tool schemas for subsequent turns.",
		InputSchema:     json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Keywords describing the external tool or capability needed"},"limit":{"type":"integer","description":"Maximum matching tools to activate (default 5)"}},"required":["query"]}`),
		ReadOnly:        true,
		Mutating:        false,
		ConcurrencySafe: true,
		Handler:         r.handleSearchTools,
	}
}

func (r *Registry) handleSearchTools(_ context.Context, call CallContext, rawInput json.RawMessage) (ToolResult, error) {
	var in searchToolsInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return ToolResult{}, fmt.Errorf("invalid search_tools input: %w", err)
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 5
	}
	if limit > 10 {
		limit = 10
	}

	matches := r.SearchBM25(in.Query, limit)
	if len(matches) == 0 {
		return ToolResult{Output: "No matching deferred tools found."}, nil
	}

	activatedNames := make([]string, 0, len(matches))
	var b strings.Builder
	fmt.Fprintf(&b, "Activated %d matching tool(s):\n", len(matches))
	for _, m := range matches {
		activatedNames = append(activatedNames, m.Name)
		if call.ActivatedTools != nil {
			call.ActivatedTools[m.Name] = true
		}
		fmt.Fprintf(&b, "- %s: %s\n", m.Name, m.Description)
	}

	return ToolResult{
		Output:    strings.TrimRight(b.String(), "\n"),
		Activated: activatedNames,
	}, nil
}

// SearchBM25 使用 Okapi BM25 (k1=1.2, b=0.75) 对所有非核心/延迟工具进行检索排序。
func (r *Registry) SearchBM25(query string, limit int) []llm.ToolSchema {
	qTokens := tokenizeBM25(query)
	if len(qTokens) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = 5
	}

	all := r.All()
	type docEntry struct {
		tool   Tool
		tf     map[string]int
		length int
	}

	var docs []docEntry
	df := make(map[string]int)
	totalLen := 0

	for _, t := range all {
		if t.Name == "search_tools" || (isCoreBuiltinTool(t.Name) && !t.IsMCP && !t.DeferLoad) {
			continue
		}
		nameTokens := tokenizeBM25(t.Name + " " + t.MCPServerID)
		bodyTokens := tokenizeBM25(t.Description + " " + string(t.InputSchema))

		tf := make(map[string]int)
		for _, tok := range nameTokens {
			tf[tok] += 2 // 工具名与服务器名赋予 2x 词频权重
		}
		for _, tok := range bodyTokens {
			tf[tok]++
		}
		docLen := len(nameTokens)*2 + len(bodyTokens)
		if docLen == 0 {
			docLen = 1
		}
		totalLen += docLen
		for term := range tf {
			df[term]++
		}
		docs = append(docs, docEntry{tool: t, tf: tf, length: docLen})
	}

	if len(docs) == 0 {
		return nil
	}

	const (
		k1 = 1.2
		b  = 0.75
	)
	avgDL := float64(totalLen) / float64(len(docs))
	nDocs := float64(len(docs))
	lowerQ := strings.ToLower(strings.TrimSpace(query))

	type scoredDoc struct {
		tool  Tool
		score float64
	}
	var scored []scoredDoc

	for _, d := range docs {
		var s float64
		for _, qt := range qTokens {
			termFreq := float64(d.tf[qt])
			if termFreq == 0 {
				continue
			}
			docFreq := float64(df[qt])
			idf := math.Log(1.0 + (nDocs-docFreq+0.5)/(docFreq+0.5))
			denom := termFreq + k1*(1.0-b+b*(float64(d.length)/avgDL))
			s += idf * ((termFreq * (k1 + 1.0)) / denom)
		}
		if lowerQ != "" && strings.Contains(strings.ToLower(d.tool.Name), lowerQ) {
			s += 2.5
		}
		if s > 0 {
			scored = append(scored, scoredDoc{tool: d.tool, score: s})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].tool.Name < scored[j].tool.Name
	})

	if len(scored) > limit {
		scored = scored[:limit]
	}

	out := make([]llm.ToolSchema, 0, len(scored))
	for _, sc := range scored {
		out = append(out, toLLMToolSchema(sc.tool, true))
	}
	return out
}

func tokenizeBM25(text string) []string {
	runes := []rune(text)
	var tokens []string
	var ident []rune

	flush := func() {
		if len(ident) == 0 {
			return
		}
		w := strings.ToLower(string(ident))
		ident = ident[:0]
		if !isBM25Stopword(w) {
			tokens = append(tokens, w)
		}
	}

	for i, r := range runes {
		switch {
		case unicode.Is(unicode.Han, r):
			flush()
			tokens = append(tokens, string(r))
			if i+1 < len(runes) && unicode.Is(unicode.Han, runes[i+1]) {
				tokens = append(tokens, string(runes[i:i+2]))
			}
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if len(ident) > 0 && unicode.IsUpper(r) && i > 0 && unicode.IsLower(runes[i-1]) {
				flush()
			}
			ident = append(ident, r)
		default:
			flush()
		}
	}
	flush()

	// 同义词微扩展：帮助自然语言查询如 "kubernetes" <-> "k8s"、"database"/"sql" <-> "postgres" 精准互认。
	expanded := make([]string, 0, len(tokens)+2)
	seen := make(map[string]struct{}, len(tokens)+2)
	add := func(tok string) {
		if tok == "" {
			return
		}
		if _, ok := seen[tok]; !ok {
			seen[tok] = struct{}{}
			expanded = append(expanded, tok)
		}
	}
	for _, tok := range tokens {
		add(tok)
		switch tok {
		case "kubernetes":
			add("k8s")
		case "k8s":
			add("kubernetes")
		case "postgresql":
			add("postgres")
		}
	}
	return expanded
}

func isBM25Stopword(w string) bool {
	switch w {
	case "a", "an", "the", "and", "or", "to", "of", "in", "on", "for", "with", "by", "object", "string", "type", "properties":
		return true
	default:
		return false
	}
}

func isCoreBuiltinTool(name string) bool {
	switch name {
	case "read_file", "edit_file", "write_file", "run_command", "search_code", "activate_skill":
		return true
	default:
		return false
	}
}

func toLLMToolSchema(t Tool, deferLoad bool) llm.ToolSchema {
	schema := t.InputSchema
	if len(bytes.TrimSpace(schema)) == 0 {
		schema = json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return llm.ToolSchema{
		Name:        t.Name,
		Description: t.Description,
		InputSchema: append(json.RawMessage(nil), schema...),
		DeferLoad:   deferLoad,
	}
}
