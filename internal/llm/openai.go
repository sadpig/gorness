package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/store"
)

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

// --- 1. OpenAI Responses API (/v1/responses) ---

type openAIResponsesReasoning struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type openAIResponsesToolWire struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type openAIResponsesRequestWire struct {
	Model           string                    `json:"model"`
	Stream          bool                      `json:"stream"`
	Instructions    string                    `json:"instructions,omitempty"`
	MaxOutputTokens int                       `json:"max_output_tokens,omitempty"`
	Reasoning       *openAIResponsesReasoning `json:"reasoning,omitempty"`
	Tools           []openAIResponsesToolWire `json:"tools,omitempty"`
	Input           []json.RawMessage         `json:"input"`
}

func streamOpenAIResponses(ctx context.Context, client *http.Client, req Request) (<-chan StreamEvent, error) {
	sortedTools, frozenSystem, history := prepareRequest(req)

	spec, hasSpec := req.Provider.Models[req.Model]
	enableThinking := false
	if hasSpec {
		enableThinking = spec.SupportsThinking
	} else {
		enableThinking = strings.TrimSpace(req.Effort) != ""
	}

	wireReq := openAIResponsesRequestWire{
		Model:        req.Model,
		Stream:       true,
		Instructions: frozenSystem,
		Input:        buildOpenAIResponsesInput(history),
	}
	if spec.MaxOutputTokens > 0 {
		wireReq.MaxOutputTokens = spec.MaxOutputTokens
	}
	if enableThinking && strings.TrimSpace(req.Effort) != "" {
		wireReq.Reasoning = &openAIResponsesReasoning{
			Effort:  normalizeOpenAIEffort(req.Effort),
			Summary: "auto",
		}
	}
	if len(sortedTools) > 0 {
		wireReq.Tools = make([]openAIResponsesToolWire, len(sortedTools))
		for i, t := range sortedTools {
			wireReq.Tools[i] = openAIResponsesToolWire{
				Type:        "function",
				Name:        t.Name,
				Description: t.Description,
				Parameters:  normalizeSchemaJSON(t.InputSchema),
			}
		}
	}

	payload, err := marshalNoEscape(wireReq)
	if err != nil {
		return nil, fmt.Errorf("marshal openai-responses request: %w", err)
	}

	endpoint := openAIResponsesURL(req.Provider.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, sanitizeError(err, req.APIKey)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if req.APIKey != "" && !hasHeaderIgnoreCase(req.Provider.Headers, "Authorization") {
		httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	applyProviderHeaders(httpReq.Header, req.Provider.Headers)

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, sanitizeError(err, requestSecrets(req)...)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, readHTTPError(resp, req.Provider.ID, requestSecrets(req)...)
	}

	out := make(chan StreamEvent, 32)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		consumeOpenAIResponsesSSE(ctx, resp.Body, req, out)
	}()
	return out, nil
}

type responsesFuncAcc struct {
	itemID      string
	outputIndex int
	callID      string
	name        string
	initialArgs string
	argsBuf     strings.Builder
	sawDelta    bool
	emitted     bool
}

