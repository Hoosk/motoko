package agent

import (
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tools"
)

func TestBuildSystemPromptIncludesRelevantSnippets(t *testing.T) {
	info := system.ContextInfo{
		Workspace:        "motoko",
		Path:             "/tmp/motoko",
		SemanticSummary:  "files:10",
		RelevantFiles:    []string{"internal/app/runtime.go [go] | symbols: RunAgent"},
		RelevantSnippets: []string{"FILE internal/app/runtime.go\nLINES 10-20\nREASON symbol match: RunAgent\nfunc RunAgent() error {\n\treturn nil\n}"},
	}
	prompt := buildSystemPrompt("default", info, []tools.Spec{{Name: "read", Summary: "Lee archivos", Usage: "read <ruta>"}}, "")
	if !strings.Contains(prompt, "[Pre-extracted Relevant Snippets]:") {
		t.Fatalf("prompt missing snippets section: %s", prompt)
	}
	if !strings.Contains(prompt, "func RunAgent() error") {
		t.Fatalf("prompt missing snippet content: %s", prompt)
	}
}

func TestBuildSystemPromptIncludesAgentsAndDesign(t *testing.T) {
	agentsContent := "Rule 1: Be fast.\nRule 2: Be precise."
	designContent := "Primary color: #00FF00\nBorder radius: 4px"

	info := system.ContextInfo{
		Workspace:  "motoko",
		Guidelines: agentsContent,
		DesignSpec: designContent,
	}

	prompt := buildSystemPrompt("default", info, nil, "Modo test: solo pruebas.")

	if !strings.Contains(prompt, "AGENTS & DESIGN RULES") {
		t.Errorf("prompt missing the operating rule alignment instruction")
	}

	if !strings.Contains(prompt, "--- AGENTS GUIDELINES (AGENTS.md) ---") {
		t.Errorf("prompt missing AGENTS.md section header")
	}
	if !strings.Contains(prompt, agentsContent) {
		t.Errorf("prompt missing AGENTS.md content")
	}

	if !strings.Contains(prompt, "--- DESIGN SPECIFICATION (DESIGN.md) ---") {
		t.Errorf("prompt missing DESIGN.md section header")
	}
	if !strings.Contains(prompt, designContent) {
		t.Errorf("prompt missing DESIGN.md content")
	}
}
