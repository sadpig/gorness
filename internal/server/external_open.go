package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/store"
	"gorness.dev/gorness/internal/tools"
)

const (
	maxSafeOpenCopyBytes = 64 * 1024 * 1024 // 64 MiB
	openCopyRetention    = 24 * time.Hour
)

type OpenAction string

const (
	OpenActionCopiedReadOnly OpenAction = "open_readonly_copy"
	OpenActionRevealInFinder OpenAction = "reveal_in_finder"
)

var (
	ErrForbiddenOpenPath = errors.New("target path is outside allowed workspace/artifact root or points into protected secrets")
	ErrTOCTOUDetected    = errors.New("file changed or swapped during safe open verification")
)

type OpenerFunc func(ctx context.Context, args []string) error

type OpenResult struct {
	Action     OpenAction `json:"action"`
	SourcePath string     `json:"source_path"`
	OpenedPath string     `json:"opened_path"`
	Mode       string     `json:"mode,omitempty"`
}

var allowlistedSafeDocExts = map[string]bool{
	".md":   true,
	".txt":  true,
	".pdf":  true,
	".png":  true,
	".jpg":  true,
	".svg":  true,
	".csv":  true,
	".toml": true,
	".log":  true,
}

// ExternalOpener 安全打开外部文件：
//  1. 解析规范路径并阻断逃逸出工作区或进入 ~/.gorness 机密区的符号链接；
//  2. 白名单文档扩展名通过 O_NOFOLLOW 打开文件描述符，核验 f.Stat() 与 os.Lstat() 的 Dev/Ino 一致（防软链掉包 TOCTOU），
//     复制为 0400 只读副本放入 <vault>/runtime/open-copies/（并自动清理 >24h 历史副本）后再交由 Opener 打开；
//  3. 非白名单文件（脚本、可执行包等）绝不直接启动，一律转为 Finder 定位展示（open -R）。
type ExternalOpener struct {
	paths  config.VaultPaths
	opener OpenerFunc
	now    func() time.Time
}

func NewExternalOpener(paths config.VaultPaths, opener OpenerFunc) *ExternalOpener {
	return NewExternalOpenerWithClock(paths, opener, time.Now)
}

func NewExternalOpenerWithClock(paths config.VaultPaths, opener OpenerFunc, nowFn func() time.Time) *ExternalOpener {
	if opener == nil {
		opener = defaultMacOSOpener
	}
	if nowFn == nil {
		nowFn = time.Now
	}
	o := &ExternalOpener{
		paths:  paths,
		opener: opener,
		now:    nowFn,
	}
	_ = o.PruneExpiredCopies()
	return o
}

func defaultMacOSOpener(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, "/usr/bin/open", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("macos open %v failed: %w (%s)", args, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// PruneExpiredCopies 清理 <vault>/runtime/open-copies/ 中超过 24 小时的只读副本。
func (o *ExternalOpener) PruneExpiredCopies() int {
	if o == nil || strings.TrimSpace(o.paths.Root) == "" {
		return 0
	}
	copiesDir := filepath.Join(o.paths.RuntimeDir(), "open-copies")
	st, err := os.Lstat(copiesDir)
	if err != nil || st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
		return 0
	}
	return pruneOpenCopiesDir(copiesDir, o.now().UTC())
}

func pruneOpenCopiesDir(copiesDir string, now time.Time) int {
	entries, err := os.ReadDir(copiesDir)
	if err != nil {
		return 0
	}
	cutoff := now.Add(-openCopyRetention)
	removed := 0
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		info, iErr := ent.Info()
		if iErr != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			full := filepath.Join(copiesDir, ent.Name())
			_ = os.Chmod(full, 0o600)
			if os.Remove(full) == nil {
				removed++
			}
		}
	}
	return removed
}

