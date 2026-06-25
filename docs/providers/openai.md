# OpenAI Provider

Motoko supports **OpenAI** as a cloud LLM provider, giving access to the GPT-5.5 series of models.

## Prerequisites

1. Create an account on the [OpenAI Developer Platform](https://platform.openai.com/).
2. Generate an API key. Make sure you have balance/credits in your developer account.

## Configuration

You can configure OpenAI either in the interactive UI or via command.

### 1. Via TUI Configuration Form

1. Type `/provider add` in the Chat input.
2. In the popup form, select the **`openai`** preset using the arrow keys (`<` and `>`).
3. The fields will pre-populate with:
   - **Name:** `openai`
   - **API Key:** (Enter your `sk-...` API key)
4. Highlight **SAVE** and press `Enter`.

### 2. Via Command Line

Run the following command to add and activate OpenAI:

```bash
# Add the provider
/provider add openai openai https://api.openai.com/v1 <YOUR_API_KEY>

# Activate it
/provider use openai
```

## Model Selection

After activation, choose a model to run:

```bash
# List available models
/models

# Select the model
/models use gpt-5.5
```

> [!TIP]
> For heavy coding tasks, `gpt-5.5` offers a great balance of speed, reasoning, and tool-calling efficiency.
