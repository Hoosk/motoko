package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/styles"
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
	question := ""
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch arg {
		case "--resume":
			resume = true
		case "--version", "-v":
			fmt.Println(Version)
			os.Exit(0)
		case "--help", "-h":
			printHelp()
			os.Exit(0)
		case "-q", "--question":
			if i+1 < len(os.Args) {
				question = os.Args[i+1]
				i++
			} else {
				fmt.Fprintln(os.Stderr, "Error: -q/--question requires a query/prompt argument")
				os.Exit(1)
			}
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

	runtimeObj := app.NewRuntime(app.RuntimeOptions{Resume: resume, Version: Version})

	if question != "" {
		runtimeObj.Start(ctx)

		var lastWasReasoning bool
		onEvent := func(ev app.AgentStreamEvent) error {
			switch ev.Kind {
			case "thinking_delta":
				if !lastWasReasoning {
					fmt.Print(styles.ReasoningBlockStyle.Render("Thinking: "))
					lastWasReasoning = true
				}
				fmt.Print(styles.ReasoningBlockStyle.Render(ev.ReasoningContent))
			case "assistant_delta":
				if lastWasReasoning {
					fmt.Println()
					lastWasReasoning = false
				}
				fmt.Print(ev.Content)
			case "tool":
				if lastWasReasoning {
					fmt.Println()
					lastWasReasoning = false
				}
				fmt.Printf("\n⚙️  Running tool: %s\n", styles.CommandStyle.Render(ev.Title))
				if strings.TrimSpace(ev.Content) != "" {
					args := strings.TrimSpace(ev.Content)
					if strings.Contains(args, "\n") {
						for _, line := range strings.Split(args, "\n") {
							fmt.Printf("   %s\n", styles.SystemStyle.Render(line))
						}
					} else {
						fmt.Printf("   %s\n", styles.SystemStyle.Render(args))
					}
				}
			case "error":
				if lastWasReasoning {
					fmt.Println()
					lastWasReasoning = false
				}
				fmt.Printf("❌ %s\n", styles.ErrorStyle.Render(ev.Content))
			case "output":
				if lastWasReasoning {
					fmt.Println()
					lastWasReasoning = false
				}
				if ev.Title == "web_search" || ev.Title == "web_fetch" {
					fmt.Printf("✓ Tool finished: %s (%d characters)\n", styles.CommandStyle.Render(ev.Title), len(ev.Content))
				} else {
					lines := strings.Split(strings.TrimSpace(ev.Content), "\n")
					if len(lines) > 5 {
						fmt.Printf("✓ Tool finished: %s (%d lines, %d bytes)\n", styles.CommandStyle.Render(ev.Title), len(lines), len(ev.Content))
					} else {
						fmt.Printf("✓ Tool finished: %s\n", styles.CommandStyle.Render(ev.Title))
						for _, line := range lines {
							if line != "" {
								fmt.Printf("   %s\n", styles.SystemStyle.Render(line))
							}
						}
					}
				}
			case "task_started":
				if lastWasReasoning {
					fmt.Println()
					lastWasReasoning = false
				}
				fmt.Printf("\n⚙️  Starting task: %s\n", styles.CommandStyle.Render(ev.Title))
			case "task_finished":
				if lastWasReasoning {
					fmt.Println()
					lastWasReasoning = false
				}
				fmt.Printf("✓ Task finished: %s\n", styles.CommandStyle.Render(ev.Title))
				if strings.TrimSpace(ev.Content) != "" {
					fmt.Printf("   %s\n", styles.SystemStyle.Render(ev.Content))
				}
			}
			return nil
		}

		info := runtimeObj.GetContextInfo()
		_, err := runtimeObj.RunAgentStream(ctx, info, question, onEvent)
		if err != nil {
			if err.Error() == "agente no configurado" {
				fmt.Fprintln(os.Stderr, "Error: Motoko agent is not configured.")
				fmt.Fprintln(os.Stderr, "Please run 'motoko' without flags first to configure your provider and model.")
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "Error running agent: %v\n", err)
			os.Exit(1)
		}
		if lastWasReasoning {
			fmt.Println()
		}
		fmt.Println()
		os.Exit(0)
	}

	// Start Tachikomas in the background via Runtime
	runtimeObj.Start(ctx)

	// Create UI Model
	m := ui.NewModel(runtimeObj)

	// Start Bubble Tea program
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		cancel()
		fmt.Printf("Error al iniciar Motoko: %v", err)
		os.Exit(1)
	}
	cancel()
}

func printHelp() {
	title := styles.HeaderStyle.Render("Motoko - AI Terminal Assistant")
	meta := styles.HeaderMetaStyle.Render("Section 9 Operative CLI")

	fmt.Println(title)
	fmt.Println(meta)
	fmt.Println()
	fmt.Println(styles.CommandStyle.Render("Usage:"))
	fmt.Println("  motoko [options]")
	fmt.Println()
	fmt.Println(styles.CommandStyle.Render("Options:"))
	fmt.Printf("  %-25s %s\n", "-q, --question <prompt>", "Run a prompt directly, stream the result, and exit.")
	fmt.Printf("  %-25s %s\n", "--resume", "Resume the last active chat session (can be combined with -q).")
	fmt.Printf("  %-25s %s\n", "-v, --version", "Print version.")
	fmt.Printf("  %-25s %s\n", "-h, --help", "Show this help menu.")
	fmt.Printf("  %-25s %s\n", "--update", "Check and install update.")
	fmt.Printf("  %-25s %s\n", "--check-update", "Check for new updates.")
	fmt.Println()
	fmt.Println("If run without arguments (or with only --resume), Motoko starts in interactive TUI mode.")
}
