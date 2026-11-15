package fleet

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gorness.dev/gorness/internal/store"
)

type TopologyKind string

const (
	TopoHybridFleet     TopologyKind = "hybrid-fleet"
	TopoReviewLoop      TopologyKind = "review-loop"
	TopoParallelExplore TopologyKind = "parallel-explore"
	TopoWorktreeTeam    TopologyKind = "worktree-team"
	TopoEventPipeline   TopologyKind = "event-pipeline"
	TopoSharedBoard     TopologyKind = "shared-board"
)

func (t TopologyKind) IsValid() bool {
	switch t {
	case TopoHybridFleet,
		TopoReviewLoop,
		TopoParallelExplore,
		TopoWorktreeTeam,
		TopoEventPipeline,
		TopoSharedBoard:
		return true
	default:
		return false
	}
}

func ParseTopologyKind(raw string) (TopologyKind, error) {
	k := TopologyKind(strings.ToLower(strings.TrimSpace(raw)))
	if !k.IsValid() {
		return "", fmt.Errorf("unsupported fleet topology %q", raw)
	}
	return k, nil
}

const (
	MissionPending   = "pending"
	MissionRunning   = "running"
	MissionCompleted = "completed"
	MissionFailed    = "failed"
	MissionCancelled = "cancelled"

	NodeIdle      = "idle"
	NodeRunning   = "running"
	NodeMerging   = "merging"
	NodeCompleted = "completed"
	NodeFailed    = "failed"
	NodeCancelled = "cancelled"
	NodeStalled   = "stalled"

	TaskOpen       = "open"
	TaskClaimed    = "claimed"
	TaskInProgress = "in_progress"
	TaskSplit      = "split"
	TaskReview     = "review"
	TaskMerged     = "merged"
	TaskCompleted  = "completed"
	TaskFailed     = "failed"

	MaxTaskSplitDepth = 3
	DefaultMaxRounds  = 3
	DefaultMaxIdle    = 3
	DefaultMaxHops    = 8
)

const (
	EventMissionStarted   = "mission_started"
	EventNodeUpdated      = "node_updated"
	EventNodeDelta        = "node_delta"
	EventNodeSteered      = "node_steered"
	EventNodeStalled      = "node_stalled"
	EventEdgePulse        = "edge_pulse"
	EventTaskUpdated      = "task_updated"
	EventNoteAdded        = "note_added"
	EventMergeQueued      = "merge_queued"
	EventMergeVerified    = "merge_verified"
	EventMergeLanded      = "merge_landed"
	EventMergeRejected    = "merge_rejected"
	EventSnapshotSync     = "snapshot_sync"
	EventMissionCompleted = "mission_completed"
	EventMissionFailed    = "mission_failed"
	EventMissionCancelled = "mission_cancelled"
)

var (
	ErrInvalidID       = store.ErrInvalidID
	ErrMissionNotFound = errors.New("mission not found")
	ErrNodeNotFound    = errors.New("fleet node not found")
	ErrTaskNotFound    = errors.New("blackboard task not found")
	ErrTaskAlreadyHeld = errors.New("blackboard task already claimed")
	ErrMaxDepthReached = errors.New("blackboard task split exceeds maximum depth")
	ErrMaxHopsExceeded = errors.New("event pipeline exceeded maximum hop limit")
	ErrBudgetExceeded  = errors.New("mission exceeded configured USD budget")
	ErrManagerClosed   = errors.New("fleet manager is closed")
)

type FleetNode struct {
	ID             string    `json:"id" toml:"id"`
	Label          string    `json:"label" toml:"label"`
	RoleID         string    `json:"role_id" toml:"role_id"`
	ProviderID     string    `json:"provider_id" toml:"provider_id"`
	ModelID        string    `json:"model_id" toml:"model_id"`
	Effort         string    `json:"effort" toml:"effort"`
	Status         string    `json:"status" toml:"status"`
	WorktreeBranch string    `json:"worktree_branch,omitempty" toml:"worktree_branch,omitempty"`
	WorktreePath   string    `json:"worktree_path,omitempty" toml:"worktree_path,omitempty"`
	SessionID      string    `json:"session_id,omitempty" toml:"session_id,omitempty"`
	ActiveRunID    string    `json:"active_run_id,omitempty" toml:"active_run_id,omitempty"`
	Tokens         int       `json:"tokens" toml:"tokens"`
	CostUSD        float64   `json:"cost_usd" toml:"cost_usd"`
	DiffStat       string    `json:"diff_stat,omitempty" toml:"diff_stat,omitempty"`
	UpdatedAt      time.Time `json:"updated_at" toml:"updated_at"`
}

type FleetEdge struct {
	From     string `json:"from" toml:"from"`
	To       string `json:"to" toml:"to"`
	Label    string `json:"label,omitempty" toml:"label,omitempty"`
	Kind     string `json:"kind,omitempty" toml:"kind,omitempty"`
	Active   bool   `json:"active" toml:"active"`
	PulseSeq uint64 `json:"pulse_seq" toml:"pulse_seq"`
}

type BlackboardTask struct {
	ID             string `json:"id" toml:"id"`
	ParentID       string `json:"parent_id,omitempty" toml:"parent_id,omitempty"`
	Title          string `json:"title" toml:"title"`
	OwnerNodeID    string `json:"owner_node_id,omitempty" toml:"owner_node_id,omitempty"`
	Status         string `json:"status" toml:"status"`
	WorktreeBranch string `json:"worktree_branch,omitempty" toml:"worktree_branch,omitempty"`
	LandedCommit   string `json:"landed_commit,omitempty" toml:"landed_commit,omitempty"`
	Depth          int    `json:"depth" toml:"depth"`
	Version        int    `json:"version" toml:"version"`
}

