package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/tachikoma"
	"github.com/Hoosk/motoko/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
	}()

	resume := false
	for _, arg := range os.Args[1:] {
		if arg == "--resume" {
			resume = true
		}
	}
	runtime := app.NewRuntime(app.RuntimeOptions{Resume: resume})

	mgr := newTachikomaManager(runtime)

	// Create UI Model
	m := ui.NewModel(runtime, cancel, ctx)
	m.SetManager(mgr)

	// Start Tachikomas in the background
	mgr.Start(ctx)

	// Start Bubble Tea program
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		cancel()
		mgr.Wait()
		fmt.Printf("Error al iniciar Motoko: %v", err)
		os.Exit(1)
	}
	cancel()
	mgr.Wait()
}

func newTachikomaManager(runtime *app.Runtime) *tachikoma.Manager {
	mgr := tachikoma.NewManager()
	mgr.Add(tachikoma.NewWorkspaceTachikoma(30 * time.Second))
	mgr.Add(tachikoma.NewGitTachikoma(5 * time.Second))
	mgr.Add(tachikoma.NewCodeTachikoma(runtime.SemanticIndex(), 20*time.Second))
	return mgr
}
