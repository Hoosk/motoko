package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/semantic"
	"github.com/Hoosk/motoko/internal/session"
	"github.com/Hoosk/motoko/internal/tools"
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
	ActionAgent   ActionType = "agent"
	ActionCompact ActionType = "compact"
)

type Action struct {
	Type         ActionType
	ShellCommand string
	AgentPrompt  string
}

type Response struct {
	Entries []Entry
	Action  *Action
	Clear   bool
	Signal  string
}

type pendingShell struct {
	Command string
}

type Runtime struct {
	mode              Mode
	inputMode         InputMode
	pending           *pendingShell
	tools             *tools.Registry
	agent             *agent.Agent
	newProviderClient func(config.ProviderConfig) (provider.Client, error)
	config            *config.AppConfig
	debug             bool
	semantic          *semantic.Index
	currentAgentName  string
	availableAgents   []agent.AgentDef
	currentSession    *session.Session
	workspaceID       string
	contextWindow     int
	wasResumed        bool
	mentionedFiles    []string
}

type RuntimeOptions struct {
	Resume bool
}

type AgentStreamEvent struct {
	Kind    string
	Title   string
	Content string
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
	if customAgents, err := agent.LoadAgentsFile(".agents"); err == nil && len(customAgents) > 0 {
		allAgents = append(allAgents, customAgents...)
	}

	r := &Runtime{
		mode:              ModePlan,
		inputMode:         InputModeChat,
		tools:             toolsRegistry,
		newProviderClient: provider.NewClient,
		config:            cfg,
		semantic:          semantic.NewIndex(),
		currentAgentName:  "plan",
		availableAgents:   allAgents,
		workspaceID:       workspaceID,
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
	r.config.UpsertProvider(active)
	if err := r.config.Save(); err != nil {
		return err
	}
	r.refreshAgent()
	return nil
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
	client, err := r.providerClient(active)
	if err != nil {
		r.agent = nil
		r.contextWindow = 0
		return
	}
	r.contextWindow = active.ContextWindow
	r.agent = agent.New(client, r.tools)
	r.agent.SetDebug(r.debug)
	// Re-apply the current agent mode system prompt.
	for _, a := range r.availableAgents {
		if strings.EqualFold(a.Name, r.currentAgentName) {
			r.agent.SetAgentOverride(a.System)
			break
		}
	}
}

func (r *Runtime) providerClient(cfg config.ProviderConfig) (provider.Client, error) {
	if r.newProviderClient != nil {
		return r.newProviderClient(cfg)
	}
	return provider.NewClient(cfg)
}
