# Terminal chat

A standalone Bubble Tea chat app using Kronk's public SDK, Bubbles' composer and scrolling viewport, and Lip Gloss styling. It replaces the scanner-based chat example. No server or browser is needed.

## Run

From the repository root:

```shell
make example-chat
```

The default model is **`ornith-ai/Ornith-1.5-9B-Q4_K_M`**, the same as `examples/agent`. Startup locates or downloads that model and its vision projector in Kronk's normal model store, and installs matching llama.cpp libraries when needed. The default context window is 65,536 tokens; hardware auto-tuning is enabled, with a single generation slot and IMC enabled.

File tools operate relative to `examples/` when launched with the Make target. To choose a workspace, snapshot directory or context size:

```shell
make example-chat ARGS='-workspace /path/to/project -directory /path/to/private/chat-sessions -context 16384'
```

Alternatively, from `examples/`, use `go run ./chat`. Run the **whole package**, not just `main.go`. Use `go run ./chat -help` to list flags.

For an already-installed model, bypass model discovery with explicit paths:

```shell
go run ./chat \
  -model "$HOME/.kronk/models/ornith-ai/Ornith-1.5-9B-GGUF/Ornith-1.5-9B-Q4_K_M.gguf" \
  -projector "$HOME/.kronk/models/ornith-ai/Ornith-1.5-9B-GGUF/mmproj-Ornith-1.5-9B-Q4_K_M.gguf" \
  -workspace /path/to/project
```

Omit `-projector` for a text-only model. `-model-source` chooses a different model to locate/download when `-model` is omitted. `-lib-path` uses an existing native-library directory instead of running library installation.

## Controls

| Control | Action |
| --- | --- |
| Enter | Send the message or execute a slash command |
| Alt+Enter | Insert a newline; Shift+Enter also works in terminals reporting that key |
| PgUp / PgDn (Mac: Fn+↑ / Fn+↓) | Scroll the conversation |
| Mouse wheel / trackpad | Scroll when the pointer is over the message window |
| Click / drag the scrollbar | Jump to a position or drag through the conversation |
| F2 or `/select` | Toggle terminal text selection versus mouse scrolling; use Fn+F2 if macOS reserves F2 |
| Ctrl+C | Cancel generation or stop a tool workflow; quit when idle |
| Ctrl+Q | Cancel active work, drain the stream, then quit |
| `/new` | Start with fresh messages; keep saved history |
| `/history` | Browse this model's saved sessions, eight per page |
| `/log` | Show the full absolute log path and copy it to the system clipboard |
| `/image PATH` | Attach one PNG/JPEG to the next message; paths may contain spaces, without quotes |
| `/clear-image` | Remove the pending attachment |
| `/help` | Show the command summary |
| `/exit` or `/quit` | Quit while idle |

In history, use ↑/↓ (or j/k) to select, Enter to open, n/p for next/previous page, and Esc to return without changing the current conversation. Resize to at least 40 columns × 18 rows.

Replies stream into the conversation. A scrollbar on the right shows your position when the messages exceed the window height. You can drag its thumb or click its track; scrolling up also stops incoming text from pulling you back to the bottom. Mouse support depends on your terminal forwarding mouse events. F2 or `/select` disables mouse reporting so you can select text and use your terminal's Copy/Paste shortcuts (Cmd+C/Cmd+V on Mac); toggle it again to restore wheel scrolling and dragging.

Reasoning is red, tool calls and their approval arguments are green, and other conversation content is white. Completed reasoning, including tool-call reasoning, stays visible for the active conversation. It is UI-only: it is not sent with later requests or added to disk history. Reopening history, starting a new conversation or restarting the app clears it. The footer reports cached prompt tokens, saved revision, final model call's prompt/output tokens, and generation speed. Each user turn has a five-minute timeout and a maximum of twelve tool rounds.

## File tools

The app provides the agent example's four capabilities and tool names:

- `tool_read_file`: read a regular file, with numbered lines.
- `tool_search_files`: regex filename/content search below the chosen workspace.
- `tool_create_file`: create an empty file and missing parent directories; never overwrite an existing file.
- `tool_go_code_editor`: add, replace or delete a line, rejecting invalid Go and formatting the result.

Create/edit calls show the exact tool arguments and require **y** to allow or **n** to deny. Read/search calls execute automatically. Denials and errors are returned to the model as structured tool results. Completed tool calls/results are visible in the transcript and retained in saved input messages. Opening history does **not** rerun them. Cancelling a workflow does not roll back already-completed writes.

Paths must be relative to `-workspace`; `os.Root` rejects traversal and symlinks escaping that directory. This example is not a sandbox: use a disposable workspace when experimenting. Reads/edits are limited to 64 KiB. Search returns at most 100 paths and visits at most 10,000 entries; its result reports truncation and skipped content reads. Search skips hidden directories, `vendor`, `node_modules`, and symlinks. There is no shell-execution tool.

For example, ask: “Read notes.txt and tell me the project code.” To test approval, ask: “Create an empty scratch.go file.”

## Images

