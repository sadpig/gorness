package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gorness.dev/gorness/internal/config"
)

var (
	ErrVaultSecretBlocked   = errors.New("access to gorness vault secret path is blocked")
	ErrPathEscapesWorkspace = errors.New("path resolves outside workspace root")
	ErrInvalidTargetPath    = errors.New("invalid target path")
)

// PathGuard 负责规范化解析 macOS 软链路径（如 /var -> /private/var、/tmp -> /private/tmp），
// 严格区分项目边界内外路径，并无条件阻断对 ~/.gorness/ 敏感凭据目录的直接或软链穿透访问。
type PathGuard struct {
	canonWs         string
	canonVault      string
	canonHomeVault  string
	userHome        string
	wsInWorktreeDir bool
}

func NewPathGuard(wsRoot, vaultRoot string) (*PathGuard, error) {
	if strings.TrimSpace(wsRoot) == "" {
		return nil, fmt.Errorf("%w: empty workspace root", ErrInvalidTargetPath)
	}
	rawWs, err := toUncleanedAbs("", wsRoot)
	if err != nil {
		return nil, fmt.Errorf("abs workspace root %s: %w", wsRoot, err)
	}
	canonWs, err := wrapPathSecErr(config.CanonicalPath(rawWs))
	if err != nil {
		return nil, fmt.Errorf("canonicalize workspace root %s: %w", wsRoot, err)
	}
	st, err := os.Stat(canonWs)
	if err != nil || !st.IsDir() {
		return nil, fmt.Errorf("%w: workspace root %s is not a directory", ErrInvalidTargetPath, canonWs)
	}

	var canonVault string
	if strings.TrimSpace(vaultRoot) != "" {
		rawVault, err := toUncleanedAbs("", vaultRoot)
		if err != nil {
			return nil, fmt.Errorf("abs vault root %s: %w", vaultRoot, err)
		}
		canonVault, err = wrapPathSecErr(config.CanonicalPath(rawVault))
		if err != nil {
			return nil, fmt.Errorf("canonicalize vault root %s: %w", vaultRoot, err)
		}
	}

	wsInWorktree := false
	if canonVault != "" {
		wtBase := filepath.Join(canonVault, "runtime", "worktrees")
		if config.IsStrictlyWithinRoot(canonWs, wtBase) {
			wsInWorktree = true
		} else if config.IsWithinRoot(canonWs, canonVault) || config.IsWithinRoot(canonVault, canonWs) {
			return nil, fmt.Errorf("%w: workspace %s overlaps vault %s", ErrVaultSecretBlocked, canonWs, canonVault)
		}
	}

	homeDir, _ := os.UserHomeDir()
	var canonHome string
	var canonHomeVault string
	if homeDir != "" {
		if resolvedHome, err := config.CanonicalPath(homeDir); err == nil {
			canonHome = resolvedHome
		} else {
			canonHome = filepath.Clean(homeDir)
		}
		hv := canonHome + "/.gorness"
		if resolvedHV, err := config.CanonicalPath(hv); err == nil {
			canonHomeVault = resolvedHV
		} else {
			canonHomeVault = filepath.Clean(hv)
		}
	}

	return &PathGuard{
		canonWs:         canonWs,
		canonVault:      canonVault,
		canonHomeVault:  canonHomeVault,
		userHome:        canonHome,
		wsInWorktreeDir: wsInWorktree,
	}, nil
}

func (p *PathGuard) WorkspaceRoot() string {
	if p == nil {
		return ""
	}
	return p.canonWs
}

func (p *PathGuard) VaultRoot() string {
	if p == nil {
		return ""
	}
	return p.canonVault
}

