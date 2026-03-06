package model

import (
	"container/heap"
	"math"
	"sort"

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

// filterHeapEntry holds an index and logit value for min-heap selection.
type filterHeapEntry struct {
	idx int
	val float32
}

// filterState holds pre-allocated buffers for applySamplerFilters to avoid
// per-call allocations. Stored on draftModel and reused across calls.
type filterState struct {
	heap     []filterHeapEntry
	rawProbs []float64
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
// Performance: finds top-K logits via min-heap in O(n log K), sorts the small
// K-element set, computes temperature-scaled softmax restricted to survivors.
// For K=20 and n=152k this is ~100x faster than a full sort.
func applySamplerFilters(logits, probs []float32, temperature, topP, minP float32, topK int32, indices []int, fs *filterState) []int {
	n := len(logits)

	// Determine the candidate set size for top-K selection.
	selectK := n
	if topK > 0 {
		selectK = int(topK)
	}

	// Reuse or grow the top-K index buffer.
	if cap(indices) < selectK {
		indices = make([]int, selectK)
	}
	indices = indices[:0]

	// Reuse or grow the heap buffer.
	if cap(fs.heap) < selectK {
		fs.heap = make([]filterHeapEntry, 0, selectK)
	}
	h := fs.heap[:0]

	// Find top-K logits using a min-heap. O(n log K).
	for i, l := range logits {
		if len(h) < selectK {
			h = append(h, filterHeapEntry{i, l})
			// Sift up.
			j := len(h) - 1
			for j > 0 {
				parent := (j - 1) / 2
				if h[parent].val <= h[j].val {
					break
				}
				h[parent], h[j] = h[j], h[parent]
				j = parent
			}
			continue
		}
		if l > h[0].val {
			h[0] = filterHeapEntry{i, l}
			j := 0
			for {
				left := 2*j + 1
				if left >= len(h) {
					break
				}
				smallest := left
				if right := left + 1; right < len(h) && h[right].val < h[left].val {
					smallest = right
				}
				if h[j].val <= h[smallest].val {
					break
				}
				h[j], h[smallest] = h[smallest], h[j]
				j = smallest
			}
		}
	}
	fs.heap = h

	// Extract indices and sort by logit descending for top-p/min-p.
	indices = indices[:len(h)]
	for i, e := range h {
		indices[i] = e.idx
	}
	sort.Slice(indices, func(a, b int) bool {
		return logits[indices[a]] > logits[indices[b]]
	})

	cutoff := len(indices)
	maxLogit := logits[indices[0]]

	// Compute raw softmax (T=1) for the selected candidates only,
	// used for top-p and min-p filter decisions.
	if cap(fs.rawProbs) < cutoff {
		fs.rawProbs = make([]float64, cutoff)
	}
	rawProbs := fs.rawProbs[:cutoff]

	var rawSum float64
	for i, idx := range indices {
		p := math.Exp(float64(logits[idx] - maxLogit))
		rawProbs[i] = p
		rawSum += p
	}
	invRawSum := 1.0 / rawSum
	for i := range rawProbs {
		rawProbs[i] *= invRawSum
	}

	maxProb := rawProbs[0]

	// Top-P (nucleus): keep smallest set whose cumulative probability >= topP.
	if topP > 0 && topP < 1.0 {
		var cumulative float64
		for i := 0; i < cutoff; i++ {
			cumulative += rawProbs[i]
			if cumulative >= float64(topP) {
				cutoff = i + 1
				break
			}
		}
	}

	// Min-P: remove tokens with probability < minP * maxProb.
	if minP > 0 && maxProb > 0 {
		threshold := float64(minP) * maxProb
		for i := 0; i < cutoff; i++ {
			if rawProbs[i] < threshold {
				cutoff = i
				break
			}
		}
		if cutoff == 0 {
			cutoff = 1
		}
	}

	// Compute temperature-scaled probabilities for survivors only.
	clear(probs)

	var invT float64 = 1.0
	if temperature > 0 && temperature != 1.0 {
		invT = 1.0 / float64(temperature)
	}

	var tempSum float64
	for i := 0; i < cutoff; i++ {
		p := math.Exp(float64(logits[indices[i]]-maxLogit) * invT)
		probs[indices[i]] = float32(p)
		tempSum += p
	}

	if tempSum > 0 {
		invSum := float32(1.0 / tempSum)
		for i := 0; i < cutoff; i++ {
			probs[indices[i]] *= invSum
		}
	}

	return indices
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
