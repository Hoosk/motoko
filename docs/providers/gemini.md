# Google Gemini Provider

Motoko supports **Google Gemini** natively using Google GenAI SDK. This provides low-latency execution and high-performance coding support (Gemini 3.5 Pro, Gemini 3.5 Flash, Gemini 3.1 Pro, etc.).

## Prerequisites

1. Get an API key from [Google AI Studio](https://aistudio.google.com/).
2. Gemini has a generous free tier for developers.

## Configuration

### 1. Via TUI Configuration Form

1. Type `/provider add` in the Chat input.
2. In the popup form, select the **`gemini`** preset using the arrow keys (`<` and `>`).
3. The fields will pre-populate with:
   - **Name:** `gemini`
   - **API Key:** (Enter your Gemini API key)
4. Highlight **SAVE** and press `Enter`.

### 2. Via Command Line

```bash
# Add the provider
/provider add gemini gemini https://generativelanguage.googleapis.com <YOUR_API_KEY>

# Activate it
/provider use gemini
```

## Model Selection

```bash
# List available models
/models

# Select the model
/models use gemini-3.5-pro
```

> [!TIP]
> Gemini models feature enormous context windows (up to 2 million tokens). This makes them exceptionally good at working with large codebases where extensive context analysis is needed.
