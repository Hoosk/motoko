package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func externalApprovalContext(t *testing.T, allow bool) context.Context {
	t.Helper()
	broker := NewQuestionBroker()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		pending, err := broker.Next(ctx)
		if err != nil {
			return
		}
		if allow {
			pending.Resolve(Answer{Selections: []string{approveExternalOption}})
			return
		}
		pending.Resolve(Answer{Selections: []string{"Deny"}})
	}()
	return WithQuestionBroker(ctx, broker)
}

func TestReadToolRequiresApprovalForExternalSymlink(t *testing.T) {
	root := withTempWorkspace(t)
	external := filepath.Join(t.TempDir(), "external.txt")
	if err := os.WriteFile(external, []byte("external contents\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "external-link")); err != nil {
		t.Fatal(err)
	}

	if _, err := NewReadTool().Run(context.Background(), "external-link"); err == nil || !strings.Contains(err.Error(), "requires approval") {
		t.Fatalf("expected missing approval error, got %v", err)
	}
	if _, err := NewReadTool().Run(externalApprovalContext(t, false), "external-link"); err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("expected denied approval error, got %v", err)
	}
	result, err := NewReadTool().Run(externalApprovalContext(t, true), "external-link")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "external contents") {
		t.Fatalf("unexpected read output %q", result.Output)
	}
}

func TestGrepToolRequiresApprovalForExternalSymlink(t *testing.T) {
	root := withTempWorkspace(t)
	external := filepath.Join(t.TempDir(), "external.txt")
	if err := os.WriteFile(external, []byte("needle outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "external-link.txt")); err != nil {
		t.Fatal(err)
	}

	result, err := NewGrepTool().Run(externalApprovalContext(t, true), "needle *.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "external-link.txt") {
		t.Fatalf("unexpected grep output %q", result.Output)
	}
}

func TestWriteToolCreatesFileBelowApprovedExternalSymlink(t *testing.T) {
	root := withTempWorkspace(t)
	externalDir := t.TempDir()
	if err := os.Symlink(externalDir, filepath.Join(root, "external-dir")); err != nil {
		t.Fatal(err)
	}

	_, err := NewWriteTool().Run(externalApprovalContext(t, true), "external-dir/nested/file.txt\napproved")
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(externalDir, "nested", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "approved" {
		t.Fatalf("unexpected external content %q", content)
	}
}

func TestWriteToolUsesApprovedDestinationIfSymlinkChanges(t *testing.T) {
	root := withTempWorkspace(t)
	first := filepath.Join(t.TempDir(), "first.txt")
	second := filepath.Join(t.TempDir(), "second.txt")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(root, "external-link")
	if err := os.Symlink(first, link); err != nil {
		t.Fatal(err)
	}

	broker := NewQuestionBroker()
	ctx := WithQuestionBroker(context.Background(), broker)
	errCh := make(chan error, 1)
	go func() {
		_, err := NewWriteTool().Run(ctx, "external-link\napproved")
		errCh <- err
	}()
	pending, err := broker.Next(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(second, link); err != nil {
		t.Fatal(err)
	}
	pending.Resolve(Answer{Selections: []string{approveExternalOption}})
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	firstContent, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	secondContent, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstContent) != "approved" || string(secondContent) != "original" {
		t.Fatalf("unexpected destinations: first=%q second=%q", firstContent, secondContent)
	}
}

func TestWriteToolRejectsCanonicalTargetReplacementAfterApproval(t *testing.T) {
	root := withTempWorkspace(t)
	externalDir := t.TempDir()
	target := filepath.Join(externalDir, "target.txt")
	backup := filepath.Join(externalDir, "target.backup")
	other := filepath.Join(t.TempDir(), "other.txt")
	if err := os.WriteFile(target, []byte("approved target"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("other target"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "external-link")); err != nil {
		t.Fatal(err)
	}

	broker := NewQuestionBroker()
	ctx := WithQuestionBroker(context.Background(), broker)
	errCh := make(chan error, 1)
	go func() {
		_, err := NewWriteTool().Run(ctx, "external-link\nchanged")
		errCh <- err
	}()
	pending, err := broker.Next(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(target, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, target); err != nil {
		t.Fatal(err)
	}
	pending.Resolve(Answer{Selections: []string{approveExternalOption}})
	if err := <-errCh; err == nil {
		t.Fatal("expected changed canonical target to be rejected")
	}

	backupContent, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	otherContent, err := os.ReadFile(other)
	if err != nil {
		t.Fatal(err)
	}
	if string(backupContent) != "approved target" || string(otherContent) != "other target" {
		t.Fatalf("replacement modified a file: backup=%q other=%q", backupContent, otherContent)
	}
}

func TestExternalApprovalFailsClosedForMixedSelections(t *testing.T) {
	root := withTempWorkspace(t)
	external := filepath.Join(t.TempDir(), "external.txt")
	if err := os.WriteFile(external, []byte("external"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "external-link")); err != nil {
		t.Fatal(err)
	}

	broker := NewQuestionBroker()
	ctx := WithQuestionBroker(context.Background(), broker)
	errCh := make(chan error, 1)
	go func() {
		_, err := NewReadTool().Run(ctx, "external-link")
		errCh <- err
	}()
	pending, err := broker.Next(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pending.Resolve(Answer{Selections: []string{approveExternalOption, "Deny"}})
	if err := <-errCh; err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("expected mixed answer to fail closed, got %v", err)
	}
}

func TestWriteToolRejectsParentReplacementForNewExternalFile(t *testing.T) {
	root := withTempWorkspace(t)
	externalBase := t.TempDir()
	targetDir := filepath.Join(externalBase, "approved-parent")
	backupDir := filepath.Join(externalBase, "approved-parent.backup")
	if err := os.Mkdir(targetDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetDir, filepath.Join(root, "external-dir")); err != nil {
		t.Fatal(err)
	}

	broker := NewQuestionBroker()
	ctx := WithQuestionBroker(context.Background(), broker)
	errCh := make(chan error, 1)
	go func() {
		_, err := NewWriteTool().Run(ctx, "external-dir/new/file.txt\nchanged")
		errCh <- err
	}()
	pending, err := broker.Next(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(targetDir, backupDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(targetDir, 0o700); err != nil {
		t.Fatal(err)
	}
	pending.Resolve(Answer{Selections: []string{approveExternalOption}})
	if err := <-errCh; err == nil || !strings.Contains(err.Error(), "parent path changed") {
		t.Fatalf("expected replaced parent to be rejected, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "new", "file.txt")); !os.IsNotExist(err) {
		t.Fatalf("replacement parent was modified: %v", err)
	}
}

func TestReadToolDoesNotInjectExternalAgentInstructions(t *testing.T) {
	root := withTempWorkspace(t)
	external := filepath.Join(t.TempDir(), "AGENTS.md")
	if err := os.WriteFile(external, []byte("external secret instructions"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}

	result, err := NewReadTool().Run(context.Background(), "internal/system/context.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Output, "external secret instructions") {
		t.Fatalf("external instructions were injected: %q", result.Output)
	}
}
