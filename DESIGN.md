# Motoko Design Document

## Architectural Overview
Motoko is built in Go and operates as a terminal coding agent with a strong local-first workflow. The architecture separates the terminal UI, the session runtime, the agent loop, the provider layer, the tool layer, background Tachikoma workers, a semantic engine, and persistent session state (Brain).

## 1. TUI Layer (`internal/ui`)
- **Framework:** `charmbracelet/bubbletea`
- **Styling:** `charmbracelet/lipgloss`
- **Layout:** main timeline plus operational sidebar
- **Input model:** multiline composer with inline suggestions and command completion
- **Goal:** feel closer to an operator console than a plain chat transcript

### UI Components
- `Header`: branding, active mode, terminal identity
- `Timeline`: rendered conversation, system events, command output and tool output. Supports streaming with reasoning visibility toggle.
- `Sidebar`: Tachikoma status, semantic summary, and context signals
- `Composer`: multiline input with command/tool completion. Shows active agent mode and thinking indicator.
- `Footer`: workspace path, git state, task count, context window usage, and pending approvals
- `Popups`: Provider configuration form, Model picker, Session picker, Agent/Mode selector

## 2. Session Runtime (`internal/app`)
The runtime is the operational core that connects the TUI to the agent, tools, and background workers.

### Responsibilities
- Track active agent mode (plan, build, search, or custom)
- Track input mode: `chat` or `shell`
- Parse slash commands
- Parse explicit shell execution with `!command`
- Enforce approval rules for shell execution
- Route `/tool ...` calls to the local tool registry
- Forward normal chat input to the agent loop when the provider is configured
- Manage sessions: create, list, load, resume, compact
- Auto-compact sessions when context window usage reaches 80%
- Auto-generate session titles from the first exchange
- Manage background tasks (launch, terminate, list)
- Enrich agent context with semantic data, brain state, and Tachikoma signals
- Delegate subagent execution for the `delegate` tool

### Slash Commands
- `/help` — Show available commands
- `/clear` — Reset timeline history
- `/compact` — Manually compact the active session
- `/plan` — Activate read-only plan mode
- `/build` — Activate active build mode
- `/agent [name]` — Switch or display active agent mode
- `/mode` — Open agent mode selector popup
- `/shell` — Activate direct shell execution mode
- `/chat` — Return to normal chat mode
- `/status` — Summarize mode, provider, workspace, and approval state
- `/debug` — Toggle agent debug output
- `/trace` — Toggle trace logging (requires build tag)
- `/context` — Show raw system prompt sent to the agent
- `/provider` — Manage providers (list, add, use, remove)
- `/models` — List or select models from the active provider
- `/sessions` — Open session picker popup
- `/tools` — Show all registered tools
- `/tool <name> <args>` — Execute a specific tool manually
- `/task` — List or manage background tasks
- `/brain` — Interact with session brain (list, read, plan, tasks, summary, clear)
- `/approve` — Execute the pending shell action
- `/deny` — Cancel the pending shell action

## 3. Tool Layer (`internal/tools`)
Motoko has a local tool registry used by the runtime, the agent loop, and slash commands.

### Registered Tools
- `read`: read files or list directories inside the workspace
- `glob`: match workspace paths by pattern
- `grep`: regex search across text files
- `bash`: execute shell commands inside the workspace (subject to approval rules)
- `patch`: apply controlled SEARCH/REPLACE edits with fuzzy matching
- `inspect`: query live data from any Tachikoma worker by name
- `delegate`: spawn a sub-agent in a specified mode to handle a sub-task
- `task`: launch long-running commands in the background; returns a task ID
- `brain_write`: write a markdown file to the session brain
- `brain_read`: read a markdown file from the session brain
- `brain_list`: list all files in the session brain
- `activate_skill`: load specialized instructions from the skills directory
- `web_search`: search the web via Mojeek (with DuckDuckGo fallback)
- `web_fetch`: fetch and extract readable content from a URL

### Design Constraints
- Tools are workspace-scoped; paths outside the workspace are rejected
- Tool outputs are truncated to 12 KB to avoid prompt bloat
- `bash` and `patch` are classified as write tools and are filtered out in plan/search modes
- `bash` remains subject to runtime permission rules

## 4. Agent Layer (`internal/agent`)
The agent loop handles the full LLM interaction cycle including tool calling.

### Behavior
- Normal input in chat mode is routed to the agent when the provider is configured
- The agent receives workspace and git context, semantic summaries, relevant file/snippet suggestions, brain state, available skills, and the registered tool catalog
- The agent can answer directly or request tools; tool results are fed back into the loop
- Maximum of 24 tool iterations per turn (configurable via `MOTOKO_MAX_ITERATIONS`)
- Cycle detection prevents infinite tool loops
- Tools are executed in parallel when multiple calls are returned in a single response
- Streaming is supported: text and reasoning deltas are forwarded to the TUI in real-time

### Multi-Agent System
- **Built-in agents:** `plan`, `build`, `search` — each with a distinct system prompt
- **Custom agents:** defined in a `.agents` INI file at the workspace root
- **Agent switching:** via `/agent <name>`, `/plan`, `/build`, `/mode`, or `Ctrl+A`
- **Tool filtering:** plan and search agents only get read-only tools; build agents get the full set
- **Subagent delegation:** the `delegate` tool spawns a temporary sub-agent with its own context

