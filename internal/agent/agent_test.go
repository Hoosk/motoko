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

type captureProvider struct {
	messages []provider.ConversationItem
	tools    provider.ToolSet
}

func (c *captureProvider) Configured() bool     { return true }
func (c *captureProvider) ProviderKind() string { return "fake" }
func (c *captureProvider) Summary() string      { return "fake:capture" }
func (c *captureProvider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	return []provider.ModelInfo{{ID: "capture"}}, nil
}
func (c *captureProvider) GetModel(ctx context.Context, model string) (provider.ModelInfo, error) {
	return provider.ModelInfo{ID: model}, nil
}
func (c *captureProvider) Complete(ctx context.Context, systemPrompt string, messages []provider.Message, tools provider.ToolSet) (provider.Response, error) {
	c.messages = append([]provider.ConversationItem(nil), messages...)
	c.tools = tools
	return provider.Response{FinalText: "ok", OutputItems: []provider.ConversationItem{provider.AssistantText("ok")}}, nil
}
func (c *captureProvider) StreamComplete(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet, onDelta func(provider.Delta) error) (provider.Response, error) {
	return c.Complete(ctx, systemPrompt, messages, tools)
}

func TestCompleteAppendsDynamicPromptAsSeparateMessage(t *testing.T) {
	p := &captureProvider{}
	a := New(p, tools.NewRegistry())
	messages := []provider.ConversationItem{provider.UserText("haz algo")}
	info := system.ContextInfo{Workspace: "motoko", Path: "/tmp/motoko", ActiveMode: "plan"}
	specs := a.tools.Specs(buildToolContext(info))

	if _, err := a.complete(context.Background(), info, messages, nil, specs); err != nil {
		t.Fatal(err)
	}
	if len(p.messages) != 2 {
		t.Fatalf("expected original message plus dynamic tail, got %#v", p.messages)
	}
	if p.messages[0].Content != "haz algo" {
		t.Fatalf("expected original user message untouched, got %#v", p.messages[0])
	}
	if p.messages[1].Role != provider.RoleUser {
		t.Fatalf("expected dynamic tail to be a user message, got %#v", p.messages[1])
	}
	if !strings.Contains(p.messages[1].Content, "<environment>") {
		t.Fatalf("expected dynamic tail to include environment context, got %q", p.messages[1].Content)
	}
	if !strings.Contains(p.messages[1].Content, "PLAN MODE") {
		t.Fatalf("expected dynamic tail to include active mode fragment/context, got %q", p.messages[1].Content)
	}
	if messages[0].Content != "haz algo" {
		t.Fatalf("expected input slice to remain unmodified, got %#v", messages)
	}
}
