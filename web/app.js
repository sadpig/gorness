// Gorness Apple HIG Workbench & Interactive Visual Multi-Agent SVG Studio
// Pure Vanilla ES Module · Zero Build Chain · Strict XSS + Bidi Neutralization

const SETTLE_GUARD_MS = 500;

const I18N = {
  zh: {
    brandSub: "macOS 纯 Go · 零数据库工作台",
    modeChat: "对话工作台",
    modeFleet: "多智能体编排画布",
    modeConfig: "配置矩阵与设置",
    alwaysOnBtn: "常驻队列",
    themeAuto: "自动",
    themeLight: "浅色",
    themeDark: "深色",
    searchPlaceholder: "跨项目搜索会话与代码片段 (⌘K)...",
    needsAttention: "待审批事项",
    projectsHeader: "工作区与会话",
    addProjectBtn: "+ 项目",
    newSessionBtn: "+ 会话",
    configStudioBtn: "配置矩阵",
    roleLabel: "角色",
    modelSelectLabel: "模型",
    permLabel: "权限",
    permAutoOpt: "自动安全门控 (Auto)",
    permAcceptEditsOpt: "自动接受编辑 (Accept Edits)",
    permAskOpt: "每次询问 (Ask)",
    permPlanOpt: "只读规划 (Plan)",
    permYoloOpt: "全放行 (Yolo)",
    modelOverridePh: "跟随角色默认模型",
    cfgPageMatrix: "异构角色模型矩阵",
    cfgPageProviders: "供应商与模型目录",
    cfgPageWorkflows: "多智能体拓扑配置",
    cfgPageGlobal: "全局与权限沙箱",
    cfgPageMCP: "MCP 扩展服务",
    cfgPageFiles: "底层源文件",
    autoSaveReady: "点选即存 · 实时热生效 ✓",
    expandConfigStudioBtn: "展开完整配置中心 ↗",
    inspMatrixHint: "直接下拉点选服务商、模型与推理强度，即刻自动写入 ~/.gorness/agents/*.md",
    finderPickerTitle: "通过 macOS Finder 图形选择器添加",
    finderPickerSub: "无需手输路径，直接弹出原生文件夹对话框选择代码仓库",
    pickFolderBtn: "📁 选择本地文件夹…",
    discoveredReposLabel: "本机常用开发目录下发现的项目（点击即可一键挂载）：",
    manualPathToggle: "或手动粘贴/微调目录路径",
    compactBtn: "压缩",
    forkBtn: "分叉",
    exportHtmlBtn: "导出预览",
    stopBtn: "停止",
    composerPlaceholder: "输入任务指令… 输入 @ 引用文件，$ 激活技能，/ 调用命令（Shift+Enter 换行，Enter 发送 / 运行中注入 Steer）",
    hintFile: "文件",
    hintSkill: "技能",
    hintCmd: "命令",
    launchFleetQuick: "启动舰队协同…",
    sendBtn: "发送",
    steerBtn: "注入 Steer",
    stopMissionBtn: "终止编排",
    newMissionBtn: "+ 启动多智能体编排",
    fleetEmptyGoal: "选择或启动任意 6 种异构模型多智能体拓扑",
    nodeInspectorTitle: "节点详情与 Git Worktree 差异",
    nodeSteerPh: "向选中节点中途注入步边界 Steer 指令…",
    steerNodeBtn: "注入 Steer",
    clickNodeHint: "点击上方拓扑中的任意 Agent 节点查看其实时流与隔离 Git Worktree Diff。",
    blackboardTitle: "Markdown 共享黑板 (tasks/*.md & notes/*.md)",
    tabFiles: "文件",
    tabSnapshots: "时光机",
    tabConfig: "配置矩阵",
    tabPreview: "沙箱预览",
    filterFilesPh: "过滤项目文件…",
    safeOpenBtn: "安全打开 / Finder",
    restoreFilesToggle: "回退时按 SHA-256 快照同步还原源码文件",
    agentMatrixTitle: "异构 Agent 角色模型矩阵",
    addProviderBtn: "+ 供应商",
    saveProviderBtn: "写入 providers.toml",
    vaultEditorTitle: "~/.gorness 实时 TOML / MD 编辑器",
    saveHotReloadBtn: "保存并热加载",
    renderSnippetBtn: "新建 HTML 沙箱",
    openStandalone: "独立窗口 ↗",
    mintPreviewBtn: "生成隔离预览",
    registerWsTitle: "注册本地 macOS 项目工作区",
    wsPathLabel: "本地目录绝对路径",
    wsNameLabel: "显示名称（可选）",
    cancelBtn: "取消",
    confirmRegisterBtn: "注册项目",
    launchFleetTitle: "启动可视化异构多智能体协同编排",
    missionGoalLabel: "编排目标 (Mission Goal)",
    verifyCmdLabel: "Refinery 验收门控命令",
    budgetLabel: "预算上限 (USD, 0=不限)",
    useWorktreesLabel: "启用独立 Git Worktree 物理隔离与串行变基快进合入 (--ff-only)",
    startMissionSubmit: "立即启动拓扑",
    alwaysOnTitle: "Always-On 休眠唤醒中枢 (Inbox · Timers · Deferred · Outbox · Dreamer)",
    aoTimersHead: "持久化定时器 (timers.toml)",
    runDreamBtn: "运行 Dreamer 提炼技能",
    aoWebhookHead: "本地事件注入 & 发件箱 (outbox.jsonl)",
    enqueueBtn: "投递",
    approveHunksBtn: "批准选中块",
    approveRememberBtn: "批准并记住",
    denyBtn: "拒绝",
    settlingLabel: "防连点冷静期 500ms…",
    settledReadyLabel: "可裁决",
    rewindHereBtn: "回退至此",
    forkHereBtn: "由此分叉",
    emptyChatHint: "选择一个工作区并输入指令，或切换到「多智能体编排画布」启动 6 种异构舰队。",
    draftSaved: "草稿已防丢保存",
    subagentLabel: "子智能体",
    forkedChildLabel: "分叉子会话",
    fleetWorkersLabel: "舰队协同子会话",
    jumpSessionBtn: "跳转会话 ↗",
    openFleetBtn: "打开拓扑 ↗",
    runErrorTitle: "运行错误"
  },
  en: {
    brandSub: "macOS Pure Go · Zero-DB Workbench",
    modeChat: "Conversation",
    modeFleet: "Fleet Studio",
    modeConfig: "Config & Matrix",
    alwaysOnBtn: "Always-On",
    themeAuto: "Auto",
    themeLight: "Light",
    themeDark: "Dark",
    searchPlaceholder: "Search sessions & code across workspaces (⌘K)...",
    needsAttention: "Needs Attention",
    projectsHeader: "Workspaces & Sessions",
    addProjectBtn: "+ Project",
    newSessionBtn: "+ Session",
    configStudioBtn: "Config Matrix",
    roleLabel: "Role",
    modelSelectLabel: "Model",
    permLabel: "Perm",
    permAutoOpt: "Smart Gate (Auto)",
    permAcceptEditsOpt: "Accept Edits",
    permAskOpt: "Always Ask",
    permPlanOpt: "Read-Only Plan",
    permYoloOpt: "Bypass Gate (Yolo)",
    modelOverridePh: "Follow Role Default",
    cfgPageMatrix: "Role-Model Matrix",
    cfgPageProviders: "Providers & Models",
    cfgPageWorkflows: "Fleet Topologies",
    cfgPageGlobal: "Global & Sandbox",
    cfgPageMCP: "MCP Extensions",
    cfgPageFiles: "Raw Vault Files",
    autoSaveReady: "Click-to-Save · Live Hot-Reload ✓",
    expandConfigStudioBtn: "Full Config Studio ↗",
    inspMatrixHint: "Select provider, model, and effort from dropdowns — auto-saved to ~/.gorness/agents/*.md",
    finderPickerTitle: "Add via macOS Native Finder Dialog",
    finderPickerSub: "Open the native macOS folder chooser without typing paths",
    pickFolderBtn: "📁 Choose Local Folder…",
    discoveredReposLabel: "Discovered repositories on this Mac (click to mount):",
    manualPathToggle: "Or paste a custom directory path",
    compactBtn: "Compact",
    forkBtn: "Fork",
    exportHtmlBtn: "Export Preview",
    stopBtn: "Stop",
    composerPlaceholder: "Ask Gorness… Type @ for files, $ for skills, / for commands (Shift+Enter newline, Enter send / Steer active run)",
    hintFile: "Files",
    hintSkill: "Skills",
    hintCmd: "Commands",
    launchFleetQuick: "Launch Fleet…",
    sendBtn: "Send",
    steerBtn: "Inject Steer",
    stopMissionBtn: "Stop Mission",
    newMissionBtn: "+ Launch Fleet Mission",
    fleetEmptyGoal: "Select or launch any of the 6 heterogeneous multi-agent topologies",
    nodeInspectorTitle: "Node Inspector & Git Worktree Diff",
    nodeSteerPh: "Inject mid-run step-boundary Steer to selected node…",
    steerNodeBtn: "Steer Node",
    clickNodeHint: "Click any agent node in the topology graph above to inspect live logs & Git Worktree diff.",
    blackboardTitle: "Markdown Shared Blackboard (tasks/*.md & notes/*.md)",
    tabFiles: "Files",
    tabSnapshots: "Snapshots",
    tabConfig: "Config",
    tabPreview: "Preview",
    filterFilesPh: "Filter workspace files…",
    safeOpenBtn: "Safe Open / Reveal",
    restoreFilesToggle: "Restore modified workspace files from SHA-256 snapshots on rewind",
    agentMatrixTitle: "Heterogeneous Agent-Model Matrix",
    addProviderBtn: "+ Provider",
    saveProviderBtn: "Save to providers.toml",
    vaultEditorTitle: "Live ~/.gorness TOML / MD Editor",
    saveHotReloadBtn: "Save & Hot-Reload",
    renderSnippetBtn: "New HTML Sandbox",
    openStandalone: "Standalone ↗",
    mintPreviewBtn: "Mint Preview",
    registerWsTitle: "Register Local macOS Workspace",
    wsPathLabel: "Absolute Directory Path",
    wsNameLabel: "Display Name (optional)",
    cancelBtn: "Cancel",
    confirmRegisterBtn: "Register",
    launchFleetTitle: "Launch Visual Multi-Agent Orchestration",
    missionGoalLabel: "Mission Goal",
    verifyCmdLabel: "Refinery Verify Gate Command",
    budgetLabel: "Budget Ceiling (USD, 0=unlimited)",
    useWorktreesLabel: "Isolate writers in Git Worktrees & land serially via Refinery (--ff-only)",
    startMissionSubmit: "Launch Topology",
    alwaysOnTitle: "Always-On Sleep-Wake Hub (Inbox · Timers · Deferred · Outbox · Dreamer)",
    aoTimersHead: "Durable Timers (timers.toml)",
    runDreamBtn: "Run Dreamer Pass",
    aoWebhookHead: "Local Webhook Ingress & Outbox (outbox.jsonl)",
    enqueueBtn: "Enqueue",
    approveHunksBtn: "Approve Selected Hunks",
    approveRememberBtn: "Approve & Remember",
    denyBtn: "Deny",
    settlingLabel: "500ms settle guard…",
    settledReadyLabel: "Ready",
    rewindHereBtn: "Rewind Here",
    forkHereBtn: "Fork Here",
    emptyChatHint: "Select a workspace and send a prompt, or switch to Fleet Studio to launch any of the 6 topologies.",
    draftSaved: "Draft persisted",
    subagentLabel: "Subagent",
    forkedChildLabel: "Forked Child Session",
    fleetWorkersLabel: "Fleet Worker Subagents",
    jumpSessionBtn: "Open Session ↗",
    openFleetBtn: "Fleet Studio ↗",
    runErrorTitle: "Run Error"
  }
};

const SIX_TOPOLOGIES = [
  { id: "hybrid-fleet", name: "Hybrid Fleet", zh: "旗舰混合舰队", desc: "Lead -> Explorers + Worktree Coders -> Blind Verifier" },
  { id: "review-loop", name: "Review Loop", zh: "生成-验证对抗闭环", desc: "Generator Coder <-> Heterogeneous Verifier" },
  { id: "parallel-explore", name: "Parallel Explore", zh: "缓存共享并发探索", desc: "Concurrent Read-Only Explorers -> Synthesis Lead" },
  { id: "worktree-team", name: "Worktree Team", zh: "独立 Worktree 专家战队", desc: "Parallel Worktree Coders -> Refinery Rebase & Gate" },
  { id: "event-pipeline", name: "Event Pipeline", zh: "强类型事件流水线", desc: "Recon -> Build -> Gate -> Debug Stage Hop Guard" },
  { id: "shared-board", name: "Shared Board", zh: "Markdown 共享黑板拆树", desc: "Flock Task Claiming + Dynamic Split Depth <= 3" }
];

// ============================================================================
// Built-in Zero-Typing Presets (Providers, Models, Role Templates, Rules, MCP)
// ============================================================================

export const PROVIDER_PRESETS = {
  anthropic: {
    id: "anthropic",
    name: "Anthropic",
    zh: "Anthropic 官方 (Claude)",
    protocol: "anthropic",
    base_url: "https://api.anthropic.com",
    endpoints: ["https://api.anthropic.com"],
    default_key_ref: "env:ANTHROPIC_API_KEY",
    env_keys: ["env:ANTHROPIC_API_KEY", "env:CLAUDE_API_KEY"],
    keychain_refs: ["keychain:gorness-anthropic", "keychain:anthropic-api-key"],
    timeout_sec: 120,
    models: {
      "claude-opus-5-5": { id: "claude-opus-5-5", name: "Claude Opus 5.5", context_window: 200000, max_output_tokens: 32000, input_cost_per_1m: 15.0, output_cost_per_1m: 75.0, cache_read_cost_per_1m: 1.5, supports_thinking: true, supports_caching: true },
      "claude-sonnet-5-5": { id: "claude-sonnet-5-5", name: "Claude Sonnet 5.5", context_window: 200000, max_output_tokens: 16000, input_cost_per_1m: 3.0, output_cost_per_1m: 15.0, cache_read_cost_per_1m: 0.3, supports_thinking: true, supports_caching: true },
      "claude-haiku-5-5": { id: "claude-haiku-5-5", name: "Claude Haiku 5.5", context_window: 200000, max_output_tokens: 8192, input_cost_per_1m: 0.8, output_cost_per_1m: 4.0, cache_read_cost_per_1m: 0.08, supports_thinking: false, supports_caching: true },
      "claude-opus-4-7": { id: "claude-opus-4-7", name: "Claude Opus 4.7", context_window: 200000, max_output_tokens: 32000, input_cost_per_1m: 15.0, output_cost_per_1m: 75.0, cache_read_cost_per_1m: 1.5, supports_thinking: true, supports_caching: true },
      "claude-sonnet-4-6": { id: "claude-sonnet-4-6", name: "Claude Sonnet 4.6", context_window: 200000, max_output_tokens: 16000, input_cost_per_1m: 3.0, output_cost_per_1m: 15.0, cache_read_cost_per_1m: 0.3, supports_thinking: true, supports_caching: true }
    }
  },
  openai: {
    id: "openai",
    name: "OpenAI",
    zh: "OpenAI 官方 (GPT / o 系列)",
    protocol: "openai-responses",
    base_url: "https://api.openai.com/v1",
    endpoints: ["https://api.openai.com/v1"],
    default_key_ref: "env:OPENAI_API_KEY",
    env_keys: ["env:OPENAI_API_KEY"],
    keychain_refs: ["keychain:gorness-openai", "keychain:openai-api-key"],
    timeout_sec: 120,
    models: {
      "gpt-5.5": { id: "gpt-5.5", name: "GPT-5.5", context_window: 256000, max_output_tokens: 32000, input_cost_per_1m: 2.5, output_cost_per_1m: 10.0, supports_thinking: true },
      "gpt-5.4": { id: "gpt-5.4", name: "GPT-5.4", context_window: 256000, max_output_tokens: 32000, input_cost_per_1m: 2.0, output_cost_per_1m: 8.0, supports_thinking: true },
      "gpt-5.4-mini": { id: "gpt-5.4-mini", name: "GPT-5.4 Mini", context_window: 200000, max_output_tokens: 16384, input_cost_per_1m: 0.4, output_cost_per_1m: 1.6, supports_thinking: true },
      "o3": { id: "o3", name: "OpenAI o3 Reasoning", context_window: 200000, max_output_tokens: 32000, input_cost_per_1m: 2.0, output_cost_per_1m: 8.0, supports_thinking: true },
      "o4-mini": { id: "o4-mini", name: "OpenAI o4-mini", context_window: 200000, max_output_tokens: 16384, input_cost_per_1m: 1.1, output_cost_per_1m: 4.4, supports_thinking: true }
    }
  },
  gemini: {
    id: "gemini",
    name: "Google Gemini",
    zh: "Google Gemini 官方",
    protocol: "gemini",
    base_url: "https://generativelanguage.googleapis.com/v1beta",
    endpoints: ["https://generativelanguage.googleapis.com/v1beta"],
    default_key_ref: "env:GEMINI_API_KEY",
    env_keys: ["env:GEMINI_API_KEY", "env:GOOGLE_API_KEY"],
    keychain_refs: ["keychain:gorness-gemini"],
    timeout_sec: 120,
    models: {
      "gemini-2.5-pro": { id: "gemini-2.5-pro", name: "Gemini 2.5 Pro", context_window: 1000000, max_output_tokens: 16384, input_cost_per_1m: 1.25, output_cost_per_1m: 10.0, supports_thinking: true },
      "gemini-2.5-flash": { id: "gemini-2.5-flash", name: "Gemini 2.5 Flash", context_window: 1000000, max_output_tokens: 16384, input_cost_per_1m: 0.15, output_cost_per_1m: 0.6, supports_thinking: true },
      "gemini-3-pro": { id: "gemini-3-pro", name: "Gemini 3 Pro", context_window: 1000000, max_output_tokens: 32768, input_cost_per_1m: 2.0, output_cost_per_1m: 12.0, supports_thinking: true }
    }
  },
  apple_local: {
    id: "apple_local",
    name: "Local Gateway (11211)",
    zh: "本机 AI 网关 (localhost:11211)",
    protocol: "openai-compat",
    base_url: "http://localhost:11211/api/openai/v1",
    endpoints: ["http://localhost:11211/api/openai/v1", "http://127.0.0.1:11211/v1"],
    default_key_ref: "env:LOCAL_GATEWAY_KEY",
    env_keys: ["env:LOCAL_GATEWAY_KEY", "env:APPLE_GENAI_KEY"],
    keychain_refs: ["keychain:gorness-local-gateway"],
    timeout_sec: 120,
    models: {
      "claude-sonnet-4-6": { id: "claude-sonnet-4-6", name: "Claude Sonnet 4.6 (Gateway)", context_window: 200000, max_output_tokens: 16384, supports_thinking: true },
      "claude-opus-4-7": { id: "claude-opus-4-7", name: "Claude Opus 4.7 (Gateway)", context_window: 200000, max_output_tokens: 32000, supports_thinking: true },
      "gpt-5.4": { id: "gpt-5.4", name: "GPT-5.4 (Gateway)", context_window: 200000, max_output_tokens: 16384, supports_thinking: true },
      "gemini-2.5-pro": { id: "gemini-2.5-pro", name: "Gemini 2.5 Pro (Gateway)", context_window: 1000000, max_output_tokens: 16384, supports_thinking: true }
    }
  },
  deepseek: {
    id: "deepseek",
    name: "DeepSeek",
    zh: "DeepSeek 深度求索",
    protocol: "openai-compat",
    base_url: "https://api.deepseek.com/v1",
    endpoints: ["https://api.deepseek.com/v1", "https://api.deepseek.com"],
    default_key_ref: "env:DEEPSEEK_API_KEY",
    env_keys: ["env:DEEPSEEK_API_KEY"],
    keychain_refs: ["keychain:gorness-deepseek"],
    timeout_sec: 120,
    models: {
      "deepseek-chat": { id: "deepseek-chat", name: "DeepSeek V3.2 Chat", context_window: 128000, max_output_tokens: 8192, input_cost_per_1m: 0.27, output_cost_per_1m: 1.1, supports_caching: true },
      "deepseek-reasoner": { id: "deepseek-reasoner", name: "DeepSeek R1 Reasoner", context_window: 128000, max_output_tokens: 16384, input_cost_per_1m: 0.55, output_cost_per_1m: 2.19, supports_thinking: true, supports_caching: true }
    }
  },
  openrouter: {
    id: "openrouter",
    name: "OpenRouter",
    zh: "OpenRouter 聚合网关",
    protocol: "openai-compat",
    base_url: "https://openrouter.ai/api/v1",
    endpoints: ["https://openrouter.ai/api/v1"],
    default_key_ref: "env:OPENROUTER_API_KEY",
    env_keys: ["env:OPENROUTER_API_KEY"],
    keychain_refs: ["keychain:gorness-openrouter"],
    timeout_sec: 120,
    models: {
      "anthropic/claude-sonnet-4": { id: "anthropic/claude-sonnet-4", name: "OR · Claude Sonnet 4", context_window: 200000, max_output_tokens: 16000, supports_thinking: true },
      "openai/gpt-5": { id: "openai/gpt-5", name: "OR · GPT-5", context_window: 200000, max_output_tokens: 16000, supports_thinking: true },
      "google/gemini-2.5-pro": { id: "google/gemini-2.5-pro", name: "OR · Gemini 2.5 Pro", context_window: 1000000, max_output_tokens: 16000, supports_thinking: true },
      "deepseek/deepseek-r1": { id: "deepseek/deepseek-r1", name: "OR · DeepSeek R1", context_window: 128000, max_output_tokens: 16000, supports_thinking: true }
    }
  },
  dashscope: {
    id: "dashscope",
    name: "Alibaba DashScope",
    zh: "阿里云百炼 (Qwen3)",
    protocol: "openai-compat",
    base_url: "https://dashscope.aliyuncs.com/compatible-mode/v1",
    endpoints: ["https://dashscope.aliyuncs.com/compatible-mode/v1"],
    default_key_ref: "env:DASHSCOPE_API_KEY",
    env_keys: ["env:DASHSCOPE_API_KEY"],
    keychain_refs: ["keychain:gorness-dashscope"],
    timeout_sec: 120,
    models: {
      "qwen3-coder-plus": { id: "qwen3-coder-plus", name: "Qwen3 Coder Plus", context_window: 131072, max_output_tokens: 16384, input_cost_per_1m: 0.5, output_cost_per_1m: 1.5, supports_thinking: true },
      "qwen-max-latest": { id: "qwen-max-latest", name: "Qwen Max Latest", context_window: 131072, max_output_tokens: 8192, input_cost_per_1m: 0.8, output_cost_per_1m: 2.4 },
      "qwen-turbo-latest": { id: "qwen-turbo-latest", name: "Qwen Turbo Fast", context_window: 131072, max_output_tokens: 8192, input_cost_per_1m: 0.05, output_cost_per_1m: 0.2 }
    }
  },
  volcengine: {
    id: "volcengine",
    name: "Volcengine Ark",
    zh: "字节火山方舟 (豆包 Seed)",
    protocol: "openai-compat",
    base_url: "https://ark.cn-beijing.volces.com/api/v3",
    endpoints: ["https://ark.cn-beijing.volces.com/api/v3"],
    default_key_ref: "env:ARK_API_KEY",
    env_keys: ["env:ARK_API_KEY", "env:VOLC_API_KEY"],
    keychain_refs: ["keychain:gorness-volcengine"],
    timeout_sec: 120,
    models: {
      "doubao-seed-1-6-thinking-250615": { id: "doubao-seed-1-6-thinking-250615", name: "Doubao Seed 1.6 Thinking", context_window: 256000, max_output_tokens: 16384, supports_thinking: true },
      "doubao-seed-1-6-250615": { id: "doubao-seed-1-6-250615", name: "Doubao Seed 1.6 Pro", context_window: 256000, max_output_tokens: 16384 },
      "deepseek-r1-250528": { id: "deepseek-r1-250528", name: "Ark · DeepSeek R1", context_window: 128000, max_output_tokens: 16384, supports_thinking: true }
    }
  },
  moonshot: {
    id: "moonshot",
    name: "Moonshot Kimi",
    zh: "月之暗面 (Kimi K2)",
    protocol: "openai-compat",
    base_url: "https://api.moonshot.cn/v1",
    endpoints: ["https://api.moonshot.cn/v1"],
    default_key_ref: "env:MOONSHOT_API_KEY",
    env_keys: ["env:MOONSHOT_API_KEY"],
    keychain_refs: ["keychain:gorness-moonshot"],
    timeout_sec: 120,
    models: {
      "kimi-k2-0711-preview": { id: "kimi-k2-0711-preview", name: "Kimi K2 Coder", context_window: 131072, max_output_tokens: 16384, input_cost_per_1m: 0.6, output_cost_per_1m: 2.5 },
      "kimi-thinking-preview": { id: "kimi-thinking-preview", name: "Kimi Thinking", context_window: 131072, max_output_tokens: 16384, supports_thinking: true }
    }
  },
  local_mlx: {
    id: "local_mlx",
    name: "Local MLX / Ollama",
    zh: "本机 Ollama / MLX / LM Studio",
    protocol: "openai-compat",
    base_url: "http://127.0.0.1:11434/v1",
    endpoints: ["http://127.0.0.1:11434/v1", "http://127.0.0.1:8080/v1", "http://127.0.0.1:1234/v1"],
    default_key_ref: "env:MLX_API_KEY",
    env_keys: ["env:MLX_API_KEY", "env:OLLAMA_API_KEY"],
    keychain_refs: ["keychain:gorness-mlx"],
    timeout_sec: 90,
    models: {
      "qwen3-coder": { id: "qwen3-coder", name: "Qwen3 Coder Local", context_window: 131072, max_output_tokens: 8192 },
      "deepseek-coder-v2": { id: "deepseek-coder-v2", name: "DeepSeek Coder V2 Local", context_window: 131072, max_output_tokens: 8192 },
      "llama-3.3-70b": { id: "llama-3.3-70b", name: "Llama 3.3 70B Local", context_window: 131072, max_output_tokens: 8192 },
      "gemma3-27b": { id: "gemma3-27b", name: "Gemma 3 27B Local", context_window: 131072, max_output_tokens: 8192 }
    }
  }
};

