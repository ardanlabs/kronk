package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/google/uuid"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

const streamChBuffer = 32

// ErrFileInputsUnsupported indicates file content parts are not supported.
var ErrFileInputsUnsupported = errors.New("file inputs are not currently supported")

// ErrMessagesMissing indicates that a chat request has no messages field.
var ErrMessagesMissing = errors.New("validate-document: no messages found in request")

// ErrMessagesInvalid indicates that a chat request's messages field has an invalid type.
var ErrMessagesInvalid = errors.New("validate-document: messages is not a slice of documents")

// ErrInvalidRequest indicates that a model request contains an invalid or
// unsupported field value.
var ErrInvalidRequest = errors.New("validate-document: invalid request")

// Chat performs a chat request and returns the final response.
// All requests (including vision/audio) use batch processing and can run
// concurrently based on the NSeqMax config value, which controls parallel
// sequence processing.
func (m *Model) Chat(ctx context.Context, d D) (ChatResponse, error) {
	if err := ValidateChatRequest(d); err != nil {
		return ChatResponse{}, err
	}

	ch, err := m.chatStreaming(ctx, d, false)
	if err != nil {
		return ChatResponse{}, err
	}

	var lastMsg ChatResponse
	for msg := range ch {
		lastMsg = msg
	}

	// If the response is an error, return the original internal error. Fall
	// back to the message in Delta if the response does not contain one.
	if len(lastMsg.Choices) > 0 && lastMsg.Choices[0].FinishReason() == FinishReasonError {
		if err := ctx.Err(); err != nil {
			return lastMsg, err
		}
		if lastMsg.internal.cause != nil {
			return lastMsg, lastMsg.internal.cause
		}

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
// When stream_options.include_usage is true, the terminal choice is followed
// by a usage response with an empty Choices slice.
// Validation failures are returned before a response channel is created.
func (m *Model) ChatStreaming(ctx context.Context, d D) (<-chan ChatResponse, error) {
	return m.chatStreaming(ctx, d, true)
}

func (m *Model) chatStreaming(ctx context.Context, d D, streaming bool) (<-chan ChatResponse, error) {
	requestStart := time.Now()

	// Increment active streams before preparing the request to prevent Unload
	// from freeing the model during template rendering or tokenization.
	active := m.activeStreams.Add(1)
	metrics.AddPoolActiveStreams(m.modelInfo.ID, 1)

	id := "chatcmpl-" + uuid.NewString()

	m.log(ctx, "chat-streaming", "status", "started", "id", id, "active_streams", active)
	m.log(ctx, "request-lifecycle",
		"stage", 2,
		"stage_name", "prepare-model-work",
		"status", "started",
		"id", id,
	)

	prepCtx, prepSpan := otel.AddSpan(ctx, "prepare-request")

	prepared, err := m.prepareChat(prepCtx, d, streaming, requestStart)
	if err != nil {
		m.releaseIMCReservationIfHeld(prepared.cache)
		prepSpan.End()
		m.log(ctx, "request-lifecycle", "stage", 2, "stage_name", "prepare-model-work",
			"status", lifecycleStatus(err), "id", id, "elapsed", time.Since(requestStart).String(), "err", err)
		m.recordChatFailure(ctx, requestStart, err)
		remaining := m.activeStreams.Add(-1)
		metrics.AddPoolActiveStreams(m.modelInfo.ID, -1)
		m.log(ctx, "chat-streaming", "status", "finished", "id", id, "active_streams", remaining)
		return nil, err
	}

	if m.cfg.InsecureLogging() {
		m.log(ctx, "chat-streaming", "IN-MESSAGES", prepared.d.Messages())
	}

	prepSpan.End()
	m.log(ctx, "request-lifecycle",
		"stage", 2,
		"stage_name", "prepare-model-work",
		"status", "complete",
		"id", id,
		"elapsed", time.Since(requestStart).String(),
		"imc_session", prepared.cache.imcSessionID,
		"imc_match_kind", prepared.cache.imcMatchKind,
	)

	returnCh := make(chan ChatResponse, streamChBuffer)
	ch := m.wrapChannelForLogging(ctx, returnCh)

	go func() {
		batching := false

		defer func() {
			if rec := recover(); rec != nil {
				if !batching {
					m.releaseIMCReservationIfHeld(prepared.cache)
				}
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
				// requests against a one-slot pool flake with "no idle pool
				// entry available to evict".
				remaining := m.activeStreams.Add(-1)
				metrics.AddPoolActiveStreams(m.modelInfo.ID, -1)
				close(ch)
				m.log(ctx, "chat-streaming", "status", "finished", "id", id, "active_streams", remaining)
			}
		}()

		if m.submitToBatchEngine(ctx, ch, id, prepared, requestStart) {
			batching = true
			return
		}
	}()

	return returnCh, nil
}

type preparedChat struct {
	d          D
	object     string
	prompt     string
	media      [][]byte
	params     Params
	cache      cacheResult
	textTokens []llama.Token
}

func (m *Model) prepareChat(ctx context.Context, d D, streaming bool, requestStart time.Time) (preparedChat, error) {
	// Establish ownership before preparation starts. Callers may reuse or modify
	// their request after ChatStreaming returns; the queued job must retain a
	// stable snapshot of all mutable JSON containers.
	d = d.Clone()

	params, d, err := m.validateOwnedDocument(ctx, d)
	if err != nil {
		return preparedChat{}, err
	}
	params.Stream = streaming

	d, object, err := m.prepareContext(ctx, d)
	if err != nil {
		return preparedChat{}, err
	}

	prompt, media, cache, err := m.prepareCacheAndPrompt(ctx, d, object, requestStart)
	prepared := preparedChat{
		d:      cache.modifiedD,
		object: object,
		prompt: prompt,
		media:  media,
		params: params,
		cache:  cache,
	}
	if err != nil {
		return prepared, err
	}

	if err := m.prepareTextBudget(ctx, &prepared); err != nil {
		return prepared, err
	}

	return prepared, nil
}

func (m *Model) prepareTextBudget(ctx context.Context, prepared *preparedChat) error {
	// Multimodal token usage depends on mtmd processing performed by the batch
	// slot. Text prompts can be fully tokenized and rejected before an HTTP
	// streaming response is committed.
	if prepared.object != ObjectChatText {
		return nil
	}

	if prepared.cache.imcTokenPlan {
		prepared.textTokens = prepared.cache.imcSamplerPromptTokens
	} else {
		prepared.textTokens = llama.Tokenize(m.vocab, prepared.prompt, m.addBOSToken, true)
	}

	contextWindow := m.cfg.ContextWindow()
	requestedMaxTokens := prepared.params.MaxTokens
	effectiveMaxTokens, ok := contextOutputBudget(len(prepared.textTokens), requestedMaxTokens, contextWindow)
	if !ok {
		return fmt.Errorf("%w: input tokens [%d] exceed context window [%d]", ErrInvalidRequest, len(prepared.textTokens), contextWindow)
	}

	prepared.params.MaxTokens = effectiveMaxTokens
	if effectiveMaxTokens < requestedMaxTokens {
		m.log(ctx, "prepare-chat",
			"status", "max-tokens-clamped",
			"prompt_tokens", len(prepared.textTokens),
			"context_window", contextWindow,
			"requested_max_tokens", requestedMaxTokens,
			"effective_max_tokens", effectiveMaxTokens)
	}

	return nil
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

// validateOwnedDocument normalizes and validates the request snapshot owned by
// the chat pipeline. Downstream functions use copy-on-write when they need to
// modify individual message maps.
func (m *Model) validateOwnedDocument(ctx context.Context, d D) (Params, D, error) {
	if err := normalizeChatTemplateKwargs(d, m.cfg.ChatTemplateKwargs); err != nil {
		return Params{}, nil, err
	}

	params, err := m.validateDocument(ctx, d)
	if err != nil {
		return Params{}, nil, err
	}

	kwargs, _ := d["chat_template_kwargs"].(D)
	finalParams := params.String() + fmt.Sprintf("chat_template_kwargs[%s]\n", chatTemplateKwargsSummary(kwargs))
	m.log(ctx, "chat-streaming", "FINAL-PARAMS", finalParams)

	return params, d, nil
}

// normalizeChatTemplateKwargs merges model defaults with vLLM/SGLang-style
// request template arguments and promotes enable_thinking because Kronk uses
// it for both parameter resolution and Jinja rendering. Request arguments
// override model defaults. Other arguments are unpacked only when the template
// renders so names that overlap sampler fields remain template-only.
func normalizeChatTemplateKwargs(d D, defaults D) error {
	requestKwargs, exists, err := requestChatTemplateKwargs(d)
	if err != nil {
		return err
	}
	if !exists && len(defaults) == 0 {
		return nil
	}

	kwargs := D{}
	maps.Copy(kwargs, defaults)
	if exists {
		maps.Copy(kwargs, requestKwargs)
	}

	if _, exists := kwargs["chat_template_kwargs"]; exists {
		return fmt.Errorf("%w: chat_template_kwargs cannot contain itself", ErrInvalidRequest)
	}
	if enableThinking, exists := kwargs["enable_thinking"]; exists {
		if _, topLevel := d["enable_thinking"]; !topLevel {
			d["enable_thinking"] = enableThinking
		}
	}

	d["chat_template_kwargs"] = kwargs

	return nil
}

func requestChatTemplateKwargs(d D) (D, bool, error) {
	value, exists := d["chat_template_kwargs"]
	if !exists {
		return nil, false, nil
	}

	var kwargs D
	switch value := value.(type) {
	case D:
		kwargs = value
	case map[string]any:
		kwargs = value
	default:
		return nil, true, fmt.Errorf("%w: chat_template_kwargs must be an object", ErrInvalidRequest)
	}

	if _, exists := kwargs["chat_template_kwargs"]; exists {
		return nil, true, fmt.Errorf("%w: chat_template_kwargs cannot contain itself", ErrInvalidRequest)
	}

	return kwargs, true, nil
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

	// IMC uses complete-conversation token planning. Render from two
	// independent top-level maps because the Jinja path injects request
	// defaults. Nested request data belongs to this request and remains
	// read-only, so recursive clones are unnecessary.
	if cachingEnabled {
		actualD := maps.Clone(d)
		stableD := maps.Clone(d)
		stableD["add_generation_prompt"] = false

		actualPrompt, actualMedia, err := m.createPrompt(ctx, actualD)
		if err != nil {
			return "", nil, cache, fmt.Errorf("chat-streaming: render complete actual prompt: %w", err)
		}
		stablePrompt, stableMedia, err := m.createPrompt(ctx, stableD)
		if err != nil {
			return "", nil, cache, fmt.Errorf("chat-streaming: render complete stable prompt: %w", err)
		}

		if object == ObjectChatMedia {
			cache = m.processIMCMediaTokenPlan(ctx, d, stableD, actualPrompt, stablePrompt, actualMedia, stableMedia, requestStart)
		} else {
			actualTokens := llama.Tokenize(m.vocab, actualPrompt, m.addBOSToken, true)
			stableTokens := llama.Tokenize(m.vocab, stablePrompt, m.addBOSToken, true)

			var finalUserBoundary int
			messages := dMessages(stableD)
			if len(messages) > 1 && messagesEndAtRealUser(messages) {
				boundaryD := maps.Clone(stableD)
				boundaryD["messages"] = messages[:len(messages)-1]
				boundaryPrompt, _, boundaryErr := m.createPrompt(ctx, boundaryD)
				if boundaryErr == nil {
					boundaryTokens := llama.Tokenize(m.vocab, boundaryPrompt, m.addBOSToken, true)
					if tokensHavePrefix(stableTokens, boundaryTokens) {
						finalUserBoundary = len(boundaryTokens)
					}
				} else {
					m.log(ctx, "imc", "status", "final-user-boundary-render-skipped", "err", boundaryErr)
				}
			}

			cache = m.processIMCTokenPlan(ctx, d, actualTokens, stableTokens, finalUserBoundary, requestStart)
		}
		if cache.err != nil {
			return "", nil, cache, cache.err
		}
		if cache.imcTokenPlan {
			return actualPrompt, nil, cache, nil
		}
		m.log(ctx, "imc", "status", "token-plan-fallback", "cache_mode", "token-v2", "reason", "render-not-prefix-compatible")
		cache.modifiedD = d
		return actualPrompt, actualMedia, cache, nil
	}

	cache.modifiedD = d

	prompt, media, err := m.createPrompt(ctx, d)
	if err != nil {
		return "", nil, cache, fmt.Errorf("chat-streaming: unable to apply jinja template: %w", err)
	}

	return prompt, media, cache, nil
}

// releaseIMCReservationIfHeld releases an IMC session reservation when the
// batch engine does not take ownership. Pure cache read-only exact/anchor hits
// carry an explicit reservation too.
func (m *Model) releaseIMCReservationIfHeld(cache cacheResult) {
	if cache.imcSession == nil {
		return
	}
	if len(cache.imcNewCacheTokens) == 0 && !cache.imcMediaBuild && !cache.imcReadOnlyReservation {
		return
	}
	m.imcReleaseReservation(cache.imcSessionID)
}

// submitToBatchEngine attempts to submit the prepared request to the batch engine.
// Returns true if the job was submitted (caller should set batching=true),
// false if batch engine is not available or not applicable.
func (m *Model) submitToBatchEngine(ctx context.Context, ch chan ChatResponse, id string, prepared preparedChat, requestStart time.Time) bool {
	cache := prepared.cache
	imcCacheHit := m.cfg.IncrementalCache() && (cache.cacheIdx > 0 || len(cache.imcNewCacheTokens) > 0 || cache.imcMediaBuild)

	_, queueSpan := otel.AddSpan(ctx, "queue-wait")
	m.log(ctx, "request-lifecycle",
		"stage", 3,
		"stage_name", "schedule-job",
		"status", "started",
		"id", id,
	)

	job := chatJob{
		id:                  id,
		ctx:                 ctx,
		queueWaitSpan:       queueSpan,
		queuedAt:            time.Now(),
		requestStart:        requestStart,
		d:                   prepared.d,
		object:              prepared.object,
		prompt:              prepared.prompt,
		media:               prepared.media,
		params:              prepared.params,
		ch:                  ch,
		textTokens:          prepared.textTokens,
		samplerPromptTokens: cache.imcSamplerPromptTokens,
		tailTokens:          cache.imcTailTokens,
		imcTokenPlan:        cache.imcTokenPlan,
		imcMatchKind:        cache.imcMatchKind,
		imcPromptPlan:       cache.imcPromptPlan,

		imcSession:         cache.imcSession,
		imcSessionMedia:    cache.imcSession != nil && (cache.imcSession.hasMedia || cache.imcMediaBuild),
		imcSessionUseMRoPE: cache.imcSession != nil && cache.imcSession.useMRoPE,
		imcPhysicalCached:  cache.imcExpectedTokens,
		imcSessionID:       cache.imcSessionID,
		imcCacheHit:        imcCacheHit,
		reusedPromptTokens: int(cache.cacheIdx),
		imcExpectedHash:    cache.imcExpectedHash,

		imcExpectedCachedMsgs:  cache.imcExpectedCachedMsgs,
		imcExpectedTokens:      cache.imcExpectedTokens,
		imcExpectedPosition:    cache.imcExpectedPosition,
		imcExpectedRenderHash:  cache.imcExpectedRenderHash,
		imcExpectedPromptPlan:  cache.imcExpectedPromptPlan,
		imcReadOnlyReservation: cache.imcReadOnlyReservation,
		imcMediaAnchorAdvance:  cache.imcMediaAnchorAdvance,
		imcNewLogicalPosition:  cache.imcNewLogicalPosition,
		imcReservationHeld:     cache.imcReadOnlyReservation || len(cache.imcNewCacheTokens) > 0 || cache.imcMediaBuild,
		imcPureHitSkipSnapshot: cache.imcPureHitSkipSnapshot,
		imcPromoteCheckpoint:   cache.imcPromoteCheckpoint,
		imcCheckpointTokens:    cache.imcCheckpointTokens,

		imcNewCacheTokens:     cache.imcNewCacheTokens,
		imcNewTotalCached:     cache.imcNewTotalCached,
		imcNewCachedMsgCount:  cache.imcNewCachedMsgCount,
		imcNewMsgsHash:        cache.imcNewMsgsHash,
		imcNewEndsAtUser:      cache.imcNewEndsAtUser,
		imcClearSeq:           cache.imcClearSeq,
		imcNewCachedTokens:    cache.imcNewCachedTokens,
		imcMediaBuild:         cache.imcMediaBuild,
		imcMediaCacheD:        cache.imcMediaCacheD,
		imcMediaKVCounts:      cache.imcMediaKVCounts,
		imcMediaSamplerTokens: cache.imcMediaSamplerTokens,
	}

	if err := m.batch.submit(&job); err != nil {
		queueSpan.RecordError(err)
		queueSpan.End()
		if !job.queuedAt.IsZero() {
			metrics.ObserveChatQueueWait(m.modelInfo.ID, time.Since(job.queuedAt))
		}

		// The batch engine never took ownership, so release any exact,
		// append, or rebuild reservation made while planning the request.
		m.releaseIMCReservationIfHeld(cache)
		m.log(ctx, "request-lifecycle",
			"stage", 3,
			"stage_name", "schedule-job",
			"status", lifecycleStatus(err),
			"id", id,
			"elapsed", time.Since(job.queuedAt).String(),
			"err", err,
		)

		m.recordChatFailure(ctx, requestStart, err)
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

			var args any
			if err := decodeJSONWithNumber(argsStr, &args); err != nil {
				continue
			}

			// ToolCallArguments implements MarshalJSON by returning a JSON
			// string, so callers that marshal it before storing it in message
			// history can produce a string containing JSON object text. Decode
			// that second layer before passing the arguments to Jinja.
			if encoded, ok := args.(string); ok {
				if err := decodeJSONWithNumber(encoded, &args); err != nil {
					continue
				}
			}

			argsMap, ok := args.(map[string]any)
			if !ok && args != nil {
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

func (m *Model) validateDocument(ctx context.Context, d D) (Params, error) {
	if err := ValidateChatRequest(d); err != nil {
		return Params{}, err
	}
	applyToolChoice(d)

	p, err := m.parseParams(ctx, d)
	if err != nil {
		if errors.Is(err, ErrInvalidRequest) {
			return Params{}, err
		}

		return Params{}, fmt.Errorf("%w: %w", ErrInvalidRequest, err)
	}

	return p, nil
}

// ValidateChatRequest validates the fields in a chat request document.
func ValidateChatRequest(d D) error {
	if err := ValidateMessages(d); err != nil {
		return err
	}
	if _, _, err := requestChatTemplateKwargs(d); err != nil {
		return err
	}
	if err := validateChoiceCount(d); err != nil {
		return err
	}
	if val, exists := d["stop"]; exists {
		if _, err := parseStop(val); err != nil {
			return err
		}
	}
	if err := validateToolChoice(d); err != nil {
		return err
	}

	return nil
}

func validateChoiceCount(d D) error {
	val, exists := d["n"]
	if !exists || val == nil {
		return nil
	}

	var supported bool
	switch value := val.(type) {
	case json.Number:
		n, err := value.Float64()
		supported = err == nil && n == 1
	case float32:
		supported = value == 1
	case float64:
		supported = value == 1
	case int:
		supported = value == 1
	case int32:
		supported = value == 1
	case int64:
		supported = value == 1
	}

	if !supported {
		return fmt.Errorf("%w: n values other than 1 are not supported", ErrInvalidRequest)
	}

	return nil
}

func parseStop(val any) ([]string, error) {
	if val == nil {
		return nil, nil
	}

	var stops []string
	switch value := val.(type) {
	case string:
		stops = []string{value}
	case []string:
		stops = slices.Clone(value)
	case []any:
		stops = make([]string, len(value))
		for i, item := range value {
			stop, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%w: stop entries must be strings", ErrInvalidRequest)
			}
			stops[i] = stop
		}
	default:
		return nil, fmt.Errorf("%w: stop must be a string or an array of strings", ErrInvalidRequest)
	}

	if len(stops) > 4 {
		return nil, fmt.Errorf("%w: stop supports at most four sequences", ErrInvalidRequest)
	}
	if slices.Contains(stops, "") {
		return nil, fmt.Errorf("%w: stop sequences must not be empty", ErrInvalidRequest)
	}

	return stops, nil
}

func validateToolChoice(d D) error {
	toolChoice, exists := d["tool_choice"]
	if !exists {
		return nil
	}

	mode, name, err := parseToolChoice(toolChoice)
	if err != nil {
		return err
	}

	if mode == "none" || mode == "auto" {
		return nil
	}

	tools := functionTools(d)
	if mode == "required" {
		if len(tools) == 0 {
			return fmt.Errorf("%w: tool_choice %q requires at least one function tool", ErrInvalidRequest, mode)
		}
		return nil
	}

	for _, tool := range tools {
		function, _ := tool["function"].(D)
		if function["name"] == name {
			return nil
		}
	}

	return fmt.Errorf("%w: tool_choice function %q does not match a declared function tool", ErrInvalidRequest, name)
}

func parseToolChoice(toolChoice any) (string, string, error) {
	switch choice := toolChoice.(type) {
	case string:
		switch choice {
		case "none", "auto", "required":
			return choice, "", nil
		default:
			return "", "", fmt.Errorf("%w: unsupported tool_choice %q", ErrInvalidRequest, choice)
		}

	case map[string]any:
		return parseToolChoice(D(choice))

	case D:
		if choice["type"] != "function" {
			return "", "", fmt.Errorf("%w: tool_choice type must be %q", ErrInvalidRequest, "function")
		}

		// Chat Completions nests the selected function under "function".
		// Responses normalizes its flat wire representation before reaching
		// this shared validator.
		value, exists := choice["function"]
		if !exists {
			return "", "", fmt.Errorf("%w: tool_choice function object is required", ErrInvalidRequest)
		}

		var name string
		switch function := value.(type) {
		case D:
			name, _ = function["name"].(string)
		case map[string]any:
			name, _ = function["name"].(string)
		default:
			return "", "", fmt.Errorf("%w: tool_choice function must be an object", ErrInvalidRequest)
		}
		if name == "" {
			return "", "", fmt.Errorf("%w: tool_choice function name is required", ErrInvalidRequest)
		}

		return "function", name, nil

	default:
		return "", "", fmt.Errorf("%w: tool_choice must be a string or function object", ErrInvalidRequest)
	}
}

func functionTools(d D) []D {
	tools, _ := d["tools"].([]D)
	functions := make([]D, 0, len(tools))
	for _, tool := range tools {
		if tool["type"] != "function" {
			continue
		}
		function, ok := tool["function"].(D)
		if !ok {
			continue
		}
		name, _ := function["name"].(string)
		if name != "" {
			functions = append(functions, tool)
		}
	}
	return functions
}

// applyToolChoice updates the cloned inference document so tool selection is
// reflected by both prompt rendering and output parsing. Validation must run
// first.
func applyToolChoice(d D) {
	mode, name, _ := parseToolChoice(d["tool_choice"])
	switch mode {
	case "none":
		delete(d, "tools")

	case "function":
		for _, tool := range functionTools(d) {
			function, _ := tool["function"].(D)
			if function["name"] == name {
				d["tools"] = []D{tool}
				d["tool_choice"] = D{
					"type":     "function",
					"function": D{"name": name},
				}
				return
			}
		}
	}
}

// ValidateMessages validates the messages field of a chat document.
func ValidateMessages(d D) error {
	messages, exists := d["messages"]
	if !exists {
		return ErrMessagesMissing
	}

	docs, ok := messages.([]D)
	if !ok {
		return ErrMessagesInvalid
	}
	if len(docs) == 0 {
		return ErrMessagesMissing
	}

	return validateMessageContentParts(docs)
}

func validateMessageContentParts(messages []D) error {
	for i, msg := range messages {
		content, exists := msg["content"]
		if !exists {
			continue
		}

		var parts []any
		switch value := content.(type) {
		case []D:
			parts = make([]any, len(value))
			for j, part := range value {
				parts[j] = part
			}
		case []map[string]any:
			parts = make([]any, len(value))
			for j, part := range value {
				parts[j] = part
			}
		case []any:
			parts = value
		default:
			continue
		}

		for j, part := range parts {
			partMap, ok := mapFromPart(part)
			if !ok {
				continue
			}
			switch partMap["type"] {
			case "file", "input_file":
				return fmt.Errorf("validate-document: messages[%d].content[%d]: %w", i, j, ErrFileInputsUnsupported)
			}
		}
	}

	return nil
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

func lifecycleStatus(err error) string {
	switch {
	case err == nil:
		return "complete"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancel"
	default:
		return "error"
	}
}

func (m *Model) sendChatError(ctx context.Context, ch chan<- ChatResponse, id string, err error) {
	m.log(ctx, "send-chat-error", "ERROR", err.Error(), "id", id)

	// I want to try and send this message before we check the context.
	select {
	case ch <- ChatResponseErr(id, ObjectChatUnknown, m.modelInfo.ID, 0, err, Usage{}):
		return
	default:
	}

	select {
	case <-ctx.Done():
		select {
		case ch <- ChatResponseErr(id, ObjectChatUnknown, m.modelInfo.ID, 0, ctx.Err(), Usage{}):
		default:
		}

	case ch <- ChatResponseErr(id, ObjectChatUnknown, m.modelInfo.ID, 0, err, Usage{}):
	}
}
