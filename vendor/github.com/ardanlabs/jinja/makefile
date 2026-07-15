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
# Project setup

# `setup` installs the non-Go tools required to run the full test suite.
# Currently the only such tool is `uv`, which TestRequestsAgainstTemplates
# uses to invoke scripts/render_python.py and produce the canonical
# HuggingFace Jinja2 reference output to compare against.
#
# On macOS this uses Homebrew. On Linux/CI the GitHub workflow installs uv
# via the astral-sh/setup-uv action, so this target is mostly for local dev.
setup:
	@if command -v uv >/dev/null 2>&1; then \
		echo "uv already installed: $$(uv --version)"; \
	elif command -v brew >/dev/null 2>&1; then \
		echo "Installing uv via Homebrew..."; \
		brew install uv; \
	else \
		echo "Neither uv nor brew is installed."; \
		echo "Install uv from https://docs.astral.sh/uv/getting-started/installation/"; \
		exit 1; \
	fi

# ==============================================================================
# Go Modules support

tidy:
	go mod tidy

deps-upgrade: bui-upgrade
	go get -u -v ./...
	go mod tidy

# ==============================================================================
# Tests

lint:
	go vet ./...
	go tool staticcheck -checks=all ./...

vuln-check:
	go tool govulncheck ./...

diff:
	go fix -diff ./...

test-only:
	@echo ========== RUN TESTS ==========
	go test -v -count=1 ./...

test: test-only lint vuln-check diff

bench:
	go test -bench=. -benchmem -count=1 -benchtime=3s

# ==============================================================================
# Examples

example-basic:
	go run ./examples/basic/

example-chat:
	go run ./examples/chat/

example-toolcall:
	go run ./examples/toolcall/
