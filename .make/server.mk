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
	export KRONK_DOWNLOAD_ENABLED=true && \
	export KRONK_INSECURE_LOGGING=true && \
	export KRONK_POOL_MODEL_CONFIG_FILE=zarf/kms/model_config.yaml && \
	go run cmd/kronk/main.go server start | go run cmd/server/api/tooling/logfmt/main.go

kronk-server-build: bui-build
	. .env 2>/dev/null || true && \
	export KRONK_DOWNLOAD_ENABLED=true && \
	export KRONK_INSECURE_LOGGING=true && \
	export KRONK_POOL_MODEL_CONFIG_FILE=zarf/kms/model_config.yaml && \
	go run cmd/kronk/main.go server start | go run cmd/server/api/tooling/logfmt/main.go

kronk-server-upgrade: install-latest-libs bui-build
	. .env 2>/dev/null || true && \
	export KRONK_DOWNLOAD_ENABLED=true && \
	export KRONK_ALLOW_UPGRADE=true && \
	export KRONK_INSECURE_LOGGING=true && \
	export KRONK_POOL_MODEL_CONFIG_FILE=zarf/kms/model_config.yaml && \
	go run cmd/kronk/main.go server start | go run cmd/server/api/tooling/logfmt/main.go

kronk-server-detach: bui-build
	go run cmd/kronk/main.go server start --detach

kronk-server-logs:
	go run cmd/kronk/main.go server logs

kronk-server-stop:
	go run cmd/kronk/main.go server stop
