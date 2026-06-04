package tools

import (
	"context"
	"fmt"
	"strings"
)

type TaskRunner interface {
	StartTask(ctx context.Context, command string) (string, error)
	TerminateTask(id string) error
}

type TaskTool struct {
	runner TaskRunner
}

func NewTaskTool(runner TaskRunner) *TaskTool {
	return &TaskTool{runner: runner}
}

func (t *TaskTool) Spec() Spec {
	return Spec{
		Name:    "task",
		Summary: "Launch a long-running command in the background (returns ID) or cancel a running task.",
		Usage:   "task <command> | task terminate <idTask>",
	}
}

func (t *TaskTool) Run(ctx context.Context, args string) (Result, error) {
	if t.runner == nil {
		return Result{}, fmt.Errorf("task runner no inicializado")
	}
	args = strings.TrimSpace(args)
	if args == "" {
		return Result{}, fmt.Errorf("uso: %s", t.Spec().Usage)
	}

	if strings.HasPrefix(args, "terminate ") {
		id := strings.TrimSpace(strings.TrimPrefix(args, "terminate "))
		if err := t.runner.TerminateTask(id); err != nil {
			return Result{}, err
		}
		return Result{
			Spec:    t.Spec(),
			Summary: fmt.Sprintf("Task %s terminated.", id),
			Output:  fmt.Sprintf("Terminated task %s", id),
		}, nil
	}

	id, err := t.runner.StartTask(ctx, args)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Spec:    t.Spec(),
		Summary: fmt.Sprintf("Task %s launched.", id),
		Output:  args,
	}, nil
}
