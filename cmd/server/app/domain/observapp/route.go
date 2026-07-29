// Package observapp provides admin endpoints for session observability.
package observapp

import (
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mid"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/session"
)

// Config contains the systems required by the context session handlers.
type Config struct {
	AuthClient       *authclient.Client
	Sessions         *session.Tracker
	AdminAuthEnabled bool
}

// Routes adds context session observability routes.
func Routes(app *web.App, cfg Config) {
	const version = "v1"

	api := newApp(cfg.Sessions)
	auth := mid.Authenticate(cfg.AuthClient, false, "")
	if cfg.AdminAuthEnabled {
		auth = mid.Authenticate(cfg.AuthClient, true, "")
	}

	app.HandlerFunc(http.MethodGet, version, "/kronk/sessions/status", api.status, auth)
	app.HandlerFunc(http.MethodGet, version, "/kronk/sessions/summary", api.summary, auth)
	app.HandlerFunc(http.MethodGet, version, "/kronk/sessions/{state}", api.list, auth)
}
