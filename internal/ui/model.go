package ui

import (
	"strings"
	"sync"
	"time"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type TachikomaMsg struct {
	Statuses map[string]string
}

type ShellResultMsg struct {
	Result app.ShellResult
}

type AgentResultMsg struct {
	Prompt    string
	Result    agent.Result
	Assistant string
	Err       error
}

type agentStreamBuffer struct {
	mu     sync.Mutex
	events []app.AgentStreamEvent
	done   bool
}

type Model struct {
	runtime          *app.Runtime
	timeline         TimelineModel
	composer         ComposerModel
	footer           FooterModel
	sidebar          SidebarModel
	width            int
	height           int
	notificationShow bool
	notificationText string
	notificationTime time.Time
	providerForm     providerForm
	modelPicker      modelPickerState
	sessionPicker    sessionPickerState
	agentStream      chan app.AgentStreamEvent
	agentBuffer      *agentStreamBuffer
	modePopup        modePopupState
	showHelp         bool
	showTools        bool
}

func NewModel(runtime *app.Runtime) Model {
	m := Model{
		runtime:  runtime,
		timeline: NewTimelineModel(),
		composer: NewComposerModel(runtime),
		footer:   NewFooterModel(runtime),
		sidebar:  NewSidebarModel(runtime),
	}

	// Load startup entries (e.g. resumed session history)
	for _, entry := range runtime.StartupEntries() {
		m.timeline.appendEntry(entry)
	}
	m.timeline.renderMessages()

	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.timeline.Init(),
		m.composer.Init(),
		m.footer.Init(),
		m.sidebar.Init(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// 1. Handle Priority Key Commands (Global)
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		}
	}

	// 2. Delegate to Active Popups (Modal state)
	if m.providerForm.active {
		cmds = append(cmds, m.providerForm.Update(msg, m.runtime))
		return m, tea.Batch(cmds...)
	}
	if m.modelPicker.active {
		cmds = append(cmds, m.modelPicker.Update(msg, m.runtime))
		return m, tea.Batch(cmds...)
	}
	if m.sessionPicker.active {
		cmds = append(cmds, m.sessionPicker.Update(msg, m.runtime))
		return m, tea.Batch(cmds...)
	}
	if m.modePopup.active {
		cmds = append(cmds, m.modePopup.Update(msg, m.runtime))
		return m, tea.Batch(cmds...)
	}

	// 3. Global Message Handling
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.SyncLayout()

	case CopySelectionMsg:
		if msg.Err == nil {
			m.notificationShow = true
			m.notificationText = "Copied to clipboard"
			m.notificationTime = time.Now()
			cmds = append(cmds, m.hideNotification())
		} else {
			m.notificationShow = true
			m.notificationText = "Copy failed: " + msg.Err.Error()
			m.notificationTime = time.Now()
			cmds = append(cmds, m.hideNotification())
		}

	case NotificationMsg:
		m.notificationShow = true
		m.notificationText = msg.Text
		m.notificationTime = time.Now()
		cmds = append(cmds, m.hideNotification())

	case hideNotificationMsg:
		if time.Since(m.notificationTime) >= 3*time.Second {
			m.notificationShow = false
		}

	case ErrorMsg:
		m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: msg.Err.Error()})
		m.timeline.renderMessages()

	case TachikomaStatusMsg:
		m.footer.tachikomaInfo = msg.Statuses
		m.sidebar.Update(msg)

	case SubmitPromptMsg:
		if strings.TrimSpace(msg.Prompt) == "" {
			break
		}
		resp := m.runtime.HandleInput(msg.Prompt, m.runtime.GetContextInfo())

		if resp.Clear {
			m.timeline.resetMessages()
		}

		for _, entry := range resp.Entries {
			m.timeline.appendEntry(entry)
		}
		m.timeline.renderMessages()

		if resp.Signal != "" {
			switch resp.Signal {
			case "open-provider-popup":
				m.providerForm.Open(m.runtime)
			case "open-models-popup":
				cmds = append(cmds, m.listModels())
			case "open-sessions-popup":
				m.sessionPicker.Open()
				cmds = append(cmds, m.listSessions())
			case "open-mode-popup":
				m.modePopup.Open(m.runtime)
			}
		}

		if resp.Action != nil {
			switch resp.Action.Type {
			case app.ActionAgent:
				m.timeline.SetStreaming(true)
				m.timeline.SetThinking(true)
				m.footer.SetThinking(true)
				m.composer.SetThinking(true)
				m.agentStream = make(chan app.AgentStreamEvent, 100)
				m.agentBuffer = &agentStreamBuffer{}
				cmds = append(cmds, m.runAgent(resp.Action.AgentPrompt), m.waitAgentStream(m.agentStream), m.thinkingTick())

			case app.ActionShell:
				cmds = append(cmds, m.runShell(resp.Action.ShellCommand))

			case app.ActionCompact:
				cmds = append(cmds, m.compactSession())
			}
		}

	case AgentStreamBatchMsg:
		for _, event := range msg.Events {
			m.timeline.Update(AgentStreamEventMsg{Event: event})
		}
		if msg.Done && m.agentBuffer != nil {
			m.agentBuffer.mu.Lock()
			m.agentBuffer.done = true
			m.agentBuffer.mu.Unlock()
		}
		if !msg.Done && m.agentStream != nil {
			cmds = append(cmds, m.waitAgentStream(m.agentStream))
		} else if msg.Done {
			m.agentStream = nil
		}

	case ThinkingTickMsg:
		if m.timeline.model.Thinking || m.footer.thinking {
			cmds = append(cmds, m.thinkingTick())
		}
		m.timeline.Update(msg)
		m.footer.Update(msg)

	case AgentResultMsg:
		if strings.TrimSpace(msg.Assistant) != "" {
			if cmd := m.timeline.Update(finalizeStreamMsg{Text: msg.Assistant}); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		m.timeline.SetThinking(false)
		m.footer.SetThinking(false)
		m.composer.SetThinking(false)
		if msg.Err != nil {
			m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: msg.Err.Error()})
			m.timeline.renderMessages()
		}
		cmds = append(cmds, m.updateContextStats())

	case ShellResultMsg:
		m.timeline.appendEntry(app.Entry{Kind: app.EntryCommand, Text: msg.Result.Command})
		if msg.Result.Output != "" {
			kind := app.EntryOutput
			if msg.Result.ExitCode != 0 {
				kind = app.EntryError
			}
			m.timeline.appendEntry(app.Entry{Kind: kind, Text: msg.Result.Output})
		}
		m.timeline.renderMessages()

	case ProviderModelsMsg:
		if msg.Err != nil {
			m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: msg.Err.Error()})
			m.timeline.renderMessages()
		} else if len(msg.Models) == 0 {
			m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: "El provider no devolvio modelos disponibles."})
			m.timeline.renderMessages()
		} else {
			m.modelPicker.Open(msg.Models)
		}

	case SessionsMsg:
		cmds = append(cmds, m.sessionPicker.Update(msg, m.runtime))

	case SessionLoadedMsg:
		if msg.Err != nil {
			m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: msg.Err.Error()})
		} else {
			m.timeline.resetMessages()
			for _, entry := range m.runtime.CurrentSessionEntries() {
				m.timeline.appendEntry(entry)
			}
			m.timeline.renderMessages()
			m.notificationShow = true
			m.notificationText = "Session loaded"
			m.notificationTime = time.Now()
			cmds = append(cmds, m.hideNotification())
		}

	case CompactResultMsg:
		if msg.Err != nil {
			m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: msg.Err.Error()})
		} else {
			m.timeline.resetMessages()
			for _, entry := range msg.Response.Entries {
				m.timeline.appendEntry(entry)
			}
			m.timeline.renderMessages()
			m.notificationShow = true
			m.notificationText = "Session compacted"
			m.notificationTime = time.Now()
			cmds = append(cmds, m.hideNotification())
		}

	case AgentChangedMsg:
		m.notificationShow = true
		m.notificationText = "Agent switched to " + msg.Name
		m.notificationTime = time.Now()
		cmds = append(cmds, m.hideNotification())

	case ModelChangedMsg:
		m.notificationShow = true
		m.notificationText = "Model switched to " + msg.Model
		m.notificationTime = time.Now()
		cmds = append(cmds, m.hideNotification())

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+p":
			m.providerForm.Open(m.runtime)
		case "ctrl+m":
			cmds = append(cmds, m.listModels())
		case "ctrl+s":
			m.sessionPicker.Open()
			cmds = append(cmds, m.listSessions())
		case "ctrl+a":
			m.modePopup.Open(m.runtime)
		case "ctrl+t":
			m.showTools = !m.showTools
		case "ctrl+h":
			m.showHelp = !m.showHelp
		case "ctrl+r":
			m.timeline.model.ShowReasoning = !m.timeline.model.ShowReasoning
			m.timeline.renderMessages()
			stateStr := "hidden"
			if m.timeline.model.ShowReasoning {
				stateStr = "visible"
			}
			m.notificationShow = true
			m.notificationText = "Reasoning is now " + stateStr
			m.notificationTime = time.Now()
			cmds = append(cmds, m.hideNotification())
		}
	}
	// 4. Delegate to standard components
	cmds = append(cmds, m.timeline.Update(msg))

	var cmd tea.Cmd
	m.composer, cmd = m.composer.Update(msg)
	cmds = append(cmds, cmd)

	var fCmd tea.Cmd
	m.footer, fCmd = m.footer.Update(msg)
	cmds = append(cmds, fCmd)

	m.sidebar, cmd = m.sidebar.Update(msg)
	cmds = append(cmds, cmd)

	m.SyncLayout()

	return m, tea.Batch(cmds...)
}