export const MATRIX_SCHEMES = [
  {
    id: "flagship_hybrid",
    zh: "✨ 旗舰异构混合方案 (推荐)",
    en: "✨ Flagship Heterogeneous Mix",
    desc: "Lead/Debug 用 Claude Opus，编码用 Sonnet，盲审用异构 GPT-5.5，侦察用极速 Haiku",
    roles: {
      lead: { provider: "anthropic", model: "claude-opus-5-5", effort: "xhigh" },
      coder: { provider: "anthropic", model: "claude-sonnet-5-5", effort: "high" },
      verifier: { provider: "openai", model: "gpt-5.5", effort: "high" },
      explorer: { provider: "anthropic", model: "claude-haiku-5-5", effort: "low" },
      debugger: { provider: "anthropic", model: "claude-opus-5-5", effort: "max" },
      dreamer: { provider: "anthropic", model: "claude-sonnet-5-5", effort: "medium" },
      utility: { provider: "anthropic", model: "claude-haiku-5-5", effort: "low" }
    }
  },
  {
    id: "all_anthropic",
    zh: "🟣 全 Claude 家族方案",
    en: "🟣 All-Anthropic Claude Suite",
    desc: "全角色统一使用 Anthropic Claude Opus 5.5 / Sonnet 5.5 / Haiku 5.5，只需一个 API Key",
    roles: {
      lead: { provider: "anthropic", model: "claude-opus-5-5", effort: "xhigh" },
      coder: { provider: "anthropic", model: "claude-sonnet-5-5", effort: "high" },
      verifier: { provider: "anthropic", model: "claude-sonnet-5-5", effort: "high" },
      explorer: { provider: "anthropic", model: "claude-haiku-5-5", effort: "low" },
      debugger: { provider: "anthropic", model: "claude-opus-5-5", effort: "max" },
      dreamer: { provider: "anthropic", model: "claude-sonnet-5-5", effort: "medium" },
      utility: { provider: "anthropic", model: "claude-haiku-5-5", effort: "low" }
    }
  },
  {
    id: "all_openai",
    zh: "🟢 全 OpenAI GPT/o 系列方案",
    en: "🟢 All-OpenAI GPT Suite",
    desc: "全角色切换至 OpenAI GPT-5.5 / GPT-5.4 / Mini，适合仅配置了 OPENAI_API_KEY 的环境",
    ensureProvider: "openai",
    roles: {
      lead: { provider: "openai", model: "gpt-5.5", effort: "xhigh" },
      coder: { provider: "openai", model: "gpt-5.5", effort: "high" },
      verifier: { provider: "openai", model: "gpt-5.5", effort: "high" },
      explorer: { provider: "openai", model: "gpt-5.4-mini", effort: "low" },
      debugger: { provider: "openai", model: "gpt-5.5", effort: "max" },
      dreamer: { provider: "openai", model: "gpt-5.4", effort: "medium" },
      utility: { provider: "openai", model: "gpt-5.4-mini", effort: "low" }
    }
  },
  {
    id: "all_gemini",
    zh: "🔵 全 Google Gemini 方案",
    en: "🔵 All-Google Gemini Suite",
    desc: "全角色使用 1M 超长上下文的 Gemini 2.5 Pro 与 Flash，适合超大型代码仓",
    ensureProvider: "gemini",
    roles: {
      lead: { provider: "gemini", model: "gemini-2.5-pro", effort: "xhigh" },
      coder: { provider: "gemini", model: "gemini-2.5-pro", effort: "high" },
      verifier: { provider: "gemini", model: "gemini-2.5-pro", effort: "high" },
      explorer: { provider: "gemini", model: "gemini-2.5-flash", effort: "low" },
      debugger: { provider: "gemini", model: "gemini-2.5-pro", effort: "max" },
      dreamer: { provider: "gemini", model: "gemini-2.5-flash", effort: "medium" },
      utility: { provider: "gemini", model: "gemini-2.5-flash", effort: "low" }
    }
  },
  {
    id: "local_gateway",
    zh: "🏠 本机/内网网关零配置方案",
    en: "🏠 Local Gateway / Ollama Suite",
    desc: "一键绑定本机 localhost:11211 网关或 Ollama/MLX，无需外网 Key 即可离线运行",
    ensureProvider: "local_mlx",
    roles: {
      lead: { provider: "local_mlx", model: "qwen3-coder", effort: "high" },
      coder: { provider: "local_mlx", model: "qwen3-coder", effort: "high" },
      verifier: { provider: "local_mlx", model: "qwen3-coder", effort: "high" },
      explorer: { provider: "local_mlx", model: "qwen3-coder", effort: "low" },
      debugger: { provider: "local_mlx", model: "qwen3-coder", effort: "high" },
      dreamer: { provider: "local_mlx", model: "qwen3-coder", effort: "medium" },
      utility: { provider: "local_mlx", model: "qwen3-coder", effort: "low" }
    }
  }
];

const CORE_TOOLS = [
  { id: "read_file", icon: "📄", zh: "读取文件 (read_file)", en: "read_file" },
  { id: "search_code", icon: "🔍", zh: "搜索代码 (search_code)", en: "search_code" },
  { id: "edit_file", icon: "✏️", zh: "修改文件 (edit_file)", en: "edit_file" },
  { id: "write_file", icon: "📝", zh: "新建/覆写 (write_file)", en: "write_file" },
  { id: "run_command", icon: "⚡", zh: "沙箱命令 (run_command)", en: "run_command" }
];

const ROLE_META = {
  lead: {
    zh: "首席架构师 (Lead Architect)",
    en: "Lead Architect",
    desc: "负责复杂任务拆解、多智能体派发与结果综合",
    defaultProvider: "anthropic",
    defaultModel: "claude-opus-5-5",
    defaultEffort: "xhigh",
    readOnly: false,
    allowedTools: null,
    prompt: "You are the Lead Architect agent in Gorness. Analyze repository context carefully, decompose complex goals into verifiable tasks, maintain architectural coherence, and synthesize specialist outputs accurately."
  },
  coder: {
    zh: "主力研发工程师 (Software Engineer)",
    en: "Software Engineer",
    desc: "负责编写高内聚、无竞态的生产级代码并运行单元测试",
    defaultProvider: "anthropic",
    defaultModel: "claude-sonnet-5-5",
    defaultEffort: "high",
    readOnly: false,
    allowedTools: null,
    prompt: "You are the primary Software Engineer agent in Gorness. Write concise, idiomatic, race-free code, preserve surrounding style, and verify every change with targeted tests."
  },
  verifier: {
    zh: "异构对抗盲审官 (Adversarial Verifier)",
    en: "Adversarial Verifier",
    desc: "只读独立审查 Diff、安全逃逸边界与测试覆盖，拒绝盲目采信",
    defaultProvider: "openai",
    defaultModel: "gpt-5.5",
    defaultEffort: "high",
    readOnly: true,
    allowedTools: ["read_file", "search_code", "run_command"],
    prompt: "You are the Adversarial Verifier agent in Gorness. Independently audit diffs, edge cases, security boundaries, and test coverage without trusting implementation claims at face value."
  },
  explorer: {
    zh: "极速代码侦察员 (Fast Explorer)",
    en: "Fast Explorer",
    desc: "高并发只读扫描代码符号、调用链与目录结构",
    defaultProvider: "anthropic",
    defaultModel: "claude-haiku-5-5",
    defaultEffort: "low",
    readOnly: true,
    allowedTools: ["read_file", "search_code"],
    prompt: "You are the Fast Explorer agent in Gorness. Quickly locate relevant files, call paths, symbols, and invariants, returning crisp file-and-line grounded findings."
  },
  debugger: {
    zh: "根因排错专家 (Root-Cause Debugger)",
    en: "Root-Cause Debugger",
    desc: "专注死锁、竞态条件、疑难崩溃复现与根因修复",
    defaultProvider: "anthropic",
    defaultModel: "claude-opus-5-5",
    defaultEffort: "max",
    readOnly: false,
    allowedTools: null,
    prompt: "You are the Root-Cause Debugger agent in Gorness. Formulate falsifiable hypotheses, reproduce failures first, trace state transitions systematically, and fix root causes rather than symptoms."
  },
  dreamer: {
    zh: "记忆与技能提炼师 (Memory Dreamer)",
    en: "Memory & Skill Dreamer",
    desc: "空闲期自动归纳排错经验与项目规范，生成候选 SKILL.md",
    defaultProvider: "anthropic",
    defaultModel: "claude-sonnet-5-5",
    defaultEffort: "medium",
    readOnly: false,
    allowedTools: null,
    prompt: "You are the Dreamer agent in Gorness. Distill recurring workflows, debugging lessons, and project invariants into concise pointer notes and reusable candidate SKILL.md definitions."
  },
  utility: {
    zh: "轻量内务助手 (Housekeeping Utility)",
    en: "Housekeeping Utility",
    desc: "零工具纯文本角色，负责自动生成会话标题与锚定增量压缩摘要",
    defaultProvider: "anthropic",
    defaultModel: "claude-haiku-5-5",
    defaultEffort: "low",
    readOnly: true,
    allowedTools: [],
    prompt: "You are the Housekeeping Utility agent in Gorness. Produce crisp session titles and faithful anchored summaries preserving exact file paths, decisions, and unresolved tasks."
  }
};

const SPECIALIST_ROLE_PRESETS = {
  security_auditor: {
    id: "security_auditor",
    name: "Security Auditor",
    zh: "🛡️ 安全审计专家 (security_auditor)",
    description: "Audits path traversal, command injection, secret leakage, and privilege boundaries.",
    provider: "anthropic",
    model: "claude-opus-5-5",
    effort: "xhigh",
    read_only: true,
    allowed_tools: ["read_file", "search_code", "run_command"],
    system_prompt: "You are the Security Auditor specialist in Gorness. Focus on command injection, symlink escapes, SSRF, auth bypasses, and secret redaction."
  },
  perf_profiler: {
    id: "perf_profiler",
    name: "Performance Profiler",
    zh: "⚡ 性能调优专家 (perf_profiler)",
    description: "Benchmarks hot paths, reduces heap allocations, and eliminates lock contention.",
    provider: "anthropic",
    model: "claude-sonnet-5-5",
    effort: "high",
    read_only: false,
    allowed_tools: null,
    system_prompt: "You are the Performance Profiler specialist in Gorness. Measure with benchmarks first, minimize allocations, and avoid premature complexity."
  },
  frontend_architect: {
    id: "frontend_architect",
    name: "Frontend UI Architect",
    zh: "🎨 Apple HIG 前端架构师 (frontend_architect)",
    description: "Crafts native-grade Cupertino interfaces, accessibility, and zero-XSS DOM state.",
    provider: "anthropic",
    model: "claude-sonnet-5-5",
    effort: "high",
    read_only: false,
    allowed_tools: null,
    system_prompt: "You are the Apple HIG Frontend Architect in Gorness. Design intuitive, zero-typing interactive controls with clean CSS and strict XSS escaping."
  },
  test_engineer: {
    id: "test_engineer",
    name: "TDD Test Engineer",
    zh: "🧪 自动化测试专家 (test_engineer)",
    description: "Writes deterministic table-driven unit, race, and integration tests.",
    provider: "anthropic",
    model: "claude-sonnet-5-5",
    effort: "high",
    read_only: false,
    allowed_tools: null,
    system_prompt: "You are the TDD Test Engineer in Gorness. Write isolated, race-free tests using temp directories and mock servers."
  }
};

const PROMPT_TEMPLATES = [
  { id: "lead", label: "🏗️ 首席架构师拆解与协调模板", text: ROLE_META.lead.prompt },
  { id: "coder", label: "💻 生产级精简无竞态编码模板", text: ROLE_META.coder.prompt },
  { id: "verifier", label: "🔍 零信任异构对抗盲审模板", text: ROLE_META.verifier.prompt },
  { id: "explorer", label: "⚡ 极速只读代码侦察模板", text: ROLE_META.explorer.prompt },
  { id: "debugger", label: "🩺 假设驱动根因排错模板", text: ROLE_META.debugger.prompt },
  { id: "dreamer", label: "🧠 经验归纳与技能提炼模板", text: ROLE_META.dreamer.prompt },
  { id: "utility", label: "🗂️ 会话标题与锚定压缩模板", text: ROLE_META.utility.prompt }
];

const COMMON_DOMAINS = [
  "api.anthropic.com",
  "api.openai.com",
  "generativelanguage.googleapis.com",
  "api.deepseek.com",
  "openrouter.ai",
  "dashscope.aliyuncs.com",
  "ark.cn-beijing.volces.com",
  "api.moonshot.cn",
  "github.com",
  "api.github.com",
  "proxy.golang.org",
  "sum.golang.org",
  "pkg.go.dev",
  "registry.npmjs.org",
  "pypi.org",
  "crates.io"
];

const COMMON_ALLOW_CMDS = [
  "go test*",
  "go vet*",
  "go build*",
  "git status*",
  "git diff*",
  "git log*",
  "npm test*",
  "pnpm test*",
  "pytest*",
  "cargo test*",
  "node --check*"
];

const COMMON_ASK_CMDS = [
  "git push*",
  "git reset --hard*",
  "git clean -fd*",
  "curl *",
  "rm -rf *",
  "docker *",
  "brew *"
];

const COMMON_DENY_CMDS = [
  "rm -rf /",
  "rm -rf /*",
  "mkfs*",
  "dd if=* of=/dev/*",
  "sudo rm -rf*"
];

const MCP_SERVER_PRESETS = [
  {
    id: "filesystem",
    zh: "📁 Filesystem 本地文件服务",
    desc: "@modelcontextprotocol/server-filesystem",
    transport: "stdio",
    command: "npx",
    args: ["-y", "@modelcontextprotocol/server-filesystem", "."],
    timeout_sec: 60
  },
  {
    id: "git",
    zh: "🌿 Git 仓库分析服务",
    desc: "mcp-server-git (uvx)",
    transport: "stdio",
    command: "uvx",
    args: ["mcp-server-git"],
    timeout_sec: 60
  },
  {
    id: "fetch",
    zh: "🌐 Web Fetch 网页抓取服务",
    desc: "mcp-server-fetch (uvx)",
    transport: "stdio",
    command: "uvx",
    args: ["mcp-server-fetch"],
    timeout_sec: 60
  },
  {
    id: "memory",
    zh: "🧠 Knowledge Graph 知识图谱",
    desc: "@modelcontextprotocol/server-memory",
    transport: "stdio",
    command: "npx",
    args: ["-y", "@modelcontextprotocol/server-memory"],
    timeout_sec: 60
  },
  {
    id: "sequential_thinking",
    zh: "🔗 Sequential Thinking 链式推演",
    desc: "@modelcontextprotocol/server-sequential-thinking",
    transport: "stdio",
    command: "npx",
    args: ["-y", "@modelcontextprotocol/server-sequential-thinking"],
    timeout_sec: 60
  },
  {
    id: "playwright",
    zh: "🎭 Playwright 浏览器自动化",
    desc: "@playwright/mcp@latest",
    transport: "stdio",
    command: "npx",
    args: ["-y", "@playwright/mcp@latest"],
    timeout_sec: 120
  }
];

export function escapeBidi(raw) {
  const str = String(raw ?? "");
  let out = "";
  for (const ch of str) {
    const cp = ch.codePointAt(0);
    if (
      cp === 0x061c ||
      cp === 0x200e ||
      cp === 0x200f ||
      (cp >= 0x202a && cp <= 0x202e) ||
      (cp >= 0x2066 && cp <= 0x2069)
    ) {
      out += `<U+${cp.toString(16).toUpperCase().padStart(4, "0")}>`;
    } else {
      out += ch;
    }
  }
  return out;
}

export function esc(raw) {
  return escapeBidi(raw)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function formatJSONSafe(val) {
  if (val == null) return "{}";
  if (typeof val === "string") {
    try {
      return JSON.stringify(JSON.parse(val), null, 2);
    } catch {
      return val;
    }
  }
  try {
    return JSON.stringify(val, null, 2);
  } catch {
    return String(val);
  }
}

export const state = {
  lang: (typeof localStorage !== "undefined" && localStorage.getItem("gorness.lang")) || "zh",
  theme: (typeof localStorage !== "undefined" && localStorage.getItem("gorness.theme")) || "auto",
  centerMode: "chat",
  configPage: "matrix",
  selectedConfigWorkflowId: "hybrid-fleet",
  wsSuggestions: [],
  inspectorTab: "files",
  config: null,
  configFiles: [],
  diagnostic: null,
  lastRunError: "",
  workspaces: [],
  sessions: [],
  activeWorkspaceId: "",
  activeSessionId: "",
  sessionLoaded: false,
  sessionEntries: [],
  sessionSnapshots: [],
  activeRuns: [],
  pendingApprovals: [],
  approvalSurfacedClientMs: new Map(),
  approvalSelectedHunks: new Map(),
  alwaysOn: { inbox: [], timers: [], deferred: [], outbox: [] },
  missions: [],
  activeMissionId: "",
  selectedNodeId: "",
  nodeLogs: new Map(),
  nodeDiffText: "",
  selectedLaunchTopology: "hybrid-fleet",
  workspaceFiles: [],
  workspaceSkills: [],
  viewedFileRel: "",
  previews: [],
  drafts: {},
  draftTimer: null,
  isComposingIME: false,
  liveStreamThinking: "",
  liveStreamText: "",
  liveStreamTools: [],
  missionSSE: null,
  missionSSEId: "",
  globalSSE: null
};

function t(key) {
  return (I18N[state.lang] && I18N[state.lang][key]) || I18N.en[key] || key;
}

async function apiJSON(path, opts = {}) {
  const headers = Object.assign({ "Content-Type": "application/json" }, opts.headers || {});
  const resp = await fetch(path, Object.assign({}, opts, { headers }));
  const text = await resp.text();
  let data = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = { raw: text };
    }
  }
  if (!resp.ok) {
    const err = new Error((data && data.error) || `HTTP ${resp.status}`);
    err.status = resp.status;
    err.code = data && data.code;
    throw err;
  }
  return data;
}

function applyTheme(themeVal, persistServer = false) {
  state.theme = themeVal;
  localStorage.setItem("gorness.theme", themeVal);
  document.documentElement.setAttribute("data-theme", themeVal);
  document.querySelectorAll(".theme-btn").forEach((btn) => {
    btn.classList.toggle("active", btn.dataset.themeVal === themeVal);
  });
  if (persistServer) {
    apiJSON("/api/config/global", {
      method: "POST",
      body: JSON.stringify({ theme: themeVal })
    }).catch(() => {});
  }
}

function applyI18nInPlace() {
  document.documentElement.lang = state.lang === "zh" ? "zh-CN" : "en";
  document.querySelectorAll("[data-i18n]").forEach((el) => {
    const key = el.getAttribute("data-i18n");
    if (key) el.textContent = t(key);
  });
  document.querySelectorAll("[data-i18n-placeholder]").forEach((el) => {
    const key = el.getAttribute("data-i18n-placeholder");
    if (key && !(document.activeElement === el && state.isComposingIME)) {
      el.setAttribute("placeholder", t(key));
    }
  });
  syncSendOrSteerButtonLabel();
  renderTopologyCardsGrid();
}

export function openSettingsModal(page = "") {
  if (page) state.configPage = page;
  document.getElementById("config-mode-view")?.classList.remove("hidden");
  renderConfigStudio();
}

globalThis.openGornessSettingsModal = (page) => openSettingsModal(page);

export function setCenterMode(mode) {
  if (mode === "config") {
    openSettingsModal();
    return;
  }
  state.centerMode = mode;
  document.getElementById("mode-chat-btn")?.classList.toggle("active", mode === "chat");
  document.getElementById("mode-fleet-btn")?.classList.toggle("active", mode === "fleet");
  document.getElementById("mode-config-btn")?.classList.remove("active");
  document.getElementById("chat-mode-view")?.classList.toggle("hidden", mode !== "chat");
  document.getElementById("fleet-mode-view")?.classList.toggle("hidden", mode !== "fleet");
  document.getElementById("config-mode-view")?.classList.add("hidden");
  if (mode === "fleet") {
    renderFleetMissionSelector();
    renderFleetStudio();
    const snap = activeMissionSnapshot();
    if (snap && snap.status === "running") {
      subscribeToMissionStream(snap.id);
    }
  }
}

function setInspectorTab(tab) {
  state.inspectorTab = tab;
  document.querySelectorAll(".insp-tab").forEach((btn) => {
    btn.classList.toggle("active", btn.dataset.tab === tab);
  });
  ["files", "snapshots", "config", "preview"].forEach((id) => {
    document.getElementById(`insp-panel-${id}`)?.classList.toggle("hidden", id !== tab);
  });
}

function openModal(id) {
  document.getElementById(id)?.classList.remove("hidden");
}

function closeModal(id) {
  document.getElementById(id)?.classList.add("hidden");
}

function activeRunForCurrentSession() {
  if (!state.activeSessionId) return null;
  return state.activeRuns.find((r) => r.session_id === state.activeSessionId) || null;
}

function syncSendOrSteerButtonLabel() {
  const btn = document.getElementById("send-or-steer-btn");
  const stopBtn = document.getElementById("stop-run-btn");
  const activeRun = activeRunForCurrentSession();
  if (btn) btn.textContent = activeRun ? t("steerBtn") : t("sendBtn");
  if (stopBtn) stopBtn.classList.toggle("hidden", !activeRun);
}

// ============================================================================
// Bootstrap & Global Live Event Stream
// ============================================================================

export async function loadBootstrap() {
  const data = await apiJSON("/api/bootstrap");
  state.config = data.config || null;
  state.configFiles = data.config_files || [];
  state.diagnostic = data.diagnostic || null;
  state.workspaces = data.workspaces || [];
  state.sessions = data.sessions || [];
  state.activeRuns = data.active_runs || [];
  state.pendingApprovals = data.pending_approvals || [];
  state.alwaysOn = data.alwayson || { inbox: [], timers: [], deferred: [], outbox: [] };
  state.missions = data.missions || [];
  state.previews = data.previews || [];

  for (const appr of state.pendingApprovals) noteApprovalSurfaced(appr);

  const vaultLabel = document.getElementById("vault-path-label");
  if (vaultLabel && data.vault_root) vaultLabel.textContent = data.vault_root;

  if (!state.activeWorkspaceId && state.workspaces.length > 0) {
    state.activeWorkspaceId = state.workspaces[0].id;
  }
  let targetSessionId = state.activeSessionId;
  if (!targetSessionId && state.sessions.length > 0) {
    const wsSess = state.sessions.find((s) => s.workspace_id === state.activeWorkspaceId && !s.archived) || state.sessions[0];
    if (wsSess) targetSessionId = wsSess.id;
  }
  if (!state.activeMissionId && state.missions.length > 0) {
    state.activeMissionId = state.missions[0].id;
  }

  renderDiagnosticBanner();
  populateAgentRoleSelect();
  renderAttentionQueue();
  renderSidebarTree();
  renderAlwaysOnIndicators();
  renderAgentMatrixAndVaultEditor();
  renderPreviewTabList();
  renderFleetMissionSelector();
  renderFleetStudio();
  const bootMission = activeMissionSnapshot();
  if (bootMission && bootMission.status === "running") {
    subscribeToMissionStream(bootMission.id);
  }

  if (state.activeWorkspaceId) {
    refreshWorkspaceContext(state.activeWorkspaceId);
  }
  if (targetSessionId) {
    await selectSession(targetSessionId, false);
  } else {
    renderChatHistory();
  }
}

function connectGlobalEventStream() {
  if (state.globalSSE) state.globalSSE.close();
  const es = new EventSource("/api/events/stream");
  state.globalSSE = es;

  const onEvent = (raw) => {
    let ev;
    try {
      ev = JSON.parse(raw.data);
    } catch {
      return;
    }
    if (!ev || ev.type === "heartbeat" || ev.type === "connected") return;

    switch (ev.type) {
      case "run_started":
        refreshActiveStatusQuietly();
        break;
      case "approval_required":
        if (ev.payload && ev.payload.id) {
          noteApprovalSurfaced(ev.payload);
          const idx = state.pendingApprovals.findIndex((a) => a.id === ev.payload.id);
          if (idx >= 0) state.pendingApprovals[idx] = ev.payload;
          else state.pendingApprovals.push(ev.payload);
          renderAttentionQueue();
          renderApprovalBanners();
        }
        break;
      case "approval_resolved":
        if (ev.payload && ev.payload.approval_id) {
          state.pendingApprovals = state.pendingApprovals.filter((a) => a.id !== ev.payload.approval_id);
          renderAttentionQueue();
          renderApprovalBanners();
        }
        break;
      case "title_updated":
        if (ev.payload && ev.payload.session_id) {
          const s = state.sessions.find((x) => x.id === ev.payload.session_id);
          if (s) s.title = ev.payload.title;
          if (state.activeSessionId === ev.payload.session_id) {
            const titleIn = document.getElementById("session-title-input");
            if (titleIn && document.activeElement !== titleIn) titleIn.value = ev.payload.title;
          }
          renderSidebarTree();
        }
        break;
      case "compacted":
      case "run_completed":
      case "run_failed":
      case "run_cancelled":
        if (ev.type === "run_failed" && (!ev.session_id || ev.session_id === state.activeSessionId)) {
          const msg = (ev.payload && (ev.payload.message || ev.payload.error)) || "Agent run failed";
          const code = ev.payload && ev.payload.code ? `[${ev.payload.code}] ` : "";
          surfaceRunError(`${code}${msg}`);
        }
        refreshActiveStatusQuietly();
        if (ev.session_id && ev.session_id === state.activeSessionId) {
          loadSessionDetail(state.activeSessionId);
        }
        break;
    }
  };

  [
    "run_started",
    "approval_required",
    "approval_resolved",
    "title_updated",
    "compacted",
    "run_completed",
    "run_failed",
    "run_cancelled"
  ].forEach((name) => es.addEventListener(name, onEvent));
}

