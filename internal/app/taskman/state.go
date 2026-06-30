package taskman

import (
	"context"
	"time"
)

type TaskState struct {
	Started  time.Time
	ID       string
	Command  string
	Output   string
	ExitCode int
	Running  bool

	cancel context.CancelFunc
}