type BlackboardNote struct {
	ID        string    `json:"id" toml:"id"`
	Seq       uint64    `json:"seq" toml:"seq"`
	NodeID    string    `json:"node_id" toml:"node_id"`
	Title     string    `json:"title,omitempty" toml:"title,omitempty"`
	Body      string    `json:"body" toml:"body"`
	CreatedAt time.Time `json:"created_at" toml:"created_at"`
}

type MissionSnapshot struct {
	ID           string           `json:"id" toml:"id"`
	WorkspaceID  string           `json:"workspace_id" toml:"workspace_id"`
	WorkflowID   string           `json:"workflow_id" toml:"workflow_id"`
	Goal         string           `json:"goal" toml:"goal"`
	Topology     TopologyKind     `json:"topology" toml:"topology"`
	Status       string           `json:"status" toml:"status"`
	Nodes        []FleetNode      `json:"nodes" toml:"nodes"`
	Edges        []FleetEdge      `json:"edges" toml:"edges"`
	Tasks        []BlackboardTask `json:"tasks" toml:"tasks"`
	Notes        []BlackboardNote `json:"notes" toml:"notes"`
	MergeQueue   []string         `json:"merge_queue" toml:"merge_queue"`
	TotalCostUSD float64          `json:"total_cost_usd" toml:"total_cost_usd"`
	BudgetUSD    float64          `json:"budget_usd,omitempty" toml:"budget_usd,omitempty"`
	Summary      string           `json:"summary,omitempty" toml:"summary,omitempty"`
	Error        string           `json:"error,omitempty" toml:"error,omitempty"`
	CreatedAt    time.Time        `json:"created_at" toml:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at" toml:"updated_at"`
}

func (s *MissionSnapshot) Clone() *MissionSnapshot {
	if s == nil {
		return nil
	}
	cp := *s
	if s.Nodes != nil {
		cp.Nodes = make([]FleetNode, len(s.Nodes))
		copy(cp.Nodes, s.Nodes)
	}
	if s.Edges != nil {
		cp.Edges = make([]FleetEdge, len(s.Edges))
		copy(cp.Edges, s.Edges)
	}
	if s.Tasks != nil {
		cp.Tasks = make([]BlackboardTask, len(s.Tasks))
		copy(cp.Tasks, s.Tasks)
	}
	if s.Notes != nil {
		cp.Notes = make([]BlackboardNote, len(s.Notes))
		copy(cp.Notes, s.Notes)
	}
	if s.MergeQueue != nil {
		cp.MergeQueue = make([]string, len(s.MergeQueue))
		copy(cp.MergeQueue, s.MergeQueue)
	}
	return &cp
}

type MissionRequest struct {
	MissionID      string            `json:"mission_id,omitempty"`
	WorkspaceID    string            `json:"workspace_id"`
	WorkflowID     string            `json:"workflow_id,omitempty"`
	Topology       TopologyKind      `json:"topology,omitempty"`
	Goal           string            `json:"goal"`
	TargetBranch   string            `json:"target_branch,omitempty"`
	MaxConcurrency int               `json:"max_concurrency,omitempty"`
	MaxRounds      int               `json:"max_rounds,omitempty"`
	MaxIdleRounds  int               `json:"max_idle_rounds,omitempty"`
	MaxHops        int               `json:"max_hops,omitempty"`
	BudgetUSD      float64           `json:"budget_usd,omitempty"`
	VerifyCommand  string            `json:"verify_command,omitempty"`
	UseWorktrees   *bool             `json:"use_worktrees,omitempty"`
	RoleModels     map[string]string `json:"role_models,omitempty"`
	PermMode       string            `json:"perm_mode,omitempty"`
	SandboxMode    string            `json:"sandbox_mode,omitempty"`
}

type MissionEvent struct {
	Seq         uint64           `json:"seq"`
	Type        string           `json:"type"`
	MissionID   string           `json:"mission_id"`
	WorkspaceID string           `json:"workspace_id"`
	NodeID      string           `json:"node_id,omitempty"`
	EdgeFrom    string           `json:"edge_from,omitempty"`
	EdgeTo      string           `json:"edge_to,omitempty"`
	TaskID      string           `json:"task_id,omitempty"`
	Message     string           `json:"message,omitempty"`
	Payload     any              `json:"payload,omitempty"`
	Snapshot    *MissionSnapshot `json:"snapshot,omitempty"`
	Timestamp   time.Time        `json:"timestamp"`
}

// ValidateID 严格拦截路径分隔符、控制字符及 "." / ".." 穿越段，保护 missions/ 与 runtime/worktrees/ 边界。
func ValidateID(id string) error {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" || trimmed != id || id == "." || id == ".." || strings.Contains(id, "..") {
		return fmt.Errorf("%w: %q", ErrInvalidID, id)
	}
	for _, r := range id {
		if r == '/' || r == '\\' || r < 0x20 || r == 0x7f {
			return fmt.Errorf("%w: %q", ErrInvalidID, id)
		}
	}
	return nil
}

func ensureStrictSubpath(baseDir, targetPath string) error {
	cleanBase := filepath.Clean(baseDir)
	cleanTarget := filepath.Clean(targetPath)
	rel, err := filepath.Rel(cleanBase, cleanTarget)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidID, err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: path %q escapes %q", ErrInvalidID, targetPath, baseDir)
	}
	return nil
}

func isTerminalMissionStatus(status string) bool {
	switch status {
	case MissionCompleted, MissionFailed, MissionCancelled:
		return true
	default:
		return false
	}
}
