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
	"github.com/Hoosk/motoko/internal/system"
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
	mode             Mode
	inputMode        InputMode
	pending          *pendingShell
	tools            *tools.Registry
	agent            *agent.Agent
	config           *config.AppConfig
	debug            bool
	semantic         *semantic.Index
	currentAgentName string
	availableAgents  []agent.AgentDef
	currentSession   *session.Session
	workspaceID      string
	contextWindow    int
	wasResumed       bool
	mentionedFiles   []string
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
		mode:             ModePlan,
		inputMode:        InputModeChat,
		tools:            toolsRegistry,
		config:           cfg,
		semantic:         semantic.NewIndex(),
		currentAgentName: "plan",
		availableAgents:  allAgents,
		workspaceID:      workspaceID,
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
	client, err := provider.NewClient(providerCfg)
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

func (r *Runtime) ListSessions() ([]*session.Session, error) {
	return session.List(r.workspaceID)
}

func (r *Runtime) LoadSession(id string) error {
	s, err := session.Load(r.workspaceID, id)
	if err != nil {
		return err
	}
	r.currentSession = s
	return nil
}

func (r *Runtime) CurrentSessionEntries() []Entry {
	if r.currentSession == nil || len(r.currentSession.Messages) == 0 {
		return nil
	}
	entries := make([]Entry, 0, len(r.currentSession.Messages))
	for _, msg := range r.currentSession.Messages {
		switch msg.Role {
		case "user":
			entries = append(entries, Entry{Kind: EntryUser, Text: msg.Content})
		case "assistant":
			entries = append(entries, Entry{Kind: EntryAssistant, Text: msg.Content})
		default:
			entries = append(entries, Entry{Kind: EntrySystem, Text: msg.Content})
		}
	}
	return entries
}

func (r *Runtime) MentionSuggestions(input string) []string {
	token, ok := trailingMentionToken(input)
	if !ok {
		return nil
	}
	prefix := strings.ToLower(strings.TrimPrefix(token, "@"))
	var result []string
	for _, name := range r.AgentNames() {
		if prefix == "" || strings.HasPrefix(strings.ToLower(name), prefix) {
			result = append(result, "@"+name)
		}
	}
	if r.semantic != nil {
		if snapshot, err := r.semantic.Ensure(context.Background()); err == nil && snapshot != nil {
			seen := make(map[string]struct{})
			for _, file := range snapshot.Files {
				path := file.Path
				if _, ok := seen[path]; ok {
					continue
				}
				if prefix == "" || strings.Contains(strings.ToLower(path), prefix) {
					seen[path] = struct{}{}
					result = append(result, "@"+path)
				}
			}
		}
	}
	if len(result) > 8 {
		result = result[:8]
	}
	return result
}

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

func (r *Runtime) Completions(input string) []string {
	trimmed := strings.TrimSpace(input)
	hasTrailingSpace := strings.HasSuffix(input, " ")
	if trimmed == "" {
		if r.inputMode == InputModeShell {
			return []string{"ls", "pwd", "git status", "go build ./...", "/chat"}
		}
		return []string{"/help", "/provider add", "/models", "/sessions", "/tool read README.md", "!git status"}
	}

	if r.inputMode == InputModeShell && !strings.HasPrefix(trimmed, "/") && !strings.HasPrefix(trimmed, "!") {
		return shellCompletions(trimmed)
	}

	if strings.HasPrefix(trimmed, "!") {
		command := strings.TrimSpace(strings.TrimPrefix(trimmed, "!"))
		if command == "" {
			return []string{"!git status", "!go build ./...", "!ls"}
		}
		return []string{"!" + command}
	}

	if !strings.HasPrefix(trimmed, "/") {
		return nil
	}

	parts := strings.Fields(strings.TrimPrefix(trimmed, "/"))
	if len(parts) == 0 {
		return commandCompletions("")
	}

	if len(parts) == 1 && !hasTrailingSpace {
		return commandCompletions(parts[0])
	}

	if strings.EqualFold(parts[0], "tool") {
		prefix := ""
		if len(parts) > 1 {
			prefix = parts[1]
		}
		matches := r.tools.Suggestions(prefix)
		result := make([]string, 0, len(matches))
		for _, spec := range matches {
			result = append(result, "/tool "+spec.Usage)
		}
		return result
	}

	if strings.EqualFold(parts[0], "models") {
		active, ok := r.config.Active()
		if !ok || len(active.Models) == 0 {
			return []string{"/models"}
		}
		prefix := ""
		if len(parts) > 1 {
			prefix = strings.Join(parts[1:], " ")
		}
		var result []string
		for _, model := range active.Models {
			if prefix == "" || strings.HasPrefix(strings.ToLower(model), strings.ToLower(prefix)) {
				result = append(result, "/models "+model)
			}
		}
		if len(result) > 0 {
			return result
		}
	}

	return nil
}

