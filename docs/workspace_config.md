# Workspace-Specific Configuration

Motoko allows you to define configurations at two levels: **Global** and **Workspace-Specific**. This enables you to share global provider settings while customizing specific workspaces (projects) to use different models, providers, search patterns, or agent parameters.

## Configuration Paths

1. **Global Configuration:**
   - Resides in your home directory config folder (e.g., `~/.config/motoko/config.json` or equivalent depending on the OS).
   - Contains your main API keys, active providers, and general preferences.

2. **Workspace-Specific Configuration:**
   - Resides in the current project directory at **`<workspace-root>/.agents/config.json`**.
   - This file is optional and can contain all or just a subset of the configuration fields.
3. **Workspace Modes & Skills:**
   - Custom agent modes live in **`<workspace-root>/.agents/modes/*.md`**.
   - Reusable skills live in **`<workspace-root>/.agents/skills/<skill-name>/SKILL.md`**.

## Merging & Priority Rules

When Motoko starts in a workspace, it loads the global configuration first and then loads the workspace-specific configuration (if present). The workspace configuration is merged on top of the global configuration with the following priority rules:

- **Active Provider:** If `active_provider` is defined in the workspace config, it overrides the global active provider for that workspace.
- **Providers:** If a provider defined in the workspace config has the same `name` as a global provider, the workspace configuration fields override the global ones. If the provider name is new, it is appended to the available providers.
- **Search Exclusions:** Workspace-specific search `exclude_patterns` are merged with global patterns, keeping a unique sorted set. Other search fields like `max_results` and `case_sensitive` override global values.
- **Agent Overrides:** Custom agent configurations (e.g., model, temperature, provider, tool filters) in the workspace config are merged with the global agent settings. Any specific override field set in the workspace takes priority.
- **Thinking Verbosity / Iterations:** `thinking_verbosity` and `max_iterations` can be set globally and overridden per workspace via `.agents/config.json`.

## Example Config

Here is an example of a workspace-level `<workspace-root>/.agents/config.json` that configures the workspace to use a local LM Studio model for planning, but fallback to Anthropic Claude for building:

```json
{
  "active_provider": "lmstudio",
  "providers": [
    {
      "name": "lmstudio",
      "preset": "lmstudio",
      "kind": "lmstudio",
      "base_url": "http://127.0.0.1:1234/v1",
      "api_key": "lm-studio",
      "model": "qwen2.5-coder-7b-instruct"
    }
  ],
  "agents": {
    "plan": {
      "model": "qwen2.5-coder-7b-instruct",
      "provider": "lmstudio"
    },
    "build": {
      "provider": "anthropic",
      "model": "claude-4.6-sonnet",
      "temperature": 0.2,
      "max_iterations": 320
    }
  },
  "thinking_verbosity": "concise",
  "max_iterations": 250,
  "search": {
    "exclude_patterns": [
      "coverage.out",
      "*.bin"
    ]
  }
}
```

In this setup:
1. `lmstudio` is registered as a local provider for this project specifically.
2. The **`plan`** agent is overridden to run on the local `qwen2.5-coder-7b-instruct` model via `lmstudio`.
3. The **`build`** agent is configured to use the `anthropic` provider with the `claude-4.6-sonnet` model (loaded from your global configuration keys) with a custom temperature of `0.2` for maximum code generation stability.
4. Custom exclude patterns (`coverage.out`, `*.bin`) are added to the search exclusions list for this workspace.
5. The workspace uses a more concise reasoning style and raises the default iteration budget.

## Custom Modes (`.agents/modes/*.md`)

Custom modes are markdown files with frontmatter plus a system prompt body.

Example:

```md
---
name: "team-helper"
description: "Coordinate multi-agent planning"
readonly: true
allow_question: true
allow_delegate: true
allow_task: false
allow_write: false
allow_brain_write: true
allow_web: true
max_iterations: 120
tool_filter: [read, grep, question, delegate, brain_read, brain_write]
exclude_tools: [bash, patch]
---
You coordinate planning work across multiple subagents and keep plan.md/tasks.md current.
```

Supported frontmatter keys:

- `name`
- `description`
- `readonly`
- `tool_filter`
- `exclude_tools`
- `allow_write`
- `allow_question`
- `allow_delegate`
- `allow_task`
- `allow_brain_write`
- `allow_web`
- `max_iterations`

## Skills (`.agents/skills/<name>/SKILL.md`)

Skills are reusable instruction bundles discovered automatically and loaded through the `activate_skill` tool. Each skill folder contains a `SKILL.md` file with YAML frontmatter (`name`, `description`) followed by markdown instructions.

> [!IMPORTANT]
> Since the workspace configuration resides in your project directory under `.agents/config.json`, you can commit it to Git (unless it contains sensitive API keys). It is recommended to configure API keys globally and reference provider names or model overrides in the workspace config.
