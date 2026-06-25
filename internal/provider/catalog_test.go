package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Hoosk/motoko/internal/config"
)

func TestParseAndPopulate(t *testing.T) {
	mockData := `{
		"openai/gpt-4o": {
			"id": "openai/gpt-4o",
			"name": "GPT-4o",
			"family": "gpt",
			"reasoning": false,
			"limit": {
				"context": 128000,
				"output": 16384
			}
		},
		"google/gemini-2.5-pro": {
			"id": "google/gemini-2.5-pro",
			"name": "Gemini 2.5 Pro",
			"family": "gemini",
			"reasoning": true,
			"limit": {
				"context": 1048576,
				"output": 65536
			}
		}
	}`

	err := parseAndPopulate([]byte(mockData))
	if err != nil {
		t.Fatalf("unexpected parsing error: %v", err)
	}

	catalogMu.RLock()
	defer catalogMu.RUnlock()

	if len(catalogCache) != 2 {
		t.Fatalf("expected 2 models, got %d", len(catalogCache))
	}

	m1, ok := catalogCache["openai/gpt-4o"]
	if !ok {
		t.Fatalf("model openai/gpt-4o not found in cache")
	}
	if m1.ContextWindow != 128000 || m1.SupportsThinking {
		t.Fatalf("unexpected properties for openai/gpt-4o: %+v", m1)
	}

	m2, ok := catalogCache["google/gemini-2.5-pro"]
	if !ok {
		t.Fatalf("model google/gemini-2.5-pro not found in cache")
	}
	if m2.ContextWindow != 1048576 || !m2.SupportsThinking {
		t.Fatalf("unexpected properties for google/gemini-2.5-pro: %+v", m2)
	}
}

func TestLookupModel(t *testing.T) {
	mockData := `{
		"openai/gpt-4o": {
			"id": "openai/gpt-4o",
			"name": "GPT-4o",
			"family": "gpt",
			"reasoning": false,
			"limit": {
				"context": 128000,
				"output": 16384
			}
		},
		"google/gemini-2.5-pro": {
			"id": "google/gemini-2.5-pro",
			"name": "Gemini 2.5 Pro",
			"family": "gemini",
			"reasoning": true,
			"limit": {
				"context": 1048576,
				"output": 65536
			}
		}
	}`

	_ = parseAndPopulate([]byte(mockData))

	tests := []struct {
		provider string
		model    string
		expected bool
		context  int
	}{
		{"openai", "openai/gpt-4o", true, 128000},
		{"openai", "gpt-4o", true, 128000},
		{"gemini", "gemini-2.5-pro", true, 1048576},
		{"google", "gemini-2.5-pro", true, 1048576},
		{"google", "google/gemini-2.5-pro", true, 1048576},
		{"other", "gemini-2.5-pro", true, 1048576}, // suffix match
		{"openai", "non-existent", false, 0},
	}

	for _, tc := range tests {
		info, ok := LookupModel(tc.provider, tc.model)
		if ok != tc.expected {
			t.Errorf("LookupModel(%q, %q) expected found=%t, got %t", tc.provider, tc.model, tc.expected, ok)
		}
		if ok && info.ContextWindow != tc.context {
			t.Errorf("LookupModel(%q, %q) expected context=%d, got %d", tc.provider, tc.model, tc.context, info.ContextWindow)
		}
	}
}

func TestLoadCatalogFileCache(t *testing.T) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("failed to get user cache dir: %v", err)
	}
	cachePath := filepath.Join(cacheDir, "motoko", "models_cache.json")

	// Backup existing cache file if any
	var backupData []byte
	backupExists := false
	if _, statErr := os.Stat(cachePath); statErr == nil {
		backupData, _ = os.ReadFile(cachePath)
		backupExists = true
		_ = os.Remove(cachePath)
	}
	defer func() {
		if backupExists {
			_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
			_ = os.WriteFile(cachePath, backupData, 0644)
		} else {
			_ = os.Remove(cachePath)
		}
	}()

	mockData := `{
		"openai/gpt-4o-mini": {
			"id": "openai/gpt-4o-mini",
			"name": "GPT-4o Mini",
			"family": "gpt",
			"reasoning": false,
			"limit": {
				"context": 128000,
				"output": 16384
			}
		}
	}`

	_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
	err = os.WriteFile(cachePath, []byte(mockData), 0644)
	if err != nil {
		t.Fatalf("failed to write test cache file: %v", err)
	}

	// Reset global state
	catalogMu.Lock()
	catalogLoaded = false
	catalogCache = make(map[string]ModelInfo)
	catalogMu.Unlock()

	err = LoadCatalog(context.Background())
	if err != nil {
		t.Fatalf("LoadCatalog failed: %v", err)
	}

	info, ok := LookupModel("openai", "gpt-4o-mini")
	if !ok {
		t.Fatalf("failed to look up gpt-4o-mini from loaded cache")
	}
	if info.ContextWindow != 128000 {
		t.Fatalf("unexpected context window: %d", info.ContextWindow)
	}
}

