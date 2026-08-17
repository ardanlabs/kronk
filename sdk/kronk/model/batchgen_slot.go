package model

import (
	"context"
	"math/rand"
	"strings"
	"time"

	classicengine "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation/classic"
	mtpengine "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation/mtp"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"go.opentelemetry.io/otel/trace"
)

// chatJob represents a validated chat request ready for generation batch processing.
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

	d                   D             // Original request document (messages, parameters)
	object              string        // Request type: ObjectChatText or ObjectChatMedia
	prompt              string        // Templated prompt string ready for tokenization
	media               [][]byte      // Raw media bytes (images/audio) for vision/audio models
	params              Params        // Sampling and generation parameters
	textTokens          []llama.Token // Complete text prompt tokenized during synchronous request preparation.
	samplerPromptTokens []llama.Token // Complete logical text-token prompt used to prime the request sampler.
	tailTokens          []llama.Token // Non-empty inference tail after the stable cached target.
	imcTokenPlan        bool          // True when samplerPromptTokens/tailTokens are authoritative.
	imcMatchKind        string        // exact, append, or rebuild; used for diagnostics.
	imcPromptPlan       promptPlan    // Logical stable prefix committed with the session.

	// -------------------------------------------------------------------------
	// Incremental Message Cache (IMC)

	imcSession         *imcSession // Matched IMC session (the session-pool entry whose KV state will be restored into the assigned slot)
	imcSessionMedia    bool        // True if session has media (snapshot at job creation; safe to read without lock)
	imcSessionUseMRoPE bool        // M-RoPE mode captured while the selected session is reserved.
	imcPhysicalCached  int         // Physical cached context owned by this job after build or extension.
	imcUsageVersion    uint64      // Session request generation assigned when this job starts execution.
	imcSessionID       int         // Session-pool index (== imcSession.id); used by imcReleaseReservation lookup and log correlation. Not related to execution slot identity.
	imcCacheHit        bool        // True when this request uses the IMC build/restore path.
	reusedPromptTokens int         // Logical prompt-prefix tokens reused by this request.
	imcSnapshotReused  bool        // True after a prior externalized target snapshot is restored successfully.
	imcExpectedHash    string      // Expected cachedMsgsHash for stale detection at startSlot (a concurrent extend may have moved the session forward)

	// Pure-hit snapshot-skip state mirrored from cacheResult.
	imcExpectedCachedMsgs  int    // Expected cachedMsgCount at startSlot.
	imcExpectedTokens      int    // Expected physical KV cells at startSlot.
	imcExpectedPosition    int    // Expected next logical position at startSlot.
	imcExpectedRenderHash  string // Expected cachedRenderInputHash at startSlot (carried forward on builds/extends so commit can refresh the session field).
	imcExpectedPromptPlan  promptPlan
	imcReadOnlyReservation bool // True when the session is reserved for restore/use without metadata or snapshot mutation.
	imcMediaAnchorAdvance  bool // True when text after a media anchor should be atomically committed as a larger snapshot.
	imcNewLogicalPosition  int  // Next logical position after a media-anchor advance.
	imcReservationHeld     bool // True until this request publishes or releases its reservation.
	imcPureHitSkipSnapshot bool // True when startSlot may skip the post-restore snapshot.
	imcPromoteCheckpoint   bool // True when current state must be retained as a user-turn checkpoint before commit.
	imcCheckpointTokens    int  // Exact target token boundary for a progressive reusable snapshot.

	// IMC dedicated slot fields.
	imcNewCacheTokens    []llama.Token // New tokens to extend the cache in the slot's sequence
	imcNewTotalCached    int           // Total cached KV positions after extension
	imcNewCachedMsgCount int           // New cachedMsgCount after extension
	imcNewMsgsHash       string        // New cachedMsgsHash after extension
	imcNewEndsAtUser     bool          // True when the new current snapshot ends at a real user message.
	imcClearSeq          bool          // True if sequence must be cleared before decoding (rebuild)
	imcNewCachedTokens   []llama.Token // Full token sequence to store in session after decode

	// IMC media cache build — deferred media decode using mtmd pipeline.
	imcMediaBuild         bool          // True if cache build requires the mtmd pipeline (images/audio)
	imcMediaCacheD        D             // Document with cacheable messages + tools for media cache build
	imcMediaKVCounts      []int         // Media KV position counts to preserve during text-only media extend
	imcMediaSamplerTokens []llama.Token // Authoritative mtmd text tokens in the stable media cache prefix.
}

