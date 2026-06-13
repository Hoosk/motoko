package app

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/brain"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/semantic"
	"github.com/Hoosk/motoko/internal/session"
	"github.com/Hoosk/motoko/internal/skills"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tachikoma"
	"github.com/Hoosk/motoko/internal/tools"
	"github.com/Hoosk/motoko/internal/updater"
)

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

type EntryKind string

const (
	EntryUser      EntryKind = "user"
	EntryAssistant EntryKind = "assistant"
	EntryReasoning EntryKind = "reasoning"
	EntrySystem    EntryKind = "system"
	EntryCommand   EntryKind = "command"
	EntryOutput    EntryKind = "output"
	EntryError     EntryKind = "error"
	EntryHelp      EntryKind = "help"
)

type Entry struct {
	Kind EntryKind
	Text string
}

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

type Response struct {
	Action  *Action
	Signal  string
	Entries []Entry
	Clear   bool
}

type pendingShell struct {
	Command string
}

type SubagentInfo struct {
	StartedAt time.Time
	Name      string
	Prompt    string
}

type Runtime struct {
	brainInitErr      error
	backgroundCtx     context.Context
	updateErr         error
	semantic          *semantic.Index
	agent             *agent.Agent
	newProviderClient func(config.ProviderConfig) (provider.Client, error)
	config            *config.AppConfig
	tasks             *TaskManager
	updateInfo        *updater.VersionInfo
	tachikomas        *tachikoma.Manager
	activeSubagents   map[string]*SubagentInfo
	pending           *pendingShell
	tools             *tools.Registry
	updateDone        chan struct{}
	brain             *brain.Brain
	currentSession    *session.Session
	workspaceID       string
	currentAgentName  string
	inputMode         InputMode
	mode              Mode
	version           string
	availableAgents   []agent.AgentDef
	mentionedFiles    []string
	availableSkills   []skills.Skill
	contextWindow     int
	subagentsMu       sync.Mutex
	wasResumed        bool
	debug             bool
}

type RuntimeOptions struct {
	Version string
	Resume  bool
}

type AgentStreamEvent struct {
	Kind             string
	Title            string
	Content          string
	ReasoningContent string
}

func NewRuntime(opts ...RuntimeOptions) *Runtime {
	toolsRegistry := tools.NewRegistry()
	cfg, _ := config.Load()
	if cfg == nil {
		cfg = &config.AppConfig{}
	}
	runtimeOpts := RuntimeOptions{}
	if len(opts) > 0 {
		runtimeOpts = opts[0]
	}
	workspacePath, _ := os.Getwd()
	workspaceID := session.WorkspaceIDFor(workspacePath)

	allAgents := append([]agent.AgentDef(nil), agent.BuiltinAgents...)
	if customAgents, err := agent.LoadWorkspaceAgents(workspacePath); err == nil && len(customAgents) > 0 {
		allAgents = append(allAgents, customAgents...)
	}

	sList, _ := skills.Discover(workspacePath)

	r := &Runtime{
		mode:              ModePlan,
		inputMode:         InputModeChat,
		tools:             toolsRegistry,
		newProviderClient: provider.NewClient,
		config:            cfg,
		semantic:          semantic.NewIndex(),
		tachikomas:        tachikoma.NewManager(),
		currentAgentName:  string(ModePlan),
		availableAgents:   allAgents,
		workspaceID:       workspaceID,
		availableSkills:   sList,
		activeSubagents:   make(map[string]*SubagentInfo),
		tasks:             NewTaskManager(),
		updateDone:        make(chan struct{}),
		version:           runtimeOpts.Version,
	}

	// Setup default tachikomas
	r.tachikomas.Add(tachikoma.NewGitTachikoma(10 * time.Second))
	r.tachikomas.Add(tachikoma.NewCodeTachikoma(r.semantic, 30*time.Second))
	r.tachikomas.Add(tachikoma.NewDiffTachikoma(r.semantic, 15*time.Second))
	r.tachikomas.Add(tachikoma.NewSearchTachikoma(r.semantic))
	r.tachikomas.Add(tachikoma.NewDependencyTachikoma())

	// Register tools that depend on tachikomas
	r.tools.Register(tools.NewInspectTool(r.tachikomas))
	r.tools.Register(tools.NewDelegateTool(r))
	r.tools.Register(tools.NewTaskTool(r))
	r.tools.Register(tools.NewBrainWriteTool(r))
	r.tools.Register(tools.NewBrainReadTool(r))
	r.tools.Register(tools.NewBrainListTool(r))

	if len(sList) > 0 {
		r.tools.Register(tools.NewActivateSkillTool(sList))
	}

	if runtimeOpts.Resume {
		if last, err := session.Last(workspaceID); err == nil && last != nil {
			r.currentSession = last
			r.wasResumed = true
		}
	}
	if r.currentSession == nil {
		r.currentSession = session.New(workspaceID, workspacePath)
	}
	r.brain, r.brainInitErr = brain.New(workspaceID, r.currentSession.ID)
	r.refreshAgent()
	return r
}

