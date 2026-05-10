# Rules

- You are a senior software engineer with 20+ years of experience.
- Think efficiently and concisely, prioritizing speed. Use short, direct reasoning steps.
- Summarize your reasoning in 50 words or less.
- Double check tool call arguments before submitting.
- Use go doc and gopls for API and doc inspection.

# Skills

**Before any external tool / API / MCP call, you MUST call:**

```
skill({ name: "kronk-mcp" })
```

The skill is named **exactly** `kronk-mcp`. Do not look for any other
skill name. Do not load `rote`, `kronk`, `web-search`, or any
similarly-named skill — they are either non-existent or stale and will
mislead you. The `kronk-mcp` skill teaches the only legal path to the
Kronk MCP service; internally it uses `rote` as the execution layer,
but the skill name to load is `kronk-mcp`.

The raw Kronk MCP wiring has been removed from every agent host config
(OpenCode, Kilo, Pi, Goose). All web research and all file edits flow
through commands documented inside the `kronk-mcp` skill, invoked from
the Bash tool.

You may **NOT** issue `curl`, `wget`, `fetch`, or any other HTTP request
to `http://localhost:9000`, to any `/mcp` endpoint, or to any MCP server
directly. The `kronk-mcp` skill provides the only sanctioned path. If
the commands inside it fail, **stop and report the failure to the
user** — do not improvise an alternative transport, do not hand-roll an
MCP handshake, do not fall back to a different tool.

## Tool Policy

This project routes **every** external tool call through `rote`. The
canonical workflow is:

1. `( cd ~/.rote/rote/workspaces/playground && rote kronk_probe "<intent>" )`
   — discover the right tool by intent.
2. `( rm -f ~/.rote/adapters/kronk/runtime/sessions/workspace_playground.json && cd ~/.rote/rote/workspaces/playground && rote kronk_call <tool> '{json args}' -s )`
   — execute. The `rm -f` is **mandatory** — it forces rote to
   re-handshake with kronk on every call instead of reusing a cached
   `Mcp-Session-Id` that the kronk-server may have evicted (process
   restart, idle timeout). Without it you get `404 session not found`.
   Tool names are **bare** in the catalog: use `web_search` or
   `fuzzy_edit`, **not** `kronk_web_search` or `kronk_fuzzy_edit`.
3. `( cd ~/.rote/rote/workspaces/playground && rote @N '.content[0].text' )`
   — extract the result. MCP responses wrap the tool output as
   `{ content: [{ type: "text", text: "<string>" }] }`. For Kronk's
   tools the inner `text` is **plain text** (e.g. formatted search
   results, an edit confirmation), not JSON — do **not** pipe through
   `fromjson` for tool calls. On failure the response carries
   `is_error: true`; on success the field is **omitted entirely**, so
   treat absent as success or use `.is_error // false`.

Do not call `rote init`. The `playground` workspace is created and
maintained by `make agents-rote-playground`. If it is missing, ask the user
to run `make agents-rote-seed`.

### Adding a new external service

Add a rote adapter (`rote adapter new-from-mcp <id> <url>`), mirror it
into `.agents/rote/adapters/`, and call through rote. **Do not wire
MCP servers directly into agent hosts** — that path was deliberately
removed.

### Reference

Full guidance, including parameter schemas, the workspace → crystallize
workflow, the workspace lifecycle, and the rote registry invite-code
requirement, is in the [`kronk-mcp`](skills/kronk-mcp/SKILL.md) skill
and in [`.agents/rote/NOTES.md`](rote/NOTES.md).

If `rote` is not installed, or the registry session is missing, **stop
and ask the user** for an invite code from the project owner (Bill).
Do not attempt to call Kronk directly via HTTP / MCP / curl — those
paths have been removed by design.
