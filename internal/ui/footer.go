package ui

import (
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/system"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type FooterModel struct {
	sysInfo       system.ContextInfo
	tachikomaInfo map[string]string
	runtime       *app.Runtime
	width         int
	thinking      bool
	thinkingFrame int
	contextTokens int
	contextWindow int
}

func NewFooterModel(runtime *app.Runtime) FooterModel {
	return FooterModel{
		sysInfo:       system.GetContextInfo(),
		tachikomaInfo: make(map[string]string),
		runtime:       runtime,
	}
}

func (m FooterModel) Init() tea.Cmd {
	return nil
}

func (m *FooterModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width

	case ThinkingTickMsg:
		if m.thinking {
			m.thinkingFrame = (m.thinkingFrame + 1) % len(thinkingFrames)
		}

	case TachikomaMsg:
		m.tachikomaInfo[msg.Name] = msg.Status
		if m.sysInfo.Signals == nil {
			m.sysInfo.Signals = make(map[string]string)
		}
		m.sysInfo.Signals[msg.Name] = msg.Status

		signals := m.sysInfo.Signals
		m.sysInfo = system.GetContextInfo()
		m.sysInfo.Signals = signals

		if m.sysInfo.Signals == nil {
			m.sysInfo.Signals = make(map[string]string)
		}
		for name, status := range m.tachikomaInfo {
			m.sysInfo.Signals[name] = status
		}

	case ShellResultMsg, ResponseAppliedMsg:
		signals := m.sysInfo.Signals
		m.sysInfo = system.GetContextInfo()
		if len(signals) > 0 {
			m.sysInfo.Signals = signals
		}
	}
	return nil
}

func (m *FooterModel) SetThinking(thinking bool) {
	m.thinking = thinking
	if thinking {
		m.thinkingFrame = 0
	}
}

func (m *FooterModel) SetContextStats(tokens, window int) {
	m.contextTokens = tokens
	m.contextWindow = window
}

func (m FooterModel) View() string {
	if m.width == 0 {
		return ""
	}

	ws := lipgloss.NewStyle().Foreground(styles.AccentViolet).Bold(true).Render("⬡ " + m.sysInfo.Workspace)
	agent := lipgloss.NewStyle().Foreground(styles.AccentViolet).Render("● " + m.runtime.AgentName())

	parts := []string{ws}
	if m.sysInfo.HasGit && m.sysInfo.GitBranch != "" {
		parts = append(parts, styles.GitStyle.Render("⎇ "+m.sysInfo.GitBranch))
	}
	parts = append(parts, agent)
	parts = append(parts, styles.SystemStyle.Render(m.runtime.ProviderSummary()))
	if m.contextWindow > 0 {
		parts = append(parts, styles.SystemStyle.Render(fmt.Sprintf("%dk/%dk", m.contextTokens/1000, m.contextWindow/1000)))
	} else if m.contextTokens > 0 {
		parts = append(parts, styles.SystemStyle.Render(fmt.Sprintf("%dk", m.contextTokens/1000)))
	}
	if title := strings.TrimSpace(m.runtime.SessionTitle()); title != "" {
		parts = append(parts, styles.SystemStyle.Render("» "+title))
	}

	if m.thinking {
		spinner := lipgloss.NewStyle().Foreground(styles.MainNeon).Bold(true).Render(thinkingFrames[m.thinkingFrame])
		label := lipgloss.NewStyle().Foreground(styles.AccentBlue).Render(agentActivityLabel(m.runtime.AgentName()))
		parts = append(parts, spinner+" "+label)
	}

	if pending := m.runtime.PendingApproval(); pending != "" {
		parts = append(parts, styles.ErrorStyle.Render("⚠ "+pending))
	}

	return styles.FooterStyle.Width(m.width - 4).Render(strings.Join(parts, "  "))
}

func (m FooterModel) GetSysInfo() system.ContextInfo {
	return m.sysInfo
}

func agentActivityLabel(agentName string) string {
	if strings.EqualFold(agentName, "compact") {
		return "compacting"
	}
	switch agentName {
	case "build":
		return "building"
	case "plan":
		return "planning"
	default:
		return "thinking"
	}
}
