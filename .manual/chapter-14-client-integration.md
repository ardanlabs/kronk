# Chapter 14: Client Integration

## Table of Contents

- [14.1 Connection Settings](#141-connection-settings)
- [14.2 OpenCode](#142-opencode)
  - [Install OpenCode](#install-opencode)
  - [Install the Kronk Bundle](#install-the-kronk-bundle)
  - [Configure a Coding Model](#configure-a-coding-model)
  - [Change the OpenCode Model](#change-the-opencode-model)
  - [Use a Protected Server](#use-a-protected-server)
- [14.3 OpenWebUI](#143-openwebui)
- [14.4 Python OpenAI SDK](#144-python-openai-sdk)
- [14.5 curl and Other HTTP Clients](#145-curl-and-other-http-clients)
- [14.6 LangChain](#146-langchain)

---

Kronk's OpenAI-compatible API works with clients that let you configure a
base URL and API key. This chapter covers the OpenCode configuration shipped
with the repository and representative setups for OpenWebUI, the OpenAI
Python SDK, curl, and LangChain.

### 14.1 Connection Settings

Most clients need three values:

| Setting | Value |
| ------- | ----- |
| Base URL | `http://localhost:11435/v1` |
| API key | Any non-empty value in open mode; a valid user token in protected mode |
| Model | An ID returned by `GET /v1/models` |

List the model IDs available to inference clients:

```shell
curl http://localhost:11435/v1/models
```

When inference authentication is enabled, add
`-H "Authorization: Bearer $KRONK_TOKEN"` and use a user token whose endpoint
grants allow the API you are calling. See Chapter 12 for token creation and
Chapter 9 for the supported endpoints.

If the client runs in a container, `localhost` refers to the container rather
than the host. Use the container runtime's host address, such as
`host.docker.internal`, or place both services on the same container network.

### 14.2 OpenCode

OpenCode is the coding agent for which this repository ships a ready-to-use
configuration. It registers Kronk as an OpenAI-compatible provider and
connects OpenCode to Kronk's MCP service.

#### Install OpenCode

Install OpenCode with its official installer:

```shell
curl -fsSL https://opencode.ai/install | bash
```

Other installation options are listed at
[opencode.ai/download](https://opencode.ai/download). Verify the result with:

```shell
opencode --version
```

#### Install the Kronk Bundle

From a Kronk source checkout, run:

```shell
make agents-default-opencode
```

> **Warning:** This target is intended to install the repository's complete
> OpenCode setup. It overwrites `opencode.jsonc`, `tui.jsonc`, `auth.json`, and
> `AGENTS.md` in `~/.config/opencode/`, then replaces that directory's
> `skills/` tree. Back up or merge an existing configuration first.

The installed files provide:

- the Kronk provider at `http://127.0.0.1:11435/v1`;
- registered coding models and a default model;
- direct access to the Kronk and gopls MCP servers;
- local API-key credentials;
- project instructions and the `kronk-mcp` and `writing-go` skills; and
- terminal UI preferences.

The Kronk MCP service starts with the model server and listens at
`http://localhost:9000/mcp` by default. Its `web_search` tool requires a Brave
Search API key; `fuzzy_edit` does not. See Chapter 16 for configuration.

#### Configure a Coding Model

OpenCode's model name must match a model registered in its
`provider.kronk.models` map. Kronk also needs a corresponding model variant in
its model configuration. The shipped default is:

```yaml
unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT:
  context-window: 131072
  nseq-max: 2
  sampling-parameters:
    temperature: 0.6
    top_k: 20
    top_p: 0.95
```

`/AGENT` is a configuration variant: it reuses the downloaded base model while
applying settings intended for coding-agent workloads. The base model must
already be present in Kronk's model directory.

Installed servers use `~/.kronk/models/model_config.yaml` by default. The
repository's development server instead points to
`zarf/kms/model_config.yaml`, which already contains the models registered by
the shipped OpenCode bundle. Restart the server after changing either file.

Incremental Message Caching is enabled by default and is useful for growing
agent conversations. `nseq-max: 2` allows two concurrent sequences, while the
large context window accommodates accumulated messages and tool results. See
Chapters 4 and 5 before changing those settings.

#### Change the OpenCode Model

To use another model:

1. Download its base model into Kronk.
2. Add an `/AGENT` variant to the active model configuration file.
3. Register the same variant under `provider.kronk.models` in
   `~/.config/opencode/opencode.jsonc`.
4. Set the top-level `model` field to `<provider>/<model-id>` or select the
   registered model from OpenCode.

For example:

```yaml
organization/my-coding-model-Q8_0/AGENT:
  context-window: 131072
  nseq-max: 2
  sampling-parameters:
    temperature: 0.6
    top_k: 20
    top_p: 0.95
```

```jsonc
"provider": {
  "kronk": {
    "models": {
      "organization/my-coding-model-Q8_0/AGENT": {
        "name": "My Coding Model",
        "limit": { "context": 131072, "output": 65536 }
      }
    }
  }
},
"model": "kronk/organization/my-coding-model-Q8_0/AGENT"
```

#### Use a Protected Server

The shipped `auth.json` uses `kronk` as a placeholder API key. That is enough
when inference authentication is disabled. For a protected server, replace
the `key` value under `kronk` in `~/.config/opencode/auth.json` with a valid
user token that permits the endpoints OpenCode calls. Do not use the master
admin token as an application credential.

### 14.3 OpenWebUI

OpenWebUI is a self-hosted chat interface that works with Kronk.

For OpenWebUI running directly on the host, configure an OpenAI connection
with:

- **URL:** `http://localhost:11435/v1`
- **API key:** any non-empty value in open mode, or a valid Kronk user token

The repository also includes a Docker Compose service preconfigured to reach
Kronk through `host.docker.internal`:

```shell
make owu-up
make owu-browse
```

OpenWebUI discovers available models through `GET /v1/models` and supports
streaming chat, system prompts, model selection, and conversation history.

### 14.4 Python OpenAI SDK

Use the official OpenAI Python library with Kronk.

```shell
pip install openai
```

Replace `<model-id-from-v1-models>` below with an ID reported by the server.

```python
import os

from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:11435/v1",
    api_key=os.getenv("KRONK_TOKEN", "kronk"),
)

response = client.chat.completions.create(
    model="<model-id-from-v1-models>",
    messages=[
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": "Hello!"},
    ],
    stream=True,
)

for chunk in response:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="")
```

### 14.5 curl and Other HTTP Clients

Any HTTP client can call Kronk's REST API directly.

```shell
curl http://localhost:11435/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -d '{
    "model": "<model-id-from-v1-models>",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": true
  }'
```

Omit the authorization header in open mode. Streaming responses use
Server-Sent Events; Chapter 9 documents endpoint behavior and Chapter 18
covers audio transcription.

### 14.6 LangChain

Use LangChain with Kronk via the OpenAI integration.

```shell
pip install langchain-openai
```

```python
import os

from langchain_openai import ChatOpenAI

llm = ChatOpenAI(
    base_url="http://localhost:11435/v1",
    api_key=os.getenv("KRONK_TOKEN", "kronk"),
    model="<model-id-from-v1-models>",
    streaming=True,
)

response = llm.invoke("Explain quantum computing briefly.")
print(response.content)
```

---

_Next: [Chapter 15: Observability](chapter-15-observability.md)_
