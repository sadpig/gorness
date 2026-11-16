package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const MaxSymlinkHops = 40

var ErrSymlinkDepthExceeded = errors.New("symlink resolution exceeded max depth (40)")

// DefaultIgnoredWalkDirs defines standard build/VCS/vendor directories skipped during workspace walks.
var DefaultIgnoredWalkDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	"target":       true,
	".next":        true,
	"__pycache__":  true,
	".venv":        true,
}

// IsIgnoredWalkDir reports whether dirName is in DefaultIgnoredWalkDirs.
func IsIgnoredWalkDir(dirName string) bool {
	return DefaultIgnoredWalkDirs[dirName]
}

var homeShellStartupFiles = map[string]bool{
	".zshrc":        true,
	".zshenv":       true,
	".zprofile":     true,
	".zlogin":       true,
	".bashrc":       true,
	".bash_profile": true,
	".profile":      true,
}

var protectedPathComponents = map[string]bool{
	".git":     true,
	".gorness": true,
	".vscode":  true,
	".idea":    true,
}

// ExpandHome expands "~" and "~/..." against userHome (falling back to os.UserHomeDir())
// without lexically collapsing ".." segments before symlink resolution.
func ExpandHome(rawPath, userHome string) string {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return ""
	}
	norm := strings.ReplaceAll(trimmed, "\\", "/")
	home := strings.TrimSpace(userHome)
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if norm == "~" && home != "" {
		return home
	}
	if strings.HasPrefix(norm, "~/") && home != "" {
		return strings.TrimRight(strings.ReplaceAll(home, "\\", "/"), "/") + "/" + norm[2:]
	}
	return trimmed
}

// CanonicalPath resolves symlinks segment-by-segment following POSIX semantics ("symlink/.."
// resolves the symlink target before ascending "..") and preserves non-existent trailing segments.
func CanonicalPath(p string) (string, error) {
	canon, _, err := CanonicalPathTracking(p, "")
	return canon, err
}

// CanonicalPathTracking resolves p segment-by-segment like CanonicalPath and also reports
// whether any symlink strictly inside watchRootCanon (when non-empty) was traversed.
func CanonicalPathTracking(p, watchRootCanon string) (string, bool, error) {
	raw := strings.TrimSpace(p)
	if raw == "" {
		return "", false, errors.New("empty path")
	}
	slash := strings.ReplaceAll(raw, "\\", "/")
	if !strings.HasPrefix(slash, "/") && filepath.VolumeName(raw) == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", false, err
		}
		if resolvedWD, eErr := filepath.EvalSymlinks(wd); eErr == nil {
			wd = resolvedWD
		}
		wdSlash := strings.TrimRight(strings.ReplaceAll(wd, "\\", "/"), "/")
		if wdSlash == "" {
			slash = "/" + slash
		} else {
			slash = wdSlash + "/" + slash
		}
	}

	watchClean := strings.TrimSpace(watchRootCanon)
	if watchClean != "" {
		watchClean = filepath.Clean(watchClean)
	}

	return resolveCanonicalSegments(filepath.FromSlash(slash), 0, watchClean)
}

