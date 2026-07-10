package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/provider"
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
	dynamicPrompt := buildDynamicPrompt("default", info)
	if strings.Contains(prompt, "[Pre-extracted Relevant Snippets]:") {
		t.Fatalf("static prompt should not contain dynamic snippets section: %s", prompt)
	}
	if !strings.Contains(dynamicPrompt, "[Pre-extracted Relevant Snippets]:") {
		t.Fatalf("dynamic prompt missing snippets section: %s", dynamicPrompt)
	}
	if !strings.Contains(dynamicPrompt, "func RunAgent() error") {
		t.Fatalf("dynamic prompt missing snippet content: %s", dynamicPrompt)
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

	if !strings.Contains(prompt, "<agents_guidelines>") {
		t.Errorf("prompt missing <agents_guidelines> section header")
	}
	if !strings.Contains(prompt, agentsContent) {
		t.Errorf("prompt missing AGENTS.md content")
	}

	if !strings.Contains(prompt, "<design_specification>") {
		t.Errorf("prompt missing <design_specification> section header")
	}
	if !strings.Contains(prompt, designContent) {
		t.Errorf("prompt missing DESIGN.md content")
	}
}

func TestCompleteAppendsDynamicPromptAsSeparateMessage(t *testing.T) {
	var capturedMessages []provider.ConversationItem
	var capturedTools provider.ToolSet
	p := &fakeProviderClient{summary: "fake:capture", models: []provider.ModelInfo{{ID: "capture"}}, completeFn: func(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet) (provider.Response, error) {
		capturedMessages = append([]provider.ConversationItem(nil), messages...)
		capturedTools = tools
		return provider.Response{FinalText: "ok", OutputItems: []provider.ConversationItem{provider.AssistantText("ok")}}, nil
	}}
	a := New(p, tools.NewRegistry())
	messages := []provider.ConversationItem{provider.UserText("haz algo")}
	info := system.ContextInfo{Workspace: "motoko", Path: "/tmp/motoko", ActiveMode: "plan"}
	specs := a.tools.Specs(buildToolContext(info))

	if _, err := a.complete(context.Background(), info, messages, nil, specs); err != nil {
		t.Fatal(err)
	}
	if len(capturedMessages) != 2 {
		t.Fatalf("expected original message plus dynamic tail, got %#v", capturedMessages)
	}
	if capturedMessages[0].Content != "haz algo" {
		t.Fatalf("expected original user message untouched, got %#v", capturedMessages[0])
	}
	if capturedMessages[1].Role != provider.RoleUser {
		t.Fatalf("expected dynamic tail to be a user message, got %#v", capturedMessages[1])
	}
	if !strings.Contains(capturedMessages[1].Content, "<environment>") {
		t.Fatalf("expected dynamic tail to include environment context, got %q", capturedMessages[1].Content)
	}
	if !strings.Contains(capturedMessages[1].Content, "PLAN MODE") {
		t.Fatalf("expected dynamic tail to include active mode fragment/context, got %q", capturedMessages[1].Content)
	}
	if len(capturedTools.Local) != len(specs) {
		t.Fatalf("expected captured tool set to match computed specs")
	}
	if messages[0].Content != "haz algo" {
		t.Fatalf("expected input slice to remain unmodified, got %#v", messages)
	}
}

func TestBuildSystemPromptIncludesPatchFormatNote(t *testing.T) {
	info := system.ContextInfo{
		Workspace: "motoko",
		Path:      "/tmp/motoko",
	}
	prompt := buildSystemPrompt("default", info, []tools.Spec{
		{Name: "patch", Summary: "Applies changes", Usage: "patch <path>"},
	}, "")

	if !strings.Contains(prompt, "<<<<<<< AST") {
		t.Fatalf("prompt missing AST patch format markers: %s", prompt)
	}
	if !strings.Contains(prompt, "Selector keys (key: value)") {
		t.Fatalf("prompt missing AST selector keys documentation: %s", prompt)
	}
	if !strings.Contains(prompt, "type, name, query, capture, action, contains, index") {
		t.Fatalf("prompt missing valid AST selector keys: %s", prompt)
	}
	if !strings.Contains(prompt, "function_declaration") {
		t.Fatalf("prompt missing AST format example: %s", prompt)
	}
}

func TestBuildSystemPromptIncludesInspectNote(t *testing.T) {
	info := system.ContextInfo{
		Workspace: "motoko",
		Path:      "/tmp/motoko",
	}
	prompt := buildSystemPrompt("default", info, []tools.Spec{
		{Name: "inspect", Summary: "Get Tachikoma data", Usage: "inspect <worker>"},
	}, "")

	if !strings.Contains(prompt, "PREFERRED way to access on-demand Tachikoma data") {
		t.Fatalf("prompt missing inspect preference statement: %s", prompt)
	}
	if !strings.Contains(prompt, "GitTachikoma, CodeTachikoma, DiffTachikoma, SearchTachikoma, DependencyTachikoma") {
		t.Fatalf("prompt missing valid inspect worker names: %s", prompt)
	}
	if !strings.Contains(prompt, "inspect CodeTachikoma") {
		t.Fatalf("prompt missing inspect example for CodeTachikoma: %s", prompt)
	}
}

func TestBuildDynamicPromptOnDemandSignalUsesInspect(t *testing.T) {
	info := system.ContextInfo{
		Workspace:       "motoko",
		Path:            "/tmp/motoko",
		OnDemandSignals: map[string]string{"DiffTachikoma": "changes available"},
	}
	prompt := buildDynamicPrompt("default", info)

	if !strings.Contains(prompt, "use the 'inspect' tool with the worker name") {
		t.Fatalf("dynamic prompt should instruct use of inspect for on-demand signals: %s", prompt)
	}
	if strings.Contains(prompt, "use your tools (read, grep, etc.)") {
		t.Fatalf("dynamic prompt should not use old generic tool instruction: %s", prompt)
	}
}
