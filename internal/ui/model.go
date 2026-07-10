package ui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/brain"
	"github.com/Hoosk/motoko/internal/provider"
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

type TaskEventMsg struct {
	Event app.TaskEvent
}

type AgentResultMsg struct {
	RequestID int
	Err       error
	Prompt    string
	Assistant string
	Result    agent.Result
}

type agentStreamBuffer struct {
	mu   sync.Mutex
	done bool
}

type sidebarLayoutState int

const (
	sidebarDefault sidebarLayoutState = iota
	sidebarForceShow
	sidebarForceHide
)

type Model struct {
	lastCtrlC              time.Time
	notificationTime       time.Time
	agentBuffer            *agentStreamBuffer
	agentStream            chan app.AgentStreamEvent
	cancelCurrent          context.CancelFunc
	runtime                *app.Runtime
	modelPicker            modelPickerState
	promptQueue            []string
	taskStatus             string
	notificationText       string
	sessionPicker          sessionPickerState
	sidebar                SidebarModel
	providerForm           providerForm
	modePopup              modePopupState
	commandPalette         commandPaletteState
	questionPopup          questionPopupState
	helpOverlay            helpOverlayState
	settingsPopup          settingsPopupState
	thinkingPicker         thinkingPickerState
	composer               ComposerModel
	timeline               TimelineModel
	footer                 FooterModel
	sidebarPref            sidebarLayoutState
	requestID              int
	height                 int
	width                  int
	queueSel               int
	prevActiveTasks        int
	prevActiveSubagents    int
	notificationShow       bool
	queueFocus             bool
	showTools              bool
	showSidebar            bool
	prevHasPendingApproval bool
}

func (m Model) sidebarLayout() (int, bool) {
	if m.width < 40 {
		return 0, false
	}
	if m.width < 84 {
		return 20, true
	}
	return 36, true
}

func (m Model) sidebarPreferredByWidth() bool {
	return m.width >= 140
}

