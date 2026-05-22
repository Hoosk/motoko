package ui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/session"
	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/tachikoma"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type TachikomaMsg tachikoma.Update
type ShellResultMsg struct{ Result app.ShellResult }
type ProviderModelsMsg struct {
	Models []provider.ModelInfo
	Err    error
}
type AgentResultMsg struct {
	Prompt    string
	Result    app.Response
	Assistant string
	Err       error
}

type ThinkingTickMsg struct{}

type providerForm struct {
	active      bool
	fieldIndex  int
	presetIndex int
	name        string
	baseURL     string
	apiKey      string
	loading     bool
	status      string
}

type agentStreamBuffer struct {
	mu     sync.Mutex
	events []app.AgentStreamEvent
	done   bool
}

type Model struct {
	timeline           TimelineModel
	composer           ComposerModel
	footer             FooterModel
	providerForm       providerForm
	runtime            *app.Runtime
	manager            *tachikoma.Manager
	cancel             func()
	showTachikomas     bool
	showToolPalette    bool
	modePickerOpen     bool
	agentList          []agent.AgentDef
	agentListIndex     int
	modelPickerOpen    bool
	modelList          []provider.ModelInfo
	modelListIndex     int
	modelPickerLoading bool
	modelPickerStep    int // 0=model selection, 1=thinking level selection
	thinkingLevelIndex int
	sessionPickerOpen  bool
	sessionList        []*session.Session
	sessionListIndex   int
	sessionLoading     bool
	width              int
	height             int
	agentStream        *agentStreamBuffer
	tachikomaCtx       context.Context
}

func NewModel(runtime *app.Runtime, cancel func(), tachikomaCtx context.Context) Model {
	if tachikomaCtx == nil {
		tachikomaCtx = context.Background()
	}
	footer := NewFooterModel(runtime)
	footer.SetContextStats(runtime.HistoryInputTokens(), runtime.ContextWindow())
	return Model{
		timeline:     NewTimelineModel(),
		composer:     NewComposerModel(runtime),
		footer:       footer,
		runtime:      runtime,
		cancel:       cancel,
		tachikomaCtx: tachikomaCtx,
	}
}

func (m *Model) SetManager(mgr *tachikoma.Manager) { m.manager = mgr }

func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, m.composer.Init(), m.timeline.Init(), m.footer.Init())
	if entries := m.runtime.StartupEntries(); len(entries) > 0 {
		cmds = append(cmds, func() tea.Msg { return ResponseAppliedMsg{Response: app.Response{Entries: entries}} })
	}
	if m.manager != nil {
		cmds = append(cmds, waitForTachikoma(m.tachikomaCtx, m.manager))
	}
	return tea.Batch(cmds...)
}

