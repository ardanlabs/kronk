##@ Probes and benchmarks

# Consolidated batch-engine, MTP, IMC, long-context, and media reliability
# probes.
#
# Run against the current source with the dedicated tools model configuration:
#
#   make server-for-tools
#   make test-load-batch-text RELIABILITY_CELLS='Dense/Att/Nslot/IMC+/MTP-/img-'
#   make kronk-server-logs  # Optional: follow the detached server log.
#   make kronk-server-stop
#
# The tools validate the active model configuration but never modify it or
# manage the server. Restart the server after editing zarf/kms/tools_model_config.yaml.
#
# Select profiles and explicit single-GPU paths (or use all). Cells accept
# whitespace-separated identities; quoted multiline lists put one cell per line.
#   make test-load-all RELIABILITY_PROFILES=batch-text RELIABILITY_CELLS='Dense/Att/Nslot/IMC+/MTP-/img-'
#   make test-load-batch-text RELIABILITY_CELLS='Dense/Att/Nslot/IMC+/MTP-/img-'
# Selection resolves pressure and media execution automatically; no extra flags:
#   make test-load-image-prefill RELIABILITY_CELLS='MoE/Hyb/Nslot/IMC+/MTP-/img+'
# Cell 41 runs shared and forced-isolated phases; cell 57 runs oversized IMC.
# Low-level custom investigations still accept explicit SCENARIOS and MATRIX.

#
# Further analysis with some harness: provide summary.json,
# events.ndjson, and tool.log from the same RELIABILITY_OUT, plus
# zarf/kms/tools_model_config.yaml for the expected runtime settings. Do not provide the
# full ~/.kronk/kronk.log unless the curated evidence is unavailable or a
# failure needs deeper investigation. Each invocation replaces RELIABILITY_OUT;
# preserve separate runs with, for example:
#
#   make test-load-mtp-embedded RELIABILITY_CELLS='MoE/Hyb/Nslot/IMC-/MTP-/img-' RELIABILITY_OUT=/tmp/kronk-tools/mtp
#   make test-load-batch-text RELIABILITY_CELLS='MoE/Hyb/Nslot/IMC-/MTP-/img-' RELIABILITY_OUT=/tmp/kronk-tools/batch
#
# Suggested analysis prompt:
#   Analyze MTP drafting and acceptance, IMC exact and append reuse, and batch
#   slot overlap, queue pressure, and isolation. Distinguish tool assertions
#   from trace-correlated server evidence, and report failures or evidence gaps.

# API-only tools server: compile current Go code without regenerating docs/BUI.
# Use the dedicated tools configuration, with normal environment/flag precedence.
server-for-tools: ## Start a detached API server for probes
	KRONK_POOL_MODEL_CONFIG_FILE="$(CURDIR)/zarf/kms/tools_model_config.yaml" \
		go run -buildvcs=true ./cmd/kronk server start --detach --web-admin-enabled=false

QUICK_CHECK_ARGS ?=

quick-check-this-code: ## Run quick checks on the current code
	@go run ./.tools/quick-check $(QUICK_CHECK_ARGS)

QUICK_COMPARE_BASELINE ?=
QUICK_COMPARE_CANDIDATE ?=
QUICK_CHECK_COMPARE_ARGS ?=

quick-compare-against-release: ## Compare quick checks against a release
	@go run ./.tools/quick-check compare \
		-baseline "$(QUICK_COMPARE_BASELINE)" -candidate "$(QUICK_COMPARE_CANDIDATE)" \
		$(QUICK_CHECK_COMPARE_ARGS)

RELIABILITY_HOST ?= http://localhost:11435
RELIABILITY_OUT ?= .tools/reliability/output
RELIABILITY_SERVER_LOG ?= $(HOME)/.kronk/kronk.log
RELIABILITY_TIMEOUT ?= 30m
RELIABILITY_SEED ?= 42
RELIABILITY_REPETITIONS ?= 1
RELIABILITY_RUN_LABEL ?=
RELIABILITY_ARGS ?=
RELIABILITY_MATRIX ?=
RELIABILITY_SCENARIOS ?=
RELIABILITY_PROFILES ?=
RELIABILITY_CELLS ?=

