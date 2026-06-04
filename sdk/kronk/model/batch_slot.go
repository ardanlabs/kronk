package model

import (
	"context"
	"strings"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"go.opentelemetry.io/otel/trace"
)

// chatJob represents a validated chat request ready for batch processing.
// Created by submitToBatchEngine after request validation and cache lookup.
type chatJob struct {

	// -------------------------------------------------------------------------
	// Request Identity

	id            string              // Unique request ID for logging and responses
	ctx           context.Context     // Request context for cancellation and tracing
	ch            chan<- ChatResponse // Channel for streaming responses back to caller
	queueWaitSpan trace.Span          // Span covering time spent waiting in the queue
	queuedAt      time.Time           // Time when the job was submitted to the queue
	requestStart  time.Time           // Time when the request entered the SDK (for end-to-end TTFT)

	// -------------------------------------------------------------------------
	// Request Content

	d      D        // Original request document (messages, parameters)
	object string   // Request type: ObjectChatText or ObjectChatMedia
	prompt string   // Templated prompt string ready for tokenization
	media  [][]byte // Raw media bytes (images/audio) for vision/audio models
	params Params   // Sampling and generation parameters

	// -------------------------------------------------------------------------
	// Incremental Message Cache (IMC)

	imcSession      *imcSession // Matched IMC session (the session-pool entry whose KV state will be restored into the assigned slot)
	imcSessionMedia bool        // True if session has media (snapshot at job creation; safe to read without lock)
	imcSlotID       int         // Session-pool index (== imcSession.slotID); used by imcClearPending lookup and log correlation
	imcCacheHit     bool        // True if conversation history was found in cache
	imcExpectedHash string      // Expected cachedMsgsHash for stale detection at startSlot (a concurrent extend may have moved the session forward between processIMC and startSlot)

	// Pure-hit snapshot-skip state mirrored from cacheResult.
	imcExpectedCachedMsgs  int    // Expected cachedMsgCount at startSlot.
	imcExpectedTokens      int    // Expected totalTokensCached at startSlot.
	imcExpectedRenderHash  string // Expected cachedRenderInputHash at startSlot (carried forward on builds/extends so commit can refresh the session field).
	imcPureHitSkipSnapshot bool   // True when startSlot may skip the post-restore snapshot.

	// IMC dedicated slot fields.
	imcNewCacheTokens    []llama.Token // New tokens to extend the cache in the slot's sequence
	imcNewTotalCached    int           // Total cached KV positions after extension
	imcNewCachedMsgCount int           // New cachedMsgCount after extension
	imcNewMsgsHash       string        // New cachedMsgsHash after extension
	imcClearSeq          bool          // True if sequence must be cleared before decoding (rebuild)
	imcNewCachedTokens   []llama.Token // Full token sequence to store in session after decode
	imcTrimPos           llama.Pos     // Position to trim KV cache from (for partial prefix rebuild)
	imcSysPromptHash     string        // Hash of system prompt message for the new cache state
	imcSysPromptTokens   int           // Token count of the system prompt in the new cache state

	// IMC media cache build — deferred media decode using mtmd pipeline.
	imcMediaBuild          bool  // True if cache build requires the mtmd pipeline (images/audio)
	imcMediaCacheD         D     // Document with cacheable messages + tools for media cache build
	imcMediaKVCounts       []int // Media KV position counts to preserve during text-only media extend
	imcMediaSkipTextTokens int   // Text tokens already in KV cache to skip during partial media extend
}

