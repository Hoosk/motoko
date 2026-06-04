package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/system"
)

func TestParseAgentsFile(t *testing.T) {
	content := `
# comentario

[codereview]
system = Eres un revisor de código experto. Detecta bugs y sugiere mejoras.
system = Responde en español.

[minimal]
system = Modo minimalista.
`
	agents := ParseAgentsFile(content)
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d: %#v", len(agents), agents)
	}
	if agents[0].Name != "codereview" {
		t.Fatalf("expected name 'codereview', got %q", agents[0].Name)
	}
	if !strings.Contains(agents[0].System, "revisor de código") {
		t.Fatalf("expected system to contain first line, got %q", agents[0].System)
	}
	if !strings.Contains(agents[0].System, "Responde en español") {
		t.Fatalf("expected system to contain second line, got %q", agents[0].System)
	}
	if agents[1].Name != "minimal" {
		t.Fatalf("expected name 'minimal', got %q", agents[1].Name)
	}
}

func TestParseAgentsFileEmpty(t *testing.T) {
	agents := ParseAgentsFile("")
	if len(agents) != 0 {
		t.Fatalf("expected 0 agents from empty input, got %d", len(agents))
	}
}

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
}

func TestBuildSystemPromptInjectsAgentMode(t *testing.T) {
	info := system.ContextInfo{Workspace: "test", Path: "/tmp/test"}
	prompt := buildSystemPrompt(info, nil, "Modo test: solo pruebas.")
	if !strings.Contains(prompt, "--- AGENT MODE ---") {
		t.Fatalf("expected AGENT MODE section in prompt, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Modo test: solo pruebas.") {
		t.Fatalf("expected agent system in prompt, got:\n%s", prompt)
	}
}

func TestBuildSystemPromptNoAgentModeWhenEmpty(t *testing.T) {
	info := system.ContextInfo{Workspace: "test", Path: "/tmp/test"}
	prompt := buildSystemPrompt(info, nil, "")
	if strings.Contains(prompt, "--- AGENT MODE ---") {
		t.Fatalf("did not expect AGENT MODE section when agentSystem is empty")
	}
}

func TestLoadAgentsFileDirFallback(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "motoko-agents-dir-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Test fallback file candidate
	agentsFilePath := filepath.Join(tmpDir, "agents.ini")
	content := `
[diragent]
system = Loaded from agents.ini inside directory
`
	if err := os.WriteFile(agentsFilePath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write agents.ini: %v", err)
	}

	agents, err := LoadAgentsFile(tmpDir)
	if err != nil {
		t.Fatalf("failed to load agents file: %v", err)
	}

	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}

	if agents[0].Name != "diragent" {
		t.Errorf("expected name 'diragent', got '%s'", agents[0].Name)
	}
}
