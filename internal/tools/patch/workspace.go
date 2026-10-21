package patch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var ErrFileChanged = errors.New("file changed while awaiting approval")

var workspaceWriteMu sync.Mutex

// ResolveWorkspacePath resolves target (relative to the working directory, or
// absolute) and rejects anything outside the workspace. Symlinks are resolved
// first, because reads and writes follow them: a link inside the workspace
// pointing at ~/.ssh or ~/.zshrc is rejected. The returned absolute path and
// workspace-relative path refer to the real file.
func ResolveWorkspacePath(target string) (string, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	workspace, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", "", err
	}

	if target == "" {
		return workspace, ".", nil
	}

	path := target
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	path = filepath.Clean(path)
	if _, outsideErr := relativeInside(cwd, path); outsideErr != nil {
		return "", "", fmt.Errorf("path outside workspace: %s", target)
	}

	real, err := evalExistingSymlinks(path)
	if err != nil {
		return "", "", err
	}
	rel, err := relativeInside(workspace, real)
	if err != nil {
		return "", "", fmt.Errorf("path resolves outside workspace through a symlink: %s", target)
	}
	return real, rel, nil
}

func resolveWorkspacePath(target string) (string, string, error) {
	return ResolveWorkspacePath(target)
}

// relativeInside returns path relative to root (slash-separated, "." for
// root itself), or an error if path is not inside root.
func relativeInside(root, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s is outside %s", path, root)
	}
	return filepath.ToSlash(rel), nil
}

// evalExistingSymlinks resolves symlinks in the longest existing prefix of
// path and re-appends the components that do not exist yet, so it also works
// for files about to be created. A dangling symlink is an error: writing
// through it would create its (unchecked) target.
func evalExistingSymlinks(path string) (string, error) {
	missing := ""
	current := path
	for {
		real, err := filepath.EvalSymlinks(current)
		if err == nil {
			return filepath.Join(real, missing), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if info, lerr := os.Lstat(current); lerr == nil && info.Mode()&fs.ModeSymlink != 0 {
			return "", fmt.Errorf("dangling symlink: %s", current)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return path, nil
		}
		missing = filepath.Join(filepath.Base(current), missing)
		current = parent
	}
}

// ValidateWritePath checks if the given target path is allowed for writes.
func ValidateWritePath(target string) error {
	_, relPath, err := resolveWorkspacePath(target)
	if err != nil {
		return err
	}

	lowerRel := strings.ToLower(relPath)

	// Block any write to .git directory (hooks, config, objects, etc.)
	if lowerRel == ".git" || strings.HasPrefix(lowerRel, ".git/") {
		return fmt.Errorf("write blocked: path inside git infrastructure (.git) not allowed")
	}

	// Block any environment configuration files (.env, .env.local, .env.development, etc.)
	baseName := strings.ToLower(filepath.Base(relPath))
	if strings.HasPrefix(baseName, ".env") {
		return fmt.Errorf("write blocked: modification of environment variable files (.env) not allowed")
	}

	// Block SSH directories or private keys
	if lowerRel == ".ssh" || strings.HasPrefix(lowerRel, ".ssh/") ||
		strings.HasPrefix(baseName, "id_rsa") || strings.HasPrefix(baseName, "id_dsa") ||
		strings.HasPrefix(baseName, "id_ecdsa") || strings.HasPrefix(baseName, "id_ed25519") ||
		baseName == "authorized_keys" || baseName == "known_hosts" {
		return fmt.Errorf("write blocked: modification of SSH keys or system credentials not allowed")
	}

	return nil
}

// resolveWorkspaceWritePath resolves target and ensures it is a safe write path.
func resolveWorkspaceWritePath(target string) (string, string, error) {
	if err := ValidateWritePath(target); err != nil {
		return "", "", err
	}
	return resolveWorkspacePath(target)
}

// ResolveWorkspaceWritePath is the exported wrapper used by other tools
// (e.g. the write tool) that need the same validation as the patch engine.
func ResolveWorkspaceWritePath(target string) (string, string, error) {
	return resolveWorkspaceWritePath(target)
}

func WriteWorkspaceFile(ctx context.Context, absPath string, expected, updated []byte, existed bool, dirPerm, filePerm os.FileMode) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	workspaceWriteMu.Lock()
	defer workspaceWriteMu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := os.ReadFile(absPath)
	if existed {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s was removed", ErrFileChanged, absPath)
		}
		if err != nil {
			return err
		}
		if !bytes.Equal(current, expected) {
			return fmt.Errorf("%w: %s", ErrFileChanged, absPath)
		}
	} else {
		if err == nil {
			return fmt.Errorf("%w: %s was created", ErrFileChanged, absPath)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), dirPerm); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Paths are validated and resolved by ValidateWritePath before they reach
	// this point; writing agent-directed files is the tool's purpose.
	return os.WriteFile(absPath, updated, filePerm) // #nosec G703
}
