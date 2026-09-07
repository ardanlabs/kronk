package malina

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ardanlabs/kronk/sdk/malina/model"
)

var (
	// ErrInvalidRequest identifies invalid generation parameters.
	ErrInvalidRequest = model.ErrInvalidRequest

	// ErrAdmissionTimeout identifies expiration while waiting for admission.
	ErrAdmissionTimeout = errors.New("generation admission timed out")

	// ErrClosed identifies use after unloading has begun.
	ErrClosed = errors.New("malina is closed")

	// ErrPoisoned identifies a terminal native generation failure.
	ErrPoisoned = errors.New("malina is poisoned")
)

type backend interface {
	Generate(context.Context, model.GenerateParams) (model.GeneratedImage, error)
	Detail(context.Context, model.DetailParams) (model.GeneratedImage, error)
	GenerateVideo(context.Context, model.VideoParams) (model.GeneratedVideo, error)
	Stop()
	Unload() error
	Config() model.Config
	Info() model.ModelInfo
}

var newBackend = func(ctx context.Context, cfg model.Config) (backend, error) {
	return model.NewModel(ctx, cfg)
}

type request struct {
	ctx   context.Context
	run   func(backend) (any, error)
	done  chan result
	mu    sync.Mutex
	start bool
	stop  bool
}

type result struct {
	value any
	err   error
}

// Malina provides a concurrency-safe API around a pool of reusable native
// model contexts. Each context performs one generation at a time.
type Malina struct {
	backends  []backend
	config    model.Config
	jobs      chan *request
	stop      chan struct{}
	done      chan struct{}
	workers   sync.WaitGroup
	mu        sync.Mutex
	closed    bool
	terminal  error
	unloadErr error
	active    atomic.Int64
	admit     chan struct{}
}

// New provides pooled image generation using a background model-loading
// context.
func New(opts ...model.Option) (*Malina, error) {
	return NewWithContext(context.Background(), opts...)
}

// NewWithContext provides pooled image generation and loads every configured
// model context using ctx.
func NewWithContext(ctx context.Context, opts ...model.Option) (*Malina, error) {
	if !Initialized() {
		return nil, errors.New("new: the Init() function has not been called")
	}

	cfg, err := model.NewConfig(opts...)
	if err != nil {
		return nil, fmt.Errorf("new: %w", err)
	}

	backends := make([]backend, 0, cfg.Concurrency)
	for range cfg.Concurrency {
		b, err := newBackend(ctx, cfg)
		if err != nil {
			for _, loaded := range backends {
				loaded.Stop()
				if unloadErr := loaded.Unload(); unloadErr != nil {
					err = errors.Join(err, fmt.Errorf("unloading model after load failure: %w", unloadErr))
				}
			}
			return nil, fmt.Errorf("new: loading model: %w", err)
		}
		backends = append(backends, b)
	}

	resolved := backends[0].Config()
	resolved.Concurrency = cfg.Concurrency
	resolved.QueueDepth = cfg.QueueDepth
	resolved.AdmissionTimeout = cfg.AdmissionTimeout

	m := Malina{
		backends: backends,
		config:   resolved,
		jobs:     make(chan *request),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
		admit:    make(chan struct{}, cfg.Concurrency+cfg.QueueDepth),
	}
	m.workers.Add(len(backends))
	for _, b := range backends {
		go m.worker(b)
	}
	go func() {
		m.workers.Wait()
		close(m.done)
	}()

	return &m, nil
}

// Generate admits and synchronously executes one image generation. Waiting
// for admission is cancellable. Canceling ctx requests native cancellation;
// the call waits for native execution to stop and resets the model context
// before returning.
func (m *Malina) Generate(ctx context.Context, params model.GenerateParams) (model.GeneratedImage, error) {
	if err := params.Validate(); err != nil {
		return model.GeneratedImage{}, err
	}
	value, err := m.submit(ctx, func(b backend) (any, error) {
		return b.Generate(ctx, params)
	})
	if err != nil {
		return model.GeneratedImage{}, err
	}
	return value.(model.GeneratedImage), nil
}

// Detail admits and synchronously executes one ADetailer refinement.
func (m *Malina) Detail(ctx context.Context, params model.DetailParams) (model.GeneratedImage, error) {
	if err := params.Validate(); err != nil {
		return model.GeneratedImage{}, err
	}

	value, err := m.submit(ctx, func(b backend) (any, error) {
		return b.Detail(ctx, params)
	})
	if err != nil {
		return model.GeneratedImage{}, err
	}

	return value.(model.GeneratedImage), nil
}

