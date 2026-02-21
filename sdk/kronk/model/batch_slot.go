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

	// -------------------------------------------------------------------------
	// Request Content

	d      D        // Original request document (messages, parameters)
	object string   // Request type: ObjectChatText or ObjectChatMedia
	prompt string   // Templated prompt string ready for tokenization
	media  [][]byte // Raw media bytes (images/audio) for vision/audio models
	params Params   // Sampling and generation parameters

	// -------------------------------------------------------------------------
	// MTMD Context

	mtmdCtx mtmd.Context // Multi-modal context for vision/audio processing

	// -------------------------------------------------------------------------
	// System Prompt Cache (SPC)

	spcCacheSeqID llama.SeqId // Dedicated SPC cache sequence ID
	spcCacheIdx   llama.Pos   // Token count in SPC cache
	spcCacheHit   bool        // True if SPC cache sequence has cached tokens

	// -------------------------------------------------------------------------
	// Incremental Message Cache (IMC)

	imcSlotID       int         // Target slot index for IMC routing
	imcSeqID        llama.SeqId // Sequence ID containing cached conversation state
	imcCacheIdx     llama.Pos   // Token position where IMC cache ends
	imcCacheHit     bool        // True if conversation history was found in cache
	imcExpectedHash string      // Expected cachedMsgsHash for stale detection at startSlot

	// IMC dedicated slot fields.
	imcNewCacheTokens  []llama.Token // New tokens to extend the cache in the slot's sequence
	imcNewTotalCached  int           // Total cached tokens after extension
	imcNewMsgIdx       int           // New lastMsgIdxCached after extension
	imcNewMsgsHash     string        // New cachedMsgsHash after extension
	imcClearSeq        bool          // True if sequence must be cleared before decoding (rebuild)
	imcNewCachedTokens []llama.Token // Full token sequence to store in session after decode
	imcTrimPos         llama.Pos     // Position to trim KV cache from (for partial prefix rebuild)
}

// slot represents a processing slot for parallel inference. Each slot can
// process one chat request at a time, with multiple slots enabling concurrent
// request handling within a single model context.
type slot struct {

	// -------------------------------------------------------------------------
	// Identity & Lifecycle

	id     int           // Slot index within the batch engine
	seqID  llama.SeqId   // KV cache sequence ID for this slot
	seqIDs []llama.SeqId // Pre-allocated slice for batch.Add calls
	job    *chatJob      // Current request being processed
	active bool          // True when slot is processing a request
	span   trace.Span    // OpenTelemetry span for request tracing
	proc   *processor    // Response processor for content streaming

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

	// -------------------------------------------------------------------------
	// IMC Hybrid State

	imcSavedState []byte // Snapshot of KV+recurrent state for IMC Hybrid restore

	// -------------------------------------------------------------------------
	// Metrics

	startTime    time.Time  // Start time for TPS calculation (set after prefill)
	prefillStart time.Time  // Start time for TTFT calculation
	prefillSpan  trace.Span // Span covering the prefill phase
	tokenGenSpan trace.Span // Span covering the token generation phase
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
	s.imcSavedState = s.imcSavedState[:0]
	s.grammarSampler = nil
	s.prefillStart = time.Time{}
	s.prefillSpan = nil
	s.tokenGenSpan = nil

	// MTMD fields.
	s.inputChunks = 0
	s.chunkIdx = 0
	s.chunkTokIdx = 0
	s.bitmaps = nil
	s.useMRoPE = false
	s.useNonCausal = false

	if s.proc != nil {
		s.proc.resetState()
	}
}
