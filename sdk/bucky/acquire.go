package bucky

import (
	"context"
	"fmt"

	"github.com/ardanlabs/bucky/pkg/whisper"
)

// acquire reserves a backpressure slot and returns the whisper
// context to use for the call. Callers must invoke release in a
// deferred call once the slot is no longer needed.
func (w *Whisper) acquire(ctx context.Context) (whisper.Context, error) {
	err := func() error {
		w.shutdown.Lock()
		defer w.shutdown.Unlock()

		if w.shutdownFlag {
			return fmt.Errorf("acquire: whisper handle has been unloaded")
		}

		w.activeStreams.Add(1)
		return nil
	}()

	if err != nil {
		return 0, err
	}

	select {
	case <-ctx.Done():
		w.activeStreams.Add(-1)
		return 0, ctx.Err()

	case w.sem <- struct{}{}:
	}

	return w.handle, nil
}

// release returns the previously acquired slot to the semaphore.
func (w *Whisper) release() {
	<-w.sem
	w.activeStreams.Add(-1)
}
