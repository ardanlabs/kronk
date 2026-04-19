package catalog

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
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
	AutoFitVRAM          bool                      `yaml:"auto-fit-vram,omitempty"`
	CacheMinTokens       int                       `yaml:"cache-min-tokens,omitempty"`
	CacheSlotTimeout     int                       `yaml:"cache-slot-timeout,omitempty"`
	CacheTypeK           model.GGMLType            `yaml:"cache-type-k,omitempty"`
	CacheTypeV           model.GGMLType            `yaml:"cache-type-v,omitempty"`
	ContextWindow        int                       `yaml:"context-window,omitempty"`
	Devices              []string                  `yaml:"devices,omitempty"`
	DraftModel           *DraftModelConfig         `yaml:"draft-model,omitempty"`
	FlashAttention       *model.FlashAttentionType `yaml:"flash-attention,omitempty"`
	IgnoreIntegrityCheck bool                      `yaml:"ignore-integrity-check,omitempty"`
	IncrementalCache     bool                      `yaml:"incremental-cache,omitempty"`
	InsecureLogging      bool                      `yaml:"insecure-logging,omitempty"`
	MainGPU              *int                      `yaml:"main-gpu,omitempty"`
	MoE                  *model.MoEConfig          `yaml:"moe,omitempty"`
	NBatch               int                       `yaml:"nbatch,omitempty"`
	NGpuLayers           *int                      `yaml:"ngpu-layers,omitempty"`
	NSeqMax              int                       `yaml:"nseq-max,omitempty"`
	NThreads             int                       `yaml:"nthreads,omitempty"`
	NThreadsBatch        int                       `yaml:"nthreads-batch,omitempty"`
	NUBatch              int                       `yaml:"nubatch,omitempty"`
	NUMA                 string                    `yaml:"numa,omitempty"`
	OffloadKQV           *bool                     `yaml:"offload-kqv,omitempty"`
	OpOffload            *bool                     `yaml:"op-offload,omitempty"`
	OpOffloadMinBatch    int                       `yaml:"op-offload-min-batch,omitempty"`
	RopeFreqBase         *float32                  `yaml:"rope-freq-base,omitempty"`
	RopeFreqScale        *float32                  `yaml:"rope-freq-scale,omitempty"`
	RopeScaling          model.RopeScalingType     `yaml:"rope-scaling-type,omitempty"`
	Sampling             SamplingConfig            `yaml:"sampling-parameters,omitempty"`
	SplitMode            *model.SplitMode          `yaml:"split-mode,omitempty"`
	SWAFull              *bool                     `yaml:"swa-full,omitempty"`
	SystemPromptCache    bool                      `yaml:"system-prompt-cache,omitempty"`
	TensorBuftOverrides  []string                  `yaml:"tensor-buft-overrides,omitempty"`
	TensorSplit          []float32                 `yaml:"tensor-split,omitempty"`
	Template             string                    `yaml:"template,omitempty"`
	UseDirectIO          bool                      `yaml:"use-direct-io,omitempty"`
	UseMMap              *bool                     `yaml:"use-mmap,omitempty"`
	YarnAttnFactor       *float32                  `yaml:"yarn-attn-factor,omitempty"`
	YarnBetaFast         *float32                  `yaml:"yarn-beta-fast,omitempty"`
	YarnBetaSlow         *float32                  `yaml:"yarn-beta-slow,omitempty"`
	YarnExtFactor        *float32                  `yaml:"yarn-ext-factor,omitempty"`
	YarnOrigCtx          *int                      `yaml:"yarn-orig-ctx,omitempty"`
}

// DraftModelConfig configures a draft model for speculative decoding.
type DraftModelConfig struct {
	Devices     []string  `yaml:"devices,omitempty"`
	MainGPU     *int      `yaml:"main-gpu,omitempty"`
	ModelID     string    `yaml:"model-id,omitempty"`
	NDraft      int       `yaml:"ndraft,omitempty"`
	NGpuLayers  *int      `yaml:"ngpu-layers,omitempty"`
	TensorSplit []float32 `yaml:"tensor-split,omitempty"`
}

