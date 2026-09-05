package pathpolicy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

var ErrFileChanged = errors.New("file changed while awaiting approval")

var workspaceWriteMu sync.Mutex

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

// WriteFile checks the approved preimage and writes through the same verified
// descriptor. Opening the path again after approval would reintroduce races.
func WriteFile(ctx context.Context, resolved Resolution, expected, data []byte, mode, dirMode fs.FileMode) (returnErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	workspaceWriteMu.Lock()
	defer workspaceWriteMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if resolved.existing && resolved.info.IsDir() {
		return fmt.Errorf("path is a directory: %s", resolved.Relative)
	}
	file, err := openFileSecure(resolved, true, mode, dirMode)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); returnErr == nil {
			returnErr = err
		}
	}()
	if resolved.existing {
		current, err := io.ReadAll(file)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, expected) {
			return fmt.Errorf("%w: %s", ErrFileChanged, resolved.Relative)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := file.Truncate(0); err != nil {
			return err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return err
		}
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	return nil
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
