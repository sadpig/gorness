package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"
	"gorness.dev/gorness/internal/config"
)

type ContentBlock struct {
	Type           string          `json:"type"`
	Text           string          `json:"text,omitempty"`
	Thinking       string          `json:"thinking,omitempty"`
	Signature      string          `json:"signature,omitempty"`
	ToolCallID     string          `json:"tool_call_id,omitempty"`
	ToolName       string          `json:"tool_name,omitempty"`
	ToolInput      json.RawMessage `json:"tool_input,omitempty"`
	IsError        bool            `json:"is_error,omitempty"`
	CompactionData json.RawMessage `json:"compaction_data,omitempty"`
}

type SessionEntry struct {
	Seq             uint64         `json:"seq"`
	ID              string         `json:"id"`
	ParentID        string         `json:"parent_id,omitempty"`
	Role            string         `json:"role"`
	ModelID         string         `json:"model_id,omitempty"`
	AgentRole       string         `json:"agent_role,omitempty"`
	Blocks          []ContentBlock `json:"blocks,omitempty"`
	InputTokens     int            `json:"input_tokens,omitempty"`
	OutputTokens    int            `json:"output_tokens,omitempty"`
	CacheReadTokens int            `json:"cache_read_tokens,omitempty"`
	CostUSD         float64        `json:"cost_usd,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
}

type SessionMeta struct {
	ID              string    `toml:"id" json:"id"`
	WorkspaceID     string    `toml:"workspace_id" json:"workspace_id"`
	ParentSessionID string    `toml:"parent_session_id,omitempty" json:"parent_session_id,omitempty"`
	ForkedAtSeq     uint64    `toml:"forked_at_seq,omitempty" json:"forked_at_seq,omitempty"`
	HeadEntryID     string    `toml:"head_entry_id,omitempty" json:"head_entry_id,omitempty"`
	HeadSeq         uint64    `toml:"head_seq,omitempty" json:"head_seq,omitempty"`
	Title           string    `toml:"title" json:"title"`
	AgentRole       string    `toml:"agent_role,omitempty" json:"agent_role,omitempty"`
	ModelOverride   string    `toml:"model_override,omitempty" json:"model_override,omitempty"`
	Draft           string    `toml:"draft,omitempty" json:"draft,omitempty"`
	WorktreePath    string    `toml:"worktree_path,omitempty" json:"worktree_path,omitempty"`
	Pinned          bool      `toml:"pinned,omitempty" json:"pinned,omitempty"`
	Archived        bool      `toml:"archived,omitempty" json:"archived,omitempty"`
	TotalTokens     int       `toml:"total_tokens,omitempty" json:"total_tokens,omitempty"`
	TotalCostUSD    float64   `toml:"total_cost_usd,omitempty" json:"total_cost_usd,omitempty"`
	CreatedAt       time.Time `toml:"created_at" json:"created_at"`
	UpdatedAt       time.Time `toml:"updated_at" json:"updated_at"`
}

type sessionAuditRecord struct {
	RecordType      string    `json:"record_type"`
	SessionID       string    `json:"session_id"`
	WorkspaceID     string    `json:"workspace_id,omitempty"`
	TargetWorkspace string    `json:"target_workspace_id,omitempty"`
	ParentSessionID string    `json:"parent_session_id,omitempty"`
	ChildSessionID  string    `json:"child_session_id,omitempty"`
	Title           string    `json:"title,omitempty"`
	AgentRole       string    `json:"agent_role,omitempty"`
	FromEntryID     string    `json:"from_entry_id,omitempty"`
	ToEntryID       string    `json:"to_entry_id,omitempty"`
	FromSeq         uint64    `json:"from_seq,omitempty"`
	ToSeq           uint64    `json:"to_seq,omitempty"`
	ForkedAtSeq     uint64    `json:"forked_at_seq,omitempty"`
	RestoreFiles    bool      `json:"restore_files,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type seqStateDoc struct {
	Seq uint64 `toml:"seq"`
}

// SessionStore 管理追加式 <sessionID>.jsonl 事件树与原子替换 <sessionID>.meta.toml 元数据索引。
type SessionStore struct {
	paths config.VaultPaths
	seqMu sync.Mutex
	mu    sync.RWMutex
	metas map[string]*SessionMeta
}

