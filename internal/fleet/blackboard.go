package fleet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"
	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/store"
)

type SubtaskSpec struct {
	ID          string `json:"id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

type missionFrontmatter struct {
	ID           string       `toml:"id"`
	WorkspaceID  string       `toml:"workspace_id"`
	WorkflowID   string       `toml:"workflow_id"`
	Topology     TopologyKind `toml:"topology"`
	Status       string       `toml:"status"`
	TotalCostUSD float64      `toml:"total_cost_usd"`
	BudgetUSD    float64      `toml:"budget_usd,omitempty"`
	CreatedAt    time.Time    `toml:"created_at"`
	UpdatedAt    time.Time    `toml:"updated_at"`
}

type noteFrontmatter struct {
	ID        string    `toml:"id"`
	Seq       uint64    `toml:"seq"`
	NodeID    string    `toml:"node_id"`
	Title     string    `toml:"title,omitempty"`
	CreatedAt time.Time `toml:"created_at"`
}

type noteSeqState struct {
	Seq uint64 `toml:"seq"`
}

// MarkdownBlackboard 管理 ~/.gorness/workspaces/<wsID>/missions/<missionID>/ 下的纯文件多 Agent 协同黑板：
// 包含 MISSION.md、tasks/<id>.md（Flock 排他抢单与深度 <=3 动态拆树）、notes/<seq>-<node>.md、diffs/<node>.diff 与 events.jsonl。
type MarkdownBlackboard struct {
	paths     config.VaultPaths
	wsID      string
	missionID string
	dir       string
	now       func() time.Time
}

func NewMarkdownBlackboard(paths config.VaultPaths, wsID, missionID string) (*MarkdownBlackboard, error) {
	return newMarkdownBlackboardWithClock(paths, wsID, missionID, time.Now)
}

func newMarkdownBlackboardWithClock(paths config.VaultPaths, wsID, missionID string, nowFn func() time.Time) (*MarkdownBlackboard, error) {
	if strings.TrimSpace(paths.Root) == "" {
		return nil, fmt.Errorf("%w: empty vault root", ErrInvalidID)
	}
	if err := ValidateID(wsID); err != nil {
		return nil, fmt.Errorf("invalid workspace id: %w", err)
	}
	if err := ValidateID(missionID); err != nil {
		return nil, fmt.Errorf("invalid mission id: %w", err)
	}
	if nowFn == nil {
		nowFn = time.Now
	}

	wsBase := paths.WorkspacesDir()
	missionDir := filepath.Join(wsBase, wsID, "missions", missionID)
	if err := ensureStrictSubpath(wsBase, missionDir); err != nil {
		return nil, err
	}

	for _, sub := range []string{
		missionDir,
		filepath.Join(missionDir, "tasks"),
		filepath.Join(missionDir, "notes"),
		filepath.Join(missionDir, "diffs"),
		filepath.Join(missionDir, "locks"),
	} {
		if err := os.MkdirAll(sub, 0o700); err != nil {
			return nil, fmt.Errorf("create blackboard dir %s: %w", sub, err)
		}
	}

	return &MarkdownBlackboard{
		paths:     paths,
		wsID:      wsID,
		missionID: missionID,
		dir:       missionDir,
		now:       nowFn,
	}, nil
}

func (b *MarkdownBlackboard) Dir() string          { return b.dir }
func (b *MarkdownBlackboard) TasksDir() string     { return filepath.Join(b.dir, "tasks") }
func (b *MarkdownBlackboard) NotesDir() string     { return filepath.Join(b.dir, "notes") }
func (b *MarkdownBlackboard) DiffsDir() string     { return filepath.Join(b.dir, "diffs") }
func (b *MarkdownBlackboard) MissionMD() string    { return filepath.Join(b.dir, "MISSION.md") }
func (b *MarkdownBlackboard) SnapshotFile() string { return filepath.Join(b.dir, "snapshot.json") }
func (b *MarkdownBlackboard) EventsFile() string   { return filepath.Join(b.dir, "events.jsonl") }

func (b *MarkdownBlackboard) taskFilePath(taskID string) (string, error) {
	if err := ValidateID(taskID); err != nil {
		return "", fmt.Errorf("invalid task id: %w", err)
	}
	p := filepath.Join(b.TasksDir(), taskID+".md")
	if err := ensureStrictSubpath(b.TasksDir(), p); err != nil {
		return "", err
	}
	return p, nil
}

func (b *MarkdownBlackboard) taskLockPath(taskID string) string {
	return filepath.Join(b.dir, "locks", "task-"+taskID+".lock")
}

func (b *MarkdownBlackboard) notesLockPath() string {
	return filepath.Join(b.dir, "locks", "notes.lock")
}

func (b *MarkdownBlackboard) eventsLockPath() string {
	return filepath.Join(b.dir, "locks", "events.lock")
}

func (b *MarkdownBlackboard) snapshotLockPath() string {
	return filepath.Join(b.dir, "locks", "snapshot.lock")
}

func (b *MarkdownBlackboard) InitMission(snap *MissionSnapshot) error {
	if snap == nil {
		return errors.New("nil MissionSnapshot")
	}
	if err := b.SaveSnapshot(snap); err != nil {
		return err
	}
	if _, stErr := os.Stat(b.EventsFile()); errors.Is(stErr, os.ErrNotExist) {
		if err := store.AtomicWriteFile(b.EventsFile(), []byte{}, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (b *MarkdownBlackboard) SaveSnapshot(snap *MissionSnapshot) error {
	if snap == nil {
		return errors.New("nil MissionSnapshot")
	}
	return store.WithFlock(b.snapshotLockPath(), true, func() error {
		fm := missionFrontmatter{
			ID:           snap.ID,
			WorkspaceID:  snap.WorkspaceID,
			WorkflowID:   snap.WorkflowID,
			Topology:     snap.Topology,
			Status:       snap.Status,
			TotalCostUSD: snap.TotalCostUSD,
			BudgetUSD:    snap.BudgetUSD,
			CreatedAt:    snap.CreatedAt,
			UpdatedAt:    snap.UpdatedAt,
		}
		headerBytes, err := toml.Marshal(fm)
		if err != nil {
			return fmt.Errorf("marshal MISSION.md frontmatter: %w", err)
		}

		var md strings.Builder
		md.WriteString("+++\n")
		md.Write(headerBytes)
		if !bytes.HasSuffix(headerBytes, []byte("\n")) {
			md.WriteByte('\n')
		}
		md.WriteString("+++\n\n# Mission Goal\n\n")
		md.WriteString(strings.TrimSpace(snap.Goal))
		md.WriteByte('\n')
		if strings.TrimSpace(snap.Summary) != "" {
			md.WriteString("\n## Summary\n\n")
			md.WriteString(strings.TrimSpace(snap.Summary))
			md.WriteByte('\n')
		}
		if strings.TrimSpace(snap.Error) != "" {
			md.WriteString("\n## Error\n\n")
			md.WriteString(strings.TrimSpace(snap.Error))
			md.WriteByte('\n')
		}
		if err := store.AtomicWriteFile(b.MissionMD(), []byte(md.String()), 0o600); err != nil {
			return err
		}

		rawJSON, err := marshalJSONNoEscape(snap)
		if err != nil {
			return fmt.Errorf("marshal snapshot.json: %w", err)
		}
		return store.AtomicWriteFile(b.SnapshotFile(), rawJSON, 0o600)
	})
}

func (b *MarkdownBlackboard) LoadSnapshot() (*MissionSnapshot, error) {
	if raw, err := os.ReadFile(b.SnapshotFile()); err == nil {
		var snap MissionSnapshot
		if uErr := json.Unmarshal(raw, &snap); uErr == nil {
			if tasks, tErr := b.ListTasks(); tErr == nil {
				snap.Tasks = tasks
			}
			if notes, nErr := b.ListNotes(); nErr == nil {
				snap.Notes = notes
			}
			return &snap, nil
		}
	}

	rawMD, err := os.ReadFile(b.MissionMD())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrMissionNotFound, b.missionID)
		}
		return nil, err
	}
	meta, body, err := config.ParseFrontmatterMD(rawMD)
	if err != nil {
		return nil, err
	}
	encoded, err := toml.Marshal(meta)
	if err != nil {
		return nil, err
	}
	var fm missionFrontmatter
	if err := toml.Unmarshal(encoded, &fm); err != nil {
		return nil, err
	}
	tasks, _ := b.ListTasks()
	notes, _ := b.ListNotes()
	return &MissionSnapshot{
		ID:           fm.ID,
		WorkspaceID:  fm.WorkspaceID,
		WorkflowID:   fm.WorkflowID,
		Goal:         strings.TrimSpace(body),
		Topology:     fm.Topology,
		Status:       fm.Status,
		Nodes:        []FleetNode{},
		Edges:        []FleetEdge{},
		Tasks:        tasks,
		Notes:        notes,
		MergeQueue:   []string{},
		TotalCostUSD: fm.TotalCostUSD,
		BudgetUSD:    fm.BudgetUSD,
		CreatedAt:    fm.CreatedAt,
		UpdatedAt:    fm.UpdatedAt,
	}, nil
}

func (b *MarkdownBlackboard) CreateTask(id, parentID, title, description string, depth int) (BlackboardTask, error) {
	if err := ValidateID(id); err != nil {
		return BlackboardTask{}, fmt.Errorf("invalid task id: %w", err)
	}
	if strings.TrimSpace(parentID) != "" {
		if err := ValidateID(parentID); err != nil {
			return BlackboardTask{}, fmt.Errorf("invalid parent task id: %w", err)
		}
	}
	if depth < 0 || depth > MaxTaskSplitDepth {
		return BlackboardTask{}, fmt.Errorf("%w: depth %d > %d", ErrMaxDepthReached, depth, MaxTaskSplitDepth)
	}
	taskPath, err := b.taskFilePath(id)
	if err != nil {
		return BlackboardTask{}, err
	}

	task := BlackboardTask{
		ID:       id,
		ParentID: parentID,
		Title:    strings.TrimSpace(title),
		Status:   TaskOpen,
		Depth:    depth,
		Version:  1,
	}
	if task.Title == "" {
		task.Title = id
	}

	err = store.WithFlock(b.taskLockPath(id), true, func() error {
		if _, stErr := os.Stat(taskPath); stErr == nil {
			return fmt.Errorf("task %q already exists", id)
		}
		return writeTaskMarkdown(taskPath, task, description)
	})
	if err != nil {
		return BlackboardTask{}, err
	}
	_ = b.AppendAuditEvent("task_created", "", id, task.Title)
	return task, nil
}

func (b *MarkdownBlackboard) GetTaskWithBody(taskID string) (BlackboardTask, string, error) {
	taskPath, err := b.taskFilePath(taskID)
	if err != nil {
		return BlackboardTask{}, "", err
	}
	return readTaskMarkdown(taskPath)
}

// ClaimTask 使用非阻塞内核级 Flock 抢占指定任务卡；仅当锁获取成功且磁盘状态为 open 时原子胜出（严格单赢家，零重复劳动）。
func (b *MarkdownBlackboard) ClaimTask(taskID, nodeID, worktreeBranch string) (*BlackboardTask, bool, error) {
	if err := ValidateID(taskID); err != nil {
		return nil, false, fmt.Errorf("invalid task id: %w", err)
	}
	if err := ValidateID(nodeID); err != nil {
		return nil, false, fmt.Errorf("invalid node id: %w", err)
	}
	taskPath, err := b.taskFilePath(taskID)
	if err != nil {
		return nil, false, err
	}

	unlock, ok, err := store.TryFlock(b.taskLockPath(taskID))
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	defer unlock()

	task, body, rErr := readTaskMarkdown(taskPath)
	if rErr != nil {
		if errors.Is(rErr, os.ErrNotExist) {
			return nil, false, fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
		}
		return nil, false, rErr
	}
	if task.Status != TaskOpen {
		return nil, false, nil
	}

	task.Status = TaskClaimed
	task.OwnerNodeID = nodeID
	if strings.TrimSpace(worktreeBranch) != "" {
		task.WorktreeBranch = strings.TrimSpace(worktreeBranch)
	}
	task.Version++
	if err := writeTaskMarkdown(taskPath, task, body); err != nil {
		return nil, false, err
	}
	_ = b.AppendAuditEvent("task_claimed", nodeID, task.ID, task.Title)
	return &task, true, nil
}

// ClaimNextOpenTask 扫描 tasks/*.md 并由并发对等节点原子抢占下一张 open 任务卡：
// 第一遍使用 TryFlock 实现无阻塞并发扇出；若遇锁竞争未抢到则第二遍使用排他 WithFlock 兜底线性确认，杜绝遗漏与重抢。
func (b *MarkdownBlackboard) ClaimNextOpenTask(nodeID, worktreeBranch string) (*BlackboardTask, bool, error) {
	if err := ValidateID(nodeID); err != nil {
		return nil, false, fmt.Errorf("invalid node id: %w", err)
	}

	ids, err := b.listTaskIDsSorted()
	if err != nil {
		return nil, false, err
	}

	var contended []string
	for _, id := range ids {
		taskPath, pErr := b.taskFilePath(id)
		if pErr != nil {
			continue
		}
		unlock, ok, lErr := store.TryFlock(b.taskLockPath(id))
		if lErr != nil {
			return nil, false, lErr
		}
		if !ok {
			contended = append(contended, id)
			continue
		}
		claimed, won, cErr := b.tryClaimUnderLock(taskPath, nodeID, worktreeBranch)
		unlock()
		if cErr != nil {
			return nil, false, cErr
		}
		if won {
			_ = b.AppendAuditEvent("task_claimed", nodeID, claimed.ID, claimed.Title)
			return claimed, true, nil
		}
	}

	for _, id := range contended {
		taskPath, pErr := b.taskFilePath(id)
		if pErr != nil {
			continue
		}
		var claimed *BlackboardTask
		var won bool
		err := store.WithFlock(b.taskLockPath(id), true, func() error {
			var cErr error
			claimed, won, cErr = b.tryClaimUnderLock(taskPath, nodeID, worktreeBranch)
			return cErr
		})
		if err != nil {
			return nil, false, err
		}
		if won {
			_ = b.AppendAuditEvent("task_claimed", nodeID, claimed.ID, claimed.Title)
			return claimed, true, nil
		}
	}

	return nil, false, nil
}

func (b *MarkdownBlackboard) tryClaimUnderLock(taskPath, nodeID, worktreeBranch string) (*BlackboardTask, bool, error) {
	task, body, err := readTaskMarkdown(taskPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if task.Status != TaskOpen {
		return nil, false, nil
	}
	task.Status = TaskClaimed
	task.OwnerNodeID = nodeID
	if strings.TrimSpace(worktreeBranch) != "" {
		task.WorktreeBranch = strings.TrimSpace(worktreeBranch)
	}
	task.Version++
	if err := writeTaskMarkdown(taskPath, task, body); err != nil {
		return nil, false, err
	}
	return &task, true, nil
}

// SplitTask 将复杂任务动态拆分为子任务卡（最大深度 MaxTaskSplitDepth = 3，超过立即返回 ErrMaxDepthReached）。
func (b *MarkdownBlackboard) SplitTask(parentTaskID, nodeID string, subtasks []SubtaskSpec) ([]BlackboardTask, error) {
	if err := ValidateID(parentTaskID); err != nil {
		return nil, fmt.Errorf("invalid parent task id: %w", err)
	}
	if err := ValidateID(nodeID); err != nil {
		return nil, fmt.Errorf("invalid node id: %w", err)
	}
	if len(subtasks) == 0 {
		return nil, errors.New("SplitTask requires at least one subtask")
	}
	parentPath, err := b.taskFilePath(parentTaskID)
	if err != nil {
		return nil, err
	}

	var created []BlackboardTask
	err = store.WithFlock(b.taskLockPath(parentTaskID), true, func() error {
		parent, parentBody, rErr := readTaskMarkdown(parentPath)
		if rErr != nil {
			if errors.Is(rErr, os.ErrNotExist) {
				return fmt.Errorf("%w: %s", ErrTaskNotFound, parentTaskID)
			}
			return rErr
		}
		if parent.Depth >= MaxTaskSplitDepth {
			return fmt.Errorf("%w: cannot split task %q at depth %d (max %d)", ErrMaxDepthReached, parentTaskID, parent.Depth, MaxTaskSplitDepth)
		}
		childDepth := parent.Depth + 1

		created = make([]BlackboardTask, 0, len(subtasks))
		for idx, spec := range subtasks {
			childID := strings.TrimSpace(spec.ID)
			if childID == "" {
				childID = fmt.Sprintf("%s.%d", parentTaskID, idx+1)
			}
			if err := ValidateID(childID); err != nil {
				return fmt.Errorf("invalid child task id %q: %w", childID, err)
			}
			childPath, pErr := b.taskFilePath(childID)
			if pErr != nil {
				return pErr
			}
			childTitle := strings.TrimSpace(spec.Title)
			if childTitle == "" {
				childTitle = childID
			}
			childTask := BlackboardTask{
				ID:       childID,
				ParentID: parent.ID,
				Title:    childTitle,
				Status:   TaskOpen,
				Depth:    childDepth,
				Version:  1,
			}
			if wErr := store.WithFlock(b.taskLockPath(childID), true, func() error {
				if _, stErr := os.Stat(childPath); stErr == nil {
					return fmt.Errorf("child task %q already exists", childID)
				}
				return writeTaskMarkdown(childPath, childTask, spec.Description)
			}); wErr != nil {
				return wErr
			}
			created = append(created, childTask)
		}

		parent.Status = TaskSplit
		parent.OwnerNodeID = nodeID
		parent.Version++
		return writeTaskMarkdown(parentPath, parent, parentBody)
	})
	if err != nil {
		return nil, err
	}

	_ = b.AppendAuditEvent("task_split", nodeID, parentTaskID, fmt.Sprintf("split into %d subtasks", len(created)))
	return created, nil
}

func (b *MarkdownBlackboard) CompleteTask(taskID, nodeID, landedCommit string) (*BlackboardTask, error) {
	if err := ValidateID(taskID); err != nil {
		return nil, fmt.Errorf("invalid task id: %w", err)
	}
	if strings.TrimSpace(nodeID) != "" {
		if err := ValidateID(nodeID); err != nil {
			return nil, fmt.Errorf("invalid node id: %w", err)
		}
	}
	taskPath, err := b.taskFilePath(taskID)
	if err != nil {
		return nil, err
	}

	var updated BlackboardTask
	err = store.WithFlock(b.taskLockPath(taskID), true, func() error {
		task, body, rErr := readTaskMarkdown(taskPath)
		if rErr != nil {
			if errors.Is(rErr, os.ErrNotExist) {
				return fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
			}
			return rErr
		}
		if strings.TrimSpace(nodeID) != "" {
			task.OwnerNodeID = nodeID
		}
		if strings.TrimSpace(landedCommit) != "" {
			task.LandedCommit = strings.TrimSpace(landedCommit)
			task.Status = TaskMerged
		} else {
			task.Status = TaskCompleted
		}
		task.Version++
		updated = task
		return writeTaskMarkdown(taskPath, task, body)
	})
	if err != nil {
		return nil, err
	}
	_ = b.AppendAuditEvent("task_completed", nodeID, taskID, updated.Status)
	return &updated, nil
}

func (b *MarkdownBlackboard) UpdateTaskStatus(taskID, status string) (*BlackboardTask, error) {
	taskPath, err := b.taskFilePath(taskID)
	if err != nil {
		return nil, err
	}
	var updated BlackboardTask
	err = store.WithFlock(b.taskLockPath(taskID), true, func() error {
		task, body, rErr := readTaskMarkdown(taskPath)
		if rErr != nil {
			return rErr
		}
		task.Status = status
		task.Version++
		updated = task
		return writeTaskMarkdown(taskPath, task, body)
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func (b *MarkdownBlackboard) ListTasks() ([]BlackboardTask, error) {
	ids, err := b.listTaskIDsSorted()
	if err != nil {
		return nil, err
	}
	tasks := make([]BlackboardTask, 0, len(ids))
	for _, id := range ids {
		p, pErr := b.taskFilePath(id)
		if pErr != nil {
			continue
		}
		t, _, rErr := readTaskMarkdown(p)
		if rErr != nil {
			continue
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (b *MarkdownBlackboard) listTaskIDsSorted() ([]string, error) {
	entries, err := os.ReadDir(b.TasksDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".md") {
			continue
		}
		id := strings.TrimSuffix(ent.Name(), ".md")
		if ValidateID(id) == nil {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func (b *MarkdownBlackboard) AddNote(nodeID, title, body string) (BlackboardNote, error) {
	if err := ValidateID(nodeID); err != nil {
		return BlackboardNote{}, fmt.Errorf("invalid node id: %w", err)
	}

	var note BlackboardNote
	err := store.WithFlock(b.notesLockPath(), true, func() error {
		seqPath := filepath.Join(b.dir, "locks", "notes-seq.toml")
		var st noteSeqState
		if raw, rErr := os.ReadFile(seqPath); rErr == nil {
			_ = toml.Unmarshal(raw, &st)
		}
		st.Seq++
		if rawSeq, mErr := toml.Marshal(st); mErr == nil {
			_ = store.AtomicWriteFile(seqPath, rawSeq, 0o600)
		}

		now := b.now().UTC()
		noteID := fmt.Sprintf("%04d-%s", st.Seq, nodeID)
		notePath := filepath.Join(b.NotesDir(), noteID+".md")
		if err := ensureStrictSubpath(b.NotesDir(), notePath); err != nil {
			return err
		}

		fm := noteFrontmatter{
			ID:        noteID,
			Seq:       st.Seq,
			NodeID:    nodeID,
			Title:     strings.TrimSpace(title),
			CreatedAt: now,
		}
		headerBytes, err := toml.Marshal(fm)
		if err != nil {
			return err
		}

		var md strings.Builder
		md.WriteString("+++\n")
		md.Write(headerBytes)
		if !bytes.HasSuffix(headerBytes, []byte("\n")) {
			md.WriteByte('\n')
		}
		md.WriteString("+++\n\n")
		md.WriteString(strings.TrimSpace(body))
		md.WriteByte('\n')

		if err := store.AtomicWriteFile(notePath, []byte(md.String()), 0o600); err != nil {
			return err
		}

		note = BlackboardNote{
			ID:        noteID,
			Seq:       st.Seq,
			NodeID:    nodeID,
			Title:     fm.Title,
			Body:      strings.TrimSpace(body),
			CreatedAt: now,
		}
		return nil
	})
	if err != nil {
		return BlackboardNote{}, err
	}
	_ = b.AppendAuditEvent("note_added", nodeID, "", note.Title)
	return note, nil
}

func (b *MarkdownBlackboard) ListNotes() ([]BlackboardNote, error) {
	entries, err := os.ReadDir(b.NotesDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var notes []BlackboardNote
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".md") {
			continue
		}
		raw, rErr := os.ReadFile(filepath.Join(b.NotesDir(), ent.Name()))
		if rErr != nil {
			continue
		}
		meta, body, pErr := config.ParseFrontmatterMD(raw)
		if pErr != nil {
			continue
		}
		encoded, mErr := toml.Marshal(meta)
		if mErr != nil {
			continue
		}
		var fm noteFrontmatter
		if uErr := toml.Unmarshal(encoded, &fm); uErr != nil {
			continue
		}
		if fm.ID == "" {
			fm.ID = strings.TrimSuffix(ent.Name(), ".md")
		}
		notes = append(notes, BlackboardNote{
			ID:        fm.ID,
			Seq:       fm.Seq,
			NodeID:    fm.NodeID,
			Title:     fm.Title,
			Body:      strings.TrimSpace(body),
			CreatedAt: fm.CreatedAt,
		})
	}
	sort.Slice(notes, func(i, j int) bool {
		if notes[i].Seq != notes[j].Seq {
			return notes[i].Seq < notes[j].Seq
		}
		return notes[i].ID < notes[j].ID
	})
	return notes, nil
}

func (b *MarkdownBlackboard) SaveNodeDiff(nodeID, diff string) error {
	if err := ValidateID(nodeID); err != nil {
		return fmt.Errorf("invalid node id: %w", err)
	}
	diffPath := filepath.Join(b.DiffsDir(), nodeID+".diff")
	if err := ensureStrictSubpath(b.DiffsDir(), diffPath); err != nil {
		return err
	}
	return store.AtomicWriteFile(diffPath, []byte(diff), 0o600)
}

func (b *MarkdownBlackboard) ReadNodeDiff(nodeID string) (string, error) {
	if err := ValidateID(nodeID); err != nil {
		return "", fmt.Errorf("invalid node id: %w", err)
	}
	diffPath := filepath.Join(b.DiffsDir(), nodeID+".diff")
	if err := ensureStrictSubpath(b.DiffsDir(), diffPath); err != nil {
		return "", err
	}
	raw, err := os.ReadFile(diffPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return string(raw), nil
}

func (b *MarkdownBlackboard) AppendEvent(ev MissionEvent) error {
	// 审计账本记录紧凑事件头，避免每行重复嵌入完整 Snapshot 导致磁盘膨胀
	compact := ev
	compact.Snapshot = nil
	return b.appendJSONLineLocked(compact)
}

func (b *MarkdownBlackboard) AppendAuditEvent(evType, nodeID, taskID, msg string) error {
	rec := map[string]any{
		"type":         evType,
		"mission_id":   b.missionID,
		"workspace_id": b.wsID,
		"node_id":      nodeID,
		"task_id":      taskID,
		"message":      msg,
		"timestamp":    b.now().UTC(),
	}
	return b.appendJSONLineLocked(rec)
}

func (b *MarkdownBlackboard) appendJSONLineLocked(v any) error {
	line, err := marshalJSONNoEscape(v)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	return store.WithFlock(b.eventsLockPath(), true, func() error {
		f, err := os.OpenFile(b.EventsFile(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return err
		}
		_, wErr := f.Write(line)
		sErr := f.Sync()
		cErr := f.Close()
		if wErr != nil {
			return wErr
		}
		if sErr != nil {
			return sErr
		}
		return cErr
	})
}

func writeTaskMarkdown(path string, task BlackboardTask, description string) error {
	headerBytes, err := toml.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal task frontmatter: %w", err)
	}
	var md strings.Builder
	md.WriteString("+++\n")
	md.Write(headerBytes)
	if !bytes.HasSuffix(headerBytes, []byte("\n")) {
		md.WriteByte('\n')
	}
	md.WriteString("+++\n\n")
	if strings.TrimSpace(description) != "" {
		md.WriteString(strings.TrimSpace(description))
		md.WriteByte('\n')
	}
	return store.AtomicWriteFile(path, []byte(md.String()), 0o600)
}

func readTaskMarkdown(path string) (BlackboardTask, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return BlackboardTask{}, "", err
	}
	meta, body, err := config.ParseFrontmatterMD(raw)
	if err != nil {
		return BlackboardTask{}, "", err
	}
	encoded, err := toml.Marshal(meta)
	if err != nil {
		return BlackboardTask{}, "", err
	}
	var task BlackboardTask
	if err := toml.Unmarshal(encoded, &task); err != nil {
		return BlackboardTask{}, "", err
	}
	return task, strings.TrimSpace(body), nil
}

func marshalJSONNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	out := buf.Bytes()
	if len(out) > 0 && out[len(out)-1] == '\n' {
		out = out[:len(out)-1]
	}
	return out, nil
}

// ConvergenceGuard 追踪多 Agent 响应式黑板轮次的任务进展：当连续无实质任务状态推进轮数达到 max_idle_rounds（或总轮次达上限）时平稳收敛停机，阻断无限响应式互唤醒循环。
type ConvergenceGuard struct {
	mu            sync.Mutex
	maxIdleRounds int
	maxRounds     int
	rounds        int
	idleRounds    int
}

func NewConvergenceGuard(maxIdleRounds, maxRounds int) *ConvergenceGuard {
	if maxIdleRounds <= 0 {
		maxIdleRounds = DefaultMaxIdle
	}
	return &ConvergenceGuard{
		maxIdleRounds: maxIdleRounds,
		maxRounds:     maxRounds,
	}
}

func (g *ConvergenceGuard) ObserveRound(progressed bool) (halt bool, reason string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.rounds++
	if progressed {
		g.idleRounds = 0
	} else {
		g.idleRounds++
	}

	if g.idleRounds >= g.maxIdleRounds {
		return true, fmt.Sprintf("max_idle_rounds reached (%d)", g.maxIdleRounds)
	}
	if g.maxRounds > 0 && g.rounds >= g.maxRounds {
		return true, fmt.Sprintf("max_rounds reached (%d)", g.maxRounds)
	}
	return false, ""
}

func (g *ConvergenceGuard) IdleRounds() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.idleRounds
}

func (g *ConvergenceGuard) Rounds() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.rounds
}

type WitnessOptions struct {
	StallTimeout  time.Duration
	CheckInterval time.Duration
	Now           func() time.Time
	OnStall       func(nodeID string, silentFor time.Duration)
}

type watchedNode struct {
	lastBeat time.Time
	alerted  bool
}

// Witness 监控活跃运行节点的流式心跳，发现超出 StallTimeout 无响应的僵死节点时及时告警。
type Witness struct {
	stallTimeout  time.Duration
	checkInterval time.Duration
	now           func() time.Time
	onStall       func(nodeID string, silentFor time.Duration)

	mu      sync.Mutex
	watched map[string]*watchedNode
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func NewWitness(opts WitnessOptions) *Witness {
	stall := opts.StallTimeout
	if stall <= 0 {
		stall = 90 * time.Second
	}
	interval := opts.CheckInterval
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	return &Witness{
		stallTimeout:  stall,
		checkInterval: interval,
		now:           nowFn,
		onStall:       opts.OnStall,
		watched:       make(map[string]*watchedNode),
	}
}

func (w *Witness) WatchNode(nodeID string) {
	if w == nil || strings.TrimSpace(nodeID) == "" {
		return
	}
	w.mu.Lock()
	w.watched[nodeID] = &watchedNode{lastBeat: w.now()}
	w.mu.Unlock()
}

func (w *Witness) Beat(nodeID string) {
	if w == nil || strings.TrimSpace(nodeID) == "" {
		return
	}
	w.mu.Lock()
	if wn, ok := w.watched[nodeID]; ok {
		wn.lastBeat = w.now()
		wn.alerted = false
	} else {
		w.watched[nodeID] = &watchedNode{lastBeat: w.now()}
	}
	w.mu.Unlock()
}

func (w *Witness) UnwatchNode(nodeID string) {
	if w == nil {
		return
	}
	w.mu.Lock()
	delete(w.watched, nodeID)
	w.mu.Unlock()
}

func (w *Witness) CheckOnce() {
	if w == nil {
		return
	}
	now := w.now()
	type stalledItem struct {
		nodeID string
		silent time.Duration
	}
	var stalled []stalledItem

	w.mu.Lock()
	for id, wn := range w.watched {
		silent := now.Sub(wn.lastBeat)
		if silent >= w.stallTimeout && !wn.alerted {
			wn.alerted = true
			stalled = append(stalled, stalledItem{nodeID: id, silent: silent})
		}
	}
	w.mu.Unlock()

	if w.onStall != nil {
		for _, item := range stalled {
			w.onStall(item.nodeID, item.silent)
		}
	}
}

func (w *Witness) Start(parent context.Context) {
	if w == nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	w.mu.Lock()
	if w.cancel != nil {
		w.cancel()
	}
	w.cancel = cancel
	w.mu.Unlock()

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ticker := time.NewTicker(w.checkInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.CheckOnce()
			}
		}
	}()
}

func (w *Witness) Stop() {
	if w == nil {
		return
	}
	w.mu.Lock()
	cancel := w.cancel
	w.cancel = nil
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	w.wg.Wait()
}
