package toolapp

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	buckypool "github.com/ardanlabs/kronk/sdk/bucky/pool"
	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/kvstorage"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/vram"
	"github.com/ardanlabs/kronk/sdk/pool"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
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
	ID                   string          `json:"id"`
	Object               string          `json:"object"`
	Created              int64           `json:"created"`
	OwnedBy              string          `json:"owned_by"`
	ModelFamily          string          `json:"model_family"`
	TokenizerFingerprint string          `json:"tokenizer_fingerprint,omitempty"`
	Size                 int64           `json:"size"`
	Modified             time.Time       `json:"modified"`
	Validated            bool            `json:"validated"`
	HasProjection        bool            `json:"has_projection"`
	Sampling             *SamplingConfig `json:"sampling,omitempty"`
	DraftModelID         string          `json:"draft_model_id,omitempty"`
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

func toListModelsInfo(modelFiles []models.File, modelConfigs map[string]models.ModelConfig, extendedConfig bool) ListModelInfoResponse {
	list := ListModelInfoResponse{
		Object: "list",
	}

	for _, mf := range modelFiles {
		detail := ListModelDetail{
			ID:                   mf.ID,
			Object:               "model",
			Created:              mf.Modified.UnixMilli(),
			OwnedBy:              mf.OwnedBy,
			ModelFamily:          mf.ModelFamily,
			TokenizerFingerprint: mf.TokenizerFingerprint,
			Size:                 mf.Size,
			Modified:             mf.Modified,
			Validated:            mf.Validated,
			HasProjection:        mf.HasProjection,
		}

		if extendedConfig {
			if rmc, ok := modelConfigs[mf.ID]; ok {
				detail.Sampling = &SamplingConfig{
					Temperature:      rmc.Sampling.Temperature,
					TopK:             rmc.Sampling.TopK,
					TopP:             rmc.Sampling.TopP,
					MinP:             rmc.Sampling.MinP,
					MaxTokens:        rmc.Sampling.MaxTokens,
					RepeatPenalty:    rmc.Sampling.RepeatPenalty,
					RepeatLastN:      rmc.Sampling.RepeatLastN,
					DryMultiplier:    rmc.Sampling.DryMultiplier,
					DryBase:          rmc.Sampling.DryBase,
					DryAllowedLen:    rmc.Sampling.DryAllowedLen,
					DryPenaltyLast:   rmc.Sampling.DryPenaltyLast,
					XtcProbability:   rmc.Sampling.XtcProbability,
					XtcThreshold:     rmc.Sampling.XtcThreshold,
					XtcMinKeep:       rmc.Sampling.XtcMinKeep,
					FrequencyPenalty: rmc.Sampling.FrequencyPenalty,
					PresencePenalty:  rmc.Sampling.PresencePenalty,
					EnableThinking:   rmc.Sampling.EnableThinking,
					ReasoningEffort:  rmc.Sampling.ReasoningEffort,
					Grammar:          rmc.Sampling.Grammar,
				}
				if rmc.DraftModel != nil && rmc.DraftModel.ModelID != "" {
					detail.DraftModelID = rmc.DraftModel.ModelID
				}
			}
		}

		list.Data = append(list.Data, detail)
	}

	slices.SortFunc(list.Data, func(a, b ListModelDetail) int {
		return strings.Compare(strings.ToLower(a.ID), strings.ToLower(b.ID))
	})

	return list
}

// =============================================================================

// ModelIntegrityArtifact provides integrity evidence for one physical model
// artifact.
type ModelIntegrityArtifact struct {
	Role       string                 `json:"role"`
	Filename   string                 `json:"filename"`
	Digest     string                 `json:"digest,omitempty"`
	Size       int64                  `json:"size"`
	Status     models.IntegrityStatus `json:"status"`
	Verified   bool                   `json:"verified"`
	VerifiedAt *time.Time             `json:"verified_at,omitempty"`
	Reason     string                 `json:"reason,omitempty"`
}

// ModelIntegrityDetail provides integrity information for one indexed model.
type ModelIntegrityDetail struct {
	ID          string                   `json:"id"`
	OwnedBy     string                   `json:"owned_by"`
	ModelFamily string                   `json:"model_family"`
	Status      models.IntegrityStatus   `json:"status"`
	Verified    bool                     `json:"verified"`
	Artifacts   []ModelIntegrityArtifact `json:"artifacts"`
}

// Encode implements the encoder interface.
func (app ModelIntegrityDetail) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

// ModelIntegrityResponse contains the integrity inventory for locally
// installed models.
type ModelIntegrityResponse struct {
	Object string                 `json:"object"`
	Data   []ModelIntegrityDetail `json:"data"`
}

