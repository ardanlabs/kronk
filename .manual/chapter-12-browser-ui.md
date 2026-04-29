# Chapter 12: Browser UI (BUI)

## Table of Contents

- [12.1 Accessing the BUI](#121-accessing-the-bui)
- [12.2 Sidebar Layout](#122-sidebar-layout)
- [12.3 What the BUI Provides](#123-what-the-bui-provides)
  - [Models](#models)
  - [Catalog](#catalog)
  - [Libraries](#libraries)
  - [Apps](#apps)
  - [Security](#security)
  - [Docs](#docs)
  - [Settings](#settings)
- [12.4 Authentication](#124-authentication)
- [12.5 Notes on Live State](#125-notes-on-live-state)

---

Kronk ships with a built-in Browser UI (BUI) served from the same port as
the API. It is a thin client over the Web API and exposes the same
operations the CLI provides — pulling libraries and models, browsing the
catalog, managing tokens, and running interactive experiments against a
loaded model. This chapter is a high-level guide to what the BUI offers;
it intentionally does not enumerate every tab, filter, or button so that
the documentation stays accurate as the UI evolves.

### 12.1 Accessing the BUI

The BUI loads automatically when you open the server root in a browser:

```
http://localhost:11435
```

It is bundled inside the `kronk` binary and served from the same address
configured by `KRONK_WEB_API_HOST` (default `0.0.0.0:11435`).

### 12.2 Sidebar Layout

Navigation is grouped into the following top-level sections in the
sidebar:

- **Home** — landing page with a project banner and feature overview
- **Models** — local model file management
- **Catalog** — personal catalog browsing
- **Libraries** — llama.cpp library installs
- **Apps** — interactive tools (Chat, Playground, VRAM Calculator)
- **Security** — keys and tokens (relevant when auth is enabled)
- **Docs** — bundled documentation (Manual, SDK, CLI, Web API)
- **Settings** — BUI preferences and the admin token

### 12.3 What the BUI Provides

#### Models

The Models area lists every model file under `~/.kronk/models/` along
with the currently running models and a page for pulling new ones by
HuggingFace URL or canonical model id.

It mirrors the CLI surface: `kronk model list`, `kronk model ps`,
`kronk model pull`, `kronk model show`, and `kronk model remove`. Per-
model details (configuration, sampling, template, GGUF metadata) are
read-only views; persistent overrides live in
`~/.kronk/model_config.yaml` (see Chapter 3).

#### Catalog

The Catalog area browses entries in `~/.kronk/catalog.yaml` — your
**personal** catalog, seeded on first run from an embedded starter
list and grown as you pull or resolve new models against HuggingFace.

It mirrors `kronk catalog list`, `kronk catalog show`, and
`kronk catalog remove`, plus model pulling via `kronk model pull`.
There is no curated upstream catalog; Chapter 7 covers the catalog
model in detail.

#### Libraries

The Libraries area downloads and manages llama.cpp shared libraries
under `~/.kronk/libraries/<os>/<arch>/<processor>/`. The active
install used at runtime is selected via `KRONK_LIB_PATH`; the BUI can
stage additional `(arch, os, processor)` bundles for other targets but
does not hot-reload the active install. See Chapter 2 and the
`kronk libs` CLI for the same operations.

#### Apps

Three interactive tools live under **Apps**:

- **Chat** — a multi-turn chat interface with model selection, system
  prompt, and full sampling controls. Useful for ad-hoc conversations
  against any loaded model.
- **Model Playground** — an interactive bench for exercising a model
  under specific configuration (context window, batch sizes, cache
  mode, sampling parameters) and for running automated sweeps. It
  lets you load a session, send chat messages, inspect rendered
  prompts, and probe tool-calling behaviour against a configurable
  set of tool definitions.
- **VRAM Calculator** — a standalone estimator for the VRAM a model
  will consume given a chosen context window, slot count, KV cache
  precision, and other parameters. The same calculator is embedded in
  per-model detail views.

#### Security

When authentication is enabled (Chapter 11), the Security area lets
you list, create, and delete signing keys and create user tokens with
chosen durations, endpoint scopes, and rate limits. These pages
require an admin token configured under Settings; with auth disabled
they remain accessible but are not meaningful.

#### Docs

The Docs area embeds the full Kronk documentation set so it is
available offline next to the running server:

- **Manual** — this manual, with chapter navigation
- **SDK** — Kronk SDK and Model API references plus usage examples
- **CLI** — reference for `kronk` subcommands (catalog, libs, model,
  run, security, server)
- **Web API** — reference for the HTTP endpoints (Chat, Messages,
  Responses, Embeddings, Rerank, Tokenize, Tools)

#### Settings

Settings holds BUI-level preferences, including the API token used
by the BUI when calling the Web API. Set this when running with
`--auth-enabled` so the BUI can reach security-protected endpoints.

### 12.4 Authentication

The BUI talks to the same `/v1` API as any other client. When
`--auth-enabled` is set on `kronk server start`, every BUI call must
carry a valid bearer token — configure it under **Settings**. With
auth disabled the BUI works without configuration. See Chapter 11 for
key and token management.

### 12.5 Notes on Live State

A few things the BUI deliberately does not do:

- It does not switch the active llama.cpp install in-process. Changing
  `KRONK_LIB_PATH` requires a server restart.
- It does not edit `~/.kronk/model_config.yaml` from the model pages.
  Persistent configuration changes are made by editing that file
  directly (see Chapter 3); the BUI's per-model views are read-only.
- The Playground's loaded session is held in server memory; closing
  the browser tab does not unload the model. Use **Unload Session**
  before adjusting model configuration.

---

_Next: [Chapter 13: Client Integration](#chapter-13-client-integration)_
