package agent

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tools"
)

// buildSystemPrompt assembles the complete system prompt for the given context,
// available tools, and active agent mode.
func buildSystemPrompt(providerKind string, info system.ContextInfo, specs []tools.Spec, agentSystem string) string {
	var lines []string
	
	// --- STATIC PART ---
	
	header := system.LoadProviderHeader(providerKind)
	lines = append(lines, header)
	lines = append(lines, "")

	lines = append(lines,
		"<system_instructions>",
		"  <general>",
		"    When answering the user, write plain text directly so it can stream cleanly to the terminal.",
		"    When you need a tool, use the provider's native tool/function call mechanism instead of printing JSON that describes a tool call.",
		"    Use tools only to explore parts of the codebase NOT already covered by the provided context.",
		"    Parallelize tool calls whenever possible (e.g. independent file reads, searches). You can request multiple tool calls in a single response to significantly improve performance.",
		"    DO NOT invent file names, functions, or command outputs.",
		"    Prefer finishing the task end-to-end instead of stopping at analysis.",
		"    If the existing context already answers the question, answer directly without unnecessary tool calls.",
		"    You have a 'delegate' tool. If a task contains independent research or sub-problems, delegate them to subagents (like 'search' or 'plan') to run in parallel in the background, rather than doing everything sequentially yourself.",
		"  </general>",
		"",
		"  <tachikomas>",
		"    Tachikomas are background workers that gather codebase context dynamically to minimize token consumption and keep your context window high-signal.",
		"    Always check '[Background Signals]' and '[Context]' sections before using any tool.",
		"    If a signal mentions 'available on-demand', use the 'inspect' tool for that worker before running expensive grep/search tools.",
		"    Use the following tactical instructions to leverage Git, Code, Diff, Search, and Dependency signals:",
		"    - GitTachikoma: Use the current branch and recent commit summary to orient yourself before proposing changes.",
		"    - CodeTachikoma: Use the semantic summary to locate symbols. If a signal is 'available on-demand', do NOT use grep; use inspect or search tools.",
		"    - DiffTachikoma: Prioritize analyzing the specific symbols and line ranges listed in the Diff signal when working on existing code.",
		"    - SearchTachikoma: Treat provided snippets as high-priority, high-signal starting points for your reasoning.",
		"    - DependencyTachikoma: Use dependency summaries to verify library versions and ecosystem constraints.",
		"  </tachikomas>",
		"",
		"  <operating_rules>",
		"    - AGENTS & DESIGN RULES: If 'AGENTS.md' guidelines are present, strictly follow them for code conventions, build and test setups. If 'DESIGN.md' specifications are present, strictly adhere to them for any UI, TUI, or visual styling.",
		"    - WEB SEARCH & FETCH PROTOCOL: When you use 'web_search', do not just rely on the search snippets. Always identify the most relevant URL(s) and use the 'web_fetch' tool to visit and read their full contents to obtain accurate details. Limit search and fetch activities to a maximum of 2-3 queries per task to avoid excessive network load.",
		"  </operating_rules>",
		"",
		"  <brain_protocol>",
		"    You have a persistent session brain — a set of markdown files that survive across turns.",
		"    Use brain_write, brain_read, and brain_list to manage your brain files.",
		"",
		"    MANDATORY BEHAVIORS:",
		"    - When starting a multi-step task, ALWAYS create plan.md with your approach BEFORE writing code.",
		"    - When a plan exists in your brain, ALWAYS read it at the start of your turn and follow it.",
		"    - Track progress in tasks.md — mark items as completed with [x] as you finish them.",
		"    - When all work is done, write summary.md with what was accomplished.",
		"    - If the user asks you to plan, save the plan to plan.md, not just in your response.",
		"",
		"    BRAIN FILES CONVENTION:",
		"    - plan.md    — Current implementation plan (goals, approach, file changes)",
		"    - tasks.md   — Checklist of tasks with completion status",
		"    - summary.md — Post-completion summary of what was done",
		"    - notes.md   — Free-form notes, discoveries, or context for future turns",
		"  </brain_protocol>",
		"</system_instructions>",
		"",
	)
	
	if agentSystem != "" {
		lines = append(lines, agentSystem, "")
	}
	
	if len(info.AvailableSkills) > 0 {
		lines = append(lines,
			"<available_skills>",
		)
		for _, s := range info.AvailableSkills {
			lines = append(lines,
				"  <skill>",
				fmt.Sprintf("    <name>%s</name>", s.Name),
				fmt.Sprintf("    <description>%s</description>", s.Description),
				"  </skill>",
			)
		}
		lines = append(lines,
			"</available_skills>",
			"",
		)
	}
	
	if info.Guidelines != "" {
		lines = append(lines,
			"<agents_guidelines>",
			info.Guidelines,
			"</agents_guidelines>",
			"",
		)
	}

	if info.DesignSpec != "" {
		lines = append(lines,
			"<design_specification>",
			info.DesignSpec,
			"</design_specification>",
			"",
		)
	}

	lines = append(lines,
		"<available_tools>",
	)
	for _, spec := range specs {
		lines = append(lines, fmt.Sprintf("  - %s: %s | usage: %s", spec.Name, spec.Summary, spec.Usage))
	}
	hasTask := false
	hasBash := false
	for _, spec := range specs {
		if spec.Name == "task" {
			hasTask = true
		}
		if spec.Name == "bash" {
			hasBash = true
		}
	}

	if hasTask {
		note := "  - task: asynchronous execution for long-running commands (installs, tests, builds). It returns immediately with a task ID; DO NOT use task for quick commands (like git status, git tag, cat) where you need to read the output immediately to make your next step."
		if hasBash {
			note += " For those, use the 'bash' tool instead."
		}
		note += " Usage: 'task <comando>' to start a task, 'task terminate <id>' to kill a running task."
		lines = append(lines, note)
	}
	lines = append(lines,
		"</available_tools>",
		"",
	)

	// Inject split token
	lines = append(lines, "--- DYNAMIC ---", "")

	// --- DYNAMIC PART ---

	lines = append(lines,
		"<environment>",
		fmt.Sprintf("  OS: %s", runtime.GOOS),
		fmt.Sprintf("  Arch: %s", runtime.GOARCH),
		fmt.Sprintf("  Current Time: %s", time.Now().Format(time.RFC3339)),
		fmt.Sprintf("  Workspace: %s", info.Workspace),
		fmt.Sprintf("  Working Directory: %s", info.Path),
		fmt.Sprintf("  Provider: %s", providerKind),
		"</environment>",
		"",
	)

	lines = append(lines,
		"<context>",
		"  The following context was prepared automatically. Use it before doing blind searches.",
		"  Some background information might be summarized as 'available on-demand' to save space.",
		"  If you see an on-demand signal, use your tools (read, grep, etc.) to fetch the specific details you need.",
		"",
		fmt.Sprintf("  [Workspace]: %s (%s)", info.Workspace, info.Path),
		fmt.Sprintf("  [Git Status]: %s", info.GitSummary()),
	)

	// Categorize Background Signals
	if len(info.Signals) > 0 || len(info.OnDemandSignals) > 0 {
		lines = append(lines, "  "+strings.ReplaceAll(info.CategorizedSignalSummary(), "\n", "\n  "))
	}

	lines = append(lines,
		"  [Project Semantic Summary]:",
		"  "+strings.ReplaceAll(info.SemanticSummary, "\n", "\n  "),
		"",
		"  [Relevant Files for your current request]:",
		"  "+strings.ReplaceAll(info.RelevantFilesSummary(), "\n", "\n  "),
		"",
		"  [Pre-extracted Relevant Snippets]:",
		"  "+strings.ReplaceAll(info.RelevantSnippetsSummary(), "\n", "\n  "),
	)
	
	if info.BrainSummary != "" {
		lines = append(lines,
			"",
			"  [Session Brain State]:",
			"  "+strings.ReplaceAll(info.BrainSummary, "\n", "\n  "),
		)
	}
	lines = append(lines,
		"</context>",
	)

	return strings.Join(lines, "\n")
}
