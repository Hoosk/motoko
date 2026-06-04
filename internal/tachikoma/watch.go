package tachikoma

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/Hoosk/motoko/internal/ignore"
	"github.com/fsnotify/fsnotify"
)

// WatchHelper provides a debounced channel for file system events.
// It watches the provided paths recursively, respecting .gitignore rules.
func WatchHelper(ctx context.Context, rootPaths []string, debounce time.Duration) (<-chan struct{}, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	out := make(chan struct{}, 1)

	// Load ignore matchers for each root
	matchers := make(map[string]*ignore.Matcher)
	for _, root := range rootPaths {
		m, _ := ignore.Load(root)
		matchers[root] = m
	}

	go func() {
		defer func() { _ = watcher.Close() }()

		var timer *time.Timer

		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				_ = err
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				// If a new directory is created, watch it if not ignored
				if event.Has(fsnotify.Create) {
					info, err := os.Stat(event.Name)
					if err == nil && info.IsDir() {
						// Find which root this belongs to
						for root, m := range matchers {
							if rel, err := filepath.Rel(root, event.Name); err == nil && !filepath.IsAbs(rel) && !m.Ignored(rel, true) {
								_ = addRecursive(watcher, event.Name, m)
								break
							}
						}
					}
				}

				if timer != nil {
					timer.Stop()
				}

				timer = time.AfterFunc(debounce, func() {
					select {
					case out <- struct{}{}:
					default:
					}
				})
			}
		}
	}()

	for _, path := range rootPaths {
		m := matchers[path]
		if err := addRecursive(watcher, path, m); err != nil {
			continue
		}
	}

	return out, nil
}

// addRecursive adds a directory and all its non-ignored subdirectories to the watcher.
func addRecursive(watcher *fsnotify.Watcher, root string, m *ignore.Matcher) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}

		if rel != "." && m != nil && m.Ignored(rel, true) {
			return filepath.SkipDir
		}

		return watcher.Add(path)
	})
}
