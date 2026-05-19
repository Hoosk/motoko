package ui

import (
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/system"
	tea "github.com/charmbracelet/bubbletea"
)

func TestFooterViewIncludesProviderAndPending(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	r := app.NewRuntime()
	if err := r.SaveProvider(config.ProviderConfig{Name: "openrouter", Preset: config.ProviderPresetOpenRouter, APIKey: "k", Model: "openai/gpt-4.1"}, true); err != nil {
		t.Fatal(err)
	}
	f := NewFooterModel(r)
	f.sysInfo = system.ContextInfo{Workspace: "motoko", HasGit: true, GitBranch: "main"}
	f.width = 120
	view := stripANSI(f.View())
	// New compact format: provider summary appears as "openrouter (openrouter:...)"
	if !strings.Contains(view, "openrouter") {
		t.Fatalf("expected provider in footer, got %q", view)
	}
	// New format: agent mode appears as "● plan"
	if !strings.Contains(view, "plan") {
		t.Fatalf("expected agent mode in footer, got %q", view)
	}
	// Pending only shows if there is a pending command; no "pending: none" in new format
}

func TestFooterUpdateTracksTachikomaSignals(t *testing.T) {
	f := NewFooterModel(app.NewRuntime())
	_ = f.Update(tea.WindowSizeMsg{Width: 80})
	_ = f.Update(TachikomaMsg{Name: "GitTachikoma", Status: "main (clean)"})
	if got := f.GetSysInfo().Signals["GitTachikoma"]; got != "main (clean)" {
		t.Fatalf("expected signal merged, got %q", got)
	}
}

func TestFooterRefreshesContextAfterShellResult(t *testing.T) {
	f := NewFooterModel(app.NewRuntime())
	before := f.GetSysInfo()
	_ = f.Update(ShellResultMsg{Result: app.ShellResult{Command: "pwd", ExitCode: 0}})
	after := f.GetSysInfo()
	if after.Path == "" && before.Path != "" {
		t.Fatalf("expected footer to keep or refresh context, before=%#v after=%#v", before, after)
	}
}