func (r *Runtime) cacheProviderModels(providerName string, models []string) {
	if r.config == nil || strings.TrimSpace(providerName) == "" || len(models) == 0 {
		return
	}
	providerCfg, ok := r.config.Provider(providerName)
	if !ok {
		return
	}
	providerCfg.Models = config.UniqueSortedKeep(providerCfg.Models, models...)
	r.config.UpsertProvider(providerCfg)
	_ = r.config.Save()
}

func (r *Runtime) HandleInput(input string, info system.ContextInfo) Response {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return Response{}
	}

	if strings.HasPrefix(trimmed, "/") {
		return r.handleSlashCommand(trimmed, info)
	}

	for _, field := range strings.Fields(trimmed) {
		if strings.HasPrefix(field, "@") && r.isAgentMention(field) {
			r.SetAgentMode(strings.TrimPrefix(field, "@"))
			break
		}
	}
	r.mentionedFiles = r.extractMentionedFiles(trimmed)

	if strings.HasPrefix(trimmed, "!") {
		return r.handleShell(strings.TrimSpace(trimmed[1:]))
	}

	if r.inputMode == InputModeShell {
		return r.handleShell(trimmed)
	}

	if r.AgentConfigured() {
		return Response{Entries: []Entry{{Kind: EntryUser, Text: trimmed}}, Action: &Action{Type: ActionAgent, AgentPrompt: trimmed}}
	}

	return Response{Entries: []Entry{
		{Kind: EntryUser, Text: trimmed},
		{Kind: EntryAssistant, Text: "El runtime esta operativo pero el agente no esta listo. Configura un provider con /provider add y luego selecciona modelo con /models <modelo>."},
	}}
}

func (r *Runtime) HandleShellResult(result ShellResult) Response {
	status := fmt.Sprintf("Comando finalizado en %s con salida %d.", result.Duration.Round(10_000_000), result.ExitCode)
	entries := []Entry{{Kind: EntrySystem, Text: status}}

	output := strings.TrimSpace(result.Output)
	if output == "" {
		output = "(sin salida)"
	}

	if result.ExitCode == 0 {
		entries = append(entries, Entry{Kind: EntryOutput, Text: output})
		return Response{Entries: entries}
	}

	entries = append(entries, Entry{Kind: EntryError, Text: output})
	return Response{Entries: entries}
}

