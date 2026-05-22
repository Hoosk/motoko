package provider

import (
	"bytes"
	"context"
	"net/http"
	"testing"
)

func TestPostJSONStreamParsesSSEPayloads(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := "data: {\"message\":\"ho\"}\n\ndata: {\"message\":\"la\"}\n\ndata: [DONE]\n\n"
		return &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Body:       ioNopCloser{bytes.NewBufferString(body)},
		}, nil
	})}
	var events []string
	err := postJSONStream(context.Background(), client, "http://example.test", map[string]string{"hello": "world"}, nil, func(data string) error {
		events = append(events, data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 stream events, got %#v", events)
	}
}

func TestPostJSONStreamReturnsProviderErrorBody(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 400,
			Header:     make(http.Header),
			Body:       ioNopCloser{bytes.NewBufferString("bad request")},
		}, nil
	})}
	err := postJSONStream(context.Background(), client, "http://example.test", map[string]string{}, nil, func(data string) error { return nil })
	if err == nil || err.Error() != "provider error 400: bad request" {
		t.Fatalf("unexpected stream error %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type ioNopCloser struct{ *bytes.Buffer }

func (ioNopCloser) Close() error { return nil }
