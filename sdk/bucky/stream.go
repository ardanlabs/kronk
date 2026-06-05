package bucky

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/bucky/model"
)

// NewStream opens a streaming transcription session against the loaded
// model. The session reserves one whisper.State for its lifetime and
// counts against ActiveStreams, so an open stream blocks Unload exactly
// like an in-flight Transcribe. Caller must call Close exactly once.
//
// Unlike Transcribe, the pool slot is NOT released when this function
// returns. The slot is held until (*model.Stream).Close, at which point
// releaseModel must run. That lifecycle wiring is a TODO for the
// implementation step.
func (b *Bucky) NewStream(ctx context.Context, opts ...model.StreamOption) (*model.Stream, error) {
	m, err := b.acquireModel(ctx)
	if err != nil {
		return nil, fmt.Errorf("new-stream: %w", err)
	}

	s, err := m.NewStream(ctx, opts...)
	if err != nil {
		// TODO(impl): once the slot is held for the stream's lifetime,
		// only release here on the error path; Close releases on the
		// happy path. For the stub, release immediately.
		b.releaseModel()
		return nil, fmt.Errorf("new-stream: %w", err)
	}

	return s, nil
}