func (r *Runtime) handleSlashCommand(input string, info system.ContextInfo) Response {
	parts := strings.Fields(strings.TrimPrefix(input, "/"))
	if len(parts) == 0 {
		return Response{}
	}

	command := strings.ToLower(parts[0])

	switch command {
	case "help":
		return Response{Entries: []Entry{{Kind: EntryHelp, Text: strings.Join([]string{
			"Comandos disponibles:",
			"/help     muestra esta ayuda",
			"/clear    limpia la timeline",
			"/compact  compacta manualmente la sesion activa",
			"/mode     abre el selector de modo/agente",
			"/plan     activa modo de solo lectura",
			"/build    activa modo de trabajo",
			"/shell    activa modo shell para ejecutar comandos directos",
			"/chat     vuelve al modo normal de chat",
			"/status   resume modo, permisos y aprobaciones",
			"/debug    activa o desactiva trazas del agente",
			"/context  muestra contexto local y git",
			"/provider gestiona providers configurados",
			"/models   lista o selecciona modelos del provider activo",
			"/sessions lista o cambia de sesion del workspace",
			"/tools    muestra las tools registradas",
			"/tool     ejecuta una tool real del runtime",
			"/approve  ejecuta la accion pendiente",
			"/deny     cancela la accion pendiente",
			"!<cmd>    ejecuta un comando shell explicito",
		}, "\n")}}}
	case "clear":
		if r.currentSession != nil {
			r.currentSession.Messages = nil
			r.currentSession.LastInputTokens = 0
			_ = r.currentSession.Save()
		}
		return Response{Clear: true, Entries: []Entry{{Kind: EntrySystem, Text: "Timeline reiniciada."}}}
	case "compact":
		return Response{
			Entries: []Entry{{Kind: EntrySystem, Text: "Compactando sesion..."}},
			Action:  &Action{Type: ActionCompact},
		}
	case "plan":
		r.SetAgentMode("plan")
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: "Modo activo: plan. Los comandos shell requieren aprobacion explicita."}}}
	case "build":
		r.SetAgentMode("build")
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: "Modo activo: build. Los comandos seguros se ejecutan directamente; los sensibles se quedan pendientes de aprobacion."}}}
	case "mode":
		return Response{Signal: "open-mode-popup"}
	case "shell":
		r.inputMode = InputModeShell
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: "Modo de entrada: shell. Cualquier linea que no empiece por / se ejecuta como comando."}}}
	case "chat":
		r.inputMode = InputModeChat
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: "Modo de entrada: chat. La entrada normal vuelve a tratarse como prompt."}}}
	case "status":
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: r.statusText(info)}}}
	case "debug":
		r.debug = !r.debug
		if r.agent != nil {
			r.agent.SetDebug(r.debug)
		}
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Debug agente: %t", r.debug)}}}
	case "context":
		enriched := r.enrichContext(context.Background(), info, "")
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: strings.Join([]string{
			fmt.Sprintf("workspace: %s", enriched.Workspace),
			fmt.Sprintf("path: %s", enriched.Path),
			fmt.Sprintf("git: %s", enriched.GitSummary()),
			fmt.Sprintf("provider: %s", r.ProviderSummary()),
			"signals:",
			enriched.SignalSummary(),
			"semantic:",
			enriched.SemanticSummary,
			"relevant files:",
			enriched.RelevantFilesSummary(),
			"relevant snippets:",
			enriched.RelevantSnippetsSummary(),
		}, "\n")}}}
	case "provider":
		return r.handleProviderCommand(parts[1:])
	case "models":
		return r.handleModelsCommand(parts[1:])
	case "sessions":
		return Response{Signal: "open-sessions-popup"}
	case "tools":
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: formatToolList(r.tools.Specs())}}}
	case "tool":
		if len(parts) < 2 {
			return Response{Entries: []Entry{{Kind: EntryError, Text: "Uso: /tool <nombre> <args>. Usa /tools para listar disponibles."}}}
		}

		toolName := parts[1]
		toolArgs := ""
		if len(parts) > 2 {
			toolArgs = strings.Join(parts[2:], " ")
		}
		if strings.EqualFold(toolName, "bash") {
			return r.handleShell(toolArgs)
		}

		result, err := r.tools.Run(context.Background(), toolName, toolArgs)
		if err != nil {
			return Response{Entries: []Entry{{Kind: EntryError, Text: err.Error()}}}
		}

		entries := []Entry{
			{Kind: EntryCommand, Text: fmt.Sprintf("tool %s %s", toolName, strings.TrimSpace(toolArgs))},
			{Kind: EntrySystem, Text: result.Summary},
		}
		if strings.TrimSpace(result.Output) != "" {
			entries = append(entries, Entry{Kind: EntryOutput, Text: result.Output})
		}
		return Response{Entries: entries}
	case "approve":
		if r.pending == nil {
			return Response{Entries: []Entry{{Kind: EntrySystem, Text: "No hay ninguna accion pendiente."}}}
		}

		pending := r.pending
		r.pending = nil
		return Response{
			Entries: []Entry{
				{Kind: EntryCommand, Text: "$ " + pending.Command},
				{Kind: EntrySystem, Text: "Aprobacion recibida. Ejecutando comando..."},
			},
			Action: &Action{Type: ActionShell, ShellCommand: pending.Command},
		}
	case "deny":
		if r.pending == nil {
			return Response{Entries: []Entry{{Kind: EntrySystem, Text: "No hay ninguna accion pendiente."}}}
		}

		command := r.pending.Command
		r.pending = nil
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Accion cancelada: %s", command)}}}
	default:
		return Response{Entries: []Entry{{Kind: EntryError, Text: fmt.Sprintf("Comando desconocido: /%s", command)}}}
	}
}