RELIABILITY_MTP_EMBEDDED_MODEL ?= unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL
RELIABILITY_MTP_COMPANION_MODEL ?= unsloth/Qwen3.8-27B-UD-Q6_K
RELIABILITY_MTP_REQUESTS ?= 0
RELIABILITY_MTP_PROMPT_TOKENS ?= 2200
RELIABILITY_MTP_MAX_TOKENS ?= 256
RELIABILITY_MTP_EMBEDDED_SLOTS ?= 4
RELIABILITY_MTP_COMPANION_SLOTS ?= 2

RELIABILITY_TEXT_STATE_MODEL ?= unsloth/Qwen3.8-Flash-Next-UD-Q2_K_XL

RELIABILITY_LONG_CONTEXT_MODEL ?= unsloth/Qwen3.8-Flash-Next-UD-Q2_K_XL
RELIABILITY_LONG_CONTEXT_STAGES ?= 4096,8192,16384,32768,65536,131072
RELIABILITY_LONG_CONTEXT_MAX_TOKENS ?= 96

RELIABILITY_BATCH_MODEL ?= unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL
RELIABILITY_BATCH_TURNS ?= 21
RELIABILITY_BATCH_TARGET_TOKENS ?= 30000
RELIABILITY_BATCH_TOKENS_PER_TURN ?= 1400
RELIABILITY_BATCH_MAX_TOKENS ?= 128
RELIABILITY_BATCH_SLOTS ?= 4
RELIABILITY_BATCH_CONVERSATIONS ?= 8
RELIABILITY_BATCH_IMAGES ?= examples/samples/giraffe.jpg,examples/samples/adetailer-face.png
RELIABILITY_BATCH_IMAGE_SUBJECTS ?= giraffe,astronaut

RELIABILITY_IMAGE_MODEL ?= unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL
RELIABILITY_IMAGE ?= examples/samples/giraffe.jpg
RELIABILITY_IMAGE_EXPECT ?= giraffe
RELIABILITY_IMAGE_STATE_MAX_TOKENS ?= 128
RELIABILITY_IMAGE_PREFILL_GENERATION_MAX_TOKENS ?= 512
RELIABILITY_IMAGE_PREFILL_MAX_TOKENS ?= 64
# Empty keeps the CLI's 1s UX warning; setting a duration opts into a hard limit.
RELIABILITY_IMAGE_PREFILL_MAX_GENERATION_GAP ?=

reliability_profiles_test-load-mtp = mtp-embedded,mtp-companion
reliability_profiles_test-load-batch = batch-text,batch-images
reliability_profiles_test-load-images = image-state,image-prefill

