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
	s.Messages = []provider.Message{{Role: "user", Content: "hola"}}
	s.LastInputTokens = 42
	if err := s.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := Load("abc", s.ID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Title != "Titulo" || len(loaded.Messages) != 1 || loaded.LastInputTokens != 42 {
		t.Fatalf("loaded session = %#v", loaded)
	}
	if got := WorkspaceIDFor("/tmp/work"); got == "" {
		t.Fatal("WorkspaceIDFor() returned empty string")
	}
	if _, err := filepath.Abs(loaded.Workspace); err != nil {
		t.Fatalf("workspace path invalid: %v", err)
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
	s.Messages = []provider.Message{{Role: "user", Content: "hola"}, {Role: "assistant", Content: "mundo"}}
	s.LastInputTokens = 99
	s.CompactWith("resumen")
	if len(s.Messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2", len(s.Messages))
	}
	if s.Messages[0].Role != "user" || s.Messages[1].Role != "assistant" {
		t.Fatalf("unexpected compacted messages = %#v", s.Messages)
	}
	if s.LastInputTokens != 0 {
		t.Fatalf("LastInputTokens = %d, want 0", s.LastInputTokens)
	}
}