func (r *Runtime) Mode() Mode {
	return r.mode
}

// AgentName returns the name of the currently active agent mode.
func (r *Runtime) AgentName() string {
	if r.currentAgentName == "" {
		return string(r.mode)
	}
	return r.currentAgentName
}

func (r *Runtime) AgentNames() []string {
	result := make([]string, 0, len(r.availableAgents))
	for _, a := range r.availableAgents {
		result = append(result, a.Name)
	}
	return result
}

// AvailableAgents returns all agents (builtin + custom from .agents).
func (r *Runtime) AvailableAgents() []agent.AgentDef {
	return r.availableAgents
}

// SetAgentMode switches to the named agent, updating the mode and system prompt.
func (r *Runtime) SetAgentMode(name string) {
	for _, a := range r.availableAgents {
		if strings.EqualFold(a.Name, name) {
			r.currentAgentName = a.Name
			if strings.EqualFold(name, "build") {
				r.mode = ModeBuild
			} else {
				r.mode = ModePlan
			}
			if r.agent != nil {
				r.agent.SetAgentOverride(a.System)
			}
			return
		}
	}
}

func (r *Runtime) InputMode() InputMode {
	return r.inputMode
}

func (r *Runtime) PendingApproval() string {
	if r.pending == nil {
		return ""
	}

	return r.pending.Command
}

func (r *Runtime) ToolSpecs() []tools.Spec {
	return r.tools.Specs()
}

func (r *Runtime) StartTask(ctx context.Context, command string) (string, error) {
	if r.tasks == nil {
		return "", fmt.Errorf("task manager no inicializado")
	}
	if r.backgroundCtx != nil {
		ctx = r.backgroundCtx
	}
	return r.tasks.Launch(ctx, command)
}

func (r *Runtime) TerminateTask(id string) error {
	if r.tasks == nil {
		return fmt.Errorf("task manager no inicializado")
	}
	return r.tasks.Terminate(id)
}

func (r *Runtime) ListTasks() []*TaskState {
	if r.tasks == nil {
		return nil
	}
	return r.tasks.List()
}

func (r *Runtime) NextTaskEvent(ctx context.Context) TaskEventResult {
	if r.tasks == nil {
		return TaskEventResult{}
	}
	return r.tasks.Next(ctx)
}

func (r *Runtime) ActiveTasks() int {
	if r.tasks == nil {
		return 0
	}
	return r.tasks.ActiveTasks()
}

func (r *Runtime) AgentConfigured() bool {
	return r.agent != nil && r.agent.Configured()
}

func (r *Runtime) Debug() bool {
	return r.debug
}

func (r *Runtime) SemanticIndex() *semantic.Index {
	return r.semantic
}

func (r *Runtime) ProviderSummary() string {
	if r.config == nil {
		return "none"
	}
	active, ok := r.config.Active()
	if !ok {
		return "none"
	}
	if strings.TrimSpace(active.Model) == "" {
		return fmt.Sprintf("%s (%s:no-model)", active.Name, active.Preset)
	}
	return fmt.Sprintf("%s (%s:%s)", active.Name, active.Preset, active.Model)
}

func (r *Runtime) ProviderPresets() []config.ProviderPreset {
	return config.ValidProviderPresets()
}

// GetActiveProviderConfig returns the currently active ProviderConfig.
func (r *Runtime) GetActiveProviderConfig() (config.ProviderConfig, bool) {
	if r.config == nil {
		return config.ProviderConfig{}, false
	}
	return r.config.Active()
}

// SetActiveModelInfo updates the model field for the active provider and saves.
func (r *Runtime) SetActiveModelInfo(model provider.ModelInfo) error {
	if r.config == nil {
		return fmt.Errorf("no hay configuracion")
	}
	active, ok := r.config.Active()
	if !ok {
		return fmt.Errorf("no hay provider activo")
	}
	active.Model = model.ID
	active.Models = config.UniqueSortedKeep(active.Models, model.ID)
	active.ContextWindow = model.ContextWindow
	active.SupportsThinking = model.SupportsThinking
	r.config.UpsertProvider(active)
	if err := r.config.Save(); err != nil {
		return err
	}
	r.refreshAgent()
	return nil
}

