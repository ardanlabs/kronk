package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/google/uuid"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

const streamChBuffer = 32

// Chat performs a chat request and returns the final response.
// All requests (including vision/audio) use batch processing and can run
// concurrently based on the NSeqMax config value, which controls parallel
// sequence processing.
func (m *Model) Chat(ctx context.Context, d D) (ChatResponse, error) {
	ch := m.ChatStreaming(ctx, d)

	var lastMsg ChatResponse
	for msg := range ch {
		lastMsg = msg
	}

	// If the response is an error, extract the error message from Delta
	// (where ChatResponseErr stores it) and return it as a Go error.
	if len(lastMsg.Choices) > 0 && lastMsg.Choices[0].FinishReason() == FinishReasonError {
		errMsg := "unknown error"
		if lastMsg.Choices[0].Delta != nil && lastMsg.Choices[0].Delta.Content != "" {
			errMsg = lastMsg.Choices[0].Delta.Content
		}
		return lastMsg, errors.New(errMsg)
	}

	if lastMsg.Object == ObjectChatText {
		lastMsg.Object = ObjectChatTextFinal
	}

	if len(lastMsg.Choices) > 0 {
		lastMsg.Choices[0].Index = 0
		lastMsg.Choices[0].Delta = nil
	}

	return lastMsg, nil
}

// ChatStreaming performs a chat request and streams the response.
// All requests (including vision/audio) use batch processing and can run
// concurrently based on the NSeqMax config value, which controls parallel
// sequence processing.
func (m *Model) ChatStreaming(ctx context.Context, d D) <-chan ChatResponse {
	returnCh := make(chan ChatResponse, streamChBuffer)
	ch := m.wrapChannelForLogging(ctx, returnCh)

	// Increment active streams before launching the goroutine to prevent a race
	// where Unload sees zero active streams and frees the model before the
	// goroutine starts executing.
	active := m.activeStreams.Add(1)
	metrics.SetPoolActiveStreams(m.modelInfo.ID, int(active))

	go func() {
		id := "chatcmpl-" + uuid.NewString()
		requestStart := time.Now()

		m.log(ctx, "chat-streaming", "status", "started", "id", id, "active_streams", active)

		batching := false

		defer func() {
			if rec := recover(); rec != nil {
				m.recordChatFailure(ctx, requestStart, fmt.Errorf("panic: %v", rec))
				m.sendChatError(ctx, ch, id, fmt.Errorf("%v", rec))
			}

			if !batching {
				// Decrement activeStreams BEFORE close(ch). The HTTP handler
				// (and Chat()'s range loop) blocks on the response channel; the
				// instant close fires, the request is considered done by the
				// caller and the next sequential request can start. The pool's
				// evictOneIdle reads ActiveStreams() once and returns
				// ErrServerBusy when it's still nonzero (no retry), so closing
				// before decrementing leaves a race window where back-to-back
				// requests against a one-slot pool flake with "all model slots
				// have active requests".
				remaining := m.activeStreams.Add(-1)
				metrics.SetPoolActiveStreams(m.modelInfo.ID, int(remaining))
				close(ch)
				m.log(ctx, "chat-streaming", "status", "finished", "id", id, "active_streams", remaining)
			}
		}()

		prepCtx, prepSpan := otel.AddSpan(ctx, "prepare-request")

		params, d, err := m.validateAndCloneDocument(prepCtx, d)
		if err != nil {
			prepSpan.End()
			m.recordChatFailure(ctx, requestStart, err)
			m.sendChatError(ctx, ch, id, err)
			return
		}

		d, object, err := m.prepareContext(prepCtx, d)
		if err != nil {
			prepSpan.End()
			m.recordChatFailure(ctx, requestStart, err)
			m.sendChatError(ctx, ch, id, err)
			return
		}

		// The per-request mtmd processing context is owned by the slot
		// (created in startSlot, freed in freeSlotResources). The chat
		// handler does not own one — it only uses m.mtmdMetaCtx for
		// SupportVision/SupportAudio metadata reads.
		//
		// NOTE: We must NOT call m.resetContext() on the failure path here.
		// resetContext() calls llama.MemoryClear(mem, true) which wipes the
		// ENTIRE KV cache — including sequences owned by other in-flight
		// batched requests and other clients' IMC sessions. It also races
		// with the batch engine's llama.Decode because it does not hold
		// m.decodeMu, which is the SIGSEGV we hit when VS Code cancelled
		// one of three concurrent chat streams. Per-request IMC cleanup on
		// submit failure is already handled inside submitToBatchEngine via
		// m.imcClearPending(cache.imcSlotID).

		prompt, media, cache, err := m.prepareCacheAndPrompt(prepCtx, d, object, requestStart)
		if err != nil {
			prepSpan.End()
			m.recordChatFailure(ctx, requestStart, err)
			m.sendChatError(ctx, ch, id, err)
			return
		}

		d = cache.modifiedD

		if m.cfg.InsecureLogging() {
			m.log(ctx, "chat-streaming", "IN-MESSAGES", d.Messages())
		}

		prepSpan.End()

		if m.submitToBatchEngine(ctx, ch, id, d, object, prompt, media, params, cache, requestStart) {
			batching = true
			return
		}
	}()

	return returnCh
}

