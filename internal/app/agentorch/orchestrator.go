package agentorch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	"github.com/Hoosk/motoko/internal/tools"
	"github.com/Hoosk/motoko/internal/tracelog"

	"github.com/Hoosk/motoko/internal/app/types"
)

type Orchestrator struct {
	semanticFn         func() *semantic.Index
	availableSkillsFn  func() []skills.Skill
	onMaybeAutoCompact func(ctx context.Context, onEvent func(types.AgentStreamEvent) error) error
	onGenerateTitle    func(ctx context.Context, userInput, assistantResponse string)
	onPersistTurn      func(agent.Result)
	activeSubagents    map[string]*types.SubagentInfo
	contextInfoFn      func() system.ContextInfo
	currentSessionFn   func() *session.Session
	brainFn            func() *brain.Brain
	toolsFn            func() *tools.Registry
	agent              *agent.Agent
	providerClientFn   func() func(config.ProviderConfig) (provider.Client, error)
	configFn           func() *config.AppConfig
	workspaceIDFn      func() string
	contextWindowFn    func() int
	availableAgentsFn  func() []agent.AgentDef
	currentAgentName   string
	mode               types.Mode
	testAgents         []agent.AgentDef
	testSkills         []skills.Skill
	mentionedFiles     []string
	subagentsMu        sync.Mutex
	debug              bool
}

type Deps struct {
	ConfigFn          func() *config.AppConfig
	ProviderClientFn  func() func(config.ProviderConfig) (provider.Client, error)
	ToolsFn           func() *tools.Registry
	SemanticFn        func() *semantic.Index
	BrainFn           func() *brain.Brain
	CurrentSessionFn  func() *session.Session
	WorkspaceIDFn     func() string
	ContextWindowFn   func() int
	AvailableAgentsFn func() []agent.AgentDef
	AvailableSkillsFn func() []skills.Skill
	ContextInfoFn     func() system.ContextInfo

	OnPersistTurn      func(agent.Result)
	OnGenerateTitle    func(ctx context.Context, userInput, assistantResponse string)
	OnMaybeAutoCompact func(ctx context.Context, onEvent func(types.AgentStreamEvent) error) error
}

func New(deps Deps) *Orchestrator {
	return &Orchestrator{
		mode:             types.ModePlan,
		currentAgentName: string(types.ModePlan),
		activeSubagents:  make(map[string]*types.SubagentInfo),

		configFn:          deps.ConfigFn,
		providerClientFn:  deps.ProviderClientFn,
		toolsFn:           deps.ToolsFn,
		semanticFn:        deps.SemanticFn,
		brainFn:           deps.BrainFn,
		currentSessionFn:  deps.CurrentSessionFn,
		workspaceIDFn:     deps.WorkspaceIDFn,
		contextWindowFn:   deps.ContextWindowFn,
		availableAgentsFn: deps.AvailableAgentsFn,
		availableSkillsFn: deps.AvailableSkillsFn,
		contextInfoFn:     deps.ContextInfoFn,

		onPersistTurn:      deps.OnPersistTurn,
		onGenerateTitle:    deps.OnGenerateTitle,
		onMaybeAutoCompact: deps.OnMaybeAutoCompact,
	}
}

func (o *Orchestrator) Mode() types.Mode                 { return o.mode }
func (o *Orchestrator) SetMode(m types.Mode)             { o.mode = m }
func (o *Orchestrator) AgentName() string                { return o.currentAgentName }
func (o *Orchestrator) Debug() bool                      { return o.debug }
func (o *Orchestrator) SetDebug(d bool)                  { o.debug = d }
func (o *Orchestrator) SetMentionedFiles(files []string) { o.mentionedFiles = files }
func (o *Orchestrator) AgentConfigured() bool            { return o.agent != nil && o.agent.Configured() }
func (o *Orchestrator) Agent() *agent.Agent              { return o.agent }

func (o *Orchestrator) AgentNames() []string {
	agents := o.AvailableAgents()
	result := make([]string, len(agents))
	for i, a := range agents {
		result[i] = a.Name
	}
	return result
}

