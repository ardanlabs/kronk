// Package model provides the low-level api for working with models.
package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/hybridgroup/yzma/pkg/llama"
)

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
		var err error
		template, err = readJinjaTemplate(cfg.JinjaFile)
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
			if errors.Is(err, io.EOF) {
				break loop
			}

			m.sendErrorResponse(ctx, ch, id, object, index, err, Usage{})
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
				m.sendErrorResponse(ctx, ch, id, object, index, err, Usage{})
				return
			}

			toolFlag = 1

			cTokens := llama.Tokenize(m.vocab, content, true, true)
			cBatch := llama.BatchGetOne(cTokens)
			completionTokens += int(cBatch.NTokens)
			outputTokens = reasonTokens + completionTokens

			finalTooling.WriteString(content)
			break loop

		case "<|channel|>":
			batch, content, err = m.channelStart(lctx, token, sampler, buf)
			if err != nil {
				m.sendErrorResponse(ctx, ch, id, object, index, err, Usage{})
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
				toolContent := content[13:]

				cTokens := llama.Tokenize(m.vocab, toolContent, true, true)
				cBatch := llama.BatchGetOne(cTokens)
				completionTokens += int(cBatch.NTokens)
				outputTokens = reasonTokens + completionTokens

				finalTooling.WriteString(toolContent)
				break loop
			}

		case "<|end|>":
			batch, err = m.channelEnd(lctx, token, sampler, buf)
			if err != nil {
				m.sendErrorResponse(ctx, ch, id, object, index, err, Usage{})
				return
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

		// ---------------------------------------------------------------------
		// We have reasoning or completion content to return to the client and
		// store for the final response.

		err = m.sendDeltaResponse(ctx, ch, id, object, index, content, reasonFlag,
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
			outputFlag++
		}

		outputTokens = reasonTokens + completionTokens
	}

	// -------------------------------------------------------------------------

	// Parse the tool call response to provide structured data.
	var respToolCall ResponseToolCall
	if toolFlag == 1 && finalTooling.Len() > 0 {
		respToolCall = parseToolCall(finalTooling)
	}

	// Send the final response that contains eveything we have sent plus
	// the final usage numbers.
	m.sendFinalResponse(ctx, ch, id, object, index, &finalContent, &finalReasoning, respToolCall,
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

func (m *Model) toolCall(lctx llama.Context, token llama.Token, sampler llama.Sampler, buf []byte) (string, error) {
	var batch llama.Batch
	var content string
	var err error
	var data strings.Builder

	// Collect the content up to the location of </tool_call>.
	for {
		batch = m.nextBatch(token)
		content, token, err = m.batchResponse(lctx, batch, sampler, buf)
		if err != nil {
			return "", err
		}

		if content == "</tool_call>" {
			break
		}

		data.WriteString(content)
	}

	return data.String(), nil
}

func (m *Model) channelStart(lctx llama.Context, token llama.Token, sampler llama.Sampler, buf []byte) (llama.Batch, string, error) {
	// <|channel|>analysis<|message|>REASONING<|end|><|start|>assistant<|channel|>final<|message|>RESPONSE
	// <|channel|>analysis<|message|>REASONING<|end|><|start|>assistant<|channel|>commentary to=functions.get_weather <|constrain|>json<|message|>{"location":"NYC"}

	var batch llama.Batch
	var content string
	var err error
	var data strings.Builder

	// Collect the content up to the location of <|message|>.
	for {
		batch = m.nextBatch(token)
		content, token, err = m.batchResponse(lctx, batch, sampler, buf)
		if err != nil {
			return batch, "<|error|>", err
		}

		if content == "<|message|>" {
			batch = m.nextBatch(token)
			break
		}

		data.WriteString(content)
	}

	msg := data.String()

	switch {
	case msg == "analysis":
		return batch, "<|reasoning|>", nil

	case msg == "final":
		return batch, "<|completion|>", nil

	case len(msg) > 10 && msg[:10] == "commentary":
		toolCall, err := m.channelToolCall(msg, batch, lctx, sampler, buf)
		if err != nil {
			return llama.Batch{}, "<|error|>", err
		}
		return llama.Batch{}, fmt.Sprintf("<|tool_call|>%s", toolCall), nil

	default:
		batch = m.nextBatch(token)
		return batch, "<|error|>", fmt.Errorf("unknown channel type: %s", msg)
	}
}

func (m *Model) channelToolCall(msg string, batch llama.Batch, lctx llama.Context, sampler llama.Sampler, buf []byte) (string, error) {
	var args strings.Builder

	for {
		v, token, err := m.batchResponse(lctx, batch, sampler, buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", err
		}

		args.WriteString(v)

		batch = m.nextBatch(token)
	}

	// msg : commentary to=functions.get_weather <|constrain|>json
	// args: {"location":"NYC"}

	arguments := make(map[string]any)
	json.Unmarshal([]byte(args.String()), &arguments)

	rtc := struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments,omitempty"`
	}{
		Name:      extractFunctionName(msg),
		Arguments: arguments,
	}

	data, err := json.Marshal(rtc)
	if err != nil {
		return "", fmt.Errorf("failed to marshal tool call: %w", err)
	}

	return string(data), nil
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

func (m *Model) storeFinalContent(finalReasoning *strings.Builder, finalContent *strings.Builder, content string, reasonFlag int) {
	switch {
	case reasonFlag > 0:
		finalReasoning.WriteString(content)
	default:
		finalContent.WriteString(content)
	}
}

func (m *Model) sendDeltaResponse(ctx context.Context, ch chan<- ChatResponse, id string, object string, index int, content string, reasonFlag int, usage Usage) error {
	select {
	case <-ctx.Done():
		select {
		case ch <- ChatResponseErr(id, object, m.modelInfo.Name, index, ctx.Err(), usage):
		default:
		}

		return ctx.Err()

	case ch <- chatResponseDelta(id, object, m.modelInfo.Name, index, content, reasonFlag > 0, usage):
	}

	return nil
}

func (m *Model) sendFinalResponse(ctx context.Context, ch chan<- ChatResponse, id string, object string, index int, finalContent *strings.Builder, finalReasoning *strings.Builder, respToolCall ResponseToolCall, usage Usage) {
	select {
	case <-ctx.Done():
		select {
		case ch <- ChatResponseErr(id, object, m.modelInfo.Name, index, ctx.Err(), usage):
		default:
		}

	case ch <- chatResponseFinal(id, object, m.modelInfo.Name, index,
		finalContent.String(),
		finalReasoning.String(),
		respToolCall,
		usage):
	}
}

func (m *Model) sendErrorResponse(ctx context.Context, ch chan<- ChatResponse, id string, object string, index int, err error, usage Usage) {
	select {
	case <-ctx.Done():

	case ch <- ChatResponseErr(id, object, m.modelInfo.Name, index,
		err,
		usage):

	default:
	}
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

func extractFunctionName(s string) string {
	for field := range strings.FieldsSeq(s) {
		if _, after, ok := strings.Cut(field, "="); ok {
			split := strings.Split(after, ".")
			if len(split) != 2 {
				return ""
			}

			switch split[0] {
			case "functions":
				return split[1]
			}

			return ""
		}
	}

	return ""
}
