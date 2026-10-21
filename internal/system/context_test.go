package system

import (
	"context"
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/git"
)

func TestContextInfoSummaries(t *testing.T) {
	info := ContextInfo{
		GitBranch:        "main",
		HasGit:           true,
		GitDirty:         true,
		Staged:           1,
		Unstaged:         2,
		Untracked:        3,
		Signals:          map[string]string{"semantic": "ok"},
		RelevantFiles:    []string{"a.go", "b.go"},
		RelevantSnippets: []string{"snippet one", "snippet two"},
	}

	if got := info.GitSummary(); got != "main (dirty staged:1 unstaged:2 untracked:3)" {
		t.Fatalf("unexpected git summary %q", got)
	}
	if got := info.SignalSummary(); got != "semantic: ok" {
		t.Fatalf("unexpected signal summary %q", got)
	}
	if got := info.RelevantFilesSummary(); got != "a.go\nb.go" {
		t.Fatalf("unexpected relevant files summary %q", got)
	}
	if got := info.RelevantSnippetsSummary(); got != "snippet one\n\nsnippet two" {
		t.Fatalf("unexpected relevant snippets summary %q", got)
	}
}

func TestContextInfoFallbackSummaries(t *testing.T) {
	info := ContextInfo{}
	if got := info.GitSummary(); got != "no git repository" {
		t.Fatalf("unexpected git fallback %q", got)
	}
	if got := info.SignalSummary(); got != "no extra signals" {
		t.Fatalf("unexpected signal fallback %q", got)
	}
	if got := info.RelevantFilesSummary(); got != "no suggested relevant files" {
		t.Fatalf("unexpected files fallback %q", got)
	}
	if got := info.RelevantSnippetsSummary(); got != "no relevant snippets" {
		t.Fatalf("unexpected snippets fallback %q", got)
	}
}

func TestApplyGitStatusCountsEntries(t *testing.T) {
	info := &ContextInfo{}
	applyGitStatus(info, git.Status{Entries: []git.Entry{
		// Worktree-only change listed first: the old --short parser trimmed
		// its leading space, turning "foo.go" into "oo.go" counted as staged.
		{Path: "foo.go", Index: '.', Worktree: 'M'},
		{Path: "bar.go", Index: 'M', Worktree: 'M'},
		{Path: "new.go", OrigPath: "old.go", Index: 'R', Worktree: '.'},
		{Path: "notes.txt", Index: '?', Worktree: '?'},
	}})

	if info.Staged != 2 || info.Unstaged != 2 || info.Untracked != 1 || !info.GitDirty {
		t.Fatalf("unexpected counters: %#v", info)
	}
	want := []string{"foo.go", "bar.go", "new.go", "notes.txt"}
	if strings.Join(info.ModifiedFiles, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected files %v, want %v", info.ModifiedFiles, want)
	}
}

func TestGetGitInfo(t *testing.T) {
	info := GetGitInfo(context.Background(), ".")
	if !info.HasGit {
		t.Log("Note: running test in non-git workspace")
		return
	}
	if info.GitBranch == "" {
		t.Error("expected non-empty GitBranch when HasGit is true")
	}
}
