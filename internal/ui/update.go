package ui

import (
	"time"

	"github.com/Hoosk/motoko/internal/app"
	tea "github.com/charmbracelet/bubbletea"
)

// Update handles all messages for the root Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	oldComposerHeight := m.composer.Height()

	// 1. Priority Key Commands (Global)
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

	// 2. Reap dialogs the broker already resolved (e.g. by timeout)
	if cmd := m.clearExpiredDialog(); cmd != nil {
		cmds = append(cmds, cmd)
	}

	// 3. Delegate to Active Popups (Modal state)
	if handled, cmd := m.updatePopups(msg, cmds); handled {
		return m, cmd
	}

	// 4. Global Message Handling
	var done bool
	if cmds, done = m.updateGlobal(msg, cmds); done {
		return m, tea.Batch(cmds...)
	}

	// 5. Delegate to standard components
	cmds = append(cmds, m.timeline.Update(msg))

	var cmd tea.Cmd
	m.composer, cmd = m.composer.Update(msg)
	cmds = append(cmds, cmd)

	var fCmd tea.Cmd
	m.footer, fCmd = m.footer.Update(msg)
	cmds = append(cmds, fCmd)

	m.sidebar, cmd = m.sidebar.Update(msg)
	cmds = append(cmds, cmd)

	// 6. Sidebar contextual auto-open
	currentHasPendingDialog := m.runtime.PendingDialogs() > 0
	currentActiveTasks := m.runtime.ActiveTasks()
	currentActiveSubagents := len(m.runtime.ActiveSubagents())

	shouldAutoOpen := (currentHasPendingDialog && !m.prevHasPendingDialog) ||
		(currentActiveTasks > 0 && m.prevActiveTasks == 0) ||
		(currentActiveSubagents > 0 && m.prevActiveSubagents == 0)
	if shouldAutoOpen && !m.showSidebar && m.sidebarPref != sidebarForceHide {
		if _, allowed := m.sidebarLayout(); allowed {
			m.showSidebar = true
			m.sidebarPref = sidebarForceShow
		}
	}
	m.prevHasPendingDialog = currentHasPendingDialog
	m.prevActiveTasks = currentActiveTasks
	m.prevActiveSubagents = currentActiveSubagents

	if _, isKey := msg.(tea.KeyMsg); !isKey || m.composer.Height() != oldComposerHeight {
		m.SyncLayout()
	}

	return m, tea.Batch(cmds...)
}

// updatePopups routes messages to the active modal popup, if any.
func (m *Model) updatePopups(msg tea.Msg, cmds []tea.Cmd) (bool, tea.Cmd) {
	if dialog, ok := msg.(DialogRequestedMsg); ok {
		cmds, _ = m.onDialogRequested(dialog, cmds)
		m.sidebar.dirty = true
		return true, tea.Batch(cmds...)
	}
	if m.approvalBar.active || m.questionPopup.active {
		if size, ok := msg.(tea.WindowSizeMsg); ok {
			m.width = size.Width
			m.height = size.Height
			m.SyncLayout()
			return true, tea.Batch(cmds...)
		}
		switch msg := msg.(type) {
		case AgentStreamBatchMsg:
			cmds = m.onAgentStreamBatch(msg, cmds)
			return true, tea.Batch(cmds...)
		case ThinkingTickMsg:
			cmds = m.onThinkingTick(msg, cmds)
			return true, tea.Batch(cmds...)
		}
		if m.approvalBar.active {
			if key, ok := msg.(tea.KeyMsg); ok {
				switch key.String() {
				case keyUp, keyDown, "pgup", "pgdown":
					cmds = append(cmds, m.timeline.Update(msg))
					return true, tea.Batch(cmds...)
				}
			}
			if mouse, ok := msg.(tea.MouseMsg); ok {
				cmds = append(cmds, m.timeline.Update(mouse))
				return true, tea.Batch(cmds...)
			}
			if done, approved := m.approvalBar.Update(msg); done {
				cmds = append(cmds, m.resolveDialog(approved))
			}
			return true, tea.Batch(cmds...)
		}
		if done := m.questionPopup.Update(msg); done {
			m.sidebar.dirty = true
			cmds = append(cmds, m.waitDialog())
		}
		return true, tea.Batch(cmds...)
	}
	if m.providerForm.active {
		cmds = append(cmds, m.providerForm.Update(msg, m.runtime))
		return true, tea.Batch(cmds...)
	}
	if m.mcpForm.active {
		cmds = append(cmds, m.mcpForm.Update(msg, m.runtime))
		return true, tea.Batch(cmds...)
	}
	if m.modelPicker.active {
		cmds = append(cmds, m.modelPicker.Update(msg))
		return true, tea.Batch(cmds...)
	}
	if m.thinkingPicker.active {
		cmds = append(cmds, m.thinkingPicker.Update(msg))
		return true, tea.Batch(cmds...)
	}
	if m.sessionPicker.active {
		cmds = append(cmds, m.sessionPicker.Update(msg, m.runtime))
		return true, tea.Batch(cmds...)
	}
	if m.modePopup.active {
		cmds = append(cmds, m.modePopup.Update(msg, m.runtime))
		return true, tea.Batch(cmds...)
	}
	if m.commandPalette.active {
		cmds = append(cmds, m.commandPalette.Update(msg))
		return true, tea.Batch(cmds...)
	}
	if m.helpOverlay.active {
		cmds = append(cmds, m.helpOverlay.Update(msg))
		return true, tea.Batch(cmds...)
	}
	if m.settingsPopup.active {
		cmds = append(cmds, m.settingsPopup.Update(msg, m.runtime))
		return true, tea.Batch(cmds...)
	}
	return false, nil
}