func TestParseAndPopulateNested(t *testing.T) {
	mockData := `{
		"models": {
			"xai/grok-4": {
				"id": "xai/grok-4",
				"name": "Grok 4",
				"family": "grok",
				"reasoning": true,
				"limit": {
					"context": 256000,
					"output": 64000
				}
			}
		},
		"providers": {
			"xai": {
				"id": "xai",
				"name": "xAI",
				"api": "https://api.x.ai/v1",
				"env": ["XAI_API_KEY"]
			}
		}
	}`

	err := parseAndPopulate([]byte(mockData))
	if err != nil {
		t.Fatalf("unexpected parsing error: %v", err)
	}

	catalogMu.RLock()
	defer catalogMu.RUnlock()

	if len(catalogCache) != 1 {
		t.Fatalf("expected 1 model, got %d", len(catalogCache))
	}

	m, ok := catalogCache["xai/grok-4"]
	if !ok {
		t.Fatalf("model xai/grok-4 not found")
	}
	if m.ContextWindow != 256000 || !m.SupportsThinking {
		t.Fatalf("unexpected model details: %+v", m)
	}

	if len(providersCache) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(providersCache))
	}

	p, ok := providersCache["xai"]
	if !ok {
		t.Fatalf("provider xai not found")
	}
	if p.Name != "xAI" || p.API != "https://api.x.ai/v1" || len(p.Env) != 1 || p.Env[0] != "XAI_API_KEY" {
		t.Fatalf("unexpected provider details: %+v", p)
	}

	// Test LookupProvider
	p2, found := LookupProvider("xai")
	if !found {
		t.Fatalf("LookupProvider failed for xai")
	}
	if p2.Name != "xAI" {
		t.Fatalf("LookupProvider returned wrong provider name: %s", p2.Name)
	}
}

func TestNewClientDynamicBaseURL(t *testing.T) {
	mockData := `{
		"models": {},
		"providers": {
			"deepseek": {
				"id": "deepseek",
				"name": "DeepSeek",
				"api": "https://api.deepseek.com/v2",
				"env": ["DEEPSEEK_API_KEY"]
			}
		}
	}`

	err := parseAndPopulate([]byte(mockData))
	if err != nil {
		t.Fatalf("unexpected parsing error: %v", err)
	}

	cfg := config.ProviderConfig{
		Name:   "my-deepseek-custom",
		Preset: "deepseek",
		APIKey: "sk-test-key",
		Model:  "deepseek-chat",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed for custom deepseek preset: %v", err)
	}

	if client == nil {
		t.Fatal("expected non-nil client")
	}

	if client.Summary() != "my-deepseek-custom:deepseek-chat" {
		t.Fatalf("expected summary 'my-deepseek-custom:deepseek-chat', got %q", client.Summary())
	}
}

func TestNewClientDynamicKind(t *testing.T) {
	mockData := `{
		"models": {},
		"providers": {
			"custom-anthropic": {
				"id": "custom-anthropic",
				"name": "Custom Anthropic",
				"api": "https://api.anthropic.com/v1",
				"env": ["ANTHROPIC_API_KEY"],
				"npm": "@ai-sdk/anthropic"
			},
			"custom-google": {
				"id": "custom-google",
				"name": "Custom Google",
				"api": "https://generativelanguage.googleapis.com",
				"env": ["GEMINI_API_KEY"],
				"npm": "@ai-sdk/google"
			}
		}
	}`

	err := parseAndPopulate([]byte(mockData))
	if err != nil {
		t.Fatalf("unexpected parsing error: %v", err)
	}

	// 1. Anthropic kind mapping
	cfgAnt := config.ProviderConfig{
		Name:   "my-anthropic",
		Preset: "custom-anthropic",
		APIKey: "sk-test-key",
		Model:  "claude-3-5-sonnet",
	}
	clientAnt, err := NewClient(cfgAnt)
	if err != nil {
		t.Fatalf("NewClient failed for custom anthropic: %v", err)
	}
	if clientAnt.ProviderKind() != "my-anthropic" {
		t.Errorf("expected ProviderKind 'my-anthropic', got %q", clientAnt.ProviderKind())
	}

	// 2. Google kind mapping
	cfgGog := config.ProviderConfig{
		Name:   "my-google",
		Preset: "custom-google",
		APIKey: "sk-test-key",
		Model:  "gemini-2.5-pro",
	}
	clientGog, err := NewClient(cfgGog)
	if err != nil {
		t.Fatalf("NewClient failed for custom google: %v", err)
	}
	if clientGog.ProviderKind() != "my-google" {
		t.Errorf("expected ProviderKind 'my-google', got %q", clientGog.ProviderKind())
	}
}
