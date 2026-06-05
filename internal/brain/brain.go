// Package brain provides per-session persistent markdown storage.
// Each session gets a brain directory where the agent can store
// plans, task lists, summaries, and arbitrary notes.
package brain

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/session"
)

type Brain struct {
	Dir       string // absolute path to brain directory
	SessionID string
}

type FileInfo struct {
	ModTime   time.Time
	Name      string
	SizeBytes int64
}

// New creates/resolves the brain directory for a given workspace and session.
func New(workspaceID, sessionID string) (*Brain, error) {
	base, err := baseDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(base, strings.TrimSpace(workspaceID), "brain", strings.TrimSpace(sessionID))
	return &Brain{
		Dir:       dir,
		SessionID: sessionID,
	}, nil
}

func baseDir() (string, error) {
	if strings.TrimSpace(session.SessionsBaseDir) != "" {
		return session.SessionsBaseDir, nil
	}
	base, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, ".local", "share", "motoko", "sessions"), nil
}

// Write writes or updates a file in the session brain.
func (b *Brain) Write(name, content string) error {
	if b == nil || b.Dir == "" {
		return fmt.Errorf("brain is nil or uninitialized")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("filename cannot be empty")
	}
	name = filepath.Clean(name)
	if isInvalidName(name) {
		return fmt.Errorf("invalid brain file name: %s", name)
	}
	if !strings.HasSuffix(name, ".md") {
		name = name + ".md"
	}
	if err := os.MkdirAll(b.Dir, 0o700); err != nil {
		return fmt.Errorf("failed to create brain dir: %w", err)
	}
	path := filepath.Join(b.Dir, name)
	return os.WriteFile(path, []byte(content), 0o600)
}

// Read reads the content of a brain file.
func (b *Brain) Read(name string) (string, error) {
	if b == nil || b.Dir == "" {
		return "", fmt.Errorf("brain is nil or uninitialized")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("filename cannot be empty")
	}
	name = filepath.Clean(name)
	if isInvalidName(name) {
		return "", fmt.Errorf("invalid brain file name: %s", name)
	}
	if !strings.HasSuffix(name, ".md") {
		name = name + ".md"
	}
	path := filepath.Join(b.Dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// List lists all files in the session brain.
func (b *Brain) List() ([]FileInfo, error) {
	if b == nil || b.Dir == "" {
		return nil, fmt.Errorf("brain is nil or uninitialized")
	}
	entries, err := os.ReadDir(b.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var files []FileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{
			Name:      entry.Name(),
			SizeBytes: info.Size(),
			ModTime:   info.ModTime(),
		})
	}
	return files, nil
}

// Delete removes a file from the session brain.
func (b *Brain) Delete(name string) error {
	if b == nil || b.Dir == "" {
		return fmt.Errorf("brain is nil or uninitialized")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("filename cannot be empty")
	}
	name = filepath.Clean(name)
	if isInvalidName(name) {
		return fmt.Errorf("invalid brain file name: %s", name)
	}
	if !strings.HasSuffix(name, ".md") {
		name = name + ".md"
	}
	path := filepath.Join(b.Dir, name)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Exists checks if a brain file exists.
func (b *Brain) Exists(name string) bool {
	if b == nil || b.Dir == "" {
		return false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	name = filepath.Clean(name)
	if isInvalidName(name) {
		return false
	}
	if !strings.HasSuffix(name, ".md") {
		name = name + ".md"
	}
	path := filepath.Join(b.Dir, name)
	_, err := os.Stat(path)
	return err == nil
}

// Summary returns a list-based text summary of all brain files.
func (b *Brain) Summary() string {
	files, err := b.List()
	if err != nil || len(files) == 0 {
		return "No brain files exist in this session."
	}
	var lines []string
	lines = append(lines, "Brain files:")
	for _, f := range files {
		ago := time.Since(f.ModTime).Truncate(time.Second)
		lines = append(lines, fmt.Sprintf("- %s (%d bytes, updated %s ago)", f.Name, f.SizeBytes, ago))
	}
	return strings.Join(lines, "\n")
}

// PlanSummary returns the truncated content of plan.md.
func (b *Brain) PlanSummary() string {
	content, err := b.Read("plan.md")
	if err != nil {
		return ""
	}
	content = strings.TrimSpace(content)
	if len(content) > 1500 {
		return content[:1500] + "\n... [plan.md truncated, use brain_read to view full plan] ..."
	}
	return content
}

// TasksSummary returns the truncated content of tasks.md.
func (b *Brain) TasksSummary() string {
	content, err := b.Read("tasks.md")
	if err != nil {
		return ""
	}
	content = strings.TrimSpace(content)
	if len(content) > 1000 {
		return content[:1000] + "\n... [tasks.md truncated, use brain_read to view full tasks list] ..."
	}
	return content
}

func isInvalidName(name string) bool {
	if name == "." || name == ".." {
		return true
	}
	if filepath.IsAbs(name) {
		return true
	}
	// Reject path separators on all platforms to prevent subdirectories and traversal.
	if strings.ContainsAny(name, "/\\") {
		return true
	}
	// Reject drive letters and alternate data streams.
	if strings.Contains(name, ":") {
		return true
	}
	prefix := ".." + string(filepath.Separator)
	return strings.HasPrefix(name, prefix)
}
