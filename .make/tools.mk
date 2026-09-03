# MTP concurrent load probe

MTP_LOAD_HOST ?= http://localhost:11435
MTP_LOAD_MODEL ?= unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT
MTP_LOAD_REQUESTS ?= 2
MTP_LOAD_PROMPT_TOKENS ?= 2200
MTP_LOAD_MAX_TOKENS ?= 256
MTP_LOAD_EXPECTED_SLOTS ?= 4
MTP_LOAD_SEED ?= 42
MTP_LOAD_OUT ?= .tools/mtp-load/output

# Builds distinct prompts, calibrates each one with /v1/tokenize, releases all
# requests through one barrier, and saves request/response JSON plus summary.json.
# Replaces MTP_LOAD_OUT at startup so it contains only the current experiment.
# Requires nseq-max: MTP_LOAD_EXPECTED_SLOTS.
mtp-load-parallel:
	python3 .tools/mtp-load/mtp-load.py \
		--host "$(MTP_LOAD_HOST)" \
		--model "$(MTP_LOAD_MODEL)" \
		--requests "$(MTP_LOAD_REQUESTS)" \
		--prompt-tokens "$(MTP_LOAD_PROMPT_TOKENS)" \
		--max-tokens "$(MTP_LOAD_MAX_TOKENS)" \
		--expected-slots "$(MTP_LOAD_EXPECTED_SLOTS)" \
		--seed "$(MTP_LOAD_SEED)" \
		--require-mtp \
		--out "$(MTP_LOAD_OUT)"

# Sends one known-good coding prompt through a barrier to verify a one-slot
# model can complete five concurrent requests. Responses stay in memory.
# Requires nseq-max: 1
LOAD_HOST ?= http://localhost:11435
LOAD_MODEL ?= unsloth/Qwen3.8-Flash-Next-UD-Q2_K_XL/AGENT
LOAD_REQUESTS ?= 5
LOAD_MAX_TOKENS ?= 4096
LOAD_SEED ?= 42

test-load:
	python3 .tools/mtp-load/mtp-load.py \
		--host "$(LOAD_HOST)" \
		--model "$(LOAD_MODEL)" \
		--requests "$(LOAD_REQUESTS)" \
		--max-tokens "$(LOAD_MAX_TOKENS)" \
		--seed "$(LOAD_SEED)" \
		--scenario tic-tac-toe

# ==============================================================================

# Single-slot hybrid state-integrity probe. Exercises deterministic cold/warm
# generation, IMC exact and append reuse, cancellation recovery, and MTP usage
# when the selected model exposes an embedded head.
HYBRID_LOAD_HOST ?= http://localhost:11435
HYBRID_LOAD_MODEL ?= unsloth/Qwen3.8-Flash-Next-UD-Q2_K_XL/AGENT
HYBRID_LOAD_TIMEOUT ?= 1800
HYBRID_LOAD_OUT ?= .tools/hybrid-load/output/summary.json

test-hybrid-load:
	python3 .tools/hybrid-load/hybrid-load.py \
		--host "$(HYBRID_LOAD_HOST)" \
		--model "$(HYBRID_LOAD_MODEL)" \
		--timeout "$(HYBRID_LOAD_TIMEOUT)" \
		--out "$(HYBRID_LOAD_OUT)"

# ==============================================================================

# Staged exact-needle retrieval and warm-IMC probe. Targets beyond the model's
# configured context are reported as skipped.
LONG_CONTEXT_LOAD_HOST ?= http://localhost:11435
LONG_CONTEXT_LOAD_MODEL ?= unsloth/Qwen3.8-Flash-Next-UD-Q2_K_XL/AGENT
LONG_CONTEXT_LOAD_STAGES ?= 4096,8192,16384,32768,65536,131072
LONG_CONTEXT_LOAD_MAX_TOKENS ?= 96
LONG_CONTEXT_LOAD_TIMEOUT ?= 1800
LONG_CONTEXT_LOAD_SEED ?= 42
LONG_CONTEXT_LOAD_OUT ?= .tools/long-context-load/output/summary.json

test-long-context-load:
	python3 .tools/long-context-load/long-context-load.py \
		--host "$(LONG_CONTEXT_LOAD_HOST)" \
		--model "$(LONG_CONTEXT_LOAD_MODEL)" \
		--stages "$(LONG_CONTEXT_LOAD_STAGES)" \
		--max-tokens "$(LONG_CONTEXT_LOAD_MAX_TOKENS)" \
		--timeout "$(LONG_CONTEXT_LOAD_TIMEOUT)" \
		--seed "$(LONG_CONTEXT_LOAD_SEED)" \
		--out "$(LONG_CONTEXT_LOAD_OUT)"

