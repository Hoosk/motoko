package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderComposerToolbar(width int) string {
	agentName := m.runtime.AgentName()
	var modeIndicator string
	switch agentName {
	case modePlan:
		modeIndicator = styles.BoldVioletStyle.Render("[plan]")
	case "build":
		modeIndicator = styles.BoldNeonStyle.Render("[build]")
	default:
		modeIndicator = styles.WarmGoldStyle.Render("[" + agentName + "]")
	}

	var statusStr string
	if m.timeline.model.Thinking || m.footer.thinking {
		frame := thinkingFrames[m.footer.thinkingFrame]
		statusStr = styles.BoldNeonStyle.Render(frame) + " " + styles.BlueStyle.Render(agentActivityLabel(agentName)+"...")
	} else {
		statusStr = styles.GrayStyle.Render("idle")
	}

	left := " " + modeIndicator + "  " + statusStr
	if queued := len(m.promptQueue); queued > 0 {
		left += "  " + styles.WarmGoldStyle.Render("queued ") + styles.WhiteStyle.Render(strconv.Itoa(queued))
	}

	var subagentsStr string
	activeSubagents := m.runtime.ActiveSubagents()
	if len(activeSubagents) > 0 {
		subagentsStr = styles.BoldBlueStyle.Render("subagents ") + styles.WhiteStyle.Render(strings.Join(activeSubagents, ", "))
	}

	helpHint := styles.GrayStyle.Render("Ctrl+K palette • Ctrl+H help • Ctrl+Q queue • Ctrl+R reasoning")

	leftContent := left
	if subagentsStr != "" {
		leftContent += "  " + subagentsStr
	}

	leftLen := lipgloss.Width(leftContent)
	rightLen := lipgloss.Width(helpHint)
	paddingLen := max(
		// Account for right margin
		width-leftLen-rightLen-2, 0)

	toolbarContent := leftContent + strings.Repeat(" ", paddingLen) + helpHint
	return styles.SystemStyle.Width(width).Render(toolbarContent)
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	sidebarWidth := m.sidebar.width
	mainWidth := m.width - sidebarWidth

	composerView := m.composer.View()
	toolbarView := m.renderComposerToolbar(mainWidth)
	queueView := m.renderQueuePanel(mainWidth)
	timelineView := m.timeline.View()
	footerView := m.footer.View()

	var mainView string
	barView := m.approvalBar.View(mainWidth)
	if sidebarWidth > 0 {
		blocks := []string{timelineView, toolbarView}
		if barView != "" {
			blocks = append(blocks, barView)
		}
		if queueView != "" {
			blocks = append(blocks, queueView)
		}
		blocks = append(blocks, composerView)
		mainContent := lipgloss.JoinVertical(lipgloss.Left, blocks...)
		sidebarView := m.sidebar.View()
		mainView = lipgloss.JoinHorizontal(lipgloss.Top, mainContent, sidebarView)
	} else {
		blocks := []string{timelineView, toolbarView}
		if barView != "" {
			blocks = append(blocks, barView)
		}
		if queueView != "" {
			blocks = append(blocks, queueView)
		}
		blocks = append(blocks, composerView)
		mainView = lipgloss.JoinVertical(lipgloss.Left, blocks...)
	}

	base := lipgloss.JoinVertical(lipgloss.Left, mainView, footerView)

	// Dynamic popup width: adapt to terminal, capped at 50
	popupWidth := min(m.width-10, 50)
	if popupWidth < 30 {
		popupWidth = 30
	}
	popupStyle := styles.PopupStyle.Width(popupWidth)

	// Dynamic wide popup width: adapt to terminal, capped at 76
	widePopupWidth := min(m.width-10, 76)
	if widePopupWidth < 40 {
		widePopupWidth = 40
	}
	widePopupStyle := styles.PopupStyle.Width(widePopupWidth)

	if m.questionPopup.active {
		popup := widePopupStyle.Render(m.questionPopup.View())
		base = overlayCenter(base, popup, m.width, m.height)
	} else if m.providerForm.active {
		popup := popupStyle.Render(m.providerForm.View(m.runtime))
		base = overlayCenter(base, popup, m.width, m.height)
	} else if m.mcpForm.active {
		popup := popupStyle.Render(m.mcpForm.View(m.runtime))
		base = overlayCenter(base, popup, m.width, m.height)
	} else if m.modelPicker.active {
		popup := popupStyle.Render(m.modelPicker.View())
		base = overlayCenter(base, popup, m.width, m.height)
	} else if m.thinkingPicker.active {
		popup := popupStyle.Render(m.thinkingPicker.View())
		base = overlayCenter(base, popup, m.width, m.height)
	} else if m.sessionPicker.active {
		popup := popupStyle.Render(m.sessionPicker.View())
		base = overlayCenter(base, popup, m.width, m.height)
	} else if m.modePopup.active {
		popup := widePopupStyle.Render(m.modePopup.View())
		base = overlayCenter(base, popup, m.width, m.height)
	} else if m.commandPalette.active {
		popup := widePopupStyle.Render(m.commandPalette.View())
		base = overlayCenter(base, popup, m.width, m.height)
	} else if m.showTools {
		popup := widePopupStyle.Render(renderToolPalette(m.runtime.ToolSpecs()))
		base = overlayCenter(base, popup, m.width, m.height)
	} else if m.helpOverlay.active {
		popup := widePopupStyle.Render(m.helpOverlay.View(m.runtime))
		base = overlayCenter(base, popup, m.width, m.height)
	} else if m.settingsPopup.active {
		popup := widePopupStyle.Render(m.settingsPopup.View(m.runtime))
		base = overlayCenter(base, popup, m.width, m.height)
	}

	if m.notificationShow {
		toast := styles.PopupStyle.
			Padding(0, 1).
			Width(30).
			BorderForeground(styles.MainNeon).
			Render(styles.BoldNeonStyle.Render("✓ ") +
				styles.WhiteStyle.Render(m.notificationText))
		base = overlayBase(base, toast, m.width)
	}

	lines := strings.Split(base, "\n")
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	return strings.Join(lines, "\n")
}

