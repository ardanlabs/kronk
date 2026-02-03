package model

import (
	"context"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/google/uuid"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"go.opentelemetry.io/otel/attribute"
)

// Chat performs a chat request and returns the final response.
// Text inference requests can run concurrently based on the NSeqMax config
// value, which controls parallel sequence processing. However, requests that
// include vision or audio content are processed sequentially due to media
// pipeline constraints.
func (m *Model) Chat(ctx context.Context, d D) (ChatResponse, error) {
	ch := m.ChatStreaming(ctx, d)

	var lastMsg ChatResponse
	for msg := range ch {
		lastMsg = msg
	}

	if lastMsg.Object == ObjectChatText {
		lastMsg.Object = ObjectChatTextFinal
	}

	if len(lastMsg.Choice) > 0 {
		lastMsg.Choice[0].Index = 0
		lastMsg.Choice[0].Delta = nil
	}

	return lastMsg, nil
}

// ChatStreaming performs a chat request and streams the response.
// Text inference requests can run concurrently based on the NSeqMax config
// value, which controls parallel sequence processing. However, requests that
// include vision or audio content are processed sequentially due to media
// pipeline constraints.
func (m *Model) ChatStreaming(ctx context.Context, d D) <-chan ChatResponse {
	returnCh := make(chan ChatResponse, 1)
	ch := m.wrapChannelForLogging(ctx, returnCh)

	// Increment active streams before launching the goroutine to prevent a race
	// where Unload sees zero active streams and frees the model before the
	// goroutine starts executing.
	active := m.activeStreams.Add(1)

	go func() {
		id := fmt.Sprintf("chatcmpl-%s", uuid.New().String())

		m.log(ctx, "chat-streaming", "status", "started", "id", id, "active_streams", active)

		batching := false

		defer func() {
			if rec := recover(); rec != nil {
				m.sendChatError(ctx, ch, id, fmt.Errorf("%v", rec))
			}

			if !batching {
				close(ch)
				remaining := m.activeStreams.Add(-1)
				m.log(ctx, "chat-streaming", "status", "finished", "id", id, "active_streams", remaining)
			}
		}()

		params, d, err := m.validateAndCloneDocument(ctx, d)
		if err != nil {
			m.sendChatError(ctx, ch, id, err)
			return
		}

		d, mtmdCtx, object, err := m.prepareContext(ctx, d)
		if err != nil {
			m.sendChatError(ctx, ch, id, err)
			return
		}

		defer func() {
			if !batching {
				if mtmdCtx != 0 {
					mtmd.Free(mtmdCtx)
				}

				m.resetContext()
			}
		}()

		prompt, media, cache, err := m.prepareCacheAndPrompt(ctx, d, object)
		if err != nil {
			m.sendChatError(ctx, ch, id, err)
			return
		}

		d = cache.modifiedD

		if m.cfg.InsecureLogging {
			m.log(ctx, "chat-streaming", "IN-MESSAGAES", d.Messages())
		}

		if m.submitToBatchEngine(ctx, ch, id, d, object, prompt, media, params, mtmdCtx, cache) {
			batching = true
			return
		}

		m.sequentialChatRequest(ctx, id, m.lctx, mtmdCtx, object, prompt, media, params, ch)
	}()

	return returnCh
}

// wrapChannelForLogging wraps the response channel with logging when insecure
// logging is enabled. Returns the channel to use for sending responses.
func (m *Model) wrapChannelForLogging(ctx context.Context, returnCh chan ChatResponse) chan ChatResponse {
	if !m.cfg.InsecureLogging {
		return returnCh
	}

	ch := make(chan ChatResponse, 1)

	go func() {
		var srl StreamingResponseLogger

		for resp := range ch {
			srl.Capture(resp)
			returnCh <- resp
		}

		m.log(ctx, "chat-streaming", "OUT-MESSAGES", srl.String())
		close(returnCh)
	}()

	return ch
}

// validateAndCloneDocument validates the request document and returns a clone
// to avoid mutating the caller's data.
func (m *Model) validateAndCloneDocument(ctx context.Context, d D) (Params, D, error) {
	params, err := m.validateDocument(d)
	if err != nil {
		return Params{}, nil, err
	}

	m.log(ctx, "chat-streaming", "FINAL-PARAMS", params.String())

	return params, d.Clone(), nil
}

