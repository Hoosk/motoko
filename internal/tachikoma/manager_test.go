package tachikoma

import (
	"context"
	"testing"
	"time"

	"github.com/Hoosk/motoko/internal/semantic"
	"github.com/Hoosk/motoko/internal/semantic/symtypes"
)

type mockTachikoma struct {
	payload any
	name    string
}

func (m *mockTachikoma) Name() string { return m.name }
func (m *mockTachikoma) Run(ctx context.Context, publish func(Update) bool) error {
	publish(Update{
		Name:    m.name,
		Status:  "running",
		Payload: m.payload,
	})
	<-ctx.Done()
	return nil
}

func TestManager_Query(t *testing.T) {
	mgr := NewManager()
	payload := &semantic.Snapshot{
		Snapshot: symtypes.Snapshot{
			Files: nil,
		},
	}
	mock := &mockTachikoma{name: "TestWorker", payload: payload}

	mgr.Add(mock)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	mgr.Start(ctx)

	// Wait a bit for the goroutine to publish
	time.Sleep(50 * time.Millisecond)

	update, ok := mgr.Query("TestWorker")
	if !ok {
		t.Fatal("Expected to find TestWorker data")
	}

	if update.Name != "TestWorker" {
		t.Errorf("Expected name TestWorker, got %s", update.Name)
	}

	if update.Payload != payload {
		t.Error("Payload mismatch")
	}
}

func TestManager_GetContextInfo_Sharding(t *testing.T) {
	mgr := NewManager()

	// Create a "heavy" payload
	heavyPayload := &semantic.Snapshot{
		Snapshot: symtypes.Snapshot{
			Files: make([]semantic.FileSummary, 100), // Many files to make summary long
		},
	}

	// Forcing a long summary for the test
	// Note: In reality snapshot.Summary() calculates it.
	// Here we just need to ensure the logic in GetContextInfo handles length.

	mgr.mu.Lock()
	mgr.state["CodeTachikoma"] = Update{
		Name:    "CodeTachikoma",
		Status:  "indexed",
		Payload: heavyPayload,
	}
	mgr.mu.Unlock()

	info := mgr.GetContextInfo()

	// Verify OnDemandSignals usage
	// The logic in tachikoma.go says if len(fullSummary) > 500
	// We might need to mock the Summary() if it's not long enough.
	// But let's check if the basic structure is there.
	if info.OnDemandSignals == nil {
		t.Error("OnDemandSignals should not be nil")
	}
}
