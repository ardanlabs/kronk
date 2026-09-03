package model

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"unsafe"

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
				nx := int32(mtmd.ImageTokensGetNX(imageTokens))
				ny := int32(mtmd.ImageTokensGetNY(imageTokens))

				m.log(ctx, "imc-media-cache", "status", "decoding-image-mrope", "seq", seqID,
					"chunk", i, "nx", nx, "ny", ny, "pos", pos)

				nDecoded, err := m.decodeEmbeddingsMRoPEIntoCache(embd, nEmbd, int32(nTokens), nx, ny, seqID, pos, useNonCausal)
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

			// Audio uses standard linear positioning (not M-RoPE).
			nDecoded, err := m.decodeEmbeddingsIntoCache(embd, nEmbd, int32(nTokens), seqID, pos, useNonCausal)
			if err != nil {
				return 0, 0, nil, nil, nil, fmt.Errorf("imc-media-cache: decode audio embeddings chunk %d: %w", i, err)
			}
			pos += nDecoded
			mediaKVCounts = append(mediaKVCounts, nDecoded)
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
	nBatch := int32(m.cfg.EffectiveNBatch())
	if nBatch <= 0 {
		nBatch = 512
	}

	m.decodeMu.Lock()
	defer m.decodeMu.Unlock()

	if useNonCausal {
		llama.SetCausalAttn(m.lctx, false)
		defer llama.SetCausalAttn(m.lctx, true)
	}

	pos := startPos

	for start := int32(0); start < nTokens; start += nBatch {
		end := min(start+nBatch, nTokens)
		batchN := end - start

		batch := llama.BatchInit(batchN, nEmbd, 1)

		embdSlice := unsafeSlice(batch.Embd, int(batchN*nEmbd))
		copy(embdSlice, embd[start*nEmbd:end*nEmbd])

		posSlice := unsafeSlice(batch.Pos, int(batchN))
		nSeqIDSlice := unsafeSlice(batch.NSeqId, int(batchN))
		seqIDPtrs := unsafeSlice(batch.SeqId, int(batchN))
		logitsSlice := unsafeSlice(batch.Logits, int(batchN))

		for i := range batchN {
			posSlice[i] = llama.Pos(pos + int(i))
			nSeqIDSlice[i] = 1
			*seqIDPtrs[i] = seqID
			logitsSlice[i] = 0
		}

		batch.NTokens = batchN

		ret, err := llama.Decode(m.lctx, batch)
		if err == nil && ret == 0 {
			llama.Synchronize(m.lctx)
		}
		llama.BatchFree(batch)

		if err != nil || ret != 0 {
			return 0, decodeError(ret, err)
		}

		pos += int(batchN)
	}

	return int(nTokens), nil
}

// decodeEmbeddingsMRoPEIntoCache decodes embeddings with M-RoPE 2D positioning
// into a KV cache sequence. Returns the number of KV positions consumed.
func (m *Model) decodeEmbeddingsMRoPEIntoCache(embd []float32, nEmbd, nTokens, nx, ny int32, seqID llama.SeqId, startPos int, useNonCausal bool) (int, error) {
	if nTokens != nx*ny {
		return 0, fmt.Errorf("mrope image layout: unsupported token count %d for grid %dx%d", nTokens, nx, ny)
	}

	nBatch := int32(m.cfg.EffectiveNBatch())
	if nBatch <= 0 {
		nBatch = 512
	}

	// Pre-compute the full 4D position array for all tokens.
	fullPosData := make([]llama.Pos, nTokens*4)
	pos0 := llama.Pos(startPos)
	fillMRoPEImagePositions(fullPosData, nTokens, nx, ny, pos0)

	m.decodeMu.Lock()
	defer m.decodeMu.Unlock()

	if useNonCausal {
		llama.SetCausalAttn(m.lctx, false)
		defer llama.SetCausalAttn(m.lctx, true)
	}

	for start := int32(0); start < nTokens; start += nBatch {
		end := min(start+nBatch, nTokens)
		batchN := end - start

		batch := llama.BatchInit(batchN, nEmbd, 1)

		// Save original pos pointer so BatchFree doesn't free Go memory.
		origPos := batch.Pos

		embdSlice := unsafeSlice(batch.Embd, int(batchN*nEmbd))
		copy(embdSlice, embd[start*nEmbd:end*nEmbd])

		// Build sub-batch position array by gathering from the full array.
		// llama.cpp expects 4 contiguous planes of batchN positions each.
		subPosData := make([]llama.Pos, batchN*4)
		for i := range batchN {
			subPosData[i] = fullPosData[start+i]
			subPosData[i+batchN] = fullPosData[start+i+nTokens]
			subPosData[i+batchN*2] = fullPosData[start+i+nTokens*2]
			subPosData[i+batchN*3] = fullPosData[start+i+nTokens*3]
		}
		batch.Pos = &subPosData[0]

		nSeqIDSlice := unsafeSlice(batch.NSeqId, int(batchN))
		seqIDPtrs := unsafeSlice(batch.SeqId, int(batchN))
		logitsSlice := unsafeSlice(batch.Logits, int(batchN))

		for i := range batchN {
			nSeqIDSlice[i] = 1
			*seqIDPtrs[i] = seqID
			logitsSlice[i] = 0
		}

		batch.NTokens = batchN

		ret, err := llama.Decode(m.lctx, batch)
		if err == nil && ret == 0 {
			llama.Synchronize(m.lctx)
		}
		runtime.KeepAlive(subPosData)

		batch.Pos = origPos
		llama.BatchFree(batch)

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
	n := int32(len(tokens))
	if n == 0 {
		return 0, nil
	}

	nBatch := int32(m.cfg.EffectiveNBatch())
	if nBatch <= 0 {
		nBatch = 512
	}

	m.decodeMu.Lock()
	defer m.decodeMu.Unlock()

	pos := startPos

	for start := int32(0); start < n; start += nBatch {
		end := min(start+nBatch, n)
		batchN := end - start

		batch := llama.BatchInit(batchN, 0, 1)

		// Save original pos pointer.
		origPos := batch.Pos

		tokenSlice := unsafe.Slice(batch.Token, int(batchN))
		copy(tokenSlice, tokens[start:end])

		// Allocate 4D position array for M-RoPE.
		posData := make([]llama.Pos, batchN*4)
		fillMRoPETextPositions(posData, batchN, llama.Pos(pos))
		batch.Pos = &posData[0]

		nSeqIDSlice := unsafe.Slice(batch.NSeqId, int(batchN))
		seqIDPtrs := unsafe.Slice(batch.SeqId, int(batchN))
		logitsSlice := unsafe.Slice(batch.Logits, int(batchN))

		for i := range batchN {
			nSeqIDSlice[i] = 1
			*seqIDPtrs[i] = seqID
			logitsSlice[i] = 0
		}

		batch.NTokens = batchN

		ret, err := llama.Decode(m.lctx, batch)
		if err == nil && ret == 0 {
			llama.Synchronize(m.lctx)
		}
		runtime.KeepAlive(posData)

		batch.Pos = origPos
		llama.BatchFree(batch)

		if err != nil || ret != 0 {
			return 0, decodeError(ret, err)
		}

		pos += int(batchN)
	}

	return int(n), nil
}