// updateGlobal dispatches the non-modal message types to their handlers.
func (m *Model) updateGlobal(msg tea.Msg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		return m.onMouse(msg, cmds)
	case tea.WindowSizeMsg:
		return m.onWindowSize(msg, cmds)
	case CopySelectionMsg:
		return m.onCopySelection(msg, cmds)
	case NotificationMsg:
		return m.onNotification(msg, cmds)
	case UpdateAvailableMsg:
		return m.onUpdateAvailable(msg, cmds)
	case hideNotificationMsg:
		return m.onHideNotification(msg, cmds)
	case ErrorMsg:
		return m.onError(msg, cmds)
	case TachikomaStatusMsg:
		return m.onTachikomaStatus(msg, cmds)
	case SubmitPromptMsg:
		return m.onSubmitPrompt(msg, cmds)
	case AgentStreamBatchMsg:
		return m.onAgentStreamBatch(msg, cmds), false
	case ThinkingTickMsg:
		return m.onThinkingTick(msg, cmds), false
	case AgentResultMsg:
		return m.onAgentResult(msg, cmds)
	case ShellResultMsg:
		return m.onShellResult(msg, cmds)
	case TaskEventMsg:
		return m.onTaskEvent(msg, cmds)
	case ScheduleEventMsg:
		return m.onScheduleEvent(msg, cmds)
	case ProviderModelsMsg:
		return m.onProviderModels(msg, cmds)
	case ModelSelectedMsg:
		return m.onModelSelected(msg, cmds)
	case ThinkingBudgetSelectedMsg:
		return m.onThinkingBudgetSelected(msg, cmds)
	case DialogRequestedMsg:
		return m.onDialogRequested(msg, cmds)
	case ShellApprovalResultMsg:
		return m.onShellApprovalResult(msg, cmds)
	case ToolResultMsg:
		return m.onToolResult(msg, cmds)
	case SessionsMsg:
		return m.onSessions(msg, cmds)
	case SessionLoadedMsg:
		return m.onSessionLoaded(msg, cmds)
	case CompactResultMsg:
		return m.onCompactResult(msg, cmds)
	case AgentChangedMsg:
		return m.onAgentChanged(msg, cmds)
	case ModelChangedMsg:
		return m.onModelChanged(msg, cmds)
	case PaletteSelectedMsg:
		return m.onPaletteSelected(msg, cmds)
	case tea.KeyMsg:
		if m.queueFocus {
			return m.onQueueKey(msg, cmds)
		}
		return m.onGlobalKey(msg, cmds)
	}
	return cmds, false
}

