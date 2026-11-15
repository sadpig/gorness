package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"
	"gorness.dev/gorness/internal/config"
)

type Workspace struct {
	ID            string            `toml:"id" json:"id"`
	Name          string            `toml:"name" json:"name"`
	RootPath      string            `toml:"root_path" json:"root_path"`
	PermMode      string            `toml:"perm_mode,omitempty" json:"perm_mode,omitempty"`
	SandboxMode   string            `toml:"sandbox_mode,omitempty" json:"sandbox_mode,omitempty"`
	RoleModels    map[string]string `toml:"role_models,omitempty" json:"role_models,omitempty"`
	TrustedHashes map[string]string `toml:"trusted_hashes,omitempty" json:"trusted_hashes,omitempty"`
	UpdatedAt     time.Time         `toml:"updated_at" json:"updated_at"`
}

// WorkspaceStore 维护 ~/.gorness/workspaces/<slug-hash>/workspace.toml 与内存读写锁索引。
type WorkspaceStore struct {
	paths  config.VaultPaths
	mu     sync.RWMutex
	byID   map[string]*Workspace
	byRoot map[string]string
}

func NewWorkspaceStore(paths config.VaultPaths) (*WorkspaceStore, error) {
	if strings.TrimSpace(paths.Root) == "" {
		return nil, fmt.Errorf("%w: empty vault root", ErrInvalidPath)
	}
	if err := os.MkdirAll(paths.WorkspacesDir(), 0o700); err != nil {
		return nil, fmt.Errorf("create workspaces dir %s: %w", paths.WorkspacesDir(), err)
	}

	s := &WorkspaceStore{
		paths:  paths,
		byID:   make(map[string]*Workspace),
		byRoot: make(map[string]string),
	}
	if err := s.loadExisting(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *WorkspaceStore) lockPath() string {
	return filepath.Join(s.paths.RuntimeDir(), "locks", "workspaces.lock")
}

func (s *WorkspaceStore) workspaceDir(id string) string {
	return filepath.Join(s.paths.WorkspacesDir(), id)
}

func (s *WorkspaceStore) workspaceTOMLPath(id string) string {
	return filepath.Join(s.workspaceDir(id), "workspace.toml")
}

func (s *WorkspaceStore) loadExisting() error {
	entries, err := os.ReadDir(s.paths.WorkspacesDir())
	if err != nil {
		return fmt.Errorf("readdir workspaces %s: %w", s.paths.WorkspacesDir(), err)
	}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		id := ent.Name()
		if validateSafeID(id) != nil {
			continue
		}
		tomlPath := s.workspaceTOMLPath(id)
		raw, rErr := os.ReadFile(tomlPath)
		if rErr != nil {
			if errors.Is(rErr, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("read %s: %w", tomlPath, rErr)
		}
		var ws Workspace
		if uErr := toml.Unmarshal(raw, &ws); uErr != nil {
			return fmt.Errorf("parse %s: %w", tomlPath, uErr)
		}
		if ws.ID == "" {
			ws.ID = id
		}
		s.byID[ws.ID] = &ws
		if ws.RootPath != "" {
			s.byRoot[ws.RootPath] = ws.ID
		}
	}
	return nil
}

func (s *WorkspaceStore) Register(path, name string) (*Workspace, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("%w: workspace path is required", ErrInvalidPath)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve abs path %s: %w", path, err)
	}
	canonRoot, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return nil, fmt.Errorf("canonicalize workspace path %s: %w", path, err)
	}
	canonRoot = filepath.Clean(canonRoot)

	st, err := os.Stat(canonRoot)
	if err != nil {
		return nil, fmt.Errorf("stat workspace %s: %w", canonRoot, err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("%w: workspace path %s must be a directory", ErrInvalidPath, canonRoot)
	}

	canonVault := filepath.Clean(s.paths.Root)
	if resolvedVault, vErr := filepath.EvalSymlinks(s.paths.Root); vErr == nil {
		canonVault = filepath.Clean(resolvedVault)
	}
	if isWithinOrEqualPath(canonRoot, canonVault) {
		return nil, fmt.Errorf("%w: cannot register gorness vault %s as workspace", ErrInvalidPath, canonRoot)
	}

	displayName := strings.TrimSpace(name)
	if displayName == "" {
		displayName = filepath.Base(canonRoot)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var result *Workspace
	err = WithFlock(s.lockPath(), true, func() error {
		now := time.Now().UTC()
		if existingID, ok := s.byRoot[canonRoot]; ok {
			existing := cloneWorkspace(s.byID[existingID])
			if strings.TrimSpace(name) != "" {
				existing.Name = displayName
			}
			existing.UpdatedAt = now
			if err := s.ensureWorkspaceLayout(existing.ID); err != nil {
				return err
			}
			if err := s.persistWorkspace(existing); err != nil {
				return err
			}
			s.byID[existing.ID] = existing
			result = cloneWorkspace(existing)
			return nil
		}

		wsID := deriveWorkspaceID(canonRoot, 4)
		if conflict, taken := s.byID[wsID]; taken && conflict.RootPath != canonRoot {
			wsID = deriveWorkspaceID(canonRoot, 8)
		}
		ws := &Workspace{
			ID:            wsID,
			Name:          displayName,
			RootPath:      canonRoot,
			RoleModels:    make(map[string]string),
			TrustedHashes: make(map[string]string),
			UpdatedAt:     now,
		}
		if err := s.ensureWorkspaceLayout(ws.ID); err != nil {
			return err
		}
		if err := s.persistWorkspace(ws); err != nil {
			return err
		}
		s.byID[ws.ID] = ws
		s.byRoot[canonRoot] = ws.ID
		result = cloneWorkspace(ws)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *WorkspaceStore) List() []*Workspace {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*Workspace, 0, len(s.byID))
	for _, ws := range s.byID {
		out = append(out, cloneWorkspace(ws))
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (s *WorkspaceStore) Get(id string) (*Workspace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ws, ok := s.byID[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrWorkspaceNotFound, id)
	}
	return cloneWorkspace(ws), nil
}

func (s *WorkspaceStore) GetByPath(path string) (*Workspace, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("%w: empty path", ErrInvalidPath)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	canonRoot, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return nil, err
	}
	canonRoot = filepath.Clean(canonRoot)

	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.byRoot[canonRoot]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrWorkspaceNotFound, canonRoot)
	}
	return cloneWorkspace(s.byID[id]), nil
}

