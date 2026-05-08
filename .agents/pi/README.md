# Pi Configuration

Configuration files for the [Pi](https://pi.dev) coding agent to work with Kronk.

## Files

| File         | Purpose                                                              |
| ------------ | -------------------------------------------------------------------- |
| `models.json` | Custom Kronk provider and model definitions                          |
| `mcp.json`    | Kronk MCP server registration (for the `pi-mcp-adapter` extension)   |
| `AGENTS.md`   | Custom instructions injected into the system prompt                  |

## Installation

Pi reads its configuration from `~/.pi/agent` by default (override with `PI_CODING_AGENT_DIR`).

```bash
mkdir -p ~/.pi/agent
cp models.json ~/.pi/agent/models.json
cp mcp.json    ~/.pi/agent/mcp.json
cp AGENTS.md   ~/.pi/agent/AGENTS.md
```

Pi has no built-in MCP support. Install the official MCP adapter extension first:

```bash
pi package add @mariozechner/pi-mcp-adapter
```

After the first `pi` session that loads `mcp.json`, prime the direct-tools
cache so `kronk_fuzzy_edit` shows up as a top-level tool instead of only
through the `mcp` proxy:

```
/mcp reconnect kronk
```

This writes `~/.pi/agent/mcp-cache.json`. Subsequent sessions register
`kronk_fuzzy_edit` directly at startup.

## Prerequisites

- The Kronk server must be running on `http://127.0.0.1:11435`
- The Kronk MCP service must be running on `http://localhost:9000/mcp`
- The `pi-mcp-adapter` extension must be installed

This will happen with the default settings when you start the kronk model server.

## Selecting the Model

Pi does not auto-select a default model. Pick one in-session with `/model` (or
cycle favorites with `Ctrl+P`), or pass it on the CLI:

```bash
pi --model "Qwen3.6 35B-A3B UD-Q4_K_M"
pi --model "Gemma4 26B-A4B UD-Q4_K_M"
```

Pi remembers the last used model across sessions.

## Notes

- The `AGENTS.md` file instructs the model to use Kronk's `kronk_fuzzy_edit` MCP tool instead of Pi's built-in `edit` tool. This provides more reliable file edits through tiered fuzzy matching.
- `pi-mcp-adapter` prefixes MCP tool names with the server name (e.g., `kronk` server → `kronk_fuzzy_edit`).
- `directTools: true` in `mcp.json` registers each Kronk MCP tool as an individual Pi tool instead of going through the `mcp` proxy tool, so `kronk_fuzzy_edit` is available directly to the model.
- The `apiKey` in `models.json` is a placeholder since Kronk runs locally. Update this if you enable authentication on the Kronk server.
- `compat.supportsDeveloperRole` and `compat.supportsReasoningEffort` are set to `false` because Kronk's OpenAI-compatible endpoint does not implement those OpenAI-specific extensions.
