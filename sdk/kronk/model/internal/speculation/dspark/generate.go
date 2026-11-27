package dspark

import (
	"errors"
	"fmt"
	"math"

	yzmaspec "github.com/hybridgroup/yzma/exp/speculative"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// Proposal contains graph-selected tokens and their sparse sampling laws.
// Distributions are present only for positive-temperature generation.
type Proposal struct {
	Candidates    []llama.Token
	Distributions [][]llama.DraftCandidate
}

// Generate returns a copied proposal block after the anchor at position. The
// accepted prefix must end at position-1. Provisional KV is removed on success
// and failure; failures disable the sequence until Reset. The caller serializes
// native access and supplies request-scoped uniform draws when temperature > 0.
func (r *Resources) Generate(seq llama.SeqId, anchor llama.Token, position llama.Pos, count int, temperature float32, random func() float32) (proposal Proposal, err error) {
	if seq < 0 || int(seq) >= len(r.sequences) {
		return Proposal{}, fmt.Errorf("DSpark generate: invalid sequence %d", seq)
	}
	s := &r.sequences[seq]
	defer func() {
		if err != nil {
			s.disabled = true
			proposal = Proposal{}
		}
	}()
	if temperature < 0 || math.IsNaN(float64(temperature)) || math.IsInf(float64(temperature), 0) || temperature > 0 && random == nil {
		return Proposal{}, fmt.Errorf("DSpark generate: invalid temperature or missing random source")
	}
	state := r.ContextState(seq, position-1)
	if s.disabled || position <= 0 || s.next != position || len(state) != r.width || r.width <= 0 {
		return Proposal{}, fmt.Errorf("DSpark generate: unusable accepted prefix for sequence %d at %d", seq, position)
	}
	for _, value := range state {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return Proposal{}, fmt.Errorf("DSpark generate: nonfinite accepted state for sequence %d", seq)
		}
	}
	last, err := llama.MemorySeqPosMax(r.Memory, seq)
	if err != nil {
		return Proposal{}, fmt.Errorf("DSpark generate accepted prefix: %w", err)
	}
	if last != position-1 {
		return Proposal{}, fmt.Errorf("DSpark generate: native prefix ends at %d, want %d", last, position-1)
	}
	count = greedyCount(count, r.proposals, int(llama.NBatch(r.Context)), int(llama.NUBatch(r.Context)), int(llama.NCtxSeq(r.Context)), position)
	if count == 0 {
		return Proposal{}, nil
	}
	vocab := llama.ModelGetVocab(r.Model)
	nVocab := llama.VocabNTokens(vocab)
	if anchor < 0 || int32(anchor) >= nVocab {
		return Proposal{}, fmt.Errorf("DSpark generate: anchor %d outside vocabulary", anchor)
	}
	if err := llama.BatchExtClear(r.batch); err != nil {
		return Proposal{}, fmt.Errorf("DSpark generation batch: %w", err)
	}
	block := r.blockState[:(count+1)*r.width]
	clear(block)
	copy(block, state)
	for i := range count + 1 {
		token := r.mask
		if i == 0 {
			token = anchor
		}
		if i > 0 && temperature > 0 {
			block[i*r.width], block[i*r.width+1] = random(), 1/temperature
		}
		idx, err := llama.BatchExtAddToken(r.batch, seq, token)
		if err != nil {
			return Proposal{}, fmt.Errorf("DSpark block row %d: %w", i, err)
		}
		if err := llama.BatchExtSetPos(r.batch, idx, position+llama.Pos(i)); err != nil {
			return Proposal{}, fmt.Errorf("DSpark block position %d: %w", i, err)
		}
		if err := llama.BatchExtSetOutputLogits(r.batch, idx, true); err != nil {
			return Proposal{}, fmt.Errorf("DSpark block output %d: %w", i, err)
		}
		if err := llama.BatchExtSetEmbdState(r.batch, idx, block[i*r.width:(i+1)*r.width], r.width); err != nil {
			return Proposal{}, fmt.Errorf("DSpark block state %d: %w", i, err)
		}
	}
	// Decode can partially mutate KV even on failure. Never remove positions
	// below the validated accepted boundary, and retain both errors on failure.
	defer func() {
		llama.Synchronize(r.Context)
		removed, cleanupErr := llama.MemorySeqRm(r.Memory, seq, position, -1)
		if cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("DSpark block cleanup: %w", cleanupErr))
		}
		if !removed {
			err = errors.Join(err, fmt.Errorf("DSpark block cleanup: native suffix removal failed"))
		}
	}()
	code, err := llama.Process(r.Context, llama.ProcessTypeDecode, r.batch)
	if err != nil {
		return Proposal{}, fmt.Errorf("DSpark block decode: %w", err)
	}
	if code != 0 {
		return Proposal{}, fmt.Errorf("DSpark block decode returned %d", code)
	}
	llama.Synchronize(r.Context)
	draws := yzmaspec.GetEmbeddingsNextN(r.Context, count+1, r.width)
	if len(draws) != len(block) {
		return Proposal{}, fmt.Errorf("DSpark block: missing packed prediction rows")
	}
	proposal.Candidates = make([]llama.Token, 0, count)
	for i := 1; i <= count; i++ {
		row := draws[i*r.width : (i+1)*r.width]
		token, err := packedToken(row[0], nVocab)
		if err != nil {
			return Proposal{}, fmt.Errorf("DSpark prediction row %d: %w", i, err)
		}
		// Truncating at a sampled EOG would condition q on a non-EOG draw.
		// Keep sampled EOG proposals; authoritative target verification stops.
		if temperature == 0 && llama.VocabIsEOG(vocab, token) {
			break
		}
		if temperature > 0 {
			q, err := packedDistribution(row, token, nVocab)
			if err != nil {
				return Proposal{}, fmt.Errorf("DSpark distribution row %d: %w", i, err)
			}
			proposal.Distributions = append(proposal.Distributions, q)
		}
		proposal.Candidates = append(proposal.Candidates, token)
	}
	return proposal, nil
}

