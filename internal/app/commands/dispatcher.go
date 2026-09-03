package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/brain"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/mcp"
	"github.com/Hoosk/motoko/internal/session"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tools"

	"github.com/Hoosk/motoko/internal/app/providerman"
	"github.com/Hoosk/motoko/internal/app/scheduleman"
	"github.com/Hoosk/motoko/internal/app/taskman"
	"github.com/Hoosk/motoko/internal/app/types"
)

const (
	CmdClear       = "clear"
	CmdStatus      = "status"
	CmdList        = "list"
	CmdTool        = "tool"
	CmdTools       = "tools"
	ValNone        = "none"
	ThemeCyberpunk = "cyberpunk"
	DefaultTheme   = ThemeCyberpunk

	CmdQuit   = "quit"
	CmdThemes = "themes"
	CmdAgent  = "agent"
	CmdShell  = "shell"
)

type Deps struct {
	ConfigFn     func() *config.AppConfig
	SaveConfigFn func() error
	ThemeFn      func() string
	SetThemeFn   func(name string) error

	InputModeFn    func() types.InputMode
	SetInputModeFn func(types.InputMode)

	ModeFn            func() types.Mode
	SetAgentModeFn    func(string)
	AgentNameFn       func() string
	AgentNamesFn      func() []string
	AgentConfiguredFn func() bool
	DebugFn           func() bool
	SetDebugFn        func(bool)
	AgentFn           func() *agent.Agent
	SystemPromptFn    func(system.ContextInfo) string

	SessionFn      func() *session.Session
	SaveSessionFn  func() error
	BrainFn        func() *brain.Brain
	BrainInitErrFn func() error

	ListTasksFn      func() []*taskman.TaskState
	TerminateTaskFn  func(id string) error
	ListSchedulesFn  func() []scheduleman.Definition
	AddScheduleFn    func(instruction string, interval time.Duration, oneShot bool) (scheduleman.Definition, error)
	RemoveScheduleFn func(id string) error

	ToolSpecsFn       func() []tools.Spec
	RunToolFn         func(ctx context.Context, name, args string) (tools.Result, error)
	MCPServersFn      func() []mcp.ServerStatus
	AddMCPServerFn    func(srv config.MCPServerConfig)
	RemoveMCPServerFn func(name string) bool
	MCPResourcesFn    func(ctx context.Context) []mcp.Resource
	MCPResourceReadFn func(ctx context.Context, serverName, uri string) (*mcp.ReadResourceResult, error)
	MCPPromptsFn      func(ctx context.Context) []mcp.Prompt
	MCPGetPromptFn    func(ctx context.Context, serverName, name string, args map[string]string) (*mcp.GetPromptResult, error)
	// MCPPromptHostsFn returns the set of (server, prompt) pairs available
	// for dynamic command lookup. Prompts become runnable as
	// /<prompt-name> [k=v ...] when the prompt name is unique across
	// servers. When the same name is hosted by multiple servers the
	// dispatcher surfaces the ambiguity via the standard error response.
	MCPPromptHostsFn func(ctx context.Context) []mcp.PromptHost

	ProvMgr *providerman.Manager

	PendingDialogsFn func() int

	ContextWindowFn func() int
}

type Dispatcher struct {
	deps     Deps
	registry *Registry
}

func New(deps Deps) *Dispatcher {
	d := &Dispatcher{deps: deps}
	d.registry = d.buildRegistry()
	return d
}

