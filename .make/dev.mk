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
	# Unset path and CI-mode overrides so a developer's shell environment
	# (e.g. KRONK_BASE_PATH=/data/kronk or KRONK_TEST_HOSTED=1) cannot leak into the suite —
	# defaults.BaseDir consults KRONK_BASE_PATH when no override is
	# supplied, and several SDK tests rely on the $HOME/.kronk default.
	unset KRONK_TEST_HOSTED KRONK_BASE_PATH KRONK_LIB_PATH KRONK_BUCKY_LIB_PATH KRONK_MALINA_LIB_PATH MALINA_LIB KRONK_PROCESSOR KRONK_ARCH KRONK_OS && \
	export RUN_IN_PARALLEL=yes && \
	export GITHUB_WORKSPACE=$(shell pwd) && \
	go test -v -p=1 -count=1 ./cmd/kronk/... && \
	go test -v -p=1 -count=1 ./cmd/server/... && \
	go test -v -p=1 -count=1 ./sdk/... && \
	go -C examples test -v -p=1 -count=1 ./...

test: test-only lint vuln-check diff

test-gh-only: install-libraries install-test-gh-models
	@echo ========== RUN GH ONLY TESTS ==========
	unset KRONK_BASE_PATH KRONK_LIB_PATH KRONK_BUCKY_LIB_PATH KRONK_PROCESSOR KRONK_ARCH KRONK_OS && \
	export RUN_IN_PARALLEL=no && \
	export GITHUB_WORKSPACE=$(shell pwd) && \
	go test -v -p=1 -count=1 ./cmd/kronk/... && \
	go test -v -p=1 -count=1 ./cmd/server/... && \
	go test -v -p=1 -count=1 $$(go list ./sdk/... | grep -v '/sdk/kronk/tests') && \
	go test -v -count=1 -timeout 20m ./sdk/kronk/tests/qwen3 && \
	go test -v -count=1 -timeout 6m -run '^TestLengthTerminatedToolCallBecomesContent$$' ./sdk/kronk/tests/qwen06 && \
	go test -v -count=1 -timeout 6m -run '^TestSuite$$' ./sdk/kronk/tests/draft && \
	go test -v -count=1 -timeout 6m -run '^TestSuite/SimpleMedia$$' ./sdk/kronk/tests/vision && \
	go test -v -count=1 -timeout 6m -run '^TestSuite$$' ./sdk/kronk/tests/vision_imc && \
	go test -v -count=1 -timeout 6m -run '^(TestSuite|TestConcurrentEmbeddings)$$' ./sdk/kronk/tests/embed && \
	go test -v -count=1 -timeout 6m -run '^TestSuite$$' ./sdk/kronk/tests/rerank && \
	go test -v -count=1 -timeout 20m ./sdk/kronk/tests/hybrid && \
	go test -v -count=1 -timeout 6m -run '^TestSuite$$' ./sdk/kronk/tests/hybrid_vision_imc

test-gh: test-gh-only lint vuln-check diff

# Run the native Malina SDK and model-server integration tests explicitly.
# These tests require stable-diffusion.cpp and the sd-1.5 model, and are not
# part of the ordinary local or GitHub test suites.
test-malina: install-test-malina
	@echo ========== RUN MALINA INTEGRATION TESTS ==========
	unset KRONK_TEST_HOSTED KRONK_BASE_PATH KRONK_LIB_PATH KRONK_BUCKY_LIB_PATH KRONK_MALINA_LIB_PATH MALINA_LIB KRONK_PROCESSOR KRONK_ARCH KRONK_OS && \
	export RUN_IN_PARALLEL=no && \
	export GITHUB_WORKSPACE=$(shell pwd) && \
	go test -v -p=1 -count=1 -timeout 20m -tags=malina_integration -run '^TestMalinaModelInference$$' ./sdk/malina && \
	go test -v -p=1 -count=1 -timeout 20m -tags=malina_integration -run '^TestImageGenerationModel$$' ./cmd/server/app/domain/imageapp

# Run the native OpenVINO diagnostic and inference probe explicitly. This test
# downloads the pinned llama.cpp bundle and small diagnostic model, and is not
# part of the ordinary local or GitHub test suites. OPENVINO_DEVICE selects the
# OpenVINO target: CPU, GPU, GPU.<index>, or NPU.
OPENVINO_DEVICE ?= CPU
test-openvino:
	@platform="$$(go env GOOS)/$$(go env GOARCH)"; \
	case "$$platform" in \
		linux/amd64|windows/amd64) ;; \
		*) echo "test-openvino requires linux/amd64 or windows/amd64; got $$platform" >&2; exit 1 ;; \
	esac
	@case "$(OPENVINO_DEVICE)" in \
		CPU|GPU|NPU|GPU.[0-9]*) ;; \
		*) echo "OPENVINO_DEVICE must be CPU, GPU, GPU.<index>, or NPU" >&2; exit 1 ;; \
	esac
	@echo ========== RUN OPENVINO INTEGRATION TEST device[$(OPENVINO_DEVICE)] ==========
	unset KRONK_LIB_PATH KRONK_ARCH KRONK_OS && \
	export KRONK_PROCESSOR=openvino && \
	export GGML_OPENVINO_DEVICE=$(OPENVINO_DEVICE) && \
	export RUN_IN_PARALLEL=no && \
	export GITHUB_WORKSPACE=$(shell pwd) && \
	go test -v -p=1 -count=1 -timeout 20m -tags=openvino_integration -run '^TestOpenVINOInference$$' ./sdk/tools/diagnose

# ==============================================================================
# Go Modules support

tidy:
	go mod tidy
	cd examples && go mod tidy

deps-upgrade: bui-upgrade
	go get -u -v ./...
	go get github.com/hybridgroup/yzma@main
	go mod tidy
	cd examples && go get -u -v ./...
	cd examples && go get github.com/hybridgroup/yzma@main
	cd examples && go mod tidy

build-deps-upgrade: deps-upgrade
	./zarf/docker/kronk/upgrade-pins.sh

yzma-latest:
	GOPROXY=direct go get github.com/hybridgroup/yzma@main
