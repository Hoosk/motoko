package agentorch

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/brain"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/semantic"
	"github.com/Hoosk/motoko/internal/semantic/symtypes"
	"github.com/Hoosk/motoko/internal/session"
	"github.com/Hoosk/motoko/internal/skills"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tools"

	"github.com/Hoosk/motoko/internal/app/types"
)

func minimalDeps() Deps {
	return Deps{
		ConfigFn:          func() *config.AppConfig { return &config.AppConfig{} },
		ProviderClientFn:  func() func(config.ProviderConfig) (provider.Client, error) { return nil },
		ToolsFn:           func() *tools.Registry { return tools.NewRegistry() },
		SemanticFn:        func() *semantic.Index { return nil },
		BrainFn:           func() *brain.Brain { return nil },
		CurrentSessionFn:  func() *session.Session { return nil },
		WorkspaceIDFn:     func() string { return "test" },
		ContextWindowFn:   func() int { return 128_000 },
		AvailableAgentsFn: func() []agent.AgentDef { return nil },
		AvailableSkillsFn: func() []skills.Skill { return nil },
		ContextInfoFn:     func() system.ContextInfo { return system.ContextInfo{} },
	}
}

func newTestOrch(deps Deps) *Orchestrator {
	return New(deps)
}

func TestEnrichContextAddsRelevantSnippets(t *testing.T) {
	semIdx := semantic.NewIndex()
	snapshot := &semantic.Snapshot{
		Snapshot: symtypes.Snapshot{
			Files: []semantic.FileSummary{{
				Path:     "internal/app/runtime.go",
				Language: "go",
				Content:  []byte("package app\n\nfunc RunAgent() error {\n\treturn nil\n}\n"),
				Symbols:  []semantic.Symbol{{Name: "RunAgent", Kind: "func", Line: 3, Range: semantic.LineRange{Start: 3, End: 5}}},
			}},
			GeneratedAt: time.Now(),
		},
	}
	semIdx.SetSnapshotForTest(snapshot)

	deps := minimalDeps()
	deps.SemanticFn = func() *semantic.Index { return semIdx }
	orch := newTestOrch(deps)

	info := orch.EnrichContext(context.Background(), system.ContextInfo{}, "")
	if len(info.RelevantSnippets) == 0 {
		t.Fatal("expected relevant snippets")
	}
	if !strings.Contains(info.RelevantSnippets[0], "RunAgent") {
		t.Fatalf("expected snippet mentioning RunAgent, got %q", info.RelevantSnippets[0])
	}
}

func TestEnrichContextAddsAvailableSkills(t *testing.T) {
	deps := minimalDeps()
	deps.AvailableSkillsFn = func() []skills.Skill {
		return []skills.Skill{
			{Name: "test-skill", Description: "A test skill"},
		}
	}
	orch := newTestOrch(deps)

	info := orch.EnrichContext(context.Background(), system.ContextInfo{}, "")
	if len(info.AvailableSkills) != 1 {
		t.Fatalf("expected 1 available skill, got %d", len(info.AvailableSkills))
	}
	if info.AvailableSkills[0].Name != "test-skill" {
		t.Errorf("expected skill 'test-skill', got %q", info.AvailableSkills[0].Name)
	}
}

func TestEnrichContextAddsAvailableAgents(t *testing.T) {
	deps := minimalDeps()
	deps.AvailableAgentsFn = func() []agent.AgentDef {
		return []agent.AgentDef{
			{Name: "plan"},
			{Name: "build"},
		}
	}
	orch := newTestOrch(deps)

	info := orch.EnrichContext(context.Background(), system.ContextInfo{}, "")
	if len(info.AvailableAgents) != 2 {
		t.Fatalf("expected 2 available agents, got %d", len(info.AvailableAgents))
	}
	if info.AvailableAgents[0] != "plan" || info.AvailableAgents[1] != "build" {
		t.Errorf("expected [plan build], got %v", info.AvailableAgents)
	}
}

func TestEnrichContextBrainSummary(t *testing.T) {
	tmpDir := t.TempDir()
	br, err := brain.New(tmpDir, "test-session")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = br.Destroy() }()

	if err := br.Write("plan.md", "This is my plan"); err != nil {
		t.Fatal(err)
	}

	deps := minimalDeps()
	deps.BrainFn = func() *brain.Brain { return br }
	orch := newTestOrch(deps)

	info := orch.EnrichContext(context.Background(), system.ContextInfo{}, "")
	if !strings.Contains(info.BrainSummary, "plan.md") {
		t.Errorf("expected BrainSummary to mention plan.md, got: %q", info.BrainSummary)
	}
	if !strings.Contains(info.BrainSummary, "This is my plan") {
		t.Errorf("expected BrainSummary to contain plan content, got: %q", info.BrainSummary)
	}
}

