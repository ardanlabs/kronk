package bucky

import (
	"context"
	"errors"
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

	// Bound only the admission wait. Once admitted, model processing continues
	// under the caller's original context.
	admissionCtx, cancel := context.WithTimeoutCause(ctx, b.cfg.AdmissionTimeout, ErrAdmissionTimeout)
	defer cancel()

	select {
	case <-admissionCtx.Done():
		b.activeStreams.Add(-1)
		if cause := context.Cause(admissionCtx); errors.Is(cause, ErrAdmissionTimeout) {
			return nil, cause
		}
		return nil, admissionCtx.Err()

	case b.admissionCh <- struct{}{}:
	}

	return b.model, nil
}

func (b *Bucky) releaseModel() {
	<-b.admissionCh
	b.activeStreams.Add(-1)
}
