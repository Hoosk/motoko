package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/app/sessiontitle"
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
	r.brain, _ = brain.New(r.workspaceID, s.ID)
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
		case "user":
			entries = append(entries, Entry{Kind: EntryUser, Text: msg.Content})
		case "assistant":
			entries = append(entries, Entry{Kind: EntryAssistant, Text: msg.Content})
		case "tool":
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
	return Response{Entries: []Entry{{Kind: EntrySystem, Text: "Sesion compactada."}}}
}

func (r *Runtime) persistTurn(result agent.Result) {
	if r.currentSession == nil {
		workspacePath, _ := os.Getwd()
		r.currentSession = session.New(r.workspaceID, workspacePath)
	}
	r.currentSession.History = append([]provider.ConversationItem(nil), result.History...)
	r.currentSession.LastInputTokens = result.Usage.InputTokens
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
		_ = onEvent(AgentStreamEvent{Kind: "compacting", Content: "Compactando sesion..."})
	}
	err := r.doCompact(ctx)
	if err == nil && onEvent != nil {
		_ = onEvent(AgentStreamEvent{Kind: "status", Content: "Sesion compactada automaticamente."})
	}
	return err
}

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
	resp, err := client.Complete(ctx,
		"Resume la conversacion para continuarla despues. Devuelve un resumen concreto, con decisiones, estado actual y siguientes pasos.",
		r.currentSession.History,
		provider.ToolSet{},
	)
	if err != nil {
		return err
	}
	r.currentSession.CompactWith(strings.TrimSpace(resp.FinalText))
	if r.brain != nil {
		_ = r.brain.Write("summary.md", strings.TrimSpace(resp.FinalText))
	}
	return r.currentSession.Save()
}

func (r *Runtime) generateTitle(ctx context.Context, userInput, assistantResponse string) {
	if r.currentSession == nil {
		return
	}
	if strings.TrimSpace(r.currentSession.Title) != "" && !strings.EqualFold(strings.TrimSpace(r.currentSession.Title), "Nueva sesion") {
		return
	}
	active, ok := r.config.Active()
	if !ok {
		return
	}
	client, err := r.providerClient(active)
	if err != nil {
		return
	}
	resp, err := client.Complete(ctx,
		"Genera un titulo corto de 4 a 8 palabras para esta sesion. Responde exactamente con un objeto JSON de una linea con este formato: {\"message\":\"titulo\"}. No devuelvas markdown, comillas triples, explicaciones, opciones ni texto adicional.",
		[]provider.ConversationItem{provider.UserText(userInput), provider.AssistantText(assistantResponse)},
		provider.ToolSet{},
	)
	if err != nil {
		return
	}
	title := titleFromModelResponse(resp)
	if title == "" {
		return
	}
	r.currentSession.Title = title
	_ = r.currentSession.Save()
}

func titleFromModelResponse(resp provider.Response) string {
	return sessiontitle.FromModelResponse(resp)
}

func extractStructuredMessage(raw string) string {
	return sessiontitle.ExtractStructuredMessage(raw)
}

func sanitizeSessionTitle(raw string) string {
	return sessiontitle.Sanitize(raw)
}
