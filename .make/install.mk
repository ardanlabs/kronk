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

# Use this to install or update llama.cpp, whisper.cpp, and
# stable-diffusion.cpp to the latest version. Used by the local `make test`
# target so developers exercise the newest bundles before bumping each
# backend's well-known defaultVersion for a release. All three backends
# support --upgrade to track the latest published release instead of the
# bundled default version.
install-libraries: install-kronk
	@echo "========== INSTALL LLAMA LIBRARIES (latest) =========="
	kronk libs --local --upgrade
	@echo
	@echo "========== INSTALL WHISPER LIBRARIES (latest) =========="
	kronk bucky libs --local --upgrade
	@echo
	@echo "========== INSTALL STABLE DIFFUSION LIBRARIES (latest) =========="
	kronk malina libs --local --upgrade
	@echo

# Use this to install the well-known defaultVersion of llama.cpp,
# whisper.cpp, and stable-diffusion.cpp baked into the SDK. This mirrors
# what CI does so `make test-gh` reproduces the GH workflow locally.
# Bumping each backend's pinned default is what rolls this target and the
# CI workflow forward.
install-libraries-gh: install-kronk
	@echo "========== INSTALL LLAMA LIBRARIES (defaultVersion) =========="
	kronk libs --local
	@echo
	@echo "========== INSTALL WHISPER LIBRARIES (defaultVersion) =========="
	kronk bucky libs --local
	@echo
	@echo "========== INSTALL STABLE DIFFUSION LIBRARIES (defaultVersion) =========="
	kronk malina libs --local
	@echo

# Use this to install the test GH models.
install-test-gh-models: install-kronk
	@echo ========== INSTALL MODELS ==========
	kronk model pull --local "unsloth/Qwen3.5-0.8B-Q8_0"
	@echo
	kronk model pull --local "unsloth/Qwen3-1.7B-Q4_K_M"
	@echo
	kronk model pull --local "unsloth/Qwen3-0.6B-Q8_0"
	@echo
	kronk model pull --local "mradermacher/Qwopus3.5-4B-Coder.Q4_K_M"
	@echo
	kronk model pull --local "nomic-ai/nomic-embed-text-v1.5.Q8_0"
	@echo
	kronk model pull --local "gpustack/bge-reranker-v2-m3-Q8_0"
	@echo
	kronk bucky model pull --local "ggml-tiny.bin"
	@echo

# Use this to install the test models.
install-test-models: install-kronk
	@echo ========== INSTALL KRONK MODELS ==========
	kronk model pull --local "unsloth/Qwen3-0.6B-Q8_0"
	@echo
	kronk model pull --local "unsloth/Qwen3.5-0.8B-Q8_0"
	@echo
	kronk model pull --local "mradermacher/Qwopus3.5-4B-Coder.Q4_K_M"
	@echo
	kronk model pull --local "unsloth/LFM2-700M-Q8_0"
	@echo
	kronk model pull --local "unsloth/gemma-4-26B-A4B-it-UD-Q4_K_M"
	@echo
	kronk model pull --local "unsloth/Qwen3.6-35B-A3B-MTP-GGUF:UD-Q2_K_XL"
	@echo
	kronk model pull --local "ggml-org/Qwen2.5-Omni-3B-Q4_K_M"
	@echo
	kronk model pull --local "unsloth/gpt-oss-20b-Q8_0"
	@echo
	kronk model pull --local "unsloth/Qwen3-1.7B-Q4_K_M"
	@echo
	kronk model pull --local "nomic-ai/nomic-embed-text-v1.5.Q8_0"
	@echo
	kronk model pull --local "gpustack/bge-reranker-v2-m3-Q8_0"
	@echo
	@echo ========== INSTALL BUCKY MODELS ==========
	kronk bucky model pull --local "ggml-tiny.bin"
	@echo
	@echo ========== INSTALL MALINA MODELS ==========
	kronk malina model pull --local "sd-1.5"
	@echo

# Use this to install models for the class.
install-class-models: install-kronk
	@echo ========== INSTALL MODELS ==========
	kronk model pull --local "unsloth/Qwen3-0.6B-Q8_0"
	@echo
	kronk model pull --local "unsloth/Qwen3.5-0.8B-Q8_0"
	@echo
	kronk model pull --local "mradermacher/Qwopus3.5-4B-Coder.Q8_0"
	@echo
	kronk model pull --local "ornith-ai/Ornith-1.5-9B-Q4_K_M"
	@echo
	kronk model pull --local "ggml-org/Qwen2.5-Omni-3B-Q8_0"
	@echo
	kronk model pull --local "Qwen/Qwen3-Embedding-0.6B-Q8_0.gguf"
	@echo
	kronk model pull --local "gpustack/bge-reranker-v2-m3-Q8_0"
	@echo
	kronk bucky model pull --local "ggml-tiny.bin"
	@echo
	kronk malina model pull --local "sd-1.5"
	@echo

OPENWEBUI  := ghcr.io/open-webui/open-webui:v0.11.0
GRAFANA    := grafana/grafana:13.1.3
PROMETHEUS := prom/prometheus:v3.13.2
TEMPO      := grafana/tempo:3.0.3
LOKI       := grafana/loki:3.7.6
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
