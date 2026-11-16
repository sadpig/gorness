package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"
	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/store"
	"gorness.dev/gorness/internal/tools"
)

type GateAction string

const (
	GateAllow GateAction = "allow"
	GateAsk   GateAction = "ask"
	GateDefer GateAction = "defer"
	GateDeny  GateAction = "deny"
)

const (
	PermModePlan        = "plan"
	PermModeAsk         = "ask"
	PermModeAcceptEdits = "accept_edits"
	PermModeAuto        = "auto"
	PermModeYolo        = "yolo"
)

type GateResult struct {
	Action      GateAction `json:"action"`
	ReasonCode  string     `json:"reason_code,omitempty"`
	Reason      string     `json:"reason,omitempty"`
	Fingerprint string     `json:"fingerprint,omitempty"`
}

func (r GateResult) RequiresAction() bool {
	return r.Action == GateAsk || r.Action == GateDefer || r.Action == GateDeny
}

type ToolInvocation struct {
	ToolCallID string            `json:"tool_call_id,omitempty"`
	ToolName   string            `json:"tool_name"`
	Args       json.RawMessage   `json:"args,omitempty"`
	Signature  string            `json:"signature,omitempty"`
	Tool       *tools.Tool       `json:"-"`
	AlwaysAsk  bool              `json:"always_ask,omitempty"`
	Staged     *tools.StagedEdit `json:"-"`
}

type EvalContext struct {
	WorkspaceID       string
	WorkspaceRoot     string
	VaultRoot         string
	UserHomeDir       string
	EffectiveMode     string
	Unattended        bool
	ConfigTampered    bool
	TrustedHashes     map[string]string
	UntrustedWarnings []string
	HookDecision      GateResult
	AgentProfile      *config.AgentProfile
	AllowedTools      []string // 非 nil 时对工具施加会话/子代理级白名单交集约束（空切片表示零工具可用）
	Rules             config.RuleSet
	PathGuard         *tools.PathGuard
}

type GrantRecord struct {
	Fingerprint string    `toml:"fingerprint" json:"fingerprint"`
	ToolName    string    `toml:"tool_name" json:"tool_name"`
	Summary     string    `toml:"summary,omitempty" json:"summary,omitempty"`
	CreatedAt   time.Time `toml:"created_at" json:"created_at"`
}

type grantsDoc struct {
	Grants []GrantRecord `toml:"grants"`
}

// GrantStore 将“批准并记住”指纹授权按项目隔离持久化至 ~/.gorness/workspaces/<wsID>/grants.toml（零数据库）。
type GrantStore struct {
	paths config.VaultPaths
	mu    sync.RWMutex
	cache map[string]map[string]GrantRecord // wsID -> fingerprint -> GrantRecord
}

func NewGrantStore(paths config.VaultPaths) *GrantStore {
	return &GrantStore{
		paths: paths,
		cache: make(map[string]map[string]GrantRecord),
	}
}

func (s *GrantStore) grantsFilePath(wsID string) string {
	return filepath.Join(s.paths.WorkspacesDir(), wsID, "grants.toml")
}

func (s *GrantStore) grantsLockPath(wsID string) string {
	return filepath.Join(s.paths.RuntimeDir(), "locks", "grants-"+wsID+".lock")
}

func (s *GrantStore) SaveGrant(wsID, fingerprint, toolName, summary string) error {
	if s == nil {
		return nil
	}
	cleanWS := strings.TrimSpace(wsID)
	cleanFP := strings.TrimSpace(fingerprint)
	if cleanWS == "" || cleanFP == "" || strings.TrimSpace(s.paths.Root) == "" {
		return nil
	}

	rec := GrantRecord{
		Fingerprint: cleanFP,
		ToolName:    strings.TrimSpace(toolName),
		Summary:     strings.TrimSpace(summary),
		CreatedAt:   time.Now().UTC(),
	}

	err := store.WithFlock(s.grantsLockPath(cleanWS), true, func() error {
		existing, rErr := s.readGrantsFile(cleanWS)
		if rErr != nil {
			return rErr
		}
		updated := false
		for i := range existing {
			if existing[i].Fingerprint == cleanFP {
				existing[i] = rec
				updated = true
				break
			}
		}
		if !updated {
			existing = append(existing, rec)
		}
		encoded, mErr := toml.Marshal(grantsDoc{Grants: existing})
		if mErr != nil {
			return fmt.Errorf("marshal grants.toml: %w", mErr)
		}
		return store.AtomicWriteFile(s.grantsFilePath(cleanWS), encoded, 0o600)
	})
	if err != nil {
		return err
	}

	s.mu.Lock()
	wsMap, ok := s.cache[cleanWS]
	if !ok {
		wsMap = make(map[string]GrantRecord)
		s.cache[cleanWS] = wsMap
	}
	wsMap[cleanFP] = rec
	s.mu.Unlock()
	return nil
}

func (s *GrantStore) HasGrant(wsID, fingerprint string) bool {
	if s == nil {
		return false
	}
	cleanWS := strings.TrimSpace(wsID)
	cleanFP := strings.TrimSpace(fingerprint)
	if cleanWS == "" || cleanFP == "" {
		return false
	}

	s.mu.RLock()
	wsMap, loaded := s.cache[cleanWS]
	if loaded {
		_, ok := wsMap[cleanFP]
		s.mu.RUnlock()
		return ok
	}
	s.mu.RUnlock()

	if strings.TrimSpace(s.paths.Root) == "" {
		return false
	}
	records, err := s.readGrantsFile(cleanWS)
	if err != nil {
		return false
	}

	s.mu.Lock()
	if s.cache[cleanWS] == nil {
		s.cache[cleanWS] = make(map[string]GrantRecord, len(records))
	}
	for _, r := range records {
		s.cache[cleanWS][r.Fingerprint] = r
	}
	_, hit := s.cache[cleanWS][cleanFP]
	s.mu.Unlock()
	return hit
}

