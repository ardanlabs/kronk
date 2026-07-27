package kronk

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func (krn *Kronk) acquireAdmission(ctx context.Context) (*model.Model, error) {
	err := func() error {
		krn.shutdown.Lock()
		defer krn.shutdown.Unlock()

		if krn.shutdownFlag {
			return fmt.Errorf("acquire-admission: kronk has been unloaded")
		}

		krn.activeStreams.Add(1)
		return nil
	}()

	if err != nil {
		return nil, err
	}

	// Bound only the admission wait. Once admitted, model processing continues
	// under the caller's original context.
	admissionCtx, cancel := context.WithTimeout(ctx, krn.cfg.AdmissionTimeout())
	defer cancel()

	select {
	case <-admissionCtx.Done():
		krn.activeStreams.Add(-1)
		return nil, admissionCtx.Err()

	case krn.admissionCh <- struct{}{}:
	}

	return krn.model, nil
}

func (krn *Kronk) releaseAdmission() {
	<-krn.admissionCh
	krn.activeStreams.Add(-1)
}