func waitForTachikoma(ctx context.Context, manager *tachikoma.Manager) tea.Cmd {
	return func() tea.Msg {
		result := manager.Next(ctx)
		if !result.OK {
			return nil
		}
		return TachikomaMsg(result.Update)
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.providerForm.active {
			cmd := m.handleProviderFormKey(msg)
			return m, cmd
		}
		if m.modePickerOpen {
			cmd := m.handleModePickerKey(msg)
			return m, cmd
		}
		if m.sessionPickerOpen {
			cmd := m.handleSessionPickerKey(msg)
			return m, cmd
		}
		if m.modelPickerOpen {
			cmd := m.handleModelPickerKey(msg)
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

		if response.Signal == "open-mode-popup" {
			m.openModePicker()
			return m, tea.Batch(cmds...)
		}

		if response.Signal == "open-models-popup" {
			m.openModelPicker()
			active, ok := m.runtime.GetActiveProviderConfig()
			if ok {
				cmds = append(cmds, loadProviderModels(m.runtime, active))
			}
			return m, tea.Batch(cmds...)
		}

		if response.Signal == "open-sessions-popup" {
			m.openSessionPicker()
			cmds = append(cmds, loadSessions(m.runtime))
			return m, tea.Batch(cmds...)
		}

		if response.Action != nil {
			if response.Action.Type == app.ActionShell {
				cmds = append(cmds, runShellCommand(response.Action.ShellCommand))
			} else if response.Action.Type == app.ActionCompact {
				m.timeline.SetThinking(true)
				m.composer.SetThinking(true)
				m.footer.SetThinking(true)
				cmds = append(cmds,
					thinkingTick(),
					func() tea.Msg {
						return CompactResultMsg{Response: m.runtime.CompactSession(context.Background())}
					},
				)
			} else if response.Action.Type == app.ActionAgent {
				m.timeline.SetThinking(true)
				m.composer.SetThinking(true)
				m.footer.SetThinking(true)
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
			// Footer keeps thinking=true until AgentResultMsg (entire agent run).
			for _, event := range msg.Events {
				if !m.runtime.Debug() {
					switch event.Kind {
					case "output", "debug":
						continue
					case "tool":
						event.Content = ""
					}
				}
				if event.Kind == "compacting" || event.Kind == "status" {
					if strings.TrimSpace(event.Content) != "" {
						m.timeline.appendEntry(app.Entry{Kind: app.EntrySystem, Text: event.Content})
						m.timeline.renderMessages()
					}
					continue
				}
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
		if m.timeline.thinking || m.footer.thinking {
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
		m.footer.SetThinking(false)
		m.footer.SetContextStats(m.runtime.HistoryInputTokens(), m.runtime.ContextWindow())
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

	case ProviderModelsMsg:
		if m.modelPickerOpen {
			m.modelPickerLoading = false
			if msg.Err != nil {
				m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: msg.Err.Error()})
				m.timeline.renderMessages()
			} else {
				m.modelList = msg.Models
				m.modelListIndex = 0
			}
		} else {
			// Fall through to timeline.
			if cmd := m.timeline.Update(msg); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	case SessionsMsg:
		if m.sessionPickerOpen {
			m.sessionLoading = false
			if msg.Err != nil {
				m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: msg.Err.Error()})
				m.timeline.renderMessages()
			} else {
				m.sessionList = msg.Sessions
				m.sessionListIndex = 0
			}
		}

	case SessionLoadedMsg:
		if msg.Err != nil {
			cmds = append(cmds, func() tea.Msg {
				return ResponseAppliedMsg{Response: app.Response{Entries: []app.Entry{{Kind: app.EntryError, Text: msg.Err.Error()}}}}
			})
			break
		}
		cmds = append(cmds, func() tea.Msg { return ClearMessagesMsg{} })
		cmds = append(cmds, func() tea.Msg {
			return ResponseAppliedMsg{Response: app.Response{Entries: m.runtime.CurrentSessionEntries()}}
		})
		m.footer.SetContextStats(m.runtime.HistoryInputTokens(), m.runtime.ContextWindow())

	case CompactResultMsg:
		m.timeline.SetThinking(false)
		m.composer.SetThinking(false)
		m.footer.SetThinking(false)
		m.footer.SetContextStats(m.runtime.HistoryInputTokens(), m.runtime.ContextWindow())
		cmds = append(cmds, func() tea.Msg { return ResponseAppliedMsg{Response: msg.Response} })

	case ShellResultMsg:
		response := m.runtime.HandleShellResult(msg.Result)
		cmds = append(cmds, func() tea.Msg { return ResponseAppliedMsg{Response: response} })

	case AgentChangedMsg:
		m.timeline.appendEntry(app.Entry{Kind: app.EntrySystem, Text: "Modo agente: " + msg.Agent})
		m.timeline.renderMessages()

	case ModelChangedMsg:
		m.timeline.appendEntry(app.Entry{Kind: app.EntrySystem, Text: fmt.Sprintf("Modelo activo para %s: %s", msg.Provider, msg.Model)})
		m.timeline.renderMessages()

	case TachikomaMsg:
		if m.manager != nil {
			cmds = append(cmds, waitForTachikoma(m.tachikomaCtx, m.manager))
		}
	}

	if !m.providerForm.active && !m.modePickerOpen && !m.modelPickerOpen && !m.sessionPickerOpen {
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
	if m.modePickerOpen {
		popup := styles.PopupStyle.Render(m.renderModePicker())
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, popup)
	}
	if m.sessionPickerOpen {
		popup := styles.PopupStyle.Render(m.renderSessionPicker())
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, popup)
	}
	if m.modelPickerOpen {
		popup := styles.PopupStyle.Render(m.renderModelPicker())
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

func loadSessions(runtime *app.Runtime) tea.Cmd {
	return func() tea.Msg {
		sessions, err := runtime.ListSessions()
		return SessionsMsg{Sessions: sessions, Err: err}
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