func (d *Dispatcher) Handle(input string, info system.ContextInfo) types.Response {
	parts := strings.Fields(strings.TrimPrefix(input, "/"))
	if len(parts) == 0 {
		return types.Response{}
	}

	command := strings.ToLower(parts[0])
	cmd, ok := d.registry.Lookup(command)
	if !ok {
		if resp, handled := d.handleDynamicPrompt(command, parts[1:]); handled {
			return resp
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Unknown command: /%s", command)}}}
	}
	return cmd.Handler(Invocation{RawInput: input, Args: parts[1:], Info: info})
}

func (d *Dispatcher) Definitions() []Definition {
	if d.registry == nil {
		return nil
	}
	return d.registry.Definitions()
}

func (d *Dispatcher) buildRegistry() *Registry {
	r := NewRegistry()
	for _, def := range commandDefinitions {
		r.Add(Command{
			Definition: def,
			Handler:    d.handlerFor(def.Name),
		})
	}
	return r
}

func (d *Dispatcher) handlerFor(command string) Handler {
	switch command {
	case "help":
		return func(inv Invocation) types.Response { return d.helpResponse() }
	case "exit", CmdQuit:
		return func(inv Invocation) types.Response { return types.Response{Signal: CmdQuit} }
	case CmdThemes:
		return d.handleThemesCommand
	case CmdClear:
		return func(inv Invocation) types.Response { return d.handleClearCommand() }
	case "compact":
		return func(inv Invocation) types.Response {
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Compacting session..."}}, Action: &types.Action{Type: types.ActionCompact}}
		}
	case string(types.ModePlan), string(types.ModeBuild):
		return d.handleModePresetCommand(command)
	case "learn":
		return func(inv Invocation) types.Response {
			d.deps.SetAgentModeFn("learn")
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Agent switched to: learn"}}, Action: &types.Action{Type: types.ActionAgent, AgentPrompt: learnPrompt()}}
		}
	case "teamwork-preview":
		return func(inv Invocation) types.Response {
			goal := strings.TrimSpace(strings.Join(inv.Args, " "))
			d.deps.SetAgentModeFn("teamwork")
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Agent switched to: teamwork"}}, Action: &types.Action{Type: types.ActionAgent, AgentPrompt: teamworkPreviewPrompt(goal)}}
		}
	case "grill-me":
		return func(inv Invocation) types.Response {
			d.deps.SetAgentModeFn("grill")
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Agent switched to: grill"}}, Action: &types.Action{Type: types.ActionAgent, AgentPrompt: grillMePrompt()}}
		}
	case CmdAgent:
		return d.handleAgentCommand
	case "mode":
		return func(inv Invocation) types.Response { return types.Response{Signal: "open-mode-popup"} }
	case CmdShell, "chat":
		return d.handleInputModeCommand(command)
	case CmdStatus:
		return func(inv Invocation) types.Response {
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: d.statusText(inv.Info)}}}
		}
	case "debug":
		return func(inv Invocation) types.Response { return d.handleDebugCommand() }
	case "context":
		return func(inv Invocation) types.Response { return d.handleContextCommand(inv.Info) }
	case "provider":
		return func(inv Invocation) types.Response { return d.deps.ProvMgr.HandleProviderCommand(inv.Args) }
	case "models", "model":
		return func(inv Invocation) types.Response { return d.deps.ProvMgr.HandleModelsCommand(inv.Args) }
	case "sessions":
		return func(inv Invocation) types.Response { return types.Response{Signal: "open-sessions-popup"} }
	case "settings":
		return func(inv Invocation) types.Response { return types.Response{Signal: "open-settings-popup"} }
	case CmdTools:
		return func(inv Invocation) types.Response {
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: formatToolList(d.deps.ToolSpecsFn())}}}
		}
	case CmdTool:
		return d.handleToolCommand
	case "mcp":
		return func(inv Invocation) types.Response { return d.handleMCPCommand(inv.Args) }
	case "trace":
		return func(inv Invocation) types.Response { return d.handleTraceCommand() }
	default:
		return func(inv Invocation) types.Response { return d.dispatchCommand(command, inv) }
	}
}

func (d *Dispatcher) dispatchCommand(command string, inv Invocation) types.Response {
	switch command {
	case "goal":
		return d.handleGoalCommand(inv.Args)
	case "schedule":
		return d.handleScheduleCommand(inv.Args)
	case "task":
		return d.handleTaskCommand(append([]string{command}, inv.Args...))
	case "brain":
		return d.handleBrainCommand(inv.Args)
	case "metrics":
		return d.handleMetricsCommand()
	}

	return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Unknown command: /%s", command)}}}
}

func (d *Dispatcher) helpResponse() types.Response {
	defs := d.Definitions()
	maxWidth := 0
	for _, def := range defs {
		if len(def.Usage) > maxWidth {
			maxWidth = len(def.Usage)
		}
	}

	lines := []string{"Available commands:"}
	for _, def := range defs {
		lines = append(lines, fmt.Sprintf("%-*s %s", maxWidth, def.Usage, def.Summary))
	}
	lines = append(lines,
		fmt.Sprintf("%-*s %s", maxWidth, "!<cmd>", "Execute an explicit shell command"),
		fmt.Sprintf("%-*s %s", maxWidth, "@<file|agent>", "Mention a file or agent in the prompt"),
	)

	return types.Response{Entries: []types.Entry{{Kind: types.EntryHelp, Text: strings.Join(lines, "\n")}}}
}
