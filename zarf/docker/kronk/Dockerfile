# syntax=docker/dockerfile:1.7
# vim: filetype=dockerfile
#
# =============================================================================
# Kronk Dockerfile
#
# Produces a slim Linux container that runs the `kronk` server. Built
# llama.cpp shared libraries for one or more processor backends (cpu / cuda /
# vulkan / rocm) are baked into the image at build time so the resulting
# container is offline-ready.
#
# The image is multi-arch (linux/amd64, linux/arm64) and runs on macOS,
# Linux, and Windows hosts via Docker Desktop or any OCI runtime that
# supports Linux containers. GPU passthrough requires the host's NVIDIA /
# Vulkan / ROCm drivers (Apple Silicon GPU is NOT exposed to containers —
# Mac hosts are CPU-only).
#
# -----------------------------------------------------------------------------
# Compatibility matrix
# -----------------------------------------------------------------------------
#
# Which image / backend works on which host. Backed by:
#   - the upstream llama.cpp build matrix (sdk/tools/libs/combinations.go)
#   - the runtime-stage installs in this Dockerfile
#   - the GPU detection probe in sdk/tools/devices/gpu_detect_linux.go
#
# ┌─────────────┬────────────┬──────────────┬─────────┬──────┬──────────────────────────────────────────────┐
# │ Host OS     │ Host GPU   │ Image        │ Backend │  ?   │ Notes                                        │
# ├─────────────┼────────────┼──────────────┼─────────┼──────┼──────────────────────────────────────────────┤
# │ Linux       │ NVIDIA     │ :cuda        │ cuda    │  ✅  │ nvidia-container-toolkit + --gpus all        │
# ├─────────────┼────────────┼──────────────┼─────────┼──────┼──────────────────────────────────────────────┤
# │ Linux       │ AMD        │ :rocm        │ rocm    │  ✅  │ /dev/kfd + /dev/dri; ROCm baked in image     │
# ├─────────────┼────────────┼──────────────┼─────────┼──────┼──────────────────────────────────────────────┤
# │ Linux       │ AMD        │ :vulkan      │ vulkan  │  ✅  │ /dev/dri; RADV via mesa-vulkan-drivers       │
# ├─────────────┼────────────┼──────────────┼─────────┼──────┼──────────────────────────────────────────────┤
# │ Linux       │ Intel      │ :vulkan      │ vulkan  │  ✅  │ /dev/dri; ANV via mesa-vulkan-drivers        │
# ├─────────────┼────────────┼──────────────┼─────────┼──────┼──────────────────────────────────────────────┤
# │ Linux       │ NVIDIA     │ :vulkan      │ vulkan  │  ✅  │ --gpus all + DRIVER_CAPABILITIES=...graphics │
# ├─────────────┼────────────┼──────────────┼─────────┼──────┼──────────────────────────────────────────────┤
# │ Linux       │ none       │ :cpu/:latest │ cpu     │  ✅  │ always works                                 │
# ├─────────────┼────────────┼──────────────┼─────────┼──────┼──────────────────────────────────────────────┤
# │ Linux arm64 │ Jetson     │ :jetson      │ cuda    │  ✅  │ JetPack 6+; --runtime nvidia (no --gpus)     │
# ├─────────────┼────────────┼──────────────┼─────────┼──────┼──────────────────────────────────────────────┤
# │ Linux arm64 │ SoC iGPU   │ :vulkan      │ vulkan  │  ✅  │ Panfrost / Freedreno / Lima via /dev/dri     │
# ├─────────────┼────────────┼──────────────┼─────────┼──────┼──────────────────────────────────────────────┤
# │ macOS       │ any        │ :latest      │ cpu     │  ⚠️  │ Apple Silicon GPU NOT exposed to Docker      │
# ├─────────────┼────────────┼──────────────┼─────────┼──────┼──────────────────────────────────────────────┤
# │ Windows     │ NVIDIA     │ :cuda        │ cuda    │  ✅  │ Docker Desktop + WSL2 + NVIDIA driver        │
# ├─────────────┼────────────┼──────────────┼─────────┼──────┼──────────────────────────────────────────────┤
# │ Windows     │ AMD/Intel  │ :cpu         │ cpu     │  ⚠️  │ Vulkan-via-WSL2 unreliable for inference     │
# ├─────────────┼────────────┼──────────────┼─────────┼──────┼──────────────────────────────────────────────┤
# │ Windows     │ none       │ :cpu         │ cpu     │  ✅  │ always works                                 │
# └─────────────┴────────────┴──────────────┴─────────┴──────┴──────────────────────────────────────────────┘
#
# Legend
#   ✅  fully supported, GPU acceleration available end-to-end
#   ⚠️  works but with significant caveats (CPU-only fallback, or unreliable)
#
#   :latest  — multi-backend image (all four backends bundled, both arches)
#   :cpu     — minimal CPU-only image (works on every host listed above)
#
# -----------------------------------------------------------------------------
# Build invocations
# -----------------------------------------------------------------------------
#
# Default (all backends bundled, both arches):
#
#   docker buildx build \
#       --platform linux/amd64,linux/arm64 \
#       -f zarf/docker/kronk/Dockerfile \
#       -t ghcr.io/ardanlabs/kronk:latest \
#       -t ghcr.io/ardanlabs/kronk:all \
#       .
#
# CPU-only (smallest, runs anywhere):
#
#   docker buildx build \
#       --platform linux/amd64,linux/arm64 \
#       --build-arg LLAMA_PROCESSORS="cpu" \
#       -t ghcr.io/ardanlabs/kronk:cpu \
#       -f zarf/docker/kronk/Dockerfile .
#
# CUDA-only:
#
#   docker buildx build \
#       --platform linux/amd64,linux/arm64 \
#       --build-arg LLAMA_PROCESSORS="cuda" \
#       -t ghcr.io/ardanlabs/kronk:cuda \
#       -f zarf/docker/kronk/Dockerfile .
#
# Vulkan-only:
#
#   docker buildx build \
#       --platform linux/amd64 \
#       --build-arg LLAMA_PROCESSORS="vulkan" \
#       -t ghcr.io/ardanlabs/kronk:vulkan \
#       -f zarf/docker/kronk/Dockerfile .
#
# ROCm (amd64 only — combinations.go has no arm64 rocm bundle). The
# build-time ROCm runtime install is gated on `rocm` appearing in
# LLAMA_PROCESSORS, so only the `:rocm` (and any multi-backend image
# that includes rocm) carries the extra ~2 GB of ROCm packages:
#
#   docker buildx build \
#       --platform linux/amd64 \
#       --build-arg LLAMA_PROCESSORS="rocm" \
#       -t ghcr.io/ardanlabs/kronk:rocm \
#       -f zarf/docker/kronk/Dockerfile .
#
# Jetson (NVIDIA Jetson Orin / Xavier — linux/arm64 with JetPack 6 / L4T
# r36.x). Selects an NVIDIA L4T CUDA runtime as the base image so the
# ABI matches the host driver injected by `nvidia-container-runtime`.
# Always builds the arm64 cuda bundle and uses `--target=runtime-jetson`:
#
#   docker buildx build \
#       --platform linux/arm64 \
#       --build-arg LLAMA_PROCESSORS="cuda" \
#       --target runtime-jetson \
#       -t ghcr.io/ardanlabs/kronk:jetson \
#       -f zarf/docker/kronk/Dockerfile .
#
# -----------------------------------------------------------------------------
# Run invocations
# -----------------------------------------------------------------------------
#
# CPU on any host:
#
#   docker run --rm \
#       -p 11435:11435 \
#       -v kronk-data:/kronk \
#       ghcr.io/ardanlabs/kronk:latest
#
# NVIDIA GPU — all cards (host needs nvidia-container-toolkit):
#
#   docker run --rm --gpus all \
#       -p 11435:11435 \
#       -v kronk-data:/kronk \
#       ghcr.io/ardanlabs/kronk:latest
#
# NVIDIA GPU — specific cards by index (run `nvidia-smi -L` on the host
# to map indices to physical cards). The quoting around `device=...` is
# required by the docker CLI parser:
#
#   docker run --rm --gpus '"device=0,1"' \
#       -p 11435:11435 \
#       -v kronk-data:/kronk \
#       ghcr.io/ardanlabs/kronk:latest
#
# NVIDIA GPU — specific cards by UUID (stable across reboots / PCI
# reordering; get UUIDs from `nvidia-smi -L`):
#
#   docker run --rm \
#       --gpus '"device=GPU-3a1f...,GPU-9b2e..."' \
#       -p 11435:11435 \
#       -v kronk-data:/kronk \
#       ghcr.io/ardanlabs/kronk:latest
#
# AMD GPU via ROCm — all cards (linux/amd64 host with ROCm-capable kernel
# driver installed; the user running docker must be in the `render` and
# `video` groups, or pass `--group-add` explicitly). `/dev/kfd` is the
# compute scheduler shared by every AMD GPU; bind-mounting the entire
# `/dev/dri` directory exposes every render node at once:
#
#   docker run --rm \
#       --device=/dev/kfd \
#       --device=/dev/dri \
#       --group-add video \
#       --group-add render \
#       --security-opt seccomp=unconfined \
#       -p 11435:11435 \
#       -v kronk-data:/kronk \
#       ghcr.io/ardanlabs/kronk:rocm
#
# AMD GPU via ROCm — specific cards by render node (each GPU gets its own
# `/dev/dri/renderD<N>` device, starting at 128; run `ls /dev/dri/` and
# `rocm-smi --showuniqueid` on the host to map render nodes to physical
# cards). Always include `/dev/kfd` regardless of how many GPUs you pass:
#
#   docker run --rm \
#       --device=/dev/kfd \
#       --device=/dev/dri/renderD128 \
#       --device=/dev/dri/renderD129 \
#       --group-add video \
#       --group-add render \
#       --security-opt seccomp=unconfined \
#       -e HIP_VISIBLE_DEVICES=0,1 \
#       -p 11435:11435 \
#       -v kronk-data:/kronk \
#       ghcr.io/ardanlabs/kronk:rocm
#
# NVIDIA Jetson Orin / Xavier (linux/arm64 host running JetPack 6+).
# Requires `nvidia-container-runtime` set as the default runtime in
# /etc/docker/daemon.json on the device — JetPack ships it pre-
# configured. The L4T base image's ABI matches what the runtime
# injects, so `libcuda.so` and friends Just Work without `--gpus`:
#
#   docker run --rm --runtime nvidia \
#       -e NVIDIA_DRIVER_CAPABILITIES=compute,utility,graphics \
#       -p 11435:11435 \
#       -v kronk-data:/kronk \
#       ghcr.io/ardanlabs/kronk:jetson
#
# Verify CUDA detection inside the Jetson container:
#
#   docker run --rm --runtime nvidia \
#       --entrypoint nvidia-smi \
#       ghcr.io/ardanlabs/kronk:jetson
#
# (On Jetson `nvidia-smi` is replaced by `tegrastats`; the toolkit also
# injects a `nvidia-smi` shim, but if it is missing fall back to
# `tegrastats` on the host to confirm GPU activity.)
#
# -----------------------------------------------------------------------------
# Vulkan run invocations
#
# Vulkan is vendor-neutral: the same `:vulkan` image runs on AMD, NVIDIA,
# and Intel GPUs as long as the host kernel exposes the right driver and
# the container can see the matching ICD (Installable Client Driver). The
# image already ships `libvulkan1`; vendor ICDs come from the host via
# either device passthrough (AMD/Intel) or the NVIDIA Container Runtime.
# Pick `:vulkan` over `:cuda`/`:rocm` when you want a single image that
# works across vendors, or for Intel Arc / iGPUs which have no dedicated
# bundle. List visible devices inside the container with `vulkaninfo
# --summary` (install `vulkan-tools` for ad-hoc debugging).
# -----------------------------------------------------------------------------
#
# Vulkan on AMD GPU (no ROCm needed — uses the open-source RADV driver
# bundled with mesa-vulkan-drivers on the host; works on any GCN/RDNA
# card, including ones not supported by ROCm):
#
#   docker run --rm \
#       --device=/dev/dri \
#       --group-add video \
#       --group-add render \
#       -p 11435:11435 \
#       -v kronk-data:/kronk \
#       ghcr.io/ardanlabs/kronk:vulkan
#
# Vulkan on NVIDIA GPU (host needs nvidia-container-toolkit; the toolkit
# injects libnvidia-glvkspirv and the NVIDIA Vulkan ICD into the
# container automatically when graphics capabilities are requested):
#
#   docker run --rm --gpus all \
#       -e NVIDIA_DRIVER_CAPABILITIES=compute,utility,graphics \
#       -p 11435:11435 \
#       -v kronk-data:/kronk \
#       ghcr.io/ardanlabs/kronk:vulkan
#
# Vulkan on Intel GPU (Arc dGPU or Iris/UHD iGPU; uses the open-source
# ANV driver. No special toolkit required — just expose /dev/dri):
#
#   docker run --rm \
#       --device=/dev/dri \
#       --group-add video \
#       --group-add render \
#       -p 11435:11435 \
#       -v kronk-data:/kronk \
#       ghcr.io/ardanlabs/kronk:vulkan
#
# Vulkan — specific GPU when the host has multiple cards (Vulkan
# enumerates devices in the order the loader discovers them; pin one
# with the standard loader env vars). `vulkaninfo --summary` inside the
# container prints each device's index and UUID:
#
#   docker run --rm \
#       --device=/dev/dri \
#       --group-add video \
#       --group-add render \
#       -e VK_LOADER_DRIVERS_SELECT='*radeon*' \
#       -e MESA_VK_DEVICE_SELECT='1002:744c' \
#       -p 11435:11435 \
#       -v kronk-data:/kronk \
#       ghcr.io/ardanlabs/kronk:vulkan
#
# Vulkan device selection — what the env vars above mean:
#
#   VK_LOADER_DRIVERS_SELECT  — Vulkan loader filter that restricts which
#                               ICD JSON manifests are loaded. The value
#                               is a glob matched against ICD filenames
#                               under /usr/share/vulkan/icd.d/. Common
#                               values: '*radeon*' (Mesa RADV for AMD),
#                               '*intel*' (Mesa ANV for Intel), '*nvidia*'
#                               (proprietary NVIDIA ICD). Omit to let the
#                               loader try every installed ICD.
#
#   MESA_VK_DEVICE_SELECT     — Mesa-only filter that picks one physical
#                               GPU when multiple devices match the
#                               loaded ICD. Format is '<vendor>:<device>'
#                               in PCI hex IDs (no 0x prefix). Vendor IDs:
#                               1002 = AMD, 8086 = Intel, 10de = NVIDIA.
#                               Device IDs are model-specific.
#
# Discover the PCI IDs of every GPU on the host with:
#
#   lspci -nn | grep -iE 'vga|3d|display'
#
# Sample output (the bracketed [vendor:device] is what you want):
#
#   03:00.0 VGA compatible controller [0300]: AMD ... [1002:744c] (rev c8)
#   0b:00.0 VGA compatible controller [0300]: AMD ... [1002:73a5] (rev c1)
#
# Two AMD cards, three common scenarios:
#
#   1. Pin to one card  →  -e MESA_VK_DEVICE_SELECT='1002:744c'
#   2. Pin to the other →  -e MESA_VK_DEVICE_SELECT='1002:73a5'
#   3. Expose both      →  drop MESA_VK_DEVICE_SELECT entirely; llama.cpp
#                          will see GPU0 and GPU1 and can split layers
#                          across them via the model config.
#
# Alternative: select by enumeration index instead of PCI ID using the
# generic loader var (matches the order shown by `vulkaninfo --summary`):
#
#   -e VK_DEVICE_SELECT='0'   # first device the loader enumerates
#   -e VK_DEVICE_SELECT='1'   # second device
#
# Two identical cards (same vendor:device PCI ID) — MESA_VK_DEVICE_SELECT
# matches BOTH and cannot pin one over the other. Example host with two
# RX 7900 XTX cards:
#
#   83:00.0 ... [1002:744c] (rev c8)
#   86:00.0 ... [1002:744c] (rev c8)
#
# Three workable approaches:
#
#   a. Expose both — drop MESA_VK_DEVICE_SELECT; llama.cpp sees GPU0 and
#      GPU1 and can split layers across them via the model config.
#
#   b. Pin by enumeration index — use VK_DEVICE_SELECT='0' or '1'. Order
#      normally follows PCI bus order; confirm with `vulkaninfo --summary`.
#
#   c. Pin by physical render node — each GPU owns its own
#      /dev/dri/renderD<N> (starts at 128). Map nodes to PCI buses on the
#      host with `ls -la /dev/dri/by-path/` and pass only the one you
#      want, e.g. `--device=/dev/dri/renderD128`. The container then
#      literally only sees that card and no env-var filtering is needed.
#
# Sanity check the filtering inside the container (the `:vulkan` image
# does not ship vulkan-tools; run a throwaway Ubuntu container with
# vulkan-tools installed and the same --device/--group-add flags to
# confirm enumeration before launching kronk).
#
# Liveness check:
#
#   curl http://localhost:11435/v1/liveness
#
# =============================================================================

