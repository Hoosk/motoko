package ui

import (
	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/app/scheduleman"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/session"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tools"
	"github.com/Hoosk/motoko/internal/updater"
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
	RequestID int
	Events    []app.AgentStreamEvent
	Done      bool
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

type UpdateAvailableMsg struct {
	Info *updater.VersionInfo
}

type ModelSelectedMsg struct {
	Model provider.ModelInfo
}

type ThinkingBudgetSelectedMsg struct {
	Model  provider.ModelInfo
	Budget int
}

type QuestionAskedMsg struct {
	Pending *tools.PendingQuestion
}

type ScheduleEventMsg struct {
	Event scheduleman.Event
}