async function refreshActiveStatusQuietly() {
  try {
    const data = await apiJSON("/api/bootstrap");
    state.workspaces = data.workspaces || [];
    state.sessions = data.sessions || [];
    state.activeRuns = data.active_runs || [];
    state.pendingApprovals = data.pending_approvals || [];
    state.alwaysOn = data.alwayson || state.alwaysOn;
    state.missions = data.missions || state.missions;
    state.diagnostic = data.diagnostic || null;
    for (const appr of state.pendingApprovals) noteApprovalSurfaced(appr);
    renderDiagnosticBanner();
    renderAttentionQueue();
    renderSidebarTree();
    renderApprovalBanners();
    renderAlwaysOnIndicators();
    syncSendOrSteerButtonLabel();
  } catch {
    // Ignore transient poll errors
  }
}

function surfaceRunError(errMsg) {
  state.lastRunError = String(errMsg || "").trim();
  renderDiagnosticBanner();
  renderChatHistory();
}

function clearRunError() {
  if (!state.lastRunError) return;
  state.lastRunError = "";
  renderDiagnosticBanner();
}

function renderDiagnosticBanner() {
  const banner = document.getElementById("reload-diag-banner");
  if (!banner) return;
  const parts = [];
  if (state.diagnostic && state.diagnostic.message) {
    parts.push(`Reload Warning (${escapeBidi(state.diagnostic.file || "vault")}): ${escapeBidi(state.diagnostic.message)}`);
  }
  if (state.lastRunError) {
    parts.push(`${t("runErrorTitle")}: ${escapeBidi(state.lastRunError)}`);
  }
  if (parts.length === 0) {
    banner.classList.add("hidden");
    banner.textContent = "";
    return;
  }
  banner.classList.remove("hidden");
  banner.textContent = parts.join("  ·  ");
}

// ============================================================================
// Left Pane: Attention Queue & Grouped Sessions
// ============================================================================

function renderAttentionQueue() {
  const sec = document.getElementById("attention-section");
  const badge = document.getElementById("attention-badge");
  const listEl = document.getElementById("attention-list");
  if (!sec || !badge || !listEl) return;

  const deferred = (state.alwaysOn && state.alwaysOn.deferred) || [];
  const total = state.pendingApprovals.length + deferred.length;
  badge.textContent = String(total);
  sec.classList.toggle("hidden", total === 0);
  if (total === 0) {
    listEl.innerHTML = "";
    return;
  }

  let html = "";
  for (const appr of state.pendingApprovals) {
    html += `<div class="attention-card" data-jump-session="${esc(appr.session_id)}">
      <div><strong>${esc(appr.tool_name)}</strong> · <span class="mono">${esc(appr.reason_code)}</span></div>
      <div class="search-hit-snippet">${esc(appr.reason)}</div>
    </div>`;
  }
  for (const card of deferred) {
    html += `<div class="attention-card" data-jump-session="${esc(card.session_id)}">
      <div><strong>[Deferred] ${esc(card.tool_name)}</strong></div>
      <div class="search-hit-snippet">${esc(card.reason)}</div>
      <div class="row-actions" style="margin-top:4px;">
        <button class="primary-xs-btn" type="button" data-resolve-deferred="${esc(card.id)}" data-approve="true">Approve</button>
        <button class="danger-xs-btn" type="button" data-resolve-deferred="${esc(card.id)}" data-approve="false">Deny</button>
      </div>
    </div>`;
  }
  listEl.innerHTML = html;
}

function renderSidebarTree() {
  const container = document.getElementById("workspace-tree");
  if (!container) return;

  if (state.workspaces.length === 0) {
    container.innerHTML = `<div style="padding:14px 10px;color:var(--ink-secondary);font-size:12px;">${esc(t("emptyChatHint"))}</div>`;
    return;
  }

  const runningBySession = new Set(state.activeRuns.map((r) => r.session_id));
  const runningByWS = new Map();
  for (const r of state.activeRuns) {
    runningByWS.set(r.workspace_id, (runningByWS.get(r.workspace_id) || 0) + 1);
  }
  for (const m of state.missions) {
    if (m.status === "running") {
      runningByWS.set(m.workspace_id, (runningByWS.get(m.workspace_id) || 0) + 1);
    }
  }

  let html = "";
  for (const ws of state.workspaces) {
    const isActiveWS = ws.id === state.activeWorkspaceId;
    const wsSessions = state.sessions.filter((s) => s.workspace_id === ws.id && !s.archived);
    const activeCount = runningByWS.get(ws.id) || 0;

    html += `<div class="ws-group">
      <div class="ws-group-header ${isActiveWS ? "active-ws" : ""}" data-select-ws="${esc(ws.id)}">
        <span>${activeCount > 0 ? `<span class="pulse-dot live"></span> ` : ""}${esc(ws.name)}</span>
        <span class="meta-chip mono">${wsSessions.length}</span>
      </div>`;

    for (const sess of wsSessions) {
      const isSelected = sess.id === state.activeSessionId;
      const isRunning = runningBySession.has(sess.id);
      html += `<div class="session-item ${isSelected ? "selected" : ""}" data-select-session="${esc(sess.id)}">
        <div class="session-item-main">
          <div class="session-item-title">${isRunning ? `<span class="pulse-dot live"></span> ` : ""}${esc(sess.title)}</div>
          <div class="session-item-sub mono">
            <span>${esc(sess.agent_role || "coder")}</span>
            <span>#${Number(sess.head_seq || 0)}</span>
          </div>
        </div>
        <button class="session-pin-btn ${sess.pinned ? "pinned" : ""}" type="button" data-pin-session="${esc(sess.id)}" data-pinned="${sess.pinned ? "1" : "0"}">★</button>
      </div>`;
    }
    html += `</div>`;
  }
  container.innerHTML = html;
}

// ============================================================================
// Mode A: Streaming Conversation & 500ms Settle Guard Approvals
// ============================================================================

function populateAgentRoleSelect() {
  const sel = document.getElementById("agent-role-select");
  if (sel && state.config && state.config.agents) {
    const currentVal = sel.value || "coder";
    const roles = Object.values(state.config.agents).sort((a, b) => a.id.localeCompare(b.id));
    sel.innerHTML = roles
      .map((a) => {
        const meta = ROLE_META[a.id];
        const title = meta ? (state.lang === "zh" ? meta.zh.split(" (")[0] : meta.en) : (a.name || a.id);
        return `<option value="${esc(a.id)}">${esc(a.id)} · ${esc(title)} (${esc(a.model)})</option>`;
      })
      .join("");
    sel.value = roles.some((r) => r.id === currentVal) ? currentVal : "coder";
  }

  const modSel = document.getElementById("model-override-input");
  if (modSel && state.config && state.config.providers) {
    const prevVal = modSel.value || "";
    const provs = Object.values(state.config.providers).sort((a, b) => a.id.localeCompare(b.id));
    let html = `<option value="">${esc(t("modelOverridePh"))}</option>`;
    for (const p of provs) {
      const mList = getModelsForProvider(p.id);
      if (mList.length === 0) continue;
      html += `<optgroup label="${esc(p.name || p.id)} (${esc(p.protocol)})">`;
      for (const m of mList) {
        const ref = `${p.id}/${m.id}`;
        html += `<option value="${esc(ref)}" ${ref === prevVal ? "selected" : ""}>${esc(ref)}${m.supports_thinking ? " 🧠" : ""}</option>`;
      }
      html += `</optgroup>`;
    }
    modSel.innerHTML = html;
    modSel.value = prevVal;
  }
}

function saveDraftLocally() {
  if (!state.activeSessionId || !state.sessionLoaded) return;
  const composer = document.getElementById("composer-input");
  if (!composer) return;
  const val = composer.value;
  state.drafts[state.activeSessionId] = val;
  clearTimeout(state.draftTimer);
  const targetSession = state.activeSessionId;
  state.draftTimer = setTimeout(() => {
    apiJSON(`/api/sessions/${encodeURIComponent(targetSession)}`, {
      method: "PATCH",
      body: JSON.stringify({ draft: val })
    })
      .then(() => {
        const ind = document.getElementById("draft-saved-indicator");
        if (ind) ind.textContent = val ? t("draftSaved") : "";
      })
      .catch(() => {});
  }, 450);
}

function flushDraftImmediate(sessionId, val) {
  if (!sessionId) return;
  state.drafts[sessionId] = val;
  clearTimeout(state.draftTimer);
  apiJSON(`/api/sessions/${encodeURIComponent(sessionId)}`, {
    method: "PATCH",
    body: JSON.stringify({ draft: val })
  }).catch(() => {});
}

export async function selectSession(sessionId, switchMode = true) {
  if (state.sessionLoaded && state.activeSessionId && state.activeSessionId !== sessionId) {
    const prevComposer = document.getElementById("composer-input");
    if (prevComposer) {
      flushDraftImmediate(state.activeSessionId, prevComposer.value);
    }
  }
  state.activeSessionId = sessionId;
  state.sessionLoaded = false;
  state.lastRunError = "";
  state.liveStreamThinking = "";
  state.liveStreamText = "";
  state.liveStreamTools = [];
  renderDiagnosticBanner();

  const cachedMeta = state.sessions.find((s) => s.id === sessionId);
  if (cachedMeta && cachedMeta.workspace_id !== state.activeWorkspaceId) {
    state.activeWorkspaceId = cachedMeta.workspace_id;
    refreshWorkspaceContext(state.activeWorkspaceId);
  }
  if (switchMode && state.centerMode !== "chat") {
    setCenterMode("chat");
  }

  renderSidebarTree();
  const detailMeta = await loadSessionDetail(sessionId);

  const composer = document.getElementById("composer-input");
  if (composer && !state.isComposingIME) {
    const hasLocalDraft = Object.prototype.hasOwnProperty.call(state.drafts, sessionId);
    const serverDraft = (detailMeta && detailMeta.draft) ?? (cachedMeta && cachedMeta.draft) ?? "";
    composer.value = hasLocalDraft ? state.drafts[sessionId] : serverDraft;
    const ind = document.getElementById("draft-saved-indicator");
    if (ind) ind.textContent = composer.value ? t("draftSaved") : "";
  }
  state.sessionLoaded = true;
  syncSendOrSteerButtonLabel();
}

async function loadSessionDetail(sessionId) {
  const data = await apiJSON(`/api/sessions/${encodeURIComponent(sessionId)}`);
  const meta = data.meta;
  state.sessionEntries = data.entries || [];
  state.sessionSnapshots = data.snapshots || [];

  const idx = state.sessions.findIndex((s) => s.id === sessionId);
  if (idx >= 0 && meta) state.sessions[idx] = meta;

  const titleIn = document.getElementById("session-title-input");
  if (titleIn && meta && document.activeElement !== titleIn) {
    titleIn.value = meta.title || "New Session";
  }
  const seqBadge = document.getElementById("session-seq-badge");
  if (seqBadge && meta) seqBadge.textContent = `seq #${Number(meta.head_seq || 0)}`;
  const costBadge = document.getElementById("session-cost-badge");
  if (costBadge && meta) {
    costBadge.textContent = `${Number(meta.total_tokens || 0)} tok · $${Number(meta.total_cost_usd || 0).toFixed(4)}`;
  }
  const roleSel = document.getElementById("agent-role-select");
  if (roleSel && meta && meta.agent_role) roleSel.value = meta.agent_role;
  const modelIn = document.getElementById("model-override-input");
  if (modelIn && meta && document.activeElement !== modelIn) {
    modelIn.value = meta.model_override || "";
  }

  renderChatHistory();
  renderLiveStreamCard();
  renderApprovalBanners();
  renderSnapshotTimeMachine();
  return meta;
}

function parseSubagentDelegation(toolName, rawInput) {
  const name = String(toolName || "").trim();
  const lower = name.toLowerCase();
  let parsed = null;
  if (rawInput && typeof rawInput === "object") {
    parsed = rawInput;
  } else if (typeof rawInput === "string") {
    try {
      parsed = JSON.parse(rawInput);
    } catch {
      parsed = null;
    }
  }
  const isNamedSubagent =
    lower === "subagent" ||
    lower === "spawn_subagent" ||
    lower === "delegate_agent" ||
    lower === "dispatch_agent" ||
    lower === "run_subagent" ||
    lower === "task" ||
    lower.includes("subagent");
  const hasRoleField = Boolean(parsed && (parsed.agent_role || parsed.subagent_type || parsed.delegate_role));
  if (!isNamedSubagent && !hasRoleField) return null;

  const role =
    (parsed && (parsed.agent_role || parsed.subagent_type || parsed.delegate_role || parsed.role)) || "specialist";
  const model = (parsed && (parsed.model || parsed.model_override)) || resolveConfiguredRoleModel(role);
  const goal =
    (parsed && (parsed.prompt || parsed.goal || parsed.task || parsed.description)) || formatJSONSafe(rawInput);
  const childSessionId = (parsed && (parsed.session_id || parsed.child_session_id)) || "";
  return { name, role, model, goal: String(goal || ""), childSessionId };
}

function resolveConfiguredRoleModel(roleId) {
  if (state.config && state.config.agents && state.config.agents[roleId]) {
    const prof = state.config.agents[roleId];
    return `${prof.provider}/${prof.model}`;
  }
  return "";
}

function renderSubagentToolCardHTML(sub, pairedResult, openByDefault = false) {
  const isErr = Boolean(pairedResult && pairedResult.is_error);
  const statusLabel = pairedResult ? (isErr ? "ERROR" : "COMPLETED") : "RUNNING";
  const badgeText = sub.model ? `${sub.role} · ${sub.model}` : sub.role;
  const jumpBtn = sub.childSessionId
    ? `<button class="ghost-xs-btn" type="button" data-select-session="${esc(sub.childSessionId)}">${esc(t("jumpSessionBtn"))}</button>`
    : "";
  return `<details class="subagent-card ${isErr ? "is-error" : ""}" ${openByDefault ? "open" : ""}>
    <summary>
      <span class="subagent-summary-left">
        <span>↳ ${esc(t("subagentLabel"))}</span>
        <span class="subagent-role-pill">${esc(badgeText)}</span>
        <span class="mono" style="color:var(--ink-secondary);font-weight:400;">${esc(sub.goal.slice(0, 64))}</span>
      </span>
      <span class="row-actions">
        <span class="meta-chip mono">${esc(statusLabel)}</span>
        ${jumpBtn}
      </span>
    </summary>
    <div class="subagent-body">
      <div><strong>Task:</strong> ${esc(sub.goal)}</div>
      ${pairedResult ? `<pre class="mono" style="margin:0;white-space:pre-wrap;">${esc(pairedResult.text || pairedResult.output || "")}</pre>` : ""}
    </div>
  </details>`;
}

function renderForkedChildCardHTML(child) {
  const role = child.agent_role || "coder";
  const model = child.model_override || resolveConfiguredRoleModel(role);
  const badge = model ? `${role} · ${model}` : role;
  return `<details class="subagent-card" data-child-session-card="${esc(child.id)}">
    <summary>
      <span class="subagent-summary-left">
        <span>⑂ ${esc(t("forkedChildLabel"))}</span>
        <span class="subagent-role-pill">${esc(badge)}</span>
        <span>${esc(child.title || child.id)}</span>
      </span>
      <span class="row-actions">
        <span class="meta-chip mono">seq #${Number(child.forked_at_seq || 0)} → #${Number(child.head_seq || 0)}</span>
        <button class="ghost-xs-btn" type="button" data-select-session="${esc(child.id)}">${esc(t("jumpSessionBtn"))}</button>
      </span>
    </summary>
    <div class="subagent-body mono">
      <span>ID: ${esc(child.id)} · ${Number(child.total_tokens || 0)} tok · $${Number(child.total_cost_usd || 0).toFixed(4)}</span>
    </div>
  </details>`;
}

function renderWorkspaceFleetSubagentsHTML() {
  if (!state.activeWorkspaceId) return "";
  const wsMissions = (state.missions || []).filter((m) => m.workspace_id === state.activeWorkspaceId);
  if (wsMissions.length === 0) return "";

  return wsMissions
    .slice(0, 3)
    .map((msn) => {
      const workersHTML = (msn.nodes || [])
        .map((n) => {
          const jump = n.session_id
            ? `<button class="ghost-xs-btn" type="button" data-select-session="${esc(n.session_id)}">${esc(t("jumpSessionBtn"))}</button>`
            : "";
          return `<div class="subagent-worker-row">
            <span class="subagent-summary-left">
              <strong>${esc(n.label || n.id)}</strong>
              <span class="subagent-role-pill">${esc(n.role_id)} · ${esc(n.provider_id)}/${esc(n.model_id)}</span>
            </span>
            <span class="row-actions">
              <span class="meta-chip mono">${esc(n.status)} · ${Number(n.tokens || 0)}t</span>
              ${jump}
            </span>
          </div>`;
        })
        .join("");

      return `<details class="subagent-card fleet-subagents-card" ${msn.status === "running" ? "open" : ""}>
        <summary>
          <span class="subagent-summary-left">
            <span>❖ ${esc(t("fleetWorkersLabel"))}</span>
            <span class="subagent-role-pill">${esc(msn.topology)}</span>
            <span>${esc(String(msn.goal || "").slice(0, 56))}</span>
          </span>
          <span class="row-actions">
            <span class="status-pill ${esc(msn.status)}">${esc(msn.status)}</span>
            <button class="ghost-xs-btn" type="button" data-open-fleet-mission="${esc(msn.id)}">${esc(t("openFleetBtn"))}</button>
          </span>
        </summary>
        <div class="subagent-body">${workersHTML}</div>
      </details>`;
    })
    .join("");
}

export function renderChatHistory() {
  const host = document.getElementById("chat-history-entries");
  const timeline = document.getElementById("chat-timeline");
  if (!host || !timeline) return;

  const wasNearBottom = timeline.scrollHeight - timeline.scrollTop - timeline.clientHeight < 90;

  const toolResultsByCallId = new Map();
  for (const ent of state.sessionEntries || []) {
    for (const blk of ent.blocks || []) {
      if (blk.type === "tool_result" && blk.tool_call_id) {
        toolResultsByCallId.set(blk.tool_call_id, blk);
      }
    }
  }
  const consumedSubagentCallIds = new Set();

  const childSessions = state.activeSessionId
    ? (state.sessions || []).filter((s) => s.parent_session_id === state.activeSessionId)
    : [];
  const childrenBySeq = new Map();
  const entrySeqSet = new Set((state.sessionEntries || []).map((e) => Number(e.seq || 0)));
  const trailingChildren = [];
  for (const ch of childSessions) {
    const fSeq = Number(ch.forked_at_seq || 0);
    if (entrySeqSet.has(fSeq)) {
      if (!childrenBySeq.has(fSeq)) childrenBySeq.set(fSeq, []);
      childrenBySeq.get(fSeq).push(ch);
    } else {
      trailingChildren.push(ch);
    }
  }

  let html = "";
  if (!state.sessionEntries || state.sessionEntries.length === 0) {
    html += `<div class="msg-card"><div class="msg-text" style="color:var(--ink-secondary);">${esc(t("emptyChatHint"))}</div></div>`;
  } else {
    for (const entry of state.sessionEntries) {
      const roleCls = `role-${esc(entry.role || "assistant")}`;
      const metaRight = [entry.agent_role, entry.model_id, `seq #${Number(entry.seq || 0)}`]
        .filter(Boolean)
        .map(esc)
        .join(" · ");

      let bodyHTML = "";
      let renderedVisibleBlock = false;
      for (const blk of entry.blocks || []) {
        if (blk.type === "thinking" && blk.thinking) {
          renderedVisibleBlock = true;
          bodyHTML += `<details class="thinking-box"><summary>Adaptive Thinking</summary><pre>${esc(blk.thinking)}</pre></details>`;
        } else if (blk.type === "tool_use") {
          renderedVisibleBlock = true;
          const sub = parseSubagentDelegation(blk.tool_name, blk.tool_input);
          if (sub) {
            const paired = blk.tool_call_id ? toolResultsByCallId.get(blk.tool_call_id) : null;
            if (blk.tool_call_id && paired) consumedSubagentCallIds.add(blk.tool_call_id);
            bodyHTML += renderSubagentToolCardHTML(sub, paired, false);
          } else {
            bodyHTML += `<div class="tool-block">
              <div class="tool-block-head"><span>Tool Call · <code>${esc(blk.tool_name)}</code></span><span class="mono">${esc(blk.tool_call_id || "")}</span></div>
              <pre>${esc(formatJSONSafe(blk.tool_input))}</pre>
            </div>`;
          }
        } else if (blk.type === "tool_result") {
          if (blk.tool_call_id && consumedSubagentCallIds.has(blk.tool_call_id)) {
            continue;
          }
          renderedVisibleBlock = true;
          bodyHTML += `<div class="tool-block ${blk.is_error ? "is-error" : ""}">
            <div class="tool-block-head"><span>Tool Result · <code>${esc(blk.tool_name || blk.tool_call_id || "tool")}</code></span><span>${blk.is_error ? "ERROR" : "OK"}</span></div>
            <pre>${esc(blk.text || "")}</pre>
          </div>`;
        } else if (blk.text) {
          renderedVisibleBlock = true;
          bodyHTML += `<div class="msg-text">${esc(blk.text)}</div>`;
        }
      }

      const seqChildren = childrenBySeq.get(Number(entry.seq || 0)) || [];
      for (const ch of seqChildren) {
        renderedVisibleBlock = true;
        bodyHTML += renderForkedChildCardHTML(ch);
      }

      if (renderedVisibleBlock) {
        html += `<article class="msg-card ${roleCls}">
          <div class="msg-head">
            <span class="msg-role-badge">${esc(entry.role)}</span>
            <span class="mono">${metaRight}</span>
          </div>
          ${bodyHTML}
        </article>`;
      }
    }
  }

  for (const ch of trailingChildren) {
    html += `<article class="msg-card">${renderForkedChildCardHTML(ch)}</article>`;
  }

  const fleetSubHTML = renderWorkspaceFleetSubagentsHTML();
  if (fleetSubHTML) {
    html += `<article class="msg-card">${fleetSubHTML}</article>`;
  }

  if (state.lastRunError) {
    html += `<article class="msg-card chat-error-card" data-run-error-card="1">
      <div class="msg-head"><span class="msg-role-badge">${esc(t("runErrorTitle"))}</span></div>
      <div class="msg-text mono">${esc(state.lastRunError)}</div>
    </article>`;
  }

  host.innerHTML = html;
  if (wasNearBottom) timeline.scrollTop = timeline.scrollHeight;
}

function renderLiveStreamCard() {
  const liveHost = document.getElementById("chat-live-stream");
  const timeline = document.getElementById("chat-timeline");
  if (!liveHost || !timeline) return;

  if (!state.liveStreamThinking && !state.liveStreamText && state.liveStreamTools.length === 0) {
    liveHost.innerHTML = "";
    return;
  }
  const wasNearBottom = timeline.scrollHeight - timeline.scrollTop - timeline.clientHeight < 100;

  let inner = "";
  if (state.liveStreamThinking) {
    inner += `<details class="thinking-box" open><summary>Thinking (Streaming…)</summary><pre>${esc(state.liveStreamThinking)}</pre></details>`;
  }
  if (state.liveStreamText) {
    inner += `<div class="msg-text">${esc(state.liveStreamText)}</div>`;
  }
  for (const tc of state.liveStreamTools) {
    const sub = parseSubagentDelegation(tc.name, tc.rawArgs);
    if (sub) {
      inner += renderSubagentToolCardHTML(
        sub,
        tc.status !== "running" ? { is_error: tc.status === "error", text: tc.preview } : null,
        true
      );
    } else {
      inner += `<div class="tool-block">
        <div class="tool-block-head"><span>Running · <code>${esc(tc.name)}</code></span><span class="mono">${esc(tc.status)}</span></div>
        <pre>${esc(tc.preview || "")}</pre>
      </div>`;
    }
  }

  liveHost.innerHTML = `<article class="msg-card role-assistant">
    <div class="msg-head">
      <span class="msg-role-badge"><span class="pulse-dot live"></span> Streaming</span>
      <span class="mono">live</span>
    </div>
    ${inner}
  </article>`;

  if (wasNearBottom) timeline.scrollTop = timeline.scrollHeight;
}

function noteApprovalSurfaced(appr) {
  if (!appr || !appr.id) return;
  if (!state.approvalSurfacedClientMs.has(appr.id)) {
    state.approvalSurfacedClientMs.set(appr.id, Date.now());
  }
  if (!state.approvalSelectedHunks.has(appr.id)) {
    state.approvalSelectedHunks.set(appr.id, new Set((appr.hunks || []).map((h) => h.id)));
  }
}

function renderDiffPreviewHTML(previewText) {
  return String(previewText || "")
    .split("\n")
    .map((line) => {
      const safe = esc(line);
      if (line.startsWith("+") && !line.startsWith("+++")) return `<span class="diff-line-add">${safe}</span>`;
      if (line.startsWith("-") && !line.startsWith("---")) return `<span class="diff-line-del">${safe}</span>`;
      return `${safe}\n`;
    })
    .join("");
}