# -----------------------------------------------------------------------------
# Build-time arguments
# -----------------------------------------------------------------------------

# Space-separated list of llama.cpp processor backends to bundle. Any value
# from `kronk libs --list-combinations` for the target arch is valid.
ARG LLAMA_PROCESSORS="cpu cuda vulkan rocm"

# Optional pin for a specific llama.cpp release. Empty string lets kronk
# choose its baked-in `defaultVersion` (see sdk/tools/libs/libs.go).
ARG LLAMA_VERSION=""

# Free-form tag string written into the OCI image labels. CI should set this
# from `git describe` or the release tag.
ARG KRONK_VERSION="dev"

# Note: BUILDPLATFORM / TARGETPLATFORM / TARGETOS / TARGETARCH are
# automatically populated by BuildKit and must NOT be declared globally
# here (declaring them with no default clobbers the auto-provided value).
# Each stage that consumes them re-declares the ones it needs.

# =============================================================================
# Stage 1 — builder
#
# One stage installs both Go (parsed from go.mod) and Node.js 22 onto an
# Ubuntu 24.04 base. It generates BUI docs, builds the BUI bundle (which
# Vite writes into cmd/server/api/services/kronk/static/), then cross-
# compiles the kronk binary with the BUI assets embedded.
# =============================================================================

