package daemonctl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/pelletier/go-toml/v2"
	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/store"
)

const (
	DefaultHost        = "127.0.0.1"
	DefaultPort        = 3456
	DefaultLoopbackURL = "http://127.0.0.1:3456"
	UDSBaseURL         = "http://localhost"

	defaultProbeTimeout = 700 * time.Millisecond
	defaultEnsureWait   = 6 * time.Second
	defaultStopWait     = 3 * time.Second
	udsDialProbeTimeout = 250 * time.Millisecond
)

var (
	ErrInvalidRuntimeMetadata = errors.New("invalid daemon.toml metadata")
	ErrVaultMismatch          = errors.New("daemon vault_root mismatch")
	ErrDaemonStartTimeout     = errors.New("timed out waiting for gorness daemon to become healthy")
)

// RuntimeRecord is the canonical daemon metadata stored at ~/.gorness/runtime/daemon.toml.
type RuntimeRecord struct {
	PID        int       `toml:"pid" json:"pid"`
	VaultRoot  string    `toml:"vault_root" json:"vault_root"`
	Host       string    `toml:"host" json:"host"`
	Port       int       `toml:"port" json:"port"`
	URL        string    `toml:"url" json:"url"`
	SocketPath string    `toml:"socket_path" json:"socket_path"`
	Version    string    `toml:"version" json:"version"`
	StartedAt  time.Time `toml:"started_at" json:"started_at"`
}

// HealthStatus mirrors GET /api/health returned by the Gorness loopback server.
type HealthStatus struct {
	OK               bool      `json:"ok"`
	Version          string    `json:"version"`
	PID              int       `json:"pid"`
	VaultRoot        string    `json:"vault_root"`
	ActiveRuns       int       `json:"active_runs"`
	PendingApprovals int       `json:"pending_approvals"`
	Timestamp        time.Time `json:"timestamp"`
}

// Endpoint describes a verified running Gorness daemon and provides UDS-first / TCP-fallback HTTP clients.
type Endpoint struct {
	// BaseURL is "http://localhost" when Unix Domain Socket IPC is active, or the loopback TCP URL otherwise.
	BaseURL string
	// WebURL is always the loopback TCP HTTP URL (e.g. "http://127.0.0.1:3456") suitable for browsers & WKWebView.
	WebURL     string
	SocketPath string
	Record     RuntimeRecord
}

// EnsureOptions configures background daemon discovery and auto-spawning.
type EnsureOptions struct {
	VaultRoot    string
	DaemonBinary string
	HomeDir      string
	EnrichPATH   bool
	WaitTimeout  time.Duration
	ExtraArgs    []string
	Env          []string
	// Starter optionally overrides process creation in tests.
	Starter func(bin string, args []string, logFile *os.File, env []string) error
}

func RuntimeRecordPath(paths config.VaultPaths) string {
	return filepath.Join(paths.RuntimeDir(), "daemon.toml")
}

func DaemonLockPath(paths config.VaultPaths) string {
	return filepath.Join(paths.RuntimeDir(), "locks", "daemon.lock")
}

func DaemonLogPath(paths config.VaultPaths) string {
	return filepath.Join(paths.LogsDir(), "daemon.log")
}

// ResolveVaultRoot resolves the active ~/.gorness vault directory from CLI flag, GORNESS_VAULT env, or userHome,
// expanding "~" / "~/..." consistently across CLI, TUI, and macOS desktop app.
func ResolveVaultRoot(flagVal string, env map[string]string, userHome string) string {
	home := strings.TrimSpace(userHome)
	if home == "" {
		home, _ = os.UserHomeDir()
	}

	if trimmed := strings.TrimSpace(flagVal); trimmed != "" {
		return filepath.Clean(config.ExpandHome(trimmed, home))
	}

	var envVal string
	if env != nil {
		envVal = strings.TrimSpace(env["GORNESS_VAULT"])
	} else {
		envVal = strings.TrimSpace(os.Getenv("GORNESS_VAULT"))
	}
	if envVal != "" {
		return filepath.Clean(config.ExpandHome(envVal, home))
	}

	if home != "" {
		return filepath.Join(filepath.Clean(home), ".gorness")
	}
	return filepath.Join(os.TempDir(), ".gorness")
}

