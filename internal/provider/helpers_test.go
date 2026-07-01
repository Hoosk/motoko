package provider

import "testing"

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