func resolveCanonicalSegments(p string, hops int, watchRootCanon string) (string, bool, error) {
	if hops > MaxSymlinkHops {
		return "", false, ErrSymlinkDepthExceeded
	}

	sep := string(os.PathSeparator)
	vol := filepath.VolumeName(p)
	rest := p[len(vol):]
	isAbs := strings.HasPrefix(rest, sep)

	cur := vol
	if isAbs {
		cur = vol + sep
	} else if cur == "" {
		cur = "."
	}

	rawParts := strings.Split(rest, sep)
	queue := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		if part == "" || part == "." {
			continue
		}
		queue = append(queue, part)
	}

	crossedWatchLink := false

	for len(queue) > 0 {
		seg := queue[0]
		queue = queue[1:]

		if seg == ".." {
			cur = filepath.Dir(cur)
			continue
		}

		var next string
		if cur == sep || strings.HasSuffix(cur, sep) {
			next = cur + seg
		} else {
			next = cur + sep + seg
		}

		fi, err := os.Lstat(next)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// Keep walking segment-by-segment without early exit so "nonexistent/../symlink/..."
				// still runs Lstat/Readlink on subsequent segments after ascending "..".
				cur = next
				continue
			}
			return "", crossedWatchLink, err
		}

		if fi.Mode()&os.ModeSymlink != 0 {
			if watchRootCanon != "" && IsStrictlyWithinRoot(next, watchRootCanon) {
				crossedWatchLink = true
			}
			hops++
			if hops > MaxSymlinkHops {
				return "", crossedWatchLink, ErrSymlinkDepthExceeded
			}
			linkTarget, rErr := os.Readlink(next)
			if rErr != nil {
				return "", crossedWatchLink, rErr
			}

			lt := filepath.FromSlash(strings.ReplaceAll(linkTarget, "\\", "/"))
			var targetRest string
			if filepath.IsAbs(lt) {
				tVol := filepath.VolumeName(lt)
				cur = tVol + sep
				targetRest = lt[len(tVol):]
			} else {
				targetRest = lt
			}

			linkParts := strings.Split(targetRest, sep)
			expanded := make([]string, 0, len(linkParts)+len(queue))
			for _, lp := range linkParts {
				if lp == "" || lp == "." {
					continue
				}
				expanded = append(expanded, lp)
			}
			expanded = append(expanded, queue...)
			queue = expanded
			continue
		}

		if !fi.IsDir() && len(queue) > 0 {
			return "", crossedWatchLink, fmt.Errorf("non-directory component %s in %s", next, p)
		}

		cur = next
	}

	return filepath.Clean(cur), crossedWatchLink, nil
}

// IsWithinRoot reports whether candidateCanon equals rootCanon or lies underneath rootCanon.
func IsWithinRoot(candidateCanon, rootCanon string) bool {
	if candidateCanon == "" || rootCanon == "" {
		return false
	}
	cleanCand := filepath.Clean(candidateCanon)
	cleanRoot := filepath.Clean(rootCanon)
	if cleanCand == cleanRoot {
		return true
	}
	prefix := cleanRoot
	if !strings.HasSuffix(prefix, string(os.PathSeparator)) {
		prefix += string(os.PathSeparator)
	}
	return strings.HasPrefix(cleanCand, prefix)
}

// IsStrictlyWithinRoot reports whether candidateCanon is strictly inside rootCanon (not equal to rootCanon).
func IsStrictlyWithinRoot(candidateCanon, rootCanon string) bool {
	if candidateCanon == "" || rootCanon == "" {
		return false
	}
	cleanCand := filepath.Clean(candidateCanon)
	cleanRoot := filepath.Clean(rootCanon)
	if cleanCand == cleanRoot {
		return false
	}
	return IsWithinRoot(cleanCand, cleanRoot)
}

