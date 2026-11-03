# Protocol Buffers idioms

**Tools:** `rg`. **Checks:** `buf-lint`, `buf-breaking` (against the base commit; with `buf.yaml`; for an untrusted change with buf's default rules, since `buf.yaml` can load plugins). A `buf-breaking` hit the ticket doesn't call for is G3, G4 for a wire break on a published proto (Boundary Keeper).

## Correctness / Contracts
Kronk's proto is `authapp.proto`; `*.pb.go` is regenerated in the same change. Field numbers stable; removed fields `reserved` (number and name); `optional` semantics; enum zero values; back/forward compatibility for every consumer; package and `go_package`/options changes that break codegen.

## Tests
Contract tests or codegen checks in CI.