func packedDistribution(row []float32, token llama.Token, vocabulary int32) ([]llama.DraftCandidate, error) {
	if len(row) < 2 {
		return nil, fmt.Errorf("missing sparse distribution header")
	}
	k := int(row[1])
	if k <= 0 || k > (len(row)-2)/2 || float32(k) != row[1] {
		return nil, fmt.Errorf("invalid sparse distribution count %g", row[1])
	}
	q := make([]llama.DraftCandidate, 0, k)
	var sum float64
	found := false
	for i := range k {
		id, err := packedToken(row[2+i], vocabulary)
		if err != nil {
			return nil, err
		}
		prob := row[2+k+i]
		if prob < 0 || math.IsNaN(float64(prob)) || math.IsInf(float64(prob), 0) {
			return nil, fmt.Errorf("invalid probability %g", prob)
		}
		for j := range i {
			if row[2+j] == float32(id) {
				return nil, fmt.Errorf("duplicate sparse token %d", id)
			}
		}
		// Underflow can leave explicit zero entries in the packed graph law.
		// Residual sampling treats omitted entries as zero q support.
		if prob > 0 {
			q = append(q, llama.DraftCandidate{Tok: id, Prob: prob})
		}
		sum += float64(prob)
		found = found || id == token && prob > 0
	}
	if !found || math.Abs(sum-1) > 0.0001 {
		return nil, fmt.Errorf("invalid sparse law: sum=%g selected token supported=%t", sum, found)
	}
	return q, nil
}

func greedyCount(requested, limit, batch, ubatch, context int, position llama.Pos) int {
	// The anchor consumes one row as well. Noncausal blocks must fit in a
	// single physical microbatch; splitting would change their attention.
	return max(0, min(requested, limit, batch-1, ubatch-1, context-int(position)-1))
}

func packedToken(value float32, vocabulary int32) (llama.Token, error) {
	v := float64(value)
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v >= float64(vocabulary) || math.Trunc(v) != v {
		return 0, fmt.Errorf("invalid packed token ID %g for vocabulary size %d", value, vocabulary)
	}
	return llama.Token(value), nil
}
