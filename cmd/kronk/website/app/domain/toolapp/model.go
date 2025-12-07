package toolapp

import (
	"encoding/json"
	"time"

	"github.com/ardanlabs/kronk/model"
	"github.com/ardanlabs/kronk/tools"
	"github.com/hybridgroup/yzma/pkg/download"
)

// Version returns information about the installed libraries.
type Version struct {
	Status    string `json:"status"`
	LibPath   string `json:"libs_paths"`
	Arch      string `json:"arch"`
	OS        string `json:"os"`
	Processor string `json:"processor"`
	Latest    string `json:"latest"`
	Current   string `json:"current"`
}

// Encode implements the encoder interface.
func (app Version) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toAppVersion(status string, libPath string, arch download.Arch, os download.OS, processor download.Processor, krn tools.VersionTag) Version {
	return Version{
		Status:    status,
		LibPath:   libPath,
		Arch:      arch.String(),
		OS:        os.String(),
		Processor: processor.String(),
		Latest:    krn.Latest,
		Current:   krn.Version,
	}
}

// =============================================================================

// ListModelInfo contains the list of models loaded in the system.
type ListModelInfo struct {
	Object string            `json:"object"`
	Data   []ListModelDetail `json:"data"`
}

// Encode implements the encoder interface.
func (app ListModelInfo) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

// ListModelDetail provides information about a model.
type ListModelDetail struct {
	ModelID     string    `json:"id"`
	Object      string    `json:"object"`
	Created     int64     `json:"created"`
	OwnedBy     string    `json:"owned_by"`
	ModelFamily string    `json:"model_family"`
	Size        int64     `json:"size"`
	Modified    time.Time `json:"modified"`
}

func toListModelsInfo(models []tools.ModelFile) ListModelInfo {
	list := ListModelInfo{
		Object: "list",
	}

	for _, model := range models {
		list.Data = append(list.Data, ListModelDetail{
			ModelID:     model.ID,
			Object:      "model",
			Created:     model.Modified.UnixMilli(),
			OwnedBy:     model.Organization,
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
func (pr *PullRequest) Decode(data []byte) error {
	return json.Unmarshal(data, pr)
}

// =============================================================================

type ModelInfo struct {
	ID          string            `json:"id"`
	Desc        string            `json:"desc"`
	Size        uint64            `json:"size"`
	HasEncoder  bool              `json:"has_encoder"`
	HasDecoder  bool              `json:"has_decoder"`
	IsRecurrent bool              `json:"is_recurrent"`
	IsHybrid    bool              `json:"is_hybrid"`
	IsGPT       bool              `json:"is_gpt"`
	Metadata    map[string]string `json:"metadata"`
}

// Encode implements the encoder interface.
func (app ModelInfo) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toModelInfo(model model.ModelInfo) ModelInfo {
	return ModelInfo{
		ID:          model.ID,
		Desc:        model.Desc,
		Size:        model.Size,
		HasEncoder:  model.HasEncoder,
		HasDecoder:  model.HasDecoder,
		IsRecurrent: model.IsRecurrent,
		IsHybrid:    model.IsHybrid,
		IsGPT:       model.IsGPT,
		Metadata:    model.Metadata,
	}
}
