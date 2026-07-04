package taskman

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/Hoosk/motoko/internal/app/types"
)

const taskTimeout = 20 * time.Minute

type Manager struct {
	active map[string]*TaskState
	events chan types.TaskEvent
	seq    int
	mu     sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		active: make(map[string]*TaskState),
		events: make(chan types.TaskEvent, 64),
	}
}

func (m *Manager) Launch(ctx context.Context, command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", fmt.Errorf("empty command")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	m.mu.Lock()
	m.seq++
	id := fmt.Sprintf("task-%d", m.seq)

	cmdCtx, cancel := context.WithTimeout(ctx, taskTimeout)
	state := &TaskState{
		ID:      id,
		Command: command,
		Started: time.Now(),
		Running: true,
		cancel:  cancel,
	}
	m.active[id] = state
	m.mu.Unlock()

	m.publish(types.TaskEvent{ID: id, Command: command, Done: false})

	go func() {
		start := time.Now()
		defer cancel()
		cmd := exec.CommandContext(cmdCtx, "bash", "-lc", command)
		cmd.Dir = wd
		output, runErr := cmd.CombinedOutput()
		trimmed := strings.TrimSpace(string(output))
		exitCode := 0
		if runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else if cmdCtx.Err() == context.DeadlineExceeded {
				exitCode = 124
			} else if cmdCtx.Err() == context.Canceled {
				exitCode = 130
				if trimmed == "" {
					trimmed = "Command terminated by the user or agent."
				}
			} else {
				exitCode = -1
				if trimmed == "" {
					trimmed = runErr.Error()
				}
			}
		}
		if cmdCtx.Err() == context.DeadlineExceeded {
			if trimmed != "" {
				trimmed += "\n"
			}
			trimmed += "Command canceled due to timeout."
		} else if cmdCtx.Err() == context.Canceled {
			if trimmed != "" {
				trimmed += "\n"
			}
			trimmed += "Command terminated."
		}

		m.mu.Lock()
		delete(m.active, id)
		m.mu.Unlock()

		m.publish(types.TaskEvent{ID: id, Command: command, Done: true, ExitCode: exitCode, Output: trimmed, Duration: time.Since(start)})
	}()

	return id, nil
}

func (m *Manager) Terminate(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.active[id]
	if !ok {
		return fmt.Errorf("task not found: %s", id)
	}
	if !state.Running {
		return fmt.Errorf("task %s has already finished", id)
	}
	if state.cancel != nil {
		state.cancel()
	}
	return nil
}

func (m *Manager) List() []*TaskState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]*TaskState, 0, len(m.active))
	for _, state := range m.active {
		list = append(list, state)
	}
	return list
}

func (m *Manager) Next(ctx context.Context) types.TaskEventResult {
	if m == nil {
		return types.TaskEventResult{}
	}
	select {
	case <-ctx.Done():
		return types.TaskEventResult{}
	case ev := <-m.events:
		return types.TaskEventResult{Event: ev, OK: true}
	}
}

func (m *Manager) ActiveTasks() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.active)
}

func (m *Manager) publish(event types.TaskEvent) {
	select {
	case m.events <- event:
	default:
	}
}
