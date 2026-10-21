package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// outsideDir returns a directory outside the temp workspace holding a
// "secret" file, standing in for ~/.ssh or a shell rc file.
func outsideDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "secret"), []byte("private key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

func TestReadToolRejectsSymlinkLeavingWorkspace(t *testing.T) {
	root := withTempWorkspace(t)
	outside := outsideDir(t)
	symlink(t, outside, filepath.Join(root, "keys"))

	result, err := NewReadTool().Run(context.Background(), `{"path":"keys/secret"}`)
	if err == nil {
		t.Fatalf("expected read through symlink to fail, got %q", result.Output)
	}
	if strings.Contains(result.Output, "private key") {
		t.Fatal("secret content leaked")
	}
}

func TestWriteToolRejectsSymlinkLeavingWorkspace(t *testing.T) {
	root := withTempWorkspace(t)
	outside := outsideDir(t)
	symlink(t, outside, filepath.Join(root, "linkdir"))
	symlink(t, filepath.Join(outside, "secret"), filepath.Join(root, "rc"))
	// Dangling: writing through it would create a file outside.
	symlink(t, filepath.Join(outside, "created"), filepath.Join(root, "dangling"))

	for _, path := range []string{"linkdir/new.txt", "rc", "dangling"} {
		if _, err := NewWriteTool().Run(context.Background(), `{"path":"`+path+`","content":"pwned"}`); err == nil {
			t.Errorf("expected write to %s to fail", path)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(outside, "secret")); string(b) != "private key\n" {
		t.Fatalf("outside file modified: %q", b)
	}
	for _, name := range []string{"new.txt", "created"} {
		if _, err := os.Stat(filepath.Join(outside, name)); err == nil {
			t.Errorf("file %s created outside the workspace", name)
		}
	}
}

func TestWriteToolFollowsSymlinkInsideWorkspace(t *testing.T) {
	root := withTempWorkspace(t)
	symlink(t, filepath.Join(root, "internal"), filepath.Join(root, "alias"))

	if _, err := NewWriteTool().Run(context.Background(), `{"path":"alias/notes.txt","content":"ok"}`); err != nil {
		t.Fatalf("write through an in-workspace symlink failed: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(root, "internal", "notes.txt")); err != nil || string(b) != "ok" {
		t.Fatalf("expected internal/notes.txt to be written, got %q, %v", b, err)
	}
}

func TestWriteToolDenyListAppliesToSymlinkTarget(t *testing.T) {
	root := withTempWorkspace(t)
	if err := os.MkdirAll(filepath.Join(root, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlink(t, filepath.Join(root, ".git", "hooks"), filepath.Join(root, "hooks"))

	if _, err := NewWriteTool().Run(context.Background(), `{"path":"hooks/pre-commit","content":"#!/bin/sh"}`); err == nil {
		t.Fatal("expected write into .git through a symlink to be blocked")
	}
}

func TestGrepSkipsSymlinkLeavingWorkspace(t *testing.T) {
	root := withTempWorkspace(t)
	outside := outsideDir(t)
	symlink(t, filepath.Join(outside, "secret"), filepath.Join(root, "id_backup"))

	result, err := NewGrepTool().Run(context.Background(), `{"pattern":"private key"}`)
	if err == nil && strings.Contains(result.Output, "private key") {
		t.Fatalf("grep read a file outside the workspace: %q", result.Output)
	}
}
