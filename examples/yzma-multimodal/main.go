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

import (
	"flag"
	"fmt"
	"io"
	"os"
	"unsafe"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

func main() {
	if err := run(); err != nil {
		if err == io.EOF {
			return
		}
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

func run() error {
	modelPath := flag.String("model", "", "Path to the GGUF model file")
	projPath := flag.String("proj", "", "Path to the mmproj (vision projector) file")
	imagePath := flag.String("image", "examples/samples/giraffe.jpg", "Path to the image file")
	seqID := flag.Int("seq", 1, "Sequence ID to use for this request")
	flag.Parse()

	if *modelPath == "" {
		return fmt.Errorf("model path is required: use -model flag")
	}
	if *projPath == "" {
		return fmt.Errorf("projector path is required: use -proj flag")
	}

	// -------------------------------------------------------------------------
	// Initialize kronk (loads the llama.cpp shared library).

	if err := kronk.Init(); err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	// -------------------------------------------------------------------------
	// Load the model.

	fmt.Println("Loading model...")

	mparams := llama.ModelDefaultParams()
	model, err := llama.ModelLoadFromFile(*modelPath, mparams)
	if err != nil {
		return fmt.Errorf("unable to load model: %w", err)
	}
	defer llama.ModelFree(model)

	vocab := llama.ModelGetVocab(model)
	nEmbd := llama.ModelNEmbd(model)

	fmt.Printf("  n_embd = %d\n", nEmbd)

	// -------------------------------------------------------------------------
	// Create llama context.

	ctxParams := llama.ContextDefaultParams()
	ctxParams.NCtx = 8192
	ctxParams.NBatch = 2048
	ctxParams.NSeqMax = 4

	lctx, err := llama.InitFromModel(model, ctxParams)
	if err != nil {
		return fmt.Errorf("unable to init context: %w", err)
	}
	defer llama.Free(lctx)

	// -------------------------------------------------------------------------
	// Initialize mtmd (multimodal) context.

	fmt.Println("Loading vision projector...")

	mtmdParams := mtmd.ContextParamsDefault()
	mtmdCtx, err := mtmd.InitFromFile(*projPath, model, mtmdParams)
	if err != nil {
		return fmt.Errorf("unable to init mtmd context: %w", err)
	}
	defer mtmd.Free(mtmdCtx)

	if !mtmd.SupportVision(mtmdCtx) {
		return fmt.Errorf("model does not support vision")
	}
	fmt.Println("  Vision support: enabled")

	// -------------------------------------------------------------------------
	// Load the image.

	fmt.Printf("Loading image: %s\n", *imagePath)

	imageData, err := os.ReadFile(*imagePath)
	if err != nil {
		return fmt.Errorf("unable to read image: %w", err)
	}

	bitmap := mtmd.BitmapInitFromBuf(mtmdCtx, &imageData[0], uint64(len(imageData)))
	if bitmap == 0 {
		return fmt.Errorf("unable to create bitmap from image")
	}
	defer mtmd.BitmapFree(bitmap)

	fmt.Printf("  Image size: %dx%d\n", mtmd.BitmapGetNx(bitmap), mtmd.BitmapGetNy(bitmap))

	// -------------------------------------------------------------------------
	// Create the prompt with image marker.

	prompt := fmt.Sprintf("%s\nWhat is in this image? Describe it in detail.", mtmd.DefaultMarker())
	fmt.Printf("Prompt: %s\n", prompt)

	// -------------------------------------------------------------------------
	// Tokenize the prompt with image.

	chunks := mtmd.InputChunksInit()
	defer mtmd.InputChunksFree(chunks)

	inputText := mtmd.NewInputText(prompt, true, true)
	bitmaps := []mtmd.Bitmap{bitmap}

	ret := mtmd.Tokenize(mtmdCtx, chunks, inputText, bitmaps)
	if ret != 0 {
		return fmt.Errorf("tokenize failed with code %d", ret)
	}

	nChunks := mtmd.InputChunksSize(chunks)
	fmt.Printf("Tokenized into %d chunks\n", nChunks)

	// -------------------------------------------------------------------------
	// Process each chunk manually with explicit sequence ID.
	// This is the key part - we control the seq_id for each chunk.

	useSeqID := llama.SeqId(*seqID)
	var nPast llama.Pos = 0

	fmt.Printf("\nProcessing chunks with seq_id=%d:\n", useSeqID)

	for i := uint32(0); i < nChunks; i++ {
		chunk := mtmd.InputChunksGet(chunks, i)
		chunkType := mtmd.InputChunkGetType(chunk)
		nTokens := mtmd.InputChunkGetNTokens(chunk)

		switch chunkType {
		case mtmd.InputChunkTypeText:
			fmt.Printf("  Chunk %d: TEXT (%d tokens)\n", i, nTokens)

			tokens := mtmd.InputChunkGetTokensText(chunk)
			if err := decodeTextChunk(lctx, tokens, useSeqID, &nPast); err != nil {
				return fmt.Errorf("decode text chunk %d: %w", i, err)
			}

		case mtmd.InputChunkTypeImage:
			fmt.Printf("  Chunk %d: IMAGE (%d tokens)\n", i, nTokens)

			if err := decodeImageChunk(mtmdCtx, lctx, model, chunk, useSeqID, &nPast); err != nil {
				return fmt.Errorf("decode image chunk %d: %w", i, err)
			}

		case mtmd.InputChunkTypeAudio:
			fmt.Printf("  Chunk %d: AUDIO (%d tokens)\n", i, nTokens)

			if err := decodeImageChunk(mtmdCtx, lctx, model, chunk, useSeqID, &nPast); err != nil {
				return fmt.Errorf("decode audio chunk %d: %w", i, err)
			}

		default:
			return fmt.Errorf("unknown chunk type %d", chunkType)
		}
	}

	fmt.Printf("\nPrefill complete. n_past=%d\n", nPast)

	// -------------------------------------------------------------------------
	// Generate response tokens.

	fmt.Print("\nMODEL> ")

	sampler := llama.SamplerChainInit(llama.SamplerChainDefaultParams())
	llama.SamplerChainAdd(sampler, llama.SamplerInitTopK(40))
	llama.SamplerChainAdd(sampler, llama.SamplerInitTopP(0.9, 1))
	llama.SamplerChainAdd(sampler, llama.SamplerInitTempExt(0.7, 0.0, 1.0))
	llama.SamplerChainAdd(sampler, llama.SamplerInitDist(1))
	defer llama.SamplerFree(sampler)

	buf := make([]byte, 256)
	maxTokens := 256

	for i := 0; i < maxTokens; i++ {
		token := llama.SamplerSample(sampler, lctx, -1)
		llama.SamplerAccept(sampler, token)

		if llama.VocabIsEOG(vocab, token) {
			break
		}

		l := llama.TokenToPiece(vocab, token, buf, 0, true)
		fmt.Print(string(buf[:l]))

		batch := llama.BatchGetOne([]llama.Token{token})
		if _, err := llama.Decode(lctx, batch); err != nil {
			return fmt.Errorf("decode failed: %w", err)
		}
		nPast++
	}

	fmt.Println()
	fmt.Printf("\nTotal tokens: %d\n", nPast)

	return nil
}

// decodeTextChunk processes a text chunk by decoding tokens with the specified sequence ID.
func decodeTextChunk(lctx llama.Context, tokens []llama.Token, seqID llama.SeqId, nPast *llama.Pos) error {
	nBatch := int32(512)

	for i := 0; i < len(tokens); i += int(nBatch) {
		end := min(i+int(nBatch), len(tokens))
		batchTokens := tokens[i:end]

		batch := createTokenBatch(batchTokens, seqID, *nPast, end == len(tokens))
		defer llama.BatchFree(batch)

		if _, err := llama.Decode(lctx, batch); err != nil {
			return fmt.Errorf("decode failed: %w", err)
		}

		*nPast += llama.Pos(len(batchTokens))
	}

	return nil
}

// decodeImageChunk encodes an image chunk and decodes its embeddings.
func decodeImageChunk(mtmdCtx mtmd.Context, lctx llama.Context, model llama.Model, chunk mtmd.InputChunk, seqID llama.SeqId, nPast *llama.Pos) error {
	// Step 1: Encode the image chunk through the vision encoder.
	ret := mtmd.EncodeChunk(mtmdCtx, chunk)
	if ret != 0 {
		return fmt.Errorf("encode chunk failed with code %d", ret)
	}

	// Step 2: Get the output embeddings.
	embdPtr := mtmd.GetOutputEmbd(mtmdCtx)
	if embdPtr == nil {
		return fmt.Errorf("get output embd returned nil")
	}

	// Step 3: Create a batch with embeddings and decode.
	nTokens := int32(mtmd.InputChunkGetNTokens(chunk))
	nEmbd := llama.ModelNEmbdInp(model)
	nBatch := int32(512)

	fmt.Printf("    Decoding %d image tokens (n_embd_inp=%d)\n", nTokens, nEmbd)

	for offset := int32(0); offset < nTokens; offset += nBatch {
		batchSize := min(nBatch, nTokens-offset)

		embdOffset := unsafe.Pointer(uintptr(unsafe.Pointer(embdPtr)) + uintptr(offset*nEmbd)*unsafe.Sizeof(float32(0)))

		batch := createEmbdBatch((*float32)(embdOffset), batchSize, nEmbd, seqID, *nPast)
		defer llama.BatchFree(batch)

		if _, err := llama.Decode(lctx, batch); err != nil {
			return fmt.Errorf("decode embeddings failed: %w", err)
		}

		*nPast += llama.Pos(batchSize)
	}

	return nil
}

// createTokenBatch creates a batch for token decoding with explicit sequence ID.
func createTokenBatch(tokens []llama.Token, seqID llama.SeqId, pos0 llama.Pos, logitsLast bool) llama.Batch {
	nTokens := int32(len(tokens))

	batch := llama.BatchInit(nTokens, 0, 1)
	batch.NTokens = nTokens

	for i := int32(0); i < nTokens; i++ {
		tokenPtr := (*llama.Token)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.Token)) + uintptr(i)*unsafe.Sizeof(llama.Token(0))))
		*tokenPtr = tokens[i]

		posPtr := (*llama.Pos)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.Pos)) + uintptr(i)*unsafe.Sizeof(llama.Pos(0))))
		*posPtr = pos0 + llama.Pos(i)

		nSeqPtr := (*int32)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.NSeqId)) + uintptr(i)*unsafe.Sizeof(int32(0))))
		*nSeqPtr = 1

		seqIDPtrPtr := (**llama.SeqId)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.SeqId)) + uintptr(i)*unsafe.Sizeof(uintptr(0))))
		if *seqIDPtrPtr != nil {
			**seqIDPtrPtr = seqID
		}

		logitPtr := (*int8)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.Logits)) + uintptr(i)*unsafe.Sizeof(int8(0))))
		if logitsLast && i == nTokens-1 {
			*logitPtr = 1
		} else {
			*logitPtr = 0
		}
	}

	return batch
}

