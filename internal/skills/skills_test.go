package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSkillFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "motoko-skills-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	skillContent := `---
name: test-skill
description: "Test skill description with : colons"
---
# Test Skill Body
This is a test skill.
`
	skillFilePath := filepath.Join(tmpDir, "SKILL.md")
	if writeErr := os.WriteFile(skillFilePath, []byte(skillContent), 0o644); writeErr != nil {
		t.Fatalf("failed to write skill file: %v", writeErr)
	}

	skill, err := ParseSkillFile(skillFilePath)
	if err != nil {
		t.Fatalf("failed to parse skill file: %v", err)
	}

	if skill.Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got '%s'", skill.Name)
	}

	if skill.Description != "Test skill description with : colons" {
		t.Errorf("expected description 'Test skill description with : colons', got '%s'", skill.Description)
	}

	if skill.Body != "# Test Skill Body\nThis is a test skill." {
		t.Errorf("expected body, got '%s'", skill.Body)
	}
}

func TestDiscover(t *testing.T) {
	// Create mock user and project directories
	tmpWorkspace, err := os.MkdirTemp("", "motoko-workspace-*")
	if err != nil {
		t.Fatalf("failed to create workspace dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpWorkspace) }()

	projSkillsDir := filepath.Join(tmpWorkspace, ".agents", "skills", "proj-skill-1")
	if mkdirErr := os.MkdirAll(projSkillsDir, 0o755); mkdirErr != nil {
		t.Fatalf("failed to create project skills dir: %v", mkdirErr)
	}

	projSkillContent := `---
name: proj-skill-1
description: Project skill description
---
Proj skill body
`
	if writeErr := os.WriteFile(filepath.Join(projSkillsDir, "SKILL.md"), []byte(projSkillContent), 0o644); writeErr != nil {
		t.Fatalf("failed to write project skill: %v", writeErr)
	}

	skills, err := Discover(tmpWorkspace)
	if err != nil {
		t.Fatalf("failed to discover skills: %v", err)
	}

	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	if skills[0].Name != "proj-skill-1" {
		t.Errorf("expected skill name 'proj-skill-1', got '%s'", skills[0].Name)
	}
}
