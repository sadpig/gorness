package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorness.dev/gorness/internal/tools"
)

const SettleWindow = 500 * time.Millisecond

var (
	ErrApprovalNotSettled = errors.New("approval card has not settled for 500ms")
	ErrApprovalNotFound   = errors.New("pending approval not found")

	approvalSeq atomic.Uint64
)

type PendingApproval struct {
	ID          string           `json:"id"`
	RunID       string           `json:"run_id"`
	WorkspaceID string           `json:"workspace_id"`
	SessionID   string           `json:"session_id"`
	ToolCallID  string           `json:"tool_call_id,omitempty"`
	ToolName    string           `json:"tool_name"`
	ReasonCode  string           `json:"reason_code"`
	Reason      string           `json:"reason"`
	Fingerprint string           `json:"fingerprint"`
	Args        json.RawMessage  `json:"args"`
	Hunks       []tools.DiffHunk `json:"hunks,omitempty"`
	SurfacedAt  time.Time        `json:"surfaced_at"`
}

type ApprovalAction string

const (
	ApprovalActionApproveAll         ApprovalAction = "approve_all"
	ApprovalActionApproveHunks       ApprovalAction = "approve_hunks"
	ApprovalActionApproveAndRemember ApprovalAction = "approve_remember"
	ApprovalActionDeny               ApprovalAction = "deny"
)

type ApprovalDecision struct {
	Action          ApprovalAction `json:"action"`
	SelectedHunkIDs []string       `json:"selected_hunk_ids"`
	Remember        bool           `json:"remember,omitempty"`
	Feedback        string         `json:"feedback,omitempty"`
}

func ApproveAll() ApprovalDecision {
	return ApprovalDecision{Action: ApprovalActionApproveAll}
}

func ApproveHunks(hunkIDs []string) ApprovalDecision {
	ids := make([]string, len(hunkIDs))
	copy(ids, hunkIDs)
	return ApprovalDecision{
		Action:          ApprovalActionApproveHunks,
		SelectedHunkIDs: ids,
	}
}

func ApproveAndRemember() ApprovalDecision {
	return ApprovalDecision{
		Action:   ApprovalActionApproveAndRemember,
		Remember: true,
	}
}

func Deny(feedback ...string) ApprovalDecision {
	var msg string
	if len(feedback) > 0 {
		msg = strings.TrimSpace(feedback[0])
	}
	return ApprovalDecision{
		Action:   ApprovalActionDeny,
		Feedback: msg,
	}
}

type pendingApprovalSlot struct {
	approval PendingApproval
	ch       chan ApprovalDecision
}

// ApprovalBroker 统一托管跨项目待批卡片队列，强制执行 500ms 视口防误触冷静锁（SettleWindow），
// 支持全部批准、逐 DiffHunk 勾选部分批准、记住内容指纹授权（持久化至 grants.toml）与拒绝。
type ApprovalBroker struct {
	mu      sync.RWMutex
	grants  *GrantStore
	now     func() time.Time
	pending map[string]*pendingApprovalSlot
}

func NewApprovalBroker(grants *GrantStore, nowFn func() time.Time) *ApprovalBroker {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &ApprovalBroker{
		grants:  grants,
		now:     nowFn,
		pending: make(map[string]*pendingApprovalSlot),
	}
}

func (b *ApprovalBroker) SetClock(nowFn func() time.Time) {
	if b == nil || nowFn == nil {
		return
	}
	b.mu.Lock()
	b.now = nowFn
	b.mu.Unlock()
}

func (b *ApprovalBroker) currentTime() time.Time {
	b.mu.RLock()
	fn := b.now
	b.mu.RUnlock()
	if fn == nil {
		return time.Now()
	}
	return fn()
}

func (b *ApprovalBroker) Submit(req PendingApproval) (PendingApproval, <-chan ApprovalDecision) {
	now := b.currentTime()
	cp := clonePendingApproval(req)
	if strings.TrimSpace(cp.ID) == "" {
		cp.ID = fmt.Sprintf("appr_%d_%s", approvalSeq.Add(1), randomShortHex(4))
	}
	if cp.SurfacedAt.IsZero() {
		cp.SurfacedAt = now
	}
	if len(cp.Args) == 0 {
		cp.Args = json.RawMessage(`{}`)
	}

	ch := make(chan ApprovalDecision, 1)
	b.mu.Lock()
	b.pending[cp.ID] = &pendingApprovalSlot{
		approval: cp,
		ch:       ch,
	}
	b.mu.Unlock()

	return clonePendingApproval(cp), ch
}

