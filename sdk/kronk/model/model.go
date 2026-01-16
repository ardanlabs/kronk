// Package model provides the low-level api for working with models.
package model

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"go.opentelemetry.io/otel/attribute"
)

// TemplateRetriever returns a configured template for a model.
type TemplateRetriever interface {
	Retrieve(modelID string) (Template, error)
}

// Model represents a model and provides a low-level API for working with it.
type Model struct {
	cfg           Config
	log           Logger
	model         llama.Model
	vocab         llama.Vocab
	ctxParams     llama.ContextParams
	lctx          llama.Context
	mem           llama.Memory
	batch         *batchEngine
	template      Template
	projFile      string
	modelInfo     ModelInfo
	activeStreams atomic.Int32
	unloaded      atomic.Bool
}

func NewModel(ctx context.Context, tmplRetriever TemplateRetriever, cfg Config) (*Model, error) {
	l := cfg.Log
	if cfg.Log == nil {
		l = func(ctx context.Context, msg string, args ...any) {}
	}

	if tmplRetriever == nil {
		return nil, fmt.Errorf("templater required, use templater.New()")
	}

	if err := validateConfig(ctx, cfg, l); err != nil {
		return nil, fmt.Errorf("validate-config: unable to validate config: %w", err)
	}

	mParams := llama.ModelDefaultParams()

	if cfg.Device != "" {
		dev := llama.GGMLBackendDeviceByName(cfg.Device)
		if dev == 0 {
			return nil, fmt.Errorf("ggml-backend-device-by-name: unknown device: %s", cfg.Device)
		}
		mParams.SetDevices([]llama.GGMLBackendDevice{dev})
	}

	// llama.cpp has a -1 default for loading all layers into the GPU
	// However, we want to make it convenient to write the configuration.
	// So, we default to invert these two values after loading them.
	switch {
	case cfg.NGpuLayers == nil:
		mParams.NGpuLayers = -1
	case *cfg.NGpuLayers == 0:
		mParams.NGpuLayers = -1
	case *cfg.NGpuLayers == -1:
		mParams.NGpuLayers = 0
	default:
		mParams.NGpuLayers = *cfg.NGpuLayers
	}

	// Set split mode for multi-GPU and tensor parallelism (expert-parallel for MoE).
	// Default to SplitModeRow (tensor parallelism) when not explicitly configured,
	// as it provides the best performance for MoE models and works well for dense models.
	if cfg.SplitMode == SplitModeNone {
		mParams.SplitMode = SplitModeRow.ToYZMAType()
	} else {
		mParams.SplitMode = cfg.SplitMode.ToYZMAType()
	}

	// -------------------------------------------------------------------------

	mdl, err := loadModelFromFiles(ctx, l, cfg.ModelFiles, mParams)
	if err != nil {
		return nil, fmt.Errorf("load-model-from-files: unable to load model: %w", err)
	}

	// -------------------------------------------------------------------------

	cfg = adjustConfig(cfg, mdl)
	modelInfo := toModelInfo(cfg, mdl)

	template, err := retrieveTemplate(tmplRetriever, cfg, mdl, modelInfo)
	if err != nil {
		return nil, fmt.Errorf("retrieve-template: failed to retrieve model template: %w", err)
	}

	modelInfo.Template = template

	// -------------------------------------------------------------------------

	ctxParams := modelCtxParams(cfg, modelInfo)

	l(ctx, "context-params", "NCtx", ctxParams.NCtx, "NBatch", ctxParams.NBatch, "NUBatch", ctxParams.NUbatch, "NSeqMax", ctxParams.NSeqMax, "TypeK", ctxParams.TypeK, "TypeV", ctxParams.TypeV, "NThreads", ctxParams.NThreads, "NThreadsBatch", ctxParams.NThreadsBatch)

	lctx, err := llama.InitFromModel(mdl, ctxParams)
	if err != nil {
		llama.ModelFree(mdl)
		return nil, fmt.Errorf("init-from-model: unable to init context: %w", err)
	}

	mem, err := llama.GetMemory(lctx)
	if err != nil {
		llama.Free(lctx)
		llama.ModelFree(mdl)
		return nil, fmt.Errorf("get-memory: unable to get memory: %w", err)
	}

	m := Model{
		cfg:       cfg,
		log:       l,
		model:     mdl,
		vocab:     llama.ModelGetVocab(mdl),
		ctxParams: ctxParams,
		lctx:      lctx,
		mem:       mem,
		template:  template,
		projFile:  cfg.ProjFile,
		modelInfo: modelInfo,
	}

	// Initialize batch engine for text-only models (no ProjFile).
	// Batching is faster even for single-sequence inference.
	if cfg.ProjFile == "" {
		nSlots := max(cfg.NSeqMax, 1)
		m.batch = newBatchEngine(&m, nSlots)
		m.batch.start(ctx)
	}

	return &m, nil
}

