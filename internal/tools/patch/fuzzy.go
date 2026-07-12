package patch

import (
	"fmt"
	"strings"
)

func fuzzyReplace(current, search, replace string) (string, error) {
	if search == "" {
		return "", fmt.Errorf("empty SEARCH block is only allowed for creating new files")
	}

	matches := countOverlappingMatches(current, search)
	if matches == 1 {
		if err := validateExactSearchBlock(search); err != nil {
			return "", err
		}
		return strings.Replace(current, search, replace, 1), nil
	}

	if matches > 1 {
		return "", fmt.Errorf("the SEARCH block appears %d times exactly; the substitution must be unique. Provide more context", matches)
	}

	if err := validateFuzzySearchBlock(search); err != nil {
		return "", err
	}

	type lineInfo struct {
		text  string
		start int
		end   int
	}
	var lines []lineInfo
	for i := 0; i < len(current); {
		nl := strings.IndexByte(current[i:], '\n')
		var end int
		if nl == -1 {
			end = len(current)
		} else {
			end = i + nl + 1
		}
		lines = append(lines, lineInfo{start: i, end: end, text: current[i:end]})
		i = end
	}

	searchLines := strings.Split(strings.TrimSpace(search), "\n")
	if len(searchLines) == 0 {
		return "", fmt.Errorf("SEARCH block not found (empty after trimming whitespace)")
	}

	var matchIndices []int
	for i := 0; i <= len(lines)-len(searchLines); i++ {
		match := true
		for j := range searchLines {
			if strings.TrimSpace(lines[i+j].text) != strings.TrimSpace(searchLines[j]) {
				match = false
				break
			}
		}
		if match {
			matchIndices = append(matchIndices, i)
		}
	}

	if len(matchIndices) == 0 {
		return "", fmt.Errorf("could not find the SEARCH block even when ignoring whitespace and indentation. Please verify the file's current content")
	}
	if len(matchIndices) > 1 {
		return "", fmt.Errorf("the SEARCH block matches in %d places when ignoring whitespace; it must be unique. Please provide more context lines", len(matchIndices))
	}

	startLine := matchIndices[0]
	endLine := startLine + len(searchLines) - 1

	originalStart := lines[startLine].start
	originalEnd := lines[endLine].end

	updated := current[:originalStart] + replace
	if len(replace) > 0 && replace[len(replace)-1] != '\n' && originalEnd < len(current) && current[originalEnd-1] == '\n' {
		updated += "\n"
	}
	updated += current[originalEnd:]

	return updated, nil
}

func validateFuzzySearchBlock(search string) error {
	trimmed := strings.TrimSpace(search)
	if trimmed == "" {
		return fmt.Errorf("the SEARCH block cannot be empty after trimming whitespace")
	}
	lines := strings.Split(trimmed, "\n")
	meaningfulLines := 0
	nonWhitespace := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		meaningfulLines++
		for _, r := range line {
			if !strings.ContainsRune("{}[]()", r) {
				nonWhitespace++
			}
		}
	}
	if meaningfulLines < 2 || nonWhitespace < 3 {
		return fmt.Errorf("the SEARCH block is too ambiguous for fuzzy replace; provide more unique context lines")
	}
	return nil
}

func validateExactSearchBlock(search string) error {
	trimmed := strings.TrimSpace(search)
	if trimmed == "" {
		return fmt.Errorf("the SEARCH block cannot be empty after trimming whitespace")
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) >= 2 {
		return nil
	}
	line := strings.TrimSpace(lines[0])
	if line == "" {
		return fmt.Errorf("the SEARCH block cannot be empty after trimming whitespace")
	}
	nonPunctuation := 0
	for _, r := range line {
		if strings.ContainsRune("{}[]()<>;:,.", r) {
			continue
		}
		if r == ' ' || r == '\t' {
			continue
		}
		nonPunctuation++
	}
	if nonPunctuation < 3 {
		return fmt.Errorf("the SEARCH block is too ambiguous for fuzzy replace; provide more unique context lines")
	}
	return nil
}

func countOverlappingMatches(current, search string) int {
	if search == "" {
		return 0
	}
	count := 0
	for start := 0; start <= len(current)-len(search); {
		idx := strings.Index(current[start:], search)
		if idx == -1 {
			break
		}
		count++
		start += idx + 1
	}
	return count
}
