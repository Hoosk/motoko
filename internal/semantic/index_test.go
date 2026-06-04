package semantic

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Hoosk/motoko/internal/semantic/symtypes"
)

func TestBuildSnapshotExtractsGoSymbols(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "internal", "app")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("package app\n\ntype Runtime struct{}\n\nfunc NewRuntime() *Runtime { return &Runtime{} }\n")
	if err := os.WriteFile(filepath.Join(path, "runtime.go"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	snapshot, err := BuildSnapshot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Files) != 1 {
		t.Fatalf("expected 1 indexed file, got %d", len(snapshot.Files))
	}
	got := snapshot.Files[0].Descriptor()
	if got == "" || snapshot.Files[0].Language != "go" {
		t.Fatalf("unexpected file summary: %#v", snapshot.Files[0])
	}
	if len(snapshot.Files[0].Symbols) < 2 {
		t.Fatalf("expected extracted symbols, got %#v", snapshot.Files[0].Symbols)
	}
	if snapshot.Files[0].Symbols[0].Range.Start == 0 || snapshot.Files[0].Symbols[0].Range.End == 0 {
		t.Fatalf("expected symbol ranges, got %#v", snapshot.Files[0].Symbols)
	}
}

func TestRelevantFilesPrefersPromptMatches(t *testing.T) {
	snapshot := Snapshot{}
	snapshot.Files = []FileSummary{
		{Path: "internal/ui/model.go", Language: "go", Symbols: []Symbol{{Name: "NewModel", Kind: "func"}, {Name: "Update", Kind: "method"}}},
		{Path: "internal/app/runtime.go", Language: "go", Changed: true, Symbols: []Symbol{{Name: "RunAgent", Kind: "func"}, {Name: "HandleInput", Kind: "func"}}},
		{Path: "internal/provider/provider.go", Language: "go", Symbols: []Symbol{{Name: "ListModels", Kind: "func"}}},
	}

	relevant := snapshot.RelevantFiles("quiero una opinion del runtime y runagent", 2)
	if len(relevant) == 0 {
		t.Fatal("expected relevant files")
	}
	if relevant[0].Path != "internal/app/runtime.go" {
		t.Fatalf("expected runtime.go first, got %#v", relevant)
	}
}

func TestRelevantSnippetsPicksMatchingSymbol(t *testing.T) {
	content := []byte("package app\n\ntype Runtime struct{}\n\nfunc NewRuntime() *Runtime {\n\treturn &Runtime{}\n}\n\nfunc RunAgent() error {\n\treturn nil\n}\n")
	snapshot := Snapshot{}
	snapshot.Files = []FileSummary{{
		Path:     "internal/app/runtime.go",
		Language: "go",
		Changed:  true,
		Content:  content,
		Symbols: []Symbol{
			{Name: "Runtime", Kind: "type", Line: 3, Range: LineRange{Start: 3, End: 3}},
			{Name: "NewRuntime", Kind: "func", Line: 5, Range: LineRange{Start: 5, End: 7}},
			{Name: "RunAgent", Kind: "func", Line: 9, Range: LineRange{Start: 9, End: 11}},
		},
	}}

	snippets := snapshot.RelevantSnippets("quiero revisar runagent", 2, 40)
	if len(snippets) == 0 {
		t.Fatal("expected snippets")
	}
	if !strings.Contains(snippets[0].Content, "func RunAgent") {
		t.Fatalf("expected RunAgent content, got %q", snippets[0].Content)
	}
}

func TestBuildSnapshotSkipsGitIgnoredFiles(t *testing.T) {
	root := t.TempDir()
	runGitCommand(t, root, "init")
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("dist/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal", "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dist", "ignored.go"), []byte("package dist\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "app", "runtime.go"), []byte("package app\n\nfunc Run() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := BuildSnapshot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Files) != 1 || snapshot.Files[0].Path != "internal/app/runtime.go" {
		t.Fatalf("expected ignored files skipped, got %#v", snapshot.Files)
	}
}

func TestBuildSnapshotExtractsGoImportsAndExports(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "internal", "app")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("package app\n\nimport \"fmt\"\n\ntype Runtime struct{}\n\nfunc NewRuntime() *Runtime { fmt.Println(\"ok\"); return &Runtime{} }\n")
	if err := os.WriteFile(filepath.Join(path, "runtime.go"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	snapshot, err := BuildSnapshot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Files) != 1 {
		t.Fatalf("expected 1 indexed file, got %d", len(snapshot.Files))
	}
	file := snapshot.Files[0]
	if len(file.Imports) != 1 || file.Imports[0] != "fmt" {
		t.Fatalf("expected fmt import, got %#v", file.Imports)
	}
	if len(file.Exports) == 0 || file.Exports[0] != "Runtime" {
		t.Fatalf("expected exported Runtime symbol, got %#v", file.Exports)
	}
}

