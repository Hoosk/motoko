package patch

import (
	"context"
	"os"
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

func TestResolveWorkspacePathRejectsOutsideWorkspace(t *testing.T) {
	root := withTempWorkspace(t)
	outside := filepath.Dir(root)
	if _, _, err := resolveWorkspacePath(outside); err == nil {
		t.Fatal("expected outside workspace error")
	}
}

func TestParsePatchInputParsesSearchReplaceBlock(t *testing.T) {
	path, search, replace, err := parsePatchInput("README.md\n<<<<<<< SEARCH\nold\n=======\nnew\n>>>>>>> REPLACE")
	if err != nil {
		t.Fatal(err)
	}
	if path != "README.md" || search != "old\n" || replace != "new\n" {
		t.Fatalf("unexpected parse result: %q %q %q", path, search, replace)
	}
}

func TestParsePatchRequestAcceptsUnifiedDiff(t *testing.T) {
	request, err := parsePatchRequest("--- a/README.md\n+++ b/README.md\n@@ -1,1 +1,1 @@\n-old\n+new")
	if err != nil {
		t.Fatal(err)
	}
	if request.Unified == nil {
		t.Fatal("expected unified patch request")
	}
	if request.Unified.OldPath != "README.md" || request.Unified.NewPath != "README.md" {
		t.Fatalf("unexpected unified paths %#v", request.Unified)
	}
}

func TestParsePatchRequestAcceptsASTPatch(t *testing.T) {
	request, err := parsePatchRequest("main.go\n<<<<<<< AST\ncapture: target\nquery:\n(function_declaration name: (identifier) @name body: (block) @target)\n=======\nfunc Run() {}\n>>>>>>> REPLACE")
	if err != nil {
		t.Fatal(err)
	}
	if request.AST == nil {
		t.Fatal("expected AST patch request")
	}
	if len(request.AST) != 1 || request.AST[0].Path != "main.go" || request.AST[0].Selector.Query == "" || request.AST[0].Selector.Capture != "target" {
		t.Fatalf("unexpected AST patch %#v", request.AST)
	}
}

func TestParsePatchRequestAcceptsMultipleASTPatches(t *testing.T) {
	request, err := parsePatchRequest("main.go\n<<<<<<< AST\ntype: function_declaration\nname: One\n=======\nfunc One() int {\n\treturn 1\n}\n>>>>>>> REPLACE\n<<<<<<< AST\ntype: function_declaration\nname: Two\n=======\nfunc Two() int {\n\treturn 2\n}\n>>>>>>> REPLACE")
	if err != nil {
		t.Fatal(err)
	}
	if len(request.AST) != 2 {
		t.Fatalf("expected 2 AST patches, got %#v", request.AST)
	}
	if request.AST[0].Selector.Name != "One" || request.AST[1].Selector.Name != "Two" {
		t.Fatalf("expected AST indices preserved, got %#v", request.AST)
	}
}

func TestFuzzyReplaceAllowsUniqueExactSingleLineSearch(t *testing.T) {
	updated, err := fuzzyReplace("uno\ndos unico\ntres\n", "dos unico", "dos cambiado")
	if err != nil {
		t.Fatalf("fuzzyReplace() error = %v", err)
	}
	if updated != "uno\ndos cambiado\ntres\n" {
		t.Fatalf("unexpected updated content %q", updated)
	}
}

func TestPatchToolAppliesSearchReplacePatch(t *testing.T) {
	root := withTempWorkspace(t)
	path := filepath.Join(root, "test.md")
	content := "Linea inicial\n## Siguientes Pasos\n- Esperar a que el usuario decida.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := New().Run(context.Background(), "test.md\n<<<<<<< SEARCH\n## Siguientes Pasos\n=======\n# mi moto alpina derrapante\n## Siguientes Pasos\n>>>>>>> REPLACE")
	if err != nil {
		t.Fatalf("patch tool error = %v", err)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	if !strings.Contains(text, "# mi moto alpina derrapante\n## Siguientes Pasos") {
		t.Fatalf("expected inserted heading, got %q", text)
	}
}

func TestPatchToolAppliesUnifiedDiffPatch(t *testing.T) {
	root := withTempWorkspace(t)
	path := filepath.Join(root, "README.md")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := New().Run(context.Background(), "--- a/README.md\n+++ b/README.md\n@@ -1,1 +1,1 @@\n-old\n+new")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(updated) != "new\n" {
		t.Fatalf("unexpected unified diff result %q", string(updated))
	}
	if !strings.Contains(result.Summary, "Unified diff applied") {
		t.Fatalf("unexpected summary %q", result.Summary)
	}
}

func TestPatchToolAppliesASTPatch(t *testing.T) {
	root := withTempWorkspace(t)
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc One() int {\n\treturn 1\n}\n\nfunc Two() int {\n\treturn 2\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := New().Run(context.Background(), "main.go\n<<<<<<< AST\ncapture: target\nindex: 2\nquery:\n(function_declaration body: (block) @target)\n=======\n{\n\treturn 9\n}\n>>>>>>> REPLACE")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "func Two() int {\n\treturn 9\n}") {
		t.Fatalf("expected AST patch replacement, got %q", string(updated))
	}
	if !strings.Contains(result.Summary, "AST patch applied") {
		t.Fatalf("unexpected summary %q", result.Summary)
	}
}

func TestPatchToolRejectsUnsafeWrites(t *testing.T) {
	_ = withTempWorkspace(t)

	unsafePaths := []string{
		".git/hooks/pre-commit",
		".git/config",
		".env",
		".env.local",
		".ssh/id_rsa",
		"id_rsa",
		".antigravitycli/settings.json",
	}

	for _, unsafePath := range unsafePaths {
		t.Run(unsafePath, func(t *testing.T) {
			// Try to run a patch on an unsafe path
			_, err := New().Run(context.Background(), unsafePath+"\n<<<<<<< SEARCH\n=======\nmalicious content\n>>>>>>> REPLACE")
			if err == nil {
				t.Fatalf("expected error for write to unsafe path: %s", unsafePath)
			}
			if !strings.Contains(err.Error(), "write blocked") {
				t.Fatalf("expected sandbox error, got: %v", err)
			}
		})
	}
}

func TestParseASTSelectorErrorsIncludeFormatHints(t *testing.T) {
	tests := []struct {
		name    string
		block   string
		wantErr string
	}{
		{
			name:    "invalid line without colon",
			block:   "function parseSizeMeters(sizeStr) {",
			wantErr: "invalid AST line",
		},
		{
			name:    "invalid line with format hint",
			block:   "just some text",
			wantErr: "key: value",
		},
		{
			name:    "unsupported key lists valid keys",
			block:   "unknownkey: value",
			wantErr: "unsupported AST key",
		},
		{
			name:    "unsupported key lists valid keys explicitly",
			block:   "unknownkey: value",
			wantErr: "valid keys:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseASTSelector(tt.block)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q missing expected substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestParseASTPatchInputErrorMentionsValidMarkers(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "missing divider mentions exact match",
			body: "<<<<<<< AST\ntype: function_declaration\n>>>>>>>> REPLACE",
			want: "EXACTLY",
		},
		{
			name: "wrong start marker lists options",
			body: "<<<<<<< INVALID\ntype: function_declaration\n=======\ncode\n>>>>>>> REPLACE",
			want: "<<<<<<< AST, <<<<<<< SEARCH, or use unified diff",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseASTPatchInput("test.js", tt.body)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q missing expected substring %q", err.Error(), tt.want)
			}
		})
	}
}
