package model

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var errIMCMediaNativePrefix = errors.New("mtmd native prefix diverged")

// targetDecodeHandoff consumes a decoded batch while decodeMu is held and its
// borrowed embeddings/hidden states are valid. Media continuation positions
// come from mtmd, not from the number of physical rows.
type targetDecodeHandoff func(batch *extendedBatch, nextPosition llama.Pos) error

const (
	imcMediaChunkText uint8 = iota + 1
	imcMediaChunkImage
	imcMediaChunkAudio
)

// imcMediaChunk records mtmd's authoritative native chunk stream. Logical
// prompt-plan media markers cannot be used here because one marker may expand
// into several native chunks and mtmd may inject text tokens around them.
type imcMediaChunk struct {
	kind    uint8
	tokens  []llama.Token
	nTokens int
	nPos    int
}

type imcMediaPrefixCursor struct {
	chunks      []imcMediaChunk
	chunkOffset int
	textOffset  int
}

func (c *imcMediaPrefixCursor) consumeText(tokens []llama.Token) ([]llama.Token, int, error) {
	skipped := 0
	for len(tokens) > 0 && !c.done() {
		cached := c.chunks[c.chunkOffset]
		if cached.kind != imcMediaChunkText {
			return nil, skipped, fmt.Errorf("%w at chunk %d: got text, want media", errIMCMediaNativePrefix, c.chunkOffset)
		}

		remaining := cached.tokens[c.textOffset:]
		n := min(len(tokens), len(remaining))
		if !slices.Equal(tokens[:n], remaining[:n]) {
			return nil, skipped, fmt.Errorf("%w at text chunk %d token %d", errIMCMediaNativePrefix, c.chunkOffset, c.textOffset)
		}

		tokens = tokens[n:]
		skipped += n
		c.textOffset += n
		if c.textOffset == len(cached.tokens) {
			c.chunkOffset++
			c.textOffset = 0
			c.skipEmptyText()
		}
	}

	return tokens, skipped, nil
}

func (c *imcMediaPrefixCursor) consumeMedia(chunk imcMediaChunk) (bool, error) {
	c.skipEmptyText()
	if c.done() {
		return false, nil
	}

	cached := c.chunks[c.chunkOffset]
	if c.textOffset != 0 || cached.kind != chunk.kind || cached.nTokens != chunk.nTokens || cached.nPos != chunk.nPos {
		return false, fmt.Errorf("%w at media chunk %d", errIMCMediaNativePrefix, c.chunkOffset)
	}

	c.chunkOffset++
	c.skipEmptyText()
	return true, nil
}

func (c *imcMediaPrefixCursor) done() bool {
	c.skipEmptyText()
	return c.chunkOffset == len(c.chunks)
}

func (c *imcMediaPrefixCursor) skipEmptyText() {
	for c.chunkOffset < len(c.chunks) && c.chunks[c.chunkOffset].kind == imcMediaChunkText && len(c.chunks[c.chunkOffset].tokens) == 0 {
		c.chunkOffset++
	}
}

// imcMediaPreparation owns the native stream and accounting across preparation
// turns. Its chunks are separate from ordinary suffix prefill so only the IMC
// scheduler can advance them. position is logical; physicalKVCells counts rows.
type imcMediaPreparation struct {
	ctx                 context.Context
	span                trace.Span
	bitmaps             []mtmd.Bitmap
	inputChunks         mtmd.InputChunks
	chunkIdx            uint64
	textPrepared        bool
	textRemaining       []llama.Token
	prefixCursor        imcMediaPrefixCursor
	position            int
	physicalKVCells     int
	totalPhysicalTokens int
	mediaKVCounts       []int
	samplerPromptTokens []llama.Token
	nativeChunks        []imcMediaChunk
}

func (p *imcMediaPreparation) free() {
	if p.inputChunks != 0 {
		mtmd.InputChunksFree(p.inputChunks)
		p.inputChunks = 0
	}
	for _, b := range p.bitmaps {
		if b != 0 {
			mtmd.BitmapFree(b)
		}
	}
	p.bitmaps = nil
	p.span.End()
}

