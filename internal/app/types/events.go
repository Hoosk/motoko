package types

import "time"

type TaskEvent struct {
	ID       string
	Command  string
	Output   string
	ExitCode int
	Duration time.Duration
	Done     bool
}

type TaskEventResult struct {
	Event TaskEvent
	OK    bool
}

type AgentStreamEvent struct {
	Kind             string
	Title            string
	Content          string
	ReasoningContent string
}

type SubagentInfo struct {
	StartedAt time.Time
	Name      string
	Prompt    string
	Progress  string
}
