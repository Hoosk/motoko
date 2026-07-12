package tachikoma

import (
	"testing"
	"time"

	"github.com/Hoosk/motoko/internal/semantic"
	"github.com/Hoosk/motoko/internal/semantic/symtypes"
)

func TestSearchTachikoma(t *testing.T) {
	index := semantic.NewIndex()
	snapshot := &semantic.Snapshot{
		Snapshot: symtypes.Snapshot{
			Files: []semantic.FileSummary{
				{
					Path:     "internal/app/runtime.go",
					Language: "go",
					Content:  []byte("package app\n\nfunc RunAgent() error {\n\treturn nil\n}"),
					Symbols: []semantic.Symbol{
						{
							Name: "RunAgent",
							Range: symtypes.LineRange{
								Start: 3,
								End:   5,
							},
						},
					},
				},
			},
		},
	}
	index.SetSnapshotForTest(snapshot)

	worker := NewSearchTachikoma(index)
	if worker.Name() != "SearchTachikoma" {
		t.Errorf("Expected SearchTachikoma name, got %s", worker.Name())
	}

	ctx := t.Context()

	updates := make(chan Update, 5)
	publish := func(u Update) bool {
		updates <- u
		return true
	}

	go func() {
		_ = worker.Run(ctx, publish)
	}()

	// Read initial idle status
	select {
	case u := <-updates:
		if u.Status != "search idle" {
			t.Errorf("expected initial status search idle, got %s", u.Status)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("timeout waiting for initial update")
	}

	// Trigger search
	worker.SetActivePrompt("RunAgent")

	select {
	case u := <-updates:
		if u.Status == "search idle" {
			// Read again if we read the idle state before search completed
			select {
			case u = <-updates:
			case <-time.After(50 * time.Millisecond):
			}
		}
		snippets, ok := u.Payload.([]semantic.Snippet)
		if !ok {
			t.Fatalf("expected payload to be []semantic.Snippet, got %T", u.Payload)
		}
		if len(snippets) == 0 {
			t.Fatal("expected at least one snippet to be returned")
		}
		if snippets[0].Path != "internal/app/runtime.go" {
			t.Errorf("expected path internal/app/runtime.go, got %s", snippets[0].Path)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout waiting for search results")
	}
}
