# Error Tripwire (failure-path visibility)

Follow every in-scope failure path (every changed one, at `diff` depth) to its end and ask: when this fails, who finds out, and is that enough to debug it? Catch defaults returned after errors, log-and-continue, success reported after partial failure, and configuration that fails open. Use the **Failure paths** section of each idiom file in your prompt; `lens=error-tripwire` rows in `checks/` flag unchecked errors mechanically — start there, then go further.

## Patrol route

1. **Every failure.** Checked? Discarded? Overwritten before use? A default returned after failure? Success reported despite partial failure?
2. **Propagation.** Cause kept where callers need it; typed errors and sentinels matched with `errors.Is/As`, not string-compared; storage or transport details not leaking upward by accident.
3. **Continuation.** Log-and-continue where it should stop; defaults masking broken config, missing libraries or models, network, or validation; retries that exhaust silently; background work with no error path; cleanup whose failure matters; `continue-on-error`, `|| true` hiding real failures.
4. **Boundaries.** Native calls (yzma/llama.cpp, whisper, stable-diffusion return codes and nil handles), HTTP handlers and `errs.FromSDK` mapping (a sentinel lost to `%v` or a missing case becomes a 500), streams after `ErrResponseCommitted`, `SessionStore` prepare/commit failures, download and integrity checks in `sdk/tools`, context cancellation and timeouts, deploy-time validation (probes, `required`).
5. **Kronk failure contracts** (`kronk-service-architecture` `references/lifecycle.md`): snapshot publication failure invalidates current state; failed load releases its reservation; failed unload retains it; System-cache retention failure never fails inference; MTP rollback/sync failure fails the slot or disables MTP. A change that swaps one of these for another outcome is a finding.

## Gates

- **G4** — a swallowed error yields false success, a published partial snapshot, leaked permit/reservation, skipped validation, a corrupted session, or a deploy that reports success while broken.
- **G3** — cause lost, hidden fallback, unobserved goroutine failure, silent retry exhaustion.
- **G2** — surfaced, but without useful context.
- **G1** — clearer wording or wrapping, same behavior.

## Extra fields

- Hidden failure: what disappears
- Impact: on caller, user, or operator
