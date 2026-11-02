# Clone Hunter (duplicate code)

Duplicated logic is a defect: copies drift, and a fix to one silently skips the others. Report each clone with every site and a single-source-of-truth refactor, and recommend it strongly — surviving duplication is a merge risk, not a nit.

## Classes (gate each separately)

1. **Intra-domain** — same package or module. Highest priority; nothing stops unifying it.
2. **Cross-domain** — across packages or modules. Extract to a shared layer, or require a stated reason to diverge.
3. **Test ↔ target** — a test re-implementing the code it tests (same algorithm for the expected value, copied formatting or validation). It passes because it mirrors the code. Assert against fixtures or call the real function.

## Detection, in order

1. **Detector output** — `dupl*` and `jscpd` rows in `checks/`; keep clones with at least one in-scope site.
2. **gopls / LSP** — `go_search`, `go_symbol_references` for repeated call shapes and twin helpers.
3. **Targeted search** — `rg`/`ast-grep` for the in-scope code's distinctive literals and call patterns, repo-wide.
4. **Inspection** — only when nothing above works for the language; say it's best-effort.

Name the method on each finding.

Kronk deliberately mirrors structure per backend (`sdk/{kronk,bucky,malina}/pool`, `sdk/tools/{libs,bucky/libs,malina/libs}`, the matching `toolapp` files). Similar shape there is expected; report it only when logic that belongs in the shared owner (`sdk/pool/engine`, `sdk/tools/backend`, `sdk/tools/downloader`) is copied, or when the copies have drifted.

## Scope

At `diff` depth, at least one site must be changed code. Report a clone that predates the change only when the diff adds a third copy or edits one of a pair. At full depth, report every clone with a site in the review set. Ignore boilerplate the framework forces.

## Gates

- **G4** — copies have already drifted, or a security/correctness rule (validation, authz, resource accounting) exists in more than one place.
- **G3** — intra-domain copy of non-trivial logic; a test that re-implements its target.
- **G2** — cross-domain copy with no reason to diverge.
- **G1** — small repeated snippet.

## Extra fields

- Class: intra-domain / cross-domain / test↔target
- Sites: every copy as `path:line`
- Detected by: method
- Drift risk: what breaks if only one copy is fixed
- Refactor: the single source of truth
