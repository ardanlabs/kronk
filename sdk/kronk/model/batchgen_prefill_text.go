package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/hybridgroup/yzma/pkg/llama"
	"go.opentelemetry.io/otel/attribute"
)

func (e *batchEngine) nextPrefillSlot() (*slot, int) {
	for offset := range e.slots {
		idx := (e.prefillNext + offset) % len(e.slots)
		s := e.slots[idx]
		if s.active && s.prefillTokens != nil {
			return s, idx
		}
	}

	return nil, -1
}

func (e *batchEngine) prefillSlotIDs() []int {
	var ids []int
	for _, s := range e.slots {
		if s.active && s.prefillTokens != nil {
			ids = append(ids, s.id)
		}
	}

	return ids
}

// stagedSlotProgress holds ordinary target progress until submission succeeds.
// It does not represent completed GPU work; existing synchronization boundaries
// must still be observed before reading or releasing native state.
type stagedSlotProgress struct {
	slot        *slot
	nextPast    llama.Pos
	outputIndex int32
}

func (p stagedSlotProgress) commit() {
	p.slot.nPast = p.nextPast
	p.slot.iBatch = p.outputIndex
}

type stagedTextPrefill struct {
	stagedSlotProgress
	nextPrefilled int
	complete      bool
	duration      time.Duration
}

func (p stagedTextPrefill) commit() {
	p.stagedSlotProgress.commit()
	p.slot.nPrefilled = p.nextPrefilled
	if p.complete {
		p.slot.prefillTokens = nil
		if p.slot.span.IsRecording() {
			p.slot.span.SetAttributes(attribute.String("prefill-nonmedia", p.duration.String()))
		}
	}
}

// addPrefillChunk stages the next chunk without advancing slot progress.
// The chunkLimit parameter caps how many tokens the current prefill owner may
// add in one decode iteration.
// Returns false on shutdown, context cancellation, or an internal error. It
// finishes the slot before returning an internal error.
func (e *batchEngine) addPrefillChunk(s *slot, chunkLimit int) (stagedTextPrefill, bool) {
	if s.prefillTokens == nil || s.nPrefilled >= len(s.prefillTokens) {
		return stagedTextPrefill{}, true
	}

	// Check for cancellation before processing chunk.
	select {
	case <-e.shutdownCh:
		return stagedTextPrefill{}, false

	case <-s.job.ctx.Done():
		return stagedTextPrefill{}, false

	default:
	}

	prefillStart := time.Now()

	nBatch := e.model.cfg.EffectiveNBatch()
	remaining := len(s.prefillTokens) - s.nPrefilled

	// Limit chunk size to available space in batch (total across all slots
	// must not exceed NBatch).
	availableInBatch := nBatch - e.batch.len()
	if availableInBatch <= 0 {
		return stagedTextPrefill{}, true
	}

	chunkSize := prefillContributionSize(remaining, availableInBatch, chunkLimit)
	if chunkSize == 0 {
		return stagedTextPrefill{}, true
	}

	// Add chunk of tokens to batch.
	batchStart := e.batch.len()
	basePos := s.nPast
	for i := range chunkSize {
		tok := s.prefillTokens[s.nPrefilled+i]
		isLast := s.nPrefilled+i == len(s.prefillTokens)-1
		output := extendedBatchOutputNone
		if isLast {
			output = extendedBatchOutputLogits
		}
		if _, err := e.batch.addToken(tok, s.nPast+llama.Pos(i), s.seqIDs, output); err != nil {
			rollbackErr := e.batch.truncate(batchStart)
			e.finishSlot(s, fmt.Errorf("add prefill token %d: %w", s.nPrefilled+i, errors.Join(err, rollbackErr)))
			return stagedTextPrefill{}, false
		}
	}
	e.speculation.TargetRowsStaged(s.id, speculation.TargetRange{
		Start:   int32(batchStart),
		Count:   int32(chunkSize),
		BasePos: basePos,
	})
	prefillDuration := time.Since(prefillStart)
	metrics.AddPrefillTime(e.model.modelInfo.ID, "text", prefillDuration)

	pending := stagedTextPrefill{
		slot:          s,
		nextPast:      basePos + llama.Pos(chunkSize),
		outputIndex:   -1,
		nextPrefilled: s.nPrefilled + chunkSize,
		duration:      prefillDuration,
	}
	pending.complete = pending.nextPrefilled == len(s.prefillTokens)
	if pending.complete {
		pending.outputIndex = int32(e.batch.len() - 1)
	}

	return pending, true
}

func prefillContributionSize(remaining, availableInBatch, chunkLimit int) int {
	return max(min(remaining, availableInBatch, chunkLimit), 0)
}
