package tachikoma

import (
	"testing"

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
