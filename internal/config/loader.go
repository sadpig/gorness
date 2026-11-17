package config

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type Options struct {
	VaultRoot      string
	UserHomeDir    string
	KeychainFunc   KeychainFunc
	KeychainStore  KeychainStore
	SecurityRunner SecurityRunnerFunc
	EnvLookup      func(key string) (string, bool)
	CmdRunner      func(ctx context.Context, shellCmd string) (string, error)
	CmdTTL         time.Duration
	Now            func() time.Time
}

type fileLoadError struct {
	File string
	Err  error
}

func (e *fileLoadError) Error() string {
	return fmt.Sprintf("%s: %v", e.File, e.Err)
}

func (e *fileLoadError) Unwrap() error {
	return e.Err
}

// Manager 负责初始化 ~/.gorness/ 零数据库目录骨架、维护原子活跃配置快照、解析项目覆盖并执行 fsnotify 热更新。
type Manager struct {
	paths       VaultPaths
	userHomeDir string
	secrets     *DefaultSecretResolver
	now         func() time.Time
	current     atomic.Pointer[Snapshot]

	reloadMu    sync.RWMutex
	reloadSeq   uint64
	onReloadFns map[uint64]func(*Snapshot)
}

func NewManager(opts Options) (*Manager, error) {
	homeDir := opts.UserHomeDir
	vaultRoot := opts.VaultRoot

	if vaultRoot == "" {
		if homeDir == "" {
			var err error
			homeDir, err = os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("resolve user home dir: %w", err)
			}
		}
		vaultRoot = filepath.Join(homeDir, ".gorness")
	} else if homeDir == "" {
		// 测试隔离：若显式传入 VaultRoot 而未指定 UserHomeDir，默认取其父目录，绝不触碰宿主机真实 ~ 目录。
		homeDir = filepath.Dir(vaultRoot)
	}

	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	paths := VaultPaths{Root: vaultRoot}
	if err := bootstrapVault(paths); err != nil {
		return nil, err
	}

	resolver := NewSecretResolver(SecretResolverOptions{
		KeychainFunc:      opts.KeychainFunc,
		KeychainStore:     opts.KeychainStore,
		SecurityRunner:    opts.SecurityRunner,
		EnvLookup:         opts.EnvLookup,
		CmdRunner:         opts.CmdRunner,
		CmdTTL:            opts.CmdTTL,
		Now:               nowFn,
		DefaultSourceFile: paths.ProvidersFile(),
	})

	m := &Manager{
		paths:       paths,
		userHomeDir: homeDir,
		secrets:     resolver,
		now:         nowFn,
	}

	snap, err := m.loadGlobalSnapshot()
	if err != nil {
		return nil, err
	}
	m.current.Store(snap)
	return m, nil
}

func (m *Manager) Paths() VaultPaths {
	return m.paths
}

func (m *Manager) Secrets() SecretResolver {
	return m.secrets
}

func (m *Manager) Keychain() KeychainStore {
	if m == nil || m.secrets == nil {
		return NewMacOSKeychainStore(nil)
	}
	return m.secrets.Keychain()
}

func (m *Manager) SetSecret(key, value string) error {
	if m == nil || m.secrets == nil {
		return errors.New("nil config manager")
	}
	return m.secrets.SetSecret(key, value)
}

func (m *Manager) DeleteSecret(key string) error {
	if m == nil || m.secrets == nil {
		return errors.New("nil config manager")
	}
	return m.secrets.DeleteSecret(key)
}

func (m *Manager) Current() *Snapshot {
	return m.current.Load()
}

// Reload 同步校验并原子替换全局活跃配置快照；若磁盘配置存在语法错误则保留原合法快照不变并返回诊断错误。
func (m *Manager) Reload() (*Snapshot, error) {
	if m == nil {
		return nil, errors.New("nil config manager")
	}
	snap, err := m.loadGlobalSnapshot()
	if err != nil {
		return m.Current(), err
	}
	m.current.Store(snap)
	m.notifyReload(snap)
	return snap, nil
}

// OnReload registers a listener invoked whenever Reload or Watch commits a new Snapshot.
func (m *Manager) OnReload(fn func(*Snapshot)) func() {
	if m == nil || fn == nil {
		return func() {}
	}
	m.reloadMu.Lock()
	if m.onReloadFns == nil {
		m.onReloadFns = make(map[uint64]func(*Snapshot))
	}
	m.reloadSeq++
	id := m.reloadSeq
	m.onReloadFns[id] = fn
	m.reloadMu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			m.reloadMu.Lock()
			delete(m.onReloadFns, id)
			m.reloadMu.Unlock()
		})
	}
}

func (m *Manager) notifyReload(snap *Snapshot) {
	if m == nil || snap == nil {
		return
	}
	m.reloadMu.RLock()
	fns := make([]func(*Snapshot), 0, len(m.onReloadFns))
	for _, fn := range m.onReloadFns {
		fns = append(fns, fn)
	}
	m.reloadMu.RUnlock()

	for _, fn := range fns {
		fn(snap)
	}
}

