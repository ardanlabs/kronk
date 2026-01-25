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
	"github.com/hybridgroup/yzma/pkg/llama"
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
	ch := make(chan ChatResponse, 1)

	go func() {
		active := m.activeStreams.Add(1)

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

		params, err := m.validateDocument(d)
		if err != nil {
			m.sendChatError(ctx, ch, id, err)
			return
		}

		d, object, mtmdCtx, err := m.prepareMediaContext(ctx, d)
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

		// fmt.Println("=======================================")
		// messages, _ := d["messages"].([]D)
		// for _, m := range messages {
		// 	fmt.Println("[DEBUG]: Role:", m["role"])
		// 	fmt.Println("[DEBUG]: Content:", m["content"])
		// 	fmt.Println("---------------------------------------")
		// }
		// fmt.Println("=======================================")

		var sysPromptNPast llama.Pos
		var sysPromptCached bool
		var prompt string
		var media [][]byte

		if (m.cfg.SystemPromptCache || m.cfg.FirstMessageCache) && object == ObjectChatText {
			cache := m.ensureFirstMessageCached(ctx, d)
			if cache.err != nil {
				m.sendChatError(ctx, ch, id, cache.err)
				return
			}

			d = cache.modifiedD
			sysPromptNPast = cache.nPast
			sysPromptCached = cache.cached
			prompt = cache.prompt
			media = cache.media
		}

		// Only call createPrompt if caching didn't already handle it.
		if prompt == "" {
			prompt, media, err = m.createPrompt(ctx, d)
			if err != nil {
				m.sendChatError(ctx, ch, id, fmt.Errorf("create-streaming: unable to apply jinja template: %w", err))
				return
			}
		}

		// ---------------------------------------------------------------------

		// Use batch engine for text-only requests when available.
		if m.batch != nil && object == ObjectChatText {
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
				sysPromptNPast:  sysPromptNPast,
				sysPromptCached: sysPromptCached,
			}

			// Engine manages activeStreams for submitted jobs.
			if err := m.batch.submit(&job); err != nil {
				m.sendChatError(ctx, ch, id, err)
				return
			}

			batching = true

			// Channel closed and activeStreams decremented by
			// engine when job completes.
			return
		}

		// ---------------------------------------------------------------------

		// Sequential path for media requests or when engine is not available.

		m.sequentialChatRequest(ctx, id, m.lctx, mtmdCtx, object, prompt, media, params, ch)
	}()

	return ch
}

func (m *Model) prepareMediaContext(ctx context.Context, d D) (D, string, mtmd.Context, error) {
	mediaType, isOpenAIFormat, msgs, err := detectMediaContent(d)
	if err != nil {
		return nil, "", 0, fmt.Errorf("prepare-media-context: %w", err)
	}

	if mediaType != MediaTypeNone && m.projFile == "" {
		return nil, "", 0, fmt.Errorf("prepare-media-context: media detected in request but model does not support media processing")
	}

	var mtmdCtx mtmd.Context
	object := ObjectChatText

	if m.projFile != "" {
		object = ObjectChatMedia

		mtmdCtx, err = m.loadProjFile(ctx)
		if err != nil {
			return nil, "", 0, fmt.Errorf("prepare-media-context: unable to init projection: %w", err)
		}

		switch mediaType {
		case MediaTypeVision:
			if !mtmd.SupportVision(mtmdCtx) {
				mtmd.Free(mtmdCtx)
				return nil, "", 0, fmt.Errorf("prepare-media-context: image/video detected but model does not support vision")
			}

		case MediaTypeAudio:
			if !mtmd.SupportAudio(mtmdCtx) {
				mtmd.Free(mtmdCtx)
				return nil, "", 0, fmt.Errorf("prepare-media-context: audio detected but model does not support audio")
			}
		}
	}

	switch {
	case isOpenAIFormat:
		d, err = convertToRawMediaMessage(d.Clone(), msgs)
		if err != nil {
			return nil, "", 0, fmt.Errorf("prepare-media-context: unable to convert document to media message: %w", err)
		}

	case mediaType != MediaTypeNone:
		d = convertPlainBase64ToBytes(d)
	}

	return d, object, mtmdCtx, nil
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

func (m *Model) validateDocument(d D) (params, error) {
	messages, exists := d["messages"]
	if !exists {
		return params{}, errors.New("validate-document: no messages found in request")
	}

	if _, ok := messages.([]D); !ok {
		return params{}, errors.New("validate-document: messages is not a slice of documents")
	}

	p, err := m.parseParams(d)
	if err != nil {
		return params{}, err
	}

	return p, nil
}

func (m *Model) sendChatError(ctx context.Context, ch chan<- ChatResponse, id string, err error) {
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