func NewModel(runtime *app.Runtime) Model {
	m := Model{
		runtime:     runtime,
		timeline:    NewTimelineModel(),
		composer:    NewComposerModel(runtime),
		footer:      NewFooterModel(runtime),
		sidebar:     NewSidebarModel(runtime),
		showSidebar: false,
		sidebarPref: sidebarDefault,
	}

	m.timeline.version = runtime.Version()
	m.timeline.SetOnboarding(timelineOnboarding(runtime))

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
		m.waitQuestion(),
		m.waitScheduleEvent(),
		m.waitTaskEvent(),
		m.checkForUpdatesCmd(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	oldComposerHeight := m.composer.Height()

	// 1. Handle Priority Key Commands (Global)
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c":
			if time.Since(m.lastCtrlC) < 2*time.Second {
				m.runtime.Stop()
				return m, tea.Quit
			}
			m.lastCtrlC = time.Now()
			return m, m.showNotification("Press Ctrl+C again to exit")
		}
	}

	// 2. Delegate to Active Popups (Modal state)
	if m.providerForm.active {
		cmds = append(cmds, m.providerForm.Update(msg, m.runtime))
		return m, tea.Batch(cmds...)
	}
	if m.modelPicker.active {
		cmds = append(cmds, m.modelPicker.Update(msg))
		return m, tea.Batch(cmds...)
	}
	if m.thinkingPicker.active {
		cmds = append(cmds, m.thinkingPicker.Update(msg))
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
	if m.commandPalette.active {
		cmds = append(cmds, m.commandPalette.Update(msg))
		return m, tea.Batch(cmds...)
	}
	if m.questionPopup.active {
		if done := m.questionPopup.Update(msg); done {
			cmds = append(cmds, m.waitQuestion())
		}
		return m, tea.Batch(cmds...)
	}
	if m.helpOverlay.active {
		cmds = append(cmds, m.helpOverlay.Update(msg))
		return m, tea.Batch(cmds...)
	}
	if m.settingsPopup.active {
		cmds = append(cmds, m.settingsPopup.Update(msg, m.runtime))
		return m, tea.Batch(cmds...)
	}

	// 3. Global Message Handling
	switch msg := msg.(type) {
	case tea.MouseMsg:
		if m.showSidebar && msg.X >= m.width-m.sidebar.width && msg.Y < m.height-1 {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				if m.sidebar.offset > 0 {
					m.sidebar.SetOffset(max(0, m.sidebar.offset-3))
				}
				m.SyncLayout()
				return m, nil
			case tea.MouseButtonWheelDown:
				m.sidebar.SetOffset(m.sidebar.offset + 3)
				m.SyncLayout()
				return m, nil
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.SyncLayout()

	case CopySelectionMsg:
		if msg.Err == nil {
			cmds = append(cmds, m.showNotification("Copied to clipboard"))
		} else {
			cmds = append(cmds, m.showNotification("Copy failed: "+msg.Err.Error()))
		}

	case NotificationMsg:
		cmds = append(cmds, m.showNotification(msg.Text))

	case UpdateAvailableMsg:
		cmds = append(cmds, m.showNotification("⬆ "+msg.Info.NewVersion+" available — motoko --update"))

	case hideNotificationMsg:
		if time.Since(m.notificationTime) >= 3*time.Second {
			m.notificationShow = false
		}

	case ErrorMsg:
		m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: msg.Err.Error()})
		m.timeline.renderMessages()

	case TachikomaStatusMsg:
		m.footer.tachikomaInfo = msg.Statuses

	case SubmitPromptMsg:
		if strings.TrimSpace(msg.Prompt) == "" {
			break
		}
		if m.timeline.model.Thinking {
			m.enqueuePrompt(msg.Prompt)
			break
		}
		resp := m.runtime.HandleInput(msg.Prompt, m.runtime.GetContextInfo())

		if resp.Clear {
			m.timeline.resetMessages()
		}

		m.timeline.model.AutoScroll = true
		for _, entry := range resp.Entries {
			m.timeline.appendEntry(entry)
		}
		m.timeline.renderMessages()

		if resp.Signal != "" {
			switch resp.Signal {
			case "quit":
				m.runtime.Stop()
				cmds = append(cmds, tea.Quit)
			case "open-provider-popup":
				m.providerForm.Open(m.runtime)
			case "open-models-popup":
				cmds = append(cmds, m.listModels())
			case "open-sessions-popup":
				m.sessionPicker.Open()
				cmds = append(cmds, m.listSessions())
			case "open-mode-popup":
				m.modePopup.Open(m.runtime)
			case "open-settings-popup":
				m.settingsPopup.Open()
			}
		}

		if resp.Action != nil {
			switch resp.Action.Type {
			case app.ActionAgent:
				m.requestID++
				m.timeline.SetStreaming(true)
				m.timeline.SetThinking(true)
				m.footer.SetThinking(true)
				m.composer.SetThinking(true)
				ctx, cancel := context.WithCancel(context.Background())
				m.cancelCurrent = cancel
				m.agentStream = make(chan app.AgentStreamEvent, 100)
				m.agentBuffer = &agentStreamBuffer{}
				cmds = append(cmds, m.runAgent(ctx, resp.Action.AgentPrompt, m.requestID, m.agentStream), m.waitAgentStream(m.agentStream, m.requestID), m.thinkingTick())

			case app.ActionShell:
				cmds = append(cmds, m.runShell(resp.Action.ShellCommand))

			case app.ActionTask:
				cmds = append(cmds, m.runTask(resp.Action.TaskCommand))

			case app.ActionCompact:
				cmds = append(cmds, m.compactSession())
			}
		}

	case AgentStreamBatchMsg:
		if msg.RequestID != m.requestID {
			break
		}
		m.timeline.ApplyStreamBatch(msg.Events)
		if msg.Done && m.agentBuffer != nil {
			m.agentBuffer.mu.Lock()
			m.agentBuffer.done = true
			m.agentBuffer.mu.Unlock()
		}
		if !msg.Done && m.agentStream != nil {
			cmds = append(cmds, m.waitAgentStream(m.agentStream, msg.RequestID))
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
		if msg.RequestID != m.requestID {
			break
		}
		if cmd := m.timeline.Update(finalizeStreamMsg{Text: msg.Assistant}); cmd != nil {
			cmds = append(cmds, cmd)
		}
		m.timeline.SetThinking(false)
		m.footer.SetThinking(false)
		m.composer.SetThinking(false)
		m.cancelCurrent = nil
		if msg.Err != nil && !errors.Is(msg.Err, context.Canceled) {
			m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: msg.Err.Error()})
			m.timeline.renderMessages()
		} else if errors.Is(msg.Err, context.Canceled) {
			m.timeline.appendEntry(app.Entry{Kind: app.EntrySystem, Text: "Request cancelled."})
			m.timeline.renderMessages()
		}
		cmds = append(cmds, m.updateContextStats())
		if next, ok := m.nextPromptAfterAgent(); ok {
			cmds = append(cmds, func() tea.Msg {
				return SubmitPromptMsg{Prompt: next}
			})
		}

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

	case TaskEventMsg:
		m.footer.taskCount = m.runtime.ActiveTasks()
		if msg.Event.Done {
			m.taskStatus = "idle"
			for _, entry := range m.runtime.HandleTaskResult(msg.Event).Entries {
				m.timeline.appendEntry(entry)
			}
			m.timeline.renderMessages()

			// Auto wake up! If another request is running, this will now queue cleanly.
			if m.runtime.AgentConfigured() {
				cmds = append(cmds, func() tea.Msg {
					return SubmitPromptMsg{Prompt: "[System: Task " + msg.Event.ID + " finished. Please continue.]"}
				})
			}
		} else {
			m.taskStatus = msg.Event.Command
			m.timeline.appendEntry(app.Entry{Kind: app.EntryCommand, Text: "$ " + msg.Event.Command})
			m.timeline.appendEntry(app.Entry{Kind: app.EntrySystem, Text: "Task launched in background..."})
			m.timeline.renderMessages()
		}
		cmds = append(cmds, m.waitTaskEvent())

	case ScheduleEventMsg:
		m.timeline.appendEntry(app.Entry{Kind: app.EntrySystem, Text: "Scheduled instruction fired: " + msg.Event.Instruction})
		m.timeline.renderMessages()
		cmds = append(cmds, m.waitScheduleEvent())
		if m.runtime.AgentConfigured() {
			cmds = append(cmds, func() tea.Msg {
				return SubmitPromptMsg{Prompt: "[System: Scheduled instruction fired from " + msg.Event.ID + "] " + msg.Event.Instruction}
			})
		}

	case ProviderModelsMsg:
		if msg.Err != nil {
			m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: msg.Err.Error()})
			m.timeline.renderMessages()
		} else if len(msg.Models) == 0 {
			m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: "The provider returned no available models."})
			m.timeline.renderMessages()
		} else {
			m.modelPicker.Open(msg.Models)
		}

	case ModelSelectedMsg:
		if msg.Model.SupportsThinking {
			currentBudget := 0
			if active, ok := m.runtime.GetActiveProviderConfig(); ok {
				currentBudget = active.ThinkingBudget
			}
			m.thinkingPicker.Open(msg.Model, currentBudget)
		} else {
			cmds = append(cmds, selectModelAndBudget(m.runtime, msg.Model, 0))
		}

	case ThinkingBudgetSelectedMsg:
		cmds = append(cmds, selectModelAndBudget(m.runtime, msg.Model, msg.Budget))

	case QuestionAskedMsg:
		if msg.Pending != nil {
			m.questionPopup.Open(msg.Pending)
		}

	case SessionsMsg:
		cmds = append(cmds, m.sessionPicker.Update(msg, m.runtime))

		case SessionLoadedMsg:
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

	case CompactResultMsg:
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

	case AgentChangedMsg:
		cmds = append(cmds, m.showNotification("Agent switched to "+msg.Name))

	case ModelChangedMsg:
		cmds = append(cmds, m.showNotification("Model switched to "+msg.Model))

	case PaletteSelectedMsg:
		if msg.SessionID != "" {
			cmds = append(cmds, m.loadSession(msg.SessionID))
			break
		}
		if msg.Shortcut != "" {
			switch msg.Shortcut {
			case "ctrl+m":
				cmds = append(cmds, m.listModels())
			case "ctrl+p":
				m.providerForm.Open(m.runtime)
			case "ctrl+o":
				m.sessionPicker.Open()
				cmds = append(cmds, m.listSessions())
			case "ctrl+a":
				m.modePopup.Open(m.runtime)
			case "ctrl+h":
				m.helpOverlay.Open()
			case "ctrl+t":
				m.showTools = true
			case "ctrl+s":
				if _, allowed := m.sidebarLayout(); allowed {
					m.toggleSidebar()
				}
			case "cancel-request":
				if m.timeline.model.Thinking && m.cancelCurrent != nil {
					m.cancelCurrent()
				}
			}
			break
		}
		if msg.Execute {
			cmds = append(cmds, func() tea.Msg {
				return SubmitPromptMsg{Prompt: msg.Prompt}
			})
		} else {
			m.composer.SetInput(msg.Prompt)
		}

	case tea.KeyMsg:
		if m.queueFocus {
			switch msg.String() {
			case keyEsc:
				m.queueFocus = false
				return m, nil
			case keyUp, keyCtrlP:
				m.queueSel = clamp(m.queueSel-1, max(0, len(m.promptQueue)-1))
				return m, nil
			case keyDown, keyCtrlN:
				m.queueSel = clamp(m.queueSel+1, max(0, len(m.promptQueue)-1))
				return m, nil
			case "backspace", "delete":
				m.removeQueuedAt(m.queueSel)
				return m, nil
			case "ctrl+up":
				m.moveQueued(m.queueSel, -1)
				return m, nil
			case "ctrl+down":
				m.moveQueued(m.queueSel, 1)
				return m, nil
			default:
				return m, nil
			}
		}
		switch msg.String() {
		case keyEsc:
			if m.showTools || m.helpOverlay.active {
				m.showTools = false
				m.helpOverlay.active = false
				return m, nil
			}
			if m.timeline.model.Thinking && m.cancelCurrent != nil {
				m.cancelCurrent()
				return m, nil
			}
		case keyCtrlP:
			m.providerForm.Open(m.runtime)
		case "ctrl+q":
			if len(m.promptQueue) > 0 {
				m.queueFocus = !m.queueFocus
				m.queueSel = clamp(m.queueSel, max(0, len(m.promptQueue)-1))
				return m, nil
			}
		case "ctrl+m":
			cmds = append(cmds, m.listModels())
		case "ctrl+o":
			m.sessionPicker.Open()
			cmds = append(cmds, m.listSessions())
		case "ctrl+s", "alt+s":
			if _, allowed := m.sidebarLayout(); !allowed {
				cmds = append(cmds, m.showNotification("Sidebar disabled: terminal width too small (min 40)"))
			} else {
				m.toggleSidebar()
			}
		case "ctrl+a":
			m.modePopup.Open(m.runtime)
		case "ctrl+k":
			m.commandPalette.Open(m.paletteContext())
		case "ctrl+t":
			m.showTools = !m.showTools
		case "ctrl+h":
			if m.helpOverlay.active {
				m.helpOverlay.active = false
			} else {
				m.helpOverlay.Open()
			}
		case "ctrl+r":
			m.timeline.model.ShowReasoning = !m.timeline.model.ShowReasoning
			m.timeline.renderMessages()
			stateStr := "hidden"
			if m.timeline.model.ShowReasoning {
				stateStr = "visible"
			}
			cmds = append(cmds, m.showNotification("Reasoning is now "+stateStr))
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

	// 5. Sidebar contextual auto-open
	currentHasPendingApproval := m.runtime.PendingApproval() != ""
	currentActiveTasks := m.runtime.ActiveTasks()
	currentActiveSubagents := len(m.runtime.ActiveSubagents())

	shouldAutoOpen := (currentHasPendingApproval && !m.prevHasPendingApproval) ||
		(currentActiveTasks > 0 && m.prevActiveTasks == 0) ||
		(currentActiveSubagents > 0 && m.prevActiveSubagents == 0)
	if shouldAutoOpen && !m.showSidebar && m.sidebarPref != sidebarForceHide {
		if _, allowed := m.sidebarLayout(); allowed {
			m.showSidebar = true
			m.sidebarPref = sidebarForceShow
		}
	}
	m.prevHasPendingApproval = currentHasPendingApproval
	m.prevActiveTasks = currentActiveTasks
	m.prevActiveSubagents = currentActiveSubagents

	if _, isKey := msg.(tea.KeyMsg); !isKey || m.composer.Height() != oldComposerHeight {
		m.SyncLayout()
	}

	return m, tea.Batch(cmds...)
}