// newIMCMediaPreparation tokenizes the cache document but does not encode or
// prefill it. The caller owns the returned native resources until completion
// or slot teardown.
func (m *Model) newIMCMediaPreparation(ctx context.Context, cacheD D, prefix []imcMediaChunk, seqID llama.SeqId, mtmdCtx mtmd.Context, startPos int) (*imcMediaPreparation, error) {
	ctx, span := otel.AddSpan(ctx, "imc-media-cache-build",
		attribute.Int("seq", int(seqID)),
	)
	p := imcMediaPreparation{ctx: ctx, span: span, position: startPos,
		prefixCursor: imcMediaPrefixCursor{chunks: prefix}}
	ready := false
	defer func() {
		if !ready {
			p.free()
		}
	}()

	// Step 1: Create prompt and extract media bytes from the cache document.
	prompt, media, err := m.createPrompt(ctx, cacheD)
	if err != nil {
		return nil, fmt.Errorf("imc-media-cache: unable to create prompt: %w", err)
	}

	m.log(ctx, "imc-media-cache", "status", "prompt-created", "seq", seqID,
		"prompt_len", len(prompt), "media_count", len(media))

	// Step 2: Create bitmaps from raw media bytes. Images are decoded in Go
	// (newMediaBitmap) and built via the stable mtmd_bitmap_init core API;
	// audio still goes through the mtmd-helper. Reject any payload that fails
	// to decode so we surface a precise error instead of a generic tokenization
	// failure.
	p.bitmaps = make([]mtmd.Bitmap, len(media))
	for i, med := range media {
		if len(med) == 0 {
			return nil, fmt.Errorf("imc-media-cache: media[%d] is empty", i)
		}
		bmp, err := newMediaBitmap(mtmdCtx, med)
		if err != nil {
			return nil, fmt.Errorf("imc-media-cache: media[%d]: %w", i, err)
		}
		p.bitmaps[i] = bmp
	}

	// Step 3: Tokenize the rendered prompt as explicit ordered text and bitmap
	// parts, producing the same interleaved chunk stream used for generation.
	p.inputChunks = mtmd.InputChunksInit()

	if err := tokenizeMedia(mtmdCtx, p.inputChunks, prompt, p.bitmaps); err != nil {
		return nil, fmt.Errorf("imc-media-cache: %w", err)
	}

	useMRoPE := mtmd.DecodeUseMRope(mtmdCtx)
	useNonCausal := mtmd.DecodeUseNonCausal(mtmdCtx, 0)

	numChunks := mtmd.InputChunksSize(p.inputChunks)
	for i := range numChunks {
		p.totalPhysicalTokens += int(mtmd.InputChunkGetNTokens(mtmd.InputChunksGet(p.inputChunks, i)))
	}

	m.log(ctx, "imc-media-cache", "status", "tokenized", "seq", seqID,
		"num_chunks", numChunks, "use_mrope", useMRoPE, "use_noncausal", useNonCausal)

	ready = true
	return &p, nil
}

