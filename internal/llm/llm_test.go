package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/store"
)

func collectStreamEvents(t *testing.T, ch <-chan StreamEvent) []StreamEvent {
	t.Helper()
	var out []StreamEvent
	for ev := range ch {
		out = append(out, ev)
	}
	return out
}

func TestAnthropicSSEAdaptiveThinkingAndPreservedBlocks(t *testing.T) {
	t.Parallel()

	largePayloadValue := strings.Repeat("A", 150*1024) // 150KB > bufio.Scanner 64KB limit
	var turnCount atomic.Int32
	var turn1WireBody []byte
	var turn2WireBody []byte
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusNotFound)
			return
		}
		if got := r.Header.Get("x-api-key"); got != "sk-ant-test-key-1234567890" {
			http.Error(w, "bad api key header", http.StatusUnauthorized)
			return
		}
		if got := r.Header.Get("anthropic-version"); got == "" {
			http.Error(w, "missing anthropic-version", http.StatusBadRequest)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		turn := turnCount.Add(1)
		mu.Lock()
		if turn == 1 {
			turn1WireBody = append([]byte(nil), body...)
		} else {
			turn2WireBody = append([]byte(nil), body...)
		}
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		if turn == 1 {
			chunk1 := fmt.Sprintf(`{"path":"pkg/big.txt","content":%q`, largePayloadValue[:80*1024])
			chunk2 := fmt.Sprintf(`%q}`, largePayloadValue[80*1024:])
			// Note: chunk1 has opening quote, chunk2 shouldn't add extra opening quote:
			chunk1 = `{"path":"pkg/big.txt","content":"` + largePayloadValue[:80*1024]
			chunk2 = largePayloadValue[80*1024:] + `"}`

			c1JSON, _ := json.Marshal(map[string]any{
				"type":  "content_block_delta",
				"index": 2,
				"delta": map[string]any{
					"type":         "input_json_delta",
					"partial_json": chunk1,
				},
			})
			c2JSON, _ := json.Marshal(map[string]any{
				"type":  "content_block_delta",
				"index": 2,
				"delta": map[string]any{
					"type":         "input_json_delta",
					"partial_json": chunk2,
				},
			})

			lines := []string{
				`event: message_start`,
				`data: {"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","usage":{"input_tokens":512,"output_tokens":1,"cache_read_input_tokens":400}}}`,
				``,
				`: keepalive comment`,
				``,
				`event: content_block_start`,
				`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
				``,
				`event: content_block_delta`,
				`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Step 1: inspect repository layout. "}}`,
				``,
				`event: content_block_delta`,
				`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Step 2: write large fixture."}}`,
				``,
				`event: content_block_delta`,
				`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig_v1_exact_opaque_8899=="}}`,
				``,
				`event: content_block_stop`,
				`data: {"type":"content_block_stop","index":0}`,
				``,
				`event: content_block_start`,
				`data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
				``,
				`event: content_block_delta`,
				`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Writing fixture file now."}}`,
				``,
				`event: content_block_stop`,
				`data: {"type":"content_block_stop","index":1}`,
				``,
				`event: content_block_start`,
				`data: {"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_01ABC","name":"write_file","input":{}}}`,
				``,
				`event: content_block_delta`,
				`data: ` + string(c1JSON),
				``,
				`event: content_block_delta`,
				`data: ` + string(c2JSON),
				``,
				`event: content_block_stop`,
				`data: {"type":"content_block_stop","index":2}`,
				``,
				`event: message_delta`,
				`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":95}}`,
				``,
				`event: message_stop`,
				`data: {"type":"message_stop"}`,
				``,
			}
			for _, ln := range lines {
				fmt.Fprintln(w, ln)
			}
			flusher.Flush()
			return
		}

		// Turn 2: simulate compaction + refusal stop reason
		lines := []string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_02","type":"message","role":"assistant","usage":{"input_tokens":640,"output_tokens":1,"cache_read_input_tokens":512}}}`,
			``,
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"compaction","content":"anchored-summary-v1"}}`,
			``,
			`event: content_block_stop`,
			`data: {"type":"content_block_stop","index":0}`,
			``,
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"I cannot fulfill that destructive request."}}`,
			``,
			`event: content_block_stop`,
			`data: {"type":"content_block_stop","index":1}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"refusal"},"usage":{"output_tokens":18}}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}
		for _, ln := range lines {
			fmt.Fprintln(w, ln)
		}
		flusher.Flush()
	}))
	defer srv.Close()

	provider := config.ProviderConfig{
		ID:       "anthropic",
		Name:     "Anthropic",
		Protocol: "anthropic",
		BaseURL:  srv.URL,
		Models: map[string]config.ModelSpec{
			"claude-opus-5-5": {
				ID:               "claude-opus-5-5",
				MaxOutputTokens:  32000,
				SupportsThinking: true,
				SupportsCaching:  true,
			},
		},
	}

	unsortedTools := []ToolSchema{
		{
			Name:        "write_file",
			Description: "Write file contents",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}}}`),
		},
		{
			Name:        "read_file",
			Description: "Read file contents",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		},
	}

	initialHistory := []store.SessionEntry{
		{
			Seq:  1,
			ID:   "ent_1",
			Role: "user",
			Blocks: []store.ContentBlock{
				{Type: "text", Text: "Create the test fixture."},
			},
		},
	}

	sortedTools, frozenSystem, augHistory := AssemblePrompt(
		[]string{"You are Lead Architect.", "Follow Zero-DB invariants."},
		unsortedTools,
		initialHistory,
		[]string{"git branch: feat/gorness-v1"},
	)

	streamer := NewNativeStreamer(srv.Client())
	ch1, err := streamer.Stream(context.Background(), Request{
		Provider:     provider,
		APIKey:       "sk-ant-test-key-1234567890",
		Model:        "claude-opus-5-5",
		Effort:       "high",
		StaticSystem: frozenSystem,
		History:      augHistory,
		Tools:        sortedTools,
	})
	if err != nil {
		t.Fatalf("Turn 1 Stream error: %v", err)
	}
	events1 := collectStreamEvents(t, ch1)

	// Verify Turn 1 wire request
	mu.Lock()
	raw1 := append([]byte(nil), turn1WireBody...)
	mu.Unlock()

	if bytes.Contains(raw1, []byte("budget_tokens")) {
		t.Fatalf("wire request must NEVER contain budget_tokens, got: %s", string(raw1))
	}

	var wire1 struct {
		Model        string            `json:"model"`
		Stream       bool              `json:"stream"`
		MaxTokens    int               `json:"max_tokens"`
		Thinking     map[string]any    `json:"thinking"`
		OutputConfig map[string]string `json:"output_config"`
		System       []struct {
			Type         string            `json:"type"`
			Text         string            `json:"text"`
			CacheControl map[string]string `json:"cache_control"`
		} `json:"system"`
		Tools []struct {
			Name         string            `json:"name"`
			CacheControl map[string]string `json:"cache_control"`
		} `json:"tools"`
		Messages []struct {
			Role    string            `json:"role"`
			Content []json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw1, &wire1); err != nil {
		t.Fatalf("unmarshal Turn 1 wire body: %v", err)
	}
	if wire1.Thinking["type"] != "adaptive" {
		t.Fatalf("expected thinking.type=adaptive, got %#v", wire1.Thinking)
	}
	if wire1.OutputConfig["effort"] != "high" {
		t.Fatalf("expected output_config.effort=high, got %#v", wire1.OutputConfig)
	}
	if len(wire1.System) != 1 || wire1.System[0].CacheControl["type"] != "ephemeral" {
		t.Fatalf("expected static system with ephemeral cache_control, got %#v", wire1.System)
	}
	if strings.Contains(wire1.System[0].Text, "feat/gorness-v1") {
		t.Fatalf("dynamic reminder leaked into static system prompt: %q", wire1.System[0].Text)
	}
	if len(wire1.Tools) != 2 || wire1.Tools[0].Name != "read_file" || wire1.Tools[1].Name != "write_file" {
		t.Fatalf("expected sorted tools [read_file, write_file], got %#v", wire1.Tools)
	}
	if len(wire1.Tools[0].CacheControl) != 0 {
		t.Fatalf("non-final tool must not carry cache_control: %#v", wire1.Tools[0])
	}
	if wire1.Tools[1].CacheControl["type"] != "ephemeral" {
		t.Fatalf("last sorted tool must carry ephemeral cache_control, got %#v", wire1.Tools[1])
	}

	// Verify Turn 1 parsed events
	var gotThinking, gotSig, gotText strings.Builder
	var gotTool *StreamEvent
	var gotDone *StreamEvent
	for i := range events1 {
		ev := events1[i]
		switch ev.Type {
		case StreamThinkingDelta:
			gotThinking.WriteString(ev.Thinking)
			gotSig.WriteString(ev.Signature)
		case StreamTextDelta:
			gotText.WriteString(ev.Text)
		case StreamToolCall:
			cp := ev
			gotTool = &cp
		case StreamDone:
			cp := ev
			gotDone = &cp
		case StreamError:
			t.Fatalf("unexpected Turn 1 StreamError: %v", ev.Err)
		}
	}

	wantThinking := "Step 1: inspect repository layout. Step 2: write large fixture."
	if gotThinking.String() != wantThinking {
		t.Fatalf("thinking mismatch: got %q, want %q", gotThinking.String(), wantThinking)
	}
	if gotSig.String() != "sig_v1_exact_opaque_8899==" {
		t.Fatalf("signature mismatch: got %q", gotSig.String())
	}
	if gotText.String() != "Writing fixture file now." {
		t.Fatalf("text mismatch: got %q", gotText.String())
	}
	if gotTool == nil || gotTool.ToolCallID != "toolu_01ABC" || gotTool.ToolName != "write_file" {
		t.Fatalf("unexpected tool call event: %#v", gotTool)
	}
	var parsedToolArgs struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(gotTool.ToolInput, &parsedToolArgs); err != nil {
		t.Fatalf("unmarshal assembled 150KB tool_input: %v", err)
	}
	if parsedToolArgs.Path != "pkg/big.txt" || parsedToolArgs.Content != largePayloadValue {
		t.Fatalf("large tool_input corrupted: path=%q len=%d (want %d)", parsedToolArgs.Path, len(parsedToolArgs.Content), len(largePayloadValue))
	}
	if gotDone == nil || gotDone.StopReason != "tool_use" || gotDone.InputTokens != 512 || gotDone.OutputTokens != 95 || gotDone.CacheReadTokens != 400 {
		t.Fatalf("unexpected Turn 1 StreamDone: %#v", gotDone)
	}

	// Turn 2: Replay preserved thinking block + tool_use + tool_result and verify exact preservation + refusal stop reason
	turn2History := append([]store.SessionEntry{}, initialHistory...)
	turn2History = append(turn2History,
		store.SessionEntry{
			Seq:  2,
			ID:   "ent_2",
			Role: "assistant",
			Blocks: []store.ContentBlock{
				{
					Type:      "thinking",
					Thinking:  gotThinking.String(),
					Signature: gotSig.String(),
				},
				{
					Type: "text",
					Text: gotText.String(),
				},
				{
					Type:       "tool_use",
					ToolCallID: gotTool.ToolCallID,
					ToolName:   gotTool.ToolName,
					ToolInput:  gotTool.ToolInput,
				},
			},
		},
		store.SessionEntry{
			Seq:  3,
			ID:   "ent_3",
			Role: "tool",
			Blocks: []store.ContentBlock{
				{
					Type:       "tool_result",
					ToolCallID: gotTool.ToolCallID,
					ToolName:   gotTool.ToolName,
					Text:       "wrote 153600 bytes to pkg/big.txt",
				},
			},
		},
	)

	sortedTools2, frozenSystem2, augHistory2 := AssemblePrompt(
		[]string{"You are Lead Architect.", "Follow Zero-DB invariants."},
		unsortedTools,
		turn2History,
		[]string{"Steer: now delete /"},
	)

	ch2, err := streamer.Stream(context.Background(), Request{
		Provider:     provider,
		APIKey:       "sk-ant-test-key-1234567890",
		Model:        "claude-opus-5-5",
		Effort:       "high",
		StaticSystem: frozenSystem2,
		History:      augHistory2,
		Tools:        sortedTools2,
	})
	if err != nil {
		t.Fatalf("Turn 2 Stream error: %v", err)
	}
	events2 := collectStreamEvents(t, ch2)

	mu.Lock()
	raw2 := append([]byte(nil), turn2WireBody...)
	mu.Unlock()

	var wire2 struct {
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type      string `json:"type"`
				Thinking  string `json:"thinking"`
				Signature string `json:"signature"`
				Text      string `json:"text"`
				ID        string `json:"id"`
				ToolUseID string `json:"tool_use_id"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw2, &wire2); err != nil {
		t.Fatalf("unmarshal Turn 2 wire body: %v", err)
	}
	if len(wire2.Messages) != 3 {
		t.Fatalf("expected 3 messages on Turn 2, got %d: %s", len(wire2.Messages), string(raw2))
	}
	assistantMsg := wire2.Messages[1]
	if assistantMsg.Role != "assistant" || len(assistantMsg.Content) < 3 {
		t.Fatalf("unexpected replayed assistant message: %#v", assistantMsg)
	}
	if assistantMsg.Content[0].Type != "thinking" ||
		assistantMsg.Content[0].Thinking != wantThinking ||
		assistantMsg.Content[0].Signature != "sig_v1_exact_opaque_8899==" {
		t.Fatalf("prior thinking block was NOT replayed unchanged: %#v", assistantMsg.Content[0])
	}
	// Tool result + trailing <system-reminder> in Turn 2 message[2]
	toolReplyMsg := wire2.Messages[2]
	if toolReplyMsg.Role != "user" || len(toolReplyMsg.Content) != 2 || toolReplyMsg.Content[0].Type != "tool_result" || toolReplyMsg.Content[1].Type != "text" {
		t.Fatalf("expected user message with tool_result followed by <system-reminder> text, got %#v", toolReplyMsg)
	}

	var gotCompaction json.RawMessage
	var gotDone2 *StreamEvent
	for i := range events2 {
		ev := events2[i]
		switch ev.Type {
		case StreamCompaction:
			gotCompaction = ev.Compaction
		case StreamDone:
			cp := ev
			gotDone2 = &cp
		case StreamError:
			t.Fatalf("unexpected Turn 2 StreamError: %v", ev.Err)
		}
	}
	if len(gotCompaction) == 0 {
		t.Fatalf("expected StreamCompaction event on Turn 2")
	}
	if gotDone2 == nil || gotDone2.StopReason != "refusal" {
		t.Fatalf("expected Turn 2 StopReason=refusal, got %#v", gotDone2)
	}
}

func TestOpenAIResponsesCompatAndGeminiAdapters(t *testing.T) {
	t.Parallel()

	tools := []ToolSchema{
		{
			Name:        "search_code",
			Description: "Search repository symbols",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
		},
		{
			Name:        "read_file",
			Description: "Read file",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		},
	}

	t.Run("openai-responses", func(t *testing.T) {
		t.Parallel()
		var gotAuth string
		var wireBody []byte

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/responses" {
				http.Error(w, "bad path: "+r.URL.Path, http.StatusNotFound)
				return
			}
			gotAuth = r.Header.Get("Authorization")
			wireBody, _ = io.ReadAll(r.Body)

			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			lines := []string{
				`event: response.reasoning_summary_text.delta`,
				`data: {"type":"response.reasoning_summary_text.delta","delta":"Checking files..."}`,
				``,
				`event: response.output_text.delta`,
				`data: {"type":"response.output_text.delta","delta":"Let me read go.mod."}`,
				``,
				`event: response.output_item.added`,
				`data: {"type":"response.output_item.added","output_index":1,"item":{"id":"fc_1","type":"function_call","call_id":"call_resp_01","name":"read_file","arguments":""}}`,
				``,
				`event: response.function_call_arguments.delta`,
				`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":1,"delta":"{\"path\":"}`,
				``,
				`event: response.function_call_arguments.delta`,
				`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":1,"delta":"\"go.mod\"}"}`,
				``,
				`event: response.function_call_arguments.done`,
				`data: {"type":"response.function_call_arguments.done","item_id":"fc_1","output_index":1,"arguments":"{\"path\":\"go.mod\"}"}`,
				``,
				`event: response.completed`,
				`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":300,"output_tokens":45,"input_tokens_details":{"cached_tokens":128}}}}`,
				``,
			}
			for _, ln := range lines {
				fmt.Fprintln(w, ln)
			}
			flusher.Flush()
		}))
		defer srv.Close()

		streamer := NewNativeStreamer(srv.Client())
		ch, err := streamer.Stream(context.Background(), Request{
			Provider: config.ProviderConfig{
				ID:       "openai",
				Protocol: "openai-responses",
				BaseURL:  srv.URL + "/v1",
				Models: map[string]config.ModelSpec{
					"gpt-5.5": {ID: "gpt-5.5", MaxOutputTokens: 16000, SupportsThinking: true},
				},
			},
			APIKey:           "sk-openai-responses-key-123456",
			Model:            "gpt-5.5",
			Effort:           "high",
			StaticSystem:     "You are Verifier.",
			DynamicReminders: []string{"check race conditions"},
			History: []store.SessionEntry{
				{Seq: 1, ID: "e1", Role: "user", Blocks: []store.ContentBlock{{Type: "text", Text: "Verify module"}}},
			},
			Tools: tools,
		})
		if err != nil {
			t.Fatalf("openai-responses Stream error: %v", err)
		}
		evs := collectStreamEvents(t, ch)

		if gotAuth != "Bearer sk-openai-responses-key-123456" {
			t.Fatalf("unexpected Authorization header: %q", gotAuth)
		}
		if !bytes.Contains(wireBody, []byte(`"instructions":"You are Verifier."`)) {
			t.Fatalf("missing instructions in openai-responses payload: %s", string(wireBody))
		}

		var think, text string
		var toolCalls []StreamEvent
		var done *StreamEvent
		for i := range evs {
			ev := evs[i]
			switch ev.Type {
			case StreamThinkingDelta:
				think += ev.Thinking
			case StreamTextDelta:
				text += ev.Text
			case StreamToolCall:
				toolCalls = append(toolCalls, ev)
			case StreamDone:
				cp := ev
				done = &cp
			case StreamError:
				t.Fatalf("unexpected StreamError: %v", ev.Err)
			}
		}
		if think != "Checking files..." || text != "Let me read go.mod." {
			t.Fatalf("unexpected think=%q text=%q", think, text)
		}
		if len(toolCalls) != 1 || toolCalls[0].ToolCallID != "call_resp_01" || toolCalls[0].ToolName != "read_file" || string(toolCalls[0].ToolInput) != `{"path":"go.mod"}` {
			t.Fatalf("unexpected toolCalls: %#v", toolCalls)
		}
		if done == nil || done.InputTokens != 300 || done.OutputTokens != 45 || done.CacheReadTokens != 128 {
			t.Fatalf("unexpected StreamDone: %#v", done)
		}
	})

	t.Run("openai-compat-apple-genai-and-ollama", func(t *testing.T) {
		t.Parallel()
		var gotCustomHeader, gotAuthHeader string
		var reqNum atomic.Int32

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/chat/completions" {
				http.Error(w, "bad path: "+r.URL.Path, http.StatusNotFound)
				return
			}
			gotCustomHeader = r.Header.Get("X-Apple-GenAI-App")
			gotAuthHeader = r.Header.Get("Authorization")
			_, _ = io.ReadAll(r.Body)

			call := reqNum.Add(1)
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)

			if call == 1 {
				// Apple GenAI / enterprise gateway shape: array content parts +reasoning_content + indexed chunked tool calls + separate usage chunk
				lines := []string{
					`data: {"id":"chat_1","choices":[{"index":0,"delta":{"reasoning_content":"Analyzing symbols..."}}]}`,
					``,
					`data: {"id":"chat_1","choices":[{"index":0,"delta":{"content":[{"type":"text","text":"Searching "},{"type":"text","text":"codebase."}]}}]}`,
					``,
					`data: {"id":"chat_1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_compat_1","type":"function","function":{"name":"search_code","arguments":"{\"query\":"}}]}}]}`,
					``,
					`data: {"id":"chat_1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Router\"}"}}]}}]}`,
					``,
					`data: {"id":"chat_1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
					``,
					`data: {"id":"chat_1","choices":[],"usage":{"prompt_tokens":210,"completion_tokens":33,"prompt_tokens_details":{"cached_tokens":90}}}`,
					``,
					`data: [DONE]`,
					``,
				}
				for _, ln := range lines {
					fmt.Fprintln(w, ln)
				}
				flusher.Flush()
				return
			}

			// Call 2: Ollama / local MLX shape (tool_calls without index, direct JSON object arguments, finish_reason without [DONE])
			lines := []string{
				`data: {"id":"mlx_1","choices":[{"index":0,"delta":{"thinking":"Local fast scan."}}]}`,
				``,
				`data: {"id":"mlx_1","choices":[{"index":0,"delta":{"content":"Found file.","tool_calls":[{"id":"call_ollama_1","type":"function","function":{"name":"read_file","arguments":{"path":"README.md"}}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":80,"completion_tokens":15}}`,
				``,
			}
			for _, ln := range lines {
				fmt.Fprintln(w, ln)
			}
			flusher.Flush()
		}))
		defer srv.Close()

		streamer := NewNativeStreamer(srv.Client())
		prov := config.ProviderConfig{
			ID:       "apple_genai",
			Protocol: "openai-compat",
			BaseURL:  srv.URL + "/v1",
			Headers: map[string]string{
				"X-Apple-GenAI-App": "gorness-workbench",
			},
		}

		// Request 1: Apple GenAI shape
		ch1, err := streamer.Stream(context.Background(), Request{
			Provider:     prov,
			APIKey:       "apple-sso-token-xyz987654",
			Model:        "qwen3-coder",
			StaticSystem: "Fast explorer.",
			History: []store.SessionEntry{
				{Seq: 1, ID: "e1", Role: "user", Blocks: []store.ContentBlock{{Type: "text", Text: "Find Router"}}},
			},
			Tools: tools,
		})
		if err != nil {
			t.Fatalf("openai-compat call 1 error: %v", err)
		}
		evs1 := collectStreamEvents(t, ch1)
		if gotCustomHeader != "gorness-workbench" {
			t.Fatalf("custom header not forwarded: got %q", gotCustomHeader)
		}
		if gotAuthHeader != "Bearer apple-sso-token-xyz987654" {
			t.Fatalf("auth header mismatch: got %q", gotAuthHeader)
		}

		var think1, text1 string
		var tc1 []StreamEvent
		var done1 *StreamEvent
		for i := range evs1 {
			ev := evs1[i]
			switch ev.Type {
			case StreamThinkingDelta:
				think1 += ev.Thinking
			case StreamTextDelta:
				text1 += ev.Text
			case StreamToolCall:
				tc1 = append(tc1, ev)
			case StreamDone:
				cp := ev
				done1 = &cp
			case StreamError:
				t.Fatalf("unexpected call 1 StreamError: %v", ev.Err)
			}
		}
		if think1 != "Analyzing symbols..." || text1 != "Searching codebase." {
			t.Fatalf("call 1 mismatch: think=%q text=%q", think1, text1)
		}
		if len(tc1) != 1 || tc1[0].ToolCallID != "call_compat_1" || tc1[0].ToolName != "search_code" || string(tc1[0].ToolInput) != `{"query":"Router"}` {
			t.Fatalf("call 1 toolCalls mismatch: %#v", tc1)
		}
		if done1 == nil || done1.StopReason != "tool_calls" || done1.InputTokens != 210 || done1.OutputTokens != 33 || done1.CacheReadTokens != 90 {
			t.Fatalf("call 1 StreamDone mismatch: %#v", done1)
		}

		// Request 2: Ollama / MLX shape (keyless local server)
		ch2, err := streamer.Stream(context.Background(), Request{
			Provider: config.ProviderConfig{
				ID:       "local_mlx",
				Protocol: "openai-compat",
				BaseURL:  srv.URL,
			},
			Model: "qwen3-coder",
			History: []store.SessionEntry{
				{Seq: 1, ID: "e1", Role: "user", Blocks: []store.ContentBlock{{Type: "text", Text: "Read README"}}},
			},
			Tools: tools,
		})
		if err != nil {
			t.Fatalf("openai-compat call 2 error: %v", err)
		}
		evs2 := collectStreamEvents(t, ch2)
		var tc2 []StreamEvent
		var done2 *StreamEvent
		for i := range evs2 {
			ev := evs2[i]
			switch ev.Type {
			case StreamToolCall:
				tc2 = append(tc2, ev)
			case StreamDone:
				cp := ev
				done2 = &cp
			case StreamError:
				t.Fatalf("unexpected call 2 StreamError: %v", ev.Err)
			}
		}
		if len(tc2) != 1 || tc2[0].ToolName != "read_file" || string(tc2[0].ToolInput) != `{"path":"README.md"}` {
			t.Fatalf("call 2 Ollama tool call mismatch: %#v", tc2)
		}
		if done2 == nil || done2.InputTokens != 80 || done2.OutputTokens != 15 {
			t.Fatalf("call 2 StreamDone mismatch: %#v", done2)
		}
	})

	t.Run("gemini", func(t *testing.T) {
		t.Parallel()
		var gotGoogKey, gotRawQuery, gotPath string
		var wireBody []byte

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotRawQuery = r.URL.RawQuery
			gotGoogKey = r.Header.Get("x-goog-api-key")
			wireBody, _ = io.ReadAll(r.Body)

			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			lines := []string{
				`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"Evaluating invariants...","thought":true,"thoughtSignature":"gem_sig_99"}]}}]}`,
				``,
				`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"Inspecting config.go."},{"functionCall":{"name":"read_file","args":{"path":"internal/config/types.go"}}}]}}]}`,
				``,
				`data: {"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":420,"candidatesTokenCount":28,"thoughtsTokenCount":12,"cachedContentTokenCount":200}}`,
				``,
			}
			for _, ln := range lines {
				fmt.Fprintln(w, ln)
			}
			flusher.Flush()
		}))
		defer srv.Close()

		streamer := NewNativeStreamer(srv.Client())
		ch, err := streamer.Stream(context.Background(), Request{
			Provider: config.ProviderConfig{
				ID:       "gemini",
				Protocol: "gemini",
				BaseURL:  srv.URL + "/v1beta",
				Models: map[string]config.ModelSpec{
					"gemini-2.5-pro": {ID: "gemini-2.5-pro", MaxOutputTokens: 16384, SupportsThinking: true},
				},
			},
			APIKey:       "AIzaSyTestGeminiSecretKey0123456789",
			Model:        "gemini-2.5-pro",
			Effort:       "high",
			StaticSystem: "Blind Gemini reviewer.",
			History: []store.SessionEntry{
				{Seq: 1, ID: "e1", Role: "user", Blocks: []store.ContentBlock{{Type: "text", Text: "Check types"}}},
				{Seq: 2, ID: "e2", Role: "assistant", Blocks: []store.ContentBlock{{Type: "tool_use", ToolCallID: "prev_1", ToolName: "search_code", ToolInput: json.RawMessage(`{"query":"Snapshot"}`)}}},
				{Seq: 3, ID: "e3", Role: "tool", Blocks: []store.ContentBlock{{Type: "tool_result", ToolCallID: "prev_1", Text: "internal/config/types.go:154"}}},
			},
			Tools: tools,
		})
		if err != nil {
			t.Fatalf("gemini Stream error: %v", err)
		}
		evs := collectStreamEvents(t, ch)

		if gotPath != "/v1beta/models/gemini-2.5-pro:streamGenerateContent" {
			t.Fatalf("unexpected gemini path: %q", gotPath)
		}
		if strings.Contains(gotRawQuery, "AIza") {
			t.Fatalf("API key must NOT appear in URL query string: %q", gotRawQuery)
		}
		if gotGoogKey != "AIzaSyTestGeminiSecretKey0123456789" {
			t.Fatalf("missing x-goog-api-key header: %q", gotGoogKey)
		}
		if !bytes.Contains(wireBody, []byte(`"functionResponse":{"id":"prev_1","name":"search_code"`)) &&
			!bytes.Contains(wireBody, []byte(`"name":"search_code"`)) {
			t.Fatalf("expected replayed functionResponse to recover tool name search_code: %s", string(wireBody))
		}

		var think, sig, text string
		var tcs []StreamEvent
		var done *StreamEvent
		for i := range evs {
			ev := evs[i]
			switch ev.Type {
			case StreamThinkingDelta:
				think += ev.Thinking
				sig += ev.Signature
			case StreamTextDelta:
				text += ev.Text
			case StreamToolCall:
				tcs = append(tcs, ev)
			case StreamDone:
				cp := ev
				done = &cp
			case StreamError:
				t.Fatalf("unexpected gemini StreamError: %v", ev.Err)
			}
		}
		if think != "Evaluating invariants..." || sig != "gem_sig_99" || text != "Inspecting config.go." {
			t.Fatalf("gemini stream mismatch: think=%q sig=%q text=%q", think, sig, text)
		}
		if len(tcs) != 1 || tcs[0].ToolCallID == "" || tcs[0].ToolName != "read_file" || string(tcs[0].ToolInput) != `{"path":"internal/config/types.go"}` {
			t.Fatalf("gemini tool call mismatch: %#v", tcs)
		}
		if done == nil || done.StopReason != "STOP" || done.InputTokens != 420 || done.OutputTokens != 40 || done.CacheReadTokens != 200 {
			t.Fatalf("gemini StreamDone mismatch: %#v", done)
		}
	})
}

