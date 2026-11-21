package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxFetchBodyBytes caps how much of a response body web_fetch reads.
const maxFetchBodyBytes = 5 << 20

// readCapped reads at most limit bytes from r and reports whether more data
// was available (i.e. the body was truncated).
func readCapped(r io.Reader, limit int64) ([]byte, bool, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > limit {
		return data[:limit], true, nil
	}
	return data, false, nil
}

type WebFetchTool struct {
	httpClient *http.Client
}

func NewWebFetchTool() *WebFetchTool {
	return &WebFetchTool{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (t *WebFetchTool) Spec() Spec {
	return Spec{
		Name:        "web_fetch",
		Summary:     "Downloads the content of a URL and extracts clean readable text without HTML tags.",
		Usage:       `web_fetch {"url": "https://example.com/docs"}`,
		InputSchema: schemaWebFetch,
	}
}

func (t *WebFetchTool) Run(ctx context.Context, args string) (Result, error) {
	targetURL := ""
	if parsed := parseJSONArgs(args); parsed != nil {
		targetURL = jsonStr(parsed, "url")
	}
	if targetURL == "" {
		return Result{}, fmt.Errorf("usage: %s", t.Spec().Usage)
	}

	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "https://" + targetURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("network error downloading URL: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("server responded with status code: %d", resp.StatusCode)
	}

	bodyBytes, truncated, err := readCapped(resp.Body, maxFetchBodyBytes)
	if err != nil {
		return Result{}, fmt.Errorf("error reading page content: %w", err)
	}

	baseURL, _ := url.Parse(targetURL)
	cleanedText := cleanHTML(string(bodyBytes), baseURL)
	summary := fmt.Sprintf("URL %s fetched successfully (%d characters extracted).", targetURL, len(cleanedText))
	if truncated {
		summary += fmt.Sprintf(" Page exceeded %d bytes; only the first %d bytes were processed.", maxFetchBodyBytes, maxFetchBodyBytes)
		cleanedText += "\n\n[Page truncated: response body exceeded the size limit]"
	}
	return Result{
		Spec:    t.Spec(),
		Summary: summary,
		Output:  cleanedText,
	}, nil
}

func cleanHTML(html string, baseURL *url.URL) string {
	cleaned := html
	for _, tag := range []string{"script", "style", "head", "nav", "header", "footer"} {
		cleaned = stripBlockTag(cleaned, tag)
	}

	var builder strings.Builder
	inTag := false
	var currentTag strings.Builder
	var linkStack []string

	tagNewlines := map[string]bool{
		"p":          true,
		"br":         true,
		"div":        true,
		"li":         true,
		"tr":         true,
		"h1":         true,
		"h2":         true,
		"h3":         true,
		"h4":         true,
		"h5":         true,
		"h6":         true,
		"ul":         true,
		"ol":         true,
		"table":      true,
		"blockquote": true,
	}

	runes := []rune(cleaned)
	for i := range runes {
		r := runes[i]
		if r == '<' {
			inTag = true
			currentTag.Reset()
		} else if r == '>' {
			inTag = false
			tagStr := currentTag.String()
			lowerTag := strings.ToLower(strings.TrimSpace(tagStr))

			isAnchor := strings.HasPrefix(lowerTag, "a ") || lowerTag == "a"
			isCloseAnchor := lowerTag == "/a"

			if isAnchor {
				href := parseHref(tagStr)
				if href != "" && baseURL != nil {
					refURL, err := url.Parse(href)
					if err == nil {
						resolved := baseURL.ResolveReference(refURL)
						if isSameDomain(resolved.Host, baseURL.Host) {
							href = resolved.String()
						} else {
							href = ""
						}
					}
				}
				if href != "" {
					linkStack = append(linkStack, href)
					builder.WriteRune('[')
				} else {
					linkStack = append(linkStack, "")
				}
			} else if isCloseAnchor {
				if len(linkStack) > 0 {
					href := linkStack[len(linkStack)-1]
					linkStack = linkStack[:len(linkStack)-1]
					if href != "" {
						builder.WriteString("](")
						builder.WriteString(href)
						builder.WriteRune(')')
					}
				}
			} else {
				tagWord := lowerTag
				if idx := strings.IndexAny(tagWord, " \t\r\n/"); idx != -1 {
					tagWord = tagWord[:idx]
				}
				if tagNewlines[tagWord] {
					builder.WriteRune('\n')
				}
			}
		} else if inTag {
			currentTag.WriteRune(r)
		} else {
			builder.WriteRune(r)
		}
	}

	text := htmlUnescape(builder.String())
	lines := strings.Split(text, "\n")
	var cleanedLines []string
	for _, line := range lines {
		cleanedLine := collapseSpaces(line)
		if cleanedLine != "" {
			cleanedLines = append(cleanedLines, cleanedLine)
		}
	}

	return strings.Join(cleanedLines, "\n")
}

// indexFold returns the index of the first occurrence of substr (which must be
// lowercase ASCII) in s, comparing ASCII letters case-insensitively. Unlike
// strings.ToLower, it never changes byte offsets, so indices are valid in s.
func indexFold(s, substr string) int {
	n := len(substr)
	for i := 0; i+n <= len(s); i++ {
		j := 0
		for ; j < n; j++ {
			c := s[i+j]
			if 'A' <= c && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != substr[j] {
				break
			}
		}
		if j == n {
			return i
		}
	}
	return -1
}

func stripBlockTag(html string, tag string) string {
	startTag := "<" + tag
	endTag := "</" + tag + ">"

	var out strings.Builder
	pos := 0
	noEnd := false
	for {
		rel := indexFold(html[pos:], startTag)
		if rel == -1 {
			break
		}
		startIdx := pos + rel

		closeRel := strings.IndexByte(html[startIdx:], '>')
		if closeRel == -1 {
			break
		}
		tagCloseIdx := startIdx + closeRel

		endRel := -1
		if !noEnd {
			endRel = indexFold(html[tagCloseIdx:], endTag)
			// No closing tag after this point means none after any later one.
			noEnd = endRel == -1
		}
		if endRel == -1 {
			out.WriteString(html[pos:startIdx])
			pos = tagCloseIdx + 1
			continue
		}

		out.WriteString(html[pos:startIdx])
		out.WriteByte(' ')
		pos = tagCloseIdx + endRel + len(endTag)
	}
	out.WriteString(html[pos:])
	return out.String()
}

func collapseSpaces(s string) string {
	var builder strings.Builder
	lastSpace := false
	for _, r := range strings.TrimSpace(s) {
		if r == ' ' || r == '\t' || r == '\r' {
			if !lastSpace {
				builder.WriteRune(' ')
				lastSpace = true
			}
		} else {
			builder.WriteRune(r)
			lastSpace = false
		}
	}
	return builder.String()
}

func parseHref(tag string) string {
	lower := strings.ToLower(tag)
	idx := strings.Index(lower, "href=")
	if idx == -1 {
		return ""
	}
	val := tag[idx+5:]
	val = strings.TrimSpace(val)
	if len(val) == 0 {
		return ""
	}
	quote := val[0]
	if quote == '"' || quote == '\'' {
		val = val[1:]
		before, _, ok := strings.Cut(val, string(quote))
		if !ok {
			return ""
		}
		return before
	}
	endIdx := strings.IndexAny(val, " \t\r\n>")
	if endIdx == -1 {
		return val
	}
	return val[:endIdx]
}

func isSameDomain(host1, host2 string) bool {
	h1 := strings.ToLower(host1)
	h2 := strings.ToLower(host2)
	if h1 == h2 {
		return true
	}
	// Strip "www." prefix for comparison
	h1 = strings.TrimPrefix(h1, "www.")
	h2 = strings.TrimPrefix(h2, "www.")
	if h1 == h2 {
		return true
	}
	// Check if one is a subdomain of the other
	if strings.HasSuffix(h1, "."+h2) || strings.HasSuffix(h2, "."+h1) {
		return true
	}
	return false
}
