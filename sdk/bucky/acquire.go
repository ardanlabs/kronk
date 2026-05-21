package bucky

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/bucky/model"
)

func (b *Bucky) acquireModel(ctx context.Context) (*model.Model, error) {
	err := func() error {
		b.shutdown.Lock()
		defer b.shutdown.Unlock()

		if b.shutdownFlag {
			return fmt.Errorf("acquire-model: whisper has been unloaded")
		}

		b.activeStreams.Add(1)
		return nil
	}()

	if err != nil {
		return nil, err
	}

	// Acquire backpressure slot.
	select {
	case <-ctx.Done():
		b.activeStreams.Add(-1)
		return nil, ctx.Err()

	case b.sem <- struct{}{}:
	}

	return b.model, nil
}

func (b *Bucky) releaseModel() {
	<-b.sem
	b.activeStreams.Add(-1)
}
