package gemini

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/provider"
)

func TestToGenAIContentUserAssistantFlow(t *testing.T) {
	messages := []provider.ConversationItem{
		provider.UserText("hello"),
		provider.AssistantText("hi there"),
	}

	contents := toGenAIContent(messages)
	if len(contents) != 2 {
		t.Fatalf("expected 2 content items, got %d", len(contents))
	}

	if contents[0].Role != "user" || len(contents[0].Parts) != 1 || contents[0].Parts[0].Text != "hello" {
		t.Fatalf("unexpected mapping of user message: %#v", contents[0])
	}

	if contents[1].Role != "model" || len(contents[1].Parts) != 1 || contents[1].Parts[0].Text != "hi there" {
		t.Fatalf("unexpected mapping of assistant message: %#v", contents[1])
	}
}

func TestToGenAIContentConsecutiveMessagesAndSnakeCaseSignature(t *testing.T) {
	messages := []provider.ConversationItem{
		provider.UserText("hello"),
		provider.AssistantText("let me think"),
		{
			Role: provider.RoleAssistant,
			Content: provider.FormatAssistantToolCallContent(provider.ToolInvocation{
				Kind:      provider.InvokeCustomTool,
				Name:      "bash",
				Arguments: json.RawMessage(`{"input":"ls"}`),
				CallID:    "call_123",
				Raw:       []byte(`{"id":"call_123","type":"function","function":{"name":"bash","arguments":"{\"input\":\"ls\"}"},"thought_signature":"c2lnbmF0dXJlX2Jhc2U2NA=="}`),
			}),
		},
	}

	contents := toGenAIContent(messages)
	if len(contents) != 2 {
		t.Fatalf("expected 2 content items due to role grouping, got %d", len(contents))
	}

	if contents[0].Role != "user" || len(contents[0].Parts) != 1 || contents[0].Parts[0].Text != "hello" {
		t.Fatalf("unexpected first user content: %#v", contents[0])
	}

	modelContent := contents[1]
	if modelContent.Role != "model" {
		t.Fatalf("expected role model, got %s", modelContent.Role)
	}
	if len(modelContent.Parts) != 2 {
		t.Fatalf("expected 2 parts in model content, got %d", len(modelContent.Parts))
	}

	if modelContent.Parts[0].Text != "let me think" {
		t.Fatalf("expected text part 'let me think', got: %q", modelContent.Parts[0].Text)
	}

	fnCallPart := modelContent.Parts[1]
	if fnCallPart.FunctionCall == nil || fnCallPart.FunctionCall.Name != "bash" {
		t.Fatalf("expected function call part, got: %#v", fnCallPart)
	}
	expectedSig := "signature_base64"
	if string(fnCallPart.ThoughtSignature) != expectedSig {
		t.Fatalf("expected thought signature %q, got %q", expectedSig, string(fnCallPart.ThoughtSignature))
	}
}

func TestToGenAIContentToolCallsAndResponses(t *testing.T) {
	callItems := provider.AssistantToolCallItems([]provider.ToolInvocation{
		{Kind: provider.InvokeCustomTool, Name: "bash", Arguments: json.RawMessage(`{"input":"ls"}`), CallID: "call_123"},
	})
	contents := toGenAIContent(callItems)
	if len(contents) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(contents))
	}
	content := contents[0]
	if content.Role != "model" || len(content.Parts) != 1 || content.Parts[0].FunctionCall == nil {
		t.Fatalf("expected function call part, got: %#v", content)
	}
	fnCall := content.Parts[0].FunctionCall
	if fnCall.Name != "bash" || fnCall.ID != "call_123" || fnCall.Args["input"] != "ls" {
		t.Fatalf("unexpected function call properties: %#v", fnCall)
	}

	resultItem := provider.ToolResultForInvocation(provider.ToolInvocation{Name: "bash", CallID: "call_123"}, "main.go")
	contents = toGenAIContent([]provider.ConversationItem{resultItem})
	if len(contents) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(contents))
	}
	content = contents[0]
	if content.Role != "user" || len(content.Parts) != 1 || content.Parts[0].FunctionResponse == nil {
		t.Fatalf("expected function response part, got: %#v", content)
	}
	fnResp := content.Parts[0].FunctionResponse
	if fnResp.Name != "bash" || fnResp.ID != "call_123" || fnResp.Response["output"] != "main.go" {
		t.Fatalf("unexpected function response properties: %#v", fnResp)
	}
}

