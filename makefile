# Check to see if we can use ash, in Alpine images, or default to BASH.
# Uses `which` to find bash on PATH for environments like NixOS where
# bash lives in the Nix store rather than /bin/bash.
SHELL := $(if $(wildcard /bin/ash),/bin/ash,$(shell which bash 2>/dev/null || echo /bin/sh))

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
#	Open browser to: http://localhost:8080
#
#	Navigate to Apps/Chat to go to the chat application. Make sure you clear
#	the session when trying different models.
#
#	Choose the `LFM2-700M-Q8_0` model first since it's the smallest. Ask it
#	a simple question like, write a hello world program in Go. If that works try
#	the other 3 models (`LFM2-700M-UD-Q8_K_XL`, `Qwen3-8B-Q8_0` and `gpt-oss-20b-Q8_0`)
#	and ask the same question. Do not be alarmed if the model server panics. It
#	just means you can't run that model. Just make a note of the models that work
#	and don't.
#
#	Now try the smallest vision model `LFM2.5-VL-1.6B-Q8_0`. There is an image
#	of a giraffe under the examples folder (examples/samples/giraffe.jpg). Select
#	that image and ask the model what it sees. If that works try the larger
#	vision model `Qwen2.5-VL-3B-Instruct-Q8_0`.
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
#	machine. As an example, the smallest model we will try is `LFM2-700M-Q8_0`
#	at 639 MB. That means you need at least 767 MB of memory on the machine to
#	run this model. The largest model we will use is `gpt-oss-20b-Q8_0` and that
#	model is 12.1 GB. That means you need at least 15 GB on the machine to run
#	this model.
#
#	LFM2-700M-Q8_0
#	Model Size: 639 MB
#	Overhead (20%): 639 MB x 0.20 = 128 MB
#	Total Required: 639 MB + 128 MB = 767 MB
#
#	gpt-oss-20b-Q8_0
#	Model Size: 12.1 GB
#	Overhead (20%): 12.1 GB x 0.20 = 2.42 GB
#	Total Required: 12.1 GB + 2.42 GB = 14.52 GB
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

install-tooling:
	brew list protobuf || brew install protobuf
	brew list grpcurl || brew install grpcurl
	brew list node || brew install node

# Install the kronk cli.
install-kronk:
	@echo ========== INSTALL KRONK ==========
	go install ./cmd/kronk
	@echo

# Use this to install or update llama.cpp to the latest version. Needed to
# run tests locally.
install-libraries:
	@echo ========== INSTALL LIBRARIES ==========
	go run cmd/kronk/main.go libs --local
	@echo

# Use this to install the test models.
install-test-models: install-kronk
	@echo ========== INSTALL MODELS ==========
	kronk model pull --local "ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/Qwen2.5-VL-3B-Instruct-Q8_0.gguf" "ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/mmproj-Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
	@echo
	kronk model pull --local "unsloth/gpt-oss-20b-GGUF/gpt-oss-20b-Q8_0.gguf"
	@echo
	kronk model pull --local "mradermacher/Qwen2-Audio-7B-GGUF/Qwen2-Audio-7B.Q8_0.gguf" "mradermacher/Qwen2-Audio-7B-GGUF/Qwen2-Audio-7B.mmproj-Q8_0.gguf"
	@echo
	kronk model pull --local "Qwen/Qwen3-8B-GGUF/Qwen3-8B-Q8_0.gguf"
	@echo
	kronk model pull --local "ggml-org/embeddinggemma-300m-qat-q8_0-GGUF/embeddinggemma-300m-qat-Q8_0.gguf"
	@echo
	kronk model pull --local "gpustack/bge-reranker-v2-m3-GGUF/bge-reranker-v2-m3-Q8_0.gguf"
	@echo