func (s *GrantStore) ListGrants(wsID string) ([]GrantRecord, error) {
	if s == nil || strings.TrimSpace(wsID) == "" || strings.TrimSpace(s.paths.Root) == "" {
		return nil, nil
	}
	return s.readGrantsFile(strings.TrimSpace(wsID))
}

func (s *GrantStore) RevokeGrant(wsID, fingerprint string) error {
	if s == nil {
		return nil
	}
	cleanWS := strings.TrimSpace(wsID)
	cleanFP := strings.TrimSpace(fingerprint)
	if cleanWS == "" || cleanFP == "" || strings.TrimSpace(s.paths.Root) == "" {
		return nil
	}

	err := store.WithFlock(s.grantsLockPath(cleanWS), true, func() error {
		existing, rErr := s.readGrantsFile(cleanWS)
		if rErr != nil {
			return rErr
		}
		if len(existing) == 0 {
			return nil
		}
		remaining := make([]GrantRecord, 0, len(existing))
		for _, rec := range existing {
			if rec.Fingerprint != cleanFP {
				remaining = append(remaining, rec)
			}
		}
		encoded, mErr := toml.Marshal(grantsDoc{Grants: remaining})
		if mErr != nil {
			return fmt.Errorf("marshal grants.toml: %w", mErr)
		}
		return store.AtomicWriteFile(s.grantsFilePath(cleanWS), encoded, 0o600)
	})
	if err != nil {
		return err
	}

	s.mu.Lock()
	if wsMap, ok := s.cache[cleanWS]; ok {
		delete(wsMap, cleanFP)
	}
	s.mu.Unlock()
	return nil
}

func (s *GrantStore) ClearGrants(wsID string) error {
	if s == nil {
		return nil
	}
	cleanWS := strings.TrimSpace(wsID)
	if cleanWS == "" || strings.TrimSpace(s.paths.Root) == "" {
		return nil
	}

	err := store.WithFlock(s.grantsLockPath(cleanWS), true, func() error {
		p := s.grantsFilePath(cleanWS)
		if rmErr := os.Remove(p); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			return rmErr
		}
		return nil
	})
	if err != nil {
		return err
	}

	s.mu.Lock()
	delete(s.cache, cleanWS)
	s.mu.Unlock()
	return nil
}

func (s *GrantStore) readGrantsFile(wsID string) ([]GrantRecord, error) {
	raw, err := os.ReadFile(s.grantsFilePath(wsID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read grants.toml for %s: %w", wsID, err)
	}
	var doc grantsDoc
	if err := toml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse grants.toml for %s: %w", wsID, err)
	}
	return doc.Grants, nil
}

// ComputeFingerprint 计算绑定了 WorkspaceID、规范工作区路径、工具名、紧凑参数及 Staged DiffHunk 指纹的 SHA-256 摘要。
func ComputeFingerprint(wsID, wsRoot string, call ToolInvocation) string {
	canonWS := strings.TrimSpace(wsRoot)
	if canonWS != "" {
		if resolved, err := filepath.EvalSymlinks(canonWS); err == nil {
			canonWS = filepath.Clean(resolved)
		} else {
			canonWS = filepath.Clean(canonWS)
		}
	}

	var buf bytes.Buffer
	buf.WriteString(strings.TrimSpace(wsID))
	buf.WriteByte(0)
	buf.WriteString(canonWS)
	buf.WriteByte(0)
	buf.WriteString(strings.TrimSpace(call.ToolName))
	buf.WriteByte(0)

	trimmedArgs := bytes.TrimSpace(call.Args)
	if len(trimmedArgs) > 0 {
		var compacted bytes.Buffer
		if err := json.Compact(&compacted, trimmedArgs); err == nil {
			buf.Write(compacted.Bytes())
		} else {
			buf.Write(trimmedArgs)
		}
	} else {
		buf.WriteString("{}")
	}

	if call.Staged != nil && len(call.Staged.Hunks) > 0 {
		buf.WriteByte(0)
		buf.WriteString(call.Staged.RelPath)
		for _, h := range call.Staged.Hunks {
			buf.WriteByte(0)
			buf.WriteString(h.Fingerprint)
		}
	}

	sum := sha256.Sum256(buf.Bytes())
	return "sha256:" + hex.EncodeToString(sum[:])
}

func NormalizePermMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case PermModePlan:
		return PermModePlan
	case PermModeAsk:
		return PermModeAsk
	case PermModeAcceptEdits:
		return PermModeAcceptEdits
	case PermModeYolo:
		return PermModeYolo
	default:
		return PermModeAuto
	}
}

// DetectTamperedProjectConfig 检测工作区是否存在未经 SHA-256 白名单授信的项目级安全敏感配置（如 .gorness/mcp.toml、.mcp.json、.gorness/hooks.toml 等）。
func DetectTamperedProjectConfig(wsRoot string, trustedHashes map[string]string, untrustedWarnings []string) bool {
	for _, w := range untrustedWarnings {
		lw := strings.ToLower(w)
		if strings.Contains(lw, "untrusted workspace") ||
			strings.Contains(lw, "skipped untrusted") ||
			strings.Contains(lw, "ignored untrusted") ||
			strings.Contains(lw, "rejected unsafe") ||
			strings.Contains(lw, "rejected cmd:") {
			return true
		}
	}
	if strings.TrimSpace(wsRoot) == "" {
		return false
	}
	_, ok := config.VerifyWorkspaceTrust(wsRoot, trustedHashes)
	return !ok
}

