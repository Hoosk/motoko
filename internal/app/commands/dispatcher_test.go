package commands

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/app/scheduleman"
	"github.com/Hoosk/motoko/internal/brain"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/mcp"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/session"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tools"

	"github.com/Hoosk/motoko/internal/app/providerman"
	"github.com/Hoosk/motoko/internal/app/taskman"
	"github.com/Hoosk/motoko/internal/app/types"
)

func baseDeps() Deps {
	return Deps{
		ConfigFn:     func() *config.AppConfig { return &config.AppConfig{} },
		SaveConfigFn: func() error { return nil },
		ThemeFn:      func() string { return "cyberpunk" },
		SetThemeFn:   func(name string) error { return nil },

		InputModeFn:    func() types.InputMode { return types.InputModeChat },
		SetInputModeFn: func(types.InputMode) {},

		ModeFn:            func() types.Mode { return types.ModePlan },
		SetAgentModeFn:    func(string) {},
		AgentNameFn:       func() string { return "plan" },
		AgentNamesFn:      func() []string { return []string{"plan", "build", "search"} },
		AgentConfiguredFn: func() bool { return true },
		DebugFn:           func() bool { return false },
		SetDebugFn:        func(bool) {},
		AgentFn:           func() *agent.Agent { return nil },
		SystemPromptFn:    func(system.ContextInfo) string { return "system prompt" },

		SessionFn:      func() *session.Session { return nil },
		SaveSessionFn:  func() error { return nil },
		BrainFn:        func() *brain.Brain { return nil },
		BrainInitErrFn: func() error { return nil },

		ListTasksFn:     func() []*taskman.TaskState { return nil },
		TerminateTaskFn: func(id string) error { return nil },
		ListSchedulesFn: func() []scheduleman.Definition { return nil },
		AddScheduleFn: func(instruction string, interval time.Duration, oneShot bool) (scheduleman.Definition, error) {
			return scheduleman.Definition{ID: "sched-1", Instruction: instruction, Interval: interval, OneShot: oneShot}, nil
		},
		RemoveScheduleFn: func(id string) error { return nil },

		ToolSpecsFn: func() []tools.Spec { return nil },
		RunToolFn:   func(ctx context.Context, name, args string) (tools.Result, error) { return tools.Result{}, nil },

		ProvMgr: providerman.NewManager(
			func() *config.AppConfig {
				return &config.AppConfig{ActiveProvider: "test"}
			},
			func() func(config.ProviderConfig) (provider.Client, error) {
				return func(config.ProviderConfig) (provider.Client, error) { return nil, nil }
			},
			func() {},
		),

		PendingDialogsFn: func() int { return 0 },

		ContextWindowFn: func() int { return 128_000 },
	}
}

func newDispatcher(d Deps) *Dispatcher {
	return New(d)
}

func TestHandleHelp(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("/help", system.ContextInfo{})
	if len(resp.Entries) == 0 || resp.Entries[0].Kind != types.EntryHelp {
		t.Fatal("expected help entry")
	}
	if !strings.Contains(resp.Entries[0].Text, "/help") {
		t.Error("expected help text to mention /help")
	}
	for _, want := range []string{"/models [list|use <model>|info <model>]", "/task [list|terminate <id>]", "/brain [list|read <file>|plan|tasks|summary|clear]", "/settings"} {
		if !strings.Contains(resp.Entries[0].Text, want) {
			t.Fatalf("expected help text to contain %q, got:\n%s", want, resp.Entries[0].Text)
		}
	}
}

func TestHandleExit(t *testing.T) {
	d := newDispatcher(baseDeps())
	for _, cmd := range []string{"/exit", "/quit"} {
		resp := d.Handle(cmd, system.ContextInfo{})
		if resp.Signal != "quit" {
			t.Errorf("expected Signal 'quit' for %q, got %q", cmd, resp.Signal)
		}
	}
}