### Brain Protocol
The agent is instructed to use the session brain for multi-step workflows:
- `plan.md` — current implementation plan
- `tasks.md` — checklist with completion status
- `summary.md` — post-completion summary
- `notes.md` — free-form notes and discoveries

## 5. Provider Layer (`internal/provider`)
Motoko supports multiple LLM providers through a unified interface.

### Supported Providers
- **OpenAI** native integration supporting both Chat Completions and Responses APIs
- **Anthropic** native integration with Claude models, including extended thinking
- **Gemini** via Google GenAI Go SDK with support for Google Search grounding, Code Execution and configuration of thinking levels
- **OpenAi Compatible providers**

### Capabilities
- Streaming via SSE (Server-Sent Events) for all providers
- Reasoning/thinking support with configurable token budget per provider (OpenAI: reasoning_effort, Anthropic: extended thinking, Gemini: thinkingBudget/thinkingLevel)
- Model listing from provider APIs
- Unified tool/function call handling across provider formats

### Configuration
Providers are configured interactively via `/provider add` or in the JSON config file at `~/.config/motoko/config.json`. Each provider stores: name, kind/preset, base URL, API key, model, thinking budget, context window, and model list. Gemini providers additionally support `enable_google_search` and `enable_code_execution` options.

## 6. Tachikoma System (`internal/tachikoma`)
Tachikomas are deterministic background workers that keep local context fresh without requiring the agent to scan everything every time.

### Active Tachikomas
- `GitTachikoma`: publishes git branch, dirty-state summary, and recent commits. Refreshes every 10 seconds.
- `CodeTachikoma`: maintains a live semantic index of the workspace using Tree-sitter. Refreshes every 30 seconds and reacts to file changes via `fsnotify`.
- `DiffTachikoma`: parses `git diff -U0` and cross-references changed line ranges with the semantic index to report which symbols were modified. Refreshes every 15 seconds and reacts to file changes.
- `SearchTachikoma`: accepts prompt updates and returns relevant code snippets from the semantic index. Triggered on every user prompt.
- `DependencyTachikoma`: scans manifest files (`go.mod`, `package.json`, `Cargo.toml`, `requirements.txt`, `Gemfile`) and publishes a per-ecosystem dependency summary. Refreshes every 15 seconds.

### Architecture
- Each Tachikoma implements the `Worker` interface: `Name() string` and `Run(ctx, publish) error`
- The `Manager` starts all workers as goroutines and collects updates via a shared channel
- Updates are published to the TUI via Bubble Tea messages
- Workers use `WatchHelper` for filesystem events with configurable debounce
- The `inspect` tool allows the agent to query any worker's latest payload on demand

## 7. Semantic Engine (`internal/semantic`)
The semantic engine provides language-aware code analysis powered by Tree-sitter.

### Capabilities
- File discovery respecting `.gitignore` rules
- AST parsing with per-language query patterns
- Symbol extraction: functions, methods, types, structs, classes, interfaces, imports
- Snapshot-based indexing with summary generation
- Relevant file ranking by keyword overlap with user prompts
- Relevant snippet extraction with configurable budget (max files and total lines)

### Supported Languages
Go, Python, Rust, C/C++, JavaScript, TypeScript, JSX/TSX, Java, Ruby, PHP, C#, Kotlin, Swift, Scala, Lua, Shell/Bash, Zig, Elixir, Haskell, OCaml, Dart, Perl, R, YAML, TOML, HCL, Dockerfile, Makefile, and more.

## 8. Session Brain (`internal/brain`)
The brain provides persistent per-session markdown storage.

### Design
- Each session gets a brain directory under `~/.local/share/motoko/sessions/<workspace>/<session>/`
- Files are restricted to `.md` extension; path traversal is rejected
- The agent is instructed to use brain files for multi-step workflows
- Brain state (file list, plan summary, tasks summary) is injected into the system prompt each turn
- Brain files can be managed via the `/brain` slash command or the `brain_write`/`brain_read`/`brain_list` tools

## 9. Skills System (`internal/skills`)
Skills are folders of instructions that extend the agent's capabilities for specialized tasks.

### Design
- Skills are discovered from `.agents/skills/` in the workspace root
- Each skill has a `SKILL.md` with YAML frontmatter (name, description) and detailed instructions
- Available skills are listed in the system prompt
- The agent calls `activate_skill` to load a skill's full instructions before proceeding

## 10. Concurrency Model
- **UI loop:** Bubble Tea event loop on the main thread
- **Runtime actions:** synchronous command and tool dispatch from the UI model
- **Shell execution:** async Bubble Tea command returning result messages
- **Agent loop:** runs as an async Bubble Tea command; streams events through a buffered channel
- **Background tasks:** managed by `TaskManager`; results delivered via event channel
- **Tachikoma workers:** goroutines publishing status updates through a shared update channel
- **Subagents:** synchronous execution within the `delegate` tool's goroutine

## 11. Project Guidelines
- **AGENTS.md:** workspace-level guidelines loaded into the system prompt automatically
- **.agents file:** custom agent definitions in INI format
- **.agents/skills/:** skill definitions with YAML+markdown
