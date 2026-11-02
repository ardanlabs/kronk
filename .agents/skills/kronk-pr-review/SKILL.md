---
name: kronk-pr-review
compatibility: claude-code
description: "Guided, read-only Kronk PR/diff review (Kronk DiffGuard), written for Claude Code subagents. Reviews only the change (plus unchanged code it breaks or the issue requires) unless a full review is requested (changed files, changed packages, or whole project). Scripts preprocess the diff and run CI's Go static checks plus focused race tests once (never model-backed suites like sdk/kronk/tests). Parallel agents review acceptance criteria (BDD, incl. required behavior missing from unchanged files), correctness against Kronk ownership and inference invariants, errors, comments and manual drift, tests, SDK/HTTP contracts, duplication, simplification, and optionally security, across Go, React/TS, Rego, proto, Helm, Docker, and shell. Asks for the GitHub issue, flags weak criteria and oversized changes, skips generated files, writes an issues-only report plus Mermaid diagrams to .reviews/<id>/<NNN>/. USE for any review request: \"review this PR\", \"review my changes\", \"pre-commit review\", \"is this PR too big\", or one aspect (security, tests, errors, duplication)."
---

# Kronk PR Review

Written for Claude Code (subagents, project memory, agent transcript files); in another runtime, run the roles yourself and use step 7's manual fallback.

You are the **orchestrator**: scripts do the mechanical work once, a context builder writes a shared brief, review agents run in parallel and write files, an aggregator verifies and writes the report and diagrams. `<skill>` = this file's directory; `<run>` = `<project-root>/.reviews/<id>/<NNN>` (printed by intake).

Rules for you:

- **Start from a fresh session.** Every orchestrator turn re-reads the whole conversation; inside a long session that costs more than the agents. If the conversation already holds unrelated work, suggest `/clear` (or a new session) before starting.
- **Keep your context small.** Read only script output, agents' one-line replies (the aggregator's also carries `findings.md`, which you only save), `ac-review.md` (when ACs are weak/missing), `references/size-guard.md` (when there are size warnings), and the `F…` headings of `findings.md`. Never read the diff, brief, check logs, or agent reports.
- **Advisory only.** Nothing here edits, stages, or commits code.
- **The change, not the codebase.** By default only the change is reviewed (`contract.md` → Review scope). Problems that predate it are dropped, not reported. Only an explicit full-review request widens that.
- **Runs are independent.** Never open a previous run's files.

Scripts take the same options in both shells (PowerShell spells them `-Base`, `-DryRun`, …; lists comma-separated). Run them inside the target repo:

- sh: `sh <skill>/scripts/intake.sh …`, `sh <skill>/scripts/checks.sh <run> …`
- PowerShell 7: `pwsh -NoProfile -ExecutionPolicy Bypass -File <skill>/scripts/intake.ps1 …` (same for `checks.ps1`; verified on pwsh 7 only)

Kronk rules the scripts apply on their own (from `AGENTS.md` and the Linux CI `static`/`race` jobs), so never ask about them or pass them by hand:

- `GITHUB_WORKSPACE` is set to the repo root for every check.
- Tests run as `go test -race -short -count=1` on the reviewed packages only (the changed ones at `diff` depth), never `./...` and never at `project` depth (`AGENTS.md` forbids a full repo test run), with `KRONK_BASE_PATH` pointed at an empty store so in-package model-backed tests skip instead of running inference. Packages under `sdk/kronk/tests`, `sdk/bucky/tests`, `sdk/kronk/pool`, and `cmd/server/api/services/kronk/tests` are never tested: they need native libraries, models, or a server, and belong to CI or humans.
- Go static checks follow CI's `static` job: `go vet ./...` per module (as CI runs it), and on the reviewed packages only `staticcheck -checks=all`, `go fix -diff`, `govulncheck`; plus `gofmt -s -l` on the reviewed files (`AGENTS.md` adds `-s`) and `go mod tidy -diff`. `goreleaser check` is not run. Build output goes to a temp dir, so checks never write to the repo. BUI TypeScript changes get `tsc --noEmit` when `node_modules` exists; Helm chart changes get `zarf/helm/kronk/test.sh` when `helm` and Python's PyYAML are installed.
- Someone else's fork PR is untrusted: intake writes `trusted=no` when the PR (`--pr`, or the current branch's open PR) comes from another repository and you are not its author, or the branch tracks `refs/pull/*`; `--untrusted` forces it. Then `checks.sh` runs nothing from the reviewed tree (`go test`, the Helm test script, and the BUI `tsc` are skipped; builds, vet, and the analyzers only read the code) unless given `--trust`, and reviewers execute nothing from it.
- Generated outputs (BUI docs components, `cli-reference.json`, the embedded `static/` bundle, `*.pb.go`, `gomod2nix.toml`) are skipped like any generated file; a changed source without its regenerated output is a reviewer finding.

## 1. Parse the request

Note what was given: scope (default: all uncommitted changes, staged and unstaged, plus untracked; or `--staged`, `--base <ref>`, `--range A..B`, `--pr <n>`, `--files <paths>`), review depth, ticket, issue id, agent set, security mode, model tier, exclusions.

