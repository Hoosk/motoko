package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultReadLimit = 200

type ReadTool struct{}

func NewReadTool() *ReadTool {
	return &ReadTool{}
}

func (t *ReadTool) Spec() Spec {
	return Spec{
		Name:    "read",
		Summary: "Lee un archivo o lista un directorio del workspace.",
		Usage:   "read <ruta> [offset] [limit]",
	}
}

func (t *ReadTool) Run(ctx context.Context, args string) (Result, error) {
	_ = ctx
	parts := strings.Fields(args)
	if len(parts) == 0 {
		return Result{}, fmt.Errorf("uso: %s", t.Spec().Usage)
	}

	offset := 1
	limit := defaultReadLimit
	if len(parts) >= 2 {
		value, err := strconv.Atoi(parts[1])
		if err != nil || value < 1 {
			return Result{}, fmt.Errorf("offset invalido: %s", parts[1])
		}
		offset = value
	}
	if len(parts) >= 3 {
		value, err := strconv.Atoi(parts[2])
		if err != nil || value < 1 {
			return Result{}, fmt.Errorf("limit invalido: %s", parts[2])
		}
		limit = value
	}

	absPath, relPath, err := resolveWorkspacePath(parts[0])
	if err != nil {
		return Result{}, err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return Result{}, err
	}

	if info.IsDir() {
		entries, err := os.ReadDir(absPath)
		if err != nil {
			return Result{}, err
		}

		var lines []string
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() {
				name += "/"
			}
			lines = append(lines, name)
		}

		return Result{
			Spec:    t.Spec(),
			Summary: fmt.Sprintf("Directorio %s con %d entradas.", relPath, len(lines)),
			Output:  strings.Join(lines, "\n"),
		}, nil
	}

	file, err := os.Open(absPath)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	lineNo := 0
	var lines []string
	for scanner.Scan() {
		lineNo++
		if lineNo < offset {
			continue
		}
		if len(lines) >= limit {
			break
		}
		lines = append(lines, fmt.Sprintf("%d: %s", lineNo, scanner.Text()))
	}
	if err := scanner.Err(); err != nil {
		return Result{}, err
	}

	if len(lines) == 0 {
		return Result{Spec: t.Spec(), Summary: fmt.Sprintf("Sin contenido visible en %s desde la linea %d.", relPath, offset), Output: ""}, nil
	}

	return Result{
		Spec:    t.Spec(),
		Summary: fmt.Sprintf("Archivo %s leido desde linea %d (%d lineas).", filepath.ToSlash(relPath), offset, len(lines)),
		Output:  strings.Join(lines, "\n"),
	}, nil
}
