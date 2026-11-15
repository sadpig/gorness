package tui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gorness.dev/gorness/internal/alwayson"
	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/core"
	"gorness.dev/gorness/internal/fleet"
	"gorness.dev/gorness/internal/store"
)

type focusArea int

const (
	focusComposer focusArea = iota
	focusSidebar
	focusApproval
)

var roleCycleOrder = []string{"coder", "lead", "explorer", "debugger", "verifier"}

// Lip Gloss adaptive styles (crisp light/dark terminal palette, zero neon clutter)
var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#1D1D1F", Dark: "#F5F5F7"}).
			Background(lipgloss.AdaptiveColor{Light: "#E8E8ED", Dark: "#1C1C24"}).
			Padding(0, 1)

	sidebarStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, true, false, false).
			BorderForeground(lipgloss.AdaptiveColor{Light: "#D2D2D7", Dark: "#38383A"}).
			Padding(0, 1)

	selectedItemStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#0071E3", Dark: "#2997FF"})

	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#6E6E73", Dark: "#98989D"})

	userRoleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#0071E3", Dark: "#2997FF"})

	assistantRoleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#248A3D", Dark: "#30D158"})

	approvalBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.AdaptiveColor{Light: "#B25000", Dark: "#FF9F0A"}).
				Padding(0, 1)

	diffAddStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#19692C", Dark: "#66E384"})

	diffDelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#A82800", Dark: "#FF7A72"})

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#1D1D1F", Dark: "#F5F5F7"}).
			Background(lipgloss.AdaptiveColor{Light: "#EBEBF0", Dark: "#15151B"}).
			Padding(0, 1)
)

type bootstrapPayload struct {
	Version          string                   `json:"version"`
	VaultRoot        string                   `json:"vault_root"`
	Config           *config.Snapshot         `json:"config"`
	Workspaces       []*store.Workspace       `json:"workspaces"`
	Sessions         []*store.SessionMeta     `json:"sessions"`
	ActiveRuns       []core.ActiveRunSummary  `json:"active_runs"`
	PendingApprovals []core.PendingApproval   `json:"pending_approvals"`
	AlwaysOn         alwayson.AlwaysOnState   `json:"alwayson"`
	Missions         []*fleet.MissionSnapshot `json:"missions"`
}

type sessionDetailPayload struct {
	Meta    *store.SessionMeta   `json:"meta"`
	Entries []store.SessionEntry `json:"entries"`
}

type bootstrapLoadedMsg struct {
	data bootstrapPayload
	err  error
}

type sessionLoadedMsg struct {
	detail sessionDetailPayload
	err    error
}

type globalSSEEventMsg struct {
	ev  core.RunEvent
	err error
}

type statusNoticeMsg struct {
	text string
	err  error
}

type sessionSwitchedMsg struct {
	sessionID string
	wsID      string
	notice    string
}

// Model 实现 Gorness 多项目多会话终端工作台（流式对话、逐块 Diff 审批、舰队状态栏）。
type Model struct {
	ctx           context.Context
	serverURL     string
	initialWsPath string
	httpClient    *http.Client

	width  int
	height int
	focus  focusArea

	boot            bootstrapPayload
	activeWsID      string
	activeSessionID string
	sidebarIdx      int
	entries         []store.SessionEntry
	agentRole       string
	scrollOffset    int
	input           string
	statusNotice    string

	liveThinking string
	liveText     string
	liveRunID    string

	hunkCursor    int
	selectedHunks map[string]map[string]bool // approvalID -> hunkID -> bool
	surfacedLocal map[string]time.Time

	sseCh chan core.RunEvent
}

func NewModel(ctx context.Context, serverURL, initialWsPath string) *Model {
	if ctx == nil {
		ctx = context.Background()
	}
	cleanURL := strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if cleanURL == "" {
		cleanURL = "http://127.0.0.1:3456"
	}
	return &Model{
		ctx:           ctx,
		serverURL:     cleanURL,
		initialWsPath: strings.TrimSpace(initialWsPath),
		httpClient:    &http.Client{},
		width:         110,
		height:        32,
		focus:         focusComposer,
		agentRole:     "coder",
		selectedHunks: make(map[string]map[string]bool),
		surfacedLocal: make(map[string]time.Time),
		sseCh:         make(chan core.RunEvent, 128),
	}
}

