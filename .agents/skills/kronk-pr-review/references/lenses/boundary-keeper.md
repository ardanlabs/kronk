# Boundary Keeper (types and contracts)

Review new or changed data shapes and the contracts Kronk exposes (at full depth, every data shape and contract in scope): do they make valid states easy, invalid states hard, and crossings explicit? Start from the `type`, `New*`, `Parse*`, and `to*`/`from*` rows in `symbols.tsv` and the `layer` column in `files.tsv` (`review-set.tsv` at full depth). Apply `kronk-service-architecture` (`SKILL.md` invariants and `references/types.md`).

Kronk's contracts, in order of blast radius:

1. **HTTP wire**: OpenAI-, Anthropic-, and Responses-compatible request/response shapes, SSE stream format, status codes from `errs.FromSDK`. External clients (OpenCode, OpenWebUI, LangChain, the BUI) depend on them. For pass-through endpoints the shape lives in `sdk/kronk` (`*HTTP` methods and `model.*Response`), not in the `app` layer.
2. **Public SDK** (`sdk-*` layers, consumed by `examples/` and external Go users): exported types, functions, options, sentinels, and `Config` fields. Observable changes usually get a `BREAKING_CHANGES.md` entry under the next version; the author decides.
3. **Config and catalog**: server config/env vars (`cmd/server/api/services/kronk/config.go`), model config and catalog YAML (`zarf/kms/`, `sdk/tools/defaults/`), CLI flags. Renames and default changes break deployments silently.
4. **proto** (`authapp.proto`): field numbers and compatibility; `*.pb.go` regenerated in the same change.

Mechanical evidence in `checks/`: `apidiff` (incompatible exported Go API changes against the base commit). An `apidiff` hit on a `sdk/*` package with a caller left un-updated is G3; with no `BREAKING_CHANGES.md` entry it is G2 with `Assumption: author may intend no entry`. Cite the log line.

## Language lens

- **Go**: exported vs unexported fields, constructors and options (`New`/`NewWithContext` + `With*`), zero-value meaning, value vs pointer receivers, type sets (`Parse`/`MustParse`, full method set, initials receivers), small consumer-owned interfaces, needless interfaces/generics, new internal type aliases, helper naming in `model.go`, sentinels and their `errs.FromSDK` mapping.
- **React/TS (BUI)**: `src/types` and API-client shapes match the Go JSON tags exactly (spelling, optionality, numeric vs string enums); props required vs optional; no internal state leaked.
- **proto**: field numbers stable, reserved fields, optional semantics.
- **YAML config**: keys match the Go struct tags that read them; defaults match the documented ones.

## Patrol route

1. **Per in-scope type** (working notes, not reported): layer, exported surface, valid states, how it's built, mutated, and serialized.
2. **Invariants.** Can invalid values be built directly (a type set built without `Parse`, a struct whose zero value is unsafe)? Validated once, at the owner? Is the type simpler than the bug it prevents?
3. **HTTP edge.** Decode shape matches the neighboring endpoint (SDK `*HTTP` pass-through, typed struct, capped multipart); typed endpoints convert SDK values through the domain's existing helpers; JSON tag spelling and `omitzero`/`omitempty` consistent with the endpoint; a new client-reachable sentinel missing from `errs.FromSDK` (surfaces as 500); OpenAI/Anthropic field semantics changed without the ticket asking; an `encoding/json/v2` or `jsontext` import on an HTTP, SSE, or exported SDK path (G3: `kronk-service-architecture` keeps wire types on v1).
4. **Ownership and imports.** `sdk` importing `cmd`, `foundation` importing `app`/`sdk`, one `<x>app` importing another (except `authapp`), `examples/` importing `cmd` or `internal`, policy (inference, caching, eviction, validation the SDK owns) re-implemented in a handler or CLI command.
5. **Public SDK.** Removed/renamed exports, changed signatures, new required parameters instead of a `Config` field + `With*`, changed defaults, changed JSON of exported structs; each needs every in-repo caller (including `examples/`) updated, and usually a `BREAKING_CHANGES.md` entry.

## Gates

- **G4**: wire-compatibility break for existing clients, constructible invalid state that reaches native code, import-direction violation, or a design that can corrupt cached/session state.
- **G3**: public SDK break with callers left broken; unmapped client-reachable sentinel; exported fields or a missing constructor allowing real invariant breakage; config key renamed without migration.
- **G2**: invariants unclear or scattered; helper bypassed; inconsistent tag spelling within an endpoint.
- **G1**: naming or doc around type intent.

Prefer compile-time guarantees, weigh each suggestion's cost; a simpler type with fewer guarantees often wins.

## Extra fields

- Type: `<Name>` (layer)
- Contract: HTTP / SDK / config / proto / none
