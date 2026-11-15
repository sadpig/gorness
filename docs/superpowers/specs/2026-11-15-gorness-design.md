# Gorness 设计规约（macOS 纯 Go · 彻底零数据库 · TOML/MD/HTML + JSONL 全透明架构）

- **日期**：2026-11-15
- **项目路径**：`/Users/tanwei/Documents/MyProjectes/gorness`
- **运行平台**：macOS（`darwin/arm64` 原生专属）
- **核心架构准则**：
  1. **100% 纯 Go 语言生态**：`CGO_ENABLED=0` 单一静态二进制 `gorness`，不混编 Swift/Rust/C，不引入前端 `node_modules` 打包链。
  2. **彻底零数据库（Zero DB）**：拒绝 SQLite 等一切黑盒数据库文件；采用各司其职的全透明纯文件架构——**`TOML` 管结构化配置、`MD` 管角色/技能/记忆/协作黑板、`JSONL` 管追加式会话树与事件账本、`HTML` 管 Apple 工作台与可视化产物**，全量收敛于 `~/.gorness/` 隐藏目录。

---

## 1. 意图与定位（Intent & Why）

为在 Mac 上同时开发多个项目的工程师打造一个**架构极致纯粹、启动快如闪电、数据完全透明可读**的全功能 AI Agent Harness：
- 单一后台守护服务同时管理多个项目与并发会话，互不阻塞。
- 支持接入任意多个模型服务商，让不同职能的 Agent（主控规划、主力编码、异构盲审、极速搜索、空闲整理、内务起名）分别调用最匹配的模型协同作业。
- 内置可视化多 Agent 编排画布（含 Git Worktree 物理隔离与串行测试合入队列）与 Anthropic Always-On 休眠唤醒五大原语。
- 打开 `~/.gorness/` 没有任何二进制烂账，每一份配置、记忆、角色、会话流与可视化产物都能直接用编辑器查看、用 `ripgrep` 秒搜、用 Git 备份。

---

## 2. 目标、非目标与反目标

### 2.1 核心目标（Goals）
1. **多项目真正并行（Concurrent Multi-Workspace）**：单个 `gorness` 守护进程同时托管并并发驱动多个本地项目；每个项目独立持有规范路径守卫（解析 macOS `/var`、`/tmp` 等软链）、Seatbelt 沙箱目录边界、Git Worktree 池、按需 LSP 实例与项目级 `.gorness/` 局部覆盖；界面侧栏跨项目实时汇总运行态与待批卡片。
2. **完备的会话全生命周期管理（JSONL Session Tree + MD Export）**：基于追加式 `.jsonl` 树（每条记录带单调 `seq`、`id`、`parent_id`）实现 `O(1)` 零转义损耗流式落盘；支持多会话并发运行、轻量模型自动命名、手动改名、置顶、归档、删除、跨项目安全移动、草稿实时防丢、内存索引 + 并发扫描毫秒级跨项目搜索、运行中步边界中途指令注入（`Steer`）与排队追问、任意节点会话分叉（`/fork`）与回退（`/rewind`：可选仅回退对话指针，或按内容寻址快照同步还原被改动的源码文件）、锚定增量压缩（`/compact`）及一键导出为精美 `.md` / `.html` 归档。
3. **多模型服务商与异构 Agent 路由（Multi-Provider & Model-per-Agent）**：`providers.toml` 支持同时注册任意数量 Provider（Anthropic、OpenAI、Google Gemini、Apple GenAI / 企业内部网关、OpenRouter、DeepSeek、Qwen、Kimi、本地 Ollama / MLX Server，支持 `keychain:`、`env:` 与 `cmd:` 动态凭据）；`agents/*.md` 为每种 Agent 独立指定绑定的供应商、模型、推理强度（`effort`）、主备回退链与工具白名单。
4. **可视化多 Agent 编排引擎（Visual Multi-Agent Orchestration）**：原生内置 6 种协同拓扑（生成-验证对抗闭环、主从缓存共享扇出、独立 Git Worktree 常驻战队、强类型事件网格、Markdown 共享黑板拆树攻关、旗舰混合舰队），配齐防卡死巡检协程与串行变基测试合入队列；工作台内置 Apple 风格动态交互画布，实时展示各异构模型 Agent 的流向、Worktree 差异与黑板任务树，支持点击节点看日志/Diff 并中途注入 `Steer`。
5. **全文件驱动的 Always-On 常驻闭环**：基于文件锁与追加式 `JSONL` + `TOML` + `MD` 实现幂等 `Inbox`（去重 + 单写者文件租约 + 连发消息微批合并）、持久化 `Timers`（自设 `Follow-up` 闹钟 + `Cron` 周期调度）、四层指针型 `Memory` Markdown 库（含空闲期 `Dreaming` 自动提炼新 `SKILL.md`）、非阻塞 `Deferred` 挂起（保存断点立刻释放 Worker）与静默优先 `Outbox`（联动工作台、TUI 与 macOS 原生通知中心）。
6. **Apple HIG 图形工作台 + Charm 终端 TUI 双表面**：同一个 `gorness` 二进制同时提供 `gorness studio`（Apple 风格三栏图形工作台）与 `gorness`（极简终端 TUI）。

