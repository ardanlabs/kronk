package mtp

import (
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// Resources owns the llama batches and hidden-state buffers used by an
// MTP backend. Context/model lifetime remains with the model loader.
type Resources struct {
	DraftBatch     llama.BatchExt
	MirrorBatch    llama.BatchExt
	DraftHidden    []float32
	MirrorHidden   []float32
	embeddingSize  int
	mirrorCapacity int
}

// NewResources allocates MTP token+embedding batches. A zero mirror capacity
// selects a shared-KV backend, which does not replay target rows.
func NewResources(ctx llama.Context, mirrorCapacity, embeddingSize int) (*Resources, error) {
	draftBatch, err := llama.BatchExtInit(ctx)
	if err != nil {
		return nil, fmt.Errorf("initializing MTP draft batch: %w", err)
	}

	r := &Resources{
		DraftBatch:     draftBatch,
		DraftHidden:    make([]float32, embeddingSize),
		embeddingSize:  embeddingSize,
		mirrorCapacity: mirrorCapacity,
	}
	if mirrorCapacity > 0 {
		r.MirrorBatch, err = llama.BatchExtInit(ctx)
		if err != nil {
			llama.BatchExtFree(r.DraftBatch)
			return nil, fmt.Errorf("initializing MTP mirror batch: %w", err)
		}
		r.MirrorHidden = make([]float32, mirrorCapacity*embeddingSize)
	}

	return r, nil
}

// EmbeddingSize returns the width of one pre-norm hidden row.
func (r *Resources) EmbeddingSize() int { return r.embeddingSize }

// MirrorCapacity returns the maximum rows in one own-KV synchronization chunk.
func (r *Resources) MirrorCapacity() int {
	return r.mirrorCapacity
}

// Free releases the extended batches.
func (r *Resources) Free() {
	if r.DraftBatch != 0 {
		llama.BatchExtFree(r.DraftBatch)
	}
	if r.MirrorBatch != 0 {
		llama.BatchExtFree(r.MirrorBatch)
	}
}