// slot represents a processing slot for parallel inference. Each slot can
// process one chat request at a time, with multiple slots enabling concurrent
// request handling within a single model context.
type slot struct {

	// -------------------------------------------------------------------------
	// Identity & Lifecycle

	id           int           // Slot index within the batch engine
	seqID        llama.SeqId   // KV cache sequence ID for this slot
	seqIDs       []llama.SeqId // Pre-allocated slice for batch.Add calls
	job          *chatJob      // Current request being processed
	active       bool          // True when slot is processing a request
	span         trace.Span    // OpenTelemetry span for request tracing
	stateMachine StateMachine  // Per-slot state machine; created from m.parser.NewStateMachine()

	// -------------------------------------------------------------------------
	// Sampling

	sampler        llama.Sampler   // Token sampler with temperature, top-p, etc.
	grammarSampler *grammarSampler // Grammar-constrained sampler (separate from chain)
	sampled        llama.Token     // Most recently sampled token
	iBatch         int32           // Index of this slot's token within the batch

	// -------------------------------------------------------------------------
	// Position & Token Counts

	nPast            llama.Pos // Current position in KV cache
	nPrompt          int       // Total prompt tokens (cached + new)
	reasonTokens     int       // Tokens in reasoning/thinking section
	completionTokens int       // Tokens in completion section

	// -------------------------------------------------------------------------
	// Text Prefill (text-only requests)

	prefillTokens []llama.Token // Tokens awaiting prefill
	nPrefilled    int           // Number of tokens already prefilled
	prefillDone   bool          // True when prefill complete, generation started

	// -------------------------------------------------------------------------
	// MTMD Prefill (vision/audio requests)

	mtmdCtx      mtmd.Context     // Per-request multimodal projector context (created in startSlot for media-bearing requests; freed in freeSlotResources). Zero for text-only requests and text-only models.
	inputChunks  mtmd.InputChunks // Tokenized chunks (text + media interleaved)
	chunkIdx     int              // Index of chunk currently being processed
	chunkTokIdx  int              // Token index within current text chunk (for partial prefill)
	bitmaps      []mtmd.Bitmap    // Image bitmaps to free when done
	useMRoPE     bool             // Model uses M-RoPE 4D positioning
	useNonCausal bool             // Model uses non-causal attention for media

	// -------------------------------------------------------------------------
	// Response Accumulation

	reasonFlag     int             // State: in reasoning section
	completionFlag int             // State: in completion section
	toolFlag       int             // State: in tool call section
	finalContent   strings.Builder // Accumulated completion text
	finalReasoning strings.Builder // Accumulated reasoning text
	finalTooling   strings.Builder // Accumulated tool call JSON
	respToolCalls  []ResponseToolCall
	utf8Buf        []byte // Buffered bytes from partial multi-byte UTF-8 codepoints

	// -------------------------------------------------------------------------
	// Logprobs

	logprobsData   []ContentLogprob // Accumulated logprobs for all tokens
	currentLogprob *ContentLogprob  // Current token's logprob (for streaming)

	// -------------------------------------------------------------------------
	// Speculative Decoding

	draftNPast         llama.Pos     // Draft model's KV cache position
	draftPrefillNeeded bool          // True when draft model needs prefill after target prefill
	draftPromptTokens  []llama.Token // Full prompt tokens for draft model prefill
	specDraftTokens    []llama.Token // Draft tokens for current speculative step
	specDraftProbs     [][]float32   // Draft probability distributions per drafted token
	specBasePast       llama.Pos     // Target nPast before speculative tokens were added
	specBaseBatch      int32         // Batch index where speculative tokens start
	specDraftedTotal   int           // Total draft tokens generated across all speculative steps
	specAcceptedTotal  int           // Total draft tokens accepted across all speculative steps
	specAccEMA         float64       // Exponential moving average of acceptance rate (persists across requests)
	specRounds         int           // Verify rounds completed this request (used to throttle per-round logging)

	// Per-slot owned buffers for speculative decoding. Avoids shared buffer
	// corruption when multiple slots generate draft tokens in the same
	// processBatch iteration.
	draftTokensBuf    []llama.Token // Owned copy of generated draft tokens
	draftCachedTokens []llama.Token // Prompt tokens in this slot's draft KV cache (persists across requests)

	// -------------------------------------------------------------------------
	// MTP (Multi-Token Prediction) per-slot state — populated only when
	// e.model.draft != nil && e.model.draft.mtp.

	// pendingH is a copy of the pre-norm hidden-state row from the
	// most-recently committed target position for this slot's sequence.
	// It is used as the embd input at slot 0 of the next "mirror"
	// batch into the MTP draft context (shift-right-by-1 alignment per
	// common_speculative.cpp). Lazy-grow, never-shrink — sized to the
	// model's embedding width on first use. Zero-length when no target
	// decode has produced a hidden row yet for this slot (e.g., very
	// first prefill chunk — slot 0 of that mirror batch is then zeroed).
	pendingH []float32

	// targetBatchStart / targetBatchCount / targetBatchBasePos record
	// the slot's contiguous range inside the shared target batch and the
	// sequence position of its first token, captured during batch
	// assembly so that the MTP mirror step (run AFTER llama_decode
	// succeeds) can find the just-decoded rows and replay them into the
	// draft KV with batch.embd populated.
	//
	// Set in three places in batch_engine.go:
	//   - addPrefillChunk    (prefill chunks)
	//   - normal gen-token add
	//   - spec verify add    (1 sampled + nDraft drafted)
	//
	// For spec batches, the mirror runs after verify resolves the
	// accepted count, so only count = 1 + accepted rows are mirrored.
	targetBatchStart   int32
	targetBatchCount   int32
	targetBatchBasePos llama.Pos

	// mtpHasBatch is true between batch.Add() and the post-decode mirror
	// step, signaling that the slot contributed rows to the most-recent
	// target decode and is awaiting an MTP mirror. Cleared by the
	// mirror step.
	mtpHasBatch bool

	// mtpDisabledForRequest is set true at startSlot when the request
	// hit IMC cache. MTP requires the draft KV to track the entire
	// sequence to make useful proposals, but IMC restores ONLY the
	// target KV — there is no draft snapshot. Running MTP against an
	// empty (or partial) draft context produces near-zero acceptance
	// for the whole request, which is worse than no speculation.
	// Also set inside finalizeSpeculativeTokens after a post-rollback
	// mirror failure. Cleared in slot.reset().
	mtpDisabledForRequest bool

	// mtpDisableReason is a short, machine-friendly label describing
	// why MTP was disabled for the current request. Surfaced in the
	// final per-request Usage block (DraftDisableReason) and the
	// chat-completion log line so an operator can immediately see why
	// a request with a high DMAR also had low draft coverage. Empty
	// while MTP is still active. Cleared in slot.reset(). Possible
	// values mirror the speculative-log status names:
	//   "imc-hit"      — IMC cache hit at startSlot.
	//   "mirror-error" — post-verify mirror failed; draft KV wiped.
	mtpDisableReason string

	// verifyH is a slot-local cache of the target context's pre-norm
	// hidden-state rows for the slot's just-decoded spec batch range.
	// Captured at the top of verifySpeculativeTokens (Phase A) BEFORE
	// any other slot's Phase B can re-decode on the target context
	// (notably restoreTargetSpecSnapshot on hybrid targets) and
	// invalidate the per-context pre-norm buffer.
	//
	// Layout: row-major, (1+nDraft) rows of nEmbd floats. Row k is the
	// pre-norm hidden state of the target batch position
	// s.targetBatchStart+k. finalizeSpeculativeTokens'
	// mirrorTargetBatchToMTPDraft reads from this buffer instead of the
	// live target pre-norm buffer so the mirror is safe to run AFTER a
	// hybrid restoreTargetSpecSnapshot re-decode.
	//
	// Lazy-grow / never-shrink. Length is set to actual bytes per
	// capture; cap is retained across requests. Cleared by the mirror
	// after consumption and by slot.reset().
	verifyH []float32

	// specSnapshot holds a snapshot of the target context's per-sequence
	// state taken right before a speculative batch is decoded. It is
	// required for HYBRID target models (transformer + recurrent layers):
	// MemorySeqRm can trim the transformer KV but cannot rewind the
	// per-sequence recurrent state, so a partial-rejection round would
	// leave the recurrent layer advanced past the accepted boundary
	// and the next llama_decode fails with -1. The snapshot lets
	// verifySpeculativeTokens restore the pre-spec state and re-decode
	// only the accepted prefix.
	//
	// Allocated lazy-grow / never-shrink. The buffer is sized via
	// llama.StateSeqGetSize before each snapshot — the required size
	// scales with current KV occupancy. Length is reset to the actual
	// snapshot bytes; cap is retained across requests.
	specSnapshot []byte

	// Pending-finalize fields populated by Phase A of speculative verify
	// (verifySpeculativeTokens) and consumed by Phase B
	// (finalizeSpeculativeTokens). The split exists because Phase B may
	// re-decode on the target context (hybrid restoreTargetSpecSnapshot),
	// which wipes the per-context logit buffer for every other slot's
	// rows. Running all spec slots through Phase A first lets every slot
	// read its logits before any restore mutates them; then a second
	// pass runs the per-slot Phase B in any order.
	//
	// specPendingFinalize gates Phase B. It is true between a successful
	// Phase A and the matching Phase B. EOG inside Phase A returns
	// without setting it, so Phase B is skipped for finished slots.
	// Cleared at the top of Phase B and by slot.reset().
	specPendingFinalize        bool
	specPendingAccepted        int
	specPendingBonusToken      llama.Token
	specPendingOriginalSampled llama.Token

	// Sparse candidate-based speculative decoding fields.
	draftSampler         llama.Sampler            // Per-slot sampler for draft model (non-greedy)
	specDraftDistsSparse [][]candidateEntry       // Sparse draft distributions per drafted token
	draftDistBuf         [][]candidateEntry       // Pre-allocated backing for specDraftDistsSparse
	draftCandDistBuf     [][]llama.DraftCandidate // Pre-allocated backing for DraftGenerate output
	adjustedDistBuf      []candidateEntry         // Scratch buffer for adjusted sampling

	// -------------------------------------------------------------------------
	// Metrics

	startTime    time.Time     // Start time for TPS calculation (set after prefill)
	prefillStart time.Time     // Start time for TTFT calculation
	prefillSpan  trace.Span    // Span covering the prefill phase
	tokenGenSpan trace.Span    // Span covering the token generation phase
	ttft         time.Duration // Time to first token (prefill duration)
}

