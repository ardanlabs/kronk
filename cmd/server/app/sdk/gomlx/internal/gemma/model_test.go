package gemma

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/gomlx/internal/gguf"
)

func TestLoadBaseModel(t *testing.T) {
	config := testConfig()
	path := filepath.Join(t.TempDir(), "gemma.gguf")
	writeTestModel(t, path, config, "gemma3")

	base, err := LoadBaseModel(path)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()

	if base.Config() != config {
		t.Fatalf("config = %+v, want %+v", base.Config(), config)
	}
	if base.store.NumVariables() != len(tensorSpecs(config))+1 {
		t.Fatalf("variables = %d, want %d", base.store.NumVariables(), len(tensorSpecs(config))+1)
	}
	for variable := range base.store.IterVariables() {
		if variable.Trainable {
			t.Fatalf("base variable %q is trainable", variable.Path())
		}
	}
	query := base.store.GetVariable("/layer_0/self_attn/MultiHeadAttention/query/dense/weights")
	if query == nil {
		t.Fatal("query projection is missing")
	}
	queryShape := []int{config.EmbeddingDimension, config.AttentionHeads * config.HeadDimension}
	if !slices.Equal(query.Shape().Dimensions, queryShape) {
		t.Fatalf("query projection shape = %v, want %v", query.Shape().Dimensions, queryShape)
	}

	norm := base.store.GetVariable("/final_norm/rms_norm/scale")
	value, err := norm.Value()
	if err != nil {
		t.Fatal(err)
	}
	var normData []float32
	if err := value.ConstFlatData(func(flat any) { normData = slices.Clone(flat.([]float32)) }); err != nil {
		t.Fatal(err)
	}
	if normData[0] != 0 {
		t.Fatalf("RMS offset value = %g, want 0", normData[0])
	}

	output := base.store.GetVariable("/output/dense/weights")
	value, err = output.Value()
	if err != nil {
		t.Fatal(err)
	}
	var outputData []float32
	if err := value.ConstFlatData(func(flat any) { outputData = slices.Clone(flat.([]float32)) }); err != nil {
		t.Fatal(err)
	}
	if outputData[0] != 0 || outputData[1] != float32(config.EmbeddingDimension) || outputData[config.VocabSize] != 1 {
		t.Fatalf("tied output was not transposed from embeddings")
	}

	base.Close()
	base.Close()
}

func TestLoadBaseModelRejectsWrongArchitecture(t *testing.T) {
	path := filepath.Join(t.TempDir(), "qwen.gguf")
	writeTestModel(t, path, testConfig(), "qwen3")

	_, err := LoadBaseModel(path)
	if err == nil || !strings.Contains(err.Error(), `general.architecture`) {
		t.Fatalf("error = %v", err)
	}
}

func testConfig() Config {
	return Config{
		VocabSize:            32,
		MaxSequenceLength:    16,
		EmbeddingDimension:   8,
		LayerCount:           1,
		AttentionHeads:       2,
		KeyValueHeads:        1,
		HeadDimension:        4,
		FeedForwardDimension: 16,
		SlidingWindow:        8,
		SlidingWindowPattern: 2,
		GlobalRoPEBase:       1_000_000,
		LocalRoPEBase:        10_000,
		TiedOutput:           true,
	}
}

func writeTestModel(t *testing.T, path string, config Config, architecture string) {
	t.Helper()

	var writer gguf.Writer
	writer.String("general.architecture", architecture)
	writer.String("general.type", "model")
	writer.Uint32("general.file_type", 0)
	writer.Uint32("general.alignment", gguf.Alignment)
	writer.Uint32("gemma3.context_length", uint32(config.MaxSequenceLength))
	writer.Uint32("gemma3.embedding_length", uint32(config.EmbeddingDimension))
	writer.Uint32("gemma3.block_count", uint32(config.LayerCount))
	writer.Uint32("gemma3.feed_forward_length", uint32(config.FeedForwardDimension))
	writer.Uint32("gemma3.attention.head_count", uint32(config.AttentionHeads))
	writer.Uint32("gemma3.attention.head_count_kv", uint32(config.KeyValueHeads))
	writer.Uint32("gemma3.attention.key_length", uint32(config.HeadDimension))
	writer.Uint32("gemma3.attention.value_length", uint32(config.HeadDimension))
	writer.Uint32("gemma3.attention.sliding_window", uint32(config.SlidingWindow))
	writer.Uint32("gemma3.attention.sliding_window_pattern", uint32(config.SlidingWindowPattern))
	writer.Float32("gemma3.attention.layer_norm_rms_epsilon", 1e-6)
	writer.Float32("gemma3.rope.freq_base", float32(config.GlobalRoPEBase))
	writer.Float32("gemma3.rope.freq_base_swa", float32(config.LocalRoPEBase))
	tokens := make([]string, config.VocabSize)
	writer.Strings("tokenizer.ggml.tokens", tokens)

	for _, spec := range tensorSpecs(config) {
		data := make([]float32, product(spec.shape))
		switch {
		case spec.ggufName == "token_embd.weight":
			for index := range data {
				data[index] = float32(index)
			}
		case spec.norm:
			for index := range data {
				data[index] = 1
			}
		case spec.dense:
			for index := range data {
				data[index] = float32(index%7+1) * 0.001
			}
		}
		writer.AddTensor(gguf.Tensor{Name: spec.ggufName, Dimensions: ggmlDimensions(spec), Data: data})
	}
	if err := writer.WriteFile(path); err != nil {
		t.Fatal(err)
	}
}
