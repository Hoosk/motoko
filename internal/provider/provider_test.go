package provider

import (
	"reflect"
	"testing"

	"github.com/Hoosk/motoko/internal/config"
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
	openAI := toOpenAIMessages("sys", []Message{{Role: "user", Content: "hola"}, {Role: "assistant", Content: "mundo"}})
	if len(openAI) != 3 || openAI[0]["role"] != "system" {
		t.Fatalf("unexpected openai messages %#v", openAI)
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

func TestCollectModelsAndUniqueSorted(t *testing.T) {
	models := collectModels([]struct{ ID string }{{ID: " gpt-4.1 "}, {ID: "gpt-4.1"}, {ID: "o4-mini"}}, func(item struct{ ID string }) string { return item.ID })
	if !reflect.DeepEqual(models, []string{"gpt-4.1", "o4-mini"}) {
		t.Fatalf("unexpected collected models %#v", models)
	}
	if got := uniqueSorted([]string{"b", "a", "a"}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("unexpected uniqueSorted result %#v", got)
	}
}

func TestParseStreamResponseKeepsUsageForPlainText(t *testing.T) {
	resp := parseStreamResponse("hola", Usage{InputTokens: 2, OutputTokens: 3, TotalTokens: 5})
	if resp.Message != "hola" || resp.Usage.TotalTokens != 5 {
		t.Fatalf("unexpected parsed stream response %#v", resp)
	}
}