func TestHandleThemesList(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("/themes", system.ContextInfo{})
	if len(resp.Entries) == 0 || resp.Entries[0].Kind != types.EntrySystem {
		t.Fatal("expected system entry for /themes")
	}
	if !strings.Contains(resp.Entries[0].Text, "cyberpunk") {
		t.Error("expected current theme in output")
	}
}

func TestHandleThemesSwitch(t *testing.T) {
	setCalled := ""
	deps := baseDeps()
	deps.SetThemeFn = func(name string) error {
		setCalled = name
		return nil
	}
	d := newDispatcher(deps)
	resp := d.Handle("/themes nord", system.ContextInfo{})
	if setCalled != "nord" {
		t.Errorf("expected SetThemeFn called with 'nord', got %q", setCalled)
	}
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "nord") {
		t.Error("expected theme changed confirmation")
	}
}

func TestHandleThemesUnknown(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("/themes invalid", system.ContextInfo{})
	if len(resp.Entries) == 0 || resp.Entries[0].Kind != types.EntryError {
		t.Fatal("expected error for unknown theme")
	}
}

func TestHandleClear(t *testing.T) {
	saved := false
	deps := baseDeps()
	deps.SessionFn = func() *session.Session {
		return &session.Session{}
	}
	deps.SaveSessionFn = func() error { saved = true; return nil }
	d := newDispatcher(deps)
	resp := d.Handle("/clear", system.ContextInfo{})
	if !resp.Clear {
		t.Error("expected Clear=true")
	}
	if !saved {
		t.Error("expected SaveSessionFn to be called")
	}
}

func TestHandleCompact(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("/compact", system.ContextInfo{})
	if resp.Action == nil || resp.Action.Type != types.ActionCompact {
		t.Error("expected ActionCompact")
	}
}

func TestHandlePlanAndBuild(t *testing.T) {
	modeSet := ""
	deps := baseDeps()
	deps.SetAgentModeFn = func(name string) { modeSet = name }
	d := newDispatcher(deps)

	resp := d.Handle("/plan", system.ContextInfo{})
	if modeSet != "plan" {
		t.Errorf("expected mode 'plan', got %q", modeSet)
	}
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "plan") {
		t.Error("expected plan confirmation")
	}

	modeSet = ""
	resp = d.Handle("/build", system.ContextInfo{})
	if modeSet != "build" {
		t.Errorf("expected mode 'build', got %q", modeSet)
	}
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "build") {
		t.Error("expected build confirmation")
	}
}

func TestHandleAgentShow(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("/agent", system.ContextInfo{})
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "plan") {
		t.Error("expected agent info with current agent")
	}
}

func TestHandleAgentSwitch(t *testing.T) {
	switched := ""
	currentName := "plan"
	deps := baseDeps()
	deps.AgentNameFn = func() string { return currentName }
	deps.SetAgentModeFn = func(name string) { switched = name; currentName = name }
	d := newDispatcher(deps)

	resp := d.Handle("/agent build", system.ContextInfo{})
	if switched != "build" {
		t.Errorf("expected switched to 'build', got %q", switched)
	}
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "build") {
		t.Errorf("expected switch confirmation, got: %q", resp.Entries[0].Text)
	}
}

func TestHandleAgentUnknown(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("/agent unknown", system.ContextInfo{})
	if len(resp.Entries) == 0 || resp.Entries[0].Kind != types.EntryError {
		t.Fatal("expected error for unknown agent")
	}
}

func TestHandleModeSignal(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("/mode", system.ContextInfo{})
	if resp.Signal != "open-mode-popup" {
		t.Errorf("expected 'open-mode-popup', got %q", resp.Signal)
	}
}

func TestHandleShellAndChat(t *testing.T) {
	mode := types.InputModeChat
	deps := baseDeps()
	deps.SetInputModeFn = func(m types.InputMode) { mode = m }
	d := newDispatcher(deps)

	resp := d.Handle("/shell", system.ContextInfo{})
	if mode != types.InputModeShell {
		t.Errorf("expected InputModeShell, got %q", mode)
	}
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "shell") {
		t.Error("expected shell mode confirmation")
	}

	resp = d.Handle("/chat", system.ContextInfo{})
	if mode != types.InputModeChat {
		t.Errorf("expected InputModeChat, got %q", mode)
	}
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "chat") {
		t.Error("expected chat mode confirmation")
	}
}

