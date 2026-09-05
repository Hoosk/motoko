package types

type ActionType string

const (
	ActionShell         ActionType = "shell"
	ActionShellApproval ActionType = "shell_approval"
	ActionTask          ActionType = "task"
	ActionAgent         ActionType = "agent"
	ActionCompact       ActionType = "compact"
	ActionTool          ActionType = "tool"
)

type Action struct {
	Type         ActionType
	ShellCommand string
	ShellReason  string
	TaskCommand  string
	AgentPrompt  string
	ToolName     string
	ToolArgs     string
}
