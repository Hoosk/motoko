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
	selectedSuggestion int
	runtime            *app.Runtime
	width              int
	height             int
	thinking           bool
}

func NewComposerModel(runtime *app.Runtime) ComposerModel {
	ta := textarea.New()
	ta.Focus()
	ta.Prompt = ""
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Text = lipgloss.NewStyle().Foreground(styles.White)
	ta.BlurredStyle.Text = lipgloss.NewStyle().Foreground(styles.White)
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(styles.Gray)
	ta.BlurredStyle.Placeholder = lipgloss.NewStyle().Foreground(styles.Gray)
	ta.EndOfBufferCharacter = ' '

	m := ComposerModel{
		textarea: ta,
		runtime:  runtime,
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
				m.applySelectedSuggestion()
				return nil
			}
		case "down", "ctrl+n":
			if len(m.suggestions) > 0 {
				m.selectedSuggestion = (m.selectedSuggestion + 1) % len(m.suggestions)
				return nil
			}
		case "up", "ctrl+p":
			if len(m.suggestions) > 0 {
				m.selectedSuggestion--
				if m.selectedSuggestion < 0 {
					m.selectedSuggestion = len(m.suggestions) - 1
				}
				return nil
			}
		case "enter":
			if len(m.suggestions) > 0 && strings.TrimSpace(m.textarea.Value()) != strings.TrimSpace(m.suggestions[m.selectedSuggestion]) {
				m.applySelectedSuggestion()
				return nil
			}
			input := m.textarea.Value()
			if strings.TrimSpace(input) != "" {
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
	
	if _, ok := msg.(tea.KeyMsg); ok {
		m.refreshSuggestions()
	}

	return tea.Batch(cmds...)
}

func (m ComposerModel) View() string {
	if m.width == 0 {
		return ""
	}
	prompt := m.renderInputPrompt()
	rows := max(3, m.textarea.Height())
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

	return styles.InputChromeStyle.Width(m.width - 4).Render(
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
	textareaWidth := max(16, m.width-13)
	m.textarea.SetWidth(textareaWidth)
	inputHeight := clamp(estimateTextareaHeight(m.textarea.Value(), textareaWidth), 3, max(3, m.height-20))
	m.textarea.SetHeight(inputHeight)
}

func (m *ComposerModel) refreshSuggestions() {
	m.syncInputChrome()
	if m.thinking {
		m.suggestions = nil
		m.selectedSuggestion = 0
		return
	}
	m.suggestions = m.runtime.Completions(m.textarea.Value())
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
	m.textarea.CursorEnd()
	m.refreshSuggestions()
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
	if m.thinking {
		return styles.InputHintStyle.Render("Esperando respuesta del agente...")
	}
	if len(m.suggestions) == 0 {
		if m.runtime.InputMode() == app.InputModeShell {
			return styles.InputHintStyle.Render("Shell directo activo. Enter ejecuta. /chat sale. Ctrl+T abre la paleta.")
		}
		return styles.InputHintStyle.Render("Tab completa. /provider add abre el formulario. /models tiene autocompletado con modelos cacheados.")
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
	return strings.Join(items, "   ")
}

func (m *ComposerModel) SetThinking(thinking bool) {
	m.thinking = thinking
}

func (m ComposerModel) Height() int {
	return m.textarea.Height() + 2
}

func (m ComposerModel) Value() string {
	return m.textarea.Value()
}
