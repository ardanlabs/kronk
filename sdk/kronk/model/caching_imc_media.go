package model

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"go.opentelemetry.io/otel/attribute"
)

var errIMCMediaNativePrefix = errors.New("mtmd native prefix diverged")

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

// decodeMediaIntoCache decodes a document containing text and media (images/audio)
// into a KV cache sequence using the mtmd pipeline. This is used by IMC media
// cache builds to populate the slot's KV cache with the full multi-modal prefix.
//
// The passed-in mtmdCtx is reused from job.mtmdCtx to avoid loading the
// projection file twice. Returns the next logical position, total physical KV
// cells, physical KV cells consumed per media chunk, and the authoritative
// mtmd text tokens decoded around the media embedding chunks.
func (m *Model) decodeMediaIntoCache(ctx context.Context, cacheD D, seqID llama.SeqId, mtmdCtx mtmd.Context) (int, int, []int, []llama.Token, []imcMediaChunk, error) {
	return m.decodeMediaIntoCacheFromPlan(ctx, cacheD, nil, seqID, mtmdCtx, 0)
}

// decodeMediaIntoCacheFromPlan verifies the current mtmd-native stream against
// a cached native prefix and decodes only the appended chunks. An empty prefix
// performs a full media cache build.
func (m *Model) decodeMediaIntoCacheFromPlan(ctx context.Context, cacheD D, prefix []imcMediaChunk, seqID llama.SeqId, mtmdCtx mtmd.Context, startPos int) (int, int, []int, []llama.Token, []imcMediaChunk, error) {
	ctx, span := otel.AddSpan(ctx, "imc-media-cache-build",
		attribute.Int("seq", int(seqID)),
	)
	defer span.End()

	// Step 1: Create prompt and extract media bytes from the cache document.
	prompt, media, err := m.createPrompt(ctx, cacheD)
	if err != nil {
		return 0, 0, nil, nil, nil, fmt.Errorf("imc-media-cache: unable to create prompt: %w", err)
	}

	m.log(ctx, "imc-media-cache", "status", "prompt-created", "seq", seqID,
		"prompt_len", len(prompt), "media_count", len(media))

	// Step 2: Create bitmaps from raw media bytes. Images are decoded in Go
	// (newMediaBitmap) and built via the stable mtmd_bitmap_init core API;
	// audio still goes through the mtmd-helper. Reject any payload that fails
	// to decode so we surface a precise error instead of a generic tokenization
	// failure.
	bitmaps := make([]mtmd.Bitmap, len(media))
	defer func() {
		for _, b := range bitmaps {
			if b != 0 {
				mtmd.BitmapFree(b)
			}
		}
	}()
	for i, med := range media {
		if len(med) == 0 {
			return 0, 0, nil, nil, nil, fmt.Errorf("imc-media-cache: media[%d] is empty", i)
		}
		bmp, err := newMediaBitmap(mtmdCtx, med)
		if err != nil {
			return 0, 0, nil, nil, nil, fmt.Errorf("imc-media-cache: media[%d]: %w", i, err)
		}
		bitmaps[i] = bmp
	}

	// Step 3: Tokenize the rendered prompt as explicit ordered text and bitmap
	// parts, producing the same interleaved chunk stream used for generation.
	inputChunks := mtmd.InputChunksInit()
	defer mtmd.InputChunksFree(inputChunks)

	if err := tokenizeMedia(mtmdCtx, inputChunks, prompt, bitmaps); err != nil {
		return 0, 0, nil, nil, nil, fmt.Errorf("imc-media-cache: %w", err)
	}

	useMRoPE := mtmd.DecodeUseMRope(mtmdCtx)
	useNonCausal := mtmd.DecodeUseNonCausal(mtmdCtx, 0)

	numChunks := mtmd.InputChunksSize(inputChunks)

	m.log(ctx, "imc-media-cache", "status", "tokenized", "seq", seqID,
		"num_chunks", numChunks, "use_mrope", useMRoPE, "use_noncausal", useNonCausal)

	// Step 4: Process each chunk, decoding into the KV cache sequence.
	pos := startPos
	var physicalKVCells int
	var mediaKVCounts []int
	var samplerPromptTokens []llama.Token
	var nativeChunks []imcMediaChunk
	prefixCursor := imcMediaPrefixCursor{chunks: prefix}

	for i := range numChunks {
		chunk := mtmd.InputChunksGet(inputChunks, i)
		chunkType := mtmd.InputChunkGetType(chunk)
		nTokens := mtmd.InputChunkGetNTokens(chunk)
		nPos := int(mtmd.InputChunkGetNPos(chunk))

		switch chunkType {
		case mtmd.InputChunkTypeText:
			tokens := mtmd.InputChunkGetTokensText(chunk)
			nativeChunks = append(nativeChunks, imcMediaChunk{kind: imcMediaChunkText, tokens: slices.Clone(tokens)})
			samplerPromptTokens = append(samplerPromptTokens, tokens...)
			var skip int
			tokens, skip, err = prefixCursor.consumeText(tokens)
			if err != nil {
				return 0, 0, nil, nil, nil, err
			}
			if len(tokens) == 0 {
				continue
			}
			physicalKVCells += len(tokens)

			m.log(ctx, "imc-media-cache", "status", "decoding-text-chunk", "seq", seqID,
				"chunk", i, "tokens", len(tokens), "skipped_tokens", skip, "pos", pos, "mrope", useMRoPE)

			switch {
			case useMRoPE:
				nDecoded, err := m.decodeTextMRoPEIntoCache(tokens, seqID, pos)
				if err != nil {
					return 0, 0, nil, nil, nil, fmt.Errorf("imc-media-cache: text chunk %d (M-RoPE): %w", i, err)
				}
				pos += nDecoded
			default:
				if err := m.decodeTokensIntoCache(ctx, tokens, seqID, pos); err != nil {
					return 0, 0, nil, nil, nil, fmt.Errorf("imc-media-cache: text chunk %d: %w", i, err)
				}
				pos += len(tokens)
			}

		case mtmd.InputChunkTypeImage:
			nativeChunk := imcMediaChunk{kind: imcMediaChunkImage, nTokens: int(nTokens), nPos: nPos}
			nativeChunks = append(nativeChunks, nativeChunk)
			cached, err := prefixCursor.consumeMedia(nativeChunk)
			if err != nil {
				return 0, 0, nil, nil, nil, err
			}
			if cached {
				continue
			}
			physicalKVCells += int(nTokens)
			m.log(ctx, "imc-media-cache", "status", "encoding-image-chunk", "seq", seqID,
				"chunk", i, "tokens", nTokens, "pos", pos)

			if err := mtmd.EncodeChunk(mtmdCtx, chunk); err != nil {
				return 0, 0, nil, nil, nil, fmt.Errorf("imc-media-cache: encode image chunk %d: %w", i, err)
			}

			nEmbd := llama.ModelNEmbdInp(m.model)
			embedSize := nEmbd * int32(nTokens)
			embd, err := mtmd.GetOutputEmbd(mtmdCtx, embedSize)
			if err != nil {
				return 0, 0, nil, nil, nil, fmt.Errorf("imc-media-cache: get image embeddings chunk %d: %w", i, err)
			}

			switch {
			case useMRoPE:
				imageTokens := mtmd.InputChunkGetTokensImage(chunk)
				positions, err := imageTokensDecoderPositions(imageTokens, llama.Pos(pos), int32(nTokens))
				if err != nil {
					return 0, 0, nil, nil, nil, fmt.Errorf("imc-media-cache: get image decoder positions for chunk %d: %w", i, err)
				}

				m.log(ctx, "imc-media-cache", "status", "decoding-image-mrope", "seq", seqID,
					"chunk", i, "tokens", nTokens, "positions", nPos, "pos", pos)

				nDecoded, err := m.decodeEmbeddingsMRoPEIntoCache(embd, nEmbd, int32(nTokens), positions, seqID, useNonCausal)
				if err != nil {
					return 0, 0, nil, nil, nil, fmt.Errorf("imc-media-cache: decode image embeddings chunk %d (M-RoPE): %w", i, err)
				}
				pos += nPos
				mediaKVCounts = append(mediaKVCounts, nDecoded)
			default:
				nDecoded, err := m.decodeEmbeddingsIntoCache(embd, nEmbd, int32(nTokens), seqID, pos, useNonCausal)
				if err != nil {
					return 0, 0, nil, nil, nil, fmt.Errorf("imc-media-cache: decode image embeddings chunk %d: %w", i, err)
				}
				pos += nDecoded
				mediaKVCounts = append(mediaKVCounts, nDecoded)
			}

		case mtmd.InputChunkTypeAudio:
			nativeChunk := imcMediaChunk{kind: imcMediaChunkAudio, nTokens: int(nTokens), nPos: nPos}
			nativeChunks = append(nativeChunks, nativeChunk)
			cached, err := prefixCursor.consumeMedia(nativeChunk)
			if err != nil {
				return 0, 0, nil, nil, nil, err
			}
			if cached {
				continue
			}
			physicalKVCells += int(nTokens)
			m.log(ctx, "imc-media-cache", "status", "encoding-audio-chunk", "seq", seqID,
				"chunk", i, "tokens", nTokens, "pos", pos)

			if err := mtmd.EncodeChunk(mtmdCtx, chunk); err != nil {
				return 0, 0, nil, nil, nil, fmt.Errorf("imc-media-cache: encode audio chunk %d: %w", i, err)
			}

			nEmbd := llama.ModelNEmbdInp(m.model)
			embedSize := nEmbd * int32(nTokens)
			embd, err := mtmd.GetOutputEmbd(mtmdCtx, embedSize)
			if err != nil {
				return 0, 0, nil, nil, nil, fmt.Errorf("imc-media-cache: get audio embeddings chunk %d: %w", i, err)
			}

			switch {
			case useMRoPE:
				positions := linearMRoPEPositions(int32(nTokens), llama.Pos(pos))
				nDecoded, err := m.decodeEmbeddingsMRoPEIntoCache(embd, nEmbd, int32(nTokens), positions, seqID, useNonCausal)
				if err != nil {
					return 0, 0, nil, nil, nil, fmt.Errorf("imc-media-cache: decode audio embeddings chunk %d (M-RoPE): %w", i, err)
				}
				pos += nPos
				mediaKVCounts = append(mediaKVCounts, nDecoded)

			default:
				nDecoded, err := m.decodeEmbeddingsIntoCache(embd, nEmbd, int32(nTokens), seqID, pos, useNonCausal)
				if err != nil {
					return 0, 0, nil, nil, nil, fmt.Errorf("imc-media-cache: decode audio embeddings chunk %d: %w", i, err)
				}
				pos += nDecoded
				mediaKVCounts = append(mediaKVCounts, nDecoded)
			}
		}
	}
	if !prefixCursor.done() {
		return 0, 0, nil, nil, nil, fmt.Errorf("%w: current stream ended first", errIMCMediaNativePrefix)
	}

	m.log(ctx, "imc-media-cache", "status", "complete", "seq", seqID,
		"logical_positions", pos, "physical_kv_cells", physicalKVCells,
		"media_kv_cells", mediaKVCounts, "num_chunks", numChunks, "cached_prefix_chunks", len(prefix))

	return pos, physicalKVCells, mediaKVCounts, samplerPromptTokens, nativeChunks, nil
}

