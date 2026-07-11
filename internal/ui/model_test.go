package ui

import (
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
	popup.Open(&tools.PendingQuestion{Question: tools.Question{
		Header:      "Decision",
		Question:    "Pick one",
		AllowCustom: true,
		Options:     []tools.QuestionOption{{Label: "one"}, {Label: "two"}},
	}})
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
	m.questionPopup.Open(&tools.PendingQuestion{Question: tools.Question{
		Header:   "Decision",
		Question: "Pick one",
		Options:  []tools.QuestionOption{{Label: "one"}},
	}})

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
	m.questionPopup.Open(&tools.PendingQuestion{Question: tools.Question{
		Header:   "Decision",
		Question: "Pick one",
		Options:  []tools.QuestionOption{{Label: "one"}},
	}})

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
