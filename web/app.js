// Gorness Apple HIG Workbench & Interactive Visual Multi-Agent SVG Studio
// Pure Vanilla ES Module · Zero Build Chain · Strict XSS + Bidi Neutralization

const SETTLE_GUARD_MS = 500;

const I18N = {
  zh: {
    brandSub: "macOS 纯 Go · 零数据库工作台",
    modeChat: "对话工作台",
    modeFleet: "多智能体编排画布",
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
    permLabel: "权限",
    modelOverridePh: "provider/model 覆盖",
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
    permLabel: "Perm",
    modelOverridePh: "provider/model override",
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

export function setCenterMode(mode) {
  state.centerMode = mode;
  document.getElementById("mode-chat-btn")?.classList.toggle("active", mode === "chat");
  document.getElementById("mode-fleet-btn")?.classList.toggle("active", mode === "fleet");
  document.getElementById("chat-mode-view")?.classList.toggle("hidden", mode !== "chat");
  document.getElementById("fleet-mode-view")?.classList.toggle("hidden", mode !== "fleet");
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
  if (!sel || !state.config || !state.config.agents) return;
  const currentVal = sel.value || "coder";
  const roles = Object.values(state.config.agents).sort((a, b) => a.id.localeCompare(b.id));
  sel.innerHTML = roles
    .map((a) => `<option value="${esc(a.id)}">${esc(a.id)} · ${esc(a.model)}</option>`)
    .join("");
  sel.value = roles.some((r) => r.id === currentVal) ? currentVal : "coder";
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

function renderAgentMatrixAndVaultEditor() {
  const rowsHost = document.getElementById("agent-matrix-rows");
  const fileSel = document.getElementById("vault-file-select");
  if (rowsHost && state.config && state.config.agents) {
    const agents = Object.values(state.config.agents).sort((a, b) => a.id.localeCompare(b.id));
    const provIds = Object.keys(state.config.providers || {}).sort();

    rowsHost.innerHTML = agents
      .map((a) => {
        const provOptions = provIds
          .map((pid) => `<option value="${esc(pid)}" ${pid === a.provider ? "selected" : ""}>${esc(pid)}</option>`)
          .join("");
        return `<div class="matrix-row" data-matrix-agent="${esc(a.id)}">
          <strong class="mono">${esc(a.id)}</strong>
          <div>
            <select class="mac-select" data-agent-prov="${esc(a.id)}" style="width:42%">${provOptions}</select>
            <input class="mac-input mono" data-agent-model="${esc(a.id)}" type="text" value="${esc(a.model)}" style="width:54%">
          </div>
          <select class="mac-select" data-agent-effort="${esc(a.id)}">
            ${["low", "medium", "high", "xhigh", "max"]
              .map((ef) => `<option value="${ef}" ${ef === (a.effort || "high") ? "selected" : ""}>${ef}</option>`)
              .join("")}
          </select>
          <button class="primary-xs-btn" type="button" data-save-agent="${esc(a.id)}">✓</button>
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

async function loadSelectedVaultConfigFile(relPath) {
  if (!relPath) return;
  try {
    const data = await apiJSON(`/api/config/files?rel=${encodeURIComponent(relPath)}`);
    const editor = document.getElementById("vault-file-editor");
    if (editor && document.activeElement !== editor) {
      editor.value = data.content || "";
    }
  } catch {
    // Ignore
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
  // Center Mode Switch
  document.getElementById("mode-chat-btn")?.addEventListener("click", () => setCenterMode("chat"));
  document.getElementById("mode-fleet-btn")?.addEventListener("click", () => setCenterMode("fleet"));

  // Theme & Bilingual Toggle
  document.querySelectorAll(".theme-btn").forEach((btn) => {
    btn.addEventListener("click", () => applyTheme(btn.dataset.themeVal || "auto", true));
  });
  document.getElementById("lang-toggle-btn")?.addEventListener("click", () => {
    state.lang = state.lang === "zh" ? "en" : "zh";
    localStorage.setItem("gorness.lang", state.lang);
    applyI18nInPlace();
    renderSidebarTree();
    renderApprovalBanners();
    renderSnapshotTimeMachine();
    renderFleetStudio();
    apiJSON("/api/config/global", {
      method: "POST",
      body: JSON.stringify({ language: state.lang })
    }).catch(() => {});
  });

  // Inspector Tabs
  document.querySelectorAll(".insp-tab").forEach((btn) => {
    btn.addEventListener("click", () => setInspectorTab(btn.dataset.tab || "files"));
  });
  document.getElementById("open-config-studio-btn")?.addEventListener("click", () => setInspectorTab("config"));

  // Modals open/close
  document.getElementById("add-workspace-btn")?.addEventListener("click", () => openModal("workspace-modal"));
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

  // Session Header Controls
  document.getElementById("session-title-input")?.addEventListener("change", async (e) => {
    if (!state.activeSessionId) return;
    await apiJSON(`/api/sessions/${encodeURIComponent(state.activeSessionId)}`, {
      method: "PATCH",
      body: JSON.stringify({ title: e.target.value })
    });
    await refreshActiveStatusQuietly();
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

  // Global Session Search (⌘K)
  const searchIn = document.getElementById("global-search-input");
  const searchPop = document.getElementById("search-results-popover");
  window.addEventListener("keydown", (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
      e.preventDefault();
      searchIn?.focus();
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

  // Always-On Modal Actions
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
    document.getElementById("timer-note-input").value = "";
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

  // Delegated Clicks (Sessions, Approvals, SVG Nodes, Palette, Time-Machine, Matrix)
  document.addEventListener("click", async (e) => {
    const target = e.target;
    if (!(target instanceof Element)) return;

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

    const saveAgentBtn = target.closest("[data-save-agent]");
    if (saveAgentBtn) {
      const id = saveAgentBtn.getAttribute("data-save-agent");
      const provider = document.querySelector(`[data-agent-prov="${CSS.escape(id)}"]`)?.value;
      const model = document.querySelector(`[data-agent-model="${CSS.escape(id)}"]`)?.value.trim();
      const effort = document.querySelector(`[data-agent-effort="${CSS.escape(id)}"]`)?.value;
      const snap = await apiJSON("/api/config/agents", {
        method: "POST",
        body: JSON.stringify({ id, provider, model, effort })
      });
      state.config = snap;
      populateAgentRoleSelect();
      return;
    }
  });

  // Approval Hunk Checkboxes
  document.addEventListener("change", (e) => {
    const target = e.target;
    if (!(target instanceof HTMLInputElement)) return;
    const apprId = target.getAttribute("data-appr-hunk");
    const hunkId = target.getAttribute("data-hunk-id");
    if (apprId && hunkId) {
      const set = state.approvalSelectedHunks.get(apprId) || new Set();
      if (target.checked) set.add(hunkId);
      else set.delete(hunkId);
      state.approvalSelectedHunks.set(apprId, set);
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
