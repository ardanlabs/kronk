# MTP concurrent load probe

MTP_LOAD_HOST ?= http://localhost:11435
MTP_LOAD_MODEL ?= unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT
MTP_LOAD_REQUESTS ?= 2
MTP_LOAD_PROMPT_TOKENS ?= 2200
MTP_LOAD_MAX_TOKENS ?= 256
MTP_LOAD_SEED ?= 42
MTP_LOAD_OUT ?= .tools/mtp-load/output

# Builds distinct prompts, calibrates each one with /v1/tokenize, releases all
# requests through one barrier, and saves request/response JSON plus summary.json.
# MTP_LOAD_OUT must not already exist so one experiment cannot overwrite another.
mtp-load-parallel:
	python3 .tools/mtp-load/mtp-load.py \
		--host "$(MTP_LOAD_HOST)" \
		--model "$(MTP_LOAD_MODEL)" \
		--requests "$(MTP_LOAD_REQUESTS)" \
		--prompt-tokens "$(MTP_LOAD_PROMPT_TOKENS)" \
		--max-tokens "$(MTP_LOAD_MAX_TOKENS)" \
		--seed "$(MTP_LOAD_SEED)" \
		--out "$(MTP_LOAD_OUT)"

# Sends one known-good coding prompt through a barrier to verify a one-slot
# model can complete five concurrent requests. Responses stay in memory.
LOAD_HOST ?= http://localhost:11435
LOAD_MODEL ?= mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT
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

# Long-running multi-slot batch-isolation probe. Every turn starts all
# conversations together, proves their streamed generation overlaps, and
# verifies that no response contains another conversation's marker. Set both
# BATCH_LOAD_SLOTS and BATCH_LOAD_CONVERSATIONS to 4 to exercise four slots.
BATCH_LOAD_HOST ?= http://localhost:11435
BATCH_LOAD_MODEL ?= unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT
BATCH_LOAD_TURNS ?= 21
BATCH_LOAD_TARGET_TOKENS ?= 30000
BATCH_LOAD_TOKENS_PER_TURN ?= 1400
BATCH_LOAD_MAX_TOKENS ?= 128
BATCH_LOAD_SLOTS ?= 3
BATCH_LOAD_CONVERSATIONS ?= 3
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

# HTTP request capture, replay, and streaming-response inspection.

HTTP_CAPTURE_LISTEN_HOST ?= 127.0.0.1
HTTP_CAPTURE_LISTEN_PORT ?= 11436
HTTP_CAPTURE_UPSTREAM_HOST ?= 127.0.0.1
HTTP_CAPTURE_UPSTREAM_PORT ?= 11435
HTTP_CAPTURE_OUT ?= .tools/http-capture/output

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

example-lifecycle-load:
	cd examples && go run ./lifecycle-load/main.go
