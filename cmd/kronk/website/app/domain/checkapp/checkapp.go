// Package checkapp maintains the app layer api for the check domain.
package checkapp

import (
	"context"
	"net/http"
	"os"
	"runtime"

	"github.com/ardanlabs/kronk/cmd/kronk/website/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/kronk/website/foundation/web"
)

type app struct {
	build string
	log   *logger.Logger
}

func newApp(build string, log *logger.Logger) *app {
	return &app{
		build: build,
		log:   log,
	}
}

// readiness checks if the system is ready and if not will return a 500 status.
func (a *app) readiness(ctx context.Context, r *http.Request) web.Encoder {
	return nil
}

// liveness returns simple status info if the service is alive.
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
