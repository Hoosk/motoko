package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeProviderBackfillsPresetKindNameAndBaseURL(t *testing.T) {
	got := NormalizeProvider(ProviderConfig{
		Kind:   "openai",
		APIKey: "  secret  ",
		Models: []string{" gpt-4.1 ", "", "gpt-4.1"},
	})

	if got.Preset != ProviderPresetOpenAI {
		t.Fatalf("expected openai preset, got %q", got.Preset)
	}
	if got.Kind != ProviderKindOpenAICompatible {
		t.Fatalf("expected openai-compatible kind, got %q", got.Kind)
	}
	if got.Name != "openai" {
		t.Fatalf("expected default name, got %q", got.Name)
	}
	if got.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("expected default base url, got %q", got.BaseURL)
	}
	if got.APIKey != "secret" {
		t.Fatalf("expected api key trimmed, got %q", got.APIKey)
	}
	if len(got.Models) != 1 || got.Models[0] != "gpt-4.1" {
		t.Fatalf("expected unique sorted models, got %#v", got.Models)
	}
}

func TestNormalizeProviderSupportsOpenRouterPreset(t *testing.T) {
	got := NormalizeProvider(ProviderConfig{Preset: ProviderPresetOpenRouter})
	if got.Kind != ProviderKindOpenAICompatible {
		t.Fatalf("expected openrouter to map to openai-compatible, got %q", got.Kind)
	}
	if got.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("expected openrouter base url, got %q", got.BaseURL)
	}
	if got.Name != "openrouter" {
		t.Fatalf("expected openrouter default name, got %q", got.Name)
	}
}

func TestAppConfigUpsertProviderReplacesCaseInsensitiveMatch(t *testing.T) {
	cfg := &AppConfig{Providers: []ProviderConfig{{Name: "OpenAI", Preset: ProviderPresetOpenAI, Kind: ProviderKindOpenAICompatible, Model: "gpt-4.1"}}}
	cfg.UpsertProvider(ProviderConfig{Name: "openai", Preset: ProviderPresetOpenRouter, APIKey: "k"})

	if len(cfg.Providers) != 1 {
		t.Fatalf("expected one provider, got %d", len(cfg.Providers))
	}
	if cfg.Providers[0].Preset != ProviderPresetOpenRouter {
		t.Fatalf("expected provider replaced, got %#v", cfg.Providers[0])
	}
}

func TestRemoveAndSetActiveProvider(t *testing.T) {
	cfg := &AppConfig{Providers: []ProviderConfig{{Name: "openai"}, {Name: "anthropic"}}, ActiveProvider: "openai"}
	if err := cfg.SetActive("anthropic"); err != nil {
		t.Fatal(err)
	}
	if cfg.ActiveProvider != "anthropic" {
		t.Fatalf("expected active provider updated, got %q", cfg.ActiveProvider)
	}
	if !cfg.RemoveProvider("anthropic") {
		t.Fatal("expected provider removed")
	}
	if cfg.ActiveProvider != "" {
		t.Fatalf("expected active provider cleared, got %q", cfg.ActiveProvider)
	}
}

func TestUniqueSortedKeepDeduplicatesAndSorts(t *testing.T) {
	got := UniqueSortedKeep([]string{"b", "a"}, "b", "c", " ")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("unexpected result %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected result %#v, want %#v", got, want)
		}
	}
}

func TestLoadAndSaveRoundTrip(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	cfg := &AppConfig{
		ActiveProvider: "openrouter",
		Providers: []ProviderConfig{{
			Name:   "openrouter",
			Preset: ProviderPresetOpenRouter,
			Kind:   ProviderKindOpenAICompatible,
			APIKey: "k",
			Model:  "openai/gpt-4.1",
		}},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ActiveProvider != "openrouter" || len(loaded.Providers) != 1 {
		t.Fatalf("unexpected loaded config %#v", loaded)
	}
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config file saved at %s: %v", path, err)
	}
	if filepath.Dir(path) != filepath.Join(configHome, "motoko") {
		t.Fatalf("unexpected config dir %q", filepath.Dir(path))
	}
}
