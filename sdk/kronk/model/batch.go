package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// chatJob represents a validated chat request ready for batch processing.
type chatJob struct {
	id              string
	ctx             context.Context
	d               D
	object          string
	prompt          string
	media           [][]byte
	params          Params
	mtmdCtx         mtmd.Context
	ch              chan<- ChatResponse
	sysPromptNPast  llama.Pos
	sysPromptCached bool
	imcID           string      // IMC session ID (from imc_id in request)
	imcSeqID        llama.SeqId // IMC session's cache sequence
	imcNPast        llama.Pos   // IMC cached token count
	imcCached       bool        // True if IMC cache should be used
}

// slot represents a processing slot for parallel inference.
type slot struct {
	id     int
	seqID  llama.SeqId
	seqIDs []llama.SeqId // Pre-allocated for batchAdd calls

	job     *chatJob
	proc    *processor
	sampler llama.Sampler

	nPast    llama.Pos
	nPrompt  int
	nDecoded int

	reasonTokens     int
	completionTokens int

	reasonFlag     int
	completionFlag int
	toolFlag       int

	index          int
	finalContent   strings.Builder
	finalReasoning strings.Builder
	finalTooling   strings.Builder
	respToolCalls  []ResponseToolCall

	startTime   time.Time
	span        trace.Span
	iBatch      int32
	sampled     llama.Token
	active      bool
	prefillDone bool

	prefillTokens []llama.Token
	nPrefilled    int

	// Logprobs tracking
	logprobsData   []ContentLogprob
	currentLogprob *ContentLogprob // For streaming the current token's logprob
}

func (s *slot) reset() {
	// Note: seqID is NOT reset - it's assigned once during slot creation
	// and remains stable for the lifetime of the slot.
	s.job = nil
	s.nPast = 0
	s.nPrompt = 0
	s.nDecoded = 0
	s.reasonTokens = 0
	s.completionTokens = 0
	s.reasonFlag = 0
	s.completionFlag = 0
	s.toolFlag = 0
	s.index = 0
	s.finalContent.Reset()
	s.finalReasoning.Reset()
	s.finalTooling.Reset()
	s.respToolCalls = nil
	s.span = nil
	s.iBatch = -1
	s.sampled = 0
	s.active = false
	s.prefillDone = false
	s.prefillTokens = nil
	s.nPrefilled = 0
	s.logprobsData = nil
	s.currentLogprob = nil

	if s.proc != nil {
		s.proc.resetState()
	}
}

// batchEngine manages parallel inference slots.
type batchEngine struct {
	model      *Model
	nSlots     int
	slots      []*slot
	batch      llama.Batch
	requestQ   chan *chatJob
	wakeCh     chan struct{}
	shutdownCh chan struct{}
	wg         sync.WaitGroup
	stopped    atomic.Bool
}

// newBatchEngine creates a new batch engine for parallel inference.
func newBatchEngine(m *Model, nSlots int) *batchEngine {
	// Create batch buffer.
	nCtx := llama.NCtx(m.lctx)
	batch := llama.BatchInit(int32(nCtx), 0, int32(nSlots))

	// Calculate sequence offset based on reserved cache sequences.
	// SPC uses seq 0 only. IMC uses seqs 0 to MaxIMCSessions-1.
	// Slots start after reserved sequences.
	cacheSeqs := 0
	if m.cfg.SystemPromptCache {
		cacheSeqs = 1
	} else if m.cfg.IncrementalCache {
		cacheSeqs = m.imcMaxSeqs
	}

	// Initialize slots.
	slots := make([]*slot, nSlots)
	for i := range slots {
		seqID := llama.SeqId(i + cacheSeqs)
		slots[i] = &slot{
			id:     i,
			seqID:  seqID,
			seqIDs: []llama.SeqId{seqID}, // Pre-allocate for batchAdd
			proc:   newProcessor(m),
		}
	}

	return &batchEngine{
		model:      m,
		nSlots:     nSlots,
		slots:      slots,
		batch:      batch,
		requestQ:   make(chan *chatJob, nSlots*2),
		wakeCh:     make(chan struct{}, 1),
		shutdownCh: make(chan struct{}),
	}
}

