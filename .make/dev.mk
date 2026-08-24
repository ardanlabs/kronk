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
# Benchmarks

# BENCH_PROFILE_FLAGS passes optional go test profiling flags to one model.
BENCH_PROFILE_FLAGS ?=
BENCH_CODEGEN_TIME ?= 3x
BENCH_CODEGEN_TIMEOUT ?= 45m

# Run the Ornith coding benchmark.
benchmark-codegen-ornith:
	RUN_IN_PARALLEL=yes GITHUB_WORKSPACE=$(shell pwd) go test -run='^$$' -bench='^BenchmarkCodeGen_Ornith15_35B_Q8$$' -benchtime=$(BENCH_CODEGEN_TIME) -timeout=$(BENCH_CODEGEN_TIMEOUT) $(BENCH_PROFILE_FLAGS) ./sdk/kronk/tests/benchmarks/

# Run the MTP Qwen3.6 coding benchmark.
benchmark-codegen-mtp-qwen36:
	RUN_IN_PARALLEL=yes GITHUB_WORKSPACE=$(shell pwd) go test -run='^$$' -bench='^BenchmarkCodeGen_MTP_Qwen36_35B_A3B_Q8$$' -benchtime=$(BENCH_CODEGEN_TIME) -timeout=$(BENCH_CODEGEN_TIMEOUT) $(BENCH_PROFILE_FLAGS) ./sdk/kronk/tests/benchmarks/

# Run the Qwen3.8 coding benchmark.
benchmark-codegen-qwen38:
	RUN_IN_PARALLEL=yes GITHUB_WORKSPACE=$(shell pwd) go test -run='^$$' -bench='^BenchmarkCodeGen_Qwen38_27B_Q4$$' -benchtime=$(BENCH_CODEGEN_TIME) -timeout=$(BENCH_CODEGEN_TIMEOUT) $(BENCH_PROFILE_FLAGS) ./sdk/kronk/tests/benchmarks/

# Run the Gemma 4 coding benchmark.
benchmark-codegen-gemma4:
	RUN_IN_PARALLEL=yes GITHUB_WORKSPACE=$(shell pwd) go test -run='^$$' -bench='^BenchmarkCodeGen_Gemma4_26B_A4B_Q8$$' -benchtime=$(BENCH_CODEGEN_TIME) -timeout=$(BENCH_CODEGEN_TIMEOUT) $(BENCH_PROFILE_FLAGS) ./sdk/kronk/tests/benchmarks/

# Run all coding benchmarks sequentially and store their artifacts together.
benchmark-codegen:
	@RESULTS=sdk/kronk/tests/benchmarks/runs/$$(date +%Y%m%d-%H%M%S); \
	mkdir -p $$RESULTS; \
	for target in \
		benchmark-codegen-ornith \
		benchmark-codegen-mtp-qwen36 \
		benchmark-codegen-qwen38 \
		benchmark-codegen-gemma4; \
	do \
		CODEGEN_RESULTS_DIR=$$RESULTS $(MAKE) $$target || exit $$?; \
	done; \
	go run ./sdk/kronk/tests/benchmarks/report -run "$$RESULTS" || exit $$?; \
	echo ""; \
	echo "Results written to $$RESULTS"

# Generate the final report for an existing coding benchmark run.
benchmark-codegen-report:
	@test -n "$(CODEGEN_RESULTS_DIR)" || (echo "CODEGEN_RESULTS_DIR is required"; exit 1)
	go run ./sdk/kronk/tests/benchmarks/report -run "$(CODEGEN_RESULTS_DIR)"

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
