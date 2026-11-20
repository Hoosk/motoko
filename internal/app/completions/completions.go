package completions

import (
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Hoosk/motoko/internal/app/commands"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/semantic"
	"github.com/Hoosk/motoko/internal/tools"

	"github.com/Hoosk/motoko/internal/app/types"
)

const (
	modelsListCmd = "/models list"
	subCmdUse     = "use"
)

type Deps struct {
	AgentNamesFn          func() []string
	SemanticFn            func() *semantic.Index
	InputModeFn           func() types.InputMode
	ToolSuggestionsFn     func(prefix string) []tools.Spec
	ActiveConfigFn        func() (config.ProviderConfig, bool)
	ConfiguredProvidersFn func() []config.ProviderConfig
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

	if after, ok := strings.CutPrefix(trimmed, "!"); ok {
		command := strings.TrimSpace(after)
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

	switch {
	case strings.EqualFold(parts[0], commands.CmdTool):
		return toolCompletions(d, parts)
	case strings.EqualFold(parts[0], "agent"):
		return agentCompletions(d, parts)
	case strings.EqualFold(parts[0], "models"):
		return modelsCompletions(d, parts, hasTrailingSpace)
	case strings.EqualFold(parts[0], "provider"):
		return providerCompletions(d, parts, hasTrailingSpace)
	case strings.EqualFold(parts[0], "themes"):
		return themesCompletions(parts)
	}

	return nil
}

func toolCompletions(d Deps, parts []string) []string {
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

func agentCompletions(d Deps, parts []string) []string {
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

func modelsCompletions(d Deps, parts []string, hasTrailingSpace bool) []string {
	active, ok := d.ActiveConfigFn()
	if !ok || len(active.Models) == 0 {
		if len(parts) == 1 {
			return []string{modelsListCmd, "/models use "}
		}
		return []string{modelsListCmd}
	}

	if len(parts) == 1 {
		return []string{modelsListCmd, "/models use ", "/models info "}
	}

	subcommand := strings.ToLower(parts[1])
	if len(parts) == 2 && !hasTrailingSpace {
		if result, matched := subcommandOptions([]string{"list", subCmdUse, "info"}, subcommand, "/models "); matched {
			return result
		}
	}

	if subcommand != subCmdUse && subcommand != "info" {
		prefix := strings.Join(parts[1:], " ")
		return modelCompletions(active.Models, prefix, "/models ")
	}

	prefix := ""
	if len(parts) > 2 {
		prefix = strings.Join(parts[2:], " ")
	}
	return modelCompletions(active.Models, prefix, "/models "+subcommand+" ")
}

func providerCompletions(d Deps, parts []string, hasTrailingSpace bool) []string {
	if len(parts) == 1 {
		return []string{"/provider list", "/provider add", "/provider use ", "/provider remove "}
	}
	subcommand := strings.ToLower(parts[1])
	if len(parts) == 2 && !hasTrailingSpace {
		if result, matched := subcommandOptions([]string{"list", "add", "use", "remove"}, subcommand, "/provider "); matched {
			return result
		}
	}
	if subcommand == "use" || subcommand == "remove" {
		prefix := ""
		if len(parts) > 2 {
			prefix = strings.Join(parts[2:], " ")
		}
		var result []string
		for _, p := range d.ConfiguredProvidersFn() {
			if prefix == "" || strings.HasPrefix(strings.ToLower(p.Name), strings.ToLower(prefix)) {
				result = append(result, "/provider "+subcommand+" "+p.Name)
			}
		}
		return result
	}
	return nil
}

func themesCompletions(parts []string) []string {
	prefix := ""
	if len(parts) > 1 {
		prefix = strings.ToLower(parts[1])
	}
	allThemes := []string{commands.ThemeCyberpunk, "ghost-cyber", "neon-shadow", "black-ice", "nord", "dracula", "monochrome"}
	var result []string
	for _, t := range allThemes {
		if prefix == "" || strings.HasPrefix(t, prefix) {
			result = append(result, "/themes "+t)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// subcommandOptions filters the available subcommands by the typed prefix.
// It reports whether any option matched so callers can fall through to
// value-based completion when nothing did.
func subcommandOptions(options []string, subcommand, commandPrefix string) ([]string, bool) {
	var result []string
	for _, option := range options {
		if strings.HasPrefix(option, subcommand) {
			result = append(result, commandPrefix+option)
		}
	}
	return result, len(result) > 0
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
	// This runs on every keystroke inside the UI update, so it must never
	// trigger a (potentially seconds long) re-index: use whatever snapshot
	// CodeTachikoma last produced.
	if sem := d.SemanticFn(); sem != nil {
		if snapshot := sem.LatestSnapshot(); snapshot != nil {
			result = append(result, fileMentions(snapshot.Files, prefix)...)
		}
	}
	if len(result) > 8 {
		result = result[:8]
	}
	return result
}

// fileMentions returns "@path" for files matching prefix, best match first:
// exact path or file name, then file name prefix, then path prefix, then any
// substring. Without ranking, "@model.go" could complete to model_picker.go.
func fileMentions(files []semantic.FileSummary, prefix string) []string {
	type ranked struct {
		path string
		rank int
	}
	var matches []ranked
	seen := make(map[string]struct{})
	for _, file := range files {
		if _, ok := seen[file.Path]; ok {
			continue
		}
		lowerPath := strings.ToLower(file.Path)
		base := strings.ToLower(filepath.Base(file.Path))
		rank := -1
		switch {
		case prefix == "":
			rank = 0
		case lowerPath == prefix || base == prefix:
			rank = 0
		case strings.HasPrefix(base, prefix):
			rank = 1
		case strings.HasPrefix(lowerPath, prefix):
			rank = 2
		case strings.Contains(lowerPath, prefix):
			rank = 3
		}
		if rank < 0 {
			continue
		}
		seen[file.Path] = struct{}{}
		matches = append(matches, ranked{path: file.Path, rank: rank})
	}
	slices.SortStableFunc(matches, func(a, b ranked) int { return a.rank - b.rank })
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		result = append(result, "@"+match.path)
	}
	return result
}

// trailingMentionToken returns the "@..." word being typed at the end of
// input. Once the input ends in whitespace the mention is complete; reporting
// it would keep the dropdown open and make Enter re-apply it forever.
func trailingMentionToken(input string) (string, bool) {
	if input == "" {
		return "", false
	}
	if last, _ := utf8.DecodeLastRuneInString(input); unicode.IsSpace(last) {
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
	defs := commands.CommandDefinitions()
	prefix = strings.ToLower(prefix)
	result := make([]string, 0, len(defs))
	for _, def := range defs {
		if strings.HasPrefix(def.Name, prefix) {
			result = append(result, "/"+def.Name)
		}
	}
	return result
}

func modelCompletions(models []string, prefix, commandPrefix string) []string {
	prefix = strings.ToLower(prefix)
	var result []string
	for _, model := range models {
		if prefix == "" || strings.HasPrefix(strings.ToLower(model), prefix) {
			result = append(result, commandPrefix+model)
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
	return result
}
