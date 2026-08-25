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

test-only: install-libraries-gh install-test-models
	@echo ========== RUN TESTS ==========
	# Unset KRONK_* path overrides so a developer's shell environment
	# (e.g. KRONK_BASE_PATH=/data/kronk) cannot leak into the suite —
	# defaults.BaseDir consults KRONK_BASE_PATH when no override is
	# supplied, and several SDK tests rely on the $HOME/.kronk default.
	unset KRONK_BASE_PATH KRONK_LIB_PATH KRONK_BUCKY_LIB_PATH KRONK_MALINA_LIB_PATH MALINA_LIB KRONK_PROCESSOR KRONK_ARCH KRONK_OS && \
	export RUN_IN_PARALLEL=yes && \
	export GITHUB_WORKSPACE=$(shell pwd) && \
	go test -v -p=1 -count=1 ./cmd/server/... && \
	go test -v -p=1 -count=1 ./sdk/...

test: test-only lint vuln-check diff

test-gh-only: install-libraries-gh install-test-gh-models
	@echo ========== RUN GH ONLY TESTS ==========
	unset KRONK_BASE_PATH KRONK_LIB_PATH KRONK_BUCKY_LIB_PATH KRONK_PROCESSOR KRONK_ARCH KRONK_OS && \
	export RUN_IN_PARALLEL=no && \
	export GITHUB_WORKSPACE=$(shell pwd) && \
	export GITHUB_ACTIONS=true && \
	go test -v -p=1 -count=1 ./cmd/kronk/... && \
	go test -v -p=1 -count=1 ./cmd/server/... && \
	go test -v -p=1 -count=1 $$(go list ./sdk/... | grep -v '/sdk/kronk/tests') && \
	go test -v -count=1 -timeout 6m -run '^TestSuite/ThinkChat$$' ./sdk/kronk/tests/qwen3 && \
	go test -v -count=1 -timeout 6m -run '^TestLengthTerminatedToolCallBecomesContent$$' ./sdk/kronk/tests/qwen06 && \
	go test -v -count=1 -timeout 6m -run '^TestSuite$$' ./sdk/kronk/tests/draft && \
	go test -v -count=1 -timeout 6m -run '^TestSuite/SimpleMedia$$' ./sdk/kronk/tests/vision && \
	go test -v -count=1 -timeout 6m -run '^TestSuite$$' ./sdk/kronk/tests/rerank && \
	go test -v -count=1 -timeout 6m -run '^TestSuite/ThinkChat$$' ./sdk/kronk/tests/hybrid

test-gh: test-gh-only lint vuln-check diff

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

build-deps-upgrade: deps-upgrade
	./zarf/docker/kronk/upgrade-pins.sh

yzma-latest:
	GOPROXY=direct go get github.com/hybridgroup/yzma@main
