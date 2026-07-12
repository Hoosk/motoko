package sessiontitle

import (
	"encoding/json"
	"strings"

	"github.com/Hoosk/motoko/internal/provider"
)

func FromModelResponse(resp provider.Response) string {
	raw := strings.TrimSpace(resp.FinalText)
	if raw == "" {
		return ""
	}
	if message := ExtractStructuredMessage(raw); strings.TrimSpace(message) != "" {
		return Sanitize(message)
	}
	return Sanitize(raw)
}

func ExtractStructuredMessage(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if after, ok := strings.CutPrefix(raw, "```"); ok {
		raw = after
		raw = strings.TrimSpace(raw)
		if strings.HasPrefix(strings.ToLower(raw), "json") {
			raw = strings.TrimSpace(raw[4:])
		}
		if end := strings.LastIndex(raw, "```"); end >= 0 {
			raw = strings.TrimSpace(raw[:end])
		}
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end < start {
		return ""
	}
	var parsed struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(raw[start:end+1]), &parsed); err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Message)
}

func Sanitize(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	best := ""
	for _, line := range lines {
		candidate := normalizeLine(line)
		if !looksLikeTitle(candidate) {
			continue
		}
		best = candidate
	}
	if best != "" {
		return best
	}
	return normalizeLine(lines[0])
}

func normalizeLine(line string) string {
	line = strings.TrimSpace(line)
	for _, prefix := range []string{"* ", "- ", "+ "} {
		line = strings.TrimSpace(strings.TrimPrefix(line, prefix))
	}
	line = strings.ReplaceAll(line, "**", "")
	line = strings.ReplaceAll(line, "*", "")
	line = strings.Trim(line, " \t\"'")
	if idx := strings.Index(line, ":"); idx > 0 {
		prefix := strings.ToLower(strings.TrimSpace(line[:idx]))
		switch {
		case strings.HasPrefix(prefix, "option "), strings.HasPrefix(prefix, "opcion "), prefix == "title", prefix == "titulo", prefix == "título", strings.HasPrefix(prefix, "constraint"):
			line = strings.TrimSpace(line[idx+1:])
		}
	}
	line = strings.Trim(line, " \t\"'.,:;()[]{}")
	return strings.Join(strings.Fields(line), " ")
}

func looksLikeTitle(line string) bool {
	if line == "" {
		return false
	}
	lower := strings.ToLower(line)
	for _, rejected := range []string{
		"the user wants a title",
		"session is just starting",
		"constraint check",
		"4 a 8 palabras",
		"4-8 palabras",
		"devuelve solo",
		"sin comillas",
		"sin markdown",
		"option ",
		"opcion ",
	} {
		if strings.Contains(lower, rejected) {
			return false
		}
	}
	words := len(strings.Fields(line))
	return words >= 2 && words <= 8
}