func TestHandleStatus(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("/status", system.ContextInfo{Workspace: "test-workspace"})
	if len(resp.Entries) == 0 || resp.Entries[0].Kind != types.EntrySystem {
		t.Fatal("expected system entry for /status")
	}
	text := resp.Entries[0].Text
	for _, want := range []string{"mode:", "input:", "workspace:", "pending approval:"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %q in status, got: %s", want, text)
		}
	}
}

func TestHandleDebug(t *testing.T) {
	debugState := false
	deps := baseDeps()
	deps.DebugFn = func() bool { return debugState }
	deps.SetDebugFn = func(d bool) { debugState = d }
	d := newDispatcher(deps)

	resp := d.Handle("/debug", system.ContextInfo{})
	if !debugState {
		t.Error("expected debug enabled")
	}
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "true") {
		t.Error("expected debug true confirmation")
	}

	resp = d.Handle("/debug", system.ContextInfo{})
	if debugState {
		t.Error("expected debug disabled")
	}
}

func TestHandleContext(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("/context", system.ContextInfo{})
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "system prompt") {
		t.Error("expected system prompt in /context output")
	}
}

func TestHandleTools(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("/tools", system.ContextInfo{})
	if len(resp.Entries) == 0 || resp.Entries[0].Kind != types.EntrySystem {
		t.Fatal("expected system entry for /tools")
	}
}

func TestHandleToolMissingName(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("/tool", system.ContextInfo{})
	if len(resp.Entries) == 0 || resp.Entries[0].Kind != types.EntryError {
		t.Fatal("expected error for missing tool name")
	}
}

func TestHandleWriteToolUsesAsyncActionWhenApprovalIsRequired(t *testing.T) {
	deps := baseDeps()
	deps.ConfigFn = func() *config.AppConfig {
		return &config.AppConfig{EditApproval: config.EditApprovalAsk}
	}
	d := newDispatcher(deps)

	resp := d.Handle("/tool write file.txt\ncontent", system.ContextInfo{})
	if resp.Action == nil || resp.Action.Type != types.ActionTool {
		t.Fatalf("expected tool action, got %#v", resp)
	}
	if resp.Action.ToolName != "write" || resp.Action.ToolArgs != "file.txt\ncontent" {
		t.Fatalf("unexpected tool action %#v", resp.Action)
	}
}

func TestHandleShellApprovalReturnsDialogAction(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("/tool bash git add file.go", system.ContextInfo{})
	if resp.Action == nil || resp.Action.Type != types.ActionShellApproval {
		t.Fatalf("expected shell approval action, got %#v", resp.Action)
	}
	if resp.Action.ShellCommand != "git add file.go" || resp.Action.ShellReason == "" {
		t.Fatalf("unexpected shell approval action %#v", resp.Action)
	}
}

func TestHandleTaskListEmpty(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("/task", system.ContextInfo{})
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "No active") {
		t.Error("expected 'no active tasks' message")
	}
}

func TestHandleTaskListWithTasks(t *testing.T) {
	deps := baseDeps()
	deps.ListTasksFn = func() []*taskman.TaskState {
		return []*taskman.TaskState{
			{ID: "task-1", Command: "go build", Started: time.Now()},
		}
	}
	d := newDispatcher(deps)
	resp := d.Handle("/task", system.ContextInfo{})
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "task-1") {
		t.Error("expected task-1 in output")
	}
}

func TestHandleTaskTerminate(t *testing.T) {
	terminated := ""
	deps := baseDeps()
	deps.TerminateTaskFn = func(id string) error { terminated = id; return nil }
	d := newDispatcher(deps)

	resp := d.Handle("/task terminate mytask", system.ContextInfo{})
	if terminated != "mytask" {
		t.Errorf("expected TerminateTaskFn with 'mytask', got %q", terminated)
	}
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "terminated") {
		t.Error("expected terminated message")
	}
}

