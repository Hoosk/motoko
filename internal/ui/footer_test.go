package ui

import (
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/system"
)

func TestFooterUpdatesOnContextInfo(t *testing.T) {
	r := app.NewRuntime()
	f := NewFooterModel(r)
	f.width = 80

	info := system.ContextInfo{
		Workspace: "motoko",
		GitBranch: "main",
		HasGit:    true,
	}

	f, _ = f.Update(ContextInfoMsg{Info: info})
	view := f.View()

	if !strings.Contains(view, "motoko") {
		t.Errorf("expected workspace name in footer, got %q", view)
	}
	if !strings.Contains(view, "main") {
		t.Errorf("expected git branch in footer, got %q", view)
	}
}

func TestFooterUpdatesOnTokens(t *testing.T) {
	r := app.NewRuntime()
	f := NewFooterModel(r)
	f.width = 80

	f, _ = f.Update(ContextTokensMsg{Tokens: 5000, Window: 128000})
	view := f.View()

	if !strings.Contains(view, "5k/128k") {
		t.Errorf("expected tokens info in footer, got %q", view)
	}
}

func TestFooterThinkingState(t *testing.T) {
	r := app.NewRuntime()
	f := NewFooterModel(r)
	f.width = 80

	f.SetThinking(true)
	f, _ = f.Update(ThinkingTickMsg{})
	view := f.View()

	// New logic uses planning/building labels
	if !strings.Contains(view, "planning") && !strings.Contains(view, "building") && !strings.Contains(view, "processing") {
		t.Errorf("expected activity label in footer, got %q", view)
	}
}
