package sessionman

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/session"

	"github.com/Hoosk/motoko/internal/app/types"
)

func TestNewManager(t *testing.T) {
	m := NewManager("test-workspace")
	if m.WorkspaceID() != "test-workspace" {
		t.Errorf("expected WorkspaceID 'test-workspace', got %q", m.WorkspaceID())
	}
	if m.CurrentSession() != nil {
		t.Error("expected no current session initially")
	}
	if m.Brain() != nil {
		t.Error("expected no brain initially")
	}
	if m.WasResumed() {
		t.Error("expected WasResumed false initially")
	}
}

func TestSetCurrentSession(t *testing.T) {
	m := NewManager("ws")
	s := session.New("ws", "/tmp")
	m.SetCurrentSession(s)

	if m.CurrentSession() == nil {
		t.Fatal("expected current session to be set")
	}
	if m.CurrentSession().ID != s.ID {
		t.Error("expected same session ID")
	}
}

func TestSessionTitle(t *testing.T) {
	m := NewManager("ws")
	if m.SessionTitle() != "" {
		t.Errorf("expected empty title, got %q", m.SessionTitle())
	}

	s := session.New("ws", "/tmp")
	s.Title = "My Session"
	m.SetCurrentSession(s)
	if m.SessionTitle() != "My Session" {
		t.Errorf("expected 'My Session', got %q", m.SessionTitle())
	}
}

func TestStartupEntries(t *testing.T) {
	m := NewManager("ws")
	s := session.New("ws", "/tmp")
	m.SetCurrentSession(s)
	m.SetWasResumed(true)

	entries := m.StartupEntries()
	if len(entries) == 0 {
		t.Fatal("expected startup entries")
	}

	foundResume := false
	for _, e := range entries {
		if e.Kind == types.EntrySystem && len(e.Text) > 0 {
			foundResume = true
			break
		}
	}
	if !foundResume {
		t.Error("expected resume entry")
	}
}

func TestStartupEntriesFreshSession(t *testing.T) {
	m := NewManager("ws")
	s := session.New("ws", "/tmp")
	m.SetCurrentSession(s)
	m.SetWasResumed(false)

	entries := m.StartupEntries()
	if entries != nil {
		t.Errorf("expected nil entries for fresh session, got %d", len(entries))
	}
}

func TestCurrentSessionEntries(t *testing.T) {
	m := NewManager("ws")
	s := session.New("ws", "/tmp")
	s.History = []provider.ConversationItem{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}
	m.SetCurrentSession(s)

	entries := m.CurrentSessionEntries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Kind != types.EntryUser || entries[0].Text != "hello" {
		t.Errorf("expected user entry, got %v", entries[0])
	}
	if entries[1].Kind != types.EntryAssistant || entries[1].Text != "hi there" {
		t.Errorf("expected assistant entry, got %v", entries[1])
	}
}

func TestHistoryInputTokens(t *testing.T) {
	m := NewManager("ws")
	s := session.New("ws", "/tmp")
	s.LastInputTokens = 500
	m.SetCurrentSession(s)

	tokens := m.HistoryInputTokens()
	if tokens != 500 {
		t.Errorf("expected 500 tokens, got %d", tokens)
	}
}

func TestPersistTurn(t *testing.T) {
	m := NewManager("ws")
	s := session.New("ws", "/tmp")
	m.SetCurrentSession(s)

	m.PersistTurn(agent.Result{
		Assistant: "response",
		History: []provider.ConversationItem{
			{Role: "user", Content: "query"},
			{Role: "assistant", Content: "response"},
		},
		Usage: provider.Usage{
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
		},
	})

	if len(s.History) != 2 {
		t.Fatalf("expected 2 history items, got %d", len(s.History))
	}
	if s.LastInputTokens != 100 {
		t.Errorf("expected LastInputTokens 100, got %d", s.LastInputTokens)
	}
	if s.TotalInputTokens != 100 {
		t.Errorf("expected TotalInputTokens 100, got %d", s.TotalInputTokens)
	}
}

func TestListSessionsEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	sessions, err := (&Manager{workspaceID: tmpDir}).ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected empty sessions list, got %d", len(sessions))
	}
}

func TestLoadSessionNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{workspaceID: tmpDir}
	err := m.LoadSession("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestSetWasResumed(t *testing.T) {
	m := NewManager("ws")
	m.SetWasResumed(true)
	if !m.WasResumed() {
		t.Error("expected WasResumed true")
	}
	m.SetWasResumed(false)
	if m.WasResumed() {
		t.Error("expected WasResumed false")
	}
}

func TestSetBrain(t *testing.T) {
	m := NewManager("ws")
	m.SetBrain(nil, nil)
	if m.Brain() != nil {
		t.Error("expected nil brain")
	}
	if m.BrainInitErr() != nil {
		t.Error("expected nil brain init error")
	}
}

func TestBrainInitErr(t *testing.T) {
	m := NewManager("ws")
	err := os.ErrNotExist
	m.SetBrain(nil, err)
	if m.BrainInitErr() == nil {
		t.Error("expected brain init error")
	}
}

func TestListSessionsWithData(t *testing.T) {
	tmpDir := t.TempDir()
	s1 := session.New(tmpDir, "/tmp")
	s1.Title = "Session One"
	s1.Save()

	s2 := session.New(tmpDir, "/tmp")
	s2.Title = "Session Two"
	s2.Save()

	m := &Manager{workspaceID: tmpDir}
	sessions, err := m.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) < 2 {
		t.Errorf("expected at least 2 sessions, got %d", len(sessions))
	}
	_ = os.Remove(filepath.Join(tmpDir, ".motoko", "sessions", s1.ID+".json"))
	_ = os.Remove(filepath.Join(tmpDir, ".motoko", "sessions", s2.ID+".json"))
}
