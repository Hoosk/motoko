# AGENTS.md

This file provides context and instructions for AI agents working on the Motoko project.

## Project Overview
Motoko is a high-performance terminal AI assistant built in Go. It utilizes the **Tachikoma System** to provide deep codebase context through concurrent background workers, minimizing token consumption and maximizing precision.

## Setup & Build
- **Language:** Go 1.24+
- **Build Command:** `go build -o motoko ./cmd/motoko`
- **Dependencies:** Uses `charmbracelets/bubbletea` for TUI and `smacker/go-tree-sitter` for code analysis.

## Testing
- **Run all tests:** `go test ./...`
- **Testing Style:** Ensure new features include unit tests in the same package (e.g., `logic_test.go`).

## Code Style & Conventions
- **Go Idioms:** Follow standard Go formatting (`gofmt`) and linting conventions.
- **TUI Architecture:** UI components are built using the Elm architecture (Model-Update-View) via Bubble Tea.
- **Surgical Updates:** Prefer precise code changes over large-scale refactoring unless requested.

### Linting & Code Quality
- **Resolve, Don't Bypass:** Never use `//nolint` directives (e.g., `//nolint:goconst`, `//nolint:govet`, `//nolint:fieldalignment`) as a shortcut to get rid of linter warnings.
- **Proper Fixes:**
  - For `goconst` warnings, refactor duplicate string literals into reusable constants.
  - For `govet` (fieldalignment) warnings, rearrange struct fields optimally by size to reduce padding/alignment overhead (use `fieldalignment -fix ./...` if available).
  - Only use `//nolint` when a bypass is technically necessary, standard, or explicitly requested.

## Role & Persona
Motoko is an AI-powered CLI assistant inspired by Ghost in the Shell. It acts as a high-speed operative within the developer's local environment, focused on efficiency, precision, and minimizing cognitive load.

## The Tachikoma Multiplier
Unlike standard AI agents that "hunt" for information by reading entire files, Motoko relies on **Tachikomas**.

### Tachikoma Protocol
1. **Passive Context Gathering:** Tachikomas are non-AI background workers (Go goroutines) that monitor the workspace.
2. **Context Synthesis:** They use deterministic tools (Tree-sitter, Git, Grep) to provide the AI with structured snippets, AST summaries, and change logs.
3. **Token Efficiency:** By serving only the *relevant* parts of the code structure, Tachikomas allow the AI to operate with a much smaller, high-signal context window.
4. **Tactical Intelligence:** Tachikoma signals are categorized into "Summary" (direct context) and "On-Demand" (requiring tool usage). Agents must prioritize "On-Demand" signals by using the `inspect` tool rather than performing broad searches.

## Operational Guidelines
- **Speed First:** Always prefer a Tachikoma-provided summary over a full file read.
- **Subtle Guidance:** Use background updates to inform the user of what information is being gathered without blocking their flow.
- **Surgical Changes:** When implementing code, follow the "Section 9" approach: precise, minimal, and impactful.
- **Interactive TUI:** Use the Bubble Tea interface to provide rich, real-time feedback on background operations.
