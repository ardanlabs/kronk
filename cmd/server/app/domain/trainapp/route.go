package trainapp

import (
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mid"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

// Config supplies the host-owned training job, its upload store, and authorization.
type Config struct {
	Job               *training.Job
	Uploads           *uploads.Store
	AuthClient        *authclient.Client
	AuthorizationMode auth.Mode
}

// Routes registers training upload, submission, and state APIs.
func Routes(webApp *web.App, cfg Config) {
	api := newApp(cfg)
	access := mid.NewAccess(cfg.AuthClient, cfg.AuthorizationMode)
	webApp.HandlerFunc(http.MethodPost, "v1", "/training/uploads", api.upload, access.Administration())
	webApp.HandlerFunc(http.MethodPost, "v1", "/training/runs", api.create, access.Administration())
	webApp.HandlerFunc(http.MethodPost, "v1", "/training/runs/{workflow_id}/{run_id}/preview/continue", api.continuePreview, access.Administration())
	webApp.HandlerFunc(http.MethodPost, "v1", "/training/runs/{workflow_id}/{run_id}/start-training", api.startTraining, access.Administration())
	webApp.HandlerFunc(http.MethodGet, "v1", "/training/runs", api.query, access.Management())
	webApp.HandlerFunc(http.MethodGet, "v1", "/training/runs/{workflow_id}/{run_id}", api.queryByID, access.Management())
	webApp.HandlerFunc(http.MethodGet, "v1", "/training/runs/{workflow_id}/{run_id}/evaluation", api.evaluation, access.Management())
	webApp.HandlerFunc(http.MethodGet, "v1", "/training/runs/{workflow_id}/{run_id}/adapter", api.adapter, access.Management())
}
