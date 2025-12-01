package model

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

// Vision performs a vision request and returns the final response.
func (m *Model) Vision(ctx context.Context, image []byte, params Params, d D) (ChatResponse, error) {
	ch := m.VisionStreaming(ctx, image, params, d)

	var lastMsg ChatResponse
	for msg := range ch {
		lastMsg = msg
	}

	return lastMsg, nil
}

// VisionStreaming performs a vision request and streams the response.
func (m *Model) VisionStreaming(ctx context.Context, image []byte, params Params, d D) <-chan ChatResponse {
	m.activeStreams.Add(1)
	defer m.activeStreams.Add(-1)

	ch := make(chan ChatResponse)

	go func() {
		id := uuid.New().String()

		defer func() {
			if rec := recover(); rec != nil {
				m.sendVisionError(ctx, ch, id, fmt.Errorf("%v", rec))
			}
			close(ch)
		}()

		if m.projFile == "" {
			m.sendVisionError(ctx, ch, id, fmt.Errorf("projection file not set"))
			return
		}

		lctx, err := llama.InitFromModel(m.model, m.ctxParams)
		if err != nil {
			m.sendVisionError(ctx, ch, id, fmt.Errorf("unable to init from model: %w", err))
			return
		}

		defer func() {
			llama.Synchronize(lctx)
			llama.Free(lctx)
		}()

		mctxParams := mtmd.ContextParamsDefault()
		mctxParams.UseGPU = true
		mctxParams.FlashAttentionType = llama.FlashAttentionTypeAuto

		mtmdCtx, err := mtmd.InitFromFile(m.projFile, m.model, mctxParams)
		if err != nil {
			m.sendVisionError(ctx, ch, id, fmt.Errorf("unable to init from model: %w", err))
			return
		}
		defer mtmd.Free(mtmdCtx)

		prompt, err := m.applyVisionRequestJinjaTemplate(d)
		if err != nil {
			m.sendVisionError(ctx, ch, id, err)
			return
		}

		bitmap, err := m.processBitmap(lctx, mtmdCtx, image, prompt)
		if err != nil {
			m.sendVisionError(ctx, ch, id, err)
			return
		}
		defer mtmd.BitmapFree(bitmap)

		m.processTokens(ctx, id, lctx, ObjectVision, prompt, params, ch)
	}()

	return ch
}

func (m *Model) processBitmap(lctx llama.Context, mtmdCtx mtmd.Context, image []byte, prompt string) (mtmd.Bitmap, error) {
	bitmap := mtmd.BitmapInitFromBuf(mtmdCtx, &image[0], uint64(len(image)))
	output := mtmd.InputChunksInit()
	input := mtmd.NewInputText(prompt, true, true)

	mtmd.Tokenize(mtmdCtx, output, input, []mtmd.Bitmap{bitmap})

	var n llama.Pos
	mtmd.HelperEvalChunks(mtmdCtx, lctx, output, 0, 0, int32(m.ctxParams.NBatch), true, &n)

	return bitmap, nil
}

func (m *Model) sendVisionError(ctx context.Context, ch chan<- ChatResponse, id string, err error) {
	// I want to try and send this message before we check the context.
	select {
	case ch <- ChatResponseErr(id, ObjectVision, m.modelInfo.Name, 0, err, Usage{}):
		return
	default:
	}

	select {
	case <-ctx.Done():
		select {
		case ch <- ChatResponseErr(id, ObjectVision, m.modelInfo.Name, 0, ctx.Err(), Usage{}):
		default:
		}

	case ch <- ChatResponseErr(id, ObjectVision, m.modelInfo.Name, 0, err, Usage{}):
	}
}
