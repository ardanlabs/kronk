---
name: kronk-service-architecture
description: Kronk Go architecture rules. Ownership along the request flow (foundation/web and mid → app/domain/*app → sdk/pool → typed pools → sdk/{kronk,bucky,malina} handles → model engines → yzma/llama.cpp) and import direction, with encoding/json v1 for wire types, SDK sentinel errors mapped in errs.FromSDK, type-set enums (Parse/MustParse, *_types.go), cross-cutting concerns in mid/observ, the lifecycle invariants (admission permits, cancellation, slot/sequence isolation, IMC, MTP, pool reservations), and shallow-branching style (default-first assignment, naked switch). Use when writing, editing, or auditing Go under cmd/kronk, cmd/server, sdk, or examples, when adding an endpoint, SDK option, error, enum, metric, span, parser, or pool/engine behavior.
---

# Kronk Service Architecture

The diagram is the request flow, not import direction (see the import rule below). Every arrow is an ownership boundary ([Chapter 20 §20.5.1](../../../.manual/chapter-20-developer-guide.md#2051-server-request-flow)); the owner of a concern is the only place it is implemented.

```
cmd/kronk (CLI)  ─┐                            examples/ (separate module, public SDK only)
                  ▼                                     │
cmd/server/foundation/{web,logger}   transport, request lifecycle, logging primitives
cmd/server/app/sdk/{mid,errs,mux,…}  middleware, error translation (errs.FromSDK), auth, mux
cmd/server/app/domain/<x>app         routes, access, model lookup, error mapping; typed endpoints
        │                              also own request/response structs (inference: SDK *HTTP writes)
        ▼
sdk/pool  ──►  sdk/{kronk,bucky,malina}/pool  ──►  sdk/pool/engine (+ resman reservations)
        ▼
sdk/kronk · sdk/bucky · sdk/malina   one-model public handles: admission, active use, unload
        ▼
sdk/kronk/model (batch engine, IMC, MTP) · sdk/bucky/model · sdk/malina/model   native state
        ▼
yzma / llama.cpp · whisper.cpp · stable-diffusion.cpp
sdk/tools/*   catalogs, downloads, libs, devices: shared by CLI and toolapp
```

## Which reference to read (MUST)

`<skill>` = this file's dir. Read the matching reference before writing or auditing the code it covers. This file holds only the invariants.

| Task                                                                                                              | Read                              |
|-------------------------------------------------------------------------------------------------------------------|-----------------------------------|
| HTTP edge (SDK `*HTTP` pass-through vs typed endpoints), enums (`*_types.go`), options, SDK errors and their HTTP mapping | `<skill>/references/types.md`     |
| Handles, pools, engines, streams, IMC, prompt plans, parsers, Responses normalization, speculative/MTP, tracing, Bucky | `<skill>/references/lifecycle.md` |
| Conditional logic in any Go code: 1-3 branches, nested if/else, type assertions on `any`                          | `<skill>/references/branching.md` |

Use `use-modern-go` for syntax in all of them. Source-of-truth order is [Chapter 20 §20.1.1](../../../.manual/chapter-20-developer-guide.md#2011-source-of-truth-hierarchy)'s: scoped `AGENTS.md`, then current code and tests, then Makefiles and workflows, then Chapter 20. This skill ranks below all of them; when it disagrees with one, follow that source and say so.

## Invariants (MUST, always)

Apply these to new and changed code. Existing code that predates a rule is not a finding by itself; follow the neighboring code when a rule and the codebase disagree, and say so.

- **Import direction.** `sdk/*` never imports `cmd/*`. `cmd/server/foundation/*` imports neither `cmd/server/app/*` nor `sdk/*`. A `<x>app` package never imports another `<y>app`. Allowed exceptions: the generated `authapp` gRPC/protobuf contract (imported by `toolapp`, `app/sdk/authclient`, `app/sdk/apitest`), and `cmd/kronk` importing `toolapp` request/response types for web mode. `examples/` imports only public SDK packages, never `cmd/*` or `internal/`.
- **Change the owner, don't duplicate policy.** Inference, caching, scheduling, eviction, and resource policy live in `sdk/*`; transport concerns live in `foundation/web` and `mid`. A handler never touches native model state, pool eviction, or KV; the CLI never re-implements a `sdk/tools` manager.
- **Who owns the HTTP shape.** OpenAI-compatible inference endpoints (`chatapp`, `respapp`, `embedapp`, `rerankapp`, `tokenapp`, `decisionapp`) hand the `ResponseWriter` to an SDK `*HTTP` method (`ChatStreamingHTTP`, `ResponseStreamingHTTP`, `EmbeddingsHTTP`, `RerankHTTP`, `TokenizeHTTP`, `DecisionHTTP`) that writes headers, SSE, and the `model.*Response` body; their wire shape is owned in `sdk/kronk`. The domain owns routing, access middleware, model lookup, and error mapping. Other endpoints (`msgsapp`, `toolapp`, `imageapp`, `audioapp`, …) own their request/response structs in the domain. See `references/types.md`.
- **Wire types stay on `encoding/json` v1.** Request decoding and response encoding for HTTP, SSE, and exported SDK types use `encoding/json`, never `encoding/json/v2` or `jsontext`, whatever `use-modern-go` suggests. v2 changes Kronk's OpenAI-compatible output and can abort streams (`references/types.md`). Switching needs explicit approval and wire-level tests.
- **Error identity survives to the edge.** The SDK returns wrapped sentinels (`%w`); handlers return `errs.FromSDK(err)`, or `errs.New`/`errs.Errorf` with an explicit code for protocol errors. A new client-reachable SDK sentinel needs a case in `errs.FromSDK` (otherwise it surfaces as `Internal`).
- **Cross-cutting concerns are not inlined into handlers.** HTTP-wide: a `mid` `web.MidFunc` (global in `mux.WebAPI`, or per route). SDK: spans through `sdk/kronk/observ/otel` (`otel.AddSpan`) and metrics on the private registry in `sdk/kronk/observ/metrics` (`promauto.With(reg)`, never the default registerer).
- **New exported enums are type sets** (`references/types.md`): struct-wrapped value, registry, `Parse<T>`/`MustParse<T>`, `String`/`Equal`/`IsZero`/`MarshalText`/`UnmarshalText`, in `<topic>_types.go`, covered by the package's `registered_types_test.go` where one exists.
- **Lifecycles are owned** (`references/lifecycle.md`): whoever creates a goroutine, stream, native object, permit, or reservation releases it on every return path, including cancellation and error.
- **Generated outputs are never hand-edited**: `DocsManual.tsx`, `DocsSDK{Kronk,Model,Pool,Bucky,BuckyModel,Malina,MalinaModel,Examples}.tsx`, `cli-reference.json`, the generated section of `Layout.tsx`, `cmd/server/api/services/kronk/static/**`, `*.pb.go`, `zarf/nix/gomod2nix.toml`. Change the source (`.manual/`, Go doc comments, `examples/`, Cobra `Long`/`Example`, BUI source) and regenerate (`make kronk-docs`, `npm run build`).
- **Native versions move together.** Yzma in `go.mod`, the pinned llama.cpp in `sdk/tools/libs`, the root and `examples` modules, and Nix data are one tested set ([§20.4.1](../../../.manual/chapter-20-developer-guide.md#2041-native-library-compatibility-and-sdk-initialization)).
- **Public SDK changes are announced.** An observable change to an exported `sdk/*` API, wire shape, or config usually gets an entry in `BREAKING_CHANGES.md` under the next version. When unsure whether one is needed, ask the author rather than assume.
- **Tests follow `AGENTS.md`**, whatever `use-modern-go` suggests: set `GITHUB_WORKSPACE`, test only the changed packages by path, never `./...` and never anything under `sdk/kronk/tests`.
- **When authoring interactively, the user controls the types.** Before committing new request/response fields, SDK options, or config fields, state the concrete types and get confirmation. Reviewers don't block on this; they report type problems as findings.
