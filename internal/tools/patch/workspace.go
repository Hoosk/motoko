package patch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var ErrFileChanged = errors.New("file changed while awaiting approval")

var workspaceWriteMu sync.Mutex

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
		return "", "", fmt.Errorf("path outside workspace: %s", target)
	}
	if rel == "." {
		return path, rel, nil
	}

	return path, filepath.ToSlash(rel), nil
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

	// Block Motoko agent settings
	if lowerRel == ".antigravitycli" || strings.HasPrefix(lowerRel, ".antigravitycli/") {
		return fmt.Errorf("write blocked: modification of agent configuration not allowed")
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
	return os.WriteFile(absPath, updated, filePerm)
}
