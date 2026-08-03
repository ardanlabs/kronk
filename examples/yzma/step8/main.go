// This example qualifies reranker model architectures for batched sequence
// processing. It downloads each candidate, evaluates query-document pairs
// independently, then evaluates them together using distinct sequence IDs in
// one llama.cpp decode call and compares the classifier outputs.
//
// Each candidate runs in a child process because an incompatible model may
// cause llama.cpp to abort instead of returning an error. A failed candidate
// therefore does not prevent the remaining candidates from running.
//
// Run the example like this from the root of the project:
// $ make example-yzma-step8

package main

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/hybridgroup/yzma/pkg/llama"
)

const (
	batchSize      = 2048
	scoreTolerance = 0.001
)

type candidate struct {
	name   string
	source string
}

var candidates = []candidate{
	{
		name:   "BGE v2 M3",
		source: "https://huggingface.co/gpustack/bge-reranker-v2-m3-GGUF/resolve/main/bge-reranker-v2-m3-Q8_0.gguf",
	},
	{
		name:   "Qwen3",
		source: "https://huggingface.co/ggml-org/Qwen3-Reranker-0.6B-Q8_0-GGUF/resolve/main/qwen3-reranker-0.6b-q8_0.gguf",
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
		return fmt.Errorf("%d reranker candidate(s) failed qualification", failed)
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
		return fmt.Errorf("load model: %w", err)
	}
	defer llama.ModelFree(mdl)

	vocab := llama.ModelGetVocab(mdl)
	nClsOut := llama.ModelNClsOut(mdl)
	if nClsOut == 0 {
		nClsOut = 1
	}

	query := "What is the capital of France?"
	documents := []string{
		"Paris is the capital and largest city of France.",
		"Berlin is the capital of Germany.",
		"France is a country in Western Europe with Paris as its capital, largest city, and an important center of culture and commerce.",
		"Paris is the capital and largest city of France.",
	}

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

	fmt.Println("Candidate   :", candidate.name)
	fmt.Println("Architecture:", modelMetadata(mdl, "general.architecture"))
	fmt.Println("Desc        :", llama.ModelDesc(mdl))
	fmt.Println("NClsOut     :", nClsOut)
	fmt.Println("Sequences   :", len(documents))
	fmt.Println("BatchTokens :", totalTokens)
	fmt.Println("Query       :", query)

	independent := make([][]float32, len(tokenized))
	for i, tokens := range tokenized {
		outputs, _, err := rerank(mdl, [][]llama.Token{tokens}, nClsOut)
		if err != nil {
			return fmt.Errorf("independent document %d: %w", i, err)
		}
		independent[i] = outputs[0]
	}

	batched, poolingType, err := rerank(mdl, tokenized, nClsOut)
	if err != nil {
		return fmt.Errorf("batched documents: %w", err)
	}

	fmt.Printf("PoolingType            : %d\n", poolingType)
	fmt.Println("IndependentDecodeCalls:", len(documents))
	fmt.Println("BatchedDecodeCalls    : 1")

	independentScores := make([]float32, len(documents))
	batchedScores := make([]float32, len(documents))
	var maximumScoreDelta float64
	scoreWithinTolerance := true
	for i := range documents {
		independentScore := sigmoid(independent[i][0])
		batchedScore := sigmoid(batched[i][0])
		rawDelta := maximumDelta(independent[i], batched[i])
		scoreDelta := math.Abs(float64(independentScore - batchedScore))
		independentScores[i] = independentScore
		batchedScores[i] = batchedScore
		maximumScoreDelta = max(maximumScoreDelta, scoreDelta)
		fmt.Printf("Document[%d] tokens=%d score=%.8f score-delta=%.8g raw-delta=%.8g\n", i, len(tokenized[i]), batchedScore, scoreDelta, rawDelta)
		if scoreDelta > scoreTolerance {
			scoreWithinTolerance = false
		}
	}

	duplicateRawDelta := maximumDelta(batched[0], batched[len(batched)-1])
	duplicateScoreDelta := math.Abs(float64(sigmoid(batched[0][0]) - sigmoid(batched[len(batched)-1][0])))
	// Exclude the final duplicate from ranking comparison. Its order relative
	// to the original is undefined when their scores differ only by rounding.
	independentRanking := ranking(independentScores[:len(independentScores)-1])
	batchedRanking := ranking(batchedScores[:len(batchedScores)-1])
	rankingMatches := slices.Equal(independentRanking, batchedRanking)

	fmt.Printf("MaximumScoreDelta      : %.8g\n", maximumScoreDelta)
	fmt.Printf("DuplicateScoreDelta    : %.8g\n", duplicateScoreDelta)
	fmt.Printf("DuplicateRawDelta      : %.8g\n", duplicateRawDelta)
	fmt.Println("IndependentRanking     :", independentRanking)
	fmt.Println("BatchedRanking         :", batchedRanking)
	fmt.Println("RankingMatches         :", rankingMatches)
	fmt.Printf("ScoreTolerance         : %.4g (%t)\n", scoreTolerance, scoreWithinTolerance)

	if !rankingMatches {
		return fmt.Errorf("batched ranking %v differs from independent ranking %v", batchedRanking, independentRanking)
	}
	if !scoreWithinTolerance {
		return fmt.Errorf("maximum relevance-score delta %.8g exceeds %.4g", maximumScoreDelta, scoreTolerance)
	}
	if duplicateScoreDelta > scoreTolerance {
		return fmt.Errorf("duplicate relevance-score delta %.8g exceeds %.4g", duplicateScoreDelta, scoreTolerance)
	}

	return nil
}

