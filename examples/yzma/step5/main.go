// This example shows how to manually iterate through vision chunks for
// parallel inference where we can process multiple clients.
//
// Demonstrates:
//   1. Chunk inspection and token counting
//   2. Separate tracking of text vs image tokens
//   3. Manual chunk processing with EncodeChunk and GetOutputEmbd
//   4. Batched decoding of text tokens and image embeddings
//
// Run the example like this from the root of the project:
// $ go run ./examples/yzma/step5 -image examples/samples/giraffe.jpg

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
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
	projPath := flag.String("proj", "", "Path to the mmproj file for vision")
	imagePath := flag.String("image", "examples/samples/giraffe.jpg", "Path to the image file")
	prompt := flag.String("prompt", "What is in this image?", "Prompt to ask about the image")
	flag.Parse()

	if *modelPath == "" || *projPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("unable to get home dir: %w", err)
		}
		*modelPath = filepath.Join(home, ".kronk/models/ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/Qwen2.5-VL-3B-Instruct-Q8_0.gguf")
		*projPath = filepath.Join(home, ".kronk/models/ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/mmproj-Qwen2.5-VL-3B-Instruct-Q8_0.gguf")
	}

	if *imagePath == "" {
		*imagePath = "examples/samples/giraffe.jpg"
	}

	// -------------------------------------------------------------------------
	// Initialize yzma (loads both llama and mtmd libraries).

	if err := initYzma(); err != nil {
		return fmt.Errorf("unable to init yzma: %w", err)
	}

	// -------------------------------------------------------------------------
	// Load the model.

	fmt.Println("Loading model...")

	mparams := llama.ModelDefaultParams()
	mdl, err := llama.ModelLoadFromFile(*modelPath, mparams)
	if err != nil {
		return fmt.Errorf("unable to load model: %w", err)
	}
	defer llama.ModelFree(mdl)

	vocab := llama.ModelGetVocab(mdl)

	fmt.Println("Model loaded")

	// -------------------------------------------------------------------------
	// Create llama context with vision-appropriate settings.

	ctxParams := llama.ContextDefaultParams()
	ctxParams.NCtx = 8192
	ctxParams.NBatch = 2048

	lctx, err := llama.InitFromModel(mdl, ctxParams)
	if err != nil {
		return fmt.Errorf("unable to init context: %w", err)
	}
	defer llama.Free(lctx)

	fmt.Printf("Context created: n_ctx=%d, n_batch=%d\n", ctxParams.NCtx, ctxParams.NBatch)

	// -------------------------------------------------------------------------
	// Initialize mtmd context for vision processing.

	mctxParams := mtmd.ContextParamsDefault()
	mtmdCtx, err := mtmd.InitFromFile(*projPath, mdl, mctxParams)
	if err != nil {
		return fmt.Errorf("unable to init mtmd context: %w", err)
	}
	defer mtmd.Free(mtmdCtx)

	if !mtmd.SupportVision(mtmdCtx) {
		return fmt.Errorf("model does not support vision")
	}

	fmt.Println("Vision support: enabled")
	fmt.Printf("Uses M-RoPE: %v\n", mtmd.DecodeUseMRope(mtmdCtx))
	fmt.Printf("Uses NonCausal: %v\n", mtmd.DecodeUseNonCausal(mtmdCtx))

	// -------------------------------------------------------------------------
	// Load and prepare the image.

	fmt.Printf("Loading image: %s\n", *imagePath)

	bitmap := mtmd.BitmapInitFromFile(mtmdCtx, *imagePath)
	if bitmap == 0 {
		return fmt.Errorf("failed to load image: %s", *imagePath)
	}
	defer mtmd.BitmapFree(bitmap)

	fmt.Printf("Image loaded: %dx%d\n", mtmd.BitmapGetNx(bitmap), mtmd.BitmapGetNy(bitmap))

	// -------------------------------------------------------------------------
	// Build the prompt with image marker and apply chat template.

	template := llama.ModelChatTemplate(mdl, "")
	if template == "" {
		template, _ = llama.ModelMetaValStr(mdl, "tokenizer.chat_template")
	}

	userMessage := mtmd.DefaultMarker() + *prompt

	fmt.Printf("\nPrompt: %s\n", *prompt)

	messages := []llama.ChatMessage{
		llama.NewChatMessage("user", userMessage),
	}

	buf := make([]byte, 4096)
	l := llama.ChatApplyTemplate(template, messages, true, buf)
	templatedPrompt := string(buf[:l])

	// -------------------------------------------------------------------------
	// Tokenize the prompt with the image using mtmd.

	output := mtmd.InputChunksInit()
	defer mtmd.InputChunksFree(output)

	input := mtmd.NewInputText(templatedPrompt, true, true)

	result := mtmd.Tokenize(mtmdCtx, output, input, []mtmd.Bitmap{bitmap})
	if result != 0 {
		return fmt.Errorf("tokenization failed with code: %d", result)
	}

	numChunks := mtmd.InputChunksSize(output)
	fmt.Printf("Tokenized into %d chunks\n", numChunks)

	// -------------------------------------------------------------------------
	// CHUNK INSPECTION - Analyze chunks before processing.

	fmt.Println("\nAnalyzing chunks...")

	var textTokens, imageTokens uint32

	for i := range numChunks {
		chunk := mtmd.InputChunksGet(output, i)
		chunkType := mtmd.InputChunkGetType(chunk)
		nTokens := mtmd.InputChunkGetNTokens(chunk)

		switch chunkType {
		case mtmd.InputChunkTypeText:
			textTokens += nTokens
			fmt.Printf("  Chunk %d: TEXT, %d tokens\n", i, nTokens)

		case mtmd.InputChunkTypeImage:
			imageTokens += nTokens
			fmt.Printf("  Chunk %d: IMAGE, %d tokens\n", i, nTokens)

		case mtmd.InputChunkTypeAudio:
			fmt.Printf("  Chunk %d: AUDIO, %d tokens\n", i, nTokens)
		}
	}

	fmt.Printf("\nToken breakdown: %d text + %d image = %d total\n", textTokens, imageTokens, textTokens+imageTokens)

	// -------------------------------------------------------------------------
	// PREFILL - Manual chunk processing for parallel inference demo.

	fmt.Println("\nProcessing chunks manually...")

	nEmbd := llama.ModelNEmbdInp(mdl)

	nPast, err := processChunksManually(mtmdCtx, lctx, mdl, output, numChunks, nEmbd, ctxParams)
	if err != nil {
		return fmt.Errorf("manual chunk processing failed: %w", err)
	}

	fmt.Printf("Prefill complete: n_past=%d\n", nPast)

	// -------------------------------------------------------------------------
	// Create sampler for token generation.

	sampler := llama.SamplerChainInit(llama.SamplerChainDefaultParams())
	defer llama.SamplerFree(sampler)

	llama.SamplerChainAdd(sampler, llama.SamplerInitTopK(40))
	llama.SamplerChainAdd(sampler, llama.SamplerInitTopP(0.9, 1))
	llama.SamplerChainAdd(sampler, llama.SamplerInitTempExt(0.7, 0.0, 1.0))
	llama.SamplerChainAdd(sampler, llama.SamplerInitDist(1))

	// -------------------------------------------------------------------------
	// Generate response tokens.

	fmt.Print("\nResponse: ")

	const maxTokens = 512
	var generatedTokens int

	for range maxTokens {
		token := llama.SamplerSample(sampler, lctx, -1)

		if llama.VocabIsEOG(vocab, token) {
			break
		}

		// Convert token to text.
		buf := make([]byte, 256)
		l := llama.TokenToPiece(vocab, token, buf, 0, true)
		fmt.Print(string(buf[:l]))

		generatedTokens++

		// Feed the token back for next iteration.
		batch := llama.BatchGetOne([]llama.Token{token})
		batch.Pos = &nPast
		llama.Decode(lctx, batch)
		nPast++
	}

	fmt.Printf("\n\nGenerated %d tokens\n", generatedTokens)

	return nil
}

