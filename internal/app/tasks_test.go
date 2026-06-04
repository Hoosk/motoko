package app

import (
	"context"
	"testing"
	"time"
)

func TestTaskManagerLaunchAndNext(t *testing.T) {
	m := NewTaskManager()
	id, err := m.Launch(context.Background(), "printf hola")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected task id")
	}

	start := time.Now()
	var started, finished bool
	for time.Since(start) < 2*time.Second {
		res := m.Next(context.Background())
		if !res.OK {
			continue
		}
		if res.Event.Done {
			finished = true
			if res.Event.ExitCode != 0 {
				t.Fatalf("expected success exit code, got %#v", res.Event)
			}
		} else {
			started = true
		}
		if started && finished {
			return
		}
	}
	t.Fatal("expected start and finish events")
}

func TestTaskManagerTerminate(t *testing.T) {
	m := NewTaskManager()
	id, err := m.Launch(context.Background(), "sleep 10")
	if err != nil {
		t.Fatal(err)
	}

	// Verify task is in active list
	list := m.List()
	if len(list) != 1 || list[0].ID != id {
		t.Fatalf("expected 1 task with id %s, got %v", id, list)
	}

	// Terminate the task
	err = m.Terminate(id)
	if err != nil {
		t.Fatalf("failed to terminate task: %v", err)
	}

	// Verify it eventually finishes with Canceled status/exit code
	start := time.Now()
	for time.Since(start) < 2*time.Second {
		res := m.Next(context.Background())
		if res.OK && res.Event.Done {
			if res.Event.ExitCode != 130 {
				t.Fatalf("expected cancel exit code 130, got %d", res.Event.ExitCode)
			}
			return
		}
	}
	t.Fatal("expected task to terminate and emit done event")
}