func (j *chatJob) hasIMCReservation() bool {
	return j != nil && j.imcSession != nil && j.imcReservationHeld
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

	nPast              llama.Pos // Current position in KV cache
	nPrompt            int       // Total prompt tokens (cached + new)
	reusedPromptTokens int       // Logical prompt-prefix tokens reused by this request
	reasonTokens       int       // Tokens in reasoning/thinking section
	completionTokens   int       // Tokens in completion section

	// -------------------------------------------------------------------------
	// Text Prefill (text-only requests)

	prefillTokens []llama.Token   // Tokens awaiting prefill
	nPrefilled    int             // Number of tokens already prefilled
	prefillDone   bool            // True when prefill complete, generation started
	imcPrep       *imcPreparation // Resumable text IMC build or extension state
	imcRestoring  bool            // True while session sequence state is restored into this slot

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

	reasonFlag       int             // State: in reasoning section
	completionFlag   int             // State: in completion section
	toolFlag         int             // State: in tool call section
	suppressTools    bool            // Request explicitly set tool_choice to none
	finalContent     strings.Builder // Accumulated completion text
	finalReasoning   strings.Builder // Accumulated reasoning text
	finalTooling     strings.Builder // Accumulated tool call JSON
	rawOutput        strings.Builder // Decoded model output retained for insecure logging
	respToolCalls    []ResponseToolCall
	finishReason     string
	stopSource       string
	utf8Buf          []byte // Buffered bytes from partial multi-byte UTF-8 codepoints
	stopGate         *stopGate
	stopUTF8Logprobs []*ContentLogprob

	// -------------------------------------------------------------------------
	// Logprobs

	logprobsData   []ContentLogprob // Accumulated logprobs for all tokens
	currentLogprob *ContentLogprob  // Current token's logprob (for streaming)

	// -------------------------------------------------------------------------
	// Speculative Decoding

	draftNPast          llama.Pos     // Draft model's KV cache position
	draftPrefillNeeded  bool          // True when draft model needs prefill after target prefill
	draftPromptTokens   []llama.Token // Full prompt tokens for draft model prefill
	specDraftTokens     []llama.Token // Draft tokens for current speculative step
	specBasePast        llama.Pos     // Target nPast before speculative tokens were added
	specBaseBatch       int32         // Batch index where speculative tokens start
	specDraftedTotal    int           // Total draft tokens generated across all speculative steps
	specAcceptedTotal   int           // Total draft tokens accepted across all speculative steps
	specCoveredTotal    int           // Emitted output tokens processed through speculative verification
	processingSpecToken bool          // True while an accepted draft or bonus token is being processed
	samplingSeeds       samplingSeeds // Derived native and Go RNG seeds for this request
	specRNG             *rand.Rand    // Request-local RNG for speculative acceptance and manual sampling

	// Per-slot owned buffers for speculative decoding. Avoids shared buffer
	// corruption when multiple slots generate draft tokens in the same
	// processBatch iteration.
	draftTokensBuf    []llama.Token // Owned copy of generated draft tokens
	draftCachedTokens []llama.Token // Prompt tokens in this slot's draft KV cache (persists across requests)
	classic           classicengine.SlotState

	// MTP owns its request-local hidden-state, synchronization, and disable
	// state. The engine stores it without duplicating those fields in slot.
	mtp mtpengine.SlotState

	// specSnapshot holds a snapshot of the target context's per-sequence
	// state taken right before a speculative batch is decoded. It is
	// required for HYBRID target models (transformer + recurrent layers):
	// MemorySeqRm can trim the transformer KV but cannot rewind the
	// per-sequence recurrent state, so a partial-rejection round would
	// leave the recurrent layer advanced past the accepted boundary
	// and the next llama_decode fails with -1. The snapshot lets
	// classic finalization restores the pre-spec state and re-decodes
	// only the accepted prefix.
	//
	// Allocated lazy-grow / never-shrink. The buffer is sized via
	// llama.StateSeqGetSize before each snapshot — the required size
	// scales with current KV occupancy. Length is reset to the actual
	// snapshot bytes; cap is retained across requests.
	specSnapshot []byte

	// Pending-finalize fields populated by speculative verification and
	// consumed by finalization. The split exists because finalization may
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
	specPendingSamplerAccepted bool
	specPendingLogprobReady    bool
	specPendingLogprob         *ContentLogprob

	// Sparse candidate-based speculative decoding fields.
	draftSampler     llama.Sampler            // Per-slot sampler for draft model (non-greedy)
	draftCandDistBuf [][]llama.DraftCandidate // Pre-allocated backing for DraftGenerate output

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
	s.reusedPromptTokens = 0
	s.reasonTokens = 0
	s.completionTokens = 0
	s.reasonFlag = 0
	s.completionFlag = 0
	s.toolFlag = 0
	s.suppressTools = false
	s.finalContent.Reset()
	s.finalReasoning.Reset()
	s.finalTooling.Reset()
	s.rawOutput.Reset()
	s.respToolCalls = nil
	s.finishReason = ""
	s.stopSource = ""
	s.utf8Buf = s.utf8Buf[:0]
	s.stopGate = nil
	s.stopUTF8Logprobs = s.stopUTF8Logprobs[:0]
	s.span = nil
	s.iBatch = -1
	s.sampled = 0
	s.active = false
	s.prefillDone = false
	s.prefillTokens = nil
	s.nPrefilled = 0
	s.imcPrep = nil
	s.imcRestoring = false
	s.logprobsData = nil
	s.currentLogprob = nil
	s.draftNPast = 0
	s.draftPrefillNeeded = false
	s.draftPromptTokens = nil
	s.specDraftTokens = nil
	s.specBasePast = 0
	s.specBaseBatch = 0
	s.specDraftedTotal = 0
	s.specAcceptedTotal = 0
	s.specCoveredTotal = 0
	s.processingSpecToken = false
	s.classic.Reset()
	s.samplingSeeds = samplingSeeds{}
	s.specRNG = nil
	s.specPendingFinalize = false
	s.specPendingAccepted = 0
	s.specPendingBonusToken = 0
	s.specPendingOriginalSampled = 0
	s.specPendingSamplerAccepted = false
	s.specPendingLogprobReady = false
	s.specPendingLogprob = nil
	s.specSnapshot = s.specSnapshot[:0]
	s.draftTokensBuf = s.draftTokensBuf[:0]
	// Note: draftCachedTokens persists across requests for incremental draft KV reuse.

	s.mtp.Reset()
	if s.draftSampler != 0 {
		llama.SamplerFree(s.draftSampler)
		s.draftSampler = 0
	}
	// Note: draftDistBuf, targetDistBuf, adjustedDistBuf are reused across requests
	s.grammarSampler = nil
	s.startTime = time.Time{}
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
