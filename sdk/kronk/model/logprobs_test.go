package model

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestLogSoftmax(t *testing.T) {
	tests := []struct {
		name   string
		logits []float32
	}{
		{
			name:   "simple case",
			logits: []float32{1.0, 2.0, 3.0},
		},
		{
			name:   "negative values",
			logits: []float32{-1.0, 0.0, 1.0},
		},
		{
			name:   "large values",
			logits: []float32{100.0, 101.0, 102.0},
		},
		{
			name:   "single element",
			logits: []float32{5.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := logSoftmax(tt.logits)

			if len(result) != len(tt.logits) {
				t.Errorf("logSoftmax() returned %d elements, want %d", len(result), len(tt.logits))
				return
			}

			// Verify that exp(log_softmax) sums to 1.0
			var sum float64
			for _, lp := range result {
				sum += math.Exp(float64(lp))
			}

			if math.Abs(sum-1.0) > 1e-5 {
				t.Errorf("exp(logSoftmax()) sum = %v, want 1.0", sum)
			}

			// Verify all log probabilities are <= 0
			for i, lp := range result {
				if lp > 0 {
					t.Errorf("logSoftmax()[%d] = %v, want <= 0", i, lp)
				}
			}

			// Verify ordering is preserved (higher logit = higher log prob)
			for i := 1; i < len(result); i++ {
				if tt.logits[i] > tt.logits[i-1] && result[i] < result[i-1] {
					t.Errorf("logSoftmax() ordering not preserved at index %d", i)
				}
			}
		})
	}
}

func TestLogSoftmaxEmpty(t *testing.T) {
	result := logSoftmax(nil)
	if result != nil {
		t.Errorf("logSoftmax(nil) = %v, want nil", result)
	}

	result = logSoftmax([]float32{})
	if result != nil {
		t.Errorf("logSoftmax([]) = %v, want nil", result)
	}
}

func TestGetTopKLogprobs(t *testing.T) {
	// Test with a simple case - we can't test the full function without
	// a real vocab, but we can verify the sorting logic indirectly
	// by checking logSoftmax ordering

	logits := []float32{1.0, 5.0, 2.0, 4.0, 3.0}
	logprobs := logSoftmax(logits)

	// Find expected order (indices sorted by logprob descending)
	// Original logits: [1.0, 5.0, 2.0, 4.0, 3.0]
	// Expected order by value: index 1 (5.0), index 3 (4.0), index 4 (3.0), index 2 (2.0), index 0 (1.0)

	// Verify the highest logprob corresponds to the highest logit
	maxIdx := 0
	for i, lp := range logprobs {
		if lp > logprobs[maxIdx] {
			maxIdx = i
		}
	}

	if maxIdx != 1 {
		t.Errorf("max logprob at index %d, want 1 (logit 5.0)", maxIdx)
	}
}

func TestSuppressTokenLogitBiases(t *testing.T) {
	tokens := []llama.Token{1, 3}
	biases := suppressTokenLogitBiases(tokens)

	if len(biases) != len(tokens) {
		t.Fatalf("bias count: got %d, want %d", len(biases), len(tokens))
	}
	for i, bias := range biases {
		if bias.Token != tokens[i] {
			t.Errorf("bias %d token: got %d, want %d", i, bias.Token, tokens[i])
		}
		if !math.IsInf(float64(bias.Bias), -1) {
			t.Errorf("bias %d value: got %v, want -Inf", i, bias.Bias)
		}
	}
}

func TestMaskSuppressTokenLogits(t *testing.T) {
	logits := []float32{100, 2, 3}
	maskSuppressTokenLogits(logits, []llama.Token{0})

	if !math.IsInf(float64(logits[0]), -1) {
		t.Errorf("suppressed logit: got %v, want -Inf", logits[0])
	}
	if logits[2] <= logits[1] {
		t.Errorf("unsuppressed maximum: got logits %v, want index 2", logits)
	}

	logprobs := logSoftmax(logits)
	if !math.IsInf(float64(logprobs[0]), -1) {
		t.Errorf("suppressed logprob: got %v, want -Inf", logprobs[0])
	}
}

func TestApplySamplerFiltersSuppressesBeforeTopK(t *testing.T) {
	logits := []float32{100, 2, 3}
	probs := make([]float32, len(logits))
	var fs filterState

	applySamplerFilters(logits, probs, []llama.Token{0}, 1, 1, 0, 1, nil, &fs)

	if probs[0] != 0 {
		t.Errorf("suppressed probability: got %v, want 0", probs[0])
	}
	if probs[2] != 1 {
		t.Errorf("highest allowed probability: got %v, want 1", probs[2])
	}
}

