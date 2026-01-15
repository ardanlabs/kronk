// This example demonstrates low-level multimodal (vision) inference using the
// yzma bindings directly. It shows how to manually process image chunks with
// explicit sequence ID control, bypassing the mutex in HelperEvalChunks.
//
// The key insight is that for image/audio chunks, you:
// 1. Call mtmd.EncodeChunk() to run the vision encoder
// 2. Get embeddings with mtmd.GetOutputEmbd()
// 3. Create a batch with Embd (not Token) and call llama.Decode()
//
// Run the example like this from the root of the project:
// $ go run examples/yzma-multimodal/main.go -model /path/to/vision-model.gguf -proj /path/to/mmproj.gguf -image examples/samples/giraffe.jpg

package main

// import (
// 	"flag"
// 	"fmt"
// 	"io"
// 	"os"
// 	"unsafe"

// 	"github.com/ardanlabs/kronk/sdk/kronk"
// 	"github.com/hybridgroup/yzma/pkg/llama"
// 	"github.com/hybridgroup/yzma/pkg/mtmd"
// )

// func main() {
// 	if err := run(); err != nil {
// 		if err == io.EOF {
// 			return
// 		}
// 		fmt.Println("Error:", err)
// 		os.Exit(1)
// 	}
// }

// func run() error {
// 	modelPath := flag.String("model", "", "Path to the GGUF model file")
// 	projPath := flag.String("proj", "", "Path to the mmproj (vision projector) file")
// 	imagePath := flag.String("image", "examples/samples/giraffe.jpg", "Path to the image file")
// 	seqID := flag.Int("seq", 1, "Sequence ID to use for this request")
// 	flag.Parse()

// 	if *modelPath == "" {
// 		return fmt.Errorf("model path is required: use -model flag")
// 	}
// 	if *projPath == "" {
// 		return fmt.Errorf("projector path is required: use -proj flag")
// 	}

// 	// -------------------------------------------------------------------------
// 	// Initialize kronk (loads the llama.cpp shared library).

// 	if err := kronk.Init(); err != nil {
// 		return fmt.Errorf("unable to init kronk: %w", err)
// 	}

// 	// -------------------------------------------------------------------------
// 	// Load the model.

// 	fmt.Println("Loading model...")

// 	mparams := llama.ModelDefaultParams()
// 	model, err := llama.ModelLoadFromFile(*modelPath, mparams)
// 	if err != nil {
// 		return fmt.Errorf("unable to load model: %w", err)
// 	}
// 	defer llama.ModelFree(model)

// 	vocab := llama.ModelGetVocab(model)
// 	nEmbd := llama.ModelNEmbd(model)

// 	fmt.Printf("  n_embd = %d\n", nEmbd)

// 	// -------------------------------------------------------------------------
// 	// Create llama context.

// 	ctxParams := llama.ContextDefaultParams()
// 	ctxParams.NCtx = 8192
// 	ctxParams.NBatch = 2048
// 	ctxParams.NSeqMax = 4

// 	lctx, err := llama.InitFromModel(model, ctxParams)
// 	if err != nil {
// 		return fmt.Errorf("unable to init context: %w", err)
// 	}
// 	defer llama.Free(lctx)

// 	// -------------------------------------------------------------------------
// 	// Initialize mtmd (multimodal) context.

// 	fmt.Println("Loading vision projector...")

// 	mtmdParams := mtmd.ContextParamsDefault()
// 	mtmdCtx, err := mtmd.InitFromFile(*projPath, model, mtmdParams)
// 	if err != nil {
// 		return fmt.Errorf("unable to init mtmd context: %w", err)
// 	}
// 	defer mtmd.Free(mtmdCtx)

// 	if !mtmd.SupportVision(mtmdCtx) {
// 		return fmt.Errorf("model does not support vision")
// 	}
// 	fmt.Println("  Vision support: enabled")

// 	// -------------------------------------------------------------------------
// 	// Load the image.

// 	fmt.Printf("Loading image: %s\n", *imagePath)

