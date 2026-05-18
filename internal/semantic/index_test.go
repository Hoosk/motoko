package semantic

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	snapshot := Snapshot{Files: []FileSummary{
		{Path: "internal/ui/model.go", Language: "go", Symbols: []Symbol{{Name: "NewModel", Kind: "func"}, {Name: "Update", Kind: "method"}}},
		{Path: "internal/app/runtime.go", Language: "go", Changed: true, Symbols: []Symbol{{Name: "RunAgent", Kind: "func"}, {Name: "HandleInput", Kind: "func"}}},
		{Path: "internal/provider/provider.go", Language: "go", Symbols: []Symbol{{Name: "ListModels", Kind: "func"}}},
	}}

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
	snapshot := Snapshot{Files: []FileSummary{{
		Path:     "internal/app/runtime.go",
		Language: "go",
		Changed:  true,
		Content:  content,
		Symbols: []Symbol{
			{Name: "Runtime", Kind: "type", Line: 3, Range: LineRange{Start: 3, End: 3}},
			{Name: "NewRuntime", Kind: "func", Line: 5, Range: LineRange{Start: 5, End: 7}},
			{Name: "RunAgent", Kind: "func", Line: 9, Range: LineRange{Start: 9, End: 11}},
		},
	}}}

	snippets := snapshot.RelevantSnippets("quiero revisar runagent", 2, 40)
	if len(snippets) == 0 {
		t.Fatal("expected snippets")
	}
	if !strings.Contains(snippets[0].Content, "func RunAgent") {
		t.Fatalf("expected RunAgent content, got %q", snippets[0].Content)
	}
}
