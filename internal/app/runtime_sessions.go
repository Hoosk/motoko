package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/brain"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/session"
)

func (r *Runtime) ListSessions() ([]*session.Session, error) {
	return session.List(r.workspaceID)
}

func (r *Runtime) LoadSession(id string) error {
	s, err := session.Load(r.workspaceID, id)
	if err != nil {
		return err
	}
	r.currentSession = s
	r.brain, r.brainInitErr = brain.New(r.workspaceID, s.ID)
	if r.brainInitErr != nil {
		return fmt.Errorf("failed to initialize session brain: %w", r.brainInitErr)
	}
	return nil
}

func (r *Runtime) CurrentSessionEntries() []Entry {
	if r.currentSession == nil || len(r.currentSession.History) == 0 {
		return nil
	}
	entries := make([]Entry, 0, len(r.currentSession.History))
	for _, msg := range r.currentSession.History {
		if _, ok := provider.ParseAssistantToolCallContent(msg.Content); ok {
			continue
		}
		switch msg.Role {
		case provider.RoleUser:
			entries = append(entries, Entry{Kind: EntryUser, Text: msg.Content})
		case provider.RoleAssistant:
			entries = append(entries, Entry{Kind: EntryAssistant, Text: msg.Content})
		case provider.RoleTool:
			_, output := provider.ParseToolResultContent(msg.Content)
			if strings.TrimSpace(output) != "" {
				entries = append(entries, Entry{Kind: EntrySystem, Text: output})
			}
		default:
			entries = append(entries, Entry{Kind: EntrySystem, Text: msg.Content})
		}
	}
	return entries
}

func (r *Runtime) CompactSession(ctx context.Context) Response {
	if err := r.doCompact(ctx); err != nil {
		return Response{Entries: []Entry{{Kind: EntryError, Text: err.Error()}}}
	}
	return Response{Entries: []Entry{{Kind: EntrySystem, Text: "Session compacted."}}}
}

func (r *Runtime) persistTurn(result agent.Result) {
	if r.currentSession == nil {
		workspacePath, _ := os.Getwd()
		r.currentSession = session.New(r.workspaceID, workspacePath)
	}
	r.currentSession.History = append([]provider.ConversationItem(nil), result.History...)
	r.currentSession.LastInputTokens = result.Usage.InputTokens

	r.currentSession.TotalInputTokens += result.Usage.InputTokens
	r.currentSession.TotalOutputTokens += result.Usage.OutputTokens
	r.currentSession.TotalTokens += result.Usage.TotalTokens
	r.currentSession.TotalReasoningTokens += result.Usage.ReasoningTokens
	r.currentSession.TotalCacheReadTokens += result.Usage.CacheReadInputTokens
	r.currentSession.TotalCacheWriteTokens += result.Usage.CacheWriteInputTokens

	_ = r.currentSession.Save()
}

func (r *Runtime) maybeAutoCompact(ctx context.Context, onEvent func(AgentStreamEvent) error) error {
	if r.currentSession == nil || r.contextWindow <= 0 || r.currentSession.LastInputTokens <= 0 {
		return nil
	}
	if float64(r.currentSession.LastInputTokens)/float64(r.contextWindow) < 0.80 {
		return nil
	}
	if onEvent != nil {
		_ = onEvent(AgentStreamEvent{Kind: "compacting", Content: "Compacting session..."})
	}
	err := r.doCompact(ctx)
	if err == nil && onEvent != nil {
		_ = onEvent(AgentStreamEvent{Kind: cmdStatus, Content: "Session auto-compacted."})
	}
	return err
}




