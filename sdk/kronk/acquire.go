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

	// -------------------------------------------------------------------------
	// Stage 1: Acquire backpressure slot

	select {
	case <-ctx.Done():
		krn.activeStreams.Add(-1)
		return nil, ctx.Err()

	case krn.sem <- struct{}{}:
	}

	// -------------------------------------------------------------------------
	// Stage 2: Acquire model instance (only for pooled models)

	if krn.pool != nil {
		select {
		case <-ctx.Done():
			<-krn.sem
			krn.activeStreams.Add(-1)
			return nil, ctx.Err()

		case m := <-krn.pool:
			return m, nil
		}
	}

	return krn.models[0], nil
}

func (krn *Kronk) releaseModel(m *model.Model) {
	if krn.pool != nil {
		krn.pool <- m
	}

	<-krn.sem
	krn.activeStreams.Add(-1)
}
