package commands

import (
	"github.com/Hoosk/motoko/internal/system"

	"github.com/Hoosk/motoko/internal/app/types"
)

type Invocation struct {
	RawInput string
	Args     []string
	Info     system.ContextInfo
}

type Handler func(Invocation) types.Response

type Definition struct {
	Name    string
	Usage   string
	Summary string
}

type Command struct {
	Definition
	Handler Handler
}

type Registry struct {
	ordered []Command
	index   map[string]int
}

func NewRegistry() *Registry {
	return &Registry{index: make(map[string]int)}
}

func (r *Registry) Add(cmd Command) {
	r.index[cmd.Name] = len(r.ordered)
	r.ordered = append(r.ordered, cmd)
}

func (r *Registry) Lookup(name string) (Command, bool) {
	idx, ok := r.index[name]
	if !ok {
		return Command{}, false
	}
	return r.ordered[idx], true
}

func (r *Registry) Definitions() []Definition {
	defs := make([]Definition, 0, len(r.ordered))
	for _, cmd := range r.ordered {
		defs = append(defs, cmd.Definition)
	}
	return defs
}

func CommandDefinitions() []Definition {
	defs := make([]Definition, len(commandDefinitions))
	copy(defs, commandDefinitions)
	return defs
}

var commandDefinitions = []Definition{
	{Name: "help", Usage: "/help", Summary: "Show this help message"},
	{Name: CmdClear, Usage: "/clear", Summary: "Clear the timeline history"},
	{Name: "compact", Usage: "/compact", Summary: "Manually compact the active session"},
	{Name: "mode", Usage: "/mode", Summary: "Open the agent mode selector"},
	{Name: string(types.ModePlan), Usage: "/plan", Summary: "Activate read-only plan mode"},
	{Name: string(types.ModeBuild), Usage: "/build", Summary: "Activate active build mode"},
	{Name: "agent", Usage: "/agent [name]", Summary: "Switch or show active agent mode"},
	{Name: "shell", Usage: "/shell", Summary: "Activate direct shell execution mode"},
	{Name: "chat", Usage: "/chat", Summary: "Return to normal chat mode"},
	{Name: CmdStatus, Usage: "/status", Summary: "Summarize mode, permissions, and approvals"},
	{Name: "context", Usage: "/context", Summary: "Show raw system prompt sent to the agent"},
	{Name: "provider", Usage: "/provider [list|add|use|remove]", Summary: "Manage configured LLM providers"},
	{Name: "models", Usage: "/models [list|use <model>|info <model>]", Summary: "List or select models from the active provider"},
	{Name: "themes", Usage: "/themes [theme]", Summary: "List or switch visual themes"},
	{Name: "sessions", Usage: "/sessions", Summary: "List or switch between workspace sessions"},
	{Name: "tools", Usage: "/tools", Summary: "Show all registered tools"},
	{Name: CmdTool, Usage: "/tool <name> [args]", Summary: "Execute a specific runtime tool"},
	{Name: "task", Usage: "/task [list|terminate <id>]", Summary: "Interact with background tasks"},
	{Name: "approve", Usage: "/approve", Summary: "Execute the pending tool command"},
	{Name: "deny", Usage: "/deny", Summary: "Cancel the pending tool command"},
	{Name: "brain", Usage: "/brain [list|read <file>|plan|tasks|summary|clear]", Summary: "Interact with the session brain"},
	{Name: "metrics", Usage: "/metrics", Summary: "Show cumulative token usage for this session"},
	{Name: "debug", Usage: "/debug", Summary: "Toggle agent debug output"},
	{Name: "trace", Usage: "/trace", Summary: "Toggle trace logging (requires -tags motoko_trace)"},
	{Name: "exit", Usage: "/exit", Summary: "Exit the application"},
	{Name: "quit", Usage: "/quit", Summary: "Exit the application"},
}
