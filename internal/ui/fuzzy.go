package ui

import "strings"

const noFuzzyMatch = -1

type fuzzyMatch struct {
	Positions []int
	Score     int
}

func scoreFuzzy(query, target string) fuzzyMatch {
	query = strings.TrimSpace(strings.ToLower(query))
	target = strings.ToLower(target)
	if query == "" {
		return fuzzyMatch{Score: 0}
	}

	queryRunes := []rune(query)
	targetRunes := []rune(target)
	positions := make([]int, 0, len(queryRunes))

	searchStart := 0
	for _, want := range queryRunes {
		found := false
		for i := searchStart; i < len(targetRunes); i++ {
			if targetRunes[i] == want {
				positions = append(positions, i)
				searchStart = i + 1
				found = true
				break
			}
		}
		if !found {
			return fuzzyMatch{Score: noFuzzyMatch}
		}
	}

	score := 100 - (positions[len(positions)-1] - positions[0])
	for i, pos := range positions {
		score += 10
		if pos == 0 || isWordBoundary(targetRunes, pos) {
			score += 20
		}
		if i > 0 && pos == positions[i-1]+1 {
			score += 25
		}
	}

	return fuzzyMatch{Positions: positions, Score: score}
}

func isWordBoundary(runes []rune, index int) bool {
	if index <= 0 || index >= len(runes) {
		return index == 0
	}
	prev := runes[index-1]
	return prev == ' ' || prev == '-' || prev == '_' || prev == '/' || prev == '['
}
