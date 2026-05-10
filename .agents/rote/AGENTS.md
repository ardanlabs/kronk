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

This project routes **every** external tool call through `rote`. The
canonical workflow is:

1. `( cd ~/.rote/rote/workspaces/playground && rote kronk_probe "<intent>" )`
   — discover the right tool by intent.
2. `( cd ~/.rote/rote/workspaces/playground && rote kronk_call <tool> '{json args}' -s )`
   — execute. Tool names are **bare** in the catalog: use `web_search`
   or `fuzzy_edit`, **not** `kronk_web_search` or `kronk_fuzzy_edit`.
3. `( cd ~/.rote/rote/workspaces/playground && rote @N '.content[0].text | fromjson | <jq>' )`
   — extract / filter results. MCP responses are double-wrapped, so
   always pipe through `fromjson`. Always check `.is_error` first.

Do not call `rote init`. The `playground` workspace is created and
maintained by `make agents-rote-playground`. If it is missing, ask the user
to run `make agents-rote-seed`.

### Current execution status (rote v0.13.0)

`rote kronk_call` is **broken** in the installed version of rote — see
[NOTES.md §9.1](rote/NOTES.md). It returns `is_error: true` because the
MCP handshake is skipped. Until the upstream fix lands:

- **File edits** → temporarily fall back to the host's built-in `edit`
  tool (normally disabled by the rote-only architecture).
- **Web research** → temporarily fall back to the host's built-in
  web-search tool (if any).

This is a temporary exception driven by the v0.13.0 bug. When the
fix lands, all calls go back through rote without changes here.

### Adding a new external service

Add a rote adapter (`rote adapter new-from-mcp <id> <url>`), mirror it
into `.agents/rote/adapters/`, and call through rote. **Do not wire
MCP servers directly into agent hosts** — that path was deliberately
removed.

### Reference

Full guidance, including parameter schemas, the canvas → crystallize
workflow, the workspace lifecycle, the v0.13.0 known issue, and the
rote registry invite-code requirement, is in the
[`rote`](skills/rote/SKILL.md) skill and in
[`.agents/rote/NOTES.md`](rote/NOTES.md).

If `rote` is not installed, or the registry session is missing, **stop
and ask the user** for an invite code from the project owner (Bill).
Do not attempt to call Kronk directly via HTTP / MCP / curl — those
paths have been removed by design.
