package agent

import (
	"os"
	"path/filepath"
	"strings"
)

// AgentFrontmatter represents the parsed YAML frontmatter from a custom agent markdown file.
type AgentFrontmatter struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	ReadOnly     bool     `yaml:"readonly"`
	ToolFilter   []string `yaml:"tool_filter"`
	ExcludeTools []string `yaml:"exclude_tools"`
}

// CustomAgentDef represents an agent loaded from a markdown file.
type CustomAgentDef struct {
	AgentDef
	Frontmatter AgentFrontmatter
}

// LoadCustomAgents loads all custom agents defined in markdown files from .agents/modes/.
func LoadCustomAgents(workspace string) ([]CustomAgentDef, error) {
	modesDir := filepath.Join(workspace, ".agents", "modes")
	entries, err := os.ReadDir(modesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Directory doesn't exist, no custom agents
		}
		return nil, err
	}

	var customAgents []CustomAgentDef
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(modesDir, entry.Name())
		agentDef, err := parseCustomAgentFile(path)
		if err != nil {
			continue // Skip files with errors for now, or log them
		}
		customAgents = append(customAgents, agentDef)
	}

	return customAgents, nil
}

func parseCustomAgentFile(path string) (CustomAgentDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CustomAgentDef{}, err
	}

	content := string(data)
	lines := strings.Split(content, "\n")
	
	var frontmatter AgentFrontmatter
	var systemPromptBuilder strings.Builder
	
	inFrontmatter := false
	frontmatterDone := false
	
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		
		if !frontmatterDone && trimmedLine == "---" {
			if !inFrontmatter {
				inFrontmatter = true
			} else {
				inFrontmatter = false
				frontmatterDone = true
			}
			continue
		}
		
		if inFrontmatter {
			// Simple key-value parsing for frontmatter
			parts := strings.SplitN(trimmedLine, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				
				// Strip quotes from value if present
				if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
					value = value[1 : len(value)-1]
				} else if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
					value = value[1 : len(value)-1]
				}
				
				switch key {
				case "name":
					frontmatter.Name = value
				case "description":
					frontmatter.Description = value
				case "readonly":
					frontmatter.ReadOnly = strings.ToLower(value) == "true"
				case "tool_filter":
					frontmatter.ToolFilter = parseList(value)
				case "exclude_tools":
					frontmatter.ExcludeTools = parseList(value)
				}
			}
		} else if frontmatterDone || (!frontmatterDone && !inFrontmatter) {
			systemPromptBuilder.WriteString(line)
			systemPromptBuilder.WriteString("\n")
		}
	}
	
	name := frontmatter.Name
	if name == "" {
		// Fallback to filename without extension
		base := filepath.Base(path)
		name = strings.TrimSuffix(base, filepath.Ext(base))
		frontmatter.Name = name
	}
	
	systemPrompt := strings.TrimSpace(systemPromptBuilder.String())
	
	return CustomAgentDef{
		AgentDef: AgentDef{
			Name:   name,
			System: systemPrompt,
		},
		Frontmatter: frontmatter,
	}, nil
}

// parseList parses a list like [tool1, tool2] or tool1, tool2 into a string slice
func parseList(value string) []string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		value = value[1 : len(value)-1]
	}
	if value == "" {
		return nil
	}
	
	parts := strings.Split(value, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			// Strip quotes from individual items
			if strings.HasPrefix(p, "\"") && strings.HasSuffix(p, "\"") {
				p = p[1 : len(p)-1]
			} else if strings.HasPrefix(p, "'") && strings.HasSuffix(p, "'") {
				p = p[1 : len(p)-1]
			}
			result = append(result, p)
		}
	}
	return result
}