// 	imageData, err := os.ReadFile(*imagePath)
// 	if err != nil {
// 		return fmt.Errorf("unable to read image: %w", err)
// 	}

// 	bitmap := mtmd.BitmapInitFromBuf(mtmdCtx, &imageData[0], uint64(len(imageData)))
// 	if bitmap == 0 {
// 		return fmt.Errorf("unable to create bitmap from image")
// 	}
// 	defer mtmd.BitmapFree(bitmap)

// 	fmt.Printf("  Image size: %dx%d\n", mtmd.BitmapGetNx(bitmap), mtmd.BitmapGetNy(bitmap))

// 	// -------------------------------------------------------------------------
// 	// Create the prompt with image marker.

// 	prompt := fmt.Sprintf("%s\nWhat is in this image? Describe it in detail.", mtmd.DefaultMarker())
// 	fmt.Printf("Prompt: %s\n", prompt)

// 	// -------------------------------------------------------------------------
// 	// Tokenize the prompt with image.

// 	chunks := mtmd.InputChunksInit()
// 	defer mtmd.InputChunksFree(chunks)

// 	inputText := mtmd.NewInputText(prompt, true, true)
// 	bitmaps := []mtmd.Bitmap{bitmap}

// 	ret := mtmd.Tokenize(mtmdCtx, chunks, inputText, bitmaps)
// 	if ret != 0 {
// 		return fmt.Errorf("tokenize failed with code %d", ret)
// 	}

// 	nChunks := mtmd.InputChunksSize(chunks)
// 	fmt.Printf("Tokenized into %d chunks\n", nChunks)

// 	// -------------------------------------------------------------------------
// 	// Process each chunk manually with explicit sequence ID.
// 	// This is the key part - we control the seq_id for each chunk.

// 	useSeqID := llama.SeqId(*seqID)
// 	var nPast llama.Pos = 0
// 	useMRoPE := mtmd.DecodeUseMRope(mtmdCtx)

// 	fmt.Printf("\nProcessing chunks with seq_id=%d (M-RoPE=%v):\n", useSeqID, useMRoPE)

// 	for i := range nChunks {
// 		chunk := mtmd.InputChunksGet(chunks, i)
// 		chunkType := mtmd.InputChunkGetType(chunk)
// 		nTokens := mtmd.InputChunkGetNTokens(chunk)

// 		switch chunkType {
// 		case mtmd.InputChunkTypeText:
// 			fmt.Printf("  Chunk %d: TEXT (%d tokens)\n", i, nTokens)

// 			tokens := mtmd.InputChunkGetTokensText(chunk)
// 			if err := decodeTextChunk(lctx, tokens, useSeqID, &nPast); err != nil {
// 				return fmt.Errorf("decode text chunk %d: %w", i, err)
// 			}

// 		case mtmd.InputChunkTypeImage:
// 			fmt.Printf("  Chunk %d: IMAGE (%d tokens)\n", i, nTokens)

// 			if err := decodeImageChunk(mtmdCtx, lctx, model, chunk, useSeqID, &nPast, useMRoPE); err != nil {
// 				return fmt.Errorf("decode image chunk %d: %w", i, err)
// 			}

// 		case mtmd.InputChunkTypeAudio:
// 			fmt.Printf("  Chunk %d: AUDIO (%d tokens)\n", i, nTokens)

// 			if err := decodeImageChunk(mtmdCtx, lctx, model, chunk, useSeqID, &nPast, useMRoPE); err != nil {
// 				return fmt.Errorf("decode audio chunk %d: %w", i, err)
// 			}

// 		default:
// 			return fmt.Errorf("unknown chunk type %d", chunkType)
// 		}
// 	}

// 	fmt.Printf("\nPrefill complete. n_past=%d\n", nPast)

// 	// -------------------------------------------------------------------------
// 	// Generate response tokens.

// 	fmt.Print("\nMODEL> ")

