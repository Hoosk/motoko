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
	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tachikoma"
	"github.com/Hoosk/motoko/internal/tools"
	"github.com/Hoosk/motoko/internal/updater"

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

type pendingShell struct {
	Command string
}

type Runtime struct {
	backgroundCtx     context.Context
	updateErr         error
	semantic          *semantic.Index
	agent             *agent.Agent
	newProviderClient func(config.ProviderConfig) (provider.Client, error)
	config            *config.AppConfig
	taskMgr           *taskman.Manager
	sesMgr            *sessionman.Manager
	updateInfo        *updater.VersionInfo
	tachikomas        *tachikoma.Manager
	activeSubagents   map[string]*SubagentInfo
	pending           *pendingShell
	tools             *tools.Registry
	updateDone        chan struct{}
	currentAgentName  string
	inputMode         InputMode
	mode              Mode
	version           string
	availableAgents   []agent.AgentDef
	mentionedFiles    []string
	availableSkills   []skills.Skill
	contextWindow     int
	subagentsMu       sync.Mutex
	debug             bool
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
		mode:              ModePlan,
		inputMode:         InputModeChat,
		tools:             toolsRegistry,
		newProviderClient: provider.NewClient,
		config:            cfg,
		sesMgr:            sesMgr,
		semantic:          semantic.NewIndex(),
		tachikomas:        tachikoma.NewManager(),
		currentAgentName:  string(ModePlan),
		availableAgents:   allAgents,
		availableSkills:   sList,
		activeSubagents:   make(map[string]*SubagentInfo),
		taskMgr:           taskman.NewManager(),
		updateDone:        make(chan struct{}),
		version:           runtimeOpts.Version,
	}

	if r.config.Theme != "" {
		styles.SetTheme(r.config.Theme)
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
			r.refreshAgent()
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
	maxOutputSize := system.MaxToolOutputBytes(r.contextWindow)
	tCtx := tools.ToolContext{
		Workspace:       r.sesMgr.WorkspaceID(),
		ActiveMode:      string(r.mode),
		MaxOutputSize:   maxOutputSize,
	}
	if len(r.availableAgents) > 0 {
		for _, a := range r.availableAgents {
			tCtx.AvailableAgents = append(tCtx.AvailableAgents, a.Name)
		}
	}
	if len(r.availableSkills) > 0 {
		for _, s := range r.availableSkills {
			tCtx.AvailableSkills = append(tCtx.AvailableSkills, s.Name)
		}
	}
	return r.tools.Specs(tCtx)
}

func (r *Runtime) ToolSuggestions(prefix string) []tools.Spec {
	maxOutputSize := system.MaxToolOutputBytes(r.contextWindow)
	tCtx := tools.ToolContext{
		Workspace:       r.sesMgr.WorkspaceID(),
		ActiveMode:      string(r.mode),
		MaxOutputSize:   maxOutputSize,
	}
	if len(r.availableAgents) > 0 {
		for _, a := range r.availableAgents {
			tCtx.AvailableAgents = append(tCtx.AvailableAgents, a.Name)
		}
	}
	if len(r.availableSkills) > 0 {
		for _, s := range r.availableSkills {
			tCtx.AvailableSkills = append(tCtx.AvailableSkills, s.Name)
		}
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
		return valNone
	}
	active, ok := r.config.Active()
	if !ok {
		return valNone
	}
	if strings.TrimSpace(active.Model) == "" {
		return fmt.Sprintf("%s (%s:no-model)", active.Name, active.Preset)
	}
	return fmt.Sprintf("%s (%s:%s)", active.Name, active.Preset, active.Model)
}

func (r *Runtime) ProviderPresets() []config.ProviderPreset {
	presets := config.ValidProviderPresets()
	seen := make(map[config.ProviderPreset]bool)
	for _, p := range presets {
		seen[p] = true
	}

	for _, cp := range provider.ListCatalogProviders() {
		preset := config.ProviderPreset(cp)
		if !seen[preset] {
			presets = append(presets, preset)
			seen[preset] = true
		}
	}
	return presets
}

func (r *Runtime) LookupCatalogProvider(id string) (provider.CatalogProvider, bool) {
	return provider.LookupProvider(id)
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
		return fmt.Errorf("no configuration")
	}
	active, ok := r.config.Active()
	if !ok {
		return fmt.Errorf("no active provider")
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
		return provider.ModelInfo{}, fmt.Errorf("no active provider")
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
		return fmt.Errorf("no configuration")
	}
	active, ok := r.config.Active()
	if !ok {
		return fmt.Errorf("no active provider")
	}
	if active.ContextWindow > 0 {
		maxAllowed := active.ContextWindow / 2
		if budget > maxAllowed {
			budget = maxAllowed
		}
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
	return r.sesMgr.HistoryInputTokens()
}

func (r *Runtime) SessionTitle() string {
	return r.sesMgr.SessionTitle()
}

func (r *Runtime) StartupEntries() []Entry {
	return r.sesMgr.StartupEntries()
}

func (r *Runtime) GetBrain() *brain.Brain {
	return r.sesMgr.Brain()
}

func (r *Runtime) ListSessions() ([]*session.Session, error) {
	return r.sesMgr.ListSessions()
}

func (r *Runtime) LoadSession(id string) error {
	return r.sesMgr.LoadSession(id)
}

func (r *Runtime) CurrentSessionEntries() []Entry {
	return r.sesMgr.CurrentSessionEntries()
}

func (r *Runtime) CompactSession(ctx context.Context) Response {
	return r.sesMgr.CompactSession(ctx, r.config, r.newProviderClient, r.contextWindow)
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
	if hasOverride && override.Model != "" {
		active.Model = override.Model
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


func (r *Runtime) ActiveSubagents() []string {
	r.subagentsMu.Lock()
	defer r.subagentsMu.Unlock()
	var list []string
	for id, info := range r.activeSubagents {
		if info.Progress != "" {
			list = append(list, fmt.Sprintf("%s: %s", id, info.Progress))
		} else {
			list = append(list, id)
		}
	}
	sort.Strings(list)
	return list
}

func (r *Runtime) Start(ctx context.Context) {
	r.backgroundCtx = ctx
	if r.tachikomas != nil {
		r.tachikomas.Start(ctx)
	}

	// Start catalog loading in background
	go func() {
		_ = provider.LoadCatalog(ctx)
	}()

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

func (r *Runtime) Version() string {
	return r.version
}