FROM --platform=$BUILDPLATFORM ubuntu:24.04 AS builder

ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH

ENV DEBIAN_FRONTEND=noninteractive

# Build dependencies: curl + ca-certificates to fetch toolchains, git for
# any go modules that need it, build-essential for any pure-Go modules
# that still shell out, xz-utils for the Node tarball.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        git \
        build-essential \
        xz-utils \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Install Node.js 22 from the official binary distribution (NodeSource was
# considered but pulls in a setup script we'd rather not run).
ENV NODE_VERSION=22.11.0
RUN set -eux; \
    case "$(uname -m)" in \
        x86_64)  NODE_ARCH=x64 ;; \
        aarch64) NODE_ARCH=arm64 ;; \
        *) echo "unsupported arch: $(uname -m)" >&2; exit 1 ;; \
    esac; \
    curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODE_ARCH}.tar.xz" \
        | tar -xJ -C /usr/local --strip-components=1
RUN node --version && npm --version

WORKDIR /src

# -----------------------------------------------------------------------------
# Pre-cache Go modules (separate layer so source edits don't bust the cache).
# -----------------------------------------------------------------------------
COPY go.mod go.sum ./

# Install the Go toolchain version pinned in go.mod so the builder always
# matches what developers use locally.
RUN set -eux; \
    GO_VERSION="$(awk '/^go [0-9]/ {print $2; exit}' go.mod)"; \
    case "$(uname -m)" in \
        x86_64)  GO_ARCH=amd64 ;; \
        aarch64) GO_ARCH=arm64 ;; \
        *) echo "unsupported arch: $(uname -m)" >&2; exit 1 ;; \
    esac; \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz" \
        | tar -xz -C /usr/local
ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOTOOLCHAIN=local

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/go/pkg/mod \
    go mod download

