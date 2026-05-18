package ui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tachikoma"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type TachikomaMsg tachikoma.Update

type ShellResultMsg struct{ Result app.ShellResult }
type ProviderModelsMsg struct {
	Models []string
	Err    error
}
type AgentResultMsg struct {
	Prompt string
	Result agent.Result
	Err    error
}
type AgentStreamChunkMsg struct {
	Chunk string
	Done  bool
}
type ThinkingTickMsg struct{}
type CopySelectionMsg struct{ Err error }

type providerForm struct {
	active     bool
	fieldIndex int
	kindIndex  int
	apiKey     string
	loading    bool
	status     string
}

type Model struct {
	viewport           viewport.Model
	viewportContent    string
	textarea           textarea.Model
	messages           []string
	runtime            *app.Runtime
	tachikomaInfo      map[string]string
	manager            *tachikoma.Manager
	cancel             func()
	sysInfo            system.ContextInfo
	showTachikomas     bool
	showToolPalette    bool
	ready              bool
	width              int
	height             int
	suggestions        []string
	selectedSuggestion int
	providerForm       providerForm
	thinking           bool
	thinkingFrame      int
	pendingPrompt      string
	autoScroll         bool
	streaming          bool
	streamedRunes      []rune
	streamMessageIndex int
	agentStream        <-chan string
	selectedMessage    int
}

const logoArt = `
  __  __  ____ _____ ____  _  _____
 |  \/  |/ __ \_   _/ __ \| |/ / _ \
 | \  / | |  | || || |  | | ' / | | |
 | |\/| | |  | || || |  | |  <| | | |
 | |  | | |__| || || |__| | . \ |_| |
 |_|  |_|\____/ |_| \____/|_|\_\___/
`

var thinkingFrames = []string{"thinking.", "thinking..", "thinking..."}

func NewModel(runtime *app.Runtime, cancel func()) Model {
	ta := textarea.New()
	ta.Placeholder = "Escribe un prompt, /tool ..., o !comando"
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

	vp := viewport.New(80, 20)
	m := Model{
		textarea:        ta,
		viewport:        vp,
		runtime:         runtime,
		tachikomaInfo:   make(map[string]string),
		cancel:          cancel,
		sysInfo:         system.GetContextInfo(),
		autoScroll:      true,
		selectedMessage: -1,
	}
	m.syncInputChrome()
	m.resetMessages()
	m.refreshSuggestions()
	return m
}

func (m *Model) SetManager(mgr *tachikoma.Manager) { m.manager = mgr }

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textarea.Blink}
	if m.manager != nil {
		cmds = append(cmds, waitForTachikoma(m.manager.Updates()))
	}
	return tea.Batch(cmds...)
}

func waitForTachikoma(updates <-chan tachikoma.Update) tea.Cmd {
	return func() tea.Msg { return TachikomaMsg(<-updates) }
}

func runShellCommand(command string) tea.Cmd {
	return func() tea.Msg { return ShellResultMsg{Result: app.RunShellCommand(context.Background(), command)} }
}

func loadProviderModels(runtime *app.Runtime, cfg config.ProviderConfig) tea.Cmd {
	return func() tea.Msg {
		models, err := runtime.ListModelsForProvider(context.Background(), cfg)
		return ProviderModelsMsg{Models: models, Err: err}
	}
}

func waitAgentStream(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		chunk, ok := <-ch
		if !ok {
			return AgentStreamChunkMsg{Done: true}
		}
		return AgentStreamChunkMsg{Chunk: chunk}
	}
}

func thinkingTick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return ThinkingTickMsg{} })
}

func copySelection(text string) tea.Cmd {
	return func() tea.Msg {
		return CopySelectionMsg{Err: writeClipboard(text)}
	}
}

func writeClipboard(text string) error {
	if err := clipboard.WriteAll(text); err == nil {
		return nil
	}
	commands := [][]string{
		{"wl-copy"},
		{"xclip", "-selection", "clipboard"},
		{"xsel", "--clipboard", "--input"},
	}
	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("no se pudo copiar: instala wl-copy, xclip o xsel si el backend actual falla")
}