func TestRoleRouterPriorityAndMidStreamRetryFailover(t *testing.T) {
	t.Parallel()

	vaultRoot := filepath.Join(t.TempDir(), ".gorness")
	var primaryHits atomic.Int32
	var fallbackHits atomic.Int32

	// Primary Anthropic server: streams partial text then disconnects mid-stream (without message_stop)
	primarySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryHits.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}}\n\n")
		fmt.Fprint(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		fmt.Fprint(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"orphaned partial chunk...\"}}\n\n")
		flusher.Flush()
		// Return early WITHOUT message_stop -> mid-stream disconnect!
	}))
	defer primarySrv.Close()

	// Fallback OpenAI Responses server: completes cleanly
	fallbackSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackHits.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Clean failover answer from GPT-5.5.\"}\n\n")
		fmt.Fprint(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"r_ok\",\"status\":\"completed\",\"usage\":{\"input_tokens\":42,\"output_tokens\":9}}}\n\n")
		flusher.Flush()
	}))
	defer fallbackSrv.Close()

	secretEnv := map[string]string{
		"ANTHROPIC_API_KEY": "sk-ant-secret-primary-99887766",
		"OPENAI_API_KEY":    "sk-openai-secret-fallback-55443322",
		"MLX_API_KEY":       "mlx-local-token-11223344",
	}

	mgr, err := config.NewManager(config.Options{
		VaultRoot: vaultRoot,
		EnvLookup: func(key string) (string, bool) {
			v, ok := secretEnv[key]
			return v, ok
		},
		CmdRunner: func(ctx context.Context, shellCmd string) (string, error) {
			if shellCmd == "fetch-sso-token" {
				return "sso-cmd-secret-token-abcdef123456\n", nil
			}
			return "", errors.New("unexpected command")
		},
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Update explorer.md to bind directly to local_mlx / qwen3-coder and lead.md fallbacks to openai/gpt-5.5
	explorerMD := `+++
id = "explorer"
provider = "local_mlx"
model = "qwen3-coder"
effort = "low"
+++
Fast local MLX explorer.
`
	if err := os.WriteFile(filepath.Join(mgr.Paths().AgentsDir(), "explorer.md"), []byte(explorerMD), 0o600); err != nil {
		t.Fatalf("write explorer.md: %v", err)
	}
	leadMD := `+++
id = "lead"
provider = "anthropic"
model = "claude-opus-5-5"
effort = "xhigh"
fallbacks = ["openai/gpt-5.5"]
+++
Lead architect.
`
	if err := os.WriteFile(filepath.Join(mgr.Paths().AgentsDir(), "lead.md"), []byte(leadMD), 0o600); err != nil {
		t.Fatalf("write lead.md: %v", err)
	}

	providersTOML := fmt.Sprintf(`
[providers.anthropic]
name = "Anthropic"
protocol = "anthropic"
base_url = %q
api_key = "env:ANTHROPIC_API_KEY"
timeout_sec = 20

[providers.anthropic.models."claude-opus-5-5"]
max_output_tokens = 32000
supports_thinking = true
supports_caching = true

[providers.anthropic.models."claude-sonnet-5-5"]
max_output_tokens = 16000
supports_thinking = true
supports_caching = true

[providers.openai]
name = "OpenAI"
protocol = "openai-responses"
base_url = %q
api_key = "cmd:fetch-sso-token"
timeout_sec = 20

[providers.openai.models."gpt-5.5"]
max_output_tokens = 32000
supports_thinking = true

[providers.local_mlx]
name = "Local MLX"
protocol = "openai-compat"
base_url = %q
api_key = "env:MLX_API_KEY"
timeout_sec = 10

[providers.local_mlx.models."qwen3-coder"]
max_output_tokens = 8192
`, primarySrv.URL, fallbackSrv.URL+"/v1", fallbackSrv.URL+"/v1")

	if err := os.WriteFile(mgr.Paths().ProvidersFile(), []byte(providersTOML), 0o600); err != nil {
		t.Fatalf("write providers.toml: %v", err)
	}

	// Reload manager cleanly
	mgr, err = config.NewManager(config.Options{
		VaultRoot: vaultRoot,
		EnvLookup: func(key string) (string, bool) {
			v, ok := secretEnv[key]
			return v, ok
		},
		CmdRunner: func(ctx context.Context, shellCmd string) (string, error) {
			if shellCmd == "fetch-sso-token" {
				return "sso-cmd-secret-token-abcdef123456\n", nil
			}
			return "", errors.New("unexpected command")
		},
	})
	if err != nil {
		t.Fatalf("reload NewManager: %v", err)
	}
	snap := mgr.Current()

	// Wrap NativeStreamer to inspect per-attempt timeout scaling (1.0x -> 1.5x -> 2.25x)
	baseStreamer := NewNativeStreamer(nil)
	var mu sync.Mutex
	var primaryTimeouts []time.Duration
	recordingStreamer := StreamerFunc(func(ctx context.Context, req Request) (<-chan StreamEvent, error) {
		if req.Provider.ID == "anthropic" {
			if dl, ok := ctx.Deadline(); ok {
				mu.Lock()
				primaryTimeouts = append(primaryTimeouts, time.Until(dl))
				mu.Unlock()
			}
		}
		return baseStreamer.Stream(ctx, req)
	})

	router := NewRouter(mgr, mgr.Secrets(), recordingStreamer)

	// 1. Verify strict priority: runOverride > roleProfile (lead->opus, verifier->gpt, explorer->mlx) > wsOverride > globalDefault
	rGlobal, err := router.Resolve("unconfigured-role", "", "", snap)
	if err != nil {
		t.Fatalf("Resolve globalDefault: %v", err)
	}
	if rGlobal.ProviderID != "anthropic" || rGlobal.ModelID != "claude-sonnet-5-5" {
		t.Fatalf("expected globalDefault anthropic/claude-sonnet-5-5, got %s/%s", rGlobal.ProviderID, rGlobal.ModelID)
	}

	rWs, err := router.Resolve("unconfigured-role", "openai/gpt-5.5", "", snap)
	if err != nil {
		t.Fatalf("Resolve wsOverride: %v", err)
	}
	if rWs.ProviderID != "openai" || rWs.ModelID != "gpt-5.5" {
		t.Fatalf("expected wsOverride openai/gpt-5.5, got %s/%s", rWs.ProviderID, rWs.ModelID)
	}

	// Bare model override matching non-default provider owning that model
	rWsBare, err := router.Resolve("unconfigured-role", "qwen3-coder", "", snap)
	if err != nil {
		t.Fatalf("Resolve bare wsOverride: %v", err)
	}
	if rWsBare.ProviderID != "local_mlx" || rWsBare.ModelID != "qwen3-coder" {
		t.Fatalf("expected bare wsOverride local_mlx/qwen3-coder, got %s/%s", rWsBare.ProviderID, rWsBare.ModelID)
	}

	rLead, err := router.Resolve("lead", "local_mlx/qwen3-coder", "", snap)
	if err != nil {
		t.Fatalf("Resolve lead: %v", err)
	}
	if rLead.ProviderID != "anthropic" || rLead.ModelID != "claude-opus-5-5" || rLead.Effort != "xhigh" {
		t.Fatalf("expected lead -> anthropic/claude-opus-5-5 (xhigh) over wsOverride, got %s/%s (%s)", rLead.ProviderID, rLead.ModelID, rLead.Effort)
	}
	if rLead.Fallback == nil || rLead.Fallback.ProviderID != "openai" || rLead.Fallback.ModelID != "gpt-5.5" {
		t.Fatalf("expected lead fallback -> openai/gpt-5.5, got %#v", rLead.Fallback)
	}

	rVerifier, err := router.Resolve("verifier", "local_mlx/qwen3-coder", "", snap)
	if err != nil {
		t.Fatalf("Resolve verifier: %v", err)
	}
	if rVerifier.ProviderID != "openai" || rVerifier.ModelID != "gpt-5.5" {
		t.Fatalf("expected verifier -> openai/gpt-5.5, got %s/%s", rVerifier.ProviderID, rVerifier.ModelID)
	}

	rExplorer, err := router.Resolve("explorer", "openai/gpt-5.5", "", snap)
	if err != nil {
		t.Fatalf("Resolve explorer: %v", err)
	}
	if rExplorer.ProviderID != "local_mlx" || rExplorer.ModelID != "qwen3-coder" || rExplorer.Effort != "low" {
		t.Fatalf("expected explorer -> local_mlx/qwen3-coder (low), got %s/%s (%s)", rExplorer.ProviderID, rExplorer.ModelID, rExplorer.Effort)
	}

	rRunOverride, err := router.Resolve("lead", "local_mlx/qwen3-coder", "local_mlx/qwen3-coder", snap)
	if err != nil {
		t.Fatalf("Resolve runOverride: %v", err)
	}
	if rRunOverride.ProviderID != "local_mlx" || rRunOverride.ModelID != "qwen3-coder" {
		t.Fatalf("expected runOverride -> local_mlx/qwen3-coder over lead roleProfile, got %s/%s", rRunOverride.ProviderID, rRunOverride.ModelID)
	}

	// 2. Simulate mid-stream disconnect on primary (initial + 2 retries = 3 attempts) -> StreamStepReset + StreamRetry -> clean failover to openai/gpt-5.5
	streamCh, err := router.StreamWithRetry(context.Background(), rLead, Request{
		StaticSystem: "Architect system prompt.",
		History: []store.SessionEntry{
			{Seq: 1, ID: "e1", Role: "user", Blocks: []store.ContentBlock{{Type: "text", Text: "Design plan"}}},
		},
	})
	if err != nil {
		t.Fatalf("StreamWithRetry returned error: %v", err)
	}
	events := collectStreamEvents(t, streamCh)

	if got := primaryHits.Load(); got != 3 {
		t.Fatalf("expected 3 attempts on primary (1 initial + 2 retries), got %d", got)
	}
	if got := fallbackHits.Load(); got != 1 {
		t.Fatalf("expected 1 attempt on fallback model, got %d", got)
	}

	// Verify 1.5x timeout scaling across primary attempts (20s -> 30s -> 45s)
	mu.Lock()
	recorded := append([]time.Duration(nil), primaryTimeouts...)
	mu.Unlock()
	if len(recorded) != 3 {
		t.Fatalf("expected 3 recorded primary deadlines, got %d", len(recorded))
	}
	expectedSecs := []float64{20, 30, 45}
	for i, wantSec := range expectedSecs {
		gotSec := recorded[i].Seconds()
		if gotSec < wantSec-2.0 || gotSec > wantSec+2.0 {
			t.Fatalf("attempt %d timeout scaling mismatch: got %.2fs, want ~%.2fs", i+1, gotSec, wantSec)
		}
	}

	// Verify StreamStepReset clears buffered text before each StreamRetry, and final text equals ONLY the clean fallback answer
	var activeText strings.Builder
	var stepResets, retries int
	var gotDone *StreamEvent
	for i := range events {
		ev := events[i]
		switch ev.Type {
		case StreamTextDelta:
			activeText.WriteString(ev.Text)
		case StreamStepReset:
			stepResets++
			activeText.Reset()
			// Next event after a mid-stream reset before retry/failover must be StreamRetry
			if i+1 >= len(events) || events[i+1].Type != StreamRetry {
				t.Fatalf("expected StreamRetry immediately after StreamStepReset at index %d, got events=%#v", i, events)
			}
		case StreamRetry:
			retries++
		case StreamDone:
			cp := ev
			gotDone = &cp
		case StreamError:
			t.Fatalf("unexpected final StreamError after failover: %v", ev.Err)
		}
	}
	if stepResets != 3 {
		t.Fatalf("expected 3 StreamStepReset events (one per dropped partial primary attempt), got %d", stepResets)
	}
	if retries != 3 {
		t.Fatalf("expected 3 StreamRetry events (2 retries + 1 failover transition), got %d", retries)
	}
	if activeText.String() != "Clean failover answer from GPT-5.5." {
		t.Fatalf("expected clean post-reset failover text, got %q", activeText.String())
	}
	if gotDone == nil || gotDone.InputTokens != 42 || gotDone.OutputTokens != 9 {
		t.Fatalf("unexpected failover StreamDone: %#v", gotDone)
	}

	// 3. Verify cmd: secret is rejected when SourceFile is outside vaultRoot, and resolved keys are redacted from errors
	untrustedRoute := ResolvedRoute{
		RoleID:     "coder",
		ProviderID: "evil_overlay",
		ModelID:    "gpt-5.5",
		Provider: config.ProviderConfig{
			ID:         "evil_overlay",
			Protocol:   "openai-compat",
			BaseURL:    fallbackSrv.URL,
			APIKeyRef:  "cmd:fetch-sso-token",
			SourceFile: filepath.Join(t.TempDir(), "untrusted-repo", ".gorness", "providers.toml"),
		},
	}
	if _, err := router.StreamWithRetry(context.Background(), untrustedRoute, Request{}); !errors.Is(err, config.ErrCmdSecretDisallowed) {
		t.Fatalf("expected ErrCmdSecretDisallowed for workspace overlay cmd: secret, got %v", err)
	}

	leakingStreamer := StreamerFunc(func(ctx context.Context, req Request) (<-chan StreamEvent, error) {
		return nil, fmt.Errorf("upstream rejected key %s with 401 Unauthorized", req.APIKey)
	})
	leakRouter := NewRouter(mgr, mgr.Secrets(), leakingStreamer)
	noFallbackRoute := rLead
	noFallbackRoute.Fallback = nil
	errCh, err := leakRouter.StreamWithRetry(context.Background(), noFallbackRoute, Request{})
	var surfacedErr string
	if err != nil {
		surfacedErr = err.Error()
	} else {
		for ev := range errCh {
			if ev.Type == StreamError && ev.Err != nil {
				surfacedErr = ev.Err.Error()
			}
		}
	}
	if strings.Contains(surfacedErr, "sk-ant-secret-primary-99887766") {
		t.Fatalf("resolved API key leaked in error string: %q", surfacedErr)
	}
	if !strings.Contains(surfacedErr, "[REDACTED]") {
		t.Fatalf("expected [REDACTED] in scrubbed error string, got: %q", surfacedErr)
	}
}

func TestPromptAssemblerPrefixDeterminism(t *testing.T) {
	t.Parallel()

	staticParts := []string{
		"You are Software Engineer in Gorness.",
		"Persistent Instructions:\n- Keep changes minimal and race-free.",
	}

	toolsPermA := []ToolSchema{
		{Name: "write_file", Description: "Write file", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)},
		{Name: "edit_file", Description: "Edit file", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)},
		{Name: "read_file", Description: "Read file", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)},
	}
	toolsPermB := []ToolSchema{
		toolsPermA[2],
		toolsPermA[0],
		toolsPermA[1],
	}

	origHistory := []store.SessionEntry{
		{
			Seq:  1,
			ID:   "ent_1",
			Role: "user",
			Blocks: []store.ContentBlock{
				{Type: "text", Text: "Implement router.go"},
			},
		},
	}

	sorted1, frozen1, hist1 := AssemblePrompt(
		staticParts,
		toolsPermA,
		origHistory,
		[]string{"timestamp: 2026-11-16T09:00:00Z", "git status: clean"},
	)

	turn2History := append([]store.SessionEntry{}, origHistory...)
	turn2History = append(turn2History,
		store.SessionEntry{
			Seq:  2,
			ID:   "ent_2",
			Role: "assistant",
			Blocks: []store.ContentBlock{
				{Type: "tool_use", ToolCallID: "tc_1", ToolName: "read_file", ToolInput: json.RawMessage(`{"path":"go.mod"}`)},
			},
		},
		store.SessionEntry{
			Seq:  3,
			ID:   "ent_3",
			Role: "tool",
			Blocks: []store.ContentBlock{
				{Type: "tool_result", ToolCallID: "tc_1", ToolName: "read_file", Text: "module gorness.dev/gorness"},
			},
		},
	)

	sorted2, frozen2, hist2 := AssemblePrompt(
		staticParts,
		toolsPermB,
		turn2History,
		[]string{"timestamp: 2026-11-16T09:04:33Z", "Steer: prioritize 1.5x retry timeout scaling"},
	)

	if frozen1 != frozen2 {
		t.Fatalf("frozenSystem mutated across turns!\nturn1: %q\nturn2: %q", frozen1, frozen2)
	}
	if strings.Contains(frozen1, "2026-11-16") || strings.Contains(frozen2, "Steer:") {
		t.Fatalf("dynamic reminders leaked into frozenSystem: %q", frozen2)
	}

	toolsBytes1, err := json.Marshal(sorted1)
	if err != nil {
		t.Fatalf("marshal sorted1: %v", err)
	}
	toolsBytes2, err := json.Marshal(sorted2)
	if err != nil {
		t.Fatalf("marshal sorted2: %v", err)
	}
	if !bytes.Equal(toolsBytes1, toolsBytes2) {
		t.Fatalf("sortedTools byte representation drifted!\nturn1: %s\nturn2: %s", toolsBytes1, toolsBytes2)
	}
	if sorted1[0].Name != "edit_file" || sorted1[1].Name != "read_file" || sorted1[2].Name != "write_file" {
		t.Fatalf("unexpected tool sort order: %#v", sorted1)
	}

	// Original caller history slice must NOT be mutated
	if len(origHistory[0].Blocks) != 1 {
		t.Fatalf("AssemblePrompt mutated caller's original history slice: %#v", origHistory[0].Blocks)
	}

	// Trailing messages in augmentedHistory must carry <system-reminder> blocks
	last1 := hist1[len(hist1)-1]
	if len(last1.Blocks) != 3 || !strings.HasPrefix(last1.Blocks[1].Text, "<system-reminder>") {
		t.Fatalf("expected <system-reminder> blocks appended to Turn 1 user message, got %#v", last1.Blocks)
	}
	last2 := hist2[len(hist2)-1]
	if last2.Blocks[0].Type != "tool_result" || !strings.Contains(last2.Blocks[len(last2.Blocks)-1].Text, "Steer: prioritize 1.5x retry timeout scaling") {
		t.Fatalf("expected <system-reminder> blocks appended after tool_result on Turn 2, got %#v", last2.Blocks)
	}
	// Earlier user turn in Turn 2 history must NOT carry reminders
	if len(hist2[0].Blocks) != 1 {
		t.Fatalf("non-trailing history entry was unexpectedly modified: %#v", hist2[0].Blocks)
	}
}