// Run 启动连接本机 Gorness 守护进程的 Bubble Tea 交互式多项目终端界面。
func Run(ctx context.Context, serverURL, initialWsPath string) error {
	return RunWithOptions(ctx, serverURL, initialWsPath, tea.WithAltScreen())
}

func RunWithOptions(ctx context.Context, serverURL, initialWsPath string, opts ...tea.ProgramOption) error {
	if ctx == nil {
		ctx = context.Background()
	}
	m := NewModel(ctx, serverURL, initialWsPath)
	allOpts := append([]tea.ProgramOption{tea.WithContext(ctx)}, opts...)
	prog := tea.NewProgram(m, allOpts...)
	_, err := prog.Run()
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return nil
	}
	return err
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.initBootstrapCmd(),
		m.startGlobalSSEWorkerCmd(),
	)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		if typed.Width > 20 {
			m.width = typed.Width
		}
		if typed.Height > 10 {
			m.height = typed.Height
		}
		return m, nil

	case bootstrapLoadedMsg:
		if typed.err != nil {
			m.statusNotice = "Server error: " + typed.err.Error()
			return m, nil
		}
		m.boot = typed.data
		for _, appr := range m.boot.PendingApprovals {
			m.ensureApprovalTracked(appr)
		}
		if m.activeWsID == "" && len(m.boot.Workspaces) > 0 {
			m.activeWsID = m.boot.Workspaces[0].ID
		}
		visible := m.visibleSessions()
		if m.activeSessionID == "" && len(visible) > 0 {
			m.activeSessionID = visible[0].ID
			m.activeWsID = visible[0].WorkspaceID
		}
		if m.currentApproval() != nil && m.focus != focusApproval {
			m.focus = focusApproval
		}
		if m.activeSessionID != "" {
			return m, m.loadSessionCmd(m.activeSessionID)
		}
		return m, nil

	case sessionLoadedMsg:
		if typed.err == nil {
			m.entries = typed.detail.Entries
			if typed.detail.Meta != nil && typed.detail.Meta.AgentRole != "" {
				m.agentRole = typed.detail.Meta.AgentRole
			}
			m.scrollOffset = 0
		}
		return m, nil

	case globalSSEEventMsg:
		if typed.err != nil {
			return m, nil
		}
		ev := typed.ev
		var cmds []tea.Cmd
		cmds = append(cmds, m.waitNextSSECmd())

		switch ev.Type {
		case core.EventRunStarted:
			if ev.SessionID == m.activeSessionID {
				m.liveRunID = ev.RunID
				m.liveThinking = ""
				m.liveText = ""
			}
		case core.EventStepReset:
			if ev.SessionID == m.activeSessionID {
				m.liveThinking = ""
				m.liveText = ""
			}
		case core.EventThinkingDelta:
			if ev.SessionID == m.activeSessionID {
				if mp, ok := ev.Payload.(map[string]any); ok {
					if s, _ := mp["thinking"].(string); s != "" {
						m.liveThinking += s
					}
				}
			}
		case core.EventTextDelta:
			if ev.SessionID == m.activeSessionID {
				if mp, ok := ev.Payload.(map[string]any); ok {
					if s, _ := mp["text"].(string); s != "" {
						m.liveText += s
					}
				}
			}
		case core.EventApprovalRequired:
			raw, _ := json.Marshal(ev.Payload)
			var appr core.PendingApproval
			if json.Unmarshal(raw, &appr) == nil && appr.ID != "" {
				m.ensureApprovalTracked(appr)
				m.upsertApproval(appr)
				if appr.SessionID == m.activeSessionID {
					m.focus = focusApproval
					m.hunkCursor = 0
				}
			}
		case core.EventApprovalResolved:
			cmds = append(cmds, m.refreshBootstrapCmd())
		case core.EventRunCompleted, core.EventRunFailed, core.EventRunCancelled, core.EventCompacted, core.EventTitleUpdated:
			if ev.SessionID == m.activeSessionID {
				m.liveRunID = ""
				m.liveThinking = ""
				m.liveText = ""
				cmds = append(cmds, m.loadSessionCmd(m.activeSessionID))
			}
			cmds = append(cmds, m.refreshBootstrapCmd())
		}
		return m, tea.Batch(cmds...)

	case statusNoticeMsg:
		if typed.err != nil {
			m.statusNotice = "Error: " + typed.err.Error()
		} else {
			m.statusNotice = typed.text
		}
		return m, m.refreshBootstrapCmd()

	case sessionSwitchedMsg:
		if typed.sessionID != "" {
			m.activeSessionID = typed.sessionID
		}
		if typed.wsID != "" {
			m.activeWsID = typed.wsID
		}
		m.statusNotice = typed.notice
		return m, tea.Batch(m.refreshBootstrapCmd(), m.loadSessionCmd(m.activeSessionID))

	case tea.KeyMsg:
		return m.handleKey(typed)
	}

	return m, nil
}

