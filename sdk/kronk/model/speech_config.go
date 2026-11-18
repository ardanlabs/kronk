package model

import (
	"fmt"
	"math"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speech"
)

// ResolveSpeechConfig resolves the supported speech resource limits for planning
// and loading a model already classified as speech. It does not select a runtime
// or apply chat defaults. Incompatible explicit settings return ErrInvalidRequest.
func ResolveSpeechConfig(cfg Config) (Config, error) {
	switch {
	case cfg.SpeechMaxFrames() < 1 || cfg.SpeechMaxFrames() > 512:
		return cfg, fmt.Errorf("%w: speech max frames must be between 1 and 512", ErrInvalidRequest)
	case cfg.ContextWindow() != 0 && cfg.ContextWindow() != speech.AutoregressiveContextWindow:
		return cfg, fmt.Errorf("%w: speech context window must be %d per sequence", ErrInvalidRequest, speech.AutoregressiveContextWindow)
	case cfg.PtrPrefillBatchSize != nil && cfg.PrefillBatchSize() != speech.AutoregressiveBatchSize:
		return cfg, fmt.Errorf("%w: speech prefill batch size must be %d", ErrInvalidRequest, speech.AutoregressiveBatchSize)
	case cfg.PtrPhysicalBatchSize != nil && cfg.PhysicalBatchSize() != speech.AutoregressiveBatchSize:
		return cfg, fmt.Errorf("%w: speech physical batch size must be %d", ErrInvalidRequest, speech.AutoregressiveBatchSize)
	case cfg.KVCacheMode() != KVCachePartitioned:
		return cfg, fmt.Errorf("%w: speech requires partitioned KV cache", ErrInvalidRequest)
	case cfg.IncrementalCache() || cfg.IMCSessionCapacity() > 0:
		return cfg, fmt.Errorf("%w: speech does not support IMC", ErrInvalidRequest)
	case cfg.PtrDraftModel != nil || cfg.MTPDrafterFile != "" ||
		(cfg.SpeculationMode() != SpeculationAuto && cfg.SpeculationMode() != SpeculationDisabled):
		return cfg, fmt.Errorf("%w: speech does not support speculative decoding", ErrInvalidRequest)
	case len(cfg.Adapters) > 0:
		return cfg, fmt.Errorf("%w: speech does not support adapters", ErrInvalidRequest)
	}

	for _, cacheType := range []GGMLType{cfg.CacheTypeK, cfg.CacheTypeV} {
		if !cacheType.IsZero() && cacheType != GGMLTypeAuto && cacheType != GGMLTypeF16 {
			return cfg, fmt.Errorf("%w: speech requires F16 KV cache, got %s", ErrInvalidRequest, cacheType)
		}
	}

	nSlots := cfg.NSeqMax()
	if nSlots <= 0 {
		nSlots = defNSeqMax
	}

	if nSlots > math.MaxUint32/speech.AutoregressiveContextWindow {
		return cfg, fmt.Errorf("%w: speech aggregate context exceeds native capacity", ErrInvalidRequest)
	}

	cfg = adjustQueueDepth(cfg)
	cfg = adjustAdmissionTimeout(cfg)
	cfg = adjustThreads(cfg)
	cfg.PtrNSeqMax = new(nSlots)
	cfg.PtrContextWindow = new(speech.AutoregressiveContextWindow)
	cfg.PtrPrefillBatchSize = new(speech.AutoregressiveBatchSize)
	cfg.PtrPhysicalBatchSize = new(speech.AutoregressiveBatchSize)
	cfg.PtrIncrementalCache = new(false)
	cfg.PtrSpeech = &SpeechConfig{MaxFrames: cfg.SpeechMaxFrames()}
	cfg.CacheTypeK, cfg.CacheTypeV = GGMLTypeF16, GGMLTypeF16
	cfg.Speculation = SpeculationDisabled
	cfg.nBatch, cfg.nUBatch = speech.AutoregressiveBatchSize, speech.AutoregressiveBatchSize

	return cfg, nil
}