# Use this to install models for the class.
install-class-models: install-kronk
	@echo ========== INSTALL MODELS ==========
	kronk model pull --local "unsloth/LFM2.5-VL-1.6B-GGUF/LFM2.5-VL-1.6B-Q8_0.gguf" "unsloth/LFM2.5-VL-1.6B-GGUF/mmproj-F16.gguf"
	@echo
	kronk model pull --local "ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/Qwen2.5-VL-3B-Instruct-Q8_0.gguf" "ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/mmproj-Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
	@echo

	kronk model pull --local "mradermacher/Qwen2-Audio-7B-GGUF/Qwen2-Audio-7B.Q8_0.gguf" "mradermacher/Qwen2-Audio-7B-GGUF/Qwen2-Audio-7B.mmproj-Q8_0.gguf"
	@echo

	kronk model pull --local "unsloth/LFM2-700M-GGUF/LFM2-700M-Q8_0.gguf"
	@echo
	kronk model pull --local "unsloth/LFM2-700M-GGUF/LFM2-700M-UD-Q8_K_XL.gguf"
	@echo
	kronk model pull --local "Qwen/Qwen3-8B-GGUF/Qwen3-8B-Q8_0.gguf"
	@echo
	kronk model pull --local "unsloth/gpt-oss-20b-GGUF/gpt-oss-20b-Q8_0.gguf"
	@echo

	kronk model pull --local "ggml-org/embeddinggemma-300m-qat-q8_0-GGUF/embeddinggemma-300m-qat-Q8_0.gguf"
	@echo
	kronk model pull --local "gpustack/bge-reranker-v2-m3-GGUF/bge-reranker-v2-m3-Q8_0.gguf"
	@echo

OPENWEBUI  := ghcr.io/open-webui/open-webui:v0.7.2
GRAFANA    := grafana/grafana:12.3.0
PROMETHEUS := prom/prometheus:v3.8.0
TEMPO      := grafana/tempo:2.9.0
LOKI       := grafana/loki:3.6.0
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
	CGO_ENABLED=0 go vet ./...
	staticcheck -checks=all ./...

vuln-check:
	govulncheck ./...

diff:
	go fix -diff ./...

# Don't change the order of these tests. This order is solving a test
# build issue with time it takes to build the test binary due to building
# the binary with the libraries.
test-only: install-test-models
	@echo ========== RUN TESTS ==========
	export RUN_IN_PARALLEL=yes && \
	export GITHUB_WORKSPACE=$(shell pwd) && \
	CGO_ENABLED=0 go test -v -count=1 -p 1 ./sdk/kronk/tests/... && \
	CGO_ENABLED=0 go test -v -count=1 ./cmd/server/api/services/kronk/tests && \
	CGO_ENABLED=0 go test -v -count=1 ./cmd/server/app/sdk/cache && \
	CGO_ENABLED=0 go test -v -count=1 ./cmd/server/app/sdk/security/... && \
	CGO_ENABLED=0 go test -v -count=1 ./sdk/kronk/model && \
	CGO_ENABLED=0 go test -v -count=1 ./sdk/tools/...

test: test-only lint vuln-check diff

# ==============================================================================
# Benchmarks
#
# Model Type | Cache Mode | IMC Strategy      | Speculative | Target
# -----------|------------|-------------------|-------------|-------
# Dense      | NonCaching | —                 | No          | benchmark-dense-nc
# Dense      | SPC        | —                 | No          | benchmark-dense-spc
# Dense      | IMC        | Deterministic     | No          | benchmark-dense-imc-det
# Dense      | IMC        | Deterministic     | Yes         | benchmark-dense-imc-det-spec
# Dense      | IMC        | Non-Deterministic | No          | benchmark-dense-imc-nondet
# MoE        | IMC        | Deterministic     | No          | benchmark-moe-imc-det
# Hybrid     | IMC        | Deterministic     | No          | benchmark-hybrid-imc-det
# MoE        | IMC        | Deterministic     | No          | benchmark-moe-spec-baseline
# MoE        | IMC        | Deterministic     | Yes         | benchmark-moe-spec-draft

