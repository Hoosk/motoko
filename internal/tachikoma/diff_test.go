package tachikoma

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Hoosk/motoko/internal/semantic"
	"github.com/Hoosk/motoko/internal/semantic/symtypes"
)

func TestDiffTachikoma_IdentifyChanges(t *testing.T) {
	// Setup a mock snapshot
	snapshot := &semantic.Snapshot{
		Snapshot: symtypes.Snapshot{
			Files: []semantic.FileSummary{
				{
					Path: "test.go",
					Symbols: []semantic.Symbol{
						{
							Name:  "FuncA",
							Kind:  "function",
							Range: semantic.LineRange{Start: 10, End: 20},
						},
						{
							Name:  "FuncB",
							Kind:  "function",
							Range: semantic.LineRange{Start: 30, End: 40},
						},
					},
				},
			},
		},
	}

	dt := &DiffTachikoma{}

	t.Run("ChangeInsideFunction", func(t *testing.T) {
		result := SemanticDiff{Files: make(map[string][]SymbolChange)}
		dt.identifyChangesInFile(snapshot, "test.go", 15, 1, &result)

		changes, ok := result.Files["test.go"]
		if !ok || len(changes) != 1 {
			t.Fatalf("Expected 1 change in test.go, got %d", len(changes))
		}
		if changes[0].Name != "FuncA" {
			t.Errorf("Expected FuncA, got %s", changes[0].Name)
		}
	})

	t.Run("ChangeOutsideFunctions", func(t *testing.T) {
		result := SemanticDiff{Files: make(map[string][]SymbolChange)}
		dt.identifyChangesInFile(snapshot, "test.go", 5, 1, &result)

		if len(result.Files["test.go"]) != 0 {
			t.Errorf("Expected 0 changes, got %d", len(result.Files["test.go"]))
		}
	})

	t.Run("SpanMultipleFunctions", func(t *testing.T) {
		result := SemanticDiff{Files: make(map[string][]SymbolChange)}
		// Change spans from 15 to 35, covering both FuncA and FuncB
		dt.identifyChangesInFile(snapshot, "test.go", 15, 20, &result)

		changes := result.Files["test.go"]
		if len(changes) != 2 {
			t.Fatalf("Expected 2 changes, got %d", len(changes))
		}

		foundA, foundB := false, false
		for _, c := range changes {
			if c.Name == "FuncA" {
				foundA = true
			}
			if c.Name == "FuncB" {
				foundB = true
			}
		}
		if !foundA || !foundB {
			t.Error("Did not find both functions covered by the span")
		}
	})
}

func TestDiffTachikomaComputeSemanticDiffIgnoresUserConfig(t *testing.T) {
	root := newWatchTestRepo(t)
	src := "package src\n\nfunc FuncA() int {\n\treturn 1\n}\n\nfunc FuncB() int {\n\treturn 2\n}\n"
	writeWatchTestFile(t, filepath.Join(root, "src", "pkg.go"), src)
	runWatchTestGit(t, root, "add", ".")
	runWatchTestGit(t, root, "commit", "-qm", "add pkg")

	// Settings that broke the old "+++ b/" parsing.
	runWatchTestGit(t, root, "config", "diff.noprefix", "true")
	runWatchTestGit(t, root, "config", "color.diff", "always")

	// Staged-only change inside FuncB, seen from a subdirectory.
	writeWatchTestFile(t, filepath.Join(root, "src", "pkg.go"), strings.Replace(src, "return 2", "return 3", 1))
	runWatchTestGit(t, root, "add", "src/pkg.go")
	t.Chdir(filepath.Join(root, "src"))

	snapshot, err := semantic.BuildSnapshot(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	index := semantic.NewIndex()
	index.SetSnapshotForTest(snapshot)

	diff, err := NewDiffTachikoma(index, time.Minute).computeSemanticDiff(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	changes := diff.Files["pkg.go"]
	if len(changes) != 1 || changes[0].Name != "FuncB" {
		t.Fatalf("expected FuncB change in pkg.go, got %#v", diff.Files)
	}
}