func (s *WorkspaceStore) Update(id string, mutFn func(*Workspace)) (*Workspace, error) {
	if err := validateSafeID(id); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	orig, ok := s.byID[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrWorkspaceNotFound, id)
	}

	var result *Workspace
	err := WithFlock(s.lockPath(), true, func() error {
		cp := cloneWorkspace(orig)
		if mutFn != nil {
			mutFn(cp)
		}
		// Preserve immutable identity and canonical root path.
		cp.ID = orig.ID
		cp.RootPath = orig.RootPath
		cp.UpdatedAt = time.Now().UTC()

		if err := s.persistWorkspace(cp); err != nil {
			return err
		}
		s.byID[id] = cp
		result = cloneWorkspace(cp)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *WorkspaceStore) ensureWorkspaceLayout(id string) error {
	base := s.workspaceDir(id)
	for _, sub := range []string{
		base,
		filepath.Join(base, "sessions"),
		filepath.Join(base, "memory"),
		filepath.Join(base, "memory", "progress"),
		filepath.Join(base, "missions"),
		filepath.Join(base, "artifacts"),
	} {
		if err := os.MkdirAll(sub, 0o700); err != nil {
			return fmt.Errorf("create workspace subdir %s: %w", sub, err)
		}
	}
	return nil
}

func (s *WorkspaceStore) persistWorkspace(ws *Workspace) error {
	encoded, err := toml.Marshal(ws)
	if err != nil {
		return fmt.Errorf("marshal workspace %s: %w", ws.ID, err)
	}
	return AtomicWriteFile(s.workspaceTOMLPath(ws.ID), encoded, 0o600)
}

func deriveWorkspaceID(canonRoot string, hashBytes int) string {
	base := strings.ToLower(filepath.Base(canonRoot))
	var b strings.Builder
	lastDash := false
	for _, r := range base {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		default:
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
		if b.Len() >= 24 {
			break
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "ws"
	}
	if hashBytes <= 0 || hashBytes > 16 {
		hashBytes = 4
	}
	sum := sha256.Sum256([]byte(canonRoot))
	return fmt.Sprintf("%s-%s", slug, hex.EncodeToString(sum[:hashBytes]))
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneWorkspace(w *Workspace) *Workspace {
	if w == nil {
		return nil
	}
	cp := *w
	cp.RoleModels = cloneStringMap(w.RoleModels)
	cp.TrustedHashes = cloneStringMap(w.TrustedHashes)
	return &cp
}
