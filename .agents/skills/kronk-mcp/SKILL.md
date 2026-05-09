---
name: kronk-mcp
description: Use the Kronk MCP services for web_search (Brave search) and fuzzy_edit (tiered file editing). Load when you need to search the web or edit files.
---

## Kronk MCP Tools

You have access to two MCP tools via the `kronk` MCP server: `kronk_web_search` and `kronk_fuzzy_edit`. These are available as Bash tool calls that invoke the MCP protocol.

### kronk_web_search

Performs a web search using Brave Search API. Returns titles, URLs, and descriptions.

**When to use:**
- Researching topics, APIs, libraries, or best practices
- Looking up documentation or recent changes
- Gathering information before writing code
- Verifying facts or configurations
- Any time you need external knowledge beyond your training data

**Parameters:**
- `query` (string, required): Your search query
- `count` (int, optional, default 10, max 20): Number of results
- `country` (string, optional): Country code (e.g. US, GB, DE)
- `freshness` (string, optional): `pd` (past day), `pw` (past week), `pm` (past month), `py` (past year)
- `safesearch` (string, optional): `off`, `moderate`, `strict` (default moderate)

**Example:**
```bash
# Search for the latest Go 1.24 features
kronk_web_search(query="Go 1.24 new features", count=5, freshness="pm")
```

### kronk_fuzzy_edit

Edits files using tiered fuzzy matching (exact -> line-ending normalization -> indentation-insensitive). This is your **primary file editing tool** for this project.

**When to use:**
- Any file modification in this project
- Replacing specific code strings with new content
- The matching is tolerant of whitespace differences, line ending differences, and indentation variations

**Parameters:**
- `file_path` (string, required): Absolute path to the file
- `old_string` (string, required): The text to find (use content from Read tool output, stripped of line number prefixes)
- `new_string` (string, required): The replacement text

**Matching tiers (applied in order):**
1. **Exact byte match** - requires exactly one occurrence
2. **Line-ending normalization** - handles CRLF vs LF differences
3. **Indentation-insensitive** - ignores leading whitespace per line

**Important rules:**
- Always use `read` tool first to get the exact file content before editing
- The `old_string` must match content from the file (after stripping line number prefixes from Read output)
- Preserve the file's existing code style and conventions
- If a match fails, re-read the file and try with more context
- Never use the built-in `edit` tool - it is disabled

**Example:**
```bash
# Replace a function signature
kronk_fuzzy_edit(
  file_path="/Users/bill/code/go/src/github.com/ardanlabs/kronk/main.go",
  old_string="func main() {",
  new_string="func main(ctx context.Context) error {"
)
```

## Workflow

1. **For research:** Call `kronk_web_search` with a targeted query. Use results to inform your implementation.
2. **For file edits:** Always `read` the file first, then call `kronk_fuzzy_edit` with the exact content you want to replace.
3. After editing Go files, run `go vet` and `gofmt -s -w` on the changed files, then `staticcheck` and `go fix` on the package.