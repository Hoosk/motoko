package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type SkillDef struct {
	Name        string
	Description string
}

type ContextInfo struct {
	Signals          map[string]string
	OnDemandSignals  map[string]string
	Path             string
	GitBranch        string
	BrainSummary     string
	Workspace        string
	SemanticSummary  string
	RelevantSnippets []string
	ModifiedFiles    []string
	RelevantFiles    []string
	AvailableSkills  []SkillDef
	AvailableAgents  []string
	Untracked        int
	Unstaged         int
	Staged           int
	GitDirty         bool
	HasGit           bool
	Guidelines       string
	DesignSpec       string
	ActiveMode       string
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

func GetGitInfo(workspacePath string) ContextInfo {
	info := ContextInfo{
		Path: workspacePath,
	}
	if workspacePath != "" {
		info.Workspace = filepath.Base(workspacePath)
	}

	info.Signals = make(map[string]string)
	info.OnDemandSignals = make(map[string]string)

	// Try to get branch name
	branchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	if info.Path != "" {
		branchCmd.Dir = info.Path
	}
	branch, err := branchCmd.Output()
	if err == nil {
		info.HasGit = true
		info.GitBranch = strings.TrimSpace(string(branch))
		populateGitStatus(&info)
	} else {
		// Fallback: check if we are inside a git repo at all
		gitCheck := exec.Command("git", "rev-parse", "--is-inside-work-tree")
		if info.Path != "" {
			gitCheck.Dir = info.Path
		}
		err := gitCheck.Run()
		if err == nil {
			info.HasGit = true
			info.GitBranch = "no branch"
			populateGitStatus(&info)
		} else {
			info.HasGit = false
		}
	}

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
			status += " | many files modified, use 'inspect GitTachikoma' for full list"
		}
	}

	return fmt.Sprintf("%s (%s)", c.GitBranch, status)
}

func (c ContextInfo) SignalSummary() string {
	if len(c.Signals) == 0 && len(c.OnDemandSignals) == 0 {
		return "no extra signals"
	}
	parts := make([]string, 0, len(c.Signals)+len(c.OnDemandSignals))
	for name, status := range c.Signals {
		parts = append(parts, fmt.Sprintf("%s: %s", name, status))
	}
	for name, hint := range c.OnDemandSignals {
		parts = append(parts, fmt.Sprintf("%s (available on-demand): %s", name, hint))
	}
	return strings.Join(parts, " | ")
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

func populateGitStatus(info *ContextInfo) {
	statusCmd := exec.Command("git", "status", "--short")
	if info.Path != "" {
		statusCmd.Dir = info.Path
	}

	output, err := statusCmd.Output()
	if err != nil {
		return
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return
	}

	for _, line := range lines {
		if len(line) < 3 {
			continue
		}

		// Capture file name (status is 2 chars + space)
		fileName := strings.TrimSpace(line[3:])
		if fileName != "" {
			info.ModifiedFiles = append(info.ModifiedFiles, fileName)
		}

		if strings.HasPrefix(line, "??") {
			info.Untracked++
			continue
		}

		if line[0] != ' ' {
			info.Staged++
		}
		if line[1] != ' ' {
			info.Unstaged++
		}
	}

	info.GitDirty = info.Staged > 0 || info.Unstaged > 0 || info.Untracked > 0
}