func loadModelFromFiles(ctx context.Context, log Logger, modelFiles []string, params llama.ModelParams) (llama.Model, error) {
	baseModelFile := path.Base(modelFiles[0])

	log(ctx, "loading model from file", "status", "started", "model", baseModelFile)
	defer log(ctx, "loading model from file", "status", "completed", "model", baseModelFile)

	_, span := otel.AddSpan(ctx, "proj-file-load-time",
		attribute.String("model-file", baseModelFile),
	)
	defer span.End()

	start := time.Now()
	defer func() {
		metrics.AddModelFileLoadTime(time.Since(start))
	}()

	var err error
	var mdl llama.Model

	switch len(modelFiles) {
	case 1:
		mdl, err = llama.ModelLoadFromFile(modelFiles[0], params)
		if err != nil {
			return 0, fmt.Errorf("model-load-from-file: unable to load model: %w", err)
		}

	default:
		mdl, err = llama.ModelLoadFromSplits(modelFiles, params)
		if err != nil {
			return 0, fmt.Errorf("model-load-from-splits: unable to load model from split: %w", err)
		}
	}

	return mdl, nil
}

func retrieveTemplate(tmlpRetriever TemplateRetriever, cfg Config, mdl llama.Model, modelInfo ModelInfo) (Template, error) {
	if cfg.JinjaFile != "" {
		data, err := readJinjaTemplate(cfg.JinjaFile)
		if err != nil {
			return Template{}, fmt.Errorf("read-jinja-template: failed to read jinja template: %w", err)
		}

		if data == "" {
			return Template{}, fmt.Errorf("read-jinja-template: jinja template is empty")
		}

		return Template{
			FileName: cfg.JinjaFile,
			Script:   data,
		}, nil
	}

	if tmlpRetriever != nil {
		template, err := tmlpRetriever.Retrieve(modelInfo.ID)
		if err == nil {
			return template, nil
		}
	}

	data := llama.ModelChatTemplate(mdl, "")
	if data == "" {
		data, _ = llama.ModelMetaValStr(mdl, "tokenizer.chat_template")
	}

	return Template{
		FileName: "tokenizer.chat_template",
		Script:   data,
	}, nil
}

func (m *Model) Unload(ctx context.Context) error {
	if !m.unloaded.CompareAndSwap(false, true) {
		return nil // Already unloaded
	}

	if _, exists := ctx.Deadline(); !exists {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}

	// Stop the batch engine if running.
	hasBatch := m.batch != nil
	if hasBatch {
		m.batch.stop(ctx)
	}

	for m.activeStreams.Load() > 0 {
		select {
		case <-ctx.Done():
			return fmt.Errorf("unload: cannot unload %d active streams: %w", m.activeStreams.Load(), ctx.Err())

		case <-time.After(100 * time.Millisecond):
		}
	}

	// Free batch buffer before context (batch references context internals).
	if hasBatch {
		m.batch.freeBatch()
	}

	// Synchronize ensures all GPU operations complete before freeing.
	llama.Synchronize(m.lctx)
	llama.Free(m.lctx)
	llama.ModelFree(m.model)
	llama.BackendFree()

	return nil
}

func (m *Model) Config() Config {
	return m.cfg
}

func (m *Model) ModelInfo() ModelInfo {
	return m.modelInfo
}

func (m *Model) resetContext() {
	llama.Synchronize(m.lctx)

	mem, err := llama.GetMemory(m.lctx)
	if err == nil {
		llama.MemoryClear(mem, true)
	}
}

