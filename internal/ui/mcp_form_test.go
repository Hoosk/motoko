package ui

import (
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/app"
	tea "github.com/charmbracelet/bubbletea"
)

func keyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func newOpenForm(t *testing.T) (*mcpForm, *app.Runtime) {
	t.Helper()
	form := &mcpForm{}
	form.Open()
	if !form.active {
		t.Fatal("expected form to be active after Open()")
	}
	if form.transport != mcpTransportStdio {
		t.Fatalf("expected default transport stdio, got %q", form.transport)
	}
	if form.fieldIndex != 0 {
		t.Fatalf("expected initial fieldIndex 0, got %d", form.fieldIndex)
	}
	return form, app.NewRuntime()
}

func TestMCPFormOpenInitialState(t *testing.T) {
	form, _ := newOpenForm(t)
	if form.name != "" || form.commandOrURL != "" || form.args != "" {
		t.Errorf("expected empty fields after Open, got name=%q cmd=%q args=%q", form.name, form.commandOrURL, form.args)
	}
	if form.status != "" {
		t.Errorf("expected empty status, got %q", form.status)
	}
}

func TestMCPFormNavigation(t *testing.T) {
	form, _ := newOpenForm(t)
	// stdio transport has 6 fields: 0..5
	form.Update(tea.KeyMsg{Type: tea.KeyTab}, nil)
	if form.fieldIndex != 1 {
		t.Errorf("expected fieldIndex 1 after tab, got %d", form.fieldIndex)
	}
	form.Update(tea.KeyMsg{Type: tea.KeyDown}, nil)
	if form.fieldIndex != 2 {
		t.Errorf("expected fieldIndex 2 after down, got %d", form.fieldIndex)
	}
	form.Update(tea.KeyMsg{Type: tea.KeyUp}, nil)
	if form.fieldIndex != 1 {
		t.Errorf("expected fieldIndex 1 after up, got %d", form.fieldIndex)
	}
	form.Update(tea.KeyMsg{Type: tea.KeyUp}, nil)
	if form.fieldIndex != 0 {
		t.Errorf("expected fieldIndex 0 after up, got %d", form.fieldIndex)
	}
	// Wrap forward: from 5 tab goes to 0
	form.fieldIndex = 5
	form.Update(tea.KeyMsg{Type: tea.KeyTab}, nil)
	if form.fieldIndex != 0 {
		t.Errorf("expected wrap to 0 from 5, got %d", form.fieldIndex)
	}
}

func TestMCPFormToggleTransport(t *testing.T) {
	form, _ := newOpenForm(t)
	// left/right and space toggle transport when on field 0
	form.Update(tea.KeyMsg{Type: tea.KeyLeft}, nil)
	if form.transport != mcpTransportHTTP {
		t.Errorf("expected http after left, got %q", form.transport)
	}
	if form.fieldCount() != 5 {
		t.Errorf("expected 5 fields for http, got %d", form.fieldCount())
	}
	// pressing space on the toggle (field 0) toggles back
	form.Update(keyRunes(" "), nil)
	if form.transport != mcpTransportStdio {
		t.Errorf("expected stdio after space toggle, got %q", form.transport)
	}
	if form.fieldCount() != 6 {
		t.Errorf("expected 6 fields for stdio, got %d", form.fieldCount())
	}
	// Enter on field 0 also toggles
	form.Update(tea.KeyMsg{Type: tea.KeyEnter}, nil)
	if form.transport != mcpTransportHTTP {
		t.Errorf("expected http after enter on toggle, got %q", form.transport)
	}
}

// TestMCPFormSpaceInArgsField is a regression test for the bug where pressing
// space in a text field was swallowed by the toggle handler, making it
// impossible to type commands like "npx -y @modelcontextprotocol/server".
func TestMCPFormSpaceInArgsField(t *testing.T) {
	form, _ := newOpenForm(t)
	// Move to Args field (index 3 in stdio)
	form.fieldIndex = 3
	form.Update(keyRunes("npx -y foo"), nil)
	if form.args != "npx -y foo" {
		t.Errorf("expected args to contain spaces, got %q", form.args)
	}
}

func TestMCPFormSpaceInNameAndCommand(t *testing.T) {
	form, _ := newOpenForm(t)
	form.fieldIndex = 1
	form.Update(keyRunes("my server"), nil)
	if form.name != "my server" {
		t.Errorf("expected name with space, got %q", form.name)
	}
	form.fieldIndex = 2
	form.Update(keyRunes("/usr/local/bin/cmd foo"), nil)
	if form.commandOrURL != "/usr/local/bin/cmd foo" {
		t.Errorf("expected command with space, got %q", form.commandOrURL)
	}
}

func TestMCPFormSpaceIgnoredOnButtons(t *testing.T) {
	form, _ := newOpenForm(t)
	form.transport = mcpTransportHTTP
	form.fieldIndex = 3 // save button
	form.Update(keyRunes(" "), nil)
	if form.transport != mcpTransportHTTP {
		t.Errorf("transport should not change on button, got %q", form.transport)
	}
}

func TestMCPFormTextInputAndBackspace(t *testing.T) {
	form, _ := newOpenForm(t)
	form.fieldIndex = 1
	form.Update(keyRunes("alpha"), nil)
	if form.name != "alpha" {
		t.Fatalf("expected name=alpha, got %q", form.name)
	}
	form.Update(tea.KeyMsg{Type: tea.KeyBackspace}, nil)
	if form.name != "alph" {
		t.Errorf("expected name=alph after backspace, got %q", form.name)
	}
	form.Update(keyRunes("a"), nil)
	if form.name != "alpha" {
		t.Errorf("expected name=alpha, got %q", form.name)
	}
}

