# OpenCode Configuration

Configuration files for the [OpenCode](https://opencode.ai) coding agent to work with Kronk.

## Files

| File             | Purpose                                             |
| ---------------- | --------------------------------------------------- |
| `opencode.jsonc` | Provider, model, MCP, and compaction settings       |
| `agent.md`       | Custom instructions injected into the system prompt |
| `auth.json`      | API key configuration for the Kronk provider        |

## Installation

Copy these files to your OpenCode config directory:

```bash
cp agent.md       ~/.config/opencode/agent.md
cp auth.json      ~/.config/opencode/auth.json
cp opencode.jsonc ~/.config/opencode/opencode.jsonc
```

## Prerequisites

- The Kronk server must be running on `http://127.0.0.1:11435`
- The Kronk MCP service must be running on `http://localhost:9000/mcp`

This will happen with the default settings when you start the kronk model server.

## Notes

- The `agent.md` file instructs the model to use Kronk's `kronk_fuzzy_edit` MCP tool instead of OpenCode's built-in `edit` tool. This provides more reliable file edits through tiered fuzzy matching.
- OpenCode prefixes MCP tool names with the server name in lowercase (e.g., `kronk` server → `kronk_fuzzy_edit`).
- The `auth.json` file uses a placeholder key since Kronk runs locally. Update this if you enable authentication on the Kronk server.
