package fleet

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gorness.dev/gorness/internal/config"
)

const (
	fallbackGitUserName  = "Gorness Fleet"
	fallbackGitUserEmail = "fleet@gorness.local"
)

type WorktreeLease struct {
	MissionID  string `json:"mission_id"`
	NodeID     string `json:"node_id"`
	RepoRoot   string `json:"repo_root"`
	Path       string `json:"path"`
	Branch     string `json:"branch"`
	BaseBranch string `json:"base_branch"`
}

// WorktreePool 管理 ~/.gorness/runtime/worktrees/<missionID>/<nodeID> 隔离 Git 工作树的创建、未提交改动落盘提交、差异统计与清理回收。
type WorktreePool struct {
	paths config.VaultPaths
	mu    sync.Mutex // 串行化极快的 git worktree add/remove/prune 元数据操作，消除 .git/config.lock 并发冲突
}

func NewWorktreePool(paths config.VaultPaths) *WorktreePool {
	return &WorktreePool{paths: paths}
}

func (p *WorktreePool) DetectDefaultBranch(ctx context.Context, repoRoot string) (string, error) {
	out, err := runGitCmd(ctx, repoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("detect repo branch in %s: %w", repoRoot, err)
	}
	branch := strings.TrimSpace(out)
	if branch == "" || branch == "HEAD" {
		sha, sErr := runGitCmd(ctx, repoRoot, "rev-parse", "HEAD")
		if sErr != nil {
			return "", fmt.Errorf("detect repo HEAD in %s: %w", repoRoot, sErr)
		}
		return strings.TrimSpace(sha), nil
	}
	return branch, nil
}

