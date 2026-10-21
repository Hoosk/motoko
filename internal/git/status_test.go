package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestParseStatus(t *testing.T) {
	out := "# branch.oid 1234\x00" +
		"# branch.head main\x00" +
		"1 .M N... 100644 100644 100644 aaa aaa foo.go\x00" +
		"1 A. N... 000000 100644 100644 000 bbb dir/with space.go\x00" +
		"2 R. N... 100644 100644 100644 ccc ccc R100 new name.go\x00old name.go\x00" +
		"u UU N... 100644 100644 100644 100644 d1 d2 d3 conflict.go\x00" +
		"? untracked.txt\x00"

	got := ParseStatus([]byte(out))
	want := Status{
		Branch: "main",
		Entries: []Entry{
			{Path: "foo.go", Index: '.', Worktree: 'M'},
			{Path: "dir/with space.go", Index: 'A', Worktree: '.'},
			{Path: "new name.go", OrigPath: "old name.go", Index: 'R', Worktree: '.'},
			{Path: "conflict.go", Index: 'U', Worktree: 'U'},
			{Path: "untracked.txt", Index: '?', Worktree: '?'},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseStatus mismatch\n got: %#v\nwant: %#v", got, want)
	}

	// A worktree-only change must not be counted as staged (the old
	// --short parser trimmed the leading space of the first line).
	if got.Entries[0].Staged() || !got.Entries[0].Unstaged() {
		t.Fatalf("worktree-only change misclassified: %#v", got.Entries[0])
	}
	if !got.Entries[1].Staged() || got.Entries[1].Unstaged() {
		t.Fatalf("index-only change misclassified: %#v", got.Entries[1])
	}
	if !got.Entries[4].Untracked() || got.Entries[4].Staged() || got.Entries[4].Unstaged() {
		t.Fatalf("untracked entry misclassified: %#v", got.Entries[4])
	}
}

func TestParseStatusDetachedHead(t *testing.T) {
	got := ParseStatus([]byte("# branch.oid abc\x00# branch.head (detached)\x00"))
	if got.Branch != "HEAD" {
		t.Fatalf("expected detached HEAD to map to %q, got %q", "HEAD", got.Branch)
	}
}

func TestReadStatusFromSubdirectory(t *testing.T) {
	root := newTestRepo(t)
	writeFile(t, filepath.Join(root, "src", "a.go"), "package src\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-qm", "init")

	writeFile(t, filepath.Join(root, "src", "a.go"), "package src\n// changed\n")
	writeFile(t, filepath.Join(root, "sub", "new", "b.go"), "package new\n")

	status, err := ReadStatus(context.Background(), filepath.Join(root, "sub"), true)
	if err != nil {
		t.Fatalf("ReadStatus: %v", err)
	}
	paths := map[string]bool{}
	for _, e := range status.Entries {
		paths[e.Path] = true
	}
	if !paths["../src/a.go"] || !paths["new/b.go"] {
		t.Fatalf("expected paths relative to sub/, got %#v", status.Entries)
	}
	if status.Branch == "" {
		t.Fatal("expected a branch name")
	}
}

func TestReadStatusDoesNotRewriteIndex(t *testing.T) {
	root := newTestRepo(t)
	file := filepath.Join(root, "a.go")
	writeFile(t, file, "package a\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-qm", "init")

	// Make the cached stat data stale: a plain `git status` would refresh it
	// by taking index.lock and rewriting .git/index.
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(file, future, future); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(root, ".git", "index")
	before, err := os.Stat(index)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadStatus(context.Background(), root, false); err != nil {
		t.Fatalf("ReadStatus: %v", err)
	}
	after, err := os.Stat(index)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("ReadStatus rewrote .git/index; optional locks must be disabled")
	}
}

func newTestRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "test")
	return root
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
