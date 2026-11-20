// Package moxierun owns on-demand initialization and shutdown of Moxie services.
package moxierun

import (
	"context"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
)

// Runtime owns one successful initialization and its cleanup. Failed
// attempts are shared by concurrent callers, but not cached for later requests.
// Ordinary connection outages leave established clients and workers intact.
type Runtime struct {
	ctx    context.Context
	cancel context.CancelFunc
	start  func(context.Context, context.Context) (Services, func(), error)
	mu     sync.Mutex
	ready  bool
	closed bool
	cfg    Services
	stop   func()
	active *moxieAttempt
}

type moxieAttempt struct {
	done chan struct{}
	cfg  Services
	err  error
}

// New constructs a runtime without contacting dependencies or creating storage.
// Resolve initializes the services on first use. The host must call Close before
// releasing the borrowed model catalog and pool.
func New(ctx context.Context, log *logger.Logger, cfg Config) *Runtime {
	return newRuntime(ctx, func(attemptCtx, serviceCtx context.Context) (Services, func(), error) {
		services, stop, err := start(attemptCtx, serviceCtx, log, cfg)
		if err != nil {
			log.Error(attemptCtx, "moxie", "status", "fine-tuning unavailable; next request can retry", "ERROR", err)
		}
		return services, stop, err
	})
}

func newRuntime(ctx context.Context, start func(context.Context, context.Context) (Services, func(), error)) *Runtime {
	ctx, cancel := context.WithCancel(ctx)
	m := Runtime{ctx: ctx, cancel: cancel, start: start}
	return &m
}

// Resolve returns established services or shares one initialization attempt with
// concurrent callers. A failed attempt can be retried by a later call.
func (m *Runtime) Resolve(ctx context.Context) (Services, error) {
	m.mu.Lock()
	switch {
	case m.closed || m.ctx.Err() != nil:
		m.mu.Unlock()
		return Services{}, context.Canceled
	case ctx.Err() != nil:
		m.mu.Unlock()
		return Services{}, ctx.Err()
	case m.ready:
		cfg := m.cfg
		m.mu.Unlock()
		return cfg, nil
	case m.active != nil:
		attempt := m.active
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return Services{}, ctx.Err()
		case <-attempt.done:
			return attempt.cfg, attempt.err
		}
	}
	attempt := &moxieAttempt{done: make(chan struct{})}
	m.active = attempt
	m.mu.Unlock()

	initCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	unhook := context.AfterFunc(m.ctx, cancel)
	cfg, stop, err := m.start(initCtx, m.ctx)
	unhook()
	cancel()

	m.mu.Lock()
	if err == nil && (m.closed || m.ctx.Err() != nil) {
		// Shutdown raced with a successful start. Release the new resources
		// before waking callers or letting close return.
		m.mu.Unlock()
		stop()
		err = context.Canceled
		m.mu.Lock()
	}
	if err == nil {
		m.cfg, m.stop, m.ready = cfg, stop, true
	}
	attempt.cfg, attempt.err = cfg, err
	m.active = nil
	close(attempt.done)
	m.mu.Unlock()
	return cfg, err
}

// Close cancels initialization and background work, then releases owned resources.
func (m *Runtime) Close() {
	m.mu.Lock()
	m.closed = true
	m.cancel()
	attempt, stop := m.active, m.stop
	m.stop = nil
	m.mu.Unlock()
	if attempt != nil {
		<-attempt.done
	}
	if stop != nil {
		stop()
	}
}