// wrapChannelForLogging wraps the response channel with logging when insecure
// logging is enabled. Returns the channel to use for sending responses.
func (m *Model) wrapChannelForLogging(ctx context.Context, returnCh chan ChatResponse) chan ChatResponse {
	if !m.cfg.InsecureLogging() {
		return returnCh
	}

	ch := make(chan ChatResponse, streamChBuffer)

	go func() {
		var srl StreamingResponseLogger

		for resp := range ch {
			srl.Capture(resp)

			select {
			case returnCh <- resp:
			case <-ctx.Done():
				m.log(ctx, "chat-streaming", "OUT-MESSAGES", srl.String())
				close(returnCh)
				return
			}
		}

		m.log(ctx, "chat-streaming", "OUT-MESSAGES", srl.String())
		close(returnCh)
	}()

	return ch
}

// validateAndCloneDocument clones the request document first to avoid mutating
// the caller's map (parseParams writes back a normalized enable_thinking), then
// validates the clone. Downstream functions (prepareTextContext,
// gptInjectToolCallNames) use copy-on-write when they need to modify individual
// message maps.
func (m *Model) validateAndCloneDocument(ctx context.Context, d D) (Params, D, error) {
	d = d.Clone()

	params, err := m.validateDocument(d)
	if err != nil {
		return Params{}, nil, err
	}

	m.log(ctx, "chat-streaming", "FINAL-PARAMS", params.String())

	return params, d, nil
}

// prepareContext prepares the document for inference, handling both text-only
// and media (vision/audio) paths. Returns the modified document and object type.
func (m *Model) prepareContext(ctx context.Context, d D) (D, string, error) {
	if m.projFile == "" {
		return m.prepareTextContext(d), ObjectChatText, nil
	}

	// If the model supports media but this request has no media content,
	// treat it as text so caching (IMC) can operate.
	mediaType, _, _, _ := detectMediaContent(d)
	if mediaType == MediaTypeNone {
		return m.prepareTextContext(d), ObjectChatText, nil
	}

	d, err := m.prepareMediaContext(ctx, d)
	if err != nil {
		return nil, ObjectChatUnknown, err
	}

	return d, ObjectChatMedia, nil
}

// prepareCacheAndPrompt handles cache processing and prompt creation. Returns
// the prompt, media bytes, cache result, and any error.
func (m *Model) prepareCacheAndPrompt(ctx context.Context, d D, object string, requestStart time.Time) (string, [][]byte, cacheResult, error) {
	var cache cacheResult

	// Deserialize tool call arguments from JSON strings to maps so Jinja
	// templates can iterate over them with |items. The OpenAI API spec
	// sends arguments as JSON-encoded strings, but templates like Qwen3
	// need them as mappings to render prior tool calls correctly.
	d = deserializeToolCallArguments(d)

	// IMC caches through media messages using the mtmd pipeline —
	// images and audio remain in the KV cache across requests.
	cachingEnabled := m.cfg.IncrementalCache() && (object == ObjectChatText || (object == ObjectChatMedia && m.projFile != ""))

	switch {
	case !cachingEnabled:
		cache.modifiedD = d

	default:
		ctx, cacheSpan := otel.AddSpan(ctx, "process-cache")

		cache = m.processCache(ctx, d, requestStart)

		cacheSpan.End()

		if cache.err != nil {
			return "", nil, cache, cache.err
		}

		d = cache.modifiedD
	}

	prompt, media, err := m.createPrompt(ctx, d)
	if err != nil {
		// processCache marks the matched session pending and relies on
		// startSlot (success) or submitToBatchEngine (submit failure) to
		// clear it. createPrompt sits between those two and is the one
		// uncovered window: if it errors after a reservation, pending
		// leaks and every subsequent request hangs on the session until
		// waitForIMCSlot times out and returns "server busy". Clear the
		// reservation here so the failure is local to this request.
		m.clearIMCPendingIfReserved(cache)
		return "", nil, cache, fmt.Errorf("chat-streaming: unable to apply jinja template: %w", err)
	}

	return prompt, media, cache, nil
}

