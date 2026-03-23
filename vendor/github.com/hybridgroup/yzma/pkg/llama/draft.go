package llama

import (
	"unsafe"

	"github.com/jupiterrider/ffi"
)

// DraftCandidate holds a token and its probability from the sampler's
// candidate list, used for sparse speculative decoding verification.
type DraftCandidate struct {
	Tok  Token
	Prob float32
}

// DraftResult holds the output of a DraftGenerate call.
type DraftResult struct {
	Tokens    []Token            // Generated draft tokens (up to nDraft)
	Dists     [][]DraftCandidate // Sparse distributions per token (non-greedy only)
	NDrafted  int                // Number of tokens successfully drafted
	FinalPast Pos                // Draft model nPast after generation
}

// DraftGenerate performs the entire auto-regressive draft token generation loop
// in a single function call. This consolidates all FFI calls (Decode, Sample,
// GetCandidates) into a tight loop, eliminating per-iteration Go overhead from
// the caller (condition checks, lazy initialization, buffer management, logging).
//
// Parameters:
//   - ctx: draft model context
//   - batch: pre-allocated single-token batch for draft model
//   - vocab: vocabulary for EOG checking
//   - sampler: sampler to use (greedy or sampling chain)
//   - lastToken: the last accepted token (starting point)
//   - nPast: current draft model KV cache position
//   - seqIDs: sequence IDs for the batch
//   - nDraft: maximum number of tokens to draft
//   - greedy: if true, skip candidate probability capture
//   - outTokens: pre-allocated output buffer for draft tokens (len >= nDraft)
//   - outDists: pre-allocated output buffer for sparse distributions (len >= nDraft, nil for greedy)
//
// Returns the number of tokens drafted and the updated nPast position.
func DraftGenerate(
	ctx Context,
	batch *Batch,
	vocab Vocab,
	sampler Sampler,
	lastToken Token,
	nPast Pos,
	seqIDs []SeqId,
	nDraft int,
	greedy bool,
	outTokens []Token,
	outDists [][]DraftCandidate,
) (drafted int, finalPast Pos) {
	if ctx == 0 || sampler == 0 || nDraft <= 0 {
		return 0, nPast
	}

	var (
		decodeResult ffi.Arg
		sampleResult ffi.Arg
		sampleIdx    int32 = -1
		candidateIdx int32 = 0
	)

	for range nDraft {
		// Clear and add single token to batch.
		batch.NTokens = 0
		batch.Add(lastToken, nPast, seqIDs, true)

		// Decode single token.
		decodeFunc.Call(unsafe.Pointer(&decodeResult), unsafe.Pointer(&ctx), unsafe.Pointer(batch))
		if int32(decodeResult) != 0 {
			break
		}
		nPast++

		// Sample from draft model. idx=-1 means "last token in batch".
		samplerSampleFunc.Call(unsafe.Pointer(&sampleResult), unsafe.Pointer(&sampler), unsafe.Pointer(&ctx), &sampleIdx)
		token := Token(sampleResult)

		// Capture sparse candidate distributions for non-greedy mode.
		// Note: GetSampledCandidates*Ith requires a non-negative batch
		// index (0 here since the batch has exactly one token).
		if !greedy && outDists != nil {
			outDists[drafted] = outDists[drafted][:0]

			var cntResult ffi.Arg
			getSampledCandidatesCountIthFunc.Call(unsafe.Pointer(&cntResult), unsafe.Pointer(&ctx), &candidateIdx)
			cnt := int(uint32(cntResult))

			if cnt > 0 {
				var candsPtr *Token
				getSampledCandidatesIthFunc.Call(unsafe.Pointer(&candsPtr), unsafe.Pointer(&ctx), &candidateIdx)

				var probsPtr *float32
				getSampledProbsIthFunc.Call(unsafe.Pointer(&probsPtr), unsafe.Pointer(&ctx), &candidateIdx)

				if candsPtr != nil && probsPtr != nil {
					cands := unsafe.Slice(candsPtr, cnt)
					probs := unsafe.Slice(probsPtr, cnt)

					// Copy from C-backed memory into Go buffer.
					for j := range cnt {
						outDists[drafted] = append(outDists[drafted], DraftCandidate{
							Tok:  cands[j],
							Prob: probs[j],
						})
					}
				}
			}

			// Accept token in sampler for non-greedy.
			samplerAcceptFunc.Call(nil, unsafe.Pointer(&sampler), unsafe.Pointer(&token))
		}

		// Check for end of generation.
		var eogResult ffi.Arg
		vocabIsEOGFunc.Call(unsafe.Pointer(&eogResult), unsafe.Pointer(&vocab), unsafe.Pointer(&token))
		if eogResult.Bool() {
			break
		}

		outTokens[drafted] = token
		drafted++
		lastToken = token
	}

	return drafted, nPast
}
