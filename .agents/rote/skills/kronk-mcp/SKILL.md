---
name: kronk-mcp
description: The ONLY sanctioned path to the Kronk MCP service (web_search, fuzzy_edit, and any future tool). Internally uses rote as the execution layer — every command below is a `rote ...` invocation in Bash. Load whenever you need to search the web, edit a file, or otherwise touch Kronk MCP. The raw MCP wiring has been removed from every agent host config and direct curl/HTTP to localhost:9000 is forbidden.
---

## Why this skill exists

This project has standardized on **rote** as the single execution layer for
every external tool / API / MCP call. The Kronk MCP server is no longer
exposed directly to any agent host (OpenCode, Kilo, Pi, Goose, Amp). The
agent's only path to Kronk's tools — `web_search`, `fuzzy_edit`, or any
future tool — is through `rote` invoked from the Bash tool.

Rationale (full context in [`.agents/rote/NOTES.md`](../../rote/NOTES.md)):

1. **One configuration surface.** Adding a new external service is one
   `rote adapter new-from-mcp` command, not a per-host MCP wiring change
   in four config files plus a hand-written skill.
2. **Token economics.** Successful explorations crystallize into flows
   that re-run for ~200 tokens vs ~12,000 for re-discovery.
3. **Determinism + collective memory.** Flows are versioned, shareable,
   and survive across sessions and teammates.

## Hard rules

- **NEVER** attempt to talk to `http://localhost:9000/mcp` directly.
  The host MCP wiring is intentionally removed.
- **ALWAYS** be inside the existing `playground` rote workspace before
  calling probe / call / query. `@N` references and template variables
  only exist inside a workspace.
- **NEVER** call `rote init` from an agent. Workspaces are created by
  make targets, not by agents. (Empirical reason: small models
  substitute literal arguments, e.g. Qwen3.6-35B once renamed
  `playground` to `test workspace`.)
- **NEVER** use Python, Node, or shell scripts to filter / transform
  response data when `rote @N '<jq>'` can do it.

## Bootstrap (per session, once)

```bash
rote --version              # confirm rote is installed (see NOTES.md §3)
rote adapter list           # confirm `kronk` is registered
ls ~/.rote/rote/workspaces/playground   # confirm the playground workspace exists
```

If `kronk` is missing from `rote adapter list`, ask the user to run
`make agents-rote-seed` from the repo root. That target seeds the
adapter from `.agents/rote/adapters/kronk/`, rebuilds the search
index, and ensures the `playground` workspace exists.

If `rote` itself is missing, ask the user to run `make agents-rote-install`
(see [NOTES.md §3](../../rote/NOTES.md) for the install + invite story
— Modiqo's registry is invite-only and the project owner Bill issues
the invite codes). Do not improvise a fallback.

If the playground workspace is missing, ask the user to run
`make agents-rote-playground`. Do **not** call `rote init` yourself.

## Per-task workflow

```diagram
╭───────────╮     ╭──────────╮     ╭──────────╮     ╭──────────╮     ╭──────────╮
│ playground│────▶│  probe   │────▶│   call   │────▶│  query   │────▶│  export  │
│ (already  │     │ (find    │     │ (execute │     │ (jq on   │     │ (flow,   │
│  exists)  │     │  tool)   │     │  via MCP)│     │ .content)│     │  reuse)  │
╰───────────╯     ╰──────────╯     ╰──────────╯     ╰──────────╯     ╰──────────╯
```

### 1. Use the existing `playground` workspace

The workspace at `~/.rote/rote/workspaces/playground/` is created and
maintained by `make agents-rote-playground` (or `make agents-rote-seed`,
which depends on it). Do not call `rote init` yourself.

Every rote command below uses a `( cd ... && rote ... )` subshell
because rote requires the shell's cwd to be the workspace dir, and
the documented `rote cd <name>` helper is **broken** on this machine
(errors with `command failed`). Each agent Bash invocation is a
fresh shell anyway, so the subshell pattern is the natural fit.

### 2. Discover the right tool with `probe`

Never guess tool names. Ask rote to find the right one by intent:

```bash
( cd ~/.rote/rote/workspaces/playground && \
    rote kronk_probe "search the web for X" )

( cd ~/.rote/rote/workspaces/playground && \
    rote kronk_probe "edit a file" )
```