// ResolveInWorkspace 解析目标路径（含未创建的新增子文件、悬空软链与父目录软链），
// 返回规范绝对路径、工作区内相对路径、是否处于工作区内，以及是否命中禁止访问的 Vault 机密区。
func (p *PathGuard) ResolveInWorkspace(target string) (canonicalAbs, rel string, inWorkspace bool, err error) {
	if p == nil {
		return "", "", false, fmt.Errorf("%w: nil PathGuard", ErrInvalidTargetPath)
	}
	trimmed := strings.TrimSpace(target)
	if trimmed == "" || strings.ContainsRune(trimmed, 0) {
		return "", "", false, fmt.Errorf("%w: empty or NUL-containing path", ErrInvalidTargetPath)
	}

	normSlash := strings.ReplaceAll(trimmed, "\\", "/")
	if (normSlash == "~/.gorness" || strings.HasPrefix(normSlash, "~/.gorness/")) && p.userHome == "" {
		return "", "", false, fmt.Errorf("%w: %s", ErrVaultSecretBlocked, target)
	}

	// 注意：委托 config.ExpandHome 与 config.CanonicalPathTracking 按 POSIX 语义逐段 Lstat/Readlink，
	// 严禁在软链解析前词法折叠 ".."。
	expanded := config.ExpandHome(normSlash, p.userHome)
	candidateRaw, err := toUncleanedAbs(p.canonWs, expanded)
	if err != nil {
		return "", "", false, err
	}

	canonicalAbs, crossedWsLink, rawErr := config.CanonicalPathTracking(candidateRaw, p.canonWs)
	if rawErr != nil {
		return "", "", false, wrapOnlyErr(rawErr)
	}

	inWS := config.IsWithinRoot(canonicalAbs, p.canonWs)

	// 检查是否命中 ~/.gorness 机密目录：
	// - 若工作区本身是 runtime/worktrees/<id>，仅放行该 worktree 内部路径；
	// - 对直接寻址（未经过工作区内恶意软链跳板）的 runtime/logs/overflow-<hex>.log 普通文件，返回 inWorkspace=false, err=nil 以允许只读分段查看。
	allowOverflow := !crossedWsLink && p.IsOverflowLogFile(canonicalAbs)
	if p.canonVault != "" && config.IsWithinRoot(canonicalAbs, p.canonVault) {
		if !(p.wsInWorktreeDir && inWS) && !allowOverflow {
			return canonicalAbs, "", false, fmt.Errorf("%w: %s", ErrVaultSecretBlocked, target)
		}
	}
	if p.canonHomeVault != "" && config.IsWithinRoot(canonicalAbs, p.canonHomeVault) {
		if !inWS && !allowOverflow {
			return canonicalAbs, "", false, fmt.Errorf("%w: %s", ErrVaultSecretBlocked, target)
		}
	}

	if !inWS {
		return canonicalAbs, "", false, nil
	}

	relOS, rErr := filepath.Rel(p.canonWs, canonicalAbs)
	if rErr != nil {
		return canonicalAbs, "", false, rErr
	}
	return canonicalAbs, filepath.ToSlash(relOS), true, nil
}

// IsOverflowLogFile 判断规范路径是否为 Vault runtime/logs/ 下可寻址的非软链普通文件 overflow-<hex>.log。
func (p *PathGuard) IsOverflowLogFile(canonicalAbs string) bool {
	if p == nil || canonicalAbs == "" {
		return false
	}
	base := filepath.Base(canonicalAbs)
	if !isValidOverflowLogFilename(base) {
		return false
	}
	parent := filepath.Dir(canonicalAbs)
	matchesLogsDir := false
	if p.canonVault != "" && parent == filepath.Join(p.canonVault, "runtime", "logs") {
		matchesLogsDir = true
	}
	if p.canonHomeVault != "" && parent == filepath.Join(p.canonHomeVault, "runtime", "logs") {
		matchesLogsDir = true
	}
	if !matchesLogsDir {
		return false
	}
	if dirSt, err := os.Lstat(parent); err != nil || dirSt.Mode()&os.ModeSymlink != 0 || !dirSt.IsDir() {
		return false
	}
	if rtSt, err := os.Lstat(filepath.Dir(parent)); err != nil || rtSt.Mode()&os.ModeSymlink != 0 || !rtSt.IsDir() {
		return false
	}
	st, err := os.Lstat(canonicalAbs)
	if err != nil || st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() {
		return false
	}
	return true
}

func isValidOverflowLogFilename(name string) bool {
	if !strings.HasPrefix(name, "overflow-") || !strings.HasSuffix(name, ".log") {
		return false
	}
	hexPart := strings.TrimSuffix(strings.TrimPrefix(name, "overflow-"), ".log")
	if len(hexPart) < 12 || len(hexPart) > 64 {
		return false
	}
	for i := 0; i < len(hexPart); i++ {
		c := hexPart[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// IsAutoRunConfigPath 委托 config.IsProtectedAutoRunPath 判断目标路径是否属于自执行/敏感工程配置。
func IsAutoRunConfigPath(canonicalAbs, rel string) bool {
	if strings.TrimSpace(rel) != "" {
		if _, hit := config.IsProtectedAutoRunPath(rel, "", ""); hit {
			return true
		}
	}
	if strings.TrimSpace(canonicalAbs) != "" {
		if _, hit := config.IsProtectedAutoRunPath(canonicalAbs, "", ""); hit {
			return true
		}
	}
	return false
}

// toUncleanedAbs 在不调用 filepath.Clean / filepath.Join 的前提下构造绝对路径串，保留原始路径段顺序供 POSIX 软链遍历。
func toUncleanedAbs(baseDir, raw string) (string, error) {
	slash := strings.ReplaceAll(raw, "\\", "/")
	if strings.HasPrefix(slash, "/") {
		return slash, nil
	}
	base := baseDir
	if base == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		if resolvedWD, eErr := filepath.EvalSymlinks(wd); eErr == nil {
			base = resolvedWD
		} else {
			base = wd
		}
	}
	base = strings.TrimRight(strings.ReplaceAll(base, "\\", "/"), "/")
	if base == "" {
		return "/" + slash, nil
	}
	return base + "/" + slash, nil
}

func wrapPathSecErr(canon string, err error) (string, error) {
	if err != nil {
		return "", wrapOnlyErr(err)
	}
	return canon, nil
}

func wrapOnlyErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, config.ErrSymlinkDepthExceeded) || strings.Contains(err.Error(), "non-directory component") || strings.Contains(err.Error(), "empty path") {
		return fmt.Errorf("%w: %v", ErrInvalidTargetPath, err)
	}
	return err
}
