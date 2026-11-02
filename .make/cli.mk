##@ CLI commands

# ==============================================================================
# Kronk CLI — llama (kronk) verbs

kronk-diagnose: ## Kronk: diagnose
	go run cmd/kronk/main.go diagnose

# ------------------------------------------------------------------------------

kronk-libs: ## Kronk: libs
	go run cmd/kronk/main.go libs

kronk-libs-local: install-libraries ## Kronk: libs locally

# ------------------------------------------------------------------------------

kronk-model-index: ## Kronk: model index
	go run cmd/kronk/main.go model index

kronk-model-index-local: ## Kronk: model index locally
	go run cmd/kronk/main.go model index --local


kronk-model-list: ## Kronk: model list
	go run cmd/kronk/main.go model list

kronk-model-list-local: ## Kronk: model list locally
	go run cmd/kronk/main.go model list --local


# make kronk-model-pull URL="unsloth/Qwen3-1.7B-UD-Q8_K_XL"
kronk-model-pull: ## Kronk: model pull
	go run cmd/kronk/main.go model pull "$(URL)"

# make kronk-model-pull-local URL="unsloth/Qwen3-1.7B-UD-Q8_K_XL"
kronk-model-pull-local: ## Kronk: model pull locally
	go run cmd/kronk/main.go model pull --local "$(URL)"


kronk-model-ps: ## Kronk: model ps
	go run cmd/kronk/main.go model ps


# make kronk-model-remove ID="bartowski/cerebras_qwen3-coder-reap-25b-a3b-q8_0"
kronk-model-remove: ## Kronk: model remove
	go run cmd/kronk/main.go model remove "$(ID)"

# make kronk-model-remove-local ID="bartowski/cerebras_qwen3-coder-reap-25b-a3b-q8_0"
kronk-model-remove-local: ## Kronk: model remove locally
	go run cmd/kronk/main.go model remove --local "$(ID)"


# make kronk-model-show ID="unsloth/Qwen3-1.7B-UD-Q8_K_XL"
kronk-model-show: ## Kronk: model show
	go run cmd/kronk/main.go model show "$(ID)"

# make kronk-model-show-local ID="unsloth/Qwen3-1.7B-UD-Q8_K_XL"
kronk-model-show-local: ## Kronk: model show locally
	go run cmd/kronk/main.go model show --local "$(ID)"

# ------------------------------------------------------------------------------

kronk-catalog-list: ## Kronk: catalog list
	go run cmd/kronk/main.go catalog list

kronk-catalog-list-local: ## Kronk: catalog list locally
	go run cmd/kronk/main.go catalog list --local


# make kronk-catalog-show ID="unsloth/Qwen3-1.7B-UD-Q8_K_XL"
kronk-catalog-show: ## Kronk: catalog show
	go run cmd/kronk/main.go catalog show "$(ID)"

# make kronk-catalog-show-local ID="unsloth/Qwen3-1.7B-UD-Q8_K_XL"
kronk-catalog-show-local: ## Kronk: catalog show locally
	go run cmd/kronk/main.go catalog show --local "$(ID)"


# ------------------------------------------------------------------------------

kronk-security-help: ## Kronk: security help
	go run cmd/kronk/main.go security --help


kronk-security-key-list: ## Kronk: security key list
	go run cmd/kronk/main.go security key list

kronk-security-key-list-local: ## Kronk: security key list locally
	go run cmd/kronk/main.go security key list --local

# make kronk-security-token-create-local D="5m" E="chat-completions"
kronk-security-token-create-local: ## Kronk: security token create locally
	go run cmd/kronk/main.go security token create --local --duration "$(D)" --endpoints "$(E)"

# ------------------------------------------------------------------------------

# Launch OpenCode with the Qwen3.6 35B-A3B coding model.
kronk-launch: ## Kronk: launch
	go run cmd/kronk/main.go launch opencode "unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL"

# ==============================================================================
# Bucky (whisper) verbs

bucky-libs: ## Bucky: libs
	go run cmd/kronk/main.go bucky libs

bucky-libs-local: ## Bucky: libs locally
	go run cmd/kronk/main.go bucky libs --local

bucky-libs-combinations: ## Bucky: libs combinations
	go run cmd/kronk/main.go bucky libs --list-combinations

bucky-libs-combinations-local: ## Bucky: libs combinations locally
	go run cmd/kronk/main.go bucky libs --local --list-combinations

bucky-libs-installs: ## Bucky: libs installs
	go run cmd/kronk/main.go bucky libs --list-installs

bucky-libs-installs-local: ## Bucky: libs installs locally
	go run cmd/kronk/main.go bucky libs --local --list-installs

# make bucky-libs-install ARCH=amd64 OS=linux PROC=cuda
bucky-libs-install: ## Bucky: libs install
	go run cmd/kronk/main.go bucky libs --install --arch="$(ARCH)" --os="$(OS)" --processor="$(PROC)"

# make bucky-libs-install-local ARCH=amd64 OS=linux PROC=cuda
bucky-libs-install-local: ## Bucky: libs install locally
	go run cmd/kronk/main.go bucky libs --local --install --arch="$(ARCH)" --os="$(OS)" --processor="$(PROC)"

# make bucky-libs-remove-install ARCH=amd64 OS=linux PROC=cuda
bucky-libs-remove-install: ## Bucky: libs remove install
	go run cmd/kronk/main.go bucky libs --remove-install --arch="$(ARCH)" --os="$(OS)" --processor="$(PROC)"

# make bucky-libs-remove-install-local ARCH=amd64 OS=linux PROC=cuda
bucky-libs-remove-install-local: ## Bucky: libs remove install locally
	go run cmd/kronk/main.go bucky libs --local --remove-install --arch="$(ARCH)" --os="$(OS)" --processor="$(PROC)"

# ------------------------------------------------------------------------------

bucky-model-list: ## Bucky: model list
	go run cmd/kronk/main.go bucky model list

bucky-model-list-local: ## Bucky: model list locally
	go run cmd/kronk/main.go bucky model list --local


# make bucky-model-pull NAME="ggml-tiny.bin"
bucky-model-pull: ## Bucky: model pull
	go run cmd/kronk/main.go bucky model pull "$(NAME)"

# make bucky-model-pull-local NAME="ggml-tiny.bin"
bucky-model-pull-local: ## Bucky: model pull locally
	go run cmd/kronk/main.go bucky model pull --local "$(NAME)"


# make bucky-model-remove NAME="ggml-tiny.bin"
bucky-model-remove: ## Bucky: model remove
	go run cmd/kronk/main.go bucky model remove "$(NAME)"

# make bucky-model-remove-local NAME="ggml-tiny.bin"
bucky-model-remove-local: ## Bucky: model remove locally
	go run cmd/kronk/main.go bucky model remove --local "$(NAME)"