func TestBuildGenerateContentConfigTools(t *testing.T) {
	client := &geminiClient{
		model:               "gemini-2.5-flash",
		thinkingBudget:      1024,
		enableGoogleSearch:  true,
		enableCodeExecution: true,
		supportsThinking:    true,
	}

	tools := provider.ToolSet{Local: []provider.LocalToolDefinition{{Name: "bash", Description: "Run command"}}}
	genaiConfig := client.buildGenerateContentConfig(context.Background(), "system instruction", tools)

	if genaiConfig.SystemInstruction == nil || genaiConfig.SystemInstruction.Parts[0].Text != "system instruction" {
		t.Fatalf("unexpected system instruction: %#v", genaiConfig.SystemInstruction)
	}

	if genaiConfig.ThinkingConfig == nil {
		t.Fatalf("expected ThinkingConfig to be set")
	}
	if genaiConfig.ThinkingConfig.ThinkingBudget == nil || *genaiConfig.ThinkingConfig.ThinkingBudget != 1024 {
		t.Fatalf("expected ThinkingBudget to be 1024, got: %v", genaiConfig.ThinkingConfig.ThinkingBudget)
	}
	if genaiConfig.ThinkingConfig.ThinkingLevel != "" {
		t.Fatalf("expected ThinkingLevel to be empty, got: %q", genaiConfig.ThinkingConfig.ThinkingLevel)
	}

	if len(genaiConfig.Tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(genaiConfig.Tools))
	}

	var hasSearch, hasCode, hasFunc bool
	for _, tool := range genaiConfig.Tools {
		if tool.GoogleSearch != nil {
			hasSearch = true
		}
		if tool.CodeExecution != nil {
			hasCode = true
		}
		if len(tool.FunctionDeclarations) == 1 && tool.FunctionDeclarations[0].Name == "bash" {
			hasFunc = true
		}
	}

	if !hasSearch || !hasCode || !hasFunc {
		t.Fatalf("missing one of the expected tools: search=%t, code=%t, func=%t", hasSearch, hasCode, hasFunc)
	}
}

func TestNewGeminiClientInitializesFields(t *testing.T) {
	cfg := config.ProviderConfig{
		Name:                "gemini",
		APIKey:              "key",
		Model:               "gemini-2.5-flash",
		ThinkingBudget:      512,
		EnableGoogleSearch:  true,
		EnableCodeExecution: true,
	}

	client := NewClient(cfg)
	gClient, ok := client.(*geminiClient)
	if !ok {
		t.Fatalf("expected *geminiClient, got %T", client)
	}

	if gClient.thinkingBudget != 512 || !gClient.enableGoogleSearch || !gClient.enableCodeExecution {
		t.Fatalf("unexpected fields values: %+v", gClient)
	}
}

func TestBuildGenerateContentConfigThinkingLevel(t *testing.T) {
	client := &geminiClient{
		model:            "gemini-3.5-flash",
		thinkingBudget:   8192,
		supportsThinking: true,
	}

	genaiConfig := client.buildGenerateContentConfig(context.Background(), "instruction", provider.ToolSet{})
	if genaiConfig.ThinkingConfig == nil {
		t.Fatalf("expected ThinkingConfig to be set")
	}

	if !genaiConfig.ThinkingConfig.IncludeThoughts {
		t.Errorf("expected IncludeThoughts to be true")
	}

	if genaiConfig.ThinkingConfig.ThinkingBudget != nil {
		t.Errorf("expected ThinkingBudget to be nil for Gemini 3.5+, got: %v", *genaiConfig.ThinkingConfig.ThinkingBudget)
	}

	if genaiConfig.ThinkingConfig.ThinkingLevel != "medium" {
		t.Errorf("expected ThinkingLevel to be 'medium', got: %q", genaiConfig.ThinkingConfig.ThinkingLevel)
	}
}

func TestGeminiClientConfiguredWithoutBaseURL(t *testing.T) {
	client, err := provider.NewClient(config.ProviderConfig{Preset: config.ProviderPresetGemini, APIKey: "k", Model: "gemini-3.5-flash"})
	if err != nil {
		t.Fatal(err)
	}
	gc, ok := client.(*geminiClient)
	if !ok {
		t.Fatalf("expected *geminiClient, got %T", client)
	}
	if !gc.Configured() {
		t.Fatal("gemini client with API key + model should be Configured()")
	}
	if gc.Summary() != "gemini:gemini-3.5-flash" {
		t.Fatalf("unexpected summary %q", gc.Summary())
	}
}
