// Package toolapp provides endpoints to handle tool management.
package toolapp

import (
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/sdk/pool"
	buckylibs "github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

type app struct {
	log         *logger.Logger
	pool        *pool.Pool
	authClient  *authclient.Client
	libs        *libs.Libs
	models      *models.Models
	buckyLibs   *buckylibs.Libs
	buckyModels *buckymodels.Models
}

func newApp(cfg Config) *app {
	return &app{
		log:         cfg.Log,
		pool:        cfg.Pool,
		authClient:  cfg.AuthClient,
		libs:        cfg.Libs,
		models:      cfg.Models,
		buckyLibs:   cfg.BuckyLibs,
		buckyModels: cfg.BuckyModels,
	}
}
