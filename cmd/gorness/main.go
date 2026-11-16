package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pelletier/go-toml/v2"
	"gorness.dev/gorness/internal/alwayson"
	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/core"
	"gorness.dev/gorness/internal/fleet"
	"gorness.dev/gorness/internal/llm"
	"gorness.dev/gorness/internal/server"
	"gorness.dev/gorness/internal/store"
	"gorness.dev/gorness/internal/tui"
)

const Version = "1.0.0"

type daemonRuntimeInfo struct {
	PID        int       `toml:"pid"`
	Host       string    `toml:"host"`
	Port       int       `toml:"port"`
	URL        string    `toml:"url"`
	SocketPath string    `toml:"socket_path"`
	StartedAt  time.Time `toml:"started_at"`
}

type cliOptions struct {
	userHomeDir     string
	streamer        llm.Streamer
	notifier        alwayson.Notifier
	opener          server.OpenerFunc
	browserLauncher func(url string) error
	cmdRunner       func(ctx context.Context, name string, args ...string) error
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	code := runCLI(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, cliOptions{})
	os.Exit(code)
}

func runCLI(
	ctx context.Context,
	args []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	opts cliOptions,
) int {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(args) == 0 {
		return cmdTUI(ctx, nil, stderr, opts)
	}

	first := strings.TrimSpace(args[0])
	switch first {
	case "-v", "--version", "version":
		fmt.Fprintf(stdout, "gorness v%s (%s/%s, zero-db)\n", Version, runtime.GOOS, runtime.GOARCH)
		return 0
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	case "studio":
		return cmdStudio(ctx, args[1:], stdout, stderr, opts)
	case "run":
		return cmdRun(ctx, args[1:], stdout, stderr, opts)
	case "daemon":
		return cmdDaemon(ctx, args[1:], stdout, stderr, opts)
	case "acp":
		return cmdACP(ctx, args[1:], stdin, stdout, stderr, opts)
	default:
		if strings.HasPrefix(first, "-") {
			return cmdTUI(ctx, args, stderr, opts)
		}
		fmt.Fprintf(stderr, "unknown gorness command %q\n\n", first)
		printUsage(stderr)
		return 2
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintf(w, `Gorness v%s — macOS Zero-DB AI Agent Harness (TOML · MD · JSONL · HTML)

Usage:
  gorness [flags]                        Launch multi-project Bubble Tea TUI (auto-starts daemon)
  gorness studio [flags]                 Open Apple HIG Workbench & Visual Multi-Agent Studio
  gorness run [flags] "<prompt>"         Execute headless single-session turn (--json for NDJSON)
  gorness daemon <serve|status|stop|install|uninstall>
  gorness acp [flags]                    Start stdio JSON-RPC 2.0 ACP server backed by Engine
  gorness --version                      Print version
`, Version)
}

// ============================================================================
// Subcommand 1: Headless `gorness run`
// ============================================================================

func cmdRun(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	opts cliOptions,
) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		vaultFlag   string
		wsFlag      string
		agentRole   string
		modelOver   string
		permMode    string
		sandboxMode string
		sessionID   string
		jsonOutput  bool
	)
	fs.StringVar(&vaultFlag, "vault", "", "Path to ~/.gorness vault directory")
	fs.StringVar(&wsFlag, "workspace", "", "Target workspace directory (defaults to cwd)")
	fs.StringVar(&agentRole, "agent", "coder", "Agent role profile (coder, lead, explorer, verifier, debugger)")
	fs.StringVar(&modelOver, "model", "", "Provider/model override (e.g. anthropic/claude-opus-5-5)")
	fs.StringVar(&permMode, "perm", "", "Permission mode override (plan, ask, accept_edits, auto, yolo)")
	fs.StringVar(&sandboxMode, "sandbox", "", "Seatbelt sandbox mode override")
	fs.StringVar(&sessionID, "session", "", "Resume existing session ID")
	fs.BoolVar(&jsonOutput, "json", false, "Stream NDJSON RunEvent objects to stdout")

	reordered := reorderFlagsBeforePositional(args, map[string]bool{
		"--json": true, "-json": true,
		"--help": true, "-help": true, "-h": true,
	})
	if err := fs.Parse(reordered); err != nil {
		return 2
	}

	prompt := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if prompt == "" {
		fmt.Fprintln(stderr, "gorness run requires a non-empty <prompt>")
		return 2
	}

	wsPath := resolveWorkspaceDir(wsFlag)
	vaultRoot := resolveVaultRoot(vaultFlag, opts.userHomeDir)

	cfgMgr, eng, err := buildCoreEngine(vaultRoot, opts)
	if err != nil {
		fmt.Fprintf(stderr, "bootstrap engine error: %v\n", err)
		return 1
	}
	defer func() { _ = eng.Close() }()
	_ = cfgMgr

	ws, err := eng.Workspaces().Register(wsPath, "")
	if err != nil {
		fmt.Fprintf(stderr, "register workspace %s: %v\n", wsPath, err)
		return 1
	}

	_, evCh, err := eng.StartRun(ctx, core.RunRequest{
		WorkspaceID:   ws.ID,
		SessionID:     sessionID,
		Prompt:        prompt,
		AgentRole:     agentRole,
		ModelOverride: modelOver,
		PermMode:      permMode,
		SandboxMode:   sandboxMode,
		Unattended:    true,
	})
	if err != nil {
		fmt.Fprintf(stderr, "start run failed: %v\n", err)
		return 1
	}

	var enc *json.Encoder
	if jsonOutput {
		enc = json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
	}

	exitCode := 0
	printedTextNL := true

	for ev := range evCh {
		if jsonOutput {
			_ = enc.Encode(ev)
		} else {
			switch ev.Type {
			case core.EventTextDelta:
				if mp, ok := ev.Payload.(map[string]any); ok {
					if txt, _ := mp["text"].(string); txt != "" {
						_, _ = io.WriteString(stdout, txt)
						printedTextNL = strings.HasSuffix(txt, "\n")
					}
				}
			case core.EventToolCall:
				if tc, ok := ev.Payload.(core.ToolCallPayload); ok {
					fmt.Fprintf(stderr, "[tool] %s\n", tc.ToolName)
				}
			case core.EventToolDeferred:
				if td, ok := ev.Payload.(core.DeferredToolPayload); ok {
					fmt.Fprintf(stderr, "[deferred] %s (%s): %s\n", td.ToolName, td.ReasonCode, td.Reason)
				}
			case core.EventRunFailed:
				if pe, ok := ev.Payload.(core.RunErrorPayload); ok {
					fmt.Fprintf(stderr, "run failed: %s\n", pe.Message)
				}
			}
		}
		if ev.Type == core.EventRunFailed || ev.Type == core.EventRunCancelled {
			exitCode = 1
		}
	}

	if !jsonOutput && !printedTextNL {
		_, _ = io.WriteString(stdout, "\n")
	}
	eng.WaitBackground()
	return exitCode
}

