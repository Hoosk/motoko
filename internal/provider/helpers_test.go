package provider

import (
	"context"
	"testing"
)

func TestNormalizeConversationRoleFallsBackToUser(t *testing.T) {
	if got := NormalizeConversationRole(" TOOL "); got != RoleUser {
		t.Fatalf("expected fallback user role, got %q", got)
	}
	if got := NormalizeConversationRole("assistant"); got != RoleAssistant {
		t.Fatalf("expected assistant role, got %q", got)
	}
	if got := NormalizeConversationRole("system"); got != RoleSystem {
		t.Fatalf("expected system role, got %q", got)
	}
}

func TestToolResultForInvocationUsesStructuredFields(t *testing.T) {
	call := ToolInvocation{Name: "read", Input: "README.md", Arguments: []byte(`{"input":"README.md"}`), CallID: "call_123"}
	got := ToolResultForInvocation(call, "ok")
	if got.Role != RoleTool || got.ToolName != "read" || got.ToolCallID != "call_123" || got.Content != "ok" {
		t.Fatalf("unexpected structured tool result: %#v", got)
	}
}

func TestAssistantTurnCarriesReasoningAndToolCalls(t *testing.T) {
	call := ToolInvocation{Kind: InvokeCustomTool, Name: "read", CallID: "call_123"}
	got := AssistantTurn("working", "thinking", []ToolInvocation{call})
	if got.Role != RoleAssistant || got.Content != "working" || got.ReasoningContent != "thinking" {
		t.Fatalf("unexpected assistant turn: %#v", got)
	}
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].Name != "read" {
		t.Fatalf("expected tool calls to be preserved: %#v", got)
	}
}

func TestTelemetryRoundTrip(t *testing.T) {
	ctx := WithTelemetry(context.Background(), "sess-123", "req-456")
	sessionID, requestID := GetTelemetry(ctx)
	if sessionID != "sess-123" || requestID != "req-456" {
		t.Fatalf("unexpected telemetry values session=%q request=%q", sessionID, requestID)
	}
}

func TestApplyTelemetryHeadersOpencode(t *testing.T) {
	headers := map[string]string{}
	ApplyTelemetryHeaders("opencode-go", headers, "sess-123", "req-456")
	if headers["x-opencode-session"] != "sess-123" {
		t.Fatalf("expected x-opencode-session, got %#v", headers)
	}
	if headers["x-opencode-request"] != "req-456" {
		t.Fatalf("expected x-opencode-request, got %#v", headers)
	}
	if headers["x-opencode-client"] != "motoko" {
		t.Fatalf("expected x-opencode-client=motoko, got %#v", headers)
	}
	if _, ok := headers["X-Session-ID"]; ok {
		t.Fatalf("did not expect generic session header for opencode provider, got %#v", headers)
	}
}

func TestApplyTelemetryHeadersFallback(t *testing.T) {
	headers := map[string]string{}
	ApplyTelemetryHeaders("deepseek", headers, "sess-123", "req-456")
	if headers["X-Session-ID"] != "sess-123" || headers["X-Request-ID"] != "req-456" {
		t.Fatalf("expected generic telemetry headers, got %#v", headers)
	}
	if headers["x-session-affinity"] != "sess-123" {
		t.Fatalf("expected x-session-affinity, got %#v", headers)
	}
	if _, ok := headers["x-opencode-session"]; ok {
		t.Fatalf("did not expect opencode headers for generic provider, got %#v", headers)
	}
}

func TestInputSchemaSanitizesNullProperties(t *testing.T) {
	// A tool whose Schema explicitly contains "properties":null (e.g. from an
	// MCP server advertising a no-arg tool) must not surface null to providers.
	// DeepSeek and others reject "null is not of type 'object'" for properties.
	tool := LocalToolDefinition{
		Name:        "noop",
		Description: "A tool with no parameters",
		Schema:      []byte(`{"type":"object","properties":null,"additionalProperties":false}`),
	}
	got := InputSchema(tool)
	if v, ok := got["properties"]; ok && v == nil {
		t.Error("InputSchema must strip properties:null from verbatim schemas")
	}
	if got["type"] != "object" {
		t.Errorf("expected type=object, got %v", got["type"])
	}
}

func TestInputSchemaKeepsValidProperties(t *testing.T) {
	tool := LocalToolDefinition{
		Name:   "greet",
		Schema: []byte(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`),
	}
	got := InputSchema(tool)
	props, ok := got["properties"].(map[string]any)
	if !ok || props["name"] == nil {
		t.Errorf("InputSchema must preserve valid properties, got %v", got["properties"])
	}
}

func TestInputSchemaFallsBackToSyntheticWhenSchemaEmpty(t *testing.T) {
	tool := LocalToolDefinition{Name: "legacy", Description: "old tool", InputHint: "legacy <arg>"}
	got := InputSchema(tool)
	props, ok := got["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected synthetic properties map, got %T", got["properties"])
	}
	if _, hasInput := props["input"]; !hasInput {
		t.Error("synthetic schema must have an 'input' property")
	}
}