func TestHandleTaskTerminateMissingID(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("/task terminate", system.ContextInfo{})
	if len(resp.Entries) == 0 || resp.Entries[0].Kind != types.EntryError {
		t.Fatal("expected error for missing task ID")
	}
}

func TestHandleBrainNoBrain(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("/brain", system.ContextInfo{})
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "not initialized") {
		t.Error("expected 'not initialized' for missing brain")
	}
}

func TestHandleMetricsNoSession(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("/metrics", system.ContextInfo{})
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "No active session") {
		t.Error("expected 'no active session' message")
	}
}

func TestHandleMetricsWithSession(t *testing.T) {
	deps := baseDeps()
	deps.SessionFn = func() *session.Session {
		return &session.Session{
			LastInputTokens:       200,
			LastOutputTokens:      80,
			LastReasoningTokens:   20,
			LastCacheReadTokens:   15,
			LastCacheWriteTokens:  5,
			TotalInputTokens:      1000,
			TotalOutputTokens:     500,
			TotalReasoningTokens:  120,
			TotalCacheReadTokens:  60,
			TotalCacheWriteTokens: 20,
			TotalTokens:           1500,
			Turns: []session.TurnUsage{{
				Turn:            1,
				InputTokens:     200,
				OutputTokens:    80,
				ReasoningTokens: 20,
				TotalTokens:     280,
				InputGrowth:     40,
				Iterations:      []provider.Usage{{InputTokens: 160, OutputTokens: 30, TotalTokens: 190, ReasoningTokens: 10}, {InputTokens: 200, OutputTokens: 50, TotalTokens: 250, ReasoningTokens: 10}},
			}},
		}
	}
	d := newDispatcher(deps)
	resp := d.Handle("/metrics", system.ContextInfo{})
	if len(resp.Entries) == 0 {
		t.Fatal("expected metrics output")
	}
	text := resp.Entries[0].Text
	for _, want := range []string{"Current Session Metrics", "Input Tokens", "Output Tokens", "Total Tokens", "Reasoning (Thinking) Tokens", "Recent Turn Trend", "iter 1:"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %q in metrics, got: %s", want, text)
		}
	}
}

func TestHandleSessionsSignal(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("/sessions", system.ContextInfo{})
	if resp.Signal != "open-sessions-popup" {
		t.Errorf("expected 'open-sessions-popup', got %q", resp.Signal)
	}
}

func TestHandleSettingsSignal(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("/settings", system.ContextInfo{})
	if resp.Signal != "open-settings-popup" {
		t.Errorf("expected 'open-settings-popup', got %q", resp.Signal)
	}
}

func TestHandleLearnStartsAgentAction(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("/learn", system.ContextInfo{})
	if resp.Action == nil || resp.Action.Type != types.ActionAgent {
		t.Fatalf("expected agent action, got %#v", resp.Action)
	}
	if !strings.Contains(strings.ToLower(resp.Action.AgentPrompt), "capture reusable project knowledge") {
		t.Fatalf("expected learn prompt to mention knowledge capture, got %q", resp.Action.AgentPrompt)
	}
}

func TestHandleTeamworkPreviewStartsAgentAction(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("/teamwork-preview auth refactor", system.ContextInfo{})
	if resp.Action == nil || resp.Action.Type != types.ActionAgent {
		t.Fatalf("expected agent action, got %#v", resp.Action)
	}
	if !strings.Contains(resp.Action.AgentPrompt, "auth refactor") {
		t.Fatalf("expected teamwork prompt to include goal, got %q", resp.Action.AgentPrompt)
	}
}

