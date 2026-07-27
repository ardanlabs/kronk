package rerankapp

import (
	"net/http"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mid"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/pool"
)

// Config contains all the mandatory systems required by handlers.
type Config struct {
	Log              *logger.Logger
	AuthClient       *authclient.Client
	Pool             *pool.Pool
	InferenceTimeout time.Duration
}

// Routes adds specific routes for this group.
func Routes(app *web.App, cfg Config) {
	const version = "v1"

	api := newApp(cfg)

	auth := mid.Authenticate(cfg.AuthClient, false, "rerank")

	timeout := mid.Timeout(cfg.InferenceTimeout)
	app.HandlerFunc(http.MethodPost, version, "/rerank", api.rerank, timeout, auth)
	app.HandlerFunc(http.MethodPost, version, "/reranking", api.rerank, timeout, auth)
}