func NewSessionStore(paths config.VaultPaths) (*SessionStore, error) {
	if strings.TrimSpace(paths.Root) == "" {
		return nil, fmt.Errorf("%w: empty vault root", ErrInvalidPath)
	}
	for _, dir := range []string{
		paths.WorkspacesDir(),
		paths.RuntimeDir(),
		filepath.Join(paths.RuntimeDir(), "locks"),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create store dir %s: %w", dir, err)
		}
	}

	s := &SessionStore{
		paths: paths,
		metas: make(map[string]*SessionMeta),
	}
	if err := s.loadAllSessions(); err != nil {
		return nil, err
	}
	return s, nil
}

// RunLockPath 返回供上层调度器单会话互斥运行（TryFlock）使用的文件锁路径，与内部元数据 IO 锁分离以防重入死锁。
func (s *SessionStore) RunLockPath(sessionID string) string {
	return filepath.Join(s.paths.RuntimeDir(), "locks", "session-run-"+sessionID+".lock")
}

func (s *SessionStore) sessionIOLockPath(sessionID string) string {
	return filepath.Join(s.paths.RuntimeDir(), "locks", "session-io-"+sessionID+".lock")
}

func (s *SessionStore) seqLockPath() string {
	return filepath.Join(s.paths.RuntimeDir(), "locks", "seq.lock")
}

func (s *SessionStore) seqStatePath() string {
	return filepath.Join(s.paths.RuntimeDir(), "seq.toml")
}

func (s *SessionStore) sessionsDir(wsID string) string {
	return filepath.Join(s.paths.WorkspacesDir(), wsID, "sessions")
}

func (s *SessionStore) sessionJSONLPath(wsID, sessionID string) string {
	return filepath.Join(s.sessionsDir(wsID), sessionID+".jsonl")
}

func (s *SessionStore) sessionMetaPath(wsID, sessionID string) string {
	return filepath.Join(s.sessionsDir(wsID), sessionID+".meta.toml")
}

func (s *SessionStore) NextSeq() (uint64, error) {
	s.seqMu.Lock()
	defer s.seqMu.Unlock()

	var next uint64
	err := WithFlock(s.seqLockPath(), true, func() error {
		cur, rErr := s.readPersistedSeq()
		if rErr != nil {
			return rErr
		}
		next = cur + 1
		return s.writePersistedSeq(next)
	})
	if err != nil {
		return 0, err
	}
	return next, nil
}

func (s *SessionStore) advanceSeqFloor(floor uint64) error {
	if floor == 0 {
		return nil
	}
	s.seqMu.Lock()
	defer s.seqMu.Unlock()

	return WithFlock(s.seqLockPath(), true, func() error {
		cur, rErr := s.readPersistedSeq()
		if rErr != nil {
			return rErr
		}
		if floor <= cur {
			return nil
		}
		return s.writePersistedSeq(floor)
	})
}

func (s *SessionStore) readPersistedSeq() (uint64, error) {
	raw, err := os.ReadFile(s.seqStatePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("read seq file: %w", err)
	}
	var doc seqStateDoc
	if err := toml.Unmarshal(raw, &doc); err != nil {
		return 0, fmt.Errorf("parse seq file: %w", err)
	}
	return doc.Seq, nil
}

func (s *SessionStore) writePersistedSeq(seq uint64) error {
	encoded, err := toml.Marshal(seqStateDoc{Seq: seq})
	if err != nil {
		return err
	}
	return AtomicWriteFile(s.seqStatePath(), encoded, 0o600)
}

func (s *SessionStore) loadAllSessions() error {
	wsEntries, err := os.ReadDir(s.paths.WorkspacesDir())
	if err != nil {
		return fmt.Errorf("readdir workspaces %s: %w", s.paths.WorkspacesDir(), err)
	}

	var wsIDs []string
	for _, ent := range wsEntries {
		if ent.IsDir() && validateSafeID(ent.Name()) == nil {
			wsIDs = append(wsIDs, ent.Name())
		}
	}
	if len(wsIDs) == 0 {
		return nil
	}

	type wsResult struct {
		metas []*SessionMeta
		err   error
	}
	resCh := make(chan wsResult, len(wsIDs))
	sem := make(chan struct{}, 8)

	var wg sync.WaitGroup
	for _, wsID := range wsIDs {
		wsID := wsID
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			metas, lErr := s.scanWorkspaceSessions(wsID)
			resCh <- wsResult{metas: metas, err: lErr}
		}()
	}
	wg.Wait()
	close(resCh)

	var maxSeq uint64
	for res := range resCh {
		if res.err != nil {
			return res.err
		}
		for _, m := range res.metas {
			s.metas[m.ID] = m
			if m.HeadSeq > maxSeq {
				maxSeq = m.HeadSeq
			}
		}
	}

	if maxSeq > 0 {
		if err := s.advanceSeqFloor(maxSeq); err != nil {
			return err
		}
	}
	return nil
}

