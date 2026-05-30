package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/skills"
)

func TestActivateSkillTool(t *testing.T) {
	mockSkills := []skills.Skill{
		{
			Name:        "test-skill",
			Description: "Test skill description",
			Location:    "/tmp/test-skill/SKILL.md",
			Body:        "Test skill instructions body",
		},
	}

	tool := NewActivateSkillTool(mockSkills)
	spec := tool.Spec()

	if spec.Name != "activate_skill" {
		t.Errorf("expected tool name 'activate_skill', got '%s'", spec.Name)
	}

	if !strings.Contains(spec.Usage, "test-skill") {
		t.Errorf("expected usage to contain 'test-skill', got '%s'", spec.Usage)
	}

	// Test successful run
	res, err := tool.Run(context.Background(), "test-skill")
	if err != nil {
		t.Fatalf("failed to run tool: %v", err)
	}

	if !strings.Contains(res.Output, "Test skill instructions body") {
		t.Errorf("expected output to contain skill body, got '%s'", res.Output)
	}

	if !strings.Contains(res.Output, "<skill_content name=\"test-skill\">") {
		t.Errorf("expected output to have structured wrapping tag, got '%s'", res.Output)
	}

	// Test case-insensitive
	resCase, err := tool.Run(context.Background(), "TEST-SKILL")
	if err != nil {
		t.Fatalf("failed case-insensitive run: %v", err)
	}
	if !strings.Contains(resCase.Output, "Test skill instructions body") {
		t.Errorf("expected case-insensitive output to contain skill body")
	}

	// Test run with empty args
	_, errEmpty := tool.Run(context.Background(), "")
	if errEmpty == nil {
		t.Error("expected error with empty args, got nil")
	}

	// Test run with unknown skill
	_, errUnknown := tool.Run(context.Background(), "unknown-skill")
	if errUnknown == nil {
		t.Error("expected error with unknown skill, got nil")
	}
}