// CanonicalVaultPath resolves symlinks segment-by-segment (handling macOS /var -> /private/var even when
// trailing directories do not exist yet) via config.CanonicalPath.
func CanonicalVaultPath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	expanded := config.ExpandHome(trimmed, "")
	if canon, err := config.CanonicalPath(expanded); err == nil && canon != "" {
		return canon
	}
	if abs, err := filepath.Abs(expanded); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(expanded)
}

// VaultRootsMatch reports whether two vault root paths resolve to the exact same canonical path.
func VaultRootsMatch(a, b string) bool {
	ca := CanonicalVaultPath(a)
	cb := CanonicalVaultPath(b)
	return ca != "" && cb != "" && ca == cb
}

// PathsOverlapVault reports whether wsPath contains vaultRoot or lies inside vaultRoot.
func PathsOverlapVault(wsPath, vaultRoot string) bool {
	canonWS := CanonicalVaultPath(wsPath)
	canonVault := CanonicalVaultPath(vaultRoot)
	if canonWS == "" || canonVault == "" {
		return false
	}
	return config.IsWithinRoot(canonWS, canonVault) || config.IsWithinRoot(canonVault, canonWS)
}

// EnrichDesktopPATH prepends standard macOS developer toolchain directories without duplicates
// so GUI-spawned daemons (Finder/Dock/LaunchServices) can locate git, go, node, and rg.
func EnrichDesktopPATH(currentPATH, homeDir string) string {
	priority := []string{
		"/opt/homebrew/bin",
		"/opt/homebrew/sbin",
		"/usr/local/bin",
	}
	if strings.TrimSpace(homeDir) != "" {
		priority = append(priority,
			filepath.Join(homeDir, "go", "bin"),
			filepath.Join(homeDir, ".local", "bin"),
		)
	}
	priority = append(priority, "/usr/bin", "/bin", "/usr/sbin", "/sbin")

	seen := make(map[string]bool, len(priority)+8)
	var out []string
	add := func(dir string) {
		d := strings.TrimSpace(dir)
		if d == "" || seen[d] {
			return
		}
		seen[d] = true
		out = append(out, d)
	}
	for _, p := range priority {
		add(p)
	}
	for _, p := range strings.Split(currentPATH, ":") {
		add(p)
	}
	return strings.Join(out, ":")
}

// BundledDaemonBinary returns the sibling gorness-engine path inside Gorness.app/Contents/MacOS.
func BundledDaemonBinary(appExecutablePath string) string {
	return filepath.Join(filepath.Dir(appExecutablePath), "gorness-engine")
}

// WriteRuntimeRecord atomically persists RuntimeRecord to runtime/daemon.toml with 0600 permissions.
func WriteRuntimeRecord(paths config.VaultPaths, rec RuntimeRecord) error {
	if strings.TrimSpace(rec.VaultRoot) == "" {
		rec.VaultRoot = CanonicalVaultPath(paths.Root)
	}
	if strings.TrimSpace(rec.Version) == "" {
		rec.Version = config.Version
	}
	if rec.StartedAt.IsZero() {
		rec.StartedAt = time.Now().UTC()
	}
	raw, err := toml.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal daemon.toml: %w", err)
	}
	return store.AtomicWriteFile(RuntimeRecordPath(paths), raw, 0o600)
}

// ReadRuntimeRecord reads and parses runtime/daemon.toml for paths.
func ReadRuntimeRecord(paths config.VaultPaths) (RuntimeRecord, error) {
	raw, err := os.ReadFile(RuntimeRecordPath(paths))
	if err != nil {
		return RuntimeRecord{}, err
	}
	var rec RuntimeRecord
	if err := toml.Unmarshal(raw, &rec); err != nil {
		return RuntimeRecord{}, fmt.Errorf("%w: %v", ErrInvalidRuntimeMetadata, err)
	}
	return rec, nil
}