func (m Model) renderComposerToolbar(width int) string {
	agentName := m.runtime.AgentName()
	var modeIndicator string
	switch agentName {
	case "plan":
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
	paddingLen := width - leftLen - rightLen - 2 // Account for right margin
	if paddingLen < 0 {
		paddingLen = 0
	}

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
	if sidebarWidth > 0 {
		blocks := []string{timelineView, toolbarView}
		if queueView != "" {
			blocks = append(blocks, queueView)
		}
		blocks = append(blocks, composerView)
		mainContent := lipgloss.JoinVertical(lipgloss.Left, blocks...)
		sidebarView := m.sidebar.View()
		mainView = lipgloss.JoinHorizontal(lipgloss.Top, mainContent, sidebarView)
	} else {
		blocks := []string{timelineView, toolbarView}
		if queueView != "" {
			blocks = append(blocks, queueView)
		}
		blocks = append(blocks, composerView)
		mainView = lipgloss.JoinVertical(lipgloss.Left, blocks...)
	}

	base := lipgloss.JoinVertical(lipgloss.Left, mainView, footerView)

	// Dynamic popup width: adapt to terminal, capped at 50
	popupWidth := m.width - 10
	if popupWidth > 50 {
		popupWidth = 50
	}
	if popupWidth < 30 {
		popupWidth = 30
	}
	popupStyle := styles.PopupStyle.Width(popupWidth)

	// Dynamic wide popup width: adapt to terminal, capped at 76
	widePopupWidth := m.width - 10
	if widePopupWidth > 76 {
		widePopupWidth = 76
	}
	if widePopupWidth < 40 {
		widePopupWidth = 40
	}
	widePopupStyle := styles.PopupStyle.Width(widePopupWidth)

	if m.providerForm.active {
		popup := popupStyle.Render(m.providerForm.View(m.runtime))
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
	} else if m.questionPopup.active {
		popup := widePopupStyle.Render(m.questionPopup.View())
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
	queueHeight := m.queuePanelHeight(mainWidth)

	timelineHeight := m.height - footerHeight - composerHeight - toolbarHeight - queueHeight
	if timelineHeight < 4 {
		timelineHeight = 4
	}

	m.timeline.SyncLayout(mainWidth, timelineHeight)
	m.sidebar.SetDimensions(sidebarWidth, timelineHeight+toolbarHeight+queueHeight+composerHeight)
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
		Pending:     m.runtime.PendingApproval(),
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

func (m *Model) enqueuePrompt(prompt string) {
	m.promptQueue = append(m.promptQueue, prompt)
	m.queueSel = clamp(m.queueSel, max(0, len(m.promptQueue)-1))
}

func (m *Model) dequeuePrompt() (string, bool) {
	if len(m.promptQueue) == 0 {
		m.queueSel = 0
		m.queueFocus = false
		return "", false
	}
	prompt := m.promptQueue[0]
	m.promptQueue = append([]string(nil), m.promptQueue[1:]...)
	if len(m.promptQueue) == 0 {
		m.queueSel = 0
		m.queueFocus = false
	} else {
		m.queueSel = clamp(m.queueSel, len(m.promptQueue)-1)
	}
	return prompt, true
}

func (m *Model) nextPromptAfterAgent() (string, bool) {
	if br := m.runtime.GetBrain(); br != nil && br.Exists("goal") {
		pending, completed := br.TaskCounts()
		if !br.Exists("tasks") {
			return "[System: Goal still active. No tasks.md exists yet. Create or refine tasks.md first, then continue.]", true
		}
		if pending > 0 {
			attempts, lastPending := readGoalState(br)
			if lastPending == pending {
				attempts++
			} else {
				attempts = 1
			}
			_ = writeGoalState(br, attempts, pending)
			if attempts > 20 {
				_ = br.Delete("goal")
				_ = br.Delete("goal_state")
				m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: "Goal auto-continue stopped after 20 attempts without progress."})
				m.timeline.renderMessages()
				return m.dequeuePrompt()
			}
			return "[System: Goal still active. Continue with the next unfinished task in tasks.md.]", true
		}
		if pending == 0 && completed == 0 {
			attempts, lastPending := readGoalState(br)
			if lastPending == 0 {
				attempts++
			} else {
				attempts = 1
			}
			_ = writeGoalState(br, attempts, 0)
			if attempts > 20 {
				_ = br.Delete("goal")
				_ = br.Delete("goal_state")
				m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: "Goal auto-continue stopped after 20 attempts without a usable tasks.md checklist."})
				m.timeline.renderMessages()
				return m.dequeuePrompt()
			}
			return "[System: Goal still active, but tasks.md has no checkboxes yet. Refine tasks.md first, then continue.]", true
		}
		_ = br.Delete("goal_state")
		_ = br.Delete("goal")
		m.timeline.appendEntry(app.Entry{Kind: app.EntrySystem, Text: "Goal completed."})
		m.timeline.renderMessages()
	}
	return m.dequeuePrompt()
}

