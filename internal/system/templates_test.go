package system

import (
	"strings"
	"testing"
)

func TestLoadProviderHeader(t *testing.T) {
	tests := []struct {
		provider string
		expected string
	}{
		{"gemini", "Google's Gemini"},
		{"claude", "Anthropic's Claude"},
		{"openai", "OpenAI"},
		{"kimi", "Moonshot AI's Kimi"},
		{"trinity", "Trinity"},
		{"unknown", "You are Motoko"},
	}

	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			got := LoadProviderHeader(tc.provider)
			if !strings.Contains(got, tc.expected) {
				t.Fatalf("expected header for %s to contain %q, got:\n%s", tc.provider, tc.expected, got)
			}
		})
	}
}
