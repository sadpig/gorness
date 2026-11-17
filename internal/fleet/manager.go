package fleet

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/core"
	"gorness.dev/gorness/internal/store"
	"gorness.dev/gorness/internal/tools"
)

type ManagerOptions struct {
	Config        *config.Manager
	Engine        *core.Engine
	Seatbelt      *tools.SeatbeltRunner
	VerifyRunner  VerifyRunner
	StallTimeout  time.Duration
	CheckInterval time.Duration
	Now           func() time.Time
}

// Manager 统筹 6 大异构多 Agent 编排拓扑、Git Worktree 池、Markdown 共享黑板、Witness 巡检与 Refinery 串行合入门控。
type Manager struct {
	cfg           *config.Manager
	engine        *core.Engine
	pool          *WorktreePool
	refinery      *Refinery
	stallTimeout  time.Duration
	checkInterval time.Duration
	now           func() time.Time

	missionNonce atomic.Uint64

	mu       sync.RWMutex
	active   map[string]*missionRuntime // key: wsID + "\x00" + missionID
	finished map[string]*MissionSnapshot
	closed   bool

	bgWG sync.WaitGroup
}

func NewManager(opts ManagerOptions) (*Manager, error) {
	if opts.Engine == nil {
		return nil, errors.New("fleet.NewManager: Engine is required")
	}
	cfgMgr := opts.Config
	if cfgMgr == nil {
		cfgMgr = opts.Engine.Config()
	}
	if cfgMgr == nil {
		return nil, errors.New("fleet.NewManager: Config manager is required")
	}

	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	paths := cfgMgr.Paths()
	sb := opts.Seatbelt
	if sb == nil {
		sb = tools.NewSeatbeltRunner(tools.WithRunnerVaultRoot(paths.Root))
	}

	pool := NewWorktreePool(paths)
	refinery := NewRefinery(pool, sb, opts.VerifyRunner)

	mgr := &Manager{
		cfg:           cfgMgr,
		engine:        opts.Engine,
		pool:          pool,
		refinery:      refinery,
		stallTimeout:  opts.StallTimeout,
		checkInterval: opts.CheckInterval,
		now:           nowFn,
		active:        make(map[string]*missionRuntime),
		finished:      make(map[string]*MissionSnapshot),
	}
	mgr.recoverInterruptedMissions()
	return mgr, nil
}

// Pool 暴露底层 Git Worktree 隔离池（供会话级 Worktree 创建与回收复用）。
func (m *Manager) Pool() *WorktreePool {
	if m == nil {
		return nil
	}
	return m.pool
}

// Refinery 暴露串行变基验收与快进合入门控器。
func (m *Manager) Refinery() *Refinery {
	if m == nil {
		return nil
	}
	return m.refinery
}

func (m *Manager) publishFleetUpdated(wsID, missionID, evType, nodeID string) {
	if m == nil || m.engine == nil {
		return
	}
	m.engine.PublishSystemEvent(core.EventFleetUpdated, map[string]any{
		"workspace_id": wsID,
		"mission_id":   missionID,
		"event":        evType,
		"node_id":      nodeID,
		"timestamp":    m.now().UTC(),
	})
}

// recoverInterruptedMissions 在守护进程启动时扫描所有工作区黑板，将前次崩溃遗留的 running/pending 任务标记为 interrupted 并释放孤儿抢占任务卡。
func (m *Manager) recoverInterruptedMissions() {
	if m == nil || m.cfg == nil {
		return
	}
	wsBase := m.cfg.Paths().WorkspacesDir()
	wsIDs := make(map[string]bool)
	if m.engine != nil && m.engine.Workspaces() != nil {
		for _, ws := range m.engine.Workspaces().List() {
			if ValidateID(ws.ID) == nil {
				wsIDs[ws.ID] = true
			}
		}
	}
	if entries, err := os.ReadDir(wsBase); err == nil {
		for _, ent := range entries {
			if ent.IsDir() && ValidateID(ent.Name()) == nil {
				wsIDs[ent.Name()] = true
			}
		}
	}

	now := m.now().UTC()
	for wsID := range wsIDs {
		missionsDir := filepath.Join(wsBase, wsID, "missions")
		if ensureStrictSubpath(wsBase, missionsDir) != nil {
			continue
		}
		mEntries, err := os.ReadDir(missionsDir)
		if err != nil {
			continue
		}
		for _, me := range mEntries {
			if !me.IsDir() || ValidateID(me.Name()) != nil {
				continue
			}
			mid := me.Name()
			mDir := filepath.Join(missionsDir, mid)
			_, stSnap := os.Stat(filepath.Join(mDir, "snapshot.toml"))
			_, stMD := os.Stat(filepath.Join(mDir, "MISSION.md"))
			if stSnap != nil && stMD != nil {
				continue
			}
			bb, bErr := newMarkdownBlackboardWithClock(m.cfg.Paths(), wsID, mid, m.now)
			if bErr != nil {
				continue
			}
			snap, lErr := bb.LoadSnapshot()
			if lErr != nil || snap == nil {
				continue
			}
			if snap.Status != MissionRunning && snap.Status != MissionPending {
				continue
			}

			_, _ = bb.ReleaseAllClaimedTasks()
			tasks, _ := bb.ListTasks()
			notes, _ := bb.ListNotes()

			snap.Status = MissionInterrupted
			if strings.TrimSpace(snap.Error) == "" {
				snap.Error = "mission interrupted by prior daemon shutdown"
			}
			for i := range snap.Nodes {
				snap.Nodes[i].ActiveRunID = ""
				switch snap.Nodes[i].Status {
				case NodeRunning, NodeMerging, NodeStalled:
					snap.Nodes[i].Status = NodeCancelled
					snap.Nodes[i].UpdatedAt = now
				}
			}
			for i := range snap.Edges {
				snap.Edges[i].Active = false
			}
			snap.MergeQueue = []string{}
			if tasks != nil {
				snap.Tasks = tasks
			}
			if notes != nil {
				snap.Notes = notes
			}
			snap.UpdatedAt = now
			_ = bb.SaveSnapshot(snap)
			_ = bb.AppendAuditEvent("mission_interrupted", "", "", snap.Error)
		}
	}
}

func missionMapKey(wsID, missionID string) string {
	return wsID + "\x00" + missionID
}