// =============================================================================
// MANUAL CHUNK PROCESSING
// =============================================================================
//
// This section demonstrates how to manually process vision chunks for parallel
// inference. Instead of using mtmd.HelperEvalChunks (which processes everything
// sequentially), we manually:
//
//   1. Iterate through each chunk (text or image)
//   2. For text: decode tokens with proper positioning
//   3. For images: encode with mtmd.EncodeChunk, get embeddings with
//      mtmd.GetOutputEmbd, then decode embeddings into the KV cache
//
// Key insight for M-RoPE models (like Qwen2.5-VL):
//   - Positions are 4-dimensional: [time, y, x, unused]
//   - Text tokens use: [linear_pos, 0, 0, 0]
//   - Image tokens use 2D grid: [pos_0, pos_0+row, pos_0+col, 0]
//   - Position advancement after image = max(nx, ny), not n_tokens
// =============================================================================

// processChunksManually processes vision chunks individually, demonstrating
// the low-level APIs needed for parallel inference across multiple clients.
func processChunksManually(mtmdCtx mtmd.Context, lctx llama.Context, mdl llama.Model,
	output mtmd.InputChunks, numChunks uint64, nEmbd int32,
	ctxParams llama.ContextParams) (llama.Pos, error) {

	var nPast llama.Pos
	useMRoPE := mtmd.DecodeUseMRope(mtmdCtx)

	for i := range numChunks {
		chunk := mtmd.InputChunksGet(output, i)
		chunkType := mtmd.InputChunkGetType(chunk)
		nTokens := mtmd.InputChunkGetNTokens(chunk)

		switch chunkType {
		case mtmd.InputChunkTypeText:
			tokens := model.InputChunkGetTokensText(chunk)
			if len(tokens) == 0 {
				continue
			}

			fmt.Printf("  Processing text chunk %d: %d tokens\n", int(i), len(tokens))

			batchSize := int(ctxParams.NBatch)
			for start := 0; start < len(tokens); start += batchSize {
				end := min(start+batchSize, len(tokens))

				batchTokens := tokens[start:end]
				n := int32(len(batchTokens))

				switch useMRoPE {
				case true:
					if err := decodeTextMRoPE(lctx, batchTokens, &nPast, 0); err != nil {
						return 0, fmt.Errorf("decode text chunk (M-RoPE) failed: %w", err)
					}

				case false:
					batch := llama.BatchGetOne(batchTokens)
					batch.Pos = &nPast

					if _, err := llama.Decode(lctx, batch); err != nil {
						return 0, fmt.Errorf("decode text chunk failed: %w", err)
					}
					nPast += llama.Pos(n)
				}
			}

		case mtmd.InputChunkTypeImage:
			fmt.Printf("  Processing image chunk %d: %d tokens\n", i, nTokens)

			// Step 1: Encode the image chunk (runs through vision encoder)
			if err := mtmd.EncodeChunk(mtmdCtx, chunk); err != nil {
				return 0, fmt.Errorf("encode image chunk failed: %w", err)
			}

			// Step 2: Retrieve the computed embeddings
			embedSize := nEmbd * int32(nTokens)
			embd, err := mtmd.GetOutputEmbd(mtmdCtx, embedSize)
			if err != nil {
				return 0, fmt.Errorf("failed to get image embeddings: %w", err)
			}

			// Step 3: Decode embeddings into the LLM's KV cache
			useNonCausal := mtmd.DecodeUseNonCausal(mtmdCtx)

			switch useMRoPE {
			case true:
				imageTokens := mtmd.InputChunkGetTokensImage(chunk)
				nx := int32(mtmd.ImageTokensGetNX(imageTokens))
				ny := int32(mtmd.ImageTokensGetNY(imageTokens))
				fmt.Printf("    M-RoPE 2D: nx=%d, ny=%d\n", nx, ny)

				if err := decodeEmbeddingsMRoPE(lctx, embd, nEmbd, int32(nTokens), nx, ny, &nPast, 0, useNonCausal); err != nil {
					return 0, fmt.Errorf("decode image embeddings (M-RoPE) failed: %w", err)
				}

			case false:
				if err := decodeEmbeddingsNormal(lctx, embd, nEmbd, int32(nTokens), &nPast, 0, useNonCausal); err != nil {
					return 0, fmt.Errorf("decode image embeddings failed: %w", err)
				}
			}

		case mtmd.InputChunkTypeAudio:
			return 0, fmt.Errorf("audio not supported")
		}
	}

	return nPast, nil
}