func (o *Orchestrator) SetTestAgents(agents []agent.AgentDef) { o.testAgents = agents }
func (o *Orchestrator) SetTestSkills(skills []skills.Skill)   { o.testSkills = skills }

func (o *Orchestrator) AvailableAgents() []agent.AgentDef {
	if len(o.testAgents) > 0 {
		return o.testAgents
	}
	return o.availableAgentsFn()
}

func (o *Orchestrator) AvailableSkills() []skills.Skill {
	if len(o.testSkills) > 0 {
		return o.testSkills
	}
	return o.availableSkillsFn()
}

func (o *Orchestrator) MentionedFiles() []string {
	return o.mentionedFiles
}

func (o *Orchestrator) SetAgentMode(name string) {
	for _, a := range o.AvailableAgents() {
		if strings.EqualFold(a.Name, name) {
			o.currentAgentName = a.Name
			if strings.EqualFold(name, "build") {
				o.mode = types.ModeBuild
			} else {
				o.mode = types.ModePlan
			}
			o.RefreshAgent()
			return
		}
	}
}

func (o *Orchestrator) ActiveSubagents() []string {
	o.subagentsMu.Lock()
	defer o.subagentsMu.Unlock()
	var list []string
	for id, info := range o.activeSubagents {
		if info.Progress != "" {
			list = append(list, fmt.Sprintf("%s: %s", id, info.Progress))
		} else {
			list = append(list, id)
		}
	}
	sort.Strings(list)
	return list
}

func (o *Orchestrator) SystemPrompt(info system.ContextInfo) string {
	if o.agent == nil {
		return "Agent not configured."
	}
	enriched := o.EnrichContext(context.Background(), info, "")
	return o.agent.SystemPrompt(enriched)
}

func (o *Orchestrator) RefreshAgent() {
	cfg := o.configFn()
	if cfg == nil {
		o.agent = nil
		return
	}
	active, ok := cfg.Active()
	if !ok {
		o.agent = nil
		return
	}

	var aDef agent.AgentDef
	found := false
	for _, a := range o.AvailableAgents() {
		if strings.EqualFold(a.Name, o.currentAgentName) {
			aDef = a
			found = true
			break
		}
	}
	if !found {
		for _, a := range o.AvailableAgents() {
			if strings.EqualFold(a.Name, "build") {
				aDef = a
				break
			}
		}
	}

	override, hasOverride := cfg.Agents[aDef.Name]

	if hasOverride && override.Provider != "" {
		if p, ok := cfg.Provider(override.Provider); ok {
			active = p
		}
	}
	if hasOverride && override.Model != "" {
		active.Model = override.Model
	}

	providerFn := o.providerClientFn()
	client, err := providerFn(active)
	if err != nil {
		o.agent = nil
		return
	}

	o.agent = o.buildAgentFromDef(client, aDef, override, hasOverride)
}

