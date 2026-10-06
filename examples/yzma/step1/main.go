// This example shows you how to use the yzma api at a basic level.
//
// This program assumes the model has already been downloaded. Run the
// chat example first.
//
// Run the example like this from the root of the project:
// $ make example-yzma

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/examples/yzma/internal/yzmainit"
	"github.com/hybridgroup/yzma/pkg/llama"
)

func main() {
	if err := run(); err != nil {
		if errors.Is(err, io.EOF) {
			return
		}

		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	if err := initYzma(); err != nil {
		return fmt.Errorf("unable to init yzma: %w", err)
	}

	// -------------------------------------------------------------------------
	// Load the model from disk into memory, extract the vocabulary, and create
	// a context for inference.

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("unable to get home dir: %w", err)
	}

	modelFile := filepath.Join(home, ".kronk/models/unsloth/Qwen3-0.6B-GGUF/Qwen3-0.6B-Q8_0.gguf")

	mparams := llama.ModelDefaultParams()
	mparams.LoadMode = llama.LoadModeMmap

	fmt.Println("Loading Model From File...")

	mdl, err := llama.ModelLoadFromFile(modelFile, mparams)
	if err != nil {
		return fmt.Errorf("unable to load model: %w", err)
	}
	defer llama.ModelFree(mdl)

	vocab := llama.ModelGetVocab(mdl)

	ctxParams := llama.ContextDefaultParams()
	lctx, err := llama.InitFromModel(mdl, ctxParams)
	if err != nil {
		return fmt.Errorf("unable to init context: %w", err)
	}
	defer llama.Free(lctx)

	fmt.Println()

	// -------------------------------------------------------------------------
	// Build the prompt by applying the model's chat template to the messages.

	template := llama.ModelChatTemplate(mdl, "")
	if template == "" {
		template, _ = llama.ModelMetaValStr(mdl, "tokenizer.chat_template")
	}

	messages := make([]llama.ChatMessage, 0, 1)
	messages = append(messages, llama.NewChatMessage("user", "hello model"))

	buf := make([]byte, 1024)
	l := llama.ChatApplyTemplate(template, messages, true, buf)
	prompt := string(buf[:l])

	fmt.Println(prompt)

	// -------------------------------------------------------------------------
	// Create a sampler chain for selecting output tokens.

	sampler := llama.SamplerChainInit(llama.SamplerChainDefaultParams())
	llama.SamplerChainAdd(sampler, llama.SamplerInitDist(llama.DefaultSeed))

	// -------------------------------------------------------------------------
	// Tokenize the prompt into a sequence of token IDs.

	tokens := llama.Tokenize(vocab, prompt, true, true)
	inputTokens := len(tokens)

	fmt.Println("LEN INPUT TOKENS", inputTokens)
	fmt.Println()

	// -------------------------------------------------------------------------
	// Create a context-sized extended batch and perform the prefill step by
	// adding all input tokens with their positions.

	batch, err := llama.BatchExtInit(lctx)
	if err != nil {
		return fmt.Errorf("unable to initialize batch: %w", err)
	}
	defer llama.BatchExtFree(batch)

	for i, token := range tokens {
		idx, err := llama.BatchExtAddToken(batch, 0, token)
		if err != nil {
			return fmt.Errorf("unable to add prefill token %d: %w", i, err)
		}
		if err := llama.BatchExtSetPos(batch, idx, llama.Pos(i)); err != nil {
			return fmt.Errorf("unable to set prefill position %d: %w", i, err)
		}
	}
	if err := llama.BatchExtSetOutputLogits(batch, int32(len(tokens)-1), true); err != nil {
		return fmt.Errorf("unable to request prefill logits: %w", err)
	}
	ret, err := llama.Process(lctx, llama.ProcessTypeDecode, batch)
	if err != nil {
		return fmt.Errorf("unable to process prefill batch: %w", err)
	}
	if ret != 0 {
		return fmt.Errorf("unable to process prefill batch: code=%d", ret)
	}
	llama.Synchronize(lctx)

	nPast := llama.Pos(len(tokens))

	// -------------------------------------------------------------------------
	// Generate output tokens one at a time until end-of-generation.

	for {
		// Sample the next token from the model's output distribution.
		token := llama.SamplerSample(sampler, lctx, -1)

		// Check if the model produced an end-of-generation token.
		if llama.VocabIsEOG(vocab, token) {
			return io.EOF
		}

		// Convert the token ID to its text representation.
		buf := make([]byte, 512)
		l := llama.TokenToPiece(vocab, token, buf, 0, true)

		// Get the string content and check it's not empty.
		content := string(buf[:l])
		if content == "" {
			return io.EOF
		}

		fmt.Print(content)

		// Feed the sampled token back into the model for the next iteration.
		if err := llama.BatchExtClear(batch); err != nil {
			return fmt.Errorf("unable to clear batch: %w", err)
		}
		idx, err := llama.BatchExtAddToken(batch, 0, token)
		if err != nil {
			return fmt.Errorf("unable to add generated token: %w", err)
		}
		if err := llama.BatchExtSetPos(batch, idx, nPast); err != nil {
			return fmt.Errorf("unable to set generated token position: %w", err)
		}
		if err := llama.BatchExtSetOutputLogits(batch, idx, true); err != nil {
			return fmt.Errorf("unable to request generated token logits: %w", err)
		}
		ret, err := llama.Process(lctx, llama.ProcessTypeDecode, batch)
		if err != nil {
			return fmt.Errorf("unable to process generated token: %w", err)
		}
		if ret != 0 {
			return fmt.Errorf("unable to process generated token: code=%d", ret)
		}
		llama.Synchronize(lctx)
		nPast++
	}
}

func initYzma() error {
	libPath, err := yzmainit.LibraryPath()
	if err != nil {
		return fmt.Errorf("prepare library path: %w", err)
	}

	if err := llama.Load(libPath); err != nil {
		return fmt.Errorf("unable to load library: %w", err)
	}

	if err := llama.Init(); err != nil {
		return fmt.Errorf("unable to initialize llama: %w", err)
	}
	llama.LogSet(llama.LogSilent())

	return nil
}
