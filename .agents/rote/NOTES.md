Hey Bill — Dave here. I've now read both the marketing site, the local `rote` CLI's `--help`, `why`, and `help` outputs, plus the VS Code extension's bundled README. Here's what rote actually is and how it would help an agent like OpenCode.

## What rote is (in one sentence)

**rote is an "execution layer" between coding agents and APIs/MCP servers.** It captures what an agent does the _first_ time it solves a problem, then crystallizes that exploration into a deterministic, replayable flow — so the next 1,000 runs cost ~200 tokens instead of ~12,000.

The marketing tagline says it well: _"Agents are creative by default. Production isn't. Rote is how they learn the difference."_

## The core problem it solves

Today an agent like OpenCode pays the same "exploration tax" every single time it does the same task:

| Per-run cost    | What it pays for                        |
| --------------- | --------------------------------------- |
| ~1,500 tok      | Discovery — "what tools exist?"         |
| ~5,000 tok      | Context storage of intermediate results |
| ~2,000 tok      | Reasoning — "what to do next?"          |
| ~3,700 tok      | Actual API payloads                     |
| **~12,200 tok** | **Total — paid every single run**       |

Worse, the same flow gets re-discovered by every teammate. The website's example screenshot of `~/agent-tools/` shows the exact failure mode: `stripe_refund.py`, `stripe_refund_v2.py`, `stripe_refund_FINAL.py` — all near-duplicates, none canonical, none remember what worked.

rote's pitch: do it once interactively, capture the trace, name it, and now it's a one-line recall for everyone.

## The five primitives — one closed loop

```
                  ╭─────────────╮
                  │   ADAPT     │  Any API/MCP/OpenAPI/GraphQL/gRPC
                  │ (adapter)   │  becomes callable. No SDK glue.
                  ╰──────┬──────╯
                         │
                         ▼
                  ╭─────────────╮
              ╭──▶│    ASK      │  Agent explores in plain language
              │   │  (canvas)   │  on the "canvas": probe → call →
              │   ╰──────┬──────╯  recover → converge
              │          │
              │          ▼
              │   ╭─────────────╮
              │   │ CRYSTALLIZE │  Successful trace compresses into
              │   │   (flow)    │  a named, deterministic flow
              │   ╰──────┬──────╯  (9 steps → 1.8s / 243 tokens)
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
                  │             │  Next Ask starts where last
                  ╰─────────────╯  Recall ended.
```

## How it actually works under the hood

From the CLI help (`rote --help`, `rote why`):

1. **Workspaces** (`rote init my-flow --seq`) — sandboxed scratch areas under `~/.rote/rote/workspaces/`.
2. **Cached responses** — every API call is saved as `@1.json`, `@2.json`, … on disk. Querying them costs **0 agent tokens** (it's just `jq`):
   ```
   rote @1 '$.result.sessionId' -s session_id -r
   ```
3. **Template substitution** — `$session_id` gets injected into later requests with `-t`, no LLM reasoning required.
4. **Adapters** — `rote adapter new-from-mcp notion https://mcp.notion.com/mcp` auto-discovers OAuth, registers a client, indexes operations, and gives you `notion_probe` (semantic search) and `notion_call` (execute) shortcuts.
5. **Export** — `rote export flow.sh` compiles the trace into a runnable shell/Deno script. From then on it runs with **zero agent involvement** — pure API tokens.
6. **Anti-pattern detection** — `rote detect <action-id>` analyzes a workflow trace and emits inline hints like:
   ```
   [HINT] Missing error check after API call
   [SUGGESTION] Add: is-error @2 && exit 1
   ```
   This is what they call **"feedback inlining"** — corrections live in the captured flow itself, not in the agent's system prompt (which would tax every run forever).
7. **Observability** — `rote ps` is "htop for MCP endpoints" — latency, throughput, error rates, anomaly detection per endpoint.

## How this makes OpenCode better and smarter

Three concrete wins:

1. **Token economics.** OpenCode's Tuesday becomes 98% cheaper on any task it does more than once. The website's benchmark on `stripe-dispute-triage`: 32s → 1.8s, 12k tokens → 243 tokens, n=100 runs.
2. **Determinism without losing creativity.** OpenCode keeps its exploratory power for _new_ problems. Solved problems collapse into `rote recall <flow>` — guaranteed-same-result, no LLM in the hot path.
3. **Collective memory.** `rote share` / `rote hub` means a flow you debug once becomes the team's default. The "five drafts → one canonical" problem from the marketing site goes away. Every wrong turn is also remembered so it isn't repeated.

The mental model they push hardest: **"curl is to HTTP as rote is to MCP."** It wants to be the standard utility you reach for when an agent has to talk to an API/MCP server reliably.

## What I'd suggest for testing

You said you want to test new skills next. Given how rote is structured, the natural first experiments are:

1. `rote config check` — see what MCP servers it already discovered on your machine.
2. `rote adapter list` — see what's installed; try `rote adapter new-from-mcp` against the Kronk MCP at `http://localhost:9000/mcp`.
3. Build a tiny canvas: `rote init kronk-search --seq`, do a `web_search` call interactively, then `rote export` it as a flow.
4. `rote install skill` — the help mentions installing a rote skill for Claude Code; we'd want the analogous wiring for OpenCode/Amp.

Want me to start with #1 and #2 to inventory what your machine already has, then design an Amp/OpenCode skill around the rote workflow?
