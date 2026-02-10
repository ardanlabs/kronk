// This example shows how to simulate a server with multiple clients processing
// requests in parallel using llama.cpp's batching capabilities.
//
// This is a Go port of the llama.cpp parallel example:
// https://github.com/ggml-org/llama.cpp/blob/master/examples/parallel/parallel.cpp
//
// Run the example like this from the root of the project:
// $ make example-yzma-parallel

package main

import (
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// Default prompts to use if no external prompts are provided.
var defaultPrompts = []string{
	"Hello, my name is",
	"The president of the United States is",
	"The capital of France is",
	"The future of AI is",
	"What is the meaning of life?",
	"Explain quantum computing in simple terms",
	"Write a haiku about programming",
	"Describe the perfect vacation",
}

// systemPrompt is the default system prompt.
const systemPrompt = "A chat between a curious user and an artificial intelligence assistant. The assistant gives helpful, detailed, and polite answers to the user's questions.\n\n"

// client represents a simulated client making requests.
type client struct {
	id       int32
	seqID    llama.SeqId
	sampled  llama.Token
	tStart   time.Time
	nPast    llama.Pos
	nPrompt  int32
	nDecoded int32
	iBatch   int32
	input    string
	prompt   string
	response strings.Builder
	sampler  llama.Sampler
}

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
	// Parse command line flags.
	modelPath := flag.String("model", "", "Path to the GGUF model file")
	nParallel := flag.Int("parallel", 2, "Number of parallel clients")
	nPredict := flag.Int("predict", 64, "Number of tokens to predict per client")
	nSequences := flag.Int("sequences", 4, "Total number of sequences to process")
	contBatching := flag.Bool("cont-batching", true, "Enable continuous batching")
	sharedPrompt := flag.Bool("shared-prompt", true, "Share system prompt in KV cache")
	flag.Parse()

	if *modelPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("unable to get home dir: %w", err)
		}

		*modelPath = filepath.Join(home, ".kronk/models/Qwen/Qwen3-8B-GGUF/Qwen3-8B-Q8_0.gguf")
	}

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

	// Create context with enough room for parallel sequences.
	// Add 1 to n_parallel for the system prompt sequence.
	ctxParams := llama.ContextDefaultParams()
	ctxParams.NSeqMax = uint32(*nParallel + 1)
	ctxParams.NCtx = 4096

	lctx, err := llama.InitFromModel(mdl, ctxParams)
	if err != nil {
		return fmt.Errorf("unable to init context: %w", err)
	}
	defer llama.Free(lctx)

	mem, err := llama.GetMemory(lctx)
	if err != nil {
		return fmt.Errorf("unable to get memory: %w", err)
	}

	nCtx := llama.NCtx(lctx)

	fmt.Printf("\nSimulating parallel requests:\n")
	fmt.Printf("  n_parallel   = %d\n", *nParallel)
	fmt.Printf("  n_sequences  = %d\n", *nSequences)
	fmt.Printf("  n_predict    = %d\n", *nPredict)
	fmt.Printf("  cont_batching = %v\n", *contBatching)
	fmt.Printf("  shared_prompt = %v\n", *sharedPrompt)
	fmt.Printf("  n_ctx        = %d\n", nCtx)
	fmt.Println()

	// -------------------------------------------------------------------------
	// Initialize clients.

	clients := make([]client, *nParallel)
	for i := range clients {
		clients[i].id = int32(i)
		clients[i].seqID = -1 // Not active
		clients[i].sampler = llama.SamplerChainInit(llama.SamplerChainDefaultParams())
		llama.SamplerChainAdd(clients[i].sampler, llama.SamplerInitTopK(40))
		llama.SamplerChainAdd(clients[i].sampler, llama.SamplerInitTopP(0.9, 1))
		llama.SamplerChainAdd(clients[i].sampler, llama.SamplerInitTempExt(0.7, 0.0, 1.0))
		llama.SamplerChainAdd(clients[i].sampler, llama.SamplerInitDist(uint32(i)+1))
	}
	defer func() {
		for i := range clients {
			llama.SamplerFree(clients[i].sampler)
		}
	}()

	// -------------------------------------------------------------------------
	// Tokenize and evaluate the system prompt.

	tokensSystem := llama.Tokenize(vocab, systemPrompt, true, true)
	nTokensSystem := int32(len(tokensSystem))

	if *sharedPrompt {
		fmt.Println("Evaluating the system prompt...")

		batch := llama.BatchGetOne(tokensSystem)
		if _, err := llama.Decode(lctx, batch); err != nil {
			return fmt.Errorf("failed to decode system prompt: %w", err)
		}

		// Copy the system prompt KV cache to all client sequences.
		for i := 1; i <= *nParallel; i++ {
			if err := llama.MemorySeqCp(mem, 0, llama.SeqId(i), -1, -1); err != nil {
				return fmt.Errorf("failed to copy memory: %w", err)
			}
		}
		fmt.Println()
	}

	fmt.Println("Processing requests...")
	fmt.Println()

	// -------------------------------------------------------------------------
	// Main processing loop.

	var gSeqID int32 = 0 // Global sequence counter
	var nTotalPrompt int32 = 0
	var nTotalGen int32 = 0
	var nCacheMiss int32 = 0

	tMainStart := time.Now()

	// Allocate batch for parallel processing.
	batch := llama.BatchInit(int32(nCtx), 0, int32(*nParallel+1))
	defer llama.BatchFree(batch)

	for {
		// Clear the batch.
		batchClear(&batch)

		// Add tokens from ongoing sequences to the batch.
		for i := range clients {
			c := &clients[i]
			if c.seqID == -1 {
				continue
			}

			c.iBatch = batch.NTokens
			batchAdd(&batch, c.sampled, c.nPast, []llama.SeqId{llama.SeqId(c.id + 1)}, true)
			c.nPast++
			c.nDecoded++
		}

		// If no active sequences, clear the KV cache and prepare for new ones.
		if batch.NTokens == 0 {
			for i := 1; i <= *nParallel; i++ {
				llama.MemorySeqRm(mem, llama.SeqId(i), -1, -1)
				if *sharedPrompt {
					llama.MemorySeqCp(mem, 0, llama.SeqId(i), -1, -1)
				}
			}
		}

		// Insert new sequences for decoding.
		if *contBatching || batch.NTokens == 0 {
			for i := range clients {
				c := &clients[i]
				if c.seqID == -1 && gSeqID < int32(*nSequences) {
					c.seqID = llama.SeqId(gSeqID)
					c.tStart = time.Now()
					c.input = defaultPrompts[rand.Intn(len(defaultPrompts))]
					c.response.Reset()

					p := fmt.Sprintf("<|im_start|>user\n%s<|im_end|>\n<|im_start|>assistant\n", c.input)

					// Build the prompt.
					c.nPast = 0
					switch *sharedPrompt {
					case true:
						c.nPast = llama.Pos(nTokensSystem)
						c.prompt = p
					case false:
						c.prompt = systemPrompt + p
					}

					llama.SamplerReset(c.sampler)

					// Tokenize the prompt (don't prepend BOS if we have system prompt).
					tokensPrompt := llama.Tokenize(vocab, c.prompt, !*sharedPrompt, true)

					// Add prompt tokens to batch.
					for j, tok := range tokensPrompt {
						batchAdd(&batch, tok, c.nPast, []llama.SeqId{llama.SeqId(c.id + 1)}, false)
						c.nPast++
						_ = j
					}

					// Enable logits for the last token.
					if batch.NTokens > 0 {
						setLogit(&batch, batch.NTokens-1, true)
					}

					c.nPrompt = int32(len(tokensPrompt))
					c.nDecoded = 0
					c.iBatch = batch.NTokens - 1

					fmt.Printf("\033[31mClient %3d, seq %4d, prompt = %4d tokens, started...\033[0m\n",
						c.id, c.seqID, c.nPrompt)

					gSeqID++
				}
			}
		}

		// No more work to do.
		if batch.NTokens == 0 {
			break
		}

		// Decode the batch.
		ret, err := llama.Decode(lctx, batch)
		if err != nil || ret != 0 {
			nCacheMiss++
			fmt.Printf("Warning: decode failed (cache miss %d), ret=%d\n", nCacheMiss, ret)
			// In production, you'd retry with smaller batch or handle the error.
			continue
		}

		// Sample tokens for each active client.
		for i := range clients {
			c := &clients[i]
			if c.iBatch < 0 {
				continue
			}

			// Sample the next token.
			token := llama.SamplerSample(c.sampler, lctx, c.iBatch)
			llama.SamplerAccept(c.sampler, token)

			// Convert token to text.
			buf := make([]byte, 256)
			l := llama.TokenToPiece(vocab, token, buf, 0, true)
			tokenStr := string(buf[:l])

			c.response.WriteString(tokenStr)
			c.sampled = token

			// Check for end of generation.
			shouldStop := false
			if c.nDecoded > 2 {
				if llama.VocabIsEOG(vocab, token) {
					shouldStop = true
				}
				if c.nDecoded >= int32(*nPredict) {
					shouldStop = true
				}
				if strings.Contains(c.response.String(), "User:") {
					shouldStop = true
					// Trim the "User:" part.
					resp := c.response.String()
					if before, _, ok := strings.Cut(resp, "User:"); ok {
						c.response.Reset()
						c.response.WriteString(before)
					}
				}
			}

			if shouldStop {
				// Remove this client's sequence from KV cache.
				llama.MemorySeqRm(mem, llama.SeqId(c.id+1), -1, -1)
				if *sharedPrompt {
					llama.MemorySeqCp(mem, 0, llama.SeqId(c.id+1), -1, -1)
				}

				elapsed := time.Since(c.tStart)
				speed := float64(c.nPrompt+c.nDecoded) / elapsed.Seconds()

				fmt.Printf("\033[32mClient %3d, seq %3d/%3d, prompt %4d t, response %4d t, time %5.2f s, speed %5.2f t/s\033[0m\n",
					c.id, c.seqID, *nSequences, c.nPrompt, c.nDecoded, elapsed.Seconds(), speed)
				fmt.Printf("\nInput:    %s\n", strings.TrimSpace(c.input))
				fmt.Printf("Response: %s\n\n", strings.TrimSpace(c.response.String()))

				nTotalPrompt += c.nPrompt
				nTotalGen += c.nDecoded

				c.seqID = -1 // Mark as inactive
			}

			c.iBatch = -1
		}
	}

	// -------------------------------------------------------------------------
	// Print summary.

	elapsed := time.Since(tMainStart)

	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Parallel inference completed at %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("n_parallel    = %d\n", *nParallel)
	fmt.Printf("n_sequences   = %d\n", *nSequences)
	fmt.Printf("cont_batching = %v\n", *contBatching)
	fmt.Printf("Total prompt tokens:  %6d, speed: %5.2f t/s\n",
		nTotalPrompt, float64(nTotalPrompt)/elapsed.Seconds())
	fmt.Printf("Total gen tokens:     %6d, speed: %5.2f t/s\n",
		nTotalGen, float64(nTotalGen)/elapsed.Seconds())
	fmt.Printf("Total speed (AVG):           speed: %5.2f t/s\n",
		float64(nTotalPrompt+nTotalGen)/elapsed.Seconds())
	fmt.Printf("Cache misses:         %6d\n", nCacheMiss)
	fmt.Printf("Total time:           %5.2f s\n", elapsed.Seconds())
	fmt.Println()

	return nil
}

