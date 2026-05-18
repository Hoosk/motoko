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
		Summary: "Busca texto por regex dentro de archivos del workspace.",
		Usage:   "grep <regex> [include-glob]",
	}
}

func (t *GrepTool) Run(ctx context.Context, args string) (Result, error) {
	_ = ctx
	parts := strings.Fields(args)
	if len(parts) == 0 {
		return Result{}, fmt.Errorf("uso: %s", t.Spec().Usage)
	}

	pattern := parts[0]
	include := ""
	if len(parts) > 1 {
		include = parts[1]
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
	err = walkWorkspace(func(relPath, absPath string, entry fs.DirEntry) error {
		if entry.IsDir() {
			return nil
		}
		if includeMatcher != nil && !includeMatcher.MatchString(relPath) {
			return nil
		}
		if !isTextFile(absPath) {
			return nil
		}

		file, err := os.Open(absPath)
		if err != nil {
			return nil
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 1024), 1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if re.MatchString(line) {
				matches = append(matches, fmt.Sprintf("%s:%d: %s", relPath, lineNo, line))
				if len(matches) >= maxGrepMatches {
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
		return Result{Spec: t.Spec(), Summary: fmt.Sprintf("Sin coincidencias para %s.", pattern), Output: ""}, nil
	}

	summary := fmt.Sprintf("%d coincidencias para %s.", len(matches), pattern)
	if include != "" {
		summary = fmt.Sprintf("%d coincidencias para %s en %s.", len(matches), pattern, include)
	}

	return Result{Spec: t.Spec(), Summary: summary, Output: strings.Join(matches, "\n")}, nil
}

var errStopWalk = fmt.Errorf("stop walk")
