package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testRoundTripFunc func(*http.Request) (*http.Response, error)

func (f testRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestWebSearchToolRun(t *testing.T) {
	mockResponse := `
<!DOCTYPE html>
<html>
<body>
<!--rs-->
<li class="r1">
  <h2><a class="title" href="https://golang.org">Go Programming Language</a></h2>
  <p class="s">Go is an open source programming language that makes it easy to build simple, <strong>reliable</strong>, and efficient software.</p>
</li>
<!--re-->
<!--rs-->
<li class="r2">
  <h2><a class="title" href="https://github.com/golang/go">GitHub - golang/go</a></h2>
  <p class="s">The Go programming language source code. Contribute to golang/go development by creating an account on GitHub.</p>
</li>
<!--re-->
</body>
</html>
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "test query" {
			t.Errorf("expected query 'test query', got '%s'", r.URL.Query().Get("q"))
		}
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	tool := NewWebSearchTool()
	targetHost := strings.TrimPrefix(server.URL, "http://")
	tool.httpClient.Transport = testRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = targetHost
		return http.DefaultTransport.RoundTrip(req)
	})

	result, err := tool.Run(context.Background(), "test query")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result.Summary, "2 resultados") {
		t.Errorf("expected summary to contain '2 resultados', got '%s'", result.Summary)
	}

	expectedOutput := "1. Go Programming Language\n   URL: https://golang.org\n   Go is an open source programming language that makes it easy to build simple, reliable, and efficient software.\n\n2. GitHub - golang/go\n   URL: https://github.com/golang/go\n   The Go programming language source code. Contribute to golang/go development by creating an account on GitHub."
	if strings.TrimSpace(result.Output) != expectedOutput {
		t.Errorf("expected output:\n%s\ngot:\n%s", expectedOutput, result.Output)
	}
}

func TestWebSearchToolRunEmptyQuery(t *testing.T) {
	tool := NewWebSearchTool()
	_, err := tool.Run(context.Background(), "   ")
	if err == nil {
		t.Fatal("expected error on empty query")
	}
}

func TestWebSearchToolRunFallback(t *testing.T) {
	mockDDGResponse := `
<!DOCTYPE html>
<html>
<body>
<div class="result">
  <h2 class="result__title">
    <a class="result__a" href="/l/?kh=-1&uddg=https%3A%2F%2Fgo.dev%2F">Go Dev Portal</a>
  </h2>
  <a class="result__snippet" href="/l/?kh=-1&uddg=https%3A%2F%2Fgo.dev%2F">Go dev features and resources.</a>
</div>
</body>
</html>
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Host, "mojeek") || strings.Contains(r.URL.Path, "search") {
			// Mojeek returns empty results
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<html><body>No results</body></html>"))
			return
		}
		// DDG returns results
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockDDGResponse))
	}))
	defer server.Close()

	tool := NewWebSearchTool()
	targetHost := strings.TrimPrefix(server.URL, "http://")
	tool.httpClient.Transport = testRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = targetHost
		return http.DefaultTransport.RoundTrip(req)
	})

	result, err := tool.Run(context.Background(), "fallback query")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result.Summary, "DuckDuckGo") {
		t.Errorf("expected summary to mention 'DuckDuckGo', got '%s'", result.Summary)
	}

	expectedOutput := "1. Go Dev Portal\n   URL: https://go.dev/\n   Go dev features and resources."
	if strings.TrimSpace(result.Output) != expectedOutput {
		t.Errorf("expected output:\n%s\ngot:\n%s", expectedOutput, result.Output)
	}
}

