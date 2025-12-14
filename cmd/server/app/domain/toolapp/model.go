package toolapp

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/cache"
	"github.com/ardanlabs/kronk/sdk/tools"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
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

func toAppVersion(status string, vt tools.VersionTag) string {
	vi := VersionResponse{
		Status:    status,
		Arch:      vt.Arch,
		OS:        vt.OS,
		Processor: vt.Processor,
		Latest:    vt.Latest,
		Current:   vt.Version,
	}

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

func toListModelsInfo(models []tools.ModelFile) ListModelInfoResponse {
	list := ListModelInfoResponse{
		Object: "list",
	}

	for _, model := range models {
		list.Data = append(list.Data, ListModelDetail{
			ID:          model.ID,
			Object:      "model",
			Created:     model.Modified.UnixMilli(),
			OwnedBy:     model.OwnedBy,
			ModelFamily: model.ModelFamily,
			Size:        model.Size,
			Modified:    model.Modified,
		})
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
	Status     string `json:"status"`
	ModelFile  string `json:"model_file,omitempty"`
	ProjFile   string `json:"proj_file,omitempty"`
	Downloaded bool   `json:"downloaded,omitempty"`
}

// Encode implements the encoder interface.
func (app PullResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toAppPull(status string, mp tools.ModelPath) string {
	pr := PullResponse{
		Status:     status,
		ModelFile:  mp.ModelFile,
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
}

// Encode implements the encoder interface.
func (app ModelInfoResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toModelInfo(model tools.ModelInfo) ModelInfoResponse {
	return ModelInfoResponse{
		ID:            model.ID,
		Object:        model.Object,
		Created:       model.Created,
		OwnedBy:       model.OwnedBy,
		Desc:          model.Details.Desc,
		Size:          model.Details.Size,
		HasProjection: model.Details.HasProjection,
		HasEncoder:    model.Details.HasEncoder,
		HasDecoder:    model.Details.HasDecoder,
		IsRecurrent:   model.Details.IsRecurrent,
		IsHybrid:      model.Details.IsHybrid,
		IsGPT:         model.Details.IsGPTModel,
		Metadata:      model.Details.Metadata,
	}
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
}

// CatalogFile represents the actual file url and size.
type CatalogFile struct {
	URL  string `json:"url"`
	Size string `json:"size"`
}

// CatalogFiles represents file information for a model.
type CatalogFiles struct {
	Model CatalogFile `json:"model"`
	Proj  CatalogFile `json:"proj"`
	Jinja CatalogFile `json:"jinja"`
}

// CatalogModelResponse represents information for a model.
type CatalogModelResponse struct {
	ID           string              `json:"id"`
	Category     string              `json:"category"`
	OwnedBy      string              `json:"owned_by"`
	ModelFamily  string              `json:"model_family"`
	WebPage      string              `json:"web_page"`
	Files        CatalogFiles        `json:"files"`
	Capabilities CatalogCapabilities `json:"capabilities"`
	Metadata     CatalogMetadata     `json:"metadata"`
	Downloaded   bool                `json:"downloaded"`
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

func toCatalogModelResponse(model catalog.Model) CatalogModelResponse {
	return CatalogModelResponse{
		ID:          model.ID,
		Category:    model.Category,
		OwnedBy:     model.OwnedBy,
		ModelFamily: model.ModelFamily,
		WebPage:     model.WebPage,
		Files: CatalogFiles{
			Model: CatalogFile{
				URL:  model.Files.Model.URL,
				Size: model.Files.Model.Size,
			},
			Proj: CatalogFile{
				URL:  model.Files.Proj.URL,
				Size: model.Files.Proj.Size,
			},
			Jinja: CatalogFile{
				URL:  model.Files.Jinja.URL,
				Size: model.Files.Jinja.Size,
			},
		},
		Capabilities: CatalogCapabilities{
			Endpoint:  model.Capabilities.Endpoint,
			Images:    model.Capabilities.Images,
			Audio:     model.Capabilities.Audio,
			Video:     model.Capabilities.Video,
			Streaming: model.Capabilities.Streaming,
			Reasoning: model.Capabilities.Reasoning,
			Tooling:   model.Capabilities.Tooling,
		},
		Metadata: CatalogMetadata{
			Created:     model.Metadata.Created,
			Collections: model.Metadata.Collections,
			Description: model.Metadata.Description,
		},
		Downloaded: model.Downloaded,
	}
}

func toCatalogModelsResponse(list []catalog.Model) CatalogModelsResponse {
	catalogModels := make([]CatalogModelResponse, len(list))

	for i, model := range list {
		catalogModels[i] = toCatalogModelResponse(model)
	}

	return catalogModels
}