func readGoalState(br *brain.Brain) (attempts int, lastPending int) {
	if br == nil {
		return 0, -1
	}
	content, err := br.Read("goal_state")
	if err != nil {
		return 0, -1
	}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "attempts:") {
			_, _ = fmt.Sscanf(line, "attempts: %d", &attempts)
		}
		if strings.HasPrefix(line, "pending:") {
			_, _ = fmt.Sscanf(line, "pending: %d", &lastPending)
		}
	}
	return attempts, lastPending
}

func writeGoalState(br *brain.Brain, attempts int, pending int) error {
	if br == nil {
		return nil
	}
	return br.Write("goal_state", fmt.Sprintf("attempts: %d\npending: %d\n", attempts, pending))
}

func (m *Model) removeQueuedAt(index int) {
	if index < 0 || index >= len(m.promptQueue) {
		return
	}
	m.promptQueue = append(m.promptQueue[:index], m.promptQueue[index+1:]...)
	if len(m.promptQueue) == 0 {
		m.queueSel = 0
		m.queueFocus = false
		return
	}
	m.queueSel = clamp(m.queueSel, len(m.promptQueue)-1)
}

func (m *Model) moveQueued(index, delta int) {
	if index < 0 || index >= len(m.promptQueue) {
		return
	}
	target := clamp(index+delta, len(m.promptQueue)-1)
	if target == index {
		return
	}
	m.promptQueue[index], m.promptQueue[target] = m.promptQueue[target], m.promptQueue[index]
	m.queueSel = target
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

func (m Model) queuePanelHeight(width int) int {
	if len(m.promptQueue) == 0 || width <= 0 {
		return 0
	}
	return lipgloss.Height(m.renderQueuePanel(width))
}

func (m Model) renderQueuePanel(width int) string {
	if len(m.promptQueue) == 0 || width <= 0 {
		return ""
	}
	contentWidth := width - 4
	if contentWidth < 0 {
		contentWidth = 0
	}
	header := styles.WarmGoldStyle.Render("Queue") + " " + styles.GrayStyle.Render("("+strconv.Itoa(len(m.promptQueue))+")")
	if m.queueFocus {
		header += "  " + styles.BoldNeonStyle.Render("Ctrl+Up/Down reorder • Backspace delete • Esc close")
	} else {
		header += "  " + styles.GrayStyle.Render("Ctrl+Q manage")
	}
	lines := []string{header}
	for i, prompt := range m.promptQueue {
		line := strconv.Itoa(i+1) + ". " + strings.TrimSpace(prompt)
		if contentWidth > 6 && lipgloss.Width(line) > contentWidth {
			line = truncate(line, contentWidth)
		}
		if m.queueFocus && i == m.queueSel {
			lines = append(lines, styles.PopupSelectionStyle.Render(line))
		} else {
			lines = append(lines, styles.GrayStyle.Render(line))
		}
	}
	body := styles.InputChromeStyle.Width(contentWidth).Render(strings.Join(lines, "\n"))
	separator := styles.GrayStyle.Width(contentWidth).Render(strings.Repeat("─", contentWidth))
	return lipgloss.JoinVertical(lipgloss.Left, separator, body)
}

type hideNotificationMsg struct{}

func selectModelAndBudget(runtime *app.Runtime, model provider.ModelInfo, budget int) tea.Cmd {
	return func() tea.Msg {
		if err := runtime.SetActiveModelInfo(model); err != nil {
			return ErrorMsg{Err: err}
		}
		if err := runtime.SetThinkingBudget(budget); err != nil {
			return ErrorMsg{Err: err}
		}
		return NotificationMsg{Text: "Model updated: " + model.ID}
	}
}