// clearIMCPendingIfReserved releases an IMC session reservation when
// processCache marked one for build/extend but a subsequent step (e.g.,
// createPrompt) failed before the batch engine took ownership. Pure cache
// hits (no new tokens, no media build) hold no reservation and need no clear.
func (m *Model) clearIMCPendingIfReserved(cache cacheResult) {
	if cache.imcSession == nil {
		return
	}
	if len(cache.imcNewCacheTokens) == 0 && !cache.imcMediaBuild {
		return
	}
	m.imcClearPending(cache.imcSlotID)
}

// submitToBatchEngine attempts to submit the request to the batch engine.
// Returns true if the job was submitted (caller should set batching=true),
// false if batch engine is not available or not applicable.
func (m *Model) submitToBatchEngine(ctx context.Context, ch chan ChatResponse, id string, d D, object string, prompt string, media [][]byte, params Params, cache cacheResult, requestStart time.Time) bool {
	imcCacheHit := m.cfg.IncrementalCache() && (cache.cacheIdx > 0 || len(cache.imcNewCacheTokens) > 0 || cache.imcMediaBuild)

	_, queueSpan := otel.AddSpan(ctx, "queue-wait")

	job := chatJob{
		id:            id,
		ctx:           ctx,
		queueWaitSpan: queueSpan,
		queuedAt:      time.Now(),
		requestStart:  requestStart,
		d:             d,
		object:        object,
		prompt:        prompt,
		media:         media,
		params:        params,
		ch:            ch,

		imcSession:      cache.imcSession,
		imcSessionMedia: cache.imcSession != nil && (cache.imcSession.hasMedia || cache.imcMediaBuild),
		imcSlotID:       cache.imcSlotID,
		imcCacheHit:     imcCacheHit,
		imcExpectedHash: cache.imcExpectedHash,

		imcExpectedCachedMsgs:  cache.imcExpectedCachedMsgs,
		imcExpectedTokens:      cache.imcExpectedTokens,
		imcExpectedRenderHash:  cache.imcExpectedRenderHash,
		imcPureHitSkipSnapshot: cache.imcPureHitSkipSnapshot,

		imcNewCacheTokens:      cache.imcNewCacheTokens,
		imcNewTotalCached:      cache.imcNewTotalCached,
		imcNewCachedMsgCount:   cache.imcNewCachedMsgCount,
		imcNewMsgsHash:         cache.imcNewMsgsHash,
		imcClearSeq:            cache.imcClearSeq,
		imcNewCachedTokens:     cache.imcNewCachedTokens,
		imcTrimPos:             cache.imcTrimPos,
		imcSysPromptHash:       cache.imcSysPromptHash,
		imcSysPromptTokens:     cache.imcSysPromptTokens,
		imcMediaBuild:          cache.imcMediaBuild,
		imcMediaCacheD:         cache.imcMediaCacheD,
		imcMediaKVCounts:       cache.imcMediaKVCounts,
		imcMediaSkipTextTokens: cache.imcMediaSkipTextTokens,
	}

	if err := m.batch.submit(&job); err != nil {
		queueSpan.End()

		// Clear IMC pending reservation if this job reserved a slot.
		// pending is set during extendIMCCache/buildIMCCacheFromScratch
		// and normally cleared in startSlot after decode.
		if len(cache.imcNewCacheTokens) > 0 || cache.imcMediaBuild {
			m.imcClearPending(cache.imcSlotID)
		}

		m.sendChatError(ctx, ch, id, err)
		return false
	}

	return true
}