func commandCompletions(prefix string) []string {
	commands := []string{"help", "clear", "compact", "mode", "plan", "build", "shell", "chat", "status", "debug", "context", "provider", "models", "sessions", "tools", "tool", "approve", "deny"}
	prefix = strings.ToLower(prefix)
	var result []string
	for _, command := range commands {
		if strings.HasPrefix(command, prefix) {
			result = append(result, "/"+command)
		}
	}
	return result
}

func shellCompletions(prefix string) []string {
	commands := []string{"ls", "pwd", "git status", "git diff", "go build ./...", "go test ./...", "npm test"}
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if prefix == "" {
		return commands
	}

	var result []string
	for _, command := range commands {
		if strings.HasPrefix(strings.ToLower(command), prefix) {
			result = append(result, command)
		}
	}
	if len(result) == 0 {
		return []string{prefix}
	}
	return result
}

func formatToolList(specs []tools.Spec) string {
	lines := []string{"Tools registradas:"}
	for _, spec := range specs {
		lines = append(lines, fmt.Sprintf("- %s: %s", spec.Usage, spec.Summary))
	}
	return strings.Join(lines, "\n")
}

func (r *Runtime) handleShell(command string) Response {
	if command == "" {
		return Response{Entries: []Entry{{Kind: EntryError, Text: "Falta el comando despues de !"}}}
	}

	decision := classifyShell(r.mode, command)
	if decision.Deny {
		return Response{Entries: []Entry{{Kind: EntryError, Text: decision.Reason}}}
	}

	if decision.RequiresApproval {
		r.pending = &pendingShell{Command: command}
		return Response{Entries: []Entry{
			{Kind: EntryCommand, Text: "$ " + command},
			{Kind: EntrySystem, Text: fmt.Sprintf("Accion pendiente: %s Usa /approve o /deny.", decision.Reason)},
		}}
	}

	return Response{
		Entries: []Entry{
			{Kind: EntryCommand, Text: "$ " + command},
			{Kind: EntrySystem, Text: "Ejecutando comando..."},
		},
		Action: &Action{Type: ActionShell, ShellCommand: command},
	}
}

func (r *Runtime) statusText(info system.ContextInfo) string {
	pending := "ninguna"
	if r.pending != nil {
		pending = r.pending.Command
	}

	return strings.Join([]string{
		fmt.Sprintf("modo: %s", r.mode),
		fmt.Sprintf("entrada: %s", r.inputMode),
		fmt.Sprintf("agente configurado: %t", r.AgentConfigured()),
		fmt.Sprintf("provider activo: %s", r.ProviderSummary()),
		fmt.Sprintf("workspace: %s", info.Workspace),
		fmt.Sprintf("git: %s", info.GitSummary()),
		fmt.Sprintf("aprobacion pendiente: %s", pending),
		"politica: plan pide aprobacion para shell; build ejecuta seguro y pide aprobacion para comandos sensibles.",
	}, "\n")
}

func (r *Runtime) handleProviderCommand(args []string) Response {
	if len(args) == 0 {
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: strings.Join([]string{
			"Uso de providers:",
			"/provider list",
			"/provider add",
			"/provider use <name>",
			"/provider remove <name>",
		}, "\n")}}}
	}

	subcommand := strings.ToLower(args[0])
	switch subcommand {
	case "list":
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: r.providerListText()}}}
	case "add":
		return Response{Signal: "open-provider-popup", Entries: []Entry{{Kind: EntrySystem, Text: "Abriendo formulario de provider..."}}}
	case "use":
		if len(args) < 2 {
			return Response{Entries: []Entry{{Kind: EntryError, Text: "Uso: /provider use <name>"}}}
		}
		if err := r.config.SetActive(args[1]); err != nil {
			return Response{Entries: []Entry{{Kind: EntryError, Text: err.Error()}}}
		}
		if err := r.config.Save(); err != nil {
			return Response{Entries: []Entry{{Kind: EntryError, Text: err.Error()}}}
		}
		r.refreshAgent()
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Provider activo: %s", r.ProviderSummary())}}}
	case "remove":
		if len(args) < 2 {
			return Response{Entries: []Entry{{Kind: EntryError, Text: "Uso: /provider remove <name>"}}}
		}
		if !r.config.RemoveProvider(args[1]) {
			return Response{Entries: []Entry{{Kind: EntryError, Text: fmt.Sprintf("Provider no encontrado: %s", args[1])}}}
		}
		if err := r.config.Save(); err != nil {
			return Response{Entries: []Entry{{Kind: EntryError, Text: err.Error()}}}
		}
		r.refreshAgent()
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Provider eliminado: %s", args[1])}}}
	default:
		return Response{Entries: []Entry{{Kind: EntryError, Text: fmt.Sprintf("Subcomando desconocido: %s", subcommand)}}}
	}
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