func TestHandleGrillMeStartsAgentAction(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("/grill-me", system.ContextInfo{})
	if resp.Action == nil || resp.Action.Type != types.ActionAgent {
		t.Fatalf("expected agent action, got %#v", resp.Action)
	}
	if !strings.Contains(strings.ToLower(resp.Action.AgentPrompt), "current plan") {
		t.Fatalf("expected grill-me prompt to mention the current plan, got %q", resp.Action.AgentPrompt)
	}
}

func TestHandleGoalStoresGoalAndStartsAgent(t *testing.T) {
	br := &brain.Brain{Dir: t.TempDir(), SessionID: "test"}
	deps := baseDeps()
	deps.BrainFn = func() *brain.Brain { return br }
	d := newDispatcher(deps)

	resp := d.Handle("/goal finish login flow", system.ContextInfo{})
	if resp.Action == nil || resp.Action.Type != types.ActionAgent {
		t.Fatalf("expected agent action, got %#v", resp.Action)
	}
	content, err := br.Read("goal")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "finish login flow") {
		t.Fatalf("expected goal file content, got %q", content)
	}
}

func TestHandleGoalPlanRequiresTasks(t *testing.T) {
	br := &brain.Brain{Dir: t.TempDir(), SessionID: "test"}
	deps := baseDeps()
	deps.BrainFn = func() *brain.Brain { return br }
	d := newDispatcher(deps)

	resp := d.Handle("/goal plan", system.ContextInfo{})
	if len(resp.Entries) == 0 || resp.Entries[0].Kind != types.EntryError {
		t.Fatalf("expected error response, got %#v", resp)
	}
}

func TestHandleGoalStatusReportsCounts(t *testing.T) {
	br := &brain.Brain{Dir: t.TempDir(), SessionID: "test"}
	if err := br.Write("goal", "# Goal\nFinish tasks"); err != nil {
		t.Fatal(err)
	}
	if err := br.Write("tasks", "# Tasks\n- [ ] one\n- [x] two"); err != nil {
		t.Fatal(err)
	}
	deps := baseDeps()
	deps.BrainFn = func() *brain.Brain { return br }
	d := newDispatcher(deps)

	resp := d.Handle("/goal status", system.ContextInfo{})
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "1 pending, 1 completed") {
		t.Fatalf("unexpected goal status: %#v", resp)
	}
}

func TestHandleScheduleAddListRemove(t *testing.T) {
	removed := ""
	deps := baseDeps()
	defs := []scheduleman.Definition{{ID: "sched-1", Instruction: "run tests", Interval: time.Minute}}
	deps.ListSchedulesFn = func() []scheduleman.Definition { return defs }
	deps.RemoveScheduleFn = func(id string) error { removed = id; defs = nil; return nil }
	d := newDispatcher(deps)

	resp := d.Handle(`/schedule add "run tests" every 1m`, system.ContextInfo{})
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "sched-1") {
		t.Fatalf("unexpected add response %#v", resp)
	}

	resp = d.Handle(`/schedule list`, system.ContextInfo{})
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "run tests") {
		t.Fatalf("unexpected list response %#v", resp)
	}

	resp = d.Handle(`/schedule remove sched-1`, system.ContextInfo{})
	if removed != "sched-1" {
		t.Fatalf("expected remove called with sched-1, got %q", removed)
	}
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "removed") {
		t.Fatalf("unexpected remove response %#v", resp)
	}
}

func TestHandleUnknownCommand(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("/nonexistent", system.ContextInfo{})
	if len(resp.Entries) == 0 || resp.Entries[0].Kind != types.EntryError {
		t.Fatal("expected error for unknown command")
	}
}

func TestHandleEmpty(t *testing.T) {
	d := newDispatcher(baseDeps())
	resp := d.Handle("", system.ContextInfo{})
	if len(resp.Entries) != 0 {
		t.Errorf("expected empty response, got %v", resp.Entries)
	}
}

