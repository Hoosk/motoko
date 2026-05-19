package tachikoma

import (
	"context"
	"sync"
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
}

func NewManager() *Manager {
	return &Manager{
		tachikomas: []Tachikoma{},
		updates:    make(chan Update, updatesBufferSize),
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
			_ = t.Run(ctx, m.publishUpdate)
		}(t)
	}
}

func (m *Manager) Wait() {
	m.wg.Wait()
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
