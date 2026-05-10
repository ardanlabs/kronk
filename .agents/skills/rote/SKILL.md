---
name: rote
description: Use rote for ALL external tool/API/MCP access in this project. Load whenever you need to search the web, edit a file, or otherwise touch a Kronk MCP service. There is no other path — the raw MCP wiring has been removed from every agent host config.
---

## Why this skill exists

This project has standardized on **rote** as the single execution layer for
every external tool / API / MCP call. The Kronk MCP server is no longer
exposed directly to any agent host (OpenCode, Kilo, Pi, Goose, Amp). The
agent's only path to Kronk's tools — `kronk_web_search`, `kronk_fuzzy_edit`,
or any future tool — is through `rote` invoked from the Bash tool.

Rationale (full context in [`.agents/rote/NOTES.md`](../../rote/NOTES.md)):

1. **One configuration surface.** Adding a new external service is one
   `rote adapter new-from-mcp` command, not a per-host MCP wiring change
   in four config files plus a hand-written skill.
2. **Token economics.** Successful explorations crystallize into flows
   that re-run for ~200 tokens vs ~12,000 for re-discovery.
3. **Determinism + collective memory.** Flows are versioned, shareable,
   and survive across sessions and teammates.

## Hard rules

- **NEVER** call the host's built-in `edit` tool. File edits go through
  `rote` (which routes to Kronk's `kronk_fuzzy_edit`).
- **NEVER** call the host's built-in web-search tool (if it has one). Web
  research goes through `rote` (which routes to Kronk's `kronk_web_search`).
- **NEVER** attempt to talk to `http://localhost:9000/mcp` directly. The
  host MCP wiring is intentionally removed.
- **ALWAYS** be inside a rote workspace before calling probe / call /
  query / template commands. `@N` references and template variables only
  exist inside a workspace.
- **NEVER** use Python, Node, or shell scripts to filter / transform
  response data when `rote @N '<jq>'` can do it.

## Bootstrap (per session, once)

```bash
rote --version              # confirm rote is installed (see NOTES.md §0)
rote adapter list           # confirm `kronk` is registered
```

If `kronk` is missing, run from the repo root:

```bash
make copy-agent-rote        # seeds ~/.rote/adapters/kronk/ from the repo
```

If `rote` itself is missing or asks you to log in and the user has no
registry session, **stop and ask the user for a rote invite code** —
the project owner (Bill) issues these. Do not improvise a fallback.
See [`.agents/rote/NOTES.md`](../../rote/NOTES.md) §0 for the full
install + invite story.

## Per-task workflow

```diagram
╭─────────────╮     ╭──────────╮     ╭──────────╮     ╭──────────╮     ╭──────────╮
│  rote init  │────▶│  probe   │────▶│   call   │────▶│  query   │────▶│  export  │
│ (workspace) │     │ (find    │     │ (execute)│     │ (jq /    │     │ (flow,   │
│             │     │  tool)   │     │          │     │  @N)     │     │  reuse)  │
╰─────────────╯     ╰──────────╯     ╰──────────╯     ╰──────────╯     ╰──────────╯
```

### 1. Open a workspace

```bash
rote init <task-name> --seq
eval "$(rote cd <task-name>)"     # changes shell into the workspace dir
```

Without a workspace, `@N` references and template variables don't work.

### 2. Discover the right tool with `probe`

Never guess tool names. Ask rote to find the right one by intent:

```bash
rote kronk_probe "search the web for X"
rote kronk_probe "edit a file"
```

The probe returns matching tools ranked by relevance, with their full
parameter schemas. Pick the one that fits and use its **exact** name in
the call.

### 3. Execute with `call`

```bash
rote kronk_call kronk_web_search '{"query":"...", "count":5}' -s
rote kronk_call kronk_fuzzy_edit '{
  "file_path":"/abs/path/to/file.go",
  "old_string":"...",
  "new_string":"..."
}' -s
```

The response is cached as `@N.json` on disk. Costs zero agent tokens to
re-read.

### 4. Query cached responses with jq

```bash
rote @1 '.results | length'                       # count
rote @1 '.results[0].url'                         # field access
rote @1 '.results[] | select(.title | test("..."))'  # filter
rote @1 '.id' -s some_id                          # save to template var
```

### 5. Chain calls with template variables

```bash
rote kronk_call <next_tool> '{"id":"$some_id"}' -t -s
```

### 6. Crystallize a successful exploration into a flow

When the trace produces something useful and reusable, export it. (Full
canvas / crystallize workflow is the next thing we plan to learn — see
NOTES.md §8 step 3.)

```bash
rote export flow.sh                # quick shell script
# OR for a parameterized, shareable, lint-checked flow:
rote flow template create --name <slug> --adapter adapter/kronk \
  --description "What this flow does" \
  --param "name:type:required:default:description" \
  --tag kronk
```

After releasing a flow, future calls become:

```bash
rote flow search "<intent>"        # find an existing flow first
```

## When the right adapter doesn't exist

If `rote adapter list` does not include the service you need, **stop and
tell the user** that an adapter must be created. Do not silently fall
back to direct HTTP, direct MCP, or hand-rolled scripts. The user will
either:

1. Create the adapter (`rote adapter new-from-mcp <id> <url>`), then
   you continue using rote, or
2. Decide an adapter isn't worth it for this one-off and explicitly
   green-light an out-of-band approach.

## Reference

- [`.agents/rote/NOTES.md`](../../rote/NOTES.md) — full project documentation
  for rote: install, invite, mirror conventions, step log, open questions.
- `~/.rote/adapters/kronk/agent.md` — auto-generated, in-depth subagent
  instructions for the kronk adapter (workspace lifecycle, write-guard,
  flow lint, release gates). Read this when working on flows, not just
  ad-hoc calls.
- `rote --help`, `rote why`, `rote how`, `rote start` — built-in CLI
  guidance.
