package gemma

import (
	"math"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gomlx/gomlx/core/tensors"
	"github.com/gomlx/gomlx/ml/model"
)

func TestRuntimeForward(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	path := filepath.Join(t.TempDir(), "gemma.gguf")
	config := testConfig()
	writeTestModel(t, path, config, "gemma3")

	runtime, err := NewRuntime(path)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	variables := runtime.base.store.NumVariables()
	result, err := runtime.Run([]int32{1, 10, 11, 12})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Finite {
		t.Fatal("forward pass returned non-finite logits")
	}
	if len(result.Shape) != 3 || result.Shape[0] != 1 || result.Shape[1] != 4 || result.Shape[2] != config.VocabSize {
		t.Fatalf("logits shape = %v", result.Shape)
	}
	if !strings.Contains(strings.ToLower(runtime.Backend()), "go") {
		t.Fatalf("backend = %q, want Go backend", runtime.Backend())
	}
	if runtime.base.store.NumVariables() != variables {
		t.Fatalf("graph added variables: before=%d after=%d", variables, runtime.base.store.NumVariables())
	}
	for variable := range runtime.base.store.IterVariables() {
		if variable.Trainable {
			t.Fatalf("forward graph made base variable %q trainable", variable.Path())
		}
	}

	runtime.Close()
	runtime.Close()
	if _, err := runtime.Run([]int32{1}); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("run-after-close error = %v", err)
	}
}

func TestRuntimeRejectsInvalidTokens(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	path := filepath.Join(t.TempDir(), "gemma.gguf")
	config := testConfig()
	writeTestModel(t, path, config, "gemma3")

	runtime, err := NewRuntime(path)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	tests := []struct {
		name   string
		tokens []int32
		match  string
	}{
		{"empty", nil, "required"},
		{"negative", []int32{-1}, "outside vocabulary"},
		{"too large", []int32{int32(config.VocabSize)}, "outside vocabulary"},
		{"too long", make([]int32, config.MaxSequenceLength+1), "exceeds"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := runtime.Run(test.tokens)
			if err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("error = %v, want containing %q", err, test.match)
			}
		})
	}
}

func TestZeroInitializedLoRAMatchesBaseModel(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	path := filepath.Join(t.TempDir(), "gemma.gguf")
	config := testConfig()
	writeTestModel(t, path, config, "gemma3")
	tokens := []int32{1, 10, 11, 12}

	base, err := NewRuntime(path)
	if err != nil {
		t.Fatal(err)
	}
	var baseLogits []float32
	if _, err := base.run(tokens, func(values []float32) { baseLogits = slices.Clone(values) }); err != nil {
		base.Close()
		t.Fatal(err)
	}
	base.Close()

	lora, err := NewLoRARuntime(path, AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: 1, Seed: 3407})
	if err != nil {
		t.Fatal(err)
	}
	defer lora.Close()
	var loraLogits []float32
	if _, err := lora.run(tokens, func(values []float32) { loraLogits = slices.Clone(values) }); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(baseLogits, loraLogits) {
		t.Fatal("zero-initialized LoRA changed Gemma logits")
	}

	wantAdapters := config.LayerCount * 7 * 2
	trainable := 0
	for variable := range lora.base.store.IterVariables() {
		if variable.Trainable {
			trainable++
			if !strings.HasSuffix(variable.Path(), "/lora/a") && !strings.HasSuffix(variable.Path(), "/lora/b") {
				t.Fatalf("base variable %q is trainable", variable.Path())
			}
		}
	}
	if trainable != wantAdapters {
		t.Fatalf("trainable variables = %d, want %d LoRA variables", trainable, wantAdapters)
	}
	if got := lora.base.store.GetVariable("/layer_0/self_attn/MultiHeadAttention/query/lora/a"); got == nil || !slices.Equal(got.Shape().Dimensions, []int{2, config.EmbeddingDimension}) {
		t.Fatalf("query LoRA A has unexpected shape")
	}
	if got := lora.base.store.GetVariable("/layer_0/self_attn/MultiHeadAttention/value/lora/b"); got == nil || !slices.Equal(got.Shape().Dimensions, []int{config.KeyValueHeads * config.HeadDimension, 2}) {
		t.Fatalf("value LoRA B has unexpected shape")
	}
	if got := lora.base.store.GetVariable("/layer_0/mlp/down_proj/lora/a"); got == nil || !slices.Equal(got.Shape().Dimensions, []int{2, config.FeedForwardDimension}) {
		t.Fatalf("feed-forward down LoRA A has unexpected shape")
	}
}

func TestLoRATrainStepUpdatesOnlyAdapter(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	path := filepath.Join(t.TempDir(), "gemma.gguf")
	writeTestModel(t, path, testConfig(), "gemma3")

	runtime, err := NewLoRARuntime(path, AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: 1, Seed: 3407})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	tokens := []int32{1, 4, 5, 6}
	if _, err := runtime.Run(tokens); err != nil {
		t.Fatal(err)
	}
	baseBefore := snapshotVariables(t, runtime.base.store, func(variable *model.Variable) bool {
		return !strings.Contains(variable.Path(), "/lora/") && !strings.HasPrefix(variable.Path(), "/#")
	})
	adapterBefore := snapshotVariables(t, runtime.base.store, func(variable *model.Variable) bool {
		return strings.Contains(variable.Path(), "/lora/")
	})

	loss, err := runtime.TrainStep(
		tokens,
		[]int32{4, 5, 6, 7},
		[]bool{false, false, true, true},
		1e-3,
	)
	if err != nil {
		t.Fatal(err)
	}
	if math.IsNaN(loss) || math.IsInf(loss, 0) {
		t.Fatalf("loss = %g, want finite", loss)
	}

	baseAfter := snapshotVariables(t, runtime.base.store, func(variable *model.Variable) bool {
		_, existedBefore := baseBefore[variable.Path()]
		return existedBefore
	})
	if !equalVariableSnapshots(baseBefore, baseAfter) {
		t.Fatal("training changed one or more frozen base variables")
	}
	adapterAfter := snapshotVariables(t, runtime.base.store, func(variable *model.Variable) bool {
		_, existedBefore := adapterBefore[variable.Path()]
		return existedBefore
	})
	if equalVariableSnapshots(adapterBefore, adapterAfter) {
		t.Fatal("training did not change any LoRA variables")
	}
	for variable := range runtime.base.store.IterVariables() {
		if variable.Trainable && !strings.Contains(variable.Path(), "/lora/") {
			t.Fatalf("non-LoRA variable %q is trainable", variable.Path())
		}
	}

	result, err := runtime.Run(tokens)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Finite {
		t.Fatal("post-training forward pass returned non-finite logits")
	}
}

