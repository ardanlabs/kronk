# ==============================================================================
# Kronk BUI

BUI_DIR := cmd/server/api/frontends/bui

bui-install:
	cd $(BUI_DIR) && npm install

bui-run: kronk-docs
	cd $(BUI_DIR) && npm run dev

bui-build:
	cd $(BUI_DIR) && npm run build

bui-upgrade:
	cd $(BUI_DIR) && npm update

bui-upgrade-latest:
	cd $(BUI_DIR) && npx npm-check-updates -u && npm install

# ==============================================================================
# Kronk Server

install-working-libs:
	@echo "========== INSTALL LLAMA LIBRARIES (working) =========="
	go run cmd/kronk/main.go libs --local --version=b9748
	@echo
	@echo "========== INSTALL WHISPER LIBRARIES (working) =========="
	go run cmd/kronk/main.go bucky libs --local --version=v1.9.1
	@echo

install-latest-libs:
	@echo "========== INSTALL LLAMA LIBRARIES (upgrade) =========="
	go run cmd/kronk/main.go libs --local --upgrade
	@echo
	@echo "========== INSTALL WHISPER LIBRARIES (upgrade) =========="
	go run cmd/kronk/main.go bucky libs --local --upgrade
	@echo

kronk-build: kronk-docs bui-build

kronk-docs:
	go run cmd/server/api/tooling/docs/*.go

kronk-server:
	. .env 2>/dev/null || true && \
	export KRONK_POOL_MODEL_CONFIG_FILE=zarf/kms/model_config.yaml && \
	go run -a cmd/kronk/main.go server start | go run cmd/server/api/tooling/logfmt/main.go

kronk-server-build: kronk-build
	. .env 2>/dev/null || true && \
	export KRONK_POOL_MODEL_CONFIG_FILE=zarf/kms/model_config.yaml && \
	go run -a cmd/kronk/main.go server start | go run cmd/server/api/tooling/logfmt/main.go

kronk-server-upgrade: install-latest-libs kronk-build
	. .env 2>/dev/null || true && \
	export KRONK_ALLOW_UPGRADE=true && \
	export KRONK_POOL_MODEL_CONFIG_FILE=zarf/kms/model_config.yaml && \
	go run -a cmd/kronk/main.go server start | go run cmd/server/api/tooling/logfmt/main.go

kronk-server-detach: kronk-build
	go run cmd/kronk/main.go -a server start --detach

kronk-server-logs:
	go run cmd/kronk/main.go server logs

kronk-server-stop:
	go run cmd/kronk/main.go server stop

llama-qwen36:
	$(HOME)/.kronk/libraries/darwin/arm64/metal/llama-server \
		-m $(HOME)/.kronk/models/unsloth/Qwen3.6-35B-A3B-GGUF/Qwen3.6-35B-A3B-UD-Q8_K_XL.gguf \
		--alias unsloth/Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT \
		--host 127.0.0.1 \
		--port 11435 \
		--no-cache-prompt \
		--ctx-size 131072 \
		--parallel 2 \
		--batch-size 4096 \
		--ubatch-size 4096 \
		--flash-attn auto \
		--swa-full \
		--cache-type-k f16 \
		--cache-type-v f16 \
		--temperature 0.6 \
		--top-k 20 \
		--top-p 1 \
		--min-p 0 \
		--repeat-last-n 64 \
		--repeat-penalty 1 \
		--dry-multiplier 0 \
		--jinja \
		--reasoning auto \
		--reasoning-format deepseek \
		--metrics