func (s *slot) reset() {
	// Note: seqID is NOT reset - it's assigned once during slot creation
	// and remains stable for the lifetime of the slot.

	s.job = nil
	s.nPast = 0
	s.nPrompt = 0
	s.reasonTokens = 0
	s.completionTokens = 0
	s.reasonFlag = 0
	s.completionFlag = 0
	s.toolFlag = 0
	s.finalContent.Reset()
	s.finalReasoning.Reset()
	s.finalTooling.Reset()
	s.respToolCalls = nil
	s.utf8Buf = s.utf8Buf[:0]
	s.span = nil
	s.iBatch = -1
	s.sampled = 0
	s.active = false
	s.prefillDone = false
	s.prefillTokens = nil
	s.nPrefilled = 0
	s.logprobsData = nil
	s.currentLogprob = nil
	s.draftNPast = 0
	s.draftPrefillNeeded = false
	s.draftPromptTokens = nil
	s.specDraftTokens = nil
	s.specDraftProbs = nil
	s.specBasePast = 0
	s.specBaseBatch = 0
	s.specDraftedTotal = 0
	s.specAcceptedTotal = 0
	s.specRounds = 0
	s.specPendingFinalize = false
	s.specPendingAccepted = 0
	s.specPendingBonusToken = 0
	s.specPendingOriginalSampled = 0
	s.draftTokensBuf = s.draftTokensBuf[:0]
	// Note: draftCachedTokens persists across requests for incremental draft KV reuse.

	// MTP per-request state. pendingH capacity is retained (lazy-grow,
	// never-shrink) but its length is reset because a new request begins
	// a fresh sequence — the hidden state from the previous request's
	// last position is no longer the natural predecessor of the new
	// request's first token.
	s.pendingH = s.pendingH[:0]
	s.verifyH = s.verifyH[:0]
	s.targetBatchStart = 0
	s.targetBatchCount = 0
	s.targetBatchBasePos = 0
	s.mtpHasBatch = false
	s.mtpDisabledForRequest = false
	s.mtpDisableReason = ""
	if s.draftSampler != 0 {
		llama.SamplerFree(s.draftSampler)
		s.draftSampler = 0
	}
	s.specDraftDistsSparse = nil
	// Note: draftDistBuf, targetDistBuf, adjustedDistBuf are reused across requests
	s.grammarSampler = nil
	s.prefillStart = time.Time{}
	s.prefillSpan = nil
	s.tokenGenSpan = nil
	s.ttft = 0

	// MTMD fields.
	s.inputChunks = 0
	s.chunkIdx = 0
	s.chunkTokIdx = 0
	s.bitmaps = nil
	s.useMRoPE = false
	s.useNonCausal = false

	if s.stateMachine != nil {
		s.stateMachine.Reset()
	}
}
