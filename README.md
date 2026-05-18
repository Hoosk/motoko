# 🦾 MOTOKO

**Public Security Section 9 - Local AI Operative**

Motoko is a high-speed, terminal-based AI assistant built in Go. It is designed to be lean, impactful, and extremely token-efficient by leveraging a unique architecture called the **Tachikoma System**.

## 🐚 The Philosophy: Ghost in the Shell
Inspired by the cyberpunk aesthetic and efficiency of Section 9, Motoko isn't just another AI wrapper. It's a localized tool that treats your code as a dynamic environment.

## 🤖 The Tachikoma System
In other AI CLIs, the AI has to "think" about how to find code, which often leads to expensive and slow context gathering. In Motoko, we use **Tachikomas**.

**What is a Tachikoma?**
- **Non-AI Background Workers:** They are fast, deterministic Go goroutines.
- **Context Gatherers:** They use tools like `go-tree-sitter` to parse your code and `git` to track changes *before* the AI even asks.
- **Force Multipliers:** They feed structured, high-signal data to the AI, reducing token usage by up to 80% and providing near-instant responses.

## ✨ Features
- **GitS Aesthetic:** A stunning TUI built with Bubble Tea and Lipgloss.
- **Tachikoma Background Context:** Real-time codebase analysis without blocking your flow.
- **Token Efficiency:** Only send the AST and relevant snippets, never full file dumps unless necessary.
- **Interactive TUI:** Toggle worker status with `Alt+T` to see your Tachikomas in action.

## 🚀 Getting Started

### Prerequisites
- Go 1.24+
- Git

### Installation
```bash
git clone https://github.com/Hoosk/motoko.git
cd motoko
go build -o motoko ./cmd/motoko
./motoko
```

### Key Bindings
- `Alt+T` / `Ctrl+Alt+T`: Toggle Tachikoma Status Bar.
- `Enter`: Send message.
- `Ctrl+C` / `Esc`: Exit.

## 🛠 Project Structure
- `internal/tachikoma`: The concurrent worker engine.
- `internal/ui`: Bubble Tea models and GitS styling.
- `internal/system`: Local environment and Git detection.
- `AGENT.md`: AI behavior and protocol definitions.
- `DESIGN.md`: Detailed technical architecture.

---
*“If we all reacted the same way, we’d be predictable, and there’s always more than one way to view a situation.”* - Major Motoko Kusanagi