# -----------------------------------------------------------------------------
# Pre-cache npm modules (separate layer keyed on the lockfile).
# -----------------------------------------------------------------------------
COPY cmd/server/api/frontends/bui/package.json \
     cmd/server/api/frontends/bui/package-lock.json \
     cmd/server/api/frontends/bui/

RUN --mount=type=cache,target=/root/.npm \
    cd cmd/server/api/frontends/bui && npm ci

# -----------------------------------------------------------------------------
# Bring in the rest of the source. Everything past this point invalidates
# only on actual code changes. node_modules is excluded by .dockerignore
# so the cached install above survives this COPY.
# -----------------------------------------------------------------------------
COPY . .

# Generate BUI docs (writes into cmd/server/api/frontends/bui/src/...) and
# build the Vite bundle (writes into cmd/server/api/services/kronk/static/).
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/go/pkg/mod \
    go run ./cmd/server/api/tooling/docs

RUN --mount=type=cache,target=/root/.npm \
    cd cmd/server/api/frontends/bui && npm run build

# Cross-compile the kronk binary. CGO_ENABLED=0 produces a fully static
# binary; the llama.cpp shared libraries are loaded at runtime via dlopen
# (purego) so CGO is not required for kronk itself.
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/go/pkg/mod \
    CGO_ENABLED=0 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    go build \
        -trimpath \
        -ldflags="-s -w" \
        -o /out/kronk \
        ./cmd/kronk

