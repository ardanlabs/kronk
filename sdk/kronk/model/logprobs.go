package model

import (
	"cmp"
	"container/heap"
	"math"
	"slices"

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
func extractLogprobs(lctx llama.Context, vocab llama.Vocab, suppressTokens []llama.Token, sampledToken llama.Token, iBatch int32, topK int, buf []byte) (*ContentLogprob, error) {
	nVocab := int(llama.VocabNTokens(vocab))

	// Get logits for the specified batch position.
	logits, err := llama.GetLogitsIth(lctx, iBatch, nVocab)
	if err != nil {
		return nil, err
	}
	maskSuppressTokenLogits(logits, suppressTokens)

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

// filterHeapEntry holds an index and logit value for min-heap selection.
type filterHeapEntry struct {
	idx int
	val float32
}

// filterState holds pre-allocated buffers for applySamplerFilters to avoid
// per-call allocations. Stored on draftCore and reused across calls.
type filterState struct {
	heap []filterHeapEntry
}

// applySamplerFilters zeroes out tokens that would be removed by the sampler
// chain (top-k → top-p → min-p) and renormalizes, so the resulting distribution
// matches what the draft sampler produces. This makes p_target comparable to
// q_draft during speculative sampling verification.
//
// The sampler chain applies top-k → top-p → min-p BEFORE temperature, so this
// function uses raw logits for filter decisions, then computes temperature-scaled
// probabilities only for the surviving tokens.
//
// Performance: finds an enabled top-K via min-heap and uses an adaptive
// candidate prefix for top-P when top-K is disabled. If truncation is disabled,
// it computes the distribution without ordering the vocabulary.
func applySamplerFilters(logits, probs []float32, suppressTokens []llama.Token, temperature, topP, minP float32, topK int32, indices []int, fs *filterState) []int {
	maskSuppressTokenLogits(logits, suppressTokens)
	n := len(logits)
	if n == 0 {
		return indices[:0]
	}

	// The returned indices are the non-zero entries from the previous call.
	// Clear only those entries before reusing the buffer for this row.
	for _, idx := range indices {
		probs[idx] = 0
	}
	indices = indices[:0]

	topPEnabled := topP < 1
	sorted := false

	switch {
	case topK > 0:
		indices = selectTopLogits(logits, min(int(topK), n), indices, fs)
		sorted = true

	case topPEnabled:
		// Match llama.cpp's adaptive top-P strategy: first inspect a small
		// ordered prefix, then expand to the full vocabulary only when the
		// requested cumulative mass requires it.
		candidateCount := min(256, n)
		if topP <= 0 {
			candidateCount = 1
		}
		indices = selectTopLogits(logits, candidateCount, indices, fs)
		sorted = true

	default:
		indices = linearIndices(n, indices)
	}

	maxIdx := indices[0]
	maxLogit := logits[maxIdx]
	if !sorted {
		for _, idx := range indices[1:] {
			if logits[idx] > maxLogit {
				maxIdx = idx
				maxLogit = logits[idx]
			}
		}
	}

	if topPEnabled && topP <= 0 {
		indices = indices[:1]
	} else if topPEnabled {
		var rawSum float64
		if topK > 0 {
			for _, idx := range indices {
				rawSum += math.Exp(float64(logits[idx] - maxLogit))
			}
		} else {
			for _, logit := range logits {
				rawSum += math.Exp(float64(logit - maxLogit))
			}
		}

		cutoff, complete := topPCutoff(logits, indices, maxLogit, rawSum, topP)
		if !complete && topK <= 0 && len(indices) < n {
			indices = linearIndices(n, indices)
			sortLogitIndices(logits, indices)
			cutoff, _ = topPCutoff(logits, indices, maxLogit, rawSum, topP)
		}
		indices = indices[:cutoff]
	}

	// Min-P compares each pre-temperature probability with a fraction of the
	// maximum probability. The common normalization term cancels, so this is
	// linear even when the candidates are not ordered.
	if minP > 0 {
		kept := 0
		for _, idx := range indices {
			relative := math.Exp(float64(logits[idx] - maxLogit))
			if relative >= float64(minP) {
				indices[kept] = idx
				kept++
			}
		}
		if kept == 0 {
			indices[0] = maxIdx
			kept = 1
		}
		indices = indices[:kept]
	}

	// Compute temperature-scaled probabilities for survivors only.
	var invT float64 = 1.0
	if temperature > 0 && temperature != 1.0 {
		invT = 1.0 / float64(temperature)
	}

	var tempSum float64
	for _, idx := range indices {
		p := math.Exp(float64(logits[idx]-maxLogit) * invT)
		probs[idx] = float32(p)
		tempSum += p
	}

	if tempSum > 0 {
		invSum := float32(1.0 / tempSum)
		for _, idx := range indices {
			probs[idx] *= invSum
		}
	}

	return indices
}

func selectTopLogits(logits []float32, count int, indices []int, fs *filterState) []int {
	if cap(fs.heap) < count {
		fs.heap = make([]filterHeapEntry, 0, count)
	}
	h := fs.heap[:0]

	for idx, logit := range logits {
		if len(h) < count {
			h = append(h, filterHeapEntry{idx: idx, val: logit})
			for child := len(h) - 1; child > 0; {
				parent := (child - 1) / 2
				if h[parent].val <= h[child].val {
					break
				}
				h[parent], h[child] = h[child], h[parent]
				child = parent
			}
			continue
		}
		if logit > h[0].val {
			h[0] = filterHeapEntry{idx: idx, val: logit}
			for parent := 0; ; {
				left := 2*parent + 1
				if left >= len(h) {
					break
				}
				smallest := left
				if right := left + 1; right < len(h) && h[right].val < h[left].val {
					smallest = right
				}
				if h[parent].val <= h[smallest].val {
					break
				}
				h[parent], h[smallest] = h[smallest], h[parent]
				parent = smallest
			}
		}
	}
	fs.heap = h

	if cap(indices) < len(h) {
		indices = make([]int, len(h))
	} else {
		indices = indices[:len(h)]
	}
	for i, entry := range h {
		indices[i] = entry.idx
	}
	sortLogitIndices(logits, indices)

	return indices
}

func linearIndices(n int, indices []int) []int {
	if cap(indices) < n {
		indices = make([]int, n)
	} else {
		indices = indices[:n]
	}
	for i := range indices {
		indices[i] = i
	}
	return indices
}

func sortLogitIndices(logits []float32, indices []int) {
	slices.SortFunc(indices, func(a, b int) int {
		if logits[a] > logits[b] {
			return -1
		}
		if logits[a] < logits[b] {
			return 1
		}
		return cmp.Compare(a, b)
	})
}

func topPCutoff(logits []float32, indices []int, maxLogit float32, rawSum float64, topP float32) (int, bool) {
	// llama.cpp stores each softmax probability and the nucleus cumulative
	// mass as float. Keep that precision here so boundary decisions agree.
	var cumulative float32
	for i, idx := range indices {
		cumulative += float32(math.Exp(float64(logits[idx]-maxLogit)) / rawSum)
		if cumulative >= topP {
			return i + 1, true
		}
	}
	return len(indices), false
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