// advanceIMCMediaChunk processes one complete media chunk or one bounded text
// slice. An image's internal decode batches remain indivisible to scheduling.
func (m *Model) advanceIMCMediaChunk(p *imcMediaPreparation, seqID llama.SeqId, mtmdCtx mtmd.Context, handoff targetDecodeHandoff) (bool, error) {
	if err := p.ctx.Err(); err != nil {
		return false, err
	}
	numChunks := mtmd.InputChunksSize(p.inputChunks)
	if p.chunkIdx >= numChunks {
		if !p.prefixCursor.done() {
			return false, fmt.Errorf("%w: current stream ended first", errIMCMediaNativePrefix)
		}
		return true, nil
	}
	chunk := mtmd.InputChunksGet(p.inputChunks, p.chunkIdx)
	chunkType := mtmd.InputChunkGetType(chunk)
	nTokens := int32(mtmd.InputChunkGetNTokens(chunk))
	nPos := int(mtmd.InputChunkGetNPos(chunk))
	useMRoPE := mtmd.DecodeUseMRope(mtmdCtx)
	useNonCausal := mtmd.DecodeUseNonCausal(mtmdCtx, 0)

	switch chunkType {
	case mtmd.InputChunkTypeText:
		if !p.textPrepared {
			tokens := mtmd.InputChunkGetTokensText(chunk)
			p.nativeChunks = append(p.nativeChunks, imcMediaChunk{kind: imcMediaChunkText, tokens: slices.Clone(tokens)})
			p.samplerPromptTokens = append(p.samplerPromptTokens, tokens...)
			remaining, _, err := p.prefixCursor.consumeText(tokens)
			if err != nil {
				return false, err
			}
			p.textRemaining = remaining
			p.textPrepared = true
		}
		n := min(len(p.textRemaining), m.cfg.PrefillBatchSize())
		if n > 0 {
			if _, err := m.decodeTextMRoPEIntoCache(p.textRemaining[:n], seqID, p.position, handoff); err != nil {
				return false, fmt.Errorf("imc-media-cache: text chunk %d: %w", p.chunkIdx, err)
			}
			p.position += n
			p.physicalKVCells += n
			p.textRemaining = p.textRemaining[n:]
		}
		if len(p.textRemaining) == 0 {
			p.textPrepared = false
			p.chunkIdx++
		}

	case mtmd.InputChunkTypeImage, mtmd.InputChunkTypeAudio:
		kind := imcMediaChunkImage
		if chunkType == mtmd.InputChunkTypeAudio {
			kind = imcMediaChunkAudio
		}
		nativeChunk := imcMediaChunk{kind: kind, nTokens: int(nTokens), nPos: nPos}
		cached, err := p.prefixCursor.consumeMedia(nativeChunk)
		if err != nil {
			return false, err
		}
		if !cached {
			m.log(p.ctx, "imc-media-cache", "status", "encoding-media-chunk", "seq", seqID,
				"chunk", p.chunkIdx, "kind", kind, "tokens", nTokens, "pos", p.position)
			encodeStarted := time.Now()
			if err := mtmd.EncodeChunk(mtmdCtx, chunk); err != nil {
				return false, fmt.Errorf("imc-media-cache: encode media chunk %d: %w", p.chunkIdx, err)
			}
			m.log(p.ctx, "imc-media-cache", "status", "encoded-media-chunk", "seq", seqID,
				"chunk", p.chunkIdx, "tokens", nTokens, "elapsed", fmtDur(time.Since(encodeStarted)))
			nEmbd := llama.ModelNEmbdInp(m.model)
			embd, err := mtmd.GetOutputEmbd(mtmdCtx, nEmbd*nTokens)
			if err != nil {
				return false, fmt.Errorf("imc-media-cache: get media embeddings chunk %d: %w", p.chunkIdx, err)
			}
			m.log(p.ctx, "imc-media-cache", "status", "decoding-media-chunk", "seq", seqID,
				"chunk", p.chunkIdx, "tokens", nTokens, "pos", p.position,
				"use_mrope", useMRoPE, "use_noncausal", useNonCausal)
			decodeStarted := time.Now()
			var decoded int
			if useMRoPE {
				positions := linearMRoPEPositions(nTokens, llama.Pos(p.position))
				if chunkType == mtmd.InputChunkTypeImage {
					positions, err = imageTokensDecoderPositions(mtmd.InputChunkGetTokensImage(chunk), llama.Pos(p.position), nTokens)
					if err != nil {
						return false, err
					}
				}
				decoded, err = m.decodeEmbeddingsMRoPEIntoCache(embd, nEmbd, nTokens, positions, seqID, useNonCausal, llama.Pos(p.position+nPos), handoff)
			} else {
				decoded, err = m.decodeEmbeddingsIntoCache(embd, nEmbd, nTokens, seqID, p.position, useNonCausal, handoff)
			}
			if err != nil {
				return false, fmt.Errorf("imc-media-cache: decode media chunk %d: %w", p.chunkIdx, err)
			}
			if useMRoPE {
				p.position += nPos
			} else {
				p.position += decoded
			}
			p.physicalKVCells += decoded
			p.mediaKVCounts = append(p.mediaKVCounts, decoded)
			m.log(p.ctx, "imc-media-cache", "status", "decoded-media-chunk", "seq", seqID,
				"chunk", p.chunkIdx, "decoded_tokens", decoded,
				"next_logical_position", p.position, "elapsed", fmtDur(time.Since(decodeStarted)))
		}
		p.nativeChunks = append(p.nativeChunks, nativeChunk)
		p.chunkIdx++

	default:
		return false, fmt.Errorf("imc-media-cache: unsupported chunk type %d", chunkType)
	}
	// Completion is a separate turn: generation can advance after the last
	// chunk before snapshotting and suffix setup occupy the engine again.
	return false, nil
}

