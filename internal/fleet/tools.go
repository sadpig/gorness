package fleet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/core"
	"gorness.dev/gorness/internal/tools"
)

var (
	reReminderCloseTag = regexp.MustCompile(`(?i)</\s*system-reminder\b[^>]*>`)
	rePeerNotesClose   = regexp.MustCompile(`(?i)</\s*peer_blackboard_notes\b[^>]*>`)
)

// makeNodeBlackboardTools 构建节点作用域的 5 个共享黑板工具（通过 core.RunRequest.ExtraTools 注入）。
func (rt *missionRuntime) makeNodeBlackboardTools(nodeID, worktreeBranch string) []core.PluginTool {
	return []core.PluginTool{
		rt.makeClaimTaskTool(nodeID, worktreeBranch),
		rt.makeCompleteTaskTool(nodeID),
		rt.makeSplitTaskTool(nodeID),
		rt.makePostNoteTool(nodeID),
		rt.makeReadNotesTool(nodeID),
	}
}

func (rt *missionRuntime) makeClaimTaskTool(nodeID, worktreeBranch string) tools.Tool {
	return tools.Tool{
		Name:            "blackboard_claim_task",
		Description:     "Atomically claim an open task card from the Fleet Markdown blackboard (`tasks/*.md`) using kernel flock. Omit `task_id` to claim the next open task.",
		InputSchema:     json.RawMessage(`{"type":"object","properties":{"task_id":{"type":"string","description":"Optional specific task ID to claim; omit to claim the next open task"}}}`),
		ReadOnly:        false,
		Mutating:        false,
		ConcurrencySafe: true,
		Handler: func(_ context.Context, _ tools.CallContext, rawInput json.RawMessage) (tools.ToolResult, error) {
			var in struct {
				TaskID string `json:"task_id"`
			}
			if len(bytes.TrimSpace(rawInput)) > 0 {
				if err := json.Unmarshal(rawInput, &in); err != nil {
					return tools.ToolResult{}, fmt.Errorf("invalid blackboard_claim_task input: %w", err)
				}
			}

			var (
				claimed *BlackboardTask
				won     bool
				err     error
			)
			taskID := strings.TrimSpace(in.TaskID)
			if taskID != "" {
				claimed, won, err = rt.bb.ClaimTask(taskID, nodeID, worktreeBranch)
			} else {
				claimed, won, err = rt.bb.ClaimNextOpenTask(nodeID, worktreeBranch)
			}
			if err != nil {
				return tools.ToolResult{}, err
			}
			if !won || claimed == nil {
				out, _ := json.Marshal(map[string]any{
					"claimed": false,
					"reason":  "no matching open task available",
				})
				return tools.ToolResult{Output: string(out)}, nil
			}

			_, desc, _ := rt.bb.GetTaskWithBody(claimed.ID)
			rt.syncBlackboardState(EventTaskUpdated, nodeID, claimed.ID, fmt.Sprintf("%s claimed %s", nodeID, claimed.ID))
			out, _ := json.Marshal(map[string]any{
				"claimed":     true,
				"task":        claimed,
				"description": desc,
			})
			return tools.ToolResult{Output: string(out)}, nil
		},
	}
}

func (rt *missionRuntime) makeCompleteTaskTool(nodeID string) tools.Tool {
	return tools.Tool{
		Name:            "blackboard_complete_task",
		Description:     "Mark a claimed task card on the Fleet Markdown blackboard (`tasks/*.md`) as completed and optionally post a summary note for peer specialists.",
		InputSchema:     json.RawMessage(`{"type":"object","properties":{"task_id":{"type":"string","description":"ID of the task to complete"},"summary":{"type":"string","description":"Optional concise summary of changes or findings"},"landed_commit":{"type":"string","description":"Optional landed git commit SHA"}},"required":["task_id"]}`),
		ReadOnly:        false,
		Mutating:        false,
		ConcurrencySafe: true,
		Handler: func(_ context.Context, _ tools.CallContext, rawInput json.RawMessage) (tools.ToolResult, error) {
			var in struct {
				TaskID       string `json:"task_id"`
				Summary      string `json:"summary"`
				LandedCommit string `json:"landed_commit"`
			}
			if err := json.Unmarshal(rawInput, &in); err != nil {
				return tools.ToolResult{}, fmt.Errorf("invalid blackboard_complete_task input: %w", err)
			}
			taskID := strings.TrimSpace(in.TaskID)
			if taskID == "" {
				return tools.ToolResult{}, errors.New("blackboard_complete_task requires task_id")
			}

			updated, err := rt.bb.CompleteTask(taskID, nodeID, in.LandedCommit)
			if err != nil {
				return tools.ToolResult{}, err
			}
			if summary := strings.TrimSpace(in.Summary); summary != "" {
				_, _ = rt.bb.AddNote(nodeID, "Task "+taskID+" Completed", summary)
			}
			rt.syncBlackboardState(EventTaskUpdated, nodeID, taskID, "completed "+taskID)
			out, _ := json.Marshal(map[string]any{
				"completed": true,
				"task":      updated,
			})
			return tools.ToolResult{Output: string(out)}, nil
		},
	}
}

