package main

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/ui"
	"github.com/Hoosk/motoko/internal/updater"
	tea "github.com/charmbracelet/bubbletea"
)

var Version = "dev"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
	}()

	resume := false
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--resume":
			resume = true
		case "--version", "-v":
			fmt.Println(Version)
			os.Exit(0)
		case "--update":
			upd := updater.NewUpdater(updater.Config{
				CurrentVersion: Version,
				GOOS:           runtime.GOOS,
				GOARCH:         runtime.GOARCH,
			})
			fmt.Println("Checking for updates...")
			err := upd.Update(ctx)
			if err != nil {
				if err == updater.ErrNoUpdateAvailable {
					fmt.Printf("Motoko is already up to date (%s)\n", Version)
					os.Exit(0)
				}
				fmt.Fprintf(os.Stderr, "Update failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Motoko has been updated successfully!")
			os.Exit(0)
		case "--check-update":
			upd := updater.NewUpdater(updater.Config{
				CurrentVersion: Version,
				GOOS:           runtime.GOOS,
				GOARCH:         runtime.GOARCH,
			})
			fmt.Println("Checking for updates...")
			info, err := upd.CheckVersion(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Check failed: %v\n", err)
				os.Exit(1)
			}
			if info.IsNewer {
				fmt.Printf("New version available: %s\nRun 'motoko --update' to update.\n", info.NewVersion)
			} else {
				fmt.Printf("Motoko is up to date (%s)\n", Version)
			}
			os.Exit(0)
		}
	}
	runtime := app.NewRuntime(app.RuntimeOptions{Resume: resume, Version: Version})

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
