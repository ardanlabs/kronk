package mtp

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	yzmaspec "github.com/hybridgroup/yzma/exp/speculative"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// LoadRequest supplies borrowed target handles and model-layer policy. A
// successful LoadCompanion transfers model ownership to Resources, including on
// subsequent load failures. NewSampler transfers sampler ownership; its token
// slice stays alive with Resources. Neither callback may retain Resources.
type LoadRequest struct {
	TargetContext llama.Context
	TargetModel   llama.Model
	TargetParams  llama.ContextParams
	NDraft        int
	Configured    bool
	DrafterFile   string
	LoadCompanion func(sharedKV bool) (llama.Model, error)
	NewSampler    func(llama.Vocab) (llama.Sampler, []llama.Token)
}

// Load creates a supported backend's native resources. Missing embedded weights
// or unavailable NextN APIs preserve the existing logged target-only fallback.
func (b Backend) Load(ctx context.Context, log applog.Logger, req LoadRequest) (*Resources, error) {
	event := "draft-model-mtp"
	prefix := "init-mtp"
	nLayers := 0
	switch {
	case b.sharedKV:
		event, prefix = "draft-model-mtp-shared", "mtp-shared-init"
	case b.artifact == speculation.MTPArtifactCompanion:
		event, prefix = "draft-model-mtp-separate", "mtp-separate-init"
	default:
		nLayers = nextNLayers(req.TargetModel)
		if nLayers == 0 {
			log(ctx, event, "status", "auto-detect-skipped", "backend", b.Name(), "reason", "no nextn_predict_layers metadata in target GGUF")
			return nil, nil
		}
	}
	if !yzmaspec.Available() {
		reason := "target GGUF declares MTP (nextn_predict_layers>0) but Yzma could not resolve the required NextN hidden-state APIs in the loaded llama library. MTP speculative decoding is DISABLED for this model. Check that the loaded llama.cpp libraries and Yzma version are compatible."
		switch {
		case b.sharedKV:
			reason = "DrafterFile is a gemma4-assistant MTP head but Yzma could not resolve the required NextN hidden-state APIs in the loaded llama library. MTP speculative decoding is DISABLED for this model. Check that the loaded llama.cpp libraries and Yzma version are compatible."
		case b.artifact == speculation.MTPArtifactCompanion:
			reason = "DrafterFile is a Qwen own-KV MTP head but Yzma could not resolve the required NextN hidden-state APIs in the loaded llama library. MTP speculative decoding is DISABLED for this model. Check that the loaded llama.cpp libraries and Yzma version are compatible."
		}
		if b.artifact == speculation.MTPArtifactEmbedded {
			log(ctx, event, "status", "DISABLED", "backend", b.Name(), "nextn-layers", nLayers, "reason", reason)
		} else {
			log(ctx, event, "status", "DISABLED", "backend", b.Name(), "reason", reason)
		}
		return nil, nil
	}

	r := &Resources{Model: req.TargetModel, Vocab: llama.ModelGetVocab(req.TargetModel), target: req.TargetContext}
	loaded := false
	defer func() {
		if !loaded {
			r.Free()
		}
	}()
	if b.artifact == speculation.MTPArtifactCompanion {
		model, err := req.LoadCompanion(b.sharedKV)
		if err != nil {
			return nil, err
		}
		r.Model, r.ownsModel = model, true
	}

	nEmbd, err := b.validateDimensions(req.TargetModel, r.Model)
	if err != nil {
		return nil, err
	}
	r.embeddingSize = nEmbd
	params := ownKVContextParams(llama.ContextDefaultParams(), req.TargetParams)
	if b.sharedKV {
		params = sharedContextParams(llama.ContextDefaultParams(), req.TargetParams, req.TargetContext)
	}
	if b.artifact == speculation.MTPArtifactEmbedded || b.sharedKV {
		status := "loading"
		if b.sharedKV {
			status = "init-context"
		}
		log(ctx, event, "status", status, "nDraft", req.NDraft, "nEmbd", nEmbd,
			"nCtx", params.NCtx, "nBatch", params.NBatch, "nUbatch", params.NUbatch,
			"nSeqMax", params.NSeqMax, "nRsSeq", params.NRsSeq,
			"nOutputsMax", params.NOutputsMax, "nOutputsMaxPerSeq", params.NOutputsMaxPerSeq)
	}
	r.Context, err = llama.InitFromModel(r.Model, params)
	if err != nil {
		return nil, fmt.Errorf("%s-context: %w", prefix, err)
	}
	r.Memory, err = llama.GetMemory(r.Context)
	if err != nil {
		memoryPrefix := "get-mtp"
		if b.artifact == speculation.MTPArtifactCompanion {
			memoryPrefix = "mtp-separate-get"
			if b.sharedKV {
				memoryPrefix = "mtp-shared-get"
			}
		}
		return nil, fmt.Errorf("%s-memory: %w", memoryPrefix, err)
	}
	// A shared assistant aliases TARGET memory. Never clear it here.
	if !b.sharedKV {
		llama.MemoryClear(r.Memory, true)
		r.mirrorCapacity = int(params.NBatch)
	}
	// Set before graph construction: dense target rows, masked draft outputs.
	yzmaspec.SetEmbeddingsNextN(req.TargetContext, true, false)
	yzmaspec.SetEmbeddingsNextN(r.Context, true, true)
	r.Sampler, r.SuppressTokens = req.NewSampler(llama.ModelGetVocab(r.Model))
	if err := r.initBatches(); err != nil {
		return nil, fmt.Errorf("%s-batches: %w", prefix, err)
	}
	if !b.sharedKV {
		r.enableDeviceRows(ctx, log)
	}
	source := "mtp-drafter-file"
	if b.artifact == speculation.MTPArtifactEmbedded {
		source = "auto-detected"
		if req.Configured {
			source = "auto-detected-configured"
		}
	}
	if b.artifact == speculation.MTPArtifactEmbedded {
		log(ctx, event, "status", "loaded", "backend", b.Name(), "source", source,
			"nDraft", req.NDraft, "nextn-layers", nLayers, "nEmbd", nEmbd, "nCtx", llama.NCtx(r.Context))
	} else {
		log(ctx, event, "status", "loaded", "backend", b.Name(), "source", source,
			"file", req.DrafterFile, "nDraft", req.NDraft, "nEmbd", nEmbd, "nCtx", llama.NCtx(r.Context))
	}
	loaded = true
	return r, nil
}

