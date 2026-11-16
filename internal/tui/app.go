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
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gorness.dev/gorness/internal/alwayson"
	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/core"
	"gorness.dev/gorness/internal/daemonctl"
	"gorness.dev/gorness/internal/fleet"
	"gorness.dev/gorness/internal/store"
)

type focusArea int

const (
	focusComposer focusArea = iota
	focusSidebar
	focusApproval
)

var defaultRoleOrder = []string{"coder", "lead", "explorer", "debugger", "verifier"}

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

	switcherBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.AdaptiveColor{Light: "#0071E3", Dark: "#2997FF"}).
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
	data           bootstrapPayload
	registeredWsID string
	err            error
}

type workspacesLoadedMsg struct {
	workspaces []*store.Workspace
	err        error
}

type alwaysOnLoadedMsg struct {
	state alwayson.AlwaysOnState
	err   error
}

type missionsLoadedMsg struct {
	missions []*fleet.MissionSnapshot
	summary  string
	err      error
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
	text               string
	reloadSession      bool
	resolvedApprovalID string
	err                error
}

type sessionSwitchedMsg struct {
	sessionID string
	wsID      string
	notice    string
}

type sessionRemovedMsg struct {
	removedID string
	notice    string
}

type approvalSettledTickMsg struct {
	approvalID string
}

// Model 实现 Gorness 多项目多会话终端工作台（流式对话、逐块全量可滚动 Diff 审批、工作区切换器、舰队状态栏）。
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
	inFlightRunID   string
	sidebarIdx      int
	entries         []store.SessionEntry
	agentRole       string
	scrollOffset    int
	input           string
	statusNotice    string

	wsSwitcherOpen bool
	wsCursor       int

	liveThinking string
	liveText     string
	liveRunID    string

	hunkCursor              int
	approvalScroll          int
	editingApprovalFeedback bool
	selectedHunks           map[string]map[string]bool // approvalID -> hunkID -> bool
	rememberApproval        map[string]bool            // approvalID -> remember grant
	approvalFeedback        map[string]string          // approvalID -> feedback text
	surfacedLocal           map[string]time.Time
	seenEventSeq            map[uint64]bool

	sseCh chan core.RunEvent
}

func NewModel(ctx context.Context, serverURL, initialWsPath string) *Model {
	httpClient, cleanURL := daemonctl.ClientForURL(serverURL, "", 0)
	return NewModelWithClient(ctx, cleanURL, initialWsPath, httpClient)
}

func NewModelWithClient(ctx context.Context, serverURL, initialWsPath string, httpClient *http.Client) *Model {
	if ctx == nil {
		ctx = context.Background()
	}
	cleanURL := strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if cleanURL == "" {
		cleanURL = daemonctl.DefaultLoopbackURL
	}
	if httpClient == nil {
		httpClient, cleanURL = daemonctl.ClientForURL(cleanURL, "", 0)
	}
	return &Model{
		ctx:              ctx,
		serverURL:        cleanURL,
		initialWsPath:    strings.TrimSpace(initialWsPath),
		httpClient:       httpClient,
		width:            110,
		height:           32,
		focus:            focusComposer,
		agentRole:        "coder",
		selectedHunks:    make(map[string]map[string]bool),
		rememberApproval: make(map[string]bool),
		approvalFeedback: make(map[string]string),
		surfacedLocal:    make(map[string]time.Time),
		seenEventSeq:     make(map[uint64]bool),
		sseCh:            make(chan core.RunEvent, 128),
	}
}

// Run 启动连接本机 Gorness 守护进程的 Bubble Tea 交互式多项目终端界面。
func Run(ctx context.Context, serverURL, initialWsPath string) error {
	return RunWithOptions(ctx, serverURL, initialWsPath, tea.WithAltScreen())
}

func RunWithOptions(ctx context.Context, serverURL, initialWsPath string, opts ...tea.ProgramOption) error {
	httpClient, cleanURL := daemonctl.ClientForURL(serverURL, "", 0)
	return RunWithClient(ctx, cleanURL, initialWsPath, httpClient, opts...)
}