// start begins the batch processing loop.
func (e *batchEngine) start(ctx context.Context) {
	e.wg.Add(1)
	go e.processLoop(ctx)
	e.model.log(ctx, "batch-engine", "status", "started", "slots", e.nSlots)
}

// stop signals shutdown and waits for completion.
func (e *batchEngine) stop(ctx context.Context) {
	if !e.stopped.CompareAndSwap(false, true) {
		e.wg.Wait() // Still wait for processLoop to exit
		return
	}

	close(e.shutdownCh)
	e.wg.Wait()

	// Free samplers - batch is freed separately in Unload.
	for _, s := range e.slots {
		if s.sampler != 0 {
			llama.SamplerFree(s.sampler)
			s.sampler = 0
		}
	}

	e.model.log(ctx, "batch-engine", "status", "stopped")
}

// freeBatch frees the batch buffer. Called from Model.Unload.
func (e *batchEngine) freeBatch() {
	llama.BatchFree(e.batch)
}

// submit adds a job to the processing queue.
func (e *batchEngine) submit(job *chatJob) error {
	select {
	case e.requestQ <- job:
		select {
		case e.wakeCh <- struct{}{}:
		default:
		}
		return nil

	case <-e.shutdownCh:
		return fmt.Errorf("submit: engine shutting down")

	case <-job.ctx.Done():
		return job.ctx.Err()
	}
}

// processLoop is the main batch processing goroutine using a signal-based wake
// algorithm. Instead of polling at a fixed interval, it wakes immediately when
// new requests arrive on requestQ, eliminating up to 1ms latency on request
// pickup. When slots are actively generating, it polls at 100µs for low-latency
// token streaming. When idle, it backs off to 5ms to reduce CPU usage.
func (e *batchEngine) processLoop(ctx context.Context) {
	defer e.wg.Done()

	buf := make([]byte, 32*1024)

	const (
		activeInterval = 100 * time.Microsecond // Fast poll when slots are generating
		idleInterval   = 5 * time.Millisecond   // Slow poll when no active slots
	)

	timer := time.NewTimer(idleInterval)
	defer timer.Stop()

	for {
		select {
		case <-e.shutdownCh:
			e.drainSlots()
			return

		case <-e.wakeCh:
			timer.Reset(0)

		case <-timer.C:
			switch e.hasActiveSlots() || len(e.requestQ) > 0 {
			case true:
				e.processBatch(ctx, buf)
				timer.Reset(activeInterval)

			case false:
				timer.Reset(idleInterval)
			}
		}
	}
}

// hasActiveSlots returns true if any slot is currently processing.
func (e *batchEngine) hasActiveSlots() bool {
	for _, s := range e.slots {
		if s.active {
			return true
		}
	}
	return false
}

