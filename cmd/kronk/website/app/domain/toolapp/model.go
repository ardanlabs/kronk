package toolapp

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/tools"
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