# =============================================================================
# Stage 2 — libs-fetcher
#
# Runs the freshly built kronk binary and uses `kronk libs --install` to
# download every requested processor bundle for the target architecture.
# Output lands under /kronk/libraries/linux/<TARGETARCH>/<processor>/
# ready to be copied into the runtime stage.
#
# This stage intentionally inherits TARGETPLATFORM (no `--platform`
# constraint) so the cross-compiled kronk binary runs natively on the
# target arch. Under multi-arch buildx that means QEMU emulation when the
# build host arch differs from the target — acceptable here because
# `kronk libs` is network-bound, not CPU-bound.
# =============================================================================

FROM ubuntu:24.04 AS libs-fetcher

ARG TARGETARCH
ARG LLAMA_PROCESSORS
ARG LLAMA_VERSION

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/kronk /usr/local/bin/kronk
RUN chmod +x /usr/local/bin/kronk && /usr/local/bin/kronk --help >/dev/null

# KRONK_BASE_PATH is honored by every kronk subcommand (see
# sdk/tools/defaults/defaults.go and the libs subcommand wiring), so the
# install lands under /kronk/libraries directly.
ENV KRONK_BASE_PATH=/kronk

# Install one bundle per requested processor. Failures (e.g. rocm on arm64
# which has no upstream build) are logged and skipped so a multi-arch
# build of an arch-specific bundle list still produces a usable image.
RUN set -eux; \
    mkdir -p /kronk; \
    for processor in ${LLAMA_PROCESSORS}; do \
        echo ">>> Installing llama.cpp libs: linux/${TARGETARCH}/${processor}"; \
        if [ -n "${LLAMA_VERSION}" ]; then \
            kronk libs --local --install \
                --arch="${TARGETARCH}" \
                --os=linux \
                --processor="${processor}" \
                --version="${LLAMA_VERSION}" \
                || echo "WARNING: skipping unsupported triple linux/${TARGETARCH}/${processor}"; \
        else \
            kronk libs --local --install \
                --arch="${TARGETARCH}" \
                --os=linux \
                --processor="${processor}" \
                || echo "WARNING: skipping unsupported triple linux/${TARGETARCH}/${processor}"; \
        fi; \
    done; \
    echo ">>> Installed bundles:"; \
    kronk libs --local --list-installs || true

