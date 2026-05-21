package bucky

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/bucky/model"
)

func (w *Whisper) acquireModel(ctx context.Context) (*model.Model, error) {
	err := func() error {
		w.shutdown.Lock()
		defer w.shutdown.Unlock()

		if w.shutdownFlag {
			return fmt.Errorf("acquire-model: whisper has been unloaded")
		}

		w.activeStreams.Add(1)
		return nil
	}()

	if err != nil {
		return nil, err
	}

	// Acquire backpressure slot.
	select {
	case <-ctx.Done():
		w.activeStreams.Add(-1)
		return nil, ctx.Err()

	case w.sem <- struct{}{}:
	}

	return w.model, nil
}

func (w *Whisper) releaseModel() {
	<-w.sem
	w.activeStreams.Add(-1)
}
