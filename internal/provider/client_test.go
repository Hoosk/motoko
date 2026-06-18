package provider_test

import (
	"testing"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/provider"

	// Register all providers
	_ "github.com/Hoosk/motoko/internal/provider/anthropic"
	_ "github.com/Hoosk/motoko/internal/provider/gemini"
	_ "github.com/Hoosk/motoko/internal/provider/lmstudio"
	_ "github.com/Hoosk/motoko/internal/provider/openai"
)

func TestNewClientRegistration(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.ProviderConfig
		wantErr bool
	}{
		{
			name: "openai compatible provider",
			cfg: config.ProviderConfig{
				Name:    "my-openai",
				Kind:    config.ProviderKindOpenAICompatible,
				BaseURL: "https://api.openai.com/v1",
				APIKey:  "sk-test",
				Model:   "gpt-4o",
			},
			wantErr: false,
		},
		{
			name: "anthropic provider",
			cfg: config.ProviderConfig{
				Name:    "my-anthropic",
				Kind:    config.ProviderKindAnthropic,
				BaseURL: "https://api.anthropic.com/v1",
				APIKey:  "sk-ant-test",
				Model:   "claude-3-5-sonnet-latest",
			},
			wantErr: false,
		},
		{
			name: "gemini provider",
			cfg: config.ProviderConfig{
				Name:    "my-gemini",
				Kind:    config.ProviderKindGemini,
				BaseURL: "https://generativelanguage.googleapis.com",
				APIKey:  "ai-test",
				Model:   "gemini-1.5-pro",
			},
			wantErr: false,
		},
		{
			name: "lmstudio provider",
			cfg: config.ProviderConfig{
				Name:    "my-lmstudio",
				Kind:    config.ProviderKindLMStudio,
				BaseURL: "http://localhost:1234/v1",
				Model:   "local-model",
			},
			wantErr: false,
		},
		{
			name: "unknown provider kind",
			cfg: config.ProviderConfig{
				Name: "unknown",
				Kind: config.ProviderKind("nonexistent"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := provider.NewClient(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && client == nil {
				t.Fatal("expected non-nil client")
			}
		})
	}
}
