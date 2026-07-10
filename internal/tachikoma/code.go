package tachikoma

import (
	"context"
	"time"

	"github.com/Hoosk/motoko/internal/semantic"
)

type CodeTachikoma struct {
	index    *semantic.Index
	interval time.Duration
}

func NewCodeTachikoma(index *semantic.Index, interval time.Duration) *CodeTachikoma {
	return &CodeTachikoma{interval: interval, index: index}
}

func (c *CodeTachikoma) Name() string {
	return "CodeTachikoma"
}

func (c *CodeTachikoma) Run(ctx context.Context, publish func(Update) bool) error {
	refresh := func() {
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
		publish(Update{Name: c.Name(), Status: status, Payload: payload})
	}

	return runRefreshLoop(ctx, c.interval, []string{"."}, 500*time.Millisecond, refresh)
}