// 	sampler := llama.SamplerChainInit(llama.SamplerChainDefaultParams())
// 	llama.SamplerChainAdd(sampler, llama.SamplerInitTopK(40))
// 	llama.SamplerChainAdd(sampler, llama.SamplerInitTopP(0.9, 1))
// 	llama.SamplerChainAdd(sampler, llama.SamplerInitTempExt(0.7, 0.0, 1.0))
// 	llama.SamplerChainAdd(sampler, llama.SamplerInitDist(1))
// 	defer llama.SamplerFree(sampler)

// 	buf := make([]byte, 256)
// 	maxTokens := 256

// 	for range maxTokens {
// 		token := llama.SamplerSample(sampler, lctx, -1)
// 		llama.SamplerAccept(sampler, token)

// 		if llama.VocabIsEOG(vocab, token) {
// 			break
// 		}

// 		l := llama.TokenToPiece(vocab, token, buf, 0, true)
// 		fmt.Print(string(buf[:l]))

// 		// Use the same sequence ID as prefill for generation.
// 		batch := createTokenBatch([]llama.Token{token}, useSeqID, nPast, true)
// 		_, err := llama.Decode(lctx, batch)
// 		llama.BatchFree(batch)

// 		if err != nil {
// 			return fmt.Errorf("decode failed: %w", err)
// 		}
// 		nPast++
// 	}

// 	fmt.Println()
// 	fmt.Printf("\nTotal tokens: %d\n", nPast)

// 	return nil
// }

// // decodeTextChunk processes a text chunk by decoding tokens with the specified sequence ID.
// func decodeTextChunk(lctx llama.Context, tokens []llama.Token, seqID llama.SeqId, nPast *llama.Pos) error {
// 	nBatch := int32(512)

// 	for i := 0; i < len(tokens); i += int(nBatch) {
// 		end := min(i+int(nBatch), len(tokens))
// 		batchTokens := tokens[i:end]

// 		batch := createTokenBatch(batchTokens, seqID, *nPast, end == len(tokens))

// 		_, err := llama.Decode(lctx, batch)
// 		llama.BatchFree(batch)

// 		if err != nil {
// 			return fmt.Errorf("decode failed: %w", err)
// 		}

// 		*nPast += llama.Pos(len(batchTokens))
// 	}

// 	return nil
// }

// // decodeImageChunk encodes an image chunk and decodes its embeddings.
// // For M-RoPE models (like Qwen2.5-VL), positions are set up as a 2D grid.
// func decodeImageChunk(mtmdCtx mtmd.Context, lctx llama.Context, model llama.Model, chunk mtmd.InputChunk, seqID llama.SeqId, nPast *llama.Pos, useMRoPE bool) error {
// 	// Step 1: Encode the image chunk through the vision encoder.
// 	ret := mtmd.EncodeChunk(mtmdCtx, chunk)
// 	if ret != 0 {
// 		return fmt.Errorf("encode chunk failed with code %d", ret)
// 	}

// 	// Step 2: Get the output embeddings.
// 	embdPtr := mtmd.GetOutputEmbd(mtmdCtx)
// 	if embdPtr == nil {
// 		return fmt.Errorf("get output embd returned nil")
// 	}

// 	// Step 3: Get image dimensions for M-RoPE.
// 	imageTokens := mtmd.InputChunkGetTokensImage(chunk)
// 	nx := int32(mtmd.ImageTokensGetNX(imageTokens))
// 	ny := int32(mtmd.ImageTokensGetNY(imageTokens))
// 	nTokens := int32(mtmd.InputChunkGetNTokens(chunk))
// 	nPos := mtmd.InputChunkGetNPos(chunk) // For M-RoPE: max(nx, ny)
// 	nEmbd := llama.ModelNEmbdInp(model)

// 	fmt.Printf("    Decoding %d image tokens (nx=%d, ny=%d, n_pos=%d, n_embd_inp=%d, mrope=%v)\n",
// 		nTokens, nx, ny, nPos, nEmbd, useMRoPE)

