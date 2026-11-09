##@ Installation

# ==============================================================================
# Setup

# Configure git to use project hooks so pre-commit runs for all developers.
setup: ## Set up the development environment
	git config core.hooksPath .githooks

# ==============================================================================
# Install

install-gotooling: ## Install Go development tools
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/nix-community/gomod2nix@latest

install-tooling: ## Install development tooling
	brew list protobuf || brew install protobuf
	brew list grpcurl || brew install grpcurl
	brew list node || brew install node
	brew list rg || brew install rg
	brew list ffmpeg || brew install ffmpeg

# Install the kronk cli.
install-kronk: ## Install the Kronk CLI
	@echo ========== INSTALL KRONK ==========
	go install ./cmd/kronk
	@echo

# Use this to install or update llama.cpp, whisper.cpp, and
# stable-diffusion.cpp to the pinned version.
install-libraries: install-kronk ## Install native inference libraries
	@echo "========== INSTALL LLAMA LIBRARIES (pinned) =========="
	kronk libs --local
	@echo
	@echo "========== INSTALL WHISPER LIBRARIES (pinned) =========="
	kronk bucky libs --local
	@echo
	@echo "========== INSTALL STABLE DIFFUSION LIBRARIES (pinned) =========="
	kronk malina libs --local
	@echo

# Use this to install the test models.
install-test-models: install-kronk ## Download test models
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
	kronk model pull --local "chaoliangUNSW/Jev-Style-0.8B-Decision-v3-Q8_0.gguf"
	@echo
	kronk model pull --local "LiquidAI/d1-Omni-2B-Q8_0"
	@echo
	kronk model pull --local "LiquidAI/LFM3-1.7B-Q4_K_M"
	@echo
	@echo ========== INSTALL BUCKY MODELS ==========
	kronk bucky model pull --local "ggml-tiny.bin"
	@echo
	kronk bucky model pull --local "silero-vad"
	@echo

# Use this to install the library and model required by the opt-in Malina tests.
install-test-malina: install-kronk ## Download Malina test fixtures
	@echo ========== INSTALL STABLE DIFFUSION LIBRARIES ==========
	kronk malina libs --local
	@echo
	@echo ========== INSTALL MALINA TEST MODEL ==========
	kronk malina model pull --local "sd-1.5"
	@echo

# Install the libraries and published fixtures for the opt-in decision suite.
# Keep this list aligned with TestDecisionModels in sdk/kronk/model.
install-test-decision: install-kronk ## Download local decision test fixtures
	@echo ========== INSTALL DECISION LIBRARIES AND MODELS ==========
	unset KRONK_TEST_HOSTED KRONK_BASE_PATH KRONK_LIB_PATH KRONK_PROCESSOR KRONK_ARCH KRONK_OS && \
	kronk libs --local && \
	kronk model pull --local "openjev/OpenJev-Q4_K_M" && \
	kronk model pull --local "ggml-org/OpenJev-Q4_K_M" && \
	kronk model pull --local "chaoliangUNSW/Jev-Style-0.8B-Decision-v3-Q8_0" && \
	kronk model pull --local "ggml-org/Laya-Q8_0" && \
	kronk model pull --local "ggml-org/Julia-1-Q8_0" && \
	kronk model pull --local "ggml-org/lev-Q4_K_M" && \
	kronk model pull --local "ggml-org/Kev-4B-Q4_K_M" && \
	kronk model pull --local "ggml-org/Bespoke-Nimble-9B-v3-Q4_K_M" && \
	kronk model pull --local "ggml-org/Clef-Flash-Q4_K_M" && \
	kronk model pull --local "ggml-org/Clef-Q4_K_M" && \
	kronk model pull --local "turintech/pplx-decider-27B-Artemis-IQ3_M" && \
	kronk model pull --local "ibm-granite/granite-switch-4.1-8b-Q4_K_M" && \
	kronk model pull --local "h2oai/h2o-lightning-4b-Q4_K_M" && \
	kronk model pull --local "LiquidAI/d1-3B-Q4_K_M" && \
	kronk model pull --local "LiquidAI/d1-omni-600M-Q8_0" && \
	kronk model pull --local "LiquidAI/d1-2B-GGUF/d1-2B-Q4_K_M" && \
	kronk model pull --local "LiquidAI/d1-Omni-2B-Q8_0" && \
	kronk model pull --local "ggml-org/vllm-d3-edge-Q8_0"
	@echo "Decision2 requires KRONK_TEST_DECISION2_MODEL; see sdk/kronk/model/decision_model_test.go"
	@echo

# Use this to install models for the class.
install-class-models: install-kronk ## Download class models
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
	kronk model pull --local "chaoliangUNSW/Jev-Style-0.8B-Decision-v3-Q8_0.gguf"
	@echo
	kronk model pull --local "LiquidAI/d1-Omni-2B-Q8_0"
	@echo

OPENWEBUI  := ghcr.io/open-webui/open-webui:v0.11.1
GRAFANA    := grafana/grafana:13.2.1
PROMETHEUS := prom/prometheus:v3.14.0
TEMPO      := grafana/tempo:3.0.3
LOKI       := grafana/loki:3.7.7
ALLOY      := grafana/alloy:v1.19.2

# Install the docker images.
install-docker: ## Pull supporting Docker images
	docker pull $(OPENWEBUI) & \
	docker pull $(GRAFANA) & \
	docker pull $(PROMETHEUS) & \
	docker pull $(TEMPO) & \
	docker pull $(LOKI) & \
	docker pull $(ALLOY) & \
	wait;
