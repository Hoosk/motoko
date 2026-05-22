package provider

import (
	"encoding/base64"
	"fmt"
	"strings"
)

func responseFromText(content string, usage Usage) Response {
	message := strings.TrimSpace(content)
	resp := Response{FinalText: message, Usage: usage}
	if message != "" {
		resp.OutputItems = []ConversationItem{AssistantText(message)}
	}
	return resp
}

func normalizeConversationRole(role string) string {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case RoleAssistant:
		return RoleAssistant
	case RoleSystem:
		return RoleSystem
	default:
		return RoleUser
	}
}

func formatToolResultContent(call ToolInvocation, output string) string {
	parts := []string{fmt.Sprintf("tool_name=%s", strings.TrimSpace(call.Name))}
	if call.CallID != "" {
		parts = append(parts, fmt.Sprintf("call_id=%s", strings.TrimSpace(call.CallID)))
	}
	if strings.TrimSpace(call.Input) != "" {
		parts = append(parts, fmt.Sprintf("tool_input=%s", strings.TrimSpace(call.Input)))
	}
	if len(call.Arguments) > 0 {
		parts = append(parts, "tool_arguments_base64="+base64.StdEncoding.EncodeToString(call.Arguments))
	}
	if strings.TrimSpace(output) != "" {
		parts = append(parts, "tool_output:\n"+strings.TrimSpace(output))
	}
	return strings.Join(parts, "\n")
}
