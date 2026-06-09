package ui

import (
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/system"
	tea "github.com/charmbracelet/bubbletea"
)

type FooterModel struct {
	runtime       *app.Runtime
	tachikomaInfo map[string]string
	sysInfo       system.ContextInfo
	width         int
	contextWindow int
	contextTokens int
	thinkingFrame int
	taskCount     int
	thinking      bool
}

func NewFooterModel(runtime *app.Runtime) FooterModel {
	return FooterModel{
		runtime: runtime,
		sysInfo: system.GetContextInfo(),
	}
}

func (m FooterModel) Init() tea.Cmd {
	return nil
}

func (m FooterModel) Update(msg tea.Msg) (FooterModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width

	case TachikomaStatusMsg:
		m.tachikomaInfo = msg.Statuses

	case ContextInfoMsg:
		m.sysInfo = msg.Info

	case ContextTokensMsg:
		m.contextTokens = msg.Tokens
		m.contextWindow = msg.Window

	case ThinkingTickMsg:
		if m.thinking {
			m.thinkingFrame = (m.thinkingFrame + 1) % len(thinkingFrames)
		}
	case TaskEventMsg:
		m.taskCount = m.runtime.ActiveTasks()
	}

	return m, nil
}

func (m FooterModel) View() string {
	if m.width == 0 {
		return ""
	}

	ws := styles.BoldVioletStyle.Render("⬡ " + m.sysInfo.Workspace)
	agent := styles.VioletStyle.Render("● " + m.runtime.AgentName())

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
		spinner := styles.BoldNeonStyle.Render(thinkingFrames[m.thinkingFrame])
		label := styles.BlueStyle.Render(agentActivityLabel(m.runtime.AgentName()))
		parts = append(parts, spinner+" "+label)
	}

	if pending := m.runtime.PendingApproval(); pending != "" {
		parts = append(parts, styles.ErrorStyle.Render("⚠ "+pending))
	}

	if m.taskCount > 0 {
		parts = append(parts, styles.BoldBlueStyle.Render(fmt.Sprintf("tasks:%d", m.taskCount)))
	}

	footerWidth := m.width - 2
	if footerWidth < 0 {
		footerWidth = 0
	}
	return styles.FooterStyle.Width(footerWidth).Render(strings.Join(parts, "  "))
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

func (m FooterModel) GetSysInfo() system.ContextInfo {
	return m.sysInfo
}
