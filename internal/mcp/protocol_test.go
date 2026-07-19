package mcp

import (
	"encoding/json"
	"testing"
)

func TestRPCEnvelopeDetection(t *testing.T) {
	tests := []struct {
		name      string
		payload   string
		wantResp  bool
		wantErr   bool
		wantNotif bool
		wantReq   bool
	}{
		{
			name:     "result response",
			payload:  `{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`,
			wantResp: true,
		},
		{
			name:    "error response",
			payload: `{"jsonrpc":"2.0","id":2,"error":{"code":-32601,"message":"no"}}`,
			wantErr: true,
		},
		{
			name:      "notification",
			payload:   `{"jsonrpc":"2.0","method":"notifications/initialized"}`,
			wantNotif: true,
		},
		{
			name:    "request",
			payload: `{"jsonrpc":"2.0","id":3,"method":"ping"}`,
			wantReq: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var env RPCEnvelope
			if err := json.Unmarshal([]byte(tt.payload), &env); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := env.IsResponse(); got != tt.wantResp {
				t.Errorf("IsResponse=%v want %v", got, tt.wantResp)
			}
			if got := env.IsErrorResponse(); got != tt.wantErr {
				t.Errorf("IsErrorResponse=%v want %v", got, tt.wantErr)
			}
			if got := env.IsNotification(); got != tt.wantNotif {
				t.Errorf("IsNotification=%v want %v", got, tt.wantNotif)
			}
			if got := env.IsRequest(); got != tt.wantReq {
				t.Errorf("IsRequest=%v want %v", got, tt.wantReq)
			}
		})
	}
}

func TestRequestIDMarshal(t *testing.T) {
	cases := []struct {
		name string
		want string
		id   RequestID
	}{
		{name: "string", id: NewStringID("abc"), want: `"abc"`},
		{name: "int", id: NewIntID(42), want: `42`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := json.Marshal(c.id.Raw())
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(got) != c.want {
				t.Errorf("got %s want %s", got, c.want)
			}
		})
	}
}

func TestInitializeParamsRoundTrip(t *testing.T) {
	params := InitializeParams{
		ProtocolVersion: ProtocolVersion,
		Capabilities: ClientCapabilities{
			Roots: &struct {
				ListChanged bool `json:"listChanged,omitempty"`
			}{ListChanged: true},
		},
		ClientInfo: Implementation{
			Name:    "motoko-test",
			Version: "0.0.1",
		},
	}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back InitializeParams
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.ProtocolVersion != params.ProtocolVersion {
		t.Errorf("protocol version lost: %q", back.ProtocolVersion)
	}
	if back.ClientInfo.Name != params.ClientInfo.Name {
		t.Errorf("client name lost: %q", back.ClientInfo.Name)
	}
	if back.Capabilities.Roots == nil || !back.Capabilities.Roots.ListChanged {
		t.Errorf("roots capability lost")
	}
}

func TestCallToolResultJSON(t *testing.T) {
	r := CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: "hello"}},
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !contains(data, `"text":"hello"`) {
		t.Errorf("missing text in marshalled payload: %s", data)
	}
}

func contains(haystack []byte, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack []byte, needle string) int {
	hn := len(haystack)
	nn := len(needle)
	if nn == 0 {
		return 0
	}
	for i := 0; i+nn <= hn; i++ {
		if string(haystack[i:i+nn]) == needle {
			return i
		}
	}
	return -1
}