define run_reliability
	$(if $(strip $(RELIABILITY_CELLS) $(RELIABILITY_PROFILES)),go run ./.tools/reliability \
		-profiles "$(if $(RELIABILITY_PROFILES),$(RELIABILITY_PROFILES),$(if $(filter test-load-all,$@),,$(if $(reliability_profiles_$@),$(reliability_profiles_$@),$(patsubst test-load-%,%,$@))))" \
		-cells "$(strip $(RELIABILITY_CELLS))" \
		-host "$(RELIABILITY_HOST)" -out "$(RELIABILITY_OUT)" \
		-server-log "$(RELIABILITY_SERVER_LOG)" -timeout "$(RELIABILITY_TIMEOUT)" \
		-seed "$(RELIABILITY_SEED)" -repetitions "$(RELIABILITY_REPETITIONS)" \
		$(if $(strip $(RELIABILITY_IMAGE_PREFILL_MAX_GENERATION_GAP)),-image-prefill-max-generation-gap "$(RELIABILITY_IMAGE_PREFILL_MAX_GENERATION_GAP)") \
		-run-label "$(RELIABILITY_RUN_LABEL)" $(RELIABILITY_ARGS),\
	go run ./.tools/reliability \
		-scenario "$(1)" \
		-host "$(RELIABILITY_HOST)" \
		-out "$(RELIABILITY_OUT)" \
		-server-log "$(RELIABILITY_SERVER_LOG)" \
		-timeout "$(RELIABILITY_TIMEOUT)" \
		-seed "$(RELIABILITY_SEED)" \
		-repetitions "$(RELIABILITY_REPETITIONS)" \
		-run-label "$(RELIABILITY_RUN_LABEL)" \
		-matrix "$(RELIABILITY_MATRIX)" \
		-mtp-embedded-model "$(RELIABILITY_MTP_EMBEDDED_MODEL)" \
		-mtp-companion-model "$(RELIABILITY_MTP_COMPANION_MODEL)" \
		-mtp-requests "$(RELIABILITY_MTP_REQUESTS)" \
		-mtp-prompt-tokens "$(RELIABILITY_MTP_PROMPT_TOKENS)" \
		-mtp-max-tokens "$(RELIABILITY_MTP_MAX_TOKENS)" \
		-mtp-embedded-slots "$(RELIABILITY_MTP_EMBEDDED_SLOTS)" \
		-mtp-companion-slots "$(RELIABILITY_MTP_COMPANION_SLOTS)" \
		-text-state-model "$(RELIABILITY_TEXT_STATE_MODEL)" \
		-long-context-model "$(RELIABILITY_LONG_CONTEXT_MODEL)" \
		-long-context-stages "$(RELIABILITY_LONG_CONTEXT_STAGES)" \
		-long-context-max-tokens "$(RELIABILITY_LONG_CONTEXT_MAX_TOKENS)" \
		-batch-model "$(RELIABILITY_BATCH_MODEL)" \
		-batch-turns "$(RELIABILITY_BATCH_TURNS)" \
		-batch-target-tokens "$(RELIABILITY_BATCH_TARGET_TOKENS)" \
		-batch-tokens-per-turn "$(RELIABILITY_BATCH_TOKENS_PER_TURN)" \
		-batch-max-tokens "$(RELIABILITY_BATCH_MAX_TOKENS)" \
		-batch-slots "$(RELIABILITY_BATCH_SLOTS)" \
		-batch-conversations "$(RELIABILITY_BATCH_CONVERSATIONS)" \
		-batch-images "$(RELIABILITY_BATCH_IMAGES)" \
		-batch-image-subjects "$(RELIABILITY_BATCH_IMAGE_SUBJECTS)" \
		-image-model "$(RELIABILITY_IMAGE_MODEL)" \
		-image-file "$(RELIABILITY_IMAGE)" \
		-image-expect "$(RELIABILITY_IMAGE_EXPECT)" \
		-image-state-max-tokens "$(RELIABILITY_IMAGE_STATE_MAX_TOKENS)" \
		-image-prefill-generation-max-tokens "$(RELIABILITY_IMAGE_PREFILL_GENERATION_MAX_TOKENS)" \
		-image-prefill-max-tokens "$(RELIABILITY_IMAGE_PREFILL_MAX_TOKENS)" \
		$(if $(strip $(RELIABILITY_IMAGE_PREFILL_MAX_GENERATION_GAP)),-image-prefill-max-generation-gap "$(RELIABILITY_IMAGE_PREFILL_MAX_GENERATION_GAP)") \
		$(2) $(RELIABILITY_ARGS))
endef

# -------------------------------

# Single-slot text state-integrity probe for Hybrid and Attention-only models.
# Exercises deterministic cold/repeat
# generation, exact and append IMC reuse, cancellation, and recovery.
# With IMC off, reuse is forbidden; state correctness checks remain active.
test-load-text-state: ## Run text state and recovery probes
	$(call run_reliability,text-state,)

# Image correctness, replay/append, cancellation, slot release, and image/text recovery.
test-load-image-state: ## Run image state and recovery probes
	$(call run_reliability,images,-image-profile state)

# Staged begin/middle/end marker retrieval and warm-IMC probe. Targets beyond
# the model's configured context window are reported as skipped.
# With IMC off, the repeat tests full-prefill correctness instead of reuse.
test-load-long-context: ## Run long context reliability probes
	$(call run_reliability,long-context,)

# Concurrent image prefill during generation, with bounded text gaps and
# request-correlated evidence of the selected image execution path.
test-load-image-prefill: ## Run image prefill responsiveness probes
	$(call run_reliability,images,-image-profile prefill)

# -------------------------------

# Text and image batch profiles: eight conversations / four slots, per-turn
# slot ownership and prepared wait/release coverage, workload-wide generation
# progress during prefill, marker isolation, IMC reuse, and 30K-token histories.
test-load-batch: ## Run batch reliability probes
	$(call run_reliability,batch,-batch-profile all)

# Text-only batch histories and isolation.
test-load-batch-text: ## Run text batch reliability probes
	$(call run_reliability,batch,-batch-profile text)

