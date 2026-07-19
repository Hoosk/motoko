// Package mcp implements a minimal Model Context Protocol (spec 2025-11-25)
// client. It currently supports:
//
//   - initialize / initialized / ping lifecycle
//   - tools/list and tools/call
//   - notifications/tools/list_changed
//   - stdio transport (Streamable HTTP is scheduled for a later phase)
//
// Every JSON-RPC envelope (requests, responses, notifications) lives here so
// other phases can extend the package without touching the transport layer.
package mcp

import (
	"encoding/json"
	"fmt"
)

// ProtocolVersion is the MCP spec revision this client targets.
const ProtocolVersion = "2025-11-25"

// jsonRPCVersion is the JSON-RPC envelope version. Per RFC 2.0 every envelope
// must carry it; we keep the literal in one place to satisfy goconst.
const jsonRPCVersion = "2.0"
const jsonRPCField = "jsonrpc"

// JSON-RPC 2.0 error codes reserved by the protocol.
const (
	ErrCodeParseError       = -32700
	ErrCodeInvalidRequest   = -32600
	ErrCodeMethodNotFound   = -32601
	ErrCodeInvalidParams    = -32602
	ErrCodeInternalError    = -32603
	ErrCodeRequestCancelled = -32800
)

// RPCEnvelope is the union of the three JSON-RPC 2.0 message types.
//
// Go's encoding/json cannot directly discriminate a tagged union, so the
// concrete payload structs below embed RawMessage where the spec allows
// extension fields that Motoko does not need to interpret yet.
type RPCEnvelope struct {
	ID      *RequestID      `json:"id,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}

// IsResponse reports whether the envelope is a non-error response.
func (e RPCEnvelope) IsResponse() bool { return e.ID != nil && e.Result != nil && e.Error == nil }

// IsErrorResponse reports whether the envelope is an error response.
func (e RPCEnvelope) IsErrorResponse() bool { return e.ID != nil && e.Error != nil }

// IsNotification reports whether the envelope is a notification (no id).
func (e RPCEnvelope) IsNotification() bool { return e.ID == nil && e.Method != "" }

// IsRequest reports whether the envelope is an in-band request.
func (e RPCEnvelope) IsRequest() bool { return e.ID != nil && e.Method != "" }

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Data    any    `json:"data,omitempty"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// Error implements the error interface so RPCError can be returned directly.
func (e *RPCError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("mcp: rpc error %d: %s", e.Code, e.Message)
}

// RequestID is the JSON-RPC 2.0 id type. Spec says it MUST be a string or
// integer and MUST NOT be null. We expose both via a discriminated union
// encoded as RawMessage in RPCEnvelope.ID.
type RequestID struct {
	raw json.RawMessage
}

// NewStringID returns a RequestID wrapping a string.
func NewStringID(s string) RequestID {
	b, _ := json.Marshal(s)
	return RequestID{raw: b}
}

// NewIntID returns a RequestID wrapping an int64.
func NewIntID(n int64) RequestID {
	b, _ := json.Marshal(n)
	return RequestID{raw: b}
}

// Raw returns the encoded value, suitable for embedding in a JSON-RPC payload.
func (r RequestID) Raw() json.RawMessage { return r.raw }

// String returns a human readable form for logs.
func (r RequestID) String() string {
	if len(r.raw) == 0 {
		return ""
	}
	return string(r.raw)
}

// MarshalJSON implements json.Marshaler so the id round-trips verbatim.
func (r RequestID) MarshalJSON() ([]byte, error) {
	if len(r.raw) == 0 {
		return []byte("null"), nil
	}
	return r.raw, nil
}

// UnmarshalJSON accepts any JSON scalar (string, number) and stores it as raw
// bytes. Per the spec the id MUST NOT be null; we tolerate the wire value for
// robustness.
func (r *RequestID) UnmarshalJSON(data []byte) error {
	r.raw = append(r.raw[:0], data...)
	return nil
}

// Implementation describes a client or server in the MCP handshake.
type Implementation struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
}

