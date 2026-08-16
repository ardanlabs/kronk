# Chapter 13: Browser UI (BUI)

## Table of Contents

- [13.1 Accessing the BUI](#131-accessing-the-bui)
- [13.2 Capabilities](#132-capabilities)
  - [Apps](#apps)
  - [System](#system)
  - [Kronk](#kronk)
  - [Bucky](#bucky)
  - [Security](#security)
  - [Testing](#testing)
  - [Docs](#docs)
- [13.3 Authentication](#133-authentication)
- [13.4 Operational Notes](#134-operational-notes)

---

Kronk includes a Browser UI (BUI) for managing a local server and trying
models interactively. It is bundled in the `kronk` binary, served from the
same port as the Web API, and uses the server's `/v1` endpoints rather than
maintaining separate state.

This chapter describes the main areas of the BUI without cataloging every
control. The CLI remains useful for scripting and headless administration;
the BUI is not intended to duplicate every CLI command.

### 13.1 Accessing the BUI

The BUI is enabled by default. Start the server and open:

```
http://localhost:11435/admin/
```

The address comes from `KRONK_WEB_API_HOST`, whose default is
`127.0.0.1:11435`. The server root and `/admin` redirect to `/admin/` while
the BUI is enabled.

For a headless deployment, disable it with either form:

```shell
export KRONK_WEB_ADMIN_ENABLED=false
kronk server start
```

```shell
kronk server start --web-admin-enabled=false
```

### 13.2 Capabilities

The sidebar groups related operations by subsystem.

#### Apps

- **Chat** provides multi-turn conversations, model selection, system prompts,
  chat history, and sampling controls.
- **VRAM Calculator** estimates model memory requirements from a HuggingFace
  model without downloading the entire model. A calculator is also available
  in local model and catalog details. Set the intended context, sequence slots,
  KV precision and placement, layer/expert offload, devices, and tensor split
  before comparing the result with available memory. The estimate reads
  per-layer GGUF metadata, including full/SWA topology, recurrent layers, and
  embedded MTP/NextN layers when present.
- **Translator** records or uploads audio for transcription through Bucky.
  You can select a whisper model, language, and response format and inspect
  timestamped segments. See
  [Chapter 18 §18.5](https://www.kronkai.com/manual#185-browser-ui).

#### System

- **Info** reports server, host, device, library, and model diagnostics.
- **Running** shows models that are loading or resident in the pool, along
  with the current resource budget. Models can also be unloaded here.
- **Slots** polls the live batch-engine scheduler for each loaded generation
  model. It shows each slot's phase and request age, prefill stage 1 establishing
  the sequence by copying reusable IMC session state into the slot, and prefill
  stage 2 decoding both stable-prefix IMC extensions (such as completed tool
  calls) and the remaining uncached request tail. The Stage 2 cards show active,
  next, and waiting slots for those two decode paths. The table also reports
  per-slot progress, generation mode, and row contribution. Use this view with
  the batch-engine logs to verify prefill ownership, rotation, and generation
  priority during concurrent requests. Embedding and reranking models do not
  have generation slots and are omitted. Each Stage 2 selector retains its
  selected slot until that decode path is complete, then advances to the next
  eligible slot.
- **IMC Sessions** shows the bounded Incremental Message Cache entries owned by
  loaded models, including current and fallback cache usage.

#### Kronk

- **Models** lists local GGUF models and their metadata, effective
  configuration, sampling defaults, chat templates, and estimated VRAM.
  Models can be pulled from HuggingFace, copied from another Kronk Model
  Server (KMS), or removed. Persistent configuration is read from
  `~/.kronk/models/model_config.yaml`; the model details are read-only. A pull
  source that includes `owner/repo/file.gguf` pins that exact Hugging Face
  repository and file; a repository-only source opens its GGUF file list for
  selection.
- **Catalog** browses the personal catalog at
  `~/.kronk/catalog/catalog.yaml`. You can refresh its on-disk state, inspect
  entries, pull their files, and remove entries. See Chapter 8 for how the
  catalog is populated and resolved.
- **Libs** downloads and removes llama.cpp bundles for supported operating
  system, architecture, and processor combinations. Bundles are stored below
  `~/.kronk/libraries/`.

#### Bucky

Bucky has separate pages for downloading and removing whisper models and
managing whisper.cpp library bundles under `~/.kronk/bucky-libraries/`.
See [Chapter 18](https://www.kronkai.com/manual#chapter-18-bucky-audio-transcription) for installation and transcription
details.

#### Security

Security pages list, create, and delete signing keys and create user tokens.
Token controls include duration, endpoint grants, and rate limits. The
**Session** page reports whether browser administration authentication is
enabled and whether the browser has an authenticated admin session.

These tools remain available in open mode. This lets you prepare keys and
tokens before enabling authentication, but anyone who can reach an open
server can also use its management APIs. See Chapter 12 before exposing the
server beyond a trusted machine or network.

#### Testing

Testing provides several model evaluation workflows:

- **Accuracy** compares a model's reproduction of source functions with the
  actual source, individually, in batches, or across models.
- **Efficiency** compares generation throughput across selected models.
- **Basic** exercises chat, prompt rendering, and tool calling against a
  model loaded with a chosen runtime configuration.
- **Sampling** runs automated sampling-parameter sweeps.
- **Configuration** runs automated runtime-configuration sweeps.

The Basic, Sampling, and Configuration tools create server-side playground
sessions. Their configuration applies to that test session; it does not edit
the persistent model configuration file.

The VRAM calculator's SWA control follows runtime precedence. An explicit
calculator value wins; for an installed model, its `swa-full` model setting is
next; otherwise the calculator uses the llama.cpp default shipped with the
Kronk release. Leaving the control unset therefore does not mean compact SWA.
The result separates model weights, KV or recurrent state, compute buffers, and
CPU/GPU placement. It is an estimate rather than a reservation guarantee, so
retain operating headroom and use the same settings that will load the model.

#### Docs

The binary includes an offline documentation snapshot built with that Kronk
release:

- **Manual** — this manual, with chapter navigation
- **SDK** — SDK and model API references with examples
- **CLI** — command reference
- **Web API** — inference and management endpoint reference

### 13.3 Authentication

By default, the BUI and management APIs do not require a login. To protect
browser administration, select a mode that protects management and configure
the SHA-256 digest of the password:

```shell
export KRONK_AUTHORIZATION_MODE=management
export KRONK_WEB_ADMIN_PASSWORD_SHA_256="$(printf '%s' 'choose-a-password' | shasum -a 256 | awk '{print $1}')"
kronk server start
```

Login creates a one-hour admin token in an HttpOnly, SameSite cookie. The
browser cannot read the token, and the server uses it to authenticate the
BUI's same-origin `/v1` requests. Sign out from the sidebar to end the browser
session.

Chapter 12 explains the `open`, `management`, `authenticated`, and
`full-protected` modes, including TLS and reverse proxy considerations.

### 13.4 Operational Notes

- Downloading a library bundle does not switch the libraries used by the
  running process. Set `KRONK_LIB_PATH` or `KRONK_BUCKY_LIB_PATH` to the
  selected bundle and restart the server.
- Model details display the effective configuration but do not persist model
  overrides. Catalog details display catalog metadata, files, templates, and
  VRAM estimates rather than the effective model configuration. Playground
  settings, including load mode, apply only to that test session. Edit
  `~/.kronk/models/model_config.yaml` and reload the model when changing
  persistent configuration; see Chapter 3.
- Closing a browser tab does not explicitly delete its playground session.
  Use **Unload Model** when finished. Otherwise, the model remains subject to
  the server pool's normal eviction policy and is removed on server restart.

---

_Next: [Chapter 14: Client Integration](https://www.kronkai.com/manual#chapter-14-client-integration)_
