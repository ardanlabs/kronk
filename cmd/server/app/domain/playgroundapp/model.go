package playgroundapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// =============================================================================

// SessionRequest represents the request to create a playground session.
type SessionRequest struct {
	ModelID        string        `json:"model_id"`
	TemplateMode   string        `json:"template_mode"`
	TemplateName   string        `json:"template_name"`
	TemplateScript string        `json:"template_script"`
	Config         SessionConfig `json:"config"`
}

// Decode implements the decoder interface.
func (s *SessionRequest) Decode(data []byte) error {
	return json.Unmarshal(data, s)
}

// Validate checks the request.
func (s *SessionRequest) Validate() error {
	if s.ModelID == "" {
		return errors.New("model_id is required")
	}

	switch s.TemplateMode {
	case "builtin", "custom":
	case "":
		s.TemplateMode = "builtin"
	default:
		return fmt.Errorf("invalid template_mode: %s", s.TemplateMode)
	}

	if s.TemplateMode == "custom" && s.TemplateScript == "" {
		return errors.New("template_script is required when template_mode is custom")
	}

	if s.TemplateName != "" {
		if err := validateTemplateName(s.TemplateName); err != nil {
			return err
		}
	}

	return s.Config.Validate()
}

// SessionConfig represents model configuration overrides. Pointer fields allow
// distinguishing "not set by user" (nil) from an explicit value, so that only
// user-provided overrides are merged on top of the catalog-resolved base config.
type SessionConfig struct {
	ContextWindow     *int                      `json:"context_window"`
	NBatch            *int                      `json:"nbatch"`
	NUBatch           *int                      `json:"nubatch"`
	NSeqMax           *int                      `json:"nseq_max"`
	FlashAttention    *model.FlashAttentionType `json:"flash_attention"`
	CacheTypeK        *model.GGMLType           `json:"cache_type_k"`
	CacheTypeV        *model.GGMLType           `json:"cache_type_v"`
	NGpuLayers        *int                      `json:"ngpu_layers"`
	SystemPromptCache *bool                     `json:"system_prompt_cache"`
	IncrementalCache  *bool                     `json:"incremental_cache"`
	RopeScaling       *model.RopeScalingType    `json:"rope_scaling_type"`
	RopeFreqBase      *float32                  `json:"rope_freq_base"`
	RopeFreqScale     *float32                  `json:"rope_freq_scale"`
	YarnExtFactor     *float32                  `json:"yarn_ext_factor"`
	YarnAttnFactor    *float32                  `json:"yarn_attn_factor"`
	YarnBetaFast      *float32                  `json:"yarn_beta_fast"`
	YarnBetaSlow      *float32                  `json:"yarn_beta_slow"`
	YarnOrigCtx       *int                      `json:"yarn_orig_ctx"`
	SplitMode         *model.SplitMode          `json:"split_mode"`
}

// ApplyTo merges user overrides onto a base model config. Only fields
// explicitly provided by the user (non-nil pointers) are applied.
func (sc SessionConfig) ApplyTo(cfg model.Config) model.Config {
	if sc.ContextWindow != nil {
		cfg.ContextWindow = *sc.ContextWindow
	}
	if sc.NBatch != nil {
		cfg.NBatch = *sc.NBatch
	}
	if sc.NUBatch != nil {
		cfg.NUBatch = *sc.NUBatch
	}
	if sc.NSeqMax != nil {
		cfg.NSeqMax = *sc.NSeqMax
	}
	if sc.FlashAttention != nil {
		cfg.FlashAttention = *sc.FlashAttention
	}
	if sc.CacheTypeK != nil {
		cfg.CacheTypeK = *sc.CacheTypeK
	}
	if sc.CacheTypeV != nil {
		cfg.CacheTypeV = *sc.CacheTypeV
	}
	if sc.NGpuLayers != nil {
		cfg.NGpuLayers = sc.NGpuLayers
	}
	if sc.SystemPromptCache != nil {
		cfg.SystemPromptCache = *sc.SystemPromptCache
	}
	if sc.IncrementalCache != nil {
		cfg.IncrementalCache = *sc.IncrementalCache
	}
	if sc.RopeScaling != nil {
		cfg.RopeScaling = *sc.RopeScaling
	}
	if sc.RopeFreqBase != nil {
		cfg.RopeFreqBase = sc.RopeFreqBase
	}
	if sc.RopeFreqScale != nil {
		cfg.RopeFreqScale = sc.RopeFreqScale
	}
	if sc.YarnExtFactor != nil {
		cfg.YarnExtFactor = sc.YarnExtFactor
	}
	if sc.YarnAttnFactor != nil {
		cfg.YarnAttnFactor = sc.YarnAttnFactor
	}
	if sc.YarnBetaFast != nil {
		cfg.YarnBetaFast = sc.YarnBetaFast
	}
	if sc.YarnBetaSlow != nil {
		cfg.YarnBetaSlow = sc.YarnBetaSlow
	}
	if sc.YarnOrigCtx != nil {
		cfg.YarnOrigCtx = sc.YarnOrigCtx
	}
	if sc.SplitMode != nil {
		cfg.SplitMode = *sc.SplitMode
	}
	return cfg
}

