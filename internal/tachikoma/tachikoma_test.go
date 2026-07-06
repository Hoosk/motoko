package tachikoma

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Hoosk/motoko/internal/semantic"
)

type fakeTachikoma struct {
	name string
}

func (f fakeTachikoma) Name() string { return f.name }
func (f fakeTachikoma) Run(ctx context.Context, publish func(Update) bool) error {
	publish(Update{Name: f.name, Status: "ok", Done: true})
	return nil
}

func TestManagerStartPublishesUpdates(t *testing.T) {
	mgr := NewManager()
	mgr.Add(fakeTachikoma{name: "one"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)

	select {
	case update := <-mgr.Updates():
		if update.Name != "one" || update.Status != "ok" {
			t.Fatalf("unexpected update %#v", update)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for tachikoma update")
	}
}

func TestWorkspaceGitAndCodeTachikomasCancelQuickly(t *testing.T) {
	publish := func(Update) bool { return true }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := NewWorkspaceTachikoma(time.Millisecond).Run(ctx, publish); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled workspace tachikoma, got %v", err)
	}
	if err := NewGitTachikoma(time.Millisecond).Run(ctx, publish); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled git tachikoma, got %v", err)
	}
	if err := NewCodeTachikoma(semantic.NewIndex(), time.Millisecond).Run(ctx, publish); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled code tachikoma, got %v", err)
	}
}

func TestManagerWaitReturnsAfterCancel(t *testing.T) {
	mgr := NewManager()
	mgr.Add(NewMockTachikoma("x"))
	ctx, cancel := context.WithCancel(context.Background())
	mgr.Start(ctx)
	time.Sleep(10 * time.Millisecond)
	cancel()
	done := make(chan struct{})
	go func() {
		mgr.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for tachikoma manager")
	}
}

func TestManagerDropsUpdatesWhenBufferIsFull(t *testing.T) {
	mgr := NewManager()
	for i := 0; i < updatesBufferSize; i++ {
		if !mgr.publishUpdate(Update{Name: "x", Status: "ok"}) {
			t.Fatalf("expected buffered publish to succeed at %d", i)
		}
	}
	if mgr.publishUpdate(Update{Name: "overflow", Status: "dropped"}) {
		t.Fatal("expected publish to drop when buffer is full")
	}
}

func TestManagerNextRespectsCancellation(t *testing.T) {
	mgr := NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := mgr.Next(ctx)
	if result.OK {
		t.Fatal("expected canceled next call to stop without update")
	}
}

func TestManagerNextReturnsPublishedUpdate(t *testing.T) {
	mgr := NewManager()
	if !mgr.publishUpdate(Update{Name: "one", Status: "ok"}) {
		t.Fatal("expected publish to succeed")
	}
	result := mgr.Next(context.Background())
	if !result.OK {
		t.Fatal("expected next call to return update")
	}
	if result.Update.Name != "one" || result.Update.Status != "ok" {
		t.Fatalf("unexpected next result %#v", result)
	}
}

func TestMockTachikomaName(t *testing.T) {
	if got := NewMockTachikoma("x").Name(); got != "x" {
		t.Fatalf("unexpected mock tachikoma name %q", got)
	}
}

func TestGetContextInfoCodeTachikomaHintUsesInspect(t *testing.T) {
	mgr := NewManager()

	langCounts := make(map[string]int)
	for i := 0; i < 20; i++ {
		langCounts["verylonglanguagenamethatforceslargesummary_"+string(rune('a'+i))] = 1000 + i
	}

	snapshot := &semantic.Snapshot{}
	snapshot.Files = []semantic.FileSummary{{Path: "main.go", Language: "go"}}
	snapshot.LanguageCounts = langCounts

	u := Update{
		Name:    "CodeTachikoma",
		Status:  "ready",
		Payload: snapshot,
	}
	mgr.mu.Lock()
	mgr.state[u.Name] = u
	mgr.mu.Unlock()

	info := mgr.GetContextInfo()

	if !strings.Contains(info.SemanticSummary, "inspect CodeTachikoma") {
		t.Fatalf("CodeTachikoma semantic summary should mention 'inspect CodeTachikoma', got: %q", info.SemanticSummary)
	}
	if strings.Contains(info.SemanticSummary, "Use search/read") {
		t.Fatalf("CodeTachikoma semantic summary should not mention old 'search/read' hint, got: %q", info.SemanticSummary)
	}
	if _, ok := info.OnDemandSignals["CodeTachikoma"]; !ok {
		t.Fatalf("expected CodeTachikoma on-demand signal")
	}
}