var (
	reRmRootCmd = regexp.MustCompile(`(?i)(^|[;&|()\s])(sudo\s+)?(/(usr/)?s?bin/)?rm\s+([^\n;|&]*\s+)?(-[a-z]*r[a-z]*f|-[a-z]*f[a-z]*r|-r\s+-f|-f\s+-r|--no-preserve-root)([^\n;|&]*\s+)?(/|/\*|~|~/\*|\$HOME|\$\{HOME\})(\s|$|[;&|)])`)
	reForkBomb  = regexp.MustCompile(`:\s*\(\s*\)\s*\{.*:\s*\|\s*:.*&`)
	reAutoCfgSh = regexp.MustCompile(`(?i)(^|[\s"'/])(\.gorness(/|$)|\.mcp\.json\b|mcp\.toml\b|\.zshrc\b|\.zshenv\b|\.zprofile\b|\.zlogin\b|\.bashrc\b|\.bash_profile\b|\.profile\b|\.git/hooks/|\.git/config\b|\.github/workflows(/|$))`)
)

// PermissionGate 严格按不可绕过的固定九步顺序裁决每一次工具调用：
// 1. Hard-Floor 绝对黑名单（含 ~/.gorness 机密路径与毁灭性命令，即使 yolo 模式也拦截）
// 2. Plan 只读模式拦截一切写操作与外部 MCP 工具
// 3. 未授信/被篡改项目级安全配置降级为逐次询问（后台无人值守直接拒绝）
// 4. PreToolUse Hook 显式阻断或询问裁决
// 5. 经核验的内置只读工具免审放行（绝不采信 MCP 自报 readOnlyHint）
// 6. 修改自启动/工程敏感配置（.gorness/、.zshrc、.git/hooks 等）强制人工确认
// 7. AlwaysAsk 跨边界高危工具每次必问（跳过指纹免审）
// 8. 匹配绑定工作区与内容指纹的持久化授权（grants.toml）
// 9. 按生效权限模式（ask / accept_edits / auto / yolo）与细粒度规则裁决（无人值守自动转 GateDefer）
type PermissionGate struct {
	grants *GrantStore
}

func NewPermissionGate(grants *GrantStore) *PermissionGate {
	return &PermissionGate{grants: grants}
}

func (g *PermissionGate) Evaluate(ctx EvalContext, call ToolInvocation) GateResult {
	fp := ComputeFingerprint(ctx.WorkspaceID, ctx.WorkspaceRoot, call)
	pg := resolveEvalPathGuard(ctx)
	targetInfo := extractTargetInfo(ctx, pg, call)

	// 1. Hard-Floor 绝对拒绝底线（即使在 yolo 模式下也绝不放行）
	if reason, blocked := checkHardFloor(ctx, call, targetInfo); blocked {
		return GateResult{
			Action:      GateDeny,
			ReasonCode:  "hard-floor",
			Reason:      reason,
			Fingerprint: fp,
		}
	}
	if reason, code, blocked := checkToolACL(ctx, call); blocked {
		return GateResult{
			Action:      GateDeny,
			ReasonCode:  code,
			Reason:      reason,
			Fingerprint: fp,
		}
	}

	mode := NormalizePermMode(ctx.EffectiveMode)

	// 2. Plan 只读模式拦截一切写工具、Shell 与外部 MCP 工具
	if mode == PermModePlan && !isStrictlyReadOnlyCall(call, targetInfo) {
		return GateResult{
			Action:      GateDeny,
			ReasonCode:  "plan-readonly",
			Reason:      "mutations, shell commands, and external MCP tools are disabled in plan mode",
			Fingerprint: fp,
		}
	}

	// 3. 项目级配置内容哈希校验：若 .gorness/mcp.toml 等未授信或被篡改，交互态降级为询问，无人值守直接拒绝
	if ctx.ConfigTampered || DetectTamperedProjectConfig(ctx.WorkspaceRoot, ctx.TrustedHashes, ctx.UntrustedWarnings) {
		if ctx.Unattended {
			return GateResult{
				Action:      GateDeny,
				ReasonCode:  "tampered-project-config",
				Reason:      "untrusted or modified workspace configuration blocked in unattended mode",
				Fingerprint: fp,
			}
		}
		return GateResult{
			Action:      GateAsk,
			ReasonCode:  "tampered-project-config",
			Reason:      "workspace contains untrusted or modified .gorness configuration; human confirmation required",
			Fingerprint: fp,
		}
	}

	// 4. PreToolUse Hook 显式要求阻断、挂起或人工确认
	if ctx.HookDecision.RequiresAction() {
		dec := ctx.HookDecision
		dec.Fingerprint = fp
		if dec.Action == GateAsk && ctx.Unattended {
			dec.Action = GateDefer
		}
		if dec.ReasonCode == "" {
			dec.ReasonCode = "hook-decision"
		}
		return dec
	}

	// 5. 语义级内置只读放行（注：MCP 服务器自报的 readOnlyHint 一律不采信为免审依据！）
	if isVerifiedBuiltinReadOnly(call, targetInfo) {
		return GateResult{
			Action:      GateAllow,
			ReasonCode:  "builtin-readonly",
			Reason:      "verified built-in in-workspace read-only tool",
			Fingerprint: fp,
		}
	}

	// 6. 修改会被自动加载执行的配置文件（.gorness/、.zshrc、.mcp.json、.git/hooks/ 等）必须人工确认
	if reason, hit := touchesAutoRunConfig(call, targetInfo); hit {
		return askOrDefer(ctx, "auto-run-config", reason, fp)
	}

	// 7. AlwaysAsk 跨边界高危工具每次必问，禁止被保存的指纹授权绕过
	if call.AlwaysAsk {
		return askOrDefer(ctx, "always-ask", "tool requires explicit confirmation on every invocation", fp)
	}

	// 8. 核验绑定了工作区 ID 与内容指纹的持久化授权（grants.toml）
	if g != nil && g.grants != nil && g.grants.HasGrant(ctx.WorkspaceID, fp) {
		return GateResult{
			Action:      GateAllow,
			ReasonCode:  "saved-grant",
			Reason:      "matched saved workspace fingerprint grant",
			Fingerprint: fp,
		}
	}

	// 9. 按当前生效模式（ask / accept_edits / auto / yolo）与细粒度规则裁决
	return evaluateByMode(ctx, mode, call, targetInfo, fp)
}