func TestMCPFormEscCloses(t *testing.T) {
	form, _ := newOpenForm(t)
	form.Update(tea.KeyMsg{Type: tea.KeyEsc}, nil)
	if form.active {
		t.Error("expected form to be inactive after esc")
	}
}

func TestMCPFormCancelButtonCloses(t *testing.T) {
	form, _ := newOpenForm(t)
	// stdio: cancel is field 5
	form.fieldIndex = 5
	form.Update(tea.KeyMsg{Type: tea.KeyEnter}, nil)
	if form.active {
		t.Error("expected form to be inactive after enter on cancel")
	}
}

func TestMCPFormSaveRequiresName(t *testing.T) {
	form, _ := newOpenForm(t)
	form.fieldIndex = 4 // save
	form.Update(tea.KeyMsg{Type: tea.KeyEnter}, nil)
	if !form.active {
		t.Error("form should remain active when validation fails")
	}
	if !strings.Contains(form.status, "Name is required") {
		t.Errorf("expected name validation error, got %q", form.status)
	}
}

func TestMCPFormSaveRequiresCommand(t *testing.T) {
	form, _ := newOpenForm(t)
	form.fieldIndex = 1
	form.Update(keyRunes("git"), nil)
	form.fieldIndex = 4 // save
	form.Update(tea.KeyMsg{Type: tea.KeyEnter}, nil)
	if !form.active {
		t.Error("form should remain active when validation fails")
	}
	if !strings.Contains(form.status, "Command is required") {
		t.Errorf("expected command validation error, got %q", form.status)
	}
}

func TestMCPFormSaveRequiresURLForHTTP(t *testing.T) {
	form, _ := newOpenForm(t)
	form.Update(tea.KeyMsg{Type: tea.KeyLeft}, nil) // to http
	form.fieldIndex = 1
	form.Update(keyRunes("remote"), nil)
	// http: save is field 3
	form.fieldIndex = 3
	form.Update(tea.KeyMsg{Type: tea.KeyEnter}, nil)
	if !form.active {
		t.Error("form should remain active when validation fails")
	}
	if !strings.Contains(form.status, "URL is required") {
		t.Errorf("expected url validation error, got %q", form.status)
	}
}

func TestMCPFormHTTPEnterOnLastTextFieldSaves(t *testing.T) {
	// In http mode, the URL field is the last text input (index 2),
	// and pressing enter on it (via the saveIdx==fieldCount-3 path) saves.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	form, runtime := newOpenForm(t)
	form.Update(tea.KeyMsg{Type: tea.KeyLeft}, runtime) // to http
	form.fieldIndex = 1
	form.Update(keyRunes("remote"), runtime)
	form.fieldIndex = 2
	form.Update(keyRunes("https://example.com"), runtime)
	form.Update(tea.KeyMsg{Type: tea.KeyEnter}, runtime)
	// With a writable config, the save should succeed and close the form.
	if form.active {
		t.Errorf("expected form to close after successful save, status=%q", form.status)
	}
	// The server should be present in the runtime's config.
	cfg := runtime.Config()
	if cfg == nil {
		t.Fatal("expected runtime config to be non-nil")
	}
	found := false
	for _, s := range cfg.MCPServers {
		if s.Name == "remote" {
			found = true
			if s.Transport != mcpTransportHTTP || s.URL != "https://example.com" {
				t.Errorf("unexpected saved server: %+v", s)
			}
		}
	}
	if !found {
		t.Errorf("expected 'remote' server in config, got %+v", cfg.MCPServers)
	}
}

func TestMCPFormSuccessfulStdioSave(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	form, runtime := newOpenForm(t)
	form.fieldIndex = 1
	form.Update(keyRunes("git"), runtime)
	form.fieldIndex = 2
	form.Update(keyRunes("npx"), runtime)
	form.fieldIndex = 3
	form.Update(keyRunes("-y @modelcontextprotocol/server-git"), runtime)
	form.fieldIndex = 4 // save
	form.Update(tea.KeyMsg{Type: tea.KeyEnter}, runtime)
	if form.active {
		t.Fatalf("expected form to close, status=%q", form.status)
	}
	for _, s := range runtime.Config().MCPServers {
		if s.Name == "git" {
			if s.Command != "npx" {
				t.Errorf("expected command=npx, got %q", s.Command)
			}
			if len(s.Args) != 2 || s.Args[0] != "-y" || s.Args[1] != "@modelcontextprotocol/server-git" {
				t.Errorf("unexpected args: %v", s.Args)
			}
			return
		}
	}
	t.Fatal("expected git server in config")
}

func TestMCPFormViewEmptyWhenClosed(t *testing.T) {
	form := &mcpForm{}
	if got := form.View(nil); got != "" {
		t.Errorf("expected empty view when closed, got %q", got)
	}
}

func TestMCPFormViewRendersWhenOpen(t *testing.T) {
	form, runtime := newOpenForm(t)
	got := form.View(runtime)
	if !strings.Contains(got, "Add MCP Server") {
		t.Errorf("expected title in view, got %q", got)
	}
	if !strings.Contains(got, "stdio") {
		t.Errorf("expected transport visible, got %q", got)
	}
	// Switch to http: Args disappears, URL appears
	form.Update(tea.KeyMsg{Type: tea.KeyLeft}, nil)
	got = form.View(runtime)
	if !strings.Contains(got, "http") {
		t.Errorf("expected http in view, got %q", got)
	}
	if !strings.Contains(got, "URL") {
		t.Errorf("expected URL label in view, got %q", got)
	}
}
