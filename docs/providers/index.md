# LLM Providers

Motoko supports multiple Large Language Model (LLM) providers, ranging from commercial cloud APIs to local-first runners. This directory contains detailed guides on how to configure and use each provider.

## Supported Providers

Click on a provider below to view its configuration guide:

* **[LM Studio](./lmstudio.md)** (Local-first)
  - Preset: `lmstudio`
  - Default URL: `http://127.0.0.1:1234/v1`
  - Great for full offline development and privacy.

* **[OpenAI](./openai.md)** (Cloud API)
  - Preset: `openai`
  - Default URL: `https://api.openai.com/v1`
  - Industry standard models (GPT-5.5 series).

* **[Anthropic](./anthropic.md)** (Cloud API)
  - Preset: `anthropic`
  - Default URL: `https://api.anthropic.com`
  - Advanced reasoning models (Claude 4.6 Sonnet).

* **[Google Gemini](./gemini.md)** (Cloud API)
  - Preset: `gemini`
  - Default URL: `https://generativelanguage.googleapis.com`
  - Native integration with high-speed Gemini 3.5 models.

* **[OpenAI-Compatible / Ollama](./openai_compatible.md)** (Custom endpoints)
  - Preset: `openai-compatible`
  - Default URL: `http://localhost:11434/v1` (Ollama default)
  - Generic client for custom local/cloud servers (Ollama, vLLM, DeepSeek, etc.).

* **[OpenRouter](./openrouter.md)** (Cloud aggregator)
  - Preset: `openrouter`
  - Default URL: `https://openrouter.ai/api/v1`
  - Access to a vast range of hosted open-source and commercial models.

---

## Dynamic Catalog-based Providers (models.dev)

Motoko dynamically queries the `models.dev` catalog to discover and configure providers. When you add a provider using the interactive TUI:
1. Run `/provider add` in the CLI to open the configuration form.
2. Select the provider preset by highlighting the **Provider** field and pressing **Enter**. This opens an interactive, searchable provider picker listing all catalog-defined providers (such as DeepSeek, Mistral, xAI, etc.).
3. Choose your provider from the list. Motoko will automatically pre-populate the **Name** and **Base URL** using the catalog values.
4. Input your **API Key** and select **Save** to activate it.

Catalog providers behave as `openai-compatible` endpoints under the hood, but preserve their custom preset names for automatic model lookup and base URL resolution.

---

## Configuration Commands Quick Reference

### List Configured Providers
```bash
/provider list
```

### Activate a Provider
```bash
/provider use <provider-name>
```

### Remove a Provider
```bash
/provider remove <provider-name>
```

### Select a Model
```bash
# List models available on the active provider
/models

# Use a specific model
/models use <model-name>
```
