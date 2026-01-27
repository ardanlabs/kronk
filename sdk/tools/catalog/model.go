package catalog

import (
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// SamplingConfig represents sampling parameters for model inference.
type SamplingConfig struct {
	Temperature     float32 `yaml:"temperature"`
	TopK            int32   `yaml:"top_k"`
	TopP            float32 `yaml:"top_p"`
	MinP            float32 `yaml:"min_p"`
	MaxTokens       int     `yaml:"max_tokens"`
	RepeatPenalty   float32 `yaml:"repeat_penalty"`
	RepeatLastN     int32   `yaml:"repeat_last_n"`
	DryMultiplier   float32 `yaml:"dry_multiplier"`
	DryBase         float32 `yaml:"dry_base"`
	DryAllowedLen   int32   `yaml:"dry_allowed_length"`
	DryPenaltyLast  int32   `yaml:"dry_penalty_last_n"`
	XtcProbability  float32 `yaml:"xtc_probability"`
	XtcThreshold    float32 `yaml:"xtc_threshold"`
	XtcMinKeep      uint32  `yaml:"xtc_min_keep"`
	EnableThinking  string  `yaml:"enable_thinking"`
	ReasoningEffort string  `yaml:"reasoning_effort"`
}

// withDefaults returns a new SamplingConfig with default values applied
// for any zero-valued fields.
func (s SamplingConfig) withDefaults() SamplingConfig {
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

// ModelConfig represents default model config settings.
type ModelConfig struct {
	Device               string                   `yaml:"device"`
	ContextWindow        int                      `yaml:"context-window"`
	NBatch               int                      `yaml:"nbatch"`
	NUBatch              int                      `yaml:"nubatch"`
	NThreads             int                      `yaml:"nthreads"`
	NThreadsBatch        int                      `yaml:"nthreads-batch"`
	CacheTypeK           model.GGMLType           `yaml:"cache-type-k"`
	CacheTypeV           model.GGMLType           `yaml:"cache-type-v"`
	UseDirectIO          bool                     `yaml:"use-direct-io"`
	FlashAttention       model.FlashAttentionType `yaml:"flash-attention"`
	IgnoreIntegrityCheck bool                     `yaml:"ignore-integrity-check"`
	NSeqMax              int                      `yaml:"nseq-max"`
	OffloadKQV           *bool                    `yaml:"offload-kqv"`
	OpOffload            *bool                    `yaml:"op-offload"`
	NGpuLayers           *int32                   `yaml:"ngpu-layers"`
	SplitMode            model.SplitMode          `yaml:"split-mode"`
	SystemPromptCache    bool                     `yaml:"system-prompt-cache"`
	FirstMessageCache    bool                     `yaml:"first-message-cache"`
	CacheMinTokens       int                      `yaml:"cache-min-tokens"`
	Sampling             SamplingConfig           `yaml:"sampling-parameters"`
}

func (mc ModelConfig) toKronkConfig() model.Config {
	return model.Config{
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
		FirstMessageCache:    mc.FirstMessageCache,
		CacheMinTokens:       mc.CacheMinTokens,
	}
}

// Metadata represents extra information about the model.
type Metadata struct {
	Created     time.Time `yaml:"created"`
	Collections string    `yaml:"collections"`
	Description string    `yaml:"description"`
}

// Capabilities represents the capabilities of a model.
type Capabilities struct {
	Endpoint  string `yaml:"endpoint"`
	Images    bool   `yaml:"images"`
	Audio     bool   `yaml:"audio"`
	Video     bool   `yaml:"video"`
	Streaming bool   `yaml:"streaming"`
	Reasoning bool   `yaml:"reasoning"`
	Tooling   bool   `yaml:"tooling"`
	Embedding bool   `yaml:"embedding"`
	Rerank    bool   `yaml:"rerank"`
}

// File represents the actual file url and size.
type File struct {
	URL  string `yaml:"url"`
	Size string `yaml:"size"`
}

// Files represents file information for a model.
type Files struct {
	Models []File `yaml:"models"`
	Proj   File   `yaml:"proj"`
}

// ToModelURLS converts a slice of File to a string of the URLs.
func (f Files) ToModelURLS() []string {
	models := make([]string, len(f.Models))

	for i, file := range f.Models {
		models[i] = file.URL
	}

	return models
}

// Model represents information for a model.
type Model struct {
	ID           string       `yaml:"id"`
	Category     string       `yaml:"category"`
	OwnedBy      string       `yaml:"owned_by"`
	ModelFamily  string       `yaml:"model_family"`
	WebPage      string       `yaml:"web_page"`
	GatedModel   bool         `yaml:"gated_model"`
	Template     string       `yaml:"template"`
	Files        Files        `yaml:"files"`
	Capabilities Capabilities `yaml:"capabilities"`
	Metadata     Metadata     `yaml:"metadata"`
	ModelConfig  ModelConfig  `yaml:"config"`
	Downloaded   bool
	Validated    bool
}

// CatalogModels represents a set of models for a given catalog.
type CatalogModels struct {
	Name   string  `yaml:"catalog"`
	Models []Model `yaml:"models"`
}
