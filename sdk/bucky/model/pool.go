package model

import (
	"context"
	"fmt"
	"sync"

	"github.com/ardanlabs/bucky/pkg/whisper"
	"github.com/ardanlabs/kronk/sdk/applog"
)

// statePool manages a pool of whisper.State instances for parallel
// transcribe and language-detect operations against a shared
// whisper.Context. The model weights live once in the context; each
// state owns its own mel spectrogram, KV cache, and compute buffer,
// so two acquires can decode independent audio in parallel.
//
// statePool is the bucky counterpart to sdk/kronk/model.contextPool
// (which pools llama.Context instances for embed/rerank parallelism).
type statePool struct {
	handle whisper.Context
	log    applog.Logger

	mu     sync.Mutex
	states []whisper.State
	avail  chan int
}

// newStatePool creates a pool of n whisper.State instances against
// the supplied whisper.Context. All states share the context's model
// weights but each carries its own decode-time scratch.
func newStatePool(ctx context.Context, handle whisper.Context, log applog.Logger, n int) (*statePool, error) {
	if n < 1 {
		n = 1
	}

	p := statePool{
		handle: handle,
		log:    log,
		states: make([]whisper.State, n),
		avail:  make(chan int, n),
	}

	for i := range n {
		state, err := whisper.InitState(handle)
		if err != nil {
			for j := range i {
				whisper.FreeState(p.states[j])
			}
			return nil, fmt.Errorf("new-state-pool: init-state[%d]: %w", i, err)
		}

		p.states[i] = state
		p.avail <- i
	}

	log(ctx, "state-pool", "status", "initialized", "size", n)

	return &p, nil
}

// poolState represents an acquired whisper.State plus its pool index.
// The index is opaque to callers; release uses it to return the slot.
type poolState struct {
	idx   int
	state whisper.State
}

// acquire blocks until a state is available or ctx is cancelled.
func (p *statePool) acquire(ctx context.Context) (poolState, error) {
	select {
	case idx := <-p.avail:
		return poolState{
			idx:   idx,
			state: p.states[idx],
		}, nil

	case <-ctx.Done():
		return poolState{}, ctx.Err()
	}
}

// release returns a state to the pool. whisper_full_with_state
// resets the state internally on the next call, so there is no
// explicit clear here.
func (p *statePool) release(ps poolState) {
	p.avail <- ps.idx
}

// close frees every whisper.State in the pool. After close the pool
// must not be used.
func (p *statePool) close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	close(p.avail)
	for range p.avail {
	}

	for i, state := range p.states {
		if state != 0 {
			whisper.FreeState(state)
			p.states[i] = 0
		}
	}
}
