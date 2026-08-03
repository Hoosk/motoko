package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Hoosk/motoko/internal/tracelog"
)

// StreamableConfig configures a StreamableTransport.
type StreamableConfig struct {
	Headers        map[string]string
	Endpoint       string
	RequestTimeout time.Duration
}

// StreamableTransport implements Transport using the Streamable HTTP transport.
// It sends messages via POST to a single MCP endpoint. On modern connections
// (2026-07-28) it is fully stateless; on legacy connections it captures the
// session id returned during initialize and sends it back on every request.
type StreamableTransport struct {
	endpointCtx    context.Context
	errCh          chan error
	postClient     *http.Client
	streamClient   *http.Client
	headers        map[string]string
	recvCh         chan []byte
	endpointCancel context.CancelFunc
	streamClosedCh chan struct{}
	protocol       string
	endpoint       string
	sessionID      string
	lastPayload    []byte
	streamWg       sync.WaitGroup
	mu             sync.Mutex
	closed         bool
}

// NewStreamableTransport creates a new StreamableTransport. The transport is
// immediately ready to send; the GET stream is opened lazily on the first
// Recv that needs it.
func NewStreamableTransport(cfg StreamableConfig) *StreamableTransport {
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 30 * time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &StreamableTransport{
		endpointCtx:    ctx,
		endpointCancel: cancel,
		endpoint:       cfg.Endpoint,
		postClient:     &http.Client{Timeout: cfg.RequestTimeout},
		streamClient:   &http.Client{},
		headers:        cfg.Headers,
		recvCh:         make(chan []byte, 100),
		errCh:          make(chan error, 1),
	}
}

// Send POSTs a JSON-RPC payload. If the server responds with 202 Accepted
// (notification or response without a body), Send returns nil. For requests
// the response (JSON or SSE) is consumed and a single payload is delivered to
// the caller via Recv.
func (t *StreamableTransport) Send(ctx context.Context, payload []byte) error {
	t.mu.Lock()
	t.lastPayload = append(t.lastPayload[:0], payload...)
	t.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	t.attachHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := t.postClient.Do(req)
	if err != nil {
		return fmt.Errorf("mcp: streamable POST: %w", err)
	}

	switch {
	case resp.StatusCode == http.StatusAccepted:
		_ = resp.Body.Close()
		return nil
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return t.consumePostResponse(ctx, resp)
	case resp.StatusCode == http.StatusNotFound && t.hasSession():
		// Session expired (legacy servers only); clear it so the next
		// attempt re-initialises.
		_ = resp.Body.Close()
		tracelog.Logf("MCP[streamable] session expired (404), clearing")
		t.clearSession()
		return fmt.Errorf("mcp: session expired (404)")
	default:
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return fmt.Errorf("mcp: streamable POST error %d: %s", resp.StatusCode, string(body))
	}
}

// requestMeta is the small subset of a JSON-RPC request body the transport
// needs to mirror into HTTP headers (Mcp-Method / Mcp-Name).
type requestMeta struct {
	Method string `json:"method"`
	Params struct {
		Name string `json:"name"`
		URI  string `json:"uri"`
	} `json:"params"`
}

// consumePostResponse handles a 2xx response to a POST. The response body is
// either a single JSON object (application/json) or an SSE stream
// (text/event-stream) — the spec says clients must support both.
func (t *StreamableTransport) consumePostResponse(ctx context.Context, resp *http.Response) error {
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		t.mu.Lock()
		if t.sessionID == "" {
			t.sessionID = sid
		}
		t.mu.Unlock()
	}
	if pv := resp.Header.Get("MCP-Protocol-Version"); pv != "" {
		t.mu.Lock()
		if t.protocol == "" {
			t.protocol = pv
		}
		t.mu.Unlock()
	}
	contentType := resp.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/json") {
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("mcp: read JSON response: %w", err)
		}
		if len(data) == 0 {
			return nil
		}
		select {
		case t.recvCh <- data:
		case <-ctx.Done():
			return ctx.Err()
		case <-t.endpointCtx.Done():
			return ErrTransportClosed
		}
		return nil
	}
	if strings.HasPrefix(contentType, "text/event-stream") {
		// The SSE stream may be short (a normal response, terminated by the
		// final result) or long-lived (subscriptions/listen). Either way the
		// events are routed to recvCh; consume the body in a background
		// goroutine so Send never blocks on a stream that stays open. The
		// goroutine owns the body and closes it.
		body := resp.Body
		go func() {
			defer body.Close()
			_ = t.consumePostSSE(ctx, body)
		}()
		return nil
	}
	_ = resp.Body.Close()
	return fmt.Errorf("mcp: streamable POST returned unexpected content-type %q", contentType)
}

func (t *StreamableTransport) consumePostSSE(ctx context.Context, body io.Reader) error {
	reader := bufio.NewReader(body)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.endpointCtx.Done():
			return ErrTransportClosed
		default:
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("mcp: read SSE response: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" {
				continue
			}
			select {
			case t.recvCh <- []byte(data):
			case <-ctx.Done():
				return ctx.Err()
			case <-t.endpointCtx.Done():
				return ErrTransportClosed
			}
		}
	}
}

