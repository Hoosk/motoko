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
	if !strings.Contains(view, "provider: openrouter") {
		t.Fatalf("expected provider in footer, got %q", view)
	}
	if !strings.Contains(view, "pending: none") {
		t.Fatalf("expected pending label in footer, got %q", view)
	}
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