// ToKronkConfig converts a catalog ModelConfig to a model.Config.
func (mc ModelConfig) ToKronkConfig() model.Config {
	cfg := model.Config{
		AutoFitVRAM:          mc.AutoFitVRAM,
		CacheMinTokens:       mc.CacheMinTokens,
		CacheSlotTimeout:     mc.CacheSlotTimeout,
		CacheTypeK:           mc.CacheTypeK,
		CacheTypeV:           mc.CacheTypeV,
		ContextWindow:        mc.ContextWindow,
		DefaultParams:        mc.Sampling.toParams(),
		Devices:              mc.Devices,
		FlashAttention:       model.DerefFlashAttention(mc.FlashAttention),
		IgnoreIntegrityCheck: mc.IgnoreIntegrityCheck,
		IncrementalCache:     mc.IncrementalCache,
		InsecureLogging:      mc.InsecureLogging,
		MainGPU:              mc.MainGPU,
		MoE:                  mc.MoE,
		NBatch:               mc.NBatch,
		NGpuLayers:           mc.NGpuLayers,
		NSeqMax:              mc.NSeqMax,
		NThreads:             mc.NThreads,
		NThreadsBatch:        mc.NThreadsBatch,
		NUBatch:              mc.NUBatch,
		NUMA:                 mc.NUMA,
		OffloadKQV:           mc.OffloadKQV,
		OpOffload:            mc.OpOffload,
		OpOffloadMinBatch:    mc.OpOffloadMinBatch,
		RopeFreqBase:         mc.RopeFreqBase,
		RopeFreqScale:        mc.RopeFreqScale,
		RopeScaling:          mc.RopeScaling,
		SplitMode:            mc.SplitMode,
		SWAFull:              mc.SWAFull,
		SystemPromptCache:    mc.SystemPromptCache,
		TensorBuftOverrides:  mc.TensorBuftOverrides,
		TensorSplit:          mc.TensorSplit,
		UseDirectIO:          mc.UseDirectIO,
		UseMMap:              mc.UseMMap,
		YarnAttnFactor:       mc.YarnAttnFactor,
		YarnBetaFast:         mc.YarnBetaFast,
		YarnBetaSlow:         mc.YarnBetaSlow,
		YarnExtFactor:        mc.YarnExtFactor,
		YarnOrigCtx:          mc.YarnOrigCtx,
	}

	if mc.DraftModel != nil {
		cfg.DraftModel = &model.DraftModelConfig{
			Devices:     mc.DraftModel.Devices,
			MainGPU:     mc.DraftModel.MainGPU,
			NDraft:      mc.DraftModel.NDraft,
			NGpuLayers:  mc.DraftModel.NGpuLayers,
			TensorSplit: mc.DraftModel.TensorSplit,
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

// TotalSizeBytes returns the total size of all model files in bytes by
// parsing the human-readable size strings (e.g. "8.71 GB"). This handles
// split models by summing all file parts.
func (f Files) TotalSizeBytes() int64 {
	var total int64
	for _, file := range f.Models {
		total += parseSizeToBytes(file.Size)
	}
	return total
}

// TotalSize returns a formatted string of the total size of all model files.
func (f Files) TotalSize() string {
	return formatTotalSize(f.TotalSizeBytes())
}

// parseSizeToBytes parses a human-readable size string like "8.71 GB"
// or "695 MB" into bytes.
func parseSizeToBytes(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	const (
		kb = 1000
		mb = kb * 1000
		gb = mb * 1000
	)

	upper := strings.ToUpper(s)

	var multiplier float64
	var numStr string

	switch {
	case strings.HasSuffix(upper, " GB"):
		multiplier = gb
		numStr = strings.TrimSuffix(upper, " GB")
	case strings.HasSuffix(upper, " MB"):
		multiplier = mb
		numStr = strings.TrimSuffix(upper, " MB")
	case strings.HasSuffix(upper, " KB"):
		multiplier = kb
		numStr = strings.TrimSuffix(upper, " KB")
	case strings.HasSuffix(upper, " B"):
		multiplier = 1
		numStr = strings.TrimSuffix(upper, " B")
	default:
		return 0
	}

	val, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
	if err != nil {
		return 0
	}

	return int64(val * multiplier)
}

// formatTotalSize formats bytes into a human-readable size string.
func formatTotalSize(bytes int64) string {
	const (
		kb = 1000
		mb = kb * 1000
		gb = mb * 1000
	)

	switch {
	case bytes >= gb:
		val := float64(bytes) / float64(gb)
		return fmt.Sprintf("%.1f GB", math.Round(val*10)/10)
	case bytes >= mb:
		val := float64(bytes) / float64(mb)
		return fmt.Sprintf("%.1f MB", math.Round(val*10)/10)
	case bytes >= kb:
		val := float64(bytes) / float64(kb)
		return fmt.Sprintf("%.1f KB", math.Round(val*10)/10)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// ModelDetails represents information for a model.
type ModelDetails struct {
	ID              string       `yaml:"id"`
	Category        string       `yaml:"category"`
	OwnedBy         string       `yaml:"owned_by,omitempty"`
	ModelFamily     string       `yaml:"model_family,omitempty"`
	Architecture    string       `yaml:"architecture,omitempty"`
	GGUFArch        string       `yaml:"gguf_arch,omitempty"`
	Parameters      string       `yaml:"parameters,omitempty"`
	WebPage         string       `yaml:"web_page,omitempty"`
	GatedModel      bool         `yaml:"gated_model,omitempty"`
	TestingModel    bool         `yaml:"testing_model,omitempty"`
	Template        string       `yaml:"template,omitempty"`
	Files           Files        `yaml:"files"`
	Capabilities    Capabilities `yaml:"capabilities,omitempty"`
	Metadata        Metadata     `yaml:"metadata,omitempty"`
	BaseModelConfig ModelConfig  `yaml:"config,omitempty"`
	Downloaded      bool         `yaml:"-"`
	Validated       bool         `yaml:"-"`
	CatalogFile     string       `yaml:"-"`
}

// ExtractParameterLabel extracts a parameter count label (e.g. "8B", "0.6B",
// "8x7B") from a model ID string. It returns an empty string if no label is found.
func ExtractParameterLabel(id string) string {
	// Match patterns like "0.6B", "8B", "70B", "8x7B", "700M" in the model ID.
	// The label is typically preceded by a "-" or start of string,
	// and followed by "-" or end of string.
	re := regexp.MustCompile(`(?i)(?:^|[-_])(\d+x)?(\d+(?:\.\d+)?[BM])(?:[-_]|$)`)
	match := re.FindStringSubmatch(id)
	if match == nil {
		return ""
	}
	return match[1] + match[2]
}

// ParseParameterCount converts a parameter label like "8B" or "0.6B" or "8x7B"
// into a raw parameter count (e.g. 8_000_000_000). Returns 0 if parsing fails.
func ParseParameterCount(label string) int64 {
	if label == "" {
		return 0
	}

	label = strings.TrimSpace(label)

	// Handle multiplier prefix like "8x" in "8x7B" or "8X7B".
	lower := strings.ToLower(label)
	multiplier := int64(1)
	if idx := strings.Index(lower, "x"); idx > 0 {
		m, err := strconv.ParseInt(lower[:idx], 10, 64)
		if err != nil {
			return 0
		}
		multiplier = m
		label = label[idx+1:]
	}

	upper := strings.ToUpper(strings.TrimSpace(label))

	var scale float64
	var numStr string

	switch {
	case strings.HasSuffix(upper, "B"):
		scale = 1e9
		numStr = strings.TrimSuffix(upper, "B")
	case strings.HasSuffix(upper, "M"):
		scale = 1e6
		numStr = strings.TrimSuffix(upper, "M")
	default:
		return 0
	}

	val, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
	if err != nil {
		return 0
	}

	return int64(math.Round(val*scale)) * multiplier
}

// CatalogModels represents a set of models for a given catalog.
type CatalogModels struct {
	Name   string         `yaml:"catalog"`
	Models []ModelDetails `yaml:"models"`
}
