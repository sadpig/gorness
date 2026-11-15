package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

type SandboxMode string

const (
	SandboxStrict       SandboxMode = "strict"
	SandboxReadOnly     SandboxMode = "readonly"
	SandboxNetworkAllow SandboxMode = "network_allow"
	SandboxOff          SandboxMode = "off"
)

const maxCapturedStreamBytes = 2 * 1024 * 1024

type CommandResult struct {
	Stdout     string        `json:"stdout"`
	Stderr     string        `json:"stderr"`
	Combined   string        `json:"combined"`
	ExitCode   int           `json:"exit_code"`
	Duration   time.Duration `json:"duration"`
	TimedOut   bool          `json:"timed_out,omitempty"`
	GitChanges string        `json:"git_changes,omitempty"`
}

type SeatbeltOption func(*SeatbeltRunner)

func WithRunnerProxyDial(fn DialContextFunc) SeatbeltOption {
	return func(r *SeatbeltRunner) {
		r.proxyDial = fn
	}
}

func WithRunnerVaultRoot(vaultRoot string) SeatbeltOption {
	return func(r *SeatbeltRunner) {
		r.vaultRoot = vaultRoot
	}
}

func WithRunnerExtraWriteDirs(dirs []string) SeatbeltOption {
	return func(r *SeatbeltRunner) {
		r.extraWriteDirs = append([]string(nil), dirs...)
	}
}

// SeatbeltRunner 驱动 macOS 原生 /usr/bin/sandbox-exec 内核级沙箱、独立进程组生命周期与嵌入式 EgressProxy 域名白名单。
type SeatbeltRunner struct {
	proxyDial      DialContextFunc
	vaultRoot      string
	extraWriteDirs []string
}

func NewSeatbeltRunner(opts ...SeatbeltOption) *SeatbeltRunner {
	r := &SeatbeltRunner{}
	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}
	return r
}

func NormalizeSandboxMode(raw string) SandboxMode {
	switch SandboxMode(strings.ToLower(strings.TrimSpace(raw))) {
	case SandboxReadOnly:
		return SandboxReadOnly
	case SandboxNetworkAllow:
		return SandboxNetworkAllow
	case SandboxOff:
		return SandboxOff
	default:
		return SandboxStrict
	}
}

