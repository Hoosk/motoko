# Motoko Design Document

## Architectural Overview
Motoko is built in Go and is evolving toward a terminal coding agent with a strong local-first workflow. The current architecture separates the terminal UI, the session runtime, the tool layer, and background Tachikoma workers.

## 1. TUI Layer (`internal/ui`)
- **Framework:** `charmbracelet/bubbletea`
- **Styling:** `charmbracelet/lipgloss`
- **Layout:** main timeline plus operational sidebar
- **Input model:** multiline composer with inline suggestions and command completion
- **Goal:** feel closer to an operator console than a plain chat transcript

### Current UI components
- `Header`: branding, active mode, terminal identity
- `Timeline`: rendered conversation, system events, command output and tool output
- `Sidebar`: suggestions, tool catalog and Tachikoma status
- `Composer`: multiline input with command/tool completion
- `Footer`: workspace, git state and pending approvals

## 2. Session Runtime (`internal/app`)
The runtime is the operational core currently used by the TUI.

### Responsibilities
- Track active mode: `plan` or `build`
- Track input mode: `chat` or `shell`
- Parse slash commands
- Parse explicit shell execution with `!command`
- Enforce approval rules for shell execution
- Route `/tool ...` calls to the local tool registry
- Forward normal chat input to the agent loop when the provider is configured

### Current commands
- `/help`
- `/clear`
- `/plan`
- `/build`
- `/shell`
- `/chat`
- `/status`
- `/context`
- `/tools`
- `/tool`
- `/approve`
- `/deny`

## 3. Tool Layer (`internal/tools`)
Motoko now has a real local tool registry that can be used by the runtime and later by the LLM agent loop.

### Implemented tools
- `read`: read files or list directories inside the workspace
- `glob`: match workspace paths by pattern
- `grep`: regex search across text files
- `bash`: execute shell commands inside the workspace
- `patch`: apply controlled SEARCH/REPLACE edits

### Design constraints
- Tools are workspace-scoped
- Paths outside the workspace are rejected
- Tool outputs are summarized and then rendered in the timeline
- `bash` remains subject to runtime permission rules

## 4. Agent Layer (`internal/agent`)
Motoko now has a minimal agent loop.

### Current behavior
- Normal input in chat mode is routed to the agent when the provider is configured
- The agent receives workspace and git context plus the registered tool catalog
- The agent can either answer directly or request one tool at a time
- Tool results are fed back into the loop until a final answer is produced

## 5. Provider Layer (`internal/provider`)
The first provider integration is OpenAI-compatible over HTTP.

### Configuration
- `MOTOKO_OPENAI_BASE_URL`
- `MOTOKO_OPENAI_API_KEY`
- `MOTOKO_OPENAI_MODEL`

If these variables are not defined, Motoko keeps the runtime and tools active but does not enable the agent loop.

## 6. Tachikoma System (`internal/tachikoma`)
Tachikomas are deterministic background workers that keep local context fresh without requiring the agent to scan everything every time.

### Current Tachikomas
- `WorkspaceTachikoma`: publishes workspace identity
- `GitTachikoma`: publishes git branch and dirty-state summary

### Planned next Tachikomas
- `SearchTachikoma`: cached file and extension index
- `DiffTachikoma`: active change summary
- `AstTachikoma`: structural symbol and AST summaries

## 7. Concurrency Model
- **UI loop:** Bubble Tea event loop on the main thread
- **Runtime actions:** synchronous command and tool dispatch from the UI model
- **Shell execution:** async Bubble Tea command returning result messages
- **Background workers:** Tachikomas run in goroutines and publish status updates through a shared update channel

## 8. Near-Term Direction
The next architectural milestone is improving the real agent loop.

### Next step
1. Add streaming responses from the provider into the timeline
2. Improve tool-call output formatting and error recovery
3. Add richer deterministic context via `SearchTachikoma`
4. Add patch-based edit validation and change summaries

## 9. Product Direction
Motoko should remain distinct in tone and aesthetics, but functionally it is now targeting the same core interaction model as opencode:
- coding-focused terminal UX
- explicit commands
- visible operational state
- deterministic local tools before expensive model calls
