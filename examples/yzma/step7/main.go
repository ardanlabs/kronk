// This example shows how to use the yzma api to perform batched embeddings.
// Multiple independent inputs are assigned unique sequence IDs and processed
// by one model, one context, and one llama.cpp decode call.
//
// This program assumes the Qwen3 embedding model has already been downloaded.
//
// Run the example like this from the root of the project:
// $ make example-yzma-step7

package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/hybridgroup/yzma/pkg/llama"
)

const batchSize = 2048

func main() {
	if err := run(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

func run() error {
	if err := initYzma(); err != nil {
		return fmt.Errorf("unable to init yzma: %w", err)
	}

	// -------------------------------------------------------------------------
	// Load one embedding model.

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("unable to get home dir: %w", err)
	}

	modelFile := filepath.Join(home, ".kronk/models/Qwen/Qwen3-Embedding-0.6B-GGUF/Qwen3-Embedding-0.6B-Q8_0.gguf")

	fmt.Println("Loading model:", modelFile)

	mdl, err := llama.ModelLoadFromFile(modelFile, llama.ModelDefaultParams())
	if err != nil {
		return fmt.Errorf("unable to load model: %w", err)
	}
	defer llama.ModelFree(mdl)

	vocab := llama.ModelGetVocab(mdl)
	nEmbd := llama.ModelNEmbdOut(mdl)
	if nEmbd <= 0 {
		return fmt.Errorf("model returned invalid output embedding size: %d", nEmbd)
	}

	inputs := []string{
		"Why is the sky blue?",
		"What causes ocean tides?",
		"How do plants convert sunlight into energy?",
	}

	// -------------------------------------------------------------------------
	// Tokenize every sequence before constructing the combined batch.

	tokenized := make([][]llama.Token, len(inputs))
	totalTokens := 0

	for i, input := range inputs {
		tokens := llama.Tokenize(vocab, input, true, true)
		if len(tokens) == 0 {
			return fmt.Errorf("input[%d] produced no tokens", i)
		}

		tokenized[i] = tokens
		totalTokens += len(tokens)
	}

	if totalTokens > batchSize {
		return fmt.Errorf("combined input has %d tokens but batch size is %d", totalTokens, batchSize)
	}

	fmt.Println("Desc       :", llama.ModelDesc(mdl))
	fmt.Println("Dimensions :", nEmbd)
	fmt.Println("Sequences  :", len(inputs))
	fmt.Println("BatchTokens:", totalTokens)

	// -------------------------------------------------------------------------
	// Create one context capable of processing every sequence in one batch.
	// Keep the physical and logical limits equal so this also works for
	// non-causal embedding models, whose batches cannot be split into ubatches.

	ctxParams := llama.ContextDefaultParams()
	ctxParams.Embeddings = 1
	ctxParams.NCtx = batchSize * uint32(len(inputs))
	ctxParams.NBatch = batchSize
	ctxParams.NUbatch = batchSize
	ctxParams.NSeqMax = uint32(len(inputs))
	ctxParams.KVUnified = 1

	lctx, err := llama.InitFromModel(mdl, ctxParams)
	if err != nil {
		return fmt.Errorf("unable to init context: %w", err)
	}
	defer func() {
		llama.Synchronize(lctx)
		llama.Free(lctx)
	}()

	fmt.Printf("PoolingType: %d\n", llama.GetPoolingType(lctx))
	fmt.Printf("NSeqMax    : %d\n", llama.NSeqMax(lctx))

	// -------------------------------------------------------------------------
	// Combine all inputs into one batch. Positions restart at zero for each
	// sequence, and every input receives a distinct sequence ID.

	batch := llama.BatchInit(int32(totalTokens), 0, 1)
	defer llama.BatchFree(batch)

	for i, tokens := range tokenized {
		seqID := llama.SeqId(i)
		for pos, token := range tokens {
			batch.Add(token, llama.Pos(pos), []llama.SeqId{seqID}, true)
		}
	}

	ret, err := llama.Decode(lctx, batch)
	if err != nil {
		return fmt.Errorf("decode failed: %w", err)
	}
	if ret != 0 {
		return fmt.Errorf("decode returned non-zero: %d", ret)
	}

	fmt.Println("DecodeCalls: 1")

	// -------------------------------------------------------------------------
	// Retrieve one pooled embedding for each sequence ID.

	for i, input := range inputs {
		rawVec, err := llama.GetEmbeddingsSeq(lctx, llama.SeqId(i), nEmbd)
		if err != nil {
			return fmt.Errorf("get embedding for sequence %d: %w", i, err)
		}
		if len(rawVec) != int(nEmbd) {
			return fmt.Errorf("sequence %d returned %d dimensions, expected %d", i, len(rawVec), nEmbd)
		}

		vec := make([]float32, len(rawVec))
		copy(vec, rawVec)
		normalize(vec)

		fmt.Printf("\nSequence %d\n", i)
		fmt.Println("Input      :", input)
		fmt.Println("Tokens     :", len(tokenized[i]))
		fmt.Printf("Embedding  : [%v ... %v]\n", vec[:3], vec[len(vec)-3:])
	}

	return nil
}

func normalize(vec []float32) {
	var sum float64
	for _, v := range vec {
		sum += float64(v * v)
	}

	if sum == 0 {
		return
	}

	norm := float32(1.0 / math.Sqrt(sum))
	for i, v := range vec {
		vec[i] = v * norm
	}
}

func initYzma() error {
	libPath := libs.Path("")

	if err := llama.Load(libPath); err != nil {
		return fmt.Errorf("unable to load library: %w", err)
	}

	llama.Init()
	llama.LogSet(llama.LogSilent())

	return nil
}
