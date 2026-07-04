package sessionman

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/system"
)

func (m *Manager) doCompact(ctx context.Context, cfg *config.AppConfig, providerFn func(config.ProviderConfig) (provider.Client, error), cw int) error {
	if m.currentSession == nil || len(m.currentSession.History) == 0 {
		return nil
	}
	active, ok := cfg.Active()
	if !ok {
		return fmt.Errorf("no active provider")
	}
	client, err := providerFn(active)
	if err != nil {
		return err
	}

	preserveTokens := system.PreserveHistoryTokens(cw)
	var recentHistory []provider.ConversationItem
	var oldHistory []provider.ConversationItem

	charBudget := preserveTokens * 4
	currentChars := 0
	splitIdx := len(m.currentSession.History)

	for i := len(m.currentSession.History) - 1; i >= 0; i-- {
		msg := m.currentSession.History[i]
		currentChars += len(msg.Content)
		if currentChars > charBudget {
			break
		}
		splitIdx = i
	}

	if splitIdx <= 0 {
		return nil
	}

	oldHistory = m.currentSession.History[:splitIdx]
	recentHistory = m.currentSession.History[splitIdx:]

	prunedOldHistory := make([]provider.ConversationItem, 0, len(oldHistory))
	for _, msg := range oldHistory {
		if msg.Role == provider.RoleTool {
			if len(msg.Content) > 2000 {
				prunedText := fmt.Sprintf("Tool output was large and has been pruned. Size: %d bytes. Summary: %s...", len(msg.Content), msg.Content[:500])
				msg = provider.ToolResultForInvocation(provider.ToolInvocation{Name: msg.ToolName, CallID: msg.ToolCallID}, prunedText)
			}
		}
		prunedOldHistory = append(prunedOldHistory, msg)
	}
	reqID := fmt.Sprintf("req-%d", time.Now().UnixNano())
	ctx = provider.WithTelemetry(ctx, m.currentSession.ID, reqID)

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

	if m.brain != nil {
		prevSummary, _ := m.brain.Read("summary.md")
		newSummary := summaryText
		if prevSummary != "" {
			newSummary = prevSummary + "\n\n### Compaction Update\n" + summaryText
		}
		if err := m.brain.Write("summary.md", newSummary); err != nil {
			return fmt.Errorf("failed to persist compact summary to session brain: %w", err)
		}
	}

	newHistory := []provider.ConversationItem{
		provider.UserText("Compacted conversation summary:\n" + summaryText),
	}
	newHistory = append(newHistory, recentHistory...)

	m.currentSession.History = newHistory
	m.currentSession.LastInputTokens = 0
	return m.currentSession.Save()
}
