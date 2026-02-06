package model

import (
	"context"
	"sync"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// contextPool manages a pool of llama contexts for parallel embedding/rerank
// operations. All contexts share the same underlying model weights, so only
// the KV cache memory is multiplied per context.
type contextPool struct {
	model     llama.Model
	ctxParams llama.ContextParams
	log       Logger

	mu       sync.Mutex
	contexts []llama.Context
	memories []llama.Memory
	avail    chan int // indices of available contexts
}

// newContextPool creates a pool of n llama contexts from the given model.
// Each context has its own KV cache but shares the model weights.
func newContextPool(ctx context.Context, model llama.Model, ctxParams llama.ContextParams, log Logger, n int) (*contextPool, error) {
	if n < 1 {
		n = 1
	}

	p := &contextPool{
		model:     model,
		ctxParams: ctxParams,
		log:       log,
		contexts:  make([]llama.Context, n),
		memories:  make([]llama.Memory, n),
		avail:     make(chan int, n),
	}

	for i := range n {
		lctx, err := llama.InitFromModel(model, ctxParams)
		if err != nil {
			// Clean up any contexts we already created.
			for j := range i {
				llama.Synchronize(p.contexts[j])
				llama.Free(p.contexts[j])
			}
			return nil, err
		}

		mem, err := llama.GetMemory(lctx)
		if err != nil {
			llama.Free(lctx)
			for j := range i {
				llama.Synchronize(p.contexts[j])
				llama.Free(p.contexts[j])
			}
			return nil, err
		}

		// Clear KV cache to ensure clean state.
		llama.MemoryClear(mem, true)

		p.contexts[i] = lctx
		p.memories[i] = mem
		p.avail <- i
	}

	log(ctx, "context-pool", "status", "initialized", "size", n)

	return p, nil
}

// poolContext represents an acquired context from the pool.
type poolContext struct {
	idx  int
	lctx llama.Context
	mem  llama.Memory
}

// acquire gets a context from the pool. Blocks until one is available or
// context is cancelled. Returns the context index for release.
func (p *contextPool) acquire(ctx context.Context) (poolContext, error) {
	select {
	case idx := <-p.avail:
		return poolContext{
			idx:  idx,
			lctx: p.contexts[idx],
			mem:  p.memories[idx],
		}, nil

	case <-ctx.Done():
		return poolContext{}, ctx.Err()
	}
}

// release returns a context to the pool.
func (p *contextPool) release(pc poolContext) {
	// Clear KV cache for next use.
	llama.MemoryClear(pc.mem, true)
	p.avail <- pc.idx
}

// size returns the number of contexts in the pool.
func (p *contextPool) size() int {
	return len(p.contexts)
}

// close frees all contexts in the pool.
func (p *contextPool) close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Drain the available channel.
	close(p.avail)
	for range p.avail {
	}

	// Free all contexts.
	for i, lctx := range p.contexts {
		if lctx != 0 {
			llama.Synchronize(lctx)
			llama.Free(lctx)
			p.contexts[i] = 0
		}
	}
}
