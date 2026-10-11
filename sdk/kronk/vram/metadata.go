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

	return buildFromMetadata(metadata, tensors, totalSize, cfg)
}

// buildFromMetadata resolves architecture-specific facts once for both local
// and remote inputs. Model profiles own architecture rules; Calculate owns math.
func buildFromMetadata(metadata map[string]string, tensors []gguf.TensorInfo, modelSizeBytes int64, cfg Config) (Result, error) {
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
	if recurrentStateCopies == 1 && att.NextNPredictLayers > 0 {
		recurrentStateCopies = max(cfg.EmbeddedMTPStateCopies, 1)
	}
	computeContexts := max(cfg.ComputeContexts, 1)
	if computeContexts == 1 && att.NextNPredictLayers > 0 && cfg.EmbeddedMTPStateCopies > 1 {
		computeContexts = 2
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
