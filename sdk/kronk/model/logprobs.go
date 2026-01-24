package model

import (
	"container/heap"
	"math"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// tokenLogprob holds a token and its log probability for sorting.
type tokenLogprob struct {
	token   llama.Token
	logprob float32
}

// minHeap implements a min-heap for tokenLogprob (smallest logprob at top).
// We use a min-heap to efficiently track the top-k largest values.
type minHeap []tokenLogprob

func (h minHeap) Len() int           { return len(h) }
func (h minHeap) Less(i, j int) bool { return h[i].logprob < h[j].logprob }
func (h minHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *minHeap) Push(x any) {
	*h = append(*h, x.(tokenLogprob))
}

func (h *minHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// extractLogprobs retrieves logits from the context and converts them to log probabilities.
// It returns the log probability for the sampled token and the top-k alternatives.
// The iBatch parameter is the batch index to extract logits from (-1 for the last position).
func extractLogprobs(lctx llama.Context, vocab llama.Vocab, sampledToken llama.Token, iBatch int32, topK int, buf []byte) (*ContentLogprob, error) {
	nVocab := int(llama.VocabNTokens(vocab))

	// Get logits for the specified batch position.
	logits, err := llama.GetLogitsIth(lctx, iBatch, nVocab)
	if err != nil {
		return nil, err
	}

	// Convert logits to log probabilities using log-softmax.
	logprobs := logSoftmax(logits)

	// Get the sampled token's text and logprob.
	l := llama.TokenToPiece(vocab, sampledToken, buf, 0, true)
	piece := string(buf[:l])
	sampledLogprob := logprobs[sampledToken]

	result := &ContentLogprob{
		Token:   piece,
		Logprob: sampledLogprob,
		Bytes:   []byte(piece),
	}

	// If topK requested, find the top-k tokens.
	if topK > 0 {
		result.TopLogprobs = getTopKLogprobs(vocab, logprobs, topK, buf)
	}

	return result, nil
}

// logSoftmax converts raw logits to log probabilities.
// log_softmax(x_i) = x_i - log(sum(exp(x_j)))
// Uses the log-sum-exp trick for numerical stability.
func logSoftmax(logits []float32) []float32 {
	if len(logits) == 0 {
		return nil
	}

	// Find max for numerical stability.
	maxLogit := logits[0]
	for _, l := range logits[1:] {
		if l > maxLogit {
			maxLogit = l
		}
	}

	// Compute sum of exp(logit - max).
	var sumExp float64
	for _, l := range logits {
		sumExp += math.Exp(float64(l - maxLogit))
	}
	logSumExp := maxLogit + float32(math.Log(sumExp))

	// Compute log probabilities.
	result := make([]float32, len(logits))
	for i, l := range logits {
		result[i] = l - logSumExp
	}

	return result
}

// getTopKLogprobs returns the top-k tokens by log probability.
// Uses a min-heap to efficiently find top-k without sorting the entire vocab.
func getTopKLogprobs(vocab llama.Vocab, logprobs []float32, k int, buf []byte) []TopLogprob {
	if k <= 0 || len(logprobs) == 0 {
		return nil
	}

	if k > len(logprobs) {
		k = len(logprobs)
	}

	// Use a min-heap of size k to track the k largest logprobs.
	// When we see a value larger than the heap minimum, replace it.
	h := make(minHeap, 0, k)
	heap.Init(&h)

	for i, lp := range logprobs {
		if h.Len() < k {
			heap.Push(&h, tokenLogprob{token: llama.Token(i), logprob: lp})
			continue
		}

		if lp > h[0].logprob {
			heap.Pop(&h)
			heap.Push(&h, tokenLogprob{token: llama.Token(i), logprob: lp})
		}
	}

	// Extract results in descending order (pop from min-heap gives ascending,
	// so we fill the result array from the end).
	result := make([]TopLogprob, h.Len())
	for i := len(result) - 1; i >= 0; i-- {
		item := heap.Pop(&h).(tokenLogprob)

		l := llama.TokenToPiece(vocab, item.token, buf, 0, true)
		piece := string(buf[:l])

		result[i] = TopLogprob{
			Token:   piece,
			Logprob: item.logprob,
			Bytes:   []byte(piece),
		}
	}

	return result
}