func askOrDefer(ctx EvalContext, reasonCode, reason, fp string) GateResult {
	if ctx.Unattended {
		return GateResult{
			Action:      GateDefer,
			ReasonCode:  reasonCode,
			Reason:      reason,
			Fingerprint: fp,
		}
	}
	return GateResult{
		Action:      GateAsk,
		ReasonCode:  reasonCode,
		Reason:      reason,
		Fingerprint: fp,
	}
}

type callTargetInfo struct {
	hasPath       bool
	rawPath       string
	canonAbs      string
	relPath       string
	inWorkspace   bool
	isOverflowLog bool
	vaultBlocked  bool
	resolveErr    error

	hasCommand bool
	command    string
}

func resolveEvalPathGuard(ctx EvalContext) *tools.PathGuard {
	if ctx.PathGuard != nil {
		return ctx.PathGuard
	}
	if strings.TrimSpace(ctx.WorkspaceRoot) == "" {
		return nil
	}
	pg, err := tools.NewPathGuard(ctx.WorkspaceRoot, ctx.VaultRoot)
	if err != nil {
		return nil
	}
	return pg
}

func extractTargetInfo(ctx EvalContext, pg *tools.PathGuard, call ToolInvocation) callTargetInfo {
	var info callTargetInfo

	if call.Staged != nil && call.Staged.AbsPath != "" {
		info.hasPath = true
		info.rawPath = call.Staged.AbsPath
		info.canonAbs = call.Staged.AbsPath
		info.relPath = call.Staged.RelPath
		info.inWorkspace = call.Staged.RelPath != "" && !strings.HasPrefix(call.Staged.RelPath, "..")
	}

	var parsed map[string]any
	if len(bytes.TrimSpace(call.Args)) > 0 {
		_ = json.Unmarshal(call.Args, &parsed)
	}

	if cmdStr, ok := parsed["command"].(string); ok && strings.TrimSpace(cmdStr) != "" {
		info.hasCommand = true
		info.command = strings.TrimSpace(cmdStr)
	}

	if !info.hasPath {
		for _, key := range []string{"path", "workdir", "file_path", "target"} {
			if s, ok := parsed[key].(string); ok && strings.TrimSpace(s) != "" {
				info.hasPath = true
				info.rawPath = strings.TrimSpace(s)
				break
			}
		}
	}

	if info.hasPath {
		expanded := expandUserHome(info.rawPath, ctx.UserHomeDir)
		if isDirectVaultSecretRef(info.rawPath, expanded, ctx.VaultRoot, ctx.UserHomeDir, ctx.WorkspaceRoot) {
			info.vaultBlocked = true
			info.inWorkspace = false
			return info
		}
		if pg != nil {
			canonAbs, rel, inWS, err := pg.ResolveInWorkspace(expanded)
			info.canonAbs = canonAbs
			info.relPath = rel
			info.inWorkspace = inWS
			info.resolveErr = err
			if errors.Is(err, tools.ErrVaultSecretBlocked) {
				info.vaultBlocked = true
			}
			if err == nil && canonAbs != "" {
				info.isOverflowLog = pg.IsOverflowLogFile(canonAbs)
			}
		} else {
			info.canonAbs = filepath.Clean(expanded)
		}
	}

	return info
}

func expandUserHome(rawPath, injectedHome string) string {
	return config.ExpandHome(rawPath, injectedHome)
}

func isDirectVaultSecretRef(rawPath, expandedPath, vaultRoot, userHome, wsRoot string) bool {
	normRaw := strings.ReplaceAll(strings.TrimSpace(rawPath), "\\", "/")
	if normRaw == "~/.gorness" || strings.HasPrefix(normRaw, "~/.gorness/") ||
		normRaw == "$HOME/.gorness" || strings.HasPrefix(normRaw, "$HOME/.gorness/") {
		return true
	}

	checkCandidate := func(cand string) bool {
		if cand == "" {
			return false
		}
		cleanCand := canonicalOrClean(cand)
		for _, vRoot := range []string{vaultRoot, joinIfNonEmpty(userHome, ".gorness")} {
			if strings.TrimSpace(vRoot) == "" {
				continue
			}
			cleanV := canonicalOrClean(vRoot)
			if isPathInsideOrEqual(cleanCand, cleanV) {
				// 允许位于 runtime/worktrees/<wt> 下的独立工作树访问其自身目录，或只读访问 overflow 日志（由 PathGuard 细分）
				wtBase := filepath.Join(cleanV, "runtime", "worktrees")
				if strings.TrimSpace(wsRoot) != "" {
					cleanWS := canonicalOrClean(wsRoot)
					if isPathStrictlyInside(cleanWS, wtBase) && isPathInsideOrEqual(cleanCand, cleanWS) {
						return false
					}
				}
				logsDir := filepath.Join(cleanV, "runtime", "logs")
				if filepath.Dir(cleanCand) == logsDir && strings.HasPrefix(filepath.Base(cleanCand), "overflow-") && strings.HasSuffix(filepath.Base(cleanCand), ".log") {
					return false
				}
				return true
			}
		}
		return false
	}

	return checkCandidate(expandedPath)
}

