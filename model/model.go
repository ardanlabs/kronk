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
		return nil, fmt.Errorf("unable to validate config: %w", err)
	}

	mparams := llama.ModelDefaultParams()
	if cfg.Device != "" {
		dev := llama.GGMLBackendDeviceByName(cfg.Device)
		if dev == 0 {
			return nil, fmt.Errorf("unknown device: %s", cfg.Device)
		}
		mparams.SetDevices([]llama.GGMLBackendDevice{dev})
	}

	mdl, err := llama.ModelLoadFromFile(cfg.ModelFile, mparams)
	if err != nil {
		return nil, fmt.Errorf("unable to load model: %w", err)
	}

	cfg = adjustConfig(cfg, mdl)
	vocab := llama.ModelGetVocab(mdl)

	// -------------------------------------------------------------------------

	modelInfo := newModelInfo(cfg, mdl)

	template, err := retrieveTemplate(cfg, mdl, modelInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve model template: %w", err)
	}

	// -------------------------------------------------------------------------

	m := Model{
		cfg:       cfg,
		model:     mdl,
		vocab:     vocab,
		ctxParams: modelCtxParams(cfg),
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
			return "", fmt.Errorf("failed to read jinja template: %w", err)
		}

		if data == "" {
			return "", fmt.Errorf("jinja template is empty")
		}

		template = data
	}

	if template == "" {
		if modelInfo.IsGPT {
			data, err := jinjaFS.ReadFile("jinja/gpt-oss.jinja")
			if err != nil {
				return "", fmt.Errorf("failed to read gpt-oss.jinja template: %w", err)
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
			return fmt.Errorf("cannot unload: %d active streams: %w", m.activeStreams.Load(), ctx.Err())

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
		err := fmt.Errorf("input tokens %d exceed context window %d", inputTokens, m.cfg.ContextWindow)
		m.sendErrorResponse(ctx, ch, id, object, 0, prompt, err, Usage{
			InputTokens:      inputTokens,
			ReasoningTokens:  reasonTokens,
			CompletionTokens: completionTokens,
			OutputTokens:     outputTokens,
		})
		return
	}

	// -------------------------------------------------------------------------

	// Capture the time we start processing the request for a wall clock.
	now := time.Now()

loop:
	for outputTokens <= params.MaxTokens {
		index++

		// For the given batch, extract the response.
		content, token, err := m.batchResponse(lctx, batch, sampler, buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break loop
			}

			m.sendErrorResponse(ctx, ch, id, object, index, prompt, err, Usage{
				InputTokens:      inputTokens,
				ReasoningTokens:  reasonTokens,
				CompletionTokens: completionTokens,
				OutputTokens:     outputTokens,
			})
			return
		}

		// ---------------------------------------------------------------------
		// Look for special tags that we will parse out of the response.

		switch content {
		case "<think>":
			batch = m.thinkStart(token, &reasonFlag, &reasonTokens)
			continue

		case "</think>":
			batch = m.thinkStop(token, &reasonFlag, &completionTokens)
			continue

		case "<tool_call>":
			content, err = m.toolCall(lctx, token, sampler, buf)
			if err != nil {
				m.sendErrorResponse(ctx, ch, id, object, index, prompt, err, Usage{
					InputTokens:      inputTokens,
					ReasoningTokens:  reasonTokens,
					CompletionTokens: completionTokens,
					OutputTokens:     outputTokens,
				})
				return
			}

			toolFlag = 1
			finalTooling.WriteString(content)
			break loop

		case "<|channel|>":
			batch, content, err = m.gptChannel(lctx, token, sampler, buf)
			if err != nil {
				m.sendErrorResponse(ctx, ch, id, object, index, prompt, err, Usage{
					InputTokens:      inputTokens,
					ReasoningTokens:  reasonTokens,
					CompletionTokens: completionTokens,
					OutputTokens:     outputTokens,
				})
				return
			}

			switch {
			case content == "<|reasoning|>":
				reasonFlag = 1
				continue

			case content == "<|completion|>":
				reasonFlag = 0
				continue

			case content[:13] == "<|tool_call|>":
				toolFlag = 1
				finalTooling.WriteString(content[13:])
				break loop
			}

		case "<|end|>":
			batch, err = m.gptEnd(lctx, token, sampler, buf)
			if err != nil {
				m.sendErrorResponse(ctx, ch, id, object, index, prompt, err, Usage{
					InputTokens:      inputTokens,
					ReasoningTokens:  reasonTokens,
					CompletionTokens: completionTokens,
					OutputTokens:     outputTokens,
				})
				return
			}
			continue
		}

		// ---------------------------------------------------------------------

		// At the start or end of a mode we might have an extra CRLF we
		// don't need.
		if m.isUnncessaryCRLF(reasonFlag, completionFlag, content) {
			batch = m.nextBatch(token)
			continue
		}

		// Capture the time it took to process these tokens and calculate
		// the tokens per second.
		elapsedSeconds := time.Since(now).Seconds()
		tokensPerSecond = float64(outputTokens) / elapsedSeconds

		// ---------------------------------------------------------------------
		// We have reasoning or completion content to return to the client and
		// store for the final response.

		err = m.sendDeltaResponse(ctx, ch, id, object, index, prompt, content, reasonFlag,
			Usage{
				InputTokens:      inputTokens,
				ReasoningTokens:  reasonTokens,
				CompletionTokens: completionTokens,
				OutputTokens:     outputTokens,
				TokensPerSecond:  tokensPerSecond,
			},
		)

		if err != nil {
			return
		}

		m.storeFinalContent(&finalReasoning, &finalContent, content, reasonFlag)

		// ---------------------------------------------------------------------
		// Get the next batch to process the next piece of content.

		batch = m.nextBatch(token)

		switch {
		case reasonFlag > 0:
			reasonTokens += int(batch.NTokens)
			reasonFlag++

		default:
			completionTokens += int(batch.NTokens)
			completionFlag++
		}

		outputTokens = reasonTokens + completionTokens
	}

	// -------------------------------------------------------------------------

	// Parse the tool call response to structured data.
	var respToolCall ResponseToolCall
	if toolFlag == 1 {
		content := finalTooling.String()
		content = strings.Trim(content, "\n")

		if len(content) > 0 {
			// We will count the tokens for the final JSON document
			// as completion tokens that would have been returned
			// if we didn't provide a structured response.
			tokens := llama.Tokenize(m.vocab, content, true, true)
			batch := llama.BatchGetOne(tokens)
			completionTokens += int(batch.NTokens)
			outputTokens = reasonTokens + completionTokens
		}

		respToolCall = parseToolCall(content)
	}

	// Send the final response that contains eveything we have sent plus
	// the final usage numbers.
	m.sendFinalResponse(ctx, ch, id, object, index, prompt, &finalContent, &finalReasoning, respToolCall,
		Usage{
			InputTokens:      inputTokens,
			ReasoningTokens:  reasonTokens,
			CompletionTokens: completionTokens,
			OutputTokens:     outputTokens,
			TokensPerSecond:  tokensPerSecond,
		},
	)
}

func (m *Model) startProcessing(lctx llama.Context, object string, prompt string, params Params) (llama.Sampler, llama.Batch, int, int) {
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
	if reasonFlag == 0 && completionFlag == 0 && (content == "\x0A\x0A" || content == "\x0A") {
		return true
	}

	return false
}

func (m *Model) storeFinalContent(finalReasoning *strings.Builder, finalContent *strings.Builder, content string, reasonFlag int) {
	switch {
	case reasonFlag > 0:
		finalReasoning.WriteString(content)
	default:
		finalContent.WriteString(content)
	}
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

func (m *Model) sendFinalResponse(ctx context.Context, ch chan<- ChatResponse, id string, object string, index int, prompt string, finalContent *strings.Builder, finalReasoning *strings.Builder, respToolCall ResponseToolCall, usage Usage) {
	select {
	case <-ctx.Done():
		select {
		case ch <- ChatResponseErr(id, object, m.modelInfo.ID, index, prompt, ctx.Err(), usage):
		default:
		}

	case ch <- chatResponseFinal(id, object, m.modelInfo.ID, index, prompt,
		finalContent.String(),
		finalReasoning.String(),
		respToolCall,
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
