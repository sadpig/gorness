package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/llm"
	"gorness.dev/gorness/internal/store"
)

const KeptRecentTurnsRatio = 0.30

type CompactionResult struct {
	SessionID      string `json:"session_id"`
	AnchorEntryID  string `json:"anchor_entry_id,omitempty"`
	AnchorSeq      uint64 `json:"anchor_seq,omitempty"`
	CompactedTurns int    `json:"compacted_turns"`
	KeptTurns      int    `json:"kept_turns"`
	MergedPrior    bool   `json:"merged_prior"`
	Summary        string `json:"summary,omitempty"`
	Skipped        bool   `json:"skipped,omitempty"`
}

// Compactor 实现锚定增量会话压缩（Anchored Incremental Compaction）：
// 提取已有锚点摘要并与新淘汰的前 70% 历史轮次通过 utility 角色模型合并为新锚点，同时原封不动保留最近 30% 轮次（含签名 Thinking 块）。
type Compactor struct {
	cfg        *config.Manager
	workspaces *store.WorkspaceStore
	sessions   *store.SessionStore
	router     *llm.Router
	now        func() time.Time
}

func NewCompactor(
	cfg *config.Manager,
	workspaces *store.WorkspaceStore,
	sessions *store.SessionStore,
	router *llm.Router,
	nowFn func() time.Time,
) *Compactor {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &Compactor{
		cfg:        cfg,
		workspaces: workspaces,
		sessions:   sessions,
		router:     router,
		now:        nowFn,
	}
}

func (c *Compactor) Compact(ctx context.Context, sessionID string) (*CompactionResult, error) {
	if c == nil || c.sessions == nil {
		return nil, errors.New("compactor: uninitialized session store")
	}
	meta, err := c.sessions.GetMeta(sessionID)
	if err != nil {
		return nil, err
	}

	branch, err := c.sessions.LoadActiveBranch(sessionID)
	if err != nil {
		return nil, err
	}
	if len(branch) == 0 {
		return &CompactionResult{SessionID: sessionID, Skipped: true}, nil
	}

	var priorSummary string
	mergedPrior := false
	conversational := branch
	for len(conversational) > 0 && isCompactionAnchorEntry(conversational[0]) {
		extracted := extractCompactionSummary(conversational[0])
		if extracted != "" {
			if priorSummary == "" {
				priorSummary = extracted
			} else {
				priorSummary += "\n\n" + extracted
			}
			mergedPrior = true
		}
		conversational = conversational[1:]
	}

	turns := groupEntriesIntoTurns(conversational)
	if len(turns) == 1 && len(turns[0]) >= 5 {
		if stepGroups := splitSingleTurnIntoStepUnits(turns[0]); len(stepGroups) >= 2 {
			turns = stepGroups
		}
	}
	totalTurns := len(turns)
	if totalTurns <= 1 {
		return &CompactionResult{
			SessionID: sessionID,
			KeptTurns: totalTurns,
			Skipped:   true,
		}, nil
	}

	keepCount := int(math.Round(float64(totalTurns) * KeptRecentTurnsRatio))
	if keepCount < 1 {
		keepCount = 1
	}
	if keepCount >= totalTurns {
		keepCount = totalTurns - 1
	}
	compactCount := totalTurns - keepCount

	olderTurns := turns[:compactCount]
	keptTurns := turns[compactCount:]

	summaryText, modelID, inTok, outTok, cacheTok, costUSD, err := c.generateMergedSummary(ctx, meta, priorSummary, olderTurns)
	if err != nil {
		return nil, err
	}

	wrappedText := wrapAnchoredSummary(summaryText)
	now := c.now().UTC()
	compJSON, _ := json.Marshal(map[string]any{
		"compacted_turns": compactCount,
		"kept_turns":      keepCount,
		"merged_prior":    mergedPrior,
		"created_at":      now.Format(time.RFC3339),
	})

	prevTotalCost := meta.TotalCostUSD

	// 1. 将会话头指针无损重置至空根（restoreFiles=false 绝不触碰源码文件，亦不删改历史 .jsonl 行）
	if _, _, err := c.sessions.Rewind(sessionID, 0, false, nil, ""); err != nil {
		return nil, fmt.Errorf("reset branch head for compaction: %w", err)
	}

	// 2. 追加全新锚点摘要作为活跃分支根节点（以 Type="text" 输出确保 Anthropic/OpenAI/Gemini 全协议合法，同时保留 CompactionData 元数据）
	anchorEntry, err := c.sessions.Append(sessionID, store.SessionEntry{
		Role:      "user",
		ModelID:   modelID,
		AgentRole: "utility",
		Blocks: []store.ContentBlock{
			{
				Type:           "text",
				Text:           wrappedText,
				CompactionData: json.RawMessage(compJSON),
			},
		},
		InputTokens:     inTok,
		OutputTokens:    outTok,
		CacheReadTokens: cacheTok,
		CostUSD:         costUSD,
		CreatedAt:       now,
	})
	if err != nil {
		return nil, fmt.Errorf("append compaction anchor: %w", err)
	}

	// 3. 逐条原样接续最近 30% 轮次（保留全部签名 Thinking、Signature、ToolInput 等字节）
	for _, turn := range keptTurns {
		for _, orig := range turn {
			cloned := cloneEntryForRechain(orig)
			if _, err := c.sessions.Append(sessionID, cloned); err != nil {
				return nil, fmt.Errorf("re-append kept verbatim entry: %w", err)
			}
		}
	}

	// 4. 保持会话累计历史账单成本单调不减
	minCumulativeCost := prevTotalCost + costUSD
	_, _ = c.sessions.UpdateMeta(sessionID, func(m *store.SessionMeta) {
		if m.TotalCostUSD < minCumulativeCost {
			m.TotalCostUSD = minCumulativeCost
		}
	})

	return &CompactionResult{
		SessionID:      sessionID,
		AnchorEntryID:  anchorEntry.ID,
		AnchorSeq:      anchorEntry.Seq,
		CompactedTurns: compactCount,
		KeptTurns:      keepCount,
		MergedPrior:    mergedPrior,
		Summary:        summaryText,
	}, nil
}