func consumeOpenAIResponsesSSE(ctx context.Context, body io.Reader, req Request, out chan<- StreamEvent) {
	reader := NewSSEReader(body)
	byItemID := make(map[string]*responsesFuncAcc)
	byIndex := make(map[int]*responsesFuncAcc)
	var ordered []*responsesFuncAcc

	var (
		inputTokens     int
		outputTokens    int
		cacheReadTokens int
		stopReason      string
		emittedAnyTool  bool
		sawCompleted    bool
	)

	getOrCreate := func(itemID string, outIdx int) *responsesFuncAcc {
		if itemID != "" {
			if acc, ok := byItemID[itemID]; ok {
				return acc
			}
		}
		if outIdx >= 0 {
			if acc, ok := byIndex[outIdx]; ok {
				if itemID != "" && acc.itemID == "" {
					acc.itemID = itemID
					byItemID[itemID] = acc
				}
				return acc
			}
		}
		acc := &responsesFuncAcc{itemID: itemID, outputIndex: outIdx}
		if itemID != "" {
			byItemID[itemID] = acc
		}
		if outIdx >= 0 {
			byIndex[outIdx] = acc
		}
		ordered = append(ordered, acc)
		return acc
	}

	emitFuncCall := func(acc *responsesFuncAcc, fallbackArgs string) error {
		if acc == nil || acc.emitted || acc.name == "" {
			return nil
		}
		acc.emitted = true
		rawStr := strings.TrimSpace(fallbackArgs)
		if acc.sawDelta {
			rawStr = strings.TrimSpace(acc.argsBuf.String())
		} else if rawStr == "" {
			rawStr = strings.TrimSpace(acc.initialArgs)
		}
		if rawStr == "" {
			rawStr = "{}"
		}
		if !json.Valid([]byte(rawStr)) {
			return fmt.Errorf("openai-responses invalid function_call JSON for %q: %w", acc.name, io.ErrUnexpectedEOF)
		}
		callID := acc.callID
		if callID == "" {
			callID = acc.itemID
		}
		if callID == "" {
			callID = fmt.Sprintf("call_%d", len(ordered))
		}
		emittedAnyTool = true
		emitEvent(ctx, out, StreamEvent{
			Type:       StreamToolCall,
			ToolCallID: callID,
			ToolName:   acc.name,
			ToolInput:  json.RawMessage(rawStr),
		})
		return nil
	}

	for {
		ev, err := reader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				if !sawCompleted {
					emitEvent(ctx, out, StreamEvent{
						Type: StreamError,
						Err:  fmt.Errorf("openai-responses stream ended before completion: %w", io.ErrUnexpectedEOF),
					})
					return
				}
				break
			}
			emitEvent(ctx, out, StreamEvent{
				Type: StreamError,
				Err:  sanitizeError(err, req.APIKey),
			})
			return
		}

		data := strings.TrimSpace(ev.Data)
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			sawCompleted = true
			break
		}

		var env struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal([]byte(data), &env)
		kind := ev.Event
		if kind == "" {
			kind = env.Type
		}

		switch kind {
		case "response.reasoning_summary_text.delta", "response.reasoning_text.delta", "response.reasoning.delta":
			var d struct {
				Delta string `json:"delta"`
			}
			if json.Unmarshal([]byte(data), &d) == nil && d.Delta != "" {
				if !emitEvent(ctx, out, StreamEvent{Type: StreamThinkingDelta, Thinking: d.Delta}) {
					return
				}
			}

		case "response.output_text.delta":
			var d struct {
				Delta string `json:"delta"`
			}
			if json.Unmarshal([]byte(data), &d) == nil && d.Delta != "" {
				if !emitEvent(ctx, out, StreamEvent{Type: StreamTextDelta, Text: d.Delta}) {
					return
				}
			}

		case "response.output_item.added":
			var added struct {
				OutputIndex int `json:"output_index"`
				Item        struct {
					ID        string `json:"id"`
					Type      string `json:"type"`
					CallID    string `json:"call_id"`
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"item"`
			}
			if uErr := json.Unmarshal([]byte(data), &added); uErr == nil && added.Item.Type == "function_call" {
				acc := getOrCreate(added.Item.ID, added.OutputIndex)
				if added.Item.CallID != "" {
					acc.callID = added.Item.CallID
				}
				if added.Item.Name != "" {
					acc.name = added.Item.Name
				}
				if added.Item.Arguments != "" {
					acc.initialArgs = added.Item.Arguments
				}
			}

		case "response.function_call_arguments.delta":
			var d struct {
				ItemID      string `json:"item_id"`
				OutputIndex *int   `json:"output_index"`
				Delta       string `json:"delta"`
			}
			if uErr := json.Unmarshal([]byte(data), &d); uErr == nil {
				idx := -1
				if d.OutputIndex != nil {
					idx = *d.OutputIndex
				}
				acc := getOrCreate(d.ItemID, idx)
				acc.sawDelta = true
				acc.argsBuf.WriteString(d.Delta)
			}

		case "response.function_call_arguments.done":
			var d struct {
				ItemID      string `json:"item_id"`
				OutputIndex *int   `json:"output_index"`
				Name        string `json:"name"`
				Arguments   string `json:"arguments"`
			}
			if uErr := json.Unmarshal([]byte(data), &d); uErr == nil {
				idx := -1
				if d.OutputIndex != nil {
					idx = *d.OutputIndex
				}
				acc := getOrCreate(d.ItemID, idx)
				if d.Name != "" && acc.name == "" {
					acc.name = d.Name
				}
				if fErr := emitFuncCall(acc, d.Arguments); fErr != nil {
					emitEvent(ctx, out, StreamEvent{Type: StreamError, Err: fErr})
					return
				}
			}

		case "response.output_item.done":
			var doneItem struct {
				OutputIndex int `json:"output_index"`
				Item        struct {
					ID        string `json:"id"`
					Type      string `json:"type"`
					CallID    string `json:"call_id"`
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"item"`
			}
			if uErr := json.Unmarshal([]byte(data), &doneItem); uErr == nil && doneItem.Item.Type == "function_call" {
				acc := getOrCreate(doneItem.Item.ID, doneItem.OutputIndex)
				if doneItem.Item.CallID != "" {
					acc.callID = doneItem.Item.CallID
				}
				if doneItem.Item.Name != "" {
					acc.name = doneItem.Item.Name
				}
				if fErr := emitFuncCall(acc, doneItem.Item.Arguments); fErr != nil {
					emitEvent(ctx, out, StreamEvent{Type: StreamError, Err: fErr})
					return
				}
			}

		case "response.completed", "response.done", "response.incomplete":
			var comp struct {
				Response struct {
					Status            string `json:"status"`
					IncompleteDetails struct {
						Reason string `json:"reason"`
					} `json:"incomplete_details"`
					Output []struct {
						ID        string `json:"id"`
						Type      string `json:"type"`
						CallID    string `json:"call_id"`
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"output"`
					Usage struct {
						InputTokens        int `json:"input_tokens"`
						OutputTokens       int `json:"output_tokens"`
						InputTokensDetails struct {
							CachedTokens int `json:"cached_tokens"`
						} `json:"input_tokens_details"`
					} `json:"usage"`
				} `json:"response"`
			}
			if uErr := json.Unmarshal([]byte(data), &comp); uErr != nil {
				emitEvent(ctx, out, StreamEvent{Type: StreamError, Err: fmt.Errorf("parse openai-responses completion: %w", uErr)})
				return
			}
			sawCompleted = true
			inputTokens = comp.Response.Usage.InputTokens
			outputTokens = comp.Response.Usage.OutputTokens
			cacheReadTokens = comp.Response.Usage.InputTokensDetails.CachedTokens

			for i, item := range comp.Response.Output {
				if item.Type == "function_call" {
					acc := getOrCreate(item.ID, i)
					if item.CallID != "" {
						acc.callID = item.CallID
					}
					if item.Name != "" {
						acc.name = item.Name
					}
					if fErr := emitFuncCall(acc, item.Arguments); fErr != nil {
						emitEvent(ctx, out, StreamEvent{Type: StreamError, Err: fErr})
						return
					}
				}
			}
			for _, acc := range ordered {
				if fErr := emitFuncCall(acc, ""); fErr != nil {
					emitEvent(ctx, out, StreamEvent{Type: StreamError, Err: fErr})
					return
				}
			}

			if comp.Response.Status == "incomplete" {
				stopReason = comp.Response.IncompleteDetails.Reason
				if stopReason == "" {
					stopReason = "incomplete"
				}
			} else if emittedAnyTool {
				stopReason = "tool_use"
			} else if comp.Response.Status != "" {
				stopReason = comp.Response.Status
			} else {
				stopReason = "completed"
			}

			emitEvent(ctx, out, StreamEvent{
				Type:            StreamDone,
				StopReason:      stopReason,
				InputTokens:     inputTokens,
				OutputTokens:    outputTokens,
				CacheReadTokens: cacheReadTokens,
			})
			return

		case "response.failed", "error":
			var failPayload struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
				Response struct {
					Error struct {
						Code    string `json:"code"`
						Message string `json:"message"`
					} `json:"error"`
				} `json:"response"`
			}
			_ = json.Unmarshal([]byte(data), &failPayload)
			msg := failPayload.Error.Message
			if msg == "" {
				msg = failPayload.Response.Error.Message
			}
			if msg == "" {
				msg = data
			}
			emitEvent(ctx, out, StreamEvent{
				Type: StreamError,
				Err: &APIError{
					ProviderID: req.Provider.ID,
					Message:    config.Redact(msg, []string{req.APIKey}),
					Retryable:  true,
				},
			})
			return
		}
	}

	// Reached [DONE] without explicit response.completed event
	for _, acc := range ordered {
		if fErr := emitFuncCall(acc, ""); fErr != nil {
			emitEvent(ctx, out, StreamEvent{Type: StreamError, Err: fErr})
			return
		}
	}
	if stopReason == "" {
		if emittedAnyTool {
			stopReason = "tool_use"
		} else {
			stopReason = "completed"
		}
	}
	emitEvent(ctx, out, StreamEvent{
		Type:            StreamDone,
		StopReason:      stopReason,
		InputTokens:     inputTokens,
		OutputTokens:    outputTokens,
		CacheReadTokens: cacheReadTokens,
	})
}

func buildOpenAIResponsesInput(history []store.SessionEntry) []json.RawMessage {
	items := make([]json.RawMessage, 0, len(history))

	for _, entry := range history {
		switch entry.Role {
		case "user", "tool":
			var textParts []map[string]string
			for _, b := range entry.Blocks {
				switch b.Type {
				case "tool_result":
					raw, err := marshalNoEscape(struct {
						Type   string `json:"type"`
						CallID string `json:"call_id"`
						Output string `json:"output"`
					}{
						Type:   "function_call_output",
						CallID: b.ToolCallID,
						Output: b.Text,
					})
					if err == nil {
						items = append(items, raw)
					}
				case "text", "compaction":
					txt := b.Text
					if txt == "" && len(b.CompactionData) > 0 {
						txt = string(b.CompactionData)
					}
					if txt != "" {
						textParts = append(textParts, map[string]string{
							"type": "input_text",
							"text": txt,
						})
					}
				}
			}
			if len(textParts) > 0 {
				raw, err := marshalNoEscape(struct {
					Type    string              `json:"type"`
					Role    string              `json:"role"`
					Content []map[string]string `json:"content"`
				}{
					Type:    "message",
					Role:    "user",
					Content: textParts,
				})
				if err == nil {
					items = append(items, raw)
				}
			}

		case "assistant":
			var textParts []map[string]string
			flushAssistantText := func() {
				if len(textParts) == 0 {
					return
				}
				raw, err := marshalNoEscape(struct {
					Type    string              `json:"type"`
					Role    string              `json:"role"`
					Content []map[string]string `json:"content"`
				}{
					Type:    "message",
					Role:    "assistant",
					Content: textParts,
				})
				if err == nil {
					items = append(items, raw)
				}
				textParts = nil
			}

			for _, b := range entry.Blocks {
				switch b.Type {
				case "text":
					if b.Text != "" {
						textParts = append(textParts, map[string]string{
							"type": "output_text",
							"text": b.Text,
						})
					}
				case "tool_use":
					flushAssistantText()
					args := strings.TrimSpace(string(b.ToolInput))
					if args == "" {
						args = "{}"
					}
					raw, err := marshalNoEscape(struct {
						Type      string `json:"type"`
						CallID    string `json:"call_id"`
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{
						Type:      "function_call",
						CallID:    b.ToolCallID,
						Name:      b.ToolName,
						Arguments: args,
					})
					if err == nil {
						items = append(items, raw)
					}
				}
			}
			flushAssistantText()
		}
	}

	return items
}

// --- 2. OpenAI Chat Completions Compat (/v1/chat/completions — Apple GenAI, OpenRouter, DeepSeek, Ollama, MLX) ---

type openAICompatFunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type openAICompatToolWire struct {
	Type     string                  `json:"type"`
	Function openAICompatFunctionDef `json:"function"`
}

type openAICompatToolCallWire struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAICompatMessageWire struct {
	Role       string                     `json:"role"`
	Content    any                        `json:"content,omitempty"`
	ToolCallID string                     `json:"tool_call_id,omitempty"`
	ToolCalls  []openAICompatToolCallWire `json:"tool_calls,omitempty"`
}

type openAICompatStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type openAICompatRequestWire struct {
	Model           string                     `json:"model"`
	Stream          bool                       `json:"stream"`
	StreamOptions   *openAICompatStreamOptions `json:"stream_options,omitempty"`
	MaxTokens       int                        `json:"max_tokens,omitempty"`
	ReasoningEffort string                     `json:"reasoning_effort,omitempty"`
	Messages        []openAICompatMessageWire  `json:"messages"`
	Tools           []openAICompatToolWire     `json:"tools,omitempty"`
}

func streamOpenAICompat(ctx context.Context, client *http.Client, req Request) (<-chan StreamEvent, error) {
	sortedTools, frozenSystem, history := prepareRequest(req)

	spec, hasSpec := req.Provider.Models[req.Model]
	wireReq := openAICompatRequestWire{
		Model:         req.Model,
		Stream:        true,
		StreamOptions: &openAICompatStreamOptions{IncludeUsage: true},
		Messages:      buildOpenAICompatMessages(frozenSystem, history),
	}
	if spec.MaxOutputTokens > 0 {
		wireReq.MaxTokens = spec.MaxOutputTokens
	}
	if hasSpec && spec.SupportsThinking && strings.TrimSpace(req.Effort) != "" {
		wireReq.ReasoningEffort = normalizeOpenAIEffort(req.Effort)
	}
	if len(sortedTools) > 0 {
		wireReq.Tools = make([]openAICompatToolWire, len(sortedTools))
		for i, t := range sortedTools {
			wireReq.Tools[i] = openAICompatToolWire{
				Type: "function",
				Function: openAICompatFunctionDef{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  normalizeSchemaJSON(t.InputSchema),
				},
			}
		}
	}

	payload, err := marshalNoEscape(wireReq)
	if err != nil {
		return nil, fmt.Errorf("marshal openai-compat request: %w", err)
	}

	endpoint := openAIChatCompletionsURL(req.Provider.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, sanitizeError(err, req.APIKey)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if req.APIKey != "" && !hasHeaderIgnoreCase(req.Provider.Headers, "Authorization") {
		httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	applyProviderHeaders(httpReq.Header, req.Provider.Headers)

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, sanitizeError(err, requestSecrets(req)...)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, readHTTPError(resp, req.Provider.ID, requestSecrets(req)...)
	}

	out := make(chan StreamEvent, 32)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		consumeOpenAICompatSSE(ctx, resp.Body, req, out)
	}()
	return out, nil
}

type compatToolAcc struct {
	slot    int
	id      string
	name    string
	argsBuf strings.Builder
	emitted bool
}

func consumeOpenAICompatSSE(ctx context.Context, body io.Reader, req Request, out chan<- StreamEvent) {
	reader := NewSSEReader(body)
	bySlot := make(map[int]*compatToolAcc)
	nextImplicitSlot := 0

	var (
		inputTokens     int
		outputTokens    int
		cacheReadTokens int
		stopReason      string
		sawFinish       bool
		sawDoneMarker   bool
	)

	resolveToolAcc := func(idxPtr *int, callID string) *compatToolAcc {
		if idxPtr != nil {
			slot := *idxPtr
			acc, ok := bySlot[slot]
			if !ok {
				acc = &compatToolAcc{slot: slot}
				bySlot[slot] = acc
				if slot >= nextImplicitSlot {
					nextImplicitSlot = slot + 1
				}
			}
			return acc
		}
		// Ollama / 本地网关省略 index 字段时：优先按已有 ID 归并，新 ID 分配独立槽位。
		if callID != "" {
			for _, acc := range bySlot {
				if acc.id == callID {
					return acc
				}
			}
			slot := nextImplicitSlot
			nextImplicitSlot++
			acc := &compatToolAcc{slot: slot, id: callID}
			bySlot[slot] = acc
			return acc
		}
		if len(bySlot) == 0 {
			acc := &compatToolAcc{slot: 0}
			bySlot[0] = acc
			nextImplicitSlot = 1
			return acc
		}
		return bySlot[nextImplicitSlot-1]
	}

	flushAllTools := func() error {
		slots := make([]int, 0, len(bySlot))
		for s := range bySlot {
			slots = append(slots, s)
		}
		sort.Ints(slots)
		for _, s := range slots {
			acc := bySlot[s]
			if acc == nil || acc.emitted || acc.name == "" {
				continue
			}
			acc.emitted = true
			rawArgs := strings.TrimSpace(acc.argsBuf.String())
			if rawArgs == "" {
				rawArgs = "{}"
			}
			if !json.Valid([]byte(rawArgs)) {
				return fmt.Errorf("openai-compat malformed tool arguments for %q: %w", acc.name, io.ErrUnexpectedEOF)
			}
			callID := acc.id
			if callID == "" {
				callID = fmt.Sprintf("call_compat_%d", s)
			}
			emitEvent(ctx, out, StreamEvent{
				Type:       StreamToolCall,
				ToolCallID: callID,
				ToolName:   acc.name,
				ToolInput:  json.RawMessage(rawArgs),
			})
		}
		return nil
	}

	for {
		ev, err := reader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			emitEvent(ctx, out, StreamEvent{
				Type: StreamError,
				Err:  sanitizeError(err, req.APIKey),
			})
			return
		}

		data := strings.TrimSpace(ev.Data)
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			sawDoneMarker = true
			break
		}

		var chunk struct {
			Error *struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
			Choices []struct {
				Index        int                `json:"index"`
				Delta        compatDeltaPayload `json:"delta"`
				Message      compatDeltaPayload `json:"message"`
				FinishReason *string            `json:"finish_reason"`
			} `json:"choices"`
			// Native Ollama NDJSON shape fallback
			Message         *compatDeltaPayload `json:"message"`
			Done            bool                `json:"done"`
			DoneReason      string              `json:"done_reason"`
			PromptEvalCount int                 `json:"prompt_eval_count"`
			EvalCount       int                 `json:"eval_count"`
			Usage           *struct {
				PromptTokens        int `json:"prompt_tokens"`
				CompletionTokens    int `json:"completion_tokens"`
				InputTokens         int `json:"input_tokens"`
				OutputTokens        int `json:"output_tokens"`
				CacheReadTokens     int `json:"cache_read_input_tokens"`
				PromptTokensDetails struct {
					CachedTokens int `json:"cached_tokens"`
				} `json:"prompt_tokens_details"`
			} `json:"usage"`
		}
		if uErr := json.Unmarshal([]byte(data), &chunk); uErr != nil {
			emitEvent(ctx, out, StreamEvent{
				Type: StreamError,
				Err:  fmt.Errorf("parse openai-compat stream chunk: %w", uErr),
			})
			return
		}

		if chunk.Error != nil && chunk.Error.Message != "" {
			emitEvent(ctx, out, StreamEvent{
				Type: StreamError,
				Err: &APIError{
					ProviderID: req.Provider.ID,
					Message:    config.Redact(chunk.Error.Message, []string{req.APIKey}),
					Retryable:  true,
				},
			})
			return
		}

		for _, ch := range chunk.Choices {
			if !emitCompatPayload(ctx, ch.Delta, resolveToolAcc, out) {
				return
			}
			// 仅当网关不在 delta 而是在最终 message 字段中一次性携带内容时兜底消费。
			if ch.Delta.isEmpty() && !ch.Message.isEmpty() {
				if !emitCompatPayload(ctx, ch.Message, resolveToolAcc, out) {
					return
				}
			}
			if ch.FinishReason != nil && *ch.FinishReason != "" {
				sawFinish = true
				stopReason = *ch.FinishReason
			}
		}

		if chunk.Message != nil && len(chunk.Choices) == 0 {
			if !emitCompatPayload(ctx, *chunk.Message, resolveToolAcc, out) {
				return
			}
		}
		if chunk.Done {
			sawFinish = true
			if chunk.DoneReason != "" && stopReason == "" {
				stopReason = chunk.DoneReason
			}
		}

		if chunk.Usage != nil {
			if chunk.Usage.PromptTokens > 0 {
				inputTokens = chunk.Usage.PromptTokens
			} else if chunk.Usage.InputTokens > 0 {
				inputTokens = chunk.Usage.InputTokens
			}
			if chunk.Usage.CompletionTokens > 0 {
				outputTokens = chunk.Usage.CompletionTokens
			} else if chunk.Usage.OutputTokens > 0 {
				outputTokens = chunk.Usage.OutputTokens
			}
			if chunk.Usage.PromptTokensDetails.CachedTokens > 0 {
				cacheReadTokens = chunk.Usage.PromptTokensDetails.CachedTokens
			} else if chunk.Usage.CacheReadTokens > 0 {
				cacheReadTokens = chunk.Usage.CacheReadTokens
			}
		}
		if chunk.PromptEvalCount > 0 && inputTokens == 0 {
			inputTokens = chunk.PromptEvalCount
		}
		if chunk.EvalCount > 0 && outputTokens == 0 {
			outputTokens = chunk.EvalCount
		}
	}

	if !sawFinish && !sawDoneMarker {
		emitEvent(ctx, out, StreamEvent{
			Type: StreamError,
			Err:  fmt.Errorf("openai-compat stream ended before finish_reason or [DONE]: %w", io.ErrUnexpectedEOF),
		})
		return
	}

	if fErr := flushAllTools(); fErr != nil {
		emitEvent(ctx, out, StreamEvent{Type: StreamError, Err: fErr})
		return
	}

	if stopReason == "" {
		if len(bySlot) > 0 {
			stopReason = "tool_calls"
		} else {
			stopReason = "stop"
		}
	}
	emitEvent(ctx, out, StreamEvent{
		Type:            StreamDone,
		StopReason:      stopReason,
		InputTokens:     inputTokens,
		OutputTokens:    outputTokens,
		CacheReadTokens: cacheReadTokens,
	})
}

type compatToolDeltaWire struct {
	Index    *int   `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

type compatDeltaPayload struct {
	ReasoningContent string                `json:"reasoning_content"`
	Reasoning        string                `json:"reasoning"`
	Thinking         string                `json:"thinking"`
	Content          json.RawMessage       `json:"content"`
	ToolCalls        []compatToolDeltaWire `json:"tool_calls"`
}

func (d compatDeltaPayload) isEmpty() bool {
	return d.ReasoningContent == "" &&
		d.Reasoning == "" &&
		d.Thinking == "" &&
		len(bytes.TrimSpace(d.Content)) == 0 &&
		len(d.ToolCalls) == 0
}

func emitCompatPayload(
	ctx context.Context,
	d compatDeltaPayload,
	resolveAcc func(*int, string) *compatToolAcc,
	out chan<- StreamEvent,
) bool {
	for _, rText := range []string{d.ReasoningContent, d.Reasoning, d.Thinking} {
		if rText != "" {
			if !emitEvent(ctx, out, StreamEvent{Type: StreamThinkingDelta, Thinking: rText}) {
				return false
			}
		}
	}

	trimmedContent := bytes.TrimSpace(d.Content)
	if len(trimmedContent) > 0 && !bytes.Equal(trimmedContent, []byte("null")) {
		switch trimmedContent[0] {
		case '"':
			var s string
			if json.Unmarshal(trimmedContent, &s) == nil && s != "" {
				if !emitEvent(ctx, out, StreamEvent{Type: StreamTextDelta, Text: s}) {
					return false
				}
			}
		case '[':
			// Apple GenAI / 企业多模态兼容网关支持的数组分块形式
			var parts []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				Thinking string `json:"thinking"`
			}
			if json.Unmarshal(trimmedContent, &parts) == nil {
				for _, p := range parts {
					switch p.Type {
					case "thinking", "reasoning":
						txt := p.Thinking
						if txt == "" {
							txt = p.Text
						}
						if txt != "" {
							if !emitEvent(ctx, out, StreamEvent{Type: StreamThinkingDelta, Thinking: txt}) {
								return false
							}
						}
					default:
						if p.Text != "" {
							if !emitEvent(ctx, out, StreamEvent{Type: StreamTextDelta, Text: p.Text}) {
								return false
							}
						}
					}
				}
			}
		}
	}

	for _, tc := range d.ToolCalls {
		acc := resolveAcc(tc.Index, tc.ID)
		if tc.ID != "" {
			acc.id = tc.ID
		}
		if tc.Function.Name != "" {
			acc.name = tc.Function.Name
		}
		rawArg := bytes.TrimSpace(tc.Function.Arguments)
		if len(rawArg) > 0 && !bytes.Equal(rawArg, []byte("null")) {
			if rawArg[0] == '"' {
				var argChunk string
				if json.Unmarshal(rawArg, &argChunk) == nil {
					acc.argsBuf.WriteString(argChunk)
				}
			} else {
				// Ollama / 本地网关直接传递 JSON 对象字面量
				acc.argsBuf.Write(rawArg)
			}
		}
	}
	return true
}

func buildOpenAICompatMessages(frozenSystem string, history []store.SessionEntry) []openAICompatMessageWire {
	msgs := make([]openAICompatMessageWire, 0, len(history)+1)
	if frozenSystem != "" {
		msgs = append(msgs, openAICompatMessageWire{
			Role:    "system",
			Content: frozenSystem,
		})
	}

	for _, entry := range history {
		switch entry.Role {
		case "user", "tool":
			var userTexts []string
			for _, b := range entry.Blocks {
				switch b.Type {
				case "tool_result":
					msgs = append(msgs, openAICompatMessageWire{
						Role:       "tool",
						ToolCallID: b.ToolCallID,
						Content:    b.Text,
					})
				case "text", "compaction":
					txt := b.Text
					if txt == "" && len(b.CompactionData) > 0 {
						txt = string(b.CompactionData)
					}
					if txt != "" {
						userTexts = append(userTexts, txt)
					}
				}
			}
			if len(userTexts) > 0 {
				msgs = append(msgs, openAICompatMessageWire{
					Role:    "user",
					Content: strings.Join(userTexts, "\n\n"),
				})
			}

		case "assistant":
			var texts []string
			var toolCalls []openAICompatToolCallWire
			for _, b := range entry.Blocks {
				switch b.Type {
				case "text":
					if b.Text != "" {
						texts = append(texts, b.Text)
					}
				case "tool_use":
					args := strings.TrimSpace(string(b.ToolInput))
					if args == "" {
						args = "{}"
					}
					var tc openAICompatToolCallWire
					tc.ID = b.ToolCallID
					tc.Type = "function"
					tc.Function.Name = b.ToolName
					tc.Function.Arguments = args
					toolCalls = append(toolCalls, tc)
				}
			}
			if len(texts) > 0 || len(toolCalls) > 0 {
				var content any
				if len(texts) > 0 {
					content = strings.Join(texts, "\n")
				}
				msgs = append(msgs, openAICompatMessageWire{
					Role:      "assistant",
					Content:   content,
					ToolCalls: toolCalls,
				})
			}
		}
	}

	return msgs
}

func normalizeOpenAIEffort(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low":
		return "low"
	case "medium":
		return "medium"
	case "xhigh":
		return "xhigh"
	case "high", "max":
		return "high"
	default:
		return strings.TrimSpace(effort)
	}
}

func openAIResponsesURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultOpenAIBaseURL
	}
	switch {
	case strings.HasSuffix(base, "/responses"):
		return base
	case strings.HasSuffix(base, "/v1"):
		return base + "/responses"
	default:
		return base + "/v1/responses"
	}
}

func openAIChatCompletionsURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultOpenAIBaseURL
	}
	switch {
	case strings.HasSuffix(base, "/chat/completions"):
		return base
	case strings.HasSuffix(base, "/v1"):
		return base + "/chat/completions"
	default:
		return base + "/v1/chat/completions"
	}
}