# =============================================================================
# Stage 3 — runtime
#
# The slimmest viable runtime image. Ubuntu 24.04 is required because
# llama.cpp's prebuilt bundles dynamically link against libstdc++, libgomp,
# libcurl, libvulkan, libopenblas, and (for CUDA/ROCm) the host-mounted
# driver libraries surfaced via NVIDIA_* / LD_LIBRARY_PATH.
# =============================================================================

FROM ubuntu:24.04 AS runtime

ARG KRONK_VERSION
ARG LLAMA_PROCESSORS
# ROCm version installed when LLAMA_PROCESSORS contains `rocm`. Pin to a
# specific patch release for reproducibility; bump in lockstep with the
# llama.cpp ROCm bundle if upstream advances its build target.
ARG ROCM_VERSION="6.4.3"

LABEL org.opencontainers.image.source="https://github.com/ardanlabs/kronk" \
      org.opencontainers.image.title="kronk" \
      org.opencontainers.image.description="Local LLM inference SDK and server CLI" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.vendor="Ardan Labs" \
      org.opencontainers.image.version="${KRONK_VERSION}" \
      org.opencontainers.image.url="https://github.com/ardanlabs/kronk" \
      org.opencontainers.image.documentation="https://github.com/ardanlabs/kronk#readme"

ENV DEBIAN_FRONTEND=noninteractive

# Runtime shared library dependencies for the bundled llama.cpp backends.
# `curl` is included so the HEALTHCHECK below can probe /v1/liveness.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        libcurl4 \
        libgomp1 \
        libnuma1 \
        libopenblas0 \
        libstdc++6 \
        libvulkan1 \
        mesa-vulkan-drivers \
        tzdata \
        vulkan-tools \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Install ROCm runtime (HIP runtime, rocBLAS, hipBLAS, rocminfo) when
