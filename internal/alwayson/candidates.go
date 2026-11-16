package alwayson

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/store"
)

var ErrCandidateNotFound = errors.New("skill candidate not found")

// CandidateSkill 表示由 Dreamer 提炼暂存在 `~/.gorness/skills/_candidates/<slug>/SKILL.md` 的待审候选技能。
type CandidateSkill struct {
	Slug        string    `json:"slug" toml:"slug"`
	Name        string    `json:"name" toml:"name"`
	Description string    `json:"description" toml:"description"`
	Paths       []string  `json:"paths,omitempty" toml:"paths,omitempty"`
	Body        string    `json:"body" toml:"body"`
	Content     string    `json:"content" toml:"content"`
	SourceFile  string    `json:"source_file" toml:"source_file"`
	UpdatedAt   time.Time `json:"updated_at" toml:"updated_at"`
}

type candidateSkillFrontmatter struct {
	Name        string   `toml:"name" json:"name"`
	Description string   `toml:"description" json:"description"`
	Paths       []string `toml:"paths,omitempty" json:"paths,omitempty"`
}

func (s *Service) candidatesLockPath() string {
	return filepath.Join(s.paths.RuntimeDir(), "locks", "alwayson-candidates.lock")
}

func (s *Service) candidatesRootDir() string {
	return filepath.Join(s.paths.SkillsDir(), "_candidates")
}

func validateCandidateSlug(slug string) (string, error) {
	if err := store.ValidateSafeID(slug); err != nil {
		return "", err
	}
	if strings.Contains(slug, "..") || slug == "_candidates" {
		return "", fmt.Errorf("%w: %q", store.ErrInvalidID, slug)
	}
	return slug, nil
}

// ListSkillCandidates 列出 `~/.gorness/skills/_candidates/*/SKILL.md` 下所有待审候选技能。
func (s *Service) ListSkillCandidates() ([]CandidateSkill, error) {
	var out []CandidateSkill
	err := store.WithFlock(s.candidatesLockPath(), false, func() error {
		entries, err := os.ReadDir(s.candidatesRootDir())
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				out = []CandidateSkill{}
				return nil
			}
			return fmt.Errorf("readdir skill candidates: %w", err)
		}
		items := make([]CandidateSkill, 0, len(entries))
		for _, ent := range entries {
			if !ent.IsDir() {
				continue
			}
			slug := ent.Name()
			if _, vErr := validateCandidateSlug(slug); vErr != nil {
				continue
			}
			skillFile := filepath.Join(s.candidatesRootDir(), slug, "SKILL.md")
			fi, sErr := os.Stat(skillFile)
			if sErr != nil || fi.IsDir() {
				continue
			}
			raw, rErr := os.ReadFile(skillFile)
			if rErr != nil {
				continue
			}
			items = append(items, parseCandidateSkillMD(raw, slug, skillFile, fi.ModTime().UTC()))
		}
		sort.Slice(items, func(i, j int) bool {
			if !items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
				return items[i].UpdatedAt.After(items[j].UpdatedAt)
			}
			return items[i].Slug < items[j].Slug
		})
		out = items
		return nil
	})
	return out, err
}

// PromoteSkillCandidate 将候选技能 `skills/_candidates/<slug>/SKILL.md` 正式晋升至 `skills/<slug>/SKILL.md` 并清理候选目录。
func (s *Service) PromoteSkillCandidate(slug string) error {
	safeSlug, err := validateCandidateSlug(slug)
	if err != nil {
		return err
	}
	candDir := filepath.Join(s.candidatesRootDir(), safeSlug)
	candFile := filepath.Join(candDir, "SKILL.md")
	destFile := filepath.Join(s.paths.SkillsDir(), safeSlug, "SKILL.md")

	pErr := store.WithFlock(s.candidatesLockPath(), true, func() error {
		raw, rErr := os.ReadFile(candFile)
		if rErr != nil {
			if errors.Is(rErr, os.ErrNotExist) {
				return fmt.Errorf("%w: %s", ErrCandidateNotFound, safeSlug)
			}
			return fmt.Errorf("read candidate SKILL.md %s: %w", safeSlug, rErr)
		}
		if wErr := store.AtomicWriteFile(destFile, raw, 0o600); wErr != nil {
			return fmt.Errorf("promote SKILL.md %s: %w", safeSlug, wErr)
		}
		if rmErr := os.RemoveAll(candDir); rmErr != nil {
			return fmt.Errorf("remove promoted candidate dir %s: %w", safeSlug, rmErr)
		}
		return nil
	})
	if pErr != nil {
		return pErr
	}
	s.publishAlwaysOnEvent("candidate_promoted", map[string]any{"slug": safeSlug})
	return nil
}

// DeleteSkillCandidate 删除指定的待审候选技能目录 `skills/_candidates/<slug>`。
func (s *Service) DeleteSkillCandidate(slug string) error {
	safeSlug, err := validateCandidateSlug(slug)
	if err != nil {
		return err
	}
	candDir := filepath.Join(s.candidatesRootDir(), safeSlug)

	dErr := store.WithFlock(s.candidatesLockPath(), true, func() error {
		if _, sErr := os.Stat(candDir); sErr != nil {
			if errors.Is(sErr, os.ErrNotExist) {
				return fmt.Errorf("%w: %s", ErrCandidateNotFound, safeSlug)
			}
			return sErr
		}
		return os.RemoveAll(candDir)
	})
	if dErr != nil {
		return dErr
	}
	s.publishAlwaysOnEvent("candidate_deleted", map[string]any{"slug": safeSlug})
	return nil
}

func parseCandidateSkillMD(raw []byte, slug, sourceFile string, updatedAt time.Time) CandidateSkill {
	content := string(raw)
	fm, body, err := config.ParseFrontmatterAs[candidateSkillFrontmatter](content)
	if err != nil {
		return CandidateSkill{
			Slug:       slug,
			Name:       slug,
			Body:       strings.TrimSpace(content),
			Content:    content,
			SourceFile: sourceFile,
			UpdatedAt:  updatedAt,
		}
	}
	name := strings.TrimSpace(fm.Name)
	if name == "" {
		name = slug
	}
	return CandidateSkill{
		Slug:        slug,
		Name:        name,
		Description: strings.TrimSpace(fm.Description),
		Paths:       fm.Paths,
		Body:        strings.TrimSpace(body),
		Content:     content,
		SourceFile:  sourceFile,
		UpdatedAt:   updatedAt,
	}
}