// processBatch handles one iteration of the batch processing loop.
func (e *batchEngine) processBatch(ctx context.Context, buf []byte) {
	// Clear the batch.
	batchClear(&e.batch)

	// Continue prefill for slots that are still prefilling.
	for _, s := range e.slots {
		if !s.active || s.prefillTokens == nil {
			continue
		}

		// Check if client cancelled.
		if s.job.ctx.Err() != nil {
			e.finishSlot(s, s.job.ctx.Err())
			continue
		}

		// addPrefillChunk returns false if shutdown or context cancelled.
		if !e.addPrefillChunk(s) {
			e.finishSlot(s, s.job.ctx.Err())
			continue
		}
	}

	// Add tokens from active slots that have completed prefill.
	for _, s := range e.slots {
		if !s.active || !s.prefillDone {
			continue
		}

		// Check if client cancelled.
		if s.job.ctx.Err() != nil {
			e.finishSlot(s, s.job.ctx.Err())
			continue
		}

		s.iBatch = e.batch.NTokens
		batchAdd(&e.batch, s.sampled, s.nPast, s.seqIDs, true)
		s.nPast++
		s.nDecoded++
	}

	// Fill empty slots from queue.
	e.fillSlots()

	// Nothing to process.
	if e.batch.NTokens == 0 {
		return
	}

	// Defensive check: batch tokens must not exceed NBatch.
	nBatch := e.model.cfg.NBatch
	if int(e.batch.NTokens) > nBatch {
		e.model.log(ctx, "process-batch", "ERROR", "batch-overflow",
			"batch_tokens", e.batch.NTokens,
			"nbatch_limit", nBatch,
			"slots", e.nSlots)

		// Log per-slot state for debugging.
		for _, s := range e.slots {
			if s.active {
				e.model.log(ctx, "process-batch", "slot-state",
					"slot", s.id,
					"prefill_remaining", len(s.prefillTokens)-s.nPrefilled,
					"prefill_done", s.prefillDone,
					"n_past", s.nPast,
					"i_batch", s.iBatch)
			}
		}

		// Fail all active slots with descriptive error.
		overflowErr := fmt.Errorf("process-batch: %d tokens exceeds NBatch limit of %d", e.batch.NTokens, nBatch)
		for _, s := range e.slots {
			if s.active {
				e.finishSlot(s, overflowErr)
			}
		}

		return
	}

	// Lock to prevent concurrent decode with cache population.
	e.model.decodeMu.Lock()
	ret, err := llama.Decode(e.model.lctx, e.batch)
	e.model.decodeMu.Unlock()

	if err != nil || ret != 0 {
		e.logDecodeError(ctx, ret, err)

		// Fail all active slots to prevent infinite retry loop.
		decodeErr := decodeError(ret, err)
		for _, s := range e.slots {
			if s.active {
				e.finishSlot(s, decodeErr)
			}
		}
		return
	}

	// Sample tokens for each active slot.
	for _, s := range e.slots {
		if s.iBatch < 0 || !s.active {
			continue
		}

		e.processSlotToken(s, buf)
	}
}

// fillSlots assigns pending requests to available slots.
func (e *batchEngine) fillSlots() {
	for _, s := range e.slots {
		if s.active {
			continue
		}

		// Try to get a request from the queue.
		select {
		case job := <-e.requestQ:
			e.startSlot(s, job)
			return // Only prefill one slot per iteration to avoid exceeding NBatch

		default:
			return
		}
	}
}

// startSlot initializes a slot with a new request.
func (e *batchEngine) startSlot(s *slot, job *chatJob) {
	s.reset()
	s.active = true
	s.job = job
	// Note: startTime is set when prefillDone=true (first output token) for accurate TPS
	// seqID is already set correctly during slot creation in newBatchEngine

	// Start span for this chat request.
	_, s.span = otel.AddSpan(job.ctx, "batch-chat-request",
		attribute.String("id", job.id),
		attribute.Int("slot", s.id),
	)

	// Create sampler for this request.
	s.sampler = e.model.toSampler(job.params)

	// Always clear the slot's sequence before starting to remove any stale KV data.
	llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)

	// Copy cached KV state if available (SPC or IMC, mutually exclusive).
	var cachedTokens llama.Pos
	switch {
	case job.sysPromptCached:
		// SPC: copy from seq 0.
		if err := e.model.copySystemPromptToSeq(s.seqID); err != nil {
			e.sendSlotError(s, fmt.Errorf("start-slot: %w", err))
			s.reset()
			return
		}
		cachedTokens = job.sysPromptNPast

	case job.imcCached:
		// IMC: copy from session's sequence.
		e.model.log(job.ctx, "start-slot", "status", "imc-copy", "src_seq", job.imcSeqID, "dst_seq", s.seqID, "tokens", job.imcNPast)
		if err := e.model.copyCachesToSeq(s.seqID, job.imcSeqID); err != nil {
			e.sendSlotError(s, fmt.Errorf("start-slot: %w", err))
			s.reset()
			return
		}
		cachedTokens = job.imcNPast
	}
	s.nPast = cachedTokens

	// Tokenize the prompt (cached messages already removed).
	addBOS := cachedTokens == 0
	tokens := llama.Tokenize(e.model.vocab, job.prompt, addBOS, true)
	s.nPrompt = len(tokens)

	// Include cached tokens in total prompt count for metrics.
	if cachedTokens > 0 {
		s.nPrompt += int(cachedTokens)
	}

	// Log token counts for debugging batch overflow.
	e.model.log(job.ctx, "start-slot", "status", "tokenized",
		"slot", s.id,
		"suffix_tokens", len(tokens),
		"cached_tokens", cachedTokens,
		"total_prompt", s.nPrompt,
		"nbatch", e.model.cfg.NBatch,
		"batch_current", e.batch.NTokens)

	// Check context window.
	if s.nPrompt > e.model.cfg.ContextWindow {
		err := fmt.Errorf("start-slot: input tokens [%d] exceed context window [%d]", s.nPrompt, e.model.cfg.ContextWindow)
		e.finishSlot(s, err)
		return
	}

	// Store tokens for chunked prefill.
	s.prefillTokens = tokens
	s.nPrefilled = 0

	// Add first chunk of prompt tokens to batch.
	if !e.addPrefillChunk(s) {
		e.finishSlot(s, job.ctx.Err())
		return
	}

	// Calculate current KV usage for diagnostics.
	var kvUsed llama.Pos
	if sysMax, err := llama.MemorySeqPosMax(e.model.mem, 0); err == nil && sysMax >= 0 {
		kvUsed += sysMax + 1
	}

	for _, slot := range e.slots {
		if slot.active && slot.id != s.id {
			if posMax, err := llama.MemorySeqPosMax(e.model.mem, slot.seqID); err == nil && posMax >= 0 {
				kvUsed += posMax + 1
			}
		}
	}

	e.model.log(job.ctx, "batch-engine", "status", "slot-started", "slot", s.id, "seq", s.seqID, "id", job.id,
		"prompt_tokens", s.nPrompt, "sys_cached", job.sysPromptCached, "imc_cached", job.imcCached, "kv_used_other", kvUsed)
}

