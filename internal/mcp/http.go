package mcp

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// HTTPTransport implements the Transport interface using Server-Sent Events (SSE)
// for receiving messages and HTTP POST for sending messages.
type HTTPTransport struct {
	endpointCtx    context.Context
	endpointCancel context.CancelFunc
	client         *http.Client
	headers        map[string]string
	recvCh         chan []byte
	errCh          chan error
	readyCh        chan struct{}
	sseURL         string
	msgURL         string
	mu             sync.Mutex
	closed         bool
}

// NewHTTPTransport creates a new HTTPTransport pointing to the given sseURL.
func NewHTTPTransport(sseURL string, headers map[string]string, timeout time.Duration) *HTTPTransport {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	t := &HTTPTransport{
		endpointCtx:    ctx,
		endpointCancel: cancel,
		sseURL:         sseURL,
		client:         &http.Client{Timeout: timeout},
		headers:        headers,
		recvCh:         make(chan []byte, 100),
		errCh:          make(chan error, 1),
		readyCh:        make(chan struct{}),
	}

	go func() {
		t.connectAndRead(ctx)
		t.mu.Lock()
		if !t.closed {
			select {
			case t.errCh <- io.EOF:
			default:
			}
		}
		t.mu.Unlock()
	}()

	return t
}

// Send POSTs a payload to the resolved message URL.
func (t *HTTPTransport) Send(ctx context.Context, payload []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.endpointCtx.Done():
		return ErrTransportClosed
	case <-t.readyCh:
	}

	t.mu.Lock()
	msgURL := t.msgURL
	t.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, "POST", msgURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("mcp: HTTP POST error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Recv retrieves the next payload from the SSE stream.
func (t *HTTPTransport) Recv(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.endpointCtx.Done():
		return nil, ErrTransportClosed
	case data := <-t.recvCh:
		return data, nil
	case err := <-t.errCh:
		return nil, err
	}
}

// Close terminates the transport, cancelling the background reader context.
func (t *HTTPTransport) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	t.mu.Unlock()

	t.endpointCancel()
	t.client.CloseIdleConnections()
	return nil
}

func (t *HTTPTransport) connectAndRead(ctx context.Context) {
	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second

	for {
		t.mu.Lock()
		if t.closed {
			t.mu.Unlock()
			return
		}
		t.mu.Unlock()

		err := t.readSSEStream(ctx)
		if err == nil {
			return
		}

		select {
		case <-ctx.Done():
			return
		default:
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (t *HTTPTransport) readSSEStream(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", t.sseURL, nil)
	if err != nil {
		return err
	}
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")

	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
	var currentEvent string
	var currentData strings.Builder

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}

		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		if line == "" {
			if currentEvent != "" || currentData.Len() > 0 {
				if err := t.dispatchEvent(currentEvent, currentData.String()); err != nil {
					return err
				}
				currentEvent = ""
				currentData.Reset()
			}
			continue
		}

		if strings.HasPrefix(line, ":") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		field := parts[0]
		var value string
		if len(parts) > 1 {
			value = strings.TrimSpace(parts[1])
		}

		switch field {
		case "event":
			currentEvent = value
		case "data":
			if currentData.Len() > 0 {
				currentData.WriteString("\n")
			}
			currentData.WriteString(value)
		}
	}
}

func (t *HTTPTransport) dispatchEvent(event, data string) error {
	switch event {
	case "endpoint":
		base, err := url.Parse(t.sseURL)
		if err != nil {
			return fmt.Errorf("invalid sse URL: %w", err)
		}
		resolved, err := base.Parse(data)
		if err != nil {
			return fmt.Errorf("failed to parse message endpoint: %w", err)
		}

		t.mu.Lock()
		firstTime := t.msgURL == ""
		t.msgURL = resolved.String()
		t.mu.Unlock()

		if firstTime {
			close(t.readyCh)
		}
	case "message":
		select {
		case t.recvCh <- []byte(data):
		case <-t.endpointCtx.Done():
			return t.endpointCtx.Err()
		}
	default:
		if event == "" && data != "" {
			select {
			case t.recvCh <- []byte(data):
			case <-t.endpointCtx.Done():
				return t.endpointCtx.Err()
			}
		}
	}
	return nil
}
