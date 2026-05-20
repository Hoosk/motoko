package provider

import (
	"testing"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/openai/openai-go/v3/responses"
)

func TestNewClientUsesNormalizedProviderKinds(t *testing.T) {
	client, err := NewClient(config.ProviderConfig{Preset: config.ProviderPresetOpenRouter, Model: "openai/gpt-4.1", APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := client.(*openAIClient); !ok {
		t.Fatalf("expected openAIClient for openrouter preset, got %T", client)
	}
}

func TestParseStructuredResponseNormalizesFencedJSON(t *testing.T) {
	resp := parseStructuredResponse("```json\n{\"message\":\"hola\"}\n```")
	if resp.Message != "hola" {
		t.Fatalf("expected parsed fenced message, got %#v", resp)
	}
}

func TestParseStructuredResponseExtractsJSONFromSurroundingText(t *testing.T) {
	resp := parseStructuredResponse("nota previa {\"tool_name\":\"read\",\"tool_input\":\"README.md\"} trailing")
	if resp.ToolCall == nil || resp.ToolCall.Name != "read" {
		t.Fatalf("expected tool call parsed, got %#v", resp)
	}
}

func TestMessageSerializationHelpers(t *testing.T) {
	// OpenAI Responses API: toResponsesInputItems maps messages to input item params.
	items := toResponsesInputItems([]Message{{Role: "user", Content: "hola"}, {Role: "assistant", Content: "mundo"}})
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].OfMessage == nil || items[0].OfMessage.Role != responses.EasyInputMessageRoleUser {
		t.Fatalf("expected user role on first item, got %#v", items[0])
	}
	if items[1].OfMessage == nil || items[1].OfMessage.Role != responses.EasyInputMessageRoleAssistant {
		t.Fatalf("expected assistant role on second item, got %#v", items[1])
	}

	anthropic := toAnthropicMessages([]Message{{Role: "system", Content: "ignored"}, {Role: "user", Content: "hola"}})
	if len(anthropic) != 1 || anthropic[0]["role"] != "user" {
		t.Fatalf("unexpected anthropic messages %#v", anthropic)
	}
	gemini := toGeminiMessages([]Message{{Role: "assistant", Content: "hola"}})
	if len(gemini) != 1 || gemini[0]["role"] != "model" {
		t.Fatalf("unexpected gemini messages %#v", gemini)
	}
}

func TestParseStreamResponseKeepsUsageForPlainText(t *testing.T) {
	resp := parseStreamResponse("hola", Usage{InputTokens: 2, OutputTokens: 3, TotalTokens: 5})
	if resp.Message != "hola" || resp.Usage.TotalTokens != 5 {
		t.Fatalf("unexpected parsed stream response %#v", resp)
	}
}
