package types

type Mode string

const (
	ModePlan  Mode = "plan"
	ModeBuild Mode = "build"
)

type InputMode string

const (
	InputModeChat  InputMode = "chat"
	InputModeShell InputMode = "shell"
)
