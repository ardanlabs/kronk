// This example shows how to use the yzma api to perform embeddings directly.
//
// This bypasses the Kronk SDK to isolate whether embedding issues are in the
// SDK layer or the yzma/llama.cpp layer.
//
// This program assumes the model has already been downloaded. Run the
// embedding example first.
//
// Run the example like this from the root of the project:
// $ make example-yzma-step6

package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/hybridgroup/yzma/pkg/llama"
)

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
	// Load the embedding model.

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("unable to get home dir: %w", err)
	}

	modelFile := filepath.Join(home, ".kronk/models/ggml-org/embeddinggemma-300m-qat-q8_0-GGUF/embeddinggemma-300m-qat-Q8_0.gguf")

	fmt.Println("Loading model:", modelFile)

	mparams := llama.ModelDefaultParams()
	mdl, err := llama.ModelLoadFromFile(modelFile, mparams)
	if err != nil {
		return fmt.Errorf("unable to load model: %w", err)
	}
	defer llama.ModelFree(mdl)

	vocab := llama.ModelGetVocab(mdl)

	// -------------------------------------------------------------------------
	// Print model info.

	nEmbd := llama.ModelNEmbd(mdl)
	nClsOut := llama.ModelNClsOut(mdl)
	desc := llama.ModelDesc(mdl)

	fmt.Println("Desc       :", desc)
	fmt.Println("Dimensions :", nEmbd)
	fmt.Println("NClsOut    :", nClsOut)

	// Print relevant metadata.
	count := llama.ModelMetaCount(mdl)
	for i := range count {
		key, ok := llama.ModelMetaKeyByIndex(mdl, i)
		if !ok {
			continue
		}
		val, ok := llama.ModelMetaValStrByIndex(mdl, i)
		if !ok {
			continue
		}

		switch key {
		case "general.architecture", "general.name", "general.type", "general.pooling_type":
			fmt.Printf("Meta       : %s = %s\n", key, val)
		}
	}

	// -------------------------------------------------------------------------
	// Create context with embeddings enabled.

	ctxParams := llama.ContextDefaultParams()
	ctxParams.Embeddings = 1
	ctxParams.NCtx = 2048
	ctxParams.NBatch = 2048
	ctxParams.NUbatch = 512

	fmt.Printf("PoolingType: %d (before InitFromModel)\n", ctxParams.PoolingType)

	lctx, err := llama.InitFromModel(mdl, ctxParams)
	if err != nil {
		return fmt.Errorf("unable to init context: %w", err)
	}
	defer func() {
		llama.Synchronize(lctx)
		llama.Free(lctx)
	}()

	resolvedPooling := llama.GetPoolingType(lctx)
	fmt.Printf("PoolingType: %d (after InitFromModel)\n", resolvedPooling)

	// -------------------------------------------------------------------------
	// Tokenize the input.

	input := "Why is the sky blue?"
	fmt.Println("\nInput:", input)

	tokens := llama.Tokenize(vocab, input, true, true)
	fmt.Println("Tokens:", len(tokens))

	// -------------------------------------------------------------------------
	// Decode with BatchGetOne (simplest approach, matches working Kronk v1.4).

	batch := llama.BatchGetOne(tokens)
	ret, err := llama.Decode(lctx, batch)
	if err != nil {
		return fmt.Errorf("decode failed: %w", err)
	}
	if ret != 0 {
		return fmt.Errorf("decode returned non-zero: %d", ret)
	}

	// -------------------------------------------------------------------------
	// Extract embedding.

	rawVec, err := llama.GetEmbeddingsSeq(lctx, 0, nEmbd)
	if err != nil {
		return fmt.Errorf("GetEmbeddingsSeq failed: %w", err)
	}

	fmt.Printf("Raw length : %d\n", len(rawVec))

	// Count zeros.
	zeros := 0
	for _, v := range rawVec {
		if v == 0 {
			zeros++
		}
	}
	fmt.Printf("Raw zeros  : %d/%d\n", zeros, len(rawVec))

	if len(rawVec) > 0 {
		fmt.Printf("Raw first  : %e\n", rawVec[0])
		fmt.Printf("Raw last   : %e\n", rawVec[len(rawVec)-1])
	}

	// -------------------------------------------------------------------------
	// Normalize.

	vec := make([]float32, len(rawVec))
	copy(vec, rawVec)

	var sum float64
	for _, v := range vec {
		sum += float64(v * v)
	}

	if sum > 0 {
		norm := float32(1.0 / math.Sqrt(sum))
		for i, v := range vec {
			vec[i] = v * norm
		}
	}

	if len(vec) >= 6 {
		fmt.Printf("Normalized : [%v ... %v]\n", vec[:3], vec[len(vec)-3:])
	}

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

	llama.Init()
	llama.LogSet(llama.LogSilent())

	return nil
}
