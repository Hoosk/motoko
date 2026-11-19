package system

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Hoosk/motoko/internal/git"
)

type SkillDef struct {
	Name        string
	Description string
}

type ContextInfo struct {
	Signals           map[string]string
	OnDemandSignals   map[string]string
	ThinkingVerbosity string
	Path              string
	BrainSummary      string
	Workspace         string
	SemanticSummary   string
	Guidelines        string
	DesignSpec        string
	ActiveMode        string
	GitBranch         string
	ModifiedFiles     []string
	RelevantFiles     []string
	AvailableSkills   []SkillDef
	AvailableAgents   []string
	RelevantSnippets  []string
	Untracked         int
	Unstaged          int
	Staged            int
	ContextWindow     int
	GitDirty          bool
	HasGit            bool
}

func GetContextInfo() ContextInfo {
	info := ContextInfo{}

	// Workspace
	cwd, err := os.Getwd()
	if err == nil {
		info.Workspace = filepath.Base(cwd)
		info.Path = cwd
	}

	info.Signals = make(map[string]string)
	info.OnDemandSignals = make(map[string]string)

	return info
}

func GetGitInfo(ctx context.Context, workspacePath string) ContextInfo {
	info := ContextInfo{
		Path: workspacePath,
	}
	if workspacePath != "" {
		info.Workspace = filepath.Base(workspacePath)
	}

	info.Signals = make(map[string]string)
	info.OnDemandSignals = make(map[string]string)

	// A single status call yields both the branch and the changed files; it
	// fails outside a repository.
	status, err := git.ReadStatus(ctx, workspacePath, false)
	if err != nil {
		return info
	}
	info.HasGit = true
	info.GitBranch = status.Branch
	if info.GitBranch == "" {
		info.GitBranch = "no branch"
	}
	applyGitStatus(&info, status)

	return info
}

func (c ContextInfo) GitSummary() string {
	if !c.HasGit {
		return "no git repository"
	}

	status := "clean"
	if c.GitDirty {
		status = fmt.Sprintf("dirty staged:%d unstaged:%d untracked:%d", c.Staged, c.Unstaged, c.Untracked)
		if len(c.ModifiedFiles) > 0 && len(c.ModifiedFiles) <= 10 {
			status += " | files: " + strings.Join(c.ModifiedFiles, ", ")
		} else if len(c.ModifiedFiles) > 10 {
			status += ` | many files modified, use inspect {"worker": "GitTachikoma"} for full list`
		}
	}

	return fmt.Sprintf("%s (%s)", c.GitBranch, status)
}

func (c ContextInfo) SignalSummary() string {
	if len(c.Signals) == 0 && len(c.OnDemandSignals) == 0 {
		return "no extra signals"
	}
	parts := make([]string, 0, len(c.Signals)+len(c.OnDemandSignals))
	for _, name := range sortedKeys(c.Signals) {
		parts = append(parts, fmt.Sprintf("%s: %s", name, c.Signals[name]))
	}
	for _, name := range sortedKeys(c.OnDemandSignals) {
		parts = append(parts, fmt.Sprintf("%s (available on-demand): %s", name, c.OnDemandSignals[name]))
	}
	return strings.Join(parts, " | ")
}

func (c ContextInfo) CategorizedSignalSummary() string {
	if len(c.Signals) == 0 && len(c.OnDemandSignals) == 0 {
		return "no extra signals"
	}

	var sb strings.Builder

	if len(c.Signals) > 0 {
		sb.WriteString("[Background Signals]\n")
		for _, name := range sortedKeys(c.Signals) {
			fmt.Fprintf(&sb, "- %s: %s\n", name, c.Signals[name])
		}
	}

	if len(c.OnDemandSignals) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("[On-Demand Signals]\n")
		for _, name := range sortedKeys(c.OnDemandSignals) {
			fmt.Fprintf(&sb, "- %s: %s\n", name, c.OnDemandSignals[name])
		}
	}

	return strings.TrimSpace(sb.String())
}

// sortedKeys returns the map keys in lexical order so prompt sections built
// from signal maps are byte-identical across calls (Go map iteration order is
// randomized, which would otherwise defeat provider prompt caching).
func sortedKeys(m map[string]string) []string {
	return slices.Sorted(maps.Keys(m))
}

func (c ContextInfo) RelevantFilesSummary() string {
	if len(c.RelevantFiles) == 0 {
		return "no suggested relevant files"
	}
	return strings.Join(c.RelevantFiles, "\n")
}

func (c ContextInfo) RelevantSnippetsSummary() string {
	if len(c.RelevantSnippets) == 0 {
		return "no relevant snippets"
	}
	return strings.Join(c.RelevantSnippets, "\n\n")
}

func applyGitStatus(info *ContextInfo, status git.Status) {
	for _, entry := range status.Entries {
		info.ModifiedFiles = append(info.ModifiedFiles, entry.Path)
		if entry.Untracked() {
			info.Untracked++
			continue
		}
		if entry.Staged() {
			info.Staged++
		}
		if entry.Unstaged() {
			info.Unstaged++
		}
	}

	info.GitDirty = info.Staged > 0 || info.Unstaged > 0 || info.Untracked > 0
}
