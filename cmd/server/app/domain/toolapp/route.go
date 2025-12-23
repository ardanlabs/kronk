package toolapp

import (
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mid"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/kronk/cache"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
)

// Config contains all the mandatory systems required by handlers.
type Config struct {
	Log        *logger.Logger
	AuthClient *authclient.Client
	Cache      *cache.Cache
	catalog    *catalog.Catalog
	templates  *templates.Templates
}

// Routes adds specific routes for this group.
func Routes(app *web.App, cfg Config) {
	const version = ""

	api := newApp(cfg)

	auth := mid.Authenticate(true, cfg.AuthClient, false, "")

	app.HandlerFunc(http.MethodGet, version, "/v1/libs", api.listLibs, auth)
	app.HandlerFunc(http.MethodPost, version, "/v1/libs/pull", api.pullLibs, auth)

	app.HandlerFunc(http.MethodGet, version, "/v1/models", api.listModels, auth)
	app.HandlerFunc(http.MethodGet, version, "/v1/models/{model}", api.showModel, auth)
	app.HandlerFunc(http.MethodGet, version, "/v1/models/ps", api.modelPS, auth)
	app.HandlerFunc(http.MethodPost, version, "/v1/models/index", api.indexModels, auth)
	app.HandlerFunc(http.MethodPost, version, "/v1/models/pull", api.pullModels, auth)
	app.HandlerFunc(http.MethodDelete, version, "/v1/models/{model}", api.removeModel, auth)

	app.HandlerFunc(http.MethodGet, version, "/v1/catalog", api.listCatalog, auth)
	app.HandlerFunc(http.MethodGet, version, "/v1/catalog/filter/{filter}", api.listCatalog, auth)
	app.HandlerFunc(http.MethodGet, version, "/v1/catalog/{model}", api.showCatalogModel, auth)
	app.HandlerFunc(http.MethodPost, version, "/v1/catalog/pull/{model}", api.pullCatalog, auth)

	// Auth is handled by the auth service for these calls.
	app.HandlerFunc(http.MethodPost, version, "/v1/security/token/create", api.createToken)
	app.HandlerFunc(http.MethodGet, version, "/v1/security/keys", api.listKeys)
	app.HandlerFunc(http.MethodPost, version, "/v1/security/keys/add", api.addKey)
	app.HandlerFunc(http.MethodPost, version, "/v1/security/keys/remove/{keyid}", api.removeKey)
}
