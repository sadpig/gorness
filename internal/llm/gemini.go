package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"gorness.dev/gorness/internal/store"
)

const defaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"

type geminiFunctionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type geminiToolGroup struct {
	FunctionDeclarations []geminiFunctionDecl `json:"functionDeclarations"`
}

type geminiThinkingConfig struct {
	IncludeThoughts bool `json:"includeThoughts,omitempty"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens int                   `json:"maxOutputTokens,omitempty"`
	ThinkingConfig  *geminiThinkingConfig `json:"thinkingConfig,omitempty"`
}

type geminiFunctionCallWire struct {
	ID   string          `json:"id,omitempty"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

type geminiFunctionResponseWire struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiPartWire struct {
	Text             string                      `json:"text,omitempty"`
	Thought          bool                        `json:"thought,omitempty"`
	ThoughtSignature string                      `json:"thoughtSignature,omitempty"`
	FunctionCall     *geminiFunctionCallWire     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponseWire `json:"functionResponse,omitempty"`
}

type geminiContentWire struct {
	Role  string           `json:"role,omitempty"`
	Parts []geminiPartWire `json:"parts"`
}

type geminiRequestWire struct {
	SystemInstruction *geminiContentWire      `json:"systemInstruction,omitempty"`
	Contents          []geminiContentWire     `json:"contents"`
	Tools             []geminiToolGroup       `json:"tools,omitempty"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

func streamGemini(ctx context.Context, client *http.Client, req Request) (<-chan StreamEvent, error) {
	sortedTools, frozenSystem, history := prepareRequest(req)

	spec, hasSpec := req.Provider.Models[req.Model]
	enableThinking := false
	if hasSpec {
		enableThinking = spec.SupportsThinking
	} else {
		enableThinking = strings.TrimSpace(req.Effort) != ""
	}

	wireReq := geminiRequestWire{
		Contents: buildGeminiContents(history),
	}
	if frozenSystem != "" {
		wireReq.SystemInstruction = &geminiContentWire{
			Parts: []geminiPartWire{{Text: frozenSystem}},
		}
	}
	if len(sortedTools) > 0 {
		decls := make([]geminiFunctionDecl, len(sortedTools))
		for i, t := range sortedTools {
			decls[i] = geminiFunctionDecl{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  normalizeSchemaJSON(t.InputSchema),
			}
		}
		wireReq.Tools = []geminiToolGroup{{FunctionDeclarations: decls}}
	}
	if spec.MaxOutputTokens > 0 || enableThinking {
		gc := &geminiGenerationConfig{
			MaxOutputTokens: spec.MaxOutputTokens,
		}
		if enableThinking {
			gc.ThinkingConfig = &geminiThinkingConfig{IncludeThoughts: true}
		}
		wireReq.GenerationConfig = gc
	}

	payload, err := marshalNoEscape(wireReq)
	if err != nil {
		return nil, fmt.Errorf("marshal gemini request: %w", err)
	}

	endpoint, err := geminiStreamURL(req.Provider.BaseURL, req.Model)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, sanitizeError(err, req.APIKey)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	// 严格通过请求头传递 Gemini API Key，杜绝将密钥拼入 URL 查询参数导致错误日志泄露。
	if req.APIKey != "" && !hasHeaderIgnoreCase(req.Provider.Headers, "x-goog-api-key") {
		httpReq.Header.Set("x-goog-api-key", req.APIKey)
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
		consumeGeminiSSE(ctx, resp.Body, req, out)
	}()
	return out, nil
}

func consumeGeminiSSE(ctx context.Context, body io.Reader, req Request, out chan<- StreamEvent) {
	reader := NewSSEReader(body)
	var (
		inputTokens     int
		outputTokens    int
		cacheReadTokens int
		stopReason      string
		sawFinish       bool
		toolSeq         int
	)

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
			sawFinish = true
			break
		}

		var chunk struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text             string                  `json:"text"`
						Thought          bool                    `json:"thought"`
						ThoughtSignature string                  `json:"thoughtSignature"`
						FunctionCall     *geminiFunctionCallWire `json:"functionCall"`
					} `json:"parts"`
				} `json:"content"`
				FinishReason string `json:"finishReason"`
			} `json:"candidates"`
			UsageMetadata *struct {
				PromptTokenCount        int `json:"promptTokenCount"`
				CandidatesTokenCount    int `json:"candidatesTokenCount"`
				ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
				CachedContentTokenCount int `json:"cachedContentTokenCount"`
			} `json:"usageMetadata"`
		}
		if uErr := json.Unmarshal([]byte(data), &chunk); uErr != nil {
			emitEvent(ctx, out, StreamEvent{
				Type: StreamError,
				Err:  fmt.Errorf("parse gemini SSE chunk: %w", uErr),
			})
			return
		}

		for _, cand := range chunk.Candidates {
			for _, part := range cand.Content.Parts {
				switch {
				case part.Thought:
					if part.Text != "" || part.ThoughtSignature != "" {
						if !emitEvent(ctx, out, StreamEvent{
							Type:      StreamThinkingDelta,
							Thinking:  part.Text,
							Signature: part.ThoughtSignature,
						}) {
							return
						}
					}
				case part.FunctionCall != nil:
					toolSeq++
					callID := part.FunctionCall.ID
					if callID == "" {
						callID = fmt.Sprintf("gemini_call_%d", toolSeq)
					}
					args := bytes.TrimSpace(part.FunctionCall.Args)
					if len(args) == 0 || bytes.Equal(args, []byte("null")) {
						args = []byte("{}")
					}
					if !emitEvent(ctx, out, StreamEvent{
						Type:       StreamToolCall,
						ToolCallID: callID,
						ToolName:   part.FunctionCall.Name,
						ToolInput:  json.RawMessage(args),
						Signature:  part.ThoughtSignature,
					}) {
						return
					}
				case part.Text != "":
					if !emitEvent(ctx, out, StreamEvent{
						Type: StreamTextDelta,
						Text: part.Text,
					}) {
						return
					}
				case part.ThoughtSignature != "":
					if !emitEvent(ctx, out, StreamEvent{
						Type:      StreamThinkingDelta,
						Signature: part.ThoughtSignature,
					}) {
						return
					}
				}
			}
			if cand.FinishReason != "" {
				sawFinish = true
				stopReason = cand.FinishReason
			}
		}

		if chunk.UsageMetadata != nil {
			if chunk.UsageMetadata.PromptTokenCount > 0 {
				inputTokens = chunk.UsageMetadata.PromptTokenCount
			}
			outTok := chunk.UsageMetadata.CandidatesTokenCount + chunk.UsageMetadata.ThoughtsTokenCount
			if outTok > 0 {
				outputTokens = outTok
			}
			if chunk.UsageMetadata.CachedContentTokenCount > 0 {
				cacheReadTokens = chunk.UsageMetadata.CachedContentTokenCount
			}
		}
	}

	if !sawFinish {
		emitEvent(ctx, out, StreamEvent{
			Type: StreamError,
			Err:  fmt.Errorf("gemini stream ended before finishReason: %w", io.ErrUnexpectedEOF),
		})
		return
	}

	emitEvent(ctx, out, StreamEvent{
		Type:            StreamDone,
		StopReason:      stopReason,
		InputTokens:     inputTokens,
		OutputTokens:    outputTokens,
		CacheReadTokens: cacheReadTokens,
	})
}

func buildGeminiContents(history []store.SessionEntry) []geminiContentWire {
	toolNamesByID := make(map[string]string)
	for _, entry := range history {
		for _, b := range entry.Blocks {
			if b.Type == "tool_use" && b.ToolCallID != "" && b.ToolName != "" {
				toolNamesByID[b.ToolCallID] = b.ToolName
			}
		}
	}

	contents := make([]geminiContentWire, 0, len(history))
	for _, entry := range history {
		switch entry.Role {
		case "assistant":
			var parts []geminiPartWire
			for _, b := range entry.Blocks {
				switch b.Type {
				case "thinking":
					if b.Thinking != "" || b.Signature != "" {
						parts = append(parts, geminiPartWire{
							Text:             b.Thinking,
							Thought:          true,
							ThoughtSignature: b.Signature,
						})
					}
				case "text":
					if b.Text != "" {
						parts = append(parts, geminiPartWire{Text: b.Text})
					}
				case "tool_use":
					args := bytes.TrimSpace(b.ToolInput)
					if len(args) == 0 || !json.Valid(args) {
						args = []byte("{}")
					}
					parts = append(parts, geminiPartWire{
						ThoughtSignature: b.Signature,
						FunctionCall: &geminiFunctionCallWire{
							ID:   b.ToolCallID,
							Name: b.ToolName,
							Args: json.RawMessage(args),
						},
					})
				}
			}
			if len(parts) > 0 {
				contents = append(contents, geminiContentWire{
					Role:  "model",
					Parts: parts,
				})
			}

		case "user", "tool":
			var parts []geminiPartWire
			for _, b := range entry.Blocks {
				switch b.Type {
				case "tool_result":
					name := b.ToolName
					if name == "" {
						name = toolNamesByID[b.ToolCallID]
					}
					if name == "" {
						name = "tool"
					}
					respObj := map[string]any{
						"output": b.Text,
					}
					if b.IsError {
						respObj["is_error"] = true
					}
					parts = append(parts, geminiPartWire{
						FunctionResponse: &geminiFunctionResponseWire{
							ID:       b.ToolCallID,
							Name:     name,
							Response: respObj,
						},
					})
				case "text", "compaction":
					txt := b.Text
					if txt == "" && len(b.CompactionData) > 0 {
						txt = string(b.CompactionData)
					}
					if txt != "" {
						parts = append(parts, geminiPartWire{Text: txt})
					}
				}
			}
			if len(parts) > 0 {
				if len(contents) > 0 && contents[len(contents)-1].Role == "user" {
					contents[len(contents)-1].Parts = append(contents[len(contents)-1].Parts, parts...)
				} else {
					contents = append(contents, geminiContentWire{
						Role:  "user",
						Parts: parts,
					})
				}
			}
		}
	}

	return contents
}

func geminiStreamURL(baseURL, model string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultGeminiBaseURL
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid gemini base_url %q: %w", baseURL, err)
	}

	modelPath := strings.TrimSpace(model)
	if !strings.HasPrefix(modelPath, "models/") {
		modelPath = "models/" + modelPath
	}

	if !strings.HasSuffix(parsed.Path, ":streamGenerateContent") {
		if !strings.Contains(parsed.Path, "/v1") {
			parsed.Path = strings.TrimRight(parsed.Path, "/") + "/v1beta/" + modelPath + ":streamGenerateContent"
		} else {
			parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + modelPath + ":streamGenerateContent"
		}
	}

	q := parsed.Query()
	q.Set("alt", "sse")
	parsed.RawQuery = q.Encode()
	return parsed.String(), nil
}