func (m *Manager) StartMission(ctx context.Context, req MissionRequest) (*MissionSnapshot, <-chan MissionEvent, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, nil, ErrManagerClosed
	}
	m.mu.RUnlock()

	wsID := strings.TrimSpace(req.WorkspaceID)
	if err := ValidateID(wsID); err != nil {
		return nil, nil, fmt.Errorf("invalid workspace_id: %w", err)
	}
	goal := strings.TrimSpace(req.Goal)
	if goal == "" {
		return nil, nil, errors.New("mission goal is required")
	}

	ws, err := m.engine.Workspaces().Get(wsID)
	if err != nil {
		return nil, nil, err
	}
	wsSnap, _, err := m.cfg.ForWorkspace(ws.RootPath, ws.TrustedHashes)
	if err != nil {
		return nil, nil, err
	}

	now := m.now().UTC()
	missionID := strings.TrimSpace(req.MissionID)
	if missionID == "" {
		missionID = fmt.Sprintf("msn_%d_%d_%s", now.UnixMilli(), m.missionNonce.Add(1), store.RandomHex(4))
	}
	if err := ValidateID(missionID); err != nil {
		return nil, nil, fmt.Errorf("invalid mission_id: %w", err)
	}

	wf, topo, err := resolveMissionWorkflow(req, wsSnap)
	if err != nil {
		return nil, nil, err
	}

	roleModels := make(map[string]string)
	for k, v := range wf.RoleModels {
		roleModels[k] = v
	}
	for k, v := range req.RoleModels {
		roleModels[k] = v
	}

	nodeSpecs := wf.Nodes
	edgeSpecs := wf.Edges
	if len(nodeSpecs) == 0 {
		nodeSpecs, edgeSpecs = defaultWorkflowNodesAndEdges(topo)
	}

	nodes := make([]FleetNode, 0, len(nodeSpecs))
	nodeWorktreeSpec := make(map[string]bool, len(nodeSpecs))
	for _, ns := range nodeSpecs {
		nid := strings.TrimSpace(ns.ID)
		if err := ValidateID(nid); err != nil {
			return nil, nil, fmt.Errorf("invalid workflow node id %q: %w", nid, err)
		}
		roleID := strings.TrimSpace(ns.Role)
		if roleID == "" {
			roleID = "coder"
		}
		label := strings.TrimSpace(ns.Label)
		if label == "" {
			label = nid
		}
		override := firstNonEmpty(roleModels[nid], roleModels[roleID])
		route, rErr := m.engine.Router().Resolve(roleID, ws.RoleModels[roleID], override, wsSnap)
		if rErr != nil {
			return nil, nil, fmt.Errorf("resolve route for node %q (role %q): %w", nid, roleID, rErr)
		}
		nodeWorktreeSpec[nid] = ns.Worktree
		nodes = append(nodes, FleetNode{
			ID:         nid,
			Label:      label,
			RoleID:     roleID,
			ProviderID: route.ProviderID,
			ModelID:    route.ModelID,
			Effort:     route.Effort,
			Status:     NodeIdle,
			UpdatedAt:  now,
		})
	}

	edges := make([]FleetEdge, 0, len(edgeSpecs))
	for _, es := range edgeSpecs {
		edges = append(edges, FleetEdge{
			From:  strings.TrimSpace(es.From),
			To:    strings.TrimSpace(es.To),
			Label: strings.TrimSpace(es.Label),
			Kind:  strings.TrimSpace(es.Kind),
		})
	}

	bb, err := newMarkdownBlackboardWithClock(m.cfg.Paths(), ws.ID, missionID, m.now)
	if err != nil {
		return nil, nil, err
	}

	maxConc := wf.MaxConcurrency
	if req.MaxConcurrency > 0 {
		maxConc = req.MaxConcurrency
	}
	if maxConc <= 0 {
		maxConc = wsSnap.Global.MaxConcurrency
	}
	if maxConc <= 0 {
		maxConc = 4
	}

	maxRounds := wf.MaxRounds
	if req.MaxRounds > 0 {
		maxRounds = req.MaxRounds
	}
	if maxRounds <= 0 && topo != TopoSharedBoard {
		maxRounds = DefaultMaxRounds
	}

	maxIdle := wf.MaxIdleRounds
	if req.MaxIdleRounds > 0 {
		maxIdle = req.MaxIdleRounds
	}
	if maxIdle <= 0 {
		maxIdle = DefaultMaxIdle
	}

	maxHops := wf.MaxHops
	if req.MaxHops > 0 {
		maxHops = req.MaxHops
	}
	if maxHops <= 0 {
		maxHops = DefaultMaxHops
	}

	budgetUSD := wf.BudgetUSD
	if req.BudgetUSD > 0 {
		budgetUSD = req.BudgetUSD
	}

	verifyCmd := strings.TrimSpace(wf.VerifyCommand)
	if strings.TrimSpace(req.VerifyCommand) != "" {
		verifyCmd = strings.TrimSpace(req.VerifyCommand)
	}

	useWorktrees := wf.UseWorktrees
	if req.UseWorktrees != nil {
		useWorktrees = *req.UseWorktrees
	}
	isGit := m.pool.IsGitRepo(ctx, ws.RootPath)
	targetBranch := strings.TrimSpace(req.TargetBranch)
	if isGit && targetBranch == "" {
		targetBranch, _ = m.pool.DetectDefaultBranch(ctx, ws.RootPath)
	}

	sbRaw := wsSnap.Global.SandboxMode
	if strings.TrimSpace(ws.SandboxMode) != "" {
		sbRaw = ws.SandboxMode
	}
	if strings.TrimSpace(req.SandboxMode) != "" {
		sbRaw = req.SandboxMode
	}

	snap := MissionSnapshot{
		ID:          missionID,
		WorkspaceID: ws.ID,
		WorkflowID:  wf.ID,
		Goal:        goal,
		Topology:    topo,
		Status:      MissionRunning,
		Nodes:       nodes,
		Edges:       edges,
		Tasks:       []BlackboardTask{},
		Notes:       []BlackboardNote{},
		MergeQueue:  []string{},
		BudgetUSD:   budgetUSD,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := bb.InitMission(&snap); err != nil {
		return nil, nil, err
	}

	missionCtx, missionCancel := context.WithCancel(ctx)
	rt := &missionRuntime{
		mgr:              m,
		ws:               ws,
		wsSnap:           wsSnap,
		wf:               wf,
		req:              req,
		bb:               bb,
		targetBranch:     targetBranch,
		isGitRepo:        isGit,
		useWorktrees:     useWorktrees,
		maxConcurrency:   maxConc,
		maxRounds:        maxRounds,
		maxIdleRounds:    maxIdle,
		maxHops:          maxHops,
		verifyCommand:    verifyCmd,
		sandboxMode:      tools.NormalizeSandboxMode(sbRaw),
		allowedDomains:   append([]string(nil), wsSnap.Rules.AllowedDomains...),
		roleModels:       roleModels,
		nodeWorktreeSpec: nodeWorktreeSpec,
		leases:             make(map[string]*WorktreeLease),
		nodeDiffs:          make(map[string]string),
		pendingSteers:      make(map[string][]string),
		stallCancelled:     make(map[string]bool),
		stallReleasedTasks: make(map[string][]string),
		snap:               snap,
		cancel:             missionCancel,
		done:               make(chan struct{}),
		subs:               make(map[uint64]chan MissionEvent),
	}

	stallTimeout := m.stallTimeout
	if stallTimeout <= 0 && wsSnap != nil && wsSnap.Global.WitnessTimeoutSeconds > 0 {
		stallTimeout = time.Duration(wsSnap.Global.WitnessTimeoutSeconds) * time.Second
	}

	rt.witness = NewWitness(WitnessOptions{
		StallTimeout:  stallTimeout,
		CheckInterval: m.checkInterval,
		Now:           m.now,
		OnStall: func(nodeID string, silentFor time.Duration) {
			rt.onNodeFirstStall(nodeID, silentFor)
		},
		OnRepeatStall: func(nodeID string, silentFor time.Duration, stallCount int) {
			rt.onNodeRepeatStall(nodeID, silentFor, stallCount)
		},
	})

	initialStream, _ := rt.addSubscriber()
	key := missionMapKey(ws.ID, missionID)

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		missionCancel()
		rt.closeSubscribers()
		return nil, nil, ErrManagerClosed
	}
	if _, exists := m.active[key]; exists {
		m.mu.Unlock()
		missionCancel()
		rt.closeSubscribers()
		return nil, nil, fmt.Errorf("mission %q is already running in workspace %q", missionID, ws.ID)
	}
	m.active[key] = rt
	m.mu.Unlock()

	initialClone := rt.snapshotClone()

	m.bgWG.Add(1)
	go func() {
		defer m.bgWG.Done()
		defer close(rt.done)
		defer func() {
			rt.witness.Stop()
			if rt.isGitRepo && rt.useWorktrees {
				_ = m.pool.CleanupMission(context.Background(), ws.RootPath, missionID)
			}
			finalSnap := rt.snapshotClone()
			m.mu.Lock()
			delete(m.active, key)
			m.finished[key] = finalSnap
			m.mu.Unlock()
			missionCancel()
			rt.closeSubscribers()
		}()

		rt.witness.Start(missionCtx)
		rt.emit(MissionEvent{
			Type:    EventMissionStarted,
			Message: fmt.Sprintf("Started %s mission", topo),
		})

		execErr := rt.executeTopology(missionCtx)
		switch {
		case execErr == nil:
			rt.finishMission(MissionCompleted, "")
		case errors.Is(execErr, context.Canceled) || errors.Is(execErr, context.DeadlineExceeded) || missionCtx.Err() != nil:
			rt.finishMission(MissionCancelled, execErr.Error())
		default:
			rt.finishMission(MissionFailed, execErr.Error())
		}
	}()

	return initialClone, initialStream, nil
}

