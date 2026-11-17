package config

import (
	"path/filepath"
	"time"
)

// Version is the canonical single-authority Gorness product version.
const Version = "1.0.0"

// VaultPaths 定义 ~/.gorness/ 零数据库透明文件仓的标准目录与文件定位契约。
type VaultPaths struct {
	Root string `json:"root"`
}

func (v VaultPaths) ConfigFile() string    { return filepath.Join(v.Root, "config.toml") }
func (v VaultPaths) ProvidersFile() string { return filepath.Join(v.Root, "providers.toml") }
func (v VaultPaths) RulesFile() string     { return filepath.Join(v.Root, "rules.toml") }
func (v VaultPaths) MCPFile() string       { return filepath.Join(v.Root, "mcp.toml") }
func (v VaultPaths) HooksFile() string     { return filepath.Join(v.Root, "hooks.toml") }
func (v VaultPaths) AgentsDir() string     { return filepath.Join(v.Root, "agents") }
func (v VaultPaths) WorkflowsDir() string  { return filepath.Join(v.Root, "workflows") }
func (v VaultPaths) SkillsDir() string     { return filepath.Join(v.Root, "skills") }
func (v VaultPaths) MemoryDir() string     { return filepath.Join(v.Root, "memory") }
func (v VaultPaths) AlwaysOnDir() string   { return filepath.Join(v.Root, "alwayson") }
func (v VaultPaths) WorkspacesDir() string { return filepath.Join(v.Root, "workspaces") }
func (v VaultPaths) RuntimeDir() string    { return filepath.Join(v.Root, "runtime") }
func (v VaultPaths) SnapshotsDir() string  { return filepath.Join(v.Root, "runtime", "snapshots") }
func (v VaultPaths) WorktreesDir() string  { return filepath.Join(v.Root, "runtime", "worktrees") }
func (v VaultPaths) SocketPath() string    { return filepath.Join(v.Root, "runtime", "daemon.sock") }
func (v VaultPaths) LogsDir() string       { return filepath.Join(v.Root, "runtime", "logs") }

type GlobalConfig struct {
	Host                        string   `toml:"host" json:"host"`
	Port                        int      `toml:"port" json:"port"`
	Theme                       string   `toml:"theme" json:"theme"`
	Language                    string   `toml:"language" json:"language"`
	UITheme                     string   `toml:"ui_theme,omitempty" json:"ui_theme,omitempty"`
	UILang                      string   `toml:"ui_lang,omitempty" json:"ui_lang,omitempty"`
	DefaultProvider             string   `toml:"default_provider" json:"default_provider"`
	DefaultModel                string   `toml:"default_model" json:"default_model"`
	DefaultEffort               string   `toml:"default_effort" json:"default_effort"`
	PermissionMode              string   `toml:"permission_mode" json:"permission_mode"`
	SandboxMode                 string   `toml:"sandbox_mode" json:"sandbox_mode"`
	MaxConcurrency              int      `toml:"max_concurrency" json:"max_concurrency"`
	MaxToolConcurrency          int      `toml:"max_tool_concurrency" json:"max_tool_concurrency"`
	MaxSteps                    int      `toml:"max_steps" json:"max_steps"`
	SubagentMaxSteps            int      `toml:"subagent_max_steps" json:"subagent_max_steps"`
	AutoCompactThresholdPercent int      `toml:"auto_compact_threshold_percent" json:"auto_compact_threshold_percent"`
	AutoCompactRatio            float64  `toml:"auto_compact_ratio" json:"auto_compact_ratio"`
	KeptRecentTurnsRatio        float64  `toml:"kept_recent_turns_ratio" json:"kept_recent_turns_ratio"`
	CommandTimeoutSeconds       int      `toml:"command_timeout_seconds" json:"command_timeout_seconds"`
	CommandTimeoutSec           int      `toml:"command_timeout_sec" json:"command_timeout_sec"`
	AutoGenerateTitle           bool     `toml:"auto_generate_title" json:"auto_generate_title"`
	AutoDream                   bool     `toml:"auto_dream" json:"auto_dream"`
	DreamerIdleMinutes          int      `toml:"dreamer_idle_minutes" json:"dreamer_idle_minutes"`
	DreamIdleMinutes            int      `toml:"dream_idle_minutes" json:"dream_idle_minutes"`
	WitnessTimeoutSeconds       int      `toml:"witness_timeout_seconds" json:"witness_timeout_seconds"`
	WitnessStallSec             int      `toml:"witness_stall_sec" json:"witness_stall_sec"`
	LLMMaxRetries               int      `toml:"llm_max_retries" json:"llm_max_retries"`
	LLMTimeoutSec               int      `toml:"llm_timeout_sec" json:"llm_timeout_sec"`
	InboxDebounceMS             int      `toml:"inbox_debounce_ms" json:"inbox_debounce_ms"`
	ExtraSkillDirs              []string `toml:"extra_skill_dirs,omitempty" json:"extra_skill_dirs,omitempty"`
}

