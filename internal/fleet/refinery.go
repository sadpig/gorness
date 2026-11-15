package fleet

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"gorness.dev/gorness/internal/tools"
)

type VerifyRunner func(ctx context.Context, cmdStr, workDir string) (tools.CommandResult, error)

type BlindReviewFunc func(ctx context.Context, diff, verifyOutput string) (accepted bool, feedback string, err error)

type LandRequest struct {
	Lease          *WorktreeLease
	TargetBranch   string
	CommitMessage  string
	VerifyCommand  string
	SandboxMode    tools.SandboxMode
	AllowedDomains []string
	BlindReview    BlindReviewFunc
	OnQueueEnter   func(item string)
	OnQueueExit    func(item string)
}

type LandResult struct {
	Accepted     bool   `json:"accepted"`
	Stage        string `json:"stage"`
	LandedCommit string `json:"landed_commit,omitempty"`
	Diff         string `json:"diff,omitempty"`
	DiffStat     string `json:"diff_stat,omitempty"`
	VerifyOutput string `json:"verify_output,omitempty"`
	ReviewNotes  string `json:"review_notes,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

// Refinery 实现多 Agent 隔离分支串行合入门控：
// 自动归集未提交文件 -> 变基至最新目标分支 -> Seatbelt 沙箱验收命令门控 -> 异构 Verifier 盲审 -> 纯快进（--ff-only）合入主干。
type Refinery struct {
	pool         *WorktreePool
	seatbelt     *tools.SeatbeltRunner
	verifyRunner VerifyRunner
	mu           sync.Mutex
}

func NewRefinery(pool *WorktreePool, seatbelt *tools.SeatbeltRunner, verifyRunner VerifyRunner) *Refinery {
	if seatbelt == nil {
		seatbelt = tools.NewSeatbeltRunner()
	}
	return &Refinery{
		pool:         pool,
		seatbelt:     seatbelt,
		verifyRunner: verifyRunner,
	}
}

func (r *Refinery) RunVerify(
	ctx context.Context,
	cmdStr, workDir string,
	mode tools.SandboxMode,
	allowedDomains []string,
) (tools.CommandResult, error) {
	trimmed := strings.TrimSpace(cmdStr)
	if trimmed == "" {
		return tools.CommandResult{ExitCode: 0}, nil
	}
	if r.verifyRunner != nil {
		return r.verifyRunner(ctx, trimmed, workDir)
	}
	sbMode := mode
	if sbMode == "" {
		sbMode = tools.SandboxStrict
	}
	return r.seatbelt.Run(ctx, trimmed, workDir, sbMode, allowedDomains)
}

// VerifyInPlace 在未启用独立 Git Worktree（UseWorktrees=false 或非 Git 目录）时，
// 通过与 Land 共享的互斥锁严格串行执行验收命令与 Verifier 盲审，杜绝多并发节点争抢同一 Verifier 会话（ErrSessionBusy）。
func (r *Refinery) VerifyInPlace(ctx context.Context, nodeID, workDir string, isGitRepo bool, req LandRequest) (*LandResult, error) {
	queueItem := strings.TrimSpace(nodeID)
	if req.OnQueueEnter != nil && queueItem != "" {
		req.OnQueueEnter(queueItem)
	}
	if req.OnQueueExit != nil && queueItem != "" {
		defer req.OnQueueExit(queueItem)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var diffText, diffStat string
	if isGitRepo && r.pool != nil && strings.TrimSpace(workDir) != "" {
		diffText, _ = r.pool.Diff(ctx, workDir, "")
		diffStat, _ = r.pool.DiffStat(ctx, workDir, "")
	}

	var verifyOut string
	if strings.TrimSpace(req.VerifyCommand) != "" {
		vRes, vErr := r.RunVerify(ctx, req.VerifyCommand, workDir, req.SandboxMode, req.AllowedDomains)
		verifyOut = strings.TrimSpace(vRes.Combined)
		if vErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return &LandResult{
				Accepted:     false,
				Stage:        "verify",
				Diff:         diffText,
				DiffStat:     diffStat,
				VerifyOutput: verifyOut,
				Reason:       fmt.Sprintf("verify_command %q execution error: %v", req.VerifyCommand, vErr),
			}, nil
		}
		if vRes.ExitCode != 0 {
			return &LandResult{
				Accepted:     false,
				Stage:        "verify",
				Diff:         diffText,
				DiffStat:     diffStat,
				VerifyOutput: verifyOut,
				Reason:       fmt.Sprintf("verify_command %q exited with code %d:\n%s", req.VerifyCommand, vRes.ExitCode, verifyOut),
			}, nil
		}
	}

	var reviewNotes string
	if req.BlindReview != nil {
		accepted, feedback, bErr := req.BlindReview(ctx, diffText, verifyOut)
		if bErr != nil {
			return nil, bErr
		}
		reviewNotes = strings.TrimSpace(feedback)
		if !accepted {
			return &LandResult{
				Accepted:     false,
				Stage:        "verifier",
				Diff:         diffText,
				DiffStat:     diffStat,
				VerifyOutput: verifyOut,
				ReviewNotes:  reviewNotes,
				Reason:       reviewNotes,
			}, nil
		}
	}

	return &LandResult{
		Accepted:     true,
		Stage:        "landed",
		Diff:         diffText,
		DiffStat:     diffStat,
		VerifyOutput: verifyOut,
		ReviewNotes:  reviewNotes,
	}, nil
}

func (r *Refinery) Land(ctx context.Context, req LandRequest) (*LandResult, error) {
	if req.Lease == nil {
		return nil, errors.New("refinery: nil WorktreeLease")
	}
	lease := req.Lease
	targetBranch := strings.TrimSpace(req.TargetBranch)
	if targetBranch == "" {
		targetBranch = strings.TrimSpace(lease.BaseBranch)
	}
	if targetBranch == "" {
		var err error
		targetBranch, err = r.pool.DetectDefaultBranch(ctx, lease.RepoRoot)
		if err != nil {
			return nil, err
		}
	}

	queueItem := lease.NodeID
	if queueItem == "" {
		queueItem = lease.Branch
	}
	if req.OnQueueEnter != nil {
		req.OnQueueEnter(queueItem)
	}
	if req.OnQueueExit != nil {
		defer req.OnQueueExit(queueItem)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// 1. 将隔离工作树中由 write_file / edit_file 产生的所有未暂存改动统一暂存并提交
	commitMsg := strings.TrimSpace(req.CommitMessage)
	if commitMsg == "" {
		commitMsg = fmt.Sprintf("feat(fleet): %s changes for mission %s", lease.NodeID, lease.MissionID)
	}
	if _, _, err := r.pool.CommitPending(ctx, lease.Path, commitMsg); err != nil {
		return &LandResult{
			Accepted: false,
			Stage:    "commit",
			Reason:   err.Error(),
		}, err
	}

	// 2. 串行变基至最新目标分支（保证后续快进合入线性无分叉）
	aheadCount, err := countCommitsAhead(ctx, lease.Path, targetBranch)
	if err != nil {
		return nil, err
	}
	if aheadCount > 0 {
		if _, rbErr := runGitWithIdentity(ctx, lease.Path, "rebase", targetBranch); rbErr != nil {
			_, _ = runGitCmd(ctx, lease.Path, "rebase", "--abort")
			return &LandResult{
				Accepted: false,
				Stage:    "rebase",
				Reason:   fmt.Sprintf("rebase onto %s failed: %v", targetBranch, rbErr),
			}, nil
		}
	}

	diffText, _ := runGitCmd(ctx, lease.Path, "diff", targetBranch+"..HEAD")
	diffStat, _ := runGitCmd(ctx, lease.Path, "diff", "--stat", targetBranch+"..HEAD")
	diffStat = strings.TrimSpace(diffStat)

	// 3. 执行自动化验收命令门控（如 go test ./...）
	var verifyOut string
	if strings.TrimSpace(req.VerifyCommand) != "" {
		vRes, vErr := r.RunVerify(ctx, req.VerifyCommand, lease.Path, req.SandboxMode, req.AllowedDomains)
		verifyOut = strings.TrimSpace(vRes.Combined)
		if vErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return &LandResult{
				Accepted:     false,
				Stage:        "verify",
				Diff:         diffText,
				DiffStat:     diffStat,
				VerifyOutput: verifyOut,
				Reason:       fmt.Sprintf("verify_command %q execution error: %v", req.VerifyCommand, vErr),
			}, nil
		}
		if vRes.ExitCode != 0 {
			return &LandResult{
				Accepted:     false,
				Stage:        "verify",
				Diff:         diffText,
				DiffStat:     diffStat,
				VerifyOutput: verifyOut,
				Reason:       fmt.Sprintf("verify_command %q exited with code %d:\n%s", req.VerifyCommand, vRes.ExitCode, verifyOut),
			}, nil
		}
	}

	// 4. 异构模型 Verifier 独立盲审（仅传入真实 git diff 与自动化测试输出，隔离编码者主观自评）
	var reviewNotes string
	if req.BlindReview != nil {
		accepted, feedback, bErr := req.BlindReview(ctx, diffText, verifyOut)
		if bErr != nil {
			return nil, bErr
		}
		reviewNotes = strings.TrimSpace(feedback)
		if !accepted {
			return &LandResult{
				Accepted:     false,
				Stage:        "verifier",
				Diff:         diffText,
				DiffStat:     diffStat,
				VerifyOutput: verifyOut,
				ReviewNotes:  reviewNotes,
				Reason:       reviewNotes,
			}, nil
		}
	}

	// 5. 纯快进合入目标分支（--ff-only 零合并提交）
	if aheadCount > 0 {
		curRepoBranch, _ := r.pool.DetectDefaultBranch(ctx, lease.RepoRoot)
		if curRepoBranch == targetBranch {
			if _, mErr := runGitCmd(ctx, lease.RepoRoot, "merge", "--ff-only", lease.Branch); mErr != nil {
				return &LandResult{
					Accepted:     false,
					Stage:        "merge",
					Diff:         diffText,
					DiffStat:     diffStat,
					VerifyOutput: verifyOut,
					ReviewNotes:  reviewNotes,
					Reason:       fmt.Sprintf("fast-forward merge %s -> %s failed: %v", lease.Branch, targetBranch, mErr),
				}, mErr
			}
		} else {
			if _, fErr := runGitCmd(ctx, lease.RepoRoot, "fetch", ".", lease.Branch+":"+targetBranch); fErr != nil {
				return &LandResult{
					Accepted:     false,
					Stage:        "merge",
					Diff:         diffText,
					DiffStat:     diffStat,
					VerifyOutput: verifyOut,
					ReviewNotes:  reviewNotes,
					Reason:       fmt.Sprintf("fast-forward ref update %s -> %s failed: %v", lease.Branch, targetBranch, fErr),
				}, fErr
			}
		}
	}

	landedSHAOut, err := runGitCmd(ctx, lease.RepoRoot, "rev-parse", targetBranch)
	if err != nil {
		return nil, err
	}

	return &LandResult{
		Accepted:     true,
		Stage:        "landed",
		LandedCommit: strings.TrimSpace(landedSHAOut),
		Diff:         diffText,
		DiffStat:     diffStat,
		VerifyOutput: verifyOut,
		ReviewNotes:  reviewNotes,
	}, nil
}

func countCommitsAhead(ctx context.Context, wtPath, baseBranch string) (int, error) {
	out, err := runGitCmd(ctx, wtPath, "rev-list", "--count", baseBranch+"..HEAD")
	if err != nil {
		return 0, fmt.Errorf("count commits ahead of %s: %w", baseBranch, err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, err
	}
	return n, nil
}
