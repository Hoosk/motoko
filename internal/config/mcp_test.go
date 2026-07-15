package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestMCPServerNormalizeTransport(t *testing.T) {
	cases := map[string]string{
		"":         "stdio",
		"stdio":    "stdio",
		"Stdio":    "stdio",
		" http ":   "http",
		"streamable": "streamable",
	}
	for in, want := range cases {
		got := MCPServerConfig{Transport: in}.NormalizeTransport()
		if got != want {
			t.Errorf("NormalizeTransport(%q)=%q want %q", in, got, want)
		}
	}
}

func TestMCPServerEnvSlice(t *testing.T) {
	got := MCPServerConfig{Env: map[string]string{"FOO": "bar", "BAZ": "qux"}}.EnvSlice()
	sort.Strings(got)
	want := []string{"BAZ=qux", "FOO=bar"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
	empty := MCPServerConfig{}.EnvSlice()
	if empty != nil {
		t.Errorf("empty env should return nil, got %v", empty)
	}
}

func TestMergeMCPServersReplacesByName(t *testing.T) {
	base := []MCPServerConfig{
		{Name: "alpha", Command: "/bin/true"},
		{Name: "beta", Command: "/bin/false"},
	}
	override := []MCPServerConfig{
		{Name: "alpha", Command: "/usr/bin/true"},
		{Name: "gamma", Command: "/bin/ls"},
	}
	got := mergeMCPServers(base, override)
	want := []MCPServerConfig{
		{Name: "alpha", Command: "/usr/bin/true"},
		{Name: "beta", Command: "/bin/false"},
		{Name: "gamma", Command: "/bin/ls"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v want %+v", got, want)
	}
}

func TestLoadMCPFileMissingReturnsNil(t *testing.T) {
	got, err := LoadMCPFile(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if got != nil {
		t.Errorf("missing file should return nil, got %v", got)
	}
}

func TestLoadMCPFileParsesServers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	payload := `{"mcp_servers":[
		{"name":"alpha","command":"/bin/true"},
		{"name":"beta","transport":"http","url":"https://example.com/mcp","headers":{"X-Token":"abc"}}
	]}`
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadMCPFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(got))
	}
	if got[0].Name != "alpha" || got[0].Command != "/bin/true" {
		t.Errorf("alpha mismatch: %+v", got[0])
	}
	if got[1].URL != "https://example.com/mcp" || got[1].Headers["X-Token"] != "abc" {
		t.Errorf("beta mismatch: %+v", got[1])
	}
}

func TestLoadMCPFileRejectsBadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(path, []byte(`{`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMCPFile(path); err == nil {
		t.Fatalf("expected error for invalid JSON")
	}
}

func TestAppConfigMarshalIncludesMCPServers(t *testing.T) {
	cfg := AppConfig{
		MCPServers: []MCPServerConfig{{Name: "alpha", Command: "/bin/true"}},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !containsBytes(data, []byte(`"mcp_servers"`)) {
		t.Errorf("mcp_servers missing in JSON: %s", data)
	}
}

func containsBytes(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if string(haystack[i:i+len(needle)]) == string(needle) {
			return true
		}
	}
	return false
}