// addPrefillChunk adds the next chunk of prefill tokens to the batch.
// Returns true if prefill completed, false if cancelled or still prefilling.
func (e *batchEngine) addPrefillChunk(s *slot) bool {
	if s.prefillTokens == nil || s.nPrefilled >= len(s.prefillTokens) {
		return true
	}

	// Check for cancellation before processing chunk.
	select {
	case <-e.shutdownCh:
		return false
	case <-s.job.ctx.Done():
		return false
	default:
	}

	prefillStart := time.Now()

	nBatch := e.model.cfg.NBatch
	remaining := len(s.prefillTokens) - s.nPrefilled

	// Limit chunk size to available space in batch (total across all slots must not exceed NBatch).
	availableInBatch := nBatch - int(e.batch.NTokens)
	if availableInBatch <= 0 {
		s.iBatch = -1
		return true
	}

	chunkSize := min(remaining, nBatch, availableInBatch)

	// Add chunk of tokens to batch.
	for i := range chunkSize {
		tok := s.prefillTokens[s.nPrefilled+i]
		isLast := s.nPrefilled+i == len(s.prefillTokens)-1
		batchAdd(&e.batch, tok, s.nPast, s.seqIDs, isLast)
		s.nPast++
	}
	s.nPrefilled += chunkSize

	prefillDuration := time.Since(prefillStart)
	metrics.AddPrefillNonMediaTime(e.model.modelInfo.ID, prefillDuration)

	// Check if prefill is complete.
	if s.nPrefilled >= len(s.prefillTokens) {
		s.iBatch = e.batch.NTokens - 1
		s.prefillTokens = nil
		s.span.SetAttributes(attribute.String("prefill-nonmedia", prefillDuration.String()))
		return true
	}

	s.iBatch = -1
	return true
}

