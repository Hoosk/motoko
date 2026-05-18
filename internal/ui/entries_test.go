package ui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/styles"
)

func TestEntriesForAgentResult(t *testing.T) {
	result := agent.Result{
		AgentLabel: "openai:gpt-4.1",
		Duration:   250 * time.Millisecond,
		Steps: []agent.Step{
			{Kind: "tool", Title: "read", Content: "README.md 1 20"},
			{Kind: "output", Title: "read", Content: "contenido"},
			{Kind: "error", Title: "bash", Content: "tool error: boom"},
			{Kind: "assistant", Title: "answer", Content: "respuesta final"},
			{Kind: "debug", Title: "provider", Content: "completion 1"},
		},
		Usage: provider.Usage{InputTokens: 11, OutputTokens: 7, TotalTokens: 18},
	}

	want := []app.Entry{
		{Kind: app.EntrySystem, Text: styles.AssistantMetaStyle.Render("agent:openai:gpt-4.1  elapsed:250ms")},
		{Kind: app.EntrySystem, Text: "tokens in:11 out:7 total:18"},
	}

	got := entriesForAgentResult(result, false)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entriesForAgentResult() = %#v, want %#v", got, want)
	}
}

func TestEntriesForAgentResultSkipsEmptyToolInputAndZeroTokens(t *testing.T) {
	result := agent.Result{
		AgentLabel: "openai:gpt-4.1",
		Steps:      []agent.Step{{Kind: "tool", Title: "grep", Content: "   "}},
	}

	want := []app.Entry{
		{Kind: app.EntrySystem, Text: styles.AssistantMetaStyle.Render("agent:openai:gpt-4.1  elapsed:0s")},
	}

	got := entriesForAgentResult(result, false)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entriesForAgentResult() = %#v, want %#v", got, want)
	}
}

func TestEntriesForAgentResultIncludesContextInDebug(t *testing.T) {
	result := agent.Result{
		Context: agent.ContextSnapshot{
			Signals:          "semantic: files:10",
			Semantic:         "files:10",
			RelevantFiles:    "internal/app/runtime.go",
			RelevantSnippets: "FILE internal/app/runtime.go",
		},
		Steps: []agent.Step{{Kind: "assistant", Title: "answer", Content: "ok"}},
	}

	got := entriesForAgentResult(result, true)
	if len(got) == 0 || got[0].Kind != app.EntrySystem {
		t.Fatalf("expected debug context entry first, got %#v", got)
	}
	if !strings.Contains(got[0].Text, "agent context") {
		t.Fatalf("expected context text, got %#v", got[0])
	}
}

func TestEstimateTextareaHeight(t *testing.T) {
	tests := []struct {
		name  string
		value string
		width int
		want  int
	}{
		{name: "empty uses minimum", value: "", width: 20, want: 3},
		{name: "short line uses minimum", value: "hola", width: 20, want: 3},
		{name: "multiple lines grow", value: "uno\ndos\ntres\ncuatro", width: 20, want: 4},
		{name: "wrapped line grows", value: "12345678901", width: 5, want: 3},
	}

	for _, tt := range tests {
		got := estimateTextareaHeight(tt.value, tt.width)
		if got != tt.want {
			t.Fatalf("%s: estimateTextareaHeight() = %d, want %d", tt.name, got, tt.want)
		}
	}
}
