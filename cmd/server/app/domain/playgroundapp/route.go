package playgroundapp

import (
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/cache"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mid"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// Config contains all the mandatory systems required by handlers.
type Config struct {
	Log        *logger.Logger
	AuthClient *authclient.Client
	Cache      *cache.Cache
	Models     *models.Models
}

// Routes adds specific routes for this group.
func Routes(app *web.App, cfg Config) {
	const version = "v1"

	api := newApp(cfg)

	auth := mid.Authenticate(cfg.AuthClient, false, "playground")

	app.HandlerFunc(http.MethodPost, version, "/playground/sessions", api.createSession, auth)
	app.HandlerFunc(http.MethodDelete, version, "/playground/sessions/{id}", api.deleteSession, auth)
	app.HandlerFunc(http.MethodPost, version, "/playground/chat/completions", api.chatCompletions, auth)
}
