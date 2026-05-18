package ui

import "github.com/Hoosk/motoko/internal/app"

// SubmitPromptMsg is emitted by the Composer when the user presses Enter
// to submit a valid prompt or command.
type SubmitPromptMsg struct {
	Prompt string
}

// ClearMessagesMsg is emitted to instruct the Timeline to clear its history.
type ClearMessagesMsg struct{}

// ResponseAppliedMsg is emitted after a response has been processed
// to coordinate UI updates (like refreshing suggestions).
type ResponseAppliedMsg struct {
	Response app.Response
}

type AgentStreamEventMsg struct {
	Event app.AgentStreamEvent
	Done  bool
}

type AgentStreamBatchMsg struct {
	Events []app.AgentStreamEvent
	Done   bool
}

type finalizeStreamMsg struct {
	Text string
}

type CopySelectionMsg struct{ Err error }
