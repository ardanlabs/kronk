package kronk

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func (krn *Kronk) acquireModel(ctx context.Context) (*model.Model, error) {
	err := func() error {
		krn.shutdown.Lock()
		defer krn.shutdown.Unlock()

		if krn.shutdownFlag {
			return fmt.Errorf("acquire-model: kronk has been unloaded")
		}

		krn.activeStreams.Add(1)
		return nil
	}()

	if err != nil {
		return nil, err
	}

	// Acquire backpressure slot.
	select {
	case <-ctx.Done():
		krn.activeStreams.Add(-1)
		return nil, ctx.Err()

	case krn.sem <- struct{}{}:
	}

	return krn.model, nil
}

func (krn *Kronk) releaseModel() {
	<-krn.sem
	krn.activeStreams.Add(-1)
}
