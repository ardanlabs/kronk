# Consolidated batch-engine, MTP, IMC, long-context, and media reliability
# probes. Each invocation replaces RELIABILITY_OUT with summary.json,
# events.ndjson, and tool.log. The runner reads only bytes appended to the
# detached server log during the invocation and retains high-value events whose
# trace IDs match its requests. Use a unique RELIABILITY_OUT to preserve a run.

RELIABILITY_HOST ?= http://localhost:11435
RELIABILITY_OUT ?= .tools/reliability/output
RELIABILITY_SERVER_LOG ?= $(HOME)/.kronk/kronk.log
RELIABILITY_TIMEOUT ?= 30m
RELIABILITY_SEED ?= 42
RELIABILITY_ARGS ?=

RELIABILITY_MTP_EMBEDDED_MODEL ?= unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT
RELIABILITY_MTP_COMPANION_MODEL ?= unsloth/Qwen3.8-27B-UD-Q4_K_XL/AGENT
RELIABILITY_MTP_REQUESTS ?= 0
RELIABILITY_MTP_PROMPT_TOKENS ?= 2200
RELIABILITY_MTP_MAX_TOKENS ?= 256
RELIABILITY_MTP_EMBEDDED_SLOTS ?= 4
RELIABILITY_MTP_COMPANION_SLOTS ?= 2

RELIABILITY_HYBRID_MODEL ?= unsloth/Qwen3.8-Flash-Next-UD-Q2_K_XL/AGENT

RELIABILITY_LONG_CONTEXT_MODEL ?= unsloth/Qwen3.8-Flash-Next-UD-Q2_K_XL/AGENT
RELIABILITY_LONG_CONTEXT_STAGES ?= 4096,8192,16384,32768,65536,131072
RELIABILITY_LONG_CONTEXT_MAX_TOKENS ?= 96

RELIABILITY_BATCH_MODEL ?= unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT
RELIABILITY_BATCH_TURNS ?= 21
RELIABILITY_BATCH_TARGET_TOKENS ?= 30000
RELIABILITY_BATCH_TOKENS_PER_TURN ?= 1400
RELIABILITY_BATCH_MAX_TOKENS ?= 128
RELIABILITY_BATCH_SLOTS ?= 4
RELIABILITY_BATCH_CONVERSATIONS ?= 5

RELIABILITY_MEDIA_MODEL ?= unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT
RELIABILITY_MEDIA_IMAGE ?= examples/samples/giraffe.jpg
RELIABILITY_MEDIA_EXPECT ?= giraffe
RELIABILITY_MEDIA_MAX_TOKENS ?= 128
RELIABILITY_MEDIA_GENERATION_MAX_TOKENS ?= 512
RELIABILITY_MEDIA_IMAGE_MAX_TOKENS ?= 64
RELIABILITY_MEDIA_MAX_GENERATION_GAP ?= 1s

define run_reliability
	go run ./.tools/reliability \
		-scenario "$(1)" \
		-host "$(RELIABILITY_HOST)" \
		-out "$(RELIABILITY_OUT)" \
		-server-log "$(RELIABILITY_SERVER_LOG)" \
		-timeout "$(RELIABILITY_TIMEOUT)" \
		-seed "$(RELIABILITY_SEED)" \
		-mtp-embedded-model "$(RELIABILITY_MTP_EMBEDDED_MODEL)" \
		-mtp-companion-model "$(RELIABILITY_MTP_COMPANION_MODEL)" \
		-mtp-requests "$(RELIABILITY_MTP_REQUESTS)" \
		-mtp-prompt-tokens "$(RELIABILITY_MTP_PROMPT_TOKENS)" \
		-mtp-max-tokens "$(RELIABILITY_MTP_MAX_TOKENS)" \
		-mtp-embedded-slots "$(RELIABILITY_MTP_EMBEDDED_SLOTS)" \
		-mtp-companion-slots "$(RELIABILITY_MTP_COMPANION_SLOTS)" \
		-hybrid-model "$(RELIABILITY_HYBRID_MODEL)" \
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
		-media-model "$(RELIABILITY_MEDIA_MODEL)" \
		-media-image "$(RELIABILITY_MEDIA_IMAGE)" \
		-media-expect "$(RELIABILITY_MEDIA_EXPECT)" \
		-media-max-tokens "$(RELIABILITY_MEDIA_MAX_TOKENS)" \
		-media-generation-max-tokens "$(RELIABILITY_MEDIA_GENERATION_MAX_TOKENS)" \
		-media-image-max-tokens "$(RELIABILITY_MEDIA_IMAGE_MAX_TOKENS)" \
		-media-max-generation-gap "$(RELIABILITY_MEDIA_MAX_GENERATION_GAP)" \
		$(2) $(RELIABILITY_ARGS)
