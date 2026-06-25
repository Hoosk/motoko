# OpenAI-Compatible Provider

Motoko supports any OpenAI-compatible endpoint. This enables integration with tools like **Ollama**, **OpenRouter**, **DeepSeek**, **vLLM**, or other proxy servers using standard chat completion APIs.

## Typical Implementations

### 1. Ollama (Local)

[Ollama](https://ollama.com/) allows you to run models locally. By default, its OpenAI-compatible endpoint runs on port `11434`.

- **Preset:** `openai-compatible`
- **Base URL:** `http://localhost:11434/v1`
- **API Key:** `ollama` (or any dummy text)

Command configuration:
```bash
/provider add ollama openai-compatible http://localhost:11434/v1 ollama
/provider use ollama
```

### 2. OpenRouter (Cloud)

[OpenRouter](https://openrouter.ai/) aggregates various commercial and open-source models under a single API.

- **Preset:** `openai-compatible`
- **Base URL:** `https://openrouter.ai/api/v1`
- **API Key:** (Your OpenRouter API Key)

Command configuration:
```bash
/provider add openrouter openai-compatible https://openrouter.ai/api/v1 <YOUR_API_KEY>
/provider use openrouter
```

### 3. Custom Local/Cloud Services (vLLM, DeepSeek, LiteLLM)

For any other proxy or self-hosted API compatible with the OpenAI specification:

- **Preset:** `openai-compatible`
- **Base URL:** `http://<your-host>:<port>/v1`
- **API Key:** (Your server API Key, or dummy text if unauthenticated)

---

## Model Selection

Once the provider is added and activated:

```bash
# List available models served by the endpoint
/models

# Select the model to use
/models use <model-identifier>
```

> [!NOTE]
> When adding custom OpenAI-compatible endpoints, Motoko automatically normalizes the URL. If you provide a URL without the `/v1` path suffix (e.g., `http://localhost:11434`), Motoko will automatically append `/v1` to ensure correct communication with the endpoint.
