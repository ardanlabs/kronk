package playgroundapp

import (
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mid"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/pool"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// Config contains all the mandatory systems required by handlers.
type Config struct {
	Log                    *logger.Logger
	AuthClient             *authclient.Client
	Pool                   *pool.Pool
	Models                 *models.Models
	AuthorizationMode      auth.Mode
	LegacyManagementAccess bool
}

// Routes adds specific routes for this group.
func Routes(app *web.App, cfg Config) {
	const version = "v1"

	api := newApp(cfg)

	playgroundAccess := mid.NewAccess(cfg.AuthClient, cfg.AuthorizationMode, cfg.LegacyManagementAccess).Playground()

	app.HandlerFunc(http.MethodPost, version, "/playground/sessions", api.createSession, playgroundAccess)
	app.HandlerFunc(http.MethodDelete, version, "/playground/sessions/{id}", api.deleteSession, playgroundAccess)
	app.HandlerFunc(http.MethodPost, version, "/playground/chat/completions", api.chatCompletions, playgroundAccess)
}
