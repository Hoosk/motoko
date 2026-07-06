package completions

import (
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/semantic"
	"github.com/Hoosk/motoko/internal/tools"

	"github.com/Hoosk/motoko/internal/app/types"
)

func noopDeps() Deps {
	return Deps{
		AgentNamesFn:          func() []string { return nil },
		SemanticFn:            func() *semantic.Index { return nil },
		InputModeFn:           func() types.InputMode { return types.InputModeChat },
		ToolSuggestionsFn:     func(prefix string) []tools.Spec { return nil },
		ActiveConfigFn:        func() (config.ProviderConfig, bool) { return config.ProviderConfig{}, false },
		ConfiguredProvidersFn: func() []config.ProviderConfig { return nil },
	}
}

func depsWithAgents(names []string) Deps {
	d := noopDeps()
	d.AgentNamesFn = func() []string { return names }
	return d
}

func depsWithShell() Deps {
	d := noopDeps()
	d.InputModeFn = func() types.InputMode { return types.InputModeShell }
	return d
}

func TestCompletionsEmptyInput(t *testing.T) {
	got := Completions(noopDeps(), "")
	if len(got) == 0 {
		t.Fatal("expected non-empty completions for empty input")
	}
	for _, c := range got {
		if !strings.HasPrefix(c, "/") && !strings.HasPrefix(c, "!") && !strings.HasPrefix(c, "ls") && !strings.HasPrefix(c, "pwd") && !strings.HasPrefix(c, "git") && !strings.HasPrefix(c, "go ") {
			t.Errorf("unexpected completion: %q", c)
		}
	}
}

func TestCompletionsEmptyInputShellMode(t *testing.T) {
	got := Completions(depsWithShell(), "")
	if len(got) == 0 {
		t.Fatal("expected shell completions for empty input")
	}
	foundLs := false
	for _, c := range got {
		if c == "ls" {
			foundLs = true
		}
	}
	if !foundLs {
		t.Error("expected 'ls' in shell completions")
	}
}

func TestCompletionsSlashPrefix(t *testing.T) {
	got := Completions(noopDeps(), "/h")
	if len(got) == 0 {
		t.Fatal("expected completions for /h")
	}
	for _, c := range got {
		if !strings.HasPrefix(c, "/help") {
			t.Errorf("expected /help completion, got %q", c)
		}
	}
}

func TestCompletionsSlashExactMatchNoSpace(t *testing.T) {
	got := Completions(noopDeps(), "/help")
	for _, c := range got {
		if !strings.HasPrefix(c, "/help") {
			t.Errorf("expected /help completion, got %q", c)
		}
	}
}

func TestCompletionsBangPrefix(t *testing.T) {
	got := Completions(noopDeps(), "!")
	if len(got) == 0 {
		t.Fatal("expected bang completions")
	}
	for _, c := range got {
		if !strings.HasPrefix(c, "!") {
			t.Errorf("expected ! prefix, got %q", c)
		}
	}
}

func TestCompletionsNonSlashNonBangInChat(t *testing.T) {
	got := Completions(noopDeps(), "hello")
	if got != nil {
		t.Errorf("expected nil for non-slash input in chat mode, got %v", got)
	}
}

func TestCompletionsNonSlashInShell(t *testing.T) {
	got := Completions(depsWithShell(), "git")
	if len(got) == 0 {
		t.Fatal("expected shell command completions for 'git'")
	}
	for _, c := range got {
		if !strings.HasPrefix(c, "git") {
			t.Errorf("expected 'git' prefix, got %q", c)
		}
	}
}

func TestCompletionsAgentSubcommand(t *testing.T) {
	d := depsWithAgents([]string{"plan", "build", "search"})
	got := Completions(d, "/agent ")
	if len(got) == 0 {
		t.Fatal("expected agent completions")
	}
	for _, c := range got {
		if !strings.HasPrefix(c, "/agent ") {
			t.Errorf("expected /agent prefix, got %q", c)
		}
	}
}

func TestCompletionsAgentSubcommandWithPrefix(t *testing.T) {
	d := depsWithAgents([]string{"plan", "build", "search"})
	got := Completions(d, "/agent s")
	if len(got) == 0 {
		t.Fatal("expected agent completions for /agent s")
	}
	for _, c := range got {
		if !strings.Contains(c, "search") {
			t.Errorf("expected 'search' in completion, got %q", c)
		}
	}
}

