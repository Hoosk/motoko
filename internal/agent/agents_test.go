package agent

import (
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/system"
)

func TestBuiltinAgentsExist(t *testing.T) {
	names := make(map[string]bool)
	for _, a := range BuiltinAgents {
		names[a.Name] = true
	}
	if !names["plan"] {
		t.Fatal("expected 'plan' in BuiltinAgents")
	}
	if !names["build"] {
		t.Fatal("expected 'build' in BuiltinAgents")
	}
	for _, name := range []string{"learn", "teamwork", "grill"} {
		if !names[name] {
			t.Fatalf("expected %q in BuiltinAgents", name)
		}
	}
}

func TestBuildSystemPromptInjectsAgentMode(t *testing.T) {
	info := system.ContextInfo{Workspace: "test", Path: "/tmp/test"}
	prompt := buildSystemPrompt("default", info, nil, "Modo test: solo pruebas.")
	if !strings.Contains(prompt, "Modo test: solo pruebas.") {
		t.Fatalf("expected agent system in prompt, got:\n%s", prompt)
	}
}

func TestBuildSystemPromptNoAgentModeWhenEmpty(t *testing.T) {
	info := system.ContextInfo{Workspace: "test", Path: "/tmp/test"}
	prompt := buildSystemPrompt("default", info, nil, "")
	if strings.Contains(prompt, "Modo test:") {
		t.Fatalf("did not expect agent system content when agentSystem is empty")
	}
}

func TestBuildSystemPromptInjectsOperationalModeForBuild(t *testing.T) {
	info := system.ContextInfo{Workspace: "test", Path: "/tmp/test", ActiveMode: "build"}
	prompt := buildSystemPrompt("default", info, nil, "")
	if !strings.Contains(prompt, "<operational_mode>") {
		t.Fatalf("expected <operational_mode> block in system prompt, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "OPERATIONAL DIRECTIVE — BUILD MODE") {
		t.Fatalf("expected build_switch content in system prompt, got:\n%s", prompt)
	}
}

func TestBuildSystemPromptInjectsOperationalModeForPlan(t *testing.T) {
	info := system.ContextInfo{Workspace: "test", Path: "/tmp/test", ActiveMode: "plan"}
	prompt := buildSystemPrompt("default", info, nil, "")
	if !strings.Contains(prompt, "<operational_mode>") {
		t.Fatalf("expected <operational_mode> block in system prompt, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "OPERATIONAL DIRECTIVE — PLAN MODE") {
		t.Fatalf("expected plan_active content in system prompt, got:\n%s", prompt)
	}
}

func TestBuildSystemPromptInjectsReasoningStyle(t *testing.T) {
	cases := []struct {
		verbosity string
		contains  string
	}{
		{"concise", "REASONING STYLE — concise"},
		{"caveman", "REASONING STYLE — caveman"},
		{"normal", "REASONING STYLE — default"},
		{"", "REASONING STYLE — default"},
		{"unknown_mode", "REASONING STYLE — default"},
	}
	for _, tc := range cases {
		t.Run(tc.verbosity, func(t *testing.T) {
			info := system.ContextInfo{
				Workspace:         "test",
				Path:              "/tmp/test",
				ThinkingVerbosity: tc.verbosity,
			}
			prompt := buildSystemPrompt("default", info, nil, "")
			if !strings.Contains(prompt, "<reasoning_style>") {
				t.Fatalf("expected <reasoning_style> block in system prompt for verbosity=%q, got:\n%s", tc.verbosity, prompt)
			}
			if !strings.Contains(prompt, tc.contains) {
				t.Fatalf("expected verbosity=%q to inject %q, got:\n%s", tc.verbosity, tc.contains, prompt)
			}
		})
	}
}
