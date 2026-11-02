# Service Steward (general correctness)

Report what affects whether the change should merge, not preferences. Test: would a careful reviewer block or request changes? If not, it's G1 at most.

Priority: correctness, explicit `AGENTS.md` rules, Kronk ownership and lifecycle invariants, Go idioms, operational safety, local maintainability.

For `.go` files apply `use-modern-go` and `kronk-service-architecture`. When in-scope code touches handles, pools, engines, IMC, parsers, Responses normalization, speculative/MTP, tracing, or Bucky, read that section of `kronk-service-architecture` `references/lifecycle.md` and check its list against the in-scope code.

A failing `go-build`, `go-vet`, or analyzer row in `checks/` on in-scope code is a finding; cite the log line. These rows mirror the Linux CI `static` job, so a hit there fails CI.

Analyzer rows in `checks/` (in-scope code only):

- `staticcheck` (run with `-checks=all`, as CI does): each hit on in-scope code is a finding at the Gate its effect warrants; style-only checks (`ST*`) are G1 unless CI would fail, then G2.
- `checklocks`: a violation of an existing `+checklocks`/`+checkatomic` annotation is a data race (G4). "may require checklocks annotation for X" is not a finding by itself: it names the lock that guards those fields, so verify the in-scope code holds it on every access.
- `nilaway`: candidate nil dereferences. Confirm the nil flow can happen before reporting; unconfirmed ones go to Open checks.
- `go-fix`, `gofmt`, `go-mod-tidy`: CI fails on any diff, so a hit on in-scope files is G2.
- `apidiff`: incompatible exported-API changes: confirm every caller in the repo (including `examples/`) was updated (step 1); an unannounced break of a `sdk/*` package is G3.
- `bui-tsc`: a type error in changed BUI code is G4 (the BUI build and server embed fail).
- `helm-test`: a failing chart check on a changed chart is G3.

## Patrol route

1. **Surface and contract.** Broken signatures, changed exported APIs not updated at call sites (callers are in `brief.md`), surprising zero values, `context` misuse (stored, not propagated, cancellation ignored).
2. **Runtime correctness.** Nil derefs, off-by-one and slice/map bounds, data races, leaked goroutines, streams, or native objects; admission permits or active-use counts not released on an error or cancellation path; reservations not released after a failed load; slot/sequence state leaking to the next job; KV or session metadata updated without its snapshot (and the reverse).
3. **Ownership.** The change is in the owning package (Chapter 20 §20.2/§20.3); no inference, caching, eviction, or validation policy duplicated in a handler, CLI command, or facade; cross-cutting concerns in `mid`/`observ`, not inlined; shallow branching.
4. **Repo rules.** Generated outputs (listed in `skipped.tsv`) regenerated when their source changed (`.manual/` → `DocsManual.tsx`; exported Go docs → `DocsSDK*.tsx`; `examples/` → `DocsSDKExamples.tsx`; Cobra `Long`/`Example` → `cli-reference.json`; BUI source → `static/assets`); a source change with no matching generated row is G2. Yzma, `sdk/tools/libs` pins, root and `examples` `go.mod`, and `gomod2nix.toml` move together. `.go-version` and `go.mod` agree on major.minor.
5. **Operational safety.** User content in logs without insecure-logging mode; unbounded spans/labels; new env/config keys without defaults or docs; upload or decode limits removed.
6. **Other languages**, same intent in their idiom: React/TS hook dependencies, effect cleanup, unhandled async, a11y, `src/components/AGENTS.md`; Rego default deny; proto compatibility; Helm contract (single replica, retained PVC, authenticated defaults, chart `version` bump); Dockerfile vs `docker.yml` matrix; shell `set -euo pipefail`, quoting, exit codes.

## Gates

- **G4**: compile or BUI build break, data race, security/authz hole, cross-slot or cross-session state leak, native resource or permit leak on a reachable path, explicit `AGENTS.md` violation.
- **G3**: likely production bug: nil/error mishandling, leaked goroutine or stream, cancellation not propagated, reservation kept after failure, CI static job would fail on a non-format check.
- **G2**: real convention or maintainability problem in scope; generated output not regenerated; `gofmt`/`go fix`/tidy diff.
- **G1**: small naming or idiom cleanup.

## Extra fields

- Rule: the `AGENTS.md` rule, Chapter 20 section, skill, or Go convention involved
