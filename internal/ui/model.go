package ui

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/tachikoma"
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
	Result app.Response
	Assistant string
	Err    error
}

type ThinkingTickMsg struct{}

type providerForm struct {
	active     bool
	fieldIndex int
	kindIndex  int
	apiKey     string
	loading    bool
	status     string
}

type agentStreamBuffer struct {
	mu     sync.Mutex
	events []app.AgentStreamEvent
	done   bool
}

type Model struct {
	timeline        TimelineModel
	composer        ComposerModel
	footer          FooterModel
	providerForm    providerForm
	runtime         *app.Runtime
	manager         *tachikoma.Manager
	cancel          func()
	showTachikomas  bool
	showToolPalette bool
	width           int
	height          int
	agentStream     *agentStreamBuffer
}

func NewModel(runtime *app.Runtime, cancel func()) Model {
	return Model{
		timeline: NewTimelineModel(),
		composer: NewComposerModel(runtime),
		footer:   NewFooterModel(runtime),
		runtime:  runtime,
		cancel:   cancel,
	}
}

func (m *Model) SetManager(mgr *tachikoma.Manager) { m.manager = mgr }

func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, m.composer.Init(), m.timeline.Init(), m.footer.Init())
	if m.manager != nil {
		cmds = append(cmds, waitForTachikoma(m.manager.Updates()))
	}
	return tea.Batch(cmds...)
}

func waitForTachikoma(updates <-chan tachikoma.Update) tea.Cmd {
	return func() tea.Msg { return TachikomaMsg(<-updates) }
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.providerForm.active {
			cmd := m.handleProviderFormKey(msg)
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
		case "c":
			if m.composer.Value() == "" {
				cmds = append(cmds, m.timeline.CopySelected())
				return m, tea.Batch(cmds...)
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.syncLayout()

	case SubmitPromptMsg:
		response := m.runtime.HandleInput(msg.Prompt, m.footer.GetSysInfo())
		
		if response.Clear {
			cmds = append(cmds, func() tea.Msg { return ClearMessagesMsg{} })
		}
		
		cmds = append(cmds, func() tea.Msg { return ResponseAppliedMsg{Response: response} })

		if response.Signal == "open-provider-popup" {
			m.openProviderForm()
			return m, tea.Batch(cmds...)
		}

		if response.Action != nil {
			if response.Action.Type == app.ActionShell {
				cmds = append(cmds, runShellCommand(response.Action.ShellCommand))
			} else if response.Action.Type == app.ActionAgent {
				m.timeline.SetThinking(true)
				m.composer.SetThinking(true)
				m.timeline.SetStreaming(true)
				m.agentStream = &agentStreamBuffer{}
				
				cmds = append(cmds, 
					waitAgentStream(m.agentStream), 
					thinkingTick(),
					func() tea.Msg {
						result, err := m.runtime.RunAgentStream(context.Background(), m.footer.GetSysInfo(), response.Action.AgentPrompt, func(event app.AgentStreamEvent) error {
							m.agentStream.push(event)
							return nil
						})
						m.agentStream.finish()
						
						resp := app.Response{}
						resp.Entries = entriesForAgentResult(result, m.runtime.Debug())
						return AgentResultMsg{Prompt: response.Action.AgentPrompt, Result: resp, Assistant: result.Assistant, Err: err}
					},
				)
			}
		}
		
	case AgentStreamBatchMsg:
		if len(msg.Events) > 0 {
			m.timeline.SetThinking(false)
			m.composer.SetThinking(false)
			for _, event := range msg.Events {
				if cmd := m.timeline.Update(AgentStreamEventMsg{Event: event}); cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}
		if !msg.Done && m.agentStream != nil {
			cmds = append(cmds, waitAgentStream(m.agentStream))
		} else if msg.Done {
			m.agentStream = nil
		}

	case ThinkingTickMsg:
		if m.timeline.thinking {
			cmds = append(cmds, thinkingTick())
		}

	case AgentResultMsg:
		if strings.TrimSpace(msg.Assistant) != "" {
			if cmd := m.timeline.Update(finalizeStreamMsg{Text: msg.Assistant}); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		m.timeline.SetThinking(false)
		m.composer.SetThinking(false)
		m.timeline.SetStreaming(false)
		
		if msg.Err != nil {
			cmds = append(cmds, func() tea.Msg { 
				return ResponseAppliedMsg{Response: app.Response{Entries: []app.Entry{{Kind: app.EntryError, Text: msg.Err.Error()}}}} 
			})
		} else {
			cmds = append(cmds, func() tea.Msg { 
				return ResponseAppliedMsg{Response: msg.Result} 
			})
		}

	case ShellResultMsg:
		response := m.runtime.HandleShellResult(msg.Result)
		cmds = append(cmds, func() tea.Msg { return ResponseAppliedMsg{Response: response} })

	case TachikomaMsg:
		if m.manager != nil {
			cmds = append(cmds, waitForTachikoma(m.manager.Updates()))
		}
	}

	if !m.providerForm.active {
		cmds = append(cmds, m.composer.Update(msg))
		cmds = append(cmds, m.timeline.Update(msg))
		cmds = append(cmds, m.footer.Update(msg))
		m.syncLayout() // Recalculate layout in case sub-models changed height
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if m.width == 0 {
		return "inicializando..."
	}

	timelineView := m.timeline.View()
	composerView := m.composer.View()
	footerView := m.footer.View()

	base := styles.MainContainerStyle.Render(lipgloss.JoinVertical(lipgloss.Left, timelineView, composerView, footerView))

	if m.providerForm.active {
		popup := styles.PopupStyle.Render(m.renderProviderForm())
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, popup)
	}
	if !m.showToolPalette {
		return base
	}
	popup := styles.PopupStyle.Render(renderToolPalette(m.runtime.ToolSpecs(), m.showTachikomas, m.footer.tachikomaInfo))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, popup)
}

func (m *Model) syncLayout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	
	composerHeight := m.composer.Height()
	viewportHeight := max(6, m.height-(composerHeight+5))
	
	m.composer.SyncLayout(m.width, composerHeight)
	m.timeline.SyncLayout(m.width, viewportHeight)
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

func (b *agentStreamBuffer) push(event app.AgentStreamEvent) {
	b.mu.Lock()
	b.events = append(b.events, event)
	b.mu.Unlock()
}

func (b *agentStreamBuffer) finish() {
	b.mu.Lock()
	b.done = true
	b.mu.Unlock()
}

func (b *agentStreamBuffer) drain() ([]app.AgentStreamEvent, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.events) == 0 {
		return nil, b.done
	}
	events := append([]app.AgentStreamEvent(nil), b.events...)
	b.events = nil
	return events, b.done
}

func waitAgentStream(buffer *agentStreamBuffer) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(16 * time.Millisecond)
		events, done := buffer.drain()
		return AgentStreamBatchMsg{Events: events, Done: done}
	}
}

func thinkingTick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return ThinkingTickMsg{} })
}
