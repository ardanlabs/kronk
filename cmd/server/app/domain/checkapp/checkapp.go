// Package checkapp maintains the app layer api for the check domain.
package checkapp

import (
	"context"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"

	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

type app struct {
	build    string
	log      *logger.Logger
	host     string
	maxProcs int
}

func newApp(cfg Config) *app {
	host, err := os.Hostname()
	if err != nil {
		host = "unavailable"
	}

	maxProcs := runtime.GOMAXPROCS(0)

	return &app{
		build:    cfg.Build,
		log:      cfg.Log,
		host:     host,
		maxProcs: maxProcs,
	}
}

func (a *app) readiness(ctx context.Context, r *http.Request) web.Encoder {
	return nil
}

func (a *app) liveness(ctx context.Context, r *http.Request) web.Encoder {
	info := Info{
		Status:     "up",
		Build:      a.build,
		Host:       a.host,
		GOMAXPROCS: a.maxProcs,
		OS:         runtime.GOOS,
	}

	// Report the running binary's provenance, never the current checkout.
	if build, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range build.Settings {
			switch setting.Key {
			case "vcs.revision":
				info.Revision = setting.Value
			case "vcs.time":
				info.CommitTime = setting.Value
			}
		}
	}

	return info
}
