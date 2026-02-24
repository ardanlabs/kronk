package downapp

import (
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

// Config contains all the mandatory systems required by handlers.
type Config struct {
	Log        *logger.Logger
	ModelsPath string
}

// Routes adds specific routes for this group.
func Routes(app *web.App, cfg Config) {
	api := newApp(cfg)

	app.RawHandlerFunc(http.MethodGet, "", "/download/{path...}", api.handle)
	app.RawHandlerFunc(http.MethodHead, "", "/download/{path...}", api.handle)
}