func (p *WorktreePool) IsGitRepo(ctx context.Context, repoRoot string) bool {
	if strings.TrimSpace(repoRoot) == "" {
		return false
	}
	out, err := runGitCmd(ctx, repoRoot, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

func (p *WorktreePool) Acquire(ctx context.Context, repoRoot, missionID, nodeID, baseBranch string) (*WorktreeLease, error) {
	if err := ValidateID(missionID); err != nil {
		return nil, fmt.Errorf("invalid mission id: %w", err)
	}
	if err := ValidateID(nodeID); err != nil {
		return nil, fmt.Errorf("invalid node id: %w", err)
	}
	if strings.TrimSpace(repoRoot) == "" {
		return nil, errors.New("empty git repository root")
	}

	canonRepo := canonicalizeDir(repoRoot)
	wtRoot := canonicalizeDir(p.paths.WorktreesDir())
	if err := os.MkdirAll(wtRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create worktrees root %s: %w", wtRoot, err)
	}
	wtRoot = canonicalizeDir(wtRoot)

	targetPath := filepath.Join(wtRoot, missionID, nodeID)
	if err := ensureStrictSubpath(wtRoot, targetPath); err != nil {
		return nil, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	resolvedBase := strings.TrimSpace(baseBranch)
	if resolvedBase == "" {
		var err error
		resolvedBase, err = p.DetectDefaultBranch(ctx, canonRepo)
		if err != nil {
			return nil, err
		}
	}

	branch := fmt.Sprintf("gorness/fleet/%s/%s", sanitizeGitRefSegment(missionID), sanitizeGitRefSegment(nodeID))

	// 清理可能由异常中断遗留的同路径工作树
	_, _ = runGitCmd(ctx, canonRepo, "worktree", "prune")
	if _, err := os.Lstat(targetPath); err == nil {
		_, _ = runGitCmd(ctx, canonRepo, "worktree", "remove", "--force", targetPath)
		_ = os.RemoveAll(targetPath)
		_, _ = runGitCmd(ctx, canonRepo, "worktree", "prune")
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
		return nil, fmt.Errorf("create mission worktree parent: %w", err)
	}

	if _, err := runGitCmd(ctx, canonRepo, "worktree", "add", "-B", branch, targetPath, resolvedBase); err != nil {
		return nil, fmt.Errorf("git worktree add %s (%s): %w", targetPath, branch, err)
	}

	canonWT := canonicalizeDir(targetPath)
	return &WorktreeLease{
		MissionID:  missionID,
		NodeID:     nodeID,
		RepoRoot:   canonRepo,
		Path:       canonWT,
		Branch:     branch,
		BaseBranch: resolvedBase,
	}, nil
}

// CommitPending 将 Gorness write_file / edit_file 工具写入隔离工作树的未暂存与新增文件统一暂存并提交，
// 附带显式 -c user.name / user.email / commit.gpgsign=false 回退配置，确保无全局 Git 配置的纯净环境零失败。
func (p *WorktreePool) CommitPending(ctx context.Context, wtPath, message string) (committed bool, headSHA string, err error) {
	if strings.TrimSpace(wtPath) == "" {
		return false, "", errors.New("empty worktree path")
	}
	statusOut, err := runGitCmd(ctx, wtPath, "status", "--porcelain")
	if err != nil {
		return false, "", fmt.Errorf("git status in %s: %w", wtPath, err)
	}

	if strings.TrimSpace(statusOut) != "" {
		if _, err := runGitCmd(ctx, wtPath, "add", "-A"); err != nil {
			return false, "", fmt.Errorf("git add -A in %s: %w", wtPath, err)
		}
		hasStaged, sErr := hasStagedChanges(ctx, wtPath)
		if sErr != nil {
			return false, "", sErr
		}
		if hasStaged {
			msg := strings.TrimSpace(message)
			if msg == "" {
				msg = "chore(fleet): commit agent worktree changes"
			}
			if _, err := runGitWithIdentity(ctx, wtPath, "commit", "-m", msg); err != nil {
				return false, "", fmt.Errorf("git commit in %s: %w", wtPath, err)
			}
			committed = true
		}
	}

	shaOut, err := runGitCmd(ctx, wtPath, "rev-parse", "HEAD")
	if err != nil {
		return committed, "", fmt.Errorf("git rev-parse HEAD in %s: %w", wtPath, err)
	}
	return committed, strings.TrimSpace(shaOut), nil
}

func (p *WorktreePool) DiffStat(ctx context.Context, wtPath, baseRef string) (string, error) {
	if strings.TrimSpace(wtPath) == "" {
		return "", nil
	}
	args := []string{"diff", "--stat"}
	if strings.TrimSpace(baseRef) != "" {
		args = append(args, strings.TrimSpace(baseRef))
	}
	statOut, err := runGitCmd(ctx, wtPath, args...)
	if err != nil {
		return "", err
	}
	untracked, _ := listUntrackedFiles(ctx, wtPath)
	var parts []string
	if strings.TrimSpace(statOut) != "" {
		parts = append(parts, strings.TrimSpace(statOut))
	}
	if len(untracked) > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked file(s): %s", len(untracked), strings.Join(untracked, ", ")))
	}
	return strings.Join(parts, "\n"), nil
}

func (p *WorktreePool) Diff(ctx context.Context, wtPath, baseRef string) (string, error) {
	if strings.TrimSpace(wtPath) == "" {
		return "", nil
	}
	args := []string{"diff"}
	if strings.TrimSpace(baseRef) != "" {
		args = append(args, strings.TrimSpace(baseRef))
	}
	trackedDiff, err := runGitCmd(ctx, wtPath, args...)
	if err != nil {
		return "", err
	}

	untracked, _ := listUntrackedFiles(ctx, wtPath)
	if len(untracked) == 0 {
		return trackedDiff, nil
	}

	var buf strings.Builder
	if strings.TrimSpace(trackedDiff) != "" {
		buf.WriteString(trackedDiff)
		if !strings.HasSuffix(trackedDiff, "\n") {
			buf.WriteByte('\n')
		}
	}
	for _, rel := range untracked {
		appendUntrackedFileDiff(&buf, wtPath, rel)
	}
	return buf.String(), nil
}

func appendUntrackedFileDiff(buf *strings.Builder, wtPath, rel string) {
	cleanRel := filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(rel), "./"))
	if cleanRel == "" || cleanRel == "." || cleanRel == ".." ||
		strings.HasPrefix(cleanRel, "../") || strings.Contains(cleanRel, "/../") {
		return
	}
	fullPath := filepath.Join(wtPath, filepath.FromSlash(cleanRel))
	if err := ensureStrictSubpath(wtPath, fullPath); err != nil {
		return
	}

	// 逐级 Lstat 校验中间目录，严禁穿过工作树内的符号链接目录跳出沙箱
	segs := strings.Split(cleanRel, "/")
	cur := wtPath
	for i := 0; i < len(segs)-1; i++ {
		cur = filepath.Join(cur, segs[i])
		dirSt, err := os.Lstat(cur)
		if err != nil || dirSt.Mode()&os.ModeSymlink != 0 || !dirSt.IsDir() {
			return
		}
	}

	st, err := os.Lstat(fullPath)
	if err != nil {
		return
	}
	if st.Mode()&os.ModeSymlink != 0 {
		// 仅读取符号链接字面指向（Readlink），绝不跟随打开目标文件，防止窃取宿主机机密（如 ~/.ssh/id_rsa）
		linkTarget, lErr := os.Readlink(fullPath)
		if lErr != nil {
			return
		}
		fmt.Fprintf(
			buf,
			"diff --git a/%s b/%s\nnew file mode 120000\n--- /dev/null\n+++ b/%s\n@@ -0,0 +1,1 @@\n+%s\n\\ No newline at end of file\n",
			cleanRel, cleanRel, cleanRel, linkTarget,
		)
		return
	}
	if !st.Mode().IsRegular() {
		return
	}

	raw, rErr := os.ReadFile(fullPath)
	if rErr != nil {
		return
	}
	if len(raw) == 0 {
		fmt.Fprintf(buf, "diff --git a/%s b/%s\nnew file mode 100644\n", cleanRel, cleanRel)
		return
	}
	if bytes.IndexByte(raw, 0) >= 0 {
		fmt.Fprintf(buf, "diff --git a/%s b/%s\nnew file mode 100644\nBinary files /dev/null and b/%s differ\n", cleanRel, cleanRel, cleanRel)
		return
	}

	text := strings.TrimSuffix(string(raw), "\n")
	lines := strings.Split(text, "\n")
	fmt.Fprintf(buf, "diff --git a/%s b/%s\nnew file mode 100644\n--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n", cleanRel, cleanRel, cleanRel, len(lines))
	for _, l := range lines {
		buf.WriteByte('+')
		buf.WriteString(l)
		buf.WriteByte('\n')
	}
	if !bytes.HasSuffix(raw, []byte("\n")) {
		buf.WriteString("\\ No newline at end of file\n")
	}
}