// RemoveRuntimeRecord deletes runtime/daemon.toml if present.
func RemoveRuntimeRecord(paths config.VaultPaths) error {
	err := os.Remove(RuntimeRecordPath(paths))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// IsUnixSocketReachable checks whether a Unix Domain Socket exists and accepts connections.
func IsUnixSocketReachable(socketPath string, timeout time.Duration) bool {
	sock := strings.TrimSpace(socketPath)
	if sock == "" {
		return false
	}
	if timeout <= 0 {
		timeout = udsDialProbeTimeout
	}
	conn, err := net.DialTimeout("unix", sock, timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// NewTransport constructs an http.Transport that prefers dialing socketPath (Unix Domain Socket)
// whenever targeting the daemon host ("localhost:80" or fallbackURL's host:port), and falls back
// seamlessly to loopback TCP fallbackURL if the socket is absent or unreachable.
func NewTransport(socketPath, fallbackURL string) *http.Transport {
	cleanSock := strings.TrimSpace(socketPath)
	fallbackHostPort := parseHostPortFromURL(fallbackURL)

	dialer := &net.Dialer{
		Timeout:   2 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	return &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          32,
		IdleConnTimeout:       60 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			isDaemonTarget := strings.EqualFold(addr, "localhost:80") ||
				strings.EqualFold(addr, "localhost") ||
				(fallbackHostPort != "" && strings.EqualFold(addr, fallbackHostPort))

			if isDaemonTarget && cleanSock != "" {
				uConn, uErr := dialer.DialContext(ctx, "unix", cleanSock)
				if uErr == nil {
					return uConn, nil
				}
				if fallbackHostPort != "" {
					return dialer.DialContext(ctx, "tcp", fallbackHostPort)
				}
				return nil, uErr
			}

			if strings.EqualFold(addr, "localhost:80") && fallbackHostPort != "" {
				return dialer.DialContext(ctx, "tcp", fallbackHostPort)
			}
			return dialer.DialContext(ctx, network, addr)
		},
	}
}

// NewHTTPClient builds a daemon HTTP client and returns its canonical base URL ("http://localhost" when
// socketPath is reachable, otherwise the normalized loopback TCP fallbackURL).
func NewHTTPClient(socketPath, fallbackURL string, timeout time.Duration) (*http.Client, string) {
	cleanSock := strings.TrimSpace(socketPath)
	cleanFallback := normalizeHTTPBaseURL(fallbackURL)

	baseURL := cleanFallback
	if cleanSock != "" && IsUnixSocketReachable(cleanSock, udsDialProbeTimeout) {
		baseURL = UDSBaseURL
	} else if baseURL == "" {
		if cleanSock != "" {
			baseURL = UDSBaseURL
		} else {
			baseURL = DefaultLoopbackURL
		}
	}

	return &http.Client{
		Transport: NewTransport(cleanSock, cleanFallback),
		Timeout:   timeout,
	}, baseURL
}

// ClientForURL constructs an HTTP client and normalized base URL for a given serverURL ("unix:///...",
// "http://localhost", or "http://127.0.0.1:PORT"), attaching UDS transport automatically when vaultRoot's
// daemon.toml matches serverURL.
func ClientForURL(serverURL, vaultRoot string, timeout time.Duration) (*http.Client, string) {
	trimmed := strings.TrimSpace(serverURL)
	if strings.HasPrefix(strings.ToLower(trimmed), "unix://") {
		sockPath := strings.TrimSpace(trimmed[len("unix://"):])
		return NewHTTPClient(sockPath, "", timeout)
	}

	cleanURL := normalizeHTTPBaseURL(trimmed)
	var sockPath string
	if strings.TrimSpace(vaultRoot) != "" {
		paths := config.VaultPaths{Root: vaultRoot}
		if rec, err := ReadRuntimeRecord(paths); err == nil {
			if VaultRootsMatch(rec.VaultRoot, vaultRoot) || strings.TrimSpace(rec.VaultRoot) == "" {
				recURL := normalizeHTTPBaseURL(rec.URL)
				if cleanURL == "" || cleanURL == recURL || cleanURL == UDSBaseURL {
					sockPath = rec.SocketPath
					if cleanURL == "" || cleanURL == UDSBaseURL {
						cleanURL = recURL
					}
				}
			}
		}
		if sockPath == "" && (cleanURL == "" || cleanURL == UDSBaseURL) {
			sockPath = paths.SocketPath()
		}
	}

	if cleanURL == "" {
		cleanURL = DefaultLoopbackURL
	}
	return NewHTTPClient(sockPath, cleanURL, timeout)
}

func (e *Endpoint) HTTPClient(timeout time.Duration) *http.Client {
	if e == nil {
		c, _ := NewHTTPClient("", DefaultLoopbackURL, timeout)
		return c
	}
	c, _ := NewHTTPClient(e.SocketPath, e.WebURL, timeout)
	return c
}

func (e *Endpoint) EffectiveBaseURL() string {
	if e == nil {
		return DefaultLoopbackURL
	}
	if strings.TrimSpace(e.BaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(e.BaseURL), "/")
	}
	if strings.TrimSpace(e.WebURL) != "" {
		return strings.TrimRight(strings.TrimSpace(e.WebURL), "/")
	}
	return DefaultLoopbackURL
}

// DoJSON executes a JSON request against the daemon over UDS (with TCP fallback) and optionally decodes respDst.
func (e *Endpoint) DoJSON(ctx context.Context, method, path string, reqBody any, respDst any) ([]byte, error) {
	client := e.HTTPClient(0)
	return doJSONWithClient(ctx, client, e.EffectiveBaseURL(), method, path, reqBody, respDst)
}

// OpenStream opens a streaming HTTP/SSE response (e.g. POST /api/runs/stream) against the daemon.
func (e *Endpoint) OpenStream(ctx context.Context, method, path string, reqBody any) (*http.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var bodyReader io.Reader
	if reqBody != nil {
		encoded, err := json.Marshal(reqBody)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(encoded)
	}
	target := e.EffectiveBaseURL() + ensureLeadingSlash(path)
	req, err := http.NewRequestWithContext(ctx, method, target, bodyReader)
	if err != nil {
		return nil, err
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "text/event-stream")
	return e.HTTPClient(0).Do(req)
}

// Probe discovers a healthy running Gorness daemon for vaultRoot, verifying canonical VaultRoot equality
// against /api/health so a daemon serving another vault is never hijacked.
// It prefers Unix Domain Socket IPC ("http://localhost" over runtime/daemon.sock) and falls back to TCP.
func Probe(ctx context.Context, vaultRoot string) (*Endpoint, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	cleanVault := strings.TrimSpace(vaultRoot)
	if cleanVault == "" {
		cleanVault = ResolveVaultRoot("", nil, "")
	}
	paths := config.VaultPaths{Root: cleanVault}

	var rec RuntimeRecord
	if diskRec, err := ReadRuntimeRecord(paths); err == nil {
		if strings.TrimSpace(diskRec.VaultRoot) == "" || VaultRootsMatch(diskRec.VaultRoot, cleanVault) {
			rec = diskRec
		}
	}

	var sockCandidates []string
	addUniqueStr(&sockCandidates, rec.SocketPath)
	addUniqueStr(&sockCandidates, paths.SocketPath())

	var tcpCandidates []string
	addUniqueStr(&tcpCandidates, normalizeHTTPBaseURL(rec.URL))
	if cfgPort := readConfiguredPort(paths); cfgPort > 0 {
		addUniqueStr(&tcpCandidates, fmt.Sprintf("http://127.0.0.1:%d", cfgPort))
	}
	addUniqueStr(&tcpCandidates, DefaultLoopbackURL)

	fallbackTCP := ""
	if len(tcpCandidates) > 0 {
		fallbackTCP = tcpCandidates[0]
	}

	// 1. Prefer Unix Domain Socket IPC when reachable & verified for this vault.
	for _, sock := range sockCandidates {
		if !IsUnixSocketReachable(sock, udsDialProbeTimeout) {
			continue
		}
		client := &http.Client{
			Transport: NewTransport(sock, ""),
			Timeout:   defaultProbeTimeout,
		}
		health, ok := fetchVerifiedHealth(ctx, client, UDSBaseURL, cleanVault)
		if !ok {
			continue
		}
		webURL := normalizeHTTPBaseURL(rec.URL)
		if webURL == "" {
			for _, tcpURL := range tcpCandidates {
				tcpClient := &http.Client{Timeout: defaultProbeTimeout}
				if _, tcpOK := fetchVerifiedHealth(ctx, tcpClient, tcpURL, cleanVault); tcpOK {
					webURL = tcpURL
					break
				}
			}
		}
		if webURL == "" {
			webURL = fallbackTCP
		}
		mergedRec := mergeRecordWithHealth(rec, health, cleanVault, webURL, sock)
		return &Endpoint{
			BaseURL:    UDSBaseURL,
			WebURL:     webURL,
			SocketPath: sock,
			Record:     mergedRec,
		}, true
	}

	// 2. Seamless fallback to loopback TCP URL candidates.
	for _, tcpURL := range tcpCandidates {
		if tcpURL == "" {
			continue
		}
		client := &http.Client{
			Transport: NewTransport("", tcpURL),
			Timeout:   defaultProbeTimeout,
		}
		health, ok := fetchVerifiedHealth(ctx, client, tcpURL, cleanVault)
		if !ok {
			continue
		}
		mergedRec := mergeRecordWithHealth(rec, health, cleanVault, tcpURL, "")
		return &Endpoint{
			BaseURL:    tcpURL,
			WebURL:     tcpURL,
			SocketPath: "",
			Record:     mergedRec,
		}, true
	}

	return nil, false
}

// ResolveDaemonBinary resolves the executable path used to spawn `daemon serve`.
func ResolveDaemonBinary(explicitBin string) (string, error) {
	if trimmed := strings.TrimSpace(explicitBin); trimmed != "" {
		if st, err := os.Stat(trimmed); err == nil && !st.IsDir() {
			return trimmed, nil
		}
		if pathBin, lookErr := exec.LookPath("gorness"); lookErr == nil {
			return pathBin, nil
		}
		return "", fmt.Errorf("daemon binary not found at %s", trimmed)
	}

	if exe, err := os.Executable(); err == nil && exe != "" {
		if resolved, rErr := filepath.EvalSymlinks(exe); rErr == nil {
			exe = resolved
		}
		sibling := BundledDaemonBinary(exe)
		if st, sErr := os.Stat(sibling); sErr == nil && !st.IsDir() {
			return sibling, nil
		}
		if !strings.HasSuffix(exe, ".test") && !strings.HasSuffix(exe, ".test.exe") {
			return exe, nil
		}
	}

	if pathBin, err := exec.LookPath("gorness"); err == nil {
		return pathBin, nil
	}
	return "", errors.New("unable to locate gorness daemon binary")
}

// EnsureDaemon returns an existing healthy daemon Endpoint for opts.VaultRoot or spawns a detached
// background daemon writing logs to runtime/logs/daemon.log and waits until /api/health is ready.
func EnsureDaemon(ctx context.Context, opts EnsureOptions) (*Endpoint, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	vaultRoot := strings.TrimSpace(opts.VaultRoot)
	if vaultRoot == "" {
		vaultRoot = ResolveVaultRoot("", nil, opts.HomeDir)
	}

	if opts.EnrichPATH {
		home := strings.TrimSpace(opts.HomeDir)
		if home == "" {
			home, _ = os.UserHomeDir()
		}
		_ = os.Setenv("PATH", EnrichDesktopPATH(os.Getenv("PATH"), home))
	}

	if ep, ok := Probe(ctx, vaultRoot); ok {
		return ep, nil
	}

	var daemonBin string
	if opts.Starter == nil {
		var err error
		daemonBin, err = ResolveDaemonBinary(opts.DaemonBinary)
		if err != nil {
			return nil, err
		}
	} else {
		daemonBin = strings.TrimSpace(opts.DaemonBinary)
	}

	paths := config.VaultPaths{Root: vaultRoot}
	if err := os.MkdirAll(paths.LogsDir(), 0o700); err != nil {
		return nil, fmt.Errorf("create logs dir: %w", err)
	}
	logFile, err := os.OpenFile(DaemonLogPath(paths), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open daemon.log: %w", err)
	}

	args := []string{"daemon", "serve", "--vault", vaultRoot}
	args = append(args, opts.ExtraArgs...)

	starter := opts.Starter
	if starter == nil {
		starter = defaultDetachedStarter
	}
	if err := starter(daemonBin, args, logFile, opts.Env); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("start gorness daemon: %w", err)
	}
	_ = logFile.Close()

	waitDur := opts.WaitTimeout
	if waitDur <= 0 {
		waitDur = defaultEnsureWait
	}
	deadline := time.Now().Add(waitDur)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if ep, ok := Probe(ctx, vaultRoot); ok {
			return ep, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return nil, ErrDaemonStartTimeout
}

// StopDaemon stops the daemon serving vaultRoot via SIGTERM after verifying vault ownership,
// waiting up to waitTimeout for shutdown.
func StopDaemon(ctx context.Context, vaultRoot string, waitTimeout time.Duration) (int, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if waitTimeout <= 0 {
		waitTimeout = defaultStopWait
	}
	cleanVault := strings.TrimSpace(vaultRoot)
	if cleanVault == "" {
		cleanVault = ResolveVaultRoot("", nil, "")
	}
	paths := config.VaultPaths{Root: cleanVault}

	rec, readErr := ReadRuntimeRecord(paths)
	if readErr != nil {
		if errors.Is(readErr, ErrInvalidRuntimeMetadata) {
			return 0, false, ErrInvalidRuntimeMetadata
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			return 0, false, readErr
		}
		// daemon.toml absent; check if a verified daemon for this vault is still reachable.
		ep, ok := Probe(ctx, cleanVault)
		if !ok || ep.Record.PID <= 0 {
			return 0, false, nil
		}
		rec = ep.Record
	} else {
		if rec.PID <= 0 {
			return 0, false, ErrInvalidRuntimeMetadata
		}
		if strings.TrimSpace(rec.VaultRoot) != "" && !VaultRootsMatch(rec.VaultRoot, cleanVault) {
			return 0, false, fmt.Errorf("%w: record=%s want=%s", ErrVaultMismatch, rec.VaultRoot, cleanVault)
		}
	}

	// If daemon is reachable over /api/health, confirm vault match and prefer live PID.
	if ep, ok := Probe(ctx, cleanVault); ok && ep.Record.PID > 0 {
		rec.PID = ep.Record.PID
	}

	if proc, err := os.FindProcess(rec.PID); err == nil {
		_ = proc.Signal(syscall.SIGTERM)
	}

	deadline := time.Now().Add(waitTimeout)
	for time.Now().Before(deadline) {
		if _, ok := Probe(ctx, cleanVault); !ok {
			break
		}
		select {
		case <-ctx.Done():
			return rec.PID, true, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	_ = RemoveRuntimeRecord(paths)
	return rec.PID, true, nil
}

// RegisterWorkspace registers dirPath in the running daemon via POST /api/workspaces and returns
// both the decoded *store.Workspace and raw response JSON bytes.
func RegisterWorkspace(ctx context.Context, ep *Endpoint, dirPath, name string) (*store.Workspace, []byte, error) {
	if ep == nil {
		return nil, nil, errors.New("nil daemon endpoint")
	}
	return RegisterWorkspaceURL(ctx, ep.HTTPClient(5*time.Second), ep.EffectiveBaseURL(), dirPath, name)
}

// RegisterWorkspaceURL registers dirPath against baseURL using client.
func RegisterWorkspaceURL(ctx context.Context, client *http.Client, baseURL, dirPath, name string) (*store.Workspace, []byte, error) {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	payload := map[string]string{
		"path": strings.TrimSpace(dirPath),
	}
	if strings.TrimSpace(name) != "" {
		payload["name"] = strings.TrimSpace(name)
	}
	var ws store.Workspace
	raw, err := doJSONWithClient(ctx, client, normalizeHTTPBaseURL(baseURL), http.MethodPost, "/api/workspaces", payload, &ws)
	if err != nil {
		return nil, nil, err
	}
	return &ws, bytes.TrimSpace(raw), nil
}

func defaultDetachedStarter(bin string, args []string, logFile *os.File, env []string) error {
	cmd := exec.Command(bin, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if len(env) > 0 {
		cmd.Env = env
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	return nil
}

func doJSONWithClient(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	method string,
	path string,
	reqBody any,
	respDst any,
) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var reader io.Reader
	if reqBody != nil {
		encoded, err := json.Marshal(reqBody)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	target := strings.TrimRight(strings.TrimSpace(baseURL), "/") + ensureLeadingSlash(path)
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, err
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return raw, fmt.Errorf("HTTP %d: %s", resp.StatusCode, extractServerErrorText(raw))
	}
	if respDst != nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, respDst); err != nil {
			return raw, err
		}
	}
	return raw, nil
}

func extractServerErrorText(raw []byte) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "request failed"
	}
	var errObj struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(trimmed, &errObj) == nil && strings.TrimSpace(errObj.Error) != "" {
		return strings.TrimSpace(errObj.Error)
	}
	return string(trimmed)
}

