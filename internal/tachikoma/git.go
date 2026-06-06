package tachikoma

import (
	"context"
	"time"

	"github.com/Hoosk/motoko/internal/system"
)

type GitTachikoma struct {
	interval time.Duration
}

func NewGitTachikoma(interval time.Duration) *GitTachikoma {
	return &GitTachikoma{interval: interval}
}

func (g *GitTachikoma) Name() string {
	return "GitTachikoma"
}

func (g *GitTachikoma) Run(ctx context.Context, publish func(Update) bool) error {
	ticker := time.NewTicker(g.interval)
	defer ticker.Stop()

	// Watch for git changes and workspace changes recursively
	events, _ := WatchHelper(ctx, []string{".", ".git"}, 1*time.Second)

	refresh := func() {
		baseInfo := system.GetContextInfo()
		gitInfo := system.GetGitInfo(baseInfo.Path)
		status := "git not detected"
		if gitInfo.HasGit {
			status = gitInfo.GitSummary()
		}

		publish(Update{Name: g.Name(), Status: status, Payload: gitInfo})
	}

	// Initial refresh
	refresh()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			refresh()
		case <-events:
			refresh()
		}
	}
}
