// Package checkapp maintains the app layer api for the check domain.
package checkapp

import (
	"context"
	"net/http"
	"os"
	"runtime"

	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

type app struct {
	build string
	log   *logger.Logger
}

func newApp(cfg Config) *app {
	return &app{
		build: cfg.Build,
		log:   cfg.Log,
	}
}

func (a *app) readiness(ctx context.Context, r *http.Request) web.Encoder {
	return nil
}

func (a *app) liveness(ctx context.Context, r *http.Request) web.Encoder {
	host, err := os.Hostname()
	if err != nil {
		host = "unavailable"
	}

	info := Info{
		Status:     "up",
		Build:      a.build,
		Host:       host,
		GOMAXPROCS: runtime.GOMAXPROCS(0),
	}

	return info
}
