package tools

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"
)

const maxGrepMatches = 200

type GrepTool struct{}

func NewGrepTool() *GrepTool {
	return &GrepTool{}
}

func (t *GrepTool) Spec() Spec {
	return Spec{
		Name:    "grep",
		Summary: "Search text by regex inside workspace files.",
		Usage:   "grep <regex> [include-glob]",
	}
}

func (t *GrepTool) Run(ctx context.Context, args string) (Result, error) {
	args = strings.TrimSpace(args)
	parts := strings.Fields(args)
	pattern := ""
	include := ""
	if parsed := parseJSONArgs(args); parsed != nil {
		pattern = jsonStr(parsed, "pattern", "regex", "query")
		include = jsonStr(parsed, "include", "glob", "file_pattern", "filePattern")
	} else {
		if len(parts) == 0 {
			return Result{}, fmt.Errorf("usage: %s", t.Spec().Usage)
		}
		pattern = parts[0]
		if len(parts) > 1 {
			include = parts[1]
		}
	}
	if pattern == "" {
		return Result{}, fmt.Errorf("usage: %s", t.Spec().Usage)
	}

	cfg := GetConfig(ctx)
	caseSensitive := false
	maxMatches := maxGrepMatches
	if cfg != nil {
		caseSensitive = cfg.Search.CaseSensitive
		if cfg.Search.MaxResults > 0 {
			maxMatches = cfg.Search.MaxResults
		}
	}

	if !caseSensitive && !strings.HasPrefix(pattern, "(?i)") {
		pattern = "(?i)" + pattern
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return Result{}, err
	}

	var includeMatcher *regexp.Regexp
	if include != "" {
		includeMatcher, err = compileGlob(include)
		if err != nil {
			return Result{}, err
		}
	}

	var matches []string
	err = walkWorkspace(ctx, func(relPath, absPath string, entry fs.DirEntry) error {
		if entry.IsDir() {
			return nil
		}
		if includeMatcher != nil && !includeMatcher.MatchString(relPath) {
			return nil
		}
		if !isTextFile(absPath) {
			return nil
		}

		file, openErr := os.Open(absPath)
		if openErr != nil {
			return nil
		}
		defer func() { _ = file.Close() }()

		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 1024), 1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if re.MatchString(line) {
				matches = append(matches, fmt.Sprintf("%s:%d: %s", relPath, lineNo, line))
				if len(matches) >= maxMatches {
					return errStopWalk
				}
			}
		}
		return nil
	})
	if err != nil && err != errStopWalk {
		return Result{}, err
	}

	sort.Strings(matches)
	if len(matches) == 0 {
		return Result{Spec: t.Spec(), Summary: fmt.Sprintf("No matches for %s.", pattern), Output: ""}, nil
	}

	summary := fmt.Sprintf("%d matches for %s.", len(matches), pattern)
	if include != "" {
		summary = fmt.Sprintf("%d matches for %s in %s.", len(matches), pattern, include)
	}

	return Result{Spec: t.Spec(), Summary: summary, Output: strings.Join(matches, "\n")}, nil
}

var errStopWalk = fmt.Errorf("stop walk")
