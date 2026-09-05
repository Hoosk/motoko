package pathpolicy

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidateWrite applies protected-file policy to both the requested path and
// the canonical destination, preventing symlink aliases from bypassing it.
func ValidateWrite(resolved Resolution) error {
	paths := []string{resolved.Relative}
	if resolved.External {
		paths = append(paths, filepath.ToSlash(resolved.Path))
	} else {
		workspace, err := Resolve("")
		if err != nil {
			return err
		}
		realRel, err := filepath.Rel(workspace.Path, resolved.Path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(realRel))
	}

	for _, path := range paths {
		if err := validateProtectedPath(path); err != nil {
			return err
		}
	}
	return nil
}

func validateProtectedPath(path string) error {
	lower := strings.ToLower(filepath.ToSlash(path))
	components := strings.Split(strings.Trim(lower, "/"), "/")
	baseName := filepath.Base(lower)

	for _, component := range components {
		switch component {
		case ".git":
			return fmt.Errorf("write blocked: path inside git infrastructure (.git) not allowed")
		case ".ssh":
			return fmt.Errorf("write blocked: modification of SSH keys or system credentials not allowed")
		case ".antigravitycli":
			return fmt.Errorf("write blocked: modification of agent configuration not allowed")
		}
	}
	if strings.HasPrefix(baseName, ".env") {
		return fmt.Errorf("write blocked: modification of environment variable files (.env) not allowed")
	}
	if strings.HasPrefix(baseName, "id_rsa") || strings.HasPrefix(baseName, "id_dsa") ||
		strings.HasPrefix(baseName, "id_ecdsa") || strings.HasPrefix(baseName, "id_ed25519") ||
		baseName == "authorized_keys" || baseName == "known_hosts" {
		return fmt.Errorf("write blocked: modification of SSH keys or system credentials not allowed")
	}
	return nil
}