// Resolve 裁决指定待批卡片：若距 SurfacedAt 不足 500ms，返回 ErrApprovalNotSettled 并保持卡片挂起态不变。
func (b *ApprovalBroker) Resolve(id string, decision ApprovalDecision) error {
	if b == nil {
		return ErrApprovalNotFound
	}
	cleanID := strings.TrimSpace(id)

	b.mu.Lock()
	slot, ok := b.pending[cleanID]
	if !ok {
		b.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrApprovalNotFound, cleanID)
	}

	nowFn := b.now
	if nowFn == nil {
		nowFn = time.Now
	}
	elapsed := nowFn().Sub(slot.approval.SurfacedAt)
	if elapsed < SettleWindow {
		b.mu.Unlock()
		return ErrApprovalNotSettled
	}

	// 归一化部分块批准切片：确保即使传入 nil，ApproveHunks 语义也保留非 nil 空切片以驱动 written=false
	norm := decision
	switch norm.Action {
	case ApprovalActionApproveHunks:
		if norm.SelectedHunkIDs == nil {
			norm.SelectedHunkIDs = make([]string, 0)
		} else {
			cp := make([]string, len(norm.SelectedHunkIDs))
			copy(cp, norm.SelectedHunkIDs)
			norm.SelectedHunkIDs = cp
		}
	case ApprovalActionApproveAndRemember:
		norm.Remember = true
	case "":
		norm.Action = ApprovalActionApproveAll
	}

	approvedMeta := slot.approval
	ch := slot.ch
	delete(b.pending, cleanID)
	grants := b.grants
	b.mu.Unlock()

	if norm.Action != ApprovalActionDeny && norm.Remember && grants != nil && approvedMeta.ReasonCode != "always-ask" {
		_ = grants.SaveGrant(
			approvedMeta.WorkspaceID,
			approvedMeta.Fingerprint,
			approvedMeta.ToolName,
			approvedMeta.Reason,
		)
	}

	ch <- norm
	close(ch)
	return nil
}

func (b *ApprovalBroker) ApproveAll(id string) error {
	return b.Resolve(id, ApproveAll())
}

func (b *ApprovalBroker) ApproveHunks(id string, hunkIDs []string) error {
	return b.Resolve(id, ApproveHunks(hunkIDs))
}

func (b *ApprovalBroker) ApproveAndRemember(id string) error {
	return b.Resolve(id, ApproveAndRemember())
}

func (b *ApprovalBroker) Deny(id string, feedback ...string) error {
	return b.Resolve(id, Deny(feedback...))
}

func (b *ApprovalBroker) GetPending(id string) (PendingApproval, bool) {
	if b == nil {
		return PendingApproval{}, false
	}
	b.mu.RLock()
	slot, ok := b.pending[strings.TrimSpace(id)]
	var out PendingApproval
	if ok {
		out = clonePendingApproval(slot.approval)
	}
	b.mu.RUnlock()
	return out, ok
}

func (b *ApprovalBroker) ListPending(wsID string) []PendingApproval {
	if b == nil {
		return nil
	}
	filterWS := strings.TrimSpace(wsID)

	b.mu.RLock()
	out := make([]PendingApproval, 0, len(b.pending))
	for _, slot := range b.pending {
		if filterWS != "" && slot.approval.WorkspaceID != filterWS {
			continue
		}
		out = append(out, clonePendingApproval(slot.approval))
	}
	b.mu.RUnlock()

	sort.Slice(out, func(i, j int) bool {
		if !out[i].SurfacedAt.Equal(out[j].SurfacedAt) {
			return out[i].SurfacedAt.Before(out[j].SurfacedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (b *ApprovalBroker) Cancel(id string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	slot, ok := b.pending[strings.TrimSpace(id)]
	if ok {
		delete(b.pending, strings.TrimSpace(id))
		close(slot.ch)
	}
	b.mu.Unlock()
}

func (b *ApprovalBroker) CancelRun(runID string) {
	if b == nil || strings.TrimSpace(runID) == "" {
		return
	}
	targetRun := strings.TrimSpace(runID)
	b.mu.Lock()
	for id, slot := range b.pending {
		if slot.approval.RunID == targetRun {
			delete(b.pending, id)
			close(slot.ch)
		}
	}
	b.mu.Unlock()
}

func clonePendingApproval(in PendingApproval) PendingApproval {
	cp := in
	if in.Args != nil {
		cp.Args = append(json.RawMessage(nil), in.Args...)
	}
	if in.Hunks != nil {
		cp.Hunks = make([]tools.DiffHunk, len(in.Hunks))
		copy(cp.Hunks, in.Hunks)
	}
	return cp
}

func randomShortHex(nBytes int) string {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%08x", approvalSeq.Add(1))
	}
	return hex.EncodeToString(buf)
}
