package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrNoUpdateAvailable is returned when the current version is already the latest.
var ErrNoUpdateAvailable = errors.New("no update available")

// Update checks for a new version and updates the binary if available.
func (u *Updater) Update(ctx context.Context) error {
	info, err := u.CheckVersion(ctx)
	if err != nil {
		return fmt.Errorf("check version: %w", err)
	}

	if !info.IsNewer {
		return ErrNoUpdateAvailable
	}

	// Get target executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("eval symlinks: %w", err)
	}
	execDir := filepath.Dir(execPath)

	// Download tarball
	req, err := http.NewRequestWithContext(ctx, "GET", info.DownloadURL, nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}
	req.Header.Set("User-Agent", "motoko-updater")

	client := &http.Client{
		Timeout: 5 * time.Minute,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %s", resp.Status)
	}

	// Create a temporary file in the same directory as the executable
	tmpFile, err := os.CreateTemp(execDir, "motoko-update-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
	}()

	// Decompress and extract
	gzipReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("init gzip reader: %w", err)
	}
	defer func() { _ = gzipReader.Close() }()

	tarReader := tar.NewReader(gzipReader)
	found := false

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar archive: %w", err)
		}

		// Look for the binary (named "motoko", "motoko_os_arch", or containing "motoko")
		baseName := filepath.Base(header.Name)
		expectedName1 := "motoko"
		expectedName2 := fmt.Sprintf("motoko_%s_%s", u.goos, u.goarch)

		if header.Typeflag == tar.TypeReg && (baseName == expectedName1 || baseName == expectedName2 || strings.Contains(baseName, "motoko")) {
			if _, err := io.Copy(tmpFile, tarReader); err != nil {
				return fmt.Errorf("extract binary to temp file: %w", err)
			}
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("binary not found in the downloaded archive")
	}

	// Make the temp binary executable
	if err := tmpFile.Chmod(0755); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// Backup current binary
	backupPath := filepath.Join(execDir, ".motoko.bak")
	_ = os.Remove(backupPath) // ignore error if it doesn't exist

	if err := os.Rename(execPath, backupPath); err != nil {
		return fmt.Errorf("create backup of current binary: %w", err)
	}

	// Replace executable with updated one
	if err := os.Rename(tmpPath, execPath); err != nil {
		// Attempt to restore backup
		_ = os.Rename(backupPath, execPath)
		return fmt.Errorf("replace binary: %w", err)
	}

	return nil
}
