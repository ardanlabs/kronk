# Rego idioms

**Tools:** `rg`; `opa`/`conftest` if installed (`opa test` on the changed package).

## Correctness / Failure paths
Kronk's policies are the auth Rego in `cmd/server/app/sdk/security/auth/rego`. Default deny; fails closed — a missing or malformed `input` field never grants access; `input` shape assumptions; unintended allows through `some`/iteration or partial rules; `else` chains.

## Tests
`opa test` with allow and deny cases for each changed rule, including missing fields.

## Security
Every rule change is an authorization change: who gains access, and is it intended per the ticket?
