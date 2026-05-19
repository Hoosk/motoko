package tachikoma

import (
	"context"
	"time"
)

type MockTachikoma struct {
	name string
}

func NewMockTachikoma(name string) *MockTachikoma {
	return &MockTachikoma{name: name}
}

func (m *MockTachikoma) Name() string {
	return m.name
}

func (m *MockTachikoma) Run(ctx context.Context, publish func(Update) bool) error {
	publish(Update{Name: m.name, Status: "Iniciando escaneo...", Done: false})
	
	select {
	case <-time.After(2 * time.Second):
		publish(Update{Name: m.name, Status: "Analizando archivos...", Done: false})
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case <-time.After(3 * time.Second):
		publish(Update{Name: m.name, Status: "Listo", Done: true})
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}
