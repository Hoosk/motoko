package app

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/semantic"
	"github.com/Hoosk/motoko/internal/system"
)

func TestCompletionsModelsKeepsTrailingSpaceContext(t *testing.T) {
	r := NewRuntime()
	r.config = &config.AppConfig{
		ActiveProvider: "openai",
		Providers: []config.ProviderConfig{{
			Name:   "openai",
			Preset: config.ProviderPresetOpenAI,
			Kind:   config.ProviderKindOpenAICompatible,
			Models: []string{"gpt-4.1", "gpt-4.1-mini", "o4-mini"},
		}},
	}

	got := r.Completions("/models ")
	want := []string{"/models gpt-4.1", "/models gpt-4.1-mini", "/models o4-mini"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Completions(/models space) = %#v, want %#v", got, want)
	}
}

func TestCompletionsModelsFiltersPrefix(t *testing.T) {
	r := NewRuntime()
	r.config = &config.AppConfig{
		ActiveProvider: "openai",
		Providers: []config.ProviderConfig{{
			Name:   "openai",
			Preset: config.ProviderPresetOpenAI,
			Kind:   config.ProviderKindOpenAICompatible,
			Models: []string{"gpt-4.1", "gpt-4.1-mini", "o4-mini"},
		}},
	}

	got := r.Completions("/models gpt-4.1-m")
	want := []string{"/models gpt-4.1-mini"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Completions(/models prefix) = %#v, want %#v", got, want)
	}
}

func TestEnrichContextAddsRelevantSnippets(t *testing.T) {
	r := NewRuntime()
	snapshot := semantic.Snapshot{Files: []semantic.FileSummary{{
		Path:     "internal/app/runtime.go",
		Language: "go",
		Content:  []byte("package app\n\nfunc RunAgent() error {\n\treturn nil\n}\n"),
		Symbols:  []semantic.Symbol{{Name: "RunAgent", Kind: "func", Line: 3, Range: semantic.LineRange{Start: 3, End: 5}}},
	}}, GeneratedAt: time.Now()}
	r.semantic = semantic.NewIndex()
	r.semantic.SetSnapshotForTest(&snapshot)

	info := r.enrichContext(context.Background(), system.ContextInfo{}, "revisa runagent")
	if len(info.RelevantSnippets) == 0 {
		t.Fatal("expected relevant snippets")
	}
	if !strings.Contains(info.RelevantSnippets[0], "RunAgent") {
		t.Fatalf("expected snippet mentioning RunAgent, got %q", info.RelevantSnippets[0])
	}
}

func TestSaveProviderNormalizesNameBeforeActivating(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	r := NewRuntime()
	err := r.SaveProvider(config.ProviderConfig{
		Preset: config.ProviderPresetOpenRouter,
		APIKey: "k",
		Model:  "openai/gpt-4.1",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if r.config.ActiveProvider != "openrouter" {
		t.Fatalf("expected normalized active provider, got %q", r.config.ActiveProvider)
	}
	active, ok := r.config.Active()
	if !ok || active.Name != "openrouter" {
		t.Fatalf("expected normalized active config, got %#v ok=%t", active, ok)
	}
}
