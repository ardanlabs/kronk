# Straight-Line Pass (behavior-preserving simplification)

Propose the smallest changes that make the in-scope code (the diff, at `diff` depth) easier to read, test, and maintain, with behavior exactly unchanged. Optimize for obvious control flow and clear names, not fewer lines; explicit beats clever.

Apply the **Clarity** section and the skills of each idiom file in your prompt (Go: `use-modern-go`, `kronk-service-architecture`) and the patterns already in that part of the repo.

## Leave alone

Don't propose behavior or public-contract changes, merging unrelated concerns, dropping domain names, cleverness, new dependencies, or edits outside the review scope. Don't flatten domain helpers, `to*`/`from*` helpers, type sets, `mid` middleware, ownership boundaries (handler → pool → handle → engine), readable test setup, or abstractions with several callers or a clear purpose — and don't report them. Don't merge the generation and sequence-batch engines, or always-rebuild/copy the Responses `model.D`: Chapter 20 forbids both.

## Patrol route

1. **Control flow.** Nested conditionals → guard clauses; branch chains the diff adds or rewrites → the idiom file's preferred form (Go: default-first assignment or a naked switch, `kronk-service-architecture` `references/branching.md`; G1, and an existing `else if` chain is never a finding); tangled success/error paths; deeply nested templates in infrastructure code.
2. **Noise.** Temp variables that hide intent, wrappers with no domain value, comments narrating the code, names that force reading the implementation. (Duplication belongs to Clone Hunter.)
3. **Dead code.** `lens=straight-line-pass` rows in `checks/` (Go `deadcode`) and dead-code lints in the linter logs. Report what the diff added or left unreachable (G2) — including unused variables, outputs, values keys, templates, or tasks — and at full depth what is already dead; a public function of a library may be API — say so instead of calling it dead.
4. **Preservation.** For each suggestion, say why observable behavior is unchanged.

## Gates

- **G4** — only for a separate correctness failure you found along the way.
- **G3** — complexity already hides a likely wrong branch or unsafe path.
- **G2** — a small refactor materially improves readability or cuts drift risk.
- **G1** — naming, comment, or layout.

## Extra fields

- Current shape: what makes it hard to read
- Why behavior is preserved
- Before/after sketch: minimal excerpts
