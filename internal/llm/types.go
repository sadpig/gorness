package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/store"
)

const (
	ProtocolAnthropic       = "anthropic"
	ProtocolOpenAIResponses = "openai-responses"
	ProtocolOpenAICompat    = "openai-compat"
	ProtocolGemini          = "gemini"
)

type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
	DeferLoad   bool            `json:"defer_load,omitempty"`
}

type StreamEventType string

const (
	StreamStepReset     StreamEventType = "step_reset"
	StreamThinkingDelta StreamEventType = "thinking_delta"
	StreamTextDelta     StreamEventType = "text_delta"
	StreamToolCall      StreamEventType = "tool_call"
	StreamCompaction    StreamEventType = "compaction"
	StreamRetry         StreamEventType = "retry"
	StreamDone          StreamEventType = "done"
	StreamError         StreamEventType = "error"
)

type StreamEvent struct {
	Type            StreamEventType `json:"type"`
	ProviderID      string          `json:"provider_id,omitempty"`
	ModelID         string          `json:"model_id,omitempty"`
	Text            string          `json:"text,omitempty"`
	Thinking        string          `json:"thinking,omitempty"`
	Signature       string          `json:"signature,omitempty"`
	ToolCallID      string          `json:"tool_call_id,omitempty"`
	ToolName        string          `json:"tool_name,omitempty"`
	ToolInput       json.RawMessage `json:"tool_input,omitempty"`
	Compaction      json.RawMessage `json:"compaction,omitempty"`
	StopReason      string          `json:"stop_reason,omitempty"`
	InputTokens     int             `json:"input_tokens,omitempty"`
	OutputTokens    int             `json:"output_tokens,omitempty"`
	CacheReadTokens int             `json:"cache_read_tokens,omitempty"`
	Attempt         int             `json:"attempt,omitempty"`
	Err             error           `json:"-"`
}

type Request struct {
	Provider         config.ProviderConfig `json:"provider"`
	APIKey           string                `json:"-"`
	Model            string                `json:"model"`
	Effort           string                `json:"effort,omitempty"`
	StaticSystem     string                `json:"static_system,omitempty"`
	DynamicReminders []string              `json:"dynamic_reminders,omitempty"`
	History          []store.SessionEntry  `json:"history,omitempty"`
	Tools            []ToolSchema          `json:"tools,omitempty"`
}

type Streamer interface {
	Stream(ctx context.Context, req Request) (<-chan StreamEvent, error)
}

type StreamerFunc func(ctx context.Context, req Request) (<-chan StreamEvent, error)

func (f StreamerFunc) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	return f(ctx, req)
}

// APIError 表示上游模型网关返回的结构化错误（已完成敏感凭据脱敏）。
type APIError struct {
	ProviderID string
	StatusCode int
	Message    string
	Retryable  bool
}

func (e *APIError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("llm provider %q HTTP %d: %s", e.ProviderID, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("llm provider %q error: %s", e.ProviderID, e.Message)
}

// NativeStreamer 聚合四大零依赖原生 SSE 协议适配器（Anthropic / OpenAI Responses / OpenAI Compat / Gemini）。
type NativeStreamer struct {
	client *http.Client
}

func NewNativeStreamer(client *http.Client) *NativeStreamer {
	if client == nil {
		// 不设置固定 Client.Timeout，交由 Router 每轮 1.5x 动态缩放的 context.WithTimeout 精确管控全生命周期。
		client = &http.Client{}
	}
	return &NativeStreamer{client: client}
}

func (n *NativeStreamer) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	protocol := strings.ToLower(strings.TrimSpace(req.Provider.Protocol))
	switch protocol {
	case ProtocolAnthropic:
		return streamAnthropic(ctx, n.client, req)
	case ProtocolOpenAIResponses:
		return streamOpenAIResponses(ctx, n.client, req)
	case ProtocolOpenAICompat:
		return streamOpenAICompat(ctx, n.client, req)
	case ProtocolGemini:
		return streamGemini(ctx, n.client, req)
	default:
		return nil, fmt.Errorf("unsupported llm protocol %q for provider %q", req.Provider.Protocol, req.Provider.ID)
	}
}

