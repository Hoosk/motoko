package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Hoosk/motoko/internal/app"
	tea "github.com/charmbracelet/bubbletea"
)

func TestModelSubmitPromptSlashCommand(t *testing.T) {
	r := app.NewRuntime()
	m := NewModel(r)

	// Submit a SubmitPromptMsg with '/help'
	resModel, _ := m.Update(SubmitPromptMsg{Prompt: "/help"})
	updatedModel := resModel.(Model)

	// Check that we got Entries appended to the timeline representing the help text
	foundHelp := false
	for _, entry := range updatedModel.timeline.model.Entries {
		if entry.Kind == app.EntryHelp || strings.Contains(entry.Text, "Available commands:") {
			foundHelp = true
			break
		}
	}

	if !foundHelp {
		t.Fatalf("expected timeline to contain help info after submitting '/help'")
	}
}

func TestModelSubmitPromptExitCommand(t *testing.T) {
	r := app.NewRuntime()
	m := NewModel(r)

	// Submit /exit
	_, cmd := m.Update(SubmitPromptMsg{Prompt: "/exit"})
	if cmd == nil {
		t.Fatal("expected tea.Quit command, got nil")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
	}

	// Submit /quit
	_, cmd = m.Update(SubmitPromptMsg{Prompt: "/quit"})
	if cmd == nil {
		t.Fatal("expected tea.Quit command, got nil")
	}
	msg = cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestModelDoubleCtrlC(t *testing.T) {
	r := app.NewRuntime()
	m := NewModel(r)

	ctrlC := tea.KeyMsg{Type: tea.KeyCtrlC}

	// First press
	resModel, cmd := m.Update(ctrlC)
	updatedModel := resModel.(Model)

	if cmd == nil {
		t.Fatal("expected a cmd to hide notification")
	}
	if !updatedModel.notificationShow || updatedModel.notificationText != "Press Ctrl+C again to exit" {
		t.Errorf("expected notification, got show=%t, text=%q", updatedModel.notificationShow, updatedModel.notificationText)
	}

	// Second press (immediately after, within 2 seconds)
	_, cmd2 := updatedModel.Update(ctrlC)
	if cmd2 == nil {
		t.Fatal("expected tea.Quit command on second Ctrl+C press")
	}
	msg := cmd2()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}

	// Test if pressing after 2 seconds does not quit but warns again
	resModelAfterDelay, _ := updatedModel.Update(ctrlC)
	updatedModelAfterDelay := resModelAfterDelay.(Model)
	// Force the lastCtrlC to be older than 2 seconds
	updatedModelAfterDelay.lastCtrlC = time.Now().Add(-3 * time.Second)

	resModelPress2, cmdPress2 := updatedModelAfterDelay.Update(ctrlC)
	updatedModelPress2 := resModelPress2.(Model)
	if cmdPress2 == nil {
		t.Fatal("expected hide notification command, not quit")
	}
	if !updatedModelPress2.notificationShow || updatedModelPress2.notificationText != "Press Ctrl+C again to exit" {
		t.Errorf("expected notification again, got show=%t, text=%q", updatedModelPress2.notificationShow, updatedModelPress2.notificationText)
	}
}

func TestModelStartsWithSidebarHidden(t *testing.T) {
	r := app.NewRuntime()
	m := NewModel(r)

	if m.showSidebar {
		t.Fatal("expected sidebar to be hidden by default")
	}
	if !m.sidebarExplicitlyHidden {
		t.Fatal("expected sidebar to be marked explicitly hidden by default")
	}
}

func TestModelSidebarToggleWorksOnSupportedWidth(t *testing.T) {
	r := app.NewRuntime()
	m := NewModel(r)

	resized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = resized.(Model)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(Model)
	if !m.showSidebar {
		t.Fatal("expected sidebar to open on supported width")
	}
	if m.sidebarExplicitlyHidden {
		t.Fatal("expected sidebar hidden flag to clear when opened")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(Model)
	if m.showSidebar {
		t.Fatal("expected sidebar to close on second toggle")
	}
	if !m.sidebarExplicitlyHidden {
		t.Fatal("expected sidebar hidden flag to be restored when closed")
	}
}

func TestModelSidebarToggleWarnsOnSmallWidth(t *testing.T) {
	r := app.NewRuntime()
	m := NewModel(r)

	resized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = resized.(Model)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(Model)
	if m.showSidebar {
		t.Fatal("expected sidebar to remain hidden on small width")
	}
	if !strings.Contains(m.notificationText, "min 84") {
		t.Fatalf("expected small-width warning, got %q", m.notificationText)
	}
}

func TestModelLargeWidthShowsSidebarAutomatically(t *testing.T) {
	r := app.NewRuntime()
	m := NewModel(r)

	resized, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 32})
	m = resized.(Model)

	if !m.showSidebar {
		t.Fatal("expected sidebar to show automatically on large terminals")
	}
}

func TestModelMediumWidthKeepsSidebarHiddenAutomatically(t *testing.T) {
	r := app.NewRuntime()
	m := NewModel(r)

	resized, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	m = resized.(Model)

	if m.showSidebar {
		t.Fatal("expected sidebar to stay hidden automatically on medium terminals")
	}
}

func TestModelSidebarAutoOpensOnPendingApproval(t *testing.T) {
	r := app.NewRuntime()
	m := NewModel(r)

	// Resize to supported width
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = resized.(Model)

	// Simulate that the sidebar is closed but not explicitly hidden by user toggle
	m.sidebarExplicitlyHidden = false
	m.showSidebar = false

	// Verify pre-condition
	if m.showSidebar {
		t.Fatal("expected sidebar to start closed")
	}

	// Trigger a command that requires approval (in Plan mode, !ls does this)
	resModel, _ := m.Update(SubmitPromptMsg{Prompt: "!ls"})
	m = resModel.(Model)

	if !m.showSidebar {
		t.Fatal("expected sidebar to auto-open when a command requires approval")
	}
}

func TestModelSidebarAutoOpensOnActiveTasks(t *testing.T) {
	r := app.NewRuntime()
	m := NewModel(r)

	// Resize to supported width
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = resized.(Model)

	m.sidebarExplicitlyHidden = false
	m.showSidebar = false

	// Start a background task (e.g. sleep) so that active tasks increases from 0 to 1
	_, err := r.StartTask(context.Background(), "sleep 10")
	if err != nil {
		t.Skip("skipping task test if start task fails: ", err)
		return
	}
	defer func() {
		// Clean up tasks if any
		for _, task := range r.ListTasks() {
			_ = r.TerminateTask(task.ID)
		}
	}()

	// Send an empty key msg to trigger update loop
	resModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = resModel.(Model)

	if !m.showSidebar {
		t.Fatal("expected sidebar to auto-open when a background task starts")
	}
}

func TestModelSidebarDoesNotAutoOpenIfExplicitlyHidden(t *testing.T) {
	r := app.NewRuntime()
	m := NewModel(r)

	// Resize to supported width
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = resized.(Model)

	// Set explicitly hidden to true
	m.sidebarExplicitlyHidden = true
	m.showSidebar = false

	// Trigger a command that requires approval
	resModel, _ := m.Update(SubmitPromptMsg{Prompt: "!ls"})
	m = resModel.(Model)

	if m.showSidebar {
		t.Fatal("expected sidebar to remain closed if it was explicitly hidden")
	}
}

