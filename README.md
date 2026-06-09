# Motoko

**The high-performance Terminal AI Assistant for codebase intelligence.**

Motoko is a specialized terminal-based AI agent designed for deep repository understanding and surgical code editing. Inspired by the efficiency of *Ghost in the Shell*, it utilizes the unique **Tachikoma System** to maintain a real-time, high-signal context of your workspace without the overhead of massive token consumption.

---

## The Tachikoma System

Unlike traditional AI agents that "blindly" search through files, Motoko relies on a fleet of **Tachikomas**—deterministic background workers that monitor your project:

- **GitTachikoma:** Real-time tracking of branch state, staged changes, and dirty-file summaries.
- **CodeTachikoma:** Powered by `go-tree-sitter`, it maintains a live index of symbols, functions, and imports across your workspace.
- **DiffTachikoma:** Correlates `git diff` hunks with the semantic index to report which symbols have been modified.
- **SearchTachikoma:** Pre-fetches relevant code snippets based on the current prompt, so context is ready before the agent even starts.
- **DependencyTachikoma:** Detects and summarizes project dependencies across ecosystems (Go, JS/TS, Python, Rust, Ruby).

All Tachikomas run as goroutines and publish updates through a shared channel. They react to file changes via `fsnotify`, keeping the context "hot" at all times.

---

## Key Features

- **Multi-Agent Modes:** Switch between `plan` (read-only analysis), `build` (active editing), and `search` (codebase exploration) modes. Define custom agents via a `.agents` file.
- **Session Brain:** Persistent per-session memory where the agent stores plans (`plan.md`), task checklists (`tasks.md`), summaries, and notes. Survives across turns and sessions.
- **Surgical Patching:** High-precision code editing using fuzzy-matching SEARCH/REPLACE blocks.
- **Direct Shell Access:** Execute shell commands with an explicit approval safety layer. Background tasks for long-running commands.
- **Multi-Provider Support:** Integrated with OpenAI, Anthropic, and Google Gemini. Supports streaming and reasoning/thinking modes.
- **Semantic Context:** Tree-sitter powered AST analysis across Go, Python, Rust, C++, JavaScript, TypeScript, and more.
- **Web Access:** Built-in `web_search` (Mojeek + DuckDuckGo fallback) and `web_fetch` tools for retrieving external information.
- **Skills System:** Extensible skill definitions in `.agents/skills/` that the agent can activate on-demand for specialized tasks.
- **Subagent Delegation:** The agent can spawn sub-agents in different modes to handle parallel tasks.
- **Auto-Compaction:** Sessions are automatically compacted when the context window reaches 80% capacity.
- **Local-First Philosophy:** All indexing and analysis happen on your machine. Your code stays local.

---

## Installation

### Prerequisites
- [Go](https://go.dev/doc/install) 1.24 or higher.

### Quick Install (Linux/macOS)
Run our installer script to build and install Motoko to your local path:

```bash
curl -sSL https://raw.githubusercontent.com/Hoosk/motoko/master/install.sh | bash
```

### Manual Installation
1. Clone the repository:
   ```bash
   git clone https://github.com/Hoosk/motoko.git
   cd motoko
   ```
2. Build the binary:
   ```bash
   go build -o motoko ./cmd/motoko
   ```
3. Move to your PATH:
   ```bash
   mv motoko /usr/local/bin/ # or any directory in your $PATH
   ```

---

## Keyboard Shortcuts

### Navigation & Interaction
- **Enter**: Send message / Apply selection.
- **Tab / Right**: Next suggestion / next mention.
- **Shift + Tab / Left**: Previous suggestion / previous mention.
- **Up / Down**: Navigate history / Move selection.
- **Alt + Up / Down**: Navigate through messages in the Timeline.
- **Alt + C**: Copy selected message content.
- **Esc**: Close popups / Cancel.
- **Ctrl + C**: Exit Motoko.

### Layout & Toggles
- **Ctrl + S / Alt + S**: Toggle Context Sidebar.
- **Ctrl + T**: Toggle Tool Catalog overlay.
- **Ctrl + H**: Toggle Help overlay.
- **Ctrl + R**: Toggle Reasoning/Thinking visibility.
- **Ctrl + P**: Open Provider configuration form.
- **Ctrl + M**: Open Model selector.
- **Ctrl + O**: Open Session picker.
- **Ctrl + A**: Open Agent/Mode selector.

---

## Usage Commands

- **!command**: Execute shell command directly (e.g., `!ls -la`).
- **/help**: Show all available commands.
- **/chat**: Switch to standard chat mode.
- **/shell**: Switch to direct shell mode.
- **/plan**: Activate read-only plan mode.
- **/build**: Activate active build mode.
- **/agent [name]**: Switch or show active agent mode.
- **/mode**: Open the agent mode selector popup.
- **/tool \<name\>**: Manually trigger a specific runtime tool.
- **/tools**: List all registered tools.
- **/provider**: Manage configured LLM providers (`list`, `add`, `use`, `remove`).
- **/models**: List and select available models.
- **/sessions**: Open the session picker.
- **/brain**: Interact with the session brain (`list`, `read`, `plan`, `tasks`, `summary`, `clear`).
- **/context**: View the raw system prompt being sent to the AI.
- **/status**: Summarize mode, provider, and workspace state.
- **/compact**: Manually compact the active session.
- **/task**: List or manage background tasks.
- **/approve**: Execute the pending shell action.
- **/deny**: Cancel the pending shell action.
- **/debug**: Toggle agent debug output.
- **/trace**: Toggle trace logging (requires `-tags motoko_trace` build).

---

## Configuration

Motoko uses a TOML config file stored at `~/.config/motoko/config.toml`. Providers can be configured interactively with `/provider add` or directly in the config file.

### Provider Setup
```
/provider add          # Opens interactive form
/provider list         # List configured providers
/provider use <name>   # Switch active provider
/models                # Select model from active provider
```

### Thinking/Reasoning Budget
Use the model selector to adjust the thinking budget. Available levels: off, low (1k), medium (8k), high (24k), xhigh (64k tokens).

---

*"If we all reacted the same way, we'd be predictable, and there's always more than one way to view a situation."* — Motoko Kusanagi
