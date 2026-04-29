// Package toolapp provides endpoints to handle tool management.
package toolapp

import (
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/sdk/pool"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

type app struct {
	log        *logger.Logger
	pool       *pool.Pool
	authClient *authclient.Client
	libs       *libs.Libs
	models     *models.Models
}

func newApp(cfg Config) *app {
	return &app{
		log:        cfg.Log,
		pool:       cfg.Pool,
		authClient: cfg.AuthClient,
		libs:       cfg.Libs,
		models:     cfg.Models,
	}
}