// ForWorkspace 基于当前全局快照合并项目 <wsRoot>/.gorness/ 局部配置：
// 1. 强制规范路径校验：任何逃逸出 canonWs 或指向 ~/.gorness/ 的符号链接一律拒绝读取，阻断宿主机密钥经 SystemPrompt 外泄；
// 2. 项目级 mcp.toml、providers.toml 与 agents/*.md 的 api_key 必须通过 SHA-256 白名单核验方可生效；
// 3. 项目级覆盖中的任何 cmd: 动态命令凭据一律无条件拦截（即使文件哈希在 trustedHashes 中也拒绝执行）。
func (m *Manager) ForWorkspace(wsRoot string, trustedHashes map[string]string) (*Snapshot, []string, error) {
	base := m.Current()
	if base == nil {
		return nil, nil, errors.New("manager has no active snapshot")
	}
	snap := base.Clone()
	var warnings []string

	if wsRoot == "" {
		return snap, nil, nil
	}

	canonWs, err := filepath.EvalSymlinks(wsRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return snap, nil, nil
		}
		return nil, nil, fmt.Errorf("resolve workspace root %s: %w", wsRoot, err)
	}
	canonWs = filepath.Clean(canonWs)

	canonVault := filepath.Clean(m.paths.Root)
	if resolvedVault, err := filepath.EvalSymlinks(m.paths.Root); err == nil {
		canonVault = filepath.Clean(resolvedVault)
	}

	if isPathWithinOrEqual(canonWs, canonVault) {
		warnings = append(warnings, fmt.Sprintf("rejected workspace %s inside gorness vault %s", wsRoot, m.paths.Root))
		snap.UntrustedWarnings = warnings
		return snap, warnings, nil
	}

	// 用户级技能目录排在前面，确保同名用户技能优先覆盖项目内技能；逃逸边界的技能目录软链不予挂载。
	for _, candSkillDir := range []string{
		filepath.Join(wsRoot, ".gorness", "skills"),
		filepath.Join(wsRoot, ".agents", "skills"),
		filepath.Join(wsRoot, ".claude", "skills"),
	} {
		_, exists, safe := isSafeWorkspacePath(candSkillDir, canonWs, canonVault)
		if exists && !safe {
			warnings = append(warnings, fmt.Sprintf("rejected unsafe symlinked workspace skill dir %s", candSkillDir))
			continue
		}
		snap.SkillDirs = append(snap.SkillDirs, candSkillDir)
	}

	wsGorness := filepath.Join(wsRoot, ".gorness")
	canonGorness, exists, safe := isSafeWorkspacePath(wsGorness, canonWs, canonVault)
	if !exists {
		snap.UntrustedWarnings = warnings
		return snap, warnings, nil
	}
	if !safe {
		warnings = append(warnings, fmt.Sprintf("rejected unsafe workspace .gorness directory %s: symlink escapes workspace or enters vault", wsGorness))
		snap.UntrustedWarnings = warnings
		return snap, warnings, nil
	}
	st, err := os.Stat(canonGorness)
	if err != nil || !st.IsDir() {
		snap.UntrustedWarnings = warnings
		return snap, warnings, nil
	}

	// 0. config.toml 安全偏好覆盖（仅允许覆盖模型/推理强度/界面偏好，不允许项目配置擅自放宽沙箱或权限级别）
	wsConfigPath := filepath.Join(wsGorness, "config.toml")
	if raw, ok, err := readSafeWorkspaceFile(wsConfigPath, canonWs, canonVault, &warnings); err != nil {
		return nil, nil, err
	} else if ok {
		var wsCfg struct {
			DefaultProvider string `toml:"default_provider"`
			DefaultModel    string `toml:"default_model"`
			DefaultEffort   string `toml:"default_effort"`
			Theme           string `toml:"theme"`
			Language        string `toml:"language"`
			UITheme         string `toml:"ui_theme"`
			UILang          string `toml:"ui_lang"`
			MaxSteps        int    `toml:"max_steps"`
		}
		if err := toml.Unmarshal(raw, &wsCfg); err != nil {
			return nil, nil, &fileLoadError{File: wsConfigPath, Err: err}
		}
		if wsCfg.DefaultProvider != "" {
			snap.Global.DefaultProvider = wsCfg.DefaultProvider
		}
		if wsCfg.DefaultModel != "" {
			snap.Global.DefaultModel = wsCfg.DefaultModel
		}
		if wsCfg.DefaultEffort != "" {
			snap.Global.DefaultEffort = wsCfg.DefaultEffort
		}
		if wsCfg.Theme != "" {
			snap.Global.Theme = wsCfg.Theme
		}
		if wsCfg.Language != "" {
			snap.Global.Language = wsCfg.Language
			if wsCfg.UILang == "" {
				snap.Global.UILang = ""
			}
		}
		if wsCfg.UITheme != "" {
			snap.Global.UITheme = wsCfg.UITheme
		}
		if wsCfg.UILang != "" {
			snap.Global.UILang = wsCfg.UILang
		}
		normalizeGlobalLanguage(&snap.Global)
		if wsCfg.MaxSteps > 0 {
			snap.Global.MaxSteps = wsCfg.MaxSteps
		}
	}

	// 1. agents/*.md 角色覆盖（逐文件校验符号链接边界；cmd: 无条件拒绝；非空 api_key 需文件 SHA-256 授信）
	wsAgentsDir := filepath.Join(wsGorness, "agents")
	if canonAgentsDir, dirExists, dirSafe := isSafeWorkspacePath(wsAgentsDir, canonWs, canonVault); dirExists {
		if !dirSafe {
			warnings = append(warnings, fmt.Sprintf("rejected unsafe symlinked workspace agents dir %s", wsAgentsDir))
		} else if info, err := os.Stat(canonAgentsDir); err == nil && info.IsDir() {
			entries, err := os.ReadDir(canonAgentsDir)
			if err != nil {
				return nil, nil, &fileLoadError{File: wsAgentsDir, Err: err}
			}
			names := make([]string, 0, len(entries))
			for _, ent := range entries {
				if strings.HasSuffix(ent.Name(), ".md") {
					names = append(names, ent.Name())
				}
			}
			sort.Strings(names)
			for _, name := range names {
				agentPath := filepath.Join(wsAgentsDir, name)
				raw, ok, err := readSafeWorkspaceFile(agentPath, canonWs, canonVault, &warnings)
				if err != nil {
					return nil, nil, err
				}
				if !ok {
					continue
				}
				prof, err := parseAgentProfileBytes(agentPath, name, raw)
				if err != nil {
					return nil, nil, err
				}
				if hasCmdSecret(prof.APIKeyRef) {
					warnings = append(warnings, fmt.Sprintf("rejected cmd: credential in workspace agent %s: cmd: is forbidden in project overlays", agentPath))
					prof.APIKeyRef = ""
				} else if strings.TrimSpace(prof.APIKeyRef) != "" {
					if !isWorkspaceFileTrusted(wsRoot, canonWs, agentPath, raw, trustedHashes) {
						warnings = append(warnings, fmt.Sprintf("stripped untrusted api_key in workspace agent %s (SHA-256 not in trustedHashes)", agentPath))
						prof.APIKeyRef = ""
					} else if !isDynamicSecretRef(prof.APIKeyRef) {
						m.secrets.RegisterLiteralSource(strings.TrimSpace(prof.APIKeyRef), agentPath)
						tightenFilePerm0600(agentPath)
					}
				}
				snap.Agents[prof.ID] = prof
			}
		}
	}

	// 2. workflows/*.toml 拓扑覆盖（逐文件校验符号链接边界；未授信工作流不放行自定义 verify_command）
	wsWorkflowsDir := filepath.Join(wsGorness, "workflows")
	if canonWorkflowsDir, dirExists, dirSafe := isSafeWorkspacePath(wsWorkflowsDir, canonWs, canonVault); dirExists {
		if !dirSafe {
			warnings = append(warnings, fmt.Sprintf("rejected unsafe symlinked workspace workflows dir %s", wsWorkflowsDir))
		} else if info, err := os.Stat(canonWorkflowsDir); err == nil && info.IsDir() {
			entries, err := os.ReadDir(canonWorkflowsDir)
			if err != nil {
				return nil, nil, &fileLoadError{File: wsWorkflowsDir, Err: err}
			}
			for _, ent := range entries {
				if !strings.HasSuffix(ent.Name(), ".toml") {
					continue
				}
				wfPath := filepath.Join(wsWorkflowsDir, ent.Name())
				raw, ok, err := readSafeWorkspaceFile(wfPath, canonWs, canonVault, &warnings)
				if err != nil {
					return nil, nil, err
				}
				if !ok {
					continue
				}
				wf, err := parseWorkflowFile(wfPath, raw)
				if err != nil {
					return nil, nil, err
				}
				if wf.VerifyCommand != "" && !isWorkspaceFileTrusted(wsRoot, canonWs, wfPath, raw, trustedHashes) {
					warnings = append(warnings, fmt.Sprintf("stripped untrusted verify_command in workspace workflow %s", wfPath))
					wf.VerifyCommand = ""
				}
				snap.Workflows[wf.ID] = wf
			}
		}
	}

	// 3. providers.toml 覆盖（校验符号链接边界；文件需哈希授信，且 cmd: 永远无条件拒绝）
	wsProvidersPath := filepath.Join(wsGorness, "providers.toml")
	if raw, ok, err := readSafeWorkspaceFile(wsProvidersPath, canonWs, canonVault, &warnings); err != nil {
		return nil, nil, err
	} else if ok {
		provs, err := parseProvidersBytes(wsProvidersPath, raw)
		if err != nil {
			return nil, nil, err
		}
		trusted := isWorkspaceFileTrusted(wsRoot, canonWs, wsProvidersPath, raw, trustedHashes)
		for id, p := range provs {
			if providerHasCmdSecret(p) {
				warnings = append(warnings, fmt.Sprintf("rejected cmd: credential for provider %q in workspace overlay %s: cmd: is always forbidden in project overlays", id, wsProvidersPath))
				continue
			}
			if !trusted {
				warnings = append(warnings, fmt.Sprintf("skipped untrusted workspace provider %q in %s (SHA-256 not in trustedHashes)", id, wsProvidersPath))
				continue
			}
			registerProviderLiteralSecret(m.secrets, p)
			snap.Providers[id] = p
		}
	}

	// 4. mcp.toml 覆盖（校验符号链接边界；必须通过 SHA-256 白名单核验，且拦截 cmd: 环境变量/头）
	wsMCPPath := filepath.Join(wsGorness, "mcp.toml")
	if raw, ok, err := readSafeWorkspaceFile(wsMCPPath, canonWs, canonVault, &warnings); err != nil {
		return nil, nil, err
	} else if ok {
		if !isWorkspaceFileTrusted(wsRoot, canonWs, wsMCPPath, raw, trustedHashes) {
			warnings = append(warnings, fmt.Sprintf("skipped untrusted workspace MCP config %s (SHA-256 not in trustedHashes)", wsMCPPath))
		} else {
			wsMCP, err := parseMCPBytes(wsMCPPath, raw)
			if err != nil {
				return nil, nil, err
			}
			for id, srv := range wsMCP.Servers {
				if mcpHasCmdSecret(srv) {
					warnings = append(warnings, fmt.Sprintf("rejected cmd: secret in workspace MCP server %q (%s)", id, wsMCPPath))
					continue
				}
				snap.MCP.Servers[id] = srv
			}
		}
	}

	// 5. rules.toml 覆盖（校验符号链接边界；始终允许收紧 deny/ask；放宽 allow 需哈希授信）
	wsRulesPath := filepath.Join(wsGorness, "rules.toml")
	if raw, ok, err := readSafeWorkspaceFile(wsRulesPath, canonWs, canonVault, &warnings); err != nil {
		return nil, nil, err
	} else if ok {
		var wsRules RuleSet
		if err := toml.Unmarshal(raw, &wsRules); err != nil {
			return nil, nil, &fileLoadError{File: wsRulesPath, Err: err}
		}
		snap.Rules.DenyCommands = append(snap.Rules.DenyCommands, wsRules.DenyCommands...)
		snap.Rules.DenyPaths = append(snap.Rules.DenyPaths, wsRules.DenyPaths...)
		snap.Rules.AskCommands = append(snap.Rules.AskCommands, wsRules.AskCommands...)
		if isWorkspaceFileTrusted(wsRoot, canonWs, wsRulesPath, raw, trustedHashes) {
			snap.Rules.AllowCommands = append(snap.Rules.AllowCommands, wsRules.AllowCommands...)
			snap.Rules.AllowPaths = append(snap.Rules.AllowPaths, wsRules.AllowPaths...)
			snap.Rules.AllowedDomains = append(snap.Rules.AllowedDomains, wsRules.AllowedDomains...)
			snap.Rules.Rules = append(snap.Rules.Rules, wsRules.Rules...)
		} else if len(wsRules.AllowCommands)+len(wsRules.AllowPaths)+len(wsRules.AllowedDomains)+len(wsRules.Rules) > 0 {
			warnings = append(warnings, fmt.Sprintf("ignored untrusted allow rules in %s", wsRulesPath))
		}
	}

	// 6. hooks.toml 覆盖（校验符号链接边界；未授信工作区 .gorness/hooks.toml 一律跳过并告警）
	wsHooksPath := filepath.Join(wsGorness, "hooks.toml")
	if raw, ok, err := readSafeWorkspaceFile(wsHooksPath, canonWs, canonVault, &warnings); err != nil {
		return nil, nil, err
	} else if ok {
		if !isWorkspaceFileTrusted(wsRoot, canonWs, wsHooksPath, raw, trustedHashes) {
			warnings = append(warnings, fmt.Sprintf("skipped untrusted workspace hooks config %s (SHA-256 not in trustedHashes)", wsHooksPath))
		} else {
			wsHooks, err := parseHooksTOML(raw, wsHooksPath)
			if err != nil {
				return nil, nil, &fileLoadError{File: wsHooksPath, Err: err}
			}
			snap.Hooks = snap.Hooks.Merge(wsHooks)
		}
	}

	snap.UntrustedWarnings = warnings
	return snap, warnings, nil
}

