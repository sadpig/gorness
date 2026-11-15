package tools

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gorness.dev/gorness/internal/config"
)

type Skill struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Paths       []string `json:"paths,omitempty"`
	Body        string   `json:"body"`
	SourceFile  string   `json:"source_file"`
	Scope       string   `json:"scope"`
}

// SkillCatalog 负责扫描用户级与项目级 SKILL.md，强制执行规范路径软链边界校验与同名用户技能覆盖项目技能策略。
type SkillCatalog struct {
	mu      sync.RWMutex
	wsRoot  string
	skills  map[string]Skill
	ordered []string
}

func NewSkillCatalog() *SkillCatalog {
	return &SkillCatalog{
		skills: make(map[string]Skill),
	}
}

func DiscoverSkills(snap *config.Snapshot, wsRoot string) *SkillCatalog {
	return NewSkillCatalog().Discover(snap, wsRoot)
}

func (c *SkillCatalog) Discover(snap *config.Snapshot, wsRoot string) *SkillCatalog {
	if c == nil {
		c = NewSkillCatalog()
	}

	var canonWs string
	rawWsAbs := ""
	if strings.TrimSpace(wsRoot) != "" {
		if abs, err := filepath.Abs(wsRoot); err == nil {
			rawWsAbs = filepath.Clean(abs)
		}
		if resolved, err := filepath.EvalSymlinks(wsRoot); err == nil {
			canonWs = filepath.Clean(resolved)
		}
	}

	var canonVault string
	if snap != nil && strings.TrimSpace(snap.Paths.Root) != "" {
		canonVault = canonicalizeExistingOrClean(snap.Paths.Root)
	}
	homeDir, _ := os.UserHomeDir()
	var canonHomeVault string
	if homeDir != "" {
		canonHomeVault = canonicalizeExistingOrClean(filepath.Join(homeDir, ".gorness"))
	}

	wsInWorktree := canonWs != "" && canonVault != "" &&
		isStrictlyWithinPath(canonWs, filepath.Join(canonVault, "runtime", "worktrees"))

	type dirSpec struct {
		dir         string
		isWorkspace bool
	}

	isWorkspaceScopedDir := func(d string) bool {
		if d == "" {
			return false
		}
		absD, err := filepath.Abs(d)
		if err != nil {
			absD = filepath.Clean(d)
		} else {
			absD = filepath.Clean(absD)
		}
		if rawWsAbs != "" && isWithinOrEqualPath(absD, rawWsAbs) {
			return true
		}
		if canonWs != "" {
			if isWithinOrEqualPath(absD, canonWs) {
				return true
			}
			// 逐级检查祖先目录规范路径（兼容 macOS /var 与 /private/var 混用，同时即使末级 skills 是越界软链仍归为项目级目录予以拦截）。
			for anc := absD; anc != string(filepath.Separator) && anc != "."; anc = filepath.Dir(anc) {
				if resolvedAnc, eErr := filepath.EvalSymlinks(anc); eErr == nil {
					if filepath.Clean(resolvedAnc) == canonWs {
						return true
					}
				}
			}
		}
		return false
	}

	var userDirs []dirSpec
	var wsDirs []dirSpec
	seenLex := make(map[string]bool)

	addCandidate := func(raw string, forceWS bool) {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return
		}
		cleaned := filepath.Clean(trimmed)
		if seenLex[cleaned] {
			return
		}
		seenLex[cleaned] = true
		wsScoped := forceWS || isWorkspaceScopedDir(cleaned)
		if wsScoped {
			wsDirs = append(wsDirs, dirSpec{dir: cleaned, isWorkspace: true})
		} else {
			userDirs = append(userDirs, dirSpec{dir: cleaned, isWorkspace: false})
		}
	}

	if snap != nil {
		for _, d := range snap.SkillDirs {
			addCandidate(d, false)
		}
	}
	if strings.TrimSpace(wsRoot) != "" {
		addCandidate(filepath.Join(wsRoot, ".gorness", "skills"), true)
		addCandidate(filepath.Join(wsRoot, ".agents", "skills"), true)
		addCandidate(filepath.Join(wsRoot, ".claude", "skills"), true)
	}

	allSpecs := append(userDirs, wsDirs...)

	discovered := make(map[string]Skill)
	var order []string

	for _, spec := range allSpecs {
		if spec.isWorkspace && canonWs == "" {
			continue
		}
		canonDir, ok := resolveSafeSkillRoot(spec.dir, spec.isWorkspace, canonWs, canonVault, canonHomeVault, wsInWorktree)
		if !ok {
			continue
		}
		entries, err := os.ReadDir(canonDir)
		if err != nil {
			continue
		}
		names := make([]string, 0, len(entries))
		for _, ent := range entries {
			name := ent.Name()
			if strings.HasPrefix(name, ".") || name == "_candidates" {
				continue
			}
			names = append(names, name)
		}
		sort.Strings(names)

		for _, entryName := range names {
			subLex := filepath.Join(canonDir, entryName)
			canonSub, subOK := resolveSafeSkillEntryDir(subLex, spec.isWorkspace, canonWs, canonVault, canonHomeVault, wsInWorktree)
			if !subOK {
				continue
			}
			skillLex := filepath.Join(canonSub, "SKILL.md")
			canonSkillFile, fileOK := resolveSafeSkillFile(skillLex, spec.isWorkspace, canonWs, canonVault, canonHomeVault, wsInWorktree)
			if !fileOK {
				continue
			}

			raw, err := os.ReadFile(canonSkillFile)
			if err != nil {
				continue
			}
			sk, err := parseSkillMD(canonSkillFile, entryName, raw, spec.isWorkspace)
			if err != nil || sk.Name == "" {
				continue
			}
			// 用户级目录先扫描，先注册者胜出，确保同名用户 SKILL.md 覆盖项目内 SKILL.md。
			if _, exists := discovered[sk.Name]; exists {
				continue
			}
			discovered[sk.Name] = sk
			order = append(order, sk.Name)
		}
	}

	sort.Strings(order)

	c.mu.Lock()
	c.wsRoot = canonWs
	c.skills = discovered
	c.ordered = order
	c.mu.Unlock()
	return c
}

