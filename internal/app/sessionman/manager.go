package sessionman

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/brain"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/session"

	"github.com/Hoosk/motoko/internal/app/types"
)

type Manager struct {
	brainInitErr   error
	currentSession *session.Session
	brain          *brain.Brain
	workspaceID    string
	wasResumed     bool
}

func NewManager(workspaceID string) *Manager {
	return &Manager{workspaceID: workspaceID}
}

func (m *Manager) WorkspaceID() string                    { return m.workspaceID }
func (m *Manager) CurrentSession() *session.Session       { return m.currentSession }
func (m *Manager) Brain() *brain.Brain                    { return m.brain }
func (m *Manager) BrainInitErr() error                    { return m.brainInitErr }
func (m *Manager) WasResumed() bool                       { return m.wasResumed }
func (m *Manager) SetCurrentSession(s *session.Session)   { m.currentSession = s }
func (m *Manager) SetBrain(b *brain.Brain, initErr error) { m.brain = b; m.brainInitErr = initErr }
func (m *Manager) SetWasResumed(v bool)                   { m.wasResumed = v }

func (m *Manager) ListSessions() ([]*session.Session, error) {
	return session.List(m.workspaceID)
}

func (m *Manager) LoadSession(id string) error {
	s, err := session.Load(m.workspaceID, id)
	if err != nil {
		return err
	}
	m.currentSession = s
	m.brain, m.brainInitErr = brain.New(m.workspaceID, s.ID)
	if m.brainInitErr != nil {
		return fmt.Errorf("failed to initialize session brain: %w", m.brainInitErr)
	}
	return nil
}

func (m *Manager) CurrentSessionEntries() []types.Entry {
	if m.currentSession == nil || len(m.currentSession.History) == 0 {
		return nil
	}
	entries := make([]types.Entry, 0, len(m.currentSession.History))
	for _, msg := range m.currentSession.History {
		if _, ok := provider.ParseAssistantToolCallContent(msg.Content); ok {
			continue
		}
		switch msg.Role {
		case provider.RoleUser:
			entries = append(entries, types.Entry{Kind: types.EntryUser, Text: msg.Content})
		case provider.RoleAssistant:
			entries = append(entries, types.Entry{Kind: types.EntryAssistant, Text: msg.Content})
		case provider.RoleTool:
			_, output := provider.ParseToolResultContent(msg.Content)
			if strings.TrimSpace(output) != "" {
				entries = append(entries, types.Entry{Kind: types.EntrySystem, Text: output})
			}
		default:
			entries = append(entries, types.Entry{Kind: types.EntrySystem, Text: msg.Content})
		}
	}
	return entries
}

func (m *Manager) CompactSession(ctx context.Context, cfg *config.AppConfig, providerFn func(config.ProviderConfig) (provider.Client, error), cw int) types.Response {
	if err := m.doCompact(ctx, cfg, providerFn, cw); err != nil {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: err.Error()}}}
	}
	return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Session compacted."}}}
}

