package provider

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

const assistantToolCallPrefix = "__motoko_tool_call__"

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

func parseToolResultContent(content string) (ToolInvocation, string) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return ToolInvocation{}, ""
	}

	var (
		call       ToolInvocation
		output     string
		outputSeen bool
	)
	lines := strings.Split(trimmed, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		switch {
		case line == "tool_output:":
			outputSeen = true
			if i+1 < len(lines) {
				output = strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
			}
			i = len(lines)
		case strings.HasPrefix(line, "tool_output:"):
			outputSeen = true
			output = strings.TrimSpace(strings.TrimPrefix(line, "tool_output:"))
			if i+1 < len(lines) {
				tail := strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
				if tail != "" {
					if output != "" {
						output += "\n" + tail
					} else {
						output = tail
					}
				}
			}
			i = len(lines)
		case strings.HasPrefix(line, "tool_name="):
			call.Name = strings.TrimSpace(strings.TrimPrefix(line, "tool_name="))
		case strings.HasPrefix(line, "call_id="):
			call.CallID = strings.TrimSpace(strings.TrimPrefix(line, "call_id="))
		case strings.HasPrefix(line, "tool_input="):
			call.Input = strings.TrimSpace(strings.TrimPrefix(line, "tool_input="))
		case strings.HasPrefix(line, "tool_arguments_base64="):
			encoded := strings.TrimSpace(strings.TrimPrefix(line, "tool_arguments_base64="))
			if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil && len(decoded) > 0 {
				call.Arguments = decoded
			}
		}
	}
	if !outputSeen {
		output = trimmed
	}
	return call, output
}

func ParseToolResultContent(content string) (ToolInvocation, string) {
	return parseToolResultContent(content)
}

func formatAssistantToolCallContent(call ToolInvocation) string {
	parts := []string{assistantToolCallPrefix, fmt.Sprintf("tool_name=%s", strings.TrimSpace(call.Name))}
	if call.CallID != "" {
		parts = append(parts, fmt.Sprintf("call_id=%s", strings.TrimSpace(call.CallID)))
	}
	if strings.TrimSpace(call.Input) != "" {
		parts = append(parts, fmt.Sprintf("tool_input=%s", strings.TrimSpace(call.Input)))
	}
	if len(call.Arguments) > 0 {
		parts = append(parts, "tool_arguments_base64="+base64.StdEncoding.EncodeToString(call.Arguments))
	}
	if len(call.Raw) > 0 {
		parts = append(parts, "tool_raw_base64="+base64.StdEncoding.EncodeToString(call.Raw))
	}
	return strings.Join(parts, "\n")
}

func parseAssistantToolCallContent(content string) (ToolInvocation, bool) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, assistantToolCallPrefix) {
		return ToolInvocation{}, false
	}
	lines := strings.Split(trimmed, "\n")
	call := ToolInvocation{Kind: InvokeCustomTool}
	for _, line := range lines[1:] {
		switch {
		case strings.HasPrefix(line, "tool_name="):
			call.Name = strings.TrimSpace(strings.TrimPrefix(line, "tool_name="))
		case strings.HasPrefix(line, "call_id="):
			call.CallID = strings.TrimSpace(strings.TrimPrefix(line, "call_id="))
		case strings.HasPrefix(line, "tool_input="):
			call.Input = strings.TrimSpace(strings.TrimPrefix(line, "tool_input="))
		case strings.HasPrefix(line, "tool_arguments_base64="):
			encoded := strings.TrimSpace(strings.TrimPrefix(line, "tool_arguments_base64="))
			if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil && len(decoded) > 0 {
				call.Arguments = decoded
			}
		case strings.HasPrefix(line, "tool_raw_base64="):
			encoded := strings.TrimSpace(strings.TrimPrefix(line, "tool_raw_base64="))
			if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil && len(decoded) > 0 {
				call.Raw = decoded
			}
		}
	}
	if call.Name == "" {
		return ToolInvocation{}, false
	}
	return call, true
}

func ParseAssistantToolCallContent(content string) (ToolInvocation, bool) {
	return parseAssistantToolCallContent(content)
}

func assistantToolCallItems(calls []ToolInvocation) []ConversationItem {
	if len(calls) == 0 {
		return nil
	}
	items := make([]ConversationItem, 0, len(calls))
	for _, call := range calls {
		items = append(items, ConversationItem{Role: RoleAssistant, Content: formatAssistantToolCallContent(call)})
	}
	return items
}

func assistantToolCallArguments(call ToolInvocation) string {
	if arguments := strings.TrimSpace(string(call.Arguments)); arguments != "" {
		return arguments
	}
	payload, err := json.Marshal(struct {
		Input string `json:"input"`
	}{Input: strings.TrimSpace(call.Input)})
	if err != nil {
		return `{}`
	}
	return string(payload)
}
