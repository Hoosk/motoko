//go:build darwin || linux

package pathpolicy

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func openFileSecure(resolved Resolution, write bool, mode, dirMode fs.FileMode) (*os.File, error) {
	parts := strings.Split(strings.TrimPrefix(filepath.Clean(resolved.Path), string(filepath.Separator)), string(filepath.Separator))
	if len(parts) == 0 || parts[0] == "" {
		return nil, fmt.Errorf("invalid target path: %s", resolved.Path)
	}

	fd, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = unix.Close(fd) }()

	currentPath := string(filepath.Separator)
	for _, component := range parts[:len(parts)-1] {
		next, openErr := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if openErr != nil && write && !resolved.existing && openErr == unix.ENOENT {
			if mkdirErr := unix.Mkdirat(fd, component, uint32(dirMode.Perm())); mkdirErr != nil && mkdirErr != unix.EEXIST {
				return nil, mkdirErr
			}
			next, openErr = unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		}
		if openErr != nil {
			return nil, fmt.Errorf("open verified parent %s: %w", component, openErr)
		}
		_ = unix.Close(fd)
		fd = next
		currentPath = filepath.Join(currentPath, component)
		verifyFD, dupErr := unix.Dup(fd)
		if dupErr != nil {
			return nil, dupErr
		}
		parent := os.NewFile(uintptr(verifyFD), currentPath)
		if parent == nil {
			_ = unix.Close(verifyFD)
			return nil, fmt.Errorf("open verified parent: invalid file descriptor")
		}
		if err := verifyAnchor(resolved, currentPath, parent); err != nil {
			_ = parent.Close()
			return nil, err
		}
		_ = parent.Close()
	}

	flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC
	if write {
		flags = unix.O_WRONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC
		if !resolved.existing {
			flags |= unix.O_CREAT | unix.O_EXCL
		}
	}
	targetFD, err := unix.Openat(fd, parts[len(parts)-1], flags, uint32(mode.Perm()))
	if err != nil {
		return nil, fmt.Errorf("open verified target: %w", err)
	}
	file := os.NewFile(uintptr(targetFD), resolved.Path)
	if file == nil {
		_ = unix.Close(targetFD)
		return nil, fmt.Errorf("open verified target: invalid file descriptor")
	}
	if err := verifyIdentity(resolved, file); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}