func TestEnrichContextMentionedFiles(t *testing.T) {
	semIdx := semantic.NewIndex()
	snapshot := &semantic.Snapshot{
		Snapshot: symtypes.Snapshot{
			Files: []semantic.FileSummary{{
				Path:     "main.go",
				Language: "go",
				Content:  []byte("package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"),
				Symbols:  []semantic.Symbol{{Name: "main", Kind: "func", Line: 3, Range: semantic.LineRange{Start: 3, End: 5}}},
			}},
			GeneratedAt: time.Now(),
		},
	}
	semIdx.SetSnapshotForTest(snapshot)

	deps := minimalDeps()
	deps.SemanticFn = func() *semantic.Index { return semIdx }
	orch := newTestOrch(deps)
	orch.SetMentionedFiles([]string{"main.go"})

	info := orch.EnrichContext(context.Background(), system.ContextInfo{}, "")
	if len(info.RelevantSnippets) == 0 {
		t.Fatal("expected relevant snippets for mentioned file")
	}
	found := false
	for _, s := range info.RelevantSnippets {
		if strings.Contains(s, "FILE main.go") && strings.Contains(s, "explicit @ mention") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected explicit @ mention snippet for main.go")
	}
}

func TestEnrichContextNoSemanticIndex(t *testing.T) {
	orch := newTestOrch(minimalDeps())
	info := orch.EnrichContext(context.Background(), system.ContextInfo{}, "")
	if info.ContextWindow != 128_000 {
		t.Errorf("expected ContextWindow 128000, got %d", info.ContextWindow)
	}
}

func TestSetAgentMode(t *testing.T) {
	deps := minimalDeps()
	deps.AvailableAgentsFn = func() []agent.AgentDef {
		return []agent.AgentDef{
			{Name: "plan"},
			{Name: "build"},
		}
	}
	orch := newTestOrch(deps)

	if orch.Mode() != types.ModePlan {
		t.Errorf("expected initial ModePlan, got %q", orch.Mode())
	}

	orch.SetAgentMode("build")
	if orch.Mode() != types.ModeBuild {
		t.Errorf("expected ModeBuild after SetAgentMode(build), got %q", orch.Mode())
	}
	if orch.AgentName() != "build" {
		t.Errorf("expected AgentName build, got %q", orch.AgentName())
	}
}

func TestSetAgentModeUnknown(t *testing.T) {
	orch := newTestOrch(minimalDeps())
	orch.SetAgentMode("nonexistent")
	if orch.Mode() != types.ModePlan {
		t.Errorf("expected ModePlan unchanged for unknown agent, got %q", orch.Mode())
	}
}

func TestSetTestAgents(t *testing.T) {
	orch := newTestOrch(minimalDeps())
	orch.SetTestAgents([]agent.AgentDef{{Name: "explore"}, {Name: "search"}})

	agents := orch.AvailableAgents()
	if len(agents) != 2 {
		t.Fatalf("expected 2 test agents, got %d", len(agents))
	}
	if agents[0].Name != "explore" || agents[1].Name != "search" {
		t.Errorf("unexpected agents: %v", agents)
	}
}

func TestSetTestSkills(t *testing.T) {
	orch := newTestOrch(minimalDeps())
	orch.SetTestSkills([]skills.Skill{{Name: "s1"}, {Name: "s2"}})

	sk := orch.AvailableSkills()
	if len(sk) != 2 {
		t.Fatalf("expected 2 test skills, got %d", len(sk))
	}
	if sk[0].Name != "s1" || sk[1].Name != "s2" {
		t.Errorf("unexpected skills: %v", sk)
	}
}

func TestModeAndSetMode(t *testing.T) {
	orch := newTestOrch(minimalDeps())
	if orch.Mode() != types.ModePlan {
		t.Errorf("expected initial ModePlan, got %q", orch.Mode())
	}
	orch.SetMode(types.ModeBuild)
	if orch.Mode() != types.ModeBuild {
		t.Errorf("expected ModeBuild after SetMode, got %q", orch.Mode())
	}
}

func TestDebugAndSetDebug(t *testing.T) {
	orch := newTestOrch(minimalDeps())
	if orch.Debug() {
		t.Error("expected Debug=false initially")
	}
	orch.SetDebug(true)
	if !orch.Debug() {
		t.Error("expected Debug=true after SetDebug(true)")
	}
}

