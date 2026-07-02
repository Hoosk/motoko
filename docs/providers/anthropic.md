# Anthropic Provider

Motoko supports **Anthropic** as a cloud LLM provider, providing native integration with Claude models (Claude 4.6 Sonnet, Claude 4.8 Opus, Claude 5 Fable, etc.).

## Prerequisites

1. Get an API key from the [Anthropic Console](https://console.anthropic.com/).
2. Ensure you have billing/credits set up.

## Configuration

### 1. Via TUI Configuration Form

1. Type `/provider add` in the Chat input.
2. In the popup form, select the **`anthropic`** preset using the arrow keys (`<` and `>`).
3. The fields will pre-populate with:
   - **Name:** `anthropic`
   - **API Key:** (Enter your `sk-ant-...` API key)
4. Highlight **SAVE** and press `Enter`.

### 2. Via Command Line

```bash
# Add the provider
/provider add anthropic anthropic https://api.anthropic.com <YOUR_API_KEY>

# Activate it
/provider use anthropic
```

## Model Selection

```bash
# List available models
/models list

# Select the model
/models use claude-4.6-sonnet

# Inspect the model metadata
/models info claude-4.6-sonnet
```

> [!NOTE]
> Motoko automatically negotiates advanced features like prompt caching and thinking budgets when using compatible Claude models, which significantly speeds up workspace scans and saves API costs.
