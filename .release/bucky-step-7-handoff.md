# Bucky Step 7 Handoff — BUI + Manual

Paste this into a new Amp thread to start step 7.

**IMPORTANT — step 6 scope changed.** Step 6 only restructured the CLI
(`cmd/kronk/{kronk,bucky}/...`); the server / HTTP wiring (transcription
endpoint, whisper libs/models endpoints) was explicitly deferred to a
later step. See `.release/bucky-integration-status.md` → "Step 6 — what
changed" for the canonical layout and the new CLI verbs.

Step 7 should therefore focus on:

1. **Manual chapters** — fully landable today; CLI verbs exist
   (`kronk bucky libs`, `kronk bucky model pull/list/remove/catalog`).
2. **BUI surface** — only the parts that do not require a whisper
   server endpoint. Anything that needs an HTTP transcription / whisper
   libs-pull endpoint should be deferred to the server-wiring step.

---

## Background

Read `.release/bucky-integration-status.md` first — it captures the
canonical design rules established in steps 1–5 (sdk layout, NSeqMax
= real parallelism, sem sizing 1:1 with NSeqMax, Init / log policy,
test infrastructure under `sdk/bucky/tests/`) and the step-6 CLI
restructure.

GitHub issue: <https://github.com/ardanlabs/kronk/issues/591>

Working branch: `bill/bucky`.

---

## Goal

Surface the whisper backend in the Browser UI and the manual so end
users can install whisper.cpp libs and models, run transcription, and
read about the new backend the same way they read about the llama
backend today.

---

## Scope

### A. Browser UI

Today the BUI's library/model/catalog pages are implicitly llama-only.
Step 7 introduces backend-kind awareness and adds a whisper surface
wherever a backend kind matters.

Pages / components to touch (verify against the live tree before
editing):

- **Library install / status** — `LibsPull.tsx`, `DocsCLILibs.tsx`,
  any `LibsStatus.tsx` if present. The BUI must let users pick the
  llama backend or the whisper backend and trigger the appropriate
  download via the step-6 server endpoint.
- **Model catalog browse / pull / remove** —
  `CatalogList.tsx`, `ModelList.tsx`, `ModelCard.tsx`,
  `ModelPull.tsx`, `ModelRemove.tsx`, `ModelPs.tsx`. Show whisper
  models from the bucky catalog (short names: `tiny`, `tiny.en`,
  `base`, `base.en`, `small`, `medium`, `large-v3`, etc.) alongside
  llama models. Tag each row with its backend kind so users
  understand which engine will load the file.
- **Model playground** — `ModelPlayground.tsx`. Decide whether
  whisper models get a dedicated audio-upload playground panel or
  link out to a separate transcription view. Whisper is single-turn
  and has no chat surface — do not force it into the chat UI.
- **Transcription view (new)** — at minimum: a file/drop input
  accepting wav / mp3 / flac, language hint dropdown driven by
  `bucky.LangStr(0..LangMaxID())`, transcript output area, optional
  per-segment timestamp display. Call the step-6 server endpoint.
- **Tooltips / labels** — every new form field must use the
  type-safe tooltip system per
  `cmd/server/api/frontends/bui/src/components/AGENTS.md` (add new
  entries to `PARAM_TOOLTIPS` in `ParamTooltips.tsx`; use
  `FieldLabel` for `<label>` and `labelWithTip` for table rows).
- **Docs panel** — `DocsCLILibs.tsx`, `DocsCLIModel.tsx`,
  `DocsCLIRun.tsx`, `DocsCLIServer.tsx`,
  `DocsSDK.tsx` / `DocsSDKExamples.tsx`. Update example snippets to
  show the whisper variants of every command introduced by step 6.

The `cmd/server/api/frontends/bui/` AGENTS.md files describe the
React + Vite layout. Run `npm run build` from
`cmd/server/api/frontends/bui/` after BUI changes; the built bundle
is embedded into the server binary under
`cmd/server/api/services/kronk/static/`.

### B. Manual chapters

Update these chapters (paths under `.manual/`):

- **chapter-01-introduction.md** — list the whisper backend alongside
  the llama backend in the "what is kronk" / supported-platforms
  overview. Note the SDK split: `sdk/kronk` (llama) and `sdk/bucky`
  (whisper).