func (p *WorktreePool) Release(ctx context.Context, lease *WorktreeLease, deleteBranch bool) error {
	if lease == nil {
		return nil
	}
	if err := ValidateID(lease.MissionID); err != nil {
		return err
	}
	if err := ValidateID(lease.NodeID); err != nil {
		return err
	}

	wtRoot := canonicalizeDir(p.paths.WorktreesDir())
	if lease.Path != "" {
		if err := ensureStrictSubpath(wtRoot, lease.Path); err != nil {
			return err
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if lease.RepoRoot != "" && lease.Path != "" {
		_, _ = runGitCmd(ctx, lease.RepoRoot, "worktree", "remove", "--force", lease.Path)
	}
	if lease.Path != "" {
		_ = os.RemoveAll(lease.Path)
	}
	if lease.RepoRoot != "" {
		_, _ = runGitCmd(ctx, lease.RepoRoot, "worktree", "prune")
		if deleteBranch && strings.TrimSpace(lease.Branch) != "" {
			_, _ = runGitCmd(ctx, lease.RepoRoot, "branch", "-D", lease.Branch)
		}
	}

	missionDir := filepath.Join(wtRoot, lease.MissionID)
	if entries, err := os.ReadDir(missionDir); err == nil && len(entries) == 0 {
		_ = os.Remove(missionDir)
	}
	return nil
}

func (p *WorktreePool) CleanupMission(ctx context.Context, repoRoot, missionID string) error {
	if err := ValidateID(missionID); err != nil {
		return err
	}
	wtRoot := canonicalizeDir(p.paths.WorktreesDir())
	missionDir := filepath.Join(wtRoot, missionID)
	if err := ensureStrictSubpath(wtRoot, missionDir); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	entries, err := os.ReadDir(missionDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if repoRoot != "" {
				_, _ = runGitCmd(ctx, repoRoot, "worktree", "prune")
			}
			return nil
		}
		return err
	}

	for _, ent := range entries {
		nodePath := filepath.Join(missionDir, ent.Name())
		if repoRoot != "" {
			_, _ = runGitCmd(ctx, repoRoot, "worktree", "remove", "--force", nodePath)
		}
		_ = os.RemoveAll(nodePath)
		if repoRoot != "" && ValidateID(ent.Name()) == nil {
			branch := fmt.Sprintf("gorness/fleet/%s/%s", sanitizeGitRefSegment(missionID), sanitizeGitRefSegment(ent.Name()))
			_, _ = runGitCmd(ctx, repoRoot, "branch", "-D", branch)
		}
	}
	_ = os.RemoveAll(missionDir)
	if repoRoot != "" {
		_, _ = runGitCmd(ctx, repoRoot, "worktree", "prune")
	}
	return nil
}

func listUntrackedFiles(ctx context.Context, wtPath string) ([]string, error) {
	out, err := runGitCmd(ctx, wtPath, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			files = append(files, trimmed)
		}
	}
	sort.Strings(files)
	return files, nil
}

