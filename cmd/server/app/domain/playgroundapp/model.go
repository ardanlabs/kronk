package playgroundapp

import (
	"encoding/json"
	"errors"
	"fmt"

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

	return s.Config.Validate()
}

// SessionConfig represents model configuration overrides.
type SessionConfig struct {
	ContextWindow     int                      `json:"context-window"`
	NBatch            int                      `json:"nbatch"`
	NUBatch           int                      `json:"nubatch"`
	NSeqMax           int                      `json:"nseq-max"`
	FlashAttention    model.FlashAttentionType `json:"flash-attention"`
	CacheTypeK        model.GGMLType           `json:"cache-type-k"`
	CacheTypeV        model.GGMLType           `json:"cache-type-v"`
	NGpuLayers        *int                     `json:"ngpu-layers"`
	SystemPromptCache bool                     `json:"system-prompt-cache"`
	RopeScaling       model.RopeScalingType    `json:"rope-scaling-type"`
	RopeFreqBase      *float32                 `json:"rope-freq-base"`
	RopeFreqScale     *float32                 `json:"rope-freq-scale"`
	YarnExtFactor     *float32                 `json:"yarn-ext-factor"`
	YarnAttnFactor    *float32                 `json:"yarn-attn-factor"`
	YarnBetaFast      *float32                 `json:"yarn-beta-fast"`
	YarnBetaSlow      *float32                 `json:"yarn-beta-slow"`
	YarnOrigCtx       *int                     `json:"yarn-orig-ctx"`
	SplitMode         model.SplitMode          `json:"split-mode"`
}

// ToModelConfig converts the session config to a model.Config.
func (sc SessionConfig) ToModelConfig() model.Config {
	return model.Config{
		ContextWindow:     sc.ContextWindow,
		NBatch:            sc.NBatch,
		NUBatch:           sc.NUBatch,
		NSeqMax:           sc.NSeqMax,
		FlashAttention:    sc.FlashAttention,
		CacheTypeK:        sc.CacheTypeK,
		CacheTypeV:        sc.CacheTypeV,
		NGpuLayers:        sc.NGpuLayers,
		SystemPromptCache: sc.SystemPromptCache,
		RopeScaling:       sc.RopeScaling,
		RopeFreqBase:      sc.RopeFreqBase,
		RopeFreqScale:     sc.RopeFreqScale,
		YarnExtFactor:     sc.YarnExtFactor,
		YarnAttnFactor:    sc.YarnAttnFactor,
		YarnBetaFast:      sc.YarnBetaFast,
		YarnBetaSlow:      sc.YarnBetaSlow,
		YarnOrigCtx:       sc.YarnOrigCtx,
		SplitMode:         sc.SplitMode,
	}
}

// Validate checks the configuration bounds.
func (sc SessionConfig) Validate() error {
	if sc.ContextWindow < 0 || sc.ContextWindow > 131072 {
		return fmt.Errorf("context-window must be between 0 and 131072, got %d", sc.ContextWindow)
	}

	if sc.NBatch < 0 || sc.NBatch > 16384 {
		return fmt.Errorf("nbatch must be between 0 and 16384, got %d", sc.NBatch)
	}

	if sc.NUBatch < 0 || sc.NUBatch > 16384 {
		return fmt.Errorf("nubatch must be between 0 and 16384, got %d", sc.NUBatch)
	}

	if sc.NSeqMax < 0 || sc.NSeqMax > 64 {
		return fmt.Errorf("nseq-max must be between 0 and 64, got %d", sc.NSeqMax)
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
