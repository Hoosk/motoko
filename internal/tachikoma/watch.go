package tachikoma

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Hoosk/motoko/internal/ignore"
	"github.com/Hoosk/motoko/internal/tracelog"
	"github.com/fsnotify/fsnotify"
)

// WatchScope selects which filesystem changes a subscriber is notified about.
type WatchScope uint8

const (
	// ScopeWorkspace covers non-ignored files in the workspace.
	ScopeWorkspace WatchScope = 1 << iota
	// ScopeGitMeta covers HEAD, the index and branch refs: commits, staging
	// and checkouts that do not touch the working tree.
	ScopeGitMeta
)

// maxWatchEntries bounds the file descriptors spent on watching. On kqueue
// platforms (macOS, BSD) fsnotify opens one descriptor per file inside every
// watched directory, so large trees would exhaust the process limit. Past the
// budget the hub stops adding directories and workers rely on their polling
// interval instead.
const maxWatchEntries = 4096

// gitMetaFiles are the entries directly inside .git whose changes affect what
// status or diff report.
var gitMetaFiles = []string{"HEAD", "index", "packed-refs"}

// WatchHub owns the single recursive watcher shared by every Tachikoma and
// fans debounced change notifications out to subscribers.
type WatchHub struct {
	watcher *fsnotify.Watcher
	matcher *ignore.Matcher
	root    string
	gitDir  string
	subs    []*watchSub
	mu      sync.Mutex
	entries int
	full    bool
}

type watchSub struct {
	out      chan struct{}
	timer    *time.Timer
	debounce time.Duration
	scope    WatchScope
}

// NewWatchHub starts watching root until ctx is cancelled.
func NewWatchHub(ctx context.Context, root string) (*WatchHub, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	matcher, _ := ignore.Load(root)
	h := &WatchHub{watcher: watcher, matcher: matcher, root: root}

	h.addTree(root)
	if gitDir := filepath.Join(root, ".git"); isDir(gitDir) {
		// Only the top of .git and the branch refs: watching objects/ and
		// logs/ is expensive, and lock files churn on every git command.
		h.gitDir = gitDir
		_ = watcher.Add(gitDir)
		if refs := filepath.Join(gitDir, "refs", "heads"); isDir(refs) {
			_ = watcher.Add(refs)
		}
	}

	go h.loop(ctx)
	return h, nil
}

// Subscribe returns a channel that receives a value once changes in scope
// have been quiet for debounce. Notifications coalesce; the channel never
// blocks the hub.
func (h *WatchHub) Subscribe(scope WatchScope, debounce time.Duration) <-chan struct{} {
	sub := &watchSub{out: make(chan struct{}, 1), debounce: debounce, scope: scope}
	h.mu.Lock()
	h.subs = append(h.subs, sub)
	h.mu.Unlock()
	return sub.out
}

func (h *WatchHub) loop(ctx context.Context) {
	defer func() {
		_ = h.watcher.Close()
		h.mu.Lock()
		for _, sub := range h.subs {
			if sub.timer != nil {
				sub.timer.Stop()
			}
		}
		h.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-h.watcher.Errors:
			if !ok {
				return
			}
			tracelog.Logf("tachikoma watch error: %v", err)
		case event, ok := <-h.watcher.Events:
			if !ok {
				return
			}
			if scope := h.classify(event); scope != 0 {
				h.notify(scope)
			}
		}
	}
}

// classify maps an event to the scope it affects, or 0 to drop it. It also
// starts watching directories created inside the workspace.
func (h *WatchHub) classify(event fsnotify.Event) WatchScope {
	// Attribute-only changes carry no content change, and on macOS they fire
	// whenever git merely reads .git/index (also for Spotlight, touch...).
	if event.Op == fsnotify.Chmod {
		return 0
	}
	if h.gitDir != "" && (event.Name == h.gitDir || strings.HasPrefix(event.Name, h.gitDir+string(filepath.Separator))) {
		rel, err := filepath.Rel(h.gitDir, event.Name)
		if err != nil {
			return 0
		}
		rel = filepath.ToSlash(rel)
		if slices.Contains(gitMetaFiles, rel) || strings.HasPrefix(rel, "refs/heads/") {
			return ScopeGitMeta
		}
		return 0
	}

	rel, err := filepath.Rel(h.root, event.Name)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return 0
	}
	rel = filepath.ToSlash(rel)

	if event.Has(fsnotify.Create) && isDir(event.Name) {
		// The matcher is a snapshot taken at startup, so a build output
		// directory created later (dist/, target/...) is not in it; ask git.
		if h.matcher.Ignored(rel, true) || h.matcher.IgnoredByGit(rel) {
			return 0
		}
		h.addTree(event.Name)
		return ScopeWorkspace
	}
	if h.matcher.Ignored(rel, false) {
		return 0
	}
	return ScopeWorkspace
}

func (h *WatchHub) notify(scope WatchScope) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, sub := range h.subs {
		if sub.scope&scope == 0 {
			continue
		}
		if sub.timer != nil {
			sub.timer.Stop()
		}
		out := sub.out
		sub.timer = time.AfterFunc(sub.debounce, func() {
			select {
			case out <- struct{}{}:
			default:
			}
		})
	}
}

// addTree watches dir and its non-ignored subdirectories, within the entry
// budget. Ignore rules are evaluated relative to the workspace root.
func (h *WatchHub) addTree(dir string) {
	if h.full {
		return
	}
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(h.root, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !d.IsDir() {
			if fileWatchesCost && !h.matcher.Ignored(rel, false) {
				h.entries++
			}
			return nil
		}
		if rel != "." && h.matcher.Ignored(rel, true) {
			return filepath.SkipDir
		}
		if h.entries >= maxWatchEntries {
			h.full = true
			tracelog.Logf("tachikoma watch budget of %d entries reached; relying on polling below %s", maxWatchEntries, rel)
			return filepath.SkipAll
		}
		if err := h.watcher.Add(path); err != nil {
			// Typically EMFILE/ENOSPC: stop instead of failing every Add.
			h.full = true
			tracelog.Logf("tachikoma watch add %s: %v", rel, err)
			return filepath.SkipAll
		}
		h.entries++
		return nil
	})
}

// fileWatchesCost reports whether watching a directory also spends a
// descriptor per file in it (kqueue), as opposed to one per directory
// (inotify, ReadDirectoryChangesW).
var fileWatchesCost = runtime.GOOS != "linux" && runtime.GOOS != "windows"

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

type watchHubKey struct{}

// withWatchHub attaches the shared hub to ctx so workers started by the
// Manager subscribe to it instead of creating watchers of their own.
func withWatchHub(ctx context.Context, h *WatchHub) context.Context {
	return context.WithValue(ctx, watchHubKey{}, h)
}

// watchHubFrom returns the shared hub and whether one was attached at all;
// an attached nil hub means watching is unavailable.
func watchHubFrom(ctx context.Context) (*WatchHub, bool) {
	h, ok := ctx.Value(watchHubKey{}).(*WatchHub)
	return h, ok
}
