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

// addPrefillChunk adds the next chunk of generation prefill tokens to the batch.
// The chunkLimit parameter caps how many tokens the current prefill owner may
// add in one decode iteration.
// Returns false on shutdown, context cancellation, or an internal error. It
// finishes the slot before returning an internal error.
func (e *batchEngine) addPrefillChunk(s *slot, chunkLimit int) bool {
	if s.prefillTokens == nil || s.nPrefilled >= len(s.prefillTokens) {
		return true
	}

	// Check for cancellation before processing chunk.
	select {
	case <-e.shutdownCh:
		return false

	case <-s.job.ctx.Done():
		return false

	default:
	}

	prefillStart := time.Now()

	nBatch := e.model.cfg.EffectiveNBatch()
	remaining := len(s.prefillTokens) - s.nPrefilled

	// Limit chunk size to available space in batch (total across all slots
	// must not exceed NBatch).
	availableInBatch := nBatch - e.batch.len()
	if availableInBatch <= 0 {
		s.iBatch = -1
		return true
	}

	chunkSize := prefillContributionSize(remaining, availableInBatch, chunkLimit)

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
			return false
		}
	}
	e.speculation.TargetRowsStaged(s.id, speculation.TargetRange{
		Start:   int32(batchStart),
		Count:   int32(chunkSize),
		BasePos: basePos,
	})
	s.nPast += llama.Pos(chunkSize)
	s.nPrefilled += chunkSize

	prefillDuration := time.Since(prefillStart)
	metrics.AddPrefillTime(e.model.modelInfo.ID, "text", prefillDuration)

	// Check if prefill is complete.
	if s.nPrefilled >= len(s.prefillTokens) {
		s.iBatch = int32(e.batch.len() - 1)
		s.prefillTokens = nil
		if s.span.IsRecording() {
			s.span.SetAttributes(attribute.String("prefill-nonmedia", prefillDuration.String()))
		}
		return true
	}

	s.iBatch = -1
	return true
}

func prefillContributionSize(remaining, availableInBatch, chunkLimit int) int {
	return max(min(remaining, availableInBatch, chunkLimit), 0)
}
