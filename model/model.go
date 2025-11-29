// Package model provides the low-level api for working with models.
package model

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// Model represents a model and provides a low-level API for working with it.
type Model struct {
	cfg       Config
	model     llama.Model
	vocab     llama.Vocab
	ctxParams llama.ContextParams
	template  string
	projFile  string
	modelInfo ModelInfo
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

	template, err := retrieveTemplate(cfg, mdl)
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
		modelInfo: newModelInfo(cfg, mdl),
	}

	return &m, nil
}

func retrieveTemplate(cfg Config, mdl llama.Model) (string, error) {
	var template string

	if cfg.JinjaFile != "" {
		template, err := readJinjaTemplate(cfg.JinjaFile)
		if err != nil {
			return "", fmt.Errorf("failed to read jinja template: %w", err)
		}

		if template == "" {
			return "", fmt.Errorf("jinja template is empty")
		}
	}

	if template == "" {
		template = llama.ModelChatTemplate(mdl, "")
		if template == "" {
			template, _ = llama.ModelMetaValStr(mdl, "tokenizer.chat_template")
		}
	}

	return template, nil
}

func (m *Model) Unload() {
	llama.ModelFree(m.model)
	llama.BackendFree()
}

func (m *Model) Config() Config {
	return m.cfg
}

func (m *Model) ModelInfo() ModelInfo {
	return m.modelInfo
}

func (m *Model) processTokens(ctx context.Context, id string, lctx llama.Context, object string, prompt string, params Params, ch chan<- ChatResponse) {
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
		reasonFlag int
		outputFlag int
		toolFlag   int
	)

	// The buffer is used to process tokens.
	const bufferSize = 32 * 1024
	buf := make([]byte, bufferSize)

	// -------------------------------------------------------------------------

	// Adjust the parameters for defaults that need to be applied.
	params = adjustParams(params)

	// Process the prompt and get the first batch for the response.
	sampler, batch, inputTokens, outputTokens := m.startProcessing(lctx, object, prompt, params)

	// -------------------------------------------------------------------------

	// Capture the time we start processing the request for a wall clock.
	now := time.Now()

loop:
	for outputTokens <= params.MaxTokens {
		index++

		// For the given batch, extract the response.
		content, token, err := m.batchResponse(lctx, batch, sampler, buf)
		if err != nil {
			break loop
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
			batch = m.toolCallStart(token, &toolFlag, &completionTokens)
			continue

		case "</tool_call>":
			batch = m.toolCallStop(token, &toolFlag, &completionTokens)
			continue

		case "<|channel|>":
			batch, content, err = m.channelStart(lctx, token, sampler, buf, &reasonFlag, &reasonTokens, &completionTokens)
			if err != nil {
				break loop
			}

			if content == "<|continue|>" {
				continue
			}

		case "<|end|>":
			batch, err = m.channelEnd(lctx, token, sampler, buf)
			if err != nil {
				break loop
			}
			continue
		}

		// ---------------------------------------------------------------------

		// At the start or end of a mode we might have an extra CRLF we
		// don't need.
		if m.isUnncessaryCRLF(reasonFlag, toolFlag, outputFlag, content) {
			batch = m.nextBatch(token)
			continue
		}

		// Capture the time it took to process these tokens and calculate
		// the tokens per second.
		elapsedSeconds := time.Since(now).Seconds()
		tokensPerSecond = float64(outputTokens) / elapsedSeconds

		// We want to return the tool calling in a single response to make
		// it easier for developers to process. We expect the model to stop
		// processing tokens once the tool call is complete.
		if toolFlag > 0 {
			finalTooling.WriteString(content)

			batch = m.nextBatch(token)
			completionTokens += int(batch.NTokens)
			outputTokens = reasonTokens + completionTokens

			continue
		}

		// ---------------------------------------------------------------------
		// We have reasoning or completion content to return to the client.

		select {
		case <-ctx.Done():
			select {
			case ch <- ChatResponseErr(id, object, m.modelInfo.Name, index, ctx.Err(), Usage{
				InputTokens:      inputTokens,
				ReasoningTokens:  reasonTokens,
				CompletionTokens: completionTokens,
				OutputTokens:     outputTokens,
				TokensPerSecond:  tokensPerSecond}):
			default:
			}

			return

		case ch <- chatResponseDelta(id, object, m.modelInfo.Name, index, content, reasonFlag > 0, Usage{
			InputTokens:      inputTokens,
			ReasoningTokens:  reasonTokens,
			CompletionTokens: completionTokens,
			OutputTokens:     outputTokens,
			TokensPerSecond:  tokensPerSecond}):
		}

		// ---------------------------------------------------------------------
		// Store the content for the final response.

		switch {
		case reasonFlag > 0:
			finalReasoning.WriteString(content)
		default:
			finalContent.WriteString(content)
		}

		// ---------------------------------------------------------------------
		// Get the next batch to process the next piece of content.

		batch = m.nextBatch(token)

		switch {
		case reasonFlag > 0:
			reasonTokens += int(batch.NTokens)
			reasonFlag++

		default:
			completionTokens += int(batch.NTokens)
			outputFlag++
		}

		outputTokens = reasonTokens + completionTokens
	}

	// -------------------------------------------------------------------------

	// Parse the tool call response to provide structured data.
	var respToolCall ResponseToolCall
	if finalTooling.Len() > 0 {
		respToolCall = parseToolCall(finalTooling)
	}

	// Send the final response that contains eveything we have sent plus
	// the final usage numbers.
	select {
	case <-ctx.Done():
		select {
		case ch <- ChatResponseErr(id, object, m.modelInfo.Name, index, ctx.Err(), Usage{
			InputTokens:      inputTokens,
			ReasoningTokens:  reasonTokens,
			CompletionTokens: completionTokens,
			OutputTokens:     outputTokens,
			TokensPerSecond:  tokensPerSecond}):
		default:
		}

	case ch <- chatResponseFinal(
		id,
		object,
		m.modelInfo.Name,
		index,
		finalContent.String(),
		finalReasoning.String(),
		respToolCall,
		Usage{
			InputTokens:      inputTokens,
			ReasoningTokens:  reasonTokens,
			CompletionTokens: completionTokens,
			OutputTokens:     reasonTokens + completionTokens,
			TokensPerSecond:  tokensPerSecond}):
	}
}

