package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"gorness.dev/gorness/internal/config"
)

var ErrArtifactNotFound = errors.New("artifact not found")

// Artifact represents a workspace deliverable (.html or .md) stored under workspaces/<wsID>/artifacts/.
type Artifact struct {
	ID          string    `toml:"id" json:"id"`
	WorkspaceID string    `toml:"workspace_id" json:"workspace_id"`
	SessionID   string    `toml:"session_id,omitempty" json:"session_id,omitempty"`
	Title       string    `toml:"title" json:"title"`
	Format      string    `toml:"format" json:"format"` // "html" | "md"
	FileName    string    `toml:"file_name" json:"file_name"`
	Path        string    `toml:"path" json:"path"`
	SizeBytes   int64     `toml:"size_bytes" json:"size_bytes"`
	Content     string    `toml:"-" json:"content,omitempty"`
	CreatedAt   time.Time `toml:"created_at" json:"created_at"`
	UpdatedAt   time.Time `toml:"updated_at" json:"updated_at"`
}

type artifactIndexDoc struct {
	Artifacts []Artifact `toml:"artifacts"`
}

// ArtifactStore manages Zero-DB HTML and Markdown artifacts under workspaces/<wsID>/artifacts/
// indexed via index.toml (never creating .json files inside the vault).
type ArtifactStore struct {
	paths config.VaultPaths
}

func NewArtifactStore(paths config.VaultPaths) (*ArtifactStore, error) {
	if strings.TrimSpace(paths.Root) == "" {
		return nil, fmt.Errorf("%w: empty vault root", ErrInvalidPath)
	}
	for _, dir := range []string{
		paths.WorkspacesDir(),
		paths.RuntimeDir(),
		filepath.Join(paths.RuntimeDir(), "locks"),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create artifact store dir %s: %w", dir, err)
		}
	}
	return &ArtifactStore{paths: paths}, nil
}

func (s *ArtifactStore) artifactsDir(wsID string) string {
	return filepath.Join(s.paths.WorkspacesDir(), wsID, "artifacts")
}

func (s *ArtifactStore) indexPath(wsID string) string {
	return filepath.Join(s.artifactsDir(wsID), "index.toml")
}

func (s *ArtifactStore) lockPath(wsID string) string {
	return filepath.Join(s.paths.RuntimeDir(), "locks", "artifacts-"+wsID+".lock")
}

func normalizeArtifactFormat(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(strings.TrimPrefix(raw, "."))) {
	case "html", "htm":
		return "html", nil
	case "md", "markdown", "":
		if strings.TrimSpace(raw) == "" {
			return "md", nil
		}
		return "md", nil
	default:
		return "", fmt.Errorf("%w: unsupported artifact format %q (expected html or md)", ErrInvalidID, raw)
	}
}

func normalizeArtifactLookupID(raw string) (id string, extHint string, err error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", fmt.Errorf("%w: empty artifact id", ErrInvalidID)
	}
	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasSuffix(lower, ".html"):
		trimmed = trimmed[:len(trimmed)-len(".html")]
		extHint = "html"
	case strings.HasSuffix(lower, ".md"):
		trimmed = trimmed[:len(trimmed)-len(".md")]
		extHint = "md"
	}
	if err := ValidateSafeID(trimmed); err != nil {
		return "", "", err
	}
	return trimmed, extHint, nil
}

