package mtp

import (
	"runtime"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// Resources owns the llama batches and pinned hidden-state buffers used by an
// MTP backend. Context/model lifetime remains with the model loader.
type Resources struct {
	DraftBatch    llama.Batch
	MirrorBatch   llama.Batch
	DraftHidden   []float32
	MirrorHidden  []float32
	embeddingSize int
	draftPin      runtime.Pinner
	mirrorPin     runtime.Pinner
}

// NewResources allocates MTP token+embedding batches. A zero mirror capacity
// selects a shared-KV backend, which does not replay target rows.
func NewResources(mirrorCapacity, embeddingSize int) *Resources {
	r := &Resources{
		DraftBatch:    llama.BatchInit(1, 0, 1),
		DraftHidden:   make([]float32, embeddingSize),
		embeddingSize: embeddingSize,
	}
	if mirrorCapacity > 0 {
		r.MirrorBatch = llama.BatchInit(int32(mirrorCapacity), 0, 1)
		r.MirrorHidden = make([]float32, mirrorCapacity*embeddingSize)
	}
	if len(r.DraftHidden) > 0 {
		r.draftPin.Pin(&r.DraftHidden[0])
		r.DraftBatch.Embd = (*float32)(unsafe.Pointer(&r.DraftHidden[0]))
	}
	if len(r.MirrorHidden) > 0 {
		r.mirrorPin.Pin(&r.MirrorHidden[0])
		r.MirrorBatch.Embd = (*float32)(unsafe.Pointer(&r.MirrorHidden[0]))
	}
	return r
}

// EmbeddingSize returns the width of one pre-norm hidden row.
func (r *Resources) EmbeddingSize() int { return r.embeddingSize }

// MirrorCapacity returns the maximum rows in one own-KV synchronization chunk.
func (r *Resources) MirrorCapacity() int {
	if r.embeddingSize == 0 {
		return 0
	}
	return len(r.MirrorHidden) / r.embeddingSize
}

// Free releases batches after detaching their Go-owned embedding buffers.
func (r *Resources) Free() {
	r.DraftBatch.Embd = nil
	r.MirrorBatch.Embd = nil
	llama.BatchFree(r.DraftBatch)
	llama.BatchFree(r.MirrorBatch)
	r.draftPin.Unpin()
	r.mirrorPin.Unpin()
}