func (m *Model) onMouse(msg tea.MouseMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	if m.showSidebar && msg.X >= m.width-m.sidebar.width && msg.Y < m.height-1 {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if m.sidebar.offset > 0 {
				m.sidebar.SetOffset(max(0, m.sidebar.offset-3))
			}
			m.SyncLayout()
			return cmds, true
		case tea.MouseButtonWheelDown:
			m.sidebar.SetOffset(m.sidebar.offset + 3)
			m.SyncLayout()
			return cmds, true
		}
	}
	return cmds, false
}

func (m *Model) onWindowSize(msg tea.WindowSizeMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	m.width = msg.Width
	m.height = msg.Height
	m.SyncLayout()
	return cmds, false
}

func (m *Model) onCopySelection(msg CopySelectionMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	if msg.Err == nil {
		cmds = append(cmds, m.showNotification("Copied to clipboard"))
	} else {
		cmds = append(cmds, m.showNotification("Copy failed: "+msg.Err.Error()))
	}
	return cmds, false
}

func (m *Model) onNotification(msg NotificationMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	cmds = append(cmds, m.showNotification(msg.Text))
	return cmds, false
}

func (m *Model) onUpdateAvailable(msg UpdateAvailableMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	cmds = append(cmds, m.showNotification("⬆ "+msg.Info.NewVersion+" available — motoko --update"))
	return cmds, false
}

func (m *Model) onHideNotification(msg hideNotificationMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	if time.Since(m.notificationTime) >= 3*time.Second {
		m.notificationShow = false
	}
	return cmds, false
}

func (m *Model) onError(msg ErrorMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: msg.Err.Error()})
	m.timeline.renderMessages()
	return cmds, false
}

func (m *Model) onTachikomaStatus(msg TachikomaStatusMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	m.footer.tachikomaInfo = msg.Statuses
	return cmds, false
}

func (m *Model) onQueueKey(msg tea.KeyMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	switch msg.String() {
	case keyEsc:
		m.queueFocus = false
	case keyUp, keyCtrlP:
		m.queueSel = clamp(m.queueSel-1, max(0, len(m.promptQueue)-1))
	case keyDown, keyCtrlN:
		m.queueSel = clamp(m.queueSel+1, max(0, len(m.promptQueue)-1))
	case "backspace", "delete":
		m.removeQueuedAt(m.queueSel)
	case "ctrl+up":
		m.moveQueued(m.queueSel, -1)
	case "ctrl+down":
		m.moveQueued(m.queueSel, 1)
	}
	return cmds, true
}

func (m *Model) onGlobalKey(msg tea.KeyMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	switch msg.String() {
	case keyEsc:
		if m.showTools || m.helpOverlay.active {
			m.showTools = false
			m.helpOverlay.active = false
			return cmds, true
		}
		if m.timeline.model.Thinking && m.cancelCurrent != nil {
			m.cancelCurrent()
			return cmds, true
		}
	case keyCtrlP:
		m.providerForm.Open(m.runtime)
	case "ctrl+q":
		if len(m.promptQueue) > 0 {
			m.queueFocus = !m.queueFocus
			m.queueSel = clamp(m.queueSel, max(0, len(m.promptQueue)-1))
			return cmds, true
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
	return cmds, false
}

func (m *Model) onPaletteSelected(msg PaletteSelectedMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	if msg.SessionID != "" {
		cmds = append(cmds, m.loadSession(msg.SessionID))
		return cmds, false
	}
	if msg.Shortcut != "" {
		switch msg.Shortcut {
		case keyCtrlM:
			cmds = append(cmds, m.listModels())
		case keyCtrlP:
			m.providerForm.Open(m.runtime)
		case keyCtrlO:
			m.sessionPicker.Open()
			cmds = append(cmds, m.listSessions())
		case keyCtrlA:
			m.modePopup.Open(m.runtime)
		case keyCtrlH:
			m.helpOverlay.Open()
		case keyCtrlT:
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
		return cmds, false
	}
	if msg.Execute {
		cmds = append(cmds, func() tea.Msg {
			return SubmitPromptMsg{Prompt: msg.Prompt}
		})
	} else {
		m.composer.SetInput(msg.Prompt)
	}
	return cmds, false
}