// =============================================================================
// BATCH DECODE HELPERS
// =============================================================================

// decodeTextMRoPE decodes text tokens for M-RoPE models.
// M-RoPE uses 4D positions laid out as: [dim0, dim1, dim2, dim3] where each
// dimension has n_tokens entries. For text: dim0=linear position, dims1-3=0.
//
// Memory safety note: We allocate our own position array and must restore
// the original batch.Pos pointer before calling BatchFree to avoid freeing
// Go heap memory from C.
func decodeTextMRoPE(lctx llama.Context, tokens []llama.Token, nPast *llama.Pos, seqID llama.SeqId) error {
	n := int32(len(tokens))
	if n == 0 {
		return nil
	}

	batch := llama.BatchInit(n, 0, 1)

	// Save original pos pointer
	origPos := batch.Pos

	// Access token array
	tokenSlice := unsafe.Slice(batch.Token, int(n))
	copy(tokenSlice, tokens)

	// Allocate 4D position array for M-RoPE
	posData := make([]llama.Pos, n*4)
	pos0 := *nPast
	for i := range n {
		posData[i] = pos0 + llama.Pos(i) // dim 0: linear position
		posData[i+n] = 0                 // dim 1: 0 for text
		posData[i+n*2] = 0               // dim 2: 0 for text
		posData[i+n*3] = 0               // dim 3: 0 for text
	}
	batch.Pos = &posData[0]

	nSeqIDSlice := unsafe.Slice(batch.NSeqId, int(n))
	seqIDPtrs := unsafe.Slice(batch.SeqId, int(n))
	logitsSlice := unsafe.Slice(batch.Logits, int(n))

	for i := range n {
		nSeqIDSlice[i] = 1
		*seqIDPtrs[i] = seqID
		logitsSlice[i] = 0
	}

	if n > 0 {
		logitsSlice[n-1] = 1
	}

	batch.NTokens = n

	_, err := llama.Decode(lctx, batch)

	// Restore and free
	batch.Pos = origPos
	llama.BatchFree(batch)

	if err != nil {
		return err
	}

	*nPast += llama.Pos(n)
	return nil
}

