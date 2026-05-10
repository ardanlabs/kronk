# rote — Project Documentation

This document is the developer reference for how Kronk uses
[rote](https://www.modiqo.ai/) as the single execution layer between
coding agents (OpenCode, Amp, Kilo, Pi, Goose) and external
tools/APIs/MCP services.

## Architecture at a glance

How an agent host (OpenCode is the worked example below; Kilo, Pi,
Goose follow the same pattern) reaches Kronk's MCP tools through rote.

### File map: source of truth → on-machine deployment

```diagram
╭───────────────────── REPO (source of truth) ─────────────────────╮
│ .agents/rote/                                                    │
│ ├── AGENTS.md                  ─┐                                │
│ ├── opencode/                   │                                │
│ │   ├── opencode.jsonc          │  shipped by                    │
│ │   └── auth.json               │  `make agents-rote-opencode`   │
│ └── skills/kronk-mcp/SKILL.md  ─┘                                │
│ .agents/rote/adapters/kronk/                                     │
│ ├── manifest.json              ─┐                                │
│ ├── tools.json                  │  shipped by                    │
│ ├── agent.md                    │  `make agents-rote-seed`       │
│ ├── config/policies.json        │                                │
│ └── toolsets/mcp:.json         ─┘                                │
╰──────────────────────────────────────────────────────────────────╯
                                  │
                                  ▼
╭──────────────── ON-MACHINE (deployed copies) ────────────────────╮
│ ~/.config/opencode/                                              │
│ ├── AGENTS.md                  loaded by OpenCode at startup     │
│ ├── opencode.jsonc             NO mcp block — direct path gone   │
│ ├── auth.json                                                    │
│ └── skills/kronk-mcp/SKILL.md  discovered by skill name          │
│                                                                  │
│ ~/.rote/                                                         │
│ ├── secrets/, registry/        login state, persisted on disk    │
│ ├── adapters/kronk/                                              │
│ │   ├── manifest.json, tools.json, agent.md, config/, toolsets/  │
│ │   ├── index/                 Tantivy search index, local       │
│ │   └── runtime/sessions/                                        │
│ │       └── workspace_playground.json   ← the buggy cache file   │
│ └── rote/workspaces/playground/                                  │
│     └── .rote/responses/@N.json   cached tool responses          │
╰──────────────────────────────────────────────────────────────────╯
```

### Call flow: a single web search, end to end

```diagram
╭─────────────╮
│ User prompt │  "Search the web for X"
╰──────┬──────╯
       ▼
╭─────────────────────────────────────────────────────────────╮
│ OpenCode model                                              │
│   reads: ~/.config/opencode/AGENTS.md                       │
│   sees:  "MUST call skill({ name: 'kronk-mcp' })"           │
│         + "NEVER curl/fetch localhost:9000 directly"        │
│         + "On failure, STOP and report — don't improvise"   │
╰──────┬──────────────────────────────────────────────────────╯
       ▼
╭─────────────────────────────────────────────────────────────╮
│ skill({ name: "kronk-mcp" })                                │
│   loads: ~/.config/opencode/skills/kronk-mcp/SKILL.md       │
│   teaches the agent the rote workflow                       │
╰──────┬──────────────────────────────────────────────────────╯
       ▼
╭─────────────────────────────────────────────────────────────╮
│ Bash tool: rote kronk_probe "search the web"                │
│   spawns: fresh `rote` process                              │
│   reads:  ~/.rote/adapters/kronk/{tools.json, index/}       │
│   does:   Tantivy semantic search (LOCAL, no server)        │
│   returns: web_search ranked first                          │
│   writes: ~/.rote/rote/workspaces/playground/...@N.json     │
╰──────┬──────────────────────────────────────────────────────╯
       ▼
╭─────────────────────────────────────────────────────────────╮
│ Bash tool: rm -f ~/.rote/.../workspace_playground.json &&   │
│            rote kronk_call web_search '{...}' -s            │
│   ① rm wipes cached Mcp-Session-Id (bug workaround, §11)    │
│   ② fresh `rote` process opens HTTP to localhost:9000/mcp   │
│   ③ POST initialize           → server returns Session-Id   │
│   ④ POST notifications/initialized                          │
│   ⑤ POST tools/call (web_search)                            │
│   ⑥ kronk MCP → Brave Search API → response                 │
│   ⑦ rote writes response to @N.json + caches new Session-Id │
╰──────┬──────────────────────────────────────────────────────╯
       ▼
╭─────────────────────────────────────────────────────────────╮
│ Bash tool: rote @N '.content[0].text'                       │
│   reads cached response, extracts plain-text result         │
╰──────┬──────────────────────────────────────────────────────╯
       ▼
╭───────────────╮
│ Model summary │ ← agent presents real Brave results to user
╰───────────────╯
```

### What each piece enforces

| Component                                                | Enforces                                                                                                                                              |
| -------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| `opencode.jsonc` (no `mcp` block)                        | OpenCode's built-in MCP client can't reach kronk — no direct path exists at the host-config level.                                                    |
| `AGENTS.md` (skill name + no-curl + no-hallucination)    | Model is instructed to load `kronk-mcp` exactly, forbidden from curl/wget/fetch to any MCP endpoint, forbidden from substituting training data on failure. |
| `SKILL.md` `kronk-mcp` (renamed from `rote`)             | Wins skill discovery against the model's first instinct ("I need a kronk MCP skill").                                                                 |
| `SKILL.md` MANDATORY `rm -f` prefix                      | Works around rote 0.14.1's cached-session bug — every `kronk_call` re-handshakes from scratch. See §11.                                               |
| `~/.rote/adapters/kronk/` (manifest + tools.json + index) | Rote knows what's callable on kronk MCP without ever talking to it for discovery.                                                                     |
| `~/.rote/rote/workspaces/playground/`                    | Holds `@N.json` responses for jq queries — the one long-lived workspace.                                                                              |
| Kronk MCP server (`cmd/server/app/domain/mcpapp/`)       | Standard `modelcontextprotocol/go-sdk` Streamable HTTP server on port 9000, rate-limited per `policies.json`.                                         |

### What's intentionally **not** wired

- OpenCode does NOT speak MCP directly to kronk. The `mcp` block is
  gone from `opencode.jsonc`.
- Brave Search API key sits in kronk-server's env (`BRAVE_API_KEY`),
  never in OpenCode, never in rote, never in agent context.
- No flow crystallization is wired up — the workspace → flow → release
  path exists in rote but we don't use it. We treat rote as a
  skill-discoverable bash wrapper around kronk MCP, with the
  workspace as a write-only response cache.

---

## Canonical references

| Resource                              | URL                                                               |
| ------------------------------------- | ----------------------------------------------------------------- |
| Marketing site                        | <https://www.modiqo.ai/>                                          |
| LLM-readable full index (recommended) | <https://www.modiqo.ai/llms-full.txt>                             |
| VS Code extension                     | <https://marketplace.visualstudio.com/items?itemName=Modiqo.rote> |
| Discord                               | <https://discord.gg/NmHjhxF3G>                                    |
| GitHub                                | <https://github.com/modiqo/rote>                                  |

---

## What rote is

rote is an **execution layer** between coding agents and APIs / MCP
servers. It captures what an agent does the _first_ time it solves a
problem and crystallizes it into a deterministic, replayable flow.
Subsequent runs cost ~200 tokens instead of ~12,000.

The five primitives form one closed loop:

```diagram
                  ╭─────────────╮
                  │   ADAPT     │  Any API/MCP/OpenAPI/GraphQL/gRPC
                  │ (adapter)   │  becomes callable. No SDK glue.
                  ╰──────┬──────╯
                         │
                         ▼
                  ╭─────────────╮
              ╭──▶│    ASK      │  Agent explores in plain language
              │   │  (canvas)   │  on the canvas: probe → call →
              │   ╰──────┬──────╯  recover → converge
              │          │
              │          ▼
              │   ╭─────────────╮
              │   │ CRYSTALLIZE │  Successful trace compresses into
              │   │   (flow)    │  a named, deterministic flow.
              │   ╰──────┬──────╯
              │          │
              │          ▼
              │   ╭─────────────╮
              │   │   SHARE     │  Publish to team hub.
              │   │   (hub)     │  One discovery → team default.
              │   ╰──────┬──────╯
              │          │
              │          ▼
              │   ╭─────────────╮
              ╰───┤   RECALL    │  Anyone re-runs instantly.
                  ╰─────────────╯
```

Key on-disk concepts:

- **Workspace** — `~/.rote/rote/workspaces/<name>/`. A scratch area
  where each request is cached as `@1.json`, `@2.json`, etc. Required
  for `probe`, `call`, `query`, `template` commands. (Modiqo's
  marketing copy and conceptual diagrams call this a *canvas* — same
  thing, the on-disk directory is the canvas. This document uses
  *workspace* throughout for consistency with the CLI and filesystem
  layout.)
- **Adapter** — `~/.rote/adapters/<name>/`. Persistent typed catalog
  of an external service: manifest, tools list, search index,
  policies (rate limits, retries, timeouts), fingerprint.
- **Flow** — `~/.rote/flows/<slug>/main.ts`. A crystallized,
  parameterized, lint-checked, releasable script generated from a
  successful workspace trace.

---

## Architecture: two self-contained agent bundles, rote is opt-in

The project ships **two fully self-contained bundles** under
`.agents/`. Each bundle is a complete drop-in for the agent host
config dirs — no shared files, no merge logic. The duplication is
intentional; the cost is keeping non-MCP fields (e.g. model
definitions) in sync across both bundles.

| Folder              | Installed via                                                                              | Description                                                                                                              |
| ------------------- | ------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------ |
| `.agents/default/`  | `make agents-default-<host>` (one of opencode / kilo / pi / goose)                          | Rote-free baseline. Host configs wire the Kronk MCP server directly. Most contributors use this.                         |
| `.agents/rote/`     | `make agents-rote-<host>` (after `agents-rote-install` and `agents-rote-seed`)              | Rote-aware variant. Host configs have the Kronk MCP wiring stripped — rote brokers MCP calls. Requires invite (see §3).  |

Per-host targets so contributors only install what they actually use
— there is no "install everything" aggregate.

Per-bundle file layout (mirrored across both):

```diagram
.agents/<bundle>/
├── AGENTS.md
├── opencode/
│   ├── opencode.jsonc
│   └── auth.json
├── kilo/
│   └── kilo.json
├── pi/
│   ├── models.json
│   └── mcp.json
└── goose/
    ├── config.yaml
    └── custom_kronk.json
```

The rote bundle additionally carries `NOTES.md`, `adapters/`, and
`skills/kronk-mcp/SKILL.md` — these have no counterpart in the default
bundle.

| Per-host file                       | Differs between bundles? | Where the difference is                                       |
| ----------------------------------- | ------------------------ | ------------------------------------------------------------- |
| `opencode/opencode.jsonc`           | ✅                        | `mcp.kronk → localhost:9000/mcp` (default) vs no `mcp` block (rote) |
| `kilo/kilo.json`                    | ✅                        | `mcp.Kronk → localhost:9000/mcp` (default) vs no `mcp` block (rote) |
| `pi/mcp.json`                       | ✅                        | `mcpServers.kronk → localhost:9000/mcp` (default) vs `{ "mcpServers": {} }` (rote) |
| `AGENTS.md`                         | ✅                        | Direct-MCP tool policy (default) vs route-everything-through-rote (rote) |
| `opencode/auth.json`                | ❌                        | Identical — duplicated for self-containment                   |
| `pi/models.json`                    | ❌                        | Identical — duplicated for self-containment                   |
| `goose/config.yaml`                 | ❌                        | Identical (goose treats Kronk as LLM provider, not MCP source) |
| `goose/custom_kronk.json`           | ❌                        | Identical                                                     |

**Sync gotcha.** When you change a non-MCP field (e.g. add a model to
`opencode.jsonc` or `kilo.json`), make the same change in both
bundles. The differing files are small (1.5 KB each); a periodic
`diff -u .agents/default/<host>/<file> .agents/rote/<host>/<file>`
should always show only the `mcp` hunk.

**Forward rule.** To add a new external tool/service to this project,
do it under whichever bundle the new service belongs to:

- _Direct MCP service_ — wire it into each `.agents/default/<host>/`
  config alongside `kronk`. Update `.agents/default/AGENTS.md`.
- _Rote-brokered service_ — `rote adapter new-from-mcp <id> <url>`,
  mirror per §4 into `.agents/rote/adapters/<id>/`, then `make
  agents-rote-seed`. **No host MCP config changes** — that's rote's
  whole point.

---

## Installing rote

### CLI

The repo's makefile wraps the upstream installer:

```sh
make agents-rote-install # just rote (idempotent)
```

This drops the `rote` binary on `PATH` (typically `~/.local/bin/rote`)
and runs an interactive wizard:

1. Registry sign-in (request invite or claim an existing one)
2. Adapter selection from the catalog
3. OAuth / token configuration
4. Live proof-of-life run against each configured adapter

The script is **idempotent** — re-running upgrades the binary without
touching `~/.rote/`.

### Registry access (invite required — ask Bill)

Modiqo's registry is currently **invite-only**. The wizard's first
step is registry sign-in. If you don't have an account yet:

1. Stop and ask Bill (project owner) for an invite code.
2. Choose **"Claim invite"** in the wizard and paste the code, or
   pick **"SSO"** if Bill has already linked your identity provider.

The local-only parts of rote (the binary, `~/.rote/`, locally created
adapters like `kronk`) work without a registry session, but the
onboarding wizard will block on sign-in.

### VS Code extension (optional)

[Modiqo.rote](https://marketplace.visualstudio.com/items?itemName=Modiqo.rote).
Sidebar tree, Gantt timeline, command/response viewer, registry
browser — all on top of the same `~/.rote/` state the CLI uses.

### Update / uninstall

```sh
curl -fsSL https://getrote.dev/install | bash    # update (idempotent)
rm -rf ~/.rote                                    # remove all state
rm "$(which rote)"                                # remove the binary
```

---

## Makefile commands

All agent-related targets live in two sections near the bottom of the
[`makefile`](../../makefile) — `Agents — Default bundle` and
`Agents — Rote bundle`. There is no "install everything" aggregate;
contributors install only the host they actually use.

| Target                       | What it does                                                                                                                          |
| ---------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| `make agents-default-opencode` | Ships `.agents/default/opencode/` + the rote-free `AGENTS.md` to `~/.config/opencode/`. No skills shipped.                          |
| `make agents-default-kilo`     | Same, for kilo (`~/.config/kilo/`).                                                                                                  |
| `make agents-default-pi`       | Same, for pi (`~/.pi/`).                                                                                                              |
| `make agents-default-goose`    | Same, for goose (`~/.config/goose/`). Goose has no Kronk MCP wiring in either bundle (Kronk is goose's LLM provider).                 |
| `make agents-rote-install`     | Installs the rote CLI from `getrote.dev/install`. No-op if `rote` is already on `PATH`.                                              |
| `make agents-rote-login`       | Runs the rote registry login flow if not already logged in. Login state persists under `~/.rote/secrets/` + `~/.rote/registry/`, so this is one-time per machine until a wipe. Invite-only — see §3. |
| `make agents-rote-playground`  | Idempotently creates the long-lived `playground` workspace at `~/.rote/rote/workspaces/playground/`. Safe to run repeatedly. Requires login (depends on an internal `agents-rote-login-check` that fails fast with a pointer to `agents-rote-login` if you're not signed in). |
| `make agents-rote-seed`        | rsyncs the project's adapters from `.agents/rote/adapters/` into `~/.rote/adapters/`, then `rote adapter reindex kronk`. Depends on `agents-rote-playground` (and therefore on login).                |
| `make agents-rote-opencode`    | Ships `.agents/rote/opencode/` + the rote-aware `AGENTS.md` + the rote skill to `~/.config/opencode/`.                                |
| `make agents-rote-kilo`        | Same, for kilo.                                                                                                                       |
| `make agents-rote-pi`          | Same, for pi.                                                                                                                         |
| `make agents-rote-goose`       | Same, for goose.                                                                                                                      |

**Two clean bootstrap paths.** Most contributors will use the first;
rote requires an invite code from Bill (§3) so it stays opt-in.

_Non-rote contributors (the default):_

```sh
make install-tooling                # standard project tooling
make agents-default-<host>          # only the host you actually use
```

_Rote-using contributors:_

```sh
make agents-rote-install            # rote CLI (idempotent)
make agents-rote-login              # one-time browser flow — needs an invite code from Bill (§3)
make agents-rote-seed               # seeds ~/.rote/ and ensures the playground workspace exists
make agents-rote-<host>             # only the host you actually use
```

The login step is one-time per machine. Credentials persist on disk under
`~/.rote/secrets/` and `~/.rote/registry/`, so after the first run every
new shell finds you signed in. You only need to repeat it after a full
`rm -rf ~/.rote` wipe (see §6) or if Modiqo's registry token expires. If
you skip login, `agents-rote-seed` fails fast via an internal
`agents-rote-login-check` guard rather than emitting a confusing
`rote requires login` error from inside `rote init`.

Switching between bundles is just running the other `agents-<bundle>-<host>`
target — every per-host target overwrites the same files in `~/.config/<host>/`,
so the most recent invocation wins.

---

## Adapters

An **adapter** is rote's persistent, indexed representation of an
external service stored at `~/.rote/adapters/<name>/`. It's a typed,
searchable catalog of every operation that service exposes.

### Creating one (`adapter new-from-mcp`)

```sh
rote adapter new-from-mcp kronk http://localhost:9000/mcp
```

This:

1. Connects, runs the MCP `initialize` handshake, calls `tools/list`.
2. Builds a Tantivy semantic-search index over each tool's name +
   description.
3. Classifies each operation as read / write / destructive.
4. Fingerprints the tool schemas so rote can warn when the upstream
   API drifts.
5. Persists manifest, search index, policies, request log under
   `~/.rote/adapters/<name>/`.

### What the adapter gives you

| Command                                  | Purpose                                                                                      |
| ---------------------------------------- | -------------------------------------------------------------------------------------------- |
| `rote <adapter>_probe "<intent>"`        | Semantic search over the adapter's tools. Returns the right tool by intent. ~0 agent tokens. |
| `rote <adapter>_call <tool> '{json}' -s` | Execute a specific tool. Response cached as `@N.json` in the workspace.                      |
| `rote adapter info <adapter>`            | Inspect the manifest, fingerprint, stats.                                                    |
| `rote adapter list`                      | List all installed adapters.                                                                 |

**Tool names are bare in the catalog.** Kronk's adapter registers
`web_search` and `fuzzy_edit`. The `kronk_` prefix only applies to
the rote _shorthand verbs_ (`kronk_probe`, `kronk_call`). The
`<tool>` argument to `kronk_call` is the bare name — not
`kronk_web_search`.

---

## Mirror conventions

The `~/.rote/` tree is **per-machine** and not in the repo. To keep
adapters reproducible, a small subset is mirrored into
`.agents/rote/`:

| Path                   | Why include                                                |
| ---------------------- | ---------------------------------------------------------- |
| `manifest.json`        | Adapter identity, fingerprint, statistics                  |
| `tools.json`           | Tool catalog — **source of truth** the index is built from |
| `agent.md`             | Auto-generated subagent template                           |
| `config/policies.json` | Rate limits, retries, timeouts, circuit breaker            |
| `toolsets/`            | Toolset definitions                                        |

Five small reviewable files. Everything else under
`~/.rote/adapters/<name>/` is regeneratable runtime state and is
**excluded** from the mirror:

| Excluded path                 | Why                                                                                                                               |
| ----------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `~/.rote/secrets/`            | Encrypted tokens — never enter the repo                                                                                           |
| `<adapter>/runtime/`          | Per-execution scratch (response bodies, session state). Same category as `node_modules/`.                                         |
| `<adapter>/index/`            | Tantivy segment UUIDs change on every reindex — would generate binary diffs every run. Rebuilt locally by `make agents-rote-seed`. |
| `<adapter>/.tantivy-*.lock`   | Runtime locks tied to a live process                                                                                              |
| Workspace `responses/@N.json` | Often contains real API response data                                                                                             |

### Mirror commands

**Repo ← machine** (after editing an adapter live):

```sh
rsync -a --delete \
  --exclude 'runtime/' \
  --exclude 'index/' \
  --exclude '.tantivy-*.lock' \
  ~/.rote/adapters/<name>/ \
  .agents/rote/adapters/<name>/
```

The `--delete` keeps the mirror in lockstep with the live adapter.

**Machine ← repo** (new contributor / fresh box):

```sh
make agents-rote-seed
```

The seeding direction does **not** use `--delete` — it's additive, so
existing `~/.rote/` state (live workspaces, runtime caches, lock
files, secrets) is preserved.

### Defense in depth: root `.gitignore`

The repo-root [`.gitignore`](../../.gitignore) carries the same
patterns at the git layer (under a _"rote mirror under .agents/rote/"_
block), so a stray `cp -r` or a forgotten `--exclude` flag cannot
accidentally land per-machine state in a commit. Verified with `git
check-ignore -v`.

---

## Per-task workflow (when execution works again)

```diagram
╭──────────────╮     ╭──────────╮     ╭──────────╮     ╭──────────╮
│  workspace   │────▶│  probe   │────▶│   call   │────▶│  query   │
│ (playground  │     │ (find    │     │ (execute)│     │ (jq /    │
│  or per-task)│     │  tool)   │     │          │     │  @N)     │
╰──────────────╯     ╰──────────╯     ╰──────────╯     ╰──────────╯
                                                              │
                                                              ▼
                                                       ╭──────────╮
                                                       │  export  │
                                                       │  (flow)  │
                                                       ╰──────────╯
```

Always operate inside a workspace. There is one long-lived
`playground` workspace for ad-hoc exploration; per-task workspaces are
created when a flow's identity matters. **Workspaces are created by
make targets, not by agents** — see _Behaviors and gotchas_ below.

```sh
# Discover the right tool by intent (local — no live server roundtrip)
( cd ~/.rote/rote/workspaces/playground && rote kronk_probe "search the web" )

# Inspect probe results — probe wraps a JSON string in content[0].text,
# so fromjson is appropriate here.
rote @1 '.content[0].text | fromjson | .results[] | {name, score}'

# Execute a tool call. The `rm -f` is mandatory — see "Behaviors and
# gotchas" below for the rote 0.14.1 cached-session bug it works around.
rm -f ~/.rote/adapters/kronk/runtime/sessions/workspace_playground.json
rote kronk_call web_search '{"query":"go 1.24","count":3}' -s

# Read the tool result. Tool responses wrap PLAIN TEXT in content[0].text
# (no fromjson). Check (.is_error // false) for failure.
rote @2 '(.is_error // false)'
rote @2 '.content[0].text'

# Crystallize a successful exploration
rote flow template create --name <slug> --adapter adapter/kronk \
  --description "What this flow does" \
  --param "name:type:required:default:description" \
  --tag kronk
rote flow lint <slug>
rote flow release <slug>
rote flow index --rebuild
```

---

## Behaviors and gotchas

Verified empirically on rote 0.14.1 against Kronk's MCP server.

| Behavior                                                                                                | Notes                                                                                                                                                 |
| ------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| `rote init` is **not idempotent**                                                                       | Second call exits 1 with verbose error. `make agents-rote-playground` guards with a directory check.                                                  |
| `rote cd <name>` is **broken** on this machine                                                          | Errors with `command failed`. Workaround: `( cd ~/.rote/rote/workspaces/<name> && rote ... )` subshell. Each agent Bash call is a fresh shell anyway. |
| Workspace path layout has no `vendor` subdir                                                            | Because rote was installed via the CLI installer, not in Cursor / Claude / HTTP-vendor mode.                                                          |
| `rote ls` exit code lies                                                                                | Empty workspace prints `@@status\nerror: No responses yet` but exits 0. Real failures produce non-zero exit and a separate error line.                |
| MCP tool responses are wrapped                                                                          | Every result is `{ content: [ { type: "text", text: "<string>" } ], is_error?: bool }`. For Kronk's tools the `text` is **plain text**, not JSON — query with `.content[0].text`, do not pipe through `fromjson`. |
| `is_error` is **omitted on success**                                                                    | The field is set to `true` on tool failure and absent otherwise. Use `(.is_error // false)` when checking.                                            |
| `rote adapter new-from-mcp` writes per-host subagent files into `~/.claude/`, `~/.cursor/`, `~/.codex/` | Modiqo's auto-wiring for hosts they recognize. None go into Amp / OpenCode and none are mirrored into the repo.                                       |
| Small models substitute literal arguments                                                               | OpenCode/Qwen3.6-35B once renamed `playground` to `test workspace`. Workspace creation is therefore a make target, never an agent action.             |
| **rote caches `Mcp-Session-Id` per workspace and never re-handshakes**                                  | `~/.rote/adapters/kronk/runtime/sessions/workspace_<name>.json` holds `mcp_wire_session_id`. After any kronk-server lifecycle event (process restart, idle eviction) the cached ID is dead, every `kronk_call` returns `404 Not Found: session not found` in ~250 ms (no real roundtrip), and rote keeps reusing the dead ID forever. **Workaround**: precede every `kronk_call` with `rm -f ~/.rote/adapters/kronk/runtime/sessions/workspace_playground.json` (already baked into the SKILL.md and AGENTS.md examples). Costs ~600 ms per call for an extra `initialize` + `notifications/initialized` roundtrip; correctness wins. Upstream fix needed in rote — should drop the cached session and retry once on 404 from `tools/call`. |

---

## Reference

- `rote --help`, `rote why`, `rote how`, `rote start` — built-in CLI
  guidance.
- <https://www.modiqo.ai/llms-full.txt> — Modiqo's agent-readable
  index. Open it in a browser when you need the full upstream picture.
- `~/.rote/adapters/<name>/agent.md` — the auto-generated subagent
  template. Has detailed write-guard, flow-lint, and release-gate
  workflows worth reading before crystallizing flows.
- [`.agents/rote/skills/kronk-mcp/SKILL.md`](./skills/kronk-mcp/SKILL.md)
  — the single project skill, shipped only by `make agents-rote-<host>`.
  Named `kronk-mcp` (not `rote`) so the agent's first instinct when
  reaching for "the kronk MCP skill" matches what's installed; the
  skill body still teaches the rote workflow.