// prepareTextContext converts messages using the OpenAI array format
// for content ([]D with type:"text") to simple string content. This is used
// for text-only inference paths. Uses copy-on-write: only allocates a new
// messages slice and message maps when array-format content is found.
func (*Model) prepareTextContext(d D) D {
	messages, ok := d["messages"].([]D)
	if !ok {
		return d
	}

	var copied bool
	for i, msg := range messages {
		content, ok := msg["content"].([]D)
		if !ok {
			continue
		}

		var text strings.Builder
		for _, part := range content {
			if part["type"] == "text" {
				if s, ok := part["text"].(string); ok {
					text.WriteString(s)
				}
			}
		}

		if text.Len() > 0 {
			if !copied {
				newMsgs := make([]D, len(messages))
				copy(newMsgs, messages)
				messages = newMsgs
				d["messages"] = messages
				copied = true
			}

			newMsg := msg.ShallowClone()
			newMsg["content"] = text.String()
			messages[i] = newMsg
		}
	}

	return d
}

func (m *Model) prepareMediaContext(ctx context.Context, d D) (D, error) {
	mediaType, isOpenAIFormat, msgs, err := detectMediaContent(d)
	if err != nil {
		return nil, fmt.Errorf("prepare-media-context: %w", err)
	}

	if mediaType != MediaTypeNone && m.projFile == "" {
		return nil, fmt.Errorf("prepare-media-context: media detected in request but model does not support media processing")
	}

	// The chat handler only needs metadata (vision/audio support), so we
	// use the long-lived metadata-only mtmd context. Per-request
	// processing contexts are created and freed by each slot.
	if m.mtmdMetaCtx == 0 {
		return nil, fmt.Errorf("prepare-media-context: model has no mtmd context loaded")
	}
	metaCtx := m.mtmdMetaCtx

	switch mediaType {
	case MediaTypeVision:
		if !mtmd.SupportVision(metaCtx) {
			return nil, fmt.Errorf("prepare-media-context: image/video detected but model does not support vision")
		}

	case MediaTypeAudio:
		if !mtmd.SupportAudio(metaCtx) {
			return nil, fmt.Errorf("prepare-media-context: audio detected but model does not support audio")
		}
	}

	switch {
	case isOpenAIFormat:
		d, err = toMediaMessage(d, msgs)
		if err != nil {
			return nil, fmt.Errorf("prepare-media-context: unable to convert document to media message: %w", err)
		}

	case mediaType != MediaTypeNone:
		d = convertPlainBase64ToBytes(d)
	}

	return d, nil
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

// deserializeToolCallArguments converts JSON-string arguments in assistant
// tool_calls to maps so Jinja templates can iterate them with |items.
func deserializeToolCallArguments(d D) D {
	messages, ok := d["messages"].([]D)
	if !ok {
		return d
	}

	var copied bool
	for i, msg := range messages {
		role, _ := msg["role"].(string)
		if role != "assistant" {
			continue
		}

		toolCalls, ok := msg["tool_calls"].([]D)
		if !ok {
			continue
		}

		for j, tc := range toolCalls {
			fn, ok := tc["function"].(D)
			if !ok {
				continue
			}

			argsStr, ok := fn["arguments"].(string)
			if !ok {
				continue
			}

			var argsMap map[string]any
			if err := json.Unmarshal([]byte(argsStr), &argsMap); err != nil {
				continue
			}

			if !copied {
				newMsgs := make([]D, len(messages))
				copy(newMsgs, messages)
				messages = newMsgs
				d["messages"] = messages
				copied = true
			}

			newFn := fn.ShallowClone()
			newFn["arguments"] = argsMap

			newTC := tc.ShallowClone()
			newTC["function"] = newFn

			newTCs := make([]D, len(toolCalls))
			copy(newTCs, toolCalls)
			newTCs[j] = newTC

			newMsg := msg.ShallowClone()
			newMsg["tool_calls"] = newTCs

			messages[i] = newMsg
			msg = newMsg
			toolCalls = newTCs
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

// recordChatFailure emits the request_total/error counters and the
// request duration histogram for a chat that failed before being handed
// to the batch engine. errors.Is checks pull context cancellations into
// the "cancel" status so dashboards can distinguish them from genuine
// errors.
func (m *Model) recordChatFailure(ctx context.Context, requestStart time.Time, err error) {
	status := "error"
	class := "pre-batch"

	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		status = "cancel"
		class = "context-cancelled"
	case ctx.Err() != nil:
		status = "cancel"
		class = "context-cancelled"
	}

	metrics.AddChatRequest(m.modelInfo.ID, status)
	metrics.AddChatError(m.modelInfo.ID, class)
	if !requestStart.IsZero() {
		metrics.ObserveChatRequestDuration(m.modelInfo.ID, time.Since(requestStart))
	}
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
