package store

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gorness.dev/gorness/internal/config"
)

type ChangeKind string

const (
	ChangeCreate ChangeKind = "create"
	ChangeModify ChangeKind = "modify"
	ChangeDelete ChangeKind = "delete"
)

type FileSnapshotRecord struct {
	Seq        uint64     `json:"seq"`
	SessionID  string     `json:"session_id"`
	RelPath    string     `json:"rel_path"`
	Kind       ChangeKind `json:"kind"`
	BeforeHash string     `json:"before_hash,omitempty"`
	AfterHash  string     `json:"after_hash,omitempty"`
}

// SnapshotStore 基于 SHA-256 内容寻址块与按会话追加账本支撑 /rewind 毫秒级精确回滚源码。
type SnapshotStore struct {
	paths config.VaultPaths
}

func NewSnapshotStore(paths config.VaultPaths) (*SnapshotStore, error) {
	if strings.TrimSpace(paths.Root) == "" {
		return nil, fmt.Errorf("%w: empty vault root", ErrInvalidPath)
	}
	s := &SnapshotStore{paths: paths}
	for _, dir := range []string{
		paths.SnapshotsDir(),
		s.blobsRoot(),
		s.ledgersRoot(),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create snapshot dir %s: %w", dir, err)
		}
	}
	return s, nil
}

func (s *SnapshotStore) blobsRoot() string {
	return filepath.Join(s.paths.SnapshotsDir(), "blobs")
}

func (s *SnapshotStore) ledgersRoot() string {
	return filepath.Join(s.paths.SnapshotsDir(), "ledgers")
}

func (s *SnapshotStore) blobPath(sha256Hex string) string {
	return filepath.Join(s.blobsRoot(), sha256Hex[:2], sha256Hex)
}

func (s *SnapshotStore) ledgerPath(sessionID string) string {
	return filepath.Join(s.ledgersRoot(), sessionID+".jsonl")
}

func (s *SnapshotStore) ledgerLockPath(sessionID string) string {
	return filepath.Join(s.paths.RuntimeDir(), "locks", "snap-"+sessionID+".lock")
}

func (s *SnapshotStore) PutBlob(data []byte) (sha256Hex string, err error) {
	sum := sha256.Sum256(data)
	sha256Hex = hex.EncodeToString(sum[:])
	target := s.blobPath(sha256Hex)

	if st, statErr := os.Stat(target); statErr == nil && st.Mode().IsRegular() && st.Size() == int64(len(data)) {
		return sha256Hex, nil
	}
	if err := AtomicWriteFile(target, data, 0o600); err != nil {
		return "", fmt.Errorf("store snapshot blob %s: %w", sha256Hex, err)
	}
	return sha256Hex, nil
}

func (s *SnapshotStore) GetBlob(sha256Hex string) ([]byte, error) {
	norm := strings.ToLower(strings.TrimSpace(sha256Hex))
	if !isValidSHA256Hex(norm) {
		return nil, fmt.Errorf("%w: invalid sha256 digest %q", ErrInvalidID, sha256Hex)
	}
	raw, err := os.ReadFile(s.blobPath(norm))
	if err != nil {
		return nil, fmt.Errorf("read snapshot blob %s: %w", norm, err)
	}
	actual := sha256.Sum256(raw)
	if hex.EncodeToString(actual[:]) != norm {
		return nil, fmt.Errorf("snapshot blob integrity mismatch for %s", norm)
	}
	return raw, nil
}

func (s *SnapshotStore) CaptureFile(absPath string) (sha256Hex string, exists bool, err error) {
	if strings.TrimSpace(absPath) == "" {
		return "", false, fmt.Errorf("%w: empty file path", ErrInvalidPath)
	}
	st, err := os.Lstat(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("lstat %s: %w", absPath, err)
	}
	if !st.Mode().IsRegular() {
		return "", true, fmt.Errorf("%w: cannot capture non-regular file %s", ErrInvalidPath, absPath)
	}

	raw, err := os.ReadFile(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", true, fmt.Errorf("read %s: %w", absPath, err)
	}
	hash, err := s.PutBlob(raw)
	if err != nil {
		return "", true, err
	}
	return hash, true, nil
}

func (s *SnapshotStore) RecordStepChange(rec FileSnapshotRecord) error {
	if err := validateSafeID(rec.SessionID); err != nil {
		return fmt.Errorf("invalid session id in snapshot record: %w", err)
	}
	normRel, err := validateLexicalRelPath(rec.RelPath)
	if err != nil {
		return err
	}
	rec.RelPath = normRel

	switch rec.Kind {
	case ChangeCreate:
		if rec.AfterHash != "" && !isValidSHA256Hex(strings.ToLower(rec.AfterHash)) {
			return fmt.Errorf("%w: invalid after_hash %q", ErrInvalidID, rec.AfterHash)
		}
	case ChangeModify, ChangeDelete:
		rec.BeforeHash = strings.ToLower(strings.TrimSpace(rec.BeforeHash))
		if !isValidSHA256Hex(rec.BeforeHash) {
			return fmt.Errorf("%w: %s requires valid 64-hex before_hash, got %q", ErrInvalidID, rec.Kind, rec.BeforeHash)
		}
		if rec.AfterHash != "" && !isValidSHA256Hex(strings.ToLower(rec.AfterHash)) {
			return fmt.Errorf("%w: invalid after_hash %q", ErrInvalidID, rec.AfterHash)
		}
	default:
		return fmt.Errorf("unknown snapshot change kind %q", rec.Kind)
	}

	return WithFlock(s.ledgerLockPath(rec.SessionID), true, func() error {
		return appendJSONLine(s.ledgerPath(rec.SessionID), rec)
	})
}