function renderApprovalBanners() {
  const host = document.getElementById("approval-banner-host");
  if (!host) return;

  const relevant = state.pendingApprovals.filter(
    (a) => !state.activeSessionId || a.session_id === state.activeSessionId
  );
  if (relevant.length === 0) {
    host.innerHTML = "";
    return;
  }

  const now = Date.now();
  let html = "";
  for (const appr of relevant) {
    noteApprovalSurfaced(appr);
    const surfacedMs = state.approvalSurfacedClientMs.get(appr.id) || now;
    const elapsed = Math.max(0, now - surfacedMs);
    const settled = elapsed >= SETTLE_GUARD_MS;
    const pct = Math.min(100, Math.round((elapsed / SETTLE_GUARD_MS) * 100));
    const selectedSet = state.approvalSelectedHunks.get(appr.id) || new Set();
    const hasHunks = Array.isArray(appr.hunks) && appr.hunks.length > 0;

    let hunksHTML = "";
    if (hasHunks) {
      for (const h of appr.hunks) {
        const checked = selectedSet.has(h.id) ? "checked" : "";
        hunksHTML += `<div class="hunk-box">
          <label class="hunk-head">
            <input type="checkbox" data-appr-hunk="${esc(appr.id)}" data-hunk-id="${esc(h.id)}" ${checked}>
            <span>${esc(h.id)} · ${esc(h.header)}</span>
          </label>
          <pre class="hunk-preview mono">${renderDiffPreviewHTML(h.preview)}</pre>
        </div>`;
      }
    } else {
      hunksHTML = `<pre class="hunk-preview mono">${esc(formatJSONSafe(appr.args))}</pre>`;
    }

    const disabledAttr = settled ? "" : "disabled";
    html += `<div class="approval-card">
      <div class="approval-head">
        <div><strong>${esc(appr.tool_name)}</strong> · <span class="meta-chip mono">${esc(appr.reason_code)}</span></div>
        <span class="meta-chip mono" data-settle-label="${esc(appr.id)}">${settled ? esc(t("settledReadyLabel")) : esc(t("settlingLabel"))}</span>
      </div>
      <div class="approval-reason">${esc(appr.reason)}</div>
      <div class="settle-bar-track"><div class="settle-bar-fill" data-settle-bar="${esc(appr.id)}" style="width:${pct}%"></div></div>
      ${hunksHTML}
      <div class="approval-actions">
        <input class="mac-input" type="text" data-appr-feedback="${esc(appr.id)}" placeholder="Feedback…">
        <div class="row-actions">
          <button class="primary-xs-btn" type="button" data-appr-resolve="${esc(appr.id)}" data-action="${hasHunks ? "approve_hunks" : "approve_all"}" ${disabledAttr}>${esc(t("approveHunksBtn"))}</button>
          <button class="ghost-xs-btn" type="button" data-appr-resolve="${esc(appr.id)}" data-action="approve_remember" ${disabledAttr}>${esc(t("approveRememberBtn"))}</button>
          <button class="danger-xs-btn" type="button" data-appr-resolve="${esc(appr.id)}" data-action="deny" ${disabledAttr}>${esc(t("denyBtn"))}</button>
        </div>
      </div>
    </div>`;
  }
  host.innerHTML = html;

  if (relevant.some((a) => now - (state.approvalSurfacedClientMs.get(a.id) || now) < SETTLE_GUARD_MS)) {
    setTimeout(updateSettleProgressInPlace, 55);
  }
}

function updateSettleProgressInPlace() {
  const now = Date.now();
  let stillSettling = false;
  for (const appr of state.pendingApprovals) {
    const surfaced = state.approvalSurfacedClientMs.get(appr.id) || now;
    const elapsed = now - surfaced;
    const settled = elapsed >= SETTLE_GUARD_MS;
    const pct = Math.min(100, Math.round((elapsed / SETTLE_GUARD_MS) * 100));
    const bar = document.querySelector(`[data-settle-bar="${CSS.escape(appr.id)}"]`);
    if (bar) bar.style.width = `${pct}%`;
    const lbl = document.querySelector(`[data-settle-label="${CSS.escape(appr.id)}"]`);
    if (lbl) lbl.textContent = settled ? t("settledReadyLabel") : t("settlingLabel");
    document.querySelectorAll(`[data-appr-resolve="${CSS.escape(appr.id)}"]`).forEach((btn) => {
      btn.disabled = !settled;
    });
    if (!settled) stillSettling = true;
  }
  if (stillSettling) setTimeout(updateSettleProgressInPlace, 55);
}

export async function sendPromptOrSteer() {
  const composer = document.getElementById("composer-input");
  if (!composer) return;
  const text = composer.value.trim();
  if (!text) return;

  clearRunError();

  if (text.startsWith("/")) {
    try {
      const handled = await tryExecuteSlashCommand(text);
      if (handled) {
        composer.value = "";
        saveDraftLocally();
        return;
      }
    } catch (err) {
      surfaceRunError(err.message || "Slash command failed");
      return;
    }
  }

  const activeRun = activeRunForCurrentSession();
  if (activeRun) {
    try {
      await apiJSON(`/api/runs/${encodeURIComponent(activeRun.run_id)}/steer`, {
        method: "POST",
        body: JSON.stringify({ message: text })
      });
      composer.value = "";
      saveDraftLocally();
    } catch (err) {
      surfaceRunError(err.message || "Failed to inject Steer");
    }
    return;
  }

  if (!state.activeWorkspaceId) {
    openModal("workspace-modal");
    return;
  }

  const role = document.getElementById("agent-role-select")?.value || "coder";
  const permMode = document.getElementById("perm-mode-select")?.value || "auto";
  const modelOverride = document.getElementById("model-override-input")?.value.trim() || "";

  let resp;
  try {
    resp = await fetch("/api/runs/stream", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        workspace_id: state.activeWorkspaceId,
        session_id: state.activeSessionId || "",
        prompt: text,
        agent_role: role,
        model_override: modelOverride,
        perm_mode: permMode
      })
    });
  } catch (netErr) {
    surfaceRunError(netErr.message || "Network error starting run");
    return;
  }

  if (!resp.ok || !resp.body) {
    let errMsg = `HTTP ${resp.status}`;
    try {
      const rawErr = await resp.text();
      if (rawErr) {
        const parsed = JSON.parse(rawErr);
        errMsg = (parsed && parsed.error) || rawErr;
      }
    } catch {
      // keep fallback HTTP status message
    }
    // Preserve composer.value & persisted draft when /api/runs/stream fails
    surfaceRunError(errMsg);
    return;
  }

  // Stream established cleanly — now clear composer and persist empty draft
  composer.value = "";
  saveDraftLocally();
  state.liveStreamThinking = "";
  state.liveStreamText = "";
  state.liveStreamTools = [];

  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buf = "";

  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    let idx;
    while ((idx = buf.indexOf("\n\n")) >= 0) {
      const chunk = buf.slice(0, idx);
      buf = buf.slice(idx + 2);
      const dataLine = chunk.split("\n").find((l) => l.startsWith("data:"));
      if (!dataLine) continue;
      let ev;
      try {
        ev = JSON.parse(dataLine.slice(5).trim());
      } catch {
        continue;
      }
      if (!ev || ev.type === "heartbeat") continue;
      handleRunStreamEvent(ev);
    }
  }

  state.liveStreamThinking = "";
  state.liveStreamText = "";
  state.liveStreamTools = [];
  renderLiveStreamCard();
  if (state.activeSessionId) {
    await loadSessionDetail(state.activeSessionId);
    state.sessionLoaded = true;
  }
  await refreshActiveStatusQuietly();
}

export function handleRunStreamEvent(ev) {
  if (ev.session_id && !state.activeSessionId) state.activeSessionId = ev.session_id;
  switch (ev.type) {
    case "step_reset":
      state.liveStreamThinking = "";
      state.liveStreamText = "";
      state.liveStreamTools = [];
      renderLiveStreamCard();
      break;
    case "thinking_delta":
      if (ev.payload && ev.payload.thinking) {
        state.liveStreamThinking += ev.payload.thinking;
        renderLiveStreamCard();
      }
      break;
    case "text_delta":
      if (ev.payload && ev.payload.text) {
        state.liveStreamText += ev.payload.text;
        renderLiveStreamCard();
      }
      break;
    case "tool_call":
      if (ev.payload) {
        state.liveStreamTools.push({
          id: ev.payload.tool_call_id,
          name: ev.payload.tool_name,
          rawArgs: ev.payload.args,
          status: "running",
          preview: formatJSONSafe(ev.payload.args)
        });
        renderLiveStreamCard();
      }
      break;
    case "tool_result":
      if (ev.payload) {
        const item = state.liveStreamTools.find((x) => x.id === ev.payload.tool_call_id);
        if (item) {
          item.status = ev.payload.is_error ? "error" : "ok";
          item.preview = String(ev.payload.output || "").slice(0, 360);
        }
        renderLiveStreamCard();
      }
      break;
    case "approval_required":
      if (ev.payload && ev.payload.id) {
        noteApprovalSurfaced(ev.payload);
        if (!state.pendingApprovals.some((a) => a.id === ev.payload.id)) {
          state.pendingApprovals.push(ev.payload);
        }
        renderAttentionQueue();
        renderApprovalBanners();
      }
      break;
    case "doom_loop":
      if (ev.payload && ev.payload.tool_name) {
        surfaceRunError(`Doom loop halted on ${ev.payload.tool_name} (${ev.payload.consecutive_failures || 3} consecutive failures)`);
      }
      break;
    case "run_failed": {
      const msg = (ev.payload && (ev.payload.message || ev.payload.error)) || "Agent run failed";
      const code = ev.payload && ev.payload.code ? `[${ev.payload.code}] ` : "";
      surfaceRunError(`${code}${msg}`);
      break;
    }
  }
}

async function tryExecuteSlashCommand(raw) {
  const [cmd, ...rest] = raw.trim().split(/\s+/);
  const arg = rest.join(" ").trim();
  switch (cmd.toLowerCase()) {
    case "/compact":
      if (state.activeSessionId) {
        await apiJSON(`/api/sessions/${encodeURIComponent(state.activeSessionId)}/compact`, { method: "POST" });
        await loadSessionDetail(state.activeSessionId);
      }
      return true;
    case "/fork":
      if (state.activeSessionId) {
        const forked = await apiJSON(`/api/sessions/${encodeURIComponent(state.activeSessionId)}/fork`, {
          method: "POST",
          body: JSON.stringify({ title: arg || undefined })
        });
        await refreshActiveStatusQuietly();
        await selectSession(forked.id);
      }
      return true;
    case "/rewind":
      if (state.activeSessionId) {
        const toSeq = Number.parseInt(arg || "0", 10) || 0;
        const restore = Boolean(document.getElementById("rewind-restore-files-cb")?.checked);
        await apiJSON(`/api/sessions/${encodeURIComponent(state.activeSessionId)}/rewind`, {
          method: "POST",
          body: JSON.stringify({ to_seq: toSeq, restore_files: restore })
        });
        await loadSessionDetail(state.activeSessionId);
      }
      return true;
    case "/fleet":
      setCenterMode("fleet");
      if (arg) {
        const goalIn = document.getElementById("fleet-goal-input");
        if (goalIn) goalIn.value = arg;
      }
      openModal("fleet-launcher-modal");
      return true;
    case "/dream":
      await apiJSON("/api/alwayson/dream", {
        method: "POST",
        body: JSON.stringify({ workspace_id: state.activeWorkspaceId })
      });
      return true;
    case "/export":
    case "/export-html":
      await exportCurrentSession("html");
      return true;
    case "/export-md":
      await exportCurrentSession("md");
      return true;
    default:
      return false;
  }
}

async function exportCurrentSession(format) {
  if (!state.activeSessionId) return;
  const res = await apiJSON(`/api/sessions/${encodeURIComponent(state.activeSessionId)}/export?format=${encodeURIComponent(format)}`);
  if (format === "html" && res.preview_url) {
    await refreshWorkspaceArtifacts();
    setInspectorTab("preview");
    loadPreviewIntoIframe(res.preview_url);
  }
}

// ============================================================================
// Composer Autocomplete Palette (@files / $skills / /commands)
// ============================================================================

function updateComposerPalette() {
  const composer = document.getElementById("composer-input");
  const palette = document.getElementById("composer-palette");
  if (!composer || !palette || state.isComposingIME) return;

  const uptoCursor = composer.value.slice(0, composer.selectionStart || 0);
  const match = uptoCursor.match(/(?:^|\s)([@$/][A-Za-z0-9_./-]*)$/);
  if (!match) {
    palette.classList.add("hidden");
    return;
  }

  const token = match[1];
  const trigger = token[0];
  const query = token.slice(1).toLowerCase();
  let items = [];

  if (trigger === "@") {
    items = state.workspaceFiles
      .filter((f) => !f.is_dir && f.rel_path.toLowerCase().includes(query))
      .slice(0, 10)
      .map((f) => ({ insert: `@${f.rel_path} `, label: f.rel_path, right: "file" }));
  } else if (trigger === "$") {
    items = state.workspaceSkills
      .filter((sk) => sk.name.toLowerCase().includes(query))
      .slice(0, 10)
      .map((sk) => ({ insert: `$${sk.name} `, label: `$${sk.name}`, right: sk.scope || "skill" }));
  } else if (trigger === "/" && uptoCursor.trim() === token) {
    const cmds = [
      { cmd: "/compact", desc: "Anchored incremental compaction" },
      { cmd: "/fork", desc: "Fork session branch" },
      { cmd: "/rewind 0", desc: "Rewind + restore SHA-256 snapshots" },
      { cmd: "/fleet ", desc: "Open 6-topology Fleet launcher" },
      { cmd: "/dream", desc: "Distill candidate SKILL.md" },
      { cmd: "/export-html", desc: "Export & open sandboxed HTML preview" }
    ];
    items = cmds
      .filter((c) => c.cmd.includes(query))
      .map((c) => ({ insert: c.cmd, label: c.cmd, right: c.desc }));
  }

  if (items.length === 0) {
    palette.classList.add("hidden");
    return;
  }

  palette.innerHTML = items
    .map(
      (it) =>
        `<div class="palette-option" data-palette-insert="${esc(it.insert)}" data-palette-toklen="${token.length}">
          <span class="mono">${esc(it.label)}</span>
          <span style="color:var(--ink-secondary);font-size:11px;">${esc(it.right)}</span>
        </div>`
    )
    .join("");
  palette.classList.remove("hidden");
}

// ============================================================================
// Mode B: Interactive Visual Multi-Agent SVG Orchestration Studio
// ============================================================================

function renderTopologyCardsGrid() {
  const grid = document.getElementById("topology-cards-grid");
  if (!grid) return;
  grid.innerHTML = SIX_TOPOLOGIES.map((tp) => {
    const selected = tp.id === state.selectedLaunchTopology ? "selected" : "";
    const title = state.lang === "zh" ? `${tp.zh} (${tp.name})` : tp.name;
    return `<div class="topo-card ${selected}" data-pick-topo="${esc(tp.id)}">
      <strong>${esc(title)}</strong>
      <span>${esc(tp.desc)}</span>
    </div>`;
  }).join("");
}

function activeMissionSnapshot() {
  return state.missions.find((m) => m.id === state.activeMissionId) || state.missions[0] || null;
}

function renderFleetMissionSelector() {
  const sel = document.getElementById("mission-select");
  const liveBadge = document.getElementById("fleet-live-badge");
  const liveCount = state.missions.filter((m) => m.status === "running").length;
  if (liveBadge) {
    liveBadge.textContent = String(liveCount);
    liveBadge.classList.toggle("hidden", liveCount === 0);
  }
  if (!sel) return;

  if (state.missions.length === 0) {
    sel.innerHTML = `<option value="">— No Missions Yet —</option>`;
    return;
  }
  if (!state.activeMissionId || !state.missions.some((m) => m.id === state.activeMissionId)) {
    state.activeMissionId = state.missions[0].id;
  }
  sel.innerHTML = state.missions
    .map((m) => `<option value="${esc(m.id)}">[${esc(m.topology)}] ${esc(m.goal.slice(0, 46))} (${esc(m.status)})</option>`)
    .join("");
  sel.value = state.activeMissionId;
}

function subscribeToMissionStream(missionId) {
  if (state.missionSSE && state.missionSSEId === missionId && state.missionSSE.readyState !== 2) {
    return;
  }
  if (state.missionSSE) {
    state.missionSSE.close();
    state.missionSSE = null;
    state.missionSSEId = "";
  }
  if (!missionId || typeof EventSource === "undefined") return;

  const es = new EventSource(`/api/fleet/missions/${encodeURIComponent(missionId)}/stream`);
  state.missionSSE = es;
  state.missionSSEId = missionId;

  const handleMissionEvent = (raw) => {
    let ev;
    try {
      ev = JSON.parse(raw.data);
    } catch {
      return;
    }
    if (!ev || ev.type === "heartbeat") return;

    if (ev.type === "node_delta" && ev.node_id && ev.message) {
      const prev = state.nodeLogs.get(ev.node_id) || "";
      state.nodeLogs.set(ev.node_id, (prev + ev.message).slice(-4000));
    }
    if (ev.snapshot) {
      const idx = state.missions.findIndex((m) => m.id === ev.snapshot.id);
      if (idx >= 0) state.missions[idx] = ev.snapshot;
      else state.missions.unshift(ev.snapshot);
    }
    renderFleetMissionSelector();
    renderFleetStudio();

    if (state.selectedNodeId && (ev.type === "merge_landed" || ev.type === "node_updated")) {
      fetchSelectedNodeDiff(missionId, state.selectedNodeId);
    }
    if (ev.type === "mission_completed" || ev.type === "mission_failed" || ev.type === "mission_cancelled") {
      es.close();
      if (state.missionSSE === es) {
        state.missionSSE = null;
        state.missionSSEId = "";
      }
    }
  };

  [
    "snapshot_sync",
    "mission_started",
    "node_updated",
    "node_delta",
    "node_steered",
    "node_stalled",
    "edge_pulse",
    "task_updated",
    "note_added",
    "merge_queued",
    "merge_landed",
    "merge_rejected",
    "mission_completed",
    "mission_failed",
    "mission_cancelled"
  ].forEach((evName) => es.addEventListener(evName, handleMissionEvent));
}

function computeLayeredNodePositions(nodes, edges) {
  const width = 920;
  const height = 430;
  if (!nodes || nodes.length === 0) return new Map();

  const inDeg = new Map(nodes.map((n) => [n.id, 0]));
  const adj = new Map(nodes.map((n) => [n.id, []]));
  for (const e of edges || []) {
    if (adj.has(e.from) && inDeg.has(e.to) && e.kind !== "feedback") {
      adj.get(e.from).push(e.to);
      inDeg.set(e.to, (inDeg.get(e.to) || 0) + 1);
    }
  }

  const layerOf = new Map();
  const queue = [];
  for (const n of nodes) {
    if ((inDeg.get(n.id) || 0) === 0) {
      layerOf.set(n.id, 0);
      queue.push(n.id);
    }
  }
  if (queue.length === 0) {
    layerOf.set(nodes[0].id, 0);
    queue.push(nodes[0].id);
  }

  while (queue.length > 0) {
    const cur = queue.shift();
    const curLayer = layerOf.get(cur) || 0;
    for (const nxt of adj.get(cur) || []) {
      const nextLayer = Math.min(4, curLayer + 1);
      if (!layerOf.has(nxt) || layerOf.get(nxt) < nextLayer) {
        layerOf.set(nxt, nextLayer);
        queue.push(nxt);
      }
    }
  }
  for (const n of nodes) {
    if (!layerOf.has(n.id)) layerOf.set(n.id, 1);
  }

  const byLayer = new Map();
  let maxLayer = 0;
  for (const n of nodes) {
    const l = layerOf.get(n.id) || 0;
    if (l > maxLayer) maxLayer = l;
    if (!byLayer.has(l)) byLayer.set(l, []);
    byLayer.get(l).push(n);
  }

  const coords = new Map();
  const colSpan = maxLayer === 0 ? 0 : (width - 280) / maxLayer;
  for (let l = 0; l <= maxLayer; l++) {
    const group = byLayer.get(l) || [];
    const x = maxLayer === 0 ? width / 2 : 140 + l * colSpan;
    const rowStep = height / (group.length + 1);
    group.forEach((n, idx) => {
      coords.set(n.id, { x, y: Math.round(rowStep * (idx + 1)) });
    });
  }
  return coords;
}

function statusStrokeColor(status) {
  switch (status) {
    case "running":
      return "var(--accent)";
    case "merging":
    case "stalled":
      return "var(--amber)";
    case "completed":
      return "var(--green)";
    case "failed":
      return "var(--red)";
    default:
      return "var(--hairline-strong)";
  }
}

function renderFleetStudio() {
  const snap = activeMissionSnapshot();
  const svg = document.getElementById("fleet-svg-canvas");
  const statusPill = document.getElementById("mission-status-pill");
  const costPill = document.getElementById("mission-cost-pill");
  const stopBtn = document.getElementById("stop-mission-btn");
  const goalBanner = document.getElementById("fleet-goal-banner");
  const queueBar = document.getElementById("refinery-queue-bar");

  if (!svg) return;

  if (!snap) {
    if (statusPill) {
      statusPill.className = "status-pill";
      statusPill.textContent = "idle";
    }
    if (costPill) costPill.textContent = "$0.0000";
    if (stopBtn) stopBtn.classList.add("hidden");
    if (goalBanner) goalBanner.textContent = t("fleetEmptyGoal");
    svg.innerHTML = `<text x="460" y="215" text-anchor="middle" fill="var(--ink-secondary)" font-size="13">${esc(t("fleetEmptyGoal"))}</text>`;
    return;
  }

  if (statusPill) {
    statusPill.className = `status-pill ${esc(snap.status)}`;
    statusPill.textContent = `${snap.topology} · ${snap.status}`;
  }
  if (costPill) costPill.textContent = `$${Number(snap.total_cost_usd || 0).toFixed(4)}`;
  if (stopBtn) stopBtn.classList.toggle("hidden", snap.status !== "running");
  if (goalBanner) goalBanner.textContent = snap.goal;

  if (queueBar) {
    const mq = snap.merge_queue || [];
    queueBar.innerHTML = mq.length
      ? `<span class="meta-chip mono">Refinery Queue: ${mq.map(esc).join(" → ")}</span>`
      : "";
  }

  if (!state.selectedNodeId && snap.nodes && snap.nodes.length > 0) {
    state.selectedNodeId = snap.nodes[0].id;
  }

  const coords = computeLayeredNodePositions(snap.nodes || [], snap.edges || []);
  const cardW = 204;
  const cardH = 84;

  let svgHTML = `<defs>
    <marker id="fleet-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
      <path d="M 0 1.5 L 9 5 L 0 8.5 z" fill="var(--ink-tertiary)"/>
    </marker>
    <marker id="fleet-arrow-active" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
      <path d="M 0 1.5 L 9 5 L 0 8.5 z" fill="var(--accent)"/>
    </marker>
  </defs>`;

  // Edges with Cubic Bezier curves + animated message pulses
  for (const edge of snap.edges || []) {
    const p1 = coords.get(edge.from);
    const p2 = coords.get(edge.to);
    if (!p1 || !p2) continue;
    const isFeedback = p2.x < p1.x;
    const startX = isFeedback ? p1.x - cardW / 2 : p1.x + cardW / 2;
    const endX = isFeedback ? p2.x + cardW / 2 : p2.x - cardW / 2;
    const startY = isFeedback ? p1.y + 18 : p1.y;
    const endY = isFeedback ? p2.y + 18 : p2.y;
    const midX = (startX + endX) / 2;
    const curveOffset = isFeedback ? 46 : 0;
    const pathD = `M ${startX} ${startY} C ${midX} ${startY + curveOffset}, ${midX} ${endY + curveOffset}, ${endX} ${endY}`;
    const stroke = edge.active ? "var(--accent)" : "var(--ink-tertiary)";
    const cls = edge.active ? "svg-edge-active" : "";
    const marker = edge.active ? "url(#fleet-arrow-active)" : "url(#fleet-arrow)";

    svgHTML += `<g>
      <path d="${pathD}" fill="none" stroke="${stroke}" stroke-width="${edge.active ? 2.2 : 1.3}" class="${cls}" marker-end="${marker}"/>
      <text x="${midX}" y="${(startY + endY) / 2 + curveOffset - 6}" text-anchor="middle" fill="var(--ink-secondary)" font-size="10.5" font-family="var(--font-mono)">${esc(edge.label || edge.kind || "")}</text>
    </g>`;
  }

  // Heterogeneous Agent Nodes
  for (const node of snap.nodes || []) {
    const pt = coords.get(node.id) || { x: 200, y: 200 };
    const x = pt.x - cardW / 2;
    const y = pt.y - cardH / 2;
    const isSel = node.id === state.selectedNodeId;
    const borderCol = isSel ? "var(--accent)" : statusStrokeColor(node.status);
    const badgeModel = `${node.provider_id}/${node.model_id}`;
    const wtShort = node.worktree_branch ? `⎇ ${node.worktree_branch.split("/").pop()}` : "shared-root";

    svgHTML += `<g class="svg-node-group" data-svg-node="${esc(node.id)}" transform="translate(${x}, ${y})">
      <rect x="0" y="0" width="${cardW}" height="${cardH}" rx="11" fill="var(--bg-elevated)" stroke="${borderCol}" stroke-width="${isSel ? 2.2 : 1.2}"/>
      <circle cx="15" cy="18" r="4.5" fill="${statusStrokeColor(node.status)}"/>
      <text x="26" y="22" fill="var(--ink-primary)" font-size="12.5" font-weight="600">${esc(node.label.slice(0, 20))}</text>
      <text x="${cardW - 10}" y="21" text-anchor="end" fill="var(--ink-secondary)" font-size="10" font-family="var(--font-mono)">${esc(node.role_id)}</text>

      <rect x="10" y="31" width="${cardW - 20}" height="20" rx="5" fill="var(--bg-subtle)"/>
      <text x="16" y="44.5" fill="var(--accent)" font-size="10.2" font-family="var(--font-mono)">${esc(badgeModel.slice(0, 27))}</text>

      <text x="12" y="69" fill="var(--ink-secondary)" font-size="10" font-family="var(--font-mono)">${esc(wtShort)}</text>
      <text x="${cardW - 10}" y="69" text-anchor="end" fill="var(--ink-secondary)" font-size="10" font-family="var(--font-mono)">${Number(node.tokens || 0)}t · $${Number(node.cost_usd || 0).toFixed(3)}</text>
    </g>`;
  }

  svg.innerHTML = svgHTML;
  renderSelectedNodePanel(snap);
  renderBlackboardPanel(snap);
}

