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

kronk-build: kronk-docs bui-build

kronk-docs:
	go run cmd/server/api/tooling/docs/*.go

kronk-server:
	. .env 2>/dev/null || true && \
	export KRONK_DOWNLOAD_ENABLED=true && \
	export KRONK_ALLOW_UPGRADE=true && \
	export KRONK_INSECURE_LOGGING=true && \
	export KRONK_POOL_MODEL_CONFIG_FILE=zarf/kms/model_config.yaml && \
	go run cmd/kronk/main.go server start | go run cmd/server/api/tooling/logfmt/main.go

kronk-server-build: kronk-build
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
