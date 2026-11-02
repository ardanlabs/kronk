# Error Tripwire (failure-path visibility)

Follow every changed failure path to its end and ask: when this fails, who finds out, and is that enough to debug it? Catch zero values returned after errors, log-and-continue, and success reported after partial failure. Vet, linter, and `errcheck` output in `checks/` flags unchecked errors mechanically — start there, then go further. An `errcheck` hit on a changed line is a finding only when the lost error matters (a failed write, commit, or close of a written file); a dropped `Close` on a read-only body is G1 at most.

## Idioms

- **Go** — errors checked, wrapped with `%w`, matched with `errors.Is/As`, not `_`-discarded when they matter; `defer` cleanup failures handled; goroutines can report failure.
- **React/TS** — promises and `await` caught; non-2xx and SSE error events handled; `catch` doesn't swallow; the UI shows failure instead of empty or stale state.
- **Rego** — fails closed; a missing `input` field never grants access.
- **Shell/PowerShell** — `set -euo pipefail` / `$ErrorActionPreference = 'Stop'`; exit codes and pipe failures checked.

## Patrol route (Go; same steps elsewhere)

1. **Every `err`.** Checked? Discarded? Overwritten before use? Zero/default returned after failure? Success reported despite partial failure?
2. **Propagation.** Cause kept with `%w` where callers need it; sentinels matched with `errors.Is/As`; storage or transport details not leaking upward by accident.
3. **Continuation.** Log-and-continue where it should stop; defaults masking broken config, missing libraries or models, network, or validation; retries that exhaust silently; goroutines with no error path; `defer` cleanup whose failure matters.
4. **Boundaries.** Native calls (yzma/llama.cpp, whisper, stable-diffusion return codes and nil handles), HTTP handlers and `errs.FromSDK` mapping (a sentinel lost to `%v` or a missing case becomes a 500), streams after `ErrResponseCommitted`, `SessionStore` prepare/commit failures, download and integrity checks in `sdk/tools`, context cancellation and timeouts.
5. **Kronk failure contracts** (`kronk-service-architecture` `references/lifecycle.md`): snapshot publication failure invalidates current state; failed load releases its reservation; failed unload retains it; System-cache retention failure never fails inference; MTP rollback/sync failure fails the slot or disables MTP. A change that swaps one of these for another outcome is a finding.

## Gates

- **G4** — a swallowed error yields false success, a published partial snapshot, leaked permit/reservation, skipped validation, or a corrupted session.
- **G3** — cause lost, hidden fallback, unobserved goroutine failure, silent retry exhaustion.
- **G2** — surfaced, but without useful context.
- **G1** — clearer wording or wrapping, same behavior.

## Extra fields

- Hidden failure: what disappears
- Impact: on caller, user, or operator