func (c *SkillCatalog) Get(name string) (Skill, bool) {
	if c == nil {
		return Skill{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	sk, ok := c.skills[strings.TrimSpace(name)]
	if !ok {
		return Skill{}, false
	}
	sk.Paths = append([]string(nil), sk.Paths...)
	return sk, true
}

func (c *SkillCatalog) All() []Skill {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Skill, 0, len(c.ordered))
	for _, name := range c.ordered {
		sk := c.skills[name]
		sk.Paths = append([]string(nil), sk.Paths...)
		out = append(out, sk)
	}
	return out
}

func (c *SkillCatalog) ActiveForPaths(touchedPaths []string) []Skill {
	if c == nil || len(touchedPaths) == 0 {
		return nil
	}
	c.mu.RLock()
	wsRoot := c.wsRoot
	skills := make([]Skill, 0, len(c.ordered))
	for _, name := range c.ordered {
		skills = append(skills, c.skills[name])
	}
	c.mu.RUnlock()

	normTouched := make([]string, 0, len(touchedPaths))
	for _, tp := range touchedPaths {
		trimmed := strings.TrimSpace(tp)
		if trimmed == "" {
			continue
		}
		if wsRoot != "" && filepath.IsAbs(trimmed) {
			canonTP := canonicalizeExistingOrClean(trimmed)
			if rel, err := filepath.Rel(wsRoot, canonTP); err == nil && !strings.HasPrefix(rel, "..") {
				trimmed = rel
			}
		}
		slash := strings.TrimPrefix(filepath.ToSlash(trimmed), "./")
		if slash != "" && slash != "." {
			normTouched = append(normTouched, slash)
		}
	}
	if len(normTouched) == 0 {
		return nil
	}

	var matched []Skill
	for _, sk := range skills {
		if len(sk.Paths) == 0 {
			continue
		}
		hit := false
		for _, globPat := range sk.Paths {
			for _, tp := range normTouched {
				if matchGlobPattern(globPat, tp) {
					hit = true
					break
				}
			}
			if hit {
				break
			}
		}
		if hit {
			cp := sk
			cp.Paths = append([]string(nil), sk.Paths...)
			matched = append(matched, cp)
		}
	}
	return matched
}

func (c *SkillCatalog) XMLSummary() string {
	all := c.All()
	if len(all) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<available_skills>\n")
	for _, sk := range all {
		b.WriteString("  <skill>\n")
		fmt.Fprintf(&b, "    <name>%s</name>\n", escapeXMLText(config.EscapeBidi(sk.Name)))
		if sk.Description != "" {
			fmt.Fprintf(&b, "    <description>%s</description>\n", escapeXMLText(config.EscapeBidi(sk.Description)))
		}
		if len(sk.Paths) > 0 {
			fmt.Fprintf(&b, "    <paths>%s</paths>\n", escapeXMLText(config.EscapeBidi(strings.Join(sk.Paths, ", "))))
		}
		b.WriteString("  </skill>\n")
	}
	b.WriteString("</available_skills>")
	return b.String()
}

func resolveSafeSkillRoot(
	dir string,
	isWorkspace bool,
	canonWs, canonVault, canonHomeVault string,
	wsInWorktree bool,
) (string, bool) {
	if _, err := os.Lstat(dir); err != nil {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", false
	}
	resolved = filepath.Clean(resolved)
	st, err := os.Stat(resolved)
	if err != nil || !st.IsDir() {
		return "", false
	}
	if isWorkspace {
		if !isStrictlyWithinPath(resolved, canonWs) {
			return "", false
		}
		if canonVault != "" && isWithinOrEqualPath(resolved, canonVault) && !wsInWorktree {
			return "", false
		}
		if canonHomeVault != "" && isWithinOrEqualPath(resolved, canonHomeVault) && !isStrictlyWithinPath(resolved, canonWs) {
			return "", false
		}
	}
	return resolved, true
}

func resolveSafeSkillEntryDir(
	subPath string,
	isWorkspace bool,
	canonWs, canonVault, canonHomeVault string,
	wsInWorktree bool,
) (string, bool) {
	if _, err := os.Lstat(subPath); err != nil {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(subPath)
	if err != nil {
		return "", false
	}
	resolved = filepath.Clean(resolved)
	st, err := os.Stat(resolved)
	if err != nil || !st.IsDir() {
		return "", false
	}
	if isWorkspace {
		if !isStrictlyWithinPath(resolved, canonWs) {
			return "", false
		}
		if canonVault != "" && isWithinOrEqualPath(resolved, canonVault) && !wsInWorktree {
			return "", false
		}
		if canonHomeVault != "" && isWithinOrEqualPath(resolved, canonHomeVault) && !isStrictlyWithinPath(resolved, canonWs) {
			return "", false
		}
	}
	return resolved, true
}

func resolveSafeSkillFile(
	filePath string,
	isWorkspace bool,
	canonWs, canonVault, canonHomeVault string,
	wsInWorktree bool,
) (string, bool) {
	if _, err := os.Lstat(filePath); err != nil {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(filePath)
	if err != nil {
		return "", false
	}
	resolved = filepath.Clean(resolved)
	st, err := os.Stat(resolved)
	if err != nil || !st.Mode().IsRegular() {
		return "", false
	}

	if isWorkspace {
		if !isStrictlyWithinPath(resolved, canonWs) {
			return "", false
		}
		if canonVault != "" && isWithinOrEqualPath(resolved, canonVault) && !wsInWorktree {
			return "", false
		}
		if canonHomeVault != "" && isWithinOrEqualPath(resolved, canonHomeVault) && !isStrictlyWithinPath(resolved, canonWs) {
			return "", false
		}
		return resolved, true
	}

	// 用户级技能目录同样禁止通过软链直接指向 Vault 顶层机密 TOML 文件。
	if canonVault != "" {
		skillsSub := filepath.Join(canonVault, "skills")
		if isWithinOrEqualPath(resolved, canonVault) && !isStrictlyWithinPath(resolved, skillsSub) {
			return "", false
		}
	}
	return resolved, true
}

func parseSkillMD(filePath, fallbackName string, raw []byte, isWorkspace bool) (Skill, error) {
	meta, body, err := config.ParseFrontmatterMD(raw)
	if err != nil {
		return Skill{}, err
	}
	name := stringFromMeta(meta, "name")
	if name == "" {
		name = stringFromMeta(meta, "id")
	}
	if name == "" {
		name = fallbackName
	}
	desc := stringFromMeta(meta, "description")
	paths := stringSliceFromMeta(meta, "paths")
	if len(paths) == 0 {
		paths = stringSliceFromMeta(meta, "globs")
	}

	scope := "user"
	if isWorkspace {
		scope = "workspace"
	}

	return Skill{
		Name:        strings.TrimSpace(name),
		Description: strings.TrimSpace(desc),
		Paths:       paths,
		Body:        strings.TrimSpace(body),
		SourceFile:  filePath,
		Scope:       scope,
	}, nil
}

func stringFromMeta(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	v, ok := meta[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func stringSliceFromMeta(meta map[string]any, key string) []string {
	if meta == nil {
		return nil
	}
	v, ok := meta[key]
	if !ok || v == nil {
		return nil
	}
	switch typed := v.(type) {
	case string:
		var out []string
		for _, p := range strings.Split(typed, ",") {
			if t := strings.TrimSpace(p); t != "" {
				out = append(out, t)
			}
		}
		return out
	case []string:
		var out []string
		for _, s := range typed {
			if t := strings.TrimSpace(s); t != "" {
				out = append(out, t)
			}
		}
		return out
	case []any:
		var out []string
		for _, elem := range typed {
			if s := strings.TrimSpace(fmt.Sprint(elem)); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// matchGlobPattern 支持标准通配符与 ** 跨目录递归通配匹配。
func matchGlobPattern(pattern, relSlash string) bool {
	pat := strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(pattern)), "./")
	target := strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(relSlash)), "./")
	if pat == "" || target == "" {
		return false
	}
	// 若模式不含路径分隔符（如 "*.go"），同时匹配文件基名。
	if !strings.Contains(pat, "/") {
		if ok, _ := path.Match(pat, path.Base(target)); ok {
			return true
		}
	}
	return matchSegments(strings.Split(pat, "/"), strings.Split(target, "/"))
}

func matchSegments(patSegs, targetSegs []string) bool {
	if len(patSegs) == 0 {
		return len(targetSegs) == 0
	}
	if patSegs[0] == "**" {
		// "**" 可匹配 0 个或多个目录段。
		if matchSegments(patSegs[1:], targetSegs) {
			return true
		}
		if len(targetSegs) > 0 {
			return matchSegments(patSegs, targetSegs[1:])
		}
		return false
	}
	if len(targetSegs) == 0 {
		return false
	}
	ok, err := path.Match(patSegs[0], targetSegs[0])
	if err != nil || !ok {
		return false
	}
	return matchSegments(patSegs[1:], targetSegs[1:])
}

func escapeXMLText(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
	)
	return r.Replace(s)
}
