package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func withTempWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "system"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal", "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "system", "context.go"), []byte("package system\n\ntype ContextInfo struct{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "app", "runtime.go"), []byte("package app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})
	return root
}

func withTempGitWorkspace(t *testing.T) string {
	root := withTempWorkspace(t)
	runGitInWorkspace(t, root, "init")
	return root
}

type fakeTool struct {
	name string
}

func (f fakeTool) Spec() Spec { return Spec{Name: f.name, Summary: "fake", Usage: f.name + " <arg>"} }
func (f fakeTool) Run(ctx context.Context, args string) (Result, error) {
	return Result{Spec: f.Spec(), Summary: "ok", Output: args}, nil
}

func TestRegistrySuggestionsAndRun(t *testing.T) {
	r := &Registry{tools: map[string]Tool{}}
	r.Register(fakeTool{name: "zeta"})
	r.Register(fakeTool{name: "alpha"})

	specs := r.Specs()
	if len(specs) != 2 || specs[0].Name != "alpha" {
		t.Fatalf("expected sorted specs, got %#v", specs)
	}
	if len(r.Suggestions("al")) != 1 {
		t.Fatalf("expected prefix suggestion, got %#v", r.Suggestions("al"))
	}
	result, err := r.Run(context.Background(), "alpha", "hola")
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "hola" {
		t.Fatalf("unexpected run result %#v", result)
	}
}

func TestRegistryRunTruncatesLargeToolOutput(t *testing.T) {
	r := &Registry{tools: map[string]Tool{}}
	r.Register(fakeTool{name: "alpha"})
	large := strings.Repeat("a", maxToolOutputBytes+50)
	result, err := r.Run(context.Background(), "alpha", large)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(result.Output, truncatedToolOutputSuffix) {
		t.Fatalf("expected truncated suffix, got %q", result.Output)
	}
	if len(result.Output) != maxToolOutputBytes+len(truncatedToolOutputSuffix) {
		t.Fatalf("unexpected truncated length %d", len(result.Output))
	}
}

func TestCompileGlobMatchesRecursivePattern(t *testing.T) {
	re, err := compileGlob("internal/**/*.go")
	if err != nil {
		t.Fatal(err)
	}
	if !re.MatchString("internal/app/runtime.go") {
		t.Fatal("expected recursive glob to match")
	}
	if re.MatchString("README.md") {
		t.Fatal("did not expect README.md match")
	}
}

func TestGlobToolFindsGoFiles(t *testing.T) {
	withTempWorkspace(t)
	result, err := NewGlobTool().Run(context.Background(), "internal/app/*.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "internal/app/runtime.go") {
		t.Fatalf("expected runtime.go in output, got %q", result.Output)
	}
}

func TestGrepToolFindsContextInfoInSystemPackage(t *testing.T) {
	withTempWorkspace(t)
	result, err := NewGrepTool().Run(context.Background(), "ContextInfo internal/system/*.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "internal/system/context.go") {
		t.Fatalf("expected context file match, got %q", result.Output)
	}
}

func TestGlobAndGrepSkipGitIgnoredPaths(t *testing.T) {
	root := withTempGitWorkspace(t)
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("internal/app/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	globResult, err := NewGlobTool().Run(context.Background(), "internal/**/*.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(globResult.Output, "internal/app/runtime.go") {
		t.Fatalf("expected ignored glob path skipped, got %q", globResult.Output)
	}
	grepResult, err := NewGrepTool().Run(context.Background(), "package internal/**/*.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(grepResult.Output, "internal/app/runtime.go") {
		t.Fatalf("expected ignored grep path skipped, got %q", grepResult.Output)
	}
	readResult, err := NewReadTool().Run(context.Background(), "internal/app/runtime.go 1 2")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readResult.Output, "package app") {
		t.Fatalf("expected read to allow explicit ignored path, got %q", readResult.Output)
	}
}

func TestReadToolReadsFileAndDirectory(t *testing.T) {
	withTempWorkspace(t)
	fileResult, err := NewReadTool().Run(context.Background(), "internal/system/context.go 1 3")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fileResult.Output, "package system") {
		t.Fatalf("expected file contents, got %q", fileResult.Output)
	}
	dirResult, err := NewReadTool().Run(context.Background(), "internal/system")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dirResult.Output, "context.go") {
		t.Fatalf("expected directory listing, got %q", dirResult.Output)
	}
}

func TestBashToolSuccessAndExitStatus(t *testing.T) {
	success, err := NewBashTool().Run(context.Background(), "printf hola")
	if err != nil {
		t.Fatal(err)
	}
	if success.Output != "hola" {
		t.Fatalf("expected hola output, got %#v", success)
	}
	failure, err := NewBashTool().Run(context.Background(), "exit 7")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(failure.Summary, "salida 7") {
		t.Fatalf("expected exit code summary, got %#v", failure)
	}
}