func (m *Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "tab":
		if m.currentApproval() != nil {
			m.focus = (m.focus + 1) % 3
		} else {
			if m.focus == focusComposer {
				m.focus = focusSidebar
			} else {
				m.focus = focusComposer
			}
		}
		return m, nil

	case "ctrl+n":
		return m, m.createNewSessionCmd()

	case "ctrl+r":
		for i, r := range roleCycleOrder {
			if r == m.agentRole {
				m.agentRole = roleCycleOrder[(i+1)%len(roleCycleOrder)]
				m.statusNotice = "Role -> " + m.agentRole
				return m, nil
			}
		}
		m.agentRole = "coder"
		return m, nil

	case "ctrl+x":
		if runID := m.activeRunIDForSession(); runID != "" {
			return m, m.stopRunCmd(runID)
		}
		return m, nil
	}

	if m.focus == focusApproval {
		if appr := m.currentApproval(); appr != nil {
			switch k.String() {
			case "up", "k":
				if m.hunkCursor > 0 {
					m.hunkCursor--
				}
				return m, nil
			case "down", "j":
				if m.hunkCursor+1 < len(appr.Hunks) {
					m.hunkCursor++
				}
				return m, nil
			case " ":
				if m.hunkCursor >= 0 && m.hunkCursor < len(appr.Hunks) {
					hid := appr.Hunks[m.hunkCursor].ID
					m.selectedHunks[appr.ID][hid] = !m.selectedHunks[appr.ID][hid]
				}
				return m, nil
			case "a", "y", "enter":
				action := core.ApprovalActionApproveAll
				if len(appr.Hunks) > 0 {
					action = core.ApprovalActionApproveHunks
				}
				return m, m.resolveApprovalCmd(*appr, action)
			case "m":
				return m, m.resolveApprovalCmd(*appr, core.ApprovalActionApproveAndRemember)
			case "d":
				return m, m.resolveApprovalCmd(*appr, core.ApprovalActionDeny)
			}
		} else {
			m.focus = focusComposer
		}
	}

	if m.focus == focusSidebar {
		visible := m.visibleSessions()
		switch k.String() {
		case "up", "k":
			if m.sidebarIdx > 0 {
				m.sidebarIdx--
			}
		case "down", "j":
			if m.sidebarIdx+1 < len(visible) {
				m.sidebarIdx++
			}
		case "enter":
			if m.sidebarIdx >= 0 && m.sidebarIdx < len(visible) {
				picked := visible[m.sidebarIdx]
				m.activeSessionID = picked.ID
				m.activeWsID = picked.WorkspaceID
				m.focus = focusComposer
				return m, m.loadSessionCmd(picked.ID)
			}
		}
		return m, nil
	}

	// Composer focus
	switch k.Type {
	case tea.KeyEnter:
		prompt := strings.TrimSpace(m.input)
		if prompt == "" {
			return m, nil
		}
		m.input = ""
		return m.dispatchComposerInput(prompt)

	case tea.KeyBackspace:
		if len(m.input) > 0 {
			_, sz := utf8.DecodeLastRuneInString(m.input)
			m.input = m.input[:len(m.input)-sz]
		}
	case tea.KeySpace:
		m.input += " "
	case tea.KeyRunes:
		m.input += string(k.Runes)
	case tea.KeyPgUp:
		m.scrollOffset += 5
	case tea.KeyPgDown:
		if m.scrollOffset >= 5 {
			m.scrollOffset -= 5
		} else {
			m.scrollOffset = 0
		}
	}
	return m, nil
}