// Encode implements the encoder interface.
func (app ModelIntegrityResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toModelIntegrity(modelIntegrity []models.IntegrityModel) ModelIntegrityResponse {
	response := ModelIntegrityResponse{
		Object: "model_integrity.list",
		Data:   make([]ModelIntegrityDetail, 0, len(modelIntegrity)),
	}

	for _, integrity := range modelIntegrity {
		response.Data = append(response.Data, toModelIntegrityDetail(integrity))
	}

	return response
}

func toModelIntegrityDetail(integrity models.IntegrityModel) ModelIntegrityDetail {
	detail := ModelIntegrityDetail{
		ID:          integrity.ID,
		OwnedBy:     integrity.OwnedBy,
		ModelFamily: integrity.ModelFamily,
		Status:      integrity.Status,
		Verified:    integrity.Verified,
		Artifacts:   make([]ModelIntegrityArtifact, 0, len(integrity.Artifacts)),
	}

	for _, artifact := range integrity.Artifacts {
		appArtifact := ModelIntegrityArtifact{
			Role:     artifact.Role,
			Filename: artifact.Filename,
			Digest:   artifact.Digest,
			Size:     artifact.Size,
			Status:   artifact.Status,
			Verified: artifact.Verified,
			Reason:   artifact.Reason,
		}
		if !artifact.VerifiedAt.IsZero() {
			appArtifact.VerifiedAt = &artifact.VerifiedAt
		}

		detail.Artifacts = append(detail.Artifacts, appArtifact)
	}

	return detail
}

// =============================================================================

// OpenAIModel is a single entry in the OpenAI-compatible /v1/models list.
type OpenAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// Encode implements the encoder interface.
func (app OpenAIModel) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

// OpenAIModelsResponse is the OpenAI-compatible response for GET /v1/models.
// Apps like OpenWebUI call this endpoint to discover available models.
type OpenAIModelsResponse struct {
	Object string        `json:"object"`
	Data   []OpenAIModel `json:"data"`
}

// Encode implements the encoder interface.
func (app OpenAIModelsResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toOpenAIModels(modelFiles []models.File) OpenAIModelsResponse {
	resp := OpenAIModelsResponse{
		Object: "list",
		Data:   make([]OpenAIModel, 0, len(modelFiles)),
	}

	for _, mf := range modelFiles {
		model := toOpenAIModel(mf)
		if !strings.Contains(model.ID, "/") {
			model.ID = model.OwnedBy + "/" + model.ID
		}
		resp.Data = append(resp.Data, model)
	}

	slices.SortFunc(resp.Data, func(a, b OpenAIModel) int {
		return strings.Compare(strings.ToLower(a.ID), strings.ToLower(b.ID))
	})

	return resp
}

func toOpenAIModel(mf models.File) OpenAIModel {
	ownedBy := mf.OwnedBy
	if ownedBy == "" {
		ownedBy = "kronk"
	}

	return OpenAIModel{
		ID:      mf.ID,
		Object:  "model",
		Created: mf.Modified.Unix(),
		OwnedBy: ownedBy,
	}
}

// =============================================================================

// PullRequest represents the input for the pull command. ModelURL
// accepts a direct HuggingFace URL, an owner/repo/file.gguf path, or a
// canonical catalog id (e.g. "unsloth/Qwen3-8B-Q8_0").
//
// DownloadServer, when set, redirects the pull to a peer Kronk server
// on the local network ("host:port"). The peer must be running with the
// download endpoint enabled. Useful in classroom/workshop settings
// where Internet access is slow or unreliable.
type PullRequest struct {
	ModelURL       string `json:"model_url"`
	ProjURL        string `json:"proj_url"`
	MTPURL         string `json:"mtp_url"`
	DownloadServer string `json:"download_server"`
}

// Decode implements the decoder interface.
func (app *PullRequest) Decode(data []byte) error {
	return json.Unmarshal(data, app)
}

// PullMeta contains metadata about a model download.
type PullMeta struct {
	ModelURL  string `json:"model_url,omitempty"`
	ProjURL   string `json:"proj_url,omitempty"`
	MTPURL    string `json:"mtp_url,omitempty"`
	ModelID   string `json:"model_id,omitempty"`
	FileIndex int    `json:"file_index,omitempty"`
	FileTotal int    `json:"file_total,omitempty"`
}

// PullProgress contains structured progress data for a file download.
type PullProgress struct {
	Src          string  `json:"src,omitempty"`
	CurrentBytes int64   `json:"current_bytes,omitempty"`
	TotalBytes   int64   `json:"total_bytes,omitempty"`
	MBPerSec     float64 `json:"mb_per_sec,omitempty"`
	Complete     bool    `json:"complete,omitempty"`
}

// PullResponse returns information about a model being downloaded.
type PullResponse struct {
	Status     string        `json:"status"`
	ModelFiles []string      `json:"model_files,omitempty"`
	ProjFile   string        `json:"proj_file,omitempty"`
	MTPFile    string        `json:"mtp_file,omitempty"`
	Downloaded bool          `json:"downloaded,omitempty"`
	Meta       *PullMeta     `json:"meta,omitempty"`
	Progress   *PullProgress `json:"progress,omitempty"`
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
		MTPFile:    mp.MTPFile,
		Downloaded: mp.Downloaded,
	}

	d, err := json.Marshal(pr)
	if err != nil {
		return fmt.Sprintf("data: {\"Status\":%q}\n", err.Error())
	}

	return fmt.Sprintf("data: %s\n", string(d))
}

func toAppPullResponse(pr PullResponse) string {
	d, err := json.Marshal(pr)
	if err != nil {
		return fmt.Sprintf("data: {\"status\":%q}\n", err.Error())
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
	FileType      int64             `json:"file_type"`
	Quantization  string            `json:"quantization"`
	HasProjection bool              `json:"has_projection"`
	Template      string            `json:"template"`
	Metadata      map[string]string `json:"metadata"`
	ModelConfig   *ModelConfig      `json:"model_config,omitempty"`
	Vram          *VRAMResponse     `json:"vram,omitempty"`
}

// Encode implements the encoder interface.
func (app ModelInfoResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toModelInfo(fi models.FileInfo, mi models.ModelInfo, rmc models.ModelConfig, vramResp *VRAMResponse) ModelInfoResponse {
	metadata := make(map[string]string, len(mi.Metadata))
	for k, v := range mi.Metadata {
		metadata[k] = formatMetadataValue(k, v)
	}

	nSeqMax := 1
	if rmc.PtrNSeqMax != nil {
		nSeqMax = max(*rmc.PtrNSeqMax, 1)
	}

	queueDepth := model.DefaultQueueDepth
	if rmc.PtrQueueDepth != nil && *rmc.PtrQueueDepth != 0 {
		queueDepth = *rmc.PtrQueueDepth
	}

	admissionCapacity := nSeqMax * queueDepth
	if mi.IsEmbedModel || mi.IsRerankModel {
		queueDepth = 1
		admissionCapacity = nSeqMax
	}

	mir := ModelInfoResponse{
		ID:            fi.ID,
		Object:        fi.Object,
		Created:       fi.Created,
		OwnedBy:       fi.OwnedBy,
		Desc:          mi.Desc,
		Size:          fi.Size,
		FileType:      mi.FileType,
		Quantization:  mi.Quantization,
		HasProjection: mi.HasProjection,
		Template:      rmc.Template,
		Metadata:      metadata,
		ModelConfig: &ModelConfig{
			PtrContextWindow:      rmc.PtrContextWindow,
			PtrPrefillBatchSize:   rmc.PtrPrefillBatchSize,
			PtrNThreads:           rmc.PtrNThreads,
			PtrNThreadsBatch:      rmc.PtrNThreadsBatch,
			CacheTypeK:            rmc.CacheTypeK,
			CacheTypeV:            rmc.CacheTypeV,
			LoadMode:              model.DerefLoadMode(rmc.PtrLoadMode),
			NUMA:                  rmc.NUMA,
			FlashAttention:        model.DerefFlashAttention(rmc.FlashAttention),
			PtrNSeqMax:            rmc.PtrNSeqMax,
			QueueDepth:            queueDepth,
			AdmissionCapacity:     admissionCapacity,
			PtrIMCSessionCapacity: rmc.PtrIMCSessionCapacity,
			PtrOffloadKQV:         rmc.PtrOffloadKQV,
			PtrOpOffload:          rmc.PtrOpOffload,
			PtrProjOnCPU:          rmc.PtrProjOnCPU,
			ProjDevice:            rmc.ProjDevice,
			PtrNGpuLayers:         rmc.PtrNGpuLayers,
			PtrSplitMode:          rmc.PtrSplitMode,
			TensorSplit:           rmc.TensorSplit,
			TensorBuftOverrides:   rmc.TensorBuftOverrides,
			PtrMainGPU:            rmc.PtrMainGPU,
			Devices:               rmc.Devices,
			MoE:                   toAppMoEConfig(rmc.MoE),
			PtrSWAFull:            rmc.PtrSWAFull,
			PtrIncrementalCache:   rmc.PtrIncrementalCache,
			PtrCacheMinTokens:     rmc.PtrCacheMinTokens,
			SessionStoreKind:      rmc.SessionStoreKind,
			RopeScaling:           rmc.RopeScaling,
			PtrRopeFreqBase:       rmc.PtrRopeFreqBase,
			PtrRopeFreqScale:      rmc.PtrRopeFreqScale,
			PtrYarnExtFactor:      rmc.PtrYarnExtFactor,
			PtrYarnAttnFactor:     rmc.PtrYarnAttnFactor,
			PtrYarnBetaFast:       rmc.PtrYarnBetaFast,
			PtrYarnBetaSlow:       rmc.PtrYarnBetaSlow,
			PtrYarnOrigCtx:        rmc.PtrYarnOrigCtx,
			Sampling: SamplingConfig{
				Temperature:      rmc.Sampling.Temperature,
				TopK:             rmc.Sampling.TopK,
				TopP:             rmc.Sampling.TopP,
				MinP:             rmc.Sampling.MinP,
				MaxTokens:        rmc.Sampling.MaxTokens,
				RepeatPenalty:    rmc.Sampling.RepeatPenalty,
				RepeatLastN:      rmc.Sampling.RepeatLastN,
				DryMultiplier:    rmc.Sampling.DryMultiplier,
				DryBase:          rmc.Sampling.DryBase,
				DryAllowedLen:    rmc.Sampling.DryAllowedLen,
				DryPenaltyLast:   rmc.Sampling.DryPenaltyLast,
				XtcProbability:   rmc.Sampling.XtcProbability,
				XtcThreshold:     rmc.Sampling.XtcThreshold,
				XtcMinKeep:       rmc.Sampling.XtcMinKeep,
				FrequencyPenalty: rmc.Sampling.FrequencyPenalty,
				PresencePenalty:  rmc.Sampling.PresencePenalty,
				EnableThinking:   rmc.Sampling.EnableThinking,
				ReasoningEffort:  rmc.Sampling.ReasoningEffort,
			},
		},
		Vram: vramResp,
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
//
// Backend distinguishes kronk (llama.cpp) entries from bucky
// (whisper) entries so the BUI can label rows and route Unload to the
// right pool.
type ModelDetail struct {
	ID            string    `json:"id"`
	Backend       string    `json:"backend"`
	OwnedBy       string    `json:"owned_by"`
	ModelFamily   string    `json:"model_family"`
	Size          int64     `json:"size"`
	VRAMTotal     int64     `json:"vram_total"`
	KVCache       int64     `json:"kv_cache"`
	Slots         int       `json:"slots"`
	MTPNDraft     int       `json:"mtp_ndraft,omitempty"`
	ExpiresAt     time.Time `json:"expires_at"`
	ActiveStreams int       `json:"active_streams"`
	Status        string    `json:"status"`
}

// ModelDetailsResponse is a collection of model detail.
type ModelDetailsResponse []ModelDetail

// Encode implements the encoder interface.
func (app ModelDetailsResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toModelDetails(models []pool.ModelDetail) ModelDetailsResponse {
	details := make(ModelDetailsResponse, len(models))

	for i, model := range models {
		details[i] = ModelDetail{
			ID:            model.ID,
			Backend:       model.Backend,
			OwnedBy:       model.OwnedBy,
			ModelFamily:   model.ModelFamily,
			Size:          model.Size,
			VRAMTotal:     model.VRAMTotal,
			KVCache:       model.KVCache,
			Slots:         model.Slots,
			MTPNDraft:     model.MTPNDraft,
			ExpiresAt:     model.ExpiresAt,
			ActiveStreams: model.ActiveStreams,
			Status:        model.Status,
		}
	}

	return details
}

// IMCSessionDetail provides the current state of one allocated IMC cache
// entry.
type IMCSessionDetail struct {
	ModelID        string    `json:"model_id"`
	ID             int       `json:"id"`
	State          string    `json:"state"`
	Context        int       `json:"context"`
	Allocated      int       `json:"allocated"`
	TotalAllocated int       `json:"total_allocated"`
	SnapshotBytes  int       `json:"snapshot_bytes"`
	PeakContext    int       `json:"peak_context"`
	Messages       int       `json:"messages"`
	InputMessages  int       `json:"input_messages"`
	InputTokens    int       `json:"input_tokens"`
	OutputTokens   int       `json:"output_tokens"`
	ContextWindow  int       `json:"context_window"`
	LastUsed       time.Time `json:"last_used"`
	HasMedia       bool      `json:"has_media"`
}

// IMCSessionsResponse is the current set of allocated IMC cache entries.
type IMCSessionsResponse []IMCSessionDetail

// Encode implements the encoder interface.
func (app IMCSessionsResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toIMCSessions(sessions []pool.IMCSessionDetail) IMCSessionsResponse {
	details := make(IMCSessionsResponse, len(sessions))

	for i, session := range sessions {
		details[i] = IMCSessionDetail{
			ModelID:        session.ModelID,
			ID:             session.ID,
			State:          string(session.State),
			Context:        session.Context,
			Allocated:      session.Allocated,
			TotalAllocated: session.TotalAllocated,
			SnapshotBytes:  session.SnapshotBytes,
			PeakContext:    session.PeakContext,
			Messages:       session.Messages,
			InputMessages:  session.InputMessages,
			InputTokens:    session.InputTokens,
			OutputTokens:   session.OutputTokens,
			ContextWindow:  session.ContextWindow,
			LastUsed:       session.LastUsed,
			HasMedia:       session.HasMedia,
		}
	}

	return details
}

// IMCSystemCacheDetail provides one immutable System preload entry.
type IMCSystemCacheDetail struct {
	ModelID        string    `json:"model_id"`
	ID             int       `json:"id"`
	Tokens         int       `json:"tokens"`
	Allocated      int       `json:"allocated"`
	SnapshotBytes  int       `json:"snapshot_bytes"`
	RestoreCount   uint64    `json:"restore_count"`
	ActiveRestores int       `json:"active_restores"`
	LastUsed       time.Time `json:"last_used"`
}

// IMCSystemCachesResponse is the current set of System cache pool entries.
type IMCSystemCachesResponse []IMCSystemCacheDetail

// Encode implements the encoder interface.
func (app IMCSystemCachesResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toIMCSystemCaches(caches []pool.IMCSystemCacheDetail) IMCSystemCachesResponse {
	details := make(IMCSystemCachesResponse, len(caches))
	for i, cache := range caches {
		details[i] = IMCSystemCacheDetail{
			ModelID:        cache.ModelID,
			ID:             cache.ID,
			Tokens:         cache.Tokens,
			Allocated:      cache.Allocated,
			SnapshotBytes:  cache.SnapshotBytes,
			RestoreCount:   cache.RestoreCount,
			ActiveRestores: cache.ActiveRestores,
			LastUsed:       cache.LastUsed,
		}
	}
	return details
}

// BatchGenerationContribution describes one slot's contribution to the latest
// logical batch.
type BatchGenerationContribution struct {
	SlotID int    `json:"slot_id"`
	Rows   int    `json:"rows"`
	Mode   string `json:"mode"`
}

// BatchSlotDetail describes one generation slot at the latest batch-loop
// boundary.
type BatchSlotDetail struct {
	ID                      int    `json:"id"`
	Phase                   string `json:"phase"`
	RequestID               string `json:"request_id"`
	RequestAgeMS            int64  `json:"request_age_ms"`
	PrefillOwner            bool   `json:"prefill_owner"`
	PromptTokens            int    `json:"prompt_tokens"`
	PrefilledTokens         int    `json:"prefilled_tokens"`
	PrefillRemaining        int    `json:"prefill_remaining"`
	GeneratedTokens         int    `json:"generated_tokens"`
	PastTokens              int    `json:"past_tokens"`
	GenerationMode          string `json:"generation_mode"`
	GenerationRows          int    `json:"generation_rows"`
	IMCPreparedTokens       int    `json:"imc_prepared_tokens"`
	IMCTotalTokens          int    `json:"imc_total_tokens"`
	IMCPreparationRemaining int    `json:"imc_preparation_remaining"`
}

// BatchEngineDetail describes one loaded model's generation scheduler.
type BatchEngineDetail struct {
	ModelID                 string                        `json:"model_id"`
	Iteration               uint64                        `json:"iteration"`
	PrefillBatchSize        int                           `json:"prefill_batch_size"`
	NBatch                  int                           `json:"nbatch"`
	NUBatch                 int                           `json:"nubatch"`
	MTP                     bool                          `json:"mtp"`
	NDraft                  int                           `json:"ndraft"`
	QueuedRequests          int                           `json:"queued_requests"`
	PendingRequests         int                           `json:"pending_requests"`
	PrefillSelectorStart    int                           `json:"prefill_selector_start"`
	PrefillSelectorSelected int                           `json:"prefill_selector_selected"`
	PrefillSelectorNext     int                           `json:"prefill_selector_next"`
	EligiblePrefillSlots    []int                         `json:"eligible_prefill_slots"`
	IMCSelectorStart        int                           `json:"imc_selector_start"`
	IMCSelectorSelected     int                           `json:"imc_selector_selected"`
	IMCSelectorNext         int                           `json:"imc_selector_next"`
	EligibleIMCSlots        []int                         `json:"eligible_imc_slots"`
	GenerationRows          int                           `json:"generation_rows"`
	PrefillRows             int                           `json:"prefill_rows"`
	TotalRows               int                           `json:"total_rows"`
	GenerationContributions []BatchGenerationContribution `json:"generation_contributions"`
	Slots                   []BatchSlotDetail             `json:"slots"`
}

// BatchEngineSlotsResponse is the latest scheduler state for loaded generation
// models.
type BatchEngineSlotsResponse []BatchEngineDetail

// Encode implements the encoder interface.
func (app BatchEngineSlotsResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toBatchEngineSnapshots(snapshots []pool.BatchEngineDetail) BatchEngineSlotsResponse {
	details := make(BatchEngineSlotsResponse, len(snapshots))
	for i, snapshot := range snapshots {
		detail := BatchEngineDetail{
			ModelID:                 snapshot.ModelID,
			Iteration:               snapshot.Iteration,
			PrefillBatchSize:        snapshot.PrefillBatchSize,
			NBatch:                  snapshot.NBatch,
			NUBatch:                 snapshot.NUBatch,
			MTP:                     snapshot.MTP,
			NDraft:                  snapshot.NDraft,
			QueuedRequests:          snapshot.QueuedRequests,
			PendingRequests:         snapshot.PendingRequests,
			PrefillSelectorStart:    snapshot.PrefillSelectorStart,
			PrefillSelectorSelected: snapshot.PrefillSelectorSelected,
			PrefillSelectorNext:     snapshot.PrefillSelectorNext,
			EligiblePrefillSlots:    append([]int{}, snapshot.EligiblePrefillSlots...),
			IMCSelectorStart:        snapshot.IMCSelectorStart,
			IMCSelectorSelected:     snapshot.IMCSelectorSelected,
			IMCSelectorNext:         snapshot.IMCSelectorNext,
			EligibleIMCSlots:        append([]int{}, snapshot.EligibleIMCSlots...),
			GenerationRows:          snapshot.GenerationRows,
			PrefillRows:             snapshot.PrefillRows,
			TotalRows:               snapshot.TotalRows,
			GenerationContributions: make([]BatchGenerationContribution, len(snapshot.GenerationContributions)),
			Slots:                   make([]BatchSlotDetail, len(snapshot.Slots)),
		}

		for j, contribution := range snapshot.GenerationContributions {
			detail.GenerationContributions[j] = BatchGenerationContribution{
				SlotID: contribution.SlotID,
				Rows:   contribution.Rows,
				Mode:   contribution.Mode,
			}
		}

		for j, slot := range snapshot.Slots {
			detail.Slots[j] = BatchSlotDetail{
				ID:                      slot.ID,
				Phase:                   slot.Phase,
				RequestID:               slot.RequestID,
				RequestAgeMS:            slot.RequestAge.Milliseconds(),
				PrefillOwner:            slot.PrefillOwner,
				PromptTokens:            slot.PromptTokens,
				PrefilledTokens:         slot.PrefilledTokens,
				PrefillRemaining:        slot.PrefillRemaining,
				GeneratedTokens:         slot.GeneratedTokens,
				PastTokens:              slot.PastTokens,
				GenerationMode:          slot.GenerationMode,
				GenerationRows:          slot.GenerationRows,
				IMCPreparedTokens:       slot.IMCPreparedTokens,
				IMCTotalTokens:          slot.IMCTotalTokens,
				IMCPreparationRemaining: slot.IMCPreparationRemaining,
			}
		}

		details[i] = detail
	}

	return details
}

// fromBuckyDetails converts bucky pool ModelDetail entries into the
// shared API response shape. Whisper has no separate KV/Slots concept,
// so KVCache stays zero and Slots is reported as 1 for parity with
// kronk's display. OwnedBy is set to "ggml" because every bundled
// whisper file is the GGML/ggerganov-hosted conversion. ModelFamily
// surfaces the whisper architecture ("tiny", "base", "small", …) plus
// an .en suffix when the model is english-only.
func fromBuckyDetails(models []buckypool.ModelDetail) ModelDetailsResponse {
	details := make(ModelDetailsResponse, len(models))

	for i, m := range models {
		family := m.ModelType
		if family != "" && !m.Multilingual {
			family += ".en"
		}

		slots := 0
		if m.Status == buckypool.ModelStatusLoaded {
			slots = 1
		}

		details[i] = ModelDetail{
			ID:            m.ID,
			Backend:       m.Backend,
			OwnedBy:       "ggml",
			ModelFamily:   family,
			Size:          m.Size,
			VRAMTotal:     m.VRAMTotal,
			Slots:         slots,
			ExpiresAt:     m.ExpiresAt,
			ActiveStreams: m.ActiveStreams,
			Status:        m.Status,
		}
	}

	return details
}

// =============================================================================

// DeviceBudget describes the budget accounting for a single device.
type DeviceBudget struct {
	Index       int    `json:"index"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	TotalBytes  int64  `json:"total_bytes"`
	BudgetBytes int64  `json:"budget_bytes"`
	UsedBytes   int64  `json:"used_bytes"`
}

// ReservationDevice records a per-device allocation belonging to a reservation.
type ReservationDevice struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// Reservation describes a single active reservation held by the resource
// manager.
type Reservation struct {
	Key       string              `json:"key"`
	VRAMBytes int64               `json:"vram_bytes"`
	RAMBytes  int64               `json:"ram_bytes"`
	Per       []ReservationDevice `json:"per"`
}

// PoolBudgetResponse describes the resource manager's current budget and
// usage. Used by the BUI to verify that BudgetPercent is being applied.
type PoolBudgetResponse struct {
	BudgetPercent int            `json:"budget_percent"`
	HeadroomBytes int64          `json:"headroom_bytes"`
	UnifiedMemory bool           `json:"unified_memory"`
	RAMTotal      int64          `json:"ram_total"`
	RAMBudget     int64          `json:"ram_budget"`
	RAMUsed       int64          `json:"ram_used"`
	Devices       []DeviceBudget `json:"devices"`
	Reservations  []Reservation  `json:"reservations"`
}

// Encode implements the encoder interface.
func (app PoolBudgetResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toPoolBudget(u resman.Usage) PoolBudgetResponse {
	out := PoolBudgetResponse{
		BudgetPercent: u.BudgetPercent,
		HeadroomBytes: u.HeadroomBytes,
		UnifiedMemory: u.UnifiedMemory,
		RAMTotal:      u.RAMTotal,
		RAMBudget:     u.RAMBudget,
		RAMUsed:       u.RAMUsed,
		Devices:       make([]DeviceBudget, len(u.Devices)),
		Reservations:  make([]Reservation, len(u.Reservations)),
	}

	for i, d := range u.Devices {
		out.Devices[i] = DeviceBudget{
			Index:       d.Index,
			Name:        d.Name,
			Type:        d.Type,
			TotalBytes:  d.TotalBytes,
			BudgetBytes: d.BudgetBytes,
			UsedBytes:   d.UsedBytes,
		}
	}

	for i, r := range u.Reservations {
		per := make([]ReservationDevice, len(r.Per))
		for j, alloc := range r.Per {
			per[j] = ReservationDevice{
				Index: alloc.Index,
				Name:  alloc.Name,
				Bytes: alloc.Bytes,
			}
		}
		out.Reservations[i] = Reservation{
			Key:       r.Key,
			VRAMBytes: r.VRAMBytes,
			RAMBytes:  r.RAMBytes,
			Per:       per,
		}
	}

	return out
}

// =============================================================================

// MoEInfo contains Mixture of Experts metadata.
type MoEInfo struct {
	IsMoE            bool  `json:"is_moe"`
	ExpertCount      int64 `json:"expert_count"`
	ExpertUsedCount  int64 `json:"expert_used_count"`
	HasSharedExperts bool  `json:"has_shared_experts"`
}

func toAppMoEInfo(m *gguf.MoEInfo) *MoEInfo {
	if m == nil {
		return nil
	}

	return &MoEInfo{
		IsMoE:            m.IsMoE,
		ExpertCount:      m.ExpertCount,
		ExpertUsedCount:  m.ExpertUsedCount,
		HasSharedExperts: m.HasSharedExperts,
	}
}

// WeightBreakdown provides per-category weight size information.
type WeightBreakdown struct {
	TotalBytes         int64   `json:"total_bytes"`
	AlwaysActiveBytes  int64   `json:"always_active_bytes"`
	ExpertBytesTotal   int64   `json:"expert_bytes_total"`
	ExpertBytesByLayer []int64 `json:"expert_bytes_by_layer"`
}

func toAppWeightBreakdown(w *gguf.WeightBreakdown) *WeightBreakdown {
	if w == nil {
		return nil
	}

	return &WeightBreakdown{
		TotalBytes:         w.TotalBytes,
		AlwaysActiveBytes:  w.AlwaysActiveBytes,
		ExpertBytesTotal:   w.ExpertBytesTotal,
		ExpertBytesByLayer: w.ExpertBytesByLayer,
	}
}

// VRAMInput contains the input parameters used for VRAM calculation.
type VRAMInput struct {
	ModelSizeBytes      int64            `json:"model_size_bytes"`
	ContextWindow       int64            `json:"context_window"`
	BlockCount          int64            `json:"block_count"`
	HeadCountKV         int64            `json:"head_count_kv"`
	KeyLength           int64            `json:"key_length"`
	ValueLength         int64            `json:"value_length"`
	BytesPerElement     int64            `json:"bytes_per_element"`
	Slots               int64            `json:"slots"`
	SlidingWindow       int64            `json:"sliding_window,omitempty"`
	SlidingWindowLayers int64            `json:"sliding_window_layers,omitempty"`
	EmbeddingLength     int64            `json:"embedding_length,omitempty"`
	MoE                 *MoEInfo         `json:"moe,omitempty"`
	Weights             *WeightBreakdown `json:"weights,omitempty"`
	GPULayers           int64            `json:"gpu_layers"`
	ExpertLayersOnGPU   int64            `json:"expert_layers_on_gpu"`
	KVCacheOnCPU        bool             `json:"kv_cache_on_cpu,omitempty"`
	SWAFull             bool             `json:"swa_full"`
}

// PerDeviceVRAM is the per-GPU VRAM split used when tensor_split /
// device_count are specified.
type PerDeviceVRAM struct {
	Label        string `json:"label"`
	WeightsBytes int64  `json:"weights_bytes"`
	KVBytes      int64  `json:"kv_bytes"`
	ComputeBytes int64  `json:"compute_bytes"`
	TotalBytes   int64  `json:"total_bytes"`
}

// SamplingConfig represents sampling parameters for model inference.
type SamplingConfig struct {
	Temperature      float32 `json:"temperature"`
	TopK             int32   `json:"top_k"`
	TopP             float32 `json:"top_p"`
	MinP             float32 `json:"min_p"`
	MaxTokens        int     `json:"max_tokens"`
	RepeatPenalty    float32 `json:"repeat_penalty"`
	RepeatLastN      int32   `json:"repeat_last_n"`
	DryMultiplier    float32 `json:"dry_multiplier"`
	DryBase          float32 `json:"dry_base"`
	DryAllowedLen    int32   `json:"dry_allowed_length"`
	DryPenaltyLast   int32   `json:"dry_penalty_last_n"`
	XtcProbability   float32 `json:"xtc_probability"`
	XtcThreshold     float32 `json:"xtc_threshold"`
	XtcMinKeep       uint32  `json:"xtc_min_keep"`
	FrequencyPenalty float32 `json:"frequency_penalty"`
	PresencePenalty  float32 `json:"presence_penalty"`
	EnableThinking   string  `json:"enable_thinking"`
	ReasoningEffort  string  `json:"reasoning_effort"`
	Grammar          string  `json:"grammar"`
}

// MoEConfig configures Mixture of Experts tensor placement.
type MoEConfig struct {
	Mode                             model.MoEMode `json:"mode,omitzero"`
	PtrKeepExpertsOnGPUForTopNLayers *int          `json:"keep_experts_top_n,omitempty"`
}

func toAppMoEConfig(m *model.MoEConfig) *MoEConfig {
	if m == nil {
		return nil
	}

	return &MoEConfig{
		Mode:                             m.Mode,
		PtrKeepExpertsOnGPUForTopNLayers: m.PtrKeepExpertsOnGPUForTopNLayers,
	}
}

// ModelConfig represents the model configuration the model will use by default.
type ModelConfig struct {
	PtrContextWindow      *int                     `json:"context-window"`
	PtrPrefillBatchSize   *int                     `json:"prefill-batch-size"`
	PtrNThreads           *int                     `json:"nthreads"`
	PtrNThreadsBatch      *int                     `json:"nthreads-batch"`
	CacheTypeK            model.GGMLType           `json:"cache-type-k"`
	CacheTypeV            model.GGMLType           `json:"cache-type-v"`
	LoadMode              model.LoadMode           `json:"load-mode"`
	NUMA                  string                   `json:"numa,omitempty"`
	FlashAttention        model.FlashAttentionType `json:"flash-attention"`
	PtrNSeqMax            *int                     `json:"nseq-max"`
	QueueDepth            int                      `json:"queue-depth"`
	AdmissionCapacity     int                      `json:"admission-capacity"`
	PtrIMCSessionCapacity *int                     `json:"imc-session-capacity"`
	PtrOffloadKQV         *bool                    `json:"offload-kqv"`
	PtrOpOffload          *bool                    `json:"op-offload"`
	PtrProjOnCPU          *bool                    `json:"proj-on-cpu"`
	ProjDevice            string                   `json:"proj-device,omitempty"`
	PtrNGpuLayers         *int                     `json:"ngpu-layers"`
	PtrSplitMode          *model.SplitMode         `json:"split-mode"`
	TensorSplit           []float32                `json:"tensor-split"`
	TensorBuftOverrides   []string                 `json:"tensor-buft-overrides"`
	PtrMainGPU            *int                     `json:"main-gpu"`
	Devices               []string                 `json:"devices"`
	MoE                   *MoEConfig               `json:"moe,omitempty"`
	PtrSWAFull            *bool                    `json:"swa-full"`
	PtrIncrementalCache   *bool                    `json:"incremental-cache"`
	PtrCacheMinTokens     *int                     `json:"cache-min-tokens"`
	SessionStoreKind      kvstorage.Kind           `json:"session-store-kind,omitzero"`
	Sampling              SamplingConfig           `json:"sampling-parameters"`
	RopeScaling           model.RopeScalingType    `json:"rope-scaling-type"`
	PtrRopeFreqBase       *float32                 `json:"rope-freq-base"`
	PtrRopeFreqScale      *float32                 `json:"rope-freq-scale"`
	PtrYarnExtFactor      *float32                 `json:"yarn-ext-factor"`
	PtrYarnAttnFactor     *float32                 `json:"yarn-attn-factor"`
	PtrYarnBetaFast       *float32                 `json:"yarn-beta-fast"`
	PtrYarnBetaSlow       *float32                 `json:"yarn-beta-slow"`
	PtrYarnOrigCtx        *int                     `json:"yarn-orig-ctx"`
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
//
// ModelURL (a HuggingFace URL or owner/repo path), ModelURLs (resolved
// split-model files), or ModelID (a local model id known to the server) must
// be supplied. When AutoFit is true the server runs the layer/expert-offload
// search using the supplied hardware constraints (GPUFreeBytes,
// SystemRAMBytes, DeviceCount, TensorSplit) and returns the best-fitting
// configuration.
type VRAMRequest struct {
	ModelURL          string    `json:"model_url"`
	ModelURLs         []string  `json:"model_urls,omitempty"`
	ModelID           string    `json:"model_id,omitempty"`
	ContextWindow     int64     `json:"context_window"`
	BytesPerElement   int64     `json:"bytes_per_element"`
	Slots             int64     `json:"slots"`
	GPULayers         int64     `json:"gpu_layers,omitempty"`
	ExpertLayersOnGPU int64     `json:"expert_layers_on_gpu,omitempty"`
	KVCacheOnCPU      bool      `json:"kv_cache_on_cpu,omitempty"`
	SWAFull           *bool     `json:"swa_full,omitempty"`
	DeviceCount       int64     `json:"device_count,omitempty"`
	TensorSplit       []float64 `json:"tensor_split,omitempty"`

	AutoFit        bool    `json:"auto_fit,omitempty"`
	GPUFreeBytes   []int64 `json:"gpu_free_bytes,omitempty"`
	GPUCapacity    int64   `json:"gpu_capacity_bytes,omitempty"`
	SystemRAMBytes int64   `json:"system_ram_bytes,omitempty"`
	UnifiedMemory  bool    `json:"unified_memory,omitempty"`
}

// Decode implements the decoder interface.
func (app *VRAMRequest) Decode(data []byte) error {
	return json.Unmarshal(data, app)
}

// AutoTuneRequest identifies a local or catalog model to analyze. Exactly one
// identifier must be provided.
type AutoTuneRequest struct {
	ModelID   string `json:"model_id,omitempty"`
	CatalogID string `json:"catalog_id,omitempty"`
}

// Decode implements the decoder interface.
func (app *AutoTuneRequest) Decode(data []byte) error {
	return json.Unmarshal(data, app)
}

// AutoTuneResponse contains hardware-aware model analysis and runtime
// recommendations produced without loading the model.
type AutoTuneResponse struct {
	models.Analysis
}

// Encode implements the encoder interface.
func (app AutoTuneResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

// VRAMResponse represents the VRAM calculation results.
type VRAMResponse struct {
	Input              VRAMInput        `json:"input"`
	KVPerTokenPerLayer int64            `json:"kv_per_token_per_layer"`
	KVPerSlot          int64            `json:"kv_per_slot"`
	SlotMemory         int64            `json:"slot_memory"`
	TotalVRAM          int64            `json:"total_vram"`
	MoE                *MoEInfo         `json:"moe,omitempty"`
	Weights            *WeightBreakdown `json:"weights,omitempty"`
	ModelWeightsGPU    int64            `json:"model_weights_gpu"`
	ModelWeightsCPU    int64            `json:"model_weights_cpu"`
	ComputeBufferEst   int64            `json:"compute_buffer_est"`

	// MoE / dense breakdown for UI display.
	AlwaysActiveGPUBytes int64 `json:"always_active_gpu_bytes,omitempty"`
	AlwaysActiveCPUBytes int64 `json:"always_active_cpu_bytes,omitempty"`
	ExpertGPUBytes       int64 `json:"expert_gpu_bytes,omitempty"`
	ExpertCPUBytes       int64 `json:"expert_cpu_bytes,omitempty"`

	// KV cache placement and total system RAM estimate.
	KVVRAMBytes       int64          `json:"kv_vram_bytes"`
	KVCPUBytes        int64          `json:"kv_cpu_bytes"`
	TotalSystemRAMEst int64          `json:"total_system_ram_est"`
	UnifiedFootprint  int64          `json:"unified_footprint"`
	AutoFitSucceeded  *bool          `json:"auto_fit_succeeded,omitempty"`
	FitAssessment     *FitAssessment `json:"fit_assessment,omitempty"`

	// Per-device split (only populated when DeviceCount/TensorSplit
	// were supplied on the request).
	PerDevice []PerDeviceVRAM `json:"per_device,omitempty"`
	RepoFiles []HFRepoFile    `json:"repo_files,omitempty"`
}

// CapacityAssessment describes the server-calculated status of one memory pool.
type CapacityAssessment struct {
	RequiredBytes int64          `json:"required_bytes"`
	CapacityBytes int64          `json:"capacity_bytes"`
	HeadroomBytes int64          `json:"headroom_bytes"`
	Status        vram.FitStatus `json:"status"`
}

// FitAssessment describes the server-calculated hardware fit verdict.
type FitAssessment struct {
	Fits      bool               `json:"fits"`
	Status    vram.FitStatus     `json:"status"`
	GPU       CapacityAssessment `json:"gpu"`
	SystemRAM CapacityAssessment `json:"system_ram"`
	Unified   CapacityAssessment `json:"unified"`
}

func toFitAssessment(assessment vram.FitAssessment) FitAssessment {
	convert := func(capacity vram.CapacityAssessment) CapacityAssessment {
		return CapacityAssessment{
			RequiredBytes: capacity.RequiredBytes,
			CapacityBytes: capacity.CapacityBytes,
			HeadroomBytes: capacity.HeadroomBytes,
			Status:        capacity.Status,
		}
	}

	return FitAssessment{
		Fits:      assessment.Fits,
		Status:    assessment.Status,
		GPU:       convert(assessment.GPU),
		SystemRAM: convert(assessment.SystemRAM),
		Unified:   convert(assessment.Unified),
	}
}

// Encode implements the encoder interface.
func (app VRAMResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toVRAMResponse(v vram.Result, repoFiles []HFRepoFile) VRAMResponse {
	resp := VRAMResponse{
		Input: VRAMInput{
			ModelSizeBytes:      v.Input.ModelSizeBytes,
			ContextWindow:       v.Input.ContextWindow,
			BlockCount:          v.Input.BlockCount,
			HeadCountKV:         v.Input.HeadCountKV,
			KeyLength:           v.Input.KeyLength,
			ValueLength:         v.Input.ValueLength,
			BytesPerElement:     v.Input.BytesPerElement,
			Slots:               v.Input.Slots,
			SlidingWindow:       v.Input.SlidingWindow,
			SlidingWindowLayers: v.Input.SlidingWindowLayers,
			EmbeddingLength:     v.Input.EmbeddingLength,
			MoE:                 toAppMoEInfo(v.Input.MoE),
			Weights:             toAppWeightBreakdown(v.Input.Weights),
			GPULayers:           v.Input.GPULayers,
			ExpertLayersOnGPU:   v.Input.ExpertLayersOnGPU,
			KVCacheOnCPU:        v.Input.KVCacheOnCPU,
			SWAFull:             v.Input.SWAFull,
		},
		KVPerTokenPerLayer:   v.KVPerTokenPerLayer,
		KVPerSlot:            v.KVPerSlot,
		SlotMemory:           v.SlotMemory,
		TotalVRAM:            v.TotalVRAM,
		MoE:                  toAppMoEInfo(v.MoE),
		Weights:              toAppWeightBreakdown(v.Weights),
		ModelWeightsGPU:      v.ModelWeightsGPU,
		ModelWeightsCPU:      v.ModelWeightsCPU,
		ComputeBufferEst:     v.ComputeBufferEst,
		AlwaysActiveGPUBytes: v.AlwaysActiveGPUBytes,
		AlwaysActiveCPUBytes: v.AlwaysActiveCPUBytes,
		ExpertGPUBytes:       v.ExpertGPUBytes,
		ExpertCPUBytes:       v.ExpertCPUBytes,
		KVVRAMBytes:          v.KVVRAMBytes,
		KVCPUBytes:           v.KVCPUBytes,
		TotalSystemRAMEst:    v.TotalSystemRAMEst,
		UnifiedFootprint:     v.UnifiedFootprint(),
		RepoFiles:            repoFiles,
	}

	if len(v.PerDevice) > 0 {
		resp.PerDevice = make([]PerDeviceVRAM, len(v.PerDevice))
		for i, d := range v.PerDevice {
			resp.PerDevice[i] = PerDeviceVRAM{
				Label:        d.Label,
				WeightsBytes: d.WeightsBytes,
				KVBytes:      d.KVBytes,
				ComputeBytes: d.ComputeBytes,
				TotalBytes:   d.TotalBytes,
			}
		}
	}

	return resp
}

// vramConfigFromRMC builds a vram.Config using the model's resolved
// configuration so the model detail screen can render an initial
// VRAM estimate without requiring user input.
func vramConfigFromRMC(rmc models.ModelConfig) vram.Config {
	contextWindow := int64(8192)
	if rmc.PtrContextWindow != nil && *rmc.PtrContextWindow > 0 {
		contextWindow = int64(*rmc.PtrContextWindow)
	}

	slots := int64(1)
	if rmc.PtrNSeqMax != nil && *rmc.PtrNSeqMax > 0 {
		slots = int64(*rmc.PtrNSeqMax)
	}

	bpe := int64(1)
	switch rmc.CacheTypeK {
	case model.GGMLTypeF32:
		bpe = vram.BytesPerElementF32
	case model.GGMLTypeF16, model.GGMLTypeBF16, model.GGMLTypeAuto:
		bpe = vram.BytesPerElementF16
	}

	kronkConfig := rmc.ToKronkConfig()

	return vram.Config{
		ContextWindow:     contextWindow,
		BytesPerElement:   bpe,
		Slots:             slots,
		NUBatch:           int64(kronkConfig.PrefillBatchSize()),
		GPULayers:         int64(kronkConfig.NGpuLayers()),
		ExpertLayersOnGPU: kronkConfig.ExpertLayersOnGPU(),
		SWAFull:           resolveSWAFull(nil, kronkConfig.PtrSWAFull),
	}
}

const llamaDefaultSWAFull = true

// resolveSWAFull applies the same precedence used by the runtime: a request
// override wins, followed by the per-model setting. Unset values use the
// llama.cpp default shipped with this Kronk version. Keeping this fallback in
// Go allows the catalog and calculator to work while the native library is in
// degraded mode.
func resolveSWAFull(requested *bool, configured *bool) bool {
	if requested != nil {
		return *requested
	}
	if configured != nil {
		return *configured
	}

	return llamaDefaultSWAFull
}

// =============================================================================

// HFRepoFile represents a GGUF file in a HuggingFace repository.
type HFRepoFile struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	SizeStr  string `json:"size_str"`
}

// =============================================================================

// UnloadResponse represents the output for a model unload operation.
type UnloadResponse struct {
	Status string `json:"status"`
	ID     string `json:"id"`
}

// Encode implements the encoder interface.
func (app UnloadResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

// UnloadRequest represents the input for unloading a model from the cache.
type UnloadRequest struct {
	ID string `json:"id"`
}

// Decode implements the decoder interface.
func (app *UnloadRequest) Decode(data []byte) error {
	return json.Unmarshal(data, app)
}

// Validate checks the request is valid.
func (app *UnloadRequest) Validate() error {
	if app.ID == "" {
		return fmt.Errorf("id is required")
	}
	return nil
}

// =============================================================================

// DevicesResponse returns information about available compute devices.
type DevicesResponse devices.Devices

// Encode implements the encoder interface.
func (d DevicesResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(d)
	return data, "application/json", err
}

// =============================================================================

// CombinationResponse describes a single supported (architecture, operating
// system, processor) triple that the upstream llama.cpp build matrix
// publishes. It mirrors libs.Combination so the BUI can populate bundle
// download selectors without needing a copy of the upstream constants.
type CombinationResponse struct {
	Arch      string `json:"arch"`
	OS        string `json:"os"`
	Processor string `json:"processor"`
}

// CombinationsResponse is the list of supported combinations returned by the
// /v1/libs/combinations endpoint.
type CombinationsResponse struct {
	Combinations []CombinationResponse `json:"combinations"`
}

// Encode implements the encoder interface.
func (app CombinationsResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toAppCombinations(in []libs.Combination) CombinationsResponse {
	out := CombinationsResponse{
		Combinations: make([]CombinationResponse, len(in)),
	}

	for i, c := range in {
		out.Combinations[i] = CombinationResponse{Arch: c.Arch, OS: c.OS, Processor: c.Processor}
	}

	return out
}

// BundleTagResponse describes a single installed library bundle.
type BundleTagResponse struct {
	Version   string `json:"version"`
	Arch      string `json:"arch"`
	OS        string `json:"os"`
	Processor string `json:"processor"`
}

// BundleListResponse is the list of installed bundles returned by the
// /v1/libs/installs endpoint.
type BundleListResponse struct {
	Bundles []BundleTagResponse `json:"bundles"`
}

// Encode implements the encoder interface.
func (app BundleListResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toAppBundleList(in []libs.VersionTag) BundleListResponse {
	out := BundleListResponse{Bundles: make([]BundleTagResponse, len(in))}

	for i, t := range in {
		out.Bundles[i] = BundleTagResponse{
			Version:   t.Version,
			Arch:      t.Arch,
			OS:        t.OS,
			Processor: t.Processor,
		}
	}

	return out
}

// BundleActionResponse is returned by mutating bundle endpoints (remove)
// to confirm which bundle was acted upon.
type BundleActionResponse struct {
	Status    string `json:"status"`
	Arch      string `json:"arch"`
	OS        string `json:"os"`
	Processor string `json:"processor"`
}

// Encode implements the encoder interface.
func (app BundleActionResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

// CatalogListResponse wraps a slice of catalog summaries with an Encode
// method so the listCatalog handler can return it as a web.Encoder.
type CatalogListResponse []models.CatalogSummary

// Encode implements web.Encoder.
func (r CatalogListResponse) Encode() ([]byte, string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	return data, "application/json", err
}

// CatalogDetailResponse wraps a catalog detail with an Encode method so
// the showCatalog handler can return it as a web.Encoder. Vram is computed
// from the same GGUF head bytes used to populate the detail and embedded
// here so the catalog detail screen does not need a second round trip.
type CatalogDetailResponse struct {
	models.CatalogDetail
	Vram           *VRAMResponse `json:"vram,omitempty"`
	SWAFullDefault bool          `json:"swa_full_default"`
}

// Encode implements web.Encoder for the detail payload.
func (d CatalogDetailResponse) Encode() ([]byte, string, error) {
	data, err := json.MarshalIndent(d, "", "  ")
	return data, "application/json", err
}

// RemoveResponse is the JSON response for DELETE /v1/catalog/{id}.
type RemoveResponse struct {
	Status string `json:"status"`
	ID     string `json:"id"`
}

// Encode implements web.Encoder.
func (r RemoveResponse) Encode() ([]byte, string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	return data, "application/json", err
}

// ReconcileResponse is the JSON response for POST /v1/catalog/reconcile.
type ReconcileResponse struct {
	Status string `json:"status"`
}

// Encode implements web.Encoder.
func (r ReconcileResponse) Encode() ([]byte, string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	return data, "application/json", err
}

// LookupRequest is the body for POST /v1/catalog/lookup. The input may be
// a HuggingFace URL ("https://huggingface.co/owner/repo[/resolve/...]") or
// a shorthand ("owner/repo" or "owner/repo:tag").
type LookupRequest struct {
	Input string `json:"input"`
}

// Decode implements the decoder interface.
func (app *LookupRequest) Decode(data []byte) error {
	return json.Unmarshal(data, app)
}

// LookupResponse is the JSON response for POST /v1/catalog/lookup. It lists
// the GGUF files available in the resolved HuggingFace repository so the
// VRAM calculator UI can let the user pick a specific shard or quant.
type LookupResponse struct {
	RepoFiles []HFRepoFile `json:"repo_files"`
}

// Encode implements web.Encoder.
func (r LookupResponse) Encode() ([]byte, string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	return data, "application/json", err
}

// ResolveRequest is the body for POST /v1/catalog/resolve. The source may be a
// full HuggingFace URL or a canonical id ("provider/family"). The resolver
// maps either form to the canonical download URLs without initiating a
// download.
type ResolveRequest struct {
	Source string `json:"source"`
}

// Decode implements the decoder interface.
func (app *ResolveRequest) Decode(data []byte) error {
	return json.Unmarshal(data, app)
}

// ResolveResponse is the JSON response for POST /v1/catalog/resolve. It
// describes what Download would fetch for the given source: canonical id,
// fully qualified download URL(s) (multiple entries for split models),
// the companion projection URL when applicable, and flags reporting
// whether the result came from the catalog cache or local disk. When
// Installed is true, every model file (and the projection when present)
// is already on disk.
//
// When the input only identifies a repository (e.g. "owner/repo" or a
// HuggingFace tree/repo URL with no filename), the resolver is not run
// and RepoFiles instead lists every GGUF in the repo so the caller can
// present a picker. In that case CanonicalID, DownloadURLs, etc. are
// empty and only Provider and Family are set.
type ResolveResponse struct {
	CanonicalID  string       `json:"canonical_id"`
	Provider     string       `json:"provider"`
	Family       string       `json:"family"`
	Revision     string       `json:"revision"`
	DownloadURLs []string     `json:"download_urls"`
	DownloadProj string       `json:"download_proj,omitempty"`
	DownloadMTP  string       `json:"download_mtp,omitempty"`
	FromCache    bool         `json:"from_cache"`
	FromLocal    bool         `json:"from_local"`
	Installed    bool         `json:"installed"`
	RepoFiles    []HFRepoFile `json:"repo_files,omitempty"`
}

// Encode implements web.Encoder.
func (r ResolveResponse) Encode() ([]byte, string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	return data, "application/json", err
}
