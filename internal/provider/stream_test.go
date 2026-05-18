package provider

import "testing"

func TestParseStreamResponseStructuredMessage(t *testing.T) {
	resp := parseStreamResponse(`{"message":"hola"}`, Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3})
	if resp.Message != "hola" {
		t.Fatalf("expected parsed message, got %#v", resp)
	}
	if resp.Usage.TotalTokens != 3 {
		t.Fatalf("expected usage preserved, got %#v", resp.Usage)
	}
}

func TestParseStreamResponseStructuredToolCall(t *testing.T) {
	resp := parseStreamResponse(`{"tool_name":"read","tool_input":"README.md 1 20"}`, Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3})
	if resp.ToolCall == nil || resp.ToolCall.Name != "read" {
		t.Fatalf("expected parsed tool call, got %#v", resp)
	}
}
