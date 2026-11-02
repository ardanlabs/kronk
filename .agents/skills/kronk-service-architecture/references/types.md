# Types, Edges, and Errors

Part of `kronk-service-architecture`. Its SKILL.md holds the layer diagram and import rules; this file covers the HTTP ↔ SDK edge, enums, constructors, and errors.

## Type choices (authoring only)

When writing code interactively, the user owns new type choices: request/response field types, SDK option and config fields, pointer vs value, `omitzero` vs `omitempty`, `model.D` vs a typed struct, wire spelling (follow the neighboring structs of that endpoint). Suggest an existing type first (an SDK type set, a type in the same package, a `model` option), and state the concrete types before committing. Reviewers don't gate on this; they report type problems as findings.

## HTTP edge (`cmd/server/app/domain/<x>app`)

Follow the neighboring endpoint; the shapes below describe what exists, not a template to force onto old code.

- **SDK `*HTTP` pass-through** (`chatapp`, `respapp`, `embedapp`, `rerankapp`, `tokenapp`, `decisionapp`): the handler decodes the body into `model.D` (or a typed `model.DecisionRequest` via a `toSDK` helper), checks only what routing needs (`model` present and a string), acquires the model from the pool, and passes `web.GetWriter(ctx)` to the SDK method (`ChatStreamingHTTP`, `ResponseStreamingHTTP`, `EmbeddingsHTTP`, `RerankHTTP`, `TokenizeHTTP`, `DecisionHTTP`). The SDK validates the document, writes headers and SSE, and encodes the `model.*Response`, so the OpenAI-compatible wire shape is owned in `sdk/kronk` and `sdk/kronk/model`. The handler returns `web.NewNoResponse()`, or `web.NewNoResponseError(errs.FromSDK(err))` once bytes are committed (`kronk.ErrResponseCommitted`), else `errs.FromSDK(err)`. SDK validation failures are sentinels (`model.ErrInvalidRequest`, …) mapped to `InvalidArgument`.
- **Typed endpoints** (`msgsapp`, `toolapp`, `imageapp`, `downapp`, and `playgroundapp`'s session/config routes; its chat route uses `ChatStreamingHTTP`): request/response structs live in the domain (usually `model.go`; `msgsapp` uses `models.go`). Requests are read with `web.Decode(r, &req)` (struct implements `Decode`, optionally `Validate`) or `json.NewDecoder`; validation errors become `errs.New(errs.InvalidArgument, err)`. Responses implement `web.Encoder` (`Encode() ([]byte, string, error)`). Protocol translation (Anthropic ↔ OpenAI) is a named domain helper (`toOpenAI`, `toMessagesResponse`).
- **Multipart uploads** (`audioapp`, `imageapp` edits): `r.ParseMultipartForm` with an explicit byte cap constant, and any decoded-size limit (pixels, duration) enforced before unbounded work starts.
- Typed responses may hold SDK type sets (`vram.FitStatus`, `model.KVCacheMode`): their `MarshalText` is the wire form. They must not hold handles, pools, channels, or mutex-carrying structs. Where a domain converts SDK values (mostly `toolapp`, `downapp`, `msgsapp`, `decisionapp`), it uses a named helper per SDK type (`toApp<Thing>`, `to<Thing>Response`, `from<Pkg><Thing>`); reuse an existing helper rather than rebuilding the same value inline.

## JSON: `encoding/json` v1 only (MUST)

HTTP, SSE, and exported SDK wire types use `encoding/json` (v1). Don't import `encoding/json/v2` or `encoding/json/jsontext` for them, and don't migrate existing code, even where `use-modern-go` recommends v2 for new code. Under Go 1.27, v2's defaults change Kronk's wire behavior:

- **Invalid UTF-8 fails to encode.** A streamed token piece that splits a multi-byte character errors instead of encoding as U+FFFD, aborting the stream.
- **Numbers in `model.D` lose precision.** Decoding into `any` yields `float64`; `chatapp` relies on v1's `UseNumber` to keep values like seeds exact.
- **Field presence changes.** Nil slices encode as `[]` instead of `null`, and `omitempty` no longer omits `0`/`false`.
- **Requests that v1 accepted fail or lose data.** Field names match case-sensitively and duplicate keys are rejected.
- **Bytes change.** Map keys are no longer sorted (tool-call `arguments` built from maps vary between runs), and `<`, `>`, `&` are no longer escaped.
- **`time.Duration` has no default encoding** and errors.

**Tags on wire types:** keep `omitempty` on existing fields and on slices, maps, strings, and numbers, and never rewrite existing tags. On v1, `omitzero` emits an empty non-nil slice or map as `[]`/`{}` where `omitempty` drops it, which changes OpenAI-compatible output. Use `omitzero` only on struct and type-set fields (which `omitempty` never omits), as Bill's code does.

Moving any wire type to v2 needs explicit approval, wire-level tests for each change above, and a `BREAKING_CHANGES.md` entry. Internal JSON that never reaches a client or exported type is the author's call.

## Type sets (enums)

New exported enumerations in `sdk/*` are type sets, normally in `<topic>_types.go` (`sdk/kronk/vram/fit_types.go` is the reference shape). Older and unexported sets (`draftKind`, `pullKind`, probe states) and a few exceptions (`BundleName` in `bundles.go`, `ComputeKind` with JSON-only methods) predate or deliberately differ from this; don't report them unless the diff touches them.

```go
// FitStatus describes how a memory requirement compares with capacity.
// Its zero value is unset, distinct from FitStatusUnknown.
type FitStatus struct{ value string }

var fitStatuses = make(map[string]FitStatus)

var (
	// FitStatusFits indicates comfortable headroom.
	FitStatusFits = newFitStatus("fits")
	// ...
)

func newFitStatus(value string) FitStatus {
	t := FitStatus{value: value}
	fitStatuses[value] = t
	return t
}

func (fs FitStatus) String() string                  { return fs.value }
func (fs FitStatus) Equal(other FitStatus) bool      { return fs == other }
func (fs FitStatus) IsZero() bool                    { return fs.value == "" }
func (fs FitStatus) MarshalText() ([]byte, error)    { return []byte(fs.value), nil }
func (fs *FitStatus) UnmarshalText(data []byte) error { /* ParseFitStatus, assign */ }

func ParseFitStatus(value string) (FitStatus, error) { /* registry lookup, error on unknown */ }
func MustParseFitStatus(value string) FitStatus      { /* panic on error */ }
```

- Struct-wrapped unexported value, so callers can't build an unregistered value. Registry populated only by the unexported `new<T>`.
- Doc comment on the type states what the zero value means (unset vs a named "unknown" member). `IsZero` must agree with it.
- Numeric wire forms (e.g. `bucky/model.EventKind`) keep `MarshalText` for logs and add `MarshalJSON`/`UnmarshalJSON` for the number.
- Value receivers named by the type's initials (`fs`, `bim`, `ek`); pointer receivers only where the method must mutate (`Unmarshal*`).
- `Parse<T>` is the constructor from external input; `MustParse<T>` for known-good literals (tests, package vars, fixed tables). No `New<T>`/`MustNew<T>` for type sets.
- Prefer `Equal`, `==`, or a switch on the exported members over comparing `String()` to a literal in new code.
- Add the new set to the package's `registered_types_test.go` when one exists (`sdk/kronk/model`, `sdk/tools/models`).

## Constructors and options

- Public handles: `New(opts ...model.Option)` and `NewWithContext(ctx, opts...)`; process-wide init: `Init(opts ...InitOption)` with `With*` options. Add a field to the owning `Config` plus a `With*` option rather than a new constructor parameter.
- Pointers signal tri-state optional overrides where the zero value is meaningful (`Steps *int`, `*model.SplitMode`). Use them only then, and build them with `new(v)` (Go 1.26), not `x := v; &x`. Otherwise use a plain value (see the tag rule below for `omitempty` vs `omitzero`).
- Type aliases: public re-exports only (`sdk/pool`, `sdk/bucky`, `sdk/malina`, `sdk/kronk`, `sdk/tools/*`, `model/decision_types.go`). #1111 removed the internal-convenience ones; don't add new ones.

## Errors (MUST)

- The owning package declares sentinels: `var ErrX = errors.New("...")`, documented. Callers match with `errors.Is` or `errors.AsType[T]`, never by message.
- Wrap with context and `%w` at each layer (`fmt.Errorf("chat-streaming-http: %w", err)`). A second cause is joined with a second `%w`, not flattened with `%v`.
- At the HTTP edge return `errs.FromSDK(err)`. A sentinel a client can trigger needs its case in `errs.FromSDK` (`cmd/server/app/sdk/errs/errs.go`), mapped to the right code: validation → `InvalidArgument`, missing model → `NotFound`, admission timeout / no capacity → `ResourceExhausted` / `Unavailable`, `context.Canceled` / `DeadlineExceeded` preserved. An unmapped sentinel becomes a 500.
- Capacity, cancellation, validation, and internal failures must stay distinguishable from the engine up to `errs.FromSDK` ([§20.5.1](../../../../.manual/chapter-20-developer-guide.md#2051-server-request-flow)).

## Conformance checklist

- [ ] The endpoint follows its neighbor: SDK `*HTTP` pass-through, typed struct + `Validate` + `web.Encoder`, or capped multipart.
- [ ] OpenAI-compatible wire changes are made in the SDK response types, not patched in the handler.
- [ ] New exported enums are type sets with the full method set, `Parse`/`MustParse`, documented zero value, initials receivers, and a `registered_types_test.go` entry where that file exists.
- [ ] New options go through `Config` + `With*`; pointers only for tri-state overrides, built with `new(v)`.
- [ ] No new internal type alias.
- [ ] Wire types use `encoding/json` v1; no `encoding/json/v2` or `jsontext` import on an HTTP, SSE, or exported SDK path.
- [ ] New client-reachable sentinels are mapped in `errs.FromSDK`; errors wrapped with `%w` and matched with `errors.Is`/`errors.AsType`.
