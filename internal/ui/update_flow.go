package ui

import (
	"context"
	"errors"
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) onSubmitPrompt(msg SubmitPromptMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	if strings.TrimSpace(msg.Prompt) == "" {
		return cmds, false
	}
	if m.timeline.model.Thinking {
		m.enqueuePrompt(msg.Prompt)
		return cmds, false
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
		case "open-mcp-popup":
			m.mcpForm.Open()
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

		case app.ActionShellApproval:
			cmds = append(cmds, m.runShellApproval(resp.Action.ShellCommand, resp.Action.ShellReason))

		case app.ActionTool:
			cmds = append(cmds, m.runTool(resp.Action.ToolName, resp.Action.ToolArgs))

		case app.ActionTask:
			cmds = append(cmds, m.runTask(resp.Action.TaskCommand))

		case app.ActionCompact:
			cmds = append(cmds, m.compactSession())
		}
	}
	return cmds, false
}

func (m *Model) onAgentStreamBatch(msg AgentStreamBatchMsg, cmds []tea.Cmd) []tea.Cmd {
	if msg.RequestID != m.requestID {
		return cmds
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
	return cmds
}

func (m *Model) onThinkingTick(msg ThinkingTickMsg, cmds []tea.Cmd) []tea.Cmd {
	if m.timeline.model.Thinking || m.footer.thinking {
		cmds = append(cmds, m.thinkingTick())
	}
	m.timeline.Update(msg)
	m.footer.Update(msg)
	return cmds
}

func (m *Model) onAgentResult(msg AgentResultMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	if msg.RequestID != m.requestID {
		return cmds, false
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
	return cmds, false
}

func (m *Model) onShellResult(msg ShellResultMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	m.timeline.appendEntry(app.Entry{Kind: app.EntryCommand, Text: msg.Result.Command})
	if msg.Result.Output != "" {
		kind := app.EntryOutput
		if msg.Result.ExitCode != 0 {
			kind = app.EntryError
		}
		m.timeline.appendEntry(app.Entry{Kind: kind, Text: msg.Result.Output})
	}
	m.timeline.renderMessages()
	return cmds, false
}

func (m *Model) onShellApprovalResult(msg ShellApprovalResultMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	if msg.Err != nil {
		m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: msg.Err.Error()})
	} else {
		m.timeline.appendEntry(app.Entry{Kind: app.EntrySystem, Text: "Shell approval received."})
	}
	m.timeline.renderMessages()
	return cmds, false
}

func (m *Model) onToolResult(msg ToolResultMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	if msg.Err != nil {
		m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: msg.Err.Error()})
	} else {
		if strings.TrimSpace(msg.Result.Summary) != "" {
			m.timeline.appendEntry(app.Entry{Kind: app.EntrySystem, Text: msg.Result.Summary})
		}
		if strings.TrimSpace(msg.Result.Output) != "" {
			m.timeline.appendEntry(app.Entry{Kind: app.EntryOutput, Text: msg.Result.Output})
		}
	}
	m.timeline.renderMessages()
	return cmds, false
}

func (m *Model) onTaskEvent(msg TaskEventMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
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
	return cmds, false
}

func (m *Model) onScheduleEvent(msg ScheduleEventMsg, cmds []tea.Cmd) ([]tea.Cmd, bool) {
	m.timeline.appendEntry(app.Entry{Kind: app.EntrySystem, Text: "Scheduled instruction fired: " + msg.Event.Instruction})
	m.timeline.renderMessages()
	cmds = append(cmds, m.waitScheduleEvent())
	if m.runtime.AgentConfigured() {
		cmds = append(cmds, func() tea.Msg {
			return SubmitPromptMsg{Prompt: "[System: Scheduled instruction fired from " + msg.Event.ID + "] " + msg.Event.Instruction}
		})
	}
	return cmds, false
}