func isPathWithinOrEqual(path, root string) bool {
	return IsWithinRoot(path, root)
}

func isSafeWorkspacePath(targetPath, canonWs, canonVault string) (canonTarget string, exists bool, safe bool) {
	if _, err := os.Lstat(targetPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, false
		}
		return "", true, false
	}
	resolved, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		return "", true, false
	}
	resolved = filepath.Clean(resolved)
	if isPathWithinOrEqual(resolved, canonVault) {
		return resolved, true, false
	}
	if !isPathWithinOrEqual(resolved, canonWs) {
		return resolved, true, false
	}
	return resolved, true, true
}

func readSafeWorkspaceFile(lexicalPath, canonWs, canonVault string, warnings *[]string) ([]byte, bool, error) {
	canonFile, exists, safe := isSafeWorkspacePath(lexicalPath, canonWs, canonVault)
	if !exists {
		return nil, false, nil
	}
	if !safe {
		*warnings = append(*warnings, fmt.Sprintf("rejected unsafe symlinked workspace file %s: target escapes workspace or enters vault", lexicalPath))
		return nil, false, nil
	}
	st, err := os.Stat(canonFile)
	if err != nil {
		return nil, false, &fileLoadError{File: lexicalPath, Err: err}
	}
	if st.IsDir() {
		return nil, false, nil
	}
	if !st.Mode().IsRegular() {
		*warnings = append(*warnings, fmt.Sprintf("skipped non-regular workspace file %s", lexicalPath))
		return nil, false, nil
	}
	raw, err := os.ReadFile(canonFile)
	if err != nil {
		return nil, false, &fileLoadError{File: lexicalPath, Err: err}
	}
	return raw, true, nil
}

func isWorkspaceFileTrusted(wsRoot, canonWs, absFile string, raw []byte, trustedHashes map[string]string) bool {
	base := canonWs
	if base == "" {
		base = wsRoot
	}
	if IsWorkspaceFileTrusted(base, absFile, raw, trustedHashes) {
		return true
	}
	if wsRoot != "" && wsRoot != base {
		return IsWorkspaceFileTrusted(wsRoot, absFile, raw, trustedHashes)
	}
	return false
}

func hasCmdSecret(ref string) bool {
	return strings.HasPrefix(strings.TrimSpace(ref), "cmd:")
}

func providerHasCmdSecret(p ProviderConfig) bool {
	if hasCmdSecret(p.APIKeyRef) {
		return true
	}
	for _, v := range p.Headers {
		if hasCmdSecret(v) {
			return true
		}
	}
	return false
}

func mcpHasCmdSecret(s MCPServerConfig) bool {
	for _, v := range s.Env {
		if hasCmdSecret(v) {
			return true
		}
	}
	for _, v := range s.Headers {
		if hasCmdSecret(v) {
			return true
		}
	}
	return false
}

func registerProviderLiteralSecret(resolver *DefaultSecretResolver, p ProviderConfig) {
	ref := strings.TrimSpace(p.APIKeyRef)
	if ref != "" && !isDynamicSecretRef(ref) && p.SourceFile != "" {
		resolver.RegisterLiteralSource(ref, p.SourceFile)
		tightenFilePerm0600(p.SourceFile)
	}
}

func (m *Manager) loadGlobalSnapshot() (*Snapshot, error) {
	globalCfg := defaultGlobalConfig()
	globalCfg.Language = ""
	globalCfg.UILang = ""
	rawConfig, err := os.ReadFile(m.paths.ConfigFile())
	if err != nil {
		return nil, &fileLoadError{File: m.paths.ConfigFile(), Err: err}
	}
	if err := toml.Unmarshal(rawConfig, &globalCfg); err != nil {
		return nil, &fileLoadError{File: m.paths.ConfigFile(), Err: err}
	}
	NormalizeGlobalConfig(&globalCfg, rawConfig)

	rawProviders, err := os.ReadFile(m.paths.ProvidersFile())
	if err != nil {
		return nil, &fileLoadError{File: m.paths.ProvidersFile(), Err: err}
	}
	providers, err := parseProvidersBytes(m.paths.ProvidersFile(), rawProviders)
	if err != nil {
		return nil, err
	}
	for _, p := range providers {
		registerProviderLiteralSecret(m.secrets, p)
	}

	var rules RuleSet
	rawRules, err := os.ReadFile(m.paths.RulesFile())
	if err != nil {
		return nil, &fileLoadError{File: m.paths.RulesFile(), Err: err}
	}
	if err := toml.Unmarshal(rawRules, &rules); err != nil {
		return nil, &fileLoadError{File: m.paths.RulesFile(), Err: err}
	}

	rawMCP, err := os.ReadFile(m.paths.MCPFile())
	if err != nil {
		return nil, &fileLoadError{File: m.paths.MCPFile(), Err: err}
	}
	mcpCfg, err := parseMCPBytes(m.paths.MCPFile(), rawMCP)
	if err != nil {
		return nil, err
	}

	var hooksCfg HooksConfig
	if rawHooks, err := os.ReadFile(m.paths.HooksFile()); err == nil {
		parsedHooks, hErr := parseHooksTOML(rawHooks, m.paths.HooksFile())
		if hErr != nil {
			return nil, &fileLoadError{File: m.paths.HooksFile(), Err: hErr}
		}
		hooksCfg = parsedHooks
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, &fileLoadError{File: m.paths.HooksFile(), Err: err}
	}

	agents, err := loadAgentsDir(m.paths.AgentsDir())
	if err != nil {
		return nil, err
	}

	workflows, err := loadWorkflowsDir(m.paths.WorkflowsDir())
	if err != nil {
		return nil, err
	}

	skillDirs := []string{
		m.paths.SkillsDir(),
		filepath.Join(m.userHomeDir, ".agents", "skills"),
		filepath.Join(m.userHomeDir, ".claude", "skills"),
	}
	skillDirs = append(skillDirs, globalCfg.ExtraSkillDirs...)

	return &Snapshot{
		Paths:     m.paths,
		Global:    globalCfg,
		Providers: providers,
		Agents:    agents,
		Workflows: workflows,
		Rules:     rules,
		MCP:       mcpCfg,
		Hooks:     hooksCfg,
		SkillDirs: skillDirs,
		LoadedAt:  m.now(),
	}, nil
}

