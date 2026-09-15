// Package gemma loads Gemma 3 base models for the GoMLX training backend.
package gemma

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/gomlx/internal/gguf"
	"github.com/gomlx/gomlx/core/tensors"
	"github.com/gomlx/gomlx/ml/model"
)

// Config contains the Gemma architecture values read from a base GGUF.
type Config struct {
	VocabSize            int
	MaxSequenceLength    int
	EmbeddingDimension   int
	LayerCount           int
	AttentionHeads       int
	KeyValueHeads        int
	HeadDimension        int
	FeedForwardDimension int
	SlidingWindow        int
	SlidingWindowPattern int
	GlobalRoPEBase       float64
	LocalRoPEBase        float64
	TiedOutput           bool
}

// BaseModel owns the frozen Gemma weights loaded for training.
type BaseModel struct {
	path   string
	config Config
	store  *model.Store
}

// LoadBaseModel validates and loads an F32 or Q8_0 Gemma 3 GGUF. The source
// file is not modified and no expanded checkpoint is written to disk.
func LoadBaseModel(path string) (*BaseModel, error) {
	if path == "" {
		return nil, fmt.Errorf("gemma base GGUF path is required")
	}

	reader, err := gguf.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	config, err := readConfig(reader)
	if err != nil {
		return nil, fmt.Errorf("validate Gemma base GGUF: %w", err)
	}
	specs, err := validateTensors(reader, config)
	if err != nil {
		return nil, fmt.Errorf("validate Gemma base GGUF: %w", err)
	}

	store := model.NewStore()
	succeeded := false
	defer func() {
		if !succeeded {
			store.Finalize()
		}
	}()

	var embedding []float32
	for _, spec := range specs {
		data, err := reader.ReadTensorF32(spec.ggufName)
		if err != nil {
			return nil, fmt.Errorf("load Gemma tensor %q: %w", spec.ggufName, err)
		}
		if spec.dense {
			outputSize := product(spec.shape[1:])
			data = transpose(data, outputSize, spec.shape[0])
		}
		if spec.norm {
			for index := range data {
				data[index]--
			}
		}
		if spec.ggufName == "token_embd.weight" {
			embedding = slices.Clone(data)
		}
		variable := store.VariableWithValue(
			spec.variablePath,
			tensors.FromFlatDataAndDimensions(data, spec.shape...),
		)
		variable.SetTrainable(false)
	}

	if config.TiedOutput {
		output := transpose(embedding, config.VocabSize, config.EmbeddingDimension)
		variable := store.VariableWithValue(
			"/output/dense/weights",
			tensors.FromFlatDataAndDimensions(output, config.EmbeddingDimension, config.VocabSize),
		)
		variable.SetTrainable(false)
	}

	succeeded = true
	return &BaseModel{path: path, config: config, store: store}, nil
}

// Config returns the architecture discovered in the GGUF.
func (base *BaseModel) Config() Config {
	if base == nil {
		return Config{}
	}
	return base.config
}

// Close releases the loaded model weights.
func (base *BaseModel) Close() {
	if base == nil || base.store == nil {
		return
	}
	base.store.Finalize()
	base.store = nil
}

