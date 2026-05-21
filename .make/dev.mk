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