// ValidateCandidate parses and validates an in-memory candidate configuration overlay
// (vault-relative or vault-absolute file paths -> raw TOML/MD bytes; nil value simulates deletion)
// without writing to disk or mutating the active snapshot.
func (m *Manager) ValidateCandidate(overlay map[string][]byte) (*Snapshot, error) {
	if m == nil {
		return nil, errors.New("nil config manager")
	}

	canonRoot := filepath.Clean(m.paths.Root)
	if c, err := CanonicalPath(m.paths.Root); err == nil && c != "" {
		canonRoot = c
	}

	normOverlay := make(map[string][]byte, len(overlay))
	for rawKey, content := range overlay {
		trimmed := strings.TrimSpace(rawKey)
		if trimmed == "" {
			return nil, errors.New("empty overlay path")
		}
		var relSlash string
		if filepath.IsAbs(trimmed) {
			cleanKey := filepath.Clean(trimmed)
			if c, err := CanonicalPath(trimmed); err == nil && c != "" {
				cleanKey = c
			}
			var rel string
			var rErr error
			if IsWithinRoot(cleanKey, canonRoot) {
				rel, rErr = filepath.Rel(canonRoot, cleanKey)
			} else if IsWithinRoot(filepath.Clean(trimmed), filepath.Clean(m.paths.Root)) {
				rel, rErr = filepath.Rel(filepath.Clean(m.paths.Root), filepath.Clean(trimmed))
			} else {
				return nil, fmt.Errorf("overlay path %q is outside vault root", rawKey)
			}
			if rErr != nil {
				return nil, fmt.Errorf("resolve overlay path %q: %w", rawKey, rErr)
			}
			relSlash = filepath.ToSlash(rel)
		} else {
			cleaned := filepath.ToSlash(filepath.Clean(trimmed))
			cleaned = strings.TrimPrefix(cleaned, "./")
			if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
				return nil, fmt.Errorf("invalid overlay path %q", rawKey)
			}
			relSlash = cleaned
		}
		if strings.EqualFold(filepath.Ext(relSlash), ".json") {
			return nil, fmt.Errorf("forbidden .json file in vault overlay: %s", relSlash)
		}
		normOverlay[relSlash] = content
	}

	readCandidateFile := func(relSlash, diskPath string) ([]byte, bool, error) {
		if data, overridden := normOverlay[relSlash]; overridden {
			if data == nil {
				return nil, false, nil
			}
			return data, true, nil
		}
		raw, err := os.ReadFile(diskPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, false, nil
			}
			return nil, false, &fileLoadError{File: diskPath, Err: err}
		}
		return raw, true, nil
	}

	globalCfg := defaultGlobalConfig()
	globalCfg.Language = ""
	globalCfg.UILang = ""
	var rawConfigCandidate []byte
	if raw, ok, err := readCandidateFile("config.toml", m.paths.ConfigFile()); err != nil {
		return nil, err
	} else if ok {
		rawConfigCandidate = raw
		if err := toml.Unmarshal(raw, &globalCfg); err != nil {
			return nil, &fileLoadError{File: m.paths.ConfigFile(), Err: err}
		}
	}
	NormalizeGlobalConfig(&globalCfg, rawConfigCandidate)

	providers := make(map[string]ProviderConfig)
	if raw, ok, err := readCandidateFile("providers.toml", m.paths.ProvidersFile()); err != nil {
		return nil, err
	} else if ok {
		provs, pErr := parseProvidersBytes(m.paths.ProvidersFile(), raw)
		if pErr != nil {
			return nil, pErr
		}
		providers = provs
	}

	var rules RuleSet
	if raw, ok, err := readCandidateFile("rules.toml", m.paths.RulesFile()); err != nil {
		return nil, err
	} else if ok {
		if err := toml.Unmarshal(raw, &rules); err != nil {
			return nil, &fileLoadError{File: m.paths.RulesFile(), Err: err}
		}
	}

	mcpCfg := MCPConfig{Servers: make(map[string]MCPServerConfig)}
	if raw, ok, err := readCandidateFile("mcp.toml", m.paths.MCPFile()); err != nil {
		return nil, err
	} else if ok {
		parsedMCP, mErr := parseMCPBytes(m.paths.MCPFile(), raw)
		if mErr != nil {
			return nil, mErr
		}
		mcpCfg = parsedMCP
	}

	var hooksCfg HooksConfig
	if raw, ok, err := readCandidateFile("hooks.toml", m.paths.HooksFile()); err != nil {
		return nil, err
	} else if ok {
		parsedHooks, hErr := parseHooksTOML(raw, m.paths.HooksFile())
		if hErr != nil {
			return nil, &fileLoadError{File: m.paths.HooksFile(), Err: hErr}
		}
		hooksCfg = parsedHooks
	}

	// Collect agents/*.md from disk + overlay
	agentFiles := make(map[string]string)
	if entries, err := os.ReadDir(m.paths.AgentsDir()); err == nil {
		for _, ent := range entries {
			if !ent.IsDir() && strings.HasSuffix(ent.Name(), ".md") {
				agentFiles["agents/"+ent.Name()] = ent.Name()
			}
		}
	}
	for rel := range normOverlay {
		if strings.HasPrefix(rel, "agents/") && strings.HasSuffix(rel, ".md") && strings.Count(rel, "/") == 1 {
			agentFiles[rel] = filepath.Base(rel)
		}
	}
	agentKeys := make([]string, 0, len(agentFiles))
	for rel := range agentFiles {
		agentKeys = append(agentKeys, rel)
	}
	sort.Strings(agentKeys)
	agents := make(map[string]AgentProfile, len(agentKeys))
	for _, rel := range agentKeys {
		name := agentFiles[rel]
		diskPath := filepath.Join(m.paths.AgentsDir(), name)
		raw, ok, err := readCandidateFile(rel, diskPath)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		prof, pErr := parseAgentProfileBytes(diskPath, name, raw)
		if pErr != nil {
			return nil, pErr
		}
		agents[prof.ID] = prof
	}

	// Collect workflows/*.toml from disk + overlay
	wfFiles := make(map[string]string)
	if entries, err := os.ReadDir(m.paths.WorkflowsDir()); err == nil {
		for _, ent := range entries {
			if !ent.IsDir() && strings.HasSuffix(ent.Name(), ".toml") {
				wfFiles["workflows/"+ent.Name()] = ent.Name()
			}
		}
	}
	for rel := range normOverlay {
		if strings.HasPrefix(rel, "workflows/") && strings.HasSuffix(rel, ".toml") && strings.Count(rel, "/") == 1 {
			wfFiles[rel] = filepath.Base(rel)
		}
	}
	wfKeys := make([]string, 0, len(wfFiles))
	for rel := range wfFiles {
		wfKeys = append(wfKeys, rel)
	}
	sort.Strings(wfKeys)
	workflows := make(map[string]WorkflowTemplate, len(wfKeys))
	for _, rel := range wfKeys {
		name := wfFiles[rel]
		diskPath := filepath.Join(m.paths.WorkflowsDir(), name)
		raw, ok, err := readCandidateFile(rel, diskPath)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		wf, wErr := parseWorkflowFile(diskPath, raw)
		if wErr != nil {
			return nil, wErr
		}
		workflows[wf.ID] = wf
	}

	// Validate generic .md frontmatter or .toml syntax for any other overlay files (e.g. skills/*/SKILL.md)
	for rel, raw := range normOverlay {
		if raw == nil {
			continue
		}
		switch rel {
		case "config.toml", "providers.toml", "rules.toml", "mcp.toml", "hooks.toml":
			continue
		}
		if (strings.HasPrefix(rel, "agents/") && strings.HasSuffix(rel, ".md") && strings.Count(rel, "/") == 1) ||
			(strings.HasPrefix(rel, "workflows/") && strings.HasSuffix(rel, ".toml") && strings.Count(rel, "/") == 1) {
			continue
		}
		ext := strings.ToLower(filepath.Ext(rel))
		switch ext {
		case ".toml":
			var generic map[string]any
			if err := toml.Unmarshal(raw, &generic); err != nil {
				return nil, &fileLoadError{File: rel, Err: err}
			}
		case ".md":
			if _, _, err := ParseFrontmatterMD(raw); err != nil {
				return nil, &fileLoadError{File: rel, Err: err}
			}
		}
	}

	skillDirs := []string{
		m.paths.SkillsDir(),
		filepath.Join(m.userHomeDir, ".agents", "skills"),
		filepath.Join(m.userHomeDir, ".claude", "skills"),
	}
	skillDirs = append(skillDirs, globalCfg.ExtraSkillDirs...)

	snap := &Snapshot{
		Paths:     m.paths,
		Global:    globalCfg,
		Providers: providers,
		Agents:    agents,
		Workflows: workflows,
		Rules:     rules,
		MCP:       mcpCfg,
		Hooks:     hooksCfg,
		SkillDirs: skillDirs,
		LoadedAt:  m.now(),
	}

	if err := validateCandidateSemantics(snap); err != nil {
		return nil, err
	}
	return snap, nil
}

func validateCandidateSemantics(snap *Snapshot) error {
	if snap == nil {
		return errors.New("nil candidate snapshot")
	}

	validProtocols := map[string]bool{
		"anthropic":        true,
		"openai-responses": true,
		"openai-compat":    true,
		"gemini":           true,
	}
	for id, prov := range snap.Providers {
		proto := strings.ToLower(strings.TrimSpace(prov.Protocol))
		if !validProtocols[proto] {
			return fmt.Errorf("provider %q has unsupported protocol %q", id, prov.Protocol)
		}
	}

	validateProviderModelRef := func(owner, provID, modelID string) error {
		pID := strings.TrimSpace(provID)
		mID := strings.TrimSpace(modelID)
		if pID == "" {
			return fmt.Errorf("%s: provider must not be empty", owner)
		}
		prov, ok := snap.Providers[pID]
		if !ok {
			return fmt.Errorf("%s: references unknown provider %q", owner, pID)
		}
		if mID == "" {
			return fmt.Errorf("%s: model must not be empty", owner)
		}
		if len(prov.Models) > 0 {
			if _, exists := prov.Models[mID]; !exists {
				return fmt.Errorf("%s: provider %q does not define model %q", owner, pID, mID)
			}
		}
		return nil
	}

	if len(snap.Providers) > 0 {
		if err := validateProviderModelRef("config.toml default", snap.Global.DefaultProvider, snap.Global.DefaultModel); err != nil {
			return err
		}
		for id, prof := range snap.Agents {
			if err := validateProviderModelRef(fmt.Sprintf("agent %q", id), prof.Provider, prof.Model); err != nil {
				return err
			}
			for _, fb := range prof.Fallbacks {
				parts := strings.SplitN(strings.TrimSpace(fb), "/", 2)
				if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
					return fmt.Errorf("agent %q fallback %q must use provider/model format", id, fb)
				}
				if err := validateProviderModelRef(fmt.Sprintf("agent %q fallback", id), parts[0], parts[1]); err != nil {
					return err
				}
			}
		}
	}

	for _, wf := range snap.Workflows {
		if err := validateWorkflowDAG(wf, snap.Agents, snap.Providers); err != nil {
			return err
		}
	}
	return nil
}

