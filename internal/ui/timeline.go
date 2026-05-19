package ui

import (
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/bubbles/viewport"
)

type TimelineModel struct {
	viewport           viewport.Model
	viewportContent    string
	messages           []string
	selectedMessage    int
	width              int
	height             int
	autoScroll         bool
	streaming          bool
	streamedRunes      []rune
	streamMessageIndex int
	thinking           bool
	thinkingFrame      int
}

func NewTimelineModel() TimelineModel {
	vp := viewport.New(80, 20)
	m := TimelineModel{
		viewport:        vp,
		autoScroll:      true,
		selectedMessage: -1,
	}
	m.resetMessages()
	return m
}

func (m TimelineModel) Init() tea.Cmd {
	return nil
}

func (m *TimelineModel) Update(msg tea.Msg) tea.Cmd {
	var vpCmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.renderMessages()

	case ClearMessagesMsg:
		m.resetMessages()

	case ResponseAppliedMsg:
		for _, entry := range msg.Response.Entries {
			m.appendEntry(entry)
		}
		m.renderMessages()

	case AgentStreamEventMsg:
		if m.streaming {
			event := msg.Event
			if event.Kind == "assistant_delta" {
				if m.streamMessageIndex == -1 {
					m.appendEntry(app.Entry{Kind: app.EntryAssistant, Text: ""})
					m.streamMessageIndex = len(m.messages) - 1
				}
				if event.Content != "" {
					m.streamedRunes = append(m.streamedRunes, []rune(event.Content)...)
					if m.streamMessageIndex >= 0 && m.streamMessageIndex < len(m.messages) {
						m.messages[m.streamMessageIndex] = styles.AssistantBlockStyle.Render(string(m.streamedRunes))
					}
				}
			} else {
				switch event.Kind {
				case "tool":
					m.appendEntry(app.Entry{Kind: app.EntryCommand, Text: fmt.Sprintf("tool %s", event.Title)})
					if strings.TrimSpace(event.Content) != "" {
						m.appendEntry(app.Entry{Kind: app.EntrySystem, Text: event.Content})
					}
				case "output":
					m.appendEntry(app.Entry{Kind: app.EntryOutput, Text: event.Content})
				case "error":
					m.appendEntry(app.Entry{Kind: app.EntryError, Text: event.Content})
				case "debug":
					m.appendEntry(app.Entry{Kind: app.EntrySystem, Text: "[debug] " + event.Content})
				}
				if m.streamMessageIndex != -1 {
					m.streamMessageIndex = -1
					m.streamedRunes = nil
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
		if m.thinking {
			m.thinkingFrame = (m.thinkingFrame + 1) % len(thinkingFrames)
			m.renderMessages()
		}

	case CopySelectionMsg:
		if msg.Err != nil {
			m.appendEntry(app.Entry{Kind: app.EntryError, Text: "clipboard: " + msg.Err.Error()})
		} else {
			m.appendEntry(app.Entry{Kind: app.EntrySystem, Text: "copiado al portapapeles"})
		}
		m.renderMessages()

	case tea.KeyMsg:
		switch msg.String() {
		case "alt+up":
			if len(m.messages) > 0 {
				if m.selectedMessage < 0 {
					m.selectedMessage = len(m.messages) - 1
				} else {
					m.selectedMessage = clamp(m.selectedMessage-1, 0, len(m.messages)-1)
				}
				m.renderMessages()
			}
			return nil
		case "alt+down":
			if len(m.messages) > 0 {
				if m.selectedMessage < 0 {
					m.selectedMessage = 0
				} else {
					m.selectedMessage = clamp(m.selectedMessage+1, 0, len(m.messages)-1)
				}
				m.renderMessages()
			}
			return nil
		case "alt+c":
			if m.selectedMessage >= 0 && m.selectedMessage < len(m.messages) {
				return copySelection(stripANSI(m.messages[m.selectedMessage]))
			}
		}
	}

	m.viewport, vpCmd = m.viewport.Update(msg)
	m.autoScroll = m.viewport.YOffset >= m.maxViewportOffset()
	cmds = append(cmds, vpCmd)

	return tea.Batch(cmds...)
}

func (m TimelineModel) View() string {
	if m.width == 0 {
		return ""
	}
	return styles.TimelineStyle.Width(m.width - 4).Height(m.viewport.Height).Render(m.viewport.View())
}

func (m *TimelineModel) SyncLayout(width, height int) {
	m.width = width
	m.viewport.Width = width - 6
	m.viewport.Height = height
	m.renderMessages()
}

func (m *TimelineModel) resetMessages() {
	styledLogo := lipgloss.NewStyle().Foreground(styles.MainNeon).Bold(true).Render(logoArt)
	m.messages = []string{
		styledLogo,
		styles.SystemStyle.Render("Motoko online. /provider add abre el formulario; /models lista o selecciona modelos del provider activo."),
	}
	m.selectedMessage = -1
	if m.viewport.Width > 0 {
		m.renderMessages()
	}
}

func (m *TimelineModel) appendEntry(entry app.Entry) {
	switch entry.Kind {
	case app.EntryUser:
		width := max(20, m.viewport.Width)
		m.messages = append(m.messages, styles.UserBlockStyle.Width(width).Render(styles.UserPromptStyle.Render(">")+" "+entry.Text))
	case app.EntryAssistant:
		m.messages = append(m.messages, styles.AssistantBlockStyle.Render(entry.Text))
	case app.EntrySystem:
		m.messages = append(m.messages, styles.SystemStyle.Render(entry.Text))
	case app.EntryCommand:
		m.messages = append(m.messages, styles.CommandStyle.Render(entry.Text))
	case app.EntryOutput:
		m.messages = append(m.messages, styles.OutputStyle.Render(entry.Text))
	case app.EntryError:
		m.messages = append(m.messages, styles.ErrorStyle.Render(entry.Text))
	default:
		m.messages = append(m.messages, entry.Text)
	}
}

func (m *TimelineModel) renderMessages() {
	var wrapped []string
	width := m.viewport.Width
	if width <= 0 {
		return
	}
	currentOffset := m.viewport.YOffset
	selectedIdx := -1
	if m.selectedMessage >= 0 && len(m.messages) > 0 {
		selectedIdx = clamp(m.selectedMessage, 0, len(m.messages)-1)
	}
	for i, msg := range m.messages {
		if i == selectedIdx {
			wrapped = append(wrapped, styles.SelectedMessageStyle.Width(width-1).Render(msg))
		} else {
			wrapped = append(wrapped, lipgloss.NewStyle().Width(width).Render(msg))
		}
	}
	if m.thinking {
		wrapped = append(wrapped, styles.SystemStyle.Render(thinkingFrames[m.thinkingFrame]))
	}
	m.viewportContent = strings.Join(wrapped, "\n\n")
	m.viewport.SetContent(m.viewportContent)
	maxOffset := m.maxViewportOffset()
	if m.autoScroll || currentOffset >= maxOffset {
		m.viewport.GotoBottom()
		m.autoScroll = true
		return
	}
	m.viewport.YOffset = clamp(currentOffset, 0, maxOffset)
}

func (m TimelineModel) maxViewportOffset() int {
	if m.viewport.Height <= 0 || m.viewportContent == "" {
		return 0
	}
	lineCount := strings.Count(m.viewportContent, "\n") + 1
	return max(0, lineCount-m.viewport.Height)
}

func (m *TimelineModel) SetThinking(thinking bool) {
	m.thinking = thinking
	if thinking {
		m.thinkingFrame = 0
	}
}

func (m *TimelineModel) SetStreaming(streaming bool) {
	m.streaming = streaming
	if streaming {
		m.streamedRunes = nil
		m.streamMessageIndex = -1
		return
	}
	m.streamedRunes = nil
	m.streamMessageIndex = -1
}

func (m *TimelineModel) CompleteStreaming(text string) {
	trimmed := strings.TrimSpace(text)
	if trimmed != "" {
		if m.streamMessageIndex == -1 {
			m.appendEntry(app.Entry{Kind: app.EntryAssistant, Text: trimmed})
		} else if m.streamMessageIndex >= 0 && m.streamMessageIndex < len(m.messages) {
			m.messages[m.streamMessageIndex] = styles.AssistantBlockStyle.Render(trimmed)
		}
	}
	m.streaming = false
	m.streamedRunes = nil
	m.streamMessageIndex = -1
	m.renderMessages()
}

func (m TimelineModel) CopySelected() tea.Cmd {
	if m.selectedMessage >= 0 && m.selectedMessage < len(m.messages) {
		return copySelection(stripANSI(m.messages[m.selectedMessage]))
	}
	return nil
}