endef

# Both embedded-head and separate companion/own-KV MTP profiles.
test-load-mtp:
	$(call run_reliability,mtp,-mtp-profile all)

test-load-mtp-embedded:
	$(call run_reliability,mtp,-mtp-profile embedded)

test-load-mtp-companion:
	$(call run_reliability,mtp,-mtp-profile companion)

test-load-hybrid:
	$(call run_reliability,hybrid-state,)

test-load-long-context:
	$(call run_reliability,long-context,)

test-load-batch:
	$(call run_reliability,batch,)

# Media group and independently selectable subprofiles.
test-load-media:
	$(call run_reliability,media,-media-profile all)

test-load-media-correctness:
	$(call run_reliability,media,-media-profile correctness)

test-load-media-prefill:
	$(call run_reliability,media,-media-profile prefill)

# Runs only load/reliability scenarios, sequentially in one artifact set.
# The separately configured lifecycle probe is intentionally excluded.
test-load-all:
	$(call run_reliability,all,)

# ==============================================================================

# HTTP request capture, replay, and streaming-response inspection.

HTTP_CAPTURE_LISTEN_HOST ?= 127.0.0.1
HTTP_CAPTURE_LISTEN_PORT ?= 11436
HTTP_CAPTURE_UPSTREAM_HOST ?= 127.0.0.1
HTTP_CAPTURE_UPSTREAM_PORT ?= 11435
HTTP_CAPTURE_OUT ?= .tools/http-capture/output

# Replaces HTTP_CAPTURE_OUT at startup so it contains only the current capture.
http-capture:
	python3 .tools/http-capture/capture-proxy.py \
		--listen-host "$(HTTP_CAPTURE_LISTEN_HOST)" \
		--listen-port "$(HTTP_CAPTURE_LISTEN_PORT)" \
		--upstream-host "$(HTTP_CAPTURE_UPSTREAM_HOST)" \
		--upstream-port "$(HTTP_CAPTURE_UPSTREAM_PORT)" \
		--output-dir "$(HTTP_CAPTURE_OUT)"

HTTP_REPLAY_CAPTURE ?= .tools/http-capture/output
HTTP_REPLAY_OUT ?= .tools/http-replay/output
HTTP_REPLAY_UPSTREAM ?= http://127.0.0.1:11435

http-replay:
	bash .tools/http-replay/replay.sh \
		"$(HTTP_REPLAY_CAPTURE)" \
		"$(HTTP_REPLAY_OUT)" \
		"$(HTTP_REPLAY_UPSTREAM)"

http-inspect-sse:
	@test -n "$(SSE_FILES)" || { echo 'usage: make http-inspect-sse SSE_FILES="response-*.sse"' >&2; exit 2; }
	python3 .tools/http-replay/inspect-sse.py $(SSE_FILES)

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
test-adversarial:
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

# Exercises the server's four-stage request lifecycle with one execution slot
# and two admission permits. It holds Stage 4 open, verifies a queued request
# cancels in Stage 3, verifies a third request times out in Stage 1, then
# cancels the holder and confirms the slot and admission permit are released.
# The selected model must use nseq-max: 1, queue-depth: 2, and
# admission-timeout: 100ms; see .tools/lifecycle-load/main.go for setup details.
# Requires nseq-max: 1
LIFECYCLE_LOAD_OUT ?= .tools/lifecycle-load/output
LIFECYCLE_SERVER_LOG ?= $(HOME)/.kronk/kronk.log

example-lifecycle-load:
	KRONK_LIFECYCLE_OUT="$(LIFECYCLE_LOAD_OUT)" \
	KRONK_SERVER_LOG="$(LIFECYCLE_SERVER_LOG)" \
	go run ./.tools/lifecycle-load

# ==============================================================================

# OpenCode coding-agent benchmark against a separately managed Kronk server.
# Eligible models come exclusively from /AGENT entries in zarf/kms/model_config.yaml.
CODEGEN_HOST ?= http://localhost:11435
CODEGEN_MODEL ?= all
CODEGEN_ATTEMPTS ?= 1
CODEGEN_STEPS ?= 40
CODEGEN_TIMEOUT ?= 30m

benchmark-codegen:
	go run ./.tools/codegen-benchmark \
		-host "$(CODEGEN_HOST)" \
		-model "$(CODEGEN_MODEL)" \
		-attempts "$(CODEGEN_ATTEMPTS)" \
		-steps "$(CODEGEN_STEPS)" \
		-timeout "$(CODEGEN_TIMEOUT)"

benchmark-codegen-list:
	go run ./.tools/codegen-benchmark -list

benchmark-codegen-report:
	go run ./.tools/codegen-benchmark -report
