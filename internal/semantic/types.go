package semantic

import (
	"github.com/Hoosk/motoko/internal/semantic/symtypes"
)

type LineRange = symtypes.LineRange
type Symbol = symtypes.Symbol
type Snippet = symtypes.Snippet
type FileSummary = symtypes.FileSummary

const defaultTopFiles = 6

type Snapshot struct {
	symtypes.Snapshot
}

func (s Snapshot) RelevantFiles(prompt string, limit int) []FileSummary {
	return relevantFilesForTokens(s, promptTokens(prompt), limit)
}

func (s Snapshot) RelevantSnippets(prompt string, fileLimit, lineBudget int) []Snippet {
	return RelevantSnippets(s, prompt, fileLimit, lineBudget)
}

type scoredFile struct {
	file  FileSummary
	score int
}