// RestoreSessionAfterSeq 按逆序 Seq 回放并撤销所有 Seq > targetSeq 的源码变更：
// 1. 先解析规范化工作区根目录 canonWs 并禁止逃逸至工作区外或 ~/.gorness/；
// 2. 逐路径段做 Lstat 符号链接穿透校验，拒绝绝对路径、".." 段与越界软链；
// 3. 严格按 Seq 降序（同 Seq 按记录逆序）回滚，使同一文件的多次修改准确退回 targetSeq 时刻字节态。
func (s *SnapshotStore) RestoreSessionAfterSeq(sessionID string, targetSeq uint64, wsRoot string) ([]string, error) {
	if err := validateSafeID(sessionID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(wsRoot) == "" {
		return nil, fmt.Errorf("%w: workspace root is required", ErrInvalidPath)
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
	if st, sErr := os.Stat(canonWs); sErr != nil || !st.IsDir() {
		return nil, fmt.Errorf("%w: workspace root %s is not a directory", ErrInvalidPath, canonWs)
	}

	canonVault := filepath.Clean(s.paths.Root)
	if resolvedVault, vErr := filepath.EvalSymlinks(s.paths.Root); vErr == nil {
		canonVault = filepath.Clean(resolvedVault)
	}
	if isWithinOrEqualPath(canonWs, canonVault) {
		return nil, fmt.Errorf("%w: workspace %s overlaps gorness vault %s", ErrInvalidPath, canonWs, canonVault)
	}

	var restored []string
	err = WithFlock(s.ledgerLockPath(sessionID), true, func() error {
		records, rErr := readSnapshotLedger(s.ledgerPath(sessionID))
		if rErr != nil {
			if errors.Is(rErr, os.ErrNotExist) {
				return nil
			}
			return rErr
		}

		type indexedRec struct {
			idx int
			rec FileSnapshotRecord
		}
		var keep []FileSnapshotRecord
		var revert []indexedRec
		for idx, rec := range records {
			if rec.Seq <= targetSeq {
				keep = append(keep, rec)
			} else {
				revert = append(revert, indexedRec{idx: idx, rec: rec})
			}
		}
		if len(revert) == 0 {
			return nil
		}

		sort.Slice(revert, func(i, j int) bool {
			if revert[i].rec.Seq != revert[j].rec.Seq {
				return revert[i].rec.Seq > revert[j].rec.Seq
			}
			return revert[i].idx > revert[j].idx
		})

		type preparedOp struct {
			rec         FileSnapshotRecord
			normRel     string
			beforeBytes []byte
		}
		ops := make([]preparedOp, 0, len(revert))
		for _, item := range revert {
			_, normRel, pErr := resolveSafeRestoreTarget(canonWs, canonVault, item.rec.RelPath)
			if pErr != nil {
				return pErr
			}
			var blob []byte
			switch item.rec.Kind {
			case ChangeCreate:
				// Undoing creation removes the file.
			case ChangeModify, ChangeDelete:
				b, bErr := s.GetBlob(item.rec.BeforeHash)
				if bErr != nil {
					return fmt.Errorf("load before_hash for %s at seq %d: %w", normRel, item.rec.Seq, bErr)
				}
				blob = b
			default:
				return fmt.Errorf("unsupported change kind %q in snapshot ledger", item.rec.Kind)
			}
			ops = append(ops, preparedOp{
				rec:         item.rec,
				normRel:     normRel,
				beforeBytes: blob,
			})
		}

		seenRel := make(map[string]struct{}, len(ops))
		for _, op := range ops {
			targetAbs, _, pErr := resolveSafeRestoreTarget(canonWs, canonVault, op.rec.RelPath)
			if pErr != nil {
				return pErr
			}

			switch op.rec.Kind {
			case ChangeCreate:
				if rmErr := os.Remove(targetAbs); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
					return fmt.Errorf("remove created file %s: %w", op.normRel, rmErr)
				}
				pruneEmptyDirsUpTo(filepath.Dir(targetAbs), canonWs)
			case ChangeModify, ChangeDelete:
				perm := os.FileMode(0o644)
				if st, stErr := os.Lstat(targetAbs); stErr == nil && st.Mode().IsRegular() {
					perm = st.Mode().Perm()
				}
				if wErr := AtomicWriteFile(targetAbs, op.beforeBytes, perm); wErr != nil {
					return fmt.Errorf("restore file %s: %w", op.normRel, wErr)
				}
			}

			if _, exists := seenRel[op.normRel]; !exists {
				seenRel[op.normRel] = struct{}{}
				restored = append(restored, op.normRel)
			}
		}

		// Atomically rewrite the ledger keeping only records at or before targetSeq.
		var buf bytes.Buffer
		for _, k := range keep {
			line, mErr := marshalJSONLine(k)
			if mErr != nil {
				return mErr
			}
			buf.Write(line)
		}
		return AtomicWriteFile(s.ledgerPath(sessionID), buf.Bytes(), 0o600)
	})
	if err != nil {
		return nil, err
	}
	return restored, nil
}

func readSnapshotLedger(path string) ([]FileSnapshotRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []FileSnapshotRecord
	r := bufio.NewReader(f)
	for {
		line, rErr := r.ReadBytes('\n')
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) > 0 {
			var rec FileSnapshotRecord
			if uErr := json.Unmarshal(trimmed, &rec); uErr != nil {
				return nil, fmt.Errorf("parse snapshot ledger %s: %w", path, uErr)
			}
			out = append(out, rec)
		}
		if errors.Is(rErr, io.EOF) {
			break
		}
		if rErr != nil {
			return nil, fmt.Errorf("read snapshot ledger %s: %w", path, rErr)
		}
	}
	return out, nil
}

