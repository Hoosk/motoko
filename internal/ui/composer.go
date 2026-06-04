package ui

import (
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/styles"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type ComposerModel struct {
	textarea           textarea.Model
	suggestions        []string
	suggestionBase     []string
	mentionSuggestions []string
	mentionIndex       int
	selectedSuggestion int
	runtime            *app.Runtime
	width              int
	height             int
	thinking           bool
	history            []string
	historyIndex       int
	savedInput         string
}

func NewComposerModel(runtime *app.Runtime) ComposerModel {
	ta := textarea.New()
	ta.Focus()
	ta.Prompt = ""
	ta.SetWidth(80)
	ta.SetHeight(2)
	ta.ShowLineNumbers = false

	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Text = styles.WhiteStyle
	ta.BlurredStyle.Text = styles.WhiteStyle
	ta.FocusedStyle.Placeholder = styles.GrayStyle
	ta.BlurredStyle.Placeholder = styles.GrayStyle
	ta.EndOfBufferCharacter = ' '

	m := ComposerModel{
		textarea:     ta,
		runtime:      runtime,
		historyIndex: -1,
	}
	m.syncInputChrome()
	m.refreshSuggestions()
	return m
}

func (m ComposerModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m ComposerModel) Update(msg tea.Msg) (ComposerModel, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.MouseMsg:
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.syncLayout()

	case ResponseAppliedMsg:
		m.thinking = false
		m.refreshSuggestions()

	case AgentResultMsg, ShellResultMsg:
		m.thinking = false
		m.refreshSuggestions()

	case tea.KeyMsg:
		if m.thinking {
			return m, nil
		}

		switch msg.String() {
		case "tab", "right":
			if len(m.mentionSuggestions) > 0 {
				m.advanceMention(1)
				return m, nil
			}
			if len(m.suggestions) > 0 {
				m.advanceSuggestion(1)
				return m, nil
			}
		case "shift+tab", "left":
			if len(m.mentionSuggestions) > 0 {
				m.advanceMention(-1)
				return m, nil
			}
			if len(m.suggestions) > 0 {
				m.advanceSuggestion(-1)
				return m, nil
			}
		case "down", "ctrl+n":
			if len(m.mentionSuggestions) > 0 {
				m.advanceMention(1)
				return m, nil
			}
			m.clearSuggestionCycle()
			m.navigateHistoryDown()
			return m, nil
		case "up", "ctrl+p":
			if len(m.mentionSuggestions) > 0 {
				m.advanceMention(-1)
				return m, nil
			}
			m.clearSuggestionCycle()
			m.navigateHistoryUp()
			return m, nil
		case "enter":
			if len(m.mentionSuggestions) > 0 {
				m.applySelectedMention()
				return m, nil
			}
			if len(m.suggestions) > 0 && shouldApplySuggestionOnEnter(m.textarea.Value(), m.suggestions[m.selectedSuggestion]) {
				m.applySelectedSuggestion()
				return m, nil
			}
			m.clearSuggestionCycle()
			input := m.textarea.Value()
			if strings.TrimSpace(input) != "" {
				m.history = append(m.history, input)
				m.historyIndex = -1
				m.savedInput = ""
				m.textarea.Reset()
				m.refreshSuggestions()
				return m, func() tea.Msg {
					return SubmitPromptMsg{Prompt: input}
				}
			}
		}
	}

	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)
	m.syncLayout()

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyRunes, tea.KeyBackspace, tea.KeyDelete:
			m.clearSuggestionCycle()
			m.historyIndex = -1
			m.savedInput = ""
		}
		m.refreshSuggestions()
	}

	return m, tea.Batch(cmds...)
}

func (m ComposerModel) View() string {
	if m.width == 0 {
		return ""
	}

	prompt := m.renderInputPrompt()
	rows := m.textarea.Height()
	promptLines := make([]string, rows)
	for i := range promptLines {
		if i == 0 {
			promptLines[i] = prompt
		} else {
			promptLines[i] = " "
		}
	}

	promptBlock := lipgloss.NewStyle().Width(3).Render(strings.Join(promptLines, "\n"))
	mentionDropdown := m.renderMentionDropdownBlock()
	suggestions := m.renderSuggestionsLine()
	suggestionsBlock := lipgloss.NewStyle().MarginTop(1).Render(suggestions)

	body := lipgloss.JoinHorizontal(lipgloss.Top, promptBlock, styles.InputStyle.Render(m.textarea.View()))

	var blocks []string
	blocks = append(blocks, body)
	if mentionDropdown != "" {
		blocks = append(blocks, mentionDropdown)
	}
	if suggestionsBlock != "" {
		blocks = append(blocks, suggestionsBlock)
	}

	chromeWidth := m.width - 4
	if chromeWidth < 0 {
		chromeWidth = 0
	}
	return styles.InputChromeStyle.Width(chromeWidth).Render(
		lipgloss.JoinVertical(lipgloss.Left, blocks...),
	)
}

func (m *ComposerModel) SetWidth(width int) {
	m.width = width
	m.syncLayout()
}

