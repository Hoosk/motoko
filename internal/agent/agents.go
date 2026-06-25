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
