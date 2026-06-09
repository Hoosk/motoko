package ui

import (
	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/session"
	"github.com/Hoosk/motoko/internal/system"
)

type TachikomaStatusMsg struct {
	Statuses map[string]string
}

type ContextInfoMsg struct {
	Info system.ContextInfo
}

type ContextTokensMsg struct {
	Tokens int
	Window int
}

type CopySelectionMsg struct {
	Err error
}

type ClearMessagesMsg struct{}

type NotificationMsg struct {
	Text string
}

type ErrorMsg struct {
	Err error
}

type ResponseAppliedMsg struct {
	Response app.Response
}

type AgentStreamEventMsg struct {
	Event app.AgentStreamEvent
}

type ThinkingTickMsg struct{}

type ProviderModelsMsg struct {
	Err    error
	Models []provider.ModelInfo
}

type finalizeStreamMsg struct {
	Text string
}

type SubmitPromptMsg struct {
	Prompt string
}

type CompactResultMsg struct {
	Err      error
	Response app.Response
}

type AgentStreamBatchMsg struct {
	Events []app.AgentStreamEvent
	Done   bool
}

type SessionsMsg struct {
	Err      error
	Sessions []*session.Session
}

type AgentChangedMsg struct {
	Name  string
	Agent string
}

type SessionLoadedMsg struct {
	Session *session.Session
	Err     error
}

type ModelChangedMsg struct {
	Model string
}
