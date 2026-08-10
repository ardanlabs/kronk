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
	Stop()
	Unload() error
	Config() model.Config
	Info() model.ModelInfo
}

var newBackend = func(ctx context.Context, cfg model.Config) (backend, error) {
	return model.NewModel(ctx, cfg)
}

type request struct {
	ctx    context.Context
	params model.GenerateParams
	done   chan result
	mu     sync.Mutex
	start  bool
	stop   bool
}

type result struct {
	image model.GeneratedImage
	err   error
}

// Malina provides a concurrency-safe API around one reusable native model
// context. Generation calls are serialized because a stable-diffusion context
// is not safe for concurrent use.
type Malina struct {
	backend   backend
	config    model.Config
	jobs      chan *request
	stop      chan struct{}
	done      chan struct{}
	mu        sync.Mutex
	closed    bool
	terminal  error
	unloadErr error
	active    atomic.Int64
	admit     chan struct{}
}

// New provides image generation using a background model-loading context.
func New(opts ...model.Option) (*Malina, error) {
	return NewWithContext(context.Background(), opts...)
}

// NewWithContext provides image generation and loads the model using ctx.
func NewWithContext(ctx context.Context, opts ...model.Option) (*Malina, error) {
	if !Initialized() {
		return nil, errors.New("new: the Init() function has not been called")
	}

	cfg, err := model.NewConfig(opts...)
	if err != nil {
		return nil, fmt.Errorf("new: %w", err)
	}

	b, err := newBackend(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("new: loading model: %w", err)
	}

	m := Malina{
		backend: b,
		config:  b.Config(),
		jobs:    make(chan *request),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		admit:   make(chan struct{}, cfg.QueueDepth),
	}
	go m.worker()

	return &m, nil
}

// Generate admits and synchronously executes one image generation. Waiting
// for admission is cancellable. Once native generation starts, the call waits
// for native completion before returning a cancellation error so the context
// cannot be reused or freed while native code is active.
func (m *Malina) Generate(ctx context.Context, params model.GenerateParams) (model.GeneratedImage, error) {
	if err := params.Validate(); err != nil {
		return model.GeneratedImage{}, err
	}
	if err := m.closedError(); err != nil {
		return model.GeneratedImage{}, err
	}

	timer := time.NewTimer(m.config.AdmissionTimeout)
	defer timer.Stop()

	select {
	case m.admit <- struct{}{}:
	case <-ctx.Done():
		return model.GeneratedImage{}, ctx.Err()
	case <-timer.C:
		return model.GeneratedImage{}, errors.Join(ErrAdmissionTimeout, context.DeadlineExceeded)
	case <-m.stop:
		return model.GeneratedImage{}, m.closedError()
	}
	defer func() { <-m.admit }()

	m.active.Add(1)
	defer m.active.Add(-1)

	r := request{
		ctx:    ctx,
		params: params,
		done:   make(chan result, 1),
	}

	select {
	case m.jobs <- &r:
	case <-ctx.Done():
		return model.GeneratedImage{}, ctx.Err()
	case <-m.stop:
		return model.GeneratedImage{}, m.closedError()
	}

	select {
	case out := <-r.done:
		return generationResult(ctx, out)

	case <-ctx.Done():
		if r.cancel() {
			return model.GeneratedImage{}, ctx.Err()
		}
		return generationResult(ctx, <-r.done)

	case <-m.stop:
		if r.cancel() {
			return model.GeneratedImage{}, m.closedError()
		}
		return generationResult(ctx, <-r.done)
	}
}

func generationResult(ctx context.Context, out result) (model.GeneratedImage, error) {
	if out.err != nil {
		return out.image, out.err
	}
	if err := ctx.Err(); err != nil {
		return model.GeneratedImage{}, err
	}
	return out.image, nil
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
	return m.backend.Info()
}

// SystemInfo returns native library and host diagnostics.
func (m *Malina) SystemInfo() SystemDiagnostics {
	return systemDiagnostics()
}

// ActiveGenerations returns the number of admitted generation calls.
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
		m.backend.Stop()
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

func (m *Malina) worker() {
	defer close(m.done)

	for {
		select {
		case <-m.stop:
			m.finishUnload()
			return

		case r := <-m.jobs:
			if err := m.start(r); err != nil {
				r.done <- result{err: err}
				continue
			}

			image, err := m.backend.Generate(r.ctx, r.params)
			if errors.Is(err, context.Canceled) && r.ctx.Err() == nil {
				if closedErr := m.closedError(); closedErr != nil {
					err = closedErr
				}
			}
			if errors.Is(err, model.ErrNativeGeneration) {
				err = errors.Join(ErrPoisoned, err)
				m.poison()
			}

			r.done <- result{image: image, err: err}
			if errors.Is(err, ErrPoisoned) {
				m.finishUnload()
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
		m.backend.Stop()
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

func (m *Malina) finishUnload() {
	err := m.backend.Unload()
	m.mu.Lock()
	m.unloadErr = err
	m.mu.Unlock()
}
