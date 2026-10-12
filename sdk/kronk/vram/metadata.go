package vram

import (
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// FromBytes computes the VRAM requirements directly from already-fetched
// GGUF header bytes (typically the first gguf.HeaderFetchSize bytes from
// the catalog cache or a local file). totalSize is the on-disk size of
// all model files combined.
func FromBytes(data []byte, totalSize int64, cfg Config) (Result, error) {
	metadata, tensors, err := gguf.ParseHeaderAndTensors(data, totalSize)
	if err != nil {
		return Result{}, fmt.Errorf("from-bytes: %w", err)
	}

	return buildFromMetadata(metadata, [][]gguf.TensorInfo{tensors}, totalSize, cfg)
}

// buildFromMetadata resolves architecture-specific facts once for both local
// and remote inputs. Model profiles own architecture rules; Calculate owns math.
func buildFromMetadata(metadata map[string]string, shards [][]gguf.TensorInfo, modelSizeBytes int64, cfg Config) (Result, error) {
	profile := modelprofile.Resolve(metadata)
	if err := profile.ValidateForVRAM(); err != nil {
		return Result{}, fmt.Errorf("build-from-metadata: %w", err)
	}

	if profile.Role == modelprofile.RoleVisionEncoder {
		// A projector also reserves a compute arena. Retain the common
		// small-model floor; image-dependent peaks remain approximate.
		compute := EstimateComputeBuffer(Input{ModelSizeBytes: modelSizeBytes})
		return Result{
			Input:            Input{ModelSizeBytes: modelSizeBytes},
			TotalVRAM:        modelSizeBytes + compute,
			ModelWeightsGPU:  modelSizeBytes,
			ComputeBufferEst: compute,
		}, nil
	}

	blockCount := profile.Dimensions.BlockCount
	headCountKV := profile.Dimensions.HeadCountKV
	keyLength := profile.Dimensions.KeyLength
	valueLength := profile.Dimensions.ValueLength
	embeddingLength := profile.Dimensions.EmbeddingLength
	moeInfo := profile.MoE
	var moePtr *gguf.MoEInfo
	if moeInfo.IsMoE {
		moePtr = &moeInfo
	}

	var tensors []gguf.TensorInfo
	for _, shard := range shards {
		tensors = append(tensors, shard...)
	}
	var weights *gguf.WeightBreakdown
	if len(tensors) > 0 {
		wb := gguf.CategorizeWeights(tensors, blockCount)
		weights = &wb
	}

	att := profile.Attention
	if att.FullHeadCountKV > 0 {
		headCountKV = att.FullHeadCountKV
	}
	if att.FullKeyLength > 0 {
		keyLength = att.FullKeyLength
	}
	if att.FullValueLength > 0 {
		valueLength = att.FullValueLength
	}
	recurrentStateCopies := max(cfg.RecurrentStateCopies, 1)
	// Embedded NextN weights alone do not imply a loaded draft context.
	// Use the same runtime activation gate as model loading and planning.
	embeddedMTP := att.NextNPredictLayers > 0 && modelprofile.MTPEnabled(profile.Speculation.MTPArchitecture)
	if recurrentStateCopies == 1 && embeddedMTP {
		recurrentStateCopies = max(cfg.EmbeddedMTPStateCopies, 1)
	}
	computeContexts := max(cfg.ComputeContexts, 1)
	if computeContexts == 1 && embeddedMTP && cfg.EmbeddedMTPStateCopies > 1 {
		computeContexts = 2
	}
	outputRows := cfg.OutputRows
	if outputRows <= 0 && profile.Purpose == modelprofile.PurposeGeneration {
		switch profile.ComputeKind {
		case modelprofile.ComputeQwen35, modelprofile.ComputeGPTOSS,
			modelprofile.ComputeMistral3, modelprofile.ComputeLFM2,
			modelprofile.ComputeGLM4MoE, modelprofile.ComputeGemma3, modelprofile.ComputeLlama:
			// Generation caps logits at one current row plus NDraft rows
			// per slot, matching current/rollback copies from callers.
			// Own-KV MTP emits only one row per slot.
			outputRows = max(cfg.Slots, 1) * recurrentStateCopies
			if cfg.MTPContext {
				outputRows = max(cfg.Slots, 1)
			}
		}
	}
	vocabularySize := profile.Dimensions.VocabularySize
	if vocabularySize <= 0 {
		// GGUF commonly omits vocab_size. Tensor dimensions retain the
		// exact count even when token strings contain spaces or delimiters.
		for _, tensor := range tensors {
			if (tensor.Name == "token_embd.weight" || tensor.Name == "output.weight") && len(tensor.Dims) == 2 {
				vocabularySize = max(vocabularySize, tensor.Dims[1])
			}
		}
	}

	input := Input{
		WeightLoading:         cfg.WeightLoading,
		LazyTable:             LazyTableForShards(profile, shards),
		ComputeBackend:        cfg.ComputeBackend,
		ComputeKind:           profile.ComputeKind,
		VocabularySize:        vocabularySize,
		ModelSizeBytes:        modelSizeBytes,
		ContextWindow:         cfg.ContextWindow,
		BlockCount:            blockCount,
		HeadCount:             profile.Dimensions.HeadCount,
		HeadCountKV:           headCountKV,
		KeyLength:             keyLength,
		ValueLength:           valueLength,
		SWAHeadCountKV:        att.SWAHeadCountKV,
		SWAKeyLength:          att.SWAKeyLength,
		SWAValueLength:        att.SWAValueLength,
		HeadCountKVByLayer:    att.HeadCountKV,
		BytesPerElement:       cfg.BytesPerElement,
		TypeK:                 cfg.TypeK,
		TypeV:                 cfg.TypeV,
		Slots:                 cfg.Slots,
		SlidingWindow:         att.SlidingWindow,
		SlidingWindowLayers:   att.SlidingWindowLayers,
		SharedKVLayers:        att.SharedKVLayers,
		SWAPattern:            att.SWAPattern,
		RecurrentPattern:      att.RecurrentPattern,
		NextNPredictLayers:    att.NextNPredictLayers,
		MTPContext:            cfg.MTPContext,
		IndexerKeyLength:      att.IndexerKeyLength,
		IndexerCompression:    att.IndexerCompression,
		IndexerTopK:           att.IndexerTopK,
		RecurrentStateBytes:   att.RecurrentStateBytes,
		RecurrentStateCopies:  recurrentStateCopies,
		NUBatch:               cfg.NUBatch,
		OutputRows:            outputRows,
		KVUnified:             false,
		EmbeddingLength:       embeddingLength,
		OutputEmbeddingLength: profile.Dimensions.OutputEmbeddingLength,
		ComputeContexts:       computeContexts,
		MoE:                   moePtr,
		Weights:               weights,
		TiedOutputFallback:    profile.TiedOutputFallback,
		GPULayers:             cfg.GPULayers,
		ExpertLayersOnGPU:     cfg.ExpertLayersOnGPU,
		MoECacheSize:          cfg.MoECacheSize,
		KVCacheOnCPU:          cfg.KVCacheOnCPU,
		SWAFull:               cfg.SWAFull,
		VTransposed:           cfg.VTransposed,
	}

	return Calculate(input), nil
}
