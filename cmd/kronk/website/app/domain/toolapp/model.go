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
	Processor string `json:"processor"`
	Latest    string `json:"latest"`
	Current   string `json:"current"`
}

// Encode implements the encoder interface.
func (app Version) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toAppVersion(status string, libPath string, processor download.Processor, krn tools.LibVersion) Version {
	return Version{
		Status:    status,
		LibPath:   libPath,
		Processor: processor.String(),
		Latest:    krn.Latest,
		Current:   krn.Current,
	}
}

// =============================================================================

// ListModelsInfo represents a collection of model information.
type ListModelsInfo []ListModelInfo

// Encode implements the encoder interface.
func (app ListModelsInfo) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

// ListModelInfo provides information about a model.
type ListModelInfo struct {
	Organization string    `json:"organization"`
	ModelName    string    `json:"model_name"`
	ModelFile    string    `json:"model_file"`
	Size         int64     `json:"size"`
	Modified     time.Time `json:"modified"`
}

func toListModelsInfo(models []tools.ModelFile) ListModelsInfo {
	var list ListModelsInfo

	for _, model := range models {
		list = append(list, ListModelInfo{
			Organization: model.Organization,
			ModelName:    model.ModelName,
			ModelFile:    model.ModelFile,
			Size:         model.Size,
			Modified:     model.Modified,
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
	Name        string            `json:"name"`
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
		Name:        model.Name,
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
