package provider

import (
	"context"
	"encoding/json"
	"strings"
)

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

const (
	ToolInputText = "text"
)

const (
	InvokeCustomTool = "custom"
)

type ToolDefinition struct {
	Name        string
	Description string
	InputHint   string
}

type LocalToolDefinition struct {
	Name        string
	Description string
	InputType   string
	InputHint   string
}

type ToolSet struct {
	Local []LocalToolDefinition
}

type ToolInvocation struct {
	Kind      string
	Name      string
	Input     string
	Arguments json.RawMessage
	CallID    string
	Raw       json.RawMessage
}

type Response struct {
	FinalText    string
	PendingCalls []ToolInvocation
	OutputItems  []ConversationItem
	Usage        Usage
}

type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

type Message struct {
	Role    string
	Content string
}

type ConversationItem = Message

func (m Message) PlainText() string {
	return m.Content
}

func UserText(content string) ConversationItem {
	return ConversationItem{Role: RoleUser, Content: content}
}

func AssistantText(content string) ConversationItem {
	return ConversationItem{Role: RoleAssistant, Content: content}
}

func ToolResultForInvocation(call ToolInvocation, output string) ConversationItem {
	call.Name = strings.TrimSpace(call.Name)
	if call.Name == "" {
		call.Name = "tool"
	}
	payload := strings.TrimSpace(output)
	if call.Arguments != nil && strings.TrimSpace(call.Input) == "" {
		payload = strings.TrimSpace(string(call.Arguments))
	}
	return ConversationItem{
		Role:    RoleTool,
		Content: formatToolResultContent(call, payload),
	}
}

type ModelInfo struct {
	ID            string
	ContextWindow int
}

type Client interface {
	Configured() bool
	Complete(ctx context.Context, systemPrompt string, messages []ConversationItem, tools ToolSet) (Response, error)
	StreamComplete(ctx context.Context, systemPrompt string, messages []ConversationItem, tools ToolSet, onDelta func(string) error) (Response, error)
	Summary() string
	ListModels(ctx context.Context) ([]ModelInfo, error)
}