// Subscribe 允许 Web 工作台 SSE 与 TUI 在任务运行中或结束后任意时刻订阅实时拓扑事件流。
func (m *Manager) Subscribe(wsID, missionID string) (*MissionSnapshot, <-chan MissionEvent, func(), error) {
	if err := ValidateID(wsID); err != nil {
		return nil, nil, nil, fmt.Errorf("invalid workspace id: %w", err)
	}
	if err := ValidateID(missionID); err != nil {
		return nil, nil, nil, fmt.Errorf("invalid mission id: %w", err)
	}

	key := missionMapKey(wsID, missionID)
	m.mu.RLock()
	rt, isLive := m.active[key]
	finSnap := m.finished[key]
	m.mu.RUnlock()

	if isLive && rt != nil {
		snap := rt.snapshotClone()
		ch, unsub := rt.addSubscriber()
		return snap, ch, unsub, nil
	}

	if finSnap != nil {
		closedCh := make(chan MissionEvent)
		close(closedCh)
		return finSnap.Clone(), closedCh, func() {}, nil
	}

	snap, err := m.GetMission(wsID, missionID)
	if err != nil {
		return nil, nil, nil, err
	}
	closedCh := make(chan MissionEvent)
	close(closedCh)
	return snap, closedCh, func() {}, nil
}

func (m *Manager) GetMission(wsID, missionID string) (*MissionSnapshot, error) {
	if err := ValidateID(wsID); err != nil {
		return nil, fmt.Errorf("invalid workspace id: %w", err)
	}
	if err := ValidateID(missionID); err != nil {
		return nil, fmt.Errorf("invalid mission id: %w", err)
	}

	key := missionMapKey(wsID, missionID)
	m.mu.RLock()
	if rt, ok := m.active[key]; ok {
		m.mu.RUnlock()
		return rt.snapshotClone(), nil
	}
	if fin, ok := m.finished[key]; ok {
		cp := fin.Clone()
		m.mu.RUnlock()
		return cp, nil
	}
	m.mu.RUnlock()

	missionDir := filepath.Join(m.cfg.Paths().WorkspacesDir(), wsID, "missions", missionID)
	if err := ensureStrictSubpath(m.cfg.Paths().WorkspacesDir(), missionDir); err != nil {
		return nil, err
	}
	if _, err := os.Stat(missionDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrMissionNotFound, missionID)
		}
		return nil, err
	}

	bb, err := newMarkdownBlackboardWithClock(m.cfg.Paths(), wsID, missionID, m.now)
	if err != nil {
		return nil, err
	}
	return bb.LoadSnapshot()
}