// BuildProfile 生成 macOS Seatbelt SBPL 策略：
// 放行基础系统库与工具链只读访问，封堵 AppleEvents 与钥匙串，严格将写权限收敛至规范化 wsRoot 与指定临时目录，
// 并按需将出站 TCP 限制在本地 EgressProxy 监听端口。
func (s *SeatbeltRunner) BuildProfile(wsRoot string, extraWriteDirs []string, mode SandboxMode, proxyPort int) string {
	canonWs := canonicalizeExistingOrClean(wsRoot)
	var canonVault string
	if s != nil && strings.TrimSpace(s.vaultRoot) != "" {
		canonVault = canonicalizeExistingOrClean(s.vaultRoot)
	}

	homeDir, _ := os.UserHomeDir()
	canonHome := canonicalizeExistingOrClean(homeDir)

	var b strings.Builder
	b.WriteString("(version 1)\n")
	b.WriteString("(deny default)\n")
	b.WriteString("(allow process*)\n")
	b.WriteString("(allow sysctl-read)\n")
	b.WriteString("(allow ipc-posix-shm*)\n")
	b.WriteString("(allow mach-lookup)\n")
	b.WriteString("(deny mach-lookup\n")
	b.WriteString("  (global-name \"com.apple.CoreServices.appleevents\")\n")
	b.WriteString("  (global-name \"com.apple.securityd\"))\n")
	b.WriteString("(deny appleevent-send)\n")
	b.WriteString("(allow file-read-metadata)\n")

	// 系统与工具链只读白名单（不放行整个 /private/var/folders，防止越界读取宿主机其他临时目录或机密）。
	b.WriteString("(allow file-read*\n")
	for _, sysLiteral := range []string{"/", "/var", "/etc", "/tmp", "/private"} {
		fmt.Fprintf(&b, "  (literal %q)\n", escapeSBPL(sysLiteral))
	}
	for _, sysSub := range []string{
		"/bin",
		"/sbin",
		"/usr",
		"/System",
		"/Library",
		"/Applications",
		"/opt",
		"/dev",
		"/private/etc",
		"/private/var/db",
		"/private/var/select",
		"/private/var/tmp",
	} {
		fmt.Fprintf(&b, "  (subpath %q)\n", escapeSBPL(sysSub))
	}
	b.WriteString("  (regex #\"^/private/var/folders/.*/xcrun_db[^/]*$\")\n")

	for _, tcPath := range discoverToolchainReadPaths(canonHome) {
		fmt.Fprintf(&b, "  (subpath %q)\n", escapeSBPL(tcPath))
	}
	if canonHome != "" {
		gitCfg := filepath.Join(canonHome, ".gitconfig")
		fmt.Fprintf(&b, "  (literal %q)\n", escapeSBPL(gitCfg))
	}
	for _, dir := range extraWriteDirs {
		cDir := canonicalizeExistingOrClean(dir)
		if cDir != "" {
			fmt.Fprintf(&b, "  (subpath %q)\n", escapeSBPL(cDir))
		}
	}
	if canonWs != "" {
		fmt.Fprintf(&b, "  (subpath %q)\n", escapeSBPL(canonWs))
	}
	b.WriteString(")\n")

	// 显式封堵敏感目录读取（~/.gorness、~/.ssh、~/Library/Keychains）；
	// 若当前 wsRoot 位于 runtime/worktrees 下，先封堵整个 vault 再单独恢复该 worktree 的读取权限。
	if canonHome != "" {
		b.WriteString("(deny file-read*\n")
		fmt.Fprintf(&b, "  (subpath %q)\n", escapeSBPL(filepath.Join(canonHome, ".ssh")))
		fmt.Fprintf(&b, "  (subpath %q)\n", escapeSBPL(filepath.Join(canonHome, "Library", "Keychains")))
		fmt.Fprintf(&b, "  (subpath %q)\n", escapeSBPL(filepath.Join(canonHome, ".gorness")))
		b.WriteString(")\n")
	}
	if canonVault != "" {
		fmt.Fprintf(&b, "(deny file-read* (subpath %q))\n", escapeSBPL(canonVault))
		if canonWs != "" && isStrictlyWithinPath(canonWs, filepath.Join(canonVault, "runtime", "worktrees")) {
			fmt.Fprintf(&b, "(allow file-read* (subpath %q))\n", escapeSBPL(canonWs))
		}
	}

	// 文件写权限：仅限 /dev 空设备、专用隔离临时目录与（非只读模式下）规范化工作区。
	b.WriteString("(allow file-write*\n")
	for _, devLit := range []string{
		"/dev/null",
		"/dev/zero",
		"/dev/tty",
		"/dev/dtracehelper",
		"/dev/urandom",
		"/dev/random",
	} {
		fmt.Fprintf(&b, "  (literal %q)\n", escapeSBPL(devLit))
	}
	b.WriteString("  (subpath \"/dev/fd\")\n")
	b.WriteString("  (regex #\"^/private/var/folders/.*/xcrun_db[^/]*$\")\n")
	for _, dir := range extraWriteDirs {
		cDir := canonicalizeExistingOrClean(dir)
		if cDir != "" {
			fmt.Fprintf(&b, "  (subpath %q)\n", escapeSBPL(cDir))
		}
	}
	if mode != SandboxReadOnly && canonWs != "" {
		fmt.Fprintf(&b, "  (subpath %q)\n", escapeSBPL(canonWs))
	}
	b.WriteString(")\n")

	// 网络策略：network_allow 放行网络；strict/readonly 仅在启动本地 EgressProxy 时放行 127.0.0.1:<proxyPort>。
	switch mode {
	case SandboxNetworkAllow:
		b.WriteString("(allow network*)\n")
	default:
		if proxyPort > 0 {
			fmt.Fprintf(&b, "(allow network-outbound (remote tcp \"localhost:%d\"))\n", proxyPort)
		}
	}

	return b.String()
}

type seatbeltRunParams struct {
	CmdStr         string
	SandboxRoot    string
	WorkDir        string
	ExtraWriteDirs []string
	Mode           SandboxMode
	AllowedDomains []string
}

