package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/store"
	"gorness.dev/gorness/internal/tools"
)

const (
	MaxSubagentSteps       = 8
	MaxSubagentReportChars = 3200
)

type SubagentStartedPayload struct {
	SubagentID string   `json:"subagent_id"`
	AgentRole  string   `json:"agent_role"`
	ModelID    string   `json:"model_id"`
	Prompt     string   `json:"prompt"`
	Paths      []string `json:"paths,omitempty"`
	ReadOnly   bool     `json:"read_only"`
	MaxSteps   int      `json:"max_steps"`
}

type SubagentCompletedPayload struct {
	SubagentID   string  `json:"subagent_id"`
	AgentRole    string  `json:"agent_role"`
	ModelID      string  `json:"model_id"`
	Status       string  `json:"status"`
	Steps        int     `json:"steps"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	Summary      string  `json:"summary,omitempty"`
	Error        string  `json:"error,omitempty"`
}

type spawnSubagentInput struct {
	AgentRole string   `json:"agent_role,omitempty"`
	Role      string   `json:"role,omitempty"`
	Prompt    string   `json:"prompt,omitempty"`
	Task      string   `json:"task,omitempty"`
	Paths     []string `json:"paths,omitempty"`
	MaxSteps  int      `json:"max_steps,omitempty"`
	ReadOnly  *bool    `json:"read_only,omitempty"`
}

type subagentParentContext struct {
	parentRunID          string
	ws                   *store.Workspace
	parentSessionID      string
	effectiveRoot        string
	wsSnap               *config.Snapshot
	configTampered       bool
	parentReadOnly       bool
	parentPermMode       string
	parentSandbox        tools.SandboxMode
	parentUnattended     bool
	parentAllowedTools   []string
	parentExtraWriteDirs []string
	pathGuard            *tools.PathGuard
	skills               *tools.SkillCatalog
	secrets              []string
	emit                 func(eventType string, payload any)
}

func (e *Engine) makeSpawnSubagentTool(parent subagentParentContext) tools.Tool {
	return tools.Tool{
		Name:            "spawn_subagent",
		Description:     "Spawn a bounded specialist subagent (explorer, verifier, or debugger; max 8 steps, read-only by default) and return a distilled report (<800 tokens).",
		InputSchema:     json.RawMessage(`{"type":"object","properties":{"agent_role":{"type":"string","enum":["explorer","verifier","debugger"],"description":"Specialist subagent role (explorer, verifier, or debugger)"},"role":{"type":"string","enum":["explorer","verifier","debugger"],"description":"Alias for agent_role"},"prompt":{"type":"string","description":"Focused task instructions for the subagent"},"paths":{"type":"array","items":{"type":"string"},"description":"Optional workspace-relative paths to focus on"},"max_steps":{"type":"integer","description":"Maximum reasoning steps (1-8, default 8)"},"read_only":{"type":"boolean","description":"Run subagent in strict read-only mode (default true)"}},"required":["prompt"]}`),
		ReadOnly:        true,
		Mutating:        false,
		ConcurrencySafe: true,
		Handler: func(ctx context.Context, call tools.CallContext, rawInput json.RawMessage) (tools.ToolResult, error) {
			return e.executeSpawnSubagent(ctx, parent, call, rawInput)
		},
	}
}

func (e *Engine) executeSpawnSubagent(
	ctx context.Context,
	parent subagentParentContext,
	_ tools.CallContext,
	rawInput json.RawMessage,
) (tools.ToolResult, error) {
	var in spawnSubagentInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return tools.ToolResult{}, fmt.Errorf("invalid spawn_subagent input: %w", err)
	}

	subRole := strings.ToLower(strings.TrimSpace(in.AgentRole))
	if subRole == "" {
		subRole = strings.ToLower(strings.TrimSpace(in.Role))
	}
	if subRole == "" {
		subRole = "explorer"
	}
	switch subRole {
	case "explorer", "verifier", "debugger":
	default:
		return tools.ToolResult{}, fmt.Errorf("unsupported subagent role %q: must be explorer, verifier, or debugger", subRole)
	}

	taskPrompt := strings.TrimSpace(in.Prompt)
	if taskPrompt == "" {
		taskPrompt = strings.TrimSpace(in.Task)
	}
	if taskPrompt == "" {
		return tools.ToolResult{}, errors.New("spawn_subagent requires a non-empty prompt")
	}

	// Validate any explicit focus paths against parent's PathGuard so subagents can never escape workspace/vault bounds.
	cleanPaths := make([]string, 0, len(in.Paths))
	for _, rawP := range in.Paths {
		trimmed := strings.TrimSpace(rawP)
		if trimmed == "" {
			continue
		}
		if parent.pathGuard != nil {
			_, rel, inWS, pErr := parent.pathGuard.ResolveInWorkspace(trimmed)
			if pErr != nil {
				return tools.ToolResult{}, fmt.Errorf("subagent path %q blocked: %w", trimmed, pErr)
			}
			if !inWS {
				return tools.ToolResult{}, fmt.Errorf("%w: subagent path %q", tools.ErrPathEscapesWorkspace, trimmed)
			}
			cleanPaths = append(cleanPaths, rel)
		} else {
			cleanPaths = append(cleanPaths, trimmed)
		}
	}

	// Read-only by default; explorer is unconditionally read-only, and parent read-only/plan always forces read-only.
	readOnly := true
	if in.ReadOnly != nil && !*in.ReadOnly && subRole != "explorer" && !parent.parentReadOnly {
		if NormalizePermMode(parent.parentPermMode) != PermModePlan && parent.parentSandbox != tools.SandboxReadOnly {
			readOnly = false
		}
	}

	maxSubSteps := MaxSubagentSteps
	if parent.wsSnap != nil && parent.wsSnap.Global.SubagentMaxSteps >= 2 {
		maxSubSteps = parent.wsSnap.Global.SubagentMaxSteps
		if maxSubSteps > 32 {
			maxSubSteps = 32
		}
	}
	stepsBudget := in.MaxSteps
	if stepsBudget <= 0 || stepsBudget > maxSubSteps {
		stepsBudget = maxSubSteps
	}

	desiredPerm := PermModePlan
	desiredSandbox := tools.SandboxReadOnly
	defaultChildTools := []string{"read_file", "search_code", "activate_skill", "search_tools"}
	var childExtraWriteDirs []string
	if !readOnly {
		desiredPerm = PermModeAuto
		desiredSandbox = tools.SandboxStrict
		defaultChildTools = []string{"read_file", "search_code", "run_command", "activate_skill", "search_tools"}
		childExtraWriteDirs = append([]string(nil), parent.parentExtraWriteDirs...)
	}

	childPermMode := intersectPermModes(parent.parentPermMode, desiredPerm)
	childSandbox := intersectSandboxModes(parent.parentSandbox, desiredSandbox)
	if childPermMode == PermModePlan {
		readOnly = true
		childSandbox = tools.SandboxReadOnly
		defaultChildTools = []string{"read_file", "search_code", "activate_skill", "search_tools"}
		childExtraWriteDirs = nil
	}

	childAllowedTools := intersectSubagentTools(parent.parentAllowedTools, defaultChildTools)

	route, err := e.router.Resolve(subRole, parent.ws.RoleModels[subRole], "", parent.wsSnap)
	if err != nil {
		return tools.ToolResult{}, fmt.Errorf("resolve subagent route (%s): %w", subRole, err)
	}

	var childProfile config.AgentProfile
	if prof, ok := parent.wsSnap.Agents[subRole]; ok {
		childProfile = prof
		childProfile.ReadOnly = readOnly
	} else {
		childProfile = config.AgentProfile{
			ID:           subRole,
			ReadOnly:     readOnly,
			SystemPrompt: defaultSubagentSystemPrompt(subRole),
		}
	}
	// Never allow recursive subagent spawning or artifact publishing inside a subagent.
	childProfile.DisallowedTools = appendUniqueStrings(childProfile.DisallowedTools, "spawn_subagent", "publish_artifact")
	if readOnly {
		childProfile.DisallowedTools = appendUniqueStrings(childProfile.DisallowedTools, "edit_file", "write_file", "run_command")
	}

	tmpVaultDir, err := os.MkdirTemp("", "gorness-subagent-*")
	if err != nil {
		return tools.ToolResult{}, fmt.Errorf("create ephemeral subagent store: %w", err)
	}
	defer os.RemoveAll(tmpVaultDir)

	subSessions, err := store.NewSessionStore(config.VaultPaths{Root: tmpVaultDir})
	if err != nil {
		return tools.ToolResult{}, fmt.Errorf("init subagent session store: %w", err)
	}
	subMeta, err := subSessions.Create(parent.ws.ID, "Subagent "+subRole, subRole)
	if err != nil {
		return tools.ToolResult{}, err
	}

	childReg, err := tools.NewRegistry(tools.RegistryOptions{
		WorkspaceID:              parent.ws.ID,
		WorkspaceRoot:            parent.effectiveRoot,
		VaultPaths:               parent.wsSnap.Paths,
		Snapshots:                e.snapshots,
		PathGuard:                parent.pathGuard,
		Seatbelt:                 e.seatbelt,
		Skills:                   parent.skills,
		ReadHashes:               e.readHashes,
		DefaultCommandTimeoutSec: parent.wsSnap.Global.CommandTimeoutSeconds,
	})
	if err != nil {
		return tools.ToolResult{}, err
	}
	childReg.Unregister("spawn_subagent")
	childReg.Unregister("publish_artifact")
	if readOnly {
		childReg.Unregister("edit_file")
		childReg.Unregister("write_file")
		childReg.Unregister("run_command")
	}

	subID := fmt.Sprintf("sub_%d_%s", e.runNonce.Add(1), store.RandomHex(4))
	if parent.emit != nil {
		parent.emit(EventSubagentStarted, SubagentStartedPayload{
			SubagentID: subID,
			AgentRole:  subRole,
			ModelID:    route.ModelID,
			Prompt:     config.EscapeBidi(config.Redact(taskPrompt, parent.secrets)),
			Paths:      cleanPaths,
			ReadOnly:   readOnly,
			MaxSteps:   stepsBudget,
		})
	}

	var dynamicCtx []string
	dynamicCtx = append(dynamicCtx, "Return a crisp, distilled report (<800 tokens) citing exact file paths and line numbers.")
	if len(cleanPaths) > 0 {
		dynamicCtx = append(dynamicCtx, "Focus investigation on paths: "+strings.Join(cleanPaths, ", "))
	}

	childLoop := newAgentLoop(agentLoopConfig{
		runID:              subID,
		ws:                 parent.ws,
		sessionID:          subMeta.ID,
		effectiveRoot:      parent.effectiveRoot,
		userHomeDir:        e.userHome,
		agentRole:          subRole,
		prompt:             taskPrompt,
		route:              route,
		snap:               parent.wsSnap,
		profile:            &childProfile,
		permMode:           childPermMode,
		sandboxMode:        childSandbox,
		unattended:         true, // Subagents never block interactively on UI approval cards
		maxSteps:           stepsBudget,
		maxToolConcurrency: parent.wsSnap.Global.MaxToolConcurrency,
		allowedTools:       childAllowedTools,
		extraWriteDirs:     childExtraWriteDirs,
		initialDynamic:     dynamicCtx,
		configTampered:     parent.configTampered,
		reg:                childReg,
		pathGuard:          parent.pathGuard,
		skills:             parent.skills,
		sessions:           subSessions,
		snapshots:          e.snapshots,
		router:             e.router,
		hooks:              e.hooks,
		gate:               e.gate,
		broker:             e.broker,
		grants:             e.grants,
		secrets:            parent.secrets,
		now:                e.now,
	})

	outcome, runErr := childLoop.Run(ctx)
	distilled := distillSubagentReport(outcome.FinalText, parent.secrets)

	var errText string
	if runErr != nil {
		errText = config.EscapeBidi(config.Redact(runErr.Error(), parent.secrets))
	}

	if parent.emit != nil {
		parent.emit(EventSubagentCompleted, SubagentCompletedPayload{
			SubagentID:   subID,
			AgentRole:    subRole,
			ModelID:      route.ModelID,
			Status:       outcome.Status,
			Steps:        outcome.Steps,
			InputTokens:  outcome.InputTokens,
			OutputTokens: outcome.OutputTokens,
			CostUSD:      outcome.CostUSD,
			Summary:      distilled,
			Error:        errText,
		})
	}

	if runErr != nil && ctx.Err() != nil {
		return tools.ToolResult{}, ctx.Err()
	}

	if runErr != nil && distilled == "" {
		return tools.ToolResult{
			Output:  fmt.Sprintf("[subagent:%s status=%s steps=%d] error: %s", subRole, outcome.Status, outcome.Steps, errText),
			IsError: true,
			Metadata: map[string]any{
				"subagent_id": subID,
				"agent_role":  subRole,
				"steps":       outcome.Steps,
				"status":      outcome.Status,
			},
		}, nil
	}

	header := fmt.Sprintf("[subagent:%s id=%s steps=%d]", subRole, subID, outcome.Steps)
	body := header
	if distilled != "" {
		body = header + "\n" + distilled
	}
	if errText != "" {
		body += "\nNote: " + errText
	}

	return tools.ToolResult{
		Output: body,
		Metadata: map[string]any{
			"subagent_id":   subID,
			"agent_role":    subRole,
			"steps":         outcome.Steps,
			"status":        outcome.Status,
			"input_tokens":  outcome.InputTokens,
			"output_tokens": outcome.OutputTokens,
			"cost_usd":      outcome.CostUSD,
		},
	}, nil
}

func distillSubagentReport(raw string, secrets []string) string {
	cleaned := strings.TrimSpace(config.EscapeBidi(config.Redact(raw, secrets)))
	if len(cleaned) <= MaxSubagentReportChars {
		return cleaned
	}
	cut := MaxSubagentReportChars
	for cut > 0 && !utf8.RuneStart(cleaned[cut]) {
		cut--
	}
	return strings.TrimSpace(cleaned[:cut]) + "\n...[distilled to <800 tokens]"
}

func permModeRank(mode string) int {
	switch NormalizePermMode(mode) {
	case PermModePlan:
		return 0
	case PermModeAsk:
		return 1
	case PermModeAcceptEdits:
		return 2
	case PermModeAuto:
		return 3
	case PermModeYolo:
		return 4
	default:
		return 3
	}
}

func intersectPermModes(parentMode, childMode string) string {
	p := NormalizePermMode(parentMode)
	c := NormalizePermMode(childMode)
	if permModeRank(p) < permModeRank(c) {
		return p
	}
	return c
}

func sandboxModeRank(mode tools.SandboxMode) int {
	switch tools.NormalizeSandboxMode(string(mode)) {
	case tools.SandboxReadOnly:
		return 0
	case tools.SandboxStrict:
		return 1
	case tools.SandboxNetworkAllow:
		return 2
	case tools.SandboxOff:
		return 3
	default:
		return 1
	}
}

func intersectSandboxModes(parentMode, childMode tools.SandboxMode) tools.SandboxMode {
	p := tools.NormalizeSandboxMode(string(parentMode))
	c := tools.NormalizeSandboxMode(string(childMode))
	if sandboxModeRank(p) < sandboxModeRank(c) {
		return p
	}
	return c
}

func intersectSubagentTools(parentAllowed []string, childDefaults []string) []string {
	var out []string
	for _, name := range childDefaults {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" || trimmed == "spawn_subagent" || trimmed == "publish_artifact" {
			continue
		}
		if parentAllowed != nil && !containsString(parentAllowed, trimmed) && trimmed != "search_tools" {
			continue
		}
		out = append(out, trimmed)
	}
	if out == nil {
		out = []string{}
	}
	return out
}

func appendUniqueStrings(base []string, extras ...string) []string {
	out := append([]string(nil), base...)
	for _, ex := range extras {
		if !containsString(out, ex) {
			out = append(out, ex)
		}
	}
	return out
}

func defaultSubagentSystemPrompt(role string) string {
	switch role {
	case "verifier":
		return "You are the Verifier subagent. Inspect code and evidence thoroughly, verify edge cases, and report concrete pass/fail findings with file:line citations."
	case "debugger":
		return "You are the Debugger subagent. Trace failure root causes across call sites, state transitions, and invariants, returning exact file:line diagnoses."
	default:
		return "You are the Explorer subagent. Rapidly search and read repository files to map architecture, symbols, and call sites with concise file:line citations."
	}
}
