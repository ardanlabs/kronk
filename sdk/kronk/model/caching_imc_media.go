package model

import (
	"context"
	"fmt"
	"unsafe"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"go.opentelemetry.io/otel/attribute"
)

// decodeMediaIntoCache decodes a document containing text and media (images/audio)
// into a KV cache sequence using the mtmd pipeline. This is used by IMC media
// cache builds to populate the slot's KV cache with the full multi-modal prefix.
//
// The passed-in mtmdCtx is reused from job.mtmdCtx to avoid loading the
// projection file twice. Returns the total number of KV positions cached and
// the KV positions consumed per media chunk.
func (m *Model) decodeMediaIntoCache(ctx context.Context, cacheD D, seqID llama.SeqId, mtmdCtx mtmd.Context) (int, []int, error) {
	ctx, span := otel.AddSpan(ctx, "imc-media-cache-build",
		attribute.Int("seq", int(seqID)),
	)
	defer span.End()

	// Step 1: Create prompt and extract media bytes from the cache document.
	prompt, media, err := m.createPrompt(ctx, cacheD)
	if err != nil {
		return 0, nil, fmt.Errorf("imc-media-cache: unable to create prompt: %w", err)
	}

	m.log(ctx, "imc-media-cache", "status", "prompt-created", "seq", seqID,
		"prompt_len", len(prompt), "media_count", len(media))

	// Step 2: Create bitmaps from raw media bytes.
	bitmaps := make([]mtmd.Bitmap, len(media))
	for i, med := range media {
		if len(med) == 0 {
			continue
		}
		bitmaps[i] = mtmd.BitmapInitFromBuf(mtmdCtx, &med[0], uint64(len(med)))
	}
	defer func() {
		for _, b := range bitmaps {
			if b != 0 {
				mtmd.BitmapFree(b)
			}
		}
	}()

	// Step 3: Tokenize the prompt with media into interleaved chunks.
	inputChunks := mtmd.InputChunksInit()
	defer mtmd.InputChunksFree(inputChunks)

	input := mtmd.NewInputText(prompt, true, true)
	if result := mtmd.Tokenize(mtmdCtx, inputChunks, input, bitmaps); result != 0 {
		return 0, nil, fmt.Errorf("imc-media-cache: tokenization failed with code %d", result)
	}

	useMRoPE := mtmd.DecodeUseMRope(mtmdCtx)
	useNonCausal := mtmd.DecodeUseNonCausal(mtmdCtx)

	numChunks := mtmd.InputChunksSize(inputChunks)

	m.log(ctx, "imc-media-cache", "status", "tokenized", "seq", seqID,
		"num_chunks", numChunks, "use_mrope", useMRoPE, "use_noncausal", useNonCausal)

	// Step 4: Process each chunk, decoding into the KV cache sequence.
	var pos int
	var mediaKVCounts []int

	for i := range numChunks {
		chunk := mtmd.InputChunksGet(inputChunks, i)
		chunkType := mtmd.InputChunkGetType(chunk)
		nTokens := mtmd.InputChunkGetNTokens(chunk)

		switch chunkType {
		case mtmd.InputChunkTypeText:
			tokens := mtmd.InputChunkGetTokensText(chunk)
			if len(tokens) == 0 {
				continue
			}

			m.log(ctx, "imc-media-cache", "status", "decoding-text-chunk", "seq", seqID,
				"chunk", i, "tokens", len(tokens), "pos", pos, "mrope", useMRoPE)

			switch {
			case useMRoPE:
				nDecoded, err := m.decodeTextMRoPEIntoCache(tokens, seqID, pos)
				if err != nil {
					return 0, nil, fmt.Errorf("imc-media-cache: text chunk %d (M-RoPE): %w", i, err)
				}
				pos += nDecoded
			default:
				if err := m.decodeTokensIntoCache(ctx, tokens, seqID, pos); err != nil {
					return 0, nil, fmt.Errorf("imc-media-cache: text chunk %d: %w", i, err)
				}
				pos += len(tokens)
			}

		case mtmd.InputChunkTypeImage:
			m.log(ctx, "imc-media-cache", "status", "encoding-image-chunk", "seq", seqID,
				"chunk", i, "tokens", nTokens, "pos", pos)

			if err := mtmd.EncodeChunk(mtmdCtx, chunk); err != nil {
				return 0, nil, fmt.Errorf("imc-media-cache: encode image chunk %d: %w", i, err)
			}

			nEmbd := llama.ModelNEmbdInp(m.model)
			embedSize := nEmbd * int32(nTokens)
			embd, err := mtmd.GetOutputEmbd(mtmdCtx, embedSize)
			if err != nil {
				return 0, nil, fmt.Errorf("imc-media-cache: get image embeddings chunk %d: %w", i, err)
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
					return 0, nil, fmt.Errorf("imc-media-cache: decode image embeddings chunk %d (M-RoPE): %w", i, err)
				}
				pos += nDecoded
				mediaKVCounts = append(mediaKVCounts, nDecoded)
			default:
				nDecoded, err := m.decodeEmbeddingsIntoCache(embd, nEmbd, int32(nTokens), seqID, pos, useNonCausal)
				if err != nil {
					return 0, nil, fmt.Errorf("imc-media-cache: decode image embeddings chunk %d: %w", i, err)
				}
				pos += nDecoded
				mediaKVCounts = append(mediaKVCounts, nDecoded)
			}

		case mtmd.InputChunkTypeAudio:
			m.log(ctx, "imc-media-cache", "status", "encoding-audio-chunk", "seq", seqID,
				"chunk", i, "tokens", nTokens, "pos", pos)

			if err := mtmd.EncodeChunk(mtmdCtx, chunk); err != nil {
				return 0, nil, fmt.Errorf("imc-media-cache: encode audio chunk %d: %w", i, err)
			}

			nEmbd := llama.ModelNEmbdInp(m.model)
			embedSize := nEmbd * int32(nTokens)
			embd, err := mtmd.GetOutputEmbd(mtmdCtx, embedSize)
			if err != nil {
				return 0, nil, fmt.Errorf("imc-media-cache: get audio embeddings chunk %d: %w", i, err)
			}

			// Audio uses standard linear positioning (not M-RoPE).
			nDecoded, err := m.decodeEmbeddingsIntoCache(embd, nEmbd, int32(nTokens), seqID, pos, useNonCausal)
			if err != nil {
				return 0, nil, fmt.Errorf("imc-media-cache: decode audio embeddings chunk %d: %w", i, err)
			}
			pos += nDecoded
			mediaKVCounts = append(mediaKVCounts, nDecoded)
		}
	}

	m.log(ctx, "imc-media-cache", "status", "complete", "seq", seqID,
		"total_kv_positions", pos, "num_chunks", numChunks)

	return pos, mediaKVCounts, nil
}

