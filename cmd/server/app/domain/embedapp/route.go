package embedapp

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
	const version = "v1"

	bearer := mid.Bearer(cfg.Security.Auth)
	authorizeEndpoint := mid.Authorize(cfg.Security.Auth, false, "embeddings")

	api := newApp(cfg.Log, cfg.Cache)

	app.HandlerFunc(http.MethodPost, version, "/embeddings", api.embeddings, bearer, authorizeEndpoint)
}
