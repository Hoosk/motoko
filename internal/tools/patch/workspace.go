package patch

import (
	"context"
	"fmt"

	"github.com/Hoosk/motoko/internal/tools/pathpolicy"
)

type ExternalApprover func(context.Context, pathpolicy.Resolution) error

func resolveWorkspacePath(target string) (string, string, error) {
	resolved, err := pathpolicy.Resolve(target)
	if err != nil {
		return "", "", err
	}
	return resolved.Path, resolved.Relative, nil
}

// ValidateWritePath checks if the given target path is allowed for writes.
func ValidateWritePath(target string) error {
	resolved, err := pathpolicy.Resolve(target)
	if err != nil {
		return err
	}
	return pathpolicy.ValidateWrite(resolved)
}

// resolveWorkspaceWritePath resolves target and ensures it is a safe write path.
func resolveWorkspaceWritePath(ctx context.Context, target string, approve ExternalApprover) (pathpolicy.Resolution, error) {
	resolved, err := pathpolicy.Resolve(target)
	if err != nil {
		return pathpolicy.Resolution{}, err
	}
	if err := pathpolicy.ValidateWrite(resolved); err != nil {
		return pathpolicy.Resolution{}, err
	}
	if resolved.External {
		if approve == nil {
			return pathpolicy.Resolution{}, fmt.Errorf("modification requires approval: symlink resolves outside workspace to %s", resolved.Path)
		}
		if err := approve(ctx, resolved); err != nil {
			return pathpolicy.Resolution{}, err
		}
	}
	return resolved, nil
}