func checkHardFloor(ctx EvalContext, call ToolInvocation, target callTargetInfo) (string, bool) {
	if target.vaultBlocked {
		return fmt.Sprintf("access to protected Gorness vault path %q is strictly forbidden", target.rawPath), true
	}
	// Overflow 日志仅允许只读分段查阅，绝不允许写入篡改
	if target.isOverflowLog && (call.ToolName == "edit_file" || call.ToolName == "write_file") {
		return "modifying runtime overflow logs is forbidden", true
	}

	// 检查路径黑名单（含默认 ~/.ssh/*、~/Library/Keychains/* 与 Rules.DenyPaths）
	if target.hasPath {
		if reason, hit := matchesDenyPath(ctx, target); hit {
			return reason, true
		}
	}

	// 检查毁灭性命令与凭据窃取命令
	if target.hasCommand {
		cmd := target.command
		if reRmRootCmd.MatchString(cmd) {
			return "destructive root or home removal command blocked by hard-floor", true
		}
		if reForkBomb.MatchString(cmd) {
			return "fork bomb blocked by hard-floor", true
		}
		lowerCmd := strings.ToLower(cmd)
		for _, fatalSub := range []string{
			"~/.gorness",
			"$home/.gorness",
			"${home}/.gorness",
			"security dump-keychain",
			"security find-generic-password",
			"security find-internet-password",
		} {
			if strings.Contains(lowerCmd, fatalSub) {
				return fmt.Sprintf("command referencing protected credential resource (%s) blocked by hard-floor", fatalSub), true
			}
		}
		if vRoot := strings.TrimSpace(ctx.VaultRoot); vRoot != "" {
			cleanV := canonicalOrClean(vRoot)
			lexV := filepath.Clean(vRoot)
			cleanWS := canonicalOrClean(ctx.WorkspaceRoot)
			lexWS := ""
			if strings.TrimSpace(ctx.WorkspaceRoot) != "" {
				lexWS = filepath.Clean(ctx.WorkspaceRoot)
			}

			scrubbed := cmd
			// 若当前运行位于 runtime/worktrees/<wt> 隔离工作树内，先剔除合法的 worktree 自身路径（禁止携带 /.. 穿越逃逸），再检查剩余命令是否触碰 Vault 机密区
			wtBase := filepath.Join(cleanV, "runtime", "worktrees")
			if cleanWS != "" && isPathStrictlyInside(cleanWS, wtBase) {
				if strings.Contains(scrubbed, cleanWS+"/..") {
					return "command traversing out of worktree into Gorness vault blocked by hard-floor", true
				}
				scrubbed = strings.ReplaceAll(scrubbed, cleanWS, "")
			}
			lexWTBase := filepath.Join(lexV, "runtime", "worktrees")
			if lexWS != "" && lexWS != cleanWS && isPathStrictlyInside(lexWS, lexWTBase) {
				if strings.Contains(scrubbed, lexWS+"/..") {
					return "command traversing out of worktree into Gorness vault blocked by hard-floor", true
				}
				scrubbed = strings.ReplaceAll(scrubbed, lexWS, "")
			}

			if (cleanV != "" && strings.Contains(scrubbed, cleanV)) ||
				(lexV != "" && lexV != "." && strings.Contains(scrubbed, lexV)) {
				return "command referencing Gorness vault directory blocked by hard-floor", true
			}
		}
		for _, pat := range ctx.Rules.DenyCommands {
			if matchWildcardCI(pat, cmd) {
				return fmt.Sprintf("command matched deny_commands pattern %q", pat), true
			}
		}
	}

	// 检查细粒度 Rules 中的显式 deny 项
	for _, r := range ctx.Rules.Rules {
		if strings.ToLower(strings.TrimSpace(r.Action)) != "deny" {
			continue
		}
		if r.Tool != "" && !matchWildcardCI(r.Tool, call.ToolName) {
			continue
		}
		subject := target.command
		if subject == "" {
			subject = target.rawPath
		}
		if subject == "" {
			subject = string(call.Args)
		}
		if matchWildcardCI(r.Pattern, subject) {
			msg := strings.TrimSpace(r.Reason)
			if msg == "" {
				msg = fmt.Sprintf("blocked by deny rule %q", r.Pattern)
			}
			return msg, true
		}
	}

	return "", false
}

func matchesDenyPath(ctx EvalContext, target callTargetInfo) (string, bool) {
	home := strings.TrimSpace(ctx.UserHomeDir)
	if home == "" {
		home, _ = os.UserHomeDir()
	}

	builtinDeny := []string{
		"~/.gorness/*",
		"~/.gorness",
		"~/.ssh/*",
		"~/.ssh",
		"~/Library/Keychains/*",
	}
	patterns := append(builtinDeny, ctx.Rules.DenyPaths...)

	candidates := collectTargetPathCandidates(target, home)

	isManagedWorktree := false
	if strings.TrimSpace(ctx.VaultRoot) != "" && strings.TrimSpace(ctx.WorkspaceRoot) != "" {
		cleanV := canonicalOrClean(ctx.VaultRoot)
		cleanWS := canonicalOrClean(ctx.WorkspaceRoot)
		if isPathStrictlyInside(cleanWS, filepath.Join(cleanV, "runtime", "worktrees")) {
			isManagedWorktree = true
		}
	}

	for _, rawPat := range patterns {
		trimmedPat := strings.TrimSpace(rawPat)
		if ((isManagedWorktree && target.inWorkspace) || target.isOverflowLog) &&
			(trimmedPat == "~/.gorness/*" || trimmedPat == "~/.gorness") {
			continue
		}
		expandedPats := expandRulePathPatterns(rawPat, home, ctx.WorkspaceRoot, ctx.VaultRoot)
		for _, ep := range expandedPats {
			baseDir := strings.TrimSuffix(ep, "/*")
			for _, cand := range candidates {
				if cand == "" {
					continue
				}
				if matchWildcardPath(ep, cand) || (strings.HasSuffix(ep, "/*") && isPathInsideOrEqual(filepath.FromSlash(cand), filepath.FromSlash(baseDir))) {
					return fmt.Sprintf("path %q matched hard-floor deny_paths rule %q", target.rawPath, rawPat), true
				}
			}
		}
	}
	return "", false
}

