package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/core"
	"gorness.dev/gorness/internal/daemonctl"
	"gorness.dev/gorness/internal/llm"
	"gorness.dev/gorness/internal/store"
	"gorness.dev/gorness/internal/tui"
)

func startMockAnthropicSSEServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		writeSSE := func(event string, data any) {
			raw, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(raw))
			if flusher != nil {
				flusher.Flush()
			}
		}

		writeSSE("message_start", map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"usage": map[string]any{"input_tokens": 24},
			},
		})
		writeSSE("content_block_start", map[string]any{
			"type":  "content_block_start",
			"index": 0,
			"content_block": map[string]any{
				"type": "text",
				"text": "",
			},
		})
		writeSSE("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]any{
				"type": "text_delta",
				"text": "Gorness headless verification passed.",
			},
		})
		writeSSE("content_block_stop", map[string]any{
			"type":  "content_block_stop",
			"index": 0,
		})
		writeSSE("message_delta", map[string]any{
			"type": "message_delta",
			"delta": map[string]any{
				"stop_reason": "end_turn",
			},
			"usage": map[string]any{
				"output_tokens": 9,
			},
		})
		writeSSE("message_stop", map[string]any{"type": "message_stop"})
	}))
}

func prepareTempVaultWithMockProvider(t *testing.T, baseURL string) (vaultDir, homeDir, wsDir string) {
	t.Helper()
	root := t.TempDir()
	homeDir = filepath.Join(root, "home")
	vaultDir = filepath.Join(homeDir, ".gorness")
	wsDir = filepath.Join(root, "sample-project")
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		t.Fatalf("mkdir homeDir: %v", err)
	}
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("mkdir wsDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	_, err := config.NewManager(config.Options{
		VaultRoot:   vaultDir,
		UserHomeDir: homeDir,
	})
	if err != nil {
		t.Fatalf("bootstrap vault: %v", err)
	}

	providersTOML := fmt.Sprintf(`[providers.anthropic]
name = "Mock Anthropic"
protocol = "anthropic"
base_url = %q
api_key = "env:GORNESS_TEST_MOCK_KEY"
timeout_sec = 30

[providers.anthropic.models."claude-sonnet-5-5"]
name = "Claude Sonnet 5.5"
context_window = 200000
max_output_tokens = 8192
input_cost_per_1m = 3.0
output_cost_per_1m = 15.0

[providers.anthropic.models."claude-haiku-5-5"]
name = "Claude Haiku 5.5"
context_window = 200000
max_output_tokens = 4096
input_cost_per_1m = 0.8
output_cost_per_1m = 4.0
`, baseURL)
	if err := os.WriteFile(filepath.Join(vaultDir, "providers.toml"), []byte(providersTOML), 0o600); err != nil {
		t.Fatalf("write providers.toml: %v", err)
	}
	t.Setenv("GORNESS_TEST_MOCK_KEY", "sk-mock-secret-value-123456")
	return vaultDir, homeDir, wsDir
}