func (m *Model) dispatchComposerInput(prompt string) (tea.Model, tea.Cmd) {
	if strings.HasPrefix(prompt, "/") {
		fields := strings.Fields(prompt)
		cmd := strings.ToLower(fields[0])
		arg := strings.TrimSpace(strings.TrimPrefix(prompt, fields[0]))
		switch cmd {
		case "/quit", "/exit":
			return m, tea.Quit
		case "/new":
			return m, m.createNewSessionCmd()
		case "/role":
			if arg != "" {
				m.agentRole = arg
				m.statusNotice = "Role -> " + arg
			}
			return m, nil
		case "/compact":
			if m.activeSessionID != "" {
				return m, m.postActionCmd(fmt.Sprintf("/api/sessions/%s/compact", url.PathEscape(m.activeSessionID)), map[string]any{}, "Session compacted")
			}
			return m, nil
		case "/fork":
			if m.activeSessionID != "" {
				return m, m.forkSessionCmd(arg)
			}
			return m, nil
		case "/rewind":
			if m.activeSessionID != "" {
				return m, m.postActionCmd(fmt.Sprintf("/api/sessions/%s/rewind", url.PathEscape(m.activeSessionID)), map[string]any{
					"to_seq":        0,
					"restore_files": true,
				}, "Rewound session & restored SHA-256 snapshots")
			}
			return m, nil
		case "/fleet":
			if arg != "" && m.activeWsID != "" {
				return m, m.postActionCmd("/api/fleet/missions", map[string]any{
					"workspace_id": m.activeWsID,
					"workflow_id":  "hybrid-fleet",
					"goal":         arg,
				}, "Launched hybrid-fleet mission")
			}
			return m, nil
		}
	}

	if runID := m.activeRunIDForSession(); runID != "" {
		return m, m.postActionCmd(fmt.Sprintf("/api/runs/%s/steer", url.PathEscape(runID)), map[string]any{
			"message": prompt,
		}, "Injected mid-run Steer")
	}

	return m, m.startRunCmd(prompt)
}

