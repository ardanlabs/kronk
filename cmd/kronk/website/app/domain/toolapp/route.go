package toolapp

import (
	"net/http"

	"github.com/ardanlabs/kronk/cmd/kronk/website/app/sdk/auth"
	"github.com/ardanlabs/kronk/cmd/kronk/website/app/sdk/krn"
	"github.com/ardanlabs/kronk/cmd/kronk/website/app/sdk/mid"
	"github.com/ardanlabs/kronk/cmd/kronk/website/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/kronk/website/foundation/web"
)

// Config contains all the mandatory systems required by handlers.
type Config struct {
	Log     *logger.Logger
	Auth    *auth.Auth
	KrnMngr *krn.Manager
}

// Routes adds specific routes for this group.
func Routes(app *web.App, cfg Config) {
	const version = "v1"

	bearer := mid.Bearer(cfg.Auth)

	api := newApp(cfg.Log, cfg.KrnMngr)

	app.HandlerFunc(http.MethodPost, version, "/libs", api.libs, bearer)
	app.HandlerFunc(http.MethodGet, version, "/models", api.list, bearer)
	app.HandlerFunc(http.MethodGet, version, "/models/{model}", api.show, bearer)
	app.HandlerFunc(http.MethodPost, version, "/models/pull", api.pull, bearer)
	app.HandlerFunc(http.MethodDelete, version, "/models/{model}", api.remove, bearer)
}
