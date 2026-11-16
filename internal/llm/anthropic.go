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

const (
	defaultAnthropicBaseURL   = "https://api.anthropic.com"
	defaultAnthropicVersion   = "2023-06-01"
	defaultAnthropicMaxTokens = 16384
)

type anthropicCacheControl struct {
	Type string `json:"type"`
}

// anthropicThinkingConfig 严格仅包含 Type 字段（"adaptive"），从类型层面彻底杜绝发送已废弃的 budget_tokens。
type anthropicThinkingConfig struct {
	Type string `json:"type"`
}

type anthropicOutputConfig struct {
	Effort string `json:"effort"`
}

type anthropicSystemBlock struct {
	Type         string                 `json:"type"`
	Text         string                 `json:"text"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

type anthropicToolWire struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description,omitempty"`
	InputSchema  json.RawMessage        `json:"input_schema"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

type anthropicMessageWire struct {
	Role    string            `json:"role"`
	Content []json.RawMessage `json:"content"`
}

type anthropicRequestWire struct {
	Model        string                   `json:"model"`
	MaxTokens    int                      `json:"max_tokens"`
	Stream       bool                     `json:"stream"`
	Thinking     *anthropicThinkingConfig `json:"thinking,omitempty"`
	OutputConfig *anthropicOutputConfig   `json:"output_config,omitempty"`
	System       []anthropicSystemBlock   `json:"system,omitempty"`
	Tools        []anthropicToolWire      `json:"tools,omitempty"`
	Messages     []anthropicMessageWire   `json:"messages"`
}

func streamAnthropic(ctx context.Context, client *http.Client, req Request) (<-chan StreamEvent, error) {
	sortedTools, frozenSystem, history := prepareRequest(req)

	spec, hasSpec := req.Provider.Models[req.Model]
	maxTokens := spec.MaxOutputTokens
	if maxTokens <= 0 {
		maxTokens = defaultAnthropicMaxTokens
	}

	enableThinking := false
	if hasSpec {
		enableThinking = spec.SupportsThinking
	} else {
		enableThinking = strings.TrimSpace(req.Effort) != "" && !strings.Contains(strings.ToLower(req.Model), "haiku")
	}

	enableCaching := true
	if hasSpec {
		enableCaching = spec.SupportsCaching
	}

	var ephemeral *anthropicCacheControl
	if enableCaching {
		ephemeral = &anthropicCacheControl{Type: "ephemeral"}
	}

	wireReq := anthropicRequestWire{
		Model:     req.Model,
		MaxTokens: maxTokens,
		Stream:    true,
		Messages:  buildAnthropicMessages(history, enableThinking),
	}

	if enableThinking {
		wireReq.Thinking = &anthropicThinkingConfig{Type: "adaptive"}
	}
	if effort := strings.TrimSpace(req.Effort); effort != "" && (!hasSpec || spec.SupportsThinking) {
		wireReq.OutputConfig = &anthropicOutputConfig{Effort: effort}
	}
	if frozenSystem != "" {
		wireReq.System = []anthropicSystemBlock{
			{
				Type:         "text",
				Text:         frozenSystem,
				CacheControl: ephemeral,
			},
		}
	}
	if len(sortedTools) > 0 {
		wireReq.Tools = make([]anthropicToolWire, len(sortedTools))
		for i, t := range sortedTools {
			var cc *anthropicCacheControl
			if i == len(sortedTools)-1 {
				cc = ephemeral
			}
			wireReq.Tools[i] = anthropicToolWire{
				Name:         t.Name,
				Description:  t.Description,
				InputSchema:  normalizeSchemaJSON(t.InputSchema),
				CacheControl: cc,
			}
		}
	}

	payload, err := marshalNoEscape(wireReq)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}

	endpoint := anthropicMessagesURL(req.Provider.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, sanitizeError(err, req.APIKey)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("anthropic-version", defaultAnthropicVersion)
	if req.APIKey != "" && !hasHeaderIgnoreCase(req.Provider.Headers, "x-api-key") {
		httpReq.Header.Set("x-api-key", req.APIKey)
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
		consumeAnthropicSSE(ctx, resp.Body, req, out)
	}()
	return out, nil
}

type anthropicStreamBlock struct {
	kind               string
	id                 string
	name               string
	initialInput       json.RawMessage
	jsonBuf            strings.Builder
	sawInputDelta      bool
	initialCompaction  json.RawMessage
	compactionBuf      strings.Builder
	sawCompactionDelta bool
	stopped            bool
}

func consumeAnthropicSSE(ctx context.Context, body io.Reader, req Request, out chan<- StreamEvent) {
	reader := NewSSEReader(body)
	blocks := make(map[int]*anthropicStreamBlock)

	var (
		inputTokens     int
		outputTokens    int
		cacheReadTokens int
		stopReason      string
		sawMessageStop  bool
	)

	flushBlock := func(idx int, b *anthropicStreamBlock) error {
		if b == nil || b.stopped {
			return nil
		}
		b.stopped = true
		switch b.kind {
		case "tool_use":
			var rawArgs json.RawMessage
			if b.sawInputDelta {
				assembled := strings.TrimSpace(b.jsonBuf.String())
				if assembled == "" {
					rawArgs = json.RawMessage("{}")
				} else {
					if !json.Valid([]byte(assembled)) {
						return fmt.Errorf("anthropic malformed tool_use JSON for %q: %w", b.name, io.ErrUnexpectedEOF)
					}
					rawArgs = json.RawMessage(assembled)
				}
			} else if len(bytes.TrimSpace(b.initialInput)) > 0 {
				rawArgs = append(json.RawMessage(nil), bytes.TrimSpace(b.initialInput)...)
			} else {
				rawArgs = json.RawMessage("{}")
			}
			emitEvent(ctx, out, StreamEvent{
				Type:       StreamToolCall,
				ToolCallID: b.id,
				ToolName:   b.name,
				ToolInput:  rawArgs,
			})
		case "compaction":
			var comp json.RawMessage
			if b.sawCompactionDelta {
				encoded, _ := marshalNoEscape(map[string]string{
					"type":    "compaction",
					"content": b.compactionBuf.String(),
				})
				comp = json.RawMessage(encoded)
			} else if len(b.initialCompaction) > 0 {
				comp = append(json.RawMessage(nil), b.initialCompaction...)
			}
			if len(comp) > 0 {
				emitEvent(ctx, out, StreamEvent{
					Type:       StreamCompaction,
					Compaction: comp,
				})
			}
		}
		return nil
	}

	for {
		ev, err := reader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				if !sawMessageStop {
					emitEvent(ctx, out, StreamEvent{
						Type: StreamError,
						Err:  fmt.Errorf("anthropic stream terminated before message_stop: %w", io.ErrUnexpectedEOF),
					})
				}
				return
			}
			emitEvent(ctx, out, StreamEvent{
				Type: StreamError,
				Err:  sanitizeError(err, req.APIKey),
			})
			return
		}

		data := strings.TrimSpace(ev.Data)
		if data == "" || ev.Event == "ping" {
			continue
		}

		var envelope struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal([]byte(data), &envelope)
		eventKind := ev.Event
		if eventKind == "" {
			eventKind = envelope.Type
		}

		switch eventKind {
		case "ping":
			continue

		case "message_start":
			var ms struct {
				Message struct {
					Usage struct {
						InputTokens          int `json:"input_tokens"`
						OutputTokens         int `json:"output_tokens"`
						CacheReadInputTokens int `json:"cache_read_input_tokens"`
					} `json:"usage"`
				} `json:"message"`
			}
			if uErr := json.Unmarshal([]byte(data), &ms); uErr != nil {
				emitEvent(ctx, out, StreamEvent{Type: StreamError, Err: fmt.Errorf("parse anthropic message_start: %w", uErr)})
				return
			}
			inputTokens = ms.Message.Usage.InputTokens
			outputTokens = ms.Message.Usage.OutputTokens
			cacheReadTokens = ms.Message.Usage.CacheReadInputTokens

		case "content_block_start":
			var cbs struct {
				Index        int             `json:"index"`
				ContentBlock json.RawMessage `json:"content_block"`
			}
			if uErr := json.Unmarshal([]byte(data), &cbs); uErr != nil {
				emitEvent(ctx, out, StreamEvent{Type: StreamError, Err: fmt.Errorf("parse anthropic content_block_start: %w", uErr)})
				return
			}
			var cb struct {
				Type      string          `json:"type"`
				Text      string          `json:"text"`
				Thinking  string          `json:"thinking"`
				Signature string          `json:"signature"`
				ID        string          `json:"id"`
				Name      string          `json:"name"`
				Input     json.RawMessage `json:"input"`
			}
			if uErr := json.Unmarshal(cbs.ContentBlock, &cb); uErr != nil {
				emitEvent(ctx, out, StreamEvent{Type: StreamError, Err: fmt.Errorf("parse anthropic content_block: %w", uErr)})
				return
			}
			st := &anthropicStreamBlock{
				kind:              cb.Type,
				id:                cb.ID,
				name:              cb.Name,
				initialInput:      cb.Input,
				initialCompaction: cbs.ContentBlock,
			}
			blocks[cbs.Index] = st

			switch cb.Type {
			case "text":
				if cb.Text != "" {
					if !emitEvent(ctx, out, StreamEvent{Type: StreamTextDelta, Text: cb.Text}) {
						return
					}
				}
			case "thinking":
				if cb.Thinking != "" || cb.Signature != "" {
					if !emitEvent(ctx, out, StreamEvent{Type: StreamThinkingDelta, Thinking: cb.Thinking, Signature: cb.Signature}) {
						return
					}
				}
			}

		case "content_block_delta":
			var cbd struct {
				Index int `json:"index"`
				Delta struct {
					Type        string `json:"type"`
					Text        string `json:"text"`
					Thinking    string `json:"thinking"`
					Signature   string `json:"signature"`
					PartialJSON string `json:"partial_json"`
					Content     string `json:"content"`
				} `json:"delta"`
			}
			if uErr := json.Unmarshal([]byte(data), &cbd); uErr != nil {
				emitEvent(ctx, out, StreamEvent{Type: StreamError, Err: fmt.Errorf("parse anthropic content_block_delta: %w", uErr)})
				return
			}
			st := blocks[cbd.Index]
			switch cbd.Delta.Type {
			case "text_delta":
				if cbd.Delta.Text != "" {
					if !emitEvent(ctx, out, StreamEvent{Type: StreamTextDelta, Text: cbd.Delta.Text}) {
						return
					}
				}
			case "thinking_delta":
				if cbd.Delta.Thinking != "" {
					if !emitEvent(ctx, out, StreamEvent{Type: StreamThinkingDelta, Thinking: cbd.Delta.Thinking}) {
						return
					}
				}
			case "signature_delta":
				if cbd.Delta.Signature != "" {
					if !emitEvent(ctx, out, StreamEvent{Type: StreamThinkingDelta, Signature: cbd.Delta.Signature}) {
						return
					}
				}
			case "input_json_delta":
				if st == nil {
					st = &anthropicStreamBlock{kind: "tool_use"}
					blocks[cbd.Index] = st
				}
				st.sawInputDelta = true
				st.jsonBuf.WriteString(cbd.Delta.PartialJSON)
			case "compaction_delta":
				if st == nil {
					st = &anthropicStreamBlock{kind: "compaction"}
					blocks[cbd.Index] = st
				}
				st.sawCompactionDelta = true
				st.compactionBuf.WriteString(cbd.Delta.Content)
			}

		case "content_block_stop":
			var cbStop struct {
				Index int `json:"index"`
			}
			if uErr := json.Unmarshal([]byte(data), &cbStop); uErr != nil {
				emitEvent(ctx, out, StreamEvent{Type: StreamError, Err: fmt.Errorf("parse anthropic content_block_stop: %w", uErr)})
				return
			}
			if fErr := flushBlock(cbStop.Index, blocks[cbStop.Index]); fErr != nil {
				emitEvent(ctx, out, StreamEvent{Type: StreamError, Err: fErr})
				return
			}

		case "message_delta":
			var md struct {
				Delta struct {
					StopReason string `json:"stop_reason"`
				} `json:"delta"`
				Usage struct {
					InputTokens          int `json:"input_tokens"`
					OutputTokens         int `json:"output_tokens"`
					CacheReadInputTokens int `json:"cache_read_input_tokens"`
				} `json:"usage"`
			}
			if uErr := json.Unmarshal([]byte(data), &md); uErr != nil {
				emitEvent(ctx, out, StreamEvent{Type: StreamError, Err: fmt.Errorf("parse anthropic message_delta: %w", uErr)})
				return
			}
			if md.Delta.StopReason != "" {
				stopReason = md.Delta.StopReason
			}
			if md.Usage.InputTokens > 0 {
				inputTokens = md.Usage.InputTokens
			}
			if md.Usage.OutputTokens > 0 {
				outputTokens = md.Usage.OutputTokens
			}
			if md.Usage.CacheReadInputTokens > 0 {
				cacheReadTokens = md.Usage.CacheReadInputTokens
			}

		case "message_stop":
			sawMessageStop = true
			indices := make([]int, 0, len(blocks))
			for idx := range blocks {
				indices = append(indices, idx)
			}
			sort.Ints(indices)
			for _, idx := range indices {
				if fErr := flushBlock(idx, blocks[idx]); fErr != nil {
					emitEvent(ctx, out, StreamEvent{Type: StreamError, Err: fErr})
					return
				}
			}
			emitEvent(ctx, out, StreamEvent{
				Type:            StreamDone,
				StopReason:      stopReason,
				InputTokens:     inputTokens,
				OutputTokens:    outputTokens,
				CacheReadTokens: cacheReadTokens,
			})
			return

		case "error":
			var errPayload struct {
				Error struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			}
			_ = json.Unmarshal([]byte(data), &errPayload)
			msg := errPayload.Error.Message
			if msg == "" {
				msg = data
			}
			retryable := errPayload.Error.Type == "overloaded_error" ||
				errPayload.Error.Type == "rate_limit_error" ||
				errPayload.Error.Type == "api_error"
			emitEvent(ctx, out, StreamEvent{
				Type: StreamError,
				Err: &APIError{
					ProviderID: req.Provider.ID,
					Message:    config.Redact(msg, []string{req.APIKey}),
					Retryable:  retryable,
				},
			})
			return
		}
	}
}

func buildAnthropicMessages(history []store.SessionEntry, enableThinking bool) []anthropicMessageWire {
	out := make([]anthropicMessageWire, 0, len(history))

	type userBuckets struct {
		toolResults []json.RawMessage
		others      []json.RawMessage
	}
	var pendingUser *userBuckets

	flushUser := func() {
		if pendingUser == nil {
			return
		}
		combined := make([]json.RawMessage, 0, len(pendingUser.toolResults)+len(pendingUser.others))
		combined = append(combined, pendingUser.toolResults...)
		combined = append(combined, pendingUser.others...)
		if len(combined) > 0 {
			out = append(out, anthropicMessageWire{
				Role:    "user",
				Content: combined,
			})
		}
		pendingUser = nil
	}

	for _, entry := range history {
		switch entry.Role {
		case "assistant":
			foreignModel := isClearlyNonAnthropicModel(entry.ModelID)
			blocks := make([]json.RawMessage, 0, len(entry.Blocks))
			for _, b := range entry.Blocks {
				switch b.Type {
				case "thinking":
					// 当目标请求未启用 thinking、缺少签名，或历史条目明确来自非 Anthropic/Claude 模型（如 gemini*、gpt*）时，跳过思考块以防 400 签名校验失败。
					if !enableThinking || b.Signature == "" || foreignModel {
						continue
					}
					raw, err := marshalNoEscape(struct {
						Type      string `json:"type"`
						Thinking  string `json:"thinking"`
						Signature string `json:"signature"`
					}{
						Type:      "thinking",
						Thinking:  b.Thinking,
						Signature: b.Signature,
					})
					if err == nil {
						blocks = append(blocks, raw)
					}
				case "redacted_thinking":
					if !enableThinking || b.Signature == "" || foreignModel {
						continue
					}
					raw, err := marshalNoEscape(struct {
						Type string `json:"type"`
						Data string `json:"data"`
					}{
						Type: "redacted_thinking",
						Data: b.Signature,
					})
					if err == nil {
						blocks = append(blocks, raw)
					}
				case "compaction":
					if raw := buildAnthropicCompactionBlock(b); len(raw) > 0 {
						blocks = append(blocks, raw)
					}
				case "text":
					if b.Text != "" {
						raw, err := marshalNoEscape(struct {
							Type string `json:"type"`
							Text string `json:"text"`
						}{
							Type: "text",
							Text: b.Text,
						})
						if err == nil {
							blocks = append(blocks, raw)
						}
					}
				case "tool_use":
					input := bytes.TrimSpace(b.ToolInput)
					if len(input) == 0 || !json.Valid(input) {
						input = []byte("{}")
					}
					raw, err := marshalNoEscape(struct {
						Type  string          `json:"type"`
						ID    string          `json:"id"`
						Name  string          `json:"name"`
						Input json.RawMessage `json:"input"`
					}{
						Type:  "tool_use",
						ID:    b.ToolCallID,
						Name:  b.ToolName,
						Input: json.RawMessage(input),
					})
					if err == nil {
						blocks = append(blocks, raw)
					}
				}
			}
			if len(blocks) > 0 {
				flushUser()
				out = append(out, anthropicMessageWire{
					Role:    "assistant",
					Content: blocks,
				})
			}

		case "user", "tool":
			if pendingUser == nil {
				pendingUser = &userBuckets{}
			}
			for _, b := range entry.Blocks {
				switch b.Type {
				case "tool_result":
					raw, err := marshalNoEscape(struct {
						Type      string `json:"type"`
						ToolUseID string `json:"tool_use_id"`
						Content   string `json:"content"`
						IsError   bool   `json:"is_error,omitempty"`
					}{
						Type:      "tool_result",
						ToolUseID: b.ToolCallID,
						Content:   b.Text,
						IsError:   b.IsError,
					})
					if err == nil {
						pendingUser.toolResults = append(pendingUser.toolResults, raw)
					}
				case "text":
					if b.Text != "" {
						raw, err := marshalNoEscape(struct {
							Type string `json:"type"`
							Text string `json:"text"`
						}{
							Type: "text",
							Text: b.Text,
						})
						if err == nil {
							pendingUser.others = append(pendingUser.others, raw)
						}
					}
				case "compaction":
					if raw := buildAnthropicCompactionBlock(b); len(raw) > 0 {
						pendingUser.others = append(pendingUser.others, raw)
					}
				}
			}
		}
	}

	flushUser()
	return out
}

func buildAnthropicCompactionBlock(b store.ContentBlock) json.RawMessage {
	trimmed := bytes.TrimSpace(b.CompactionData)
	if len(trimmed) > 0 && json.Valid(trimmed) {
		var probe map[string]any
		if json.Unmarshal(trimmed, &probe) == nil {
			if _, hasType := probe["type"]; hasType {
				return append(json.RawMessage(nil), trimmed...)
			}
		}
		raw, err := marshalNoEscape(struct {
			Type    string          `json:"type"`
			Content json.RawMessage `json:"content"`
		}{
			Type:    "compaction",
			Content: json.RawMessage(trimmed),
		})
		if err == nil {
			return raw
		}
	}
	if b.Text != "" {
		raw, err := marshalNoEscape(struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		}{
			Type:    "compaction",
			Content: b.Text,
		})
		if err == nil {
			return raw
		}
	}
	return nil
}

func anthropicMessagesURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultAnthropicBaseURL
	}
	switch {
	case strings.HasSuffix(base, "/v1/messages"):
		return base
	case strings.HasSuffix(base, "/v1"):
		return base + "/messages"
	default:
		return base + "/v1/messages"
	}
}

func isClearlyNonAnthropicModel(modelID string) bool {
	m := strings.ToLower(strings.TrimSpace(modelID))
	if m == "" {
		return false
	}
	if strings.Contains(m, "claude") || strings.Contains(m, "anthropic") {
		return false
	}
	if _, rest, ok := strings.Cut(m, "/"); ok && rest != "" {
		m = rest
	}
	for _, pref := range []string{
		"gemini", "gpt-", "gpt4", "gpt5", "chatgpt",
		"o1", "o3", "o4",
		"qwen", "deepseek", "llama", "mistral", "mixtral", "gemma", "glm", "kimi", "yi-", "command-r",
	} {
		if strings.HasPrefix(m, pref) {
			return true
		}
	}
	return false
}
