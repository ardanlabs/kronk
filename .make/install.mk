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
	brew list ffmpeg || brew install ffmpeg

# Install the kronk cli.
install-kronk:
	@echo ========== INSTALL KRONK ==========
	go install ./cmd/kronk
	@echo

# Use this to install or update llama.cpp + whisper.cpp to the latest
# version. Used by the local `make test` target so developers exercise
# the newest bundles before bumping the well-known defaultVersion in
# sdk/tools/libs/libs.go (llama) and sdk/tools/bucky/libs/libs.go
# (whisper) for a release. Both backends support --upgrade to track the
# latest published release instead of the bundled default version.
install-libraries: install-kronk
	@echo "========== INSTALL LLAMA LIBRARIES (latest) =========="
	kronk libs --local --upgrade
	@echo
	@echo "========== INSTALL WHISPER LIBRARIES (latest) =========="
	kronk bucky libs --local --upgrade
	@echo

# Use this to install the well-known defaultVersion of llama.cpp +
# whisper.cpp baked into the SDK. This mirrors what CI does so
# `make test-gh` reproduces the GH workflow locally. Bumping
# defaultVersion in sdk/tools/libs/libs.go (llama) and
# download.DefaultWhisperVersion in github.com/ardanlabs/bucky is what
# rolls both this target and the CI workflow forward.
install-libraries-gh: install-kronk
	@echo "========== INSTALL LLAMA LIBRARIES (defaultVersion) =========="
	kronk libs --local
	@echo
	@echo "========== INSTALL WHISPER LIBRARIES (defaultVersion) =========="
	kronk bucky libs --local
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
	kronk bucky model pull --local "tiny.en"
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
	kronk model pull --local "ggml-org/Qwen2.5-Omni-3B-Q8_0"
	@echo
	kronk model pull --local "unsloth/gpt-oss-20b-Q8_0"
	@echo
	kronk model pull --local "Qwen/Qwen3-8B-Q8_0"
	@echo
	kronk model pull --local "ggml-org/embeddinggemma-300m-qat-Q8_0"
	@echo
	kronk model pull --local "gpustack/bge-reranker-v2-m3-Q8_0"
	@echo
	kronk bucky model pull --local "tiny.en"
	@echo

# Use this to install models for the class.
install-class-models: install-kronk
	@echo ========== INSTALL MODELS ==========
	kronk model pull --local "unsloth/Qwen3.5-0.8B-Q8_0"
	@echo
	kronk model pull --local "unsloth/LFM2.5-VL-1.6B-Q8_0"
	@echo
	kronk model pull --local "mradermacher/Qwopus3.5-4B-Coder.Q8_0"
	@echo
	kronk model pull --local "unsloth/gemma-4-E4B-it-qat-UD-Q4_K_XL"
	@echo
	kronk model pull --local "ggml-org/Qwen2.5-Omni-3B-Q8_0"
	@echo
	kronk model pull --local "unsloth/Qwen3-0.6B-Q8_0"
	@echo
	kronk model pull --local "unsloth/LFM2-700M-Q8_0"
	@echo
	kronk model pull --local "ggml-org/embeddinggemma-300m-qat-Q8_0"
	@echo
	kronk model pull --local "gpustack/bge-reranker-v2-m3-Q8_0"
	@echo
	kronk bucky model pull --local "tiny.en"
	@echo

OPENWEBUI  := ghcr.io/open-webui/open-webui:v0.9.6
GRAFANA    := grafana/grafana:13.0.2
PROMETHEUS := prom/prometheus:v3.12.0
TEMPO      := grafana/tempo:3.0.0
LOKI       := grafana/loki:3.7.2
PROMTAIL   := grafana/promtail:3.6.11

# Install the docker images.
install-docker:
	docker pull docker.io/$(OPENWEBUI) & \
	docker pull docker.io/$(GRAFANA) & \
	docker pull docker.io/$(PROMETHEUS) & \
	docker pull docker.io/$(TEMPO) & \
	docker pull docker.io/$(LOKI) & \
	docker pull docker.io/$(PROMTAIL) & \
	wait;