benchmark-dense-nc:
	CGO_ENABLED=0 go test -run=none -bench=BenchmarkDense_NonCaching -benchtime=3x -timeout=30m ./sdk/kronk/model/

benchmark-dense-spc:
	CGO_ENABLED=0 go test -run=none -bench=BenchmarkDense_SPC -benchtime=3x -timeout=30m ./sdk/kronk/model/

benchmark-dense-imc-det:
	CGO_ENABLED=0 go test -run=none -bench=BenchmarkDense_IMCDeterministic$$ -benchtime=3x -timeout=30m ./sdk/kronk/model/

benchmark-dense-imc-nondet:
	CGO_ENABLED=0 go test -run=none -bench=BenchmarkDense_IMCNonDeterministic -benchtime=3x -timeout=30m ./sdk/kronk/model/

benchmark-dense-imc-det-spec:
	CGO_ENABLED=0 go test -run=none -bench=BenchmarkDense_IMCDeterministic_Speculative -benchtime=3x -timeout=30m ./sdk/kronk/model/

benchmark-moe-imc-det:
	CGO_ENABLED=0 go test -run=none -bench=BenchmarkMoE_IMCDeterministic$$ -benchtime=3x -timeout=30m ./sdk/kronk/model/

benchmark-moe-spec-baseline:
	CGO_ENABLED=0 go test -run=none -bench=BenchmarkMoE_Speculative_Baseline -benchtime=3x -timeout=30m ./sdk/kronk/model/

benchmark-moe-spec-draft:
	CGO_ENABLED=0 go test -run=none -bench=BenchmarkMoE_Speculative_WithDraft -benchtime=3x -timeout=30m ./sdk/kronk/model/

benchmark-hybrid-imc-det:
	CGO_ENABLED=0 go test -run=none -bench=BenchmarkHybrid_IMCDeterministic -benchtime=3x -timeout=30m ./sdk/kronk/model/

# ==============================================================================
# Kronk BUI

BUI_DIR := cmd/server/api/frontends/bui

bui-install:
	cd $(BUI_DIR) && npm install

bui-run:
	cd $(BUI_DIR) && npm run dev

bui-build:
	cd $(BUI_DIR) && npm run build

bui-upgrade:
	cd $(BUI_DIR) && npm update

bui-upgrade-latest:
	cd $(BUI_DIR) && npx npm-check-updates -u && npm install

# ==============================================================================
# Kronk CLI

# CGO_ENABLED=1 go run -ldflags='-linkmode=external -extldflags "-Wl,-platform_version,macos,26.0,26.3"' examples/chat/main.go

kronk-build: kronk-docs bui-build