func checkToolACL(ctx EvalContext, call ToolInvocation) (reason, code string, blocked bool) {
	name := strings.TrimSpace(call.ToolName)
	if ctx.AgentProfile != nil {
		for _, dis := range ctx.AgentProfile.DisallowedTools {
			if strings.TrimSpace(dis) == name {
				return fmt.Sprintf("tool %q is disallowed for agent role %q", name, ctx.AgentProfile.ID), "agent-disallowed-tool", true
			}
		}
		if ctx.AgentProfile.AllowedTools != nil {
			isSearchHelper := name == "search_tools" && len(ctx.AgentProfile.AllowedTools) > 0
			if !isSearchHelper && !containsString(ctx.AgentProfile.AllowedTools, name) {
				return fmt.Sprintf("tool %q is not in allowed_tools for agent role %q", name, ctx.AgentProfile.ID), "agent-tool-not-allowed", true
			}
		}
		if ctx.AgentProfile.ReadOnly && (name == "edit_file" || name == "write_file" || isMCPInvocation(call)) {
			return fmt.Sprintf("mutating or external tool %q is disabled for read-only role %q", name, ctx.AgentProfile.ID), "agent-readonly", true
		}
	}

	if ctx.AllowedTools != nil {
		isSearchHelper := name == "search_tools" && len(ctx.AllowedTools) > 0
		if !isSearchHelper && !containsString(ctx.AllowedTools, name) {
			return fmt.Sprintf("tool %q is outside run AllowedTools restriction", name), "subagent-tool-not-allowed", true
		}
	}
	return "", "", false
}

func isMCPInvocation(call ToolInvocation) bool {
	if call.Tool != nil && call.Tool.IsMCP {
		return true
	}
	return strings.HasPrefix(strings.TrimSpace(call.ToolName), "mcp__")
}

func isStrictlyReadOnlyCall(call ToolInvocation, target callTargetInfo) bool {
	if isMCPInvocation(call) {
		return false
	}
	if call.Tool != nil {
		if !call.Tool.ReadOnly || call.Tool.Mutating || call.Tool.IsMCP {
			return false
		}
	} else {
		switch call.ToolName {
		case "read_file", "search_code", "search_tools", "activate_skill":
		default:
			return false
		}
	}
	if target.hasPath && target.resolveErr != nil {
		return false
	}
	if target.hasPath && !target.inWorkspace && !target.isOverflowLog {
		return false
	}
	return true
}

func isVerifiedBuiltinReadOnly(call ToolInvocation, target callTargetInfo) bool {
	// 安全铁律：任何外部 MCP 工具（即便自报 annotations.readOnlyHint == true）一律不作为内置只读工具免审！
	return isStrictlyReadOnlyCall(call, target)
}

func touchesAutoRunConfig(call ToolInvocation, target callTargetInfo) (string, bool) {
	isMutatingFileTool := call.ToolName == "edit_file" || call.ToolName == "write_file" ||
		(call.Tool != nil && call.Tool.Mutating && !call.Tool.IsMCP && target.hasPath && !target.hasCommand)

	if isMutatingFileTool && target.hasPath {
		if tools.IsAutoRunConfigPath(target.canonAbs, target.relPath) ||
			tools.IsAutoRunConfigPath(target.rawPath, target.relPath) ||
			isSkillDefinitionPath(target.canonAbs, target.relPath, target.rawPath) {
			return fmt.Sprintf("modifying auto-executed or sensitive configuration %q requires human confirmation", target.rawPath), true
		}
	}

	if target.hasCommand && reAutoCfgSh.MatchString(target.command) {
		return "command touches auto-executed configuration (.gorness, shell rc, MCP, or git hooks)", true
	}

	return "", false
}

func isSkillDefinitionPath(canonAbs, rel, raw string) bool {
	for _, s := range []string{rel, canonAbs, raw} {
		slash := filepath.ToSlash(strings.TrimSpace(s))
		if strings.Contains(slash, ".agents/skills/") || strings.Contains(slash, ".claude/skills/") || strings.HasSuffix(slash, "/SKILL.md") {
			return true
		}
	}
	return false
}

