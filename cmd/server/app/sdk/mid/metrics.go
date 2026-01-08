package mid

import (
	"context"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
)

// Metrics updates program counters.
func Metrics() web.MidFunc {
	m := func(next web.HandlerFunc) web.HandlerFunc {
		h := func(ctx context.Context, r *http.Request) web.Encoder {
			resp := next(ctx, r)

			n := metrics.AddRequests()

			if n%1000 == 0 {
				metrics.AddGoroutines()
			}

			if checkIsError(resp) != nil {
				metrics.AddErrors()
			}

			return resp
		}

		return h
	}

	return m
}