func readConfig(reader *gguf.Reader) (Config, error) {
	if reader.Version() != gguf.Version {
		return Config{}, fmt.Errorf("unsupported GGUF version %d, want %d", reader.Version(), gguf.Version)
	}
	if err := requireString(reader, "general.architecture", "gemma3"); err != nil {
		return Config{}, err
	}
	if err := requireString(reader, "general.type", "model"); err != nil {
		return Config{}, err
	}
	if reader.HasMetadata("general.alignment") {
		alignment, err := reader.Uint32("general.alignment")
		if err != nil {
			return Config{}, err
		}
		if alignment != gguf.Alignment {
			return Config{}, fmt.Errorf("metadata %q is %d, want %d", "general.alignment", alignment, gguf.Alignment)
		}
	}
	fileType, err := reader.Uint32("general.file_type")
	if err != nil {
		return Config{}, err
	}
	if fileType != 0 && fileType != 7 {
		return Config{}, fmt.Errorf("metadata %q is %d, want 0 (F32) or 7 (Q8_0)", "general.file_type", fileType)
	}

	config := Config{}
	for _, field := range []struct {
		key         string
		destination *int
	}{
		{"gemma3.context_length", &config.MaxSequenceLength},
		{"gemma3.embedding_length", &config.EmbeddingDimension},
		{"gemma3.block_count", &config.LayerCount},
		{"gemma3.feed_forward_length", &config.FeedForwardDimension},
		{"gemma3.attention.head_count", &config.AttentionHeads},
		{"gemma3.attention.head_count_kv", &config.KeyValueHeads},
		{"gemma3.attention.key_length", &config.HeadDimension},
		{"gemma3.attention.sliding_window", &config.SlidingWindow},
	} {
		value, err := reader.Uint32(field.key)
		if err != nil {
			return Config{}, err
		}
		if value == 0 {
			return Config{}, fmt.Errorf("metadata %q must be greater than zero", field.key)
		}
		*field.destination = int(value)
	}
	valueDimension, err := reader.Uint32("gemma3.attention.value_length")
	if err != nil {
		return Config{}, err
	}
	if int(valueDimension) != config.HeadDimension {
		return Config{}, fmt.Errorf("metadata %q is %d, want key length %d", "gemma3.attention.value_length", valueDimension, config.HeadDimension)
	}
	if config.KeyValueHeads > config.AttentionHeads || config.AttentionHeads%config.KeyValueHeads != 0 {
		return Config{}, fmt.Errorf("attention head count %d is incompatible with %d key/value heads", config.AttentionHeads, config.KeyValueHeads)
	}
	if config.SlidingWindow > config.MaxSequenceLength {
		return Config{}, fmt.Errorf("sliding window %d exceeds context length %d", config.SlidingWindow, config.MaxSequenceLength)
	}

	config.SlidingWindowPattern = 6
	if reader.HasMetadata("gemma3.attention.sliding_window_pattern") {
		pattern, err := reader.Uint32("gemma3.attention.sliding_window_pattern")
		if err != nil {
			return Config{}, err
		}
		if pattern == 0 {
			return Config{}, fmt.Errorf("metadata %q must be greater than zero", "gemma3.attention.sliding_window_pattern")
		}
		config.SlidingWindowPattern = int(pattern)
	}

	config.GlobalRoPEBase = 1_000_000
	if reader.HasMetadata("gemma3.rope.freq_base") {
		value, err := positiveFloat(reader, "gemma3.rope.freq_base")
		if err != nil {
			return Config{}, err
		}
		config.GlobalRoPEBase = float64(value)
	}
	config.LocalRoPEBase = 10_000
	if reader.HasMetadata("gemma3.rope.freq_base_swa") {
		value, err := positiveFloat(reader, "gemma3.rope.freq_base_swa")
		if err != nil {
			return Config{}, err
		}
		config.LocalRoPEBase = float64(value)
	}

	epsilon, err := reader.Float32("gemma3.attention.layer_norm_rms_epsilon")
	if err != nil {
		return Config{}, err
	}
	if math.Float32bits(epsilon) != math.Float32bits(1e-6) {
		return Config{}, fmt.Errorf("metadata %q is %g, want 1e-6", "gemma3.attention.layer_norm_rms_epsilon", epsilon)
	}

	for _, tensor := range reader.Tensors() {
		if tensor.Name != "token_embd.weight" {
			continue
		}
		if len(tensor.Dimensions) != 2 || tensor.Dimensions[0] != uint64(config.EmbeddingDimension) {
			return Config{}, fmt.Errorf("tensor %q dimensions %v do not match embedding dimension %d", tensor.Name, tensor.Dimensions, config.EmbeddingDimension)
		}
		if tensor.Dimensions[1] > math.MaxInt {
			return Config{}, fmt.Errorf("tensor %q vocabulary dimension is too large", tensor.Name)
		}
		config.VocabSize = int(tensor.Dimensions[1])
		break
	}
	if config.VocabSize <= 3 {
		return Config{}, fmt.Errorf("token embedding must establish a vocabulary larger than three tokens")
	}
	if reader.HasMetadata("tokenizer.ggml.tokens") {
		count, err := reader.ArrayLength("tokenizer.ggml.tokens")
		if err != nil {
			return Config{}, err
		}
		if count != uint64(config.VocabSize) {
			return Config{}, fmt.Errorf("tokenizer has %d tokens, want %d", count, config.VocabSize)
		}
	}

	config.TiedOutput = true
	for _, tensor := range reader.Tensors() {
		if tensor.Name == "output.weight" {
			config.TiedOutput = false
			break
		}
	}
	if err := validateDimensionProducts(config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func requireString(reader *gguf.Reader, key, expected string) error {
	value, err := reader.String(key)
	if err != nil {
		return err
	}
	if value != expected {
		return fmt.Errorf("metadata %q is %q, want %q", key, value, expected)
	}
	return nil
}

func positiveFloat(reader *gguf.Reader, key string) (float32, error) {
	value, err := reader.Float32(key)
	if err != nil {
		return 0, err
	}
	if value <= 0 || math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
		return 0, fmt.Errorf("metadata %q must be finite and greater than zero", key)
	}
	return value, nil
}

func validateDimensionProducts(config Config) error {
	checks := []struct {
		name       string
		dimensions []int
	}{
		{"token embedding", []int{config.VocabSize, config.EmbeddingDimension}},
		{"query projection", []int{config.EmbeddingDimension, config.AttentionHeads, config.HeadDimension}},
		{"key/value projection", []int{config.EmbeddingDimension, config.KeyValueHeads, config.HeadDimension}},
		{"attention output", []int{config.AttentionHeads, config.HeadDimension, config.EmbeddingDimension}},
		{"feed-forward projection", []int{config.EmbeddingDimension, config.FeedForwardDimension}},
	}
	for _, check := range checks {
		count := 1
		for _, dimension := range check.dimensions {
			if count > math.MaxInt/dimension {
				return fmt.Errorf("%s dimensions %v are too large", check.name, check.dimensions)
			}
			count *= dimension
		}
	}
	return nil
}

type tensorSpec struct {
	variablePath string
	ggufName     string
	shape        []int
	dense        bool
	norm         bool
}

func validateTensors(reader *gguf.Reader, config Config) ([]tensorSpec, error) {
	specs := tensorSpecs(config)
	expected := make(map[string]tensorSpec, len(specs))
	for _, spec := range specs {
		expected[spec.ggufName] = spec
	}

	tensors := reader.Tensors()
	if len(tensors) != len(specs) {
		return nil, fmt.Errorf("base model contains %d tensors, want exactly %d", len(tensors), len(specs))
	}
	for _, tensor := range tensors {
		spec, exists := expected[tensor.Name]
		if !exists {
			return nil, fmt.Errorf("base model contains unexpected tensor %q", tensor.Name)
		}
		if tensor.Type != 0 && tensor.Type != 8 {
			return nil, fmt.Errorf("tensor %q uses unsupported GGML type %d", tensor.Name, tensor.Type)
		}
		want := ggmlDimensions(spec)
		if !slices.Equal(tensor.Dimensions, want) {
			return nil, fmt.Errorf("tensor %q has dimensions %v, want %v", tensor.Name, tensor.Dimensions, want)
		}
		delete(expected, tensor.Name)
	}
	if len(expected) != 0 {
		for name := range expected {
			return nil, fmt.Errorf("base model is missing tensor %q", name)
		}
	}
	return specs, nil
}

func tensorSpecs(config Config) []tensorSpec {
	specs := []tensorSpec{
		{"/token_embed/embeddings", "token_embd.weight", []int{config.VocabSize, config.EmbeddingDimension}, false, false},
		{"/final_norm/rms_norm/scale", "output_norm.weight", []int{config.EmbeddingDimension}, false, true},
		{"/output/dense/weights", "output.weight", []int{config.EmbeddingDimension, config.VocabSize}, true, false},
	}
	for layer := range config.LayerCount {
		prefix := fmt.Sprintf("/layer_%d", layer)
		block := fmt.Sprintf("blk.%d", layer)
		specs = append(specs,
			tensorSpec{prefix + "/self_attn/MultiHeadAttention/query/dense/weights", block + ".attn_q.weight", []int{config.EmbeddingDimension, config.AttentionHeads * config.HeadDimension}, true, false},
			tensorSpec{prefix + "/self_attn/MultiHeadAttention/query/rms_norm/scale", block + ".attn_q_norm.weight", []int{config.HeadDimension}, false, true},
			tensorSpec{prefix + "/self_attn/MultiHeadAttention/key/dense/weights", block + ".attn_k.weight", []int{config.EmbeddingDimension, config.KeyValueHeads * config.HeadDimension}, true, false},
			tensorSpec{prefix + "/self_attn/MultiHeadAttention/key/rms_norm/scale", block + ".attn_k_norm.weight", []int{config.HeadDimension}, false, true},
			tensorSpec{prefix + "/self_attn/MultiHeadAttention/value/dense/weights", block + ".attn_v.weight", []int{config.EmbeddingDimension, config.KeyValueHeads * config.HeadDimension}, true, false},
			tensorSpec{prefix + "/self_attn/MultiHeadAttention/output/dense/weights", block + ".attn_output.weight", []int{config.AttentionHeads * config.HeadDimension, config.EmbeddingDimension}, true, false},
			tensorSpec{prefix + "/input_norm/rms_norm/scale", block + ".attn_norm.weight", []int{config.EmbeddingDimension}, false, true},
			tensorSpec{prefix + "/post_attention_norm/rms_norm/scale", block + ".post_attention_norm.weight", []int{config.EmbeddingDimension}, false, true},
			tensorSpec{prefix + "/pre_feedforward_norm/rms_norm/scale", block + ".ffn_norm.weight", []int{config.EmbeddingDimension}, false, true},
			tensorSpec{prefix + "/post_feedforward_norm/rms_norm/scale", block + ".post_ffw_norm.weight", []int{config.EmbeddingDimension}, false, true},
			tensorSpec{prefix + "/mlp/gate_proj/dense/weights", block + ".ffn_gate.weight", []int{config.EmbeddingDimension, config.FeedForwardDimension}, true, false},
			tensorSpec{prefix + "/mlp/up_proj/dense/weights", block + ".ffn_up.weight", []int{config.EmbeddingDimension, config.FeedForwardDimension}, true, false},
			tensorSpec{prefix + "/mlp/down_proj/dense/weights", block + ".ffn_down.weight", []int{config.FeedForwardDimension, config.EmbeddingDimension}, true, false},
		)
	}
	if config.TiedOutput {
		specs = slices.DeleteFunc(specs, func(spec tensorSpec) bool { return spec.ggufName == "output.weight" })
	}
	slices.SortFunc(specs, func(left, right tensorSpec) int { return strings.Compare(left.ggufName, right.ggufName) })
	return specs
}

func ggmlDimensions(spec tensorSpec) []uint64 {
	if spec.dense {
		return []uint64{uint64(spec.shape[0]), uint64(product(spec.shape[1:]))}
	}
	dimensions := make([]uint64, len(spec.shape))
	for index, dimension := range spec.shape {
		dimensions[len(dimensions)-1-index] = uint64(dimension)
	}
	return dimensions
}

func product(dimensions []int) int {
	result := 1
	for _, dimension := range dimensions {
		result *= dimension
	}
	return result
}

func transpose(data []float32, rows, columns int) []float32 {
	result := make([]float32, len(data))
	for row := range rows {
		for column := range columns {
			result[column*rows+row] = data[row*columns+column]
		}
	}
	return result
}