// IsProtectedAutoRunPath checks whether absTarget touches auto-executing or sensitive configuration
// surfaces (.git, .gorness, .vscode, .idea, .env*, .github/workflows, shell startup files, MCP/SKILL configs).
func IsProtectedAutoRunPath(absTarget, wsRoot, userHome string) (string, bool) {
	trimmed := strings.TrimSpace(absTarget)
	if trimmed == "" {
		return "", false
	}

	home := strings.TrimSpace(userHome)
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	expanded := ExpandHome(trimmed, home)
	if !filepath.IsAbs(expanded) && strings.TrimSpace(wsRoot) != "" {
		expanded = strings.TrimRight(wsRoot, "/\\") + "/" + expanded
	}

	cleanTarget := filepath.Clean(expanded)
	if filepath.IsAbs(cleanTarget) {
		if canon, err := CanonicalPath(expanded); err == nil && canon != "" {
			cleanTarget = canon
		}
	}

	cleanHome := ""
	if home != "" {
		cleanHome = filepath.Clean(home)
		if canonHome, err := CanonicalPath(cleanHome); err == nil && canonHome != "" {
			cleanHome = canonHome
		}
	}

	cleanWS := ""
	if strings.TrimSpace(wsRoot) != "" {
		cleanWS = filepath.Clean(wsRoot)
		if canonWS, err := CanonicalPath(cleanWS); err == nil && canonWS != "" {
			cleanWS = canonWS
		}
	}

	base := filepath.Base(cleanTarget)
	lowerBase := strings.ToLower(base)

	if homeShellStartupFiles[base] {
		if cleanHome != "" && cleanTarget == filepath.Join(cleanHome, base) {
			return fmt.Sprintf("shell startup file (~/%s)", base), true
		}
		return fmt.Sprintf("shell startup file (%s)", base), true
	}

	if strings.EqualFold(base, "SKILL.md") {
		return "executable skill definition (SKILL.md)", true
	}
	if lowerBase == ".mcp.json" || lowerBase == "mcp.toml" {
		return fmt.Sprintf("MCP server configuration (%s)", base), true
	}
	if lowerBase == ".env" || strings.HasPrefix(lowerBase, ".env.") || lowerBase == ".envrc" || strings.HasPrefix(lowerBase, ".env") {
		return fmt.Sprintf("environment secret/auto-run file (%s)", base), true
	}

	slashPath := filepath.ToSlash(cleanTarget)
	if strings.Contains(slashPath, "/.agents/skills/") || strings.Contains(slashPath, "/.claude/skills/") {
		return "skill directory definition", true
	}
	if strings.Contains(slashPath, "/.github/workflows/") || strings.HasSuffix(slashPath, "/.github/workflows") {
		return "CI workflow directory (.github/workflows)", true
	}

	checkPath := slashPath
	if cleanWS != "" && IsWithinRoot(cleanTarget, cleanWS) {
		if rel, err := filepath.Rel(cleanWS, cleanTarget); err == nil {
			relSlash := filepath.ToSlash(rel)
			if relSlash == "." {
				return "", false
			}
			if relSlash == ".github/workflows" || strings.HasPrefix(relSlash, ".github/workflows/") {
				return "CI workflow directory (.github/workflows)", true
			}
			if strings.HasPrefix(relSlash, ".agents/skills/") || strings.HasPrefix(relSlash, ".claude/skills/") {
				return "skill directory definition", true
			}
			checkPath = relSlash
		}
	}

	for _, seg := range strings.Split(checkPath, "/") {
		if seg == ".gorness" {
			if checkPath == slashPath && !hasNonWorktreeGornessPart(slashPath) {
				continue
			}
			return "protected repository/IDE config (.gorness)", true
		}
		if protectedPathComponents[seg] {
			return fmt.Sprintf("protected repository/IDE config (%s)", seg), true
		}
	}

	return "", false
}

func hasNonWorktreeGornessPart(slashPath string) bool {
	if slashPath == ".gorness" || strings.HasPrefix(slashPath, ".gorness/") || strings.HasSuffix(slashPath, "/.gorness") {
		return true
	}
	rest := slashPath
	for {
		idx := strings.Index(rest, "/.gorness/")
		if idx < 0 {
			return false
		}
		after := rest[idx+len("/.gorness/"):]
		if !strings.HasPrefix(after, "runtime/worktrees/") {
			return true
		}
		rest = after
	}
}

// IsWorkspaceFileTrusted verifies raw bytes against SHA-256 entries in trusted, matching
// workspace-relative slash paths (".gorness/mcp.toml"), ".gorness/"-trimmed keys ("mcp.toml"),
// raw paths, and canonical paths.
func IsWorkspaceFileTrusted(wsPath, targetFile string, raw []byte, trusted map[string]string) bool {
	if len(trusted) == 0 {
		return false
	}
	sum := sha256.Sum256(raw)
	actualHex := hex.EncodeToString(sum[:])

	canonWS := filepath.Clean(wsPath)
	if c, err := CanonicalPath(wsPath); err == nil && c != "" {
		canonWS = c
	}
	absTarget := targetFile
	if !filepath.IsAbs(absTarget) {
		absTarget = filepath.Join(wsPath, absTarget)
	}
	absTarget = filepath.Clean(absTarget)
	canonTarget := absTarget
	if c, err := CanonicalPath(absTarget); err == nil && c != "" {
		canonTarget = c
	}

	candidates := []string{absTarget, canonTarget}
	if rel, err := filepath.Rel(canonWS, canonTarget); err == nil {
		relSlash := filepath.ToSlash(rel)
		candidates = append(candidates, relSlash)
		if trimmed, ok := strings.CutPrefix(relSlash, ".gorness/"); ok {
			candidates = append(candidates, trimmed)
		}
	}
	if relRaw, err := filepath.Rel(filepath.Clean(wsPath), absTarget); err == nil {
		relSlash := filepath.ToSlash(relRaw)
		candidates = append(candidates, relSlash)
		if trimmed, ok := strings.CutPrefix(relSlash, ".gorness/"); ok {
			candidates = append(candidates, trimmed)
		}
	}

	matched := false
	for _, key := range candidates {
		want, ok := trusted[key]
		if !ok {
			continue
		}
		normWant := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(want), "sha256:")))
		if normWant == "" || normWant != actualHex {
			return false
		}
		matched = true
	}
	return matched
}

