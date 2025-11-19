# Check to see if we can use ash, in Alpine images, or default to BASH.
SHELL_PATH = /bin/ash
SHELL = $(if $(wildcard $(SHELL_PATH)),/bin/ash,/bin/bash)

# ==============================================================================
# Tests

test:
	export LD_LIBRARY_PATH=libraries && \
	export YZMA_LIB=libraries && \
	export CONCURRENCY=3 && \
	go test -v -count=1

# ==============================================================================
# Go Modules support

tidy:
	go mod tidy

deps-upgrade:
	go get -u -v ./...
	GOPROXY=direct go get github.com/hybridgroup/yzma@main
	go mod tidy
