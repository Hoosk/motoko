package providerman

import (
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/provider"

	"github.com/Hoosk/motoko/internal/app/types"
)

func testManager(cfg *config.AppConfig) *Manager {
	return NewManager(
		func() *config.AppConfig { return cfg },
		func() func(config.ProviderConfig) (provider.Client, error) {
			return func(config.ProviderConfig) (provider.Client, error) { return nil, nil }
		},
		func() {},
	)
}

func TestProviderSummary(t *testing.T) {
	cfg := &config.AppConfig{
		ActiveProvider: "openai",
		Providers: []config.ProviderConfig{{
			Name:  "openai",
			Model: "gpt-4.1",
		}},
	}
	m := testManager(cfg)
	summary := m.ProviderSummary()
	if !strings.Contains(summary, "openai") {
		t.Errorf("expected 'openai' in summary, got %q", summary)
	}
	if !strings.Contains(summary, "gpt-4.1") {
		t.Errorf("expected 'gpt-4.1' in summary, got %q", summary)
	}
}

func TestProviderSummaryNoActive(t *testing.T) {
	cfg := &config.AppConfig{ActiveProvider: ""}
	m := testManager(cfg)
	summary := m.ProviderSummary()
	if summary != "none" {
		t.Errorf("expected 'none', got %q", summary)
	}
}

func TestProviderPresets(t *testing.T) {
	m := testManager(&config.AppConfig{})
	presets := m.ProviderPresets()
	if len(presets) == 0 {
		t.Fatal("expected non-empty presets")
	}
}

func TestGetActiveProviderConfig(t *testing.T) {
	cfg := &config.AppConfig{
		ActiveProvider: "openai",
		Providers: []config.ProviderConfig{{
			Name:   "openai",
			Preset: config.ProviderPresetOpenAI,
			Model:  "gpt-4.1",
		}},
	}
	m := testManager(cfg)
	active, ok := m.GetActiveProviderConfig()
	if !ok {
		t.Fatal("expected active provider")
	}
	if active.Name != "openai" {
		t.Errorf("expected 'openai', got %q", active.Name)
	}
	if active.Model != "gpt-4.1" {
		t.Errorf("expected 'gpt-4.1', got %q", active.Model)
	}
}

func TestGetActiveProviderConfigNoActive(t *testing.T) {
	cfg := &config.AppConfig{ActiveProvider: ""}
	m := testManager(cfg)
	_, ok := m.GetActiveProviderConfig()
	if ok {
		t.Error("expected no active provider")
	}
}

func TestLookupCatalogProvider(t *testing.T) {
	m := testManager(&config.AppConfig{})
	_, ok := m.LookupCatalogProvider("nonexistent")
	if ok {
		t.Error("expected no catalog provider for nonexistent")
	}
}

func TestProviderListText(t *testing.T) {
	cfg := &config.AppConfig{
		ActiveProvider: "openai",
		Providers: []config.ProviderConfig{
			{Name: "openai", Preset: config.ProviderPresetOpenAI, Model: "gpt-4.1"},
			{Name: "anthropic", Preset: config.ProviderPresetAnthropic, Model: "claude-sonnet"},
		},
	}
	m := testManager(cfg)
	text := m.ProviderListText()
	if !strings.Contains(text, "* openai") {
		t.Error("expected active marker on openai")
	}
	if !strings.Contains(text, "anthropic") {
		t.Error("expected anthropic in list")
	}
}

func TestSetActiveModelInfo(t *testing.T) {
	cfg := &config.AppConfig{
		ActiveProvider: "openai",
		Providers: []config.ProviderConfig{{
			Name:   "openai",
			Preset: config.ProviderPresetOpenAI,
			Model:  "old-model",
		}},
	}
	m := testManager(cfg)
	err := m.SetActiveModelInfo(provider.ModelInfo{ID: "gpt-4.1", EffortPresets: []string{"low", "medium"}, BudgetMax: 24576})
	if err != nil {
		t.Fatalf("SetActiveModelInfo: %v", err)
	}
	active, ok := cfg.Active()
	if !ok || active.Model != "gpt-4.1" {
		t.Errorf("expected model set to gpt-4.1, got %q", active.Model)
	}
	if len(active.EffortPresets) != 2 || active.BudgetMax != 24576 {
		t.Fatalf("expected reasoning metadata copied to provider config, got %+v", active)
	}
}

