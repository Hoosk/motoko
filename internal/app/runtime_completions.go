package app

import (
	"context"
	"strings"

	"github.com/Hoosk/motoko/internal/config"
)

func (r *Runtime) MentionSuggestions(input string) []string {
	token, ok := trailingMentionToken(input)
	if !ok {
		return nil
	}
	prefix := strings.ToLower(strings.TrimPrefix(token, "@"))
	var result []string
	for _, name := range r.AgentNames() {
		if prefix == "" || strings.HasPrefix(strings.ToLower(name), prefix) {
			result = append(result, "@"+name)
		}
	}
	if r.semantic != nil {
		if snapshot, err := r.semantic.Ensure(context.Background()); err == nil && snapshot != nil {
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

func (r *Runtime) ReplaceTrailingMention(input, mention string) string {
	token, ok := trailingMentionToken(input)
	if !ok {
		return input
	}
	idx := strings.LastIndex(input, token)
	if idx == -1 {
		return input
	}
	replacement := mention
	if strings.HasPrefix(mention, "@") && r.isAgentMention(mention) {
		replacement += " "
	}
	if strings.HasPrefix(mention, "@") && !r.isAgentMention(mention) {
		replacement += " "
	}
	return input[:idx] + replacement
}

func (r *Runtime) isAgentMention(mention string) bool {
	name := strings.TrimPrefix(strings.TrimSpace(mention), "@")
	for _, agentName := range r.AgentNames() {
		if strings.EqualFold(agentName, name) {
			return true
		}
	}
	return false
}

func (r *Runtime) extractMentionedFiles(input string) []string {
	fields := strings.Fields(input)
	var files []string
	seen := make(map[string]struct{})
	for _, field := range fields {
		if !strings.HasPrefix(field, "@") {
			continue
		}
		mention := strings.TrimPrefix(field, "@")
		if mention == "" || r.isAgentMention(field) {
			continue
		}
		if _, ok := seen[mention]; ok {
			continue
		}
		seen[mention] = struct{}{}
		files = append(files, mention)
	}
	return files
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

func (r *Runtime) Completions(input string) []string {
	trimmed := strings.TrimSpace(input)
	hasTrailingSpace := strings.HasSuffix(input, " ")
	if trimmed == "" {
		if r.inputMode == InputModeShell {
			return []string{"ls", "pwd", "git status", "go build ./...", "/chat"}
		}
		return []string{"/help", "/provider add", "/models", "/sessions", "/tool read README.md", "!git status"}
	}

	if r.inputMode == InputModeShell && !strings.HasPrefix(trimmed, "/") && !strings.HasPrefix(trimmed, "!") {
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

	if strings.EqualFold(parts[0], cmdTool) {
		prefix := ""
		if len(parts) > 1 {
			prefix = parts[1]
		}
		matches := r.ToolSuggestions(prefix)
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
		for _, name := range r.AgentNames() {
			if prefix == "" || strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
				result = append(result, "/agent "+name)
			}
		}
		return result
	}

	if strings.EqualFold(parts[0], "models") {
		active, ok := r.config.Active()
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

	return nil
}

func (r *Runtime) cacheProviderModels(providerName string, models []string) {
	if r.config == nil || strings.TrimSpace(providerName) == "" || len(models) == 0 {
		return
	}
	providerCfg, ok := r.config.Provider(providerName)
	if !ok {
		return
	}
	providerCfg.Models = config.UniqueSortedKeep(providerCfg.Models, models...)
	r.config.UpsertProvider(providerCfg)
	_ = r.config.Save()
}

func commandCompletions(prefix string) []string {
	commands := []string{"help", cmdClear, "compact", "mode", string(ModePlan), string(ModeBuild), "agent", "shell", "chat", cmdStatus, "debug", "trace", "context", "provider", "models", "sessions", "tools", cmdTool, "approve", "deny", "metrics"}
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