// ClientCapabilities lists features the Motoko client supports.
type ClientCapabilities struct {
	Experimental map[string]any `json:"experimental,omitempty"`
	Roots        *struct {
		ListChanged bool `json:"listChanged,omitempty"`
	} `json:"roots,omitempty"`
	Sampling    *struct{} `json:"sampling,omitempty"`
	Elicitation *struct{} `json:"elicitation,omitempty"`
	Tasks       *struct {
		List     *struct{} `json:"list,omitempty"`
		Cancel   *struct{} `json:"cancel,omitempty"`
		Requests *struct{} `json:"requests,omitempty"`
	} `json:"tasks,omitempty"`
}

// ServerCapabilities lists features the MCP server exposes.
type ServerCapabilities struct {
	Experimental map[string]any `json:"experimental,omitempty"`
	Logging      *struct{}      `json:"logging,omitempty"`
	Completions  *struct{}      `json:"completions,omitempty"`
	Prompts      *struct {
		ListChanged bool `json:"listChanged,omitempty"`
	} `json:"prompts,omitempty"`
	Resources *struct {
		Subscribe   bool `json:"subscribe,omitempty"`
		ListChanged bool `json:"listChanged,omitempty"`
	} `json:"resources,omitempty"`
	Tools *struct {
		ListChanged bool `json:"listChanged,omitempty"`
	} `json:"tools,omitempty"`
	Tasks *struct {
		List     *struct{} `json:"list,omitempty"`
		Cancel   *struct{} `json:"cancel,omitempty"`
		Requests *struct {
			Tools *struct {
				Call *struct{} `json:"call,omitempty"`
			} `json:"tools,omitempty"`
		} `json:"requests,omitempty"`
	} `json:"tasks,omitempty"`
}

// InitializeParams is sent as `params` of the `initialize` request.
type InitializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      Implementation     `json:"clientInfo"`
}

// InitializeResult is the response of the `initialize` request.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      Implementation     `json:"serverInfo"`
	Instructions    string             `json:"instructions,omitempty"`
}

// Icon is a sized icon the client can render. Captured for future UI use;
// Motoko does not yet surface icons in the TUI.
type Icon struct {
	Src      string   `json:"src"`
	MimeType string   `json:"mimeType,omitempty"`
	Theme    string   `json:"theme,omitempty"`
	Sizes    []string `json:"sizes,omitempty"`
}

// ToolAnnotations mirrors the optional ToolAnnotations fields from the spec.
type ToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    bool   `json:"readOnlyHint,omitempty"`
	DestructiveHint bool   `json:"destructiveHint,omitempty"`
	IdempotentHint  bool   `json:"idempotentHint,omitempty"`
	OpenWorldHint   bool   `json:"openWorldHint,omitempty"`
}

// Tool describes a tool the server exposes.
type Tool struct {
	Name         string           `json:"name"`
	Title        string           `json:"title,omitempty"`
	Description  string           `json:"description,omitempty"`
	InputSchema  json.RawMessage  `json:"inputSchema"`
	OutputSchema json.RawMessage  `json:"outputSchema,omitempty"`
	Annotations  *ToolAnnotations `json:"annotations,omitempty"`
	Icons        []Icon           `json:"icons,omitempty"`
	Meta         json.RawMessage  `json:"_meta,omitempty"`
}

// ListToolsResult is the response of `tools/list`.
type ListToolsResult struct {
	Tools      []Tool          `json:"tools"`
	NextCursor string          `json:"nextCursor,omitempty"`
	Meta       json.RawMessage `json:"_meta,omitempty"`
}

// CallToolParams is the parameters object for `tools/call`.
type CallToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

// ContentBlock is a single item inside a CallToolResult. Motoko only cares
// about the type discriminator and the string payload for now; structured
// content and binary blobs are preserved as RawMessage.
type ContentBlock struct {
	Type        string          `json:"type"`
	Text        string          `json:"text,omitempty"`
	Data        string          `json:"data,omitempty"`
	MimeType    string          `json:"mimeType,omitempty"`
	Name        string          `json:"name,omitempty"`
	URI         string          `json:"uri,omitempty"`
	Title       string          `json:"title,omitempty"`
	Resource    json.RawMessage `json:"resource,omitempty"`
	Annotations json.RawMessage `json:"annotations,omitempty"`
	Meta        json.RawMessage `json:"_meta,omitempty"`
}