func (m *Manager) ListMissions(wsID string) ([]*MissionSnapshot, error) {
	if err := ValidateID(wsID); err != nil {
		return nil, fmt.Errorf("invalid workspace id: %w", err)
	}

	byID := make(map[string]*MissionSnapshot)

	m.mu.RLock()
	for _, rt := range m.active {
		if rt.ws.ID == wsID {
			s := rt.snapshotClone()
			byID[s.ID] = s
		}
	}
	for _, fin := range m.finished {
		if fin.WorkspaceID == wsID {
			if _, live := byID[fin.ID]; !live {
				byID[fin.ID] = fin.Clone()
			}
		}
	}
	m.mu.RUnlock()

	missionsDir := filepath.Join(m.cfg.Paths().WorkspacesDir(), wsID, "missions")
	if err := ensureStrictSubpath(m.cfg.Paths().WorkspacesDir(), missionsDir); err != nil {
		return nil, err
	}
	if entries, err := os.ReadDir(missionsDir); err == nil {
		for _, ent := range entries {
			if !ent.IsDir() {
				continue
			}
			mid := ent.Name()
			if ValidateID(mid) != nil {
				continue
			}
			if _, already := byID[mid]; already {
				continue
			}
			if bb, bErr := newMarkdownBlackboardWithClock(m.cfg.Paths(), wsID, mid, m.now); bErr == nil {
				if snap, lErr := bb.LoadSnapshot(); lErr == nil {
					byID[mid] = snap
				}
			}
		}
	}

	out := make([]*MissionSnapshot, 0, len(byID))
	for _, s := range byID {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (m *Manager) SteerNode(wsID, missionID, nodeID, instruction string) error {
	if err := ValidateID(wsID); err != nil {
		return fmt.Errorf("invalid workspace id: %w", err)
	}
	if err := ValidateID(missionID); err != nil {
		return fmt.Errorf("invalid mission id: %w", err)
	}
	if err := ValidateID(nodeID); err != nil {
		return fmt.Errorf("invalid node id: %w", err)
	}
	trimmed := strings.TrimSpace(instruction)
	if trimmed == "" {
		return errors.New("steer instruction is required")
	}

	key := missionMapKey(wsID, missionID)
	m.mu.RLock()
	rt, ok := m.active[key]
	m.mu.RUnlock()
	if !ok || rt == nil {
		return fmt.Errorf("%w: %s", ErrMissionNotFound, missionID)
	}

	return rt.steerNode(nodeID, trimmed)
}

func (m *Manager) StopMission(wsID, missionID string) error {
	if err := ValidateID(wsID); err != nil {
		return fmt.Errorf("invalid workspace id: %w", err)
	}
	if err := ValidateID(missionID); err != nil {
		return fmt.Errorf("invalid mission id: %w", err)
	}

	key := missionMapKey(wsID, missionID)
	m.mu.RLock()
	rt, ok := m.active[key]
	m.mu.RUnlock()
	if !ok || rt == nil {
		return fmt.Errorf("%w: %s", ErrMissionNotFound, missionID)
	}

	rt.stopAllRuns()
	return nil
}

func (m *Manager) InspectNodeDiff(wsID, missionID, nodeID string) (string, error) {
	if err := ValidateID(wsID); err != nil {
		return "", fmt.Errorf("invalid workspace id: %w", err)
	}
	if err := ValidateID(missionID); err != nil {
		return "", fmt.Errorf("invalid mission id: %w", err)
	}
	if err := ValidateID(nodeID); err != nil {
		return "", fmt.Errorf("invalid node id: %w", err)
	}

	key := missionMapKey(wsID, missionID)
	m.mu.RLock()
	rt := m.active[key]
	m.mu.RUnlock()

	if rt != nil {
		return rt.inspectNodeDiff(context.Background(), nodeID)
	}

	snap, err := m.GetMission(wsID, missionID)
	if err != nil {
		return "", err
	}
	foundNode := false
	for _, n := range snap.Nodes {
		if n.ID == nodeID {
			foundNode = true
			break
		}
	}
	if !foundNode {
		return "", fmt.Errorf("%w: %s", ErrNodeNotFound, nodeID)
	}

	bb, err := newMarkdownBlackboardWithClock(m.cfg.Paths(), wsID, missionID, m.now)
	if err != nil {
		return "", err
	}
	return bb.ReadNodeDiff(nodeID)
}

// DeleteMission 删除已结束或已中断的 Fleet 任务（清理关联 Git Worktree 分支与任务黑板目录）；若任务仍在活跃运行中则拒绝删除。
// 参数 id 支持纯 `missionID` 或 `wsID/missionID`。
func (m *Manager) DeleteMission(id string) error {
	if m == nil {
		return ErrManagerClosed
	}
	raw := strings.TrimSpace(id)
	if raw == "" {
		return fmt.Errorf("%w: empty mission id", ErrInvalidID)
	}

	var explicitWS, missionID string
	if wsPart, mPart, hasSlash := strings.Cut(raw, "/"); hasSlash {
		if err := ValidateID(wsPart); err != nil {
			return fmt.Errorf("invalid workspace id: %w", err)
		}
		if err := ValidateID(mPart); err != nil {
			return fmt.Errorf("invalid mission id: %w", err)
		}
		explicitWS = wsPart
		missionID = mPart
	} else {
		if err := ValidateID(raw); err != nil {
			return fmt.Errorf("invalid mission id: %w", err)
		}
		missionID = raw
	}

	m.mu.Lock()
	for _, rt := range m.active {
		if rt.snap.ID == missionID && (explicitWS == "" || rt.ws.ID == explicitWS) {
			m.mu.Unlock()
			return fmt.Errorf("cannot delete actively running mission %q; stop it first", missionID)
		}
	}

	resolvedWS := explicitWS
	if resolvedWS == "" {
		for _, fin := range m.finished {
			if fin.ID == missionID {
				resolvedWS = fin.WorkspaceID
				break
			}
		}
	}
	m.mu.Unlock()

	wsBase := m.cfg.Paths().WorkspacesDir()
	if resolvedWS == "" {
		if entries, err := os.ReadDir(wsBase); err == nil {
			for _, ent := range entries {
				if !ent.IsDir() || ValidateID(ent.Name()) != nil {
					continue
				}
				candDir := filepath.Join(wsBase, ent.Name(), "missions", missionID)
				if st, sErr := os.Stat(candDir); sErr == nil && st.IsDir() {
					resolvedWS = ent.Name()
					break
				}
			}
		}
	}
	if resolvedWS == "" {
		return fmt.Errorf("%w: %s", ErrMissionNotFound, missionID)
	}

	missionDir := filepath.Join(wsBase, resolvedWS, "missions", missionID)
	if err := ensureStrictSubpath(wsBase, missionDir); err != nil {
		return err
	}

	m.mu.Lock()
	key := missionMapKey(resolvedWS, missionID)
	if _, live := m.active[key]; live {
		m.mu.Unlock()
		return fmt.Errorf("cannot delete actively running mission %q; stop it first", missionID)
	}
	_, inFinished := m.finished[key]
	delete(m.finished, key)
	m.mu.Unlock()

	_, stErr := os.Stat(missionDir)
	if errors.Is(stErr, os.ErrNotExist) && !inFinished {
		return fmt.Errorf("%w: %s", ErrMissionNotFound, missionID)
	}

	repoRoot := ""
	if m.engine != nil && m.engine.Workspaces() != nil {
		if ws, gErr := m.engine.Workspaces().Get(resolvedWS); gErr == nil && ws != nil {
			repoRoot = ws.RootPath
		}
	}
	_ = m.pool.CleanupMission(context.Background(), repoRoot, missionID)
	if err := os.RemoveAll(missionDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove mission dir %s: %w", missionDir, err)
	}

	m.publishFleetUpdated(resolvedWS, missionID, EventMissionDeleted, "")
	return nil
}

// DiffSessionWorktree 计算独立会话 Worktree 相对于工作区主仓库当前/默认分支的完整 Git Diff（含未追踪文件）及 dirty 标志。
func (m *Manager) DiffSessionWorktree(wsPath, wtPath string) (string, bool, error) {
	if m == nil || m.pool == nil {
		return "", false, ErrManagerClosed
	}
	cleanWS := canonicalizeDir(wsPath)
	cleanWT := canonicalizeDir(wtPath)
	if cleanWS == "" || cleanWT == "" {
		return "", false, errors.New("workspace path and worktree path are required")
	}
	if _, err := os.Stat(cleanWT); err != nil {
		return "", false, fmt.Errorf("stat worktree %s: %w", cleanWT, err)
	}

	ctx := context.Background()
	if !m.pool.IsGitRepo(ctx, cleanWS) {
		return "", false, fmt.Errorf("workspace %s is not a git repository", cleanWS)
	}
	if !m.pool.IsGitRepo(ctx, cleanWT) {
		return "", false, fmt.Errorf("worktree %s is not a git repository", cleanWT)
	}

	baseBranch, err := m.pool.DetectDefaultBranch(ctx, cleanWS)
	if err != nil {
		return "", false, err
	}
	diff, err := m.pool.Diff(ctx, cleanWT, baseBranch)
	if err != nil {
		return "", false, err
	}
	statusOut, _ := runGitCmd(ctx, cleanWT, "status", "--porcelain")
	ahead, _ := countCommitsAhead(ctx, cleanWT, baseBranch)
	dirty := strings.TrimSpace(diff) != "" || strings.TrimSpace(statusOut) != "" || ahead > 0
	return diff, dirty, nil
}

// LandSessionWorktree 通过 Refinery 串行门控将会话隔离 Worktree 的改动自动归集提交、变基、执行可选验收命令并快进（--ff-only）合入主工作区。
func (m *Manager) LandSessionWorktree(ctx context.Context, wsPath, wtPath, branch, verifyCmd string) (MergeReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if m == nil || m.pool == nil || m.refinery == nil {
		return MergeReport{}, ErrManagerClosed
	}
	cleanWS := canonicalizeDir(wsPath)
	cleanWT := canonicalizeDir(wtPath)
	if cleanWS == "" || cleanWT == "" {
		return MergeReport{}, errors.New("workspace path and worktree path are required")
	}
	if !m.pool.IsGitRepo(ctx, cleanWS) {
		return MergeReport{}, fmt.Errorf("workspace %s is not a git repository", cleanWS)
	}
	if !m.pool.IsGitRepo(ctx, cleanWT) {
		return MergeReport{}, fmt.Errorf("worktree %s is not a git repository", cleanWT)
	}

	targetBranch, err := m.pool.DetectDefaultBranch(ctx, cleanWS)
	if err != nil {
		return MergeReport{}, err
	}
	wtBranch := strings.TrimSpace(branch)
	if wtBranch == "" {
		wtBranch, err = m.pool.DetectDefaultBranch(ctx, cleanWT)
		if err != nil {
			return MergeReport{}, err
		}
	}

	sbMode := tools.SandboxStrict
	var allowedDomains []string
	if m.engine != nil && m.engine.Workspaces() != nil {
		if ws, gErr := m.engine.Workspaces().GetByPath(cleanWS); gErr == nil && ws != nil {
			if wsSnap, _, cErr := m.cfg.ForWorkspace(ws.RootPath, ws.TrustedHashes); cErr == nil && wsSnap != nil {
				rawMode := wsSnap.Global.SandboxMode
				if strings.TrimSpace(ws.SandboxMode) != "" {
					rawMode = ws.SandboxMode
				}
				sbMode = tools.NormalizeSandboxMode(rawMode)
				allowedDomains = append([]string(nil), wsSnap.Rules.AllowedDomains...)
			}
		}
	}

	nodeSlug := sanitizeGitRefSegment(filepath.Base(cleanWT))
	lease := &WorktreeLease{
		MissionID:  "session",
		NodeID:     nodeSlug,
		RepoRoot:   cleanWS,
		Path:       cleanWT,
		Branch:     wtBranch,
		BaseBranch: targetBranch,
	}

	res, err := m.refinery.Land(ctx, LandRequest{
		Lease:          lease,
		TargetBranch:   targetBranch,
		CommitMessage:  fmt.Sprintf("feat(session): land worktree %s", wtBranch),
		VerifyCommand:  strings.TrimSpace(verifyCmd),
		SandboxMode:    sbMode,
		AllowedDomains: allowedDomains,
	})
	if res == nil {
		return MergeReport{}, err
	}
	return *res, err
}

func (m *Manager) Wait() {
	if m == nil {
		return
	}
	m.bgWG.Wait()
}

func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	rts := make([]*missionRuntime, 0, len(m.active))
	for _, rt := range m.active {
		rts = append(rts, rt)
	}
	m.mu.Unlock()

	for _, rt := range rts {
		rt.stopAllRuns()
	}
	m.bgWG.Wait()
	return nil
}

type missionRuntime struct {
	mgr              *Manager
	ws               *store.Workspace
	wsSnap           *config.Snapshot
	wf               config.WorkflowTemplate
	req              MissionRequest
	bb               *MarkdownBlackboard
	witness          *Witness
	targetBranch     string
	isGitRepo        bool
	useWorktrees     bool
	maxConcurrency   int
	maxRounds        int
	maxIdleRounds    int
	maxHops          int
	verifyCommand    string
	sandboxMode      tools.SandboxMode
	allowedDomains   []string
	roleModels       map[string]string
	nodeWorktreeSpec map[string]bool

	mu                 sync.Mutex
	snap               MissionSnapshot
	leases             map[string]*WorktreeLease
	nodeDiffs          map[string]string
	pendingSteers      map[string][]string
	stallCancelled     map[string]bool
	stallReleasedTasks map[string][]string

	cancel context.CancelFunc
	done   chan struct{}

	subsMu     sync.Mutex
	subs       map[uint64]chan MissionEvent
	subSeq     uint64
	subsClosed bool

	eventSeq atomic.Uint64
	pulseSeq atomic.Uint64
}

func (rt *missionRuntime) addSubscriber() (<-chan MissionEvent, func()) {
	ch := make(chan MissionEvent, 256)
	rt.subsMu.Lock()
	if rt.subsClosed {
		rt.subsMu.Unlock()
		close(ch)
		return ch, func() {}
	}
	rt.subSeq++
	id := rt.subSeq
	rt.subs[id] = ch
	rt.subsMu.Unlock()

	var once sync.Once
	unsub := func() {
		once.Do(func() {
			rt.subsMu.Lock()
			if existing, ok := rt.subs[id]; ok {
				delete(rt.subs, id)
				close(existing)
			}
			rt.subsMu.Unlock()
		})
	}
	return ch, unsub
}

func (rt *missionRuntime) closeSubscribers() {
	rt.subsMu.Lock()
	if rt.subsClosed {
		rt.subsMu.Unlock()
		return
	}
	rt.subsClosed = true
	for id, ch := range rt.subs {
		delete(rt.subs, id)
		close(ch)
	}
	rt.subsMu.Unlock()
}

func (rt *missionRuntime) emit(ev MissionEvent) {
	now := rt.mgr.now().UTC()
	snap := rt.snapshotClone()

	ev.Seq = rt.eventSeq.Add(1)
	ev.MissionID = snap.ID
	ev.WorkspaceID = snap.WorkspaceID
	ev.Snapshot = snap
	ev.Timestamp = now

	_ = rt.bb.AppendEvent(ev)

	isTerminal := ev.Type == EventMissionCompleted || ev.Type == EventMissionFailed || ev.Type == EventMissionCancelled

	rt.subsMu.Lock()
	if !rt.subsClosed {
		for _, ch := range rt.subs {
			select {
			case ch <- ev:
			default:
				if isTerminal {
					// 终态事件若遇到满缓冲，淘汰最旧一帧确保终态必达
					select {
					case <-ch:
					default:
					}
					select {
					case ch <- ev:
					default:
					}
				}
			}
		}
	}
	rt.subsMu.Unlock()

	if ev.Type != EventNodeDelta {
		rt.mgr.publishFleetUpdated(ev.WorkspaceID, ev.MissionID, ev.Type, ev.NodeID)
	}
}

func (rt *missionRuntime) snapshotClone() *MissionSnapshot {
	rt.mu.Lock()
	cp := rt.snap.Clone()
	rt.mu.Unlock()
	return cp
}

func (rt *missionRuntime) nodesSnapshot() []FleetNode {
	rt.mu.Lock()
	out := make([]FleetNode, len(rt.snap.Nodes))
	copy(out, rt.snap.Nodes)
	rt.mu.Unlock()
	return out
}

func (rt *missionRuntime) firstNodeByRole(role string) *FleetNode {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	for _, n := range rt.snap.Nodes {
		if n.RoleID == role {
			cp := n
			return &cp
		}
	}
	return nil
}

func (rt *missionRuntime) nodesByRole(role string) []FleetNode {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	var out []FleetNode
	for _, n := range rt.snap.Nodes {
		if n.RoleID == role {
			out = append(out, n)
		}
	}
	return out
}

func (rt *missionRuntime) shouldUseWorktree(nodeID string) bool {
	if !rt.isGitRepo {
		return false
	}
	if rt.req.UseWorktrees != nil && !*rt.req.UseWorktrees {
		return false
	}
	if specWT, exists := rt.nodeWorktreeSpec[nodeID]; exists && specWT {
		return true
	}
	return rt.useWorktrees
}

func (rt *missionRuntime) acquireNodeWorktree(ctx context.Context, nodeID, wtSlotName string) (*WorktreeLease, error) {
	lease, err := rt.mgr.pool.Acquire(ctx, rt.ws.RootPath, rt.snap.ID, wtSlotName, rt.targetBranch)
	if err != nil {
		return nil, err
	}
	now := rt.mgr.now().UTC()
	rt.mu.Lock()
	rt.leases[nodeID] = lease
	for i := range rt.snap.Nodes {
		if rt.snap.Nodes[i].ID == nodeID {
			rt.snap.Nodes[i].WorktreePath = lease.Path
			rt.snap.Nodes[i].WorktreeBranch = lease.Branch
			rt.snap.Nodes[i].UpdatedAt = now
			break
		}
	}
	rt.snap.UpdatedAt = now
	rt.mu.Unlock()

	rt.emit(MissionEvent{
		Type:    EventNodeUpdated,
		NodeID:  nodeID,
		Message: "allocated worktree " + lease.Branch,
	})
	return lease, nil
}

func (rt *missionRuntime) releaseWorktree(ctx context.Context, nodeID string, lease *WorktreeLease, deleteBranch bool) {
	if lease == nil {
		return
	}
	rt.mu.Lock()
	if cur, ok := rt.leases[nodeID]; ok && cur.Path == lease.Path {
		delete(rt.leases, nodeID)
	}
	rt.mu.Unlock()
	_ = rt.mgr.pool.Release(ctx, lease, deleteBranch)
}

func (rt *missionRuntime) ensureNodeSession(nodeID string) (FleetNode, error) {
	rt.mu.Lock()
	idx := -1
	for i := range rt.snap.Nodes {
		if rt.snap.Nodes[i].ID == nodeID {
			idx = i
			break
		}
	}
	if idx < 0 {
		rt.mu.Unlock()
		return FleetNode{}, fmt.Errorf("%w: %s", ErrNodeNotFound, nodeID)
	}
	node := rt.snap.Nodes[idx]
	rt.mu.Unlock()

	if node.SessionID != "" {
		return node, nil
	}

	title := fmt.Sprintf("Fleet %s · %s", rt.snap.ID, node.Label)
	sess, err := rt.mgr.engine.Sessions().Create(rt.ws.ID, title, node.RoleID)
	if err != nil {
		return FleetNode{}, err
	}

	rt.mu.Lock()
	if rt.snap.Nodes[idx].SessionID == "" {
		rt.snap.Nodes[idx].SessionID = sess.ID
	}
	node = rt.snap.Nodes[idx]
	rt.mu.Unlock()
	return node, nil
}

func (rt *missionRuntime) runNode(
	ctx context.Context,
	nodeID, prompt, worktreePath string,
	dynamicCtx []string,
) (core.RunOutcome, error) {
	for attempt := 0; attempt < 2; attempt++ {
		attemptDyn := dynamicCtx
		if attempt > 0 {
			attemptDyn = append(append([]string(nil), dynamicCtx...), "Witness recovery requeue: your previous execution stalled without progress; complete the assigned task concisely.")
		}
		outcome, err := rt.runNodeOnce(ctx, nodeID, prompt, worktreePath, attemptDyn)
		if err == nil {
			return outcome, nil
		}
		if ctx.Err() == nil && rt.consumeStallCancel(nodeID) {
			if rt.snap.Topology == TopoSharedBoard {
				return outcome, errNodeStallReleased
			}
			if attempt == 0 {
				rt.reclaimStallTasks(nodeID, rt.nodeWorktreeBranch(nodeID))
				continue
			}
			return outcome, fmt.Errorf("node %s stalled repeatedly and was cancelled", nodeID)
		}
		return outcome, err
	}
	return core.RunOutcome{}, fmt.Errorf("node %s failed after stall recovery retry", nodeID)
}

func (rt *missionRuntime) nodeWorktreeBranch(nodeID string) string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if l := rt.leases[nodeID]; l != nil {
		return l.Branch
	}
	for _, n := range rt.snap.Nodes {
		if n.ID == nodeID {
			return n.WorktreeBranch
		}
	}
	return ""
}