function renderSelectedNodePanel(snap) {
  const titleEl = document.getElementById("selected-node-title");
  const badgeEl = document.getElementById("selected-node-badge");
  const streamEl = document.getElementById("selected-node-stream");
  const diffEl = document.getElementById("selected-node-diff");
  const node = (snap.nodes || []).find((n) => n.id === state.selectedNodeId);
  if (!node) return;

  if (titleEl) titleEl.textContent = `${node.label} (${node.id})`;
  if (badgeEl) badgeEl.textContent = `${node.provider_id}/${node.model_id} · ${node.status}`;
  if (streamEl) streamEl.textContent = escapeBidi(state.nodeLogs.get(node.id) || "");
  if (diffEl && state.nodeDiffText) {
    diffEl.innerHTML = renderDiffPreviewHTML(state.nodeDiffText);
  }
}

async function fetchSelectedNodeDiff(missionId, nodeId) {
  try {
    const res = await apiJSON(`/api/fleet/missions/${encodeURIComponent(missionId)}/nodes/${encodeURIComponent(nodeId)}/diff`);
    state.nodeDiffText = res.diff || "(clean worktree — no file diff yet)";
    const snap = activeMissionSnapshot();
    if (snap) renderSelectedNodePanel(snap);
  } catch {
    state.nodeDiffText = "";
  }
}

function renderBlackboardPanel(snap) {
  const tasksEl = document.getElementById("blackboard-tasks-list");
  const notesEl = document.getElementById("blackboard-notes-list");
  const countsEl = document.getElementById("blackboard-counts");
  const tasks = snap.tasks || [];
  const notes = snap.notes || [];
  if (countsEl) countsEl.textContent = `${tasks.length} tasks · ${notes.length} notes`;

  if (tasksEl) {
    tasksEl.innerHTML = tasks
      .map(
        (tk) => `<div class="bb-task-row" style="margin-left:${Math.min(3, Number(tk.depth || 0)) * 12}px">
          <span><code class="mono">${esc(tk.id)}</code> ${esc(tk.title)}</span>
          <span class="meta-chip mono">${esc(tk.owner_node_id ? `${tk.owner_node_id} · ` : "")}${esc(tk.status)}</span>
        </div>`
      )
      .join("");
  }
  if (notesEl) {
    notesEl.innerHTML = notes
      .map(
        (nt) => `<div class="bb-note-card">
          <div><strong>${esc(nt.node_id)}</strong> · ${esc(nt.title || nt.id)}</div>
          <div style="color:var(--ink-secondary);">${esc(String(nt.body || "").slice(0, 200))}</div>
        </div>`
      )
      .join("");
  }
}

// ============================================================================
// Right Inspector Tabs: Files, Time-Machine, Config Matrix & Sandboxed Preview
// ============================================================================

async function refreshWorkspaceContext(wsId) {
  if (!wsId) return;
  try {
    const [filesData, skillsData, artData] = await Promise.all([
      apiJSON(`/api/workspaces/${encodeURIComponent(wsId)}/files`),
      apiJSON(`/api/workspaces/${encodeURIComponent(wsId)}/skills`),
      apiJSON(`/api/workspaces/${encodeURIComponent(wsId)}/artifacts`)
    ]);
    state.workspaceFiles = filesData.files || [];
    state.workspaceSkills = skillsData.skills || [];
    if (artData.previews) state.previews = artData.previews;
    renderWorkspaceFilesList();
    renderPreviewTabList();
  } catch {
    // Ignore if workspace deleted
  }
}

async function refreshWorkspaceArtifacts() {
  if (!state.activeWorkspaceId) return;
  const artData = await apiJSON(`/api/workspaces/${encodeURIComponent(state.activeWorkspaceId)}/artifacts`);
  if (artData.previews) state.previews = artData.previews;
  renderPreviewTabList();
}

function renderWorkspaceFilesList() {
  const listEl = document.getElementById("workspace-files-list");
  const filterVal = (document.getElementById("file-filter-input")?.value || "").toLowerCase();
  if (!listEl) return;

  const filtered = state.workspaceFiles
    .filter((f) => !f.is_dir && f.rel_path.toLowerCase().includes(filterVal))
    .slice(0, 150);

  listEl.innerHTML = filtered
    .map(
      (f) => `<div class="file-row" data-view-file="${esc(f.rel_path)}">
        <span class="mono">${esc(f.rel_path)}</span>
        <span class="mono" style="color:var(--ink-tertiary);font-size:10.5px;">${Number(f.size || 0)}B</span>
      </div>`
    )
    .join("");
}

function renderSnapshotTimeMachine() {
  const listEl = document.getElementById("snapshots-timeline-list");
  if (!listEl) return;

  const snapsBySeq = new Map();
  for (const rec of state.sessionSnapshots || []) {
    if (!snapsBySeq.has(rec.seq)) snapsBySeq.set(rec.seq, []);
    snapsBySeq.get(rec.seq).push(rec);
  }

  let html = `<div class="snap-card">
    <div class="snap-card-head"><span class="mono">Seq #0 (Clean Root)</span>
      <button class="ghost-xs-btn" type="button" data-rewind-seq="0">${esc(t("rewindHereBtn"))}</button>
    </div>
  </div>`;

  for (const ent of state.sessionEntries || []) {
    const touched = snapsBySeq.get(ent.seq) || [];
    const tags = touched
      .map((r) => `<span class="meta-chip mono">${esc(r.kind)}: ${esc(r.rel_path)}</span>`)
      .join("");
    html += `<div class="snap-card">
      <div class="snap-card-head">
        <span class="mono">Seq #${Number(ent.seq)} · ${esc(ent.role)}</span>
        <div class="row-actions">
          <button class="ghost-xs-btn" type="button" data-fork-seq="${Number(ent.seq)}">${esc(t("forkHereBtn"))}</button>
          <button class="ghost-xs-btn" type="button" data-rewind-seq="${Number(ent.seq)}">${esc(t("rewindHereBtn"))}</button>
        </div>
      </div>
      ${tags ? `<div class="snap-files-tags">${tags}</div>` : ""}
    </div>`;
  }
  listEl.innerHTML = html;
}

// ============================================================================
// Mode C & Inspector Tab 3: Zero-Typing Settings & Agent-Model Matrix Studio
// ============================================================================

function flashAutoSaveIndicator(msg = "已实时保存并热生效 ✓") {
  const pill = document.getElementById("config-autosave-pill");
  if (!pill) return;
  pill.className = "status-pill ok";
  pill.textContent = msg;
}

export function getModelsForProvider(provId) {
  const map = new Map();
  const cfgProv = state.config && state.config.providers && state.config.providers[provId];
  const preset = PROVIDER_PRESETS[provId];

  if (cfgProv && cfgProv.models) {
    for (const [k, m] of Object.entries(cfgProv.models)) {
      map.set(k, Object.assign({ id: k, name: m.name || k }, m));
    }
  }
  if (preset && preset.models) {
    for (const [k, m] of Object.entries(preset.models)) {
      if (!map.has(k)) {
        map.set(k, Object.assign({ id: k, name: m.name || k }, m));
      }
    }
  }
  if (map.size === 0 && cfgProv) {
    for (const p of Object.values(PROVIDER_PRESETS)) {
      if (p.protocol === cfgProv.protocol) {
        for (const [k, m] of Object.entries(p.models)) {
          if (!map.has(k)) map.set(k, Object.assign({ id: k, name: m.name || k }, m));
        }
      }
    }
  }
  return Array.from(map.values());
}

function getAllProviderModelRefs() {
  if (!state.config || !state.config.providers) return [];
  const out = [];
  const provIds = Object.keys(state.config.providers).sort();
  for (const pid of provIds) {
    const models = getModelsForProvider(pid);
    for (const m of models) {
      out.push({ ref: `${pid}/${m.id}`, provider: pid, model: m.id, name: m.name || m.id });
    }
  }
  return out;
}

export async function applyProviderPreset(presetId) {
  const preset = PROVIDER_PRESETS[presetId];
  if (!preset) return null;
  const existing = state.config && state.config.providers && state.config.providers[preset.id];
  const mergedModels = Object.assign({}, preset.models, (existing && existing.models) || {});
  const snap = await apiJSON("/api/config/providers", {
    method: "POST",
    body: JSON.stringify({
      id: preset.id,
      name: preset.name,
      protocol: preset.protocol,
      base_url: (existing && existing.base_url) || preset.base_url,
      api_key: (existing && existing.api_key && existing.api_key !== "[REDACTED]") ? existing.api_key : preset.default_key_ref,
      timeout_sec: (existing && existing.timeout_sec) || preset.timeout_sec,
      models: mergedModels
    })
  });
  state.config = snap;
  populateAgentRoleSelect();
  renderAgentMatrixAndVaultEditor();
  renderConfigStudio();
  flashAutoSaveIndicator(`已一键启用供应商 ${preset.zh || preset.name} ✓`);
  return snap;
}

export async function patchAgentRole(agentId, patch) {
  if (!agentId) return null;
  const payload = Object.assign({ id: agentId }, patch);
  const snap = await apiJSON("/api/config/agents", {
    method: "POST",
    body: JSON.stringify(payload)
  });
  state.config = snap;
  populateAgentRoleSelect();
  renderAgentMatrixAndVaultEditor();
  renderConfigStudio();
  flashAutoSaveIndicator(`角色 ${agentId} 已自动保存 ✓`);
  return snap;
}

export async function applyMatrixScheme(schemeId) {
  const scheme = MATRIX_SCHEMES.find((s) => s.id === schemeId);
  if (!scheme) return;
  if (scheme.ensureProvider && (!state.config.providers || !state.config.providers[scheme.ensureProvider])) {
    await applyProviderPreset(scheme.ensureProvider);
  }
  for (const [roleId, cfg] of Object.entries(scheme.roles)) {
    const snap = await apiJSON("/api/config/agents", {
      method: "POST",
      body: JSON.stringify({
        id: roleId,
        provider: cfg.provider,
        model: cfg.model,
        effort: cfg.effort
      })
    });
    state.config = snap;
  }
  populateAgentRoleSelect();
  renderAgentMatrixAndVaultEditor();
  renderConfigStudio();
  flashAutoSaveIndicator(`已一键套用「${scheme.zh}」✓`);
}

export async function toggleRuleChip(field, item) {
  if (!state.config || !state.config.rules) return null;
  const currentList = Array.isArray(state.config.rules[field]) ? [...state.config.rules[field]] : [];
  const idx = currentList.indexOf(item);
  if (idx >= 0) currentList.splice(idx, 1);
  else currentList.push(item);

  const snap = await apiJSON("/api/config/rules", {
    method: "POST",
    body: JSON.stringify({ [field]: currentList })
  });
  state.config = snap;
  renderConfigStudio();
  flashAutoSaveIndicator(`安全规则 ${field} 已同步 ✓`);
  return snap;
}

export async function toggleMCPPreset(presetId) {
  const preset = MCP_SERVER_PRESETS.find((p) => p.id === presetId);
  if (!preset) return null;
  const servers = (state.config && state.config.mcp && state.config.mcp.servers) || {};
  const existing = servers[preset.id];
  const nowEnabled = existing ? !(existing.enabled !== false) : true;

  const snap = await apiJSON("/api/config/mcp", {
    method: "POST",
    body: JSON.stringify({
      id: preset.id,
      transport: preset.transport,
      command: preset.command,
      args: preset.args,
      timeout_sec: preset.timeout_sec,
      enabled: nowEnabled
    })
  });
  state.config = snap;
  renderConfigStudio();
  flashAutoSaveIndicator(`MCP 服务 ${preset.id} ${nowEnabled ? "已启用" : "已停用"} ✓`);
  return snap;
}

function renderAgentMatrixAndVaultEditor() {
  const rowsHost = document.getElementById("agent-matrix-rows");
  const fileSel = document.getElementById("vault-file-select");
  if (rowsHost && state.config && state.config.agents) {
    const roleOrder = ["lead", "coder", "verifier", "explorer", "debugger", "dreamer", "utility"];
    const agents = Object.values(state.config.agents).sort((a, b) => {
      const ia = roleOrder.indexOf(a.id);
      const ib = roleOrder.indexOf(b.id);
      if (ia !== -1 && ib !== -1) return ia - ib;
      if (ia !== -1) return -1;
      if (ib !== -1) return 1;
      return a.id.localeCompare(b.id);
    });
    const provIds = Object.keys(state.config.providers || {}).sort();

    rowsHost.innerHTML = agents
      .map((a) => {
        const meta = ROLE_META[a.id];
        const roleLabel = meta ? (state.lang === "zh" ? meta.zh.split(" (")[0] : meta.en) : (a.name || a.id);
        const provOptions = provIds
          .map((pid) => {
            const p = state.config.providers[pid];
            return `<option value="${esc(pid)}" ${pid === a.provider ? "selected" : ""}>${esc((p && p.name) || pid)}</option>`;
          })
          .join("");
        const models = getModelsForProvider(a.provider);
        const hasCurrentModel = models.some((m) => m.id === a.model);
        const modelOptions = [
          ...(hasCurrentModel ? [] : [{ id: a.model, name: a.model }]),
          ...models
        ]
          .map((m) => `<option value="${esc(m.id)}" ${m.id === a.model ? "selected" : ""}>${esc(m.id)}</option>`)
          .join("");

        return `<div class="matrix-row" data-matrix-agent="${esc(a.id)}">
          <div class="matrix-row-head">
            <span><strong class="mono">${esc(a.id)}</strong> · <span class="tiny-muted">${esc(roleLabel)}</span></span>
            <button class="chip-btn ${a.read_only ? "active" : ""}" type="button" data-toggle-agent-ro="${esc(a.id)}" style="padding:1px 7px;font-size:10.5px;">
              ${a.read_only ? "🔒 只读" : "🔓 读写"}
            </button>
          </div>
          <div class="matrix-row-selects">
            <select class="mac-select" data-agent-prov-select="${esc(a.id)}" title="选择服务商">${provOptions}</select>
            <select class="mac-select mono" data-agent-model-select="${esc(a.id)}" title="选择模型">${modelOptions}</select>
            <select class="mac-select" data-agent-effort-select="${esc(a.id)}" title="推理强度">
              ${["low", "medium", "high", "xhigh", "max"]
                .map((ef) => `<option value="${ef}" ${ef === (a.effort || "high") ? "selected" : ""}>${ef}</option>`)
                .join("")}
            </select>
          </div>
        </div>`;
      })
      .join("");
  }

  if (fileSel && state.configFiles.length > 0) {
    const prev = fileSel.value;
    fileSel.innerHTML = state.configFiles.map((f) => `<option value="${esc(f)}">${esc(f)}</option>`).join("");
    fileSel.value = state.configFiles.includes(prev) ? prev : state.configFiles[0];
    loadSelectedVaultConfigFile(fileSel.value);
  }
}

export function renderConfigStudio() {
  const container = document.getElementById("config-studio-content");
  if (!container || !state.config) return;

  const pageKeyMap = {
    matrix: "cfgPageMatrix",
    providers: "cfgPageProviders",
    workflows: "cfgPageWorkflows",
    global: "cfgPageGlobal",
    mcp: "cfgPageMCP",
    files: "cfgPageFiles"
  };
  const titleEl = document.getElementById("settings-dialog-page-title");
  if (titleEl) {
    titleEl.textContent = t(pageKeyMap[state.configPage] || "cfgPageMatrix");
  }

  document.querySelectorAll("[data-cfg-page]").forEach((btn) => {
    btn.classList.toggle("active", btn.getAttribute("data-cfg-page") === state.configPage);
  });

  switch (state.configPage) {
    case "matrix":
      container.innerHTML = renderConfigMatrixPage();
      break;
    case "providers":
      container.innerHTML = renderConfigProvidersPage();
      break;
    case "workflows":
      container.innerHTML = renderConfigWorkflowsPage();
      break;
    case "global":
      container.innerHTML = renderConfigGlobalAndRulesPage();
      break;
    case "mcp":
      container.innerHTML = renderConfigMCPPage();
      break;
    case "files":
      container.innerHTML = renderConfigRawFilesPage();
      loadSelectedVaultConfigFile(document.getElementById("studio-vault-file-select")?.value || state.configFiles[0] || "config.toml");
      break;
    default:
      container.innerHTML = renderConfigMatrixPage();
  }
}

function renderConfigMatrixPage() {
  const roleOrder = ["lead", "coder", "verifier", "explorer", "debugger", "dreamer", "utility"];
  const agents = Object.values(state.config.agents || {}).sort((a, b) => {
    const ia = roleOrder.indexOf(a.id);
    const ib = roleOrder.indexOf(b.id);
    if (ia !== -1 && ib !== -1) return ia - ib;
    if (ia !== -1) return -1;
    if (ib !== -1) return 1;
    return a.id.localeCompare(b.id);
  });
  const provIds = Object.keys(state.config.providers || {}).sort();
  const allRefs = getAllProviderModelRefs();

  const schemesHTML = MATRIX_SCHEMES.map(
    (s) => `<button class="scheme-card" type="button" data-apply-scheme="${esc(s.id)}">
      <div class="scheme-card-top">
        <strong>${esc(state.lang === "zh" ? s.zh : s.en)}</strong>
        <span class="meta-chip">一键套用</span>
      </div>
      <span>${esc(s.desc)}</span>
    </button>`
  ).join("");

  const specialistOpts = Object.values(SPECIALIST_ROLE_PRESETS)
    .map((sp) => `<option value="${esc(sp.id)}">${esc(sp.zh)}</option>`)
    .join("");

  const roleCardsHTML = agents
    .map((a) => {
      const meta = ROLE_META[a.id] || SPECIALIST_ROLE_PRESETS[a.id];
      const title = meta ? meta.zh : `${a.name || a.id} (${a.id})`;
      const desc = (meta && meta.desc) || a.description || "";

      const provOpts = provIds
        .map((pid) => {
          const p = state.config.providers[pid];
          return `<option value="${esc(pid)}" ${pid === a.provider ? "selected" : ""}>${esc((p && p.name) || pid)} (${esc((p && p.protocol) || "")})</option>`;
        })
        .join("");

      const models = getModelsForProvider(a.provider);
      const hasCurrent = models.some((m) => m.id === a.model);
      const modOpts = [...(hasCurrent ? [] : [{ id: a.model, name: a.model }]), ...models]
        .map((m) => {
          const ctxK = m.context_window ? ` · ${Math.round(m.context_window / 1000)}K` : "";
          const thk = m.supports_thinking ? " · 🧠" : "";
          return `<option value="${esc(m.id)}" ${m.id === a.model ? "selected" : ""}>${esc(m.name || m.id)} (${esc(m.id)})${ctxK}${thk}</option>`;
        })
        .join("");

      const efforts = [
        { id: "low", label: "Low 极速" },
        { id: "medium", label: "Medium 均衡" },
        { id: "high", label: "High 深度" },
        { id: "xhigh", label: "XHigh 强化" },
        { id: "max", label: "Max 极限" }
      ];
      const curEffort = a.effort || "high";
      const effortPills = efforts
        .map(
          (ef) => `<button class="pill-opt ${curEffort === ef.id ? "active" : ""}" type="button" data-role-effort="${esc(a.id)}" data-val="${ef.id}">${esc(ef.label)}</button>`
        )
        .join("");

      const hasExplicitTools = Array.isArray(a.allowed_tools);
      const allowedSet = new Set(hasExplicitTools ? a.allowed_tools : CORE_TOOLS.map((t) => t.id));
      const isZeroTools = hasExplicitTools && a.allowed_tools.length === 0;
      const isAllTools = !hasExplicitTools || a.allowed_tools.length === CORE_TOOLS.length;
      const isReadOnlyTools =
        hasExplicitTools &&
        a.allowed_tools.length === 2 &&
        allowedSet.has("read_file") &&
        allowedSet.has("search_code");

      const toolPills = CORE_TOOLS.map((tl) => {
        const on = !hasExplicitTools || allowedSet.has(tl.id);
        return `<button class="chip-btn ${on ? "active" : ""}" type="button" data-toggle-role-tool="${esc(a.id)}" data-tool="${esc(tl.id)}">
          <span>${tl.icon} ${esc(tl.id)}</span>
        </button>`;
      }).join("");

      const fallbacks = Array.isArray(a.fallbacks) ? a.fallbacks : [];
      const fallbackChips = fallbacks
        .map(
          (fb) => `<button class="chip-btn active mono" type="button" data-remove-fallback="${esc(a.id)}" data-fb="${esc(fb)}" title="点击移除此备用模型">
            <span>${esc(fb)}</span><span class="chip-remove">×</span>
          </button>`
        )
        .join("");

      const fbOptions = allRefs
        .filter((item) => item.ref !== `${a.provider}/${a.model}` && !fallbacks.includes(item.ref))
        .map((item) => `<option value="${esc(item.ref)}">${esc(item.ref)} (${esc(item.name)})</option>`)
        .join("");

      const promptOpts = PROMPT_TEMPLATES.map(
        (pt) => `<option value="${esc(pt.id)}" ${pt.id === a.id ? "selected" : ""}>${esc(pt.label)}</option>`
      ).join("");

      return `<div class="role-matrix-card" data-role-card="${esc(a.id)}">
        <div class="role-card-head">
          <div class="role-title-wrap">
            <div class="role-title-line">
              <span class="role-id-badge">${esc(a.id)}</span>
              <strong>${esc(title)}</strong>
            </div>
            <span class="tiny-muted">${esc(desc)}</span>
          </div>
          <button class="ghost-xs-btn" type="button" data-reset-role-default="${esc(a.id)}">↺ 恢复推荐</button>
        </div>

        <div class="role-field-grid">
          <label class="cfg-field-label">
            <span>1. 模型供应商 (Provider 下拉即选)</span>
            <select class="mac-select" data-agent-prov-select="${esc(a.id)}">${provOpts}</select>
          </label>
          <label class="cfg-field-label">
            <span>2. 绑定主模型 (Model 级联下拉)</span>
            <select class="mac-select mono" data-agent-model-select="${esc(a.id)}">${modOpts}</select>
          </label>
        </div>

        <div class="settings-form-row" style="padding:6px 0;">
          <span class="tiny-muted">3. 推理思考强度 (Reasoning Effort)</span>
          <div class="pill-bar">${effortPills}</div>
        </div>

        <div class="settings-form-row" style="padding:6px 0;">
          <span class="tiny-muted">4. 文件系统与命令读写边界</span>
          <div class="pill-bar">
            <button class="pill-opt ${!a.read_only ? "active" : ""}" type="button" data-set-role-ro="${esc(a.id)}" data-ro="false">🔓 读写与执行</button>
            <button class="pill-opt ${a.read_only ? "active" : ""}" type="button" data-set-role-ro="${esc(a.id)}" data-ro="true">🔒 严格只读 (Read-Only)</button>
          </div>
        </div>

        <div class="cfg-field-label">
          <div class="subhead-row">
            <span>5. 可调用工具白名单 (点击胶囊开关 · 支持预设)</span>
            <div class="row-actions">
              <button class="ghost-xs-btn ${isAllTools ? "active" : ""}" type="button" data-role-tool-preset="${esc(a.id)}" data-preset="all">全部工具</button>
              <button class="ghost-xs-btn ${isReadOnlyTools ? "active" : ""}" type="button" data-role-tool-preset="${esc(a.id)}" data-preset="readonly">仅只读侦察</button>
              <button class="ghost-xs-btn" type="button" data-role-tool-preset="${esc(a.id)}" data-preset="noshell">仅文件读写</button>
              <button class="ghost-xs-btn ${isZeroTools ? "active" : ""}" type="button" data-role-tool-preset="${esc(a.id)}" data-preset="none">无工具(纯文本)</button>
            </div>
          </div>
          <div class="chips-wrap" style="margin-top:4px;">${toolPills}</div>
        </div>

        <div class="cfg-field-label">
          <span>6. 故障自动回退链 (Fallbacks — 下拉点选添加，点击 × 移除)</span>
          <div class="chips-wrap" style="margin-top:4px;">
            ${fallbackChips || `<span class="tiny-muted">未配置回退模型</span>`}
            <select class="mac-select mono" data-add-role-fallback="${esc(a.id)}" style="width:auto;min-width:190px;">
              <option value="">+ 下拉添加备用回退模型…</option>
              ${fbOptions}
            </select>
          </div>
        </div>

        <div class="cfg-field-label">
          <span>7. 角色系统提示词模板 (System Prompt Preset)</span>
          <select class="mac-select" data-role-prompt-template="${esc(a.id)}">
            <option value="">— 选择内置专业提示词模板立即套用 —</option>
            ${promptOpts}
          </select>
        </div>
      </div>`;
    })
    .join("");

  return `
    <div class="cfg-section-card">
      <div class="cfg-section-head">
        <div>
          <h3 class="cfg-section-title">一键套用多智能体异构模型方案 (One-Click Matrix Schemes)</h3>
          <p class="cfg-section-desc">无需逐个手动输入，点击任意预设卡片即可一键为全部 7 个 Agent 角色分配最佳异构模型组合。</p>
        </div>
        <div class="row-actions">
          <select id="add-specialist-role-select" class="mac-select">
            <option value="">+ 从预设模板一键新增专家角色…</option>
            ${specialistOpts}
          </select>
        </div>
      </div>
      <div class="scheme-presets-grid">${schemesHTML}</div>
    </div>

    <div class="role-cards-grid">${roleCardsHTML}</div>
  `;
}

