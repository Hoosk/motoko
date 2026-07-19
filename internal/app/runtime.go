package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/brain"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/mcp"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/semantic"
	"github.com/Hoosk/motoko/internal/session"
	"github.com/Hoosk/motoko/internal/skills"
	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tachikoma"
	"github.com/Hoosk/motoko/internal/tools"
	"github.com/Hoosk/motoko/internal/updater"

	"github.com/Hoosk/motoko/internal/app/agentorch"
	"github.com/Hoosk/motoko/internal/app/commands"
	"github.com/Hoosk/motoko/internal/app/completions"
	"github.com/Hoosk/motoko/internal/app/providerman"
	"github.com/Hoosk/motoko/internal/app/scheduleman"
	"github.com/Hoosk/motoko/internal/app/sessionman"
	"github.com/Hoosk/motoko/internal/app/taskman"
	"github.com/Hoosk/motoko/internal/app/types"

	_ "github.com/Hoosk/motoko/internal/provider/anthropic"
	_ "github.com/Hoosk/motoko/internal/provider/gemini"
	_ "github.com/Hoosk/motoko/internal/provider/lmstudio"
	_ "github.com/Hoosk/motoko/internal/provider/openai"
)

type (
	Mode             = types.Mode
	InputMode        = types.InputMode
	EntryKind        = types.EntryKind
	Entry            = types.Entry
	ActionType       = types.ActionType
	Action           = types.Action
	TaskEvent        = types.TaskEvent
	TaskEventResult  = types.TaskEventResult
	Response         = types.Response
	ShellResult      = types.ShellResult
	ShellDecision    = types.ShellDecision
	AgentStreamEvent = types.AgentStreamEvent
	SubagentInfo     = types.SubagentInfo
	RuntimeOptions   = types.RuntimeOptions
)

const (
	ModePlan  = types.ModePlan
	ModeBuild = types.ModeBuild
)

const (
	InputModeChat  = types.InputModeChat
	InputModeShell = types.InputModeShell
)

const (
	EntryUser      = types.EntryUser
	EntryAssistant = types.EntryAssistant
	EntryReasoning = types.EntryReasoning
	EntrySystem    = types.EntrySystem
	EntryCommand   = types.EntryCommand
	EntryOutput    = types.EntryOutput
	EntryError     = types.EntryError
	EntryHelp      = types.EntryHelp
)

const (
	ActionShell   = types.ActionShell
	ActionTask    = types.ActionTask
	ActionAgent   = types.ActionAgent
	ActionCompact = types.ActionCompact
)

type TaskState = taskman.TaskState

var ThinkingBudgetLevels = providerman.ThinkingBudgetLevels
var ThinkingBudgetLabels = providerman.ThinkingBudgetLabels

type pendingShell struct {
	Command string
}

type Runtime struct {
	cplDeps           completions.Deps
	updateErr         error
	backgroundCtx     context.Context
	provMgr           *providerman.Manager
	cmdDispatch       *commands.Dispatcher
	newProviderClient func(config.ProviderConfig) (provider.Client, error)
	config            *config.AppConfig
	taskMgr           *taskman.Manager
	scheduleMgr       *scheduleman.Manager
	sesMgr            *sessionman.Manager
	questionBroker    *tools.QuestionBroker
	agOrch            *agentorch.Orchestrator
	semantic          *semantic.Index
	backgroundCancel  context.CancelFunc
	updateInfo        *updater.VersionInfo
	tachikomas        *tachikoma.Manager
	pending           *pendingShell
	tools             *tools.Registry
	mcpMgr            *mcp.Manager
	updateDone        chan struct{}
	inputMode         InputMode
	version           string
	contextWindow     int
	updateMu          sync.RWMutex
}

