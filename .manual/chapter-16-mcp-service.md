# Chapter 16: MCP Service

## Table of Contents

- [16.1 Architecture and Security](#161-architecture-and-security)
- [16.2 Prerequisites](#162-prerequisites)
- [16.3 Configuration](#163-configuration)
- [16.4 Available Tools](#164-available-tools)
  - [web_search](#web_search)
  - [fuzzy_edit](#fuzzy_edit)
- [16.5 Client Configuration](#165-client-configuration)
  - [OpenCode](#opencode)
- [16.6 Testing with curl](#166-testing-with-curl)

---

Kronk includes a [Model Context Protocol (MCP)](https://modelcontextprotocol.io/)
service with two tools:

- **`web_search`** searches the web through the
  [Brave Search API](https://brave.com/search/api/).
- **`fuzzy_edit`** replaces a uniquely matching block of text in a local file.

MCP clients discover and invoke these tools through Streamable HTTP. The
repository includes a ready-to-use OpenCode configuration, described in
Chapter 14.

### 16.1 Architecture and Security

The service can run in either of these modes:

- **Embedded (default):** `kronk server start` listens on
  `localhost:9000` when `KRONK_MCP_ENABLED` is true and
  `KRONK_MCP_HOST` is empty.
- **Standalone:** `make mcp-server` runs the MCP service without the model
  server. It listens on `localhost:9000` and starts a debug server on
  `localhost:9010` by default.

The standalone service can also be run directly:

```shell
go run cmd/server/api/services/mcp/main.go
```

`fuzzy_edit` can read and overwrite any file accessible to the service process
when given its absolute path. Keep MCP and debug endpoints bound to loopback by
default. MCP bearer authentication can be enabled as described below, but it
does not provide TLS or protect the separate debug endpoint. If remote access
is required, also use TLS, firewall the listeners, and keep debug endpoints
private.

MCP sessions and replay data are stored in process memory. A restart
invalidates existing session IDs. Compliant clients reinitialize after the
server responds to a stale session with HTTP 404.

### 16.2 Prerequisites

`web_search` requires a Brave Search API key. Obtain one from the
[Brave Search API](https://brave.com/search/api/) site. Search queries are sent
to Brave and are also included in Kronk's structured logs.

`fuzzy_edit` requires no external credentials. It operates with the same
filesystem permissions as the Kronk process.

### 16.3 Configuration

| Variable | Purpose | Default |
| -------- | ------- | ------- |
| `KRONK_MCP_ENABLED` | Enable the embedded MCP listener | `true` |
| `KRONK_MCP_AUTH_ENABLED` | Require a Kronk admin bearer token for embedded MCP | `false` |
| `KRONK_MCP_BRAVE_API_KEY` | Brave key for embedded mode | — |
| `KRONK_MCP_HOST` | Non-empty value disables embedded MCP | — |
| `MCP_MCP_AUTH_ENABLED` | Require admin bearer authentication for standalone MCP | `false` |
| `MCP_MCP_BRAVE_API_KEY` | Brave key for standalone mode | — |
| `MCP_MCP_HOST` | Standalone MCP listen address | `localhost:9000` |
| `MCP_AUTH_HOST` | Auth gRPC service used by protected standalone MCP | — |
| `MCP_WEB_DEBUG_HOST` | Standalone debug listen address | `localhost:9010` |

Start the model server with embedded MCP:

```shell
export KRONK_MCP_BRAVE_API_KEY=<your-brave-api-key>
kronk server start
```

The corresponding CLI option is `--mcp-brave-api-key`. Disable embedded MCP
with `--mcp-enabled=false` or `KRONK_MCP_ENABLED=false`. To use a separately
managed MCP service, set `KRONK_MCP_HOST` or pass `--mcp-host`; a non-empty
value only prevents the embedded service from starting. Kronk does not connect
or proxy to that address.

Protect embedded MCP with the existing Kronk JWT system:

```shell
kronk server start --mcp-auth-enabled
```

This requires an admin bearer token on every MCP request and also enables
administrative authentication for the REST API and BUI. Configure the MCP
client to send `Authorization: Bearer <admin-token>`. Application tokens with
inference endpoint grants are not sufficient for MCP access. Before exposing
the model server outside a trusted host, replace the BUI's default `kronk`
password as described in
[Chapter 13](chapter-13-browser-ui.md#133-authentication-and-session-behavior) or
disable the BUI; otherwise that known password can be exchanged for an admin
session.

Start the standalone service with:

```shell
export MCP_MCP_BRAVE_API_KEY=<your-brave-api-key>
make mcp-server
```

To protect a standalone MCP listener, connect it to an auth service that has
authentication enabled:

```shell
export MCP_MCP_AUTH_ENABLED=true
export MCP_AUTH_HOST=localhost:6000
make mcp-server
```

The standalone MCP service then requires an admin token issued by that auth
service. Startup fails if MCP authentication is enabled without
`MCP_AUTH_HOST`.

### 16.4 Available Tools

#### web_search

Returns matching page titles, URLs, and descriptions as plain text.

| Parameter | Type | Required | Description |
| --------- | ---- | -------- | ----------- |
| `query` | string | Yes | Search query |
| `count` | int | No | Result count; default 10, maximum 20 |
| `country` | string | No | Search country code, such as `US`, `GB`, or `DE` |
| `freshness` | string | No | `pd`, `pw`, `pm`, or `py` for the past day, week, month, or year |
| `safesearch` | string | No | `off`, `moderate`, or `strict`; Brave defaults to `moderate` |

#### fuzzy_edit

Replaces one occurrence of `old_string` with `new_string` in an existing file.
Use it as a fallback when the client's normal exact-match edit fails because
of whitespace or line-ending differences.

The tool tries these matching tiers in order:

1. Exact text.
2. Text after normalizing CRLF and LF line endings.
3. Lines compared without surrounding whitespace.

Each tier must identify exactly one block. An absent or ambiguous match returns
an error without modifying the file. The replacement is inserted as provided.

| Parameter | Type | Required | Description |
| --------- | ---- | -------- | ----------- |
| `file_path` | string | Yes | Absolute path to the existing file |
| `old_string` | string | Yes | Unique text to replace |
| `new_string` | string | Yes | Replacement text |

### 16.5 Client Configuration

Configure an MCP-compatible client to use the Streamable HTTP endpoint:

```text
http://localhost:9000/mcp
```

#### OpenCode

Install the repository's complete OpenCode bundle with
`make agents-default-opencode`; review its overwrite warning in Chapter 14
first. The installed configuration includes:

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

With the shipped `kronk` server key, OpenCode exposes the tools as
`kronk_web_search` and `kronk_fuzzy_edit`.

### 16.6 Testing with curl

The makefile provides commands for a complete stateful MCP handshake. First,
initialize a session and copy the `Mcp-Session-Id` response header:

```shell
make curl-mcp-init
```

Then send the required initialized notification:

```shell
make curl-mcp-initialized SESSIONID=<session-id>
```

The session can now list and call tools:

```shell
make curl-mcp-tools-list SESSIONID=<session-id>
make curl-mcp-web-search SESSIONID=<session-id>
```

When MCP authentication is enabled, include
`Authorization: Bearer <admin-token>` in every initialization, notification,
tool-listing, tool-call, and session-deletion request.

If the service restarts, initialize a new session instead of reusing the old
ID.

---

_Next: [Chapter 17: Troubleshooting](chapter-17-troubleshooting.md)_