// HasOverrides reports whether any configuration field was explicitly
// provided by the user.
func (sc SessionConfig) HasOverrides() bool {
	return sc.ContextWindow != nil ||
		sc.NBatch != nil ||
		sc.NUBatch != nil ||
		sc.NSeqMax != nil ||
		sc.FlashAttention != nil ||
		sc.CacheTypeK != nil ||
		sc.CacheTypeV != nil ||
		sc.NGpuLayers != nil ||
		sc.SystemPromptCache != nil ||
		sc.IncrementalCache != nil ||
		sc.RopeScaling != nil ||
		sc.RopeFreqBase != nil ||
		sc.RopeFreqScale != nil ||
		sc.YarnExtFactor != nil ||
		sc.YarnAttnFactor != nil ||
		sc.YarnBetaFast != nil ||
		sc.YarnBetaSlow != nil ||
		sc.YarnOrigCtx != nil ||
		sc.SplitMode != nil
}

// HasOverrides reports whether the request contains any config or template
// overrides that require a separate model instance from the Chat path.
func (s SessionRequest) HasOverrides() bool {
	return s.Config.HasOverrides() ||
		s.TemplateMode == "custom" ||
		s.TemplateName != "" ||
		s.TemplateScript != ""
}

// Validate checks the configuration bounds.
func (sc SessionConfig) Validate() error {
	if sc.ContextWindow != nil && (*sc.ContextWindow < 0 || *sc.ContextWindow > 131072) {
		return fmt.Errorf("context-window must be between 0 and 131072, got %d", *sc.ContextWindow)
	}

	if sc.NBatch != nil && (*sc.NBatch < 0 || *sc.NBatch > 16384) {
		return fmt.Errorf("nbatch must be between 0 and 16384, got %d", *sc.NBatch)
	}

	if sc.NUBatch != nil && (*sc.NUBatch < 0 || *sc.NUBatch > 16384) {
		return fmt.Errorf("nubatch must be between 0 and 16384, got %d", *sc.NUBatch)
	}

	if sc.NSeqMax != nil && (*sc.NSeqMax < 0 || *sc.NSeqMax > 64) {
		return fmt.Errorf("nseq-max must be between 0 and 64, got %d", *sc.NSeqMax)
	}

	return nil
}

// =============================================================================

// SessionResponse represents the response from creating a session.
type SessionResponse struct {
	SessionID       string         `json:"session_id"`
	Status          string         `json:"status"`
	EffectiveConfig map[string]any `json:"effective_config"`
}

// Encode implements the encoder interface.
func (s SessionResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(s)
	return data, "application/json", err
}

// =============================================================================

// SessionDeleteResponse represents the response from deleting a session.
type SessionDeleteResponse struct {
	Status string `json:"status"`
}

// Encode implements the encoder interface.
func (s SessionDeleteResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(s)
	return data, "application/json", err
}

// =============================================================================

// TemplateListResponse contains the list of available templates.
type TemplateListResponse struct {
	Templates []TemplateInfo `json:"templates"`
}

// Encode implements the encoder interface.
func (t TemplateListResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(t)
	return data, "application/json", err
}

// TemplateInfo represents information about a template file.
type TemplateInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// =============================================================================

// TemplateContentResponse contains a template's content.
type TemplateContentResponse struct {
	Name   string `json:"name"`
	Script string `json:"script"`
}

// Encode implements the encoder interface.
func (t TemplateContentResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(t)
	return data, "application/json", err
}

// =============================================================================

// TemplateSaveRequest is the request to save a template.
type TemplateSaveRequest struct {
	Name   string `json:"name"`
	Script string `json:"script"`
}

// Decode implements the decoder interface.
func (t *TemplateSaveRequest) Decode(data []byte) error {
	return json.Unmarshal(data, t)
}

// Validate checks the request.
func (t *TemplateSaveRequest) Validate() error {
	if err := validateTemplateName(t.Name); err != nil {
		return err
	}
	if len(t.Script) > 64*1024 {
		return fmt.Errorf("template script too large (max 64KB)")
	}
	return nil
}

func validateTemplateName(name string) error {
	if name == "" {
		return fmt.Errorf("missing template name")
	}
	if len(name) > 255 {
		return fmt.Errorf("template name too long")
	}
	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("invalid template name: %s", name)
	}
	if name[0] == '.' {
		return fmt.Errorf("template name must not start with a dot")
	}
	return nil
}

// TemplateSaveResponse is the response from saving a template.
type TemplateSaveResponse struct {
	Status string `json:"status"`
}

// Encode implements the encoder interface.
func (t TemplateSaveResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(t)
	return data, "application/json", err
}

// =============================================================================

// ChatRequest represents a playground chat request.
type ChatRequest struct {
	SessionID string `json:"session_id"`
}

// Decode implements the decoder interface.
func (c *ChatRequest) Decode(data []byte) error {
	return json.Unmarshal(data, c)
}
