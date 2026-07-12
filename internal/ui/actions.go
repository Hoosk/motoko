package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/app/shell"
	"github.com/Hoosk/motoko/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) runAgent(ctx context.Context, prompt string, requestID int, ch chan app.AgentStreamEvent) tea.Cmd {
	return func() tea.Msg {
		defer close(ch)
		info := m.runtime.GetContextInfo()
		res, err := m.runtime.RunAgentStream(ctx, info, prompt, func(ev app.AgentStreamEvent) error {
			select {
			case ch <- ev:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		return AgentResultMsg{RequestID: requestID, Prompt: prompt, Result: res, Assistant: res.Assistant, Err: err}
	}
}

func (m *Model) listModels() tea.Cmd {
	return func() tea.Msg {
		cfg, ok := m.runtime.GetActiveProviderConfig()
		if !ok {
			return ErrorMsg{Err: fmt.Errorf("no active provider")}
		}
		models, err := m.runtime.ListModelsForProvider(context.Background(), cfg)
		return ProviderModelsMsg{Models: models, Err: err}
	}
}

func (m *Model) listSessions() tea.Cmd {
	return func() tea.Msg {
		sessions, err := m.runtime.ListSessions()
		return SessionsMsg{Sessions: sessions, Err: err}
	}
}

func (m *Model) loadSession(id string) tea.Cmd {
	return func() tea.Msg {
		if err := m.runtime.LoadSession(id); err != nil {
			return SessionLoadedMsg{Err: err}
		}
		return SessionLoadedMsg{}
	}
}

func (m *Model) updateContextStats() tea.Cmd {
	return func() tea.Msg {
		tokens := m.runtime.HistoryInputTokens()
		window := m.runtime.ContextWindow()
		return ContextTokensMsg{Tokens: tokens, Window: window}
	}
}

func (m Model) hideNotification() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return hideNotificationMsg{}
	})
}

func (m Model) thinkingTick() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return ThinkingTickMsg{}
	})
}

func (m Model) waitAgentStream(ch chan app.AgentStreamEvent, requestID int) tea.Cmd {
	return func() tea.Msg {
		var events []app.AgentStreamEvent
		// Read at most 10 events to avoid blocking too long
		for range 10 {
			select {
			case ev, ok := <-ch:
				if !ok {
					return AgentStreamBatchMsg{RequestID: requestID, Events: events, Done: true}
				}
				events = append(events, ev)
			default:
				if len(events) > 0 {
					return AgentStreamBatchMsg{RequestID: requestID, Events: events, Done: false}
				}
				select {
				case ev, ok := <-ch:
					if !ok {
						return AgentStreamBatchMsg{RequestID: requestID, Events: events, Done: true}
					}
					events = append(events, ev)
				case <-time.After(10 * time.Millisecond):
					return AgentStreamBatchMsg{RequestID: requestID, Events: events, Done: false}
				}
			}
		}
		return AgentStreamBatchMsg{RequestID: requestID, Events: events, Done: false}
	}
}

func loadProviderModels(runtime *app.Runtime, cfg config.ProviderConfig) tea.Cmd {
	return func() tea.Msg {
		models, err := runtime.ListModelsForProvider(context.Background(), cfg)
		return ProviderModelsMsg{Models: models, Err: err}
	}
}

func (m *Model) runShell(command string) tea.Cmd {
	return func() tea.Msg {
		res := shell.RunCommand(context.Background(), command)
		return ShellResultMsg{Result: res}
	}
}

func (m *Model) runTask(command string) tea.Cmd {
	return func() tea.Msg {
		_, err := m.runtime.StartTask(context.Background(), command)
		if err != nil {
			return ErrorMsg{Err: err}
		}
		return NotificationMsg{Text: "Task launched in background"}
	}
}

func (m *Model) compactSession() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		resp := m.runtime.CompactSession(ctx)
		var err error
		if len(resp.Entries) > 0 && resp.Entries[0].Kind == app.EntryError {
			err = fmt.Errorf("%s", resp.Entries[0].Text)
		}
		return CompactResultMsg{Response: resp, Err: err}
	}
}

func (m Model) waitQuestion() tea.Cmd {
	return func() tea.Msg {
		pending, err := m.runtime.QuestionBroker().Next(m.runtime.BackgroundContext())
		if err != nil || pending == nil {
			return nil
		}
		return QuestionAskedMsg{Pending: pending}
	}
}

func (m Model) waitScheduleEvent() tea.Cmd {
	return func() tea.Msg {
		res := m.runtime.NextScheduleEvent(m.runtime.BackgroundContext())
		if !res.OK {
			return nil
		}
		return ScheduleEventMsg{Event: res.Event}
	}
}

func (m Model) waitTaskEvent() tea.Cmd {
	return func() tea.Msg {
		res := m.runtime.NextTaskEvent(m.runtime.BackgroundContext())
		if !res.OK {
			return nil
		}
		return TaskEventMsg{Event: res.Event}
	}
}

func (m Model) checkForUpdatesCmd() tea.Cmd {
	return func() tea.Msg {
		info, err := m.runtime.WaitForUpdate()
		if err != nil {
			return nil
		}
		if info != nil && info.IsNewer {
			return UpdateAvailableMsg{Info: info}
		}
		return nil
	}
}
