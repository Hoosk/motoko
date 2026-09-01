package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/ui"
	"github.com/Hoosk/motoko/internal/updater"
	tea "github.com/charmbracelet/bubbletea"
)

var Version = "dev"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resume, question, handled, exitCode := parseFlags(ctx, os.Args[1:])
	if handled {
		os.Exit(exitCode)
	}

	runtimeObj := app.NewRuntime(app.RuntimeOptions{Resume: resume, Version: Version})

	if question != "" {
		runQuestion(ctx, runtimeObj, question)
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
}

// parseFlags processes the CLI arguments. handled reports whether main should
// exit immediately (help/version/update flows), in which case exitCode is the
// process exit code to use.
func parseFlags(ctx context.Context, args []string) (resume bool, question string, handled bool, exitCode int) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--resume":
			resume = true
		case "--version", "-v":
			fmt.Println(Version)
			return false, "", true, 0
		case "--help", "-h":
			printHelp()
			return false, "", true, 0
		case "-q", "--question":
			if i+1 < len(args) {
				question = args[i+1]
				i++
			} else {
				fmt.Fprintln(os.Stderr, "Error: -q/--question requires a query/prompt argument")
				return false, "", true, 1
			}
		case "--update":
			runUpdate(ctx)
			return false, "", true, 0
		case "--check-update":
			runCheckUpdate(ctx)
			return false, "", true, 0
		}
	}
	return resume, question, false, 0
}

func runUpdate(ctx context.Context) {
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
			return
		}
		fmt.Fprintf(os.Stderr, "Update failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Motoko has been updated successfully!")
}

func runCheckUpdate(ctx context.Context) {
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
}

// runQuestion executes a single prompt headlessly, streaming the result to
// stdout before exiting.
func runQuestion(ctx context.Context, runtimeObj *app.Runtime, question string) {
	if cfg := runtimeObj.Config(); cfg != nil && config.NormalizeEditApproval(cfg.EditApproval) == config.EditApprovalAsk {
		fmt.Fprintln(os.Stderr, "Error: --question cannot use edit_approval=ask; run interactively or set edit_approval=auto")
		os.Exit(1)
	}
	runtimeObj.Start(ctx)

	// Wait up to 2 seconds for Tachikomas to complete their first run
	startT := time.Now()
	for time.Since(startT) < 2*time.Second {
		_, gitDone := runtimeObj.Tachikomas().Query("GitTachikoma")
		_, codeDone := runtimeObj.Tachikomas().Query("CodeTachikoma")
		if gitDone && codeDone {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Detect and configure agent mode prefix if present (e.g. @search)
	question = applyAgentPrefix(runtimeObj, question)

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
			fmt.Printf("\n⚙️  Running tool: %s...\n", ev.Title)
		}
		return nil
	}

	info := runtimeObj.GetContextInfo()
	_, err := runtimeObj.RunAgentStream(ctx, info, question, onEvent)
	if err != nil {
		if err.Error() == "agente no configurado" {
			fmt.Fprintln(os.Stderr, "Error: Motoko agent is not configured. Run interactively first.")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if lastWasReasoning {
		fmt.Println()
	}
	fmt.Println()
}

// applyAgentPrefix switches the active agent when the prompt starts with an
// @agent mention and returns the prompt with that prefix stripped.
func applyAgentPrefix(runtimeObj *app.Runtime, question string) string {
	trimmedQ := strings.TrimSpace(question)
	fields := strings.Fields(trimmedQ)
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "@") {
		return question
	}
	agentName := strings.TrimPrefix(fields[0], "@")
	for _, name := range runtimeObj.AgentNames() {
		if strings.EqualFold(name, agentName) {
			runtimeObj.SetAgentMode(name)
			return strings.TrimSpace(strings.TrimPrefix(trimmedQ, fields[0]))
		}
	}
	return question
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
