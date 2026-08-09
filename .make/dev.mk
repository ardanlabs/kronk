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
	unset KRONK_BASE_PATH KRONK_LIB_PATH KRONK_BUCKY_LIB_PATH KRONK_PROCESSOR KRONK_ARCH KRONK_OS && \
	export RUN_IN_PARALLEL=yes && \
	export GITHUB_WORKSPACE=$(shell pwd) && \
	go test -v -p=1 -count=1 ./cmd/server/... && \
	go test -v -p=1 -count=1 ./sdk/...

test: test-only lint vuln-check diff

test-gh-only: install-libraries-gh install-test-gh-models
	@echo ========== RUN GH ONLY TESTS ==========
	unset KRONK_BASE_PATH KRONK_LIB_PATH KRONK_BUCKY_LIB_PATH KRONK_PROCESSOR KRONK_ARCH KRONK_OS && \
	export RUN_IN_PARALLEL=yes && \
	export GITHUB_WORKSPACE=$(shell pwd) && \
	export GITHUB_ACTIONS=true && \
	go test -v -p=1 -count=1 ./cmd/server/... && \
	go test -v -p=1 -count=1 $(go list ./sdk/... | grep -v '/sdk/kronk/tests')

test-gh: test-gh-only lint vuln-check diff

# Adversarial probe harness against a running (or self-started) Kronk server.
# Everything is env-tunable — see zarf/scripts/kronk-stress.sh -h. Results land
# in zarf/tmp/kronk-stress. Pass groups or a tier through ARGS:
#   make test-stress ARGS="--tier=smoke"
#   make test-stress ARGS="stream structured"
#
# The script exits 1 when it flags anything. Print the triage prompt regardless, then
# re-raise the script's status so the target still fails on findings.
test-stress:
	@echo ========== RUN STRESS PROBES ==========
	@zarf/scripts/kronk-stress.sh $(ARGS); \
	status=$$?; \
	echo; \
	echo "========== TRIAGE PROMPT =========="; \
	echo "Hand this to a coding agent to turn the findings into a verified bug report:"; \
	echo; \
	cat zarf/scripts/kronk-stress-triage.md; \
	exit $$status

# ==============================================================================
# Benchmarks

# BENCH_PROFILE_FLAGS passes optional go test profiling flags to one benchmark.
# Run CPU and allocation profiles separately: -memprofilerate=1 intentionally
# records every allocation and would distort a CPU profile collected in the
# same run.
BENCH_PROFILE_FLAGS ?=

# CPU: make benchmark-dense-nc BENCH_PROFILE_FLAGS='-cpuprofile=/tmp/benchmark-dense-nc.cpu.pprof'
# Allocations: make benchmark-dense-nc BENCH_PROFILE_FLAGS='-memprofile=/tmp/benchmark-dense-nc.mem.pprof -memprofilerate=1'
benchmark-dense-nc:
	go test -run=none -bench=BenchmarkDense_NonCaching -benchtime=3x -timeout=30m $(BENCH_PROFILE_FLAGS) ./sdk/kronk/tests/benchmarks/

# CPU: make benchmark-dense-imc BENCH_PROFILE_FLAGS='-cpuprofile=/tmp/benchmark-dense-imc.cpu.pprof'
# Allocations: make benchmark-dense-imc BENCH_PROFILE_FLAGS='-memprofile=/tmp/benchmark-dense-imc.mem.pprof -memprofilerate=1'
benchmark-dense-imc:
	go test -run=none -bench=BenchmarkDense_IMC -benchtime=3x -timeout=30m $(BENCH_PROFILE_FLAGS) ./sdk/kronk/tests/benchmarks/

# CPU: make benchmark-moe-nc BENCH_PROFILE_FLAGS='-cpuprofile=/tmp/benchmark-moe-nc.cpu.pprof'
# Allocations: make benchmark-moe-nc BENCH_PROFILE_FLAGS='-memprofile=/tmp/benchmark-moe-nc.mem.pprof -memprofilerate=1'
benchmark-moe-nc:
	go test -run=none -bench=BenchmarkMoE_NonCaching -benchtime=3x -timeout=30m $(BENCH_PROFILE_FLAGS) ./sdk/kronk/tests/benchmarks/

