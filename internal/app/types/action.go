package types

type ActionType string

const (
	ActionShell   ActionType = "shell"
	ActionTask    ActionType = "task"
	ActionAgent   ActionType = "agent"
	ActionCompact ActionType = "compact"
)

type Action struct {
	Type         ActionType
	ShellCommand string
	TaskCommand  string
	AgentPrompt  string
}
