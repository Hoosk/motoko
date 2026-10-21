package semantic

import (
	"bytes"
	"maps"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

func RelevantFiles(s Snapshot, prompt string, limit int) []FileSummary {
	return relevantFilesForTokens(s, promptTokens(prompt), limit)
}

func RelevantSnippets(s Snapshot, prompt string, fileLimit, lineBudget int) []Snippet {
	if lineBudget <= 0 {
		lineBudget = defaultSnippetBudget
	}
	tokens := promptTokens(prompt)
	files := relevantFilesForTokens(s, tokens, fileLimit)
	result := make([]Snippet, 0, len(files))
	remaining := lineBudget
	for _, file := range files {
		if remaining < 12 {
			break
		}
		snippet, ok := bestSnippetForFile(file, tokens, remaining)
		if !ok {
			continue
		}
		result = append(result, snippet)
		remaining -= snippet.EndLine - snippet.StartLine + 1
	}
	return result
}

func relevantFilesForTokens(s Snapshot, tokens []string, limit int) []FileSummary {
	if limit <= 0 {
		limit = defaultTopFiles
	}

	// 1. Calculate initial scores
	scores := make(map[string]int)
	for _, file := range s.Files {
		scores[file.Path] = scoreFile(file, tokens)
	}

	// 2. Semantic graph score propagation (expanded scores)
	expanded := make(map[string]int)
	maps.Copy(expanded, scores)

	// Only the strongest direct matches propagate. Short prompt tokens match
	// most files, so propagating from every positive score made this
	// quadratic in the size of the workspace.
	var seeds []scoredFile
	for _, file := range s.Files {
		if score := scores[file.Path]; score > 0 {
			seeds = append(seeds, scoredFile{file: file, score: score})
		}
	}
	sortScoredFiles(seeds)
	if len(seeds) > maxPropagationSeeds {
		seeds = seeds[:maxPropagationSeeds]
	}
	for _, seed := range seeds {
		a := seed.file
		for _, b := range s.Files {
			if a.Path == b.Path {
				continue
			}
			if areConnected(a, b) {
				// Propagate 30% of A's score to B
				expanded[b.Path] += seed.score * 3 / 10
			}
		}
	}

	// 3. Rank files based on final expanded scores
	var ranked []scoredFile
	for _, file := range s.Files {
		score := expanded[file.Path]
		if score > 0 {
			ranked = append(ranked, scoredFile{file: file, score: score})
		}
	}

	sortScoredFiles(ranked)
	result := make([]FileSummary, 0, limit)
	seen := make(map[string]struct{})
	for _, item := range ranked {
		if len(result) >= limit {
			break
		}
		seen[item.file.Path] = struct{}{}
		result = append(result, item.file)
	}
	if len(result) < limit {
		fallbacks := fallbackFiles(s, limit-len(result))
		for _, file := range fallbacks {
			if _, ok := seen[file.Path]; ok {
				continue
			}
			result = append(result, file)
			if len(result) >= limit {
				break
			}
		}
	}
	return result
}

func fallbackFiles(s Snapshot, limit int) []FileSummary {
	if limit <= 0 {
		return nil
	}
	var ranked []scoredFile
	for _, file := range s.Files {
		score := 0
		if file.Changed {
			score += 100
		}
		score += len(file.Symbols)
		lowerPath := strings.ToLower(file.Path)
		if strings.Contains(lowerPath, "/app/") || strings.Contains(lowerPath, "/ui/") || strings.Contains(lowerPath, "main") || strings.Contains(lowerPath, "runtime") || strings.Contains(lowerPath, "model") {
			score += 8
		}
		if score > 0 {
			ranked = append(ranked, scoredFile{file: file, score: score})
		}
	}
	sortScoredFiles(ranked)
	result := make([]FileSummary, 0, limit)
	for _, item := range ranked {
		result = append(result, item.file)
		if len(result) >= limit {
			break
		}
	}
	return result
}

func sortScoredFiles(ranked []scoredFile) {
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].file.Path < ranked[j].file.Path
		}
		return ranked[i].score > ranked[j].score
	})
}

