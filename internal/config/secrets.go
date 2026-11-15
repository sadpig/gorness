package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	ErrCmdSecretDisallowed = errors.New("cmd: secret resolution is forbidden in this scope")
	ErrEmptySecretRef      = errors.New("empty secret reference")
)

type KeychainFunc func(ctx context.Context, service string) (string, error)

type KeychainStore interface {
	GetSecret(ctx context.Context, service string) (string, error)
}

type SecretResolver interface {
	Resolve(ctx context.Context, ref string, allowCmd bool) (string, error)
}

type SecretResolverOptions struct {
	KeychainFunc      KeychainFunc
	KeychainStore     KeychainStore
	EnvLookup         func(key string) (string, bool)
	CmdRunner         func(ctx context.Context, shellCmd string) (string, error)
	CmdTTL            time.Duration
	Now               func() time.Time
	DefaultSourceFile string
}

type cachedSecret struct {
	value     string
	expiresAt time.Time
}

// DefaultSecretResolver 解析 keychain:/env:/cmd:/明文凭据；明文凭据会自动收紧来源配置文件权限为 0600。
type DefaultSecretResolver struct {
	keychainFunc      KeychainFunc
	keychainStore     KeychainStore
	envLookup         func(key string) (string, bool)
	cmdRunner         func(ctx context.Context, shellCmd string) (string, error)
	cmdTTL            time.Duration
	now               func() time.Time
	defaultSourceFile string

	mu            sync.Mutex
	cmdCache      map[string]cachedSecret
	literalSource map[string]string
	knownSecrets  map[string]struct{}
}

func NewSecretResolver(opts SecretResolverOptions) *DefaultSecretResolver {
	envLookup := opts.EnvLookup
	if envLookup == nil {
		envLookup = os.LookupEnv
	}
	cmdRunner := opts.CmdRunner
	if cmdRunner == nil {
		cmdRunner = defaultShellRunner
	}
	ttl := opts.CmdTTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	return &DefaultSecretResolver{
		keychainFunc:      opts.KeychainFunc,
		keychainStore:     opts.KeychainStore,
		envLookup:         envLookup,
		cmdRunner:         cmdRunner,
		cmdTTL:            ttl,
		now:               nowFn,
		defaultSourceFile: opts.DefaultSourceFile,
		cmdCache:          make(map[string]cachedSecret),
		literalSource:     make(map[string]string),
		knownSecrets:      make(map[string]struct{}),
	}
}

func (r *DefaultSecretResolver) RegisterLiteralSource(literal, sourceFile string) {
	if strings.TrimSpace(literal) == "" || sourceFile == "" {
		return
	}
	r.mu.Lock()
	r.literalSource[literal] = sourceFile
	r.mu.Unlock()
}

func (r *DefaultSecretResolver) KnownSecrets() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.knownSecrets))
	for s := range r.knownSecrets {
		out = append(out, s)
	}
	return out
}

func (r *DefaultSecretResolver) rememberSecret(val string) {
	if strings.TrimSpace(val) == "" {
		return
	}
	r.mu.Lock()
	r.knownSecrets[val] = struct{}{}
	r.mu.Unlock()
}

func (r *DefaultSecretResolver) Resolve(ctx context.Context, ref string, allowCmd bool) (string, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return "", ErrEmptySecretRef
	}

	switch {
	case strings.HasPrefix(trimmed, "env:"):
		varName := strings.TrimSpace(strings.TrimPrefix(trimmed, "env:"))
		if varName == "" {
			return "", errors.New("empty env: variable name")
		}
		val, ok := r.envLookup(varName)
		if !ok || strings.TrimSpace(val) == "" {
			return "", fmt.Errorf("environment variable %q is not set", varName)
		}
		r.rememberSecret(val)
		return val, nil

	case strings.HasPrefix(trimmed, "keychain:"):
		service := strings.TrimSpace(strings.TrimPrefix(trimmed, "keychain:"))
		if service == "" {
			return "", errors.New("empty keychain: service name")
		}
		val, err := r.resolveKeychain(ctx, service)
		if err != nil {
			return "", err
		}
		r.rememberSecret(val)
		return val, nil

	case strings.HasPrefix(trimmed, "cmd:"):
		if !allowCmd {
			return "", ErrCmdSecretDisallowed
		}
		shellCmd := strings.TrimSpace(strings.TrimPrefix(trimmed, "cmd:"))
		if shellCmd == "" {
			return "", errors.New("empty cmd: command")
		}
		return r.resolveCmd(ctx, shellCmd)

	default:
		r.mu.Lock()
		src := r.literalSource[trimmed]
		if src == "" {
			src = r.defaultSourceFile
		}
		r.mu.Unlock()
		if src != "" {
			tightenFilePerm0600(src)
		}
		r.rememberSecret(trimmed)
		return trimmed, nil
	}
}

func (r *DefaultSecretResolver) resolveKeychain(ctx context.Context, service string) (string, error) {
	if r.keychainStore != nil {
		val, err := r.keychainStore.GetSecret(ctx, service)
		if err != nil {
			return "", fmt.Errorf("keychain lookup %q: %w", service, err)
		}
		return strings.TrimRight(val, "\r\n"), nil
	}
	if r.keychainFunc != nil {
		val, err := r.keychainFunc(ctx, service)
		if err != nil {
			return "", fmt.Errorf("keychain lookup %q: %w", service, err)
		}
		return strings.TrimRight(val, "\r\n"), nil
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", service, "-w")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("macos security keychain lookup %q: %w", service, err)
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

func (r *DefaultSecretResolver) resolveCmd(ctx context.Context, shellCmd string) (string, error) {
	now := r.now()
	r.mu.Lock()
	if entry, ok := r.cmdCache[shellCmd]; ok && now.Before(entry.expiresAt) {
		r.mu.Unlock()
		return entry.value, nil
	}
	r.mu.Unlock()

	rawOut, err := r.cmdRunner(ctx, shellCmd)
	if err != nil {
		return "", fmt.Errorf("cmd: credential command failed: %w", err)
	}
	val := strings.TrimRight(rawOut, "\r\n")
	if strings.TrimSpace(val) == "" {
		return "", errors.New("cmd: credential command returned empty output")
	}

	r.mu.Lock()
	r.cmdCache[shellCmd] = cachedSecret{
		value:     val,
		expiresAt: r.now().Add(r.cmdTTL),
	}
	r.knownSecrets[val] = struct{}{}
	r.mu.Unlock()
	return val, nil
}

func defaultShellRunner(ctx context.Context, shellCmd string) (string, error) {
	cmd := exec.CommandContext(ctx, "/bin/zsh", "-c", shellCmd)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func isDynamicSecretRef(ref string) bool {
	trimmed := strings.TrimSpace(ref)
	return strings.HasPrefix(trimmed, "env:") ||
		strings.HasPrefix(trimmed, "keychain:") ||
		strings.HasPrefix(trimmed, "cmd:")
}

// tightenFilePerm0600 仅在文件权限包含组或其他用户位时执行 chmod 0600，避免触发无意义的 fsnotify Chmod 循环。
func tightenFilePerm0600(path string) {
	if path == "" {
		return
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return
	}
	if st.Mode().Perm() != 0o600 {
		_ = os.Chmod(path, 0o600)
	}
}