### 2.2 非目标与反目标（Non-Goals & Anti-Goals）
- **仅面向 macOS**：不写 Linux/Windows 兼容分支，直接调用 macOS 原生 `/usr/bin/sandbox-exec`（Seatbelt）、`/usr/bin/security`（钥匙串）、通知中心与 `launchd`。
- **四不铁律**：**零数据库**（严禁引入 SQLite 等任何 DB）、**零重型 Agent 框架**（严禁 LangChain 等）、**零源码向量库 RAG**、**零前端 npm 构建链**。

---

## 3. `~/.gorness/` 全透明文件架构（TOML / MD / HTML + JSONL 各司其职）

四种文件格式严格按各自最擅长的职责分工，配置与运行时目录清晰分离：

| 格式 | 承担职责 | 核心优势 |
| :--- | :--- | :--- |
| **`TOML`** | 全局/项目配置、多模型供应商目录、权限与沙箱规则、MCP 声明、编排拓扑参数、定时器清单 | 类型明确、层级清晰、无缩进陷阱，Go 解析极快 |
| **`MD`** | Agent 角色定义（TOML Frontmatter + 提示词）、`SKILL.md` 技能、Always-On 长期记忆、多 Agent 共享黑板任务卡与调查笔记 | 人与 LLM 原生直读，Obsidian/Git 完美兼容 |
| **`JSONL`** | 追加式会话事件树（`<session>.jsonl`）、Always-On 收发事件账本（`inbox.jsonl` / `outbox.jsonl`）、脱敏运行日志 | 追加 `O(1)` 永不写坏文件，原样保存复杂代码与签名 `thinking` 块零转义折腾 |
| **`HTML`** | `embed.FS` 内嵌 Apple HIG 工作台、多 Agent 编排可视化快照、Agent 生成的独立交互报告/图表产物（`artifacts/*.html`） | 浏览器/安全沙箱 `iframe` 零依赖直接渲染 |

### 3.1 目录全景结构

