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

type SearchResponseItem struct {
	URL     string
	Title   string
	Snippet string
}

type WebSearchTool struct {
	httpClient *http.Client
}

func NewWebSearchTool() *WebSearchTool {
	return &WebSearchTool{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (t *WebSearchTool) Spec() Spec {
	return Spec{
		Name:    "web_search",
		Summary: "Busca en la web utilizando el motor de búsqueda Mojeek.",
		Usage:   "web_search <consulta>",
	}
}

func (t *WebSearchTool) Run(ctx context.Context, args string) (Result, error) {
	query := strings.TrimSpace(args)
	if query == "" {
		return Result{}, fmt.Errorf("uso: %s", t.Spec().Usage)
	}

	results, err := t.fetchMojeek(ctx, query)
	usedFallback := false
	if err != nil || len(results) == 0 {
		fallbackResults, fallbackErr := t.fetchDuckDuckGo(ctx, query)
		if fallbackErr == nil && len(fallbackResults) > 0 {
			results = fallbackResults
			usedFallback = true
		}
	}

	engineName := "Mojeek"
	if usedFallback {
		engineName = "DuckDuckGo (fallback)"
	}

	if len(results) == 0 {
		return Result{
			Spec:    t.Spec(),
			Summary: fmt.Sprintf("Búsqueda finalizada para '%s' en %s (0 resultados).", query, engineName),
			Output:  "No se encontraron resultados para esta consulta.",
		}, nil
	}

	var builder strings.Builder
	for i, item := range results {
		fmt.Fprintf(&builder, "%d. %s\n   URL: %s\n   %s\n\n", i+1, item.Title, item.URL, item.Snippet)
	}

	return Result{
		Spec:    t.Spec(),
		Summary: fmt.Sprintf("Búsqueda finalizada para '%s' en %s (%d resultados).", query, engineName, len(results)),
		Output:  strings.TrimSpace(builder.String()),
	}, nil
}

func (t *WebSearchTool) fetchMojeek(ctx context.Context, query string) ([]SearchResponseItem, error) {
	searchURL := fmt.Sprintf("https://www.mojeek.com/search?q=%s", url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parseMojeekHTML(string(bodyBytes)), nil
}

func (t *WebSearchTool) fetchDuckDuckGo(ctx context.Context, query string) ([]SearchResponseItem, error) {
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parseDuckDuckGoHTML(string(bodyBytes)), nil
}

func parseMojeekHTML(html string) []SearchResponseItem {
	var items []SearchResponseItem
	parts := strings.Split(html, "<!--rs-->")
	for _, part := range parts[1:] {
		endIdx := strings.Index(part, "<!--re-->")
		if endIdx != -1 {
			part = part[:endIdx]
		}

		titleH2Start := strings.Index(part, "<h2>")
		if titleH2Start == -1 {
			continue
		}
		titleH2End := strings.Index(part[titleH2Start:], "</h2>")
		if titleH2End == -1 {
			continue
		}
		h2Content := part[titleH2Start : titleH2Start+titleH2End]

		hrefIdx := strings.Index(h2Content, "href=\"")
		if hrefIdx == -1 {
			continue
		}
		hrefVal := h2Content[hrefIdx+6:]
		hrefCloseIdx := strings.Index(hrefVal, "\"")
		if hrefCloseIdx == -1 {
			continue
		}
		urlStr := hrefVal[:hrefCloseIdx]

		anchorCloseIdx := strings.Index(hrefVal, ">")
		if anchorCloseIdx == -1 {
			continue
		}
		titleVal := hrefVal[anchorCloseIdx+1:]
		titleEndIdx := strings.Index(titleVal, "</a>")
		if titleEndIdx == -1 {
			continue
		}
		titleText := stripHTML(titleVal[:titleEndIdx])

		snippetStart := strings.Index(part, "class=\"s\"")
		if snippetStart == -1 {
			snippetStart = strings.Index(part, "class='s'")
		}
		var snippetText string
		if snippetStart != -1 {
			snippetVal := part[snippetStart:]
			closeTagIdx := strings.Index(snippetVal, ">")
			if closeTagIdx != -1 {
				snippetVal = snippetVal[closeTagIdx+1:]
				snippetEndIdx := strings.Index(snippetVal, "</p>")
				if snippetEndIdx != -1 {
					snippetText = stripHTML(snippetVal[:snippetEndIdx])
				}
			}
		}

		items = append(items, SearchResponseItem{
			URL:     urlStr,
			Title:   htmlUnescape(strings.TrimSpace(titleText)),
			Snippet: htmlUnescape(strings.TrimSpace(snippetText)),
		})
	}
	return items
}

func parseDuckDuckGoHTML(html string) []SearchResponseItem {
	var items []SearchResponseItem
	parts := strings.Split(html, "class=\"result__a\"")
	for _, part := range parts[1:] {
		hrefIdx := strings.Index(part, "href=\"")
		if hrefIdx == -1 {
			continue
		}
		hrefVal := part[hrefIdx+6:]
		hrefCloseIdx := strings.Index(hrefVal, "\"")
		if hrefCloseIdx == -1 {
			continue
		}
		urlStr := hrefVal[:hrefCloseIdx]

		// Unescape URL if it's redirected through ddg, e.g. /l/?kh=-1&uddg=https%3A%2F%2F...
		if strings.Contains(urlStr, "uddg=") {
			if uIdx := strings.Index(urlStr, "uddg="); uIdx != -1 {
				decoded, err := url.QueryUnescape(urlStr[uIdx+5:])
				if err == nil {
					if ampIdx := strings.Index(decoded, "&"); ampIdx != -1 {
						decoded = decoded[:ampIdx]
					}
					urlStr = decoded
				}
			}
		}

		anchorCloseIdx := strings.Index(hrefVal, ">")
		if anchorCloseIdx == -1 {
			continue
		}
		titleVal := hrefVal[anchorCloseIdx+1:]
		titleEndIdx := strings.Index(titleVal, "</a>")
		if titleEndIdx == -1 {
			continue
		}
		titleText := stripHTML(titleVal[:titleEndIdx])

		snippetText := ""
		snippetIdx := strings.Index(part, "class=\"result__snippet\"")
		if snippetIdx != -1 {
			snippetVal := part[snippetIdx:]
			closeTagIdx := strings.Index(snippetVal, ">")
			if closeTagIdx != -1 {
				snippetVal = snippetVal[closeTagIdx+1:]
				snippetEndIdx := strings.Index(snippetVal, "</a>")
				if snippetEndIdx != -1 {
					snippetText = stripHTML(snippetVal[:snippetEndIdx])
				}
			}
		}

		items = append(items, SearchResponseItem{
			URL:     urlStr,
			Title:   htmlUnescape(strings.TrimSpace(titleText)),
			Snippet: htmlUnescape(strings.TrimSpace(snippetText)),
		})
	}
	return items
}

func stripHTML(s string) string {
	var builder strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
		} else if r == '>' {
			inTag = false
		} else if !inTag {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func htmlUnescape(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	return s
}