# ==============================================================================

# One-slot multimodal correctness, media IMC reuse, and modality-state isolation
# probe. This is complementary to test-media-load, which requires concurrent slots.
MEDIA_SMOKE_HOST ?= http://localhost:11435
MEDIA_SMOKE_MODEL ?= unsloth/Qwen3.8-Flash-Next-UD-Q2_K_XL/AGENT
MEDIA_SMOKE_IMAGE ?= examples/samples/giraffe.jpg
MEDIA_SMOKE_MAX_TOKENS ?= 128
MEDIA_SMOKE_TIMEOUT ?= 1800
MEDIA_SMOKE_OUT ?= .tools/media-smoke/output/summary.json

test-media-smoke:
	python3 .tools/media-smoke/media-smoke.py \
		--host "$(MEDIA_SMOKE_HOST)" \
		--model "$(MEDIA_SMOKE_MODEL)" \
		--image "$(MEDIA_SMOKE_IMAGE)" \
		--max-tokens "$(MEDIA_SMOKE_MAX_TOKENS)" \
		--timeout "$(MEDIA_SMOKE_TIMEOUT)" \
		--out "$(MEDIA_SMOKE_OUT)"

# ==============================================================================

# Long-running multi-slot batch-isolation probe. Every turn starts all
# conversations together, proves their streamed generation overlaps, and
# verifies that no response contains another conversation's marker. Use
# BATCH_LOAD_SLOTS and BATCH_LOAD_CONVERSATIONS to exercise N slots.
# Requires nseq-max: 4
BATCH_LOAD_HOST ?= http://localhost:11435
BATCH_LOAD_MODEL ?= unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT
BATCH_LOAD_TURNS ?= 21
BATCH_LOAD_TARGET_TOKENS ?= 30000
BATCH_LOAD_TOKENS_PER_TURN ?= 1400
BATCH_LOAD_MAX_TOKENS ?= 128
BATCH_LOAD_SLOTS ?= 4
BATCH_LOAD_CONVERSATIONS ?= 4
BATCH_LOAD_OUT ?= .tools/batch-load/output/summary.json

test-batch-load:
	python3 .tools/batch-load/batch-load.py \
		--host "$(BATCH_LOAD_HOST)" \
		--model "$(BATCH_LOAD_MODEL)" \
		--turns "$(BATCH_LOAD_TURNS)" \
		--target-tokens "$(BATCH_LOAD_TARGET_TOKENS)" \
		--tokens-per-turn "$(BATCH_LOAD_TOKENS_PER_TURN)" \
		--max-tokens "$(BATCH_LOAD_MAX_TOKENS)" \
		--slots "$(BATCH_LOAD_SLOTS)" \
		--conversations "$(BATCH_LOAD_CONVERSATIONS)" \
		--out "$(BATCH_LOAD_OUT)"

# ==============================================================================

# Media-prefill concurrency probe. Starts a deterministic streaming text
# generation, then submits an image and verifies phase-2 media prefill never
# creates an excessive gap between text content events. The loaded multimodal
# model must have at least two slots. All timing and output knobs are tunable.
# Requires nseq-max: 4
MEDIA_LOAD_HOST ?= http://localhost:11435
MEDIA_LOAD_MODEL ?= unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT
MEDIA_LOAD_IMAGE ?= examples/samples/giraffe.jpg
MEDIA_LOAD_GENERATION_MAX_TOKENS ?= 512
MEDIA_LOAD_IMAGE_MAX_TOKENS ?= 64
MEDIA_LOAD_MAX_GENERATION_CONTENT_EVENT_GAP ?= 1.0
MEDIA_LOAD_TIMEOUT ?= 1800
MEDIA_LOAD_OUT ?= .tools/media-load/output/summary.json

test-media-load:
	python3 .tools/media-load/media-load.py \
		--host "$(MEDIA_LOAD_HOST)" \
		--model "$(MEDIA_LOAD_MODEL)" \
		--image "$(MEDIA_LOAD_IMAGE)" \
		--generation-max-tokens "$(MEDIA_LOAD_GENERATION_MAX_TOKENS)" \
		--image-max-tokens "$(MEDIA_LOAD_IMAGE_MAX_TOKENS)" \
		--max-generation-content-event-gap "$(MEDIA_LOAD_MAX_GENERATION_CONTENT_EVENT_GAP)" \
		--timeout "$(MEDIA_LOAD_TIMEOUT)" \
		--out "$(MEDIA_LOAD_OUT)"

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
example-lifecycle-load:
	go run .tools/lifecycle-load/main.go

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
