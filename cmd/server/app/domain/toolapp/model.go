package toolapp

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/cache"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// VersionResponse returns information about the installed libraries.
type VersionResponse struct {
	Status    string `json:"status"`
	Arch      string `json:"arch,omitempty"`
	OS        string `json:"os,omitempty"`
	Processor string `json:"processor,omitempty"`
	Latest    string `json:"latest,omitempty"`
	Current   string `json:"current,omitempty"`
}

// Encode implements the encoder interface.
func (app VersionResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toAppVersionTag(status string, vt libs.VersionTag) VersionResponse {
	return VersionResponse{
		Status:    status,
		Arch:      vt.Arch,
		OS:        vt.OS,
		Processor: vt.Processor,
		Latest:    vt.Latest,
		Current:   vt.Version,
	}
}

func toAppVersion(status string, vt libs.VersionTag) string {
	vi := toAppVersionTag(status, vt)

	d, err := json.Marshal(vi)
	if err != nil {
		return fmt.Sprintf("data: {\"Status\":%q}\n", err.Error())
	}

	return fmt.Sprintf("data: %s\n", string(d))
}

// =============================================================================

// ListModelDetail provides information about a model.
type ListModelDetail struct {
	ID          string    `json:"id"`
	Object      string    `json:"object"`
	Created     int64     `json:"created"`
	OwnedBy     string    `json:"owned_by"`
	ModelFamily string    `json:"model_family"`
	Size        int64     `json:"size"`
	Modified    time.Time `json:"modified"`
	Validated   bool      `json:"validated"`
}

// ListModelInfoResponse contains the list of models loaded in the system.
type ListModelInfoResponse struct {
	Object string            `json:"object"`
	Data   []ListModelDetail `json:"data"`
}

// Encode implements the encoder interface.
func (app ListModelInfoResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toListModelsInfo(modelFiles []models.File, modelConfigs map[string]catalog.ModelConfig) ListModelInfoResponse {
	list := ListModelInfoResponse{
		Object: "list",
	}

	for _, mf := range modelFiles {
		detail := ListModelDetail{
			ID:          mf.ID,
			Object:      "model",
			Created:     mf.Modified.UnixMilli(),
			OwnedBy:     mf.OwnedBy,
			ModelFamily: mf.ModelFamily,
			Size:        mf.Size,
			Modified:    mf.Modified,
			Validated:   mf.Validated,
		}

		list.Data = append(list.Data, detail)
	}

	return list
}

// =============================================================================

// PullRequest represents the input for the pull command.
type PullRequest struct {
	ModelURL string `json:"model_url"`
	ProjURL  string `json:"proj_url"`
}

// Decode implements the decoder interface.
func (app *PullRequest) Decode(data []byte) error {
	return json.Unmarshal(data, app)
}

// PullResponse returns information about a model being downloaded.
type PullResponse struct {
	Status     string   `json:"status"`
	ModelFiles []string `json:"model_files,omitempty"`
	ProjFile   string   `json:"proj_file,omitempty"`
	Downloaded bool     `json:"downloaded,omitempty"`
}

// Encode implements the encoder interface.
func (app PullResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toAppPull(status string, mp models.Path) string {
	pr := PullResponse{
		Status:     status,
		ModelFiles: mp.ModelFiles,
		ProjFile:   mp.ProjFile,
		Downloaded: mp.Downloaded,
	}

	d, err := json.Marshal(pr)
	if err != nil {
		return fmt.Sprintf("data: {\"Status\":%q}\n", err.Error())
	}

	return fmt.Sprintf("data: %s\n", string(d))
}

// =============================================================================

// ModelInfoResponse returns information about a model.
type ModelInfoResponse struct {
	ID            string            `json:"id"`
	Object        string            `json:"object"`
	Created       int64             `json:"created"`
	OwnedBy       string            `json:"owned_by"`
	Desc          string            `json:"desc"`
	Size          uint64            `json:"size"`
	HasProjection bool              `json:"has_projection"`
	HasEncoder    bool              `json:"has_encoder"`
	HasDecoder    bool              `json:"has_decoder"`
	IsRecurrent   bool              `json:"is_recurrent"`
	IsHybrid      bool              `json:"is_hybrid"`
	IsGPT         bool              `json:"is_gpt"`
	Metadata      map[string]string `json:"metadata"`
	ModelConfig   *ModelConfig      `json:"model_config,omitempty"`
}

// Encode implements the encoder interface.
func (app ModelInfoResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toModelInfo(info models.Info, mi model.ModelInfo, mc catalog.ModelConfig) ModelInfoResponse {
	modelConfig := &ModelConfig{
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
		Sampling: SamplingConfig{
			Temperature:     mc.Sampling.Temperature,
			TopK:            mc.Sampling.TopK,
			TopP:            mc.Sampling.TopP,
			MinP:            mc.Sampling.MinP,
			MaxTokens:       mc.Sampling.MaxTokens,
			RepeatPenalty:   mc.Sampling.RepeatPenalty,
			RepeatLastN:     mc.Sampling.RepeatLastN,
			DryMultiplier:   mc.Sampling.DryMultiplier,
			DryBase:         mc.Sampling.DryBase,
			DryAllowedLen:   mc.Sampling.DryAllowedLen,
			DryPenaltyLast:  mc.Sampling.DryPenaltyLast,
			XtcProbability:  mc.Sampling.XtcProbability,
			XtcThreshold:    mc.Sampling.XtcThreshold,
			XtcMinKeep:      mc.Sampling.XtcMinKeep,
			EnableThinking:  mc.Sampling.EnableThinking,
			ReasoningEffort: mc.Sampling.ReasoningEffort,
		},
	}

	mir := ModelInfoResponse{
		ID:            info.ID,
		Object:        info.Object,
		Created:       info.Created,
		OwnedBy:       info.OwnedBy,
		Desc:          mi.Desc,
		Size:          mi.Size,
		HasProjection: mi.HasProjection,
		HasEncoder:    mi.HasEncoder,
		HasDecoder:    mi.HasDecoder,
		IsRecurrent:   mi.IsRecurrent,
		IsHybrid:      mi.IsHybrid,
		IsGPT:         mi.IsGPTModel,
		Metadata:      mi.Metadata,
		ModelConfig:   modelConfig,
	}

	return mir
}

// =============================================================================

// ModelDetail provides details for the models in the cache.
type ModelDetail struct {
	ID            string    `json:"id"`
	OwnedBy       string    `json:"owned_by"`
	ModelFamily   string    `json:"model_family"`
	Size          int64     `json:"size"`
	ExpiresAt     time.Time `json:"expires_at"`
	ActiveStreams int       `json:"active_streams"`
}

// ModelDetailsResponse is a collection of model detail.
type ModelDetailsResponse []ModelDetail

// Encode implements the encoder interface.
func (app ModelDetailsResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toModelDetails(models []cache.ModelDetail) ModelDetailsResponse {
	details := make(ModelDetailsResponse, len(models))

	for i, model := range models {
		details[i] = ModelDetail{
			ID:            model.ID,
			OwnedBy:       model.OwnedBy,
			ModelFamily:   model.ModelFamily,
			Size:          model.Size,
			ExpiresAt:     model.ExpiresAt,
			ActiveStreams: model.ActiveStreams,
		}
	}

	return details
}

// =============================================================================

// SamplingConfig represents sampling parameters for model inference.
type SamplingConfig struct {
	Temperature     float32 `json:"temperature"`
	TopK            int32   `json:"top_k"`
	TopP            float32 `json:"top_p"`
	MinP            float32 `json:"min_p"`
	MaxTokens       int     `json:"max_tokens"`
	RepeatPenalty   float32 `json:"repeat_penalty"`
	RepeatLastN     int32   `json:"repeat_last_n"`
	DryMultiplier   float32 `json:"dry_multiplier"`
	DryBase         float32 `json:"dry_base"`
	DryAllowedLen   int32   `json:"dry_allowed_length"`
	DryPenaltyLast  int32   `json:"dry_penalty_last_n"`
	XtcProbability  float32 `json:"xtc_probability"`
	XtcThreshold    float32 `json:"xtc_threshold"`
	XtcMinKeep      uint32  `json:"xtc_min_keep"`
	EnableThinking  string  `json:"enable_thinking"`
	ReasoningEffort string  `json:"reasoning_effort"`
}

// ModelConfig represents the model configuration the model will use by default.
type ModelConfig struct {
	Device               string                   `json:"device"`
	ContextWindow        int                      `json:"context-window"`
	NBatch               int                      `json:"nbatch"`
	NUBatch              int                      `json:"nubatch"`
	NThreads             int                      `json:"nthreads"`
	NThreadsBatch        int                      `json:"nthreads-batch"`
	CacheTypeK           model.GGMLType           `json:"cache-type-k"`
	CacheTypeV           model.GGMLType           `json:"cache-type-v"`
	UseDirectIO          bool                     `json:"use-direct-io"`
	FlashAttention       model.FlashAttentionType `json:"flash-attention"`
	IgnoreIntegrityCheck bool                     `json:"ignore-integrity-check"`
	NSeqMax              int                      `json:"nseq-max"`
	OffloadKQV           *bool                    `json:"offload-kqv"`
	OpOffload            *bool                    `json:"op-offload"`
	NGpuLayers           *int32                   `json:"ngpu-layers"`
	SplitMode            model.SplitMode          `json:"split-mode"`
	SystemPromptCache    bool                     `json:"system-prompt-cache"`
	FirstMessageCache    bool                     `json:"first-message-cache"`
	CacheMinTokens       int                      `json:"cache-min-tokens"`
	Sampling             SamplingConfig           `json:"sampling-parameters"`
}

// CatalogMetadata represents extra information about the model.
type CatalogMetadata struct {
	Created     time.Time `json:"created"`
	Collections string    `json:"collections"`
	Description string    `json:"description"`
}

// CatalogCapabilities represents the capabilities of a model.
type CatalogCapabilities struct {
	Endpoint  string `json:"endpoint"`
	Images    bool   `json:"images"`
	Audio     bool   `json:"audio"`
	Video     bool   `json:"video"`
	Streaming bool   `json:"streaming"`
	Reasoning bool   `json:"reasoning"`
	Tooling   bool   `json:"tooling"`
	Embedding bool   `json:"embedding"`
	Rerank    bool   `json:"rerank"`
}

// CatalogFile represents the actual file url and size.
type CatalogFile struct {
	URL  string `json:"url"`
	Size string `json:"size"`
}

// CatalogFiles represents file information for a model.
type CatalogFiles struct {
	Models []CatalogFile `json:"model"`
	Proj   CatalogFile   `json:"proj"`
}

// CatalogModelResponse represents information for a model.
type CatalogModelResponse struct {
	ID           string              `json:"id"`
	Category     string              `json:"category"`
	OwnedBy      string              `json:"owned_by"`
	ModelFamily  string              `json:"model_family"`
	WebPage      string              `json:"web_page"`
	GatedModel   bool                `json:"gated_model"`
	Template     string              `json:"template"`
	Files        CatalogFiles        `json:"files"`
	Capabilities CatalogCapabilities `json:"capabilities"`
	Metadata     CatalogMetadata     `json:"metadata"`
	ModelConfig  *ModelConfig        `json:"model_config,omitempty"`
	Downloaded   bool                `json:"downloaded"`
	Validated    bool                `json:"validated"`
}

// Encode implements the encoder interface.
func (app CatalogModelResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

// CatalogModelsResponse represents a list of catalog models.
type CatalogModelsResponse []CatalogModelResponse

// Encode implements the encoder interface.
func (app CatalogModelsResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toCatalogModelResponse(model catalog.Model, mc *catalog.ModelConfig) CatalogModelResponse {
	models := make([]CatalogFile, len(model.Files.Models))
	for i, model := range model.Files.Models {
		models[i] = CatalogFile(model)
	}

	resp := CatalogModelResponse{
		ID:          model.ID,
		Category:    model.Category,
		OwnedBy:     model.OwnedBy,
		ModelFamily: model.ModelFamily,
		WebPage:     model.WebPage,
		GatedModel:  model.GatedModel,
		Template:    model.Template,
		Files: CatalogFiles{
			Models: models,
			Proj:   CatalogFile(model.Files.Proj),
		},
		Capabilities: CatalogCapabilities{
			Endpoint:  model.Capabilities.Endpoint,
			Images:    model.Capabilities.Images,
			Audio:     model.Capabilities.Audio,
			Video:     model.Capabilities.Video,
			Streaming: model.Capabilities.Streaming,
			Reasoning: model.Capabilities.Reasoning,
			Tooling:   model.Capabilities.Tooling,
			Embedding: model.Capabilities.Embedding,
			Rerank:    model.Capabilities.Rerank,
		},
		Metadata: CatalogMetadata{
			Created:     model.Metadata.Created,
			Collections: model.Metadata.Collections,
			Description: model.Metadata.Description,
		},
		Downloaded: model.Downloaded,
		Validated:  model.Validated,
	}

	if mc != nil {
		resp.ModelConfig = &ModelConfig{
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
			Sampling: SamplingConfig{
				Temperature:     mc.Sampling.Temperature,
				TopK:            mc.Sampling.TopK,
				TopP:            mc.Sampling.TopP,
				MinP:            mc.Sampling.MinP,
				MaxTokens:       mc.Sampling.MaxTokens,
				RepeatPenalty:   mc.Sampling.RepeatPenalty,
				RepeatLastN:     mc.Sampling.RepeatLastN,
				DryMultiplier:   mc.Sampling.DryMultiplier,
				DryBase:         mc.Sampling.DryBase,
				DryAllowedLen:   mc.Sampling.DryAllowedLen,
				DryPenaltyLast:  mc.Sampling.DryPenaltyLast,
				XtcProbability:  mc.Sampling.XtcProbability,
				XtcThreshold:    mc.Sampling.XtcThreshold,
				XtcMinKeep:      mc.Sampling.XtcMinKeep,
				EnableThinking:  mc.Sampling.EnableThinking,
				ReasoningEffort: mc.Sampling.ReasoningEffort,
			},
		}
	}

	return resp
}

func toCatalogModelsResponse(list []catalog.Model) CatalogModelsResponse {
	catalogModels := make([]CatalogModelResponse, len(list))

	for i, model := range list {
		catalogModels[i] = toCatalogModelResponse(model, nil)
	}

	return catalogModels
}

// =============================================================================

// KeyResponse represents a key in the system.
type KeyResponse struct {
	ID      string `json:"id"`
	Created string `json:"created"`
}

// KeysResponse is a collection of keys.
type KeysResponse []KeyResponse

// Encode implements the encoder interface.
func (app KeysResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toKeys(keys []authclient.Key) KeysResponse {
	keyResponse := make([]KeyResponse, len(keys))

	for i, key := range keys {
		keyResponse[i] = KeyResponse{
			ID:      key.ID,
			Created: key.Created,
		}
	}

	return keyResponse
}

// =============================================================================

// RateLimit defines the rate limit configuration for an endpoint.
type RateLimit struct {
	Limit  int    `json:"limit"`
	Window string `json:"window"`
}

// TokenRequest represents the input for the create token command.
type TokenRequest struct {
	Admin     bool                 `json:"admin"`
	Endpoints map[string]RateLimit `json:"endpoints"`
	Duration  time.Duration        `json:"duration"`
}

// Decode implements the decoder interface.
func (app *TokenRequest) Decode(data []byte) error {
	return json.Unmarshal(data, app)
}

// TokenResponse represents the response for a successful token creation.
type TokenResponse struct {
	Token string `json:"token"`
}

// Encode implements the encoder interface.
func (app TokenResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}
