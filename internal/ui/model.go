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
	return Model{
		runtime:  runtime,
		timeline: NewTimelineModel(),
		composer: NewComposerModel(runtime),
		footer:   NewFooterModel(runtime),
		sidebar:  NewSidebarModel(runtime),
	}
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
		m.timeline.appendEntry(app.Entry{Kind: app.EntryUser, Text: msg.Prompt})
		m.timeline.renderMessages()
		m.timeline.SetStreaming(true)
		m.timeline.SetThinking(true)
		m.footer.SetThinking(true)
		m.composer.SetThinking(true)
		m.agentStream = make(chan app.AgentStreamEvent, 100)
		m.agentBuffer = &agentStreamBuffer{}
		cmds = append(cmds, m.runAgent(msg.Prompt), m.waitAgentStream(m.agentStream), m.thinkingTick())

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

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	sidebarWidth := 36
	if m.width < 110 {
		sidebarWidth = 28
	}
	if m.width < 90 {
		sidebarWidth = 0
	}
	mainWidth := m.width - sidebarWidth

	footerHeight := 1
	composerHeight := m.composer.Height()
	// The sidebar should cover exactly the same height as timeline + composer
	timelineHeight := m.height - footerHeight - composerHeight

	m.timeline.SyncLayout(mainWidth, timelineHeight)
	m.composer.SetWidth(mainWidth)
	m.footer.width = m.width

	timelineView := m.timeline.View()
	composerView := m.composer.View()
	footerView := m.footer.View()

	var mainView string
	if sidebarWidth > 0 {
		m.sidebar.width = sidebarWidth
		// Use lipgloss.Height to ensure exact matching if there's any internal padding/border
		m.sidebar.height = lipgloss.Height(timelineView) + lipgloss.Height(composerView)
		sidebarView := m.sidebar.View()
		mainContent := lipgloss.JoinVertical(lipgloss.Left, timelineView, composerView)
		mainView = lipgloss.JoinHorizontal(lipgloss.Top, mainContent, sidebarView)
	} else {
		mainView = lipgloss.JoinVertical(lipgloss.Left, timelineView, composerView)
	}

	base := lipgloss.JoinVertical(lipgloss.Left, mainView, footerView)

	if m.notificationShow {
		toast := styles.PopupStyle.
			Padding(0, 1).
			Width(30).
			BorderForeground(styles.MainNeon).
			Render(styles.BoldNeonStyle.Render("✓ ") +
				styles.WhiteStyle.Render(m.notificationText))
		return overlayBase(base, toast, m.width, m.height)
	}

	if m.providerForm.active {
		popup := styles.PopupStyle.Render(m.providerForm.View(m.runtime))
		return overlayBase(base, popup, m.width, m.height)
	}

	if m.modelPicker.active {
		popup := styles.PopupStyle.Render(m.modelPicker.View())
		return overlayBase(base, popup, m.width, m.height)
	}

	if m.sessionPicker.active {
		popup := styles.PopupStyle.Render(m.sessionPicker.View())
		return overlayBase(base, popup, m.width, m.height)
	}

	if m.modePopup.active {
		popup := styles.PopupStyle.Render(m.modePopup.View())
		return overlayBase(base, popup, m.width, m.height)
	}

	if m.showTools {
		popup := styles.PopupStyle.Render(renderToolPalette(m.runtime.ToolSpecs(), m.footer.tachikomaInfo))
		return overlayBase(base, popup, m.width, m.height)
	}

	if m.showHelp {
		popup := styles.PopupStyle.Render(helpView())
		return overlayBase(base, popup, m.width, m.height)
	}

	return base
}

func (m *Model) SyncLayout() {
	// Handled in View for now to ensure consistency
}

type hideNotificationMsg struct{}