// 	// Step 4: Create batch with proper position setup and decode.
// 	// For M-RoPE, we need to decode all tokens in one batch to maintain the 2D position grid.
// 	// Splitting would require recalculating partial 2D positions which is complex.
// 	if useMRoPE {
// 		batch := createEmbdBatchMRoPE(embdPtr, nTokens, nEmbd, seqID, *nPast, nx, ny)
// 		_, err := llama.Decode(lctx, batch)
// 		freeEmbdBatchMRoPE(batch)

// 		if err != nil {
// 			return fmt.Errorf("decode embeddings failed: %w", err)
// 		}
// 	} else {
// 		nBatch := int32(512)
// 		for offset := int32(0); offset < nTokens; offset += nBatch {
// 			batchSize := min(nBatch, nTokens-offset)
// 			embdOffset := unsafe.Pointer(uintptr(unsafe.Pointer(embdPtr)) + uintptr(offset*nEmbd)*unsafe.Sizeof(float32(0)))
// 			batch := createEmbdBatch((*float32)(embdOffset), batchSize, nEmbd, seqID, *nPast+llama.Pos(offset))

// 			_, err := llama.Decode(lctx, batch)
// 			llama.BatchFree(batch)

// 			if err != nil {
// 				return fmt.Errorf("decode embeddings failed: %w", err)
// 			}
// 		}
// 	}

// 	// Advance position by n_pos (for M-RoPE: max(nx,ny), otherwise: n_tokens)
// 	*nPast += nPos

// 	return nil
// }

// // createTokenBatch creates a batch for token decoding with explicit sequence ID.
// func createTokenBatch(tokens []llama.Token, seqID llama.SeqId, pos0 llama.Pos, logitsLast bool) llama.Batch {
// 	nTokens := int32(len(tokens))

// 	batch := llama.BatchInit(nTokens, 0, 1)
// 	batch.NTokens = nTokens

// 	for i := int32(0); i < nTokens; i++ {
// 		tokenPtr := (*llama.Token)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.Token)) + uintptr(i)*unsafe.Sizeof(llama.Token(0))))
// 		*tokenPtr = tokens[i]

// 		posPtr := (*llama.Pos)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.Pos)) + uintptr(i)*unsafe.Sizeof(llama.Pos(0))))
// 		*posPtr = pos0 + llama.Pos(i)

// 		nSeqPtr := (*int32)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.NSeqId)) + uintptr(i)*unsafe.Sizeof(int32(0))))
// 		*nSeqPtr = 1

// 		seqIDPtrPtr := (**llama.SeqId)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.SeqId)) + uintptr(i)*unsafe.Sizeof(uintptr(0))))
// 		if *seqIDPtrPtr != nil {
// 			**seqIDPtrPtr = seqID
// 		}

// 		logitPtr := (*int8)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.Logits)) + uintptr(i)*unsafe.Sizeof(int8(0))))
// 		if logitsLast && i == nTokens-1 {
// 			*logitPtr = 1
// 		} else {
// 			*logitPtr = 0
// 		}
// 	}

// 	return batch
// }

// // createEmbdBatch creates a batch for embedding decoding with explicit sequence ID.
// // nEmbd should be llama.ModelNEmbdInp(model).
// // NOTE: We use embd=0 in BatchInit because we provide our own embedding pointer.
// // BatchFree will NOT free the embedding memory (it's owned by mtmd).
// func createEmbdBatch(embd *float32, nTokens int32, nEmbd int32, seqID llama.SeqId, pos0 llama.Pos) llama.Batch {
// 	batch := llama.BatchInit(nTokens, 0, 1) // embd=0: don't allocate embedding memory
// 	batch.NTokens = nTokens
// 	batch.Token = nil
// 	batch.Embd = embd // Point to mtmd's embedding buffer (we don't own this)

// 	for i := range nTokens {
// 		posPtr := (*llama.Pos)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.Pos)) + uintptr(i)*unsafe.Sizeof(llama.Pos(0))))
// 		*posPtr = pos0 + llama.Pos(i)

// 		nSeqPtr := (*int32)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.NSeqId)) + uintptr(i)*unsafe.Sizeof(int32(0))))
// 		*nSeqPtr = 1