// prepareContext prepares the document for inference, handling both text-only
// and media (vision/audio) paths. Returns the modified document, media context,
// and object type.
func (m *Model) prepareContext(ctx context.Context, d D) (D, mtmd.Context, string, error) {
	if m.projFile == "" {
		return m.prepareTextContext(d), 0, ObjectChatText, nil
	}

	d, mtmdCtx, err := m.prepareMediaContext(ctx, d)
	if err != nil {
		return nil, 0, ObjectChatUnknown, err
	}

	return d, mtmdCtx, ObjectChatMedia, nil
}

// prepareCacheAndPrompt handles cache processing and prompt creation. Returns
// the prompt, media bytes, cache result, and any error.
func (m *Model) prepareCacheAndPrompt(ctx context.Context, d D, object string) (string, [][]byte, cacheResult, error) {
	var cache cacheResult

	// For GPT models, inject tool_call_name into tool response messages before
	// caching. This must happen before processCache so both the full prompt and
	// prefix have consistent tool names when templated.
	if m.modelInfo.IsGPTModel {
		d = m.gptInjectToolCallNames(ctx, d)
	}

	cachingEnabled := (m.cfg.SystemPromptCache || m.cfg.IncrementalCache) && object == ObjectChatText

	switch {
	case !cachingEnabled:
		cache.modifiedD = d

	default:
		cache = m.processCache(ctx, d)
		if cache.err != nil {
			return "", nil, cache, cache.err
		}

		if cache.newDataCached != "" {
			return cache.newDataCached, cache.media, cache, nil
		}

		d = cache.modifiedD
	}

	prompt, media, err := m.createPrompt(ctx, d)
	if err != nil {
		return "", nil, cache, fmt.Errorf("chat-streaming: unable to apply jinja template: %w", err)
	}

	return prompt, media, cache, nil
}

// submitToBatchEngine attempts to submit the request to the batch engine.
// Returns true if the job was submitted (caller should set batching=true),
// false if batch engine is not available or not applicable.
func (m *Model) submitToBatchEngine(ctx context.Context, ch chan ChatResponse, id string, d D, object string, prompt string, media [][]byte, params Params, mtmdCtx mtmd.Context, cache cacheResult) bool {
	if m.batch == nil || object != ObjectChatText {
		return false
	}

	sysPromptCached := false
	if m.cfg.SystemPromptCache {
		sysPromptCached = cache.cached
	}

	job := chatJob{
		id:              id,
		ctx:             ctx,
		d:               d,
		object:          object,
		prompt:          prompt,
		media:           media,
		params:          params,
		mtmdCtx:         mtmdCtx,
		ch:              ch,
		sysPromptNPast:  cache.nPast,
		sysPromptCached: sysPromptCached,
		imcID:           cache.imcID,
		imcSeqID:        cache.imcSeqID,
		imcNPast:        cache.nPast,
		imcCached:       cache.imcID != "",
	}

	if err := m.batch.submit(&job); err != nil {
		m.sendChatError(ctx, ch, id, err)
		return false
	}

	return true
}

// prepareTextContext converts messages using the OpenAI array format
// for content ([]D with type:"text") to simple string content. This is used
// for text-only inference paths.
func (*Model) prepareTextContext(d D) D {
	messages, ok := d["messages"].([]D)
	if !ok {
		return d
	}

	for i, msg := range messages {
		content, ok := msg["content"].([]D)
		if !ok {
			continue
		}

		for _, part := range content {
			if part["type"] == "text" {
				if text, ok := part["text"].(string); ok {
					messages[i]["content"] = text
					break
				}
			}
		}
	}

	return d
}

func (m *Model) prepareMediaContext(ctx context.Context, d D) (D, mtmd.Context, error) {
	mediaType, isOpenAIFormat, msgs, err := detectMediaContent(d)
	if err != nil {
		return nil, 0, fmt.Errorf("prepare-media-context: %w", err)
	}

	if mediaType != MediaTypeNone && m.projFile == "" {
		return nil, 0, fmt.Errorf("prepare-media-context: media detected in request but model does not support media processing")
	}

	var mtmdCtx mtmd.Context

	mtmdCtx, err = m.loadProjFile(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("prepare-media-context: unable to init projection: %w", err)
	}

	switch mediaType {
	case MediaTypeVision:
		if !mtmd.SupportVision(mtmdCtx) {
			mtmd.Free(mtmdCtx)
			return nil, 0, fmt.Errorf("prepare-media-context: image/video detected but model does not support vision")
		}

	case MediaTypeAudio:
		if !mtmd.SupportAudio(mtmdCtx) {
			mtmd.Free(mtmdCtx)
			return nil, 0, fmt.Errorf("prepare-media-context: audio detected but model does not support audio")
		}
	}

	switch {
	case isOpenAIFormat:
		d, err = toMediaMessage(d, msgs)
		if err != nil {
			return nil, 0, fmt.Errorf("prepare-media-context: unable to convert document to media message: %w", err)
		}

	case mediaType != MediaTypeNone:
		d = convertPlainBase64ToBytes(d)
	}

	return d, mtmdCtx, nil
}