func scoreFile(file FileSummary, tokens []string) int {
	score := 0
	lowerPath := strings.ToLower(file.Path)
	for _, token := range tokens {
		if token == "" {
			continue
		}
		if strings.Contains(lowerPath, token) {
			score += 10
		}
		for _, symbol := range file.Symbols {
			if strings.ToLower(symbol.Name) == token {
				score += 25
			} else if strings.Contains(strings.ToLower(symbol.Name), token) {
				score += 15
			}
		}
	}
	if file.Changed {
		score += 30
	}
	return score
}

func bestSnippetForFile(file FileSummary, tokens []string, budget int) (Snippet, bool) {
	if len(file.Content) == 0 {
		return Snippet{}, false
	}
	lines := splitLines(file.Content)
	if len(lines) == 0 {
		return Snippet{}, false
	}
	bestSymbol, reason := chooseSymbol(file, tokens)
	var start, end int
	if bestSymbol.Name != "" {
		start, end = boundedRange(bestSymbol.Range.Start, bestSymbol.Range.End, len(lines), budget)
	} else {
		start, end = boundedRange(1, min(len(lines), 20), len(lines), budget)
		reason = "fallback top of file"
	}
	if start <= 0 || end > len(lines) || start > end {
		return Snippet{}, false
	}
	content := strings.Join(lines[start-1:end], "\n")
	return Snippet{
		Path:      file.Path,
		Language:  file.Language,
		Reason:    reason,
		StartLine: start,
		EndLine:   end,
		Content:   content,
	}, true
}

func chooseSymbol(file FileSummary, tokens []string) (Symbol, string) {
	bestScore := -1
	var best Symbol
	reason := "fallback top-level symbol"
	for _, symbol := range file.Symbols {
		score := 0
		name := strings.ToLower(symbol.Name)
		for _, token := range tokens {
			if token == "" {
				continue
			}
			if name == token {
				score += 20
			}
			if strings.Contains(name, token) || strings.Contains(token, name) {
				score += 12
			}
		}
		if file.Changed {
			score += 5
		}
		if score > bestScore {
			bestScore = score
			best = symbol
			if score > 0 {
				reason = "symbol match: " + symbol.Name
			}
		}
	}
	if bestScore < 0 || best.Name == "" {
		return Symbol{}, ""
	}
	return best, reason
}

func splitLines(content []byte) []string {
	s := string(content)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(s, "\n")
}

func boundedRange(start, end, totalLines, budget int) (int, int) {
	if budget <= 0 {
		budget = 20
	}
	if budget > maxSnippetLines {
		budget = maxSnippetLines
	}
	lines := end - start + 1
	if lines > budget {
		return start, start + budget - 1
	}
	needed := budget - lines
	before := needed / 2
	after := needed - before
	newStart := max(1, start-before)
	newEnd := min(totalLines, end+after)
	return newStart, newEnd
}

func promptTokens(prompt string) []string {
	var tokens []string
	var current strings.Builder
	for _, r := range prompt {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			current.WriteRune(unicode.ToLower(r))
		} else {
			if current.Len() > 2 {
				tokens = append(tokens, current.String())
			}
			current.Reset()
		}
	}
	if current.Len() > 2 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

func isImportedBy(b, a FileSummary) bool {
	for _, imp := range a.Imports {
		impClean := strings.Trim(imp, "\"`' ")
		if impClean == "" {
			continue
		}
		// If B's path contains the import suffix or vice-versa
		lowerB := strings.ToLower(b.Path)
		lowerImp := strings.ToLower(impClean)
		if strings.Contains(lowerB, lowerImp) || strings.Contains(lowerImp, strings.ToLower(filepath.Base(b.Path))) {
			return true
		}
	}
	return false
}

func referencesSymbol(a, b FileSummary) bool {
	if len(b.Exports) == 0 || len(a.Content) == 0 {
		return false
	}
	// Identifiers are case-sensitive, so match them as-is instead of
	// lowercasing a copy of the whole file for every pair.
	for _, exp := range b.Exports {
		if exp == "" {
			continue
		}
		if bytes.Contains(a.Content, []byte(exp)) {
			return true
		}
	}
	return false
}

func areConnected(a, b FileSummary) bool {
	if a.Path == b.Path {
		return false
	}
	if isImportedBy(b, a) {
		return true
	}
	if isImportedBy(a, b) {
		return true
	}
	if referencesSymbol(a, b) {
		return true
	}
	if referencesSymbol(b, a) {
		return true
	}
	return false
}
