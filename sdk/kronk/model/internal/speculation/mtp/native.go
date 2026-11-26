package mtp

import (
	"errors"
	"fmt"

	yzmaspec "github.com/hybridgroup/yzma/exp/speculative"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// PrepareDraft replaces the one-step batch with a token and its hidden state.
func (r *Resources) PrepareDraft(token llama.Token, position llama.Pos, seqIDs []llama.SeqId, hidden []float32) error {
	mode := "mtp"
	if r.mirrorCapacity == 0 {
		mode = "mtp shared"
	}
	if err := llama.BatchExtClear(r.DraftBatch); err != nil {
		return fmt.Errorf("%s draft: clearing batch: %w", mode, err)
	}
	if err := addBatchEntry(r.DraftBatch, token, position, seqIDs, hidden, r.embeddingSize); err != nil {
		return fmt.Errorf("%s draft: add token at pos %d: %w", mode, position, err)
	}
	return nil
}

func addBatchEntry(batch llama.BatchExt, token llama.Token, position llama.Pos, seqIDs []llama.SeqId, hidden []float32, nEmbd int) error {
	if len(seqIDs) == 0 {
		return errors.New("adding MTP batch entry: no sequence ID")
	}
	idx, err := llama.BatchExtAddToken(batch, seqIDs[0], token)
	if err != nil {
		return fmt.Errorf("adding token: %w", err)
	}
	for _, seqID := range seqIDs[1:] {
		if err := llama.BatchExtAddSeq(batch, idx, seqID); err != nil {
			return fmt.Errorf("adding sequence %d: %w", seqID, err)
		}
	}
	if err := llama.BatchExtSetEmbdState(batch, idx, hidden, nEmbd); err != nil {
		return fmt.Errorf("setting MTP state embedding: %w", err)
	}
	if err := llama.BatchExtSetPos(batch, idx, position); err != nil {
		return fmt.Errorf("setting position %d: %w", position, err)
	}
	if err := llama.BatchExtSetOutputLogits(batch, idx, true); err != nil {
		return fmt.Errorf("requesting logits: %w", err)
	}
	return nil
}

// DecodeDraft executes and synchronizes one prepared draft step. The host owns
// decode-code translation and request-local fallback before sampling any output.
func (r *Resources) DecodeDraft() (int32, error) {
	return r.decode(r.DraftBatch)
}

// SampleDraft reads the completed step's token and borrowed hidden output.
func (r *Resources) SampleDraft() (llama.Token, []float32) {
	return llama.SamplerSample(r.Sampler, r.Context, -1), yzmaspec.GetEmbeddingsNextNIth(r.Context, 0, r.embeddingSize)
}

// DecodeMirror executes a chunk rendered by the model's media-aware adapter.
func (r *Resources) DecodeMirror() (int32, error) {
	return r.decode(r.MirrorBatch)
}

func (r *Resources) decode(batch llama.BatchExt) (int32, error) {
	ret, err := llama.Process(r.Context, llama.ProcessTypeDecode, batch)
	if err == nil && ret == 0 {
		llama.Synchronize(r.Context)
	}
	return ret, err
}

// DeviceRows reports whether the target-to-draft device link is active.
func (r *Resources) DeviceRows() bool { return r.stateSource != 0 }

// TargetHidden borrows dense host rows until the target's next decode. Device
// mode returns nil so synchronization uses device row references instead.
func (r *Resources) TargetHidden(rows int) []float32 {
	if r.DeviceRows() {
		return nil
	}
	return yzmaspec.GetEmbeddingsNextN(r.target, rows, r.embeddingSize)
}

// CopyTargetHidden captures a range before target output can be overwritten.
// A missing host buffer returns false; device read failures return an error.
func (r *Resources) CopyTargetHidden(total, start, count int, out []float32) (bool, error) {
	if r.DeviceRows() {
		if !yzmaspec.GetEmbeddingsNextNRows(r.target, int64(start), r.embeddingSize, out) {
			return false, errors.New("verify-prenorm-capture: reading target device rows")
		}
		return true, nil
	}
	hidden := r.TargetHidden(total)
	if hidden == nil {
		return false, nil
	}
	copy(out, hidden[start*r.embeddingSize:(start+count)*r.embeddingSize])
	return true, nil
}

// PreparePendingRow seeds the sequence's reserved device row for shifted replay.
func (r *Resources) PreparePendingRow(seqID llama.SeqId, hidden []float32) (int32, error) {
	row := int32(llama.NBatch(r.target)) + int32(seqID)
	pending := r.MirrorHidden[:r.embeddingSize]
	clear(pending)
	if len(hidden) == r.embeddingSize {
		copy(pending, hidden)
	}
	if !yzmaspec.SetEmbeddingsNextNRows(r.target, int64(row), r.embeddingSize, pending) {
		return 0, errors.New("MTP device replay: writing pending hidden row")
	}
	return row, nil
}

// SetMirrorState attaches shifted MTP state without changing rendered token,
// projected input embeddings, positions, or sequence aliases. A nil hidden row
// selects the device row supplied by the host's target-batch mapping.
func (r *Resources) SetMirrorState(index int32, hidden []float32, deviceRow int32) error {
	if hidden == nil {
		if !yzmaspec.BatchExtSetEmbdStateRow(r.MirrorBatch, index, deviceRow) {
			return fmt.Errorf("setting MTP mirror device row %d", deviceRow)
		}
		return nil
	}
	if err := llama.BatchExtSetEmbdState(r.MirrorBatch, index, hidden, r.embeddingSize); err != nil {
		return fmt.Errorf("setting MTP mirror state: %w", err)
	}
	return nil
}

// ReadLastRow captures the next pending state from a completed target batch.
func (r *Resources) ReadLastRow(row int, out []float32) error {
	if !yzmaspec.GetEmbeddingsNextNRows(r.target, int64(row), r.embeddingSize, out) {
		return errors.New("MTP device replay: reading final hidden row")
	}
	return nil
}
