package tachikoma

import "context"

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
	Run(ctx context.Context, updates chan<- Update) error
}

// Manager coordinates multiple Tachikomas
type Manager struct {
	tachikomas []Tachikoma
	updates    chan Update
}

func NewManager() *Manager {
	return &Manager{
		tachikomas: []Tachikoma{},
		updates:    make(chan Update, 10),
	}
}

func (m *Manager) Add(t Tachikoma) {
	m.tachikomas = append(m.tachikomas, t)
}

func (m *Manager) Start(ctx context.Context) {
	for _, t := range m.tachikomas {
		go func(t Tachikoma) {
			_ = t.Run(ctx, m.updates)
		}(t)
	}
}

func (m *Manager) Updates() <-chan Update {
	return m.updates
}