func RunWithClient(ctx context.Context, serverURL, initialWsPath string, httpClient *http.Client, opts ...tea.ProgramOption) error {
	if ctx == nil {
		ctx = context.Background()
	}
	m := NewModelWithClient(ctx, serverURL, initialWsPath, httpClient)
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
		var cmds []tea.Cmd
		for _, appr := range m.boot.PendingApprovals {
			if m.ensureApprovalTracked(appr) {
				cmds = append(cmds, scheduleApprovalSettleTick(appr.ID))
			}
		}
		if m.activeWsID == "" {
			if typed.registeredWsID != "" {
				m.activeWsID = typed.registeredWsID
			} else if len(m.boot.Workspaces) > 0 {
				m.activeWsID = m.boot.Workspaces[0].ID
			}
		}
		visible := m.visibleSessions()
		if m.activeSessionID != "" && !m.isVisibleSession(m.activeSessionID, visible) {
			m.activeSessionID = ""
			m.entries = nil
		}
		if m.activeSessionID == "" && len(visible) > 0 {
			m.activeSessionID = visible[0].ID
			m.activeWsID = visible[0].WorkspaceID
		}
		if m.sidebarIdx >= len(visible) {
			if len(visible) > 0 {
				m.sidebarIdx = len(visible) - 1
			} else {
				m.sidebarIdx = 0
			}
		}
		if m.currentApproval() != nil && m.focus != focusApproval {
			m.focus = focusApproval
		} else if m.currentApproval() == nil && m.focus == focusApproval {
			m.focus = focusComposer
			m.editingApprovalFeedback = false
		}
		if m.activeSessionID != "" {
			cmds = append(cmds, m.loadSessionCmd(m.activeSessionID))
		}
		if len(cmds) > 0 {
			return m, tea.Batch(cmds...)
		}
		return m, nil

	case workspacesLoadedMsg:
		if typed.err != nil {
			m.statusNotice = "Error: " + typed.err.Error()
			return m, nil
		}
		m.boot.Workspaces = typed.workspaces
		m.syncWorkspaceCursor()
		return m, nil

	case alwaysOnLoadedMsg:
		if typed.err == nil {
			m.boot.AlwaysOn = typed.state
		}
		return m, nil

	case missionsLoadedMsg:
		if typed.err != nil {
			m.statusNotice = "Error: " + typed.err.Error()
			return m, nil
		}
		if typed.missions != nil {
			m.boot.Missions = typed.missions
		}
		if typed.summary != "" {
			m.statusNotice = typed.summary
		}
		return m, nil

	case approvalSettledTickMsg:
		return m, nil

	case sessionLoadedMsg:
		if typed.err == nil {
			m.entries = typed.detail.Entries
			if typed.detail.Meta != nil {
				if typed.detail.Meta.AgentRole != "" {
					m.agentRole = typed.detail.Meta.AgentRole
				}
				m.upsertSessionMeta(typed.detail.Meta)
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

		if ev.Seq > 0 {
			if m.seenEventSeq[ev.Seq] {
				return m, tea.Batch(cmds...)
			}
			m.seenEventSeq[ev.Seq] = true
		}

		adoptIfNewSession := func(sessID, wsID, runID string) {
			if sessID == "" {
				return
			}
			if (m.activeSessionID == "" || m.inFlightRunID == "pending") && (m.activeWsID == "" || wsID == "" || wsID == m.activeWsID) {
				m.activeSessionID = sessID
				if m.activeWsID == "" && wsID != "" {
					m.activeWsID = wsID
				}
				if runID != "" {
					m.inFlightRunID = runID
				}
				cmds = append(cmds, m.refreshBootstrapCmd(), m.loadSessionCmd(m.activeSessionID))
			} else if runID != "" && m.inFlightRunID != "" && runID == m.inFlightRunID && m.activeSessionID != sessID {
				m.activeSessionID = sessID
				if wsID != "" {
					m.activeWsID = wsID
				}
				cmds = append(cmds, m.refreshBootstrapCmd(), m.loadSessionCmd(m.activeSessionID))
			}
		}

		switch ev.Type {
		case core.EventRunStarted:
			adoptIfNewSession(ev.SessionID, ev.WorkspaceID, ev.RunID)
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
			adoptIfNewSession(ev.SessionID, ev.WorkspaceID, ev.RunID)
			if ev.SessionID == m.activeSessionID {
				if mp, ok := ev.Payload.(map[string]any); ok {
					if s, _ := mp["thinking"].(string); s != "" {
						m.liveThinking += s
					}
				}
			}
		case core.EventTextDelta:
			adoptIfNewSession(ev.SessionID, ev.WorkspaceID, ev.RunID)
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
				sessID := appr.SessionID
				if sessID == "" {
					sessID = ev.SessionID
				}
				wsID := appr.WorkspaceID
				if wsID == "" {
					wsID = ev.WorkspaceID
				}
				adoptIfNewSession(sessID, wsID, appr.RunID)
				if m.ensureApprovalTracked(appr) {
					cmds = append(cmds, scheduleApprovalSettleTick(appr.ID))
				}
				m.upsertApproval(appr)
				if sessID == m.activeSessionID || m.activeSessionID == "" {
					m.focus = focusApproval
					m.hunkCursor = 0
					m.approvalScroll = 0
					m.editingApprovalFeedback = false
				}
			}
		case core.EventApprovalResolved:
			cmds = append(cmds, m.refreshBootstrapCmd())
		case core.EventAlwaysOnUpdated:
			cmds = append(cmds, m.fetchAlwaysOnCmd())
		case core.EventFleetUpdated:
			cmds = append(cmds, m.fetchMissionsCmd(false))
		case core.EventConfigReloaded, core.EventSessionUpdated:
			cmds = append(cmds, m.refreshBootstrapCmd())
			if ev.SessionID != "" && ev.SessionID == m.activeSessionID {
				cmds = append(cmds, m.loadSessionCmd(m.activeSessionID))
			}
		case core.EventRunCompleted, core.EventRunFailed, core.EventRunCancelled, core.EventCompacted, core.EventTitleUpdated:
			if m.inFlightRunID != "" && ev.RunID == m.inFlightRunID {
				m.inFlightRunID = ""
			}
			adoptIfNewSession(ev.SessionID, ev.WorkspaceID, ev.RunID)
			if ev.SessionID == m.activeSessionID && m.activeSessionID != "" {
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
			return m, m.refreshBootstrapCmd()
		}
		if typed.resolvedApprovalID != "" {
			m.removeApprovalLocal(typed.resolvedApprovalID)
		}
		m.statusNotice = typed.text
		if typed.reloadSession && m.activeSessionID != "" {
			return m, tea.Batch(m.refreshBootstrapCmd(), m.loadSessionCmd(m.activeSessionID))
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

	case sessionRemovedMsg:
		if m.activeSessionID == typed.removedID {
			m.activeSessionID = ""
			m.entries = nil
			m.liveThinking = ""
			m.liveText = ""
			m.liveRunID = ""
		}
		var remaining []*store.SessionMeta
		for _, s := range m.boot.Sessions {
			if s.ID == typed.removedID {
				continue
			}
			remaining = append(remaining, s)
		}
		m.boot.Sessions = remaining
		visible := m.visibleSessions()
		if m.activeSessionID == "" && len(visible) > 0 {
			m.activeSessionID = visible[0].ID
		}
		m.statusNotice = typed.notice
		if m.activeSessionID != "" {
			return m, tea.Batch(m.refreshBootstrapCmd(), m.loadSessionCmd(m.activeSessionID))
		}
		return m, m.refreshBootstrapCmd()

	case tea.KeyMsg:
		return m.handleKey(typed)
	}

	return m, nil
}

func (m *Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.String() == "ctrl+c" {
		return m, tea.Quit
	}

	if k.String() == "ctrl+w" {
		if m.wsSwitcherOpen {
			m.wsSwitcherOpen = false
			return m, nil
		}
		m.wsSwitcherOpen = true
		m.syncWorkspaceCursor()
		return m, m.fetchWorkspacesCmd()
	}

	if m.wsSwitcherOpen {
		return m.handleWorkspaceSwitcherKey(k)
	}

	switch k.String() {
	case "tab":
		if m.editingApprovalFeedback {
			m.editingApprovalFeedback = false
			return m, nil
		}
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
		return m, m.createNewSessionCmd("New Session", false)

	case "ctrl+r":
		roles := m.availableAgentRoles()
		if len(roles) == 0 {
			m.agentRole = "coder"
			return m, nil
		}
		nextRole := roles[0]
		for i, r := range roles {
			if r == m.agentRole {
				nextRole = roles[(i+1)%len(roles)]
				break
			}
		}
		m.agentRole = nextRole
		m.statusNotice = "Role -> " + m.agentRole
		if m.activeSessionID != "" {
			return m, m.patchSessionCmd(m.activeSessionID, map[string]any{"agent_role": m.agentRole}, "Role -> "+m.agentRole)
		}
		return m, nil

	case "ctrl+x":
		if runID := m.activeRunIDForSession(); runID != "" {
			return m, m.stopRunCmd(runID)
		}
		return m, nil
	}

	if m.focus == focusApproval {
		if appr := m.currentApproval(); appr != nil {
			return m.handleApprovalKey(*appr, k)
		}
		m.focus = focusComposer
		m.editingApprovalFeedback = false
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

func (m *Model) handleWorkspaceSwitcherKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "q":
		m.wsSwitcherOpen = false
		return m, nil
	case "up", "k":
		if m.wsCursor > 0 {
			m.wsCursor--
		}
		return m, nil
	case "down", "j":
		if m.wsCursor+1 < len(m.boot.Workspaces) {
			m.wsCursor++
		}
		return m, nil
	case "enter":
		m.wsSwitcherOpen = false
		if m.wsCursor < 0 || m.wsCursor >= len(m.boot.Workspaces) {
			return m, nil
		}
		picked := m.boot.Workspaces[m.wsCursor]
		m.activeWsID = picked.ID
		m.activeSessionID = ""
		m.sidebarIdx = 0
		m.entries = nil
		m.liveThinking = ""
		m.liveText = ""
		m.liveRunID = ""
		m.statusNotice = "Workspace -> " + picked.Name
		visible := m.visibleSessions()
		if len(visible) > 0 {
			m.activeSessionID = visible[0].ID
			return m, tea.Batch(m.refreshBootstrapCmd(), m.loadSessionCmd(m.activeSessionID))
		}
		return m, m.refreshBootstrapCmd()
	}
	return m, nil
}

func (m *Model) handleApprovalKey(appr core.PendingApproval, k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.editingApprovalFeedback {
		switch k.Type {
		case tea.KeyEsc, tea.KeyEnter:
			m.editingApprovalFeedback = false
			return m, nil
		case tea.KeyBackspace:
			cur := m.approvalFeedback[appr.ID]
			if len(cur) > 0 {
				_, sz := utf8.DecodeLastRuneInString(cur)
				m.approvalFeedback[appr.ID] = cur[:len(cur)-sz]
			}
			return m, nil
		case tea.KeySpace:
			m.approvalFeedback[appr.ID] += " "
			return m, nil
		case tea.KeyRunes:
			m.approvalFeedback[appr.ID] += string(k.Runes)
			return m, nil
		}
		return m, nil
	}

	switch k.String() {
	case "up", "k":
		if len(appr.Hunks) > 0 && m.hunkCursor > 0 {
			m.hunkCursor--
			m.syncApprovalScrollToCursor(appr)
		} else if m.approvalScroll > 0 {
			m.approvalScroll--
		}
		return m, nil
	case "down", "j":
		if len(appr.Hunks) > 0 && m.hunkCursor+1 < len(appr.Hunks) {
			m.hunkCursor++
			m.syncApprovalScrollToCursor(appr)
		} else {
			m.approvalScroll++
		}
		return m, nil
	case "pgup", "[", "ctrl+u":
		if m.approvalScroll >= 5 {
			m.approvalScroll -= 5
		} else {
			m.approvalScroll = 0
		}
		return m, nil
	case "pgdown", "]", "ctrl+d":
		m.approvalScroll += 5
		return m, nil
	case " ":
		if m.hunkCursor >= 0 && m.hunkCursor < len(appr.Hunks) {
			hid := appr.Hunks[m.hunkCursor].ID
			m.selectedHunks[appr.ID][hid] = !m.selectedHunks[appr.ID][hid]
		}
		return m, nil
	case "r":
		m.rememberApproval[appr.ID] = !m.rememberApproval[appr.ID]
		return m, nil
	case "f":
		m.editingApprovalFeedback = true
		return m, nil
	case "a", "y", "enter":
		remember := m.rememberApproval[appr.ID]
		action := core.ApprovalActionApproveAll
		if len(appr.Hunks) > 0 {
			action = core.ApprovalActionApproveHunks
		} else if remember {
			action = core.ApprovalActionApproveAndRemember
		}
		return m, m.resolveApprovalCmd(appr, action, remember)
	case "m":
		return m, m.resolveApprovalCmd(appr, core.ApprovalActionApproveAndRemember, true)
	case "d":
		return m, m.resolveApprovalCmd(appr, core.ApprovalActionDeny, false)
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
			isolateGit := false
			var titleParts []string
			for _, tok := range fields[1:] {
				if tok == "--worktree" || tok == "-w" {
					isolateGit = true
					continue
				}
				titleParts = append(titleParts, tok)
			}
			title := strings.TrimSpace(strings.Join(titleParts, " "))
			if title == "" {
				title = "New Session"
			}
			return m, m.createNewSessionCmd(title, isolateGit)

		case "/role":
			if arg != "" {
				m.agentRole = arg
				m.statusNotice = "Role -> " + arg
				if m.activeSessionID != "" {
					return m, m.patchSessionCmd(m.activeSessionID, map[string]any{"agent_role": arg}, "Role -> "+arg)
				}
			}
			return m, nil

		case "/perm":
			if arg != "" && m.activeWsID != "" {
				return m, m.patchWorkspaceCmd(m.activeWsID, map[string]any{"perm_mode": arg}, "Workspace perm -> "+arg)
			}
			return m, nil

		case "/compact":
			if m.activeSessionID == "" {
				m.statusNotice = "No active session to compact"
				return m, nil
			}
			return m, m.postSessionActionCmd(
				fmt.Sprintf("/api/sessions/%s/compact", url.PathEscape(m.activeSessionID)),
				map[string]any{},
				"Session compacted",
			)

		case "/fork":
			if m.activeSessionID == "" {
				m.statusNotice = "No active session to fork"
				return m, nil
			}
			return m, m.forkSessionCmd(arg)

		case "/rewind":
			return m, m.rewindSessionCmd(fields[1:])

		case "/delete":
			if m.activeSessionID == "" {
				m.statusNotice = "No active session to delete"
				return m, nil
			}
			force := false
			for _, tok := range fields[1:] {
				if tok == "--force" || tok == "-f" {
					force = true
				}
			}
			return m, m.deleteSessionCmd(m.activeSessionID, force)

		case "/archive":
			if m.activeSessionID == "" {
				m.statusNotice = "No active session to archive"
				return m, nil
			}
			return m, m.archiveSessionCmd(m.activeSessionID)

		case "/land":
			if m.activeSessionID == "" {
				m.statusNotice = "No active session to land"
				return m, nil
			}
			return m, m.landSessionCmd(m.activeSessionID, arg)

		case "/alwayson":
			return m, m.fetchAlwaysOnCmd()

		case "/fleet":
			return m, m.dispatchFleetSlashCmd(fields[1:], arg)
		}
	}

	if runID := m.activeRunIDForSession(); runID != "" {
		return m, m.postActionCmd(fmt.Sprintf("/api/runs/%s/steer", url.PathEscape(runID)), map[string]any{
			"instruction": prompt,
		}, "Injected mid-run Steer")
	}

	if m.activeSessionID == "" {
		m.inFlightRunID = "pending"
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
	wtBadge := ""
	if meta := m.activeSessionMeta(); meta != nil && strings.TrimSpace(meta.WorktreePath) != "" {
		wtBadge = "  ·  [worktree]"
	}
	modelBadge := m.resolveRoleBadge()
	topBar := headerStyle.Width(m.width).Render(
		fmt.Sprintf("Gorness TUI  ·  %s%s  ·  role:%s (%s)  ·  approvals:%d",
			wsLabel, wtBadge, config.EscapeBidi(m.agentRole), config.EscapeBidi(modelBadge), len(m.boot.PendingApprovals)),
	)

	// Left Sidebar (filtered to active workspace)
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
		wtMark := ""
		if strings.TrimSpace(s.WorktreePath) != "" {
			wtMark = "⎇ "
		}
		title := truncateRunes(wtMark+config.EscapeBidi(s.Title), sideW-5)
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

	// Right Main Conversation + Workspace Switcher / Inline Approval + Composer
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

	var overlayRendered string
	overlayHeight := 0
	if m.wsSwitcherOpen {
		overlayRendered = m.renderWorkspaceSwitcher(mainW - 4)
		overlayHeight = strings.Count(overlayRendered, "\n") + 1
	} else if appr := m.currentApproval(); appr != nil {
		maxDiffViewport := bodyH / 2
		if maxDiffViewport < 8 {
			maxDiffViewport = 8
		}
		overlayRendered = m.renderInlineApproval(*appr, mainW-4, maxDiffViewport)
		overlayHeight = strings.Count(overlayRendered, "\n") + 1
	}

	composerPrefix := "> "
	if m.activeRunIDForSession() != "" {
		composerPrefix = "[Steer] > "
	}
	composerLine := composerPrefix + config.EscapeBidi(m.input) + "█"

	availTranscript := bodyH - overlayHeight - 2
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
	if overlayRendered != "" {
		rightParts = append(rightParts, overlayRendered)
	}
	rightParts = append(rightParts, lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0071E3", Dark: "#2997FF"}).Render(truncateRunes(composerLine, mainW)))
	rightCol := lipgloss.NewStyle().Width(mainW).Height(bodyH).Render(strings.Join(rightParts, "\n"))

	middle := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightCol)

	// Fleet & Always-On Footer Status Bar
	fleetSummary := m.formatFleetStatusBar()
	aoSummary := fmt.Sprintf("Always-On: %d queued · %d deferred", m.boot.AlwaysOn.QueuedCount, m.boot.AlwaysOn.DeferredCount)
	notice := m.statusNotice
	if notice == "" {
		notice = "Tab:focus · Ctrl+W:workspaces · Ctrl+N:new · Ctrl+R:role · Ctrl+X:stop · Ctrl+C:quit"
	}
	footer := statusBarStyle.Width(m.width).Render(
		truncateRunes(fmt.Sprintf("%s  |  %s  |  %s", fleetSummary, aoSummary, config.EscapeBidi(notice)), m.width-2),
	)

	return strings.Join([]string{topBar, middle, footer}, "\n")
}

func (m *Model) renderWorkspaceSwitcher(maxW int) string {
	var lines []string
	lines = append(lines, "⇄ Switch Workspace (↑/↓:select · Enter:switch · Esc:close)")
	if len(m.boot.Workspaces) == 0 {
		lines = append(lines, mutedStyle.Render("  (no workspaces registered)"))
	} else {
		for i, ws := range m.boot.Workspaces {
			cur := "  "
			if i == m.wsCursor {
				cur = "→ "
			}
			activeMark := " "
			if ws.ID == m.activeWsID {
				activeMark = "●"
			}
			row := fmt.Sprintf("%s%s %s  (%s)",
				cur,
				activeMark,
				config.EscapeBidi(ws.Name),
				truncateRunes(config.EscapeBidi(ws.RootPath), maxW-24),
			)
			if i == m.wsCursor {
				row = selectedItemStyle.Render(row)
			}
			lines = append(lines, row)
		}
	}
	return switcherBoxStyle.Width(maxW).Render(strings.Join(lines, "\n"))
}

func (m *Model) buildApprovalDiffLines(appr core.PendingApproval, maxW int) ([]string, []int) {
	var diffLines []string
	hunkStartIdx := make([]int, len(appr.Hunks))
	for idx, h := range appr.Hunks {
		hunkStartIdx[idx] = len(diffLines)
		box := "[ ]"
		if m.selectedHunks[appr.ID][h.ID] {
			box = "[x]"
		}
		cur := " "
		if idx == m.hunkCursor {
			cur = ">"
		}
		head := fmt.Sprintf("%s %s %s %s", cur, box, config.EscapeBidi(h.ID), config.EscapeBidi(h.Header))
		if idx == m.hunkCursor {
			head = selectedItemStyle.Render(head)
		}
		diffLines = append(diffLines, head)
		for _, pl := range strings.Split(h.Preview, "\n") {
			if pl == "" {
				continue
			}
			escaped := truncateRunes(config.EscapeBidi(pl), maxW-8)
			if strings.HasPrefix(pl, "+") && !strings.HasPrefix(pl, "+++") {
				diffLines = append(diffLines, "    "+diffAddStyle.Render(escaped))
			} else if strings.HasPrefix(pl, "-") && !strings.HasPrefix(pl, "---") {
				diffLines = append(diffLines, "    "+diffDelStyle.Render(escaped))
			} else {
				diffLines = append(diffLines, "    "+mutedStyle.Render(escaped))
			}
		}
	}
	return diffLines, hunkStartIdx
}

func (m *Model) syncApprovalScrollToCursor(appr core.PendingApproval) {
	if len(appr.Hunks) == 0 || m.hunkCursor < 0 || m.hunkCursor >= len(appr.Hunks) {
		return
	}
	_, starts := m.buildApprovalDiffLines(appr, m.width)
	target := starts[m.hunkCursor]
	if target < m.approvalScroll {
		m.approvalScroll = target
	} else if target > m.approvalScroll+5 {
		m.approvalScroll = target - 2
	}
}

func (m *Model) renderInlineApproval(appr core.PendingApproval, maxW int, maxViewportLines int) string {
	settled := time.Since(m.surfacedLocal[appr.ID]) >= core.SettleWindow
	stateTag := "500ms settling..."
	if m.editingApprovalFeedback {
		stateTag = "typing feedback — Enter/Esc:done"
	} else if settled {
		stateTag = "y:approve selected · r:remember · m:approve+remember · f:feedback · d:deny · space:toggle hunk"
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("⚠ Approval Required: %s [%s] (%s)",
		config.EscapeBidi(appr.ToolName), config.EscapeBidi(appr.ReasonCode), stateTag))
	if reason := strings.TrimSpace(appr.Reason); reason != "" {
		lines = append(lines, "  Reason: "+truncateRunes(collapseNewlines(config.EscapeBidi(reason)), maxW-12))
	}
	if compactArgs := compactJSONArgs(appr.Args); compactArgs != "" && compactArgs != "{}" {
		lines = append(lines, "  Args:   "+truncateRunes(collapseNewlines(config.EscapeBidi(compactArgs)), maxW-12))
	}

	remBox := "[ ]"
	if m.rememberApproval[appr.ID] {
		remBox = "[x]"
	}
	fbText := m.approvalFeedback[appr.ID]
	var fbDisplay string
	if m.editingApprovalFeedback {
		fbDisplay = config.EscapeBidi(fbText) + "█"
	} else if strings.TrimSpace(fbText) != "" {
		fbDisplay = config.EscapeBidi(fbText)
	} else {
		fbDisplay = mutedStyle.Render("(press f to add feedback)")
	}
	lines = append(lines, fmt.Sprintf("  Remember: %s  ·  Feedback: %s", remBox, fbDisplay))

	if len(appr.Hunks) > 0 {
		diffLines, _ := m.buildApprovalDiffLines(appr, maxW)
		avail := maxViewportLines - len(lines) - 1
		if avail < 4 {
			avail = 4
		}
		maxScroll := 0
		if len(diffLines) > avail {
			maxScroll = len(diffLines) - avail
		}
		scroll := m.approvalScroll
		if scroll > maxScroll {
			scroll = maxScroll
		}
		if scroll < 0 {
			scroll = 0
		}
		end := scroll + avail
		if end > len(diffLines) {
			end = len(diffLines)
		}
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("  Diff Hunks (%d/%d · lines %d-%d of %d · j/k:hunk · [/]:scroll)",
			m.hunkCursor+1, len(appr.Hunks), scroll+1, end, len(diffLines))))
		lines = append(lines, diffLines[scroll:end]...)
	}

	return approvalBoxStyle.Width(maxW).Render(strings.Join(lines, "\n"))
}

