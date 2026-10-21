package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Hoosk/motoko/internal/tools"
)

func TestWorkspaceTrustPromptAppliesProjectConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home) // os.UserConfigDir ignores XDG_CONFIG_HOME on macOS
	t.Setenv("XDG_CONFIG_HOME", home)
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".agents", "config.json"), []byte(`{"theme": "nord"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workspace)

	r := NewRuntime()
	if r.Config().Theme == "nord" || r.Config().PendingWorkspace() == nil {
		t.Fatal("untrusted project config must not load at startup")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		// Answer the way the question popup does.
		pending, err := r.Broker().Next(ctx)
		if err != nil {
			return
		}
		pending.Resolve(tools.DialogDecision{Answer: tools.Answer{Selections: []string{workspaceTrustOption}}})
	}()
	if !r.AskWorkspaceTrust(ctx) {
		t.Fatal("expected the workspace to be trusted")
	}
	r.ApplyPendingWorkspace()
	if r.Config().Theme != "nord" || r.Config().PendingWorkspace() != nil {
		t.Fatalf("trusted config not applied: theme=%q", r.Config().Theme)
	}

	// Remembered for the next start.
	if again := NewRuntime(); again.Config().Theme != "nord" {
		t.Fatal("trust was not remembered")
	}
}

func TestWorkspaceTrustPromptDeclined(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".agents", "config.json"), []byte(`{"theme": "nord"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workspace)

	r := NewRuntime()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		pending, err := r.Broker().Next(ctx)
		if err != nil {
			return
		}
		pending.Resolve(tools.DialogDecision{Answer: tools.Answer{Selections: []string{"Not now"}}})
	}()
	if r.AskWorkspaceTrust(ctx) {
		t.Fatal("declining must not trust the workspace")
	}
	if again := NewRuntime(); again.Config().Theme == "nord" {
		t.Fatal("declined workspace config loaded on the next start")
	}
}
