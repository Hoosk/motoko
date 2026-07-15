package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/mcp"
)

type stubAdapter struct {
	spec    mcp.ToolSpec
	summary string
	output  string
	err     error
}

func (s *stubAdapter) Spec() mcp.ToolSpec { return s.spec }
func (s *stubAdapter) Run(_ context.Context, _ string) (mcp.ToolResult, error) {
	if s.err != nil {
		return mcp.ToolResult{}, s.err
	}
	return mcp.ToolResult{Spec: s.spec, Summary: s.summary, Output: s.output}, nil
}
func (s *stubAdapter) ServerName() string    { return "stub" }
func (s *stubAdapter) OriginalName() string  { return strings.TrimPrefix(s.spec.Name, "mcp_stub_") }
func (s *stubAdapter) RegisteredName() string { return s.spec.Name }
func (s *stubAdapter) IsReadOnly() bool     { return false }

func TestMCPRemoteToolRun(t *testing.T) {
	adapter := &stubAdapter{
		spec:    mcp.ToolSpec{Name: "mcp_stub_x", Summary: "do x", Usage: "mcp usage"},
		summary: "ran x",
		output:  "x-output",
	}
	tool := NewMCPRemoteTool(adapter)
	out, err := tool.Run(context.Background(), `{"a":1}`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out.Output != "x-output" || out.Summary != "ran x" {
		t.Fatalf("unexpected result: %+v", out)
	}
	if tool.Name() != "mcp_stub_x" {
		t.Errorf("unexpected registered name: %s", tool.Name())
	}
	// For a non-RemoteTool adapter, ServerName/OriginalName default to
	// empty (the type assertion inside MCPRemoteTool does not match).
	if tool.ServerName() != "" || tool.OriginalName() != "" {
		t.Errorf("unexpected metadata for stub adapter: %s/%s",
			tool.ServerName(), tool.OriginalName())
	}
}

func TestMCPRemoteToolRunError(t *testing.T) {
	adapter := &stubAdapter{
		spec: mcp.ToolSpec{Name: "mcp_stub_x"},
		err:  errors.New("boom"),
	}
	tool := NewMCPRemoteTool(adapter)
	if _, err := tool.Run(context.Background(), ""); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRegistryUnregisterRemovesTool(t *testing.T) {
	r := &Registry{tools: map[string]Tool{}, order: []string{}}
	r.Register(fakeTool{name: "alpha"})
	r.Register(fakeTool{name: "beta"})

	if !r.Unregister("alpha") {
		t.Fatalf("expected alpha to be unregistered")
	}
	if r.Unregister("alpha") {
		t.Fatalf("second unregister should be false")
	}
	if _, ok := r.tools["alpha"]; ok {
		t.Fatalf("alpha still in tools")
	}
	if len(r.order) != 1 || r.order[0] != "beta" {
		t.Fatalf("order unexpected: %v", r.order)
	}
}
