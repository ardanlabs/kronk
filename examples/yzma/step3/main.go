// This example demonstrates a web server architecture for parallel LLM inference.
// Multiple HTTP clients submit requests which are queued and processed in batches
// by a single batch processor goroutine.
//
// Architecture:
//   HTTP Handlers ──► Request Queue ──► Batch Processor ──► Response Channels
//                        (chan)         (single goroutine)
//
// Run the example like this from the root of the project:
// $ go run examples/yzma-parallel/step2/main.go -model /path/to/model.gguf

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/hybridgroup/yzma/pkg/llama"
)

func main() {
	modelPath := flag.String("model", "", "Path to the GGUF model file")
	nParallel := flag.Int("parallel", 2, "Number of parallel slots")
	nPredict := flag.Int("predict", 128, "Default max tokens per request")
	addr := flag.String("addr", ":8090", "Server address")
	flag.Parse()

	if *modelPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("Unable to get home dir: %v", err)
		}

		*modelPath = filepath.Join(home, ".kronk/models/Qwen/Qwen3-8B-GGUF/Qwen3-8B-Q8_0.gguf")
	}

	// Create batch processor.
	log.Println("Initializing batch processor...")
	bp, err := NewBatchProcessor(*modelPath, *nParallel, *nPredict)
	if err != nil {
		log.Fatalf("Failed to create batch processor: %v", err)
	}

	// Start batch processor.
	bp.Start()

	// Create and start HTTP server.
	server := NewServer(bp, *addr)

	// Handle graceful shutdown.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		log.Println("Shutting down...")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		server.Shutdown(ctx)
		bp.Close()
	}()

	log.Printf("Server ready at http://localhost%s", *addr)
	log.Println("Endpoints:")
	log.Println("  POST /v1/completions - Submit completion request")
	log.Println("  GET  /v1/stats       - Get server statistics")
	log.Println("  GET  /health         - Health check")
	log.Println()
	log.Println("Example:")
	log.Println(`  curl -X POST http://localhost:8090/v1/completions \`)
	log.Println(`    -H "Content-Type: application/json" \`)
	log.Println(`    -d '{"prompt": "Hello, how are you?", "max_tokens": 50}'`)

	if err := server.Start(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}

// =============================================================================
// Request/Response types

// InferenceRequest represents an incoming inference request.
type InferenceRequest struct {
	ID         string
	Prompt     string
	MaxTokens  int
	ResponseCh chan InferenceResponse
	ctx        context.Context
}

// InferenceResponse represents a streaming response token or completion.
type InferenceResponse struct {
	Token    string
	Done     bool
	Error    error
	Duration time.Duration
}

// =============================================================================
// Slot represents a processing slot for a client request

type slot struct {
	id            int32
	seqID         llama.SeqId
	sampled       llama.Token
	tStart        time.Time
	nPast         llama.Pos
	nPrompt       int32
	nDecoded      int32
	iBatch        int32
	maxTokens     int32
	response      strings.Builder
	sampler       llama.Sampler
	request       *InferenceRequest
	active        bool
	hasPromptDone bool // true after initial prompt decode, ready for token generation
}

func (s *slot) reset() {
	s.seqID = -1
	s.sampled = 0
	s.nPast = 0
	s.nPrompt = 0
	s.nDecoded = 0
	s.iBatch = -1
	s.response.Reset()
	s.request = nil
	s.active = false
	s.hasPromptDone = false
}

// =============================================================================
// BatchProcessor handles batched inference

type BatchProcessor struct {
	model        llama.Model
	vocab        llama.Vocab
	lctx         llama.Context
	mem          llama.Memory
	batch        llama.Batch
	slots        []*slot
	nParallel    int
	nPredict     int
	systemTokens int32
	requestQueue chan *InferenceRequest
	shutdownCh   chan struct{}
	wg           sync.WaitGroup

	// Stats
	mu            sync.Mutex
	totalPrompt   int64
	totalGen      int64
	totalRequests int64
}

// NewBatchProcessor creates a new batch processor.
func NewBatchProcessor(modelPath string, nParallel, nPredict int) (*BatchProcessor, error) {
	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	// Load model.
	mparams := llama.ModelDefaultParams()
	mdl, err := llama.ModelLoadFromFile(modelPath, mparams)
	if err != nil {
		return nil, fmt.Errorf("unable to load model: %w", err)
	}

	vocab := llama.ModelGetVocab(mdl)

	// Create context with slots for parallel sequences.
	ctxParams := llama.ContextDefaultParams()
	ctxParams.NSeqMax = uint32(nParallel + 1) // +1 for system prompt
	ctxParams.NCtx = 4096

	lctx, err := llama.InitFromModel(mdl, ctxParams)
	if err != nil {
		llama.ModelFree(mdl)
		return nil, fmt.Errorf("unable to init context: %w", err)
	}

	mem, err := llama.GetMemory(lctx)
	if err != nil {
		llama.Free(lctx)
		llama.ModelFree(mdl)
		return nil, fmt.Errorf("unable to get memory: %w", err)
	}

	nCtx := llama.NCtx(lctx)

	// Allocate batch.
	batch := llama.BatchInit(int32(nCtx), 0, int32(nParallel+1))

	// Initialize slots.
	slots := make([]*slot, nParallel)
	for i := range slots {
		slots[i] = &slot{
			id:      int32(i),
			seqID:   -1,
			sampler: llama.SamplerChainInit(llama.SamplerChainDefaultParams()),
		}
		llama.SamplerChainAdd(slots[i].sampler, llama.SamplerInitTopK(40))
		llama.SamplerChainAdd(slots[i].sampler, llama.SamplerInitTopP(0.9, 1))
		llama.SamplerChainAdd(slots[i].sampler, llama.SamplerInitTempExt(0.7, 0.0, 1.0))
		llama.SamplerChainAdd(slots[i].sampler, llama.SamplerInitDist(uint32(i)+1))
	}

	bp := &BatchProcessor{
		model:        mdl,
		vocab:        vocab,
		lctx:         lctx,
		mem:          mem,
		batch:        batch,
		slots:        slots,
		nParallel:    nParallel,
		nPredict:     nPredict,
		requestQueue: make(chan *InferenceRequest, 100),
		shutdownCh:   make(chan struct{}),
	}

	// Evaluate system prompt and cache it.
	if err := bp.initSystemPrompt(); err != nil {
		bp.Close()
		return nil, err
	}

	return bp, nil
}

// Qwen3 ChatML format
const systemPrompt = "<|im_start|>system\nYou are a helpful assistant.<|im_end|>\n"

func (bp *BatchProcessor) initSystemPrompt() error {
	tokensSystem := llama.Tokenize(bp.vocab, systemPrompt, true, true)
	bp.systemTokens = int32(len(tokensSystem))

	batch := llama.BatchGetOne(tokensSystem)
	if _, err := llama.Decode(bp.lctx, batch); err != nil {
		return fmt.Errorf("failed to decode system prompt: %w", err)
	}

	// Copy system prompt KV cache to all slots.
	for i := 1; i <= bp.nParallel; i++ {
		if err := llama.MemorySeqCp(bp.mem, 0, llama.SeqId(i), -1, -1); err != nil {
			return fmt.Errorf("failed to copy memory: %w", err)
		}
	}

	log.Printf("System prompt cached (%d tokens)", bp.systemTokens)
	return nil
}

// Submit adds a request to the processing queue.
func (bp *BatchProcessor) Submit(req *InferenceRequest) {
	select {
	case bp.requestQueue <- req:
	case <-bp.shutdownCh:
		req.ResponseCh <- InferenceResponse{Error: fmt.Errorf("server shutting down"), Done: true}
	}
}

// Start begins the batch processing loop.
func (bp *BatchProcessor) Start() {
	bp.wg.Add(1)
	go bp.processLoop()
}

// processLoop is the main batch processing goroutine.
func (bp *BatchProcessor) processLoop() {
	defer bp.wg.Done()

	log.Printf("Batch processor started (parallel=%d, predict=%d)", bp.nParallel, bp.nPredict)

	ticker := time.NewTicker(1 * time.Millisecond) // Fast polling for responsiveness
	defer ticker.Stop()

	for {
		select {
		case <-bp.shutdownCh:
			bp.drainSlots()
			return
		case <-ticker.C:
			bp.processBatch()
		}
	}
}

// processBatch handles one iteration of the batch processing loop.
func (bp *BatchProcessor) processBatch() {
	// Clear the batch.
	batchClear(&bp.batch)

	// Add tokens from active slots that have completed prompt decode.
	for _, s := range bp.slots {
		if !s.active || !s.hasPromptDone {
			continue
		}

		// Check if client cancelled.
		if s.request.ctx.Err() != nil {
			bp.finishSlot(s, fmt.Errorf("client cancelled"))
			continue
		}

		s.iBatch = bp.batch.NTokens
		batchAdd(&bp.batch, s.sampled, s.nPast, []llama.SeqId{llama.SeqId(s.id + 1)}, true)
		s.nPast++
		s.nDecoded++
	}

	// Fill empty slots from queue.
	bp.fillSlots()

	// Nothing to process.
	if bp.batch.NTokens == 0 {
		return
	}

	// Decode the batch.
	ret, err := llama.Decode(bp.lctx, bp.batch)
	if err != nil || ret != 0 {
		log.Printf("Warning: decode failed, ret=%d, err=%v", ret, err)
		return
	}

	// Sample tokens for each active slot.
	for _, s := range bp.slots {
		if s.iBatch < 0 || !s.active {
			continue
		}

		// Sample the next token.
		token := llama.SamplerSample(s.sampler, bp.lctx, s.iBatch)
		llama.SamplerAccept(s.sampler, token)

		// Convert token to text.
		buf := make([]byte, 256)
		l := llama.TokenToPiece(bp.vocab, token, buf, 0, true)
		tokenStr := string(buf[:l])

		s.response.WriteString(tokenStr)
		s.sampled = token
		s.hasPromptDone = true // Mark that prompt decode is complete

		// Stream token to client.
		select {
		case s.request.ResponseCh <- InferenceResponse{Token: tokenStr}:
		default:
			// Client not reading, skip
		}

		// Check for end of generation.
		shouldStop := false
		if s.nDecoded > 2 {
			if llama.VocabIsEOG(bp.vocab, token) {
				shouldStop = true
			}
			if s.nDecoded >= s.maxTokens {
				shouldStop = true
			}
			if strings.Contains(s.response.String(), "<|im_end|>") {
				shouldStop = true
			}
		}

		if shouldStop {
			bp.finishSlot(s, nil)
		}

		s.iBatch = -1
	}
}

// fillSlots assigns pending requests to available slots.
func (bp *BatchProcessor) fillSlots() {
	for _, s := range bp.slots {
		if s.active {
			continue
		}

		// Try to get a request from the queue.
		select {
		case req := <-bp.requestQueue:
			bp.startSlot(s, req)
		default:
			// No pending requests
			return
		}
	}
}

// startSlot initializes a slot with a new request.
func (bp *BatchProcessor) startSlot(s *slot, req *InferenceRequest) {
	s.active = true
	s.request = req
	s.tStart = time.Now()
	s.response.Reset()
	s.nPast = llama.Pos(bp.systemTokens)
	s.nDecoded = 0

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = bp.nPredict
	}
	s.maxTokens = int32(maxTokens)

	// Build prompt in Qwen3 ChatML format.
	prompt := "<|im_start|>user\n" + req.Prompt + "<|im_end|>\n<|im_start|>assistant\n"

	llama.SamplerReset(s.sampler)

	// Tokenize (no BOS since we have system prompt).
	tokens := llama.Tokenize(bp.vocab, prompt, false, true)
	s.nPrompt = int32(len(tokens))

	// Add prompt tokens to batch.
	for _, tok := range tokens {
		batchAdd(&bp.batch, tok, s.nPast, []llama.SeqId{llama.SeqId(s.id + 1)}, false)
		s.nPast++
	}

	// Enable logits for last token.
	if bp.batch.NTokens > 0 {
		setLogit(&bp.batch, bp.batch.NTokens-1, true)
	}

	s.iBatch = bp.batch.NTokens - 1
	s.seqID = llama.SeqId(s.id)

	log.Printf("[Slot %d] Started request %s (%d prompt tokens)", s.id, req.ID, s.nPrompt)
}

// finishSlot completes a slot and sends the final response.
func (bp *BatchProcessor) finishSlot(s *slot, err error) {
	if !s.active {
		return
	}

	// Clear KV cache for this slot.
	llama.MemorySeqRm(bp.mem, llama.SeqId(s.id+1), -1, -1)
	llama.MemorySeqCp(bp.mem, 0, llama.SeqId(s.id+1), -1, -1)

	elapsed := time.Since(s.tStart)

	// Send final response.
	resp := InferenceResponse{
		Done:     true,
		Duration: elapsed,
		Error:    err,
	}
	select {
	case s.request.ResponseCh <- resp:
	default:
	}
	close(s.request.ResponseCh)

	// Update stats.
	bp.mu.Lock()
	bp.totalPrompt += int64(s.nPrompt)
	bp.totalGen += int64(s.nDecoded)
	bp.totalRequests++
	bp.mu.Unlock()

	log.Printf("[Slot %d] Finished request %s (prompt=%d, gen=%d, time=%.2fs)",
		s.id, s.request.ID, s.nPrompt, s.nDecoded, elapsed.Seconds())

	s.reset()
}

// drainSlots finishes all active slots during shutdown.
func (bp *BatchProcessor) drainSlots() {
	for _, s := range bp.slots {
		if s.active {
			bp.finishSlot(s, fmt.Errorf("server shutting down"))
		}
	}
}

// Stats returns current statistics.
func (bp *BatchProcessor) Stats() (prompt, gen, requests int64) {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	return bp.totalPrompt, bp.totalGen, bp.totalRequests
}

// Close shuts down the processor and frees resources.
func (bp *BatchProcessor) Close() {
	close(bp.shutdownCh)
	bp.wg.Wait()

	for _, s := range bp.slots {
		llama.SamplerFree(s.sampler)
	}
	llama.BatchFree(bp.batch)
	llama.Free(bp.lctx)
	llama.ModelFree(bp.model)

	log.Println("Batch processor shut down")
}

// =============================================================================
// HTTP Server

type Server struct {
	bp     *BatchProcessor
	server *http.Server
	reqID  int64
	mu     sync.Mutex
}

func NewServer(bp *BatchProcessor, addr string) *Server {
	s := &Server{bp: bp}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/completions", s.handleCompletions)
	mux.HandleFunc("/v1/stats", s.handleStats)
	mux.HandleFunc("/health", s.handleHealth)

	s.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return s
}

type CompletionRequest struct {
	Prompt    string `json:"prompt"`
	MaxTokens int    `json:"max_tokens"`
	Stream    bool   `json:"stream"`
}

type CompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Choices []struct {
		Text         string `json:"text"`
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
}

func (s *Server) handleCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Prompt == "" {
		http.Error(w, "Prompt is required", http.StatusBadRequest)
		return
	}

	// Generate request ID.
	s.mu.Lock()
	s.reqID++
	reqID := fmt.Sprintf("req-%d", s.reqID)
	s.mu.Unlock()

	// Create inference request.
	infReq := &InferenceRequest{
		ID:         reqID,
		Prompt:     req.Prompt,
		MaxTokens:  req.MaxTokens,
		ResponseCh: make(chan InferenceResponse, 100),
		ctx:        r.Context(),
	}

	// Submit to batch processor.
	s.bp.Submit(infReq)

	switch req.Stream {
	case true:
		s.handleStreamingResponse(w, infReq)
	case false:
		s.handleNonStreamingResponse(w, infReq, reqID)
	}
}

func (s *Server) handleStreamingResponse(w http.ResponseWriter, req *InferenceRequest) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	for resp := range req.ResponseCh {
		if resp.Error != nil {
			fmt.Fprintf(w, "data: {\"error\": \"%s\"}\n\n", resp.Error.Error())
			flusher.Flush()
			return
		}

		if resp.Done {
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			return
		}

		data := map[string]any{
			"choices": []map[string]any{
				{"text": resp.Token, "index": 0},
			},
		}
		jsonData, _ := json.Marshal(data)
		fmt.Fprintf(w, "data: %s\n\n", jsonData)
		flusher.Flush()
	}
}

func (s *Server) handleNonStreamingResponse(w http.ResponseWriter, req *InferenceRequest, reqID string) {
	var fullText strings.Builder

	for resp := range req.ResponseCh {
		if resp.Error != nil {
			http.Error(w, resp.Error.Error(), http.StatusInternalServerError)
			return
		}
		if !resp.Done {
			fullText.WriteString(resp.Token)
		}
	}

	response := CompletionResponse{
		ID:      reqID,
		Object:  "text_completion",
		Created: time.Now().Unix(),
		Choices: []struct {
			Text         string `json:"text"`
			Index        int    `json:"index"`
			FinishReason string `json:"finish_reason,omitempty"`
		}{
			{Text: fullText.String(), Index: 0, FinishReason: "stop"},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	prompt, gen, requests := s.bp.Stats()
	stats := map[string]any{
		"total_prompt_tokens": prompt,
		"total_gen_tokens":    gen,
		"total_requests":      requests,
		"pending_requests":    len(s.bp.requestQueue),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *Server) Start() error {
	log.Printf("HTTP server starting on %s", s.server.Addr)
	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
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

func setLogit(batch *llama.Batch, idx int32, logits bool) {
	logitPtr := (*int8)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.Logits)) + uintptr(idx)*unsafe.Sizeof(int8(0))))
	switch logits {
	case true:
		*logitPtr = 1
	case false:
		*logitPtr = 0
	}
}