func (rt *missionRuntime) consumeStallCancel(nodeID string) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.stallCancelled[nodeID] {
		delete(rt.stallCancelled, nodeID)
		return true
	}
	return false
}

func (rt *missionRuntime) reclaimStallTasks(nodeID, branch string) {
	rt.mu.Lock()
	ids := rt.stallReleasedTasks[nodeID]
	delete(rt.stallReleasedTasks, nodeID)
	rt.mu.Unlock()
	if len(ids) == 0 {
		return
	}
	claimedAny := false
	for _, tid := range ids {
		if _, won, err := rt.bb.ClaimTask(tid, nodeID, branch); err == nil && won {
			claimedAny = true
		}
	}
	if claimedAny {
		rt.syncBlackboardState(EventTaskUpdated, nodeID, "", "reclaimed tasks on stall retry for "+nodeID)
	}
}

func (rt *missionRuntime) restoreRunningIfStalled(nodeID string) {
	rt.mu.Lock()
	for i := range rt.snap.Nodes {
		if rt.snap.Nodes[i].ID == nodeID && rt.snap.Nodes[i].Status == NodeStalled {
			rt.snap.Nodes[i].Status = NodeRunning
			rt.snap.Nodes[i].UpdatedAt = rt.mgr.now().UTC()
			break
		}
	}
	rt.mu.Unlock()
}