func TestCompletionsModelsSubcommand(t *testing.T) {
	d := noopDeps()
	d.ActiveConfigFn = func() (config.ProviderConfig, bool) {
		return config.ProviderConfig{
			Models: []string{"gpt-4.1", "gpt-3.5-turbo"},
		}, true
	}
	got := Completions(d, "/models ")
	if len(got) == 0 {
		t.Fatal("expected model completions")
	}
	want := []string{"/models list", "/models use ", "/models info "}
	if !slicesEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestCompletionsModelsSubcommandNoActive(t *testing.T) {
	d := noopDeps()
	got := Completions(d, "/models ")
	want := []string{"/models list", "/models use "}
	if !slicesEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestCompletionsModelsUseSubcommand(t *testing.T) {
	d := noopDeps()
	d.ActiveConfigFn = func() (config.ProviderConfig, bool) {
		return config.ProviderConfig{Models: []string{"gpt-4.1", "gpt-3.5-turbo"}}, true
	}
	got := Completions(d, "/models use gpt")
	want := []string{"/models use gpt-4.1", "/models use gpt-3.5-turbo"}
	if !slicesEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestCompletionsProviderSubcommand(t *testing.T) {
	d := noopDeps()
	d.ConfiguredProvidersFn = func() []config.ProviderConfig {
		return []config.ProviderConfig{
			{Name: "openai"},
			{Name: "deepseek"},
		}
	}
	got := Completions(d, "/provider ")
	want := []string{"/provider list", "/provider add", "/provider use ", "/provider remove "}
	if !slicesEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestCompletionsProviderSubcommandPrefix(t *testing.T) {
	d := noopDeps()
	got := Completions(d, "/provider u")
	want := []string{"/provider use"}
	if !slicesEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestCompletionsProviderUseFiltersPrefix(t *testing.T) {
	d := noopDeps()
	d.ConfiguredProvidersFn = func() []config.ProviderConfig {
		return []config.ProviderConfig{
			{Name: "openai"},
			{Name: "deepseek"},
			{Name: "anthropic"},
		}
	}
	got := Completions(d, "/provider use ")
	want := []string{"/provider use openai", "/provider use deepseek", "/provider use anthropic"}
	if !slicesEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestCompletionsProviderUsePrefixFilter(t *testing.T) {
	d := noopDeps()
	d.ConfiguredProvidersFn = func() []config.ProviderConfig {
		return []config.ProviderConfig{
			{Name: "openai"},
			{Name: "deepseek"},
		}
	}
	got := Completions(d, "/provider use d")
	want := []string{"/provider use deepseek"}
	if !slicesEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestCompletionsProviderRemovePrefixFilter(t *testing.T) {
	d := noopDeps()
	d.ConfiguredProvidersFn = func() []config.ProviderConfig {
		return []config.ProviderConfig{
			{Name: "openai"},
			{Name: "deepseek"},
		}
	}
	got := Completions(d, "/provider remove o")
	want := []string{"/provider remove openai"}
	if !slicesEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestCompletionsThemesSubcommand(t *testing.T) {
	got := Completions(noopDeps(), "/themes ")
	if len(got) == 0 {
		t.Fatal("expected theme completions")
	}
	for _, c := range got {
		if !strings.HasPrefix(c, "/themes ") {
			t.Errorf("expected /themes prefix, got %q", c)
		}
	}
}

func TestCompletionsThemesSubcommandWithPrefix(t *testing.T) {
	got := Completions(noopDeps(), "/themes c")
	if len(got) == 0 {
		t.Fatal("expected theme completions for /themes c")
	}
	for _, c := range got {
		if !strings.Contains(c, "cyber") {
			t.Errorf("expected 'cyber' in completion, got %q", c)
		}
	}
}

func TestCompletionsToolSubcommand(t *testing.T) {
	d := noopDeps()
	d.ToolSuggestionsFn = func(prefix string) []tools.Spec {
		return []tools.Spec{
			{Usage: "read file.md"},
			{Usage: "write file.go"},
		}
	}
	got := Completions(d, "/tool ")
	if len(got) != 2 {
		t.Fatalf("expected 2 completions, got %v", got)
	}
	if got[0] != "/tool read file.md" || got[1] != "/tool write file.go" {
		t.Errorf("unexpected tool completions: %v", got)
	}
}

func TestCompletionsNonSlashWithTrailingSpace(t *testing.T) {
	got := Completions(noopDeps(), "hi ")
	for _, c := range got {
		t.Errorf("expected nil for non-slash with space, got %v", c)
	}
}

func TestMentionSuggestionsNoToken(t *testing.T) {
	got := MentionSuggestions(noopDeps(), "hello")
	if got != nil {
		t.Errorf("expected nil for no @ token, got %v", got)
	}
}

func TestMentionSuggestionsAgentPrefix(t *testing.T) {
	d := depsWithAgents([]string{"plan", "build", "search"})
	got := MentionSuggestions(d, "use @p")
	if len(got) == 0 {
		t.Fatal("expected mention suggestions for @p")
	}
	foundPlan := false
	for _, c := range got {
		if c == "@plan" {
			foundPlan = true
		}
	}
	if !foundPlan {
		t.Errorf("expected @plan in suggestions, got %v", got)
	}
}

func TestMentionSuggestionsExactAgent(t *testing.T) {
	d := depsWithAgents([]string{"plan", "build"})
	got := MentionSuggestions(d, "use @plan")
	if len(got) == 0 {
		t.Fatal("expected mention suggestions for @plan")
	}
}

func TestMentionSuggestionsTruncation(t *testing.T) {
	d := depsWithAgents([]string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"})
	got := MentionSuggestions(d, "use @")
	if len(got) > 8 {
		t.Errorf("expected max 8 suggestions, got %d", len(got))
	}
}

func TestTrailingMentionTokenEmpty(t *testing.T) {
	_, ok := trailingMentionToken("")
	if ok {
		t.Error("expected false for empty input")
	}
}

func TestTrailingMentionTokenNoAt(t *testing.T) {
	_, ok := trailingMentionToken("hello world")
	if ok {
		t.Error("expected false for no @ token")
	}
}

func TestTrailingMentionTokenWithAt(t *testing.T) {
	token, ok := trailingMentionToken("hello @world")
	if !ok || token != "@world" {
		t.Errorf("expected @world, got %q ok=%t", token, ok)
	}
}

func TestTrailingMentionTokenMultipleAts(t *testing.T) {
	token, ok := trailingMentionToken("@user check @file.go")
	if !ok || token != "@file.go" {
		t.Errorf("expected @file.go, got %q ok=%t", token, ok)
	}
}

func TestCommandCompletionsEmpty(t *testing.T) {
	got := commandCompletions("")
	if len(got) == 0 {
		t.Fatal("expected all commands for empty prefix")
	}
	for _, want := range []string{"/task", "/brain", "/exit", "/quit"} {
		if !contains(got, want) {
			t.Fatalf("expected %s in completions, got %v", want, got)
		}
	}
}

func TestCommandCompletionsH(t *testing.T) {
	got := commandCompletions("h")
	for _, c := range got {
		if !strings.HasPrefix(c, "/help") {
			t.Errorf("expected /help completion, got %q", c)
		}
	}
}

func TestCommandCompletionsNoMatch(t *testing.T) {
	got := commandCompletions("zzz")
	if len(got) != 0 {
		t.Errorf("expected no completions for 'zzz', got %v", got)
	}
}

func TestShellCompletionsEmpty(t *testing.T) {
	got := shellCompletions("")
	if len(got) == 0 {
		t.Fatal("expected shell completions for empty prefix")
	}
}

func TestShellCompletionsGit(t *testing.T) {
	got := shellCompletions("git")
	for _, c := range got {
		if !strings.HasPrefix(c, "git") {
			t.Errorf("expected 'git' prefix, got %q", c)
		}
	}
}

func TestShellCompletionsNoMatch(t *testing.T) {
	got := shellCompletions("xyzabc")
	if len(got) != 1 || got[0] != "xyzabc" {
		t.Errorf("expected [xyzabc] for unknown prefix, got %v", got)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
