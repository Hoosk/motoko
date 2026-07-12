package scheduleman

import (
	"context"
	"testing"
	"time"
)

func TestAddAndFireOneShotSchedule(t *testing.T) {
	m := NewManager()
	ctx := t.Context()
	m.AttachContext(ctx)

	def, err := m.Add("run once", 20*time.Millisecond, true)
	if err != nil {
		t.Fatal(err)
	}
	if def.ID == "" {
		t.Fatal("expected schedule id")
	}

	res := m.Next(context.Background())
	if !res.OK {
		t.Fatal("expected schedule event")
	}
	if res.Event.Instruction != "run once" {
		t.Fatalf("unexpected instruction: %q", res.Event.Instruction)
	}

	if got := len(m.List()); got != 0 {
		t.Fatalf("expected one-shot schedule removed after fire, got %d entries", got)
	}
}

func TestRestoreSchedules(t *testing.T) {
	m := NewManager()
	ctx := t.Context()
	m.AttachContext(ctx)

	m.Restore([]Definition{{ID: "sched-8", Instruction: "tick", Interval: 10 * time.Millisecond}})
	list := m.List()
	if len(list) != 1 {
		t.Fatalf("expected one restored schedule, got %d", len(list))
	}
	if list[0].ID != "sched-8" {
		t.Fatalf("unexpected restored id: %s", list[0].ID)
	}
}