func (rt *missionRuntime) runNodeOnce(
	ctx context.Context,
	nodeID, prompt, worktreePath string,
	dynamicCtx []string,
) (core.RunOutcome, error) {
	var zero core.RunOutcome
	if ctx.Err() != nil {
		return zero, ctx.Err()
	}
	if err := rt.checkBudget(); err != nil {
		return zero, err
	}

	node, err := rt.ensureNodeSession(nodeID)
	if err != nil {
		return zero, err
	}

	override := firstNonEmpty(rt.roleModels[nodeID], rt.roleModels[node.RoleID])
	wtBranch := rt.nodeWorktreeBranch(nodeID)

	// 先提取已排队的节点 Steer 指令
	var dyn []string
	if len(dynamicCtx) > 0 {
		dyn = append(dyn, dynamicCtx...)
	}
	rt.mu.Lock()
	delete(rt.stallCancelled, nodeID)
	if queued := rt.pendingSteers[nodeID]; len(queued) > 0 {
		for _, s := range queued {
			dyn = append(dyn, "Steer: "+s)
		}
		delete(rt.pendingSteers, nodeID)
	}
	rt.mu.Unlock()

	runID, runStream, err := rt.mgr.engine.StartRun(ctx, core.RunRequest{
		WorkspaceID:    rt.ws.ID,
		SessionID:      node.SessionID,
		Prompt:         prompt,
		AgentRole:      node.RoleID,
		ModelOverride:  override,
		PermMode:       rt.req.PermMode,
		SandboxMode:    rt.req.SandboxMode,
		WorktreePath:   worktreePath,
		ExtraTools:     rt.makeNodeBlackboardTools(nodeID, wtBranch),
		DynamicContext: dyn,
	})
	if err != nil {
		return zero, err
	}

	now := rt.mgr.now().UTC()
	var lateSteers []string
	rt.mu.Lock()
	for i := range rt.snap.Nodes {
		if rt.snap.Nodes[i].ID == nodeID {
			rt.snap.Nodes[i].Status = NodeRunning
			rt.snap.Nodes[i].ActiveRunID = runID
			if worktreePath != "" {
				rt.snap.Nodes[i].WorktreePath = worktreePath
			}
			rt.snap.Nodes[i].UpdatedAt = now
			break
		}
	}
	if queued := rt.pendingSteers[nodeID]; len(queued) > 0 {
		lateSteers = append(lateSteers, queued...)
		delete(rt.pendingSteers, nodeID)
	}
	rt.snap.UpdatedAt = now
	rt.mu.Unlock()

	for _, ls := range lateSteers {
		_ = rt.mgr.engine.Steer(runID, ls)
	}

	rt.witness.WatchNode(nodeID)
	defer rt.witness.UnwatchNode(nodeID)

	rt.emit(MissionEvent{
		Type:    EventNodeUpdated,
		NodeID:  nodeID,
		Message: "running " + runID,
	})

	var (
		outcome core.RunOutcome
		runErr  error
	)

	for ev := range runStream {
		if ev.Type != core.EventSteerInjected {
			rt.witness.Beat(nodeID)
			rt.restoreRunningIfStalled(nodeID)
		}
		switch ev.Type {
		case core.EventTextDelta:
			if m, ok := ev.Payload.(map[string]any); ok {
				if txt, _ := m["text"].(string); txt != "" {
					rt.emit(MissionEvent{
						Type:    EventNodeDelta,
						NodeID:  nodeID,
						Message: txt,
					})
				}
			}

		case core.EventSteerInjected:
			if p, ok := ev.Payload.(core.SteerInjectedPayload); ok {
				rt.emit(MissionEvent{
					Type:    EventNodeSteered,
					NodeID:  nodeID,
					Message: p.Message,
				})
			}

		case core.EventApprovalRequired:
			rt.witness.PauseNode(nodeID)
			rt.emit(MissionEvent{
				Type:    EventApprovalRequired,
				NodeID:  nodeID,
				Message: "approval required",
				Payload: ev.Payload,
			})

		case core.EventApprovalResolved:
			rt.witness.ResumeNode(nodeID)

		case core.EventToolResult:
			if worktreePath != "" {
				rt.refreshLiveWorktreeDiff(ctx, nodeID, worktreePath)
			}

		case core.EventRunCompleted:
			switch p := ev.Payload.(type) {
			case core.RunOutcome:
				outcome = p
			case *core.RunOutcome:
				if p != nil {
					outcome = *p
				}
			}

		case core.EventRunFailed:
			switch p := ev.Payload.(type) {
			case core.RunErrorPayload:
				runErr = errors.New(p.Message)
			case *core.RunErrorPayload:
				if p != nil {
					runErr = errors.New(p.Message)
				}
			default:
				runErr = fmt.Errorf("node %s run failed", nodeID)
			}

		case core.EventRunCancelled:
			if ctx.Err() != nil {
				runErr = ctx.Err()
			} else {
				runErr = context.Canceled
			}
		}
	}

	if worktreePath != "" {
		rt.refreshLiveWorktreeDiff(context.Background(), nodeID, worktreePath)
	}

	endStatus := NodeCompleted
	if runErr != nil {
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
			endStatus = NodeCancelled
		} else {
			endStatus = NodeFailed
		}
	}

	stepTokens := outcome.InputTokens + outcome.OutputTokens + outcome.CacheReadTokens
	finNow := rt.mgr.now().UTC()

	rt.mu.Lock()
	var totalCost float64
	for i := range rt.snap.Nodes {
		if rt.snap.Nodes[i].ID == nodeID {
			rt.snap.Nodes[i].ActiveRunID = ""
			rt.snap.Nodes[i].Status = endStatus
			rt.snap.Nodes[i].Tokens += stepTokens
			rt.snap.Nodes[i].CostUSD += outcome.CostUSD
			rt.snap.Nodes[i].UpdatedAt = finNow
		}
		totalCost += rt.snap.Nodes[i].CostUSD
	}
	rt.snap.TotalCostUSD = totalCost
	rt.snap.UpdatedAt = finNow
	snapToPersist := rt.snap.Clone()
	rt.mu.Unlock()

	_ = rt.bb.SaveSnapshot(snapToPersist)
	rt.emit(MissionEvent{
		Type:    EventNodeUpdated,
		NodeID:  nodeID,
		Message: endStatus,
	})

	if runErr != nil {
		return outcome, runErr
	}
	if err := rt.checkBudget(); err != nil {
		return outcome, err
	}
	return outcome, nil
}