// ============================================================================
// HTTP / SSE Client Commands
// ============================================================================

func (m *Model) initBootstrapCmd() tea.Cmd {
	return func() tea.Msg {
		var registeredID string
		if m.initialWsPath != "" {
			if rawWS, err := m.doJSON(http.MethodPost, "/api/workspaces", map[string]string{
				"path": m.initialWsPath,
			}); err == nil {
				var ws store.Workspace
				if json.Unmarshal(rawWS, &ws) == nil {
					registeredID = ws.ID
				}
			}
		}
		var data bootstrapPayload
		raw, err := m.doJSON(http.MethodGet, "/api/bootstrap", nil)
		if err != nil {
			return bootstrapLoadedMsg{err: err}
		}
		if err := json.Unmarshal(raw, &data); err != nil {
			return bootstrapLoadedMsg{err: err}
		}
		return bootstrapLoadedMsg{data: data, registeredWsID: registeredID}
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

func (m *Model) fetchWorkspacesCmd() tea.Cmd {
	return func() tea.Msg {
		raw, err := m.doJSON(http.MethodGet, "/api/workspaces", nil)
		if err != nil {
			return workspacesLoadedMsg{err: err}
		}
		var list []*store.Workspace
		if err := json.Unmarshal(raw, &list); err != nil {
			return workspacesLoadedMsg{err: err}
		}
		return workspacesLoadedMsg{workspaces: list}
	}
}

func (m *Model) fetchAlwaysOnCmd() tea.Cmd {
	return func() tea.Msg {
		raw, err := m.doJSON(http.MethodGet, "/api/alwayson", nil)
		if err != nil {
			return alwaysOnLoadedMsg{err: err}
		}
		var st alwayson.AlwaysOnState
		if err := json.Unmarshal(raw, &st); err != nil {
			return alwaysOnLoadedMsg{err: err}
		}
		return alwaysOnLoadedMsg{state: st}
	}
}

func (m *Model) fetchMissionsCmd(showNotice bool) tea.Cmd {
	wsID := m.activeWsID
	return func() tea.Msg {
		q := "/api/fleet/missions"
		if wsID != "" {
			q += "?workspace_id=" + url.QueryEscape(wsID)
		}
		raw, err := m.doJSON(http.MethodGet, q, nil)
		if err != nil {
			return missionsLoadedMsg{err: err}
		}
		var list []*fleet.MissionSnapshot
		if err := json.Unmarshal(raw, &list); err != nil {
			return missionsLoadedMsg{err: err}
		}
		var summary string
		if showNotice {
			if len(list) == 0 {
				summary = "Fleet: no missions in workspace"
			} else {
				summary = formatMissionSnapshotSummary(list[0])
			}
		}
		return missionsLoadedMsg{missions: list, summary: summary}
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

func (m *Model) createNewSessionCmd(title string, isolateGit bool) tea.Cmd {
	wsID := m.activeWsID
	role := m.agentRole
	return func() tea.Msg {
		if wsID == "" {
			return statusNoticeMsg{err: errors.New("no workspace registered")}
		}
		payload := map[string]any{
			"workspace_id": wsID,
			"title":        title,
			"agent_role":   role,
		}
		if isolateGit {
			payload["isolate_git"] = true
		}
		raw, err := m.doJSON(http.MethodPost, "/api/sessions", payload)
		if err != nil {
			return statusNoticeMsg{err: err}
		}
		var meta store.SessionMeta
		if err := json.Unmarshal(raw, &meta); err != nil {
			return statusNoticeMsg{err: err}
		}
		notice := "Created session " + meta.ID
		if strings.TrimSpace(meta.WorktreePath) != "" {
			notice += " (isolated git worktree)"
		}
		return sessionSwitchedMsg{
			sessionID: meta.ID,
			wsID:      meta.WorkspaceID,
			notice:    notice,
		}
	}
}

func (m *Model) patchSessionCmd(sessionID string, patch map[string]any, okNotice string) tea.Cmd {
	return func() tea.Msg {
		if _, err := m.doJSON(http.MethodPatch, "/api/sessions/"+url.PathEscape(sessionID), patch); err != nil {
			return statusNoticeMsg{err: err}
		}
		return statusNoticeMsg{text: okNotice}
	}
}

func (m *Model) patchWorkspaceCmd(wsID string, patch map[string]any, okNotice string) tea.Cmd {
	return func() tea.Msg {
		if _, err := m.doJSON(http.MethodPatch, "/api/workspaces/"+url.PathEscape(wsID), patch); err != nil {
			return statusNoticeMsg{err: err}
		}
		return statusNoticeMsg{text: okNotice}
	}
}

func (m *Model) archiveSessionCmd(sessionID string) tea.Cmd {
	return func() tea.Msg {
		_, err := m.doJSON(http.MethodPatch, "/api/sessions/"+url.PathEscape(sessionID), map[string]any{
			"archived": true,
		})
		if err != nil {
			return statusNoticeMsg{err: err}
		}
		return sessionRemovedMsg{
			removedID: sessionID,
			notice:    "Archived session " + sessionID,
		}
	}
}

func (m *Model) deleteSessionCmd(sessionID string, force bool) tea.Cmd {
	return func() tea.Msg {
		path := "/api/sessions/" + url.PathEscape(sessionID)
		if force {
			path += "?force=true"
		}
		if _, err := m.doJSON(http.MethodDelete, path, nil); err != nil {
			return statusNoticeMsg{err: err}
		}
		return sessionRemovedMsg{
			removedID: sessionID,
			notice:    "Deleted session " + sessionID,
		}
	}
}

func (m *Model) landSessionCmd(sessionID, verifyCmd string) tea.Cmd {
	return func() tea.Msg {
		body := map[string]any{}
		if strings.TrimSpace(verifyCmd) != "" {
			body["verify_command"] = strings.TrimSpace(verifyCmd)
		}
		raw, err := m.doJSON(http.MethodPost, fmt.Sprintf("/api/sessions/%s/land", url.PathEscape(sessionID)), body)
		if err != nil {
			return statusNoticeMsg{err: err}
		}
		var res struct {
			CommitSHA string `json:"commit_sha"`
			Branch    string `json:"branch"`
			Message   string `json:"message"`
		}
		_ = json.Unmarshal(raw, &res)
		notice := "Landed session worktree " + sessionID
		if res.CommitSHA != "" {
			short := res.CommitSHA
			if len(short) > 8 {
				short = short[:8]
			}
			notice += " (" + short + ")"
		}
		return statusNoticeMsg{text: notice, reloadSession: true}
	}
}

func (m *Model) rewindSessionCmd(tokens []string) tea.Cmd {
	sid := m.activeSessionID
	if sid == "" {
		return func() tea.Msg {
			return statusNoticeMsg{err: errors.New("no active session to rewind")}
		}
	}

	restoreFiles := true
	var explicitSeq *uint64
	for _, tok := range tokens {
		if tok == "--no-files" || tok == "--keep-files" {
			restoreFiles = false
			continue
		}
		val, err := strconv.ParseUint(tok, 10, 64)
		if err != nil {
			badTok := tok
			return func() tea.Msg {
				return statusNoticeMsg{err: fmt.Errorf("invalid /rewind target %q (usage: /rewind [seq] [--no-files])", badTok)}
			}
		}
		explicitSeq = &val
	}

	var targetSeq uint64
	if explicitSeq != nil {
		targetSeq = *explicitSeq
	} else {
		maxSeq := m.maxKnownSessionSeq()
		if maxSeq == 0 {
			return func() tea.Msg {
				return statusNoticeMsg{err: errors.New("session has no turns to rewind")}
			}
		}
		if maxSeq == 1 {
			return func() tea.Msg {
				return statusNoticeMsg{err: errors.New("already at #1; use '/rewind 0' to clear session")}
			}
		}
		targetSeq = maxSeq - 1
	}

	return func() tea.Msg {
		_, err := m.doJSON(http.MethodPost, fmt.Sprintf("/api/sessions/%s/rewind", url.PathEscape(sid)), map[string]any{
			"to_seq":        targetSeq,
			"restore_files": restoreFiles,
		})
		if err != nil {
			return statusNoticeMsg{err: err}
		}
		msg := fmt.Sprintf("Rewound session to #%d", targetSeq)
		if restoreFiles {
			msg += " & restored snapshots"
		} else {
			msg += " (files untouched)"
		}
		return statusNoticeMsg{text: msg, reloadSession: true}
	}
}

func (m *Model) dispatchFleetSlashCmd(tokens []string, rawArg string) tea.Cmd {
	if strings.TrimSpace(rawArg) == "" {
		return m.fetchMissionsCmd(true)
	}
	wsID := m.activeWsID
	if wsID == "" {
		return func() tea.Msg {
			return statusNoticeMsg{err: errors.New("no workspace selected for /fleet")}
		}
	}

	workflowID := "hybrid-fleet"
	goal := strings.TrimSpace(rawArg)

	if len(tokens) >= 2 && (tokens[0] == "--workflow" || tokens[0] == "-w") {
		workflowID = tokens[1]
		goal = strings.TrimSpace(strings.Join(tokens[2:], " "))
	} else if len(tokens) >= 1 && strings.HasPrefix(tokens[0], "--workflow=") {
		workflowID = strings.TrimPrefix(tokens[0], "--workflow=")
		goal = strings.TrimSpace(strings.Join(tokens[1:], " "))
	} else if len(tokens) >= 2 && m.isKnownWorkflowOrTopology(tokens[0]) {
		workflowID = m.resolveCanonicalWorkflowID(tokens[0])
		goal = strings.TrimSpace(strings.Join(tokens[1:], " "))
	}

	if goal == "" {
		return func() tea.Msg {
			return statusNoticeMsg{err: errors.New("/fleet requires a mission <goal>")}
		}
	}

	return func() tea.Msg {
		raw, err := m.doJSON(http.MethodPost, "/api/fleet/missions", map[string]any{
			"workspace_id": wsID,
			"workflow_id":  workflowID,
			"goal":         goal,
		})
		if err != nil {
			return statusNoticeMsg{err: err}
		}
		var snap fleet.MissionSnapshot
		if json.Unmarshal(raw, &snap) == nil && snap.ID != "" {
			return missionsLoadedMsg{
				missions: []*fleet.MissionSnapshot{&snap},
				summary:  formatMissionSnapshotSummary(&snap),
			}
		}
		return statusNoticeMsg{text: fmt.Sprintf("Launched %s mission", workflowID)}
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
		// Forward SSE frames into m.sseCh (deduplicated by RunEvent.Seq in Update) so zero events are missed on startup.
		m.forwardSSEResponseBody(resp.Body)
		return statusNoticeMsg{text: "Turn complete"}
	}
}

func (m *Model) stopRunCmd(runID string) tea.Cmd {
	return m.postActionCmd(fmt.Sprintf("/api/runs/%s/stop", url.PathEscape(runID)), map[string]any{}, "Stopped run "+runID)
}

func (m *Model) resolveApprovalCmd(appr core.PendingApproval, action core.ApprovalAction, remember bool) tea.Cmd {
	var selected []string
	for hid, ok := range m.selectedHunks[appr.ID] {
		if ok {
			selected = append(selected, hid)
		}
	}
	sort.Strings(selected)
	feedback := strings.TrimSpace(m.approvalFeedback[appr.ID])
	if action == core.ApprovalActionApproveAndRemember {
		remember = true
	}

	payload := map[string]any{
		"action":            action,
		"selected_hunk_ids": selected,
		"remember":          remember,
	}
	if feedback != "" {
		payload["feedback"] = feedback
	}

	apprID := appr.ID
	return func() tea.Msg {
		_, err := m.doJSON(http.MethodPost, fmt.Sprintf("/api/approvals/%s/resolve", url.PathEscape(apprID)), payload)
		if err != nil {
			return statusNoticeMsg{resolvedApprovalID: apprID, err: err}
		}
		return statusNoticeMsg{
			text:               "Resolved approval " + apprID,
			resolvedApprovalID: apprID,
		}
	}
}

func (m *Model) postActionCmd(path string, payload any, okNotice string) tea.Cmd {
	return func() tea.Msg {
		if _, err := m.doJSON(http.MethodPost, path, payload); err != nil {
			return statusNoticeMsg{err: err}
		}
		return statusNoticeMsg{text: okNotice}
	}
}

func (m *Model) postSessionActionCmd(path string, payload any, okNotice string) tea.Cmd {
	return func() tea.Msg {
		if _, err := m.doJSON(http.MethodPost, path, payload); err != nil {
			return statusNoticeMsg{err: err}
		}
		return statusNoticeMsg{text: okNotice, reloadSession: true}
	}
}

func (m *Model) startGlobalSSEWorkerCmd() tea.Cmd {
	return func() tea.Msg {
		go m.runGlobalSSELoop()
		return m.waitNextSSECmd()()
	}
}

func (m *Model) runGlobalSSELoop() {
	backoff := 100 * time.Millisecond
	const maxBackoff = 2 * time.Second

	for {
		if m.ctx.Err() != nil {
			return
		}
		connected := false
		req, err := http.NewRequestWithContext(m.ctx, http.MethodGet, m.serverURL+"/api/events/stream", nil)
		if err == nil {
			resp, doErr := m.httpClient.Do(req)
			if doErr == nil {
				if resp.StatusCode == http.StatusOK {
					connected = true
					m.forwardSSEResponseBody(resp.Body)
				}
				_ = resp.Body.Close()
			}
		}
		if m.ctx.Err() != nil {
			return
		}
		if connected {
			backoff = 100 * time.Millisecond
		}
		timer := time.NewTimer(backoff)
		select {
		case <-timer.C:
		case <-m.ctx.Done():
			timer.Stop()
			return
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func (m *Model) forwardSSEResponseBody(body io.Reader) {
	sc := bufio.NewScanner(body)
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

func (m *Model) availableAgentRoles() []string {
	if m.boot.Config == nil || len(m.boot.Config.Agents) == 0 {
		out := make([]string, len(defaultRoleOrder))
		copy(out, defaultRoleOrder)
		return out
	}

	seen := make(map[string]bool, len(m.boot.Config.Agents))
	var ordered []string
	for _, r := range defaultRoleOrder {
		if _, ok := m.boot.Config.Agents[r]; ok {
			seen[r] = true
			ordered = append(ordered, r)
		}
	}

	var custom []string
	for id := range m.boot.Config.Agents {
		if !seen[id] {
			custom = append(custom, id)
		}
	}
	sort.Strings(custom)
	ordered = append(ordered, custom...)
	return ordered
}

func (m *Model) isKnownWorkflowOrTopology(token string) bool {
	cand := strings.TrimSpace(token)
	if cand == "" {
		return false
	}
	if m.boot.Config != nil && len(m.boot.Config.Workflows) > 0 {
		for k := range m.boot.Config.Workflows {
			if strings.EqualFold(k, cand) {
				return true
			}
		}
	}
	_, err := fleet.ParseTopologyKind(cand)
	return err == nil
}

func (m *Model) resolveCanonicalWorkflowID(token string) string {
	cand := strings.TrimSpace(token)
	if m.boot.Config != nil {
		for k := range m.boot.Config.Workflows {
			if strings.EqualFold(k, cand) {
				return k
			}
		}
	}
	return strings.ToLower(cand)
}

func formatMissionSnapshotSummary(ms *fleet.MissionSnapshot) string {
	if ms == nil {
		return "Fleet: idle"
	}
	doneNodes := 0
	for _, n := range ms.Nodes {
		if n.Status == fleet.NodeCompleted {
			doneNodes++
		}
	}
	wf := ms.WorkflowID
	if wf == "" {
		wf = string(ms.Topology)
	}
	return fmt.Sprintf("Mission %s [%s · %s] %d/%d nodes · $%.3f",
		ms.ID, wf, ms.Status, doneNodes, len(ms.Nodes), ms.TotalCostUSD)
}

func (m *Model) formatFleetStatusBar() string {
	runningMissions := 0
	var active *fleet.MissionSnapshot
	for _, ms := range m.boot.Missions {
		if ms == nil {
			continue
		}
		if ms.Status == fleet.MissionRunning && (m.activeWsID == "" || ms.WorkspaceID == m.activeWsID) {
			runningMissions++
			if active == nil {
				active = ms
			}
		}
	}
	if active != nil {
		doneNodes := 0
		for _, n := range active.Nodes {
			if n.Status == fleet.NodeCompleted {
				doneNodes++
			}
		}
		wf := active.WorkflowID
		if wf == "" {
			wf = string(active.Topology)
		}
		return fmt.Sprintf("Fleet: %d live (%s %d/%d nodes · $%.2f)",
			runningMissions, wf, doneNodes, len(active.Nodes), active.TotalCostUSD)
	}
	return fmt.Sprintf("Fleet: %d live", runningMissions)
}

func (m *Model) visibleSessions() []*store.SessionMeta {
	var out []*store.SessionMeta
	for _, s := range m.boot.Sessions {
		if s == nil || s.Archived {
			continue
		}
		if m.activeWsID != "" && s.WorkspaceID != m.activeWsID {
			continue
		}
		out = append(out, s)
	}
	return out
}

func (m *Model) isVisibleSession(id string, visible []*store.SessionMeta) bool {
	for _, s := range visible {
		if s.ID == id {
			return true
		}
	}
	return false
}

func (m *Model) activeSessionMeta() *store.SessionMeta {
	if m.activeSessionID == "" {
		return nil
	}
	for _, s := range m.boot.Sessions {
		if s != nil && s.ID == m.activeSessionID {
			return s
		}
	}
	return nil
}

func (m *Model) upsertSessionMeta(meta *store.SessionMeta) {
	if meta == nil || meta.ID == "" {
		return
	}
	for i, s := range m.boot.Sessions {
		if s != nil && s.ID == meta.ID {
			m.boot.Sessions[i] = meta
			return
		}
	}
	m.boot.Sessions = append(m.boot.Sessions, meta)
}

func (m *Model) maxKnownSessionSeq() uint64 {
	var maxSeq uint64
	for _, e := range m.entries {
		if e.Seq > maxSeq {
			maxSeq = e.Seq
		}
	}
	if meta := m.activeSessionMeta(); meta != nil && meta.HeadSeq > maxSeq {
		maxSeq = meta.HeadSeq
	}
	return maxSeq
}

func (m *Model) syncWorkspaceCursor() {
	for i, ws := range m.boot.Workspaces {
		if ws != nil && ws.ID == m.activeWsID {
			m.wsCursor = i
			return
		}
	}
	if m.wsCursor >= len(m.boot.Workspaces) {
		m.wsCursor = 0
	}
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
	if m.activeSessionID != "" {
		for i := range m.boot.PendingApprovals {
			if m.boot.PendingApprovals[i].SessionID == m.activeSessionID {
				return &m.boot.PendingApprovals[i]
			}
		}
		return nil
	}
	for i := range m.boot.PendingApprovals {
		if m.activeWsID == "" || m.boot.PendingApprovals[i].WorkspaceID == m.activeWsID {
			return &m.boot.PendingApprovals[i]
		}
	}
	return nil
}

func (m *Model) ensureApprovalTracked(appr core.PendingApproval) bool {
	newlyTracked := false
	if _, ok := m.surfacedLocal[appr.ID]; !ok {
		m.surfacedLocal[appr.ID] = time.Now()
		newlyTracked = true
	}
	if _, ok := m.selectedHunks[appr.ID]; !ok {
		set := make(map[string]bool, len(appr.Hunks))
		for _, h := range appr.Hunks {
			set[h.ID] = true
		}
		m.selectedHunks[appr.ID] = set
	}
	return newlyTracked
}

func (m *Model) removeApprovalLocal(approvalID string) {
	var kept []core.PendingApproval
	for _, a := range m.boot.PendingApprovals {
		if a.ID != approvalID {
			kept = append(kept, a)
		}
	}
	m.boot.PendingApprovals = kept
	delete(m.selectedHunks, approvalID)
	delete(m.rememberApproval, approvalID)
	delete(m.approvalFeedback, approvalID)
	m.editingApprovalFeedback = false
	m.hunkCursor = 0
	m.approvalScroll = 0
	if m.currentApproval() == nil && m.focus == focusApproval {
		m.focus = focusComposer
	}
}

func scheduleApprovalSettleTick(approvalID string) tea.Cmd {
	return tea.Tick(core.SettleWindow+15*time.Millisecond, func(time.Time) tea.Msg {
		return approvalSettledTickMsg{approvalID: approvalID}
	})
}

func compactJSONArgs(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, trimmed); err == nil {
		return buf.String()
	}
	return string(trimmed)
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
