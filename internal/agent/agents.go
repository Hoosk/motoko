package agent

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// AgentDef defines a named agent with a mode-specific system prompt.
type AgentDef struct {
	Name   string
	System string
}

// BuiltinAgents are the compiled-in agents that ship with Motoko.
var BuiltinAgents = []AgentDef{
	{
		Name:   "plan",
		System: "Plan mode: read and analyze code, provide plans and diagnostics. DO NOT write or modify files without explicit approval. Explain what you would do before doing it.",
	},
	{
		Name:   "build",
		System: "Build mode: implement code changes directly and precisely. Always verify current state before writing. Prefer incremental and verifiable changes.",
	},
	{
		Name:   "search",
		System: "Search mode: explore and locate files, classes, methods, variables, and code patterns within the codebase. Formulate precise search strategy using grep, glob, inspect, and read tools. Report the exact location and usage of symbols clearly.",
	},
}

// LoadAgentsFile reads a .agents INI file from path.
// Returns nil error and empty slice if the file does not exist.
func LoadAgentsFile(path string) ([]AgentDef, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	if info.IsDir() {
		// Fallback: look for 'agents', 'agents.ini', or 'config' inside the directory
		candidates := []string{
			filepath.Join(path, "agents"),
			filepath.Join(path, "agents.ini"),
			filepath.Join(path, "config"),
		}
		foundFile := false
		for _, cand := range candidates {
			if candInfo, candErr := os.Stat(cand); candErr == nil && !candInfo.IsDir() {
				path = cand
				foundFile = true
				break
			}
		}
		if !foundFile {
			// It is a directory, but no agents config file was found.
			return nil, nil
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return ParseAgentsFile(string(data)), nil
}

// ParseAgentsFile parses a .agents INI file content.
// Format:
//
//	[agent-name]
//	system = one-line system prompt
//	system = continuation line (appended with newline)
func ParseAgentsFile(content string) []AgentDef {
	var agents []AgentDef
	var current *AgentDef
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if current != nil && strings.TrimSpace(current.Name) != "" {
				agents = append(agents, *current)
			}
			name := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			current = &AgentDef{Name: name}
			continue
		}
		if current == nil {
			continue
		}
		if strings.HasPrefix(trimmed, "system") {
			rest := strings.TrimPrefix(trimmed, "system")
			rest = strings.TrimSpace(rest)
			if strings.HasPrefix(rest, "=") {
				rest = strings.TrimSpace(strings.TrimPrefix(rest, "="))
				if current.System == "" {
					current.System = rest
				} else {
					current.System += "\n" + rest
				}
			}
		}
	}
	if current != nil && strings.TrimSpace(current.Name) != "" {
		agents = append(agents, *current)
	}
	return agents
}
