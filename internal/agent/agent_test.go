package agent

import (
	"os"
	"path/filepath"
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
	prompt := buildSystemPrompt(info, []tools.Spec{{Name: "read", Summary: "Lee archivos", Usage: "read <ruta>"}}, "")
	if !strings.Contains(prompt, "[Pre-extracted Relevant Snippets]:") {
		t.Fatalf("prompt missing snippets section: %s", prompt)
	}
	if !strings.Contains(prompt, "func RunAgent() error") {
		t.Fatalf("prompt missing snippet content: %s", prompt)
	}
}

func TestBuildSystemPromptIncludesAgentsAndDesign(t *testing.T) {
	tmpDir := t.TempDir()

	// Write mock AGENTS.md
	agentsContent := "Rule 1: Be fast.\nRule 2: Be precise."
	if err := os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte(agentsContent), 0644); err != nil {
		t.Fatalf("failed to write mock AGENTS.md: %v", err)
	}

	// Write mock DESIGN.md
	designContent := "Primary color: #00FF00\nBorder radius: 4px"
	if err := os.WriteFile(filepath.Join(tmpDir, "DESIGN.md"), []byte(designContent), 0644); err != nil {
		t.Fatalf("failed to write mock DESIGN.md: %v", err)
	}

	info := system.ContextInfo{
		Workspace: "motoko",
		Path:      tmpDir,
	}

	prompt := buildSystemPrompt(info, nil, "")

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