// processSlotToken handles a sampled token for a slot.
func (e *batchEngine) processSlotToken(s *slot, buf []byte) {
	// Sample the next token.
	token := llama.SamplerSample(s.sampler, e.model.lctx, s.iBatch)

	// Extract logprobs BEFORE accepting - Accept modifies sampler state.
	// Reset currentLogprob each token; it's used for streaming.
	s.currentLogprob = nil
	if s.job.params.Logprobs {
		logprob, err := extractLogprobs(e.model.lctx, e.model.vocab, token, s.iBatch, s.job.params.TopLogprobs, buf)
		switch {
		case err != nil:
			e.model.log(s.job.ctx, "batch-engine", "status", "logprobs-error", "slot", s.id, "error", err.Error())
		case logprob != nil:
			s.currentLogprob = logprob
			s.logprobsData = append(s.logprobsData, *logprob)
		}
	}

	llama.SamplerAccept(s.sampler, token)

	// Check for end of generation.
	if llama.VocabIsEOG(e.model.vocab, token) {
		e.finishSlot(s, nil)
		return
	}

	// Convert token to text.
	l := llama.TokenToPiece(e.model.vocab, token, buf, 0, true)
	content := string(buf[:l])

	if content == "" {
		e.finishSlot(s, nil)
		return
	}

	s.sampled = token
	if !s.prefillDone {
		s.prefillDone = true
		s.startTime = time.Now() // Start TPS clock after prefill, when first output token is generated
	}
	s.index++

	// Process through the state machine.
	isGPT := e.model.modelInfo.IsGPTModel
	var resp response
	var eog bool

	switch isGPT {
	case true:
		resp, eog = s.proc.stepGPT(content)

	default:
		resp, eog = s.proc.stepStandard(content)
	}

	if eog {
		e.finishSlot(s, nil)
		return
	}

	// [DEBUG]: Show model response.
	// fmt.Println(resp.content)

	// Update flags based on response status.
	switch resp.status {
	case statusReasoning:
		s.reasonFlag++
		s.completionFlag = 0
		s.toolFlag = 0

	case statusCompletion:
		s.completionFlag++
		s.reasonFlag = 0
		s.toolFlag = 0

	case statusTooling:
		s.toolFlag++
		s.reasonFlag = 0
		s.completionFlag = 0

	default:
		// No streamable content (statusNone) - skip without counting.
		// This happens for control tokens like <|end|> which shouldn't be counted.
		s.iBatch = -1
		return
	}

	// Store content for final response.
	switch {
	case s.reasonFlag > 0:
		s.finalReasoning.WriteString(resp.content)

	case s.toolFlag > 0:
		s.finalTooling.WriteString(resp.content)

	default:
		s.finalContent.WriteString(resp.content)
	}

	// Update token counts.
	switch {
	case s.reasonFlag > 0:
		s.reasonTokens++

	default:
		s.completionTokens++
	}

	// Calculate output tokens for logging (after incrementing counts).
	outputTokens := s.reasonTokens + s.completionTokens

	// Stream response if not tooling.
	if s.toolFlag == 0 {
		// Skip unnecessary CRLF at mode transitions.
		if e.model.isUnncessaryCRLF(s.reasonFlag, s.completionFlag, resp.content) {
			s.iBatch = -1
			return
		}

		// Per OpenAI spec, usage is only sent in the final response, not deltas.
		err := e.model.sendDeltaResponse(s.job.ctx, s.job.ch, s.job.id, s.job.object, 0, "", resp.content, s.reasonFlag, outputTokens, s.currentLogprob)
		if err != nil {
			e.finishSlot(s, err)
			return
		}
	}

	// Check max tokens.
	if s.nDecoded >= s.job.params.MaxTokens {
		e.finishSlot(s, nil)
		return
	}

	s.iBatch = -1
}

