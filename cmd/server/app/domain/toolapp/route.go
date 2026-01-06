package toolapp

import (
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/cache"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mid"
	"github.com/ardanlabs/kronk/cmd/server/foundation/async"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
)

// Config contains all the mandatory systems required by handlers.
type Config struct {
	Log        *logger.Logger
	AuthClient *authclient.Client
	Cache      *cache.Cache
	Libs       *libs.Libs
	Models     *models.Models
	Catalog    *catalog.Catalog
	Templates  *templates.Templates
	Async      *async.Async
}

// Routes adds specific routes for this group.
func Routes(app *web.App, cfg Config) {
	const version = "v1"

	api := newApp(cfg)

	auth := mid.Authenticate(cfg.AuthClient, false, "")
	authAdmin := mid.Authenticate(cfg.AuthClient, true, "")

	app.HandlerFunc(http.MethodGet, version, "/libs", api.listLibs, auth)
	app.HandlerFunc(http.MethodPost, version, "/libs/pull", api.pullLibs, authAdmin)

	app.HandlerFunc(http.MethodGet, version, "/models", api.listModels, auth)
	app.HandlerFunc(http.MethodGet, version, "/models/", api.missingModel, auth)
	app.HandlerFunc(http.MethodGet, version, "/models/{model}", api.showModel, auth)
	app.HandlerFunc(http.MethodGet, version, "/models/ps", api.modelPS, auth)
	app.HandlerFunc(http.MethodPost, version, "/models/index", api.indexModels, authAdmin)
	app.HandlerFunc(http.MethodPost, version, "/models/pull", api.pullModels, authAdmin)
	app.HandlerFunc(http.MethodGet, version, "/models/pull/{sessionid}", api.pullModelsSession, authAdmin)
	app.HandlerFunc(http.MethodDelete, version, "/models/{model}", api.removeModel, authAdmin)

	app.HandlerFunc(http.MethodGet, version, "/catalog", api.listCatalog, auth)
	app.HandlerFunc(http.MethodGet, version, "/catalog/filter/{filter}", api.listCatalog, auth)
	app.HandlerFunc(http.MethodGet, version, "/catalog/{model}", api.showCatalogModel, auth)
	app.HandlerFunc(http.MethodPost, version, "/catalog/pull/{model}", api.pullCatalog, auth)

	// Auth is handled by the auth service for these calls.
	app.HandlerFunc(http.MethodPost, version, "/security/token/create", api.createToken)
	app.HandlerFunc(http.MethodGet, version, "/security/keys", api.listKeys)
	app.HandlerFunc(http.MethodPost, version, "/security/keys/add", api.addKey)
	app.HandlerFunc(http.MethodPost, version, "/security/keys/remove/{keyid}", api.removeKey)
}
