package dspark

import (
	"errors"
	"fmt"
	"slices"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	yzmaspec "github.com/hybridgroup/yzma/exp/speculative"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// LoadRequest borrows the target and delegates file/placement policy to the
// model layer. A successful LoadCompanion transfers model ownership to Load,
// including cleanup on subsequent failure. No target handle is transferred.
type LoadRequest struct {
	TargetContext  llama.Context
	TargetModel    llama.Model
	TargetParams   llama.ContextParams
	TargetMetadata map[string]string
	DraftMetadata  map[string]string
	DraftTensors   []gguf.TensorInfo
	NDraft         int
	LoadCompanion  func() (llama.Model, error)
}

// Load creates the qualified Markov-DFlash resource owner. It does not enable
// proposal generation. Native model loading validates remaining tensor shapes;
// this boundary rejects other DFlash variants before constructing their graph.
func Load(req LoadRequest) (_ *Resources, err error) {
	if req.TargetContext == 0 || req.TargetModel == 0 {
		return nil, fmt.Errorf("DSpark load: missing target handles")
	}
	if !yzmaspec.Available() {
		return nil, fmt.Errorf("DSpark load: NextN APIs unavailable; check the llama library and Yzma versions")
	}
	if _, err := ProposalLimit(req.TargetMetadata, req.DraftMetadata, req.NDraft); err != nil {
		return nil, err
	}
	if err := validateTensors(req.DraftTensors); err != nil {
		return nil, err
	}
	r := Resources{target: req.TargetContext}
	loaded := false
	defer func() {
		if !loaded {
			err = errors.Join(err, r.Free())
		}
	}()
	r.Model, err = req.LoadCompanion()
	if err != nil {
		return nil, err
	}
	r.width = int(llama.ModelNEmbd(req.TargetModel))
	layers := llama.ModelTargetLayerIDs(r.Model)
	if r.width <= 0 || int(llama.ModelNEmbd(r.Model)) != r.width || len(layers) == 0 {
		return nil, fmt.Errorf("DSpark load: incompatible native embedding widths or missing extraction layers")
	}
	metadataLayers, err := gguf.ParseInt64OrArray(req.DraftMetadata, "dflash.target_layers")
	if err != nil || len(layers) != len(metadataLayers) {
		return nil, fmt.Errorf("DSpark load: native extraction layers differ from metadata")
	}
	for i, layer := range layers {
		if int64(layer) != metadataLayers[i] || layer < 0 || layer > llama.ModelNLayer(req.TargetModel) {
			return nil, fmt.Errorf("DSpark load: invalid native extraction layer %d", layer)
		}
	}
	if err := validateVocab(llama.ModelGetVocab(req.TargetModel), llama.ModelGetVocab(r.Model)); err != nil {
		return nil, err
	}
	block, err := gguf.ParseInt64(req.DraftMetadata, "dflash.block_size")
	if err != nil {
		return nil, err
	}
	params := contextParams(req.TargetParams, req.TargetContext, uint32(block))
	r.Context, err = llama.InitFromModel(r.Model, params)
	if err != nil {
		return nil, fmt.Errorf("DSpark context: %w", err)
	}
	r.Memory, err = llama.GetMemory(r.Context)
	if err != nil {
		return nil, fmt.Errorf("DSpark memory: %w", err)
	}
	llama.MemoryClear(r.Memory, true)
	llama.SetCausalAttn(r.Context, false)
	yzmaspec.SetEmbeddingsNextN(r.Context, true, false)
	r.batch, err = llama.BatchExtInit(r.Context)
	if err != nil {
		return nil, fmt.Errorf("DSpark injection batch: %w", err)
	}
	r.chunk = int(llama.NUBatch(r.Context))
	r.features = make([]float32, int(params.NBatch)*r.width*len(layers))
	r.sequences = make([]sequence, int(params.NSeqMax))
	for i := range r.sequences {
		r.sequences[i] = newSequence(int(block), r.width)
	}
	for _, layer := range layers {
		if err := llama.SetEmbeddingsLayerInp(r.target, uint32(layer), true); err != nil {
			return nil, fmt.Errorf("DSpark register target layer %d: %w", layer, err)
		}
		r.layers = append(r.layers, layer)
	}
	loaded = true
	return &r, nil
}

func contextParams(target llama.ContextParams, targetCtx llama.Context, block uint32) llama.ContextParams {
	params := target
	params.CtxType = llama.ContextTypeDefault
	params.CtxOther = targetCtx
	params.NRsSeq = 0
	params.NOutputsMax = min(target.NBatch, block*target.NSeqMax)
	params.NOutputsMaxPerSeq = min(target.NBatch, block)
	params.Embeddings = 0
	params.Samplers, params.NSamplers = 0, 0
	params.MoeCacheSize = 0
	return params
}

func validateVocab(target, draft llama.Vocab) error {
	n := llama.VocabNTokens(target)
	if n <= 0 || llama.VocabNTokens(draft) != n {
		return fmt.Errorf("DSpark load: target/sidecar public vocabulary sizes differ")
	}
	mask := llama.VocabMASK(draft)
	if mask < 0 || int32(mask) >= n {
		return fmt.Errorf("DSpark load: mask token %d outside public vocabulary", mask)
	}
	for token := range n {
		if llama.VocabGetText(target, llama.Token(token)) != llama.VocabGetText(draft, llama.Token(token)) {
			return fmt.Errorf("DSpark load: public vocabulary token %d differs from target", token)
		}
	}
	return nil
}

func validateTensors(tensors []gguf.TensorInfo) error {
	for _, name := range []string{"markov_w1.weight", "markov_w2.weight", "markov_ctx_mix.weight", "markov_ctx_norm.weight"} {
		if !slices.ContainsFunc(tensors, func(t gguf.TensorInfo) bool { return t.Name == name }) {
			return fmt.Errorf("DSpark load: missing required Markov tensor %q", name)
		}
	}
	for _, name := range []string{"d2t", "selector_hidden.weight", "conf_proj.weight"} {
		if slices.ContainsFunc(tensors, func(t gguf.TensorInfo) bool { return t.Name == name }) {
			return fmt.Errorf("DSpark load: unqualified graph tensor %q", name)
		}
	}
	return nil
}
