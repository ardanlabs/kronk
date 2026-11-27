package dspark

import "github.com/hybridgroup/yzma/pkg/llama"

// SyncInput borrows the completed target batch's row mapping and resource owner.
// The host keeps target outputs valid until SynchronizeTarget returns.
type SyncInput struct {
	Resources *Resources
	RowCount  int
	Sequences []SequenceInput
}

// SequenceInput describes one active slot's actual contribution to the batch.
// RowError reports inputs the host cannot represent as accepted feature rows.
type SequenceInput struct {
	Slot     int
	Sequence llama.SeqId
	Rows     []FeatureRow
	RowError error
}

// SyncResult reports a completed handoff or request-local synchronization loss.
type SyncResult struct {
	Slot         int
	Sequence     llama.SeqId
	Rows         int
	NextPosition llama.Pos
	Err          error
}

// SynchronizeTarget captures target features once, then synchronizes each
// contributing sequence. Missing prefixes or unusable inputs disable only that
// sequence until Reset; they never authorize injecting a suffix as a prefix.
func SynchronizeTarget(input SyncInput) []SyncResult {
	if input.RowCount == 0 {
		return nil
	}
	r := input.Resources
	captureErr := r.Capture(input.RowCount)
	return synchronizeCaptured(input, captureErr)
}

func synchronizeCaptured(input SyncInput, captureErr error) []SyncResult {
	r := input.Resources
	var results []SyncResult
	for _, seq := range input.Sequences {
		if r.sequences[seq.Sequence].disabled || len(seq.Rows) == 0 && seq.RowError == nil {
			continue
		}
		err := captureErr
		if err == nil {
			err = seq.RowError
		}
		if err == nil {
			err = r.Synchronize(seq.Rows)
		}
		if err != nil {
			r.sequences[seq.Sequence].disabled = true
		}
		results = append(results, SyncResult{Slot: seq.Slot, Sequence: seq.Sequence,
			Rows: len(seq.Rows), NextPosition: r.NextPosition(seq.Sequence), Err: err})
	}
	return results
}