// 		seqIDPtrPtr := (**llama.SeqId)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.SeqId)) + uintptr(i)*unsafe.Sizeof(uintptr(0))))
// 		if *seqIDPtrPtr != nil {
// 			**seqIDPtrPtr = seqID
// 		}

// 		logitPtr := (*int8)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.Logits)) + uintptr(i)*unsafe.Sizeof(int8(0))))
// 		if i == nTokens-1 {
// 			*logitPtr = 1
// 		} else {
// 			*logitPtr = 0
// 		}
// 	}

// 	return batch
// }

// // mropeBatch holds a batch with M-RoPE position data.
// // For M-RoPE, positions are stored as 4 values per token: [temporal, height, width, unused].
// // The position array is laid out as: [t0,t1,...,tN, h0,h1,...,hN, w0,w1,...,wN, 0,0,...,0]
// type mropeBatch struct {
// 	batch   llama.Batch
// 	posData []llama.Pos    // 4 * nTokens positions
// 	seqIDs  []llama.SeqId  // nTokens seq IDs (all same value)
// 	seqPtrs []*llama.SeqId // nTokens pointers to seq IDs
// 	nSeqIds []int32        // nTokens n_seq_ids (all 1s)
// 	logits  []int8         // nTokens logits flags
// }

// // createEmbdBatchMRoPE creates an embedding batch with M-RoPE 2D position grid.
// // For images, positions are set up as a 2D grid where:
// //   - pos[i] = pos0 (temporal dimension, constant)
// //   - pos[i + n] = pos0 + y (height dimension)
// //   - pos[i + 2n] = pos0 + x (width dimension)
// //   - pos[i + 3n] = 0 (unused)
// func createEmbdBatchMRoPE(embd *float32, nTokens int32, nEmbd int32, seqID llama.SeqId, pos0 llama.Pos, nx, ny int32) llama.Batch {
// 	mb := &mropeBatch{
// 		posData: make([]llama.Pos, nTokens*4),
// 		seqIDs:  make([]llama.SeqId, nTokens),
// 		seqPtrs: make([]*llama.SeqId, nTokens),
// 		nSeqIds: make([]int32, nTokens),
// 		logits:  make([]int8, nTokens),
// 	}

// 	// Set up 2D position grid for M-RoPE
// 	for y := int32(0); y < ny; y++ {
// 		for x := int32(0); x < nx; x++ {
// 			i := y*nx + x
// 			if i >= nTokens {
// 				break
// 			}
// 			mb.posData[i] = pos0                           // temporal (constant)
// 			mb.posData[i+nTokens] = pos0 + llama.Pos(y)    // height
// 			mb.posData[i+nTokens*2] = pos0 + llama.Pos(x)  // width
// 			mb.posData[i+nTokens*3] = 0                    // unused
// 		}
// 	}

// 	// Set up seq IDs and other arrays
// 	for i := range nTokens {
// 		mb.seqIDs[i] = seqID
// 		mb.seqPtrs[i] = &mb.seqIDs[i]
// 		mb.nSeqIds[i] = 1
// 		mb.logits[i] = 0
// 	}
// 	mb.logits[nTokens-1] = 1 // Only last token needs logits

// 	mb.batch = llama.Batch{
// 		NTokens: nTokens,
// 		Token:   nil,
// 		Embd:    embd,
// 		Pos:     &mb.posData[0],
// 		NSeqId:  &mb.nSeqIds[0],
// 		SeqId:   (**llama.SeqId)(unsafe.Pointer(&mb.seqPtrs[0])),
// 		Logits:  &mb.logits[0],
// 	}

// 	// Store reference to prevent GC
// 	mropeBatches[&mb.batch] = mb

// 	return mb.batch
// }

// // mropeBatches keeps M-RoPE batch data alive during decode.
// var mropeBatches = make(map[*llama.Batch]*mropeBatch)

// // freeEmbdBatchMRoPE frees an M-RoPE batch.
// func freeEmbdBatchMRoPE(batch llama.Batch) {
// 	delete(mropeBatches, &batch)
// }