func rerank(mdl llama.Model, tokenized [][]llama.Token, nClsOut uint32) ([][]float32, llama.PoolingType, error) {
	totalTokens := 0
	for _, tokens := range tokenized {
		totalTokens += len(tokens)
	}
	if totalTokens > batchSize {
		return nil, 0, fmt.Errorf("batch has %d tokens but batch size is %d", totalTokens, batchSize)
	}

	ctxParams := llama.ContextDefaultParams()
	ctxParams.Embeddings = 1
	ctxParams.PoolingType = llama.PoolingTypeRank
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

	outputs := make([][]float32, len(tokenized))
	for i := range tokenized {
		raw, err := llama.GetEmbeddingsSeq(lctx, llama.SeqId(i), int32(nClsOut))
		if err != nil {
			return nil, 0, fmt.Errorf("get sequence %d output: %w", i, err)
		}
		if len(raw) != int(nClsOut) {
			return nil, 0, fmt.Errorf("sequence %d returned %d outputs, expected %d", i, len(raw), nClsOut)
		}

		outputs[i] = append([]float32(nil), raw...)
		for _, value := range outputs[i] {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return nil, 0, fmt.Errorf("sequence %d output contains a non-finite value", i)
			}
		}
	}

	return outputs, llama.GetPoolingType(lctx), nil
}

func maximumDelta(a, b []float32) float64 {
	var maximum float64
	for i, value := range a {
		maximum = max(maximum, math.Abs(float64(value-b[i])))
	}

	return maximum
}

func ranking(scores []float32) []int {
	indices := make([]int, len(scores))
	for i := range indices {
		indices[i] = i
	}

	slices.SortFunc(indices, func(a, b int) int {
		if scoreOrder := cmp.Compare(scores[b], scores[a]); scoreOrder != 0 {
			return scoreOrder
		}
		return cmp.Compare(a, b)
	})

	return indices
}

func sigmoid(value float32) float32 {
	return float32(1.0 / (1.0 + math.Exp(-float64(value))))
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
		return fmt.Errorf("load library: %w", err)
	}

	llama.Init()
	llama.LogSet(llama.LogSilent())

	return nil
}
