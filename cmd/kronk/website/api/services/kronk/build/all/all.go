// Package all binds all the routes into the specified app.
package all

import (
	"github.com/ardanlabs/kronk/cmd/kronk/website/app/domain/checkapp"
	"github.com/ardanlabs/kronk/cmd/kronk/website/app/domain/toolapp"
	"github.com/ardanlabs/kronk/cmd/kronk/website/app/sdk/mux"
	"github.com/ardanlabs/kronk/cmd/kronk/website/foundation/web"
)

// Routes constructs the add value which provides the implementation of
// of RouteAdder for specifying what routes to bind to this instance.
func Routes() add {
	return add{}
}

type add struct{}

// Add implements the RouterAdder interface.
func (add) Add(app *web.App, cfg mux.Config) {
	checkapp.Routes(app, checkapp.Config{
		Build: cfg.Build,
		Log:   cfg.Log,
	})

	toolapp.Routes(app, toolapp.Config{
		Log:     cfg.Log,
		Auth:    cfg.Auth,
		KrnMngr: cfg.KrnMngr,
	})
}
