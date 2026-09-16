Review and implement the proposed Malina and stable-diffusion.cpp upgrade for
Kronk as one complete change. Do not assume a dependency bump is sufficient.
Inspect upstream source and Kronk's actual use, make every required integration
and documentation change, regenerate derived artifacts, and run focused
verification. Ask for direction only if an unresolved incompatibility or
product decision prevents a safe implementation.

## Upgrade Range

- Treat changes to `github.com/ardanlabs/malina` in the root and examples Go
  modules as the proposed Malina upgrade. Compare the committed version with
  the proposed version in the worktree.
- Resolve both Malina versions to exact tags and commits.
- Resolve each version's `download.DefaultSDVersion` to the exact
  stable-diffusion.cpp release, commit, and authenticated manifest digest.
- Compare the exact stable-diffusion.cpp revisions, not merely release notes or
  tag names.
- Confirm that Malina publishes native artifacts for every platform/runtime
  combination Kronk advertises. Do not claim support for an artifact that is
  not published.

## Upstream Review

Use authoritative Malina and stable-diffusion.cpp source, history, release
notes, headers, tests, examples, and manifests. Do not infer compatibility from
commit titles alone. Cite evidence for every material conclusion.

Determine:

1. Every exported Malina Go API change: additions, removals, signature or type
   changes, struct fields, constants, defaults, ownership, lifetimes,
   concurrency, cancellation, and behavioral contracts.
2. Every native ABI or C API change used by Malina or Kronk: exported symbols,
   function signatures, argument order, structs, field offsets and sizes,
   enums, callbacks, allocators, and free functions.
3. Whether old Malina bindings can load the new native library and whether new
   bindings can load the old library. Treat an ABI mismatch as a coordinated
   dependency/runtime upgrade, even when Go source remains compatible.
4. Whether `pkg/download` changed its API, default pin, artifact matrix,
   manifest, digest verification, install records, or upgrade behavior.
5. Whether new or superseding APIs should be adopted by Kronk. Inspect actual
   semantics and return values; do not keep an older compatibility wrapper when
   doing so loses correctness or metadata.
6. Whether new model families or workflows require additional context paths,
   generation inputs, outputs, catalog roles, bundles, validation, resource
   accounting, examples, or server/API support.
7. Security, correctness, memory, performance, backend, thread-safety, and
   cancellation changes that affect Kronk.

Pay particular attention to text-to-image, image-to-image, ControlNet,
ADetailer, AnimateDiff and other video generation, audio-conditioned video,
upscaling, model/context construction, generated audio, effective frame rate,
tokenizers, progress/log callbacks, cancellation, native result ownership, and
model unloading.

## Kronk Integration Audit

Trace all Malina imports and stable-diffusion references. At minimum inspect:

- `sdk/malina/`, especially `model.Config`, native context construction,
  generation parameter translation, result conversion, video, upscaling,
  cancellation, and pooling;
- `sdk/tools/malina/libs/`, including `download.DefaultSDVersion`, version
  comparison, managed-install replacement, validation, atomic activation,
  user-managed read-only paths, and supported combinations;
- `sdk/tools/malina/models/`, including catalog drift, component roles, bundle
  manifests, and VRAM estimates;
- server routes, error mapping, pool/resource ownership, progress reporting,
  and browser UI surfaces that expose Malina;
- every Malina example and make target.

Verify that an installation recorded for the previous native version is
replaced by the new compatible bundle. Explicitly document that user-managed
library paths must be rebuilt or replaced when the ABI changes. Do not silently
allow a known-incompatible old or arbitrary newer native bundle merely because
its files exist or its numeric build is greater.

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
  `gomod2nix` workflow and containing the new Malina version and NAR hash;
- any explicit stable-diffusion.cpp pins, manifests, install metadata, scripts,
  CI configuration, or packaging references;
- the README compatibility table and exact authenticated pin text.

Avoid unrelated dependency upgrades. If Go tooling changes transitive modules,
identify why they are required and review the resulting diff rather than
accepting broad churn blindly.

## Documentation and Generated Artifacts

Documentation is part of the upgrade, not a follow-up. Audit and update:

- `README.md`, including the compatibility table and ABI migration warning;
- `.manual/chapter-19-malina.md`, including installation behavior, effective
  API semantics, examples, supported capabilities, and explicit limitations;
- relevant exported Go doc comments in `sdk/malina` and `sdk/malina/model`;
- example source when the recommended API usage changes;
- release/breaking-change material when the repository's current release
  process requires it.

Run `make kronk-docs` after source documentation or exported API comments
change. Review and retain the corresponding generated BUI documentation,
including `DocsManual.tsx`, Malina SDK docs, and example docs. Build the BUI so
the embedded production assets match the generated documentation. Render and
inspect the affected manual and SDK sections for readable text, code examples,
navigation, clipping, and layout defects.

## Verification

Follow `AGENTS.md` and the writing-Go guidance. For changed Go packages:

1. Run `gofmt -s` on changed Go files.
2. Run `go fix` on changed packages and review any edits it makes.
3. Run scoped `go vet` and `staticcheck`.
4. Set `RUN_IN_PARALLEL=yes` and absolute `GITHUB_WORKSPACE`, then run focused
   tests for `sdk/malina/...` and affected `sdk/tools/malina/...` packages.
5. Build direct dependents and every affected Malina example with an explicit
   output path when a package directory has the same name as the binary.
6. Run the repository-required build checks without running a prohibited full
   repository test suite or launching tests from `sdk/kronk/tests`.
7. Run `make kronk-docs`, `npm run build` in the BUI directory, and the server
   embedding/build check when documentation or production assets change.
8. Run `git diff --check` and inspect the complete diff for stale versions,
   omitted generated files, and unrelated changes.

Do not download multi-gigabyte models solely for verification. If compatible
native libraries and models are already installed, run the smallest relevant
generation smoke test. Otherwise, state clearly that model-backed generation
was not run and name the missing prerequisite. Compilation alone verifies Go
API compatibility, not native ABI compatibility; establish the ABI conclusion
from authoritative headers, Malina FFI definitions/tests, pins, and manifests.

## Final Report

Report concisely:

- exact old and new Malina and stable-diffusion.cpp revisions and manifest
  digests;
- the Go API and native ABI compatibility verdict;
- source, dependency, installer, model/catalog, example, documentation, Nix,
  and generated-artifact changes made;
- new APIs adopted and capabilities intentionally not exposed, with rationale;
- verification commands and results;
- model-backed checks not run and any remaining risks or required user action.

Do not claim that no Kronk changes are needed until the source integration,
native install policy, version alignment, documentation, generated artifacts,
and focused verification have all been checked.
