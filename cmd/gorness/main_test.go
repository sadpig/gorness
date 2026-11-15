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
	"testing"
	"time"

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/core"
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
