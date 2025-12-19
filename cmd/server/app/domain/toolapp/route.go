package toolapp

import (
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mid"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/kronk/cache"
	"github.com/ardanlabs/kronk/sdk/tools/security"
)

// Config contains all the mandatory systems required by handlers.
type Config struct {
	Log      *logger.Logger
	Security *security.Security
	Cache    *cache.Cache
}

// Routes adds specific routes for this group.
func Routes(app *web.App, cfg Config) {
	const version = ""

	bearer := mid.Bearer(cfg.Security.Auth)
	authorizeAdmin := mid.Authorize(cfg.Security.Auth, true, "")

	api := newApp(cfg.Log, cfg.Cache, cfg.Security)

	app.HandlerFunc(http.MethodPost, version, "/v1/libs/pull", api.pullLibs, bearer)

	app.HandlerFunc(http.MethodGet, version, "/v1/models", api.listModels, bearer)
	app.HandlerFunc(http.MethodGet, version, "/v1/models/{model}", api.showModel, bearer)
	app.HandlerFunc(http.MethodGet, version, "/v1/models/ps", api.modelPS, bearer)
	app.HandlerFunc(http.MethodPost, version, "/v1/models/pull", api.pullModels, bearer)
	app.HandlerFunc(http.MethodDelete, version, "/v1/models/{model}", api.removeModel, bearer)

	app.HandlerFunc(http.MethodGet, version, "/v1/catalog", api.listCatalog, bearer)
	app.HandlerFunc(http.MethodGet, version, "/v1/catalog/filter/{filter}", api.listCatalog, bearer)
	app.HandlerFunc(http.MethodGet, version, "/v1/catalog/{model}", api.showCatalogModel, bearer)
	app.HandlerFunc(http.MethodPost, version, "/v1/catalog/pull/{model}", api.pullCatalog, bearer)

	app.HandlerFunc(http.MethodPost, version, "/v1/security/token/create", api.createToken, bearer, authorizeAdmin)
	app.HandlerFunc(http.MethodGet, version, "/v1/security/keys", api.listKeys, bearer, authorizeAdmin)
	app.HandlerFunc(http.MethodPost, version, "/v1/security/keys/add", api.addKey, bearer, authorizeAdmin)
	app.HandlerFunc(http.MethodPost, version, "/v1/security/keys/remove/{keyid}", api.removeKey, bearer, authorizeAdmin)
}
