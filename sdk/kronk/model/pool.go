package model

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// contextPool manages a pool of single-sequence llama contexts for parallel
// embedding/rerank operations. The contexts share the model weights but own
// independent context memory and compute buffers.
type contextPool struct {
	model     llama.Model
	ctxParams llama.ContextParams
	log       applog.Logger

	mu       sync.Mutex
	contexts []llama.Context
	memories []llama.Memory
	batches  []*extendedBatch
	avail    chan int // indices of available contexts
}

// newContextPool creates a pool of n llama contexts from the given model.
// Each context has its own KV cache but shares the model weights.
func newContextPool(ctx context.Context, model llama.Model, ctxParams llama.ContextParams, log applog.Logger, n int) (*contextPool, error) {
	if n < 1 {
		n = 1
	}
	ctxParams = contextPoolFallbackParams(ctxParams)

	p := &contextPool{
		model:     model,
		ctxParams: ctxParams,
		log:       log,
		contexts:  make([]llama.Context, n),
		memories:  make([]llama.Memory, n),
		batches:   make([]*extendedBatch, n),
		avail:     make(chan int, n),
	}

	for i := range n {
		lctx, err := llama.InitFromModel(model, ctxParams)
		if err != nil {
			return nil, errors.Join(err, p.freeContexts(i))
		}

		mem, err := llama.GetMemory(lctx)
		if err != nil {
			return nil, errors.Join(err, llama.Free(lctx), p.freeContexts(i))
		}

		llama.MemoryClear(mem, true)
		batch, err := newExtendedBatch(lctx)
		if err != nil {
			return nil, errors.Join(err, llama.Free(lctx), p.freeContexts(i))
		}

		p.contexts[i] = lctx
		p.memories[i] = mem
		p.batches[i] = batch
		p.avail <- i
	}

	log(ctx, "context-pool", "status", "initialized", "size", n)

	return p, nil
}

// contextPoolFallbackParams converts aggregate multi-sequence parameters into
// the single-sequence parameters used by each context in the fallback pool.
// Each independent context receives one sequence's share of NCtx and does not
// use multi-sequence KV memory.
func contextPoolFallbackParams(params llama.ContextParams) llama.ContextParams {
	if params.NSeqMax > 1 && params.NCtx > 0 {
		params.NCtx /= params.NSeqMax
	}
	if params.NOutputsMaxPerSeq > 0 {
		params.NOutputsMax = params.NOutputsMaxPerSeq
	}

	params.NSeqMax = 1
	params.KVUnified = 0

	return params
}

// poolContext represents an acquired context from the pool.
type poolContext struct {
	idx   int
	lctx  llama.Context
	mem   llama.Memory
	batch *extendedBatch
}

// acquire gets a context from the pool. Blocks until one is available or
// context is cancelled. Returns the context index for release.
func (p *contextPool) acquire(ctx context.Context) (poolContext, error) {
	select {
	case idx := <-p.avail:
		return poolContext{
			idx:   idx,
			lctx:  p.contexts[idx],
			mem:   p.memories[idx],
			batch: p.batches[idx],
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

// close frees all contexts in the pool.
func (p *contextPool) close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Drain the available channel.
	close(p.avail)
	for range p.avail {
	}

	return p.freeContexts(len(p.contexts))
}

func (p *contextPool) freeContexts(count int) error {
	errList := make([]error, 0, count*3)
	for i := range count {
		if p.batches[i] != nil {
			if err := p.batches[i].free(); err != nil {
				errList = append(errList, fmt.Errorf("free context pool batch %d: %w", i, err))
			}
			p.batches[i] = nil
		}

		lctx := p.contexts[i]
		if lctx == 0 {
			continue
		}
		if err := llama.Synchronize(lctx); err != nil {
			errList = append(errList, fmt.Errorf("synchronize context pool context %d: %w", i, err))
		}
		if err := llama.Free(lctx); err != nil {
			errList = append(errList, fmt.Errorf("free context pool context %d: %w", i, err))
		}
		p.contexts[i] = 0
	}

	return errors.Join(errList...)
}
