# Gorness 1.0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the complete Gorness 1.0 macOS-native AI agent harness as a single `CGO_ENABLED=0` Go binary (`gorness`) backed by a zero-database `~/.gorness/` file vault (`TOML / MD / HTML + JSONL`), supporting concurrent multi-project workspaces, full session lifecycle, multi-provider heterogeneous model-per-agent routing, visual multi-agent fleet orchestration with Git worktrees, Always-On sleep-wake primitives, Apple HIG web workbench, and Charm Bubble Tea TUI.

**Architecture:** One pure-Go binary (`cmd/gorness`) serves both as the local loopback/Unix-socket background daemon (`gorness daemon` / auto-spawned) and as all user surfaces (`gorness` TUI, `gorness studio` embedded Apple HIG three-pane web workbench with live SVG multi-agent canvas, `gorness run` headless CLI, and `gorness acp` JSON-RPC server). All configuration, agent profiles, skills, memory, workflows, sessions, Always-On queues, and fleet blackboards live as transparent `TOML`, `MD`, `JSONL`, and `HTML` files inside `~/.gorness/` (with project-local `.gorness/` overlays), coordinated in-process via Go goroutines, `sync.RWMutex` memory indexes, APFS atomic temp-file renames, and `syscall.Flock` locks.

**Tech Stack:** Go 1.27 (`CGO_ENABLED=0`, `darwin/arm64`), `github.com/pelletier/go-toml/v2`, `github.com/fsnotify/fsnotify`, `github.com/robfig/cron/v3`, `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, Go standard library (`net/http`, `embed`, `syscall`, `crypto/sha256`, `os/exec` driving macOS `/usr/bin/sandbox-exec`, `/usr/bin/security`, and `osascript`), vanilla ES Module + CSS Variables + SVG (`web/`).

**Spec:** `docs/superpowers/specs/2026-11-15-gorness-design.md`

## Global Constraints

- **Platform:** `darwin/arm64` macOS native only; build with `CGO_ENABLED=0`.
- **Zero Database:** Strictly no SQLite, BoltDB, Badger, Postgres, or any binary DB engine. Only `TOML`, `MD`, `JSONL`, `HTML`, and SHA-256 content-addressed snapshot files under `~/.gorness/`.
- **Dependency Ceiling:** Only `github.com/pelletier/go-toml/v2`, `github.com/fsnotify/fsnotify`, `github.com/robfig/cron/v3`, `github.com/charmbracelet/bubbletea`, and `github.com/charmbracelet/lipgloss` (plus their transitive Go subpackages). Zero agentic frameworks (no LangChain/Eino), zero code vector DBs, zero `node_modules` frontend bundlers.
- **Language & Copy Convention:** User-facing UI strings and setting labels carry `[中文, English]` bilingual pairs; Go identifiers and commit messages are English (conventional commits with scope); critical invariant comments are Chinese; built-in LLM prompts, tool schemas, and tool error messages are concise English.
- **Test Isolation:** Every Go test injects a temporary `VaultDir` (`t.TempDir()`) and fake `UserHomeDir` + injectable external runners (`LLMStreamer`, `CommandRunner`, `KeychainStore`, `Notifier`)—never reading or mutating the developer's real `~/.gorness/`, real macOS Keychain, or real Notification Center. All packages pass `CGO_ENABLED=0 go test -race ./...`.

## Review Focus

1. **macOS `/var` & `/tmp` Symlink Aliasing vs. Out-of-Root Symlink Escape:** Workspaces inside `/var/folders/...` (symlinked to `/private/var/folders/...`) must allow legitimate in-workspace file operations while rejecting in-workspace symlinks pointing outside the canonical workspace root or into `~/.gorness/` (pinned in Task 4 `TestPathGuardMacOSSymlinksAndEscape`).
2. **Malformed TOML or Frontmatter Saved Mid-Edit (`fsnotify`):** Saving syntactically invalid `providers.toml` or `agents/*.md` while editing must keep the previous valid in-memory `Snapshot` active, surface a structured `ReloadDiagnostic` with file path, and recover cleanly on the next valid save without dropping active runs (pinned in Task 1 `TestWatcherPreservesValidSnapshotOnSyntaxError`).
3. **Partial Hunk Approval with Zero Selected Hunks or Stale External IDE Edit:** Approving a file edit with zero `hunk_ids` selected must leave disk untouched and return `written: false` to the model; if an external editor modified the file after `read_file`, SHA-256 precondition verification must abort before writing and preserve the external edit (pinned in Task 4 `TestEditHunksZeroSelectionAndStalePrecondition`).
4. **Rapid Double-Approval (`<500ms` Settle Guard) & Cross-Origin / Preview XSS Escape:** Resolving a newly surfaced approval within `500ms` must fail with `ErrApprovalNotSettled` keeping it pending; cross-origin `POST` requests to the loopback daemon must be blocked with `403`, and `/preview/:token` must emit strict CSP (`connect-src 'none'; frame-src 'none'`) inside a sandboxed iframe without `allow-same-origin` while neutralizing HTML/Bidi characters in logs (pinned in Task 5 `TestApprovalBrokerSettleGuard` and Task 8 `TestServerOriginGuardAndIsolatedPreviewCSP`).
5. **Daemon Crash (`SIGKILL`) Leaving Stale `running` Inbox Events:** Restarting `Inbox` when `inbox.jsonl` has unfinished `running` items must re-queue them up to `MaxAttempts = 3` (then mark `failed`) while rejecting duplicate `(source, source_id)` deliveries (pinned in Task 6 `TestInboxRecoverAndIdempotency`).

---

### Task 1: Module Bootstrap, `~/.gorness/` Vault Skeleton & Hot-Reloading Config Engine

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `internal/config/types.go`
- Create: `internal/config/frontmatter.go`
- Create: `internal/config/secrets.go`
- Create: `internal/config/redact.go`
- Create: `internal/config/loader.go`
- Create: `internal/config/watcher.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: Go stdlib, `github.com/pelletier/go-toml/v2`, `github.com/fsnotify/fsnotify`.
- Produces:
  - `type VaultPaths struct { Root string }` with helpers `ConfigFile()`, `ProvidersFile()`, `RulesFile()`, `MCPFile()`, `AgentsDir()`, `WorkflowsDir()`, `SkillsDir()`, `MemoryDir()`, `AlwaysOnDir()`, `WorkspacesDir()`, `SnapshotsDir()`, `WorktreesDir()`, `SocketPath()`, `LogsDir()`.
  - `type GlobalConfig`, `type ProviderConfig`, `type ModelSpec`, `type AgentProfile`, `type WorkflowTemplate`, `type RuleSet`, `type MCPConfig`, `type Snapshot`, `type ReloadDiagnostic`.
  - `type SecretResolver interface { Resolve(ctx context.Context, ref string, allowCmd bool) (string, error) }` supporting `"keychain:<service>"`, `"env:<VAR>"`, `"cmd:<shell>"` (cached with TTL, rejected when `allowCmd == false`), and literal values (tightens source file to `0600`).
  - `func Redact(text string, secrets []string) string` and `func EscapeBidi(text string) string`.
  - `func ParseFrontmatterMD(raw []byte) (meta map[string]any, body string, err error)` supporting `+++` TOML and `---` key-value/TOML headers.
  - `type Manager struct` with `NewManager(opts Options) (*Manager, error)` (bootstraps `~/.gorness/` `0700` with default `.toml` configs, 7 default `agents/*.md`, 6 default `workflows/*.toml`, and `memory/{INSTRUCTIONS,INDEX,REPORTED}.md`), `Current() *Snapshot`, `ForWorkspace(wsRoot string, trustedHashes map[string]string) (*Snapshot, []string, error)`, `Watch(ctx context.Context, onChange func(*Snapshot, *ReloadDiagnostic)) error`.

- [ ] **Step 1: Initialize Go module and write failing config/vault tests in `internal/config/config_test.go`**

Initialize `go.mod` (`module gorness.dev/gorness`, Go 1.27) and `.gitignore` (`dist/`, `gorness`, `.DS_Store`), pull the 5 approved Go dependencies, and write tests in `internal/config/config_test.go`:
- `TestBootstrapVaultCreatesDefaultFilesAndPermissions`: verifies `0700` dir, `config.toml`, `providers.toml`, `rules.toml`, `mcp.toml`, all 7 `agents/{lead,coder,verifier,explorer,debugger,dreamer,utility}.md`, all 6 `workflows/{hybrid-fleet,review-loop,parallel-explore,worktree-team,event-pipeline,shared-board}.toml`, and `memory/{INSTRUCTIONS,INDEX,REPORTED}.md`.
- `TestWorkspaceOverlayAndUntrustedConfigGating`: verifies `<ws>/.gorness/agents/coder.md` overrides global `coder.md`, while `<ws>/.gorness/mcp.toml` or `cmd:` credential in workspace overlay is NOT activated unless its SHA-256 hash is in `trustedHashes` (`cmd:` in project overlay is always rejected).
- `TestSecretResolverAndRedaction`: tests `env:`, injected fake `KeychainStore`, TTL-cached `cmd:`, `Redact` scrubbing bearer tokens/keys, and `EscapeBidi` turning `\u202e` into `<U+202E>`.
- `TestWatcherPreservesValidSnapshotOnSyntaxError`: starts `Watch`, writes broken TOML to `providers.toml`, asserts `Current()` retains previous valid providers and emits non-nil `*ReloadDiagnostic`, then writes valid TOML with a new provider and asserts `Current()` updates within 300ms.

- [ ] **Step 2: Run tests to verify failure**

Run: `CGO_ENABLED=0 go test -race ./internal/config/...`
Expected: FAIL (missing `internal/config` implementation files).

- [ ] **Step 3: Implement `internal/config/{types,frontmatter,secrets,redact,loader,watcher}.go`**

Implement the exact types and functions in `Interfaces`:
- `Bootstrap` writes clean, commented default TOML and Markdown files only if missing (`os.OpenFile` with `O_CREATE|O_EXCL`).
- `SecretResolver` invokes `/usr/bin/security find-generic-password -s <name> -w` by default when no test `KeychainFunc` is injected.
- `Watcher` uses `fsnotify` with a `80ms` debounce timer watching `Root`, `AgentsDir`, `WorkflowsDir`, and `SkillsDir`.

- [ ] **Step 4: Run tests with race detector to verify pass**

Run: `CGO_ENABLED=0 go test -v -race ./internal/config/...`
Expected: PASS (all 4 test functions green, 0 data races).

- [ ] **Step 5: Commit Task 1**

```bash
git add go.mod go.sum .gitignore internal/config
git commit -m "feat(config): bootstrap ~/.gorness zero-DB vault and hot-reloading config manager"
```

---

### Task 2: Zero-DB File Store — Atomic I/O, Snapshot Blobs, Workspaces & JSONL Session Tree

**Files:**
- Create: `internal/store/atomic.go`
- Create: `internal/store/snapshots.go`
- Create: `internal/store/workspaces.go`
- Create: `internal/store/sessions.go`
- Create: `internal/store/search.go`
- Test: `internal/store/store_test.go`

**Interfaces:**
- Consumes: `internal/config.VaultPaths`.
- Produces:
  - `func AtomicWriteFile(path string, data []byte, perm os.FileMode) error` and `func WithFlock(lockPath string, exclusive bool, fn func() error) error` / `func TryFlock(lockPath string) (unlock func(), ok bool, err error)`.
  - `type ChangeKind string` (`ChangeCreate`, `ChangeModify`, `ChangeDelete`), `type FileSnapshotRecord struct { Seq uint64; SessionID, RelPath string; Kind ChangeKind; BeforeHash, AfterHash string }`.
  - `type SnapshotStore struct` with `PutBlob(data []byte) (sha256Hex string, err error)`, `CaptureFile(absPath string) (sha256Hex string, exists bool, err error)`, `RecordStepChange(rec FileSnapshotRecord) error`, `RestoreSessionAfterSeq(sessionID string, targetSeq uint64, wsRoot string) ([]string, error)`.
  - `type Workspace struct { ID, Name, RootPath string; PermMode, SandboxMode string; RoleModels map[string]string; TrustedHashes map[string]string; UpdatedAt time.Time }` and `type WorkspaceStore struct` (`Register(path, name string) (*Workspace, error)` canonicalizing via `filepath.EvalSymlinks`, `List()`, `Get(id)`, `Update(id, mutFn)`).
  - `type ContentBlock struct { Type string; Text string; Thinking string; Signature string; ToolCallID, ToolName string; ToolInput json.RawMessage; IsError bool; CompactionData json.RawMessage }`.
  - `type SessionEntry struct { Seq uint64; ID, ParentID, Role, ModelID, AgentRole string; Blocks []ContentBlock; InputTokens, OutputTokens, CacheReadTokens int; CostUSD float64; CreatedAt time.Time }`.
  - `type SessionMeta struct { ID, WorkspaceID, ParentSessionID string; ForkedAtSeq uint64; HeadEntryID string; HeadSeq uint64; Title, AgentRole, ModelOverride, Draft string; WorktreePath string; Pinned, Archived bool; TotalTokens int; TotalCostUSD float64; CreatedAt, UpdatedAt time.Time }`.
  - `type SessionStore struct` with `NextSeq() (uint64, error)`, `Create(wsID, title, agentRole string) (*SessionMeta, error)`, `List(wsID string, includeArchived bool) []*SessionMeta`, `GetMeta(id string) (*SessionMeta, error)`, `Append(sessionID string, entry SessionEntry) (*SessionEntry, error)`, `LoadActiveBranch(sessionID string) ([]SessionEntry, error)`, `Fork(sessionID string, atSeq uint64, newTitle string) (*SessionMeta, error)`, `Rewind(sessionID string, toSeq uint64, restoreFiles bool, snaps *SnapshotStore, wsRoot string) (*SessionMeta, []string, error)`, `MoveWorkspace(sessionID, targetWsID string) (*SessionMeta, error)`, `UpdateMeta(sessionID string, fn func(*SessionMeta)) (*SessionMeta, error)`, `Delete(sessionID string) error`, `Search(query string, limit int) ([]SearchHit, error)`, `ExportMarkdown(sessionID string) (string, error)`, `ExportHTML(sessionID string) (string, error)`.

- [ ] **Step 1: Write failing zero-DB store tests in `internal/store/store_test.go`**

Write tests covering:
- `TestAtomicWriteAndFlockConcurrency`: 20 goroutines contending on `WithFlock` + `NextSeq()` produce strictly monotonic unique `1..20` sequences with zero torn files.
- `TestSessionJSONLTreeForkAndRewindWithFileSnapshots`: creates a workspace and session, records a `ChangeCreate` (`a.go`) at `seq=2` and `ChangeModify` (`b.go`) at `seq=4`, forks at `seq=2` verifying branch isolation, then calls `Rewind(id, 1, true, snaps, wsRoot)` verifying `a.go` is removed, `b.go` reverts to exact pre-edit bytes, and `LoadActiveBranch` stops at `seq=1` while preserving signed `Thinking` and `CompactionData` blocks byte-for-byte.
- `TestCrossWorkspaceSearchAndExports`: indexes sessions across 2 workspaces (Chinese + English + code identifiers), verifies `Search` ranks title/body hits accurately, and checks `ExportMarkdown` / `ExportHTML` self-contained output.

- [ ] **Step 2: Run tests to verify failure**

Run: `CGO_ENABLED=0 go test -race ./internal/store/...`
Expected: FAIL (missing `internal/store` implementation).

- [ ] **Step 3: Implement `internal/store/{atomic,snapshots,workspaces,sessions,search}.go`**

Implement the zero-DB file store:
- Each session lives at `~/.gorness/workspaces/<wsID>/sessions/<sessionID>.jsonl` accompanied by `<sessionID>.meta.toml` (atomic rename for instant metadata updates like drafts/pins/head pointer without rewriting the append-only `.jsonl` event log!).
- `LoadActiveBranch` walks `ParentID` pointers from `Meta.HeadEntryID` back to root, making `/fork` and `/rewind` $O(1)$ pointer updates plus an audit record line appended to `.jsonl`.

- [ ] **Step 4: Run tests with race detector to verify pass**

Run: `CGO_ENABLED=0 go test -v -race ./internal/store/...`
Expected: PASS.

- [ ] **Step 5: Commit Task 2**

```bash
git add internal/store
git commit -m "feat(store): implement zero-DB JSONL session tree, flock primitives, and SHA-256 snapshot store"
```

---

### Task 3: Multi-Provider Native Streaming LLM Layer & Heterogeneous Role Router

**Files:**
- Create: `internal/llm/types.go`
- Create: `internal/llm/sse.go`
- Create: `internal/llm/anthropic.go`
- Create: `internal/llm/openai.go`
- Create: `internal/llm/gemini.go`
- Create: `internal/llm/prompt.go`
- Create: `internal/llm/router.go`
- Test: `internal/llm/llm_test.go`

**Interfaces:**
- Consumes: `internal/config`, `internal/store.ContentBlock`, `internal/store.SessionEntry`.
- Produces:
  - `type ToolSchema struct { Name, Description string; InputSchema json.RawMessage; DeferLoad bool }`.
  - `type StreamEventType string` (`StreamStepReset`, `StreamThinkingDelta`, `StreamTextDelta`, `StreamToolCall`, `StreamCompaction`, `StreamRetry`, `StreamDone`, `StreamError`).
  - `type StreamEvent struct { Type StreamEventType; Text, Thinking, Signature string; ToolCallID, ToolName string; ToolInput json.RawMessage; Compaction json.RawMessage; StopReason string; InputTokens, OutputTokens, CacheReadTokens int; Attempt int; Err error }`.
  - `type Request struct { Provider config.ProviderConfig; APIKey string; Model string; Effort string; StaticSystem string; DynamicReminders []string; History []store.SessionEntry; Tools []ToolSchema }`.
  - `type Streamer interface { Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) }`.
  - `func AssemblePrompt(staticParts []string, tools []ToolSchema, history []store.SessionEntry, dynamicReminders []string) (sortedTools []ToolSchema, frozenSystem string, augmentedHistory []store.SessionEntry)` — sorts tools deterministically by `Name` and injects `<system-reminder>` blocks onto the trailing user/tool message without mutating `frozenSystem`.
  - `type ResolvedRoute struct { RoleID, ProviderID, ModelID, Effort string; Provider config.ProviderConfig; Spec config.ModelSpec; Fallback *ResolvedRoute }`.
  - `type Router struct` with `NewRouter(cfg *config.Manager, secrets config.SecretResolver, transport Streamer) *Router`, `Resolve(roleID, wsModelOverride, runModelOverride string, snap *config.Snapshot) (ResolvedRoute, error)`, `StreamWithRetry(ctx context.Context, route ResolvedRoute, req Request) (<-chan StreamEvent, error)` (implements `1.5x` timeout scaling up to 2 retries, emits `StreamStepReset` if partial chunks streamed before drop, then fails over to `route.Fallback`).

- [ ] **Step 1: Write failing multi-provider SSE & heterogeneous routing tests in `internal/llm/llm_test.go`**

Use `httptest.NewServer` mock SSE endpoints to test:
- `TestAnthropicSSEAdaptiveThinkingAndPreservedBlocks`: verifies wire request sends `thinking: {"type":"adaptive"}`, `output_config: {"effort":"high"}`, `cache_control` on static system & last sorted tool, never sends `budget_tokens`, parses `thinking_delta` + `signature_delta` + `input_json_delta` tool calls + `refusal` stop reason, and replays prior `thinking` blocks unchanged on subsequent turns.
- `TestOpenAIResponsesCompatAndGeminiAdapters`: verifies SSE streaming and tool-call assembly across `openai-responses`, `openai-compat` (custom headers + Apple GenAI/Ollama shapes), and `gemini`.
- `TestRoleRouterPriorityAndMidStreamRetryFailover`: verifies `runOverride > roleProfile (lead->opus, verifier->gpt, explorer->mlx) > wsOverride > globalDefault`, simulates a mid-stream disconnect after emitting partial text, asserts `StreamStepReset` + `StreamRetry` are emitted before clean failover to fallback model.
- `TestPromptAssemblerPrefixDeterminism`: verifies `frozenSystem` and `sortedTools` byte representation stay identical across turns when timestamps or `Steer` reminders change.

- [ ] **Step 2: Run tests to verify failure**

Run: `CGO_ENABLED=0 go test -race ./internal/llm/...`
Expected: FAIL.

- [ ] **Step 3: Implement `internal/llm/{types,sse,anthropic,openai,gemini,prompt,router}.go`**

Implement the 4 wire adapters over zero-dependency `bufio.Scanner` SSE reader (expanded buffer to `4MB` per event line so large tool inputs never truncate), `AssemblePrompt`, and `Router`.

- [ ] **Step 4: Run tests with race detector to verify pass**

Run: `CGO_ENABLED=0 go test -v -race ./internal/llm/...`
Expected: PASS.

- [ ] **Step 5: Commit Task 3**

```bash
git add internal/llm
git commit -m "feat(llm): add 4 native streaming adapters, cache-stable prompt assembler, and role router"
```

---

### Task 4: 5 Core Tools, Partial Hunk Diff Merger, macOS Seatbelt, Skills & MCP Bridge

**Files:**
- Create: `internal/tools/registry.go`
- Create: `internal/tools/path_guard.go`
- Create: `internal/tools/diff_hunks.go`
- Create: `internal/tools/builtin_fs.go`
- Create: `internal/tools/builtin_search.go`
- Create: `internal/tools/seatbelt.go`
- Create: `internal/tools/egress_proxy.go`
- Create: `internal/tools/skills.go`
- Create: `internal/tools/mcp_client.go`
- Test: `internal/tools/tools_test.go`

**Interfaces:**
- Consumes: `internal/config`, `internal/store`, `internal/llm.ToolSchema`.
- Produces:
  - `type DiffHunk struct { ID, Fingerprint, Header string; OldStart, OldLines, NewStart, NewLines int; Preview string }`.
  - `type StagedEdit struct { AbsPath, RelPath string; Kind store.ChangeKind; BeforeBytes, ProposedBytes []byte; PreconditionHash string; Hunks []DiffHunk }`.
  - `func ComputeStagedEdit(absPath, relPath, oldStr, newStr string, replaceAll bool, readHash string) (*StagedEdit, error)` (4-stage match cascade: exact -> trailing-space normalized -> typographic quote/dash normalized -> relative indentation) and `func ApplySelectedHunks(staged *StagedEdit, selectedHunkIDs []string, snaps *store.SnapshotStore, sessionID string, seq uint64) (written bool, applied int, err error)`.
  - `type PathGuard struct` (`NewPathGuard(wsRoot, vaultRoot string) (*PathGuard, error)`, `ResolveInWorkspace(target string) (canonicalAbs, rel string, inWorkspace bool, err error)` blocking `~/.gorness` secret paths and resolving parent/target symlinks accurately on macOS).
  - `type SandboxMode string` (`SandboxStrict`, `SandboxReadOnly`, `SandboxNetworkAllow`, `SandboxOff`), `type SeatbeltRunner struct` (`BuildProfile(wsRoot string, extraWriteDirs []string, mode SandboxMode, proxyPort int) string`, `Run(ctx context.Context, cmdStr, workDir string, mode SandboxMode, allowedDomains []string) (CommandResult, error)` managing process group `Setpgid: true` and embedded `EgressProxy`).
  - `func WrapUntrustedContent(source, body string) string` (defangs closing `</untrusted_content>` tags and truncates oversized outputs while saving addressable overflow to `runtime/logs/`).
  - `type SkillCatalog struct` (`Discover(snap *config.Snapshot, wsRoot string) *SkillCatalog`, `ActiveForPaths(touchedPaths []string) []Skill`, `XMLSummary() string`).
  - `type Registry struct` exposing the 5 core tools (`read_file`, `edit_file`, `write_file`, `run_command`, `search_code`), dynamic `activate_skill`, MCP stdio/HTTP tools (`MCPManager`), and automatic BM25 `search_tools` folding when total tools exceed 12 (`VisibleSchemas(activated map[string]bool) []llm.ToolSchema`).

- [ ] **Step 1: Write failing tools, symlink, hunk-diff, Seatbelt & BM25 tests in `internal/tools/tools_test.go`**

Write tests covering:
- `TestPathGuardMacOSSymlinksAndEscape`: creates workspace under `os.MkdirTemp("", "gorness-ws-*")` (macOS `/var/folders/...` symlink), verifies normal inside paths and non-existent new child files resolve cleanly as `inWorkspace == true`, while an inside symlink pointing to `/etc/hosts` or `vaultRoot/providers.toml` resolves as `inWorkspace == false` (and vault secret path errors outright).
- `TestEditHunksZeroSelectionAndStalePrecondition`: stages a 2-hunk edit, verifies `ApplySelectedHunks` with `[]string{}` returns `written == false` without touching disk, verifies selecting only hunk #1 writes hunk #1 while leaving hunk #2 untouched, and verifies external file mutation after `read_file` fails with `ErrStalePrecondition`.
- `TestSeatbeltExecutionAndDomainProxy`: runs `/usr/bin/sandbox-exec` on macOS verifying writing inside `wsRoot` succeeds while touching a file in another temp directory outside `wsRoot` is denied by the kernel (`Operation not permitted`), and verifies `EgressProxy` blocks unlisted hostnames while allowing allowlisted ones.
- `TestSearchCodeRepoMapSkillsAndBM25DeferredTools`: registers 18 MCP tools, asserts `VisibleSchemas` exposes only the core tools + `search_tools` until BM25 activates matches, and verifies user-level `SKILL.md` shadows same-named project `SKILL.md`.

- [ ] **Step 2: Run tests to verify failure**

Run: `CGO_ENABLED=0 go test -race ./internal/tools/...`
Expected: FAIL.

- [ ] **Step 3: Implement `internal/tools/*.go`**

Implement `PathGuard`, `ComputeStagedEdit` / `ApplySelectedHunks`, the 5 built-in tools (`search_code` includes concurrent worker pool + Go/TS/Python/Rust top-level symbol extractor for `RepoMap(wsRoot, maxTokens)`), `SeatbeltRunner` (allowing `/bin`, `/usr`, `/System`, `/Library`, `/dev`, `/private/var/tmp`, read access to workspace & toolchain, write access strictly to canonical `wsRoot` + temp dir), `EgressProxy`, `SkillCatalog`, `MCPManager`, and `WrapUntrustedContent`.

- [ ] **Step 4: Run tests with race detector to verify pass**

Run: `CGO_ENABLED=0 go test -v -race ./internal/tools/...`
Expected: PASS.

- [ ] **Step 5: Commit Task 4**

```bash
git add internal/tools
git commit -m "feat(tools): implement 5 core tools, partial hunk diff merger, macOS Seatbelt sandbox, skills, and MCP"
```

---

### Task 5: Microkernel Hook Bus, 9-Step PermissionGate, Multi-Project Scheduler & AgentLoop

**Files:**
- Create: `internal/core/hooks.go`
- Create: `internal/core/permission_gate.go`
- Create: `internal/core/approvals.go`
- Create: `internal/core/compaction.go`
- Create: `internal/core/loop.go`
- Create: `internal/core/engine.go`
- Test: `internal/core/core_test.go`

**Interfaces:**
- Consumes: `internal/config`, `internal/store`, `internal/llm`, `internal/tools`.
- Produces:
  - `type Scope struct` & `type HookBus struct` (`MountPlugin`, `UnmountPlugin` LIFO cleanup, `Freeze()` compiling immutable handler slices for `PreStep`, `PreToolUse`, `PostToolUse`, `TurnStopping`).
  - `type GateAction string` (`GateAllow`, `GateAsk`, `GateDefer`, `GateDeny`), `type GateResult struct { Action GateAction; ReasonCode, Reason, Fingerprint string }`.
  - `type PermissionGate struct` implementing the exact 9-step evaluation order (`Evaluate(ctx EvalContext, call ToolInvocation) GateResult`).
  - `type PendingApproval struct { ID, RunID, WorkspaceID, SessionID, ToolName, ReasonCode, Reason, Fingerprint string; Args json.RawMessage; Hunks []tools.DiffHunk; SurfacedAt time.Time }` and `type ApprovalBroker struct` enforcing `SettleWindow = 500 * time.Millisecond` (`ErrApprovalNotSettled`), supporting `ApproveAll`, `ApproveHunks(hunkIDs []string)`, `ApproveAndRemember`, and `Deny`.
  - `type RunEvent struct { Seq uint64; Type, RunID, WorkspaceID, SessionID, AgentRole, ModelID string; Payload any; Timestamp time.Time }`.
  - `type Engine struct` with `StartRun(ctx context.Context, req RunRequest) (runID string, stream <-chan RunEvent, err error)`, `Steer(runID, message string) error`, `StopRun(runID string) error`, `ActiveRuns() []ActiveRunSummary`, `Broker() *ApprovalBroker`, `CompactSession(ctx context.Context, sessionID string) error`.

- [ ] **Step 1: Write failing microkernel, 9-step gate, 500ms approval, multi-project concurrency & loop tests in `internal/core/core_test.go`**

Write tests covering:
- `TestMicrokernelScopeLIFOUnwindAndZeroOverheadFrozenHooks`: mounts two dependent plugins registering tools and hooks, freezes bus, unmounts base plugin, verifies dependent cleans up first and registered tools/hooks vanish cleanly.
- `TestPermissionGateNineStepOrder`: asserts (1) hard-floor blocks `rm -rf /` and `~/.gorness/providers.toml` even in `yolo` mode, (2) `plan` mode denies writes and MCP tools, (3) untrusted modified `.gorness/mcp.toml` downgrades calls to `ask` (and `deny` when unattended), (4) MCP `readOnlyHint: true` is NOT exempted as built-in readonly, (5) touching `.gorness/` or `.zshrc` triggers `auto-run-config` ask, (6) unattended background run needing confirmation returns `GateDefer`.
- `TestApprovalBrokerSettleGuard`: creates pending approval, attempts `Resolve` at `+100ms` using injectable clock, asserts `ErrApprovalNotSettled` and approval remains pending; advances clock past `500ms` and verifies partial hunk approval applies selected hunks and resumes loop.
- `TestMultiWorkspaceParallelRunsSteerDoomLoopAndCompaction`: starts 2 runs concurrently across Workspace A and Workspace B, injects mid-run `Steer` into Run A verifying injection at step boundary without breaking static system prompt, triggers 3 identical failing tool calls verifying `DoomLoop` halts runaway repetition, and verifies `CompactSession` merges anchored summary while keeping recent 30% turns verbatim.

- [ ] **Step 2: Run tests to verify failure**

Run: `CGO_ENABLED=0 go test -race ./internal/core/...`
Expected: FAIL.

- [ ] **Step 3: Implement `internal/core/{hooks,permission_gate,approvals,compaction,loop,engine}.go`**

Implement `HookBus`, `PermissionGate`, `ApprovalBroker`, `Compactor`, `AgentLoop` (partitioning consecutive concurrency-safe read tools via goroutine `errgroup` up to concurrency 8 while serializing mutating/shell tools), and `Engine` (per-session `TryFlock` single-writer guard + async `utility` role auto-titler after turn 1).

- [ ] **Step 4: Run tests with race detector to verify pass**

Run: `CGO_ENABLED=0 go test -v -race ./internal/core/...`
Expected: PASS.

- [ ] **Step 5: Commit Task 5**

```bash
git add internal/core
git commit -m "feat(core): build microkernel hook bus, 9-step permission gate, 500ms approval broker, and concurrent agent loop"
```

---

### Task 6: File-Native Always-On Engine — Inbox, Timers, Pointer Memory, Deferred Resume & Outbox

**Files:**
- Create: `internal/alwayson/inbox.go`
- Create: `internal/alwayson/timers.go`
- Create: `internal/alwayson/memory.go`
- Create: `internal/alwayson/dreamer.go`
- Create: `internal/alwayson/outbox.go`
- Create: `internal/alwayson/service.go`
- Create: `internal/alwayson/launchd.go`
- Test: `internal/alwayson/alwayson_test.go`

**Interfaces:**
- Consumes: `internal/config`, `internal/store`, `internal/core`, `github.com/robfig/cron/v3`.
- Produces:
  - `type InboxEvent struct { ID, InboxKey, Source, SourceID, WorkspaceID, SessionID, AgentRole string; Payload map[string]any; Status string; Attempts int; CreatedAt time.Time }`.
  - `type DeferredCard struct { ID, RunID, WorkspaceID, SessionID, ToolName, ReasonCode, Reason string; Args json.RawMessage; Hunks []tools.DiffHunk; CreatedAt time.Time }`.
  - `type TimerEntry struct { ID, WorkspaceID, SessionID, Kind, CronExpr, Note string; DueAt time.Time; Enabled bool }`.
  - `type OutboxItem struct { ID, WorkspaceID, SessionID, Outcome, Title, Body string; Delivered bool; CreatedAt time.Time }` (`Outcome`: `"send" | "silent" | "defer"`).
  - `type Notifier interface { NotifyMac(title, subtitle, body string) error }`.
  - `type Service struct` with `Enqueue(ev InboxEvent) (deduped bool, err error)`, `ScheduleFollowUp(wsID, sessionID string, after time.Duration, note string) (*TimerEntry, error)`, `UpsertCron(entry TimerEntry) error`, `ListDeferred() ([]DeferredCard, error)`, `ResolveDeferred(id string, approved bool, selectedHunks []string) error`, `RunDreamPass(ctx context.Context, wsID string) ([]string, error)`, `StateSnapshot() AlwaysOnState`, `Start(ctx context.Context) error`, and `GenerateLaunchdPlist(binPath, vaultRoot string) ([]byte, string)`.

- [ ] **Step 1: Write failing Always-On 5-primitive tests in `internal/alwayson/alwayson_test.go`**

Write tests covering:
- `TestInboxRecoverAndIdempotency`: writes a stale `running` item and duplicate `(source, source_id)` into `inbox.jsonl`, calls `Start`, verifies stale item retries up to `MaxAttempts = 3` (then transitions to `failed`) and duplicate `(source, source_id)` is ignored (`deduped == true`).
- `TestMicroBatchingAndSilentVsSendOutbox`: enqueues 3 rapid user events within debounce window (`50ms` in test config), verifies worker batches all 3 into a single `Engine.StartRun` invocation, checks `REPORTED.md` deduplication suppresses repeat notifications (`Outcome == "silent"`), and verifies new findings deliver to `Outbox` + injected `Notifier`.
- `TestNonBlockingDeferredCallAndEventResume`: triggers an unattended run that hits a `GateDefer` tool call, verifies `alwayson/deferred/<id>.md` is written and the Inbox worker immediately processes a second unrelated event without blocking; then calls `ResolveDeferred(id, true, nil)` and verifies an approval resume event enters `Inbox`, executes the deferred tool, and finishes the original session!
- `TestTimersAndDreamerCandidateSkill`: schedules a due `followup` and `cron` entry in `timers.toml`, verifies tick moves them into `Inbox` without calling LLM directly, and runs `RunDreamPass` verifying candidate `SKILL.md` creation in `~/.gorness/skills/_candidates/`.

- [ ] **Step 2: Run tests to verify failure**

Run: `CGO_ENABLED=0 go test -race ./internal/alwayson/...`
Expected: FAIL.

- [ ] **Step 3: Implement `internal/alwayson/{inbox,timers,memory,dreamer,outbox,service,launchd}.go`**

Implement all 5 file-native Always-On primitives (`inbox.jsonl` + atomic state projection `inbox.state.toml` under `Flock`, `timers.toml`, `memory/*.md`, `deferred/*.md`, `outbox.jsonl` + safe `/usr/bin/osascript` argv notification, and `launchd` plist generator).

- [ ] **Step 4: Run tests with race detector to verify pass**

Run: `CGO_ENABLED=0 go test -v -race ./internal/alwayson/...`
Expected: PASS.

- [ ] **Step 5: Commit Task 6**

```bash
git add internal/alwayson
git commit -m "feat(alwayson): implement file-native inbox micro-batching, timers, pointer memory, deferred resume, and outbox"
```

---

### Task 7: Visual Multi-Agent Fleet Orchestrator, Git Worktrees, Markdown Blackboard & Refinery

**Files:**
- Create: `internal/fleet/types.go`
- Create: `internal/fleet/worktree.go`
- Create: `internal/fleet/blackboard.go`
- Create: `internal/fleet/refinery.go`
- Create: `internal/fleet/coordinators.go`
- Create: `internal/fleet/manager.go`
- Test: `internal/fleet/fleet_test.go`

**Interfaces:**
- Consumes: `internal/config`, `internal/store`, `internal/core`, `internal/llm`.
- Produces:
  - `type TopologyKind string` (`TopoHybridFleet`, `TopoReviewLoop`, `TopoParallelExplore`, `TopoWorktreeTeam`, `TopoEventPipeline`, `TopoSharedBoard`).
  - `type FleetNode struct { ID, Label, RoleID, ProviderID, ModelID, Effort, Status, WorktreeBranch, WorktreePath, SessionID, ActiveRunID string; Tokens int; CostUSD float64; DiffStat string; UpdatedAt time.Time }`.
  - `type FleetEdge struct { From, To, Label, Kind string; Active bool; PulseSeq uint64 }`.
  - `type BlackboardTask struct { ID, ParentID, Title, OwnerNodeID, Status, WorktreeBranch, LandedCommit string; Depth, Version int }`.
  - `type MissionSnapshot struct { ID, WorkspaceID, WorkflowID, Goal string; Topology TopologyKind; Status string; Nodes []FleetNode; Edges []FleetEdge; Tasks []BlackboardTask; Notes []BlackboardNote; MergeQueue []string; TotalCostUSD, BudgetUSD float64; CreatedAt, UpdatedAt time.Time }`.
  - `type Manager struct` with `StartMission(ctx context.Context, req MissionRequest) (*MissionSnapshot, <-chan MissionEvent, error)`, `GetMission(wsID, missionID string) (*MissionSnapshot, error)`, `ListMissions(wsID string) ([]*MissionSnapshot, error)`, `SteerNode(wsID, missionID, nodeID, instruction string) error`, `StopMission(wsID, missionID string) error`, `InspectNodeDiff(wsID, missionID, nodeID string) (string, error)`.

- [ ] **Step 1: Write failing multi-agent topologies, Git worktree isolation, blackboard flock & Refinery merge tests in `internal/fleet/fleet_test.go`**

Create a real temporary Git repository (`git init`, initial commit) in tests and cover:
- `TestWorktreePoolAndRefinerySerialMerge`: spawns 2 concurrent `worktree-team` coder agents editing `pkg_a.go` and `pkg_b.go` in isolated worktrees under `runtime/worktrees/`, runs heterogeneously routed `verifier` (`openai` profile) blind review + `go test ./...` gate, then runs `Refinery` serial rebase + fast-forward merge onto `main`, asserting both files land cleanly on `main` and temporary worktrees clean up.
- `TestMarkdownBlackboardFlockClaimSplitAndConvergenceGuard`: 8 concurrent peer workers contend to claim `missions/<id>/tasks/*.md` via `Flock` (asserting exact 1 winner per task, zero duplicate work), dynamically split a complex task to depth 3, share `notes/*.md`, and verify `ConvergenceGuard` halts cleanly when `max_idle_rounds` is reached (preventing infinite reactive loops).
- `TestAllSixTopologiesAndLiveNodeSteer`: exercises `review-loop` (reject -> fix -> accept), `parallel-explore`, `event-pipeline` (`MaxHops = 8` loop breaker), and `hybrid-fleet`, verifying `MissionSnapshot` emits live `Nodes`/`Edges` with per-role heterogeneous model metadata and `SteerNode` forwards mid-run instructions to the targeted worker.

- [ ] **Step 2: Run tests to verify failure**

Run: `CGO_ENABLED=0 go test -race ./internal/fleet/...`
Expected: FAIL.

- [ ] **Step 3: Implement `internal/fleet/{types,worktree,blackboard,refinery,coordinators,manager}.go`**

Implement `WorktreePool` (`git worktree add/remove`, `git diff --stat`), `MarkdownBlackboard` (`MISSION.md`, `tasks/<id>.md`, `notes/<seq>-<node>.md`, `events.jsonl`), `Witness` heartbeat/stall monitor, `Refinery` serial rebase & verification runner, and all 6 topology coordinators dispatching heterogeneous roles through `core.Engine`.

- [ ] **Step 4: Run tests with race detector to verify pass**

Run: `CGO_ENABLED=0 go test -v -race ./internal/fleet/...`
Expected: PASS.

- [ ] **Step 5: Commit Task 7**

```bash
git add internal/fleet
git commit -m "feat(fleet): implement 6 visual multi-agent topologies, git worktree pool, markdown blackboard, and refinery merge gate"
```

---

### Task 8: Loopback Server, Isolated Preview Shell, Apple HIG Workbench UI, Charm TUI & Unified `gorness` CLI

**Files:**
- Create: `internal/server/guard.go`
- Create: `internal/server/preview.go`
- Create: `internal/server/external_open.go`
- Create: `internal/server/server.go`
- Create: `web/embed.go`
- Create: `web/index.html`
- Create: `web/styles.css`
- Create: `web/app.js`
- Create: `internal/tui/app.go`
- Create: `cmd/gorness/main.go`
- Test: `internal/server/server_test.go`
- Test: `cmd/gorness/main_test.go`

**Interfaces:**
- Consumes: `internal/config`, `internal/store`, `internal/llm`, `internal/tools`, `internal/core`, `internal/alwayson`, `internal/fleet`, `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`.
- Produces:
  - `type Server struct` (`NewServer(deps Deps) *Server`, `Handler() http.Handler` serving embedded `web/` + `/api/health`, `/api/bootstrap`, `/api/config/*`, `/api/workspaces/*`, `/api/sessions/*` [create, list, search, fork, rewind, compact, move, export], `/api/runs/*` [4s-heartbeat SSE stream, steer, stop], `/api/approvals/:id/resolve`, `/api/alwayson/*` [webhook ingress, timers, deferred, outbox, dream], `/api/fleet/*` [start mission, list, SSE topology stream, inspect node diff, steer node, stop], `/api/open-safe`, and `/preview/:token`).
  - Embedded Apple HIG Workbench (`web/index.html`, `web/styles.css`, `web/app.js`):
    - Cupertino light/dark adaptive design system + `[中文, English]` instant switch + DOM morphing preserving active input/IME/scroll.
    - **Left Pane (resizable):** Cross-workspace `Needs Attention` approval queue, live multi-project running activity indicators, instant global session search, pinned & project-grouped sessions, `~/.gorness/` vault status & Config Studio launcher.
    - **Center Pane (dual mode):** (A) Streaming conversation with collapsible Adaptive Thinking, nested subagent cards, `@` file / `$` skill / `/` command palette, per-session persisted drafts, and `500ms`-guarded Partial Hunk Diff Approval banner; (B) **Interactive Visual Multi-Agent Orchestration Canvas** (live animated SVG DAG/swimlane for all 6 topologies showing heterogeneous `Provider/Model` badges on every agent node, live message pulses, Markdown blackboard task tree, click-node-to-inspect Worktree diff/logs & send targeted `Steer`).
    - **Right Inspector (resizable, 4 tabs):** Workspace Files, Snapshot Time-Machine (`/rewind` & `/fork`), Heterogeneous Agent-Model Matrix & Live `~/.gorness/` TOML/MD Editor, and Sandboxed HTML Artifact Preview (`sandbox="allow-scripts"` without `allow-same-origin`).
  - `internal/tui.Run(ctx context.Context, serverURL, initialWsPath string) error`: Charm Bubble Tea + Lip Gloss multi-project/multi-session terminal UI with live streaming, inline hunk diff review, and fleet status bar.
  - `cmd/gorness/main.go`: subcommands `gorness` (default TUI in `cwd`, auto-starting background daemon on `127.0.0.1:3456` if not running), `gorness studio` (ensures daemon & opens dedicated Mac app-mode window or browser), `gorness run [flags] "<prompt>"` (`--agent`, `--model`, `--workspace`, `--json`), `gorness daemon [serve|install|uninstall|status|stop]`, and `gorness acp` (stdio JSON-RPC ACP server routing edits/commands through Gorness's own transactional `Engine`).

- [ ] **Step 1: Write failing server security, full HTTP/SSE API, embedded UI & CLI integration tests in `internal/server/server_test.go` and `cmd/gorness/main_test.go`**

Write tests covering:
- `TestServerOriginGuardAndIsolatedPreviewCSP`: verifies requests with `Origin: https://evil.example` receive `403 Forbidden` and zero `Access-Control-Allow-Origin` headers; verifies `/preview/:token` renders inside a strict CSP shell (`default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'none'; frame-src 'none'`) and `external_open` copies allowlisted docs read-only (`0400`) into `runtime/` verifying FD/inode before invoking injected opener.
- `TestEndToEndMultiProjectSessionFleetAndAlwaysOnAPI`: drives `Server.Handler()` in-memory via `httptest` across 2 concurrent workspaces—registers workspaces, creates providers/agent profiles via `/api/config`, runs a streaming session over `/api/runs/stream`, forks & rewinds session, launches a `hybrid-fleet` mission streaming topology events over `/api/fleet/missions/:id/stream`, triggers an Always-On webhook into `/api/alwayson/webhook/local`, and verifies embedded `/` serves `web/index.html` & `web/app.js` cleanly.
- `TestCLIHeadlessJSONAndACPRoundTrip`: runs `gorness run --json` against a mock provider in a temp vault and verifies valid NDJSON event output and exit code `0`, plus an ACP `initialize` + `session/new` stdio JSON-RPC handshake.

- [ ] **Step 2: Run tests to verify failure**

Run: `CGO_ENABLED=0 go test -race ./internal/server/... ./cmd/gorness/...`
Expected: FAIL.

- [ ] **Step 3: Implement `internal/server/*`, `web/*`, `internal/tui/*`, and `cmd/gorness/main.go`**

Implement the loopback server, safe copy-opener, isolated preview shell, the complete Apple HIG three-pane web workbench + interactive SVG multi-agent orchestration studio (`web/index.html`, `web/styles.css`, `web/app.js` verified syntax-clean with `node --check`), the Bubble Tea TUI, and the unified `cmd/gorness` CLI entrypoint.

- [ ] **Step 4: Run full repository test suite, JS syntax check, and static binary build**

Run: `node --check web/app.js && CGO_ENABLED=0 go test -v -race ./... && CGO_ENABLED=0 go build -o gorness ./cmd/gorness && ./gorness --version`
Expected: `web/app.js` syntax valid, 100% of Go packages PASS with `-race`, static `darwin/arm64` `gorness` binary built and prints version cleanly.

- [ ] **Step 5: Commit Task 8**

```bash
git add internal/server web internal/tui cmd/gorness
git commit -m "feat(app): deliver loopback server, Apple HIG workbench with visual fleet studio, Bubble Tea TUI, and unified gorness CLI"
```