```text
~/.gorness/
├── config.toml                    # [TOML] 全局主配置（端口、亮暗主题、中英语言、默认模型槽位、沙箱/权限默认模式、并发上限）
├── providers.toml                 # [TOML] 多模型服务商与模型单价/窗口目录（支持 keychain: / env: / cmd: 凭据）
├── rules.toml                     # [TOML] 细粒度命令/路径权限规则（allow / ask / deny）与出站网络域名白名单
├── mcp.toml                       # [TOML] 全局 MCP 服务器声明（stdio / http / sse）
├── agents/                        # [MD] 异构模型 Agent 角色库（放一个 .md 即刻热注册新 Agent 类型）
│   ├── lead.md                    # 架构统筹主控（例：绑定 anthropic / claude-opus-5-5, effort = "xhigh"）
│   ├── coder.md                   # 主力研发编码（例：绑定 anthropic / claude-sonnet-5-5, effort = "high"）
│   ├── verifier.md                # 对抗盲审验证官（例：跨厂商绑定 openai / gpt-5.5 或 gemini, effort = "high"）
│   ├── explorer.md                # 并发只读侦察兵（例：绑定 haiku-5-5 或 local_mlx, effort = "low"）
│   ├── debugger.md                # 深度根因排障官（例：绑定 opus-5-5 / deepseek-reasoner, effort = "max"）
│   ├── dreamer.md                 # 空闲记忆整理与技能提炼员
│   └── utility.md                 # 会话自动起标题与增量摘要压缩员
├── workflows/                     # [TOML] 6 种多 Agent 编排拓扑模板（配置角色模型映射、并发度、Worktree 与验收门控命令）
│   ├── hybrid-fleet.toml          # 默认旗舰混合舰队
│   ├── review-loop.toml           # 生成-验证对抗闭环
│   ├── parallel-explore.toml      # 主从缓存共享并发探索
│   ├── worktree-team.toml         # 独立 Git Worktree 常驻专家战队
│   ├── event-pipeline.toml        # 强类型事件驱动流水线
│   └── shared-board.toml          # 共享黑板动态拆树协同
├── skills/                        # [MD] 用户级技能 (<skill>/SKILL.md)，自动兼容扫描 ~/.agents/skills 与 ~/.claude/skills
├── memory/                        # [MD] Always-On 全局四层指针记忆库
│   ├── INSTRUCTIONS.md            # 用户长期行为指令（每次唤醒固定纳入静态缓存前缀）
│   ├── INDEX.md                   # 记忆总目录索引
│   ├── REPORTED.md                # 已播报事项清单（防重复打扰）
│   └── notes/                     # 按需读取的细粒度记忆笔记 (*.md)
├── alwayson/                      # [TOML + JSONL + MD] 全文件驱动的常驻引擎队列
│   ├── timers.toml                # Follow-up 自设闹钟与 Cron 周期任务表
│   ├── inbox.jsonl                # 幂等事件收件箱追加账本（配合同目录 flock 消费游标与防抖微批合并）
│   ├── deferred/                  # 非阻塞挂起待批卡片 (<run-id>.md，含可读说明与精确恢复元数据头)
│   └── outbox.jsonl               # 受控发件箱投递记录
├── workspaces/                    # [多项目隔离存储区]
│   └── <project-slug-hash>/
│       ├── workspace.toml         # 项目路径、权限/沙箱覆盖、角色模型项目级覆盖、已信任项目配置 SHA-256 清单
│       ├── memory/                # 项目专属记忆与任务进度 (progress/*.md)
│       ├── sessions/              # 每个会话一个独立追加式 <session-id>.jsonl（头行元数据 + 单调 seq 事件树）
│       ├── missions/              # 多 Agent 编排现场 (<mission-id>/MISSION.md, tasks/*.md, notes/*.md, events.jsonl)
│       └── artifacts/             # Agent 生成的可视化方案、架构图与复盘快照 (*.html / *.md)
└── runtime/                       # [纯运行时缓存与锁，.gitignore 忽略]
    ├── snapshots/                 # SHA-256 内容寻址原文件备份块（支撑 /rewind 精确还原源码）
    ├── worktrees/                 # 多 Agent 并行改码自动创建与回收的临时 Git Worktree 池
    ├── daemon.sock                # 本机 CLI/TUI 极速通信 Unix Socket + daemon.pid (syscall.Flock)
    └── logs/                      # 结构化脱敏运行日志 (*.jsonl)
```

### 3.2 关键文件机制与并发安全保证
1. **无数据库下的高速查询与原子性**：
   - **内存轻量索引**：守护进程启动时并发扫描各项目 `sessions/*.jsonl` 首行元数据头（标题、置顶、更新时间、模型、分支指针），驻留内存 `sync.RWMutex` 索引表——千级会话列表渲染 `<1ms`，全文搜索直接并发流式匹配 `.jsonl` 与 `.md`，秒级返回。
   - **APFS 原子写与文件锁**：所有 `TOML` / `MD` 更新统一走“写同目录临时文件 → `Sync()` → `os.Rename()` 原子替换”；单会话互斥运行、`Inbox` 单写者消费与多 Agent `tasks/*.md` 抢单统一使用 macOS 内核级 `syscall.Flock` 非阻塞排他锁，进程崩溃内核自动释放锁，零死锁、零损坏。
2. **分层覆盖与 `fsnotify` 零重启热加载**：
   - 优先级：`单次运行指定` > `项目目录 <repo>/.gorness/*` > `用户全局 ~/.gorness/*` > `内置安全默认值`。
   - 用编辑器修改任何 `*.toml`、`agents/*.md`、`workflows/*.toml` 或 `SKILL.md` 保存后 100ms 内热加载生效；未按 SHA-256 哈希显式信任的外部克隆仓库 `.gorness/mcp.toml` 或命令钩子绝不自动执行。

---

## 4. 已拍板的五项工程边界规范（架构师默认决策）

