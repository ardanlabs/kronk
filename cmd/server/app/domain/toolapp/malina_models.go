package toolapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/tools/downloader"
	malinamodels "github.com/ardanlabs/kronk/sdk/tools/malina/models"
)

// MalinaModelEntry describes one installed stable-diffusion model bundle.
type MalinaModelEntry struct {
	ID               string `json:"id"`
	Description      string `json:"description"`
	Size             int64  `json:"size"`
	BasicTextToImage bool   `json:"basic_text_to_image"`
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

// MalinaCatalogFile describes one file in a curated Malina model bundle.
type MalinaCatalogFile struct {
	Role     string `json:"role"`
	Filename string `json:"filename"`
	URL      string `json:"url"`
	Size     string `json:"size"`
}

// MalinaCatalogEntry describes one curated Malina model bundle.
type MalinaCatalogEntry struct {
	ID               string              `json:"id"`
	Description      string              `json:"description"`
	License          string              `json:"license"`
	Gated            bool                `json:"gated"`
	BasicTextToImage bool                `json:"basic_text_to_image"`
	Files            []MalinaCatalogFile `json:"files"`
}

// MalinaCatalogResponse lists the curated Malina model bundles.
type MalinaCatalogResponse struct {
	Models []MalinaCatalogEntry `json:"models"`
}

// Encode implements the encoder interface.
func (m MalinaCatalogResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(m)
	return data, "application/json", err
}

// MalinaPullRequest is the body shape for /v1/malina/models/pull.
type MalinaPullRequest struct {
	Source string `json:"source"`
}

// Decode implements the decoder interface.
func (m *MalinaPullRequest) Decode(data []byte) error {
	return json.Unmarshal(data, m)
}

// MalinaModelActionResponse confirms a mutating Malina model operation.
type MalinaModelActionResponse struct {
	Status string `json:"status"`
	ID     string `json:"id"`
}

// Encode implements the encoder interface.
func (m MalinaModelActionResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(m)
	return data, "application/json", err
}

// =============================================================================

func (a *app) listMalinaModels(ctx context.Context, r *http.Request) web.Encoder {
	installed, err := a.malinaModels.Installed()
	if err != nil {
		return errs.Errorf(errs.Internal, "unable to retrieve malina model list: %s", err)
	}

	models := make([]MalinaModelEntry, 0, len(installed))
	for _, bundle := range installed {
		models = append(models, MalinaModelEntry{
			ID:               bundle.Name.String(),
			Description:      bundle.Description,
			Size:             bundle.Size,
			BasicTextToImage: bundle.BasicTextToImage,
		})
	}

	return MalinaModelsResponse{Models: models}
}

func (a *app) listMalinaCatalog(ctx context.Context, r *http.Request) web.Encoder {
	catalog := malinamodels.Catalog()
	models := make([]MalinaCatalogEntry, len(catalog))

	for i, bundle := range catalog {
		files := make([]MalinaCatalogFile, len(bundle.Files))
		for j, file := range bundle.Files {
			files[j] = MalinaCatalogFile{
				Role:     string(file.Role),
				Filename: file.Filename,
				URL:      file.URL,
				Size:     file.Size,
			}
		}

		models[i] = MalinaCatalogEntry{
			ID:               bundle.Name.String(),
			Description:      bundle.Description,
			License:          bundle.License,
			Gated:            bundle.Gated,
			BasicTextToImage: bundle.BasicTextToImage,
			Files:            files,
		}
	}

	return MalinaCatalogResponse{Models: models}
}

func (a *app) pullMalinaModel(ctx context.Context, r *http.Request) web.Encoder {
	var req MalinaPullRequest
	if err := web.Decode(r, &req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	source := strings.TrimSpace(req.Source)
	if source == "" {
		return errs.Errorf(errs.InvalidArgument, "source is required")
	}

	bundleName, err := malinamodels.ParseBundleName(source)
	if err != nil {
		return errs.FromSDK(err)
	}

	a.log.Info(ctx, "pull-malina-model", "source", source)

	w := web.GetWriter(ctx)
	f, ok := w.(http.Flusher)
	if !ok {
		return errs.Errorf(errs.Internal, "streaming not supported")
	}

	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Now().Add(24 * time.Hour)); err != nil {
		a.log.Info(ctx, "pull-malina-model", "set-write-deadline", "ERROR", err)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	f.Flush()

	progress := func(src string, currentSize int64, totalSize int64, mbPerSec float64, complete bool) {
		response := PullResponse{
			Status: "downloading",
			Progress: &PullProgress{
				Src:          src,
				CurrentBytes: currentSize,
				TotalBytes:   totalSize,
				MBPerSec:     mbPerSec,
				Complete:     complete,
			},
		}
		event := toAppPullResponse(response)
		a.log.Info(ctx, "pull-malina-model", "info", event[:len(event)-1])
		fmt.Fprint(w, event)
		f.Flush()
	}

	tracker := downloader.NewProgressReader(progress, downloader.SizeIntervalMB10)
	if _, err := a.malinaModels.DownloadBundleWithProgress(ctx, bundleName, tracker); err != nil {
		event := toAppPullResponse(PullResponse{Status: err.Error()})
		a.log.Info(ctx, "pull-malina-model", "status", "ERROR", "error", err.Error())
		fmt.Fprint(w, event)
		f.Flush()
		return web.NewNoResponse()
	}

	fmt.Fprint(w, toAppPullResponse(PullResponse{Status: "downloaded:" + source}))
	f.Flush()

	return web.NewNoResponse()
}

func (a *app) removeMalinaModel(ctx context.Context, r *http.Request) web.Encoder {
	modelID := web.Param(r, "model")

	a.log.Info(ctx, "remove-malina-model", "modelID", modelID)

	mp, err := a.malinaModels.FullPath(modelID)
	if err != nil {
		return errs.FromSDK(err)
	}

	if err := a.malinaModels.Remove(mp, a.log.Info); err != nil {
		return errs.Errorf(errs.Internal, "failed to remove malina model: %s", err)
	}

	return MalinaModelActionResponse{Status: "removed", ID: modelID}
}
