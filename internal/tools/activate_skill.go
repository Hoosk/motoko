package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Hoosk/motoko/internal/skills"
)

type ActivateSkillTool struct {
	availableSkills []skills.Skill
}

func NewActivateSkillTool(available []skills.Skill) *ActivateSkillTool {
	return &ActivateSkillTool{availableSkills: available}
}

func (t *ActivateSkillTool) Spec() Spec {
	return Spec{
		Name:        "activate_skill",
		Summary:     "Activates and loads detailed instructions of a skill from the catalog.",
		Usage:       `activate_skill {"name": "golang-testing"}`,
		InputSchema: schemaActivateSkill,
	}
}

func (t *ActivateSkillTool) DynamicSpec(ctx ToolContext) Spec {
	spec := t.Spec()
	if len(ctx.AvailableSkills) > 0 {
		var xmlBuilder strings.Builder
		xmlBuilder.WriteString("Activates and loads the instructions of a skill. Available skills:\n<available-skills>\n")
		for _, s := range ctx.AvailableSkills {
			fmt.Fprintf(&xmlBuilder, "  <skill name=\"%s\" />\n", s)
		}
		xmlBuilder.WriteString("</available-skills>")
		spec.Summary = xmlBuilder.String()
		spec.Usage = fmt.Sprintf(`activate_skill {"name": "<%s>"}`, strings.Join(ctx.AvailableSkills, "|"))
	}
	return spec
}

func (t *ActivateSkillTool) Run(ctx context.Context, args string) (Result, error) {
	_ = ctx
	name := ""
	if parsed := parseJSONArgs(args); parsed != nil {
		name = jsonStr(parsed, "name", "skill", "skill_name", "skillName")
	}
	if name == "" {
		return Result{}, fmt.Errorf("usage: %s", t.Spec().Usage)
	}

	// Case-insensitive lookup
	var found *skills.Skill
	for i, s := range t.availableSkills {
		if strings.EqualFold(s.Name, name) {
			found = &t.availableSkills[i]
			break
		}
	}

	if found == nil {
		return Result{}, fmt.Errorf("unknown skill: %s", name)
	}

	// Structured Wrapping as described in the specification:
	// We wrap skill content in xml tags, which has practical benefits:
	// - The model can clearly distinguish skill instructions from other conversation content
	// - Relative paths can be resolved against the skill's base directory
	outputBuilder := &strings.Builder{}
	fmt.Fprintf(outputBuilder, "<skill_content name=%q>\n", found.Name)
	outputBuilder.WriteString(found.Body)
	outputBuilder.WriteString("\n\n")
	fmt.Fprintf(outputBuilder, "Skill directory: %s\n", filepath.Dir(found.Location))
	outputBuilder.WriteString("Relative paths in this skill are relative to the skill directory.\n")
	outputBuilder.WriteString("</skill_content>")

	return Result{
		Spec:    t.Spec(),
		Summary: fmt.Sprintf("Skill %s activated successfully.", found.Name),
		Output:  outputBuilder.String(),
	}, nil
}
