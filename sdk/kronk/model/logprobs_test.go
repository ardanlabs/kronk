package model

import (
	"math"
	"testing"
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