// decodeEmbeddingsIntoCache decodes embeddings into a KV cache sequence with
// standard linear positioning. Returns the number of KV positions consumed.
func (m *Model) decodeEmbeddingsIntoCache(embd []float32, nEmbd, nTokens int32, seqID llama.SeqId, startPos int, useNonCausal bool) (int, error) {
	batch := llama.BatchInit(nTokens, nEmbd, 1)
	defer llama.BatchFree(batch)

	embdSlice := unsafeSlice(batch.Embd, int(nTokens*nEmbd))
	copy(embdSlice, embd)

	posSlice := unsafeSlice(batch.Pos, int(nTokens))
	nSeqIDSlice := unsafeSlice(batch.NSeqId, int(nTokens))
	seqIDPtrs := unsafeSlice(batch.SeqId, int(nTokens))
	logitsSlice := unsafeSlice(batch.Logits, int(nTokens))

	for i := range nTokens {
		posSlice[i] = llama.Pos(startPos + int(i))
		nSeqIDSlice[i] = 1
		*seqIDPtrs[i] = seqID
		logitsSlice[i] = 0
	}

	batch.NTokens = nTokens

	m.decodeMu.Lock()
	if useNonCausal {
		llama.SetCausalAttn(m.lctx, false)
	}
	ret, err := llama.Decode(m.lctx, batch)
	if useNonCausal {
		llama.SetCausalAttn(m.lctx, true)
	}
	m.decodeMu.Unlock()

	if err != nil || ret != 0 {
		return 0, decodeError(ret, err)
	}

	return int(nTokens), nil
}

