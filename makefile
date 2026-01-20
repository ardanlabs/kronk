# Check to see if we can use ash, in Alpine images, or default to BASH.
SHELL_PATH = /bin/ash
SHELL = $(if $(wildcard $(SHELL_PATH)),/bin/ash,/bin/bash)

UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
	OPEN_CMD := open
else
	OPEN_CMD := xdg-open
endif

# ==============================================================================
# Setup

# Configure git to use project hooks so pre-commit runs for all developers.
setup:
	git config core.hooksPath .githooks

# ==============================================================================
# Install

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

# Use this to install models. Needed to run tests locally.
install-models: install-kronk
	@echo ========== INSTALL MODELS ==========
	kronk model pull --local "https://huggingface.co/ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/resolve/main/Qwen2.5-VL-3B-Instruct-Q8_0.gguf" "https://huggingface.co/ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/resolve/main/mmproj-Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
	@echo
	kronk model pull --local "https://huggingface.co/unsloth/gpt-oss-20b-GGUF/resolve/main/gpt-oss-20b-Q8_0.gguf"
	@echo
	kronk model pull --local "https://huggingface.co/mradermacher/Qwen2-Audio-7B-GGUF/resolve/main/Qwen2-Audio-7B.Q8_0.gguf" "https://huggingface.co/mradermacher/Qwen2-Audio-7B-GGUF/resolve/main/Qwen2-Audio-7B.mmproj-Q8_0.gguf"
	@echo
	kronk model pull --local "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf"
	@echo
	kronk model pull --local "https://huggingface.co/ggml-org/embeddinggemma-300m-qat-q8_0-GGUF/resolve/main/embeddinggemma-300m-qat-Q8_0.gguf"
	@echo
	kronk model pull --local "https://huggingface.co/gpustack/bge-reranker-v2-m3-GGUF/resolve/main/bge-reranker-v2-m3-Q8_0.gguf"
	@echo

# Use this to see what devices are available on your machine. You need to
# install llama first.
llama-bench:
	$$HOME/.kronk/libraries/llama-bench --list-devices

# Use this to rebuild tooling when https://files.slack.com/files-pri/T032G0ZL4-F0A8991CEJV/download/chat-export-1767998185593.json?origin_team=T032G0ZL4new versions of Go are released.
install-gotooling:
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

install-tooling:
	brew list protobuf || brew install protobuf
	brew list grpcurl || brew install grpcurl

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

# Don't change the order of these tests. This order is solving a test
# build issue with time it takes to build the test binary due to building
# the binary with the libraries.
test-only: install-models
	@echo ========== RUN TESTS ==========
	export RUN_IN_PARALLEL=yes && \
	export GITHUB_WORKSPACE=$(shell pwd) && \
	CGO_ENABLED=0 go test -v -count=1 ./cmd/server/api/services/kronk/tests && \
	CGO_ENABLED=0 go test -v -count=1 ./sdk/kronk/tests && \
	CGO_ENABLED=0 go test -v -count=1 ./cmd/server/app/sdk/cache && \
	CGO_ENABLED=0 go test -v -count=1 ./cmd/server/app/sdk/security/... && \
	CGO_ENABLED=0 go test -v -count=1 ./sdk/kronk/model && \
	CGO_ENABLED=0 go test -v -count=1 ./sdk/tools/...

test: test-only lint vuln-check

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

kronk-build: kronk-docs bui-build

kronk-docs:
	go run cmd/server/api/tooling/docs/*.go

kronk-server: kronk-build
	export KRONK_CACHE_MODEL_CONFIG_FILE=zarf/kms/model_config.yaml && \
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


# make kronk-model-pull URL="https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf"
kronk-model-pull:
	go run cmd/kronk/main.go model pull "$(URL)"

# make kronk-model-pull-local URL="https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf"
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


# make kronk-model-show ID="qwen3-8b-q8_0"
kronk-model-show:
	go run cmd/kronk/main.go model show "$(ID)"

# make kronk-model-show-local ID="qwen3-8b-q8_0"
kronk-model-show-local:
	go run cmd/kronk/main.go model show --local "$(ID)"

# ------------------------------------------------------------------------------

kronk-catalog-update-local:
	go run cmd/kronk/main.go catalog update --local


kronk-catalog-list:
	go run cmd/kronk/main.go catalog list

kronk-catalog-list-local:
	go run cmd/kronk/main.go catalog list --local


# make kronk-catalog-show ID="qwen3-8b-q8_0"
kronk-catalog-show:
	go run cmd/kronk/main.go catalog show "$(ID)"

# make kronk-catalog-show-local ID="qwen3-8b-q8_0"
kronk-catalog-show-local:
	go run cmd/kronk/main.go catalog show --local "$(ID)"


# make kronk-catalog-pull ID="qwen3-8b-q8_0"
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

# make kronk-run ID="cerebras_qwen3-coder-reap-25b-a3b-q8_0"
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
		"model_url": "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf" \
	}'

curl-kronk-remove:
	curl -i -X DELETE http://localhost:8080/v1/models/qwen3-8b-q8_0

curl-kronk-show:
	curl -i -X GET http://localhost:8080/v1/models/qwen3-8b-q8_0

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
	 	"model": "cerebras_Qwen3-Coder-REAP-25B-A3B-Q8_0", \
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

# ==============================================================================
# Examples

example-audio:
	CGO_ENABLED=0 go run examples/audio/main.go

example-chat:
	CGO_ENABLED=0 go run examples/chat/main.go

example-embedding:
	CGO_ENABLED=0 go run examples/embedding/main.go

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
