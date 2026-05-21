package toolapp

import (
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mid"
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
	Log         *logger.Logger
	AuthClient  *authclient.Client
	Pool        *pool.Pool
	Libs        *libs.Libs
	Models      *models.Models
	BuckyLibs   *buckylibs.Libs
	BuckyModels *buckymodels.Models
}

// Routes adds specific routes for this group.
func Routes(app *web.App, cfg Config) {
	const version = "v1"

	api := newApp(cfg)

	auth := mid.Authenticate(cfg.AuthClient, false, "")
	authAdmin := mid.Authenticate(cfg.AuthClient, true, "")

	// -------------------------------------------------------------------------
	// Kronk (llama.cpp) backend — libs, models, catalog.

	app.HandlerFunc(http.MethodGet, version, "/kronk/libs", api.listLibs, auth)
	app.HandlerFunc(http.MethodPost, version, "/kronk/libs/pull", api.pullLibs, authAdmin)
	app.HandlerFunc(http.MethodGet, version, "/kronk/libs/combinations", api.listLibsCombinations, auth)
	app.HandlerFunc(http.MethodGet, version, "/kronk/libs/installs", api.listLibsInstalls, auth)
	app.HandlerFunc(http.MethodDelete, version, "/kronk/libs/installs", api.removeLibsInstall, authAdmin)

	app.HandlerFunc(http.MethodGet, version, "/kronk/models", api.listModels, auth)
	app.HandlerFunc(http.MethodGet, version, "/kronk/models/", api.missingModel, auth)
	app.HandlerFunc(http.MethodGet, version, "/kronk/models/{model}", api.showModel, auth)
	app.HandlerFunc(http.MethodGet, version, "/kronk/models/ps", api.modelPS, auth)
	app.HandlerFunc(http.MethodPost, version, "/kronk/models/index", api.indexModels, authAdmin)
	app.HandlerFunc(http.MethodPost, version, "/kronk/models/pull", api.pullModels, authAdmin)
	app.HandlerFunc(http.MethodPost, version, "/kronk/models/vram", api.calculateVRAM, auth)
	app.HandlerFunc(http.MethodPost, version, "/kronk/models/unload", api.unloadModel, authAdmin)
	app.HandlerFunc(http.MethodDelete, version, "/kronk/models/{model}", api.removeModel, authAdmin)

	app.HandlerFunc(http.MethodGet, version, "/kronk/catalog", api.listCatalog, auth)
	app.HandlerFunc(http.MethodPost, version, "/kronk/catalog/reconcile", api.reconcileCatalog, authAdmin)
	app.HandlerFunc(http.MethodPost, version, "/kronk/catalog/lookup", api.lookupCatalog, auth)
	app.HandlerFunc(http.MethodPost, version, "/kronk/catalog/resolve", api.resolveCatalog, authAdmin)
	app.HandlerFunc(http.MethodGet, version, "/kronk/catalog/{id...}", api.showCatalog, auth)
	app.HandlerFunc(http.MethodDelete, version, "/kronk/catalog/{id...}", api.removeCatalog, authAdmin)

	// -------------------------------------------------------------------------
	// Bucky (whisper.cpp) backend — libs, models. Whisper has no
	// resolver-backed catalog: the bundled short-name list is exposed
	// under /bucky/models/catalog.

	app.HandlerFunc(http.MethodGet, version, "/bucky/libs", api.listBuckyLibs, auth)
	app.HandlerFunc(http.MethodPost, version, "/bucky/libs/pull", api.pullBuckyLibs, authAdmin)
	app.HandlerFunc(http.MethodGet, version, "/bucky/libs/combinations", api.listBuckyLibsCombinations, auth)
	app.HandlerFunc(http.MethodGet, version, "/bucky/libs/installs", api.listBuckyLibsInstalls, auth)
	app.HandlerFunc(http.MethodDelete, version, "/bucky/libs/installs", api.removeBuckyLibsInstall, authAdmin)

	app.HandlerFunc(http.MethodGet, version, "/bucky/models", api.listBuckyModels, auth)
	app.HandlerFunc(http.MethodGet, version, "/bucky/models/catalog", api.listBuckyCatalog, auth)
	app.HandlerFunc(http.MethodPost, version, "/bucky/models/pull", api.pullBuckyModel, authAdmin)
	app.HandlerFunc(http.MethodGet, version, "/bucky/models/{model}/details", api.detailsBuckyModel, auth)
	app.HandlerFunc(http.MethodDelete, version, "/bucky/models/{model}", api.removeBuckyModel, authAdmin)

	// -------------------------------------------------------------------------
	// Cross-backend infrastructure.

	app.HandlerFunc(http.MethodGet, version, "/pool/budget", api.poolBudget, auth)
	app.HandlerFunc(http.MethodGet, version, "/devices", api.listDevices, auth)

	// Auth is handled by the auth service for these calls.
	app.HandlerFunc(http.MethodPost, version, "/security/token/create", api.createToken)
	app.HandlerFunc(http.MethodGet, version, "/security/keys", api.listKeys)
	app.HandlerFunc(http.MethodPost, version, "/security/keys/add", api.addKey)
	app.HandlerFunc(http.MethodPost, version, "/security/keys/remove/{keyid}", api.removeKey)
}