func (b Backend) validateDimensions(target, draft llama.Model) (int, error) {
	if b.artifact == speculation.MTPArtifactEmbedded {
		nEmbd := b.embeddingWidth(target)
		if nEmbd <= 0 {
			return 0, fmt.Errorf("invalid MTP embedding width %d from target model", nEmbd)
		}
		return nEmbd, nil
	}
	if b.sharedKV {
		nEmbd, nEmbdOut := int(llama.ModelNEmbd(target)), int(llama.ModelNEmbdOut(draft))
		if nEmbd <= 0 {
			return 0, fmt.Errorf("mtp-shared: invalid target nEmbd %d", nEmbd)
		}
		if nEmbdOut != nEmbd {
			return 0, fmt.Errorf("mtp-shared: assistant n_embd_out %d != target n_embd %d", nEmbdOut, nEmbd)
		}
		return nEmbd, nil
	}
	targetTokens := llama.VocabNTokens(llama.ModelGetVocab(target))
	draftTokens := llama.VocabNTokens(llama.ModelGetVocab(draft))
	if targetTokens != draftTokens {
		return 0, fmt.Errorf("mtp-separate vocabulary mismatch: target has %d tokens, draft has %d tokens", targetTokens, draftTokens)
	}
	nEmbd, draftNEmbd := int(llama.ModelNEmbdOut(target)), int(llama.ModelNEmbdOut(draft))
	if nEmbd <= 0 || draftNEmbd != nEmbd {
		return 0, fmt.Errorf("mtp-separate output embedding width %d does not match target output embedding width %d", draftNEmbd, nEmbd)
	}
	return nEmbd, nil
}

// nextNLayers reads the architecture-independent native metadata signal.
func nextNLayers(model llama.Model) int {
	for i := range llama.ModelMetaCount(model) {
		key, ok := llama.ModelMetaKeyByIndex(model, i)
		if !ok || !strings.Contains(key, "nextn_predict_layers") {
			continue
		}
		raw, ok := llama.ModelMetaValStrByIndex(model, i)
		if !ok {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || n < 0 {
			return 0
		}
		return n
	}
	return 0
}

// Only own-KV single-head models can borrow device rows. Unsupported symbols,
// allocations, events, placement or widths leave host synchronization intact.
func (r *Resources) enableDeviceRows(ctx context.Context, log applog.Logger) {
	if !yzmaspec.DeviceRowsAvailable() || nextNLayers(r.Model) != 1 {
		return
	}
	if !yzmaspec.SetEmbeddingsNextNDev(r.target, true) {
		return
	}
	if !yzmaspec.SetCtxStateSrc(r.Context, r.target) {
		yzmaspec.SetCtxStateSrc(r.Context, 0)
		yzmaspec.SetEmbeddingsNextNDev(r.target, false)
		return
	}
	r.stateSource = r.target
	log(ctx, "draft-model-mtp", "status", "device-state-rows-enabled")
}