// decodeEmbeddingsIntoCache decodes embeddings into a KV cache sequence with
// standard linear positioning. Returns the number of KV positions consumed.
func (m *Model) decodeEmbeddingsIntoCache(embd []float32, nEmbd, nTokens int32, seqID llama.SeqId, startPos int, useNonCausal bool, handoff targetDecodeHandoff) (int, error) {
	if nTokens == 0 {
		return 0, nil
	}
	if err := m.validateMediaBatchCapacity(nTokens, useNonCausal); err != nil {
		return 0, err
	}

	nBatch := m.cfg.EffectiveNBatch()
	if nBatch <= 0 {
		nBatch = 512
	}
	batch, err := newExtendedBatchCapacity(m.lctx, min(nBatch, int(nTokens)))
	if err != nil {
		return 0, fmt.Errorf("imc media cache: create embedding batch: %w", err)
	}
	defer batch.free()

	m.decodeMu.Lock()
	defer m.decodeMu.Unlock()

	if useNonCausal {
		wasCausal := llama.GetCausalAttn(m.lctx)
		llama.SetCausalAttn(m.lctx, false)
		defer llama.SetCausalAttn(m.lctx, wasCausal)
	}

	pos := startPos
	sequenceIDs := []llama.SeqId{seqID}

	for start := 0; start < int(nTokens); start += nBatch {
		end := min(start+nBatch, int(nTokens))
		batchN := end - start

		positions := linearPositions(int32(batchN), llama.Pos(pos))
		if err := stageEmbeddingRows(batch, embd[start*int(nEmbd):end*int(nEmbd)], nEmbd, int32(batchN), positions, sequenceIDs, extendedBatchOutputNone); err != nil {
			return 0, fmt.Errorf("imc media cache: stage embedding rows at position %d: %w", pos, err)
		}

		ret, err := batch.process(llama.ProcessTypeDecode)
		if err == nil && ret == 0 {
			llama.Synchronize(m.lctx)
		}

		if err != nil || ret != 0 {
			return 0, decodeError(ret, err)
		}

		pos += int(batchN)
		if handoff != nil {
			if err := handoff(batch, llama.Pos(pos)); err != nil {
				return 0, err
			}
		}
	}

	return int(nTokens), nil
}

