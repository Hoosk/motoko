package git

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"sync"
)

// Entry is one changed path reported by git status.
type Entry struct {
	// Path is relative to the directory passed to ReadStatus (it may start
	// with "../" when the change lives outside that directory).
	Path string
	// OrigPath is the source path of a rename or copy.
	OrigPath string
	// Index and Worktree are the porcelain X/Y codes; '.' means unchanged.
	Index    byte
	Worktree byte
}

func (e Entry) Untracked() bool { return e.Index == '?' }
func (e Entry) Staged() bool    { return !e.Untracked() && e.Index != '.' }
func (e Entry) Unstaged() bool  { return !e.Untracked() && e.Worktree != '.' }

// Status is the parsed output of `git status --porcelain=v2 --branch -z`.
type Status struct {
	// Branch is the checked-out branch, or "HEAD" when detached.
	Branch  string
	Entries []Entry
}

// ReadStatus returns the status of the repository containing dir. With
// allUntracked, files inside untracked directories are listed individually
// instead of collapsing to "dir/".
func ReadStatus(ctx context.Context, dir string, allUntracked bool) (Status, error) {
	args := []string{"status", "--porcelain=v2", "--branch", "-z"}
	if allUntracked {
		args = append(args, "--untracked-files=all")
	}
	out, err := Output(ctx, dir, args...)
	if err != nil {
		return Status{}, err
	}
	status := ParseStatus(out)
	prefix, err := showPrefix(ctx, dir)
	if err != nil {
		return Status{}, err
	}
	if prefix != "" {
		for i := range status.Entries {
			status.Entries[i].Path = relativeTo(prefix, status.Entries[i].Path)
			if status.Entries[i].OrigPath != "" {
				status.Entries[i].OrigPath = relativeTo(prefix, status.Entries[i].OrigPath)
			}
		}
	}
	return status, nil
}

// ParseStatus parses porcelain v2 output produced with --branch and -z.
// Paths are left relative to the repository root.
func ParseStatus(out []byte) Status {
	var status Status
	records := bytes.Split(out, []byte{0})
	for i := 0; i < len(records); i++ {
		rec := string(records[i])
		if len(rec) < 2 {
			continue
		}
		switch rec[0] {
		case '#':
			if head, ok := strings.CutPrefix(rec, "# branch.head "); ok {
				if head == "(detached)" {
					head = "HEAD"
				}
				status.Branch = head
			}
		case '1':
			// 1 XY sub mH mI mW hH hI path
			if f := strings.SplitN(rec, " ", 9); len(f) == 9 && len(f[1]) == 2 {
				status.Entries = append(status.Entries, Entry{Path: f[8], Index: f[1][0], Worktree: f[1][1]})
			}
		case '2':
			// 2 XY sub mH mI mW hH hI Xscore path, followed by the original path record
			if f := strings.SplitN(rec, " ", 10); len(f) == 10 && len(f[1]) == 2 {
				entry := Entry{Path: f[9], Index: f[1][0], Worktree: f[1][1]}
				if i+1 < len(records) {
					i++
					entry.OrigPath = string(records[i])
				}
				status.Entries = append(status.Entries, entry)
			}
		case 'u':
			// u XY sub m1 m2 m3 mW h1 h2 h3 path
			if f := strings.SplitN(rec, " ", 11); len(f) == 11 && len(f[1]) == 2 {
				status.Entries = append(status.Entries, Entry{Path: f[10], Index: f[1][0], Worktree: f[1][1]})
			}
		case '?':
			status.Entries = append(status.Entries, Entry{Path: rec[2:], Index: '?', Worktree: '?'})
		}
	}
	return status
}

var prefixCache sync.Map // abs dir -> repo-relative prefix

// showPrefix returns dir's path relative to the repository root ("" at the
// root, "sub/" below it). It never changes for a directory, so it is cached.
func showPrefix(ctx context.Context, dir string) (string, error) {
	key := dir
	if abs, err := filepath.Abs(dir); err == nil {
		key = abs
	}
	if cached, ok := prefixCache.Load(key); ok {
		return cached.(string), nil
	}
	out, err := Output(ctx, dir, "rev-parse", "--show-prefix")
	if err != nil {
		return "", err
	}
	prefix := strings.TrimSpace(string(out))
	prefixCache.Store(key, prefix)
	return prefix, nil
}

func relativeTo(prefix, rootRel string) string {
	rel, err := filepath.Rel(filepath.FromSlash(prefix), filepath.FromSlash(rootRel))
	if err != nil {
		return rootRel
	}
	return filepath.ToSlash(rel)
}
