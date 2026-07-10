package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/ui/timeline"
	tea "github.com/charmbracelet/bubbletea"
)

type TimelineModel struct {
	version    string
	onboarding []string
	model      timeline.Model
}

func (m *TimelineModel) startupMessages() []string {
	messages := []string{styles.BoldNeonStyle.Render(m.getLogo())}
	if len(m.onboarding) > 0 {
		messages = append(messages, m.onboarding...)
	} else {
		messages = append(messages,
			styles.SystemStyle.Render("Inspect code, edit files, run tools, or ask for a focused review."),
			styles.GrayStyle.Render("Try: /help  /models list  /sessions  /provider add"),
		)
	}
	return messages
}

func NewTimelineModel() TimelineModel {
	m := TimelineModel{
		model:   timeline.New(80, 20),
		version: "dev",
	}
	m.resetMessages()
	return m
}

func (m *TimelineModel) SetOnboarding(lines []string) {
	m.onboarding = append([]string(nil), lines...)
	if len(m.model.Entries) == 0 {
		m.resetMessages()
	}
}

func (m *TimelineModel) getLogo() string {
	lines := strings.Split(timeline.LogoArt, "\n")
	if len(lines) == 0 {
		return timeline.LogoArt
	}
	lines[len(lines)-1] += "  " + "// v" + m.version

	// Calculate max width of any line
	maxW := 0
	for _, line := range lines {
		w := utf8.RuneCountInString(line)
		if w > maxW {
			maxW = w
		}
	}

	// Pad lines to center the logo based on model width
	padding := 0
	if m.model.Width > maxW {
		padding = (m.model.Width - maxW) / 2
	}

	if padding > 0 {
		padStr := strings.Repeat(" ", padding)
		for i, line := range lines {
			lines[i] = padStr + line
		}
	}

	return strings.Join(lines, "\n")
}

func (m TimelineModel) Init() tea.Cmd {
	return nil
}

func (m *TimelineModel) Update(msg tea.Msg) tea.Cmd {
	var vpCmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.model.Width = msg.Width
		m.renderMessages()

	case ClearMessagesMsg:
		m.resetMessages()

	case ResponseAppliedMsg:
		for _, entry := range msg.Response.Entries {
			m.appendEntry(entry)
		}
		m.renderMessages()

	case AgentStreamEventMsg:
		if m.model.Streaming {
			event := msg.Event
			if event.Kind == "assistant_delta" || event.Kind == "thinking_delta" {
				targetKind := app.EntryAssistant
				content := event.Content
				if event.Kind == "thinking_delta" {
					targetKind = app.EntryReasoning
					content = event.ReasoningContent
				}

				if m.model.StreamEntryIndex == -1 || m.model.Entries[m.model.StreamEntryIndex].Kind != targetKind {
					m.appendEntry(app.Entry{Kind: targetKind, Text: ""})
					m.model.StreamEntryIndex = len(m.model.Entries) - 1
					m.model.StreamedRunes = nil
				}
				if content != "" {
					m.model.StreamedRunes = append(m.model.StreamedRunes, []rune(content)...)
					if m.model.StreamEntryIndex >= 0 && m.model.StreamEntryIndex < len(m.model.Entries) {
						m.model.Entries[m.model.StreamEntryIndex].Text = string(m.model.StreamedRunes)
					}
				}
			} else {
				m.appendStreamEvent(event)
				if m.model.StreamEntryIndex != -1 {
					m.model.StreamEntryIndex = -1
					m.model.StreamedRunes = nil
				}
			}
			m.renderMessages()
		}

	case finalizeStreamMsg:
		m.CompleteStreaming(msg.Text)

	case ProviderModelsMsg:
		for _, entry := range entriesForProviderModels(msg.Models, msg.Err) {
			m.appendEntry(entry)
		}
		m.renderMessages()

	case ThinkingTickMsg:
		if m.model.Thinking {
			m.model.ThinkingFrame = (m.model.ThinkingFrame + 1) % len(timeline.ThinkingFrames)
			m.renderMessages()
		}

	case tea.MouseMsg:
		switch msg.Action {
		case tea.MouseActionPress:
			if msg.Button == tea.MouseButtonLeft {
				cx, cy, ok := m.MouseContentCoords(msg.X, msg.Y)
				if !ok {
					if m.CancelSelection() {
						return nil
					}
				} else if m.BeginSelection(cx, cy) {
					return nil
				}
			}
		case tea.MouseActionMotion:
			cx, cy := m.ClampMouseContentCoords(msg.X, msg.Y)
			if m.UpdateSelection(cx, cy) {
				return nil
			}
		case tea.MouseActionRelease:
			cx, cy := m.ClampMouseContentCoords(msg.X, msg.Y)
			if cmd := m.EndSelection(cx, cy); cmd != nil {
				return cmd
			}
		}

	case tea.KeyMsg:
		switch msg.String() {
		case keyUp, keyDown:
			return nil
		case "alt+up":
			if len(m.model.Messages) > 0 {
				if m.model.SelectedMessage < 0 {
					m.model.SelectedMessage = len(m.model.Messages) - 1
				} else {
					m.model.SelectedMessage = clamp(m.model.SelectedMessage-1, len(m.model.Messages)-1)
				}
				m.renderMessages()
			}
			return nil
		case "alt+down":
			if len(m.model.Messages) > 0 {
				if m.model.SelectedMessage < 0 {
					m.model.SelectedMessage = 0
				} else {
					m.model.SelectedMessage = clamp(m.model.SelectedMessage+1, len(m.model.Messages)-1)
				}
				m.renderMessages()
			}
			return nil
		case "alt+c":
			if text, ok := m.model.SelectedText(); ok {
				return copySelection(text)
			}
			if m.model.SelectedMessage >= 0 && m.model.SelectedMessage < len(m.model.Messages) {
				return copySelection(timeline.StripANSI(m.model.Messages[m.model.SelectedMessage]))
			}
		}
	}

	m.model.Viewport, vpCmd = m.model.Viewport.Update(msg)
	m.model.AutoScroll = m.model.Viewport.YOffset >= m.model.MaxViewportOffset()
	cmds = append(cmds, vpCmd)

	return tea.Batch(cmds...)
}

