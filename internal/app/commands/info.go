package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/app/shell"
	"github.com/Hoosk/motoko/internal/app/types"
	"github.com/Hoosk/motoko/internal/session"
	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/system"
)

func (d *Dispatcher) handleThemesCommand(inv Invocation) types.Response {
	parts := append([]string{"themes"}, inv.Args...)
	if len(parts) < 2 {
		current := d.deps.ThemeFn()
		if current == "" {
			current = DefaultTheme
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf(
			"Current theme: %s\n"+
				"Available themes:\n"+
				"  cyberpunk    Default dark neon green (default)\n"+
				"  ghost-cyber  Restrained dark cyberpunk with precise accents\n"+
				"  neon-shadow  Dramatic high-contrast magenta and cyan\n"+
				"  black-ice    Cold technical with ice-blue accents\n"+
				"  nord         Arctic blue palette\n"+
				"  dracula      Classic purple and green\n"+
				"  monochrome   Pure green-on-black terminal\n"+
				"Usage: /themes <name>",
			current)}}}
	}
	themeName := strings.ToLower(parts[1])
	switch themeName {
	case ThemeCyberpunk, "ghost-cyber", "neon-shadow", "black-ice", "nord", "dracula", "monochrome":
		if err := d.deps.SetThemeFn(themeName); err != nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: err.Error()}}}
		}
		styles.SetTheme(themeName)
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Theme changed to: " + themeName}}}
	default:
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Unknown theme: %s. Available: cyberpunk, ghost-cyber, neon-shadow, black-ice, nord, dracula, monochrome", themeName)}}}
	}
}

func (d *Dispatcher) handleClearCommand() types.Response {
	if sess := d.deps.SessionFn(); sess != nil {
		sess.History = nil
		sess.LastInputTokens = 0
		_ = d.deps.SaveSessionFn()
	}
	return types.Response{Clear: true, Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Timeline reset."}}}
}

func (d *Dispatcher) handleShell(command string) types.Response {
	return shell.PrepareCommand(d.deps.ModeFn(), command)
}

func (d *Dispatcher) statusText(info system.ContextInfo) string {
	pending := ValNone
	if d.deps.PendingDialogsFn != nil {
		if count := d.deps.PendingDialogsFn(); count > 0 {
			pending = fmt.Sprintf("%d dialog(s)", count)
		}
	}

	agentsStatus := "not found"
	if info.Path != "" {
		if _, err := os.Stat(filepath.Join(info.Path, "AGENTS.md")); err == nil {
			agentsStatus = "loaded"
		}
	}

	designStatus := "not found"
	if info.Path != "" {
		if _, err := os.Stat(filepath.Join(info.Path, "DESIGN.md")); err == nil {
			designStatus = "loaded"
		}
	}

	return strings.Join([]string{
		fmt.Sprintf("mode: %s", d.deps.ModeFn()),
		fmt.Sprintf("input: %s", d.deps.InputModeFn()),
		fmt.Sprintf("agent configured: %t", d.deps.AgentConfiguredFn()),
		fmt.Sprintf("active provider: %s", d.deps.ProvMgr.ProviderSummary()),
		fmt.Sprintf("workspace: %s", info.Workspace),
		fmt.Sprintf("git: %s", info.GitSummary()),
		fmt.Sprintf("agents.md guidelines: %s", agentsStatus),
		fmt.Sprintf("design.md specification: %s", designStatus),
		fmt.Sprintf("pending approval: %s", pending),
		"policy: plan asks for shell approval; build runs safe commands and asks for sensitive ones.",
	}, "\n")
}

func (d *Dispatcher) handleTaskCommand(parts []string) types.Response {
	if len(parts) < 2 || strings.EqualFold(parts[1], CmdList) {
		return d.formatTaskList()
	}
	subcmd := strings.ToLower(parts[1])
	switch subcmd {
	case "terminate":
		if len(parts) < 3 {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /task terminate <idTask>"}}}
		}
		id := parts[2]
		if err := d.deps.TerminateTaskFn(id); err != nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: err.Error()}}}
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("Task %s terminated.", id)}}}
	default:
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Unknown subcommand: %s. Usage: /task or /task terminate <idTask>", subcmd)}}}
	}
}

func (d *Dispatcher) formatTaskList() types.Response {
	tasks := d.deps.ListTasksFn()
	if len(tasks) == 0 {
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "No active background tasks."}}}
	}
	var sb strings.Builder
	sb.WriteString("Active tasks:\n")
	for _, t := range tasks {
		fmt.Fprintf(&sb, "- %s: %q (started %s ago)\n", t.ID, t.Command, time.Since(t.Started).Round(time.Second))
	}
	return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: strings.TrimSpace(sb.String())}}}
}

