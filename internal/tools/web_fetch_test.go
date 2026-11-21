package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

	result, err := tool.Run(context.Background(), `{"url":"`+server.URL+`"}`)
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
	_, err := tool.Run(context.Background(), `{"url":"   "}`)
	if err == nil {
		t.Fatal("expected error on empty URL")
	}
}

func TestStripBlockTagInvalidUTF8DoesNotPanic(t *testing.T) {
	in := strings.Repeat("\xe9", 20) + "<script>x</script>after"
	got := stripBlockTag(in, "script")
	want := strings.Repeat("\xe9", 20) + " after"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStripBlockTagLatin1KeepsContent(t *testing.T) {
	in := "caf\xe9 \xe9\xe9\xe9 <style>p{}</style><p>ok</p>"
	got := stripBlockTag(in, "style")
	want := "caf\xe9 \xe9\xe9\xe9  <p>ok</p>"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStripBlockTagKelvinSignBeforeTag(t *testing.T) {
	in := strings.Repeat("\u212a", 5) + "<SCRIPT>secret()</SCRIPT>tail"
	got := stripBlockTag(in, "script")
	if strings.Contains(got, "secret") {
		t.Fatalf("script body leaked: %q", got)
	}
	if got != strings.Repeat("\u212a", 5)+" tail" {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestStripBlockTagManyTagsIsFast(t *testing.T) {
	for _, in := range []string{
		strings.Repeat("<script></script>", 20000) + strings.Repeat("x", 2<<20),
		strings.Repeat("<script>", 20000) + strings.Repeat("x", 2<<20),
	} {
		start := time.Now()
		_ = stripBlockTag(in, "script")
		if d := time.Since(start); d > 3*time.Second {
			t.Fatalf("stripBlockTag too slow: %v", d)
		}
	}
}

func TestWebFetchTruncatesHugeBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", maxFetchBodyBytes+1000)))
	}))
	defer srv.Close()

	res, err := NewWebFetchTool().Run(context.Background(), `{"url": "`+srv.URL+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Output) > maxFetchBodyBytes+200 {
		t.Fatalf("output not capped: %d bytes", len(res.Output))
	}
	if !strings.Contains(res.Output, "Page truncated") || !strings.Contains(res.Summary, "only the first") {
		t.Fatalf("truncation not visible: %q", res.Summary)
	}
}

func TestWebSearchFetchHTMLCapsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", maxSearchBodyBytes+1000)))
	}))
	defer srv.Close()

	body, err := NewWebSearchTool().fetchHTML(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != maxSearchBodyBytes {
		t.Fatalf("expected body capped at %d, got %d", maxSearchBodyBytes, len(body))
	}
}
