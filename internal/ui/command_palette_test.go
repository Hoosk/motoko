package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/app/taskman"
	"github.com/Hoosk/motoko/internal/brain"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/session"
	"github.com/Hoosk/motoko/internal/skills"
	"github.com/Hoosk/motoko/internal/system"
)

func TestCommandPaletteItemsIncludeContextualActions(t *testing.T) {
	b, err := brain.New("workspace", "session-1")
	if err != nil {
		t.Fatalf("brain.New: %v", err)
	}
	if err := b.Write("plan", "plan body"); err != nil {
		t.Fatalf("brain write: %v", err)
	}

	ctx := paletteContext{
		Info:           system.ContextInfo{ModifiedFiles: []string{"README.md"}},
		Providers:      []config.ProviderConfig{{Name: "openai", Preset: config.ProviderPresetOpenAI, Models: []string{"gpt-4.1"}}},
		Skills:         []skills.Skill{{Name: "golang-patterns", Description: "Go skill"}},
		Sessions:       []*session.Session{{ID: "s1", Title: "Recent", UpdatedAt: time.Now(), History: []provider.ConversationItem{{Content: "hi"}}}},
		Tasks:          []*taskman.TaskState{{ID: "task-1", Command: "go test", Running: true}},
		Agents:         []agent.AgentDef{{Name: "build"}},
		ActiveProvider: config.ProviderConfig{Name: "openai", Preset: config.ProviderPresetOpenAI, Models: []string{"gpt-4.1"}},
		Pending:        "git status",
		Thinking:       true,
		QueueLen:       2,
		Brain:          b,
	}

	items := commandPaletteItems(ctx)
	joined := renderPaletteItems(items)
	for _, want := range []string{"Approve pending command", "Deny pending command", "Cancel current request", "Manage queue (2 prompts)", "Model: gpt-4.1", "Skill: golang-patterns", "Brain: plan.md", "Terminate task: task-1", "Mention file: @README.md"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected palette items to include %q, got:\n%s", want, joined)
		}
	}
}

func TestFilterListViewRendersCategoryHeaders(t *testing.T) {
	fl := NewFilterList("Palette", "Search...")
	fl.Active = true
	fl.SetItems([]FilterableItem{
		paletteItem{category: "Actions", title: "Approve", searchKey: "approve"},
		paletteItem{category: "Workspace", title: "Skill", searchKey: "skill"},
	})

	view := fl.View()
	if !strings.Contains(view, "Actions") || !strings.Contains(view, "Workspace") {
		t.Fatalf("expected category headers in view, got %q", view)
	}
}

func renderPaletteItems(items []FilterableItem) string {
	var lines []string
	for _, item := range items {
		lines = append(lines, stripANSI(item.Render(false)))
	}
	return strings.Join(lines, "\n")
}
