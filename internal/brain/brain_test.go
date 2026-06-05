package brain

import (
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/session"
)

func TestBrain(t *testing.T) {
	tmpDir := t.TempDir()
	prev := session.SessionsBaseDir
	session.SessionsBaseDir = tmpDir
	t.Cleanup(func() {
		session.SessionsBaseDir = prev
	})

	b, err := New("workspace123", "session456")
	if err != nil {
		t.Fatalf("failed to create brain: %v", err)
	}

	// 1. Exists & Read before write
	if b.Exists("plan") {
		t.Error("plan should not exist yet")
	}
	_, err = b.Read("plan")
	if err == nil {
		t.Error("reading non-existent file should return error")
	}

	// 2. Write
	err = b.Write("plan", "My first implementation plan")
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// 3. Exists & Read after write
	if !b.Exists("plan") {
		t.Error("plan should exist after write")
	}
	if !b.Exists("plan.md") {
		t.Error("plan.md should exist after write")
	}

	content, err := b.Read("plan")
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if content != "My first implementation plan" {
		t.Errorf("got %q; want %q", content, "My first implementation plan")
	}

	// 4. List
	files, err := b.List()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files; want 1", len(files))
	}
	if files[0].Name != "plan.md" {
		t.Errorf("got name %q; want %q", files[0].Name, "plan.md")
	}

	// 5. Write another file
	err = b.Write("tasks.md", "Task 1: do something")
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// 6. Summary, PlanSummary, TasksSummary
	summary := b.Summary()
	if !strings.Contains(summary, "plan.md") || !strings.Contains(summary, "tasks.md") {
		t.Errorf("summary doesn't contain plan.md or tasks.md: %q", summary)
	}

	planSum := b.PlanSummary()
	if planSum != "My first implementation plan" {
		t.Errorf("plan summary mismatch: %q", planSum)
	}

	tasksSum := b.TasksSummary()
	if tasksSum != "Task 1: do something" {
		t.Errorf("tasks summary mismatch: %q", tasksSum)
	}

	// 7. Invalid name protection
	err = b.Write("../outside", "malicious")
	if err == nil {
		t.Error("expected error writing outside directory")
	}

	err = b.Write("notes..md", "safe")
	if err != nil {
		t.Errorf("expected notes..md to be valid, got error: %v", err)
	}

	// 8. Delete
	err = b.Delete("plan")
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if b.Exists("plan") {
		t.Error("plan should not exist after delete")
	}
}

func TestBrainTruncation(t *testing.T) {
	tmpDir := t.TempDir()
	prev := session.SessionsBaseDir
	session.SessionsBaseDir = tmpDir
	t.Cleanup(func() {
		session.SessionsBaseDir = prev
	})

	b, _ := New("w", "s")

	longPlan := strings.Repeat("P", 2000)
	_ = b.Write("plan.md", longPlan)

	planSum := b.PlanSummary()
	if !strings.HasSuffix(planSum, "... [plan.md truncated, use brain_read to view full plan] ...") {
		t.Errorf("expected plan summary to be truncated, got length %d", len(planSum))
	}
	if len(planSum) != 1500+len("\n... [plan.md truncated, use brain_read to view full plan] ...") {
		t.Errorf("unexpected length for truncated plan: %d", len(planSum))
	}

	longTasks := strings.Repeat("T", 1500)
	_ = b.Write("tasks.md", longTasks)

	tasksSum := b.TasksSummary()
	if !strings.HasSuffix(tasksSum, "... [tasks.md truncated, use brain_read to view full tasks list] ...") {
		t.Errorf("expected tasks summary to be truncated, got length %d", len(tasksSum))
	}
	if len(tasksSum) != 1000+len("\n... [tasks.md truncated, use brain_read to view full tasks list] ...") {
		t.Errorf("unexpected length for truncated tasks: %d", len(tasksSum))
	}
}
