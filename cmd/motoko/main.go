package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Hoosk/motoko/internal/app"
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

	// Start Tachikomas in the background via Runtime
	runtime.Start(ctx)

	// Create UI Model
	m := ui.NewModel(runtime)

	// Start Bubble Tea program
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		cancel()
		fmt.Printf("Error al iniciar Motoko: %v", err)
		os.Exit(1)
	}
	cancel()
}
