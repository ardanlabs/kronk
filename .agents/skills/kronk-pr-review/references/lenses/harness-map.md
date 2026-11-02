# Harness Map (behavior-to-test mapping)

Would the tests fail for the bugs this diff is most likely to introduce (at full depth, the bugs most likely in the code in scope)? Judge by regression value, not coverage percentage; skip trivial accessors.

**Tests already ran** — see `checks/summary.tsv`, the `go-test*` logs, and `checks/setup.txt`. They ran like the CI `race` job: `-race -short`, the reviewed packages only, with the model-backed trees in `setup.txt` excluded; at `project` depth no tests run (`go-test` is `skipped`). A failing or timed-out test is a finding when the change can cause it (at full depth, any test in a reviewed package), or an Open check when it's environmental (missing native libraries or models: say which). At `diff` depth, a failure unrelated to the change is out of scope: drop it. A `WARNING: DATA RACE` whose stack touches in-scope code is G4 — quote the two conflicting stack frames. Don't re-run the suite; one `GITHUB_WORKSPACE=<root> go test -count=1 -run TestX ./pkg` to prove a point is fine. Never run anything in an excluded tree, `./...`, or `make test`.

**Kronk test layout.** Unit tests sit next to the owner (`sdk/kronk/model`, `sdk/kronk/parsers/<family>`, `sdk/pool/engine`, `cmd/server/app/domain/<x>app`). Model-backed suites live in `sdk/kronk/tests/<suite>`, `sdk/bucky/tests/transcribe`, and `cmd/server/api/services/kronk/tests`; they count as coverage when they truly exercise the behavior, but a behavior that only they cover still needs a focused unit test where one is feasible. A new required CI model belongs in `.github/test-models.txt`.

## Language lens

- **Go** — table-driven tests with named subtests; `t.Context()`; `errors.Is`/`errors.AsType` on error paths; negative and validation cases; boundaries; cancellation and timeouts (permit and slot released, stream closed); failed load or unload (reservation released or retained as specified); concurrent slots or acquisitions (isolation, coalescing); HTTP request/response through the domain handler including malformed input; `to*`/`from*` helpers; type-set `Parse` round-trips; parsers fed fragmented as well as whole input; Responses normalization in-place mutation.
- **React/TS (BUI)** — no test runner exists; `bui-tsc` is the automated check. Name the manual check (page, action, expected result).
- **Rego / proto / Helm** — policy allow/deny cases, codegen in sync, `zarf/helm/kronk/test.sh`.
- No automated path to test a change? Say so and name the manual check.

## Patrol route

1. **Behaviors** (working notes, not reported): for each changed production symbol (at full depth, each in-scope one), its success, failure, boundary, state, and layer-crossing behavior.
2. **Match to tests** via `gopls` references or `rg` in test files; integration tests count when they truly exercise the behavior.
3. **Judge the tests.** Flag ones that assert implementation details, wouldn't catch a realistic regression, are too broad, or weaken assertions, skip, or mock around the real problem.

## Gates

- **G4** — critical behavior untested (auth, cross-slot/session isolation, permit/reservation release, KV/snapshot integrity, validation, security), or an in-scope test fails for a code reason.
- **G3** — an important error, cancellation, or boundary path untested; streaming vs non-streaming equivalence untested for a parser change.
- **G2** — tests exist but are brittle, incomplete, or poorly named.
- **G1** — a small case, name, or assertion.

## Extra fields

- Untested behavior
- Regression it would catch
- Test sketch: Arrange / Act / Assert