func TestPatchToolSupportsASTDeleteAction(t *testing.T) {
	root := withTempWorkspace(t)
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc One() {}\n\nfunc Two() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NewPatchTool().Run(context.Background(), "main.go\n<<<<<<< AST\ntype: function_declaration\naction: delete\nname: One\n=======\n>>>>>>> REPLACE")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	if strings.Contains(text, "func One()") {
		t.Fatalf("expected function deleted, got %q", text)
	}
	if !strings.Contains(text, "func Two() {}") {
		t.Fatalf("expected remaining function preserved, got %q", text)
	}
}

func TestPatchToolRejectsAmbiguousASTPatch(t *testing.T) {
	root := withTempWorkspace(t)
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc One() {}\n\nfunc Two() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NewPatchTool().Run(context.Background(), "main.go\n<<<<<<< AST\ncapture: target\nquery:\n(function_declaration) @target\n=======\nfunc One() {}\n>>>>>>> REPLACE")
	if err == nil {
		t.Fatal("expected ambiguous AST patch to fail")
	}
	if !strings.Contains(err.Error(), "query AST coincide") {
		t.Fatalf("unexpected AST ambiguity error: %v", err)
	}
}

func TestPatchToolRejectsMissingQueryCapture(t *testing.T) {
	root := withTempWorkspace(t)
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc Run() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NewPatchTool().Run(context.Background(), "main.go\n<<<<<<< AST\ncapture: target\nquery:\n(function_declaration) @fn\n=======\nfunc Run() {}\n>>>>>>> REPLACE")
	if err == nil {
		t.Fatal("expected missing capture AST patch to fail")
	}
	if !strings.Contains(err.Error(), "captura requerida") {
		t.Fatalf("unexpected missing capture error: %v", err)
	}
}

func TestPatchToolAppliesASTPatchInJavaScript(t *testing.T) {
	root := withTempWorkspace(t)
	path := filepath.Join(root, "main.js")
	if err := os.WriteFile(path, []byte("function one() {\n  return 1\n}\n\nfunction two() {\n  return 2\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NewPatchTool().Run(context.Background(), "main.js\n<<<<<<< AST\ncapture: target\nquery: (function_declaration body: (statement_block) @target)\nindex: 2\n=======\n{\n  return 9\n}\n>>>>>>> REPLACE")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "function two() {\n  return 9\n}") {
		t.Fatalf("expected JS AST patch replacement, got %q", string(updated))
	}
	if !strings.Contains(string(updated), "function one() {\n  return 1\n}") {
		t.Fatalf("expected first JS function untouched, got %q", string(updated))
	}
}

func TestPatchToolCreatesFileFromUnifiedDiff(t *testing.T) {
	root := withTempWorkspace(t)
	path := filepath.Join(root, "notes.txt")
	_, err := NewPatchTool().Run(context.Background(), "--- /dev/null\n+++ b/notes.txt\n@@ -0,0 +1,2 @@\n+uno\n+dos")
	if err != nil {
		t.Fatal(err)
	}
	created, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(created) != "uno\ndos\n" {
		t.Fatalf("unexpected created file %q", string(created))
	}
}

func TestPatchToolInsertsBeforeASTNode(t *testing.T) {
	root := withTempWorkspace(t)
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc One() {}\nfunc Two() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NewPatchTool().Run(context.Background(), "main.go\n<<<<<<< AST\ntype: function_declaration\naction: insert_before\nname: Two\n=======\nfunc New() {}\n>>>>>>> REPLACE")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	newIdx := strings.Index(text, "func New()")
	twoIdx := strings.Index(text, "func Two()")
	if newIdx == -1 || twoIdx == -1 || newIdx >= twoIdx {
		t.Fatalf("expected New() inserted before Two(), got %q", text)
	}
	if !strings.Contains(text, "func One() {}") {
		t.Fatalf("expected One() preserved, got %q", text)
	}
}

func TestPatchToolInsertsAfterASTNode(t *testing.T) {
	root := withTempWorkspace(t)
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc One() {}\nfunc Two() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NewPatchTool().Run(context.Background(), "main.go\n<<<<<<< AST\ntype: function_declaration\naction: insert_after\nname: One\n=======\nfunc New() {}\n>>>>>>> REPLACE")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	oneIdx := strings.Index(text, "func One()")
	newIdx := strings.Index(text, "func New()")
	twoIdx := strings.Index(text, "func Two()")
	if oneIdx == -1 || newIdx == -1 || twoIdx == -1 {
		t.Fatalf("expected all three functions present, got %q", text)
	}
	if !(oneIdx < newIdx && newIdx < twoIdx) {
		t.Fatalf("expected One() < New() < Two(), got %q", text)
	}
}

func runGitInWorkspace(t *testing.T, workdir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = workdir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}
