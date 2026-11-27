package model

import (
	"fmt"
	"slices"

	dsparkengine "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation/dspark"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// WithDSparkTarget supplies a stable target batch to the internal controller.
// Slot bookkeeping and decode locking remain owned by the batch engine.
func (e *batchEngine) WithDSparkTarget(consume func(dsparkengine.SyncInput) []dsparkengine.SyncResult) {
	batch := e.decodedPrefill
	if batch == nil {
		e.model.decodeMu.Lock()
		defer e.model.decodeMu.Unlock()
		batch = e.batch
	}
	d := e.model.draft.(*dsparkDrafter)
	d.withTargetRows(e, batch, consume)
}

// ResetDSparkSequence maps an execution slot to its owned sidecar sequence.
func (e *batchEngine) ResetDSparkSequence(slotID int) error {
	d := e.model.draft.(*dsparkDrafter)
	return d.r.Reset(e.slots[slotID].seqID)
}

// DSparkGenerationInput adapts request parameters and target capacity without
// interpreting DSpark's block or accepted-prefix policy.
func (e *batchEngine) DSparkGenerationInput(slotID int) dsparkengine.GenerationInput {
	s := e.slots[slotID]
	d := e.model.draft.(*dsparkDrafter)
	return dsparkengine.GenerationInput{
		Temperature:        s.job.params.Temperature,
		SamplingCompatible: independentDraftSamplingSupported(s.job.params),
		HasMedia:           len(s.job.media) > 0 || s.job.imcSessionMedia,
		MaxDraft:           e.maxDraftForSlot(s, d.c.nDraft),
		Generate: func(count int) (dsparkengine.Proposal, error) {
			proposal, err := d.r.Generate(s.seqID, s.sampled, s.nPast, count, s.job.params.Temperature, s.specRNG.Float32)
			if err == nil {
				s.specDraftedTotal += len(proposal.Candidates)
			}
			return proposal, err
		},
	}
}

func (d *dsparkDrafter) withTargetRows(e *batchEngine, batch *extendedBatch, consume func(dsparkengine.SyncInput) []dsparkengine.SyncResult) {
	input := dsparkengine.SyncInput{Resources: d.r, RowCount: batch.len()}
	for _, s := range e.slots {
		if !s.active {
			continue
		}
		rows, err := dsparkFeatureRows(batch.entries, s.seqID)
		input.Sequences = append(input.Sequences, dsparkengine.SequenceInput{
			Slot: s.id, Sequence: s.seqID, Rows: rows, RowError: err,
		})
	}
	for _, result := range consume(input) {
		s := e.slots[result.Slot]
		if result.Err != nil {
			e.model.log(s.job.ctx, "draft-model-dspark", "status", "feature-sync-disabled",
				"slot", s.id, "seq", result.Sequence, "reason", "accepted-prefix-unavailable", "err", result.Err)
			continue
		}
		e.model.log(s.job.ctx, "draft-model-dspark", "status", "features-synchronized",
			"slot", s.id, "seq", result.Sequence, "rows", result.Rows, "next_position", result.NextPosition)
	}
}

func dsparkFeatureRows(entries []extendedBatchEntry, seq llama.SeqId) ([]dsparkengine.FeatureRow, error) {
	var rows []dsparkengine.FeatureRow
	for i, entry := range entries {
		if entry.sequenceID != seq && !slices.Contains(entry.extraSequenceIDs, seq) {
			continue
		}
		if !entry.hasToken || len(entry.embedding) != 0 || entry.positionCount != 1 {
			return nil, fmt.Errorf("DSpark supports only text rows with one position (row %d)", i)
		}
		rows = append(rows, dsparkengine.FeatureRow{Row: i, Sequence: seq, Position: entry.positions[0]})
	}
	return rows, nil
}

var _ dsparkengine.Host = (*batchEngine)(nil)
