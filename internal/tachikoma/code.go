package tachikoma

import (
	"context"
	"time"

	"github.com/Hoosk/motoko/internal/semantic"
)

type CodeTachikoma struct {
	interval time.Duration
	index    *semantic.Index
}

func NewCodeTachikoma(index *semantic.Index, interval time.Duration) *CodeTachikoma {
	return &CodeTachikoma{interval: interval, index: index}
}

func (c *CodeTachikoma) Name() string {
	return "CodeTachikoma"
}

func (c *CodeTachikoma) Run(ctx context.Context, updates chan<- Update) error {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		status := "semantic index unavailable"
		var payload any
		if c.index != nil {
			snapshot, err := c.index.Refresh(ctx)
			if err != nil {
				status = err.Error()
			} else {
				status = snapshot.Summary()
				payload = snapshot
			}
		}
		updates <- Update{Name: c.Name(), Status: status, Payload: payload}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
