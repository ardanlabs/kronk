---
name: kronk-mcp
description: The ONLY sanctioned path to the Kronk MCP service (`web_search`, `fuzzy_edit`) in the rote-routed config. Every command is a `rote ...` invocation in Bash. Load whenever you need web research, or when a host edit fails on exact-string matching and you need the rote-routed `fuzzy_edit` fallback.
---

# Kronk MCP (rote config)

Kronk MCP is reachable only through `rote`. Direct HTTP/MCP/curl is
removed by design.

## Tool selection

| Operation                | Use                                                                                                                                  |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------ |
| Web research             | `web_search` via this skill (no host alternative)                                                                                    |
| Modify an existing file  | Host's native edit/replace **first**; only on exact-match failure call `fuzzy_edit` via this skill                                   |
| Create / read / shell / `go doc` / `gopls` | Host's native tools — not this skill                                                                                                 |

`fuzzy_edit` is a fallback, not a primary. `web_search` is primary.

## Hard rules

- **Never** call `http://localhost:9000/mcp` directly. The host MCP
  wiring is intentionally removed.
- **Try the host's native edit first.** Only call `fuzzy_edit` after
  exact-match fails.
- **Never** call `rote init`. The `playground` workspace is created
  by `make agents-rote-playground`. If missing, ask the user to run
  `make agents-rote-seed`.
- **Always** be inside the `playground` workspace when invoking
  `rote` (use the `( cd ... && rote ... )` pattern below).
- If a `kronk-mcp` command fails, **stop and report** to the user.
  Do not improvise an alternative transport.

## Workflow: probe → call → query

### Discover the right tool

```bash
( cd ~/.rote/rote/workspaces/playground && \
    rote kronk_probe "<intent, e.g. 'edit a file' or 'search the web'>" )
```

Tool names in the catalog are **bare**: `web_search`, `fuzzy_edit`
(not `kronk_*`). The `kronk_` prefix only applies to rote shorthand
verbs (`kronk_probe`, `kronk_call`).

### Execute

**`rm -f` of the session cache is mandatory before every `kronk_call`** —
without it you get `404 session not found` after the kronk-server
recycles.

```bash
( rm -f ~/.rote/adapters/kronk/runtime/sessions/workspace_playground.json && \
  cd ~/.rote/rote/workspaces/playground && \
    rote kronk_call web_search '{"query":"...", "count":5}' -s )

( rm -f ~/.rote/adapters/kronk/runtime/sessions/workspace_playground.json && \
  cd ~/.rote/rote/workspaces/playground && \
    rote kronk_call fuzzy_edit '{
      "file_path":"/abs/path/to/file.go",
      "old_string":"...",
      "new_string":"..."
    }' -s )
```

Response is cached as `@N.json`.

### Query the response

MCP responses wrap output as
`{ content: [{ type: "text", text: "<string>" }], is_error?: bool }`.
For Kronk's tools the inner `text` is **plain text** — do not
`fromjson` it. `is_error` is omitted on success.

```bash
( cd ~/.rote/rote/workspaces/playground && rote @1 '(.is_error // false)' )
( cd ~/.rote/rote/workspaces/playground && rote @1 '.content[0].text' )
```

## When the right adapter doesn't exist

If `rote adapter list` doesn't include the service you need, **stop
and tell the user**. Do not silently fall back to direct HTTP, MCP,
or hand-rolled scripts.