func (m *Manager) PersistTurn(result agent.Result) {
	if m.currentSession == nil {
		workspacePath, _ := os.Getwd()
		m.currentSession = session.New(m.workspaceID, workspacePath)
	}
	m.currentSession.History = append([]provider.ConversationItem(nil), result.History...)
	m.currentSession.LastInputTokens = result.Usage.InputTokens

	m.currentSession.TotalInputTokens += result.Usage.InputTokens
	m.currentSession.TotalOutputTokens += result.Usage.OutputTokens
	m.currentSession.TotalTokens += result.Usage.TotalTokens
	m.currentSession.TotalReasoningTokens += result.Usage.ReasoningTokens
	m.currentSession.TotalCacheReadTokens += result.Usage.CacheReadInputTokens
	m.currentSession.TotalCacheWriteTokens += result.Usage.CacheWriteInputTokens

	totalChars := result.Usage.SystemStaticChars + result.Usage.SystemDynamicChars + result.Usage.ToolsChars + result.Usage.HistoryChars
	if totalChars > 0 && result.Usage.InputTokens > 0 {
		inputTokens := result.Usage.InputTokens
		m.currentSession.LastSystemStaticTokens = int(float64(result.Usage.SystemStaticChars) / float64(totalChars) * float64(inputTokens))
		m.currentSession.LastSystemDynamicTokens = int(float64(result.Usage.SystemDynamicChars) / float64(totalChars) * float64(inputTokens))
		m.currentSession.LastToolsTokens = int(float64(result.Usage.ToolsChars) / float64(totalChars) * float64(inputTokens))
		m.currentSession.LastHistoryTokens = int(float64(result.Usage.HistoryChars) / float64(totalChars) * float64(inputTokens))

		sumEst := m.currentSession.LastSystemStaticTokens + m.currentSession.LastSystemDynamicTokens + m.currentSession.LastToolsTokens + m.currentSession.LastHistoryTokens
		diff := inputTokens - sumEst
		if diff != 0 {
			m.currentSession.LastSystemStaticTokens += diff
		}

		m.currentSession.TotalSystemStaticTokens += m.currentSession.LastSystemStaticTokens
		m.currentSession.TotalSystemDynamicTokens += m.currentSession.LastSystemDynamicTokens
		m.currentSession.TotalToolsTokens += m.currentSession.LastToolsTokens
		m.currentSession.TotalHistoryTokens += m.currentSession.LastHistoryTokens
	} else {
		m.currentSession.LastSystemStaticTokens = 0
		m.currentSession.LastSystemDynamicTokens = 0
		m.currentSession.LastToolsTokens = 0
		m.currentSession.LastHistoryTokens = 0
	}

	_ = m.currentSession.Save()
}

func (m *Manager) MaybeAutoCompact(ctx context.Context, onEvent func(types.AgentStreamEvent) error, cfg *config.AppConfig, providerFn func(config.ProviderConfig) (provider.Client, error), cw int) error {
	if m.currentSession == nil || cw <= 0 || m.currentSession.LastInputTokens <= 0 {
		return nil
	}
	if float64(m.currentSession.LastInputTokens)/float64(cw) < 0.80 {
		return nil
	}
	if onEvent != nil {
		_ = onEvent(types.AgentStreamEvent{Kind: "compacting", Content: "Compacting session..."})
	}
	err := m.doCompact(ctx, cfg, providerFn, cw)
	if err == nil && onEvent != nil {
		_ = onEvent(types.AgentStreamEvent{Kind: "status", Content: "Session auto-compacted."})
	}
	return err
}

func (m *Manager) SessionTitle() string {
	if m.currentSession == nil {
		return ""
	}
	return strings.TrimSpace(m.currentSession.Title)
}

func (m *Manager) StartupEntries() []types.Entry {
	if !m.wasResumed || m.currentSession == nil {
		return nil
	}
	entries := []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("Sesion resumida: %s", m.currentSession.Title)}}
	if m.brain != nil {
		var hints []string
		if m.brain.Exists("plan") {
			if plan, err := m.brain.Read("plan"); err == nil {
				hints = append(hints, fmt.Sprintf("plan.md (%.1fKB)", float64(len(plan))/1024.0))
			}
		}
		if m.brain.Exists("tasks") {
			if tasks, err := m.brain.Read("tasks"); err == nil {
				hints = append(hints, fmt.Sprintf("tasks.md (%.1fKB)", float64(len(tasks))/1024.0))
			}
		}
		if len(hints) > 0 {
			entries = append(entries, types.Entry{
				Kind: types.EntrySystem,
				Text: fmt.Sprintf("Session brain found: %s. The agent will continue from the existing plan.", strings.Join(hints, ", ")),
			})
		}
	}
	entries = append(entries, m.CurrentSessionEntries()...)
	return entries
}

func (m *Manager) HistoryInputTokens() int {
	if m.currentSession == nil {
		return 0
	}
	return m.currentSession.LastInputTokens
}
