Review and implement the proposed Bucky and whisper.cpp upgrade for Kronk as
one complete change. Do not assume a dependency bump is sufficient. Inspect
upstream source and Kronk's actual use, make every required integration and
documentation change, regenerate derived artifacts, and run focused
verification. Ask for direction only if an unresolved incompatibility or
product decision prevents a safe implementation.

## Upgrade Range

- Treat changes to `github.com/ardanlabs/bucky` in the root and examples Go
  modules as the proposed Bucky upgrade. Compare the committed version with
  the proposed version in the worktree.
- Resolve both Bucky versions to exact tags and commits.
- Resolve each version's `download.DefaultWhisperVersion` to the exact
  whisper.cpp release, commit, and authenticated manifest digest.
- Compare the exact whisper.cpp revisions, not merely release notes or tag
  names.
- Confirm that Bucky publishes native artifacts for every platform/runtime
  combination Kronk advertises. Do not claim support for an artifact that is
  not published.

## Upstream Review

Use authoritative Bucky and whisper.cpp source, history, release notes,
headers, tests, examples, and manifests. Do not infer compatibility from commit
titles alone. Cite evidence for every material conclusion.

Determine:

1. Every exported Bucky Go API change: additions, removals, signature or type
   changes, struct fields, constants, defaults, ownership, lifetimes,
   concurrency, cancellation, and behavioral contracts.
2. Every native ABI or C API change used by Bucky or Kronk: exported symbols,
   function signatures, argument order, structs, field offsets and sizes,
   enums, callbacks, allocators, and free functions. Include whisper.cpp's
   context, full-decoding, token, and VAD parameter/result structures and any
   shared ggml interfaces used during initialization.
3. Whether old Bucky bindings can load the new native library and whether new
   bindings can load the old library. Treat an ABI mismatch as a coordinated
   dependency/runtime upgrade, even when Go source remains compatible.
4. Whether `pkg/download` changed its API, default pin, artifact matrix,
   manifest, digest verification, install records, or upgrade behavior.
5. Whether new or superseding APIs should be adopted by Kronk. Inspect actual
   semantics and return values; do not keep an older compatibility wrapper when
   doing so loses correctness or metadata.
6. Whether new model formats or workflows require additional configuration,
   audio inputs, transcript outputs, catalog entries, validation, resource
   accounting, examples, or server/API support.
7. Security, correctness, memory, performance, backend, thread-safety, and
   cancellation changes that affect Kronk.

Pay particular attention to batch transcription, translation to English,
language detection, English-only models, greedy and beam-search sampling,
fallback thresholds, initial prompts, segment and word timestamps, token
metadata, channel-separated diarization, audio decoding/resampling, streaming
partials and finals, prompt carryover, energy-based and Silero VAD, native
callbacks, result ownership, and model unloading.

## Kronk Integration Audit

Trace all upstream Bucky imports and whisper.cpp references. Distinguish the
upstream `github.com/ardanlabs/bucky` bindings from Kronk's `sdk/bucky` facade.
At minimum inspect:

- `sdk/bucky/`, especially initialization, logging, admission, cancellation,
  shutdown, file/channel transcription, and stream capacity ownership;
- `sdk/bucky/model/`, especially `Config`, native context and state creation,
  state pooling, sampling parameter translation, language operations, audio
  decoding, result conversion, timestamps, streaming, and VAD;
- `sdk/bucky/ffmpeg/`, including fallback decoding, channel handling, sample
  formats, and subprocess cancellation;
- `sdk/bucky/pool/` and affected shared pool/resource owners, including model
  planning, resolved configuration, memory reservations, loading, and unloading;
- `sdk/tools/bucky/libs/`, including `download.DefaultWhisperVersion`, version
  comparison, managed-install replacement, manifest verification, atomic
  activation, user-managed read-only paths, runtime compatibility selection,
  and supported combinations;
- `sdk/tools/bucky/models/`, including catalog drift, GGML header parsing,
  transcription models, and the auxiliary Silero VAD model;
- `cmd/kronk/bucky/`, server startup, audio and management routes, error mapping,
  authentication, and the BUI Translator, Whisper Libraries, and Whisper Models;
- every Bucky example and make target, including `examples/bucky/` and
  `examples/bucky-stream/`.

Verify that an installation recorded for the previous native version is
replaced by the new compatible bundle. Explicitly document that user-managed
library paths must be rebuilt or replaced when the ABI changes. Do not silently
allow a known-incompatible old or arbitrary newer native bundle merely because
its files exist or its numeric version is greater.

Check coexistence with llama.cpp: library resolution, shared ggml symbols and
backend registration, device discovery, initialization order, and degraded
startup must remain correct. Do not assume independently compatible Whisper
and llama bundles are automatically compatible when loaded in one process.

