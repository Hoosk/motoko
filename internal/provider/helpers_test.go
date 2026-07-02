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
