package ui

import (
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) onProviderModels(msg ProviderModelsMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	if msg.Err != nil {
		m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: msg.Err.Error()})
		m.timeline.renderMessages()
	} else if len(msg.Models) == 0 {
		m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: "The provider returned no available models."})
		m.timeline.renderMessages()
	} else if active, ok := m.runtime.GetActiveProviderConfig(); ok && strings.TrimSpace(active.Model) == "" {
		// Provider has no model selected yet: auto-pick the first one so
		// the agent becomes immediately usable without requiring a manual
		// /models use invocation.
		first := msg.Models[0]
		cmds = append(cmds, selectModelAndBudget(m.runtime, first, active.ThinkingBudget))
	} else {
		m.modelPicker.Open(msg.Models)
	}
	return cmds, false
}

func (m *Model) onModelSelected(msg ModelSelectedMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	if msg.Model.SupportsThinking {
		currentBudget := 0
		if active, ok := m.runtime.GetActiveProviderConfig(); ok {
			currentBudget = active.ThinkingBudget
		}
		m.thinkingPicker.Open(msg.Model, currentBudget)
	} else {
		cmds = append(cmds, selectModelAndBudget(m.runtime, msg.Model, 0))
	}
	return cmds, false
}

func (m *Model) onThinkingBudgetSelected(msg ThinkingBudgetSelectedMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	cmds = append(cmds, selectModelAndBudget(m.runtime, msg.Model, msg.Budget))
	return cmds, false
}

func (m *Model) onQuestionAsked(msg QuestionAskedMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	if msg.Pending != nil {
		m.questionPopup.Open(msg.Pending)
	}
	return cmds, false
}

func (m *Model) onSessions(msg SessionsMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	cmds = append(cmds, m.sessionPicker.Update(msg, m.runtime))
	return cmds, false
}

func (m *Model) onSessionLoaded(msg SessionLoadedMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	if msg.Err != nil {
		m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: msg.Err.Error()})
	} else {
		m.timeline.SetOnboarding(timelineOnboarding(m.runtime))
		m.timeline.resetMessages()
		for _, entry := range m.runtime.CurrentSessionEntries() {
			m.timeline.appendEntry(entry)
		}
		m.timeline.renderMessages()
		cmds = append(cmds, m.showNotification("Session loaded"))
	}
	return cmds, false
}

func (m *Model) onCompactResult(msg CompactResultMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	if msg.Err != nil {
		m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: msg.Err.Error()})
	} else {
		m.timeline.resetMessages()
		for _, entry := range msg.Response.Entries {
			m.timeline.appendEntry(entry)
		}
		m.timeline.renderMessages()
		cmds = append(cmds, m.showNotification("Session compacted"))
	}
	return cmds, false
}

func (m *Model) onAgentChanged(msg AgentChangedMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	cmds = append(cmds, m.showNotification("Agent switched to "+msg.Name))
	return cmds, false
}

func (m *Model) onModelChanged(msg ModelChangedMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	cmds = append(cmds, m.showNotification("Model switched to "+msg.Model))
	return cmds, false
}