func (rt *missionRuntime) makeSplitTaskTool(nodeID string) tools.Tool {
	return tools.Tool{
		Name:            "blackboard_split_task",
		Description:     "Split a complex blackboard task into concrete subtasks (`tasks/*.md`, maximum depth 3) so peer specialists can claim them concurrently.",
		InputSchema:     json.RawMessage(`{"type":"object","properties":{"task_id":{"type":"string","description":"Parent task ID to split"},"subtasks":{"type":"array","description":"Subtasks to create","items":{"type":"object","properties":{"id":{"type":"string","description":"Optional subtask ID"},"title":{"type":"string","description":"Subtask title"},"description":{"type":"string","description":"Subtask brief and file scope"}},"required":["title"]}}},"required":["task_id","subtasks"]}`),
		ReadOnly:        false,
		Mutating:        false,
		ConcurrencySafe: true,
		Handler: func(_ context.Context, _ tools.CallContext, rawInput json.RawMessage) (tools.ToolResult, error) {
			var in struct {
				TaskID   string        `json:"task_id"`
				Subtasks []SubtaskSpec `json:"subtasks"`
			}
			if err := json.Unmarshal(rawInput, &in); err != nil {
				return tools.ToolResult{}, fmt.Errorf("invalid blackboard_split_task input: %w", err)
			}
			parentID := strings.TrimSpace(in.TaskID)
			if parentID == "" {
				return tools.ToolResult{}, errors.New("blackboard_split_task requires task_id")
			}
			created, err := rt.bb.SplitTask(parentID, nodeID, in.Subtasks)
			if err != nil {
				return tools.ToolResult{}, err
			}
			rt.syncBlackboardState(EventTaskUpdated, nodeID, parentID, fmt.Sprintf("split %s into %d subtasks", parentID, len(created)))
			out, _ := json.Marshal(map[string]any{
				"split":     true,
				"parent_id": parentID,
				"subtasks":  created,
			})
			return tools.ToolResult{Output: string(out)}, nil
		},
	}
}