func (s *SeatbeltRunner) Run(
	ctx context.Context,
	cmdStr, workDir string,
	mode SandboxMode,
	allowedDomains []string,
) (CommandResult, error) {
	return s.runWithParams(ctx, seatbeltRunParams{
		CmdStr:         cmdStr,
		SandboxRoot:    workDir,
		WorkDir:        workDir,
		Mode:           mode,
		AllowedDomains: allowedDomains,
	})
}

func (s *SeatbeltRunner) runWithParams(ctx context.Context, p seatbeltRunParams) (CommandResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(p.CmdStr) == "" {
		return CommandResult{}, errors.New("empty command string")
	}
	mode := p.Mode
	if mode == "" {
		mode = SandboxStrict
	}

	canonWorkDir := canonicalizeExistingOrClean(p.WorkDir)
	if canonWorkDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return CommandResult{}, err
		}
		canonWorkDir = canonicalizeExistingOrClean(cwd)
	}
	canonSandboxRoot := canonicalizeExistingOrClean(p.SandboxRoot)
	if canonSandboxRoot == "" {
		canonSandboxRoot = canonWorkDir
	}

	// 为每次命令分配专属隔离临时目录（执行完毕即清理），避免污染或放行宿主机全局 /tmp 与其他会话临时目录。
	cmdTmpDir, err := os.MkdirTemp("", "gorness-sbx-tmp-*")
	if err != nil {
		return CommandResult{}, fmt.Errorf("create sandbox scratch dir: %w", err)
	}
	canonTmpDir := canonicalizeExistingOrClean(cmdTmpDir)
	defer func() { _ = os.RemoveAll(canonTmpDir) }()

	var proxy *EgressProxy
	proxyPort := 0
	if mode != SandboxOff && mode != SandboxNetworkAllow && len(p.AllowedDomains) > 0 {
		var opts []EgressProxyOption
		if s != nil && s.proxyDial != nil {
			opts = append(opts, WithProxyDialContext(s.proxyDial))
		}
		proxy, err = NewEgressProxy(p.AllowedDomains, opts...)
		if err != nil {
			return CommandResult{}, err
		}
		proxyPort = proxy.Port()
		defer func() { _ = proxy.Close() }()
	}

	var writeDirs []string
	if mode != SandboxReadOnly {
		if s != nil && len(s.extraWriteDirs) > 0 {
			writeDirs = append(writeDirs, s.extraWriteDirs...)
		}
		if len(p.ExtraWriteDirs) > 0 {
			writeDirs = append(writeDirs, p.ExtraWriteDirs...)
		}
	}
	writeDirs = append(writeDirs, canonTmpDir)

	var cmd *exec.Cmd
	if mode == SandboxOff {
		cmd = exec.CommandContext(ctx, "/bin/zsh", "-f", "-c", p.CmdStr)
	} else {
		profile := s.BuildProfile(canonSandboxRoot, writeDirs, mode, proxyPort)
		cmd = exec.CommandContext(ctx, "/usr/bin/sandbox-exec", "-p", profile, "/bin/zsh", "-f", "-c", p.CmdStr)
	}

	cmd.Dir = canonWorkDir
	cmd.Env = buildCleanCommandEnv(canonTmpDir, proxy)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil && cmd.Process.Pid > 0 {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	cmd.WaitDelay = 500 * time.Millisecond

	var stdoutBuf, stderrBuf, combinedBuf cappedBuffer
	stdoutBuf.limit = maxCapturedStreamBytes
	stderrBuf.limit = maxCapturedStreamBytes
	combinedBuf.limit = maxCapturedStreamBytes * 2

	var combMu sync.Mutex
	cmd.Stdout = &teeWriter{primary: &stdoutBuf, combined: &combinedBuf, mu: &combMu}
	cmd.Stderr = &teeWriter{primary: &stderrBuf, combined: &combinedBuf, mu: &combMu}

	started := time.Now()
	if err := cmd.Start(); err != nil {
		return CommandResult{}, fmt.Errorf("start command: %w", err)
	}
	pid := cmd.Process.Pid

	waitErr := cmd.Wait()
	// 回收同一进程组内可能残留的后台孙进程。
	if pid > 0 {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
	elapsed := time.Since(started)

	res := CommandResult{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		Combined: combinedBuf.String(),
		Duration: elapsed,
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		res.TimedOut = true
		res.ExitCode = -1
		return res, ctx.Err()
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		res.ExitCode = -1
		return res, ctx.Err()
	}

	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
			return res, nil
		}
		return res, waitErr
	}
	return res, nil
}

type cappedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	origLen := len(p)
	rem := c.limit - c.buf.Len()
	if rem <= 0 {
		return origLen, nil
	}
	if len(p) > rem {
		p = p[:rem]
	}
	_, _ = c.buf.Write(p)
	return origLen, nil
}

func (c *cappedBuffer) String() string {
	return c.buf.String()
}

type teeWriter struct {
	primary  *cappedBuffer
	combined *cappedBuffer
	mu       *sync.Mutex
}

func (t *teeWriter) Write(p []byte) (int, error) {
	_, _ = t.primary.Write(p)
	t.mu.Lock()
	_, _ = t.combined.Write(p)
	t.mu.Unlock()
	return len(p), nil
}

func buildCleanCommandEnv(tmpDir string, proxy *EgressProxy) []string {
	allowKeys := map[string]bool{
		"PATH":          true,
		"HOME":          true,
		"USER":          true,
		"LOGNAME":       true,
		"SHELL":         true,
		"LANG":          true,
		"LC_ALL":        true,
		"LC_CTYPE":      true,
		"TERM":          true,
		"GOROOT":        true,
		"GOPATH":        true,
		"GOMODCACHE":    true,
		"GOPROXY":       true,
		"GOSUMDB":       true,
		"DEVELOPER_DIR": true,
	}

	var env []string
	hasPath := false
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			continue
		}
		k := kv[:eq]
		if allowKeys[k] {
			if k == "PATH" {
				hasPath = true
			}
			env = append(env, kv)
		}
	}
	if !hasPath {
		env = append(env, "PATH=/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin")
	}
	env = append(env,
		"TMPDIR="+tmpDir,
		"GOCACHE="+filepath.Join(tmpDir, "gocache"),
		"GIT_TERMINAL_PROMPT=0",
	)
	if proxy != nil && proxy.Port() > 0 {
		pURL := proxy.URL()
		env = append(env,
			"HTTP_PROXY="+pURL,
			"HTTPS_PROXY="+pURL,
			"ALL_PROXY="+pURL,
			"http_proxy="+pURL,
			"https_proxy="+pURL,
			"all_proxy="+pURL,
			"NO_PROXY=",
			"no_proxy=",
		)
	}
	return env
}

func discoverToolchainReadPaths(canonHome string) []string {
	candidates := []string{runtime.GOROOT()}
	for _, envKey := range []string{"GOROOT", "GOPATH", "GOMODCACHE"} {
		if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
			candidates = append(candidates, v)
		}
	}
	if canonHome != "" {
		candidates = append(candidates,
			filepath.Join(canonHome, "go"),
			filepath.Join(canonHome, "Library", "Developer"),
			filepath.Join(canonHome, "Library", "Caches", "go-build"),
			filepath.Join(canonHome, ".config", "git"),
			filepath.Join(canonHome, ".cargo"),
			filepath.Join(canonHome, ".rustup"),
			filepath.Join(canonHome, ".nvm"),
			filepath.Join(canonHome, ".local"),
		)
	}

	seen := make(map[string]struct{})
	var out []string
	for _, c := range candidates {
		if strings.TrimSpace(c) == "" {
			continue
		}
		resolved, err := filepath.EvalSymlinks(c)
		if err != nil {
			continue
		}
		resolved = filepath.Clean(resolved)
		if _, ok := seen[resolved]; !ok {
			seen[resolved] = struct{}{}
			out = append(out, resolved)
		}
	}
	return out
}

func canonicalizeExistingOrClean(p string) string {
	trimmed := strings.TrimSpace(p)
	if trimmed == "" {
		return ""
	}
	rawAbs, err := toUncleanedAbs("", trimmed)
	if err != nil {
		return filepath.Clean(trimmed)
	}
	if resolved, err := resolveCanonicalPath(rawAbs, 0); err == nil {
		return resolved
	}
	return filepath.Clean(rawAbs)
}

func escapeSBPL(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '"':
			b.WriteString(`\"`)
		case r < 0x20 || r == 0x7f:
			// Strip control characters in SBPL path literals.
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
