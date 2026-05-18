package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func trimLastRune(value string) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	return string(runes[:len(runes)-1])
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func estimateTextareaHeight(value string, width int) int {
	if width <= 1 {
		return 3
	}
	lines := strings.Split(value, "\n")
	count := 0
	for _, line := range lines {
		lineWidth := lipgloss.Width(line)
		if lineWidth == 0 {
			count++
			continue
		}
		count += (lineWidth-1)/width + 1
	}
	return max(3, count)
}

func stripANSI(value string) string {
	return ansiPattern.ReplaceAllString(value, "")
}
