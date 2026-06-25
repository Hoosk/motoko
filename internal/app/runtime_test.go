package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/semantic"
	"github.com/Hoosk/motoko/internal/semantic/symtypes"
	"github.com/Hoosk/motoko/internal/session"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tools"
)

type fakeRuntimeTool struct {
	name string
}

type fakeProviderClient struct {
	complete func(context.Context, string, []provider.ConversationItem, provider.ToolSet) (provider.Response, error)
	models   []provider.ModelInfo
}

func (f fakeRuntimeTool) Spec() tools.Spec {
	return tools.Spec{Name: f.name, Summary: "fake", Usage: f.name + " <arg>"}
}

func (f fakeRuntimeTool) Run(ctx context.Context, args string) (tools.Result, error) {
	return tools.Result{Spec: f.Spec(), Summary: "ok", Output: args}, nil
}

func (f fakeProviderClient) Configured() bool {
	return true
}

func (f fakeProviderClient) ProviderKind() string {
	return "fake"
}

func (f fakeProviderClient) Complete(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, toolSet provider.ToolSet) (provider.Response, error) {
	if f.complete == nil {
		return provider.Response{}, nil
	}
	return f.complete(ctx, systemPrompt, messages, toolSet)
}
func (f fakeProviderClient) StreamComplete(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, toolSet provider.ToolSet, onDelta func(provider.Delta) error) (provider.Response, error) {
	resp, err := f.Complete(ctx, systemPrompt, messages, toolSet)
	if err != nil {
		return provider.Response{}, err
	}
	if onDelta != nil && strings.TrimSpace(resp.FinalText) != "" {
		if err := onDelta(provider.Delta{Content: resp.FinalText}); err != nil {
			return provider.Response{}, err
		}
	}
	return resp, nil
}

func (f fakeProviderClient) Summary() string {
	return "fake:test"
}

func (f fakeProviderClient) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	return append([]provider.ModelInfo(nil), f.models...), nil
}

func (f fakeProviderClient) GetModel(ctx context.Context, model string) (provider.ModelInfo, error) {
	for _, m := range f.models {
		if m.ID == model {
			return m, nil
		}
	}
	return provider.ModelInfo{ID: model}, nil
}

func withSessionBaseDir(t *testing.T) {
	t.Helper()
	prev := session.SessionsBaseDir
	session.SessionsBaseDir = t.TempDir()
	t.Cleanup(func() {
		session.SessionsBaseDir = prev
	})
}

func TestCompletionsModelsKeepsTrailingSpaceContext(t *testing.T) {
	r := NewRuntime()
	r.config = &config.AppConfig{
		ActiveProvider: "openai",
		Providers: []config.ProviderConfig{{
			Name:   "openai",
			Preset: config.ProviderPresetOpenAI,
			Kind:   config.ProviderKindOpenAICompatible,
			Models: []string{"gpt-4.1", "gpt-4.1-mini", "o4-mini"},
		}},
	}

	got := r.Completions("/models ")
	want := []string{"/models gpt-4.1", "/models gpt-4.1-mini", "/models o4-mini"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Completions(/models space) = %#v, want %#v", got, want)
	}
}

func TestCompletionsModelsFiltersPrefix(t *testing.T) {
	r := NewRuntime()
	r.config = &config.AppConfig{
		ActiveProvider: "openai",
		Providers: []config.ProviderConfig{{
			Name:   "openai",
			Preset: config.ProviderPresetOpenAI,
			Kind:   config.ProviderKindOpenAICompatible,
			Models: []string{"gpt-4.1", "gpt-4.1-mini", "o4-mini"},
		}},
	}

	got := r.Completions("/models gpt-4.1-m")
	want := []string{"/models gpt-4.1-mini"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Completions(/models prefix) = %#v, want %#v", got, want)
	}
}

