package pathpolicy

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func OpenRead(resolved Resolution) (*os.File, error) {
	if !resolved.existing {
		return nil, os.ErrNotExist
	}
	return openFileSecure(resolved, false, 0, 0)
}

func ReadFile(resolved Resolution) ([]byte, error) {
	file, err := OpenRead(resolved)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return io.ReadAll(file)
}

func WriteFile(resolved Resolution, data []byte, mode, dirMode fs.FileMode) error {
	if resolved.existing && resolved.info.IsDir() {
		return fmt.Errorf("path is a directory: %s", resolved.Relative)
	}
	file, err := openFileSecure(resolved, true, mode, dirMode)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	if resolved.existing {
		if err := file.Truncate(0); err != nil {
			return err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return err
		}
	}
	_, err = file.Write(data)
	return err
}

func verifyIdentity(resolved Resolution, file *os.File) error {
	if !resolved.existing || resolved.info == nil {
		return nil
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(resolved.info, info) {
		return fmt.Errorf("path changed after validation: %s", resolved.Relative)
	}
	return nil
}

func verifyAnchor(resolved Resolution, path string, file *os.File) error {
	if resolved.anchorInfo == nil || filepath.Clean(path) != resolved.anchor {
		return nil
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(resolved.anchorInfo, info) {
		return fmt.Errorf("parent path changed after validation: %s", resolved.Relative)
	}
	return nil
}