func (rt *missionRuntime) makePostNoteTool(nodeID string) tools.Tool {
	return tools.Tool{
		Name:            "blackboard_post_note",
		Description:     "Publish a structured peer note (`notes/*.md`) to the Fleet blackboard to share findings, interface contracts, or blockers with other nodes.",
		InputSchema:     json.RawMessage(`{"type":"object","properties":{"title":{"type":"string","description":"Concise note title"},"body":{"type":"string","description":"Markdown body content"},"content":{"type":"string","description":"Alias for body"}},"required":["title"]}`),
		ReadOnly:        false,
		Mutating:        false,
		ConcurrencySafe: true,
		Handler: func(_ context.Context, _ tools.CallContext, rawInput json.RawMessage) (tools.ToolResult, error) {
			var in struct {
				Title   string `json:"title"`
				Body    string `json:"body"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(rawInput, &in); err != nil {
				return tools.ToolResult{}, fmt.Errorf("invalid blackboard_post_note input: %w", err)
			}
			body := strings.TrimSpace(in.Body)
			if body == "" {
				body = strings.TrimSpace(in.Content)
			}
			if strings.TrimSpace(in.Title) == "" && body == "" {
				return tools.ToolResult{}, errors.New("blackboard_post_note requires title or body")
			}
			note, err := rt.recordNote(nodeID, in.Title, body)
			if err != nil {
				return tools.ToolResult{}, err
			}
			out, _ := json.Marshal(map[string]any{
				"posted": true,
				"note":   note,
			})
			return tools.ToolResult{Output: string(out)}, nil
		},
	}
}

func (rt *missionRuntime) makeReadNotesTool(nodeID string) tools.Tool {
	return tools.Tool{
		Name:            "blackboard_read_notes",
		Description:     "Read peer notes (`notes/*.md`) posted to the Fleet Markdown blackboard.",
		InputSchema:     json.RawMessage(`{"type":"object","properties":{"limit":{"type":"integer","description":"Maximum number of recent notes to return (default 20)"},"exclude_self":{"type":"boolean","description":"When true, omit notes authored by the calling node"}}}`),
		ReadOnly:        true,
		Mutating:        false,
		ConcurrencySafe: true,
		Handler: func(_ context.Context, _ tools.CallContext, rawInput json.RawMessage) (tools.ToolResult, error) {
			var in struct {
				Limit       int  `json:"limit"`
				ExcludeSelf bool `json:"exclude_self"`
			}
			if len(bytes.TrimSpace(rawInput)) > 0 {
				if err := json.Unmarshal(rawInput, &in); err != nil {
					return tools.ToolResult{}, fmt.Errorf("invalid blackboard_read_notes input: %w", err)
				}
			}
			limit := in.Limit
			if limit <= 0 {
				limit = 20
			}
			notes, err := rt.bb.ListNotes()
			if err != nil {
				return tools.ToolResult{}, err
			}
			filtered := make([]BlackboardNote, 0, len(notes))
			for _, n := range notes {
				if in.ExcludeSelf && n.NodeID == nodeID {
					continue
				}
				n.Title = sanitizePeerText(n.Title)
				n.Body = sanitizePeerText(n.Body)
				filtered = append(filtered, n)
			}
			if len(filtered) > limit {
				filtered = filtered[len(filtered)-limit:]
			}
			out, _ := json.Marshal(map[string]any{
				"notes": filtered,
			})
			return tools.ToolResult{Output: string(out)}, nil
		},
	}
}

// peerNotesPromptBlock 读取当前黑板上由其他节点发布的对等笔记并格式化为安全紧凑摘要段落，供专家与综合节点提示词注入。
func (rt *missionRuntime) peerNotesPromptBlock(excludeNodeID string) string {
	if rt == nil || rt.bb == nil {
		return ""
	}
	notes, err := rt.bb.ListNotes()
	if err != nil || len(notes) == 0 {
		return ""
	}
	return formatPeerNotesPromptSection(notes, excludeNodeID, 12, 320)
}

func formatPeerNotesPromptSection(notes []BlackboardNote, excludeNodeID string, maxNotes, maxBodyBytes int) string {
	if len(notes) == 0 {
		return ""
	}
	if maxNotes <= 0 {
		maxNotes = 12
	}
	if maxBodyBytes <= 0 {
		maxBodyBytes = 320
	}

	peers := make([]BlackboardNote, 0, len(notes))
	for _, n := range notes {
		if excludeNodeID != "" && n.NodeID == excludeNodeID {
			continue
		}
		if strings.TrimSpace(n.Title) == "" && strings.TrimSpace(n.Body) == "" {
			continue
		}
		peers = append(peers, n)
	}
	if len(peers) == 0 {
		return ""
	}
	if len(peers) > maxNotes {
		peers = peers[len(peers)-maxNotes:]
	}

	var lines []string
	for _, n := range peers {
		title := strings.TrimSpace(sanitizePeerText(n.Title))
		body := strings.Join(strings.Fields(sanitizePeerText(n.Body)), " ")
		if len(body) > maxBodyBytes {
			body = body[:maxBodyBytes] + "..."
		}
		if title != "" && body != "" {
			lines = append(lines, fmt.Sprintf("- [#%d from %s — %s]: %s", n.Seq, n.NodeID, title, body))
		} else if body != "" {
			lines = append(lines, fmt.Sprintf("- [#%d from %s]: %s", n.Seq, n.NodeID, body))
		} else {
			lines = append(lines, fmt.Sprintf("- [#%d from %s]: %s", n.Seq, n.NodeID, title))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "<peer_blackboard_notes>\n" + strings.Join(lines, "\n") + "\n</peer_blackboard_notes>"
}

func appendPeerNotesSection(prompt, peerBlock string) string {
	if strings.TrimSpace(peerBlock) == "" {
		return prompt
	}
	return strings.TrimRight(prompt, "\n") + "\n\nPeer Blackboard Notes:\n" + peerBlock
}

func sanitizePeerText(s string) string {
	clean := config.EscapeBidi(s)
	clean = reReminderCloseTag.ReplaceAllString(clean, "[/system-reminder]")
	clean = rePeerNotesClose.ReplaceAllString(clean, "[/peer_blackboard_notes]")
	return clean
}
