package symtypes

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type LineRange struct {
	Start int
	End   int
}

type Symbol struct {
	Name  string
	Kind  string
	Line  int
	Range LineRange
}

type Snippet struct {
	Path      string
	Language  string
	Reason    string
	Content   string
	StartLine int
	EndLine   int
}

func (s Snippet) Descriptor() string {
	return fmt.Sprintf("FILE %s\nLINES %d-%d\nREASON %s\n%s", s.Path, s.StartLine, s.EndLine, s.Reason, s.Content)
}

type FileSummary struct {
	Path     string
	Language string
	Symbols  []Symbol
	Imports  []string
	Exports  []string
	Content  []byte
	Lines    int
	Changed  bool
}

func (f FileSummary) Descriptor() string {
	parts := []string{fmt.Sprintf("%s [%s]", f.Path, f.Language)}
	if f.Changed {
		parts = append(parts, "changed")
	}
	if len(f.Symbols) > 0 {
		names := make([]string, 0, min(6, len(f.Symbols)))
		for i := 0; i < len(f.Symbols) && i < 6; i++ {
			names = append(names, f.Symbols[i].Name)
		}
		parts = append(parts, "symbols: "+strings.Join(names, ", "))
	}
	return strings.Join(parts, " | ")
}

// SymbolAtLine returns the symbol that contains the given 1-based line number.
func (f FileSummary) SymbolAtLine(line int) *Symbol {
	var best *Symbol
	for i := range f.Symbols {
		s := &f.Symbols[i]
		if line >= s.Range.Start && line <= s.Range.End {
			// If we find nested symbols, we want the most specific one (smallest range)
			if best == nil || (s.Range.End-s.Range.Start < best.Range.End-best.Range.Start) {
				best = s
			}
		}
	}
	return best
}

type Snapshot struct {
	GeneratedAt    time.Time
	Root           string
	Directories    []string
	LanguageCounts map[string]int
	ChangedPaths   []string
	Files          []FileSummary
}

func (s Snapshot) Empty() bool {
	return len(s.Files) == 0
}

func (s Snapshot) Summary() string {
	if len(s.Files) == 0 {
		return "indice semantico vacio"
	}
	parts := []string{fmt.Sprintf("files:%d", len(s.Files))}
	if len(s.Directories) > 0 {
		dirs := s.Directories
		if len(dirs) > 6 {
			dirs = dirs[:6]
		}
		parts = append(parts, "dirs:"+strings.Join(dirs, ", "))
	}
	if len(s.LanguageCounts) > 0 {
		langs := make([]string, 0, len(s.LanguageCounts))
		for lang, count := range s.LanguageCounts {
			langs = append(langs, fmt.Sprintf("%s:%d", lang, count))
		}
		sort.Strings(langs)
		parts = append(parts, "langs:"+strings.Join(langs, ", "))
	}
	if len(s.ChangedPaths) > 0 {
		changed := s.ChangedPaths
		if len(changed) > 5 {
			changed = changed[:5]
		}
		parts = append(parts, "changed:"+strings.Join(changed, ", "))
	}
	return strings.Join(parts, " | ")
}