func TestHandleMCPCommand(t *testing.T) {
	deps := baseDeps()
	deps.MCPServersFn = func() []mcp.ServerStatus {
		return []mcp.ServerStatus{
			{
				Name:      "alpha",
				Transport: "stdio",
				Connected: true,
				ToolCount: 2,
				Tools:     []string{"mcp_alpha_read", "mcp_alpha_write"},
			},
			{
				Name:      "beta",
				Transport: "http",
				Connected: false,
				ToolCount: 0,
			},
		}
	}
	deps.ToolSpecsFn = func() []tools.Spec {
		return []tools.Spec{
			{Name: "read", Summary: "read file"},
			{Name: "mcp_alpha_read", Summary: "mcp alpha read"},
		}
	}
	d := newDispatcher(deps)

	resp := d.Handle("/mcp", system.ContextInfo{})
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "alpha") {
		t.Fatalf("unexpected /mcp response: %#v", resp)
	}

	resp = d.Handle("/mcp tools", system.ContextInfo{})
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "mcp_alpha_read") {
		t.Fatalf("unexpected /mcp tools response: %#v", resp)
	}

	resp = d.Handle("/mcp info alpha", system.ContextInfo{})
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "Transport: stdio") {
		t.Fatalf("unexpected /mcp info response: %#v", resp)
	}

	resp = d.Handle("/mcp info nonexistent", system.ContextInfo{})
	if len(resp.Entries) == 0 || resp.Entries[0].Kind != types.EntryError {
		t.Fatalf("expected error for nonexistent server info: %#v", resp)
	}

	addedName := ""
	deps.AddMCPServerFn = func(srv config.MCPServerConfig) {
		addedName = srv.Name
	}
	d = newDispatcher(deps)
	resp = d.Handle("/mcp add test-srv npx -y server-test", system.ContextInfo{})
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "added") {
		t.Fatalf("unexpected /mcp add response: %#v", resp)
	}
	if addedName != "test-srv" {
		t.Fatalf("expected AddMCPServerFn called with test-srv, got %q", addedName)
	}

	removedName := ""
	deps.RemoveMCPServerFn = func(name string) bool {
		removedName = name
		return true
	}
	d = newDispatcher(deps)
	resp = d.Handle("/mcp remove test-srv", system.ContextInfo{})
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "removed") {
		t.Fatalf("unexpected /mcp remove response: %#v", resp)
	}
	if removedName != "test-srv" {
		t.Fatalf("expected RemoveMCPServerFn called with test-srv, got %q", removedName)
	}
}

func TestHandleMCPResources(t *testing.T) {
	deps := baseDeps()
	deps.MCPResourcesFn = func(_ context.Context) []mcp.Resource {
		return []mcp.Resource{
			{URI: "file:///a.txt", Name: "alpha.txt", MimeType: "text/plain"},
			{URI: "file:///b.json", Name: "beta.json", MimeType: "application/json"},
		}
	}
	d := newDispatcher(deps)

	resp := d.Handle("/mcp resources", system.ContextInfo{})
	if len(resp.Entries) == 0 {
		t.Fatal("expected entries")
	}
	text := resp.Entries[0].Text
	if !strings.Contains(text, "alpha.txt") || !strings.Contains(text, "beta.json") {
		t.Errorf("expected both resources listed, got %q", text)
	}

	// Filter argument
	resp = d.Handle("/mcp resources beta", system.ContextInfo{})
	text = resp.Entries[0].Text
	if !strings.Contains(text, "beta.json") {
		t.Errorf("expected beta.json in filtered output, got %q", text)
	}
	if strings.Contains(text, "alpha.txt") {
		t.Errorf("filter should drop alpha.txt, got %q", text)
	}
}

func TestHandleMCPResourcesEmpty(t *testing.T) {
	deps := baseDeps()
	deps.MCPResourcesFn = func(_ context.Context) []mcp.Resource { return nil }
	d := newDispatcher(deps)
	resp := d.Handle("/mcp resources", system.ContextInfo{})
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "No MCP resources") {
		t.Fatalf("expected empty-state message, got %#v", resp.Entries)
	}
}

