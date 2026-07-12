package tools

import (
	"strings"
)

// levenshtein calculates the Levenshtein distance between two strings.
func levenshtein(s1, s2 string) int {
	r1, r2 := []rune(s1), []rune(s2)
	n, m := len(r1), len(r2)
	if n == 0 {
		return m
	}
	if m == 0 {
		return n
	}

	d := make([][]int, n+1)
	for i := range d {
		d[i] = make([]int, m+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}

	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			cost := 1
			if r1[i-1] == r2[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, min(d[i][j-1]+1, d[i-1][j-1]+cost))
		}
	}
	return d[n][m]
}

// RepairToolName attempts to find a matching tool name if the LLM makes a typo.
// If the distance is small enough (<= 2 for short words, <= 3 for longer), it returns the corrected name.
// Returns an empty string if no good match is found.
func RepairToolName(requested string, available []string) string {
	requested = strings.ToLower(strings.TrimSpace(requested))
	for _, a := range available {
		if strings.ToLower(a) == requested {
			return a
		}
	}

	var bestMatch string
	bestDist := 999

	for _, a := range available {
		dist := levenshtein(requested, strings.ToLower(a))
		if dist < bestDist {
			bestDist = dist
			bestMatch = a
		}
	}

	threshold := 2
	if len(requested) > 6 {
		threshold = 3
	}

	if bestDist <= threshold {
		return bestMatch
	}
	return ""
}
