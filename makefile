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
#	Now try the audio model `Qwen2-Audio-7B.Q8_0`. There is a wav file under the
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

# ==============================================================================
# Setup

# Configure git to use project hooks so pre-commit runs for all developers.
setup:
	git config core.hooksPath .githooks

# ==============================================================================
# Install

install-gotooling:
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/nix-community/gomod2nix@latest

install-tooling:
	brew list protobuf || brew install protobuf
	brew list grpcurl || brew install grpcurl
	brew list node || brew install node
	brew list rg || brew install rg

# Install the kronk cli.
install-kronk:
	@echo ========== INSTALL KRONK ==========
	go install ./cmd/kronk
	@echo

# Use this to install or update llama.cpp to the latest version. Used by
# the local `make test` target so developers exercise the newest llama.cpp
# bundle before bumping the well-known defaultVersion in
# sdk/tools/libs/libs.go for a release.
install-libraries: install-kronk
	@echo "========== INSTALL LIBRARIES (latest) =========="
	kronk libs --local --upgrade
	@echo

# Use this to install the well-known defaultVersion of llama.cpp baked into
# the SDK. This mirrors what CI does so `make test-gh` reproduces the GH
# workflow locally. Bumping defaultVersion in sdk/tools/libs/libs.go is what
# rolls both this target and the CI workflow forward.
install-libraries-gh: install-kronk
	@echo "========== INSTALL LIBRARIES (defaultVersion) =========="
	kronk libs --local
	@echo

# Use this to install the test GH models.
install-test-gh-models: install-kronk
	@echo ========== INSTALL MODELS ==========
	kronk model pull --local "unsloth/Qwen3.5-0.8B-Q8_0"
	@echo
	kronk model pull --local "Qwen/Qwen3-8B-Q8_0"
	@echo
	kronk model pull --local "ggml-org/embeddinggemma-300m-qat-Q8_0"
	@echo
	kronk model pull --local "gpustack/bge-reranker-v2-m3-Q8_0"
	@echo

# Use this to install the test models.
install-test-models: install-kronk
	@echo ========== INSTALL MODELS ==========
	kronk model pull --local "unsloth/Qwen3.5-0.8B-Q8_0"
	@echo
	kronk model pull --local "unsloth/gemma-4-26B-A4B-it-UD-Q4_K_M"
	@echo
	kronk model pull --local "unsloth/Qwen3.6-35B-A3B-UD-Q4_K_M"
	@echo
	kronk model pull --local "mradermacher/Qwen2-Audio-7B.Q8_0"
	@echo
	kronk model pull --local "unsloth/gpt-oss-20b-Q8_0"
	@echo
	kronk model pull --local "Qwen/Qwen3-8B-Q8_0"
	@echo
	kronk model pull --local "ggml-org/embeddinggemma-300m-qat-Q8_0"
	@echo
	kronk model pull --local "gpustack/bge-reranker-v2-m3-Q8_0"
	@echo

# Use this to install models for the class.
install-class-models: install-kronk
	@echo ========== INSTALL MODELS ==========
	kronk model pull --local "unsloth/Qwen3.5-0.8B-Q8_0"
	@echo
	kronk model pull --local "unsloth/LFM2.5-VL-1.6B-Q8_0"
	@echo
	kronk model pull --local "mradermacher/Qwen2-Audio-7B.Q8_0"
	@echo
	kronk model pull --local "unsloth/Qwen3-0.6B-Q8_0"
	@echo
	kronk model pull --local "unsloth/LFM2-700M-Q8_0"
	@echo
	kronk model pull --local "Qwen/Qwen3-8B-Q8_0"
	@echo
	kronk model pull --local "unsloth/gpt-oss-20b-Q8_0"
	@echo
	kronk model pull --local "ggml-org/embeddinggemma-300m-qat-Q8_0"
	@echo
	kronk model pull --local "gpustack/bge-reranker-v2-m3-Q8_0"
	@echo

OPENWEBUI  := ghcr.io/open-webui/open-webui:v0.9.2
GRAFANA    := grafana/grafana:12.3.1
PROMETHEUS := prom/prometheus:v3.11.0
TEMPO      := grafana/tempo:2.10.0
LOKI       := grafana/loki:3.7.0
PROMTAIL   := grafana/promtail:3.6.0

# Install the docker images.
install-docker:
	docker pull docker.io/$(OPENWEBUI) & \
	docker pull docker.io/$(GRAFANA) & \
	docker pull docker.io/$(PROMETHEUS) & \
	docker pull docker.io/$(TEMPO) & \
	docker pull docker.io/$(LOKI) & \
	docker pull docker.io/$(PROMTAIL) & \
	wait;