- **chapter-02-installation.md** — document the step-6 whisper CLI
  verbs:
    - `kronk bucky libs` — install whisper.cpp libs for the current host
    - `kronk bucky libs --install --arch=… --os=… --processor=…` —
      install a non-host triple alongside
    - `kronk bucky libs --list-installs` / `--list-combinations` /
      `--remove-install`
    - `kronk bucky model catalog` — list bundled short names
    - `kronk bucky model pull <name|url>` — download a model
    - `kronk bucky model list` / `remove`
  Walk through downloading `tiny.en` for the example. Cross-link
  `examples/bucky/main.go`.
- **chapter-08-model-server.md** — add a whisper-backend section
  describing the SDK surface (`sdk/bucky`) and the on-disk lib/model
  layout (`~/.kronk/bucky-libraries`, `~/.kronk/bucky-models`), and
  how whisper differs from llama (no batch engine, no KV cache slots,
  NSeqMax = real parallelism, single-stream per State). **Defer the
  server-hosted whisper description until the server-wiring step
  lands** — note in the chapter that whisper is currently CLI / SDK
  only and the server runs llama only.
- **chapter-09-api-endpoints.md** — **defer transcription endpoint
  documentation** until the server-wiring step adds the endpoint.
  Optionally add a short "Coming soon" paragraph naming the planned
  transcription endpoint so users know not to call something that
  does not yet exist.
- **chapter-13-browser-ui.md** — describe only the BUI tabs / surfaces
  that step 7 actually ships (likely a catalog browse view and a
  manage-installed-models view backed by a server endpoint that
  already exists, or none if every whisper BUI surface needs the
  deferred server work). Skip the transcription view section until
  the server endpoint lands.
- **AGENTS.md** (repo root) — update the chapter-index table if any
  new chapter is added (none planned, but verify).

Keep the writing voice and section conventions used elsewhere in the
manual — terse, present-tense, link-rich.

---

## Out of scope

- The CLI restructure itself (step 6 — done; see status doc).
- **Server wiring (transcription endpoint, whisper libs/models
  endpoints)** — deferred to a later step. Do not add BUI surfaces or
  manual docs that imply those endpoints exist.
- The runtime / SDK design (steps 1–5 — frozen, do not change).
- Re-running the bucky FFI dance (leave the `replace` directives in
  whatever state they were in — `LangAutoDetectWithState` is still
  on Bill's local working tree as of the step-6 verification run).
- New release notes — that's the release-prep prompt
  (`.release/prompt.md`), not this work.

---

## Verification checklist

- `cd cmd/server/api/frontends/bui && npm run build` (or whatever the
  current build target is — confirm from the bui `package.json`)
  produces a clean bundle.
- `go build ./...` from repo root passes (the embedded static
  bundle compiles).
- Manual chapters render correctly when previewed (markdown lints
  cleanly; cross-links resolve).
- `RUN_IN_PARALLEL=yes GITHUB_WORKSPACE=$(pwd) go test ./sdk/bucky/...`
  still passes (no regressions to step-5 tests).
- A manual smoke pass via the CLI (server transcription endpoint
  does not yet exist):
  ```
  go run ./cmd/kronk bucky libs
  go run ./cmd/kronk bucky model pull tiny.en
  go run ./cmd/kronk bucky model list
  go run ./examples/bucky    # transcribes the JFK clip in-process
  ```

---

## Process constraints

- Load the `writing-go` skill before touching any `.go`.
- Run the post-edit chain (`gofmt -s -w`, `go vet`, `staticcheck`,
  `go build ./...`) after every `.go` change.
- Never run repo-wide tests; never run tests from `sdk/kronk/tests`.
- BUI tooltips MUST use the type-safe system in
  `cmd/server/api/frontends/bui/src/components/ParamTooltips.tsx` —
  read the AGENTS.md in that directory before adding any form field.
- Manual chapters MUST follow the existing voice / section layout
  (see chapter-08 and chapter-13 as the closest analogs to the new
  whisper content).

When step 7 lands, update `.release/bucky-integration-status.md` to
flip step 7 to ✅ Done.
