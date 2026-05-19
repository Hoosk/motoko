package tachikoma

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Hoosk/motoko/internal/semantic"
)

type fakeTachikoma struct {
	name string
}

func (f fakeTachikoma) Name() string { return f.name }
func (f fakeTachikoma) Run(ctx context.Context, updates chan<- Update) error {
	updates <- Update{Name: f.name, Status: "ok", Done: true}
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
	updates := make(chan Update, 4)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := NewWorkspaceTachikoma(time.Millisecond).Run(ctx, updates); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled workspace tachikoma, got %v", err)
	}
	if err := NewGitTachikoma(time.Millisecond).Run(ctx, updates); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled git tachikoma, got %v", err)
	}
	if err := NewCodeTachikoma(semantic.NewIndex(), time.Millisecond).Run(ctx, updates); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled code tachikoma, got %v", err)
	}
}

func TestMockTachikomaName(t *testing.T) {
	if got := NewMockTachikoma("x").Name(); got != "x" {
		t.Fatalf("unexpected mock tachikoma name %q", got)
	}
}