// decodeEmbeddingsMRoPEIntoCache decodes embeddings with M-RoPE 2D positioning
// into a KV cache sequence. Returns the number of KV positions consumed, which
// is max(nx, ny) to avoid position overlap with subsequent tokens.
func (m *Model) decodeEmbeddingsMRoPEIntoCache(embd []float32, nEmbd, nTokens, nx, ny int32, seqID llama.SeqId, startPos int, useNonCausal bool) (int, error) {
	batch := llama.BatchInit(nTokens, nEmbd, 1)

	embdSlice := unsafeSlice(batch.Embd, int(nTokens*nEmbd))
	copy(embdSlice, embd)

	// Save original pos pointer so BatchFree doesn't free Go memory.
	origPos := batch.Pos

	// Allocate 4D position array for M-RoPE.
	posData := make([]llama.Pos, nTokens*4)
	pos0 := llama.Pos(startPos)
	for y := range ny {
		for x := range nx {
			i := y*nx + x
			if i >= nTokens {
				break
			}
			posData[i] = pos0
			posData[i+nTokens] = pos0 + llama.Pos(y)
			posData[i+nTokens*2] = pos0 + llama.Pos(x)
			posData[i+nTokens*3] = 0
		}
	}
	batch.Pos = &posData[0]

	nSeqIDSlice := unsafeSlice(batch.NSeqId, int(nTokens))
	seqIDPtrs := unsafeSlice(batch.SeqId, int(nTokens))
	logitsSlice := unsafeSlice(batch.Logits, int(nTokens))

	for i := range nTokens {
		nSeqIDSlice[i] = 1
		*seqIDPtrs[i] = seqID
		logitsSlice[i] = 0
	}

	batch.NTokens = nTokens

	m.decodeMu.Lock()
	if useNonCausal {
		llama.SetCausalAttn(m.lctx, false)
	}
	ret, err := llama.Decode(m.lctx, batch)
	if useNonCausal {
		llama.SetCausalAttn(m.lctx, true)
	}
	m.decodeMu.Unlock()

	// Restore original pos pointer before freeing.
	batch.Pos = origPos
	llama.BatchFree(batch)

	if err != nil || ret != 0 {
		return 0, decodeError(ret, err)
	}

	nPos := max(ny, nx)
	return int(nPos), nil
}

// decodeTextMRoPEIntoCache decodes text tokens with M-RoPE 4D positioning
// into a KV cache sequence. Returns the number of KV positions consumed.
func (m *Model) decodeTextMRoPEIntoCache(tokens []llama.Token, seqID llama.SeqId, startPos int) (int, error) {
	n := int32(len(tokens))
	if n == 0 {
		return 0, nil
	}

	nBatch := int32(m.cfg.NBatch)
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
		for i := range batchN {
			posData[i] = llama.Pos(pos + int(i))
			posData[i+batchN] = 0
			posData[i+batchN*2] = 0
			posData[i+batchN*3] = 0
		}
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

		batch.Pos = origPos
		llama.BatchFree(batch)

		if err != nil || ret != 0 {
			return 0, decodeError(ret, err)
		}

		pos += int(batchN)
	}

	return int(n), nil
}