// Save writes an HTML or Markdown artifact atomically under workspaces/<wsID>/artifacts/<id>.<format>
// and updates workspaces/<wsID>/artifacts/index.toml under Flock.
func (s *ArtifactStore) Save(wsID string, art Artifact) (*Artifact, error) {
	if err := ValidateSafeID(wsID); err != nil {
		return nil, fmt.Errorf("invalid workspace id: %w", err)
	}
	formatRaw := strings.TrimSpace(art.Format)
	if formatRaw == "" && art.FileName != "" {
		formatRaw = filepath.Ext(art.FileName)
	}
	format, err := normalizeArtifactFormat(formatRaw)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	id := strings.TrimSpace(art.ID)
	if id == "" && strings.TrimSpace(art.FileName) != "" {
		parsedID, _, nErr := normalizeArtifactLookupID(art.FileName)
		if nErr == nil {
			id = parsedID
		}
	}
	if id == "" {
		id = fmt.Sprintf("art_%d_%s", now.UnixMilli(), RandomHex(4))
	} else {
		parsedID, _, nErr := normalizeArtifactLookupID(id)
		if nErr != nil {
			return nil, fmt.Errorf("invalid artifact id: %w", nErr)
		}
		id = parsedID
	}

	if art.SessionID != "" {
		if err := ValidateSafeID(art.SessionID); err != nil {
			return nil, fmt.Errorf("invalid session id: %w", err)
		}
	}

	title := strings.TrimSpace(art.Title)
	if title == "" {
		title = id
	}
	fileName := id + "." + format
	artDir := s.artifactsDir(wsID)
	fullPath := filepath.Join(artDir, fileName)
	contentBytes := []byte(art.Content)

	var saved Artifact
	err = WithFlock(s.lockPath(wsID), true, func() error {
		if err := os.MkdirAll(artDir, 0o700); err != nil {
			return fmt.Errorf("create artifacts dir %s: %w", artDir, err)
		}
		idxMap, lErr := s.loadIndexMapUnlocked(wsID)
		if lErr != nil {
			return lErr
		}

		createdAt := now
		if prev, exists := idxMap[id]; exists && !prev.CreatedAt.IsZero() {
			createdAt = prev.CreatedAt
			// If format changed across overwrite, remove old file extension cleanly.
			if prev.FileName != "" && prev.FileName != fileName {
				_ = os.Remove(filepath.Join(artDir, prev.FileName))
			}
		} else if !art.CreatedAt.IsZero() {
			createdAt = art.CreatedAt.UTC()
		}

		if err := AtomicWriteFile(fullPath, contentBytes, 0o600); err != nil {
			return fmt.Errorf("write artifact %s: %w", fullPath, err)
		}

		meta := Artifact{
			ID:          id,
			WorkspaceID: wsID,
			SessionID:   art.SessionID,
			Title:       title,
			Format:      format,
			FileName:    fileName,
			Path:        fullPath,
			SizeBytes:   int64(len(contentBytes)),
			CreatedAt:   createdAt,
			UpdatedAt:   now,
		}
		idxMap[id] = meta
		if err := s.saveIndexMapUnlocked(wsID, idxMap); err != nil {
			return err
		}

		saved = meta
		saved.Content = art.Content
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

// List returns all HTML and Markdown artifacts in workspaces/<wsID>/artifacts/ sorted newest-first.
func (s *ArtifactStore) List(wsID string) ([]Artifact, error) {
	if err := ValidateSafeID(wsID); err != nil {
		return nil, fmt.Errorf("invalid workspace id: %w", err)
	}
	artDir := s.artifactsDir(wsID)
	if _, err := os.Stat(artDir); errors.Is(err, os.ErrNotExist) {
		return []Artifact{}, nil
	}

	var out []Artifact
	err := WithFlock(s.lockPath(wsID), false, func() error {
		merged, mErr := s.reconcileArtifactsUnlocked(wsID)
		if mErr != nil {
			return mErr
		}
		out = make([]Artifact, 0, len(merged))
		for _, item := range merged {
			out = append(out, item)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Get loads a single artifact (including Content) by ID or filename.
func (s *ArtifactStore) Get(wsID, idOrName string) (*Artifact, error) {
	if err := ValidateSafeID(wsID); err != nil {
		return nil, fmt.Errorf("invalid workspace id: %w", err)
	}
	id, extHint, err := normalizeArtifactLookupID(idOrName)
	if err != nil {
		return nil, err
	}

	var got Artifact
	err = WithFlock(s.lockPath(wsID), false, func() error {
		merged, mErr := s.reconcileArtifactsUnlocked(wsID)
		if mErr != nil {
			return mErr
		}
		meta, ok := merged[id]
		if !ok {
			return fmt.Errorf("%w: %s", ErrArtifactNotFound, id)
		}
		if extHint != "" && meta.Format != extHint {
			altPath := filepath.Join(s.artifactsDir(wsID), id+"."+extHint)
			if rawAlt, rErr := os.ReadFile(altPath); rErr == nil {
				meta.Format = extHint
				meta.FileName = id + "." + extHint
				meta.Path = altPath
				meta.SizeBytes = int64(len(rawAlt))
				meta.Content = string(rawAlt)
				got = meta
				return nil
			}
		}
		raw, rErr := os.ReadFile(meta.Path)
		if rErr != nil {
			if errors.Is(rErr, os.ErrNotExist) {
				return fmt.Errorf("%w: %s", ErrArtifactNotFound, id)
			}
			return fmt.Errorf("read artifact %s: %w", meta.Path, rErr)
		}
		meta.SizeBytes = int64(len(raw))
		meta.Content = string(raw)
		got = meta
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &got, nil
}

// Delete removes an artifact file and its index.toml entry.
func (s *ArtifactStore) Delete(wsID, idOrName string) error {
	if err := ValidateSafeID(wsID); err != nil {
		return fmt.Errorf("invalid workspace id: %w", err)
	}
	id, _, err := normalizeArtifactLookupID(idOrName)
	if err != nil {
		return err
	}

	return WithFlock(s.lockPath(wsID), true, func() error {
		artDir := s.artifactsDir(wsID)
		idxMap, lErr := s.loadIndexMapUnlocked(wsID)
		if lErr != nil {
			return lErr
		}

		removedAny := false
		if meta, ok := idxMap[id]; ok {
			if meta.FileName != "" {
				if err := os.Remove(filepath.Join(artDir, meta.FileName)); err == nil {
					removedAny = true
				} else if !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
			delete(idxMap, id)
			removedAny = true
		}

		for _, ext := range []string{".html", ".md"} {
			p := filepath.Join(artDir, id+ext)
			if err := os.Remove(p); err == nil {
				removedAny = true
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}

		if !removedAny {
			return fmt.Errorf("%w: %s", ErrArtifactNotFound, id)
		}
		if len(idxMap) == 0 {
			if _, stErr := os.Stat(s.indexPath(wsID)); errors.Is(stErr, os.ErrNotExist) {
				return nil
			}
		}
		return s.saveIndexMapUnlocked(wsID, idxMap)
	})
}

func (s *ArtifactStore) loadIndexMapUnlocked(wsID string) (map[string]Artifact, error) {
	out := make(map[string]Artifact)
	raw, err := os.ReadFile(s.indexPath(wsID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return nil, fmt.Errorf("read artifact index %s: %w", s.indexPath(wsID), err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return out, nil
	}
	var doc artifactIndexDoc
	if err := toml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse artifact index %s: %w", s.indexPath(wsID), err)
	}
	artDir := s.artifactsDir(wsID)
	for _, a := range doc.Artifacts {
		if ValidateSafeID(a.ID) != nil {
			continue
		}
		a.WorkspaceID = wsID
		if a.FileName == "" && a.Format != "" {
			a.FileName = a.ID + "." + a.Format
		}
		if a.FileName != "" {
			a.Path = filepath.Join(artDir, a.FileName)
		}
		out[a.ID] = a
	}
	return out, nil
}

func (s *ArtifactStore) saveIndexMapUnlocked(wsID string, idxMap map[string]Artifact) error {
	list := make([]Artifact, 0, len(idxMap))
	for _, a := range idxMap {
		cp := a
		cp.Content = ""
		list = append(list, cp)
	}
	sort.Slice(list, func(i, j int) bool {
		if !list[i].UpdatedAt.Equal(list[j].UpdatedAt) {
			return list[i].UpdatedAt.After(list[j].UpdatedAt)
		}
		return list[i].ID < list[j].ID
	})
	encoded, err := toml.Marshal(artifactIndexDoc{Artifacts: list})
	if err != nil {
		return fmt.Errorf("marshal artifact index: %w", err)
	}
	return AtomicWriteFile(s.indexPath(wsID), encoded, 0o600)
}

func (s *ArtifactStore) reconcileArtifactsUnlocked(wsID string) (map[string]Artifact, error) {
	artDir := s.artifactsDir(wsID)
	idxMap, err := s.loadIndexMapUnlocked(wsID)
	if err != nil {
		return nil, err
	}

	entries, rErr := os.ReadDir(artDir)
	if rErr != nil {
		if errors.Is(rErr, os.ErrNotExist) {
			return map[string]Artifact{}, nil
		}
		return nil, fmt.Errorf("readdir %s: %w", artDir, rErr)
	}

	merged := make(map[string]Artifact, len(entries))
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".html" && ext != ".md" {
			continue
		}
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		if ValidateSafeID(stem) != nil {
			continue
		}
		full := filepath.Join(artDir, name)
		info, iErr := ent.Info()
		if iErr != nil || !info.Mode().IsRegular() {
			continue
		}
		format := strings.TrimPrefix(ext, ".")
		if indexed, ok := idxMap[stem]; ok && indexed.Format == format {
			indexed.Path = full
			indexed.FileName = name
			indexed.SizeBytes = info.Size()
			merged[stem] = indexed
			continue
		}
		mod := info.ModTime().UTC()
		merged[stem] = Artifact{
			ID:          stem,
			WorkspaceID: wsID,
			Title:       stem,
			Format:      format,
			FileName:    name,
			Path:        full,
			SizeBytes:   info.Size(),
			CreatedAt:   mod,
			UpdatedAt:   mod,
		}
	}
	return merged, nil
}
