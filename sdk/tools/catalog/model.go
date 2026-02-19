package catalog

import (
	"time"

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
		MaxTokens:       model.DefMaxTokens,
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
	if s.MaxTokens == 0 {
		s.MaxTokens = defaults.MaxTokens
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
	Device               string                   `yaml:"device,omitempty"`
	ContextWindow        int                      `yaml:"context-window,omitempty"`
	NBatch               int                      `yaml:"nbatch,omitempty"`
	NUBatch              int                      `yaml:"nubatch,omitempty"`
	NThreads             int                      `yaml:"nthreads,omitempty"`
	NThreadsBatch        int                      `yaml:"nthreads-batch,omitempty"`
	CacheTypeK           model.GGMLType           `yaml:"cache-type-k,omitempty"`
	CacheTypeV           model.GGMLType           `yaml:"cache-type-v,omitempty"`
	UseDirectIO          bool                     `yaml:"use-direct-io,omitempty"`
	FlashAttention       model.FlashAttentionType `yaml:"flash-attention,omitempty"`
	IgnoreIntegrityCheck bool                     `yaml:"ignore-integrity-check,omitempty"`
	NSeqMax              int                      `yaml:"nseq-max,omitempty"`
	OffloadKQV           *bool                    `yaml:"offload-kqv,omitempty"`
	OpOffload            *bool                    `yaml:"op-offload,omitempty"`
	NGpuLayers           *int                     `yaml:"ngpu-layers,omitempty"`
	SplitMode            model.SplitMode          `yaml:"split-mode,omitempty"`
	SystemPromptCache    bool                     `yaml:"system-prompt-cache,omitempty"`
	IncrementalCache     bool                     `yaml:"incremental-cache,omitempty"`
	CacheMinTokens       int                      `yaml:"cache-min-tokens,omitempty"`
	CacheSlotTimeout     int                      `yaml:"cache-slot-timeout,omitempty"`
	InsecureLogging      bool                     `yaml:"insecure-logging,omitempty"`
	RopeScaling          model.RopeScalingType    `yaml:"rope-scaling-type,omitempty"`
	RopeFreqBase         *float32                 `yaml:"rope-freq-base,omitempty"`
	RopeFreqScale        *float32                 `yaml:"rope-freq-scale,omitempty"`
	YarnExtFactor        *float32                 `yaml:"yarn-ext-factor,omitempty"`
	YarnAttnFactor       *float32                 `yaml:"yarn-attn-factor,omitempty"`
	YarnBetaFast         *float32                 `yaml:"yarn-beta-fast,omitempty"`
	YarnBetaSlow         *float32                 `yaml:"yarn-beta-slow,omitempty"`
	YarnOrigCtx          *int                     `yaml:"yarn-orig-ctx,omitempty"`
	DraftModel           *DraftModelConfig        `yaml:"draft-model,omitempty"`
	Sampling             SamplingConfig           `yaml:"sampling-parameters,omitempty"`
}

// DraftModelConfig configures a draft model for speculative decoding.
type DraftModelConfig struct {
	ModelID    string `yaml:"model-id,omitempty"`
	NDraft     int    `yaml:"ndraft,omitempty"`
	NGpuLayers *int   `yaml:"ngpu-layers,omitempty"`
	Device     string `yaml:"device,omitempty"`
}

// ToKronkConfig converts a catalog ModelConfig to a model.Config.
func (mc ModelConfig) ToKronkConfig() model.Config {
	cfg := model.Config{
		Device:               mc.Device,
		ContextWindow:        mc.ContextWindow,
		NBatch:               mc.NBatch,
		NUBatch:              mc.NUBatch,
		NThreads:             mc.NThreads,
		NThreadsBatch:        mc.NThreadsBatch,
		CacheTypeK:           mc.CacheTypeK,
		CacheTypeV:           mc.CacheTypeV,
		UseDirectIO:          mc.UseDirectIO,
		FlashAttention:       mc.FlashAttention,
		IgnoreIntegrityCheck: mc.IgnoreIntegrityCheck,
		NSeqMax:              mc.NSeqMax,
		OffloadKQV:           mc.OffloadKQV,
		OpOffload:            mc.OpOffload,
		NGpuLayers:           mc.NGpuLayers,
		SplitMode:            mc.SplitMode,
		SystemPromptCache:    mc.SystemPromptCache,
		IncrementalCache:     mc.IncrementalCache,
		CacheMinTokens:       mc.CacheMinTokens,
		CacheSlotTimeout:     mc.CacheSlotTimeout,
		InsecureLogging:      mc.InsecureLogging,
		RopeScaling:          mc.RopeScaling,
		RopeFreqBase:         mc.RopeFreqBase,
		RopeFreqScale:        mc.RopeFreqScale,
		YarnExtFactor:        mc.YarnExtFactor,
		YarnAttnFactor:       mc.YarnAttnFactor,
		YarnBetaFast:         mc.YarnBetaFast,
		YarnBetaSlow:         mc.YarnBetaSlow,
		YarnOrigCtx:          mc.YarnOrigCtx,
		DefaultParams:        mc.Sampling.toParams(),
	}

	if mc.DraftModel != nil {
		cfg.DraftModel = &model.DraftModelConfig{
			NDraft:     mc.DraftModel.NDraft,
			NGpuLayers: mc.DraftModel.NGpuLayers,
			Device:     mc.DraftModel.Device,
		}
	}

	return cfg
}

// Metadata represents extra information about the model.
type Metadata struct {
	Created     time.Time `yaml:"created,omitempty"`
	Collections string    `yaml:"collections,omitempty"`
	Description string    `yaml:"description,omitempty"`
}

// Capabilities represents the capabilities of a model.
type Capabilities struct {
	Endpoint  string `yaml:"endpoint,omitempty"`
	Images    bool   `yaml:"images,omitempty"`
	Audio     bool   `yaml:"audio,omitempty"`
	Video     bool   `yaml:"video,omitempty"`
	Streaming bool   `yaml:"streaming,omitempty"`
	Reasoning bool   `yaml:"reasoning,omitempty"`
	Tooling   bool   `yaml:"tooling,omitempty"`
	Embedding bool   `yaml:"embedding,omitempty"`
	Rerank    bool   `yaml:"rerank,omitempty"`
}

// File represents the actual file url and size.
type File struct {
	URL  string `yaml:"url,omitempty"`
	Size string `yaml:"size,omitempty"`
}

// Files represents file information for a model.
type Files struct {
	Models []File `yaml:"models"`
	Proj   File   `yaml:"proj,omitempty"`
}

// ToModelURLS converts a slice of File to a string of the URLs.
func (f Files) ToModelURLS() []string {
	models := make([]string, len(f.Models))

	for i, file := range f.Models {
		models[i] = file.URL
	}

	return models
}

// ModelDetails represents information for a model.
type ModelDetails struct {
	ID              string       `yaml:"id"`
	Category        string       `yaml:"category"`
	OwnedBy         string       `yaml:"owned_by,omitempty"`
	ModelFamily     string       `yaml:"model_family,omitempty"`
	WebPage         string       `yaml:"web_page,omitempty"`
	GatedModel      bool         `yaml:"gated_model,omitempty"`
	Template        string       `yaml:"template,omitempty"`
	Files           Files        `yaml:"files"`
	Capabilities    Capabilities `yaml:"capabilities,omitempty"`
	Metadata        Metadata     `yaml:"metadata,omitempty"`
	BaseModelConfig ModelConfig  `yaml:"config,omitempty"`
	Downloaded      bool         `yaml:"-"`
	Validated       bool         `yaml:"-"`
	CatalogFile     string       `yaml:"-"`
}

// CatalogModels represents a set of models for a given catalog.
type CatalogModels struct {
	Name   string         `yaml:"catalog"`
	Models []ModelDetails `yaml:"models"`
}