function renderConfigProvidersPage() {
  const configured = state.config.providers || {};
  const presetCardsHTML = Object.values(PROVIDER_PRESETS)
    .map((p) => {
      const isEnabled = Boolean(configured[p.id]);
      const modelCount = Object.keys((configured[p.id] && configured[p.id].models) || p.models || {}).length;
      return `<button class="scheme-card ${isEnabled ? "active" : ""}" type="button" data-enable-provider-preset="${esc(p.id)}">
        <div class="scheme-card-top">
          <strong>${esc(p.zh || p.name)}</strong>
          <span class="status-pill ${isEnabled ? "ok" : ""}">${isEnabled ? "已启用 ✓" : "+ 一键启用"}</span>
        </div>
        <span class="mono">${esc(p.protocol)} · ${modelCount} models</span>
        <span>${esc(p.base_url)}</span>
      </button>`;
    })
    .join("");

  const provList = Object.values(configured).sort((a, b) => a.id.localeCompare(b.id));
  const provCardsHTML = provList
    .map((p) => {
      const preset = PROVIDER_PRESETS[p.id] || PROVIDER_PRESETS.openai;
      const candidateURLs = Array.from(
        new Set([
          p.base_url,
          ...(preset.endpoints || []),
          "https://api.anthropic.com",
          "https://api.openai.com/v1",
          "https://generativelanguage.googleapis.com/v1beta",
          "http://localhost:11211/api/openai/v1",
          "https://api.deepseek.com/v1",
          "https://openrouter.ai/api/v1",
          "https://dashscope.aliyuncs.com/compatible-mode/v1",
          "https://ark.cn-beijing.volces.com/api/v3",
          "https://api.moonshot.cn/v1",
          "http://127.0.0.1:11434/v1",
          "http://127.0.0.1:8080/v1"
        ].filter(Boolean))
      );

      const candidateKeyRefs = Array.from(
        new Set([
          ...(preset.env_keys || []),
          ...(preset.keychain_refs || []),
          "env:ANTHROPIC_API_KEY",
          "env:OPENAI_API_KEY",
          "env:GEMINI_API_KEY",
          "env:DEEPSEEK_API_KEY",
          "env:OPENROUTER_API_KEY",
          "env:DASHSCOPE_API_KEY",
          "env:ARK_API_KEY",
          "env:MOONSHOT_API_KEY",
          "env:LOCAL_GATEWAY_KEY",
          "env:MLX_API_KEY",
          "keychain:gorness-" + p.id
        ])
      );

      const curKeyRef = p.api_key && p.api_key !== "[REDACTED]" ? p.api_key : preset.default_key_ref || "env:OPENAI_API_KEY";
      const allModelsMap = Object.assign({}, preset.models || {}, p.models || {});
      const activeModels = p.models || {};

      const modelPills = Object.values(allModelsMap)
        .map((m) => {
          const id = m.id || "";
          const enabled = Boolean(activeModels[id]);
          const ctxK = m.context_window ? `${Math.round(m.context_window / 1000)}K` : "";
          return `<button class="chip-btn ${enabled ? "active" : ""}" type="button" data-toggle-prov-model="${esc(p.id)}" data-model-id="${esc(id)}">
            <span>${enabled ? "✓" : "+"} ${esc(m.name || id)}</span>
            <span class="mono" style="opacity:0.75;">${esc(id)}${ctxK ? ` · ${ctxK}` : ""}${m.supports_thinking ? " 🧠" : ""}</span>
          </button>`;
        })
        .join("");

      const timeouts = [30, 60, 120, 180, 300];
      const curTimeout = p.timeout_sec || 120;

      return `<div class="cfg-section-card" data-prov-card="${esc(p.id)}">
        <div class="cfg-section-head">
          <div>
            <div class="role-title-line">
              <span class="role-id-badge">${esc(p.id)}</span>
              <h4 class="cfg-section-title" style="margin:0;">${esc(p.name || p.id)}</h4>
              <span class="status-pill ok">${esc(p.protocol)}</span>
            </div>
          </div>
          <div class="row-actions">
            ${provList.length > 1 ? `<button class="danger-xs-btn" type="button" data-delete-provider="${esc(p.id)}">移除服务商</button>` : ""}
          </div>
        </div>

        <div class="settings-form-row">
          <div class="settings-row-info">
            <strong>通信协议 (Wire Protocol)</strong>
            <span>点选四套原生 SSE 流式线协议适配器</span>
          </div>
          <div class="pill-bar">
            ${["anthropic", "openai-responses", "openai-compat", "gemini"]
              .map(
                (proto) => `<button class="pill-opt ${p.protocol === proto ? "active" : ""}" type="button" data-set-prov-proto="${esc(p.id)}" data-proto="${proto}">${proto}</button>`
              )
              .join("")}
          </div>
        </div>

        <div class="settings-form-row">
          <div class="settings-row-info">
            <strong>接口根地址 (Endpoint Base URL)</strong>
            <span>直接从常用云端或本机网关地址下拉选择</span>
          </div>
          <select class="mac-select mono" data-set-prov-url="${esc(p.id)}" style="min-width:310px;">
            ${candidateURLs.map((u) => `<option value="${esc(u)}" ${u === p.base_url ? "selected" : ""}>${esc(u)}</option>`).join("")}
          </select>
        </div>

        <div class="settings-form-row">
          <div class="settings-row-info">
            <strong>凭据来源 (API Key Source)</strong>
            <span>下拉选择环境变量 (env:) 或 macOS 钥匙串 (keychain:)，无需明文暴露密钥</span>
          </div>
          <div class="row-actions">
            <select class="mac-select mono" data-set-prov-keyref="${esc(p.id)}" style="min-width:240px;">
              ${candidateKeyRefs.map((kr) => `<option value="${esc(kr)}" ${kr === curKeyRef ? "selected" : ""}>${esc(kr)}</option>`).join("")}
            </select>
          </div>
        </div>

        <div class="settings-form-row">
          <div class="settings-row-info">
            <strong>请求超时上限 (Timeout)</strong>
            <span>单步流式请求超时秒数</span>
          </div>
          <div class="pill-bar">
            ${timeouts
              .map(
                (sec) => `<button class="pill-opt ${curTimeout === sec ? "active" : ""}" type="button" data-set-prov-timeout="${esc(p.id)}" data-sec="${sec}">${sec}s</button>`
              )
              .join("")}
          </div>
        </div>

        <div class="cfg-field-label">
          <span>可用模型目录开关 (点击胶囊即可启用/停用对应模型并自动同步至角色下拉菜单)</span>
          <div class="chips-wrap" style="margin-top:6px;">${modelPills}</div>
        </div>
      </div>`;
    })
    .join("");

  return `
    <div class="cfg-section-card">
      <div class="cfg-section-head">
        <div>
          <h3 class="cfg-section-title">一键启用主流模型服务商 (1-Click Provider Catalog)</h3>
          <p class="cfg-section-desc">点击下方任意服务商卡片，即刻自动配置协议、端点地址、默认凭据引用与完整模型目录。</p>
        </div>
      </div>
      <div class="scheme-presets-grid">${presetCardsHTML}</div>
    </div>
    ${provCardsHTML}
  `;
}

function renderConfigWorkflowsPage() {
  const workflows = state.config.workflows || {};
  const wfId = workflows[state.selectedConfigWorkflowId] ? state.selectedConfigWorkflowId : Object.keys(workflows)[0] || "hybrid-fleet";
  state.selectedConfigWorkflowId = wfId;
  const wf = workflows[wfId] || { id: wfId, name: wfId, topology: wfId, max_concurrency: 4, use_worktrees: true, nodes: [], edges: [] };
  const roles = Object.keys(state.config.agents || {}).sort();
  const allRefs = getAllProviderModelRefs();

  const topoSelectCards = SIX_TOPOLOGIES.map(
    (tp) => `<button class="scheme-card ${tp.id === wfId ? "active" : ""}" type="button" data-select-cfg-wf="${esc(tp.id)}">
      <div class="scheme-card-top">
        <strong>${esc(tp.zh)}</strong>
        <span class="mono" style="font-size:10.5px;">${esc(tp.id)}</span>
      </div>
      <span>${esc(tp.desc)}</span>
    </button>`
  ).join("");

  const verifyCommands = [
    "go test ./...",
    "CGO_ENABLED=0 go test -race ./...",
    "npm test",
    "pnpm test",
    "pytest",
    "cargo test",
    "make test",
    ""
  ];
  const curVerify = wf.verify_command || "";

  const nodeRows = (wf.nodes || [])
    .map((nd, idx) => {
      const roleOverride = (wf.role_models && (wf.role_models[nd.id] || wf.role_models[nd.role])) || "";
      return `<div class="settings-form-row">
        <div class="settings-row-info">
          <strong>节点 <code class="mono">${esc(nd.id)}</code> · ${esc(nd.label || nd.id)}</strong>
          <span>绑定执行角色、可选节点模型覆盖与独立 Git Worktree 开关</span>
        </div>
        <div class="row-actions">
          <select class="mac-select" data-wf-node-role="${esc(wf.id)}" data-node-idx="${idx}" title="绑定角色">
            ${roles.map((r) => `<option value="${esc(r)}" ${r === nd.role ? "selected" : ""}>角色: ${esc(r)}</option>`).join("")}
          </select>
          <select class="mac-select mono" data-wf-role-model="${esc(wf.id)}" data-role-key="${esc(nd.role)}" title="角色模型覆盖">
            <option value="">跟随 ${esc(nd.role)} 默认模型</option>
            ${allRefs.map((m) => `<option value="${esc(m.ref)}" ${m.ref === roleOverride ? "selected" : ""}>${esc(m.ref)}</option>`).join("")}
          </select>
          <button class="chip-btn ${nd.worktree ? "active" : ""}" type="button" data-wf-node-wt="${esc(wf.id)}" data-node-idx="${idx}">
            ${nd.worktree ? "🌿 Worktree 隔离" : "📂 共享工作区"}
          </button>
        </div>
      </div>`;
    })
    .join("");

  return `
    <div class="cfg-section-card">
      <div class="cfg-section-head">
        <div>
          <h3 class="cfg-section-title">选择要配置的异构多智能体编排拓扑 (workflows/*.toml)</h3>
          <p class="cfg-section-desc">所有拓扑参数、并发度、验收命令及每个节点的角色/模型均通过点选与下拉菜单直接修改。</p>
        </div>
      </div>
      <div class="scheme-presets-grid">${topoSelectCards}</div>
    </div>

    <div class="cfg-section-card">
      <div class="cfg-section-head">
        <div>
          <h3 class="cfg-section-title">${esc(wf.name || wf.id)} (<code class="mono">workflows/${esc(wf.id)}.toml</code>)</h3>
          <p class="cfg-section-desc">${esc(wf.description || "")}</p>
        </div>
      </div>

      <div class="settings-form-row">
        <div class="settings-row-info">
          <strong>Git Worktree 物理隔离与 Refinery 串行合入</strong>
          <span>为写入型节点分配独立分支并在合入前自动变基验证</span>
        </div>
        <div class="pill-bar">
          <button class="pill-opt ${wf.use_worktrees ? "active" : ""}" type="button" data-wf-set-wt="${esc(wf.id)}" data-val="true">🌿 启用 Worktree 隔离</button>
          <button class="pill-opt ${!wf.use_worktrees ? "active" : ""}" type="button" data-wf-set-wt="${esc(wf.id)}" data-val="false">📂 共享主工作区</button>
        </div>
      </div>

      <div class="settings-form-row">
        <div class="settings-row-info">
          <strong>最大并发智能体数 (max_concurrency)</strong>
          <span>同时并行运行的子节点上限</span>
        </div>
        <div class="pill-bar">
          ${[1, 2, 3, 4, 6, 8]
            .map(
              (n) => `<button class="pill-opt ${(wf.max_concurrency || 4) === n ? "active" : ""}" type="button" data-wf-set-conc="${esc(wf.id)}" data-val="${n}">${n} 并发</button>`
            )
            .join("")}
        </div>
      </div>

      <div class="settings-form-row">
        <div class="settings-row-info">
          <strong>最大迭代轮次 / 跳数上限 (max_rounds / max_hops)</strong>
          <span>防止多智能体死循环的收敛硬边界</span>
        </div>
        <div class="pill-bar">
          ${[1, 2, 3, 5, 8, 10]
            .map(
              (n) => `<button class="pill-opt ${(wf.max_rounds || wf.max_idle_rounds || wf.max_hops || 3) === n ? "active" : ""}" type="button" data-wf-set-rounds="${esc(wf.id)}" data-val="${n}">${n} 轮</button>`
            )
            .join("")}
        </div>
      </div>

      <div class="settings-form-row">
        <div class="settings-row-info">
          <strong>Refinery 验收门控命令 (verify_command)</strong>
          <span>合入主分支前在 Seatbelt 沙箱中自动执行的测试命令</span>
        </div>
        <select class="mac-select mono" data-wf-set-verify="${esc(wf.id)}" style="min-width:260px;">
          ${verifyCommands
            .map((cmd) => `<option value="${esc(cmd)}" ${cmd === curVerify ? "selected" : ""}>${cmd ? esc(cmd) : "（无验收命令 / 仅盲审）"}</option>`)
            .join("")}
        </select>
      </div>

      <div class="settings-form-row">
        <div class="settings-row-info">
          <strong>单次编排预算上限 (budget_usd)</strong>
          <span>超出预算自动安全熔断</span>
        </div>
        <div class="pill-bar">
          ${[0, 1, 3, 5, 10, 25]
            .map(
              (b) => `<button class="pill-opt ${Number(wf.budget_usd || 0) === b ? "active" : ""}" type="button" data-wf-set-budget="${esc(wf.id)}" data-val="${b}">${b === 0 ? "不限 $0" : `$${b}`}</button>`
            )
            .join("")}
        </div>
      </div>

      <div style="margin-top:6px;">
        <strong style="font-size:12.5px;">拓扑节点角色与模型映射 (Nodes &amp; Role Models)</strong>
        ${nodeRows}
      </div>
    </div>
  `;
}

function renderConfigGlobalAndRulesPage() {
  const g = state.config.global || {};
  const rules = state.config.rules || {};
  const provIds = Object.keys(state.config.providers || {}).sort();
  const defModels = getModelsForProvider(g.default_provider || "anthropic");

  const permModes = [
    { id: "auto", zh: "🛡️ 智能安全门控 (auto)", desc: "工作区内常规读写自动放行，高危命令与域外访问弹窗审批" },
    { id: "accept_edits", zh: "✏️ 自动接受编辑 (accept_edits)", desc: "文件修改自动放行，终端命令执行需确认" },
    { id: "ask", zh: "🙋 每次操作询问 (ask)", desc: "所有文件写入与命令执行均需人工确认" },
    { id: "plan", zh: "📐 只读规划模式 (plan)", desc: "严格禁止任何文件修改与写命令，仅用于架构分析" },
    { id: "yolo", zh: "⚡ 全放行模式 (yolo)", desc: "除 ~/.gorness 金库硬底线外全部自动放行" }
  ];

  const sandboxModes = [
    { id: "strict", zh: "🔒 严格白名单沙箱 (strict)", desc: "Seatbelt 限制写工作区 + 仅放行域名白名单网络代理" },
    { id: "readonly", zh: "👁️ 严格只读沙箱 (readonly)", desc: "内核级禁止一切磁盘写入" },
    { id: "network_allow", zh: "🌐 允许外网沙箱 (network_allow)", desc: "Seatbelt 限制文件写入，但允许外部网络访问" },
    { id: "off", zh: "🔓 关闭沙箱 (off)", desc: "不挂载 sandbox-exec（仅受 PermissionGate 保护）" }
  ];

  const domainSet = new Set(rules.allowed_domains || []);
  const allDomains = Array.from(new Set([...COMMON_DOMAINS, ...(rules.allowed_domains || [])]));

  const allowCmdSet = new Set(rules.allow_commands || []);
  const allAllowCmds = Array.from(new Set([...COMMON_ALLOW_CMDS, ...(rules.allow_commands || [])]));

  const askCmdSet = new Set(rules.ask_commands || []);
  const allAskCmds = Array.from(new Set([...COMMON_ASK_CMDS, ...(rules.ask_commands || [])]));

  const denyCmdSet = new Set(rules.deny_commands || []);
  const allDenyCmds = Array.from(new Set([...COMMON_DENY_CMDS, ...(rules.deny_commands || [])]));

  return `
    <div class="cfg-section-card">
      <div class="cfg-section-head">
        <div>
          <h3 class="cfg-section-title">全局默认模型与并发调度 (config.toml)</h3>
          <p class="cfg-section-desc">所有全局参数均通过下拉菜单与分段胶囊直接点选生效。</p>
        </div>
      </div>

      <div class="settings-form-row">
        <div class="settings-row-info">
          <strong>默认兜底服务商与模型 (default_provider / default_model)</strong>
          <span>当角色未指定特定服务商时使用的全局默认模型</span>
        </div>
        <div class="row-actions">
          <select class="mac-select" data-global-select="default_provider">
            ${provIds.map((pid) => `<option value="${esc(pid)}" ${pid === g.default_provider ? "selected" : ""}>${esc(pid)}</option>`).join("")}
          </select>
          <select class="mac-select mono" data-global-select="default_model">
            ${defModels.map((m) => `<option value="${esc(m.id)}" ${m.id === g.default_model ? "selected" : ""}>${esc(m.id)}</option>`).join("")}
          </select>
        </div>
      </div>

      <div class="settings-form-row">
        <div class="settings-row-info">
          <strong>全局默认推理强度 (default_effort)</strong>
          <span>默认 Adaptive Thinking 推理深度</span>
        </div>
        <div class="pill-bar">
          ${["low", "medium", "high", "xhigh", "max"]
            .map(
              (ef) => `<button class="pill-opt ${(g.default_effort || "high") === ef ? "active" : ""}" type="button" data-global-pill="default_effort" data-val="${ef}">${ef}</button>`
            )
            .join("")}
        </div>
      </div>

      <div class="settings-form-row">
        <div class="settings-row-info">
          <strong>多项目最大并发运行数 (max_concurrency)</strong>
          <span>跨工作区同时活跃的 Agent 运行槽位数</span>
        </div>
        <div class="pill-bar">
          ${[1, 2, 4, 6, 8, 12]
            .map(
              (n) => `<button class="pill-opt ${(g.max_concurrency || 4) === n ? "active" : ""}" type="button" data-global-num="max_concurrency" data-val="${n}">${n}</button>`
            )
            .join("")}
        </div>
      </div>

      <div class="settings-form-row">
        <div class="settings-row-info">
          <strong>单步并行只读工具上限 (max_tool_concurrency)</strong>
          <span>同一推理步内并发执行 read_file / search_code 的协程数</span>
        </div>
        <div class="pill-bar">
          ${[2, 4, 8, 12, 16]
            .map(
              (n) => `<button class="pill-opt ${(g.max_tool_concurrency || 8) === n ? "active" : ""}" type="button" data-global-num="max_tool_concurrency" data-val="${n}">${n}</button>`
            )
            .join("")}
        </div>
      </div>

      <div class="settings-form-row">
        <div class="settings-row-info">
          <strong>单轮最大推理步数 (max_steps)</strong>
          <span>单次任务允许的最大工具调用循环步数</span>
        </div>
        <div class="pill-bar">
          ${[16, 32, 64, 100, 128]
            .map(
              (n) => `<button class="pill-opt ${(g.max_steps || 64) === n ? "active" : ""}" type="button" data-global-num="max_steps" data-val="${n}">${n} 步</button>`
            )
            .join("")}
        </div>
      </div>

      <div class="settings-form-row">
        <div class="settings-row-info">
          <strong>Always-On 事件微批合并窗口 (inbox_debounce_ms)</strong>
          <span>相同 InboxKey 突发事件合并防抖毫秒数</span>
        </div>
        <div class="pill-bar">
          ${[200, 400, 800, 1500, 3000]
            .map(
              (ms) => `<button class="pill-opt ${(g.inbox_debounce_ms || 400) === ms ? "active" : ""}" type="button" data-global-num="inbox_debounce_ms" data-val="${ms}">${ms}ms</button>`
            )
            .join("")}
        </div>
      </div>
    </div>

    <div class="cfg-section-card">
      <h3 class="cfg-section-title">九步权限门控与 macOS Seatbelt 沙箱模式</h3>
      <div class="cfg-field-label">
        <span>默认权限门控模式 (permission_mode — 点击卡片立即切换)</span>
        <div class="scheme-presets-grid" style="margin-top:6px;">
          ${permModes
            .map(
              (pm) => `<button class="scheme-card ${(g.permission_mode || "auto") === pm.id ? "active" : ""}" type="button" data-global-pill="permission_mode" data-val="${pm.id}">
                <strong>${esc(pm.zh)}</strong>
                <span>${esc(pm.desc)}</span>
              </button>`
            )
            .join("")}
        </div>
      </div>

      <div class="cfg-field-label" style="margin-top:8px;">
        <span>macOS 内核沙箱模式 (sandbox_mode — 点击卡片立即切换)</span>
        <div class="scheme-presets-grid" style="margin-top:6px;">
          ${sandboxModes
            .map(
              (sm) => `<button class="scheme-card ${(g.sandbox_mode || "strict") === sm.id ? "active" : ""}" type="button" data-global-pill="sandbox_mode" data-val="${sm.id}">
                <strong>${esc(sm.zh)}</strong>
                <span>${esc(sm.desc)}</span>
              </button>`
            )
            .join("")}
        </div>
      </div>
    </div>

    <div class="cfg-section-card">
      <div class="cfg-section-head">
        <div>
          <h3 class="cfg-section-title">细粒度安全规则与沙箱网络白名单 (rules.toml)</h3>
          <p class="cfg-section-desc">点击胶囊即可开关允许访问的外网域名、自动放行命令、需审批命令与高危拦截命令。</p>
        </div>
      </div>

      <div class="cfg-field-label">
        <span>🌐 允许出站访问的域名白名单 (allowed_domains — 供 Seatbelt 本地代理校验)</span>
        <div class="chips-wrap" style="margin-top:6px;">
          ${allDomains
            .map(
              (d) => `<button class="chip-btn mono ${domainSet.has(d) ? "active" : ""}" type="button" data-rule-chip="allowed_domains" data-val="${esc(d)}">
                <span>${domainSet.has(d) ? "✓" : "+"} ${esc(d)}</span>
              </button>`
            )
            .join("")}
        </div>
      </div>

      <div class="cfg-field-label" style="margin-top:8px;">
        <span>✅ 自动放行的安全命令模式 (allow_commands)</span>
        <div class="chips-wrap" style="margin-top:6px;">
          ${allAllowCmds
            .map(
              (c) => `<button class="chip-btn mono ${allowCmdSet.has(c) ? "active" : ""}" type="button" data-rule-chip="allow_commands" data-val="${esc(c)}">
                <span>${allowCmdSet.has(c) ? "✓" : "+"} ${esc(c)}</span>
              </button>`
            )
            .join("")}
        </div>
      </div>

      <div class="cfg-field-label" style="margin-top:8px;">
        <span>⚠️ 触发人工确认的敏感命令模式 (ask_commands)</span>
        <div class="chips-wrap" style="margin-top:6px;">
          ${allAskCmds
            .map(
              (c) => `<button class="chip-btn warn-chip mono ${askCmdSet.has(c) ? "active" : ""}" type="button" data-rule-chip="ask_commands" data-val="${esc(c)}">
                <span>${askCmdSet.has(c) ? "✓" : "+"} ${esc(c)}</span>
              </button>`
            )
            .join("")}
        </div>
      </div>

      <div class="cfg-field-label" style="margin-top:8px;">
        <span>🛑 绝对拦截的高危破坏命令模式 (deny_commands)</span>
        <div class="chips-wrap" style="margin-top:6px;">
          ${allDenyCmds
            .map(
              (c) => `<button class="chip-btn danger-chip mono ${denyCmdSet.has(c) ? "active" : ""}" type="button" data-rule-chip="deny_commands" data-val="${esc(c)}">
                <span>${denyCmdSet.has(c) ? "✓" : "+"} ${esc(c)}</span>
              </button>`
            )
            .join("")}
        </div>
      </div>
    </div>
  `;
}

function renderConfigMCPPage() {
  const servers = (state.config.mcp && state.config.mcp.servers) || {};
  const cards = MCP_SERVER_PRESETS.map((p) => {
    const existing = servers[p.id];
    const isEnabled = Boolean(existing && existing.enabled !== false);
    return `<button class="scheme-card ${isEnabled ? "active" : ""}" type="button" data-toggle-mcp-preset="${esc(p.id)}">
      <div class="scheme-card-top">
        <strong>${esc(p.zh)}</strong>
        <span class="status-pill ${isEnabled ? "ok" : ""}">${isEnabled ? "已启用 ✓" : "点击启用"}</span>
      </div>
      <span class="mono">${esc(p.command)} ${esc((p.args || []).join(" "))}</span>
      <span>${esc(p.desc)}</span>
    </button>`;
  }).join("");

  return `
    <div class="cfg-section-card">
      <div class="cfg-section-head">
        <div>
          <h3 class="cfg-section-title">一键挂载标准 MCP 工具扩展服务 (mcp.toml)</h3>
          <p class="cfg-section-desc">点击卡片即可一键启用或停用常用 MCP Server，工具超 12 个时自动开启 BM25 按需延迟加载。</p>
        </div>
      </div>
      <div class="scheme-presets-grid">${cards}</div>
    </div>
  `;
}