Preserve one shared model context with independent states for concurrent work.
Admission capacity, state release, stream worker exit, and shutdown must agree
on every success, error, and cancellation path. Streams reserve capacity until
their workers exit; preserve final-event delivery, partial replacement/drop
semantics, close/final flush, reset behavior, and timestamp continuity.

When upstream adds an API, decide separately whether Kronk:

- already exposes the capability correctly;
- can adopt it with a small correctness-preserving change;
- needs new high-level configuration or request/result fields;
- needs catalog/model/server work before the capability is usable; or
- should explicitly document that the capability is not yet exposed.

Implement clear, in-scope fixes and API adoption without waiting for another
prompt. Do not add speculative abstractions or unrelated features.

## Version and Dependency Alignment

Keep all applicable version references aligned:

- root `go.mod` and `go.sum`;
- `examples/go.mod` and `examples/go.sum`;
- `zarf/nix/gomod2nix.toml`, generated with the repository's configured
  `gomod2nix` workflow and containing the new Bucky version and NAR hash;
- any explicit whisper.cpp pins, manifests, install metadata, scripts, CI
  configuration, or packaging references;
- the README compatibility table and any exact authenticated pin text.

Avoid unrelated dependency upgrades. If Go tooling changes transitive modules,
identify why they are required and review the resulting diff rather than
accepting broad churn blindly.

## Documentation and Generated Artifacts

Documentation is part of the upgrade, not a follow-up. Audit and update:

- `README.md`, including the compatibility table and ABI migration warning;
- `.manual/chapter-18-bucky.md`, including installation and restart behavior,
  configuration, transcription/translation APIs, SDK and streaming semantics,
  supported capabilities, and explicit limitations;
- relevant Bucky implementation and lifecycle guidance in
  `.manual/chapter-20-developer-guide.md` when those contracts change;
- relevant exported Go doc comments in `sdk/bucky` and `sdk/bucky/model`;
- example source when the recommended API usage changes;
- release/breaking-change material when the repository's current release
  process requires it.

Run `make kronk-docs` after source documentation or exported API comments
change. Review and retain the corresponding generated BUI documentation,
including `DocsManual.tsx`, Bucky SDK docs, and example docs. Build the BUI so
the embedded production assets match the generated documentation. Render and
inspect the affected manual and SDK sections for readable text, code examples,
navigation, clipping, and layout defects.

## Verification

Follow `AGENTS.md` and the Developer Guide. For changed Go packages:

1. Run `go fix` on changed packages and review any edits it makes.
2. Run `gofmt -s -w` on all changed Go files.
3. Run scoped `go vet` and `staticcheck`.
4. Set `RUN_IN_PARALLEL=yes` and absolute `GITHUB_WORKSPACE`, then run focused
   tests in the affected `sdk/bucky` and `sdk/tools/bucky` packages. Select
   model-backed suites separately according to available prerequisites.
5. Build direct dependents and every affected Bucky example with an explicit
   output path when a package directory has the same name as the binary.
6. Run the repository-required build checks without running a prohibited full
   repository test suite or launching tests from `sdk/kronk/tests`.
7. Run `make kronk-docs`, `npm run build` in the BUI directory, and the server
   embedding/build check when documentation or production assets change.
8. Run `git diff --check` and inspect the complete diff for stale versions,
   omitted generated files, and unrelated changes.

Cover changed contracts with discriminating tests: sampling defaults and
overrides, language restrictions, timestamp units, channel ordering, malformed
audio, admission/state release, streaming close/cancellation, and installer
verification/replacement as applicable. For audio routes, check multipart
validation, the upload limit, translation behavior, and affected JSON, text,
SRT, and WebVTT response formats.

Do not download large models solely for verification. If compatible native
libraries and models are already installed, run the smallest relevant
transcription smoke test and, for streaming changes, a finite audio fixture
that exercises partial/final events and cleanup without requiring a microphone.
Exercise Silero VAD only when its model is available. Otherwise, state clearly
which model-backed checks were not run and name the missing prerequisites.
Compilation alone verifies Go API compatibility, not native ABI compatibility;
establish the ABI conclusion from authoritative headers, Bucky FFI
definitions/tests, pins, and manifests.

## Final Report

Report concisely:

- exact old and new Bucky and whisper.cpp revisions and manifest digests;
- the Go API and native ABI compatibility verdict, including relevant
  llama.cpp/ggml coexistence findings;
- source, dependency, installer, model/catalog, example, documentation, Nix,
  and generated-artifact changes made;
- new APIs adopted and capabilities intentionally not exposed, with rationale;
- verification commands and results;
- model-backed checks not run and any remaining risks or required user action.

Do not claim that no Kronk changes are needed until the source integration,
native install policy, version alignment, documentation, generated artifacts,
and focused verification have all been checked.
