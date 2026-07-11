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
	refresh := func() {
		baseInfo := system.GetContextInfo()
		gitInfo := system.GetGitInfo(baseInfo.Path)
		status := "git not detected"
		if gitInfo.HasGit {
			status = gitInfo.GitSummary()
		}

		publish(Update{Name: g.Name(), Status: status, Payload: gitInfo})
	}

	return runRefreshLoop(ctx, g.interval, []string{".", ".git"}, time.Second, refresh)
}