// decodeEmbeddingsMRoPEIntoCache decodes embeddings with M-RoPE positioning
// into a KV cache sequence. Returns the number of physical KV cells consumed.
func (m *Model) decodeEmbeddingsMRoPEIntoCache(embd []float32, nEmbd, nTokens int32, positions []llama.Pos, seqID llama.SeqId, useNonCausal bool, nextPosition llama.Pos, handoff targetDecodeHandoff) (int, error) {
	if len(positions) != int(nTokens*4) {
		return 0, fmt.Errorf("mrope embedding positions: got %d, want %d", len(positions), nTokens*4)
	}
	if nTokens == 0 {
		return 0, nil
	}
	if err := m.validateMediaBatchCapacity(nTokens, useNonCausal); err != nil {
		return 0, err
	}

	nBatch := m.cfg.EffectiveNBatch()
	if nBatch <= 0 {
		nBatch = 512
	}
	batch, err := newExtendedBatchCapacity(m.lctx, min(nBatch, int(nTokens)))
	if err != nil {
		return 0, fmt.Errorf("imc media cache: create M-RoPE embedding batch: %w", err)
	}
	defer batch.free()

	m.decodeMu.Lock()
	defer m.decodeMu.Unlock()

	if useNonCausal {
		wasCausal := llama.GetCausalAttn(m.lctx)
		llama.SetCausalAttn(m.lctx, false)
		defer llama.SetCausalAttn(m.lctx, wasCausal)
	}

	sequenceIDs := []llama.SeqId{seqID}
	for start := 0; start < int(nTokens); start += nBatch {
		end := min(start+nBatch, int(nTokens))
		batchN := end - start

		// Build sub-batch position array by gathering from the full array.
		// Extended-batch staging accepts the same four section-major planes.
		subPosData := make([]llama.Pos, batchN*4)
		for i := range batchN {
			subPosData[i] = positions[start+i]
			subPosData[i+batchN] = positions[start+i+int(nTokens)]
			subPosData[i+batchN*2] = positions[start+i+int(nTokens)*2]
			subPosData[i+batchN*3] = positions[start+i+int(nTokens)*3]
		}
		if err := stageEmbeddingRows(batch, embd[start*int(nEmbd):end*int(nEmbd)], nEmbd, int32(batchN), subPosData, sequenceIDs, extendedBatchOutputNone); err != nil {
			return 0, fmt.Errorf("imc media cache: stage M-RoPE embedding rows at row %d: %w", start, err)
		}

		ret, err := batch.process(llama.ProcessTypeDecode)
		if err == nil && ret == 0 {
			llama.Synchronize(m.lctx)
		}

		if err != nil || ret != 0 {
			return 0, decodeError(ret, err)
		}
		if handoff != nil {
			// The image's logical end is authoritative. No generation can
			// run between slices; carry hidden state across every slice.
			if err := handoff(batch, nextPosition); err != nil {
				return 0, err
			}
		}
	}

	return int(nTokens), nil
}

// decodeTextMRoPEIntoCache decodes text tokens with M-RoPE 4D positioning
// into a KV cache sequence. Returns the number of physical KV cells consumed;
// the caller advances its logical position with mtmd.InputChunkGetNPos.
func (m *Model) decodeTextMRoPEIntoCache(tokens []llama.Token, seqID llama.SeqId, startPos int, handoff targetDecodeHandoff) (int, error) {
	n := len(tokens)
	if n == 0 {
		return 0, nil
	}

	nBatch := m.cfg.EffectiveNBatch()
	if nBatch <= 0 {
		nBatch = 512
	}
	batch, err := newExtendedBatchCapacity(m.lctx, min(nBatch, n))
	if err != nil {
		return 0, fmt.Errorf("imc media cache: create M-RoPE text batch: %w", err)
	}
	defer batch.free()

	m.decodeMu.Lock()
	defer m.decodeMu.Unlock()

	pos := startPos
	sequenceIDs := []llama.SeqId{seqID}

	for start := 0; start < n; start += nBatch {
		end := min(start+nBatch, n)
		if err := stageMRoPEText(batch, tokens[start:end], llama.Pos(pos), sequenceIDs, extendedBatchOutputNone); err != nil {
			return 0, fmt.Errorf("imc media cache: stage M-RoPE text at position %d: %w", pos, err)
		}

		ret, err := batch.process(llama.ProcessTypeDecode)
		if err == nil && ret == 0 {
			llama.Synchronize(m.lctx)
		}

		if err != nil || ret != 0 {
			return 0, decodeError(ret, err)
		}

		pos += end - start
		if handoff != nil {
			if err := handoff(batch, llama.Pos(pos)); err != nil {
				return 0, err
			}
		}
	}

	return n, nil
}
