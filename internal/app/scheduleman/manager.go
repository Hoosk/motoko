package scheduleman

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const emitTimeout = 2 * time.Second

type Definition struct {
	ID          string        `json:"id"`
	Instruction string        `json:"instruction"`
	Interval    time.Duration `json:"interval"`
	OneShot     bool          `json:"one_shot,omitempty"`
}

type Event struct {
	FiredAt     time.Time
	ID          string
	Instruction string
	OneShot     bool
}

type EventResult struct {
	Event Event
	OK    bool
}

type entry struct {
	cancel context.CancelFunc
	def    Definition
}

type Manager struct {
	baseCtx   context.Context
	schedules map[string]*entry
	events    chan Event
	onChange  func([]Definition)
	nextID    int
	mu        sync.Mutex
}

func NewManager() *Manager {
	return &Manager{
		schedules: make(map[string]*entry),
		events:    make(chan Event, 32),
		nextID:    1,
	}
}

func (m *Manager) SetOnChange(fn func([]Definition)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChange = fn
}

func (m *Manager) AttachContext(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.baseCtx = ctx
	for _, sched := range m.schedules {
		if sched.cancel != nil {
			sched.cancel()
			sched.cancel = nil
		}
		m.startLocked(sched)
	}
}

func (m *Manager) Add(instruction string, interval time.Duration, oneShot bool) (Definition, error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return Definition{}, fmt.Errorf("instruction cannot be empty")
	}
	if interval <= 0 {
		return Definition{}, fmt.Errorf("interval must be greater than zero")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	def := Definition{
		ID:          fmt.Sprintf("sched-%d", m.nextID),
		Instruction: instruction,
		Interval:    interval,
		OneShot:     oneShot,
	}
	m.nextID++
	item := &entry{def: def}
	m.schedules[def.ID] = item
	m.startLocked(item)
	m.notifyLocked()
	return def, nil
}

func (m *Manager) Restore(defs []Definition) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, sched := range m.schedules {
		if sched.cancel != nil {
			sched.cancel()
		}
	}
	m.schedules = make(map[string]*entry, len(defs))
	maxID := 0
	for _, def := range defs {
		if strings.TrimSpace(def.ID) == "" || strings.TrimSpace(def.Instruction) == "" || def.Interval <= 0 {
			continue
		}
		item := &entry{def: def}
		m.schedules[def.ID] = item
		m.startLocked(item)
		if n, err := parseNumericSuffix(def.ID); err == nil && n > maxID {
			maxID = n
		}
	}
	if maxID > 0 {
		m.nextID = maxID + 1
	} else {
		m.nextID = 1
	}
	m.notifyLocked()
}

func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.schedules[strings.TrimSpace(id)]
	if !ok {
		return fmt.Errorf("schedule not found: %s", id)
	}
	if item.cancel != nil {
		item.cancel()
	}
	delete(m.schedules, item.def.ID)
	m.notifyLocked()
	return nil
}

func (m *Manager) List() []Definition {
	m.mu.Lock()
	defer m.mu.Unlock()
	defs := make([]Definition, 0, len(m.schedules))
	for _, sched := range m.schedules {
		defs = append(defs, sched.def)
	}
	sort.Slice(defs, func(i, j int) bool {
		return defs[i].ID < defs[j].ID
	})
	return defs
}

func (m *Manager) Next(ctx context.Context) EventResult {
	select {
	case ev := <-m.events:
		return EventResult{Event: ev, OK: true}
	case <-ctx.Done():
		return EventResult{}
	}
}

func (m *Manager) startLocked(item *entry) {
	if item == nil || item.cancel != nil || m.baseCtx == nil {
		return
	}
	ctx, cancel := context.WithCancel(m.baseCtx)
	item.cancel = cancel
	go m.runSchedule(ctx, item.def)
}

func (m *Manager) runSchedule(ctx context.Context, def Definition) {
	if def.OneShot {
		timer := time.NewTimer(def.Interval)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if m.emit(ctx, def) {
				m.removeAfterFire(def.ID)
			}
			return
		}
	}

	ticker := time.NewTicker(def.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !m.emit(ctx, def) {
				return
			}
		}
	}
}

func (m *Manager) emit(ctx context.Context, def Definition) bool {
	timer := time.NewTimer(emitTimeout)
	defer timer.Stop()
	select {
	case m.events <- Event{ID: def.ID, Instruction: def.Instruction, FiredAt: time.Now(), OneShot: def.OneShot}:
		return true
	case <-ctx.Done():
		return false
	case <-timer.C:
		return false
	}
}

func (m *Manager) removeAfterFire(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.schedules[id]
	if !ok {
		return
	}
	item.cancel = nil
	delete(m.schedules, id)
	m.notifyLocked()
}

func (m *Manager) notifyLocked() {
	if m.onChange == nil {
		return
	}
	defs := make([]Definition, 0, len(m.schedules))
	for _, sched := range m.schedules {
		defs = append(defs, sched.def)
	}
	sort.Slice(defs, func(i, j int) bool {
		return defs[i].ID < defs[j].ID
	})
	m.onChange(defs)
}

func parseNumericSuffix(id string) (int, error) {
	parts := strings.Split(strings.TrimSpace(id), "-")
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid schedule id")
	}
	return strconv.Atoi(parts[len(parts)-1])
}
