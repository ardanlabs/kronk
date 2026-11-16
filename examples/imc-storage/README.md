# Disk-backed IMC example

This example saves a conversation and its input checkpoint, exits, and continues the conversation in a fresh process. It reads the stored messages and final assistant text through the public `imc.Store` contract. Passing that conversation back to `Chat` lets IMC discover and restore a compatible checkpoint automatically. There is no manual native-state restore API here.

Run commands from the repository root. Install matching native libraries first with `kronk libs --local`. Use the same model, projector, MTP drafter, configuration, and library version in every phase. A save requires an empty directory. Use a private directory: messages, assistant text, and checkpoints are sensitive and are not encrypted.

## Text

Install the model with `kronk model pull unsloth/Qwen3-0.6B-Q8_0 --local`, or use an already installed text model.

```sh
MODEL="$HOME/.kronk/models/unsloth/Qwen3-0.6B-GGUF/Qwen3-0.6B-Q8_0.gguf"
DIRECTORY=$(mktemp -d)
for phase in save cold restore; do
    make example-imc-storage ARGS="-phase $phase -model \"$MODEL\" -directory \"$DIRECTORY\"" || break
done
```

- `save` asks for project code `713`, then checks that the latest snapshot contains native target state and the delivered assistant text.
- `cold` reads the saved conversation but does **not** configure model storage. It runs the same continuation as `restore`, requiring the correct answer, zero cached tokens, no native restore, and unchanged saved history.
- `restore` requires the correct answer, positive cached tokens, a native target restore event, and a newer revision under the same conversation ID. A cold fallback cannot pass.

Successful phases print `PASS` and exit zero. A failed assertion exits nonzero. You can repeat `cold` and `restore` to extend the conversation further. The backend retains only the latest snapshot for each conversation, not every revision.

## Vision and changed-image control

Use a compatible vision model and its matching projector. The example generates solid red and blue PNGs in memory; no external image fixtures are needed. For example, with these installed files:

```sh
MODEL="$HOME/.kronk/models/unsloth/Qwen3.5-0.8B-GGUF/Qwen3.5-0.8B-Q8_0.gguf"
PROJ="$HOME/.kronk/models/unsloth/Qwen3.5-0.8B-GGUF/mmproj-Qwen3.5-0.8B-Q8_0.gguf"
DIRECTORY=$(mktemp -d)
for phase in save cold restore; do
    make example-imc-storage ARGS="-scenario vision -phase $phase -model \"$MODEL\" -proj \"$PROJ\" -directory \"$DIRECTORY\"" || break
done
```

The original image is red. Save, cold, and restore require `red`. The example also reads the snapshot payloads through `Session.Read` and verifies the saved image bytes. This extra verification uses a separate allocation bounded to 512 MiB of target state; it is an example limit, not the engine's storage limit. Vision runs disable speculation to isolate media restoration.

Run the changed-image control against a **separate original save**, before any restore has extended it:

```sh
CONTROL_DIRECTORY=$(mktemp -d)
for phase in save changed-image; do
    make example-imc-storage ARGS="-scenario vision -phase $phase -model \"$MODEL\" -proj \"$PROJ\" -directory \"$CONTROL_DIRECTORY\"" || break
done
```

`changed-image` changes only the image to blue, preserving the original input text and not replaying the saved red answer. It requires `blue`, zero cached tokens, no native restore, a new conversation ID, and an unchanged original summary. This checks that matching text cannot authorize reuse of a different image. Start with a new directory to repeat this control; it deliberately leaves two conversations.

## MTP: independent draft KV

Use a model with own-KV MTP, such as this installed embedded-MTP model:

```sh
MODEL="$HOME/.kronk/models/unsloth/Qwen3.6-35B-A3B-MTP-GGUF/mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL.gguf"
DIRECTORY=$(mktemp -d)
for phase in save cold restore; do
    make example-imc-storage ARGS="-scenario mtp-own -phase $phase -model \"$MODEL\" -directory \"$DIRECTORY\"" || break
done
```

For a separate MTP head, add `-mtp-drafter /path/to/matching-mtp.gguf` in every phase. Do not pass a separate head as the target model.

The expected answer is `713` exactly eight times, giving drafting enough output to run. Every phase requires positive drafted tokens and no draft-disable reason. Save and restore require nonempty draft state. Restore additionally requires successful native target and draft restores, restored hidden-state evidence, and resume source `restored-draft-kv`.

## MTP: shared target KV

Use a model with shared-target-KV MTP and its matching head:

```sh
MODEL="$HOME/.kronk/models/unsloth/gemma-4-E4B-it-GGUF/gemma-4-E4B-it-Q4_K_M.gguf"
DRAFTER="$HOME/.kronk/models/unsloth/gemma-4-E4B-it-GGUF/mtp-gemma-4-E4B-it-Q4_K_M.gguf"
DIRECTORY=$(mktemp -d)
for phase in save cold restore; do
    make example-imc-storage ARGS="-scenario mtp-shared -phase $phase -model \"$MODEL\" -mtp-drafter \"$DRAFTER\" -directory \"$DIRECTORY\"" || break
done
```

This scenario requires active drafting, **empty** independent draft state, and, on restore, native target restoration with resume source `shared-target-kv`. It must not report an independent draft restore.

## Limits of this verification

These are local, separate-process functional checks, not performance benchmarks or reliability-pressure tests. They do not establish identical sampling behavior across restarts. Expected answers are deliberately strict; a different model may fail the answer check even when persistence works. MTP scenarios explicitly select MTP, never automatic target-only fallback.

Native restore and MTP-resume assertions inspect structured engine diagnostics through the configured logger. Those diagnostic event names are not a stable part of `imc.Store`; the verifier must track changes to them. The example does not interpret private restore metadata.

Failure-recovery scenarios and chat-history UI integration are separate follow-up work. Remove the temporary directories when finished; the example never deletes saved conversations automatically.
