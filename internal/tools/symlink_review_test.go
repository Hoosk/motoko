package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/config"
	patchtool "github.com/Hoosk/motoko/internal/tools/patch"
)

func TestReadInstructionsFromSymlinkedWorkspace(t *testing.T) {
	root := withTempWorkspace(t)
	alias := filepath.Join(t.TempDir(), "workspace")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(alias); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PWD", alias)
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("workspace convention"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := NewReadTool().Run(context.Background(), "internal/system/context.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "workspace convention") {
		t.Fatalf("missing workspace instructions: %s", result.Output)
	}
}

func TestGrepSkipsDanglingSymlinks(t *testing.T) {
	root := withTempWorkspace(t)
	if err := os.Symlink("missing.txt", filepath.Join(root, "broken.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "normal.txt"), []byte("needle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := NewGrepTool().Run(context.Background(), "needle *.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "normal.txt:1: needle") {
		t.Fatalf("missing match: %s", result.Output)
	}
}

func TestExternalApprovalRejectsCustomContradiction(t *testing.T) {
	root := withTempWorkspace(t)
	external := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(external, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	broker := NewBroker()
	ctx = WithBroker(ctx, broker)
	done := make(chan error, 1)
	go func() { _, err := NewReadTool().Run(ctx, "linked"); done <- err }()
	pending, err := broker.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pending.Resolve(DialogDecision{Answer: Answer{Selections: []string{approveExternalOption}, Custom: "Do not allow access"}})
	if err := <-done; err == nil {
		t.Fatal("contradictory approval accepted")
	}
}

func TestWriteRejectsStaleExternalDiffApproval(t *testing.T) {
	root := withTempWorkspace(t)
	external := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(external, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}

	broker := NewBroker()
	ctx := WithBroker(WithConfig(context.Background(), &config.AppConfig{EditApproval: config.EditApprovalAsk}), broker)
	done := make(chan error, 1)
	go func() {
		_, err := NewWriteTool().Run(ctx, "linked\nproposed\n")
		done <- err
	}()

	externalApproval, err := broker.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if externalApproval.Kind != DialogQuestion {
		t.Fatalf("first dialog kind = %q, want external question", externalApproval.Kind)
	}
	externalApproval.Resolve(DialogDecision{Answer: Answer{Selections: []string{approveExternalOption}}})

	diffApproval, err := broker.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if diffApproval.Kind != DialogFileChange {
		t.Fatalf("second dialog kind = %q, want file change", diffApproval.Kind)
	}
	if err := os.WriteFile(external, []byte("concurrent change\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	diffApproval.Resolve(DialogDecision{Approved: true})

	if err := <-done; !errors.Is(err, patchtool.ErrFileChanged) {
		t.Fatalf("expected stale external file error, got %v", err)
	}
	content, err := os.ReadFile(external)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "concurrent change\n" {
		t.Fatalf("stale approval overwrote external content: %q", content)
	}
}
