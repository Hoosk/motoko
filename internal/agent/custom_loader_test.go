package agent

import (
	"os"
	"path/filepath"
	"strings"
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

func TestLoadWorkspaceAgentsAppliesExtendedFrontmatterPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	modesDir := filepath.Join(tmpDir, ".agents", "modes")
	if err := os.MkdirAll(modesDir, 0755); err != nil {
		t.Fatalf("failed to create modes dir: %v", err)
	}

	content := `---
name: "team-helper"
description: "Team coordination mode"
readonly: true
allow_question: true
allow_delegate: true
allow_task: false
allow_write: false
allow_brain_write: true
allow_web: false
max_iterations: 77
tool_filter: [read, grep, question, delegate]
exclude_tools: [bash]
---
Coordinate the team.
`
	if err := os.WriteFile(filepath.Join(modesDir, "team-helper.md"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test md: %v", err)
	}

	agents, err := LoadWorkspaceAgents(tmpDir)
	if err != nil {
		t.Fatalf("LoadWorkspaceAgents error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	agent := agents[0]
	if agent.Name != "team-helper" {
		t.Fatalf("expected team-helper, got %q", agent.Name)
	}
	if !strings.Contains(agent.System, "Coordinate the team") {
		t.Fatalf("unexpected system prompt: %q", agent.System)
	}
	if !agent.Permissions.AllowQuestion || !agent.Permissions.AllowDelegate {
		t.Fatalf("expected question+delegate permissions, got %#v", agent.Permissions)
	}
	if agent.Permissions.AllowWrite || agent.Permissions.AllowTask || agent.Permissions.AllowWebAccess == true {
		t.Fatalf("unexpected write/task/web permissions: %#v", agent.Permissions)
	}
	if agent.Permissions.MaxIterations != 77 {
		t.Fatalf("expected max iterations 77, got %d", agent.Permissions.MaxIterations)
	}
	if len(agent.Permissions.AllowedTools) != 4 || len(agent.Permissions.DeniedTools) != 1 {
		t.Fatalf("unexpected tool filters: %#v / %#v", agent.Permissions.AllowedTools, agent.Permissions.DeniedTools)
	}
}