func TestMultiMegabyteSSEAndRetryCancelBoundaries(t *testing.T) {
	t.Parallel()

	// 1. 5 MiB single SSE line through SSEReader (>> 4MB & >> 64KB bufio.Scanner cap)
	fiveMB := strings.Repeat("Z", 5*1024*1024)
	rawSSE := "event: delta\ndata: " + fiveMB + "\r\n\r\n"
	reader := NewSSEReader(strings.NewReader(rawSSE))
	ev, err := reader.Next()
	if err != nil {
		t.Fatalf("5 MiB SSEReader.Next failed: %v", err)
	}
	if ev.Event != "delta" || len(ev.Data) != len(fiveMB) || ev.Data != fiveMB {
		t.Fatalf("5 MiB SSEReader data mismatch: event=%q len=%d", ev.Event, len(ev.Data))
	}
	if _, err := reader.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF after final SSE event, got %v", err)
	}

	// 2. HTTP 429 before any partial chunks emits StreamRetry WITHOUT StreamStepReset, then succeeds on retry #1
	var attempts atomic.Int32
	srv429 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"rate limited for token custom-corp-header-secret-777"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Recovered.\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer srv429.Close()

	vaultRoot := filepath.Join(t.TempDir(), ".gorness")
	mgr, err := config.NewManager(config.Options{
		VaultRoot: vaultRoot,
		CmdRunner: func(ctx context.Context, shellCmd string) (string, error) {
			return "evil-out", nil
		},
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	router := NewRouter(mgr, mgr.Secrets(), NewNativeStreamer(srv429.Client()))
	route := ResolvedRoute{
		RoleID:     "coder",
		ProviderID: "corp",
		ModelID:    "qwen3-coder",
		Provider: config.ProviderConfig{
			ID:         "corp",
			Protocol:   "openai-compat",
			BaseURL:    srv429.URL,
			Headers:    map[string]string{"X-Corp-Auth-Token": "custom-corp-header-secret-777"},
			TimeoutSec: 5,
		},
	}

	ch, err := router.StreamWithRetry(context.Background(), route, Request{})
	if err != nil {
		t.Fatalf("StreamWithRetry 429 error: %v", err)
	}
	evs := collectStreamEvents(t, ch)
	var sawReset, sawRetry, sawDone bool
	for _, e := range evs {
		if e.Type == StreamStepReset {
			sawReset = true
		}
		if e.Type == StreamRetry {
			sawRetry = true
			if e.Err != nil && strings.Contains(e.Err.Error(), "custom-corp-header-secret-777") {
				t.Fatalf("custom header secret leaked in retry error: %v", e.Err)
			}
		}
		if e.Type == StreamDone {
			sawDone = true
		}
	}
	if sawReset {
		t.Fatalf("must NOT emit StreamStepReset when HTTP 429 occurred before any partial chunks")
	}
	if !sawRetry || !sawDone {
		t.Fatalf("expected StreamRetry + StreamDone after 429 recovery, got %#v", evs)
	}

	// 3. Parent context cancellation stops immediately without retrying or failing over
	ctx, cancel := context.WithCancel(context.Background())
	var cancelCalls atomic.Int32
	cancelStreamer := StreamerFunc(func(callCtx context.Context, req Request) (<-chan StreamEvent, error) {
		cancelCalls.Add(1)
		cancel()
		return nil, callCtx.Err()
	})
	cancelRouter := NewRouter(mgr, mgr.Secrets(), cancelStreamer)
	fallbackTarget := route
	routeWithFallback := route
	routeWithFallback.Fallback = &fallbackTarget
	cCh, err := cancelRouter.StreamWithRetry(ctx, routeWithFallback, Request{})
	if err != nil {
		t.Fatalf("StreamWithRetry cancel error: %v", err)
	}
	_ = collectStreamEvents(t, cCh)
	if got := cancelCalls.Load(); got != 1 {
		t.Fatalf("cancelled parent context must not retry or failover, got %d calls", got)
	}

	// 4. Untrusted provider SourceFile with cmd: in Headers is rejected even if apiKeySourceFile is inside global vault
	mixedRoute := ResolvedRoute{
		RoleID:     "coder",
		ProviderID: "untrusted_prov",
		ModelID:    "qwen3-coder",
		Provider: config.ProviderConfig{
			ID:         "untrusted_prov",
			Protocol:   "openai-compat",
			BaseURL:    srv429.URL,
			Headers:    map[string]string{"X-Evil": "cmd:id"},
			SourceFile: filepath.Join(t.TempDir(), "ws", ".gorness", "providers.toml"),
		},
		apiKeySourceFile: filepath.Join(mgr.Paths().AgentsDir(), "coder.md"),
	}
	if _, err := router.StreamWithRetry(context.Background(), mixedRoute, Request{}); !errors.Is(err, config.ErrCmdSecretDisallowed) {
		t.Fatalf("expected ErrCmdSecretDisallowed for untrusted header cmd:, got %v", err)
	}
}

func TestFixRound1FailClosedEmptySourceAndCancelDeterminism(t *testing.T) {
	t.Parallel()

	vaultRoot := filepath.Join(t.TempDir(), ".gorness")
	var cmdExecutions atomic.Int32
	mgr, err := config.NewManager(config.Options{
		VaultRoot: vaultRoot,
		CmdRunner: func(ctx context.Context, shellCmd string) (string, error) {
			cmdExecutions.Add(1)
			return "should-never-run-token\n", nil
		},
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	noopStreamer := StreamerFunc(func(ctx context.Context, req Request) (<-chan StreamEvent, error) {
		ch := make(chan StreamEvent, 1)
		ch <- StreamEvent{Type: StreamDone, StopReason: "end_turn"}
		close(ch)
		return ch, nil
	})
	router := NewRouter(mgr, mgr.Secrets(), noopStreamer)

	// 1a. Deserialized / synthetic ProviderConfig with empty SourceFile MUST fail closed on cmd:
	emptySourceRoute := ResolvedRoute{
		RoleID:     "coder",
		ProviderID: "synthetic",
		ModelID:    "gpt-5.5",
		Provider: config.ProviderConfig{
			ID:         "synthetic",
			Protocol:   "openai-compat",
			BaseURL:    "http://127.0.0.1:1",
			APIKeyRef:  "cmd:whoami",
			SourceFile: "", // empty SourceFile (e.g., JSON/API deserialized)
		},
	}
	if _, err := router.StreamWithRetry(context.Background(), emptySourceRoute, Request{}); !errors.Is(err, config.ErrCmdSecretDisallowed) {
		t.Fatalf("expected ErrCmdSecretDisallowed when ProviderConfig.SourceFile is empty, got %v", err)
	}

	// 1b. Router with nil *config.Manager MUST also fail closed on cmd:
	nilCfgRouter := NewRouter(nil, mgr.Secrets(), noopStreamer)
	if _, err := nilCfgRouter.StreamWithRetry(context.Background(), emptySourceRoute, Request{}); !errors.Is(err, config.ErrCmdSecretDisallowed) {
		t.Fatalf("expected ErrCmdSecretDisallowed when Router cfg is nil, got %v", err)
	}

	// 1c. Synthetic AgentProfile with empty SourceFile overriding APIKeyRef on a global provider MUST fail closed on cmd:
	snap := mgr.Current().Clone()
	snap.Agents["synthetic_agent"] = config.AgentProfile{
		ID:         "synthetic_agent",
		Provider:   "anthropic",
		Model:      "claude-sonnet-5-5",
		APIKeyRef:  "cmd:whoami",
		SourceFile: "", // empty SourceFile on profile override
	}
	profRoute, err := router.Resolve("synthetic_agent", "", "", snap)
	if err != nil {
		t.Fatalf("Resolve synthetic_agent: %v", err)
	}
	if _, err := router.StreamWithRetry(context.Background(), profRoute, Request{}); !errors.Is(err, config.ErrCmdSecretDisallowed) {
		t.Fatalf("expected ErrCmdSecretDisallowed when AgentProfile.SourceFile is empty, got %v", err)
	}
	if got := cmdExecutions.Load(); got != 0 {
		t.Fatalf("cmd: runner must NEVER execute for empty SourceFile, executed %d times", got)
	}

	// 2. Deterministic terminal StreamError delivery across 100 cancelled runs (zero 50/50 select drops)
	for iter := 0; iter < 100; iter++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancelMidAttempt := StreamerFunc(func(callCtx context.Context, req Request) (<-chan StreamEvent, error) {
			cancel()
			return nil, callCtx.Err()
		})
		r := NewRouter(mgr, mgr.Secrets(), cancelMidAttempt)
		safeRoute := ResolvedRoute{
			RoleID:     "coder",
			ProviderID: "local",
			ModelID:    "qwen3-coder",
			Provider: config.ProviderConfig{
				ID:       "local",
				Protocol: "openai-compat",
				BaseURL:  "http://127.0.0.1:1",
			},
		}
		ch, err := r.StreamWithRetry(ctx, safeRoute, Request{})
		if err != nil {
			t.Fatalf("iter %d StreamWithRetry error: %v", iter, err)
		}
		evs := collectStreamEvents(t, ch)
		if len(evs) != 1 || evs[0].Type != StreamError || !errors.Is(evs[0].Err, context.Canceled) {
			t.Fatalf("iter %d dropped terminal StreamError on cancelled ctx: got %#v", iter, evs)
		}
	}

	// 3. Full output buffer on StreamDone must NOT deadlock when parent ctx is cancelled without draining out
	started := make(chan struct{})
	burstStreamer := StreamerFunc(func(callCtx context.Context, req Request) (<-chan StreamEvent, error) {
		up := make(chan StreamEvent, 1)
		up <- StreamEvent{Type: StreamDone, StopReason: "end_turn"}
		close(up)
		close(started)
		return up, nil
	})
	burstRouter := NewRouter(mgr, mgr.Secrets(), burstStreamer)
	parentCtx, cancelParent := context.WithCancel(context.Background())
	fullOut := make(chan StreamEvent, 64)
	for i := 0; i < cap(fullOut); i++ {
		fullOut <- StreamEvent{Type: StreamTextDelta, Text: "x"}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = burstRouter.runSingleAttempt(parentCtx, parentCtx, Request{}, 1, fullOut)
	}()

	<-started
	cancelParent()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("runSingleAttempt deadlocked sending StreamDone to full channel after parent ctx cancellation")
	}
}

func TestCrossModelThinkingSignatureIsolationAndFallbackAnsweringModel(t *testing.T) {
	t.Parallel()

	history := []store.SessionEntry{
		{
			Seq:  1,
			ID:   "u1",
			Role: "user",
			Blocks: []store.ContentBlock{
				{Type: "text", Text: "Inspect repo"},
			},
		},
		{
			Seq:     2,
			ID:      "a_gem",
			Role:    "assistant",
			ModelID: "gemini-2.5-pro",
			Blocks: []store.ContentBlock{
				{Type: "thinking", Thinking: "Gemini thought", Signature: "gem_thought_sig_1"},
				{Type: "tool_use", ToolCallID: "call_g1", ToolName: "read_file", ToolInput: json.RawMessage(`{"path":"a.go"}`), Signature: "gem_fc_sig_1"},
			},
		},
		{
			Seq:  3,
			ID:   "t_gem",
			Role: "tool",
			Blocks: []store.ContentBlock{
				{Type: "tool_result", ToolCallID: "call_g1", ToolName: "read_file", Text: "package a"},
			},
		},
		{
			Seq:     4,
			ID:      "a_claude",
			Role:    "assistant",
			ModelID: "claude-opus-5-5",
			Blocks: []store.ContentBlock{
				{Type: "thinking", Thinking: "Claude thought", Signature: "claude_sig_v1"},
				{Type: "tool_use", ToolCallID: "call_c1", ToolName: "read_file", ToolInput: json.RawMessage(`{"path":"b.go"}`), Signature: "claude_foreign_tool_sig"},
			},
		},
		{
			Seq:  5,
			ID:   "t_claude",
			Role: "tool",
			Blocks: []store.ContentBlock{
				{Type: "tool_result", ToolCallID: "call_c1", ToolName: "read_file", Text: "package b"},
			},
		},
	}

	// 1. Anthropic with enableThinking=true MUST drop gemini-2.5-pro thinking blocks while keeping claude-opus-5-5 signed thinking!
	antThinkOn := buildAnthropicMessages(history, true)
	rawAntOn, _ := json.Marshal(antThinkOn)
	if strings.Contains(string(rawAntOn), "gem_thought_sig_1") || strings.Contains(string(rawAntOn), "Gemini thought") {
		t.Fatalf("buildAnthropicMessages(enableThinking=true) leaked foreign Gemini thinking/signature:\n%s", string(rawAntOn))
	}
	if !strings.Contains(string(rawAntOn), "claude_sig_v1") {
		t.Fatalf("buildAnthropicMessages(enableThinking=true) dropped native Claude thinking signature:\n%s", string(rawAntOn))
	}

	// 2. Anthropic with enableThinking=false (e.g. Haiku) MUST omit ALL thinking blocks!
	antThinkOff := buildAnthropicMessages(history, false)
	rawAntOff, _ := json.Marshal(antThinkOff)
	if strings.Contains(string(rawAntOff), `"type":"thinking"`) || strings.Contains(string(rawAntOff), "claude_sig_v1") {
		t.Fatalf("buildAnthropicMessages(enableThinking=false) emitted thinking block:\n%s", string(rawAntOff))
	}

	// 3. Gemini MUST replay ThoughtSignature for gemini-2.5-pro (both thinking & tool_use) and strip Claude signatures!
	gemContents := buildGeminiContents(history)
	rawGem, _ := json.Marshal(gemContents)
	if !strings.Contains(string(rawGem), "gem_thought_sig_1") || !strings.Contains(string(rawGem), "gem_fc_sig_1") {
		t.Fatalf("buildGeminiContents dropped native Gemini thoughtSignature:\n%s", string(rawGem))
	}
	if strings.Contains(string(rawGem), "claude_sig_v1") || strings.Contains(string(rawGem), "claude_foreign_tool_sig") {
		t.Fatalf("buildGeminiContents leaked foreign Claude signature into thoughtSignature:\n%s", string(rawGem))
	}

	// 4. Router.StreamWithRetry attaches the fallback route's ProviderID & ModelID on StreamDone!
	vaultDir := filepath.Join(t.TempDir(), "vault")
	mgr, err := config.NewManager(config.Options{
		VaultRoot: vaultDir,
		EnvLookup: func(k string) (string, bool) { return "test-key-" + k, true },
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	failoverStreamer := StreamerFunc(func(ctx context.Context, req Request) (<-chan StreamEvent, error) {
		if req.Provider.ID == "anthropic" {
			return nil, &APIError{ProviderID: "anthropic", StatusCode: 400, Message: "primary rejected", Retryable: false}
		}
		ch := make(chan StreamEvent, 2)
		ch <- StreamEvent{Type: StreamTextDelta, Text: "Answered by fallback"}
		ch <- StreamEvent{Type: StreamDone, StopReason: "completed", InputTokens: 100, OutputTokens: 50}
		close(ch)
		return ch, nil
	})
	router := NewRouter(mgr, mgr.Secrets(), failoverStreamer)
	route, err := router.Resolve("coder", "", "", mgr.Current())
	if err != nil {
		t.Fatalf("Resolve coder: %v", err)
	}
	ch, err := router.StreamWithRetry(context.Background(), route, Request{
		History: []store.SessionEntry{{Seq: 1, ID: "u1", Role: "user", Blocks: []store.ContentBlock{{Type: "text", Text: "hi"}}}},
	})
	if err != nil {
		t.Fatalf("StreamWithRetry: %v", err)
	}
	var doneEv *StreamEvent
	for ev := range ch {
		if ev.Type == StreamDone {
			cp := ev
			doneEv = &cp
		}
	}
	if doneEv == nil || doneEv.ProviderID != "openai" || doneEv.ModelID != "gpt-5.5" {
		t.Fatalf("expected StreamDone to report fallback openai/gpt-5.5, got %#v", doneEv)
	}
}

func TestSlice1ExponentialBackoffRetryAfterAndNonRetryableOpenAI(t *testing.T) {
	t.Parallel()

	vaultRoot := filepath.Join(t.TempDir(), ".gorness")
	mgr, err := config.NewManager(config.Options{
		VaultRoot: vaultRoot,
		EnvLookup: func(k string) (string, bool) { return "test-secret-" + k, true },
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// 1. Exponential backoff (500ms * 2^attempt + jitter) + Retry-After header floor with zero real sleep via SetSleepFunc
	var backoffCalls atomic.Int32
	srvBackoff := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := backoffCalls.Add(1)
		switch n {
		case 1:
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"message":"upstream overloaded"}}`))
		case 2:
			w.Header().Set("Retry-After", "4")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"rate limit hit"}}`))
		default:
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"BackoffOK\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		}
	}))
	defer srvBackoff.Close()

	router := NewRouter(mgr, mgr.Secrets(), NewNativeStreamer(srvBackoff.Client()))
	var mu sync.Mutex
	var sleeps []time.Duration
	router.SetSleepFunc(func(ctx context.Context, d time.Duration) error {
		mu.Lock()
		sleeps = append(sleeps, d)
		mu.Unlock()
		return ctx.Err()
	})

	routeCompat := ResolvedRoute{
		RoleID:     "coder",
		ProviderID: "compat",
		ModelID:    "qwen3-coder",
		Provider: config.ProviderConfig{
			ID:         "compat",
			Protocol:   "openai-compat",
			BaseURL:    srvBackoff.URL,
			TimeoutSec: 5,
		},
	}

	ch, err := router.StreamWithRetry(context.Background(), routeCompat, Request{})
	if err != nil {
		t.Fatalf("StreamWithRetry backoff: %v", err)
	}
	evs := collectStreamEvents(t, ch)
	var text strings.Builder
	for _, ev := range evs {
		if ev.Type == StreamTextDelta {
			text.WriteString(ev.Text)
		}
	}
	if text.String() != "BackoffOK" {
		t.Fatalf("expected BackoffOK, got %q", text.String())
	}
	mu.Lock()
	gotSleeps := append([]time.Duration(nil), sleeps...)
	mu.Unlock()
	if len(gotSleeps) != 2 {
		t.Fatalf("expected 2 recorded sleep durations, got %v", gotSleeps)
	}
	if gotSleeps[0] < 500*time.Millisecond || gotSleeps[0] > 650*time.Millisecond {
		t.Fatalf("attempt 0 backoff expected in [500ms, 650ms], got %v", gotSleeps[0])
	}
	if gotSleeps[1] < 4*time.Second || gotSleeps[1] > 30*time.Second {
		t.Fatalf("attempt 1 backoff expected Retry-After floor >= 4s, got %v", gotSleeps[1])
	}

	// 2. OpenAI Responses SSE context_length_exceeded MUST NOT retry!
	var respHits atomic.Int32
	srvResponses := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respHits.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"context_length_exceeded\",\"message\":\"Prompt exceeds maximum context length\"}}}\n\n")
	}))
	defer srvResponses.Close()

	respRouter := NewRouter(mgr, mgr.Secrets(), NewNativeStreamer(srvResponses.Client()))
	respRouter.SetBackoff(0)
	chResp, err := respRouter.StreamWithRetry(context.Background(), ResolvedRoute{
		RoleID:     "coder",
		ProviderID: "openai",
		ModelID:    "gpt-5.5",
		Provider: config.ProviderConfig{
			ID:         "openai",
			Protocol:   "openai-responses",
			BaseURL:    srvResponses.URL,
			TimeoutSec: 5,
		},
	}, Request{})
	if err != nil {
		t.Fatalf("StreamWithRetry openai-responses: %v", err)
	}
	_ = collectStreamEvents(t, chResp)
	if got := respHits.Load(); got != 1 {
		t.Fatalf("expected context_length_exceeded SSE to NOT retry (1 hit), got %d hits", got)
	}

	// 3. OpenAI Compat SSE invalid_request_error MUST NOT retry!
	var compatHits atomic.Int32
	srvCompatErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		compatHits.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"error\":{\"type\":\"invalid_request_error\",\"message\":\"Unsupported parameter tool_choice\"}}\n\n")
	}))
	defer srvCompatErr.Close()

	compatRouter := NewRouter(mgr, mgr.Secrets(), NewNativeStreamer(srvCompatErr.Client()))
	compatRouter.SetBackoff(0)
	chCompatErr, err := compatRouter.StreamWithRetry(context.Background(), ResolvedRoute{
		RoleID:     "coder",
		ProviderID: "compat_err",
		ModelID:    "qwen3-coder",
		Provider: config.ProviderConfig{
			ID:         "compat_err",
			Protocol:   "openai-compat",
			BaseURL:    srvCompatErr.URL,
			TimeoutSec: 5,
		},
	}, Request{})
	if err != nil {
		t.Fatalf("StreamWithRetry openai-compat invalid_request_error: %v", err)
	}
	_ = collectStreamEvents(t, chCompatErr)
	if got := compatHits.Load(); got != 1 {
		t.Fatalf("expected invalid_request_error SSE to NOT retry (1 hit), got %d hits", got)
	}
}
