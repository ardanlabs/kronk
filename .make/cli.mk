##@ CLI commands

# ==============================================================================
# Kronk CLI — llama (kronk) verbs

kronk-diagnose: ## Diagnose the host and installed library bundles
	go run cmd/kronk/main.go diagnose

# ------------------------------------------------------------------------------

kronk-libs: ## Install pinned llama.cpp libraries through the server
	go run cmd/kronk/main.go libs

kronk-libs-local: install-libraries ## Install all pinned inference libraries locally

# ------------------------------------------------------------------------------

kronk-model-index: ## Rebuild the model index through the server
	go run cmd/kronk/main.go model index

kronk-model-index-local: ## Rebuild the local model index without a server
	go run cmd/kronk/main.go model index --local


kronk-model-list: ## List downloaded models through the server
	go run cmd/kronk/main.go model list

kronk-model-list-local: ## List locally downloaded models without a server
	go run cmd/kronk/main.go model list --local


# make kronk-model-pull URL="unsloth/Qwen3-1.7B-UD-Q8_K_XL"
kronk-model-pull: ## Download a model through the server; URL=<source>
	go run cmd/kronk/main.go model pull "$(URL)"

# make kronk-model-pull-local URL="unsloth/Qwen3-1.7B-UD-Q8_K_XL"
kronk-model-pull-local: ## Download a model without a server; URL=<source>
	go run cmd/kronk/main.go model pull --local "$(URL)"


kronk-model-ps: ## List models currently loaded by the server
	go run cmd/kronk/main.go model ps


# make kronk-model-remove ID="bartowski/cerebras_qwen3-coder-reap-25b-a3b-q8_0"
kronk-model-remove: ## Remove a downloaded model through the server; ID=<model>
	go run cmd/kronk/main.go model remove "$(ID)"

# make kronk-model-remove-local ID="bartowski/cerebras_qwen3-coder-reap-25b-a3b-q8_0"
kronk-model-remove-local: ## Remove a local model without a server; ID=<model>
	go run cmd/kronk/main.go model remove --local "$(ID)"


# make kronk-model-show ID="unsloth/Qwen3-1.7B-UD-Q8_K_XL"
kronk-model-show: ## Show model details through the server; ID=<model>
	go run cmd/kronk/main.go model show "$(ID)"

# make kronk-model-show-local ID="unsloth/Qwen3-1.7B-UD-Q8_K_XL"
kronk-model-show-local: ## Show local model details without a server; ID=<model>
	go run cmd/kronk/main.go model show --local "$(ID)"

# ------------------------------------------------------------------------------

kronk-catalog-list: ## List available catalog entries through the server
	go run cmd/kronk/main.go catalog list

kronk-catalog-list-local: ## List catalog entries without a server
	go run cmd/kronk/main.go catalog list --local


# make kronk-catalog-show ID="unsloth/Qwen3-1.7B-UD-Q8_K_XL"
kronk-catalog-show: ## Show a catalog entry through the server; ID=<model>
	go run cmd/kronk/main.go catalog show "$(ID)"

# make kronk-catalog-show-local ID="unsloth/Qwen3-1.7B-UD-Q8_K_XL"
kronk-catalog-show-local: ## Show a catalog entry without a server; ID=<model>
	go run cmd/kronk/main.go catalog show --local "$(ID)"


# ------------------------------------------------------------------------------

kronk-security-help: ## Show usage for security commands
	go run cmd/kronk/main.go security --help


kronk-security-key-list: ## List signing keys through the server
	go run cmd/kronk/main.go security key list

kronk-security-key-list-local: ## List local signing keys without a server
	go run cmd/kronk/main.go security key list --local

# make kronk-security-token-create-local D="5m" E="chat-completions"
kronk-security-token-create-local: ## Create a local token; D=<duration> E=<endpoint grants>
	go run cmd/kronk/main.go security token create --local --duration "$(D)" --endpoints "$(E)"

# ------------------------------------------------------------------------------

# Launch OpenCode with the Qwen3.6 35B-A3B coding model.
kronk-launch: ## Launch OpenCode with the Qwen3.6 35B-A3B coding model
	go run cmd/kronk/main.go launch opencode "unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL"

# ==============================================================================
# Bucky (whisper) verbs

bucky-libs: ## Install pinned Whisper libraries through the server
	go run cmd/kronk/main.go bucky libs

bucky-libs-local: ## Install pinned Whisper libraries without a server
	go run cmd/kronk/main.go bucky libs --local

bucky-libs-combinations: ## List supported Whisper platforms through the server
	go run cmd/kronk/main.go bucky libs --list-combinations

bucky-libs-combinations-local: ## List supported Whisper platforms without a server
	go run cmd/kronk/main.go bucky libs --local --list-combinations

bucky-libs-installs: ## List installed Whisper bundles through the server
	go run cmd/kronk/main.go bucky libs --list-installs

bucky-libs-installs-local: ## List installed Whisper bundles without a server
	go run cmd/kronk/main.go bucky libs --local --list-installs

# make bucky-libs-install ARCH=amd64 OS=linux PROC=cuda
bucky-libs-install: ## Install a Whisper bundle via server; ARCH=<arch> OS=<os> PROC=<processor>
	go run cmd/kronk/main.go bucky libs --install --arch="$(ARCH)" --os="$(OS)" --processor="$(PROC)"

# make bucky-libs-install-local ARCH=amd64 OS=linux PROC=cuda
bucky-libs-install-local: ## Install a local Whisper bundle; ARCH=<arch> OS=<os> PROC=<processor>
	go run cmd/kronk/main.go bucky libs --local --install --arch="$(ARCH)" --os="$(OS)" --processor="$(PROC)"

# make bucky-libs-remove-install ARCH=amd64 OS=linux PROC=cuda
bucky-libs-remove-install: ## Remove a Whisper bundle via server; ARCH=<arch> OS=<os> PROC=<processor>
	go run cmd/kronk/main.go bucky libs --remove-install --arch="$(ARCH)" --os="$(OS)" --processor="$(PROC)"

# make bucky-libs-remove-install-local ARCH=amd64 OS=linux PROC=cuda
bucky-libs-remove-install-local: ## Remove a local Whisper bundle; ARCH=<arch> OS=<os> PROC=<processor>
	go run cmd/kronk/main.go bucky libs --local --remove-install --arch="$(ARCH)" --os="$(OS)" --processor="$(PROC)"

# ------------------------------------------------------------------------------

bucky-model-list: ## List downloaded Whisper models through the server
	go run cmd/kronk/main.go bucky model list

bucky-model-list-local: ## List downloaded Whisper models without a server
	go run cmd/kronk/main.go bucky model list --local


# make bucky-model-pull NAME="ggml-tiny.bin"
bucky-model-pull: ## Download a Whisper model through the server; NAME=<model>
	go run cmd/kronk/main.go bucky model pull "$(NAME)"

# make bucky-model-pull-local NAME="ggml-tiny.bin"
bucky-model-pull-local: ## Download a Whisper model without a server; NAME=<model>
	go run cmd/kronk/main.go bucky model pull --local "$(NAME)"


# make bucky-model-remove NAME="ggml-tiny.bin"
bucky-model-remove: ## Remove a Whisper model through the server; NAME=<model>
	go run cmd/kronk/main.go bucky model remove "$(NAME)"

# make bucky-model-remove-local NAME="ggml-tiny.bin"
bucky-model-remove-local: ## Remove a Whisper model without a server; NAME=<model>
	go run cmd/kronk/main.go bucky model remove --local "$(NAME)"