With a vision model/projector loaded, enter `/image /absolute/path/to/photo.jpg`, press Enter, then type a question and send it. A new attachment replaces the pending one. Accepted files are PNG/JPEG up to 8 MiB, 8,192 pixels per side, and 32 megapixels. The terminal displays an image label, not an inline image preview.

Image bytes are embedded in the saved messages and checkpoint media. The source image does not need to exist when the conversation is reopened. After a failed/cancelled request that ran no tools, the question and attachment return to the composer for retry.

## Persistence and restore

Defaults:

- Snapshots: `~/.kronk/examples/chat/`
- Storage and model diagnostic log: `~/.kronk/examples/chat.log`

Use `/log` to see and copy the full path, including when `-log` overrides the default. Clipboard copying uses the host operating system's clipboard service; if it is unavailable, the path remains visible and F2 enables manual selection. Share that file or its path when reporting a problem. The app clears the log on startup, after acquiring the snapshot directory lock, so it contains one run. Copy it before restarting if you need to retain it. `/new` starts a conversation without clearing this run's log or deleting saved sessions. The same logging function captures backend operations and existing model lifecycle diagnostics with microsecond timestamps. Insecure request/response logging remains disabled.

The disk backend retains **only the latest snapshot per IMC session**, with gzip-compressed full checkpoints. History comes from `imc.Store.List`; opening an entry uses `Open` to read its request and appends its separately saved final assistant text. Checkpoint buffers are not read just to show history. Saved final requests also contain any earlier tool-call/result messages in that conversation.

Opening history loads messages immediately. The **next sent message** lets IMC discover and verify a matching input checkpoint, restore it, replay the assistant text, and process the new input. Keep model files, projector, native libraries, context size and execution settings unchanged when demonstrating reuse across restarts. An incompatible checkpoint can fall back to normal prefill; a nonzero cached count alone is not proof of a disk read when RAM is already warm.

To demonstrate disk restore, send a message, wait for “saved revision”, quit, restart with the same settings/directory, choose `/history`, open the conversation, and send a follow-up without tools. Look for these events in order in the new run's log:

1. `disk-read` with `err <nil>`: the complete checkpoint and any media were read successfully. This alone does not prove native restoration.
2. `imc-restore-start`: the checkpoint is being restored into the active model sequence. `snapshot_bytes` reports its size.
3. `imc-restore-done`: native restoration consumed the full checkpoint byte count. `restored_bytes` must match `snapshot_bytes`; `physical_kv_cells` and `next_logical_position` describe the restored state. A short native restore fails the request instead of logging this success event.
4. `prefill-done` for the same slot and sequence: processing the remaining input is complete and output generation begins. This must follow `imc-restore-done`.
5. A successful replacement `disk-save` under the session ID from `disk-read`, with an increased revision: the restored conversation continued and was saved again.

The example has one generation slot, so this ordering is unambiguous in a cold-start follow-up without tools. Later RAM hits also emit `imc-restore-done`; use the preceding `disk-read` to distinguish disk recovery. A tool workflow contains multiple model calls, so its final cached-token count alone cannot prove the first call restored from disk. These are existing backend and engine diagnostics, not new storage-specific engine instrumentation.

Restoration reloads the complete saved **input checkpoint**, not model weights or a checkpoint taken after the assistant's final answer. The separately saved assistant text and new user message are processed after restoration, before the next response is generated.

Persistence is best effort. A failed save does not fail the reply; the footer says “no new snapshot” and the disk log records the error. The next complete save can recover without the previous save. This is cache-backed history, not a separate durable chat database: `/new` clears messages, but identical/prefix-related conversations can reuse an IMC session. A failure or interruption during a tool workflow may leave the most recent completed model turn rather than the whole user turn saved. Tool-only outputs and UI-only reasoning are not separately recoverable under the current storage contract; reopening history never resumes pending tool actions. Unsaved drafts and cancelled output are not durable.

Snapshots contain conversation text, tool results (including file contents), image data and model state. Files are private but **not encrypted**. Keep one app process per storage directory, and manage retention and disk usage yourself. Do not use a tool workspace containing secrets you do not want persisted.

## Supply your own storage

Replace `disk.New` in `main.go` with your `imc.Store` implementation and adjust backend cleanup. Both the model and chat UI use that interface; only startup knows the concrete backend. `disk.WithLog` and `model.WithLog` receive the same callback, which writes to a file instead of corrupting the terminal. No concrete logger is required by the storage contract.

For the minimal SDK integration without a TUI or tool loop, see [`../imc-storage`](../imc-storage).

## Tests

From the repository root:

```shell
GITHUB_WORKSPACE="$PWD" go -C examples test -race ./chat
```

This checks history's image/tool/settings preservation and file-tool behavior without loading a model. To include real inference and reopen storage with a fresh model:

```shell
GITHUB_WORKSPACE="$PWD" \
KRONK_CHAT_TEST_MODEL="$HOME/.kronk/models/ornith-ai/Ornith-1.5-9B-GGUF/Ornith-1.5-9B-Q4_K_M.gguf" \
go -C examples test -race ./chat -count=1 -timeout=10m
```

The opt-in scenario drives the Bubble Tea message/command interface, selects saved history, checks the remembered answer, and requires nonzero cached tokens after unloading/reloading the model. It does not require a graphical terminal.
