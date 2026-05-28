package timeline

import (
	"regexp"
	"strings"

	"github.com/mattn/go-runewidth"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func StripANSI(value string) string {
	return ansiPattern.ReplaceAllString(value, "")
}

func WrapText(text string, width int) string {
	if width <= 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		result = append(result, WrapOneLine(line, width))
	}
	return strings.Join(result, "\n")
}

func WrapOneLine(line string, width int) string {
	if runewidth.StringWidth(line) <= width {
		return line
	}
	runes := []rune(line)
	var out strings.Builder
	col := 0

	for i := 0; i < len(runes); {
		r := runes[i]
		if r == ' ' || r == '\t' {
			rw := runewidth.RuneWidth(r)
			if col > 0 && col+rw <= width {
				out.WriteRune(r)
				col += rw
			}
			i++
			continue
		}
		j, wordW := i, 0
		for j < len(runes) && runes[j] != ' ' && runes[j] != '\t' {
			wordW += runewidth.RuneWidth(runes[j])
			j++
		}
		word := runes[i:j]
		switch {
		case col == 0:
			for _, wr := range word {
				wrW := runewidth.RuneWidth(wr)
				if col+wrW > width && col > 0 {
					out.WriteByte('\n')
					col = 0
				}
				out.WriteRune(wr)
				col += wrW
			}
		case col+wordW <= width:
			out.WriteString(string(word))
			col += wordW
		default:
			out.WriteByte('\n')
			col = 0
			for _, wr := range word {
				wrW := runewidth.RuneWidth(wr)
				if col+wrW > width && col > 0 {
					out.WriteByte('\n')
					col = 0
				}
				out.WriteRune(wr)
				col += wrW
			}
		}
		i = j
	}
	return out.String()
}
