package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/brain"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/semantic"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tools"
	"github.com/Hoosk/motoko/internal/tracelog"
)

func (r *Runtime) RunAgent(ctx context.Context, info system.ContextInfo, input string) (agent.Result, error) {
	if r.agent == nil || !r.agent.Configured() {
		return agent.Result{}, fmt.Errorf("agente no configurado")
	}
	if info.Path == "" {
		info = r.GetContextInfo()
	}
	info = r.enrichContext(ctx, info, input)
	priorHistory := []provider.ConversationItem(nil)
	if r.currentSession != nil {
		priorHistory = append(priorHistory, r.currentSession.History...)
		reqID := fmt.Sprintf("req-%d", time.Now().UnixNano())
		ctx = provider.WithTelemetry(ctx, r.currentSession.ID, reqID)
	}
	ctx = tools.WithBrain(ctx, r.brain)
	result, err := r.agent.Run(ctx, info, input, priorHistory)
	if err != nil {
		return result, err
	}
	r.persistTurn(result)
	if r.currentSession != nil && (strings.TrimSpace(r.currentSession.Title) == "" || strings.EqualFold(strings.TrimSpace(r.currentSession.Title), "New session")) {
		go r.generateTitle(context.Background(), input, result.Assistant)
	}
	return result, r.maybeAutoCompact(ctx, nil)
}

func (r *Runtime) RunAgentStream(ctx context.Context, info system.ContextInfo, input string, onEvent func(AgentStreamEvent) error) (agent.Result, error) {
	if r.agent == nil || !r.agent.Configured() {
		return agent.Result{}, fmt.Errorf("agente no configurado")
	}
	if info.Path == "" {
		info = r.GetContextInfo()
	}
	info = r.enrichContext(ctx, info, input)
	priorHistory := []provider.ConversationItem(nil)
	if r.currentSession != nil {
		priorHistory = append(priorHistory, r.currentSession.History...)
		reqID := fmt.Sprintf("req-%d", time.Now().UnixNano())
		ctx = provider.WithTelemetry(ctx, r.currentSession.ID, reqID)
	}
	ctx = tools.WithBrain(ctx, r.brain)
	result, err := r.agent.RunStream(ctx, info, input, priorHistory, func(event agent.StreamEvent) error {
		if onEvent == nil {
			return nil
		}
		return onEvent(AgentStreamEvent{
			Kind:             event.Kind,
			Title:            event.Title,
			Content:          event.Content,
			ReasoningContent: event.ReasoningContent,
		})
	})
	if err != nil {
		return result, err
	}
	r.persistTurn(result)
	if r.currentSession != nil && (strings.TrimSpace(r.currentSession.Title) == "" || strings.EqualFold(strings.TrimSpace(r.currentSession.Title), "New session")) {
		go r.generateTitle(context.Background(), input, result.Assistant)
	}
	if err := r.maybeAutoCompact(ctx, onEvent); err != nil {
		return result, err
	}
	return result, nil
}

func (r *Runtime) enrichContext(ctx context.Context, info system.ContextInfo, input string) system.ContextInfo {
	br := tools.GetBrain(ctx)
	if br == nil {
		br = r.brain
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

	if len(r.availableSkills) > 0 {
		info.AvailableSkills = make([]system.SkillDef, len(r.availableSkills))
		for i, s := range r.availableSkills {
			info.AvailableSkills[i] = system.SkillDef{
				Name:        s.Name,
				Description: s.Description,
			}
		}
	}

	if len(r.availableAgents) > 0 {
		info.AvailableAgents = make([]string, len(r.availableAgents))
		for i, a := range r.availableAgents {
			info.AvailableAgents[i] = a.Name
		}
	}

	if r.semantic == nil {
		return info
	}

	var snapshot *semantic.Snapshot
	var err error

	if info.SemanticSummary == "" {
		snapshot, err = r.semantic.Ensure(ctx)
		if err != nil {
			if info.Signals == nil {
				info.Signals = make(map[string]string)
			}
			info.Signals["semantic_error"] = err.Error()
			return info
		}
		info.SemanticSummary = snapshot.Summary()
	} else {
		snapshot = r.semantic.LatestSnapshot()
	}

	if snapshot == nil {
		return info
	}

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
		info.ActiveMode = r.currentAgentName
	}
	
	return info
}

func (r *Runtime) createSubagent(name string, cfg tools.SubagentConfig) (*agent.Agent, error) {
	active, ok := r.config.Active()
	if !ok {
		return nil, fmt.Errorf("no hay provider activo configurado")
	}

	var aDef agent.AgentDef
	found := false
	for _, a := range r.availableAgents {
		if strings.EqualFold(a.Name, name) {
			aDef = a
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("agente desconocido: %s", name)
	}

	override := r.config.Agents[aDef.Name]
	
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
		return nil, err
	}

	return r.buildAgentFromDef(client, aDef, override, hasOverride), nil
}

type subagentDepthKey struct{}

func (r *Runtime) RunSubagent(ctx context.Context, cfg tools.SubagentConfig) (string, error) {
	currentDepth := 0
	if v, ok := ctx.Value(subagentDepthKey{}).(int); ok {
		currentDepth = v
	}
	maxDepth := cfg.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 2 // default max depth
	}
	if currentDepth >= maxDepth {
		return "", fmt.Errorf("se alcanzó la profundidad máxima de delegación (%d)", maxDepth)
	}

	r.subagentsMu.Lock()
	if r.activeSubagents == nil {
		r.activeSubagents = make(map[string]*SubagentInfo)
	}
	id := fmt.Sprintf("%s-%d", cfg.Mode, len(r.activeSubagents)+1)
	r.activeSubagents[id] = &SubagentInfo{
		Name:      cfg.Mode,
		Prompt:    cfg.Task,
		StartedAt: time.Now(),
	}
	r.subagentsMu.Unlock()

	defer func() {
		r.subagentsMu.Lock()
		delete(r.activeSubagents, id)
		r.subagentsMu.Unlock()
	}()

	subSessionID := fmt.Sprintf("%s-%s", r.currentSession.ID, id)
	subBrain, err := brain.New(r.workspaceID, subSessionID)
	if err != nil {
		return "", err
	}

	if cfg.InheritBrain && r.brain != nil {
		if copyErr := r.brain.CopyTo(subBrain); copyErr != nil {
			return "", copyErr
		}
	}

	sub, err := r.createSubagent(cfg.Mode, cfg)
	if err != nil {
		return "", err
	}

	subCtx := tools.WithBrain(ctx, subBrain)
	subCtx = context.WithValue(subCtx, subagentDepthKey{}, currentDepth+1)
	reqID := fmt.Sprintf("subreq-%d", time.Now().UnixNano())
	subCtx = provider.WithTelemetry(subCtx, subSessionID, reqID)
	
	info := r.GetContextInfo()
	info = r.enrichContext(subCtx, info, cfg.Task)

	res, err := sub.RunStream(subCtx, info, cfg.Task, nil, func(event agent.StreamEvent) error {
		if event.Kind == "tool_start" || event.Kind == "content" {
			r.subagentsMu.Lock()
			if subInfo, ok := r.activeSubagents[id]; ok {
				if event.Title != "" {
					subInfo.Progress = event.Title
				}
			}
			r.subagentsMu.Unlock()
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