func TestCLIHeadlessJSONAndACPRoundTrip(t *testing.T) {
	mockLLM := startMockAnthropicSSEServer(t)
	defer mockLLM.Close()

	vaultDir, homeDir, wsDir := prepareTempVaultWithMockProvider(t, mockLLM.URL)

	t.Run("headless_run_ndjson", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		code := runCLI(ctx, []string{
			"run",
			"--vault", vaultDir,
			"--workspace", wsDir,
			"--agent", "coder",
			"--json",
			"Verify repository readiness",
		}, strings.NewReader(""), &stdout, &stderr, cliOptions{
			userHomeDir: homeDir,
		})
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d (stderr=%s, stdout=%s)", code, stderr.String(), stdout.String())
		}

		lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
		if len(lines) == 0 || lines[0] == "" {
			t.Fatalf("expected non-empty NDJSON output from gorness run --json")
		}

		var (
			sawStarted   bool
			sawDelta     bool
			sawCompleted bool
		)
		for i, line := range lines {
			var ev core.RunEvent
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				t.Fatalf("line %d is not valid JSON (%q): %v", i+1, line, err)
			}
			switch ev.Type {
			case core.EventRunStarted:
				sawStarted = true
			case core.EventTextDelta:
				sawDelta = true
			case core.EventRunCompleted:
				sawCompleted = true
			}
		}
		if !sawStarted || !sawDelta || !sawCompleted {
			t.Fatalf("missing expected NDJSON lifecycle events: started=%v delta=%v completed=%v\n%s",
				sawStarted, sawDelta, sawCompleted, stdout.String())
		}
	})

	t.Run("acp_stdio_initialize_and_session_roundtrip", func(t *testing.T) {
		inR, inW := io.Pipe()
		outR, outW := io.Pipe()
		var stderr bytes.Buffer

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		doneCh := make(chan int, 1)
		go func() {
			code := runCLI(ctx, []string{
				"acp",
				"--vault", vaultDir,
			}, inR, outW, &stderr, cliOptions{
				userHomeDir: homeDir,
			})
			_ = outW.Close()
			doneCh <- code
		}()

		reader := bufio.NewReader(outR)
		sendRPC := func(msg map[string]any) {
			t.Helper()
			b, err := json.Marshal(msg)
			if err != nil {
				t.Fatalf("marshal rpc: %v", err)
			}
			if _, err := inW.Write(append(b, '\n')); err != nil {
				t.Fatalf("write rpc: %v", err)
			}
		}
		readRPCResponse := func(wantID int) map[string]any {
			t.Helper()
			for {
				line, err := reader.ReadBytes('\n')
				if err != nil {
					t.Fatalf("read rpc line waiting for id=%d: %v (stderr=%s)", wantID, err, stderr.String())
				}
				var obj map[string]any
				if err := json.Unmarshal(bytes.TrimSpace(line), &obj); err != nil {
					t.Fatalf("invalid JSON-RPC line %q: %v", string(line), err)
				}
				if idVal, hasID := obj["id"]; hasID && int(idVal.(float64)) == wantID {
					return obj
				}
			}
		}

		// 1. ACP initialize handshake
		sendRPC(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "initialize",
			"params": map[string]any{
				"protocolVersion":    1,
				"clientCapabilities": map[string]any{},
			},
		})
		initResp := readRPCResponse(1)
		if initResp["error"] != nil {
			t.Fatalf("ACP initialize returned error: %+v", initResp["error"])
		}
		initResult, _ := initResp["result"].(map[string]any)
		if initResult == nil || initResult["protocolVersion"] == nil {
			t.Fatalf("ACP initialize missing protocolVersion in result: %+v", initResp)
		}

		// 2. ACP session/new handshake
		sendRPC(map[string]any{
			"jsonrpc": "2.0",
			"id":      2,
			"method":  "session/new",
			"params": map[string]any{
				"cwd":       wsDir,
				"agentRole": "coder",
			},
		})
		sessResp := readRPCResponse(2)
		if sessResp["error"] != nil {
			t.Fatalf("ACP session/new returned error: %+v", sessResp["error"])
		}
		sessResult, _ := sessResp["result"].(map[string]any)
		sessionID, _ := sessResult["sessionId"].(string)
		if sessionID == "" {
			t.Fatalf("ACP session/new returned empty sessionId: %+v", sessResp)
		}

		// 3. ACP session/prompt routed through transactional Engine
		sendRPC(map[string]any{
			"jsonrpc": "2.0",
			"id":      3,
			"method":  "session/prompt",
			"params": map[string]any{
				"sessionId": sessionID,
				"prompt": []map[string]any{
					{"type": "text", "text": "Ping engine over ACP"},
				},
			},
		})
		promptResp := readRPCResponse(3)
		if promptResp["error"] != nil {
			t.Fatalf("ACP session/prompt returned error: %+v", promptResp["error"])
		}

		_ = inW.Close()
		select {
		case code := <-doneCh:
			if code != 0 {
				t.Fatalf("expected gorness acp exit code 0, got %d (stderr=%s)", code, stderr.String())
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for gorness acp to exit after stdin EOF")
		}
	})

	t.Run("daemon_serve_status_and_tui_model_view", func(t *testing.T) {
		dCtx, dCancel := context.WithCancel(context.Background())
		defer dCancel()

		var dOut, dErr bytes.Buffer
		dDone := make(chan int, 1)
		go func() {
			dDone <- runCLI(dCtx, []string{
				"daemon", "serve",
				"--vault", vaultDir,
				"--host", "127.0.0.1",
				"--port", "0",
			}, strings.NewReader(""), &dOut, &dErr, cliOptions{
				userHomeDir: homeDir,
			})
		}()

		var daemonURL string
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			u, _ := probeRunningDaemon(vaultDir)
			if u != "" {
				daemonURL = u
				break
			}
			time.Sleep(30 * time.Millisecond)
		}
		if daemonURL == "" {
			t.Fatalf("daemon did not become healthy (stderr=%s)", dErr.String())
		}

		var stOut, stErr bytes.Buffer
		if stCode := runCLI(context.Background(), []string{"daemon", "status", "--vault", vaultDir}, strings.NewReader(""), &stOut, &stErr, cliOptions{userHomeDir: homeDir}); stCode != 0 {
			t.Fatalf("expected daemon status 0, got %d: %s", stCode, stErr.String())
		}

		// Exercise Bubble Tea TUI Model lifecycle against live loopback daemon
		m := tui.NewModel(dCtx, daemonURL, wsDir)
		rendered := m.View()
		if !strings.Contains(rendered, "Gorness TUI") {
			t.Fatalf("unexpected TUI View output:\n%s", rendered)
		}

		dCancel()
		select {
		case code := <-dDone:
			if code != 0 {
				t.Fatalf("expected daemon serve clean exit 0, got %d (stderr=%s)", code, dErr.String())
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for daemon serve shutdown")
		}

		// Verify strict Zero-DB file format invariant inside vaultDir
		if err := filepath.WalkDir(vaultDir, func(path string, d os.DirEntry, wErr error) error {
			if wErr != nil {
				return wErr
			}
			if !d.IsDir() && strings.EqualFold(filepath.Ext(d.Name()), ".json") {
				t.Fatalf("Zero-DB violation: forbidden .json file in vault: %s", path)
			}
			return nil
		}); err != nil {
			t.Fatalf("walk vaultDir: %v", err)
		}
	})
}

