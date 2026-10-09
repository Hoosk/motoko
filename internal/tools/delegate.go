package tools

import (
	"context"
	"fmt"
	"strings"
)

const delegateUsage = `delegate {"agent": "plan", "instruction": "..."}`

type SubagentConfig struct {
	ProgressChan  chan<- string `json:"-"`
	Mode          string        `json:"mode"`
	Task          string        `json:"task"`
	ToolFilter    []string      `json:"tool_filter"`
	MaxIterations int           `json:"max_iterations"`
	MaxDepth      int           `json:"max_depth"`
	AllowDelegate bool          `json:"allow_delegate"`
	InheritBrain  bool          `json:"inherit_brain"`
}

type AgentRunner interface {
	RunSubagent(ctx context.Context, cfg SubagentConfig) (string, error)
}

type DelegateTool struct {
	runner AgentRunner
}

func NewDelegateTool(runner AgentRunner) *DelegateTool {
	return &DelegateTool{runner: runner}
}

func (t *DelegateTool) Spec() Spec {
	return Spec{
		Name:        "delegate",
		Summary:     "Delegate a sub-task to another agent in the background. Available agents: plan, search.",
		Usage:       delegateUsage,
		InputSchema: schemaDelegate,
	}
}

func (t *DelegateTool) DynamicSpec(ctx ToolContext) Spec {
	spec := t.Spec()
	if len(ctx.AvailableAgents) > 0 {
		spec.Summary = fmt.Sprintf("Delegate a sub-task to another agent in the background. Available agents: %s. Usage: %s", strings.Join(ctx.AvailableAgents, ", "), delegateUsage)
	}
	return spec
}

func (t *DelegateTool) Run(ctx context.Context, args string) (Result, error) {
	if t.runner == nil {
		return Result{}, fmt.Errorf("agent runner not initialized")
	}

	cfg, err := parseDelegateArgs(args)
	if err != nil {
		return Result{}, err
	}

	resultText, err := t.runner.RunSubagent(ctx, cfg)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Spec:    t.Spec(),
		Summary: fmt.Sprintf("Sub-agent %s finished its task.", cfg.Mode),
		Output:  resultText,
	}, nil
}

func newSubagentConfig() SubagentConfig {
	return SubagentConfig{
		MaxIterations: 10,
		MaxDepth:      2,
		InheritBrain:  true,
	}
}

// parseDelegateArgs parses the native JSON payload declared in the tool's
// input schema ({"agent": ..., "instruction": ...} plus optional knobs).
func parseDelegateArgs(args string) (SubagentConfig, error) {
	cfg := newSubagentConfig()
	parsed := parseJSONArgs(args)
	if parsed == nil {
		return cfg, fmt.Errorf("usage: %s", delegateUsage)
	}
	agentName := jsonStr(parsed, "agent", "agent_name", "mode", "name")
	instruction := jsonStr(parsed, "instruction", "task", "prompt")
	if agentName == "" || instruction == "" {
		return cfg, fmt.Errorf("usage: %s", delegateUsage)
	}
	cfg.Mode = agentName
	cfg.Task = instruction
	applyDelegateExtras(&cfg, parsed)
	return cfg, nil
}

// applyDelegateExtras folds optional knobs from a native JSON payload into
// the config, keeping the defaults for every omitted field.
func applyDelegateExtras(cfg *SubagentConfig, parsed map[string]any) {
	if v, ok := jsonInt(parsed, "max_iterations"); ok && v > 0 {
		cfg.MaxIterations = v
	}
	if v, ok := jsonInt(parsed, "max_depth"); ok && v > 0 {
		cfg.MaxDepth = v
	}
	if v, ok := parsed["allow_delegate"].(bool); ok {
		cfg.AllowDelegate = v
	}
	if v, ok := parsed["inherit_brain"].(bool); ok {
		cfg.InheritBrain = v
	}
	if list, ok := jsonStringList(parsed, "tool_filter"); ok {
		cfg.ToolFilter = list
	}
}
