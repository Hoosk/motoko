package types

import "time"

type ShellDecision struct {
	Reason           string
	RequiresApproval bool
	Deny             bool
}

type ShellResult struct {
	Command  string
	Output   string
	ExitCode int
	Duration time.Duration
}