// ============================================================================
// Subcommand 2: TUI (`gorness`) & Apple HIG Workbench (`gorness studio`)
// ============================================================================

func cmdTUI(ctx context.Context, args []string, stderr io.Writer, opts cliOptions) int {
	fs := flag.NewFlagSet("gorness", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		vaultFlag string
		wsFlag    string
		serverURL string
	)
	fs.StringVar(&vaultFlag, "vault", "", "Path to ~/.gorness vault directory")
	fs.StringVar(&wsFlag, "workspace", "", "Initial workspace directory")
	fs.StringVar(&serverURL, "server", "", "Existing loopback server URL")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	vaultRoot := resolveVaultRoot(vaultFlag, opts.userHomeDir)
	wsPath, wsErr := resolveInteractiveWorkspaceDir(wsFlag, vaultRoot)
	if wsErr != nil {
		fmt.Fprintf(stderr, "invalid workspace: %v\n", wsErr)
		return 1
	}

	targetURL := strings.TrimSpace(serverURL)
	if targetURL == "" {
		var err error
		targetURL, err = ensureBackgroundDaemon(ctx, vaultRoot, opts)
		if err != nil {
			fmt.Fprintf(stderr, "failed to start gorness daemon: %v\n", err)
			return 1
		}
	}

	if err := tui.Run(ctx, targetURL, wsPath); err != nil {
		fmt.Fprintf(stderr, "tui exited with error: %v\n", err)
		return 1
	}
	return 0
}

func cmdStudio(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	opts cliOptions,
) int {
	fs := flag.NewFlagSet("studio", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		vaultFlag string
		wsFlag    string
		noOpen    bool
	)
	fs.StringVar(&vaultFlag, "vault", "", "Path to ~/.gorness vault directory")
	fs.StringVar(&wsFlag, "workspace", "", "Workspace directory to register")
	fs.BoolVar(&noOpen, "no-open", false, "Ensure daemon without launching browser window")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	vaultRoot := resolveVaultRoot(vaultFlag, opts.userHomeDir)
	wsPath, wsErr := resolveInteractiveWorkspaceDir(wsFlag, vaultRoot)
	if wsErr != nil {
		fmt.Fprintf(stderr, "invalid workspace: %v\n", wsErr)
		return 1
	}

	daemonURL, err := ensureBackgroundDaemon(ctx, vaultRoot, opts)
	if err != nil {
		fmt.Fprintf(stderr, "failed to ensure gorness daemon: %v\n", err)
		return 1
	}

	// Register current workspace into daemon so Studio opens directly onto it
	if wsPath != "" {
		regBody, _ := json.Marshal(map[string]string{"path": wsPath})
		resp, pErr := http.Post(daemonURL+"/api/workspaces", "application/json", bytes.NewReader(regBody))
		if pErr == nil {
			if resp.StatusCode >= 400 && strings.TrimSpace(wsFlag) != "" {
				rawErr, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				fmt.Fprintf(stderr, "register workspace %s: HTTP %d: %s\n", wsPath, resp.StatusCode, strings.TrimSpace(string(rawErr)))
				return 1
			}
			_ = resp.Body.Close()
		}
	}

	fmt.Fprintf(stdout, "Gorness Studio ready at %s\n", daemonURL)
	if !noOpen {
		launcher := opts.browserLauncher
		if launcher == nil {
			launcher = openMacStudioWindow
		}
		if err := launcher(daemonURL); err != nil {
			fmt.Fprintf(stderr, "warning: could not launch browser automatically (%v); open %s manually.\n", err, daemonURL)
		}
	}
	return 0
}