func (m *Model) sequentialChatRequest(ctx context.Context, id string, lctx llama.Context, mtmdCtx mtmd.Context, object string, prompt string, media [][]byte, params params, ch chan<- ChatResponse) {
	m.log(ctx, "process-chat-request", "status", "started", "id", id, "object", object)
	defer m.log(ctx, "process-chat-request", "status", "completed", "id", id, "object", object)

	ctx, span := otel.AddSpan(ctx, "process-chat-request",
		attribute.String("id", id),
		attribute.String("object", object),
	)
	defer span.End()

	// These are for token counting.
	var (
		inputTokens      int
		completionTokens int
		reasonTokens     int
		tokensPerSecond  float64
	)

	// These flags track what mode the model is operating in.
	var (
		reasonFlag     int
		completionFlag int
		toolFlag       int
	)

	// These builders contain the final content for each of these items.
	var (
		finalReasoning strings.Builder
		finalContent   strings.Builder
		finalTooling   strings.Builder
	)

	// Index is used to provide the index for each response.
	var index int

	// The buffer is used to process tokens.
	const bufferSize = 32 * 1024
	buf := make([]byte, bufferSize)

	// -------------------------------------------------------------------------

	m.log(ctx, "process-chat-request", "status", "process input tokens")

	// Start timing for time-to-first-token metric.
	ttftStart := time.Now()

	// Process the prompt and get the first batch for the response.
	sampler, batch, inputTokens, outputTokens, bitmaps := m.processInputTokens(ctx, lctx, mtmdCtx, object, prompt, media, params)
	defer llama.SamplerFree(sampler)
	defer func() {
		for _, b := range bitmaps {
			mtmd.BitmapFree(b)
		}
	}()

	// Check that we have not exceeded the context window.
	if inputTokens > m.cfg.ContextWindow {
		returnPrompt := ""
		if params.ReturnPrompt {
			returnPrompt = prompt
		}

		err := fmt.Errorf("input tokens %d exceed context window %d", inputTokens, m.cfg.ContextWindow)
		m.sendErrorResponse(ctx, ch, id, object, 1, returnPrompt, err, Usage{
			PromptTokens:     inputTokens,
			ReasoningTokens:  reasonTokens,
			CompletionTokens: completionTokens,
			OutputTokens:     outputTokens,
			TotalTokens:      inputTokens + outputTokens,
		})
		return
	}

	// -------------------------------------------------------------------------

	m.log(ctx, "process-chat-request", "status", "start processing loop")

	// Capture the time we start processing the request for a wall clock.
	start := time.Now()

	// We need to know if we are processing a standard or GPT model.
	isGTP := m.modelInfo.IsGPTModel

	// Create a processor to process the tokens.
	processor := newProcessor(m)

	// Track whether this is the first iteration. After prefill, the logits are
	// already computed so we sample directly without re-decoding the prompt.
	firstIteration := true

loop:
	for outputTokens <= params.MaxTokens {
		var err error
		var token llama.Token
		var resp response

		// Index is used to provide the index for each response.
		index++

		// ---------------------------------------------------------------------

		// Process a set of tokens based on the model class.
		switch {
		case firstIteration && isGTP:
			resp, token, err = processor.gptFirst(lctx, sampler, buf)
			firstIteration = false

			since := time.Since(ttftStart)
			metrics.AddTimeToFirstToken(since)
			span.SetAttributes(
				attribute.String("ttft", since.String()),
			)

		case firstIteration:
			resp, token, err = processor.standardFirst(lctx, sampler, buf)
			firstIteration = false

			since := time.Since(ttftStart)
			metrics.AddTimeToFirstToken(since)
			span.SetAttributes(
				attribute.String("ttft", since.String()),
			)

		case isGTP:
			resp, token, err = processor.gpt(lctx, batch, sampler, buf)

		default:
			resp, token, err = processor.standard(lctx, batch, sampler, buf)
		}

		// Did we get an error or are we at the end of the token stream.
		if err != nil {
			if errors.Is(err, io.EOF) {
				break loop
			}

			returnPrompt := ""
			if params.ReturnPrompt {
				returnPrompt = prompt
			}

			m.sendErrorResponse(ctx, ch, id, object, index, returnPrompt, err, Usage{
				PromptTokens:     inputTokens,
				ReasoningTokens:  reasonTokens,
				CompletionTokens: completionTokens,
				OutputTokens:     outputTokens,
				TotalTokens:      inputTokens + outputTokens,
			})
			return
		}

		// ---------------------------------------------------------------------

		// Set the flags so we know how to process the response.
		switch resp.status {
		case statusReasoning:
			reasonFlag++
			completionFlag = 0
			toolFlag = 0

		case statusCompletion:
			completionFlag++
			reasonFlag = 0
			toolFlag = 0

		case statusTooling:
			toolFlag++
			reasonFlag = 0
			completionFlag = 0

		default:
			batch = m.nextBatch(token)
			continue
		}

		// ---------------------------------------------------------------------

		// Capture the time it took to process these tokens and calculate
		// the tokens per second.

		elapsedSeconds := time.Since(start).Seconds()
		tokensPerSecond = float64(outputTokens) / elapsedSeconds

		// ---------------------------------------------------------------------

		// Do this if we are not processing tooling tokens.
		if toolFlag == 0 {
			// At the start or end of a mode we might have an extra CRLF we don't need.
			if m.isUnncessaryCRLF(reasonFlag, completionFlag, resp.content) {
				batch = m.nextBatch(token)
				continue
			}

			// We have reasoning or completion content to return to the client.
			err = m.sendDeltaResponse(ctx, ch, id, object, index, prompt, resp.content, reasonFlag,
				Usage{
					PromptTokens:     inputTokens,
					ReasoningTokens:  reasonTokens,
					CompletionTokens: completionTokens,
					OutputTokens:     outputTokens,
					TotalTokens:      inputTokens + outputTokens,
					TokensPerSecond:  tokensPerSecond,
				},
			)

			if err != nil {
				return
			}
		}

		// ---------------------------------------------------------------------

		// Store content for the final response.
		switch {
		case reasonFlag > 0:
			finalReasoning.WriteString(resp.content)

		case toolFlag > 0:
			finalTooling.WriteString(resp.content)

		default:
			finalContent.WriteString(resp.content)
		}

		// ---------------------------------------------------------------------

		// Get the next batch to process the next piece of content.
		batch = m.nextBatch(token)

		// ---------------------------------------------------------------------

		// Calculate token counts.
		switch {
		case reasonFlag > 0:
			reasonTokens += int(batch.NTokens)

		default:
			completionTokens += int(batch.NTokens)
		}

		outputTokens = reasonTokens + completionTokens
	}

	// -------------------------------------------------------------------------

	// If a tool call was provided, count tokens and process the tool call
	// response into the slice of ResponseToolCall.
	var respToolCalls []ResponseToolCall
	if toolFlag > 0 {
		content := finalTooling.String()
		content = strings.TrimSuffix(content, "\n")

		if len(content) > 0 {
			// We will count the tokens for the final JSON document
			// as completion tokens that would have been returned
			// if we didn't provide a structured response.
			tokens := llama.Tokenize(m.vocab, content, true, true)
			batch := llama.BatchGetOne(tokens)
			completionTokens += int(batch.NTokens)
			outputTokens = reasonTokens + completionTokens
		}

		switch isGTP {
		case true:
			respToolCalls = parseGPTToolCall(content)
		default:
			respToolCalls = parseToolCall(content)
		}
	}

	// -------------------------------------------------------------------------

	totalTokens := inputTokens + outputTokens

	// Add all this data to metrics and otel.
	span.SetAttributes(
		attribute.Int("prompt_tokens", inputTokens),
		attribute.Int("reasoning_tokens", reasonTokens),
		attribute.Int("completion_tokens", completionTokens),
		attribute.Int("output_tokens", outputTokens),
		attribute.Int("total_tokens", totalTokens),
		attribute.Float64("tokens_per_second", tokensPerSecond),
	)

	metrics.AddChatCompletionsUsage(inputTokens, reasonTokens, completionTokens, outputTokens, totalTokens, tokensPerSecond)

	// -------------------------------------------------------------------------

	// Send the final response that contains eveything we have sent plus
	// the final usage numbers.
	returnPrompt := ""
	if params.ReturnPrompt {
		returnPrompt = prompt
	}

	m.sendFinalResponse(ctx, ch, id, object, index, returnPrompt, &finalContent, &finalReasoning, respToolCalls,
		Usage{
			PromptTokens:     inputTokens,
			ReasoningTokens:  reasonTokens,
			CompletionTokens: completionTokens,
			OutputTokens:     outputTokens,
			TotalTokens:      totalTokens,
			TokensPerSecond:  tokensPerSecond,
		},
	)
}