func (m *Model) resetMessages() {
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

func (m *Model) refreshContext() {
	signals := m.sysInfo.Signals
	m.sysInfo = system.GetContextInfo()
	if len(signals) > 0 {
		m.sysInfo.Signals = signals
	}
}

func (m *Model) refreshSuggestions() {
	m.syncInputChrome()
	if m.providerForm.active || m.thinking {
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

func (m *Model) syncInputChrome() {
	if m.runtime.InputMode() == app.InputModeShell {
		m.textarea.Placeholder = "Modo shell activo: escribe un comando o /chat para salir"
		m.textarea.Prompt = ""
		return
	}
	m.textarea.Placeholder = "Escribe un prompt, /tool ..., o !comando"
	m.textarea.Prompt = ""
}

func (m *Model) applySelectedSuggestion() {
	if len(m.suggestions) == 0 {
		return
	}
	m.textarea.SetValue(m.suggestions[m.selectedSuggestion])
	m.textarea.CursorEnd()
	m.refreshSuggestions()
}

func (m *Model) renderMessages() {
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
			// Left-bar indicator: border takes 1 char, so content = width-1
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

func (m *Model) syncLayout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	textareaWidth := max(16, m.width-13)
	m.textarea.SetWidth(textareaWidth)
	inputHeight := clamp(estimateTextareaHeight(m.textarea.Value(), textareaWidth), 3, max(3, m.height-20))
	m.textarea.SetHeight(inputHeight)
	// Total fixed overhead: timeline_border(2) + composer_border(2)
	// + composer_padding_v(2) + suggestions(1) + footer(1) = 8
	viewportHeight := max(6, m.height-(inputHeight+8))
	if !m.ready {
		m.viewport = viewport.New(m.width-6, viewportHeight)
		m.ready = true
		return
	}
	m.viewport.Width = m.width - 6
	m.viewport.Height = viewportHeight
}

func (m Model) maxViewportOffset() int {
	if m.viewport.Height <= 0 || m.viewportContent == "" {
		return 0
	}
	lineCount := strings.Count(m.viewportContent, "\n") + 1
	return max(0, lineCount-m.viewport.Height)
}

func (m *Model) appendEntry(entry app.Entry) {
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

func (m *Model) applyResponse(response app.Response) tea.Cmd {
	if response.Clear {
		m.resetMessages()
	}
	for _, entry := range response.Entries {
		m.appendEntry(entry)
	}
	if response.Signal == "open-provider-popup" {
		m.openProviderForm()
		return nil
	}
	if response.Action == nil {
		return nil
	}
	if response.Action.Type == app.ActionShell {
		return runShellCommand(response.Action.ShellCommand)
	}
	if response.Action.Type == app.ActionAgent {
		m.thinking = true
		m.thinkingFrame = 0
		m.pendingPrompt = response.Action.AgentPrompt
		m.streaming = true
		m.streamedRunes = nil
		m.streamMessageIndex = -1
		m.agentStream = nil
		m.renderMessages()
		streamCh := make(chan string, 64)
		m.agentStream = streamCh
		cmd := func() tea.Msg {
			result, err := m.runtime.RunAgentStream(context.Background(), m.sysInfo, response.Action.AgentPrompt, func(event app.AgentStreamEvent) error {
				if event.Kind == "assistant_delta" && event.Content != "" {
					streamCh <- event.Content
				}
				return nil
			})
			close(streamCh)
			return AgentResultMsg{Prompt: response.Action.AgentPrompt, Result: result, Err: err}
		}
		return tea.Batch(cmd, waitAgentStream(streamCh), thinkingTick())
	}
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var taCmd, vpCmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.providerForm.active {
			cmd := m.handleProviderFormKey(msg)
			m.refreshSuggestions()
			return m, cmd
		}
		switch msg.String() {
		case "ctrl+c", "esc":
			if m.showToolPalette {
				m.showToolPalette = false
				return m, nil
			}
			m.cancel()
			return m, tea.Quit
		case "alt+t":
			m.showTachikomas = !m.showTachikomas
			return m, nil
		case "ctrl+t":
			m.showToolPalette = !m.showToolPalette
			return m, nil
		case "tab":
			if len(m.suggestions) > 0 {
				m.applySelectedSuggestion()
				return m, nil
			}
		case "right":
			if len(m.suggestions) > 0 {
				m.applySelectedSuggestion()
				return m, nil
			}
		case "down", "ctrl+n":
			if len(m.suggestions) > 0 {
				m.selectedSuggestion = (m.selectedSuggestion + 1) % len(m.suggestions)
				return m, nil
			}
		case "up", "ctrl+p":
			if len(m.suggestions) > 0 {
				m.selectedSuggestion--
				if m.selectedSuggestion < 0 {
					m.selectedSuggestion = len(m.suggestions) - 1
				}
				return m, nil
			}
		case "alt+up":
			if len(m.messages) > 0 {
				if m.selectedMessage < 0 {
					m.selectedMessage = len(m.messages) - 1
				} else {
					m.selectedMessage = clamp(m.selectedMessage-1, 0, len(m.messages)-1)
				}
				m.renderMessages()
			}
			return m, nil
		case "alt+down":
			if len(m.messages) > 0 {
				if m.selectedMessage < 0 {
					m.selectedMessage = 0
				} else {
					m.selectedMessage = clamp(m.selectedMessage+1, 0, len(m.messages)-1)
				}
				m.renderMessages()
			}
			return m, nil
		case "alt+c":
			if m.selectedMessage >= 0 && m.selectedMessage < len(m.messages) {
				return m, copySelection(stripANSI(m.messages[m.selectedMessage]))
			}
		case "c":
			if m.selectedMessage >= 0 && strings.TrimSpace(m.textarea.Value()) == "" {
				return m, copySelection(stripANSI(m.messages[m.selectedMessage]))
			}
		case "enter":
			if m.showToolPalette {
				m.showToolPalette = false
				return m, nil
			}
			if m.thinking || m.streaming {
				return m, nil
			}
			if len(m.suggestions) > 0 && strings.TrimSpace(m.textarea.Value()) != strings.TrimSpace(m.suggestions[m.selectedSuggestion]) {
				m.applySelectedSuggestion()
				return m, nil
			}
			input := m.textarea.Value()
			if strings.TrimSpace(input) != "" {
				response := m.runtime.HandleInput(input, m.sysInfo)
				m.textarea.Reset()
				cmds = append(cmds, m.applyResponse(response))
				m.refreshContext()
				m.refreshSuggestions()
				m.renderMessages()
				return m, tea.Batch(cmds...)
			}
			// Empty input: copy selected message if any
			if m.selectedMessage >= 0 && m.selectedMessage < len(m.messages) {
				return m, copySelection(stripANSI(m.messages[m.selectedMessage]))
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.syncLayout()
		m.renderMessages()

	case TachikomaMsg:
		m.tachikomaInfo[msg.Name] = msg.Status
		if m.sysInfo.Signals == nil {
			m.sysInfo.Signals = make(map[string]string)
		}
		m.sysInfo.Signals[msg.Name] = msg.Status
		m.refreshContext()
		if m.sysInfo.Signals == nil {
			m.sysInfo.Signals = make(map[string]string)
		}
		for name, status := range m.tachikomaInfo {
			m.sysInfo.Signals[name] = status
		}
		if m.manager != nil {
			cmds = append(cmds, waitForTachikoma(m.manager.Updates()))
		}

	case ShellResultMsg:
		m.refreshContext()
		cmds = append(cmds, m.applyResponse(m.runtime.HandleShellResult(msg.Result)))
		m.renderMessages()

	case ProviderModelsMsg:
		for _, entry := range entriesForProviderModels(msg.Models, msg.Err) {
			m.appendEntry(entry)
		}
		m.renderMessages()

	case AgentResultMsg:
		m.thinking = false
		m.pendingPrompt = ""
		if msg.Err != nil {
			m.streaming = false
			m.appendEntry(app.Entry{Kind: app.EntryError, Text: msg.Err.Error()})
		} else {
			entries := entriesForAgentResult(msg.Result, m.runtime.Debug())
			for _, entry := range entries {
				if entry.Kind == app.EntryAssistant {
					if m.streamMessageIndex >= 0 && m.streamMessageIndex < len(m.messages) {
						m.messages[m.streamMessageIndex] = styles.AssistantBlockStyle.Render(entry.Text)
						continue
					}
				}
				m.appendEntry(entry)
			}
			m.streaming = false
			m.streamedRunes = nil
			m.streamMessageIndex = -1
		}
		m.renderMessages()

	case AgentStreamChunkMsg:
		if m.streaming {
			if m.streamMessageIndex == -1 {
				m.appendEntry(app.Entry{Kind: app.EntryAssistant, Text: ""})
				m.streamMessageIndex = len(m.messages) - 1
			}
			if msg.Chunk != "" {
				m.streamedRunes = append(m.streamedRunes, []rune(msg.Chunk)...)
				if m.streamMessageIndex >= 0 && m.streamMessageIndex < len(m.messages) {
					m.messages[m.streamMessageIndex] = styles.AssistantBlockStyle.Render(string(m.streamedRunes))
				}
				m.renderMessages()
				if m.agentStream != nil {
					cmds = append(cmds, waitAgentStream(m.agentStream))
				}
			}
			if msg.Done {
				m.agentStream = nil
			}
		}

	case ThinkingTickMsg:
		if m.thinking {
			m.thinkingFrame = (m.thinkingFrame + 1) % len(thinkingFrames)
			m.renderMessages()
			cmds = append(cmds, thinkingTick())
		}

	case CopySelectionMsg:
		if msg.Err != nil {
			m.appendEntry(app.Entry{Kind: app.EntryError, Text: "clipboard: " + msg.Err.Error()})
		} else {
			m.appendEntry(app.Entry{Kind: app.EntrySystem, Text: "copiado al portapapeles"})
		}
		m.renderMessages()
	}

	if !m.providerForm.active {
		m.textarea, taCmd = m.textarea.Update(msg)
		m.syncLayout()
		m.viewport, vpCmd = m.viewport.Update(msg)
		m.autoScroll = m.viewport.YOffset >= m.maxViewportOffset()
		cmds = append(cmds, taCmd, vpCmd)
	}
	m.refreshSuggestions()
	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if m.width == 0 {
		return "inicializando..."
	}
	timeline := styles.TimelineStyle.Width(m.width - 4).Height(m.viewport.Height).Render(m.viewport.View())
	composer := m.renderComposer()
	footer := m.renderFooter()
	base := styles.MainContainerStyle.Render(lipgloss.JoinVertical(lipgloss.Left, timeline, composer, footer))
	if m.providerForm.active {
		popup := styles.PopupStyle.Render(m.renderProviderForm())
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, popup)
	}
	if !m.showToolPalette {
		return base
	}
	popup := styles.PopupStyle.Render(m.renderToolPalette())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, popup)
}
