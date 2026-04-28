// Package toolapp provides endpoints to handle tool management.
package toolapp

import (
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/cache"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

type app struct {
	log        *logger.Logger
	cache      *cache.Cache
	authClient *authclient.Client
	libs       *libs.Libs
	models     *models.Models
}

func newApp(cfg Config) *app {
	return &app{
		log:        cfg.Log,
		cache:      cfg.Cache,
		authClient: cfg.AuthClient,
		libs:       cfg.Libs,
		models:     cfg.Models,
	}
}
