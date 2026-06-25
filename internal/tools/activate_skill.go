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
		Name:    "activate_skill",
		Summary: "Activa y carga las instrucciones detalladas de una habilidad (skill) del catalogo.",
		Usage:   "activate_skill <nombre>",
	}
}

func (t *ActivateSkillTool) DynamicSpec(ctx ToolContext) Spec {
	spec := t.Spec()
	if len(ctx.AvailableSkills) > 0 {
		var xmlBuilder strings.Builder
		xmlBuilder.WriteString("Activa y carga las instrucciones de un skill. Skills disponibles:\n<available-skills>\n")
		for _, s := range ctx.AvailableSkills {
			fmt.Fprintf(&xmlBuilder, "  <skill name=\"%s\" />\n", s)
		}
		xmlBuilder.WriteString("</available-skills>")
		spec.Summary = xmlBuilder.String()
		spec.Usage = fmt.Sprintf("activate_skill %s", strings.Join(ctx.AvailableSkills, "|"))
	}
	return spec
}

func (t *ActivateSkillTool) Run(ctx context.Context, args string) (Result, error) {
	_ = ctx
	args = strings.TrimSpace(args)
	if args == "" {
		return Result{}, fmt.Errorf("uso: %s", t.Spec().Usage)
	}

	// Case-insensitive lookup
	var found *skills.Skill
	for i, s := range t.availableSkills {
		if strings.EqualFold(s.Name, args) {
			found = &t.availableSkills[i]
			break
		}
	}

	if found == nil {
		return Result{}, fmt.Errorf("habilidad desconocida: %s", args)
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
		Summary: fmt.Sprintf("Habilidad %s activada exitosamente.", found.Name),
		Output:  outputBuilder.String(),
	}, nil
}
