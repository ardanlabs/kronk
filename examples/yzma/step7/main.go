// This example qualifies embedding model architectures for batched sequence
// processing. It downloads each candidate, evaluates the inputs independently,
// then evaluates them together using distinct sequence IDs in one llama.cpp
// decode call and compares the resulting vectors.
//
// Each candidate runs in a child process because an incompatible model may
// cause llama.cpp to abort instead of returning an error. A failed candidate
// therefore does not prevent the remaining candidates from running.
//
// Run the example like this from the root of the project:
// $ make example-yzma-step7

package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/hybridgroup/yzma/pkg/llama"
)

const (
	batchSize         = 2048
	minimumSimilarity = 0.99999
)

type candidate struct {
	name   string
	source string
	prefix string
}

var candidates = []candidate{
	{
		name:   "Qwen3",
		source: "https://huggingface.co/Qwen/Qwen3-Embedding-0.6B-GGUF/resolve/main/Qwen3-Embedding-0.6B-Q8_0.gguf",
	},
	{
		name:   "BERT/MiniLM",
		source: "https://huggingface.co/second-state/All-MiniLM-L6-v2-Embedding-GGUF/resolve/main/all-MiniLM-L6-v2-Q8_0.gguf",
	},
	{
		name:   "Nomic BERT",
		source: "https://huggingface.co/nomic-ai/nomic-embed-text-v1.5-GGUF/resolve/main/nomic-embed-text-v1.5.Q8_0.gguf",
		prefix: "search_query: ",
	},
	{
		name:   "Jina BERT v2",
		source: "https://huggingface.co/ggml-org/jina-embeddings-v2-base-en-Q8_0-GGUF/resolve/main/jina-embeddings-v2-base-en-q8_0.gguf",
	},
	{
		name:   "EmbeddingGemma",
		source: "https://huggingface.co/ggml-org/embeddinggemma-300m-qat-q8_0-GGUF/resolve/main/embeddinggemma-300m-qat-Q8_0.gguf",
	},
}

func main() {
	candidateIndex := flag.Int("candidate", -1, "candidate index used by the child process")
	modelFile := flag.String("model-file", "", "downloaded model used by the child process")
	flag.Parse()

	var err error
	if *candidateIndex >= 0 {
		err = runCandidate(*candidateIndex, *modelFile)
	} else {
		err = run()
	}

	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

func run() error {
	mdls, err := models.New()
	if err != nil {
		return fmt.Errorf("initialize models: %w", err)
	}

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}

	passed := 0
	failed := 0

	for index, candidate := range candidates {
		fmt.Printf("\n%s\n", candidate.name)
		fmt.Println("Source:", candidate.source)
		fmt.Println("Downloading model if needed...")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		mp, err := mdls.Download(ctx, kronk.FmtLogger, candidate.source)
		cancel()
		if err != nil {
			failed++
			fmt.Println("RESULT: FAIL")
			fmt.Println("Reason:", err)
			continue
		}
		if len(mp.ModelFiles) == 0 {
			failed++
			fmt.Println("RESULT: FAIL")
			fmt.Println("Reason: download returned no model files")
			continue
		}

		cmd := exec.Command(executable,
			"-candidate="+strconv.Itoa(index),
			"-model-file="+mp.ModelFiles[0],
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			failed++
			fmt.Println("RESULT: FAIL")
			fmt.Println("Reason: child process:", err)
			continue
		}

		passed++
		fmt.Println("RESULT: PASS")
	}

	fmt.Printf("\nSummary: %d passed, %d failed\n", passed, failed)
	if failed > 0 {
		return fmt.Errorf("%d embedding candidate(s) failed qualification", failed)
	}

	return nil
}