func (s *SessionStore) scanWorkspaceSessions(wsID string) ([]*SessionMeta, error) {
	sessDir := s.sessionsDir(wsID)
	entries, err := os.ReadDir(sessDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("readdir %s: %w", sessDir, err)
	}

	seenIDs := make(map[string]struct{})
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		switch {
		case strings.HasSuffix(name, ".meta.toml"):
			id := strings.TrimSuffix(name, ".meta.toml")
			if validateSafeID(id) == nil {
				seenIDs[id] = struct{}{}
			}
		case strings.HasSuffix(name, ".jsonl"):
			id := strings.TrimSuffix(name, ".jsonl")
			if validateSafeID(id) == nil {
				seenIDs[id] = struct{}{}
			}
		}
	}

	var out []*SessionMeta
	for id := range seenIDs {
		metaPath := s.sessionMetaPath(wsID, id)
		rawMeta, rErr := os.ReadFile(metaPath)
		if rErr == nil {
			var m SessionMeta
			if uErr := toml.Unmarshal(rawMeta, &m); uErr != nil {
				return nil, fmt.Errorf("parse %s: %w", metaPath, uErr)
			}
			if m.ID == "" {
				m.ID = id
			}
			m.WorkspaceID = wsID
			out = append(out, &m)
			continue
		}
		if !errors.Is(rErr, os.ErrNotExist) {
			return nil, fmt.Errorf("read %s: %w", metaPath, rErr)
		}

		// Reconstruct .meta.toml if only .jsonl exists on disk.
		rebuilt, recErr := s.reconstructMetaFromJSONL(wsID, id)
		if recErr != nil {
			return nil, recErr
		}
		if rebuilt != nil {
			out = append(out, rebuilt)
		}
	}
	return out, nil
}