# CPU: make benchmark-moe-imc BENCH_PROFILE_FLAGS='-cpuprofile=/tmp/benchmark-moe-imc.cpu.pprof'
# Allocations: make benchmark-moe-imc BENCH_PROFILE_FLAGS='-memprofile=/tmp/benchmark-moe-imc.mem.pprof -memprofilerate=1'
benchmark-moe-imc:
	go test -run=none -bench=BenchmarkMoE_IMC -benchtime=3x -timeout=30m $(BENCH_PROFILE_FLAGS) ./sdk/kronk/tests/benchmarks/

# CPU: make benchmark-hybrid-nc BENCH_PROFILE_FLAGS='-cpuprofile=/tmp/benchmark-hybrid-nc.cpu.pprof'
# Allocations: make benchmark-hybrid-nc BENCH_PROFILE_FLAGS='-memprofile=/tmp/benchmark-hybrid-nc.mem.pprof -memprofilerate=1'
benchmark-hybrid-nc:
	go test -run=none -bench=BenchmarkHybrid_NonCaching -benchtime=3x -timeout=30m $(BENCH_PROFILE_FLAGS) ./sdk/kronk/tests/benchmarks/

# CPU: make benchmark-hybrid-imc BENCH_PROFILE_FLAGS='-cpuprofile=/tmp/benchmark-hybrid-imc.cpu.pprof'
# Allocations: make benchmark-hybrid-imc BENCH_PROFILE_FLAGS='-memprofile=/tmp/benchmark-hybrid-imc.mem.pprof -memprofilerate=1'
benchmark-hybrid-imc:
	go test -run=none -bench=BenchmarkHybrid_IMC -benchtime=3x -timeout=30m $(BENCH_PROFILE_FLAGS) ./sdk/kronk/tests/benchmarks/

# CPU: make benchmark-embedding-fallback BENCH_PROFILE_FLAGS='-cpuprofile=/tmp/benchmark-embedding-fallback.cpu.pprof'
# Allocations: make benchmark-embedding-fallback BENCH_PROFILE_FLAGS='-memprofile=/tmp/benchmark-embedding-fallback.mem.pprof -memprofilerate=1'
benchmark-embedding-fallback:
	go test -tags=kronk_benchmark -run=none -bench='^BenchmarkEmbedding_Qwen3_ContextPoolFallback$$' -benchtime=10s -cpu=4 -timeout=30m $(BENCH_PROFILE_FLAGS) ./sdk/kronk/tests/benchmarks/

# CPU: make benchmark-embedding-batchseq BENCH_PROFILE_FLAGS='-cpuprofile=/tmp/benchmark-embedding-batchseq.cpu.pprof'
# Allocations: make benchmark-embedding-batchseq BENCH_PROFILE_FLAGS='-memprofile=/tmp/benchmark-embedding-batchseq.mem.pprof -memprofilerate=1'
benchmark-embedding-batchseq:
	go test -tags=kronk_benchmark -run=none -bench='^BenchmarkEmbedding_Qwen3_BatchSeq$$' -benchtime=10s -cpu=4 -timeout=30m $(BENCH_PROFILE_FLAGS) ./sdk/kronk/tests/benchmarks/

# CPU: make benchmark-rerank-fallback BENCH_PROFILE_FLAGS='-cpuprofile=/tmp/benchmark-rerank-fallback.cpu.pprof'
# Allocations: make benchmark-rerank-fallback BENCH_PROFILE_FLAGS='-memprofile=/tmp/benchmark-rerank-fallback.mem.pprof -memprofilerate=1'
benchmark-rerank-fallback:
	go test -run=none -bench=BenchmarkRerank_Qwen3_ContextPoolFallback -benchtime=10s -timeout=30m $(BENCH_PROFILE_FLAGS) ./sdk/kronk/tests/benchmarks/

# CPU: make benchmark-rerank-batchseq BENCH_PROFILE_FLAGS='-cpuprofile=/tmp/benchmark-rerank-batchseq.cpu.pprof'
# Allocations: make benchmark-rerank-batchseq BENCH_PROFILE_FLAGS='-memprofile=/tmp/benchmark-rerank-batchseq.mem.pprof -memprofilerate=1'
benchmark-rerank-batchseq:
	go test -run=none -bench=BenchmarkRerank_BGE_BatchSeq -benchtime=10s -timeout=30m $(BENCH_PROFILE_FLAGS) ./sdk/kronk/tests/benchmarks/

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