func (m *ComposerModel) SyncLayout(width, height int) {
	m.width = width
	m.height = height
	m.syncLayout()
}

func (m *ComposerModel) syncLayout() {
	if m.width <= 0 {
		return
	}
	// Overhead: Chrome (4) + promptBlock (3) = 7
	textareaWidth := max(16, m.width-7)
	m.textarea.SetWidth(textareaWidth)
	m.textarea.SetHeight(2)
}
func (m *ComposerModel) refreshSuggestions() {
	m.syncInputChrome()
	if m.thinking {
		m.suggestions = nil
		m.mentionSuggestions = nil
		m.selectedSuggestion = 0
		m.mentionIndex = 0
		return
	}
	m.mentionSuggestions = m.runtime.MentionSuggestions(m.textarea.Value())
	if len(m.mentionSuggestions) == 0 {
		m.suggestions = m.runtime.Completions(m.textarea.Value())
	} else {
		m.suggestions = nil
		if m.mentionIndex < 0 || m.mentionIndex >= len(m.mentionSuggestions) {
			m.mentionIndex = 0
		}
	}
	m.suggestionBase = nil
	if len(m.suggestions) == 0 && len(m.mentionSuggestions) == 0 {
		m.selectedSuggestion = 0
		return
	}
	if len(m.suggestions) > 0 {
		if m.selectedSuggestion >= len(m.suggestions) {
			m.selectedSuggestion = len(m.suggestions) - 1
		}
		if m.selectedSuggestion < 0 {
			m.selectedSuggestion = 0
		}
	}
}

func (m *ComposerModel) applySelectedSuggestion() {
	if len(m.suggestions) == 0 {
		return
	}
	if m.selectedSuggestion < 0 || m.selectedSuggestion >= len(m.suggestions) {
		m.selectedSuggestion = 0
	}
	m.textarea.SetValue(m.suggestions[m.selectedSuggestion])
	m.textarea.CursorEnd()
	m.clearSuggestionCycle()
}

func (m *ComposerModel) advanceSuggestion(step int) {
	if len(m.suggestions) == 0 {
		return
	}
	if len(m.suggestionBase) == 0 {
		m.suggestionBase = append([]string(nil), m.suggestions...)
		current := strings.TrimSpace(m.textarea.Value())
		for i, suggestion := range m.suggestionBase {
			if strings.TrimSpace(suggestion) == current {
				m.selectedSuggestion = i
				break
			}
		}
	}
	m.suggestions = append([]string(nil), m.suggestionBase...)
	m.selectedSuggestion = (m.selectedSuggestion + step + len(m.suggestions)) % len(m.suggestions)
	if m.selectedSuggestion < 0 || m.selectedSuggestion >= len(m.suggestions) {
		m.selectedSuggestion = 0
	}
	m.textarea.SetValue(m.suggestions[m.selectedSuggestion])
	m.textarea.CursorEnd()
}

func (m *ComposerModel) clearSuggestionCycle() {
	m.suggestionBase = nil
}

func (m *ComposerModel) advanceMention(step int) {
	if len(m.mentionSuggestions) == 0 {
		return
	}
	if m.mentionIndex < 0 || m.mentionIndex >= len(m.mentionSuggestions) {
		m.mentionIndex = 0
	}
	m.mentionIndex = (m.mentionIndex + step + len(m.mentionSuggestions)) % len(m.mentionSuggestions)
}

func (m *ComposerModel) applySelectedMention() {
	if len(m.mentionSuggestions) == 0 {
		return
	}
	if m.mentionIndex < 0 || m.mentionIndex >= len(m.mentionSuggestions) {
		m.mentionIndex = 0
	}
	m.textarea.SetValue(m.runtime.ReplaceTrailingMention(m.textarea.Value(), m.mentionSuggestions[m.mentionIndex]))
	m.textarea.CursorEnd()
	m.refreshSuggestions()
}

func (m *ComposerModel) syncInputChrome() {
	if m.runtime.InputMode() == app.InputModeShell {
		m.textarea.Placeholder = "Shell mode active: type a command or /chat to exit"
		return
	}
	m.textarea.Placeholder = "Type a prompt, /tool ..., or !command"
}

func (m ComposerModel) renderInputPrompt() string {
	if m.runtime.InputMode() == app.InputModeShell {
		return styles.WarmGoldStyle.Bold(true).Render("$")
	}
	return styles.BoldNeonStyle.Render(">")
}