func (m *Model) View() string {
	sideW := 28
	if m.width < 80 {
		sideW = 22
	}
	mainW := m.width - sideW - 3
	if mainW < 30 {
		mainW = 30
	}
	bodyH := m.height - 4
	if bodyH < 8 {
		bodyH = 8
	}

	wsLabel := "no-workspace"
	for _, ws := range m.boot.Workspaces {
		if ws.ID == m.activeWsID {
			wsLabel = config.EscapeBidi(ws.Name)
			break
		}
	}
	modelBadge := m.resolveRoleBadge()
	topBar := headerStyle.Width(m.width).Render(
		fmt.Sprintf("Gorness TUI  ·  %s  ·  role:%s (%s)  ·  approvals:%d",
			wsLabel, config.EscapeBidi(m.agentRole), config.EscapeBidi(modelBadge), len(m.boot.PendingApprovals)),
	)

	// Left Sidebar
	visible := m.visibleSessions()
	var sbLines []string
	sbHeader := "Sessions"
	if m.focus == focusSidebar {
		sbHeader = "▶ Sessions"
	}
	sbLines = append(sbLines, lipgloss.NewStyle().Bold(true).Render(sbHeader))
	for i, s := range visible {
		if len(sbLines) >= bodyH-1 {
			break
		}
		prefix := "  "
		if s.ID == m.activeSessionID {
			prefix = "● "
		}
		if m.focus == focusSidebar && i == m.sidebarIdx {
			prefix = "→ "
		}
		title := truncateRunes(config.EscapeBidi(s.Title), sideW-5)
		line := prefix + title
		if s.ID == m.activeSessionID || (m.focus == focusSidebar && i == m.sidebarIdx) {
			line = selectedItemStyle.Render(line)
		}
		sbLines = append(sbLines, line)
	}
	for len(sbLines) < bodyH {
		sbLines = append(sbLines, "")
	}
	leftCol := sidebarStyle.Width(sideW).Height(bodyH).Render(strings.Join(sbLines, "\n"))

	// Right Main Conversation + Inline Approval + Composer
	var mainLines []string
	for _, ent := range m.entries {
		roleHead := userRoleStyle.Render("USER")
		if ent.Role == "assistant" {
			roleHead = assistantRoleStyle.Render(fmt.Sprintf("ASSISTANT (%s)", config.EscapeBidi(ent.AgentRole)))
		} else if ent.Role == "tool" {
			roleHead = mutedStyle.Render("TOOL")
		}
		mainLines = append(mainLines, fmt.Sprintf("%s  #%d", roleHead, ent.Seq))
		for _, blk := range ent.Blocks {
			switch blk.Type {
			case "thinking":
				if blk.Thinking != "" {
					mainLines = append(mainLines, mutedStyle.Render("  ◌ thinking: "+truncateRunes(collapseNewlines(config.EscapeBidi(blk.Thinking)), mainW-16)))
				}
			case "tool_use":
				mainLines = append(mainLines, mutedStyle.Render(fmt.Sprintf("  ⚙ %s(%s)", config.EscapeBidi(blk.ToolName), truncateRunes(string(blk.ToolInput), mainW-18))))
			case "tool_result":
				tag := "ok"
				if blk.IsError {
					tag = "err"
				}
				mainLines = append(mainLines, mutedStyle.Render(fmt.Sprintf("  ↳ [%s] %s", tag, truncateRunes(collapseNewlines(config.EscapeBidi(blk.Text)), mainW-14))))
			default:
				if blk.Text != "" {
					for _, wl := range wrapText(config.EscapeBidi(blk.Text), mainW-2) {
						mainLines = append(mainLines, "  "+wl)
					}
				}
			}
		}
		mainLines = append(mainLines, "")
	}

	if m.liveThinking != "" || m.liveText != "" {
		mainLines = append(mainLines, assistantRoleStyle.Render("● STREAMING"))
		if m.liveThinking != "" {
			mainLines = append(mainLines, mutedStyle.Render("  ◌ "+truncateRunes(collapseNewlines(config.EscapeBidi(m.liveThinking)), mainW-8)))
		}
		if m.liveText != "" {
			for _, wl := range wrapText(config.EscapeBidi(m.liveText), mainW-2) {
				mainLines = append(mainLines, "  "+wl)
			}
		}
	}

	var approvalRendered string
	approvalHeight := 0
	if appr := m.currentApproval(); appr != nil {
		approvalRendered = m.renderInlineApproval(*appr, mainW-4)
		approvalHeight = strings.Count(approvalRendered, "\n") + 1
	}

	composerPrefix := "> "
	if m.activeRunIDForSession() != "" {
		composerPrefix = "[Steer] > "
	}
	composerLine := composerPrefix + config.EscapeBidi(m.input) + "█"

	availTranscript := bodyH - approvalHeight - 2
	if availTranscript < 3 {
		availTranscript = 3
	}
	if len(mainLines) > availTranscript {
		end := len(mainLines) - m.scrollOffset
		if end < availTranscript {
			end = availTranscript
		}
		if end > len(mainLines) {
			end = len(mainLines)
		}
		start := end - availTranscript
		if start < 0 {
			start = 0
		}
		mainLines = mainLines[start:end]
	}
	for len(mainLines) < availTranscript {
		mainLines = append(mainLines, "")
	}

	var rightParts []string
	rightParts = append(rightParts, strings.Join(mainLines, "\n"))
	if approvalRendered != "" {
		rightParts = append(rightParts, approvalRendered)
	}
	rightParts = append(rightParts, lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0071E3", Dark: "#2997FF"}).Render(truncateRunes(composerLine, mainW)))
	rightCol := lipgloss.NewStyle().Width(mainW).Height(bodyH).Render(strings.Join(rightParts, "\n"))

	middle := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightCol)

	// Fleet & Always-On Footer Status Bar
	runningMissions := 0
	var latestTopo string
	for _, ms := range m.boot.Missions {
		if ms.Status == fleet.MissionRunning {
			runningMissions++
			latestTopo = string(ms.Topology)
		}
	}
	fleetSummary := fmt.Sprintf("Fleet: %d live", runningMissions)
	if latestTopo != "" {
		fleetSummary += " (" + latestTopo + ")"
	}
	aoSummary := fmt.Sprintf("Always-On: %d queued · %d deferred", m.boot.AlwaysOn.QueuedCount, m.boot.AlwaysOn.DeferredCount)
	notice := m.statusNotice
	if notice == "" {
		notice = "Tab:focus · Enter:send/steer · Ctrl+N:new · Ctrl+R:role · Ctrl+X:stop · Ctrl+C:quit"
	}
	footer := statusBarStyle.Width(m.width).Render(
		truncateRunes(fmt.Sprintf("%s  |  %s  |  %s", fleetSummary, aoSummary, config.EscapeBidi(notice)), m.width-2),
	)

	return strings.Join([]string{topBar, middle, footer}, "\n")
}

