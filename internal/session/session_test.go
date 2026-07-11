package session

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Hoosk/motoko/internal/provider"
)

func TestSessionSaveLoad(t *testing.T) {
	SessionsBaseDir = t.TempDir()
	t.Cleanup(func() { SessionsBaseDir = "" })

	s := New("abc", "/tmp/work")
	s.Title = "Titulo"
	s.History = []provider.ConversationItem{provider.UserText("hola")}
	s.LastInputTokens = 42
	if err := s.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := Load("abc", s.ID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Title != "Titulo" || len(loaded.History) != 1 || loaded.LastInputTokens != 42 {
		t.Fatalf("loaded session = %#v", loaded)
	}
	if got := WorkspaceIDFor("/tmp/work"); got == "" {
		t.Fatal("WorkspaceIDFor() returned empty string")
	}
	if _, err := filepath.Abs(loaded.Workspace); err != nil {
		t.Fatalf("workspace path invalid: %v", err)
	}
}

func TestSessionSaveLoadTokenBreakdown(t *testing.T) {
	SessionsBaseDir = t.TempDir()
	t.Cleanup(func() { SessionsBaseDir = "" })

	s := New("abc", "/tmp/work")
	s.TotalSystemStaticTokens = 100
	s.TotalSystemDynamicTokens = 200
	s.TotalToolsTokens = 30
	s.TotalHistoryTokens = 50

	s.LastSystemStaticTokens = 10
	s.LastSystemDynamicTokens = 20
	s.LastToolsTokens = 3
	s.LastHistoryTokens = 5

	if err := s.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := Load("abc", s.ID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.TotalSystemStaticTokens != 100 ||
		loaded.TotalSystemDynamicTokens != 200 ||
		loaded.TotalToolsTokens != 30 ||
		loaded.TotalHistoryTokens != 50 ||
		loaded.LastSystemStaticTokens != 10 ||
		loaded.LastSystemDynamicTokens != 20 ||
		loaded.LastToolsTokens != 3 ||
		loaded.LastHistoryTokens != 5 {
		t.Fatalf("loaded session fields mismatch = %#v", loaded)
	}
}

func TestSessionListAndLast(t *testing.T) {
	SessionsBaseDir = t.TempDir()
	t.Cleanup(func() { SessionsBaseDir = "" })

	older := New("abc", "/tmp/work")
	older.Title = "older"
	older.UpdatedAt = time.Now().UTC().Add(-time.Hour)
	if err := older.Save(); err != nil {
		t.Fatalf("older.Save() error = %v", err)
	}

	newer := New("abc", "/tmp/work")
	newer.Title = "newer"
	if err := newer.Save(); err != nil {
		t.Fatalf("newer.Save() error = %v", err)
	}

	list, err := List("abc")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(List()) = %d, want 2", len(list))
	}
	last, err := Last("abc")
	if err != nil {
		t.Fatalf("Last() error = %v", err)
	}
	if last.ID != list[0].ID {
		t.Fatalf("Last().ID = %s, want %s", last.ID, list[0].ID)
	}
}

func TestSessionCompactWith(t *testing.T) {
	s := New("abc", "/tmp/work")
	s.History = []provider.ConversationItem{provider.UserText("hola"), provider.AssistantText("mundo")}
	s.LastInputTokens = 99
	s.LastOutputTokens = 44
	s.LastReasoningTokens = 22
	s.CompactWith("resumen")
	if len(s.History) != 2 {
		t.Fatalf("len(History) = %d, want 2", len(s.History))
	}
	if s.History[0].Role != provider.RoleUser || s.History[1].Role != provider.RoleAssistant {
		t.Fatalf("unexpected compacted history = %#v", s.History)
	}
	if s.LastInputTokens != 0 {
		t.Fatalf("LastInputTokens = %d, want 0", s.LastInputTokens)
	}
	if s.LastOutputTokens != 0 || s.LastReasoningTokens != 0 {
		t.Fatalf("expected last output/reasoning tokens reset, got %#v", s)
	}
}

func TestSessionAddTurnKeepsLastTwenty(t *testing.T) {
	s := New("abc", "/tmp/work")
	for i := 1; i <= 25; i++ {
		s.AddTurn(TurnUsage{Turn: i, InputTokens: i})
	}
	if len(s.Turns) != 20 {
		t.Fatalf("expected 20 turns, got %d", len(s.Turns))
	}
	if s.Turns[0].Turn != 6 || s.Turns[len(s.Turns)-1].Turn != 25 {
		t.Fatalf("unexpected turn window %#v", s.Turns)
	}
}
