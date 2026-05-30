package tools

import (
	"context"
	"fmt"
	"strings"
)

type AgentRunner interface {
	RunSubagent(ctx context.Context, name, prompt string) (string, error)
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
		Usage:   "delegate <nombre_agente>: <instruccion>",
	}
}

func (t *DelegateTool) Run(ctx context.Context, args string) (Result, error) {
	if t.runner == nil {
		return Result{}, fmt.Errorf("agent runner no inicializado")
	}

	parts := strings.SplitN(args, ":", 2)
	if len(parts) < 2 {
		return Result{}, fmt.Errorf("uso: %s", t.Spec().Usage)
	}

	agentName := strings.TrimSpace(parts[0])
	instruction := strings.TrimSpace(parts[1])

	if agentName == "" || instruction == "" {
		return Result{}, fmt.Errorf("uso: %s", t.Spec().Usage)
	}

	resultText, err := t.runner.RunSubagent(ctx, agentName, instruction)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Spec:    t.Spec(),
		Summary: fmt.Sprintf("Subagente %s finalizo su tarea.", agentName),
		Output:  resultText,
	}, nil
}
