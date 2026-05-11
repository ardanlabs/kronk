# Chapter 13: Client Integration

## Table of Contents

- [13.1 Coding Agent Model Configuration](#131-coding-agent-model-configuration)
- [13.2 Agent Bundles in `.agents/`](#132-agent-bundles-in-agents)
- [13.3 Default Bundle (Direct MCP)](#133-default-bundle-direct-mcp)
  - [13.3.1 OpenCode](#1331-opencode)
  - [13.3.2 Kilo Code](#1332-kilo-code)
  - [13.3.3 Pi](#1333-pi)
  - [13.3.4 Goose](#1334-goose)
- [13.4 Rote Bundle (MCP via rote)](#134-rote-bundle-mcp-via-rote)
- [13.5 Wiping Agent State](#135-wiping-agent-state)
- [13.6 OpenWebUI](#136-openwebui)
- [13.7 Python OpenAI SDK](#137-python-openai-sdk)
- [13.8 curl and HTTP Clients](#138-curl-and-http-clients)
- [13.9 LangChain](#139-langchain)

---

Kronk's OpenAI-compatible API works with popular AI clients, coding agents,
and tools. This chapter covers configuration for the CLI-style coding
agents that talk to Kronk, plus a few general-purpose clients.

Reference configuration files for each agent are provided in the `.agents/`
directory at the project root. Each supported host has a ready-to-deploy
bundle, installed via a `make` target — you do not hand-edit each host
config.

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

**Kronk MCP Service:**

The Kronk MCP service exposes two tools to coding agents:

- `web_search` — Brave-powered web search.
- `fuzzy_edit` — fallback file editor for when the host's exact-match edit
  tool misses on whitespace or line-ending drift.

It starts automatically with the Kronk server on
`http://localhost:9000/mcp`. Every bundle below wires this endpoint into
the host (directly, or through rote).

### 13.2 Agent Bundles in `.agents/`

Two bundles ship in the repo. Pick one based on whether you want Kronk's
MCP service wired directly into your agent host, or routed through the
[rote](https://www.modiqo.ai/) execution layer.

```
.agents/
├── default/        # Direct MCP — most contributors use this
│   ├── AGENTS.md
│   ├── opencode/
│   ├── kilo/
│   ├── pi/
│   ├── goose/
│   └── skills/
│       ├── kronk-mcp/
│       └── writing-go/
└── rote/           # Same hosts, but MCP traffic goes through rote
    ├── AGENTS.md
    ├── adapters/kronk/
    ├── opencode/
    ├── kilo/
    ├── pi/
    ├── goose/
    ├── skills/
    └── NOTES.md
```

Both bundles ship four pieces to each host's config directory:

1. The host's provider/MCP config (`opencode.jsonc`, `kilo.json`, etc.).
2. An `AGENTS.md` file — house rules for the agent (mandatory skills,
   editing policy, "never curl `localhost:9000` directly", etc.).
3. A `skills/` tree — at minimum `kronk-mcp` (how to use Kronk's MCP
   tools) and `writing-go` (Go toolchain workflow + post-edit chain).
4. Per-host extras (e.g. `auth.json` for OpenCode, `custom_kronk.json`
   for Goose).

Supported hosts: **OpenCode**, **Kilo Code**, **Pi**, **Goose**. Cline is
no longer supported.

### 13.3 Default Bundle (Direct MCP)

The default bundle wires Kronk's MCP server directly into each host so
the agent can call `web_search` and `fuzzy_edit` over raw MCP. No extra
runtime layer.

Install the bundle for the host you actually use:

```shell
make agents-default-opencode
make agents-default-kilo
make agents-default-pi
make agents-default-goose
```

Each target creates the host's config directory if needed, copies the
host config, drops in `AGENTS.md`, and refreshes the `skills/` tree.
Re-running a target is idempotent.

#### 13.3.1 OpenCode

Target: `make agents-default-opencode`

Files installed under `~/.config/opencode/`:

- `opencode.jsonc` — Kronk registered as a custom provider plus MCP
  server entry.
- `auth.json` — placeholder API key for local use.
- `AGENTS.md` — house rules (skill loading policy, editing policy).
- `skills/` — `kronk-mcp`, `writing-go`.

Key settings in `opencode.jsonc`:

```jsonc
{
  "model": "kronk/Qwen3.6-35B-A3B-UD-Q4_K_M/AGENT",
  "provider": {
    "kronk": {
      "npm": "@ai-sdk/openai-compatible",
      "options": { "baseURL": "http://127.0.0.1:11435/v1" }
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

OpenCode prefixes MCP tool names with the (lowercase) server name —
`kronk_web_search`, `kronk_fuzzy_edit`.

#### 13.3.2 Kilo Code

Target: `make agents-default-kilo`

Files installed under `~/.config/kilo/`:

- `kilo.json` — Kronk provider, MCP entry, model definitions.
- `AGENTS.md` — house rules.
- `skills/` — `kronk-mcp`, `writing-go`.

Key settings in `kilo.json`:

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
      "url": "http://localhost:9000/mcp",
      "enabled": true,
      "timeout": 60000
    }
  }
}
```

Kilo prefixes MCP tool names with the server name (capitalized as
configured) — `Kronk_web_search`, `Kronk_fuzzy_edit`.

#### 13.3.3 Pi

Target: `make agents-default-pi`

Files installed under `~/.pi/`:

- `agent/models.json` — Kronk provider + model definitions.
- `agent/mcp.json` — Kronk MCP server entry (`directTools: true`).
- `AGENTS.md` — house rules.
- `skills/` — `kronk-mcp`, `writing-go`.

Because Pi sets `directTools: true`, MCP tool names are exposed without
the server prefix: `web_search`, `fuzzy_edit`.

#### 13.3.4 Goose

Target: `make agents-default-goose`

Files installed under `~/.config/goose/`:

- `config.yaml` — selects Kronk provider/model and configures Goose
  built-in extensions.
- `custom_providers/custom_kronk.json` — Kronk provider definition
  (OpenAI-compatible engine, `http://localhost:11435/v1`).
- `AGENTS.md` — house rules.
- `skills/` — `kronk-mcp`, `writing-go`.

Key settings in `config.yaml`:

```yaml
GOOSE_PROVIDER: kronk
GOOSE_MODEL: gemma-4-26B-A4B-it-UD-Q4_K_M/AGENT
```

### 13.4 Rote Bundle (MCP via rote)

The rote bundle replaces the host's direct MCP wiring with the
[rote](https://www.modiqo.ai/) execution layer. The agent calls Kronk's
MCP tools by shelling out to the `rote` CLI inside a `playground`
workspace, instead of opening an MCP HTTP connection itself.

Rote is **opt-in** — none of these targets are pulled in by
`install-tooling` or any default-bundle target. Modiqo's registry is
invite-only; see [.agents/rote/NOTES.md](../.agents/rote/NOTES.md) for
the full architecture, file map, and call flow.

**Standard install order:**

```shell
make agents-rote-install   # install the rote CLI
make agents-rote-login     # one-time interactive registry login
make agents-rote-seed      # seed ~/.rote/ with the kronk adapter
                           # and create the playground workspace
make agents-rote-<host>    # ship the rote-aware bundle for your host
```

Per-host targets:

```shell
make agents-rote-opencode
make agents-rote-kilo
make agents-rote-pi
make agents-rote-goose
```

Each per-host target ships the same four pieces as the default bundle,
but:

- The host config has **no** `mcp` block (the direct path is removed
  by design).
- `AGENTS.md` and the `kronk-mcp` skill teach the agent to drive Kronk
  via `rote kronk_probe` / `rote kronk_call` from Bash, inside the
  `playground` workspace.

If you don't have a Modiqo invite, use the default bundle.

### 13.5 Wiping Agent State

Use `make agents-wipe` when you want to verify a bundle in isolation,
without leftovers from a previous install. It removes:

- `~/.rote/` (workspaces, adapters, secrets, registry session, caches).
- The `rote` binary on `PATH`, if installed.
- `~/.config/opencode/`, `~/.config/kilo/`, `~/.pi/`, `~/.config/goose/`
  in their entirety.

Idempotent — safe to re-run on an already-clean machine. After wiping,
re-install with `make agents-default-<host>` or
`make agents-rote-<host>`.

### 13.6 OpenWebUI

OpenWebUI is a self-hosted chat interface that works with Kronk.

**Configure OpenWebUI:**

1. Open OpenWebUI settings.
2. Navigate to Connections → OpenAI API.
3. Set the base URL:

```
http://localhost:11435/v1
```

4. Set API key to your Kronk token (or any value if auth is disabled).
5. Save and refresh models.

**Features that work:**

- Chat completions with streaming.
- Model selection from available models.
- System prompts.
- Conversation history.

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