func fetchVerifiedHealth(ctx context.Context, client *http.Client, baseURL, wantVaultRoot string) (HealthStatus, bool) {
	reqCtx, cancel := context.WithTimeout(ctx, defaultProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/health", nil)
	if err != nil {
		return HealthStatus{}, false
	}
	resp, err := client.Do(req)
	if err != nil {
		return HealthStatus{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return HealthStatus{}, false
	}
	var h HealthStatus
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil || !h.OK {
		return HealthStatus{}, false
	}
	if strings.TrimSpace(wantVaultRoot) != "" {
		if strings.TrimSpace(h.VaultRoot) == "" || !VaultRootsMatch(h.VaultRoot, wantVaultRoot) {
			return HealthStatus{}, false
		}
	}
	return h, true
}

func mergeRecordWithHealth(rec RuntimeRecord, h HealthStatus, vaultRoot, webURL, sockPath string) RuntimeRecord {
	out := rec
	if out.PID <= 0 && h.PID > 0 {
		out.PID = h.PID
	} else if h.PID > 0 {
		out.PID = h.PID
	}
	if strings.TrimSpace(h.VaultRoot) != "" {
		out.VaultRoot = CanonicalVaultPath(h.VaultRoot)
	} else if strings.TrimSpace(out.VaultRoot) == "" {
		out.VaultRoot = CanonicalVaultPath(vaultRoot)
	}
	if webURL != "" {
		out.URL = webURL
		if u, err := url.Parse(webURL); err == nil {
			out.Host = u.Hostname()
			if p := u.Port(); p != "" {
				var portNum int
				_, _ = fmt.Sscanf(p, "%d", &portNum)
				if portNum > 0 {
					out.Port = portNum
				}
			}
		}
	}
	if sockPath != "" {
		out.SocketPath = sockPath
	}
	if strings.TrimSpace(h.Version) != "" {
		out.Version = strings.TrimSpace(h.Version)
	} else if strings.TrimSpace(out.Version) == "" {
		out.Version = config.Version
	}
	return out
}

func readConfiguredPort(paths config.VaultPaths) int {
	raw, err := os.ReadFile(paths.ConfigFile())
	if err != nil {
		return 0
	}
	var doc struct {
		Port   int `toml:"port"`
		Global struct {
			Port int `toml:"port"`
		} `toml:"global"`
	}
	if toml.Unmarshal(raw, &doc) != nil {
		return 0
	}
	if doc.Global.Port > 0 {
		return doc.Global.Port
	}
	return doc.Port
}

func normalizeHTTPBaseURL(raw string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return ""
	}
	return trimmed
}

func parseHostPortFromURL(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return ""
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" {
		return ""
	}
	if _, _, splitErr := net.SplitHostPort(u.Host); splitErr == nil {
		return u.Host
	}
	if strings.EqualFold(u.Scheme, "https") {
		return net.JoinHostPort(u.Hostname(), "443")
	}
	return net.JoinHostPort(u.Hostname(), "80")
}

func ensureLeadingSlash(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "/"
	}
	if strings.HasPrefix(trimmed, "/") {
		return trimmed
	}
	return "/" + trimmed
}

func addUniqueStr(dst *[]string, val string) {
	trimmed := strings.TrimSpace(val)
	if trimmed == "" {
		return
	}
	for _, existing := range *dst {
		if existing == trimmed {
			return
		}
	}
	*dst = append(*dst, trimmed)
}
