package ui

import (
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/system"
	tea "github.com/charmbracelet/bubbletea"
)

type FooterModel struct {
	sysInfo       system.ContextInfo
	tachikomaInfo map[string]string
	runtime       *app.Runtime
	width         int
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

func (m FooterModel) View() string {
	if m.width == 0 {
		return ""
	}
	ws := styles.WorkspaceStyle.Render("workspace: " + m.sysInfo.Workspace)
	git := styles.GitStyle.Render("git: " + m.sysInfo.GitSummary())
	pending := styles.SystemStyle.Render("pending: " + pendingLabel(m.runtime.PendingApproval()))
	provider := styles.SystemStyle.Render("provider: " + m.runtime.ProviderSummary())

	parts := []string{ws, git, pending, provider}
	return styles.FooterStyle.Width(m.width - 4).Render(strings.Join(parts, "  │  "))
}

func (m FooterModel) GetSysInfo() system.ContextInfo {
	return m.sysInfo
}