// processInputTokens handles the prefill phase for both text and media requests.
// It tokenizes the prompt, processes any media attachments, and prepares the
// model's KV cache for autoregressive generation. Returns a sampler configured
// with the request parameters, an initial batch for generation, token counts,
// and any bitmaps that need to be freed by the caller.
func (m *Model) processInputTokens(ctx context.Context, lctx llama.Context, mtmdCtx mtmd.Context, object string, prompt string, media [][]byte, params params) (llama.Sampler, llama.Batch, int, int, []mtmd.Bitmap) {
	_, span := otel.AddSpan(ctx, "process-input-tokens")
	defer span.End()

	// Apply any parameters to this request like temperature or top_p.
	sampler := m.toSampler(params)

	// Tokenize the prompt to get the input token count.
	tokens := llama.Tokenize(m.vocab, prompt, true, true)
	inputTokens := len(tokens)

	var batch llama.Batch
	var outputTokens int
	var bitmaps []mtmd.Bitmap

	switch object {
	case ObjectChatMedia:
		// Convert raw media bytes into bitmap structures for the vision encoder.
		bitmaps = make([]mtmd.Bitmap, len(media))
		for i, med := range media {
			if len(med) == 0 {
				continue
			}
			bitmaps[i] = mtmd.BitmapInitFromBuf(mtmdCtx, &med[0], uint64(len(med)))
		}

		// Create input chunks that interleave text tokens with image embeddings.
		output := mtmd.InputChunksInit()
		defer mtmd.InputChunksFree(output)

		// Tokenize produces a sequence of chunks: text tokens and image patches.
		input := mtmd.NewInputText(prompt, true, true)
		mtmd.Tokenize(mtmdCtx, output, input, bitmaps)

		start := time.Now()

		// Prefill: Process all chunks through the model, populating the KV cache.
		// This handles both text token decoding and vision encoder forward passes.
		var n llama.Pos
		mtmd.HelperEvalChunks(mtmdCtx, lctx, output, 0, 0, int32(m.ctxParams.NBatch), true, &n)

		since := time.Since(start)
		metrics.AddPrefillMediaTime(since)
		span.SetAttributes(
			attribute.String("prefill-media", since.String()),
		)

		// Sample the first token to start autoregressive generation.
		batch = m.nextBatch(llama.SamplerSample(sampler, lctx, -1))
		outputTokens = int(batch.NTokens)

	default:
		// Text-only prefill phase:
		// - BatchGetOne: Wraps tokens into a batch structure for the model.
		// - Decode: Processes the batch, updating the model's internal KV cache
		//           with attention states.
		// - After prefill: Logits are ready for sampling in processChatRequest.

		nBatch := int(m.ctxParams.NBatch)
		start := time.Now()

		switch {
		case inputTokens <= nBatch:
			// Small prompt: process in a single batch.
			llama.Decode(lctx, llama.BatchGetOne(tokens))

		default:
			// Large prompt: chunk into multiple batches to avoid memory issues.
			for i := 0; i < len(tokens); i += nBatch {
				end := min(i+nBatch, len(tokens))
				chunk := tokens[i:end]
				llama.Decode(lctx, llama.BatchGetOne(chunk))
			}
		}

		since := time.Since(start)
		metrics.AddPrefillNonMediaTime(since)
		span.SetAttributes(
			attribute.String("prefill-nonmedia", since.String()),
		)
	}

	return sampler, batch, inputTokens, outputTokens, bitmaps
}

