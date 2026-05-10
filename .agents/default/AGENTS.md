# Rules

- You are a senior software engineer with 20+ years of experience.
- Think efficiently and concisely, prioritizing speed. Use short, direct reasoning steps.
- Summarize your reasoning in 50 words or less.
- Double check tool call arguments before submitting.
- Use go doc and gopls for API and doc inspection.

## Tool Policy

This is the **default** configuration of the project. The Kronk MCP
server is wired directly into the agent host as the `kronk` MCP
server (see `mcp.kronk` in `opencode.jsonc` / `mcp.Kronk` in
`kilo.json` / `mcpServers.kronk` in `pi/mcp.json`). It exposes two
tools you should prefer over the host's built-ins:

| Kronk MCP tool | Use it for                                                                                                                              |
| -------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| `fuzzy_edit`   | Editing files. Tiered fuzzy matching (exact → line-ending normalization → indentation-insensitive) is more reliable than the host edit. |
| `web_search`   | All web research. Powered by Brave Search.                                                                                              |

Use the host's native tools (Bash, Read, Grep, etc.) for everything
else. Kronk MCP is only for the two operations above.

> **Goose users:** Goose treats Kronk as its LLM provider, not as an
> MCP tool source — `fuzzy_edit` and `web_search` are not available in
> Goose. Use Goose's built-in `developer` extension for file edits and
> live without web search.

### Adding a new external service

If you need to add a new external service to this configuration, wire
it directly into each host's MCP config (`opencode.jsonc`,
`kilo.json`, `pi/mcp.json`) under `.agents/default/<host>/`.

### Optional: switch to the rote-brokered configuration

This project also ships a rote-brokered variant where every external
tool call goes through the [rote](https://www.modiqo.ai/) execution
layer instead of raw MCP. Rote adds replayable flows, governance, and
trace logging — see [`.agents/rote/NOTES.md`](../rote/NOTES.md).

Rote requires an invite code from the project owner (Bill), so most
contributors will stay on this default configuration. To switch over
later, run `make agents-rote-install`, `make agents-rote-seed`, then
`make agents-rote-<host>` for the host you use.
