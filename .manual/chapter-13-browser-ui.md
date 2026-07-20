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
`0.0.0.0:11435`. The server root and `/admin` redirect to `/admin/` while
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
  in local model and catalog details.
- **Translator** records or uploads audio for transcription through Bucky.
  You can select a whisper model, language, and response format and inspect
  timestamped segments. See
  [Chapter 18 §18.6](chapter-18-bucky.md#186-bui-usage).

#### System

- **Info** reports server, host, device, library, and model diagnostics.
- **Running** shows models that are loading or resident in the pool, along
  with the current resource budget. Models can also be unloaded here.

#### Kronk

- **Models** lists local GGUF models and their metadata, effective
  configuration, sampling defaults, chat templates, and estimated VRAM.
  Models can be pulled from HuggingFace, copied from another Kronk Model
  Server (KMS), or removed. Persistent configuration is read from
  `~/.kronk/models/model_config.yaml`; the model details are read-only.
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
See [Chapter 18](chapter-18-bucky.md) for installation and transcription
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

#### Docs

The binary includes an offline documentation snapshot built with that Kronk
release:

- **Manual** — this manual, with chapter navigation
- **SDK** — SDK and model API references with examples
- **CLI** — command reference
- **Web API** — inference and management endpoint reference

### 13.3 Authentication

By default, the BUI and management APIs do not require a login. To protect
browser administration, enable admin authentication and configure the SHA-256
digest of the password:

```shell
export KRONK_AUTH_ADMIN_ENABLED=true
export KRONK_WEB_ADMIN_PASSWORD_SHA256="$(printf '%s' 'choose-a-password' | shasum -a 256 | awk '{print $1}')"
kronk server start
```

Login creates a one-hour admin token in an HttpOnly, SameSite cookie. The
browser cannot read the token, and the server uses it to authenticate the
BUI's same-origin `/v1` requests. Sign out from the sidebar to end the browser
session.

General authentication also enables admin authentication. Chapter 12 explains
the open, admin-only, and fully protected modes, including TLS and reverse
proxy considerations.

### 13.4 Operational Notes

- Downloading a library bundle does not switch the libraries used by the
  running process. Set `KRONK_LIB_PATH` or `KRONK_BUCKY_LIB_PATH` to the
  selected bundle and restart the server.
- Model and catalog detail pages display configuration but do not persist
  model overrides. Edit `~/.kronk/models/model_config.yaml` and reload the
  model when changing persistent configuration; see Chapter 3.
- Closing a browser tab does not explicitly delete its playground session.
  Use **Unload Model** when finished. Otherwise, the model remains subject to
  the server pool's normal eviction policy and is removed on server restart.

---

_Next: [Chapter 14: Client Integration](chapter-14-client-integration.md)_