func NewRuntime(opts ...RuntimeOptions) *Runtime {
	toolsRegistry := tools.NewRegistry()
	workspacePath, _ := os.Getwd()
	cfg, _ := config.Load(workspacePath)
	if cfg == nil {
		cfg = &config.AppConfig{}
	}
	runtimeOpts := RuntimeOptions{}
	if len(opts) > 0 {
		runtimeOpts = opts[0]
	}
	workspaceID := session.WorkspaceIDFor(workspacePath)

	sesMgr := sessionman.NewManager(workspaceID)
	if runtimeOpts.Resume {
		if last, err := session.Last(workspaceID); err == nil && last != nil {
			sesMgr.SetCurrentSession(last)
			sesMgr.SetWasResumed(true)
		}
	}
	if sesMgr.CurrentSession() == nil {
		sesMgr.SetCurrentSession(session.New(workspaceID, workspacePath))
	}
	b, brainErr := brain.New(workspaceID, sesMgr.CurrentSession().ID)
	sesMgr.SetBrain(b, brainErr)

	allAgents := append([]agent.AgentDef(nil), agent.BuiltinAgents...)
	if customAgents, err := agent.LoadWorkspaceAgents(workspacePath); err == nil && len(customAgents) > 0 {
		allAgents = append(allAgents, customAgents...)
	}

	sList, _ := skills.Discover(workspacePath)

	r := &Runtime{
		inputMode:         InputModeChat,
		tools:             toolsRegistry,
		newProviderClient: provider.NewClient,
		config:            cfg,
		sesMgr:            sesMgr,
		semantic:          semantic.NewIndex(),
		tachikomas:        tachikoma.NewManager(),
		taskMgr:           taskman.NewManager(),
		scheduleMgr:       scheduleman.NewManager(),
		questionBroker:    tools.NewQuestionBroker(),
		updateDone:        make(chan struct{}),
		version:           runtimeOpts.Version,
	}
	r.agOrch = agentorch.New(agentorch.Deps{
		ConfigFn:          func() *config.AppConfig { return r.config },
		ProviderClientFn:  func() func(config.ProviderConfig) (provider.Client, error) { return r.newProviderClient },
		ToolsFn:           func() *tools.Registry { return r.tools },
		SemanticFn:        func() *semantic.Index { return r.semantic },
		BrainFn:           func() *brain.Brain { return r.sesMgr.Brain() },
		CurrentSessionFn:  func() *session.Session { return r.sesMgr.CurrentSession() },
		WorkspaceIDFn:     func() string { return r.sesMgr.WorkspaceID() },
		ContextWindowFn:   func() int { return r.contextWindow },
		AvailableAgentsFn: func() []agent.AgentDef { return allAgents },
		AvailableSkillsFn: func() []skills.Skill { return sList },
		ContextInfoFn:     func() system.ContextInfo { return r.GetContextInfo() },
		OnPersistTurn:     func(result agent.Result) { r.sesMgr.PersistTurn(result) },
		OnGenerateTitle: func(ctx context.Context, userInput, assistantResponse string) {
			r.sesMgr.GenerateTitle(ctx, userInput, assistantResponse, r.config, r.newProviderClient)
		},
		OnMaybeAutoCompact: func(ctx context.Context, onEvent func(types.AgentStreamEvent) error) error {
			return r.sesMgr.MaybeAutoCompact(ctx, onEvent, r.config, r.newProviderClient, r.contextWindow)
		},
	})
	r.provMgr = providerman.NewManager(func() *config.AppConfig { return r.config }, func() func(config.ProviderConfig) (provider.Client, error) { return r.newProviderClient }, r.agOrch.RefreshAgent)
	r.cmdDispatch = commands.New(commands.Deps{
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
			if r.mcpMgr != nil {
				return r.mcpMgr.StopServer(name)
			}
			return false
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
	})
	r.cplDeps = completions.Deps{
		AgentNamesFn:          func() []string { return r.agOrch.AgentNames() },
		SemanticFn:            func() *semantic.Index { return r.semantic },
		InputModeFn:           func() types.InputMode { return r.inputMode },
		ToolSuggestionsFn:     func(prefix string) []tools.Spec { return r.ToolSuggestions(prefix) },
		ActiveConfigFn:        func() (config.ProviderConfig, bool) { return r.config.Active() },
		ConfiguredProvidersFn: func() []config.ProviderConfig { return r.ConfiguredProviders() },
	}

	if r.config.Theme != "" {
		styles.SetTheme(r.config.Theme)
	}

	r.scheduleMgr.SetOnChange(func(defs []scheduleman.Definition) {
		r.persistSchedules(defs)
	})
	r.restoreSchedules()

	r.tachikomas.Add(tachikoma.NewGitTachikoma(10 * time.Second))
	r.tachikomas.Add(tachikoma.NewCodeTachikoma(r.semantic, 30*time.Second))
	r.tachikomas.Add(tachikoma.NewDiffTachikoma(r.semantic, 15*time.Second))
	r.tachikomas.Add(tachikoma.NewSearchTachikoma(r.semantic))
	r.tachikomas.Add(tachikoma.NewDependencyTachikoma())

	r.tools.Register(tools.NewInspectTool(r.tachikomas))
	r.tools.Register(tools.NewDelegateTool(r))
	r.tools.Register(tools.NewTaskTool(r))
	r.tools.Register(tools.NewQuestionTool(r.questionBroker))
	r.tools.Register(tools.NewBrainWriteTool(r))
	r.tools.Register(tools.NewBrainReadTool(r))
	r.tools.Register(tools.NewBrainListTool(r))

	if len(sList) > 0 {
		r.tools.Register(tools.NewActivateSkillTool(sList))
	}

	// MCP manager is built last so it can publish tools directly into the
	// already-populated registry. Servers are started on Runtime.Start so the
	// background context is available for transports and to honour the
	// process-wide cancellation during shutdown.
	r.mcpMgr = mcp.NewManager(mcp.ManagerConfig{
		Registry: mcp.ToolRegistrar{
			Register: func(adapter mcp.ToolAdapter) {
				if adapter == nil {
					return
				}
				r.tools.Register(tools.NewMCPRemoteTool(adapter))
			},
			Unregister: func(name string) bool {
				return r.tools.Unregister(name)
			},
		},
		RootsFn: func(ctx context.Context) ([]mcp.Root, error) {
			var path string
			if r.sesMgr != nil && r.sesMgr.CurrentSession() != nil {
				path = r.sesMgr.CurrentSession().Workspace
			}
			if path == "" {
				var err error
				path, err = os.Getwd()
				if err != nil {
					return nil, err
				}
			}
			uri := "file://" + filepath.ToSlash(path)
			return []mcp.Root{
				{
					URI:  uri,
					Name: "workspace",
				},
			}, nil
		},
		SamplingFn: func(ctx context.Context, params mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			items := make([]provider.ConversationItem, len(params.Messages))
			for i, m := range params.Messages {
				role := provider.RoleUser
				if m.Role == "assistant" {
					role = provider.RoleAssistant
				}
				items[i] = provider.ConversationItem{
					Role:    role,
					Content: m.Content.Text,
				}
			}

			cfg, ok := r.provMgr.GetActiveProviderConfig()
			if !ok {
				return nil, fmt.Errorf("no active provider configured")
			}
			pClient, err := r.provMgr.ProviderClient(cfg)
			if err != nil {
				return nil, err
			}

			resp, err := pClient.Complete(ctx, params.SystemPrompt, items, provider.ToolSet{})
			if err != nil {
				return nil, err
			}

			var modelName string
			if base, ok := pClient.(interface{ Model() string }); ok {
				modelName = base.Model()
			} else {
				modelName = pClient.Summary()
			}

			return &mcp.CreateMessageResult{
				Content: mcp.ContentBlock{
					Type: "text",
					Text: resp.FinalText,
				},
				Model: modelName,
				Role:  "assistant",
			}, nil
		},
	})

	return r
}

