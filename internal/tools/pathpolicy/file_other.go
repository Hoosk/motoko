//go:build !darwin && !linux

package pathpolicy

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func openFileSecure(resolved Resolution, write bool, mode, dirMode fs.FileMode) (*os.File, error) {
	current, err := Resolve(resolved.Requested)
	if err != nil {
		return nil, err
	}
	if current.Path != resolved.Path || current.External != resolved.External || current.existing != resolved.existing {
		return nil, fmt.Errorf("path changed after validation: %s", resolved.Relative)
	}
	if resolved.anchorInfo != nil {
		anchorInfo, statErr := os.Stat(resolved.anchor)
		if statErr != nil || !os.SameFile(resolved.anchorInfo, anchorInfo) {
			return nil, fmt.Errorf("parent path changed after validation: %s", resolved.Relative)
		}
	}
	if write && !resolved.existing {
		if err := os.MkdirAll(filepath.Dir(resolved.Path), dirMode); err != nil {
			return nil, err
		}
	}
	flags := os.O_RDONLY
	if write {
		flags = os.O_WRONLY
		if !resolved.existing {
			flags |= os.O_CREATE | os.O_EXCL
		}
	}
	file, err := os.OpenFile(resolved.Path, flags, mode)
	if err != nil {
		return nil, err
	}
	if err := verifyIdentity(resolved, file); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}
