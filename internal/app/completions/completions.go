package completions

import (
	"context"
	"strings"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/semantic"
	"github.com/Hoosk/motoko/internal/tools"

	"github.com/Hoosk/motoko/internal/app/types"
)

const (
	ThemeCyberpunk = "cyberpunk"
	CmdTool        = "tool"
	CmdClear       = "clear"
	CmdStatus      = "status"
)

type Deps struct {
	AgentNamesFn      func() []string
	SemanticFn        func() *semantic.Index
	InputModeFn       func() types.InputMode
	ToolSuggestionsFn func(prefix string) []tools.Spec
	ActiveConfigFn    func() (config.ProviderConfig, bool)
}

func Completions(d Deps, input string) []string {
	trimmed := strings.TrimSpace(input)
	hasTrailingSpace := strings.HasSuffix(input, " ")
	if trimmed == "" {
		if d.InputModeFn() == types.InputModeShell {
			return []string{"ls", "pwd", "git status", "go build ./...", "/chat"}
		}
		return []string{"/help", "/provider add", "/models", "/sessions", "/tool read README.md", "!git status"}
	}

	if d.InputModeFn() == types.InputModeShell && !strings.HasPrefix(trimmed, "/") && !strings.HasPrefix(trimmed, "!") {
		return shellCompletions(trimmed)
	}

	if strings.HasPrefix(trimmed, "!") {
		command := strings.TrimSpace(strings.TrimPrefix(trimmed, "!"))
		if command == "" {
			return []string{"!git status", "!go build ./...", "!ls"}
		}
		return []string{"!" + command}
	}

	if !strings.HasPrefix(trimmed, "/") {
		return nil
	}

	parts := strings.Fields(strings.TrimPrefix(trimmed, "/"))
	if len(parts) == 0 {
		return commandCompletions("")
	}

	if len(parts) == 1 && !hasTrailingSpace {
		return commandCompletions(parts[0])
	}

	if strings.EqualFold(parts[0], CmdTool) {
		prefix := ""
		if len(parts) > 1 {
			prefix = parts[1]
		}
		matches := d.ToolSuggestionsFn(prefix)
		result := make([]string, 0, len(matches))
		for _, spec := range matches {
			result = append(result, "/tool "+spec.Usage)
		}
		return result
	}

	if strings.EqualFold(parts[0], "agent") {
		prefix := ""
		if len(parts) > 1 {
			prefix = parts[1]
		}
		var result []string
		for _, name := range d.AgentNamesFn() {
			if prefix == "" || strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
				result = append(result, "/agent "+name)
			}
		}
		return result
	}

	if strings.EqualFold(parts[0], "models") {
		active, ok := d.ActiveConfigFn()
		if !ok || len(active.Models) == 0 {
			return []string{"/models"}
		}
		prefix := ""
		if len(parts) > 1 {
			prefix = strings.Join(parts[1:], " ")
		}
		var result []string
		for _, model := range active.Models {
			if prefix == "" || strings.HasPrefix(strings.ToLower(model), strings.ToLower(prefix)) {
				result = append(result, "/models "+model)
			}
		}
		if len(result) > 0 {
			return result
		}
	}

	if strings.EqualFold(parts[0], "themes") {
		prefix := ""
		if len(parts) > 1 {
			prefix = strings.ToLower(parts[1])
		}
		allThemes := []string{ThemeCyberpunk, "ghost-cyber", "neon-shadow", "black-ice", "nord", "dracula", "monochrome"}
		var result []string
		for _, t := range allThemes {
			if prefix == "" || strings.HasPrefix(t, prefix) {
				result = append(result, "/themes "+t)
			}
		}
		if len(result) > 0 {
			return result
		}
	}

	return nil
}

func MentionSuggestions(d Deps, input string) []string {
	token, ok := trailingMentionToken(input)
	if !ok {
		return nil
	}
	prefix := strings.ToLower(strings.TrimPrefix(token, "@"))
	var result []string
	for _, name := range d.AgentNamesFn() {
		if prefix == "" || strings.HasPrefix(strings.ToLower(name), prefix) {
			result = append(result, "@"+name)
		}
	}
	sem := d.SemanticFn()
	if sem != nil {
		if snapshot, err := sem.Ensure(context.Background()); err == nil && snapshot != nil {
			seen := make(map[string]struct{})
			for _, file := range snapshot.Files {
				path := file.Path
				if _, ok := seen[path]; ok {
					continue
				}
				if prefix == "" || strings.Contains(strings.ToLower(path), prefix) {
					seen[path] = struct{}{}
					result = append(result, "@"+path)
				}
			}
		}
	}
	if len(result) > 8 {
		result = result[:8]
	}
	return result
}

func trailingMentionToken(input string) (string, bool) {
	if input == "" {
		return "", false
	}
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return "", false
	}
	last := fields[len(fields)-1]
	if !strings.HasPrefix(last, "@") {
		return "", false
	}
	return last, true
}

func commandCompletions(prefix string) []string {
	commands := []string{"help", CmdClear, "compact", "mode", string(types.ModePlan), string(types.ModeBuild), "agent", "shell", "chat", CmdStatus, "debug", "trace", "context", "provider", "models", "themes", "sessions", "tools", CmdTool, "approve", "deny", "metrics"}
	prefix = strings.ToLower(prefix)
	var result []string
	for _, command := range commands {
		if strings.HasPrefix(command, prefix) {
			result = append(result, "/"+command)
		}
	}
	return result
}

func shellCompletions(prefix string) []string {
	commands := []string{"ls", "pwd", "git status", "git diff", "go build ./...", "go test ./...", "npm test"}
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if prefix == "" {
		return commands
	}

	var result []string
	for _, command := range commands {
		if strings.HasPrefix(strings.ToLower(command), prefix) {
			result = append(result, command)
		}
	}
	if len(result) == 0 {
		return []string{prefix}
	}
	return result
}