func TestHandleMCPRead(t *testing.T) {
	deps := baseDeps()
	deps.MCPResourceReadFn = func(_ context.Context, serverName, uri string) (*mcp.ReadResourceResult, error) {
		if serverName != "alpha" || uri != "file:///a.txt" {
			return nil, fmt.Errorf("unexpected call: %s %s", serverName, uri)
		}
		return &mcp.ReadResourceResult{
			Contents: []mcp.ResourceContent{
				{URI: "file:///a.txt", MimeType: "text/plain", Text: "hello world"},
			},
		}, nil
	}
	d := newDispatcher(deps)
	resp := d.Handle("/mcp read alpha file:///a.txt", system.ContextInfo{})
	if len(resp.Entries) == 0 {
		t.Fatal("expected entries")
	}
	if !strings.Contains(resp.Entries[0].Text, "hello world") {
		t.Errorf("expected read content, got %q", resp.Entries[0].Text)
	}

	// Missing args
	resp = d.Handle("/mcp read", system.ContextInfo{})
	if len(resp.Entries) == 0 || resp.Entries[0].Kind != types.EntryError {
		t.Errorf("expected error for missing args, got %#v", resp.Entries)
	}

	// Read error
	deps.MCPResourceReadFn = func(_ context.Context, _, _ string) (*mcp.ReadResourceResult, error) {
		return nil, fmt.Errorf("boom")
	}
	d = newDispatcher(deps)
	resp = d.Handle("/mcp read alpha file:///missing", system.ContextInfo{})
	if len(resp.Entries) == 0 || resp.Entries[0].Kind != types.EntryError || !strings.Contains(resp.Entries[0].Text, "boom") {
		t.Errorf("expected error containing boom, got %#v", resp.Entries)
	}
}

func TestHandleMCPPrompts(t *testing.T) {
	deps := baseDeps()
	deps.MCPPromptsFn = func(_ context.Context) []mcp.Prompt {
		return []mcp.Prompt{
			{
				Name:        "summarize",
				Description: "Summarise a file",
				Arguments: []mcp.PromptArgument{
					{Name: "path", Required: true},
					{Name: "max_words"},
				},
			},
			{
				Name:        "explain",
				Description: "Explain code",
			},
		}
	}
	d := newDispatcher(deps)
	resp := d.Handle("/mcp prompts", system.ContextInfo{})
	if len(resp.Entries) == 0 {
		t.Fatal("expected entries")
	}
	text := resp.Entries[0].Text
	if !strings.Contains(text, "summarize") || !strings.Contains(text, "explain") {
		t.Errorf("expected both prompts listed, got %q", text)
	}
	if !strings.Contains(text, "path*") {
		t.Errorf("expected required argument marker *, got %q", text)
	}
}

func TestHandleMCPGetPrompt(t *testing.T) {
	deps := baseDeps()
	var gotName, gotServer string
	var gotArgs map[string]string
	deps.MCPGetPromptFn = func(_ context.Context, serverName, name string, args map[string]string) (*mcp.GetPromptResult, error) {
		gotName = name
		gotServer = serverName
		gotArgs = args
		return &mcp.GetPromptResult{
			Description: "Summary",
			Messages: []mcp.PromptMessage{
				{Role: "user", Content: mcp.ContentBlock{Type: "text", Text: "Please summarise /tmp/x"}},
			},
		}, nil
	}
	d := newDispatcher(deps)
	resp := d.Handle("/mcp prompt alpha summarize path=/tmp/x max_words=100", system.ContextInfo{})
	if len(resp.Entries) == 0 {
		t.Fatal("expected entries")
	}
	if gotName != "summarize" || gotServer != "alpha" {
		t.Errorf("unexpected name/server: %q %q", gotName, gotServer)
	}
	if gotArgs["path"] != "/tmp/x" || gotArgs["max_words"] != "100" {
		t.Errorf("unexpected args: %v", gotArgs)
	}
	if !strings.Contains(resp.Entries[0].Text, "Please summarise /tmp/x") {
		t.Errorf("expected prompt text in output, got %q", resp.Entries[0].Text)
	}

	// Bad kv arg
	resp = d.Handle("/mcp prompt alpha summarize badkv", system.ContextInfo{})
	if len(resp.Entries) == 0 || resp.Entries[0].Kind != types.EntryError {
		t.Errorf("expected error for bad kv, got %#v", resp.Entries)
	}

	// Missing args
	resp = d.Handle("/mcp prompt", system.ContextInfo{})
	if len(resp.Entries) == 0 || resp.Entries[0].Kind != types.EntryError {
		t.Errorf("expected error for missing args, got %#v", resp.Entries)
	}
}

