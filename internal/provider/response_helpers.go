package provider

import (
	"encoding/json"
	"strings"
)

func ResponseFromText(content string, usage Usage) Response {
	message := strings.TrimSpace(content)
	resp := Response{FinalText: message, Usage: usage}
	if message != "" {
		resp.OutputItems = []ConversationItem{AssistantText(message)}
	}
	return resp
}

func FinalizeResponse(content, reasoning string, pending []ToolInvocation, usage Usage) Response {
	content = strings.TrimSpace(content)
	reasoning = strings.TrimSpace(reasoning)
	resp := Response{
		FinalText:    content,
		PendingCalls: pending,
		Usage:        usage,
	}
	if content != "" || reasoning != "" || len(pending) > 0 {
		resp.OutputItems = []ConversationItem{AssistantTurn(content, reasoning, pending)}
	}
	if len(pending) > 0 {
		resp.FinalText = ""
	}
	return resp
}

func NormalizeConversationRole(role string) string {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case RoleAssistant:
		return RoleAssistant
	case RoleSystem:
		return RoleSystem
	default:
		return RoleUser
	}
}

func AssistantTurn(content, reasoning string, toolCalls []ToolInvocation) ConversationItem {
	item := ConversationItem{
		Role:             RoleAssistant,
		Content:          strings.TrimSpace(content),
		ReasoningContent: strings.TrimSpace(reasoning),
	}
	if len(toolCalls) > 0 {
		item.ToolCalls = append([]ToolInvocation(nil), toolCalls...)
	}
	return item
}

func ToolResultForInvocation(call ToolInvocation, output string) ConversationItem {
	call.Name = strings.TrimSpace(call.Name)
	if call.Name == "" {
		call.Name = "tool"
	}
	return ConversationItem{
		Role:       RoleTool,
		Content:    strings.TrimSpace(output),
		ToolCallID: strings.TrimSpace(call.CallID),
		ToolName:   call.Name,
	}
}

func AssistantToolCallItems(calls []ToolInvocation) []ConversationItem {
	if len(calls) == 0 {
		return nil
	}
	items := make([]ConversationItem, 0, len(calls))
	for _, call := range calls {
		items = append(items, AssistantTurn("", "", []ToolInvocation{call}))
	}
	return items
}

func AssistantToolCallArguments(call ToolInvocation) string {
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

func ToolInputDescription(tool LocalToolDefinition) string {
	hint := strings.TrimSpace(tool.InputHint)
	if hint != "" {
		prefix := tool.Name + " "
		if strings.HasPrefix(strings.ToLower(hint), strings.ToLower(prefix)) {
			hint = strings.TrimSpace(hint[len(prefix):])
		}
		return hint
	}
	if desc := strings.TrimSpace(tool.Description); desc != "" {
		return desc
	}
	return "Raw text input for the tool."
}
