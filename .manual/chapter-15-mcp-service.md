# Chapter 15: MCP Service

## Table of Contents

- [15.1 Architecture](#151-architecture)
- [15.2 Prerequisites](#152-prerequisites)
- [15.3 Configuration](#153-configuration)
- [15.4 Available Tools](#154-available-tools)
  - [web_search](#web_search)
- [15.5 Client Configuration](#155-client-configuration)
  - [Cline](#cline)
  - [Kilo Code](#kilo-code)
- [15.6 Testing with curl](#156-testing-with-curl)

---



Kronk includes a built-in [Model Context Protocol (MCP)](https://modelcontextprotocol.io/)
service that exposes tools to MCP-compatible clients. The initial tool
provided is `web_search`, powered by the [Brave Search API](https://brave.com/search/api/).

MCP is an open standard that lets AI agents call external tools over a
simple JSON-RPC protocol. By running the MCP service, any MCP-compatible
client (Cline, Kilo Code, Cursor, etc.) can discover and invoke tools
served by Kronk.

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

### 15.5 Client Configuration

The MCP service uses the Streamable HTTP transport. Configure your
MCP-compatible client to connect to `http://localhost:9000/mcp`.

#### Cline

Add the following to your Cline MCP settings:

```json
{
  "mcpServers": {
    "Kronk": {
      "autoApprove": ["web_search"],
      "disabled": false,
      "timeout": 60,
      "type": "streamableHttp",
      "url": "http://localhost:9000/mcp"
    }
  }
}
```

#### Kilo Code

Add the following to your Kilo Code MCP settings:

```json
{
  "mcpServers": {
    "Kronk": {
      "type": "streamable-http",
      "url": "http://localhost:9000/mcp",
      "disabled": true,
      "alwaysAllow": ["web_search"],
      "timeout": 60
    }
  }
}
```

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