// GetModelInfoForActiveProvider queries the active provider client for detailed model info (like SupportsThinking).
func (r *Runtime) GetModelInfoForActiveProvider(ctx context.Context, modelID string) (provider.ModelInfo, error) {
	active, ok := r.config.Active()
	if !ok {
		return provider.ModelInfo{}, fmt.Errorf("no hay provider activo")
	}
	client, err := r.providerClient(active)
	if err != nil {
		return provider.ModelInfo{}, err
	}
	return client.GetModel(ctx, modelID)
}

// SetThinkingBudget updates the thinking budget for the active provider.
// level: 0=off, 1=low(1024), 2=medium(4096), 3=high(16000).
func (r *Runtime) SetThinkingBudget(budget int) error {
	if r.config == nil {
		return fmt.Errorf("no hay configuracion")
	}
	active, ok := r.config.Active()
	if !ok {
		return fmt.Errorf("no hay provider activo")
	}
	active.ThinkingBudget = budget
	r.config.UpsertProvider(active)
	if err := r.config.Save(); err != nil {
		return err
	}
	r.refreshAgent()
	return nil
}

// ThinkingBudgetLevels returns the ordered token-budget values for thinking modes.
// Index: 0=off, 1=low, 2=medium, 3=high, 4=xhigh.
// Values match the official Gemini/OpenAI reasoning mapping.
// xhigh (65536) maps to reasoning_effort="xhigh" on OpenAI o-series/gpt-5 models.
var ThinkingBudgetLevels = []int{0, 1024, 8192, 24576, 65536}
var ThinkingBudgetLabels = []string{"off", "low (1k)", "medium (8k)", "high (24k)", "xhigh (64k)"}

func (r *Runtime) ListModelsForProvider(ctx context.Context, providerCfg config.ProviderConfig) ([]provider.ModelInfo, error) {
	client, err := r.providerClient(providerCfg)
	if err != nil {
		return nil, err
	}
	models, err := client.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	r.cacheProviderModels(providerCfg.Name, ids)
	return models, nil
}

func (r *Runtime) ContextWindow() int {
	return r.contextWindow
}

func (r *Runtime) HistoryInputTokens() int {
	if r.currentSession == nil {
		return 0
	}
	return r.currentSession.LastInputTokens
}

func (r *Runtime) SessionTitle() string {
	if r.currentSession == nil {
		return ""
	}
	return strings.TrimSpace(r.currentSession.Title)
}

func (r *Runtime) StartupEntries() []Entry {
	if !r.wasResumed || r.currentSession == nil {
		return nil
	}
	entries := []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Sesion resumida: %s", r.currentSession.Title)}}
	if r.brain != nil {
		var hints []string
		if r.brain.Exists("plan") {
			if plan, err := r.brain.Read("plan"); err == nil {
				hints = append(hints, fmt.Sprintf("plan.md (%.1fKB)", float64(len(plan))/1024.0))
			}
		}
		if r.brain.Exists("tasks") {
			if tasks, err := r.brain.Read("tasks"); err == nil {
				hints = append(hints, fmt.Sprintf("tasks.md (%.1fKB)", float64(len(tasks))/1024.0))
			}
		}
		if len(hints) > 0 {
			entries = append(entries, Entry{
				Kind: EntrySystem,
				Text: fmt.Sprintf("Session brain found: %s. The agent will continue from the existing plan.", strings.Join(hints, ", ")),
			})
		}
	}
	entries = append(entries, r.CurrentSessionEntries()...)
	return entries
}

func (r *Runtime) SaveProvider(providerCfg config.ProviderConfig, activate bool) error {
	if r.config == nil {
		r.config = &config.AppConfig{}
	}
	providerCfg = config.NormalizeProvider(providerCfg)
	r.config.UpsertProvider(providerCfg)
	if activate || strings.TrimSpace(r.config.ActiveProvider) == "" {
		if err := r.config.SetActive(providerCfg.Name); err != nil {
			return err
		}
	}
	if err := r.config.Save(); err != nil {
		return err
	}
	r.refreshAgent()
	return nil
}

func (r *Runtime) refreshAgent() {
	if r.config == nil {
		r.agent = nil
		r.contextWindow = 0
		return
	}
	active, ok := r.config.Active()
	if !ok {
		r.agent = nil
		r.contextWindow = 0
		return
	}
	
	var aDef agent.AgentDef
	found := false
	for _, a := range r.availableAgents {
		if strings.EqualFold(a.Name, r.currentAgentName) {
			aDef = a
			found = true
			break
		}
	}
	if !found {
		// Should not happen, fallback to build
		for _, a := range r.availableAgents {
			if strings.EqualFold(a.Name, "build") {
				aDef = a
				break
			}
		}
	}

	override, hasOverride := r.config.Agents[aDef.Name]
	
	// AppConfig provider override
	if hasOverride && override.Provider != "" {
		if p, ok := r.config.Provider(override.Provider); ok {
			active = p
		}
	}

	client, err := r.providerClient(active)
	if err != nil {
		r.agent = nil
		r.contextWindow = 0
		return
	}
	r.contextWindow = active.ContextWindow

	r.agent = r.buildAgentFromDef(client, aDef, override, hasOverride)
}

