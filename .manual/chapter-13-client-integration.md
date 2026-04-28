# Chapter 13: Client Integration

## Table of Contents

- [13.1 Coding Agent Model Configuration](#131-coding-agent-model-configuration)
- [13.2 Cline](#132-cline)
- [13.3 Kilo Code](#133-kilo-code)
- [13.4 OpenCode](#134-opencode)
- [13.5 Goose](#135-goose)
- [13.6 OpenWebUI](#136-openwebui)
- [13.7 Python OpenAI SDK](#137-python-openai-sdk)
- [13.8 curl and HTTP Clients](#138-curl-and-http-clients)
- [13.9 LangChain](#139-langchain)

---

Kronk's OpenAI-compatible API works with popular AI clients, coding agents,
and tools. This chapter covers configuration for the CLI-style coding
agents that talk to Kronk, plus a few general-purpose clients.

Reference configuration files for each agent are provided in the `.agents/`
directory at the project root. These files are ready to copy into each
agent's CLI config directory.

```
.agents/
├── cline/       # Cline (~/.cline)
├── goose/       # Goose (~/.config/goose)
├── kilo/        # Kilo Code (~/.config/kilo)
└── opencode/    # OpenCode (~/.config/opencode)
```

### 13.1 Coding Agent Model Configuration

All coding agents share the same Kronk server and model configuration. The
model is configured in `model_config.yaml` (or the catalog) with an `/AGENT`
suffix that the agent references as its model name.

**Recommended Configuration:**

```yaml
Qwen3.6-35B-A3B-UD-Q4_K_M/AGENT:
  context-window: 131072
  nseq-max: 2
  incremental-cache: true
  sampling-parameters:
    temperature: 0.6
    top_k: 20
    top_p: 0.95
```

Another model that works well for coding:

```yaml
gemma-4-26B-A4B-it-UD-Q4_K_M/AGENT:
  context-window: 131072
  nseq-max: 2
  incremental-cache: true
  sampling-parameters:
    temperature: 1.0
    top_k: 64
    top_p: 0.95
```

See `zarf/kms/model_config.yaml` for additional pre-configured examples.

**Why these settings matter:**

- **`incremental-cache: true`** — IMC caches the conversation prefix in RAM
  between requests, so only the new message needs prefilling on each turn.
  This is essential for iterative coding workflows where conversations grow
  to tens of thousands of tokens.
- **`nseq-max: 2`** — Two sessions allow the agent's main conversation and
  a sub-agent to run concurrently without evicting each other's cache.
- **`context-window: 131072`** — Large context windows are important for
  coding agents that accumulate tool results, file contents, and long
  conversations.

**MCP Service:**

The Kronk MCP service provides tools (like `web_search`) to coding agents.
It starts automatically with the Kronk server on `http://localhost:9000/mcp`.
All agent configs below reference this endpoint.

### 13.2 Cline

[Cline](https://cline.bot) is a coding agent that stores its state under
`~/.cline/`.

**Installation:**

Copy the MCP settings file from `.agents/cline/` into Cline's settings
directory:

```bash
cp .agents/cline/cline_mcp_settings.json \
   ~/.cline/data/settings/cline_mcp_settings.json
```

This registers Kronk's MCP service so Cline can discover the
`web_search` and other tools served from `http://localhost:9000/mcp`.

**Connection settings:**

Point Cline at the Kronk Web API:

```
Base URL: http://localhost:11435/v1
API Key: <your-kronk-token> or 123 if auth is disabled
Model:   Qwen3.6-35B-A3B-UD-Q4_K_M/AGENT
```

The `.agents/cline/globalState.json` file is included as a reference for
which fields Cline expects (model id, base URL, auto-approval settings).
It is not meant to be copied wholesale — Cline manages this file itself.

Reference files: `.agents/cline/`

### 13.3 Kilo Code

[Kilo Code](https://kilocode.ai) is a coding agent that reads its
configuration from `~/.config/kilo/`.

**Installation:**

Copy the config files from `.agents/kilo/` to Kilo's config directory:

```bash
cp .agents/kilo/agent.md  ~/.config/kilo/agent.md
cp .agents/kilo/kilo.json ~/.config/kilo/kilo.json
```

The `kilo.json` configures Kronk as a custom provider with model definitions
and MCP settings. The `agent.md` file provides custom instructions that tell
the model to use Kronk's `kronk_fuzzy_edit` MCP tool for file edits.

**Key settings in `kilo.json`:**

```json
{
  "model": "Qwen3.6-35B-A3B-UD-Q4_K_M/AGENT",
  "provider": {
    "kronk": {
      "npm": "@ai-sdk/openai-compatible",
      "options": {
        "baseURL": "http://localhost:11435/v1",
        "apiKey": "123"
      }
    }
  },
  "mcp": {
    "Kronk": {
      "type": "remote",
      "url": "http://localhost:9000/mcp"
    }
  }
}
```

_Note: Kilo prefixes MCP tool names with the server name (e.g., `Kronk`
server → `Kronk_fuzzy_edit`). If you see tool name mismatches, check the
MCP server key in `kilo.json`._

Reference files: `.agents/kilo/`

### 13.4 OpenCode

[OpenCode](https://opencode.ai) is a terminal-based coding agent.

**Installation:**

Copy the config files from `.agents/opencode/` to your OpenCode config
directory:

```bash
cp .agents/opencode/agent.md       ~/.config/opencode/agent.md
cp .agents/opencode/auth.json      ~/.config/opencode/auth.json
cp .agents/opencode/opencode.jsonc ~/.config/opencode/opencode.jsonc
```

The `opencode.jsonc` configures Kronk as a custom provider. The `agent.md`
file provides custom instructions that tell the model to use Kronk's
`kronk_fuzzy_edit` MCP tool for file edits. The `auth.json` file provides
a placeholder API key for local use.

**Key settings in `opencode.jsonc`:**

```json
{
  "model": "kronk/Qwen3.6-35B-A3B-UD-Q4_K_M/AGENT",
  "provider": {
    "kronk": {
      "npm": "@ai-sdk/openai-compatible",
      "options": {
        "baseURL": "http://127.0.0.1:11435/v1"
      }
    }
  },
  "mcp": {
    "kronk": {
      "type": "remote",
      "url": "http://localhost:9000/mcp"
    }
  }
}
```

_Note: OpenCode prefixes MCP tool names with the server name in lowercase
(e.g., `kronk` server → `kronk_fuzzy_edit`)._

Reference files: `.agents/opencode/`

### 13.5 Goose

[Goose](https://block.github.io/goose/) is a terminal-based AI agent from
Block.

**Installation:**

Copy the config from `.agents/goose/` to Goose's config directory:

```bash
cp .agents/goose/config.yaml       ~/.config/goose/config.yaml
cp .agents/goose/custom_kronk.json ~/.config/goose/custom_providers/custom_kronk.json
```

**Key settings in `config.yaml`:**

```yaml
GOOSE_PROVIDER: kronk
GOOSE_MODEL: Qwen3.6-35B-A3B-UD-Q4_K_M/AGENT
```

The `custom_kronk.json` file configures the Kronk provider connection.

Reference files: `.agents/goose/`

### 13.6 OpenWebUI

OpenWebUI is a self-hosted chat interface that works with Kronk.

**Configure OpenWebUI:**

1. Open OpenWebUI settings
2. Navigate to Connections → OpenAI API
3. Set the base URL:

```
http://localhost:11435/v1
```

4. Set API key to your Kronk token (or any value if auth is disabled)
5. Save and refresh models

**Features that work:**

- Chat completions with streaming
- Model selection from available models
- System prompts
- Conversation history

### 13.7 Python OpenAI SDK

Use the official OpenAI Python library with Kronk.

**Installation:**

```shell
pip install openai
```

**Usage:**

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:11435/v1",
    api_key="your-kronk-token"  # Or any string if auth disabled
)

response = client.chat.completions.create(
    model="Qwen3.6-35B-A3B-UD-Q4_K_M/AGENT",
    messages=[
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": "Hello!"}
    ],
    stream=True
)

for chunk in response:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="")
```

### 13.8 curl and HTTP Clients

Any HTTP client can call Kronk's REST API directly.

**Basic Request:**

```shell
curl http://localhost:11435/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -d '{
    "model": "Qwen3.6-35B-A3B-UD-Q4_K_M/AGENT",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": true
  }'
```

**Streaming Response:**

Streaming responses use Server-Sent Events (SSE) format:

```
data: {"id":"...","choices":[{"delta":{"content":"Hello"}}],...}

data: {"id":"...","choices":[{"delta":{"content":"!"}}],...}

data: [DONE]
```

### 13.9 LangChain

Use LangChain with Kronk via the OpenAI integration.

**Installation:**

```shell
pip install langchain-openai
```

**Usage:**

```python
from langchain_openai import ChatOpenAI

llm = ChatOpenAI(
    base_url="http://localhost:11435/v1",
    api_key="your-kronk-token",
    model="Qwen3.6-35B-A3B-UD-Q4_K_M/AGENT",
    streaming=True
)

response = llm.invoke("Explain quantum computing briefly.")
print(response.content)
```

---

_Next: [Chapter 14: Observability](chapter-14-observability.md)_