func (o *ExternalOpener) Open(ctx context.Context, wsRoot, rawTarget string) (OpenResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	trimmed := strings.TrimSpace(rawTarget)
	if trimmed == "" || strings.ContainsRune(trimmed, 0) {
		return OpenResult{}, fmt.Errorf("%w: empty or invalid path", ErrForbiddenOpenPath)
	}

	_ = o.PruneExpiredCopies()

	canonTarget, err := o.resolveAllowedTarget(wsRoot, trimmed)
	if err != nil {
		return OpenResult{}, err
	}

	lst, err := os.Lstat(canonTarget)
	if err != nil {
		return OpenResult{}, err
	}
	if !lst.Mode().IsRegular() {
		return OpenResult{}, fmt.Errorf("%w: target %s is not a regular file", ErrForbiddenOpenPath, canonTarget)
	}
	lstatSys, ok := lst.Sys().(*syscall.Stat_t)
	if !ok || lstatSys == nil {
		return OpenResult{}, errors.New("unable to inspect lstat inode metadata")
	}

	ext := strings.ToLower(filepath.Ext(canonTarget))
	if !allowlistedSafeDocExts[ext] {
		if err := o.opener(ctx, []string{"-R", canonTarget}); err != nil {
			return OpenResult{}, err
		}
		return OpenResult{
			Action:     OpenActionRevealInFinder,
			SourcePath: canonTarget,
			OpenedPath: canonTarget,
		}, nil
	}

	// 使用 O_NOFOLLOW | O_CLOEXEC 打开规范路径并在同一 FD 上二次比对 Dev/Ino，彻底阻断 Lstat -> Open 间的符号链接掉包攻击。
	fd, err := syscall.Open(canonTarget, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return OpenResult{}, fmt.Errorf("%w: symlink swap blocked", ErrTOCTOUDetected)
		}
		return OpenResult{}, fmt.Errorf("open %s: %w", canonTarget, err)
	}
	f := os.NewFile(uintptr(fd), canonTarget)
	defer f.Close()

	fst, err := f.Stat()
	if err != nil {
		return OpenResult{}, fmt.Errorf("fstat %s: %w", canonTarget, err)
	}
	if !fst.Mode().IsRegular() {
		return OpenResult{}, fmt.Errorf("%w: open fd is not a regular file", ErrTOCTOUDetected)
	}
	fstatSys, ok := fst.Sys().(*syscall.Stat_t)
	if !ok || fstatSys == nil || fstatSys.Dev != lstatSys.Dev || fstatSys.Ino != lstatSys.Ino {
		return OpenResult{}, fmt.Errorf("%w: inode/device mismatch between lstat and open fd", ErrTOCTOUDetected)
	}

	data, err := io.ReadAll(io.LimitReader(f, maxSafeOpenCopyBytes+1))
	if err != nil {
		return OpenResult{}, fmt.Errorf("read verified fd %s: %w", canonTarget, err)
	}
	if int64(len(data)) > maxSafeOpenCopyBytes {
		return OpenResult{}, fmt.Errorf("file exceeds %d MiB safe copy limit", maxSafeOpenCopyBytes/(1024*1024))
	}

	copiesDir, err := o.ensureSafeCopiesDir()
	if err != nil {
		return OpenResult{}, err
	}
	pruneOpenCopiesDir(copiesDir, o.now().UTC())

	sumInput := append([]byte(canonTarget+"\x00"), data...)
	sum := sha256.Sum256(sumInput)
	digest := hex.EncodeToString(sum[:6])

	stem := sanitizeCopyStem(strings.TrimSuffix(filepath.Base(canonTarget), filepath.Ext(canonTarget)))
	destPath := filepath.Join(copiesDir, fmt.Sprintf("%s-%s%s", digest, stem, ext))

	if err := writeReadOnly0400Copy(destPath, data); err != nil {
		return OpenResult{}, err
	}

	if err := o.opener(ctx, []string{destPath}); err != nil {
		return OpenResult{}, err
	}

	return OpenResult{
		Action:     OpenActionCopiedReadOnly,
		SourcePath: canonTarget,
		OpenedPath: destPath,
		Mode:       "0400",
	}, nil
}

