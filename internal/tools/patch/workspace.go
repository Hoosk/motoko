package patch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolveWorkspacePath(target string) (string, string, error) {
	workspace, err := os.Getwd()
	if err != nil {
		return "", "", err
	}

	if target == "" {
		return workspace, ".", nil
	}

	path := target
	if !filepath.IsAbs(path) {
		path = filepath.Join(workspace, path)
	}
	path = filepath.Clean(path)

	rel, err := filepath.Rel(workspace, path)
	if err != nil {
		return "", "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("ruta fuera del workspace: %s", target)
	}
	if rel == "." {
		return path, rel, nil
	}

	return path, filepath.ToSlash(rel), nil
}