// EstimateCostUSD 根据模型目录单价（USD / 1M tokens）估算单步调用费用。
func EstimateCostUSD(spec config.ModelSpec, inputTokens, outputTokens, cacheReadTokens int) float64 {
	var total float64
	if inputTokens > 0 && spec.InputCostPer1M > 0 {
		total += (float64(inputTokens) / 1_000_000.0) * spec.InputCostPer1M
	}
	if outputTokens > 0 && spec.OutputCostPer1M > 0 {
		total += (float64(outputTokens) / 1_000_000.0) * spec.OutputCostPer1M
	}
	if cacheReadTokens > 0 && spec.CacheReadCostPer1M > 0 {
		total += (float64(cacheReadTokens) / 1_000_000.0) * spec.CacheReadCostPer1M
	}
	return total
}

type scrubbedError struct {
	msg       string
	retryable bool
	hasRetry  bool
	sentinels []error
}

func (e *scrubbedError) Error() string {
	return e.msg
}

func (e *scrubbedError) Is(target error) bool {
	for _, s := range e.sentinels {
		if errors.Is(s, target) {
			return true
		}
	}
	return false
}

// sanitizeError 对错误消息执行 config.Redact 脱敏，仅保留安全的标准哨兵错误映射，绝不通过 Unwrap 暴露含密钥的原始错误对象。
func sanitizeError(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		cp := *apiErr
		cp.Message = config.Redact(cp.Message, secrets)
		return &cp
	}

	var sentinels []error
	for _, candidate := range []error{
		context.Canceled,
		context.DeadlineExceeded,
		io.ErrUnexpectedEOF,
		io.EOF,
		config.ErrCmdSecretDisallowed,
		config.ErrEmptySecretRef,
	} {
		if errors.Is(err, candidate) {
			sentinels = append(sentinels, candidate)
		}
	}

	var se *scrubbedError
	hasRetry := false
	retryVal := false
	if errors.As(err, &se) && se.hasRetry {
		hasRetry = true
		retryVal = se.retryable
	}

	return &scrubbedError{
		msg:       config.Redact(err.Error(), secrets),
		retryable: retryVal,
		hasRetry:  hasRetry,
		sentinels: sentinels,
	}
}

func isHTTPStatusRetryable(code int) bool {
	return code == http.StatusRequestTimeout ||
		code == http.StatusConflict ||
		code == http.StatusTooManyRequests ||
		code >= 500
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, config.ErrCmdSecretDisallowed) || errors.Is(err, config.ErrEmptySecretRef) {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Retryable
	}
	var se *scrubbedError
	if errors.As(err, &se) && se.hasRetry {
		return se.retryable
	}
	return true
}

func readHTTPError(resp *http.Response, providerID string, secrets ...string) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	msg := strings.TrimSpace(string(raw))
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	return &APIError{
		ProviderID: providerID,
		StatusCode: resp.StatusCode,
		Message:    config.Redact(msg, secrets),
		Retryable:  isHTTPStatusRetryable(resp.StatusCode),
	}
}

func emitEvent(ctx context.Context, ch chan<- StreamEvent, ev StreamEvent) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case ch <- ev:
		return true
	default:
	}
	select {
	case ch <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}

func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	b := buf.Bytes()
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	return b, nil
}

func normalizeSchemaJSON(raw json.RawMessage) json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, trimmed); err == nil {
		return json.RawMessage(compacted.Bytes())
	}
	cp := make(json.RawMessage, len(trimmed))
	copy(cp, trimmed)
	return cp
}

func applyProviderHeaders(h http.Header, custom map[string]string) {
	for k, v := range custom {
		key := strings.TrimSpace(k)
		if key != "" {
			h.Set(key, v)
		}
	}
}

func hasHeaderIgnoreCase(custom map[string]string, target string) bool {
	for k := range custom {
		if strings.EqualFold(strings.TrimSpace(k), target) {
			return true
		}
	}
	return false
}

func requestSecrets(req Request) []string {
	var out []string
	if strings.TrimSpace(req.APIKey) != "" {
		out = append(out, req.APIKey)
	}
	for k, v := range req.Provider.Headers {
		if strings.TrimSpace(v) == "" {
			continue
		}
		lk := strings.ToLower(k)
		if strings.Contains(lk, "key") ||
			strings.Contains(lk, "token") ||
			strings.Contains(lk, "auth") ||
			strings.Contains(lk, "secret") ||
			strings.Contains(lk, "cookie") {
			out = append(out, v)
		}
	}
	return out
}
