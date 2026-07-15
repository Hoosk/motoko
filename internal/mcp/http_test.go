package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestHTTPTransport(t *testing.T) {
	var (
		mu           sync.Mutex
		receivedPOST []string
		sseDone      = make(chan struct{})
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/sse" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
				return
			}

			msgPath := "/messages?sessionId=test-session"
			_, _ = fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", msgPath)
			flusher.Flush()

			<-sseDone
			return
		}

		if r.Method == "POST" && r.URL.Path == "/messages" {
			mu.Lock()
			receivedPOST = append(receivedPOST, r.URL.RawQuery)
			mu.Unlock()

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	sseURL := server.URL + "/sse"
	transport := NewHTTPTransport(sseURL, map[string]string{"X-Test-Header": "yes"}, 5*time.Second)
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	select {
	case <-transport.readyCh:
	case <-ctx.Done():
		t.Fatalf("timeout waiting for endpoint event: %v", ctx.Err())
	}

	err := transport.Send(ctx, []byte(`{"jsonrpc":"2.0","method":"initialize","id":1}`))
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	mu.Lock()
	count := len(receivedPOST)
	mu.Unlock()

	if count != 1 {
		t.Errorf("expected 1 POST request, got %d", count)
	}

	close(sseDone)
}
