# Chapter 14: Client Integration

## Table of Contents

- [14.1 Installing OpenCode](#141-installing-opencode)
- [14.2 Coding Agent Model Configuration](#142-coding-agent-model-configuration)
- [14.3 Agent Bundles in `.agents/`](#143-agent-bundles-in-agents)
- [14.4 Default Bundle (Direct MCP)](#144-default-bundle-direct-mcp)
- [14.5 Changing the Model OpenCode Uses](#145-changing-the-model-opencode-uses)
- [14.6 Rote Bundle (MCP via rote)](#146-rote-bundle-mcp-via-rote)
- [14.7 Wiping Agent State](#147-wiping-agent-state)
- [14.8 OpenWebUI](#148-openwebui)
- [14.9 Python OpenAI SDK](#149-python-openai-sdk)
- [14.10 curl and HTTP Clients](#1410-curl-and-http-clients)
- [14.11 LangChain](#1411-langchain)

---

Kronk's OpenAI-compatible API works with popular AI clients, coding
agents, and tools. OpenCode is the only coding agent the project
supports and ships configuration for. This chapter covers installing
OpenCode, wiring it into Kronk via the bundles in `.agents/`, swapping
out the model OpenCode uses, plus a few general-purpose clients
(OpenWebUI, Python SDK, curl, LangChain).

### 14.1 Installing OpenCode

Install OpenCode with the official installer:

```shell
curl -fsSL https://opencode.ai/install | bash
```

For other install options (Homebrew, npm, manual binaries, etc.) see
the official download page:

[https://opencode.ai/download](https://opencode.ai/download)

Verify the install:

```shell
opencode --version
```

Once OpenCode is on your `PATH`, install the Kronk bundle to wire it
into the local Kronk server:

```shell
make agents-default-opencode
```

That target copies a ready-to-run config (provider, MCP, skills,
`AGENTS.md`) into `~/.config/opencode/`. See section 13.4 for the
details. To change which model OpenCode uses after install, see
section 13.5.

### 14.2 Coding Agent Model Configuration

OpenCode and the Kronk server share the same model configuration. The
model is configured in `model_config.yaml` (or the catalog) with an
`/AGENT` suffix that OpenCode references as its model name.

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

The Kronk MCP service exposes two tools to OpenCode:

- `web_search` — Brave-powered web search.
- `fuzzy_edit` — fallback file editor for when the host's exact-match edit
  tool misses on whitespace or line-ending drift.

It starts automatically with the Kronk server on
`http://localhost:9000/mcp`. Both bundles below wire this endpoint into
OpenCode (directly, or through rote).

### 14.3 Agent Bundles in `.agents/`

Two bundles ship in the repo. Pick one based on whether you want Kronk's
MCP service wired directly into OpenCode, or routed through the
[rote](https://www.modiqo.ai/) execution layer.

```
.agents/
├── default/        # Direct MCP — most contributors use this
│   ├── AGENTS.md
│   ├── opencode/
│   │   ├── opencode.jsonc
│   │   └── auth.json
│   └── skills/
│       ├── kronk-mcp/
│       └── writing-go/
└── rote/           # Same host, but MCP traffic goes through rote
    ├── AGENTS.md
    ├── adapters/kronk/
    ├── opencode/
    │   ├── opencode.jsonc
    │   └── auth.json
    ├── skills/
    └── NOTES.md
```

Both bundles ship four pieces to OpenCode's config directory
(`~/.config/opencode/`):

1. `opencode.jsonc` — provider/MCP config.
2. `auth.json` — placeholder API key for local use.
3. `AGENTS.md` — house rules for the agent (mandatory skills,
   editing policy, "never curl `localhost:9000` directly", etc.).
4. `skills/` — at minimum `kronk-mcp` (how to use Kronk's MCP tools)
   and `writing-go` (Go toolchain workflow + post-edit chain).

### 14.4 Default Bundle (Direct MCP)

The default bundle wires Kronk's MCP server directly into OpenCode so
the agent can call `web_search` and `fuzzy_edit` over raw MCP. No extra
runtime layer.

Install the bundle:

```shell
make agents-default-opencode
```

The target creates `~/.config/opencode/` if needed, copies the host
config, drops in `AGENTS.md`, and refreshes the `skills/` tree.
Re-running it is idempotent.

Files installed under `~/.config/opencode/`:

- `opencode.jsonc` — Kronk registered as a custom provider plus MCP
  server entry.
- `auth.json` — placeholder API key for local use.
- `AGENTS.md` — house rules (skill loading policy, editing policy).
- `skills/` — `kronk-mcp`, `writing-go`.

Key settings in `opencode.jsonc`:

```jsonc
{
  "model": "kronk/Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT",
  "provider": {
    "kronk": {
      "npm": "@ai-sdk/openai-compatible",
      "options": { "baseURL": "http://127.0.0.1:11435/v1" },
      "models": {
        "Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT": { ... },
        "gemma-4-26B-A4B-it-UD-Q8_K_XL/AGENT": { ... }
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

OpenCode prefixes MCP tool names with the (lowercase) server name —
`kronk_web_search`, `kronk_fuzzy_edit`.

### 14.5 Changing the Model OpenCode Uses

Swapping the model OpenCode talks to has two parts: register the model
on the Kronk side (so the server can serve it) and tell OpenCode to use
it on the client side.

**1. Make sure the model is configured on the Kronk server.**

Add (or confirm) the model in `zarf/kms/model_config.yaml` with the
`/AGENT` suffix. Use the recommended settings from section 13.2:

```yaml
my-new-model-Q4_K_M/AGENT:
  context-window: 131072
  nseq-max: 2
  incremental-cache: true
  sampling-parameters:
    temperature: 0.6
    top_k: 20
    top_p: 0.95
```

Restart the Kronk server (`make kronk-server`) so the new model is
picked up.

**2. Point OpenCode at the new model.**

Edit `~/.config/opencode/opencode.jsonc` and update two places:

- The top-level `model` field — this is the active default. Format is
  `<provider>/<model-name>`, so for the Kronk provider:

  ```jsonc
  "model": "kronk/my-new-model-Q4_K_M/AGENT"
  ```

- The `provider.kronk.models` map — add a new entry so OpenCode knows
  the model exists and what its limits are:

  ```jsonc
  "models": {
    "my-new-model-Q4_K_M/AGENT": {
      "name": "My New Model Q4_K_M",
      "limit": { "context": 131072, "output": 65536 }
    }
  }
  ```

You can pre-register multiple models in the `models` map and switch
between them inside OpenCode with the `/models` command — the
top-level `model` field just sets the startup default.

**3. Re-shipping the bundle.**

If you want this to be the default for everyone using the bundle (not
just your machine), make the same edits in
`.agents/default/opencode/opencode.jsonc` (and
`.agents/rote/opencode/opencode.jsonc` if you use rote), then re-run
`make agents-default-opencode` (or `make agents-rote-opencode`) on
each machine to push the new config into `~/.config/opencode/`.

### 14.6 Rote Bundle (MCP via rote)

The rote bundle replaces OpenCode's direct MCP wiring with the
[rote](https://www.modiqo.ai/) execution layer. The agent calls Kronk's
MCP tools by shelling out to the `rote` CLI inside a `playground`
workspace, instead of opening an MCP HTTP connection itself.

Rote is **opt-in** — none of these targets are pulled in by
`install-tooling` or by `agents-default-opencode`. Modiqo's registry is
invite-only; see [.agents/rote/NOTES.md](../.agents/rote/NOTES.md) for
the full architecture, file map, and call flow.

**Standard install order:**

```shell
make agents-rote-install   # install the rote CLI
make agents-rote-login     # one-time interactive registry login
make agents-rote-seed      # seed ~/.rote/ with the kronk adapter
                           # and create the playground workspace
make agents-rote-opencode  # ship the rote-aware bundle for OpenCode
```

The `agents-rote-opencode` target ships the same four pieces as the
default bundle, but:

- The host config has **no** `mcp` block (the direct path is removed
  by design).
- `AGENTS.md` and the `kronk-mcp` skill teach the agent to drive Kronk
  via `rote kronk_probe` / `rote kronk_call` from Bash, inside the
  `playground` workspace.

If you don't have a Modiqo invite, use the default bundle.

### 14.7 Wiping Agent State

Use `make agents-wipe` when you want to verify a bundle in isolation,
without leftovers from a previous install. It removes:

- `~/.rote/` (workspaces, adapters, secrets, registry session, caches).
- The `rote` binary on `PATH`, if installed.
- `~/.config/opencode/` in its entirety.

Idempotent — safe to re-run on an already-clean machine. After wiping,
re-install with `make agents-default-opencode` or
`make agents-rote-opencode`.

### 14.8 OpenWebUI

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

### 14.9 Python OpenAI SDK

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

### 14.10 curl and HTTP Clients

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

**Audio Transcription (Bucky):**

```shell
curl -X POST http://localhost:11435/v1/audio/transcriptions \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -F file=@samples/jfk.wav \
  -F model=ggml-tiny.bin \
  -F response_format=json
```

See [Chapter 18 §18.7](chapter-18-bucky.md#187-api-endpoint) for the
full form-field reference and supported response formats
(`json`, `verbose_json`, `text`, `srt`, `vtt`).

### 14.11 LangChain

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

_Next: [Chapter 15: Observability](#chapter-15-observability)_
