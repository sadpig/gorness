// Gorness Studio — Apple HIG Zero-DB Agent Workbench (Pure Vanilla ES Module)
// Single Settings Home (⌘,) · 2 Center Modes (Chat | Fleet) · Slide-Over Always-On Hub · Canonical REST/SSE

const I18N = {
  zh: {
    brandSub: "macOS 纯 Go · 零数据库工作台",
    modeChat: "简单对话",
    modeFleet: "舰队协同",
    alwaysOnBtn: "常驻中枢",
    settingsBtn: "系统设置",
    searchPlaceholder: "跨项目搜索会话、笔记与产物 (⌘K)...",
    needsAttention: "待审批事项",
    projectsHeader: "工作区与会话",
    addProjectBtn: "+ 项目",
    newSessionBtn: "+ 会话",
    wtDiffBtn: "差异对比 (Diff)",
    wtLandBtn: "合入 Land",
    roleLabel: "角色",
    modelSelectLabel: "模型",
    autoModelOpt: "跟随角色默认模型",
    permLabel: "权限",
    permAutoOpt: "自动安全门控 (Auto)",
    permAcceptEditsOpt: "自动接受编辑 (Accept Edits)",
    permAskOpt: "每次询问 (Ask)",
    permPlanOpt: "只读规划 (Plan)",
    permYoloOpt: "全放行 (Yolo)",
    stopBtn: "停止",
    actNewWorktreeSession: "🌿 新建隔离 Worktree 会话",
    actCompact: "🗜️ 锚定增量压缩 (/compact)",
    actExportHTML: "📄 下载 HTML 归档",
    actExportMD: "📝 下载 Markdown 记录",
    actPublishPreview: "🔬 生成沙箱预览产物",
    actMoveWorkspace: "📂 迁移至其他项目…",
    actArchive: "📦 归档 / 取消归档会话",
    actDelete: "🗑️ 删除会话",
    composerPlaceholder: "输入任务指令… 输入 @ 引用文件，$ 激活技能，/ 调用命令（Shift+Enter 换行，Enter 发送 / 运行中注入 Steer）",
    hintFile: "文件 / @git",
    hintSkill: "技能",
    hintCmd: "命令",
    sendBtn: "发送对话",
    switchToFleetBtn: "切换舰队协同",
    backToChatBtn: "💬 返回简单对话",
    fleetAdvancedSummary: "高级运行参数（可选）",
    steerBtn: "注入 Steer",
    stopMissionBtn: "终止编排",
    deleteMissionBtn: "删除记录",
    newMissionBtn: "+ 新建舰队编排",
    launchFleetTitle: "启动异构多智能体协同编排",
    collapseBtn: "收起 ✕",
    fleetWorkflowLabel: "选择工作流模板 (workflows/*.toml)",
    fleetTopologyLabel: "编排拓扑引擎覆盖 (Topology Override)",
    budgetLabel: "预算上限 (USD, 0=不限)",
    verifyCmdLabel: "Refinery 验收门控命令 (可自由编辑或点选快捷芯片)",
    noVerifyChip: "无命令",
    missionGoalLabel: "编排目标 (Mission Goal)",
    goalPresetArch: "🏗️ 架构梳理与加固",
    goalPresetSec: "🛡️ 安全与竞态审计",
    goalPresetTDD: "🧪 TDD 补测与盲审",
    fleetGoalPh: "输入具体编排目标，或点选上方快捷模板后补充细节…",
    useWorktreesLabel: "启用独立 Git Worktree 物理隔离与串行变基合入 (--ff-only)",
    startMissionSubmit: "立即启动",
    fleetEmptyGoal: "选择或启动任意异构模型多智能体拓扑",
    nodeInspectorTitle: "节点详情与 Git Worktree 差异",
    jumpSessionBtn: "打开会话 ↗",
    nodeSteerPh: "向选中节点中途注入步边界 Steer 指令 (Enter 发送)…",
    steerNodeBtn: "注入 Steer",
    clickNodeHint: "点击上方拓扑中的任意 Agent 节点查看其实时流、会话记录与隔离 Git Worktree Diff。",
    blackboardTitle: "Markdown 共享黑板 (tasks/*.md & notes/*.md)",
    tabFiles: "文件",
    tabSnapshots: "时光机",
    tabArtifacts: "产物预览",
    tabContext: "上下文与技能",
    filterFilesPh: "过滤项目文件…",
    expandAllFiles: "展开",
    collapseAllFiles: "折叠",
    citeFileBtn: "@ 引用",
    truncatedBadge: "已截断 256KB",
    safeOpenBtn: "安全打开 / Finder",
    restoreFilesToggle: "回退时按 SHA-256 快照还原工作区文件",
    renderSnippetBtn: "+ HTML 沙箱片段",
    openStandalone: "独立窗口 ↗",
    mintPreviewBtn: "生成隔离预览",
    settingsCenterTitle: "Gorness 设置中心",
    cfgGroupSystem: "系统与外观",
    cfgPageGeneral: "通用与外观",
    cfgGroupModels: "模型与智能体",
    cfgPageMatrix: "角色模型矩阵",
    cfgPageProviders: "供应商与钥匙串",
    cfgPageAgents: "智能体专家定义",
    cfgGroupOrch: "编排、安全与扩展",
    cfgPageWorkflows: "多智能体工作流",
    cfgPagePermissions: "权限、信任与钩子",
    cfgPageMCP: "MCP 扩展服务",
    cfgGroupVault: "记忆与经验",
    cfgPageMemory: "四层记忆与候选技能",
    autoSaveReady: "实时热生效 ✓",
    autoSaving: "保存中…",
    autoSaveErr: "校验未通过",
    refreshConfigBtn: "↻ 刷新快照",
    alwaysOnTitle: "Always-On 休眠唤醒中枢",
    runDreamBtn: "✨ 立即运行 Dreamer",
    registerWsTitle: "挂载本地 macOS 项目工作区",
    finderPickerTitle: "通过 macOS 原生文件夹选择器挂载",
    finderPickerSub: "直接弹出原生 Finder 对话框选择代码仓库",
    pickFolderBtn: "📁 选择本地文件夹…",
    discoveredReposLabel: "本机常用开发目录下发现的项目（点击一键挂载）：",
    manualPathToggle: "或手动输入/粘贴绝对路径",
    wsPathLabel: "本地目录绝对路径",
    wsNameLabel: "显示名称（可选）",
    cancelBtn: "取消",
    confirmRegisterBtn: "挂载项目",
    wtModalTitle: "隔离会话 Git Worktree 审查与合入",
    wtVerifyLabel: "合入前验收测试命令 (可选，Refinery 通过后合入主分支)",
    wtConfirmLandBtn: "变基验证并合入 (Land)",
    moveSessionTitle: "迁移会话至目标工作区",
    targetWsLabel: "选择目标项目工作区",
    confirmMoveBtn: "确认迁移",
    thinkingSummary: "思考推理轨迹 (展开查看)",
    approveAll: "全部批准",
    approveSelected: "批准勾选块",
    denyBtn: "拒绝",
    rememberRule: "记住该工作区授权规则",
    settleGuardWait: "防误触冷却 500ms…",
    forkHere: "从此步分叉",
    rewindHere: "回退至此步",
    emptyTimeline: "选择一个项目或直接发送指令。所有配置、会话、记忆与黑板均以纯文本存储于 ~/.gorness/。"
  },
  en: {
    brandSub: "macOS Pure Go · Zero-DB Agent Workbench",
    modeChat: "Simple Chat",
    modeFleet: "Fleet Collaboration",
    alwaysOnBtn: "Always-On Hub",
    settingsBtn: "System Settings",
    searchPlaceholder: "Search sessions, notes & artifacts across projects (⌘K)...",
    needsAttention: "Needs Attention",
    projectsHeader: "Workspaces & Sessions",
    addProjectBtn: "+ Project",
    newSessionBtn: "+ Session",
    wtDiffBtn: "Diff",
    wtLandBtn: "Land",
    roleLabel: "Role",
    modelSelectLabel: "Model",
    autoModelOpt: "Follow Agent Default",
    permLabel: "Gate",
    permAutoOpt: "Auto Safe Gate",
    permAcceptEditsOpt: "Accept Edits",
    permAskOpt: "Always Ask",
    permPlanOpt: "Plan Read-Only",
    permYoloOpt: "Yolo Bypass",
    stopBtn: "Stop",
    actNewWorktreeSession: "🌿 New Isolated Worktree Session",
    actCompact: "🗜️ Anchored Compact (/compact)",
    actExportHTML: "📄 Download HTML Archive",
    actExportMD: "📝 Download Markdown Transcript",
    actPublishPreview: "🔬 Mint Sandboxed HTML Preview",
    actMoveWorkspace: "📂 Move to Workspace…",
    actArchive: "📦 Toggle Archive Session",
    actDelete: "🗑️ Delete Session",
    composerPlaceholder: "Ask Gorness… Type @ for files, $ for skills, / for commands (Shift+Enter newline, Enter send/steer)",
    hintFile: "file / @git",
    hintSkill: "skill",
    hintCmd: "command",
    sendBtn: "Send Message",
    switchToFleetBtn: "Switch to Fleet",
    backToChatBtn: "💬 Back to Chat",
    fleetAdvancedSummary: "Advanced Parameters (Optional)",
    steerBtn: "Inject Steer",
    stopMissionBtn: "Stop Fleet",
    deleteMissionBtn: "Delete Record",
    newMissionBtn: "+ Launch Fleet",
    launchFleetTitle: "Launch Heterogeneous Multi-Agent Fleet",
    collapseBtn: "Collapse ✕",
    fleetWorkflowLabel: "Select Workflow Template (workflows/*.toml)",
    fleetTopologyLabel: "Topology Engine Override",
    budgetLabel: "Budget Cap (USD, 0=unlimited)",
    verifyCmdLabel: "Refinery Verify Gate Command (editable or click a chip)",
    noVerifyChip: "None",
    missionGoalLabel: "Mission Goal",
    goalPresetArch: "🏗️ Architecture Hardening",
    goalPresetSec: "🛡️ Concurrency & Security Audit",
    goalPresetTDD: "🧪 TDD & Blind Review",
    fleetGoalPh: "Describe mission goal or pick a quick-fill chip above…",
    useWorktreesLabel: "Enable isolated Git worktrees & serialized Refinery merge (--ff-only)",
    startMissionSubmit: "Launch Now",
    fleetEmptyGoal: "Select or launch a heterogeneous multi-agent topology",
    nodeInspectorTitle: "Node Session, Stream & Git Worktree Diff",
    jumpSessionBtn: "Open Session ↗",
    nodeSteerPh: "Inject mid-turn step-boundary steer into selected node (Enter)…",
    steerNodeBtn: "Steer",
    clickNodeHint: "Click any agent node in the topology graph to inspect its live stream, session log & Git diff.",
    blackboardTitle: "Markdown Shared Blackboard (tasks/*.md & notes/*.md)",
    tabFiles: "Files",
    tabSnapshots: "Snapshots",
    tabArtifacts: "Artifacts",
    tabContext: "Context & Skills",
    filterFilesPh: "Filter workspace files…",
    expandAllFiles: "Expand",
    collapseAllFiles: "Collapse",
    citeFileBtn: "@ Cite",
    truncatedBadge: "Truncated 256KB",
    safeOpenBtn: "Safe Open / Reveal",
    restoreFilesToggle: "Restore workspace files from SHA-256 blobs on rewind",
    renderSnippetBtn: "+ HTML Snippet",
    openStandalone: "Standalone ↗",
    mintPreviewBtn: "Mint Isolated Preview",
    settingsCenterTitle: "Gorness Settings",
    cfgGroupSystem: "System & Appearance",
    cfgPageGeneral: "General & Appearance",
    cfgGroupModels: "Models & Agents",
    cfgPageMatrix: "Role-Model Matrix",
    cfgPageProviders: "Providers & Keychain",
    cfgPageAgents: "Agent Definitions",
    cfgGroupOrch: "Orchestration & Security",
    cfgPageWorkflows: "Multi-Agent Workflows",
    cfgPagePermissions: "Permissions, Trust & Hooks",
    cfgPageMCP: "MCP Extensions",
    cfgGroupVault: "Memory & Skills",
    cfgPageMemory: "4-Layer Memory & Skills",
    autoSaveReady: "Live Reloaded ✓",
    autoSaving: "Saving…",
    autoSaveErr: "Validation Error",
    refreshConfigBtn: "↻ Refresh",
    alwaysOnTitle: "Always-On Sleep-Wake Hub",
    runDreamBtn: "✨ Run Dreamer Now",
    registerWsTitle: "Mount Local macOS Workspace",
    finderPickerTitle: "Mount via Native macOS Folder Picker",
    finderPickerSub: "Open native Finder NSOpenPanel to select a repository",
    pickFolderBtn: "📁 Choose Folder…",
    discoveredReposLabel: "Discovered repositories in common local directories (1-click mount):",
    manualPathToggle: "Or enter/paste an absolute path manually",
    wsPathLabel: "Absolute Directory Path",
    wsNameLabel: "Display Name (optional)",
    cancelBtn: "Cancel",
    confirmRegisterBtn: "Mount Project",
    wtModalTitle: "Session Isolated Git Worktree Diff & Land",
    wtVerifyLabel: "Optional pre-land verification command (Refinery runs before --ff-only merge)",
    wtConfirmLandBtn: "Rebase, Verify & Land",
    moveSessionTitle: "Move Session Across Workspaces",
    targetWsLabel: "Target Workspace",
    confirmMoveBtn: "Move Session",
    thinkingSummary: "Thinking Trace (expand)",
    approveAll: "Approve All",
    approveSelected: "Approve Selected",
    denyBtn: "Deny",
    rememberRule: "Remember rule for this workspace",
    settleGuardWait: "500ms Settle Guard…",
    forkHere: "Fork Here",
    rewindHere: "Rewind Here",
    emptyTimeline: "Select a project or send a prompt. All config, sessions, memories & blackboards live as plain text in ~/.gorness/."
  }
};

// ============================================================================
// Compact Quick-Fill Presets (Populate Editable Controls — Never Lock Users In)
// ============================================================================

const PROVIDER_PRESETS = {
  anthropic: {
    id: "anthropic",
    name: "Anthropic",
    protocol: "anthropic",
    base_url: "https://api.anthropic.com",
    api_key: "keychain:ANTHROPIC_API_KEY",
    keychain_key: "ANTHROPIC_API_KEY",
    timeout_sec: 180,
    models: {
      "claude-opus-5-5": { name: "Claude Opus 5.5", context_window: 200000, max_output_tokens: 32000, supports_thinking: true, input_cost_per_1m: 15, output_cost_per_1m: 75 },
      "claude-sonnet-5-5": { name: "Claude Sonnet 5.5", context_window: 200000, max_output_tokens: 16384, supports_thinking: true, input_cost_per_1m: 3, output_cost_per_1m: 15 }
    }
  },
  openai: {
    id: "openai",
    name: "OpenAI",
    protocol: "openai-compat",
    base_url: "https://api.openai.com/v1",
    api_key: "keychain:OPENAI_API_KEY",
    keychain_key: "OPENAI_API_KEY",
    timeout_sec: 180,
    models: {
      "gpt-5.4": { name: "GPT-5.4", context_window: 272000, max_output_tokens: 32768, supports_thinking: true, input_cost_per_1m: 2.5, output_cost_per_1m: 10 },
      "gpt-5.4-mini": { name: "GPT-5.4 Mini", context_window: 272000, max_output_tokens: 16384, supports_thinking: true, input_cost_per_1m: 0.4, output_cost_per_1m: 1.6 }
    }
  },
  gemini: {
    id: "gemini",
    name: "Google Gemini",
    protocol: "gemini",
    base_url: "https://generativelanguage.googleapis.com/v1beta",
    api_key: "keychain:GEMINI_API_KEY",
    keychain_key: "GEMINI_API_KEY",
    timeout_sec: 180,
    models: {
      "gemini-3.1-pro": { name: "Gemini 3.1 Pro", context_window: 1000000, max_output_tokens: 32768, supports_thinking: true, input_cost_per_1m: 1.25, output_cost_per_1m: 5 },
      "gemini-3.1-flash": { name: "Gemini 3.1 Flash", context_window: 1000000, max_output_tokens: 16384, supports_thinking: true, input_cost_per_1m: 0.15, output_cost_per_1m: 0.6 }
    }
  },
  deepseek: {
    id: "deepseek",
    name: "DeepSeek",
    protocol: "openai-compat",
    base_url: "https://api.deepseek.com/v1",
    api_key: "keychain:DEEPSEEK_API_KEY",
    keychain_key: "DEEPSEEK_API_KEY",
    timeout_sec: 180,
    models: {
      "deepseek-reasoner": { name: "DeepSeek R1 Reasoner", context_window: 128000, max_output_tokens: 16384, supports_thinking: true, input_cost_per_1m: 0.55, output_cost_per_1m: 2.19 },
      "deepseek-chat": { name: "DeepSeek V3 Chat", context_window: 128000, max_output_tokens: 8192, supports_thinking: false, input_cost_per_1m: 0.27, output_cost_per_1m: 1.10 }
    }
  },
  ollama: {
    id: "ollama",
    name: "Ollama (Local)",
    protocol: "openai-compat",
    base_url: "http://127.0.0.1:11434/v1",
    api_key: "ollama-local",
    keychain_key: "",
    timeout_sec: 300,
    models: {
      "qwen3-coder:30b": { name: "Qwen3 Coder 30B", context_window: 131072, max_output_tokens: 16384, supports_thinking: true },
      "deepseek-r1:32b": { name: "DeepSeek R1 32B", context_window: 131072, max_output_tokens: 16384, supports_thinking: true }
    }
  },
  openrouter: {
    id: "openrouter",
    name: "OpenRouter",
    protocol: "openai-compat",
    base_url: "https://openrouter.ai/api/v1",
    api_key: "keychain:OPENROUTER_API_KEY",
    keychain_key: "OPENROUTER_API_KEY",
    timeout_sec: 180,
    models: {
      "anthropic/claude-sonnet-4": { name: "OR · Claude Sonnet", context_window: 200000, max_output_tokens: 16384, supports_thinking: true },
      "deepseek/deepseek-r1": { name: "OR · DeepSeek R1", context_window: 128000, max_output_tokens: 16384, supports_thinking: true }
    }
  }
};

const MATRIX_SCHEMES = [
  {
    id: "flagship-heterogeneous",
    badge: "异构盲审推荐",
    zh: "旗舰多厂商异构盲审",
    en: "Flagship Heterogeneous Blind-Review",
    descZh: "Claude Opus 架构规划 · GPT-5.4 编码实现 · Gemini Flash 极速探索 · 异构模型独立验收",
    descEn: "Opus Lead · GPT-5.4 Coder · Gemini Flash Explorer · Cross-vendor Verifier",
    default_provider: "anthropic",
    default_model: "claude-sonnet-5-5",
    default_effort: "high",
    roles: {
      lead: { provider: "anthropic", model: "claude-opus-5-5", effort: "high" },
      coder: { provider: "openai", model: "gpt-5.4", effort: "high" },
      explorer: { provider: "gemini", model: "gemini-3.1-flash", effort: "low" },
      verifier: { provider: "anthropic", model: "claude-opus-5-5", effort: "max" },
      utility: { provider: "gemini", model: "gemini-3.1-flash", effort: "low" }
    }
  },
  {
    id: "anthropic-pure",
    badge: "Anthropic",
    zh: "Anthropic 全栈组合",
    en: "Anthropic Full-Stack",
    descZh: "仅需一把 Anthropic Key：Opus 负责架构与审查，Sonnet 负责编码、探索与维护",
    descEn: "Single Anthropic key: Opus for Lead/Verifier, Sonnet for Coder/Explorer/Utility",
    default_provider: "anthropic",
    default_model: "claude-sonnet-5-5",
    default_effort: "high",
    roles: {
      lead: { provider: "anthropic", model: "claude-opus-5-5", effort: "high" },
      coder: { provider: "anthropic", model: "claude-sonnet-5-5", effort: "high" },
      explorer: { provider: "anthropic", model: "claude-sonnet-5-5", effort: "low" },
      verifier: { provider: "anthropic", model: "claude-opus-5-5", effort: "max" },
      utility: { provider: "anthropic", model: "claude-sonnet-5-5", effort: "low" }
    }
  },
  {
    id: "deepseek-value",
    badge: "高性价比",
    zh: "DeepSeek R1 + V3 方案",
    en: "DeepSeek R1 + V3 Value",
    descZh: "R1 负责深度架构与对抗验证，V3 负责高并发编码与上下文压缩",
    descEn: "DeepSeek Reasoner for Lead/Verifier, DeepSeek V3 for Coder/Explorer/Utility",
    default_provider: "deepseek",
    default_model: "deepseek-chat",
    default_effort: "high",
    roles: {
      lead: { provider: "deepseek", model: "deepseek-reasoner", effort: "high" },
      coder: { provider: "deepseek", model: "deepseek-chat", effort: "high" },
      explorer: { provider: "deepseek", model: "deepseek-chat", effort: "low" },
      verifier: { provider: "deepseek", model: "deepseek-reasoner", effort: "max" },
      utility: { provider: "deepseek", model: "deepseek-chat", effort: "low" }
    }
  },
  {
    id: "local-offline",
    badge: "离线零成本",
    zh: "本机 Ollama 私有矩阵",
    en: "Local Ollama Air-Gapped",
    descZh: "数据不出本机，基于 Apple Silicon 统一内存运行本地开源模型",
    descEn: "100% local inference on Apple Silicon unified memory via Ollama",
    default_provider: "ollama",
    default_model: "qwen3-coder:30b",
    default_effort: "medium",
    roles: {
      lead: { provider: "ollama", model: "deepseek-r1:32b", effort: "high" },
      coder: { provider: "ollama", model: "qwen3-coder:30b", effort: "high" },
      explorer: { provider: "ollama", model: "qwen3-coder:30b", effort: "low" },
      verifier: { provider: "ollama", model: "deepseek-r1:32b", effort: "high" },
      utility: { provider: "ollama", model: "qwen3-coder:30b", effort: "low" }
    }
  }
];

const ROLE_META = {
  lead: { icon: "🧭", zh: "Lead 架构主控", en: "Lead Architect" },
  coder: { icon: "🛠️", zh: "Coder 主力研发", en: "Implementation Coder" },
  explorer: { icon: "🔭", zh: "Explorer 极速探索", en: "Fast Explorer" },
  verifier: { icon: "⚖️", zh: "Verifier 异构盲审", en: "Adversarial Verifier" },
  utility: { icon: "🧹", zh: "Utility 压缩与命名", en: "Housekeeping Utility" }
};

const SPECIALIST_ROLE_PRESETS = [
  {
    id: "security-auditor",
    name: "安全审计专家 (Security Auditor)",
    description: "只读对抗性安全与供应链代码审查专家",
    effort: "max",
    read_only: true,
    allowed_tools: ["read_file", "list_files", "grep_search"],
    system_prompt: "You are a Principal Security Engineer. Audit diffs and code paths for command injection, SSRF, path traversal, symlink TOCTOU, auth bypass, and secret leaks. Cite exact file:line evidence."
  },
  {
    id: "test-architect",
    name: "测试架构专家 (Test Architect)",
    description: "TDD 边界条件与高并发竞态测试专家",
    effort: "high",
    read_only: false,
    allowed_tools: ["read_file", "write_file", "edit_file", "list_files", "grep_search", "run_command"],
    system_prompt: "You are a Test Architect. Design deterministic integration and concurrency tests covering failure modes and invariants, then verify them via run_command."
  },
  {
    id: "perf-profiler",
    name: "性能剖析专家 (Performance Profiler)",
    description: "低延迟、内存分配与锁竞争优化专家",
    effort: "high",
    read_only: true,
    allowed_tools: ["read_file", "list_files", "grep_search", "run_command"],
    system_prompt: "You are a Systems Performance Specialist. Identify allocations, O(N^2) loops, lock contention, and I/O bottlenecks with concrete benchmark evidence."
  }
];

const SIX_TOPOLOGIES = [
  { id: "hybrid-fleet", zh: "Hybrid Fleet 旗舰全流程", en: "Hybrid Fleet", descZh: "探索 → 规划 → 多 Worktree 并发编码 → 异构盲审 → Refinery 串行合入", descEn: "Explore → Plan → Worktree Coders → Blind Review → Refinery Merge" },
  { id: "review-loop", zh: "Generator-Verifier 对抗环", en: "Review Loop", descZh: "编码模型与独立厂商审查模型循环迭代直至 ACCEPT", descEn: "Coder & cross-vendor Verifier iterate until ACCEPT" },
  { id: "parallel-explore", zh: "Parallel Explore 并发侦察", en: "Parallel Explore", descZh: "多路只读智能体并发扫描代码仓后由主控综合决策", descEn: "Read-only scouts scan in parallel before Lead synthesis" },
  { id: "worktree-team", zh: "Worktree Team 隔离研发组", en: "Worktree Team", descZh: "每个 Worker 独占物理 Git 分支与目录，零写冲突并发", descEn: "Isolated Git worktree per coder + serialized rebase merge" },
  { id: "event-pipeline", zh: "Event Pipeline 确定性流水线", en: "Event Pipeline", descZh: "按有向无环图阶段顺序推进，每站输出写入共享黑板", descEn: "Deterministic staged DAG with blackboard handoffs" },
  { id: "shared-board", zh: "Shared Board 黑板协作", en: "Shared Board", descZh: "多专家围绕 Markdown 共享任务板自主认领并汇总", descEn: "Specialists collaborate around Markdown tasks/*.md & notes/*.md" }
];

const MCP_SERVER_PRESETS = [
  { id: "filesystem", name: "Filesystem MCP", transport: "stdio", command: "npx", args: ["-y", "@modelcontextprotocol/server-filesystem", "."], timeout_sec: 60 },
  { id: "git", name: "Git Repository MCP", transport: "stdio", command: "uvx", args: ["mcp-server-git"], timeout_sec: 60 },
  { id: "fetch", name: "Web Fetch MCP", transport: "stdio", command: "uvx", args: ["mcp-server-fetch"], timeout_sec: 60 },
  { id: "memory", name: "Knowledge Graph MCP", transport: "stdio", command: "npx", args: ["-y", "@modelcontextprotocol/server-memory"], timeout_sec: 60 },
  { id: "playwright", name: "Playwright Browser MCP", transport: "stdio", command: "npx", args: ["-y", "@playwright/mcp@latest"], timeout_sec: 120 }
];

const ALL_BUILTIN_TOOLS = [
  "read_file", "write_file", "edit_file", "list_files", "grep_search",
  "run_command", "web_fetch", "spawn_subagent", "memory_write", "schedule_timer", "external_open"
];

const SAFE_CMD_PRESETS = ["git status*", "git diff*", "git log*", "go test*", "go vet*", "npm test*", "pytest*", "cargo check*"];
const ASK_CMD_PRESETS = ["git push*", "git commit*", "git reset*", "rm *", "docker *", "kubectl *"];
const DENY_CMD_PRESETS = ["rm -rf /", "sudo *", "curl * | sh*", "mkfs*"];
const SAFE_DOMAIN_PRESETS = ["api.anthropic.com", "api.openai.com", "generativelanguage.googleapis.com", "api.deepseek.com", "openrouter.ai", "github.com", "pkg.go.dev"];

// ============================================================================
// Reactive Client State
// ============================================================================

const state = {
  lang: localStorage.getItem("gorness.lang") === "en" ? "en" : "zh",
  theme: localStorage.getItem("gorness.theme") || "auto",
  centerMode: "chat",
  inspectorTab: "files",
  settingsOpen: false,
  configPage: "matrix",
  matrixSubTab: "core",
  activeRuleCategory: "allow_commands",
  alwaysOnOpen: false,

  vaultRoot: "~/.gorness",
  version: "dev",
  health: { connected: true, pid: 0, activeRuns: 0 },
  config: null,
  configFiles: [],
  selectedVaultFile: "config.toml",
  vaultValidationError: "",

  editingProviderId: "",
  editingProviderDraft: null,
  fetchedProviderModels: [],
  fetchedModelsSource: "",
  fetchedModelsWarning: "",
  fetchedModelsFilter: "",
  selectedFetchedModels: new Set(),
  editingAgentId: "coder",
  editingAgentDraft: null,
  editingWorkflowId: "",
  editingWorkflowDraft: null,
  editingMCPId: "",
  editingMCPDraft: null,
  editingNoteSlug: "",
  editingProgressSlug: "",
  editingTimerId: "",
  editingTimerDraft: null,

  memoryData: { instructions: "", index: "", notes: {} },
  workspaceProgress: {},
  candidateSkills: [],
  workspaceGrants: [],

  workspaces: [],
  wsSuggestions: [],
  activeWorkspaceId: "",
  workspaceGit: null,
  workspaceFiles: [],
  workspaceTreeEntries: [],
  expandedTreeDirs: new Set(),
  selectedFilePath: "",
  workspaceSkills: [],
  workspaceArtifacts: [],
  previews: [],
  mdArtifactCache: {},
  selectedArtifactItem: null,

  sessions: [],
  activeSessionId: "",
  activeSessionMeta: null,
  activeEntries: [],
  activeSnapshots: [],
  sessionLoaded: false,
  draftSaveTimer: null,

  activeRunsBySession: {},
  inlineStreamSessionId: "",
  attachedRunSSE: null,
  attachedRunId: "",
  streamingText: "",
  streamingThinking: "",
  streamingTools: [],
  lastRunError: "",

  pendingApprovals: [],
  approvalTimers: new Map(),
  alwaysOn: { inbox: [], timers: [], deferred: [], outbox: [] },

  missions: [],
  activeMissionId: "",
  selectedNodeId: "",
  selectedNodeSessionEntries: [],
  nodeStreamBuffers: {},
  missionSSE: null,
  missionSSEId: "",

  paletteItems: [],
  paletteActiveIndex: 0
};

// ============================================================================
// Escaping, Safe Markdown, Toast & Canonical Fetch Helpers
// ============================================================================

const $ = (id) => document.getElementById(id);
const $$ = (sel) => Array.from(document.querySelectorAll(sel));

const BIDI_RE = /[\u202A-\u202E\u2066-\u2069\u200E\u200F\u061C]/g;

function escapeBidi(str) {
  return String(str ?? "").replace(BIDI_RE, (ch) => {
    const hex = ch.codePointAt(0).toString(16).toUpperCase().padStart(4, "0");
    return `<U+${hex}>`;
  });
}

// Pure HTML-entity escaping for strings ALREADY sanitized by backend config.EscapeBidi()
function escHtml(raw) {
  return String(raw ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

// Client-side Bidi + HTML escape for untrusted raw inputs not pre-escaped by server
function esc(raw) {
  return escHtml(escapeBidi(raw));
}

function t(key) {
  const dict = I18N[state.lang] || I18N.zh;
  return dict[key] ?? I18N.en[key] ?? key;
}

function tr(zhText, enText) {
  return state.lang === "zh" ? zhText : enText;
}

function showToast(message, kind = "info") {
  const stack = $("toast-stack");
  if (!stack || typeof document.createElement !== "function" || typeof stack.appendChild !== "function") {
    return;
  }
  const el = document.createElement("div");
  el.className = `toast-item ${kind}`;
  el.innerHTML = `<span>${escHtml(message)}</span>`;
  stack.appendChild(el);
  setTimeout(() => {
    if (el.parentNode) el.parentNode.removeChild(el);
  }, 3800);
}

function notifyError(err, prefix = "") {
  const msg = err?.message || String(err || "Unknown error");
  showToast(prefix ? `${prefix}: ${msg}` : msg, "error");
}

function setAutoSaveIndicator(status, detail = "") {
  const pill = $("config-autosave-pill");
  if (!pill) return;
  if (status === "saving") {
    pill.textContent = t("autoSaving");
    pill.className = "status-pill running";
  } else if (status === "err") {
    pill.textContent = detail ? `${t("autoSaveErr")}: ${detail.slice(0, 38)}` : t("autoSaveErr");
    pill.className = "status-pill err";
  } else {
    pill.textContent = t("autoSaveReady");
    pill.className = "status-pill ok";
  }
}

async function apiFetch(path, opts = {}) {
  const headers = Object.assign({ Accept: "application/json" }, opts.headers || {});
  let body = opts.body;
  if (body !== undefined && typeof body !== "string") {
    headers["Content-Type"] = "application/json";
    body = JSON.stringify(body);
  }
  const resp = await fetch(path, Object.assign({}, opts, { headers, body }));
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
    const errMsg = (data && (data.error || data.message)) || `HTTP ${resp.status}`;
    const err = new Error(errMsg);
    err.status = resp.status;
    err.payload = data;
    throw err;
  }
  return data;
}

// Safe Zero-XSS Native Markdown Renderer for Inspector Artifacts & Candidate Skills
function renderInlineMarkdown(escapedLine) {
  return escapedLine
    .replace(/`([^`]+)`/g, "<code>$1</code>")
    .replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>")
    .replace(/\*([^*]+)\*/g, "<em>$1</em>");
}

function renderMarkdownHTML(mdRaw) {
  const lines = String(mdRaw ?? "").replace(/\r\n/g, "\n").split("\n");
  const out = [];
  let inCode = false;
  let codeBuf = [];
  let inList = false;

  const flushList = () => {
    if (inList) {
      out.push("</ul>");
      inList = false;
    }
  };

  for (const rawLine of lines) {
    if (rawLine.trimStart().startsWith("```")) {
      flushList();
      if (!inCode) {
        inCode = true;
        codeBuf = [];
      } else {
        out.push(`<pre class="mono"><code>${escHtml(codeBuf.join("\n"))}</code></pre>`);
        inCode = false;
        codeBuf = [];
      }
      continue;
    }
    if (inCode) {
      codeBuf.push(rawLine);
      continue;
    }
    const trimmed = rawLine.trim();
    if (!trimmed) {
      flushList();
      continue;
    }
    const hMatch = trimmed.match(/^(#{1,4})\s+(.*)$/);
    if (hMatch) {
      flushList();
      const lvl = hMatch[1].length;
      out.push(`<h${lvl}>${renderInlineMarkdown(escHtml(hMatch[2]))}</h${lvl}>`);
      continue;
    }
    if (/^[-*+]\s+/.test(trimmed)) {
      if (!inList) {
        out.push("<ul>");
        inList = true;
      }
      out.push(`<li>${renderInlineMarkdown(escHtml(trimmed.replace(/^[-*+]\s+/, "")))}</li>`);
      continue;
    }
    if (trimmed.startsWith(">")) {
      flushList();
      out.push(`<blockquote>${renderInlineMarkdown(escHtml(trimmed.replace(/^>\s?/, "")))}</blockquote>`);
      continue;
    }
    flushList();
    out.push(`<p>${renderInlineMarkdown(escHtml(rawLine))}</p>`);
  }
  flushList();
  if (inCode && codeBuf.length > 0) {
    out.push(`<pre class="mono"><code>${escHtml(codeBuf.join("\n"))}</code></pre>`);
  }
  return out.join("\n") || `<p class="tiny-muted">${tr("（空文档）", "(Empty document)")}</p>`;
}

// Backend already runs config.EscapeBidi() on diff strings — use escHtml() without re-escaping Bidi!
function renderDiffPreviewHTML(diffText) {
  return String(diffText || "")
    .split("\n")
    .map((line) => {
      const safe = escHtml(line);
      if (line.startsWith("+") && !line.startsWith("+++")) {
        return `<span class="diff-line-add">${safe}</span>`;
      }
      if (line.startsWith("-") && !line.startsWith("---")) {
        return `<span class="diff-line-del">${safe}</span>`;
      }
      return safe + "\n";
    })
    .join("");
}

// ============================================================================
// Theme, Language & Canonical Global Preference Sync
// ============================================================================

function applyTheme(theme, persistServer = false) {
  const valid = ["auto", "light", "dark"].includes(theme) ? theme : "auto";
  state.theme = valid;
  localStorage.setItem("gorness.theme", valid);
  document.documentElement.setAttribute("data-theme", valid);
  if (persistServer) {
    saveGlobalConfigPatch({ ui_theme: valid, theme: valid });
  }
}

function applyLanguage(lang, persistServer = false) {
  const valid = lang === "en" ? "en" : "zh";
  state.lang = valid;
  localStorage.setItem("gorness.lang", valid);
  document.documentElement.lang = valid === "zh" ? "zh-CN" : "en";

  $$("[data-i18n]").forEach((el) => {
    const key = el.getAttribute("data-i18n");
    if (key) el.textContent = t(key);
  });
  $$("[data-i18n-placeholder]").forEach((el) => {
    const key = el.getAttribute("data-i18n-placeholder");
    if (key) el.setAttribute("placeholder", t(key));
  });

  syncSettingsDialogPageTitle();
  updateRunActionState();
  renderTopologyCards();
  renderChatHistory();
  if (state.settingsOpen) {
    renderConfigStudio();
  }
  if (state.alwaysOnOpen) {
    renderAlwaysOn();
  }
  if (persistServer) {
    saveGlobalConfigPatch({ ui_lang: valid, language: valid });
  }
}

async function saveGlobalConfigPatch(patch) {
  setAutoSaveIndicator("saving");
  try {
    const snap = await apiFetch("/api/config/global", { method: "PUT", body: patch });
    if (snap && snap.global) {
      state.config = snap;
    }
    setAutoSaveIndicator("ok");
    populateAgentAndModelControls();
    return snap;
  } catch (err) {
    setAutoSaveIndicator("err", err.message);
    notifyError(err, tr("全局配置保存失败", "Failed to save global config"));
    return null;
  }
}

// ============================================================================
// Center Modes (Chat | Fleet), ONE Settings Modal & Slide-Over Always-On Drawer
// ============================================================================

function syncSettingsDialogPageTitle() {
  const titleKeyMap = {
    general: "cfgPageGeneral",
    matrix: "cfgPageMatrix",
    providers: "cfgPageProviders",
    agents: "cfgPageAgents",
    workflows: "cfgPageWorkflows",
    permissions: "cfgPagePermissions",
    mcp: "cfgPageMCP",
    memory: "cfgPageMemory"
  };
  const titleEl = $("settings-dialog-page-title");
  const key = titleKeyMap[state.configPage] || "cfgPageMatrix";
  if (titleEl) {
    titleEl.setAttribute("data-i18n", key);
    titleEl.textContent = t(key);
  }
  $$(".settings-nav-btn[data-cfg-page]").forEach((b) => {
    b.classList.toggle("active", b.dataset.cfgPage === state.configPage);
  });
}

function openSettingsModal(page) {
  if (page) state.configPage = page === "files" ? "matrix" : page;
  else if (state.configPage === "files") state.configPage = "matrix";
  state.settingsOpen = true;
  syncSettingsDialogPageTitle();
  $("config-mode-view")?.classList.remove("hidden");
  loadSettingsAuxData(state.configPage);
  renderConfigStudio();
}

function closeSettingsModal() {
  state.settingsOpen = false;
  $("config-mode-view")?.classList.add("hidden");
}

function openAlwaysOnDrawer() {
  state.alwaysOnOpen = true;
  $("alwayson-drawer")?.classList.remove("hidden");
  refreshAlwaysOn();
}

function closeAlwaysOnDrawer() {
  state.alwaysOnOpen = false;
  $("alwayson-drawer")?.classList.add("hidden");
}

// Preserves setCenterMode("config") compatibility for server_test.go while keeping 2 center modes!
function setCenterMode(mode) {
  if (mode === "config") {
    openSettingsModal("matrix");
    return;
  }
  const target = mode === "fleet" ? "fleet" : "chat";
  state.centerMode = target;
  $("mode-chat-btn")?.classList.toggle("active", target === "chat");
  $("mode-fleet-btn")?.classList.toggle("active", target === "fleet");
  $("chat-mode-view")?.classList.toggle("hidden", target !== "chat");
  $("fleet-mode-view")?.classList.toggle("hidden", target !== "fleet");
  if (target === "fleet") {
    syncFleetLauncherWorkflows();
    renderFleetStudio();
    ensureActiveMissionSSE();
  }
}

function switchInspectorTab(tab) {
  const valid = ["files", "snapshots", "artifacts", "context"].includes(tab) ? tab : "files";
  state.inspectorTab = valid;
  $$(".insp-tab").forEach((btn) => {
    btn.classList.toggle("active", btn.dataset.tab === valid);
  });
  ["files", "snapshots", "artifacts", "context"].forEach((name) => {
    $(`insp-panel-${name}`)?.classList.toggle("hidden", name !== valid);
  });
  if (valid === "context") {
    renderContextInspector();
  }
}

function initSplitters() {
  const leftSplit = $("left-splitter");
  const rightSplit = $("right-splitter");
  const root = document.documentElement;

  const attachDrag = (el, side) => {
    if (!el) return;
    el.addEventListener("mousedown", (ev) => {
      ev.preventDefault();
      const onMove = (moveEv) => {
        if (side === "left") {
          const w = Math.min(420, Math.max(210, moveEv.clientX));
          root.style.setProperty("--left-width", `${w}px`);
        } else {
          const w = Math.min(540, Math.max(280, window.innerWidth - moveEv.clientX));
          root.style.setProperty("--right-width", `${w}px`);
        }
      };
      const onUp = () => {
        window.removeEventListener("mousemove", onMove);
        window.removeEventListener("mouseup", onUp);
      };
      window.addEventListener("mousemove", onMove);
      window.addEventListener("mouseup", onUp);
    });
  };

  attachDrag(leftSplit, "left");
  attachDrag(rightSplit, "right");
}

// ============================================================================
// Bootstrap, Live Health Pill & Global SSE (/api/events/stream + /api/runs/:id/stream)
// ============================================================================

async function loadBootstrap() {
  const data = await apiFetch("/api/bootstrap");
  state.version = data.version || "dev";
  state.vaultRoot = data.vault_root || "~/.gorness";
  state.config = data.config || null;
  state.configFiles = data.config_files || [];
  state.workspaces = data.workspaces || [];
  state.sessions = data.sessions || [];
  state.pendingApprovals = data.pending_approvals || [];
  state.alwaysOn = data.alwayson || { inbox: [], timers: [], deferred: [], outbox: [] };
  state.missions = data.missions || [];
  state.previews = data.previews || [];

  // Sync persisted theme & language from config.toml if configured
  const g = state.config?.global || {};
  const srvTheme = g.ui_theme || g.theme;
  const srvLang = g.ui_lang || g.language;
  if (srvTheme && ["auto", "light", "dark"].includes(srvTheme)) {
    applyTheme(srvTheme, false);
  }
  if (srvLang && ["zh", "en"].includes(srvLang)) {
    applyLanguage(srvLang, false);
  }

  state.activeRunsBySession = {};
  (data.active_runs || []).forEach((r) => {
    state.activeRunsBySession[r.session_id] = r;
  });

  updateHealthPill(true, (data.active_runs || []).length);

  renderReloadBanner(data.diagnostic || "");
  populateAgentAndModelControls();

  if (!state.activeWorkspaceId && state.workspaces.length > 0) {
    state.activeWorkspaceId = state.workspaces[0].id;
  }

  renderWorkspaceTree();
  renderAttentionQueue();
  renderApprovalCards();
  renderAlwaysOn();

  if (state.activeWorkspaceId) {
    await refreshWorkspaceInspector(state.activeWorkspaceId);
    const wsSessions = state.sessions.filter((s) => s.workspace_id === state.activeWorkspaceId && !s.archived);
    if (!state.activeSessionId && wsSessions.length > 0) {
      await selectSession(wsSessions[0].id);
    } else if (state.activeSessionId) {
      await selectSession(state.activeSessionId);
    } else {
      renderChatHistory();
    }
  } else {
    renderChatHistory();
  }

  if (!state.activeMissionId && state.missions.length > 0) {
    state.activeMissionId = state.missions[0].id;
  }
  syncFleetLauncherWorkflows();
  renderFleetStudio();
  ensureActiveMissionSSE();

  if (state.settingsOpen) {
    renderConfigStudio();
  }
}

async function refreshConfigSnapshot() {
  try {
    const res = await apiFetch("/api/config");
    state.config = res.snapshot || res.config || state.config;
    state.configFiles = res.files || res.config_files || state.configFiles;
    renderReloadBanner(res.diagnostic?.message || res.diagnostic || "");
    populateAgentAndModelControls();
    syncFleetLauncherWorkflows();
    if (state.settingsOpen) {
      renderConfigStudio();
    }
  } catch (err) {
    notifyError(err, tr("刷新配置快照失败", "Failed to refresh config"));
  }
}

function updateHealthPill(connected, activeRuns = 0) {
  state.health.connected = connected;
  state.health.activeRuns = activeRuns;
  const pill = $("health-pill");
  if (!pill) return;
  if (!connected) {
    pill.className = "status-pill warn";
    pill.textContent = tr("127.0.0.1 · 重连中", "127.0.0.1 · reconnecting");
    return;
  }
  pill.className = activeRuns > 0 ? "status-pill running" : "status-pill ok";
  pill.textContent = activeRuns > 0 ? tr(`127.0.0.1 · ${activeRuns} 运行中`, `127.0.0.1 · ${activeRuns} live`) : "127.0.0.1";
}

function renderReloadBanner(diagText) {
  const banner = $("reload-diag-banner");
  if (!banner) return;
  const msg = String(diagText || "").trim();
  if (!msg) {
    banner.classList.add("hidden");
    banner.textContent = "";
    return;
  }
  banner.classList.remove("hidden");
  banner.textContent = msg;
}

function connectGlobalEvents() {
  if (typeof EventSource === "undefined") return;
  const es = new EventSource("/api/events/stream");

  es.onopen = () => {
    updateHealthPill(true, Object.keys(state.activeRunsBySession).length);
  };
  es.onerror = () => {
    updateHealthPill(false, Object.keys(state.activeRunsBySession).length);
  };

  es.addEventListener("connected", () => {
    updateHealthPill(true, Object.keys(state.activeRunsBySession).length);
  });
  es.addEventListener("heartbeat", () => {
    updateHealthPill(true, Object.keys(state.activeRunsBySession).length);
  });

  const reloadEvents = ["config_reloaded", "config_diagnostic"];
  reloadEvents.forEach((evName) => {
    es.addEventListener(evName, async (ev) => {
      try {
        const data = JSON.parse(ev.data || "{}");
        if (data.diagnostic !== undefined) {
          renderReloadBanner(data.diagnostic);
        }
      } catch {}
      await refreshConfigSnapshot();
    });
  });

  const runEvents = [
    "run_started", "run_completed", "run_failed", "run_cancelled",
    "approval_required", "approval_resolved", "title_updated", "session_updated", "compacted"
  ];
  runEvents.forEach((evName) => {
    es.addEventListener(evName, async (ev) => {
      let data = {};
      try {
        data = JSON.parse(ev.data || "{}");
      } catch {}

      if (evName === "approval_required" || evName === "approval_resolved") {
        await refreshApprovals();
        return;
      }
      if (evName === "run_started" && data.session_id) {
        state.activeRunsBySession[data.session_id] = {
          run_id: data.run_id,
          session_id: data.session_id,
          workspace_id: data.workspace_id,
          agent_role: data.agent_role
        };
        updateHealthPill(true, Object.keys(state.activeRunsBySession).length);
        updateRunActionState();
        renderWorkspaceTree();
        if (data.session_id === state.activeSessionId) {
          attachActiveRunStreamIfNeeded(data.session_id);
        }
      }
      if (evName === "run_completed" || evName === "run_failed" || evName === "run_cancelled") {
        if (data.session_id) {
          delete state.activeRunsBySession[data.session_id];
        }
        updateHealthPill(true, Object.keys(state.activeRunsBySession).length);
        updateRunActionState();
        await refreshSessionsAndActive();
      }
      if (evName === "title_updated" || evName === "session_updated" || evName === "compacted") {
        await refreshSessionsAndActive();
      }
    });
  });

  es.addEventListener("alwayson_updated", () => {
    refreshAlwaysOn();
  });

  es.addEventListener("fleet_updated", async () => {
    try {
      state.missions = await apiFetch("/api/fleet/missions");
      renderFleetStudio();
      ensureActiveMissionSSE();
    } catch {}
  });
}

function closeAttachedRunSSE() {
  if (state.attachedRunSSE) {
    state.attachedRunSSE.close();
    state.attachedRunSSE = null;
    state.attachedRunId = "";
  }
}

// Re-attach live SSE stream via GET /api/runs/{id}/stream when reloading into an active background/resumed run
function attachActiveRunStreamIfNeeded(sessionId) {
  if (typeof EventSource === "undefined" || !sessionId) return;
  if (state.inlineStreamSessionId === sessionId) return;
  const activeRun = state.activeRunsBySession[sessionId];
  if (!activeRun || !activeRun.run_id) {
    closeAttachedRunSSE();
    return;
  }
  if (state.attachedRunId === activeRun.run_id && state.attachedRunSSE && state.attachedRunSSE.readyState !== 2) {
    return;
  }
  closeAttachedRunSSE();
  const es = new EventSource(`/api/runs/${encodeURIComponent(activeRun.run_id)}/stream`);
  state.attachedRunSSE = es;
  state.attachedRunId = activeRun.run_id;

  const types = [
    "run_started", "step_reset", "thinking_delta", "text_delta", "tool_call", "tool_result",
    "doom_loop", "approval_required", "approval_resolved", "compacted", "title_updated",
    "run_completed", "run_failed", "run_cancelled"
  ];
  types.forEach((evType) => {
    es.addEventListener(evType, (ev) => {
      try {
        const parsed = JSON.parse(ev.data || "{}");
        handleRunStreamEvent(parsed);
        if (evType === "run_completed" || evType === "run_failed" || evType === "run_cancelled") {
          closeAttachedRunSSE();
          refreshSessionsAndActive();
        }
      } catch {}
    });
  });
  es.onerror = () => {
    closeAttachedRunSSE();
  };
}

// ============================================================================
// Workspaces, Sessions, Worktree Pill, Consolidated Menu & Time-Machine
// ============================================================================

async function refreshSessionsAndActive() {
  try {
    state.sessions = await apiFetch("/api/sessions?include_archived=true");
    renderWorkspaceTree();
    if (state.activeSessionId && state.inlineStreamSessionId !== state.activeSessionId) {
      const detail = await apiFetch(`/api/sessions/${encodeURIComponent(state.activeSessionId)}`);
      state.activeSessionMeta = detail.meta;
      state.activeEntries = detail.entries || [];
      state.activeSnapshots = detail.snapshots || [];
      syncSessionHeaderControls();
      renderChatHistory();
      renderSnapshotsInspector();
    }
  } catch {}
}

function renderWorkspaceTree() {
  const tree = $("workspace-tree");
  if (!tree) return;
  if (state.workspaces.length === 0) {
    tree.innerHTML = `<div class="insp-note tiny-muted" style="padding:12px;">${tr(
      "尚未挂载本地项目。点击上方「+ 项目」选择任意 macOS 代码目录。",
      "No workspace mounted yet. Click '+ Project' above to choose a local folder."
    )}</div>`;
    return;
  }

  tree.innerHTML = state.workspaces
    .map((ws) => {
      const isActiveWs = ws.id === state.activeWorkspaceId;
      const wsSessions = state.sessions
        .filter((s) => s.workspace_id === ws.id && !s.archived)
        .sort((a, b) => {
          if (Boolean(a.pinned) !== Boolean(b.pinned)) return a.pinned ? -1 : 1;
          return new Date(b.updated_at || 0) - new Date(a.updated_at || 0);
        });

      const itemsHTML = wsSessions
        .map((s) => {
          const isSelected = s.id === state.activeSessionId;
          const isRunning = Boolean(state.activeRunsBySession[s.id]);
          const hasWT = Boolean(s.worktree_path);
          return `
            <div class="session-item ${isSelected ? "selected" : ""}" data-session-id="${esc(s.id)}" data-ws-id="${esc(ws.id)}">
              <div class="session-item-main">
                <div class="session-item-title">
                  ${isRunning ? '<span class="pulse-dot live"></span> ' : ""}${esc(s.title || tr("未命名会话", "Untitled"))}
                </div>
                <div class="session-item-sub mono">
                  <span>${esc(s.agent_role || "coder")}</span>
                  ${hasWT ? '<span>⎇ wt</span>' : ""}
                  <span>$${Number(s.estimated_usd || 0).toFixed(3)}</span>
                </div>
              </div>
              <button class="session-pin-btn ${s.pinned ? "pinned" : ""}" type="button" data-pin-session="${esc(s.id)}" title="${tr("置顶会话", "Pin")}">★</button>
            </div>
          `;
        })
        .join("");

      return `
        <div class="ws-group">
          <div class="ws-group-header ${isActiveWs ? "active-ws" : ""}" data-select-ws="${esc(ws.id)}">
            <span>📁 ${esc(ws.name)}</span>
            <span class="meta-chip mono">${wsSessions.length}</span>
          </div>
          ${itemsHTML}
        </div>
      `;
    })
    .join("");
}

async function populateWorkspaceSuggestions() {
  const host = $("ws-suggestions-list");
  if (!host) return;
  try {
    state.wsSuggestions = await apiFetch("/api/workspaces/suggestions");
  } catch {
    state.wsSuggestions = [];
  }
  if (!state.wsSuggestions || state.wsSuggestions.length === 0) {
    host.innerHTML = `<span class="tiny-muted">${tr("未扫描到常用项目目录，可使用上方原生文件夹选择器。", "No repos discovered in common folders; use Choose Folder above.")}</span>`;
    return;
  }
  host.innerHTML = state.wsSuggestions
    .map((sug) => {
      const isReg = Boolean(sug.registered || sug.already_registered);
      return `
      <button class="ws-sug-card ${isReg ? "registered" : ""}" type="button" data-mount-sug-path="${esc(sug.path)}" data-mount-sug-name="${esc(sug.name)}">
        <div class="scheme-card-top">
          <strong>${sug.is_git ? "⎇" : "📁"} ${esc(sug.name)}</strong>
          <span class="status-pill ${isReg ? "ok" : ""}">${isReg ? tr("已挂载", "Mounted") : tr("一键挂载", "Mount")}</span>
        </div>
        <span class="mono tiny-muted" style="word-break:break-all;">${esc(sug.path)}</span>
      </button>
    `;
    })
    .join("");
}

async function pickNativeMacFolder() {
  const btn = $("pick-native-folder-btn");
  if (btn) btn.disabled = true;
  try {
    const res = await apiFetch("/api/workspaces/pick-folder", { method: "POST", body: {} });
    if (res && !res.cancelled && res.path) {
      const ws = await apiFetch("/api/workspaces", {
        method: "POST",
        body: { path: res.path, name: res.name || "" }
      });
      $("workspace-modal")?.classList.add("hidden");
      await loadBootstrap();
      await selectWorkspace(ws.id);
      showToast(`${tr("已挂载项目", "Mounted workspace")}: ${ws.name}`, "ok");
    }
  } catch (err) {
    notifyError(err, tr("选择文件夹失败", "Folder picker failed"));
  } finally {
    if (btn) btn.disabled = false;
  }
}

async function selectWorkspace(wsId) {
  if (!wsId) return;
  state.activeWorkspaceId = wsId;
  renderWorkspaceTree();
  await refreshWorkspaceInspector(wsId);
  const wsSessions = state.sessions.filter((s) => s.workspace_id === wsId && !s.archived);
  if (wsSessions.length > 0) {
    await selectSession(wsSessions[0].id);
  } else {
    state.activeSessionId = "";
    state.activeSessionMeta = null;
    state.activeEntries = [];
    state.activeSnapshots = [];
    state.sessionLoaded = false;
    state.lastRunError = "";
    syncSessionHeaderControls();
    renderChatHistory();
    renderSnapshotsInspector();
  }
  if (state.settingsOpen && (state.configPage === "permissions" || state.configPage === "memory")) {
    loadSettingsAuxData(state.configPage);
  }
}

function getActiveSessionContextWindow() {
  const meta = state.activeSessionMeta;
  const roleId = meta?.agent_role || $("agent-role-select")?.value || "coder";
  const modelRef = (meta?.model_override || $("model-override-input")?.value || state.config?.agents?.[roleId]?.model || state.config?.global?.default_model || "").trim();
  if (modelRef && modelRef.includes("/")) {
    const [provId, ...rest] = modelRef.split("/");
    const modelId = rest.join("/");
    const provModels = state.config?.providers?.[provId]?.models || PROVIDER_PRESETS[provId]?.models || {};
    const cw = Number(provModels?.[modelId]?.context_window || 0);
    if (cw > 0) return cw;
  }
  return 200000;
}

function updateSessionContextMeter() {
  const meterEl = $("session-context-meter");
  if (!meterEl) return;
  const meta = state.activeSessionMeta;
  const usedTokens = meta ? Number(meta.input_tokens || 0) + Number(meta.output_tokens || 0) : 0;
  const ctxWindow = getActiveSessionContextWindow();
  const pct = Math.min(100, Math.round((usedTokens / Math.max(1, ctxWindow)) * 100));
  const filled = Math.min(6, Math.max(usedTokens > 0 ? 1 : 0, Math.round((pct / 100) * 6)));
  const bar = "▓".repeat(filled) + "░".repeat(6 - filled);
  meterEl.textContent = `[${bar}] ${pct}%`;
  meterEl.className = `context-meter-pill mono${pct > 85 ? " danger" : pct > 75 ? " warn" : ""}`;
  meterEl.title = tr(
    `上下文容量：${usedTokens.toLocaleString()} / ${ctxWindow.toLocaleString()} tok (${pct}%) · 点击执行 /compact 压缩`,
    `Context: ${usedTokens.toLocaleString()} / ${ctxWindow.toLocaleString()} tok (${pct}%) · Click to /compact`
  );
}

function updateSessionGitPill() {
  const gitPill = $("session-git-pill");
  if (!gitPill) return;
  const meta = state.activeSessionMeta;
  if (meta?.worktree_path || !state.workspaceGit || !state.workspaceGit.branch) {
    gitPill.classList.add("hidden");
    return;
  }
  gitPill.classList.remove("hidden");
  gitPill.textContent = `⎇ ${state.workspaceGit.branch}${state.workspaceGit.dirty ? "*" : ""}`;
}

function syncSessionHeaderControls() {
  const meta = state.activeSessionMeta;
  const titleEl = $("session-title-input");
  const seqEl = $("session-seq-badge");
  const costEl = $("session-cost-badge");
  const wtPill = $("session-worktree-pill");
  const wtBranch = $("session-wt-branch");

  updateSessionContextMeter();
  updateSessionGitPill();

  if (!meta) {
    if (titleEl) titleEl.value = tr("新会话", "New Session");
    if (seqEl) seqEl.textContent = "seq #0";
    if (costEl) costEl.textContent = "0 tok · $0.0000";
    if ($("perm-mode-select")) {
      $("perm-mode-select").value = state.config?.permissions?.mode || "auto";
    }
    wtPill?.classList.add("hidden");
    return;
  }

  if (titleEl) titleEl.value = meta.title || tr("未命名会话", "Untitled");
  if (seqEl) seqEl.textContent = `seq #${meta.head_seq || 0}`;
  const tok = Number(meta.input_tokens || 0) + Number(meta.output_tokens || 0);
  if (costEl) costEl.textContent = `${tok} tok · $${Number(meta.estimated_usd || 0).toFixed(4)}`;

  if (meta.agent_role && $("agent-role-select")) {
    $("agent-role-select").value = meta.agent_role;
  }
  if ($("perm-mode-select")) {
    $("perm-mode-select").value = meta.permission_mode || state.config?.permissions?.mode || "auto";
  }
  if ($("model-override-input")) {
    const ov = meta.model_override || "";
    const sel = $("model-override-input");
    if (ov && !Array.from(sel.options || []).some((o) => o.value === ov)) {
      const opt = typeof document.createElement === "function" ? document.createElement("option") : null;
      if (opt && typeof sel.appendChild === "function") {
        opt.value = ov;
        opt.textContent = ov;
        sel.appendChild(opt);
      }
    }
    sel.value = ov;
  }

  if (meta.worktree_path) {
    wtPill?.classList.remove("hidden");
    const shortName = String(meta.worktree_path).split("/").pop() || "worktree";
    if (wtBranch) wtBranch.textContent = `⎇ ${shortName}`;
  } else {
    wtPill?.classList.add("hidden");
  }
}

async function selectSession(sessionId) {
  if (!sessionId) return;
  if (state.draftSaveTimer) {
    clearTimeout(state.draftSaveTimer);
    state.draftSaveTimer = null;
  }
  state.sessionLoaded = false;
  state.activeSessionId = sessionId;
  state.lastRunError = "";
  const detail = await apiFetch(`/api/sessions/${encodeURIComponent(sessionId)}`);
  state.activeSessionMeta = detail.meta;
  state.activeWorkspaceId = detail.meta.workspace_id;
  state.activeEntries = detail.entries || [];
  state.activeSnapshots = detail.snapshots || [];

  syncSessionHeaderControls();

  const comp = $("composer-input");
  if (comp && document.activeElement !== comp) {
    comp.value = detail.meta.draft || "";
  }
  state.sessionLoaded = true;

  renderWorkspaceTree();
  renderChatHistory();
  renderSnapshotsInspector();
  updateRunActionState();
  attachActiveRunStreamIfNeeded(sessionId);
}

async function createNewSession(isolateGit = false) {
  if (!state.activeWorkspaceId) {
    populateWorkspaceSuggestions();
    $("workspace-modal")?.classList.remove("hidden");
    return;
  }
  try {
    const role = $("agent-role-select")?.value || "coder";
    const permMode = $("perm-mode-select")?.value || "auto";
    const created = await apiFetch("/api/sessions", {
      method: "POST",
      body: {
        workspace_id: state.activeWorkspaceId,
        title: isolateGit ? tr("Worktree 隔离会话", "Worktree Isolated Session") : tr("新会话", "New Session"),
        agent_role: role,
        permission_mode: permMode,
        model_override: ($("model-override-input")?.value || "").trim(),
        isolate_git: Boolean(isolateGit)
      }
    });
    state.sessions.unshift(created);
    await selectSession(created.id);
    $("composer-input")?.focus?.();
    if (isolateGit) {
      showToast(tr("已创建独立 Git Worktree 隔离会话", "Created isolated Git worktree session"), "ok");
    }
  } catch (err) {
    notifyError(err, tr("创建会话失败", "Failed to create session"));
  }
}

async function openSessionWorktreeModal() {
  if (!state.activeSessionId) return;
  $("session-worktree-modal")?.classList.remove("hidden");
  const metaEl = $("wt-modal-meta");
  const dirtyEl = $("wt-modal-dirty-pill");
  const diffEl = $("wt-modal-diff");
  if (diffEl) diffEl.textContent = tr("正在读取 Worktree Diff…", "Loading worktree diff…");
  try {
    const res = await apiFetch(`/api/sessions/${encodeURIComponent(state.activeSessionId)}/diff`);
    if (metaEl) metaEl.textContent = `${res.branch || "gorness/session"} · ${res.worktree_path || ""}`;
    if (dirtyEl) {
      dirtyEl.textContent = res.dirty ? tr("有变更 (dirty)", "dirty") : tr("干净 (clean)", "clean");
      dirtyEl.className = `status-pill ${res.dirty ? "warn" : "ok"}`;
    }
    if (diffEl) {
      diffEl.innerHTML = res.diff ? renderDiffPreviewHTML(res.diff) : escHtml(tr("（当前 Worktree 无未提交或分叉差异）", "(Clean worktree — no diff against base)"));
    }
  } catch (err) {
    if (diffEl) diffEl.textContent = err.message;
    notifyError(err, tr("读取会话 Worktree 差异失败", "Failed to load session worktree diff"));
  }
}

async function landSessionWorktree() {
  if (!state.activeSessionId) return;
  const verifyCmd = ($("wt-land-verify-input")?.value || "").trim();
  const btn = $("confirm-wt-land-btn");
  if (btn) btn.disabled = true;
  try {
    const res = await apiFetch(`/api/sessions/${encodeURIComponent(state.activeSessionId)}/land`, {
      method: "POST",
      body: { verify_command: verifyCmd }
    });
    $("session-worktree-modal")?.classList.add("hidden");
    showToast(tr("Worktree 已成功变基并合入主分支", "Worktree rebased & landed cleanly") + (res?.merged_commit ? ` (${String(res.merged_commit).slice(0, 8)})` : ""), "ok");
    await refreshSessionsAndActive();
    if (state.activeWorkspaceId) await refreshWorkspaceInspector(state.activeWorkspaceId);
  } catch (err) {
    notifyError(err, tr("Worktree 合入失败", "Worktree land failed"));
  } finally {
    if (btn) btn.disabled = false;
  }
}

async function exportSessionDownload(format) {
  if (!state.activeSessionId) return;
  const ext = format === "html" ? "html" : "md";
  try {
    const resp = await fetch(`/api/sessions/${encodeURIComponent(state.activeSessionId)}/export?format=${ext}`);
    const rawText = await resp.text();
    if (!resp.ok) {
      throw new Error(`HTTP ${resp.status}: ${rawText}`);
    }
    let content = rawText;
    const ctype = resp.headers.get("Content-Type") || "";
    if (ctype.includes("application/json")) {
      try {
        const parsed = JSON.parse(rawText);
        if (typeof parsed.content === "string") content = parsed.content;
      } catch {}
    }
    if (typeof Blob !== "undefined" && typeof URL !== "undefined" && typeof document.createElement === "function") {
      const blob = new Blob([content], { type: ext === "html" ? "text/html;charset=utf-8" : "text/markdown;charset=utf-8" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${state.activeSessionId}.${ext}`;
      a.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    }
    showToast(`${tr("已导出", "Exported")} ${state.activeSessionId}.${ext}`, "ok");
  } catch (err) {
    notifyError(err, tr("导出失败", "Export failed"));
  }
}

async function publishSessionPreview(format = "html") {
  if (!state.activeSessionId) return;
  try {
    const res = await apiFetch(`/api/sessions/${encodeURIComponent(state.activeSessionId)}/export?format=${encodeURIComponent(format)}`, {
      method: "POST",
      body: { format }
    });
    if (format === "md" && res.artifact_path && res.content) {
      state.mdArtifactCache[res.artifact_path] = res.content;
    }
    if (state.activeWorkspaceId) {
      await refreshWorkspaceInspector(state.activeWorkspaceId);
    }
    switchInspectorTab("artifacts");
    if (res.preview_url) {
      selectArtifactByValue(res.preview_url);
    } else if (res.artifact_path) {
      selectArtifactByValue(`md:${res.artifact_path}`);
    }
    showToast(tr("已生成隔离预览产物", "Artifact persisted to Inspector"), "ok");
  } catch (err) {
    notifyError(err, tr("生成预览产物失败", "Failed to publish preview"));
  }
}

async function handleSessionMenuAction(action) {
  $("session-actions-menu")?.classList.add("hidden");
  if (action === "new-worktree") {
    await createNewSession(true);
    return;
  }
  if (!state.activeSessionId) {
    showToast(tr("请先选择或创建一个会话", "Select or create a session first"), "info");
    return;
  }
  switch (action) {
    case "compact":
      await compactActiveSession();
      break;
    case "export-html":
      await exportSessionDownload("html");
      break;
    case "export-md":
      await exportSessionDownload("md");
      break;
    case "publish-html":
      await publishSessionPreview("html");
      break;
    case "move": {
      const sel = $("move-target-ws-select");
      if (sel) {
        sel.innerHTML = state.workspaces
          .map((w) => `<option value="${esc(w.id)}" ${w.id === state.activeWorkspaceId ? "selected" : ""}>${esc(w.name)} (${esc(w.root_path)})</option>`)
          .join("");
      }
      $("move-session-modal")?.classList.remove("hidden");
      break;
    }
    case "archive": {
      try {
        const nextArch = !Boolean(state.activeSessionMeta?.archived);
        await apiFetch(`/api/sessions/${encodeURIComponent(state.activeSessionId)}`, {
          method: "PATCH",
          body: { archived: nextArch }
        });
        showToast(nextArch ? tr("会话已归档", "Session archived") : tr("已取消归档", "Session unarchived"), "ok");
        await loadBootstrap();
      } catch (err) {
        notifyError(err, tr("归档会话失败", "Failed to archive session"));
      }
      break;
    }
    case "delete": {
      try {
        const delId = state.activeSessionId;
        await apiFetch(`/api/sessions/${encodeURIComponent(delId)}`, { method: "DELETE" });
        state.activeSessionId = "";
        state.activeSessionMeta = null;
        showToast(tr("会话已删除", "Session deleted"), "ok");
        await loadBootstrap();
      } catch (err) {
        notifyError(err, tr("删除会话失败", "Failed to delete session"));
      }
      break;
    }
  }
}

async function compactActiveSession() {
  if (!state.activeSessionId) return;
  try {
    const res = await apiFetch(`/api/sessions/${encodeURIComponent(state.activeSessionId)}/compact`, {
      method: "POST",
      body: {}
    });
    state.activeSessionMeta = res.meta;
    state.activeEntries = res.entries || [];
    syncSessionHeaderControls();
    renderChatHistory();
    showToast(tr("已完成锚定增量压缩", "Session compacted cleanly"), "ok");
  } catch (err) {
    notifyError(err, tr("压缩会话失败", "Compaction failed"));
  }
}

async function forkAtSeq(atSeq) {
  if (!state.activeSessionId) return;
  try {
    const body = atSeq !== undefined ? { at_seq: Number(atSeq) } : {};
    const forked = await apiFetch(`/api/sessions/${encodeURIComponent(state.activeSessionId)}/fork`, {
      method: "POST",
      body
    });
    state.sessions.unshift(forked);
    await selectSession(forked.id);
    showToast(`${tr("已分叉新会话", "Forked session")}: ${forked.title}`, "ok");
  } catch (err) {
    notifyError(err, tr("会话分叉失败", "Fork failed"));
  }
}

async function rewindToSeq(toSeq, explicitRestoreFiles) {
  if (!state.activeSessionId) return;
  const restore = explicitRestoreFiles !== undefined
    ? Boolean(explicitRestoreFiles)
    : ($("rewind-restore-files-cb")?.checked ?? true);
  try {
    await apiFetch(`/api/sessions/${encodeURIComponent(state.activeSessionId)}/rewind`, {
      method: "POST",
      body: { to_seq: Number(toSeq), restore_files: restore }
    });
    await selectSession(state.activeSessionId);
    if (state.activeWorkspaceId) {
      await refreshWorkspaceInspector(state.activeWorkspaceId);
    }
    showToast(`${tr("已回退至", "Rewound to")} seq #${toSeq}${restore ? tr("（已还原快照文件）", " (files restored)") : ""}`, "ok");
  } catch (err) {
    notifyError(err, tr("时光机回退失败", "Rewind failed"));
  }
}

function renderSnapshotsInspector() {
  const host = $("snapshots-timeline-list");
  if (!host) return;
  if (!state.activeEntries || state.activeEntries.length === 0) {
    host.innerHTML = `<span class="tiny-muted">${tr("当前会话暂无快照点。", "No turn checkpoints yet.")}</span>`;
    return;
  }

  const snapsBySeq = new Map();
  (state.activeSnapshots || []).forEach((rec) => {
    const list = snapsBySeq.get(rec.turn_seq) || [];
    list.push(rec);
    snapsBySeq.set(rec.turn_seq, list);
  });

  const rows = [
    `<div class="snap-card">
      <div class="snap-card-head">
        <span class="mono">seq #0 (${tr("初始状态", "Initial Baseline")})</span>
        <button class="ghost-xs-btn" type="button" data-rewind-seq="0">${t("rewindHere")}</button>
      </div>
    </div>`
  ];

  state.activeEntries.forEach((ent) => {
    const touched = snapsBySeq.get(ent.seq) || [];
    const fileTags = touched
      .map((f) => `<span class="meta-chip mono">${escHtml(f.rel_path)}</span>`)
      .join("");
    rows.push(`
      <div class="snap-card">
        <div class="snap-card-head">
          <span class="mono">seq #${ent.seq} · ${esc(ent.role)}</span>
          <div class="row-actions">
            <button class="ghost-xs-btn" type="button" data-fork-seq="${ent.seq}">${t("forkHere")}</button>
            <button class="ghost-xs-btn" type="button" data-rewind-seq="${ent.seq}">${t("rewindHere")}</button>
          </div>
        </div>
        ${fileTags ? `<div class="snap-files-tags">${fileTags}</div>` : ""}
      </div>
    `);
  });

  host.innerHTML = rows.join("");
}

// ============================================================================
// Right Inspector: Files, Artifacts (Markdown + HTML Sandbox) & Context
// ============================================================================

function normalizeWorkspaceFileEntries(rawList) {
  const entries = [];
  (rawList || []).forEach((item) => {
    if (!item) return;
    if (typeof item === "string") {
      const clean = item.replace(/^\/+|\/+$/g, "");
      if (!clean) return;
      const parts = clean.split("/");
      entries.push({
        path: clean,
        name: parts[parts.length - 1],
        is_dir: false,
        size: 0
      });
      return;
    }
    const rel = String(item.path || item.rel_path || item.name || "").replace(/^\/+|\/+$/g, "");
    if (!rel) return;
    const parts = rel.split("/");
    entries.push({
      path: rel,
      name: item.name || parts[parts.length - 1],
      is_dir: Boolean(item.is_dir || item.isDir || item.type === "dir" || item.kind === "dir"),
      size: Number(item.size || 0)
    });
  });
  return entries;
}

function buildWorkspaceFileTree(rawEntries) {
  const entries = normalizeWorkspaceFileEntries(rawEntries);
  const rootMap = new Map();

  const ensureDirNode = (map, dirName, dirPath) => {
    let node = map.get(dirName);
    if (!node) {
      node = { name: dirName, path: dirPath, is_dir: true, size: 0, _childMap: new Map(), children: [] };
      map.set(dirName, node);
    } else if (!node.is_dir) {
      node.is_dir = true;
      if (!node._childMap) node._childMap = new Map();
    }
    return node;
  };

  entries.forEach((ent) => {
    const parts = ent.path.split("/").filter(Boolean);
    if (parts.length === 0) return;
    let curMap = rootMap;
    let curPath = "";
    for (let i = 0; i < parts.length; i++) {
      const seg = parts[i];
      curPath = curPath ? `${curPath}/${seg}` : seg;
      const isLast = i === parts.length - 1;
      if (!isLast || ent.is_dir) {
        const dirNode = ensureDirNode(curMap, seg, curPath);
        curMap = dirNode._childMap;
      } else {
        if (!curMap.has(seg)) {
          curMap.set(seg, {
            name: seg,
            path: curPath,
            is_dir: false,
            size: ent.size || 0,
            children: []
          });
        }
      }
    }
  });

  const finalizeNodes = (map) => {
    const list = Array.from(map.values());
    list.forEach((n) => {
      if (n.is_dir && n._childMap) {
        n.children = finalizeNodes(n._childMap);
        delete n._childMap;
      }
    });
    list.sort((a, b) => {
      if (Boolean(a.is_dir) !== Boolean(b.is_dir)) return a.is_dir ? -1 : 1;
      return String(a.name).localeCompare(String(b.name));
    });
    return list;
  };

  return finalizeNodes(rootMap);
}

function getFileIcon(name) {
  const lower = String(name || "").toLowerCase();
  if (lower.endsWith(".go")) return "🐹";
  if (lower.endsWith(".js") || lower.endsWith(".ts")) return "📜";
  if (lower.endsWith(".html") || lower.endsWith(".css")) return "🎨";
  if (lower.endsWith(".md")) return "📝";
  if (lower.endsWith(".toml") || lower.endsWith(".json") || lower.endsWith(".yaml") || lower.endsWith(".yml")) return "⚙️";
  if (lower.endsWith(".sh")) return "🐚";
  return "📄";
}

function formatFileSize(bytes) {
  const n = Number(bytes || 0);
  if (!n || n <= 0) return "";
  if (n < 1024) return `${n}B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)}KB`;
  return `${(n / (1024 * 1024)).toFixed(1)}MB`;
}

async function refreshWorkspaceInspector(wsId) {
  if (!wsId) return;
  try {
    const treePromise = apiFetch(`/api/workspaces/${encodeURIComponent(wsId)}/tree`).catch(() => null);
    const filesPromise = apiFetch(`/api/workspaces/${encodeURIComponent(wsId)}/files?limit=400`).catch(() => null);
    const gitPromise = apiFetch(`/api/workspaces/${encodeURIComponent(wsId)}/git`).catch(() => null);
    const [treeRes, filesRes, skillsRes, artRes, gitRes] = await Promise.all([
      treePromise,
      filesPromise,
      apiFetch(`/api/workspaces/${encodeURIComponent(wsId)}/skills`),
      apiFetch(`/api/workspaces/${encodeURIComponent(wsId)}/artifacts`),
      gitPromise
    ]);
    const rawEntries =
      treeRes?.entries ||
      treeRes?.tree ||
      treeRes?.files ||
      filesRes?.entries ||
      filesRes?.tree ||
      filesRes?.files ||
      [];
    const normEntries = normalizeWorkspaceFileEntries(rawEntries);
    state.workspaceTreeEntries = normEntries;
    state.workspaceFiles = normEntries.filter((e) => !e.is_dir).map((e) => e.path);
    if (!(state.expandedTreeDirs instanceof Set)) {
      state.expandedTreeDirs = new Set();
    }
    if (state.expandedTreeDirs.size === 0) {
      buildWorkspaceFileTree(normEntries)
        .slice(0, 6)
        .forEach((n) => {
          if (n.is_dir) state.expandedTreeDirs.add(n.path);
        });
    }
    state.workspaceGit = gitRes?.branch ? gitRes : null;
    state.workspaceSkills = skillsRes?.skills || [];
    state.workspaceArtifacts = artRes?.artifacts || [];
    state.previews = artRes?.previews || state.previews;
    updateSessionGitPill();
    renderFilesList();
    renderArtifactsInspector();
    if (state.inspectorTab === "context") {
      renderContextInspector();
    }
  } catch {}
}

function renderFilesList() {
  const host = $("workspace-files-list");
  if (!host) return;
  const q = ($("file-filter-input")?.value || "").trim().toLowerCase();
  const baseEntries =
    state.workspaceTreeEntries && state.workspaceTreeEntries.length > 0
      ? state.workspaceTreeEntries
      : normalizeWorkspaceFileEntries(state.workspaceFiles);

  const filteredEntries = q
    ? baseEntries.filter((e) => e.path.toLowerCase().includes(q))
    : baseEntries;

  if (filteredEntries.length === 0) {
    host.innerHTML = `<span class="tiny-muted" style="padding:8px;">${tr("无匹配文件", "No matching files")}</span>`;
    return;
  }

  const treeNodes = buildWorkspaceFileTree(filteredEntries);

  const renderNodeHTML = (node) => {
    if (node.is_dir) {
      const isExpanded = Boolean(q) || state.expandedTreeDirs.has(node.path);
      const childrenHTML = isExpanded ? (node.children || []).map(renderNodeHTML).join("") : "";
      return `
        <div class="tree-dir-group">
          <div class="tree-dir-row mono" data-toggle-dir="${escHtml(node.path)}" role="treeitem" aria-expanded="${isExpanded}">
            <div class="tree-row-left">
              <span class="tree-chevron">${isExpanded ? "▾" : "▸"}</span>
              <span class="tree-icon">${isExpanded ? "📂" : "📁"}</span>
              <span class="tree-name">${escHtml(node.name)}</span>
            </div>
            <span class="meta-chip mono">${(node.children || []).length}</span>
          </div>
          ${isExpanded ? `<div class="tree-children" role="group">${childrenHTML}</div>` : ""}
        </div>
      `;
    }
    const isSel = state.selectedFilePath === node.path;
    const sizeText = formatFileSize(node.size);
    return `
      <div class="file-row tree-file-row mono ${isSel ? "selected" : ""}" data-open-file="${escHtml(node.path)}" role="treeitem">
        <div class="tree-row-left">
          <span class="tree-icon">${getFileIcon(node.name)}</span>
          <span class="tree-name" title="${escHtml(node.path)}">${escHtml(node.name)}</span>
        </div>
        <div class="tree-meta-right">
          ${sizeText ? `<span class="tiny-muted mono" style="font-size:10px;">${escHtml(sizeText)}</span>` : ""}
          <button class="tree-cite-btn" type="button" data-cite-file="${escHtml(node.path)}" title="${tr("引用该文件到对话框", "Cite file in chat")}">@</button>
        </div>
      </div>
    `;
  };

  host.innerHTML = treeNodes.map(renderNodeHTML).join("");
}

function expandAllWorkspaceFolders() {
  const baseEntries =
    state.workspaceTreeEntries && state.workspaceTreeEntries.length > 0
      ? state.workspaceTreeEntries
      : normalizeWorkspaceFileEntries(state.workspaceFiles);
  const walk = (nodes) => {
    (nodes || []).forEach((n) => {
      if (n.is_dir) {
        state.expandedTreeDirs.add(n.path);
        walk(n.children);
      }
    });
  };
  walk(buildWorkspaceFileTree(baseEntries));
  renderFilesList();
}

function collapseAllWorkspaceFolders() {
  state.expandedTreeDirs.clear();
  renderFilesList();
}

function citeFileInComposer(relPath) {
  if (!relPath) return;
  const comp = $("composer-input");
  if (!comp) return;
  const token = `@${relPath} `;
  const cur = comp.value || "";
  if (!cur.includes(`@${relPath}`)) {
    comp.value = cur && !cur.endsWith(" ") ? `${cur} ${token}` : `${cur}${token}`;
  }
  comp.focus?.();
}

async function openFilePreview(relPath) {
  if (!state.activeWorkspaceId || !relPath) return;
  state.selectedFilePath = relPath;
  renderFilesList();
  try {
    const res = await apiFetch(
      `/api/workspaces/${encodeURIComponent(state.activeWorkspaceId)}/file?path=${encodeURIComponent(relPath)}`
    );
    const card = $("file-viewer-card");
    if (card) {
      card.classList.remove("hidden");
      card.dataset.currentPath = relPath;
    }
    if ($("file-viewer-path")) $("file-viewer-path").textContent = res.path || relPath;
    $("file-truncated-badge")?.classList.toggle("hidden", !res.truncated);
    // Backend already applies config.EscapeBidi() on res.content -> assign textContent directly (zero double-escaping!)
    if ($("file-viewer-code")) $("file-viewer-code").textContent = res.content || "";
  } catch (err) {
    notifyError(err, tr("读取文件失败", "Failed to read file"));
  }
}

async function safeOpenPath(pathStr) {
  if (!pathStr) return;
  try {
    const res = await apiFetch("/api/open-safe", {
      method: "POST",
      body: { workspace_id: state.activeWorkspaceId, path: pathStr }
    });
    showToast(`${tr("安全打开完成", "Safe open")}: ${res.action || "ok"}`, "ok");
  } catch (err) {
    notifyError(err, tr("安全打开被拦截", "Safe open blocked"));
  }
}

function renderArtifactsInspector() {
  const sel = $("preview-artifact-select");
  if (!sel) return;
  const items = [];
  const seen = new Set();

  (state.previews || []).forEach((p) => {
    if (p.url && !seen.has(p.url)) {
      seen.add(p.url);
      items.push({ value: p.url, kind: "html", label: `🔬 HTML · ${p.title || p.token}`, url: p.url, path: p.saved_path || "" });
    }
  });
  (state.workspaceArtifacts || []).forEach((a) => {
    if (a.kind === "html" && a.preview_url && !seen.has(a.preview_url)) {
      seen.add(a.preview_url);
      items.push({ value: a.preview_url, kind: "html", label: `📄 HTML · ${a.name}`, url: a.preview_url, path: a.path || "" });
    } else if (a.kind === "md") {
      const val = `md:${a.path || a.name}`;
      if (!seen.has(val)) {
        seen.add(val);
        if (typeof a.content === "string") {
          state.mdArtifactCache[a.path || a.name] = a.content;
        }
        items.push({ value: val, kind: "md", label: `📝 Markdown · ${a.name}`, name: a.name, path: a.path || "", content: a.content });
      }
    }
  });

  state._artifactSelectItems = items;

  if (items.length === 0) {
    sel.innerHTML = `<option value="">${tr("暂无预览产物 (支持 .html 沙箱与 .md 原生预览)", "No artifacts yet (.html sandbox & .md native preview)")}</option>`;
    $("inspector-md-preview")?.classList.add("hidden");
    $("inspector-preview-frame")?.classList.remove("hidden");
    $("safe-open-artifact-btn")?.classList.add("hidden");
    $("open-preview-standalone")?.classList.add("hidden");
    if ($("open-preview-standalone")) $("open-preview-standalone").href = "#";
    return;
  }

  sel.innerHTML = items.map((it) => `<option value="${esc(it.value)}">${esc(it.label)}</option>`).join("");
  selectArtifactByValue(items[0].value);
}

async function selectArtifactByValue(val) {
  const items = state._artifactSelectItems || [];
  const found = items.find((it) => it.value === val) || items[0];
  if (!found) return;
  state.selectedArtifactItem = found;
  const sel = $("preview-artifact-select");
  if (sel && sel.value !== found.value) sel.value = found.value;

  const mdBox = $("inspector-md-preview");
  const frame = $("inspector-preview-frame");
  const link = $("open-preview-standalone");
  const safeBtn = $("safe-open-artifact-btn");

  if (found.kind === "md") {
    frame?.classList.add("hidden");
    mdBox?.classList.remove("hidden");
    link?.classList.add("hidden");
    if (found.path) safeBtn?.classList.remove("hidden");

    let mdText = found.content ?? state.mdArtifactCache[found.path || found.name];
    if (mdText === undefined && found.name && found.name.endsWith(".md")) {
      const maybeSessId = found.name.replace(/\.md$/i, "");
      try {
        const expResp = await fetch(`/api/sessions/${encodeURIComponent(maybeSessId)}/export?format=md`);
        if (expResp.ok) {
          const raw = await expResp.text();
          try {
            const j = JSON.parse(raw);
            mdText = j.content ?? raw;
          } catch {
            mdText = raw;
          }
          state.mdArtifactCache[found.path || found.name] = mdText;
        }
      } catch {}
    }
    if (mdBox) {
      mdBox.innerHTML = renderMarkdownHTML(mdText || tr("（Markdown 产物已就绪）", "(Markdown artifact ready)"));
    }
  } else {
    mdBox?.classList.add("hidden");
    frame?.classList.remove("hidden");
    link?.classList.remove("hidden");
    safeBtn?.classList.toggle("hidden", !found.path);
    if (frame) frame.src = found.url;
    if (link) link.href = found.url;
  }
}

function renderContextInspector() {
  const host = $("inspector-context-content");
  if (!host) return;
  const ws = state.workspaces.find((w) => w.id === state.activeWorkspaceId);
  if (!ws) {
    host.innerHTML = `<span class="tiny-muted">${tr("请先选择工作区。", "Select a workspace first.")}</span>`;
    return;
  }

  const trustCount = Object.keys(ws.trusted_hashes || {}).length;
  const skillsHTML = (state.workspaceSkills || []).length > 0
    ? state.workspaceSkills.map((sk) => `<button class="chip-btn mono" type="button" data-insert-skill="${esc(sk.name)}">$${esc(sk.name)}</button>`).join("")
    : `<span class="tiny-muted">${tr("当前无激活技能（可在设置 → 四层记忆晋升候选技能）", "No skills discovered yet.")}</span>`;

  host.innerHTML = `
    <div class="ctx-card">
      <div class="subhead-row">
        <strong>📁 ${esc(ws.name)}</strong>
        <span class="status-pill ${trustCount > 0 ? "ok" : ""}">${trustCount > 0 ? tr(`已信任 ${trustCount} 项`, `${trustCount} trusted`) : tr("标准策略", "standard")}</span>
      </div>
      <code class="mono tiny-muted" style="word-break:break-all;">${esc(ws.root_path)}</code>
      <div class="subhead-row">
        <span class="tiny-muted">${tr("门控", "Gate")}: <code>${esc(ws.permission_mode || "auto")}</code> · ${tr("沙箱", "Sandbox")}: <code>${esc(ws.sandbox_mode || "workspace-write")}</code></span>
        <div class="row-actions">
          <button class="ghost-xs-btn" type="button" id="ctx-trust-ws-btn">${tr("信任 .gorness", "Trust .gorness")}</button>
        </div>
      </div>
    </div>

    <div class="ctx-card">
      <div class="subhead-row">
        <strong>⚡ ${tr("可用技能 (点击插入 $skill)", "Active Skills (click to insert)")}</strong>
        <span class="meta-chip mono">${(state.workspaceSkills || []).length}</span>
      </div>
      <div class="chips-wrap">${skillsHTML}</div>
    </div>

    <div class="ctx-card">
      <div class="subhead-row">
        <strong>🧠 ${tr("四层记忆与经验提取", "4-Layer Memory & Candidate Skills")}</strong>
      </div>
      <span class="tiny-muted">${tr(
        "全局 INSTRUCTIONS.md、INDEX.md、主题笔记与项目级进度记忆在每轮对话自动按需注入。",
        "Global INSTRUCTIONS.md, INDEX.md, topic notes & workspace progress inject automatically."
      )}</span>
    </div>
  `;
}

// ============================================================================
// Role & Model Selectors
// ============================================================================

function getModelsForProvider(provId) {
  const cfgProvs = state.config?.providers || {};
  const out = new Map();
  const cfgModels = cfgProvs[provId]?.models || {};
  Object.entries(cfgModels).forEach(([mId, mSpec]) => {
    out.set(mId, mSpec?.name || mId);
  });
  if (out.size === 0) {
    const presetModels = PROVIDER_PRESETS[provId]?.models || {};
    Object.entries(presetModels).forEach(([mId, mSpec]) => {
      if (!out.has(mId)) out.set(mId, mSpec.name || mId);
    });
  }
  return Array.from(out.entries()).map(([id, name]) => ({ id, name }));
}

function getAllAvailableModelRefs() {
  const provs = state.config?.providers || {};
  const list = [];
  Object.entries(provs).forEach(([pId, pCfg]) => {
    const pName = pCfg.name || pId;
    const models = getModelsForProvider(pId);
    models.forEach((m) => {
      list.push({
        ref: `${pId}/${m.id}`,
        provider: pId,
        providerName: pName,
        model: m.id,
        name: m.name,
        label: `${pName} · ${m.name} (${pId}/${m.id})`
      });
    });
  });
  return list;
}

function renderModelCatalogOptgroupsHTML(selectedValue = "", emptyOptLabel = "") {
  const provs = state.config?.providers || {};
  const parts = [];
  if (emptyOptLabel !== null && emptyOptLabel !== undefined && emptyOptLabel !== false) {
    parts.push(`<option value="" ${!selectedValue ? "selected" : ""}>${esc(emptyOptLabel)}</option>`);
  }
  let foundSelected = !selectedValue;
  Object.entries(provs).forEach(([pId, pCfg]) => {
    const pName = pCfg.name || pId;
    const models = getModelsForProvider(pId);
    if (models.length === 0) return;
    const optionsHTML = models
      .map((m) => {
        const ref = `${pId}/${m.id}`;
        if (ref === selectedValue) foundSelected = true;
        return `<option value="${esc(ref)}" ${ref === selectedValue ? "selected" : ""}>${esc(m.name)} (${esc(ref)})</option>`;
      })
      .join("");
    parts.push(`<optgroup label="${esc(pName)} (${esc(pId)})">${optionsHTML}</optgroup>`);
  });
  if (selectedValue && !foundSelected) {
    parts.push(`<option value="${esc(selectedValue)}" selected>${esc(selectedValue)}</option>`);
  }
  return parts.join("");
}

function populateAgentAndModelControls() {
  const roleSel = $("agent-role-select");
  if (roleSel && state.config?.agents) {
    const prev = roleSel.value || "coder";
    const roles = Object.keys(state.config.agents).sort();
    roleSel.innerHTML = roles
      .map((r) => {
        const a = state.config.agents[r];
        return `<option value="${esc(r)}">${esc(r)} (${esc(a.model || state.config.global?.default_model || "")})</option>`;
      })
      .join("");
    if (roles.includes(prev)) {
      roleSel.value = prev;
    } else if (roles.includes("coder")) {
      roleSel.value = "coder";
    }
  }

  const modelSel = $("model-override-input");
  if (modelSel) {
    const prevVal = modelSel.value || state.activeSessionMeta?.model_override || "";
    modelSel.innerHTML = renderModelCatalogOptgroupsHTML(prevVal, t("autoModelOpt"));
    modelSel.value = prevVal;
  }
}

// ============================================================================
// Conversation Timeline (Session-Scoped Subagents ONLY — Zero Fleet Sprawl!)
// ============================================================================

function renderSubagentDelegationCard(toolBlock, pairedResult) {
  let inputObj = {};
  if (toolBlock.tool_input) {
    try {
      inputObj = typeof toolBlock.tool_input === "string" ? JSON.parse(toolBlock.tool_input) : toolBlock.tool_input;
    } catch {}
  }
  const subRole = inputObj.agent_role || inputObj.role || "explorer";
  const subTask = inputObj.prompt || inputObj.task || inputObj.goal || "";
  const isDone = Boolean(pairedResult);
  const isErr = Boolean(pairedResult?.is_error);
  const statusClass = isErr ? "err" : isDone ? "completed" : "running";
  const statusText = isErr ? tr("失败", "failed") : isDone ? tr("已完成", "done") : tr("运行中", "running");

  return `
    <details class="subagent-card ${isErr ? "is-error" : ""}" ${!isDone ? "open" : ""}>
      <summary>
        <span class="subagent-summary-left">
          <span>🤖 ${tr("子智能体委派", "Subagent")}</span>
          <span class="subagent-role-pill">${esc(subRole)}</span>
          <span class="tiny-muted" style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:400px;">${esc(subTask)}</span>
        </span>
        <span class="status-pill ${statusClass}">${esc(statusText)}</span>
      </summary>
      <div class="subagent-body">
        ${subTask ? `<div><strong>${tr("任务指令", "Task")}:</strong> ${esc(subTask)}</div>` : ""}
        ${
          pairedResult
            ? `<div class="tool-block ${isErr ? "is-error" : ""}">
                <div class="tool-block-head mono">
                  <span>↳ ${esc(subRole)} ${tr("返回结果", "Result")}</span>
                  <span>${isErr ? tr("错误", "error") : tr("完成", "ok")}</span>
                </div>
                <pre>${esc(pairedResult.text || "")}</pre>
              </div>`
            : `<div class="tiny-muted mono">${tr("子智能体正在执行…", "Subagent running…")}</div>`
        }
      </div>
    </details>
  `;
}

function renderForkedChildSubagentCard(childSess) {
  const isRunning = Boolean(state.activeRunsBySession[childSess.id]);
  return `
    <details class="subagent-card">
      <summary>
        <span class="subagent-summary-left">
          <span>🌿 ${tr("分叉子会话", "Forked Branch")}</span>
          <span class="subagent-role-pill">${esc(childSess.agent_role || "coder")}</span>
          <span>${esc(childSess.title || childSess.id)}</span>
        </span>
        <span class="row-actions">
          <span class="meta-chip mono">fork @#${childSess.forked_at_seq || 0}</span>
          <span class="status-pill ${isRunning ? "running" : "ok"}">${isRunning ? tr("运行中", "running") : `seq #${childSess.head_seq || 0}`}</span>
        </span>
      </summary>
      <div class="subagent-body">
        <div class="subagent-worker-row">
          <span class="mono tiny-muted">${esc(childSess.id)} · $${Number(childSess.estimated_usd || 0).toFixed(4)}</span>
          <button class="primary-xs-btn" type="button" data-jump-session="${esc(childSess.id)}">${tr("打开子会话 ↗", "Open Branch ↗")}</button>
        </div>
      </div>
    </details>
  `;
}

function renderChatHistory() {
  const host = $("chat-history-entries");
  if (!host) return;

  const forkedChildren = state.activeSessionId
    ? (state.sessions || []).filter((s) => s.parent_session_id === state.activeSessionId && !s.archived)
    : [];

  const errorBannerCardHTML = state.lastRunError
    ? `<article class="msg-card chat-error-card">
        <div class="msg-head">
          <span class="msg-role-badge">⚠️ ${tr("执行异常", "Run Error")}</span>
          <button class="ghost-xs-btn" type="button" data-dismiss-run-error="1">${tr("关闭", "Dismiss")}</button>
        </div>
        <div class="msg-text mono">${esc(state.lastRunError)}</div>
      </article>`
    : "";

  if ((!state.activeEntries || state.activeEntries.length === 0) && forkedChildren.length === 0) {
    host.innerHTML =
      errorBannerCardHTML +
      `
      <div class="onboarding-empty-state">
        <div class="onboarding-hero-icon" aria-hidden="true">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12 3L19 7V17L12 21L5 17V7L12 3Z"/>
            <circle cx="12" cy="12" r="2.5"/>
          </svg>
        </div>
        <h3 class="onboarding-title">${tr("开始探索与构建你的项目", "Explore & Build Your Workspace")}</h3>
        <p class="onboarding-sub">${esc(t("emptyTimeline"))}</p>
        <div class="onboarding-grid">
          <button class="onboarding-card" type="button" data-onboarding-card="arch">
            <span class="onboarding-card-title">🏗️ ${tr("解释当前项目架构", "Explain Project Architecture")}</span>
            <span class="onboarding-card-desc">${tr("梳理核心模块职责、数据流向与关键入口文件", "Map core modules, data flows, and entry points")}</span>
          </button>
          <button class="onboarding-card" type="button" data-onboarding-card="git">
            <span class="onboarding-card-title">🔍 ${tr("审查 Git 未提交变更与潜在 Bug", "Review Uncommitted Git Changes")}</span>
            <span class="onboarding-card-desc">${tr("引用 @git 检查当前改动中的竞态、边界条件与回归风险", "Cite @git to audit current diff for bugs and edge cases")}</span>
          </button>
          <button class="onboarding-card" type="button" data-onboarding-card="tdd">
            <span class="onboarding-card-title">🧪 ${tr("为核心模块编写单元测试", "Write Unit Tests for Core Modules")}</span>
            <span class="onboarding-card-desc">${tr("按 TDD 规范补充高覆盖率单元测试并运行验证", "Generate table-driven tests and verify with test runner")}</span>
          </button>
          <button class="onboarding-card" type="button" data-onboarding-card="fleet">
            <span class="onboarding-card-title">🛸 ${tr("切换多智能体舰队协同", "Switch to Multi-Agent Fleet")}</span>
            <span class="onboarding-card-desc">${tr("启动异构模型并发探索、Worktree 隔离编码与盲审合入", "Launch heterogeneous agents with isolated Git worktrees")}</span>
          </button>
        </div>
      </div>
      `;
    return;
  }

  const subagentCallIDs = new Set();
  const subagentResultByCallID = new Map();
  (state.activeEntries || []).forEach((ent) => {
    (ent.blocks || []).forEach((b) => {
      if (b.type === "tool_use" && b.tool_name === "spawn_subagent" && b.tool_call_id) {
        subagentCallIDs.add(b.tool_call_id);
      }
    });
  });
  (state.activeEntries || []).forEach((ent) => {
    (ent.blocks || []).forEach((b) => {
      if (b.type === "tool_result" && b.tool_call_id && (b.tool_name === "spawn_subagent" || subagentCallIDs.has(b.tool_call_id))) {
        subagentResultByCallID.set(b.tool_call_id, b);
      }
    });
  });

  const forksBySeq = new Map();
  forkedChildren.forEach((ch) => {
    const seqKey = Number(ch.forked_at_seq || 0);
    const arr = forksBySeq.get(seqKey) || [];
    arr.push(ch);
    forksBySeq.set(seqKey, arr);
  });
  const renderedForkIDs = new Set();

  const cards = [];
  (state.activeEntries || []).forEach((ent) => {
    const blocksHTML = (ent.blocks || [])
      .map((b) => {
        if (b.type === "thinking" && b.thinking) {
          return `<details class="thinking-box"><summary>${esc(t("thinkingSummary"))}</summary><pre>${esc(b.thinking)}</pre></details>`;
        }
        if (b.type === "text" && b.text) {
          return `<div class="msg-text">${esc(b.text)}</div>`;
        }
        if (b.type === "tool_use") {
          if (b.tool_name === "spawn_subagent") {
            return renderSubagentDelegationCard(b, subagentResultByCallID.get(b.tool_call_id));
          }
          const rawIn = typeof b.tool_input === "string" ? b.tool_input : JSON.stringify(b.tool_input || {}, null, 2);
          return `<div class="tool-block"><div class="tool-block-head mono"><span>⚙ ${esc(b.tool_name)}</span><span>${esc(b.tool_call_id || "")}</span></div><pre>${esc(rawIn)}</pre></div>`;
        }
        if (b.type === "tool_result") {
          if (b.tool_call_id && subagentResultByCallID.has(b.tool_call_id)) {
            return "";
          }
          return `<div class="tool-block ${b.is_error ? "is-error" : ""}"><div class="tool-block-head mono"><span>↳ ${esc(b.tool_name || tr("结果", "result"))}</span><span>${b.is_error ? tr("错误", "error") : tr("完成", "ok")}</span></div><pre>${esc(b.text || "")}</pre></div>`;
        }
        return "";
      })
      .filter(Boolean)
      .join("");

    const seqForks = forksBySeq.get(Number(ent.seq)) || [];
    const inlineForksHTML = seqForks
      .map((ch) => {
        renderedForkIDs.add(ch.id);
        return renderForkedChildSubagentCard(ch);
      })
      .join("");

    if (!blocksHTML && !inlineForksHTML && ent.role === "tool") {
      return;
    }

    cards.push(`
      <article class="msg-card role-${esc(ent.role)}">
        <div class="msg-head">
          <span class="msg-role-badge">${esc(ent.role)}${ent.agent_role ? ` · ${esc(ent.agent_role)}` : ""}${ent.model_id ? ` (${esc(ent.model_id)})` : ""}</span>
          <div class="row-actions">
            <span class="mono">#${ent.seq}</span>
            <button class="ghost-xs-btn" type="button" data-fork-seq="${ent.seq}" title="${esc(t("forkHere"))}">⑂</button>
            <button class="ghost-xs-btn" type="button" data-rewind-seq="${ent.seq}" title="${esc(t("rewindHere"))}">↺</button>
          </div>
        </div>
        ${blocksHTML}
        ${inlineForksHTML}
      </article>
    `);
  });

  forkedChildren.forEach((ch) => {
    if (!renderedForkIDs.has(ch.id)) {
      cards.push(`<article class="msg-card">${renderForkedChildSubagentCard(ch)}</article>`);
    }
  });

  host.innerHTML = cards.join("") + errorBannerCardHTML;
}

function renderLiveStreamingCard() {
  const host = $("chat-live-stream");
  if (!host) return;
  if (!state.streamingThinking && !state.streamingText && state.streamingTools.length === 0) {
    host.innerHTML = "";
    return;
  }

  const thinkHTML = state.streamingThinking
    ? `<details class="thinking-box" open><summary>${esc(t("thinkingSummary"))}</summary><pre>${esc(state.streamingThinking)}</pre></details>`
    : "";
  const toolsHTML = state.streamingTools
    .map((tb) => {
      if (tb.name === "spawn_subagent") {
        return renderSubagentDelegationCard(
          { tool_name: "spawn_subagent", tool_input: tb.input, tool_call_id: tb.id },
          tb.result !== undefined ? { text: tb.result, is_error: tb.isError } : null
        );
      }
      return `<div class="tool-block ${tb.isError ? "is-error" : ""}">
        <div class="tool-block-head mono"><span>⚙ ${esc(tb.name)}</span><span>${esc(tb.status)}</span></div>
        <pre>${esc(tb.output || tb.input || "")}</pre>
      </div>`;
    })
    .join("");
  const textHTML = state.streamingText ? `<div class="msg-text">${esc(state.streamingText)}</div>` : "";

  host.innerHTML = `
    <article class="msg-card role-assistant">
      <div class="msg-head">
        <span class="msg-role-badge"><span class="pulse-dot live"></span> ${tr("流式生成中…", "streaming…")}</span>
      </div>
      ${thinkHTML}
      ${toolsHTML}
      ${textHTML}
    </article>
  `;
  scrollTimelineBottom();
}

function scrollTimelineBottom() {
  const tl = $("chat-timeline");
  if (!tl) return;
  const nearBottom = tl.scrollHeight - tl.scrollTop - tl.clientHeight < 180;
  if (nearBottom) {
    tl.scrollTop = tl.scrollHeight;
  }
}

function updateRunActionState() {
  const activeRun = state.activeSessionId ? state.activeRunsBySession[state.activeSessionId] : null;
  const sendBtn = $("send-or-steer-btn");
  const stopBtn = $("stop-run-btn");
  if (activeRun) {
    if (sendBtn) sendBtn.textContent = t("steerBtn");
    stopBtn?.classList.remove("hidden");
  } else {
    if (sendBtn) sendBtn.textContent = t("sendBtn");
    stopBtn?.classList.add("hidden");
  }
}

async function sendPromptOrSteer() {
  const comp = $("composer-input");
  const raw = (comp?.value || "").trim();
  if (!raw) return;

  if (!state.activeWorkspaceId) {
    populateWorkspaceSuggestions();
    $("workspace-modal")?.classList.remove("hidden");
    return;
  }

  // Mid-run step-boundary steer via canonical {"instruction": ...}
  const activeRun = state.activeSessionId ? state.activeRunsBySession[state.activeSessionId] : null;
  if (activeRun && activeRun.run_id) {
    try {
      await apiFetch(`/api/runs/${encodeURIComponent(activeRun.run_id)}/steer`, {
        method: "POST",
        body: { instruction: raw }
      });
      if (comp) comp.value = "";
      showToast(tr("已注入步边界 Steer 指令", "Steer instruction queued"), "ok");
    } catch (err) {
      notifyError(err, tr("注入 Steer 失败", "Steer failed"));
    }
    return;
  }

  // Built-in Slash Commands
  if (raw.startsWith("/")) {
    const handled = await handleSlashCommand(raw);
    if (handled) {
      if (comp) comp.value = "";
      return;
    }
  }

  if (state.draftSaveTimer) {
    clearTimeout(state.draftSaveTimer);
    state.draftSaveTimer = null;
  }

  closeAttachedRunSSE();
  state.lastRunError = "";
  state.streamingText = "";
  state.streamingThinking = "";
  state.streamingTools = [];
  renderReloadBanner("");

  const role = $("agent-role-select")?.value || "coder";
  const perm = $("perm-mode-select")?.value || "auto";
  const modelOverride = ($("model-override-input")?.value || "").trim();

  const resp = await fetch("/api/runs/stream", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      workspace_id: state.activeWorkspaceId,
      session_id: state.activeSessionId,
      agent_role: role,
      model_override: modelOverride,
      perm_mode: perm,
      prompt: raw
    })
  });

  if (!resp.ok) {
    const errText = await resp.text();
    let msg = errText;
    try {
      const errObj = JSON.parse(errText);
      msg = errObj.error || errText;
    } catch {}
    state.lastRunError = msg;
    renderReloadBanner(msg);
    renderChatHistory();
    showToast(msg, "error");
    return;
  }

  if (comp) comp.value = "";
  if ($("draft-saved-indicator")) $("draft-saved-indicator").textContent = "";

  const runId = resp.headers.get("X-Gorness-Run-ID") || "";
  if (state.activeSessionId && runId) {
    state.activeRunsBySession[state.activeSessionId] = { run_id: runId, session_id: state.activeSessionId };
    state.inlineStreamSessionId = state.activeSessionId;
    updateRunActionState();
  }

  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const frames = buffer.split("\n\n");
    buffer = frames.pop() || "";

    for (const frame of frames) {
      const dataLine = frame
        .split("\n")
        .find((l) => l.startsWith("data:"));
      if (!dataLine) continue;
      const jsonStr = dataLine.slice(5).trim();
      if (!jsonStr) continue;
      let ev;
      try {
        ev = JSON.parse(jsonStr);
      } catch {
        continue;
      }
      handleRunStreamEvent(ev);
    }
  }

  state.inlineStreamSessionId = "";
  state.streamingText = "";
  state.streamingThinking = "";
  state.streamingTools = [];
  renderLiveStreamingCard();

  if (state.activeSessionId) {
    delete state.activeRunsBySession[state.activeSessionId];
    const savedRunErr = state.lastRunError;
    await selectSession(state.activeSessionId);
    if (savedRunErr) {
      state.lastRunError = savedRunErr;
      renderReloadBanner(savedRunErr);
      renderChatHistory();
    }
    state.sessions = await apiFetch("/api/sessions?include_archived=true");
    renderWorkspaceTree();
  }
  if (state.activeWorkspaceId) {
    await refreshWorkspaceInspector(state.activeWorkspaceId);
  }
  updateRunActionState();
}

// /rewind [seq] [--no-files] defaults restore_files: true unless --no-files is explicitly passed!
async function handleSlashCommand(raw) {
  const tokens = raw.trim().split(/\s+/);
  const cmd = tokens[0].toLowerCase();
  if (cmd === "/compact") {
    await compactActiveSession();
    return true;
  }
  if (cmd === "/fork") {
    const seqArg = tokens[1] ? Number(tokens[1]) : undefined;
    await forkAtSeq(Number.isFinite(seqArg) ? seqArg : undefined);
    return true;
  }
  if (cmd === "/rewind") {
    const hasNoFiles = tokens.some((tk) => tk === "--no-files" || tk === "--keep-files");
    const numTok = tokens.slice(1).find((tk) => !tk.startsWith("-"));
    const toSeq = numTok !== undefined && Number.isFinite(Number(numTok)) ? Number(numTok) : 0;
    await rewindToSeq(toSeq, !hasNoFiles);
    return true;
  }
  if (cmd === "/fleet") {
    setCenterMode("fleet");
    const rest = tokens.slice(1).join(" ").trim();
    if (rest && $("fleet-goal-input")) {
      $("fleet-goal-input").value = rest;
      $("launch-fleet-form")?.classList.remove("hidden");
    }
    return true;
  }
  if (cmd === "/settings" || cmd === "/config") {
    openSettingsModal(tokens[1] || "matrix");
    return true;
  }
  if (cmd === "/alwayson") {
    openAlwaysOnDrawer();
    return true;
  }
  if (cmd === "/worktree") {
    await createNewSession(true);
    return true;
  }
  return false;
}

function handleRunStreamEvent(ev) {
  if (!ev || ev.type === "heartbeat") return;
  const p = ev.payload || {};

  if (ev.session_id && !state.activeSessionId) {
    state.activeSessionId = ev.session_id;
    state.inlineStreamSessionId = ev.session_id;
  }
  if (ev.type === "run_started" && ev.session_id && ev.run_id) {
    state.activeRunsBySession[ev.session_id] = { run_id: ev.run_id, session_id: ev.session_id };
    updateRunActionState();
  }
  if (ev.type === "step_reset") {
    state.streamingThinking = "";
    state.streamingText = "";
    state.streamingTools = [];
    renderLiveStreamingCard();
  } else if (ev.type === "thinking_delta") {
    state.streamingThinking += p.thinking ?? ev.thinking ?? "";
    renderLiveStreamingCard();
  } else if (ev.type === "text_delta") {
    state.streamingText += p.text ?? ev.text ?? "";
    renderLiveStreamingCard();
  } else if (ev.type === "tool_call") {
    const rawArgs = p.args !== undefined ? p.args : ev.tool_input;
    const inputRaw = typeof rawArgs === "string" ? rawArgs : JSON.stringify(rawArgs || {}, null, 2);
    state.streamingTools.push({
      id: p.tool_call_id ?? ev.tool_call_id,
      name: p.tool_name ?? ev.tool_name,
      status: "running",
      input: inputRaw,
      output: "",
      result: undefined,
      isError: false
    });
    renderLiveStreamingCard();
  } else if (ev.type === "tool_result") {
    const tcId = p.tool_call_id ?? ev.tool_call_id;
    const out = p.output ?? ev.tool_output ?? "";
    const isErr = Boolean(p.is_error ?? ev.is_error);
    const found = state.streamingTools.find((x) => x.id === tcId);
    if (found) {
      found.status = isErr ? "error" : "done";
      found.output = out;
      found.result = out;
      found.isError = isErr;
    }
    renderLiveStreamingCard();
  } else if (ev.type === "doom_loop") {
    state.streamingTools.push({
      id: "doom_loop",
      name: p.tool_name || "doom_loop",
      status: "error",
      input: "",
      output: p.message || "Doom loop detected",
      result: p.message || "Doom loop detected",
      isError: true
    });
    renderLiveStreamingCard();
  } else if (ev.type === "approval_required" || ev.type === "approval_resolved") {
    refreshApprovals();
  } else if (ev.type === "run_failed") {
    const msg = p.message || p.error || ev.error || "Run failed";
    const code = p.code ? `[${p.code}] ` : "";
    const errMsg = `${code}${msg}`;
    state.lastRunError = errMsg;
    renderReloadBanner(errMsg);
    renderChatHistory();
  }
}

// ============================================================================
// Composer Palette (@file / $skill / /command) with Keyboard Navigation
// ============================================================================

function closeComposerPalette() {
  state.paletteItems = [];
  state.paletteActiveIndex = 0;
  $("composer-palette")?.classList.add("hidden");
}

function renderComposerPaletteList() {
  const palette = $("composer-palette");
  if (!palette) return;
  if (!state.paletteItems || state.paletteItems.length === 0) {
    palette.classList.add("hidden");
    return;
  }
  palette.innerHTML = state.paletteItems
    .map(
      (it, idx) =>
        `<div class="palette-option mono ${idx === state.paletteActiveIndex ? "active" : ""}" role="option" aria-selected="${idx === state.paletteActiveIndex}" data-palette-idx="${idx}" data-insert="${esc(it.insert)}"><span>${esc(it.label)}</span><span class="tiny-muted">${esc(it.hint || "")}</span></div>`
    )
    .join("");
  palette.classList.remove("hidden");
}

function updateComposerPalette() {
  const comp = $("composer-input");
  const palette = $("composer-palette");
  if (!comp || !palette) return;

  const val = comp.value;
  const caret = comp.selectionStart ?? val.length;
  const before = val.slice(0, caret);
  const match = before.match(/(?:^|\s)([@$/])([^\s]*)$/);
  if (!match) {
    closeComposerPalette();
    return;
  }

  const trig = match[1];
  const query = match[2].toLowerCase();
  let items = [];

  if (trig === "@") {
    const fileStrings = (state.workspaceFiles || [])
      .map((f) => (typeof f === "string" ? f : f?.path || f?.rel_path || ""))
      .filter(Boolean);
    const gitItems = !query || "git".includes(query)
      ? [{ insert: "@git ", label: "@git", hint: tr("当前工作区 Git 状态与未提交 Diff", "Workspace Git status & diff") }]
      : [];
    const fileItems = fileStrings
      .filter((f) => f.toLowerCase().includes(query))
      .slice(0, 12)
      .map((f) => ({ insert: `@${f} `, label: `@${f}`, hint: tr("文件", "file") }));
    items = [...gitItems, ...fileItems];
  } else if (trig === "$") {
    items = state.workspaceSkills
      .filter((s) => s.name.toLowerCase().includes(query))
      .slice(0, 12)
      .map((s) => ({ insert: `$${s.name} `, label: `$${s.name}`, hint: s.description || tr("技能", "skill") }));
  } else if (trig === "/") {
    const cmds = [
      { name: "/compact", desc: tr("锚定增量压缩会话", "Anchored iterative compaction") },
      { name: "/fork", desc: tr("从当前步分叉新会话", "Fork session branch") },
      { name: "/rewind 0", desc: tr("时光机回退并还原快照文件 (--no-files 仅回退消息)", "Rewind & restore files (--no-files keeps files)") },
      { name: "/worktree", desc: tr("新建独立 Git Worktree 会话", "Create isolated Git worktree session") },
      { name: "/fleet ", desc: tr("进入多智能体编排画布", "Open Fleet Studio") },
      { name: "/settings", desc: tr("打开系统设置中心 (⌘,)", "Open Settings Center (⌘,)") },
      { name: "/alwayson", desc: tr("打开常驻休眠唤醒抽屉", "Open Always-On Drawer") }
    ];
    items = cmds
      .filter((c) => c.name.slice(1).startsWith(query))
      .map((c) => ({ insert: c.name, label: c.name, hint: c.desc }));
  }

  if (items.length === 0) {
    closeComposerPalette();
    return;
  }

  state.paletteItems = items;
  state.paletteActiveIndex = 0;
  renderComposerPaletteList();
}

function applyPaletteItem(idx) {
  const item = state.paletteItems[idx];
  const comp = $("composer-input");
  if (!item || !comp) return false;
  const caret = comp.selectionStart ?? comp.value.length;
  const before = comp.value.slice(0, caret).replace(/([@$/])([^\s]*)$/, item.insert);
  const after = comp.value.slice(caret);
  comp.value = before + after;
  closeComposerPalette();
  comp.focus?.();
  if (typeof comp.setSelectionRange === "function") {
    comp.setSelectionRange(before.length, before.length);
  }
  return true;
}

function handleComposerKeydown(ev) {
  const paletteOpen = state.paletteItems.length > 0 && !$("composer-palette")?.classList.contains("hidden");
  if (paletteOpen) {
    if (ev.key === "ArrowDown") {
      ev.preventDefault();
      state.paletteActiveIndex = (state.paletteActiveIndex + 1) % state.paletteItems.length;
      renderComposerPaletteList();
      return;
    }
    if (ev.key === "ArrowUp") {
      ev.preventDefault();
      state.paletteActiveIndex = (state.paletteActiveIndex - 1 + state.paletteItems.length) % state.paletteItems.length;
      renderComposerPaletteList();
      return;
    }
    if (ev.key === "Tab" || (ev.key === "Enter" && !ev.shiftKey)) {
      ev.preventDefault();
      applyPaletteItem(state.paletteActiveIndex);
      return;
    }
    if (ev.key === "Escape") {
      ev.preventDefault();
      closeComposerPalette();
      return;
    }
  }
  if (ev.key === "Enter" && !ev.shiftKey) {
    ev.preventDefault();
    sendPromptOrSteer();
  }
}

function scheduleDraftSave() {
  if (!state.activeSessionId || !state.sessionLoaded) return;
  if (state.draftSaveTimer) clearTimeout(state.draftSaveTimer);
  const targetSessionId = state.activeSessionId;
  const draftVal = $("composer-input")?.value || "";
  state.draftSaveTimer = setTimeout(async () => {
    if (state.activeSessionId !== targetSessionId || !state.sessionLoaded) return;
    try {
      await apiFetch(`/api/sessions/${encodeURIComponent(targetSessionId)}`, {
        method: "PATCH",
        body: { draft: draftVal }
      });
      const ind = $("draft-saved-indicator");
      if (ind) ind.textContent = tr("草稿已保存", "draft saved");
    } catch {}
  }, 450);
}

// ============================================================================
// 500ms-Guarded Partial Hunk Diff Approvals & Cross-Workspace Attention Queue
// ============================================================================

async function refreshApprovals() {
  try {
    state.pendingApprovals = await apiFetch("/api/approvals");
    renderAttentionQueue();
    renderApprovalCards();
  } catch {}
}

function renderAttentionQueue() {
  const sec = $("attention-section");
  const list = $("attention-list");
  const badge = $("attention-badge");
  if (!sec || !list || !badge) return;

  const defPending = (state.alwaysOn.deferred || []).filter((d) => d.status === "pending");
  const total = state.pendingApprovals.length + defPending.length;
  if (total === 0) {
    sec.classList.add("hidden");
    return;
  }

  sec.classList.remove("hidden");
  badge.textContent = String(total);

  const items = [
    ...state.pendingApprovals.map(
      (a) => `
      <div class="attention-card" data-jump-session="${esc(a.session_id)}">
        <div class="mono"><strong>${esc(a.tool_name)}</strong></div>
        <div class="tiny-muted">${esc(a.reason || a.summary)}</div>
      </div>`
    ),
    ...defPending.map(
      (d) => `
      <div class="attention-card" data-open-alwayson="1">
        <div class="mono"><strong>⏰ ${esc(d.tool_name)}</strong></div>
        <div class="tiny-muted">${esc(d.summary || d.reason)}</div>
      </div>`
    )
  ];
  list.innerHTML = items.join("");
}

function renderApprovalCards() {
  const host = $("approval-banner-host");
  if (!host) return;
  state.approvalTimers.forEach((id) => clearInterval(id));
  state.approvalTimers.clear();

  const relevant = state.pendingApprovals.filter(
    (a) => !state.activeSessionId || a.session_id === state.activeSessionId
  );
  if (relevant.length === 0) {
    host.innerHTML = "";
    return;
  }

  host.innerHTML = relevant
    .map((appr) => {
      const hunks = appr.hunks || [];
      const hunksHTML = hunks
        .map(
          (hk) => `
          <div class="hunk-box">
            <label class="hunk-head">
              <input type="checkbox" class="hunk-cb" data-appr-id="${esc(appr.id)}" value="${esc(hk.id)}" checked>
              <span>${escHtml(hk.file_path || "")} ${escHtml(hk.header || "")}</span>
            </label>
            <pre class="hunk-preview mono">${renderDiffPreviewHTML(hk.preview || "")}</pre>
          </div>
        `
        )
        .join("");

      const cmdSummary = !hunks.length ? `<pre class="hunk-preview mono">${escHtml(appr.summary || "")}</pre>` : "";

      return `
        <div class="approval-card" data-card-appr="${esc(appr.id)}">
          <div class="approval-head">
            <strong>🛡️ ${esc(appr.tool_name)}</strong>
            <span class="status-pill" id="settle-label-${esc(appr.id)}">${esc(t("settleGuardWait"))}</span>
          </div>
          <div class="approval-reason">${esc(appr.reason || "")}</div>
          <div class="settle-bar-track"><div class="settle-bar-fill" id="settle-bar-${esc(appr.id)}" style="width:0%"></div></div>
          ${cmdSummary}
          ${hunksHTML}
          <div class="approval-actions">
            <label class="checkbox-row">
              <input type="checkbox" id="remember-${esc(appr.id)}">
              <span>${esc(t("rememberRule"))}</span>
            </label>
            <div class="row-actions">
              <button class="danger-xs-btn appr-act-btn" type="button" data-appr-resolve="${esc(appr.id)}" data-act="deny" disabled>${esc(t("denyBtn"))}</button>
              ${
                hunks.length > 0
                  ? `<button class="ghost-xs-btn appr-act-btn" type="button" data-appr-resolve="${esc(appr.id)}" data-act="approve_hunks" disabled>${esc(t("approveSelected"))}</button>`
                  : ""
              }
              <button class="primary-xs-btn appr-act-btn" type="button" data-appr-resolve="${esc(appr.id)}" data-act="approve_all" disabled>${esc(t("approveAll"))}</button>
            </div>
          </div>
        </div>
      `;
    })
    .join("");

  relevant.forEach((appr) => {
    const settleMs = new Date(appr.settle_at).getTime();
    const createdMs = new Date(appr.created_at).getTime();
    const span = Math.max(500, settleMs - createdMs);

    const tick = () => {
      const now = Date.now();
      const pct = Math.min(100, Math.max(0, ((now - createdMs) / span) * 100));
      const bar = $(`settle-bar-${appr.id}`);
      if (bar) bar.style.width = `${pct}%`;
      if (now >= settleMs) {
        const lbl = $(`settle-label-${appr.id}`);
        if (lbl) {
          lbl.textContent = tr("可审批", "ready");
          lbl.className = "status-pill ok";
        }
        $$(`[data-card-appr="${CSS.escape(appr.id)}"] .appr-act-btn`).forEach((b) => {
          b.disabled = false;
        });
        clearInterval(timerId);
        state.approvalTimers.delete(appr.id);
      }
    };
    const timerId = setInterval(tick, 50);
    state.approvalTimers.set(appr.id, timerId);
    tick();
  });
}

async function resolveApproval(apprId, action) {
  const checkedHunks = $$(`.hunk-cb[data-appr-id="${CSS.escape(apprId)}"]:checked`).map((el) => el.value);
  const remember = $(`remember-${apprId}`)?.checked || false;
  try {
    await apiFetch(`/api/approvals/${encodeURIComponent(apprId)}/resolve`, {
      method: "POST",
      body: {
        action,
        selected_hunk_ids: checkedHunks,
        remember_for_workspace: remember
      }
    });
    await refreshApprovals();
  } catch (err) {
    if (err.status === 409) {
      setTimeout(() => resolveApproval(apprId, action), 160);
    } else {
      notifyError(err, tr("审批提交失败", "Failed to resolve approval"));
    }
  }
}

// ============================================================================
// Mode 2: Unified Visual Multi-Agent Fleet Studio
// ============================================================================

function syncFleetLauncherWorkflows() {
  const wfSel = $("fleet-workflow-select");
  if (!wfSel) return;
  const workflows = state.config?.workflows || {};
  const ids = Object.keys(workflows).sort();
  const prev = wfSel.value || (ids.includes("hybrid-fleet") ? "hybrid-fleet" : ids[0] || "");
  const opts = ids.map((id) => {
    const wf = workflows[id];
    return `<option value="${esc(id)}">${esc(wf.name || id)} (${esc(wf.topology || id)})</option>`;
  });
  opts.push(`<option value="">${tr("自定义临时拓扑 (不绑定模板)", "Ad-hoc Custom Topology")}</option>`);
  wfSel.innerHTML = opts.join("");
  if (ids.includes(prev) || prev === "") {
    wfSel.value = prev;
  }
  applyWorkflowDefaultsToLauncher(wfSel.value);
}

function applyWorkflowDefaultsToLauncher(wfId) {
  const wf = (state.config?.workflows || {})[wfId];
  if (!wf) return;
  if ($("fleet-topology-select") && wf.topology) {
    $("fleet-topology-select").value = wf.topology;
  }
  if ($("fleet-verify-input") && !$("fleet-verify-input").dataset.userEdited) {
    $("fleet-verify-input").value = wf.verify_command || "";
  }
  if ($("fleet-budget-input") && wf.budget_usd !== undefined) {
    $("fleet-budget-input").value = String(wf.budget_usd || 0);
  }
  if ($("fleet-worktrees-cb") && wf.use_worktrees !== undefined) {
    $("fleet-worktrees-cb").checked = Boolean(wf.use_worktrees);
  }
  renderTopologyCards();
}

function renderTopologyCards() {
  const grid = $("topology-cards-grid");
  if (!grid) return;
  const currentTopo = $("fleet-topology-select")?.value || "hybrid-fleet";
  grid.innerHTML = SIX_TOPOLOGIES.map(
    (tp) => `
      <div class="topo-card ${tp.id === currentTopo ? "selected" : ""}" data-topo-pick="${esc(tp.id)}">
        <strong>${esc(state.lang === "zh" ? tp.zh : tp.en)}</strong>
        <span>${esc(state.lang === "zh" ? tp.descZh : tp.descEn)}</span>
      </div>
    `
  ).join("");
}

function activeMission() {
  return state.missions.find((m) => m.id === state.activeMissionId) || state.missions[0] || null;
}

function closeMissionSSE() {
  if (state.missionSSE) {
    state.missionSSE.close();
    state.missionSSE = null;
  }
  state.missionSSEId = "";
}

function ensureActiveMissionSSE() {
  const snap = activeMission();
  if (!snap || snap.status !== "running") {
    closeMissionSSE();
    return;
  }
  if (state.missionSSEId === snap.id && state.missionSSE && state.missionSSE.readyState !== 2) {
    return;
  }
  subscribeMissionStream(snap.id);
}

function renderFleetStudio() {
  const liveCount = state.missions.filter((m) => m.status === "running").length;
  const badge = $("fleet-live-badge");
  if (badge) {
    badge.textContent = String(liveCount);
    badge.classList.toggle("hidden", liveCount === 0);
  }

  const sel = $("mission-select");
  if (sel) {
    if (state.missions.length === 0) {
      sel.innerHTML = `<option value="">${tr("暂无编排记录 — 点击右侧 + 新建舰队编排", "No missions yet — click + Launch Fleet")}</option>`;
    } else {
      sel.innerHTML = state.missions
        .map((m) => `<option value="${esc(m.id)}" ${m.id === state.activeMissionId ? "selected" : ""}>[${esc(m.topology)}] ${esc((m.goal || m.id).slice(0, 44))} (${esc(m.status)})</option>`)
        .join("");
    }
  }

  const snap = activeMission();
  const statusPill = $("mission-status-pill");
  const costPill = $("mission-cost-pill");
  const stopBtn = $("stop-mission-btn");
  const delBtn = $("delete-mission-btn");

  if (!snap) {
    if (statusPill) {
      statusPill.textContent = tr("空闲", "idle");
      statusPill.className = "status-pill";
    }
    if (costPill) costPill.textContent = "$0.0000";
    stopBtn?.classList.add("hidden");
    delBtn?.classList.add("hidden");
    renderFleetSVG(null);
    return;
  }

  state.activeMissionId = snap.id;
  if (statusPill) {
    statusPill.textContent = snap.status;
    statusPill.className = `status-pill ${snap.status}`;
  }
  if (costPill) costPill.textContent = `$${Number(snap.total_cost_usd ?? snap.estimated_usd ?? 0).toFixed(4)}`;
  stopBtn?.classList.toggle("hidden", snap.status !== "running");
  delBtn?.classList.toggle("hidden", snap.status === "running");
  if ($("fleet-goal-banner")) $("fleet-goal-banner").textContent = `${snap.topology} — ${snap.goal || ""}`;

  const qBar = $("refinery-queue-bar");
  if (qBar) {
    const q = snap.merge_queue || [];
    qBar.innerHTML = q.length
      ? `<span class="meta-chip mono">Refinery: ${q.map(esc).join(" → ")}</span>`
      : "";
  }

  renderFleetSVG(snap);
  renderBlackboard(snap);
  if (state.selectedNodeId) {
    renderSelectedNodeDetails(snap, state.selectedNodeId, false);
  }
}

function subscribeMissionStream(missionId) {
  if (!missionId || typeof EventSource === "undefined") return;
  closeMissionSSE();
  const es = new EventSource(`/api/fleet/missions/${encodeURIComponent(missionId)}/stream`);
  state.missionSSE = es;
  state.missionSSEId = missionId;

  const handler = (ev) => {
    let mev;
    try {
      mev = JSON.parse(ev.data);
    } catch {
      return;
    }
    if (mev.type === "node_text_delta" && mev.node_id) {
      const chunk = mev.message || mev.payload?.text || mev.delta || "";
      state.nodeStreamBuffers[mev.node_id] = (state.nodeStreamBuffers[mev.node_id] || "") + chunk;
      if (state.selectedNodeId === mev.node_id && $("selected-node-stream")) {
        $("selected-node-stream").textContent = state.nodeStreamBuffers[mev.node_id];
      }
    }
    if (mev.snapshot) {
      const idx = state.missions.findIndex((m) => m.id === mev.snapshot.id);
      if (idx >= 0) state.missions[idx] = mev.snapshot;
      else state.missions.unshift(mev.snapshot);
      renderFleetStudio();
    }
    if (mev.type === "mission_completed" || mev.type === "mission_failed" || mev.type === "mission_cancelled") {
      closeMissionSSE();
      if (state.activeWorkspaceId) refreshWorkspaceInspector(state.activeWorkspaceId);
    }
  };

  [
    "snapshot_sync", "mission_started", "node_queued", "node_started",
    "node_text_delta", "node_tool_call", "node_completed", "node_failed",
    "review_verdict", "refinery_queued", "refinery_merging", "refinery_merged",
    "refinery_rejected", "blackboard_sync", "mission_completed", "mission_failed", "mission_cancelled"
  ].forEach((evt) => es.addEventListener(evt, handler));
}

function renderFleetSVG(snap) {
  const svg = $("fleet-svg-canvas");
  if (!svg) return;
  if (!snap || !snap.nodes || snap.nodes.length === 0) {
    svg.innerHTML = `
      <text x="460" y="205" text-anchor="middle" fill="var(--ink-secondary)" font-size="13">
        ${esc(t("fleetEmptyGoal"))}
      </text>
      <text x="460" y="230" text-anchor="middle" fill="var(--ink-tertiary)" font-size="11.5">
        hybrid-fleet · review-loop · parallel-explore · worktree-team · event-pipeline · shared-board
      </text>
    `;
    return;
  }

  const nodes = snap.nodes;
  const edges = snap.edges || [];
  const nodeMap = new Map(nodes.map((n) => [n.id, n]));

  const inDeg = new Map(nodes.map((n) => [n.id, 0]));
  edges.forEach((e) => {
    if (e.kind !== "feedback" && inDeg.has(e.to)) {
      inDeg.set(e.to, (inDeg.get(e.to) || 0) + 1);
    }
  });

  const layerOf = new Map();
  const assignLayer = (id, depth = 0) => {
    if (depth > 8) return;
    const prev = layerOf.get(id) ?? -1;
    if (depth > prev) layerOf.set(id, depth);
    edges.forEach((e) => {
      if (e.from === id && e.kind !== "feedback") assignLayer(e.to, depth + 1);
    });
  };
  nodes.forEach((n) => {
    if ((inDeg.get(n.id) || 0) === 0) assignLayer(n.id, 0);
  });
  nodes.forEach((n, idx) => {
    if (!layerOf.has(n.id)) layerOf.set(n.id, idx);
  });

  const columns = new Map();
  nodes.forEach((n) => {
    const col = layerOf.get(n.id) || 0;
    const arr = columns.get(col) || [];
    arr.push(n);
    columns.set(col, arr);
  });

  const maxCol = Math.max(...Array.from(columns.keys()), 0);
  const pos = new Map();
  const width = 920;
  const height = 430;
  const nodeW = 176;
  const nodeH = 74;

  columns.forEach((colNodes, colIdx) => {
    const x = maxCol === 0 ? width / 2 : 120 + (colIdx / maxCol) * (width - 240);
    colNodes.forEach((n, rowIdx) => {
      const y = ((rowIdx + 1) / (colNodes.length + 1)) * height;
      pos.set(n.id, { x, y });
    });
  });

  const statusColor = (st) => {
    if (st === "running") return "var(--accent)";
    if (st === "merging") return "var(--amber)";
    if (st === "completed") return "var(--green)";
    if (st === "failed") return "var(--red)";
    return "var(--hairline-strong)";
  };

  const edgesSVG = edges
    .map((e) => {
      const p1 = pos.get(e.from);
      const p2 = pos.get(e.to);
      if (!p1 || !p2) return "";
      const fromNode = nodeMap.get(e.from);
      const toNode = nodeMap.get(e.to);
      const isActive = fromNode?.status === "running" || toNode?.status === "running" || toNode?.status === "merging";
      const isFeedback = e.kind === "feedback";
      const x1 = isFeedback ? p1.x : p1.x + nodeW / 2;
      const y1 = isFeedback ? p1.y - nodeH / 2 : p1.y;
      const x2 = isFeedback ? p2.x : p2.x - nodeW / 2;
      const y2 = isFeedback ? p2.y - nodeH / 2 : p2.y;
      const curveY = isFeedback ? Math.min(y1, y2) - 54 : (y1 + y2) / 2;
      const midX = (x1 + x2) / 2;
      const d = isFeedback
        ? `M ${x1} ${y1} Q ${midX} ${curveY} ${x2} ${y2}`
        : `M ${x1} ${y1} C ${midX} ${y1}, ${midX} ${y2}, ${x2} ${y2}`;
      const stroke = isFeedback ? "var(--amber)" : isActive ? "var(--accent)" : "var(--hairline-strong)";
      return `<path d="${d}" fill="none" stroke="${stroke}" stroke-width="${isActive ? 2.2 : 1.5}" class="${isActive ? "svg-edge-active" : ""}" />`;
    })
    .join("");

  const nodesSVG = nodes
    .map((n) => {
      const p = pos.get(n.id);
      const x = p.x - nodeW / 2;
      const y = p.y - nodeH / 2;
      const stroke = statusColor(n.status);
      const isSelected = state.selectedNodeId === n.id;
      const roleName = n.role_id || n.agent_role || "coder";
      const branchTag = n.worktree_branch ? "⎇ worktree" : roleName;
      const modelShort = (n.model_id || roleName || "").slice(0, 22);
      return `
        <g class="svg-node-group" data-fleet-node="${esc(n.id)}" transform="translate(${x}, ${y})">
          <rect width="${nodeW}" height="${nodeH}" rx="12" fill="var(--bg-surface)" stroke="${isSelected ? "var(--accent)" : stroke}" stroke-width="${isSelected ? 2.5 : 1.5}" />
          <circle cx="15" cy="18" r="4.5" fill="${stroke}" />
          <text x="26" y="22" fill="var(--ink-primary)" font-size="12" font-weight="600">${esc((n.label || n.id).slice(0, 18))}</text>
          <text x="14" y="42" fill="var(--ink-secondary)" font-family="var(--font-mono)" font-size="10.5">${esc(modelShort)}</text>
          <text x="14" y="59" fill="var(--ink-tertiary)" font-family="var(--font-mono)" font-size="10">${esc(branchTag)} · ${esc(n.status)}</text>
        </g>
      `;
    })
    .join("");

  svg.innerHTML = edgesSVG + nodesSVG;
}

function renderBlackboard(snap) {
  const countsEl = $("blackboard-counts");
  const tasksEl = $("blackboard-tasks-list");
  const notesEl = $("blackboard-notes-list");
  if (!tasksEl || !notesEl) return;

  const tasks = snap?.tasks || [];
  const notes = snap?.notes || [];
  if (countsEl) countsEl.textContent = tr(`${tasks.length} 任务 · ${notes.length} 笔记`, `${tasks.length} tasks · ${notes.length} notes`);

  tasksEl.innerHTML = tasks
    .map(
      (tk) => `
      <div class="bb-task-row">
        <span><strong>${esc(tk.title || tk.id)}</strong> <span class="tiny-muted mono">(${esc(tk.owner_node_id || tk.owner || tr("待认领", "unclaimed"))})</span></span>
        <span class="status-pill ${esc(tk.status)}">${esc(tk.status)}</span>
      </div>
    `
    )
    .join("");

  notesEl.innerHTML = notes
    .map(
      (nt) => `
      <div class="bb-note-card">
        <div class="mono"><strong>${esc(nt.title || nt.node_id || nt.author || nt.name || tr("笔记", "note"))}</strong></div>
        <div class="tiny-muted">${esc((nt.body || "").slice(0, 220))}</div>
      </div>
    `
    )
    .join("");
}

async function renderSelectedNodeDetails(snap, nodeId, loadDiffAndSession = true) {
  const node = (snap?.nodes || []).find((n) => n.id === nodeId);
  if (!node) return;
  const roleName = node.role_id || node.agent_role || "coder";
  if ($("selected-node-title")) $("selected-node-title").textContent = `${node.label || node.id} (${roleName})`;
  if ($("selected-node-badge")) $("selected-node-badge").textContent = `${node.status} · $${Number(node.cost_usd ?? node.estimated_usd ?? 0).toFixed(4)}`;

  const openSessBtn = $("open-node-session-btn");
  if (openSessBtn) {
    openSessBtn.classList.toggle("hidden", !node.session_id);
    openSessBtn.dataset.nodeSessionId = node.session_id || "";
  }

  const streamText = state.nodeStreamBuffers[nodeId] || node.summary || node.error || "";
  if ($("selected-node-stream")) {
    $("selected-node-stream").textContent = streamText || tr("（等待节点输出或读取持久化会话记录…）", "(Waiting for node stream or loading persisted session…)");
  }

  if (!loadDiffAndSession) return;

  // Also load persisted node session transcript if node.session_id exists and stream buffer is empty
  if (node.session_id && !state.nodeStreamBuffers[nodeId]) {
    try {
      const sessRes = await apiFetch(`/api/sessions/${encodeURIComponent(node.session_id)}`);
      const summaryLines = (sessRes.entries || [])
        .slice(-4)
        .map((e) => {
          const txt = (e.blocks || []).map((b) => b.text || (b.tool_name ? `[${b.tool_name}]` : "")).filter(Boolean).join(" ");
          return `[#${e.seq} ${e.role}] ${txt.slice(0, 240)}`;
        })
        .join("\n");
      if (summaryLines && $("selected-node-stream")) {
        $("selected-node-stream").textContent = summaryLines;
      }
    } catch {}
  }

  try {
    const res = await apiFetch(
      `/api/fleet/missions/${encodeURIComponent(snap.id)}/nodes/${encodeURIComponent(nodeId)}/diff?workspace_id=${encodeURIComponent(snap.workspace_id)}`
    );
    const diffPre = $("selected-node-diff");
    if (diffPre) {
      diffPre.innerHTML = res.diff
        ? renderDiffPreviewHTML(res.diff)
        : escHtml(node.summary || tr("（该节点无 Git 文件差异）", "(Clean worktree / read-only node)"));
    }
  } catch {
    if ($("selected-node-diff")) $("selected-node-diff").textContent = node.summary || node.error || "(No worktree diff)";
  }
}

async function steerSelectedNode() {
  const snap = activeMission();
  const inp = $("node-steer-input");
  const msg = (inp?.value || "").trim();
  if (!snap || !state.selectedNodeId || !msg) return;
  try {
    await apiFetch(
      `/api/fleet/missions/${encodeURIComponent(snap.id)}/nodes/${encodeURIComponent(state.selectedNodeId)}/steer`,
      {
        method: "POST",
        body: { workspace_id: snap.workspace_id, instruction: msg }
      }
    );
    if (inp) inp.value = "";
    showToast(`${tr("已向节点注入 Steer", "Steered node")}: ${state.selectedNodeId}`, "ok");
  } catch (err) {
    notifyError(err, tr("节点 Steer 注入失败", "Node steer failed"));
  }
}

async function deleteActiveMission() {
  const snap = activeMission();
  if (!snap) return;
  try {
    await apiFetch(`/api/fleet/missions/${encodeURIComponent(snap.id)}?workspace_id=${encodeURIComponent(snap.workspace_id)}`, {
      method: "DELETE"
    });
    state.missions = state.missions.filter((m) => m.id !== snap.id);
    state.activeMissionId = state.missions[0]?.id || "";
    renderFleetStudio();
    showToast(tr("已删除编排记录", "Mission deleted"), "ok");
  } catch (err) {
    notifyError(err, tr("删除编排记录失败", "Failed to delete mission"));
  }
}

// ============================================================================
// Slide-Over Always-On Drawer (Inbox · Timers · Deferred · Outbox · Dreamer)
// ============================================================================

async function refreshAlwaysOn() {
  try {
    state.alwaysOn = await apiFetch("/api/alwayson");
    renderAlwaysOn();
    renderAttentionQueue();
  } catch (err) {
    if (state.alwaysOnOpen) {
      notifyError(err, tr("读取常驻中枢失败", "Failed to load Always-On state"));
    }
  }
}

function renderAlwaysOn() {
  const ao = state.alwaysOn || {};
  const inbox = ao.inbox || [];
  const timers = ao.timers || [];
  const deferred = (ao.deferred || []).filter((d) => d.status === "pending");
  const outbox = ao.outbox || [];
  const unreadOutbox = outbox.filter((o) => !o.acked && !o.read);

  const activeItems = inbox.filter((i) => i.status === "queued" || i.status === "running").length + deferred.length + unreadOutbox.length;
  const countEl = $("alwayson-count");
  const dotEl = $("alwayson-dot");
  if (countEl) {
    countEl.textContent = String(activeItems);
    countEl.classList.toggle("hidden", activeItems === 0);
  }
  if (dotEl) {
    dotEl.className = `pulse-dot ${activeItems > 0 ? "live" : "idle"}`;
  }
  if ($("ao-summary-sub")) {
    const qCnt = inbox.filter((i) => i.status === "queued").length;
    const rCnt = inbox.filter((i) => i.status === "running").length;
    $("ao-summary-sub").textContent = tr(
      `${qCnt} 排队 · ${rCnt} 运行中 · ${deferred.length} 待决 · ${timers.length} 定时器`,
      `${qCnt} queued · ${rCnt} running · ${deferred.length} deferred · ${timers.length} timers`
    );
  }

  const host = $("alwayson-drawer-body");
  if (!host) return;

  const timerEdit = state.editingTimerDraft || {
    id: "",
    kind: "cron",
    cron_expr: "0 9 * * 1-5",
    after_seconds: 300,
    note: "",
    enabled: true
  };

  host.innerHTML = `
    <!-- 1. Deferred Approval Cards (Hunk Checkboxes + Remember + Feedback) -->
    <section class="ao-section-card">
      <div class="subhead-row">
        <strong>🛡️ ${tr("挂起延迟审批卡片 (Deferred Cards)", "Deferred Approval Cards")}</strong>
        <span class="pill-count ${deferred.length ? "warn" : ""}">${deferred.length}</span>
      </div>
      <div class="ao-list">
        ${
          deferred.length === 0
            ? `<span class="tiny-muted">${tr("无待处理的无人值守审批卡片。", "No pending deferred cards.")}</span>`
            : deferred
                .map((d) => {
                  const hunks = d.hunks || [];
                  const hunksHTML = hunks
                    .map(
                      (hk) => `
                      <label class="checkbox-row mono" style="font-size:11px;">
                        <input type="checkbox" class="ao-def-hunk-cb" data-def-id="${esc(d.id)}" value="${esc(hk.id)}" checked>
                        <span>${escHtml(hk.file_path || "")} ${escHtml(hk.header || "")}</span>
                      </label>
                    `
                    )
                    .join("");
                  return `
                    <div class="ao-item" data-ao-def-card="${esc(d.id)}">
                      <div class="subhead-row">
                        <strong class="mono">${esc(d.tool_name)}</strong>
                        <span class="meta-chip mono">${esc(d.workspace_id || "")}</span>
                      </div>
                      <div class="tiny-muted">${esc(d.summary || d.reason || "")}</div>
                      ${hunksHTML ? `<div style="display:flex;flex-direction:column;gap:3px;">${hunksHTML}</div>` : ""}
                      <input class="mac-input ao-def-feedback-input" data-def-feedback="${esc(d.id)}" type="text" placeholder="${tr("可选审批反馈说明 (Feedback)…", "Optional feedback note…")}">
                      <div class="subhead-row">
                        <label class="checkbox-row">
                          <input type="checkbox" class="ao-def-remember-cb" data-def-remember="${esc(d.id)}">
                          <span>${tr("记住授权规则", "Remember rule")}</span>
                        </label>
                        <div class="row-actions">
                          <button class="danger-xs-btn" type="button" data-def-resolve="${esc(d.id)}" data-approve="false">${tr("拒绝", "Deny")}</button>
                          ${
                            hunks.length > 0
                              ? `<button class="ghost-xs-btn" type="button" data-def-resolve="${esc(d.id)}" data-approve="selected">${tr("批准勾选块", "Approve Hunks")}</button>`
                              : ""
                          }
                          <button class="primary-xs-btn" type="button" data-def-resolve="${esc(d.id)}" data-approve="true">${tr("全部批准", "Approve")}</button>
                        </div>
                      </div>
                    </div>
                  `;
                })
                .join("")
        }
      </div>
    </section>

    <!-- 2. Cron & Follow-Up Timers (Create / Edit / Toggle / Trigger / Delete) -->
    <section class="ao-section-card">
      <div class="subhead-row">
        <strong>⏰ ${tr("Cron 与自续航定时器 (timers.toml)", "Cron & Follow-Up Timers (timers.toml)")}</strong>
        <div class="chips-wrap">
          <button class="chip-btn mono" type="button" data-ao-cron-chip="*/30 * * * *">*/30 * * * *</button>
          <button class="chip-btn mono" type="button" data-ao-cron-chip="0 9 * * 1-5">0 9 * * 1-5</button>
          <button class="chip-btn mono" type="button" data-ao-cron-chip="0 2 * * *">0 2 * * *</button>
        </div>
      </div>

      <div class="mini-card-form">
        <div class="three-col">
          <label class="cfg-field-label">
            <span>${tr("类型 (Kind)", "Kind")}</span>
            <select id="ao-timer-kind" class="mac-select">
              <option value="cron" ${timerEdit.kind === "cron" ? "selected" : ""}>${tr("cron (周期计划)", "cron")}</option>
              <option value="once" ${timerEdit.kind === "once" ? "selected" : ""}>${tr("once (单次延迟秒数)", "once (delay sec)")}</option>
            </select>
          </label>
          <label class="cfg-field-label">
            <span>${tr("Cron 表达式", "Cron Expression")}</span>
            <input id="ao-timer-cron" class="mac-input mono" type="text" value="${esc(timerEdit.cron_expr || "")}" placeholder="0 9 * * 1-5">
          </label>
          <label class="cfg-field-label">
            <span>${tr("延迟秒数 (once)", "Delay Sec (once)")}</span>
            <input id="ao-timer-delay" class="mac-input mono" type="number" min="5" value="${Number(timerEdit.after_seconds || 300)}">
          </label>
        </div>
        <div class="inline-form">
          <input id="ao-timer-note" class="mac-input" type="text" value="${esc(timerEdit.note || "")}" placeholder="${tr("唤醒后执行的巡检/构建指令说明…", "Wake-up instruction note…")}">
          <button id="ao-save-timer-btn" class="primary-xs-btn" type="button">${timerEdit.id ? tr("保存修改", "Update Timer") : tr("+ 添加定时器", "+ Add Timer")}</button>
          ${timerEdit.id ? `<button id="ao-cancel-timer-edit-btn" class="ghost-xs-btn" type="button">${tr("取消", "Cancel")}</button>` : ""}
        </div>
      </div>

      <div class="ao-list">
        ${
          timers.length === 0
            ? `<span class="tiny-muted">${tr("暂无定时器。", "No timers configured.")}</span>`
            : timers
                .map(
                  (tm) => `
                <div class="ao-item">
                  <div class="subhead-row">
                    <span class="mono"><strong>[${esc(tm.kind)}]</strong> ${esc(tm.cron_expr || tm.fire_at || "")}</span>
                    <div class="row-actions">
                      <button class="ghost-xs-btn" type="button" data-ao-timer-toggle="${esc(tm.id)}" data-enabled="${tm.enabled ? "1" : "0"}">${tm.enabled ? tr("已启用", "Enabled") : tr("已暂停", "Paused")}</button>
                      <button class="ghost-xs-btn" type="button" data-ao-timer-trigger="${esc(tm.id)}">${tr("立即触发", "Trigger")}</button>
                      <button class="ghost-xs-btn" type="button" data-ao-timer-edit="${esc(tm.id)}">${tr("编辑", "Edit")}</button>
                      <button class="danger-xs-btn" type="button" data-ao-timer-del="${esc(tm.id)}">✕</button>
                    </div>
                  </div>
                  <div class="tiny-muted">${esc(tm.note || tm.id)}</div>
                </div>
              `
                )
                .join("")
        }
      </div>
    </section>

    <!-- 3. Webhook Trigger & Sleep-Wake Inbox Feed -->
    <section class="ao-section-card">
      <div class="subhead-row">
        <strong>📥 ${tr("Webhook 唤醒与事件收件箱 (inbox.jsonl)", "Webhook & Sleep-Wake Inbox (inbox.jsonl)")}</strong>
        <span class="meta-chip mono">${inbox.length}</span>
      </div>
      <div class="inline-form">
        <input id="ao-webhook-source" class="mac-input mono" style="max-width:120px;" type="text" value="local" placeholder="source">
        <input id="ao-webhook-text" class="mac-input" type="text" placeholder="${tr("发送本地 Webhook 唤醒事件指令…", "Inject local webhook event text…")}">
        <button id="ao-send-webhook-btn" class="primary-xs-btn" type="button">${tr("投递唤醒", "Send")}</button>
      </div>
      <div class="ao-list">
        ${
          inbox.length === 0
            ? `<span class="tiny-muted">${tr("收件箱空闲休眠中（零轮询 Token 消耗）。", "Inbox idle — zero idle token burn.")}</span>`
            : inbox
                .slice(-20)
                .reverse()
                .map(
                  (i) => `
                <div class="ao-item">
                  <div class="subhead-row">
                    <span class="mono"><strong>${esc(i.source)}</strong></span>
                    <span class="status-pill ${i.status === "done" ? "ok" : i.status === "failed" ? "err" : "running"}">${esc(i.status)}</span>
                  </div>
                  <div class="tiny-muted">${esc(i.payload?.text || i.summary || i.error || i.id || "")}</div>
                </div>
              `
                )
                .join("")
        }
      </div>
    </section>

    <!-- 4. Outbox Notifications + Ack -->
    <section class="ao-section-card">
      <div class="subhead-row">
        <strong>📤 ${tr("异步投递发件箱 (outbox.jsonl)", "Notification Outbox (outbox.jsonl)")}</strong>
        ${
          unreadOutbox.length > 0
            ? `<button id="ao-ack-all-outbox-btn" class="ghost-xs-btn" type="button">${tr("全部确认已读 (Ack All)", "Ack All")}</button>`
            : `<span class="meta-chip mono">${outbox.length}</span>`
        }
      </div>
      <div class="ao-list">
        ${
          outbox.length === 0
            ? `<span class="tiny-muted">${tr("暂无异步通知记录。", "Outbox is empty.")}</span>`
            : outbox
                .slice(-20)
                .reverse()
                .map((o) => {
                  const isAcked = Boolean(o.acked || o.read);
                  return `
                    <div class="ao-item">
                      <div class="subhead-row">
                        <strong>${esc(o.title || tr("系统通知", "Notification"))}</strong>
                        <div class="row-actions">
                          <span class="status-pill ${isAcked ? "" : "ok"}">${isAcked ? tr("已读", "acked") : tr("新通知", "new")}</span>
                          ${!isAcked && o.id ? `<button class="ghost-xs-btn" type="button" data-ao-ack-id="${esc(o.id)}">${tr("已读 (Ack)", "Ack")}</button>` : ""}
                        </div>
                      </div>
                      <div class="tiny-muted">${esc(o.body || "")}</div>
                    </div>
                  `;
                })
                .join("")
        }
      </div>
    </section>
  `;
}

async function resolveDeferredCard(defId, mode) {
  const approved = mode !== "false";
  const selectedHunkIds =
    mode === "selected"
      ? $$(`.ao-def-hunk-cb[data-def-id="${CSS.escape(defId)}"]:checked`).map((c) => c.value)
      : [];
  const remember = Boolean(document.querySelector(`[data-def-remember="${CSS.escape(defId)}"]`)?.checked);
  const feedback = (document.querySelector(`[data-def-feedback="${CSS.escape(defId)}"]`)?.value || "").trim();
  try {
    await apiFetch(`/api/alwayson/deferred/${encodeURIComponent(defId)}/resolve`, {
      method: "POST",
      body: {
        approved,
        decision: approved ? (mode === "selected" ? "approve_hunks" : "approve_all") : "deny",
        selected_hunk_ids: selectedHunkIds,
        remember,
        feedback
      }
    });
    showToast(approved ? tr("已批准挂起任务并恢复执行", "Deferred card approved") : tr("已拒绝挂起任务", "Deferred card denied"), "ok");
    await refreshAlwaysOn();
  } catch (err) {
    notifyError(err, tr("处理延迟审批失败", "Failed to resolve deferred card"));
  }
}

async function submitTimerForm() {
  const kind = $("ao-timer-kind")?.value || "cron";
  const cronExpr = ($("ao-timer-cron")?.value || "").trim();
  const delaySec = Number($("ao-timer-delay")?.value || 300);
  const note = ($("ao-timer-note")?.value || "").trim();
  const editId = state.editingTimerDraft?.id || "";

  if (!state.activeWorkspaceId && state.workspaces[0]?.id) {
    state.activeWorkspaceId = state.workspaces[0].id;
  }
  try {
    if (editId) {
      await apiFetch(`/api/alwayson/timers/${encodeURIComponent(editId)}`, {
        method: "PATCH",
        body: {
          kind,
          cron_expr: kind === "cron" ? cronExpr : "",
          after_seconds: kind === "once" ? delaySec : 0,
          note
        }
      });
      state.editingTimerDraft = null;
      showToast(tr("定时器已更新", "Timer updated"), "ok");
    } else {
      await apiFetch("/api/alwayson/timers", {
        method: "POST",
        body: {
          workspace_id: state.activeWorkspaceId,
          session_id: state.activeSessionId,
          kind,
          cron_expr: kind === "cron" ? cronExpr : "",
          after_seconds: kind === "once" ? delaySec : 0,
          note,
          enabled: true
        }
      });
      showToast(tr("定时器已创建", "Timer created"), "ok");
    }
    await refreshAlwaysOn();
  } catch (err) {
    notifyError(err, tr("保存定时器失败", "Failed to save timer"));
  }
}

async function toggleTimerEnabled(timerId, currentlyEnabled) {
  try {
    await apiFetch(`/api/alwayson/timers/${encodeURIComponent(timerId)}`, {
      method: "PATCH",
      body: { enabled: !currentlyEnabled }
    });
    await refreshAlwaysOn();
  } catch (err) {
    notifyError(err, tr("切换定时器状态失败", "Failed to toggle timer"));
  }
}

async function triggerTimerNow(timerId) {
  try {
    await apiFetch(`/api/alwayson/timers/${encodeURIComponent(timerId)}/trigger`, {
      method: "POST",
      body: {}
    });
    showToast(tr("定时器已立即触发入队", "Timer triggered into Inbox"), "ok");
    await refreshAlwaysOn();
  } catch (err) {
    notifyError(err, tr("触发定时器失败", "Failed to trigger timer"));
  }
}

async function deleteTimerItem(timerId) {
  try {
    await apiFetch(`/api/alwayson/timers/${encodeURIComponent(timerId)}`, { method: "DELETE" });
    if (state.editingTimerDraft?.id === timerId) state.editingTimerDraft = null;
    showToast(tr("定时器已删除", "Timer deleted"), "ok");
    await refreshAlwaysOn();
  } catch (err) {
    notifyError(err, tr("删除定时器失败", "Failed to delete timer"));
  }
}

async function ackOutboxItems(ids) {
  if (!ids || ids.length === 0) return;
  try {
    await apiFetch("/api/alwayson/outbox/ack", { method: "POST", body: { ids } });
    await refreshAlwaysOn();
  } catch (err) {
    notifyError(err, tr("确认发件箱通知失败", "Failed to ack outbox"));
  }
}

async function runDreamNow() {
  const btn = $("trigger-dream-btn");
  if (btn) btn.disabled = true;
  try {
    const res = await apiFetch("/api/alwayson/dream", {
      method: "POST",
      body: { workspace_id: state.activeWorkspaceId }
    });
    const count = (res?.candidate_skills || []).length;
    showToast(`${tr("Dreamer 记忆归纳完成，新增候选技能", "Dreamer pass completed — candidate skills")}: ${count}`, "ok");
    await refreshAlwaysOn();
  } catch (err) {
    notifyError(err, tr("Dreamer 运行失败", "Dreamer pass failed"));
  } finally {
    if (btn) btn.disabled = false;
  }
}

// ============================================================================
// ONE Canonical Settings Center (#config-mode-view · All 9 Editable Pages)
// ============================================================================

async function loadSettingsAuxData(page) {
  if (page === "permissions" && state.activeWorkspaceId) {
    try {
      const res = await apiFetch(`/api/workspaces/${encodeURIComponent(state.activeWorkspaceId)}/grants`);
      state.workspaceGrants = Array.isArray(res) ? res : res?.grants || [];
    } catch {
      state.workspaceGrants = [];
    }
    if (state.settingsOpen && state.configPage === "permissions") renderConfigStudio();
  }
  if (page === "memory") {
    try {
      const mem = await apiFetch("/api/memory");
      state.memoryData = {
        instructions: mem?.instructions || "",
        index: mem?.index || "",
        notes: mem?.notes || {}
      };
      if (!state.editingNoteSlug) {
        state.editingNoteSlug = Object.keys(state.memoryData.notes)[0] || "architecture";
      }
    } catch {
      // Fallback: read INSTRUCTIONS.md & INDEX.md via /api/config/files if /api/memory not yet mounted
      try {
        const [ins, idx] = await Promise.all([
          apiFetch("/api/config/files?rel=memory/INSTRUCTIONS.md").catch(() => ({ content: "" })),
          apiFetch("/api/config/files?rel=memory/INDEX.md").catch(() => ({ content: "" }))
        ]);
        state.memoryData = { instructions: ins.content || "", index: idx.content || "", notes: {} };
      } catch {}
    }
    try {
      const cand = await apiFetch("/api/skills/candidates");
      state.candidateSkills = Array.isArray(cand) ? cand : cand?.candidates || [];
    } catch {
      state.candidateSkills = [];
    }
    if (state.activeWorkspaceId) {
      const slug = state.editingProgressSlug || "current";
      state.editingProgressSlug = slug;
      try {
        const prog = await apiFetch(`/api/workspaces/${encodeURIComponent(state.activeWorkspaceId)}/memory/progress/${encodeURIComponent(slug)}`);
        if (prog && typeof prog.notes === "object") {
          state.workspaceProgress = prog.notes;
        } else if (prog && typeof prog.content === "string") {
          state.workspaceProgress[slug] = prog.content;
        }
      } catch {}
    }
    if (state.settingsOpen && state.configPage === "memory") renderConfigStudio();
  }
}

function renderConfigStudio() {
  const host = $("config-studio-content");
  if (!host || !state.config) return;

  syncSettingsDialogPageTitle();

  switch (state.configPage) {
    case "general":
      host.innerHTML = renderGeneralSettingsPage();
      break;
    case "matrix":
      host.innerHTML = renderMatrixSettingsPage();
      break;
    case "providers":
      host.innerHTML = renderProvidersSettingsPage();
      break;
    case "agents":
      host.innerHTML = renderAgentsSettingsPage();
      break;
    case "workflows":
      host.innerHTML = renderWorkflowsSettingsPage();
      break;
    case "permissions":
      host.innerHTML = renderPermissionsSettingsPage();
      break;
    case "mcp":
      host.innerHTML = renderMCPSettingsPage();
      break;
    case "memory":
      host.innerHTML = renderMemorySettingsPage();
      break;
    default:
      host.innerHTML = renderMatrixSettingsPage();
  }
}

// --- Page 1: General & Appearance ---
function renderGeneralSettingsPage() {
  const g = state.config.global || {};
  const permModes = [
    { v: "auto", l: tr("自动门控 (Auto)", "Auto") },
    { v: "accept_edits", l: tr("接受编辑 (Accept Edits)", "Accept Edits") },
    { v: "ask", l: tr("逐次询问 (Ask)", "Ask") },
    { v: "plan", l: tr("只读规划 (Plan)", "Plan") },
    { v: "yolo", l: tr("全放行 (Yolo)", "Yolo") }
  ];
  const sbModes = [
    { v: "workspace-write", l: tr("仅写工作区 (Seatbelt)", "workspace-write (Seatbelt)") },
    { v: "read-only", l: tr("只读沙箱 (read-only)", "read-only") },
    { v: "off", l: tr("关闭沙箱 (off)", "off") }
  ];
  const themeModes = [
    { v: "auto", l: tr("自动跟随系统 (Auto)", "Auto (System)") },
    { v: "light", l: tr("浅色模式 (Light)", "Light") },
    { v: "dark", l: tr("深色模式 (Dark)", "Dark") }
  ];

  const compactThresh = Number(g.auto_compact_threshold_percent ?? 85);
  const toolConc = Number(g.max_tool_concurrency || 4);
  const subSteps = Number(g.subagent_max_steps || 8);
  const cmdTimeout = Number(g.command_timeout_seconds || 120);
  const autoTitle = g.auto_generate_title !== undefined ? Boolean(g.auto_generate_title) : true;
  const dreamerIdle = Number(g.dreamer_idle_minutes ?? 15);
  const witnessTimeout = Number(g.witness_timeout_seconds || 180);

  return `
    <div class="cfg-section-card">
      <div class="cfg-section-head">
        <div>
          <h3 class="cfg-section-title">🖥️ ${tr("本地守护进程与透明文件仓状态", "Local Daemon & Zero-DB Vault Status")}</h3>
          <p class="cfg-section-desc">${tr("纯本机 127.0.0.1 回环服务，零外部数据库依赖，所有设置即时热加载。", "Loopback-only 127.0.0.1 daemon · zero SQLite/Postgres · instant fsnotify reload.")}</p>
        </div>
        <span class="status-pill ${state.health.connected ? "ok" : "warn"}">${state.health.connected ? tr("SSE 已连接", "SSE Connected") : tr("重连中", "Reconnecting")}</span>
      </div>
      <div class="three-col">
        <div class="ctx-card">
          <span class="tiny-muted">${tr("守护进程版本", "Daemon Version")}</span>
          <strong class="mono">${esc(state.version)}</strong>
        </div>
        <div class="ctx-card">
          <span class="tiny-muted">${tr("零数据库文件仓路径", "Zero-DB Vault Path")}</span>
          <code class="mono" style="word-break:break-all;">${esc(state.vaultRoot)}</code>
        </div>
        <div class="ctx-card">
          <span class="tiny-muted">${tr("活跃运行流 / 已挂载项目", "Active Runs / Workspaces")}</span>
          <strong class="mono">${Object.keys(state.activeRunsBySession).length} / ${state.workspaces.length}</strong>
        </div>
      </div>
    </div>

    <div class="cfg-section-card">
      <div class="cfg-section-head">
        <div>
          <h3 class="cfg-section-title">🎨 ${tr("界面外观与默认安全策略 (config.toml)", "Appearance & Default Security Policy (config.toml)")}</h3>
          <p class="cfg-section-desc">${tr("选择下拉项后立即生效并同步保存至 ~/.gorness/config.toml。", "Select from dropdowns to update immediately and persist via PUT /api/config/global.")}</p>
        </div>
      </div>

      <div class="inset-group">
        <div class="inset-row">
          <div class="settings-row-info">
            <strong>${tr("色彩主题 (Theme)", "Color Theme")}</strong>
            <span>${tr("支持自动跟随 macOS 外观或锁定浅色/深色模式", "Follow macOS system appearance or force Light / Dark")}</span>
          </div>
          <select id="cfg-ui-theme-select" class="mac-select wide" data-cfg-ui-theme-select="1">
            ${themeModes.map((it) => `<option value="${it.v}" ${state.theme === it.v ? "selected" : ""}>${esc(it.l)}</option>`).join("")}
          </select>
        </div>
        <div class="inset-row">
          <div class="settings-row-info">
            <strong>${tr("界面语言 (Language)", "Interface Language")}</strong>
            <span>${tr("实时切换工作台与设置中心界面语言", "Switch UI language immediately")}</span>
          </div>
          <select id="cfg-ui-lang-select" class="mac-select wide" data-cfg-ui-lang-select="1">
            <option value="zh" ${state.lang === "zh" ? "selected" : ""}>简体中文</option>
            <option value="en" ${state.lang === "en" ? "selected" : ""}>English</option>
          </select>
        </div>
        <div class="inset-row">
          <div class="settings-row-info">
            <strong>${tr("默认权限门控 (permission_mode)", "Default Permission Mode")}</strong>
            <span>${tr("新会话默认工具执行与文件编辑门控级别", "Default gate mode for new sessions")}</span>
          </div>
          <select id="cfg-global-perm-select" class="mac-select wide" data-global-select="permission_mode">
            ${permModes.map((m) => `<option value="${m.v}" ${(g.permission_mode || "auto") === m.v ? "selected" : ""}>${esc(m.l)}</option>`).join("")}
          </select>
        </div>
        <div class="inset-row">
          <div class="settings-row-info">
            <strong>${tr("默认 macOS Seatbelt 沙箱 (sandbox_mode)", "Default macOS Seatbelt Sandbox")}</strong>
            <span>${tr("子进程内核级 Seatbelt 写入边界隔离模式", "Kernel-level Seatbelt sandbox boundary")}</span>
          </div>
          <select id="cfg-global-sandbox-select" class="mac-select wide" data-global-select="sandbox_mode">
            ${sbModes.map((m) => `<option value="${m.v}" ${(g.sandbox_mode || "workspace-write") === m.v ? "selected" : ""}>${esc(m.l)}</option>`).join("")}
          </select>
        </div>
      </div>
    </div>

    <div class="cfg-section-card">
      <div class="cfg-section-head">
        <div>
          <h3 class="cfg-section-title">⚙️ ${tr("Harness 引擎与自动化策略", "Harness Engine & Automation Policy")}</h3>
          <p class="cfg-section-desc">${tr("全量极简下拉与开关配置，实时热更新至 PUT /api/config/global。", "Zero-typing dropdowns and switches persisted immediately via PUT /api/config/global.")}</p>
        </div>
      </div>

      <div class="inset-group">
        <div class="inset-row">
          <div class="settings-row-info">
            <strong>${tr("自动上下文压缩阈值 (auto_compact_threshold_percent)", "Auto Compact Threshold")}</strong>
            <span>${tr("当会话上下文占用超过该比例时自动执行锚定增量压缩", "Trigger anchored compaction when context exceeds threshold")}</span>
          </div>
          <select id="cfg-auto-compact-threshold" class="mac-select wide" data-global-num-select="auto_compact_threshold_percent">
            <option value="85" ${compactThresh === 85 ? "selected" : ""}>${tr("85%（推荐）", "85% (Recommended)")}</option>
            <option value="75" ${compactThresh === 75 ? "selected" : ""}>75%</option>
            <option value="80" ${compactThresh === 80 ? "selected" : ""}>80%</option>
            <option value="90" ${compactThresh === 90 ? "selected" : ""}>90%</option>
            <option value="0" ${compactThresh === 0 ? "selected" : ""}>${tr("关闭 (0)", "Off (0)")}</option>
          </select>
        </div>

        <div class="inset-row">
          <div class="settings-row-info">
            <strong>${tr("单轮工具最大并发数 (max_tool_concurrency)", "Max Tool Concurrency")}</strong>
            <span>${tr("单轮响应内只读工具并发执行上限", "Maximum parallel tool calls per turn")}</span>
          </div>
          <select id="cfg-max-tool-concurrency" class="mac-select wide" data-global-num-select="max_tool_concurrency">
            <option value="4" ${toolConc === 4 ? "selected" : ""}>${tr("4（推荐）", "4 (Recommended)")}</option>
            <option value="1" ${toolConc === 1 ? "selected" : ""}>${tr("1（串行）", "1 (Serial)")}</option>
            <option value="2" ${toolConc === 2 ? "selected" : ""}>2</option>
            <option value="8" ${toolConc === 8 ? "selected" : ""}>8</option>
          </select>
        </div>

        <div class="inset-row">
          <div class="settings-row-info">
            <strong>${tr("子智能体最大步数 (subagent_max_steps)", "Subagent Max Steps")}</strong>
            <span>${tr("spawn_subagent 委派子智能体的单次最大推理步数", "Maximum reasoning steps for delegated subagents")}</span>
          </div>
          <select id="cfg-subagent-max-steps" class="mac-select wide" data-global-num-select="subagent_max_steps">
            <option value="8" ${subSteps === 8 ? "selected" : ""}>${tr("8 步（推荐）", "8 steps (Recommended)")}</option>
            <option value="4" ${subSteps === 4 ? "selected" : ""}>${tr("4 步", "4 steps")}</option>
            <option value="12" ${subSteps === 12 ? "selected" : ""}>${tr("12 步", "12 steps")}</option>
            <option value="16" ${subSteps === 16 ? "selected" : ""}>${tr("16 步", "16 steps")}</option>
          </select>
        </div>

        <div class="inset-row">
          <div class="settings-row-info">
            <strong>${tr("命令执行超时 (command_timeout_seconds)", "Command Timeout")}</strong>
            <span>${tr("run_command 单条命令默认最长执行秒数", "Default timeout in seconds for shell commands")}</span>
          </div>
          <select id="cfg-command-timeout" class="mac-select wide" data-global-num-select="command_timeout_seconds">
            <option value="120" ${cmdTimeout === 120 ? "selected" : ""}>${tr("120 秒（推荐）", "120s (Recommended)")}</option>
            <option value="30" ${cmdTimeout === 30 ? "selected" : ""}>${tr("30 秒", "30s")}</option>
            <option value="60" ${cmdTimeout === 60 ? "selected" : ""}>${tr("60 秒", "60s")}</option>
            <option value="300" ${cmdTimeout === 300 ? "selected" : ""}>${tr("300 秒", "300s")}</option>
          </select>
        </div>

        <div class="inset-row">
          <div class="settings-row-info">
            <strong>${tr("自动生成会话标题 (auto_generate_title)", "Auto-Generate Session Title")}</strong>
            <span>${tr("首轮对话完成后利用轻量模型自动提炼简明标题", "Automatically summarize first prompt into session title")}</span>
          </div>
          <select id="cfg-auto-generate-title" class="mac-select wide" data-global-bool-select="auto_generate_title">
            <option value="true" ${autoTitle ? "selected" : ""}>${tr("开启（推荐）", "Enabled (Recommended)")}</option>
            <option value="false" ${!autoTitle ? "selected" : ""}>${tr("关闭", "Disabled")}</option>
          </select>
        </div>

        <div class="inset-row">
          <div class="settings-row-info">
            <strong>${tr("Dreamer 空闲触发时长 (dreamer_idle_minutes)", "Dreamer Idle Trigger")}</strong>
            <span>${tr("工作台空闲达到该时长后自动归纳记忆与候选技能", "Run background memory & skill consolidation after idle")}</span>
          </div>
          <select id="cfg-dreamer-idle-minutes" class="mac-select wide" data-global-num-select="dreamer_idle_minutes">
            <option value="15" ${dreamerIdle === 15 ? "selected" : ""}>${tr("15 分钟（推荐）", "15 min (Recommended)")}</option>
            <option value="5" ${dreamerIdle === 5 ? "selected" : ""}>${tr("5 分钟", "5 min")}</option>
            <option value="30" ${dreamerIdle === 30 ? "selected" : ""}>${tr("30 分钟", "30 min")}</option>
            <option value="0" ${dreamerIdle === 0 ? "selected" : ""}>${tr("关闭 (0)", "Off (0)")}</option>
          </select>
        </div>

        <div class="inset-row">
          <div class="settings-row-info">
            <strong>${tr("Witness 停滞检测超时 (witness_timeout_seconds)", "Witness Stall Timeout")}</strong>
            <span>${tr("舰队协同节点无响应超时告警与自愈检测阈值", "Fleet node stall detection timeout in seconds")}</span>
          </div>
          <select id="cfg-witness-timeout" class="mac-select wide" data-global-num-select="witness_timeout_seconds">
            <option value="180" ${witnessTimeout === 180 ? "selected" : ""}>${tr("180 秒（推荐）", "180s (Recommended)")}</option>
            <option value="60" ${witnessTimeout === 60 ? "selected" : ""}>${tr("60 秒", "60s")}</option>
            <option value="120" ${witnessTimeout === 120 ? "selected" : ""}>${tr("120 秒", "120s")}</option>
            <option value="300" ${witnessTimeout === 300 ? "selected" : ""}>${tr("300 秒", "300s")}</option>
          </select>
        </div>
      </div>

      <div class="three-col" style="margin-top:8px;">
        <label class="cfg-field-label">
          <span>${tr("最大并发运行数 (max_concurrency)", "Max Concurrent Runs")}</span>
          <input id="gen-max-runs" class="mac-input mono" type="number" min="1" max="64" value="${Number(g.max_concurrency || g.max_concurrent_runs || 8)}">
        </label>
        <label class="cfg-field-label">
          <span>${tr("单轮最大步数 (max_steps)", "Default Max Steps")}</span>
          <input id="gen-max-steps" class="mac-input mono" type="number" min="1" max="200" value="${Number(g.max_steps || g.default_max_steps || 32)}">
        </label>
        <label class="cfg-field-label">
          <span>${tr("常驻防抖毫秒 (inbox_debounce_ms)", "Always-On Debounce (ms)")}</span>
          <input id="gen-debounce-ms" class="mac-input mono" type="number" min="0" max="60000" value="${Number(g.inbox_debounce_ms ?? g.always_on_debounce_ms ?? 400)}">
        </label>
      </div>
      <div class="modal-actions">
        <button id="save-general-limits-btn" class="primary-xs-btn" type="button">${tr("保存引擎数值参数", "Save Engine Limits")}</button>
      </div>
    </div>
  `;
}

// --- Page 2: Role-Model Matrix (Sub-tab Categories + 4-Col Single-Row Alignment + Effort Select) ---
function renderMatrixSettingsPage() {
  const g = state.config.global || {};
  const provIds = Object.keys(state.config.providers || {}).sort();
  const defProv = g.default_provider || provIds[0] || "anthropic";
  const defModels = getModelsForProvider(defProv);
  const defModel = g.default_model || defModels[0]?.id || "";
  const allRefs = getAllAvailableModelRefs();
  const effortOpts = [
    { v: "low", l: tr("低 (low)", "Low (low)") },
    { v: "medium", l: tr("中 (medium)", "Medium (medium)") },
    { v: "high", l: tr("高 (high)", "High (high)") },
    { v: "xhigh", l: tr("超高 (xhigh)", "Extra High (xhigh)") },
    { v: "max", l: tr("极高 (max)", "Max (max)") }
  ];

  const schemesHTML = MATRIX_SCHEMES.map(
    (sc) => `
      <button class="scheme-card" type="button" data-apply-scheme="${esc(sc.id)}">
        <div class="scheme-card-top">
          <strong>${esc(state.lang === "zh" ? sc.zh : sc.en)}</strong>
          <span class="status-pill ok">${esc(sc.badge)}</span>
        </div>
        <span>${esc(state.lang === "zh" ? sc.descZh : sc.descEn)}</span>
      </button>
    `
  ).join("");

  const roleOrder = ["lead", "coder", "explorer", "verifier", "utility"];
  const agentKeys = Object.keys(state.config.agents || {}).sort((a, b) => {
    const ia = roleOrder.indexOf(a);
    const ib = roleOrder.indexOf(b);
    if (ia !== -1 && ib !== -1) return ia - ib;
    if (ia !== -1) return -1;
    if (ib !== -1) return 1;
    return a.localeCompare(b);
  });

  const subTabs = [
    { id: "core", label: tr("💬 核心对话角色", "💬 Core Chat Roles"), match: (id) => id === "lead" || id === "coder" },
    { id: "fleet", label: tr("🛸 舰队协同专家", "🛸 Fleet Specialists"), match: (id) => id === "explorer" || id === "verifier" },
    { id: "utility", label: tr("⚡ 轻量辅助与自定义", "⚡ Utility & Custom"), match: (id) => id === "utility" || !["lead", "coder", "explorer", "verifier"].includes(id) },
    { id: "all", label: tr("📋 全部角色一览", "📋 All Roles"), match: () => true }
  ];
  const activeTabObj = subTabs.find((st) => st.id === state.matrixSubTab) || subTabs[0];
  let visibleKeys = agentKeys.filter(activeTabObj.match);
  if (visibleKeys.length === 0) visibleKeys = agentKeys;

  const roleRowsHTML = visibleKeys
    .map((aId) => {
      const ag = state.config.agents[aId] || {};
      const meta = ROLE_META[aId] || { icon: "🤖", zh: ag.name || aId, en: ag.name || aId };
      const curProv = ag.provider || defProv;
      const curProvName = state.config.providers?.[curProv]?.name || curProv;
      const models = getModelsForProvider(curProv);
      const curModel = ag.model || defModel;
      const curEffort = ag.effort || g.default_effort || "high";
      const fallbacks = Array.isArray(ag.fallbacks) ? ag.fallbacks : [];

      const provOpts = provIds.includes(curProv) ? provIds : [curProv, ...provIds];
      const modelList = models.some((m) => m.id === curModel) ? models : [{ id: curModel, name: curModel }, ...models];

      const fbChips = fallbacks
        .map(
          (fb) => `<span class="chip-btn active mono">${esc(fb)} <span class="chip-remove" data-remove-fallback="${esc(aId)}" data-fb="${esc(fb)}">✕</span></span>`
        )
        .join("");

      const addFbOpts = allRefs
        .filter((r) => r.ref !== `${curProv}/${curModel}` && !fallbacks.includes(r.ref))
        .map((r) => `<option value="${esc(r.ref)}">+ ${esc(r.ref)}</option>`)
        .join("");

      return `
        <div class="role-matrix-card">
          <div class="role-card-head">
            <div class="role-title-wrap">
              <div class="role-title-line">
                <span>${meta.icon}</span>
                <strong>${esc(state.lang === "zh" ? meta.zh : meta.en)}</strong>
                <span class="role-id-badge">${esc(aId)}</span>
                ${ag.read_only ? `<span class="status-pill">${tr("只读", "RO")}</span>` : ""}
              </div>
              <div class="row-actions" style="gap:6px;">
                <button class="ghost-xs-btn" type="button" data-jump-edit-agent="${esc(aId)}">${tr("编辑详情 ↗", "Edit ↗")}</button>
                <details style="font-size:11px;">
                  <summary class="tiny-muted" style="cursor:pointer;">${tr("降级链", "Fallbacks")} (${fallbacks.length})</summary>
                  <div class="chips-wrap" style="margin-top:4px;">
                    ${fbChips || `<span class="tiny-muted">${tr("无", "None")}</span>`}
                    <select class="mac-select mono" style="max-width:160px;" data-add-fallback="${esc(aId)}" title="${tr("添加备选降级模型", "Add fallback model")}">
                      <option value="">${tr("+ 备选…", "+ Add…")}</option>
                      ${addFbOpts}
                    </select>
                  </div>
                </details>
              </div>
            </div>
          </div>

          <label class="cfg-field-label">
            <select class="mac-select matrix-select" data-agent-provider="${esc(aId)}" title="${esc(curProvName)}">
              ${provOpts.map((p) => `<option value="${esc(p)}" ${p === curProv ? "selected" : ""}>${esc(state.config.providers?.[p]?.name || p)}</option>`).join("")}
            </select>
          </label>

          <label class="cfg-field-label">
            <select class="mac-select matrix-select mono" data-agent-model-select="${esc(aId)}" title="${esc(curModel)}">
              ${modelList.map((m) => `<option value="${esc(m.id)}" ${m.id === curModel ? "selected" : ""}>${esc(m.name)} (${esc(m.id)})</option>`).join("")}
            </select>
          </label>

          <label class="cfg-field-label">
            <select class="mac-select matrix-select" data-agent-effort-select="${esc(aId)}" title="${esc(curEffort)}">
              ${effortOpts.map((ef) => `<option value="${ef.v}" ${curEffort === ef.v ? "selected" : ""}>${esc(ef.l)}</option>`).join("")}
            </select>
          </label>
        </div>
      `;
    })
    .join("");

  return `
    <div class="cfg-section-card">
      <div class="subhead-row">
        <div class="matrix-subtabs" role="tablist">
          ${subTabs
            .map(
              (st) => `<button class="pill-opt ${state.matrixSubTab === st.id ? "active" : ""}" type="button" data-matrix-subtab="${st.id}">${esc(st.label)}</button>`
            )
            .join("")}
        </div>
      </div>

      <details class="advanced-config-disclosure">
        <summary>⚡ ${tr("一键异构模型组合预设（点击展开）", "1-Click Heterogeneous Matrix Presets")}</summary>
        <div class="scheme-presets-grid" style="margin-top:8px;">${schemesHTML}</div>
      </details>

      <div class="three-col" style="padding-top:8px;border-top:1px solid var(--hairline);">
        <label class="cfg-field-label">
          <span>${tr("全局默认供应商 (default_provider)", "Global Default Provider")}</span>
          <select id="cfg-global-provider" class="mac-select" title="${esc(defProv)}">
            ${provIds.map((p) => `<option value="${esc(p)}" ${p === defProv ? "selected" : ""}>${esc(state.config.providers?.[p]?.name || p)}</option>`).join("")}
          </select>
        </label>
        <label class="cfg-field-label">
          <span>${tr("全局默认模型 (default_model)", "Global Default Model")}</span>
          <select id="cfg-global-model" class="mac-select mono" title="${esc(defModel)}">
            ${(defModels.some((m) => m.id === defModel) ? defModels : [{ id: defModel, name: defModel }, ...defModels])
              .map((m) => `<option value="${esc(m.id)}" ${m.id === defModel ? "selected" : ""}>${esc(m.name)} (${esc(m.id)})</option>`)
              .join("")}
          </select>
        </label>
        <label class="cfg-field-label">
          <span>${tr("全局默认思考强度 (default_effort)", "Global Default Effort")}</span>
          <select id="cfg-global-effort" class="mac-select" title="${esc(g.default_effort || "high")}">
            ${effortOpts
              .map((ef) => `<option value="${ef.v}" ${(g.default_effort || "high") === ef.v ? "selected" : ""}>${esc(ef.l)}</option>`)
              .join("")}
          </select>
        </label>
      </div>
    </div>

    <div class="matrix-table">
      <div class="matrix-header-row">
        <span>${tr("角色名称与职责", "Role & Responsibility")}</span>
        <span>${tr("模型供应商", "Provider")}</span>
        <span>${tr("绑定模型", "Bound Model")}</span>
        <span>${tr("思考强度", "Reasoning Effort")}</span>
      </div>
      ${roleRowsHTML}
    </div>
  `;
}

// --- Page 3: Providers & macOS Keychain ---
function ensureEditingProviderDraft() {
  const provs = state.config?.providers || {};
  const ids = Object.keys(provs).sort();
  if (!state.editingProviderId && ids.length > 0 && state.editingProviderId !== "__new__") {
    state.editingProviderId = ids[0];
  }
  if (!state.editingProviderDraft) {
    if (state.editingProviderId && state.editingProviderId !== "__new__" && provs[state.editingProviderId]) {
      const src = provs[state.editingProviderId];
      state.editingProviderDraft = {
        id: state.editingProviderId,
        name: src.name || state.editingProviderId,
        protocol: src.protocol || "openai-compat",
        base_url: src.base_url || "",
        api_key: src.api_key || "",
        timeout_sec: Number(src.timeout_sec || 180),
        headers: Object.assign({}, src.headers || {}),
        models: JSON.parse(JSON.stringify(src.models || {}))
      };
    } else {
      state.editingProviderDraft = {
        id: "",
        name: "",
        protocol: "openai-compat",
        base_url: "https://api.example.com/v1",
        api_key: "keychain:CUSTOM_API_KEY",
        timeout_sec: 180,
        headers: {},
        models: {}
      };
    }
  }
  return state.editingProviderDraft;
}

function renderProvidersSettingsPage() {
  const provs = state.config.providers || {};
  const provIds = Object.keys(provs).sort();
  const draft = ensureEditingProviderDraft();

  const headersLines = Object.entries(draft.headers || {})
    .map(([k, v]) => `${k}: ${v}`)
    .join("\n");

  const modelEntries = Object.entries(draft.models || {});
  const modelsTableRows = modelEntries
    .map(
      ([mId, mSpec]) => `
      <tr>
        <td class="mono"><strong>${esc(mId)}</strong></td>
        <td>${esc(mSpec?.name || mId)}</td>
        <td class="mono">${Number(mSpec?.context_window || 128000)}</td>
        <td class="mono">$${Number(mSpec?.input_cost_per_1m || 0)} / $${Number(mSpec?.output_cost_per_1m || 0)}</td>
        <td><button class="danger-xs-btn" type="button" data-del-draft-model="${esc(mId)}">✕</button></td>
      </tr>
    `
    )
    .join("");

  return `
    <!-- Section A: macOS Security Keychain Secrets -->
    <div class="cfg-section-card">
      <div class="cfg-section-head">
        <div>
          <h3 class="cfg-section-title">🔐 ${tr("macOS 系统钥匙串密钥管理 (keychain:*)", "macOS Security Keychain Secrets (keychain:*)")}</h3>
          <p class="cfg-section-desc">${tr("密钥直接存入 macOS 系统钥匙串，绝不明文落盘到 providers.toml；供应商配置通过 keychain:KEY_NAME 引用。", "Stored in macOS Keychain via POST/DELETE /api/config/secrets — never written as plaintext in TOML.")}</p>
        </div>
      </div>
      <div class="chips-wrap">
        <span class="tiny-muted">${tr("快捷填充键名：", "Quick key names:")}</span>
        ${["ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "DEEPSEEK_API_KEY", "OPENROUTER_API_KEY"]
          .map((k) => `<button class="chip-btn mono" type="button" data-fill-keychain-key="${k}">${k}</button>`)
          .join("")}
      </div>
      <div class="three-col">
        <label class="cfg-field-label">
          <span>${tr("钥匙串键名 (Key Name)", "Keychain Key Name")}</span>
          <input id="keychain-key-input" class="mac-input mono" type="text" value="ANTHROPIC_API_KEY" placeholder="ANTHROPIC_API_KEY">
        </label>
        <label class="cfg-field-label">
          <span>${tr("API 密钥明文 (仅提交至系统钥匙串)", "Secret Value")}</span>
          <input id="keychain-val-input" class="mac-input mono" type="password" placeholder="sk-...">
        </label>
        <div class="cfg-field-label" style="justify-content:flex-end;">
          <div class="row-actions">
            <button id="save-keychain-secret-btn" class="primary-sm-btn" type="button">${tr("存入钥匙串", "Save Secret")}</button>
            <button id="delete-keychain-secret-btn" class="danger-xs-btn" type="button">${tr("删除密钥", "Delete")}</button>
          </div>
        </div>
      </div>
    </div>

    <!-- Section B: Quick-Fill Provider Templates + Editable Provider Form -->
    <div class="cfg-section-card">
      <div class="cfg-section-head">
        <div>
          <h3 class="cfg-section-title">🔌 ${tr("模型供应商目录与完整自定义编辑 (providers.toml)", "Model Providers & Custom Gateway Editor (providers.toml)")}</h3>
          <p class="cfg-section-desc">${tr("点选预置模板一键注册，或在下方自由创建/编辑任意网关 ID、协议、Base URL、自定义请求头与模型目录。", "1-click register presets or edit any custom provider ID, protocol, Base URL, headers & models.")}</p>
        </div>
        <button id="new-provider-draft-btn" class="primary-xs-btn" type="button">${tr("+ 新建自定义供应商", "+ New Custom Provider")}</button>
      </div>

      <div class="chips-wrap">
        <span class="tiny-muted">${tr("一键注册模板：", "1-click add preset:")}</span>
        ${Object.values(PROVIDER_PRESETS)
          .map(
            (pr) => `<button class="chip-btn ${provs[pr.id] ? "active" : ""}" type="button" data-apply-prov-preset="${esc(pr.id)}">${provs[pr.id] ? "✓ " : "+ "}${esc(pr.name)}</button>`
          )
          .join("")}
      </div>

      <div class="pill-bar">
        ${provIds
          .map(
            (id) => `<button class="pill-opt ${state.editingProviderId === id ? "active" : ""}" type="button" data-select-edit-prov="${esc(id)}">${esc(provs[id].name || id)} (${esc(id)})</button>`
          )
          .join("")}
        ${state.editingProviderId === "__new__" ? `<button class="pill-opt active" type="button">${tr("✨ 新建供应商", "✨ New Provider")}</button>` : ""}
      </div>

      <div class="three-col">
        <label class="cfg-field-label">
          <span>${tr("供应商 ID (唯一标识)", "Provider ID")}</span>
          <input id="prov-edit-id" class="mac-input mono" type="text" value="${esc(draft.id)}" placeholder="my-gateway">
        </label>
        <label class="cfg-field-label">
          <span>${tr("显示名称 (Name)", "Display Name")}</span>
          <input id="prov-edit-name" class="mac-input" type="text" value="${esc(draft.name)}" placeholder="My Internal Gateway">
        </label>
        <label class="cfg-field-label">
          <span>${tr("传输协议 (Protocol)", "Wire Protocol")}</span>
          <select id="prov-edit-protocol" class="mac-select mono">
            ${["anthropic", "openai-compat", "gemini"]
              .map((pt) => `<option value="${pt}" ${draft.protocol === pt ? "selected" : ""}>${pt}</option>`)
              .join("")}
          </select>
        </label>
      </div>

      <div class="two-col">
        <label class="cfg-field-label">
          <span>Base URL</span>
          <input id="prov-edit-baseurl" class="mac-input mono" type="text" value="${esc(draft.base_url)}" placeholder="https://api.openai.com/v1">
        </label>
        <label class="cfg-field-label">
          <span>${tr("API Key (支持输入明文 sk-... 自动存入钥匙串，或 keychain:KEY / env:VAR)", "API Key (type raw key or keychain:KEY / env:VAR)")}</span>
          <div class="inline-form" style="flex-wrap:nowrap;">
            <input id="prov-edit-apikey" class="mac-input mono" type="text" value="${esc(draft.api_key)}" placeholder="keychain:OPENAI_API_KEY 或 sk-...">
            <button id="fetch-provider-models-btn" class="primary-xs-btn" type="button" data-fetch-provider-models="${esc(draft.id || "custom")}" style="flex-shrink:0;">🔄 ${tr("获取模型列表", "Fetch Models")}</button>
          </div>
        </label>
      </div>

      ${
        state.fetchedProviderModels && state.fetchedProviderModels.length > 0
          ? (() => {
              const fq = (state.fetchedModelsFilter || "").trim().toLowerCase();
              const visibleFetched = state.fetchedProviderModels.filter(
                (m) => !fq || String(m.id).toLowerCase().includes(fq) || String(m.name || "").toLowerCase().includes(fq)
              );
              return `
                <div class="fetched-models-panel" id="fetched-models-panel">
                  <div class="subhead-row">
                    <div>
                      <strong>📋 ${tr("选择要保留到模型目录的模型", "Select Models to Keep in Catalog")}</strong>
                      <span class="meta-chip mono">${state.selectedFetchedModels.size} / ${state.fetchedProviderModels.length}</span>
                      ${state.fetchedModelsSource ? `<span class="status-pill ok mono">${esc(state.fetchedModelsSource)}</span>` : ""}
                    </div>
                    <div class="row-actions">
                      <input id="fetched-models-filter" class="mac-input mono" style="max-width:170px;" type="search" value="${esc(state.fetchedModelsFilter || "")}" placeholder="${tr("搜索模型 ID…", "Filter models…")}">
                      <button id="select-all-fetched-models-btn" class="ghost-xs-btn" type="button">${tr("全选", "Select All")}</button>
                      <button id="clear-all-fetched-models-btn" class="ghost-xs-btn" type="button">${tr("清空", "Clear")}</button>
                      <button id="confirm-save-catalog-models-btn" class="primary-xs-btn" type="button">✓ ${tr("确认选入模型目录并保存", "Save Selected to Catalog")}</button>
                    </div>
                  </div>
                  ${state.fetchedModelsWarning ? `<div class="tiny-muted">${esc(state.fetchedModelsWarning)}</div>` : ""}
                  <div class="fetched-models-grid">
                    ${visibleFetched
                      .map((m) => {
                        const checked = state.selectedFetchedModels.has(m.id);
                        return `
                          <label class="fetched-model-item ${checked ? "selected" : ""}">
                            <div class="checkbox-row mono" style="min-width:0;flex:1;">
                              <input type="checkbox" data-toggle-fetched-model="${esc(m.id)}" ${checked ? "checked" : ""}>
                              <span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" title="${esc(m.id)}">${esc(m.name || m.id)} <small class="tiny-muted">(${esc(m.id)})</small></span>
                            </div>
                            <span class="meta-chip mono">${Math.round(Number(m.context_window || 128000) / 1000)}k</span>
                          </label>
                        `;
                      })
                      .join("")}
                  </div>
                </div>
              `;
            })()
          : ""
      }

      <details class="advanced-config-disclosure">
        <summary>${tr("高级连接选项：超时与自定义 HTTP 请求头 (可选)", "Advanced: Timeout & Custom HTTP Headers (Optional)")}</summary>
        <div class="advanced-config-body">
          <div class="two-col">
            <label class="cfg-field-label">
              <span>${tr("超时秒数 (timeout_sec)", "Timeout (sec)")}</span>
              <input id="prov-edit-timeout" class="mac-input mono" type="number" min="5" max="1800" value="${Number(draft.timeout_sec || 180)}">
            </label>
            <label class="cfg-field-label">
              <span>${tr("自定义请求头 (Key: Value)", "Custom Headers (Key: Value)")}</span>
              <input id="prov-edit-headers" class="mac-input mono" type="text" value="${esc(Object.entries(draft.headers || {}).map(([k, v]) => `${k}: ${v}`).join("; "))}" placeholder="X-Team: platform; X-Env: prod">
            </label>
          </div>
        </div>
      </details>

      <div class="cfg-field-label">
        <span>📚 ${tr("已启用模型目录 (models — 全局下拉框仅展示此处保留的模型)", "Enabled Model Catalog (models)")}</span>
        <div class="catalog-model-pills" style="margin-bottom:6px;">
          ${
            modelEntries.length > 0
              ? modelEntries
                  .map(
                    ([mId, mSpec]) =>
                      `<span class="chip-btn active mono">${esc(mSpec?.name || mId)} <small>(${esc(mId)})</small> <span class="chip-remove" data-del-draft-model="${esc(mId)}" title="${tr("从模型目录移除", "Remove from catalog")}">×</span></span>`
                  )
                  .join("")
              : `<span class="tiny-muted">${tr("当前供应商模型目录为空，可点击上方「🔄 获取模型列表」在线拉取勾选，或在下方手动添加。", "Catalog empty — click 'Fetch Models' above or add a model ID below.")}</span>`
          }
        </div>
        <table class="editable-table">
          <thead>
            <tr>
              <th>${tr("模型 ID (Model ID)", "Model ID")}</th>
              <th>${tr("名称", "Name")}</th>
              <th>${tr("上下文窗口 (Context)", "Context")}</th>
              <th>${tr("每百万 Token 价格 (入/出 USD)", "USD / 1M (in/out)")}</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            ${modelsTableRows || `<tr><td colspan="5" class="tiny-muted">${tr("暂无模型，请在下方输入模型 ID 添加", "No models yet — add one below")}</td></tr>`}
          </tbody>
        </table>
        <div class="inline-form" style="margin-top:6px;">
          <input id="prov-new-model-id" class="mac-input mono" type="text" placeholder="model-id (e.g. qwen3-coder)">
          <input id="prov-new-model-name" class="mac-input" type="text" placeholder="${tr("显示名称", "Display Name")}">
          <input id="prov-new-model-ctx" class="mac-input mono" style="max-width:110px;" type="number" value="128000" placeholder="Context">
          <input id="prov-new-model-in" class="mac-input mono" style="max-width:85px;" type="number" step="0.01" value="0" placeholder="In $">
          <input id="prov-new-model-out" class="mac-input mono" style="max-width:85px;" type="number" step="0.01" value="0" placeholder="Out $">
          <button id="add-draft-model-btn" class="ghost-xs-btn" type="button">${tr("+ 手动添加模型 ID", "+ Add Model ID")}</button>
        </div>
      </div>

      <div class="subhead-row" style="padding-top:6px;border-top:1px solid var(--hairline);">
        ${
          draft.id && provs[draft.id]
            ? `<button id="delete-provider-btn" class="danger-xs-btn" type="button" data-del-prov="${esc(draft.id)}">${tr("🗑️ 删除此供应商", "🗑️ Delete Provider")}</button>`
            : `<span></span>`
        }
        <button id="save-provider-form-btn" class="primary-sm-btn" type="button">${tr("保存供应商配置", "Save Provider")}</button>
      </div>
    </div>
  `;
}

// --- Page 4: Agent Definitions (Full Editable Prompt, Tools, Model & Delete) ---
function ensureEditingAgentDraft() {
  const agents = state.config?.agents || {};
  const ids = Object.keys(agents).sort();
  if (!state.editingAgentId && ids.length > 0 && state.editingAgentId !== "__new__") {
    state.editingAgentId = ids.includes("coder") ? "coder" : ids[0];
  }
  if (!state.editingAgentDraft) {
    if (state.editingAgentId && state.editingAgentId !== "__new__" && agents[state.editingAgentId]) {
      const a = agents[state.editingAgentId];
      state.editingAgentDraft = {
        id: state.editingAgentId,
        name: a.name || state.editingAgentId,
        description: a.description || "",
        provider: a.provider || state.config?.global?.default_provider || "anthropic",
        model: a.model || state.config?.global?.default_model || "",
        effort: a.effort || "high",
        max_steps: Number(a.max_steps || 0),
        read_only: Boolean(a.read_only),
        allowed_tools: Array.isArray(a.allowed_tools) ? [...a.allowed_tools] : [],
        disallowed_tools: Array.isArray(a.disallowed_tools) ? [...a.disallowed_tools] : [],
        fallbacks: Array.isArray(a.fallbacks) ? [...a.fallbacks] : [],
        system_prompt: a.system_prompt || ""
      };
    } else {
      state.editingAgentDraft = {
        id: "",
        name: "",
        description: "",
        provider: state.config?.global?.default_provider || "anthropic",
        model: state.config?.global?.default_model || "",
        effort: "high",
        max_steps: 28,
        read_only: false,
        allowed_tools: ["read_file", "list_files", "grep_search"],
        disallowed_tools: [],
        fallbacks: [],
        system_prompt: "You are a focused specialist agent."
      };
    }
  }
  return state.editingAgentDraft;
}

function renderAgentsSettingsPage() {
  const agents = state.config.agents || {};
  const agentIds = Object.keys(agents).sort();
  const provIds = Object.keys(state.config.providers || {}).sort();
  const draft = ensureEditingAgentDraft();
  const modelsForProv = getModelsForProvider(draft.provider);
  const agentModelList = modelsForProv.some((m) => m.id === draft.model)
    ? modelsForProv
    : draft.model
      ? [{ id: draft.model, name: draft.model }, ...modelsForProv]
      : modelsForProv;

  return `
    <div class="cfg-section-card">
      <div class="cfg-section-head">
        <div>
          <h3 class="cfg-section-title">🤖 ${tr("智能体专家定义 (agents/*.md — Frontmatter + System Prompt)", "Agent Definitions (agents/*.md)")}</h3>
          <p class="cfg-section-desc">${tr("每个智能体保存为 ~/.gorness/agents/<id>.md，支持自由编辑角色 ID、从已启用模型目录下拉选择模型、只读约束、工具白/黑名单与完整 System Prompt。", "Persisted in ~/.gorness/agents/<id>.md via POST/DELETE /api/config/agents.")}</p>
        </div>
        <button id="new-agent-draft-btn" class="primary-xs-btn" type="button">${tr("+ 新建智能体", "+ New Agent")}</button>
      </div>

      <div class="chips-wrap">
        <span class="tiny-muted">${tr("快捷专家模板填充：", "Quick-fill specialist template:")}</span>
        ${SPECIALIST_ROLE_PRESETS.map(
          (sp) => `<button class="chip-btn" type="button" data-fill-agent-preset="${esc(sp.id)}">✨ ${esc(sp.name)}</button>`
        ).join("")}
      </div>

      <div class="pill-bar">
        ${agentIds
          .map(
            (id) => `<button class="pill-opt ${state.editingAgentId === id ? "active" : ""}" type="button" data-select-edit-agent="${esc(id)}">${esc(agents[id].name || id)} (${esc(id)})</button>`
          )
          .join("")}
        ${state.editingAgentId === "__new__" ? `<button class="pill-opt active" type="button">${tr("✨ 新建专家", "✨ New Agent")}</button>` : ""}
      </div>

      <div class="three-col">
        <label class="cfg-field-label">
          <span>${tr("智能体 ID (agents/<id>.md)", "Agent ID")}</span>
          <input id="ag-edit-id" class="mac-input mono" type="text" value="${esc(draft.id)}" placeholder="security-auditor">
        </label>
        <label class="cfg-field-label">
          <span>${tr("显示名称 (Name)", "Display Name")}</span>
          <input id="ag-edit-name" class="mac-input" type="text" value="${esc(draft.name)}" placeholder="Security Auditor">
        </label>
        <label class="cfg-field-label">
          <span>${tr("职责简述 (Description)", "Description")}</span>
          <input id="ag-edit-desc" class="mac-input" type="text" value="${esc(draft.description)}" placeholder="Read-only security reviewer">
        </label>
      </div>

      <div class="three-col">
        <label class="cfg-field-label">
          <span>${tr("供应商 (Provider)", "Provider")}</span>
          <select id="ag-edit-provider" class="mac-select">
            ${provIds.map((p) => `<option value="${esc(p)}" ${p === draft.provider ? "selected" : ""}>${esc(state.config.providers?.[p]?.name || p)} (${esc(p)})</option>`).join("")}
          </select>
        </label>
        <label class="cfg-field-label">
          <span>${tr("绑定模型 (从已启用模型目录下拉选择)", "Model (select from enabled catalog)")}</span>
          <select id="ag-edit-model" class="mac-select mono">
            ${agentModelList.map((m) => `<option value="${esc(m.id)}" ${m.id === draft.model ? "selected" : ""}>${esc(m.name)} (${esc(m.id)})</option>`).join("")}
          </select>
        </label>
        <label class="cfg-field-label">
          <span>${tr("推理强度 / 最大步数", "Effort & Max Steps")}</span>
          <div class="inline-form">
            <select id="ag-edit-effort" class="mac-select">
              ${["low", "medium", "high", "xhigh", "max"].map((ef) => `<option value="${ef}" ${draft.effort === ef ? "selected" : ""}>${ef}</option>`).join("")}
            </select>
            <input id="ag-edit-maxsteps" class="mac-input mono" style="max-width:84px;" type="number" min="0" max="200" value="${Number(draft.max_steps || 0)}" title="max_steps (0=default)">
          </div>
        </label>
      </div>

      <label class="checkbox-row">
        <input id="ag-edit-readonly" type="checkbox" ${draft.read_only ? "checked" : ""}>
        <span>${tr("强制只读模式 (read_only = true，禁止写入文件或执行非只读变更)", "Enforce read_only = true")}</span>
      </label>

      <div class="cfg-field-label">
        <span>${tr("允许工具集 (点选芯片切换，留空代表继承角色默认权限；也可在输入框逗号分隔补充)", "Allowed Tools (click chips or edit comma-separated list)")}</span>
        <div class="chips-wrap">
          ${ALL_BUILTIN_TOOLS.map(
            (tl) => `<button class="chip-btn mono ${draft.allowed_tools.includes(tl) ? "active" : ""}" type="button" data-ag-toggle-tool="${tl}">${tl}</button>`
          ).join("")}
        </div>
        <input id="ag-edit-allow-tools" class="mac-input mono" type="text" value="${esc(draft.allowed_tools.join(", "))}" placeholder="read_file, grep_search, list_files">
      </div>

      <div class="two-col">
        <label class="cfg-field-label">
          <span>${tr("禁用工具黑名单 (disallowed_tools，逗号分隔)", "Disallowed Tools (comma-separated)")}</span>
          <input id="ag-edit-disallow-tools" class="mac-input mono" type="text" value="${esc(draft.disallowed_tools.join(", "))}" placeholder="write_file, edit_file">
        </label>
        <label class="cfg-field-label">
          <span>${tr("降级备选链 (fallbacks，逗号分隔 provider/model)", "Fallbacks (comma-separated provider/model)")}</span>
          <input id="ag-edit-fallbacks" class="mac-input mono" type="text" value="${esc(draft.fallbacks.join(", "))}" placeholder="openai/gpt-5.4, gemini/gemini-3.1-pro">
        </label>
      </div>

      <label class="cfg-field-label">
        <span>${tr("系统提示词正文 (System Prompt Markdown Body)", "System Prompt Markdown Body")}</span>
        <textarea id="ag-edit-sysprompt" class="vault-editor mono" rows="8" placeholder="You are...">${esc(draft.system_prompt)}</textarea>
      </label>

      <div class="subhead-row" style="padding-top:6px;border-top:1px solid var(--hairline);">
        ${
          draft.id && agents[draft.id]
            ? `<button id="delete-agent-def-btn" class="danger-xs-btn" type="button" data-del-agent="${esc(draft.id)}">${tr("🗑️ 删除此智能体", "🗑️ Delete Agent")}</button>`
            : `<span></span>`
        }
        <button id="save-agent-def-btn" class="primary-sm-btn" type="button">${tr("保存智能体定义", "Save Agent Definition")}</button>
      </div>
    </div>
  `;
}

// --- Page 5: Workflows (Editable ID, Topology, Verify Command, Nodes, Per-Node role_models[nd.id], Edges & Delete) ---
function ensureEditingWorkflowDraft() {
  const workflows = state.config?.workflows || {};
  const ids = Object.keys(workflows).sort();
  if (!state.editingWorkflowId && ids.length > 0 && state.editingWorkflowId !== "__new__") {
    state.editingWorkflowId = ids.includes("hybrid-fleet") ? "hybrid-fleet" : ids[0];
  }
  if (!state.editingWorkflowDraft) {
    if (state.editingWorkflowId && state.editingWorkflowId !== "__new__" && workflows[state.editingWorkflowId]) {
      const wf = workflows[state.editingWorkflowId];
      state.editingWorkflowDraft = {
        id: wf.id || state.editingWorkflowId,
        name: wf.name || state.editingWorkflowId,
        topology: wf.topology || "hybrid-fleet",
        verify_command: wf.verify_command || "",
        max_concurrency: Number(wf.max_concurrency || 3),
        max_rounds: Number(wf.max_rounds ?? wf.max_review_rounds ?? 2),
        budget_usd: Number(wf.budget_usd || 0),
        use_worktrees: wf.use_worktrees !== false,
        role_models: Object.assign({}, wf.role_models || {}),
        nodes: JSON.parse(JSON.stringify(wf.nodes || [])).map((nd) => ({
          id: nd.id || "",
          label: nd.label || "",
          role: nd.role || nd.agent_role || "coder",
          worktree: Boolean(nd.worktree)
        })),
        edges: JSON.parse(JSON.stringify(wf.edges || []))
      };
    } else {
      state.editingWorkflowDraft = {
        id: "",
        name: "",
        topology: "hybrid-fleet",
        verify_command: "go test ./...",
        max_concurrency: 3,
        max_rounds: 2,
        budget_usd: 0,
        use_worktrees: true,
        role_models: {},
        nodes: [
          { id: "scout", label: "Explorer Scout", role: "explorer", worktree: false },
          { id: "coder_a", label: "Worktree Coder", role: "coder", worktree: true },
          { id: "reviewer", label: "Blind Verifier", role: "verifier", worktree: false }
        ],
        edges: [
          { from: "scout", to: "coder_a", kind: "seq" },
          { from: "coder_a", to: "reviewer", kind: "seq" },
          { from: "reviewer", to: "coder_a", kind: "feedback" }
        ]
      };
    }
  }
  return state.editingWorkflowDraft;
}

function buildStandardTopologyTemplate(topology) {
  switch (topology) {
    case "review-loop":
      return {
        nodes: [
          { id: "coder_a", label: "编码实现阶段", role: "coder", worktree: true },
          { id: "reviewer", label: "对抗盲审阶段", role: "verifier", worktree: false }
        ],
        edges: [
          { from: "coder_a", to: "reviewer", kind: "seq" },
          { from: "reviewer", to: "coder_a", kind: "feedback" }
        ]
      };
    case "worktree-team":
      return {
        nodes: [
          { id: "lead_plan", label: "架构拆解阶段", role: "lead", worktree: false },
          { id: "coder_a", label: "并行工作树研发 A", role: "coder", worktree: true },
          { id: "coder_b", label: "并行工作树研发 B", role: "coder", worktree: true }
        ],
        edges: [
          { from: "lead_plan", to: "coder_a", kind: "seq" },
          { from: "lead_plan", to: "coder_b", kind: "seq" }
        ]
      };
    case "parallel-explore":
      return {
        nodes: [
          { id: "scout_a", label: "并发侦察路 A", role: "explorer", worktree: false },
          { id: "scout_b", label: "并发侦察路 B", role: "explorer", worktree: false },
          { id: "synthesizer", label: "主控综合决策", role: "lead", worktree: false }
        ],
        edges: [
          { from: "scout_a", to: "synthesizer", kind: "seq" },
          { from: "scout_b", to: "synthesizer", kind: "seq" }
        ]
      };
    case "event-pipeline":
      return {
        nodes: [
          { id: "stage_explore", label: "第一阶段：探索分析", role: "explorer", worktree: false },
          { id: "stage_code", label: "第二阶段：核心编码", role: "coder", worktree: true },
          { id: "stage_verify", label: "第三阶段：盲审验收", role: "verifier", worktree: false }
        ],
        edges: [
          { from: "stage_explore", to: "stage_code", kind: "seq" },
          { from: "stage_code", to: "stage_verify", kind: "seq" }
        ]
      };
    case "shared-board":
      return {
        nodes: [
          { id: "coordinator", label: "黑板协调主控", role: "lead", worktree: false },
          { id: "worker_coder", label: "黑板研发专家", role: "coder", worktree: true },
          { id: "worker_verifier", label: "黑板审计专家", role: "verifier", worktree: false }
        ],
        edges: [
          { from: "coordinator", to: "worker_coder", kind: "seq" },
          { from: "worker_coder", to: "worker_verifier", kind: "seq" }
        ]
      };
    default:
      return {
        nodes: [
          { id: "scout", label: "探索阶段", role: "explorer", worktree: false },
          { id: "coder_a", label: "编码阶段", role: "coder", worktree: true },
          { id: "reviewer", label: "审查阶段", role: "verifier", worktree: false }
        ],
        edges: [
          { from: "scout", to: "coder_a", kind: "seq" },
          { from: "coder_a", to: "reviewer", kind: "seq" },
          { from: "reviewer", to: "coder_a", kind: "feedback" }
        ]
      };
  }
}

function formatTopologyFlowPreview(topology, conc, rounds) {
  const c = Number(conc || 3);
  const r = Number(rounds || 2);
  switch (topology) {
    case "review-loop":
      return `💻 ${tr("主控编码实现", "Coder Implementation")} ➔ 🔍 ${tr("异构盲审验收", "Blind Review")} (↺ ${tr("最多", "max")} ${r} ${tr("轮", "rounds")})`;
    case "worktree-team":
      return `🧭 ${tr("任务拆解", "Task Split")} ➔ 🌿 ${tr("并行编码", "Worktree Coders")} ×${c} ➔ 🏁 ${tr("Refinery 变基合入", "Refinery Merge")}`;
    case "parallel-explore":
      return `🔭 ${tr("多路并行探索", "Parallel Scouts")} ×${c} ➔ 🧭 ${tr("主控汇总决策", "Lead Synthesis")}`;
    case "event-pipeline":
      return `🧭 ${tr("探索分析", "Explore")} ➔ 💻 ${tr("串行编码", "Pipeline Code")} ➔ 🔍 ${tr("终审门控", "Verify Gate")}`;
    case "shared-board":
      return `📋 ${tr("共享黑板派单", "Shared Blackboard")} ➔ 💻 ${tr("多专家协同", "Specialists")} ×${c} ➔ 🔍 ${tr("汇总验收", "Final Review")}`;
    default:
      return `🧭 ${tr("探索", "Explore")} ➔ 💻 ${tr("并行编码", "Parallel Code")} ×${c} ➔ 🔍 ${tr("盲审验收", "Blind Verify")}`;
  }
}

function renderWorkflowsSettingsPage() {
  const workflows = state.config.workflows || {};
  const wfIds = Object.keys(workflows).sort();
  const agentRoles = Object.keys(state.config.agents || {}).sort();
  const allRefs = getAllAvailableModelRefs();
  const draft = ensureEditingWorkflowDraft();

  const topoOptions = [
    { id: "hybrid-fleet", label: tr("异构混合舰队 (hybrid-fleet)", "Hybrid Fleet (hybrid-fleet)") },
    { id: "review-loop", label: tr("主从实现与盲审 (review-loop)", "Review Loop (review-loop)") },
    { id: "worktree-team", label: tr("并行工作树小队 (worktree-team)", "Worktree Team (worktree-team)") },
    { id: "parallel-explore", label: tr("多路并行探索 (parallel-explore)", "Parallel Explore (parallel-explore)") },
    { id: "event-pipeline", label: tr("串行流水线 (event-pipeline)", "Event Pipeline (event-pipeline)") },
    { id: "shared-board", label: tr("共享黑板协同 (shared-board)", "Shared Board (shared-board)") }
  ];
  const verifyPresets = [
    { v: "go test ./...", l: "Go: go test ./..." },
    { v: "CGO_ENABLED=0 go test -race ./...", l: "Go Race: CGO_ENABLED=0 go test -race ./..." },
    { v: "npm test", l: "Node: npm test" },
    { v: "pytest", l: "Python: pytest" },
    { v: "cargo test", l: "Rust: cargo test" },
    { v: "", l: tr("自定义 / 无验收命令", "Custom / No verify command") }
  ];
  const concVal = Number(draft.max_concurrency || 3);
  const concChoices = [1, 2, 3, 4, 5, 6].includes(concVal) ? [1, 2, 3, 4, 5, 6] : [concVal, 1, 2, 3, 4, 5, 6];
  const roundsVal = Number(draft.max_rounds ?? draft.max_review_rounds ?? 2);
  const roundsChoices = [1, 2, 3, 4, 5].includes(roundsVal) ? [1, 2, 3, 4, 5] : [roundsVal, 1, 2, 3, 4, 5];

  const stageCardsHTML = (draft.nodes || [])
    .map((nd, idx) => {
      const nodeRole = nd.role || nd.agent_role || "coder";
      const overrideVal = draft.role_models?.[nd.id] ?? draft.role_models?.[nodeRole] ?? "";
      const roleMeta = ROLE_META[nodeRole] || { icon: "🤖", zh: nodeRole, en: nodeRole };
      return `
        <div class="wf-stage-row">
          <div class="role-title-line">
            <span>${roleMeta.icon}</span>
            <strong>${esc(nd.label || nd.id)}</strong>
            <span class="role-id-badge">${esc(nd.id)}</span>
          </div>
          <label class="cfg-field-label">
            <span>${tr("绑定角色", "Stage Role")}</span>
            <select class="mac-select mono" data-wf-node-idx="${idx}" data-wf-node-field="role" title="${esc(nodeRole)}">
              ${(agentRoles.includes(nodeRole) ? agentRoles : [nodeRole, ...agentRoles])
                .map((r) => `<option value="${esc(r)}" ${r === nodeRole ? "selected" : ""}>${esc(state.config.agents?.[r]?.name || r)} (${esc(r)})</option>`)
                .join("")}
            </select>
          </label>
          <label class="cfg-field-label">
            <span>${tr("指定模型", "Stage Model")}</span>
            <select class="mac-select mono" data-wf-node-idx="${idx}" data-wf-node-field="model_override" title="${esc(overrideVal || tr("跟随角色矩阵", "Follow Matrix"))}">
              ${renderModelCatalogOptgroupsHTML(overrideVal, tr("跟随角色矩阵", "Follow Matrix"))}
            </select>
          </label>
        </div>
      `;
    })
    .join("");

  return `
    <div class="cfg-section-card">
      <div class="cfg-section-head">
        <div>
          <h3 class="cfg-section-title">🕸️ ${tr("多智能体拓扑极简配置 (workflows/*.toml)", "Simplified Multi-Agent Topology Config (workflows/*.toml)")}</h3>
          <p class="cfg-section-desc">${tr("三步完成多智能体协同拓扑配置：1. 选择拓扑模式 ➔ 2. 设定并发与验收参数 ➔ 3. 绑定各阶段角色与模型。", "3-step topology setup: 1. Pick Topology Mode ➔ 2. Set Concurrency & Verify Gate ➔ 3. Bind Stage Roles & Models.")}</p>
        </div>
        <button id="new-workflow-draft-btn" class="primary-xs-btn" type="button">${tr("+ 新建工作流", "+ New Workflow")}</button>
      </div>

      <div class="pill-bar">
        ${wfIds
          .map(
            (id) => `<button class="pill-opt ${state.editingWorkflowId === id ? "active" : ""}" type="button" data-select-edit-wf="${esc(id)}">${esc(workflows[id].name || id)} (${esc(id)})</button>`
          )
          .join("")}
        ${state.editingWorkflowId === "__new__" ? `<button class="pill-opt active" type="button">${tr("✨ 新建工作流", "✨ New Workflow")}</button>` : ""}
      </div>

      <!-- Step 1: Basic Info & Topology Preset -->
      <div class="three-col">
        <label class="cfg-field-label">
          <span>1. ${tr("工作流 ID (workflows/<id>.toml)", "Workflow ID")}</span>
          <input id="wf-edit-id" class="mac-input mono" type="text" value="${esc(draft.id)}" placeholder="custom-review-fleet">
        </label>
        <label class="cfg-field-label">
          <span>${tr("显示名称 (Name)", "Display Name")}</span>
          <input id="wf-edit-name" class="mac-input" type="text" value="${esc(draft.name)}" placeholder="Custom Review Fleet">
        </label>
        <label class="cfg-field-label">
          <span>${tr("拓扑预设模式 (切换自动生成阶段)", "Topology Mode Preset")}</span>
          <select id="wf-edit-topology" class="mac-select mono" title="${esc(draft.topology)}">
            ${topoOptions.map((tp) => `<option value="${esc(tp.id)}" ${draft.topology === tp.id ? "selected" : ""}>${esc(tp.label)}</option>`).join("")}
          </select>
        </label>
      </div>

      <!-- Step 2: Core Run Parameters -->
      <div class="three-col">
        <label class="cfg-field-label">
          <span>2. ${tr("并发执行数 (max_concurrency)", "Max Concurrency")}</span>
          <select id="wf-edit-conc" class="mac-select mono">
            ${concChoices.map((n) => `<option value="${n}" ${concVal === n ? "selected" : ""}>${n} ${tr("并发", "workers")}</option>`).join("")}
          </select>
        </label>
        <label class="cfg-field-label">
          <span>${tr("最大审查轮次 (max_rounds)", "Max Review Rounds")}</span>
          <select id="wf-edit-rounds" class="mac-select mono">
            ${roundsChoices.map((n) => `<option value="${n}" ${roundsVal === n ? "selected" : ""}>${n} ${tr("轮", "rounds")}</option>`).join("")}
          </select>
        </label>
        <label class="cfg-field-label">
          <span>${tr("验收命令预设", "Verify Command Preset")}</span>
          <select id="wf-verify-preset-select" class="mac-select mono">
            ${verifyPresets.map((vp) => `<option value="${esc(vp.v)}" ${(draft.verify_command || "") === vp.v ? "selected" : ""}>${esc(vp.l)}</option>`).join("")}
          </select>
        </label>
      </div>

      <div class="two-col" style="align-items:end;">
        <label class="cfg-field-label">
          <span>${tr("验收测试命令 (可自由编辑)", "Verify Command (editable)")}</span>
          <input id="wf-edit-verify" class="mac-input mono" type="text" value="${esc(draft.verify_command || "")}" placeholder="go test ./...">
        </label>
        <label class="checkbox-row" style="padding-bottom:6px;">
          <input id="wf-edit-wt" type="checkbox" ${draft.use_worktrees ? "checked" : ""}>
          <span>${tr("启用 Git Worktree 物理隔离与串行合入", "Isolate stages in Git worktrees")}</span>
        </label>
      </div>

      <!-- Step 3: Visual Flow Preview Bar + Stage Role & Model Cards -->
      <div class="cfg-field-label">
        <span>3. ${tr("阶段角色与模型绑定（可视化拓扑流程）", "Stage Roles & Model Bindings")}</span>
        <div class="topo-flow-preview" id="wf-flow-preview-bar">
          ${formatTopologyFlowPreview(draft.topology, concVal, roundsVal)}
        </div>
        <div style="display:flex;flex-direction:column;gap:8px;margin-top:6px;">
          ${stageCardsHTML}
        </div>
      </div>

      <div class="subhead-row" style="padding-top:6px;border-top:1px solid var(--hairline);">
        ${
          draft.id && workflows[draft.id]
            ? `<button id="delete-workflow-def-btn" class="danger-xs-btn" type="button" data-del-wf="${esc(draft.id)}">${tr("🗑️ 删除此工作流", "🗑️ Delete Workflow")}</button>`
            : `<span></span>`
        }
        <button id="save-workflow-def-btn" class="primary-sm-btn" type="button">${tr("保存工作流模板", "Save Workflow")}</button>
      </div>
    </div>
  `;
}

// --- Page 6: Permissions, Workspace Trust, Visual Rules & Lifecycle Hooks ---
function renderPermissionsSettingsPage() {
  const r = state.config.rules || {};
  const allowCmds = Array.isArray(r.allow_commands) ? r.allow_commands : [];
  const askCmds = Array.isArray(r.ask_commands) ? r.ask_commands : [];
  const denyCmds = Array.isArray(r.deny_commands) ? r.deny_commands : [];
  const allowPaths = Array.isArray(r.allow_paths) ? r.allow_paths : [];
  const denyPaths = Array.isArray(r.deny_paths) ? r.deny_paths : [];
  const allowDomains = Array.isArray(r.allowed_domains) ? r.allowed_domains : [];
  const customRules = Array.isArray(r.rules) ? r.rules : [];

  const ruleCategories = [
    { key: "allow_commands", label: tr("✅ 自动放行命令 (allow_commands)", "✅ Allow Commands (allow_commands)"), list: allowCmds, chipClass: "" },
    { key: "ask_commands", label: tr("⚠️ 每次询问命令 (ask_commands)", "⚠️ Ask Commands (ask_commands)"), list: askCmds, chipClass: "warn-chip" },
    { key: "deny_commands", label: tr("🚫 严禁执行命令 (deny_commands)", "🚫 Deny Commands (deny_commands)"), list: denyCmds, chipClass: "danger-chip" },
    { key: "allow_paths", label: tr("📂 允许读写路径 (allow_paths)", "📂 Allow Paths (allow_paths)"), list: allowPaths, chipClass: "" },
    { key: "deny_paths", label: tr("🔒 敏感路径黑名单 (deny_paths)", "🔒 Deny Paths (deny_paths)"), list: denyPaths, chipClass: "danger-chip" },
    { key: "allowed_domains", label: tr("🌐 出网域名白名单 (allowed_domains)", "🌐 Allowed Domains (allowed_domains)"), list: allowDomains, chipClass: "" }
  ];
  const activeCatKey = state.activeRuleCategory || "allow_commands";

  const activeWs = state.workspaces.find((w) => w.id === state.activeWorkspaceId) || null;
  const trustHashes = activeWs ? Object.keys(activeWs.trusted_hashes || {}) : [];

  const categoryTagsHTML = ruleCategories
    .map((cat) => {
      const tags = cat.list
        .map(
          (val) =>
            `<span class="chip-btn active ${cat.chipClass} mono">${esc(val)} <span class="chip-remove" data-remove-rule-tag="${esc(cat.key)}" data-val="${esc(val)}">✕</span></span>`
        )
        .join("");
      return `
        <div class="subagent-worker-row" style="align-items:flex-start;">
          <strong style="min-width:190px;font-size:11.5px;">${esc(cat.label)}</strong>
          <div class="chips-wrap" style="flex:1;">
            ${tags || `<span class="tiny-muted">${tr("暂无规则", "Empty")}</span>`}
          </div>
        </div>
      `;
    })
    .join("");

  const hooksCfg = state.config.hooks || {};
  const hookPhases = [
    { key: "pre_tool_use", label: tr("PreToolUse · 工具调用前", "PreToolUse · Before Tool") },
    { key: "post_tool_use", label: tr("PostToolUse · 工具调用后", "PostToolUse · After Tool") },
    { key: "turn_stopping", label: tr("TurnStopping · 回合结束前", "TurnStopping · Turn Stopping") }
  ];
  const hookRows = [];
  hookPhases.forEach((ph) => {
    const arr = Array.isArray(hooksCfg[ph.key]) ? hooksCfg[ph.key] : [];
    arr.forEach((hk, idx) => {
      hookRows.push(`
        <div class="subagent-worker-row mono">
          <div class="row-actions" style="min-width:0;flex:1;">
            <span class="role-id-badge">${esc(ph.label)}</span>
            <span class="meta-chip mono">${esc(hk.matcher || (hk.tools || []).join("|") || "*")}</span>
            <code style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">${esc(hk.command || "")}</code>
            <span class="tiny-muted">${Number(hk.timeout_sec || 10)}s</span>
          </div>
          <button class="danger-xs-btn" type="button" data-del-hook-phase="${esc(ph.key)}" data-del-hook-idx="${idx}">✕</button>
        </div>
      `);
    });
  });

  return `
    <!-- Section A: Active Workspace Policy, .gorness Repo Trust & Persisted Grants -->
    <div class="cfg-section-card">
      <div class="cfg-section-head">
        <div>
          <h3 class="cfg-section-title">📁 ${tr("当前项目信任状态与已记住授权 (Trust & Grants)", "Active Workspace Policy, .gorness Trust & Grants")}</h3>
          <p class="cfg-section-desc">${tr("管理项目级权限覆盖、校验信任仓库内 .gorness 配置，以及查看/撤销已持久化的授权指纹。", "Manage workspace policy, trust repo-local .gorness files, and revoke persisted approval grants.")}</p>
        </div>
      </div>

      ${
        !activeWs
          ? `<span class="tiny-muted">${tr("请先在左侧挂载并选择一个项目工作区。", "Select or mount a workspace first.")}</span>`
          : `
          <div class="settings-form-row">
            <div class="settings-row-info">
              <strong>${esc(activeWs.name)} — ${tr("项目级门控与沙箱", "Workspace Gate & Sandbox")}</strong>
              <span class="mono">${esc(activeWs.root_path)}</span>
            </div>
            <div class="row-actions">
              <select id="ws-perm-override-sel" class="mac-select">
                ${["auto", "accept_edits", "ask", "plan", "yolo"].map((m) => `<option value="${m}" ${(activeWs.permission_mode || "auto") === m ? "selected" : ""}>${tr("门控", "Gate")}: ${m}</option>`).join("")}
              </select>
              <select id="ws-sb-override-sel" class="mac-select">
                ${["workspace-write", "read-only", "off"].map((m) => `<option value="${m}" ${(activeWs.sandbox_mode || "workspace-write") === m ? "selected" : ""}>${tr("沙箱", "Sandbox")}: ${m}</option>`).join("")}
              </select>
            </div>
          </div>

          <div class="settings-form-row">
            <div class="settings-row-info">
              <strong>🛡️ ${tr("仓库本地 .gorness 配置哈希信任", "Repo-Local .gorness Config SHA-256 Trust")}</strong>
              <span>${trustHashes.length > 0 ? `${tr("已信任文件数", "Trusted files")}: ${trustHashes.length}` : tr("未信任项目内执行类扩展（防止克隆恶意仓库自动提权）", "Untrusted repo-local execution overrides")}</span>
            </div>
            <button id="trust-workspace-btn" class="primary-xs-btn" type="button">${tr("信任当前仓库 .gorness", "Trust Repo .gorness")}</button>
          </div>

          <div class="cfg-field-label">
            <div class="subhead-row">
              <span>${tr("已记住的审批授权指纹 (grants.toml)", "Persisted Approval Grants (grants.toml)")}</span>
              ${
                state.workspaceGrants.length > 0
                  ? `<button id="revoke-all-grants-btn" class="danger-xs-btn" type="button">${tr("撤销全部授权", "Revoke All")}</button>`
                  : ""
              }
            </div>
            <div class="ao-list" style="max-height:130px;">
              ${
                state.workspaceGrants.length === 0
                  ? `<span class="tiny-muted">${tr("当前工作区暂无已记住的持久化授权。", "No persisted approval grants for this workspace.")}</span>`
                  : state.workspaceGrants
                      .map((gr) => {
                        const fp = gr.fingerprint || gr.id || gr.pattern || String(gr);
                        return `
                          <div class="subagent-worker-row mono">
                            <span style="overflow:hidden;text-overflow:ellipsis;">${esc(gr.tool_name ? `${gr.tool_name}: ` : "")}${esc(gr.summary || fp)}</span>
                            <button class="danger-xs-btn" type="button" data-revoke-grant="${esc(fp)}">${tr("撤销", "Revoke")}</button>
                          </div>
                        `;
                      })
                      .join("")
              }
            </div>
          </div>
        `
      }
    </div>

    <!-- Section B: Visual Rules Tag List & Single-Line Quick Add (rules.toml) -->
    <div class="cfg-section-card">
      <div class="cfg-section-head">
        <div>
          <h3 class="cfg-section-title">📜 ${tr("全局命令、路径与域名可视化规则 (rules.toml)", "Visual Command, Path & Network Rules (rules.toml)")}</h3>
          <p class="cfg-section-desc">${tr("选择分类后单行快速添加，或点击标签 ✕ 立即移除并自动保存至 PUT /api/config/rules。", "Pick a category to add rules or click ✕ on any tag to auto-save via PUT /api/config/rules.")}</p>
        </div>
      </div>

      <div class="inline-form">
        <select id="rules-category-select" class="mac-select" style="max-width:250px;">
          ${ruleCategories.map((c) => `<option value="${esc(c.key)}" ${c.key === activeCatKey ? "selected" : ""}>${esc(c.label)}</option>`).join("")}
        </select>
        <input id="rules-quick-add-input" class="mac-input mono" type="text" placeholder="${tr("输入通配规则，如 go test* / .env* / api.openai.com", "Enter glob rule, e.g. go test* / .env* / api.openai.com")}">
        <button id="rules-quick-add-btn" class="primary-xs-btn" type="button">${tr("+ 快速添加", "+ Add Rule")}</button>
      </div>

      <div class="chips-wrap">
        <span class="tiny-muted">${tr("常用快捷点选：", "Quick presets:")}</span>
        ${SAFE_CMD_PRESETS.map((c) => `<button class="chip-btn mono ${allowCmds.includes(c) ? "active" : ""}" type="button" data-rule-chip="allow_commands" data-val="${esc(c)}">${esc(c)}</button>`).join("")}
        ${SAFE_DOMAIN_PRESETS.map((d) => `<button class="chip-btn mono ${allowDomains.includes(d) ? "active" : ""}" type="button" data-rule-chip="allowed_domains" data-val="${esc(d)}">${esc(d)}</button>`).join("")}
      </div>

      <div style="display:flex;flex-direction:column;gap:6px;">
        ${categoryTagsHTML}
      </div>
    </div>

    <!-- Section C: Visual Lifecycle Hooks Card (hooks.toml) -->
    <div class="cfg-section-card">
      <div class="cfg-section-head">
        <div>
          <h3 class="cfg-section-title">🪝 ${tr("生命周期钩子可视化配置 (hooks.toml)", "Lifecycle Command Hooks (hooks.toml)")}</h3>
          <p class="cfg-section-desc">${tr("在工具执行前 (PreToolUse)、执行后 (PostToolUse) 或回合结束前 (TurnStopping) 自动运行校验脚本，实时保存至 PUT /api/config/hooks。", "Run shell hooks on PreToolUse, PostToolUse, or TurnStopping (saved via PUT /api/config/hooks).")}</p>
        </div>
      </div>

      <div class="three-col">
        <label class="cfg-field-label">
          <span>${tr("触发阶段 (Phase)", "Lifecycle Phase")}</span>
          <select id="hook-phase-select" class="mac-select">
            ${hookPhases.map((p) => `<option value="${esc(p.key)}">${esc(p.label)}</option>`).join("")}
          </select>
        </label>
        <label class="cfg-field-label">
          <span>${tr("匹配工具 (Matcher)", "Target Tool")}</span>
          <select id="hook-matcher-select" class="mac-select mono">
            <option value="*">* (${tr("全部工具", "All Tools")})</option>
            ${ALL_BUILTIN_TOOLS.map((tl) => `<option value="${esc(tl)}">${esc(tl)}</option>`).join("")}
          </select>
        </label>
        <label class="cfg-field-label">
          <span>${tr("超时秒数 (timeout_sec)", "Timeout (sec)")}</span>
          <input id="hook-timeout-input" class="mac-input mono" type="number" min="1" max="300" value="10">
        </label>
      </div>

      <div class="inline-form">
        <input id="hook-command-input" class="mac-input mono" type="text" placeholder="${tr("输入钩子 Shell 命令，例如：go vet ./... 或自定义安全审计脚本", "Shell command, e.g. go vet ./...")}">
        <button id="add-hook-btn" class="primary-xs-btn" type="button">${tr("+ 添加钩子", "+ Add Hook")}</button>
      </div>

      <div style="display:flex;flex-direction:column;gap:6px;">
        ${hookRows.join("") || `<span class="tiny-muted">${tr("当前暂无已配置的生命周期钩子。", "No lifecycle hooks configured yet.")}</span>`}
      </div>
    </div>
  `;
}

// --- Page 7: MCP Servers (Render ALL mcp.toml Servers + Full Custom stdio/http/sse Editor) ---
function ensureEditingMCPDraft() {
  const servers = state.config?.mcp?.servers || {};
  const ids = Object.keys(servers).sort();
  if (!state.editingMCPDraft) {
    if (state.editingMCPId && servers[state.editingMCPId]) {
      const s = servers[state.editingMCPId];
      state.editingMCPDraft = {
        id: state.editingMCPId,
        transport: s.transport || "stdio",
        command: s.command || "",
        args: Array.isArray(s.args) ? [...s.args] : [],
        url: s.url || "",
        env: Object.assign({}, s.env || {}),
        headers: Object.assign({}, s.headers || {}),
        timeout_sec: Number(s.timeout_sec || 60),
        enabled: s.enabled !== false
      };
    } else {
      state.editingMCPDraft = {
        id: "",
        transport: "stdio",
        command: "npx",
        args: ["-y", "@modelcontextprotocol/server-filesystem", "."],
        url: "",
        env: {},
        headers: {},
        timeout_sec: 60,
        enabled: true
      };
    }
  }
  return state.editingMCPDraft;
}

function renderMCPSettingsPage() {
  const servers = state.config.mcp?.servers || {};
  const serverIds = Object.keys(servers).sort();
  const draft = ensureEditingMCPDraft();

  const envText = Object.entries(draft.env || {}).map(([k, v]) => `${k}=${v}`).join("\n");
  const hdrText = Object.entries(draft.headers || {}).map(([k, v]) => `${k}: ${v}`).join("\n");

  const allServersListHTML = serverIds.length === 0
    ? `<span class="tiny-muted">${tr("当前 mcp.toml 暂无已注册服务。可点选下方模板或填写自定义表单添加。", "No MCP servers configured in mcp.toml yet.")}</span>`
    : serverIds
        .map((id) => {
          const sv = servers[id];
          const summary = sv.transport === "stdio"
            ? `${sv.command || ""} ${(sv.args || []).join(" ")}`
            : sv.url || "";
          return `
            <div class="subagent-worker-row">
              <div style="min-width:0;flex:1;">
                <div class="row-actions">
                  <strong class="mono">${esc(id)}</strong>
                  <span class="meta-chip mono">${esc(sv.transport || "stdio")}</span>
                  <span class="status-pill ${sv.enabled ? "ok" : ""}">${sv.enabled ? tr("已启用", "enabled") : tr("已停用", "disabled")}</span>
                </div>
                <div class="mono tiny-muted" style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">${esc(summary)}</div>
              </div>
              <div class="row-actions">
                <button class="ghost-xs-btn" type="button" data-mcp-toggle-enable="${esc(id)}">${sv.enabled ? tr("停用", "Disable") : tr("启用", "Enable")}</button>
                <button class="ghost-xs-btn" type="button" data-mcp-edit="${esc(id)}">${tr("编辑", "Edit")}</button>
                <button class="danger-xs-btn" type="button" data-mcp-delete="${esc(id)}">✕</button>
              </div>
            </div>
          `;
        })
        .join("");

  return `
    <div class="cfg-section-card">
      <div class="cfg-section-head">
        <div>
          <h3 class="cfg-section-title">🔌 ${tr("已注册 MCP 扩展服务列表 (mcp.toml 全量视图)", "Configured MCP Servers (All mcp.toml Entries)")}</h3>
          <p class="cfg-section-desc">${tr("完整展示 mcp.toml 中的每一台 stdio / http / sse 服务，支持即时启停、编辑与删除。", "Displays every configured server in mcp.toml with instant toggle, edit & delete.")}</p>
        </div>
        <button id="new-mcp-draft-btn" class="primary-xs-btn" type="button">${tr("+ 新建自定义 MCP", "+ New Custom MCP")}</button>
      </div>
      <div style="display:flex;flex-direction:column;gap:6px;">${allServersListHTML}</div>
    </div>

    <div class="cfg-section-card">
      <div class="cfg-section-head">
        <div>
          <h3 class="cfg-section-title">🛠️ ${tr("MCP 服务自定义编辑器 (支持 stdio / http / sse)", "Custom MCP Server Editor (stdio / http / sse)")}</h3>
          <p class="cfg-section-desc">${tr("点选快捷模板自动填充表单，或手动配置任意本地命令、远程端点、环境变量与认证请求头。", "Click a preset chip to pre-fill, or configure custom command, args, URL, env & headers.")}</p>
        </div>
      </div>

      <div class="chips-wrap">
        <span class="tiny-muted">${tr("快捷模板填充：", "Quick-fill presets:")}</span>
        ${MCP_SERVER_PRESETS.map(
          (mp) => `<button class="chip-btn mono" type="button" data-fill-mcp-preset="${esc(mp.id)}">+ ${esc(mp.id)}</button>`
        ).join("")}
      </div>

      <div class="three-col">
        <label class="cfg-field-label">
          <span>Server ID</span>
          <input id="mcp-edit-id" class="mac-input mono" type="text" value="${esc(draft.id)}" placeholder="my-mcp-server">
        </label>
        <label class="cfg-field-label">
          <span>${tr("传输方式 (Transport)", "Transport")}</span>
          <select id="mcp-edit-transport" class="mac-select mono">
            ${["stdio", "http", "sse"].map((tp) => `<option value="${tp}" ${draft.transport === tp ? "selected" : ""}>${tp}</option>`).join("")}
          </select>
        </label>
        <label class="cfg-field-label">
          <span>${tr("超时秒数 (timeout_sec)", "Timeout (sec)")}</span>
          <input id="mcp-edit-timeout" class="mac-input mono" type="number" min="5" max="600" value="${Number(draft.timeout_sec || 60)}">
        </label>
      </div>

      ${
        draft.transport === "stdio"
          ? `
          <div class="two-col">
            <label class="cfg-field-label">
              <span>${tr("启动命令 (command)", "Command")}</span>
              <input id="mcp-edit-cmd" class="mac-input mono" type="text" value="${esc(draft.command)}" placeholder="npx / uvx / /usr/local/bin/mcp">
            </label>
            <label class="cfg-field-label">
              <span>${tr("命令参数 (args，空格或换行分隔)", "Arguments")}</span>
              <input id="mcp-edit-args" class="mac-input mono" type="text" value="${esc((draft.args || []).join(" "))}" placeholder="-y @modelcontextprotocol/server-filesystem .">
            </label>
          </div>
          <details class="advanced-config-disclosure">
            <summary>${tr("高级选项：环境变量 (KEY=VALUE，可选)", "Advanced: Environment Variables (Optional)")}</summary>
            <div class="advanced-config-body">
              <input id="mcp-edit-env" class="mac-input mono" type="text" value="${esc(Object.entries(draft.env || {}).map(([k, v]) => `${k}=${v}`).join("; "))}" placeholder="GITHUB_TOKEN=keychain:GITHUB_TOKEN">
            </div>
          </details>
        `
          : `
          <label class="cfg-field-label">
            <span>${tr("远程服务 URL (http / sse)", "Endpoint URL")}</span>
            <input id="mcp-edit-url" class="mac-input mono" type="text" value="${esc(draft.url)}" placeholder="http://127.0.0.1:8090/mcp">
          </label>
          <details class="advanced-config-disclosure">
            <summary>${tr("高级选项：自定义请求头 (Header: Value，可选)", "Advanced: Custom HTTP Headers (Optional)")}</summary>
            <div class="advanced-config-body">
              <input id="mcp-edit-headers" class="mac-input mono" type="text" value="${esc(Object.entries(draft.headers || {}).map(([k, v]) => `${k}: ${v}`).join("; "))}" placeholder="Authorization: Bearer ...">
            </div>
          </details>
        `
      }

      <div class="subhead-row">
        <label class="checkbox-row">
          <input id="mcp-edit-enabled" type="checkbox" ${draft.enabled ? "checked" : ""}>
          <span>${tr("保存后立即启用此 MCP 服务", "Enable immediately")}</span>
        </label>
        <button id="save-mcp-form-btn" class="primary-sm-btn" type="button">${tr("保存 MCP 服务", "Save MCP Server")}</button>
      </div>
    </div>
  `;
}

// --- Page 8: 4-Layer Memory & Candidate Skills ---
function renderMemorySettingsPage() {
  const notesMap = state.memoryData?.notes || {};
  const noteSlugs = Object.keys(notesMap).sort();
  const activeNoteSlug = state.editingNoteSlug || noteSlugs[0] || "architecture";
  const activeNoteBody = notesMap[activeNoteSlug] ?? "";

  const progSlugs = Object.keys(state.workspaceProgress || {}).sort();
  const activeProgSlug = state.editingProgressSlug || progSlugs[0] || "current";
  const activeProgBody = state.workspaceProgress?.[activeProgSlug] ?? "";

  const candidates = state.candidateSkills || [];

  return `
    <!-- Layer 1 & Layer 2: Global INSTRUCTIONS.md & INDEX.md -->
    <div class="cfg-section-card">
      <div class="cfg-section-head">
        <div>
          <h3 class="cfg-section-title">🧠 ${tr("第 1 & 2 层：全局固定指令与记忆索引 (INSTRUCTIONS.md / INDEX.md)", "Layers 1 & 2: Global Instructions & Memory Index")}</h3>
          <p class="cfg-section-desc">${tr("存储于 ~/.gorness/memory/，对所有项目共享长期编码偏好与主题索引。", "Persisted via PUT /api/memory/layer across all workspaces.")}</p>
        </div>
      </div>
      <div class="two-col">
        <div class="cfg-field-label">
          <div class="subhead-row">
            <span class="mono"><strong>memory/INSTRUCTIONS.md</strong></span>
            <button class="primary-xs-btn" type="button" data-save-mem-layer="instructions">${tr("保存", "Save")}</button>
          </div>
          <textarea id="mem-instructions-editor" class="vault-editor mono short" rows="6">${esc(state.memoryData?.instructions || "")}</textarea>
        </div>
        <div class="cfg-field-label">
          <div class="subhead-row">
            <span class="mono"><strong>memory/INDEX.md</strong></span>
            <button class="primary-xs-btn" type="button" data-save-mem-layer="index">${tr("保存", "Save")}</button>
          </div>
          <textarea id="mem-index-editor" class="vault-editor mono short" rows="6">${esc(state.memoryData?.index || "")}</textarea>
        </div>
      </div>
    </div>

    <!-- Layer 3: Topic Notes & Layer 4: Workspace Progress -->
    <div class="two-col">
      <div class="cfg-section-card">
        <div class="subhead-row">
          <h3 class="cfg-section-title">📒 ${tr("第 3 层：按需主题笔记 (notes/*.md)", "Layer 3: Topic Notes (notes/*.md)")}</h3>
          <div class="chips-wrap">
            ${noteSlugs.map((sl) => `<button class="chip-btn mono ${sl === activeNoteSlug ? "active" : ""}" type="button" data-pick-note-slug="${esc(sl)}">${esc(sl)}</button>`).join("")}
          </div>
        </div>
        <div class="inline-form">
          <input id="mem-note-slug" class="mac-input mono" type="text" value="${esc(activeNoteSlug)}" placeholder="topic-slug">
          <button id="save-mem-note-btn" class="primary-xs-btn" type="button">${tr("保存笔记", "Save Note")}</button>
          ${notesMap[activeNoteSlug] !== undefined ? `<button id="del-mem-note-btn" class="danger-xs-btn" type="button">${tr("删除", "Delete")}</button>` : ""}
        </div>
        <textarea id="mem-note-body" class="vault-editor mono short" rows="6" placeholder="# Topic Note...">${esc(activeNoteBody)}</textarea>
      </div>

      <div class="cfg-section-card">
        <div class="subhead-row">
          <h3 class="cfg-section-title">📈 ${tr("第 4 层：项目进度记忆 (progress/*.md)", "Layer 4: Workspace Progress (progress/*.md)")}</h3>
          <div class="chips-wrap">
            ${progSlugs.map((sl) => `<button class="chip-btn mono ${sl === activeProgSlug ? "active" : ""}" type="button" data-pick-prog-slug="${esc(sl)}">${esc(sl)}</button>`).join("")}
          </div>
        </div>
        ${
          !state.activeWorkspaceId
            ? `<span class="tiny-muted">${tr("请先选择一个项目工作区以编辑第 4 层进度记忆。", "Select a workspace first.")}</span>`
            : `
            <div class="inline-form">
              <input id="mem-prog-slug" class="mac-input mono" type="text" value="${esc(activeProgSlug)}" placeholder="sprint-progress">
              <button id="save-mem-prog-btn" class="primary-xs-btn" type="button">${tr("保存进度", "Save Progress")}</button>
              <button id="del-mem-prog-btn" class="danger-xs-btn" type="button">${tr("删除", "Delete")}</button>
            </div>
            <textarea id="mem-prog-body" class="vault-editor mono short" rows="6" placeholder="# Workspace Progress...">${esc(activeProgBody)}</textarea>
          `
        }
      </div>
    </div>

    <!-- Dreamer Candidate Skills Promotion / Discard -->
    <div class="cfg-section-card">
      <div class="cfg-section-head">
        <div>
          <h3 class="cfg-section-title">✨ ${tr("Dreamer 候选技能审批台 (skills/_candidates/*/SKILL.md)", "Dreamer Candidate Skills Review")}</h3>
          <p class="cfg-section-desc">${tr("后台 Dreamer 从历史高价值会话归纳的候选技能，经人工审阅后一键晋升为正式 $skill 或丢弃。", "Promote vetted candidate skills into ~/.gorness/skills/ or discard.")}</p>
        </div>
        <span class="pill-count">${candidates.length}</span>
      </div>
      ${
        candidates.length === 0
          ? `<span class="tiny-muted">${tr("当前无待审阅的候选技能（可在常驻中枢点击「立即运行 Dreamer」生成）。", "No candidate skills pending review.")}</span>`
          : candidates
              .map((c) => {
                const slug = c.slug || c.name || "";
                return `
                  <div class="ctx-card">
                    <div class="subhead-row">
                      <div>
                        <strong class="mono">$${esc(slug)}</strong>
                        <span class="tiny-muted" style="margin-left:8px;">${esc(c.description || "")}</span>
                      </div>
                      <div class="row-actions">
                        <button class="primary-xs-btn" type="button" data-promote-skill="${esc(slug)}">${tr("✓ 晋升为正式技能", "✓ Promote")}</button>
                        <button class="danger-xs-btn" type="button" data-discard-skill="${esc(slug)}">${tr("✕ 丢弃", "✕ Discard")}</button>
                      </div>
                    </div>
                    ${c.content || c.body ? `<div class="md-preview-box" style="max-height:160px;">${renderMarkdownHTML(c.content || c.body)}</div>` : ""}
                  </div>
                `;
              })
              .join("")
      }
    </div>
  `;
}

// ============================================================================
// Exported Config Action Helpers (Preserving 100% server_test.go Contracts)
// ============================================================================

async function applyProviderPreset(presetId, plainApiKey = "") {
  const preset = PROVIDER_PRESETS[presetId];
  if (!preset) return null;
  setAutoSaveIndicator("saving");
  try {
    const existing = state.config?.providers?.[preset.id] || {};
    let apiKeyVal = existing.api_key || preset.api_key;
    if (plainApiKey && plainApiKey.trim()) {
      const trimmed = plainApiKey.trim();
      // Store secret in macOS Keychain when keychain_key is defined and endpoint is available
      if (preset.keychain_key && !trimmed.startsWith("keychain:") && !trimmed.startsWith("env:")) {
        try {
          await apiFetch("/api/config/secrets", {
            method: "POST",
            body: { key: preset.keychain_key, value: trimmed }
          });
          apiKeyVal = `keychain:${preset.keychain_key}`;
        } catch {
          apiKeyVal = trimmed;
        }
      } else {
        apiKeyVal = trimmed;
      }
    }
    const mergedModels = Object.assign({}, preset.models, existing.models || {});
    const snap = await apiFetch("/api/config/providers", {
      method: "POST",
      body: {
        id: preset.id,
        name: existing.name || preset.name,
        protocol: existing.protocol || preset.protocol,
        base_url: existing.base_url || preset.base_url,
        api_key: apiKeyVal,
        timeout_sec: existing.timeout_sec || preset.timeout_sec,
        models: mergedModels
      }
    });
    state.config = snap;
    state.editingProviderId = preset.id;
    state.editingProviderDraft = null;
    setAutoSaveIndicator("ok");
    populateAgentAndModelControls();
    if (state.settingsOpen) renderConfigStudio();
    return snap;
  } catch (err) {
    setAutoSaveIndicator("err", err.message);
    notifyError(err, tr("注册供应商模板失败", "Failed to apply provider preset"));
    return null;
  }
}

async function patchAgentRole(agentId, patch) {
  if (!agentId) return null;
  setAutoSaveIndicator("saving");
  try {
    const payload = Object.assign({ id: agentId }, patch);
    if (payload.provider && !payload.model) {
      const firstModel = getModelsForProvider(payload.provider)[0]?.id || "";
      if (firstModel) payload.model = firstModel;
    }
    const snap = await apiFetch("/api/config/agents", {
      method: "POST",
      body: payload
    });
    state.config = snap;
    if (state.editingAgentId === agentId) state.editingAgentDraft = null;
    setAutoSaveIndicator("ok");
    populateAgentAndModelControls();
    if (state.settingsOpen) renderConfigStudio();
    return snap;
  } catch (err) {
    setAutoSaveIndicator("err", err.message);
    notifyError(err, tr("更新角色配置失败", "Failed to update agent role"));
    return null;
  }
}

async function applyMatrixScheme(schemeId) {
  const scheme = MATRIX_SCHEMES.find((s) => s.id === schemeId);
  if (!scheme) return null;
  setAutoSaveIndicator("saving");
  try {
    const neededProvs = new Set([scheme.default_provider]);
    Object.values(scheme.roles).forEach((r) => neededProvs.add(r.provider));
    for (const pid of neededProvs) {
      if (!state.config?.providers?.[pid] && PROVIDER_PRESETS[pid]) {
        const pr = PROVIDER_PRESETS[pid];
        state.config = await apiFetch("/api/config/providers", {
          method: "POST",
          body: {
            id: pr.id,
            name: pr.name,
            protocol: pr.protocol,
            base_url: pr.base_url,
            api_key: pr.api_key,
            timeout_sec: pr.timeout_sec,
            models: pr.models
          }
        });
      }
    }
    state.config = await apiFetch("/api/config/global", {
      method: "PUT",
      body: {
        default_provider: scheme.default_provider,
        default_model: scheme.default_model,
        default_effort: scheme.default_effort
      }
    });
    for (const [roleId, spec] of Object.entries(scheme.roles)) {
      state.config = await apiFetch("/api/config/agents", {
        method: "POST",
        body: {
          id: roleId,
          provider: spec.provider,
          model: spec.model,
          effort: spec.effort
        }
      });
    }
    setAutoSaveIndicator("ok");
    populateAgentAndModelControls();
    if (state.settingsOpen) renderConfigStudio();
    showToast(`${tr("已应用矩阵方案", "Applied matrix scheme")}: ${state.lang === "zh" ? scheme.zh : scheme.en}`, "ok");
    return state.config;
  } catch (err) {
    setAutoSaveIndicator("err", err.message);
    notifyError(err, tr("应用矩阵方案失败", "Failed to apply matrix scheme"));
    return null;
  }
}

async function toggleRuleChip(field, value) {
  const cur = Array.isArray(state.config?.rules?.[field]) ? [...state.config.rules[field]] : [];
  const idx = cur.indexOf(value);
  if (idx >= 0) cur.splice(idx, 1);
  else cur.push(value);
  setAutoSaveIndicator("saving");
  try {
    const snap = await apiFetch("/api/config/rules", {
      method: "PUT",
      body: { [field]: cur }
    });
    state.config = snap;
    setAutoSaveIndicator("ok");
    if (state.settingsOpen) renderConfigStudio();
    return snap;
  } catch (err) {
    setAutoSaveIndicator("err", err.message);
    notifyError(err, tr("保存规则失败", "Failed to update rules"));
    return null;
  }
}

async function toggleMCPPreset(presetId) {
  const preset = MCP_SERVER_PRESETS.find((m) => m.id === presetId);
  const existing = state.config?.mcp?.servers?.[presetId];
  setAutoSaveIndicator("saving");
  try {
    let snap;
    if (existing) {
      snap = await apiFetch(`/api/config/mcp/${encodeURIComponent(presetId)}`, { method: "DELETE" });
    } else if (preset) {
      snap = await apiFetch("/api/config/mcp", {
        method: "POST",
        body: {
          id: preset.id,
          transport: preset.transport,
          command: preset.command,
          args: preset.args,
          timeout_sec: preset.timeout_sec,
          enabled: true
        }
      });
    }
    if (snap) state.config = snap;
    setAutoSaveIndicator("ok");
    if (state.settingsOpen) renderConfigStudio();
    return snap;
  } catch (err) {
    setAutoSaveIndicator("err", err.message);
    notifyError(err, tr("切换 MCP 模板失败", "Failed to toggle MCP preset"));
    return null;
  }
}

const applyMatrixPreset = applyMatrixScheme;
const patchAgentField = patchAgentRole;

// ============================================================================
// Form Parsing & Delegated Event Bindings for All 9 Settings Pages
// ============================================================================

function parseLinesList(raw) {
  return String(raw || "")
    .split(/\r?\n/)
    .map((s) => s.trim())
    .filter(Boolean);
}

function parseSemicolonOrLinesList(raw) {
  return String(raw || "")
    .split(/[\r\n;]+/)
    .map((s) => s.trim())
    .filter(Boolean);
}

function parseArgsText(raw) {
  const s = String(raw || "").trim();
  if (!s) return [];
  if (s.includes("\n")) return parseLinesList(s);
  return s.split(/\s+/).filter(Boolean);
}

function parseCommaList(raw) {
  return String(raw || "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
}

function parseHeadersText(raw) {
  const out = {};
  parseSemicolonOrLinesList(raw).forEach((line) => {
    const idx = line.indexOf(":");
    if (idx > 0) {
      const k = line.slice(0, idx).trim();
      const v = line.slice(idx + 1).trim();
      if (k) out[k] = v;
    }
  });
  return out;
}

function parseEnvText(raw) {
  const out = {};
  parseSemicolonOrLinesList(raw).forEach((line) => {
    const idx = line.indexOf("=");
    if (idx > 0) {
      const k = line.slice(0, idx).trim();
      const v = line.slice(idx + 1).trim();
      if (k) out[k] = v;
    }
  });
  return out;
}

function syncProviderFormIntoDraft() {
  const d = ensureEditingProviderDraft();
  if ($("prov-edit-id")) d.id = $("prov-edit-id").value.trim();
  if ($("prov-edit-name")) d.name = $("prov-edit-name").value.trim();
  if ($("prov-edit-protocol")) d.protocol = $("prov-edit-protocol").value;
  if ($("prov-edit-baseurl")) d.base_url = $("prov-edit-baseurl").value.trim();
  if ($("prov-edit-apikey")) d.api_key = $("prov-edit-apikey").value.trim();
  if ($("prov-edit-timeout")) d.timeout_sec = Number($("prov-edit-timeout").value || 180);
  if ($("prov-edit-headers")) d.headers = parseHeadersText($("prov-edit-headers").value);
  return d;
}

async function fetchProviderModels() {
  const d = syncProviderFormIntoDraft();
  if (!d.id) {
    showToast(tr("请先填写供应商 ID", "Provider ID is required first"), "error");
    return;
  }
  const rawApiKey = (d.api_key || "").trim();
  if (rawApiKey && !rawApiKey.startsWith("keychain:") && !rawApiKey.startsWith("env:")) {
    const kcKey = `${d.id.toUpperCase().replace(/[^A-Z0-9_]/g, "_")}_API_KEY`;
    try {
      await apiFetch("/api/config/secrets", { method: "POST", body: { key: kcKey, value: rawApiKey } });
      d.api_key = `keychain:${kcKey}`;
      if ($("prov-edit-apikey")) $("prov-edit-apikey").value = d.api_key;
    } catch (_) {
      // Keep rawApiKey for discovery request if keychain write fails in test/mock env
    }
  }
  setAutoSaveIndicator("saving");
  let models = [];
  let source = "remote";
  let warning = "";
  try {
    const res = await apiFetch("/api/config/providers/fetch-models", {
      method: "POST",
      body: {
        id: d.id,
        provider_id: d.id,
        type: d.protocol,
        protocol: d.protocol,
        base_url: d.base_url,
        api_key: rawApiKey || d.api_key,
        headers: d.headers || {}
      }
    });
    if (Array.isArray(res?.models)) models = res.models;
    else if (res?.models && typeof res.models === "object") {
      models = Object.entries(res.models).map(([id, spec]) => Object.assign({ id }, spec));
    }
    source = res?.source || "remote";
    warning = res?.warning || "";
  } catch (err) {
    const presetModels = PROVIDER_PRESETS[d.id]?.models || {};
    models = Object.entries(presetModels).map(([id, spec]) => Object.assign({ id }, spec));
    source = "preset_fallback";
    warning = err?.message || tr("在线探测不可用，已回退至内置推荐目录", "Remote fetch unavailable; using preset catalog");
  }
  state.fetchedProviderModels = models.map((m) => ({
    id: m.id,
    name: m.name || m.id,
    context_window: Number(m.context_window || 128000),
    max_output_tokens: Number(m.max_output_tokens || 16384),
    supports_thinking: m.supports_thinking !== false,
    input_cost_per_1m: Number(m.input_cost_per_1m || 0),
    output_cost_per_1m: Number(m.output_cost_per_1m || 0)
  }));
  state.fetchedModelsSource = source;
  state.fetchedModelsWarning = warning;
  state.fetchedModelsFilter = "";
  const existingIds = Object.keys(d.models || {});
  const nextSel = new Set();
  state.fetchedProviderModels.forEach((m) => {
    if (existingIds.length > 0 ? Boolean(d.models?.[m.id]) : true) {
      nextSel.add(m.id);
    }
  });
  state.selectedFetchedModels = nextSel;
  setAutoSaveIndicator("ok");
  renderConfigStudio();
  showToast(
    `${tr("已获取模型列表", "Fetched model list")}: ${state.fetchedProviderModels.length} ${tr("个模型", "models")}`,
    "ok"
  );
}

async function confirmAndSaveCatalogModels() {
  const d = syncProviderFormIntoDraft();
  if (!d.id) {
    showToast(tr("请填写供应商 ID", "Provider ID is required"), "error");
    return;
  }
  const nextModels = {};
  const existingModels = d.models || {};
  const isModelSelected = (id) =>
    state.selectedFetchedModels instanceof Set
      ? state.selectedFetchedModels.has(id)
      : Boolean(state.selectedFetchedModels?.[id]);
  state.fetchedProviderModels.forEach((m) => {
    if (isModelSelected(m.id)) {
      nextModels[m.id] = existingModels[m.id] || {
        name: m.name || m.id,
        context_window: Number(m.context_window || 128000),
        max_output_tokens: Number(m.max_output_tokens || 16384),
        supports_thinking: m.supports_thinking !== false,
        input_cost_per_1m: Number(m.input_cost_per_1m || 0),
        output_cost_per_1m: Number(m.output_cost_per_1m || 0)
      };
    }
  });
  Object.entries(existingModels).forEach(([id, spec]) => {
    const wasInFetched = state.fetchedProviderModels.some((fm) => fm.id === id);
    if (!wasInFetched) nextModels[id] = spec;
  });
  d.models = nextModels;
  setAutoSaveIndicator("saving");
  try {
    const snap = await apiFetch("/api/config/providers", { method: "POST", body: d });
    state.config = snap;
    state.editingProviderId = d.id;
    state.editingProviderDraft = null;
    state.fetchedProviderModels = [];
    setAutoSaveIndicator("ok");
    populateAgentAndModelControls();
    renderConfigStudio();
    showToast(`${tr("已确认并保存模型目录", "Confirmed & saved model catalog")}: ${Object.keys(nextModels).length}`, "ok");
  } catch (err) {
    setAutoSaveIndicator("err", err.message);
    notifyError(err, tr("保存模型目录失败", "Failed to save model catalog"));
  }
}

function syncWorkflowFormIntoDraft() {
  const d = ensureEditingWorkflowDraft();
  if ($("wf-edit-id")) d.id = $("wf-edit-id").value.trim();
  if ($("wf-edit-name")) d.name = $("wf-edit-name").value.trim();
  if ($("wf-edit-topology")) d.topology = $("wf-edit-topology").value;
  if ($("wf-edit-verify")) d.verify_command = $("wf-edit-verify").value.trim();
  if ($("wf-edit-conc")) d.max_concurrency = Number($("wf-edit-conc").value || 3);
  if ($("wf-edit-rounds")) d.max_rounds = Number($("wf-edit-rounds").value || 2);
  if ($("wf-edit-budget")) d.budget_usd = Number($("wf-edit-budget").value || 0);
  if ($("wf-edit-wt")) d.use_worktrees = Boolean($("wf-edit-wt").checked);
  if (!Array.isArray(d.edges) || d.edges.length === 0) {
    d.edges = buildStandardTopologyTemplate(d.topology).edges;
  }
  return d;
}

function bindConfigStudioEvents() {
  // Sidebar navigation across all 8 visual pages
  $$(".settings-nav-btn[data-cfg-page]").forEach((btn) => {
    btn.addEventListener("click", () => {
      state.configPage = btn.dataset.cfgPage;
      state.vaultValidationError = "";
      loadSettingsAuxData(state.configPage);
      renderConfigStudio();
    });
  });

  $("reload-config-btn")?.addEventListener("click", () => {
    refreshConfigSnapshot();
    loadSettingsAuxData(state.configPage);
  });

  const studio = $("config-studio-content");
  if (!studio) return;

  studio.addEventListener("click", async (ev) => {
    // --- Page 1: General ---
    if (ev.target.closest("#save-general-limits-btn")) {
      const maxConc = Number($("gen-max-runs")?.value || 8);
      const maxSteps = Number($("gen-max-steps")?.value || 32);
      const debounceMs = Number($("gen-debounce-ms")?.value || 400);
      await saveGlobalConfigPatch({
        max_concurrency: maxConc,
        max_steps: maxSteps,
        inbox_debounce_ms: debounceMs,
        max_concurrent_runs: maxConc,
        default_max_steps: maxSteps,
        always_on_debounce_ms: debounceMs
      });
      showToast(tr("全局引擎参数已保存", "Engine limits saved"), "ok");
      renderConfigStudio();
      return;
    }

    // --- Page 2: Matrix ---
    const subTabBtn = ev.target.closest("[data-matrix-subtab]");
    if (subTabBtn) {
      state.matrixSubTab = subTabBtn.dataset.matrixSubtab;
      renderConfigStudio();
      return;
    }
    const schemeBtn = ev.target.closest("[data-apply-scheme]");
    if (schemeBtn) {
      await applyMatrixScheme(schemeBtn.dataset.applyScheme);
      return;
    }
    const rmFb = ev.target.closest("[data-remove-fallback]");
    if (rmFb) {
      const aId = rmFb.dataset.removeFallback;
      const fb = rmFb.dataset.fb;
      const cur = Array.isArray(state.config?.agents?.[aId]?.fallbacks) ? [...state.config.agents[aId].fallbacks] : [];
      await patchAgentRole(aId, { fallbacks: cur.filter((x) => x !== fb) });
      return;
    }
    const jumpEditAg = ev.target.closest("[data-jump-edit-agent]");
    if (jumpEditAg) {
      state.editingAgentId = jumpEditAg.dataset.jumpEditAgent;
      state.editingAgentDraft = null;
      state.configPage = "agents";
      renderConfigStudio();
      return;
    }

    // --- Page 3: Providers & Keychain ---
    const fillKc = ev.target.closest("[data-fill-keychain-key]");
    if (fillKc && $("keychain-key-input")) {
      $("keychain-key-input").value = fillKc.dataset.fillKeychainKey;
      $("keychain-val-input")?.focus?.();
      return;
    }
    if (ev.target.closest("#save-keychain-secret-btn")) {
      const key = ($("keychain-key-input")?.value || "").trim();
      const value = ($("keychain-val-input")?.value || "").trim();
      if (!key || !value) {
        showToast(tr("请填写钥匙串键名与密钥值", "Enter both key name and secret value"), "error");
        return;
      }
      setAutoSaveIndicator("saving");
      try {
        await apiFetch("/api/config/secrets", { method: "POST", body: { key, value } });
        if ($("keychain-val-input")) $("keychain-val-input").value = "";
        setAutoSaveIndicator("ok");
        showToast(`${tr("已安全存入 macOS 钥匙串", "Saved to macOS Keychain")}: keychain:${key}`, "ok");
      } catch (err) {
        setAutoSaveIndicator("err", err.message);
        notifyError(err, tr("存入钥匙串失败", "Failed to save secret"));
      }
      return;
    }
    if (ev.target.closest("#delete-keychain-secret-btn")) {
      const key = ($("keychain-key-input")?.value || "").trim();
      if (!key) return;
      try {
        await apiFetch(`/api/config/secrets/${encodeURIComponent(key)}`, { method: "DELETE" });
        showToast(`${tr("已从钥匙串删除", "Deleted secret")}: ${key}`, "ok");
      } catch (err) {
        notifyError(err, tr("删除钥匙串密钥失败", "Failed to delete secret"));
      }
      return;
    }
    const provPresetBtn = ev.target.closest("[data-apply-prov-preset]");
    if (provPresetBtn) {
      await applyProviderPreset(provPresetBtn.dataset.applyProvPreset);
      showToast(`${tr("已注册供应商模板", "Added provider")}: ${provPresetBtn.dataset.applyProvPreset}`, "ok");
      return;
    }
    const selEditProv = ev.target.closest("[data-select-edit-prov]");
    if (selEditProv) {
      state.editingProviderId = selEditProv.dataset.selectEditProv;
      state.editingProviderDraft = null;
      state.fetchedProviderModels = [];
      renderConfigStudio();
      return;
    }
    if (ev.target.closest("#new-provider-draft-btn")) {
      state.editingProviderId = "__new__";
      state.editingProviderDraft = null;
      state.fetchedProviderModels = [];
      renderConfigStudio();
      return;
    }
    if (ev.target.closest("#fetch-provider-models-btn") || ev.target.closest("[data-fetch-provider-models]")) {
      await fetchProviderModels();
      return;
    }
    if (ev.target.closest("#select-all-fetched-models-btn")) {
      syncProviderFormIntoDraft();
      if (!(state.selectedFetchedModels instanceof Set)) state.selectedFetchedModels = new Set();
      const q = (state.fetchedModelsFilter || "").trim().toLowerCase();
      state.fetchedProviderModels.forEach((m) => {
        if (!q || m.id.toLowerCase().includes(q) || (m.name || "").toLowerCase().includes(q)) {
          state.selectedFetchedModels.add(m.id);
        }
      });
      renderConfigStudio();
      return;
    }
    if (ev.target.closest("#clear-all-fetched-models-btn")) {
      syncProviderFormIntoDraft();
      state.selectedFetchedModels = new Set();
      renderConfigStudio();
      return;
    }
    if (ev.target.closest("#confirm-save-catalog-models-btn")) {
      await confirmAndSaveCatalogModels();
      return;
    }
    if (ev.target.closest("#add-draft-model-btn")) {
      const d = syncProviderFormIntoDraft();
      const mId = ($("prov-new-model-id")?.value || "").trim();
      if (!mId) {
        showToast(tr("请输入模型 ID", "Enter model ID"), "error");
        return;
      }
      d.models[mId] = {
        name: ($("prov-new-model-name")?.value || "").trim() || mId,
        context_window: Number($("prov-new-model-ctx")?.value || 128000),
        max_output_tokens: 16384,
        supports_thinking: true,
        input_cost_per_1m: Number($("prov-new-model-in")?.value || 0),
        output_cost_per_1m: Number($("prov-new-model-out")?.value || 0)
      };
      renderConfigStudio();
      return;
    }
    const delModelBtn = ev.target.closest("[data-del-draft-model]");
    if (delModelBtn) {
      const d = syncProviderFormIntoDraft();
      const rmId = delModelBtn.dataset.delDraftModel;
      delete d.models[rmId];
      if (state.selectedFetchedModels instanceof Set) state.selectedFetchedModels.delete(rmId);
      if (d.id && state.config?.providers?.[d.id]) {
        try {
          const snap = await apiFetch("/api/config/providers", { method: "POST", body: d });
          state.config = snap;
          populateAgentAndModelControls();
        } catch (_) {}
      }
      renderConfigStudio();
      return;
    }
    if (ev.target.closest("#save-provider-form-btn")) {
      const d = syncProviderFormIntoDraft();
      if (!d.id) {
        showToast(tr("请填写供应商 ID", "Provider ID is required"), "error");
        return;
      }
      setAutoSaveIndicator("saving");
      try {
        const snap = await apiFetch("/api/config/providers", { method: "POST", body: d });
        state.config = snap;
        state.editingProviderId = d.id;
        state.editingProviderDraft = null;
        setAutoSaveIndicator("ok");
        populateAgentAndModelControls();
        renderConfigStudio();
        showToast(`${tr("供应商已保存", "Provider saved")}: ${d.id}`, "ok");
      } catch (err) {
        setAutoSaveIndicator("err", err.message);
        notifyError(err, tr("保存供应商失败", "Failed to save provider"));
      }
      return;
    }
    const delProvBtn = ev.target.closest("[data-del-prov]");
    if (delProvBtn) {
      const pid = delProvBtn.dataset.delProv;
      try {
        const snap = await apiFetch(`/api/config/providers/${encodeURIComponent(pid)}`, { method: "DELETE" });
        state.config = snap;
        state.editingProviderId = "";
        state.editingProviderDraft = null;
        populateAgentAndModelControls();
        renderConfigStudio();
        showToast(`${tr("已删除供应商", "Deleted provider")}: ${pid}`, "ok");
      } catch (err) {
        notifyError(err, tr("删除供应商失败", "Failed to delete provider"));
      }
      return;
    }

    // --- Page 4: Agents ---
    const selEditAg = ev.target.closest("[data-select-edit-agent]");
    if (selEditAg) {
      state.editingAgentId = selEditAg.dataset.selectEditAgent;
      state.editingAgentDraft = null;
      renderConfigStudio();
      return;
    }
    if (ev.target.closest("#new-agent-draft-btn")) {
      state.editingAgentId = "__new__";
      state.editingAgentDraft = null;
      renderConfigStudio();
      return;
    }
    const fillAgPreset = ev.target.closest("[data-fill-agent-preset]");
    if (fillAgPreset) {
      const sp = SPECIALIST_ROLE_PRESETS.find((x) => x.id === fillAgPreset.dataset.fillAgentPreset);
      if (sp) {
        state.editingAgentId = "__new__";
        state.editingAgentDraft = {
          id: sp.id,
          name: sp.name,
          description: sp.description,
          provider: state.config?.global?.default_provider || "anthropic",
          model: state.config?.global?.default_model || "",
          effort: sp.effort,
          max_steps: 28,
          read_only: sp.read_only,
          allowed_tools: [...sp.allowed_tools],
          disallowed_tools: [],
          fallbacks: [],
          system_prompt: sp.system_prompt
        };
        renderConfigStudio();
      }
      return;
    }
    const toggleAgTool = ev.target.closest("[data-ag-toggle-tool]");
    if (toggleAgTool) {
      const tl = toggleAgTool.dataset.agToggleTool;
      const list = parseCommaList($("ag-edit-allow-tools")?.value || "");
      const idx = list.indexOf(tl);
      if (idx >= 0) list.splice(idx, 1);
      else list.push(tl);
      if ($("ag-edit-allow-tools")) $("ag-edit-allow-tools").value = list.join(", ");
      toggleAgTool.classList.toggle("active", idx < 0);
      return;
    }
    if (ev.target.closest("#save-agent-def-btn")) {
      const id = ($("ag-edit-id")?.value || "").trim();
      if (!id) {
        showToast(tr("请填写智能体 ID", "Agent ID is required"), "error");
        return;
      }
      const allowRaw = ($("ag-edit-allow-tools")?.value || "").trim();
      const payload = {
        id,
        name: ($("ag-edit-name")?.value || "").trim() || id,
        description: ($("ag-edit-desc")?.value || "").trim(),
        provider: $("ag-edit-provider")?.value || "",
        model: ($("ag-edit-model")?.value || "").trim(),
        effort: $("ag-edit-effort")?.value || "high",
        max_steps: Number($("ag-edit-maxsteps")?.value || 0),
        read_only: Boolean($("ag-edit-readonly")?.checked),
        disallowed_tools: parseCommaList($("ag-edit-disallow-tools")?.value || ""),
        fallbacks: parseCommaList($("ag-edit-fallbacks")?.value || ""),
        system_prompt: $("ag-edit-sysprompt")?.value || ""
      };
      if (allowRaw || id === "utility") {
        payload.allowed_tools = parseCommaList(allowRaw);
      }
      setAutoSaveIndicator("saving");
      try {
        const snap = await apiFetch("/api/config/agents", { method: "POST", body: payload });
        state.config = snap;
        state.editingAgentId = id;
        state.editingAgentDraft = null;
        setAutoSaveIndicator("ok");
        populateAgentAndModelControls();
        renderConfigStudio();
        showToast(`${tr("智能体已保存", "Agent saved")}: ${id}`, "ok");
      } catch (err) {
        setAutoSaveIndicator("err", err.message);
        notifyError(err, tr("保存智能体失败", "Failed to save agent"));
      }
      return;
    }
    const delAgBtn = ev.target.closest("[data-del-agent]");
    if (delAgBtn) {
      const aid = delAgBtn.dataset.delAgent;
      try {
        const snap = await apiFetch(`/api/config/agents/${encodeURIComponent(aid)}`, { method: "DELETE" });
        state.config = snap;
        state.editingAgentId = "";
        state.editingAgentDraft = null;
        populateAgentAndModelControls();
        renderConfigStudio();
        showToast(`${tr("已删除智能体", "Deleted agent")}: ${aid}`, "ok");
      } catch (err) {
        notifyError(err, tr("删除智能体失败", "Failed to delete agent"));
      }
      return;
    }

    // --- Page 5: Workflows ---
    const selEditWf = ev.target.closest("[data-select-edit-wf]");
    if (selEditWf) {
      state.editingWorkflowId = selEditWf.dataset.selectEditWf;
      state.editingWorkflowDraft = null;
      renderConfigStudio();
      return;
    }
    if (ev.target.closest("#new-workflow-draft-btn")) {
      state.editingWorkflowId = "__new__";
      state.editingWorkflowDraft = null;
      renderConfigStudio();
      return;
    }
    if (ev.target.closest("#save-workflow-def-btn")) {
      const d = syncWorkflowFormIntoDraft();
      if (!d.id) {
        showToast(tr("请填写工作流 ID", "Workflow ID is required"), "error");
        return;
      }
      setAutoSaveIndicator("saving");
      try {
        const snap = await apiFetch("/api/config/workflows", { method: "POST", body: d });
        state.config = snap;
        state.editingWorkflowId = d.id;
        state.editingWorkflowDraft = null;
        setAutoSaveIndicator("ok");
        syncFleetLauncherWorkflows();
        renderConfigStudio();
        showToast(`${tr("工作流已保存", "Workflow saved")}: ${d.id}`, "ok");
      } catch (err) {
        setAutoSaveIndicator("err", err.message);
        notifyError(err, tr("保存工作流失败", "Failed to save workflow"));
      }
      return;
    }
    const delWfBtn = ev.target.closest("[data-del-wf]");
    if (delWfBtn) {
      const wid = delWfBtn.dataset.delWf;
      try {
        const snap = await apiFetch(`/api/config/workflows/${encodeURIComponent(wid)}`, { method: "DELETE" });
        state.config = snap;
        state.editingWorkflowId = "";
        state.editingWorkflowDraft = null;
        syncFleetLauncherWorkflows();
        renderConfigStudio();
        showToast(`${tr("已删除工作流", "Deleted workflow")}: ${wid}`, "ok");
      } catch (err) {
        notifyError(err, tr("删除工作流失败", "Failed to delete workflow"));
      }
      return;
    }

    // --- Page 6: Permissions, Trust, Grants & Hooks ---
    const ruleChip = ev.target.closest("[data-rule-chip]");
    if (ruleChip) {
      state.activeRuleCategory = ruleChip.dataset.ruleChip;
      await toggleRuleChip(ruleChip.dataset.ruleChip, ruleChip.dataset.val);
      return;
    }
    const rmRuleTag = ev.target.closest("[data-remove-rule-tag]");
    if (rmRuleTag) {
      await toggleRuleChip(rmRuleTag.dataset.removeRuleTag, rmRuleTag.dataset.val);
      return;
    }
    if (ev.target.closest("#rules-quick-add-btn")) {
      const cat = $("rules-category-select")?.value || state.activeRuleCategory || "allow_commands";
      const val = ($("rules-quick-add-input")?.value || "").trim();
      if (!val) return;
      state.activeRuleCategory = cat;
      const curList = Array.isArray(state.config?.rules?.[cat]) ? state.config.rules[cat] : [];
      if (!curList.includes(val)) {
        await toggleRuleChip(cat, val);
      }
      if ($("rules-quick-add-input")) $("rules-quick-add-input").value = "";
      return;
    }
    if (ev.target.closest("#add-hook-btn")) {
      const phase = $("hook-phase-select")?.value || "pre_tool_use";
      const tool = $("hook-tool-select")?.value || "*";
      const command = ($("hook-command-input")?.value || "").trim();
      const timeoutSec = Number($("hook-timeout-input")?.value || 15);
      if (!command) {
        showToast(tr("请填写钩子执行命令", "Hook command is required"), "error");
        return;
      }
      const curHooks = state.config?.hooks || {};
      const nextHooks = {
        pre_tool_use: Array.isArray(curHooks.pre_tool_use) ? [...curHooks.pre_tool_use] : [],
        post_tool_use: Array.isArray(curHooks.post_tool_use) ? [...curHooks.post_tool_use] : [],
        turn_stopping: Array.isArray(curHooks.turn_stopping) ? [...curHooks.turn_stopping] : []
      };
      if (!Array.isArray(nextHooks[phase])) nextHooks[phase] = [];
      nextHooks[phase].push({ tool, command, timeout_sec: timeoutSec });
      setAutoSaveIndicator("saving");
      try {
        const res = await apiFetch("/api/config/hooks", { method: "PUT", body: nextHooks });
        if (res?.config) state.config = res.config;
        else if (res?.global) state.config = res;
        else state.config = Object.assign({}, state.config || {}, { hooks: res?.hooks || nextHooks });
        setAutoSaveIndicator("ok");
        renderConfigStudio();
        showToast(tr("生命周期钩子已添加并保存", "Lifecycle hook saved"), "ok");
      } catch (err) {
        setAutoSaveIndicator("err", err.message);
        notifyError(err, tr("保存钩子失败", "Failed to save hooks"));
      }
      return;
    }
    const delHookBtn = ev.target.closest("[data-del-hook-phase]");
    if (delHookBtn) {
      const phase = delHookBtn.dataset.delHookPhase;
      const idx = Number(delHookBtn.dataset.delHookIdx);
      const curHooks = state.config?.hooks || {};
      const nextHooks = {
        pre_tool_use: Array.isArray(curHooks.pre_tool_use) ? [...curHooks.pre_tool_use] : [],
        post_tool_use: Array.isArray(curHooks.post_tool_use) ? [...curHooks.post_tool_use] : [],
        turn_stopping: Array.isArray(curHooks.turn_stopping) ? [...curHooks.turn_stopping] : []
      };
      if (Array.isArray(nextHooks[phase])) {
        nextHooks[phase].splice(idx, 1);
      }
      setAutoSaveIndicator("saving");
      try {
        const res = await apiFetch("/api/config/hooks", { method: "PUT", body: nextHooks });
        if (res?.config) state.config = res.config;
        else if (res?.global) state.config = res;
        else state.config = Object.assign({}, state.config || {}, { hooks: res?.hooks || nextHooks });
        setAutoSaveIndicator("ok");
        renderConfigStudio();
        showToast(tr("已移除该钩子", "Hook removed"), "ok");
      } catch (err) {
        setAutoSaveIndicator("err", err.message);
        notifyError(err, tr("删除钩子失败", "Failed to delete hook"));
      }
      return;
    }
    if (ev.target.closest("#trust-workspace-btn")) {
      if (!state.activeWorkspaceId) return;
      try {
        const updatedWs = await apiFetch(`/api/workspaces/${encodeURIComponent(state.activeWorkspaceId)}/trust`, {
          method: "POST",
          body: {}
        });
        const idx = state.workspaces.findIndex((w) => w.id === state.activeWorkspaceId);
        if (idx >= 0 && updatedWs?.id) state.workspaces[idx] = updatedWs;
        showToast(tr("已信任当前仓库 .gorness 配置哈希", "Trusted repo-local .gorness hashes"), "ok");
        await refreshWorkspaceInspector(state.activeWorkspaceId);
        renderConfigStudio();
      } catch (err) {
        notifyError(err, tr("信任 .gorness 失败", "Failed to trust .gorness"));
      }
      return;
    }
    const revGrant = ev.target.closest("[data-revoke-grant]");
    if (revGrant && state.activeWorkspaceId) {
      try {
        await apiFetch(
          `/api/workspaces/${encodeURIComponent(state.activeWorkspaceId)}/grants?fingerprint=${encodeURIComponent(revGrant.dataset.revokeGrant)}`,
          { method: "DELETE" }
        );
        await loadSettingsAuxData("permissions");
        showToast(tr("已撤销该项授权", "Grant revoked"), "ok");
      } catch (err) {
        notifyError(err, tr("撤销授权失败", "Failed to revoke grant"));
      }
      return;
    }
    if (ev.target.closest("#revoke-all-grants-btn") && state.activeWorkspaceId) {
      try {
        await apiFetch(`/api/workspaces/${encodeURIComponent(state.activeWorkspaceId)}/grants`, { method: "DELETE" });
        await loadSettingsAuxData("permissions");
        showToast(tr("已清空当前工作区记住的授权", "All workspace grants revoked"), "ok");
      } catch (err) {
        notifyError(err, tr("撤销全部授权失败", "Failed to revoke grants"));
      }
      return;
    }

    // --- Page 7: MCP Servers ---
    if (ev.target.closest("#new-mcp-draft-btn")) {
      state.editingMCPId = "";
      state.editingMCPDraft = null;
      renderConfigStudio();
      return;
    }
    const fillMcp = ev.target.closest("[data-fill-mcp-preset]");
    if (fillMcp) {
      const mp = MCP_SERVER_PRESETS.find((x) => x.id === fillMcp.dataset.fillMcpPreset);
      if (mp) {
        state.editingMCPId = mp.id;
        state.editingMCPDraft = {
          id: mp.id,
          transport: mp.transport,
          command: mp.command,
          args: [...mp.args],
          url: "",
          env: {},
          headers: {},
          timeout_sec: mp.timeout_sec,
          enabled: true
        };
        renderConfigStudio();
      }
      return;
    }
    const mcpToggleEn = ev.target.closest("[data-mcp-toggle-enable]");
    if (mcpToggleEn) {
      const id = mcpToggleEn.dataset.mcpToggleEnable;
      const cur = state.config?.mcp?.servers?.[id];
      if (!cur) return;
      try {
        const snap = await apiFetch("/api/config/mcp", {
          method: "POST",
          body: Object.assign({}, cur, { id, enabled: !cur.enabled })
        });
        state.config = snap;
        renderConfigStudio();
      } catch (err) {
        notifyError(err, tr("切换 MCP 状态失败", "Failed to toggle MCP server"));
      }
      return;
    }
    const mcpEditBtn = ev.target.closest("[data-mcp-edit]");
    if (mcpEditBtn) {
      state.editingMCPId = mcpEditBtn.dataset.mcpEdit;
      state.editingMCPDraft = null;
      renderConfigStudio();
      return;
    }
    const mcpDelBtn = ev.target.closest("[data-mcp-delete]");
    if (mcpDelBtn) {
      const id = mcpDelBtn.dataset.mcpDelete;
      try {
        const snap = await apiFetch(`/api/config/mcp/${encodeURIComponent(id)}`, { method: "DELETE" });
        state.config = snap;
        if (state.editingMCPId === id) {
          state.editingMCPId = "";
          state.editingMCPDraft = null;
        }
        renderConfigStudio();
        showToast(`${tr("已删除 MCP 服务", "Deleted MCP server")}: ${id}`, "ok");
      } catch (err) {
        notifyError(err, tr("删除 MCP 服务失败", "Failed to delete MCP server"));
      }
      return;
    }
    if (ev.target.closest("#save-mcp-form-btn")) {
      const id = ($("mcp-edit-id")?.value || "").trim();
      const transport = $("mcp-edit-transport")?.value || "stdio";
      if (!id) {
        showToast(tr("请填写 MCP Server ID", "MCP Server ID is required"), "error");
        return;
      }
      const payload = {
        id,
        transport,
        timeout_sec: Number($("mcp-edit-timeout")?.value || 60),
        enabled: Boolean($("mcp-edit-enabled")?.checked),
        command: transport === "stdio" ? ($("mcp-edit-cmd")?.value || "").trim() : "",
        args: transport === "stdio" ? parseArgsText($("mcp-edit-args")?.value) : [],
        env: transport === "stdio" ? parseEnvText($("mcp-edit-env")?.value) : {},
        url: transport !== "stdio" ? ($("mcp-edit-url")?.value || "").trim() : "",
        headers: transport !== "stdio" ? parseHeadersText($("mcp-edit-headers")?.value) : {}
      };
      setAutoSaveIndicator("saving");
      try {
        const snap = await apiFetch("/api/config/mcp", { method: "POST", body: payload });
        state.config = snap;
        state.editingMCPId = id;
        state.editingMCPDraft = null;
        setAutoSaveIndicator("ok");
        renderConfigStudio();
        showToast(`${tr("MCP 服务已保存", "MCP server saved")}: ${id}`, "ok");
      } catch (err) {
        setAutoSaveIndicator("err", err.message);
        notifyError(err, tr("保存 MCP 服务失败", "Failed to save MCP server"));
      }
      return;
    }

    // --- Page 8: 4-Layer Memory & Candidate Skills ---
    const saveMemLayer = ev.target.closest("[data-save-mem-layer]");
    if (saveMemLayer) {
      const layer = saveMemLayer.dataset.saveMemLayer;
      const content = layer === "instructions" ? $("mem-instructions-editor")?.value || "" : $("mem-index-editor")?.value || "";
      try {
        await apiFetch("/api/memory/layer", { method: "PUT", body: { layer, content } });
        state.memoryData[layer] = content;
        showToast(`${tr("已保存全局记忆层", "Saved memory layer")}: ${layer}`, "ok");
      } catch (err) {
        notifyError(err, tr("保存全局记忆失败", "Failed to save memory layer"));
      }
      return;
    }
    const pickNote = ev.target.closest("[data-pick-note-slug]");
    if (pickNote) {
      state.editingNoteSlug = pickNote.dataset.pickNoteSlug;
      renderConfigStudio();
      return;
    }
    if (ev.target.closest("#save-mem-note-btn")) {
      const slug = ($("mem-note-slug")?.value || "").trim();
      const content = $("mem-note-body")?.value || "";
      if (!slug) return;
      try {
        await apiFetch(`/api/memory/notes/${encodeURIComponent(slug)}`, { method: "PUT", body: { content } });
        state.editingNoteSlug = slug;
        state.memoryData.notes[slug] = content;
        renderConfigStudio();
        showToast(`${tr("已保存主题笔记", "Saved topic note")}: ${slug}`, "ok");
      } catch (err) {
        notifyError(err, tr("保存主题笔记失败", "Failed to save note"));
      }
      return;
    }
    if (ev.target.closest("#del-mem-note-btn")) {
      const slug = ($("mem-note-slug")?.value || "").trim();
      if (!slug) return;
      try {
        await apiFetch(`/api/memory/notes/${encodeURIComponent(slug)}`, { method: "DELETE" });
        delete state.memoryData.notes[slug];
        state.editingNoteSlug = Object.keys(state.memoryData.notes)[0] || "";
        renderConfigStudio();
        showToast(`${tr("已删除主题笔记", "Deleted note")}: ${slug}`, "ok");
      } catch (err) {
        notifyError(err, tr("删除主题笔记失败", "Failed to delete note"));
      }
      return;
    }
    const pickProg = ev.target.closest("[data-pick-prog-slug]");
    if (pickProg) {
      state.editingProgressSlug = pickProg.dataset.pickProgSlug;
      await loadSettingsAuxData("memory");
      return;
    }
    if (ev.target.closest("#save-mem-prog-btn") && state.activeWorkspaceId) {
      const slug = ($("mem-prog-slug")?.value || "").trim();
      const content = $("mem-prog-body")?.value || "";
      if (!slug) return;
      try {
        await apiFetch(
          `/api/workspaces/${encodeURIComponent(state.activeWorkspaceId)}/memory/progress/${encodeURIComponent(slug)}`,
          { method: "PUT", body: { content } }
        );
        state.editingProgressSlug = slug;
        state.workspaceProgress[slug] = content;
        renderConfigStudio();
        showToast(`${tr("已保存项目进度记忆", "Saved workspace progress")}: ${slug}`, "ok");
      } catch (err) {
        notifyError(err, tr("保存进度记忆失败", "Failed to save progress"));
      }
      return;
    }
    if (ev.target.closest("#del-mem-prog-btn") && state.activeWorkspaceId) {
      const slug = ($("mem-prog-slug")?.value || "").trim();
      if (!slug) return;
      try {
        await apiFetch(
          `/api/workspaces/${encodeURIComponent(state.activeWorkspaceId)}/memory/progress/${encodeURIComponent(slug)}`,
          { method: "DELETE" }
        );
        delete state.workspaceProgress[slug];
        state.editingProgressSlug = Object.keys(state.workspaceProgress)[0] || "current";
        renderConfigStudio();
        showToast(`${tr("已删除项目进度记忆", "Deleted workspace progress")}: ${slug}`, "ok");
      } catch (err) {
        notifyError(err, tr("删除进度记忆失败", "Failed to delete progress"));
      }
      return;
    }
    const promoteBtn = ev.target.closest("[data-promote-skill]");
    if (promoteBtn) {
      const slug = promoteBtn.dataset.promoteSkill;
      try {
        await apiFetch(`/api/skills/candidates/${encodeURIComponent(slug)}/promote`, { method: "POST", body: {} });
        await loadSettingsAuxData("memory");
        if (state.activeWorkspaceId) await refreshWorkspaceInspector(state.activeWorkspaceId);
        showToast(`${tr("已晋升正式技能", "Promoted skill")}: $${slug}`, "ok");
      } catch (err) {
        notifyError(err, tr("晋升候选技能失败", "Failed to promote skill"));
      }
      return;
    }
    const discardBtn = ev.target.closest("[data-discard-skill]");
    if (discardBtn) {
      const slug = discardBtn.dataset.discardSkill;
      try {
        await apiFetch(`/api/skills/candidates/${encodeURIComponent(slug)}`, { method: "DELETE" });
        await loadSettingsAuxData("memory");
        showToast(`${tr("已丢弃候选技能", "Discarded candidate skill")}: ${slug}`, "ok");
      } catch (err) {
        notifyError(err, tr("丢弃候选技能失败", "Failed to discard candidate skill"));
      }
      return;
    }

  });

  studio.addEventListener("keydown", async (ev) => {
    if (ev.target.id === "rules-quick-add-input" && ev.key === "Enter") {
      ev.preventDefault();
      $("rules-quick-add-btn")?.click?.();
    }
  });

  studio.addEventListener("input", (ev) => {
    if (ev.target.id === "fetched-models-filter") {
      syncProviderFormIntoDraft();
      state.fetchedModelsFilter = ev.target.value || "";
      renderConfigStudio();
      const inp = $("fetched-models-filter");
      if (inp) {
        inp.focus?.();
        try {
          inp.setSelectionRange?.(inp.value.length, inp.value.length);
        } catch (_) {}
      }
    }
  });

  studio.addEventListener("change", async (ev) => {
    const tEl = ev.target;

    if (tEl.dataset.toggleFetchedModel) {
      syncProviderFormIntoDraft();
      if (!(state.selectedFetchedModels instanceof Set)) state.selectedFetchedModels = new Set();
      const mid = tEl.dataset.toggleFetchedModel;
      if (tEl.checked) state.selectedFetchedModels.add(mid);
      else state.selectedFetchedModels.delete(mid);
      return;
    }

    // General page selects
    if (tEl.id === "cfg-ui-theme-select") {
      applyTheme(tEl.value, true);
      renderConfigStudio();
      return;
    }
    if (tEl.id === "cfg-ui-lang-select") {
      applyLanguage(tEl.value, true);
      return;
    }
    if (tEl.dataset.globalSelect) {
      await saveGlobalConfigPatch({ [tEl.dataset.globalSelect]: tEl.value });
      renderConfigStudio();
      return;
    }
    if (tEl.dataset.globalNumSelect) {
      await saveGlobalConfigPatch({ [tEl.dataset.globalNumSelect]: Number(tEl.value) });
      updateSessionContextMeter();
      renderConfigStudio();
      return;
    }
    if (tEl.dataset.globalBoolSelect) {
      await saveGlobalConfigPatch({ [tEl.dataset.globalBoolSelect]: tEl.value === "true" });
      renderConfigStudio();
      return;
    }

    // Matrix global selects
    if (tEl.id === "cfg-global-provider") {
      const p = tEl.value;
      const firstM = getModelsForProvider(p)[0]?.id || "";
      await saveGlobalConfigPatch({ default_provider: p, default_model: firstM });
      renderConfigStudio();
      return;
    }
    if (tEl.id === "cfg-global-model") {
      await saveGlobalConfigPatch({ default_model: tEl.value });
      return;
    }
    if (tEl.id === "cfg-global-effort") {
      await saveGlobalConfigPatch({ default_effort: tEl.value });
      return;
    }

    // Matrix per-role selects
    if (tEl.dataset.agentProvider) {
      await patchAgentRole(tEl.dataset.agentProvider, { provider: tEl.value });
      return;
    }
    if (tEl.dataset.agentModelSelect) {
      await patchAgentRole(tEl.dataset.agentModelSelect, { model: tEl.value });
      return;
    }
    if (tEl.dataset.agentEffortSelect) {
      await patchAgentRole(tEl.dataset.agentEffortSelect, { effort: tEl.value });
      return;
    }
    if (tEl.dataset.addFallback && tEl.value) {
      const aId = tEl.dataset.addFallback;
      const cur = Array.isArray(state.config?.agents?.[aId]?.fallbacks) ? [...state.config.agents[aId].fallbacks] : [];
      if (!cur.includes(tEl.value)) cur.push(tEl.value);
      await patchAgentRole(aId, { fallbacks: cur });
      return;
    }

    // Agent editor provider change refreshes model select dropdown
    if (tEl.id === "ag-edit-provider" && state.editingAgentDraft) {
      state.editingAgentDraft.provider = tEl.value;
      const models = getModelsForProvider(tEl.value);
      const nextModel = models[0]?.id || "";
      state.editingAgentDraft.model = nextModel;
      const sel = $("ag-edit-model");
      if (sel) {
        sel.innerHTML = models
          .map((m) => `<option value="${esc(m.id)}">${esc(m.name || m.id)} (${Math.round((m.context_window || 128000) / 1000)}K)</option>`)
          .join("");
        sel.value = nextModel;
      }
      return;
    }

    // Workflows 3-step visual configuration
    if (tEl.id === "wf-edit-topology") {
      const d = syncWorkflowFormIntoDraft();
      const tpl = buildStandardTopologyTemplate(tEl.value);
      d.topology = tEl.value;
      d.nodes = tpl.nodes;
      d.edges = tpl.edges;
      renderConfigStudio();
      return;
    }
    if (tEl.id === "wf-verify-preset-select" && tEl.value) {
      if ($("wf-edit-verify")) $("wf-edit-verify").value = tEl.value;
      return;
    }
    if (tEl.id === "wf-edit-conc" || tEl.id === "wf-edit-rounds") {
      const d = syncWorkflowFormIntoDraft();
      const bar = $("wf-flow-preview-bar");
      if (bar) bar.textContent = formatTopologyFlowPreview(d.topology, d.max_concurrency, d.max_rounds);
      return;
    }

    // Workflow node row edits (keying role_models by node.id!)
    if (tEl.dataset.wfNodeIdx !== undefined) {
      const d = syncWorkflowFormIntoDraft();
      const idx = Number(tEl.dataset.wfNodeIdx);
      const field = tEl.dataset.wfNodeField;
      const nd = d.nodes[idx];
      if (!nd) return;
      if (field === "id") {
        const oldId = nd.id;
        const newId = tEl.value.trim();
        if (oldId && d.role_models?.[oldId] && newId) {
          d.role_models[newId] = d.role_models[oldId];
          delete d.role_models[oldId];
        }
        nd.id = newId;
      } else if (field === "label") {
        nd.label = tEl.value;
      } else if (field === "role" || field === "agent_role") {
        nd.role = tEl.value;
        delete nd.agent_role;
      } else if (field === "worktree") {
        nd.worktree = Boolean(tEl.checked);
      } else if (field === "model_override") {
        d.role_models = Object.assign({}, d.role_models || {});
        if (tEl.value) d.role_models[nd.id] = tEl.value;
        else delete d.role_models[nd.id];
      }
      return;
    }

    // Permissions rule category switch
    if (tEl.id === "rules-category-select") {
      state.activeRuleCategory = tEl.value;
      renderConfigStudio();
      return;
    }

    // MCP transport switch re-renders stdio vs http/sse fields
    if (tEl.id === "mcp-edit-transport") {
      const d = ensureEditingMCPDraft();
      d.id = ($("mcp-edit-id")?.value || "").trim();
      d.transport = tEl.value;
      d.timeout_sec = Number($("mcp-edit-timeout")?.value || 60);
      d.enabled = Boolean($("mcp-edit-enabled")?.checked);
      renderConfigStudio();
      return;
    }

    // Workspace permission/sandbox override via canonical PATCH /api/workspaces/{wsID}
    if ((tEl.id === "ws-perm-override-sel" || tEl.id === "ws-sb-override-sel") && state.activeWorkspaceId) {
      const body =
        tEl.id === "ws-perm-override-sel"
          ? { permission_mode: tEl.value }
          : { sandbox_mode: tEl.value };
      try {
        const updated = await apiFetch(`/api/workspaces/${encodeURIComponent(state.activeWorkspaceId)}`, {
          method: "PATCH",
          body
        });
        const idx = state.workspaces.findIndex((w) => w.id === state.activeWorkspaceId);
        if (idx >= 0) state.workspaces[idx] = updated;
        showToast(tr("项目策略已更新", "Workspace policy updated"), "ok");
      } catch (err) {
        notifyError(err, tr("更新项目策略失败", "Failed to update workspace policy"));
      }
      return;
    }
  });
}

function bindAlwaysOnDrawerEvents() {
  $("alwayson-drawer-btn")?.addEventListener("click", () => openAlwaysOnDrawer());
  $("refresh-alwayson-btn")?.addEventListener("click", () => refreshAlwaysOn());
  $("trigger-dream-btn")?.addEventListener("click", () => runDreamNow());

  const body = $("alwayson-drawer-body");
  if (!body) return;

  body.addEventListener("click", async (ev) => {
    const defRes = ev.target.closest("[data-def-resolve]");
    if (defRes) {
      await resolveDeferredCard(defRes.dataset.defResolve, defRes.dataset.approve);
      return;
    }
    const cronChip = ev.target.closest("[data-ao-cron-chip]");
    if (cronChip && $("ao-timer-cron")) {
      $("ao-timer-cron").value = cronChip.dataset.aoCronChip;
      if ($("ao-timer-kind")) $("ao-timer-kind").value = "cron";
      return;
    }
    if (ev.target.closest("#ao-save-timer-btn")) {
      await submitTimerForm();
      return;
    }
    if (ev.target.closest("#ao-cancel-timer-edit-btn")) {
      state.editingTimerDraft = null;
      renderAlwaysOn();
      return;
    }
    const tmToggle = ev.target.closest("[data-ao-timer-toggle]");
    if (tmToggle) {
      await toggleTimerEnabled(tmToggle.dataset.aoTimerToggle, tmToggle.dataset.enabled === "1");
      return;
    }
    const tmTrigger = ev.target.closest("[data-ao-timer-trigger]");
    if (tmTrigger) {
      await triggerTimerNow(tmTrigger.dataset.aoTimerTrigger);
      return;
    }
    const tmEdit = ev.target.closest("[data-ao-timer-edit]");
    if (tmEdit) {
      const found = (state.alwaysOn.timers || []).find((x) => x.id === tmEdit.dataset.aoTimerEdit);
      if (found) {
        state.editingTimerDraft = Object.assign({}, found);
        renderAlwaysOn();
      }
      return;
    }
    const tmDel = ev.target.closest("[data-ao-timer-del]");
    if (tmDel) {
      await deleteTimerItem(tmDel.dataset.aoTimerDel);
      return;
    }
    if (ev.target.closest("#ao-send-webhook-btn")) {
      const source = ($("ao-webhook-source")?.value || "local").trim() || "local";
      const text = ($("ao-webhook-text")?.value || "").trim();
      if (!text) return;
      try {
        await apiFetch(`/api/alwayson/webhook/${encodeURIComponent(source)}`, {
          method: "POST",
          body: {
            workspace_id: state.activeWorkspaceId,
            session_id: state.activeSessionId,
            text
          }
        });
        if ($("ao-webhook-text")) $("ao-webhook-text").value = "";
        showToast(tr("Webhook 唤醒事件已投递", "Webhook enqueued"), "ok");
        await refreshAlwaysOn();
      } catch (err) {
        notifyError(err, tr("投递 Webhook 失败", "Webhook failed"));
      }
      return;
    }
    const ackBtn = ev.target.closest("[data-ao-ack-id]");
    if (ackBtn) {
      await ackOutboxItems([ackBtn.dataset.aoAckId]);
      return;
    }
    if (ev.target.closest("#ao-ack-all-outbox-btn")) {
      const ids = (state.alwaysOn.outbox || []).filter((o) => !o.acked && !o.read && o.id).map((o) => o.id);
      await ackOutboxItems(ids);
    }
  });
}

// ============================================================================
// Global Workbench Event Listeners & Keyboard Shortcuts
// ============================================================================

function bindEvents() {
  // Topbar: 2 Center Modes (Simple Chat | Fleet Collaboration) + Sole Bottom-Left Settings Button (#open-config-studio-btn)
  $("mode-chat-btn")?.addEventListener("click", () => setCenterMode("chat"));
  $("mode-fleet-btn")?.addEventListener("click", () => setCenterMode("fleet"));
  $("switch-to-fleet-btn")?.addEventListener("click", () => setCenterMode("fleet"));
  $("back-to-chat-btn")?.addEventListener("click", () => setCenterMode("chat"));
  $("launcher-back-to-chat-btn")?.addEventListener("click", () => setCenterMode("chat"));
  $("open-config-studio-btn")?.addEventListener("click", () => openSettingsModal(state.configPage || "matrix"));

  // Left Sidebar: Workspaces & Sessions
  $("add-workspace-btn")?.addEventListener("click", () => {
    populateWorkspaceSuggestions();
    $("workspace-modal")?.classList.remove("hidden");
  });
  $("pick-native-folder-btn")?.addEventListener("click", () => pickNativeMacFolder());
  $("ws-suggestions-list")?.addEventListener("click", async (ev) => {
    const card = ev.target.closest("[data-mount-sug-path]");
    if (!card) return;
    try {
      const ws = await apiFetch("/api/workspaces", {
        method: "POST",
        body: { path: card.dataset.mountSugPath, name: card.dataset.mountSugName || "" }
      });
      $("workspace-modal")?.classList.add("hidden");
      await loadBootstrap();
      await selectWorkspace(ws.id);
      showToast(`${tr("已挂载项目", "Mounted workspace")}: ${ws.name}`, "ok");
    } catch (err) {
      notifyError(err, tr("挂载项目失败", "Failed to mount workspace"));
    }
  });
  $("new-session-btn")?.addEventListener("click", () => createNewSession(false));

  $("workspace-tree")?.addEventListener("click", async (ev) => {
    const pinBtn = ev.target.closest("[data-pin-session]");
    if (pinBtn) {
      ev.stopPropagation();
      const sid = pinBtn.dataset.pinSession;
      const cur = state.sessions.find((s) => s.id === sid);
      if (cur) {
        try {
          const updated = await apiFetch(`/api/sessions/${encodeURIComponent(sid)}`, {
            method: "PATCH",
            body: { pinned: !cur.pinned }
          });
          cur.pinned = updated.pinned;
          renderWorkspaceTree();
        } catch (err) {
          notifyError(err);
        }
      }
      return;
    }
    const sessRow = ev.target.closest("[data-session-id]");
    if (sessRow) {
      setCenterMode("chat");
      await selectSession(sessRow.dataset.sessionId);
      return;
    }
    const wsHead = ev.target.closest("[data-select-ws]");
    if (wsHead) {
      await selectWorkspace(wsHead.dataset.selectWs);
    }
  });

  // Global Cross-Project Search
  let searchTimer = null;
  $("global-search-input")?.addEventListener("input", (ev) => {
    const q = ev.target.value.trim();
    const pop = $("search-results-popover");
    if (!pop) return;
    if (!q) {
      pop.classList.add("hidden");
      return;
    }
    clearTimeout(searchTimer);
    searchTimer = setTimeout(async () => {
      try {
        const res = await apiFetch(`/api/sessions/search?q=${encodeURIComponent(q)}&limit=15`);
        const hits = res.hits || [];
        pop.innerHTML = hits.length
          ? hits
              .map(
                (h) => `
              <div class="search-hit-item" data-hit-session="${esc(h.session_id)}">
                <div class="search-hit-title">${esc(h.title)} <span class="mono tiny-muted">#${h.seq}</span></div>
                <div class="search-hit-snippet">${esc(h.snippet)}</div>
              </div>`
              )
              .join("")
          : `<div class="search-hit-item tiny-muted">${tr("未找到匹配记录", "No matches found")}</div>`;
        pop.classList.remove("hidden");
      } catch (err) {
        notifyError(err, tr("搜索失败", "Search failed"));
      }
    }, 180);
  });

  $("global-search-input")?.addEventListener("keydown", (ev) => {
    if (ev.key === "Escape") {
      $("search-results-popover")?.classList.add("hidden");
      $("global-search-input")?.blur?.();
    }
  });

  $("search-results-popover")?.addEventListener("click", async (ev) => {
    const row = ev.target.closest("[data-hit-session]");
    if (!row) return;
    $("search-results-popover")?.classList.add("hidden");
    setCenterMode("chat");
    await selectSession(row.dataset.hitSession);
  });

  // Attention Queue
  $("attention-list")?.addEventListener("click", async (ev) => {
    const jump = ev.target.closest("[data-jump-session]");
    if (jump) {
      setCenterMode("chat");
      await selectSession(jump.dataset.jumpSession);
      return;
    }
    if (ev.target.closest("[data-open-alwayson]")) {
      openAlwaysOnDrawer();
    }
  });

  // Session Bar: Title, Role, Model, Worktree Pill & ONE Consolidated Actions Menu
  $("session-title-input")?.addEventListener("change", async (ev) => {
    if (!state.activeSessionId) return;
    try {
      const updated = await apiFetch(`/api/sessions/${encodeURIComponent(state.activeSessionId)}`, {
        method: "PATCH",
        body: { title: ev.target.value }
      });
      const idx = state.sessions.findIndex((s) => s.id === updated.id);
      if (idx >= 0) state.sessions[idx] = updated;
      renderWorkspaceTree();
    } catch (err) {
      notifyError(err);
    }
  });

  $("agent-role-select")?.addEventListener("change", async (ev) => {
    if (!state.activeSessionId) return;
    try {
      const updated = await apiFetch(`/api/sessions/${encodeURIComponent(state.activeSessionId)}`, {
        method: "PATCH",
        body: { agent_role: ev.target.value }
      });
      if (updated?.id) {
        const idx = state.sessions.findIndex((s) => s.id === updated.id);
        if (idx >= 0) state.sessions[idx] = updated;
        state.activeSessionMeta = updated;
        updateSessionContextMeter();
        renderWorkspaceTree();
      }
    } catch (err) {
      notifyError(err);
    }
  });

  $("model-override-input")?.addEventListener("change", async (ev) => {
    if (!state.activeSessionId) return;
    try {
      const updated = await apiFetch(`/api/sessions/${encodeURIComponent(state.activeSessionId)}`, {
        method: "PATCH",
        body: { model_override: (ev.target.value || "").trim() }
      });
      if (updated?.id) {
        const idx = state.sessions.findIndex((s) => s.id === updated.id);
        if (idx >= 0) state.sessions[idx] = updated;
        state.activeSessionMeta = updated;
        updateSessionContextMeter();
      }
    } catch (err) {
      notifyError(err);
    }
  });

  $("perm-mode-select")?.addEventListener("change", async (ev) => {
    if (!state.activeSessionId) return;
    try {
      const updated = await apiFetch(`/api/sessions/${encodeURIComponent(state.activeSessionId)}`, {
        method: "PATCH",
        body: { permission_mode: ev.target.value }
      });
      if (updated?.id) {
        const idx = state.sessions.findIndex((s) => s.id === updated.id);
        if (idx >= 0) state.sessions[idx] = updated;
        state.activeSessionMeta = updated;
      }
    } catch (err) {
      notifyError(err);
    }
  });

  $("session-context-meter")?.addEventListener("click", () => {
    compactActiveSession();
  });

  $("session-wt-diff-btn")?.addEventListener("click", () => openSessionWorktreeModal());
  $("session-wt-land-open-btn")?.addEventListener("click", () => openSessionWorktreeModal());
  $("confirm-wt-land-btn")?.addEventListener("click", () => landSessionWorktree());

  $("session-actions-menu-btn")?.addEventListener("click", (ev) => {
    ev.stopPropagation();
    $("session-actions-menu")?.classList.toggle("hidden");
  });

  $("session-actions-menu")?.addEventListener("click", async (ev) => {
    const item = ev.target.closest("[data-session-action]");
    if (!item) return;
    await handleSessionMenuAction(item.dataset.sessionAction);
  });

  $("move-session-form")?.addEventListener("submit", async (ev) => {
    ev.preventDefault();
    const targetWs = $("move-target-ws-select")?.value;
    if (!state.activeSessionId || !targetWs) return;
    try {
      await apiFetch(`/api/sessions/${encodeURIComponent(state.activeSessionId)}/move`, {
        method: "POST",
        body: { target_workspace_id: targetWs }
      });
      $("move-session-modal")?.classList.add("hidden");
      showToast(tr("会话已迁移至目标工作区", "Session moved"), "ok");
      await loadBootstrap();
      await selectWorkspace(targetWs);
    } catch (err) {
      notifyError(err, tr("迁移会话失败", "Failed to move session"));
    }
  });

  $("stop-run-btn")?.addEventListener("click", async () => {
    const activeRun = state.activeSessionId ? state.activeRunsBySession[state.activeSessionId] : null;
    if (activeRun?.run_id) {
      try {
        await apiFetch(`/api/runs/${encodeURIComponent(activeRun.run_id)}/stop`, { method: "POST", body: {} });
      } catch (err) {
        notifyError(err);
      }
    }
  });

  // Chat Timeline Actions
  $("chat-timeline")?.addEventListener("click", async (ev) => {
    const onboardCard = ev.target.closest("[data-onboarding-card]");
    if (onboardCard) {
      const kind = onboardCard.dataset.onboardingCard;
      const comp = $("composer-input");
      if (kind === "fleet") {
        setCenterMode("fleet");
        return;
      }
      if (comp) {
        if (kind === "arch") {
          comp.value = tr(
            "请梳理当前工作区的整体项目架构、核心包职责与关键调用链路，并指出最重要的入口文件。",
            "Explain the overall architecture, core modules, and key entry points of this workspace."
          );
        } else if (kind === "git") {
          comp.value = tr(
            "@git 请审查当前工作区的 Git 未提交变更，指出潜在 Bug、竞态条件或边界遗漏。",
            "@git Review uncommitted Git changes in this workspace and flag potential bugs or edge cases."
          );
        } else if (kind === "tdd") {
          comp.value = tr(
            "请分析当前项目的核心模块，为关键业务逻辑编写高覆盖率的单元测试并运行验证。",
            "Analyze the core modules and write comprehensive unit tests for critical business logic."
          );
        }
        comp.focus?.();
        scheduleDraftSave();
      }
      return;
    }
    if (ev.target.closest("[data-dismiss-run-error]")) {
      state.lastRunError = "";
      renderReloadBanner("");
      renderChatHistory();
      return;
    }
    const jumpSess = ev.target.closest("[data-jump-session]");
    if (jumpSess) {
      await selectSession(jumpSess.dataset.jumpSession);
      return;
    }
    const openFleet = ev.target.closest("[data-open-fleet-mission]");
    if (openFleet) {
      state.activeMissionId = openFleet.dataset.openFleetMission;
      setCenterMode("fleet");
      return;
    }
    const forkBtn = ev.target.closest("[data-fork-seq]");
    if (forkBtn) {
      await forkAtSeq(Number(forkBtn.dataset.forkSeq));
      return;
    }
    const rewBtn = ev.target.closest("[data-rewind-seq]");
    if (rewBtn) {
      await rewindToSeq(Number(rewBtn.dataset.rewindSeq));
    }
  });

  // Approval Banner Actions
  $("approval-banner-host")?.addEventListener("click", (ev) => {
    const btn = ev.target.closest("[data-appr-resolve]");
    if (btn && !btn.disabled) {
      resolveApproval(btn.dataset.apprResolve, btn.dataset.act);
    }
  });

  // Composer & Keyboard-Navigable Palette
  $("send-or-steer-btn")?.addEventListener("click", () => sendPromptOrSteer());
  $("composer-input")?.addEventListener("keydown", handleComposerKeydown);
  $("composer-input")?.addEventListener("input", () => {
    updateComposerPalette();
    scheduleDraftSave();
  });
  $("composer-palette")?.addEventListener("click", (ev) => {
    const opt = ev.target.closest("[data-palette-idx]");
    if (!opt) return;
    applyPaletteItem(Number(opt.dataset.paletteIdx));
  });

  // Right Inspector Tabs & Actions
  $$(".insp-tab").forEach((btn) => {
    btn.addEventListener("click", () => switchInspectorTab(btn.dataset.tab));
  });
  $("file-filter-input")?.addEventListener("input", () => renderFilesList());
  $("expand-all-files-btn")?.addEventListener("click", () => expandAllWorkspaceFolders());
  $("collapse-all-files-btn")?.addEventListener("click", () => collapseAllWorkspaceFolders());
  $("refresh-files-btn")?.addEventListener("click", () => {
    if (state.activeWorkspaceId) refreshWorkspaceInspector(state.activeWorkspaceId);
  });
  $("workspace-files-list")?.addEventListener("click", (ev) => {
    const citeBtn = ev.target.closest("[data-cite-file]");
    if (citeBtn) {
      ev.stopPropagation();
      citeFileInComposer(citeBtn.dataset.citeFile);
      return;
    }
    const dirRow = ev.target.closest("[data-toggle-dir]");
    if (dirRow) {
      const dirPath = dirRow.dataset.toggleDir;
      if (!(state.expandedTreeDirs instanceof Set)) state.expandedTreeDirs = new Set();
      if (state.expandedTreeDirs.has(dirPath)) state.expandedTreeDirs.delete(dirPath);
      else state.expandedTreeDirs.add(dirPath);
      renderFilesList();
      return;
    }
    const row = ev.target.closest("[data-open-file]");
    if (row) openFilePreview(row.dataset.openFile);
  });
  $("cite-file-in-chat-btn")?.addEventListener("click", () => {
    const rel = $("file-viewer-card")?.dataset.currentPath || "";
    if (rel) citeFileInComposer(rel);
  });
  $("close-file-viewer-btn")?.addEventListener("click", () => {
    $("file-viewer-card")?.classList.add("hidden");
  });
  $("safe-open-file-btn")?.addEventListener("click", () => {
    const rel = $("file-viewer-card")?.dataset.currentPath || "";
    safeOpenPath(rel);
  });
  $("snapshots-timeline-list")?.addEventListener("click", async (ev) => {
    const f = ev.target.closest("[data-fork-seq]");
    if (f) return forkAtSeq(Number(f.dataset.forkSeq));
    const r = ev.target.closest("[data-rewind-seq]");
    if (r) return rewindToSeq(Number(r.dataset.rewindSeq));
  });
  $("preview-artifact-select")?.addEventListener("change", (ev) => {
    selectArtifactByValue(ev.target.value);
  });
  $("safe-open-artifact-btn")?.addEventListener("click", () => {
    if (state.selectedArtifactItem?.path) {
      safeOpenPath(state.selectedArtifactItem.path);
    }
  });
  $("new-html-snippet-btn")?.addEventListener("click", () => {
    $("custom-preview-composer")?.classList.toggle("hidden");
  });
  $("mint-preview-btn")?.addEventListener("click", async () => {
    const title = $("custom-preview-title")?.value.trim() || "Sandbox Snippet";
    const html = $("custom-preview-html")?.value || "";
    if (!html.trim()) return;
    try {
      const pv = await apiFetch("/api/previews", {
        method: "POST",
        body: { workspace_id: state.activeWorkspaceId, title, html, save_name: title }
      });
      state.previews.unshift(pv);
      renderArtifactsInspector();
      selectArtifactByValue(pv.url);
      $("custom-preview-composer")?.classList.add("hidden");
      showToast(tr("沙箱预览已生成", "Sandboxed preview minted"), "ok");
    } catch (err) {
      notifyError(err, tr("生成沙箱预览失败", "Failed to mint preview"));
    }
  });
  $("inspector-context-content")?.addEventListener("click", async (ev) => {
    const skBtn = ev.target.closest("[data-insert-skill]");
    if (skBtn && $("composer-input")) {
      const comp = $("composer-input");
      comp.value = (comp.value ? comp.value + " " : "") + `$${skBtn.dataset.insertSkill} `;
      comp.focus?.();
      return;
    }
    if (ev.target.closest("#ctx-trust-ws-btn") && state.activeWorkspaceId) {
      try {
        await apiFetch(`/api/workspaces/${encodeURIComponent(state.activeWorkspaceId)}/trust`, { method: "POST", body: {} });
        await loadBootstrap();
        showToast(tr("已信任当前项目 .gorness", "Trusted workspace .gorness"), "ok");
      } catch (err) {
        notifyError(err);
      }
    }
  });

  // Unified Fleet Studio Events
  $("new-mission-modal-btn")?.addEventListener("click", () => {
    syncFleetLauncherWorkflows();
    renderTopologyCards();
    $("launch-fleet-form")?.classList.toggle("hidden");
  });
  $("close-fleet-launcher-btn")?.addEventListener("click", () => {
    $("launch-fleet-form")?.classList.add("hidden");
  });
  $("fleet-workflow-select")?.addEventListener("change", (ev) => {
    if ($("fleet-verify-input")) delete $("fleet-verify-input").dataset.userEdited;
    applyWorkflowDefaultsToLauncher(ev.target.value);
  });
  $("fleet-topology-select")?.addEventListener("change", () => {
    renderTopologyCards();
  });
  $("topology-cards-grid")?.addEventListener("click", (ev) => {
    const card = ev.target.closest("[data-topo-pick]");
    if (!card) return;
    if ($("fleet-topology-select")) $("fleet-topology-select").value = card.dataset.topoPick;
    renderTopologyCards();
  });
  $("fleet-verify-input")?.addEventListener("input", (ev) => {
    ev.target.dataset.userEdited = "1";
  });
  $("launch-fleet-form")?.addEventListener("click", (ev) => {
    const vChip = ev.target.closest("[data-verify-chip]");
    if (vChip && $("fleet-verify-input")) {
      $("fleet-verify-input").value = vChip.dataset.verifyChip;
      $("fleet-verify-input").dataset.userEdited = "1";
      return;
    }
    const gChip = ev.target.closest("[data-goal-preset]");
    if (gChip && $("fleet-goal-input")) {
      $("fleet-goal-input").value = gChip.dataset.goalPreset;
      $("fleet-goal-input").focus?.();
    }
  });
  $("launch-fleet-form")?.addEventListener("submit", async (ev) => {
    ev.preventDefault();
    if (!state.activeWorkspaceId) {
      populateWorkspaceSuggestions();
      $("workspace-modal")?.classList.remove("hidden");
      return;
    }
    const wfId = $("fleet-workflow-select")?.value || "";
    const topo = $("fleet-topology-select")?.value || "hybrid-fleet";
    const goal = ($("fleet-goal-input")?.value || "").trim();
    const verifyCmd = ($("fleet-verify-input")?.value || "").trim();
    const budgetUsd = Number($("fleet-budget-input")?.value || 0);
    const useWT = $("fleet-worktrees-cb")?.checked ?? true;
    if (!goal) return;

    try {
      const snap = await apiFetch("/api/fleet/missions", {
        method: "POST",
        body: {
          workspace_id: state.activeWorkspaceId,
          workflow_id: wfId,
          topology: topo,
          goal,
          verify_command: verifyCmd,
          budget_usd: budgetUsd,
          use_worktrees: useWT
        }
      });
      state.missions.unshift(snap);
      state.activeMissionId = snap.id;
      $("launch-fleet-form")?.classList.add("hidden");
      renderFleetStudio();
      subscribeMissionStream(snap.id);
      showToast(`${tr("多智能体舰队已启动", "Fleet mission launched")}: ${snap.topology}`, "ok");
    } catch (err) {
      notifyError(err, tr("启动舰队编排失败", "Failed to launch fleet mission"));
    }
  });

  $("mission-select")?.addEventListener("change", (ev) => {
    state.activeMissionId = ev.target.value;
    renderFleetStudio();
    ensureActiveMissionSSE();
  });
  $("stop-mission-btn")?.addEventListener("click", async () => {
    const snap = activeMission();
    if (snap) {
      try {
        await apiFetch(`/api/fleet/missions/${encodeURIComponent(snap.id)}/stop`, { method: "POST", body: {} });
        const fresh = await apiFetch(`/api/fleet/missions/${encodeURIComponent(snap.id)}`);
        const idx = state.missions.findIndex((m) => m.id === fresh.id);
        if (idx >= 0) state.missions[idx] = fresh;
        renderFleetStudio();
      } catch (err) {
        notifyError(err);
      }
    }
  });
  $("delete-mission-btn")?.addEventListener("click", () => deleteActiveMission());
  $("fleet-svg-canvas")?.addEventListener("click", (ev) => {
    const g = ev.target.closest("[data-fleet-node]");
    if (!g) return;
    state.selectedNodeId = g.dataset.fleetNode;
    const snap = activeMission();
    renderFleetSVG(snap);
    renderSelectedNodeDetails(snap, state.selectedNodeId, true);
  });
  $("open-node-session-btn")?.addEventListener("click", async (ev) => {
    const sid = ev.currentTarget?.dataset?.nodeSessionId;
    if (sid) {
      setCenterMode("chat");
      await selectSession(sid);
    }
  });
  $("node-steer-send-btn")?.addEventListener("click", () => steerSelectedNode());
  $("node-steer-input")?.addEventListener("keydown", (ev) => {
    if (ev.key === "Enter") {
      ev.preventDefault();
      steerSelectedNode();
    }
  });

  // Register Workspace Form
  $("register-ws-form")?.addEventListener("submit", async (ev) => {
    ev.preventDefault();
    const path = $("ws-path-input")?.value.trim();
    const name = $("ws-name-input")?.value.trim();
    if (!path) return;
    try {
      const ws = await apiFetch("/api/workspaces", { method: "POST", body: { path, name } });
      $("workspace-modal")?.classList.add("hidden");
      if ($("ws-path-input")) $("ws-path-input").value = "";
      if ($("ws-name-input")) $("ws-name-input").value = "";
      await loadBootstrap();
      await selectWorkspace(ws.id);
      showToast(`${tr("已挂载项目", "Mounted workspace")}: ${ws.name}`, "ok");
    } catch (err) {
      notifyError(err, tr("挂载项目失败", "Failed to register workspace"));
    }
  });

  // Close Modals & Drawers via [data-close] or backdrop click
  $$(".close-modal-btn").forEach((btn) => {
    btn.addEventListener("click", () => {
      const targetId = btn.dataset.close;
      if (targetId === "config-mode-view") closeSettingsModal();
      else if (targetId === "alwayson-drawer") closeAlwaysOnDrawer();
      else if (targetId) $(targetId)?.classList.add("hidden");
    });
  });

  ["config-mode-view", "alwayson-drawer", "workspace-modal", "session-worktree-modal", "move-session-modal"].forEach((id) => {
    $(id)?.addEventListener("click", (ev) => {
      if (ev.target.id === id) {
        if (id === "config-mode-view") closeSettingsModal();
        else if (id === "alwayson-drawer") closeAlwaysOnDrawer();
        else $(id)?.classList.add("hidden");
      }
    });
  });

  // Outside-click dismissal for search popover & session actions dropdown
  document.addEventListener("click", (ev) => {
    const menuWrap = $("session-menu-wrap");
    if (menuWrap && typeof menuWrap.contains === "function" && !menuWrap.contains(ev.target)) {
      $("session-actions-menu")?.classList.add("hidden");
    }
    const searchWrap = $("search-box-wrap");
    if (searchWrap && typeof searchWrap.contains === "function" && !searchWrap.contains(ev.target)) {
      $("search-results-popover")?.classList.add("hidden");
    }
    const compShell = $("composer-shell");
    if (compShell && typeof compShell.contains === "function" && !compShell.contains(ev.target)) {
      closeComposerPalette();
    }
  });

  // Global macOS Keyboard Shortcuts: ⌘K (Search), ⌘, (Settings), Esc (Dismiss Popovers/Modals)
  document.addEventListener("keydown", (ev) => {
    if ((ev.metaKey || ev.ctrlKey) && ev.key.toLowerCase() === "k") {
      ev.preventDefault();
      $("global-search-input")?.focus?.();
      return;
    }
    if ((ev.metaKey || ev.ctrlKey) && ev.key === ",") {
      ev.preventDefault();
      if (state.settingsOpen) closeSettingsModal();
      else openSettingsModal(state.configPage || "matrix");
      return;
    }
    if (ev.key === "Escape") {
      closeComposerPalette();
      $("search-results-popover")?.classList.add("hidden");
      $("session-actions-menu")?.classList.add("hidden");
      if (state.alwaysOnOpen) {
        closeAlwaysOnDrawer();
      } else if (state.settingsOpen) {
        closeSettingsModal();
      } else {
        ["workspace-modal", "session-worktree-modal", "move-session-modal"].forEach((id) => $(id)?.classList.add("hidden"));
      }
    }
  });

  bindConfigStudioEvents();
  bindAlwaysOnDrawerEvents();
}

// ============================================================================
// Native macOS WKWebView Bridges (cocoa_darwin.m NSOpenPanel & ⌘, Menu)
// ============================================================================

if (typeof globalThis.window !== "undefined") {
  globalThis.window.openGornessSettingsModal = (page = "matrix") => {
    openSettingsModal(page);
  };

  // Called by macOS native NSOpenPanel without reloading the WKWebView!
  globalThis.window.onNativeWorkspaceRegistered = async (wsPayload) => {
    try {
      const ws = typeof wsPayload === "string" ? JSON.parse(wsPayload) : wsPayload;
      $("workspace-modal")?.classList.add("hidden");
      await loadBootstrap();
      if (ws?.id) {
        await selectWorkspace(ws.id);
      }
      if (ws?.name) {
        showToast(`${tr("已挂载项目", "Mounted workspace")}: ${ws.name}`, "ok");
      }
    } catch (err) {
      notifyError(err, tr("同步原生挂载工作区失败", "Native workspace sync failed"));
    }
  };
}

// ============================================================================
// Boot & Module Exports
// ============================================================================

async function boot() {
  applyTheme(state.theme, false);
  applyLanguage(state.lang, false);
  initSplitters();
  renderTopologyCards();
  bindEvents();
  await loadBootstrap();
  connectGlobalEvents();
}

export {
  state,
  loadBootstrap,
  sendPromptOrSteer,
  setCenterMode,
  openSettingsModal,
  closeSettingsModal,
  openAlwaysOnDrawer,
  closeAlwaysOnDrawer,
  applyProviderPreset,
  applyMatrixScheme,
  applyMatrixPreset,
  patchAgentRole,
  patchAgentField,
  toggleRuleChip,
  toggleMCPPreset,
  escapeBidi,
  escHtml,
  esc,
  selectSession,
  renderChatHistory,
  handleRunStreamEvent,
  handleSlashCommand,
  handleComposerKeydown,
  updateComposerPalette,
  renderConfigStudio,
  getModelsForProvider,
  renderModelCatalogOptgroupsHTML,
  fetchProviderModels,
  confirmAndSaveCatalogModels,
  buildWorkspaceFileTree,
  updateSessionContextMeter,
  citeFileInComposer,
  renderMarkdownHTML
};

if (!globalThis.__GORNESS_TEST_NO_AUTOBOOT__) {
  boot();
}
