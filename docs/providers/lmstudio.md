# Integration Guide: LM Studio

Motoko supports **LM Studio** as a local-first LLM provider. This allows you to run open-source models completely locally (like Llama 3, Qwen, Mistral, or Phi) while maintaining full support for advanced features like stateful chat, MCP (Model Context Protocol), and custom agent tools.

## Prerequisites

1. Download and install [LM Studio](https://lmstudio.ai/).
2. Load a model of your choice within LM Studio (e.g., `qwen2.5-coder-7b-instruct` or `llama-3-8b-instruct`).
3. Start the local server in LM Studio (typically runs on port `1234`).
   - Make sure the server option for CORS is enabled if you want to connect external services.
   - Note the port number; by default, LM Studio uses port `1234`.

## Configuring LM Studio in Motoko

You can configure LM Studio in two ways: via the TUI configuration popup or through a command.

### 1. Via TUI Configuration Form

1. In the Motoko chat interface, type `/provider add` or press the provider shortcut.
2. In the popup form, use the arrow keys (`<` and `>`) to select **`lmstudio`** as the provider preset.
3. The fields will automatically pre-populate with:
   - **Name:** `lmstudio`
   - **Base URL:** `http://127.0.0.1:1234/v1`
   - **API Key:** `lm-studio`
4. If your local LM Studio instance runs on a different port or requires a custom configuration, you can edit these fields.
5. Highlight the **SAVE** button and press `Enter`.

### 2. Via Command Line

You can configure and activate the provider directly using the slash commands:

```bash
# Add the LM Studio provider
/provider add lmstudio lmstudio http://127.0.0.1:1234/v1 lm-studio

# Activate the provider
/provider use lmstudio
```

## Model Selection

Once the provider is activated, you can view and select the model running on your local server.

```bash
# List available models
/models list

# Select the local model to use
/models use <model-name>

# Inspect the model metadata
/models info <model-name>
```

> [!NOTE]
> Since local models may take longer to process inputs (especially with large codebase contexts), Motoko automatically increases the client request timeout to **15 minutes** when executing calls. This prevents connection timeouts during heavy prompt processing.

## Benefits of Local Execution

- **Complete Privacy:** No data leaves your machine. Your codebase context remains local.
- **Zero Token Cost:** Run queries and edit code without incurring commercial API expenses.
- **Offline Mode:** Develop and use Motoko's agentic workspace features completely offline.
