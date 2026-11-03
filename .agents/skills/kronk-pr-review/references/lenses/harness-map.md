# Harness Map (behavior-to-test mapping)

Would the tests fail for the bugs this diff is most likely to introduce (at full depth, the bugs most likely in the code in scope)? Judge by regression value, not coverage percentage; skip trivial accessors. Use the **Tests** section of each idiom file in your prompt.

**Tests ran only for a maintainer PR** (`trust=maintainer`); otherwise their rows say "runs the reviewed code" and you judge the tests by reading them — don't report that they didn't run. When they ran, see the `lens=harness-map` rows of `checks/summary.tsv` (`go-test*`, approved `x-*` test gates) and `checks/setup.txt`: they ran like the CI `race` job, `-race -short` on the reviewed packages only, with the model-backed trees in `setup.txt` excluded, and never at `project` depth. A failing or timed-out test is a finding when the change can cause it (at full depth, any test in a reviewed package), or an Open check when it's environmental (missing native libraries or models: say which). At `diff` depth, a failure unrelated to the change is out of scope: drop it. Don't re-run the suite; one targeted test run (maintainer PR only) to prove a point is fine. Never run anything in an excluded tree, `./...`, or `make test`.

**Kronk test layout.** Unit tests sit next to the owner (`sdk/kronk/model`, `sdk/kronk/parsers/<family>`, `sdk/pool/engine`, `cmd/server/app/domain/<x>app`). Model-backed suites live in `sdk/kronk/tests/<suite>`, `sdk/bucky/tests/transcribe`, and `cmd/server/api/services/kronk/tests`; they count as coverage when they truly exercise the behavior, but a behavior that only they cover still needs a focused unit test where one is feasible. A new required CI model belongs in `.github/test-models.txt`.

For infrastructure and pipelines, tests include policy tests, chart checks (`zarf/helm/kronk/test.sh`), and render/validate checks. When the change has no automated path to test it, say so and name the manual check.

## Patrol route

1. **Behaviors** (working notes, not reported): for each changed production symbol, resource, template, or task (at full depth, each in-scope one), its success, failure, boundary, state, and boundary-crossing behavior.
2. **Match to tests** via the language tool or `rg` in test files; integration tests count when they truly exercise the behavior.
3. **Judge the tests.** Flag ones that assert implementation details, wouldn't catch a realistic regression, are too broad, or weaken assertions, skip, or mock around the real problem.

## Gates

- **G4** — critical behavior untested (auth, cross-slot/session isolation, permit/reservation release, KV/snapshot integrity, validation, security, destructive infrastructure change), or an in-scope test fails for a code reason.
- **G3** — an important error, cancellation, or boundary path untested; streaming vs non-streaming equivalence untested for a parser change; a policy untested.
- **G2** — tests exist but are brittle, incomplete, or poorly named.
- **G1** — a small case, name, or assertion.

## Extra fields

- Untested behavior
- Regression it would catch
- Test sketch: Arrange / Act / Assert
