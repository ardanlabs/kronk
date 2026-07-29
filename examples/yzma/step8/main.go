// This example shows how to use the yzma api to perform batched reranking.
// Multiple query-document pairs are assigned unique sequence IDs and processed
// by one model, one context, and one llama.cpp decode call.
//
// This program assumes the BGE reranker model has already been downloaded.
//
// Run the example like this from the root of the project:
// $ make example-yzma-step8

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
	// Load one reranker model.

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("unable to get home dir: %w", err)
	}

	modelFile := filepath.Join(home, ".kronk/models/gpustack/bge-reranker-v2-m3-GGUF/bge-reranker-v2-m3-Q8_0.gguf")

	fmt.Println("Loading model:", modelFile)

	mdl, err := llama.ModelLoadFromFile(modelFile, llama.ModelDefaultParams())
	if err != nil {
		return fmt.Errorf("unable to load model: %w", err)
	}
	defer llama.ModelFree(mdl)

	vocab := llama.ModelGetVocab(mdl)
	nClsOut := llama.ModelNClsOut(mdl)
	if nClsOut <= 0 {
		return fmt.Errorf("model returned invalid classifier output size: %d", nClsOut)
	}

	query := "What is the capital of France?"
	documents := []string{
		"Paris is the capital and largest city of France.",
		"Berlin is the capital of Germany.",
		"The Eiffel Tower is located in Paris.",
		"London is the capital of England.",
		"France is a country in Western Europe.",
	}

	// -------------------------------------------------------------------------
	// Tokenize every query-document pair before constructing the combined batch.

	tokenized := make([][]llama.Token, len(documents))
	totalTokens := 0

	for i, document := range documents {
		pair := fmt.Sprintf("%s %s", query, document)
		tokens := llama.Tokenize(vocab, pair, true, true)
		if len(tokens) == 0 {
			return fmt.Errorf("document[%d] produced no tokens", i)
		}

		tokenized[i] = tokens
		totalTokens += len(tokens)
	}

	if totalTokens > batchSize {
		return fmt.Errorf("combined input has %d tokens but batch size is %d", totalTokens, batchSize)
	}

	fmt.Println("Desc       :", llama.ModelDesc(mdl))
	fmt.Println("NClsOut    :", nClsOut)
	fmt.Println("Sequences  :", len(documents))
	fmt.Println("BatchTokens:", totalTokens)
	fmt.Println("Query      :", query)

	// -------------------------------------------------------------------------
	// Create one rank-pooling context capable of processing every pair in one
	// batch. Keep the physical and logical limits equal for non-causal models.

	ctxParams := llama.ContextDefaultParams()
	ctxParams.Embeddings = 1
	ctxParams.PoolingType = llama.PoolingTypeRank
	ctxParams.NCtx = batchSize * uint32(len(documents))
	ctxParams.NBatch = batchSize
	ctxParams.NUbatch = batchSize
	ctxParams.NSeqMax = uint32(len(documents))
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
	// Combine all pairs into one batch. Positions restart at zero for each pair,
	// and every pair receives a distinct sequence ID.

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
	// Retrieve one classifier output for each sequence ID and convert its raw
	// logit to the same [0, 1] score returned by Kronk's reranking API.

	for i, document := range documents {
		rawScore, err := llama.GetEmbeddingsSeq(lctx, llama.SeqId(i), int32(nClsOut))
		if err != nil {
			return fmt.Errorf("get score for sequence %d: %w", i, err)
		}
		if len(rawScore) != int(nClsOut) {
			return fmt.Errorf("sequence %d returned %d classifier outputs, expected %d", i, len(rawScore), nClsOut)
		}

		score := sigmoid(rawScore[0])

		fmt.Printf("\nSequence %d\n", i)
		fmt.Println("Document   :", document)
		fmt.Println("Tokens     :", len(tokenized[i]))
		fmt.Printf("RawScore   : %.6f\n", rawScore[0])
		fmt.Printf("Score      : %.6f\n", score)
	}

	return nil
}

func sigmoid(value float32) float32 {
	return float32(1.0 / (1.0 + math.Exp(-float64(value))))
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