func TestAgentNames(t *testing.T) {
	deps := minimalDeps()
	deps.AvailableAgentsFn = func() []agent.AgentDef {
		return []agent.AgentDef{{Name: "plan"}, {Name: "build"}}
	}
	orch := newTestOrch(deps)

	names := orch.AgentNames()
	if len(names) != 2 || names[0] != "plan" || names[1] != "build" {
		t.Errorf("expected [plan build], got %v", names)
	}
}

func TestActiveSubagentsEmpty(t *testing.T) {
	orch := newTestOrch(minimalDeps())
	subs := orch.ActiveSubagents()
	if len(subs) != 0 {
		t.Errorf("expected 0 active subagents, got %v", subs)
	}
}

func TestSystemPromptNoAgent(t *testing.T) {
	orch := newTestOrch(minimalDeps())
	prompt := orch.SystemPrompt(system.ContextInfo{})
	if prompt != "Agent not configured." {
		t.Errorf("expected 'Agent not configured.', got %q", prompt)
	}
}

// stubReadOnlyTool is an MCP-like tool that reports a readOnlyHint.
type stubReadOnlyTool struct {
	name     string
	readOnly bool
}

func (s stubReadOnlyTool) Spec() tools.Spec { return tools.Spec{Name: s.name} }
func (s stubReadOnlyTool) Run(context.Context, string) (tools.Result, error) {
	return tools.Result{}, nil
}
func (s stubReadOnlyTool) IsReadOnly() bool { return s.readOnly }

// stubPlainTool does not implement the IsReadOnly interface (a local tool).
type stubPlainTool struct {
	name string
}

func (s stubPlainTool) Spec() tools.Spec { return tools.Spec{Name: s.name} }
func (s stubPlainTool) Run(context.Context, string) (tools.Result, error) {
	return tools.Result{}, nil
}

func TestToolAllowedForAgentReadOnlyFiltering(t *testing.T) {
	readOnlyDef := agent.AgentDef{Name: "plan"} // AllowWrite defaults to false
	buildDef := agent.AgentDef{Name: "build", Permissions: agent.AgentPermissions{AllowWrite: true}}

	mcpReadOnly := stubReadOnlyTool{name: "mcp_git_read", readOnly: true}
	mcpMutating := stubReadOnlyTool{name: "mcp_git_write", readOnly: false}
	local := stubPlainTool{name: "read"}

	cases := []struct {
		name string
		tool tools.Tool
		def  agent.AgentDef
		want bool
	}{
		{"read-only agent keeps annotated read-only MCP tool", mcpReadOnly, readOnlyDef, true},
		{"read-only agent excludes mutating MCP tool", mcpMutating, readOnlyDef, false},
		{"read-only agent keeps local tools (no IsReadOnly interface)", local, readOnlyDef, true},
		{"build agent keeps mutating MCP tool", mcpMutating, buildDef, true},
		{"build agent keeps read-only MCP tool", mcpReadOnly, buildDef, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := toolAllowedForAgent(tc.tool, tc.def, nil, nil); got != tc.want {
				t.Errorf("toolAllowedForAgent(%s) = %v, want %v", tc.tool.Spec().Name, got, tc.want)
			}
		})
	}
}

func TestToolAllowedForAgentExplicitLists(t *testing.T) {
	def := agent.AgentDef{Name: "custom", Permissions: agent.AgentPermissions{
		AllowedTools: []string{"read"},
		AllowWrite:   true,
	}}
	// The allow/deny slices are resolved at the call site (agent def or
	// override) and passed in; nil means "no explicit list".
	if !toolAllowedForAgent(stubPlainTool{name: "read"}, def, []string{"read"}, nil) {
		t.Error("read should be allowed by the allowed list")
	}
	if toolAllowedForAgent(stubPlainTool{name: "write"}, def, []string{"read"}, nil) {
		t.Error("write should be excluded by the allowed list")
	}

	// Deny list: an explicitly denied tool is always excluded, even with
	// allow-write enabled.
	defNoWrite := agent.AgentDef{Name: "custom", Permissions: agent.AgentPermissions{AllowWrite: true}}
	if toolAllowedForAgent(stubPlainTool{name: "write"}, defNoWrite, nil, []string{"write"}) {
		t.Error("write should be excluded by the denied list")
	}

	// Write gating: with allow-write false, bash is excluded even without
	// any explicit list.
	defReadOnly := agent.AgentDef{Name: "custom"}
	if toolAllowedForAgent(stubPlainTool{name: "bash"}, defReadOnly, nil, nil) {
		t.Error("bash should be excluded when allow-write is false")
	}
}