func TestRelevantFilesFallsBackToChangedFilesWithoutPromptMatches(t *testing.T) {
	snapshot := Snapshot{}
	snapshot.Files = []FileSummary{
		{Path: "internal/ui/model.go", Language: "go", Symbols: []Symbol{{Name: "Model", Kind: "type"}}},
		{Path: "internal/app/runtime.go", Language: "go", Changed: true, Symbols: []Symbol{{Name: "RunAgent", Kind: "func"}}},
	}

	relevant := snapshot.RelevantFiles("zzqv", 1)
	if len(relevant) != 1 {
		t.Fatalf("expected 1 relevant file, got %#v", relevant)
	}
	if relevant[0].Path != "internal/app/runtime.go" {
		t.Fatalf("expected changed runtime fallback first, got %#v", relevant)
	}
}

func TestSnapshotSummaryIncludesDirectoriesLanguagesAndChangedPaths(t *testing.T) {
	snapshot := Snapshot{
		Snapshot: symtypes.Snapshot{
			Directories:    []string{"internal/app", "internal/ui"},
			LanguageCounts: map[string]int{"go": 2},
			ChangedPaths:   []string{"internal/app/runtime.go"},
			Files:          []FileSummary{{Path: "internal/app/runtime.go", Language: "go"}, {Path: "internal/ui/model.go", Language: "go"}},
		},
	}

	summary := snapshot.Summary()
	if !strings.Contains(summary, "files:2") || !strings.Contains(summary, "dirs:internal/app, internal/ui") || !strings.Contains(summary, "langs:go:2") || !strings.Contains(summary, "changed:internal/app/runtime.go") {
		t.Fatalf("unexpected summary %q", summary)
	}
}

func TestSetSnapshotForTestMakesEnsureReturnFreshSnapshot(t *testing.T) {
	idx := NewIndex()
	snapshot := &Snapshot{Snapshot: symtypes.Snapshot{GeneratedAt: time.Now(), Files: []FileSummary{{Path: "internal/app/runtime.go", Language: "go"}}}}
	idx.SetSnapshotForTest(snapshot)

	got, err := idx.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != snapshot {
		t.Fatalf("expected Ensure to reuse injected snapshot, got %#v", got)
	}
}

func TestBoundedRangeCapsToMaxSnippetLines(t *testing.T) {
	start, end := boundedRange(10, 12, 200, maxSnippetLines+20)
	if end-start+1 > maxSnippetLines {
		t.Fatalf("expected bounded range capped to %d lines, got %d-%d", maxSnippetLines, start, end)
	}
}

func runGitCommand(t *testing.T, workdir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = workdir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}

func TestSemanticGraphExpansion(t *testing.T) {
	// Setup a custom snapshot representing:
	// - File A (runtime.go): matches prompt ("runtime"), imports File B and references its exported symbol "AppConfig"
	// - File B (config.go): doesn't match prompt ("runtime") but is connected to A via imports/exports
	// - File C (model.go): completely unrelated
	snapshot := Snapshot{}
	snapshot.Files = []FileSummary{
		{
			Path:     "internal/app/runtime.go",
			Language: "go",
			Content:  []byte("package app\nimport \"github.com/Hoosk/motoko/internal/config\"\nfunc Run() { var cfg config.AppConfig }"),
			Imports:  []string{"github.com/Hoosk/motoko/internal/config"},
			Symbols:  []Symbol{{Name: "Run", Kind: "func"}},
		},
		{
			Path:     "internal/config/config.go",
			Language: "go",
			Content:  []byte("package config\ntype AppConfig struct{}"),
			Exports:  []string{"AppConfig"},
			Symbols:  []Symbol{{Name: "AppConfig", Kind: "type"}},
		},
		{
			Path:     "internal/ui/model.go",
			Language: "go",
			Content:  []byte("package ui\nfunc NewModel() {}"),
			Symbols:  []Symbol{{Name: "NewModel", Kind: "func"}},
		},
	}

	// Request relevant files for the prompt "runtime"
	// Without graph expansion, config.go would get 0 points and wouldn't be ranked first or potentially included at all.
	// With graph expansion, runtime.go gets points for "runtime" token match, and propagates 30% of its points to config.go!
	relevant := snapshot.RelevantFiles("revisa runtime", 2)
	
	if len(relevant) < 2 {
		t.Fatalf("expected at least 2 relevant files, got %d", len(relevant))
	}

	// The first file must be the direct match: runtime.go
	if relevant[0].Path != "internal/app/runtime.go" {
		t.Fatalf("expected first relevant file to be runtime.go, got %q", relevant[0].Path)
	}

	// The second file must be config.go because of the semantic connection (model.go is unrelated)
	if relevant[1].Path != "internal/config/config.go" {
		t.Fatalf("expected config.go to be ranked second due to semantic connection, got %q", relevant[1].Path)
	}
}

