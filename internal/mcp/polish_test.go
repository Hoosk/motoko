package mcp

import (
	"bufio"
	"context"
	"io"
	"testing"
	"time"
)

func TestClientRateLimiting(t *testing.T) {
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		defer serverWriter.Close()
		scanner := bufio.NewScanner(serverReader)
		for scanner.Scan() {
			// Read but do not reply to hold the sema slots
		}
	}()

	transport := newPipeTransport(
		bufio.NewReader(clientReader),
		clientWriter,
		clientWriter,
	)

	client := NewClient(ClientConfig{
		Transport:      transport,
		ClientInfo:     Implementation{Name: "test", Version: "0"},
		RequestTimeout: 100 * time.Millisecond,
	})
	client.Start(ctx)
	defer client.Close()

	// Fill the semaphore slots (limit is 10)
	for i := 0; i < 10; i++ {
		go func() {
			_ = client.Request(ctx, "hold", nil, nil)
		}()
	}

	time.Sleep(20 * time.Millisecond)

	// The 11th request should be blocked.
	shortCtx, shortCancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer shortCancel()

	start := time.Now()
	err := client.Request(shortCtx, "blocked", nil, nil)
	duration := time.Since(start)

	if err == nil {
		t.Error("expected 11th request to fail due to rate limiting/timeout")
	}
	if duration < 10*time.Millisecond {
		t.Errorf("expected request to block for at least 10ms, finished in %v", duration)
	}
}

func TestManagerReconnection(t *testing.T) {
	m := NewManager(ManagerConfig{
		Timeout: 50 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &managedServer{
		cfg: ServerConfig{
			Name:      "reconnect-test",
			Transport: "stdio",
			Command:   "nonexistent-command-to-force-transport-failure",
		},
	}

	go func() {
		m.runServer(ctx, s)
	}()

	time.Sleep(150 * time.Millisecond)
	cancel()

	m.mu.Lock()
	err := s.err
	m.mu.Unlock()

	if err == nil {
		t.Error("expected manager to record transport failure errors")
	}
}