func (rt *missionRuntime) verifyAndMaybeLand(
	ctx context.Context,
	authorNodeID string,
	verifier *FleetNode,
	lease *WorktreeLease,
	commitMsg string,
) (*LandResult, error) {
	blindFn := BlindReviewFunc(nil)
	if verifier != nil {
		verID := verifier.ID
		blindFn = func(bCtx context.Context, diff, verifyOut string) (bool, string, error) {
			wtForVerifier := ""
			if lease != nil {
				wtForVerifier = lease.Path
			}
			diffPreview := diff
			if strings.TrimSpace(diffPreview) == "" {
				diffPreview = "(no file diff)"
			}
			auditPrompt := fmt.Sprintf(
				"Adversarial Blind Review for Mission Goal:\n%s\n\nCandidate Node: %s\nAutomated Verify Output:\n%s\n\nGit Diff:\n%s\n\nEnd your review with `VERDICT: ACCEPT` or `VERDICT: REJECT`.",
				rt.snap.Goal, authorNodeID, verifyOut, diffPreview,
			)
			vOut, err := rt.runNode(bCtx, verID, auditPrompt, wtForVerifier, nil)
			if err != nil {
				return false, "", err
			}
			accepted, feedback := parseVerifierVerdict(vOut.FinalText)
			return accepted, feedback, nil
		}
	}

	landReq := LandRequest{
		Lease:          lease,
		TargetBranch:   rt.targetBranch,
		CommitMessage:  commitMsg,
		VerifyCommand:  rt.verifyCommand,
		SandboxMode:    rt.sandboxMode,
		AllowedDomains: rt.allowedDomains,
		BlindReview:    blindFn,
		OnQueueEnter: func(item string) {
			rt.pushMergeQueue(item)
		},
		OnQueueExit: func(item string) {
			rt.popMergeQueue(item)
		},
	}

	var (
		landRes *LandResult
		err     error
	)
	if lease == nil {
		// 未使用独立 Git Worktree 时仍通过 Refinery 互斥锁严格串行化验收与盲审，防止并发争用同一 Verifier 会话
		landRes, err = rt.mgr.refinery.VerifyInPlace(ctx, authorNodeID, rt.ws.RootPath, rt.isGitRepo, landReq)
	} else {
		landRes, err = rt.mgr.refinery.Land(ctx, landReq)
	}
	if err != nil {
		return nil, err
	}

	if landRes.Diff != "" || landRes.DiffStat != "" {
		rt.cacheNodeDiff(authorNodeID, landRes.Diff, landRes.DiffStat)
	}

	if landRes.Accepted {
		rt.emit(MissionEvent{
			Type:    EventMergeLanded,
			NodeID:  authorNodeID,
			Message: fmt.Sprintf("landed %s onto %s (%s)", authorNodeID, rt.targetBranch, shortSHA(landRes.LandedCommit)),
		})
	} else {
		rt.emit(MissionEvent{
			Type:    EventMergeRejected,
			NodeID:  authorNodeID,
			Message: fmt.Sprintf("rejected %s at %s", authorNodeID, landRes.Stage),
		})
	}
	return landRes, nil
}

func (rt *missionRuntime) refreshLiveWorktreeDiff(ctx context.Context, nodeID, wtPath string) {
	diff, dErr := rt.mgr.pool.Diff(ctx, wtPath, rt.targetBranch)
	stat, sErr := rt.mgr.pool.DiffStat(ctx, wtPath, rt.targetBranch)
	if dErr == nil || sErr == nil {
		rt.cacheNodeDiff(nodeID, diff, stat)
	}
}

func (rt *missionRuntime) cacheNodeDiff(nodeID, diff, diffStat string) {
	now := rt.mgr.now().UTC()
	rt.mu.Lock()
	if strings.TrimSpace(diff) != "" {
		rt.nodeDiffs[nodeID] = diff
	}
	if strings.TrimSpace(diffStat) != "" {
		for i := range rt.snap.Nodes {
			if rt.snap.Nodes[i].ID == nodeID {
				rt.snap.Nodes[i].DiffStat = strings.TrimSpace(diffStat)
				rt.snap.Nodes[i].UpdatedAt = now
				break
			}
		}
		rt.snap.UpdatedAt = now
	}
	rt.mu.Unlock()

	if strings.TrimSpace(diff) != "" {
		_ = rt.bb.SaveNodeDiff(nodeID, diff)
	}
}

func (rt *missionRuntime) inspectNodeDiff(ctx context.Context, nodeID string) (string, error) {
	rt.mu.Lock()
	found := false
	for _, n := range rt.snap.Nodes {
		if n.ID == nodeID {
			found = true
			break
		}
	}
	lease := rt.leases[nodeID]
	cached := rt.nodeDiffs[nodeID]
	rt.mu.Unlock()

	if !found {
		return "", fmt.Errorf("%w: %s", ErrNodeNotFound, nodeID)
	}
	if lease != nil && lease.Path != "" {
		if _, err := os.Stat(lease.Path); err == nil {
			if liveDiff, dErr := rt.mgr.pool.Diff(ctx, lease.Path, rt.targetBranch); dErr == nil && strings.TrimSpace(liveDiff) != "" {
				return liveDiff, nil
			}
		}
	}
	if strings.TrimSpace(cached) != "" {
		return cached, nil
	}
	return rt.bb.ReadNodeDiff(nodeID)
}

func (rt *missionRuntime) steerNode(nodeID, instruction string) error {
	rt.mu.Lock()
	idx := -1
	for i := range rt.snap.Nodes {
		if rt.snap.Nodes[i].ID == nodeID {
			idx = i
			break
		}
	}
	if idx < 0 {
		rt.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrNodeNotFound, nodeID)
	}
	if isTerminalMissionStatus(rt.snap.Status) {
		rt.mu.Unlock()
		return fmt.Errorf("mission %s is already %s", rt.snap.ID, rt.snap.Status)
	}
	activeRunID := rt.snap.Nodes[idx].ActiveRunID
	sessID := rt.snap.Nodes[idx].SessionID
	rt.mu.Unlock()

	if activeRunID != "" {
		if err := rt.mgr.engine.Steer(activeRunID, instruction); err == nil {
			return nil
		}
	}

	// 若 StartRun 刚完成注册但尚未写回 ActiveRunID，通过 SessionID 反查活跃运行直接投递
	if sessID != "" {
		for _, ar := range rt.mgr.engine.ActiveRuns() {
			if ar.SessionID == sessID {
				if err := rt.mgr.engine.Steer(ar.RunID, instruction); err == nil {
					return nil
				}
			}
		}
	}

	// 节点尚处于排队或轮次间隙：缓存至该节点待注入队列，下一轮 StartRun 立即生效
	rt.mu.Lock()
	rt.pendingSteers[nodeID] = append(rt.pendingSteers[nodeID], instruction)
	rt.mu.Unlock()

	rt.emit(MissionEvent{
		Type:    EventNodeSteered,
		NodeID:  nodeID,
		Message: instruction,
	})
	return nil
}

func (rt *missionRuntime) stopAllRuns() {
	rt.mu.Lock()
	var runIDs []string
	for _, n := range rt.snap.Nodes {
		if n.ActiveRunID != "" {
			runIDs = append(runIDs, n.ActiveRunID)
		}
	}
	rt.mu.Unlock()

	for _, rid := range runIDs {
		_ = rt.mgr.engine.StopRun(rid)
	}
	if rt.cancel != nil {
		rt.cancel()
	}
}

func (rt *missionRuntime) pulseEdge(fromID, toID, msg string) {
	seq := rt.pulseSeq.Add(1)
	now := rt.mgr.now().UTC()

	rt.mu.Lock()
	matched := false
	for i := range rt.snap.Edges {
		if rt.snap.Edges[i].From == fromID && rt.snap.Edges[i].To == toID {
			rt.snap.Edges[i].Active = true
			rt.snap.Edges[i].PulseSeq = seq
			matched = true
		} else {
			rt.snap.Edges[i].Active = false
		}
	}
	if !matched && fromID != "" && toID != "" {
		rt.snap.Edges = append(rt.snap.Edges, FleetEdge{
			From:     fromID,
			To:       toID,
			Label:    msg,
			Kind:     "event",
			Active:   true,
			PulseSeq: seq,
		})
	}
	rt.snap.UpdatedAt = now
	rt.mu.Unlock()

	rt.emit(MissionEvent{
		Type:     EventEdgePulse,
		EdgeFrom: fromID,
		EdgeTo:   toID,
		Message:  msg,
	})
}