function renderConfigRawFilesPage() {
  const filePills = (state.configFiles || [])
    .map(
      (f, idx) => `<button class="chip-btn mono ${idx === 0 ? "active" : ""}" type="button" data-pick-raw-vault-file="${esc(f)}">${esc(f)}</button>`
    )
    .join("");

  return `
    <div class="cfg-section-card">
      <div class="cfg-section-head">
        <div>
          <h3 class="cfg-section-title">~/.gorness 零数据库透明文件仓检视器</h3>
          <p class="cfg-section-desc">所有图形化点选操作都会实时生成规范的 TOML / Markdown 文件，你也可以在此点击文件标签直接查看。</p>
        </div>
        <button id="studio-save-vault-file-btn" class="primary-sm-btn" type="button">保存并热加载 ✓</button>
      </div>
      <div class="chips-wrap">${filePills}</div>
      <input id="studio-vault-file-select" type="hidden" value="${esc((state.configFiles && state.configFiles[0]) || "config.toml")}">
      <textarea id="studio-vault-file-editor" class="vault-editor mono" style="min-height:360px;" spellcheck="false"></textarea>
    </div>
  `;
}

async function loadSelectedVaultConfigFile(relPath) {
  if (!relPath) return;
  try {
    const data = await apiJSON(`/api/config/files?rel=${encodeURIComponent(relPath)}`);
    const editor = document.getElementById("vault-file-editor");
    if (editor && document.activeElement !== editor) {
      editor.value = data.content || "";
    }
    const studioEditor = document.getElementById("studio-vault-file-editor");
    const studioSel = document.getElementById("studio-vault-file-select");
    if (studioSel) studioSel.value = data.rel_path || relPath;
    if (studioEditor && document.activeElement !== studioEditor) {
      studioEditor.value = data.content || "";
    }
  } catch {
    // Ignore
  }
}

async function loadWorkspaceSuggestions() {
  const listEl = document.getElementById("ws-suggestions-list");
  if (!listEl) return;
  try {
    const data = await apiJSON("/api/workspaces/suggestions");
    state.wsSuggestions = data.suggestions || [];
    if (state.wsSuggestions.length === 0) {
      listEl.innerHTML = `<div class="tiny-muted">未在常用目录发现未挂载仓库，请点击上方「📁 选择本地文件夹…」按钮选择。</div>`;
      return;
    }
    listEl.innerHTML = state.wsSuggestions
      .map(
        (s) => `<button class="ws-sug-card ${s.registered ? "registered" : ""}" type="button" data-mount-suggested-ws="${esc(s.path)}" data-ws-name="${esc(s.name)}">
          <div class="scheme-card-top">
            <strong>${s.is_git ? "🌿 " : "📁 "}${esc(s.name)}</strong>
            <span class="meta-chip">${s.registered ? "已挂载 ✓" : "点击挂载"}</span>
          </div>
          <span class="mono tiny-muted" style="font-size:10.5px;word-break:break-all;">${esc(s.path)}</span>
        </button>`
      )
      .join("");
  } catch {
    listEl.innerHTML = "";
  }
}

function renderPreviewTabList() {
  const sel = document.getElementById("preview-artifact-select");
  if (!sel) return;
  if (!state.previews || state.previews.length === 0) {
    sel.innerHTML = `<option value="">— No HTML Previews Minted Yet —</option>`;
    return;
  }
  sel.innerHTML = state.previews
    .map((p) => `<option value="${esc(p.url)}">${esc(p.title)} (${esc(p.token.slice(0, 8))})</option>`)
    .join("");
  loadPreviewIntoIframe(sel.value);
}

function loadPreviewIntoIframe(previewURL) {
  const frame = document.getElementById("inspector-preview-frame");
  const link = document.getElementById("open-preview-standalone");
  if (!frame || !previewURL) return;
  frame.setAttribute("sandbox", "allow-scripts");
  frame.src = previewURL;
  if (link) link.href = previewURL;
}

// ============================================================================
// Always-On Indicators & Modal Rendering
// ============================================================================

function renderAlwaysOnIndicators() {
  const dot = document.getElementById("alwayson-dot");
  const badge = document.getElementById("alwayson-count");
  const ao = state.alwaysOn || {};
  const activeCount = Number(ao.queued_count || 0) + Number(ao.running_count || 0) + Number(ao.deferred_count || 0);

  if (dot) dot.className = `pulse-dot ${ao.running ? "live" : "idle"}`;
  if (badge) {
    badge.textContent = String(activeCount);
    badge.classList.toggle("hidden", activeCount === 0);
  }

  const timersList = document.getElementById("ao-timers-list");
  if (timersList) {
    timersList.innerHTML = (ao.timers || [])
      .map(
        (tm) => `<div class="ao-item mono">[${esc(tm.kind)}] ${esc(tm.cron_expr || "followup")} — ${esc(tm.note || tm.id)}</div>`
      )
      .join("");
  }
  const outboxList = document.getElementById("ao-outbox-list");
  if (outboxList) {
    outboxList.innerHTML = (ao.outbox || [])
      .slice()
      .reverse()
      .slice(0, 25)
      .map(
        (ob) => `<div class="ao-item"><strong>[${esc(ob.outcome)}]</strong> ${esc(ob.title || "")} <div style="color:var(--ink-secondary);">${esc(ob.body || "")}</div></div>`
      )
      .join("");
  }
}

// ============================================================================
// Resizable Splitters & Event Delegation
// ============================================================================

function setupResizableSplitters() {
  const leftSplit = document.getElementById("left-splitter");
  const rightSplit = document.getElementById("right-splitter");
  const rootStyle = document.documentElement.style;

  let dragging = null;
  leftSplit?.addEventListener("mousedown", () => {
    dragging = "left";
  });
  rightSplit?.addEventListener("mousedown", () => {
    dragging = "right";
  });
  window.addEventListener("mousemove", (e) => {
    if (!dragging) return;
    if (dragging === "left") {
      const w = Math.max(210, Math.min(440, e.clientX));
      rootStyle.setProperty("--left-width", `${w}px`);
    } else if (dragging === "right") {
      const w = Math.max(280, Math.min(580, window.innerWidth - e.clientX));
      rootStyle.setProperty("--right-width", `${w}px`);
    }
  });
  window.addEventListener("mouseup", () => {
    dragging = null;
  });
}

