package kronk

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

func (m *model) vision(ctx context.Context, message ChatMessage, imageFile string, params Params) (string, error) {
	ch := m.visionStreaming(ctx, message, imageFile, params)

	var finalResponse strings.Builder

	for msg := range ch {
		if msg.Err != nil {
			return "", fmt.Errorf("error from model: %w", msg.Err)
		}

		finalResponse.WriteString(msg.Response)
	}

	return finalResponse.String(), nil
}

func (m *model) visionStreaming(ctx context.Context, message ChatMessage, imageFile string, params Params) <-chan ChatResponse {
	ch := make(chan ChatResponse)

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				ch <- ChatResponse{Err: fmt.Errorf("%v", rec)}
			}
			close(ch)
		}()

		if m.projFile == "" {
			ch <- ChatResponse{Err: fmt.Errorf("projection file not set")}
			return
		}

		lctx, err := llama.InitFromModel(m.model, m.ctxParams)
		if err != nil {
			ch <- ChatResponse{Err: fmt.Errorf("unable to init from model: %w", err)}
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
			ch <- ChatResponse{Err: fmt.Errorf("unable to init from model: %w", err)}
			return
		}
		defer mtmd.Free(mtmdCtx)

		prompt := m.applyVisionTemplate(message)

		bitmap, err := m.processBitmap(lctx, mtmdCtx, imageFile, prompt)
		if err != nil {
			ch <- ChatResponse{Err: err}
			return
		}
		defer mtmd.BitmapFree(bitmap)

		m.processTokens(ctx, lctx, modeVision, prompt, params, ch)
	}()

	return ch
}

func (m *model) applyVisionTemplate(message ChatMessage) string {
	msgs := []llama.ChatMessage{
		llama.NewChatMessage(message.Role, message.Content),
		llama.NewChatMessage("user", mtmd.DefaultMarker()),
	}

	buf := make([]byte, m.cfg.ContextWindow)
	l := llama.ChatApplyTemplate(m.template, msgs, true, buf)

	return string(buf[:l])
}

func (m *model) processBitmap(lctx llama.Context, mtmdCtx mtmd.Context, imageFile string, prompt string) (mtmd.Bitmap, error) {
	if _, err := os.Stat(imageFile); err != nil {
		return 0, fmt.Errorf("error accessing file %q: %w", imageFile, err)
	}

	bitmap := mtmd.BitmapInitFromFile(mtmdCtx, imageFile)
	output := mtmd.InputChunksInit()
	input := mtmd.NewInputText(prompt, true, true)

	mtmd.Tokenize(mtmdCtx, output, input, []mtmd.Bitmap{bitmap})

	var n llama.Pos
	mtmd.HelperEvalChunks(mtmdCtx, lctx, output, 0, 0, int32(m.ctxParams.NBatch), true, &n)

	return bitmap, nil
}