// decodeEmbeddingsNormal decodes image embeddings with standard linear positioning.
// Used for non-M-RoPE models where positions are simply sequential integers.
func decodeEmbeddingsNormal(lctx llama.Context, embd []float32, nEmbd, nTokens int32, nPast *llama.Pos, seqID llama.SeqId, useNonCausal bool) error {
	batch := llama.BatchInit(nTokens, nEmbd, 1)
	defer llama.BatchFree(batch)

	embdSlice := unsafe.Slice(batch.Embd, int(nTokens*nEmbd))
	copy(embdSlice, embd)

	posSlice := unsafe.Slice(batch.Pos, int(nTokens))
	nSeqIDSlice := unsafe.Slice(batch.NSeqId, int(nTokens))
	seqIDPtrs := unsafe.Slice(batch.SeqId, int(nTokens))
	logitsSlice := unsafe.Slice(batch.Logits, int(nTokens))

	for i := range nTokens {
		posSlice[i] = *nPast + llama.Pos(i)
		nSeqIDSlice[i] = 1
		*seqIDPtrs[i] = seqID
		logitsSlice[i] = 0
	}

	if nTokens > 0 {
		logitsSlice[nTokens-1] = 1
	}

	batch.NTokens = nTokens

	if useNonCausal {
		llama.SetCausalAttn(lctx, false)
	}

	if _, err := llama.Decode(lctx, batch); err != nil {
		return err
	}

	if useNonCausal {
		llama.SetCausalAttn(lctx, true)
	}

	*nPast += llama.Pos(nTokens)

	return nil
}