func (m ComposerModel) renderSuggestionsLine() string {
	chromeWidth := m.width - 4
	if chromeWidth < 0 {
		chromeWidth = 0
	}

	statusWidth := 22
	showStatus := chromeWidth >= 45

	var status string
	if showStatus {
		if m.thinking {
			status = lipgloss.NewStyle().Width(statusWidth).Align(lipgloss.Right).Render(styles.InputHintStyle.Render("[" + composerActivityLabel(m.runtime.AgentName()) + "]"))
		} else {
			status = lipgloss.NewStyle().Width(statusWidth).Align(lipgloss.Right).Render("")
		}
	}

	availWidth := chromeWidth
	if showStatus {
		availWidth -= statusWidth
	}
	if availWidth < 0 {
		availWidth = 0
	}

	var detail string
	if len(m.suggestions) == 0 {
		if m.runtime.InputMode() == app.InputModeShell {
			if chromeWidth < 50 {
				detail = "Shell active • /chat to exit"
			} else if chromeWidth < 75 {
				detail = "Direct shell active • /chat: exit • Ctrl+T: tools"
			} else {
				detail = "Direct shell active. Enter to execute. /chat to exit. Ctrl+T for tools."
			}
		} else {
			if chromeWidth < 50 {
				detail = "Tab: suggest • Ctrl+H: help"
			} else if chromeWidth < 75 {
				detail = "Tab: rotate suggestions • /provider add • /models"
			} else {
				detail = "Tab to rotate suggestions. /provider add for config. /models for selection."
			}
		}
		detail = styles.InputHintStyle.Render(detail)
	} else {
		// Render completions
		limit := len(m.suggestions)
		// Dynamically decrease limit if it doesn't fit
		for limit > 0 {
			items := make([]string, 0, limit)
			for i := 0; i < min(limit, len(m.suggestions)); i++ {
				if i == m.selectedSuggestion {
					items = append(items, styles.SelectionStyle.Render(m.suggestions[i]))
				} else {
					items = append(items, styles.SuggestionStyle.Render(m.suggestions[i]))
				}
			}
			detail = strings.Join(items, "   ")
			if lipgloss.Width(detail) <= availWidth || limit == 1 {
				break
			}
			limit--
		}
		// If even 1 suggestion is too wide, truncate it
		if lipgloss.Width(detail) > availWidth && availWidth > 5 {
			detail = truncate(detail, availWidth)
		}
	}

	if showStatus {
		// Pad detail to availWidth so status is right-aligned
		detailLen := lipgloss.Width(detail)
		pad := availWidth - detailLen
		if pad > 0 {
			detail += strings.Repeat(" ", pad)
		}
		return lipgloss.JoinHorizontal(lipgloss.Left, detail, status)
	}
	return detail
}

func (m ComposerModel) renderMentionDropdownBlock() string {
	if len(m.mentionSuggestions) == 0 {
		return ""
	}
	chromeWidth := m.width - 4
	if chromeWidth < 0 {
		chromeWidth = 0
	}
	idx := m.mentionIndex
	if idx < 0 || idx >= len(m.mentionSuggestions) {
		idx = 0
	}
	rows := make([]string, 0, min(4, len(m.mentionSuggestions))+1)
	rows = append(rows, styles.PopupMutedStyle.Render("Mentions"))
	limit := min(4, len(m.mentionSuggestions))
	for i := 0; i < limit; i++ {
		line := m.mentionSuggestions[i]
		if len(line) > chromeWidth-2 && chromeWidth > 5 {
			line = truncate(line, chromeWidth-2)
		}
		if i == idx {
			rows = append(rows, styles.PopupSelectionStyle.Render(line))
		} else {
			rows = append(rows, styles.PopupFieldLabelStyle.Render(line))
		}
	}
	return lipgloss.NewStyle().MarginTop(1).Render(strings.Join(rows, "\n"))
}


func (m *ComposerModel) SetThinking(thinking bool) {
	m.thinking = thinking
}

func suggestionHasPlaceholder(value string) bool {
	return strings.Contains(value, "<") && strings.Contains(value, ">")
}

func shouldApplySuggestionOnEnter(current, suggestion string) bool {
	current = strings.TrimSpace(current)
	suggestion = strings.TrimSpace(suggestion)
	if current == "" || suggestion == "" || current == suggestion {
		return false
	}
	if suggestionHasPlaceholder(suggestion) {
		return false
	}
	return true
}

func composerActivityLabel(agentName string) string {
	return agentActivityLabel(agentName)
}

func (m ComposerModel) Height() int {
	height := m.textarea.Height() + 6 // base height: textarea(2) + suggestions(2) + chrome(4)
	if len(m.mentionSuggestions) > 0 {
		// MarginTop(1) = 1 line
		// Title "Mentions" = 1 line
		// Items = min(4, len) lines
		height += 2 + min(4, len(m.mentionSuggestions))
	}
	return height
}

func (m ComposerModel) Value() string {
	return m.textarea.Value()
}

func (m *ComposerModel) navigateHistoryUp() {
	if len(m.history) == 0 {
		return
	}
	if m.historyIndex == -1 {
		m.savedInput = m.textarea.Value()
		m.historyIndex = len(m.history) - 1
	} else if m.historyIndex > 0 {
		m.historyIndex--
	}
	m.textarea.SetValue(m.history[m.historyIndex])
	m.textarea.CursorEnd()
}

func (m *ComposerModel) navigateHistoryDown() {
	if m.historyIndex == -1 {
		return
	}
	if m.historyIndex == len(m.history)-1 {
		m.historyIndex = -1
		m.textarea.SetValue(m.savedInput)
		m.textarea.CursorEnd()
	} else {
		m.historyIndex++
		m.textarea.SetValue(m.history[m.historyIndex])
		m.textarea.CursorEnd()
	}
}