func (r *Runtime) handleModelsCommand(args []string) Response {
	active, ok := r.config.Active()
	if !ok {
		return Response{Entries: []Entry{{Kind: EntryError, Text: "No hay provider activo. Usa /provider add o /provider use primero."}}}
	}

	if len(args) == 0 {
		// Open interactive model picker popup.
		return Response{Signal: "open-models-popup"}
	}

	model := strings.TrimSpace(strings.Join(args, " "))
	active.Model = model
	active.Models = config.UniqueSortedKeep(active.Models, model)
	active.ContextWindow = 0
	r.config.UpsertProvider(active)
	if err := r.config.Save(); err != nil {
		return Response{Entries: []Entry{{Kind: EntryError, Text: err.Error()}}}
	}
	r.refreshAgent()
	return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Modelo activo para %s: %s", active.Name, active.Model)}}}
}

func (r *Runtime) providerListText() string {
	if r.config == nil || len(r.config.Providers) == 0 {
		return "No hay providers configurados. Usa /provider add."
	}

	lines := []string{"Providers configurados:"}
	for _, providerCfg := range r.config.Providers {
		marker := " "
		if strings.EqualFold(providerCfg.Name, r.config.ActiveProvider) {
			marker = "*"
		}
		model := providerCfg.Model
		if strings.TrimSpace(model) == "" {
			model = "no-model"
		}
		label := string(providerCfg.Preset)
		if strings.TrimSpace(label) == "" {
			label = string(providerCfg.Kind)
		}
		lines = append(lines, fmt.Sprintf("%s %s [%s] %s", marker, providerCfg.Name, label, model))
	}
	return strings.Join(lines, "\n")
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
	client, err := provider.NewClient(active)
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

func (r *Runtime) RunAgent(ctx context.Context, info system.ContextInfo, input string) (agent.Result, error) {
	if r.agent == nil || !r.agent.Configured() {
		return agent.Result{}, fmt.Errorf("agente no configurado")
	}
	info = r.enrichContext(ctx, info, input)
	priorHistory := []provider.Message(nil)
	if r.currentSession != nil {
		priorHistory = append(priorHistory, r.currentSession.Messages...)
	}
	result, err := r.agent.Run(ctx, info, input, priorHistory)
	if err != nil {
		return result, err
	}
	r.persistTurn(result)
	return result, r.maybeAutoCompact(ctx, nil)
}

func (r *Runtime) RunAgentStream(ctx context.Context, info system.ContextInfo, input string, onEvent func(AgentStreamEvent) error) (agent.Result, error) {
	if r.agent == nil || !r.agent.Configured() {
		return agent.Result{}, fmt.Errorf("agente no configurado")
	}
	info = r.enrichContext(ctx, info, input)
	priorHistory := []provider.Message(nil)
	firstTurn := r.currentSession == nil || len(r.currentSession.Messages) == 0
	if r.currentSession != nil {
		priorHistory = append(priorHistory, r.currentSession.Messages...)
	}
	result, err := r.agent.RunStream(ctx, info, input, priorHistory, func(event agent.StreamEvent) error {
		if onEvent == nil {
			return nil
		}
		return onEvent(AgentStreamEvent{Kind: event.Kind, Title: event.Title, Content: event.Content})
	})
	if err != nil {
		return result, err
	}
	r.persistTurn(result)
	if firstTurn {
		go r.generateTitle(context.Background(), input, result.Assistant)
	}
	if err := r.maybeAutoCompact(ctx, onEvent); err != nil {
		return result, err
	}
	return result, nil
}

func (r *Runtime) CompactSession(ctx context.Context) Response {
	if err := r.doCompact(ctx); err != nil {
		return Response{Entries: []Entry{{Kind: EntryError, Text: err.Error()}}}
	}
	return Response{Entries: []Entry{{Kind: EntrySystem, Text: "Sesion compactada."}}}
}

func (r *Runtime) persistTurn(result agent.Result) {
	if r.currentSession == nil {
		workspacePath, _ := os.Getwd()
		r.currentSession = session.New(r.workspaceID, workspacePath)
	}
	r.currentSession.Messages = append([]provider.Message(nil), result.Messages...)
	r.currentSession.LastInputTokens = result.Usage.InputTokens
	_ = r.currentSession.Save()
}

func (r *Runtime) maybeAutoCompact(ctx context.Context, onEvent func(AgentStreamEvent) error) error {
	if r.currentSession == nil || r.contextWindow <= 0 || r.currentSession.LastInputTokens <= 0 {
		return nil
	}
	if float64(r.currentSession.LastInputTokens)/float64(r.contextWindow) < 0.80 {
		return nil
	}
	if onEvent != nil {
		_ = onEvent(AgentStreamEvent{Kind: "compacting", Content: "Compactando sesion..."})
	}
	err := r.doCompact(ctx)
	if err == nil && onEvent != nil {
		_ = onEvent(AgentStreamEvent{Kind: "status", Content: "Sesion compactada automaticamente."})
	}
	return err
}

func (r *Runtime) doCompact(ctx context.Context) error {
	if r.currentSession == nil || len(r.currentSession.Messages) == 0 {
		return nil
	}
	active, ok := r.config.Active()
	if !ok {
		return fmt.Errorf("no hay provider activo")
	}
	client, err := provider.NewClient(active)
	if err != nil {
		return err
	}
	resp, err := client.Complete(ctx,
		"Resume la conversacion para continuarla despues. Devuelve un resumen concreto, con decisiones, estado actual y siguientes pasos.",
		r.currentSession.Messages,
		nil,
	)
	if err != nil {
		return err
	}
	r.currentSession.CompactWith(resp.Message)
	return r.currentSession.Save()
}

func (r *Runtime) generateTitle(ctx context.Context, userInput, assistantResponse string) {
	if r.currentSession == nil {
		return
	}
	if strings.TrimSpace(r.currentSession.Title) != "" && !strings.EqualFold(strings.TrimSpace(r.currentSession.Title), "Nueva sesion") {
		return
	}
	active, ok := r.config.Active()
	if !ok {
		return
	}
	client, err := provider.NewClient(active)
	if err != nil {
		return
	}
	resp, err := client.Complete(ctx,
		"Genera un titulo corto de 4 a 8 palabras para esta sesion. Devuelve solo el titulo en el campo message.",
		[]provider.Message{{Role: "user", Content: userInput}, {Role: "assistant", Content: assistantResponse}},
		nil,
	)
	if err != nil {
		return
	}
	title := strings.TrimSpace(resp.Message)
	if title == "" {
		return
	}
	r.currentSession.Title = title
	_ = r.currentSession.Save()
}
func (r *Runtime) enrichContext(ctx context.Context, info system.ContextInfo, input string) system.ContextInfo {
	if r.semantic == nil {
		return info
	}
	snapshot, err := r.semantic.Ensure(ctx)
	if err != nil {
		if info.Signals == nil {
			info.Signals = make(map[string]string)
		}
		info.Signals["semantic"] = err.Error()
		return info
	}
	info.SemanticSummary = snapshot.Summary()
	relevant := snapshot.RelevantFiles(input, 4)
	info.RelevantFiles = make([]string, 0, len(relevant))
	for _, file := range relevant {
		info.RelevantFiles = append(info.RelevantFiles, file.Descriptor())
	}
	snippets := snapshot.RelevantSnippets(input, 3, 180)
	info.RelevantSnippets = make([]string, 0, len(snippets))
	for _, snippet := range snippets {
		info.RelevantSnippets = append(info.RelevantSnippets, snippet.Descriptor())
	}
	if info.Signals == nil {
		info.Signals = make(map[string]string)
	}
	info.Signals["semantic"] = snapshot.Summary()
	for _, path := range r.mentionedFiles {
		for _, file := range snapshot.Files {
			if file.Path != path {
				continue
			}
			info.RelevantFiles = append([]string{file.Descriptor()}, info.RelevantFiles...)
			content := string(file.Content)
			lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
			limit := min(120, len(lines))
			var numbered []string
			for i := 0; i < limit; i++ {
				numbered = append(numbered, fmt.Sprintf("%d: %s", i+1, lines[i]))
			}
			info.RelevantSnippets = append([]string{fmt.Sprintf("FILE %s\nLINES 1-%d\nREASON explicit @ mention\n%s", file.Path, limit, strings.Join(numbered, "\n"))}, info.RelevantSnippets...)
			break
		}
	}
	return info
}