func validateWorkflowDAG(wf WorkflowTemplate, agents map[string]AgentProfile, providers map[string]ProviderConfig) error {
	for role, modelRef := range wf.RoleModels {
		if len(agents) > 0 {
			if _, ok := agents[role]; !ok {
				return fmt.Errorf("workflow %q role_models references unknown agent role %q", wf.ID, role)
			}
		}
		if len(providers) > 0 && strings.Contains(modelRef, "/") {
			parts := strings.SplitN(strings.TrimSpace(modelRef), "/", 2)
			prov, ok := providers[parts[0]]
			if !ok {
				return fmt.Errorf("workflow %q role_models[%q] references unknown provider %q", wf.ID, role, parts[0])
			}
			if len(prov.Models) > 0 {
				if _, mOK := prov.Models[parts[1]]; !mOK {
					return fmt.Errorf("workflow %q role_models[%q] references unknown model %q on provider %q", wf.ID, role, parts[1], parts[0])
				}
			}
		}
	}

	if len(wf.Nodes) == 0 && len(wf.Edges) == 0 {
		return nil
	}

	nodeSet := make(map[string]bool, len(wf.Nodes))
	for _, n := range wf.Nodes {
		nID := strings.TrimSpace(n.ID)
		if nID == "" {
			return fmt.Errorf("workflow %q contains a node with empty id", wf.ID)
		}
		if nodeSet[nID] {
			return fmt.Errorf("workflow %q contains duplicate node id %q", wf.ID, nID)
		}
		nodeSet[nID] = true
		if r := strings.TrimSpace(n.Role); r != "" && len(agents) > 0 {
			if _, ok := agents[r]; !ok {
				return fmt.Errorf("workflow %q node %q references unknown agent role %q", wf.ID, nID, r)
			}
		}
	}

	allowFeedbackEdges := wf.Topology == "review-loop" || wf.Topology == "event-pipeline"
	adj := make(map[string][]string, len(nodeSet))
	for _, e := range wf.Edges {
		from := strings.TrimSpace(e.From)
		to := strings.TrimSpace(e.To)
		if from == "" || to == "" {
			return fmt.Errorf("workflow %q has an edge with empty endpoint (%q -> %q)", wf.ID, e.From, e.To)
		}
		if !nodeSet[from] {
			return fmt.Errorf("workflow %q edge references unknown source node %q", wf.ID, from)
		}
		if !nodeSet[to] {
			return fmt.Errorf("workflow %q edge references unknown target node %q", wf.ID, to)
		}
		if from == to {
			return fmt.Errorf("workflow %q contains self-loop on node %q", wf.ID, from)
		}
		if allowFeedbackEdges && strings.EqualFold(strings.TrimSpace(e.Kind), "feedback") {
			continue
		}
		adj[from] = append(adj[from], to)
	}

	const (
		unvisited = 0
		visiting  = 1
		visited   = 2
	)
	color := make(map[string]int, len(nodeSet))
	var dfs func(u string) error
	dfs = func(u string) error {
		color[u] = visiting
		for _, v := range adj[u] {
			if color[v] == visiting {
				return fmt.Errorf("workflow %q graph contains a cycle (%s -> %s)", wf.ID, u, v)
			}
			if color[v] == unvisited {
				if err := dfs(v); err != nil {
					return err
				}
			}
		}
		color[u] = visited
		return nil
	}

	sortedNodes := make([]string, 0, len(nodeSet))
	for id := range nodeSet {
		sortedNodes = append(sortedNodes, id)
	}
	sort.Strings(sortedNodes)
	for _, id := range sortedNodes {
		if color[id] == unvisited {
			if err := dfs(id); err != nil {
				return err
			}
		}
	}
	return nil
}

type providersDoc struct {
	Providers map[string]ProviderConfig `toml:"providers"`
}

func parseProvidersBytes(filePath string, raw []byte) (map[string]ProviderConfig, error) {
	var doc providersDoc
	if err := toml.Unmarshal(raw, &doc); err != nil {
		return nil, &fileLoadError{File: filePath, Err: err}
	}
	out := make(map[string]ProviderConfig, len(doc.Providers))
	for id, p := range doc.Providers {
		if p.ID == "" {
			p.ID = id
		}
		p.SourceFile = filePath
		if p.Models != nil {
			models := make(map[string]ModelSpec, len(p.Models))
			for mID, spec := range p.Models {
				if spec.ID == "" {
					spec.ID = mID
				}
				models[mID] = spec
			}
			p.Models = models
		}
		out[id] = p
	}
	return out, nil
}

func parseMCPBytes(filePath string, raw []byte) (MCPConfig, error) {
	var cfg MCPConfig
	if err := toml.Unmarshal(raw, &cfg); err != nil {
		return MCPConfig{}, &fileLoadError{File: filePath, Err: err}
	}
	if cfg.Servers == nil {
		cfg.Servers = make(map[string]MCPServerConfig)
	}
	for id, s := range cfg.Servers {
		if s.ID == "" {
			s.ID = id
		}
		s.SourceFile = filePath
		cfg.Servers[id] = s
	}
	return cfg, nil
}

func loadAgentsDir(dir string) (map[string]AgentProfile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, &fileLoadError{File: dir, Err: err}
	}
	names := make([]string, 0, len(entries))
	for _, ent := range entries {
		if !ent.IsDir() && strings.HasSuffix(ent.Name(), ".md") {
			names = append(names, ent.Name())
		}
	}
	sort.Strings(names)

	agents := make(map[string]AgentProfile, len(names))
	for _, name := range names {
		p := filepath.Join(dir, name)
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, &fileLoadError{File: p, Err: err}
		}
		prof, err := parseAgentProfileBytes(p, name, raw)
		if err != nil {
			return nil, err
		}
		agents[prof.ID] = prof
	}
	return agents, nil
}

func parseAgentProfileBytes(filePath, fileName string, raw []byte) (AgentProfile, error) {
	meta, body, err := ParseFrontmatterMD(raw)
	if err != nil {
		return AgentProfile{}, &fileLoadError{File: filePath, Err: err}
	}
	var prof AgentProfile
	if len(meta) > 0 {
		encoded, err := toml.Marshal(meta)
		if err != nil {
			return AgentProfile{}, &fileLoadError{File: filePath, Err: err}
		}
		if err := toml.Unmarshal(encoded, &prof); err != nil {
			return AgentProfile{}, &fileLoadError{File: filePath, Err: err}
		}
		// 显式保留空切片 []string{} 与未设置 nil 的语义区别（如 utility 角色 allowed_tools = [] 表示零工具权限）。
		if v, exists := meta["allowed_tools"]; exists && v != nil && prof.AllowedTools == nil {
			prof.AllowedTools = []string{}
		}
		if v, exists := meta["disallowed_tools"]; exists && v != nil && prof.DisallowedTools == nil {
			prof.DisallowedTools = []string{}
		}
		if v, exists := meta["fallbacks"]; exists && v != nil && prof.Fallbacks == nil {
			prof.Fallbacks = []string{}
		}
	}
	stem := strings.TrimSuffix(fileName, ".md")
	if prof.ID == "" {
		prof.ID = stem
	}
	if prof.Name == "" {
		prof.Name = prof.ID
	}
	prof.SystemPrompt = strings.TrimSpace(body)
	prof.SourceFile = filePath
	return prof, nil
}

func loadWorkflowsDir(dir string) (map[string]WorkflowTemplate, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, &fileLoadError{File: dir, Err: err}
	}
	workflows := make(map[string]WorkflowTemplate)
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".toml") {
			continue
		}
		p := filepath.Join(dir, ent.Name())
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, &fileLoadError{File: p, Err: err}
		}
		wf, err := parseWorkflowFile(p, raw)
		if err != nil {
			return nil, err
		}
		workflows[wf.ID] = wf
	}
	return workflows, nil
}

func parseWorkflowFile(filePath string, raw []byte) (WorkflowTemplate, error) {
	var wf WorkflowTemplate
	if err := toml.Unmarshal(raw, &wf); err != nil {
		return WorkflowTemplate{}, &fileLoadError{File: filePath, Err: err}
	}
	stem := strings.TrimSuffix(filepath.Base(filePath), ".toml")
	if wf.ID == "" {
		wf.ID = stem
	}
	if wf.Topology == "" {
		wf.Topology = wf.ID
	}
	wf.SourceFile = filePath
	return wf, nil
}

