package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/tools"
	"github.com/Hoosk/motoko/internal/ui/timeline"
	tea "github.com/charmbracelet/bubbletea"
)

func TestQueueOperations(t *testing.T) {
	t.Run("dequeue empty", func(t *testing.T) {
		m := Model{}
		got, ok := m.dequeuePrompt()
		if ok || got != "" {
			t.Fatalf("expected empty dequeue, got %q ok=%v", got, ok)
		}
	})

	t.Run("enqueue dequeue preserves order", func(t *testing.T) {
		m := Model{}
		m.enqueuePrompt("one")
		m.enqueuePrompt("two")

		got, ok := m.dequeuePrompt()
		if !ok || got != "one" {
			t.Fatalf("expected first prompt, got %q ok=%v", got, ok)
		}
		if len(m.promptQueue) != 1 || m.promptQueue[0] != "two" {
			t.Fatalf("expected remaining queue [two], got %#v", m.promptQueue)
		}
	})

	t.Run("remove queued item clamps selection", func(t *testing.T) {
		m := Model{promptQueue: []string{"one", "two", "three"}, queueSel: 2, queueFocus: true}
		m.removeQueuedAt(1)
		if len(m.promptQueue) != 2 {
			t.Fatalf("expected queue len 2, got %d", len(m.promptQueue))
		}
		if m.promptQueue[0] != "one" || m.promptQueue[1] != "three" {
			t.Fatalf("unexpected queue contents %#v", m.promptQueue)
		}
		if m.queueSel != 1 {
			t.Fatalf("expected selection 1, got %d", m.queueSel)
		}
	})

	t.Run("remove last queued item clears focus", func(t *testing.T) {
		m := Model{promptQueue: []string{"one"}, queueSel: 0, queueFocus: true}
		m.removeQueuedAt(0)
		if len(m.promptQueue) != 0 {
			t.Fatalf("expected empty queue, got %#v", m.promptQueue)
		}
		if m.queueSel != 0 {
			t.Fatalf("expected selection reset, got %d", m.queueSel)
		}
		if m.queueFocus {
			t.Fatal("expected queue focus to clear")
		}
	})

	t.Run("move queued item up and down", func(t *testing.T) {
		m := Model{promptQueue: []string{"one", "two", "three"}, queueSel: 1}
		m.moveQueued(1, -1)
		if m.promptQueue[0] != "two" || m.promptQueue[1] != "one" {
			t.Fatalf("expected swapped queue, got %#v", m.promptQueue)
		}
		if m.queueSel != 0 {
			t.Fatalf("expected selection 0, got %d", m.queueSel)
		}

		m.moveQueued(0, 1)
		if m.promptQueue[0] != "one" || m.promptQueue[1] != "two" {
			t.Fatalf("expected moved back queue, got %#v", m.promptQueue)
		}
		if m.queueSel != 1 {
			t.Fatalf("expected selection 1, got %d", m.queueSel)
		}
	})

	t.Run("move out of bounds is ignored", func(t *testing.T) {
		m := Model{promptQueue: []string{"one", "two"}, queueSel: 0}
		m.moveQueued(0, -1)
		if m.promptQueue[0] != "one" || m.promptQueue[1] != "two" {
			t.Fatalf("expected queue unchanged, got %#v", m.promptQueue)
		}
	})
}

func TestSubmitPromptQueuesWhileThinking(t *testing.T) {
	m := NewModel(app.NewRuntime())
	m.timeline.SetThinking(true)

	updated, cmd := m.Update(SubmitPromptMsg{Prompt: "queued prompt"})
	if cmd != nil {
		t.Fatal("expected no immediate command when queuing prompt")
	}

	got := updated.(Model)
	if len(got.promptQueue) != 1 || got.promptQueue[0] != "queued prompt" {
		t.Fatalf("expected prompt to be queued, got %#v", got.promptQueue)
	}
}

func TestDialogRequestActivatesApprovalBar(t *testing.T) {
	m := Model{settingsPopup: settingsPopupState{active: true}}
	pending := &tools.Pending{Request: tools.DialogRequest{Kind: tools.DialogFileChange}}

	updated, _ := m.Update(DialogRequestedMsg{Pending: pending})
	got := updated.(Model)
	if !got.approvalBar.active || got.approvalBar.pending != pending {
		t.Fatalf("expected approval bar to open, got %#v", got.approvalBar)
	}
	if got.approvalBar.sel != 0 {
		t.Fatalf("expected approve button selected initially, got %d", got.approvalBar.sel)
	}
}

