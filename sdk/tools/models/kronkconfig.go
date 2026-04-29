package models

import (
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// SamplingConfig represents sampling parameters for model inference.
type SamplingConfig struct {
	Temperature      float32 `yaml:"temperature,omitempty"`
	TopK             int32   `yaml:"top_k,omitempty"`
	TopP             float32 `yaml:"top_p,omitempty"`
	MinP             float32 `yaml:"min_p,omitempty"`
	MaxTokens        int     `yaml:"max_tokens,omitempty"`
	RepeatPenalty    float32 `yaml:"repeat_penalty,omitempty"`
	RepeatLastN      int32   `yaml:"repeat_last_n,omitempty"`
	DryMultiplier    float32 `yaml:"dry_multiplier,omitempty"`
	DryBase          float32 `yaml:"dry_base,omitempty"`
	DryAllowedLen    int32   `yaml:"dry_allowed_length,omitempty"`
	DryPenaltyLast   int32   `yaml:"dry_penalty_last_n,omitempty"`
	XtcProbability   float32 `yaml:"xtc_probability,omitempty"`
	XtcThreshold     float32 `yaml:"xtc_threshold,omitempty"`
	XtcMinKeep       uint32  `yaml:"xtc_min_keep,omitempty"`
	FrequencyPenalty float32 `yaml:"frequency_penalty,omitempty"`
	PresencePenalty  float32 `yaml:"presence_penalty,omitempty"`
	EnableThinking   string  `yaml:"enable_thinking,omitempty"`
	ReasoningEffort  string  `yaml:"reasoning_effort,omitempty"`
	Grammar          string  `yaml:"grammar,omitempty"`
}

// WithDefaults returns a new SamplingConfig with default values applied
// for any zero-valued fields.
func (s SamplingConfig) WithDefaults() SamplingConfig {
	defaults := SamplingConfig{
		Temperature:     model.DefTemp,
		TopK:            model.DefTopK,
		TopP:            model.DefTopP,
		MinP:            model.DefMinP,
		RepeatPenalty:   model.DefRepeatPenalty,
		RepeatLastN:     model.DefRepeatLastN,
		DryMultiplier:   model.DefDryMultiplier,
		DryBase:         model.DefDryBase,
		DryAllowedLen:   model.DefDryAllowedLen,
		DryPenaltyLast:  model.DefDryPenaltyLast,
		XtcProbability:  model.DefXtcProbability,
		XtcThreshold:    model.DefXtcThreshold,
		XtcMinKeep:      model.DefXtcMinKeep,
		EnableThinking:  model.DefEnableThinking,
		ReasoningEffort: model.DefReasoningEffort,
	}

	if s.Temperature == 0 {
		s.Temperature = defaults.Temperature
	}
	if s.TopK == 0 {
		s.TopK = defaults.TopK
	}
	if s.TopP == 0 {
		s.TopP = defaults.TopP
	}
	if s.MinP == 0 {
		s.MinP = defaults.MinP
	}
	if s.RepeatPenalty == 0 {
		s.RepeatPenalty = defaults.RepeatPenalty
	}
	if s.RepeatLastN == 0 {
		s.RepeatLastN = defaults.RepeatLastN
	}
	if s.DryMultiplier == 0 {
		s.DryMultiplier = defaults.DryMultiplier
	}
	if s.DryBase == 0 {
		s.DryBase = defaults.DryBase
	}
	if s.DryAllowedLen == 0 {
		s.DryAllowedLen = defaults.DryAllowedLen
	}
	if s.DryPenaltyLast == 0 {
		s.DryPenaltyLast = defaults.DryPenaltyLast
	}
	if s.XtcProbability == 0 {
		s.XtcProbability = defaults.XtcProbability
	}
	if s.XtcThreshold == 0 {
		s.XtcThreshold = defaults.XtcThreshold
	}
	if s.XtcMinKeep == 0 {
		s.XtcMinKeep = defaults.XtcMinKeep
	}
	if s.EnableThinking == "" {
		s.EnableThinking = defaults.EnableThinking
	}
	if s.ReasoningEffort == "" {
		s.ReasoningEffort = defaults.ReasoningEffort
	}

	return s
}

// mergeSampling merges the override sampling config on top of the base,
// keeping base values for any zero-valued fields in the override.
func mergeSampling(base SamplingConfig, override SamplingConfig) SamplingConfig {
	if override.Temperature != 0 {
		base.Temperature = override.Temperature
	}
	if override.TopK != 0 {
		base.TopK = override.TopK
	}
	if override.TopP != 0 {
		base.TopP = override.TopP
	}
	if override.MinP != 0 {
		base.MinP = override.MinP
	}
	if override.MaxTokens != 0 {
		base.MaxTokens = override.MaxTokens
	}
	if override.RepeatPenalty != 0 {
		base.RepeatPenalty = override.RepeatPenalty
	}
	if override.RepeatLastN != 0 {
		base.RepeatLastN = override.RepeatLastN
	}
	if override.DryMultiplier != 0 {
		base.DryMultiplier = override.DryMultiplier
	}
	if override.DryBase != 0 {
		base.DryBase = override.DryBase
	}
	if override.DryAllowedLen != 0 {
		base.DryAllowedLen = override.DryAllowedLen
	}
	if override.DryPenaltyLast != 0 {
		base.DryPenaltyLast = override.DryPenaltyLast
	}
	if override.XtcProbability != 0 {
		base.XtcProbability = override.XtcProbability
	}
	if override.XtcThreshold != 0 {
		base.XtcThreshold = override.XtcThreshold
	}
	if override.XtcMinKeep != 0 {
		base.XtcMinKeep = override.XtcMinKeep
	}
	if override.FrequencyPenalty != 0 {
		base.FrequencyPenalty = override.FrequencyPenalty
	}
	if override.PresencePenalty != 0 {
		base.PresencePenalty = override.PresencePenalty
	}
	if override.EnableThinking != "" {
		base.EnableThinking = override.EnableThinking
	}
	if override.ReasoningEffort != "" {
		base.ReasoningEffort = override.ReasoningEffort
	}
	if override.Grammar != "" {
		base.Grammar = override.Grammar
	}

	return base
}

func (s SamplingConfig) toParams() model.Params {
	s = s.WithDefaults()

	return model.Params{
		Temperature:      s.Temperature,
		TopK:             s.TopK,
		TopP:             s.TopP,
		MinP:             s.MinP,
		MaxTokens:        s.MaxTokens,
		RepeatPenalty:    s.RepeatPenalty,
		RepeatLastN:      s.RepeatLastN,
		DryMultiplier:    s.DryMultiplier,
		DryBase:          s.DryBase,
		DryAllowedLen:    s.DryAllowedLen,
		DryPenaltyLast:   s.DryPenaltyLast,
		FrequencyPenalty: s.FrequencyPenalty,
		PresencePenalty:  s.PresencePenalty,
		XtcProbability:   s.XtcProbability,
		XtcThreshold:     s.XtcThreshold,
		XtcMinKeep:       s.XtcMinKeep,
		Thinking:         s.EnableThinking,
		ReasoningEffort:  s.ReasoningEffort,
		Grammar:          s.Grammar,
	}
}

// ModelConfig represents default model config settings.
type ModelConfig struct {
	PtrCacheMinTokens    *int                      `yaml:"cache-min-tokens,omitempty"`
	PtrCacheSlotTimeout  *int                      `yaml:"cache-slot-timeout,omitempty"`
	CacheTypeK           model.GGMLType            `yaml:"cache-type-k,omitempty"`
	CacheTypeV           model.GGMLType            `yaml:"cache-type-v,omitempty"`
	PtrContextWindow     *int                      `yaml:"context-window,omitempty"`
	Devices              []string                  `yaml:"devices,omitempty"`
	DraftModel           *DraftModelConfig         `yaml:"draft-model,omitempty"`
	FlashAttention       *model.FlashAttentionType `yaml:"flash-attention,omitempty"`
	PtrIncrementalCache  *bool                     `yaml:"incremental-cache,omitempty"`
	PtrInsecureLogging   *bool                     `yaml:"insecure-logging,omitempty"`
	PtrMainGPU           *int                      `yaml:"main-gpu,omitempty"`
	MoE                  *model.MoEConfig          `yaml:"moe,omitempty"`
	PtrNBatch            *int                      `yaml:"nbatch,omitempty"`
	PtrNGpuLayers        *int                      `yaml:"ngpu-layers,omitempty"`
	PtrNSeqMax           *int                      `yaml:"nseq-max,omitempty"`
	PtrNThreads          *int                      `yaml:"nthreads,omitempty"`
	PtrNThreadsBatch     *int                      `yaml:"nthreads-batch,omitempty"`
	PtrNUBatch           *int                      `yaml:"nubatch,omitempty"`
	NUMA                 string                    `yaml:"numa,omitempty"`
	PtrOffloadKQV        *bool                     `yaml:"offload-kqv,omitempty"`
	PtrOpOffload         *bool                     `yaml:"op-offload,omitempty"`
	PtrOpOffloadMinBatch *int                      `yaml:"op-offload-min-batch,omitempty"`
	PtrRopeFreqBase      *float32                  `yaml:"rope-freq-base,omitempty"`
	PtrRopeFreqScale     *float32                  `yaml:"rope-freq-scale,omitempty"`
	RopeScaling          model.RopeScalingType     `yaml:"rope-scaling-type,omitempty"`
	Sampling             SamplingConfig            `yaml:"sampling-parameters,omitempty"`
	PtrSplitMode         *model.SplitMode          `yaml:"split-mode,omitempty"`
	PtrSWAFull           *bool                     `yaml:"swa-full,omitempty"`
	TensorBuftOverrides  []string                  `yaml:"tensor-buft-overrides,omitempty"`
	TensorSplit          []float32                 `yaml:"tensor-split,omitempty"`
	Template             string                    `yaml:"template,omitempty"`
	PtrUseDirectIO       *bool                     `yaml:"use-direct-io,omitempty"`
	PtrUseMMap           *bool                     `yaml:"use-mmap,omitempty"`
	PtrYarnAttnFactor    *float32                  `yaml:"yarn-attn-factor,omitempty"`
	PtrYarnBetaFast      *float32                  `yaml:"yarn-beta-fast,omitempty"`
	PtrYarnBetaSlow      *float32                  `yaml:"yarn-beta-slow,omitempty"`
	PtrYarnExtFactor     *float32                  `yaml:"yarn-ext-factor,omitempty"`
	PtrYarnOrigCtx       *int                      `yaml:"yarn-orig-ctx,omitempty"`
}

// DraftModelConfig configures a draft model for speculative decoding.
type DraftModelConfig struct {
	Devices       []string  `yaml:"devices,omitempty"`
	PtrMainGPU    *int      `yaml:"main-gpu,omitempty"`
	ModelID       string    `yaml:"model-id,omitempty"`
	NDraft        int       `yaml:"ndraft,omitempty"`
	PtrNGpuLayers *int      `yaml:"ngpu-layers,omitempty"`
	TensorSplit   []float32 `yaml:"tensor-split,omitempty"`
}

// ToKronkConfig converts a ModelConfig to a model.Config.
func (mc ModelConfig) ToKronkConfig() model.Config {
	cfg := model.Config{
		PtrCacheMinTokens:    mc.PtrCacheMinTokens,
		PtrCacheSlotTimeout:  mc.PtrCacheSlotTimeout,
		CacheTypeK:           mc.CacheTypeK,
		CacheTypeV:           mc.CacheTypeV,
		PtrContextWindow:     mc.PtrContextWindow,
		DefaultParams:        mc.Sampling.toParams(),
		Devices:              mc.Devices,
		FlashAttention:       model.DerefFlashAttention(mc.FlashAttention),
		PtrIncrementalCache:  mc.PtrIncrementalCache,
		PtrInsecureLogging:   mc.PtrInsecureLogging,
		PtrMainGPU:           mc.PtrMainGPU,
		MoE:                  mc.MoE,
		PtrNBatch:            mc.PtrNBatch,
		PtrNGpuLayers:        mc.PtrNGpuLayers,
		PtrNSeqMax:           mc.PtrNSeqMax,
		PtrNThreads:          mc.PtrNThreads,
		PtrNThreadsBatch:     mc.PtrNThreadsBatch,
		PtrNUBatch:           mc.PtrNUBatch,
		NUMA:                 mc.NUMA,
		PtrOffloadKQV:        mc.PtrOffloadKQV,
		PtrOpOffload:         mc.PtrOpOffload,
		PtrOpOffloadMinBatch: mc.PtrOpOffloadMinBatch,
		PtrRopeFreqBase:      mc.PtrRopeFreqBase,
		PtrRopeFreqScale:     mc.PtrRopeFreqScale,
		RopeScaling:          mc.RopeScaling,
		PtrSplitMode:         mc.PtrSplitMode,
		PtrSWAFull:           mc.PtrSWAFull,
		TensorBuftOverrides:  mc.TensorBuftOverrides,
		TensorSplit:          mc.TensorSplit,
		PtrUseDirectIO:       mc.PtrUseDirectIO,
		PtrUseMMap:           mc.PtrUseMMap,
		PtrYarnAttnFactor:    mc.PtrYarnAttnFactor,
		PtrYarnBetaFast:      mc.PtrYarnBetaFast,
		PtrYarnBetaSlow:      mc.PtrYarnBetaSlow,
		PtrYarnExtFactor:     mc.PtrYarnExtFactor,
		PtrYarnOrigCtx:       mc.PtrYarnOrigCtx,
	}

	if mc.DraftModel != nil {
		cfg.DraftModel = &model.DraftModelConfig{
			Devices:       mc.DraftModel.Devices,
			PtrMainGPU:    mc.DraftModel.PtrMainGPU,
			NDraft:        mc.DraftModel.NDraft,
			PtrNGpuLayers: mc.DraftModel.PtrNGpuLayers,
			TensorSplit:   mc.DraftModel.TensorSplit,
		}
	}

	return cfg
}
