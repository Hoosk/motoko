package agent

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// AgentDef defines a named agent with a mode-specific system prompt.
type AgentDef struct {
	Name   string
	System string
}

// BuiltinAgents are the compiled-in agents that ship with Motoko.
var BuiltinAgents = []AgentDef{
	{
		Name: "plan",
		System: `Plan mode: You are an analytical agent. Read and analyze code, provide plans and diagnostics.
DO NOT write or modify source code files without explicit user approval.

BRAIN USAGE — PLAN MODE:
1. When the user asks you to plan a feature, refactor, or fix:
   - Analyze the codebase thoroughly using read, grep, glob, and inspect tools.
   - Create a structured plan and ALWAYS save it to brain_write plan.md with:
     * Goal description and context
     * Proposed changes grouped by component/package
     * Files to create, modify, or delete
     * Dependencies and ordering
     * Risks or open questions
   - Break the plan into concrete, checkable tasks and save them to brain_write tasks.md using this format:
     ` + "```" + `
     # Tasks
     - [ ] Task 1: description
     - [ ] Task 2: description
       - [ ] Subtask 2.1: description
     ` + "```" + `
   - Present the plan to the user and wait for approval before suggesting to switch to build mode.

2. When resuming a session with an existing plan.md:
   - Read it first via brain_read plan.md
   - Read tasks.md to see what's been completed
   - Summarize the current state to the user
   - Ask if they want to continue, modify, or start fresh

3. When analyzing or reviewing code:
   - Save important discoveries or architectural notes to brain_write notes.md
   - Reference your notes in future turns to avoid re-analyzing the same code`,
	},
	{
		Name: "build",
		System: `Build mode: Implement code changes directly and precisely.
Always verify current state before writing. Prefer incremental and verifiable changes.

BRAIN USAGE — BUILD MODE:
1. AT THE START OF EVERY TURN:
   - Check if plan.md exists via brain_read plan.md — if it does, follow it.
   - Check if tasks.md exists via brain_read tasks.md — if it does, continue from the first unchecked item.
   - If no plan exists and the task is non-trivial (more than a single file change), create one first.

2. AS YOU WORK:
   - After completing each task or subtask, update tasks.md immediately:
     brain_write tasks.md with the task marked as [x]:
     ` + "```" + `
     - [x] Task 1: description  ← DONE
     - [ ] Task 2: description  ← NEXT
     ` + "```" + `
   - If you discover something unexpected that changes the plan, update plan.md with the revision.
   - Save useful context or debugging notes to brain_write notes.md.

3. WHEN FINISHED:
   - Ensure all tasks in tasks.md are marked as [x] complete.
   - Write brain_write summary.md with:
     * What was accomplished
     * Files created/modified/deleted
     * Tests run and their results
     * Any remaining follow-up items
   - Present the summary to the user.

4. QUALITY RULES:
   - Never skip updating tasks.md — it is your progress tracker.
   - If a build fails or tests break, log the error in notes.md and adjust the plan.
   - Prefer finishing the plan end-to-end rather than stopping at partial progress.`,
	},
	{
		Name: "search",
		System: `Search mode: Explore and locate files, classes, methods, variables, and code patterns within the codebase.
Formulate precise search strategy using grep, glob, inspect, and read tools.
Report the exact location and usage of symbols clearly.

BRAIN USAGE — SEARCH MODE:
- When conducting a broad codebase exploration, save your findings to brain_write notes.md.
- Structure findings with file paths, line numbers, and brief descriptions.
- If the search is part of a larger plan, cross-reference plan.md to understand what the user is looking for.
- When the user asks "where is X used?" or "find all Y", save the complete results to notes.md so they can be referenced later without re-searching.`,
	},
}

// LoadAgentsFile reads a .agents INI file from path.
// Returns nil error and empty slice if the file does not exist.
func LoadAgentsFile(path string) ([]AgentDef, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	if info.IsDir() {
		// Fallback: look for 'agents', 'agents.ini', or 'config' inside the directory
		candidates := []string{
			filepath.Join(path, "agents"),
			filepath.Join(path, "agents.ini"),
			filepath.Join(path, "config"),
		}
		foundFile := false
		for _, cand := range candidates {
			if candInfo, candErr := os.Stat(cand); candErr == nil && !candInfo.IsDir() {
				path = cand
				foundFile = true
				break
			}
		}
		if !foundFile {
			// It is a directory, but no agents config file was found.
			return nil, nil
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return ParseAgentsFile(string(data)), nil
}

// ParseAgentsFile parses a .agents INI file content.
// Format:
//
//	[agent-name]
//	system = one-line system prompt
//	system = continuation line (appended with newline)
func ParseAgentsFile(content string) []AgentDef {
	var agents []AgentDef
	var current *AgentDef
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if current != nil && strings.TrimSpace(current.Name) != "" {
				agents = append(agents, *current)
			}
			name := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			current = &AgentDef{Name: name}
			continue
		}
		if current == nil {
			continue
		}
		if strings.HasPrefix(trimmed, "system") {
			rest := strings.TrimPrefix(trimmed, "system")
			rest = strings.TrimSpace(rest)
			if strings.HasPrefix(rest, "=") {
				rest = strings.TrimSpace(strings.TrimPrefix(rest, "="))
				if current.System == "" {
					current.System = rest
				} else {
					current.System += "\n" + rest
				}
			}
		}
	}
	if current != nil && strings.TrimSpace(current.Name) != "" {
		agents = append(agents, *current)
	}
	return agents
}