func TestApprovalBarRendersShellCommand(t *testing.T) {
	m := NewModel(app.NewRuntime())
	pending := &tools.Pending{Request: tools.DialogRequest{
		Kind: tools.DialogShellCommand,
		ShellCommand: tools.ShellCommand{
			Command: "git add file.go",
			Reason:  "The command may modify files or repository state.",
		},
	}}
	m.approvalBar.Open(pending)
	view := stripANSI(m.approvalBar.View(100))
	for _, want := range []string{"Approve shell command", "$ git add file.go", "[ approve ]", "[ reject ]", "←/→ select"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected shell approval bar to contain %q, got %q", want, view)
		}
	}
}

func TestDialogRequestAppendsContentToTimeline(t *testing.T) {
	m := NewModel(app.NewRuntime())
	pending := &tools.Pending{Request: tools.DialogRequest{
		Kind:   tools.DialogFileChange,
		Change: tools.FileChange{Path: "main.go", Diff: "--- a/main.go\n+++ b/main.go\n@@ -1,1 +1,1 @@\n-old\n+new"},
	}}

	updated, _ := m.Update(DialogRequestedMsg{Pending: pending})
	got := updated.(Model)

	foundRequest, foundDiff := false, false
	for _, entry := range got.timeline.model.Entries {
		if entry.Kind == app.EntrySystem && strings.Contains(entry.Text, "Approval requested: main.go") {
			foundRequest = true
		}
		if entry.Kind == app.EntryOutput && strings.Contains(entry.Text, "+new") {
			foundDiff = true
		}
	}
	if !foundRequest || !foundDiff {
		t.Fatalf("expected approval content in timeline, request=%v diff=%v", foundRequest, foundDiff)
	}
}

func TestApprovalBarButtonNavigationAndConfirm(t *testing.T) {
	m := NewModel(app.NewRuntime())
	result := make(chan error, 1)
	go func() {
		result <- m.runtime.Broker().RequestShellCommand(context.Background(), tools.ShellCommand{Command: "printf ok", Reason: "test"})
	}()

	pending, err := m.runtime.Broker().Next(m.runtime.BackgroundContext())
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(DialogRequestedMsg{Pending: pending})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)
	if m.approvalBar.sel != 1 {
		t.Fatalf("expected selection on reject after right, got %d", m.approvalBar.sel)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.approvalBar.active {
		t.Fatal("expected bar closed after confirm")
	}

	if err := <-result; !errors.Is(err, tools.ErrCommandRejected) {
		t.Fatalf("expected rejection via selected button, got %v", err)
	}
	if got := m.runtime.Broker().PendingCount(); got != 0 {
		t.Fatalf("expected no pending dialogs after confirm, got %d", got)
	}
}

func TestApprovalBarQuickKeys(t *testing.T) {
	t.Run("y approves", func(t *testing.T) {
		m := NewModel(app.NewRuntime())
		result := make(chan error, 1)
		go func() {
			result <- m.runtime.Broker().RequestShellCommand(context.Background(), tools.ShellCommand{Command: "printf ok", Reason: "test"})
		}()
		pending, err := m.runtime.Broker().Next(m.runtime.BackgroundContext())
		if err != nil {
			t.Fatal(err)
		}
		updated, _ := m.Update(DialogRequestedMsg{Pending: pending})
		m = updated.(Model)

		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
		m = updated.(Model)
		if m.approvalBar.active {
			t.Fatal("expected bar closed after y")
		}
		if err := <-result; err != nil {
			t.Fatalf("expected approval via y, got %v", err)
		}
	})

	t.Run("esc rejects", func(t *testing.T) {
		m := NewModel(app.NewRuntime())
		result := make(chan error, 1)
		go func() {
			result <- m.runtime.Broker().RequestShellCommand(context.Background(), tools.ShellCommand{Command: "printf ok", Reason: "test"})
		}()
		pending, err := m.runtime.Broker().Next(m.runtime.BackgroundContext())
		if err != nil {
			t.Fatal(err)
		}
		updated, _ := m.Update(DialogRequestedMsg{Pending: pending})
		m = updated.(Model)

		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m = updated.(Model)
		if m.approvalBar.active {
			t.Fatal("expected bar closed after esc")
		}
		if err := <-result; !errors.Is(err, tools.ErrCommandRejected) {
			t.Fatalf("expected rejection via esc, got %v", err)
		}
	})
}