func (r *Runtime) buildAgentFromDef(client provider.Client, aDef agent.AgentDef, override config.AgentOverride, hasOverride bool) *agent.Agent {
	// Apply overrides
	sysPrompt := aDef.System
	if hasOverride && override.SystemPrompt != "" {
		sysPrompt = override.SystemPrompt
	}
	
	allowedTools := aDef.Permissions.AllowedTools
	if hasOverride && len(override.ToolFilter) > 0 {
		allowedTools = override.ToolFilter
	}
	
	deniedTools := aDef.Permissions.DeniedTools
	if hasOverride && len(override.ExcludeTools) > 0 {
		deniedTools = override.ExcludeTools
	}

	toolsRegistry := r.tools.Filter(func(t tools.Tool) bool {
		name := t.Spec().Name
		
		// 1. Check explicit allow list
		if len(allowedTools) > 0 {
			allowed := false
			for _, allowedName := range allowedTools {
				if strings.EqualFold(name, allowedName) {
					allowed = true
					break
				}
			}
			if !allowed {
				return false
			}
		}
		
		// 2. Check explicit deny list
		if len(deniedTools) > 0 {
			for _, deniedName := range deniedTools {
				if strings.EqualFold(name, deniedName) {
					return false
				}
			}
		}
		
		// 3. Fallback to boolean permissions
		if tools.IsWriteTool(name) && !aDef.Permissions.AllowWrite {
			return false
		}
		
		if name == "delegate" && !aDef.Permissions.AllowDelegate {
			return false
		}
		
		if name == "task" && !aDef.Permissions.AllowTask {
			return false
		}
		
		if strings.HasPrefix(name, "brain_") && !aDef.Permissions.AllowBrainWrite {
			// Actually brain_read and brain_list should be allowed even if AllowBrainWrite is false
			if name == "brain_write" {
				return false
			}
		}
		
		if (name == "web_search" || name == "web_fetch") && !aDef.Permissions.AllowWebAccess {
			return false
		}
		
		return true
	})

	newAgent := agent.New(client, toolsRegistry)
	newAgent.SetDebug(r.debug)
	newAgent.SetAgentOverride(sysPrompt)
	
	// We can also apply Temperature, ThinkingBudget, and MaxIterations if added to agent
	// For now we just create the agent
	return newAgent
}


// ActiveSubagents returns a sorted list of currently active subagent labels.
func (r *Runtime) ActiveSubagents() []string {
	r.subagentsMu.Lock()
	defer r.subagentsMu.Unlock()
	var list []string
	for id := range r.activeSubagents {
		list = append(list, id)
	}
	sort.Strings(list)
	return list
}

func (r *Runtime) Start(ctx context.Context) {
	r.backgroundCtx = ctx
	if r.tachikomas != nil {
		r.tachikomas.Start(ctx)
	}

	// Start update check in background
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
		info, err := upd.CheckVersion(ctx)
		if err != nil {
			r.updateErr = err
			return
		}
		r.updateInfo = info
	}()
}

// WaitForUpdate blocks until background update check is complete and returns the result.
func (r *Runtime) WaitForUpdate() (*updater.VersionInfo, error) {
	if r.backgroundCtx != nil {
		select {
		case <-r.updateDone:
			return r.updateInfo, r.updateErr
		case <-r.backgroundCtx.Done():
			return nil, r.backgroundCtx.Err()
		}
	}
	<-r.updateDone
	return r.updateInfo, r.updateErr
}

// Tachikomas returns the background worker manager.
func (r *Runtime) Tachikomas() *tachikoma.Manager {
	return r.tachikomas
}

func (r *Runtime) GetContextInfo() system.ContextInfo {
	if r.tachikomas != nil {
		return r.tachikomas.GetContextInfo()
	}
	return system.GetContextInfo()
}

// SystemPrompt returns the current raw system prompt for debugging purposes.
func (r *Runtime) SystemPrompt(info system.ContextInfo) string {
	if r.agent == nil {
		return "Agent not configured."
	}
	// Make sure we enrich it first like we do in RunAgent
	enriched := r.enrichContext(context.Background(), info, "")
	return r.agent.SystemPrompt(enriched)
}

func (r *Runtime) providerClient(cfg config.ProviderConfig) (provider.Client, error) {
	if r.newProviderClient != nil {
		return r.newProviderClient(cfg)
	}
	return provider.NewClient(cfg)
}

func (r *Runtime) GetBrain() *brain.Brain {
	return r.brain
}

func (r *Runtime) Version() string {
	return r.version
}