# ==============================================================================
# Llama.cpp programs

# Use this to see what devices are available on your machine. You need to
# install llama first.
llama-bench:
	$$HOME/.kronk/libraries/llama-bench --list-devices

# ==============================================================================
# Protobuf support

authapp-proto-gen:
	protoc --go_out=cmd/server/app/domain/authapp --go_opt=paths=source_relative \
		--go-grpc_out=cmd/server/app/domain/authapp --go-grpc_opt=paths=source_relative \
		--proto_path=cmd/server/app/domain/authapp \
		cmd/server/app/domain/authapp/authapp.proto

# ==============================================================================
# Tests

lint:
	go vet ./...
	staticcheck -checks=all ./...

vuln-check:
	govulncheck ./...

diff:
	go fix -diff ./...

test-only: install-libraries install-test-models
	@echo ========== RUN TESTS ==========
	export RUN_IN_PARALLEL=yes && \
	export GITHUB_WORKSPACE=$(shell pwd) && \
	go test -v -p=1 -count=1 ./cmd/server/... && \
	go test -v -p=1 -count=1 ./sdk/...

test: test-only lint vuln-check diff

test-gh-only: install-libraries-gh install-test-gh-models
	@echo ========== RUN GH ONLY TESTS ==========
	export RUN_IN_PARALLEL=yes && \
	export GITHUB_WORKSPACE=$(shell pwd) && \
	export GITHUB_ACTIONS=true && \
	go test -v -p=1 -count=1 ./cmd/server/... && \
	go test -v -p=1 -count=1 $(go list ./sdk/... | grep -v '/sdk/kronk/tests')

test-gh: test-gh-only lint vuln-check diff

# ==============================================================================
# Benchmarks

benchmark-dense-nc:
	go test -run=none -bench=BenchmarkDense_NonCaching -benchtime=3x -timeout=30m ./sdk/kronk/tests/benchmarks/

benchmark-dense-imc:
	go test -run=none -bench=BenchmarkDense_IMC -benchtime=3x -timeout=30m ./sdk/kronk/tests/benchmarks/

benchmark-moe-nc:
	go test -run=none -bench=BenchmarkMoE_NonCaching -benchtime=3x -timeout=30m ./sdk/kronk/tests/benchmarks/

benchmark-moe-imc:
	go test -run=none -bench=BenchmarkMoE_IMC -benchtime=3x -timeout=30m ./sdk/kronk/tests/benchmarks/

benchmark-hybrid-nc:
	go test -run=none -bench=BenchmarkHybrid_NonCaching -benchtime=3x -timeout=30m ./sdk/kronk/tests/benchmarks/

benchmark-hybrid-imc:
	go test -run=none -bench=BenchmarkHybrid_IMC -benchtime=3x -timeout=30m ./sdk/kronk/tests/benchmarks/

# Run all benchmarks sequentially (each target loads/unloads its own model)
# and write combined raw output to a single file under runs/.
# Usage: make benchmark-all BENCH_KRONK=v1.20.4
BENCH_KRONK ?= dev

benchmark-all:
	@FILE=sdk/kronk/tests/benchmarks/runs/$$(date +%Y-%m-%d).txt; \
	mkdir -p sdk/kronk/tests/benchmarks/runs; \
	echo "# Date: $$(date +%Y-%m-%d)" > $$FILE; \
	echo "# Kronk: $(BENCH_KRONK)" >> $$FILE; \
	echo "" >> $$FILE; \
	for target in \
		benchmark-dense-nc \
		benchmark-dense-imc \
		benchmark-moe-nc \
		benchmark-moe-imc \
		benchmark-hybrid-nc \
		benchmark-hybrid-imc; \
	do \
		echo "" >> $$FILE; \
		echo "## $$target" >> $$FILE; \
		$(MAKE) $$target 2>&1 | tee -a $$FILE; \
	done; \
	echo ""; \
	echo "Results written to $$FILE"

# Format benchmark results from runs/ into BENCH_RESULTS.txt.
benchmark-fmt:
	go run cmd/server/api/tooling/benchfmt/main.go

# Append a single run file to the top of BENCH_RESULTS.txt with diffs.
# Usage: make benchmark-fmt-file FILE=2026-03-01.txt
benchmark-fmt-file:
	go run cmd/server/api/tooling/benchfmt/main.go $(FILE)

# ==============================================================================
# Kronk BUI

BUI_DIR := cmd/server/api/frontends/bui

bui-install:
	cd $(BUI_DIR) && npm install

bui-run: kronk-docs
	cd $(BUI_DIR) && npm run dev

