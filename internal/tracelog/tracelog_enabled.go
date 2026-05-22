//go:build motoko_trace

package tracelog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

var enabled atomic.Bool
var mu sync.Mutex

func Available() bool { return true }

func Enabled() bool { return enabled.Load() }

func SetEnabled(v bool) bool {
	enabled.Store(v)
	return true
}

func Path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "motoko-trace.txt"
	}
	return filepath.Join(home, ".local", "share", "motoko", "motoko-trace.txt")
}

func Logf(format string, args ...any) {
	if !Enabled() {
		return
	}
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
}
