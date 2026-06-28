<p align="center">
  <h1 align="center">Motoko</h1>
</p>
<p align="center">A local-first AI coding companion for your terminal.</p>
<p align="center">
  <a href="https://github.com/Hoosk/motoko/actions/workflows/verify.yml"><img alt="Build status" src="https://img.shields.io/github/actions/workflow/status/Hoosk/motoko/verify.yml?style=flat-square&branch=master" /></a>
  <img alt="Go Version" src="https://img.shields.io/github/go-mod/go-version/Hoosk/motoko?style=flat-square" />
</p>

---

### Installation

```bash
# YOLO
curl -sSL https://raw.githubusercontent.com/Hoosk/motoko/master/install.sh | bash

# Manual Build
git clone https://github.com/Hoosk/motoko.git
cd motoko
go build -o motoko ./cmd/motoko
mv motoko /usr/local/bin/ # or any directory in your $PATH
```

### Usage

Run `motoko` without flags to enter the interactive TUI mode:
```bash
motoko
```

#### Command-Line Options
Motoko supports several command-line flags for automation, updates, and quick queries:

*   `-q, --question "<prompt>"`: Ask a question directly to the AI assistant, stream the output, and exit.
*   `--resume`: Resume the last active chat session (can be combined with `-q`).
*   `-h, --help`: Show the help menu with all available options.
*   `-v, --version`: Print the version information.
*   `--update`: Check and install the latest updates.
*   `--check-update`: Check if a newer version is available.

For example, to run a direct query on your codebase using Tachikoma-gathered context:
```bash
motoko -q "Dime cuántos archivos hay en este proyecto"
```

### Agents & Modes

Motoko includes task-focused agent modes that you can switch between:

*   **build** - Default, full-access agent for codebase edits.
    *   Has read and write tool access (including shell execution and patching).
*   **plan** - Read-only agent for architecture analysis and code planning.
    *   Denies code edits and file modifications by default.
    *   Focuses on building the session plan before committing to modifications.
*   **search** - Read-only exploration mode.
    *   Optimized for codebase queries and semantic search.

Define custom agents using a `.agents` configuration file at the root of your workspace.

---

### Technical Architecture & Context (Tachikomas)

Unlike tools that perform exhaustive or blind codebase scans, Motoko runs **Tachikomas**—lightweight, deterministic background goroutines that monitor your files and feed structured context into the prompt:

*   **Git Tracking (`GitTachikoma`):** Tracks branch state, staged diffs, and untracked/dirty file summaries.
*   **AST Semantic Indexing (`CodeTachikoma`):** Uses Tree-sitter to parse and index symbols, functions, methods, and imports. Refreshes on file changes.
*   **Semantic Diff Correlation (`DiffTachikoma`):** Correlates active git diff hunks with the AST index to map edits to specific code symbols.
*   **Incremental Search (`SearchTachikoma`):** Ranks and pre-retrieves relevant code blocks based on user queries.
*   **Dependency Audit (`DependencyTachikoma`):** Summarizes project manifests (`go.mod`, `package.json`, `Cargo.toml`, etc.).

All background workers use debounced filesystem notifications (`fsnotify`) and coordinate via a unified Go channel to keep context fresh without impacting system performance.

---

### Keyboard Shortcuts

| Shortcut | Description |
| :--- | :--- |
| **Enter** | Send message / Apply active selection |
| **Tab** / **Right** | Next inline suggestion / mention |
| **Shift+Tab** / **Left** | Previous inline suggestion / mention |
| **Up** / **Down** | Navigate command history / Select options |
| **Alt + Up / Down** | Scroll through message timeline |
| **Alt + C** | Copy selected message contents |
| **Esc** | Close popups / Cancel action |
| **Ctrl + C** | Exit Motoko |
| **Ctrl + S** / **Alt + S** | Toggle Context Sidebar visibility (only available when terminal width >= 100) |
| **Ctrl + T** | Toggle Tool Catalog overlay |
| **Ctrl + H** | Toggle Help overlay |
| **Ctrl + R** | Toggle Reasoning/Thinking output visibility |
| **Ctrl + P** | Open LLM Provider configuration |
| **Ctrl + M** | Open Model selector |
| **Ctrl + O** | Open Session picker |
| **Ctrl + A** | Open Agent Mode selector |

---

### Command Reference

| Command | Action |
| :--- | :--- |
| **`!cmd`** | Run shell command directly (e.g. `!go test ./...`) |
| **`/help`** | Display all available commands and help overlay |
| **`/chat`** | Switch input mode to standard chat |
| **`/shell`** | Switch input mode to direct shell execution |
| **`/plan`** | Shortcut to activate read-only `plan` mode |
| **`/build`** | Shortcut to activate editing `build` mode |
| **`/agent [name]`** | Switch to or show active agent mode |
| **`/mode`** | Open agent mode selection popup |
| **`/tool <name> <args>`** | Manually execute a registered tool |
| **`/tools`** | List all available developer tools |
| **`/provider`** | Manage configurations (`list`, `add`, `use`, `remove`) |
| **`/models`** | List and select LLM models |
| **`/themes [name]`** | List available visual themes or switch themes (`cyberpunk`, `nord`, `dracula`, `monochrome`) |
| **`/sessions`** | Open the session switcher |
| **`/brain`** | Manage session brain files (`list`, `read`, `plan`, `tasks`, `summary`, `clear`) |
| **`/context`** | View raw system prompt generated for the next turn |
| **`/status`** | Display current model, provider, and workspace states |
| **`/compact`** | Trigger manual conversation compaction |
| **`/task`** | Manage background commands and task logs |
| **`/approve`** / **`/deny`** | Accept or reject a pending shell execution request |
| **`/debug`** | Toggle agent debugging log output |
| **`/trace`** | Toggle tracing logs (requires compilation with `-tags motoko_trace`) |

---

### Configuration

Motoko reads its configuration from `~/.config/motoko/config.json`. You can configure API keys and providers interactively using `/provider add` or edit the file manually.

#### Themes & Visual Settings
Use the `/themes` slash command in Motoko to dynamically switch visual themes. The current configuration is stored in the `theme` field inside `~/.config/motoko/config.json`.

Available themes:
*   **cyberpunk** (Default) - Neon green, purple, and gold cyberpunk details.
*   **nord** - Cool frosted blues and snow accents.
*   **dracula** - High-contrast purple and pink classic theme.
*   **monochrome** - Matrix green on black look.

#### Sidebar Behavior
The sidebar is automatically enabled on large terminals (width >= 100 columns) and automatically hidden on small terminals (width < 100 columns) to prevent overlapping layout and maintain spacing. When resizing terminal width from below 100 back to 100 or wider, the sidebar is automatically restored unless it was explicitly hidden by the user.

#### Managing Providers
```bash
/provider add          # Add a new LLM provider config
/provider list         # List configured providers
/provider use <name>   # Switch active provider
/models                # Select models for the active provider
```
---

*"If we all reacted the same way, we'd be predictable, and there's always more than one way to view a situation."* — Motoko Kusanagi