func (r *Runtime) Mode() Mode                        { return r.agOrch.Mode() }
func (r *Runtime) AgentName() string                 { return r.agOrch.AgentName() }
func (r *Runtime) AgentNames() []string              { return r.agOrch.AgentNames() }
func (r *Runtime) AvailableAgents() []agent.AgentDef { return r.agOrch.AvailableAgents() }
func (r *Runtime) AvailableSkills() []skills.Skill   { return r.agOrch.AvailableSkills() }
func (r *Runtime) SetAgentMode(name string)          { r.agOrch.SetAgentMode(name) }
func (r *Runtime) SetTestAgents(a []agent.AgentDef)  { r.agOrch.SetTestAgents(a) }
func (r *Runtime) SetTestSkills(s []skills.Skill)    { r.agOrch.SetTestSkills(s) }
func (r *Runtime) InputMode() InputMode              { return r.inputMode }
func (r *Runtime) AgentConfigured() bool             { return r.agOrch.AgentConfigured() }
func (r *Runtime) Debug() bool                       { return r.agOrch.Debug() }
func (r *Runtime) SemanticIndex() *semantic.Index    { return r.semantic }
func (r *Runtime) ContextWindow() int                { return r.contextWindow }

func (r *Runtime) handleSlashCommand(input string, info system.ContextInfo) Response {
	return r.cmdDispatch.Handle(input, info)
}

func (r *Runtime) Completions(input string) []string {
	return completions.Completions(r.cplDeps, input)
}

func (r *Runtime) MentionSuggestions(input string) []string {
	return completions.MentionSuggestions(r.cplDeps, input)
}