// CallToolResult is the response of `tools/call`.
type CallToolResult struct {
	Content           []ContentBlock  `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
	Meta              json.RawMessage `json:"_meta,omitempty"`
	IsError           bool            `json:"isError,omitempty"`
}

// Resource describes a resource exposed by the server.
type Resource struct {
	URI         string          `json:"uri"`
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	MimeType    string          `json:"mimeType,omitempty"`
	Icons       []Icon          `json:"icons,omitempty"`
	Annotations json.RawMessage `json:"annotations,omitempty"`
	Size        int64           `json:"size,omitempty"`
}

// ResourceTemplate describes a parameterized resource template.
type ResourceTemplate struct {
	URITemplate string          `json:"uriTemplate"`
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	MimeType    string          `json:"mimeType,omitempty"`
	Icons       []Icon          `json:"icons,omitempty"`
	Annotations json.RawMessage `json:"annotations,omitempty"`
}

// ResourceContent contains the content of a resource.
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}

// ListResourcesResult is the response of `resources/list`.
type ListResourcesResult struct {
	Resources  []Resource      `json:"resources"`
	NextCursor string          `json:"nextCursor,omitempty"`
	Meta       json.RawMessage `json:"_meta,omitempty"`
}

// ListResourceTemplatesResult is the response of `resources/templates/list`.
type ListResourceTemplatesResult struct {
	ResourceTemplates []ResourceTemplate `json:"resourceTemplates"`
	NextCursor        string             `json:"nextCursor,omitempty"`
	Meta              json.RawMessage    `json:"_meta,omitempty"`
}

// ReadResourceResult is the response of `resources/read`.
type ReadResourceResult struct {
	Contents []ResourceContent `json:"contents"`
	Meta     json.RawMessage   `json:"_meta,omitempty"`
}

// PromptArgument describes an argument accepted by a prompt template.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// Prompt describes a prompt template.
type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptMessage represents a message in a prompt.
type PromptMessage struct {
	Role    string       `json:"role"`
	Content ContentBlock `json:"content"`
}

// ListPromptsResult is the response of `prompts/list`.
type ListPromptsResult struct {
	Prompts    []Prompt        `json:"prompts"`
	NextCursor string          `json:"nextCursor,omitempty"`
	Meta       json.RawMessage `json:"_meta,omitempty"`
}

// GetPromptResult is the response of `prompts/get`.
type GetPromptResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
	Meta        json.RawMessage `json:"_meta,omitempty"`
}

// Root describes a root directory/URI the server can operate in.
type Root struct {
	URI  string `json:"uri"`
	Name string `json:"name,omitempty"`
}

// ListRootsResult is the response of `roots/list`.
type ListRootsResult struct {
	Roots []Root          `json:"roots"`
	Meta  json.RawMessage `json:"_meta,omitempty"`
}

// SamplingMessage represents a message for sampling/createMessage.
type SamplingMessage struct {
	Role    string       `json:"role"`
	Content ContentBlock `json:"content"`
}

// ModelPreferences hints the client on model selection.
type ModelPreferences struct {
	ModelHint string  `json:"modelHint,omitempty"`
	MinScore  float64 `json:"minScore,omitempty"`
	Speed     float64 `json:"speed,omitempty"`
	Cost      float64 `json:"cost,omitempty"`
}

// CreateMessageParams is sent by the server for `sampling/createMessage`.
type CreateMessageParams struct {
	ModelPreferences *ModelPreferences `json:"modelPreferences,omitempty"`
	SystemPrompt     string            `json:"systemPrompt,omitempty"`
	IncludeContext   string            `json:"includeContext,omitempty"`
	Messages         []SamplingMessage `json:"messages"`
	StopSequences    []string          `json:"stopSequences,omitempty"`
	Temperature      float64           `json:"temperature,omitempty"`
	MaxTokens        int               `json:"maxTokens"`
}

// CreateMessageResult is returned by the client for `sampling/createMessage`.
type CreateMessageResult struct {
	Model      string          `json:"model"`
	Role       string          `json:"role"`
	StopReason string          `json:"stopReason,omitempty"`
	Content    ContentBlock    `json:"content"`
	Meta       json.RawMessage `json:"meta,omitempty"`
}