func runCandidate(index int, modelFile string) error {
	if index >= len(candidates) {
		return fmt.Errorf("candidate index %d is out of range", index)
	}
	if modelFile == "" {
		return fmt.Errorf("candidate %d has no model file", index)
	}

	if err := initYzma(); err != nil {
		return fmt.Errorf("initialize yzma: %w", err)
	}

	candidate := candidates[index]

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
		candidate.prefix + "Why is the sky blue?",
		candidate.prefix + "What causes ocean tides?",
		candidate.prefix + "How do plants convert sunlight into energy through photosynthesis, and why is chlorophyll important to that process?",
		candidate.prefix + "Why is the sky blue?",
	}

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

	architecture := modelMetadata(mdl, "general.architecture")

	fmt.Println("Candidate  :", candidate.name)
	fmt.Println("Architecture:", architecture)
	fmt.Println("Desc       :", llama.ModelDesc(mdl))
	fmt.Println("Dimensions :", nEmbd)
	fmt.Println("Sequences  :", len(inputs))
	fmt.Println("BatchTokens:", totalTokens)

	independent := make([][]float32, len(tokenized))
	for i, tokens := range tokenized {
		vectors, _, err := embed(mdl, [][]llama.Token{tokens}, nEmbd)
		if err != nil {
			return fmt.Errorf("independent input %d: %w", i, err)
		}
		independent[i] = vectors[0]
	}

	batched, poolingType, err := embed(mdl, tokenized, nEmbd)
	if err != nil {
		return fmt.Errorf("batched inputs: %w", err)
	}

	fmt.Printf("PoolingType: %d\n", poolingType)
	fmt.Println("IndependentDecodeCalls:", len(inputs))
	fmt.Println("BatchedDecodeCalls    : 1")

	for i := range inputs {
		similarity := cosineSimilarity(independent[i], batched[i])
		fmt.Printf("Input[%d] tokens=%d similarity=%.8f\n", i, len(tokenized[i]), similarity)
		if similarity < minimumSimilarity {
			return fmt.Errorf("input %d similarity %.8f is below %.5f", i, similarity, minimumSimilarity)
		}
	}

	duplicateSimilarity := cosineSimilarity(batched[0], batched[len(batched)-1])
	fmt.Printf("DuplicateSimilarity   : %.8f\n", duplicateSimilarity)
	if duplicateSimilarity < minimumSimilarity {
		return fmt.Errorf("duplicate similarity %.8f is below %.5f", duplicateSimilarity, minimumSimilarity)
	}

	return nil
}

func embed(mdl llama.Model, tokenized [][]llama.Token, nEmbd int32) ([][]float32, llama.PoolingType, error) {
	totalTokens := 0
	for _, tokens := range tokenized {
		totalTokens += len(tokens)
	}
	if totalTokens > batchSize {
		return nil, 0, fmt.Errorf("batch has %d tokens but batch size is %d", totalTokens, batchSize)
	}

	ctxParams := llama.ContextDefaultParams()
	ctxParams.Embeddings = 1
	ctxParams.NCtx = uint32(batchSize * len(tokenized))
	ctxParams.NBatch = batchSize
	ctxParams.NUbatch = batchSize
	ctxParams.NSeqMax = uint32(len(tokenized))
	ctxParams.KVUnified = 0

	lctx, err := llama.InitFromModel(mdl, ctxParams)
	if err != nil {
		return nil, 0, fmt.Errorf("initialize context: %w", err)
	}
	defer func() {
		llama.Synchronize(lctx)
		llama.Free(lctx)
	}()

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
		return nil, 0, fmt.Errorf("decode: %w", err)
	}
	if ret != 0 {
		return nil, 0, fmt.Errorf("decode returned non-zero: %d", ret)
	}

	vectors := make([][]float32, len(tokenized))
	for i := range tokenized {
		rawVec, err := llama.GetEmbeddingsSeq(lctx, llama.SeqId(i), nEmbd)
		if err != nil {
			return nil, 0, fmt.Errorf("get sequence %d embedding: %w", i, err)
		}
		if len(rawVec) != int(nEmbd) {
			return nil, 0, fmt.Errorf("sequence %d returned %d dimensions, expected %d", i, len(rawVec), nEmbd)
		}

		vectors[i] = append([]float32(nil), rawVec...)
		if err := normalize(vectors[i]); err != nil {
			return nil, 0, fmt.Errorf("normalize sequence %d: %w", i, err)
		}
	}

	return vectors, llama.GetPoolingType(lctx), nil
}

func normalize(vec []float32) error {
	var sum float64
	for _, v := range vec {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return fmt.Errorf("embedding contains a non-finite value")
		}
		sum += float64(v * v)
	}

	if sum == 0 {
		return fmt.Errorf("embedding contains only zero values")
	}

	norm := float32(1.0 / math.Sqrt(sum))
	for i, v := range vec {
		vec[i] = v * norm
	}

	return nil
}

func cosineSimilarity(a, b []float32) float64 {
	var dot float64
	for i, value := range a {
		dot += float64(value * b[i])
	}

	return dot
}

func modelMetadata(mdl llama.Model, wanted string) string {
	for i := range llama.ModelMetaCount(mdl) {
		key, ok := llama.ModelMetaKeyByIndex(mdl, i)
		if !ok || key != wanted {
			continue
		}

		value, ok := llama.ModelMetaValStrByIndex(mdl, i)
		if ok {
			return value
		}
	}

	return "unknown"
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
