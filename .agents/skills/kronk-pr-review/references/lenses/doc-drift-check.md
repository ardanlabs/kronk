# Doc Drift Check (comment truthfulness)

Treat each in-scope comment, doc string, README/manual section (`kind=doc`, mostly `.manual/chapter-*.md`), `BREAKING_CHANGES.md` entry, and TODO/FIXME as a contract (at `diff` depth: the changed ones, plus unchanged ones the change makes false) and check the code honors it. A comment earns its place by explaining intent, constraints, or domain rules the code doesn't make obvious.

Flag comments that contradict the code, promise behavior it lacks, omit an important caveat on an exported API, narrate obvious code, carry outdated assumptions, or freeze temporary context as design. In in-scope docs, check commands, flags, config keys, env vars, defaults, and examples against the code, and every `§`/anchor link against the target heading.

**Drift the diff causes elsewhere.** Exported `sdk/*` doc comments feed generated SDK docs, and the manual is the user's reference. When the diff changes a flag, config key, default, endpoint, or exported behavior, search `.manual/` (`rg`) for the old value and report chapters left stale (unchanged file, cite the line). When it changes `.manual/`, exported Go docs, `examples/`, or Cobra `Long`/`Example`, check `skipped.tsv` for the regenerated output; missing → G2. An observable public SDK change without a `BREAKING_CHANGES.md` entry → G2, stated as an assumption for the author to confirm. The generated section of `Layout.tsx` (between the docs tool's markers) follows the same rule.

## Language lens

- The **Clarity** section of each idiom file in your prompt (doc conventions such as Go's exported-identifier comments, chart READMEs).
- **Markdown (`.manual`)** — each chapter's own section index matches its headings; Chapter 20 stays orientation (no per-function narration); the `AGENTS.md` manual index matches chapter titles; user-facing chapters follow the `AGENTS.md` Documentation Rules (semantics and exact commands, not internals).
- **All** — comments describe behavior, errors, side effects, ordering, invariants; examples still compile or render against the current API; TODOs are still true and actionable.

## Patrol route

1. **Claims** (working notes, not reported): what each in-scope comment asserts — inputs, outputs, errors, side effects, ordering, concurrency, invariants, ownership.
2. **Match** each claim against the code it describes, changed or not.
3. **Decide**: rewrite stale or incomplete claims, remove narration. Accurate comments aren't reported.

## Gates

- **G4** — a misleading comment could cause unsafe use, data or security mistakes, or a wrong operational action.
- **G3** — an exported-API or domain comment contradicts behavior.
- **G2** — non-obvious behavior or an invariant lacks needed context.
- **G1** — redundant or wordy.

## Extra fields

- Current claim
- Suggested wording (when useful)
