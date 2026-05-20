# Motoko

**Terminal-based AI assistant for codebase intelligence.**

Motoko is a high-performance terminal AI assistant built in Go. It is designed for efficiency and speed, utilizing the Tachikoma System to provide deep codebase context without excessive token consumption.

## The Tachikoma System

Motoko avoids the overhead of manual context gathering by using background workers called **Tachikomas**:

- **Concurrent Analysis:** Background Go routines that index your project in real-time.
- **High-Signal Context:** Uses `go-tree-sitter` and `git` integration to provide the AI with only the most relevant code snippets and AST data.
- **Token Efficiency:** Significantly reduces context window bloat, leading to faster responses and lower costs.

## Features

- **Interactive TUI:** A modern terminal interface built with Bubble Tea and Lipgloss.
- **Automated Indexing:** Real-time analysis of file structures and git changes.
- **Optimized Workflow:** Focuses on providing actionable intelligence directly within your terminal.
- **Customizable Modes:** Support for both planning and building phases of development.

## Installation

### From Source

Requires Go 1.24+.

```bash
git clone https://github.com/Hoosk/motoko.git
cd motoko
go build -o motoko ./cmd/motoko
```

## Usage

Run the application from your project root:

```bash
./motoko
```

### Key Bindings

- **Alt + T**: Toggle Tachikoma status bar.
- **Enter**: Send message.
- **Ctrl + C**: Exit application.

## Project Documentation

- **DESIGN.md**: Standards for design and interface consistency.
- **AGENTS.md**: AI behavior protocols and system instructions.
- **internal/**: Core implementation details of the Tachikoma engine and TUI.

---
*"If we all reacted the same way, we’d be predictable, and there’s always more than one way to view a situation."* — Motoko Kusanagi
