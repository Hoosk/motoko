package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/config"
	patchtool "github.com/Hoosk/motoko/internal/tools/patch"
)

func TestWriteToolCreatesNewFile(t *testing.T) {
	withTempWorkspace(t)
	tool := NewWriteTool()

	res, err := tool.Run(context.Background(), `{"path":"src/new.go","content":"package new\n\nconst V = 1\n"}`)
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

func TestWriteToolRequiresAndHonorsApproval(t *testing.T) {
	root := withTempWorkspace(t)
	path := filepath.Join(root, "approved.txt")
	broker := NewBroker()
	cfg := &config.AppConfig{EditApproval: config.EditApprovalAsk}
	ctx := WithBroker(WithConfig(context.Background(), cfg), broker)
	tool := NewWriteTool()
	result := make(chan error, 1)

	go func() {
		_, err := tool.Run(ctx, `{"path":"approved.txt","content":"new content\n"}`)
		result <- err
	}()
	pending, err := broker.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Change.Path != "approved.txt" || !strings.Contains(pending.Change.Diff, "+new content") {
		t.Fatalf("unexpected approval request %#v", pending.Change)
	}
	pending.Resolve(DialogDecision{Approved: true})
	if err := <-result; err != nil {
		t.Fatalf("approved write failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new content\n" {
		t.Fatalf("unexpected approved file content %q", data)
	}

	result = make(chan error, 1)
	go func() {
		_, err := tool.Run(ctx, `{"path":"rejected.txt","content":"not written\n"}`)
		result <- err
	}()
	pending, err = broker.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pending.Resolve(DialogDecision{Approved: false})
	err = <-result
	if !errors.Is(err, ErrChangeRejected) {
		t.Fatalf("expected rejected write error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "rejected.txt")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("rejected write created a file, stat error: %v", statErr)
	}
}

func TestWriteToolFailsClosedWithoutDialogBroker(t *testing.T) {
	withTempWorkspace(t)
	cfg := &config.AppConfig{EditApproval: config.EditApprovalAsk}
	ctx := WithConfig(context.Background(), cfg)

	_, err := NewWriteTool().Run(ctx, `{"path":"blocked.txt","content":"content"}`)
	if !errors.Is(err, ErrApprovalUnavailable) {
		t.Fatalf("expected missing broker error, got %v", err)
	}
	if _, statErr := os.Stat("blocked.txt"); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("write without broker created a file, stat error: %v", statErr)
	}
}

func TestWriteToolRejectsStaleApproval(t *testing.T) {
	root := withTempWorkspace(t)
	path := filepath.Join(root, "stale.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	broker := NewBroker()
	cfg := &config.AppConfig{EditApproval: config.EditApprovalAsk}
	ctx := WithBroker(WithConfig(context.Background(), cfg), broker)
	result := make(chan error, 1)
	go func() {
		_, err := NewWriteTool().Run(ctx, `{"path":"stale.txt","content":"proposed\n"}`)
		result <- err
	}()
	pending, err := broker.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("external\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pending.Resolve(DialogDecision{Approved: true})
	if err := <-result; !errors.Is(err, patchtool.ErrFileChanged) {
		t.Fatalf("expected stale file error, got %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "external\n" {
		t.Fatalf("stale approval overwrote external content: %q", data)
	}
}

func TestWriteToolHonorsCancelledContext(t *testing.T) {
	root := withTempWorkspace(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewWriteTool().Run(ctx, `{"path":"cancelled.txt","content":"content"}`)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "cancelled.txt")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("cancelled write created a file, stat error: %v", statErr)
	}
}

func TestWriteToolOverwritesExistingFile(t *testing.T) {
	root := withTempWorkspace(t)
	existing := filepath.Join(root, "foo.txt")
	if err := os.WriteFile(existing, []byte("OLD CONTENT"), 0o600); err != nil {
		t.Fatal(err)
	}

	tool := NewWriteTool()
	res, err := tool.Run(context.Background(), `{"path":"foo.txt","content":"NEW CONTENT"}`)
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

	_, err := tool.Run(context.Background(), `{"path":"deep/nested/path/file.txt","content":"hello"}`)
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

	_, err := tool.Run(context.Background(), `{"path":"foo.txt","content":""}`)
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

	_, err := tool.Run(context.Background(), `{"content":"x"}`)
	if err == nil {
		t.Fatal("expected error for empty path and content")
	}
}

func TestWriteToolRejectsPathTraversal(t *testing.T) {
	withTempWorkspace(t)
	tool := NewWriteTool()

	_, err := tool.Run(context.Background(), `{"path":"../escape.txt","content":"bad"}`)
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

	_, err := tool.Run(context.Background(), `{"path":"/etc/passwd","content":"bad"}`)
	if err == nil {
		t.Fatal("expected error for absolute path outside workspace")
	}
}

func TestWriteToolRejectsGitDirectory(t *testing.T) {
	withTempWorkspace(t)
	tool := NewWriteTool()

	_, err := tool.Run(context.Background(), `{"path":".git/hooks/pre-commit","content":"#!/bin/sh\nrm -rf /\n"}`)
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

	_, err := tool.Run(context.Background(), `{"path":".env","content":"SECRET=hack"}`)
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

	_, err := tool.Run(context.Background(), `{"path":".env.local","content":"SECRET=hack"}`)
	if err == nil {
		t.Fatal("expected error writing .env.local")
	}
}

func TestWriteToolRejectsSSHKeys(t *testing.T) {
	withTempWorkspace(t)
	tool := NewWriteTool()

	_, err := tool.Run(context.Background(), `{"path":".ssh/id_rsa","content":"PRIVATE"}`)
	if err == nil {
		t.Fatal("expected error writing SSH key")
	}
}

func TestWriteToolRejectsAntigravityConfig(t *testing.T) {
	withTempWorkspace(t)
	tool := NewWriteTool()

	_, err := tool.Run(context.Background(), `{"path":".antigravitycli/agent.json","content":"{}"}`)
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
	_, err := tool.Run(context.Background(), `{"path":"subdir","content":"content"}`)
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

	_, err := tool.Run(context.Background(), `{"path":"internal/system/context.go","content":"replaced content\n"}`)
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

	res, err := tool.Run(context.Background(), `{"path":"abs.txt","content":"hello world"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	absExpected, _ := filepath.Abs(filepath.Join(root, "abs.txt"))
	if !strings.Contains(res.Output, absExpected) {
		t.Errorf("expected output to include absolute path %q, got %q", absExpected, res.Output)
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
