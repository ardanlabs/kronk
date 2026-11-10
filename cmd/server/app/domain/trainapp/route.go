package trainapp

import (
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mid"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

// Config supplies the host-owned upload store, optional training job, and authorization.
type Config struct {
	Job               *training.Job
	Uploads           *uploads.Store
	Datasets          *preparation.DatasetStore
	AuthClient        *authclient.Client
	AuthorizationMode auth.Mode
}

// Routes registers training upload, submission, and state APIs.
func Routes(webApp *web.App, cfg Config) {
	api := newApp(cfg)
	access := mid.NewAccess(cfg.AuthClient, cfg.AuthorizationMode)
	if cfg.Uploads != nil {
		webApp.HandlerFunc(http.MethodPost, "v1", "/training/uploads", api.upload, access.Administration())
	}
	webApp.HandlerFunc(http.MethodGet, "v1", "/training/datasets", api.listDatasets, access.Management(), api.requireDatasets)
	webApp.HandlerFunc(http.MethodPost, "v1", "/training/datasets", api.createDataset, access.Administration(), api.requireDatasets)
	webApp.HandlerFunc(http.MethodGet, "v1", "/training/datasets/{dataset_id}", api.getDataset, access.Management(), api.requireDatasets)
	webApp.HandlerFunc(http.MethodDelete, "v1", "/training/datasets/{dataset_id}/documents/{document_id}", api.removeDocument, access.Administration(), api.requireDatasets)
	webApp.HandlerFunc(http.MethodGet, "v1", "/training/datasets/{dataset_id}/versions/{version}/download", api.downloadDatasetVersion, access.Management(), api.requireDatasets)

	webApp.HandlerFunc(http.MethodPost, "v1", "/training/runs", api.create, access.Administration(), api.requireJob)
	webApp.HandlerFunc(http.MethodPost, "v1", "/training/runs/{workflow_id}/{run_id}/preview/continue", api.continuePreview, access.Administration(), api.requireJob)
	webApp.HandlerFunc(http.MethodPost, "v1", "/training/runs/{workflow_id}/{run_id}/start-training", api.startTraining, access.Administration(), api.requireJob)
	webApp.HandlerFunc(http.MethodPost, "v1", "/training/runs/{workflow_id}/{run_id}/stop", api.stop, access.Administration(), api.requireJob)
	webApp.HandlerFunc(http.MethodGet, "v1", "/training/runs", api.query, access.Management(), api.requireJob)
	webApp.HandlerFunc(http.MethodGet, "v1", "/training/runs/{workflow_id}/{run_id}", api.queryByID, access.Management(), api.requireJob)
	webApp.HandlerFunc(http.MethodGet, "v1", "/training/runs/{workflow_id}/{run_id}/evaluation", api.evaluation, access.Management(), api.requireJob)
	webApp.HandlerFunc(http.MethodGet, "v1", "/training/runs/{workflow_id}/{run_id}/adapter", api.adapter, access.Management(), api.requireJob)
	webApp.HandlerFunc(http.MethodGet, "v1", "/training/runs/{workflow_id}/{run_id}/dataset", api.dataset, access.Management(), api.requireJob)
}
