package tachikoma

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Hoosk/motoko/internal/git"
)

const (
	watchTestDebounce = 20 * time.Millisecond
	watchTestWait     = 3 * time.Second
	watchTestQuiet    = 500 * time.Millisecond
)

func TestWatchHubSkipsGitInternalsAndIgnoredDirectories(t *testing.T) {
	root := newWatchTestRepo(t)
	hub := startWatchHub(t, root)

	watched := hub.watcher.WatchList()
	for _, want := range []string{root, filepath.Join(root, "src"), filepath.Join(root, ".git"), filepath.Join(root, ".git", "refs", "heads")} {
		if !slices.Contains(watched, want) {
			t.Errorf("expected %s to be watched; watching %v", want, watched)
		}
	}
	for _, unwanted := range []string{filepath.Join(root, "dist"), filepath.Join(root, ".git", "objects")} {
		if slices.Contains(watched, unwanted) {
			t.Errorf("did not expect %s to be watched", unwanted)
		}
	}
}

func TestWatchHubNotifiesWorkspaceChanges(t *testing.T) {
	root := newWatchTestRepo(t)
	hub := startWatchHub(t, root)
	events := hub.Subscribe(ScopeWorkspace, watchTestDebounce)

	writeWatchTestFile(t, filepath.Join(root, "src", "b.go"), "package src\n")
	expectWatchEvent(t, events)
}

func TestWatchHubIgnoresOwnGitStatusAndLateIgnoredDirectories(t *testing.T) {
	root := newWatchTestRepo(t)
	hub := startWatchHub(t, root)
	events := hub.Subscribe(ScopeWorkspace|ScopeGitMeta, watchTestDebounce)

	// Make the index stat data stale so a locking status would rewrite it.
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(root, "src", "a.go"), future, future); err != nil {
		t.Fatal(err)
	}
	expectNoWatchEvent(t, events) // attribute-only change

	for range 3 {
		if _, err := git.ReadStatus(context.Background(), root, false); err != nil {
			t.Fatal(err)
		}
	}
	expectNoWatchEvent(t, events)

	// target/ is ignored but did not exist when the hub started.
	writeWatchTestFile(t, filepath.Join(root, "target", "debug", "out.bin"), "x")
	time.Sleep(watchTestQuiet)
	writeWatchTestFile(t, filepath.Join(root, "target", "debug", "out2.bin"), "y")
	expectNoWatchEvent(t, events)
	if slices.Contains(hub.watcher.WatchList(), filepath.Join(root, "target")) {
		t.Fatal("late ignored directory should not be watched")
	}
}

func TestWatchHubGitMetaScope(t *testing.T) {
	root := newWatchTestRepo(t)
	hub := startWatchHub(t, root)
	events := hub.Subscribe(ScopeGitMeta, watchTestDebounce)

	writeWatchTestFile(t, filepath.Join(root, "src", "c.go"), "package src\n")
	expectNoWatchEvent(t, events)

	runWatchTestGit(t, root, "add", "src/c.go")
	expectWatchEvent(t, events)
}

func startWatchHub(t *testing.T, root string) *WatchHub {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	hub, err := NewWatchHub(ctx, root)
	if err != nil {
		t.Fatalf("NewWatchHub: %v", err)
	}
	return hub
}

func newWatchTestRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	runWatchTestGit(t, root, "init", "-q")
	runWatchTestGit(t, root, "config", "user.email", "test@example.com")
	runWatchTestGit(t, root, "config", "user.name", "test")
	writeWatchTestFile(t, filepath.Join(root, ".gitignore"), "dist/\ntarget/\n")
	writeWatchTestFile(t, filepath.Join(root, "src", "a.go"), "package src\n")
	writeWatchTestFile(t, filepath.Join(root, "dist", "bundle.js"), "x")
	runWatchTestGit(t, root, "add", ".")
	runWatchTestGit(t, root, "commit", "-qm", "init")
	return root
}

func runWatchTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeWatchTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func expectWatchEvent(t *testing.T, events <-chan struct{}) {
	t.Helper()
	select {
	case <-events:
	case <-time.After(watchTestWait):
		t.Fatal("timed out waiting for a watch notification")
	}
}

func expectNoWatchEvent(t *testing.T, events <-chan struct{}) {
	t.Helper()
	select {
	case <-events:
		t.Fatal("unexpected watch notification")
	case <-time.After(watchTestQuiet):
	}
}
