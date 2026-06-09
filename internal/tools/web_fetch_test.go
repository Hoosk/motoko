package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebFetchToolRun(t *testing.T) {
	// We will resolve relative URLs and subdomains, but strip external hosts
	mockResponse := `
<!DOCTYPE html>
<html>
<head>
  <title>Skip this title</title>
  <style>body { color: red; }</style>
</head>
<body>
  <header>
    <nav>Home | Contact</nav>
  </header>
  <h1>Main Heading</h1>
  <div>
    <p>This is paragraph one with <strong>bold</strong> and <a href="https://example.com">external link</a>.</p>
    <script>console.log("skip this");</script>
    <p>Paragraph two with a <a href="/relative/path">subpage</a>.</p>
    <p>Paragraph three with a <a href="SUBDOMAIN_PLACEHOLDER">subdomain</a>.</p>
  </div>
  <footer>Copyright 2026</footer>
</body>
</html>
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Replace placeholder with dynamic subdomain on test server port
		html := strings.ReplaceAll(mockResponse, "SUBDOMAIN_PLACEHOLDER", "http://sub."+r.Host+"/subpath")
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	targetHost := strings.TrimPrefix(server.URL, "http://")
	tool.httpClient.Transport = testRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = targetHost
		return http.DefaultTransport.RoundTrip(req)
	})

	result, err := tool.Run(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}

	expectedOutput := "Main Heading\nThis is paragraph one with bold and external link.\nParagraph two with a [subpage](" + server.URL + "/relative/path).\nParagraph three with a [subdomain](http://sub." + targetHost + "/subpath)."
	if strings.TrimSpace(result.Output) != expectedOutput {
		t.Errorf("expected output:\n%q\ngot:\n%q", expectedOutput, result.Output)
	}
}

func TestWebFetchToolRunInvalidURL(t *testing.T) {
	tool := NewWebFetchTool()
	_, err := tool.Run(context.Background(), "   ")
	if err == nil {
		t.Fatal("expected error on empty URL")
	}
}
