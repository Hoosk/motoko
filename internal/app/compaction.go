package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/provider"
)

func (r *Runtime) doCompact(ctx context.Context) error {
	if r.currentSession == nil || len(r.currentSession.History) == 0 {
		return nil
	}
	active, ok := r.config.Active()
	if !ok {
		return fmt.Errorf("no hay provider activo")
	}
	client, err := r.providerClient(active)
	if err != nil {
		return err
	}

	// 1. Identify which part of the history to compact and which to preserve (recent ~40k tokens).
	// We want to preserve roughly the last 40,000 tokens.
	preserveTokens := 40000
	var recentHistory []provider.ConversationItem
	var oldHistory []provider.ConversationItem

	// Simple heuristic: count chars from the end. 40k tokens ~ 160k chars.
	charBudget := preserveTokens * 4
	currentChars := 0
	splitIdx := len(r.currentSession.History)

	for i := len(r.currentSession.History) - 1; i >= 0; i-- {
		msg := r.currentSession.History[i]
		currentChars += len(msg.Content)
		if currentChars > charBudget {
			break
		}
		splitIdx = i
	}

	// Make sure we always compact at least something if we triggered compaction, 
	// but if splitIdx is 0, we shouldn't compact anything.
	if splitIdx <= 0 {
		return nil // nothing to compact
	}

	oldHistory = r.currentSession.History[:splitIdx]
	recentHistory = r.currentSession.History[splitIdx:]

	// 2. Prune large tool outputs in oldHistory before summarizing
	// to avoid blowing up the context of the summary request itself.
	prunedOldHistory := make([]provider.ConversationItem, 0, len(oldHistory))
	for _, msg := range oldHistory {
		if msg.Role == provider.RoleTool {
			// Extract tool output and truncate it if it's too large
			call, output := provider.ParseToolResultContent(msg.Content)
			if len(output) > 2000 {
				prunedText := fmt.Sprintf("Tool output was large and has been pruned. Size: %d bytes. Summary: %s...", len(output), output[:500])
				msg = provider.ToolResultForInvocation(call, prunedText)
			}
		}
		prunedOldHistory = append(prunedOldHistory, msg)
	}

	// 3. Ask LLM to summarize only the pruned oldHistory
	resp, err := client.Complete(ctx,
		"You are the memory compaction engine. Summarize the following older portion of the conversation. Extract key context, important decisions, and completed tasks. Keep a structured format (e.g., bulleted Markdown) and be very concise.",
		prunedOldHistory,
		provider.ToolSet{},
	)
	if err != nil {
		return err
	}

	summaryText := strings.TrimSpace(resp.FinalText)
	if summaryText == "" {
		return fmt.Errorf("compaction returned empty summary")
	}

	// 4. Update the brain summary.md
	if r.brain != nil {
		prevSummary, _ := r.brain.Read("summary.md")
		newSummary := summaryText
		if prevSummary != "" {
			newSummary = prevSummary + "\n\n### Compaction Update\n" + summaryText
		}
		if err := r.brain.Write("summary.md", newSummary); err != nil {
			return fmt.Errorf("failed to persist compact summary to session brain: %w", err)
		}
	}

	// 5. Update session history: [Summary System Msg] + [Recent History]
	newHistory := []provider.ConversationItem{
		{Role: "system", Content: "Compacted conversation summary:\n" + summaryText},
	}
	newHistory = append(newHistory, recentHistory...)
	
	r.currentSession.History = newHistory
	r.currentSession.LastInputTokens = 0
	return r.currentSession.Save()
}
