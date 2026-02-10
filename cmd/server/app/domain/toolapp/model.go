package toolapp

import (
	"encoding/json"
	"fmt"
	"strings"
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
	Status       string `json:"status"`
	Arch         string `json:"arch,omitempty"`
	OS           string `json:"os,omitempty"`
	Processor    string `json:"processor,omitempty"`
	Latest       string `json:"latest,omitempty"`
	Current      string `json:"current,omitempty"`
	AllowUpgrade bool   `json:"allow_upgrade"`
}

// Encode implements the encoder interface.
func (app VersionResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toAppVersionTag(status string, vt libs.VersionTag, allowUpgrade bool) VersionResponse {
	return VersionResponse{
		Status:       status,
		Arch:         vt.Arch,
		OS:           vt.OS,
		Processor:    vt.Processor,
		Latest:       vt.Latest,
		Current:      vt.Version,
		AllowUpgrade: allowUpgrade,
	}
}

func toAppVersion(status string, vt libs.VersionTag, allowUpgrade bool) string {
	vi := toAppVersionTag(status, vt, allowUpgrade)

	d, err := json.Marshal(vi)
	if err != nil {
		return fmt.Sprintf("data: {\"Status\":%q}\n", err.Error())
	}

	return fmt.Sprintf("data: %s\n", string(d))
}

// =============================================================================

// ListModelDetail provides information about a model.
type ListModelDetail struct {
	ID          string          `json:"id"`
	Object      string          `json:"object"`
	Created     int64           `json:"created"`
	OwnedBy     string          `json:"owned_by"`
	ModelFamily string          `json:"model_family"`
	Size        int64           `json:"size"`
	Modified    time.Time       `json:"modified"`
	Validated   bool            `json:"validated"`
	Sampling    *SamplingConfig `json:"sampling,omitempty"`
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

func toListModelsInfo(modelFiles []models.File, modelConfigs map[string]catalog.ModelConfig, extendedConfig bool) ListModelInfoResponse {
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

		if extendedConfig {
			if rmc, ok := modelConfigs[mf.ID]; ok {
				detail.Sampling = &SamplingConfig{
					Temperature:     rmc.Sampling.Temperature,
					TopK:            rmc.Sampling.TopK,
					TopP:            rmc.Sampling.TopP,
					MinP:            rmc.Sampling.MinP,
					MaxTokens:       rmc.Sampling.MaxTokens,
					RepeatPenalty:   rmc.Sampling.RepeatPenalty,
					RepeatLastN:     rmc.Sampling.RepeatLastN,
					DryMultiplier:   rmc.Sampling.DryMultiplier,
					DryBase:         rmc.Sampling.DryBase,
					DryAllowedLen:   rmc.Sampling.DryAllowedLen,
					DryPenaltyLast:  rmc.Sampling.DryPenaltyLast,
					XtcProbability:  rmc.Sampling.XtcProbability,
					XtcThreshold:    rmc.Sampling.XtcThreshold,
					XtcMinKeep:      rmc.Sampling.XtcMinKeep,
					EnableThinking:  rmc.Sampling.EnableThinking,
					ReasoningEffort: rmc.Sampling.ReasoningEffort,
				}
			}
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
	Size          int64             `json:"size"`
	HasProjection bool              `json:"has_projection"`
	IsGPT         bool              `json:"is_gpt"`
	Metadata      map[string]string `json:"metadata"`
	VRAM          *VRAM             `json:"vram,omitempty"`
	ModelConfig   *ModelConfig      `json:"model_config,omitempty"`
}

// Encode implements the encoder interface.
func (app ModelInfoResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toModelInfo(fi models.FileInfo, mi models.ModelInfo, rmc catalog.ModelConfig, vram models.VRAM) ModelInfoResponse {
	metadata := make(map[string]string, len(mi.Metadata))
	for k, v := range mi.Metadata {
		metadata[k] = formatMetadataValue(k, v)
	}

	mir := ModelInfoResponse{
		ID:            fi.ID,
		Object:        fi.Object,
		Created:       fi.Created,
		OwnedBy:       fi.OwnedBy,
		Desc:          mi.Desc,
		Size:          fi.Size,
		HasProjection: mi.HasProjection,
		IsGPT:         mi.IsGPTModel,
		Metadata:      metadata,
		ModelConfig: &ModelConfig{
			Device:               rmc.Device,
			Devices:              rmc.Devices,
			ContextWindow:        rmc.ContextWindow,
			NBatch:               rmc.NBatch,
			NUBatch:              rmc.NUBatch,
			NThreads:             rmc.NThreads,
			NThreadsBatch:        rmc.NThreadsBatch,
			CacheTypeK:           rmc.CacheTypeK,
			CacheTypeV:           rmc.CacheTypeV,
			UseDirectIO:          rmc.UseDirectIO,
			FlashAttention:       rmc.FlashAttention,
			IgnoreIntegrityCheck: rmc.IgnoreIntegrityCheck,
			NSeqMax:              rmc.NSeqMax,
			OffloadKQV:           rmc.OffloadKQV,
			OpOffload:            rmc.OpOffload,
			NGpuLayers:           rmc.NGpuLayers,
			SplitMode:            rmc.SplitMode,
			SystemPromptCache:    rmc.SystemPromptCache,
			IncrementalCache:     rmc.IncrementalCache,
			MaxCacheSessions:     rmc.MaxCacheSessions,
			CacheMinTokens:       rmc.CacheMinTokens,
			RopeScaling:          rmc.RopeScaling,
			RopeFreqBase:         rmc.RopeFreqBase,
			RopeFreqScale:        rmc.RopeFreqScale,
			YarnExtFactor:        rmc.YarnExtFactor,
			YarnAttnFactor:       rmc.YarnAttnFactor,
			YarnBetaFast:         rmc.YarnBetaFast,
			YarnBetaSlow:         rmc.YarnBetaSlow,
			YarnOrigCtx:          rmc.YarnOrigCtx,
			Sampling: SamplingConfig{
				Temperature:     rmc.Sampling.Temperature,
				TopK:            rmc.Sampling.TopK,
				TopP:            rmc.Sampling.TopP,
				MinP:            rmc.Sampling.MinP,
				MaxTokens:       rmc.Sampling.MaxTokens,
				RepeatPenalty:   rmc.Sampling.RepeatPenalty,
				RepeatLastN:     rmc.Sampling.RepeatLastN,
				DryMultiplier:   rmc.Sampling.DryMultiplier,
				DryBase:         rmc.Sampling.DryBase,
				DryAllowedLen:   rmc.Sampling.DryAllowedLen,
				DryPenaltyLast:  rmc.Sampling.DryPenaltyLast,
				XtcProbability:  rmc.Sampling.XtcProbability,
				XtcThreshold:    rmc.Sampling.XtcThreshold,
				XtcMinKeep:      rmc.Sampling.XtcMinKeep,
				EnableThinking:  rmc.Sampling.EnableThinking,
				ReasoningEffort: rmc.Sampling.ReasoningEffort,
			},
		},
		VRAM: &VRAM{
			Input: VRAMInput{
				ModelSizeBytes:  vram.Input.ModelSizeBytes,
				ContextWindow:   vram.Input.ContextWindow,
				BlockCount:      vram.Input.BlockCount,
				HeadCountKV:     vram.Input.HeadCountKV,
				KeyLength:       vram.Input.KeyLength,
				ValueLength:     vram.Input.ValueLength,
				BytesPerElement: vram.Input.BytesPerElement,
				Slots:           vram.Input.Slots,
				CacheSequences:  vram.Input.CacheSequences,
			},
			KVPerTokenPerLayer: vram.KVPerTokenPerLayer,
			KVPerSlot:          vram.KVPerSlot,
			TotalSlots:         vram.TotalSlots,
			SlotMemory:         vram.SlotMemory,
			TotalVRAM:          vram.TotalVRAM,
		},
	}

	return mir
}

func formatMetadataValue(key string, value string) string {
	if len(value) < 2 || value[0] != '[' {
		return value
	}

	inner := value[1 : len(value)-1]
	elements := strings.Split(inner, " ")

	if len(elements) <= 6 {
		return value
	}

	if key == "tokenizer.chat_template" {
		return value
	}

	first := elements[:3]

	return fmt.Sprintf("[%s, ...]", strings.Join(first, ", "))
}

// =============================================================================

// ModelDetail provides details for the models in the cache.
type ModelDetail struct {
	ID            string    `json:"id"`
	OwnedBy       string    `json:"owned_by"`
	ModelFamily   string    `json:"model_family"`
	Size          int64     `json:"size"`
	VRAMTotal     int64     `json:"vram_total"`
	SlotMemory    int64     `json:"slot_memory"`
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
			VRAMTotal:     model.VRAMTotal,
			SlotMemory:    model.SlotMemory,
			ExpiresAt:     model.ExpiresAt,
			ActiveStreams: model.ActiveStreams,
		}
	}

	return details
}

// =============================================================================

// VRAMInput contains the input parameters used for VRAM calculation.
type VRAMInput struct {
	ModelSizeBytes  int64 `json:"model_size_bytes"`
	ContextWindow   int64 `json:"context_window"`
	BlockCount      int64 `json:"block_count"`
	HeadCountKV     int64 `json:"head_count_kv"`
	KeyLength       int64 `json:"key_length"`
	ValueLength     int64 `json:"value_length"`
	BytesPerElement int64 `json:"bytes_per_element"`
	Slots           int64 `json:"slots"`
	CacheSequences  int64 `json:"cache_sequences"`
}

// VRAM contains the calculated VRAM requirements.
type VRAM struct {
	Input              VRAMInput `json:"input"`
	KVPerTokenPerLayer int64     `json:"kv_per_token_per_layer"`
	KVPerSlot          int64     `json:"kv_per_slot"`
	TotalSlots         int64     `json:"total_slots"`
	SlotMemory         int64     `json:"slot_memory"`
	TotalVRAM          int64     `json:"total_vram"`
}

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
	Devices              []string                 `json:"devices,omitempty"`
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
	NGpuLayers           *int                     `json:"ngpu-layers"`
	SplitMode            model.SplitMode          `json:"split-mode"`
	SystemPromptCache    bool                     `json:"system-prompt-cache"`
	IncrementalCache     bool                     `json:"incremental-cache"`
	MaxCacheSessions     int                      `json:"max-cache-sessions"`
	CacheMinTokens       int                      `json:"cache-min-tokens"`
	Sampling             SamplingConfig           `json:"sampling-parameters"`
	RopeScaling          model.RopeScalingType    `json:"rope-scaling-type"`
	RopeFreqBase         *float32                 `json:"rope-freq-base"`
	RopeFreqScale        *float32                 `json:"rope-freq-scale"`
	YarnExtFactor        *float32                 `json:"yarn-ext-factor"`
	YarnAttnFactor       *float32                 `json:"yarn-attn-factor"`
	YarnBetaFast         *float32                 `json:"yarn-beta-fast"`
	YarnBetaSlow         *float32                 `json:"yarn-beta-slow"`
	YarnOrigCtx          *int                     `json:"yarn-orig-ctx"`
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
	ID            string              `json:"id"`
	Category      string              `json:"category"`
	OwnedBy       string              `json:"owned_by"`
	ModelFamily   string              `json:"model_family"`
	WebPage       string              `json:"web_page"`
	GatedModel    bool                `json:"gated_model"`
	Template      string              `json:"template"`
	Files         CatalogFiles        `json:"files"`
	Capabilities  CatalogCapabilities `json:"capabilities"`
	Metadata      CatalogMetadata     `json:"metadata"`
	ModelConfig   *ModelConfig        `json:"model_config,omitempty"`
	BaseConfig    *ModelConfig        `json:"base_config,omitempty"`
	ModelMetadata map[string]string   `json:"model_metadata,omitempty"`
	VRAM          *VRAM               `json:"vram,omitempty"`
	Downloaded    bool                `json:"downloaded"`
	Validated     bool                `json:"validated"`
	CatalogFile   string              `json:"catalog_file,omitempty"`
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

func toCatalogModelResponse(catDetails catalog.ModelDetails, rmc *catalog.ModelConfig, metadata map[string]string, vram *models.VRAM) CatalogModelResponse {
	mdls := make([]CatalogFile, len(catDetails.Files.Models))
	for i, model := range catDetails.Files.Models {
		model.URL = models.NormalizeHuggingFaceDownloadURL(model.URL)
		mdls[i] = CatalogFile(model)
	}

	formattedMetadata := make(map[string]string)
	for k, v := range metadata {
		formattedMetadata[k] = formatMetadataValue(k, v)
		if k == "tokenizer.chat_template" {
			if catDetails.Template != "" {
				formattedMetadata[k] = catDetails.Template
			}
		}
	}

	catDetails.Files.Proj.URL = models.NormalizeHuggingFaceDownloadURL(catDetails.Files.Proj.URL)

	resp := CatalogModelResponse{
		ID:          catDetails.ID,
		Category:    catDetails.Category,
		OwnedBy:     catDetails.OwnedBy,
		ModelFamily: catDetails.ModelFamily,
		WebPage:     models.NormalizeHuggingFaceURL(catDetails.WebPage),
		GatedModel:  catDetails.GatedModel,
		Template:    catDetails.Template,
		Files: CatalogFiles{
			Models: mdls,
			Proj:   CatalogFile(catDetails.Files.Proj),
		},
		Capabilities: CatalogCapabilities{
			Endpoint:  catDetails.Capabilities.Endpoint,
			Images:    catDetails.Capabilities.Images,
			Audio:     catDetails.Capabilities.Audio,
			Video:     catDetails.Capabilities.Video,
			Streaming: catDetails.Capabilities.Streaming,
			Reasoning: catDetails.Capabilities.Reasoning,
			Tooling:   catDetails.Capabilities.Tooling,
			Embedding: catDetails.Capabilities.Embedding,
			Rerank:    catDetails.Capabilities.Rerank,
		},
		Metadata: CatalogMetadata{
			Created:     catDetails.Metadata.Created,
			Collections: catDetails.Metadata.Collections,
			Description: catDetails.Metadata.Description,
		},
		ModelMetadata: formattedMetadata,
		Downloaded:    catDetails.Downloaded,
		Validated:     catDetails.Validated,
		CatalogFile:   catDetails.CatalogFile,
	}

	if rmc != nil {
		resp.ModelConfig = &ModelConfig{
			Device:               rmc.Device,
			Devices:              rmc.Devices,
			ContextWindow:        rmc.ContextWindow,
			NBatch:               rmc.NBatch,
			NUBatch:              rmc.NUBatch,
			NThreads:             rmc.NThreads,
			NThreadsBatch:        rmc.NThreadsBatch,
			CacheTypeK:           rmc.CacheTypeK,
			CacheTypeV:           rmc.CacheTypeV,
			UseDirectIO:          rmc.UseDirectIO,
			FlashAttention:       rmc.FlashAttention,
			IgnoreIntegrityCheck: rmc.IgnoreIntegrityCheck,
			NSeqMax:              rmc.NSeqMax,
			OffloadKQV:           rmc.OffloadKQV,
			OpOffload:            rmc.OpOffload,
			NGpuLayers:           rmc.NGpuLayers,
			SplitMode:            rmc.SplitMode,
			SystemPromptCache:    rmc.SystemPromptCache,
			IncrementalCache:     rmc.IncrementalCache,
			MaxCacheSessions:     rmc.MaxCacheSessions,
			CacheMinTokens:       rmc.CacheMinTokens,
			RopeScaling:          rmc.RopeScaling,
			RopeFreqBase:         rmc.RopeFreqBase,
			RopeFreqScale:        rmc.RopeFreqScale,
			YarnExtFactor:        rmc.YarnExtFactor,
			YarnAttnFactor:       rmc.YarnAttnFactor,
			YarnBetaFast:         rmc.YarnBetaFast,
			YarnBetaSlow:         rmc.YarnBetaSlow,
			YarnOrigCtx:          rmc.YarnOrigCtx,
			Sampling: SamplingConfig{
				Temperature:     rmc.Sampling.Temperature,
				TopK:            rmc.Sampling.TopK,
				TopP:            rmc.Sampling.TopP,
				MinP:            rmc.Sampling.MinP,
				MaxTokens:       rmc.Sampling.MaxTokens,
				RepeatPenalty:   rmc.Sampling.RepeatPenalty,
				RepeatLastN:     rmc.Sampling.RepeatLastN,
				DryMultiplier:   rmc.Sampling.DryMultiplier,
				DryBase:         rmc.Sampling.DryBase,
				DryAllowedLen:   rmc.Sampling.DryAllowedLen,
				DryPenaltyLast:  rmc.Sampling.DryPenaltyLast,
				XtcProbability:  rmc.Sampling.XtcProbability,
				XtcThreshold:    rmc.Sampling.XtcThreshold,
				XtcMinKeep:      rmc.Sampling.XtcMinKeep,
				EnableThinking:  rmc.Sampling.EnableThinking,
				ReasoningEffort: rmc.Sampling.ReasoningEffort,
			},
		}
	}

	bmc := catDetails.BaseModelConfig
	resp.BaseConfig = &ModelConfig{
		Device:               bmc.Device,
		Devices:              bmc.Devices,
		ContextWindow:        bmc.ContextWindow,
		NBatch:               bmc.NBatch,
		NUBatch:              bmc.NUBatch,
		NThreads:             bmc.NThreads,
		NThreadsBatch:        bmc.NThreadsBatch,
		CacheTypeK:           bmc.CacheTypeK,
		CacheTypeV:           bmc.CacheTypeV,
		UseDirectIO:          bmc.UseDirectIO,
		FlashAttention:       bmc.FlashAttention,
		IgnoreIntegrityCheck: bmc.IgnoreIntegrityCheck,
		NSeqMax:              bmc.NSeqMax,
		OffloadKQV:           bmc.OffloadKQV,
		OpOffload:            bmc.OpOffload,
		NGpuLayers:           bmc.NGpuLayers,
		SplitMode:            bmc.SplitMode,
		SystemPromptCache:    bmc.SystemPromptCache,
		IncrementalCache:     bmc.IncrementalCache,
		MaxCacheSessions:     bmc.MaxCacheSessions,
		CacheMinTokens:       bmc.CacheMinTokens,
		RopeScaling:          bmc.RopeScaling,
		RopeFreqBase:         bmc.RopeFreqBase,
		RopeFreqScale:        bmc.RopeFreqScale,
		YarnExtFactor:        bmc.YarnExtFactor,
		YarnAttnFactor:       bmc.YarnAttnFactor,
		YarnBetaFast:         bmc.YarnBetaFast,
		YarnBetaSlow:         bmc.YarnBetaSlow,
		YarnOrigCtx:          bmc.YarnOrigCtx,
		Sampling: SamplingConfig{
			Temperature:     bmc.Sampling.Temperature,
			TopK:            bmc.Sampling.TopK,
			TopP:            bmc.Sampling.TopP,
			MinP:            bmc.Sampling.MinP,
			MaxTokens:       bmc.Sampling.MaxTokens,
			RepeatPenalty:   bmc.Sampling.RepeatPenalty,
			RepeatLastN:     bmc.Sampling.RepeatLastN,
			DryMultiplier:   bmc.Sampling.DryMultiplier,
			DryBase:         bmc.Sampling.DryBase,
			DryAllowedLen:   bmc.Sampling.DryAllowedLen,
			DryPenaltyLast:  bmc.Sampling.DryPenaltyLast,
			XtcProbability:  bmc.Sampling.XtcProbability,
			XtcThreshold:    bmc.Sampling.XtcThreshold,
			XtcMinKeep:      bmc.Sampling.XtcMinKeep,
			EnableThinking:  bmc.Sampling.EnableThinking,
			ReasoningEffort: bmc.Sampling.ReasoningEffort,
		},
	}

	if vram != nil {
		resp.VRAM = &VRAM{
			Input: VRAMInput{
				ModelSizeBytes:  vram.Input.ModelSizeBytes,
				ContextWindow:   vram.Input.ContextWindow,
				BlockCount:      vram.Input.BlockCount,
				HeadCountKV:     vram.Input.HeadCountKV,
				KeyLength:       vram.Input.KeyLength,
				ValueLength:     vram.Input.ValueLength,
				BytesPerElement: vram.Input.BytesPerElement,
				Slots:           vram.Input.Slots,
				CacheSequences:  vram.Input.CacheSequences,
			},
			KVPerTokenPerLayer: vram.KVPerTokenPerLayer,
			KVPerSlot:          vram.KVPerSlot,
			TotalSlots:         vram.TotalSlots,
			SlotMemory:         vram.SlotMemory,
			TotalVRAM:          vram.TotalVRAM,
		}
	}

	return resp
}

func toCatalogModelsResponse(list []catalog.ModelDetails) CatalogModelsResponse {
	catalogModels := make([]CatalogModelResponse, len(list))

	for i, model := range list {
		catalogModels[i] = toCatalogModelResponse(model, nil, nil, nil)
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

// =============================================================================

// VRAMRequest represents the input for VRAM calculation.
type VRAMRequest struct {
	ModelURL         string `json:"model_url"`
	ContextWindow    int64  `json:"context_window"`
	BytesPerElement  int64  `json:"bytes_per_element"`
	Slots            int64  `json:"slots"`
	CacheSequences   int64  `json:"cache_sequences"`
	IncrementalCache bool   `json:"incremental_cache"`
}

// Decode implements the decoder interface.
func (app *VRAMRequest) Decode(data []byte) error {
	return json.Unmarshal(data, app)
}

// VRAMResponse represents the VRAM calculation results.
type VRAMResponse struct {
	Input              VRAMInput `json:"input"`
	KVPerTokenPerLayer int64     `json:"kv_per_token_per_layer"`
	KVPerSlot          int64     `json:"kv_per_slot"`
	TotalSlots         int64     `json:"total_slots"`
	SlotMemory         int64     `json:"slot_memory"`
	TotalVRAM          int64     `json:"total_vram"`
}

// Encode implements the encoder interface.
func (app VRAMResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

// =============================================================================

// HFLookupRequest represents the input for a HuggingFace model lookup.
type HFLookupRequest struct {
	Input string `json:"input"`
}

// Decode implements the decoder interface.
func (app *HFLookupRequest) Decode(data []byte) error {
	return json.Unmarshal(data, app)
}

// HFRepoFile represents a GGUF file in a HuggingFace repository.
type HFRepoFile struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	SizeStr  string `json:"size_str"`
}

// HFLookupResponse contains the results from a HuggingFace lookup.
type HFLookupResponse struct {
	Model     CatalogModelResponse `json:"model"`
	RepoFiles []HFRepoFile         `json:"repo_files"`
}

// Encode implements the encoder interface.
func (app HFLookupResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toHFLookupResponse(result catalog.HFLookupResult) HFLookupResponse {
	resp := HFLookupResponse{
		Model: toCatalogModelResponse(result.ModelDetails, nil, nil, nil),
	}

	for _, f := range result.RepoFiles {
		resp.RepoFiles = append(resp.RepoFiles, HFRepoFile{
			Filename: f.Filename,
			Size:     f.Size,
			SizeStr:  f.SizeStr,
		})
	}

	return resp
}

// =============================================================================

// SaveCatalogRequest represents the input for saving a catalog entry.
type SaveCatalogRequest struct {
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
	Config       *ModelConfig        `json:"config,omitempty"`
	CatalogFile  string              `json:"catalog_file"`
}

// Decode implements the decoder interface.
func (app *SaveCatalogRequest) Decode(data []byte) error {
	return json.Unmarshal(data, app)
}

func (app SaveCatalogRequest) toModelDetails() catalog.ModelDetails {
	modelFiles := make([]catalog.File, len(app.Files.Models))
	for i, f := range app.Files.Models {
		modelFiles[i] = catalog.File{URL: f.URL, Size: f.Size}
	}

	md := catalog.ModelDetails{
		ID:          app.ID,
		Category:    app.Category,
		OwnedBy:     app.OwnedBy,
		ModelFamily: app.ModelFamily,
		WebPage:     app.WebPage,
		GatedModel:  app.GatedModel,
		Template:    app.Template,
		Files: catalog.Files{
			Models: modelFiles,
			Proj:   catalog.File{URL: app.Files.Proj.URL, Size: app.Files.Proj.Size},
		},
		Capabilities: catalog.Capabilities{
			Endpoint:  app.Capabilities.Endpoint,
			Images:    app.Capabilities.Images,
			Audio:     app.Capabilities.Audio,
			Video:     app.Capabilities.Video,
			Streaming: app.Capabilities.Streaming,
			Reasoning: app.Capabilities.Reasoning,
			Tooling:   app.Capabilities.Tooling,
			Embedding: app.Capabilities.Embedding,
			Rerank:    app.Capabilities.Rerank,
		},
		Metadata: catalog.Metadata{
			Created:     app.Metadata.Created,
			Collections: app.Metadata.Collections,
			Description: app.Metadata.Description,
		},
	}

	if app.Config != nil {
		md.BaseModelConfig = catalog.ModelConfig{
			Device:               app.Config.Device,
			Devices:              app.Config.Devices,
			ContextWindow:        app.Config.ContextWindow,
			NBatch:               app.Config.NBatch,
			NUBatch:              app.Config.NUBatch,
			NThreads:             app.Config.NThreads,
			NThreadsBatch:        app.Config.NThreadsBatch,
			CacheTypeK:           app.Config.CacheTypeK,
			CacheTypeV:           app.Config.CacheTypeV,
			UseDirectIO:          app.Config.UseDirectIO,
			FlashAttention:       app.Config.FlashAttention,
			IgnoreIntegrityCheck: app.Config.IgnoreIntegrityCheck,
			NSeqMax:              app.Config.NSeqMax,
			OffloadKQV:           app.Config.OffloadKQV,
			OpOffload:            app.Config.OpOffload,
			NGpuLayers:           app.Config.NGpuLayers,
			SplitMode:            app.Config.SplitMode,
			SystemPromptCache:    app.Config.SystemPromptCache,
			IncrementalCache:     app.Config.IncrementalCache,
			MaxCacheSessions:     app.Config.MaxCacheSessions,
			CacheMinTokens:       app.Config.CacheMinTokens,
			InsecureLogging:      false,
			RopeScaling:          app.Config.RopeScaling,
			RopeFreqBase:         app.Config.RopeFreqBase,
			RopeFreqScale:        app.Config.RopeFreqScale,
			YarnExtFactor:        app.Config.YarnExtFactor,
			YarnAttnFactor:       app.Config.YarnAttnFactor,
			YarnBetaFast:         app.Config.YarnBetaFast,
			YarnBetaSlow:         app.Config.YarnBetaSlow,
			YarnOrigCtx:          app.Config.YarnOrigCtx,
			Sampling: catalog.SamplingConfig{
				Temperature:     app.Config.Sampling.Temperature,
				TopK:            app.Config.Sampling.TopK,
				TopP:            app.Config.Sampling.TopP,
				MinP:            app.Config.Sampling.MinP,
				MaxTokens:       app.Config.Sampling.MaxTokens,
				RepeatPenalty:   app.Config.Sampling.RepeatPenalty,
				RepeatLastN:     app.Config.Sampling.RepeatLastN,
				DryMultiplier:   app.Config.Sampling.DryMultiplier,
				DryBase:         app.Config.Sampling.DryBase,
				DryAllowedLen:   app.Config.Sampling.DryAllowedLen,
				DryPenaltyLast:  app.Config.Sampling.DryPenaltyLast,
				XtcProbability:  app.Config.Sampling.XtcProbability,
				XtcThreshold:    app.Config.Sampling.XtcThreshold,
				XtcMinKeep:      app.Config.Sampling.XtcMinKeep,
				EnableThinking:  app.Config.Sampling.EnableThinking,
				ReasoningEffort: app.Config.Sampling.ReasoningEffort,
			},
		}
	}

	return md
}

// SaveCatalogResponse is returned after saving or deleting a catalog entry.
type SaveCatalogResponse struct {
	Status string `json:"status"`
	ID     string `json:"id"`
}

// Encode implements the encoder interface.
func (app SaveCatalogResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

// PublishCatalogRequest represents the input for publishing a catalog file
// to the cloned repository.
type PublishCatalogRequest struct {
	CatalogFile string `json:"catalog_file"`
}

// Decode implements the decoder interface.
func (app *PublishCatalogRequest) Decode(data []byte) error {
	return json.Unmarshal(data, app)
}

// PublishCatalogResponse is returned after publishing a catalog file.
type PublishCatalogResponse struct {
	Status string `json:"status"`
}

// Encode implements the encoder interface.
func (app PublishCatalogResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

// RepoPathResponse is returned for querying the repo path configuration.
type RepoPathResponse struct {
	RepoPath string `json:"repo_path"`
}

// Encode implements the encoder interface.
func (app RepoPathResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

// CatalogFileInfoResponse represents a catalog file.
type CatalogFileInfoResponse struct {
	Name       string `json:"name"`
	ModelCount int    `json:"model_count"`
}

// CatalogFilesListResponse is a list of catalog files.
type CatalogFilesListResponse []CatalogFileInfoResponse

// Encode implements the encoder interface.
func (app CatalogFilesListResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toCatalogFilesResponse(files []catalog.CatalogFileInfo) CatalogFilesListResponse {
	resp := make(CatalogFilesListResponse, len(files))
	for i, f := range files {
		resp[i] = CatalogFileInfoResponse{
			Name:       f.Name,
			ModelCount: f.ModelCount,
		}
	}
	return resp
}