type ModelSpec struct {
	ID                 string  `toml:"id,omitempty" json:"id"`
	Name               string  `toml:"name,omitempty" json:"name,omitempty"`
	ContextWindow      int     `toml:"context_window,omitempty" json:"context_window,omitempty"`
	MaxOutputTokens    int     `toml:"max_output_tokens,omitempty" json:"max_output_tokens,omitempty"`
	InputCostPer1M     float64 `toml:"input_cost_per_1m,omitempty" json:"input_cost_per_1m,omitempty"`
	OutputCostPer1M    float64 `toml:"output_cost_per_1m,omitempty" json:"output_cost_per_1m,omitempty"`
	CacheReadCostPer1M float64 `toml:"cache_read_cost_per_1m,omitempty" json:"cache_read_cost_per_1m,omitempty"`
	SupportsThinking   bool    `toml:"supports_thinking,omitempty" json:"supports_thinking,omitempty"`
	SupportsCaching    bool    `toml:"supports_caching,omitempty" json:"supports_caching,omitempty"`
}

type ProviderConfig struct {
	ID         string               `toml:"id,omitempty" json:"id"`
	Name       string               `toml:"name,omitempty" json:"name,omitempty"`
	Protocol   string               `toml:"protocol" json:"protocol"`
	BaseURL    string               `toml:"base_url" json:"base_url"`
	APIKeyRef  string               `toml:"api_key" json:"api_key"`
	Headers    map[string]string    `toml:"headers,omitempty" json:"headers,omitempty"`
	TimeoutSec int                  `toml:"timeout_sec,omitempty" json:"timeout_sec,omitempty"`
	Models     map[string]ModelSpec `toml:"models,omitempty" json:"models,omitempty"`
	SourceFile string               `toml:"-" json:"source_file,omitempty"`
}

type AgentProfile struct {
	ID              string   `toml:"id,omitempty" json:"id"`
	Name            string   `toml:"name,omitempty" json:"name,omitempty"`
	Description     string   `toml:"description,omitempty" json:"description,omitempty"`
	Provider        string   `toml:"provider" json:"provider"`
	Model           string   `toml:"model" json:"model"`
	Effort          string   `toml:"effort,omitempty" json:"effort,omitempty"`
	Fallbacks       []string `toml:"fallbacks,omitempty" json:"fallbacks,omitempty"`
	AllowedTools    []string `toml:"allowed_tools,omitempty" json:"allowed_tools,omitempty"`
	DisallowedTools []string `toml:"disallowed_tools,omitempty" json:"disallowed_tools,omitempty"`
	ReadOnly        bool     `toml:"read_only,omitempty" json:"read_only,omitempty"`
	APIKeyRef       string   `toml:"api_key,omitempty" json:"api_key,omitempty"`
	SystemPrompt    string   `toml:"-" json:"system_prompt"`
	SourceFile      string   `toml:"-" json:"source_file,omitempty"`
}

type WorkflowNodeSpec struct {
	ID       string `toml:"id" json:"id"`
	Label    string `toml:"label,omitempty" json:"label,omitempty"`
	Role     string `toml:"role" json:"role"`
	Worktree bool   `toml:"worktree,omitempty" json:"worktree,omitempty"`
}

type WorkflowEdgeSpec struct {
	From  string `toml:"from" json:"from"`
	To    string `toml:"to" json:"to"`
	Label string `toml:"label,omitempty" json:"label,omitempty"`
	Kind  string `toml:"kind,omitempty" json:"kind,omitempty"`
}