func (c *Compactor) generateMergedSummary(
	ctx context.Context,
	meta *store.SessionMeta,
	priorSummary string,
	olderTurns [][]store.SessionEntry,
) (summary, modelID string, inTok, outTok, cacheTok int, costUSD float64, err error) {
	transcript := formatTurnsForCompaction(olderTurns)

	var promptBuf strings.Builder
	promptBuf.WriteString("Merge any prior anchored summary with the newly aged-out conversation turns into one concise, structured anchored summary.\n")
	promptBuf.WriteString("Required sections:\n")
	promptBuf.WriteString("## Goal & Intent\n## Key Decisions & Context\n## Files & Artifacts Touched\n## Progress & Pending Next Steps\n\n")
	if strings.TrimSpace(priorSummary) != "" {
		promptBuf.WriteString("<prior_anchored_summary>\n")
		promptBuf.WriteString(strings.TrimSpace(priorSummary))
		promptBuf.WriteString("\n</prior_anchored_summary>\n\n")
	}
	promptBuf.WriteString("<turns_to_compact>\n")
	promptBuf.WriteString(transcript)
	promptBuf.WriteString("\n</turns_to_compact>")

	if c.router == nil || c.cfg == nil {
		return buildDeterministicAnchoredFallback(priorSummary, transcript), "deterministic", 0, 0, 0, 0, nil
	}

	res, uErr := executeUtilityPrompt(ctx, c.cfg, c.workspaces, c.router, meta.WorkspaceID, "utility", "", promptBuf.String())
	if uErr != nil {
		return "", "", 0, 0, 0, 0, uErr
	}

	out := strings.TrimSpace(res.Text)
	if out == "" {
		out = buildDeterministicAnchoredFallback(priorSummary, transcript)
	}
	return out, res.ModelID, res.InputTokens, res.OutputTokens, res.CacheReadTokens, res.CostUSD, nil
}

const defaultUtilitySystemPrompt = "You are the Housekeeping Utility agent in Gorness. Produce crisp session titles and faithful anchored summaries preserving exact file paths, decisions, and unresolved tasks."

type utilityPromptResult struct {
	Text            string
	ModelID         string
	InputTokens     int
	OutputTokens    int
	CacheReadTokens int
	CostUSD         float64
}

