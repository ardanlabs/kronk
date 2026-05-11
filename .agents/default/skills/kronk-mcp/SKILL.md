---
name: kronk-mcp
description: Using the Kronk MCP service in the default agent configuration, where Kronk is wired directly into the host (OpenCode, Kilo, Pi) as an MCP server. Exposes `fuzzy_edit` (fallback for the host's exact-match edit when whitespace / line-endings cause it to miss) and `web_search` (Brave-powered). Load whenever you need web research, or when a host edit fails on exact matching.
---

# Kronk MCP (default config)

Kronk MCP is registered directly with the host (server name `kronk`
or `Kronk`). No rote layer.

## Tool selection

| Operation                | Use                                                                                                                                  |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------ |
| Web research             | `web_search` (no host alternative)                                                                                                   |
| Modify an existing file  | Host's native edit/replace **first**; only on exact-match failure (whitespace, line-endings, indentation drift) call `fuzzy_edit`    |
| Create / read / shell    | Host's native tools — not `fuzzy_edit`                                                                                               |

`fuzzy_edit` is a fallback. Don't reach for it pre-emptively.

## Hard rules

- **Never** call `http://localhost:9000/mcp` directly via curl/wget.
- **Try the host's native edit tool first.** Only call `fuzzy_edit`
  after exact-match fails.
- If a tool returns `is_error: true`, **stop and report**. Do not
  retry blindly or improvise an alternative transport.

## Tool names per host

- OpenCode, Kilo: prefixed → `kronk_fuzzy_edit`, `kronk_web_search`.
- Pi (`directTools: true`): bare → `fuzzy_edit`, `web_search`.

If unsure, list the host's available tools and match by suffix.

## `fuzzy_edit` schema

| Field         | Meaning                                |
| ------------- | -------------------------------------- |
| `file_path`   | Absolute path to existing file.        |
| `old_string`  | Exact text to replace (unique).        |
| `new_string`  | Replacement text.                      |

Workflow: `Read` the file → pick a uniquely identifiable
`old_string` (add 1–2 lines of context if ambiguous) → call
`fuzzy_edit` → re-`Read` to verify.

## `web_search` schema

| Field   | Meaning                                |
| ------- | -------------------------------------- |
| `query` | Search query string.                   |
| `count` | Optional result count.                 |

Returns formatted text (title / URL / description). For full page
contents, follow up with the host's web-page reader against a
specific URL.
