package ui

import (
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/app"
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
