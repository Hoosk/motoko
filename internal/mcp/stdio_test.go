package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeMCPServerScript is a minimal JSON-RPC-over-stdio MCP server used to
// exercise the stdio transport against a real subprocess. It replies to
// `initialize`, `tools/list` and `tools/call`.
const fakeMCPServerScript = `#!/usr/bin/env bash
set -e
while IFS= read -r line; do
  [ -z "$line" ] && continue
  id=$(printf '%s' "$line" | sed -n 's/.*"id":\([0-9]*\).*/\1/p')
  method=$(printf '%s' "$line" | sed -n 's/.*"method":"\([^"]*\)".*/\1/p')
  case "$method" in
    initialize)
      printf '{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2025-11-25","capabilities":{"tools":{"listChanged":true}},"serverInfo":{"name":"echo","version":"0"}}}\n' "$id"
      ;;
    tools/list)
      printf '{"jsonrpc":"2.0","id":%s,"result":{"tools":[{"name":"echo","inputSchema":{}}]}}\n' "$id"
      ;;
    tools/call)
      printf '{"jsonrpc":"2.0","id":%s,"result":{"content":[{"type":"text","text":"echo-reply"}]}}\n' "$id"
      ;;
    ping)
      printf '{"jsonrpc":"2.0","id":%s,"result":{}}\n' "$id"
      ;;
    *)
      printf '{"jsonrpc":"2.0","id":%s,"error":{"code":-32601,"message":"unknown"}}\n' "$id"
      ;;
  esac
done
`

func writeFakeServer(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fake_mcp.sh")
	if err := os.WriteFile(path, []byte(fakeMCPServerScript), 0o755); err != nil {
		t.Fatalf("write fake server: %v", err)
	}
	return path
}

func TestStdioTransportEndToEnd(t *testing.T) {
	if _, err := lookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	server := writeFakeServer(t)
	transport, err := NewStdioTransport(StdioConfig{
		Command: "bash",
		Args:    []string{server},
		Stderr:  io.Discard,
	})
	if err != nil {
		t.Fatalf("stdio transport: %v", err)
	}
	if transport.PID() == 0 {
		t.Fatalf("expected non-zero PID")
	}

	client := NewClient(ClientConfig{
		Transport:      transport,
		ClientInfo:     Implementation{Name: "test", Version: "0"},
		RequestTimeout: 5 * time.Second,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client.Start(ctx)
	defer func() { _ = client.Close() }()

	if _, initErr := client.Initialize(ctx); initErr != nil {
		t.Fatalf("initialize: %v", initErr)
	}

	tools, err := client.ListAllTools(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("unexpected tools: %+v", tools)
	}

	result, err := client.CallTool(ctx, "echo", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "echo-reply" {
		t.Fatalf("unexpected call result: %+v", result)
	}
}

// lookPath is a tiny wrapper to make the test intent obvious.
func lookPath(name string) (string, error) {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		full := filepath.Join(dir, name)
		if info, err := os.Stat(full); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return full, nil
		}
	}
	return "", errors.New("not found")
}
