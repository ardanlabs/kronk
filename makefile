# Check to see if we can use ash, in Alpine images, or default to BASH.
SHELL_PATH = /bin/ash
SHELL = $(if $(wildcard $(SHELL_PATH)),/bin/ash,/bin/bash)

# ==============================================================================
# Install

# Install the kronk cli.
install-kronk:
	@echo ========== INSTALL KRONK ==========
	go install ./cmd/kronk
	@echo

# Use this to install or update llama.cpp to the latest version. Needed to
# run tests locally.
install-libraries: install-kronk
	@echo ========== INSTALL LIBRARIES ==========
	go run cmd/kronk/main.go libs
	@echo

# Use this to install models. Needed to run tests locally.
install-models: install-kronk
	@echo ========== INSTALL MODELS ==========
	kronk pull "https://huggingface.co/ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/resolve/main/Qwen2.5-VL-3B-Instruct-Q8_0.gguf" "https://huggingface.co/ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/resolve/main/mmproj-Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
	@echo
	kronk pull "https://huggingface.co/unsloth/gpt-oss-20b-GGUF/resolve/main/gpt-oss-20b-Q8_0.gguf"
	@echo
	kronk pull "https://huggingface.co/mradermacher/Qwen2-Audio-7B-GGUF/resolve/main/Qwen2-Audio-7B.Q8_0.gguf"
	@echo
	kronk pull "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf"
	@echo
	kronk pull "https://huggingface.co/ggml-org/embeddinggemma-300m-qat-q8_0-GGUF/resolve/main/embeddinggemma-300m-qat-Q8_0.gguf"
	@echo

# Use this to see what devices are available on your machine. You need to
# install llama first.
llama-bench:
	$$HOME/kronk/libraries/llama-bench --list-devices

# Use this to rebuild tooling when new versions of Go are released.
install-gotooling:
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest

# ==============================================================================
# Kronk CLI

kronk-server:
	go run cmd/kronk/main.go server | go run cmd/kronk/website/api/tooling/logfmt/main.go

kronk-libs: install-libraries

# make kronk-pull URL="https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf"
kronk-pull:
	go run cmd/kronk/main.go pull "$(URL)"

kronk-list:
	go run cmd/kronk/main.go list

# make kronk-show FILE="Qwen3-8B-Q8_0.gguf"
kronk-show:
	go run cmd/kronk/main.go show "$(FILE)"

# ==============================================================================
# Tests

test: install-libraries install-models
	@echo ========== RUN TESTS ==========
	export GOROUTINES=1 && \
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
	CGO_ENABLED=0 go run examples/audio/main.go

example-chat:
	CGO_ENABLED=0 go run examples/chat/main.go

example-embedding:
	CGO_ENABLED=0 go run examples/embedding/main.go

example-question:
	CGO_ENABLED=0 go run examples/question/main.go

example-rerank:
	CGO_ENABLED=0 go run examples/rerank/main.go

example-vision:
	CGO_ENABLED=0 go run examples/vision/main.go

example-web:
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

# ==============================================================================
# Kronk Endpoints

curl-liveness:
	curl -i http://localhost:3000/v1/liveness

curl-readiness:
	curl -i http://localhost:3000/v1/readiness

curl-libs:
	curl -i http://localhost:3000/v1/mngt/libs

curl-model-list:
	curl -i http://localhost:3000/v1/mngt/model/list

curl-kronk-pull:
	curl -i -X POST http://localhost:3000/v1/mngt/model/pull \
	-d '{ \
		"model_url": "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf" \
	}'
