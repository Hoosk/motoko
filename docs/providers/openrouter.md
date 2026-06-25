# OpenRouter Provider

Motoko supports **OpenRouter** as a provider, allowing you to access a wide variety of open-source and commercial models (like Claude 4.6 Sonnet, Llama 4, Mistral, Qwen, or DeepSeek) via a unified OpenAI-compatible API.

## Prerequisites

1. Create an account on [OpenRouter](https://openrouter.ai/).
2. Generate an API Key. Ensure you have balance/credits in your account to query commercial models.

## Configuration

OpenRouter is supported as a configuration preset. You can configure it either via the UI or by command.

### 1. Via TUI Configuration Form

1. Type `/provider add` in the Chat input.
2. In the popup form, select the **`openrouter`** preset using the arrow keys (`<` and `>`).
3. The fields will pre-populate with:
   - **Name:** `openrouter`
   - **Base URL:** `https://openrouter.ai/api/v1`
   - **API Key:** (Enter your OpenRouter API Key)
4. Highlight **SAVE** and press `Enter`.

### 2. Via Command Line

```bash
# Add the provider
/provider add openrouter openrouter https://openrouter.ai/api/v1 <YOUR_API_KEY>

# Activate it
/provider use openrouter
```

## Model Selection

Once OpenRouter is activated, you can select any model supported by the OpenRouter platform. Use OpenRouter's model identifiers (e.g., `meta-llama/llama-4-8b-instruct:free`, `anthropic/claude-4.6-sonnet`, etc.):

```bash
# List all models available on the endpoint
/models

# Select the model to use
/models use <model-identifier>
```

> [!NOTE]
> Under the hood, Motoko normalizes the OpenRouter preset to use the `openai-compatible` client kind, enabling seamless standard OpenAI chat completion API compatibility while keeping the implementation lightweight.
