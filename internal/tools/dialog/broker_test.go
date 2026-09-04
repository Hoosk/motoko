package dialog

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
)

func TestBrokerAutoModeDoesNotBlockFileChanges(t *testing.T) {
	broker := NewBroker()
	ctx := WithMode(context.Background(), ModeAuto)

	if err := broker.RequestFileChange(ctx, FileChange{Path: "main.go", Diff: "+new"}); err != nil {
		t.Fatalf("unexpected auto-mode error: %v", err)
	}
	if got := broker.PendingCount(); got != 0 {
		t.Fatalf("expected no pending dialogs, got %d", got)
	}
}

func TestBrokerResolvesFileChange(t *testing.T) {
	broker := NewBroker()
	ctx := WithMode(context.Background(), ModeAsk)
	result := make(chan error, 1)

	go func() {
		result <- broker.RequestFileChange(ctx, FileChange{Path: "main.go", Diff: "+new"})
	}()

	pending, err := broker.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Kind != KindFileChange || pending.Change.Path != "main.go" || !strings.Contains(pending.Change.Diff, "+new") {
		t.Fatalf("unexpected pending change %#v", pending)
	}
	pending.Resolve(Decision{Approved: true})

	if err := <-result; err != nil {
		t.Fatalf("approved change returned an error: %v", err)
	}
}

func TestBrokerRejectedFileChangeReturnsSentinel(t *testing.T) {
	broker := NewBroker()
	ctx := WithMode(context.Background(), ModeAsk)
	result := make(chan error, 1)

	go func() {
		result <- broker.RequestFileChange(ctx, FileChange{Path: "main.go", Diff: "+new"})
	}()

	pending, err := broker.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pending.Resolve(Decision{Approved: false})

	err = <-result
	if !errors.Is(err, ErrChangeRejected) {
		t.Fatalf("expected ErrChangeRejected, got %v", err)
	}
	if !strings.Contains(err.Error(), "main.go") {
		t.Fatalf("expected rejected path in error, got %v", err)
	}
}

