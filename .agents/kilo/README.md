# Kilo Code Configuration

Configuration files for the [Kilo Code](https://kilocode.ai) VS Code extension to work with Kronk.

## Files

| File        | Purpose                                             |
| ----------- | --------------------------------------------------- |
| `kilo.json` | Provider, model, MCP, and permission settings       |
| `agent.md`  | Custom instructions injected into the system prompt |

## Installation

Copy these files to your Kilo config directory:

```bash
cp agent.md  ~/.config/kilo/agent.md
cp kilo.json ~/.config/kilo/kilo.json
```

## Prerequisites

- The Kronk server must be running on `http://localhost:11435`
- The Kronk MCP service must be running on `http://localhost:9000/mcp`

This will happen with the default settings when you start the kronk model server.

## Notes

- The `agent.md` file instructs the model to use Kronk's `kronk_fuzzy_edit` MCP tool instead of Kilo's built-in edit tools. This provides more reliable file edits through tiered fuzzy matching.
- Kilo prefixes MCP tool names with the server name as defined in the config key (e.g., `Kronk` server → `Kronk_fuzzy_edit`). If you see tool name mismatches, check the MCP server key in `kilo.json`.
- The `permission` field is set to `"allow"` to auto-approve tool calls. Change to `"ask"` if you prefer manual approval.