// createEmbdBatch creates a batch for embedding decoding with explicit sequence ID.
// nEmbd should be llama.ModelNEmbdInp(model).
func createEmbdBatch(embd *float32, nTokens int32, nEmbd int32, seqID llama.SeqId, pos0 llama.Pos) llama.Batch {
	batch := llama.BatchInit(nTokens, nEmbd, 1)
	batch.NTokens = nTokens
	batch.Token = nil
	batch.Embd = embd

	for i := int32(0); i < nTokens; i++ {
		posPtr := (*llama.Pos)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.Pos)) + uintptr(i)*unsafe.Sizeof(llama.Pos(0))))
		*posPtr = pos0 + llama.Pos(i)

		nSeqPtr := (*int32)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.NSeqId)) + uintptr(i)*unsafe.Sizeof(int32(0))))
		*nSeqPtr = 1

		seqIDPtrPtr := (**llama.SeqId)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.SeqId)) + uintptr(i)*unsafe.Sizeof(uintptr(0))))
		if *seqIDPtrPtr != nil {
			**seqIDPtrPtr = seqID
		}

		logitPtr := (*int8)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.Logits)) + uintptr(i)*unsafe.Sizeof(int8(0))))
		if i == nTokens-1 {
			*logitPtr = 1
		} else {
			*logitPtr = 0
		}
	}

	return batch
}
