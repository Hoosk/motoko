package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/styles"
)

func entriesForProviderModels(models []string, err error) []app.Entry {
	if err != nil {
		return []app.Entry{{Kind: app.EntryError, Text: err.Error()}}
	}
	if len(models) == 0 {
		return []app.Entry{{Kind: app.EntrySystem, Text: "El provider no devolvio modelos."}}
	}
	return []app.Entry{{
		Kind: app.EntrySystem,
		Text: fmt.Sprintf("%d modelos cargados. Usa /models para verlos o /models <modelo> para seleccionarlo.", len(models)),
	}}
}

func entriesForAgentResult(result agent.Result, showContext bool) []app.Entry {
	entries := make([]app.Entry, 0, len(result.Steps)+4)
	if showContext && (result.Context.Signals != "" || result.Context.Semantic != "" || result.Context.RelevantFiles != "" || result.Context.RelevantSnippets != "") {
		entries = append(entries, app.Entry{Kind: app.EntrySystem, Text: strings.Join([]string{
			"agent context:",
			"signals: " + result.Context.Signals,
			"semantic: " + result.Context.Semantic,
			"relevant files:",
			result.Context.RelevantFiles,
			"relevant snippets:",
			result.Context.RelevantSnippets,
		}, "\n")})
	}
	if strings.TrimSpace(result.AgentLabel) != "" || result.Duration > 0 {
		entries = append(entries, app.Entry{Kind: app.EntrySystem, Text: styles.AssistantMetaStyle.Render(fmt.Sprintf("agent:%s  elapsed:%s", result.AgentLabel, result.Duration.Round(time.Millisecond)))})
	}
	if result.Usage.TotalTokens > 0 {
		entries = append(entries, app.Entry{
			Kind: app.EntrySystem,
			Text: fmt.Sprintf("tokens in:%d out:%d total:%d", result.Usage.InputTokens, result.Usage.OutputTokens, result.Usage.TotalTokens),
		})
	}
	return entries
}
