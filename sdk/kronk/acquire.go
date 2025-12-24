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
			return fmt.Errorf("acquire-model:kronk has been unloaded")
		}

		krn.activeStreams.Add(1)
		return nil
	}()

	if err != nil {
		return nil, err
	}

	// -------------------------------------------------------------------------

	select {
	case <-ctx.Done():
		krn.activeStreams.Add(-1)
		return nil, ctx.Err()

	case llama, ok := <-krn.models:
		if !ok {
			krn.activeStreams.Add(-1)
			return nil, fmt.Errorf("acquire-model:kronk has been unloaded")
		}

		return llama, nil
	}
}

func (krn *Kronk) releaseModel(llama *model.Model) {
	krn.models <- llama
	krn.activeStreams.Add(-1)
}
