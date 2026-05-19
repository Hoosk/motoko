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
	prompt := buildSystemPrompt(info, []tools.Spec{{Name: "read", Summary: "Lee archivos", Usage: "read <ruta>"}}, "")
	if !strings.Contains(prompt, "[Pre-extracted Relevant Snippets]:") {
		t.Fatalf("prompt missing snippets section: %s", prompt)
	}
	if !strings.Contains(prompt, "func RunAgent() error") {
		t.Fatalf("prompt missing snippet content: %s", prompt)
	}
}