kronk-docs:
	go run cmd/server/api/tooling/docs/*.go

kronk-server:
	source .env 2>/dev/null || true && \
	export KRONK_INSECURE_LOGGING=true && \
	export KRONK_CATALOG_MODEL_CONFIG_FILE=zarf/kms/model_config.yaml && \
	export KRONK_CATALOG_REPO_PATH=$$HOME/code/go/src/github.com/ardanlabs/kronk_catalogs && \
	go run cmd/kronk/main.go server start | go run cmd/server/api/tooling/logfmt/main.go

kronk-server-build: kronk-build
	source .env 2>/dev/null || true && \
	export KRONK_INSECURE_LOGGING=true && \
	export KRONK_CATALOG_MODEL_CONFIG_FILE=zarf/kms/model_config.yaml && \
	export KRONK_CATALOG_REPO_PATH=$$HOME/code/go/src/github.com/ardanlabs/kronk_catalogs && \
	go run cmd/kronk/main.go server start | go run cmd/server/api/tooling/logfmt/main.go

kronk-server-download: kronk-build
	source .env 2>/dev/null || true && \
	export KRONK_DOWNLOAD_ENABLED=true && \
	export KRONK_INSECURE_LOGGING=true && \
	export KRONK_CATALOG_MODEL_CONFIG_FILE=zarf/kms/model_config.yaml && \
	export KRONK_CATALOG_REPO_PATH=$$HOME/code/go/src/github.com/ardanlabs/kronk_catalogs && \
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


# make kronk-model-pull URL="Qwen/Qwen3-8B-GGUF/Qwen3-8B-Q8_0.gguf"
kronk-model-pull:
	go run cmd/kronk/main.go model pull "$(URL)"

# make kronk-model-pull-local URL="Qwen/Qwen3-8B-GGUF/Qwen3-8B-Q8_0.gguf"
kronk-model-pull-local:
	go run cmd/kronk/main.go model pull --local "$(URL)"


kronk-model-ps:
	go run cmd/kronk/main.go model ps


# make kronk-model-remove ID="cerebras_qwen3-coder-reap-25b-a3b-q8_0"
kronk-model-remove:
	go run cmd/kronk/main.go model remove "$(ID)"

# make kronk-model-remove-local ID="cerebras_qwen3-coder-reap-25b-a3b-q8_0"
kronk-model-remove-local:
	go run cmd/kronk/main.go model remove --local "$(ID)"


# make kronk-model-show ID="Qwen3-8B-Q8_0"
kronk-model-show:
	go run cmd/kronk/main.go model show "$(ID)"

# make kronk-model-show-local ID="Qwen3-8B-Q8_0"
kronk-model-show-local:
	go run cmd/kronk/main.go model show --local "$(ID)"

# ------------------------------------------------------------------------------

kronk-catalog-update-local:
	go run cmd/kronk/main.go catalog update --local


kronk-catalog-list:
	go run cmd/kronk/main.go catalog list

kronk-catalog-list-local:
	go run cmd/kronk/main.go catalog list --local


# make kronk-catalog-show ID="Qwen3-8B-Q8_0"
kronk-catalog-show:
	go run cmd/kronk/main.go catalog show "$(ID)"

# make kronk-catalog-show-local ID="Qwen2.5-VL-3B-Instruct-Q8_0"
kronk-catalog-show-local:
	go run cmd/kronk/main.go catalog show --local "$(ID)"


# make kronk-catalog-pull ID="Qwen3-8B-Q8_0"
kronk-catalog-pull:
	go run cmd/kronk/main.go catalog pull "$(ID)"

# make kronk-catalog-pull-local ID="Qwen3-Coder-30B-A3B-Instruct-Q8_0"
kronk-catalog-pull-local:
	go run cmd/kronk/main.go catalog pull --local "$(ID)"

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

# make kronk-run ID="Qwen3-8B-Q8_0"
kronk-run:
	go run cmd/kronk/main.go run "$(ID)"

# ==============================================================================
# Kronk Endpoints

curl-liveness:
	curl -i -X GET http://localhost:8080/v1/liveness

curl-readiness:
	curl -i -X GET http://localhost:8080/v1/readiness

curl-libs:
	curl -i -X POST http://localhost:8080/v1/libs/pull

curl-model-list:
	curl -i -X GET http://localhost:8080/v1/models

curl-kronk-pull:
	curl -i -X POST http://localhost:8080/v1/models/pull \
	-d '{ \
		"model_url": "Qwen/Qwen3-8B-GGUF/Qwen3-8B-Q8_0.gguf" \
	}'

curl-kronk-remove:
	curl -i -X DELETE http://localhost:8080/v1/models/Qwen3-8B-Q8_0

curl-kronk-show:
	curl -i -X GET http://localhost:8080/v1/models/Qwen3-8B-Q8_0

curl-model-status:
	curl -i -X GET http://localhost:8080/v1/models/status

curl-kronk-chat:
	curl -i -X POST http://localhost:8080/v1/chat/completions \
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
		curl -i -X POST http://localhost:8080/v1/chat/completions \
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
	curl -i -X POST http://localhost:8080/v1/chat/completions \
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
	curl -i -X POST http://localhost:8080/v1/chat/completions \
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
	curl -i -X POST http://localhost:8080/v1/chat/completions \
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
	curl -i -X POST http://localhost:8080/v1/chat/completions \
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
	curl -i -X POST http://localhost:8080/v1/embeddings \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"model": "embeddinggemma-300m-qat-Q8_0", \
  		"input": "Why is the sky blue?" \
    }'

curl-kronk-rerank:
	curl -i -X POST http://localhost:8080/v1/rerank \
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
	curl -i -X POST http://localhost:8080/v1/responses \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"stream": true, \
	 	"model": "cerebras_qwen3-coder-reap-25b-a3b-q8_0", \
		"input": "Hello model" \
    }'

curl-kronk-responses-image:
	curl -i -X POST http://localhost:8080/v1/responses \
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
	curl -i -X POST http://localhost:8080/v1/chat/completions \
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
	curl -i -X POST http://localhost:8080/v1/tokenize \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"model": "Qwen3-8B-Q8_0", \
		"input": "The quick brown fox jumps over the lazy dog" \
    }'

curl-tokenize-template:
	curl -i -X POST http://localhost:8080/v1/tokenize \
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
# Download Service
#
# Start the server with download enabled:
#   make kronk-server-download
#
# Test downloading a model file (HEAD to check, GET to download):
#   make curl-download-head
#   make curl-download-get

# Check a model file exists and get its size.
# make curl-download-head FILE="bartowski/cerebras_Qwen3-Coder-REAP-25B-A3B-GGUF/resolve/main/cerebras_Qwen3-Coder-REAP-25B-A3B-Q8_0.gguf"
curl-download-head:
	curl -I http://localhost:8080/download/$(FILE)

# Download a model file.
# make curl-download-get FILE="bartowski/cerebras_Qwen3-Coder-REAP-25B-A3B-GGUF/resolve/main/cerebras_Qwen3-Coder-REAP-25B-A3B-Q8_0.gguf"
curl-download-get:
	curl -o /dev/null -w "HTTP %{http_code} - %{size_download} bytes\n" \
		http://localhost:8080/download/$(FILE)

# Download a sha file.
# make curl-download-sha FILE="bartowski/cerebras_Qwen3-Coder-REAP-25B-A3B-GGUF/raw/main/cerebras_Qwen3-Coder-REAP-25B-A3B-Q8_0.gguf"
curl-download-sha:
	curl http://localhost:8080/download/$(FILE)

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
	$(OPEN_CMD) http://localhost:8080/

statsviz:
	$(OPEN_CMD) http://localhost:8090/debug/statsviz

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

deps-upgrade: bui-upgrade
	go get -u -v ./...
	go mod tidy

yzma-latest:
	GOPROXY=direct go get github.com/hybridgroup/yzma@main

gonja-latest:
	GOPROXY=direct go get github.com/nikolalohinski/gonja/v2@master

# ==============================================================================
# Examples

example-audio:
	CGO_ENABLED=0 go run examples/audio/main.go

example-chat:
	CGO_ENABLED=0 go run examples/chat/main.go

example-embedding:
	CGO_ENABLED=0 go run examples/embedding/main.go

example-grammar:
	CGO_ENABLED=0 go run examples/grammar/main.go

example-rerank:
	CGO_ENABLED=0 go run examples/rerank/main.go

example-question:
	CGO_ENABLED=0 go run examples/question/main.go

example-response:
	CGO_ENABLED=0 go run examples/response/main.go

example-vision:
	CGO_ENABLED=0 go run examples/vision/main.go

# ------------------------------------------------------------------------------

example-yzma-step1:
	CGO_ENABLED=0 go run examples/yzma/step1/main.go

example-yzma-step2:
	CGO_ENABLED=0 go run examples/yzma/step2/main.go

example-yzma-step3:
	CGO_ENABLED=0 go run examples/yzma/step3/main.go

example-yzma-step4:
	CGO_ENABLED=0 go run examples/yzma/step4/main.go

example-yzma-step5:
	CGO_ENABLED=0 go run examples/yzma/step5/main.go

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