func executeUtilityPrompt(
	ctx context.Context,
	cfg *config.Manager,
	workspaces *store.WorkspaceStore,
	router *llm.Router,
	workspaceID string,
	role string,
	systemOverride string,
	userPrompt string,
) (utilityPromptResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg == nil || router == nil {
		return utilityPromptResult{}, errors.New("utility prompt runner requires config and router")
	}
	cleanRole := strings.TrimSpace(role)
	if cleanRole == "" {
		cleanRole = "utility"
	}

	snap := cfg.Current()
	var wsRoleModel string
	if workspaces != nil && strings.TrimSpace(workspaceID) != "" {
		if ws, wErr := workspaces.Get(strings.TrimSpace(workspaceID)); wErr == nil && ws != nil {
			wsRoleModel = ws.RoleModels[cleanRole]
			if wsSnap, _, fErr := cfg.ForWorkspace(ws.RootPath, ws.TrustedHashes); fErr == nil && wsSnap != nil {
				snap = wsSnap
			}
		}
	}

	route, err := router.Resolve(cleanRole, wsRoleModel, "", snap)
	if err != nil {
		return utilityPromptResult{}, err
	}

	staticSys := strings.TrimSpace(systemOverride)
	if staticSys == "" {
		staticSys = defaultUtilitySystemPrompt
		if snap != nil {
			if prof, ok := snap.Agents[cleanRole]; ok && strings.TrimSpace(prof.SystemPrompt) != "" {
				staticSys = prof.SystemPrompt
			}
		}
	}

	ch, err := router.StreamWithRetry(ctx, route, llm.Request{
		Model:        route.ModelID,
		Effort:       route.Effort,
		StaticSystem: staticSys,
		History: []store.SessionEntry{
			{
				Seq:  1,
				ID:   "utility_req",
				Role: "user",
				Blocks: []store.ContentBlock{
					{Type: "text", Text: userPrompt},
				},
			},
		},
	})
	if err != nil {
		return utilityPromptResult{}, err
	}

	var (
		textBuf     strings.Builder
		answeringPr string
		answeringID string
		inTok       int
		outTok      int
		cacheTok    int
	)
	for ev := range ch {
		switch ev.Type {
		case llm.StreamStepReset:
			textBuf.Reset()
		case llm.StreamTextDelta:
			textBuf.WriteString(ev.Text)
		case llm.StreamDone:
			answeringPr = ev.ProviderID
			answeringID = ev.ModelID
			inTok = ev.InputTokens
			outTok = ev.OutputTokens
			cacheTok = ev.CacheReadTokens
		case llm.StreamError:
			if ev.Err != nil {
				return utilityPromptResult{}, ev.Err
			}
			return utilityPromptResult{}, errors.New("utility stream failed")
		}
	}

	usedModel := route.ModelID
	usedSpec := route.Spec
	if strings.TrimSpace(answeringID) != "" {
		usedModel = strings.TrimSpace(answeringID)
		usedSpec = resolveRouteSpec(route, snap, answeringPr, usedModel)
	}
	cost := llm.EstimateCostUSD(usedSpec, inTok, outTok, cacheTok)

	return utilityPromptResult{
		Text:            strings.TrimSpace(textBuf.String()),
		ModelID:         usedModel,
		InputTokens:     inTok,
		OutputTokens:    outTok,
		CacheReadTokens: cacheTok,
		CostUSD:         cost,
	}, nil
}

func splitSingleTurnIntoStepUnits(entries []store.SessionEntry) [][]store.SessionEntry {
	if len(entries) < 5 {
		return nil
	}
	var units [][]store.SessionEntry
	for _, e := range entries {
		if len(units) == 0 || e.Role == "assistant" {
			units = append(units, []store.SessionEntry{e})
		} else {
			units[len(units)-1] = append(units[len(units)-1], e)
		}
	}
	// Never split an assistant entry with unanswered tool_use at the end of olderTurns.
	if len(units) < 2 {
		return nil
	}
	return units
}

func isCompactionAnchorEntry(e store.SessionEntry) bool {
	if len(e.Blocks) == 0 {
		return false
	}
	for _, b := range e.Blocks {
		if b.Type == "compaction" ||
			strings.HasPrefix(strings.TrimSpace(b.Text), "<anchored_summary>") ||
			(e.Role == "user" && b.Type == "text" && len(b.CompactionData) > 0) {
			return true
		}
	}
	return false
}

