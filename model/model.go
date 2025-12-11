// Package model provides the low-level api for working with models.
package model

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

//go:embed jinja/*
var jinjaFS embed.FS

// Model represents a model and provides a low-level API for working with it.
type Model struct {
	cfg           Config
	model         llama.Model
	vocab         llama.Vocab
	ctxParams     llama.ContextParams
	template      string
	projFile      string
	modelInfo     ModelInfo
	activeStreams atomic.Int32
}

func NewModel(cfg Config) (*Model, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("new-model:unable to validate config: %w", err)
	}

	mparams := llama.ModelDefaultParams()
	if cfg.Device != "" {
		dev := llama.GGMLBackendDeviceByName(cfg.Device)
		if dev == 0 {
			return nil, fmt.Errorf("new-model:unknown device: %s", cfg.Device)
		}
		mparams.SetDevices([]llama.GGMLBackendDevice{dev})
	}

	mdl, err := llama.ModelLoadFromFile(cfg.ModelFile, mparams)
	if err != nil {
		return nil, fmt.Errorf("new-model:unable to load model: %w", err)
	}

	cfg = adjustConfig(cfg, mdl)
	vocab := llama.ModelGetVocab(mdl)

	// -------------------------------------------------------------------------

	modelInfo := toModelInfo(cfg, mdl)

	template, err := retrieveTemplate(cfg, mdl, modelInfo)
	if err != nil {
		return nil, fmt.Errorf("new-model:failed to retrieve model template: %w", err)
	}

	// -------------------------------------------------------------------------

	m := Model{
		cfg:       cfg,
		model:     mdl,
		vocab:     vocab,
		ctxParams: modelCtxParams(cfg, modelInfo),
		template:  template,
		projFile:  cfg.ProjectionFile,
		modelInfo: modelInfo,
	}

	return &m, nil
}

func retrieveTemplate(cfg Config, mdl llama.Model, modelInfo ModelInfo) (string, error) {
	var template string

	if cfg.JinjaFile != "" {
		data, err := readJinjaTemplate(cfg.JinjaFile)
		if err != nil {
			return "", fmt.Errorf("retrieve-template:failed to read jinja template: %w", err)
		}

		if data == "" {
			return "", fmt.Errorf("retrieve-template:jinja template is empty")
		}

		template = data
	}

	if template == "" {
		if modelInfo.IsGPTModel {
			data, err := jinjaFS.ReadFile("jinja/gpt-oss.jinja")
			if err != nil {
				return "", fmt.Errorf("retrieve-template:failed to read gpt-oss.jinja template: %w", err)
			}

			return string(data), nil
		}

		template = llama.ModelChatTemplate(mdl, "")
		if template == "" {
			template, _ = llama.ModelMetaValStr(mdl, "tokenizer.chat_template")
		}
	}

	return template, nil
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
			return fmt.Errorf("unload:cannot unload: %d active streams: %w", m.activeStreams.Load(), ctx.Err())

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
	var inputTokens int
	var completionTokens int
	var reasonTokens int
	var tokensPerSecond float64

	// These builders contain the final content for each of these items.
	var (
		finalReasoning strings.Builder
		finalContent   strings.Builder
		finalTooling   strings.Builder
	)

	// index is used to provide the index for each response.
	var index int

	// These flags track what mode the model is operating in.
	var (
		reasonFlag     int
		completionFlag int
		toolFlag       int
	)

	// The buffer is used to process tokens.
	const bufferSize = 32 * 1024
	buf := make([]byte, bufferSize)

	// -------------------------------------------------------------------------

	// Process the prompt and get the first batch for the response.
	sampler, batch, inputTokens, outputTokens := m.startProcessing(lctx, object, prompt, params)
	defer llama.SamplerFree(sampler)

	// Check that we have not exceeded the context window.
	if inputTokens > m.cfg.ContextWindow {
		err := fmt.Errorf("process-chat-request:input tokens %d exceed context window %d", inputTokens, m.cfg.ContextWindow)
		m.sendErrorResponse(ctx, ch, id, object, 0, prompt, err, Usage{
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
	now := time.Now()

	// We need to know if we are processing a standard or GPT model.
	isGTP := m.modelInfo.IsGPTModel

	// Create a processor to process the tokens.
	processor := newProcessor(m)

loop:
	for outputTokens <= params.MaxTokens {
		var err error
		var token llama.Token
		var resp response

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
			reasonFlag = 0
			completionFlag++
			toolFlag = 0

		case statusTooling:
			reasonFlag = 0
			completionFlag = 0
			toolFlag++

		default:
			batch = m.nextBatch(token)
			continue
		}

		// ---------------------------------------------------------------------

		// Capture the time it took to process these tokens and calculate
		// the tokens per second.

		elapsedSeconds := time.Since(now).Seconds()
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

	// Send the final response that contains eveything we have sent plus
	// the final usage numbers.
	m.sendFinalResponse(ctx, ch, id, object, index, prompt, &finalContent, &finalReasoning, respToolCalls,
		Usage{
			PromptTokens:     inputTokens,
			ReasoningTokens:  reasonTokens,
			CompletionTokens: completionTokens,
			OutputTokens:     outputTokens,
			TotalTokens:      inputTokens + outputTokens,
			TokensPerSecond:  tokensPerSecond,
		},
	)
}

func (m *Model) startProcessing(lctx llama.Context, object string, prompt string, params Params) (llama.Sampler, llama.Batch, int, int) {
	// Apply any parameters to this request like temperature or top_p.
	sampler := toSampler(params)

	// Process the prompt and get the number of tokens plus the initial batch
	// for the model response. If this is a vision call, we are just doing this
	// for the input token count and the batch will be ignored.

	tokens := llama.Tokenize(m.vocab, prompt, true, true)
	batch := llama.BatchGetOne(tokens)
	inputTokens := int(batch.NTokens)

	// If this is a chat with media, then input processing has already happened
	// using the mtmd package. This will provide the initial batch for the
	// model response.

	var outputTokens int
	if object == ObjectChatMedia {
		batch = m.nextBatch(llama.SamplerSample(sampler, lctx, -1))
		outputTokens = int(batch.NTokens)
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

	l := llama.TokenToPiece(m.vocab, token, buf, 0, false)

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
}

func (m *Model) sendErrorResponse(ctx context.Context, ch chan<- ChatResponse, id string, object string, index int, prompt string, err error, usage Usage) {
	select {
	case <-ctx.Done():

	case ch <- ChatResponseErr(id, object, m.modelInfo.ID, index, prompt,
		err,
		usage):

	default:
	}
}