func (m *Model) renderInlineApproval(appr core.PendingApproval, maxW int) string {
	settled := time.Since(m.surfacedLocal[appr.ID]) >= core.SettleWindow
	stateTag := "500ms settling..."
	if settled {
		stateTag = "y:approve selected · m:remember · d:deny · space:toggle hunk"
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("⚠ Approval Required: %s [%s] (%s)",
		config.EscapeBidi(appr.ToolName), config.EscapeBidi(appr.ReasonCode), stateTag))
	for idx, h := range appr.Hunks {
		if idx >= 3 {
			lines = append(lines, fmt.Sprintf("  ... +%d more hunks", len(appr.Hunks)-3))
			break
		}
		box := "[ ]"
		if m.selectedHunks[appr.ID][h.ID] {
			box = "[x]"
		}
		cur := " "
		if idx == m.hunkCursor {
			cur = ">"
		}
		lines = append(lines, fmt.Sprintf("%s %s %s %s", cur, box, config.EscapeBidi(h.ID), config.EscapeBidi(h.Header)))
		for _, pl := range strings.Split(h.Preview, "\n") {
			if strings.HasPrefix(pl, "+") && !strings.HasPrefix(pl, "+++") {
				lines = append(lines, "    "+diffAddStyle.Render(truncateRunes(config.EscapeBidi(pl), maxW-8)))
			} else if strings.HasPrefix(pl, "-") && !strings.HasPrefix(pl, "---") {
				lines = append(lines, "    "+diffDelStyle.Render(truncateRunes(config.EscapeBidi(pl), maxW-8)))
			}
		}
	}
	return approvalBoxStyle.Width(maxW).Render(strings.Join(lines, "\n"))
}

// ============================================================================
// HTTP / SSE Client Commands
// ============================================================================

func (m *Model) initBootstrapCmd() tea.Cmd {
	return func() tea.Msg {
		if m.initialWsPath != "" {
			_, _ = m.doJSON(http.MethodPost, "/api/workspaces", map[string]string{
				"path": m.initialWsPath,
			})
		}
		var data bootstrapPayload
		raw, err := m.doJSON(http.MethodGet, "/api/bootstrap", nil)
		if err != nil {
			return bootstrapLoadedMsg{err: err}
		}
		if err := json.Unmarshal(raw, &data); err != nil {
			return bootstrapLoadedMsg{err: err}
		}
		return bootstrapLoadedMsg{data: data}
	}
}

func (m *Model) refreshBootstrapCmd() tea.Cmd {
	return func() tea.Msg {
		var data bootstrapPayload
		raw, err := m.doJSON(http.MethodGet, "/api/bootstrap", nil)
		if err != nil {
			return bootstrapLoadedMsg{err: err}
		}
		if err := json.Unmarshal(raw, &data); err != nil {
			return bootstrapLoadedMsg{err: err}
		}
		return bootstrapLoadedMsg{data: data}
	}
}

