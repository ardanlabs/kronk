package mtp

import (
	"fmt"

	yzmaspec "github.com/hybridgroup/yzma/exp/speculative"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// Resources owns an MTP context, sampler, batches, device link, and any loaded
// companion model. Embedded target models and the target context are borrowed.
// Exported handles are borrowed views for model-engine cache integration; callers
// must not free them. Free is the sole teardown path, including failed loads.
type Resources struct {
	Model          llama.Model
	Vocab          llama.Vocab
	Context        llama.Context
	Memory         llama.Memory
	Sampler        llama.Sampler
	SuppressTokens []llama.Token
	DraftBatch     llama.BatchExt
	MirrorBatch    llama.BatchExt
	DraftHidden    []float32
	MirrorHidden   []float32
	target         llama.Context
	stateSource    llama.Context
	ownsModel      bool
	embeddingSize  int
	mirrorCapacity int
}

// initBatches allocates token+embedding batches. Shared-KV MTP does not replay
// target rows and therefore has no mirror batch. Free handles partial failures.
func (r *Resources) initBatches() error {
	var err error
	r.DraftBatch, err = llama.BatchExtInit(r.Context)
	if err != nil {
		return fmt.Errorf("initializing MTP draft batch: %w", err)
	}
	r.DraftHidden = make([]float32, r.embeddingSize)
	if r.mirrorCapacity > 0 {
		r.MirrorBatch, err = llama.BatchExtInit(r.Context)
		if err != nil {
			return fmt.Errorf("initializing MTP mirror batch: %w", err)
		}
		r.MirrorHidden = make([]float32, r.mirrorCapacity*r.embeddingSize)
	}
	return nil
}

// EmbeddingSize returns the width of one pre-norm hidden row.
func (r *Resources) EmbeddingSize() int { return r.embeddingSize }

// MirrorCapacity returns the maximum rows in one own-KV synchronization chunk.
func (r *Resources) MirrorCapacity() int {
	return r.mirrorCapacity
}

// Free detaches borrowed device rows before releasing owned native resources.
// It never clears or frees the target's memory or model.
func (r *Resources) Free() {
	if r.stateSource != 0 {
		llama.Synchronize(r.Context)
		yzmaspec.SetCtxStateSrc(r.Context, 0)
		yzmaspec.SetEmbeddingsNextNDev(r.stateSource, false)
	}
	if r.Sampler != 0 {
		llama.SamplerFree(r.Sampler)
	}
	if r.DraftBatch != 0 {
		llama.BatchExtFree(r.DraftBatch)
	}
	if r.MirrorBatch != 0 {
		llama.BatchExtFree(r.MirrorBatch)
	}
	if r.Context != 0 {
		llama.Free(r.Context)
	}
	if r.ownsModel && r.Model != 0 {
		llama.ModelFree(r.Model)
	}
	*r = Resources{}
}
