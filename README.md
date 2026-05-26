# Motoko

**The high-performance Terminal AI Assistant for codebase intelligence.**

Motoko is a specialized terminal-based AI agent designed for deep repository understanding and surgical code editing. Inspired by the efficiency of *Ghost in the Shell*, it utilizes the unique **Tachikoma System** to maintain a real-time, high-signal context of your workspace without the overhead of massive token consumption.

---

## The Tachikoma System 🤖

Unlike traditional AI agents that "blindly" search through files, Motoko relies on a fleet of **Tachikomas**—deterministic background workers that monitor your project:

- **Reactive Intelligence:** Driven by `fsnotify`, Tachikomas react instantly to file changes, keeping the context "hot."
- **Semantic Awareness:** Powered by `go-tree-sitter`, they maintain a live index of symbols, functions, and imports.
- **Context Sharding:** High-density data (like full AST snapshots) is kept in memory and served to the AI **on-demand**, preventing prompt bloat and reducing costs.
- **Git Integration:** Real-time tracking of branch state, staged changes, and semantic diffs.

---

## Key Features 🚀

- **Context Sidebar (`Alt + S`):** A real-time dashboard showing Git status, Tachikoma health, and pro-actively suggested relevant files.
- **Surgical Patching:** High-precision code editing using both fuzzy-matching and direct AST manipulation across multiple languages.
- **Direct Shell Access:** Seamlessly execute shell commands with an explicit approval safety layer.
- **Multi-Provider Support:** Integrated with OpenAI, Anthropic, and Google Gemini.
- **Local-First Philosophy:** All indexing and analysis happen on your machine. Your code stays local.

---

## Installation 📦

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

## Keyboard Shortcuts ⌨️

### Navigation & Interaction
- **Enter**: Send message / Apply selection.
- **Tab / Right**: Next suggestion / next mention.
- **Shift + Tab / Left**: Previous suggestion / previous mention.
- **Up / Ctrl + P**: Previous history item / Move selection up.
- **Down / Ctrl + N**: Next history item / Move selection down.
- **Alt + Up / Down**: Navigate through messages in the Timeline (for selection).
- **Alt + C**: Copy selected message content (markdown-aware).
- **Esc**: Close popups / Cancel.
- **Ctrl + C**: Exit Motoko.

### Layout & Toggles
- **Alt + S**: Toggle Context Sidebar (Auto-shows on wide terminals).
- **Ctrl + T**: Open Tool Palette (Command Catalog).

---

## Usage Commands 🛠️

- **!command**: Execute shell command directly (e.g., `!ls -la`).
- **/chat**: Switch to standard chat mode.
- **/shell**: Switch to direct shell mode.
- **/tool <name>**: Manually trigger a specific runtime tool.
- **/provider add**: Configure a new LLM provider.
- **/models**: List and select available models.
- **/context**: Peek at the raw system prompt being sent to the AI.
- **/plan / /build**: Toggle between read-only planning and active building modes.

---

## Project Status
Motoko is in active development. We are currently focusing on:
1. **DependencyTachikoma**: Automatic loading of imported function signatures.
2. **Build/Linter Tachikoma**: Real-time compilation feedback for the Agent.
3. **Interactive Timeline**: Re-running tool calls directly from history.

---
*"If we all reacted the same way, we’d be predictable, and there’s always more than one way to view a situation."* — Motoko Kusanagi
