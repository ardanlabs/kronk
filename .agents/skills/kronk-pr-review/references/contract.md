# Reviewer contract

Every review agent follows this file, its lens checklists, and the idiom files in its prompt. `<run>`, `<skill>`, the review depth, and the idiom files come from your prompt.

## Review scope

The depth (`Review depth:` in your prompt, `depth=` in `scope.txt`) sets what you may report on. Read whatever you need for context; scope limits only what becomes a finding.

- **`diff`** (default): review the change, not the code around it. A finding must sit on a line the diff adds, modifies, or removes, or on unchanged code that is either:
  - **broken by the change**, such as a caller of a changed signature, a reader of a changed format, or a test the change makes fail. Cite the unchanged line and the change that breaks it.
  - **required to change by the criteria in force**, such as a site the ticket covers that the diff skipped (spec lens).

  A problem that predates the change, and that the change neither causes nor worsens, is out of scope. Drop it silently: it is not a finding, an Open check, or a note. The same applies to `checks/` hits on lines the diff doesn't touch.
- **`files` / `packages` / `project`**: full review. Every line of the files in `review-set.tsv` (or of your shard) is in scope, including problems that predate the change. Add `- Origin: pre-existing` to each finding on a line the diff didn't touch. Rows with `status` other than `-` are the change; read those most closely.

## Inputs (in `<run>/context/`, already prepared)

| File                              | Holds                                                                                                   |
|-----------------------------------|---------------------------------------------------------------------------------------------------------|
| `brief.md`                        | **Read first.** Summary, acceptance criteria (AC1…), per-file notes, changed symbols and callers, repo rules. |
| `scope.txt`                       | Scope, head commit, counts, ecosystems, idioms, `generated=` (skipped or included), `trust=` (`maintainer` or `untrusted`) with `trust_reason=`, routing signals. |
| `files.tsv`                       | `status added deleted lang kind layer path` per reviewed file; `kind` is prod (code and infrastructure), test, doc, config, or — only with `generated=included` — generated/vendor. `layer` is the Kronk owner (`cli`, `foundation`, `app-sdk`, `app`, `service`, `tooling`, `bui`, `sdk-kronk`, `sdk-model`, `sdk-parsers`, `sdk-pool`, `sdk-bucky`, `sdk-malina`, `sdk-tools`, `sdk`, `examples`, `manual`, `deploy`, `ci`, or `-`). |
| `stack.tsv`, `stack.md`           | `eco unit files marker` per build/deploy unit (Go module, chart, …); `stack.md` (when present): the repo's ecosystems, its own quality gates, and idioms for ecosystems with no idiom file. |
| `symbols.tsv`                     | `status kind name path line` for added, removed, and modified funcs, types, Terraform blocks (addresses such as `aws_s3_bucket.this`, `var.x`), and Helm templates. |
| `refs.tsv`                        | `symbol path:line` — word-match references to each changed symbol (capped, `+N more`). A starting list; confirm with the language tool. |
| `diff.patch`                      | The diff (8 context lines). Find `diff --git a/<path>` and read only the sections you need.             |
| `review-set.tsv`                  | Full depth only: the `files.tsv` columns plus `lines` for every file in scope; `status` `-` = untouched by the diff. Empty at `diff` depth. |
| `shards.tsv`                      | `shard path`, large diffs or review sets only. With a shard in your prompt, review only its files.      |
| `checks/summary.tsv`              | `check status exit lines file lens` — builds, linters, tests, analyzers, scanners, clone detectors, and user-approved repo gates (`x-*`), already run. Status `pass`, `fail`, `findings` (hits reported on output), or `skipped`; `lens` owns its failures. Open a log only when its row matters. Unit-wide tools (Go packages, crates, modules, charts) report on whole units: only hits in scope (see Review scope) are findings. |
| `ticket.md`, `pr.md`, `commits.txt` | Ticket, PR description, commit messages (whichever exist).                                          |
| `rules.txt`                       | `AGENTS.md`/`CLAUDE.md` files governing the changed paths; the brief digests them.                     |
| `skipped.tsv`                     | Files not reviewed: symlinks, and generated/vendored files unless `generated=included`. Never read them or `generated.patch`. Never open a symlink (`kind=symlink`): its target can be outside the repo. |

## Rules

