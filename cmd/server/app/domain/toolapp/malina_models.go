package toolapp

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

// MalinaModelEntry describes one installed stable-diffusion model bundle.
type MalinaModelEntry struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Size        int64  `json:"size"`
}

// MalinaModelsResponse lists installed stable-diffusion model bundles.
type MalinaModelsResponse struct {
	Models []MalinaModelEntry `json:"models"`
}

// Encode implements the encoder interface.
func (m MalinaModelsResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(m)
	return data, "application/json", err
}

func (a *app) listMalinaModels(ctx context.Context, r *http.Request) web.Encoder {
	installed, err := a.malinaModels.Installed()
	if err != nil {
		return errs.Errorf(errs.Internal, "unable to retrieve malina model list: %s", err)
	}

	models := make([]MalinaModelEntry, 0, len(installed))
	for _, bundle := range installed {
		if !bundle.BasicTextToImage {
			continue
		}
		models = append(models, MalinaModelEntry{
			ID:          bundle.Name.String(),
			Description: bundle.Description,
			Size:        bundle.Size,
		})
	}

	return MalinaModelsResponse{Models: models}
}
