package semantic

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	workspaceignore "github.com/Hoosk/motoko/internal/ignore"
)

const (
	maxIndexedFileSize   = 256 * 1024
	maxSymbolsPerFile    = 24
	defaultSnippetFiles  = 3
	defaultSnippetBudget = 220
	maxSnippetLines      = 48
	maxIndexDepth        = 12
	staleAfter           = 30 * time.Second
	refreshTimeout       = 5 * time.Second
)

type Index struct {
	lastSnapshot *Snapshot
	mu           sync.RWMutex
}

func NewIndex() *Index {
	return &Index{}
}

func (idx *Index) Ensure(ctx context.Context) (*Snapshot, error) {
	idx.mu.RLock()
	s := idx.lastSnapshot
	idx.mu.RUnlock()
	if s != nil && time.Since(s.GeneratedAt) < staleAfter {
		return s, nil
	}
	return idx.Refresh(ctx)
}

func (idx *Index) Refresh(ctx context.Context) (*Snapshot, error) {
	return idx.RefreshDir(ctx, "")
}

func (idx *Index) RefreshDir(ctx context.Context, root string) (*Snapshot, error) {
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	root = filepath.Clean(root)
	if ctx == nil {
		ctx = context.Background()
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, refreshTimeout)
		defer cancel()
	}
	snapshot := &Snapshot{}
	snapshot.GeneratedAt = time.Now()
	snapshot.Root = root
	snapshot.LanguageCounts = make(map[string]int)

	matcher, err := workspaceignore.Load(root)
	if err != nil {
		return nil, err
	}
	changed := findChangedFiles(root)
	snapshot.ChangedPaths = changed
	changedMap := make(map[string]bool)
	for _, p := range changed {
		changedMap[p] = true
	}
	dirsSeen := make(map[string]bool)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if depth := strings.Count(rel, "/") + 1; depth > maxIndexDepth {
				return filepath.SkipDir
			}
			if matcher.Ignored(rel, true) {
				return filepath.SkipDir
			}
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if matcher.Ignored(rel, false) {
			return nil
		}
		if !isSupported(rel) {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil || info.Size() > maxIndexedFileSize {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lang, langName := languageForPath(rel)
		symbols, imports, exports := extractSymbolsAndDeps(content, lang, langName)
		summary := FileSummary{
			Path:     rel,
			Language: langName,
			Lines:    strings.Count(string(content), "\n") + 1,
			Changed:  changedMap[rel],
			Symbols:  symbols,
			Imports:  imports,
			Exports:  exports,
			Content:  content,
		}
		snapshot.Files = append(snapshot.Files, summary)
		snapshot.LanguageCounts[langName]++
		dir := filepath.Dir(rel)
		if dir != "." && !dirsSeen[dir] {
			dirsSeen[dir] = true
			snapshot.Directories = append(snapshot.Directories, dir)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	idx.mu.Lock()
	idx.lastSnapshot = snapshot
	idx.mu.Unlock()
	return snapshot, nil
}

func findChangedFiles(root string) []string {
	cmd := exec.Command("git", "status", "--short")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var paths []string
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < 3 {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		paths = append(paths, parts[len(parts)-1])
	}
	return paths
}

func (idx *Index) SetSnapshotForTest(s *Snapshot) {
	idx.mu.Lock()
	idx.lastSnapshot = s
	idx.mu.Unlock()
}

func (idx *Index) LatestSnapshot() *Snapshot {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.lastSnapshot
}

func BuildSnapshot(ctx context.Context, root string) (*Snapshot, error) {
	idx := NewIndex()
	return idx.RefreshDir(ctx, root)
}
