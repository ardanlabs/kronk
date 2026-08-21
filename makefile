# ==============================================================================
# Shell Configuration
#
# Use ash in Alpine images and otherwise default to Bash. On Windows/MSYS2,
# derive bash.exe from the default sh.exe path. On Unix, use `which` to support
# environments such as NixOS, where Bash may live outside /bin.
ifeq ($(OS),Windows_NT)
    SHELL := $(subst sh.exe,bash.exe,$(SHELL))
else
    SHELL := $(if $(wildcard /bin/ash),/bin/ash,$(shell which bash 2>/dev/null || echo /bin/sh))
endif

# ==============================================================================
# Class Setup
#
# 1. Install the required Go and project tooling:
#
#      make install-gotooling
#      make install-tooling
#
# 2. Initialize the frontend:
#
#      make bui-install
#
# 3. Download the models used in class:
#
#      make install-class-models
#
# 4. Build and start the model server:
#
#      make kronk-server-build
#
#    Open http://localhost:11435 and navigate to Apps/Chat. Clear the session
#    before switching models.
#
# Model Validation
#
# Start with `unsloth/Qwen3-0.6B-Q8_0`, the smallest text model. Ask it a simple question,
# such as how to write a Hello World program in Go.
#
# For vision, start with `unsloth/Qwen3.5-0.8B-Q8_0`. Upload
# `examples/samples/giraffe.jpg` and ask the model what it sees.
#
# Now try the audio model `ggml-org/Qwen2.5-Omni-3B-Q8_0`. There is a wav file under the
# examples folder (examples/samples/jfk.wav). Select that wav file and ask the
# model what it hears.
#
# Hopefully all the models work for you, but again don't worry if the model
# server panics. Just send me an email (bill@ardanlabs.com) and I will try
# to help you.
#
# You can also try running the examples such as:
#
# 	make example-question
#	make example-agent
#	make example-vision
#	make example-audio
#
# Hardware Notes
#
# Memory is usually the primary constraint. On Apple Silicon, select models that
# use no more than roughly 80% of total system memory. On systems with separate
# CPU and GPU memory, all GPU memory is available; apply the same 80% guideline
# if any part of the model runs on the CPU.
#
# A GPU is strongly recommended because these models are not designed to run
# efficiently on a CPU alone. The class is still useful if only some models run.
#
# Platform Notes
#
# macOS on Apple Silicon is the most extensively tested platform. Linux users
# may need to install GPU drivers before class. Windows is supported but has
# received less testing.
#
# For help before class, email bill@ardanlabs.com.

# ==============================================================================
# Target Index
#
# Targets are grouped in topic-specific files under `.make/`:
#
#   .make/agents.mk     Default and rote agent-bundle management.
#   .make/cli.mk        Diagnostics, libraries, models, catalogs, and security.
#   .make/dev.mk        Linting, tests, benchmarks, dependencies, and generation.
#   .make/endpoints.mk  Health, inference, tokenization, and MCP requests.
#   .make/examples.mk   Runnable SDK and yzma examples.
#   .make/install.mk    Setup, tooling, libraries, models, and Docker.
#   .make/ops.mk        Open WebUI, Grafana, Statsviz, website, and debugging.
#   .make/server.mk     Browser UI, documentation, builds, and server lifecycle.
#   .make/tools.mk      MTP load and adversarial probes.

# ==============================================================================
# Includes

include .make/agents.mk
include .make/cli.mk
include .make/dev.mk
include .make/endpoints.mk
include .make/examples.mk
include .make/install.mk
include .make/ops.mk
include .make/server.mk
include .make/tools.mk