func (s *SessionStore) reconstructMetaFromJSONL(wsID, sessionID string) (*SessionMeta, error) {
	jsonlPath := s.sessionJSONLPath(wsID, sessionID)
	entries, audits, err := readSessionJSONLWithAudits(jsonlPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	now := time.Now().UTC()
	meta := &SessionMeta{
		ID:          sessionID,
		WorkspaceID: wsID,
		Title:       "Recovered Session",
		AgentRole:   "coder",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	byID := make(map[string]SessionEntry, len(entries))
	for _, e := range entries {
		byID[e.ID] = e
	}

	for _, ev := range audits {
		if ev.isEntry {
			meta.HeadEntryID = ev.entry.ID
			meta.HeadSeq = ev.entry.Seq
			if !ev.entry.CreatedAt.IsZero() {
				meta.UpdatedAt = ev.entry.CreatedAt
			}
			continue
		}
		switch ev.audit.RecordType {
		case "session_created":
			if ev.audit.Title != "" {
				meta.Title = ev.audit.Title
			}
			if ev.audit.AgentRole != "" {
				meta.AgentRole = ev.audit.AgentRole
			}
			if ev.audit.ParentSessionID != "" {
				meta.ParentSessionID = ev.audit.ParentSessionID
				meta.ForkedAtSeq = ev.audit.ForkedAtSeq
			}
			if !ev.audit.CreatedAt.IsZero() {
				meta.CreatedAt = ev.audit.CreatedAt
				meta.UpdatedAt = ev.audit.CreatedAt
			}
		case "rewind", "fork":
			meta.HeadEntryID = ev.audit.ToEntryID
			if ent, ok := byID[ev.audit.ToEntryID]; ok {
				meta.HeadSeq = ent.Seq
			} else {
				meta.HeadSeq = 0
			}
			if !ev.audit.CreatedAt.IsZero() {
				meta.UpdatedAt = ev.audit.CreatedAt
			}
		}
	}

	// Recompute active-branch token & cost totals from HeadEntryID back to root.
	visited := make(map[string]struct{}, len(byID))
	for curID := meta.HeadEntryID; curID != ""; {
		if _, seen := visited[curID]; seen {
			break
		}
		visited[curID] = struct{}{}
		ent, ok := byID[curID]
		if !ok {
			break
		}
		meta.TotalTokens += entryTokenSum(ent)
		meta.TotalCostUSD += ent.CostUSD
		curID = ent.ParentID
	}

	_ = s.persistMetaFile(meta)
	return meta, nil
}

func (s *SessionStore) Create(wsID, title, agentRole string) (*SessionMeta, error) {
	if err := validateSafeID(wsID); err != nil {
		return nil, fmt.Errorf("invalid workspace id: %w", err)
	}
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		cleanTitle = "New Session"
	}
	cleanRole := strings.TrimSpace(agentRole)
	if cleanRole == "" {
		cleanRole = "coder"
	}

	now := time.Now().UTC()
	sessionID := fmt.Sprintf("sess_%d_%s", now.UnixMilli(), randHex(6))
	meta := &SessionMeta{
		ID:          sessionID,
		WorkspaceID: wsID,
		Title:       cleanTitle,
		AgentRole:   cleanRole,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	err := WithFlock(s.sessionIOLockPath(sessionID), true, func() error {
		if err := os.MkdirAll(s.sessionsDir(wsID), 0o700); err != nil {
			return fmt.Errorf("create sessions dir: %w", err)
		}
		audit := sessionAuditRecord{
			RecordType:  "session_created",
			SessionID:   sessionID,
			WorkspaceID: wsID,
			Title:       cleanTitle,
			AgentRole:   cleanRole,
			CreatedAt:   now,
		}
		if err := appendJSONLine(s.sessionJSONLPath(wsID, sessionID), audit); err != nil {
			return err
		}
		return s.persistMetaFile(meta)
	})
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.metas[sessionID] = meta
	s.mu.Unlock()

	return cloneSessionMeta(meta), nil
}

func (s *SessionStore) List(wsID string, includeArchived bool) []*SessionMeta {
	s.mu.RLock()
	out := make([]*SessionMeta, 0, len(s.metas))
	for _, m := range s.metas {
		if wsID != "" && m.WorkspaceID != wsID {
			continue
		}
		if !includeArchived && m.Archived {
			continue
		}
		out = append(out, cloneSessionMeta(m))
	}
	s.mu.RUnlock()

	sort.Slice(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		if out[i].HeadSeq != out[j].HeadSeq {
			return out[i].HeadSeq > out[j].HeadSeq
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (s *SessionStore) GetMeta(id string) (*SessionMeta, error) {
	cp, ok := s.locateSession(id)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, id)
	}
	return cp, nil
}

// locateSession returns an O(1) cloned SessionMeta from the in-memory index, falling back to a single
// on-disk stat/recovery pass when another local process (e.g. CLI/ACP) created a session concurrently.
func (s *SessionStore) locateSession(id string) (*SessionMeta, bool) {
	if ValidateSafeID(id) != nil {
		return nil, false
	}
	s.mu.RLock()
	m, ok := s.metas[id]
	if ok {
		cp := cloneSessionMeta(m)
		s.mu.RUnlock()
		return cp, true
	}
	s.mu.RUnlock()

	wsEntries, err := os.ReadDir(s.paths.WorkspacesDir())
	if err != nil {
		return nil, false
	}
	for _, ent := range wsEntries {
		if !ent.IsDir() || ValidateSafeID(ent.Name()) != nil {
			continue
		}
		wsID := ent.Name()
		metaPath := s.sessionMetaPath(wsID, id)
		if raw, rErr := os.ReadFile(metaPath); rErr == nil {
			var diskMeta SessionMeta
			if toml.Unmarshal(raw, &diskMeta) == nil {
				if diskMeta.ID == "" {
					diskMeta.ID = id
				}
				diskMeta.WorkspaceID = wsID
				s.mu.Lock()
				s.metas[id] = &diskMeta
				s.mu.Unlock()
				return cloneSessionMeta(&diskMeta), true
			}
		}
		if _, stErr := os.Stat(s.sessionJSONLPath(wsID, id)); stErr == nil {
			if rebuilt, recErr := s.reconstructMetaFromJSONL(wsID, id); recErr == nil && rebuilt != nil {
				s.mu.Lock()
				s.metas[id] = rebuilt
				s.mu.Unlock()
				return cloneSessionMeta(rebuilt), true
			}
		}
	}
	return nil, false
}

func (s *SessionStore) Append(sessionID string, entry SessionEntry) (*SessionEntry, error) {
	if err := validateSafeID(sessionID); err != nil {
		return nil, err
	}

	var storedEntry *SessionEntry
	err := WithFlock(s.sessionIOLockPath(sessionID), true, func() error {
		meta, err := s.GetMeta(sessionID)
		if err != nil {
			return err
		}

		e := cloneSessionEntry(entry)
		if e.Seq == 0 {
			seq, seqErr := s.NextSeq()
			if seqErr != nil {
				return seqErr
			}
			e.Seq = seq
		} else {
			if fErr := s.advanceSeqFloor(e.Seq); fErr != nil {
				return fErr
			}
		}
		if strings.TrimSpace(e.ID) == "" {
			e.ID = fmt.Sprintf("ent_%d_%s", e.Seq, randHex(6))
		}
		if e.ParentID == "" && meta.HeadEntryID != "" {
			e.ParentID = meta.HeadEntryID
		}
		if e.CreatedAt.IsZero() {
			e.CreatedAt = time.Now().UTC()
		}

		jsonlPath := s.sessionJSONLPath(meta.WorkspaceID, sessionID)
		if err := appendJSONLine(jsonlPath, e); err != nil {
			return err
		}

		meta.HeadEntryID = e.ID
		meta.HeadSeq = e.Seq
		meta.TotalTokens += entryTokenSum(e)
		meta.TotalCostUSD += e.CostUSD
		meta.UpdatedAt = time.Now().UTC()
		if e.CreatedAt.After(meta.UpdatedAt) {
			meta.UpdatedAt = e.CreatedAt
		}

		if err := s.persistMetaFile(meta); err != nil {
			return err
		}

		s.mu.Lock()
		s.metas[sessionID] = meta
		s.mu.Unlock()

		cp := cloneSessionEntry(e)
		storedEntry = &cp
		return nil
	})
	if err != nil {
		return nil, err
	}
	return storedEntry, nil
}

// LoadActiveBranch 从 Meta.HeadEntryID 沿 ParentID 指针链回溯至根节点，实现 O(1) 头指针切换下的分支隔离读取。
func (s *SessionStore) LoadActiveBranch(sessionID string) ([]SessionEntry, error) {
	if err := validateSafeID(sessionID); err != nil {
		return nil, err
	}

	var branch []SessionEntry
	err := WithFlock(s.sessionIOLockPath(sessionID), false, func() error {
		meta, err := s.GetMeta(sessionID)
		if err != nil {
			return err
		}
		b, err := s.loadActiveBranchForMeta(meta)
		if err != nil {
			return err
		}
		branch = b
		return nil
	})
	if err != nil {
		return nil, err
	}
	return branch, nil
}

func (s *SessionStore) loadActiveBranchForMeta(meta *SessionMeta) ([]SessionEntry, error) {
	if meta.HeadEntryID == "" {
		return []SessionEntry{}, nil
	}
	entries, _, err := readSessionJSONL(s.sessionJSONLPath(meta.WorkspaceID, meta.ID))
	if err != nil {
		return nil, err
	}
	byID := make(map[string]SessionEntry, len(entries))
	for _, e := range entries {
		byID[e.ID] = e
	}

	var rev []SessionEntry
	visited := make(map[string]struct{}, len(entries))
	curID := meta.HeadEntryID
	for curID != "" {
		if _, seen := visited[curID]; seen {
			return nil, fmt.Errorf("cycle detected in session %s entry chain at %s", meta.ID, curID)
		}
		visited[curID] = struct{}{}

		ent, ok := byID[curID]
		if !ok {
			return nil, fmt.Errorf("entry %q not found in session %s", curID, meta.ID)
		}
		rev = append(rev, cloneSessionEntry(ent))
		curID = ent.ParentID
	}

	out := make([]SessionEntry, len(rev))
	for i := range rev {
		out[i] = rev[len(rev)-1-i]
	}
	return out, nil
}

func (s *SessionStore) Fork(sessionID string, atSeq uint64, newTitle string) (*SessionMeta, error) {
	if err := validateSafeID(sessionID); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	forkID := fmt.Sprintf("sess_%d_%s", now.UnixMilli(), randHex(6))

	var srcMeta *SessionMeta
	var prefix []SessionEntry
	err := WithFlock(s.sessionIOLockPath(sessionID), true, func() error {
		m, gErr := s.GetMeta(sessionID)
		if gErr != nil {
			return gErr
		}
		branch, bErr := s.loadActiveBranchForMeta(m)
		if bErr != nil {
			return bErr
		}
		for _, e := range branch {
			if e.Seq <= atSeq {
				prefix = append(prefix, cloneSessionEntry(e))
			}
		}
		srcMeta = m

		// Append fork audit record to source session log without rewriting existing entries.
		forkOutAudit := sessionAuditRecord{
			RecordType:     "fork_created",
			SessionID:      sessionID,
			ChildSessionID: forkID,
			ForkedAtSeq:    atSeq,
			CreatedAt:      now,
		}
		return appendJSONLine(s.sessionJSONLPath(m.WorkspaceID, sessionID), forkOutAudit)
	})
	if err != nil {
		return nil, err
	}

	title := strings.TrimSpace(newTitle)
	if title == "" {
		title = srcMeta.Title + " (Fork)"
	}

	var headID string
	var headSeq uint64
	var totalTokens int
	var totalCost float64
	for _, e := range prefix {
		headID = e.ID
		headSeq = e.Seq
		totalTokens += entryTokenSum(e)
		totalCost += e.CostUSD
	}

	forkMeta := &SessionMeta{
		ID:              forkID,
		WorkspaceID:     srcMeta.WorkspaceID,
		ParentSessionID: srcMeta.ID,
		ForkedAtSeq:     atSeq,
		HeadEntryID:     headID,
		HeadSeq:         headSeq,
		Title:           title,
		AgentRole:       srcMeta.AgentRole,
		ModelOverride:   srcMeta.ModelOverride,
		TotalTokens:     totalTokens,
		TotalCostUSD:    totalCost,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	err = WithFlock(s.sessionIOLockPath(forkID), true, func() error {
		var buf bytes.Buffer
		initLine, mErr := marshalJSONLine(sessionAuditRecord{
			RecordType:      "session_created",
			SessionID:       forkID,
			WorkspaceID:     forkMeta.WorkspaceID,
			ParentSessionID: srcMeta.ID,
			ForkedAtSeq:     atSeq,
			Title:           title,
			AgentRole:       forkMeta.AgentRole,
			CreatedAt:       now,
		})
		if mErr != nil {
			return mErr
		}
		buf.Write(initLine)

		for _, e := range prefix {
			line, lErr := marshalJSONLine(e)
			if lErr != nil {
				return lErr
			}
			buf.Write(line)
		}

		forkAuditLine, fErr := marshalJSONLine(sessionAuditRecord{
			RecordType:      "fork",
			SessionID:       forkID,
			ParentSessionID: srcMeta.ID,
			ForkedAtSeq:     atSeq,
			ToEntryID:       headID,
			ToSeq:           headSeq,
			CreatedAt:       now,
		})
		if fErr != nil {
			return fErr
		}
		buf.Write(forkAuditLine)

		if wErr := AtomicWriteFile(s.sessionJSONLPath(forkMeta.WorkspaceID, forkID), buf.Bytes(), 0o600); wErr != nil {
			return wErr
		}
		return s.persistMetaFile(forkMeta)
	})
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.metas[forkID] = forkMeta
	s.mu.Unlock()

	return cloneSessionMeta(forkMeta), nil
}

func (s *SessionStore) Rewind(sessionID string, toSeq uint64, restoreFiles bool, snaps *SnapshotStore, wsRoot string) (*SessionMeta, []string, error) {
	if err := validateSafeID(sessionID); err != nil {
		return nil, nil, err
	}
	if restoreFiles {
		if snaps == nil {
			return nil, nil, errors.New("rewind with restoreFiles=true requires non-nil SnapshotStore")
		}
		if strings.TrimSpace(wsRoot) == "" {
			return nil, nil, fmt.Errorf("%w: rewind with restoreFiles=true requires workspace root", ErrInvalidPath)
		}
	}

	var updatedMeta *SessionMeta
	var restoredFiles []string

	err := WithFlock(s.sessionIOLockPath(sessionID), true, func() error {
		meta, gErr := s.GetMeta(sessionID)
		if gErr != nil {
			return gErr
		}
		branch, bErr := s.loadActiveBranchForMeta(meta)
		if bErr != nil {
			return bErr
		}

		var newHeadID string
		var newHeadSeq uint64
		var retainedTokens int
		var retainedCost float64
		for _, e := range branch {
			if e.Seq <= toSeq {
				newHeadID = e.ID
				newHeadSeq = e.Seq
				retainedTokens += entryTokenSum(e)
				retainedCost += e.CostUSD
			}
		}

		if restoreFiles {
			files, rErr := snaps.RestoreSessionAfterSeq(sessionID, toSeq, wsRoot)
			if rErr != nil {
				return rErr
			}
			restoredFiles = files
		}

		now := time.Now().UTC()
		audit := sessionAuditRecord{
			RecordType:   "rewind",
			SessionID:    sessionID,
			FromEntryID:  meta.HeadEntryID,
			FromSeq:      meta.HeadSeq,
			ToEntryID:    newHeadID,
			ToSeq:        toSeq,
			RestoreFiles: restoreFiles,
			CreatedAt:    now,
		}
		if err := appendJSONLine(s.sessionJSONLPath(meta.WorkspaceID, sessionID), audit); err != nil {
			return err
		}

		meta.HeadEntryID = newHeadID
		meta.HeadSeq = newHeadSeq
		meta.TotalTokens = retainedTokens
		meta.TotalCostUSD = retainedCost
		meta.UpdatedAt = now

		if err := s.persistMetaFile(meta); err != nil {
			return err
		}

		s.mu.Lock()
		s.metas[sessionID] = meta
		s.mu.Unlock()

		updatedMeta = cloneSessionMeta(meta)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return updatedMeta, restoredFiles, nil
}

func (s *SessionStore) MoveWorkspace(sessionID, targetWsID string) (*SessionMeta, error) {
	if err := validateSafeID(sessionID); err != nil {
		return nil, err
	}
	if err := validateSafeID(targetWsID); err != nil {
		return nil, fmt.Errorf("invalid target workspace id: %w", err)
	}

	var movedMeta *SessionMeta
	err := WithFlock(s.sessionIOLockPath(sessionID), true, func() error {
		meta, gErr := s.GetMeta(sessionID)
		if gErr != nil {
			return gErr
		}
		if meta.WorkspaceID == targetWsID {
			movedMeta = cloneSessionMeta(meta)
			return nil
		}

		oldWsID := meta.WorkspaceID
		oldJSONL := s.sessionJSONLPath(oldWsID, sessionID)
		oldMeta := s.sessionMetaPath(oldWsID, sessionID)

		if err := os.MkdirAll(s.sessionsDir(targetWsID), 0o700); err != nil {
			return fmt.Errorf("create target workspace sessions dir: %w", err)
		}
		newJSONL := s.sessionJSONLPath(targetWsID, sessionID)
		if err := os.Rename(oldJSONL, newJSONL); err != nil {
			return fmt.Errorf("move session jsonl %s -> %s: %w", oldJSONL, newJSONL, err)
		}

		now := time.Now().UTC()
		_ = appendJSONLine(newJSONL, sessionAuditRecord{
			RecordType:      "move_workspace",
			SessionID:       sessionID,
			WorkspaceID:     oldWsID,
			TargetWorkspace: targetWsID,
			CreatedAt:       now,
		})

		meta.WorkspaceID = targetWsID
		meta.UpdatedAt = now
		if err := s.persistMetaFile(meta); err != nil {
			return err
		}
		_ = os.Remove(oldMeta)

		s.mu.Lock()
		s.metas[sessionID] = meta
		s.mu.Unlock()

		movedMeta = cloneSessionMeta(meta)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return movedMeta, nil
}

func (s *SessionStore) UpdateMeta(sessionID string, fn func(*SessionMeta)) (*SessionMeta, error) {
	if err := validateSafeID(sessionID); err != nil {
		return nil, err
	}

	var updated *SessionMeta
	err := WithFlock(s.sessionIOLockPath(sessionID), true, func() error {
		orig, gErr := s.GetMeta(sessionID)
		if gErr != nil {
			return gErr
		}
		cp := cloneSessionMeta(orig)
		prevUpdatedAt := cp.UpdatedAt
		if fn != nil {
			fn(cp)
		}
		// Preserve identity and workspace binding (use MoveWorkspace for cross-workspace moves).
		cp.ID = orig.ID
		cp.WorkspaceID = orig.WorkspaceID
		cp.CreatedAt = orig.CreatedAt
		if cp.UpdatedAt.Equal(prevUpdatedAt) {
			cp.UpdatedAt = time.Now().UTC()
		}

		if err := s.persistMetaFile(cp); err != nil {
			return err
		}

		s.mu.Lock()
		s.metas[sessionID] = cp
		s.mu.Unlock()

		updated = cloneSessionMeta(cp)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *SessionStore) Delete(sessionID string) error {
	if err := validateSafeID(sessionID); err != nil {
		return err
	}

	return WithFlock(s.sessionIOLockPath(sessionID), true, func() error {
		meta, gErr := s.GetMeta(sessionID)
		if gErr != nil {
			return gErr
		}
		jsonlPath := s.sessionJSONLPath(meta.WorkspaceID, sessionID)
		metaPath := s.sessionMetaPath(meta.WorkspaceID, sessionID)

		if err := os.Remove(metaPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove session meta %s: %w", metaPath, err)
		}
		if err := os.Remove(jsonlPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove session jsonl %s: %w", jsonlPath, err)
		}
		snaps := &SnapshotStore{paths: s.paths}
		_ = snaps.DeleteSessionSnapshots(meta.WorkspaceID, sessionID)

		s.mu.Lock()
		delete(s.metas, sessionID)
		s.mu.Unlock()
		return nil
	})
}

func (s *SessionStore) persistMetaFile(meta *SessionMeta) error {
	encoded, err := toml.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal session meta %s: %w", meta.ID, err)
	}
	return AtomicWriteFile(s.sessionMetaPath(meta.WorkspaceID, meta.ID), encoded, 0o600)
}

type sessionLogItem struct {
	isEntry bool
	entry   SessionEntry
	audit   sessionAuditRecord
}

func readSessionJSONL(path string) ([]SessionEntry, *sessionAuditRecord, error) {
	entries, items, err := readSessionJSONLWithAudits(path)
	if err != nil {
		return nil, nil, err
	}
	var initAudit *sessionAuditRecord
	for i := range items {
		if !items[i].isEntry && items[i].audit.RecordType == "session_created" {
			a := items[i].audit
			initAudit = &a
			break
		}
	}
	return entries, initAudit, nil
}

func readSessionJSONLWithAudits(path string) ([]SessionEntry, []sessionLogItem, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	var entries []SessionEntry
	var items []sessionLogItem
	r := bufio.NewReader(f)
	for {
		line, rErr := r.ReadBytes('\n')
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) > 0 {
			var probe struct {
				RecordType string `json:"record_type"`
				ID         string `json:"id"`
			}
			if uErr := json.Unmarshal(trimmed, &probe); uErr != nil {
				return nil, nil, fmt.Errorf("parse session jsonl %s: %w", path, uErr)
			}
			if probe.RecordType != "" {
				var a sessionAuditRecord
				if uErr := json.Unmarshal(trimmed, &a); uErr != nil {
					return nil, nil, fmt.Errorf("parse session audit in %s: %w", path, uErr)
				}
				items = append(items, sessionLogItem{audit: a})
			} else if probe.ID != "" {
				var ent SessionEntry
				if uErr := json.Unmarshal(trimmed, &ent); uErr != nil {
					return nil, nil, fmt.Errorf("parse session entry in %s: %w", path, uErr)
				}
				entries = append(entries, ent)
				items = append(items, sessionLogItem{isEntry: true, entry: ent})
			}
		}
		if errors.Is(rErr, io.EOF) {
			break
		}
		if rErr != nil {
			return nil, nil, fmt.Errorf("read session jsonl %s: %w", path, rErr)
		}
	}
	return entries, items, nil
}

func entryTokenSum(e SessionEntry) int {
	total := e.InputTokens + e.OutputTokens + e.CacheReadTokens
	if total < 0 {
		return 0
	}
	return total
}

func cloneRawMessage(in json.RawMessage) json.RawMessage {
	if in == nil {
		return nil
	}
	out := make(json.RawMessage, len(in))
	copy(out, in)
	return out
}

func cloneContentBlock(b ContentBlock) ContentBlock {
	cp := b
	cp.ToolInput = cloneRawMessage(b.ToolInput)
	cp.CompactionData = cloneRawMessage(b.CompactionData)
	return cp
}

func cloneSessionEntry(e SessionEntry) SessionEntry {
	cp := e
	if e.Blocks != nil {
		cp.Blocks = make([]ContentBlock, len(e.Blocks))
		for i, b := range e.Blocks {
			cp.Blocks[i] = cloneContentBlock(b)
		}
	}
	return cp
}

func cloneSessionMeta(m *SessionMeta) *SessionMeta {
	if m == nil {
		return nil
	}
	cp := *m
	return &cp
}
