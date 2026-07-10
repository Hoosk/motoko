package tachikoma

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/semantic"
)

type DiffTachikoma struct {
	index    *semantic.Index
	interval time.Duration
}

type SemanticDiff struct {
	Files map[string][]SymbolChange
}

type SymbolChange struct {
	Name string
	Kind string
	Type string // "modified", "added", "removed"
}

func NewDiffTachikoma(index *semantic.Index, interval time.Duration) *DiffTachikoma {
	return &DiffTachikoma{index: index, interval: interval}
}

func (d *DiffTachikoma) Name() string {
	return "DiffTachikoma"
}

func (d *DiffTachikoma) Run(ctx context.Context, publish func(Update) bool) error {
	refresh := func() {
		diff, err := d.computeSemanticDiff(ctx)
		var status string
		var payload any
		if err != nil {
			status = "error: " + err.Error()
		} else if len(diff.Files) > 0 {
			count := 0
			for _, changes := range diff.Files {
				count += len(changes)
			}
			status = fmt.Sprintf("detected changes in %d symbols across %d files", count, len(diff.Files))
			payload = diff
		} else {
			status = "no recent semantic changes"
		}
		publish(Update{Name: d.Name(), Status: status, Payload: payload})
	}

	return runRefreshLoop(ctx, d.interval, []string{"."}, time.Second, refresh)
}

func (d *DiffTachikoma) computeSemanticDiff(ctx context.Context) (SemanticDiff, error) {
	result := SemanticDiff{Files: make(map[string][]SymbolChange)}

	snapshot := d.index.LatestSnapshot()
	if snapshot == nil {
		return result, nil
	}

	// Get abbreviated diff to identify changed lines
	cmd := exec.CommandContext(ctx, "git", "diff", "-U0")
	out, err := cmd.Output()
	if err != nil {
		return result, err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	var currentFile string

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "+++ b/") {
			currentFile = line[6:]
			continue
		}
		if strings.HasPrefix(line, "@@ ") && currentFile != "" {
			// Format: @@ -line,count +line,count @@
			parts := strings.Split(line, " ")
			if len(parts) < 3 {
				continue
			}
			newRange := parts[2] // +line,count
			newRange = strings.TrimPrefix(newRange, "+")

			startLine := 0
			rangeParts := strings.Split(newRange, ",")
			startLine, _ = strconv.Atoi(rangeParts[0])
			count := 1
			if len(rangeParts) > 1 {
				count, _ = strconv.Atoi(rangeParts[1])
			}

			// Find symbols in this range
			d.identifyChangesInFile(snapshot, currentFile, startLine, count, &result)
		}
	}

	return result, nil
}

func (d *DiffTachikoma) identifyChangesInFile(snapshot *semantic.Snapshot, path string, start, count int, result *SemanticDiff) {
	var file *semantic.FileSummary
	for i := range snapshot.Files {
		if snapshot.Files[i].Path == path {
			file = &snapshot.Files[i]
			break
		}
	}

	if file == nil {
		return
	}

	seen := make(map[string]bool)
	for i := 0; i < count || (count == 0 && i == 0); i++ {
		line := start + i
		symbol := file.SymbolAtLine(line)
		if symbol != nil && !seen[symbol.Name] {
			seen[symbol.Name] = true
			result.Files[path] = append(result.Files[path], SymbolChange{
				Name: symbol.Name,
				Kind: symbol.Kind,
				Type: "modified",
			})
		}
	}
}
