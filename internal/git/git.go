// Package git runs read-only git commands for background workers.
//
// Every command runs with GIT_OPTIONAL_LOCKS=0 so that polling never takes
// .git/index.lock (which would race with the user's own git commands and
// generate filesystem events that retrigger the watchers), and with a
// timeout so a hung git cannot block a worker forever.
package git

import (
	"context"
	"os"
	"os/exec"
	"time"
)

// DefaultTimeout bounds commands whose context carries no deadline.
const DefaultTimeout = 10 * time.Second

// Output runs git with args in dir and returns its stdout.
func Output(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultTimeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	return cmd.Output()
}