type WorkflowTemplate struct {
	ID             string             `toml:"id" json:"id"`
	Name           string             `toml:"name" json:"name"`
	Description    string             `toml:"description,omitempty" json:"description,omitempty"`
	Topology       string             `toml:"topology" json:"topology"`
	MaxConcurrency int                `toml:"max_concurrency,omitempty" json:"max_concurrency,omitempty"`
	UseWorktrees   bool               `toml:"use_worktrees,omitempty" json:"use_worktrees,omitempty"`
	VerifyCommand  string             `toml:"verify_command,omitempty" json:"verify_command,omitempty"`
	MaxRounds      int                `toml:"max_rounds,omitempty" json:"max_rounds,omitempty"`
	MaxIdleRounds  int                `toml:"max_idle_rounds,omitempty" json:"max_idle_rounds,omitempty"`
	MaxHops        int                `toml:"max_hops,omitempty" json:"max_hops,omitempty"`
	BudgetUSD      float64            `toml:"budget_usd,omitempty" json:"budget_usd,omitempty"`
	RoleModels     map[string]string  `toml:"role_models,omitempty" json:"role_models,omitempty"`
	Roles          []string           `toml:"roles,omitempty" json:"roles,omitempty"`
	Nodes          []WorkflowNodeSpec `toml:"nodes,omitempty" json:"nodes,omitempty"`
	Edges          []WorkflowEdgeSpec `toml:"edges,omitempty" json:"edges,omitempty"`
	SourceFile     string             `toml:"-" json:"source_file,omitempty"`
}

type PermissionRule struct {
	Tool    string `toml:"tool,omitempty" json:"tool,omitempty"`
	Pattern string `toml:"pattern" json:"pattern"`
	Action  string `toml:"action" json:"action"`
	Reason  string `toml:"reason,omitempty" json:"reason,omitempty"`
}

type RuleSet struct {
	AllowCommands  []string         `toml:"allow_commands,omitempty" json:"allow_commands,omitempty"`
	AskCommands    []string         `toml:"ask_commands,omitempty" json:"ask_commands,omitempty"`
	DenyCommands   []string         `toml:"deny_commands,omitempty" json:"deny_commands,omitempty"`
	AllowPaths     []string         `toml:"allow_paths,omitempty" json:"allow_paths,omitempty"`
	DenyPaths      []string         `toml:"deny_paths,omitempty" json:"deny_paths,omitempty"`
	AllowedDomains []string         `toml:"allowed_domains,omitempty" json:"allowed_domains,omitempty"`
	Rules          []PermissionRule `toml:"rules,omitempty" json:"rules,omitempty"`
}

type MCPServerConfig struct {
	ID         string            `toml:"id,omitempty" json:"id"`
	Transport  string            `toml:"transport" json:"transport"`
	Command    string            `toml:"command,omitempty" json:"command,omitempty"`
	Args       []string          `toml:"args,omitempty" json:"args,omitempty"`
	URL        string            `toml:"url,omitempty" json:"url,omitempty"`
	Env        map[string]string `toml:"env,omitempty" json:"env,omitempty"`
	Headers    map[string]string `toml:"headers,omitempty" json:"headers,omitempty"`
	Enabled    *bool             `toml:"enabled,omitempty" json:"enabled,omitempty"`
	TimeoutSec int               `toml:"timeout_sec,omitempty" json:"timeout_sec,omitempty"`
	SourceFile string            `toml:"-" json:"source_file,omitempty"`
}

type MCPConfig struct {
	Servers map[string]MCPServerConfig `toml:"servers" json:"servers"`
}

// Snapshot 是不可变配置快照；热更新失败或项目级覆盖均通过深拷贝隔离，绝不原地篡改全局活动快照。
type Snapshot struct {
	Paths             VaultPaths                  `json:"paths"`
	Global            GlobalConfig                `json:"global"`
	Providers         map[string]ProviderConfig   `json:"providers"`
	Agents            map[string]AgentProfile     `json:"agents"`
	Workflows         map[string]WorkflowTemplate `json:"workflows"`
	Rules             RuleSet                     `json:"rules"`
	MCP               MCPConfig                   `json:"mcp"`
	Hooks             HooksConfig                 `json:"hooks"`
	SkillDirs         []string                    `json:"skill_dirs"`
	UntrustedWarnings []string                    `json:"untrusted_warnings,omitempty"`
	LoadedAt          time.Time                   `json:"loaded_at"`
}

