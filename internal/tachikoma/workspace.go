package tachikoma

import (
	"context"
	"fmt"
	"time"

	"github.com/Hoosk/motoko/internal/system"
)

type WorkspaceTachikoma struct {
	interval time.Duration
}

func NewWorkspaceTachikoma(interval time.Duration) *WorkspaceTachikoma {
	return &WorkspaceTachikoma{interval: interval}
}

func (w *WorkspaceTachikoma) Name() string {
	return "WorkspaceTachikoma"
}

func (w *WorkspaceTachikoma) Run(ctx context.Context, publish func(Update) bool) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		info := system.GetContextInfo()
		publish(Update{Name: w.Name(), Status: fmt.Sprintf("workspace %s", info.Workspace)})

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
