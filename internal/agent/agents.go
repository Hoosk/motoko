package agent

import (
	"bufio"
	"os"
	"strings"
)

// AgentDef defines a named agent with a mode-specific system prompt.
type AgentDef struct {
	Name   string
	System string
}

// BuiltinAgents are the two compiled-in agents that ship with Motoko.
var BuiltinAgents = []AgentDef{
	{
		Name:   "plan",
		System: "Modo plan: lee y analiza el código, proporciona planes y diagnósticos. NO escribas ni modifiques archivos sin aprobación explícita. Explica lo que harías antes de hacerlo.",
	},
	{
		Name:   "build",
		System: "Modo build: implementa cambios en el código directamente y de forma precisa. Verifica siempre el estado actual antes de escribir. Prefiere cambios incrementales y verificables.",
	},
}

// LoadAgentsFile reads a .agents INI file from path.
// Returns nil error and empty slice if the file does not exist.
func LoadAgentsFile(path string) ([]AgentDef, error) {
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