func (m *Model) startProcessing(lctx llama.Context, object string, prompt string, params Params) (llama.Sampler, llama.Batch, int, int) {
	sampler := toSampler(params)

	// Process the prompt and get the number of tokens plus the initial batch
	// for the model response. If this is a vision call, we are just doing this
	// for the input token count and the batch will be ignored.

	tokens := llama.Tokenize(m.vocab, prompt, true, true)
	batch := llama.BatchGetOne(tokens)
	inputTokens := int(batch.NTokens)

	// If this is a vision call, then input processing has already happened
	// using the mtmd package. This will provide the initial batch for the
	// model response.

	var outputTokens int
	if object == ObjectVision {
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

func (m *Model) thinkStart(token llama.Token, reasonFlag *int, reasonTokens *int) llama.Batch {
	*reasonFlag = 1

	batch := m.nextBatch(token)
	*reasonTokens += int(batch.NTokens)

	return batch
}

func (m *Model) thinkStop(token llama.Token, reasonFlag *int, completionTokens *int) llama.Batch {
	*reasonFlag = 0

	batch := m.nextBatch(token)
	*completionTokens += int(batch.NTokens)

	return batch
}

func (m *Model) toolCallStart(token llama.Token, toolFlag *int, completionTokens *int) llama.Batch {
	*toolFlag = 1

	batch := m.nextBatch(token)
	*completionTokens += int(batch.NTokens)

	return batch
}

func (m *Model) toolCallStop(token llama.Token, toolFlag *int, completionTokens *int) llama.Batch {
	*toolFlag = 0

	batch := m.nextBatch(token)
	*completionTokens += int(batch.NTokens)

	return batch
}

func (m *Model) channelStart(lctx llama.Context, token llama.Token, sampler llama.Sampler, buf []byte, reasoning *int, reasonTokens *int, completionTokens *int) (llama.Batch, string, error) {
	// <|channel|>analysis<|message|>REASONING<|end|><|start|>assistant<|channel|>final<|message|>RESPONSE

	batch := m.nextBatch(token)
	content, token, err := m.batchResponse(lctx, batch, sampler, buf)
	if err != nil {
		return batch, "", err
	}

	switch content {
	case "analysis":
		batch, err = m.channelAnalysis(lctx, token, sampler, buf, reasoning, reasonTokens)
		if err != nil {
			return batch, "", err
		}

	case "final":
		batch, err = m.channelFinal(lctx, token, sampler, buf, reasoning, completionTokens)
		if err != nil {
			return batch, "", err
		}

	default:
		return batch, content, nil
	}

	return batch, "<|continue|>", nil
}

func (m *Model) channelAnalysis(lctx llama.Context, token llama.Token, sampler llama.Sampler, buf []byte, reasoning *int, reasonTokens *int) (llama.Batch, error) {
	*reasoning = 1

	batch := m.nextBatch(token)
	_, token, err := m.batchResponse(lctx, batch, sampler, buf) // <|message|>
	if err != nil {
		return batch, err
	}

	batch = m.nextBatch(token)
	*reasonTokens += int(batch.NTokens)

	return batch, nil
}

func (m *Model) channelFinal(lctx llama.Context, token llama.Token, sampler llama.Sampler, buf []byte, reasoning *int, completionTokens *int) (llama.Batch, error) {
	*reasoning = 0

	batch := m.nextBatch(token)
	_, token, err := m.batchResponse(lctx, batch, sampler, buf) // <|message|>
	if err != nil {
		return batch, err
	}

	batch = m.nextBatch(token)
	*completionTokens += int(batch.NTokens)

	return batch, nil
}

func (m *Model) channelEnd(lctx llama.Context, token llama.Token, sampler llama.Sampler, buf []byte) (llama.Batch, error) {
	batch := m.nextBatch(token)

	_, token, err := m.batchResponse(lctx, batch, sampler, buf) // <|start|>
	if err != nil {
		return batch, err
	}

	batch = m.nextBatch(token)

	_, token, err = m.batchResponse(lctx, batch, sampler, buf) // assistant
	if err != nil {
		return batch, err
	}

	batch = m.nextBatch(token)
	return batch, nil
}

func (m *Model) isUnncessaryCRLF(reasoning int, tooling int, completion int, content string) bool {
	// We just started reasoning or tool calling so remove leading CR.
	if (reasoning == 1 || tooling == 1) && content == "\x0A" {
		return true
	}

	// We just started completion so remove leading CR.
	if reasoning == 0 && tooling == 0 && completion == 0 && (content == "\x0A\x0A" || content == "\x0A") {
		return true
	}

	return false
}

// =============================================================================

func parseToolCall(tooling strings.Builder) ResponseToolCall {
	// The idea is to add a unique ID to the tool call. The user
	// can use this ID to reference the tool call in the future.

	var toolCall ResponseToolCall
	if err := json.Unmarshal([]byte(tooling.String()), &toolCall); err != nil {
		return ResponseToolCall{}
	}

	toolCall.ID = uuid.NewString()

	return toolCall
}