func (rt *missionRuntime) pushMergeQueue(item string) {
	now := rt.mgr.now().UTC()
	rt.mu.Lock()
	rt.snap.MergeQueue = append(rt.snap.MergeQueue, item)
	for i := range rt.snap.Nodes {
		if rt.snap.Nodes[i].ID == item {
			rt.snap.Nodes[i].Status = NodeMerging
			rt.snap.Nodes[i].UpdatedAt = now
		}
	}
	rt.snap.UpdatedAt = now
	rt.mu.Unlock()

	rt.emit(MissionEvent{
		Type:    EventMergeQueued,
		NodeID:  item,
		Message: "queued in refinery: " + item,
	})
}

func (rt *missionRuntime) popMergeQueue(item string) {
	now := rt.mgr.now().UTC()
	rt.mu.Lock()
	filtered := make([]string, 0, len(rt.snap.MergeQueue))
	for _, x := range rt.snap.MergeQueue {
		if x != item {
			filtered = append(filtered, x)
		}
	}
	rt.snap.MergeQueue = filtered
	for i := range rt.snap.Nodes {
		if rt.snap.Nodes[i].ID == item && rt.snap.Nodes[i].Status == NodeMerging {
			rt.snap.Nodes[i].Status = NodeCompleted
			rt.snap.Nodes[i].UpdatedAt = now
		}
	}
	rt.snap.UpdatedAt = now
	rt.mu.Unlock()
}

func (rt *missionRuntime) recordNote(nodeID, title, body string) (BlackboardNote, error) {
	note, err := rt.bb.AddNote(nodeID, title, body)
	if err != nil {
		return BlackboardNote{}, err
	}
	rt.syncBlackboardState(EventNoteAdded, nodeID, "", title)
	return note, nil
}

func (rt *missionRuntime) syncBlackboardState(evType, nodeID, taskID, msg string) {
	tasks, _ := rt.bb.ListTasks()
	notes, _ := rt.bb.ListNotes()
	now := rt.mgr.now().UTC()

	rt.mu.Lock()
	if tasks != nil {
		rt.snap.Tasks = tasks
	}
	if notes != nil {
		rt.snap.Notes = notes
	}
	rt.snap.UpdatedAt = now
	snapCopy := rt.snap.Clone()
	rt.mu.Unlock()

	_ = rt.bb.SaveSnapshot(snapCopy)
	rt.emit(MissionEvent{
		Type:    evType,
		NodeID:  nodeID,
		TaskID:  taskID,
		Message: msg,
	})
}

func (rt *missionRuntime) setSummary(summary string) {
	rt.mu.Lock()
	rt.snap.Summary = strings.TrimSpace(summary)
	rt.snap.UpdatedAt = rt.mgr.now().UTC()
	rt.mu.Unlock()
}

func (rt *missionRuntime) onNodeFirstStall(nodeID string, silentFor time.Duration) {
	now := rt.mgr.now().UTC()
	var activeRunID string
	rt.mu.Lock()
	for i := range rt.snap.Nodes {
		if rt.snap.Nodes[i].ID == nodeID {
			if rt.snap.Nodes[i].Status == NodeRunning {
				rt.snap.Nodes[i].Status = NodeStalled
				rt.snap.Nodes[i].UpdatedAt = now
			}
			activeRunID = rt.snap.Nodes[i].ActiveRunID
		}
	}
	rt.snap.UpdatedAt = now
	rt.mu.Unlock()

	steerMsg := fmt.Sprintf("Witness stall recovery: node %s has been silent for %s; conclude current step or report blocker immediately.", nodeID, silentFor.Round(time.Millisecond))
	if activeRunID != "" {
		_ = rt.mgr.engine.Steer(activeRunID, steerMsg)
	}

	rt.emit(MissionEvent{
		Type:    EventNodeStalled,
		NodeID:  nodeID,
		Message: fmt.Sprintf("node %s silent for %s (steered once)", nodeID, silentFor.Round(time.Millisecond)),
	})
}

func (rt *missionRuntime) onNodeRepeatStall(nodeID string, silentFor time.Duration, stallCount int) {
	if stallCount < 2 {
		return
	}
	now := rt.mgr.now().UTC()
	var activeRunID string
	rt.mu.Lock()
	for i := range rt.snap.Nodes {
		if rt.snap.Nodes[i].ID == nodeID {
			rt.snap.Nodes[i].Status = NodeStalled
			rt.snap.Nodes[i].UpdatedAt = now
			activeRunID = rt.snap.Nodes[i].ActiveRunID
		}
	}
	rt.stallCancelled[nodeID] = true
	rt.snap.UpdatedAt = now
	rt.mu.Unlock()

	if released, err := rt.bb.ReleaseNodeTasks(nodeID); err == nil && len(released) > 0 {
		ids := make([]string, 0, len(released))
		for _, t := range released {
			ids = append(ids, t.ID)
		}
		rt.mu.Lock()
		rt.stallReleasedTasks[nodeID] = ids
		rt.mu.Unlock()
		rt.syncBlackboardState(EventTaskUpdated, nodeID, "", fmt.Sprintf("released %d stalled task(s) from %s", len(ids), nodeID))
	}

	rt.emit(MissionEvent{
		Type:    EventNodeStalled,
		NodeID:  nodeID,
		Message: fmt.Sprintf("node %s remained stalled (%s); cancelling run and releasing tasks", nodeID, silentFor.Round(time.Millisecond)),
	})

	if activeRunID != "" {
		_ = rt.mgr.engine.StopRun(activeRunID)
	}
}

func (rt *missionRuntime) checkBudget() error {
	rt.mu.Lock()
	budget := rt.snap.BudgetUSD
	spent := rt.snap.TotalCostUSD
	rt.mu.Unlock()
	if budget > 0 && spent > budget {
		return fmt.Errorf("%w: spent $%.4f > budget $%.4f", ErrBudgetExceeded, spent, budget)
	}
	return nil
}

func (rt *missionRuntime) finishMission(status, errMsg string) {
	tasks, _ := rt.bb.ListTasks()
	notes, _ := rt.bb.ListNotes()
	now := rt.mgr.now().UTC()

	rt.mu.Lock()
	rt.snap.Status = status
	if strings.TrimSpace(errMsg) != "" {
		rt.snap.Error = strings.TrimSpace(errMsg)
	}
	if tasks != nil {
		rt.snap.Tasks = tasks
	}
	if notes != nil {
		rt.snap.Notes = notes
	}
	for i := range rt.snap.Edges {
		rt.snap.Edges[i].Active = false
	}
	rt.snap.MergeQueue = []string{}
	rt.snap.UpdatedAt = now
	snapCopy := rt.snap.Clone()
	rt.mu.Unlock()

	_ = rt.bb.SaveSnapshot(snapCopy)

	evType := EventMissionCompleted
	switch status {
	case MissionFailed:
		evType = EventMissionFailed
	case MissionCancelled:
		evType = EventMissionCancelled
	}
	rt.emit(MissionEvent{
		Type:    evType,
		Message: firstNonEmpty(errMsg, status),
	})
}

func resolveMissionWorkflow(req MissionRequest, snap *config.Snapshot) (config.WorkflowTemplate, TopologyKind, error) {
	wfID := strings.TrimSpace(req.WorkflowID)
	topoRaw := strings.TrimSpace(string(req.Topology))

	if wfID == "" && topoRaw != "" {
		wfID = topoRaw
	}
	if wfID == "" {
		wfID = string(TopoHybridFleet)
	}

	if snap != nil {
		if wf, ok := snap.Workflows[wfID]; ok {
			tStr := firstNonEmpty(topoRaw, wf.Topology, wf.ID)
			topo, err := ParseTopologyKind(tStr)
			if err != nil {
				return config.WorkflowTemplate{}, "", err
			}
			return wf, topo, nil
		}
	}

	topo, err := ParseTopologyKind(firstNonEmpty(topoRaw, wfID))
	if err != nil {
		return config.WorkflowTemplate{}, "", err
	}
	nodes, edges := defaultWorkflowNodesAndEdges(topo)
	return config.WorkflowTemplate{
		ID:           wfID,
		Name:         wfID,
		Topology:     string(topo),
		UseWorktrees: topo != TopoParallelExplore,
		Nodes:        nodes,
		Edges:        edges,
	}, topo, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
