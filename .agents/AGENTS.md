# Rules

- You are a senior software engineer with 20+ years of experience.
- Think efficiently and concisely, prioritizing speed. Use short, direct reasoning steps.
- Summarize your reasoning in 50 words or less.
- Double check tool call arguments before submitting.
- Use go doc and gopls for API and doc inspection.

# Skills

Load the `rote` skill **before** any external tool / API / MCP call. There
is no other path — the raw Kronk MCP wiring has been removed from every
agent host config (OpenCode, Kilo, Pi, Goose). All web research and all
file edits flow through `rote` invoked from the Bash tool.

```
skill({ name: "rote" })
```

## Tool Policy

This project routes **every** external tool call through `rote`:

- **File edits** → `rote kronk_call kronk_fuzzy_edit '{...}'`
  (NEVER use the host's built-in `edit` tool — it is disabled.)
- **Web research** → `rote kronk_call kronk_web_search '{...}'`
  (NEVER use the host's built-in web-search tool, if any.)
- **Future external services** → add a rote adapter
  (`rote adapter new-from-mcp <id> <url>`), then call through rote.
  Do not wire MCP servers directly into agent hosts.

Mandatory workflow before any rote call:

1. `rote init <task> --seq` — open a workspace.
2. `rote kronk_probe "<intent>"` — discover the right tool by intent.
3. `rote kronk_call <tool_name> '{json args}' -s` — execute.
4. `rote @N '<jq query>'` — extract / filter results without spending agent
   tokens.

Full guidance, including parameter schemas, the canvas → crystallize
workflow, the workspace lifecycle, and the rote registry invite-code
requirement, is in the [`rote`](skills/rote/SKILL.md) skill and in
[`.agents/rote/NOTES.md`](rote/NOTES.md).

If `rote` is not installed, or the registry session is missing, **stop and
ask the user** for an invite code from the project owner (Bill). Do not
attempt to call Kronk directly via HTTP / MCP / curl — those paths have
been removed by design.
