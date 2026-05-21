package bucky

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/bucky/model"
)

// Transcribe runs the whisper.cpp pipeline on the provided 16 kHz
// mono float32 PCM samples and returns the decoded text along with
// per-segment metadata. The call participates in the per-handle
// backpressure semaphore and blocks until a slot is available.
func (b *Bucky) Transcribe(ctx context.Context, samples []float32, opts ...model.TranscribeOption) (model.Transcription, error) {
	m, err := b.acquireModel(ctx)
	if err != nil {
		return model.Transcription{}, fmt.Errorf("transcribe: %w", err)
	}
	defer b.releaseModel()

	return m.Transcribe(ctx, samples, opts...)
}
