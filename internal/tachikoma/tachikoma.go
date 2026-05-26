package tachikoma

import (
	"context"
	"sync"

	"github.com/Hoosk/motoko/internal/semantic"
	"github.com/Hoosk/motoko/internal/system"
)

const updatesBufferSize = 32

// Update represents a status update from a Tachikoma
type Update struct {
	Name    string
	Status  string
	Done    bool
	Payload interface{}
}

// Tachikoma is the interface for background context gatherers
type Tachikoma interface {
	Name() string
	Run(ctx context.Context, publish func(Update) bool) error
}

// Manager coordinates multiple Tachikomas
type Manager struct {
	tachikomas []Tachikoma
	updates    chan Update
	wg         sync.WaitGroup
	mu         sync.RWMutex
	state      map[string]Update
}

func NewManager() *Manager {
	return &Manager{
		tachikomas: []Tachikoma{},
		updates:    make(chan Update, updatesBufferSize),
		state:      make(map[string]Update),
	}
}

func (m *Manager) Add(t Tachikoma) {
	m.tachikomas = append(m.tachikomas, t)
}

func (m *Manager) Start(ctx context.Context) {
	for _, t := range m.tachikomas {
		m.wg.Add(1)
		go func(t Tachikoma) {
			defer m.wg.Done()
			_ = t.Run(ctx, func(u Update) bool {
				m.mu.Lock()
				m.state[u.Name] = u
				m.mu.Unlock()
				return m.publishUpdate(u)
			})
		}(t)
	}
}

func (m *Manager) GetContextInfo() system.ContextInfo {
	info := system.GetContextInfo() // Base info

	m.mu.RLock()
	defer m.mu.RUnlock()

	if info.Signals == nil {
		info.Signals = make(map[string]string)
	}
	if info.OnDemandSignals == nil {
		info.OnDemandSignals = make(map[string]string)
	}

	for _, update := range m.state {
		switch update.Name {
		case "GitTachikoma":
			info.Signals[update.Name] = update.Status
		case "DiffTachikoma":
			if diff, ok := update.Payload.(SemanticDiff); ok && len(diff.Files) > 0 {
				info.OnDemandSignals[update.Name] = "Recent changes mapped to functions/symbols are available."
				info.Signals[update.Name] = update.Status
			}
		case "CodeTachikoma":
			if snapshot, ok := update.Payload.(*semantic.Snapshot); ok && snapshot != nil {
				// Sharding: Don't put the full summary in the prompt if it's too large.
				fullSummary := snapshot.Summary()
				if len(fullSummary) > 500 {
					info.OnDemandSignals[update.Name] = "Detailed semantic index of the codebase is ready."
					info.SemanticSummary = "Codebase indexed (heavy). Use search/read tools for details."
				} else {
					info.SemanticSummary = fullSummary
					info.Signals[update.Name] = update.Status
				}

				// Provide recently changed files as "relevant" for the sidebar if empty
				if len(info.RelevantFiles) == 0 && len(snapshot.ChangedPaths) > 0 {
					for _, path := range snapshot.ChangedPaths {
						info.RelevantFiles = append(info.RelevantFiles, path)
						if len(info.RelevantFiles) >= 5 {
							break
						}
					}
				}
			}
		default:
			info.Signals[update.Name] = update.Status
		}
	}

	return info
}

func (m *Manager) Wait() {
	m.wg.Wait()
}

// Query returns the detailed payload or status of a specific Tachikoma by name.
func (m *Manager) Query(name string) (Update, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	update, ok := m.state[name]
	return update, ok
}

type NextResult struct {
	Update Update
	OK     bool
}

func (m *Manager) Next(ctx context.Context) NextResult {
	if m == nil {
		return NextResult{}
	}
	select {
	case <-ctx.Done():
		return NextResult{}
	case update := <-m.updates:
		return NextResult{Update: update, OK: true}
	}
}

func (m *Manager) publishUpdate(update Update) bool {
	select {
	case m.updates <- update:
		return true
	default:
		return false
	}
}

func (m *Manager) Updates() <-chan Update {
	return m.updates
}
