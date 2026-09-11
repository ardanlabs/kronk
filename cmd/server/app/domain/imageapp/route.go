// Package imageapp provides image-generation API endpoints.
package imageapp

import (
	"net/http"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/malinaprogress"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mid"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/pool"
)

// Config contains all the mandatory systems required by handlers.
type Config struct {
	Log               *logger.Logger
	AuthClient        *authclient.Client
	Pool              *pool.Pool
	MalinaProgress    *malinaprogress.Broker
	AuthorizationMode auth.Mode
	InferenceTimeout  time.Duration
}

// Routes adds image-generation routes.
func Routes(app *web.App, cfg Config) {
	const version = "v1"

	api := newApp(cfg)
	inferenceAccess := mid.NewAccess(cfg.AuthClient, cfg.AuthorizationMode, false).Inference("image-generations")

	app.HandlerFunc(http.MethodGet, version, "/images/events", api.events, inferenceAccess)
	app.HandlerFunc(http.MethodPost, version, "/images/generations", api.generations, mid.Timeout(cfg.InferenceTimeout), inferenceAccess)
	app.HandlerFunc(http.MethodPost, version, "/images/edits", api.edits, mid.Timeout(cfg.InferenceTimeout), inferenceAccess)
}