// Recv returns the next inbound message. On legacy connections it lazily
// opens the GET stream the first time it's called; modern (stateless)
// connections have no GET stream — change notifications arrive on the
// subscriptions/listen stream instead (not yet wired).
func (t *StreamableTransport) Recv(ctx context.Context) ([]byte, error) {
	t.mu.Lock()
	modern := t.protocol == ProtocolVersionModern
	if t.streamClosedCh == nil && !modern {
		t.openGetStream()
	}
	t.mu.Unlock()
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

// openGetStream launches the background goroutine that maintains an SSE GET
// stream for unsolicited server-to-client messages.
func (t *StreamableTransport) openGetStream() {
	t.streamClosedCh = make(chan struct{})
	t.streamWg.Add(1)
	go func() {
		defer t.streamWg.Done()
		defer close(t.streamClosedCh)
		t.runGetStream(t.endpointCtx)
	}()
}

func (t *StreamableTransport) runGetStream(ctx context.Context) {
	backoff := 1 * time.Second
	const maxBackoff = 30 * time.Second
	for {
		if t.isClosed() {
			return
		}
		err := t.readGetStream(ctx)
		if ctx.Err() != nil {
			return
		}
		tracelog.Logf("MCP[streamable] GET stream ended: %v (reconnecting in %s)", err, backoff)
		select {
		case <-ctx.Done():
			return
		case <-t.endpointCtx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func (t *StreamableTransport) readGetStream(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.endpoint, nil)
	if err != nil {
		return err
	}
	t.attachHeaders(req)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := t.streamClient.Do(req)
	if err != nil {
		return fmt.Errorf("mcp: streamable GET: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusMethodNotAllowed {
		// Server doesn't support unsolicited GET streams; close quietly.
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mcp: streamable GET error %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
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
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" {
				continue
			}
			select {
			case t.recvCh <- []byte(data):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

// Close terminates the transport, cancelling the background reader context.
func (t *StreamableTransport) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	t.mu.Unlock()

	t.endpointCancel()
	t.postClient.CloseIdleConnections()
	t.streamClient.CloseIdleConnections()
	t.streamWg.Wait()
	return nil
}

func (t *StreamableTransport) isClosed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}

// attachHeaders sets the auth, session, and protocol-version headers that
// every request must carry (per spec).
func (t *StreamableTransport) attachHeaders(req *http.Request) {
	t.mu.Lock()
	sessionID := t.sessionID
	protocol := t.protocol
	// Copy lastPayload under the lock so the read does not race with the
	// write in Send() which also holds t.mu when updating it.
	var lastPayload []byte
	if len(t.lastPayload) > 0 {
		lastPayload = append(lastPayload, t.lastPayload...)
	}
	t.mu.Unlock()

	// In the stateless protocol the session id is gone; only legacy
	// connections (2025-11-25 and earlier) carry it.
	if sessionID != "" && protocol != ProtocolVersionModern {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	if protocol != "" {
		req.Header.Set("MCP-Protocol-Version", protocol)
	}

	// Mirror the JSON-RPC method and object name into headers so
	// intermediaries can route without parsing the body (spec 2026-07-28).
	var meta requestMeta
	_ = json.Unmarshal(lastPayload, &meta)
	if meta.Method != "" {
		req.Header.Set("Mcp-Method", meta.Method)
	}
	switch meta.Method {
	case "tools/call", "resources/read", "prompts/get":
		name := meta.Params.Name
		if name == "" {
			name = meta.Params.URI
		}
		if name != "" {
			req.Header.Set("Mcp-Name", encodeHeaderValue(name))
		}
	}

	for k, v := range t.headers {
		req.Header.Set(k, v)
	}
}

// encodeHeaderValue renders a value for use as an HTTP header. Values that
// are plain visible ASCII pass through; values with non-ASCII characters,
// control characters, or leading/trailing whitespace are wrapped in the
// base64 sentinel format required by the spec.
func encodeHeaderValue(v string) string {
	trimmed := strings.TrimLeft(v, " \t")
	if trimmed != v || trimmed != strings.TrimRight(v, " \t") {
		return "=?base64?" + base64.StdEncoding.EncodeToString([]byte(v)) + "?="
	}
	safe := true
	for _, r := range v {
		if r < 0x21 || r > 0x7E {
			safe = false
			break
		}
	}
	if safe && !strings.HasPrefix(v, "=?base64?") {
		return v
	}
	return "=?base64?" + base64.StdEncoding.EncodeToString([]byte(v)) + "?="
}

// SetSession stores the session id returned by the server.
func (t *StreamableTransport) SetSession(sessionID string) {
	t.mu.Lock()
	t.sessionID = sessionID
	t.mu.Unlock()
}

// SetProtocol records the negotiated protocol version so it can be sent as
// the MCP-Protocol-Version header on every subsequent request.
func (t *StreamableTransport) SetProtocol(version string) {
	t.mu.Lock()
	t.protocol = version
	t.mu.Unlock()
}

func (t *StreamableTransport) hasSession() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sessionID != ""
}

func (t *StreamableTransport) clearSession() {
	t.mu.Lock()
	t.sessionID = ""
	t.mu.Unlock()
}
