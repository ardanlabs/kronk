package mngtapp

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

	app.HandlerFunc(http.MethodGet, version, "/mngt/libs", api.libs, bearer)
	app.HandlerFunc(http.MethodGet, version, "/mngt/model/list", api.list, bearer)
	app.HandlerFunc(http.MethodGet, version, "/mngt/model/show/{model}", api.show, bearer)
	app.HandlerFunc(http.MethodPost, version, "/mngt/model/pull", api.pull, bearer)
	app.HandlerFunc(http.MethodPost, version, "/mngt/model/remove", api.remove, bearer)
}
