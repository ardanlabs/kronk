package bucky

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/bucky/model"
)

// NewStream opens a streaming transcription session against the loaded
// model. The session reserves one whisper.State for its lifetime and
// counts against ActiveStreams, so an open stream blocks Unload exactly
// like an in-flight Transcribe. The backpressure slot and pool state are
// both released when the returned stream's Close completes. Caller must
// call Close exactly once.
func (b *Bucky) NewStream(ctx context.Context, opts ...model.StreamOption) (*model.Stream, error) {
	m, err := b.acquireModel(ctx)
	if err != nil {
		return nil, fmt.Errorf("new-stream: %w", err)
	}

	s, err := m.NewStream(ctx, b.releaseModel, opts...)
	if err != nil {
		b.releaseModel()
		return nil, fmt.Errorf("new-stream: %w", err)
	}

	return s, nil
}