// finishSlot completes a slot and sends the final response.
func (e *batchEngine) finishSlot(s *slot, err error) {
	if !s.active {
		return
	}

	ctx := s.job.ctx
	jobID := s.job.id
	slotID := s.id
	seqID := s.seqID

	defer func() {
		close(s.job.ch)
		s.span.End()
		s.reset()
		e.freeSlotResources(s)

		remaining := e.model.activeStreams.Add(-1)

		e.model.log(ctx, "batch-engine",
			"status", "slot-finished",
			"slot", slotID,
			"seq", seqID,
			"id", jobID,
			"active_streams", remaining,
		)
	}()

	elapsed := time.Since(s.startTime)

	// Clear KV cache for this slot's sequence.
	llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)

	// Restore SPC cache if enabled. IMC sessions persist across requests,
	// so we don't pre-populate slots with IMC cache - it's done per-request in startSlot.
	if e.model.cfg.SystemPromptCache {
		e.model.copySystemPromptToSeq(s.seqID)
	}

	// Handle error case.
	if err != nil {
		usage := Usage{
			PromptTokens:     s.nPrompt,
			ReasoningTokens:  s.reasonTokens,
			CompletionTokens: s.completionTokens,
			OutputTokens:     s.reasonTokens + s.completionTokens,
			TotalTokens:      s.nPrompt + s.reasonTokens + s.completionTokens,
		}

		e.model.sendErrorResponse(ctx, s.job.ch, s.job.id, s.job.object, 0, "", err, usage)

		return
	}

	// Process tool calls if any. Token counts are already tracked
	// per-token in processSlotToken, so no re-tokenization needed.
	if s.toolFlag > 0 {
		content := strings.TrimSuffix(s.finalTooling.String(), "\n")
		if len(content) > 0 {
			switch {
			case e.model.modelInfo.IsGPTModel:
				s.respToolCalls = parseGPTToolCall(content)

			default:
				s.respToolCalls = parseToolCall(content)
			}
		}
	}

	// Calculate final metrics.
	outputTokens := s.reasonTokens + s.completionTokens
	totalTokens := s.nPrompt + outputTokens
	tokensPerSecond := float64(outputTokens) / elapsed.Seconds()

	usage := Usage{
		PromptTokens:     s.nPrompt,
		ReasoningTokens:  s.reasonTokens,
		CompletionTokens: s.completionTokens,
		OutputTokens:     outputTokens,
		TotalTokens:      totalTokens,
		TokensPerSecond:  tokensPerSecond,
	}

	// Add span attributes and end span.
	s.span.SetAttributes(
		attribute.Int("prompt_tokens", s.nPrompt),
		attribute.Int("reasoning_tokens", s.reasonTokens),
		attribute.Int("completion_tokens", s.completionTokens),
		attribute.Int("output_tokens", outputTokens),
		attribute.Int("total_tokens", totalTokens),
		attribute.Float64("tokens_per_second", tokensPerSecond),
	)

	// Add metrics.
	metrics.AddChatCompletionsUsage(e.model.modelInfo.ID, s.nPrompt, s.reasonTokens, s.completionTokens, outputTokens, totalTokens, tokensPerSecond)

	// Send final response.
	returnPrompt := ""
	if s.job.params.ReturnPrompt {
		returnPrompt = s.job.prompt
	}

	e.model.sendFinalResponse(ctx, s.job.ch, s.job.id, s.job.object, 0, returnPrompt,
		&s.finalContent, &s.finalReasoning, s.respToolCalls, s.logprobsData, s.job.params.Stream, usage)

	e.model.log(ctx, "batch-engine", "status", "slot-finished", "slot", s.id, "id", s.job.id,
		"prompt", s.nPrompt, "output", outputTokens, "time", elapsed.String())
}

func (e *batchEngine) freeSlotResources(s *slot) {
	if s.sampler != 0 {
		llama.SamplerFree(s.sampler)
		s.sampler = 0
	}
}

func (e *batchEngine) sendSlotError(s *slot, err error) {
	usage := Usage{PromptTokens: s.nPrompt}
	e.model.sendErrorResponse(s.job.ctx, s.job.ch, s.job.id, s.job.object, 0, "", err, usage)
	close(s.job.ch)
}

// drainSlots finishes all active slots and pending jobs during shutdown.
func (e *batchEngine) drainSlots() {
	ctx := context.Background()

	activeCount := 0
	for _, s := range e.slots {
		if s.active {
			activeCount++
		}
	}

	pendingCount := len(e.requestQ)

	e.model.log(ctx, "batch-engine", "status", "drain-started", "active_slots", activeCount, "pending_jobs", pendingCount)

	for _, s := range e.slots {
		if s.active {
			e.finishSlot(s, fmt.Errorf("drain-slots: engine shutting down"))
		}
	}

	// Drain pending jobs that were never assigned to a slot.
	drained := 0
	for {
		select {
		case job := <-e.requestQ:
			close(job.ch)
			e.model.activeStreams.Add(-1)
			drained++
		default:
			e.model.log(ctx, "batch-engine", "status", "drain-finished", "drained_pending", drained)
			return
		}
	}
}