1. **多 Agent 编排交互模式**：采用 **`workflows/*.toml` 声明式参数化配置 + 实时交互式动态 SVG 拓扑监控台**（内置 6 个确定性 Go 编排协调器；画布实时展示每个异构模型 Agent 的工作流脉冲、Git Worktree 差异与 `missions/<id>/tasks/*.md` 任务树生长，支持点击节点看实时流/Diff 并注入 `Steer` 指令，不搞臃肿低代码自由连线器）。
2. **默认权限姿态与 Seatbelt 沙箱域名代理**：
   - 默认 `auto` 模式：项目目录内文件修改（自动写 `runtime/snapshots/` 可随时 `/rewind`）与常规沙箱内开发命令顺畅执行；越出项目目录、改动自执行配置文件（`.gorness/`、`.mcp`、Shell 启动文件）、高危命令或外部发布动作触发 **500ms 防连点审批卡**（文件改动支持逐 Diff 块勾选部分批准；常驻后台无人值守自动写入 `alwayson/deferred/<id>.md` 非阻塞挂起并释放协程）。
   - macOS `/usr/bin/sandbox-exec` 强制限制文件写范围并封堵 AppleEvents/钥匙串读取；配置了域名白名单时，由 Go 内置本地轻量 HTTP CONNECT 出站代理配合 Seatbelt 仅放行本地代理端口实现精确域名过滤。
3. **Apple GenAI 与全模型凭据支持**：`providers.toml` 原生支持四大线协议（`anthropic`、`openai-responses`、`openai-compat`、`gemini`）；凭据支持 `"keychain:..."`（调 `/usr/bin/security`）、`"env:..."` 及全局受信任配置专属的 `"cmd:..."`（动态执行 SSO/CLI 令牌命令并按 TTL 内存缓存）与自定义 `headers`。
4. **Always-On 本机安全闭环**：守护进程严格只监听 `127.0.0.1` 与 `daemon.sock`；`Inbox` 接收本地 Webhook/脚本/定时器事件，`Outbox` 联动推送 Apple 工作台、终端 TUI 与 **macOS 原生通知中心**。
5. **同项目并发与快照还原边界**：同项目多会话默认共享主目录并支持一键勾选为单会话隔离 Git Worktree；多 Agent 并发写战队强制自动分配独立 Git Worktree。文件工具涵盖创建/修改/删除并由 `snapshots/` 实现 `O(1)` 精确还原；`run_command` 前后附带轻量 `git status` 变更提示。

---

## 5. 纯粹 Go 仓库结构（全仓仅 8 个内部包，零数据库依赖）

外部依赖克制到极致：**仅用 `pelletier/go-toml/v2`、`fsnotify/fsnotify`、`robfig/cron/v3`、`charmbracelet/bubbletea` + `lipgloss`**，其余 100% Go 标准库自研！

```text
gorness/
├── go.mod                     # module gorness.dev/gorness (CGO_ENABLED=0, 零数据库依赖)
├── cmd/gorness/               # 唯一可执行文件：gorness (TUI) | gorness studio | gorness run | gorness daemon
├── internal/
│   ├── config/                # ~/.gorness/ 分层加载、TOML & MD Frontmatter 解析、Keychain/cmd 凭据、fsnotify 热更新、脱敏器
│   ├── store/                 # 纯文件存储引擎：JSONL 会话树读写/分叉/回退、内存高速索引、Flock 原子队列、SHA-256 快照库
│   ├── llm/                   # 四大原生流式协议客户端、静态前缀缓存分割、1.5x 超时重试、异构 Agent 角色模型路由器
│   ├── tools/                 # 5 核心内置工具 (read/edit/write/bash/search) + 逐块 Diff 合并 + Seatbelt & 域名代理 + MCP + Skills
│   ├── core/                  # 微内核 Hook 总线、多项目并发会话调度器、AgentLoop、步边界 Steer、锚定增量压缩
│   ├── fleet/                 # 6 种多 Agent 编排协调器、Git Worktree 池、MD 共享黑板抢单、防僵死巡检与串行变基合入队列
│   ├── alwayson/              # Inbox JSONL 单活消费与防抖微批、Cron/Follow-up 定时器、指针记忆与 Dreamer、Deferred 恢复、Outbox
│   ├── server/                # 本机同源守卫 HTTP + 4s 心跳 SSE 服务、无 allow-same-origin 隔离 HTML 预览壳、embed.FS 挂载
│   └── tui/                   # Charm Bubble Tea + Lip Gloss 多项目多会话终端交互界面
└── web/                       # embed.FS 内嵌的零构建链 Apple HIG 三栏工作台与 SVG 可视化多 Agent 编排中心
```

---

## 6. 留给工程实现阶段的内部细节

- 各 Go 内部包私有文件组织、`gorness studio` 在 Mac 上优先唤起独立无地址栏 App 模式窗口并优雅回退默认浏览器的启动细节、以及 Apple HIG 亮暗自适应样式微调。