func (m *TimelineModel) ApplyStreamBatch(events []app.AgentStreamEvent) {
	if !m.model.Streaming || len(events) == 0 {
		return
	}
	for _, event := range events {
		if event.Kind == "assistant_delta" || event.Kind == "thinking_delta" {
			targetKind := app.EntryAssistant
			content := event.Content
			if event.Kind == "thinking_delta" {
				targetKind = app.EntryReasoning
				content = event.ReasoningContent
			}

			if m.model.StreamEntryIndex == -1 || m.model.Entries[m.model.StreamEntryIndex].Kind != targetKind {
				m.appendEntry(app.Entry{Kind: targetKind, Text: ""})
				m.model.StreamEntryIndex = len(m.model.Entries) - 1
				m.model.StreamedRunes = nil
			}
			if content != "" {
				m.model.StreamedRunes = append(m.model.StreamedRunes, []rune(content)...)
				if m.model.StreamEntryIndex >= 0 && m.model.StreamEntryIndex < len(m.model.Entries) {
					m.model.Entries[m.model.StreamEntryIndex].Text = string(m.model.StreamedRunes)
				}
			}
			continue
		}

		m.appendStreamEvent(event)
		if m.model.StreamEntryIndex != -1 {
			m.model.StreamEntryIndex = -1
			m.model.StreamedRunes = nil
		}
	}
	m.renderMessages()
}

func (m TimelineModel) View() string {
	if m.model.Width <= 0 || m.model.Height <= 0 {
		return ""
	}
	vpWidth := m.model.Width - 2
	if vpWidth < 0 {
		vpWidth = 0
	}
	vpHeight := m.model.Height - 2
	if vpHeight < 0 {
		vpHeight = 0
	}
	return styles.TimelineStyle.Width(vpWidth).Height(vpHeight).Render(m.model.Viewport.View())
}

func (m *TimelineModel) SyncLayout(width, height int) {
	widthChanged := m.model.Width != width
	heightChanged := m.model.Height != height

	if !widthChanged && !heightChanged {
		return
	}

	m.model.Width = width
	m.model.Height = height

	vpWidth := width - 2
	if vpWidth < 0 {
		vpWidth = 0
	}
	m.model.Viewport.Width = vpWidth

	vpHeight := height - 2
	if vpHeight < 0 {
		vpHeight = 0
	}
	m.model.Viewport.Height = vpHeight

	if widthChanged {
		m.renderMessages()
	} else if heightChanged {
		m.model.SyncHighlight()
	}
}

func (m *TimelineModel) resetMessages() {
	m.model.Messages = m.startupMessages()
	m.model.Entries = nil
	m.model.SelectedMessage = -1
	m.model.AutoScroll = true
	if m.model.Viewport.Width > 0 {
		m.renderMessages()
	}
}

func (m *TimelineModel) appendEntry(entry app.Entry) {
	m.model.Entries = append(m.model.Entries, entry)
	m.model.AutoScroll = true
}

func (m *TimelineModel) renderMessages() {
	width := m.model.Viewport.Width
	if width <= 0 {
		return
	}
	selectedIdx := -1
	m.model.RenderLines = m.model.RenderLines[:0]
	m.model.Messages = m.model.Messages[:0]
	m.model.Messages = append(m.model.Messages, m.startupMessages()...)
	for _, entry := range m.model.VisibleEntries() {
		m.model.Messages = append(m.model.Messages, m.model.RenderEntry(entry))
	}
	if m.model.SelectedMessage >= 0 && len(m.model.Messages) > 0 {
		selectedIdx = clamp(m.model.SelectedMessage, len(m.model.Messages)-1)
	}
	for i, msg := range m.model.Messages {
		rendered := msg
		if i == selectedIdx {
			rendered = styles.SelectedMessageStyle.Render(msg)
		}
		m.model.AppendRenderedBlock(rendered, m.model.RenderLineMetadata(i), i < len(m.model.Messages)-1)
	}
	if m.model.Thinking {
		spinner := styles.BoldNeonStyle.Render(timeline.ThinkingFrames[m.model.ThinkingFrame])
		label := styles.ItalicGrayStyle.Render("  processing")
		m.model.AppendRenderedBlock(spinner+label, []timeline.RenderLine{{Content: timeline.StripANSI(spinner + label)}}, false)
	}

	styledLines := make([]string, len(m.model.RenderLines))
	for i, line := range m.model.RenderLines {
		styledLines[i] = line.Styled
	}
	m.model.ViewportContent = strings.Join(styledLines, "\n")
	m.model.SyncHighlight()
}