// =============================================================================
// Batch manipulation helpers using unsafe pointer arithmetic.
// These mirror the common_batch_add / common_batch_clear from llama.cpp.

// batchClear resets the batch to empty state.
func batchClear(batch *llama.Batch) {
	batch.NTokens = 0
}

// batchAdd adds a token to the batch at the current position.
func batchAdd(batch *llama.Batch, token llama.Token, pos llama.Pos, seqIDs []llama.SeqId, logits bool) {
	i := batch.NTokens

	// Set token.
	tokenPtr := (*llama.Token)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.Token)) + uintptr(i)*unsafe.Sizeof(llama.Token(0))))
	*tokenPtr = token

	// Set position.
	posPtr := (*llama.Pos)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.Pos)) + uintptr(i)*unsafe.Sizeof(llama.Pos(0))))
	*posPtr = pos

	// Set number of sequence IDs.
	nSeqPtr := (*int32)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.NSeqId)) + uintptr(i)*unsafe.Sizeof(int32(0))))
	*nSeqPtr = int32(len(seqIDs))

	// Set sequence IDs.
	// SeqId is **SeqId, so we need to get the pointer to the array of SeqId pointers.
	seqIDPtrPtr := (**llama.SeqId)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.SeqId)) + uintptr(i)*unsafe.Sizeof(uintptr(0))))
	if *seqIDPtrPtr != nil && len(seqIDs) > 0 {
		for j, sid := range seqIDs {
			seqPtr := (*llama.SeqId)(unsafe.Pointer(uintptr(unsafe.Pointer(*seqIDPtrPtr)) + uintptr(j)*unsafe.Sizeof(llama.SeqId(0))))
			*seqPtr = sid
		}
	}

	// Set logits flag.
	logitPtr := (*int8)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.Logits)) + uintptr(i)*unsafe.Sizeof(int8(0))))
	switch logits {
	case true:
		*logitPtr = 1
	case false:
		*logitPtr = 0
	}

	batch.NTokens++
}

// setLogit sets the logit flag for a specific token index.
func setLogit(batch *llama.Batch, idx int32, logits bool) {
	logitPtr := (*int8)(unsafe.Pointer(uintptr(unsafe.Pointer(batch.Logits)) + uintptr(idx)*unsafe.Sizeof(int8(0))))
	switch logits {
	case true:
		*logitPtr = 1
	case false:
		*logitPtr = 0
	}
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

	llama.Init()
	llama.LogSet(llama.LogSilent())

	return nil
}
