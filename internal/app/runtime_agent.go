package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/agent"
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
	if info.Path == "" {
		info = r.GetContextInfo()
	}
	info = r.enrichContext(ctx, info, input)
	priorHistory := []provider.ConversationItem(nil)
	firstTurn := r.currentSession == nil || len(r.currentSession.History) == 0
	if r.currentSession != nil {
		priorHistory = append(priorHistory, r.currentSession.History...)
	}
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
	if firstTurn {
		go r.generateTitle(context.Background(), input, result.Assistant)
	}
	if err := r.maybeAutoCompact(ctx, onEvent); err != nil {
		return result, err
	}
	return result, nil
}

func (r *Runtime) enrichContext(ctx context.Context, info system.ContextInfo, input string) system.ContextInfo {
	if len(r.availableSkills) > 0 {
		info.AvailableSkills = make([]system.SkillDef, len(r.availableSkills))
		for i, s := range r.availableSkills {
			info.AvailableSkills[i] = system.SkillDef{
				Name:        s.Name,
				Description: s.Description,
			}
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
	return info
}

func (r *Runtime) createSubagent(name string) (*agent.Agent, error) {
	active, ok := r.config.Active()
	if !ok {
		return nil, fmt.Errorf("no hay provider activo configurado")
	}
	client, err := r.providerClient(active)
	if err != nil {
		return nil, err
	}

	var toolsRegistry *tools.Registry
	if strings.EqualFold(name, "build") {
		toolsRegistry = r.tools
	} else {
		// Filter out write tools for plan, search, and other subagents
		toolsRegistry = r.tools.Filter(func(t tools.Tool) bool {
			return !tools.IsWriteTool(t.Spec().Name)
		})
	}

	sub := agent.New(client, toolsRegistry)
	sub.SetDebug(r.debug)

	found := false
	for _, a := range r.availableAgents {
		if strings.EqualFold(a.Name, name) {
			sub.SetAgentOverride(a.System)
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("agente desconocido: %s", name)
	}

	return sub, nil
}

func (r *Runtime) RunSubagent(ctx context.Context, name, prompt string) (string, error) {
	r.subagentsMu.Lock()
	if r.activeSubagents == nil {
		r.activeSubagents = make(map[string]*SubagentInfo)
	}
	id := fmt.Sprintf("%s-%d", name, len(r.activeSubagents)+1)
	r.activeSubagents[id] = &SubagentInfo{
		Name:      name,
		Prompt:    prompt,
		StartedAt: time.Now(),
	}
	r.subagentsMu.Unlock()

	defer func() {
		r.subagentsMu.Lock()
		delete(r.activeSubagents, id)
		r.subagentsMu.Unlock()
	}()

	sub, err := r.createSubagent(name)
	if err != nil {
		return "", err
	}

	info := r.GetContextInfo()
	info = r.enrichContext(ctx, info, prompt)

	res, err := sub.Run(ctx, info, prompt, nil)
	if err != nil {
		return "", err
	}

	tracelog.Logf("subagent %s finished usage_in=%d usage_out=%d total=%d", name, res.Usage.InputTokens, res.Usage.OutputTokens, res.Usage.TotalTokens)
	return res.Assistant, nil
}