bui-build:
	cd $(BUI_DIR) && npm run build

bui-upgrade:
	cd $(BUI_DIR) && npm update

bui-upgrade-latest:
	cd $(BUI_DIR) && npx npm-check-updates -u && npm install

# ==============================================================================
# Kronk CLI

kronk-build: kronk-docs bui-build

kronk-docs:
	go run cmd/server/api/tooling/docs/*.go

# export KRONK_LIB_VERSION=b9163 && \

kronk-server:
	. .env 2>/dev/null || true && \
	export KRONK_DOWNLOAD_ENABLED=true && \
	export KRONK_LIB_VERSION=b9163 && \
	export KRONK_INSECURE_LOGGING=true && \
	export KRONK_POOL_MODEL_CONFIG_FILE=zarf/kms/model_config.yaml && \
	go run cmd/kronk/main.go server start | go run cmd/server/api/tooling/logfmt/main.go

kronk-server-build: kronk-build
	. .env 2>/dev/null || true && \
	export KRONK_DOWNLOAD_ENABLED=true && \
	export KRONK_LIB_VERSION=b9163 && \
	export KRONK_INSECURE_LOGGING=true && \
	export KRONK_POOL_MODEL_CONFIG_FILE=zarf/kms/model_config.yaml && \
	go run cmd/kronk/main.go server start | go run cmd/server/api/tooling/logfmt/main.go

kronk-server-detach: bui-build
	go run cmd/kronk/main.go server start --detach

kronk-server-logs:
	go run cmd/kronk/main.go server logs

kronk-server-stop:
	go run cmd/kronk/main.go server stop

# ------------------------------------------------------------------------------

kronk-libs:
	go run cmd/kronk/main.go libs

kronk-libs-local: install-libraries

# ------------------------------------------------------------------------------

kronk-model-index:
	go run cmd/kronk/main.go model index

kronk-model-index-local:
	go run cmd/kronk/main.go model index --local


kronk-model-list:
	go run cmd/kronk/main.go model list

kronk-model-list-local:
	go run cmd/kronk/main.go model list --local


# make kronk-model-pull URL="Qwen/Qwen3-8B-Q8_0.gguf"
kronk-model-pull:
	go run cmd/kronk/main.go model pull "$(URL)"

# make kronk-model-pull-local URL="Qwen/Qwen3-8B-Q8_0.gguf"
kronk-model-pull-local:
	go run cmd/kronk/main.go model pull --local "$(URL)"


kronk-model-ps:
	go run cmd/kronk/main.go model ps


# make kronk-model-remove ID="bartowski/cerebras_qwen3-coder-reap-25b-a3b-q8_0"
kronk-model-remove:
	go run cmd/kronk/main.go model remove "$(ID)"

# make kronk-model-remove-local ID="bartowski/cerebras_qwen3-coder-reap-25b-a3b-q8_0"
kronk-model-remove-local:
	go run cmd/kronk/main.go model remove --local "$(ID)"


# make kronk-model-show ID="Qwen/Qwen3-8B-Q8_0"
kronk-model-show:
	go run cmd/kronk/main.go model show "$(ID)"

# make kronk-model-show-local ID="Qwen/Qwen3-8B-Q8_0"
kronk-model-show-local:
	go run cmd/kronk/main.go model show --local "$(ID)"

# ------------------------------------------------------------------------------

kronk-catalog-list:
	go run cmd/kronk/main.go catalog list

kronk-catalog-list-local:
	go run cmd/kronk/main.go catalog list --local


# make kronk-catalog-show ID="Qwen/Qwen3-8B-Q8_0"
kronk-catalog-show:
	go run cmd/kronk/main.go catalog show "$(ID)"

# make kronk-catalog-show-local ID="Qwen/Qwen3-8B-Q8_0"
kronk-catalog-show-local:
	go run cmd/kronk/main.go catalog show --local "$(ID)"


# ------------------------------------------------------------------------------

kronk-security-help:
	go run cmd/kronk/main.go security --help


kronk-security-key-list:
	go run cmd/kronk/main.go security key list

kronk-security-key-list-local:
	go run cmd/kronk/main.go security key list --local

# make kronk-security-token-create-local U="bill" D="5m" E="chat-completions"
kronk-security-token-create-local:
	go run cmd/kronk/main.go security token create --local --username "$(U)" --duration "$(D)" --endpoints "$(E)"

# ------------------------------------------------------------------------------

# make kronk-run ID="Qwen/Qwen3-8B-Q8_0"
kronk-run:
	go run cmd/kronk/main.go run "$(ID)"

# ==============================================================================
# Kronk Endpoints

curl-liveness:
	curl -i -X GET http://localhost:11435/v1/liveness

curl-readiness:
	curl -i -X GET http://localhost:11435/v1/readiness

curl-kronk-chat:
	curl -i -X POST http://localhost:11435/v1/chat/completions \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"model": "gpt-oss-20b-Q8_0", \
		"stream": true, \
		"messages": [ \
			{ \
				"role": "user", \
				"content": "Hello model" \
			} \
		] \
    }'

curl-kronk-chat-load:
	for i in {1..3}; do \
		curl -i -X POST http://localhost:11435/v1/chat/completions \
		-H "Authorization: Bearer ${KRONK_TOKEN}" \
		-H "Content-Type: application/json" \
		-d '{ \
			"model": "gpt-oss-20b-Q8_0", \
			"stream": true, \
			"messages": [ \
				{ \
					"role": "user", \
					"content": "Hello model" \
				} \
			] \
		}' & \
	done; wait

FILE_GIRAFFE := $(shell base64 < examples/samples/giraffe.jpg)

curl-kronk-chat-image:
	curl -i -X POST http://localhost:11435/v1/chat/completions \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"stream": true, \
	 	"model": "Qwen2.5-VL-3B-Instruct-Q8_0", \
		"messages": [ \
			{ \
				"role": "user", \
				"content": "What is in this image?" \
			}, \
			{ \
				"role": "user", \
				"content": "$(FILE_GIRAFFE)" \
			} \
		] \
    }'

curl-kronk-chat-openai-image:
	curl -i -X POST http://localhost:11435/v1/chat/completions \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"stream": true, \
	 	"model": "Qwen2.5-VL-3B-Instruct-Q8_0", \
		"messages": [ \
			{ \
				"role": "user", \
				"content": [ \
					{"type": "text", "text": "What is in this image?"}, \
					{"type": "image_url", "image_url": {"url": "data:image/jpeg;base64,'$(FILE_GIRAFFE)'"}} \
				] \
			} \
		] \
    }'

curl-kronk-chat-gpt:
	curl -i -X POST http://localhost:11435/v1/chat/completions \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"stream": true, \
	 	"model": "gpt-oss-20b-Q8_0", \
		"messages": [ \
			{ \
				"role": "user", \
				"content": "Hello model" \
			} \
		] \
    }'

curl-kronk-chat-tool:
	curl -i -X POST http://localhost:11435/v1/chat/completions \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"model": "Qwen3-8B-Q8_0", \
		"stream": true, \
		"messages": [ \
			{ \
				"role": "user", \
				"content": "what is the weather in NYC" \
			} \
		], \
		"tool_selection": "auto", \
		"tools": [ \
			{ \
				"type": "function", \
				"function": { \
					"name": "get_weather", \
					"description": "Get the current weather for a location", \
					"parameters": { \
						"type": "object", \
						"properties": { \
							"location": { \
								"type": "string", \
								"description": "The location to get the weather for, e.g. San Francisco, CA" \
							} \
						}, \
						"required": ["location"] \
					} \
				} \
			} \
		] \
    }'

curl-kronk-embeddings:
	curl -i -X POST http://localhost:11435/v1/embeddings \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"model": "embeddinggemma-300m-qat-Q8_0", \
  		"input": "Why is the sky blue?" \
    }'

curl-kronk-rerank:
	curl -i -X POST http://localhost:11435/v1/rerank \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"model": "bge-reranker-v2-m3-Q8_0", \
  		"query": "What is the capital of France?", \
		"documents": [ \
			"Paris is the capital and largest city of France.", \
			"Berlin is the capital of Germany.", \
			"The Eiffel Tower is located in Paris.", \
			"London is the capital of England.", \
			"France is a country in Western Europe." \
		], \
		"top_n": 3, \
		"return_documents": true \
    }'

curl-kronk-responses:
	curl -i -X POST http://localhost:11435/v1/responses \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"stream": true, \
	 	"model": "cerebras_qwen3-coder-reap-25b-a3b-q8_0", \
		"input": "Hello model" \
    }'

curl-kronk-responses-image:
	curl -i -X POST http://localhost:11435/v1/responses \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"stream": true, \
	 	"model": "Qwen2.5-VL-3B-Instruct-Q8_0", \
		"input": [ \
			{ \
				"type": "input_text", \
				"text": "What is in this image?" \
			}, \
			{ \
				"type": "input_image", \
				"image_url": "data:image/jpeg;base64,'$(FILE_GIRAFFE)'" \
			} \
		] \
    }'

curl-kronk-tool-response:
	curl -i -X POST http://localhost:11435/v1/chat/completions \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
		"model": "Qwen3-8B-Q8_0", \
		"max_tokens": 32768, \
		"temperature": 0.1, \
		"top_p": 0.1, \
		"top_k": 50, \
		"messages": [ \
			{ \
				"role": "user", \
				"content": "What is the weather like in San Fran?" \
			}, \
			{ \
				"role": "assistant", \
				"tool_calls": [ \
					{ \
						"id": "76803ff7-339e-44c4-b51e-769c2b5fa68e", \
						"type": "function", \
						"function": { \
							"name": "tool_get_weather", \
							"arguments": "{\"location\":\"San Francisco\"}" \
						} \
					} \
				] \
			}, \
			{ \
				"role": "tool", \
				"tool_call_id": "76803ff7-339e-44c4-b51e-769c2b5fa68e", \
				"content": "{\"status\":\"SUCCESS\",\"data\":{\"description\":\"The weather in San Francisco, CA is hot and humid\\n\",\"humidity\":80,\"temperature\":28,\"wind_speed\":10}}" \
			} \
		], \
		"tool_selection": "auto", \
		"tools": [ \
			{ \
				"type": "function", \
				"function": { \
					"name": "tool_get_weather", \
					"description": "Get the current weather for a location", \
					"parameters": { \
						"type": "object", \
						"properties": { \
							"location": { \
								"type": "string", \
								"description": "The location to get the weather for, e.g. San Francisco, CA" \
							} \
						}, \
						"required": ["location"] \
					} \
				} \
			} \
		] \
	}'

curl-tokenize:
	curl -i -X POST http://localhost:11435/v1/tokenize \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"model": "Qwen3-8B-Q8_0", \
		"input": "The quick brown fox jumps over the lazy dog" \
    }'

curl-tokenize-template:
	curl -i -X POST http://localhost:11435/v1/tokenize \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"model": "Qwen3-8B-Q8_0", \
		"input": "The quick brown fox jumps over the lazy dog", \
		"apply_template": true \
    }'

# ==============================================================================
# MCP Service

# Start the standalone MCP server.
mcp-server:
	go run cmd/server/api/services/mcp/main.go

# Initialize the MCP session. The response includes the Mcp-Session-Id header
# needed for subsequent requests.
# Usage: make curl-mcp-init
curl-mcp-init:
	curl -i -X POST "http://localhost:9000/mcp" \
	-H "Content-Type: application/json" \
	-H "Accept: application/json, text/event-stream" \
	-d '{ \
		"jsonrpc": "2.0", \
		"id": 1, \
		"method": "initialize", \
		"params": { \
			"protocolVersion": "2025-03-26", \
			"capabilities": {}, \
			"clientInfo": {"name": "curl-client", "version": "1.0.0"} \
		} \
	}'

# Send the initialized notification.
# make curl-mcp-initialized SESSIONID=<Mcp-Session-Id-from-init>
curl-mcp-initialized:
	curl -X POST "http://localhost:9000/mcp" \
	-H "Content-Type: application/json" \
	-H "Accept: application/json, text/event-stream" \
	-H "Mcp-Session-Id: $(SESSIONID)" \
	-d '{ \
		"jsonrpc": "2.0", \
		"method": "notifications/initialized" \
	}'

# List available tools.
# make curl-mcp-tools-list SESSIONID=<Mcp-Session-Id-from-init>
curl-mcp-tools-list:
	curl -i -X POST "http://localhost:9000/mcp" \
	-H "Content-Type: application/json" \
	-H "Accept: application/json, text/event-stream" \
	-H "Mcp-Session-Id: $(SESSIONID)" \
	-d '{ \
		"jsonrpc": "2.0", \
		"id": 2, \
		"method": "tools/list", \
		"params": {} \
	}'

# Call the web_search tool.
# make curl-mcp-web-search SESSIONID=<Mcp-Session-Id-from-init>
curl-mcp-web-search:
	curl -i -X POST "http://localhost:9000/mcp" \
	-H "Content-Type: application/json" \
	-H "Accept: application/json, text/event-stream" \
	-H "Mcp-Session-Id: $(SESSIONID)" \
	-d '{ \
		"jsonrpc": "2.0", \
		"id": 3, \
		"method": "tools/call", \
		"params": { \
			"name": "web_search", \
			"arguments": {"query": "what is the Model Context Protocol", "count": 5} \
		} \
	}'

# ==============================================================================
# Running OpenWebUI 

owu-up:
	docker compose -f zarf/docker/compose.yaml up openwebui

owu-down:
	docker compose -f zarf/docker/compose.yaml down openwebui

owu-browse:
	$(OPEN_CMD) http://localhost:8081/

# ==============================================================================
# Metrics and Tracing

UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
	OPEN_CMD := open
else
	OPEN_CMD := xdg-open
endif

website:
	$(OPEN_CMD) http://localhost:11435/

statsviz:
	$(OPEN_CMD) http://localhost:11445/debug/statsviz

grafana-up:
	docker compose -f zarf/docker/compose.yaml up grafana loki prometheus promtail tempo

grafana-down:
	docker compose -f zarf/docker/compose.yaml down grafana loki prometheus promtail tempo

grafana-browse:
	$(OPEN_CMD) http://localhost:3100/

# ==============================================================================
# Go Modules support

tidy:
	go mod tidy
	cd examples && go mod tidy

deps-upgrade: bui-upgrade
	go get -u -v ./...
	go mod tidy
	cd examples && go get -u -v ./...
	cd examples && go mod tidy

yzma-latest:
	GOPROXY=direct go get github.com/hybridgroup/yzma@main

# ==============================================================================
# Examples

example-agent:
	cd examples && go run ./agent/...

example-audio:
	cd examples && go run ./audio/main.go

example-chat:
	cd examples && go run ./chat/main.go

example-concurrency:
	cd examples && go run ./concurrency/main.go

example-embedding:
	cd examples && go run ./embedding/main.go

example-grammar:
	cd examples && go run ./grammar/main.go

example-pool:
	cd examples && go run ./pool/main.go

example-rag:
	cd examples && go run ./rag/main.go

example-rerank:
	cd examples && go run ./rerank/main.go

example-question:
	cd examples && go run ./question/main.go

example-response:
	cd examples && go run ./response/main.go

example-vision:
	cd examples && go run ./vision/main.go

# ------------------------------------------------------------------------------

example-yzma-step1:
	cd examples && go run ./yzma/step1/main.go

example-yzma-step2:
	cd examples && go run ./yzma/step2/main.go

example-yzma-step3:
	cd examples && go run ./yzma/step3/main.go

example-yzma-step4:
	cd examples && go run ./yzma/step4/main.go

example-yzma-step5:
	cd examples && go run ./yzma/step5/main.go

example-yzma-step6:
	cd examples && go run ./yzma/step6/main.go

example-yzma-parallel-curl1:
	curl -X POST http://localhost:8090/v1/completions \
	-H "Content-Type: application/json" \
	-d '{"prompt": "Hello, how are you?", "max_tokens": 50}'

example-yzma-parallel-curl2:
	curl -X POST http://localhost:8090/v1/completions \
	-H "Content-Type: application/json" \
	-d '{"prompt": "Hello", "max_tokens": 50, "stream": true}'

example-yzma-parallel-curl3:
	curl http://localhost:8090/v1/stats

example-yzma-parallel-load:
	for i in {1..20}; do \
		curl -s -X POST http://localhost:8090/v1/completions \
		-H "Content-Type: application/json" \
		-d "{\"prompt\": \"Request $$i: Hello\", \"max_tokens\": 30}" & \
	done; wait

# ==============================================================================
# Debugging

debug-responses-qwen:
	curl -s http://localhost:11435/v1/responses -H "Content-Type: application/json" -d '{"model":"Qwen3.5-35B-A3B-UD-Q8_K_XL","stream":false,"instructions":"You are a helpful assistant.","input":"Create a file called test.txt","tools":[{"type":"function","name":"editor","description":"Create or edit files","parameters":{"type":"object","properties":{"path":{"type":"string"},"new_text":{"type":"string"}},"required":["path","new_text"]}}]}' | python3 -m json.tool

debug-responses-gemma:
	curl -s http://localhost:11435/v1/responses -H "Content-Type: application/json" -d '{"model":"gemma-4-26B-A4B-it-UD-Q8_K_XL","stream":false,"instructions":"You are a helpful assistant.","input":"Create a file called test.txt","tools":[{"type":"function","name":"editor","description":"Create or edit files","parameters":{"type":"object","properties":{"path":{"type":"string"},"new_text":{"type":"string"}},"required":["path","new_text"]}}]}' | python3 -m json.tool

debug-completions-qwen:
	curl -s http://localhost:11435/v1/chat/completions -H "Content-Type: application/json" -d '{"model":"gemma-4-26B-A4B-it-UD-Q8_K_XL","stream":false,"messages":[{"role":"system","content":"You are a helpful assistant."},{"role":"user","content":"Please edit this file `sdk/kronk/model/yzma.go` using the `tool_go_code_editor` tool and add a comment to the top that says \"BILL WAS HERE\"."}],"tools":[{"type":"function","function":{"name":"tool_go_code_editor","description":"Edit Golang source code files including adding, replacing, and deleting lines.","parameters":{"type":"object","properties":{"path":{"type":"string","description":"Relative path and name of the Golang file"},"line_number":{"type":"integer","description":"The line number for the code change"},"type_change":{"type":"string","description":"The type of change to make: add, replace, delete"},"line_change":{"type":"string","description":"The text to add, replace, delete"}},"required":["path","line_number","type_change","line_change"]}}}]}' | python3 -m json.tool

debug-completions-gemma:
	curl -s http://localhost:11434/v1/chat/completions -H "Content-Type: application/json" -d '{"model":"gemma4:26b","stream":false,"messages":[{"role":"system","content":"You are a helpful assistant."},{"role":"user","content":"Please edit this file `sdk/kronk/model/yzma.go` using the `tool_go_code_editor` tool and add a comment to the top that says \"BILL WAS HERE\"."}],"tools":[{"type":"function","function":{"name":"tool_go_code_editor","description":"Edit Golang source code files including adding, replacing, and deleting lines.","parameters":{"type":"object","properties":{"path":{"type":"string","description":"Relative path and name of the Golang file"},"line_number":{"type":"integer","description":"The line number for the code change"},"type_change":{"type":"string","description":"The type of change to make: add, replace, delete"},"line_change":{"type":"string","description":"The text to add, replace, delete"}},"required":["path","line_number","type_change","line_change"]}}}]}' | python3 -m json.tool

# ==============================================================================
# Agents — Default bundle
#
# Rote-free baseline. Host configs wire the Kronk MCP server directly
# into each host so agents can call `web_search` and `fuzzy_edit` over
# raw MCP. Most contributors use this.
#
# Note on `rm -rf … skills`: keeps the copy idempotent and also prunes
# any rote skill left behind from a previous `agents-rote-<host>` run
# before the default skill tree is laid down.

agents-default-opencode:
	mkdir -p $$HOME/.config/opencode
	cp .agents/default/opencode/opencode.jsonc $$HOME/.config/opencode/opencode.jsonc
	cp .agents/default/opencode/auth.json $$HOME/.config/opencode/auth.json
	cp .agents/default/AGENTS.md $$HOME/.config/opencode/AGENTS.md
	rm -rf $$HOME/.config/opencode/skills
	cp -r .agents/default/skills $$HOME/.config/opencode/skills

# ==============================================================================
# Agents — Rote bundle
#
# All targets related to the rote execution layer (https://www.modiqo.ai/).
# Full documentation: .agents/rote/NOTES.md.
#
# Rote is OPT-IN — none of these targets are pulled in by install-tooling
# or any default-bundle target. Standard order for opting into rote:
#
#   make agents-rote-install        # installs the rote CLI
#   make agents-rote-login          # one-time interactive registry login
#                                   # (browser flow). Persisted on disk under
#                                   # ~/.rote/, survives reboots; only needs
#                                   # re-running after a wipe or token expiry.
#   make agents-rote-seed           # seeds ~/.rote/ with the project's
#                                   # adapters, rebuilds the search index,
#                                   # and ensures the `playground`
#                                   # workspace exists.
#   make agents-rote-<host>         # ships the rote-aware bundle for
#                                   # the agent host you actually use.
#
# Per-host targets ship .agents/rote/<host>/* + the rote-aware AGENTS.md
# + the rote skill to the host's config directory.

# Install the rote CLI from the upstream installer. The script is idempotent
# (re-run upgrades the binary without touching ~/.rote/), so we only skip it
# when `rote` is already on PATH. The VS Code extension is NOT required —
# this gives you the same ~/.rote/ state the extension would.
agents-rote-install:
	@command -v rote >/dev/null 2>&1 \
		&& echo "rote already installed at $$(command -v rote)" \
		|| curl -fsSL https://getrote.dev/install | bash

# Run the rote registry login flow. Required after `agents-rote-install` on
# a fresh box (or after `rm -rf ~/.rote`) before `agents-rote-seed` will
# work — `rote init` (used by agents-rote-playground) refuses to run without
# a registry session. Login state persists on disk under ~/.rote/secrets/
# and ~/.rote/registry/, so this is one-time per machine until you wipe.
# Modiqo's registry is invite-only — see .agents/rote/NOTES.md §3.
agents-rote-login:
	@rote whoami 2>&1 | grep -q "Not logged in" \
		&& rote login \
		|| echo "rote already logged in"

# Internal: fail fast with a clear pointer when seed/playground are run
# without a registry session, instead of letting `rote init` emit its
# generic "rote requires login" error and a non-obvious make stack trace.
agents-rote-login-check:
	@rote whoami 2>&1 | grep -q "Not logged in" && { \
		echo "rote is not logged in — run \`make agents-rote-login\` first."; \
		echo "(invite-only registry; see .agents/rote/NOTES.md §3)"; \
		exit 1; \
	} || true

# Create the long-lived `playground` workspace used for ad-hoc exploration
# with the adapter. (Modiqo's docs sometimes call this a "canvas" — same
# thing as a workspace, see .agents/rote/NOTES.md §1.) `rote init` is NOT
# idempotent — running it twice on the same name exits 1 with a verbose
# error — so we guard with a directory existence check. See
# .agents/rote/NOTES.md §8 step 3 for why workspace creation is a make
# target rather than something agents do.
agents-rote-playground: agents-rote-login-check
	@if [ -d "$$HOME/.rote/rote/workspaces/playground" ]; then \
		echo "playground workspace already exists at $$HOME/.rote/rote/workspaces/playground"; \
	else \
		rote init playground --seq && echo "playground workspace created"; \
	fi

# Seed the user's ~/.rote/ tree with the project's rote artifacts (the kronk
# adapter so far). See .agents/rote/NOTES.md §6 for what lives in this mirror
# and why.
#
# What we mirror: manifest.json, tools.json, agent.md, config/, toolsets/.
# What we exclude: runtime/ (per-execution scratch), index/ (Tantivy search
# index — segment UUIDs change on every reindex, so committing them creates
# binary noise on every diff). The index is rebuilt locally with
# `rote adapter reindex` immediately after the rsync, producing a fully
# usable adapter from a single make invocation.
agents-rote-seed: agents-rote-playground
	mkdir -p $$HOME/.rote/adapters
	rsync -a \
		--exclude 'runtime/' \
		--exclude 'index/' \
		--exclude '.tantivy-*.lock' \
		.agents/rote/adapters/kronk/ $$HOME/.rote/adapters/kronk/
	rote adapter reindex kronk

agents-rote-opencode:
	mkdir -p $$HOME/.config/opencode
	cp .agents/rote/opencode/opencode.jsonc $$HOME/.config/opencode/opencode.jsonc
	cp .agents/rote/opencode/auth.json $$HOME/.config/opencode/auth.json
	cp .agents/rote/AGENTS.md $$HOME/.config/opencode/AGENTS.md
	rm -rf $$HOME/.config/opencode/skills
	cp -r .agents/rote/skills $$HOME/.config/opencode/skills


# ==============================================================================
# Agents — Wipe
#
# Nuke every trace of every agent bundle this makefile knows how to install,
# so the next `agents-default-<host>` or `agents-rote-<host>` runs against a
# clean box. Use this when you want to verify a bundle in isolation —
# without it you'd be testing one bundle layered over leftovers from the
# other (or from a previous install of the same one), which has bitten us
# before.
#
# What this removes (regardless of which bundle put it there):
#   1. ~/.rote/                                 — workspaces, adapters,
#                                                 secrets, registry session,
#                                                 runtime caches. Per
#                                                 .agents/rote/NOTES.md
#                                                 §Update/uninstall.
#   2. The `rote` binary on PATH                — installed by
#                                                 agents-rote-install.
#   3. Every host config dir we ever write to   — opencode, kilo, pi, goose.
#                                                 We blow away the whole
#                                                 directory (configs,
#                                                 sessions, threads, the lot)
#                                                 rather than cherry-picking
#                                                 files, so anything either
#                                                 bundle (default or rote)
#                                                 ever dropped is gone.
#
# Idempotent: every step uses `rm -f`/`rm -rf`, so re-running on an
# already-clean machine is a no-op.
agents-wipe:
	@echo "==> removing ~/.rote/"
	rm -rf $$HOME/.rote
	@echo "==> removing rote binary (if present)"
	@if command -v rote >/dev/null 2>&1; then \
		rm -f "$$(command -v rote)" && echo "removed $$(command -v rote 2>/dev/null || echo rote)"; \
	else \
		echo "rote binary not on PATH — skipping"; \
	fi
	@echo "==> removing opencode agent config"
	rm -rf $$HOME/.config/opencode
	@echo "==> removing kilo agent config"
	rm -rf $$HOME/.config/kilo
	@echo "==> removing pi agent config"
	rm -rf $$HOME/.pi
	@echo "==> removing goose agent config"
	rm -rf $$HOME/.config/goose
	@echo "==> done. machine is in a pre-install state; run agents-default-<host> or agents-rote-<host> to reinstall."
