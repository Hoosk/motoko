package ui

import (
	"strings"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) openModePicker() {
	m.agentList = m.runtime.AvailableAgents()
	current := m.runtime.AgentName()
	m.agentListIndex = 0
	for i, a := range m.agentList {
		if strings.EqualFold(a.Name, current) {
			m.agentListIndex = i
			break
		}
	}
	m.modePickerOpen = true
}

func (m *Model) handleModePickerKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.modePickerOpen = false
		return nil
	case "up", "ctrl+p":
		m.agentListIndex--
		if m.agentListIndex < 0 {
			m.agentListIndex = len(m.agentList) - 1
		}
		return nil
	case "down", "ctrl+n", "tab":
		if len(m.agentList) > 0 {
			m.agentListIndex = (m.agentListIndex + 1) % len(m.agentList)
		}
		return nil
	case "enter":
		if len(m.agentList) == 0 {
			m.modePickerOpen = false
			return nil
		}
		chosen := m.agentList[m.agentListIndex]
		m.modePickerOpen = false
		m.runtime.SetAgentMode(chosen.Name)
		return func() tea.Msg { return AgentChangedMsg{Agent: chosen.Name} }
	}
	return nil
}

func (m Model) renderModePicker() string {
	rows := []string{
		styles.PopupTitleStyle.Render("Agent Mode"),
		styles.PopupMutedStyle.Render("↑↓ navigate  Enter select  Esc cancel"),
		"",
	}
	for i, a := range m.agentList {
		label := styles.PopupFieldLabelStyle.Render(a.Name)
		var desc string
		if a.System != "" {
			desc = styles.PopupMutedStyle.Render(truncateAgentDesc(a.System, 60))
		}
		line := label
		if desc != "" {
			line += "  " + desc
		}
		if i == m.agentListIndex {
			rows = append(rows, styles.PopupSelectionStyle.Render(line))
		} else {
			rows = append(rows, line)
		}
	}
	return strings.Join(rows, "\n")
}

func truncateAgentDesc(s string, max int) string {
	// Use first line only.
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	if len([]rune(s)) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max-1]) + "…"
}

// agentDefsEqual checks if two slices of AgentDef are equal by name.
func agentDefsEqual(a, b []agent.AgentDef) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name {
			return false
		}
	}
	return true
}
