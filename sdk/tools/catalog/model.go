package catalog

import (
	"time"
)

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

// DefaultContext represents default sampling parameters for a model.
type DefaultContext struct {
	Temperature     *float32 `yaml:"temperature"`
	TopK            *int32   `yaml:"top_k"`
	TopP            *float32 `yaml:"top_p"`
	MinP            *float32 `yaml:"min_p"`
	MaxTokens       *int     `yaml:"max_tokens"`
	RepeatPenalty   *float32 `yaml:"repeat_penalty"`
	RepeatLastN     *int32   `yaml:"repeat_last_n"`
	DryMultiplier   *float32 `yaml:"dry_multiplier"`
	DryBase         *float32 `yaml:"dry_base"`
	DryAllowedLen   *int32   `yaml:"dry_allowed_length"`
	DryPenaltyLast  *int32   `yaml:"dry_penalty_last_n"`
	XtcProbability  *float32 `yaml:"xtc_probability"`
	XtcThreshold    *float32 `yaml:"xtc_threshold"`
	XtcMinKeep      *uint32  `yaml:"xtc_min_keep"`
	Thinking        *string  `yaml:"enable_thinking"`
	ReasoningEffort *string  `yaml:"reasoning_effort"`
	ReturnPrompt    *bool    `yaml:"return_prompt"`
	IncludeUsage    *bool    `yaml:"include_usage"`
	Logprobs        *bool    `yaml:"logprobs"`
	TopLogprobs     *int     `yaml:"top_logprobs"`
	Stream          *bool    `yaml:"stream"`
}

// Model represents information for a model.
type Model struct {
	ID             string         `yaml:"id"`
	Category       string         `yaml:"category"`
	OwnedBy        string         `yaml:"owned_by"`
	ModelFamily    string         `yaml:"model_family"`
	WebPage        string         `yaml:"web_page"`
	GatedModel     bool           `yaml:"gated_model"`
	Template       string         `yaml:"template"`
	Files          Files          `yaml:"files"`
	Capabilities   Capabilities   `yaml:"capabilities"`
	Metadata       Metadata       `yaml:"metadata"`
	DefaultContext DefaultContext `yaml:"default_context"`
	Downloaded     bool
	Validated      bool
}

// CatalogModels represents a set of models for a given catalog.
type CatalogModels struct {
	Name   string  `yaml:"catalog"`
	Models []Model `yaml:"models"`
}

// ApplyDefaults applies default context values from DefaultContext to a map.
// It only sets values if they don't already exist in the map.
func (dc DefaultContext) ApplyDefaults(d map[string]any) map[string]any {
	if _, exists := d["temperature"]; !exists && dc.Temperature != nil {
		d["temperature"] = *dc.Temperature
	}
	if _, exists := d["top_k"]; !exists && dc.TopK != nil {
		d["top_k"] = *dc.TopK
	}
	if _, exists := d["top_p"]; !exists && dc.TopP != nil {
		d["top_p"] = *dc.TopP
	}
	if _, exists := d["min_p"]; !exists && dc.MinP != nil {
		d["min_p"] = *dc.MinP
	}
	if _, exists := d["max_tokens"]; !exists && dc.MaxTokens != nil {
		d["max_tokens"] = *dc.MaxTokens
	}
	if _, exists := d["repeat_penalty"]; !exists && dc.RepeatPenalty != nil {
		d["repeat_penalty"] = *dc.RepeatPenalty
	}
	if _, exists := d["repeat_last_n"]; !exists && dc.RepeatLastN != nil {
		d["repeat_last_n"] = *dc.RepeatLastN
	}
	if _, exists := d["dry_multiplier"]; !exists && dc.DryMultiplier != nil {
		d["dry_multiplier"] = *dc.DryMultiplier
	}
	if _, exists := d["dry_base"]; !exists && dc.DryBase != nil {
		d["dry_base"] = *dc.DryBase
	}
	if _, exists := d["dry_allowed_length"]; !exists && dc.DryAllowedLen != nil {
		d["dry_allowed_length"] = *dc.DryAllowedLen
	}
	if _, exists := d["dry_penalty_last_n"]; !exists && dc.DryPenaltyLast != nil {
		d["dry_penalty_last_n"] = *dc.DryPenaltyLast
	}
	if _, exists := d["xtc_probability"]; !exists && dc.XtcProbability != nil {
		d["xtc_probability"] = *dc.XtcProbability
	}
	if _, exists := d["xtc_threshold"]; !exists && dc.XtcThreshold != nil {
		d["xtc_threshold"] = *dc.XtcThreshold
	}
	if _, exists := d["xtc_min_keep"]; !exists && dc.XtcMinKeep != nil {
		d["xtc_min_keep"] = *dc.XtcMinKeep
	}
	if _, exists := d["enable_thinking"]; !exists && dc.Thinking != nil {
		d["enable_thinking"] = *dc.Thinking
	}
	if _, exists := d["reasoning_effort"]; !exists && dc.ReasoningEffort != nil {
		d["reasoning_effort"] = *dc.ReasoningEffort
	}
	if _, exists := d["return_prompt"]; !exists && dc.ReturnPrompt != nil {
		d["return_prompt"] = *dc.ReturnPrompt
	}
	if _, exists := d["include_usage"]; !exists && dc.IncludeUsage != nil {
		d["include_usage"] = *dc.IncludeUsage
	}
	if _, exists := d["logprobs"]; !exists && dc.Logprobs != nil {
		d["logprobs"] = *dc.Logprobs
	}
	if _, exists := d["top_logprobs"]; !exists && dc.TopLogprobs != nil {
		d["top_logprobs"] = *dc.TopLogprobs
	}
	if _, exists := d["stream"]; !exists && dc.Stream != nil {
		d["stream"] = *dc.Stream
	}

	return d
}
