package tools

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

type stubAgentRunner struct {
	cfg SubagentConfig
	err error
}

func (s *stubAgentRunner) RunSubagent(ctx context.Context, cfg SubagentConfig) (string, error) {
	s.cfg = cfg
	if s.err != nil {
		return "", s.err
	}
	return "sub-agent output", nil
}

func newDelegateTestTool() (*DelegateTool, *stubAgentRunner) {
	runner := &stubAgentRunner{}
	return NewDelegateTool(runner), runner
}

func TestDelegateToolRunsFromNativeJSONArgs(t *testing.T) {
	tool, runner := newDelegateTestTool()

	res, err := tool.Run(context.Background(), `{"agent":"plan","instruction":"Investigate the flaky test"}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if runner.cfg.Mode != "plan" {
		t.Fatalf("expected mode plan, got %q", runner.cfg.Mode)
	}
	if runner.cfg.Task != "Investigate the flaky test" {
		t.Fatalf("expected task from instruction, got %q", runner.cfg.Task)
	}
	if runner.cfg.MaxIterations != 10 || runner.cfg.MaxDepth != 2 || !runner.cfg.InheritBrain {
		t.Fatalf("expected default config, got %#v", runner.cfg)
	}
	if res.Output != "sub-agent output" {
		t.Fatalf("unexpected output %q", res.Output)
	}
	if !strings.Contains(res.Summary, "plan") {
		t.Fatalf("expected summary mentioning the agent, got %q", res.Summary)
	}
}

func TestDelegateToolJSONConfigOverridesDefaults(t *testing.T) {
	tool, runner := newDelegateTestTool()

	args := `{"agent":"search","instruction":"Find usages","max_iterations":25,"max_depth":3,"allow_delegate":true,"inherit_brain":false,"tool_filter":["read","grep"]}`
	if _, err := tool.Run(context.Background(), args); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if runner.cfg.Mode != "search" || runner.cfg.Task != "Find usages" {
		t.Fatalf("unexpected mode/task %#v", runner.cfg)
	}
	if runner.cfg.MaxIterations != 25 || runner.cfg.MaxDepth != 3 {
		t.Fatalf("expected overridden limits, got %#v", runner.cfg)
	}
	if !runner.cfg.AllowDelegate || runner.cfg.InheritBrain {
		t.Fatalf("expected overridden booleans, got %#v", runner.cfg)
	}
	if !reflect.DeepEqual(runner.cfg.ToolFilter, []string{"read", "grep"}) {
		t.Fatalf("expected tool filter parsed, got %#v", runner.cfg.ToolFilter)
	}
}

func TestDelegateToolRejectsMissingFields(t *testing.T) {
	tool, _ := newDelegateTestTool()

	cases := []string{
		"",
		"plan",
		`{"agent":"plan"}`,
		`{"instruction":"no agent"}`,
		`{"agent":"","instruction":"empty agent"}`,
		"not json at all",
	}
	for _, args := range cases {
		if _, err := tool.Run(context.Background(), args); err == nil {
			t.Errorf("expected error for args %q", args)
		}
	}
}

func TestDelegateToolPropagatesRunnerError(t *testing.T) {
	tool, runner := newDelegateTestTool()
	runner.err = context.DeadlineExceeded

	if _, err := tool.Run(context.Background(), `{"agent":"plan","instruction":"x"}`); err == nil {
		t.Fatal("expected runner error to propagate")
	}
}