// ComputeWorkspaceTrustHashes hashes all trust-gated configuration files currently inside wsPath
// (.gorness/mcp.toml, .gorness/hooks.toml, .gorness/providers.toml, .gorness/rules.toml,
// .gorness/agents/*.md, .gorness/workflows/*.toml, .mcp.json), rejecting symlinks that escape wsPath.
func ComputeWorkspaceTrustHashes(wsPath string) (map[string]string, error) {
	trimmed := strings.TrimSpace(wsPath)
	if trimmed == "" {
		return nil, errors.New("workspace path is empty")
	}
	canonWS, err := CanonicalPath(trimmed)
	if err != nil {
		return nil, fmt.Errorf("canonicalize workspace path: %w", err)
	}

	hashes := make(map[string]string)
	hashCandidate := func(relSlash string) error {
		full := filepath.Join(canonWS, filepath.FromSlash(relSlash))
		st, lErr := os.Lstat(full)
		if lErr != nil {
			if errors.Is(lErr, os.ErrNotExist) {
				return nil
			}
			return lErr
		}
		canonFull, cErr := CanonicalPath(full)
		if cErr != nil {
			return nil
		}
		if !IsStrictlyWithinRoot(canonFull, canonWS) {
			return fmt.Errorf("refusing to trust symlink escaping workspace: %s", relSlash)
		}
		info, sErr := os.Stat(canonFull)
		if sErr != nil || info.IsDir() || (!st.Mode().IsRegular() && st.Mode()&os.ModeSymlink == 0) {
			return nil
		}
		raw, rErr := os.ReadFile(canonFull)
		if rErr != nil {
			return rErr
		}
		sum := sha256.Sum256(raw)
		hashes[relSlash] = hex.EncodeToString(sum[:])
		return nil
	}

	hashDirExt := func(subDir, ext string) {
		dirPath := filepath.Join(canonWS, ".gorness", subDir)
		entries, rErr := os.ReadDir(dirPath)
		if rErr != nil {
			return
		}
		for _, ent := range entries {
			if ent.IsDir() || !strings.HasSuffix(strings.ToLower(ent.Name()), ext) {
				continue
			}
			rel := ".gorness/" + subDir + "/" + ent.Name()
			full := filepath.Join(dirPath, ent.Name())
			canonFile, cErr := CanonicalPath(full)
			if cErr != nil || !IsStrictlyWithinRoot(canonFile, canonWS) {
				continue
			}
			if info, sErr := os.Stat(canonFile); sErr == nil && info.Mode().IsRegular() {
				if raw, fErr := os.ReadFile(canonFile); fErr == nil {
					sum := sha256.Sum256(raw)
					hashes[rel] = hex.EncodeToString(sum[:])
				}
			}
		}
	}

	wsGorness := filepath.Join(canonWS, ".gorness")
	if _, err := os.Lstat(wsGorness); err == nil {
		canonGorness, cErr := CanonicalPath(wsGorness)
		if cErr != nil || !IsStrictlyWithinRoot(canonGorness, canonWS) {
			return nil, fmt.Errorf("refusing to trust .gorness symlink escaping workspace")
		}
		for _, rel := range []string{
			".gorness/mcp.toml",
			".gorness/hooks.toml",
			".gorness/providers.toml",
			".gorness/rules.toml",
		} {
			if err := hashCandidate(rel); err != nil {
				return nil, err
			}
		}
		hashDirExt("agents", ".md")
		hashDirExt("workflows", ".toml")
	}

	if err := hashCandidate(".mcp.json"); err != nil {
		return nil, err
	}

	return hashes, nil
}