// nextBatch wraps a single sampled token into a batch for the next
// autoregressive decode step. Creates a 1-token batch from the sampled
// tokens which prepares us for next iteration.
func (m *Model) nextBatch(token llama.Token) llama.Batch {
	tokens := []llama.Token{token}
	return llama.BatchGetOne(tokens)
}

// batchResponse decodes the current batch into the model context, samples the
// next token, and returns its string representation. Returns io.EOF when an
// end-of-generation token is sampled.
func (m *Model) batchResponse(lctx llama.Context, batch llama.Batch, sampler llama.Sampler, buf []byte) (string, llama.Token, error) {
	llama.Decode(lctx, batch)
	return m.sampleToken(lctx, sampler, buf)
}

// sampleToken samples the next token from the current logits without decoding.
// Use this after prefill when logits are already computed.
func (m *Model) sampleToken(lctx llama.Context, sampler llama.Sampler, buf []byte) (string, llama.Token, error) {
	token := llama.SamplerSample(sampler, lctx, -1)

	if llama.VocabIsEOG(m.vocab, token) {
		return "", 0, io.EOF
	}

	l := llama.TokenToPiece(m.vocab, token, buf, 0, true)

	content := string(buf[:l])
	if content == "" {
		return "", 0, io.EOF
	}

	return content, token, nil
}