func TestEnrichContextAddsRelevantSnippets(t *testing.T) {
	r := NewRuntime(RuntimeOptions{})
	snapshot := semantic.Snapshot{
		Snapshot: symtypes.Snapshot{
			Files: []semantic.FileSummary{{
				Path:     "internal/app/runtime.go",
				Language: "go",
				Content:  []byte("package app\n\nfunc RunAgent() error {\n\treturn nil\n}\n"),
				Symbols:  []semantic.Symbol{{Name: "RunAgent", Kind: "func", Line: 3, Range: semantic.LineRange{Start: 3, End: 5}}},
			}},
			GeneratedAt: time.Now(),
		},
	}
	r.semantic = semantic.NewIndex()
	r.semantic.SetSnapshotForTest(&snapshot)

	info := r.enrichContext(context.Background(), system.ContextInfo{}, "revisa runagent")
	if len(info.RelevantSnippets) == 0 {
		t.Fatal("expected relevant snippets")
	}
	if !strings.Contains(info.RelevantSnippets[0], "RunAgent") {
		t.Fatalf("expected snippet mentioning RunAgent, got %q", info.RelevantSnippets[0])
	}
}

func TestSaveProviderNormalizesNameBeforeActivating(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	r := NewRuntime()
	err := r.SaveProvider(config.ProviderConfig{
		Preset: config.ProviderPresetOpenRouter,
		APIKey: "k",
		Model:  "openai/gpt-4.1",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if r.config.ActiveProvider != "openrouter" {
		t.Fatalf("expected normalized active provider, got %q", r.config.ActiveProvider)
	}
	active, ok := r.config.Active()
	if !ok || active.Name != "openrouter" {
		t.Fatalf("expected normalized active config, got %#v ok=%t", active, ok)
	}
}

func TestMentionSuggestionsPreferAgentsAndFiles(t *testing.T) {
	r := NewRuntime()
	r.availableAgents = append(r.availableAgents, agent.AgentDef{Name: "explore", System: "Busca codigo"})
	r.semantic = semantic.NewIndex()
	r.semantic.SetSnapshotForTest(&semantic.Snapshot{
		Snapshot: symtypes.Snapshot{
			GeneratedAt: time.Now(),
			Files:       []semantic.FileSummary{{Path: "internal/app/runtime.go", Language: "go", Content: []byte("package app\n")}},
		},
	})
	got := r.MentionSuggestions("revisa @ex")
	if len(got) == 0 || got[0] != "@explore" {
		t.Fatalf("expected @explore first, got %#v", got)
	}
	got = r.MentionSuggestions("revisa @runtime")
	if len(got) == 0 || !strings.Contains(strings.Join(got, " "), "@internal/app/runtime.go") {
		t.Fatalf("expected file mention suggestion, got %#v", got)
	}
}

func TestSanitizeSessionTitlePrefersCleanFinalTitle(t *testing.T) {
	raw := `(The user wants a title for the session. The session is just starting, so it's a general programming session.)

* *Option 1:* Sesion de programacion con Motoko
* *Option 2:* Asistencia experta en desarrollo de software
* *Option 3:* Tu asistente personal de programacion

* *Constraint Check:* "4 a 8 palabras".

Asistencia experta en desarrollo de software`
	got := sanitizeSessionTitle(raw)
	if got != "Asistencia experta en desarrollo de software" {
		t.Fatalf("sanitizeSessionTitle() = %q", got)
	}
}

func TestSanitizeSessionTitleKeepsSingleLineTitle(t *testing.T) {
	got := sanitizeSessionTitle("Depuracion de tools en Gemini")
	if got != "Depuracion de tools en Gemini" {
		t.Fatalf("sanitizeSessionTitle() = %q", got)
	}
}

func TestTitleFromModelResponsePrefersStructuredMessage(t *testing.T) {
	resp := provider.Response{FinalText: `{"message":"Depuracion de tools en Gemini"}`}
	got := titleFromModelResponse(resp)
	if got != "Depuracion de tools en Gemini" {
		t.Fatalf("titleFromModelResponse() = %q", got)
	}
}

func TestExtractStructuredMessageAcceptsFencedJSON(t *testing.T) {
	raw := "```json\n{\"message\":\"Asistencia experta en desarrollo de software\"}\n```"
	got := extractStructuredMessage(raw)
	if got != "Asistencia experta en desarrollo de software" {
		t.Fatalf("extractStructuredMessage() = %q", got)
	}
}

func TestCurrentSessionEntriesMapsRolesToEntryKinds(t *testing.T) {
	r := NewRuntime()
	r.currentSession = &session.Session{History: []provider.ConversationItem{
		{Role: "user", Content: "hola"},
		{Role: "assistant", Content: "mundo"},
		{Role: "system", Content: "nota"},
	}}

	got := r.CurrentSessionEntries()
	want := []Entry{
		{Kind: EntryUser, Text: "hola"},
		{Kind: EntryAssistant, Text: "mundo"},
		{Kind: EntrySystem, Text: "nota"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CurrentSessionEntries() = %#v, want %#v", got, want)
	}
}

func TestHandleInputBangDispatchesImmediateShellInBuildMode(t *testing.T) {
	r := NewRuntime()
	r.mode = ModeBuild

	resp := r.HandleInput("!pwd", system.ContextInfo{})
	if resp.Action == nil || resp.Action.Type != ActionShell || resp.Action.ShellCommand != "pwd" {
		t.Fatalf("HandleInput(!pwd) action = %#v", resp.Action)
	}
	if len(resp.Entries) == 0 || resp.Entries[0].Kind != EntryCommand {
		t.Fatalf("expected shell command entry, got %#v", resp.Entries)
	}
}

func TestHandleInputTracksAgentAndFileMentions(t *testing.T) {
	r := NewRuntime()
	r.availableAgents = append(r.availableAgents, agent.AgentDef{Name: "explore", System: "Busca codigo"})

	_ = r.HandleInput("revisa @explore @internal/app/runtime.go", system.ContextInfo{})

	if r.AgentName() != "explore" {
		t.Fatalf("expected agent mode switched to explore, got %q", r.AgentName())
	}
	if !reflect.DeepEqual(r.mentionedFiles, []string{"internal/app/runtime.go"}) {
		t.Fatalf("expected mentioned files tracked, got %#v", r.mentionedFiles)
	}
}

func TestHandleShellResultFormatsSuccessAndFailure(t *testing.T) {
	r := NewRuntime()
	success := r.HandleShellResult(ShellResult{Output: "ok", ExitCode: 0, Duration: time.Second})
	if len(success.Entries) != 2 || success.Entries[1].Kind != EntryOutput || success.Entries[1].Text != "ok" {
		t.Fatalf("unexpected success shell response %#v", success)
	}

	failure := r.HandleShellResult(ShellResult{Output: "boom", ExitCode: 7, Duration: time.Second})
	if len(failure.Entries) != 2 || failure.Entries[1].Kind != EntryError || failure.Entries[1].Text != "boom" {
		t.Fatalf("unexpected failure shell response %#v", failure)
	}
}

func TestHandleModelsCommandUpdatesActiveModel(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	r := NewRuntime()
	r.config = &config.AppConfig{
		ActiveProvider: "openai",
		Providers: []config.ProviderConfig{{
			Name:   "openai",
			Preset: config.ProviderPresetOpenAI,
			Kind:   config.ProviderKindOpenAICompatible,
			APIKey: "k",
			Model:  "old-model",
		}},
	}

	resp := r.handleModelsCommand([]string{"gpt-4.1"})
	active, ok := r.config.Active()
	if !ok {
		t.Fatal("expected active provider config")
	}
	if active.Model != "gpt-4.1" {
		t.Fatalf("expected active model updated, got %#v", active)
	}
	if len(resp.Entries) != 1 || !strings.Contains(resp.Entries[0].Text, "gpt-4.1") {
		t.Fatalf("unexpected models response %#v", resp)
	}
}

func TestProviderListTextMarksActiveProvider(t *testing.T) {
	r := NewRuntime()
	r.config = &config.AppConfig{
		ActiveProvider: "openai",
		Providers: []config.ProviderConfig{{
			Name:   "openai",
			Preset: config.ProviderPresetOpenAI,
			Model:  "gpt-4.1",
		}, {
			Name:   "anthropic",
			Preset: config.ProviderPresetAnthropic,
			Model:  "claude-sonnet",
		}},
	}

	text := r.providerListText()
	if !strings.Contains(text, "* openai [openai] gpt-4.1") {
		t.Fatalf("expected active provider marker, got %q", text)
	}
	if !strings.Contains(text, "  anthropic [anthropic] claude-sonnet") {
		t.Fatalf("expected secondary provider listed, got %q", text)
	}
}

func TestHandleInputStatusIncludesModeWorkspaceAndPendingApproval(t *testing.T) {
	r := NewRuntime()
	r.mode = ModePlan
	r.inputMode = InputModeShell
	r.pending = &pendingShell{Command: "git status"}

	resp := r.HandleInput("/status", system.ContextInfo{Workspace: "motoko"})
	if len(resp.Entries) != 1 {
		t.Fatalf("expected one status entry, got %#v", resp)
	}
	text := resp.Entries[0].Text
	for _, want := range []string{
		"mode: plan",
		"input: shell",
		"workspace: motoko",
		"pending approval: git status",
		"agents.md guidelines: not found",
		"design.md specification: not found",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in status text %q", want, text)
		}
	}
}

func TestHandleInputStatusIncludesLoadedAgentsAndDesign(t *testing.T) {
	tmpDir := t.TempDir()

	// Write mock AGENTS.md and DESIGN.md
	if err := os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte("agents"), 0644); err != nil {
		t.Fatalf("failed to write AGENTS.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "DESIGN.md"), []byte("design"), 0644); err != nil {
		t.Fatalf("failed to write DESIGN.md: %v", err)
	}

	r := NewRuntime()
	resp := r.HandleInput("/status", system.ContextInfo{Workspace: "motoko", Path: tmpDir})
	if len(resp.Entries) != 1 {
		t.Fatalf("expected one status entry, got %#v", resp)
	}
	text := resp.Entries[0].Text
	for _, want := range []string{
		"agents.md guidelines: loaded",
		"design.md specification: loaded",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in status text %q", want, text)
		}
	}
}

func TestHandleInputProviderListUsesFormattedProviderSummary(t *testing.T) {
	r := NewRuntime()
	r.config = &config.AppConfig{
		ActiveProvider: "openai",
		Providers: []config.ProviderConfig{{
			Name:   "openai",
			Preset: config.ProviderPresetOpenAI,
			Model:  "gpt-4.1",
		}, {
			Name:   "anthropic",
			Preset: config.ProviderPresetAnthropic,
			Model:  "claude-sonnet",
		}},
	}

	resp := r.HandleInput("/provider list", system.ContextInfo{})
	if len(resp.Entries) != 1 || resp.Entries[0].Kind != EntrySystem {
		t.Fatalf("unexpected provider list response %#v", resp)
	}
	text := resp.Entries[0].Text
	if !strings.Contains(text, "* openai [openai] gpt-4.1") {
		t.Fatalf("expected active provider in list, got %q", text)
	}
	if !strings.Contains(text, "  anthropic [anthropic] claude-sonnet") {
		t.Fatalf("expected secondary provider in list, got %q", text)
	}
}

func TestHandleInputToolRunsRegisteredTool(t *testing.T) {
	r := NewRuntime()
	r.tools.Register(fakeRuntimeTool{name: "fake"})

	resp := r.HandleInput("/tool fake hola mundo", system.ContextInfo{})
	if len(resp.Entries) != 3 {
		t.Fatalf("expected command, summary and output entries, got %#v", resp)
	}
	if resp.Entries[0] != (Entry{Kind: EntryCommand, Text: "tool fake hola mundo"}) {
		t.Fatalf("unexpected command entry %#v", resp.Entries[0])
	}
	if resp.Entries[1] != (Entry{Kind: EntrySystem, Text: "ok"}) {
		t.Fatalf("unexpected summary entry %#v", resp.Entries[1])
	}
	if resp.Entries[2] != (Entry{Kind: EntryOutput, Text: "hola mundo"}) {
		t.Fatalf("unexpected output entry %#v", resp.Entries[2])
	}
}

func TestHandleInputToolPreservesNewlines(t *testing.T) {
	r := NewRuntime()
	r.tools.Register(fakeRuntimeTool{name: "fake"})

	resp := r.HandleInput("/tool fake line1\nline2\n  line3", system.ContextInfo{})
	if len(resp.Entries) != 3 {
		t.Fatalf("expected command, summary and output entries, got %#v", resp)
	}
	if resp.Entries[2] != (Entry{Kind: EntryOutput, Text: "line1\nline2\n  line3"}) {
		t.Fatalf("expected multiline output with preserved formatting, got %q", resp.Entries[2].Text)
	}
}

func TestCompactSessionReturnsErrorWithoutActiveProviderWhenHistoryExists(t *testing.T) {
	r := NewRuntime()
	r.config = &config.AppConfig{}
	r.currentSession = &session.Session{
		History: []provider.ConversationItem{provider.UserText("hola"), provider.AssistantText("mundo")},
	}

	resp := r.CompactSession(context.Background())
	if len(resp.Entries) != 1 || resp.Entries[0].Kind != EntryError {
		t.Fatalf("expected compact error response, got %#v", resp)
	}
	if !strings.Contains(resp.Entries[0].Text, "no hay provider activo") {
		t.Fatalf("unexpected compact error %q", resp.Entries[0].Text)
	}
}

func TestMaybeAutoCompactSkipsWhenHistoryUsageBelowThreshold(t *testing.T) {
	r := NewRuntime()
	r.contextWindow = 1000
	r.currentSession = &session.Session{
		History:         []provider.ConversationItem{provider.UserText("hola")},
		LastInputTokens: 799,
	}
	events := 0

	err := r.maybeAutoCompact(context.Background(), func(AgentStreamEvent) error {
		events++
		return nil
	})
	if err != nil {
		t.Fatalf("maybeAutoCompact() error = %v", err)
	}
	if events != 0 {
		t.Fatalf("expected no compact events below threshold, got %d", events)
	}
	title := strings.TrimSpace(r.currentSession.Title)
	if title != "" && !strings.EqualFold(title, "New session") {
		return
	}
	if len(r.currentSession.History) != 1 || r.currentSession.LastInputTokens != 799 {
		t.Fatalf("expected session unchanged, got %#v", r.currentSession)
	}
}

func TestCompactSessionCompactsHistoryWithProviderSummary(t *testing.T) {
	withSessionBaseDir(t)
	r := NewRuntime()
	r.config = &config.AppConfig{
		ActiveProvider: "openai",
		Providers: []config.ProviderConfig{{
			Name:   "openai",
			Preset: config.ProviderPresetOpenAI,
			Kind:   config.ProviderKindOpenAICompatible,
			APIKey: "k",
			Model:  "gpt-4.1",
		}},
	}
	r.currentSession = session.New("ws", "/workspace")
	r.currentSession.History = []provider.ConversationItem{provider.UserText("hola"), provider.AssistantText("mundo")}
	r.currentSession.LastInputTokens = 900
	r.newProviderClient = func(cfg config.ProviderConfig) (provider.Client, error) {
		return fakeProviderClient{complete: func(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, toolSet provider.ToolSet) (provider.Response, error) {
			if !strings.Contains(systemPrompt, "You are the memory compaction") {
				return provider.Response{}, fmt.Errorf("unexpected system prompt %q", systemPrompt)
			}
			if len(messages) < 1 {
				return provider.Response{}, fmt.Errorf("unexpected message count %d", len(messages))
			}
			return provider.Response{FinalText: "resumen breve"}, nil
		}}, nil
	}

	longMsg := strings.Repeat("A", 200000)
	r.currentSession.History = []provider.ConversationItem{
		provider.UserText(longMsg),
		provider.AssistantText("mundo"),
	}

	resp := r.CompactSession(context.Background())
	if len(resp.Entries) != 1 || resp.Entries[0] != (Entry{Kind: EntrySystem, Text: "Session compacted."}) {
		t.Fatalf("unexpected compact response %#v", resp)
	}
	if len(r.currentSession.History) != 2 {
		t.Fatalf("expected compacted two-message history, got %#v", r.currentSession.History)
	}
	if got := r.currentSession.History[0].Role; got != provider.RoleUser {
		t.Fatalf("expected compacted summary role %q, got %q", provider.RoleUser, got)
	}
	if got := r.currentSession.History[0].PlainText(); !strings.Contains(got, "resumen breve") {
		t.Fatalf("expected compacted summary in history, got %q", got)
	}
	if r.currentSession.LastInputTokens != 0 {
		t.Fatalf("expected input tokens reset, got %d", r.currentSession.LastInputTokens)
	}
}

func TestMaybeAutoCompactCompactsAndEmitsEventsAtThreshold(t *testing.T) {
	withSessionBaseDir(t)
	r := NewRuntime()
	r.contextWindow = 1000
	r.config = &config.AppConfig{
		ActiveProvider: "openai",
		Providers: []config.ProviderConfig{{
			Name:   "openai",
			Preset: config.ProviderPresetOpenAI,
			Kind:   config.ProviderKindOpenAICompatible,
			APIKey: "k",
			Model:  "gpt-4.1",
		}},
	}
	r.currentSession = session.New("ws", "/workspace")
	longMsg := strings.Repeat("A", 200000)
	r.currentSession.History = []provider.ConversationItem{provider.UserText(longMsg), provider.AssistantText("mundo")}
	r.currentSession.LastInputTokens = 800
	r.newProviderClient = func(cfg config.ProviderConfig) (provider.Client, error) {
		return fakeProviderClient{complete: func(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, toolSet provider.ToolSet) (provider.Response, error) {
			return provider.Response{FinalText: "resumen automatico"}, nil
		}}, nil
	}

	var events []AgentStreamEvent
	err := r.maybeAutoCompact(context.Background(), func(event AgentStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("maybeAutoCompact() error = %v", err)
	}
	if !reflect.DeepEqual(events, []AgentStreamEvent{{Kind: "compacting", Content: "Compacting session..."}, {Kind: "status", Content: "Session auto-compacted."}}) {
		t.Fatalf("unexpected compact events %#v", events)
	}
	if got := r.currentSession.History[0].Role; got != provider.RoleUser {
		t.Fatalf("expected auto-compacted summary role %q, got %q", provider.RoleUser, got)
	}
	if got := r.currentSession.History[0].PlainText(); !strings.Contains(got, "resumen automatico") {
		t.Fatalf("expected auto-compact summary in history, got %q", got)
	}
}

func TestCompactSessionPruningPreservesToolMetadata(t *testing.T) {
	withSessionBaseDir(t)
	r := NewRuntime()
	r.config = &config.AppConfig{
		ActiveProvider: "openai",
		Providers: []config.ProviderConfig{{
			Name:   "openai",
			Preset: config.ProviderPresetOpenAI,
			Kind:   config.ProviderKindOpenAICompatible,
			APIKey: "k",
			Model:  "gpt-4.1",
		}},
	}
	r.currentSession = session.New("ws", "/workspace")

	call := provider.ToolInvocation{Name: "my_custom_tool", CallID: "call_123"}
	longOutput := strings.Repeat("A", 3000)
	toolMsg := provider.ToolResultForInvocation(call, longOutput)

	r.currentSession.History = []provider.ConversationItem{
		provider.UserText("hello"),
		toolMsg,
		provider.AssistantText("done"),
		provider.UserText(strings.Repeat("B", 200000)),
	}
	r.currentSession.LastInputTokens = 1000

	var capturedMessages []provider.ConversationItem
	r.newProviderClient = func(cfg config.ProviderConfig) (provider.Client, error) {
		return fakeProviderClient{complete: func(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, toolSet provider.ToolSet) (provider.Response, error) {
			capturedMessages = messages
			return provider.Response{FinalText: "resumen breve"}, nil
		}}, nil
	}

	resp := r.CompactSession(context.Background())
	if len(resp.Entries) != 1 || resp.Entries[0].Text != "Session compacted." {
		t.Fatalf("unexpected compact response %#v", resp)
	}

	foundTool := false
	for _, msg := range capturedMessages {
		if msg.Role == provider.RoleTool {
			foundTool = true
			parsedCall, parsedOutput := provider.ParseToolResultContent(msg.Content)
			if parsedCall.Name != "my_custom_tool" {
				t.Errorf("expected tool name 'my_custom_tool', got %q", parsedCall.Name)
			}
			if parsedCall.CallID != "call_123" {
				t.Errorf("expected call ID 'call_123', got %q", parsedCall.CallID)
			}
			if !strings.Contains(parsedOutput, "Tool output was large and has been pruned") {
				t.Errorf("expected pruned message in output, got %q", parsedOutput)
			}
		}
	}
	if !foundTool {
		t.Fatal("expected tool message to be passed to the provider client for summarization")
	}
}

func TestGenerateTitleUpdatesCurrentSessionFromStructuredResponse(t *testing.T) {
	withSessionBaseDir(t)
	r := NewRuntime()
	r.config = &config.AppConfig{
		ActiveProvider: "openai",
		Providers: []config.ProviderConfig{{
			Name:   "openai",
			Preset: config.ProviderPresetOpenAI,
			Kind:   config.ProviderKindOpenAICompatible,
			APIKey: "k",
			Model:  "gpt-4.1",
		}},
	}
	r.currentSession = session.New("ws", "/workspace")
	r.newProviderClient = func(cfg config.ProviderConfig) (provider.Client, error) {
		return fakeProviderClient{complete: func(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, toolSet provider.ToolSet) (provider.Response, error) {
			if !strings.Contains(systemPrompt, "Generate a short title") {
				return provider.Response{}, fmt.Errorf("unexpected title prompt %q", systemPrompt)
			}
			if len(messages) != 2 || messages[0].Role != provider.RoleUser || messages[1].Role != provider.RoleAssistant {
				return provider.Response{}, fmt.Errorf("unexpected title messages %#v", messages)
			}
			return provider.Response{FinalText: "```json\n{\"message\":\"Sesion de pruebas runtime\"}\n```"}, nil
		}}, nil
	}

	r.generateTitle(context.Background(), "haz pruebas", "hecho")
	if r.currentSession.Title != "Sesion de pruebas runtime" {
		t.Fatalf("expected generated title, got %q", r.currentSession.Title)
	}
}

func TestRuntimeSkillsIntegration(t *testing.T) {
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	skillDir := filepath.Join(tmpDir, ".agents", "skills", "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	skillContent := `---\nname: test-skill\ndescription: "A simple testing skill"\n---\n# Test Skill Body\n`
	skillContent = strings.ReplaceAll(skillContent, `\n`, "\n")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewRuntime()
	if len(r.availableSkills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(r.availableSkills))
	}
	if r.availableSkills[0].Name != "test-skill" {
		t.Errorf("expected skill name 'test-skill', got %q", r.availableSkills[0].Name)
	}

	spec, found := r.tools.Spec(tools.ToolContext{AvailableSkills: []string{"test-skill"}}, "activate_skill")
	if !found {
		t.Fatal("expected activate_skill tool to be registered")
	}
	if !strings.Contains(spec.Usage, "test-skill") {
		t.Errorf("expected usage to contain skill name, got %q", spec.Usage)
	}

	info := r.enrichContext(context.Background(), system.ContextInfo{}, "test query")
	if len(info.AvailableSkills) != 1 {
		t.Fatalf("expected 1 available skill in context, got %d", len(info.AvailableSkills))
	}
	if info.AvailableSkills[0].Name != "test-skill" {
		t.Errorf("expected skill 'test-skill' in context, got %q", info.AvailableSkills[0].Name)
	}
}

func TestRuntimeBrainCommands(t *testing.T) {
	withSessionBaseDir(t)

	r := NewRuntime()
	if r.brain == nil {
		t.Fatal("expected session brain to be initialized")
	}

	// Test writing via tool
	_, err := r.tools.Run(context.Background(), "brain_write", "plan.md This is my plan")
	if err != nil {
		t.Fatalf("failed to write plan via tool: %v", err)
	}

	// Test /brain plan command
	resp := r.handleSlashCommand("/brain plan", system.ContextInfo{})
	if len(resp.Entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(resp.Entries))
	}
	if !strings.Contains(resp.Entries[1].Text, "This is my plan") {
		t.Errorf("expected plan output, got: %q", resp.Entries[1].Text)
	}

	// Test /brain tasks shortcut (empty case)
	resp = r.handleSlashCommand("/brain tasks", system.ContextInfo{})
	if len(resp.Entries) == 0 || resp.Entries[0].Kind != EntryError {
		t.Errorf("expected error entry for missing tasks.md, got: %#v", resp)
	}

	// Test writing tasks
	_, err = r.tools.Run(context.Background(), "brain_write", "tasks.md - [ ] Task 1")
	if err != nil {
		t.Fatalf("failed to write tasks: %v", err)
	}

	// Test /brain tasks command
	resp = r.handleSlashCommand("/brain tasks", system.ContextInfo{})
	if len(resp.Entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(resp.Entries))
	}
	if !strings.Contains(resp.Entries[1].Text, "- [ ] Task 1") {
		t.Errorf("expected tasks output, got: %q", resp.Entries[1].Text)
	}

	// Test /brain list
	resp = r.handleSlashCommand("/brain list", system.ContextInfo{})
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "plan.md") {
		t.Errorf("expected file listing containing plan.md, got: %#v", resp)
	}

	// Test /brain read
	resp = r.handleSlashCommand("/brain read plan.md", system.ContextInfo{})
	if len(resp.Entries) < 2 || !strings.Contains(resp.Entries[1].Text, "This is my plan") {
		t.Errorf("expected plan file content, got: %#v", resp)
	}

	// Test enrichContext system prompt integration
	info := r.enrichContext(context.Background(), system.ContextInfo{}, "query")
	if !strings.Contains(info.BrainSummary, "plan.md") || !strings.Contains(info.BrainSummary, "This is my plan") {
		t.Errorf("expected brain summary to contain plan and its contents, got: %q", info.BrainSummary)
	}

	// Test /brain clear
	resp = r.handleSlashCommand("/brain clear", system.ContextInfo{})
	if len(resp.Entries) == 0 || !strings.Contains(resp.Entries[0].Text, "deleted") {
		t.Errorf("expected deletion confirmation, got: %#v", resp)
	}

	if r.brain.Exists("plan") {
		t.Error("plan should not exist after brain clear")
	}
}

func TestHandleInputExitAndQuitCommands(t *testing.T) {
	r := NewRuntime()

	for _, cmd := range []string{"/exit", "/quit"} {
		resp := r.HandleInput(cmd, system.ContextInfo{})
		if resp.Signal != "quit" {
			t.Errorf("expected Signal to be 'quit' for command %q, got %q", cmd, resp.Signal)
		}
	}
}

