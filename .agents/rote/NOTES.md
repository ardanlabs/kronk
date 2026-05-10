# rote — Project Documentation

This document is the developer reference for how Kronk uses
[rote](https://www.modiqo.ai/) as the single execution layer between
coding agents (OpenCode, Amp, Kilo, Pi, Goose) and external
tools/APIs/MCP services.

## Status (read first)

| Capability                                  | Works? | Notes                                                             |
| ------------------------------------------- | ------ | ----------------------------------------------------------------- |
| `rote kronk_probe "<intent>"`               | ✅     | Local Tantivy lookup over the cached tools catalog                |
| `rote adapter list` / `rote adapter info`   | ✅     | Local manifest reads                                              |
| `rote kronk_call <tool> '{...}'`            | ❌     | Rote v0.13.0 skips MCP handshake (see *Known issues*)             |
| Crystallizing flows that call kronk tools   | ❌     | Blocked on `kronk_call` — can't crystallize what won't execute    |

**Bottom line:** today the agent can *discover* tools via rote but
cannot *execute* them. Until the upstream bug is fixed, agents fall
back to the host's built-in tools for file edits and web research.

## Canonical references

| Resource                                  | URL                                                                | Local snapshot                                       |
| ----------------------------------------- | ------------------------------------------------------------------ | ---------------------------------------------------- |
| Marketing site                            | <https://www.modiqo.ai/>                                           | —                                                    |
| **LLM-readable full index** (recommended) | <https://www.modiqo.ai/llms-full.txt>                              | [`refs/llms-full.txt`](./refs/llms-full.txt)         |
| VS Code extension                         | <https://marketplace.visualstudio.com/items?itemName=Modiqo.rote>  | `~/.vscode/extensions/modiqo.rote-0.13.0/readme.md`  |
| Discord                                   | <https://discord.gg/NmHjhxF3G>                                     | —                                                    |
| GitHub                                    | <https://github.com/modiqo/rote>                                   | —                                                    |

Refresh the local snapshot of `llms-full.txt` with:

```sh
curl -fsSL https://www.modiqo.ai/llms-full.txt -o .agents/rote/refs/llms-full.txt
```

(Note: `modiqo.ai` 302-redirects to `www.modiqo.ai`. Always use `-L`
or the `www.` URL directly.)

---

## 1. What rote is

rote is an **execution layer** between coding agents and APIs / MCP
servers. It captures what an agent does the *first* time it solves a
problem and crystallizes it into a deterministic, replayable flow.
Subsequent runs cost ~200 tokens instead of ~12,000.

Mental model the docs push:
> *curl is to HTTP as rote is to MCP.*

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

- **Workspace (canvas)** — `~/.rote/rote/workspaces/<name>/`. A
  scratch area where each request is cached as `@1.json`, `@2.json`,
  etc. Required for `probe`, `call`, `query`, `template` commands.
- **Adapter** — `~/.rote/adapters/<name>/`. Persistent typed catalog
  of an external service: manifest, tools list, search index,
  policies (rate limits, retries, timeouts), fingerprint.
- **Flow** — `~/.rote/flows/<slug>/main.ts`. A crystallized,
  parameterized, lint-checked, releasable script generated from a
  successful canvas trace.

---

## 2. Architecture: rote is the only path

Every agent host config has had its raw kronk MCP wiring removed.
Hosts call rote via their built-in Bash tool. This is enforced by the
**absence** of MCP wiring in host configs, not by skill text alone.

| Host                                       | rote wiring                          |
| ------------------------------------------ | ------------------------------------ |
| `.agents/opencode/opencode.jsonc`          | No `mcp` block                       |
| `.agents/kilo/kilo.json`                   | No `mcp` block                       |
| `.agents/pi/mcp.json`                      | `{ "mcpServers": {} }`               |
| `.agents/goose/config.yaml`                | Platform extensions only             |
| `.agents/skills/rote/SKILL.md`             | Single project skill                 |
| `.agents/AGENTS.md`                        | Tool policy routes everything to rote |

**Forward rule.** To add a new external tool/service to this project:

1. `rote adapter new-from-mcp <id> <url>` (or
   `rote adapter new <id> <openapi-or-graphql-spec>`).
2. Mirror per §4: `rsync` into `.agents/rote/adapters/<id>/`.
3. `make copy-agent-rote` (or commit so other contributors pick it
   up via `make copy-agent-configs`).
4. **No skill file changes. No host MCP config changes.**

---

## 3. Installing rote

### CLI

The repo's makefile wraps the upstream installer:

```sh
make install-rote      # just rote (idempotent)
make install-tooling   # rote + protobuf + grpcurl + node
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

## 4. Makefile commands

All rote-related targets live in a "Rote" section near the bottom of
the [`makefile`](../../makefile).

| Target                  | What it does                                                                                                                                  |
| ----------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `make install-rote`     | Installs the rote CLI from `getrote.dev/install`. No-op if `rote` is already on `PATH`. Pulled in by `make install-tooling`.                  |
| `make rote-playground`  | Idempotently creates the long-lived `playground` canvas at `~/.rote/rote/workspaces/playground/`. Safe to run repeatedly.                     |
| `make copy-agent-rote`  | rsyncs the project's adapters from `.agents/rote/adapters/` into `~/.rote/adapters/`, then runs `rote adapter reindex kronk` to rebuild the local Tantivy index. Depends on `rote-playground`. |
| `make copy-agent-configs` | Runs `copy-agent-rote` plus all the per-host config copies (opencode, kilo, pi, goose).                                                     |

**A new contributor's bootstrap is two commands:**

```sh
make install-tooling      # rote + brew tooling (one-time)
make copy-agent-configs   # seeds ~/.rote/, ~/.config/opencode, ~/.config/kilo, ~/.pi, ~/.config/goose
```

---

## 5. Adapters

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

| Command                                        | Purpose                                                                                                  |
| ---------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| `rote <adapter>_probe "<intent>"`              | Semantic search over the adapter's tools. Returns the right tool by intent. ~0 agent tokens.             |
| `rote <adapter>_call <tool> '{json}' -s`       | Execute a specific tool. Response cached as `@N.json` in the workspace.                                  |
| `rote adapter info <adapter>`                  | Inspect the manifest, fingerprint, stats.                                                                |
| `rote adapter list`                            | List all installed adapters.                                                                             |

**Tool names are bare in the catalog.** Kronk's adapter registers
`web_search` and `fuzzy_edit`. The `kronk_` prefix only applies to
the rote *shorthand verbs* (`kronk_probe`, `kronk_call`). The
`<tool>` argument to `kronk_call` is the bare name — not
`kronk_web_search`.

---

## 6. Mirror conventions

The `~/.rote/` tree is **per-machine** and not in the repo. To keep
adapters reproducible, a small subset is mirrored into
`.agents/rote/`:

| Path                       | Why include                                                                       |
| -------------------------- | --------------------------------------------------------------------------------- |
| `manifest.json`            | Adapter identity, fingerprint, statistics                                         |
| `tools.json`               | Tool catalog — **source of truth** the index is built from                        |
| `agent.md`                 | Auto-generated subagent template                                                  |
| `config/policies.json`     | Rate limits, retries, timeouts, circuit breaker                                   |
| `toolsets/`                | Toolset definitions                                                               |

Five small reviewable files. Everything else under
`~/.rote/adapters/<name>/` is regeneratable runtime state and is
**excluded** from the mirror:

| Excluded path                 | Why                                                                                                                        |
| ----------------------------- | -------------------------------------------------------------------------------------------------------------------------- |
| `~/.rote/secrets/`            | Encrypted tokens — never enter the repo                                                                                    |
| `<adapter>/runtime/`          | Per-execution scratch (response bodies, session state). Same category as `node_modules/`.                                  |
| `<adapter>/index/`            | Tantivy segment UUIDs change on every reindex — would generate binary diffs every run. Rebuilt locally by `make copy-agent-rote`. |
| `<adapter>/.tantivy-*.lock`   | Runtime locks tied to a live process                                                                                       |
| Workspace `responses/@N.json` | Often contains real API response data                                                                                      |

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
make copy-agent-rote
```

The seeding direction does **not** use `--delete` — it's additive, so
existing `~/.rote/` state (live workspaces, runtime caches, lock
files, secrets) is preserved.

### Defense in depth: root `.gitignore`

The repo-root [`.gitignore`](../../.gitignore) carries the same
patterns at the git layer (under a *"rote mirror under .agents/rote/"*
block), so a stray `cp -r` or a forgotten `--exclude` flag cannot
accidentally land per-machine state in a commit. Verified with `git
check-ignore -v`.

---

## 7. Per-task workflow (when execution works again)

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
`playground` canvas for ad-hoc exploration; per-task workspaces are
created when a flow's identity matters. **Workspaces are created by
make targets, not by agents** — see *Behaviors and gotchas* below.

```sh
# Discover the right tool by intent (local — no live server roundtrip)
( cd ~/.rote/rote/workspaces/playground && rote kronk_probe "search the web" )

# Inspect probe results (MCP responses are double-wrapped — see gotchas)
rote @1 '.content[0].text | fromjson | .results[] | {name, score}'

# Execute (BROKEN in v0.13.0 — see "Known issues")
rote kronk_call web_search '{"query":"go 1.24","count":3}' -s

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

## 8. Behaviors and gotchas

These have been verified empirically on rote v0.13.0 against
Kronk's MCP server.

| Behavior                                                                                                              | Notes                                                                                                                                                |
| --------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| `rote init` is **not idempotent**                                                                                     | Second call exits 1 with verbose error. `make rote-playground` guards with a directory check.                                                        |
| `rote cd <name>` is **broken** on this machine                                                                        | Errors with `command failed`. Workaround: `( cd ~/.rote/rote/workspaces/<name> && rote ... )` subshell. Each agent Bash call is a fresh shell anyway. |
| Workspace path layout has no `vendor` subdir                                                                          | Because rote was installed via the CLI installer, not in Cursor / Claude / HTTP-vendor mode.                                                          |
| `rote ls` exit code lies                                                                                              | Empty workspace prints `@@status\nerror: No responses yet` but exits 0. Real failures produce non-zero exit and a separate error line.               |
| MCP tool responses are **double-wrapped**                                                                             | Every result is `{ content: [ { type: "text", text: "<json string>" } ], is_error?: bool }`. Use `.content[0].text \| fromjson \| <real query>`.     |
| The documented `-m / --mcp` unwrap flag on `rote @N` **does not work** in v0.13.0                                     | Errors with `configuration error: invalid flag`. Use `fromjson` until upstream fixes it.                                                             |
| Always check `.is_error` before trusting `.content[0].text`                                                           | `is_error: true` means the tool itself failed even though the HTTP transport returned 200.                                                           |
| `rote adapter new-from-mcp` writes per-host subagent files into `~/.claude/`, `~/.cursor/`, `~/.codex/`               | Modiqo's auto-wiring for hosts they recognize. None go into Amp / OpenCode and none are mirrored into the repo.                                      |
| Small models substitute literal arguments                                                                              | OpenCode/Qwen3.6-35B once renamed `playground` to `test workspace`. Workspace creation is therefore a make target, never an agent action.            |

---

## 9. Known issues

### 9.1 `kronk_call` skips the MCP handshake (rote v0.13.0)

**Symptom.** Every `rote kronk_call <tool>` returns
`@@status ok: @N` but `.is_error` is `true` and `.content[0].text` is:

```
HTTP execution failed for web_search: MCP error:
  {"code":0,"message":"method \"tools/call\" is invalid during session initialization"}
```

**Root cause — verified at the wire level.** rote v0.13.0 sends
`tools/call` as a stateless one-shot HTTP POST with **no
`initialize`, no `notifications/initialized`, no `Mcp-Session-Id`
header**. The MCP spec mandates a session — every spec-compliant
streamable-HTTP server will reject this exactly the way Kronk's
`github.com/modelcontextprotocol/go-sdk` v1.6.0 does.

The bug also affects introspection (`adapter new-from-mcp`), which
sends `initialize` then jumps directly to `tools/list`, skipping
`notifications/initialized` between them. Kronk's go-sdk happens to
let this slide; stricter MCP servers will not.

**Status.** Upstream issue to be filed with Modiqo. Until fixed,
`rote kronk_probe` works (it's a local Tantivy lookup) but no kronk
tool can actually be executed through rote.

**Workaround.** Until the fix lands, agents fall back to host-native
tools for the operations rote was supposed to broker. The rote-only
architecture stays as the documented end state.

---

## 10. Reference

- `rote --help`, `rote why`, `rote how`, `rote start` — built-in CLI
  guidance.
- [`refs/llms-full.txt`](./refs/llms-full.txt) — Modiqo's
  agent-readable index. Load this first in any rote-related session.
- `~/.rote/adapters/<name>/agent.md` — the auto-generated subagent
  template. Has detailed write-guard, flow-lint, and release-gate
  workflows worth reading before crystallizing flows.
- [`.agents/skills/rote/SKILL.md`](../skills/rote/SKILL.md) — the
  single project skill loaded by every agent host.
