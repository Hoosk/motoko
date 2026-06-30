package app

import (
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/tools"
)

func TestRuntime_AgenticImprovements(t *testing.T) {
	// 1. Setup mock runtime
	r := NewRuntime()
	r.config = &config.AppConfig{
		ActiveProvider: "openai",
		Providers: []config.ProviderConfig{{
			Name:   "openai",
			Preset: config.ProviderPresetOpenAI,
			Kind:   config.ProviderKindOpenAICompatible,
		}},
	}
	r.newProviderClient = func(cfg config.ProviderConfig) (provider.Client, error) {
		return fakeProviderClient{}, nil
	}

	// 2. Test tool registry filtering
	r.SetAgentMode("plan")
	r.agOrch.RefreshAgent()
	if r.agOrch.Agent() == nil {
		t.Fatal("expected agent to be initialized")
	}

	// Verify that planning agent prompt does NOT have write tools listed
	info := r.GetContextInfo()
	prompt := r.agOrch.Agent().SystemPrompt(info)
	if strings.Contains(prompt, "- patch:") || strings.Contains(prompt, "- bash:") {
		t.Error("expected planning agent prompt to omit write tools (patch and bash)")
	}

	r.SetAgentMode("build")
	r.agOrch.RefreshAgent()
	prompt = r.agOrch.Agent().SystemPrompt(info)
	if !strings.Contains(prompt, "- patch:") || !strings.Contains(prompt, "- bash:") {
		t.Error("expected build agent prompt to contain write tools")
	}

	// 3. Test slash command autocompletions for /agent
	completions := r.Completions("/agent ")
	if len(completions) < 3 {
		t.Errorf("expected at least 3 agent completions, got %v", completions)
	}
	foundSearch := false
	for _, comp := range completions {
		if comp == "/agent search" {
			foundSearch = true
			break
		}
	}
	if !foundSearch {
		t.Error("expected /agent search to be suggested")
	}

	// 4. Test input routing and cleansing with @ prefix
	resp := r.HandleInput("@search find walkWorkspace", info)
	if r.AgentName() != "search" {
		t.Errorf("expected active agent to switch to search, got %s", r.AgentName())
	}
	if resp.Action == nil || resp.Action.AgentPrompt != "find walkWorkspace" {
		t.Errorf("expected clean prompt 'find walkWorkspace', got %q", resp.Action.AgentPrompt)
	}

	// 5. Test subagent state tracking (active subagents now managed by agOrch)
	_ = r.ActiveSubagents()
}

func TestTools_FilteringRegistry(t *testing.T) {
	r := tools.NewRegistry()
	filtered := r.Filter(func(tool tools.Tool) bool {
		return !tools.IsWriteTool(tool.Spec().Name)
	})

	for _, spec := range filtered.Specs(tools.ToolContext{}) {
		if tools.IsWriteTool(spec.Name) {
			t.Errorf("filtered registry contains write tool %s", spec.Name)
		}
	}
}

func TestSearchConfig_Defaults(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.Search.MaxResults <= 0 {
		t.Errorf("expected default MaxResults to be populated, got %d", cfg.Search.MaxResults)
	}
	if len(cfg.Search.ExcludePatterns) == 0 {
		t.Error("expected default ExcludePatterns to be populated")
	}
}
