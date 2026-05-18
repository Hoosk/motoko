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

func (g *GitTachikoma) Run(ctx context.Context, updates chan<- Update) error {
	ticker := time.NewTicker(g.interval)
	defer ticker.Stop()

	for {
		info := system.GetContextInfo()
		status := "git no detectado"
		if info.HasGit {
			status = info.GitSummary()
		}

		updates <- Update{Name: g.Name(), Status: status}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
