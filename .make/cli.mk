# ==============================================================================
# Kronk CLI — llama (kronk) verbs

kronk-libs:
	go run cmd/kronk/main.go libs

kronk-libs-local: install-libraries

# ------------------------------------------------------------------------------

kronk-model-index:
	go run cmd/kronk/main.go model index

kronk-model-index-local:
	go run cmd/kronk/main.go model index --local


kronk-model-list:
	go run cmd/kronk/main.go model list

kronk-model-list-local:
	go run cmd/kronk/main.go model list --local


# make kronk-model-pull URL="Qwen/Qwen3-8B-Q8_0.gguf"
kronk-model-pull:
	go run cmd/kronk/main.go model pull "$(URL)"

# make kronk-model-pull-local URL="Qwen/Qwen3-8B-Q8_0.gguf"
kronk-model-pull-local:
	go run cmd/kronk/main.go model pull --local "$(URL)"


kronk-model-ps:
	go run cmd/kronk/main.go model ps


# make kronk-model-remove ID="bartowski/cerebras_qwen3-coder-reap-25b-a3b-q8_0"
kronk-model-remove:
	go run cmd/kronk/main.go model remove "$(ID)"

# make kronk-model-remove-local ID="bartowski/cerebras_qwen3-coder-reap-25b-a3b-q8_0"
kronk-model-remove-local:
	go run cmd/kronk/main.go model remove --local "$(ID)"


# make kronk-model-show ID="Qwen/Qwen3-8B-Q8_0"
kronk-model-show:
	go run cmd/kronk/main.go model show "$(ID)"

# make kronk-model-show-local ID="Qwen/Qwen3-8B-Q8_0"
kronk-model-show-local:
	go run cmd/kronk/main.go model show --local "$(ID)"

# ------------------------------------------------------------------------------

kronk-catalog-list:
	go run cmd/kronk/main.go catalog list

kronk-catalog-list-local:
	go run cmd/kronk/main.go catalog list --local


# make kronk-catalog-show ID="Qwen/Qwen3-8B-Q8_0"
kronk-catalog-show:
	go run cmd/kronk/main.go catalog show "$(ID)"

# make kronk-catalog-show-local ID="Qwen/Qwen3-8B-Q8_0"
kronk-catalog-show-local:
	go run cmd/kronk/main.go catalog show --local "$(ID)"


# ------------------------------------------------------------------------------

kronk-security-help:
	go run cmd/kronk/main.go security --help


kronk-security-key-list:
	go run cmd/kronk/main.go security key list

kronk-security-key-list-local:
	go run cmd/kronk/main.go security key list --local

# make kronk-security-token-create-local U="bill" D="5m" E="chat-completions"
kronk-security-token-create-local:
	go run cmd/kronk/main.go security token create --local --username "$(U)" --duration "$(D)" --endpoints "$(E)"

# ------------------------------------------------------------------------------

# make kronk-run ID="Qwen/Qwen3-8B-Q8_0"
kronk-run:
	go run cmd/kronk/main.go run "$(ID)"

# ==============================================================================
# Bucky (whisper) verbs

bucky-libs:
	go run cmd/kronk/main.go bucky libs

bucky-libs-local:
	go run cmd/kronk/main.go bucky libs --local

bucky-libs-combinations:
	go run cmd/kronk/main.go bucky libs --list-combinations

bucky-libs-combinations-local:
	go run cmd/kronk/main.go bucky libs --local --list-combinations

bucky-libs-installs:
	go run cmd/kronk/main.go bucky libs --list-installs

bucky-libs-installs-local:
	go run cmd/kronk/main.go bucky libs --local --list-installs

# make bucky-libs-install ARCH=amd64 OS=linux PROC=cuda
bucky-libs-install:
	go run cmd/kronk/main.go bucky libs --install --arch="$(ARCH)" --os="$(OS)" --processor="$(PROC)"

# make bucky-libs-install-local ARCH=amd64 OS=linux PROC=cuda
bucky-libs-install-local:
	go run cmd/kronk/main.go bucky libs --local --install --arch="$(ARCH)" --os="$(OS)" --processor="$(PROC)"

# make bucky-libs-remove-install ARCH=amd64 OS=linux PROC=cuda
bucky-libs-remove-install:
	go run cmd/kronk/main.go bucky libs --remove-install --arch="$(ARCH)" --os="$(OS)" --processor="$(PROC)"

# make bucky-libs-remove-install-local ARCH=amd64 OS=linux PROC=cuda
bucky-libs-remove-install-local:
	go run cmd/kronk/main.go bucky libs --local --remove-install --arch="$(ARCH)" --os="$(OS)" --processor="$(PROC)"

# ------------------------------------------------------------------------------

bucky-model-list:
	go run cmd/kronk/main.go bucky model list

bucky-model-list-local:
	go run cmd/kronk/main.go bucky model list --local


# make bucky-model-pull NAME="tiny.en"
bucky-model-pull:
	go run cmd/kronk/main.go bucky model pull "$(NAME)"

# make bucky-model-pull-local NAME="tiny.en"
bucky-model-pull-local:
	go run cmd/kronk/main.go bucky model pull --local "$(NAME)"


# make bucky-model-remove NAME="tiny.en"
bucky-model-remove:
	go run cmd/kronk/main.go bucky model remove "$(NAME)"

# make bucky-model-remove-local NAME="tiny.en"
bucky-model-remove-local:
	go run cmd/kronk/main.go bucky model remove --local "$(NAME)"
