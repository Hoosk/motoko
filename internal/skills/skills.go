package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Skill struct {
	Name        string
	Description string
	Location    string // Absolute path to SKILL.md
	Body        string // Markdown content after frontmatter
}

// Discover searches for SKILL.md files in:
// 1. <workspace>/.agents/skills/
// 2. ~/.agents/skills/
// Project-level skills override user-level skills.
func Discover(workspacePath string) ([]Skill, error) {
	skillsMap := make(map[string]Skill)

	// 1. Scan user-level skills: ~/.agents/skills/
	homeDir, err := os.UserHomeDir()
	if err == nil {
		userSkillsPath := filepath.Join(homeDir, ".agents", "skills")
		scanDir(userSkillsPath, skillsMap)
	}

	// 2. Scan project-level skills: <workspace>/.agents/skills/
	if workspacePath != "" {
		projectSkillsPath := filepath.Join(workspacePath, ".agents", "skills")
		scanDir(projectSkillsPath, skillsMap)
	}

	// Convert map to slice
	var result []Skill
	for _, s := range skillsMap {
		result = append(result, s)
	}

	// Sort skills by name to ensure stable ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

func scanDir(dirPath string, skillsMap map[string]Skill) {
	if info, err := os.Stat(dirPath); err != nil || !info.IsDir() {
		return
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillFilePath := filepath.Join(dirPath, entry.Name(), "SKILL.md")
		if info, err := os.Stat(skillFilePath); err == nil && !info.IsDir() {
			skill, err := ParseSkillFile(skillFilePath)
			if err == nil {
				// If there's a collision, project-level will override user-level
				skillsMap[skill.Name] = skill
			}
		}
	}
}

// ParseSkillFile extracts frontmatter and body from a SKILL.md file.
func ParseSkillFile(location string) (Skill, error) {
	data, err := os.ReadFile(location)
	if err != nil {
		return Skill{}, err
	}
	content := string(data)

	// Frontmatter delimiter
	delim := "---"

	contentTrimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(contentTrimmed, delim) {
		return Skill{}, fmt.Errorf("missing frontmatter delimiter at start")
	}

	// Find the next "---"
	secondDelimIndex := strings.Index(contentTrimmed[len(delim):], delim)
	if secondDelimIndex == -1 {
		return Skill{}, fmt.Errorf("missing closing frontmatter delimiter")
	}

	secondDelimIndex += len(delim) // adjust index relative to original trimmed string

	frontmatter := contentTrimmed[len(delim):secondDelimIndex]
	body := strings.TrimSpace(contentTrimmed[secondDelimIndex+len(delim):])

	// Parse YAML frontmatter simply and leniently
	var name, description string
	lines := strings.Split(frontmatter, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		// Remove enclosing quotes if any
		val = strings.Trim(val, `"'`)

		switch strings.ToLower(key) {
		case "name":
			name = val
		case "description":
			description = val
		}
	}

	if name == "" {
		// Fallback: use directory name as the name if missing from frontmatter
		dirName := filepath.Base(filepath.Dir(location))
		name = dirName
	}

	if description == "" {
		return Skill{}, fmt.Errorf("description is required but missing or empty in frontmatter")
	}

	return Skill{
		Name:        name,
		Description: description,
		Location:    location,
		Body:        body,
	}, nil
}
