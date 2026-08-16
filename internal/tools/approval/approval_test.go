package approval

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestBrokerAutoModeDoesNotBlock(t *testing.T) {
	broker := NewBroker()
	ctx := WithMode(context.Background(), ModeAuto)

	if err := broker.Request(ctx, FileChange{Path: "main.go", Diff: "+new"}); err != nil {
		t.Fatalf("unexpected auto-mode error: %v", err)
	}
}

func TestBrokerResolvesApproval(t *testing.T) {
	broker := NewBroker()
	ctx := WithMode(context.Background(), ModeAsk)
	result := make(chan error, 1)

	go func() {
		result <- broker.Request(ctx, FileChange{Path: "main.go", Diff: "+new"})
	}()

	pending, err := broker.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Change.Path != "main.go" || !strings.Contains(pending.Change.Diff, "+new") {
		t.Fatalf("unexpected pending change %#v", pending.Change)
	}
	pending.Resolve(true)

	if err := <-result; err != nil {
		t.Fatalf("approved change returned an error: %v", err)
	}
}

func TestBrokerRejectedApprovalReturnsSentinel(t *testing.T) {
	broker := NewBroker()
	ctx := WithMode(context.Background(), ModeAsk)
	result := make(chan error, 1)

	go func() {
		result <- broker.Request(ctx, FileChange{Path: "main.go", Diff: "+new"})
	}()

	pending, err := broker.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pending.Resolve(false)

	err = <-result
	if !errors.Is(err, ErrChangeRejected) {
		t.Fatalf("expected ErrChangeRejected, got %v", err)
	}
	if !strings.Contains(err.Error(), "main.go") {
		t.Fatalf("expected rejected path in error, got %v", err)
	}
}

func TestBrokerRequestHonorsContextCancellation(t *testing.T) {
	broker := NewBroker()
	ctx, cancel := context.WithCancel(WithMode(context.Background(), ModeAsk))
	result := make(chan error, 1)

	go func() {
		result <- broker.Request(ctx, FileChange{Path: "main.go", Diff: "+new"})
	}()
	if _, err := broker.Next(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()

	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}
