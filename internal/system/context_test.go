package system

import "testing"

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

func TestPopulateGitStatusCountsEntries(t *testing.T) {
	info := &ContextInfo{}
	populateGitStatus(info)
	if info.Staged < 0 || info.Unstaged < 0 || info.Untracked < 0 {
		t.Fatalf("git counters should never be negative: %#v", info)
	}
}

func TestGetGitInfo(t *testing.T) {
	info := GetGitInfo(".")
	if !info.HasGit {
		t.Log("Note: running test in non-git workspace")
		return
	}
	if info.GitBranch == "" {
		t.Error("expected non-empty GitBranch when HasGit is true")
	}
}
