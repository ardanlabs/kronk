# Pluggable IMC storage example

This is one small example for trying an `imc.Store` implementation. It uses `sdk/storage/disk` by default, saves a conversation, exits, and continues that conversation in a fresh process. The model saves and restores checkpoints automatically; the application supplies the store and replays saved messages plus the final assistant text.

## Run it

From the repository root, install matching native libraries and a text model:

```sh
kronk libs --local
kronk model pull unsloth/Qwen3-0.6B-Q8_0 --local
```

Run each phase as a separate process, using the same model, directory, configuration, and native libraries:

```sh
MODEL="$HOME/.kronk/models/unsloth/Qwen3-0.6B-GGUF/Qwen3-0.6B-Q8_0.gguf"
DIRECTORY=$(mktemp -d)
make example-imc-storage ARGS="-phase save -model \"$MODEL\" -directory \"$DIRECTORY\""
make example-imc-storage ARGS="-phase restore -model \"$MODEL\" -directory \"$DIRECTORY\""
```

The `save` phase requires empty history. It asks for project code `713` and prints the assistant's answer, cached prompt-token count, and latest saved session ID/revision. The `restore` phase reads the latest saved conversation and appends its assistant response and a follow-up question. With a compatible checkpoint, expect positive cached tokens and a newer revision under the same session ID. Repeat `restore` to continue further.

This example does not assert an exact model answer or parse engine logs. A correct answer alone does not establish checkpoint reuse: inspect cached-token usage and session revisions. Storage is best effort; a missing or incompatible checkpoint can cause ordinary cold processing. An inference success is not a guarantee that saving succeeded. Check storage errors and the latest saved revision.

Use `-lib-path /path/to/libraries` if needed. The example disables speculation and uses a 2048-token context. Use a dedicated, private directory: messages and checkpoints are sensitive, and the disk implementation does not encrypt them. Remove the directory when finished; the example never deletes history automatically.

## Try your own implementation

In `main.go`, replace `disk.New(...)` with your backend's constructor. Its result must implement `imc.Store`; provide it to the existing `model.WithStorage(model.StorageConfig{Store: store})` call. Adjust backend-specific configuration and cleanup as needed. The application owns storage and unloads the model before closing it. `imc.Store` itself does not require a store-level `Close` method.

The example's `continueConversation` function accepts `imc.Store`, not a disk-specific type. It uses `Open`, `Record`, and `Session.Close` to read history without allocating checkpoint buffers. IMC calls `Find`, verifies candidate prefixes, and reads compatible model state when processing the next request. The application does not manually restore native state or force a match by session ID.

Implement the contract in `sdk/kronk/imc`:

- `Save` consumes borrowed snapshot data synchronously and atomically publishes the latest revision. Do not retain borrowed slices or require another checkpoint-sized copy. Reject stale revisions with `imc.ErrConflict`.
- `Find` returns a bounded, ordered set of eligible summaries, not a selected match. Do not read complete requests or checkpoint payloads during discovery. The model owns prefix matching and compatibility verification.
- `Open` returns an exact pinned revision implementing `imc.Session`. Supply owned record metadata, uncompressed sizes, complete verified payload reads, and handle cleanup. Missing sessions/revisions return an error matching `fs.ErrNotExist`.
- `List` provides paginated summaries; `Delete` removes discovery and future opens while preserving already-open readers.

Support concurrent calls and preserve wrapped context/I/O errors. The supplied store owns access isolation and namespacing. Compression, integrity checking, physical layout, and documented durability guarantees belong to the backend. The full interface comments are authoritative.

## Optional disk logging

The example passes `disk.WithLog(kronk.FmtLogger)` so disk operations are visible. Replace the formatter with any concurrency-safe function matching `func(context.Context, string, ...any)`, or omit `WithLog` for silence. This option belongs to the disk backend, not the storage interface. Custom stores choose their own logging configuration.

The disk callback receives one completion event per operation, outside storage locks, with key/value fields including duration and error. Relevant events include session IDs/revisions, uncompressed payload sizes, discovery counts, and startup recovery counts. A save's `published` field distinguishes an error before publication from an error after the replacement became visible. Messages, assistant text, payload contents, and pagination cursors are omitted; returned errors may contain filesystem paths. Startup and close use a background context; other calls retain their operation context.

A successful disk open/read is not a claim that the engine matched or restored the checkpoint. Diagnostic names and fields are not a versioned API.

## Maintainer verification

The larger verification harness is test-only, not part of the runnable example. Ordinary tests skip native scenarios unless model paths are supplied. For the text cold/restart controls and all 13 failure-recovery cases:

```sh
GITHUB_WORKSPACE="$PWD" \
KRONK_IMC_TEST_MODEL="$HOME/.kronk/models/unsloth/Qwen3-0.6B-GGUF/Qwen3-0.6B-Q8_0.gguf" \
go -C examples test -race ./imc-storage -run '^(TestStorageScenarios|TestFailureRecovery)$' -count=1 -timeout=5m -v
```

Optional native scenarios use `KRONK_IMC_TEST_VISION_MODEL` with `KRONK_IMC_TEST_VISION_PROJ`, `KRONK_IMC_TEST_MTP_OWN_MODEL` (and optional `KRONK_IMC_TEST_MTP_OWN_DRAFTER`), or `KRONK_IMC_TEST_MTP_SHARED_MODEL` with `KRONK_IMC_TEST_MTP_SHARED_DRAFTER`. Set `KRONK_IMC_TEST_LIB_PATH` for non-default libraries. Only configure the scenarios you intend to run; larger models may need a longer test timeout.

These tests retain cold controls, changed-image rejection, own/shared-KV MTP checks, and storage failure recovery. They reopen stores and load fresh models within the test process; the example commands above exercise separate processes. They are functional checks, not performance benchmarks, process-crash simulations, or reliability-pressure runs. Native assertions inspect engine diagnostics only in test code and must track diagnostic changes. MTP/media-specific failure recovery and chat-history UI integration remain outside this example.