func TestHeadlessGateAskDeferAndACPMidRunCancel(t *testing.T) {
	mockLLM := startMockAnthropicSSEServer(t)
	defer mockLLM.Close()

	vaultDir, homeDir, wsDir := prepareTempVaultWithMockProvider(t, mockLLM.URL)

	t.Run("headless_run_gate_ask_defers_instead_of_hanging", func(t *testing.T) {
		askToolStreamer := llm.StreamerFunc(func(_ context.Context, _ llm.Request) (<-chan llm.StreamEvent, error) {
			ch := make(chan llm.StreamEvent, 4)
			args, _ := json.Marshal(map[string]any{"command": "git push origin main"})
			ch <- llm.StreamEvent{
				Type:       llm.StreamToolCall,
				ToolCallID: "call_push_1",
				ToolName:   "run_command",
				ToolInput:  args,
			}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use", InputTokens: 15, OutputTokens: 10}
			close(ch)
			return ch, nil
		})

		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()

		var stdout, stderr bytes.Buffer
		code := runCLI(ctx, []string{
			"run",
			"--vault", vaultDir,
			"--workspace", wsDir,
			"--perm", "ask",
			"--json",
			"Push branch",
		}, strings.NewReader(""), &stdout, &stderr, cliOptions{
			userHomeDir: homeDir,
			streamer:    askToolStreamer,
		})
		if code != 0 {
			t.Fatalf("expected exit code 0 on unattended GateDefer, got %d (stderr=%s)", code, stderr.String())
		}

		var sawDeferred, sawCompletedDeferred bool
		for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var ev struct {
				Type    string `json:"type"`
				Payload struct {
					Status string `json:"status"`
				} `json:"payload"`
			}
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				t.Fatalf("invalid NDJSON line %q: %v", line, err)
			}
			if ev.Type == core.EventToolDeferred {
				sawDeferred = true
			}
			if ev.Type == core.EventRunCompleted && ev.Payload.Status == "deferred" {
				sawCompletedDeferred = true
			}
		}
		if !sawDeferred || !sawCompletedDeferred {
			t.Fatalf("expected tool_deferred and run_completed(status=deferred), got:\n%s", stdout.String())
		}
	})

	t.Run("acp_gate_ask_defers_and_session_cancel_interrupts_mid_run", func(t *testing.T) {
		startedSlowCh := make(chan struct{}, 1)
		acpStreamer := llm.StreamerFunc(func(sCtx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
			var lastPrompt string
			if len(req.History) > 0 {
				for _, b := range req.History[len(req.History)-1].Blocks {
					if b.Text != "" {
						lastPrompt = b.Text
					}
				}
			}
			ch := make(chan llm.StreamEvent, 4)
			if strings.Contains(lastPrompt, "SLOW_BLOCKING_RUN") {
				select {
				case startedSlowCh <- struct{}{}:
				default:
				}
				go func() {
					defer close(ch)
					ch <- llm.StreamEvent{Type: llm.StreamTextDelta, Text: "starting slow work..."}
					<-sCtx.Done()
					ch <- llm.StreamEvent{Type: llm.StreamError, Err: sCtx.Err()}
				}()
				return ch, nil
			}
			args, _ := json.Marshal(map[string]any{"command": "git push origin main"})
			ch <- llm.StreamEvent{
				Type:       llm.StreamToolCall,
				ToolCallID: "call_acp_push_1",
				ToolName:   "run_command",
				ToolInput:  args,
			}
			ch <- llm.StreamEvent{Type: llm.StreamDone, StopReason: "tool_use"}
			close(ch)
			return ch, nil
		})

		inR, inW := io.Pipe()
		outR, outW := io.Pipe()
		var stderr bytes.Buffer

		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		doneCh := make(chan int, 1)
		go func() {
			code := runCLI(ctx, []string{"acp", "--vault", vaultDir}, inR, outW, &stderr, cliOptions{
				userHomeDir: homeDir,
				streamer:    acpStreamer,
			})
			_ = outW.Close()
			doneCh <- code
		}()

		responsesByID := make(map[int]map[string]any)
		var respMu sync.Mutex
		respSignal := make(chan struct{}, 32)
		go func() {
			r := bufio.NewReader(outR)
			for {
				line, err := r.ReadBytes('\n')
				if err != nil {
					return
				}
				var obj map[string]any
				if json.Unmarshal(bytes.TrimSpace(line), &obj) == nil {
					if idRaw, ok := obj["id"].(float64); ok {
						respMu.Lock()
						responsesByID[int(idRaw)] = obj
						respMu.Unlock()
						select {
						case respSignal <- struct{}{}:
						default:
						}
					}
				}
			}
		}()

		sendRPC := func(msg map[string]any) {
			t.Helper()
			b, _ := json.Marshal(msg)
			if _, err := inW.Write(append(b, '\n')); err != nil {
				t.Fatalf("write rpc: %v", err)
			}
		}
		waitRPC := func(wantID int) map[string]any {
			t.Helper()
			deadline := time.After(4 * time.Second)
			for {
				respMu.Lock()
				got, ok := responsesByID[wantID]
				respMu.Unlock()
				if ok {
					return got
				}
				select {
				case <-respSignal:
				case <-deadline:
					t.Fatalf("timed out waiting for JSON-RPC response id=%d (stderr=%s)", wantID, stderr.String())
				}
			}
		}

		sendRPC(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": 1}})
		_ = waitRPC(1)

		sendRPC(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "session/new", "params": map[string]any{"cwd": wsDir}})
		sessResp := waitRPC(2)
		sessID := sessResp["result"].(map[string]any)["sessionId"].(string)

		// Start slow blocking prompt (id=10), wait until streamer begins, then send session/cancel (id=11) mid-run!
		sendRPC(map[string]any{
			"jsonrpc": "2.0",
			"id":      10,
			"method":  "session/prompt",
			"params": map[string]any{
				"sessionId": sessID,
				"prompt":    "SLOW_BLOCKING_RUN",
			},
		})
		select {
		case <-startedSlowCh:
		case <-time.After(3 * time.Second):
			t.Fatalf("slow ACP run never started")
		}

		sendRPC(map[string]any{
			"jsonrpc": "2.0",
			"id":      11,
			"method":  "session/cancel",
			"params": map[string]any{
				"sessionId": sessID,
			},
		})
		cancelResp := waitRPC(11)
		if cancelled, _ := cancelResp["result"].(map[string]any)["cancelled"].(bool); !cancelled {
			t.Fatalf("expected session/cancel cancelled=true while prompt active, got: %+v", cancelResp)
		}

		prompt10Resp := waitRPC(10)
		if sr, _ := prompt10Resp["result"].(map[string]any)["stopReason"].(string); sr != "cancelled" {
			t.Fatalf("expected cancelled stopReason for interrupted prompt, got: %+v", prompt10Resp)
		}

		// Now trigger a GateAsk command over ACP and verify it defers immediately without hanging.
		sendRPC(map[string]any{
			"jsonrpc": "2.0",
			"id":      12,
			"method":  "session/prompt",
			"params": map[string]any{
				"sessionId": sessID,
				"prompt":    "Run git push",
			},
		})
		prompt12Resp := waitRPC(12)
		if sr, _ := prompt12Resp["result"].(map[string]any)["stopReason"].(string); sr != "deferred" {
			t.Fatalf("expected stopReason=deferred for unattended GateAsk in ACP, got: %+v", prompt12Resp)
		}

		_ = inW.Close()
		if code := <-doneCh; code != 0 {
			t.Fatalf("expected gorness acp exit code 0, got %d", code)
		}
	})
}

