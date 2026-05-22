package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tracelog"
)

func (r *Runtime) RunAgent(ctx context.Context, info system.ContextInfo, input string) (agent.Result, error) {
	if r.agent == nil || !r.agent.Configured() {
		return agent.Result{}, fmt.Errorf("agente no configurado")
	}
	info = r.enrichContext(ctx, info, input)
	priorHistory := []provider.ConversationItem(nil)
	if r.currentSession != nil {
		priorHistory = append(priorHistory, r.currentSession.History...)
	}
	result, err := r.agent.Run(ctx, info, input, priorHistory)
	if err != nil {
		return result, err
	}
	r.persistTurn(result)
	return result, r.maybeAutoCompact(ctx, nil)
}

func (r *Runtime) RunAgentStream(ctx context.Context, info system.ContextInfo, input string, onEvent func(AgentStreamEvent) error) (agent.Result, error) {
	if r.agent == nil || !r.agent.Configured() {
		return agent.Result{}, fmt.Errorf("agente no configurado")
	}
	info = r.enrichContext(ctx, info, input)
	priorHistory := []provider.ConversationItem(nil)
	firstTurn := r.currentSession == nil || len(r.currentSession.History) == 0
	if r.currentSession != nil {
		priorHistory = append(priorHistory, r.currentSession.History...)
	}
	result, err := r.agent.RunStream(ctx, info, input, priorHistory, func(event agent.StreamEvent) error {
		if onEvent == nil {
			return nil
		}
		return onEvent(AgentStreamEvent{Kind: event.Kind, Title: event.Title, Content: event.Content})
	})
	if err != nil {
		return result, err
	}
	r.persistTurn(result)
	if firstTurn {
		go r.generateTitle(context.Background(), input, result.Assistant)
	}
	if err := r.maybeAutoCompact(ctx, onEvent); err != nil {
		return result, err
	}
	return result, nil
}

func (r *Runtime) enrichContext(ctx context.Context, info system.ContextInfo, input string) system.ContextInfo {
	if r.semantic == nil {
		return info
	}
	snapshot, err := r.semantic.Ensure(ctx)
	if err != nil {
		if info.Signals == nil {
			info.Signals = make(map[string]string)
		}
		info.Signals["semantic"] = err.Error()
		return info
	}
	info.SemanticSummary = snapshot.Summary()
	relevant := snapshot.RelevantFiles(input, 4)
	info.RelevantFiles = make([]string, 0, len(relevant))
	for _, file := range relevant {
		info.RelevantFiles = append(info.RelevantFiles, file.Descriptor())
	}
	snippets := snapshot.RelevantSnippets(input, 3, 180)
	info.RelevantSnippets = make([]string, 0, len(snippets))
	for _, snippet := range snippets {
		info.RelevantSnippets = append(info.RelevantSnippets, snippet.Descriptor())
	}
	if info.Signals == nil {
		info.Signals = make(map[string]string)
	}
	info.Signals["semantic"] = snapshot.Summary()
	for _, path := range r.mentionedFiles {
		for _, file := range snapshot.Files {
			if file.Path != path {
				continue
			}
			info.RelevantFiles = append([]string{file.Descriptor()}, info.RelevantFiles...)
			content := string(file.Content)
			lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
			limit := min(120, len(lines))
			var numbered []string
			for i := 0; i < limit; i++ {
				numbered = append(numbered, fmt.Sprintf("%d: %s", i+1, lines[i]))
			}
			info.RelevantSnippets = append([]string{fmt.Sprintf("FILE %s\nLINES 1-%d\nREASON explicit @ mention\n%s", file.Path, limit, strings.Join(numbered, "\n"))}, info.RelevantSnippets...)
			break
		}
	}
	tracelog.Logf("runtime context semantic=%q relevant_files=%d relevant_snippets=%d", info.SemanticSummary, len(info.RelevantFiles), len(info.RelevantSnippets))
	return info
}
