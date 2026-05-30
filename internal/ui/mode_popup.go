package ui

import (
	"strings"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
)

type modePopupState struct {
	active bool
	index  int
	agents []agent.AgentDef
}

func (p *modePopupState) Open(runtime *app.Runtime) {
	p.agents = runtime.AvailableAgents()
	current := runtime.AgentName()
	p.index = 0
	for i, a := range p.agents {
		if strings.EqualFold(a.Name, current) {
			p.index = i
			break
		}
	}
	if p.index < 0 || p.index >= len(p.agents) {
		p.index = 0
	}
	p.active = true
}

func (p *modePopupState) Update(msg tea.Msg, runtime *app.Runtime) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if !p.active {
			return nil
		}
		switch msg.String() {
		case "esc":
			p.active = false
			return nil
		case "up", "ctrl+p":
			if len(p.agents) == 0 {
				p.index = 0
				return nil
			}
			p.index--
			if p.index < 0 {
				p.index = len(p.agents) - 1
			}
			return nil
		case "down", "ctrl+n", "tab":
			if len(p.agents) == 0 {
				p.index = 0
				return nil
			}
			p.index = (p.index + 1) % len(p.agents)
			return nil
		case "enter":
			if len(p.agents) == 0 {
				p.active = false
				return nil
			}
			if p.index < 0 || p.index >= len(p.agents) {
				p.index = 0
			}
			chosen := p.agents[p.index]
			p.active = false
			runtime.SetAgentMode(chosen.Name)
			return func() tea.Msg { return AgentChangedMsg{Name: chosen.Name, Agent: chosen.Name} }
		}
	}
	return nil
}

func (p modePopupState) View() string {
	if !p.active {
		return ""
	}
	rows := []string{
		styles.PopupTitleStyle.Render("Agent Mode"),
		styles.PopupMutedStyle.Render("↑↓ navigate  Enter select  Esc cancel"),
		"",
	}
	idx := p.index
	if idx < 0 || idx >= len(p.agents) {
		idx = 0
	}
	for i, a := range p.agents {
		label := styles.PopupFieldLabelStyle.Render(a.Name)
		var desc string
		if a.System != "" {
			desc = styles.PopupMutedStyle.Render(truncateAgentDesc(a.System, 60))
		}
		line := label
		if desc != "" {
			line += "  " + desc
		}
		if i == idx {
			rows = append(rows, styles.PopupSelectionStyle.Render(line))
		} else {
			rows = append(rows, line)
		}
	}
	return strings.Join(rows, "\n")
}

func truncateAgentDesc(s string, maxLen int) string {
	// Use first line only.
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	if len([]rune(s)) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen-1]) + "…"
}
