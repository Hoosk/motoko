package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCustomAgents(t *testing.T) {
	tmpDir := t.TempDir()
	modesDir := filepath.Join(tmpDir, ".agents", "modes")
	if err := os.MkdirAll(modesDir, 0755); err != nil {
		t.Fatalf("failed to create modes dir: %v", err)
	}

	content := `---
name: "frontend"
description: "React and CSS expert"
readonly: false
tool_filter: ["read", "grep", "bash"]
---
You are a frontend expert.
`
	if err := os.WriteFile(filepath.Join(modesDir, "frontend.md"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test md: %v", err)
	}

	agents, err := LoadCustomAgents(tmpDir)
	if err != nil {
		t.Fatalf("LoadCustomAgents error: %v", err)
	}

	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}

	agent := agents[0]
	if agent.Name != "frontend" {
		t.Errorf("expected name 'frontend', got '%s'", agent.Name)
	}
	if agent.Frontmatter.Description != "React and CSS expert" {
		t.Errorf("expected description 'React and CSS expert', got '%s'", agent.Frontmatter.Description)
	}
	if agent.Frontmatter.ReadOnly != false {
		t.Errorf("expected readonly false")
	}
	if len(agent.Frontmatter.ToolFilter) != 3 {
		t.Errorf("expected 3 tools, got %d", len(agent.Frontmatter.ToolFilter))
	}
	if agent.System != "You are a frontend expert." {
		t.Errorf("expected system prompt, got '%s'", agent.System)
	}
}

func TestParseList(t *testing.T) {
	cases := []struct {
		input    string
		expected []string
	}{
		{"[read, write]", []string{"read", "write"}},
		{"read, write", []string{"read", "write"}},
		{`["read", "write"]`, []string{"read", "write"}},
		{"", nil},
		{"[]", nil},
	}

	for _, c := range cases {
		result := parseList(c.input)
		if len(result) != len(c.expected) {
			t.Errorf("parseList(%q) returned %d items, expected %d", c.input, len(result), len(c.expected))
		}
		for i := range result {
			if result[i] != c.expected[i] {
				t.Errorf("parseList(%q)[%d] = %q, expected %q", c.input, i, result[i], c.expected[i])
			}
		}
	}
}
