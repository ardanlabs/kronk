# Chapter 15: MCP Service

## Table of Contents

- [15.1 Architecture](#151-architecture)
- [15.2 Prerequisites](#152-prerequisites)
- [15.3 Configuration](#153-configuration)
- [15.4 Available Tools](#154-available-tools)
  - [web_search](#web_search)
  - [fuzzy_edit](#fuzzy_edit)
- [15.5 Client Configuration](#155-client-configuration)
  - [Cline](#cline)
  - [Kilo Code](#kilo-code)
  - [OpenCode](#opencode)
  - [Goose](#goose)
- [15.6 Testing with curl](#156-testing-with-curl)

---



Kronk includes a built-in [Model Context Protocol (MCP)](https://modelcontextprotocol.io/)
service that exposes tools to MCP-compatible clients. Two tools are
provided today:

- **`web_search`** — Powered by the
  [Brave Search API](https://brave.com/search/api/).
- **`fuzzy_edit`** — A tiered-fuzzy-matching file edit tool that is
  more reliable than the built-in `edit` tools used by most coding
  agents.

MCP is an open standard that lets AI agents call external tools over a
simple JSON-RPC protocol. By running the MCP service, any MCP-compatible
client (Cline, Kilo Code, OpenCode, Goose, Cursor, etc.) can discover
and invoke tools served by Kronk.

### 15.1 Architecture

The MCP service can run in two modes:

**Embedded (default)** — When the Kronk model server starts and no external
MCP host is configured (`--mcp-host` is empty), it automatically starts an
embedded MCP server on `localhost:9000`. No extra process is needed.

**Standalone** — Run the MCP service as its own process for independent
scaling or when you don't need the full model server:

```shell
make mcp-server
```

Or directly:

```shell
go run cmd/server/api/services/mcp/main.go
```

Both modes serve the same MCP protocol on the same default port (`9000`).

### 15.2 Prerequisites

The `web_search` tool requires a Brave Search API key. Get a free key at
[https://brave.com/search/api/](https://brave.com/search/api/).

The `fuzzy_edit` tool needs no external credentials — it operates on the
local filesystem and is available as soon as the MCP service starts.

### 15.3 Configuration

**Environment Variables:**

| Variable                | Description                               | Default          |
| ----------------------- | ----------------------------------------- | ---------------- |
| `MCP_MCP_HOST`          | MCP listen address (standalone mode)      | `localhost:9000` |
| `MCP_MCP_BRAVEAPIKEY`   | Brave Search API key (standalone mode)    | —                |
| `KRONK_MCP_HOST`        | External MCP host (empty = embedded mode) | —                |
| `KRONK_MCP_BRAVEAPIKEY` | Brave Search API key (embedded mode)      | —                |

**Embedded mode** — Pass the Brave API key when starting the Kronk server:

```shell
export KRONK_MCP_BRAVEAPIKEY=<your-brave-api-key>
kronk server start
```

The embedded MCP server will start automatically on `localhost:9000`.

**Standalone mode** — Start the MCP service as a separate process:

```shell
export MCP_MCP_BRAVEAPIKEY=<your-brave-api-key>
make mcp-server
```

### 15.4 Available Tools

#### web_search

Performs a web search and returns a list of relevant web pages with titles,
URLs, and descriptions.

**Parameters:**

| Parameter    | Type   | Required | Description                                                                                 |
| ------------ | ------ | -------- | ------------------------------------------------------------------------------------------- |
| `query`      | string | Yes      | Search query                                                                                |
| `count`      | int    | No       | Number of results to return (default 10, max 20)                                            |
| `country`    | string | No       | Country code for search context (e.g. `US`, `GB`, `DE`)                                     |
| `freshness`  | string | No       | Filter by freshness: `pd` (past day), `pw` (past week), `pm` (past month), `py` (past year) |
| `safesearch` | string | No       | Safe search filter: `off`, `moderate`, `strict` (default `moderate`)                        |

#### fuzzy_edit

Edits a file by replacing one occurrence of `old_string` with
`new_string`. Useful for coding agents whose built-in edit tools are
brittle around whitespace or line endings.

The tool tries three matching strategies in order and stops at the first
that produces exactly one match:

1. **Exact** — byte-for-byte `strings.Replace`.
2. **Line-ending normalized** — folds `\r\n` → `\n` on both sides, then
   exact-matches; preserves the file's original line endings on write.
3. **Indentation insensitive** — strips leading whitespace per line for
   comparison; replacement text is inserted as-is.

If no tier yields exactly one match, the call returns an error and the
file is not modified.

**Parameters:**

| Parameter    | Type   | Required | Description                                                              |
| ------------ | ------ | -------- | ------------------------------------------------------------------------ |
| `file_path`  | string | Yes      | Absolute path to the file to edit                                        |
| `old_string` | string | Yes      | Text to find in the file (fuzzy whitespace matching is applied)          |
| `new_string` | string | Yes      | Replacement text                                                         |

### 15.5 Client Configuration

The MCP service uses the Streamable HTTP transport. Configure your
MCP-compatible client to connect to `http://localhost:9000/mcp`.

#### Cline

Add the following to your Cline MCP settings
(`~/.cline/data/settings/cline_mcp_settings.json`):

```json
{
  "mcpServers": {
    "Kronk": {
      "autoApprove": ["web_search", "fuzzy_edit"],
      "disabled": false,
      "timeout": 60,
      "type": "streamableHttp",
      "url": "http://localhost:9000/mcp"
    }
  }
}
```

#### Kilo Code

Add the following to your Kilo Code MCP settings
(inside `~/.config/kilo/kilo.json`):

```json
{
  "mcpServers": {
    "Kronk": {
      "type": "streamable-http",
      "url": "http://localhost:9000/mcp",
      "disabled": false,
      "alwaysAllow": ["web_search", "fuzzy_edit"],
      "timeout": 60
    }
  }
}
```

Kilo prefixes MCP tool names with the server key, so the tools are
exposed to the model as `Kronk_web_search` and `Kronk_fuzzy_edit`.

#### OpenCode

Inside `~/.config/opencode/opencode.jsonc`:

```jsonc
{
  "mcp": {
    "kronk": {
      "type": "remote",
      "url": "http://localhost:9000/mcp"
    }
  }
}
```

OpenCode lowercases the server prefix, so the tools are exposed as
`kronk_web_search` and `kronk_fuzzy_edit`.

#### Goose

Goose discovers tools from any MCP server it is configured against.
Add an entry to your Goose configuration that points to
`http://localhost:9000/mcp` using the streamable-HTTP transport. Both
`web_search` and `fuzzy_edit` will appear in the tool list once the
connection is established.

### 15.6 Testing with curl

You can test the MCP service manually using curl. See the makefile targets
for convenience commands.

**Initialize a session:**

```shell
make curl-mcp-init
```

This returns the `Mcp-Session-Id` header needed for subsequent requests.

**List available tools:**

```shell
make curl-mcp-tools-list SESSIONID=<session-id>
```

**Call web_search:**

```shell
make curl-mcp-web-search SESSIONID=<session-id>
```

---

_Next: [Chapter 16: Troubleshooting](#chapter-16-troubleshooting)_