func bootstrapVault(paths VaultPaths) error {
	dirs := []string{
		paths.Root,
		paths.AgentsDir(),
		paths.WorkflowsDir(),
		paths.SkillsDir(),
		paths.MemoryDir(),
		filepath.Join(paths.MemoryDir(), "notes"),
		paths.AlwaysOnDir(),
		filepath.Join(paths.AlwaysOnDir(), "deferred"),
		paths.WorkspacesDir(),
		paths.RuntimeDir(),
		paths.SnapshotsDir(),
		paths.WorktreesDir(),
		paths.LogsDir(),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create vault dir %s: %w", dir, err)
		}
	}
	if err := os.Chmod(paths.Root, 0o700); err != nil {
		return fmt.Errorf("chmod vault root %s: %w", paths.Root, err)
	}

	files := map[string]struct {
		perm os.FileMode
		body string
	}{
		paths.ConfigFile():    {0o600, defaultConfigTOML},
		paths.ProvidersFile(): {0o600, defaultProvidersTOML},
		paths.RulesFile():     {0o600, defaultRulesTOML},
		paths.MCPFile():       {0o600, defaultMCPTOML},
		paths.HooksFile():     {0o600, defaultHooksTOML},
		filepath.Join(paths.MemoryDir(), "INSTRUCTIONS.md"): {0o600, defaultMemoryInstructionsMD},
		filepath.Join(paths.MemoryDir(), "INDEX.md"):        {0o600, defaultMemoryIndexMD},
		filepath.Join(paths.MemoryDir(), "REPORTED.md"):     {0o600, defaultMemoryReportedMD},
	}

	for id, body := range defaultAgentsMD {
		files[filepath.Join(paths.AgentsDir(), id+".md")] = struct {
			perm os.FileMode
			body string
		}{0o600, body}
	}
	for id, body := range defaultWorkflowsTOML {
		files[filepath.Join(paths.WorkflowsDir(), id+".toml")] = struct {
			perm os.FileMode
			body string
		}{0o600, body}
	}

	for filePath, spec := range files {
		if err := writeIfMissing(filePath, []byte(spec.body), spec.perm); err != nil {
			return err
		}
	}
	return nil
}

// writeIfMissing 使用 O_CREATE|O_EXCL 保证幂等初始化，绝不覆盖用户已编辑过的配置文件。
func writeIfMissing(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return fmt.Errorf("create default file %s: %w", path, err)
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		return fmt.Errorf("write default file %s: %w", path, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close default file %s: %w", path, closeErr)
	}
	return nil
}

func defaultGlobalConfig() GlobalConfig {
	return GlobalConfig{
		Host:                        "127.0.0.1",
		Port:                        3456,
		Theme:                       "auto",
		Language:                    "zh",
		UILang:                      "zh",
		DefaultProvider:             "anthropic",
		DefaultModel:                "claude-sonnet-5-5",
		DefaultEffort:               "high",
		PermissionMode:              "auto",
		SandboxMode:                 "strict",
		MaxConcurrency:              4,
		MaxToolConcurrency:          4,
		MaxSteps:                    64,
		SubagentMaxSteps:            8,
		AutoCompactThresholdPercent: 85,
		AutoCompactRatio:            0.85,
		KeptRecentTurnsRatio:        0.30,
		CommandTimeoutSeconds:       120,
		CommandTimeoutSec:           120,
		AutoGenerateTitle:           true,
		AutoDream:                   true,
		DreamerIdleMinutes:          15,
		DreamIdleMinutes:            15,
		WitnessTimeoutSeconds:       180,
		WitnessStallSec:             180,
		LLMMaxRetries:               2,
		LLMTimeoutSec:               120,
		InboxDebounceMS:             400,
	}
}

func normalizeGlobalLanguage(cfg *GlobalConfig) {
	if cfg == nil {
		return
	}
	uiLang := strings.ToLower(strings.TrimSpace(cfg.UILang))
	lang := strings.ToLower(strings.TrimSpace(cfg.Language))
	if uiLang == "en" || (uiLang == "" && lang == "en") {
		cfg.UILang = "en"
	} else {
		cfg.UILang = "zh"
	}
	if lang == "en" {
		cfg.Language = "en"
	} else {
		cfg.Language = cfg.UILang
	}
}