The probe returns matching tools ranked by relevance, with their full
parameter schemas. Pick the one that fits and use its **exact name**
in the call. Names in the catalog are bare: `web_search`, `fuzzy_edit`
— **not** `kronk_web_search` or `kronk_fuzzy_edit`. The `kronk_`
prefix only applies to the rote shorthand verbs (`kronk_probe`,
`kronk_call`).

### 3. Execute with `call`

**MANDATORY**: every `kronk_call` MUST be preceded by `rm -f` of the
cached MCP session file. This forces rote to re-handshake with kronk
on every call instead of reusing a cached `Mcp-Session-Id` that the
server may have evicted (process restart, idle timeout, etc.). Without
this, the second-and-later call after any kronk-server lifecycle event
fails with `404 Not Found: session not found` and rote will keep
retrying the dead session forever — see *Behaviors and gotchas* in
[`.agents/rote/NOTES.md`](../../rote/NOTES.md). Cost is one extra
~600 ms handshake per call; correctness wins.

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

The response is cached as `@N.json` on disk. Costs zero agent tokens
to re-read.

**Do not omit the `rm -f`.** If you see a `404 session not found`
error from a `kronk_call`, that means the rm step was skipped or
something else re-cached the session ID — re-run the same call with
the rm prefix and it will succeed.

### 4. Query cached responses with jq

MCP responses are wrapped as
`{ content: [{ type: "text", text: "<string>" }], is_error?: bool }`.

- `is_error` is **omitted on success** and set to `true` on failure.
  Use `(.is_error // false)` so missing-means-success works correctly.
- For Kronk's tools the inner `text` is **plain text** — `web_search`
  returns a formatted "Result 1: Title: ... URL: ..." block,
  `fuzzy_edit` returns a confirmation string. Do **not** pipe through
  `fromjson` for tool calls; it isn't JSON.

```bash
# 1. Check whether the call failed (absent → success)
( cd ~/.rote/rote/workspaces/playground && rote @1 '(.is_error // false)' )

# 2. Read the result text
( cd ~/.rote/rote/workspaces/playground && rote @1 '.content[0].text' )

# 3. Save to a template variable for chaining
( cd ~/.rote/rote/workspaces/playground && \
    rote @1 '.content[0].text' -s search_results )
```

### 5. Chain calls with template variables

```bash
( rm -f ~/.rote/adapters/kronk/runtime/sessions/workspace_playground.json && \
  cd ~/.rote/rote/workspaces/playground && \
    rote kronk_call <next_tool> '{"id":"$some_id"}' -t -s )
```

### 6. Crystallize a successful exploration into a flow

When the trace produces something useful and reusable, export it.

```bash
( cd ~/.rote/rote/workspaces/playground && rote export flow.sh )

# OR for a parameterized, shareable, lint-checked flow:
( cd ~/.rote/rote/workspaces/playground && \
    rote flow template create --name <slug> --adapter adapter/kronk \
      --description "What this flow does" \
      --param "name:type:required:default:description" \
      --tag kronk )
```

After releasing a flow, future calls become:

```bash
rote flow search "<intent>"        # find an existing flow first
```

## When the right adapter doesn't exist

If `rote adapter list` does not include the service you need, **stop
and tell the user** that an adapter must be created. Do not silently
fall back to direct HTTP, direct MCP, or hand-rolled scripts. The user
will either:

1. Create the adapter (`rote adapter new-from-mcp <id> <url>`), then
   you continue using rote, or
2. Decide an adapter isn't worth it for this one-off and explicitly
   green-light an out-of-band approach.

## Reference

- [`.agents/rote/NOTES.md`](../../rote/NOTES.md) — full project
  documentation: install (§3), makefile commands (§4), adapters (§5),
  mirror conventions (§6), per-task workflow (§7), behaviors and
  gotchas (§8).
- `~/.rote/adapters/kronk/agent.md` — auto-generated, in-depth
  subagent instructions for the kronk adapter (workspace lifecycle,
  write-guard, flow lint, release gates). Read this when working on
  flows, not just ad-hoc calls.
- `rote --help`, `rote why`, `rote how`, `rote start` — built-in CLI
  guidance.
