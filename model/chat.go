package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

// Chat performs a chat request and returns the final response.
func (m *Model) Chat(ctx context.Context, d D) (ChatResponse, error) {
	ch := m.ChatStreaming(ctx, d)

	var lastMsg ChatResponse
	for msg := range ch {
		lastMsg = msg
	}

	return lastMsg, nil
}

// ChatStreaming performs a chat request and streams the response.
func (m *Model) ChatStreaming(ctx context.Context, d D) <-chan ChatResponse {
	ch := make(chan ChatResponse)

	go func() {
		m.activeStreams.Add(1)
		defer m.activeStreams.Add(-1)

		id := uuid.New().String()

		defer func() {
			if rec := recover(); rec != nil {
				m.sendChatError(ctx, ch, id, fmt.Errorf("%v", rec))
			}
			close(ch)
		}()

		params, err := m.validateDocument(d)
		if err != nil {
			m.sendChatError(ctx, ch, "", err)
			return
		}

		lctx, err := llama.InitFromModel(m.model, m.ctxParams)
		if err != nil {
			m.sendChatError(ctx, ch, id, fmt.Errorf("unable to init model: %w", err))
			return
		}

		defer func() {
			llama.Synchronize(lctx)
			llama.Free(lctx)
		}()

		var mtmdCtx mtmd.Context

		if m.projFile != "" {
			mctxParams := mtmd.ContextParamsDefault()

			mtmdCtx, err = mtmd.InitFromFile(m.projFile, m.model, mctxParams)
			if err != nil {
				m.sendChatError(ctx, ch, id, fmt.Errorf("unable to init projection: %w", err))
				return
			}
			defer mtmd.Free(mtmdCtx)
		}

		prompt, media, err := m.applyRequestJinjaTemplate(d)
		if err != nil {
			m.sendChatError(ctx, ch, id, fmt.Errorf("unable to apply jinja template: %w", err))
			return
		}

		object := ObjectChatText

		if len(media) > 0 {
			object = ObjectChatMedia

			bitmap, err := m.processBitmap(lctx, mtmdCtx, prompt, media)
			if err != nil {
				m.sendChatError(ctx, ch, id, err)
				return
			}

			defer func() {
				for _, b := range bitmap {
					mtmd.BitmapFree(b)
				}
			}()
		}

		m.processChatRequest(ctx, id, lctx, object, prompt, params, ch)
	}()

	return ch
}

func (m *Model) validateDocument(d D) (Params, error) {
	messages, exists := d["messages"]
	if !exists {
		return Params{}, errors.New("no messages found in request")
	}

	if _, ok := messages.([]D); !ok {
		return Params{}, errors.New("messages is not a slice of documents")
	}

	params, err := parseParams(d)
	if err != nil {
		return Params{}, err
	}

	return params, nil
}

func (m *Model) processBitmap(lctx llama.Context, mtmdCtx mtmd.Context, prompt string, media [][]byte) ([]mtmd.Bitmap, error) {
	bitmaps := make([]mtmd.Bitmap, len(media))
	for i, med := range media {
		bitmaps[i] = mtmd.BitmapInitFromBuf(mtmdCtx, &med[0], uint64(len(med)))
	}

	output := mtmd.InputChunksInit()
	input := mtmd.NewInputText(prompt, true, true)

	mtmd.Tokenize(mtmdCtx, output, input, bitmaps)

	var n llama.Pos
	mtmd.HelperEvalChunks(mtmdCtx, lctx, output, 0, 0, int32(m.ctxParams.NBatch), true, &n)

	return bitmaps, nil
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