# `rocm` is present in LLAMA_PROCESSORS. AMD has no equivalent of the
# NVIDIA Container Toolkit — the host kernel module is exposed via
# /dev/kfd + /dev/dri but the userspace HIP/HSA libraries that
# llama.cpp's ROCm bundle dlopens must live inside the container.
# Adds ~2 GB to images that bundle the ROCm backend; zero impact on
# images that do not (e.g. `:cpu`, `:cuda`, `:vulkan`).
#
# `rocminfo` doubles as the GPU detection probe for `DetectGPU()` (see
# sdk/tools/devices/gpu_detect_linux.go), so installing it lets the
# server auto-pick `--processor=rocm` instead of falling back to cpu.
RUN set -eux; \
    if echo " ${LLAMA_PROCESSORS} " | grep -q ' rocm '; then \
        apt-get update; \
        apt-get install -y --no-install-recommends \
            gnupg \
            wget; \
        wget -qO- https://repo.radeon.com/rocm/rocm.gpg.key \
            | gpg --dearmor > /usr/share/keyrings/rocm-archive-keyring.gpg; \
        echo "deb [arch=amd64 signed-by=/usr/share/keyrings/rocm-archive-keyring.gpg] \
            https://repo.radeon.com/rocm/apt/${ROCM_VERSION} noble main" \
            > /etc/apt/sources.list.d/rocm.list; \
        echo "Package: *" > /etc/apt/preferences.d/rocm-pin-600; \
        echo "Pin: release o=repo.radeon.com" >> /etc/apt/preferences.d/rocm-pin-600; \
        echo "Pin-Priority: 600" >> /etc/apt/preferences.d/rocm-pin-600; \
        apt-get update; \
        apt-get install -y --no-install-recommends \
            rocm-hip-libraries \
            rocm-smi-lib \
            rocminfo; \
        echo "/opt/rocm/lib" > /etc/ld.so.conf.d/rocm.conf; \
        ldconfig; \
        apt-get clean; \
        rm -rf /var/lib/apt/lists/*; \
    fi

# Create the `render` group so `--group-add render` works at run time
# without callers having to look up the host's numeric GID first. GID 110
# matches the default `/dev/dri/renderD*` ownership on Debian/Ubuntu
# hosts (set when the host installs mesa-vulkan-drivers / ROCm). On
# hosts where the render GID differs, callers should fall back to the
# numeric form: --group-add "$(getent group render | cut -d: -f3)".
RUN groupadd --system --gid 110 render || true

# kronk binary.
COPY --from=builder /out/kronk /usr/local/bin/kronk

# Pre-baked llama.cpp bundles for the target arch.
COPY --from=libs-fetcher /kronk/libraries /kronk/libraries

# Default model config — overridable by bind-mounting another file at the
# path referenced by KRONK_POOL_MODEL_CONFIG_FILE below.
COPY zarf/kms/model_config.yaml /etc/kronk/model_config.yaml

# Persistent state lives under /kronk. Models are huge and must be mounted;
# libraries are baked above; catalog/keys/logs land under /kronk too. All
# subcommands honor KRONK_BASE_PATH (set below) so this is the single
# source of truth for kronk's on-disk layout.
RUN mkdir -p /kronk/models /kronk/catalog /kronk/keys

# -----------------------------------------------------------------------------
# Runtime configuration
# -----------------------------------------------------------------------------
ENV KRONK_BASE_PATH=/kronk \
    KRONK_WEB_API_HOST=0.0.0.0:11435 \
    KRONK_DOWNLOAD_ENABLED=true \
    KRONK_INSECURE_LOGGING=false \
    KRONK_POOL_MODEL_CONFIG_FILE=/etc/kronk/model_config.yaml

# NVIDIA GPU passthrough hints — honoured by the NVIDIA Container Runtime
# when the user runs with `--gpus all` (or equivalent). Harmless on hosts
# without NVIDIA drivers. /opt/rocm/lib is appended for images that
# bundled the ROCm backend (the conditional install above also wires it
# into ld.so.conf.d, but exporting it here covers callers that bypass
# the dynamic linker cache, e.g. dlopen with absolute paths).
ENV LD_LIBRARY_PATH=/usr/local/nvidia/lib:/usr/local/nvidia/lib64:/opt/rocm/lib \
    NVIDIA_DRIVER_CAPABILITIES=compute,utility \
    NVIDIA_VISIBLE_DEVICES=all \
    PATH=/opt/rocm/bin:${PATH}

# Persistence is the caller's responsibility: mount a host directory or a
# named volume on /kronk to keep models, catalog, keys, libraries, and
# badger across container restarts (every example in this file does so
# with `-v kronk-data:/kronk`). We deliberately do NOT declare a
# `VOLUME ["/kronk/models"]` here because Docker would then create an
# anonymous volume mounted on /kronk/models even when the caller already
# mounted something on the parent /kronk, shadowing the models subdir
# inside their named volume — and `docker run --rm` would delete that
# anonymous volume on container exit, silently throwing away every
# downloaded model.

# 11435: public V1 API (chat, embeddings, models, /v1/liveness, ...).
# 11445: debug server (Prometheus /metrics, pprof, statsviz) — exposed so
#        operators can wire up dashboards and profilers out of the box.
#        Bind to 127.0.0.1 inside the container by overriding
#        KRONK_WEB_DEBUG_HOST if you want to keep it local-only.
EXPOSE 11435 11445

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD curl -fsS http://127.0.0.1:11435/v1/liveness || exit 1

ENTRYPOINT ["/usr/local/bin/kronk"]
CMD ["server", "start", "--base-path", "/kronk"]

# =============================================================================
# Stage 4 — runtime-jetson (optional)
#
# Alternative runtime image for NVIDIA Jetson Orin / Xavier devices
# running JetPack 6 (L4T r36.x). Ubuntu 24.04 cannot be used as the
# base because the Jetson kernel ships an older glibc/CUDA pairing and
# the NVIDIA Container Runtime injects Tegra-specific driver libraries
# (`libcuda.so` from /usr/lib/aarch64-linux-gnu/tegra/) that are
# ABI-tied to the L4T release. Using NVIDIA's official L4T CUDA
# runtime image as the base guarantees that match.
#
# Selected via `--target=runtime-jetson` and built natively on the
# Jetson (or cross-built with `--platform=linux/arm64` on a host with
# QEMU). LLAMA_PROCESSORS should be `cuda` — the libs-fetcher will
# pull the linux/arm64/cuda llama.cpp bundle which links against the
# CUDA runtime that L4T provides.
#
# This stage intentionally omits mesa-vulkan-drivers / vulkan-tools —
# Jetson's Vulkan ICD is the proprietary Tegra driver injected by the
# container runtime, not Mesa, and shipping mesa-vulkan-drivers would
# cause the loader to discover a duplicate (non-functional) RADV ICD
# alongside the real Tegra one.
# =============================================================================

FROM nvcr.io/nvidia/l4t-cuda:12.6.11-runtime AS runtime-jetson

ARG KRONK_VERSION

LABEL org.opencontainers.image.source="https://github.com/ardanlabs/kronk" \
      org.opencontainers.image.title="kronk" \
      org.opencontainers.image.description="Local LLM inference SDK and server CLI (Jetson L4T runtime)" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.vendor="Ardan Labs" \
      org.opencontainers.image.version="${KRONK_VERSION}" \
      org.opencontainers.image.url="https://github.com/ardanlabs/kronk" \
      org.opencontainers.image.documentation="https://github.com/ardanlabs/kronk#readme"

ENV DEBIAN_FRONTEND=noninteractive

# L4T r36 ships Ubuntu 22.04 (jammy) — its bundled libstdc++6 is older
# than what the llama.cpp prebuilt CUDA bundle expects. Add the test
# toolchain PPA and upgrade libstdc++6 to satisfy the GLIBCXX symbol
# requirements (the same workaround yzma's INSTALL.md documents for
# Jetson Vulkan installs).
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        gnupg \
        software-properties-common \
    && add-apt-repository -y ppa:ubuntu-toolchain-r/test \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
        libcurl4 \
        libgomp1 \
        libnuma1 \
        libopenblas0 \
        libstdc++6 \
        libvulkan1 \
        tzdata \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# `render` group for /dev/dri/renderD* on Jetson (matches the host GID).
RUN groupadd --system --gid 110 render || true

# kronk binary.
COPY --from=builder /out/kronk /usr/local/bin/kronk

# Pre-baked llama.cpp arm64/cuda bundle (libs-fetcher honours
# TARGETARCH so this resolves to /kronk/libraries/linux/arm64/cuda/).
COPY --from=libs-fetcher /kronk/libraries /kronk/libraries

# Default model config — overridable by bind-mounting another file at the
# path referenced by KRONK_POOL_MODEL_CONFIG_FILE below.
COPY zarf/kms/model_config.yaml /etc/kronk/model_config.yaml

RUN mkdir -p /kronk/models /kronk/catalog /kronk/keys

# -----------------------------------------------------------------------------
# Runtime configuration
# -----------------------------------------------------------------------------
#
# KRONK_PROCESSOR is hard-set to `cuda` so the server skips
# auto-detection (`nvidia-smi` is shimmed by the L4T container runtime
# but is not always present, and `tegrastats` is the canonical Jetson
# probe — neither matches the Linux DetectGPU() switch).
ENV KRONK_BASE_PATH=/kronk \
    KRONK_WEB_API_HOST=0.0.0.0:11435 \
    KRONK_DOWNLOAD_ENABLED=true \
    KRONK_INSECURE_LOGGING=false \
    KRONK_POOL_MODEL_CONFIG_FILE=/etc/kronk/model_config.yaml \
    KRONK_PROCESSOR=cuda

# NVIDIA Container Runtime hints. On Jetson the toolkit injects Tegra
# driver libraries from /usr/lib/aarch64-linux-gnu/tegra/ — adding that
# path to LD_LIBRARY_PATH makes them discoverable to dlopen call sites
# that bypass the dynamic linker cache.
ENV LD_LIBRARY_PATH=/usr/lib/aarch64-linux-gnu/tegra:/usr/local/cuda/lib64 \
    NVIDIA_DRIVER_CAPABILITIES=compute,utility,graphics \
    NVIDIA_VISIBLE_DEVICES=all

# Persistence is the caller's responsibility (see the same note in the
# default `runtime` stage above).

EXPOSE 11435 11445

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD curl -fsS http://127.0.0.1:11435/v1/liveness || exit 1

ENTRYPOINT ["/usr/local/bin/kronk"]
CMD ["server", "start", "--base-path", "/kronk"]