# Batch with image history, subject changes, and replays; no output-equality check.
test-load-batch-images: ## Run image batch reliability probes
	$(call run_reliability,batch,-batch-profile images)

# -------------------------------

# Image state and prefill group; each profile remains independently selectable.
test-load-images: ## Run image state and prefill probes
	$(call run_reliability,images,-image-profile all)

# Both embedded-head and companion/own-KV MTP profiles.
# With MTP off these are concurrent target-only controls, not drafter tests.
test-load-mtp: ## Run embedded and companion MTP probes
	$(call run_reliability,mtp,-mtp-profile all)

# Calibrated requests, supplied-value/session isolation and post-wave recovery.
test-load-mtp-embedded: ## Run embedded MTP pressure and recovery probes
	$(call run_reliability,mtp,-mtp-profile embedded)

test-load-mtp-companion: ## Run companion MTP pressure and recovery probes
	$(call run_reliability,mtp,-mtp-profile companion)

# -------------------------------

# Runs only load/reliability scenarios, sequentially in one artifact set.
# The separately configured lifecycle probe is intentionally excluded.
test-load-all: ## Run explicitly selected reliability profiles and cells
	$(call run_reliability,$(RELIABILITY_SCENARIOS),)

# ==============================================================================

# Exercises the server's four-stage request lifecycle with one execution slot
# and two admission permits. It holds Stage 4 open, verifies a queued request
# cancels in Stage 3, verifies a third request times out in Stage 1, then
# cancels the holder and confirms the slot and admission permit are released.
# The selected model must use nseq-max: 1, queue-depth: 2, and
# admission-timeout: 100ms; see .tools/lifecycle-load/main.go for setup details.
# Requires nseq-max: 1
LIFECYCLE_LOAD_OUT ?= .tools/lifecycle-load/output
LIFECYCLE_SERVER_LOG ?= $(HOME)/.kronk/kronk.log

example-lifecycle-load: ## Probe server admission, cancellation, and slot release
	KRONK_LIFECYCLE_OUT="$(LIFECYCLE_LOAD_OUT)" \
	KRONK_SERVER_LOG="$(LIFECYCLE_SERVER_LOG)" \
	go run ./.tools/lifecycle-load

# ==============================================================================

# Adversarial probe harness against a running (or self-started) Kronk server.
# Everything is env-tunable — see .tools/adversarial/adversarial.sh -h. Results
# land in .tools/adversarial/output. Pass groups or a tier through ARGS:
#   make test-adversarial ARGS="--tier=smoke"
#   make test-adversarial ARGS="stream structured"
# Set SERVER=1 to start and manage a server instead of probing the running one.
#
# The script exits 1 when it flags anything. Print the triage prompt regardless, then
# re-raise the script's status so the target still fails on findings.
# Requires nseq-max: 4
test-adversarial: ## Run adversarial probes and print the triage prompt
	@echo ========== RUN ADVERSARIAL PROBES ==========
	@.tools/adversarial/adversarial.sh $(ARGS); \
	status=$$?; \
	echo; \
	echo "========== TRIAGE PROMPT =========="; \
	echo "Hand this to a coding agent to turn the findings into a verified bug report:"; \
	echo; \
	cat .tools/adversarial/adversarial-triage.md; \
	exit $$status

# ==============================================================================

# OpenCode coding-agent benchmark against a separately managed Kronk server.
# Eligible models come exclusively from /AGENT entries in zarf/kms/model_config.yaml.
CODEGEN_HOST ?= http://localhost:11435
CODEGEN_MODEL ?= all
CODEGEN_ATTEMPTS ?= 1
CODEGEN_STEPS ?= 40
CODEGEN_TIMEOUT ?= 30m

benchmark-codegen: ## Run the OpenCode coding benchmark
	go run ./.tools/codegen-benchmark \
		-host "$(CODEGEN_HOST)" \
		-model "$(CODEGEN_MODEL)" \
		-attempts "$(CODEGEN_ATTEMPTS)" \
		-steps "$(CODEGEN_STEPS)" \
		-timeout "$(CODEGEN_TIMEOUT)"

benchmark-codegen-list: ## List coding benchmark models
	go run ./.tools/codegen-benchmark -list

benchmark-codegen-report: ## Show the coding benchmark report
	go run ./.tools/codegen-benchmark -report