// decodeEmbeddingsMRoPE decodes image embeddings with M-RoPE 2D positioning.
// For M-RoPE, positions are laid out as 4 contiguous arrays:
//
//	[dim0: n_tokens] [dim1: n_tokens] [dim2: n_tokens] [dim3: n_tokens]
//
// For an image grid of nx columns × ny rows:
//   - dim0 (temporal): pos_0 for all tokens
//   - dim1 (row/y):    pos_0 + y
//   - dim2 (col/x):    pos_0 + x
//   - dim3 (unused):   0
//
// CRITICAL: Position advancement after the image must be max(nx, ny), not n_tokens,
// to avoid position overlap with subsequent text tokens.
func decodeEmbeddingsMRoPE(lctx llama.Context, embd []float32, nEmbd, nTokens int32, nx, ny int32, nPast *llama.Pos, seqID llama.SeqId, useNonCausal bool) error {
	// For M-RoPE, we need 4x the position slots (4D positions)
	nPosPerEmbd := int32(4)

	batch := llama.BatchInit(nTokens, nEmbd, 1)

	embdSlice := unsafe.Slice(batch.Embd, int(nTokens*nEmbd))
	copy(embdSlice, embd)

	// Save original pos pointer so BatchFree doesn't try to free Go memory
	origPos := batch.Pos

	// Allocate our own position array for M-RoPE (4D)
	// and replace the batch's pos pointer
	posData := make([]llama.Pos, nTokens*nPosPerEmbd)

	// Set up 2D M-RoPE positions for image grid
	// Layout: positions for dim0, then dim1, then dim2, then dim3
	pos0 := *nPast
	for y := range ny {
		for x := range nx {
			i := y*nx + x
			if i >= nTokens {
				break
			}
			// dim 0: constant pos_0 (temporal/base position)
			posData[i] = pos0
			// dim 1: y position (row)
			posData[i+nTokens] = pos0 + llama.Pos(y)
			// dim 2: x position (column)
			posData[i+nTokens*2] = pos0 + llama.Pos(x)
			// dim 3: unused (always 0)
			posData[i+nTokens*3] = 0
		}
	}
	batch.Pos = &posData[0]

	nSeqIDSlice := unsafe.Slice(batch.NSeqId, int(nTokens))
	seqIDPtrs := unsafe.Slice(batch.SeqId, int(nTokens))
	logitsSlice := unsafe.Slice(batch.Logits, int(nTokens))

	for i := range nTokens {
		nSeqIDSlice[i] = 1
		*seqIDPtrs[i] = seqID
		logitsSlice[i] = 0
	}
	if nTokens > 0 {
		logitsSlice[nTokens-1] = 1
	}
	batch.NTokens = nTokens

	if useNonCausal {
		llama.SetCausalAttn(lctx, false)
	}

	_, err := llama.Decode(lctx, batch)

	// Restore original pos pointer before freeing to avoid freeing Go memory
	batch.Pos = origPos
	llama.BatchFree(batch)

	if err != nil {
		return err
	}

	if useNonCausal {
		llama.SetCausalAttn(lctx, true)
	}

	// For M-RoPE, n_pos is max(nx, ny) to avoid position overlap
	nPos := max(ny, nx)
	*nPast += llama.Pos(nPos)

	return nil
}

func initYzma() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("unable to get home dir: %w", err)
	}

	libPath := filepath.Join(home, ".kronk/libraries")

	if err := llama.Load(libPath); err != nil {
		return fmt.Errorf("unable to load library: %w", err)
	}

	if err := mtmd.Load(libPath); err != nil {
		return fmt.Errorf("unable to load mtmd library: %w", err)
	}

	if err := model.InitYzmaWorkarounds(libPath); err != nil {
		return fmt.Errorf("unable to init yzma workarounds: %w", err)
	}

	llama.Init()
	llama.LogSet(llama.LogSilent())
	mtmd.LogSet(llama.LogSilent())

	return nil
}