func (m Model) renderComposerToolbar(width int) string {
	agentName := m.runtime.AgentName()
	var modeIndicator string
	if agentName == "plan" {
		modeIndicator = styles.BoldVioletStyle.Render("● plan")
	} else if agentName == "build" {
		modeIndicator = styles.BoldNeonStyle.Render("● build")
	} else {
		modeIndicator = styles.WarmGoldStyle.Render("● " + agentName)
	}

	var statusStr string
	if m.timeline.model.Thinking || m.footer.thinking {
		frame := thinkingFrames[m.footer.thinkingFrame]
		statusStr = "  " + styles.BoldNeonStyle.Render(frame) + " " + styles.BlueStyle.Render(agentActivityLabel(agentName) + "...")
	} else {
		statusStr = "  " + styles.GrayStyle.Render("idle")
	}

	left := modeIndicator + statusStr

	var subagentsStr string
	activeSubagents := m.runtime.ActiveSubagents()
	if len(activeSubagents) > 0 {
		subagentsStr = "  " + styles.BoldBlueStyle.Render("Subagents: ") + styles.WhiteStyle.Render(strings.Join(activeSubagents, ", "))
	}

	helpHint := styles.GrayStyle.Render("Ctrl+H help • Ctrl+A modes • Ctrl+T tools • Ctrl+R reasoning")

	leftContent := "  " + left
	if subagentsStr != "" {
		leftContent += "   " + subagentsStr
	}

	leftLen := lipgloss.Width(leftContent)
	rightLen := lipgloss.Width(helpHint)
	paddingLen := width - leftLen - rightLen - 2 // Account for right margin
	if paddingLen < 0 {
		paddingLen = 0
	}

	toolbarContent := leftContent + strings.Repeat(" ", paddingLen) + helpHint
	return toolbarContent
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	sidebarWidth := m.sidebar.width
	mainWidth := m.width - sidebarWidth

	composerView := m.composer.View()
	toolbarView := m.renderComposerToolbar(mainWidth)
	timelineView := m.timeline.View()
	footerView := m.footer.View()

	var mainView string
	if sidebarWidth > 0 {
		mainContent := lipgloss.JoinVertical(lipgloss.Left, timelineView, toolbarView, composerView)
		sidebarView := m.sidebar.View()
		mainView = lipgloss.JoinHorizontal(lipgloss.Top, mainContent, sidebarView)
	} else {
		mainView = lipgloss.JoinVertical(lipgloss.Left, timelineView, toolbarView, composerView)
	}

	base := lipgloss.JoinVertical(lipgloss.Left, mainView, footerView)

	// Dynamic popup width: adapt to terminal, capped at 72
	popupWidth := m.width - 10
	if popupWidth > 72 {
		popupWidth = 72
	}
	if popupWidth < 30 {
		popupWidth = 30
	}
	popupStyle := styles.PopupStyle.Width(popupWidth)

	if m.notificationShow {
		toast := styles.PopupStyle.
			Padding(0, 1).
			Width(30).
			BorderForeground(styles.MainNeon).
			Render(styles.BoldNeonStyle.Render("✓ ") +
				styles.WhiteStyle.Render(m.notificationText))
		base = overlayBase(base, toast, m.width, m.height)
	} else if m.providerForm.active {
		popup := popupStyle.Render(m.providerForm.View(m.runtime))
		base = overlayCenter(base, popup, m.width, m.height)
	} else if m.modelPicker.active {
		popup := popupStyle.Render(m.modelPicker.View())
		base = overlayCenter(base, popup, m.width, m.height)
	} else if m.sessionPicker.active {
		popup := popupStyle.Render(m.sessionPicker.View())
		base = overlayCenter(base, popup, m.width, m.height)
	} else if m.modePopup.active {
		popup := popupStyle.Render(m.modePopup.View())
		base = overlayCenter(base, popup, m.width, m.height)
	} else if m.showTools {
		popup := popupStyle.Render(renderToolPalette(m.runtime.ToolSpecs(), m.footer.tachikomaInfo))
		base = overlayCenter(base, popup, m.width, m.height)
	} else if m.showHelp {
		popup := popupStyle.Render(helpView())
		base = overlayCenter(base, popup, m.width, m.height)
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

	sidebarWidth := 36
	if m.width < 110 {
		sidebarWidth = 28
	}
	if m.width < 90 {
		sidebarWidth = 0
	}
	mainWidth := m.width - sidebarWidth

	m.composer.SetWidth(mainWidth)
	composerView := m.composer.View()
	composerHeight := lipgloss.Height(composerView)

	footerHeight := 1
	m.footer.width = m.width

	toolbarHeight := 1

	timelineHeight := m.height - footerHeight - composerHeight - toolbarHeight
	if timelineHeight < 4 {
		timelineHeight = 4
	}

	m.timeline.SyncLayout(mainWidth, timelineHeight)
	m.sidebar.width = sidebarWidth
	m.sidebar.height = timelineHeight + toolbarHeight + composerHeight
}

type hideNotificationMsg struct{}
