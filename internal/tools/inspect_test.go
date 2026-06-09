package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Hoosk/motoko/internal/tachikoma"
)

type mockDepTachikoma struct {
	payload tachikoma.ProjectDependencies
}

func (m *mockDepTachikoma) Name() string {
	return "DependencyTachikoma"
}

func (m *mockDepTachikoma) Run(ctx context.Context, publish func(tachikoma.Update) bool) error {
	publish(tachikoma.Update{
		Name:    "DependencyTachikoma",
		Status:  "Go: 1 deps | Rust: 1 deps",
		Payload: m.payload,
	})
	<-ctx.Done()
	return ctx.Err()
}

func TestInspectTool_Run(t *testing.T) {
	mgr := tachikoma.NewManager()

	t.Run("WorkerFound", func(t *testing.T) {
		tool := NewInspectTool(mgr)
		_, err := tool.Run(context.Background(), "NonExistent")
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("Expected error for non-existent worker, got %v", err)
		}
	})

	t.Run("EmptyArgs", func(t *testing.T) {
		tool := NewInspectTool(mgr)
		_, err := tool.Run(context.Background(), "")
		if err == nil || !strings.Contains(err.Error(), "usage") {
			t.Errorf("Expected usage error, got %v", err)
		}
	})

	t.Run("DependenciesFormatting", func(t *testing.T) {
		mgr := tachikoma.NewManager()
		deps := tachikoma.ProjectDependencies{
			Ecosystems: map[string][]string{
				"Go":   {"github.com/charmbracelet/bubbletea"},
				"Rust": {"serde"},
			},
		}

		mock := &mockDepTachikoma{payload: deps}
		mgr.Add(mock)

		ctx, cancel := context.WithCancel(context.Background())
		mgr.Start(ctx)

		var ok bool
		for i := 0; i < 50; i++ {
			_, ok = mgr.Query("DependencyTachikoma")
			if ok {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
		mgr.Wait()

		if !ok {
			t.Fatalf("Failed to initialize DependencyTachikoma state in manager")
		}

		tool := NewInspectTool(mgr)
		res, err := tool.Run(context.Background(), "DependencyTachikoma")
		if err != nil {
			t.Fatalf("Unexpected error running inspect tool: %v", err)
		}

		expectedContent := []string{
			"Worker: DependencyTachikoma",
			"Status: Go: 1 deps | Rust: 1 deps",
			"Detected Dependencies by Ecosystem:",
			"Go:",
			"- github.com/charmbracelet/bubbletea",
			"Rust:",
			"- serde",
		}

		for _, exp := range expectedContent {
			if !strings.Contains(res.Output, exp) {
				t.Errorf("Expected output to contain %q, but got:\n%s", exp, res.Output)
			}
		}
	})
}