// VerifyWorkspaceTrust checks all security-sensitive project files inside wsPath against trusted.
// It returns the sorted relative paths of any untrusted or tampered project config files and ok == (len == 0).
func VerifyWorkspaceTrust(wsPath string, trusted map[string]string) ([]string, bool) {
	trimmed := strings.TrimSpace(wsPath)
	if trimmed == "" {
		return nil, true
	}
	canonWS, err := CanonicalPath(trimmed)
	if err != nil || canonWS == "" {
		canonWS = filepath.Clean(trimmed)
	}

	var untrusted []string
	seen := make(map[string]bool)
	addUntrusted := func(rel string) {
		if !seen[rel] {
			seen[rel] = true
			untrusted = append(untrusted, rel)
		}
	}

	wsGorness := filepath.Join(canonWS, ".gorness")
	if _, lErr := os.Lstat(wsGorness); lErr == nil {
		canonGorness, cErr := CanonicalPath(wsGorness)
		if cErr != nil || !IsStrictlyWithinRoot(canonGorness, canonWS) {
			addUntrusted(".gorness")
			return untrusted, false
		}
	}

	checkFile := func(relSlash string, requiredToken string) {
		full := filepath.Join(canonWS, filepath.FromSlash(relSlash))
		st, err := os.Lstat(full)
		if err != nil {
			return
		}
		if st.Mode()&os.ModeSymlink != 0 && (relSlash == ".gorness/mcp.toml" || relSlash == ".gorness/hooks.toml" || relSlash == ".mcp.json") {
			// Symlinks for executable MCP/hook configs must resolve strictly inside workspace AND match hash;
			// escaping symlinks or unhashed symlinks are untrusted.
		}
		canonFull, cErr := CanonicalPath(full)
		if cErr != nil || !IsStrictlyWithinRoot(canonFull, canonWS) {
			addUntrusted(relSlash)
			return
		}
		info, sErr := os.Stat(canonFull)
		if sErr != nil || info.IsDir() || !info.Mode().IsRegular() {
			return
		}
		raw, rErr := os.ReadFile(canonFull)
		if rErr != nil {
			addUntrusted(relSlash)
			return
		}
		if bytes.Contains(raw, []byte("cmd:")) {
			if relSlash == ".gorness/providers.toml" || relSlash == ".gorness/mcp.toml" || strings.HasPrefix(relSlash, ".gorness/agents/") {
				addUntrusted(relSlash)
				return
			}
		}
		if requiredToken != "" && !bytes.Contains(raw, []byte(requiredToken)) {
			return
		}
		rawTarget := filepath.Join(trimmed, filepath.FromSlash(relSlash))
		if !IsWorkspaceFileTrusted(trimmed, rawTarget, raw, trusted) && !IsWorkspaceFileTrusted(canonWS, canonFull, raw, trusted) {
			addUntrusted(relSlash)
		}
	}

	checkFile(".gorness/mcp.toml", "")
	checkFile(".gorness/hooks.toml", "")
	checkFile(".gorness/providers.toml", "")
	checkFile(".mcp.json", "")

	agentsDir := filepath.Join(canonWS, ".gorness", "agents")
	if entries, err := os.ReadDir(agentsDir); err == nil {
		for _, ent := range entries {
			if ent.IsDir() || !strings.HasSuffix(strings.ToLower(ent.Name()), ".md") {
				continue
			}
			checkFile(".gorness/agents/"+ent.Name(), "api_key")
		}
	}

	workflowsDir := filepath.Join(canonWS, ".gorness", "workflows")
	if entries, err := os.ReadDir(workflowsDir); err == nil {
		for _, ent := range entries {
			if ent.IsDir() || !strings.HasSuffix(strings.ToLower(ent.Name()), ".toml") {
				continue
			}
			checkFile(".gorness/workflows/"+ent.Name(), "verify_command")
		}
	}

	sort.Strings(untrusted)
	return untrusted, len(untrusted) == 0
}
