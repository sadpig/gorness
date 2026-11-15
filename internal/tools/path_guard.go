package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxSymlinkHops = 40

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
	absWs, err := filepath.Abs(wsRoot)
	if err != nil {
		return nil, fmt.Errorf("abs workspace root %s: %w", wsRoot, err)
	}
	canonWs, err := filepath.EvalSymlinks(absWs)
	if err != nil {
		return nil, fmt.Errorf("canonicalize workspace root %s: %w", wsRoot, err)
	}
	canonWs = filepath.Clean(canonWs)
	st, err := os.Stat(canonWs)
	if err != nil || !st.IsDir() {
		return nil, fmt.Errorf("%w: workspace root %s is not a directory", ErrInvalidTargetPath, canonWs)
	}

	var canonVault string
	if strings.TrimSpace(vaultRoot) != "" {
		absVault, err := filepath.Abs(vaultRoot)
		if err != nil {
			return nil, fmt.Errorf("abs vault root %s: %w", vaultRoot, err)
		}
		canonVault, err = resolveCanonicalPath(absVault, 0)
		if err != nil {
			return nil, fmt.Errorf("canonicalize vault root %s: %w", vaultRoot, err)
		}
	}

	wsInWorktree := false
	if canonVault != "" {
		wtBase := filepath.Join(canonVault, "runtime", "worktrees")
		if isStrictlyWithinPath(canonWs, wtBase) {
			wsInWorktree = true
		} else if isWithinOrEqualPath(canonWs, canonVault) || isWithinOrEqualPath(canonVault, canonWs) {
			return nil, fmt.Errorf("%w: workspace %s overlaps vault %s", ErrVaultSecretBlocked, canonWs, canonVault)
		}
	}

	homeDir, _ := os.UserHomeDir()
	var canonHomeVault string
	if homeDir != "" {
		hv := filepath.Join(homeDir, ".gorness")
		if resolvedHV, err := resolveCanonicalPath(hv, 0); err == nil {
			canonHomeVault = resolvedHV
		} else {
			canonHomeVault = filepath.Clean(hv)
		}
	}

	return &PathGuard{
		canonWs:         canonWs,
		canonVault:      canonVault,
		canonHomeVault:  canonHomeVault,
		userHome:        homeDir,
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
	if normSlash == "~/.gorness" || strings.HasPrefix(normSlash, "~/.gorness/") {
		return "", "", false, fmt.Errorf("%w: %s", ErrVaultSecretBlocked, target)
	}

	expanded := trimmed
	if expanded == "~" && p.userHome != "" {
		expanded = p.userHome
	} else if strings.HasPrefix(normSlash, "~/") && p.userHome != "" {
		expanded = filepath.Join(p.userHome, filepath.FromSlash(normSlash[2:]))
	}

	var candidateAbs string
	if filepath.IsAbs(expanded) {
		candidateAbs = filepath.Clean(expanded)
	} else {
		candidateAbs = filepath.Join(p.canonWs, expanded)
	}

	canonicalAbs, err = resolveCanonicalPath(candidateAbs, 0)
	if err != nil {
		return "", "", false, err
	}

	inWS := isWithinOrEqualPath(canonicalAbs, p.canonWs)

	// 检查是否命中 ~/.gorness 机密目录（若工作区本身是 runtime/worktrees/<id>，仅放行该 worktree 内部路径）。
	if p.canonVault != "" && isWithinOrEqualPath(canonicalAbs, p.canonVault) {
		if !(p.wsInWorktreeDir && inWS) {
			return canonicalAbs, "", false, fmt.Errorf("%w: %s", ErrVaultSecretBlocked, target)
		}
	}
	if p.canonHomeVault != "" && isWithinOrEqualPath(canonicalAbs, p.canonHomeVault) {
		if !inWS {
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

// IsAutoRunConfigPath 判断目标路径是否属于自执行/敏感工程配置（供 PermissionGate 第 5 步拦截确认）。
func IsAutoRunConfigPath(canonicalAbs, rel string) bool {
	check := filepath.ToSlash(strings.TrimSpace(rel))
	if check == "" {
		check = filepath.ToSlash(canonicalAbs)
	}
	check = strings.TrimPrefix(check, "./")
	base := filepath.Base(check)

	switch base {
	case ".zshrc", ".zshenv", ".zprofile", ".zlogin",
		".bashrc", ".bash_profile", ".profile",
		".mcp.json", "mcp.toml":
		return true
	}
	if check == ".gorness" || strings.HasPrefix(check, ".gorness/") || strings.Contains(check, "/.gorness/") {
		return true
	}
	if strings.HasPrefix(check, ".git/hooks/") || strings.Contains(check, "/.git/hooks/") {
		return true
	}
	if strings.HasPrefix(check, ".github/workflows/") || strings.Contains(check, "/.github/workflows/") {
		return true
	}
	return false
}

// resolveCanonicalPath 逐段做 Lstat + Readlink 规范化解析，支持尚不存在的子目录/子文件与悬空符号链接。
func resolveCanonicalPath(absPath string, depth int) (string, error) {
	if depth > maxSymlinkHops {
		return "", fmt.Errorf("%w: too many symlink hops resolving %s", ErrInvalidTargetPath, absPath)
	}
	cleaned := filepath.Clean(absPath)
	if !filepath.IsAbs(cleaned) {
		var err error
		cleaned, err = filepath.Abs(cleaned)
		if err != nil {
			return "", err
		}
	}

	// 快速路径：若整条路径已存在且无悬空软链，直接走系统 EvalSymlinks。
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		return filepath.Clean(resolved), nil
	}

	// 逐段从根向叶扫描，精确处理父级软链、叶级悬空软链与不存在的新建子目录。
	relFromRoot := strings.TrimPrefix(cleaned, string(filepath.Separator))
	if relFromRoot == "" {
		return string(filepath.Separator), nil
	}
	parts := strings.Split(relFromRoot, string(filepath.Separator))

	cur := string(filepath.Separator)
	for i, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			cur = filepath.Dir(cur)
			continue
		}
		next := filepath.Join(cur, part)
		st, err := os.Lstat(next)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// 后续段尚不存在，直接拼接剩余段即可（祖先链已完成规范化）。
				rest := append([]string{next}, parts[i+1:]...)
				return filepath.Clean(filepath.Join(rest...)), nil
			}
			return "", err
		}

		if st.Mode()&os.ModeSymlink != 0 {
			linkTarget, rErr := os.Readlink(next)
			if rErr != nil {
				return "", rErr
			}
			var targetAbs string
			if filepath.IsAbs(linkTarget) {
				targetAbs = linkTarget
			} else {
				targetAbs = filepath.Join(cur, linkTarget)
			}
			if i+1 < len(parts) {
				tail := filepath.Join(parts[i+1:]...)
				targetAbs = filepath.Join(targetAbs, tail)
			}
			return resolveCanonicalPath(targetAbs, depth+1)
		}
		cur = next
	}
	return filepath.Clean(cur), nil
}

func isWithinOrEqualPath(path, root string) bool {
	if root == "" {
		return false
	}
	if path == root {
		return true
	}
	if root == string(filepath.Separator) {
		return strings.HasPrefix(path, root)
	}
	return strings.HasPrefix(path, root+string(filepath.Separator))
}

func isStrictlyWithinPath(path, root string) bool {
	if path == root {
		return false
	}
	return isWithinOrEqualPath(path, root)
}