func evaluateByMode(ctx EvalContext, mode string, call ToolInvocation, target callTargetInfo, fp string) GateResult {
	// 细粒度 Rules 中的显式 ask / allow 匹配
	for _, r := range ctx.Rules.Rules {
		if r.Tool != "" && !matchWildcardCI(r.Tool, call.ToolName) {
			continue
		}
		subject := target.command
		if subject == "" {
			subject = target.rawPath
		}
		if subject == "" {
			subject = string(call.Args)
		}
		if !matchWildcardCI(r.Pattern, subject) {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(r.Action)) {
		case "ask":
			reason := strings.TrimSpace(r.Reason)
			if reason == "" {
				reason = fmt.Sprintf("matched ask rule %q", r.Pattern)
			}
			return askOrDefer(ctx, "rule-ask", reason, fp)
		case "allow":
			if mode != PermModeAsk {
				return GateResult{
					Action:      GateAllow,
					ReasonCode:  "rule-allow",
					Reason:      fmt.Sprintf("matched allow rule %q", r.Pattern),
					Fingerprint: fp,
				}
			}
		}
	}

	// 工作区外路径访问检查（除非在 AllowPaths 白名单内或处于 yolo 模式）
	if target.hasPath && !target.inWorkspace && !target.isOverflowLog {
		if mode != PermModeYolo && !matchesAllowPaths(ctx, target) {
			return askOrDefer(ctx, "outside-workspace", fmt.Sprintf("path %q resolves outside workspace root", target.rawPath), fp)
		}
	}

	switch mode {
	case PermModeYolo:
		return GateResult{
			Action:      GateAllow,
			ReasonCode:  "mode-yolo",
			Reason:      "allowed in yolo mode after passing hard-floor and auto-run-config checks",
			Fingerprint: fp,
		}

	case PermModeAsk:
		return askOrDefer(ctx, "mode-ask", fmt.Sprintf("tool %q requires confirmation in ask mode", call.ToolName), fp)

	case PermModeAcceptEdits:
		if isMCPInvocation(call) {
			return askOrDefer(ctx, "mcp-tool", fmt.Sprintf("external MCP tool %q requires confirmation", call.ToolName), fp)
		}
		if call.ToolName == "edit_file" || call.ToolName == "write_file" {
			return GateResult{
				Action:      GateAllow,
				ReasonCode:  "accept-edits",
				Reason:      "file edit allowed in accept_edits mode",
				Fingerprint: fp,
			}
		}
		if call.ToolName == "read_file" || call.ToolName == "search_code" || (call.Tool != nil && !call.Tool.Mutating && !call.Tool.IsMCP && !target.hasCommand) {
			return GateResult{
				Action:      GateAllow,
				ReasonCode:  "accept-edits-read",
				Reason:      "non-mutating local tool allowed in accept_edits mode",
				Fingerprint: fp,
			}
		}
		if target.hasCommand {
			for _, allowPat := range ctx.Rules.AllowCommands {
				if matchWildcardCI(allowPat, target.command) {
					return GateResult{
						Action:      GateAllow,
						ReasonCode:  "rule-allow-command",
						Reason:      fmt.Sprintf("command matched allow_commands %q", allowPat),
						Fingerprint: fp,
					}
				}
			}
			return askOrDefer(ctx, "command-confirm", "shell command requires confirmation in accept_edits mode", fp)
		}
		return askOrDefer(ctx, "accept-edits-confirm", fmt.Sprintf("tool %q requires confirmation", call.ToolName), fp)

	default: // PermModeAuto
		if isMCPInvocation(call) {
			return askOrDefer(ctx, "mcp-tool", fmt.Sprintf("external MCP tool %q requires confirmation", call.ToolName), fp)
		}
		if target.hasCommand {
			if reason, risky := isRiskyCommand(ctx.Rules, target.command); risky {
				return askOrDefer(ctx, "risky-command", reason, fp)
			}
			return GateResult{
				Action:      GateAllow,
				ReasonCode:  "auto-safe-command",
				Reason:      "sandboxed in-workspace command allowed in auto mode",
				Fingerprint: fp,
			}
		}
		if call.ToolName == "edit_file" || call.ToolName == "write_file" {
			return GateResult{
				Action:      GateAllow,
				ReasonCode:  "auto-in-workspace",
				Reason:      "snapshot-backed in-workspace edit allowed in auto mode",
				Fingerprint: fp,
			}
		}
		if call.Tool != nil && !call.Tool.Mutating && !call.Tool.IsMCP {
			return GateResult{
				Action:      GateAllow,
				ReasonCode:  "auto-non-mutating",
				Reason:      "non-mutating local tool allowed in auto mode",
				Fingerprint: fp,
			}
		}
		return askOrDefer(ctx, "auto-confirm", fmt.Sprintf("tool %q requires confirmation", call.ToolName), fp)
	}
}

func isRiskyCommand(rules config.RuleSet, cmd string) (string, bool) {
	for _, pat := range rules.AskCommands {
		if matchWildcardCI(pat, cmd) {
			return fmt.Sprintf("command matched ask_commands pattern %q", pat), true
		}
	}
	lower := strings.ToLower(strings.TrimSpace(cmd))
	riskyPrefixes := []string{
		"git push",
		"git reset --hard",
		"git clean -fd",
		"git checkout -- .",
		"sudo ",
		"rm -rf ",
		"rm -fr ",
		"curl ",
		"wget ",
		"ssh ",
		"scp ",
		"rsync ",
		"npm publish",
		"cargo publish",
		"docker push",
		"kubectl apply",
		"kubectl delete",
		"terraform apply",
		"terraform destroy",
		"launchctl ",
		"osascript ",
		"gh pr merge",
		"gh release create",
	}
	for _, pref := range riskyPrefixes {
		if strings.HasPrefix(lower, pref) ||
			strings.Contains(lower, "&& "+pref) ||
			strings.Contains(lower, "; "+pref) ||
			strings.Contains(lower, "| "+pref) {
			for _, allowPat := range rules.AllowCommands {
				if matchWildcardCI(allowPat, cmd) {
					return "", false
				}
			}
			return fmt.Sprintf("high-impact or external command (%s) requires confirmation", strings.TrimSpace(pref)), true
		}
	}
	return "", false
}