func (r *Runtime) PendingApproval() string {
	if r.pending == nil {
		return ""
	}
	return r.pending.Command
}

func (r *Runtime) ToolSpecs() []tools.Spec {
	maxOutputSize := system.MaxToolOutputBytes(r.contextWindow)
	tCtx := tools.ToolContext{
		Workspace:     r.sesMgr.WorkspaceID(),
		ActiveMode:    string(r.agOrch.Mode()),
		MaxOutputSize: maxOutputSize,
	}
	for _, a := range r.agOrch.AvailableAgents() {
		tCtx.AvailableAgents = append(tCtx.AvailableAgents, a.Name)
	}
	for _, s := range r.agOrch.AvailableSkills() {
		tCtx.AvailableSkills = append(tCtx.AvailableSkills, s.Name)
	}
	return r.tools.Specs(tCtx)
}

func (r *Runtime) ToolSuggestions(prefix string) []tools.Spec {
	maxOutputSize := system.MaxToolOutputBytes(r.contextWindow)
	tCtx := tools.ToolContext{
		Workspace:     r.sesMgr.WorkspaceID(),
		ActiveMode:    string(r.agOrch.Mode()),
		MaxOutputSize: maxOutputSize,
	}
	for _, a := range r.agOrch.AvailableAgents() {
		tCtx.AvailableAgents = append(tCtx.AvailableAgents, a.Name)
	}
	for _, s := range r.agOrch.AvailableSkills() {
		tCtx.AvailableSkills = append(tCtx.AvailableSkills, s.Name)
	}
	return r.tools.Suggestions(tCtx, prefix)
}

func (r *Runtime) StartTask(ctx context.Context, command string) (string, error) {
	if r.taskMgr == nil {
		return "", fmt.Errorf("task manager not initialized")
	}
	if r.backgroundCtx != nil {
		ctx = r.backgroundCtx
	}
	return r.taskMgr.Launch(ctx, command)
}

func (r *Runtime) TerminateTask(id string) error {
	if r.taskMgr == nil {
		return fmt.Errorf("task manager not initialized")
	}
	return r.taskMgr.Terminate(id)
}

func (r *Runtime) ListTasks() []*TaskState {
	if r.taskMgr == nil {
		return nil
	}
	return r.taskMgr.List()
}

func (r *Runtime) NextTaskEvent(ctx context.Context) TaskEventResult {
	if r.taskMgr == nil {
		return TaskEventResult{}
	}
	return r.taskMgr.Next(ctx)
}

func (r *Runtime) ActiveTasks() int {
	if r.taskMgr == nil {
		return 0
	}
	return r.taskMgr.ActiveTasks()
}

func (r *Runtime) NextScheduleEvent(ctx context.Context) scheduleman.EventResult {
	if r.scheduleMgr == nil {
		return scheduleman.EventResult{}
	}
	return r.scheduleMgr.Next(ctx)
}

func (r *Runtime) AddSchedule(instruction string, interval time.Duration, oneShot bool) (scheduleman.Definition, error) {
	if r.scheduleMgr == nil {
		return scheduleman.Definition{}, fmt.Errorf("schedule manager not initialized")
	}
	return r.scheduleMgr.Add(instruction, interval, oneShot)
}

func (r *Runtime) RemoveSchedule(id string) error {
	if r.scheduleMgr == nil {
		return fmt.Errorf("schedule manager not initialized")
	}
	return r.scheduleMgr.Remove(id)
}

func (r *Runtime) ListSchedules() []scheduleman.Definition {
	if r.scheduleMgr == nil {
		return nil
	}
	return r.scheduleMgr.List()
}

func (r *Runtime) Config() *config.AppConfig { return r.config }

func (r *Runtime) SaveConfig() error { return r.config.Save() }