func openMacStudioWindow(targetURL string) error {
	// Prefer dedicated chromeless macOS app-mode window when available; fall back cleanly to default browser.
	appBinaries := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
	}
	for _, bin := range appBinaries {
		if st, err := os.Stat(bin); err == nil && !st.IsDir() {
			cmd := exec.Command(bin, "--app="+targetURL)
			if err := cmd.Start(); err == nil {
				return nil
			}
		}
	}
	return exec.Command("/usr/bin/open", targetURL).Start()
}

// ============================================================================
// Subcommand 3: `gorness daemon [serve|install|uninstall|status|stop]`
// ============================================================================

func cmdDaemon(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	opts cliOptions,
) int {
	sub := "serve"
	subArgs := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub = strings.ToLower(strings.TrimSpace(args[0]))
		subArgs = args[1:]
	}

	switch sub {
	case "serve":
		return daemonServe(ctx, subArgs, stdout, stderr, opts)
	case "status":
		return daemonStatus(subArgs, stdout, stderr, opts)
	case "stop":
		return daemonStop(subArgs, stdout, stderr, opts)
	case "install":
		return daemonInstall(ctx, subArgs, stdout, stderr, opts)
	case "uninstall":
		return daemonUninstall(ctx, subArgs, stdout, stderr, opts)
	default:
		fmt.Fprintf(stderr, "unknown daemon action %q (expected serve|status|stop|install|uninstall)\n", sub)
		return 2
	}
}

func daemonInfoPath(paths config.VaultPaths) string {
	return filepath.Join(paths.RuntimeDir(), "daemon.toml")
}

func daemonLockPath(paths config.VaultPaths) string {
	return filepath.Join(paths.RuntimeDir(), "locks", "daemon.lock")
}

