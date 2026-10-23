package model

import (
	"context"
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

// prepareDecisionMedia owns all native preparation resources. Returned rows own
// their embeddings, so queued work remains valid after cancellation or another
// encoder pass. Protocols own prompt layout and decision spans, not MTMD state.
func (m *Model) prepareDecisionMedia(ctx context.Context, prompt string, images [][]byte) ([]extendedBatchEntry, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	mctx, err := mtmd.InitFromFile(m.projFile, m.model, mtmdContextParams(m.cfg, m.projDevice))
	if err != nil {
		return nil, 0, fmt.Errorf("%w: initialize decision projector: %w", ErrDecisionRequest, err)
	}
	defer mtmd.Free(mctx)

	bitmaps := make([]mtmd.Bitmap, len(images))
	defer func() {
		for _, bitmap := range bitmaps {
			if bitmap != 0 {
				mtmd.BitmapFree(bitmap)
			}
		}
	}()
	for i, image := range images {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		bitmap, err := newImageBitmap(image)
		if err != nil {
			return nil, 0, fmt.Errorf("%w: images[%d]: %w", ErrDecisionRequest, i, err)
		}
		bitmaps[i] = bitmap
	}
	chunks := mtmd.InputChunksInit()
	defer mtmd.InputChunksFree(chunks)
	if err := tokenizeMedia(mctx, chunks, prompt, bitmaps); err != nil {
		return nil, 0, fmt.Errorf("%w: decision media prompt: %w", ErrDecisionRequest, err)
	}

	limit := min(m.decision.contextWindow, m.decision.maxTokens)
	var rows int
	for i := range mtmd.InputChunksSize(chunks) {
		rows += int(mtmd.InputChunkGetNTokens(mtmd.InputChunksGet(chunks, i)))
	}
	if rows > limit {
		return nil, 0, fmt.Errorf("%w: image prefix needs %d text/image rows, limit is %d; increase context-window or prefill-batch-size", ErrDecisionBudget, rows, limit)
	}

	batch := extendedBatch{capacity: limit}
	seq := []llama.SeqId{0}
	var position llama.Pos
	width := llama.ModelNEmbdInp(m.model)
	for i := range mtmd.InputChunksSize(chunks) {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		chunk := mtmd.InputChunksGet(chunks, i)
		switch mtmd.InputChunkGetType(chunk) {
		case mtmd.InputChunkTypeText:
			for _, token := range mtmd.InputChunkGetTokensText(chunk) {
				if _, err := batch.addToken(token, position, seq, extendedBatchOutputNone); err != nil {
					return nil, 0, err
				}
				position++
			}
		case mtmd.InputChunkTypeImage:
			if err := mtmd.EncodeChunk(mctx, chunk); err != nil {
				return nil, 0, fmt.Errorf("encode decision image: %w", err)
			}
			count := int32(mtmd.InputChunkGetNTokens(chunk))
			embeddings, err := mtmd.GetOutputEmbd(mctx, width*count)
			if err != nil {
				return nil, 0, fmt.Errorf("decision image embeddings: %w", err)
			}
			for j := range count {
				p := position + llama.Pos(j)
				positions := []llama.Pos{p, p, p, p}
				if mtmd.DecodeUseMRope(mctx) {
					rel := mtmd.ImageTokensGetDecoderPos(mtmd.InputChunkGetTokensImage(chunk), position, uint64(j))
					positions = []llama.Pos{llama.Pos(rel.T), llama.Pos(rel.Y), llama.Pos(rel.X), llama.Pos(rel.Z)}
				}
				if _, err := batch.addEmbedding(embeddings[j*width:(j+1)*width], int(width), positions, seq, extendedBatchOutputNone); err != nil {
					return nil, 0, err
				}
			}
			if mtmd.DecodeUseMRope(mctx) {
				position += mtmd.InputChunkGetNPos(chunk)
			} else {
				position += llama.Pos(count)
			}
		default:
			return nil, 0, fmt.Errorf("%w: decision media supports images only", ErrDecisionRequest)
		}
	}
	return batch.entries, int(position), ctx.Err()
}
