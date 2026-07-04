package taskman

import (
	"context"
	"time"
)

type TaskState struct {
	Started  time.Time
	cancel   context.CancelFunc
	ID       string
	Command  string
	Output   string
	ExitCode int
	Running  bool
}