func (m *Model) loadSessionCmd(sessionID string) tea.Cmd {
	return func() tea.Msg {
		raw, err := m.doJSON(http.MethodGet, "/api/sessions/"+url.PathEscape(sessionID), nil)
		if err != nil {
			return sessionLoadedMsg{err: err}
		}
		var detail sessionDetailPayload
		if err := json.Unmarshal(raw, &detail); err != nil {
			return sessionLoadedMsg{err: err}
		}
		return sessionLoadedMsg{detail: detail}
	}
}

func (m *Model) createNewSessionCmd() tea.Cmd {
	wsID := m.activeWsID
	role := m.agentRole
	return func() tea.Msg {
		if wsID == "" {
			return statusNoticeMsg{err: errors.New("no workspace registered")}
		}
		raw, err := m.doJSON(http.MethodPost, "/api/sessions", map[string]any{
			"workspace_id": wsID,
			"title":        "New Session",
			"agent_role":   role,
		})
		if err != nil {
			return statusNoticeMsg{err: err}
		}
		var meta store.SessionMeta
		if err := json.Unmarshal(raw, &meta); err != nil {
			return statusNoticeMsg{err: err}
		}
		return sessionSwitchedMsg{
			sessionID: meta.ID,
			wsID:      meta.WorkspaceID,
			notice:    "Created session " + meta.ID,
		}
	}
}

func (m *Model) forkSessionCmd(title string) tea.Cmd {
	sid := m.activeSessionID
	return func() tea.Msg {
		raw, err := m.doJSON(http.MethodPost, fmt.Sprintf("/api/sessions/%s/fork", url.PathEscape(sid)), map[string]any{
			"title": title,
		})
		if err != nil {
			return statusNoticeMsg{err: err}
		}
		var meta store.SessionMeta
		if err := json.Unmarshal(raw, &meta); err != nil {
			return statusNoticeMsg{err: err}
		}
		return sessionSwitchedMsg{
			sessionID: meta.ID,
			wsID:      meta.WorkspaceID,
			notice:    "Forked session " + meta.ID,
		}
	}
}