func hasStagedChanges(ctx context.Context, wtPath string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, "git", "-C", wtPath, "diff", "--cached", "--quiet")
	cmd.Env = cleanGitEnv()
	err := cmd.Run()
	if err == nil {
		return false, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return true, nil
	}
	return false, fmt.Errorf("git diff --cached --quiet: %w", err)
}

func runGitWithIdentity(ctx context.Context, dir string, args ...string) (string, error) {
	fullArgs := []string{
		"-c", "user.name=" + fallbackGitUserName,
		"-c", "user.email=" + fallbackGitUserEmail,
		"-c", "commit.gpgsign=false",
	}
	fullArgs = append(fullArgs, args...)
	return runGitCmd(ctx, dir, fullArgs...)
}

func runGitCmd(ctx context.Context, dir string, args ...string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmdArgs := make([]string, 0, len(args)+2)
	if strings.TrimSpace(dir) != "" {
		cmdArgs = append(cmdArgs, "-C", dir)
	}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Env = cleanGitEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText == "" {
			errText = strings.TrimSpace(stdout.String())
		}
		return stdout.String(), fmt.Errorf("git %s: %w (%s)", strings.Join(args, " "), err, errText)
	}
	return stdout.String(), nil
}

func cleanGitEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GIT_DIR=") ||
			strings.HasPrefix(kv, "GIT_WORK_TREE=") ||
			strings.HasPrefix(kv, "GIT_INDEX_FILE=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true")
	return env
}

func sanitizeGitRefSegment(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	res := strings.Trim(b.String(), "-.")
	if res == "" {
		return "node"
	}
	return res
}

func canonicalizeDir(p string) string {
	trimmed := strings.TrimSpace(p)
	if trimmed == "" {
		return ""
	}
	if abs, err := filepath.Abs(trimmed); err == nil {
		trimmed = abs
	}
	if resolved, err := filepath.EvalSymlinks(trimmed); err == nil {
		return filepath.Clean(resolved)
	}
	// 若末级尚不存在，尝试规范化已存在的父目录（消除 macOS /var -> /private/var 偏差）
	parent := filepath.Dir(trimmed)
	if resolvedParent, err := filepath.EvalSymlinks(parent); err == nil {
		return filepath.Join(filepath.Clean(resolvedParent), filepath.Base(trimmed))
	}
	return filepath.Clean(trimmed)
}
