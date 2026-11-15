package llm

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"

	"gorness.dev/gorness/internal/store"
)

// AssemblePrompt 构造跨轮次字节级确定的静态系统前缀（frozenSystem）与按名称排序的工具定义（sortedTools），
// 并将动态提醒（如时间戳、Git 状态、中途 Steer 指令）作为 <system-reminder> 注入末尾用户/工具消息，绝不污染静态缓存前缀。
func AssemblePrompt(
	staticParts []string,
	tools []ToolSchema,
	history []store.SessionEntry,
	dynamicReminders []string,
) (sortedTools []ToolSchema, frozenSystem string, augmentedHistory []store.SessionEntry) {
	// 1. 构建静态系统提示词前缀，严格排除动态提醒。
	cleanStatic := make([]string, 0, len(staticParts))
	for _, part := range staticParts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			cleanStatic = append(cleanStatic, trimmed)
		}
	}
	frozenSystem = strings.Join(cleanStatic, "\n\n")

	// 2. 深拷贝并按工具名称确定性排序，同时紧凑化 JSON Schema 消除无意义空白漂移。
	sortedTools = make([]ToolSchema, len(tools))
	for i, t := range tools {
		cp := t
		if len(t.InputSchema) > 0 {
			trimmed := bytes.TrimSpace(t.InputSchema)
			var compacted bytes.Buffer
			if err := json.Compact(&compacted, trimmed); err == nil {
				cp.InputSchema = json.RawMessage(compacted.Bytes())
			} else {
				cp.InputSchema = append(json.RawMessage(nil), trimmed...)
			}
		}
		sortedTools[i] = cp
	}
	sort.SliceStable(sortedTools, func(i, j int) bool {
		if sortedTools[i].Name != sortedTools[j].Name {
			return sortedTools[i].Name < sortedTools[j].Name
		}
		return sortedTools[i].Description < sortedTools[j].Description
	})

	// 3. 深拷贝历史会话切片，确保绝不原地修改调用方或 SessionStore 持有的切片。
	augmentedHistory = cloneHistory(history)

	// 4. 将非空动态提醒封装为 <system-reminder> 块并挂载至末尾 user/tool 消息。
	reminderBlocks := make([]store.ContentBlock, 0, len(dynamicReminders))
	for _, rem := range dynamicReminders {
		wrapped := wrapSystemReminder(rem)
		if wrapped == "" {
			continue
		}
		reminderBlocks = append(reminderBlocks, store.ContentBlock{
			Type: "text",
			Text: wrapped,
		})
	}
	if len(reminderBlocks) == 0 {
		return sortedTools, frozenSystem, augmentedHistory
	}

	if len(augmentedHistory) > 0 {
		last := &augmentedHistory[len(augmentedHistory)-1]
		if last.Role == "user" || last.Role == "tool" {
			for _, rb := range reminderBlocks {
				if !hasBlockWithText(last.Blocks, rb.Text) {
					last.Blocks = append(last.Blocks, rb)
				}
			}
			return sortedTools, frozenSystem, augmentedHistory
		}
	}

	augmentedHistory = append(augmentedHistory, store.SessionEntry{
		Role:   "user",
		Blocks: reminderBlocks,
	})
	return sortedTools, frozenSystem, augmentedHistory
}

func prepareRequest(req Request) (sortedTools []ToolSchema, frozenSystem string, augmentedHistory []store.SessionEntry) {
	return AssemblePrompt([]string{req.StaticSystem}, req.Tools, req.History, req.DynamicReminders)
}

func wrapSystemReminder(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "<system-reminder>") && strings.HasSuffix(trimmed, "</system-reminder>") {
		return trimmed
	}
	return "<system-reminder>\n" + trimmed + "\n</system-reminder>"
}

func hasBlockWithText(blocks []store.ContentBlock, text string) bool {
	for i := range blocks {
		if blocks[i].Type == "text" && blocks[i].Text == text {
			return true
		}
	}
	return false
}

func cloneHistory(in []store.SessionEntry) []store.SessionEntry {
	if in == nil {
		return nil
	}
	out := make([]store.SessionEntry, len(in))
	for i, e := range in {
		cp := e
		if e.Blocks != nil {
			cp.Blocks = make([]store.ContentBlock, len(e.Blocks))
			for j, b := range e.Blocks {
				bCopy := b
				if b.ToolInput != nil {
					bCopy.ToolInput = append(json.RawMessage(nil), b.ToolInput...)
				}
				if b.CompactionData != nil {
					bCopy.CompactionData = append(json.RawMessage(nil), b.CompactionData...)
				}
				cp.Blocks[j] = bCopy
			}
		}
		out[i] = cp
	}
	return out
}
