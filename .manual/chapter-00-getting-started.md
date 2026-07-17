# Getting Started

Kronk is your personal engine for running open source models locally. Find
your hardware below, copy the one command, and run it. Then open
<http://localhost:11435> in your browser to download a model and start
chatting.

## Table of Contents

- [Quick Start — Copy, Paste, Run](#quick-start--copy-paste-run)
- [Which One Should I Use?](#which-one-should-i-use)
- [Going to Production](#going-to-production)

---

## Quick Start — Copy, Paste, Run

**🍎 On a Mac (MacBook, Mac mini, Mac Studio, iMac)**

Installs Kronk as a normal app and uses your Mac's GPU automatically. Paste
this into the Terminal app:

```shell
brew tap ardanlabs/kronk && brew trust ardanlabs/kronk && brew install kronk
KRONK_DOWNLOAD_ENABLED=true kronk server start
```

**🟩 If you have an NVIDIA graphics card (Linux or Windows)**

Runs in Docker with GPU acceleration. Needs Docker + the
[NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html):

```shell
docker run -d --name kronk --restart unless-stopped --gpus all \
  -e KRONK_DOWNLOAD_ENABLED=true \
  -p 11435:11435 -v kronk-data:/kronk \
  ghcr.io/ardanlabs/kronk:latest-cuda
```

**🖥️ If you have an AMD graphics card (Linux)**

Runs in Docker using ROCm. Needs Docker and access to `/dev/kfd` and
`/dev/dri`:

```shell
docker run -d --name kronk --restart unless-stopped \
  --device=/dev/kfd --device=/dev/dri --group-add video --group-add render \
  --security-opt seccomp=unconfined \
  -e KRONK_DOWNLOAD_ENABLED=true \
  -p 11435:11435 -v kronk-data:/kronk \
  ghcr.io/ardanlabs/kronk:latest-rocm
```

**🤷 Not sure, or none of the above**

This runs on any computer with Docker, using just the CPU. It works
everywhere, but don't expect great performance — larger models will be slow:

```shell
docker run -d --name kronk --restart unless-stopped \
  -e KRONK_DOWNLOAD_ENABLED=true \
  -p 11435:11435 -v kronk-data:/kronk \
  ghcr.io/ardanlabs/kronk:latest
```

**Now open <http://localhost:11435>** in your browser. Go to **Catalog**,
download a small model to try (e.g. `Qwopus3.5-4B-Coder.Q8_0`), then open
**Chat** and ask it something. That's it — Kronk is running locally, at zero
per-token cost, and nothing you type leaves your machine.

> **Heads up:** the Docker commands above publish port `11435` on every
> network interface with no authentication and downloads enabled — fine on
> your own machine, but if the host is reachable by anyone else (a cloud VM,
> a shared network), turn on auth and lock down the port first. See
> [Going to Production](#going-to-production) below.

## Which One Should I Use?

The quick start above already picked for you, but here's the difference in
plain terms:

- **Standalone app** (the Mac command) — Kronk installed like any normal
  program. Best for your own laptop or desktop. Full details in
  [2.3 Installing the CLI](chapter-02-installation.md#23-installing-the-cli).
- **Docker container** (the graphics-card and CPU commands) — Kronk runs from
  a ready-made image, nothing to install but Docker itself. Best for a server
  or remote machine that should keep running on its own. Full details in
  [2.4 Docker / OCI Container](chapter-02-installation.md#24-docker--oci-container).

## Going to Production

Before you expose Kronk beyond your own machine, turn on authentication:
enable it, retrieve the admin token, and mint scoped user tokens. See
[Chapter 12: Security & Authentication](chapter-12-security-authentication.md).
For unattended remote hosts,
[Chapter 2.4: Docker / OCI Container](chapter-02-installation.md#24-docker--oci-container)
covers running headless with auto-restart, updates, and uninstalling.

---

_Next: [Chapter 1: Introduction](chapter-01-introduction.md)_
