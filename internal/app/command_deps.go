package app

import (
	"context"
	"fmt"
	"time"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/brain"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/mcp"
	"github.com/Hoosk/motoko/internal/session"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tools"

	"github.com/Hoosk/motoko/internal/app/commands"
	"github.com/Hoosk/motoko/internal/app/scheduleman"
	"github.com/Hoosk/motoko/internal/app/taskman"
	"github.com/Hoosk/motoko/internal/app/types"
)

// commandDeps exposes the runtime capabilities to the slash-command
// dispatcher as explicit closures. Keeping this wiring isolated from the
// constructor keeps NewRuntime small and makes the dependency graph visible.
func (r *Runtime) commandDeps() commands.Deps {
	return commands.Deps{
		ConfigFn:     func() *config.AppConfig { return r.config },
		SaveConfigFn: func() error { return r.config.Save() },
		ThemeFn:      func() string { return r.config.Theme },
		SetThemeFn: func(name string) error {
			r.config.Theme = name
			return r.config.Save()
		},

		InputModeFn:    func() types.InputMode { return r.inputMode },
		SetInputModeFn: func(m types.InputMode) { r.inputMode = m },

		ModeFn:            func() types.Mode { return r.agOrch.Mode() },
		SetAgentModeFn:    func(name string) { r.agOrch.SetAgentMode(name) },
		AgentNameFn:       func() string { return r.agOrch.AgentName() },
		AgentNamesFn:      func() []string { return r.agOrch.AgentNames() },
		AgentConfiguredFn: func() bool { return r.agOrch.AgentConfigured() },
		DebugFn:           func() bool { return r.agOrch.Debug() },
		SetDebugFn:        func(d bool) { r.agOrch.SetDebug(d) },
		AgentFn:           func() *agent.Agent { return r.agOrch.Agent() },
		SystemPromptFn:    func(info system.ContextInfo) string { return r.agOrch.SystemPrompt(info) },

		SessionFn: func() *session.Session { return r.sesMgr.CurrentSession() },
		SaveSessionFn: func() error {
			if s := r.sesMgr.CurrentSession(); s != nil {
				return s.Save()
			}
			return nil
		},
		BrainFn:        func() *brain.Brain { return r.sesMgr.Brain() },
		BrainInitErrFn: func() error { return r.sesMgr.BrainInitErr() },

		ListTasksFn:     func() []*taskman.TaskState { return r.taskMgr.List() },
		TerminateTaskFn: func(id string) error { return r.taskMgr.Terminate(id) },

		ListSchedulesFn: func() []scheduleman.Definition { return r.ListSchedules() },
		AddScheduleFn: func(instruction string, interval time.Duration, oneShot bool) (scheduleman.Definition, error) {
			return r.AddSchedule(instruction, interval, oneShot)
		},
		RemoveScheduleFn: func(id string) error { return r.RemoveSchedule(id) },

		ToolSpecsFn: func() []tools.Spec { return r.ToolSpecs() },
		RunToolFn: func(ctx context.Context, name, args string) (tools.Result, error) {
			return r.tools.Run(ctx, name, args)
		},
		MCPServersFn: func() []mcp.ServerStatus {
			if r.mcpMgr == nil {
				return nil
			}
			return r.mcpMgr.Servers()
		},
		AddMCPServerFn: func(srv config.MCPServerConfig) {
			if r.mcpMgr != nil {
				r.mcpMgr.Start(r.backgroundCtx, mcpServerConfigs([]config.MCPServerConfig{srv}))
			}
		},
		RemoveMCPServerFn: func(name string) bool {
			if r.mcpMgr == nil {
				return false
			}
			return r.mcpMgr.StopServer(name)
		},
		MCPResourcesFn: func(ctx context.Context) []mcp.Resource {
			if r.mcpMgr == nil {
				return nil
			}
			return r.mcpMgr.ListResources(ctx)
		},
		MCPResourceReadFn: func(ctx context.Context, serverName, uri string) (*mcp.ReadResourceResult, error) {
			if r.mcpMgr == nil {
				return nil, fmt.Errorf("no MCP manager available")
			}
			return r.mcpMgr.ReadResource(ctx, serverName, uri)
		},
		MCPPromptsFn: func(ctx context.Context) []mcp.Prompt {
			if r.mcpMgr == nil {
				return nil
			}
			return r.mcpMgr.ListPrompts(ctx)
		},
		MCPPromptHostsFn: func(_ context.Context) []mcp.PromptHost {
			if r.mcpMgr == nil {
				return nil
			}
			return r.mcpMgr.ListPromptHosts()
		},
		MCPGetPromptFn: func(ctx context.Context, serverName, name string, args map[string]string) (*mcp.GetPromptResult, error) {
			if r.mcpMgr == nil {
				return nil, fmt.Errorf("no MCP manager available")
			}
			return r.mcpMgr.GetPrompt(ctx, serverName, name, args)
		},

		ProvMgr: r.provMgr,

		PendingFn: func() string {
			if r.pending == nil {
				return ""
			}
			return r.pending.Command
		},
		SetPendingFn: func(cmd string) { r.pending = &pendingShell{Command: cmd} },
		ClearPendingFn: func() string {
			cmd := r.pending.Command
			r.pending = nil
			return cmd
		},

		ContextWindowFn: func() int { return r.contextWindow },
	}
}