**Review depth** is `diff` (changes only) unless the user explicitly asks for a full review: `--full-code`, "full review", "full code review", or "review the whole file / package / project". Picking every agent (*All agents*) is not a full review. A request that names the extent maps straight to a depth: files → `files`, packages → `packages`, project or repo → `project`. Otherwise ask in step 2.

## 2. Preview

If a full review was requested without an extent, ask first; the preview needs the answer: *Changed files* (default) · *Changed packages* (every file in their directories) · *Whole project* (no tests run). Map the answer to `--depth files|packages|project`.

`intake.sh --dry-run <scope> [--depth <depth>] [--id <ticket id>]` writes nothing. On exit `3` the reviewed code isn't checked out: relay the printed command and stop. On `4` there is nothing to review: say so and stop. On `2` fix the arguments.

**Missing tools come first.** If `missing_tools > 0`, before any question show the `# missing tools` rows as a table (tool, what it checks, install command) and say: install them now and they are used by this run; the review goes ahead either way, and a missing tool's checks show as `skipped`. Rows marked `rebuild` are installed but built with an older Go than the active toolchain, so they fail on its standard library — the same install command rebuilds them. A `cgo` row means `go test -race` will fail until a C compiler is available. Rows whose purpose says `optional, not in CI` are extra analyzers: name them in one line and don't push installs; their package-wide output is filtered to in-scope code by the reviewers.

## 3. Ask

One round (native question UI if available, ≤2 batches). Skip what the request already answered.

1. **Ticket link** — always ask if none was given (normally an `ardanlabs/kronk` GitHub issue). "No ticket" is valid: spec review then works best-effort from the PR description and commits.
2. **Scope and id** — confirm the preview's scope, counts, `id` (ticket id, else branch name — same work, same directory), and depth (at full depth, `review_files` / `review_lines` too).
3. **Size** — if `size_warnings > 0`, follow `references/size-guard.md`.
4. **Agents** — *Routed* · *All agents* · *All agents + Security* · *Partial* (see step 6), naming the agents and checklists in each option, and saying when the small-diff merge applies. *Partial* asks a follow-up: *Risk only* or *Polish only*.
5. **Security** — only with *All agents + Security*; then ask:
   - fan-out: *single* · *capped* (≤3 attacker sub-agents) · *full* (attacker + refuter per route);
   - report: *redacted* (default — `findings.md` says only "Security issues detected."; details only in `security-issues/`) · *named* (security issues listed in `findings.md` and `diagrams.md` like any finding).
6. **Secret-scan generated files** — if `skipped_files > 0`. Off by default. Give the file count and `noise_tokens_est` as the worst-case cost; a secret scanner in `tools.tsv` makes it much cheaper.
7. **Model tier** — if your runtime picks models per subagent: run the context builder and generated-file scan on a cheaper model? Every run. Reviewers and the aggregator always use the session model.
8. **Security PoC target** — *All agents + Security* only, and only if memory has no `kronk-project-interaction-guide`: is a disposable local Kronk server running for PoCs (host, port, how to get a token), or are PoCs static-only? Save the answer as project memory `kronk-project-interaction-guide` (no memory → ask every run). Never ask about test setup: `checks.sh` already applies the Kronk rules above.
9. **Untrusted code** — if the preview says `trusted=no`: name `trust_reason`, say its tests, Helm test script, and BUI `tsc` won't run and no agent will execute its code, then ask *Keep untrusted* (default) · *Trust it* (only once you have read the diff; adds `--trust` to `checks.sh`). Reviewers still never run it.

The answers are the confirmation; echo the final setup in ≤6 lines and continue.

## 4. Prepare

1. `intake.sh <scope> --id <id> [--depth <depth>] [--exclude <path>]…` creates `<run>`. `checks.sh` reads the depth from `scope.txt`.
2. Save the ticket raw to `<run>/context/ticket.md` without reading it — `gh issue view <n> --repo ardanlabs/kronk --comments > …` first, then a tracker MCP, then web fetch.
3. In parallel: `checks.sh <run> [--trust] [--exclude <pkg path>]… [--timeout D]` in the background, and the context builder: "Follow `<skill>/references/roles/context-builder.md`. Run directory: `<run>`."

## 5. Acceptance-criteria gate

If the builder replies `ac_status=weak` or `missing`, read `<run>/context/ac-review.md`, show its problems and suggested Given/When/Then criteria, and ask:

- **I'll fix the ticket** — suggest pasting the BDD criteria in, so each can be reported Met / Partial / Unmet. Once done, re-fetch the ticket and tell the builder: "Refresh only the acceptance criteria in brief.md and ac-review.md from the updated ticket."
- **Use the suggestions for this run** — no builder re-run: give spec and the aggregator `Criteria in force: suggested BDD (context/ac-review.md)`.
- **Proceed as-is** — spec reviews the criteria as written and reports their weaknesses.

## 6. Review agents