func TestApprovalBarKeepsScrollKeysForTimeline(t *testing.T) {
	m := NewModel(app.NewRuntime())
	pending := &tools.Pending{Request: tools.DialogRequest{Kind: tools.DialogFileChange}}
	updated, _ := m.Update(DialogRequestedMsg{Pending: pending})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if !m.approvalBar.active {
		t.Fatal("expected bar to stay active while scrolling")
	}
	if m.approvalBar.sel != 0 {
		t.Fatalf("expected selection unchanged by scroll key, got %d", m.approvalBar.sel)
	}
}

func TestExpiredDialogClearsApprovalBar(t *testing.T) {
	m := NewModel(app.NewRuntime())
	pending := &tools.Pending{Request: tools.DialogRequest{Kind: tools.DialogFileChange}}
	pending.Resolve(tools.DialogDecision{})

	updated, _ := m.Update(DialogRequestedMsg{Pending: pending})
	m = updated.(Model)
	if !m.approvalBar.active {
		t.Fatal("expected bar open for delivered dialog")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = updated.(Model)
	if m.approvalBar.active {
		t.Fatal("expected stale bar to be cleared")
	}
	found := false
	for _, entry := range m.timeline.model.Entries {
		if strings.Contains(entry.Text, "Approval expired") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected expiry entry in timeline")
	}
}

func TestShellApprovalRunsOnlyAfterResolution(t *testing.T) {
	m := NewModel(app.NewRuntime())
	cmd := m.runShellApproval("printf approved", "test command")
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()

	pending, err := m.runtime.Broker().Next(m.runtime.BackgroundContext())
	if err != nil {
		t.Fatal(err)
	}
	if pending.Kind != tools.DialogShellCommand {
		t.Fatalf("expected shell command dialog, got %s", pending.Kind)
	}
	pending.Resolve(tools.DialogDecision{Approved: true})

	msg := <-result
	shellResult, ok := msg.(ShellResultMsg)
	if !ok {
		t.Fatalf("expected shell result after approval, got %#v", msg)
	}
	if shellResult.Result.Output != "approved" || shellResult.Result.ExitCode != 0 {
		t.Fatalf("unexpected approved shell result %#v", shellResult.Result)
	}
}

func TestNextPromptAfterAgentKeepsGoalAliveWithoutTasks(t *testing.T) {
	m := NewModel(app.NewRuntime())
	br := m.runtime.GetBrain()
	if err := br.Write("goal", "# Goal\nDo the thing"); err != nil {
		t.Fatal(err)
	}

	next, ok := m.nextPromptAfterAgent()
	if !ok || !strings.Contains(next, "No tasks.md exists yet") {
		t.Fatalf("unexpected next prompt: %q ok=%v", next, ok)
	}
	if !br.Exists("goal") {
		t.Fatal("goal should remain active when tasks.md does not exist")
	}
}

func TestNextPromptAfterAgentCompletesGoalWhenTasksDone(t *testing.T) {
	m := NewModel(app.NewRuntime())
	br := m.runtime.GetBrain()
	if err := br.Write("goal", "# Goal\nDo the thing"); err != nil {
		t.Fatal(err)
	}
	if err := br.Write("tasks", "# Tasks\n- [x] done"); err != nil {
		t.Fatal(err)
	}

	next, ok := m.nextPromptAfterAgent()
	if ok || next != "" {
		t.Fatalf("expected no auto-continue prompt, got %q ok=%v", next, ok)
	}
	if br.Exists("goal") {
		t.Fatal("goal should be cleared when tasks are complete")
	}
}

func TestQuestionPopupSwitchesBetweenListAndCustomFocus(t *testing.T) {
	var popup questionPopupState
	popup.Open(&tools.Pending{Request: tools.DialogRequest{Kind: tools.DialogQuestion, Question: tools.Question{
		Header:      "Decision",
		Question:    "Pick one",
		AllowCustom: true,
		Options:     []tools.QuestionOption{{Label: "one"}, {Label: "two"}},
	}}})
	if popup.focus != questionFocusList {
		t.Fatalf("expected initial list focus, got %v", popup.focus)
	}
	popup.Update(tea.KeyMsg{Type: tea.KeyTab})
	if popup.focus != questionFocusCustom {
		t.Fatalf("expected custom focus after tab, got %v", popup.focus)
	}
	popup.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if popup.focus != questionFocusList {
		t.Fatalf("expected list focus after shift+tab, got %v", popup.focus)
	}
}

func TestQuestionPopupKeepsAgentStreamPollingAlive(t *testing.T) {
	m := NewModel(app.NewRuntime())
	m.requestID = 7
	m.agentStream = make(chan app.AgentStreamEvent, 1)
	m.questionPopup.Open(&tools.Pending{Request: tools.DialogRequest{Kind: tools.DialogQuestion, Question: tools.Question{
		Header:   "Decision",
		Question: "Pick one",
		Options:  []tools.QuestionOption{{Label: "one"}},
	}}})

	updated, cmd := m.Update(AgentStreamBatchMsg{
		RequestID: 7,
		Events:    []app.AgentStreamEvent{{Kind: "assistant_delta", Content: "hola"}},
		Done:      false,
	})
	m = updated.(Model)

	if cmd == nil {
		t.Fatal("expected waitAgentStream to be re-armed while question popup is active")
	}
	if !m.questionPopup.active {
		t.Fatal("expected question popup to remain active")
	}
}

func TestQuestionPopupKeepsThinkingTickAlive(t *testing.T) {
	m := NewModel(app.NewRuntime())
	m.timeline.SetThinking(true)
	m.footer.SetThinking(true)
	m.questionPopup.Open(&tools.Pending{Request: tools.DialogRequest{Kind: tools.DialogQuestion, Question: tools.Question{
		Header:   "Decision",
		Question: "Pick one",
		Options:  []tools.QuestionOption{{Label: "one"}},
	}}})

	updated, cmd := m.Update(ThinkingTickMsg{})
	m = updated.(Model)

	if cmd == nil {
		t.Fatal("expected thinking tick to be re-armed while question popup is active")
	}
	if !m.questionPopup.active {
		t.Fatal("expected question popup to remain active")
	}
}

func TestModelMouseDragSelectionCopiesText(t *testing.T) {
	m := NewModel(app.NewRuntime())
	m.width = 80
	m.height = 24
	m.SyncLayout()

	response := app.Response{Entries: []app.Entry{{Kind: app.EntryAssistant, Text: "texto util"}}}
	updated, _ := m.Update(ResponseAppliedMsg{Response: response})
	m = updated.(Model)

	assistantLine := -1
	for i, line := range m.timeline.model.RenderLines {
		if strings.Contains(line.Plain, "texto util") {
			assistantLine = i
			break
		}
	}
	if assistantLine < 0 {
		t.Fatalf("expected assistant line in render map")
	}

	pressY := assistantLine - int(m.timeline.model.Viewport.YOffset) + timeline.TimelineMouseOffsetY
	pressX := timeline.TimelineMouseOffsetX + timeline.AssistantContentX

	updated, _ = m.Update(tea.MouseMsg{X: pressX, Y: pressY, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m = updated.(Model)
	if !m.timeline.model.Selecting {
		t.Fatalf("expected model mouse press to begin selection")
	}

	updated, _ = m.Update(tea.MouseMsg{X: pressX + 5, Y: pressY, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft})
	m = updated.(Model)
	if !m.timeline.model.SelectionDragged {
		t.Fatalf("expected model mouse drag to extend selection")
	}

	updated, cmd := m.Update(tea.MouseMsg{X: pressX + 5, Y: pressY, Action: tea.MouseActionRelease, Button: tea.MouseButtonNone})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected model mouse release to produce copy command")
	}

	selected, ok := m.timeline.model.SelectedText()
	if !ok || !strings.Contains(selected, "texto") {
		t.Fatalf("expected selected text to include assistant content, got %q", selected)
	}
}