func validateLexicalRelPath(relPath string) (string, error) {
	if strings.TrimSpace(relPath) == "" || strings.ContainsRune(relPath, 0) {
		return "", fmt.Errorf("%w: empty or NUL-containing relative path %q", ErrInvalidPath, relPath)
	}
	if strings.HasPrefix(relPath, "/") || strings.HasPrefix(relPath, "\\") || filepath.IsAbs(relPath) {
		return "", fmt.Errorf("%w: relative path must not be absolute: %q", ErrInvalidPath, relPath)
	}
	slash := strings.ReplaceAll(relPath, "\\", "/")
	for _, seg := range strings.Split(slash, "/") {
		if seg == ".." {
			return "", fmt.Errorf("%w: parent traversal disallowed in %q", ErrInvalidPath, relPath)
		}
	}
	clean := filepath.Clean(filepath.FromSlash(slash))
	if clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: invalid relative path %q", ErrInvalidPath, relPath)
	}
	return filepath.ToSlash(clean), nil
}

func resolveSafeRestoreTarget(canonWs, canonVault, relPath string) (targetAbs string, normRel string, err error) {
	normRel, err = validateLexicalRelPath(relPath)
	if err != nil {
		return "", "", err
	}
	cleanOSRel := filepath.FromSlash(normRel)
	parts := strings.Split(cleanOSRel, string(filepath.Separator))

	cur := canonWs
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", "", fmt.Errorf("%w: invalid path segment %q in %q", ErrInvalidPath, part, relPath)
		}
		next := filepath.Join(cur, part)
		isLeaf := i == len(parts)-1

		st, lErr := os.Lstat(next)
		if lErr == nil {
			if st.Mode()&os.ModeSymlink != 0 {
				if isLeaf {
					return "", "", fmt.Errorf("%w: refusing to overwrite or remove symlink leaf %q", ErrInvalidPath, relPath)
				}
				resolvedDir, eErr := filepath.EvalSymlinks(next)
				if eErr != nil {
					return "", "", fmt.Errorf("%w: unresolvable symlink %q: %v", ErrInvalidPath, relPath, eErr)
				}
				resolvedDir = filepath.Clean(resolvedDir)
				if !isStrictlyWithinPath(resolvedDir, canonWs) {
					return "", "", fmt.Errorf("%w: symlink in %q escapes workspace root", ErrInvalidPath, relPath)
				}
				if canonVault != "" && isWithinOrEqualPath(resolvedDir, canonVault) {
					return "", "", fmt.Errorf("%w: symlink in %q enters gorness vault", ErrInvalidPath, relPath)
				}
				cur = resolvedDir
				continue
			}
			if !isLeaf && !st.IsDir() {
				return "", "", fmt.Errorf("%w: non-directory parent in %q", ErrInvalidPath, relPath)
			}
		} else if !errors.Is(lErr, os.ErrNotExist) {
			return "", "", fmt.Errorf("lstat %s: %w", next, lErr)
		}
		cur = next
	}

	cur = filepath.Clean(cur)
	if !isStrictlyWithinPath(cur, canonWs) {
		return "", "", fmt.Errorf("%w: resolved target %q escapes workspace", ErrInvalidPath, relPath)
	}
	if canonVault != "" && isWithinOrEqualPath(cur, canonVault) {
		return "", "", fmt.Errorf("%w: resolved target %q enters vault", ErrInvalidPath, relPath)
	}
	return cur, normRel, nil
}

func pruneEmptyDirsUpTo(dir, stopRoot string) {
	cur := filepath.Clean(dir)
	stop := filepath.Clean(stopRoot)
	for isStrictlyWithinPath(cur, stop) {
		if err := os.Remove(cur); err != nil {
			return
		}
		cur = filepath.Dir(cur)
	}
}

func isValidSHA256Hex(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
