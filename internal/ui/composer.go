package ui

import (
	"fmt"
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
	suggestionSource   string
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
	ta.FocusedStyle.Text = lipgloss.NewStyle().Foreground(styles.White)
	ta.BlurredStyle.Text = lipgloss.NewStyle().Foreground(styles.White)
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(styles.Gray)
	ta.BlurredStyle.Placeholder = lipgloss.NewStyle().Foreground(styles.Gray)
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

func (m *ComposerModel) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
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
			return nil
		}

		switch msg.String() {
		case "tab", "right":
			if len(m.suggestions) > 0 {
				m.advanceSuggestion(1)
				return nil
			}
		case "shift+tab", "left":
			if len(m.suggestions) > 0 {
				m.advanceSuggestion(-1)
				return nil
			}
		case "down", "ctrl+n":
			m.clearSuggestionCycle()
			m.navigateHistoryDown()
			return nil
		case "up", "ctrl+p":
			m.clearSuggestionCycle()
			m.navigateHistoryUp()
			return nil
		case "enter":
			if len(m.suggestions) > 0 && strings.TrimSpace(m.textarea.Value()) != strings.TrimSpace(m.suggestions[m.selectedSuggestion]) && !suggestionHasPlaceholder(m.suggestions[m.selectedSuggestion]) {
				m.applySelectedSuggestion()
				return nil
			}
			input := m.textarea.Value()
			if strings.TrimSpace(input) != "" {
				m.history = append(m.history, input)
				m.historyIndex = -1
				m.savedInput = ""
				m.textarea.Reset()
				m.refreshSuggestions()
				return func() tea.Msg {
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

	return tea.Batch(cmds...)
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
	suggestions := m.renderSuggestionsLine()
	suggestionsBlock := lipgloss.NewStyle().MarginTop(1).Render(suggestions)

	body := lipgloss.JoinHorizontal(lipgloss.Top, promptBlock, styles.InputStyle.Render(m.textarea.View()))

	return styles.InputChromeStyle.Width(m.width - 6).Render(
		lipgloss.JoinVertical(lipgloss.Left, body, suggestionsBlock),
	)
}

func (m *ComposerModel) SyncLayout(width, height int) {
	m.width = width
	m.height = height
	m.syncLayout()
}

func (m *ComposerModel) syncLayout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	// Overhead: MainContainerStyle.Padding(0,1)=2 + Chrome border H=2 + Chrome Padding(1,1) H=2 + promptBlock=3 = 9
	textareaWidth := max(16, m.width-9)
	m.textarea.SetWidth(textareaWidth)
	m.textarea.SetHeight(2)
}

func (m *ComposerModel) refreshSuggestions() {
	m.syncInputChrome()
	if m.thinking {
		m.suggestions = nil
		m.selectedSuggestion = 0
		return
	}
	if mentionSuggestions := m.runtime.MentionSuggestions(m.textarea.Value()); len(mentionSuggestions) > 0 {
		m.suggestions = mentionSuggestions
		m.suggestionSource = "mention"
	} else {
		m.suggestions = m.runtime.Completions(m.textarea.Value())
		m.suggestionSource = "default"
	}
	m.suggestionBase = nil
	if len(m.suggestions) == 0 {
		m.selectedSuggestion = 0
		return
	}
	if m.selectedSuggestion >= len(m.suggestions) {
		m.selectedSuggestion = len(m.suggestions) - 1
	}
	if m.selectedSuggestion < 0 {
		m.selectedSuggestion = 0
	}
}

func (m *ComposerModel) applySelectedSuggestion() {
	if len(m.suggestions) == 0 {
		return
	}
	m.textarea.SetValue(m.suggestions[m.selectedSuggestion])
	if m.suggestionSource == "mention" {
		m.textarea.SetValue(m.runtime.ReplaceTrailingMention(m.textarea.Value(), m.suggestions[m.selectedSuggestion]))
	} else {
		m.textarea.SetValue(m.suggestions[m.selectedSuggestion])
	}
	m.textarea.CursorEnd()
}

func (m *ComposerModel) advanceSuggestion(step int) {
	if len(m.suggestions) == 0 {
		return
	}
	if len(m.suggestionBase) == 0 {
		m.suggestionBase = append([]string(nil), m.suggestions...)
	}
	m.suggestions = append([]string(nil), m.suggestionBase...)
	current := strings.TrimSpace(m.textarea.Value())
	selected := strings.TrimSpace(m.suggestions[m.selectedSuggestion])
	if current == "" || current == selected {
		m.selectedSuggestion = (m.selectedSuggestion + step + len(m.suggestions)) % len(m.suggestions)
	}
	m.applySelectedSuggestion()
}

func (m *ComposerModel) clearSuggestionCycle() {
	m.suggestionBase = nil
	m.suggestionSource = ""
}

func (m *ComposerModel) syncInputChrome() {
	if m.runtime.InputMode() == app.InputModeShell {
		m.textarea.Placeholder = "Modo shell activo: escribe un comando o /chat para salir"
		return
	}
	m.textarea.Placeholder = "Escribe un prompt, /tool ..., o !comando"
}

func (m ComposerModel) renderInputPrompt() string {
	if m.runtime.InputMode() == app.InputModeShell {
		return lipgloss.NewStyle().Foreground(styles.WarmGold).Bold(true).Render("$")
	}
	return lipgloss.NewStyle().Foreground(styles.MainNeon).Bold(true).Render(">")
}

func (m ComposerModel) renderSuggestionsLine() string {
	statusWidth := 22
	status := lipgloss.NewStyle().Width(statusWidth).Align(lipgloss.Left).Render("")
	if m.thinking {
		status = lipgloss.NewStyle().Width(statusWidth).Align(lipgloss.Left).Render(styles.InputHintStyle.Render("[" + composerActivityLabel(m.runtime.AgentName()) + "]"))
	}
	var detail string
	if len(m.suggestions) == 0 {
		if m.runtime.InputMode() == app.InputModeShell {
			detail = styles.InputHintStyle.Render("Shell directo activo. Enter ejecuta. /chat sale. Ctrl+T abre la paleta.")
			return lipgloss.JoinHorizontal(lipgloss.Left, detail, status)
		}
		detail = styles.InputHintStyle.Render("Tab rota y completa. /provider add abre el formulario. /models tiene autocompletado con modelos cacheados.")
		return lipgloss.JoinHorizontal(lipgloss.Left, detail, status)
	}
	limit := min(3, len(m.suggestions))
	items := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		if i == m.selectedSuggestion {
			items = append(items, styles.SelectionStyle.Render(m.suggestions[i]))
			continue
		}
		items = append(items, styles.SuggestionStyle.Render(m.suggestions[i]))
	}
	detail = strings.Join(items, "   ")
	return lipgloss.JoinHorizontal(lipgloss.Left, detail, status)
}

func (m *ComposerModel) SetThinking(thinking bool) {
	m.thinking = thinking
}

func suggestionHasPlaceholder(value string) bool {
	return strings.Contains(value, "<") && strings.Contains(value, ">")
}

func composerActivityLabel(agentName string) string {
	return fmt.Sprintf("%s", agentActivityLabel(agentName))
}

func (m ComposerModel) Height() int {
	return m.textarea.Height() + 6
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
