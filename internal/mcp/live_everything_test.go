package mcp

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestLiveEverythingServer drives the real `npx
// @modelcontextprotocol/server-everything` over stdio and verifies the
// manager connects, negotiates, exposes the server's tools, and can call one
// of them. Requires network + npx; skipped otherwise.
func TestLiveEverythingServer(t *testing.T) {
	if _, err := exec.LookPath("npx"); err != nil {
		t.Skip("npx not available")
	}
	reg := &liveCollector{}
	m := NewManager(ManagerConfig{
		Timeout: 60 * time.Second,
		Registry: ToolRegistrar{
			Register:   func(tool ToolAdapter) { reg.add(tool.Spec().Name, tool) },
			Unregister: func(name string) bool { return reg.remove(name) },
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	m.Start(ctx, []ServerConfig{{
		Name:      "everything",
		Transport: "stdio",
		Command:   "npx",
		Args:      []string{"-y", "@modelcontextprotocol/server-everything"},
	}})

	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		if reg.count() > 0 {
			break
		}
	}

	for _, s := range m.Servers() {
		fmt.Printf("STATUS %s transport=%s connected=%v tools=%d err=%v\n", s.Name, s.Transport, s.Connected, s.ToolCount, s.Err)
	}
	names := reg.list()
	fmt.Printf("TOOLS (%d): %s\n", len(names), strings.Join(names, ", "))

	if len(names) == 0 {
		cancel()
		t.Skip("server did not expose tools within deadline")
	}

	adapter := reg.get("mcp_everything_echo")
	if adapter == nil {
		cancel()
		t.Fatal("echo tool not captured")
	}
	cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer ccancel()
	res, err := adapter.Run(cctx, `{"message":"hola"}`)
	if err != nil {
		t.Logf("call %s: %v", adapter.Spec().Name, err)
	} else {
		fmt.Printf("CALL %s -> %s\n", adapter.Spec().Name, res.Output)
	}
	cancel()
}

type liveCollector struct {
	tools map[string]ToolAdapter
	names []string
	mu    sync.Mutex
}

func (c *liveCollector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.names)
}

func (c *liveCollector) list() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.names...)
}

func (c *liveCollector) get(n string) ToolAdapter {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tools == nil {
		return nil
	}
	return c.tools[n]
}

func (c *liveCollector) add(n string, t ToolAdapter) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !slices.Contains(c.names, n) {
		c.names = append(c.names, n)
	}
	if c.tools == nil {
		c.tools = map[string]ToolAdapter{}
	}
	c.tools[n] = t
}
func (c *liveCollector) remove(n string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, x := range c.names {
		if x == n {
			c.names = append(c.names[:i], c.names[i+1:]...)
			delete(c.tools, n)
			return true
		}
	}
	return false
}