1. **Batch your reads.** Each turn re-reads your whole context, so issue independent reads (contract, lenses, brief, the files you need) together in one turn, not one per turn.
2. **Don't redo preprocessing.** No `git` to rediscover scope, no reading the whole diff to get oriented, no re-running what `checks/` covers. One narrow command to prove or refute a finding is fine (an `rg`, a single render; one test by name only with `trust=maintainer`: `GITHUB_WORKSPACE=<root> go test -count=1 -run TestX ./pkg`), within `checks/setup.txt`. Never one that deploys, applies, or mutates state. Never run `./...`, any `make test*` target (`test`, `test-only`, `test-gh`, `test-gh-only`, `test-malina`, `test-openvino`, `test-load-*`, `test-adversarial`), or anything under `sdk/kronk/tests` or the other excluded trees in `setup.txt`; never download models or libraries. A finding that needs a model-backed run is an Open check naming the suite.
3. **Read-only, and never run the reviewed code unless `trust=maintainer`.** No tests, builds, scripts, or PoCs of the change otherwise; reading, `rg`, and the language server are fine. Rows skipped with "runs the reviewed code" are not findings and not Open checks: the aggregator notes them once. Write only your report (security may also write PoCs). If a write is refused, never retry it through Bash or any other tool: put the content in your reply and say the write was refused.
4. **Never open another run** under `.reviews/`.
5. **Tools before eyeballing.** The language tool your idiom file names (Go: `gopls` MCP), then `rg`/`ast-grep`.
6. **Repo rules are authoritative.** `AGENTS.md` (via `rules.txt` and the brief) and `.manual/chapter-20-developer-guide.md` define ownership and verification; then the repo's lint configs and the skills your idiom files name (Go: `use-modern-go`, `kronk-service-architecture`).
7. **No "by design" without proof.** Dismissing a gap as intentional or handled elsewhere needs a citation (ticket, code, contract). Otherwise report it and state the assumption.
8. **Issues only.** No praise, no "looks good", no lists of what you checked or what is fine. No problems → `No findings.` Sole exception: the spec agent's acceptance-criteria table lists every criterion, Met included.

## Gates

| Gate   | Name    | Meaning                                                                                  |
|--------|---------|------------------------------------------------------------------------------------------|
| **G4** | Stop    | Likely correctness, security, data-integrity, compile, boundary, or reliability failure. |
| **G3** | Repair  | Likely user-visible bug, operational blind spot, real test gap, or misleading contract.  |
| **G2** | Tighten | Locally safe, but avoidable risk, ambiguity, weak design, or brittleness.                |
| **G1** | Polish  | Low-risk clarity, naming, comment, or small test refinement. At most 5 per lens.         |
| **G0** | Clear   | No findings.                                                                             |

A G2+ finding needs `file:line`, Proof, why it matters, Patch direction (smallest safe fix), and Verify. Anything weaker is an Open check.

**Verify before writing.** Treat each finding as a hypothesis: re-open the cited line, confirm the Proof shows the claim, and look for what would make it wrong (stale line, out of scope for the depth, handled elsewhere, false pattern match). Drop, downgrade, or move to Open checks what doesn't hold. Set the Gate last.

## Idioms

Review each file in its own language's idiom; no language is primary. Infrastructure (Terraform/OpenTofu, Helm, K8s, Ansible, Dockerfiles, CI pipelines) is code: every lens applies to it.

- Read the idiom files your prompt lists (`<skill>/references/langs/<name>.md`) with your first batch, and `context/stack.md` when it exists. Apply the section your lens names (Correctness, Failure paths, Contracts, Tests, Security, Clarity) plus the file's **Tools** and **Checks** lines.
- A language with no idiom file: use `stack.md`'s inline idioms, else the language's established practice and its linters' rules; note "best-effort idioms: <lang>" under Open checks.
- `generated=included`: generated and vendored files are in scope. Patch direction fixes the generator or its input, never the generated file by hand.

## Report

Write to the path in your prompt:

```markdown
# <Agent> report
Gate: G<n>
Depth: <diff | files | packages | project>
Lenses: <lenses applied>
Shard: <k | all>

## Findings
### [G3] <short title>
- Lens: <lens>
- Where: `path:line`
- Proof: <evidence; cite checks/ logs or PoC paths>
- Why it matters: <impact>
- Patch direction: <smallest safe fix>
- Verify: <test, command, or review step>
- Assumption: <only when it rests on unconfirmed intent>
- Origin: pre-existing <full depth only, on lines the diff didn't touch>
<extra fields from your lens>

## Open checks
- <unproven suspicion, missing tool, intent to confirm>

<extra sections from your lens>
```

Order findings G4 → G1.

## Reply

One line only — nobody else reads your report until aggregation:

```
<agent>[#<shard>]: G<n> — G4:<a> G3:<b> G2:<c> G1:<d> — open:<e> — <report path>
```