func matchesAllowPaths(ctx EvalContext, target callTargetInfo) bool {
	if len(ctx.Rules.AllowPaths) == 0 {
		return false
	}
	home := ctx.UserHomeDir
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	candidates := collectTargetPathCandidates(target, home)
	for _, rawPat := range ctx.Rules.AllowPaths {
		for _, ep := range expandRulePathPatterns(rawPat, home, ctx.WorkspaceRoot, ctx.VaultRoot) {
			base := strings.TrimSuffix(ep, "/*")
			for _, cand := range candidates {
				if cand == "" {
					continue
				}
				if matchWildcardPath(ep, cand) || (strings.HasSuffix(ep, "/*") && isPathInsideOrEqual(filepath.FromSlash(cand), filepath.FromSlash(base))) {
					return true
				}
			}
		}
	}
	return false
}

func collectTargetPathCandidates(target callTargetInfo, home string) []string {
	var out []string
	add := func(s string) {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" {
			out = append(out, filepath.ToSlash(trimmed))
		}
	}
	add(target.rawPath)
	add(target.relPath)
	add(target.canonAbs)
	if exp := expandUserHome(target.rawPath, home); exp != "" && exp != target.rawPath {
		add(exp)
		add(canonicalizePatternPath(filepath.ToSlash(exp)))
	}
	return out
}

func expandRulePathPatterns(rawPat, home, wsRoot, vaultRoot string) []string {
	pat := strings.TrimSpace(rawPat)
	if pat == "" {
		return nil
	}
	var out []string
	seen := make(map[string]bool)
	add := func(p string) {
		slash := filepath.ToSlash(strings.TrimSpace(p))
		if slash == "" || seen[slash] {
			return
		}
		seen[slash] = true
		out = append(out, slash)
	}

	add(pat)
	if home != "" && (pat == "~" || strings.HasPrefix(pat, "~/")) {
		exp := expandUserHome(pat, home)
		add(exp)
		add(canonicalizePatternPath(filepath.ToSlash(exp)))
	}
	if vaultRoot != "" && strings.HasPrefix(pat, "~/.gorness") {
		suffix := strings.TrimPrefix(pat, "~/.gorness")
		vPat := filepath.Clean(vaultRoot) + suffix
		add(vPat)
		add(canonicalizePatternPath(filepath.ToSlash(vPat)))
	}
	if strings.HasPrefix(pat, "/") {
		add(canonicalizePatternPath(filepath.ToSlash(pat)))
	} else if !strings.HasPrefix(pat, "~") && strings.TrimSpace(wsRoot) != "" {
		canonWS := canonicalOrClean(wsRoot)
		add(canonWS + "/" + strings.TrimPrefix(filepath.ToSlash(pat), "./"))
	}
	return out
}

func canonicalizePatternPath(slashPat string) string {
	if !strings.HasPrefix(slashPat, "/") {
		return slashPat
	}
	wildIdx := strings.IndexAny(slashPat, "*?[")
	prefix := slashPat
	suffix := ""
	if wildIdx >= 0 {
		lastSlash := strings.LastIndexByte(slashPat[:wildIdx], '/')
		if lastSlash <= 0 {
			return slashPat
		}
		prefix = slashPat[:lastSlash]
		suffix = slashPat[lastSlash:]
	}

	// 沿已存在的祖先目录解析 macOS 软链（如 /var -> /private/var），保留尚未创建的末级路径段与通配符后缀
	cur := filepath.Clean(filepath.FromSlash(prefix))
	var tail []string
	for cur != "/" && cur != "." {
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			resolved = filepath.Clean(resolved)
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return filepath.ToSlash(resolved) + suffix
		}
		tail = append(tail, filepath.Base(cur))
		cur = filepath.Dir(cur)
	}
	return slashPat
}

func matchWildcardCI(pattern, value string) bool {
	p := strings.ToLower(strings.TrimSpace(pattern))
	v := strings.ToLower(strings.TrimSpace(value))
	if p == "" {
		return false
	}
	if p == "*" {
		return true
	}
	return wildcardMatch(p, v)
}

func matchWildcardPath(pattern, value string) bool {
	p := strings.TrimSpace(pattern)
	v := strings.TrimSpace(value)
	if p == "" || v == "" {
		return false
	}
	if ok, err := path.Match(p, v); err == nil && ok {
		return true
	}
	return wildcardMatch(p, v)
}

func wildcardMatch(pat, str string) bool {
	px, sx := 0, 0
	nextPx, nextSx := 0, 0
	for px < len(pat) || sx < len(str) {
		if px < len(pat) {
			c := pat[px]
			switch c {
			case '?':
				if sx < len(str) {
					px++
					sx++
					continue
				}
			case '*':
				nextPx = px
				nextSx = sx + 1
				px++
				continue
			default:
				if sx < len(str) && str[sx] == c {
					px++
					sx++
					continue
				}
			}
		}
		if nextSx > 0 && nextSx <= len(str) {
			px = nextPx
			sx = nextSx
			continue
		}
		return false
	}
	return true
}

func containsString(list []string, target string) bool {
	for _, item := range list {
		if strings.TrimSpace(item) == target {
			return true
		}
	}
	return false
}

func canonicalOrClean(p string) string {
	trimmed := strings.TrimSpace(p)
	if trimmed == "" {
		return ""
	}
	if canon, err := config.CanonicalPath(trimmed); err == nil && canon != "" {
		return canon
	}
	if abs, err := filepath.Abs(trimmed); err == nil {
		trimmed = abs
	}
	return filepath.Clean(trimmed)
}

func joinIfNonEmpty(base, child string) string {
	if strings.TrimSpace(base) == "" {
		return ""
	}
	return filepath.Join(base, child)
}

func isPathInsideOrEqual(target, root string) bool {
	return config.IsWithinRoot(target, root)
}

func isPathStrictlyInside(target, root string) bool {
	return config.IsStrictlyWithinRoot(target, root)
}
