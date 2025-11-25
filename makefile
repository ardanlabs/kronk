# Check to see if we can use ash, in Alpine images, or default to BASH.
SHELL_PATH = /bin/ash
SHELL = $(if $(wildcard $(SHELL_PATH)),/bin/ash,/bin/bash)

# ==============================================================================
# Install

# Use this to install or update llamacpp to the latest version. Needed to
# run tests locally.
install-llamacpp:
	go run cmd/installer/main.go

# Use this to install models. Needed to run tests locally.
install-models:
	mkdir -p models
	curl -Lo models/qwen2.5-0.5b-instruct-q8_0.gguf "https://huggingface.co/Qwen/Qwen2.5-0.5B-Instruct-GGUF/resolve/main/qwen2.5-0.5b-instruct-q8_0.gguf?download=true"
	curl -Lo models/Qwen3-8B-Q8_0.gguf "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf?download=true"
	curl -Lo models/gpt-oss-20b-Q8_0.gguf "https://huggingface.co/unsloth/gpt-oss-20b-GGUF/resolve/main/gpt-oss-20b-Q8_0.gguf?download=true"
	curl -Lo models/Qwen2.5-VL-3B-Instruct-Q8_0.gguf "https://huggingface.co/ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/resolve/main/Qwen2.5-VL-3B-Instruct-Q8_0.gguf?download=true"
	curl -Lo models/mmproj-Qwen2.5-VL-3B-Instruct-Q8_0.gguf "https://huggingface.co/ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/resolve/main/mmproj-Qwen2.5-VL-3B-Instruct-Q8_0.gguf?download=true"
	curl -Lo models/embeddinggemma-300m-qat-Q8_0.gguf "https://huggingface.co/ggml-org/embeddinggemma-300m-qat-q8_0-GGUF/resolve/main/embeddinggemma-300m-qat-Q8_0.gguf?download=true"

# Use this to see what devices are available on your machine. You need to
# install llama first.
llama-bench:
	libraries/llama-bench --list-devices

# ==============================================================================
# Tests

test:
	export LD_LIBRARY_PATH=libraries && \
	export CONCURRENCY=1 && \
	export RUN_MACOS=1 && \
	export INSTALL_LLAMA=1 && \
	CGO_ENABLED=0 go test -v -count=1

test-upgrade: deps-upgrade
	export LD_LIBRARY_PATH=libraries && \
	export CONCURRENCY=3 && \
	export INSTALL_LLAMA=1 && \
	CGO_ENABLED=0 go test -v -count=1

# ==============================================================================
# Go Modules support

tidy:
	go mod tidy

deps-upgrade:
	go get -u -v ./...
	go mod tidy