function bindWorkbenchEvents() {
  // Center Mode Switch & Pop-up Settings Dialog Triggers
  document.getElementById("mode-chat-btn")?.addEventListener("click", () => setCenterMode("chat"));
  document.getElementById("mode-fleet-btn")?.addEventListener("click", () => setCenterMode("fleet"));
  document.getElementById("mode-config-btn")?.addEventListener("click", () => openSettingsModal("matrix"));
  document.getElementById("open-settings-top-btn")?.addEventListener("click", () => openSettingsModal("matrix"));

  // Theme & Bilingual Toggle
  document.querySelectorAll(".theme-btn").forEach((btn) => {
    btn.addEventListener("click", () => applyTheme(btn.dataset.themeVal || "auto", true));
  });
  document.getElementById("lang-toggle-btn")?.addEventListener("click", () => {
    state.lang = state.lang === "zh" ? "en" : "zh";
    localStorage.setItem("gorness.lang", state.lang);
    applyI18nInPlace();
    populateAgentRoleSelect();
    renderSidebarTree();
    renderApprovalBanners();
    renderSnapshotTimeMachine();
    renderAgentMatrixAndVaultEditor();
    renderConfigStudio();
    renderFleetStudio();
    apiJSON("/api/config/global", {
      method: "POST",
      body: JSON.stringify({ language: state.lang })
    }).catch(() => {});
  });

  // Inspector Tabs & Config Studio Launchers
  document.querySelectorAll(".insp-tab").forEach((btn) => {
    btn.addEventListener("click", () => {
      const tab = btn.dataset.tab || "files";
      setInspectorTab(tab);
      if (tab === "config") {
        openSettingsModal("matrix");
      }
    });
  });
  document.getElementById("open-config-studio-btn")?.addEventListener("click", () => {
    openSettingsModal("matrix");
  });
  document.getElementById("expand-full-config-btn")?.addEventListener("click", () => {
    openSettingsModal("matrix");
  });

  // Modals open/close
  document.getElementById("add-workspace-btn")?.addEventListener("click", () => {
    loadWorkspaceSuggestions();
    openModal("workspace-modal");
  });
  document.getElementById("pick-native-folder-btn")?.addEventListener("click", async () => {
    try {
      const res = await apiJSON("/api/workspaces/pick-folder", { method: "POST" });
      if (res && !res.cancelled && res.path) {
        const ws = await apiJSON("/api/workspaces", {
          method: "POST",
          body: JSON.stringify({ path: res.path, name: res.name || "" })
        });
        closeModal("workspace-modal");
        state.activeWorkspaceId = ws.id;
        await loadBootstrap();
      }
    } catch {
      // Ignore if cancelled
    }
  });
  document.getElementById("new-mission-modal-btn")?.addEventListener("click", () => {
    renderTopologyCardsGrid();
    openModal("fleet-launcher-modal");
  });
  document.getElementById("launch-fleet-quick-btn")?.addEventListener("click", () => {
    renderTopologyCardsGrid();
    openModal("fleet-launcher-modal");
  });
  document.getElementById("alwayson-drawer-btn")?.addEventListener("click", async () => {
    await refreshActiveStatusQuietly();
    openModal("alwayson-modal");
  });
  document.querySelectorAll(".close-modal-btn").forEach((btn) => {
    btn.addEventListener("click", () => closeModal(btn.dataset.close));
  });

  // Register Workspace Form
  document.getElementById("register-ws-form")?.addEventListener("submit", async (e) => {
    e.preventDefault();
    const path = document.getElementById("ws-path-input")?.value.trim();
    const name = document.getElementById("ws-name-input")?.value.trim();
    if (!path) return;
    const ws = await apiJSON("/api/workspaces", {
      method: "POST",
      body: JSON.stringify({ path, name })
    });
    closeModal("workspace-modal");
    state.activeWorkspaceId = ws.id;
    await loadBootstrap();
  });

  // New Session
  document.getElementById("new-session-btn")?.addEventListener("click", async () => {
    if (!state.activeWorkspaceId) {
      loadWorkspaceSuggestions();
      openModal("workspace-modal");
      return;
    }
    const role = document.getElementById("agent-role-select")?.value || "coder";
    const meta = await apiJSON("/api/sessions", {
      method: "POST",
      body: JSON.stringify({
        workspace_id: state.activeWorkspaceId,
        title: "New Session",
        agent_role: role
      })
    });
    state.sessions.unshift(meta);
    await selectSession(meta.id);
  });

  // Session Header Controls (Zero-Typing Role, Model Override & Permission Dropdowns)
  document.getElementById("session-title-input")?.addEventListener("change", async (e) => {
    if (!state.activeSessionId) return;
    await apiJSON(`/api/sessions/${encodeURIComponent(state.activeSessionId)}`, {
      method: "PATCH",
      body: JSON.stringify({ title: e.target.value })
    });
    await refreshActiveStatusQuietly();
  });
  document.getElementById("agent-role-select")?.addEventListener("change", async (e) => {
    if (!state.activeSessionId) return;
    await apiJSON(`/api/sessions/${encodeURIComponent(state.activeSessionId)}`, {
      method: "PATCH",
      body: JSON.stringify({ agent_role: e.target.value })
    });
    await refreshActiveStatusQuietly();
  });
  document.getElementById("model-override-input")?.addEventListener("change", async (e) => {
    if (!state.activeSessionId) return;
    await apiJSON(`/api/sessions/${encodeURIComponent(state.activeSessionId)}`, {
      method: "PATCH",
      body: JSON.stringify({ model_override: e.target.value })
    });
  });
  document.getElementById("compact-session-btn")?.addEventListener("click", () => tryExecuteSlashCommand("/compact"));
  document.getElementById("fork-session-btn")?.addEventListener("click", () => tryExecuteSlashCommand("/fork"));
  document.getElementById("export-html-btn")?.addEventListener("click", () => exportCurrentSession("html"));
  document.getElementById("stop-run-btn")?.addEventListener("click", async () => {
    const active = activeRunForCurrentSession();
    if (active) {
      await apiJSON(`/api/runs/${encodeURIComponent(active.run_id)}/stop`, { method: "POST" });
    }
  });

  // Composer IME, Draft Persistence, Autocomplete & Send/Steer
  const composer = document.getElementById("composer-input");
  composer?.addEventListener("compositionstart", () => {
    state.isComposingIME = true;
  });
  composer?.addEventListener("compositionend", () => {
    state.isComposingIME = false;
    updateComposerPalette();
  });
  composer?.addEventListener("input", () => {
    saveDraftLocally();
    updateComposerPalette();
  });
  composer?.addEventListener("keydown", (e) => {
    if (e.key === "Enter" && !e.shiftKey && !state.isComposingIME) {
      e.preventDefault();
      sendPromptOrSteer();
    }
  });
  document.getElementById("send-or-steer-btn")?.addEventListener("click", () => sendPromptOrSteer());

  // Global Session Search (⌘K) & Settings Dialog Shortcut (⌘, / Esc)
  const searchIn = document.getElementById("global-search-input");
  const searchPop = document.getElementById("search-results-popover");
  window.addEventListener("keydown", (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
      e.preventDefault();
      searchIn?.focus();
    } else if ((e.metaKey || e.ctrlKey) && e.key === ",") {
      e.preventDefault();
      openSettingsModal("matrix");
    } else if (e.key === "Escape") {
      ["config-mode-view", "workspace-modal", "fleet-launcher-modal", "alwayson-modal"].forEach((id) => closeModal(id));
    }
  });
  let searchTimer = null;
  searchIn?.addEventListener("input", () => {
    clearTimeout(searchTimer);
    const q = searchIn.value.trim();
    if (!q) {
      searchPop?.classList.add("hidden");
      return;
    }
    searchTimer = setTimeout(async () => {
      const res = await apiJSON(`/api/sessions/search?q=${encodeURIComponent(q)}&limit=12`);
      const hits = res.hits || [];
      if (!searchPop) return;
      if (hits.length === 0) {
        searchPop.innerHTML = `<div class="search-hit-item"><div class="search-hit-snippet">No matches</div></div>`;
      } else {
        searchPop.innerHTML = hits
          .map(
            (h) => `<div class="search-hit-item" data-select-session="${esc(h.session_id)}">
              <div class="search-hit-title">${esc(h.title)}</div>
              <div class="search-hit-snippet">${esc(h.snippet)}</div>
            </div>`
          )
          .join("");
      }
      searchPop.classList.remove("hidden");
    }, 140);
  });

  // Launch Fleet Mission Form
  document.getElementById("launch-fleet-form")?.addEventListener("submit", async (e) => {
    e.preventDefault();
    if (!state.activeWorkspaceId) {
      closeModal("fleet-launcher-modal");
      openModal("workspace-modal");
      return;
    }
    const goal = document.getElementById("fleet-goal-input")?.value.trim();
    const verifyCmd = document.getElementById("fleet-verify-input")?.value.trim() || "";
    const budget = Number.parseFloat(document.getElementById("fleet-budget-input")?.value || "0") || 0;
    const useWT = Boolean(document.getElementById("fleet-worktrees-cb")?.checked);
    if (!goal) return;

    const snap = await apiJSON("/api/fleet/missions", {
      method: "POST",
      body: JSON.stringify({
        workspace_id: state.activeWorkspaceId,
        workflow_id: state.selectedLaunchTopology,
        topology: state.selectedLaunchTopology,
        goal,
        verify_command: verifyCmd,
        budget_usd: budget,
        use_worktrees: useWT
      })
    });
    closeModal("fleet-launcher-modal");
    state.missions.unshift(snap);
    state.activeMissionId = snap.id;
    state.selectedNodeId = (snap.nodes && snap.nodes[0] && snap.nodes[0].id) || "";
    setCenterMode("fleet");
    renderFleetMissionSelector();
    renderFleetStudio();
    subscribeToMissionStream(snap.id);
  });

  document.getElementById("mission-select")?.addEventListener("change", (e) => {
    state.activeMissionId = e.target.value;
    state.selectedNodeId = "";
    state.nodeDiffText = "";
    renderFleetStudio();
    const snap = activeMissionSnapshot();
    if (snap && snap.status === "running") subscribeToMissionStream(snap.id);
  });

  document.getElementById("stop-mission-btn")?.addEventListener("click", async () => {
    if (!state.activeMissionId) return;
    await apiJSON(`/api/fleet/missions/${encodeURIComponent(state.activeMissionId)}/stop`, { method: "POST" });
  });

  document.getElementById("node-steer-send-btn")?.addEventListener("click", async () => {
    const input = document.getElementById("node-steer-input");
    const msg = input?.value.trim();
    if (!msg || !state.activeMissionId || !state.selectedNodeId) return;
    await apiJSON(
      `/api/fleet/missions/${encodeURIComponent(state.activeMissionId)}/nodes/${encodeURIComponent(state.selectedNodeId)}/steer`,
      { method: "POST", body: JSON.stringify({ instruction: msg }) }
    );
    input.value = "";
  });

  // Inspector Tab 1: Files & Safe Copy-Open
  document.getElementById("file-filter-input")?.addEventListener("input", () => renderWorkspaceFilesList());
  document.getElementById("refresh-files-btn")?.addEventListener("click", () => refreshWorkspaceContext(state.activeWorkspaceId));
  document.getElementById("safe-open-file-btn")?.addEventListener("click", async () => {
    if (!state.activeWorkspaceId || !state.viewedFileRel) return;
    await apiJSON("/api/open-safe", {
      method: "POST",
      body: JSON.stringify({ workspace_id: state.activeWorkspaceId, path: state.viewedFileRel })
    });
  });

  // Inspector Tab 3: Config Matrix & Live Vault Editor
  document.getElementById("toggle-add-provider-btn")?.addEventListener("click", () => {
    document.getElementById("quick-provider-form")?.classList.toggle("hidden");
  });
  document.getElementById("quick-provider-form")?.addEventListener("submit", async (e) => {
    e.preventDefault();
    const id = document.getElementById("prov-id-in")?.value.trim();
    const protocol = document.getElementById("prov-proto-in")?.value;
    const base_url = document.getElementById("prov-url-in")?.value.trim();
    const api_key = document.getElementById("prov-key-in")?.value.trim();
    const modId = document.getElementById("prov-model-in")?.value.trim();
    const models = modId ? { [modId]: { id: modId, name: modId } } : undefined;
    const snap = await apiJSON("/api/config/providers", {
      method: "POST",
      body: JSON.stringify({ id, name: id, protocol, base_url, api_key, models })
    });
    state.config = snap;
    document.getElementById("quick-provider-form")?.classList.add("hidden");
    populateAgentRoleSelect();
    renderAgentMatrixAndVaultEditor();
  });

  document.getElementById("vault-file-select")?.addEventListener("change", (e) => {
    loadSelectedVaultConfigFile(e.target.value);
  });
  document.getElementById("save-vault-file-btn")?.addEventListener("click", async () => {
    const rel_path = document.getElementById("vault-file-select")?.value;
    const content = document.getElementById("vault-file-editor")?.value || "";
    const statusEl = document.getElementById("vault-save-status");
    if (!rel_path) return;
    try {
      const res = await apiJSON("/api/config/files", {
        method: "PUT",
        body: JSON.stringify({ rel_path, content })
      });
      state.config = res.snapshot || state.config;
      state.diagnostic = null;
      renderDiagnosticBanner();
      populateAgentRoleSelect();
      if (statusEl) statusEl.textContent = `Hot-reloaded ${rel_path} ✓`;
    } catch (err) {
      if (statusEl) statusEl.textContent = `Error: ${err.message}`;
    }
  });

  // Inspector Tab 4: Isolated Sandboxed Preview
  document.getElementById("preview-artifact-select")?.addEventListener("change", (e) => {
    loadPreviewIntoIframe(e.target.value);
  });
  document.getElementById("new-html-snippet-btn")?.addEventListener("click", () => {
    document.getElementById("custom-preview-composer")?.classList.toggle("hidden");
  });
  document.getElementById("mint-preview-btn")?.addEventListener("click", async () => {
    const title = document.getElementById("custom-preview-title")?.value.trim() || "Sandbox Artifact";
    const html = document.getElementById("custom-preview-html")?.value || "<h1>Sandbox</h1>";
    const created = await apiJSON("/api/previews", {
      method: "POST",
      body: JSON.stringify({
        workspace_id: state.activeWorkspaceId,
        title,
        html,
        save_name: title
      })
    });
    state.previews.unshift(created);
    renderPreviewTabList();
    loadPreviewIntoIframe(created.url);
    document.getElementById("custom-preview-composer")?.classList.add("hidden");
  });

  // Always-On Modal Actions & Preset Selectors
  document.getElementById("timer-note-preset")?.addEventListener("change", (e) => {
    const noteIn = document.getElementById("timer-note-input");
    if (noteIn && e.target.value) noteIn.value = e.target.value;
  });
  document.getElementById("add-timer-form")?.addEventListener("submit", async (e) => {
    e.preventDefault();
    if (!state.activeWorkspaceId) return;
    const cron = document.getElementById("timer-cron-input")?.value.trim();
    const note = document.getElementById("timer-note-input")?.value.trim();
    await apiJSON("/api/alwayson/timers", {
      method: "POST",
      body: JSON.stringify({
        workspace_id: state.activeWorkspaceId,
        kind: cron ? "cron" : "followup",
        cron_expr: cron,
        after_seconds: 60,
        note
      })
    });
    await refreshActiveStatusQuietly();
  });
  document.getElementById("send-webhook-form")?.addEventListener("submit", async (e) => {
    e.preventDefault();
    if (!state.activeWorkspaceId) return;
    const text = document.getElementById("webhook-text-input")?.value.trim();
    if (!text) return;
    await apiJSON("/api/alwayson/webhook/local", {
      method: "POST",
      body: JSON.stringify({ workspace_id: state.activeWorkspaceId, text })
    });
    document.getElementById("webhook-text-input").value = "";
    await refreshActiveStatusQuietly();
  });
  document.getElementById("trigger-dream-btn")?.addEventListener("click", async () => {
    await apiJSON("/api/alwayson/dream", {
      method: "POST",
      body: JSON.stringify({ workspace_id: state.activeWorkspaceId })
    });
    await refreshActiveStatusQuietly();
  });

  // Delegated Clicks (Sessions, Approvals, SVG Nodes, Palette, Time-Machine, Zero-Typing Config Studio)
  document.addEventListener("click", async (e) => {
    const target = e.target;
    if (!(target instanceof Element)) return;

    if (target.classList.contains("modal-backdrop") && target.id) {
      closeModal(target.id);
      return;
    }

    const openPageBtn = target.closest("[data-open-settings-page]");
    if (openPageBtn) {
      openSettingsModal(openPageBtn.getAttribute("data-open-settings-page") || "matrix");
      return;
    }

    // 1. Config Studio Sub-Navigation Pills
    const cfgNavBtn = target.closest("[data-cfg-page]");
    if (cfgNavBtn) {
      state.configPage = cfgNavBtn.getAttribute("data-cfg-page") || "matrix";
      renderConfigStudio();
      return;
    }

    // 2. 1-Click Discovered Workspace Mount
    const sugWsBtn = target.closest("[data-mount-suggested-ws]");
    if (sugWsBtn) {
      const path = sugWsBtn.getAttribute("data-mount-suggested-ws");
      const name = sugWsBtn.getAttribute("data-ws-name") || "";
      if (path) {
        const ws = await apiJSON("/api/workspaces", {
          method: "POST",
          body: JSON.stringify({ path, name })
        });
        closeModal("workspace-modal");
        state.activeWorkspaceId = ws.id;
        await loadBootstrap();
      }
      return;
    }

    // 3. Fleet Launcher Goal Preset Pills & Webhook Preset Pills
    const goalPresetBtn = target.closest("[data-goal-preset]");
    if (goalPresetBtn) {
      const goalIn = document.getElementById("fleet-goal-input");
      if (goalIn) goalIn.value = goalPresetBtn.getAttribute("data-goal-preset") || "";
      return;
    }
    const whPresetBtn = target.closest("[data-wh-preset]");
    if (whPresetBtn && state.activeWorkspaceId) {
      const text = whPresetBtn.getAttribute("data-wh-preset") || "";
      if (text) {
        await apiJSON("/api/alwayson/webhook/local", {
          method: "POST",
          body: JSON.stringify({ workspace_id: state.activeWorkspaceId, text })
        });
        await refreshActiveStatusQuietly();
      }
      return;
    }

    // 4. 1-Click Matrix Scheme Cards
    const schemeBtn = target.closest("[data-apply-scheme]");
    if (schemeBtn) {
      const schemeId = schemeBtn.getAttribute("data-apply-scheme");
      if (schemeId) await applyMatrixScheme(schemeId);
      return;
    }

    // 5. 1-Click Provider Preset Cards
    const provPresetBtn = target.closest("[data-enable-provider-preset]");
    if (provPresetBtn) {
      const presetId = provPresetBtn.getAttribute("data-enable-provider-preset");
      if (presetId) await applyProviderPreset(presetId);
      return;
    }

    // 6. Provider Card Pill Controls (Protocol, Timeout, Model Toggle, Delete)
    const setProvProtoBtn = target.closest("[data-set-prov-proto]");
    if (setProvProtoBtn) {
      const pid = setProvProtoBtn.getAttribute("data-set-prov-proto");
      const protocol = setProvProtoBtn.getAttribute("data-proto");
      if (pid && protocol) {
        state.config = await apiJSON("/api/config/providers", {
          method: "POST",
          body: JSON.stringify({ id: pid, protocol })
        });
        populateAgentRoleSelect();
        renderAgentMatrixAndVaultEditor();
        renderConfigStudio();
        flashAutoSaveIndicator(`已切换 ${pid} 协议为 ${protocol} ✓`);
      }
      return;
    }

    const setProvTimeoutBtn = target.closest("[data-set-prov-timeout]");
    if (setProvTimeoutBtn) {
      const pid = setProvTimeoutBtn.getAttribute("data-set-prov-timeout");
      const timeout_sec = Number.parseInt(setProvTimeoutBtn.getAttribute("data-sec") || "120", 10);
      if (pid && timeout_sec > 0) {
        state.config = await apiJSON("/api/config/providers", {
          method: "POST",
          body: JSON.stringify({ id: pid, timeout_sec })
        });
        renderConfigStudio();
        flashAutoSaveIndicator(`已设置 ${pid} 超时为 ${timeout_sec}s ✓`);
      }
      return;
    }

    const toggleProvModelBtn = target.closest("[data-toggle-prov-model]");
    if (toggleProvModelBtn) {
      const pid = toggleProvModelBtn.getAttribute("data-toggle-prov-model");
      const modId = toggleProvModelBtn.getAttribute("data-model-id");
      const curProv = state.config && state.config.providers && state.config.providers[pid];
      if (pid && modId && curProv) {
        const preset = PROVIDER_PRESETS[pid] || PROVIDER_PRESETS.openai;
        const nextModels = Object.assign({}, curProv.models || {});
        if (nextModels[modId]) {
          if (Object.keys(nextModels).length > 1) delete nextModels[modId];
        } else {
          nextModels[modId] = (preset.models && preset.models[modId]) || { id: modId, name: modId, context_window: 200000 };
        }
        state.config = await apiJSON("/api/config/providers", {
          method: "POST",
          body: JSON.stringify({ id: pid, models: nextModels })
        });
        populateAgentRoleSelect();
        renderAgentMatrixAndVaultEditor();
        renderConfigStudio();
        flashAutoSaveIndicator(`已更新 ${pid} 模型目录 ✓`);
      }
      return;
    }

    const delProvBtn = target.closest("[data-delete-provider]");
    if (delProvBtn) {
      const pid = delProvBtn.getAttribute("data-delete-provider");
      if (pid) {
        state.config = await apiJSON(`/api/config/providers/${encodeURIComponent(pid)}`, { method: "DELETE" });
        populateAgentRoleSelect();
        renderAgentMatrixAndVaultEditor();
        renderConfigStudio();
        flashAutoSaveIndicator(`已移除服务商 ${pid} ✓`);
      }
      return;
    }

    // 7. Agent Role Matrix Pill Controls (Effort, Read-Only, Tool Chips, Tool Presets, Remove Fallback, Reset Default)
    const roleEffortBtn = target.closest("[data-role-effort]");
    if (roleEffortBtn) {
      const roleId = roleEffortBtn.getAttribute("data-role-effort");
      const effort = roleEffortBtn.getAttribute("data-val");
      if (roleId && effort) await patchAgentRole(roleId, { effort });
      return;
    }

    const setRoleRoBtn = target.closest("[data-set-role-ro]");
    if (setRoleRoBtn) {
      const roleId = setRoleRoBtn.getAttribute("data-set-role-ro");
      const read_only = setRoleRoBtn.getAttribute("data-ro") === "true";
      if (roleId) await patchAgentRole(roleId, { read_only });
      return;
    }

    const toggleAgentRoBtn = target.closest("[data-toggle-agent-ro]");
    if (toggleAgentRoBtn) {
      const roleId = toggleAgentRoBtn.getAttribute("data-toggle-agent-ro");
      const cur = state.config && state.config.agents && state.config.agents[roleId];
      if (roleId && cur) await patchAgentRole(roleId, { read_only: !cur.read_only });
      return;
    }

    const toggleRoleToolBtn = target.closest("[data-toggle-role-tool]");
    if (toggleRoleToolBtn) {
      const roleId = toggleRoleToolBtn.getAttribute("data-toggle-role-tool");
      const toolId = toggleRoleToolBtn.getAttribute("data-tool");
      const cur = state.config && state.config.agents && state.config.agents[roleId];
      if (roleId && toolId && cur) {
        const baseList = Array.isArray(cur.allowed_tools) ? [...cur.allowed_tools] : CORE_TOOLS.map((t) => t.id);
        const idx = baseList.indexOf(toolId);
        if (idx >= 0) baseList.splice(idx, 1);
        else baseList.push(toolId);
        await patchAgentRole(roleId, { allowed_tools: baseList });
      }
      return;
    }

    const roleToolPresetBtn = target.closest("[data-role-tool-preset]");
    if (roleToolPresetBtn) {
      const roleId = roleToolPresetBtn.getAttribute("data-role-tool-preset");
      const preset = roleToolPresetBtn.getAttribute("data-preset");
      if (roleId && preset) {
        if (preset === "all") {
          await patchAgentRole(roleId, { allowed_tools: CORE_TOOLS.map((t) => t.id), read_only: false });
        } else if (preset === "readonly") {
          await patchAgentRole(roleId, { allowed_tools: ["read_file", "search_code"], read_only: true });
        } else if (preset === "noshell") {
          await patchAgentRole(roleId, { allowed_tools: ["read_file", "search_code", "edit_file", "write_file"] });
        } else if (preset === "none") {
          await patchAgentRole(roleId, { allowed_tools: [], read_only: true });
        }
      }
      return;
    }

    const rmFbBtn = target.closest("[data-remove-fallback]");
    if (rmFbBtn) {
      const roleId = rmFbBtn.getAttribute("data-remove-fallback");
      const fb = rmFbBtn.getAttribute("data-fb");
      const cur = state.config && state.config.agents && state.config.agents[roleId];
      if (roleId && fb && cur) {
        const nextFb = (cur.fallbacks || []).filter((x) => x !== fb);
        await patchAgentRole(roleId, { fallbacks: nextFb });
      }
      return;
    }

    const resetRoleBtn = target.closest("[data-reset-role-default]");
    if (resetRoleBtn) {
      const roleId = resetRoleBtn.getAttribute("data-reset-role-default");
      const meta = ROLE_META[roleId];
      if (roleId && meta) {
        await patchAgentRole(roleId, {
          provider: meta.defaultProvider,
          model: meta.defaultModel,
          effort: meta.defaultEffort,
          read_only: meta.readOnly,
          allowed_tools: meta.allowedTools === null ? CORE_TOOLS.map((t) => t.id) : meta.allowedTools,
          system_prompt: meta.prompt
        });
      }
      return;
    }

    // 8. Workflow Topology Config Studio Controls
    const selCfgWfBtn = target.closest("[data-select-cfg-wf]");
    if (selCfgWfBtn) {
      state.selectedConfigWorkflowId = selCfgWfBtn.getAttribute("data-select-cfg-wf") || "hybrid-fleet";
      renderConfigStudio();
      return;
    }

    const wfSetWtBtn = target.closest("[data-wf-set-wt]");
    if (wfSetWtBtn) {
      const wfId = wfSetWtBtn.getAttribute("data-wf-set-wt");
      const wf = state.config && state.config.workflows && state.config.workflows[wfId];
      if (wf) {
        const nextWf = Object.assign({}, wf, { use_worktrees: wfSetWtBtn.getAttribute("data-val") === "true" });
        state.config = await apiJSON("/api/config/workflows", { method: "POST", body: JSON.stringify(nextWf) });
        renderConfigStudio();
        flashAutoSaveIndicator(`拓扑 ${wfId} Worktree 策略已更新 ✓`);
      }
      return;
    }

    const wfSetConcBtn = target.closest("[data-wf-set-conc]");
    if (wfSetConcBtn) {
      const wfId = wfSetConcBtn.getAttribute("data-wf-set-conc");
      const wf = state.config && state.config.workflows && state.config.workflows[wfId];
      if (wf) {
        const nextWf = Object.assign({}, wf, { max_concurrency: Number.parseInt(wfSetConcBtn.getAttribute("data-val") || "4", 10) });
        state.config = await apiJSON("/api/config/workflows", { method: "POST", body: JSON.stringify(nextWf) });
        renderConfigStudio();
        flashAutoSaveIndicator(`拓扑 ${wfId} 并发数已更新 ✓`);
      }
      return;
    }

    const wfSetRoundsBtn = target.closest("[data-wf-set-rounds]");
    if (wfSetRoundsBtn) {
      const wfId = wfSetRoundsBtn.getAttribute("data-wf-set-rounds");
      const wf = state.config && state.config.workflows && state.config.workflows[wfId];
      if (wf) {
        const n = Number.parseInt(wfSetRoundsBtn.getAttribute("data-val") || "3", 10);
        const patch = wf.topology === "shared-board" ? { max_idle_rounds: n } : wf.topology === "event-pipeline" ? { max_hops: n } : { max_rounds: n };
        const nextWf = Object.assign({}, wf, patch);
        state.config = await apiJSON("/api/config/workflows", { method: "POST", body: JSON.stringify(nextWf) });
        renderConfigStudio();
        flashAutoSaveIndicator(`拓扑 ${wfId} 收敛轮次已更新 ✓`);
      }
      return;
    }

    const wfSetBudgetBtn = target.closest("[data-wf-set-budget]");
    if (wfSetBudgetBtn) {
      const wfId = wfSetBudgetBtn.getAttribute("data-wf-set-budget");
      const wf = state.config && state.config.workflows && state.config.workflows[wfId];
      if (wf) {
        const nextWf = Object.assign({}, wf, { budget_usd: Number.parseFloat(wfSetBudgetBtn.getAttribute("data-val") || "0") });
        state.config = await apiJSON("/api/config/workflows", { method: "POST", body: JSON.stringify(nextWf) });
        renderConfigStudio();
        flashAutoSaveIndicator(`拓扑 ${wfId} 预算上限已更新 ✓`);
      }
      return;
    }

    const wfNodeWtBtn = target.closest("[data-wf-node-wt]");
    if (wfNodeWtBtn) {
      const wfId = wfNodeWtBtn.getAttribute("data-wf-node-wt");
      const idx = Number.parseInt(wfNodeWtBtn.getAttribute("data-node-idx") || "0", 10);
      const wf = state.config && state.config.workflows && state.config.workflows[wfId];
      if (wf && Array.isArray(wf.nodes) && wf.nodes[idx]) {
        const nextNodes = wf.nodes.map((n, i) => (i === idx ? Object.assign({}, n, { worktree: !n.worktree }) : n));
        const nextWf = Object.assign({}, wf, { nodes: nextNodes });
        state.config = await apiJSON("/api/config/workflows", { method: "POST", body: JSON.stringify(nextWf) });
        renderConfigStudio();
        flashAutoSaveIndicator(`节点 ${wf.nodes[idx].id} Worktree 开关已更新 ✓`);
      }
      return;
    }

    // 9. Global Settings & Security Rule Chips & MCP Preset Cards
    const globalPillBtn = target.closest("[data-global-pill]");
    if (globalPillBtn) {
      const key = globalPillBtn.getAttribute("data-global-pill");
      const val = globalPillBtn.getAttribute("data-val");
      if (key && val) {
        state.config = await apiJSON("/api/config/global", { method: "POST", body: JSON.stringify({ [key]: val }) });
        if (key === "permission_mode") {
          const permSel = document.getElementById("perm-mode-select");
          if (permSel) permSel.value = val;
        }
        renderConfigStudio();
        flashAutoSaveIndicator(`全局配置 ${key} 已切换为 ${val} ✓`);
      }
      return;
    }

    const globalNumBtn = target.closest("[data-global-num]");
    if (globalNumBtn) {
      const key = globalNumBtn.getAttribute("data-global-num");
      const val = Number.parseInt(globalNumBtn.getAttribute("data-val") || "0", 10);
      if (key) {
        state.config = await apiJSON("/api/config/global", { method: "POST", body: JSON.stringify({ [key]: val }) });
        renderConfigStudio();
        flashAutoSaveIndicator(`全局配置 ${key} = ${val} ✓`);
      }
      return;
    }

    const ruleChipBtn = target.closest("[data-rule-chip]");
    if (ruleChipBtn) {
      const field = ruleChipBtn.getAttribute("data-rule-chip");
      const val = ruleChipBtn.getAttribute("data-val");
      if (field && val) await toggleRuleChip(field, val);
      return;
    }

    const mcpPresetBtn = target.closest("[data-toggle-mcp-preset]");
    if (mcpPresetBtn) {
      const presetId = mcpPresetBtn.getAttribute("data-toggle-mcp-preset");
      if (presetId) await toggleMCPPreset(presetId);
      return;
    }

    const rawFilePill = target.closest("[data-pick-raw-vault-file]");
    if (rawFilePill) {
      const rel = rawFilePill.getAttribute("data-pick-raw-vault-file");
      document.querySelectorAll("[data-pick-raw-vault-file]").forEach((b) => {
        b.classList.toggle("active", b.getAttribute("data-pick-raw-vault-file") === rel);
      });
      await loadSelectedVaultConfigFile(rel);
      return;
    }

    if (target.closest("#studio-save-vault-file-btn")) {
      const rel_path = document.getElementById("studio-vault-file-select")?.value;
      const content = document.getElementById("studio-vault-file-editor")?.value || "";
      if (rel_path) {
        const res = await apiJSON("/api/config/files", {
          method: "PUT",
          body: JSON.stringify({ rel_path, content })
        });
        state.config = res.snapshot || state.config;
        state.diagnostic = null;
        renderDiagnosticBanner();
        populateAgentRoleSelect();
        renderAgentMatrixAndVaultEditor();
        flashAutoSaveIndicator(`已保存并热加载 ${rel_path} ✓`);
      }
      return;
    }

    const pinBtn = target.closest("[data-pin-session]");
    if (pinBtn) {
      e.stopPropagation();
      const sid = pinBtn.getAttribute("data-pin-session");
      const nowPinned = pinBtn.getAttribute("data-pinned") !== "1";
      await apiJSON(`/api/sessions/${encodeURIComponent(sid)}`, {
        method: "PATCH",
        body: JSON.stringify({ pinned: nowPinned })
      });
      await refreshActiveStatusQuietly();
      return;
    }

    const wsHead = target.closest("[data-select-ws]");
    if (wsHead) {
      const wsId = wsHead.getAttribute("data-select-ws");
      state.activeWorkspaceId = wsId;
      renderSidebarTree();
      refreshWorkspaceContext(wsId);
      return;
    }

    const sessRow = target.closest("[data-select-session]");
    if (sessRow) {
      searchPop?.classList.add("hidden");
      const sid = sessRow.getAttribute("data-select-session");
      if (sid) await selectSession(sid);
      return;
    }

    const jumpCard = target.closest("[data-jump-session]");
    if (jumpCard && !target.closest("[data-resolve-deferred]")) {
      const sid = jumpCard.getAttribute("data-jump-session");
      if (sid) await selectSession(sid);
      return;
    }

    const defBtn = target.closest("[data-resolve-deferred]");
    if (defBtn) {
      const id = defBtn.getAttribute("data-resolve-deferred");
      const approved = defBtn.getAttribute("data-approve") === "true";
      await apiJSON(`/api/alwayson/deferred/${encodeURIComponent(id)}/resolve`, {
        method: "POST",
        body: JSON.stringify({ approved })
      });
      await refreshActiveStatusQuietly();
      return;
    }

    const apprBtn = target.closest("[data-appr-resolve]");
    if (apprBtn) {
      const id = apprBtn.getAttribute("data-appr-resolve");
      const action = apprBtn.getAttribute("data-action");
      const fbIn = document.querySelector(`[data-appr-feedback="${CSS.escape(id)}"]`);
      const feedback = fbIn ? fbIn.value.trim() : "";
      const selected = Array.from(state.approvalSelectedHunks.get(id) || []);
      try {
        await apiJSON(`/api/approvals/${encodeURIComponent(id)}/resolve`, {
          method: "POST",
          body: JSON.stringify({
            action,
            selected_hunk_ids: selected,
            remember: action === "approve_remember",
            feedback
          })
        });
        state.pendingApprovals = state.pendingApprovals.filter((a) => a.id !== id);
        renderAttentionQueue();
        renderApprovalBanners();
      } catch (err) {
        if (err.code === "approval_not_settled") {
          state.approvalSurfacedClientMs.set(id, Date.now());
          renderApprovalBanners();
        }
      }
      return;
    }

    const paletteOpt = target.closest("[data-palette-insert]");
    if (paletteOpt && composer) {
      const ins = paletteOpt.getAttribute("data-palette-insert") || "";
      const tokLen = Number.parseInt(paletteOpt.getAttribute("data-palette-toklen") || "0", 10);
      const pos = composer.selectionStart || composer.value.length;
      composer.value = composer.value.slice(0, pos - tokLen) + ins + composer.value.slice(pos);
      document.getElementById("composer-palette")?.classList.add("hidden");
      composer.focus();
      saveDraftLocally();
      return;
    }

    const topoCard = target.closest("[data-pick-topo]");
    if (topoCard) {
      state.selectedLaunchTopology = topoCard.getAttribute("data-pick-topo") || "hybrid-fleet";
      renderTopologyCardsGrid();
      return;
    }

    const svgNode = target.closest("[data-svg-node]");
    if (svgNode) {
      const nodeId = svgNode.getAttribute("data-svg-node");
      if (nodeId) {
        state.selectedNodeId = nodeId;
        renderFleetStudio();
        if (state.activeMissionId) await fetchSelectedNodeDiff(state.activeMissionId, nodeId);
      }
      return;
    }

    const fileRow = target.closest("[data-view-file]");
    if (fileRow && state.activeWorkspaceId) {
      const rel = fileRow.getAttribute("data-view-file");
      const data = await apiJSON(
        `/api/workspaces/${encodeURIComponent(state.activeWorkspaceId)}/file?path=${encodeURIComponent(rel)}`
      );
      state.viewedFileRel = data.rel_path;
      document.getElementById("file-viewer-card")?.classList.remove("hidden");
      document.getElementById("file-viewer-path").textContent = data.rel_path;
      document.getElementById("file-viewer-code").textContent = data.content || "";
      return;
    }

    const rewindBtn = target.closest("[data-rewind-seq]");
    if (rewindBtn && state.activeSessionId) {
      const seq = Number.parseInt(rewindBtn.getAttribute("data-rewind-seq") || "0", 10);
      const restore = Boolean(document.getElementById("rewind-restore-files-cb")?.checked);
      await apiJSON(`/api/sessions/${encodeURIComponent(state.activeSessionId)}/rewind`, {
        method: "POST",
        body: JSON.stringify({ to_seq: seq, restore_files: restore })
      });
      await loadSessionDetail(state.activeSessionId);
      return;
    }

    const forkSeqBtn = target.closest("[data-fork-seq]");
    if (forkSeqBtn && state.activeSessionId) {
      const seq = Number.parseInt(forkSeqBtn.getAttribute("data-fork-seq") || "0", 10);
      const forked = await apiJSON(`/api/sessions/${encodeURIComponent(state.activeSessionId)}/fork`, {
        method: "POST",
        body: JSON.stringify({ at_seq: seq })
      });
      await refreshActiveStatusQuietly();
      await selectSession(forked.id);
      return;
    }

    const openFleetBtn = target.closest("[data-open-fleet-mission]");
    if (openFleetBtn) {
      const msnId = openFleetBtn.getAttribute("data-open-fleet-mission");
      if (msnId) {
        state.activeMissionId = msnId;
        setCenterMode("fleet");
        const snap = activeMissionSnapshot();
        if (snap && snap.status === "running") {
          subscribeToMissionStream(snap.id);
        }
      }
      return;
    }
  });

  // Delegated Change Events (Approval Hunk Checkboxes + Zero-Typing Cascading Dropdowns)
  document.addEventListener("change", async (e) => {
    const target = e.target;
    if (!(target instanceof Element)) return;

    if (target instanceof HTMLInputElement) {
      const apprId = target.getAttribute("data-appr-hunk");
      const hunkId = target.getAttribute("data-hunk-id");
      if (apprId && hunkId) {
        const set = state.approvalSelectedHunks.get(apprId) || new Set();
        if (target.checked) set.add(hunkId);
        else set.delete(hunkId);
        state.approvalSelectedHunks.set(apprId, set);
        return;
      }
    }

    if (target instanceof HTMLSelectElement) {
      const provRole = target.getAttribute("data-agent-prov-select");
      if (provRole) {
        const newProv = target.value;
        const models = getModelsForProvider(newProv);
        const defaultModel = (models[0] && models[0].id) || "";
        await patchAgentRole(provRole, { provider: newProv, ...(defaultModel ? { model: defaultModel } : {}) });
        return;
      }

      const modRole = target.getAttribute("data-agent-model-select");
      if (modRole) {
        await patchAgentRole(modRole, { model: target.value });
        return;
      }

      const effRole = target.getAttribute("data-agent-effort-select");
      if (effRole) {
        await patchAgentRole(effRole, { effort: target.value });
        return;
      }

      const addFbRole = target.getAttribute("data-add-role-fallback");
      if (addFbRole && target.value) {
        const cur = state.config && state.config.agents && state.config.agents[addFbRole];
        if (cur) {
          const nextFb = Array.from(new Set([...(cur.fallbacks || []), target.value]));
          await patchAgentRole(addFbRole, { fallbacks: nextFb });
        }
        return;
      }

      const promptRole = target.getAttribute("data-role-prompt-template");
      if (promptRole && target.value) {
        const tpl = PROMPT_TEMPLATES.find((p) => p.id === target.value);
        if (tpl) await patchAgentRole(promptRole, { system_prompt: tpl.text });
        return;
      }

      if (target.id === "add-specialist-role-select" && target.value) {
        const sp = SPECIALIST_ROLE_PRESETS[target.value];
        if (sp) {
          await patchAgentRole(sp.id, {
            name: sp.name,
            description: sp.description,
            provider: sp.provider,
            model: sp.model,
            effort: sp.effort,
            read_only: sp.read_only,
            allowed_tools: sp.allowed_tools,
            system_prompt: sp.system_prompt
          });
        }
        return;
      }

      const provUrlId = target.getAttribute("data-set-prov-url");
      if (provUrlId && target.value) {
        state.config = await apiJSON("/api/config/providers", {
          method: "POST",
          body: JSON.stringify({ id: provUrlId, base_url: target.value })
        });
        renderConfigStudio();
        flashAutoSaveIndicator(`已更新 ${provUrlId} 接口地址 ✓`);
        return;
      }

      const provKeyId = target.getAttribute("data-set-prov-keyref");
      if (provKeyId && target.value) {
        state.config = await apiJSON("/api/config/providers", {
          method: "POST",
          body: JSON.stringify({ id: provKeyId, api_key: target.value })
        });
        renderConfigStudio();
        flashAutoSaveIndicator(`已切换 ${provKeyId} 凭据源为 ${target.value} ✓`);
        return;
      }

      const wfVerifyId = target.getAttribute("data-wf-set-verify");
      if (wfVerifyId) {
        const wf = state.config && state.config.workflows && state.config.workflows[wfVerifyId];
        if (wf) {
          const nextWf = Object.assign({}, wf, { verify_command: target.value });
          state.config = await apiJSON("/api/config/workflows", { method: "POST", body: JSON.stringify(nextWf) });
          renderConfigStudio();
          flashAutoSaveIndicator(`拓扑 ${wfVerifyId} 验收命令已更新 ✓`);
        }
        return;
      }

      const wfNodeRoleId = target.getAttribute("data-wf-node-role");
      if (wfNodeRoleId) {
        const idx = Number.parseInt(target.getAttribute("data-node-idx") || "0", 10);
        const wf = state.config && state.config.workflows && state.config.workflows[wfNodeRoleId];
        if (wf && Array.isArray(wf.nodes) && wf.nodes[idx]) {
          const nextNodes = wf.nodes.map((n, i) => (i === idx ? Object.assign({}, n, { role: target.value }) : n));
          const nextWf = Object.assign({}, wf, { nodes: nextNodes });
          state.config = await apiJSON("/api/config/workflows", { method: "POST", body: JSON.stringify(nextWf) });
          renderConfigStudio();
          flashAutoSaveIndicator(`节点 ${wf.nodes[idx].id} 绑定角色已切换为 ${target.value} ✓`);
        }
        return;
      }

      const wfRoleModelId = target.getAttribute("data-wf-role-model");
      if (wfRoleModelId) {
        const roleKey = target.getAttribute("data-role-key");
        const wf = state.config && state.config.workflows && state.config.workflows[wfRoleModelId];
        if (wf && roleKey) {
          const nextRoleModels = Object.assign({}, wf.role_models || {});
          if (target.value) nextRoleModels[roleKey] = target.value;
          else delete nextRoleModels[roleKey];
          const nextWf = Object.assign({}, wf, { role_models: nextRoleModels });
          state.config = await apiJSON("/api/config/workflows", { method: "POST", body: JSON.stringify(nextWf) });
          renderConfigStudio();
          flashAutoSaveIndicator(`拓扑 ${wfRoleModelId} 角色模型覆盖已更新 ✓`);
        }
        return;
      }

      const globalSelectKey = target.getAttribute("data-global-select");
      if (globalSelectKey && target.value) {
        const patch = { [globalSelectKey]: target.value };
        if (globalSelectKey === "default_provider") {
          const mList = getModelsForProvider(target.value);
          if (mList[0] && mList[0].id) patch.default_model = mList[0].id;
        }
        state.config = await apiJSON("/api/config/global", { method: "POST", body: JSON.stringify(patch) });
        renderConfigStudio();
        flashAutoSaveIndicator(`全局默认模型已更新 ✓`);
      }
    }
  });
}

// Boot Workbench when running inside browser DOM (unless imported under Node test harness)
if (typeof document !== "undefined" && !globalThis.__GORNESS_TEST_NO_AUTOBOOT__) {
  applyTheme(state.theme, false);
  applyI18nInPlace();
  setupResizableSplitters();
  bindWorkbenchEvents();
  loadBootstrap()
    .then(() => connectGlobalEventStream())
    .catch((err) => {
      const hp = document.getElementById("health-pill");
      if (hp) {
        hp.className = "status-pill err";
        hp.textContent = err.message;
      }
    });
}
