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
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"gorness.dev/gorness/internal/alwayson"
	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/core"
	"gorness.dev/gorness/internal/daemonctl"
	"gorness.dev/gorness/internal/fleet"
	"gorness.dev/gorness/internal/llm"
	"gorness.dev/gorness/internal/server"
	"gorness.dev/gorness/internal/store"
	"gorness.dev/gorness/internal/tui"
)

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
		fmt.Fprintf(stdout, "gorness v%s (%s/%s, zero-db)\n", config.Version, runtime.GOOS, runtime.GOARCH)
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
  gorness acp [flags]                    Start stdio JSON-RPC 2.0 ACP server (routes via daemon)
  gorness --version                      Print version
`, config.Version)
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
		standalone  bool
	)
	fs.StringVar(&vaultFlag, "vault", "", "Path to ~/.gorness vault directory")
	fs.StringVar(&wsFlag, "workspace", "", "Target workspace directory (defaults to cwd)")
	fs.StringVar(&agentRole, "agent", "coder", "Agent role profile (coder, lead, explorer, verifier, debugger)")
	fs.StringVar(&modelOver, "model", "", "Provider/model override (e.g. anthropic/claude-opus-5-5)")
	fs.StringVar(&permMode, "perm", "", "Permission mode override (plan, ask, accept_edits, auto, yolo)")
	fs.StringVar(&sandboxMode, "sandbox", "", "Seatbelt sandbox mode override")
	fs.StringVar(&sessionID, "session", "", "Resume existing session ID")
	fs.BoolVar(&jsonOutput, "json", false, "Stream NDJSON RunEvent objects to stdout")
	fs.BoolVar(&standalone, "standalone", false, "Run directly in-process without connecting to daemon")

	reordered := reorderFlagsBeforePositional(args, map[string]bool{
		"--json": true, "-json": true,
		"--standalone": true, "-standalone": true,
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

	if opts.streamer != nil || standalone {
		return runInProcessHeadless(ctx, vaultRoot, wsPath, core.RunRequest{
			SessionID:     sessionID,
			Prompt:        prompt,
			AgentRole:     agentRole,
			ModelOverride: modelOver,
			PermMode:      permMode,
			SandboxMode:   sandboxMode,
			Unattended:    true,
		}, jsonOutput, stdout, stderr, opts)
	}

	return runDaemonHeadless(ctx, vaultRoot, wsPath, core.RunRequest{
		SessionID:     sessionID,
		Prompt:        prompt,
		AgentRole:     agentRole,
		ModelOverride: modelOver,
		PermMode:      permMode,
		SandboxMode:   sandboxMode,
		Unattended:    true,
	}, jsonOutput, stdout, stderr, opts)
}

func runInProcessHeadless(
	ctx context.Context,
	vaultRoot string,
	wsPath string,
	req core.RunRequest,
	jsonOutput bool,
	stdout io.Writer,
	stderr io.Writer,
	opts cliOptions,
) int {
	_, eng, err := buildCoreEngine(vaultRoot, opts)
	if err != nil {
		fmt.Fprintf(stderr, "bootstrap engine error: %v\n", err)
		return 1
	}
	defer func() { _ = eng.Close() }()

	ws, err := eng.Workspaces().Register(wsPath, "")
	if err != nil {
		fmt.Fprintf(stderr, "register workspace %s: %v\n", wsPath, err)
		return 1
	}
	req.WorkspaceID = ws.ID

	_, evCh, err := eng.StartRun(ctx, req)
	if err != nil {
		fmt.Fprintf(stderr, "start run failed: %v\n", err)
		return 1
	}

	exitCode := consumeHeadlessRunEvents(evCh, jsonOutput, stdout, stderr)
	eng.WaitBackground()
	return exitCode
}

func runDaemonHeadless(
	ctx context.Context,
	vaultRoot string,
	wsPath string,
	req core.RunRequest,
	jsonOutput bool,
	stdout io.Writer,
	stderr io.Writer,
	opts cliOptions,
) int {
	ep, cleanup, err := connectOrStartDaemon(ctx, vaultRoot, opts)
	if err != nil {
		fmt.Fprintf(stderr, "connect gorness daemon: %v\n", err)
		return 1
	}
	defer cleanup()

	ws, _, err := daemonctl.RegisterWorkspace(ctx, ep, wsPath, "")
	if err != nil {
		fmt.Fprintf(stderr, "register workspace %s: %v\n", wsPath, err)
		return 1
	}
	req.WorkspaceID = ws.ID

	runID, evCh, err := openDaemonRunStream(ctx, ep, req)
	if err != nil {
		fmt.Fprintf(stderr, "start run failed: %v\n", err)
		return 1
	}

	streamDone := make(chan struct{})
	if runID != "" {
		go func() {
			select {
			case <-ctx.Done():
				stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				_, _ = ep.DoJSON(stopCtx, http.MethodPost, "/api/runs/"+url.PathEscape(runID)+"/stop", map[string]any{}, nil)
				cancel()
			case <-streamDone:
			}
		}()
	}

	exitCode := consumeHeadlessRunEvents(evCh, jsonOutput, stdout, stderr)
	close(streamDone)
	return exitCode
}

func consumeHeadlessRunEvents(
	evCh <-chan core.RunEvent,
	jsonOutput bool,
	stdout io.Writer,
	stderr io.Writer,
) int {
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
				if tc, ok := decodeEventPayload[core.ToolCallPayload](ev.Payload); ok {
					fmt.Fprintf(stderr, "[tool] %s\n", tc.ToolName)
				}
			case core.EventToolDeferred:
				if td, ok := decodeEventPayload[core.DeferredToolPayload](ev.Payload); ok {
					fmt.Fprintf(stderr, "[deferred] %s (%s): %s\n", td.ToolName, td.ReasonCode, td.Reason)
				}
			case core.EventRunFailed:
				if pe, ok := decodeEventPayload[core.RunErrorPayload](ev.Payload); ok && pe.Message != "" {
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
	fs.StringVar(&serverURL, "server", "", "Existing loopback server URL or unix:///path/to/daemon.sock")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	vaultRoot := resolveVaultRoot(vaultFlag, opts.userHomeDir)
	wsPath, wsErr := resolveInteractiveWorkspaceDir(wsFlag, vaultRoot)
	if wsErr != nil {
		fmt.Fprintf(stderr, "invalid workspace: %v\n", wsErr)
		return 1
	}

	trimmedServer := strings.TrimSpace(serverURL)
	if trimmedServer != "" {
		httpClient, baseURL := daemonctl.ClientForURL(trimmedServer, vaultRoot, 0)
		if err := tui.RunWithClient(ctx, baseURL, wsPath, httpClient, nil...); err != nil {
			fmt.Fprintf(stderr, "tui exited with error: %v\n", err)
			return 1
		}
		return 0
	}

	ep, cleanup, err := connectOrStartDaemon(ctx, vaultRoot, opts)
	if err != nil {
		fmt.Fprintf(stderr, "failed to start gorness daemon: %v\n", err)
		return 1
	}
	defer cleanup()

	if err := tui.RunWithClient(ctx, ep.EffectiveBaseURL(), wsPath, ep.HTTPClient(0)); err != nil {
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

	ep, cleanup, err := connectOrStartDaemon(ctx, vaultRoot, opts)
	if err != nil {
		fmt.Fprintf(stderr, "failed to ensure gorness daemon: %v\n", err)
		return 1
	}
	defer cleanup()

	if wsPath != "" {
		if _, _, regErr := daemonctl.RegisterWorkspace(ctx, ep, wsPath, ""); regErr != nil && strings.TrimSpace(wsFlag) != "" {
			fmt.Fprintf(stderr, "register workspace %s: %v\n", wsPath, regErr)
			return 1
		}
	}

	webURL := ep.WebURL
	if webURL == "" {
		webURL = ep.EffectiveBaseURL()
	}
	fmt.Fprintf(stdout, "Gorness Studio ready at %s\n", webURL)
	if !noOpen {
		launcher := opts.browserLauncher
		if launcher == nil {
			launcher = openMacStudioWindow
		}
		if err := launcher(webURL); err != nil {
			fmt.Fprintf(stderr, "warning: could not launch browser automatically (%v); open %s manually.\n", err, webURL)
		}
	}
	return 0
}

func openMacStudioWindow(targetURL string) error {
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
		return daemonStatus(ctx, subArgs, stdout, stderr, opts)
	case "stop":
		return daemonStop(ctx, subArgs, stdout, stderr, opts)
	case "install":
		return daemonInstall(ctx, subArgs, stdout, stderr, opts)
	case "uninstall":
		return daemonUninstall(ctx, subArgs, stdout, stderr, opts)
	default:
		fmt.Fprintf(stderr, "unknown daemon action %q (expected serve|status|stop|install|uninstall)\n", sub)
		return 2
	}
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
	unlock, ok, err := store.TryFlock(daemonctl.DaemonLockPath(paths))
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
		host = daemonctl.DefaultHost
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		fmt.Fprintf(stderr, "refusing to bind non-loopback host %q\n", host)
		return 1
	}
	port := portFlag
	if port < 0 {
		port = snap.Global.Port
		if port <= 0 {
			port = daemonctl.DefaultPort
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
		Version:  config.Version,
	})
	defer func() { _ = srv.Close() }()

	tcpAddr := net.JoinHostPort(host, strconv.Itoa(port))
	tcpLn, err := net.Listen("tcp", tcpAddr)
	if err != nil && portFlag < 0 {
		// Fall back cleanly to an ephemeral loopback port when another vault already holds default port 3456.
		tcpLn, err = net.Listen("tcp", net.JoinHostPort(host, "0"))
	}
	if err != nil {
		fmt.Fprintf(stderr, "listen %s: %v\n", tcpAddr, err)
		return 1
	}
	defer tcpLn.Close()

	actualPort := tcpLn.Addr().(*net.TCPAddr).Port
	urlStr := fmt.Sprintf("http://%s:%d", host, actualPort)

	sockPath := paths.SocketPath()
	_ = os.Remove(sockPath)
	var activeSockPath string
	unixLn, unixErr := net.Listen("unix", sockPath)
	if unixErr == nil {
		activeSockPath = sockPath
		_ = os.Chmod(sockPath, 0o600)
		defer func() {
			_ = unixLn.Close()
			_ = os.Remove(sockPath)
		}()
	}

	rec := daemonctl.RuntimeRecord{
		PID:        os.Getpid(),
		VaultRoot:  daemonctl.CanonicalVaultPath(paths.Root),
		Host:       host,
		Port:       actualPort,
		URL:        urlStr,
		SocketPath: activeSockPath,
		Version:    config.Version,
		StartedAt:  time.Now().UTC(),
	}
	if err := daemonctl.WriteRuntimeRecord(paths, rec); err == nil {
		defer func() { _ = daemonctl.RemoveRuntimeRecord(paths) }()
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

	fmt.Fprintf(stdout, "gorness daemon serving at %s (pid=%d, vault=%s)\n", urlStr, rec.PID, paths.Root)

	select {
	case <-ctx.Done():
	case serveErr := <-errCh:
		fmt.Fprintf(stderr, "daemon server error: %v\n", serveErr)
		return 1
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutCtx)
	eng.WaitBackground()
	return 0
}

func daemonStatus(ctx context.Context, args []string, stdout, stderr io.Writer, opts cliOptions) int {
	fs := flag.NewFlagSet("daemon status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var vaultFlag string
	fs.StringVar(&vaultFlag, "vault", "", "Path to ~/.gorness vault")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	vaultRoot := resolveVaultRoot(vaultFlag, opts.userHomeDir)
	ep, ok := daemonctl.Probe(ctx, vaultRoot)
	if !ok {
		fmt.Fprintln(stdout, "gorness daemon: stopped")
		return 1
	}
	displayURL := ep.WebURL
	if displayURL == "" {
		displayURL = ep.EffectiveBaseURL()
	}
	fmt.Fprintf(stdout, "gorness daemon: running at %s (pid=%d)\n", displayURL, ep.Record.PID)
	return 0
}

func daemonStop(ctx context.Context, args []string, stdout, stderr io.Writer, opts cliOptions) int {
	fs := flag.NewFlagSet("daemon stop", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var vaultFlag string
	fs.StringVar(&vaultFlag, "vault", "", "Path to ~/.gorness vault")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	vaultRoot := resolveVaultRoot(vaultFlag, opts.userHomeDir)
	pid, stopped, err := daemonctl.StopDaemon(ctx, vaultRoot, 3*time.Second)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	if !stopped {
		fmt.Fprintln(stdout, "gorness daemon is not running")
		return 0
	}
	fmt.Fprintf(stdout, "stopped gorness daemon (pid=%d)\n", pid)
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
	return daemonctl.DaemonLogPath(paths)
}

func probeRunningDaemon(vaultRoot string) (string, daemonctl.RuntimeRecord) {
	ep, ok := daemonctl.Probe(context.Background(), vaultRoot)
	if !ok || ep == nil {
		return "", daemonctl.RuntimeRecord{}
	}
	if ep.WebURL != "" {
		return ep.WebURL, ep.Record
	}
	return ep.EffectiveBaseURL(), ep.Record
}

func connectOrStartDaemon(ctx context.Context, vaultRoot string, opts cliOptions) (*daemonctl.Endpoint, func(), error) {
	if ep, ok := daemonctl.Probe(ctx, vaultRoot); ok {
		return ep, func() {}, nil
	}
	if isGoTestBinary() {
		return startEphemeralTestDaemon(ctx, vaultRoot, opts)
	}
	ep, err := daemonctl.EnsureDaemon(ctx, daemonctl.EnsureOptions{
		VaultRoot: vaultRoot,
		HomeDir:   opts.userHomeDir,
	})
	if err != nil {
		return nil, func() {}, err
	}
	return ep, func() {}, nil
}

func isGoTestBinary() bool {
	if flag.Lookup("test.v") != nil {
		return true
	}
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return strings.HasSuffix(exe, ".test") || strings.HasSuffix(exe, ".test.exe")
}

func startEphemeralTestDaemon(parentCtx context.Context, vaultRoot string, opts cliOptions) (*daemonctl.Endpoint, func(), error) {
	dCtx, dCancel := context.WithCancel(context.Background())
	var dOut, dErr bytes.Buffer
	doneCh := make(chan int, 1)
	go func() {
		doneCh <- daemonServe(dCtx, []string{
			"--vault", vaultRoot,
			"--host", "127.0.0.1",
			"--port", "0",
		}, &dOut, &dErr, opts)
	}()

	cleanup := func() {
		dCancel()
		<-doneCh
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if parentCtx.Err() != nil {
			cleanup()
			return nil, func() {}, parentCtx.Err()
		}
		if ep, ok := daemonctl.Probe(parentCtx, vaultRoot); ok {
			return ep, cleanup, nil
		}
		select {
		case code := <-doneCh:
			return nil, func() {}, fmt.Errorf("daemon exited early (code=%d): %s", code, strings.TrimSpace(dErr.String()))
		case <-parentCtx.Done():
			cleanup()
			return nil, func() {}, parentCtx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	cleanup()
	return nil, func() {}, fmt.Errorf("timed out waiting for daemon: %s", strings.TrimSpace(dErr.String()))
}

func openDaemonRunStream(ctx context.Context, ep *daemonctl.Endpoint, req core.RunRequest) (string, <-chan core.RunEvent, error) {
	resp, err := ep.OpenStream(ctx, http.MethodPost, "/api/runs/stream", req)
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return "", nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	runID := strings.TrimSpace(resp.Header.Get("X-Gorness-Run-ID"))
	evCh := make(chan core.RunEvent, 64)
	go func() {
		defer close(evCh)
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			raw := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if raw == "" {
				continue
			}
			var ev core.RunEvent
			if json.Unmarshal([]byte(raw), &ev) != nil || ev.Type == "heartbeat" || ev.Type == "connected" {
				continue
			}
			select {
			case evCh <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return runID, evCh, nil
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

type acpBackend interface {
	RegisterWorkspace(ctx context.Context, wsDir string) (*store.Workspace, error)
	CreateSession(ctx context.Context, wsID, title, agentRole string) (*store.SessionMeta, error)
	LoadSession(ctx context.Context, sessionID string) (*store.SessionMeta, []store.SessionEntry, error)
	UpdateSessionAgentRole(ctx context.Context, sessionID, agentRole string) (*store.SessionMeta, error)
	StartRun(ctx context.Context, req core.RunRequest) (string, <-chan core.RunEvent, error)
	StopRun(ctx context.Context, runID string) error
	Close()
}

type engineACPBackend struct {
	eng *core.Engine
}

func (b *engineACPBackend) RegisterWorkspace(_ context.Context, wsDir string) (*store.Workspace, error) {
	return b.eng.Workspaces().Register(wsDir, "")
}

func (b *engineACPBackend) CreateSession(_ context.Context, wsID, title, agentRole string) (*store.SessionMeta, error) {
	return b.eng.Sessions().Create(wsID, title, agentRole)
}

func (b *engineACPBackend) LoadSession(_ context.Context, sessionID string) (*store.SessionMeta, []store.SessionEntry, error) {
	meta, err := b.eng.Sessions().GetMeta(sessionID)
	if err != nil {
		return nil, nil, err
	}
	entries, _ := b.eng.Sessions().LoadActiveBranch(meta.ID)
	return meta, entries, nil
}

func (b *engineACPBackend) UpdateSessionAgentRole(_ context.Context, sessionID, agentRole string) (*store.SessionMeta, error) {
	return b.eng.Sessions().UpdateMeta(sessionID, func(m *store.SessionMeta) {
		m.AgentRole = strings.TrimSpace(agentRole)
	})
}

func (b *engineACPBackend) StartRun(ctx context.Context, req core.RunRequest) (string, <-chan core.RunEvent, error) {
	return b.eng.StartRun(ctx, req)
}

func (b *engineACPBackend) StopRun(_ context.Context, runID string) error {
	return b.eng.StopRun(runID)
}

func (b *engineACPBackend) Close() {
	b.eng.WaitBackground()
	_ = b.eng.Close()
}

type daemonACPBackend struct {
	ep      *daemonctl.Endpoint
	cleanup func()
}

func (b *daemonACPBackend) RegisterWorkspace(ctx context.Context, wsDir string) (*store.Workspace, error) {
	ws, _, err := daemonctl.RegisterWorkspace(ctx, b.ep, wsDir, "")
	return ws, err
}

func (b *daemonACPBackend) CreateSession(ctx context.Context, wsID, title, agentRole string) (*store.SessionMeta, error) {
	var meta store.SessionMeta
	_, err := b.ep.DoJSON(ctx, http.MethodPost, "/api/sessions", map[string]any{
		"workspace_id": wsID,
		"title":        title,
		"agent_role":   agentRole,
	}, &meta)
	if err != nil {
		return nil, err
	}
	return &meta, nil
}

func (b *daemonACPBackend) LoadSession(ctx context.Context, sessionID string) (*store.SessionMeta, []store.SessionEntry, error) {
	var detail struct {
		Meta    *store.SessionMeta   `json:"meta"`
		Entries []store.SessionEntry `json:"entries"`
	}
	_, err := b.ep.DoJSON(ctx, http.MethodGet, "/api/sessions/"+url.PathEscape(sessionID), nil, &detail)
	if err != nil {
		return nil, nil, err
	}
	if detail.Meta == nil {
		return nil, nil, errors.New("session not found")
	}
	return detail.Meta, detail.Entries, nil
}

func (b *daemonACPBackend) UpdateSessionAgentRole(ctx context.Context, sessionID, agentRole string) (*store.SessionMeta, error) {
	var updated store.SessionMeta
	_, err := b.ep.DoJSON(ctx, http.MethodPatch, "/api/sessions/"+url.PathEscape(sessionID), map[string]any{
		"agent_role": strings.TrimSpace(agentRole),
	}, &updated)
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func (b *daemonACPBackend) StartRun(ctx context.Context, req core.RunRequest) (string, <-chan core.RunEvent, error) {
	return openDaemonRunStream(ctx, b.ep, req)
}

func (b *daemonACPBackend) StopRun(ctx context.Context, runID string) error {
	stopCtx := ctx
	if stopCtx == nil || stopCtx.Err() != nil {
		var cancel context.CancelFunc
		stopCtx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
	}
	_, err := b.ep.DoJSON(stopCtx, http.MethodPost, "/api/runs/"+url.PathEscape(runID)+"/stop", map[string]any{}, nil)
	return err
}

func (b *daemonACPBackend) Close() {
	if b.cleanup != nil {
		b.cleanup()
	}
}

func availableACPModes() []map[string]string {
	return []map[string]string{
		{"id": "plan", "name": "Plan"},
		{"id": "ask", "name": "Ask"},
		{"id": "accept_edits", "name": "Accept Edits"},
		{"id": "auto", "name": "Auto"},
		{"id": "yolo", "name": "YOLO"},
	}
}

func isACPPermMode(modeID string) bool {
	switch strings.ToLower(strings.TrimSpace(modeID)) {
	case "plan", "ask", "accept_edits", "auto", "yolo":
		return true
	default:
		return false
	}
}

func isKnownBuiltinAgentRole(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "coder", "lead", "explorer", "debugger", "verifier":
		return true
	default:
		return false
	}
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
		vaultFlag  string
		wsFlag     string
		standalone bool
	)
	fs.StringVar(&vaultFlag, "vault", "", "Path to ~/.gorness vault")
	fs.StringVar(&wsFlag, "workspace", "", "Default workspace directory")
	fs.BoolVar(&standalone, "standalone", false, "Run ACP directly in-process without connecting to daemon")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	vaultRoot := resolveVaultRoot(vaultFlag, opts.userHomeDir)
	defaultWS := resolveWorkspaceDir(wsFlag)

	var backend acpBackend
	if opts.streamer != nil || standalone {
		_, eng, err := buildCoreEngine(vaultRoot, opts)
		if err != nil {
			fmt.Fprintf(stderr, "acp bootstrap failed: %v\n", err)
			return 1
		}
		backend = &engineACPBackend{eng: eng}
	} else {
		ep, cleanup, err := connectOrStartDaemon(ctx, vaultRoot, opts)
		if err != nil {
			fmt.Fprintf(stderr, "acp connect daemon failed: %v\n", err)
			return 1
		}
		backend = &daemonACPBackend{ep: ep, cleanup: cleanup}
	}
	defer backend.Close()

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
		stateMu            sync.Mutex
		activeRunBySession = make(map[string]string)
		modeBySession      = make(map[string]string)
		promptWG           sync.WaitGroup
	)

	getSessionMode := func(sessID string) string {
		stateMu.Lock()
		defer stateMu.Unlock()
		if m := modeBySession[sessID]; m != "" {
			return m
		}
		return "auto"
	}

	setSessionMode := func(sessID, modeID string) {
		stateMu.Lock()
		modeBySession[sessID] = modeID
		stateMu.Unlock()
	}

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
					"version": config.Version,
				},
				"agentCapabilities": map[string]any{
					"loadSession": true,
					"setMode":     true,
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
				ModeID    string `json:"modeId"`
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
			ws, wErr := backend.RegisterWorkspace(ctx, wsDir)
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
			meta, cErr := backend.CreateSession(ctx, ws.ID, title, role)
			if cErr != nil {
				sendError(msg.ID, -32603, "create session failed: "+cErr.Error())
				continue
			}
			initialMode := "auto"
			if isACPPermMode(params.ModeID) {
				initialMode = strings.ToLower(strings.TrimSpace(params.ModeID))
			}
			setSessionMode(meta.ID, initialMode)

			sendResult(msg.ID, map[string]any{
				"sessionId":   meta.ID,
				"workspaceId": ws.ID,
				"agentRole":   meta.AgentRole,
				"modes": map[string]any{
					"currentModeId":  initialMode,
					"availableModes": availableACPModes(),
				},
			})

		case "session/load":
			var params struct {
				SessionID string `json:"sessionId"`
			}
			_ = json.Unmarshal(msg.Params, &params)
			meta, entries, gErr := backend.LoadSession(ctx, params.SessionID)
			if gErr != nil {
				sendError(msg.ID, -32602, gErr.Error())
				continue
			}
			curMode := getSessionMode(meta.ID)
			sendResult(msg.ID, map[string]any{
				"sessionId":   meta.ID,
				"workspaceId": meta.WorkspaceID,
				"agentRole":   meta.AgentRole,
				"entries":     entries,
				"modes": map[string]any{
					"currentModeId":  curMode,
					"availableModes": availableACPModes(),
				},
			})

		case "session/set_mode":
			var params struct {
				SessionID string `json:"sessionId"`
				ModeID    string `json:"modeId"`
				Mode      string `json:"mode"`
				AgentRole string `json:"agentRole"`
			}
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				sendError(msg.ID, -32602, "invalid session/set_mode params: "+err.Error())
				continue
			}
			if strings.TrimSpace(params.SessionID) == "" {
				sendError(msg.ID, -32602, "sessionId is required")
				continue
			}
			meta, _, gErr := backend.LoadSession(ctx, params.SessionID)
			if gErr != nil {
				sendError(msg.ID, -32602, gErr.Error())
				continue
			}

			rawMode := strings.TrimSpace(params.ModeID)
			if rawMode == "" {
				rawMode = strings.TrimSpace(params.Mode)
			}
			reqRole := strings.TrimSpace(params.AgentRole)
			if rawMode == "" && reqRole == "" {
				sendError(msg.ID, -32602, "modeId or agentRole is required")
				continue
			}

			effectiveMode := getSessionMode(meta.ID)
			effectiveRole := meta.AgentRole

			if strings.HasPrefix(strings.ToLower(rawMode), "role:") {
				roleFromMode := strings.TrimSpace(rawMode[len("role:"):])
				if roleFromMode == "" {
					sendError(msg.ID, -32602, "empty role in modeId")
					continue
				}
				reqRole = roleFromMode
			} else if rawMode != "" {
				lowerMode := strings.ToLower(rawMode)
				if isACPPermMode(lowerMode) {
					effectiveMode = lowerMode
					setSessionMode(meta.ID, effectiveMode)
				} else if isKnownBuiltinAgentRole(lowerMode) && reqRole == "" {
					reqRole = lowerMode
				} else {
					sendError(msg.ID, -32602, fmt.Sprintf("unsupported modeId %q", rawMode))
					continue
				}
			}

			if reqRole != "" {
				updatedMeta, uErr := backend.UpdateSessionAgentRole(ctx, meta.ID, reqRole)
				if uErr != nil {
					sendError(msg.ID, -32603, "update session role failed: "+uErr.Error())
					continue
				}
				effectiveRole = updatedMeta.AgentRole
			}

			sendNotification("session/update", map[string]any{
				"sessionId": meta.ID,
				"update": map[string]any{
					"sessionUpdate": "current_mode_update",
					"currentModeId": effectiveMode,
					"agentRole":     effectiveRole,
				},
			})
			if hasID {
				sendResult(msg.ID, map[string]any{
					"sessionId":     meta.ID,
					"currentModeId": effectiveMode,
					"agentRole":     effectiveRole,
				})
			}

		case "session/prompt", "session/run":
			var params struct {
				SessionID string          `json:"sessionId"`
				AgentRole string          `json:"agentRole"`
				Model     string          `json:"model"`
				ModeID    string          `json:"modeId"`
				PermMode  string          `json:"permMode"`
				Prompt    json.RawMessage `json:"prompt"`
			}
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				sendError(msg.ID, -32602, "invalid session/prompt params: "+err.Error())
				continue
			}
			meta, _, gErr := backend.LoadSession(ctx, params.SessionID)
			if gErr != nil {
				sendError(msg.ID, -32602, gErr.Error())
				continue
			}
			promptText := extractACPPromptText(params.Prompt)
			if strings.TrimSpace(promptText) == "" {
				sendError(msg.ID, -32602, "empty prompt")
				continue
			}

			role := strings.TrimSpace(params.AgentRole)
			if role == "" {
				role = meta.AgentRole
			}
			permMode := strings.TrimSpace(params.PermMode)
			if permMode == "" && isACPPermMode(params.ModeID) {
				permMode = strings.ToLower(strings.TrimSpace(params.ModeID))
			}
			if permMode == "" {
				permMode = getSessionMode(meta.ID)
			}

			runID, evCh, rErr := backend.StartRun(ctx, core.RunRequest{
				WorkspaceID:   meta.WorkspaceID,
				SessionID:     meta.ID,
				Prompt:        promptText,
				AgentRole:     role,
				ModelOverride: params.Model,
				PermMode:      permMode,
				Unattended:    true,
			})
			if rErr != nil {
				sendError(msg.ID, -32603, rErr.Error())
				continue
			}

			sessID := meta.ID
			reqID := append(json.RawMessage(nil), msg.ID...)

			stateMu.Lock()
			activeRunBySession[sessID] = runID
			stateMu.Unlock()

			promptWG.Add(1)
			go func() {
				defer promptWG.Done()
				var (
					finalText  string
					stopReason = "end_turn"
					failedErr  string
				)
				for ev := range evCh {
					if runID == "" && ev.RunID != "" {
						stateMu.Lock()
						if activeRunBySession[sessID] == "" {
							activeRunBySession[sessID] = ev.RunID
						}
						runID = ev.RunID
						stateMu.Unlock()
					}
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
						if tc, ok := decodeEventPayload[core.ToolCallPayload](ev.Payload); ok {
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
						if tr, ok := decodeEventPayload[core.ToolResultPayload](ev.Payload); ok {
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
						if td, ok := decodeEventPayload[core.DeferredToolPayload](ev.Payload); ok {
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
						if out, ok := decodeEventPayload[core.RunOutcome](ev.Payload); ok {
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
						if pe, ok := decodeEventPayload[core.RunErrorPayload](ev.Payload); ok && pe.Message != "" {
							failedErr = pe.Message
						} else {
							failedErr = "engine run failed"
						}
					}
				}

				stateMu.Lock()
				if activeRunBySession[sessID] == runID {
					delete(activeRunBySession, sessID)
				}
				stateMu.Unlock()

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
			stateMu.Lock()
			runID := activeRunBySession[params.SessionID]
			stateMu.Unlock()
			if runID != "" {
				_ = backend.StopRun(ctx, runID)
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

func decodeEventPayload[T any](payload any) (T, bool) {
	var zero T
	if payload == nil {
		return zero, false
	}
	if typed, ok := payload.(T); ok {
		return typed, true
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return zero, false
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero, false
	}
	return out, true
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
	return daemonctl.ResolveVaultRoot(flagVal, nil, injectedHome)
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
	if !daemonctl.PathsOverlapVault(wsPath, vaultRoot) {
		return wsPath, nil
	}
	if explicit {
		return "", fmt.Errorf("workspace %q overlaps Gorness vault %q", wsPath, vaultRoot)
	}
	return "", nil
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
