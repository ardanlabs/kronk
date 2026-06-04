# Check to see if we can use ash, in Alpine images, or default to BASH.
# On Windows/MSYS2, derive bash.exe from the default sh.exe path.
# On Unix, uses `which` to find bash for environments like NixOS where
# bash lives in the Nix store rather than /bin/bash.
ifeq ($(OS),Windows_NT)
    SHELL := $(subst sh.exe,bash.exe,$(SHELL))
else
    SHELL := $(if $(wildcard /bin/ash),/bin/ash,$(shell which bash 2>/dev/null || echo /bin/sh))
endif


# ==============================================================================
# Class Notes
#
# At this point you have cloned the project so we need to install a few things.
# 	make install-gotooling
#	make install-tooling
#
# Now let's get the frontend system initialized.
#	make bui-install
#
# Next we need to download the models for the class.
#	make install-class-models
#
# Let's test if these models are working by starting model server.
#	make kronk-server-build
#	Open browser to: http://localhost:11435
#
#	Navigate to Apps/Chat to go to the chat application. Make sure you clear
#	the session when trying different models.
#
#	Choose the `Qwen3-0.6B-Q8_0` model first since it's the smallest. Ask it
#	a simple question like, write a hello world program in Go. If that works try
#	the other 3 models (`LFM2-700M-Q8_0`, `Qwen3-8B-Q8_0` and `gpt-oss-20b-Q8_0`)
#	and ask the same question. Do not be alarmed if the model server panics. It
#	just means you can't run that model. Just make a note of the models that work
#	and don't.
#
#	Now try the smallest vision model `Qwen3.5-0.8B-Q8_0`. There is an image
#	of a giraffe under the examples folder (examples/samples/giraffe.jpg). Select
#	that image and ask the model what it sees. If that works try the two larger
#	vision model `LFM2.5-VL-1.6B-Q8_0` and `Qwen2.5-VL-3B-Instruct-Q8_0`.
#
#	Now try the audio model `Qwen2.5-Omni-3B-Q8_0`. There is a wav file under the
#	examples folder (examples/samples/jfk.wav). Select that wav file and ask the
#	model what it hears.
#
#	Hopefully all the models work for you, but again don't worry if the model
#	server panics. Just send me an email (bill@ardanlabs.com) and I will try
#	to help you.
#
# Memory
#	This is going to be your first biggest obstacle. You basically won't be able
#	to use a model that is larger than 80% of the total memory you have on the
#	machine if you are using Apple Silicon. For systems that have separate CPU
#   and GPU memory, you are free to use all of the GPU memory, but if some of the
#   model will run on CPU, I like the 80% rule again.
#
# GPU
#	This is going to be your second biggest obstacle. These models are not
#	designed to run at any level of performance on CPU alone. Without a GPU,
#	I'm not sure how things will run. Don't stress if you can run everything in
#	the class, you will still learn a lot.
#
# Operating Systems
#	I've been testing mostly on a MacBook Pro M4. If you have a Mac I feel pretty
#	good things should work. Llama.cpp is good at recognizing the Mac and the
#	GPU that exists.
#
#	If you are running Linux, you most likely will need to download drivers for
#	your GPU. You need to talk to me before you come to class so I can try to
#	help you.
#
#	If you are on Windows, we have tested the code will run but not extensively.
#	We will have to learn in class as we go.
#
# Having Problems
#	You need to email me (bill@ardanlabs.com) if you are running into problems
#	and need help.
#
# ------------------------------------------------------------------------------
# Where to find things
#
# Targets are split across topic-specific files under `make/`. Open the one
# that matches what you're trying to do:
#
#   .make/install.mk      setup, install-gotooling/tooling/kronk,
#                        install-libraries, install-*-models, install-docker
#
#   .make/dev.mk          lint, vuln-check, diff, test, test-gh, llama-bench,
#                        authapp-proto-gen, benchmark-*, tidy, deps-upgrade,
#                        yzma-latest
#
#   .make/server.mk       bui-install/run/build/upgrade, kronk-build,
#                        kronk-docs, kronk-server, kronk-server-build,
#                        kronk-server-detach/-logs/-stop
#
#   .make/cli.mk          kronk-libs, kronk-model-*, kronk-catalog-*,
#                        kronk-security-*, kronk-run, bucky-libs,
#                        bucky-model-*
#
#   .make/endpoints.mk    curl-liveness/readiness, curl-kronk-* (chat,
#                        embeddings, rerank, responses, tools, tokenize),
#                        mcp-server, curl-mcp-*
#
#   .make/ops.mk          owu-*, grafana-*, statsviz, website,
#                        debug-responses-*, debug-completions-*
#
#   .make/examples.mk     example-* (agent, audio, bucky, chat, ...) and
#                        example-yzma-step1..6, example-yzma-parallel-*
#
#   .make/agents.mk       agents-default-*, agents-rote-*, agents-wipe

include .make/install.mk
include .make/dev.mk
include .make/server.mk
include .make/cli.mk
include .make/endpoints.mk
include .make/ops.mk
include .make/examples.mk
include .make/agents.mk
