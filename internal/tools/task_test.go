package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type mockTaskRunner struct {
	started    string
	terminated string
	err        error
}

func (m *mockTaskRunner) StartTask(ctx context.Context, command string) (string, error) {
	m.started = command
	if m.err != nil {
		return "", m.err
	}
	return "task-123", nil
}

func (m *mockTaskRunner) TerminateTask(id string) error {
	m.terminated = id
	return m.err
}

func TestTaskTool_Run(t *testing.T) {
	t.Run("LaunchTask", func(t *testing.T) {
		runner := &mockTaskRunner{}
		tool := NewTaskTool(runner)
		res, err := tool.Run(context.Background(), "go build ./...")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.started != "go build ./..." {
			t.Errorf("expected started command to be 'go build ./...', got %q", runner.started)
		}
		if !strings.Contains(res.Summary, "task-123") {
			t.Errorf("expected summary to contain task ID, got %q", res.Summary)
		}
	})

	t.Run("TerminateTask", func(t *testing.T) {
		runner := &mockTaskRunner{}
		tool := NewTaskTool(runner)
		res, err := tool.Run(context.Background(), "terminate task-123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.terminated != "task-123" {
			t.Errorf("expected terminated task ID to be 'task-123', got %q", runner.terminated)
		}
		if !strings.Contains(res.Summary, "terminated") {
			t.Errorf("expected summary to contain 'terminated', got %q", res.Summary)
		}
	})

	t.Run("EmptyArgs", func(t *testing.T) {
		runner := &mockTaskRunner{}
		tool := NewTaskTool(runner)
		_, err := tool.Run(context.Background(), "")
		if err == nil || !strings.Contains(err.Error(), "uso") {
			t.Errorf("expected usage error, got %v", err)
		}
	})

	t.Run("RunnerError", func(t *testing.T) {
		runner := &mockTaskRunner{err: errors.New("runner failed")}
		tool := NewTaskTool(runner)
		_, err := tool.Run(context.Background(), "go test")
		if err == nil || err.Error() != "runner failed" {
			t.Errorf("expected 'runner failed' error, got %v", err)
		}
	})
}
