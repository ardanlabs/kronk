// Package model provides the low-level api for working with models.
package model

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ardanlabs/kronk/sdk/observ/metrics"
	"github.com/hybridgroup/yzma/pkg/llama"
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
	template      Template
	projFile      string
	modelInfo     ModelInfo
	activeStreams atomic.Int32
}

func NewModel(tmlpRetriever TemplateRetriever, cfg Config) (*Model, error) {
	l := cfg.Log
	if cfg.Log == nil {
		l = func(ctx context.Context, msg string, args ...any) {}
	}

	if tmlpRetriever == nil {
		return nil, fmt.Errorf("templater required, use templater.New()")
	}

	if err := validateConfig(cfg, l); err != nil {
		return nil, fmt.Errorf("new-model: unable to validate config: %w", err)
	}

	mparams := llama.ModelDefaultParams()
	if cfg.Device != "" {
		dev := llama.GGMLBackendDeviceByName(cfg.Device)
		if dev == 0 {
			return nil, fmt.Errorf("new-model: unknown device: %s", cfg.Device)
		}
		mparams.SetDevices([]llama.GGMLBackendDevice{dev})
	}

	// OTEL: WANT TO KNOW HOW LONG THESE FUNCTION CALLS TAKES
	start := time.Now()

	var err error
	var mdl llama.Model
	switch len(cfg.ModelFiles) {
	case 1:
		mdl, err = llama.ModelLoadFromFile(cfg.ModelFiles[0], mparams)
		if err != nil {
			return nil, fmt.Errorf("new-model: unable to load model: %w", err)
		}

	default:
		mdl, err = llama.ModelLoadFromSplits(cfg.ModelFiles, mparams)
		if err != nil {
			return nil, fmt.Errorf("new-model: unable to load model from split: %w", err)
		}
	}

	metrics.AddModelFileLoadTime(time.Since(start))

	cfg = adjustConfig(cfg, mdl)
	vocab := llama.ModelGetVocab(mdl)

	// -------------------------------------------------------------------------

	modelInfo := toModelInfo(cfg, mdl)

	template, err := retrieveTemplate(tmlpRetriever, cfg, mdl, modelInfo)
	if err != nil {
		return nil, fmt.Errorf("new-model: failed to retrieve model template: %w", err)
	}

	modelInfo.Template = template

	// -------------------------------------------------------------------------

	m := Model{
		cfg:       cfg,
		log:       l,
		model:     mdl,
		vocab:     vocab,
		ctxParams: modelCtxParams(cfg, modelInfo),
		template:  template,
		projFile:  cfg.ProjFile,
		modelInfo: modelInfo,
	}

	return &m, nil
}

func retrieveTemplate(tmlpRetriever TemplateRetriever, cfg Config, mdl llama.Model, modelInfo ModelInfo) (Template, error) {
	if cfg.JinjaFile != "" {
		data, err := readJinjaTemplate(cfg.JinjaFile)
		if err != nil {
			return Template{}, fmt.Errorf("retrieve-template: failed to read jinja template: %w", err)
		}

		if data == "" {
			return Template{}, fmt.Errorf("retrieve-template: jinja template is empty")
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
	if _, exists := ctx.Deadline(); !exists {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}

	for m.activeStreams.Load() > 0 {
		select {
		case <-ctx.Done():
			return fmt.Errorf("unload: cannot unload %d active streams: %w", m.activeStreams.Load(), ctx.Err())

		case <-time.After(100 * time.Millisecond):
		}
	}

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

func (m *Model) processChatRequest(ctx context.Context, id string, lctx llama.Context, object string, prompt string, params Params, ch chan<- ChatResponse) {
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

	// Process the prompt and get the first batch for the response.
	sampler, batch, inputTokens, outputTokens := m.startProcessing(lctx, object, prompt, params)
	defer llama.SamplerFree(sampler)

	// Check that we have not exceeded the context window.
	if inputTokens > m.cfg.ContextWindow {
		err := fmt.Errorf("process-chat-request: input tokens %d exceed context window %d", inputTokens, m.cfg.ContextWindow)
		m.sendErrorResponse(ctx, ch, id, object, 1, prompt, err, Usage{
			PromptTokens:     inputTokens,
			ReasoningTokens:  reasonTokens,
			CompletionTokens: completionTokens,
			OutputTokens:     outputTokens,
			TotalTokens:      inputTokens + outputTokens,
		})
		return
	}

	// -------------------------------------------------------------------------

	// Capture the time we start processing the request for a wall clock.
	start := time.Now()

	// We need to know if we are processing a standard or GPT model.
	isGTP := m.modelInfo.IsGPTModel

	// Create a processor to process the tokens.
	processor := newProcessor(m)

loop:
	for outputTokens <= params.MaxTokens {
		var err error
		var token llama.Token
		var resp response

		// Index is used to provide the index for each response.
		index++

		// ---------------------------------------------------------------------

		// Process a set of tokens based on the model class.
		switch isGTP {
		case true:
			resp, token, err = processor.gpt(lctx, batch, sampler, buf)

		default:
			resp, token, err = processor.standard(lctx, batch, sampler, buf)
		}

		// Did we get an error or are we at the end of the token stream.
		if err != nil {
			if errors.Is(err, io.EOF) {
				break loop
			}

			m.sendErrorResponse(ctx, ch, id, object, index, prompt, err, Usage{
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

	// OTEL: ADD DATA TO OTEL SPAN

	totalTokens := inputTokens + outputTokens
	metrics.AddChatCompletionsUsage(inputTokens, reasonTokens, completionTokens, outputTokens, totalTokens, tokensPerSecond)

	// Send the final response that contains eveything we have sent plus
	// the final usage numbers.
	m.sendFinalResponse(ctx, ch, id, object, index, prompt, &finalContent, &finalReasoning, respToolCalls,
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

func (m *Model) startProcessing(lctx llama.Context, object string, prompt string, params Params) (llama.Sampler, llama.Batch, int, int) {
	// Apply any parameters to this request like temperature or top_p.
	sampler := toSampler(params)

	// Process the prompt and get the number of tokens plus the initial batch
	// for the model response. If this is a media call, we are just doing this
	// for the input token count and the batch will be ignored.

	// OTEL: WANT TO KNOW HOW LONG THESE FUNCTION CALLS TAKES
	start := time.Now()

	tokens := llama.Tokenize(m.vocab, prompt, true, true)

	if object != ObjectChatMedia {
		metrics.AddPrefillNonMediaTime(time.Since(start))
	}

	batch := llama.BatchGetOne(tokens)
	inputTokens := int(batch.NTokens)

	if object != ObjectChatMedia {
		metrics.AddTimeToFirstToken(time.Since(start))
	}

	// If this is a chat with media, then input processing has already happened
	// using the mtmd package. This will provide the initial batch for the
	// model response.

	var outputTokens int
	if object == ObjectChatMedia {

		// OTEL: WANT TO KNOW HOW LONG THESE FUNCTION CALLS TAKES
		start := time.Now()

		batch = m.nextBatch(llama.SamplerSample(sampler, lctx, -1))
		outputTokens = int(batch.NTokens)

		metrics.AddTimeToFirstToken(time.Since(start))
	}

	return sampler, batch, inputTokens, outputTokens
}

func (m *Model) nextBatch(token llama.Token) llama.Batch {
	tokens := []llama.Token{token}
	return llama.BatchGetOne(tokens)
}

func (m *Model) batchResponse(lctx llama.Context, batch llama.Batch, sampler llama.Sampler, buf []byte) (string, llama.Token, error) {
	llama.Decode(lctx, batch)
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
	if index%100 == 0 {
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