func clampIntRange(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func clampFloatRange(v, minV, maxV float64) float64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

// NormalizeGlobalConfig applies defaults, alias synchronization, and safe range clamping to GlobalConfig.
func NormalizeGlobalConfig(cfg *GlobalConfig, rawTOML []byte) {
	if cfg == nil {
		return
	}
	normalizeGlobalLanguage(cfg)

	var rawMap map[string]any
	if len(rawTOML) > 0 {
		_ = toml.Unmarshal(rawTOML, &rawMap)
	}
	hasRawKey := func(k string) bool {
		if rawMap == nil {
			return false
		}
		_, ok := rawMap[k]
		return ok
	}

	if cfg.MaxConcurrency <= 0 {
		cfg.MaxConcurrency = 4
	}
	cfg.MaxConcurrency = clampIntRange(cfg.MaxConcurrency, 1, 32)

	if cfg.MaxToolConcurrency <= 0 {
		cfg.MaxToolConcurrency = 4
	}
	cfg.MaxToolConcurrency = clampIntRange(cfg.MaxToolConcurrency, 1, 16)

	if cfg.MaxSteps <= 0 {
		cfg.MaxSteps = 64
	}
	cfg.MaxSteps = clampIntRange(cfg.MaxSteps, 4, 256)

	if cfg.SubagentMaxSteps <= 0 {
		cfg.SubagentMaxSteps = 8
	}
	cfg.SubagentMaxSteps = clampIntRange(cfg.SubagentMaxSteps, 2, 32)

	// Auto-compact threshold percent (50..95 or 0 to disable) & ratio (0.50..0.95 or 0 to disable)
	switch {
	case hasRawKey("auto_compact_threshold_percent"):
		if cfg.AutoCompactThresholdPercent <= 0 {
			cfg.AutoCompactThresholdPercent = 0
			cfg.AutoCompactRatio = 0
		} else {
			cfg.AutoCompactThresholdPercent = clampIntRange(cfg.AutoCompactThresholdPercent, 50, 95)
			cfg.AutoCompactRatio = float64(cfg.AutoCompactThresholdPercent) / 100.0
		}
	case hasRawKey("auto_compact_ratio"):
		if cfg.AutoCompactRatio <= 0 {
			cfg.AutoCompactThresholdPercent = 0
			cfg.AutoCompactRatio = 0
		} else {
			r := cfg.AutoCompactRatio
			if r > 1.0 {
				r = r / 100.0
			}
			r = clampFloatRange(r, 0.50, 0.95)
			cfg.AutoCompactRatio = r
			cfg.AutoCompactThresholdPercent = int(math.Round(r * 100))
		}
	default:
		if cfg.AutoCompactThresholdPercent <= 0 && cfg.AutoCompactRatio <= 0 {
			if rawMap != nil {
				cfg.AutoCompactThresholdPercent = 85
				cfg.AutoCompactRatio = 0.85
			} else {
				cfg.AutoCompactThresholdPercent = 0
				cfg.AutoCompactRatio = 0
			}
		} else if cfg.AutoCompactThresholdPercent > 0 && cfg.AutoCompactThresholdPercent != 85 {
			cfg.AutoCompactThresholdPercent = clampIntRange(cfg.AutoCompactThresholdPercent, 50, 95)
			cfg.AutoCompactRatio = float64(cfg.AutoCompactThresholdPercent) / 100.0
		} else if cfg.AutoCompactRatio > 0 && cfg.AutoCompactRatio != 0.85 {
			r := cfg.AutoCompactRatio
			if r > 1.0 {
				r = r / 100.0
			}
			r = clampFloatRange(r, 0.50, 0.95)
			cfg.AutoCompactRatio = r
			cfg.AutoCompactThresholdPercent = int(math.Round(r * 100))
		} else {
			cfg.AutoCompactThresholdPercent = clampIntRange(cfg.AutoCompactThresholdPercent, 50, 95)
			cfg.AutoCompactRatio = float64(cfg.AutoCompactThresholdPercent) / 100.0
		}
	}

	if cfg.KeptRecentTurnsRatio <= 0 {
		cfg.KeptRecentTurnsRatio = 0.30
	} else if cfg.KeptRecentTurnsRatio > 1.0 {
		cfg.KeptRecentTurnsRatio = cfg.KeptRecentTurnsRatio / 100.0
	}
	cfg.KeptRecentTurnsRatio = clampFloatRange(cfg.KeptRecentTurnsRatio, 0.10, 0.50)

	// Command timeout seconds (10..1800, default 120)
	cmdTimeout := cfg.CommandTimeoutSeconds
	if hasRawKey("command_timeout_seconds") {
		cmdTimeout = cfg.CommandTimeoutSeconds
	} else if hasRawKey("command_timeout_sec") {
		cmdTimeout = cfg.CommandTimeoutSec
	} else if cfg.CommandTimeoutSeconds > 0 && cfg.CommandTimeoutSeconds != 120 {
		cmdTimeout = cfg.CommandTimeoutSeconds
	} else if cfg.CommandTimeoutSec > 0 {
		cmdTimeout = cfg.CommandTimeoutSec
	}
	if cmdTimeout <= 0 {
		cmdTimeout = 120
	}
	cmdTimeout = clampIntRange(cmdTimeout, 10, 1800)
	cfg.CommandTimeoutSeconds = cmdTimeout
	cfg.CommandTimeoutSec = cmdTimeout

	if rawMap != nil && !hasRawKey("auto_generate_title") {
		cfg.AutoGenerateTitle = true
	}
	if rawMap != nil && !hasRawKey("auto_dream") {
		cfg.AutoDream = true
	}

	// Dreamer idle minutes (0..240, default 15; 0 disables idle trigger)
	switch {
	case hasRawKey("dreamer_idle_minutes"):
		if cfg.DreamerIdleMinutes <= 0 {
			cfg.DreamerIdleMinutes = 0
			cfg.DreamIdleMinutes = 0
		} else {
			v := clampIntRange(cfg.DreamerIdleMinutes, 1, 240)
			cfg.DreamerIdleMinutes = v
			cfg.DreamIdleMinutes = v
		}
	case hasRawKey("dream_idle_minutes"):
		if cfg.DreamIdleMinutes <= 0 {
			cfg.DreamerIdleMinutes = 0
			cfg.DreamIdleMinutes = 0
		} else {
			v := clampIntRange(cfg.DreamIdleMinutes, 1, 240)
			cfg.DreamerIdleMinutes = v
			cfg.DreamIdleMinutes = v
		}
	default:
		if cfg.DreamerIdleMinutes <= 0 && cfg.DreamIdleMinutes <= 0 {
			if rawMap != nil {
				cfg.DreamerIdleMinutes = 15
				cfg.DreamIdleMinutes = 15
			} else {
				cfg.DreamerIdleMinutes = 0
				cfg.DreamIdleMinutes = 0
			}
		} else {
			v := cfg.DreamerIdleMinutes
			if v <= 0 || (v == 15 && cfg.DreamIdleMinutes > 0 && cfg.DreamIdleMinutes != 15) {
				v = cfg.DreamIdleMinutes
			}
			v = clampIntRange(v, 1, 240)
			cfg.DreamerIdleMinutes = v
			cfg.DreamIdleMinutes = v
		}
	}

	// Witness stall timeout seconds (30..1800, default 180)
	witnessSec := cfg.WitnessTimeoutSeconds
	if hasRawKey("witness_timeout_seconds") {
		witnessSec = cfg.WitnessTimeoutSeconds
	} else if hasRawKey("witness_stall_sec") {
		witnessSec = cfg.WitnessStallSec
	} else if cfg.WitnessTimeoutSeconds > 0 && cfg.WitnessTimeoutSeconds != 180 {
		witnessSec = cfg.WitnessTimeoutSeconds
	} else if cfg.WitnessStallSec > 0 {
		witnessSec = cfg.WitnessStallSec
	}
	if witnessSec <= 0 {
		witnessSec = 180
	}
	witnessSec = clampIntRange(witnessSec, 30, 1800)
	cfg.WitnessTimeoutSeconds = witnessSec
	cfg.WitnessStallSec = witnessSec

	// LLM max retries (0..5, default 2) & timeout sec (15..600, default 120)
	if rawMap != nil && !hasRawKey("llm_max_retries") {
		cfg.LLMMaxRetries = 2
	} else {
		cfg.LLMMaxRetries = clampIntRange(cfg.LLMMaxRetries, 0, 5)
	}
	if cfg.LLMTimeoutSec <= 0 {
		cfg.LLMTimeoutSec = 120
	}
	cfg.LLMTimeoutSec = clampIntRange(cfg.LLMTimeoutSec, 15, 600)

	if cfg.InboxDebounceMS <= 0 {
		cfg.InboxDebounceMS = 400
	}
	cfg.InboxDebounceMS = clampIntRange(cfg.InboxDebounceMS, 50, 10000)
}

const defaultConfigTOML = `# Gorness Global Configuration (全局主配置 / Global Settings)
host = "127.0.0.1"
port = 3456
theme = "auto"                # auto | light | dark
language = "zh"               # zh | en
ui_lang = "zh"                # zh | en
default_provider = "anthropic"
default_model = "claude-sonnet-5-5"
default_effort = "high"       # low | medium | high | xhigh | max
permission_mode = "auto"      # plan | ask | accept_edits | auto | yolo
sandbox_mode = "strict"       # strict | readonly | network_allow | off
max_concurrency = 4
max_tool_concurrency = 4
max_steps = 64
subagent_max_steps = 8
auto_compact_threshold_percent = 85
auto_compact_ratio = 0.85
kept_recent_turns_ratio = 0.30
command_timeout_seconds = 120
command_timeout_sec = 120
auto_generate_title = true
auto_dream = true
dreamer_idle_minutes = 15
dream_idle_minutes = 15
witness_timeout_seconds = 180
witness_stall_sec = 180
llm_max_retries = 2
llm_timeout_sec = 120
inbox_debounce_ms = 400
`

const defaultProvidersTOML = `# Gorness Multi-Provider Catalog (多模型供应商目录)
# Supported protocols: anthropic | openai-responses | openai-compat | gemini
# Supported api_key refs: keychain:<service> | env:<VAR> | cmd:<shell>

[providers.anthropic]
name = "Anthropic"
protocol = "anthropic"
base_url = "https://api.anthropic.com"
api_key = "env:ANTHROPIC_API_KEY"
timeout_sec = 120

[providers.anthropic.models."claude-opus-5-5"]
name = "Claude Opus 5.5"
context_window = 200000
max_output_tokens = 32000
input_cost_per_1m = 15.0
output_cost_per_1m = 75.0
cache_read_cost_per_1m = 1.5
supports_thinking = true
supports_caching = true

[providers.anthropic.models."claude-sonnet-5-5"]
name = "Claude Sonnet 5.5"
context_window = 200000
max_output_tokens = 16000
input_cost_per_1m = 3.0
output_cost_per_1m = 15.0
cache_read_cost_per_1m = 0.3
supports_thinking = true
supports_caching = true

[providers.anthropic.models."claude-haiku-5-5"]
name = "Claude Haiku 5.5"
context_window = 200000
max_output_tokens = 8192
input_cost_per_1m = 0.8
output_cost_per_1m = 4.0
cache_read_cost_per_1m = 0.08
supports_thinking = false
supports_caching = true

[providers.openai]
name = "OpenAI"
protocol = "openai-responses"
base_url = "https://api.openai.com/v1"
api_key = "env:OPENAI_API_KEY"
timeout_sec = 120

[providers.openai.models."gpt-5.5"]
name = "GPT-5.5"
context_window = 256000
max_output_tokens = 32000
input_cost_per_1m = 2.5
output_cost_per_1m = 10.0
supports_thinking = true

[providers.gemini]
name = "Google Gemini"
protocol = "gemini"
base_url = "https://generativelanguage.googleapis.com/v1beta"
api_key = "env:GEMINI_API_KEY"
timeout_sec = 120

[providers.gemini.models."gemini-2.5-pro"]
name = "Gemini 2.5 Pro"
context_window = 1000000
max_output_tokens = 16384
input_cost_per_1m = 1.25
output_cost_per_1m = 10.0
supports_thinking = true

[providers.local_mlx]
name = "Local MLX / Ollama"
protocol = "openai-compat"
base_url = "http://127.0.0.1:8080/v1"
api_key = "env:MLX_API_KEY"
timeout_sec = 60

[providers.local_mlx.models."qwen3-coder"]
name = "Qwen3 Coder Local"
context_window = 131072
max_output_tokens = 8192
input_cost_per_1m = 0.0
output_cost_per_1m = 0.0
`

const defaultRulesTOML = `# Gorness Fine-Grained Permission & Egress Rules (权限规则与域名白名单)
allow_commands = ["go test*", "go vet*", "git status*", "git diff*", "git log*"]
ask_commands = ["git push*", "git reset --hard*", "curl *", "rm -rf *"]
deny_commands = ["rm -rf /", "rm -rf /*", "mkfs*", "dd if=* of=/dev/*"]
deny_paths = ["~/.gorness/providers.toml", "~/.ssh/*"]
allowed_domains = ["api.anthropic.com", "api.openai.com", "generativelanguage.googleapis.com", "proxy.golang.org", "sum.golang.org", "github.com"]
`

const defaultMCPTOML = `# Gorness Global MCP Server Declarations (全局 MCP 服务器声明)
# Example:
# [servers.filesystem]
# transport = "stdio"
# command = "npx"
# args = ["-y", "@modelcontextprotocol/server-filesystem", "."]

[servers]
`

const defaultMemoryInstructionsMD = `# Persistent Instructions (长期行为指令 / Always-On Static Prefix)

- Keep changes minimal, Idiomatic Go, and race-free.
- Verify behavior with automated tests before reporting completion.
`

const defaultMemoryIndexMD = `# Memory Index (全局记忆索引)

Pointers to durable engineering notes stored under ` + "`notes/*.md`" + `.
`

const defaultMemoryReportedMD = `# Reported Findings Ledger (已播报事项账本)

Tracks delivered Always-On alerts to suppress duplicate notifications.
`

var defaultAgentsMD = map[string]string{
	"lead": `+++
id = "lead"
name = "Lead Architect"
description = "Plans multi-step architecture, decomposes fleet missions, and coordinates specialists."
provider = "anthropic"
model = "claude-opus-5-5"
effort = "xhigh"
fallbacks = ["anthropic/claude-sonnet-5-5", "openai/gpt-5.5"]
+++
You are the Lead Architect agent in Gorness. Analyze repository context carefully, decompose complex goals into verifiable tasks, maintain architectural coherence, and synthesize specialist outputs accurately.
`,
	"coder": `+++
id = "coder"
name = "Software Engineer"
description = "Implements clean, tested production code changes within workspace boundaries."
provider = "anthropic"
model = "claude-sonnet-5-5"
effort = "high"
fallbacks = ["openai/gpt-5.5"]
+++
You are the primary Software Engineer agent in Gorness. Write concise, idiomatic, race-free code, preserve surrounding style, and verify every change with targeted tests.
`,
	"verifier": `+++
id = "verifier"
name = "Adversarial Verifier"
description = "Performs independent cross-model blind review and test verification."
provider = "openai"
model = "gpt-5.5"
effort = "high"
fallbacks = ["gemini/gemini-2.5-pro", "anthropic/claude-sonnet-5-5"]
read_only = true
allowed_tools = ["read_file", "search_code", "run_command"]
+++
You are the Adversarial Verifier agent in Gorness. Independently audit diffs, edge cases, security boundaries, and test coverage without trusting implementation claims at face value.
`,
	"explorer": `+++
id = "explorer"
name = "Fast Explorer"
description = "Conducts rapid read-only codebase reconnaissance and symbol discovery."
provider = "anthropic"
model = "claude-haiku-5-5"
effort = "low"
fallbacks = ["local_mlx/qwen3-coder"]
read_only = true
allowed_tools = ["read_file", "search_code"]
+++
You are the Fast Explorer agent in Gorness. Quickly locate relevant files, call paths, symbols, and invariants, returning crisp file-and-line grounded findings.
`,
	"debugger": `+++
id = "debugger"
name = "Root-Cause Debugger"
description = "Investigates failures, data races, deadlocks, and subtle regressions."
provider = "anthropic"
model = "claude-opus-5-5"
effort = "max"
fallbacks = ["openai/gpt-5.5"]
+++
You are the Root-Cause Debugger agent in Gorness. Formulate falsifiable hypotheses, reproduce failures first, trace state transitions systematically, and fix root causes rather than symptoms.
`,
	"dreamer": `+++
id = "dreamer"
name = "Memory & Skill Dreamer"
description = "Consolidates session lessons into pointer memory and candidate SKILL.md files during idle periods."
provider = "anthropic"
model = "claude-sonnet-5-5"
effort = "medium"
+++
You are the Dreamer agent in Gorness. Distill recurring workflows, debugging lessons, and project invariants into concise pointer notes and reusable candidate SKILL.md definitions.
`,
	"utility": `+++
id = "utility"
name = "Housekeeping Utility"
description = "Generates concise session titles and anchored incremental compaction summaries."
provider = "anthropic"
model = "claude-haiku-5-5"
effort = "low"
read_only = true
allowed_tools = []
+++
You are the Housekeeping Utility agent in Gorness. Produce crisp session titles and faithful anchored summaries preserving exact file paths, decisions, and unresolved tasks.
`,
}

var defaultWorkflowsTOML = map[string]string{
	"hybrid-fleet": `# Flagship Hybrid Fleet Topology (默认旗舰混合舰队)
id = "hybrid-fleet"
name = "Hybrid Fleet"
description = "Lead architect fans out explorers, dispatches isolated worktree coders, and gates landing via adversarial verifier."
topology = "hybrid-fleet"
max_concurrency = 4
use_worktrees = true
verify_command = "go test ./..."
max_rounds = 3

[[nodes]]
id = "lead"
label = "Lead Architect"
role = "lead"

[[nodes]]
id = "explorer"
label = "Code Explorer"
role = "explorer"

[[nodes]]
id = "coder"
label = "Worktree Coder"
role = "coder"
worktree = true

[[nodes]]
id = "verifier"
label = "Blind Verifier"
role = "verifier"

[[edges]]
from = "lead"
to = "explorer"
label = "recon"
kind = "fanout"

[[edges]]
from = "lead"
to = "coder"
label = "implement"
kind = "dispatch"

[[edges]]
from = "coder"
to = "verifier"
label = "audit"
kind = "gate"
`,
	"review-loop": `# Generator-Verifier Adversarial Loop (生成-验证对抗闭环)
id = "review-loop"
name = "Review Loop"
description = "Coder implements and heterogeneous Verifier critiques iteratively until checks and blind review pass."
topology = "review-loop"
max_concurrency = 2
use_worktrees = true
verify_command = "go test ./..."
max_rounds = 3

[[nodes]]
id = "coder"
label = "Generator Coder"
role = "coder"
worktree = true

[[nodes]]
id = "verifier"
label = "Adversarial Verifier"
role = "verifier"

[[edges]]
from = "coder"
to = "verifier"
label = "submit diff"
kind = "review"

[[edges]]
from = "verifier"
to = "coder"
label = "feedback"
kind = "feedback"
`,
	"parallel-explore": `# Cache-Shared Parallel Exploration (主从缓存共享并发探索)
id = "parallel-explore"
name = "Parallel Explore"
description = "Fans out concurrent read-only explorers across subsystems and synthesizes findings via Lead."
topology = "parallel-explore"
max_concurrency = 4
use_worktrees = false

[[nodes]]
id = "lead"
label = "Synthesis Lead"
role = "lead"

[[nodes]]
id = "explorer_1"
label = "Explorer A"
role = "explorer"

[[nodes]]
id = "explorer_2"
label = "Explorer B"
role = "explorer"

[[edges]]
from = "lead"
to = "explorer_1"
label = "partition A"
kind = "fanout"

[[edges]]
from = "lead"
to = "explorer_2"
label = "partition B"
kind = "fanout"
`,
	"worktree-team": `# Isolated Git Worktree Specialist Team (独立 Git Worktree 常驻专家战队)
id = "worktree-team"
name = "Worktree Team"
description = "Parallel coders work in isolated Git worktrees and land changes serially through Refinery rebase and test gates."
topology = "worktree-team"
max_concurrency = 3
use_worktrees = true
verify_command = "go test ./..."

[[nodes]]
id = "coder_1"
label = "Teammate 1"
role = "coder"
worktree = true

[[nodes]]
id = "coder_2"
label = "Teammate 2"
role = "coder"
worktree = true

[[nodes]]
id = "verifier"
label = "Refinery Verifier"
role = "verifier"

[[edges]]
from = "coder_1"
to = "verifier"
label = "rebase & test"
kind = "merge_queue"

[[edges]]
from = "coder_2"
to = "verifier"
label = "rebase & test"
kind = "merge_queue"
`,
	"event-pipeline": `# Typed Event-Driven Pipeline (强类型事件驱动流水线)
id = "event-pipeline"
name = "Event Pipeline"
description = "Deterministic stage pipeline from reconnaissance to implementation, debugging, and verification with hop guard."
topology = "event-pipeline"
max_concurrency = 2
use_worktrees = true
max_hops = 8
verify_command = "go test ./..."

[[nodes]]
id = "explorer"
label = "Recon Stage"
role = "explorer"

[[nodes]]
id = "coder"
label = "Build Stage"
role = "coder"
worktree = true

[[nodes]]
id = "debugger"
label = "Debug Stage"
role = "debugger"
worktree = true

[[nodes]]
id = "verifier"
label = "Gate Stage"
role = "verifier"

[[edges]]
from = "explorer"
to = "coder"
label = "context_ready"
kind = "event"

[[edges]]
from = "coder"
to = "verifier"
label = "patch_ready"
kind = "event"

[[edges]]
from = "verifier"
to = "debugger"
label = "check_failed"
kind = "event"
`,
	"shared-board": `# Markdown Shared Blackboard Swarm (共享黑板动态拆树协同)
id = "shared-board"
name = "Shared Board"
description = "Peer agents atomically claim Markdown task cards via Flock, split complex tasks up to depth 3, and converge cleanly."
topology = "shared-board"
max_concurrency = 4
use_worktrees = true
max_idle_rounds = 3
verify_command = "go test ./..."

[[nodes]]
id = "lead"
label = "Board Seed"
role = "lead"

[[nodes]]
id = "worker_1"
label = "Board Worker 1"
role = "coder"
worktree = true

[[nodes]]
id = "worker_2"
label = "Board Worker 2"
role = "coder"
worktree = true

[[nodes]]
id = "verifier"
label = "Board Verifier"
role = "verifier"

[[edges]]
from = "lead"
to = "worker_1"
label = "tasks/*.md"
kind = "blackboard"

[[edges]]
from = "lead"
to = "worker_2"
label = "tasks/*.md"
kind = "blackboard"

[[edges]]
from = "worker_1"
to = "verifier"
label = "land"
kind = "merge_queue"

[[edges]]
from = "worker_2"
to = "verifier"
label = "land"
kind = "merge_queue"
`,
}