func (o *Orchestrator) buildAgentFromDef(client provider.Client, aDef agent.AgentDef, override config.AgentOverride, hasOverride bool) *agent.Agent {
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
	if !hasOverride && aDef.Permissions.MaxIterations > 0 {
		override.MaxIterations = &aDef.Permissions.MaxIterations
		hasOverride = true
	}

	toolsRegistry := o.toolsFn().Filter(func(t tools.Tool) bool {
		name := t.Spec().Name
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
		if len(deniedTools) > 0 {
			for _, deniedName := range deniedTools {
				if strings.EqualFold(name, deniedName) {
					return false
				}
			}
		}
		if tools.IsWriteTool(name) && !aDef.Permissions.AllowWrite {
			return false
		}
		if name == "delegate" && !aDef.Permissions.AllowDelegate {
			return false
		}
		if name == "question" && !aDef.Permissions.AllowQuestion {
			return false
		}
		if name == "task" && !aDef.Permissions.AllowTask {
			return false
		}
		if strings.HasPrefix(name, "brain_") && !aDef.Permissions.AllowBrainWrite {
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
	newAgent.SetDebug(o.debug)
	newAgent.SetAgentOverride(sysPrompt)
	return newAgent
}

func (o *Orchestrator) createSubagent(name string, cfg tools.SubagentConfig) (*agent.Agent, error) {
	c := o.configFn()
	active, ok := c.Active()
	if !ok {
		return nil, fmt.Errorf("no active provider configured")
	}

	var aDef agent.AgentDef
	found := false
	for _, a := range o.AvailableAgents() {
		if strings.EqualFold(a.Name, name) {
			aDef = a
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("unknown agent: %s", name)
	}

	override := c.Agents[aDef.Name]

	hasOverride := true
	if len(cfg.ToolFilter) > 0 {
		override.ToolFilter = append(override.ToolFilter, cfg.ToolFilter...)
	}
	if !cfg.AllowDelegate {
		override.ExcludeTools = append(override.ExcludeTools, "delegate")
	}
	if cfg.MaxIterations > 0 {
		override.MaxIterations = &cfg.MaxIterations
	}

	if hasOverride && override.Provider != "" {
		if p, ok := c.Provider(override.Provider); ok {
			active = p
		}
	}
	if hasOverride && override.Model != "" {
		active.Model = override.Model
	}

	providerFn := o.providerClientFn()
	client, err := providerFn(active)
	if err != nil {
		return nil, err
	}

	return o.buildAgentFromDef(client, aDef, override, hasOverride), nil
}

type subagentDepthKey struct{}

func (o *Orchestrator) RunSubagent(ctx context.Context, cfg tools.SubagentConfig) (string, error) {
	currentDepth := 0
	if v, ok := ctx.Value(subagentDepthKey{}).(int); ok {
		currentDepth = v
	}
	maxDepth := cfg.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 2
	}
	if currentDepth >= maxDepth {
		return "", fmt.Errorf("maximum delegation depth reached (%d)", maxDepth)
	}

	o.subagentsMu.Lock()
	if o.activeSubagents == nil {
		o.activeSubagents = make(map[string]*types.SubagentInfo)
	}
	id := fmt.Sprintf("%s-%d", cfg.Mode, len(o.activeSubagents)+1)
	o.activeSubagents[id] = &types.SubagentInfo{
		Name:      cfg.Mode,
		Prompt:    cfg.Task,
		StartedAt: time.Now(),
	}
	o.subagentsMu.Unlock()

	defer func() {
		o.subagentsMu.Lock()
		delete(o.activeSubagents, id)
		o.subagentsMu.Unlock()
	}()

	sess := o.currentSessionFn()
	sessionID := ""
	if sess != nil {
		sessionID = sess.ID
	}
	subSessionID := fmt.Sprintf("%s-%s", sessionID, id)
	subBrain, err := brain.New(o.workspaceIDFn(), subSessionID)
	if err != nil {
		return "", err
	}
	defer func() { _ = subBrain.Destroy() }()

	if cfg.InheritBrain && o.brainFn() != nil {
		if copyErr := o.brainFn().CopyTo(subBrain); copyErr != nil {
			return "", copyErr
		}
	}

	sub, err := o.createSubagent(cfg.Mode, cfg)
	if err != nil {
		return "", err
	}

	subCtx := tools.WithBrain(ctx, subBrain)
	subCtx = tools.WithQuestionBroker(subCtx, tools.GetQuestionBroker(ctx))
	subCtx = tools.WithMaxOutputSize(subCtx, system.MaxToolOutputBytes(o.contextWindowFn()))
	subCtx = context.WithValue(subCtx, subagentDepthKey{}, currentDepth+1)
	reqID := fmt.Sprintf("subreq-%d", time.Now().UnixNano())
	subCtx = provider.WithTelemetry(subCtx, subSessionID, reqID)

	info := o.contextInfoFn()
	info = o.EnrichContext(subCtx, info, cfg.Task)

	res, err := sub.RunStream(subCtx, info, cfg.Task, nil, func(event agent.StreamEvent) error {
		if event.Kind == "tool_start" || event.Kind == "content" {
			o.subagentsMu.Lock()
			if subInfo, ok := o.activeSubagents[id]; ok {
				if event.Title != "" {
					subInfo.Progress = event.Title
				}
			}
			o.subagentsMu.Unlock()
			if cfg.ProgressChan != nil && event.Title != "" {
				select {
				case cfg.ProgressChan <- fmt.Sprintf("[%s] %s", cfg.Mode, event.Title):
				default:
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	tracelog.Logf("subagent %s finished usage_in=%d usage_out=%d total=%d", cfg.Mode, res.Usage.InputTokens, res.Usage.OutputTokens, res.Usage.TotalTokens)
	return res.Assistant, nil
}

func (o *Orchestrator) RunAgent(ctx context.Context, info system.ContextInfo, input string) (agent.Result, error) {
	if o.agent == nil || !o.agent.Configured() {
		return agent.Result{}, fmt.Errorf("agent not configured")
	}
	if info.Path == "" {
		info = o.contextInfoFn()
	}
	info = o.EnrichContext(ctx, info, input)
	priorHistory := []provider.ConversationItem(nil)
	if sess := o.currentSessionFn(); sess != nil {
		priorHistory = append(priorHistory, sess.History...)
		reqID := fmt.Sprintf("req-%d", time.Now().UnixNano())
		ctx = provider.WithTelemetry(ctx, sess.ID, reqID)
	}
	ctx = tools.WithBrain(ctx, o.brainFn())
	ctx = tools.WithQuestionBroker(ctx, tools.GetQuestionBroker(ctx))
	ctx = tools.WithMaxOutputSize(ctx, system.MaxToolOutputBytes(o.contextWindowFn()))
	result, err := o.agent.Run(ctx, info, input, priorHistory)
	if err != nil {
		return result, err
	}
	if o.onPersistTurn != nil {
		o.onPersistTurn(result)
	}
	if sess := o.currentSessionFn(); sess != nil && (strings.TrimSpace(sess.Title) == "" || strings.EqualFold(strings.TrimSpace(sess.Title), "New session")) {
		if o.onGenerateTitle != nil {
			go o.onGenerateTitle(ctx, input, result.Assistant)
		}
	}
	if o.onMaybeAutoCompact != nil {
		_ = o.onMaybeAutoCompact(ctx, nil)
	}
	return result, nil
}

func (o *Orchestrator) RunAgentStream(ctx context.Context, info system.ContextInfo, input string, onEvent func(types.AgentStreamEvent) error) (agent.Result, error) {
	if o.agent == nil || !o.agent.Configured() {
		return agent.Result{}, fmt.Errorf("agent not configured")
	}
	if info.Path == "" {
		info = o.contextInfoFn()
	}
	info = o.EnrichContext(ctx, info, input)
	priorHistory := []provider.ConversationItem(nil)
	if sess := o.currentSessionFn(); sess != nil {
		priorHistory = append(priorHistory, sess.History...)
		reqID := fmt.Sprintf("req-%d", time.Now().UnixNano())
		ctx = provider.WithTelemetry(ctx, sess.ID, reqID)
	}
	ctx = tools.WithBrain(ctx, o.brainFn())
	ctx = tools.WithQuestionBroker(ctx, tools.GetQuestionBroker(ctx))
	ctx = tools.WithMaxOutputSize(ctx, system.MaxToolOutputBytes(o.contextWindowFn()))
	result, err := o.agent.RunStream(ctx, info, input, priorHistory, func(event agent.StreamEvent) error {
		if onEvent == nil {
			return nil
		}
		return onEvent(types.AgentStreamEvent{
			Kind:             event.Kind,
			Title:            event.Title,
			Content:          event.Content,
			ReasoningContent: event.ReasoningContent,
		})
	})
	if err != nil {
		return result, err
	}
	if o.onPersistTurn != nil {
		o.onPersistTurn(result)
	}
	if sess := o.currentSessionFn(); sess != nil && (strings.TrimSpace(sess.Title) == "" || strings.EqualFold(strings.TrimSpace(sess.Title), "New session")) {
		if o.onGenerateTitle != nil {
			go o.onGenerateTitle(ctx, input, result.Assistant)
		}
	}
	if o.onMaybeAutoCompact != nil {
		_ = o.onMaybeAutoCompact(ctx, onEvent)
	}
	return result, nil
}

func (o *Orchestrator) EnrichContext(ctx context.Context, info system.ContextInfo, input string) system.ContextInfo {
	info.ContextWindow = o.contextWindowFn()
	if cfg := o.configFn(); cfg != nil {
		info.ThinkingVerbosity = cfg.ThinkingVerbosity
		ctx = tools.WithConfig(ctx, cfg)
	}

	br := tools.GetBrain(ctx)
	if br == nil {
		br = o.brainFn()
	}

	if br != nil {
		var sb strings.Builder
		sb.WriteString(br.Summary())
		if br.Exists("plan") {
			if plan := br.PlanSummary(); plan != "" {
				sb.WriteString("\n\n[plan.md]:\n")
				sb.WriteString(plan)
			}
		}
		if br.Exists("tasks") {
			if tasks := br.TasksSummary(); tasks != "" {
				sb.WriteString("\n\n[tasks.md]:\n")
				sb.WriteString(tasks)
			}
		}
		info.BrainSummary = sb.String()
	}

	skillList := o.AvailableSkills()
	if len(skillList) > 0 {
		info.AvailableSkills = make([]system.SkillDef, len(skillList))
		for i, s := range skillList {
			info.AvailableSkills[i] = system.SkillDef{
				Name:        s.Name,
				Description: s.Description,
			}
		}
	}

	agentList := o.AvailableAgents()
	if len(agentList) > 0 {
		info.AvailableAgents = make([]string, len(agentList))
		for i, a := range agentList {
			info.AvailableAgents[i] = a.Name
		}
	}

	sem := o.semanticFn()
	if sem == nil {
		return info
	}

	var snapshot *semantic.Snapshot
	var err error

	if info.SemanticSummary == "" {
		snapshot, err = sem.Ensure(ctx)
		if err != nil {
			if info.Signals == nil {
				info.Signals = make(map[string]string)
			}
			info.Signals["semantic_error"] = err.Error()
			return info
		}
		info.SemanticSummary = snapshot.Summary()
	} else {
		snapshot = sem.LatestSnapshot()
	}

	if snapshot == nil {
		return info
	}

	limits := system.GetSemanticLimits(o.contextWindowFn())

	relevant := snapshot.RelevantFiles(input, limits.NumFiles)
	info.RelevantFiles = make([]string, 0, len(relevant))
	for _, file := range relevant {
		info.RelevantFiles = append(info.RelevantFiles, file.Descriptor())
	}
	snippets := snapshot.RelevantSnippets(input, limits.NumSnippets, limits.SnippetLines)
	info.RelevantSnippets = make([]string, 0, len(snippets))
	for _, snippet := range snippets {
		info.RelevantSnippets = append(info.RelevantSnippets, snippet.Descriptor())
	}
	if info.Signals == nil {
		info.Signals = make(map[string]string)
	}
	info.Signals["semantic"] = snapshot.Summary()
	for _, path := range o.mentionedFiles {
		for _, file := range snapshot.Files {
			if file.Path != path {
				continue
			}
			info.RelevantFiles = append([]string{file.Descriptor()}, info.RelevantFiles...)
			content := string(file.Content)
			lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
			limit := min(limits.ExplicitLimit, len(lines))
			var numbered []string
			for i := 0; i < limit; i++ {
				numbered = append(numbered, fmt.Sprintf("%d: %s", i+1, lines[i]))
			}
			info.RelevantSnippets = append([]string{fmt.Sprintf("FILE %s\nLINES 1-%d\nREASON explicit @ mention\n%s", file.Path, limit, strings.Join(numbered, "\n"))}, info.RelevantSnippets...)
			break
		}
	}
	tracelog.Logf("runtime context semantic=%q relevant_files=%d relevant_snippets=%d", info.SemanticSummary, len(info.RelevantFiles), len(info.RelevantSnippets))

	if info.Path != "" {
		agentsPath := filepath.Join(info.Path, "AGENTS.md")
		if data, err := os.ReadFile(agentsPath); err == nil && len(data) > 0 {
			info.Guidelines = string(data)
		}

		designPath := filepath.Join(info.Path, "DESIGN.md")
		if data, err := os.ReadFile(designPath); err == nil && len(data) > 0 {
			info.DesignSpec = string(data)
		}
		info.ActiveMode = o.currentAgentName
	}

	return info
}
