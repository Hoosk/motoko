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