func (m *Model) startRunCmd(prompt string) tea.Cmd {
	wsID := m.activeWsID
	sessID := m.activeSessionID
	role := m.agentRole
	return func() tea.Msg {
		reqBody, _ := json.Marshal(map[string]any{
			"workspace_id": wsID,
			"session_id":   sessID,
			"agent_role":   role,
			"prompt":       prompt,
		})
		req, err := http.NewRequestWithContext(m.ctx, http.MethodPost, m.serverURL+"/api/runs/stream", bytes.NewReader(reqBody))
		if err != nil {
			return statusNoticeMsg{err: err}
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := m.httpClient.Do(req)
		if err != nil {
			return statusNoticeMsg{err: err}
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			raw, _ := io.ReadAll(resp.Body)
			return statusNoticeMsg{err: fmt.Errorf("run HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))}
		}
		// Drain stream; live deltas arrive via global SSE subscription simultaneously
		_, _ = io.Copy(io.Discard, resp.Body)
		return statusNoticeMsg{text: "Turn complete"}
	}
}

func (m *Model) stopRunCmd(runID string) tea.Cmd {
	return m.postActionCmd(fmt.Sprintf("/api/runs/%s/stop", url.PathEscape(runID)), map[string]any{}, "Stopped run "+runID)
}

func (m *Model) resolveApprovalCmd(appr core.PendingApproval, action core.ApprovalAction) tea.Cmd {
	var selected []string
	for hid, ok := range m.selectedHunks[appr.ID] {
		if ok {
			selected = append(selected, hid)
		}
	}
	return m.postActionCmd(fmt.Sprintf("/api/approvals/%s/resolve", url.PathEscape(appr.ID)), map[string]any{
		"action":            action,
		"selected_hunk_ids": selected,
		"remember":          action == core.ApprovalActionApproveAndRemember,
	}, "Resolved approval "+appr.ID)
}

func (m *Model) postActionCmd(path string, payload any, okNotice string) tea.Cmd {
	return func() tea.Msg {
		if _, err := m.doJSON(http.MethodPost, path, payload); err != nil {
			return statusNoticeMsg{err: err}
		}
		return statusNoticeMsg{text: okNotice}
	}
}

func (m *Model) startGlobalSSEWorkerCmd() tea.Cmd {
	return func() tea.Msg {
		go func() {
			req, err := http.NewRequestWithContext(m.ctx, http.MethodGet, m.serverURL+"/api/events/stream", nil)
			if err != nil {
				return
			}
			resp, err := m.httpClient.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()

			sc := bufio.NewScanner(resp.Body)
			sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for sc.Scan() {
				line := sc.Text()
				if !strings.HasPrefix(line, "data:") {
					continue
				}
				raw := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if raw == "" {
					continue
				}
				var ev core.RunEvent
				if json.Unmarshal([]byte(raw), &ev) != nil || ev.Type == "heartbeat" || ev.Type == "connected" {
					continue
				}
				select {
				case m.sseCh <- ev:
				case <-m.ctx.Done():
					return
				}
			}
		}()
		return m.waitNextSSECmd()()
	}
}

func (m *Model) waitNextSSECmd() tea.Cmd {
	return func() tea.Msg {
		select {
		case ev, ok := <-m.sseCh:
			if !ok {
				return globalSSEEventMsg{err: io.EOF}
			}
			return globalSSEEventMsg{ev: ev}
		case <-m.ctx.Done():
			return globalSSEEventMsg{err: m.ctx.Err()}
		}
	}
}

func (m *Model) doJSON(method, path string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(m.ctx, method, m.serverURL+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return raw, nil
}

// ============================================================================
// State & Layout Helpers
// ============================================================================

func (m *Model) visibleSessions() []*store.SessionMeta {
	var out []*store.SessionMeta
	for _, s := range m.boot.Sessions {
		if !s.Archived {
			out = append(out, s)
		}
	}
	return out
}

func (m *Model) activeRunIDForSession() string {
	if m.liveRunID != "" {
		return m.liveRunID
	}
	for _, r := range m.boot.ActiveRuns {
		if r.SessionID == m.activeSessionID {
			return r.RunID
		}
	}
	return ""
}

func (m *Model) currentApproval() *core.PendingApproval {
	for i := range m.boot.PendingApprovals {
		if m.boot.PendingApprovals[i].SessionID == m.activeSessionID {
			return &m.boot.PendingApprovals[i]
		}
	}
	return nil
}

func (m *Model) ensureApprovalTracked(appr core.PendingApproval) {
	if _, ok := m.surfacedLocal[appr.ID]; !ok {
		m.surfacedLocal[appr.ID] = time.Now()
	}
	if _, ok := m.selectedHunks[appr.ID]; !ok {
		set := make(map[string]bool, len(appr.Hunks))
		for _, h := range appr.Hunks {
			set[h.ID] = true
		}
		m.selectedHunks[appr.ID] = set
	}
}

func (m *Model) upsertApproval(appr core.PendingApproval) {
	for i := range m.boot.PendingApprovals {
		if m.boot.PendingApprovals[i].ID == appr.ID {
			m.boot.PendingApprovals[i] = appr
			return
		}
	}
	m.boot.PendingApprovals = append(m.boot.PendingApprovals, appr)
}

func (m *Model) resolveRoleBadge() string {
	if m.boot.Config != nil && m.boot.Config.Agents != nil {
		if prof, ok := m.boot.Config.Agents[m.agentRole]; ok {
			return prof.Provider + "/" + prof.Model
		}
	}
	return "default"
}

func collapseNewlines(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncateRunes(s string, max int) string {
	if max <= 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func wrapText(s string, width int) []string {
	if width <= 8 {
		return []string{s}
	}
	var out []string
	for _, rawLine := range strings.Split(s, "\n") {
		runes := []rune(rawLine)
		if len(runes) == 0 {
			out = append(out, "")
			continue
		}
		for len(runes) > width {
			out = append(out, string(runes[:width]))
			runes = runes[width:]
		}
		out = append(out, string(runes))
	}
	return out
}