func (r *Runtime) ProviderSummary() string                  { return r.provMgr.ProviderSummary() }
func (r *Runtime) ProviderPresets() []config.ProviderPreset { return r.provMgr.ProviderPresets() }
func (r *Runtime) ConfiguredProviders() []config.ProviderConfig {
	providers := append([]config.ProviderConfig(nil), r.config.Providers...)
	return providers
}
func (r *Runtime) LookupCatalogProvider(id string) (provider.CatalogProvider, bool) {
	return r.provMgr.LookupCatalogProvider(id)
}
func (r *Runtime) GetActiveProviderConfig() (config.ProviderConfig, bool) {
	return r.provMgr.GetActiveProviderConfig()
}
func (r *Runtime) SetActiveModelInfo(model provider.ModelInfo) error {
	return r.provMgr.SetActiveModelInfo(model)
}
func (r *Runtime) GetModelInfoForActiveProvider(ctx context.Context, modelID string) (provider.ModelInfo, error) {
	return r.provMgr.GetModelInfoForActiveProvider(ctx, modelID)
}
func (r *Runtime) SetThinkingBudget(budget int) error { return r.provMgr.SetThinkingBudget(budget) }
func (r *Runtime) ListModelsForProvider(ctx context.Context, providerCfg config.ProviderConfig) ([]provider.ModelInfo, error) {
	return r.provMgr.ListModelsForProvider(ctx, providerCfg)
}
func (r *Runtime) SaveProvider(providerCfg config.ProviderConfig, activate bool) error {
	return r.provMgr.SaveProvider(providerCfg, activate)
}

func (r *Runtime) HistoryInputTokens() int                   { return r.sesMgr.HistoryInputTokens() }
func (r *Runtime) SessionTitle() string                      { return r.sesMgr.SessionTitle() }
func (r *Runtime) StartupEntries() []Entry                   { return r.sesMgr.StartupEntries() }
func (r *Runtime) GetBrain() *brain.Brain                    { return r.sesMgr.Brain() }
func (r *Runtime) ListSessions() ([]*session.Session, error) { return r.sesMgr.ListSessions() }
func (r *Runtime) LoadSession(id string) error {
	if err := r.sesMgr.LoadSession(id); err != nil {
		return err
	}
	r.restoreSchedules()
	return nil
}
func (r *Runtime) CurrentSessionEntries() []Entry { return r.sesMgr.CurrentSessionEntries() }
func (r *Runtime) CompactSession(ctx context.Context) Response {
	return r.sesMgr.CompactSession(ctx, r.config, r.newProviderClient, r.contextWindow)
}

func (r *Runtime) ActiveSubagents() []string                   { return r.agOrch.ActiveSubagents() }
func (r *Runtime) SystemPrompt(info system.ContextInfo) string { return r.agOrch.SystemPrompt(info) }
func (r *Runtime) RunAgent(ctx context.Context, info system.ContextInfo, input string) (agent.Result, error) {
	ctx = tools.WithQuestionBroker(ctx, r.questionBroker)
	return r.agOrch.RunAgent(ctx, info, input)
}
func (r *Runtime) RunAgentStream(ctx context.Context, info system.ContextInfo, input string, onEvent func(AgentStreamEvent) error) (agent.Result, error) {
	ctx = tools.WithQuestionBroker(ctx, r.questionBroker)
	return r.agOrch.RunAgentStream(ctx, info, input, func(ev types.AgentStreamEvent) error {
		return onEvent(AgentStreamEvent(ev))
	})
}
func (r *Runtime) RunSubagent(ctx context.Context, cfg tools.SubagentConfig) (string, error) {
	ctx = tools.WithQuestionBroker(ctx, r.questionBroker)
	return r.agOrch.RunSubagent(ctx, cfg)
}

func (r *Runtime) Start(ctx context.Context) {
	r.backgroundCtx, r.backgroundCancel = context.WithCancel(ctx)
	if r.scheduleMgr != nil {
		r.scheduleMgr.AttachContext(r.backgroundCtx)
	}
	if r.tachikomas != nil {
		r.tachikomas.Start(r.backgroundCtx)
	}
	if r.mcpMgr != nil && r.config != nil && len(r.config.MCPServers) > 0 {
		r.mcpMgr.Start(r.backgroundCtx, mcpServerConfigs(r.config.MCPServers))
	}
	_ = provider.LoadCatalog(context.Background())
	r.agOrch.RefreshAgent()
	go func() {
		defer close(r.updateDone)
		if r.version == "dev" || r.version == "" {
			return
		}
		upd := updater.NewUpdater(updater.Config{
			CurrentVersion: r.version,
			GOOS:           runtime.GOOS,
			GOARCH:         runtime.GOARCH,
		})
		info, err := upd.CheckVersion(r.backgroundCtx)
		r.updateMu.Lock()
		defer r.updateMu.Unlock()
		if err != nil {
			r.updateErr = err
			return
		}
		r.updateInfo = info
	}()
}

