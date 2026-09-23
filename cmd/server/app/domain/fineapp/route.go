package fineapp

import (
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/finetune"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mid"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

// Config contains the systems required by the fine-tuning handlers.
type Config struct {
	AuthClient        *authclient.Client
	AuthorizationMode auth.Mode
	Preparer          DocumentPreparer
	Builder           DatasetBuilder
	FineTuner         FineTuner
	Recipes           TrainingRecipeProvider
	Models            ModelResolver
	PreparationModels PreparationModelCatalog
	DatasetsPath      string
	Runtime           finetune.Runtime
}

// Routes registers the fine-tuning API.
func Routes(webApp *web.App, cfg Config) {
	const version = "v1"

	api := newApp(cfg)
	access := mid.NewAccess(cfg.AuthClient, cfg.AuthorizationMode)
	managementAccess := access.Management()
	administrationAccess := access.Administration()

	webApp.HandlerFunc(http.MethodPost, version, "/fine-tuning/datasets", api.uploadDataset, administrationAccess)
	webApp.HandlerFunc(http.MethodGet, version, "/fine-tuning/prepare/models", api.preparationModels, managementAccess)
	webApp.HandlerFunc(http.MethodGet, version, "/fine-tuning/prepare/current", api.currentPreparation, managementAccess)
	webApp.HandlerFunc(http.MethodPost, version, "/fine-tuning/prepare/current/stop", api.stopPreparation, administrationAccess)
	webApp.HandlerFunc(http.MethodPost, version, "/fine-tuning/prepare/preview", api.previewDataset, administrationAccess)
	webApp.HandlerFunc(http.MethodPost, version, "/fine-tuning/prepare/build", api.buildDataset, administrationAccess)

	webApp.HandlerFunc(http.MethodGet, version, "/fine-tuning/recipe", api.trainingRecipe, managementAccess)
	webApp.HandlerFunc(http.MethodPost, version, "/fine-tuning/runs", api.startRun, administrationAccess)
	webApp.HandlerFunc(http.MethodGet, version, "/fine-tuning/runs", api.history, managementAccess)
	webApp.HandlerFunc(http.MethodGet, version, "/fine-tuning/runs/current", api.currentRun, managementAccess)
	webApp.HandlerFunc(http.MethodPost, version, "/fine-tuning/runs/current/stop", api.stopRun, administrationAccess)
	webApp.HandlerFunc(http.MethodGet, version, "/fine-tuning/runs/{id}", api.runByID, managementAccess)
	webApp.HandlerFunc(http.MethodPost, version, "/fine-tuning/runs/{id}/resume", api.resumeRun, administrationAccess)
}