func TestHandleDynamicPrompt(t *testing.T) {
	deps := baseDeps()
	deps.MCPPromptHostsFn = func(_ context.Context) []mcp.PromptHost {
		return []mcp.PromptHost{
			{Server: "alpha", Prompt: mcp.Prompt{Name: "summarize", Description: "Summarise a file"}},
		}
	}
	deps.MCPGetPromptFn = func(_ context.Context, serverName, name string, args map[string]string) (*mcp.GetPromptResult, error) {
		if serverName != "alpha" || name != "summarize" {
			return nil, fmt.Errorf("unexpected call: %s %s", serverName, name)
		}
		return &mcp.GetPromptResult{
			Messages: []mcp.PromptMessage{
				{Role: "user", Content: mcp.ContentBlock{Type: "text", Text: "Hello from prompt"}},
			},
		}, nil
	}
	d := newDispatcher(deps)

	// Invoke by prompt name only
	resp := d.Handle("/summarize path=/tmp/x", system.ContextInfo{})
	if len(resp.Entries) == 0 {
		t.Fatal("expected entries")
	}
	if !strings.Contains(resp.Entries[0].Text, "Hello from prompt") {
		t.Errorf("expected prompt text, got %q", resp.Entries[0].Text)
	}

	// Unknown command (not a prompt either)
	resp = d.Handle("/nope", system.ContextInfo{})
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "Unknown command") {
		t.Errorf("expected unknown-command error, got %#v", resp.Entries)
	}

	// Bad kv arg
	resp = d.Handle("/summarize badkv", system.ContextInfo{})
	if len(resp.Entries) == 0 || resp.Entries[0].Kind != types.EntryError {
		t.Errorf("expected error for bad kv, got %#v", resp.Entries)
	}
}

func TestHandleDynamicPromptAmbiguous(t *testing.T) {
	deps := baseDeps()
	deps.MCPPromptHostsFn = func(_ context.Context) []mcp.PromptHost {
		return []mcp.PromptHost{
			{Server: "alpha", Prompt: mcp.Prompt{Name: "summarize"}},
			{Server: "beta", Prompt: mcp.Prompt{Name: "summarize"}},
		}
	}
	deps.MCPGetPromptFn = func(_ context.Context, _, _ string, _ map[string]string) (*mcp.GetPromptResult, error) {
		t.Fatal("getPromptFn should not be called for ambiguous names")
		return nil, nil
	}
	d := newDispatcher(deps)
	resp := d.Handle("/summarize", system.ContextInfo{})
	if len(resp.Entries) == 0 || resp.Entries[0].Kind != types.EntryError {
		t.Fatalf("expected ambiguity error, got %#v", resp.Entries)
	}
	if !strings.Contains(resp.Entries[0].Text, "multiple servers") {
		t.Errorf("expected ambiguity message, got %q", resp.Entries[0].Text)
	}
}

func TestDynamicPromptDoesNotShadowStaticCommand(t *testing.T) {
	// The static /help command must always win over a dynamic prompt
	// with the same name (none here, but the static path is verified
	// by ensuring /help still works without any prompt hosts).
	deps := baseDeps()
	d := newDispatcher(deps)
	resp := d.Handle("/help", system.ContextInfo{})
	if len(resp.Entries) == 0 || resp.Entries[0].Kind != types.EntryHelp {
		t.Errorf("expected help entry, got %#v", resp.Entries)
	}
}
