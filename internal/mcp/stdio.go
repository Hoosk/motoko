package mcp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// StdioConfig holds the configuration required to spawn a stdio MCP server.
type StdioConfig struct {
	Stderr  io.Writer
	Command string
	Args    []string
	Env     []string
}

type readResult struct {
	err  error
	line []byte
}

// StdioTransport implements Transport by exec-ing a child process and
// exchanging JSON-RPC messages over its stdin/stdout pipes.
type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
	recvCh chan readResult
	stopCh chan struct{}
	mu     sync.Mutex
	closed bool
}

// NewStdioTransport starts the given command and returns a transport ready to
// exchange JSON-RPC messages. The caller MUST call Close to reap the child.
func NewStdioTransport(cfg StdioConfig) (*StdioTransport, error) {
	if cfg.Command == "" {
		return nil, fmt.Errorf("mcp: stdio transport requires Command")
	}
	cmd := exec.Command(cfg.Command, cfg.Args...)
	cmd.Env = append([]string{}, cfg.Env...)
	if cfg.Stderr != nil {
		cmd.Stderr = cfg.Stderr
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("mcp: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("mcp: start command: %w", err)
	}
	t := &StdioTransport{
		cmd:    cmd,
		stdin:  stdin,
		reader: bufio.NewReader(stdout),
		recvCh: make(chan readResult),
		stopCh: make(chan struct{}),
	}
	go t.readLoop()
	return t, nil
}

func (t *StdioTransport) readLoop() {
	for {
		line, err := t.reader.ReadBytes('\n')
		select {
		case <-t.stopCh:
			return
		case t.recvCh <- readResult{line: line, err: err}:
		}
		if err != nil {
			return
		}
	}
}

// Send writes a JSON-RPC payload terminated with a single newline. Per the
// spec, messages MUST NOT contain embedded newlines; we still escape nothing
// and rely on the JSON encoder having already produced a single line.
func (t *StdioTransport) Send(ctx context.Context, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return ErrTransportClosed
	}
	if _, err := t.stdin.Write(payload); err != nil {
		return fmt.Errorf("mcp: write payload: %w", err)
	}
	if _, err := t.stdin.Write([]byte("\n")); err != nil {
		return fmt.Errorf("mcp: write newline: %w", err)
	}
	return nil
}

// Recv reads one newline-terminated JSON-RPC payload from the child stdout.
func (t *StdioTransport) Recv(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.stopCh:
		return nil, ErrTransportClosed
	case r, ok := <-t.recvCh:
		if !ok {
			return nil, ErrTransportClosed
		}
		if r.err != nil {
			if isEOF(r.err) {
				return nil, io.EOF
			}
			return nil, fmt.Errorf("mcp: read payload: %w", r.err)
		}
		return trimNewline(r.line), nil
	}
}

// Close stops the transport gracefully. It first closes stdin (so well-behaved
// servers can exit), waits briefly, then escalates to SIGKILL if the process
// has not exited on its own. The brief grace period keeps tests that exercise
// the happy path deterministic without spinning.
func (t *StdioTransport) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	close(t.stopCh)
	t.mu.Unlock()

	_ = t.stdin.Close()
	if t.cmd == nil || t.cmd.Process == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		_ = t.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-time.After(2 * time.Second):
		_ = t.cmd.Process.Kill()
		<-done
		return nil
	}
}

// PID returns the child process PID, or 0 if the command has not been
// started. Useful for diagnostics.
func (t *StdioTransport) PID() int {
	if t.cmd == nil || t.cmd.Process == nil {
		return 0
	}
	return t.cmd.Process.Pid
}

func trimNewline(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}
