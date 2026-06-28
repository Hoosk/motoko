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
	if m.sidebarPref != sidebarDefault {
		t.Fatal("expected sidebar preference to be default")
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
	if m.sidebarPref != sidebarForceShow {
		t.Fatal("expected sidebar preference to be forceShow when opened")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(Model)
	if m.showSidebar {
		t.Fatal("expected sidebar to close on second toggle")
	}
	if m.sidebarPref != sidebarForceHide {
		t.Fatal("expected sidebar preference to be forceHide when closed")
	}
}

func TestModelSidebarToggleWarnsOnSmallWidth(t *testing.T) {
	r := app.NewRuntime()
	m := NewModel(r)

	resized, _ := m.Update(tea.WindowSizeMsg{Width: 30, Height: 24})
	m = resized.(Model)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(Model)
	if m.showSidebar {
		t.Fatal("expected sidebar to remain hidden on small width")
	}
	if !strings.Contains(m.notificationText, "min 40") {
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
	m.sidebarPref = sidebarDefault
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

	m.sidebarPref = sidebarDefault
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

	// Set preference to forceHide
	m.sidebarPref = sidebarForceHide
	m.showSidebar = false

	// Trigger a command that requires approval
	resModel, _ := m.Update(SubmitPromptMsg{Prompt: "!ls"})
	m = resModel.(Model)

	if m.showSidebar {
		t.Fatal("expected sidebar to remain closed if it was explicitly hidden")
	}
}

func TestModelSidebarCanBeClosedOnWideScreen(t *testing.T) {
	r := app.NewRuntime()
	m := NewModel(r)

	// Resize to wide screen (>= 140)
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 32})
	m = resized.(Model)

	if !m.showSidebar {
		t.Fatal("expected sidebar to auto-open on wide screens by default")
	}

	// Toggle it closed using Ctrl+S
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(Model)

	if m.showSidebar {
		t.Fatal("expected sidebar to close after toggling Ctrl+S on wide screens")
	}
	if m.sidebarPref != sidebarForceHide {
		t.Fatal("expected sidebar preference to be forceHide")
	}

	// Resize window again at wide width, it must REMAIN closed!
	resized2, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 32})
	m = resized2.(Model)

	if m.showSidebar {
		t.Fatal("expected sidebar to remain closed on resize if user explicitly closed it")
	}
}

func TestModelSidebarDualWidthLayout(t *testing.T) {
	r := app.NewRuntime()
	m := NewModel(r)

	// Case 1: width 60 (should be width 20)
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	m = resized.(Model)
	w, ok := m.sidebarLayout()
	if !ok || w != 20 {
		t.Fatalf("expected sidebar layout of 20 columns on narrow width 60, got width=%d ok=%t", w, ok)
	}

	// Case 2: width 100 (should be width 36)
	resized2, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = resized2.(Model)
	w2, ok2 := m.sidebarLayout()
	if !ok2 || w2 != 36 {
		t.Fatalf("expected sidebar layout of 36 columns on normal width 100, got width=%d ok=%t", w2, ok2)
	}
}