func (m *Model) loadProjFile(ctx context.Context) (mtmd.Context, error) {
	baseProjFile := path.Base(m.projFile)

	m.log(context.Background(), "loading-prof-file", "status", "started", "proj", baseProjFile)
	defer m.log(context.Background(), "loading-prof-file", "status", "completed", "proj", baseProjFile)

	_, span := otel.AddSpan(ctx, "proj-file-load-time",
		attribute.String("proj-file", baseProjFile),
	)
	defer span.End()

	start := time.Now()
	defer func() {
		metrics.AddProjFileLoadTime(m.modelInfo.ID, time.Since(start))
	}()

	mtmdCtx, err := mtmd.InitFromFile(m.projFile, m.model, mtmd.ContextParamsDefault())
	if err != nil {
		return 0, err
	}

	return mtmdCtx, nil
}

func (m *Model) createPrompt(ctx context.Context, d D) (string, [][]byte, error) {
	ctx, span := otel.AddSpan(ctx, "create-prompt")
	defer span.End()

	start := time.Now()
	defer func() {
		metrics.AddPromptCreationTime(m.modelInfo.ID, time.Since(start))
	}()

	prompt, media, err := m.applyRequestJinjaTemplate(ctx, d)
	if err != nil {
		return "", nil, err
	}

	return prompt, media, nil
}

// gptInjectToolCallNames adds tool_call_name to tool response messages for GPT
// models. The gpt-oss.jinja template requires this field to render the function
// name in the output, but OpenAI's standard tool response only includes
// tool_call_id.
//
// This function builds a mapping from tool_call_id to function name by scanning
// assistant messages with tool_calls, then injects tool_call_name into any
// subsequent tool role messages.
func (m *Model) gptInjectToolCallNames(ctx context.Context, d D) D {
	messages, ok := d["messages"].([]D)
	if !ok {
		return d
	}

	// Build a map of tool_call_id -> function name from assistant messages.
	toolCallIDToName := make(map[string]string)

	for _, msg := range messages {
		role, _ := msg["role"].(string)
		if role != "assistant" {
			continue
		}

		toolCalls, ok := msg["tool_calls"].([]D)
		if !ok {
			continue
		}

		for _, tcMap := range toolCalls {
			id, _ := tcMap["id"].(string)
			if id == "" {
				continue
			}

			fn, ok := tcMap["function"].(D)
			if !ok {
				continue
			}

			name, _ := fn["name"].(string)
			if name != "" {
				toolCallIDToName[id] = name
			}
		}
	}

	if len(toolCallIDToName) == 0 {
		return d
	}

	// Inject tool_call_name into tool response messages.
	for _, msg := range messages {
		role, _ := msg["role"].(string)
		if role != "tool" {
			continue
		}

		toolCallID, _ := msg["tool_call_id"].(string)
		if toolCallID == "" {
			continue
		}

		if name, exists := toolCallIDToName[toolCallID]; exists {
			m.log(ctx, "gpt-inject-tool-call-names", "status", "injecting name", "tool-call-id", toolCallID, "name", name)
			msg["tool_call_name"] = name
		}
	}

	return d
}

func (m *Model) validateDocument(d D) (Params, error) {
	messages, exists := d["messages"]
	if !exists {
		return Params{}, errors.New("validate-document: no messages found in request")
	}

	if _, ok := messages.([]D); !ok {
		return Params{}, errors.New("validate-document: messages is not a slice of documents")
	}

	p, err := m.parseParams(d)
	if err != nil {
		return Params{}, err
	}

	return p, nil
}

func (m *Model) sendChatError(ctx context.Context, ch chan<- ChatResponse, id string, err error) {
	m.log(ctx, "send-chat-error", "ERROR", err.Error(), "id", id)

	// I want to try and send this message before we check the context.
	select {
	case ch <- ChatResponseErr(id, ObjectChatUnknown, m.modelInfo.ID, 0, "", err, Usage{}):
		return
	default:
	}

	select {
	case <-ctx.Done():
		select {
		case ch <- ChatResponseErr(id, ObjectChatUnknown, m.modelInfo.ID, 0, "", ctx.Err(), Usage{}):
		default:
		}

	case ch <- ChatResponseErr(id, ObjectChatUnknown, m.modelInfo.ID, 0, "", err, Usage{}):
	}
}