func (m *Model) SyncLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}

	sidebarWidth, sidebarAllowed := m.sidebarLayout()
	if !sidebarAllowed {
		m.showSidebar = false
	} else {
		switch m.sidebarPref {
		case sidebarForceShow:
			m.showSidebar = true
		case sidebarForceHide:
			m.showSidebar = false
		default: // sidebarDefault
			m.showSidebar = m.sidebarPreferredByWidth()
		}
	}

	if !m.showSidebar {
		sidebarWidth = 0
	}
	mainWidth := m.width - sidebarWidth

	m.composer.SetWidth(mainWidth)
	composerHeight := m.composer.Height()

	footerHeight := 1
	m.footer.width = m.width

	toolbarHeight := 1
	barHeight := m.approvalBarHeight(mainWidth)
	queueHeight := m.queuePanelHeight(mainWidth)

	timelineHeight := max(m.height-footerHeight-composerHeight-toolbarHeight-queueHeight-barHeight, 4)

	m.timeline.SyncLayout(mainWidth, timelineHeight)
	m.sidebar.SetDimensions(sidebarWidth, timelineHeight+toolbarHeight+queueHeight+barHeight+composerHeight)
}

func timelineOnboarding(runtime *app.Runtime) []string {
	provider := runtime.ProviderSummary()
	mode := runtime.AgentName()
	workspace := runtime.GetContextInfo().Workspace
	if workspace == "" {
		workspace = "workspace unavailable"
	}

	return []string{
		styles.SystemStyle.Render("Inspect code, edit files, run tools, or ask for a focused review."),
		styles.GrayStyle.Render("Workspace: " + workspace),
		styles.GrayStyle.Render("Mode: " + mode + "  •  Provider: " + provider),
		styles.GrayStyle.Render("Shortcuts: Ctrl+K palette  •  Ctrl+H help  •  Ctrl+M models  •  Ctrl+P provider"),
		styles.GrayStyle.Render("Try: /help  /models list  /sessions  /provider add  @README.md explain the entry point"),
	}
}

func (m Model) paletteContext() paletteContext {
	ctx := paletteContext{
		Info:        m.runtime.GetContextInfo(),
		Providers:   m.runtime.ConfiguredProviders(),
		Skills:      m.runtime.AvailableSkills(),
		Tasks:       m.runtime.ListTasks(),
		Agents:      m.runtime.AvailableAgents(),
		Thinking:    m.timeline.model.Thinking,
		QueueLen:    len(m.promptQueue),
		ShowSidebar: m.showSidebar,
		Brain:       m.runtime.GetBrain(),
	}
	ctx.ActiveProvider, ctx.HasActiveProvider = m.runtime.GetActiveProviderConfig()
	if sessions, err := m.runtime.ListSessions(); err == nil {
		ctx.Sessions = sessions
	}
	return ctx
}

func (m *Model) showNotification(text string) tea.Cmd {
	m.notificationShow = true
	m.notificationText = text
	m.notificationTime = time.Now()
	return m.hideNotification()
}

func (m *Model) toggleSidebar() {
	if m.showSidebar {
		m.sidebarPref = sidebarForceHide
		m.showSidebar = false
	} else {
		m.sidebarPref = sidebarForceShow
		m.showSidebar = true
	}
	m.SyncLayout()
}
