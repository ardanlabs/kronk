// Package async provides a way to run tasks asynchronously.
package async

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/foundation/async/stores/dbsession"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/sdk/observ/otel"
	"github.com/google/uuid"
)

type taskFn func(ctx context.Context, sessionID uuid.UUID) (data []byte, err error)

type Async struct {
	log       *logger.Logger
	dbSession *dbsession.Store
	g         sync.Map
	wg        sync.WaitGroup
	timeout   time.Duration
}

func New(log *logger.Logger, dbSession *dbsession.Store, taskTimeout time.Duration) *Async {
	return &Async{
		log:       log,
		dbSession: dbSession,
		g:         sync.Map{},
		wg:        sync.WaitGroup{},
		timeout:   taskTimeout,
	}
}

func (a *Async) Shutdown(ctx context.Context) {
	a.g.Range(func(key, value any) bool {
		a.log.Info(ctx, "shutdown: async job", "SESSION_ID", key)
		value.(context.CancelFunc)()
		return true
	})

	a.log.Info(ctx, "waiting for all async jobs to shut down completely")
	a.wg.Wait()
}

// Store returns the store API.
func (a *Async) Store() *dbsession.Store {
	return a.dbSession
}

func (a *Async) Session(ctx context.Context, sessionID uuid.UUID) (dbsession.SessionData, error) {
	return a.dbSession.GetSession(ctx, sessionID)
}

func (a *Async) Run(ctx context.Context, task taskFn) (uuid.UUID, error) {
	sd, err := a.dbSession.NewSession()
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to create session: %w", err)
	}

	a.log.Info(ctx, "async: created", "SESSION_ID", sd.SessionID)

	traceID := otel.GetTraceID(ctx)

	a.wg.Go(func() {
		// We can't use the context being passed in because that will be
		// cancelled when the caller sends the session id back to the caller.

		ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
		defer cancel()

		ctx = otel.SetTraceID(ctx, traceID)

		a.log.Info(ctx, "async: started job", "SESSION_ID", sd.SessionID)

		a.g.Store(sd.SessionID, cancel)

		var err error
		defer func() {
			a.log.Info(ctx, "async: completed job", "SESSION_ID", sd.SessionID)

			cancel()
			a.g.Delete(sd.SessionID)

			if err != nil {
				a.log.Error(ctx, "async: error: failed to perform chat completion", "ERROR", err)

				resp := struct {
					Message string `json:"error"`
				}{
					Message: err.Error(),
				}

				data, err := json.Marshal(resp)
				if err != nil {
					data = []byte(err.Error())
					a.log.Error(ctx, "async: error: failed to marshal error response", "ERROR", err)
				}

				a.log.Info(ctx, "async: error: update session status:", "SESSION_ID", sd.SessionID, "status", dbsession.Error)

				if _, err = a.dbSession.UpdateSessionStatus(sd.SessionID, dbsession.Error, data); err != nil {
					a.log.Error(ctx, "async: error: failed to update session status", "ERROR", err, "SESSION_ID", sd.SessionID)
				}
			}
		}()

		resp, err := task(ctx, sd.SessionID)
		if err != nil {
			err = fmt.Errorf("failed to perform task: %w", err)
			return
		}

		a.log.Info(ctx, "async: update session status:", "SESSION_ID", sd.SessionID, "status", dbsession.Completed)

		if _, err = a.dbSession.UpdateSessionStatus(sd.SessionID, dbsession.Completed, resp); err != nil {
			err = fmt.Errorf("failed to update session status: %w", err)
			return
		}
	})

	return sd.SessionID, nil
}