func daemonServe(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	opts cliOptions,
) int {
	fs := flag.NewFlagSet("daemon serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		vaultFlag string
		hostFlag  string
		portFlag  int
	)
	fs.StringVar(&vaultFlag, "vault", "", "Path to ~/.gorness vault")
	fs.StringVar(&hostFlag, "host", "", "Loopback host (127.0.0.1)")
	fs.IntVar(&portFlag, "port", -1, "Loopback TCP port (default config.toml 3456; 0 for ephemeral)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	vaultRoot := resolveVaultRoot(vaultFlag, opts.userHomeDir)
	cfgMgr, eng, err := buildCoreEngine(vaultRoot, opts)
	if err != nil {
		fmt.Fprintf(stderr, "daemon bootstrap failed: %v\n", err)
		return 1
	}
	defer func() { _ = eng.Close() }()

	paths := cfgMgr.Paths()
	unlock, ok, err := store.TryFlock(daemonLockPath(paths))
	if err != nil {
		fmt.Fprintf(stderr, "acquire daemon lock: %v\n", err)
		return 1
	}
	if !ok {
		fmt.Fprintln(stderr, "gorness daemon is already running")
		return 0
	}
	defer unlock()

	snap := cfgMgr.Current()
	host := strings.TrimSpace(hostFlag)
	if host == "" {
		host = strings.TrimSpace(snap.Global.Host)
	}
	if host == "" {
		host = "127.0.0.1"
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		fmt.Fprintf(stderr, "refusing to bind non-loopback host %q\n", host)
		return 1
	}
	port := portFlag
	if port < 0 {
		port = snap.Global.Port
		if port <= 0 {
			port = 3456
		}
	}

	aoSvc, err := alwayson.NewService(alwayson.ServiceOptions{
		Config:   cfgMgr,
		Engine:   eng,
		Notifier: opts.notifier,
	})
	if err != nil {
		fmt.Fprintf(stderr, "init alwayson service: %v\n", err)
		return 1
	}
	if err := aoSvc.Start(ctx); err != nil {
		fmt.Fprintf(stderr, "start alwayson service: %v\n", err)
		return 1
	}
	defer func() { _ = aoSvc.Close() }()

	fleetMgr, err := fleet.NewManager(fleet.ManagerOptions{
		Config: cfgMgr,
		Engine: eng,
	})
	if err != nil {
		fmt.Fprintf(stderr, "init fleet manager: %v\n", err)
		return 1
	}
	defer func() { _ = fleetMgr.Close() }()

	srv := server.NewServer(server.Deps{
		Config:   cfgMgr,
		Engine:   eng,
		AlwaysOn: aoSvc,
		Fleet:    fleetMgr,
		Opener:   opts.opener,
		Version:  Version,
	})
	defer func() { _ = srv.Close() }()

	tcpAddr := net.JoinHostPort(host, strconv.Itoa(port))
	tcpLn, err := net.Listen("tcp", tcpAddr)
	if err != nil {
		fmt.Fprintf(stderr, "listen %s: %v\n", tcpAddr, err)
		return 1
	}
	defer tcpLn.Close()

	actualPort := tcpLn.Addr().(*net.TCPAddr).Port
	urlStr := fmt.Sprintf("http://%s:%d", host, actualPort)

	sockPath := paths.SocketPath()
	_ = os.Remove(sockPath)
	unixLn, unixErr := net.Listen("unix", sockPath)
	if unixErr == nil {
		_ = os.Chmod(sockPath, 0o600)
		defer func() {
			_ = unixLn.Close()
			_ = os.Remove(sockPath)
		}()
	}

	info := daemonRuntimeInfo{
		PID:        os.Getpid(),
		Host:       host,
		Port:       actualPort,
		URL:        urlStr,
		SocketPath: sockPath,
		StartedAt:  time.Now().UTC(),
	}
	if rawTOML, mErr := toml.Marshal(info); mErr == nil {
		_ = store.AtomicWriteFile(daemonInfoPath(paths), rawTOML, 0o600)
		defer func() { _ = os.Remove(daemonInfoPath(paths)) }()
	}

	httpSrv := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 2)
	go func() {
		if sErr := httpSrv.Serve(tcpLn); sErr != nil && !errors.Is(sErr, http.ErrServerClosed) {
			errCh <- sErr
		}
	}()
	if unixLn != nil {
		go func() {
			if sErr := httpSrv.Serve(unixLn); sErr != nil && !errors.Is(sErr, http.ErrServerClosed) {
				errCh <- sErr
			}
		}()
	}

	fmt.Fprintf(stdout, "gorness daemon serving at %s (pid=%d, vault=%s)\n", urlStr, info.PID, paths.Root)

	select {
	case <-ctx.Done():
	case serveErr := <-errCh:
		fmt.Fprintf(stderr, "daemon server error: %v\n", serveErr)
		return 1
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutCtx)
	return 0
}