// GenerateVideo admits and synchronously executes one AnimateDiff generation.
func (m *Malina) GenerateVideo(ctx context.Context, params model.VideoParams) (model.GeneratedVideo, error) {
	if err := params.Validate(); err != nil {
		return model.GeneratedVideo{}, err
	}

	value, err := m.submit(ctx, func(b backend) (any, error) {
		return b.GenerateVideo(ctx, params)
	})
	if err != nil {
		return model.GeneratedVideo{}, err
	}

	return value.(model.GeneratedVideo), nil
}

func (m *Malina) submit(ctx context.Context, run func(backend) (any, error)) (any, error) {
	if err := m.closedError(); err != nil {
		return nil, err
	}

	timer := time.NewTimer(m.config.AdmissionTimeout)
	defer timer.Stop()

	select {
	case m.admit <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, errors.Join(ErrAdmissionTimeout, context.DeadlineExceeded)
	case <-m.stop:
		return nil, m.closedError()
	}
	defer func() { <-m.admit }()

	m.active.Add(1)
	defer m.active.Add(-1)

	r := request{
		ctx:  ctx,
		run:  run,
		done: make(chan result, 1),
	}

	select {
	case m.jobs <- &r:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-m.stop:
		return nil, m.closedError()
	}

	select {
	case out := <-r.done:
		return generationResult(ctx, out)

	case <-ctx.Done():
		if r.cancel() {
			return nil, ctx.Err()
		}
		return generationResult(ctx, <-r.done)

	case <-m.stop:
		if r.cancel() {
			return nil, m.closedError()
		}
		return generationResult(ctx, <-r.done)
	}
}

func generationResult(ctx context.Context, out result) (any, error) {
	if out.err != nil {
		return out.value, out.err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return out.value, nil
}

func (r *request) cancel() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.start {
		return false
	}
	r.stop = true

	return true
}

// ModelConfig returns a copy of the resolved model configuration.
func (m *Malina) ModelConfig() model.Config {
	return m.config
}

// ModelInfo returns descriptive information for the loaded model.
func (m *Malina) ModelInfo() model.ModelInfo {
	return m.backends[0].Info()
}

// SystemInfo returns native library and host diagnostics.
func (m *Malina) SystemInfo() SystemDiagnostics {
	return systemDiagnostics()
}

// ActiveGenerations returns the number of running and queued generation calls.
func (m *Malina) ActiveGenerations() int {
	return int(m.active.Load())
}

// Ready reports whether the model can accept generation requests.
func (m *Malina) Ready() bool {
	return m.closedError() == nil
}

// Unload stops admission and waits for safe native context release. If ctx
// expires, cleanup continues and a later call can wait for its completion.
func (m *Malina) Unload(ctx context.Context) error {
	m.mu.Lock()
	if !m.closed {
		m.closed = true
		m.stopBackends()
		close(m.stop)
	}
	m.mu.Unlock()

	select {
	case <-m.done:
		m.mu.Lock()
		err := m.unloadErr
		m.mu.Unlock()
		return err

	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Malina) worker(b backend) {
	defer m.workers.Done()
	defer m.unloadBackend(b)

	for {
		select {
		case <-m.stop:
			return

		case r := <-m.jobs:
			if err := m.start(r); err != nil {
				r.done <- result{err: err}
				continue
			}

			value, err := r.run(b)
			if errors.Is(err, context.Canceled) && r.ctx.Err() == nil {
				if closedErr := m.closedError(); closedErr != nil {
					err = closedErr
				}
			}
			if errors.Is(err, model.ErrNativeGeneration) {
				err = errors.Join(ErrPoisoned, err)
				m.poison()
			}

			r.done <- result{value: value, err: err}
			if errors.Is(err, ErrPoisoned) {
				return
			}
		}
	}
}

func (m *Malina) start(r *request) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()

	if m.closed {
		return errors.Join(ErrClosed, m.terminal)
	}
	if r.stop {
		return r.ctx.Err()
	}
	if err := r.ctx.Err(); err != nil {
		r.stop = true
		return err
	}

	r.start = true

	return nil
}

func (m *Malina) poison() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.terminal = ErrPoisoned
	if !m.closed {
		m.closed = true
		m.stopBackends()
		close(m.stop)
	}
}

func (m *Malina) closedError() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.closed {
		return nil
	}
	return errors.Join(ErrClosed, m.terminal)
}

func (m *Malina) stopBackends() {
	for _, b := range m.backends {
		b.Stop()
	}
}

func (m *Malina) unloadBackend(b backend) {
	err := b.Unload()
	m.mu.Lock()
	m.unloadErr = errors.Join(m.unloadErr, err)
	m.mu.Unlock()
}
