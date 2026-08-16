package toolapp

import (
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mid"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/pool"
	buckylibs "github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// Config contains all the mandatory systems required by handlers.
type Config struct {
	Log                    *logger.Logger
	AuthClient             *authclient.Client
	Pool                   *pool.Pool
	Libs                   *libs.Libs
	Models                 *models.Models
	BuckyLibs              *buckylibs.Libs
	BuckyModels            *buckymodels.Models
	AuthorizationMode      auth.Mode
	LegacyManagementAccess bool
}

// Routes adds specific routes for this group.
func Routes(app *web.App, cfg Config) {
	const version = "v1"

	api := newApp(cfg)

	access := mid.NewAccess(cfg.AuthClient, cfg.AuthorizationMode, cfg.LegacyManagementAccess)
	modelDiscoveryAccess := access.ModelDiscovery()
	managementAccess := access.Management()
	administrationAccess := access.Administration()

	// -------------------------------------------------------------------------
	// OpenAI-compatible model discovery. Apps like OpenWebUI call
	// GET /v1/models to enumerate available models. The native,
	// Kronk-specific listing lives at GET /v1/kronk/models.

	app.HandlerFunc(http.MethodGet, version, "/models", api.listModelsOpenAI, modelDiscoveryAccess)
	app.HandlerFunc(http.MethodGet, version, "/models/{model}", api.retrieveModelOpenAI, modelDiscoveryAccess)

	// -------------------------------------------------------------------------
	// Kronk (llama.cpp) backend — libs, models, catalog.

	app.HandlerFunc(http.MethodGet, version, "/kronk/libs", api.listLibs, managementAccess)
	app.HandlerFunc(http.MethodPost, version, "/kronk/libs/pull", api.pullLibs, administrationAccess)
	app.HandlerFunc(http.MethodGet, version, "/kronk/libs/combinations", api.listLibsCombinations, managementAccess)
	app.HandlerFunc(http.MethodGet, version, "/kronk/libs/installs", api.listLibsInstalls, managementAccess)
	app.HandlerFunc(http.MethodDelete, version, "/kronk/libs/installs", api.removeLibsInstall, administrationAccess)

	app.HandlerFunc(http.MethodGet, version, "/kronk/models", api.listModels, managementAccess)
	app.HandlerFunc(http.MethodGet, version, "/kronk/models/", api.missingModel, managementAccess)
	app.HandlerFunc(http.MethodGet, version, "/kronk/models/integrity", api.listModelsIntegrity, managementAccess)
	app.HandlerFunc(http.MethodGet, version, "/kronk/models/integrity/{model}", api.retrieveModelIntegrity, managementAccess)
	app.HandlerFunc(http.MethodGet, version, "/kronk/models/{model}", api.showModel, managementAccess)
	app.HandlerFunc(http.MethodGet, version, "/kronk/models/ps", api.modelPS, managementAccess)
	app.HandlerFunc(http.MethodGet, version, "/kronk/models/slots", api.batchEngineSlots, managementAccess)
	app.HandlerFunc(http.MethodGet, version, "/kronk/models/imc-sessions", api.imcSessions, managementAccess)
	app.HandlerFunc(http.MethodPost, version, "/kronk/models/index", api.indexModels, administrationAccess)
	app.HandlerFunc(http.MethodPost, version, "/kronk/models/pull", api.pullModels, administrationAccess)
	app.HandlerFunc(http.MethodPost, version, "/kronk/models/autotune", api.autoTuneModel, managementAccess)
	app.HandlerFunc(http.MethodPost, version, "/kronk/models/vram", api.calculateVRAM, managementAccess)
	app.HandlerFunc(http.MethodPost, version, "/kronk/models/unload", api.unloadModel, administrationAccess)
	app.HandlerFunc(http.MethodDelete, version, "/kronk/models/{model}", api.removeModel, administrationAccess)

	app.HandlerFunc(http.MethodGet, version, "/kronk/catalog", api.listCatalog, managementAccess)
	app.HandlerFunc(http.MethodPost, version, "/kronk/catalog/reconcile", api.reconcileCatalog, administrationAccess)
	app.HandlerFunc(http.MethodPost, version, "/kronk/catalog/lookup", api.lookupCatalog, managementAccess)
	app.HandlerFunc(http.MethodPost, version, "/kronk/catalog/resolve", api.resolveCatalog, administrationAccess)
	app.HandlerFunc(http.MethodGet, version, "/kronk/catalog/{id...}", api.showCatalog, managementAccess)
	app.HandlerFunc(http.MethodDelete, version, "/kronk/catalog/{id...}", api.removeCatalog, administrationAccess)

	// -------------------------------------------------------------------------
	// Bucky (whisper.cpp) backend — libs, models. Whisper has no
	// resolver-backed catalog: the bundled short-name list is exposed
	// under /bucky/models/catalog.

	app.HandlerFunc(http.MethodGet, version, "/bucky/libs", api.listBuckyLibs, managementAccess)
	app.HandlerFunc(http.MethodPost, version, "/bucky/libs/pull", api.pullBuckyLibs, administrationAccess)
	app.HandlerFunc(http.MethodGet, version, "/bucky/libs/combinations", api.listBuckyLibsCombinations, managementAccess)
	app.HandlerFunc(http.MethodGet, version, "/bucky/libs/installs", api.listBuckyLibsInstalls, managementAccess)
	app.HandlerFunc(http.MethodDelete, version, "/bucky/libs/installs", api.removeBuckyLibsInstall, administrationAccess)

	app.HandlerFunc(http.MethodGet, version, "/bucky/models", api.listBuckyModels, managementAccess)
	app.HandlerFunc(http.MethodGet, version, "/bucky/models/catalog", api.listBuckyCatalog, managementAccess)
	app.HandlerFunc(http.MethodPost, version, "/bucky/models/pull", api.pullBuckyModel, administrationAccess)
	app.HandlerFunc(http.MethodGet, version, "/bucky/models/{model}/details", api.detailsBuckyModel, managementAccess)
	app.HandlerFunc(http.MethodDelete, version, "/bucky/models/{model}", api.removeBuckyModel, administrationAccess)

	// -------------------------------------------------------------------------
	// Cross-backend infrastructure.

	app.HandlerFunc(http.MethodGet, version, "/pool/budget", api.poolBudget, managementAccess)
	app.HandlerFunc(http.MethodGet, version, "/devices", api.listDevices, managementAccess)
	app.HandlerFunc(http.MethodGet, version, "/diagnose", api.diagnose, managementAccess)

	// -------------------------------------------------------------------------
	// Accuracy app — model code-recall comparison.

	app.HandlerFunc(http.MethodGet, version, "/accuracy/functions", api.listAccuracyFunctions, managementAccess)
	app.HandlerFunc(http.MethodPost, version, "/accuracy/test", api.runAccuracy, managementAccess)

	// -------------------------------------------------------------------------
	// Efficiency app — model throughput comparison.

	app.HandlerFunc(http.MethodPost, version, "/efficiency/run", api.runEfficiency, managementAccess)

	// Auth is handled by the auth service for these calls.
	app.HandlerFunc(http.MethodPost, version, "/security/token/create", api.createToken, managementAccess)
	app.HandlerFunc(http.MethodGet, version, "/security/keys", api.listKeys, managementAccess)
	app.HandlerFunc(http.MethodPost, version, "/security/keys/add", api.addKey, managementAccess)
	app.HandlerFunc(http.MethodPost, version, "/security/keys/remove/{keyid}", api.removeKey, managementAccess)
}
