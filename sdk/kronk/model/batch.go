package model

import (
	"context"
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
	id      string
	ctx     context.Context
	d       D
	object  string
	prompt  string
	media   [][]byte
	params  Params
	mtmdCtx mtmd.Context
	ch      chan<- ChatResponse
}

// slot represents a processing slot for parallel inference.
type slot struct {
	id    int
	seqID llama.SeqId

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
}

func (s *slot) reset() {
	s.seqID = -1
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
	shutdownCh chan struct{}
	wg         sync.WaitGroup
	stopped    atomic.Bool
}

// newBatchEngine creates a new batch engine for parallel inference.
func newBatchEngine(m *Model, nSlots int) *batchEngine {
	// Create batch buffer.
	nCtx := llama.NCtx(m.lctx)
	batch := llama.BatchInit(int32(nCtx), 0, int32(nSlots))

	// Initialize slots.
	slots := make([]*slot, nSlots)
	for i := range slots {
		slots[i] = &slot{
			id:    i,
			seqID: llama.SeqId(i + 1), // SeqID 0 reserved for system prompt if needed
			proc:  newProcessor(m),
		}
		slots[i].reset()
	}

	return &batchEngine{
		model:      m,
		nSlots:     nSlots,
		slots:      slots,
		batch:      batch,
		requestQ:   make(chan *chatJob, nSlots*2),
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
		return // Already stopped
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

		case job := <-e.requestQ:
			// Requeue job for fillSlots to handle in correct order
			// (after batchClear but before decode).
			e.requestQ <- job
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
		batchAdd(&e.batch, s.sampled, s.nPast, []llama.SeqId{s.seqID}, true)
		s.nPast++
		s.nDecoded++
	}

	// Fill empty slots from queue.
	e.fillSlots()

	// Nothing to process.
	if e.batch.NTokens == 0 {
		return
	}

	// Decode the batch.
	ret, err := llama.Decode(e.model.lctx, e.batch)
	if err != nil || ret != 0 {
		e.model.log(ctx, "batch-engine", "status", "decode-error", "ret", ret, "err", err)
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
	s.startTime = time.Now()
	s.seqID = llama.SeqId(s.id + 1)

	// Start span for this chat request.
	_, s.span = otel.AddSpan(job.ctx, "batch-chat-request",
		attribute.String("id", job.id),
		attribute.Int("slot", s.id),
	)

	// Create sampler for this request.
	s.sampler = toSampler(job.params)

	// Tokenize the prompt.
	tokens := llama.Tokenize(e.model.vocab, job.prompt, true, true)
	s.nPrompt = len(tokens)

	// Check context window.
	if s.nPrompt > e.model.cfg.ContextWindow {
		err := fmt.Errorf("start-slot: input tokens [%d] exceed context window [%d]", s.nPrompt, e.model.cfg.ContextWindow)
		e.sendSlotError(s, err)
		s.reset()
		return
	}

	// Track prefill time.
	prefillStart := time.Now()

	// Add prompt tokens to batch for prefill.
	for i, tok := range tokens {
		logits := i == len(tokens)-1 // Only need logits for last token
		batchAdd(&e.batch, tok, s.nPast, []llama.SeqId{s.seqID}, logits)
		s.nPast++
	}

	prefillDuration := time.Since(prefillStart)
	metrics.AddPrefillNonMediaTime(prefillDuration)
	s.span.SetAttributes(attribute.String("prefill-nonmedia", prefillDuration.String()))

	s.iBatch = e.batch.NTokens - 1

	e.model.log(job.ctx, "batch-engine", "status", "slot-started", "slot", s.id, "id", job.id, "prompt_tokens", s.nPrompt)
}

// processSlotToken handles a sampled token for a slot.
func (e *batchEngine) processSlotToken(s *slot, buf []byte) {
	// Sample the next token.
	token := llama.SamplerSample(s.sampler, e.model.lctx, s.iBatch)
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
	s.prefillDone = true
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
		// No content to process.
		s.iBatch = -1
		return
	}

	// Calculate tokens per second.
	elapsedSeconds := time.Since(s.startTime).Seconds()
	outputTokens := s.reasonTokens + s.completionTokens
	tokensPerSecond := float64(outputTokens) / elapsedSeconds

	// Stream response if not tooling.
	if s.toolFlag == 0 {
		// Skip unnecessary CRLF at mode transitions.
		if e.model.isUnncessaryCRLF(s.reasonFlag, s.completionFlag, resp.content) {
			s.iBatch = -1
			return
		}

		usage := Usage{
			PromptTokens:     s.nPrompt,
			ReasoningTokens:  s.reasonTokens,
			CompletionTokens: s.completionTokens,
			OutputTokens:     outputTokens,
			TotalTokens:      s.nPrompt + outputTokens,
			TokensPerSecond:  tokensPerSecond,
		}

		err := e.model.sendDeltaResponse(s.job.ctx, s.job.ch, s.job.id, s.job.object, s.index, "", resp.content, s.reasonFlag, usage)
		if err != nil {
			e.finishSlot(s, err)
			return
		}
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

	defer func() {
		close(s.job.ch)
		s.span.End()
		s.reset()
		e.freeSlotResources(s)
		e.model.activeStreams.Add(-1)
	}()

	ctx := s.job.ctx
	elapsed := time.Since(s.startTime)

	// Clear KV cache for this slot's sequence.
	llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)

	// Handle error case.
	if err != nil {
		usage := Usage{
			PromptTokens:     s.nPrompt,
			ReasoningTokens:  s.reasonTokens,
			CompletionTokens: s.completionTokens,
			OutputTokens:     s.reasonTokens + s.completionTokens,
			TotalTokens:      s.nPrompt + s.reasonTokens + s.completionTokens,
		}

		e.model.sendErrorResponse(ctx, s.job.ch, s.job.id, s.job.object, s.index, "", err, usage)

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
	metrics.AddChatCompletionsUsage(s.nPrompt, s.reasonTokens, s.completionTokens, outputTokens, totalTokens, tokensPerSecond)

	// Send final response.
	returnPrompt := ""
	if s.job.params.ReturnPrompt {
		returnPrompt = s.job.prompt
	}

	e.model.sendFinalResponse(ctx, s.job.ch, s.job.id, s.job.object, s.index, returnPrompt,
		&s.finalContent, &s.finalReasoning, s.respToolCalls, usage)

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

// drainSlots finishes all active slots during shutdown.
func (e *batchEngine) drainSlots() {
	for _, s := range e.slots {
		if s.active {
			e.finishSlot(s, fmt.Errorf("darin-slots: engine shutting down"))
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
	if logits {
		*logitPtr = 1
	} else {
		*logitPtr = 0
	}

	batch.NTokens++
}
