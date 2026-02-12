package model

import (
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"go.opentelemetry.io/otel/attribute"
)

// addPrefillChunk adds the next chunk of prefill tokens to the batch.
// Returns false only on shutdown or context cancellation; true otherwise.
func (e *batchEngine) addPrefillChunk(s *slot) bool {
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

	nBatch := e.model.cfg.NBatch
	remaining := len(s.prefillTokens) - s.nPrefilled

	// Limit chunk size to available space in batch (total across all slots
	// must not exceed NBatch).
	availableInBatch := nBatch - int(e.batch.NTokens)
	if availableInBatch <= 0 {
		s.iBatch = -1
		return true
	}

	chunkSize := min(remaining, availableInBatch)

	// Add chunk of tokens to batch.
	for i := range chunkSize {
		tok := s.prefillTokens[s.nPrefilled+i]
		isLast := s.nPrefilled+i == len(s.prefillTokens)-1
		e.batch.Add(tok, s.nPast, s.seqIDs, isLast)
		s.nPast++
	}
	s.nPrefilled += chunkSize

	prefillDuration := time.Since(prefillStart)
	metrics.AddPrefillTime(e.model.modelInfo.ID, prefillDuration)

	// Check if prefill is complete.
	if s.nPrefilled >= len(s.prefillTokens) {
		s.iBatch = e.batch.NTokens - 1
		s.prefillTokens = nil
		if s.span.IsRecording() {
			s.span.SetAttributes(attribute.String("prefill-nonmedia", prefillDuration.String()))
		}
		return true
	}

	s.iBatch = -1
	return true
}
