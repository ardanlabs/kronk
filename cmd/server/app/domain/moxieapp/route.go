package moxieapp

import (
	"context"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mid"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/moxierun"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

// Config supplies host-owned dataset and training services, uploads, and authorization.
type Config struct {
	Job               *training.Job
	Uploads           *uploads.Store
	Datasets          *datasets.Service
	AuthClient        *authclient.Client
	AuthorizationMode auth.Mode
	Resolve           func(context.Context) (moxierun.Services, error)
}

// Routes registers training upload, submission, and state APIs.
func Routes(webApp *web.App, cfg Config) {
	access := mid.NewAccess(cfg.AuthClient, cfg.AuthorizationMode)
	if cfg.Uploads != nil || cfg.Resolve != nil {
		webApp.HandlerFunc(http.MethodPost, "v1", "/training/uploads", cfg.readiness((*app).upload, nil), access.Administration())
	}

	webApp.HandlerFunc(http.MethodGet, "v1", "/training/datasets", cfg.readiness((*app).listDatasets, (*app).requireDatasets), access.Management())
	webApp.HandlerFunc(http.MethodPost, "v1", "/training/datasets", cfg.readiness((*app).createDataset, (*app).requireDatasets), access.Administration())
	webApp.HandlerFunc(http.MethodGet, "v1", "/training/datasets/{dataset_id}", cfg.readiness((*app).getDataset, (*app).requireDatasets), access.Management())
	webApp.HandlerFunc(http.MethodDelete, "v1", "/training/datasets/{dataset_id}/documents/{document_id}", cfg.readiness((*app).removeDocument, (*app).requireDatasets), access.Administration())
	webApp.HandlerFunc(http.MethodGet, "v1", "/training/datasets/{dataset_id}/versions/{version}/download", cfg.readiness((*app).downloadDatasetVersion, (*app).requireDatasets), access.Management())

	webApp.HandlerFunc(http.MethodPost, "v1", "/training/runs", cfg.readiness((*app).create, (*app).requireRuns), access.Administration())
	webApp.HandlerFunc(http.MethodPost, "v1", "/training/runs/{workflow_id}/{run_id}/preview/continue", cfg.readiness((*app).continuePreview, (*app).requireRuns), access.Administration())
	webApp.HandlerFunc(http.MethodPost, "v1", "/training/runs/{workflow_id}/{run_id}/preparation/pause", cfg.readiness((*app).pausePreparation, (*app).requireRuns), access.Administration())
	webApp.HandlerFunc(http.MethodPost, "v1", "/training/runs/{workflow_id}/{run_id}/preparation/continue", cfg.readiness((*app).continuePreparation, (*app).requireRuns), access.Administration())
	webApp.HandlerFunc(http.MethodPost, "v1", "/training/runs/{workflow_id}/{run_id}/training/pause", cfg.readiness((*app).pauseTraining, (*app).requireJob), access.Administration())
	webApp.HandlerFunc(http.MethodPost, "v1", "/training/runs/{workflow_id}/{run_id}/training/resume", cfg.readiness((*app).resumeTraining, (*app).requireJob), access.Administration())
	webApp.HandlerFunc(http.MethodPost, "v1", "/training/runs/{workflow_id}/{run_id}/start-training", cfg.readiness((*app).startTraining, (*app).requireJob), access.Administration())
	webApp.HandlerFunc(http.MethodPost, "v1", "/training/runs/{workflow_id}/{run_id}/stop", cfg.readiness((*app).stop, (*app).requireRuns), access.Administration())
	webApp.HandlerFunc(http.MethodGet, "v1", "/training/runs", cfg.readiness((*app).query, (*app).requireCatalog), access.Management())
	webApp.HandlerFunc(http.MethodGet, "v1", "/training/runs/{workflow_id}/{run_id}", cfg.readiness((*app).queryByID, (*app).requireCatalog), access.Management())
	webApp.HandlerFunc(http.MethodGet, "v1", "/training/runs/{workflow_id}/{run_id}/evaluation", cfg.readiness((*app).evaluation, (*app).requireJob), access.Management())
	webApp.HandlerFunc(http.MethodGet, "v1", "/training/runs/{workflow_id}/{run_id}/adapter", cfg.readiness((*app).adapter, (*app).requireJob), access.Management())
	webApp.HandlerFunc(http.MethodGet, "v1", "/training/runs/{workflow_id}/{run_id}/dataset", cfg.readiness((*app).dataset, (*app).requireRuns), access.Management())
}