After checks finish, launch the chosen agents in one message.

| Agent         | Checklists (`references/lenses/`)                                                                    | Routed when                            |
|---------------|------------------------------------------------------------------------------------------------------|----------------------------------------|
| `security`    | `security-sentinel`                                                                                  | *All agents + Security* only           |
| `spec`        | `spec-cartographer`                                                                                  | ticket given (then mandatory)          |
| `correctness` | `service-steward`, + `error-tripwire` if `failure=1`, + `boundary-keeper` if `contract=1`            | `prod_files > 0`                       |
| `tests`       | `harness-map`                                                                                        | `prod_files > 0` or `test_files > 0`   |
| `polish`      | `clone-hunter` if `logic=1`, `doc-drift-check` if `comments=1`; `straight-line-pass` in All agents/Polish only | `logic=1` or `comments=1`    |

*Routed*: the non-security agents whose condition holds. *All agents*: every agent and checklist except `security` (spec best-effort without a ticket). *All agents + Security*: *All agents* plus `security`. *Partial* → *Risk only*: spec, correctness, tests; *Polish only*: spec, polish. Only *All agents + Security* runs `security`; spec is never dropped when there is a ticket. The agent set never changes the review depth.

**Small diff** (`code_lines_changed ≤ 150` at `diff` depth; at full depth, `review_code_files ≤ 5` and `review_lines ≤ 600`): run the chosen `correctness` and `tests` checklists as one `review` agent — each agent costs a full context before it reads anything. Security, spec, and polish stay separate (polish findings get crowded out when merged).

If `shards > 0`, run `correctness` and `tests` once per shard.

Prompt:

```text
You are the <agent> reviewer in a PR review run.
Read <skill>/references/contract.md, then <skill>/references/lenses/<a>.md[, …].
Run directory: <run>. Shard: <k | all>. Review depth: <diff | files | packages | project>.
Write your report to <run>/lenses/<agent>[-<k>].md and reply with the one line the contract specifies.
```

Append for `security`: `Fan-out: <mode>. Project interaction guide: <memory text | none>. PoCs only under <run>/security-issues/poc/<name>/.` For `spec`: `Criteria in force: <ticket | suggested BDD (context/ac-review.md) | best-effort>.`

If the generated-file scan was accepted, also launch: "Follow `<skill>/references/roles/generated-scan.md`. Run directory: `<run>`. Skill directory: `<skill>`." No other agent reads generated files.

## 7. Aggregate and draw

Aggregator: "Follow `<skill>/references/roles/aggregator.md`. Run directory: `<run>`. Skill directory: `<skill>`. Review depth: <…>. Criteria in force: <…>. Security: <off | redacted | named>." It verifies the findings, writes `security-issues/` (when security ran) and `diagrams.md`, and returns `findings.md` in its reply instead of writing it (some runtimes refuse subagent writes of report files). Before launching the aggregator, if any reviewer's reply says its write was refused, save the report from that reply verbatim to `<run>/lenses/<agent>[-<k>].md`.

Save `findings.md` without re-typing it: if your runtime gives the aggregator's transcript as a file (Claude Code: the agent's `output_file`), run `sh <skill>/scripts/extract-findings.sh <output_file> <run>` (PowerShell: `extract-findings.ps1`). On exit `3` (no `jq`) or `4`, or with no transcript file, write the text between `===== findings.md =====` and `===== end =====` to `<run>/findings.md` verbatim — no edits, no summary. If a subagent says a write was refused, never have it, or anyone, retry through Bash; tell the user.

## 8. Report

From its reply and `rg '^#### F' <run>/findings.md`: review depth, overall Gate, counts per Gate, G4/G3 titles, the security-issue count (titles only in *named* mode), AC verdict counts, size warnings, and the paths to `findings.md`, `diagrams.md`, and `security-issues/`. If `reopened` is not `no`, offer to re-run the agent it names. Don't restate the report.

## Run layout

```
<project-root>/.reviews/<id>/<NNN>/     NNN increments per run; never overwritten
  findings.md                    issues only; the AC table is the one place "Met" appears;
                                 in *redacted* mode security issues appear only as "Security issues detected."
  diagrams.md                    green added, red removed, amber modified, purple ⚠ findings (security only in *named* mode)
  security-issues/               only when security review ran
  security-issues/issue-NNN.md   one file per verified security finding
  security-issues/open-checks.md unproven security suspicions
  security-issues/poc/<name>/    optional PoCs
  lenses/<agent>[-<k>].md        raw agent reports
  context/                       scope.txt files.tsv skipped.tsv symbols.tsv refs.tsv signals.txt shards.tsv
                                 size.tsv packages.tsv rules.txt tools.tsv missing.tsv diff.patch generated.patch
                                 review-set.tsv (full depth)
                                 ticket.md pr.md commits.txt brief.md ac-review.md checks/
```

Generated and vendored files (lockfiles included) and symlinks are never reviewed: intake moves them to `skipped.tsv` and `generated.patch`. Never open a symlink in the reviewed repo: its target can be outside the repo.
