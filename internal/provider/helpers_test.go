package provider

import (
	"strings"
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

func TestFormatToolResultContentIncludesMetadata(t *testing.T) {
	call := ToolInvocation{Name: "read", Input: "README.md", Arguments: []byte(`{"input":"README.md"}`), CallID: "call_123"}
	got := FormatToolResultContent(call, "ok")
	if !strings.Contains(got, "tool_name=read") || !strings.Contains(got, "call_id=call_123") || !strings.Contains(got, "tool_input=README.md") {
		t.Fatalf("expected metadata in formatted tool result, got %q", got)
	}
}

func TestParseToolResultContentRoundTripsMetadata(t *testing.T) {
	call := ToolInvocation{Name: "read", Input: "README.md", Arguments: []byte(`{"input":"README.md"}`), CallID: "call_123"}
	parsedCall, output := ParseToolResultContent(FormatToolResultContent(call, "ok"))
	if parsedCall.Name != call.Name || parsedCall.Input != call.Input || parsedCall.CallID != call.CallID {
		t.Fatalf("unexpected parsed tool call %#v", parsedCall)
	}
	if string(parsedCall.Arguments) != string(call.Arguments) {
		t.Fatalf("unexpected parsed arguments %s", string(parsedCall.Arguments))
	}
	if output != "ok" {
		t.Fatalf("unexpected parsed output %q", output)
	}
}

func TestAssistantToolCallContentRoundTrips(t *testing.T) {
	call := ToolInvocation{Kind: InvokeCustomTool, Name: "read", Input: "README.md", Arguments: []byte(`{"input":"README.md"}`), CallID: "call_123", Raw: []byte(`{"id":"call_123","type":"function","function":{"name":"read","arguments":"{\"input\":\"README.md\"}"},"thought_signature":"sig"}`)}
	parsed, ok := ParseAssistantToolCallContent(FormatAssistantToolCallContent(call))
	if !ok {
		t.Fatal("expected assistant tool call metadata")
	}
	if parsed.Name != call.Name || parsed.Input != call.Input || parsed.CallID != call.CallID {
		t.Fatalf("unexpected parsed call %#v", parsed)
	}
	if string(parsed.Arguments) != string(call.Arguments) {
		t.Fatalf("unexpected parsed arguments %s", string(parsed.Arguments))
	}
	if string(parsed.Raw) != string(call.Raw) {
		t.Fatalf("unexpected parsed raw payload %s", string(parsed.Raw))
	}
}