func TestInteractiveLaunchFromHomeSkipsVaultContainingCwdAndWritesDaemonLog(t *testing.T) {
	mockLLM := startMockAnthropicSSEServer(t)
	defer mockLLM.Close()

	vaultDir, homeDir, _ := prepareTempVaultWithMockProvider(t, mockLLM.URL)

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(homeDir); err != nil {
		t.Fatalf("Chdir(homeDir): %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	// 1. Start daemon in-process on ephemeral port so cmdStudio probes it and verify cwd=$HOME is cleanly skipped when --workspace is omitted!
	dCtx, dCancel := context.WithCancel(context.Background())
	defer dCancel()
	var dOut, dErr bytes.Buffer
	dDone := make(chan int, 1)
	go func() {
		dDone <- runCLI(dCtx, []string{
			"daemon", "serve",
			"--vault", vaultDir,
			"--host", "127.0.0.1",
			"--port", "0",
		}, strings.NewReader(""), &dOut, &dErr, cliOptions{userHomeDir: homeDir})
	}()

	var daemonURL string
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if u, _ := probeRunningDaemon(vaultDir); u != "" {
			daemonURL = u
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if daemonURL == "" {
		t.Fatalf("daemon did not become healthy: %s", dErr.String())
	}

	var stOut, stErr bytes.Buffer
	code := runCLI(dCtx, []string{
		"studio",
		"--vault", vaultDir,
		"--no-open",
	}, strings.NewReader(""), &stOut, &stErr, cliOptions{userHomeDir: homeDir})
	if code != 0 {
		t.Fatalf("expected studio --no-open from $HOME to succeed with code 0, got %d (stderr=%s)", code, stErr.String())
	}

	resp, err := http.Get(daemonURL + "/api/workspaces")
	if err != nil {
		t.Fatalf("GET /api/workspaces: %v", err)
	}
	var wsList []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&wsList)
	resp.Body.Close()
	if len(wsList) != 0 {
		t.Fatalf("expected 0 workspaces registered when launching from $HOME without --workspace, got %+v", wsList)
	}

	// Explicit --workspace pointing to $HOME must be rejected cleanly with non-zero exit code!
	var badOut, badErr bytes.Buffer
	badCode := runCLI(dCtx, []string{
		"studio",
		"--vault", vaultDir,
		"--workspace", homeDir,
		"--no-open",
	}, strings.NewReader(""), &badOut, &badErr, cliOptions{userHomeDir: homeDir})
	if badCode == 0 {
		t.Fatal("expected explicit --workspace $HOME to fail with non-zero exit code")
	}
	if gotLog := daemonBackgroundLogPath(config.VaultPaths{Root: vaultDir}); filepath.Base(gotLog) != "daemon.log" {
		t.Fatalf("expected daemon background stdout log at runtime/logs/daemon.log, got %s", gotLog)
	}

	dCancel()
	<-dDone
}

func TestCLILiveDaemonUDSRoutingACPSetModeCancelAndStandalone(t *testing.T) {
	slowStartedCh := make(chan struct{}, 4)
	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawReq, _ := io.ReadAll(r.Body)
		bodyStr := string(rawReq)

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		writeSSE := func(event string, data any) {
			raw, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(raw))
			if flusher != nil {
				flusher.Flush()
			}
		}

		if strings.Contains(bodyStr, "SLOW_DAEMON_STREAM") {
			writeSSE("message_start", map[string]any{
				"type":    "message_start",
				"message": map[string]any{"usage": map[string]any{"input_tokens": 10}},
			})
			writeSSE("content_block_start", map[string]any{
				"type":          "content_block_start",
				"index":         0,
				"content_block": map[string]any{"type": "text", "text": ""},
			})
			writeSSE("content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]any{"type": "text_delta", "text": "Streaming via live daemon..."},
			})
			select {
			case slowStartedCh <- struct{}{}:
			default:
			}
			<-r.Context().Done()
			return
		}

		if strings.Contains(bodyStr, "TRIGGER_SHELL_TOOL") {
			writeSSE("message_start", map[string]any{
				"type":    "message_start",
				"message": map[string]any{"usage": map[string]any{"input_tokens": 12}},
			})
			writeSSE("content_block_start", map[string]any{
				"type":  "content_block_start",
				"index": 0,
				"content_block": map[string]any{
					"type":  "tool_use",
					"id":    "toolu_live_push_1",
					"name":  "run_command",
					"input": map[string]any{},
				},
			})
			writeSSE("content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]any{
					"type":         "input_json_delta",
					"partial_json": `{"command":"git push origin main"}`,
				},
			})
			writeSSE("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
			writeSSE("message_delta", map[string]any{
				"type":  "message_delta",
				"delta": map[string]any{"stop_reason": "tool_use"},
				"usage": map[string]any{"output_tokens": 10},
			})
			writeSSE("message_stop", map[string]any{"type": "message_stop"})
			return
		}

		writeSSE("message_start", map[string]any{
			"type":    "message_start",
			"message": map[string]any{"usage": map[string]any{"input_tokens": 16}},
		})
		writeSSE("content_block_start", map[string]any{
			"type":          "content_block_start",
			"index":         0,
			"content_block": map[string]any{"type": "text", "text": ""},
		})
		writeSSE("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]any{"type": "text_delta", "text": "Live daemon response ok."},
		})
		writeSSE("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
		writeSSE("message_delta", map[string]any{
			"type":  "message_delta",
			"delta": map[string]any{"stop_reason": "end_turn"},
			"usage": map[string]any{"output_tokens": 6},
		})
		writeSSE("message_stop", map[string]any{"type": "message_stop"})
	}))
	defer mockLLM.Close()

	// Short /tmp directory so runtime/daemon.sock fits well within macOS 104-byte sun_path limit!
	shortBase, err := os.MkdirTemp("/tmp", "gcli-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(shortBase) })

	homeDir := filepath.Join(shortBase, "h")
	vaultDir := filepath.Join(homeDir, ".gorness")
	wsDir := filepath.Join(shortBase, "ws")
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		t.Fatalf("mkdir homeDir: %v", err)
	}
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("mkdir wsDir: %v", err)
	}
	if _, err := config.NewManager(config.Options{VaultRoot: vaultDir, UserHomeDir: homeDir}); err != nil {
		t.Fatalf("bootstrap short vault: %v", err)
	}
	providersTOML := fmt.Sprintf(`[providers.anthropic]
name = "Mock Anthropic"
protocol = "anthropic"
base_url = %q
api_key = "env:GORNESS_TEST_MOCK_KEY"
timeout_sec = 30

[providers.anthropic.models."claude-sonnet-5-5"]
name = "Claude Sonnet 5.5"
context_window = 200000
max_output_tokens = 8192

[providers.anthropic.models."claude-haiku-5-5"]
name = "Claude Haiku 5.5"
context_window = 200000
max_output_tokens = 4096
`, mockLLM.URL)
	if err := os.WriteFile(filepath.Join(vaultDir, "providers.toml"), []byte(providersTOML), 0o600); err != nil {
		t.Fatalf("write providers.toml: %v", err)
	}
	t.Setenv("GORNESS_TEST_MOCK_KEY", "sk-mock-live-daemon-key")

	// 1. Check `gorness --version` references config.Version
	var verOut, verErr bytes.Buffer
	if code := runCLI(context.Background(), []string{"--version"}, strings.NewReader(""), &verOut, &verErr, cliOptions{}); code != 0 {
		t.Fatalf("--version failed: %d", code)
	}
	if !strings.Contains(verOut.String(), "gorness v"+config.Version) {
		t.Fatalf("expected --version to report config.Version %q, got %q", config.Version, verOut.String())
	}

	// 2. Start live daemon serving UDS + loopback TCP
	dCtx, dCancel := context.WithCancel(context.Background())
	defer dCancel()
	var dOut, dErr bytes.Buffer
	dDone := make(chan int, 1)
	go func() {
		dDone <- runCLI(dCtx, []string{
			"daemon", "serve",
			"--vault", vaultDir,
			"--host", "127.0.0.1",
			"--port", "0",
		}, strings.NewReader(""), &dOut, &dErr, cliOptions{userHomeDir: homeDir})
	}()

	var ep *daemonctl.Endpoint
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if probed, ok := daemonctl.Probe(dCtx, vaultDir); ok {
			ep = probed
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if ep == nil {
		t.Fatalf("live daemon failed to become healthy: %s", dErr.String())
	}
	if ep.BaseURL != daemonctl.UDSBaseURL || ep.SocketPath == "" {
		t.Fatalf("expected live daemon Probe to prefer UDS socket, got BaseURL=%q SocketPath=%q", ep.BaseURL, ep.SocketPath)
	}

	// 3. Execute `gorness run` without injected streamer and without --standalone -> routes through live daemon!
	var runOut, runErr bytes.Buffer
	runCode := runCLI(dCtx, []string{
		"run",
		"--vault", vaultDir,
		"--workspace", wsDir,
		"Hello live daemon",
	}, strings.NewReader(""), &runOut, &runErr, cliOptions{userHomeDir: homeDir})
	if runCode != 0 {
		t.Fatalf("gorness run over live daemon failed: code=%d stderr=%s", runCode, runErr.String())
	}
	if !strings.Contains(runOut.String(), "Live daemon response ok.") {
		t.Fatalf("unexpected gorness run output: %q", runOut.String())
	}

	var daemonSessions []*store.SessionMeta
	if _, err := ep.DoJSON(dCtx, http.MethodGet, "/api/sessions", nil, &daemonSessions); err != nil {
		t.Fatalf("GET /api/sessions on live daemon: %v", err)
	}
	if len(daemonSessions) != 1 {
		t.Fatalf("expected 1 session persisted inside live daemon after `gorness run`, got %d", len(daemonSessions))
	}

	// 4. Execute `gorness acp` over live daemon: test session/new, session/set_mode, deferred ask mode, and mid-run session/cancel!
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	var acpErr bytes.Buffer
	acpDone := make(chan int, 1)
	go func() {
		code := runCLI(dCtx, []string{"acp", "--vault", vaultDir}, inR, outW, &acpErr, cliOptions{userHomeDir: homeDir})
		_ = outW.Close()
		acpDone <- code
	}()

	var (
		rpcMu         sync.Mutex
		responsesByID = make(map[int]map[string]any)
		modeUpdates   []map[string]any
		rpcSig        = make(chan struct{}, 64)
	)
	go func() {
		r := bufio.NewReader(outR)
		for {
			line, err := r.ReadBytes('\n')
			if err != nil {
				return
			}
			var obj map[string]any
			if json.Unmarshal(bytes.TrimSpace(line), &obj) != nil {
				continue
			}
			rpcMu.Lock()
			if idVal, ok := obj["id"].(float64); ok {
				responsesByID[int(idVal)] = obj
			} else if obj["method"] == "session/update" {
				if params, ok := obj["params"].(map[string]any); ok {
					if upd, ok := params["update"].(map[string]any); ok && upd["sessionUpdate"] == "current_mode_update" {
						modeUpdates = append(modeUpdates, upd)
					}
				}
			}
			rpcMu.Unlock()
			select {
			case rpcSig <- struct{}{}:
			default:
			}
		}
	}()

	sendRPC := func(msg map[string]any) {
		t.Helper()
		b, _ := json.Marshal(msg)
		if _, err := inW.Write(append(b, '\n')); err != nil {
			t.Fatalf("write rpc: %v", err)
		}
	}
	waitRPC := func(wantID int) map[string]any {
		t.Helper()
		timeout := time.After(5 * time.Second)
		for {
			rpcMu.Lock()
			got, ok := responsesByID[wantID]
			rpcMu.Unlock()
			if ok {
				return got
			}
			select {
			case <-rpcSig:
			case <-timeout:
				t.Fatalf("timeout waiting for ACP id=%d (stderr=%s)", wantID, acpErr.String())
			}
		}
	}

	sendRPC(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": 1}})
	initRes := waitRPC(1)["result"].(map[string]any)
	if srvInfo, _ := initRes["serverInfo"].(map[string]any); srvInfo["version"] != config.Version {
		t.Fatalf("expected ACP initialize version %q, got %+v", config.Version, srvInfo)
	}

	sendRPC(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "session/new", "params": map[string]any{"cwd": wsDir, "agentRole": "coder"}})
	acpSessID := waitRPC(2)["result"].(map[string]any)["sessionId"].(string)

	// Switch ACP session mode to "ask" and agentRole to "verifier" via session/set_mode
	sendRPC(map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "session/set_mode",
		"params": map[string]any{
			"sessionId": acpSessID,
			"modeId":    "ask",
			"agentRole": "verifier",
		},
	})
	setModeResp := waitRPC(3)
	if setModeResp["error"] != nil {
		t.Fatalf("session/set_mode returned error: %+v", setModeResp["error"])
	}
	setModeRes := setModeResp["result"].(map[string]any)
	if setModeRes["currentModeId"] != "ask" || setModeRes["agentRole"] != "verifier" {
		t.Fatalf("unexpected session/set_mode result: %+v", setModeRes)
	}

	rpcMu.Lock()
	gotModeNotifs := len(modeUpdates)
	rpcMu.Unlock()
	if gotModeNotifs == 0 {
		t.Fatal("expected session/update current_mode_update notification from session/set_mode")
	}

	// Verify live daemon persisted agent_role = "verifier"
	var sessDetail struct {
		Meta *store.SessionMeta `json:"meta"`
	}
	if _, err := ep.DoJSON(dCtx, http.MethodGet, "/api/sessions/"+acpSessID, nil, &sessDetail); err != nil {
		t.Fatalf("GET /api/sessions/%s: %v", acpSessID, err)
	}
	if sessDetail.Meta == nil || sessDetail.Meta.AgentRole != "verifier" {
		t.Fatalf("expected live daemon session role 'verifier', got %+v", sessDetail.Meta)
	}

	// Verify invalid modeId returns JSON-RPC -32602 error
	sendRPC(map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "session/set_mode",
		"params": map[string]any{
			"sessionId": acpSessID,
			"modeId":    "bogus_nonexistent_mode",
		},
	})
	badModeResp := waitRPC(4)
	if badModeResp["error"] == nil {
		t.Fatalf("expected error for invalid modeId, got %+v", badModeResp)
	}

	// Switch back to coder role while keeping "ask" permission mode, then verify TRIGGER_SHELL_TOOL defers on live daemon!
	sendRPC(map[string]any{
		"jsonrpc": "2.0",
		"id":      5,
		"method":  "session/set_mode",
		"params": map[string]any{
			"sessionId": acpSessID,
			"modeId":    "role:coder",
		},
	})
	_ = waitRPC(5)

	sendRPC(map[string]any{
		"jsonrpc": "2.0",
		"id":      6,
		"method":  "session/prompt",
		"params": map[string]any{
			"sessionId": acpSessID,
			"prompt":    "TRIGGER_SHELL_TOOL",
		},
	})
	deferResp := waitRPC(6)
	if sr, _ := deferResp["result"].(map[string]any)["stopReason"].(string); sr != "deferred" {
		t.Fatalf("expected stopReason=deferred after session/set_mode(ask) on live daemon, got %+v", deferResp)
	}

	// Mid-run cancellation over live daemon HTTP/SSE (`POST /api/runs/{id}/stop`)
	sendRPC(map[string]any{
		"jsonrpc": "2.0",
		"id":      7,
		"method":  "session/prompt",
		"params": map[string]any{
			"sessionId": acpSessID,
			"prompt":    "SLOW_DAEMON_STREAM",
		},
	})
	select {
	case <-slowStartedCh:
	case <-time.After(4 * time.Second):
		t.Fatal("slow daemon run never started")
	}

	sendRPC(map[string]any{
		"jsonrpc": "2.0",
		"id":      8,
		"method":  "session/cancel",
		"params":  map[string]any{"sessionId": acpSessID},
	})
	cancelRes := waitRPC(8)["result"].(map[string]any)
	if cancelled, _ := cancelRes["cancelled"].(bool); !cancelled {
		t.Fatalf("expected live daemon session/cancel cancelled=true, got %+v", cancelRes)
	}
	slowPromptRes := waitRPC(7)["result"].(map[string]any)
	if slowPromptRes["stopReason"] != "cancelled" {
		t.Fatalf("expected stopReason=cancelled for stopped daemon run, got %+v", slowPromptRes)
	}

	_ = inW.Close()
	if code := <-acpDone; code != 0 {
		t.Fatalf("expected acp exit code 0, got %d (stderr=%s)", code, acpErr.String())
	}

	dCancel()
	if code := <-dDone; code != 0 {
		t.Fatalf("expected daemon serve exit 0, got %d (stderr=%s)", code, dErr.String())
	}

	// 5. Verify `--standalone` runs directly in-process with no daemon running
	standaloneVault, standaloneHome, standaloneWS := prepareTempVaultWithMockProvider(t, mockLLM.URL)
	var saOut, saErr bytes.Buffer
	saCode := runCLI(context.Background(), []string{
		"run",
		"--standalone",
		"--vault", standaloneVault,
		"--workspace", standaloneWS,
		"Standalone execution check",
	}, strings.NewReader(""), &saOut, &saErr, cliOptions{userHomeDir: standaloneHome})
	if saCode != 0 || !strings.Contains(saOut.String(), "Live daemon response ok.") {
		t.Fatalf("standalone gorness run failed: code=%d out=%q err=%q", saCode, saOut.String(), saErr.String())
	}

	// 6. Enforce strict Zero-DB invariant (zero .json files inside vaultDir)
	for _, vDir := range []string{vaultDir, standaloneVault} {
		if err := filepath.WalkDir(vDir, func(path string, d os.DirEntry, wErr error) error {
			if wErr != nil {
				return wErr
			}
			if !d.IsDir() && strings.EqualFold(filepath.Ext(d.Name()), ".json") {
				t.Fatalf("Zero-DB violation: forbidden .json file in vault: %s", path)
			}
			return nil
		}); err != nil {
			t.Fatalf("walk %s: %v", vDir, err)
		}
	}
}
