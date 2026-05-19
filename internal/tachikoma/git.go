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

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		info := system.GetContextInfo()
		status := "git no detectado"
		if info.HasGit {
			status = info.GitSummary()
		}

		publish(Update{Name: g.Name(), Status: status})

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