func (o *ExternalOpener) resolveAllowedTarget(wsRoot, target string) (string, error) {
	canonVault := ""
	if strings.TrimSpace(o.paths.Root) != "" {
		if v, err := config.CanonicalPath(o.paths.Root); err == nil && v != "" {
			canonVault = v
		} else {
			canonVault = filepath.Clean(o.paths.Root)
		}
	}

	if strings.TrimSpace(wsRoot) != "" {
		pg, err := tools.NewPathGuard(wsRoot, o.paths.Root)
		if err != nil {
			return "", err
		}
		canonAbs, _, inWS, rErr := pg.ResolveInWorkspace(target)
		if rErr != nil {
			if errors.Is(rErr, tools.ErrVaultSecretBlocked) {
				if resolved, eErr := config.CanonicalPath(target); eErr == nil {
					if isSafeVaultArtifactPath(canonVault, resolved) {
						return resolved, nil
					}
				}
			}
			return "", fmt.Errorf("%w: %v", ErrForbiddenOpenPath, rErr)
		}
		if inWS || pg.IsOverflowLogFile(canonAbs) || isSafeVaultArtifactPath(canonVault, canonAbs) {
			return canonAbs, nil
		}
		return "", fmt.Errorf("%w: %s escapes workspace %s", ErrForbiddenOpenPath, target, pg.WorkspaceRoot())
	}

	// 无工作区上下文时仅允许打开 ~/.gorness/workspaces/<wsID>/artifacts/* 或 runtime/logs/overflow-*.log
	resolved, err := config.CanonicalPath(target)
	if err != nil {
		return "", err
	}
	if _, stErr := os.Lstat(resolved); stErr != nil {
		return "", stErr
	}
	if isSafeVaultArtifactPath(canonVault, resolved) || isSafeVaultOverflowLog(canonVault, resolved) {
		return resolved, nil
	}
	return "", fmt.Errorf("%w: workspace_id is required to open non-artifact path %s", ErrForbiddenOpenPath, target)
}

func isSafeVaultArtifactPath(canonVault, absPath string) bool {
	if canonVault == "" || absPath == "" {
		return false
	}
	wsRoot := filepath.Join(canonVault, "workspaces")
	clean := filepath.Clean(absPath)
	if !config.IsStrictlyWithinRoot(clean, wsRoot) {
		return false
	}
	rel, err := filepath.Rel(wsRoot, clean)
	if err != nil {
		return false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) != 3 || parts[1] != "artifacts" {
		return false
	}
	if store.ValidateSafeID(parts[0]) != nil {
		return false
	}
	if parts[2] == "" || parts[2] == "." || parts[2] == ".." || strings.ContainsAny(parts[2], `/\`) {
		return false
	}
	return true
}

func isSafeVaultOverflowLog(canonVault, absPath string) bool {
	if canonVault == "" || absPath == "" {
		return false
	}
	logsDir := filepath.Join(canonVault, "runtime", "logs")
	clean := filepath.Clean(absPath)
	if !config.IsStrictlyWithinRoot(clean, logsDir) {
		return false
	}
	base := filepath.Base(clean)
	return filepath.Dir(clean) == logsDir && strings.HasPrefix(base, "overflow-") && strings.HasSuffix(base, ".log")
}

func (o *ExternalOpener) ensureSafeCopiesDir() (string, error) {
	copiesDir := filepath.Join(o.paths.RuntimeDir(), "open-copies")
	if err := os.MkdirAll(copiesDir, 0o700); err != nil {
		return "", fmt.Errorf("create runtime/open-copies: %w", err)
	}
	st, err := os.Lstat(copiesDir)
	if err != nil {
		return "", err
	}
	if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
		return "", fmt.Errorf("%w: runtime/open-copies is not a safe directory", ErrTOCTOUDetected)
	}
	_ = os.Chmod(copiesDir, 0o700)
	return copiesDir, nil
}

func writeReadOnly0400Copy(destPath string, data []byte) error {
	if st, err := os.Lstat(destPath); err == nil {
		if st.Mode()&os.ModeSymlink != 0 || st.IsDir() {
			return fmt.Errorf("%w: refusing to replace non-regular open copy %s", ErrTOCTOUDetected, destPath)
		}
		_ = os.Chmod(destPath, 0o600)
		_ = os.Remove(destPath)
	}
	if err := store.AtomicWriteFile(destPath, data, 0o400); err != nil {
		return err
	}
	return os.Chmod(destPath, 0o400)
}

func sanitizeCopyStem(stem string) string {
	var b strings.Builder
	for _, r := range stem {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
		if b.Len() >= 48 {
			break
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return "document"
	}
	return out
}
