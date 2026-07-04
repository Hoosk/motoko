package agent

import (
	_ "embed"
)

//go:embed prompts/plan.txt
var promptPlan string

//go:embed prompts/build.txt
var promptBuild string

//go:embed prompts/search.txt
var promptSearch string

//go:embed prompts/learn.txt
var promptLearn string

//go:embed prompts/teamwork.txt
var promptTeamwork string

//go:embed prompts/grill.txt
var promptGrill string

type AgentDef struct {
	Name        string
	System      string
	Permissions AgentPermissions
}

// BuiltinAgents are the compiled-in agents that ship with Motoko.
var BuiltinAgents = []AgentDef{
	{
		Name:        "plan",
		Permissions: DefaultPlanPermissions(),
		System:      promptPlan,
	},
	{
		Name:        "build",
		Permissions: DefaultBuildPermissions(),
		System:      promptBuild,
	},
	{
		Name:        "search",
		Permissions: DefaultSearchPermissions(),
		System:      promptSearch,
	},
	{
		Name:        "learn",
		Permissions: DefaultBuildPermissions(),
		System:      promptLearn,
	},
	{
		Name:        "teamwork",
		Permissions: DefaultPlanPermissions(),
		System:      promptTeamwork,
	},
	{
		Name:        "grill",
		Permissions: DefaultPlanPermissions(),
		System:      promptGrill,
	},
}

// LoadWorkspaceAgents loads custom agents from the workspace.
// It looks for markdown agents in .agents/modes/.
func LoadWorkspaceAgents(workspace string) ([]AgentDef, error) {
	var allCustom []AgentDef

	customDefs, err := LoadCustomAgents(workspace)
	if err == nil {
		for _, c := range customDefs {
			// Convert CustomAgentDef to AgentDef and map permissions
			perms := DefaultBuildPermissions() // Start with base
			if c.Frontmatter.ReadOnly {
				perms = DefaultPlanPermissions()
			}
			applyFrontmatterPermissions(&perms, c.Frontmatter)
			if len(c.Frontmatter.ToolFilter) > 0 {
				perms.AllowedTools = c.Frontmatter.ToolFilter
			}
			if len(c.Frontmatter.ExcludeTools) > 0 {
				perms.DeniedTools = c.Frontmatter.ExcludeTools
			}

			allCustom = append(allCustom, AgentDef{
				Name:        c.Name,
				System:      c.System,
				Permissions: perms,
			})
		}
	}

	return allCustom, nil
}

func applyFrontmatterPermissions(perms *AgentPermissions, frontmatter AgentFrontmatter) {
	if perms == nil {
		return
	}
	if frontmatter.AllowWrite != nil {
		perms.AllowWrite = *frontmatter.AllowWrite
	}
	if frontmatter.AllowQuestion != nil {
		perms.AllowQuestion = *frontmatter.AllowQuestion
	}
	if frontmatter.AllowDelegate != nil {
		perms.AllowDelegate = *frontmatter.AllowDelegate
	}
	if frontmatter.AllowTask != nil {
		perms.AllowTask = *frontmatter.AllowTask
	}
	if frontmatter.AllowBrainWrite != nil {
		perms.AllowBrainWrite = *frontmatter.AllowBrainWrite
	}
	if frontmatter.AllowWebAccess != nil {
		perms.AllowWebAccess = *frontmatter.AllowWebAccess
	}
	if frontmatter.MaxIterations > 0 {
		perms.MaxIterations = frontmatter.MaxIterations
	}
}