func TestBrokerResolvesQuestion(t *testing.T) {
	broker := NewBroker()
	result := make(chan Answer, 1)
	errResult := make(chan error, 1)

	go func() {
		answer, err := broker.Ask(context.Background(), Question{
			Header:   "Decision",
			Question: "Continue?",
			Options:  []QuestionOption{{Label: "yes"}},
		})
		result <- answer
		errResult <- err
	}()

	pending, err := broker.Next(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if pending.Kind != KindQuestion || pending.Question.Question != "Continue?" {
		t.Fatalf("unexpected pending question %#v", pending)
	}
	pending.Resolve(Decision{Answer: Answer{Selections: []string{"yes"}}})

	if err := <-errResult; err != nil {
		t.Fatalf("answered question returned an error: %v", err)
	}
	if answer := <-result; len(answer.Selections) != 1 || answer.Selections[0] != "yes" {
		t.Fatalf("unexpected answer %#v", answer)
	}
}

func TestBrokerRejectsShellCommand(t *testing.T) {
	broker := NewBroker()
	result := make(chan error, 1)

	go func() {
		result <- broker.RequestShellCommand(context.Background(), ShellCommand{
			Command: "git add file.go",
			Reason:  "The command may modify files or repository state.",
		})
	}()

	pending, err := broker.Next(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if pending.Kind != KindShellCommand || pending.ShellCommand.Command != "git add file.go" {
		t.Fatalf("unexpected pending shell command %#v", pending)
	}
	pending.Resolve(Decision{Approved: false})

	err = <-result
	if !errors.Is(err, ErrCommandRejected) {
		t.Fatalf("expected ErrCommandRejected, got %v", err)
	}
	if broker.PendingCount() != 0 {
		t.Fatalf("expected no pending dialogs after rejection, got %d", broker.PendingCount())
	}
}

func TestBrokerCancellationReturnsQuestionSentinel(t *testing.T) {
	broker := NewBroker()
	result := make(chan error, 1)

	go func() {
		_, err := broker.Ask(context.Background(), Question{Question: "Continue?", Options: []QuestionOption{{Label: "yes"}}})
		result <- err
	}()
	pending, err := broker.Next(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pending.Resolve(Decision{Answer: Answer{Cancelled: true}})

	if err := <-result; !errors.Is(err, ErrQuestionCancelled) {
		t.Fatalf("expected ErrQuestionCancelled, got %v", err)
	}
}

func TestBrokerPreservesFIFOAcrossKinds(t *testing.T) {
	broker := NewBroker()
	askResult := make(chan error, 1)
	changeResult := make(chan error, 1)
	shellResult := make(chan error, 1)

	go func() {
		_, err := broker.Ask(context.Background(), Question{Question: "First", Options: []QuestionOption{{Label: "ok"}}})
		askResult <- err
	}()
	first, err := broker.Next(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.Kind != KindQuestion {
		t.Fatalf("expected question first, got %s", first.Kind)
	}

	ctx := WithMode(context.Background(), ModeAsk)
	go func() {
		changeResult <- broker.RequestFileChange(ctx, FileChange{Path: "one.go", Diff: "+one"})
	}()
	for i := 0; i < 10_000 && broker.PendingCount() < 2; i++ {
		runtime.Gosched()
	}
	if broker.PendingCount() < 2 {
		t.Fatal("file change was not queued")
	}
	go func() {
		shellResult <- broker.RequestShellCommand(context.Background(), ShellCommand{Command: "go test", Reason: "test"})
	}()
	for i := 0; i < 10_000 && broker.PendingCount() < 3; i++ {
		runtime.Gosched()
	}
	if broker.PendingCount() < 3 {
		t.Fatal("shell command was not queued")
	}

	first.Resolve(Decision{Answer: Answer{Selections: []string{"ok"}}})
	if err := <-askResult; err != nil {
		t.Fatalf("question failed: %v", err)
	}

	second, err := broker.Next(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.Kind != KindFileChange {
		t.Fatalf("expected file change second, got %s", second.Kind)
	}
	second.Resolve(Decision{Approved: true})

	third, err := broker.Next(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if third.Kind != KindShellCommand {
		t.Fatalf("expected shell command third, got %s", third.Kind)
	}
	third.Resolve(Decision{Approved: true})

	if err := <-changeResult; err != nil {
		t.Fatalf("file change failed: %v", err)
	}
	if err := <-shellResult; err != nil {
		t.Fatalf("shell command failed: %v", err)
	}
}

func TestBrokerRequestHonorsContextCancellation(t *testing.T) {
	broker := NewBroker()
	ctx, cancel := context.WithCancel(WithMode(context.Background(), ModeAsk))
	result := make(chan error, 1)

	go func() {
		result <- broker.RequestFileChange(ctx, FileChange{Path: "main.go", Diff: "+new"})
	}()
	if _, err := broker.Next(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()

	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if got := broker.PendingCount(); got != 0 {
		t.Fatalf("expected cancelled dialog to be removed, got %d pending", got)
	}
}

func TestBrokerAskModeRequiresBroker(t *testing.T) {
	ctx := WithMode(context.Background(), ModeAsk)
	var broker *Broker

	if err := broker.RequestFileChange(ctx, FileChange{Path: "main.go", Diff: "+new"}); !errors.Is(err, ErrBrokerUnavailable) {
		t.Fatalf("expected missing broker error, got %v", err)
	}
}

func TestBrokerCancellationRemovesQueuedRequest(t *testing.T) {
	broker := NewBroker()
	ctx, cancel := context.WithCancel(WithMode(context.Background(), ModeAsk))
	result := make(chan error, 1)

	go func() {
		result <- broker.RequestFileChange(ctx, FileChange{Path: "cancelled.go", Diff: "+old"})
	}()

	queued := false
	for i := 0; i < 10_000; i++ {
		broker.mu.Lock()
		queued = len(broker.pending) > 0
		broker.mu.Unlock()
		if queued {
			break
		}
		runtime.Gosched()
	}
	if !queued {
		t.Fatal("request was not queued")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}

	ctx2 := WithMode(context.Background(), ModeAsk)
	result2 := make(chan error, 1)
	go func() {
		result2 <- broker.RequestFileChange(ctx2, FileChange{Path: "next.go", Diff: "+new"})
	}()
	pending, err := broker.Next(ctx2)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Change.Path != "next.go" {
		t.Fatalf("expected next request, got %q", pending.Change.Path)
	}
	pending.Resolve(Decision{Approved: true})
	if err := <-result2; err != nil {
		t.Fatalf("next request failed: %v", err)
	}
}
