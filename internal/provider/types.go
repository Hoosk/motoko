package provider

import (
	"context"
	"encoding/json"
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
	Kind      string          `json:"kind,omitempty"`
	Name      string          `json:"name"`
	Input     string          `json:"input,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Raw       json.RawMessage `json:"raw,omitempty"`
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

	// Character counts for breakdown estimation
	SystemStaticChars  int
	SystemDynamicChars int
	ToolsChars         int
	HistoryChars       int
}

type Message struct {
	Role             string           `json:"role"`
	Content          string           `json:"content,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolInvocation `json:"tool_calls,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
	ToolName         string           `json:"tool_name,omitempty"`
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

type Delta struct {
	Content          string
	ReasoningContent string
}

type ModelInfo struct {
	ID               string   `json:"id"`
	ContextWindow    int      `json:"context_window,omitempty"`
	SupportsThinking bool     `json:"supports_thinking,omitempty"`
	EffortPresets    []string `json:"effort_presets,omitempty"`
	BudgetMin        int      `json:"budget_min,omitempty"`
	BudgetMax        int      `json:"budget_max,omitempty"`
}

type ReasoningOption struct {
	Type   string   `json:"type"`
	Values []string `json:"values,omitempty"`
	Min    int      `json:"min,omitempty"`
	Max    int      `json:"max,omitempty"`
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
