// This example shows how to manually iterate through vision chunks, preparing
// for parallel inference where we can process multiple clients.
//
// Currently uses mtmd.HelperEvalChunks for the prefill, but demonstrates:
//   1. Chunk inspection and token counting
//   2. Separate tracking of text vs image tokens
//   3. Structure ready for manual chunk processing when EncodeChunk is fixed
//
// NOTE: Manual chunk processing with mtmd.EncodeChunk crashes due to FFI issues.
// The code is preserved below (commented out) for future investigation.
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
	// Initialize kronk (loads both llama and mtmd libraries).

	if err := kronk.Init(); err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
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

	messages := []llama.ChatMessage{
		llama.NewChatMessage("user", userMessage),
	}

	buf := make([]byte, 4096)
	l := llama.ChatApplyTemplate(template, messages, true, buf)
	templatedPrompt := string(buf[:l])

	fmt.Printf("\nPrompt: %s\n", *prompt)

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
			tokens := mtmd.InputChunkGetTokensText(chunk)
			textTokens += uint32(len(tokens))
			fmt.Printf("  Chunk %d: TEXT, %d tokens\n", i, len(tokens))

		case mtmd.InputChunkTypeImage:
			imageTokens += nTokens
			fmt.Printf("  Chunk %d: IMAGE, %d tokens\n", i, nTokens)

		case mtmd.InputChunkTypeAudio:
			fmt.Printf("  Chunk %d: AUDIO, %d tokens\n", i, nTokens)
		}
	}

	fmt.Printf("\nToken breakdown: %d text + %d image = %d total\n", textTokens, imageTokens, textTokens+imageTokens)

	// -------------------------------------------------------------------------
	// PREFILL - Use HelperEvalChunks (manual chunk processing crashes).
	//
	// NOTE: The code below for manual chunk processing using EncodeChunk
	// crashes with SIGSEGV due to FFI binding issues. Until resolved,
	// we use HelperEvalChunks which handles everything internally.
	//
	// TO TRY MANUAL PROCESSING: Comment out the HelperEvalChunks block below
	// and uncomment processChunksManually:
	//
	// nEmbd := llama.ModelNEmbd(mdl)
	// nPast, err := processChunksManually(mtmdCtx, lctx, output, numChunks, nEmbd, ctxParams)
	// if err != nil {
	//     return err
	// }

	fmt.Println("\nEvaluating chunks with HelperEvalChunks...")

	var nPast llama.Pos
	evalResult := mtmd.HelperEvalChunks(mtmdCtx, lctx, output, 0, 0, int32(ctxParams.NBatch), true, &nPast)
	if evalResult != 0 {
		return fmt.Errorf("eval chunks failed with code: %d", evalResult)
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

	for i := 0; i < maxTokens; i++ {
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
// MANUAL CHUNK PROCESSING - CURRENTLY BROKEN
// =============================================================================
//
// The code below attempts to manually process chunks instead of using
// HelperEvalChunks. This would allow separating image encoding from text
// decoding, enabling parallel text token batching across multiple clients.
//
// PROBLEM: mtmd.EncodeChunk crashes with SIGSEGV when called via FFI.
// The function is correctly loaded (address resolves, other mtmd functions
// work), but the actual call causes a crash. This appears to be an issue
// with how the FFI binding interacts with the C++ internals.
//
// PRESERVED FOR FUTURE INVESTIGATION:
//
// func processChunksManually(mtmdCtx mtmd.Context, lctx llama.Context,
//     output mtmd.InputChunks, numChunks uint32, nEmbd int32,
//     ctxParams llama.ContextParams) (llama.Pos, error) {
//
//     var nPast llama.Pos
//
//     for i := uint32(0); i < numChunks; i++ {
//         chunk := mtmd.InputChunksGet(output, i)
//         chunkType := mtmd.InputChunkGetType(chunk)
//         nTokens := mtmd.InputChunkGetNTokens(chunk)
//
//         switch chunkType {
//         case mtmd.InputChunkTypeText:
//             tokens := mtmd.InputChunkGetTokensText(chunk)
//             if len(tokens) == 0 {
//                 continue
//             }
//
//             // Decode text tokens in batches
//             batchSize := int(ctxParams.NBatch)
//             for start := 0; start < len(tokens); start += batchSize {
//                 end := start + batchSize
//                 if end > len(tokens) {
//                     end = len(tokens)
//                 }
//
//                 batch := llama.BatchGetOne(tokens[start:end])
//                 batch.Pos = &nPast
//
//                 if _, err := llama.Decode(lctx, batch); err != nil {
//                     return 0, fmt.Errorf("decode text chunk failed: %w", err)
//                 }
//
//                 nPast += llama.Pos(end - start)
//             }
//
//         case mtmd.InputChunkTypeImage:
//             // THIS CRASHES - FFI issue with EncodeChunk
//             encResult := mtmd.EncodeChunk(mtmdCtx, chunk)
//             if encResult != 0 {
//                 return 0, fmt.Errorf("encode image chunk failed: %d", encResult)
//             }
//
//             embdPtr := mtmd.GetOutputEmbd(mtmdCtx)
//             if embdPtr == nil {
//                 return 0, fmt.Errorf("failed to get image embeddings")
//             }
//
//             // Decode embeddings...
//             // (additional code to create batch with embeddings)
//
//         case mtmd.InputChunkTypeAudio:
//             return 0, fmt.Errorf("audio not supported")
//         }
//     }
//
//     return nPast, nil
// }
