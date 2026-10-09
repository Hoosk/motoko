package tools

import (
	"context"
	"fmt"
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
		Name:        "task",
		Summary:     "Launch a long-running command in the background (returns ID) or cancel a running task.",
		Usage:       `task {"command": "go test ./..."} | task {"terminate": "<task_id>"}`,
		InputSchema: schemaTask,
	}
}

func (t *TaskTool) Run(ctx context.Context, args string) (Result, error) {
	if t.runner == nil {
		return Result{}, fmt.Errorf("task runner not initialized")
	}
	parsed := parseJSONArgs(args)
	if parsed == nil {
		return Result{}, fmt.Errorf("usage: %s", t.Spec().Usage)
	}

	if command := jsonStr(parsed, "command", "cmd"); command != "" {
		id, err := t.runner.StartTask(ctx, command)
		if err != nil {
			return Result{}, err
		}
		return Result{
			Spec:    t.Spec(),
			Summary: fmt.Sprintf("Task %s launched.", id),
			Output:  command,
		}, nil
	}

	if id := jsonStr(parsed, "terminate", "task_id", "taskId", "id"); id != "" {
		if err := t.runner.TerminateTask(id); err != nil {
			return Result{}, err
		}
		return Result{
			Spec:    t.Spec(),
			Summary: fmt.Sprintf("Task %s terminated.", id),
			Output:  fmt.Sprintf("Terminated task %s", id),
		}, nil
	}

	return Result{}, fmt.Errorf("usage: %s", t.Spec().Usage)
}
