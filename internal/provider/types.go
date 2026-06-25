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

	schemaInput                = "input"
	schemaDescription          = "description"
	schemaObject               = "object"
	schemaProperties           = "properties"
	schemaRequired             = "required"
	schemaAdditionalProperties = "additionalProperties"
	schemaString               = "string"

	keyRole     = "role"
	schemaType  = "type"
	keyModel    = "model"
	keyContent  = "content"
	keyFunction = "function"
	keyName     = "name"
	valHigh     = "high"
	valMedium   = "medium"
	valLow      = "low"
)

const (
	ToolInputText = "text"
)

const (
	InvokeCustomTool = "custom"
)

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

	ReasoningTokens       int
	CacheReadInputTokens  int
	CacheWriteInputTokens int
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
		Content: FormatToolResultContent(call, payload),
	}
}

type Delta struct {
	Content          string
	ReasoningContent string
}

type ModelInfo struct {
	ID               string
	ContextWindow    int
	SupportsThinking bool
}

type BatchRequestItem struct {
	CustomID     string
	SystemPrompt string
	Messages     []ConversationItem
	Tools        ToolSet
}

type BatchResponse struct {
	ID               string
	ProcessingStatus string
	ResultsURL       string
	ProcessingCount  int
	SucceededCount   int
	ErroredCount     int
}

type BatchClient interface {
	CreateBatch(ctx context.Context, requests []BatchRequestItem) (BatchResponse, error)
	RetrieveBatch(ctx context.Context, batchID string) (BatchResponse, error)
	CancelBatch(ctx context.Context, batchID string) (BatchResponse, error)
}

type Client interface {
	Configured() bool
	ProviderKind() string
	Complete(ctx context.Context, systemPrompt string, messages []ConversationItem, tools ToolSet) (Response, error)
	StreamComplete(ctx context.Context, systemPrompt string, messages []ConversationItem, tools ToolSet, onDelta func(Delta) error) (Response, error)
	Summary() string
	ListModels(ctx context.Context) ([]ModelInfo, error)
	GetModel(ctx context.Context, model string) (ModelInfo, error)
}

type telemetryKey string

const (
	sessionIDKey telemetryKey = "session_id"
	requestIDKey telemetryKey = "request_id"
)

func WithTelemetry(ctx context.Context, sessionID, requestID string) context.Context {
	if sessionID != "" {
		ctx = context.WithValue(ctx, sessionIDKey, sessionID)
	}
	if requestID != "" {
		ctx = context.WithValue(ctx, requestIDKey, requestID)
	}
	return ctx
}

func GetTelemetry(ctx context.Context) (string, string) {
	sessionID, _ := ctx.Value(sessionIDKey).(string)
	requestID, _ := ctx.Value(requestIDKey).(string)
	return sessionID, requestID
}