func TestApplySamplerFiltersTopPZeroKeepsHighestToken(t *testing.T) {
	logits := []float32{-2, 4, 1, 3}
	probs := make([]float32, len(logits))
	var fs filterState

	indices := applySamplerFilters(logits, probs, nil, 1, 0, 0, 0, nil, &fs)

	if !slices.Equal(indices, []int{1}) {
		t.Errorf("survivor indices: got %v, want [1]", indices)
	}
	if probs[1] != 1 {
		t.Errorf("highest probability: got %v, want 1", probs[1])
	}
}

func TestApplySamplerFiltersDisabledTopK(t *testing.T) {
	tests := []struct {
		name        string
		topP        float32
		minP        float32
		wantIndices []int
	}{
		{name: "no truncation", topP: 1, wantIndices: []int{0, 1, 2, 3}},
		{name: "nucleus", topP: 0.8, wantIndices: []int{3, 2}},
		{name: "minimum probability", topP: 1, minP: 0.2, wantIndices: []int{2, 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logits := []float32{0, 1, 2, 3}
			probs := make([]float32, len(logits))
			var fs filterState

			indices := applySamplerFilters(logits, probs, nil, 1, tt.topP, tt.minP, -1, nil, &fs)

			if !slices.Equal(indices, tt.wantIndices) {
				t.Errorf("survivor indices: got %v, want %v", indices, tt.wantIndices)
			}
			var sum float32
			for _, probability := range probs {
				sum += probability
			}
			if math.Abs(float64(sum-1)) > 1e-6 {
				t.Errorf("probability sum: got %v, want 1", sum)
			}
		})
	}
}

func TestApplySamplerFiltersClearsPreviousSurvivors(t *testing.T) {
	logits := []float32{0, 1, 2, 3}
	probs := make([]float32, len(logits))
	var fs filterState

	indices := applySamplerFilters(logits, probs, nil, 1, 1, 0, 0, nil, &fs)
	indices = applySamplerFilters(logits, probs, nil, 1, 1, 0, 1, indices, &fs)

	if !slices.Equal(indices, []int{3}) {
		t.Errorf("survivor indices: got %v, want [3]", indices)
	}
	if !slices.Equal(probs, []float32{0, 0, 0, 1}) {
		t.Errorf("probabilities: got %v, want [0 0 0 1]", probs)
	}
}

func TestApplySamplerFiltersTopPExpandsPastAdaptivePrefix(t *testing.T) {
	const size = 300
	logits := make([]float32, size)
	for i := range logits {
		logits[i] = float32(i) / size
	}
	probs := make([]float32, size)
	var fs filterState

	indices := applySamplerFilters(logits, probs, nil, 1, 0.99, 0, 0, nil, &fs)

	if len(indices) <= 256 {
		t.Fatalf("survivor count: got %d, want more than adaptive prefix 256", len(indices))
	}
	for i, idx := range indices {
		want := size - 1 - i
		if idx != want {
			t.Fatalf("survivor %d: got token %d, want %d", i, idx, want)
		}
	}
	var sum float32
	for _, probability := range probs {
		sum += probability
	}
	if math.Abs(float64(sum-1)) > 1e-6 {
		t.Errorf("probability sum: got %v, want 1", sum)
	}
}

func BenchmarkApplySamplerFilters(b *testing.B) {
	const qwenVocab = 151936
	logits := make([]float32, qwenVocab)
	for i := range logits {
		rank := (i * 7919) % qwenVocab
		logits[i] = -float32(rank) / 50
	}

	tests := []struct {
		topK int32
		topP float32
		minP float32
	}{
		{topK: 20, topP: 0.95},
		{topK: 0, topP: 0},
		{topK: 0, topP: 0.95},
		{topK: 0, topP: 1},
		{topK: 0, topP: 1, minP: 0.05},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("top_k=%d/top_p=%.2f/min_p=%.2f", tt.topK, tt.topP, tt.minP)
		b.Run(name, func(b *testing.B) {
			probs := make([]float32, len(logits))
			var indices []int
			var fs filterState
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				indices = applySamplerFilters(logits, probs, nil, 0.9, tt.topP, tt.minP, tt.topK, indices, &fs)
			}
		})
	}
}
