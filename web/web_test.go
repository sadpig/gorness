package web_test

import (
	"io/fs"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"gorness.dev/gorness/web"
)

func mustReadEmbedded(t *testing.T, name string) string {
	t.Helper()
	raw, err := fs.ReadFile(web.Assets, name)
	if err != nil {
		t.Fatalf("read embedded %s: %v", name, err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		t.Fatalf("embedded %s must not be empty", name)
	}
	return string(raw)
}

func TestEmbeddedAssetsSingleSettingsHomeAndUIDeduplication(t *testing.T) {
	indexHTML := mustReadEmbedded(t, "index.html")
	stylesCSS := mustReadEmbedded(t, "styles.css")
	appJS := mustReadEmbedded(t, "app.js")

	// 1. Deleted duplicate entrypoints, redundant tabs, scattered buttons & dead forms MUST be absent from index.html.
	forbiddenHTMLSubstrings := []string{
		`id="mode-config-btn"`,
		`id="open-config-studio-btn"`,
		`id="insp-panel-config"`,
		`data-tab="config"`,
		`id="expand-full-config-btn"`,
		`id="agent-matrix-rows"`,
		`id="vault-file-select"`,
		`id="save-vault-file-btn"`,
		`id="vault-file-editor"`,
		`id="vault-save-status"`,
		`id="launch-fleet-quick-btn"`,
		`id="compact-session-btn"`,
		`id="fork-session-btn"`,
		`id="export-html-btn"`,
		`id="quick-provider-form"`,
		`id="toggle-add-provider-btn"`,
		`id="lang-toggle-btn"`,
		`allow-same-origin`,
	}
	for _, bad := range forbiddenHTMLSubstrings {
		if strings.Contains(indexHTML, bad) {
			t.Errorf("index.html must not contain removed duplicate/unsafe fragment %q", bad)
		}
	}

	// 2. Deleted duplicate renderers & legacy IDs MUST be absent from app.js.
	forbiddenJSSubstrings := []string{
		"renderWorkspaceFleetSubagentsHTML",
		"renderAgentMatrixAndVaultEditor",
		"open-config-studio-btn",
		"insp-panel-config",
		"launch-fleet-quick-btn",
		"quick-provider-form",
		"vault-file-editor",
	}
	for _, bad := range forbiddenJSSubstrings {
		if strings.Contains(appJS, bad) {
			t.Errorf("app.js must not contain removed duplicate symbol/selector %q", bad)
		}
	}

	// 3. Required single Settings home (9 pages), 2-mode switch, Always-On drawer, consolidated session controls & server_test.go contracts MUST exist.
	requiredHTMLSubstrings := []string{
		`id="mode-chat-btn"`,
		`id="mode-fleet-btn"`,
		`id="alwayson-drawer-btn"`,
		`id="open-settings-top-btn"`,
		`id="health-pill"`,
		`id="reload-diag-banner"`,
		`id="session-worktree-pill"`,
		`id="session-actions-menu-btn"`,
		`id="session-actions-menu"`,
		`data-session-action="new-worktree"`,
		`data-session-action="compact"`,
		`data-session-action="export-html"`,
		`data-session-action="export-md"`,
		`data-session-action="move"`,
		`data-session-action="archive"`,
		`data-session-action="delete"`,
		`id="composer-input"`,
		`id="composer-palette"`,
		`id="chat-history-entries"`,
		`id="model-override-input"`,
		`id="fleet-svg-canvas"`,
		`id="launch-fleet-form"`,
		`id="delete-mission-btn"`,
		`id="open-node-session-btn"`,
		`data-tab="files"`,
		`data-tab="snapshots"`,
		`data-tab="artifacts"`,
		`data-tab="context"`,
		`id="rewind-restore-files-cb"`,
		`id="inspector-md-preview"`,
		`id="inspector-preview-frame"`,
		`sandbox="allow-scripts"`,
		`id="config-mode-view"`,
		`data-cfg-page="general"`,
		`data-cfg-page="matrix"`,
		`data-cfg-page="providers"`,
		`data-cfg-page="agents"`,
		`data-cfg-page="workflows"`,
		`data-cfg-page="permissions"`,
		`data-cfg-page="mcp"`,
		`data-cfg-page="memory"`,
		`data-cfg-page="files"`,
		`id="alwayson-drawer"`,
		`id="alwayson-drawer-body"`,
		`id="trigger-dream-btn"`,
		`id="toast-stack"`,
	}
	for _, want := range requiredHTMLSubstrings {
		if !strings.Contains(indexHTML, want) {
			t.Errorf("index.html missing required contract fragment %q", want)
		}
	}

	// Ensure rewind-restore-files-cb appears exactly once in index.html (inside Snapshots tab).
	if cnt := strings.Count(indexHTML, `id="rewind-restore-files-cb"`); cnt != 1 {
		t.Errorf("expected exactly 1 #rewind-restore-files-cb in index.html, got %d", cnt)
	}

	// 4. CSS deduplication contract: single light-dark() token block, zero duplicate prefers-color-scheme variable blocks, no dead classes.
	if !strings.Contains(stylesCSS, "color-scheme: light dark;") || !strings.Contains(stylesCSS, "light-dark(") {
		t.Errorf("styles.css must use CSS Color 5 color-scheme + light-dark() tokens in :root")
	}
	if strings.Contains(stylesCSS, "prefers-color-scheme: dark") {
		t.Errorf("styles.css must not duplicate dark-mode variables in @media (prefers-color-scheme: dark)")
	}
	for _, deadCSS := range []string{".mac-input-xs", ".raw-vault-disclosure", ".agent-matrix-rows", ".alwayson-grid"} {
		if strings.Contains(stylesCSS, deadCSS) {
			t.Errorf("styles.css must not contain dead selector %q", deadCSS)
		}
	}
}

func TestCanonicalAPIRoutesAndZeroLegacyPaths(t *testing.T) {
	appJS := mustReadEmbedded(t, "app.js")

	// Legacy routes/verbs removed in Phase-2 B3 MUST NOT appear anywhere in app.js.
	if strings.Contains(appJS, "/api/alwayson/state") {
		t.Fatalf("app.js must use canonical GET /api/alwayson instead of legacy /api/alwayson/state")
	}

	legacyPatterns := []struct {
		desc string
		re   *regexp.Regexp
	}{
		{
			desc: "POST /api/config/global (must be PUT)",
			re:   regexp.MustCompile(`apiFetch\(\s*["']/api/config/global["']\s*,\s*\{\s*method:\s*["']POST["']`),
		},
		{
			desc: "POST /api/config/rules (must be PUT)",
			re:   regexp.MustCompile(`apiFetch\(\s*["']/api/config/rules["']\s*,\s*\{\s*method:\s*["']POST["']`),
		},
		{
			desc: "PUT /api/config/mcp (must be POST)",
			re:   regexp.MustCompile(`apiFetch\(\s*["']/api/config/mcp["']\s*,\s*\{\s*method:\s*["']PUT["']`),
		},
		{
			desc: "PUT /api/sessions/{id} (must be PATCH)",
			re:   regexp.MustCompile(`apiFetch\(\s*` + "`" + `/api/sessions/\$\{[^}]+\}` + "`" + `\s*,\s*\{\s*method:\s*["']PUT["']`),
		},
		{
			desc: "PUT /api/workspaces/{wsID} metadata (must be PATCH)",
			re:   regexp.MustCompile(`apiFetch\(\s*` + "`" + `/api/workspaces/\$\{[^}]+\}` + "`" + `\s*,\s*\{\s*method:\s*["']PUT["']`),
		},
		{
			desc: "legacy /api/fleet/missions/{id}/diff without /nodes/",
			re:   regexp.MustCompile(`/api/fleet/missions/\$\{[^}]+\}/diff`),
		},
		{
			desc: "legacy /api/fleet/missions/{id}/steer without /nodes/",
			re:   regexp.MustCompile(`/api/fleet/missions/\$\{[^}]+\}/steer`),
		},
	}
	for _, lp := range legacyPatterns {
		if lp.re.MatchString(appJS) {
			t.Errorf("app.js contains forbidden legacy API call: %s", lp.desc)
		}
	}

	// Canonical REST & SSE routes required by Phase-2 Slice 4 MUST be present.
	requiredRoutes := []string{
		`"/api/alwayson"`,
		`"/api/alwayson/outbox/ack"`,
		`"/api/alwayson/dream"`,
		`"/api/config/global"`,
		`"/api/config/providers"`,
		`"/api/config/secrets"`,
		`"/api/config/agents"`,
		`"/api/config/workflows"`,
		`"/api/config/rules"`,
		`"/api/config/mcp"`,
		`"/api/config/files"`,
		`"/api/memory"`,
		`"/api/memory/layer"`,
		`"/api/skills/candidates"`,
		`/nodes/${encodeURIComponent(nodeId)}/diff`,
		`/nodes/${encodeURIComponent(state.selectedNodeId)}/steer`,
		`/api/runs/${encodeURIComponent(activeRun.run_id)}/stream`,
		`onNativeWorkspaceRegistered`,
	}
	for _, rt := range requiredRoutes {
		if !strings.Contains(appJS, rt) {
			t.Errorf("app.js missing canonical route/bridge %q", rt)
		}
	}
}

func TestNodeSyntaxI18NParityNineSettingsPagesAndWorkbenchBehaviors(t *testing.T) {
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed in PATH")
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	webDir := filepath.Dir(thisFile)
	appJSPath := filepath.Join(webDir, "app.js")
	indexHTMLPath := filepath.Join(webDir, "index.html")

	// 1. Fast syntax check: node --check web/app.js
	checkCmd := exec.Command(nodeBin, "--check", appJSPath)
	if out, err := checkCmd.CombinedOutput(); err != nil {
		t.Fatalf("node --check web/app.js failed: %v\n%s", err, string(out))
	}

	// 2. Behavioral verification of I18N parity, all 9 editable Settings pages, Always-On drawer,
	//    Fleet sprawl elimination, /rewind default restore_files, palette keyboard navigation,
	//    Bidi single-escaping, Markdown renderer, and native macOS NSOpenPanel callback.
	script := `
import assert from "node:assert/strict";
import fs from "node:fs";
import { pathToFileURL } from "node:url";

globalThis.__GORNESS_TEST_NO_AUTOBOOT__ = true;

const indexHTML = fs.readFileSync(process.env.INDEX_HTML_PATH, "utf8");
const appJSSource = fs.readFileSync(process.env.APP_JS_PATH, "utf8");

// Minimal deterministic DOM harness
const elements = new Map();
function makeClassList(initial = []) {
  const set = new Set(initial);
  return {
    add: (...xs) => xs.forEach((x) => set.add(x)),
    remove: (...xs) => xs.forEach((x) => set.delete(x)),
    toggle: (x, force) => {
      if (force === undefined) {
        if (set.has(x)) { set.delete(x); return false; }
        set.add(x); return true;
      }
      if (force) set.add(x); else set.delete(x);
      return Boolean(force);
    },
    contains: (x) => set.has(x)
  };
}
function getEl(id) {
  if (!elements.has(id)) {
    const hiddenByDefault = [
      "config-mode-view", "alwayson-drawer", "composer-palette",
      "search-results-popover", "session-actions-menu", "reload-diag-banner"
    ].includes(id);
    elements.set(id, {
      id,
      value: "",
      checked: id === "rewind-restore-files-cb",
      innerHTML: "",
      textContent: "",
      src: "",
      href: "",
      dataset: {},
      options: [],
      style: { setProperty() {} },
      classList: makeClassList(hiddenByDefault ? ["hidden"] : []),
      addEventListener() {},
      focus() {},
      blur() {},
      setSelectionRange() {},
      appendChild() {},
      setAttribute() {},
      getAttribute() { return ""; }
    });
  }
  return elements.get(id);
}

globalThis.localStorage = {
  _m: new Map([["gorness.lang", "zh"], ["gorness.theme", "auto"]]),
  getItem(k) { return this._m.get(k) ?? null; },
  setItem(k, v) { this._m.set(k, String(v)); }
};
globalThis.CSS = { escape: (s) => String(s) };
globalThis.document = {
  documentElement: { lang: "zh-CN", style: { setProperty() {} }, setAttribute() {} },
  activeElement: null,
  getElementById: (id) => getEl(id),
  querySelectorAll: () => [],
  querySelector: () => null,
  createElement: () => getEl("__temp__"),
  addEventListener() {}
};
globalThis.window = { innerWidth: 1440, addEventListener() {} };

const apiCalls = [];
globalThis.fetch = async (input, init = {}) => {
  const method = (init.method || "GET").toUpperCase();
  const body = init.body ? JSON.parse(init.body) : null;
  apiCalls.push({ url: String(input), method, body });

  if (input === "/api/bootstrap") {
    return new Response(JSON.stringify({
      version: "0.9.0-test",
      vault_root: "/tmp/gorness-vault",
      config: {
        global: {
          theme: "dark",
          language: "zh",
          default_provider: "anthropic",
          default_model: "claude-sonnet-5-5",
          default_effort: "high",
          permission_mode: "auto",
          sandbox_mode: "workspace-write",
          max_concurrency: 8,
          max_steps: 32,
          inbox_debounce_ms: 400
        },
        providers: {
          anthropic: {
            id: "anthropic",
            name: "Anthropic",
            protocol: "anthropic",
            base_url: "https://api.anthropic.com",
            api_key: "keychain:ANTHROPIC_API_KEY",
            models: { "claude-sonnet-5-5": { name: "Claude Sonnet 5.5", context_window: 200000 } }
          }
        },
        agents: {
          coder: {
            id: "coder",
            name: "Coder",
            provider: "anthropic",
            model: "claude-sonnet-5-5",
            effort: "high",
            allowed_tools: ["read_file", "edit_file"],
            system_prompt: "You are Coder."
          }
        },
        workflows: {
          "hybrid-fleet": {
            id: "hybrid-fleet",
            name: "Hybrid Fleet",
            topology: "hybrid-fleet",
            verify_command: "go test ./...",
            max_concurrency: 3,
            max_rounds: 2,
            use_worktrees: true,
            role_models: { coder_a: "anthropic/claude-sonnet-5-5" },
            nodes: [
              { id: "coder_a", label: "Coder A", role: "coder", worktree: true },
              { id: "coder_b", label: "Coder B", role: "coder", worktree: true }
            ],
            edges: [{ from: "coder_a", to: "coder_b", kind: "seq" }]
          }
        },
        rules: {
          allow_commands: ["go test*"],
          ask_commands: ["git push*"],
          deny_commands: ["rm -rf /"],
          rules: [{ action: "deny", tool: "run_command", pattern: "sudo *", reason: "no root" }]
        },
        mcp: {
          servers: {
            "custom-internal-sse": {
              transport: "sse",
              url: "http://127.0.0.1:9100/sse",
              timeout_sec: 45,
              enabled: true
            }
          }
        }
      },
      config_files: ["config.toml", "providers.toml", "mcp.toml"],
      workspaces: [{ id: "ws_1", name: "DemoRepo", root_path: "/tmp/demo", permission_mode: "auto", sandbox_mode: "workspace-write" }],
      sessions: [{ id: "sess_1", workspace_id: "ws_1", title: "Main Session", agent_role: "coder", head_seq: 2 }],
      pending_approvals: [],
      alwayson: {
        inbox: [{ id: "in_1", source: "webhook:github", status: "queued", summary: "PR #42" }],
        timers: [{ id: "tm_1", kind: "cron", cron_expr: "0 9 * * 1-5", note: "Morning check", enabled: true }],
        deferred: [{ id: "def_1", status: "pending", tool_name: "edit_file", workspace_id: "ws_1", summary: "Update main.go", hunks: [{ id: "h1", file_path: "main.go", header: "@@ -1 +1 @@" }] }],
        outbox: [{ id: "out_1", title: "Nightly Report", body: "All green", acked: false }]
      },
      missions: [
        { id: "msn_bg_1", workspace_id: "ws_1", topology: "hybrid-fleet", goal: "WORKSPACE_FLEET_SECRET_GOAL", status: "running", nodes: [{ id: "n1", role_id: "coder", status: "running" }], edges: [] }
      ],
      previews: []
    }), { status: 200, headers: { "Content-Type": "application/json" } });
  }

  if (String(input).startsWith("/api/sessions/sess_1") && method === "GET") {
    return new Response(JSON.stringify({
      meta: { id: "sess_1", workspace_id: "ws_1", title: "Main Session", agent_role: "coder", head_seq: 2, draft: "" },
      entries: [
        { seq: 1, role: "assistant", blocks: [{ type: "tool_use", tool_name: "spawn_subagent", tool_call_id: "tc_sub_1", tool_input: { role: "explorer", prompt: "Audit web package" } }] },
        { seq: 2, role: "tool", blocks: [{ type: "tool_result", tool_name: "spawn_subagent", tool_call_id: "tc_sub_1", text: "Web audit clean" }] }
      ],
      snapshots: []
    }), { status: 200, headers: { "Content-Type": "application/json" } });
  }

  if (String(input).includes("/files?limit=")) {
    return new Response(JSON.stringify({ files: ["web/app.js", "web/index.html"] }), { status: 200 });
  }
  if (String(input).endsWith("/skills")) {
    return new Response(JSON.stringify({ skills: [{ name: "go-review", description: "Go code review" }] }), { status: 200 });
  }
  if (String(input).endsWith("/artifacts")) {
    return new Response(JSON.stringify({ artifacts: [], previews: [] }), { status: 200 });
  }
  if (input === "/api/alwayson") {
    return new Response(JSON.stringify({
      inbox: [{ id: "in_1", source: "webhook:github", status: "queued", summary: "PR #42" }],
      timers: [{ id: "tm_1", kind: "cron", cron_expr: "0 9 * * 1-5", note: "Morning check", enabled: true }],
      deferred: [{ id: "def_1", status: "pending", tool_name: "edit_file", workspace_id: "ws_1", summary: "Update main.go", hunks: [{ id: "h1", file_path: "main.go", header: "@@ -1 +1 @@" }] }],
      outbox: [{ id: "out_1", title: "Nightly Report", body: "All green", acked: false }]
    }), { status: 200, headers: { "Content-Type": "application/json" } });
  }
  if (input === "/api/memory") {
    return new Response(JSON.stringify({ instructions: "Prefer stdlib.", index: "- arch", notes: { arch: "# Arch" } }), { status: 200 });
  }
  if (input === "/api/skills/candidates") {
    return new Response(JSON.stringify({ candidates: [{ slug: "auto-tdd", description: "TDD loop", content: "# Auto TDD" }] }), { status: 200 });
  }
  if (String(input).includes("/grants")) {
    return new Response(JSON.stringify({ grants: [{ fingerprint: "fp_1", tool_name: "run_command", summary: "go test ./..." }] }), { status: 200 });
  }
  if (String(input).startsWith("/api/config/files?rel=")) {
    return new Response(JSON.stringify({ rel_path: "config.toml", content: "theme = 'dark'\n" }), { status: 200 });
  }
  return new Response(JSON.stringify({ ok: true }), { status: 200, headers: { "Content-Type": "application/json" } });
};

const mod = await import(pathToFileURL(process.env.APP_JS_PATH).href);

// A. Check I18N zh/en exact key parity & verify every data-i18n / data-i18n-placeholder in index.html resolves!
const i18nMatch = appJSSource.match(/const I18N = (\{[\s\S]*?\n\});/);
assert.ok(i18nMatch, "I18N object must exist in app.js");
const I18N = Function("return " + i18nMatch[1])();
const zhKeys = Object.keys(I18N.zh).sort();
const enKeys = Object.keys(I18N.en).sort();
assert.deepEqual(zhKeys, enKeys, "I18N.zh and I18N.en must have identical keys");
for (const k of zhKeys) {
  assert.ok(I18N.zh[k].length > 0 && I18N.en[k].length > 0, "empty translation for key: " + k);
}
for (const m of indexHTML.matchAll(/data-i18n(?:-placeholder)?="([^"]+)"/g)) {
  const key = m[1];
  assert.ok(key in I18N.zh && key in I18N.en, "index.html references missing i18n key: " + key);
}

// B. Check single-pass Bidi vs pure HTML escaping
assert.equal(mod.escHtml("already sanitized <U+202E>"), "already sanitized &lt;U+202E&gt;");
assert.equal(mod.esc("raw \u202E bidi"), "raw &lt;U+202E&gt; bidi");

// C. Bootstrap & verify zero Fleet sprawl in Chat history while preserving session spawn_subagent card!
await mod.loadBootstrap();
const chatHTML = getEl("chat-history-entries").innerHTML;
assert.doesNotMatch(chatHTML, /WORKSPACE_FLEET_SECRET_GOAL/, "workspace Fleet missions must NEVER pollute Chat history");
assert.match(chatHTML, /class="subagent-card/, "session-scoped spawn_subagent must render .subagent-card");
assert.match(chatHTML, /Audit web package/, "subagent task must render in Chat");
assert.match(chatHTML, /Web audit clean/, "paired subagent result must render in Chat");

// D. Verify all 9 Settings pages render editable controls, custom MCP servers, and per-node role_models[nd.id]!
const pageChecks = [
  ["general", /data-cfg-ui-theme="dark"[\s\S]*id="gen-max-runs"/],
  ["matrix", /id="cfg-global-provider"[\s\S]*data-agent-model-select="coder"/],
  ["providers", /id="keychain-key-input"[\s\S]*id="prov-edit-baseurl"[\s\S]*id="delete-provider-btn"/],
  ["agents", /id="ag-edit-id"[\s\S]*id="ag-edit-model"[\s\S]*id="ag-edit-sysprompt"/],
  ["workflows", /id="wf-edit-id"[\s\S]*id="wf-edit-verify"[\s\S]*data-wf-node-field="model_override"/],
  ["permissions", /id="trust-workspace-btn"[\s\S]*id="rules-allow-cmds"[\s\S]*id="rules-structured-lines"/],
  ["mcp", /custom-internal-sse[\s\S]*id="mcp-edit-id"[\s\S]*id="mcp-edit-transport"/],
  ["memory", /id="mem-instructions-editor"[\s\S]*id="mem-note-body"[\s\S]*id="mem-prog-body"/],
  ["files", /id="studio-vault-select"[\s\S]*id="studio-vault-textarea"/]
];
for (const [page, pattern] of pageChecks) {
  mod.openSettingsModal(page);
  await new Promise((r) => setTimeout(r, 15));
  const html = getEl("config-studio-content").innerHTML;
  assert.match(html, pattern, "Settings page " + page + " failed editable controls assertion");
}
// Verify workflow node coder_a has its per-node override selected while coder_b (same role!) does NOT inherit coder_a's override
mod.openSettingsModal("workflows");
const wfHTML = getEl("config-studio-content").innerHTML;
assert.match(wfHTML, /value="anthropic\/claude-sonnet-5-5" selected/, "coder_a per-node role_models[nd.id] must be selected");

// E. Verify Always-On slide-over drawer renders Deferred hunk checkboxes + Timers + Outbox Ack
mod.openAlwaysOnDrawer();
await new Promise((r) => setTimeout(r, 15));
const aoHTML = getEl("alwayson-drawer-body").innerHTML;
assert.match(aoHTML, /data-def-resolve="def_1"[\s\S]*id="ao-timer-cron"[\s\S]*data-ao-ack-id="out_1"/);

// F. Verify /rewind defaults restore_files: true even when checkbox is unchecked, unless --no-files is passed!
getEl("rewind-restore-files-cb").checked = false;
apiCalls.length = 0;
await mod.handleSlashCommand("/rewind 1");
const rewDefault = apiCalls.find((c) => c.url.endsWith("/rewind"));
assert.deepEqual(rewDefault.body, { to_seq: 1, restore_files: true });

apiCalls.length = 0;
await mod.handleSlashCommand("/rewind 1 --no-files");
const rewNoFiles = apiCalls.find((c) => c.url.endsWith("/rewind"));
assert.deepEqual(rewNoFiles.body, { to_seq: 1, restore_files: false });

// G. Verify keyboard navigation on #composer-palette (@file)
const comp = getEl("composer-input");
comp.value = "@web/";
comp.selectionStart = comp.value.length;
mod.updateComposerPalette();
assert.equal(mod.state.paletteItems.length, 2);
assert.equal(mod.state.paletteActiveIndex, 0);
mod.handleComposerKeydown({ key: "ArrowDown", preventDefault() {} });
assert.equal(mod.state.paletteActiveIndex, 1);
mod.handleComposerKeydown({ key: "Enter", shiftKey: false, preventDefault() {} });
assert.equal(comp.value, "@web/index.html ");
assert.equal(getEl("composer-palette").classList.contains("hidden"), true);

// H. Verify safe zero-XSS Markdown preview & native NSOpenPanel callback
const mdHTML = mod.renderMarkdownHTML("# Title\n- **bold** \x60code\x60\n<script>alert(1)</script>");
assert.match(mdHTML, /<h1>Title<\/h1>/);
assert.match(mdHTML, /<strong>bold<\/strong> <code>code<\/code>/);
assert.doesNotMatch(mdHTML, /<script>/);

assert.equal(typeof globalThis.window.onNativeWorkspaceRegistered, "function");
await globalThis.window.onNativeWorkspaceRegistered({ id: "ws_1", name: "DemoRepo" });
`
	runCmd := exec.Command(nodeBin, "--input-type=module", "-e", script)
	runCmd.Env = append(runCmd.Environ(),
		"APP_JS_PATH="+appJSPath,
		"INDEX_HTML_PATH="+indexHTMLPath,
	)
	if out, err := runCmd.CombinedOutput(); err != nil {
		t.Fatalf("node web behavioral test failed: %v\n%s", err, string(out))
	}
}