func extractCompactionSummary(e store.SessionEntry) string {
	for _, b := range e.Blocks {
		if b.Text != "" {
			txt := strings.TrimSpace(b.Text)
			txt = strings.TrimPrefix(txt, "<anchored_summary>")
			txt = strings.TrimSuffix(txt, "</anchored_summary>")
			return strings.TrimSpace(txt)
		}
		if len(b.CompactionData) > 0 {
			var parsed struct {
				Content string `json:"content"`
			}
			if json.Unmarshal(b.CompactionData, &parsed) == nil && parsed.Content != "" {
				txt := strings.TrimSpace(parsed.Content)
				txt = strings.TrimPrefix(txt, "<anchored_summary>")
				txt = strings.TrimSuffix(txt, "</anchored_summary>")
				return strings.TrimSpace(txt)
			}
		}
	}
	return ""
}

func groupEntriesIntoTurns(entries []store.SessionEntry) [][]store.SessionEntry {
	if len(entries) == 0 {
		return nil
	}
	var turns [][]store.SessionEntry
	for _, e := range entries {
		isNewUserTurn := e.Role == "user" && !isToolResultOnlyEntry(e) && !isSteerReminderOnlyEntry(e)
		if len(turns) == 0 || isNewUserTurn {
			turns = append(turns, []store.SessionEntry{e})
		} else {
			turns[len(turns)-1] = append(turns[len(turns)-1], e)
		}
	}
	return turns
}

func isToolResultOnlyEntry(e store.SessionEntry) bool {
	if len(e.Blocks) == 0 {
		return false
	}
	for _, b := range e.Blocks {
		if b.Type != "tool_result" {
			return false
		}
	}
	return true
}

func isSteerReminderOnlyEntry(e store.SessionEntry) bool {
	if len(e.Blocks) == 0 {
		return false
	}
	for _, b := range e.Blocks {
		trimmed := strings.TrimSpace(b.Text)
		if b.Type != "text" || !strings.HasPrefix(trimmed, "<system-reminder>") {
			return false
		}
	}
	return true
}

func formatTurnsForCompaction(turns [][]store.SessionEntry) string {
	var b strings.Builder
	for tIdx, turn := range turns {
		fmt.Fprintf(&b, "--- Turn %d ---\n", tIdx+1)
		for _, e := range turn {
			fmt.Fprintf(&b, "[%s]\n", e.Role)
			for _, blk := range e.Blocks {
				switch blk.Type {
				case "text":
					if blk.Text != "" {
						b.WriteString(blk.Text)
						b.WriteByte('\n')
					}
				case "tool_use":
					fmt.Fprintf(&b, "tool_use %s(%s)\n", blk.ToolName, string(blk.ToolInput))
				case "tool_result":
					snippet := blk.Text
					if len(snippet) > 600 {
						snippet = snippet[:600] + "...[truncated]"
					}
					fmt.Fprintf(&b, "tool_result (%s, err=%v): %s\n", blk.ToolCallID, blk.IsError, snippet)
				}
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func wrapAnchoredSummary(summary string) string {
	trimmed := strings.TrimSpace(summary)
	if strings.HasPrefix(trimmed, "<anchored_summary>") && strings.HasSuffix(trimmed, "</anchored_summary>") {
		return trimmed
	}
	return "<anchored_summary>\n" + trimmed + "\n</anchored_summary>"
}

func buildDeterministicAnchoredFallback(prior, transcript string) string {
	var b strings.Builder
	b.WriteString("## Goal & Intent\nCompacted session history.\n")
	if strings.TrimSpace(prior) != "" {
		b.WriteString("## Prior Anchor\n")
		b.WriteString(strings.TrimSpace(prior))
		b.WriteByte('\n')
	}
	b.WriteString("## Newly Compacted Turns\n")
	if len(transcript) > 2000 {
		b.WriteString(transcript[:2000] + "\n...[truncated]")
	} else {
		b.WriteString(transcript)
	}
	return b.String()
}

func cloneEntryForRechain(orig store.SessionEntry) store.SessionEntry {
	cp := orig
	cp.Seq = 0
	cp.ID = ""
	cp.ParentID = ""
	if orig.Blocks != nil {
		cp.Blocks = make([]store.ContentBlock, len(orig.Blocks))
		for i, blk := range orig.Blocks {
			bCopy := blk
			if blk.ToolInput != nil {
				bCopy.ToolInput = append(json.RawMessage(nil), blk.ToolInput...)
			}
			if blk.CompactionData != nil {
				bCopy.CompactionData = append(json.RawMessage(nil), blk.CompactionData...)
			}
			cp.Blocks[i] = bCopy
		}
	}
	return cp
}