func (r *Runtime) restoreSchedules() {
	if r.scheduleMgr == nil || r.sesMgr == nil || r.sesMgr.Brain() == nil {
		return
	}
	if !r.sesMgr.Brain().Exists("schedule") {
		r.scheduleMgr.Restore(nil)
		return
	}
	content, err := r.sesMgr.Brain().Read("schedule")
	if err != nil {
		return
	}
	r.scheduleMgr.Restore(scheduleman.ParseScheduleBrain(content))
}

func (r *Runtime) persistSchedules(defs []scheduleman.Definition) {
	if r.sesMgr == nil || r.sesMgr.Brain() == nil {
		return
	}
	if len(defs) == 0 {
		_ = r.sesMgr.Brain().Delete("schedule")
		return
	}
	_ = r.sesMgr.Brain().Write("schedule", scheduleman.FormatScheduleBrain(defs))
}

func (r *Runtime) WaitForUpdate() (*updater.VersionInfo, error) {
	if r.backgroundCtx != nil {
		select {
		case <-r.updateDone:
			r.updateMu.RLock()
			defer r.updateMu.RUnlock()
			return r.updateInfo, r.updateErr
		case <-r.backgroundCtx.Done():
			return nil, r.backgroundCtx.Err()
		}
	}
	<-r.updateDone
	r.updateMu.RLock()
	defer r.updateMu.RUnlock()
	return r.updateInfo, r.updateErr
}

func (r *Runtime) Tachikomas() *tachikoma.Manager { return r.tachikomas }

func (r *Runtime) QuestionBroker() *tools.QuestionBroker { return r.questionBroker }

func (r *Runtime) BackgroundContext() context.Context {
	if r.backgroundCtx != nil {
		return r.backgroundCtx
	}
	return context.Background()
}

func (r *Runtime) Stop() {
	if r.backgroundCancel != nil {
		r.backgroundCancel()
	}
	if r.mcpMgr != nil {
		r.mcpMgr.Stop()
	}
}

func (r *Runtime) GetContextInfo() system.ContextInfo {
	if r.tachikomas != nil {
		return r.tachikomas.GetContextInfo()
	}
	return system.GetContextInfo()
}

func (r *Runtime) Version() string { return r.version }

func (r *Runtime) ReplaceTrailingMention(input, mention string) string {
	token, ok := trailingMentionToken(input)
	if !ok {
		return input
	}
	idx := strings.LastIndex(input, token)
	if idx == -1 {
		return input
	}
	replacement := mention
	if strings.HasPrefix(mention, "@") && r.isAgentMention(mention) {
		replacement += " "
	}
	if strings.HasPrefix(mention, "@") && !r.isAgentMention(mention) {
		replacement += " "
	}
	return input[:idx] + replacement
}

func (r *Runtime) isAgentMention(mention string) bool {
	name := strings.TrimPrefix(strings.TrimSpace(mention), "@")
	for _, agentName := range r.AgentNames() {
		if strings.EqualFold(agentName, name) {
			return true
		}
	}
	return false
}

func (r *Runtime) extractMentionedFiles(input string) []string {
	fields := strings.Fields(input)
	var files []string
	seen := make(map[string]struct{})
	for _, field := range fields {
		if !strings.HasPrefix(field, "@") {
			continue
		}
		mention := strings.TrimPrefix(field, "@")
		if mention == "" || r.isAgentMention(field) {
			continue
		}
		if _, ok := seen[mention]; ok {
			continue
		}
		seen[mention] = struct{}{}
		files = append(files, mention)
	}
	return files
}

func trailingMentionToken(input string) (string, bool) {
	if input == "" {
		return "", false
	}
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return "", false
	}
	last := fields[len(fields)-1]
	if !strings.HasPrefix(last, "@") {
		return "", false
	}
	return last, true
}

// mcpServerConfigs converts the persisted config entries into the shape the
// mcp manager consumes. The conversion is intentionally explicit so future
// fields (auth, headers, env templates) can be added without leaking the
// persistence type into the mcp package.
func mcpServerConfigs(in []config.MCPServerConfig) []mcp.ServerConfig {
	if len(in) == 0 {
		return nil
	}
	out := make([]mcp.ServerConfig, 0, len(in))
	for _, s := range in {
		out = append(out, mcp.ServerConfig{
			Name:      s.Name,
			Transport: s.NormalizeTransport(),
			Command:   s.Command,
			Args:      s.Args,
			Env:       s.EnvSlice(),
			URL:       s.URL,
			Headers:   s.Headers,
			Disabled:  s.Disabled,
		})
	}
	return out
}
