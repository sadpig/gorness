package alwayson

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/llm"
	"gorness.dev/gorness/internal/store"
)

var reFencedCodeBlock = regexp.MustCompile("(?s)```(?:markdown|md|yaml|toml|json)?\\s*\n(.*?)```")

type dreamCandidateSpec struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Paths       []string `json:"paths,omitempty"`
	Body        string   `json:"body"`
}

type dreamNoteSpec struct {
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Body    string `json:"body"`
}

type dreamJSONEnvelope struct {
	Skills []dreamCandidateSpec `json:"skills"`
	Notes  []dreamNoteSpec      `json:"notes"`
}

// RunDreamPass 在空闲整理阶段使用 dreamer 角色回顾近期会话轨迹，
// 将高频排障经验与工作流提炼为指针记忆（memory/INDEX.md + notes/*.md）及候选技能（~/.gorness/skills/_candidates/<slug>/SKILL.md）。
func (s *Service) RunDreamPass(ctx context.Context, wsID string) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.engine == nil || s.cfg == nil {
		return nil, errors.New("alwayson service requires Engine and Config")
	}

	cleanWS := strings.TrimSpace(wsID)
	snap := s.cfg.Current()
	var wsRoleModel string
	if cleanWS != "" {
		if ws, err := s.engine.Workspaces().Get(cleanWS); err == nil && ws != nil {
			wsRoleModel = ws.RoleModels["dreamer"]
			if wsSnap, _, fErr := s.cfg.ForWorkspace(ws.RootPath, ws.TrustedHashes); fErr == nil && wsSnap != nil {
				snap = wsSnap
			}
		}
	}

	route, err := s.engine.Router().Resolve("dreamer", wsRoleModel, "", snap)
	if err != nil {
		return nil, fmt.Errorf("resolve dreamer route: %w", err)
	}

	digest, sessionCount := s.collectDreamDigest(cleanWS, 10)

	staticSys := "You are the Dreamer agent in Gorness. Distill recurring workflows, debugging lessons, and project invariants into concise pointer notes and reusable candidate SKILL.md definitions."
	if snap != nil {
		if prof, ok := snap.Agents["dreamer"]; ok && strings.TrimSpace(prof.SystemPrompt) != "" {
			staticSys = prof.SystemPrompt
		}
	}

	userPrompt := fmt.Sprintf(
		"Review the following %d recent session transcript(s) and output a reusable candidate SKILL.md (with `---` YAML frontmatter containing `name` and `description`, followed by step-by-step instructions).\n\n%s",
		sessionCount,
		digest,
	)

	streamCh, err := s.engine.Router().StreamWithRetry(ctx, route, llm.Request{
		Model:        route.ModelID,
		Effort:       route.Effort,
		StaticSystem: staticSys,
		History: []store.SessionEntry{
			{
				Seq:    1,
				ID:     "dream_req",
				Role:   "user",
				Blocks: []store.ContentBlock{{Type: "text", Text: userPrompt}},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("stream dreamer pass: %w", err)
	}

	var rawOut strings.Builder
	for ev := range streamCh {
		switch ev.Type {
		case llm.StreamStepReset:
			rawOut.Reset()
		case llm.StreamTextDelta:
			rawOut.WriteString(ev.Text)
		case llm.StreamError:
			if ev.Err != nil {
				return nil, ev.Err
			}
			return nil, errors.New("dreamer stream failed")
		}
	}

	now := s.now().UTC()
	skills, notes := parseDreamerOutput(rawOut.String(), cleanWS)

	candidatesRoot := filepath.Join(s.paths.SkillsDir(), "_candidates")
	createdPaths := make([]string, 0, len(skills))

	for _, sk := range skills {
		slug := sanitizeSlug(sk.Name)
		if slug == "" {
			slug = fmt.Sprintf("candidate-skill-%d", now.Unix())
		}
		desc := collapseWhitespace(sk.Description)
		if desc == "" {
			desc = "Candidate skill distilled during Always-On Dreamer pass."
		}
		body := strings.TrimSpace(sk.Body)
		if body == "" {
			body = "# " + slug + "\n\nDistilled workflow instructions."
		}

		skillFile := filepath.Join(candidatesRoot, slug, "SKILL.md")
		formattedMD := formatCandidateSkillMD(slug, desc, sk.Paths, body)
		if err := store.AtomicWriteFile(skillFile, []byte(formattedMD), 0o600); err != nil {
			return nil, fmt.Errorf("write candidate SKILL.md %s: %w", skillFile, err)
		}
		createdPaths = append(createdPaths, skillFile)

		// Synchronize a corresponding pointer note in memory/INDEX.md + memory/notes/<slug>.md.
		_, _ = s.memory.upsertNote(
			"dream-"+slug,
			"Dreamed Skill: "+slug,
			desc,
			fmt.Sprintf("Candidate skill staged at `%s`.\n\n%s", skillFile, body),
			now,
		)
	}

	for _, nt := range notes {
		_, _ = s.memory.upsertNote(nt.Slug, nt.Title, nt.Summary, nt.Body, now)
	}

	if cleanWS != "" && len(createdPaths) > 0 {
		progBody := fmt.Sprintf("# Dreamer Pass (%s)\n\nGenerated %d candidate skill(s):\n- %s\n",
			now.Format("2006-01-02T15:04:05Z"),
			len(createdPaths),
			strings.Join(createdPaths, "\n- "),
		)
		_, _ = s.memory.writeWorkspaceProgress(cleanWS, "dream-latest", progBody)
	}

	return createdPaths, nil
}

func (s *Service) collectDreamDigest(wsID string, maxSessions int) (string, int) {
	metas := s.engine.Sessions().List(wsID, false)
	if len(metas) > maxSessions {
		metas = metas[:maxSessions]
	}
	if len(metas) == 0 {
		return "No prior sessions recorded yet; distill baseline workspace verification hygiene.", 0
	}

	var b strings.Builder
	count := 0
	for _, m := range metas {
		branch, err := s.engine.Sessions().LoadActiveBranch(m.ID)
		if err != nil || len(branch) == 0 {
			continue
		}
		count++
		fmt.Fprintf(&b, "## Session %s (%s)\n", m.ID, m.Title)
		for _, ent := range branch {
			for _, blk := range ent.Blocks {
				switch blk.Type {
				case "text":
					if t := strings.TrimSpace(blk.Text); t != "" {
						fmt.Fprintf(&b, "[%s]: %s\n", ent.Role, truncateString(t, 600))
					}
				case "tool_use":
					fmt.Fprintf(&b, "[tool_use %s]: %s\n", blk.ToolName, truncateString(string(blk.ToolInput), 240))
				}
			}
		}
		b.WriteByte('\n')
	}
	if count == 0 {
		return "Sessions exist without active message blocks; distill standard verification workflow.", 0
	}
	return b.String(), count
}

func parseDreamerOutput(raw string, wsID string) ([]dreamCandidateSpec, []dreamNoteSpec) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		defaultSlug := "workspace-workflow"
		if wsID != "" {
			defaultSlug = sanitizeSlug(wsID) + "-workflow"
		}
		return []dreamCandidateSpec{{
			Name:        defaultSlug,
			Description: "Distilled workflow patterns from recent Gorness sessions.",
			Body:        "# Workspace Workflow\n\n1. Inspect relevant files first.\n2. Run targeted tests before concluding.",
		}}, nil
	}

	// 1. Try JSON envelope (either raw or inside a fenced code block).
	candidates := []string{trimmed}
	for _, m := range reFencedCodeBlock.FindAllStringSubmatch(trimmed, -1) {
		if len(m) > 1 {
			candidates = append(candidates, strings.TrimSpace(m[1]))
		}
	}

	for _, cand := range candidates {
		if strings.HasPrefix(cand, "{") {
			var env dreamJSONEnvelope
			if err := json.Unmarshal([]byte(cand), &env); err == nil && len(env.Skills) > 0 {
				return env.Skills, env.Notes
			}
		}
	}

	// 2. Try SKILL.md frontmatter (--- or +++).
	for _, cand := range candidates {
		if strings.HasPrefix(cand, "---") || strings.HasPrefix(cand, "+++") {
			meta, body, err := config.ParseFrontmatterMD([]byte(cand))
			if err == nil && len(meta) > 0 {
				name := strings.TrimSpace(fmt.Sprint(meta["name"]))
				if meta["name"] == nil || name == "" {
					name = strings.TrimSpace(fmt.Sprint(meta["id"]))
				}
				desc := ""
				if meta["description"] != nil {
					desc = strings.TrimSpace(fmt.Sprint(meta["description"]))
				}
				if name != "" {
					return []dreamCandidateSpec{{
						Name:        name,
						Description: desc,
						Body:        strings.TrimSpace(body),
					}}, nil
				}
			}
		}
	}

	// 3. Fallback: synthesize a structured candidate SKILL.md from plain markdown text.
	slug := "distilled-workflow"
	if wsID != "" {
		slug = sanitizeSlug(wsID) + "-workflow"
	}
	firstLine := trimmed
	if idx := strings.IndexByte(firstLine, '\n'); idx >= 0 {
		firstLine = firstLine[:idx]
	}
	firstLine = strings.TrimSpace(strings.TrimLeft(firstLine, "#*- "))
	if s := sanitizeSlug(firstLine); s != "" {
		slug = s
	}

	return []dreamCandidateSpec{{
		Name:        slug,
		Description: truncateString(collapseWhitespace(firstLine), 140),
		Body:        trimmed,
	}}, nil
}

func formatCandidateSkillMD(name, description string, paths []string, body string) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %q\n", name)
	fmt.Fprintf(&b, "description: %q\n", description)
	if len(paths) > 0 {
		quoted := make([]string, 0, len(paths))
		for _, p := range paths {
			if t := strings.TrimSpace(p); t != "" {
				quoted = append(quoted, fmt.Sprintf("%q", t))
			}
		}
		if len(quoted) > 0 {
			fmt.Fprintf(&b, "paths: [%s]\n", strings.Join(quoted, ", "))
		}
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(body))
	b.WriteByte('\n')
	return b.String()
}
