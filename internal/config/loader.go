package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type Options struct {
	VaultRoot     string
	UserHomeDir   string
	KeychainFunc  KeychainFunc
	KeychainStore KeychainStore
	EnvLookup     func(key string) (string, bool)
	CmdRunner     func(ctx context.Context, shellCmd string) (string, error)
	CmdTTL        time.Duration
	Now           func() time.Time
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

func (m *Manager) Current() *Snapshot {
	return m.current.Load()
}

// ForWorkspace 基于当前全局快照合并项目 <wsRoot>/.gorness/ 局部配置：
// 1. 项目级 mcp.toml 与 providers.toml 必须通过 SHA-256 白名单核验方可激活；
// 2. 项目级覆盖中的任何 cmd: 动态命令凭据一律无条件拦截（即使文件哈希在 trustedHashes 中也拒绝执行）。
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

	// 用户级技能目录排在前面，确保同名用户技能优先覆盖项目内技能。
	snap.SkillDirs = append(snap.SkillDirs,
		filepath.Join(wsRoot, ".gorness", "skills"),
		filepath.Join(wsRoot, ".agents", "skills"),
		filepath.Join(wsRoot, ".claude", "skills"),
	)

	wsGorness := filepath.Join(wsRoot, ".gorness")
	st, err := os.Stat(wsGorness)
	if err != nil || !st.IsDir() {
		return snap, nil, nil
	}

	// 0. config.toml 安全偏好覆盖（仅允许覆盖模型/推理强度/界面偏好，不允许项目配置擅自放宽沙箱或权限级别）
	wsConfigPath := filepath.Join(wsGorness, "config.toml")
	if raw, err := os.ReadFile(wsConfigPath); err == nil {
		var wsCfg struct {
			DefaultProvider string `toml:"default_provider"`
			DefaultModel    string `toml:"default_model"`
			DefaultEffort   string `toml:"default_effort"`
			Theme           string `toml:"theme"`
			Language        string `toml:"language"`
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
		}
		if wsCfg.MaxSteps > 0 {
			snap.Global.MaxSteps = wsCfg.MaxSteps
		}
	}

	// 1. agents/*.md 角色覆盖（拦截 cmd: 凭据）
	wsAgentsDir := filepath.Join(wsGorness, "agents")
	if info, err := os.Stat(wsAgentsDir); err == nil && info.IsDir() {
		profiles, err := loadAgentsDir(wsAgentsDir)
		if err != nil {
			return nil, nil, err
		}
		for id, prof := range profiles {
			if hasCmdSecret(prof.APIKeyRef) {
				warnings = append(warnings, fmt.Sprintf("rejected cmd: credential in workspace agent %s: cmd: is forbidden in project overlays", prof.SourceFile))
				prof.APIKeyRef = ""
			}
			snap.Agents[id] = prof
		}
	}

	// 2. workflows/*.toml 拓扑覆盖（未授信工作流不放行自定义 verify_command）
	wsWorkflowsDir := filepath.Join(wsGorness, "workflows")
	if info, err := os.Stat(wsWorkflowsDir); err == nil && info.IsDir() {
		entries, err := os.ReadDir(wsWorkflowsDir)
		if err != nil {
			return nil, nil, err
		}
		for _, ent := range entries {
			if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".toml") {
				continue
			}
			wfPath := filepath.Join(wsWorkflowsDir, ent.Name())
			raw, err := os.ReadFile(wfPath)
			if err != nil {
				return nil, nil, &fileLoadError{File: wfPath, Err: err}
			}
			wf, err := parseWorkflowFile(wfPath, raw)
			if err != nil {
				return nil, nil, err
			}
			if wf.VerifyCommand != "" && !isWorkspaceFileTrusted(wsRoot, wfPath, raw, trustedHashes) {
				warnings = append(warnings, fmt.Sprintf("stripped untrusted verify_command in workspace workflow %s", wfPath))
				wf.VerifyCommand = ""
			}
			snap.Workflows[wf.ID] = wf
		}
	}

	// 3. providers.toml 覆盖（文件需哈希授信，且 cmd: 永远无条件拒绝）
	wsProvidersPath := filepath.Join(wsGorness, "providers.toml")
	if raw, err := os.ReadFile(wsProvidersPath); err == nil {
		provs, err := parseProvidersBytes(wsProvidersPath, raw)
		if err != nil {
			return nil, nil, err
		}
		trusted := isWorkspaceFileTrusted(wsRoot, wsProvidersPath, raw, trustedHashes)
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

	// 4. mcp.toml 覆盖（必须通过 SHA-256 白名单核验，且拦截 cmd: 环境变量/头）
	wsMCPPath := filepath.Join(wsGorness, "mcp.toml")
	if raw, err := os.ReadFile(wsMCPPath); err == nil {
		if !isWorkspaceFileTrusted(wsRoot, wsMCPPath, raw, trustedHashes) {
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

	// 5. rules.toml 覆盖（始终允许收紧 deny/ask；放宽 allow 需哈希授信）
	wsRulesPath := filepath.Join(wsGorness, "rules.toml")
	if raw, err := os.ReadFile(wsRulesPath); err == nil {
		var wsRules RuleSet
		if err := toml.Unmarshal(raw, &wsRules); err != nil {
			return nil, nil, &fileLoadError{File: wsRulesPath, Err: err}
		}
		snap.Rules.DenyCommands = append(snap.Rules.DenyCommands, wsRules.DenyCommands...)
		snap.Rules.DenyPaths = append(snap.Rules.DenyPaths, wsRules.DenyPaths...)
		snap.Rules.AskCommands = append(snap.Rules.AskCommands, wsRules.AskCommands...)
		if isWorkspaceFileTrusted(wsRoot, wsRulesPath, raw, trustedHashes) {
			snap.Rules.AllowCommands = append(snap.Rules.AllowCommands, wsRules.AllowCommands...)
			snap.Rules.AllowPaths = append(snap.Rules.AllowPaths, wsRules.AllowPaths...)
			snap.Rules.AllowedDomains = append(snap.Rules.AllowedDomains, wsRules.AllowedDomains...)
			snap.Rules.Rules = append(snap.Rules.Rules, wsRules.Rules...)
		} else if len(wsRules.AllowCommands)+len(wsRules.AllowPaths)+len(wsRules.AllowedDomains)+len(wsRules.Rules) > 0 {
			warnings = append(warnings, fmt.Sprintf("ignored untrusted allow rules in %s", wsRulesPath))
		}
	}

	snap.UntrustedWarnings = warnings
	return snap, warnings, nil
}

func isWorkspaceFileTrusted(wsRoot, absFile string, raw []byte, trustedHashes map[string]string) bool {
	if len(trustedHashes) == 0 {
		return false
	}
	sumBytes := sha256.Sum256(raw)
	actualHex := hex.EncodeToString(sumBytes[:])

	candidates := []string{absFile}
	if canon, err := filepath.EvalSymlinks(absFile); err == nil && canon != absFile {
		candidates = append(candidates, canon)
	}
	if rel, err := filepath.Rel(wsRoot, absFile); err == nil {
		relSlash := filepath.ToSlash(rel)
		candidates = append(candidates, relSlash)
		if trimmed, ok := strings.CutPrefix(relSlash, ".gorness/"); ok {
			candidates = append(candidates, trimmed)
		}
	}

	matched := false
	for _, key := range candidates {
		expected, exists := trustedHashes[key]
		if !exists {
			continue
		}
		norm := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(expected)), "sha256:")
		if norm != actualHex {
			return false
		}
		matched = true
	}
	return matched
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
	if raw, err := os.ReadFile(m.paths.ConfigFile()); err != nil {
		return nil, &fileLoadError{File: m.paths.ConfigFile(), Err: err}
	} else if err := toml.Unmarshal(raw, &globalCfg); err != nil {
		return nil, &fileLoadError{File: m.paths.ConfigFile(), Err: err}
	}

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
		SkillDirs: skillDirs,
		LoadedAt:  m.now(),
	}, nil
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
		meta, body, err := ParseFrontmatterMD(raw)
		if err != nil {
			return nil, &fileLoadError{File: p, Err: err}
		}
		var prof AgentProfile
		if len(meta) > 0 {
			encoded, err := toml.Marshal(meta)
			if err != nil {
				return nil, &fileLoadError{File: p, Err: err}
			}
			if err := toml.Unmarshal(encoded, &prof); err != nil {
				return nil, &fileLoadError{File: p, Err: err}
			}
		}
		stem := strings.TrimSuffix(name, ".md")
		if prof.ID == "" {
			prof.ID = stem
		}
		if prof.Name == "" {
			prof.Name = prof.ID
		}
		prof.SystemPrompt = strings.TrimSpace(body)
		prof.SourceFile = p
		agents[prof.ID] = prof
	}
	return agents, nil
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
		Host:               "127.0.0.1",
		Port:               3456,
		Theme:              "auto",
		Language:           "zh",
		DefaultProvider:    "anthropic",
		DefaultModel:       "claude-sonnet-5-5",
		DefaultEffort:      "high",
		PermissionMode:     "auto",
		SandboxMode:        "strict",
		MaxConcurrency:     4,
		MaxToolConcurrency: 8,
		MaxSteps:           64,
		InboxDebounceMS:    400,
	}
}

const defaultConfigTOML = `# Gorness Global Configuration (全局主配置 / Global Settings)
host = "127.0.0.1"
port = 3456
theme = "auto"                # auto | light | dark
language = "zh"               # zh | en
default_provider = "anthropic"
default_model = "claude-sonnet-5-5"
default_effort = "high"       # low | medium | high | xhigh | max
permission_mode = "auto"      # plan | ask | accept_edits | auto | yolo
sandbox_mode = "strict"       # strict | readonly | network_allow | off
max_concurrency = 4
max_tool_concurrency = 8
max_steps = 64
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
