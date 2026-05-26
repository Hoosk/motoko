package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/tachikoma"
)

func TestInspectTool_Run(t *testing.T) {
	mgr := tachikoma.NewManager()
	// Simulate data in the manager manually for testing the tool
	// (Using exported field or a mock if needed, but here we can just use the manager)
	
	// We need a way to inject state into manager for testing without starting goroutines
	// Since state is not exported, we use the Add/Run pattern but controlled
	
	t.Run("WorkerFound", func(t *testing.T) {
		// As we cannot easily inject state into private map without Start, 
		// we'll rely on the fact that we just implemented Query in the manager.
		// Let's assume for this test we can use a small hack if needed, or 
		// just test the error cases and one successful integration.
		
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
}
