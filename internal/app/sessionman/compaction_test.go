package sessionman

import (
	"context"
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/session"

	"github.com/Hoosk/motoko/internal/app/types"
)

type fakeCompactClient struct {
	finalText string
	err       error
}

func (f fakeCompactClient) Configured() bool     { return true }
func (f fakeCompactClient) ProviderKind() string { return "fake" }

func (f fakeCompactClient) Complete(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, toolSet provider.ToolSet) (provider.Response, error) {
	if f.err != nil {
		return provider.Response{}, f.err
	}
	return provider.Response{FinalText: f.finalText}, nil
}

func (f fakeCompactClient) StreamComplete(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, toolSet provider.ToolSet, onDelta func(provider.Delta) error) (provider.Response, error) {
	return f.Complete(ctx, systemPrompt, messages, toolSet)
}

func (f fakeCompactClient) Summary() string { return "fake" }

func (f fakeCompactClient) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	return nil, nil
}

func (f fakeCompactClient) GetModel(ctx context.Context, model string) (provider.ModelInfo, error) {
	return provider.ModelInfo{}, nil
}

func compactTestConfig() *config.AppConfig {
	return &config.AppConfig{
		ActiveProvider: "openai",
		Providers: []config.ProviderConfig{{
			Name:   "openai",
			Preset: config.ProviderPresetOpenAI,
			Kind:   config.ProviderKindOpenAICompatible,
			APIKey: "k",
			Model:  "gpt-4.1",
		}},
	}
}

func compactTestProviderFn(t *testing.T, client provider.Client) func(config.ProviderConfig) (provider.Client, error) {
	t.Helper()
	return func(config.ProviderConfig) (provider.Client, error) { return client, nil }
}

func withSessionBaseDirSessionman(t *testing.T) {
	t.Helper()
	prev := session.SessionsBaseDir
	session.SessionsBaseDir = t.TempDir()
	t.Cleanup(func() {
		session.SessionsBaseDir = prev
	})
}

func TestMaybeAutoCompactSkipsWhenContextWindowZero(t *testing.T) {
	withSessionBaseDirSessionman(t)
	m := NewManager("ws")
	m.SetCurrentSession(&session.Session{
		History:         []provider.ConversationItem{provider.UserText(strings.Repeat("A", 5000))},
		LastInputTokens: 900,
	})

	events := 0
	err := m.MaybeAutoCompact(context.Background(), func(types.AgentStreamEvent) error {
		events++
		return nil
	}, compactTestConfig(), compactTestProviderFn(t, fakeCompactClient{finalText: "resumen"}), 0)
	if err != nil {
		t.Fatalf("MaybeAutoCompact() error = %v", err)
	}
	if events != 0 {
		t.Fatalf("expected no compact events with unknown context window, got %d", events)
	}
	if m.CurrentSession().LastInputTokens != 900 {
		t.Fatalf("expected session untouched, got LastInputTokens %d", m.CurrentSession().LastInputTokens)
	}
}

func TestPersistTurnLastInputTokensUsesLastIteration(t *testing.T) {
	m := NewManager("ws")
	s := session.New("ws", "/tmp")
	m.SetCurrentSession(s)

	m.PersistTurn(agent.Result{
		Assistant: "response",
		Iterations: []provider.Usage{
			{InputTokens: 60, TotalTokens: 80},
			{InputTokens: 100, TotalTokens: 130},
		},
		History: []provider.ConversationItem{
			{Role: provider.RoleUser, Content: "query"},
			{Role: provider.RoleAssistant, Content: "response"},
		},
		Usage: provider.Usage{
			InputTokens: 160,
			TotalTokens: 210,
		},
	})

	if s.LastInputTokens != 100 {
		t.Errorf("expected LastInputTokens 100 (last iteration), got %d", s.LastInputTokens)
	}
	if s.TotalInputTokens != 160 {
		t.Errorf("expected TotalInputTokens 160 (turn sum), got %d", s.TotalInputTokens)
	}
	if len(s.Turns) != 1 || s.Turns[0].InputGrowth != 40 {
		t.Fatalf("unexpected turns %#v", s.Turns)
	}
}