func cloneSlice[T any](in []T) []T {
	if in == nil {
		return nil
	}
	out := make([]T, len(in))
	copy(out, in)
	return out
}

func (s *Snapshot) Clone() *Snapshot {
	if s == nil {
		return nil
	}
	cp := *s
	cp.Global.ExtraSkillDirs = cloneSlice(s.Global.ExtraSkillDirs)

	cp.Providers = make(map[string]ProviderConfig, len(s.Providers))
	for k, p := range s.Providers {
		pCopy := p
		if p.Headers != nil {
			pCopy.Headers = make(map[string]string, len(p.Headers))
			for hk, hv := range p.Headers {
				pCopy.Headers[hk] = hv
			}
		}
		if p.Models != nil {
			pCopy.Models = make(map[string]ModelSpec, len(p.Models))
			for mk, mv := range p.Models {
				pCopy.Models[mk] = mv
			}
		}
		cp.Providers[k] = pCopy
	}

	cp.Agents = make(map[string]AgentProfile, len(s.Agents))
	for k, a := range s.Agents {
		aCopy := a
		aCopy.Fallbacks = cloneSlice(a.Fallbacks)
		aCopy.AllowedTools = cloneSlice(a.AllowedTools)
		aCopy.DisallowedTools = cloneSlice(a.DisallowedTools)
		cp.Agents[k] = aCopy
	}

	cp.Workflows = make(map[string]WorkflowTemplate, len(s.Workflows))
	for k, w := range s.Workflows {
		wCopy := w
		wCopy.Roles = cloneSlice(w.Roles)
		wCopy.Nodes = cloneSlice(w.Nodes)
		wCopy.Edges = cloneSlice(w.Edges)
		if w.RoleModels != nil {
			wCopy.RoleModels = make(map[string]string, len(w.RoleModels))
			for rk, rv := range w.RoleModels {
				wCopy.RoleModels[rk] = rv
			}
		}
		cp.Workflows[k] = wCopy
	}

	cp.Rules = RuleSet{
		AllowCommands:  cloneSlice(s.Rules.AllowCommands),
		AskCommands:    cloneSlice(s.Rules.AskCommands),
		DenyCommands:   cloneSlice(s.Rules.DenyCommands),
		AllowPaths:     cloneSlice(s.Rules.AllowPaths),
		DenyPaths:      cloneSlice(s.Rules.DenyPaths),
		AllowedDomains: cloneSlice(s.Rules.AllowedDomains),
		Rules:          cloneSlice(s.Rules.Rules),
	}

	cp.MCP = MCPConfig{
		Servers: make(map[string]MCPServerConfig, len(s.MCP.Servers)),
	}
	for k, srv := range s.MCP.Servers {
		srvCopy := srv
		srvCopy.Args = cloneSlice(srv.Args)
		if srv.Env != nil {
			srvCopy.Env = make(map[string]string, len(srv.Env))
			for ek, ev := range srv.Env {
				srvCopy.Env[ek] = ev
			}
		}
		if srv.Headers != nil {
			srvCopy.Headers = make(map[string]string, len(srv.Headers))
			for hk, hv := range srv.Headers {
				srvCopy.Headers[hk] = hv
			}
		}
		if srv.Enabled != nil {
			b := *srv.Enabled
			srvCopy.Enabled = &b
		}
		cp.MCP.Servers[k] = srvCopy
	}

	cp.Hooks = s.Hooks.Clone()
	cp.SkillDirs = cloneSlice(s.SkillDirs)
	cp.UntrustedWarnings = cloneSlice(s.UntrustedWarnings)
	return &cp
}

type ReloadDiagnostic struct {
	File      string    `json:"file"`
	Message   string    `json:"message"`
	Err       error     `json:"-"`
	Timestamp time.Time `json:"timestamp"`
}
