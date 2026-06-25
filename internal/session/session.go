package session

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/provider"
)

var SessionsBaseDir string

type Session struct {
	ID              string                      `json:"id"`
	Title           string                      `json:"title"`
	WorkspaceID     string                      `json:"workspace_id"`
	Workspace       string                      `json:"workspace"`
	CreatedAt       time.Time                   `json:"created_at"`
	UpdatedAt       time.Time                   `json:"updated_at"`
	History         []provider.ConversationItem `json:"history,omitempty"`
	LastInputTokens int                         `json:"last_input_tokens,omitempty"`
	
	TotalInputTokens      int                         `json:"total_input_tokens,omitempty"`
	TotalOutputTokens     int                         `json:"total_output_tokens,omitempty"`
	TotalTokens           int                         `json:"total_tokens,omitempty"`
	TotalReasoningTokens  int                         `json:"total_reasoning_tokens,omitempty"`
	TotalCacheReadTokens  int                         `json:"total_cache_read_tokens,omitempty"`
	TotalCacheWriteTokens int                         `json:"total_cache_write_tokens,omitempty"`
}

func WorkspaceIDFor(path string) string {
	abs := strings.TrimSpace(path)
	if abs == "" {
		abs = "."
	}
	hash := sha1.Sum([]byte(abs))
	return hex.EncodeToString(hash[:8])
}

func New(workspaceID, workspacePath string) *Session {
	now := time.Now().UTC()
	return &Session{
		ID:          newSessionID(now),
		Title:       "New session",
		WorkspaceID: strings.TrimSpace(workspaceID),
		Workspace:   strings.TrimSpace(workspacePath),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func (s *Session) Save() error {
	if s == nil {
		return fmt.Errorf("sesion nil")
	}
	if strings.TrimSpace(s.ID) == "" {
		s.ID = newSessionID(time.Now().UTC())
	}
	if strings.TrimSpace(s.Title) == "" {
		s.Title = "New session"
	}
	if strings.TrimSpace(s.WorkspaceID) == "" {
		return fmt.Errorf("workspace_id vacio")
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	s.UpdatedAt = time.Now().UTC()

	dir, err := workspaceDir(s.WorkspaceID)
	if err != nil {
		return err
	}
	err = os.MkdirAll(dir, 0o700)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, s.ID+".json"), data, 0o600)
}

func (s *Session) CompactWith(summary string) {
	if s == nil {
		return
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return
	}
	s.History = []provider.ConversationItem{
		provider.UserText("[Resumen de la conversacion previa]\n" + summary),
		provider.AssistantText("Entendido, continuo desde este resumen."),
	}
	s.LastInputTokens = 0
}

func Load(workspaceID, id string) (*Session, error) {
	dir, err := workspaceDir(workspaceID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, strings.TrimSpace(id)+".json"))
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func List(workspaceID string) ([]*Session, error) {
	dir, err := workspaceDir(workspaceID)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	result := make([]*Session, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		result = append(result, &s)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	return result, nil
}

func Last(workspaceID string) (*Session, error) {
	sessions, err := List(workspaceID)
	if err != nil || len(sessions) == 0 {
		return nil, err
	}
	return sessions[0], nil
}

func workspaceDir(workspaceID string) (string, error) {
	base, err := baseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, strings.TrimSpace(workspaceID)), nil
}

func baseDir() (string, error) {
	if strings.TrimSpace(SessionsBaseDir) != "" {
		return SessionsBaseDir, nil
	}
	base, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, ".local", "share", "motoko", "sessions"), nil
}

func newSessionID(now time.Time) string {
	random := make([]byte, 2)
	_, _ = rand.Read(random)
	return now.UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(random)
}
