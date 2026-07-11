package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteToolCreatesNewFile(t *testing.T) {
	withTempWorkspace(t)
	tool := NewWriteTool()

	res, err := tool.Run(context.Background(), "src/new.go\npackage new\n\nconst V = 1\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Summary, "created") {
		t.Errorf("expected summary to mention 'created', got %q", res.Summary)
	}

	data, err := os.ReadFile("src/new.go")
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(data) != "package new\n\nconst V = 1\n" {
		t.Errorf("unexpected file content: %q", string(data))
	}
}

func TestWriteToolOverwritesExistingFile(t *testing.T) {
	root := withTempWorkspace(t)
	existing := filepath.Join(root, "foo.txt")
	if err := os.WriteFile(existing, []byte("OLD CONTENT"), 0o600); err != nil {
		t.Fatal(err)
	}

	tool := NewWriteTool()
	res, err := tool.Run(context.Background(), "foo.txt\nNEW CONTENT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Summary, "overwrote") {
		t.Errorf("expected summary to mention 'overwrote', got %q", res.Summary)
	}

	data, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "NEW CONTENT" {
		t.Errorf("file not overwritten; got %q", string(data))
	}
}

func TestWriteToolCreatesNestedDirectories(t *testing.T) {
	root := withTempWorkspace(t)
	tool := NewWriteTool()

	_, err := tool.Run(context.Background(), "deep/nested/path/file.txt\nhello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := filepath.Join(root, "deep", "nested", "path", "file.txt")
	data, err := os.ReadFile(expected)
	if err != nil {
		t.Fatalf("nested file not created: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("unexpected content: %q", string(data))
	}
}

func TestWriteToolAcceptsJSONArgs(t *testing.T) {
	root := withTempWorkspace(t)
	tool := NewWriteTool()

	payload, _ := json.Marshal(map[string]string{
		"path":    "config.json",
		"content": `{"k":"v"}`,
	})
	res, err := tool.Run(context.Background(), string(payload))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Summary, "created") {
		t.Errorf("expected created, got %q", res.Summary)
	}

	data, err := os.ReadFile(filepath.Join(root, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"k":"v"}` {
		t.Errorf("unexpected JSON content: %q", string(data))
	}

	if !strings.Contains(res.Output, "absolute:") {
		t.Errorf("expected output to include absolute path, got %q", res.Output)
	}
}

func TestWriteToolRejectsEmptyContent(t *testing.T) {
	withTempWorkspace(t)
	tool := NewWriteTool()

	_, err := tool.Run(context.Background(), "foo.txt\n")
	if err == nil {
		t.Fatal("expected error for empty content")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected error to mention empty content, got %v", err)
	}

	if _, statErr := os.Stat("foo.txt"); statErr == nil {
		t.Error("file should not have been created")
	}
}

func TestWriteToolRejectsMissingPath(t *testing.T) {
	withTempWorkspace(t)
	tool := NewWriteTool()

	_, err := tool.Run(context.Background(), "   \n   ")
	if err == nil {
		t.Fatal("expected error for empty path and content")
	}
}

func TestWriteToolRejectsPathTraversal(t *testing.T) {
	withTempWorkspace(t)
	tool := NewWriteTool()

	_, err := tool.Run(context.Background(), "../escape.txt\nbad")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
	if !strings.Contains(err.Error(), "outside workspace") && !strings.Contains(err.Error(), "path") {
		t.Errorf("expected path error, got %v", err)
	}
}

func TestWriteToolRejectsAbsolutePathOutsideWorkspace(t *testing.T) {
	withTempWorkspace(t)
	tool := NewWriteTool()

	_, err := tool.Run(context.Background(), "/etc/passwd\nbad")
	if err == nil {
		t.Fatal("expected error for absolute path outside workspace")
	}
}

func TestWriteToolRejectsGitDirectory(t *testing.T) {
	withTempWorkspace(t)
	tool := NewWriteTool()

	_, err := tool.Run(context.Background(), ".git/hooks/pre-commit\n#!/bin/sh\nrm -rf /\n")
	if err == nil {
		t.Fatal("expected error writing to .git/")
	}
	if !strings.Contains(err.Error(), ".git") {
		t.Errorf("expected .git error, got %v", err)
	}
}

func TestWriteToolRejectsEnvFiles(t *testing.T) {
	withTempWorkspace(t)
	tool := NewWriteTool()

	_, err := tool.Run(context.Background(), ".env\nSECRET=hack")
	if err == nil {
		t.Fatal("expected error writing .env")
	}
	if !strings.Contains(err.Error(), ".env") {
		t.Errorf("expected .env error, got %v", err)
	}
}

func TestWriteToolRejectsEnvLocalFile(t *testing.T) {
	withTempWorkspace(t)
	tool := NewWriteTool()

	_, err := tool.Run(context.Background(), ".env.local\nSECRET=hack")
	if err == nil {
		t.Fatal("expected error writing .env.local")
	}
}

func TestWriteToolRejectsSSHKeys(t *testing.T) {
	withTempWorkspace(t)
	tool := NewWriteTool()

	_, err := tool.Run(context.Background(), ".ssh/id_rsa\nPRIVATE")
	if err == nil {
		t.Fatal("expected error writing SSH key")
	}
}

func TestWriteToolRejectsAntigravityConfig(t *testing.T) {
	withTempWorkspace(t)
	tool := NewWriteTool()

	_, err := tool.Run(context.Background(), ".antigravitycli/agent.json\n{}")
	if err == nil {
		t.Fatal("expected error writing .antigravitycli/")
	}
}

func TestWriteToolRejectsDirectoryAsTarget(t *testing.T) {
	root := withTempWorkspace(t)
	if err := os.MkdirAll(filepath.Join(root, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	tool := NewWriteTool()
	_, err := tool.Run(context.Background(), "subdir\ncontent")
	if err == nil {
		t.Fatal("expected error when target is an existing directory")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Errorf("expected directory error, got %v", err)
	}
}

func TestWriteToolAcceptsPathInsideWorkspace(t *testing.T) {
	root := withTempWorkspace(t)
	tool := NewWriteTool()

	_, err := tool.Run(context.Background(), "internal/system/context.go\nreplaced content\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "internal", "system", "context.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "replaced content\n" {
		t.Errorf("file content not replaced; got %q", string(data))
	}
}

func TestWriteToolRejectsJSONMissingPath(t *testing.T) {
	withTempWorkspace(t)
	tool := NewWriteTool()

	payload, _ := json.Marshal(map[string]string{"content": "x"})
	_, err := tool.Run(context.Background(), string(payload))
	if err == nil {
		t.Fatal("expected error for missing path in JSON")
	}
}

func TestWriteToolRejectsJSONMissingContent(t *testing.T) {
	withTempWorkspace(t)
	tool := NewWriteTool()

	payload, _ := json.Marshal(map[string]string{"path": "x.txt"})
	_, err := tool.Run(context.Background(), string(payload))
	if err == nil {
		t.Fatal("expected error for missing content in JSON")
	}
}

func TestWriteToolOutputIncludesAbsolutePath(t *testing.T) {
	root := withTempWorkspace(t)
	tool := NewWriteTool()

	res, err := tool.Run(context.Background(), "abs.txt\nhello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	absExpected, _ := filepath.Abs(filepath.Join(root, "abs.txt"))
	if !strings.Contains(res.Output, absExpected) {
		t.Errorf("expected output to include absolute path %q, got %q", absExpected, res.Output)
	}
}

func TestWriteToolTrimsLeadingWriteToken(t *testing.T) {
	withTempWorkspace(t)
	tool := NewWriteTool()

	res, err := tool.Run(context.Background(), "write token.txt\nhello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Summary, "created") {
		t.Errorf("expected created, got %q", res.Summary)
	}

	data, err := os.ReadFile("token.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("unexpected content: %q", string(data))
	}
}

func TestIsWriteToolRecognizesWrite(t *testing.T) {
	if !IsWriteTool("write") {
		t.Error("expected IsWriteTool(\"write\") to be true")
	}
	if !IsWriteTool("WRITE") {
		t.Error("expected IsWriteTool to be case-insensitive")
	}
}

func TestNewRegistryIncludesWrite(t *testing.T) {
	r := NewRegistry()
	spec, ok := r.Spec(ToolContext{}, "write")
	if !ok {
		t.Fatal("expected write tool to be registered in default registry")
	}
	if spec.Name != "write" {
		t.Errorf("expected spec name 'write', got %q", spec.Name)
	}
}
