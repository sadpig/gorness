package store

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
)

var fallbackNonce atomic.Uint64

var (
	ErrWorkspaceNotFound = errors.New("workspace not found")
	ErrSessionNotFound   = errors.New("session not found")
	ErrInvalidPath       = errors.New("invalid or escaping path")
	ErrInvalidID         = errors.New("invalid identifier")
)

// AtomicWriteFile 通过“同目录临时文件写入 -> Chmod -> Sync -> APFS 原子 Rename”确保配置文件与元数据绝不出现半写撕裂态。
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%w: empty file path", ErrInvalidPath)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create parent dir %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmpName)
		}
	}()

	if err := writeAll(tmp, data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file %s: %w", tmpName, err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("atomic rename %s -> %s: %w", tmpName, path, err)
	}
	renamed = true
	return nil
}

// WithFlock 使用 macOS 内核级 syscall.Flock 执行阻塞共享锁或排他锁保护的临界区；进程崩溃时内核自动释放文件锁。
func WithFlock(lockPath string, exclusive bool, fn func() error) error {
	if strings.TrimSpace(lockPath) == "" {
		return fmt.Errorf("%w: empty lock path", ErrInvalidPath)
	}
	if fn == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return fmt.Errorf("create lock dir for %s: %w", lockPath, err)
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open lock file %s: %w", lockPath, err)
	}

	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}
	if err := flockWithRetry(int(f.Fd()), how); err != nil {
		_ = f.Close()
		return fmt.Errorf("acquire flock %s: %w", lockPath, err)
	}
	defer func() {
		_ = flockWithRetry(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		runtime.KeepAlive(f)
	}()

	return fn()
}

// TryFlock 非阻塞尝试获取排他文件锁；若已被其他协程或进程持有则立即返回 (nil, false, nil)。
func TryFlock(lockPath string) (unlock func(), ok bool, err error) {
	if strings.TrimSpace(lockPath) == "" {
		return nil, false, fmt.Errorf("%w: empty lock path", ErrInvalidPath)
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return nil, false, fmt.Errorf("create lock dir for %s: %w", lockPath, err)
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, fmt.Errorf("open lock file %s: %w", lockPath, err)
	}

	if err := flockWithRetry(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("try flock %s: %w", lockPath, err)
	}

	var once sync.Once
	release := func() {
		once.Do(func() {
			_ = flockWithRetry(int(f.Fd()), syscall.LOCK_UN)
			_ = f.Close()
			runtime.KeepAlive(f)
		})
	}
	return release, true, nil
}

func flockWithRetry(fd int, how int) error {
	for {
		err := syscall.Flock(fd, how)
		if !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}

func writeAll(f *os.File, data []byte) error {
	for len(data) > 0 {
		n, err := f.Write(data)
		if err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

// MarshalJSONLine encodes v as a single JSONL line with SetEscapeHTML(false) so <, >, and & bytes round-trip verbatim.
func MarshalJSONLine(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func marshalJSONLine(v any) ([]byte, error) {
	return MarshalJSONLine(v)
}

// AppendJSONLine appends v as an unescaped JSONL line to path with fsync durability and 0600 permissions.
func AppendJSONLine(path string, v any) error {
	line, err := MarshalJSONLine(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create dir for %s: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open append %s: %w", path, err)
	}
	wErr := writeAll(f, line)
	sErr := f.Sync()
	cErr := f.Close()
	if wErr != nil {
		return fmt.Errorf("append %s: %w", path, wErr)
	}
	if sErr != nil {
		return fmt.Errorf("sync %s: %w", path, sErr)
	}
	if cErr != nil {
		return fmt.Errorf("close %s: %w", path, cErr)
	}
	return nil
}

func appendJSONLine(path string, v any) error {
	return AppendJSONLine(path, v)
}

// ValidateSafeID rejects empty, whitespace-padded, path-separating, dot, or control-character identifiers.
func ValidateSafeID(id string) error {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" || trimmed != id || id == "." || id == ".." {
		return fmt.Errorf("%w: %q", ErrInvalidID, id)
	}
	for _, r := range id {
		if r == '/' || r == '\\' || r < 0x20 || r == 0x7f {
			return fmt.Errorf("%w: %q", ErrInvalidID, id)
		}
	}
	return nil
}

func validateSafeID(id string) error {
	return ValidateSafeID(id)
}

// RandomHex generates a cryptographically random lowercase hex string of length 2*numBytes.
func RandomHex(numBytes int) string {
	if numBytes <= 0 {
		numBytes = 6
	}
	b := make([]byte, numBytes)
	if _, err := rand.Read(b); err != nil {
		n := fallbackNonce.Add(1)
		return fmt.Sprintf("%012x", n)
	}
	return hex.EncodeToString(b)
}

func randHex(numBytes int) string {
	return RandomHex(numBytes)
}

func isWithinOrEqualPath(path, root string) bool {
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
