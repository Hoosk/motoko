package ui

import (
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/styles"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type TimelineModel struct {
	viewport         viewport.Model
	viewportContent  string
	messages         []string
	entries          []app.Entry
	selectedMessage  int
	width            int
	height           int
	autoScroll       bool
	streaming        bool
	streamedRunes    []rune
	streamEntryIndex int
	thinking         bool
	thinkingFrame    int
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
				if m.streamEntryIndex == -1 {
					m.appendEntry(app.Entry{Kind: app.EntryAssistant, Text: ""})
					m.streamEntryIndex = len(m.entries) - 1
				}
				if event.Content != "" {
					m.streamedRunes = append(m.streamedRunes, []rune(event.Content)...)
					if m.streamEntryIndex >= 0 && m.streamEntryIndex < len(m.entries) {
						m.entries[m.streamEntryIndex].Text = string(m.streamedRunes)
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
				if m.streamEntryIndex != -1 {
					m.streamEntryIndex = -1
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
		case "up", "down":
			// Reserved for composer history navigation; do not scroll viewport.
			return nil
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
	m.entries = nil
	m.selectedMessage = -1
	if m.viewport.Width > 0 {
		m.renderMessages()
	}
}

func (m *TimelineModel) appendEntry(entry app.Entry) {
	m.entries = append(m.entries, entry)
}

func (m *TimelineModel) renderMessages() {
	var wrapped []string
	width := m.viewport.Width
	if width <= 0 {
		return
	}
	currentOffset := m.viewport.YOffset
	selectedIdx := -1
	m.messages = m.messages[:0]
	styledLogo := lipgloss.NewStyle().Foreground(styles.MainNeon).Bold(true).Render(logoArt)
	m.messages = append(m.messages,
		styledLogo,
		styles.SystemStyle.Render("Motoko online. /provider add abre el formulario; /models lista o selecciona modelos del provider activo."),
	)
	for _, entry := range m.entries {
		m.messages = append(m.messages, m.renderEntry(entry))
	}
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
		spinner := lipgloss.NewStyle().Foreground(styles.MainNeon).Bold(true).Render(thinkingFrames[m.thinkingFrame])
		label := lipgloss.NewStyle().Foreground(styles.Gray).Italic(true).Render("  processing")
		wrapped = append(wrapped, spinner+label)
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

func (m *TimelineModel) renderEntry(entry app.Entry) string {
	switch entry.Kind {
	case app.EntryUser:
		return renderUserMessage(entry.Text, max(20, m.viewport.Width))
	case app.EntryAssistant:
		wrapped := wrapText(entry.Text, m.assistantWidth())
		return styles.AssistantBlockStyle.Width(m.assistantWidth()).Render(wrapped)
	case app.EntrySystem:
		return styles.SystemStyle.Render(entry.Text)
	case app.EntryCommand:
		return styles.CommandStyle.Render(entry.Text)
	case app.EntryOutput:
		return renderDiffOutput(entry.Text)
	case app.EntryError:
		return styles.ErrorStyle.Render(entry.Text)
	case app.EntryHelp:
		return renderHelpEntry(entry.Text)
	default:
		return entry.Text
	}
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
		m.streamEntryIndex = -1
		return
	}
	m.streamedRunes = nil
	m.streamEntryIndex = -1
}

func (m *TimelineModel) CompleteStreaming(text string) {
	trimmed := strings.TrimSpace(text)
	if trimmed != "" {
		if m.streamEntryIndex == -1 {
			m.appendEntry(app.Entry{Kind: app.EntryAssistant, Text: trimmed})
		} else if m.streamEntryIndex >= 0 && m.streamEntryIndex < len(m.entries) {
			m.entries[m.streamEntryIndex].Text = trimmed
		}
	}
	m.streaming = false
	m.streamedRunes = nil
	m.streamEntryIndex = -1
	m.renderMessages()
}

func (m TimelineModel) CopySelected() tea.Cmd {
	if m.selectedMessage >= 0 && m.selectedMessage < len(m.messages) {
		return copySelection(stripANSI(m.messages[m.selectedMessage]))
	}
	return nil
}

// renderUserMessage renders a user prompt between two thin horizontal rules.
func renderUserMessage(text string, width int) string {
	w := max(20, width) - 3
	ruleStyle := lipgloss.NewStyle().Foreground(styles.AccentViolet)
	rule := ruleStyle.Render(strings.Repeat("─", w))
	body := " " + styles.UserPromptStyle.Render(">") + "  " +
		lipgloss.NewStyle().Foreground(styles.White).Render(text)
	return strings.Join([]string{rule, body, rule}, "\n")
}

// assistantWidth returns the inner text width for AssistantBlockStyle rendering.
// AssistantBlockStyle has BorderLeft(1) + PaddingLeft(2) = 3 chars overhead.
func (m *TimelineModel) assistantWidth() int {
	return max(40, m.viewport.Width-3)
}

// renderHelpEntry renders the /help output with colourised sections.
// Expected format: first line = title, subsequent lines = "/cmd   description"
// or "!<cmd>   description".
func renderHelpEntry(text string) string {
	lines := strings.Split(text, "\n")
	titleStyle := lipgloss.NewStyle().Foreground(styles.MainNeon).Bold(true)
	cmdStyle := lipgloss.NewStyle().Foreground(styles.AccentBlue).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(styles.Gray)

	var rendered []string
	for i, line := range lines {
		if i == 0 {
			rendered = append(rendered, titleStyle.Render(line))
			continue
		}
		if line == "" {
			rendered = append(rendered, "")
			continue
		}
		// Split command name from description at first double-space run.
		idx := strings.Index(line, "  ")
		if idx <= 0 {
			rendered = append(rendered, descStyle.Render(line))
			continue
		}
		cmd := line[:idx]
		desc := strings.TrimSpace(line[idx:])
		rendered = append(rendered, cmdStyle.Render(cmd)+"  "+descStyle.Render(desc))
	}
	return strings.Join(rendered, "\n")
}
func renderDiffOutput(text string) string {
	lines := strings.Split(text, "\n")
	isDiff := false
	for _, line := range lines {
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "@@ ") {
			isDiff = true
			break
		}
	}
	if !isDiff {
		return styles.OutputStyle.Render(text)
	}

	changedCount := 0
	for _, line := range lines {
		if len(line) > 0 {
			if line[0] == '+' && !strings.HasPrefix(line, "+++ ") {
				changedCount++
			} else if line[0] == '-' && !strings.HasPrefix(line, "--- ") {
				changedCount++
			}
		}
	}

	if changedCount > 20 {
		var result []string
		for _, line := range lines {
			if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
				result = append(result, styles.DiffMetaStyle.Render(line))
			} else if strings.HasPrefix(line, "@@ ") {
				result = append(result, styles.DiffHeaderStyle.Render(line))
			}
		}
		result = append(result, styles.DiffMetaStyle.Render(fmt.Sprintf("... (%d líneas cambiadas, colapsado)", changedCount)))
		return strings.Join(result, "\n")
	}

	var result []string
	for _, line := range lines {
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
			result = append(result, styles.DiffMetaStyle.Render(line))
		} else if strings.HasPrefix(line, "@@ ") {
			result = append(result, styles.DiffHeaderStyle.Render(line))
		} else if len(line) > 0 && line[0] == '+' {
			result = append(result, styles.DiffAddStyle.Render(line))
		} else if len(line) > 0 && line[0] == '-' {
			result = append(result, styles.DiffRemoveStyle.Render(line))
		} else {
			result = append(result, styles.DiffContextStyle.Render(line))
		}
	}
	return strings.Join(result, "\n")
}
