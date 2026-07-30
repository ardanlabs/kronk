package kronk

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// ErrAdmissionTimeout indicates that a request could not obtain an admission
// permit within the configured admission timeout.
var ErrAdmissionTimeout = errors.New("admission timeout")

func (krn *Kronk) acquireAdmission(ctx context.Context) (*model.Model, error) {
	started := time.Now()
	if krn.cfg.Log != nil {
		krn.cfg.Log(ctx, "request-lifecycle",
			"stage", 1,
			"stage_name", "admit-request",
			"status", "started",
			"capacity", cap(krn.admissionCh),
			"admitted", len(krn.admissionCh),
		)
	}

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
		if krn.cfg.Log != nil {
			krn.cfg.Log(ctx, "request-lifecycle",
				"stage", 1,
				"stage_name", "admit-request",
				"status", "error",
				"elapsed", time.Since(started),
				"err", err,
			)
		}
		return nil, err
	}

	// Bound only the admission wait. Once admitted, model processing continues
	// under the caller's original context.
	admissionCtx, cancel := context.WithTimeoutCause(ctx, krn.cfg.AdmissionTimeout(), ErrAdmissionTimeout)
	defer cancel()

	select {
	case <-admissionCtx.Done():
		krn.activeStreams.Add(-1)
		admissionErr := admissionCtx.Err()
		if cause := context.Cause(admissionCtx); errors.Is(cause, ErrAdmissionTimeout) {
			admissionErr = cause
		}
		if krn.cfg.Log != nil {
			status := "cancel"
			if admissionCtx.Err() == context.DeadlineExceeded {
				status = "timeout"
			}
			krn.cfg.Log(ctx, "request-lifecycle",
				"stage", 1,
				"stage_name", "admit-request",
				"status", status,
				"elapsed", time.Since(started),
				"capacity", cap(krn.admissionCh),
				"admitted", len(krn.admissionCh),
				"err", admissionErr,
			)
		}
		return nil, admissionErr

	case krn.admissionCh <- struct{}{}:
	}

	if krn.cfg.Log != nil {
		krn.cfg.Log(ctx, "request-lifecycle",
			"stage", 1,
			"stage_name", "admit-request",
			"status", "complete",
			"elapsed", time.Since(started),
			"capacity", cap(krn.admissionCh),
			"admitted", len(krn.admissionCh),
		)
	}

	return krn.model, nil
}

func (krn *Kronk) releaseAdmission() {
	<-krn.admissionCh
	krn.activeStreams.Add(-1)
}
