package pathpolicy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resolution describes both the path requested inside the workspace and the
// canonical destination reached after following symbolic links.
type Resolution struct {
	Path       string
	Requested  string
	Relative   string
	External   bool
	existing   bool
	info       os.FileInfo
	anchor     string
	anchorInfo os.FileInfo
}

func (r Resolution) Existing() bool { return r.existing }

// Resolve accepts only paths lexically inside the workspace and resolves the
// nearest existing ancestor so new files below symlinked directories are safe.
func Resolve(target string) (Resolution, error) {
	workspace, err := os.Getwd()
	if err != nil {
		return Resolution{}, err
	}
	workspace = filepath.Clean(workspace)

	requested := target
	if requested == "" {
		requested = workspace
	} else if !filepath.IsAbs(requested) {
		requested = filepath.Join(workspace, requested)
	}
	requested = filepath.Clean(requested)

	rel, err := filepath.Rel(workspace, requested)
	if err != nil {
		return Resolution{}, err
	}
	if isOutside(rel) {
		return Resolution{}, fmt.Errorf("path outside workspace: %s", target)
	}

	realWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return Resolution{}, fmt.Errorf("resolve workspace: %w", err)
	}
	realPath, info, existing, anchor, anchorInfo, err := resolveExistingAncestor(requested)
	if err != nil {
		return Resolution{}, fmt.Errorf("resolve path %s: %w", target, err)
	}
	realRel, err := filepath.Rel(realWorkspace, realPath)
	if err != nil {
		return Resolution{}, err
	}

	return Resolution{
		Path:       realPath,
		Requested:  requested,
		Relative:   filepath.ToSlash(rel),
		External:   isOutside(realRel),
		existing:   existing,
		info:       info,
		anchor:     anchor,
		anchorInfo: anchorInfo,
	}, nil
}

func resolveExistingAncestor(path string) (string, os.FileInfo, bool, string, os.FileInfo, error) {
	candidate := path
	var missing []string
	for {
		_, err := os.Lstat(candidate)
		if err == nil {
			resolved, resolveErr := filepath.EvalSymlinks(candidate)
			if resolveErr != nil {
				return "", nil, false, "", nil, resolveErr
			}
			anchorInfo, statErr := os.Stat(resolved)
			if statErr != nil {
				return "", nil, false, "", nil, statErr
			}
			anchor := filepath.Clean(resolved)
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			resolved = filepath.Clean(resolved)
			if len(missing) > 0 {
				return resolved, nil, false, anchor, anchorInfo, nil
			}
			return resolved, anchorInfo, true, anchor, anchorInfo, nil
		}
		if !os.IsNotExist(err) {
			return "", nil, false, "", nil, err
		}

		parent := filepath.Dir(candidate)
		if parent == candidate {
			return "", nil, false, "", nil, err
		}
		missing = append(missing, filepath.Base(candidate))
		candidate = parent
	}
}

func isOutside(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel)
}