func TestTrainStepRejectsInvalidInput(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	path := filepath.Join(t.TempDir(), "gemma.gguf")
	writeTestModel(t, path, testConfig(), "gemma3")

	base, err := NewRuntime(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := base.TrainStep([]int32{1}, []int32{1}, []bool{true}, 1e-3); err == nil || !strings.Contains(err.Error(), "no LoRA") {
		base.Close()
		t.Fatalf("base runtime training error = %v", err)
	}
	base.Close()

	runtime, err := NewLoRARuntime(path, AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: 1, Seed: 3407})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	tests := []struct {
		name         string
		tokens       []int32
		targets      []int32
		mask         []bool
		learningRate float64
		match        string
	}{
		{"missing target", []int32{1}, nil, []bool{true}, 1e-3, "targets are required"},
		{"target length", []int32{1, 2}, []int32{2}, []bool{true, true}, 1e-3, "target count"},
		{"mask length", []int32{1}, []int32{2}, nil, 1e-3, "loss mask count"},
		{"empty mask", []int32{1}, []int32{2}, []bool{false}, 1e-3, "select at least one"},
		{"invalid learning rate", []int32{1}, []int32{2}, []bool{true}, math.NaN(), "learning rate"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := runtime.TrainStep(test.tokens, test.targets, test.mask, test.learningRate)
			if err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("error = %v, want containing %q", err, test.match)
			}
		})
	}
}

func TestTrainStepReusesPaddedSequenceGraphs(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	path := filepath.Join(t.TempDir(), "gemma.gguf")
	config := testConfig()
	config.MaxSequenceLength = 64
	writeTestModel(t, path, config, "gemma3")

	runtime, err := NewLoRARuntime(path, AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: 1, Seed: 3407})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	// Without shape bucketing, the 33rd distinct sequence length exceeds
	// GoMLX's default executable graph cache.
	for length := 1; length <= 40; length++ {
		tokens := make([]int32, length)
		targets := make([]int32, length)
		mask := make([]bool, length)
		for index := range length {
			tokens[index] = 1
			targets[index] = 2
		}
		mask[length-1] = true
		if _, err := runtime.TrainStep(tokens, targets, mask, 1e-3); err != nil {
			t.Fatalf("sequence length %d: %v", length, err)
		}
	}
}

func TestPaddedSequenceLength(t *testing.T) {
	tests := []struct {
		length  int
		maximum int
		want    int
	}{
		{1, 64, 1},
		{2, 64, 2},
		{3, 64, 4},
		{33, 64, 64},
		{60, 60, 60},
	}
	for _, test := range tests {
		if got := paddedSequenceLength(test.length, test.maximum); got != test.want {
			t.Errorf("paddedSequenceLength(%d, %d) = %d, want %d", test.length, test.maximum, got, test.want)
		}
	}
}

func snapshotVariables(t *testing.T, store *model.Store, include func(*model.Variable) bool) map[string][]float32 {
	t.Helper()
	result := make(map[string][]float32)
	for variable := range store.IterVariables() {
		if !include(variable) {
			continue
		}
		value, err := variable.Value()
		if err != nil {
			t.Fatalf("read variable %q: %v", variable.Path(), err)
		}
		if err := tensors.ConstFlatData[float32](value, func(data []float32) {
			result[variable.Path()] = slices.Clone(data)
		}); err != nil {
			t.Fatalf("copy variable %q: %v", variable.Path(), err)
		}
	}
	return result
}

func equalVariableSnapshots(left, right map[string][]float32) bool {
	if len(left) != len(right) {
		return false
	}
	for path, leftData := range left {
		rightData, exists := right[path]
		if !exists || !slices.Equal(leftData, rightData) {
			return false
		}
	}
	return true
}

func TestLoRARuntimeRejectsInvalidConfig(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	path := filepath.Join(t.TempDir(), "gemma.gguf")
	config := testConfig()
	writeTestModel(t, path, config, "gemma3")

	tests := []struct {
		name   string
		config AdapterConfig
	}{
		{"zero rank", AdapterConfig{Rank: 0, Alpha: 4, AdapterScale: 1}},
		{"oversized rank", AdapterConfig{Rank: config.EmbeddingDimension + 1, Alpha: 4, AdapterScale: 1}},
		{"invalid alpha", AdapterConfig{Rank: 2, Alpha: math.NaN(), AdapterScale: 1}},
		{"negative scale", AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: -1}},
		{"infinite scale", AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: math.Inf(1)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime, err := NewLoRARuntime(path, test.config)
			if runtime != nil {
				runtime.Close()
			}
			if err == nil {
				t.Fatal("expected invalid LoRA configuration error")
			}
		})
	}
}