func daemonStatus(args []string, stdout, stderr io.Writer, opts cliOptions) int {
	fs := flag.NewFlagSet("daemon status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var vaultFlag string
	fs.StringVar(&vaultFlag, "vault", "", "Path to ~/.gorness vault")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	vaultRoot := resolveVaultRoot(vaultFlag, opts.userHomeDir)
	urlStr, info := probeRunningDaemon(vaultRoot)
	if urlStr == "" {
		fmt.Fprintln(stdout, "gorness daemon: stopped")
		return 1
	}
	fmt.Fprintf(stdout, "gorness daemon: running at %s (pid=%d)\n", urlStr, info.PID)
	return 0
}

func daemonStop(args []string, stdout, stderr io.Writer, opts cliOptions) int {
	fs := flag.NewFlagSet("daemon stop", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var vaultFlag string
	fs.StringVar(&vaultFlag, "vault", "", "Path to ~/.gorness vault")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	vaultRoot := resolveVaultRoot(vaultFlag, opts.userHomeDir)
	paths := config.VaultPaths{Root: vaultRoot}
	raw, err := os.ReadFile(daemonInfoPath(paths))
	if err != nil {
		fmt.Fprintln(stdout, "gorness daemon is not running")
		return 0
	}
	var info daemonRuntimeInfo
	if err := toml.Unmarshal(raw, &info); err != nil || info.PID <= 0 {
		fmt.Fprintln(stderr, "invalid daemon.toml metadata")
		return 1
	}
	proc, err := os.FindProcess(info.PID)
	if err == nil {
		_ = proc.Signal(syscall.SIGTERM)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if urlStr, _ := probeRunningDaemon(vaultRoot); urlStr == "" {
			break
		}
		time.Sleep(60 * time.Millisecond)
	}
	fmt.Fprintf(stdout, "stopped gorness daemon (pid=%d)\n", info.PID)
	return 0
}

func daemonInstall(ctx context.Context, args []string, stdout, stderr io.Writer, opts cliOptions) int {
	fs := flag.NewFlagSet("daemon install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var vaultFlag string
	fs.StringVar(&vaultFlag, "vault", "", "Path to ~/.gorness vault")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	vaultRoot := resolveVaultRoot(vaultFlag, opts.userHomeDir)
	exePath, err := os.Executable()
	if err != nil {
		exePath = "/usr/local/bin/gorness"
	}
	plistBytes, plistPath := alwayson.GenerateLaunchdPlist(exePath, vaultRoot)
	if err := store.AtomicWriteFile(plistPath, plistBytes, 0o600); err != nil {
		fmt.Fprintf(stderr, "write LaunchAgent plist %s: %v\n", plistPath, err)
		return 1
	}
	runner := opts.cmdRunner
	if runner == nil {
		runner = defaultExecRunner
	}
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	_ = runner(ctx, "/bin/launchctl", "bootout", domain, plistPath)
	if err := runner(ctx, "/bin/launchctl", "bootstrap", domain, plistPath); err != nil {
		fmt.Fprintf(stderr, "launchctl bootstrap warning: %v (plist written to %s)\n", err, plistPath)
	} else {
		fmt.Fprintf(stdout, "installed LaunchAgent %s\n", plistPath)
	}
	return 0
}

func daemonUninstall(ctx context.Context, args []string, stdout, stderr io.Writer, opts cliOptions) int {
	fs := flag.NewFlagSet("daemon uninstall", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var vaultFlag string
	fs.StringVar(&vaultFlag, "vault", "", "Path to ~/.gorness vault")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	vaultRoot := resolveVaultRoot(vaultFlag, opts.userHomeDir)
	_, plistPath := alwayson.GenerateLaunchdPlist("", vaultRoot)
	runner := opts.cmdRunner
	if runner == nil {
		runner = defaultExecRunner
	}
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	_ = runner(ctx, "/bin/launchctl", "bootout", domain, plistPath)
	_ = os.Remove(plistPath)
	fmt.Fprintf(stdout, "uninstalled LaunchAgent %s\n", plistPath)
	return 0
}

func daemonBackgroundLogPath(paths config.VaultPaths) string {
	return filepath.Join(paths.LogsDir(), "daemon.log")
}

func ensureBackgroundDaemon(ctx context.Context, vaultRoot string, opts cliOptions) (string, error) {
	if urlStr, _ := probeRunningDaemon(vaultRoot); urlStr != "" {
		return urlStr, nil
	}

	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate gorness binary: %w", err)
	}
	paths := config.VaultPaths{Root: vaultRoot}
	if err := os.MkdirAll(paths.LogsDir(), 0o700); err != nil {
		return "", err
	}
	logFile, err := os.OpenFile(daemonBackgroundLogPath(paths), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return "", err
	}

	cmd := exec.Command(exe, "daemon", "serve", "--vault", vaultRoot)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return "", err
	}
	_ = cmd.Process.Release()
	_ = logFile.Close()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if urlStr, _ := probeRunningDaemon(vaultRoot); urlStr != "" {
			return urlStr, nil
		}
		time.Sleep(60 * time.Millisecond)
	}
	return "", errors.New("timed out waiting for background gorness daemon to become healthy")
}

func probeRunningDaemon(vaultRoot string) (string, daemonRuntimeInfo) {
	paths := config.VaultPaths{Root: vaultRoot}
	var info daemonRuntimeInfo
	candidates := []string{}
	if raw, err := os.ReadFile(daemonInfoPath(paths)); err == nil {
		if toml.Unmarshal(raw, &info) == nil && strings.TrimSpace(info.URL) != "" {
			candidates = append(candidates, strings.TrimRight(info.URL, "/"))
		}
	}
	candidates = append(candidates, "http://127.0.0.1:3456")

	wantCanon := canonicalOrCleanPath(vaultRoot)
	client := &http.Client{Timeout: 600 * time.Millisecond}
	for _, base := range candidates {
		resp, err := client.Get(base + "/api/health")
		if err != nil {
			continue
		}
		var h struct {
			OK        bool   `json:"ok"`
			PID       int    `json:"pid"`
			VaultRoot string `json:"vault_root"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&h)
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK && h.OK {
			if wantCanon != "" && h.VaultRoot != "" && canonicalOrCleanPath(h.VaultRoot) != wantCanon {
				continue
			}
			if info.PID == 0 {
				info.PID = h.PID
			}
			return base, info
		}
	}
	return "", daemonRuntimeInfo{}
}

func canonicalOrCleanPath(p string) string {
	trimmed := strings.TrimSpace(p)
	if trimmed == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(trimmed); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(trimmed)
}

// ============================================================================
// Subcommand 4: `gorness acp` (Stdio JSON-RPC 2.0 Agent Client Protocol Server)
// ============================================================================

type jsonRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func cmdACP(
	ctx context.Context,
	args []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	opts cliOptions,
) int {
	fs := flag.NewFlagSet("acp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		vaultFlag string
		wsFlag    string
	)
	fs.StringVar(&vaultFlag, "vault", "", "Path to ~/.gorness vault")
	fs.StringVar(&wsFlag, "workspace", "", "Default workspace directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	vaultRoot := resolveVaultRoot(vaultFlag, opts.userHomeDir)
	defaultWS := resolveWorkspaceDir(wsFlag)

	_, eng, err := buildCoreEngine(vaultRoot, opts)
	if err != nil {
		fmt.Fprintf(stderr, "acp bootstrap failed: %v\n", err)
		return 1
	}
	defer func() { _ = eng.Close() }()

	var writeMu sync.Mutex
	sendJSON := func(payload map[string]any) {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if enc.Encode(payload) != nil {
			return
		}
		writeMu.Lock()
		_, _ = stdout.Write(buf.Bytes())
		writeMu.Unlock()
	}

	sendResult := func(id json.RawMessage, result any) {
		sendJSON(map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"result":  result,
		})
	}

	sendError := func(id json.RawMessage, code int, msg string) {
		sendJSON(map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"error": map[string]any{
				"code":    code,
				"message": msg,
			},
		})
	}

	sendNotification := func(method string, params any) {
		sendJSON(map[string]any{
			"jsonrpc": "2.0",
			"method":  method,
			"params":  params,
		})
	}

	var (
		activeRunMu        sync.Mutex
		activeRunBySession = make(map[string]string)
		promptWG           sync.WaitGroup
	)

	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	for scanner.Scan() {
		if ctx.Err() != nil {
			break
		}
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var msg jsonRPCMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			sendError(json.RawMessage("null"), -32700, "parse error: "+err.Error())
			continue
		}

		hasID := len(bytes.TrimSpace(msg.ID)) > 0 && !bytes.Equal(bytes.TrimSpace(msg.ID), []byte("null"))

		switch msg.Method {
		case "initialize":
			var params struct {
				ProtocolVersion any `json:"protocolVersion"`
			}
			_ = json.Unmarshal(msg.Params, &params)
			protoVer := any(1)
			if params.ProtocolVersion != nil {
				protoVer = params.ProtocolVersion
			}
			sendResult(msg.ID, map[string]any{
				"protocolVersion": protoVer,
				"serverInfo": map[string]any{
					"name":    "gorness",
					"version": Version,
				},
				"agentCapabilities": map[string]any{
					"loadSession": true,
					"promptCapabilities": map[string]any{
						"embeddedContext": true,
					},
				},
			})

		case "initialized", "notifications/initialized":
			// JSON-RPC notification, no response required

		case "session/new":
			var params struct {
				Cwd       string `json:"cwd"`
				Workspace string `json:"workspace"`
				AgentRole string `json:"agentRole"`
				Title     string `json:"title"`
			}
			_ = json.Unmarshal(msg.Params, &params)
			wsDir := strings.TrimSpace(params.Cwd)
			if wsDir == "" {
				wsDir = strings.TrimSpace(params.Workspace)
			}
			if wsDir == "" {
				wsDir = defaultWS
			}
			ws, wErr := eng.Workspaces().Register(wsDir, "")
			if wErr != nil {
				sendError(msg.ID, -32602, "register workspace failed: "+wErr.Error())
				continue
			}
			role := strings.TrimSpace(params.AgentRole)
			if role == "" {
				role = "coder"
			}
			title := strings.TrimSpace(params.Title)
			if title == "" {
				title = "ACP Session"
			}
			meta, cErr := eng.Sessions().Create(ws.ID, title, role)
			if cErr != nil {
				sendError(msg.ID, -32603, "create session failed: "+cErr.Error())
				continue
			}
			sendResult(msg.ID, map[string]any{
				"sessionId":   meta.ID,
				"workspaceId": ws.ID,
				"modes": map[string]any{
					"currentModeId": "auto",
					"availableModes": []map[string]string{
						{"id": "plan", "name": "Plan"},
						{"id": "ask", "name": "Ask"},
						{"id": "accept_edits", "name": "Accept Edits"},
						{"id": "auto", "name": "Auto"},
					},
				},
			})

		case "session/load":
			var params struct {
				SessionID string `json:"sessionId"`
			}
			_ = json.Unmarshal(msg.Params, &params)
			meta, gErr := eng.Sessions().GetMeta(params.SessionID)
			if gErr != nil {
				sendError(msg.ID, -32602, gErr.Error())
				continue
			}
			entries, _ := eng.Sessions().LoadActiveBranch(meta.ID)
			sendResult(msg.ID, map[string]any{
				"sessionId":   meta.ID,
				"workspaceId": meta.WorkspaceID,
				"entries":     entries,
			})

		case "session/prompt", "session/run":
			var params struct {
				SessionID string          `json:"sessionId"`
				AgentRole string          `json:"agentRole"`
				Model     string          `json:"model"`
				Prompt    json.RawMessage `json:"prompt"`
			}
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				sendError(msg.ID, -32602, "invalid session/prompt params: "+err.Error())
				continue
			}
			meta, gErr := eng.Sessions().GetMeta(params.SessionID)
			if gErr != nil {
				sendError(msg.ID, -32602, gErr.Error())
				continue
			}
			promptText := extractACPPromptText(params.Prompt)
			if strings.TrimSpace(promptText) == "" {
				sendError(msg.ID, -32602, "empty prompt")
				continue
			}

			runID, evCh, rErr := eng.StartRun(ctx, core.RunRequest{
				WorkspaceID:   meta.WorkspaceID,
				SessionID:     meta.ID,
				Prompt:        promptText,
				AgentRole:     params.AgentRole,
				ModelOverride: params.Model,
				Unattended:    true,
			})
			if rErr != nil {
				sendError(msg.ID, -32603, rErr.Error())
				continue
			}

			sessID := meta.ID
			reqID := append(json.RawMessage(nil), msg.ID...)

			activeRunMu.Lock()
			activeRunBySession[sessID] = runID
			activeRunMu.Unlock()

			// 异步消费单轮 evCh，使主协程继续读取 stdin 并即时响应 session/cancel 中途取消请求。
			promptWG.Add(1)
			go func() {
				defer promptWG.Done()
				var (
					finalText  string
					stopReason = "end_turn"
					failedErr  string
				)
				for ev := range evCh {
					switch ev.Type {
					case core.EventThinkingDelta:
						if mp, ok := ev.Payload.(map[string]any); ok {
							if th, _ := mp["thinking"].(string); th != "" {
								sendNotification("session/update", map[string]any{
									"sessionId": sessID,
									"update": map[string]any{
										"sessionUpdate": "agent_thought_chunk",
										"content":       map[string]any{"type": "text", "text": th},
									},
								})
							}
						}
					case core.EventTextDelta:
						if mp, ok := ev.Payload.(map[string]any); ok {
							if txt, _ := mp["text"].(string); txt != "" {
								sendNotification("session/update", map[string]any{
									"sessionId": sessID,
									"update": map[string]any{
										"sessionUpdate": "agent_message_chunk",
										"content":       map[string]any{"type": "text", "text": txt},
									},
								})
							}
						}
					case core.EventToolCall:
						if tc, ok := ev.Payload.(core.ToolCallPayload); ok {
							sendNotification("session/update", map[string]any{
								"sessionId": sessID,
								"update": map[string]any{
									"sessionUpdate": "tool_call",
									"toolCallId":    tc.ToolCallID,
									"title":         tc.ToolName,
									"rawInput":      tc.Args,
								},
							})
						}
					case core.EventToolResult:
						if tr, ok := ev.Payload.(core.ToolResultPayload); ok {
							sendNotification("session/update", map[string]any{
								"sessionId": sessID,
								"update": map[string]any{
									"sessionUpdate": "tool_call_update",
									"toolCallId":    tr.ToolCallID,
									"isError":       tr.IsError,
									"output":        tr.Output,
								},
							})
						}
					case core.EventToolDeferred:
						stopReason = "deferred"
						if td, ok := ev.Payload.(core.DeferredToolPayload); ok {
							sendNotification("session/update", map[string]any{
								"sessionId": sessID,
								"update": map[string]any{
									"sessionUpdate": "tool_deferred",
									"toolCallId":    td.ToolCallID,
									"title":         td.ToolName,
									"reasonCode":    td.ReasonCode,
									"reason":        td.Reason,
								},
							})
						}
					case core.EventRunCompleted:
						if out, ok := ev.Payload.(core.RunOutcome); ok {
							finalText = out.FinalText
							if out.Status == "deferred" {
								stopReason = "deferred"
							} else if out.StopReason != "" {
								stopReason = out.StopReason
							}
						}
					case core.EventRunCancelled:
						stopReason = "cancelled"
					case core.EventRunFailed:
						if pe, ok := ev.Payload.(core.RunErrorPayload); ok {
							failedErr = pe.Message
						} else {
							failedErr = "engine run failed"
						}
					}
				}

				activeRunMu.Lock()
				if activeRunBySession[sessID] == runID {
					delete(activeRunBySession, sessID)
				}
				activeRunMu.Unlock()

				if failedErr != "" {
					sendError(reqID, -32603, failedErr)
				} else {
					sendResult(reqID, map[string]any{
						"sessionId":  sessID,
						"stopReason": stopReason,
						"finalText":  finalText,
					})
				}
			}()

		case "session/cancel":
			var params struct {
				SessionID string `json:"sessionId"`
			}
			_ = json.Unmarshal(msg.Params, &params)
			activeRunMu.Lock()
			runID := activeRunBySession[params.SessionID]
			activeRunMu.Unlock()
			if runID != "" {
				_ = eng.StopRun(runID)
			}
			if hasID {
				sendResult(msg.ID, map[string]any{"cancelled": runID != ""})
			}

		case "shutdown":
			if hasID {
				sendResult(msg.ID, map[string]any{})
			}

		default:
			if hasID {
				sendError(msg.ID, -32601, fmt.Sprintf("method not found: %s", msg.Method))
			}
		}
	}

	promptWG.Wait()
	eng.WaitBackground()
	return 0
}

func extractACPPromptText(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}
	var asStr string
	if json.Unmarshal(trimmed, &asStr) == nil {
		return asStr
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(trimmed, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if strings.TrimSpace(b.Text) != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// ============================================================================
// Bootstrap & Argument Helpers
// ============================================================================

func buildCoreEngine(vaultRoot string, opts cliOptions) (*config.Manager, *core.Engine, error) {
	cfgMgr, err := config.NewManager(config.Options{
		VaultRoot:   vaultRoot,
		UserHomeDir: opts.userHomeDir,
	})
	if err != nil {
		return nil, nil, err
	}
	var router *llm.Router
	if opts.streamer != nil {
		router = llm.NewRouter(cfgMgr, cfgMgr.Secrets(), opts.streamer)
	}
	eng, err := core.NewEngine(core.EngineOptions{
		Config:   cfgMgr,
		Router:   router,
		UserHome: opts.userHomeDir,
	})
	if err != nil {
		return nil, nil, err
	}
	return cfgMgr, eng, nil
}

func resolveVaultRoot(flagVal, injectedHome string) string {
	if trimmed := strings.TrimSpace(flagVal); trimmed != "" {
		return filepath.Clean(trimmed)
	}
	if envVal := strings.TrimSpace(os.Getenv("GORNESS_VAULT")); envVal != "" {
		return filepath.Clean(envVal)
	}
	home := strings.TrimSpace(injectedHome)
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if home == "" {
		return filepath.Join(os.TempDir(), ".gorness")
	}
	return filepath.Join(home, ".gorness")
}

func resolveWorkspaceDir(flagVal string) string {
	if trimmed := strings.TrimSpace(flagVal); trimmed != "" {
		return trimmed
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func resolveInteractiveWorkspaceDir(wsFlag, vaultRoot string) (string, error) {
	explicit := strings.TrimSpace(wsFlag) != ""
	wsPath := resolveWorkspaceDir(wsFlag)
	if !pathsOverlapVault(wsPath, vaultRoot) {
		return wsPath, nil
	}
	if explicit {
		return "", fmt.Errorf("workspace %q overlaps Gorness vault %q", wsPath, vaultRoot)
	}
	// 未显式指定 --workspace 且当前目录为 $HOME 或 ~/.gorness 时，跳过自动注册当前目录以保证正常启动。
	return "", nil
}

func pathsOverlapVault(wsPath, vaultRoot string) bool {
	canonWS := canonicalizeWithAncestors(wsPath)
	canonVault := canonicalizeWithAncestors(vaultRoot)
	if canonWS == "" || canonVault == "" {
		return false
	}
	return isPathInsideOrEqualCLI(canonWS, canonVault) || isPathInsideOrEqualCLI(canonVault, canonWS)
}

func canonicalizeWithAncestors(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		abs = filepath.Clean(trimmed)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(resolved)
	}
	cur := abs
	var tail []string
	for cur != "/" && cur != "." {
		tail = append(tail, filepath.Base(cur))
		cur = filepath.Dir(cur)
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			out := filepath.Clean(resolved)
			for i := len(tail) - 1; i >= 0; i-- {
				out = filepath.Join(out, tail[i])
			}
			return filepath.Clean(out)
		}
	}
	return filepath.Clean(abs)
}

func isPathInsideOrEqualCLI(target, root string) bool {
	if target == root {
		return true
	}
	if root == string(filepath.Separator) {
		return strings.HasPrefix(target, root)
	}
	return strings.HasPrefix(target, root+string(filepath.Separator))
}

func defaultExecRunner(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %w (%s)", name, args, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func reorderFlagsBeforePositional(args []string, boolFlags map[string]bool) []string {
	var flagsPart []string
	var posPart []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			posPart = append(posPart, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "-") {
			flagsPart = append(flagsPart, a)
			if !strings.Contains(a, "=") && !boolFlags[a] && i+1 < len(args) {
				i++
				flagsPart = append(flagsPart, args[i])
			}
			continue
		}
		posPart = append(posPart, a)
	}
	return append(flagsPart, posPart...)
}