func (m *TimelineModel) SetThinking(thinking bool) {
	if m.model.Thinking == thinking {
		return
	}
	m.model.Thinking = thinking
	if thinking {
		m.model.ThinkingFrame = 0
	}
	m.renderMessages()
}

func (m *TimelineModel) SetStreaming(streaming bool) {
	m.model.Streaming = streaming
	if streaming {
		m.model.StreamedRunes = nil
		m.model.StreamEntryIndex = -1
		return
	}
	m.model.StreamedRunes = nil
	m.model.StreamEntryIndex = -1
}

func (m *TimelineModel) CompleteStreaming(text string) {
	trimmed := strings.TrimSpace(text)
	if trimmed != "" {
		if m.model.StreamEntryIndex == -1 {
			m.appendEntry(app.Entry{Kind: app.EntryAssistant, Text: trimmed})
		} else if m.model.StreamEntryIndex >= 0 && m.model.StreamEntryIndex < len(m.model.Entries) {
			m.model.Entries[m.model.StreamEntryIndex].Text = trimmed
		}
	}
	m.model.Streaming = false
	m.model.StreamedRunes = nil
	m.model.StreamEntryIndex = -1
	m.renderMessages()
}

func (m *TimelineModel) MouseContentCoords(x, y int) (int, int, bool) {
	return m.model.MouseContentCoords(x, y)
}

func (m *TimelineModel) ClampMouseContentCoords(x, y int) (int, int) {
	return m.model.ClampMouseContentCoords(x, y)
}

func (m *TimelineModel) BeginSelection(x, y int) bool {
	return m.model.BeginSelection(x, y)
}

func (m *TimelineModel) UpdateSelection(x, y int) bool {
	return m.model.UpdateSelection(x, y)
}

func (m *TimelineModel) EndSelection(x, y int) tea.Cmd {
	text := m.model.EndSelection(x, y)
	if text != "" {
		return copySelection(text)
	}
	return nil
}

func (m *TimelineModel) CancelSelection() bool {
	return m.model.CancelSelection()
}

func (m *TimelineModel) MessageAtY(y int) int {
	return m.model.MessageAtY(y)
}

func (m TimelineModel) CopySelected() tea.Cmd {
	if m.model.SelectedMessage >= 0 && m.model.SelectedMessage < len(m.model.Messages) {
		return copySelection(timeline.StripANSI(m.model.Messages[m.model.SelectedMessage]))
	}
	return nil
}

func (m TimelineModel) CopyRange(startIdx, endIdx int) tea.Cmd {
	if startIdx < 0 || endIdx < 0 {
		return nil
	}
	if startIdx > endIdx {
		startIdx, endIdx = endIdx, startIdx
	}

	var parts []string
	for i := startIdx; i <= endIdx && i < len(m.model.Messages); i++ {
		parts = append(parts, timeline.StripANSI(m.model.Messages[i]))
	}

	if len(parts) == 0 {
		return nil
	}

	return copySelection(strings.Join(parts, "\n\n"))
}

func (m *TimelineModel) appendStreamEvent(event app.AgentStreamEvent) {
	switch event.Kind {
	case "tool":
		m.appendEntry(app.Entry{Kind: app.EntryCommand, Text: "tool " + event.Title})
		if strings.TrimSpace(event.Content) != "" {
			m.appendEntry(app.Entry{Kind: app.EntrySystem, Text: event.Content})
		}
	case "task_started":
		m.appendEntry(app.Entry{Kind: app.EntryCommand, Text: "$ " + event.Title})
		m.appendEntry(app.Entry{Kind: app.EntrySystem, Text: "Task launched in background..."})
	case "task_finished":
		m.appendEntry(app.Entry{Kind: app.EntrySystem, Text: event.Content})
		if strings.TrimSpace(event.ReasoningContent) != "" {
			m.appendEntry(app.Entry{Kind: app.EntryOutput, Text: event.ReasoningContent})
		}
	case "output":
		if event.Title == "web_search" || event.Title == "web_fetch" {
			m.appendEntry(app.Entry{Kind: app.EntrySystem, Text: fmt.Sprintf("[%s: %d characters]", event.Title, len(event.Content))})
		} else {
			m.appendEntry(app.Entry{Kind: app.EntryOutput, Text: event.Content})
		}
	case "error":
		m.appendEntry(app.Entry{Kind: app.EntryError, Text: event.Content})
	case "debug":
		m.appendEntry(app.Entry{Kind: app.EntrySystem, Text: "[debug] " + event.Content})
	}
}
