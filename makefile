# Check to see if we can use ash, in Alpine images, or default to BASH.
SHELL_PATH = /bin/ash
SHELL = $(if $(wildcard $(SHELL_PATH)),/bin/ash,/bin/bash)

# ==============================================================================
# Install

# Use this to install or update llama.cpp to the latest version. Needed to
# run tests locally.
install-llamacpp:
	go run cmd/installer/main.go

# Use this to install models. Needed to run tests locally.
install-models:
	mkdir -p tests/models
	curl -Lo tests/models/Qwen3-8B-Q8_0.gguf "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf"
	curl -Lo tests/models/gpt-oss-20b-Q8_0.gguf "https://huggingface.co/unsloth/gpt-oss-20b-GGUF/resolve/main/gpt-oss-20b-Q8_0.gguf"
	curl -Lo tests/models/Qwen2.5-VL-3B-Instruct-Q8_0.gguf "https://huggingface.co/ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/resolve/main/Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
	curl -Lo tests/models/mmproj-Qwen2.5-VL-3B-Instruct-Q8_0.gguf "https://huggingface.co/ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/resolve/main/mmproj-Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
	curl -Lo tests/models/embeddinggemma-300m-qat-Q8_0.gguf "https://huggingface.co/ggml-org/embeddinggemma-300m-qm-q8_0-GGUF/resolve/main/embeddinggemma-300m-qat-Q8_0.gguf"
	curl -Lo tests/models/Qwen2-Audio-7B.Q8_0.gguf "https://huggingface.co/mradermacher/Qwen2-Audio-7B-GGUF/resolve/main/Qwen2-Audio-7B.Q8_0.gguf"
	curl -Lo tests/models/Qwen2-Audio-7B.mmproj-Q8_0.gguf "https://huggingface.co/mradermacher/Qwen2-Audio-7B-GGUF/resolve/main/Qwen2-Audio-7B.mmproj-Q8_0.gguf"

# Use this to see what devices are available on your machine. You need to
# install llama first.
llama-bench:
	libraries/llama-bench --list-devices

# Use this to rebuild tooling when new versions of Go are released.
dev-gotooling:
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest

# ==============================================================================
# Server

kronk-build:
	go build -o bin/kronk cmd/kronk/main.go

# make kronk-pull URL="https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf"
kronk-pull:
	go run cmd/kronk/main.go pull "$(URL)"

# ==============================================================================
# Tests

test:
	export LD_LIBRARY_PATH=tests/libraries && \
	export GOROUTINES=1 && \
	export INSTALL_LLAMA=1 && \
	export RUN_IN_PARALLEL=1 && \
	export GITHUB_WORKSPACE=$(shell pwd) && \
	CGO_ENABLED=0 go test -v -count=1 ./tests

# ==============================================================================
# Go Modules support

tidy:
	go mod tidy

deps-upgrade:
	go get -u -v ./...
	go mod tidy

yzma-latest:
	GOPROXY=direct go get github.com/hybridgroup/yzma@main

# ==============================================================================
# Examples

example-audio:
	export LD_LIBRARY_PATH=${LD_LIBRARY_PATH}:tests/libraries && \
	CGO_ENABLED=0 go run examples/audio/main.go

example-chat:
	export LD_LIBRARY_PATH=${LD_LIBRARY_PATH}:tests/libraries && \
	CGO_ENABLED=0 go run examples/chat/main.go

example-embedding:
	export LD_LIBRARY_PATH=${LD_LIBRARY_PATH}:tests/libraries && \
	CGO_ENABLED=0 go run examples/embedding/main.go

example-question:
	export LD_LIBRARY_PATH=${LD_LIBRARY_PATH}:tests/libraries && \
	CGO_ENABLED=0 go run examples/question/main.go

example-rerank:
	export LD_LIBRARY_PATH=${LD_LIBRARY_PATH}:tests/libraries && \
	CGO_ENABLED=0 go run examples/rerank/main.go

example-vision:
	export LD_LIBRARY_PATH=${LD_LIBRARY_PATH}:tests/libraries && \
	CGO_ENABLED=0 go run examples/vision/main.go

example-web:
	export LD_LIBRARY_PATH=${LD_LIBRARY_PATH}:tests/libraries && \
	CGO_ENABLED=0 go run examples/web/main.go

example-web-curl1:
	curl -i -X POST http://0.0.0.0:8080/chat \
     -H "Content-Type: application/json" \
     -d '{ \
		"messages": [ \
			{ \
				"role": "user", \
				"content": "How do you declare an interface in Go?" \
			} \
		] \
    }'

example-web-curl2:
	curl -i -X POST http://0.0.0.0:8080/chat \
     -H "Content-Type: application/json" \
     -d '{ \
		"messages": [ \
			{ \
				"role": "user", \
				"content": "What is the weather in London, England?" \
			} \
		] \
    }'