func TestSetActiveModelInfoNoProvider(t *testing.T) {
	cfg := &config.AppConfig{ActiveProvider: ""}
	m := testManager(cfg)
	err := m.SetActiveModelInfo(provider.ModelInfo{ID: "gpt-4.1"})
	if err == nil {
		t.Error("expected error for no active provider")
	}
}

func TestSaveProvider(t *testing.T) {
	cfg := &config.AppConfig{}
	m := testManager(cfg)
	pc := config.ProviderConfig{
		Name:   "test-provider",
		Preset: config.ProviderPresetOpenAI,
		APIKey: "sk-test",
		Model:  "gpt-4.1",
	}
	err := m.SaveProvider(pc, true)
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	if cfg.ActiveProvider != "test-provider" {
		t.Errorf("expected ActiveProvider 'test-provider', got %q", cfg.ActiveProvider)
	}
	if len(cfg.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(cfg.Providers))
	}
}

func TestSaveProviderNormalizesName(t *testing.T) {
	cfg := &config.AppConfig{}
	m := testManager(cfg)
	err := m.SaveProvider(config.ProviderConfig{
		Preset: config.ProviderPresetOpenRouter,
		APIKey: "k",
		Model:  "openai/gpt-4.1",
	}, true)
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	if cfg.ActiveProvider != "openrouter" {
		t.Errorf("expected normalized active provider, got %q", cfg.ActiveProvider)
	}
}

func TestSaveProviderUpdateExisting(t *testing.T) {
	cfg := &config.AppConfig{
		ActiveProvider: "openai",
		Providers: []config.ProviderConfig{{
			Name:  "openai",
			Model: "old-model",
		}},
	}
	m := testManager(cfg)
	err := m.SaveProvider(config.ProviderConfig{
		Name:  "openai",
		Model: "new-model",
	}, false)
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	provider, _ := cfg.Provider("openai")
	if provider.Model != "new-model" {
		t.Errorf("expected model updated to 'new-model', got %q", provider.Model)
	}
}

func TestHandleProviderCommand(t *testing.T) {
	cfg := &config.AppConfig{
		ActiveProvider: "openai",
		Providers: []config.ProviderConfig{{
			Name:   "openai",
			Preset: config.ProviderPresetOpenAI,
			APIKey: "sk-test",
			Model:  "gpt-4.1",
		}},
	}
	m := testManager(cfg)
	resp := m.HandleProviderCommand([]string{"list"})
	if len(resp.Entries) == 0 {
		t.Fatal("expected provider list output")
	}
	text := resp.Entries[0].Text
	if !strings.Contains(text, "active") && !strings.Contains(text, "openai") {
		t.Errorf("expected provider info in output, got %q", text)
	}
}

func TestHandleModelsCommandList(t *testing.T) {
	cfg := &config.AppConfig{
		ActiveProvider: "openai",
		Providers: []config.ProviderConfig{{
			Name:   "openai",
			Preset: config.ProviderPresetOpenAI,
			APIKey: "sk-test",
			Model:  "gpt-4.1",
			Models: []string{"gpt-4.1", "gpt-3.5"},
		}},
	}
	m := testManager(cfg)
	resp := m.HandleModelsCommand(nil)
	if len(resp.Entries) == 0 {
		t.Fatal("expected models output")
	}
}

func TestHandleModelsCommandNoProvider(t *testing.T) {
	cfg := &config.AppConfig{ActiveProvider: ""}
	m := testManager(cfg)
	resp := m.HandleModelsCommand(nil)
	if len(resp.Entries) == 0 || resp.Entries[0].Kind != types.EntryError {
		t.Error("expected error for no provider")
	}
}

func TestSetThinkingBudget(t *testing.T) {
	cfg := &config.AppConfig{
		ActiveProvider: "openai",
		Providers: []config.ProviderConfig{{
			Name:   "openai",
			Preset: config.ProviderPresetOpenAI,
			Model:  "gpt-4.1",
		}},
	}
	m := testManager(cfg)
	err := m.SetThinkingBudget(16000)
	if err != nil {
		t.Fatalf("SetThinkingBudget: %v", err)
	}
	active, ok := cfg.Active()
	if !ok || active.ThinkingBudget != 16000 {
		t.Errorf("expected thinking budget 16000, got %d", active.ThinkingBudget)
	}
}

func TestSetThinkingBudgetNoProvider(t *testing.T) {
	cfg := &config.AppConfig{ActiveProvider: ""}
	m := testManager(cfg)
	err := m.SetThinkingBudget(16000)
	if err == nil {
		t.Error("expected error for no active provider")
	}
}