// decodeEmbeddingsIntoCache decodes embeddings into a KV cache sequence with
// standard linear positioning. Returns the number of KV positions consumed.
func (m *Model) decodeEmbeddingsIntoCache(embd []float32, nEmbd, nTokens int32, seqID llama.SeqId, startPos int, useNonCausal bool) (int, error) {
	if nTokens == 0 {
		return 0, nil
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
	}

	return int(nTokens), nil
}

// decodeEmbeddingsMRoPEIntoCache decodes embeddings with M-RoPE positioning
// into a KV cache sequence. Returns the number of physical KV cells consumed.
func (m *Model) decodeEmbeddingsMRoPEIntoCache(embd []float32, nEmbd, nTokens int32, positions []llama.Pos, seqID llama.SeqId, useNonCausal bool) (int, error) {
	if len(positions) != int(nTokens*4) {
		return 0, fmt.Errorf("mrope embedding positions: got %d, want %d", len(positions), nTokens*4)
	}
	if nTokens == 0 {
		return 0, nil
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
	}

	return int(nTokens), nil
}

// decodeTextMRoPEIntoCache decodes text tokens with M-RoPE 4D positioning
// into a KV cache sequence. Returns the number of physical KV cells consumed;
// the caller advances its logical position with mtmd.InputChunkGetNPos.
func (m *Model) decodeTextMRoPEIntoCache(tokens []llama.Token, seqID llama.SeqId, startPos int) (int, error) {
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
	}

	return n, nil
}