func (d *Dispatcher) handleMetricsCommand() types.Response {
	sess := d.deps.SessionFn()
	if sess == nil {
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "No active session."}}}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Current Session Metrics (%s):\n", sess.ID)
	fmt.Fprintf(&sb, "- Created at: %s\n", sess.CreatedAt.Local().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&sb, "- History Messages: %d\n", len(sess.History))

	writeTokenBreakdown(&sb, "Last Turn Token Usage", sess.LastInputTokens, sess.LastSystemStaticTokens, sess.LastSystemDynamicTokens, sess.LastToolsTokens, sess.LastHistoryTokens, sess.LastCacheReadTokens, sess.LastOutputTokens, sess.LastReasoningTokens, sess.LastCacheWriteTokens)
	writeTokenBreakdown(&sb, "Cumulative Token Usage", sess.TotalInputTokens, sess.TotalSystemStaticTokens, sess.TotalSystemDynamicTokens, sess.TotalToolsTokens, sess.TotalHistoryTokens, sess.TotalCacheReadTokens, sess.TotalOutputTokens, sess.TotalReasoningTokens, sess.TotalCacheWriteTokens)
	fmt.Fprintf(&sb, "- Total Tokens:  %d\n", sess.TotalTokens)

	if len(sess.Turns) > 0 {
		sb.WriteString("\nRecent Turn Trend:\n")
		for _, turn := range sess.Turns {
			fmt.Fprintf(&sb, "- Turn %d", turn.Turn)
			if turn.AgentLabel != "" {
				fmt.Fprintf(&sb, " [%s]", turn.AgentLabel)
			}
			fmt.Fprintf(&sb, ": in=%d out=%d reasoning=%d total=%d", turn.InputTokens, turn.OutputTokens, turn.ReasoningTokens, turn.TotalTokens)
			if turn.InputGrowth != 0 {
				growthPct := growthPercentage(&turn)
				fmt.Fprintf(&sb, " input_growth=%+d (%.1f%%)", turn.InputGrowth, growthPct)
				if turn.InputGrowth > 0 && growthPct >= 15.0 {
					sb.WriteString(" BLOAT")
				}
			}
			if turn.CacheReadTokens > 0 || turn.CacheWriteTokens > 0 {
				fmt.Fprintf(&sb, " cache=%d/%d", turn.CacheReadTokens, turn.CacheWriteTokens)
			}
			sb.WriteByte('\n')
			for idx, iter := range turn.Iterations {
				fmt.Fprintf(&sb, "  iter %d: in=%d out=%d reasoning=%d total=%d", idx+1, iter.InputTokens, iter.OutputTokens, iter.ReasoningTokens, iter.TotalTokens)
				if idx > 0 {
					delta := iter.InputTokens - turn.Iterations[idx-1].InputTokens
					fmt.Fprintf(&sb, " input_delta=%+d", delta)
				}
				if iter.CacheReadInputTokens > 0 || iter.CacheWriteInputTokens > 0 {
					fmt.Fprintf(&sb, " cache=%d/%d", iter.CacheReadInputTokens, iter.CacheWriteInputTokens)
				}
				sb.WriteByte('\n')
			}
		}
	}
	return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: sb.String()}}}
}

func writeTokenBreakdown(sb *strings.Builder, label string, input, static, dynamic, toolDefs, history, cacheRead, output, reasoning, cacheWrite int) {
	fmt.Fprintf(sb, "\n%s:\n", label)
	fmt.Fprintf(sb, "- Input Tokens: %d\n", input)
	if input > 0 {
		fmt.Fprintf(sb, "  * System Prompt (Static):  %d (%.1f%% of input)\n", static, percentage(static, input))
		fmt.Fprintf(sb, "  * System Prompt (Dynamic): %d (%.1f%% of input)\n", dynamic, percentage(dynamic, input))
		fmt.Fprintf(sb, "  * Tool Definitions:       %d (%.1f%% of input)\n", toolDefs, percentage(toolDefs, input))
		fmt.Fprintf(sb, "  * History & Query:        %d (%.1f%% of input)\n", history, percentage(history, input))
	}
	if input > 0 && cacheRead > 0 {
		fmt.Fprintf(sb, "  * Cache Read:  %d (%.1f%% of input)\n", cacheRead, percentage(cacheRead, input))
	}
	if cacheWrite > 0 {
		fmt.Fprintf(sb, "  * Cache Write: %d\n", cacheWrite)
	}
	fmt.Fprintf(sb, "- Output Tokens: %d\n", output)
	if output > 0 && reasoning > 0 {
		fmt.Fprintf(sb, "  * Reasoning (Thinking) Tokens: %d (%.1f%% of output)\n", reasoning, percentage(reasoning, output))
	}
}

func percentage(value, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(value) / float64(total) * 100
}

func growthPercentage(turn *session.TurnUsage) float64 {
	if turn == nil || len(turn.Iterations) == 0 {
		return 0
	}
	return percentage(turn.InputGrowth, turn.Iterations[0].InputTokens)
}
