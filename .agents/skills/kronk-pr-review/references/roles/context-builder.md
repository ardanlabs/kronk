# Context builder

Write the brief every reviewer reads first, so none of them re-discovers the change. **Describe; don't review** — no findings, gates, or quality opinions. Your one judgment is on the acceptance criteria.

**Inputs** (`<run>/context/`): `scope.txt`, `files.tsv`, `stack.tsv`, `symbols.tsv`, `refs.tsv`, `diff.patch`, `rules.txt`, `review-set.tsv` (non-empty at full depth only), and whichever of `stack.md`, `ticket.md`, `pr.md`, `commits.txt` exist. Don't run `git`. Ignore `skipped.tsv` and never read `generated.patch`. Write only the two files below.

## `brief.md` (≤ 400 lines)

```markdown
# Change brief

## Summary
<≤10 lines: what changed and why>

## Acceptance criteria
Source: <ticket | PR description | commits | none>
In force: <ticket | suggested BDD>
- AC1: <verbatim from the source>

## Files
<grouped by unit (stack.tsv) or package/area, every row of files.tsv>
- `path` (status, lang, layer) — <what changed, one line>

## Changed symbols and callers
<exported or cross-package symbols from symbols.tsv>
- `Name` (status, `path:line`) — callers: `file:line`, … (≤15, then "+N more")

## Review set
<full depth only (`depth` ≠ `diff` in scope.txt): one line per area of review-set.tsv — files, lines, what it holds; don't describe untouched files one by one>
- `dir/` — <n> files, <lines> lines — <what it holds>

## Repo rules
- <review-relevant rule> (`source path`)
- <the Chapter 20 §20.2 row(s) owning the changed paths: owner, read-adjacent, focused verification>

## Risk pointers
<facts only: changed code touching admission permits, active-use counts, pool reservations, slots/sequences, IMC sessions or snapshots, native objects, speculative/MTP state, concurrency, downloads/external I/O, auth, config/secrets, public SDK or HTTP API, generated outputs; for infrastructure: RBAC, network exposure, stateful resources (PVCs), rollout settings, CI triggers and permissions>
- `path:line` — <what it touches>
```

Start from `refs.tsv`; confirm or prune with the language tool (Go: `gopls` MCP `go_symbol_references`) when a name is ambiguous. For removed symbols, list the references that remain — a dangling caller is worth recording. Read your inputs in one batch.

## `ac-review.md`

```markdown
# Acceptance-criteria assessment
Status: ok | weak | missing

## Problems
- AC<n>: <missing | ambiguous | untestable | under-specified | contradictory | doesn't make sense> — <why>

## Suggested BDD criteria
### AC1 — <name>
- Given <precondition>
- When <action>
- Then <observable outcome>
```

- **missing** — no ticket, or no explicit criteria. Derive suggestions from ticket, PR, commits, and diff; label each one's origin.
- **weak** — at least one criterion can't be judged Met / Partial / Unmet without guessing (ambiguous, untestable, contradictory, nonsensical, or no failure/edge behavior).
- **ok** — all concrete and testable; omit the suggestions.

Suggestions cover failure, empty, and edge states, not just the happy path.

## Reply

```
brief: ACs=<n> ac_status=<ok|weak|missing> symbols=<n> — <run>/context/brief.md
```
