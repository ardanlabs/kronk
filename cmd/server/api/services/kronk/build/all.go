// Package build binds all the routes into the specified app.
package build

import (
	"github.com/ardanlabs/kronk/cmd/server/app/domain/chatapp"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/checkapp"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/embedapp"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mux"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

// Routes constructs the all value which provides the implementation of
// of RouteAdder for specifying what routes to bind to this instance.
func Routes() all {
	return all{}
}

type all struct{}

// Add implements the RouterAdder interface.
func (all) Add(app *web.App, cfg mux.Config) {
	checkapp.Routes(app, checkapp.Config{
		Build: cfg.Build,
		Log:   cfg.Log,
	})

	toolapp.Routes(app, toolapp.Config{
		Log:      cfg.Log,
		Security: cfg.Security,
		Cache:    cfg.Cache,
	})

	chatapp.Routes(app, chatapp.Config{
		Log:      cfg.Log,
		Security: cfg.Security,
		Cache:    cfg.Cache,
	})

	embedapp.Routes(app, embedapp.Config{
		Log:      cfg.Log,
		Security: cfg.Security,
		Cache:    cfg.Cache,
	})
}