func (m *Model) isUnncessaryCRLF(reasonFlag int, completionFlag int, content string) bool {
	// We just started reasoning or tool calling so remove leading CR.
	if reasonFlag == 1 && content == "\x0A" {
		return true
	}

	// We just started completion so remove leading CR.
	if completionFlag == 1 && (content == "\x0A\x0A" || content == "\x0A") {
		return true
	}

	return false
}

func (m *Model) sendDeltaResponse(ctx context.Context, ch chan<- ChatResponse, id string, object string, index int, prompt string, content string, reasonFlag int, usage Usage) error {
	if index%500 == 0 {
		m.log(ctx, "chat-completion", "status", "delta", "id", id, "index", index, "object", object, "reasoning", reasonFlag, "content", len(content))
	}

	select {
	case <-ctx.Done():
		select {
		case ch <- ChatResponseErr(id, object, m.modelInfo.ID, index, prompt, ctx.Err(), usage):
		default:
		}

		return ctx.Err()

	case ch <- chatResponseDelta(id, object, m.modelInfo.ID, index, content, reasonFlag > 0, usage):
	}

	return nil
}

func (m *Model) sendFinalResponse(ctx context.Context, ch chan<- ChatResponse, id string, object string, index int, prompt string, finalContent *strings.Builder, finalReasoning *strings.Builder, respToolCalls []ResponseToolCall, usage Usage) {
	m.log(ctx, "chat-completion", "status", "final", "id", id, "index", index, "object", object, "tooling", len(respToolCalls) > 0, "reasoning", finalReasoning.Len(), "content", finalContent.Len())

	select {
	case <-ctx.Done():
		select {
		case ch <- ChatResponseErr(id, object, m.modelInfo.ID, index, prompt, ctx.Err(), usage):
		default:
		}

	case ch <- chatResponseFinal(id, object, m.modelInfo.ID, index, prompt,
		finalContent.String(),
		finalReasoning.String(),
		respToolCalls,
		usage):
	}

	contextTokens := usage.PromptTokens + usage.CompletionTokens
	contextWindow := m.cfg.ContextWindow
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	m.log(ctx, "chat-completion", "prompt", usage.PromptTokens, "output", usage.OutputTokens,
		"context", contextTokens, "down", fmt.Sprintf("(%.0f%% of %.0fK) TPS: %.2f", percentage, of, usage.TokensPerSecond))
}

func (m *Model) sendErrorResponse(ctx context.Context, ch chan<- ChatResponse, id string, object string, index int, prompt string, err error, usage Usage) {
	m.log(ctx, "chat-completion", "status", "ERROR", "msg", err, "id", id, "object", object, "index", index)

	select {
	case <-ctx.Done():

	case ch <- ChatResponseErr(id, object, m.modelInfo.ID, index, prompt,
		err,
		usage):

	default:
	}
}
