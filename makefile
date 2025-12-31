# Check to see if we can use ash, in Alpine images, or default to BASH.
SHELL_PATH = /bin/ash
SHELL = $(if $(wildcard $(SHELL_PATH)),/bin/ash,/bin/bash)

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

# Use this to see what devices are available on your machine. You need to
# install llama first.
llama-bench:
	$$HOME/.kronk/libraries/llama-bench --list-devices

# Use this to rebuild tooling when new versions of Go are released.
install-gotooling:
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/divan/expvarmon@latest

install-tooling:
	brew list protobuf || brew install protobuf
	brew list grpcurl || brew install grpcurl

# ==============================================================================
# Protobuf support

authapp-proto-gen:
	protoc --go_out=cmd/server/app/domain/authapp --go_opt=paths=source_relative \
		--go-grpc_out=cmd/server/app/domain/authapp --go-grpc_opt=paths=source_relative \
		--proto_path=cmd/server/app/domain/authapp \
		cmd/server/app/domain/authapp/authapp.proto

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

# make kronk-catalog-pull-local ID="qwen3-8b-q8_0"
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
	 	"stream": true, \
	 	"model": "qwen3-8b-q8_0", \
		"messages": [ \
			{ \
				"role": "user", \
				"content": "Hello model" \
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

curl-kronk-embeddings:
	curl -i -X POST http://localhost:8080/v1/embeddings \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"model": "embeddinggemma-300m-qat-Q8_0", \
  		"input": "Why is the sky blue?" \
    }'

# ==============================================================================
# Running OpenWebUI 

UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
	OPEN_CMD := open
else
	OPEN_CMD := xdg-open
endif

install-owu:
	docker pull ghcr.io/open-webui/open-webui:v0.6.41

owu-up:
	docker compose -f zarf/docker/compose.yaml up openwebui

owu-down:
	docker compose -f zarf/docker/compose.yaml down openwebui

owu-browse:
	$(OPEN_CMD) http://localhost:8081/

# ==============================================================================
# Tests

lint:
	CGO_ENABLED=0 go vet ./...
	staticcheck -checks=all ./...

vuln-check:
	govulncheck ./...

test-only: install-libraries install-models
	@echo ========== RUN TESTS ==========
	export GOROUTINES=1 && \
	export RUN_IN_PARALLEL=1 && \
	export GITHUB_WORKSPACE=$(shell pwd) && \
	CGO_ENABLED=0 go test -v -count=1 ./sdk/security/... && \
	CGO_ENABLED=0 go test -v -count=1 ./sdk/tools/... && \
	CGO_ENABLED=0 go test -v -count=1 ./sdk/kronk/... && \
	CGO_ENABLED=0 go test -v -count=1 ./cmd/server/app/sdk/... && \
	CGO_ENABLED=0 go test -v -count=1 ./cmd/server/api/services/kronk/tests

test: test-only lint vuln-check

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

metrics-view:
	expvarmon -ports="localhost:8090" -vars="service_goroutines,service_requests,service_errors,service_panics,model_load_avg,model_prompt_creation_avg,model_prefill_nonmedia_avg,model_ttft_avg,mem:memstats.HeapAlloc,mem:memstats.HeapSys,mem:memstats.Sys"

grafana:
	$(OPEN_CMD) http://localhost:3100/

statsviz:
	$(OPEN_CMD) http://localhost:8090/debug/statsviz

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

example-question:
	CGO_ENABLED=0 go run examples/question/main.go

example-vision:
	CGO_ENABLED=0 go run examples/vision/main.go