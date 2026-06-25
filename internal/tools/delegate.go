package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type SubagentConfig struct {
	ProgressChan  chan<- string   `json:"-"`
	Mode          string          `json:"mode"`
	Task          string          `json:"task"`
	ToolFilter    []string        `json:"tool_filter"`
	MaxIterations int             `json:"max_iterations"`
	MaxDepth      int             `json:"max_depth"`
	AllowDelegate bool            `json:"allow_delegate"`
	InheritBrain  bool            `json:"inherit_brain"`
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
		Name:    "delegate",
		Summary: "Delegar una sub-tarea a otro agente en segundo plano. Los agentes disponibles son: plan, search.",
		Usage:   "delegate <nombre_agente>: <instruccion> [|| <json_config>]",
	}
}

func (t *DelegateTool) DynamicSpec(ctx ToolContext) Spec {
	spec := t.Spec()
	if len(ctx.AvailableAgents) > 0 {
		spec.Summary = fmt.Sprintf("Delegar una sub-tarea a otro agente en segundo plano. Agentes disponibles: %s. Uso: delegate <nombre_agente>: <instruccion> [|| {\"allow_delegate\": true, \"inherit_brain\": true, \"max_iterations\": 10}]", strings.Join(ctx.AvailableAgents, ", "))
	}
	return spec
}

func (t *DelegateTool) Run(ctx context.Context, args string) (Result, error) {
	if t.runner == nil {
		return Result{}, fmt.Errorf("agent runner no inicializado")
	}

	parts := strings.SplitN(args, "||", 2)
	mainArgs := strings.TrimSpace(parts[0])

	argsParts := strings.SplitN(mainArgs, ":", 2)
	if len(argsParts) < 2 {
		return Result{}, fmt.Errorf("uso: %s", t.Spec().Usage)
	}

	agentName := strings.TrimSpace(argsParts[0])
	instruction := strings.TrimSpace(argsParts[1])

	if agentName == "" || instruction == "" {
		return Result{}, fmt.Errorf("uso: %s", t.Spec().Usage)
	}

	cfg := SubagentConfig{
		Mode:          agentName,
		Task:          instruction,
		MaxIterations: 10,
		MaxDepth:      2,
		InheritBrain:  true,
	}

	if len(parts) == 2 {
		jsonStr := strings.TrimSpace(parts[1])
		if jsonStr != "" {
			if err := json.Unmarshal([]byte(jsonStr), &cfg); err != nil {
				return Result{}, fmt.Errorf("error parseando json config: %v", err)
			}
			cfg.Mode = agentName // force override
			cfg.Task = instruction // force override
		}
	}

	resultText, err := t.runner.RunSubagent(ctx, cfg)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Spec:    t.Spec(),
		Summary: fmt.Sprintf("Subagente %s finalizo su tarea.", agentName),
		Output:  resultText,
	}, nil
}
