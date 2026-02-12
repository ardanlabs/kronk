// This example shows how to use the low-level yzma/mtmd APIs to perform
// vision inference with a single client. This is the foundation for building
// parallel media inference.
//
// Run the example like this from the root of the project:
// $ go run ./examples/yzma-parallel/step1-media -model <model-path> -proj <proj-path> -image <image-path>

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

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
		//*modelPath = filepath.Join(home, ".kronk/models/unsloth/Qwen3-VL-30B-A3B-Instruct-GGUF/Qwen3-VL-30B-A3B-Instruct-UD-Q8_K_XL.gguf")
		//*projPath = filepath.Join(home, ".kronk/models/unsloth/Qwen3-VL-30B-A3B-Instruct-GGUF/mmproj-Qwen3-VL-30B-A3B-Instruct-UD-Q8_K_XL.gguf")
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

	// The prompt needs to include the media marker where the image goes.
	userMessage := mtmd.DefaultMarker() + *prompt

	messages := []llama.ChatMessage{
		llama.NewChatMessage("user", userMessage),
	}

	buf := make([]byte, 4096)
	l := llama.ChatApplyTemplate(template, messages, true, buf)
	templatedPrompt := string(buf[:l])

	fmt.Printf("\nPrompt: %s\n", *prompt)
	fmt.Printf("Templated prompt length: %d bytes\n", len(templatedPrompt))

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

	// Print chunk info.
	var totalTokens uint64
	for i := range numChunks {
		chunk := mtmd.InputChunksGet(output, i)
		chunkType := mtmd.InputChunkGetType(chunk)
		nTokens := mtmd.InputChunkGetNTokens(chunk)
		totalTokens += nTokens

		typeName := "text"
		switch chunkType {
		case mtmd.InputChunkTypeImage:
			typeName = "image"
		case mtmd.InputChunkTypeAudio:
			typeName = "audio"
		}
		fmt.Printf("  Chunk %d: type=%s, tokens=%d\n", i, typeName, nTokens)
	}
	fmt.Printf("Total input tokens: %d\n", totalTokens)

	// -------------------------------------------------------------------------
	// Evaluate all chunks (prefill stage).

	fmt.Println("\nEvaluating chunks...")

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

	llama.Init()
	llama.LogSet(llama.LogSilent())
	mtmd.LogSet(llama.LogSilent())

	return nil
}