// =============================================================================
// Batch manipulation helpers

func batchClear(batch *llama.Batch) {
	batch.NTokens = 0
}

func batchAdd(batch *llama.Batch, token llama.Token, pos llama.Pos, seqIDs []llama.SeqId, logits bool) {
	i := batch.NTokens

	tokenPtr := (*llama.Token)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.Token)) + uintptr(i)*unsafe.Sizeof(llama.Token(0))))
	*tokenPtr = token

	posPtr := (*llama.Pos)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.Pos)) + uintptr(i)*unsafe.Sizeof(llama.Pos(0))))
	*posPtr = pos

	nSeqPtr := (*int32)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.NSeqId)) + uintptr(i)*unsafe.Sizeof(int32(0))))
	*nSeqPtr = int32(len(seqIDs))

	seqIDPtrPtr := (**llama.SeqId)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.SeqId)) + uintptr(i)*unsafe.Sizeof(uintptr(0))))
	if *seqIDPtrPtr != nil && len(seqIDs) > 0 {
		for j, sid := range seqIDs {
			seqPtr := (*llama.SeqId)(unsafe.Pointer(uintptr(unsafe.Pointer(*seqIDPtrPtr)) + uintptr(j)*unsafe.Sizeof(llama.SeqId(0))))
			*seqPtr = sid
		}
	}

	logitPtr := (*int8)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.Logits)) + uintptr(i)*unsafe.Sizeof(int8(0))))
	switch logits {
	case true:
		*logitPtr = 1
	case false:
		*logitPtr = 0
	}

	batch.NTokens++
}

// logDecodeError logs detailed KV cache diagnostics when decode fails.
func (e *batchEngine) logDecodeError(ctx context.Context, ret int32, err error) {
	nCtx := llama.NCtx(e.model.lctx)

	// Collect per-slot diagnostics.
	var totalTokens llama.Pos
	slotInfo := make([]string, 0, e.nSlots+1)

	// Check system prompt cache (seq 0).
	if sysMax, sysErr := llama.MemorySeqPosMax(e.model.mem, 0); sysErr == nil && sysMax >= 0 {
		slotInfo = append(slotInfo, fmt.Sprintf("sys[0]=%d", sysMax+1))
		totalTokens += sysMax + 1
	}

	// Check each slot's sequence.
	for _, s := range e.slots {
		if !s.active {
			continue
		}
		posMax, posErr := llama.MemorySeqPosMax(e.model.mem, s.seqID)
		if posErr == nil && posMax >= 0 {
			tokens := posMax + 1
			slotInfo = append(slotInfo, fmt.Sprintf("slot[%d,seq=%d]=%d", s.id, s.seqID, tokens))
			totalTokens += tokens
		}
	}

	e.model.log(ctx, "batch-engine",
		"status", "decode-error",
		"ret", ret,
		"err", err,
		"n_ctx", nCtx,
		"kv_used", totalTokens,
		"batch_tokens", e.batch.NTokens,
		"active_slots", len(slotInfo),
		"slot_usage", strings.Join(slotInfo, ","),
	)
}

// decodeError returns a human-readable error message for llama_decode return codes.
// Return codes from llama.cpp:
//
//	0  - success
//	1  - could not find a KV slot for the batch (try reducing batch size or increase context)
//	2  - aborted
//	-1 - invalid input batch
//	<-1 - fatal error
func decodeError(ret int32, err error) error {
	var msg string
	switch ret {
	case 1:
		msg = "unable to process request: the context window is full. Please reduce the input size or increase the context window"
	case 2:
		msg = "request was cancelled"
	case -1:
		msg = "unable to process request: the input could not be processed. Please try reducing the input size or context length"
	default:
		switch {
		case ret < -1:
			msg = "an internal error occurred while processing your request"
		default:
			msg = "an unexpected error occurred while processing your request"
		}
	}

	if err != nil {
		return fmt.Errorf("%s: %w", msg, err)
	}
	return errors.New(msg)
}
